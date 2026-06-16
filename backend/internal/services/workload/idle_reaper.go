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
	if cl == nil {
		return nil
	}
	if threshold <= 0 {
		threshold = defaultIdleTimeout
	}
	pods, err := cl.ListPodsByLabel(ctx, "", idleResourceTypeLabel)
	if err != nil {
		return err
	}
	for _, pod := range pods {
		reapIdleInteractivePod(ctx, cl, store, pod, now, threshold, deletionEnabled)
	}
	return reapStaleJobRecords(ctx, cl, store, now)
}

func reapIdleInteractivePod(
	ctx context.Context,
	cl *cluster.Client,
	store platform.RecordStore,
	pod cluster.PodInfo,
	now time.Time,
	threshold time.Duration,
	deletionEnabled bool,
) {
	resourceType := strings.TrimSpace(pod.Labels[idleResourceTypeLabel])
	if !strings.HasPrefix(resourceType, interactiveResourcePrefix) {
		return
	}
	lastActivity, ok := parseLastActivity(pod.Annotations)
	if !ok {
		return
	}
	idleFor := now.Sub(lastActivity)
	if idleFor <= threshold {
		return
	}
	jobID := strings.TrimSpace(pod.Labels[cluster.LabelJobID])
	if !deletionEnabled {
		slog.Warn("idle reaper: automated pod deletion disabled, leaving idle pod running",
			"pod", pod.Name, "namespace", pod.Namespace, "job_id", jobID)
		return
	}
	if jobID != "" {
		reapIdleJobPod(ctx, cl, store, pod, jobID, idleFor, threshold)
		return
	}
	if err := cl.DeletePod(ctx, pod.Namespace, pod.Name); err != nil {
		slog.Warn("idle reaper: delete idle pod failed", "pod", pod.Name, "namespace", pod.Namespace, "error", err)
	}
}

func reapIdleJobPod(
	ctx context.Context,
	cl *cluster.Client,
	store platform.RecordStore,
	pod cluster.PodInfo,
	jobID string,
	idleFor time.Duration,
	threshold time.Duration,
) {
	if _, err := cl.CleanupJobResources(ctx, pod.Namespace, jobID); err != nil {
		slog.Warn("idle reaper: cleanup job resources failed", "job_id", jobID, "namespace", pod.Namespace, "error", err)
		return
	}
	reason := "Reaped: idle timeout exceeded (idle_for=" + idleFor.String() + ", threshold=" + threshold.String() + ")"
	markJobFailed(ctx, store, jobID, reason)
}

func parseLastActivity(annotations map[string]string) (time.Time, bool) {
	raw := strings.TrimSpace(annotations[lastActivityAnnotation])
	if raw == "" {
		return time.Time{}, false
	}
	value, err := time.Parse(time.RFC3339, raw)
	return value, err == nil
}

func reapStaleJobRecords(ctx context.Context, cl *cluster.Client, store platform.RecordStore, now time.Time) error {
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
		markJobFailed(ctx, store, record.ID, "Resource no longer exists in cluster")
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
		return reapIdleInteractiveWorkloads(
			ctx,
			app.Cluster,
			app.Store,
			time.Now().UTC(),
			app.Config.WorkloadIdleTimeout,
			app.Config.AutomatedPodDeletion,
		)
	})
}
