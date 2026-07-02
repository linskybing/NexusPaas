package workload

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

var statusReconcilerLiveStatuses = map[string]bool{
	jobStatusSubmitted:    true,
	jobStatusWaitingInfra: true,
	jobStatusQueued:       true,
	jobStatusRunning:      true,
	"pending":             true,
	"scheduled":           true,
	"active":              true,
}

func reconcileNativeWorkloadStatuses(ctx context.Context, cl *cluster.Client, store platform.RecordStore, now time.Time) error {
	return reconcileNativeWorkloadStatusesWithReservationRelease(ctx, cl, store, nil, now)
}

func reconcileNativeWorkloadStatusesWithReservationRelease(ctx context.Context, cl *cluster.Client, store platform.RecordStore, release reservationReleaseFunc, now time.Time) error {
	jobs := jobRepositoryFromStore(store)
	if cl == nil || jobs == nil {
		return nil
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	for _, record := range jobs.ListLifecycleReconcileCandidates(ctx) {
		if err := reconcileNativeWorkloadRecord(ctx, cl, jobs, release, record, now); err != nil {
			return err
		}
	}
	return nil
}

func reconcileNativeWorkloadRecord(
	ctx context.Context,
	cl *cluster.Client,
	jobs *recordStoreWorkloadJobRepository,
	release reservationReleaseFunc,
	record contracts.Record[map[string]any],
	now time.Time,
) error {
	namespace := shared.TextValue(record.Data, "namespace", "Namespace", "pod_namespace", "podNamespace")
	jobID := shared.FirstNonEmpty(shared.TextValue(record.Data, "job_id", "jobId"), record.ID)
	if namespace == "" || jobID == "" {
		return nil
	}
	lifecycle, err := cl.NativeJobLifecycle(ctx, namespace, jobID)
	if err != nil {
		return err
	}
	if !lifecycle.Found || lifecycle.Status == "" {
		return nil
	}
	if !jobs.ApplyLifecycleObservation(ctx, record, lifecycle, now) {
		slog.Warn("status reconciler: failed to update job", "job_id", record.ID, "status", lifecycle.Status)
		return nil
	}
	if lifecycle.Status == jobStatusCompleted || lifecycle.Status == jobStatusFailed {
		releaseJobReservation(ctx, release, nil, record.Data)
	}
	return nil
}

func statusReconcileUpdate(record contracts.Record[map[string]any], lifecycle cluster.JobLifecycle, now time.Time) map[string]any {
	update := map[string]any{}
	current := currentJobStatus(record.Data)
	if current != lifecycle.Status {
		update["status"] = lifecycle.Status
	}
	if lifecycle.Reason != "" {
		update["status_reason"] = lifecycle.Reason
	}
	switch lifecycle.Status {
	case jobStatusQueued:
		update["completed_at"] = nil
	case jobStatusRunning:
		update["completed_at"] = nil
		if shared.TextValue(record.Data, "started_at", "startedAt") == "" {
			update["started_at"] = lifecycleTimeOrNow(lifecycle.StartedAt, now)
		}
	case jobStatusCompleted:
		update["completed_at"] = lifecycleTimeOrNow(lifecycle.CompletedAt, now)
		update["error_message"] = ""
	case jobStatusFailed:
		update["completed_at"] = lifecycleTimeOrNow(lifecycle.CompletedAt, now)
		update["error_message"] = shared.FirstNonEmpty(lifecycle.Reason, "Kubernetes workload failed")
	}
	return removeNoopStatusUpdate(record.Data, update)
}

func removeNoopStatusUpdate(data, update map[string]any) map[string]any {
	for key, value := range update {
		if sameStatusValue(data[key], value) {
			delete(update, key)
		}
	}
	return update
}

func sameStatusValue(current, next any) bool {
	if next == nil {
		return current == nil
	}
	return strings.TrimSpace(shared.TextValue(map[string]any{"value": current}, "value")) ==
		strings.TrimSpace(shared.TextValue(map[string]any{"value": next}, "value"))
}

func currentJobStatus(data map[string]any) string {
	return strings.ToLower(shared.TextValue(data, "status", "Status"))
}

func lifecycleTimeOrNow(value *time.Time, now time.Time) string {
	if value != nil && !value.IsZero() {
		return value.UTC().Format(time.RFC3339)
	}
	return now.UTC().Format(time.RFC3339)
}

func registerStatusReconciler(app *platform.App) {
	app.RegisterMaintenanceTaskForService(serviceName, "workload-status-reconciler", func(ctx context.Context) error {
		return reconcileNativeWorkloadStatusesWithReservationRelease(ctx, app.Cluster, app.Store, schedulerReservationReleaseFuncForApp(app), time.Now().UTC())
	})
}
