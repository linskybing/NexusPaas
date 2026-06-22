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
	if len(statuses) != 1 || statuses[0].Consumer != "c1" || statuses[0].Applied != 1 || statuses[0].LastEventID != "e1" || statuses[0].Lag != 0 {
		t.Fatalf("projection status = %#v, want c1 applied=1 last=e1 lag=0", statuses)
	}

	publishTestEvent(t, app, "e2", "Thing")
	statuses = app.ProjectionStatuses()
	if len(statuses) != 1 || statuses[0].Lag != 1 {
		t.Fatalf("projection status after new event = %#v, want lag=1", statuses)
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

func TestRunProjectionKeepsLagVisibleWhenConsumeFails(t *testing.T) {
	stream := &consumeErrorEventStream{events: []contracts.Event{testEvent(1)}}
	app := NewApp(Config{}, WithEventBus(stream))

	app.RunProjection(context.Background(), "broken-consumer", func(contracts.Event) error {
		t.Fatal("apply must not run when Consume fails")
		return nil
	})

	if stream.checkpointed {
		t.Fatal("projection checkpointed after consume failure; lag would be hidden")
	}
	statuses := app.ProjectionStatuses()
	if len(statuses) != 1 || statuses[0].Consumer != "broken-consumer" || statuses[0].Lag != 1 {
		t.Fatalf("projection status after consume failure = %#v, want broken-consumer lag=1", statuses)
	}
}

func TestReplayProjectionRetriesOnlyUnresolvedDeadLetters(t *testing.T) {
	app := NewApp(Config{})
	publishTestEvent(t, app, "ok", "Thing")
	publishTestEvent(t, app, "retry", "Thing")
	ctx := context.Background()

	applied := map[string]int{}
	apply := func(event contracts.Event) error {
		applied[event.EventID]++
		if event.EventID == "retry" && applied[event.EventID] == 1 {
			return errors.New("transient")
		}
		return nil
	}
	app.RunProjection(ctx, "c1", apply)
	if applied["ok"] != 1 || applied["retry"] != 1 {
		t.Fatalf("applied before replay = %#v, want ok=1 retry=1", applied)
	}
	if _, ok := app.Store.Get(ctx, deadLetterResource, "c1:retry"); !ok {
		t.Fatal("missing dead-letter record before retry replay")
	}

	app.ReplayProjection("c1")
	statuses := app.ProjectionStatuses()
	if len(statuses) != 1 || statuses[0].ReplayCount != 1 || !statuses[0].ReplayPending || statuses[0].Lag != 0 || statuses[0].LastReplayAt.IsZero() {
		t.Fatalf("status after replay request = %#v, want replay_count=1 pending lag=0", statuses)
	}

	app.RunProjection(ctx, "c1", apply)
	if applied["ok"] != 1 || applied["retry"] != 2 {
		t.Fatalf("applied after retry replay = %#v, want ok=1 retry=2", applied)
	}
	if _, ok := app.Store.Get(ctx, deadLetterResource, "c1:retry"); ok {
		t.Fatal("dead-letter record remained after successful retry")
	}
	statuses = app.ProjectionStatuses()
	if len(statuses) != 1 || statuses[0].ReplayPending || statuses[0].ReplayCount != 1 || statuses[0].Lag != 0 || statuses[0].Applied != 2 {
		t.Fatalf("status after retry completion = %#v, want replay_count=1 pending=false lag=0 applied=2", statuses)
	}

	app.ReplayProjection("c1")
	app.RunProjection(ctx, "c1", apply)
	if applied["ok"] != 1 || applied["retry"] != 2 {
		t.Fatalf("applied after second replay = %#v, want no duplicate apply", applied)
	}
}

func TestRunProjectionTracksDeadLetterRetries(t *testing.T) {
	app := NewApp(Config{})
	publishTestEvent(t, app, "bad", "Poison")
	ctx := context.Background()
	fail := func(contracts.Event) error { return errors.New("boom") }

	app.RunProjection(ctx, "c1", fail)
	record, ok := app.Store.Get(ctx, deadLetterResource, "c1:bad")
	if !ok {
		t.Fatal("missing dead-letter record after first failure")
	}
	if got := projectionMetadataInt(record.Data, "attempt_count"); got != 1 {
		t.Fatalf("first dead-letter attempt_count = %d, want 1", got)
	}
	if got := projectionMetadataInt(record.Data, "retry_count"); got != 0 {
		t.Fatalf("first dead-letter retry_count = %d, want 0", got)
	}
	statuses := app.ProjectionStatuses()
	if len(statuses) != 1 || statuses[0].DeadLettered != 1 || statuses[0].RetryCount != 0 {
		t.Fatalf("status after first failure = %#v, want dead_lettered=1 retry_count=0", statuses)
	}

	app.ReplayProjection("c1")
	statuses = app.ProjectionStatuses()
	if len(statuses) != 1 || statuses[0].ReplayCount != 1 || !statuses[0].ReplayPending || statuses[0].Lag != 0 {
		t.Fatalf("status after retry replay request = %#v, want replay_count=1 pending lag=0", statuses)
	}

	app.RunProjection(ctx, "c1", fail)
	record, ok = app.Store.Get(ctx, deadLetterResource, "c1:bad")
	if !ok {
		t.Fatal("missing dead-letter record after retry failure")
	}
	if got := projectionMetadataInt(record.Data, "attempt_count"); got != 2 {
		t.Fatalf("retried dead-letter attempt_count = %d, want 2", got)
	}
	if got := projectionMetadataInt(record.Data, "retry_count"); got != 1 {
		t.Fatalf("retried dead-letter retry_count = %d, want 1", got)
	}
	if record.Data["last_failed_at"] == "" {
		t.Fatalf("retried dead-letter missing last_failed_at: %#v", record.Data)
	}
	statuses = app.ProjectionStatuses()
	if len(statuses) != 1 || statuses[0].DeadLettered != 2 || statuses[0].RetryCount != 1 || statuses[0].ReplayPending || statuses[0].Lag != 0 {
		t.Fatalf("status after retry failure = %#v, want dead_lettered=2 retry_count=1 pending=false lag=0", statuses)
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

type consumeErrorEventStream struct {
	events       []contracts.Event
	checkpointed bool
}

func (s *consumeErrorEventStream) Publish(_ context.Context, event contracts.Event) error {
	s.events = append(s.events, event)
	return nil
}

func (s *consumeErrorEventStream) Consume(context.Context, string, contracts.Event) (bool, error) {
	return false, errors.New("consume failed")
}

func (s *consumeErrorEventStream) Outbox() []contracts.Event {
	return append([]contracts.Event(nil), s.events...)
}

func (s *consumeErrorEventStream) Checkpoint(string) {
	s.checkpointed = true
}

func (s *consumeErrorEventStream) Lag(string) int {
	return len(s.events)
}

func (s *consumeErrorEventStream) ResetConsumer(string) {}

func (s *consumeErrorEventStream) ResetConsumerEvents(string, []string) {}
