package platform

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
)

func publishTestEvent(t *testing.T, app *App, id, name string) {
	t.Helper()
	err := app.Events.Publish(context.Background(), contracts.Event{
		EventID: id, Name: name, Source: "test", TraceID: "t", SchemaVersion: 1,
		OccurredAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
}

func TestRunProjectionAppliesOnceAndRecordsFreshness(t *testing.T) {
	app := NewApp(Config{})
	publishTestEvent(t, app, "e1", "Thing")
	ctx := context.Background()

	applied := 0
	run := func() { app.RunProjection(ctx, "c1", func(contracts.Event) error { applied++; return nil }) }
	run()
	run() // second pass is idempotent: the event is already consumed
	if applied != 1 {
		t.Fatalf("applied = %d, want 1 (idempotent)", applied)
	}

	statuses := app.ProjectionStatuses()
	if len(statuses) != 1 || statuses[0].Consumer != "c1" || statuses[0].Applied != 1 || statuses[0].LastEventID != "e1" {
		t.Fatalf("projection status = %#v, want c1 applied=1 last=e1", statuses)
	}
}

func TestRunProjectionDeadLettersFailedEvent(t *testing.T) {
	app := NewApp(Config{})
	publishTestEvent(t, app, "bad", "Poison")
	ctx := context.Background()

	app.RunProjection(ctx, "c1", func(contracts.Event) error { return errors.New("boom") })

	dlq := app.Store.List(ctx, deadLetterResource)
	if len(dlq) != 1 || asString(dlq[0].Data["event_id"]) != "bad" {
		t.Fatalf("dead letters = %#v, want one for event bad", dlq)
	}
	if statuses := app.ProjectionStatuses(); len(statuses) != 1 || statuses[0].DeadLettered != 1 || statuses[0].Applied != 0 {
		t.Fatalf("status = %#v, want dead_lettered=1 applied=0", statuses)
	}
}

func TestReplayProjectionReappliesEvents(t *testing.T) {
	app := NewApp(Config{})
	publishTestEvent(t, app, "e1", "Thing")
	ctx := context.Background()

	applied := 0
	apply := func(contracts.Event) error { applied++; return nil }
	app.RunProjection(ctx, "c1", apply)
	app.RunProjection(ctx, "c1", apply) // idempotent, still 1
	if applied != 1 {
		t.Fatalf("applied before replay = %d, want 1", applied)
	}

	app.ReplayProjection("c1")
	app.RunProjection(ctx, "c1", apply)
	if applied != 2 {
		t.Fatalf("applied after replay = %d, want 2", applied)
	}
}

func TestServiceFallbackDisabledKeepsStoreLocal(t *testing.T) {
	cfg := Config{
		ServiceName:             "ide-service",
		ServiceURLs:             map[string]string{"identity-service": "http://identity-service"},
		ServiceAPIKey:           "k",
		ServiceFallbackDisabled: true,
	}
	app := NewApp(cfg)
	if _, ok := app.Store.(*crossServiceStore); ok {
		t.Fatal("DISABLE_SERVICE_FALLBACK should not install the cross-service HTTP fallback")
	}

	// Without the switch, the fallback decorator is installed.
	cfg.ServiceFallbackDisabled = false
	app2 := NewApp(cfg)
	if _, ok := app2.Store.(*crossServiceStore); !ok {
		t.Fatal("isolated service with SERVICE_URLS should install the cross-service fallback by default")
	}
}
