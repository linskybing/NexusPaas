package resourcehours

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/platform/cluster"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
)

func TestResourceHoursCollectorScansPodsAndComputesSummaries(t *testing.T) {
	now := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
	runningAt := metav1.NewTime(now.Add(-30 * time.Minute))
	pod := resourceHoursPod("proj-p1-alice", "job-pod", "uid-1", corev1.PodRunning, &runningAt, nil, map[string]string{
		cluster.LabelJobID: "j1", cluster.LabelProjectID: "p1", cluster.LabelUserID: "u1",
	})
	app := resourceHoursTestApp(pod)
	ctx := context.Background()

	if err := collectResourceHours(ctx, app.Cluster, app.Store, now); err != nil {
		t.Fatal(err)
	}

	row, found := app.Store.Get(ctx, resourceName, "j1")
	if !found {
		t.Fatal("resource-hours summary was not created")
	}
	assertFloat(t, row.Data["cpu_hours"], 1)
	assertFloat(t, row.Data["gpu_hours"], 0.5)
	assertFloat(t, row.Data["memory_gb_hours"], 2)
	if row.Data["is_finalized"] != false {
		t.Fatalf("is_finalized = %v, want false for active pod", row.Data["is_finalized"])
	}
	if _, found := app.Store.Get(ctx, podRecordsResource, "j1/proj-p1-alice/job-pod/uid-1"); !found {
		t.Fatal("pod resource record was not tracked")
	}
}

func TestResourceHoursCollectorMarksMissingPodsTerminated(t *testing.T) {
	now := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
	runningAt := metav1.NewTime(now.Add(-30 * time.Minute))
	app := resourceHoursTestApp(resourceHoursPod("proj-p1-alice", "job-pod", "uid-1", corev1.PodRunning, &runningAt, nil, map[string]string{
		cluster.LabelJobID: "j1", cluster.LabelProjectID: "p1", cluster.LabelUserID: "u1",
	}))
	ctx := context.Background()
	if err := collectResourceHours(ctx, app.Cluster, app.Store, now); err != nil {
		t.Fatal(err)
	}

	app.Cluster = cluster.New(fake.NewSimpleClientset(), "proj")
	later := now.Add(10 * time.Minute)
	if err := collectResourceHours(ctx, app.Cluster, app.Store, later); err != nil {
		t.Fatal(err)
	}

	podRecord, _ := app.Store.Get(ctx, podRecordsResource, "j1/proj-p1-alice/job-pod/uid-1")
	if podRecord.Data["is_active"] != false || podRecord.Data["pod_phase"] != "Missing" {
		t.Fatalf("pod record after missing scan = %#v, want terminated missing", podRecord.Data)
	}
	row, _ := app.Store.Get(ctx, resourceName, "j1")
	assertFloat(t, row.Data["gpu_hours"], 40.0/60.0)
	if row.Data["is_finalized"] != true {
		t.Fatalf("summary finalized = %v, want true after pod disappears", row.Data["is_finalized"])
	}
}

func TestResourceHoursCollectorRegisteredMaintenanceTask(t *testing.T) {
	runningAt := metav1.NewTime(time.Now().UTC().Add(-15 * time.Minute))
	app := resourceHoursTestApp(resourceHoursPod("proj-p1-alice", "job-pod", "uid-1", corev1.PodRunning, &runningAt, nil, map[string]string{
		cluster.LabelJobID: "j1", cluster.LabelProjectID: "p1", cluster.LabelUserID: "u1",
	}))
	Register(app)

	app.RunMaintenanceOnce(context.Background(), time.Minute)

	if _, found := app.Store.Get(context.Background(), resourceName, "j1"); !found {
		t.Fatal("registered resource-hours maintenance task did not create summary")
	}
}

func resourceHoursTestApp(objects ...runtime.Object) *platform.App {
	cl := cluster.New(fake.NewSimpleClientset(objects...), "proj")
	return platform.NewApp(platform.Config{ServiceName: serviceName, HTTPAddr: ":0"}, platform.WithCluster(cl))
}

func resourceHoursPod(namespace, name, uid string, phase corev1.PodPhase, runningAt, finishedAt *metav1.Time, labels map[string]string) runtime.Object {
	state := corev1.ContainerState{}
	if finishedAt != nil {
		state.Terminated = &corev1.ContainerStateTerminated{StartedAt: *runningAt, FinishedAt: *finishedAt}
	} else if runningAt != nil {
		state.Running = &corev1.ContainerStateRunning{StartedAt: *runningAt}
	}
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
			UID:       types.UID(uid),
			Labels:    labels,
		},
		Spec: corev1.PodSpec{Containers: []corev1.Container{{
			Name: "main",
			Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{
				corev1.ResourceCPU:                    resource.MustParse("2"),
				corev1.ResourceMemory:                 resource.MustParse("4Gi"),
				corev1.ResourceName("nvidia.com/gpu"): resource.MustParse("1"),
			}},
		}}},
		Status: corev1.PodStatus{
			Phase: phase,
			ContainerStatuses: []corev1.ContainerStatus{{
				Name:  "main",
				State: state,
			}},
		},
	}
}

func assertFloat(t *testing.T, value any, want float64) {
	t.Helper()
	got, ok := value.(float64)
	if !ok {
		t.Fatalf("%v (%T) is not float64", value, value)
	}
	if math.Abs(got-want) > 0.000001 {
		t.Fatalf("value = %v, want %v", got, want)
	}
}
