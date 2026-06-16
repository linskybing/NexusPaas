package workload

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/platform/cluster"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
)

func TestReconcileNativeWorkloadStatusesUpdatesLiveRecords(t *testing.T) {
	now := time.Date(2026, 6, 14, 17, 10, 0, 0, time.UTC)
	started := metav1.NewTime(now.Add(-5 * time.Minute))
	completed := metav1.NewTime(now.Add(-time.Minute))
	cl := cluster.New(fake.NewSimpleClientset(
		reconcilerBatchJob("proj-p1", "complete", "cluster-complete", batchv1.JobStatus{
			StartTime:      &started,
			CompletionTime: &completed,
			Conditions: []batchv1.JobCondition{{
				Type: batchv1.JobComplete, Status: corev1.ConditionTrue, Reason: "Complete",
			}},
		}),
		reconcilerBatchJob("proj-p1", "failed", "cluster-failed", batchv1.JobStatus{
			Conditions: []batchv1.JobCondition{{
				Type: batchv1.JobFailed, Status: corev1.ConditionTrue, Reason: "BackoffLimitExceeded", Message: "pod failed",
			}},
		}),
		reconcilerPod("proj-p1", "pending", "cluster-queued", corev1.PodPending, nil),
		reconcilerPod("proj-p1", "running", "cluster-running", corev1.PodRunning, &started),
	), "proj")
	app := platform.NewApp(platform.Config{ServiceName: serviceName}, platform.WithCluster(cl))
	ctx := context.Background()
	createWorkloadRecord(t, app, jobsResource, map[string]any{
		"id": "store-complete", "job_id": "cluster-complete", "status": jobStatusRunning, "namespace": "proj-p1", "error_message": "old error",
	})
	createWorkloadRecord(t, app, jobsResource, map[string]any{
		"id": "store-failed", "job_id": "cluster-failed", "status": jobStatusRunning, "namespace": "proj-p1",
	})
	createWorkloadRecord(t, app, jobsResource, map[string]any{
		"id": "store-queued", "job_id": "cluster-queued", "status": jobStatusRunning, "namespace": "proj-p1",
	})
	createWorkloadRecord(t, app, jobsResource, map[string]any{
		"id": "store-running", "job_id": "cluster-running", "status": jobStatusQueued, "namespace": "proj-p1",
	})

	if err := reconcileNativeWorkloadStatuses(ctx, app.Cluster, app.Store, now); err != nil {
		t.Fatal(err)
	}

	complete, _ := app.Store.Get(ctx, jobsResource, "store-complete")
	if complete.Data["status"] != jobStatusCompleted || complete.Data["completed_at"] == nil || complete.Data["error_message"] != "" {
		t.Fatalf("complete record = %#v, want completed with timestamp and cleared error", complete.Data)
	}
	failed, _ := app.Store.Get(ctx, jobsResource, "store-failed")
	if failed.Data["status"] != jobStatusFailed || !strings.Contains(failed.Data["error_message"].(string), "BackoffLimitExceeded") {
		t.Fatalf("failed record = %#v, want failed with Kubernetes reason", failed.Data)
	}
	queued, _ := app.Store.Get(ctx, jobsResource, "store-queued")
	if queued.Data["status"] != jobStatusQueued || queued.Data["completed_at"] != nil {
		t.Fatalf("queued record = %#v, want queued with no completion time", queued.Data)
	}
	running, _ := app.Store.Get(ctx, jobsResource, "store-running")
	if running.Data["status"] != jobStatusRunning || running.Data["started_at"] == nil {
		t.Fatalf("running record = %#v, want running with start time", running.Data)
	}
}

func TestReconcileNativeWorkloadStatusesSkipsTerminalAndMissingRecords(t *testing.T) {
	now := time.Date(2026, 6, 14, 17, 10, 0, 0, time.UTC)
	cl := cluster.New(fake.NewSimpleClientset(
		reconcilerPod("proj-p1", "failed", "cluster-cancelled", corev1.PodFailed, nil),
	), "proj")
	app := platform.NewApp(platform.Config{ServiceName: serviceName}, platform.WithCluster(cl))
	ctx := context.Background()
	createWorkloadRecord(t, app, jobsResource, map[string]any{
		"id": "store-cancelled", "job_id": "cluster-cancelled", "status": "cancelled", "namespace": "proj-p1",
	})
	createWorkloadRecord(t, app, jobsResource, map[string]any{
		"id": "store-missing", "job_id": "cluster-missing", "status": jobStatusRunning, "namespace": "proj-p1",
	})

	if err := reconcileNativeWorkloadStatuses(ctx, app.Cluster, app.Store, now); err != nil {
		t.Fatal(err)
	}

	cancelled, _ := app.Store.Get(ctx, jobsResource, "store-cancelled")
	if cancelled.Data["status"] != "cancelled" || cancelled.Data["completed_at"] != nil {
		t.Fatalf("cancelled record = %#v, want terminal state untouched", cancelled.Data)
	}
	missing, _ := app.Store.Get(ctx, jobsResource, "store-missing")
	if missing.Data["status"] != jobStatusRunning || missing.Data["status_reason"] != nil {
		t.Fatalf("missing record = %#v, want no update without cluster resources", missing.Data)
	}
}

func TestStatusReconcilerRegistrationRunsMaintenanceTask(t *testing.T) {
	now := metav1.NewTime(time.Now().UTC())
	cl := cluster.New(fake.NewSimpleClientset(
		reconcilerBatchJob("proj-p1", "complete", "j-complete", batchv1.JobStatus{
			CompletionTime: &now,
			Conditions: []batchv1.JobCondition{{
				Type: batchv1.JobComplete, Status: corev1.ConditionTrue,
			}},
		}),
	), "proj")
	app := platform.NewApp(platform.Config{ServiceName: serviceName}, platform.WithCluster(cl))
	ctx := context.Background()
	createWorkloadRecord(t, app, jobsResource, map[string]any{
		"id": "j-complete", "status": jobStatusRunning, "namespace": "proj-p1",
	})

	Register(app)
	app.RunMaintenanceOnce(ctx, time.Minute)

	record, _ := app.Store.Get(ctx, jobsResource, "j-complete")
	if record.Data["status"] != jobStatusCompleted {
		t.Fatalf("registered reconciler record = %#v, want completed", record.Data)
	}
}

func reconcilerBatchJob(namespace, name, jobID string, status batchv1.JobStatus) runtime.Object {
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name, Labels: map[string]string{cluster.LabelJobID: jobID}},
		Status:     status,
	}
}

func reconcilerPod(namespace, name, jobID string, phase corev1.PodPhase, startedAt *metav1.Time) runtime.Object {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name, Labels: map[string]string{cluster.LabelJobID: jobID}},
		Status:     corev1.PodStatus{Phase: phase, StartTime: startedAt},
	}
}
