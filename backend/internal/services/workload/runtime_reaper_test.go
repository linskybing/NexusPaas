package workload

import (
	"context"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/platform/cluster"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
)

func TestRuntimeLimitExpired(t *testing.T) {
	now := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
	created := now.Add(-2 * time.Hour)
	cases := []struct {
		name   string
		labels map[string]string
		want   bool
	}{
		{"expired", map[string]string{cluster.RuntimeLimitSecondsKey: "60"}, true},
		{"not-yet", map[string]string{cluster.RuntimeLimitSecondsKey: "36000"}, false},
		{"no-label", map[string]string{}, false},
		{"bad-label", map[string]string{cluster.RuntimeLimitSecondsKey: "abc"}, false},
		{"zero", map[string]string{cluster.RuntimeLimitSecondsKey: "0"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := runtimeLimitExpired(tc.labels, created, now); got != tc.want {
				t.Fatalf("runtimeLimitExpired = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestReapCleansJobLabeledResourcesAndMarksJobFailed(t *testing.T) {
	now := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
	expiredPod := podWithRuntimeLimit("proj-p1-alice", "job-pod", "60", now.Add(-time.Hour), map[string]string{
		cluster.LabelJobID: "j1", cluster.LabelProjectID: "p1",
	})
	freshPod := podWithRuntimeLimit("proj-p1-alice", "fresh-pod", "36000", now, map[string]string{
		cluster.LabelJobID: "j2", cluster.LabelProjectID: "p1",
	})
	cl := cluster.New(fake.NewSimpleClientset(expiredPod, freshPod), "proj")

	app := platform.NewApp(platform.Config{ServiceName: serviceName}, platform.WithCluster(cl))
	ctx := context.Background()
	if _, err := app.Store.Create(ctx, jobsResource, map[string]any{"id": "j1", "status": "running"}); err != nil {
		t.Fatal(err)
	}

	if err := reapExpiredRuntimeWorkloads(ctx, app.Cluster, app.Store, now); err != nil {
		t.Fatal(err)
	}

	// Expired job pod cleaned up; fresh pod retained.
	remaining, _ := cl.ListPodsByLabel(ctx, "proj-p1-alice", "")
	if len(remaining) != 1 || remaining[0].Name != "fresh-pod" {
		t.Fatalf("remaining pods = %+v, want only fresh-pod", remaining)
	}
	// Job j1 marked failed.
	rec, _ := app.Store.Get(ctx, jobsResource, "j1")
	if rec.Data["status"] != "failed" {
		t.Fatalf("job status = %v, want failed", rec.Data["status"])
	}
}

func TestReapDeletesUnmanagedExpiredResourceDirectly(t *testing.T) {
	now := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
	// project-managed but no job-id -> deleted directly, no job marking.
	pod := podWithRuntimeLimit("proj-p1-alice", "lonely", "60", now.Add(-time.Hour), map[string]string{
		cluster.LabelProjectID: "p1",
	})
	cl := cluster.New(fake.NewSimpleClientset(pod), "proj")
	app := platform.NewApp(platform.Config{ServiceName: serviceName}, platform.WithCluster(cl))
	ctx := context.Background()

	if err := reapExpiredRuntimeWorkloads(ctx, app.Cluster, app.Store, now); err != nil {
		t.Fatal(err)
	}
	remaining, _ := cl.ListPodsByLabel(ctx, "proj-p1-alice", "")
	if len(remaining) != 0 {
		t.Fatalf("expired unmanaged pod not deleted: %+v", remaining)
	}
}

func TestReapDeletesExpiredDeploymentControllerAndMarksJobFailed(t *testing.T) {
	now := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
	deployment := deploymentWithRuntimeLimit("proj-p1-alice", "train", "60", now.Add(-time.Hour), map[string]string{
		cluster.LabelJobID: "j-deploy", cluster.LabelProjectID: "p1",
	})
	pod := podWithRuntimeLimit("proj-p1-alice", "train-pod", "60", now.Add(-time.Hour), map[string]string{
		cluster.LabelJobID: "j-deploy", cluster.LabelProjectID: "p1",
	})
	cl := cluster.New(fake.NewSimpleClientset(deployment, pod), "proj")
	app := platform.NewApp(platform.Config{ServiceName: serviceName}, platform.WithCluster(cl))
	ctx := context.Background()
	if _, err := app.Store.Create(ctx, jobsResource, map[string]any{"id": "j-deploy", "status": "running"}); err != nil {
		t.Fatal(err)
	}

	if err := reapExpiredRuntimeWorkloads(ctx, app.Cluster, app.Store, now); err != nil {
		t.Fatal(err)
	}

	deployments, _ := cl.Clientset().AppsV1().Deployments("proj-p1-alice").List(ctx, metav1.ListOptions{})
	pods, _ := cl.Clientset().CoreV1().Pods("proj-p1-alice").List(ctx, metav1.ListOptions{})
	if len(deployments.Items) != 0 || len(pods.Items) != 0 {
		t.Fatalf("remaining deployments=%d pods=%d, want expired deployment workload deleted", len(deployments.Items), len(pods.Items))
	}
	rec, _ := app.Store.Get(ctx, jobsResource, "j-deploy")
	if rec.Data["status"] != "failed" {
		t.Fatalf("deployment job status = %v, want failed", rec.Data["status"])
	}
}

func TestReapDegradedWhenNoCluster(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: serviceName})
	if err := reapExpiredRuntimeWorkloads(context.Background(), app.Cluster, app.Store, time.Now()); err != nil {
		t.Fatalf("degraded reap should be a no-op, got %v", err)
	}
}

func podWithRuntimeLimit(namespace, name, limitSeconds string, created time.Time, labels map[string]string) runtime.Object {
	all := map[string]string{cluster.RuntimeLimitSecondsKey: limitSeconds}
	for k, v := range labels {
		all[k] = v
	}
	return &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Namespace:         namespace,
		Name:              name,
		Labels:            all,
		CreationTimestamp: metav1.NewTime(created),
	}}
}

func deploymentWithRuntimeLimit(namespace, name, limitSeconds string, created time.Time, labels map[string]string) runtime.Object {
	all := map[string]string{cluster.RuntimeLimitSecondsKey: limitSeconds}
	for k, v := range labels {
		all[k] = v
	}
	return &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{
		Namespace:         namespace,
		Name:              name,
		Labels:            all,
		CreationTimestamp: metav1.NewTime(created),
	}}
}
