package workload

import (
	"context"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/platform/cluster"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
)

func TestIdleReaperCleansIdleInteractiveJobPodAndMarksJobFailed(t *testing.T) {
	now := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
	expired := idlePod("proj-p1-alice", "idle-ide", "interactive-ide", now.Add(-3*time.Hour), map[string]string{
		cluster.LabelJobID: "j1", cluster.LabelProjectID: "p1", cluster.LabelUserID: "u1",
	})
	fresh := idlePod("proj-p1-alice", "fresh-ide", "interactive-ide", now.Add(-5*time.Minute), map[string]string{
		cluster.LabelJobID: "j2", cluster.LabelProjectID: "p1", cluster.LabelUserID: "u1",
	})
	batch := idlePod("proj-p1-alice", "batch-job", "batch", now.Add(-3*time.Hour), map[string]string{
		cluster.LabelJobID: "j3", cluster.LabelProjectID: "p1",
	})
	cl := cluster.New(fake.NewSimpleClientset(expired, fresh, batch), "proj")
	app := platform.NewApp(platform.Config{ServiceName: serviceName}, platform.WithCluster(cl))
	ctx := context.Background()
	if _, err := app.Store.Create(ctx, jobsResource, map[string]any{"id": "j1", "status": "running"}); err != nil {
		t.Fatal(err)
	}

	if err := reapIdleInteractiveWorkloads(ctx, app.Cluster, app.Store, now, time.Hour, true); err != nil {
		t.Fatal(err)
	}

	remaining, _ := cl.ListPodsByLabel(ctx, "proj-p1-alice", "")
	names := podNames(remaining)
	if strings.Join(names, ",") != "batch-job,fresh-ide" {
		t.Fatalf("remaining pods = %v, want batch-job and fresh-ide", names)
	}
	rec, _ := app.Store.Get(ctx, jobsResource, "j1")
	if rec.Data["status"] != "failed" || !strings.Contains(rec.Data["status_reason"].(string), "idle timeout") {
		t.Fatalf("job after idle reap = %#v, want failed idle-timeout status", rec.Data)
	}
}

func TestIdleReaperDeletesIdleInteractivePodWithoutJob(t *testing.T) {
	now := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
	pod := idlePod("proj-p1-alice", "idle-terminal", "interactive-terminal", now.Add(-90*time.Minute), map[string]string{
		cluster.LabelProjectID: "p1", cluster.LabelUserID: "u1",
	})
	cl := cluster.New(fake.NewSimpleClientset(pod), "proj")
	app := platform.NewApp(platform.Config{ServiceName: serviceName}, platform.WithCluster(cl))
	ctx := context.Background()

	if err := reapIdleInteractiveWorkloads(ctx, app.Cluster, app.Store, now, time.Hour, true); err != nil {
		t.Fatal(err)
	}
	remaining, _ := cl.ListPodsByLabel(ctx, "proj-p1-alice", "")
	if len(remaining) != 0 {
		t.Fatalf("remaining pods = %+v, want idle pod deleted", remaining)
	}
}

func TestIdleReaperHonorsDeletionDisabled(t *testing.T) {
	now := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
	pod := idlePod("proj-p1-alice", "idle-ide", "interactive-vscode", now.Add(-3*time.Hour), map[string]string{
		cluster.LabelJobID: "j1", cluster.LabelProjectID: "p1", cluster.LabelUserID: "u1",
	})
	cl := cluster.New(fake.NewSimpleClientset(pod), "proj")
	app := platform.NewApp(platform.Config{ServiceName: serviceName}, platform.WithCluster(cl))
	ctx := context.Background()
	if _, err := app.Store.Create(ctx, jobsResource, map[string]any{"id": "j1", "status": "running"}); err != nil {
		t.Fatal(err)
	}

	if err := reapIdleInteractiveWorkloads(ctx, app.Cluster, app.Store, now, time.Hour, false); err != nil {
		t.Fatal(err)
	}
	remaining, _ := cl.ListPodsByLabel(ctx, "proj-p1-alice", "")
	if len(remaining) != 1 || remaining[0].Name != "idle-ide" {
		t.Fatalf("remaining pods = %+v, want idle pod retained", remaining)
	}
	rec, _ := app.Store.Get(ctx, jobsResource, "j1")
	if rec.Data["status"] != "running" {
		t.Fatalf("job status = %v, want running when deletion disabled", rec.Data["status"])
	}
}

func TestIdleReaperFailsOnlyStaleJobsWithoutMatchingPod(t *testing.T) {
	now := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
	livePod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Namespace: "ns-native",
		Name:      "native-deploy-pod",
		Labels:    map[string]string{cluster.LabelJobID: "j-live"},
	}}
	cl := cluster.New(fake.NewSimpleClientset(livePod), "proj")
	app := platform.NewApp(platform.Config{ServiceName: serviceName}, platform.WithCluster(cl))
	ctx := context.Background()
	old := now.Add(-10 * time.Minute).Format(time.RFC3339)
	rows := []map[string]any{
		{"id": "j-missing", "status": "running", "namespace": "ns-native", "created_at": old},
		{"id": "j-live", "status": "queued", "namespace": "ns-native", "created_at": old},
		{"id": "j-fresh", "status": "running", "namespace": "ns-native", "created_at": now.Add(-time.Minute).Format(time.RFC3339)},
		{"id": "j-done", "status": "completed", "namespace": "ns-native", "created_at": old},
	}
	for _, row := range rows {
		if _, err := app.Store.Create(ctx, jobsResource, row); err != nil {
			t.Fatal(err)
		}
	}

	if err := reapIdleInteractiveWorkloads(ctx, app.Cluster, app.Store, now, time.Hour, true); err != nil {
		t.Fatal(err)
	}

	missing, _ := app.Store.Get(ctx, jobsResource, "j-missing")
	if missing.Data["status"] != "failed" || missing.Data["status_reason"] != "Resource no longer exists in cluster" {
		t.Fatalf("missing job = %#v, want failed stale marker", missing.Data)
	}
	for _, id := range []string{"j-live", "j-fresh", "j-done"} {
		rec, _ := app.Store.Get(ctx, jobsResource, id)
		if rec.Data["status"] == "failed" {
			t.Fatalf("%s unexpectedly failed: %#v", id, rec.Data)
		}
	}
}

func TestIdleReaperIsRegisteredMaintenanceTask(t *testing.T) {
	old := idlePod("proj-p1-alice", "registered-idle", "interactive-notebook", time.Now().UTC().Add(-3*time.Hour), map[string]string{
		cluster.LabelJobID: "j1", cluster.LabelProjectID: "p1", cluster.LabelUserID: "u1",
	})
	cl := cluster.New(fake.NewSimpleClientset(old), "proj")
	app := platform.NewApp(platform.Config{
		ServiceName:          serviceName,
		WorkloadIdleTimeout:  time.Hour,
		AutomatedPodDeletion: true,
	}, platform.WithCluster(cl))
	Register(app)
	ctx := context.Background()
	if _, err := app.Store.Create(ctx, jobsResource, map[string]any{"id": "j1", "status": "running"}); err != nil {
		t.Fatal(err)
	}

	app.RunMaintenanceOnce(ctx, time.Minute)

	remaining, _ := cl.ListPodsByLabel(ctx, "proj-p1-alice", "")
	if len(remaining) != 0 {
		t.Fatalf("registered idle reaper left pods = %+v, want none", remaining)
	}
}

func idlePod(namespace, name, resourceType string, lastActivity time.Time, labels map[string]string) runtime.Object {
	all := map[string]string{idleResourceTypeLabel: resourceType}
	for k, v := range labels {
		all[k] = v
	}
	return &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Namespace: namespace,
		Name:      name,
		Labels:    all,
		Annotations: map[string]string{
			lastActivityAnnotation: lastActivity.Format(time.RFC3339),
		},
	}}
}

func podNames(pods []cluster.PodInfo) []string {
	names := make([]string, 0, len(pods))
	for _, pod := range pods {
		names = append(names, pod.Name)
	}
	sort.Strings(names)
	return names
}
