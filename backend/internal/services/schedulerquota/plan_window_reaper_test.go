package schedulerquota

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/platform/cluster"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
)

// fakeEvictor records the jobs the reaper asks workload to mark evicted.
type fakeEvictor struct {
	calls map[string]string // jobID -> reason
	err   error
}

func newFakeEvictor() *fakeEvictor { return &fakeEvictor{calls: map[string]string{}} }

func (f *fakeEvictor) Evict(_ context.Context, jobID string, req workloadEvictRequest) error {
	if f.err != nil {
		return f.err
	}
	f.calls[jobID] = req.Reason
	return nil
}

func jobPod(namespace, name, jobID string) *corev1.Pod {
	return &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Namespace: namespace,
		Name:      name,
		Labels:    map[string]string{cluster.LabelJobID: jobID},
	}}
}

func podExists(t *testing.T, cl *cluster.Client, namespace, name string) bool {
	t.Helper()
	_, err := cl.Clientset().CoreV1().Pods(namespace).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		t.Fatalf("get pod %s/%s: %v", namespace, name, err)
	}
	return err == nil
}

// reaperFixture builds a scheduler-quota app with a fake cluster seeded with one
// namespace ("proj-p1-alice") and one job pod for project p1, plus a fake evictor.
func reaperFixture(t *testing.T, project, plan map[string]any) (*platform.App, *cluster.Client, *fakeEvictor) {
	t.Helper()
	cl := cluster.New(fake.NewSimpleClientset(
		ns("proj-p1-alice"),
		runtime.Object(jobPod("proj-p1-alice", "pod-1", "job-1")),
	), "proj")
	app := platform.NewApp(platform.Config{ServiceName: serviceName}, platform.WithCluster(cl))
	ctx := context.Background()
	if plan != nil {
		plan["id"] = "PL1"
		if _, err := app.Store.Create(ctx, plansResource, plan); err != nil {
			t.Fatal(err)
		}
	}
	project["id"] = "p1"
	if _, err := app.Store.Create(ctx, projectsResource, project); err != nil {
		t.Fatal(err)
	}
	return app, cl, newFakeEvictor()
}

func TestReapExpiredPlanWindows(t *testing.T) {
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	active := map[string]any{
		"valid_from":  now.Add(-time.Hour).Format(time.RFC3339),
		"valid_until": now.Add(time.Hour).Format(time.RFC3339),
	}

	cases := []struct {
		name       string
		project    map[string]any
		plan       map[string]any
		wantEvict  bool
		wantReason string
	}{
		{name: "active plan not evicted", project: map[string]any{"plan_id": "PL1"}, plan: active, wantEvict: false},
		{name: "no plan id evicted", project: map[string]any{}, plan: nil, wantEvict: true, wantReason: reasonNoActivePlan},
		{name: "plan not found evicted", project: map[string]any{"plan_id": "missing"}, plan: active, wantEvict: true, wantReason: reasonNoActivePlan},
		{
			name:    "expired validity evicted",
			project: map[string]any{"plan_id": "PL1"},
			plan: map[string]any{
				"valid_from":  now.Add(-2 * time.Hour).Format(time.RFC3339),
				"valid_until": now.Add(-time.Hour).Format(time.RFC3339),
			},
			wantEvict: true, wantReason: reasonValidityEnded,
		},
		{
			name:    "closed week window evicted",
			project: map[string]any{"plan_id": "PL1"},
			// A 1-second window far from now's week-second is always closed.
			plan:      map[string]any{"week_windows": `[{"start":0,"end":1}]`},
			wantEvict: true, wantReason: reasonWindowClosed,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app, cl, evictor := reaperFixture(t, tc.project, tc.plan)
			if err := reapExpiredPlanWindows(context.Background(), cl, app.Store, evictor.Evict, now, true); err != nil {
				t.Fatalf("reap: %v", err)
			}
			gotEvict := !podExists(t, cl, "proj-p1-alice", "pod-1")
			if gotEvict != tc.wantEvict {
				t.Fatalf("pod evicted = %v, want %v", gotEvict, tc.wantEvict)
			}
			if tc.wantEvict {
				if reason := evictor.calls["job-1"]; reason != tc.wantReason {
					t.Fatalf("evict reason = %q, want %q", reason, tc.wantReason)
				}
			} else if len(evictor.calls) != 0 {
				t.Fatalf("active plan should not evict: %#v", evictor.calls)
			}
		})
	}
}

func TestReapExpiredPlanWindowsRespectsGracePeriod(t *testing.T) {
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	// Build a week window that closed 60s ago: it is closed at now but still open at
	// now-grace, so the grace period must keep the job running (no flapping).
	nowSecond := admissionWeekSecond(now)
	plan := map[string]any{"week_windows": []map[string]any{
		{"start": float64(nowSecond - 7200), "end": float64(nowSecond - 60)},
	}}
	app, cl, evictor := reaperFixture(t, map[string]any{"plan_id": "PL1"}, plan)

	if err := reapExpiredPlanWindows(context.Background(), cl, app.Store, evictor.Evict, now, true); err != nil {
		t.Fatalf("reap: %v", err)
	}
	if !podExists(t, cl, "proj-p1-alice", "pod-1") {
		t.Fatal("job within grace period should not be evicted")
	}
	if len(evictor.calls) != 0 {
		t.Fatalf("grace period should suppress eviction: %#v", evictor.calls)
	}
}

// TestReapExpiredPlanWindowsEvictFailureDefersPodCleanup proves the half-evict fix
// (review Finding 1): when the workload eviction contract fails, pods are left
// running so the job record is never orphaned (pods gone, record still active). The
// next cycle retries.
func TestReapExpiredPlanWindowsEvictFailureDefersPodCleanup(t *testing.T) {
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	app, cl, evictor := reaperFixture(t, map[string]any{}, nil) // no plan -> evict
	evictor.err = errors.New("workload eviction contract unreachable")

	if err := reapExpiredPlanWindows(context.Background(), cl, app.Store, evictor.Evict, now, true); err != nil {
		t.Fatalf("reap: %v", err)
	}
	if !podExists(t, cl, "proj-p1-alice", "pod-1") {
		t.Fatal("eviction-contract failure must defer pod cleanup, not delete the pod")
	}
	if _, attempted := evictor.calls["job-1"]; attempted {
		t.Fatalf("failed evictor must not record a successful call: %#v", evictor.calls)
	}
}

func TestReapExpiredPlanWindowsKillSwitchOff(t *testing.T) {
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	app, cl, evictor := reaperFixture(t, map[string]any{}, nil)

	if err := reapExpiredPlanWindows(context.Background(), cl, app.Store, evictor.Evict, now, false); err != nil {
		t.Fatalf("reap: %v", err)
	}
	if !podExists(t, cl, "proj-p1-alice", "pod-1") {
		t.Fatal("kill-switch off must leave pods running")
	}
	if len(evictor.calls) != 0 {
		t.Fatalf("kill-switch off must not mark jobs evicted: %#v", evictor.calls)
	}
}

func TestReapExpiredPlanWindowsDegradedNoop(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: serviceName})
	evictor := newFakeEvictor()
	if err := reapExpiredPlanWindows(context.Background(), app.Cluster, app.Store, evictor.Evict, time.Now(), true); err != nil {
		t.Fatalf("degraded reap should not error: %v", err)
	}
	if len(evictor.calls) != 0 {
		t.Fatalf("degraded reap should not evict: %#v", evictor.calls)
	}
}

func TestReapExpiredPlanWindowsNilEvictorNoop(t *testing.T) {
	now := time.Now().UTC()
	app, cl, _ := reaperFixture(t, map[string]any{}, nil)
	if err := reapExpiredPlanWindows(context.Background(), cl, app.Store, nil, now, true); err != nil {
		t.Fatalf("nil evictor should no-op: %v", err)
	}
	if !podExists(t, cl, "proj-p1-alice", "pod-1") {
		t.Fatal("nil evictor must not evict pods")
	}
}
