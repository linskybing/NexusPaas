package cluster

import (
	"context"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
)

func TestNativeJobLifecycleMapsBatchJobTerminalStates(t *testing.T) {
	started := metav1.NewTime(time.Date(2026, 6, 14, 10, 0, 0, 0, time.UTC))
	completed := metav1.NewTime(started.Add(2 * time.Minute))
	cl := New(fake.NewSimpleClientset(
		nativeStatusJob("proj-p1", "complete", "j-complete", batchv1.JobStatus{
			StartTime:      &started,
			CompletionTime: &completed,
			Conditions: []batchv1.JobCondition{{
				Type: batchv1.JobComplete, Status: corev1.ConditionTrue, Reason: "Complete",
			}},
		}),
		nativeStatusJob("proj-p1", "failed", "j-failed", batchv1.JobStatus{
			Conditions: []batchv1.JobCondition{{
				Type: batchv1.JobFailed, Status: corev1.ConditionTrue, Reason: "BackoffLimitExceeded", Message: "pod failed",
			}},
		}),
	), "proj")

	got, err := cl.NativeJobLifecycle(context.Background(), "proj-p1", "j-complete")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Found || got.Status != JobLifecycleCompleted || got.CompletedAt == nil || !got.CompletedAt.Equal(completed.Time) {
		t.Fatalf("complete lifecycle = %#v, want completed with completion time", got)
	}

	got, err = cl.NativeJobLifecycle(context.Background(), "proj-p1", "j-failed")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Found || got.Status != JobLifecycleFailed || got.Reason != "BackoffLimitExceeded: pod failed" {
		t.Fatalf("failed lifecycle = %#v, want failed with condition reason", got)
	}
}

func TestNativeJobLifecycleMapsPodsAndDeployments(t *testing.T) {
	started := metav1.NewTime(time.Date(2026, 6, 14, 10, 0, 0, 0, time.UTC))
	failedAt := metav1.NewTime(started.Add(3 * time.Minute))
	cl := New(fake.NewSimpleClientset(
		nativeStatusPod("proj-p1", "pending", "j-pending", corev1.PodPending, nil),
		nativeStatusPod("proj-p1", "running", "j-running", corev1.PodRunning, &started),
		nativeStatusDeployment("proj-p1", "available", "j-deploy", appsv1.DeploymentStatus{AvailableReplicas: 1}),
		nativeStatusDeployment("proj-p1", "failed", "j-deploy-failed", appsv1.DeploymentStatus{
			Conditions: []appsv1.DeploymentCondition{{
				Type: appsv1.DeploymentProgressing, Status: corev1.ConditionFalse,
				Reason: "ProgressDeadlineExceeded", Message: "replica set timed out", LastUpdateTime: failedAt,
			}},
		}),
	), "proj")

	tests := []struct {
		name string
		id   string
		want string
	}{
		{name: "pod pending", id: "j-pending", want: JobLifecycleQueued},
		{name: "pod running", id: "j-running", want: JobLifecycleRunning},
		{name: "deployment available", id: "j-deploy", want: JobLifecycleRunning},
		{name: "deployment failed", id: "j-deploy-failed", want: JobLifecycleFailed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := cl.NativeJobLifecycle(context.Background(), "proj-p1", tt.id)
			if err != nil {
				t.Fatal(err)
			}
			if !got.Found || got.Status != tt.want {
				t.Fatalf("lifecycle = %#v, want %s", got, tt.want)
			}
		})
	}
}

func TestNativeJobLifecycleReportsMissingResources(t *testing.T) {
	got, err := New(fake.NewSimpleClientset(), "proj").NativeJobLifecycle(context.Background(), "proj-p1", "missing")
	if err != nil {
		t.Fatal(err)
	}
	if got.Found || got.Status != "" {
		t.Fatalf("missing lifecycle = %#v, want no status", got)
	}
}

func nativeStatusJob(namespace, name, jobID string, status batchv1.JobStatus) runtime.Object {
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name, Labels: map[string]string{LabelJobID: jobID}},
		Status:     status,
	}
}

func nativeStatusPod(namespace, name, jobID string, phase corev1.PodPhase, startedAt *metav1.Time) runtime.Object {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name, Labels: map[string]string{LabelJobID: jobID}},
		Status:     corev1.PodStatus{Phase: phase, StartTime: startedAt},
	}
}

func nativeStatusDeployment(namespace, name, jobID string, status appsv1.DeploymentStatus) runtime.Object {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name, Labels: map[string]string{LabelJobID: jobID}},
		Status:     status,
	}
}
