package schedulerquota

import (
	"context"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/platform/cluster"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
)

func ns(name string) *corev1.Namespace {
	return &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}}
}

func seedPlanAndProject(t *testing.T, app *platform.App, projectID, planID string, project, plan map[string]any) {
	t.Helper()
	ctx := context.Background()
	plan["id"] = planID
	if _, err := app.Store.Create(ctx, plansResource, plan); err != nil {
		t.Fatal(err)
	}
	project["id"] = projectID
	project["plan_id"] = planID
	if _, err := app.Store.Create(ctx, projectsResource, project); err != nil {
		t.Fatal(err)
	}
}

func TestReconcileResourceQuotasEnforcesBoundPlan(t *testing.T) {
	objs := []runtime.Object{ns("proj-p1-alice"), ns("proj-p1-bob"), ns("other-ns")}
	cl := cluster.New(fake.NewSimpleClientset(objs...), "proj")
	app := platform.NewApp(platform.Config{ServiceName: serviceName}, platform.WithCluster(cl))
	seedPlanAndProject(t, app, "p1", "PL1",
		map[string]any{"max_concurrent_jobs_per_user": float64(5)},
		map[string]any{"cpu_limit_cores": float64(4), "memory_limit_gb": float64(8), "gpu_limit": float64(2)})

	if err := reconcileResourceQuotas(context.Background(), app.Cluster, app.Store, time.Now().UTC()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	ctx := context.Background()
	for _, namespace := range []string{"proj-p1-alice", "proj-p1-bob"} {
		rq, err := cl.Clientset().CoreV1().ResourceQuotas(namespace).Get(ctx, namespace+"-quota", metav1.GetOptions{})
		if err != nil {
			t.Fatalf("expected quota in %s: %v", namespace, err)
		}
		if got := rq.Spec.Hard[corev1.ResourceCPU]; got.Cmp(resource.MustParse("4")) != 0 {
			t.Errorf("%s cpu = %v, want 4", namespace, got.String())
		}
		if got := rq.Spec.Hard[corev1.ResourceMemory]; got.Cmp(resource.MustParse("8Gi")) != 0 {
			t.Errorf("%s memory = %v, want 8Gi", namespace, got.String())
		}
		// pods = max_concurrent_jobs_per_user(5) * 2 = 10.
		if got := rq.Spec.Hard[corev1.ResourcePods]; got.Cmp(resource.MustParse("10")) != 0 {
			t.Errorf("%s pods = %v, want 10", namespace, got.String())
		}
	}

	// Namespaces outside the project prefix are never touched.
	if _, err := cl.Clientset().CoreV1().ResourceQuotas("other-ns").Get(ctx, "other-ns-quota", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("unexpected quota in other-ns: err=%v", err)
	}
}

func TestReconcileResourceQuotasSkipsUnboundProject(t *testing.T) {
	cl := cluster.New(fake.NewSimpleClientset(ns("proj-p2-alice")), "proj")
	app := platform.NewApp(platform.Config{ServiceName: serviceName}, platform.WithCluster(cl))
	ctx := context.Background()
	// Project with no plan_id: must not get a quota.
	if _, err := app.Store.Create(ctx, projectsResource, map[string]any{"id": "p2"}); err != nil {
		t.Fatal(err)
	}

	if err := reconcileResourceQuotas(ctx, app.Cluster, app.Store, time.Now().UTC()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if _, err := cl.Clientset().CoreV1().ResourceQuotas("proj-p2-alice").Get(ctx, "proj-p2-alice-quota", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("unbound project should not get a quota: err=%v", err)
	}
}

func TestReconcileResourceQuotasDegradedNoop(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: serviceName})
	if err := reconcileResourceQuotas(context.Background(), app.Cluster, app.Store, time.Now()); err != nil {
		t.Fatalf("degraded reconcile should not error: %v", err)
	}
}

func TestQuotaPodLimit(t *testing.T) {
	cases := []struct {
		name    string
		project map[string]any
		want    int
	}{
		{"default-when-unset", map[string]any{}, defaultQuotaPods},
		{"double-concurrent", map[string]any{"max_concurrent_jobs_per_user": float64(8)}, 16},
		{"default-when-zero", map[string]any{"max_concurrent_jobs_per_user": float64(0)}, defaultQuotaPods},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := quotaPodLimit(tc.project); got != tc.want {
				t.Fatalf("quotaPodLimit = %d, want %d", got, tc.want)
			}
		})
	}
}
