package platform

import (
	"context"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
)

func TestProjectionReconcilerRepairsInjectedDrift(t *testing.T) {
	app := NewApp(Config{ServiceName: "all", HTTPAddr: ":0"})
	ctx := context.Background()
	const (
		source   = "owner-service:rows"
		local    = "reader-service:rows_rm"
		consumer = "reader-service:rows_projection"
	)
	sync := func(ctx context.Context) {
		app.RunProjection(ctx, consumer, func(event contracts.Event) error {
			if event.Name != "RowCreated" {
				return nil
			}
			id, _ := event.Data["id"].(string)
			if _, ok := app.Store.Update(ctx, local, id, event.Data); !ok {
				_, err := app.Store.Create(ctx, local, event.Data)
				return err
			}
			return nil
		})
	}
	app.RegisterProjectionReconciler(ProjectionReconcilerSpec{
		Owner:     "reader-service",
		Consumers: []string{consumer},
		Drift: func(ctx context.Context) (int, error) {
			drift := 0
			for _, row := range app.Store.List(ctx, source) {
				if _, ok := app.Store.Get(ctx, local, row.ID); !ok {
					drift++
				}
			}
			return drift, nil
		},
		Sync: sync,
	})

	if _, err := app.Store.Create(ctx, source, map[string]any{"id": "r1", "name": "row-1"}); err != nil {
		t.Fatalf("seed source: %v", err)
	}
	if err := app.Events.Publish(ctx, contracts.Event{
		EventID: "evt-r1", Name: "RowCreated", Source: "owner-service",
		OccurredAt: time.Now().UTC(), TraceID: "trace-r1", SchemaVersion: 1,
		Data: map[string]any{"id": "r1", "name": "row-1"},
	}); err != nil {
		t.Fatalf("publish: %v", err)
	}

	// tick 1: catch-up sync builds the read model; no drift events published
	app.RunMaintenanceOnce(ctx, time.Minute)
	if _, ok := app.Store.Get(ctx, local, "r1"); !ok {
		t.Fatal("catch-up sync did not build the read model")
	}
	for _, event := range app.Events.Outbox() {
		if event.Name == "ProjectionDriftDetected" {
			t.Fatalf("drift event published on a clean tick: %#v", event)
		}
	}

	// inject drift: the read-model row disappears while the consumer has
	// already marked the event applied — plain catch-up cannot repair this
	app.Store.Delete(ctx, local, "r1")
	app.RunMaintenanceOnce(ctx, time.Minute)

	if _, ok := app.Store.Get(ctx, local, "r1"); !ok {
		t.Fatal("reconciler did not rebuild the read model after injected drift")
	}
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
