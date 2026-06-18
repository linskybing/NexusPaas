package schedulerquota

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/platform/cluster"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

// planWindowGracePeriod prevents flapping at weekly-window boundaries: a window that
// closed within this period is not yet enforced. Mirrors the reference reaper.
const planWindowGracePeriod = 5 * time.Minute

const (
	reasonNoActivePlan  = "No active resource plan"
	reasonValidityEnded = "Plan validity period expired"
	reasonWindowClosed  = "Plan window expired"
)

// planWindowReaper carries the dependencies shared across a single reaping cycle so
// the per-project/per-pod helpers stay small.
type planWindowReaper struct {
	cl              *cluster.Client
	evictor         workloadEvictFunc
	deletionEnabled bool
}

// reapExpiredPlanWindows evicts running job resources for projects whose bound
// resource plan window has closed (or that have no active plan). It is the
// scheduler-quota port of reference cron.StartPlanWindowReaper: scheduler-quota owns
// the enforcement decision and the cluster-side cleanup (as it already does for
// preemption), while the owning workload job's status transition goes through the
// workload eviction contract via evictor. A nil cluster client (degraded mode) is a
// no-op.
func reapExpiredPlanWindows(
	ctx context.Context,
	cl *cluster.Client,
	store platform.RecordStore,
	evictor workloadEvictFunc,
	now time.Time,
	deletionEnabled bool,
) error {
	return reapExpiredPlanWindowsWithReader(ctx, cl, store, newAdmissionReader(store), evictor, now, deletionEnabled)
}

func reapExpiredPlanWindowsWithReader(
	ctx context.Context,
	cl *cluster.Client,
	store platform.RecordStore,
	reader admissionReader,
	evictor workloadEvictFunc,
	now time.Time,
	deletionEnabled bool,
) error {
	repo := schedulerQuotaRepositoryFromStore(store)
	if cl == nil || store == nil || repo == nil || reader == nil || evictor == nil {
		return nil
	}
	reaper := planWindowReaper{cl: cl, evictor: evictor, deletionEnabled: deletionEnabled}
	checkTime := now.Add(-planWindowGracePeriod)
	for _, project := range reader.ListProjects(ctx) {
		reason := planWindowEvictionReason(ctx, repo, project, now, checkTime)
		if reason == "" {
			continue
		}
		slog.Info("plan window reaper: plan inactive", "project_id", project.ID, "reason", reason)
		reaper.evictProjectJobs(ctx, project.ID, reason)
	}
	return nil
}

// planWindowEvictionReason returns an eviction reason for a project, or "" when its
// plan is active. Plan validity (valid_from/valid_until) is checked at now with no
// grace; the weekly window is checked at checkTime (now - grace) to avoid flapping.
func planWindowEvictionReason(
	ctx context.Context,
	repo *recordStoreSchedulerQuotaRepository,
	project contracts.Record[map[string]any],
	now, checkTime time.Time,
) string {
	planID := shared.TextValue(project.Data, "plan_id", "planId", "resource_plan_id", "resourcePlanId")
	if planID == "" {
		return reasonNoActivePlan
	}
	plan, found := repo.GetPlan(ctx, planID)
	if !found {
		return reasonNoActivePlan
	}
	if planValidityExpired(plan.Data, now) {
		return reasonValidityEnded
	}
	if !admissionWeekWindowsContain(admissionWeekWindows(plan.Data), checkTime) {
		return reasonWindowClosed
	}
	return ""
}

// planValidityExpired reports whether the plan's validity period (valid_from /
// valid_until) excludes now. It reuses the admission plan-window time parsing.
func planValidityExpired(data map[string]any, now time.Time) bool {
	if from := admissionTimeValue(data, "valid_from", "validFrom", "ValidFrom"); from != nil && now.Before(*from) {
		return true
	}
	if until := admissionTimeValue(data, "valid_until", "validUntil", "ValidUntil"); until != nil && now.After(*until) {
		return true
	}
	return false
}

func (r planWindowReaper) evictProjectJobs(ctx context.Context, projectID, reason string) {
	namespaces, err := r.cl.ListProjectNamespaces(ctx, projectID)
	if err != nil {
		slog.Warn("plan window reaper: list namespaces failed", "project_id", projectID, "error", err)
		return
	}
	evicted := make(map[string]bool)
	for _, ns := range namespaces {
		pods, err := r.cl.ListPodsByLabel(ctx, ns, cluster.LabelJobID)
		if err != nil {
			slog.Warn("plan window reaper: list pods failed", "namespace", ns, "error", err)
			continue
		}
		for _, pod := range pods {
			r.evictPlanWindowPod(ctx, ns, pod, reason, evicted)
		}
	}
}

func (r planWindowReaper) evictPlanWindowPod(ctx context.Context, namespace string, pod cluster.PodInfo, reason string, evicted map[string]bool) {
	jobID := strings.TrimSpace(pod.Labels[cluster.LabelJobID])
	if jobID == "" {
		return
	}
	key := namespace + "/" + jobID
	if evicted[key] {
		return
	}
	if !r.deletionEnabled {
		slog.Warn("plan window reaper: pod deletion disabled, leaving job running",
			"pod", pod.Name, "namespace", namespace, "job_id", jobID, "reason", reason)
		return
	}
	// Mark the workload job evicted FIRST through the owner contract: that is the
	// authoritative state transition. Only after it succeeds do we delete the pods, so
	// a failed or unreachable eviction contract defers pod cleanup to the next cycle
	// instead of orphaning pods against a still-active job record (review Finding 1).
	if err := r.evictor(ctx, jobID, workloadEvictRequest{Reason: reason}); err != nil {
		slog.Warn("plan window reaper: mark job evicted failed, deferring pod cleanup",
			"job_id", jobID, "namespace", namespace, "reason", reason, "error", err)
		return
	}
	evicted[key] = true
	if _, err := r.cl.CleanupJobResources(ctx, namespace, jobID); err != nil {
		slog.Warn("plan window reaper: cleanup job resources failed",
			"job_id", jobID, "namespace", namespace, "reason", reason, "error", err)
	}
}

// registerPlanWindowReaper wires the reaper as a lease-gated maintenance task owned by
// scheduler-quota-service. It runs only once StartMaintenance is called, and no-ops
// cleanly when the workload eviction contract is unreachable (degraded/isolated mode).
func registerPlanWindowReaper(app *platform.App) {
	app.RegisterMaintenanceTaskForService(serviceName, "plan-window-reaper", func(ctx context.Context) error {
		evictor, err := newWorkloadEvictionClient(app)
		if err != nil {
			slog.Warn("plan window reaper: workload eviction contract unavailable, skipping cycle", "error", err)
			return nil
		}
		return reapExpiredPlanWindowsWithReader(ctx, app.Cluster, app.Store, newAdmissionReaderForApp(app), evictor, time.Now().UTC(), app.Config.PlanWindowPodDeletion)
	})
}
