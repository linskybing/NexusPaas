package platform

import (
	"context"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
)

const (
	reconcileTestSource   = "owner-service:rows"
	reconcileTestLocal    = "reader-service:rows_rm"
	reconcileTestConsumer = "reader-service:rows_projection"
)

func TestProjectionReconcilerRepairsInjectedDrift(t *testing.T) {
	app := NewApp(Config{ServiceName: "all", HTTPAddr: ":0"})
	ctx := context.Background()
	registerRowsTestReconciler(app)
	seedRowWithEvent(t, ctx, app)

	// tick 1: catch-up sync builds the read model; no drift events published
	app.RunMaintenanceOnce(ctx, time.Minute)
	if _, ok := app.Store.Get(ctx, reconcileTestLocal, "r1"); !ok {
		t.Fatal("catch-up sync did not build the read model")
	}
	for _, event := range app.Events.Outbox() {
		if event.Name == "ProjectionDriftDetected" {
			t.Fatalf("drift event published on a clean tick: %#v", event)
		}
	}

	// inject drift: the read-model row disappears while the consumer has
	// already marked the event applied — plain catch-up cannot repair this
	app.Store.Delete(ctx, reconcileTestLocal, "r1")
	app.RunMaintenanceOnce(ctx, time.Minute)

	if _, ok := app.Store.Get(ctx, reconcileTestLocal, "r1"); !ok {
		t.Fatal("reconciler did not rebuild the read model after injected drift")
	}
	requireDriftLifecycleEvents(t, app)
}

func registerRowsTestReconciler(app *App) {
	app.RegisterProjectionReconciler(ProjectionReconcilerSpec{
		Owner:     "reader-service",
		Consumers: []string{reconcileTestConsumer},
		Drift:     func(ctx context.Context) (int, error) { return rowsTestDrift(ctx, app), nil },
		Sync:      func(ctx context.Context) { syncRowsTestReadModel(ctx, app) },
	})
}

func syncRowsTestReadModel(ctx context.Context, app *App) {
	app.RunProjection(ctx, reconcileTestConsumer, func(event contracts.Event) error {
		if event.Name != "RowCreated" {
			return nil
		}
		id, _ := event.Data["id"].(string)
		if _, ok := app.Store.Update(ctx, reconcileTestLocal, id, event.Data); !ok {
			_, err := app.Store.Create(ctx, reconcileTestLocal, event.Data)
			return err
		}
		return nil
	})
}

func rowsTestDrift(ctx context.Context, app *App) int {
	drift := 0
	for _, row := range app.Store.List(ctx, reconcileTestSource) {
		if _, ok := app.Store.Get(ctx, reconcileTestLocal, row.ID); !ok {
			drift++
		}
	}
	return drift
}

func seedRowWithEvent(t *testing.T, ctx context.Context, app *App) {
	t.Helper()
	if _, err := app.Store.Create(ctx, reconcileTestSource, map[string]any{"id": "r1", "name": "row-1"}); err != nil {
		t.Fatalf("seed source: %v", err)
	}
	if err := app.Events.Publish(ctx, contracts.Event{
		EventID: "evt-r1", Name: "RowCreated", Source: "owner-service",
		OccurredAt: time.Now().UTC(), TraceID: "trace-r1", SchemaVersion: 1,
		Data: map[string]any{"id": "r1", "name": "row-1"},
	}); err != nil {
		t.Fatalf("publish: %v", err)
	}
}

func requireDriftLifecycleEvents(t *testing.T, app *App) {
	t.Helper()
	var detected, rebuilt bool
	for _, event := range app.Events.Outbox() {
		switch event.Name {
		case "ProjectionDriftDetected":
			detected = true
			if drift, _ := event.Data["drift"].(int); drift != 1 {
				t.Fatalf("drift event data = %#v, want drift=1", event.Data)
			}
		case "ProjectionRebuilt":
			rebuilt = true
			if after, _ := event.Data["drift_after"].(int); after != 0 {
				t.Fatalf("rebuilt event data = %#v, want drift_after=0", event.Data)
			}
		}
	}
	if !detected || !rebuilt {
		t.Fatalf("drift lifecycle events missing: detected=%v rebuilt=%v", detected, rebuilt)
	}
}

func TestProjectionReconcilerRegistrationRequiresClosures(t *testing.T) {
	app := NewApp(Config{ServiceName: "all", HTTPAddr: ":0"})
	app.RegisterProjectionReconciler(ProjectionReconcilerSpec{Owner: "x", Consumers: []string{"c"}})
	for _, name := range app.MaintenanceTaskNames() {
		if name == "projection-reconcile:c" {
			t.Fatal("reconciler registered without drift/sync closures")
		}
	}
}

// TestProjectionReconcilerSkipsRepeatRebuildOnResidualDrift pins the
// non-convergence guard: drift the replay cannot repair is rebuilt once, then
// only reported until the drift count changes again.
func TestProjectionReconcilerSkipsRepeatRebuildOnResidualDrift(t *testing.T) {
	app := NewApp(Config{ServiceName: "all", HTTPAddr: ":0"})
	ctx := context.Background()
	rebuilds := 0
	app.RegisterProjectionReconciler(ProjectionReconcilerSpec{
		Owner:     "reader-service",
		Consumers: []string{"stuck_projection"},
		Drift:     func(context.Context) (int, error) { return 2, nil },
		Sync:      func(context.Context) { rebuilds++ },
	})

	// Each tick calls Sync once for catch-up; only a rebuild calls it twice.
	app.RunMaintenanceOnce(ctx, time.Minute)
	if rebuilds != 2 {
		t.Fatalf("first tick sync calls = %d, want 2 (catch-up + rebuild)", rebuilds)
	}
	app.RunMaintenanceOnce(ctx, time.Minute)
	if rebuilds != 3 {
		t.Fatalf("second tick sync calls = %d, want 3 (catch-up only, no repeat rebuild)", rebuilds)
	}
}
