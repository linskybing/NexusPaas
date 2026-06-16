package workload

import (
	"context"
	"log/slog"
	"strconv"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/platform/cluster"
)

const jobsResource = serviceName + ":jobs"

// activeJobStatuses are the job states a runtime-limit eviction may transition to
// "failed", mirroring the reference UpdateStatusIfActive guard (only live jobs).
var activeJobStatuses = map[string]bool{
	"pending": true, "queued": true, "scheduled": true, "running": true, "active": true,
}

// reapExpiredRuntimeWorkloads deletes platform-managed workloads whose runtime
// limit has elapsed. It is the microservice port of references/.../internal/cron
// .reapExpiredRuntimeWorkloads: resources carrying a job-id label are cleaned up as
// a unit (and the job marked failed, once), other resources are deleted directly.
func reapExpiredRuntimeWorkloads(ctx context.Context, cl *cluster.Client, store platform.RecordStore, now time.Time) error {
	if cl == nil {
		return nil
	}
	resources, err := cl.ListRuntimeLimited(ctx)
	if err != nil {
		return err
	}
	seenJobs := map[string]bool{}
	for _, res := range resources {
		if !runtimeLimitExpired(res.Labels, res.CreatedAt, now) || !isPlatformManaged(res.Labels) {
			continue
		}
		reapRuntimeResource(ctx, cl, store, res, seenJobs)
	}
	return nil
}

func reapRuntimeResource(ctx context.Context, cl *cluster.Client, store platform.RecordStore, res cluster.RuntimeResource, seenJobs map[string]bool) {
	jobID := res.Labels[cluster.LabelJobID]
	if jobID != "" {
		key := res.Namespace + "/" + jobID
		if seenJobs[key] {
			return
		}
		seenJobs[key] = true
		result, err := cl.CleanupJobResources(ctx, res.Namespace, jobID)
		if err != nil {
			slog.Warn("runtime reaper: cleanup job resources failed", "job_id", jobID, "namespace", res.Namespace, "error", err)
			return
		}
		slog.Info("runtime reaper: cleanup issued", "job_id", jobID, "namespace", res.Namespace, "deleted", result.Total())
		markJobFailed(ctx, store, jobID, "Runtime limit exceeded")
		return
	}
	if err := cl.DeleteResource(ctx, res.Kind, res.Namespace, res.Name); err != nil {
		slog.Warn("runtime reaper: delete expired workload failed", "kind", res.Kind, "namespace", res.Namespace, "name", res.Name, "error", err)
		return
	}
	slog.Info("runtime reaper: deleted expired workload", "kind", res.Kind, "namespace", res.Namespace, "name", res.Name)
}

// markJobFailed transitions a job record to "failed" only if it is currently
// active, mirroring the reference UpdateStatusIfActive. It is a best-effort no-op
// when the jobs read model has no record (the typed job domain lands in Phase 3).
func markJobFailed(ctx context.Context, store platform.RecordStore, jobID, reason string) {
	jobs := jobRepositoryFromStore(store)
	if jobs == nil || jobID == "" {
		return
	}
	record, found := jobs.FindJob(ctx, jobID)
	if !found || !activeJobStatuses[currentJobStatus(record.Data)] {
		return
	}
	if !jobs.MarkFailedIfActive(ctx, jobID, reason) {
		slog.Warn("runtime reaper: failed to mark job failed", "job_id", jobID)
	}
}

// runtimeLimitExpired reports whether createdAt + runtime-limit-seconds is in the
// past. Pure decision logic ported from the reference reaper.
func runtimeLimitExpired(labels map[string]string, createdAt, now time.Time) bool {
	seconds, ok := runtimeLimitSeconds(labels)
	if !ok || createdAt.IsZero() {
		return false
	}
	return !createdAt.Add(time.Duration(seconds) * time.Second).After(now)
}

func runtimeLimitSeconds(labels map[string]string) (int64, bool) {
	raw := labels[cluster.RuntimeLimitSecondsKey]
	if raw == "" {
		return 0, false
	}
	seconds, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || seconds <= 0 {
		return 0, false
	}
	return seconds, true
}

func isPlatformManaged(labels map[string]string) bool {
	return labels[cluster.LabelJobID] != "" || labels[cluster.LabelProjectID] != ""
}

// registerRuntimeReaper wires the workload runtime reaper as a lease-gated
// maintenance task. It runs only once StartMaintenance is called.
func registerRuntimeReaper(app *platform.App) {
	app.RegisterMaintenanceTaskForService(serviceName, "workload-runtime-reaper", func(ctx context.Context) error {
		return reapExpiredRuntimeWorkloads(ctx, app.Cluster, app.Store, time.Now().UTC())
	})
}
