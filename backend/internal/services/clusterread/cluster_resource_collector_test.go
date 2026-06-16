package clusterread

import (
	"context"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/platform/cluster"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
)

func node(name string, cpu, mem, gpu string) *corev1.Node {
	alloc := corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse(cpu),
		corev1.ResourceMemory: resource.MustParse(mem),
	}
	if gpu != "" {
		alloc[corev1.ResourceName("nvidia.com/gpu")] = resource.MustParse(gpu)
	}
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Status:     corev1.NodeStatus{Allocatable: alloc},
	}
}

func runningPod(name, nodeName, cpu, mem, gpu string) *corev1.Pod {
	req := corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse(cpu),
		corev1.ResourceMemory: resource.MustParse(mem),
	}
	if gpu != "" {
		req[corev1.ResourceName("nvidia.com/gpu")] = resource.MustParse(gpu)
	}
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: corev1.PodSpec{
			NodeName:   nodeName,
			Containers: []corev1.Container{{Name: "main", Resources: corev1.ResourceRequirements{Requests: req}}},
		},
		Status: corev1.PodStatus{Phase: corev1.PodRunning},
	}
}

func TestCollectClusterResourcesPopulatesReadModel(t *testing.T) {
	now := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
	objs := []runtime.Object{
		node("node-a", "4", "8Gi", "2"),
		node("node-b", "8", "16Gi", ""),
		runningPod("p1", "node-a", "500m", "1Gi", "1"),
		// Terminal pods and unscheduled pods must not count toward usage.
		func() runtime.Object {
			p := runningPod("done", "node-a", "1", "1Gi", "1")
			p.Status.Phase = corev1.PodSucceeded
			return p
		}(),
	}
	cl := cluster.New(fake.NewSimpleClientset(objs...), "proj")
	app := platform.NewApp(platform.Config{ServiceName: serviceName}, platform.WithCluster(cl))
	ctx := context.Background()

	if err := collectClusterResources(ctx, app.Cluster, app.Store, now); err != nil {
		t.Fatalf("collect: %v", err)
	}

	records := app.Store.List(ctx, clusterReadModelResource)
	if len(records) != 1 {
		t.Fatalf("read model records = %d, want 1", len(records))
	}
	summary, _ := records[0].Data["summary"].(map[string]any)
	if summary == nil {
		t.Fatalf("read model record has no summary: %#v", records[0].Data)
	}

	assertInt(t, summary, "nodeCount", 2)
	// CPU allocatable: 4000 + 8000 milli; used: 500 milli (terminal pod excluded).
	assertInt64(t, summary, "totalCpuAllocatableMilli", 12000)
	assertInt64(t, summary, "totalCpuUsedMilli", 500)
	// GPU allocatable: only node-a contributes 2; used: 1 (terminal pod excluded).
	assertInt64(t, summary, "totalGpuAllocatable", 2)
	assertInt64(t, summary, "totalGpuUsed", 1)
}

func TestCollectClusterResourcesUpsertsSingleRecord(t *testing.T) {
	now := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
	cl := cluster.New(fake.NewSimpleClientset(node("node-a", "4", "8Gi", "")), "proj")
	app := platform.NewApp(platform.Config{ServiceName: serviceName}, platform.WithCluster(cl))
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		if err := collectClusterResources(ctx, app.Cluster, app.Store, now); err != nil {
			t.Fatalf("collect %d: %v", i, err)
		}
	}
	if got := len(app.Store.List(ctx, clusterReadModelResource)); got != 1 {
		t.Fatalf("read model records = %d, want 1 (upsert, not append)", got)
	}
}

func TestCollectClusterResourcesDegradedNoop(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: serviceName})
	ctx := context.Background()
	// app.Cluster is nil in degraded mode: collector must be a silent no-op.
	if err := collectClusterResources(ctx, app.Cluster, app.Store, time.Now()); err != nil {
		t.Fatalf("degraded collect should not error: %v", err)
	}
	if got := len(app.Store.List(ctx, clusterReadModelResource)); got != 0 {
		t.Fatalf("degraded collect wrote %d records, want 0", got)
	}
}

func assertInt(t *testing.T, m map[string]any, key string, want int) {
	t.Helper()
	if got, ok := m[key].(int); !ok || got != want {
		t.Fatalf("%s = %v (%T), want %d", key, m[key], m[key], want)
	}
}

func assertInt64(t *testing.T, m map[string]any, key string, want int64) {
	t.Helper()
	if got, ok := m[key].(int64); !ok || got != want {
		t.Fatalf("%s = %v (%T), want %d", key, m[key], m[key], want)
	}
}
