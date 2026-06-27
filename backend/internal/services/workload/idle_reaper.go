package workload

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/platform/cluster"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

const (
	defaultIdleTimeout        = 2 * time.Hour
	idleResourceTypeLabel     = "platform-go/resource-type"
	lastActivityAnnotation    = "platform-go/last-activity"
	interactiveResourcePrefix = "interactive-"
	staleJobGracePeriod       = 5 * time.Minute
)

type idleReaperRun struct {
	cl              *cluster.Client
	store           platform.RecordStore
	release         reservationReleaseFunc
	now             time.Time
	threshold       time.Duration
	deletionEnabled bool
}

// reapIdleInteractiveWorkloads deletes interactive workload pods whose last
// activity timestamp exceeds the configured idle timeout, then fails stale active
// job records whose Kubernetes resources are gone. It ports the reference idle
// reaper into workload-service, where job status is owned.
func reapIdleInteractiveWorkloads(
	ctx context.Context,
	cl *cluster.Client,
	store platform.RecordStore,
	now time.Time,
	threshold time.Duration,
	deletionEnabled bool,
) error {
	return reapIdleInteractiveWorkloadsWithReservationRelease(ctx, cl, store, nil, now, threshold, deletionEnabled)
}

func reapIdleInteractiveWorkloadsWithReservationRelease(
	ctx context.Context,
	cl *cluster.Client,
	store platform.RecordStore,
	release reservationReleaseFunc,
	now time.Time,
	threshold time.Duration,
	deletionEnabled bool,
) error {
	if cl == nil {
		return nil
	}
	if threshold <= 0 {
		threshold = defaultIdleTimeout
	}
	run := idleReaperRun{cl: cl, store: store, release: release, now: now, threshold: threshold, deletionEnabled: deletionEnabled}
	return reapIdleInteractiveWorkloadsRun(ctx, run)
}

func reapIdleInteractiveWorkloadsRun(ctx context.Context, run idleReaperRun) error {
	pods, err := run.cl.ListPodsByLabel(ctx, "", idleResourceTypeLabel)
	if err != nil {
		return err
	}
	for _, pod := range pods {
		reapIdleInteractivePod(ctx, run, pod)
	}
	return reapStaleJobRecords(ctx, run.cl, run.store, run.release, run.now)
}

func reapIdleInteractivePod(
	ctx context.Context,
	run idleReaperRun,
	pod cluster.PodInfo,
) {
	resourceType := strings.TrimSpace(pod.Labels[idleResourceTypeLabel])
	if !strings.HasPrefix(resourceType, interactiveResourcePrefix) {
		return
	}
	lastActivity, ok := parseLastActivity(pod.Annotations)
	if !ok {
		return
	}
	idleFor := run.now.Sub(lastActivity)
	if idleFor <= run.threshold {
		return
	}
	jobID := strings.TrimSpace(pod.Labels[cluster.LabelJobID])
	if !run.deletionEnabled {
		slog.Warn("idle reaper: automated pod deletion disabled, leaving idle pod running",
			"pod", pod.Name, "namespace", pod.Namespace, "job_id", jobID)
		return
	}
	if jobID != "" {
		reapIdleJobPod(ctx, run, pod, jobID, idleFor)
		return
	}
	if err := run.cl.DeletePod(ctx, pod.Namespace, pod.Name); err != nil {
		slog.Warn("idle reaper: delete idle pod failed", "pod", pod.Name, "namespace", pod.Namespace, "error", err)
	}
}

func reapIdleJobPod(
	ctx context.Context,
	run idleReaperRun,
	pod cluster.PodInfo,
	jobID string,
	idleFor time.Duration,
) {
	if _, err := run.cl.CleanupJobResources(ctx, pod.Namespace, jobID); err != nil {
		slog.Warn("idle reaper: cleanup job resources failed", "job_id", jobID, "namespace", pod.Namespace, "error", err)
		return
	}
	reason := "Reaped: idle timeout exceeded (idle_for=" + idleFor.String() + ", threshold=" + run.threshold.String() + ")"
	markJobFailed(ctx, run.store, run.release, jobID, reason)
}

func parseLastActivity(annotations map[string]string) (time.Time, bool) {
	raw := strings.TrimSpace(annotations[lastActivityAnnotation])
	if raw == "" {
		return time.Time{}, false
	}
	value, err := time.Parse(time.RFC3339, raw)
	return value, err == nil
}

func reapStaleJobRecords(ctx context.Context, cl *cluster.Client, store platform.RecordStore, release reservationReleaseFunc, now time.Time) error {
	jobs := jobRepositoryFromStore(store)
	if cl == nil || jobs == nil {
		return nil
	}
	existingPods, err := existingJobPodSet(ctx, cl)
	if err != nil {
		return err
	}
	for _, record := range jobs.ListStaleJobCandidates(ctx, now) {
		namespace := shared.TextValue(record.Data, "namespace", "Namespace", "pod_namespace", "podNamespace")
		if namespace == "" || existingPods[namespace+"/"+record.ID] {
			continue
		}
		markJobFailed(ctx, store, release, record.ID, "Resource no longer exists in cluster")
	}
	return nil
}

func isStaleJobStatus(status string) bool {
	return status == "running" || status == "queued"
}

func existingJobPodSet(ctx context.Context, cl *cluster.Client) (map[string]bool, error) {
	pods, err := cl.ListPodsByLabel(ctx, "", cluster.LabelJobID)
	if err != nil {
		return nil, err
	}
	existing := make(map[string]bool, len(pods))
	for _, pod := range pods {
		jobID := strings.TrimSpace(pod.Labels[cluster.LabelJobID])
		if jobID != "" {
			existing[pod.Namespace+"/"+jobID] = true
		}
	}
	return existing, nil
}

func jobCreatedAt(data map[string]any, fallback time.Time) time.Time {
	for _, key := range []string{"created_at", "createdAt", "CreatedAt"} {
		switch value := data[key].(type) {
		case time.Time:
			return value
		case string:
			if parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(value)); err == nil {
				return parsed
			}
		}
	}
	return fallback
}

func registerIdleReaper(app *platform.App) {
	app.RegisterMaintenanceTaskForService(serviceName, "idle-reaper", func(ctx context.Context) error {
		return reapIdleInteractiveWorkloadsWithReservationRelease(
			ctx,
			app.Cluster,
			app.Store,
			schedulerReservationReleaseFuncForApp(app),
			time.Now().UTC(),
			app.Config.WorkloadIdleTimeout,
			app.Config.AutomatedPodDeletion,
		)
	})
}
