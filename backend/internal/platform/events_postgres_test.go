package platform

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/linskybing/nexuspaas/backend/internal/contracts"
)

func TestPostgresEventBusPublishAndOutbox(t *testing.T) {
	now := time.Date(2026, 6, 20, 15, 0, 0, 0, time.UTC)
	db := &fakePostgresDB{
		execTags: []pgconn.CommandTag{pgconn.NewCommandTag("INSERT 0 1")},
		queryResults: []*fakePostgresRows{{rows: [][]any{
			{"e1", "UserCreated", "identity-service", "trace-1", 1, "idem-1", []byte(`{"user_id":"US1"}`), now},
		}}},
	}
	bus := newPostgresEventBusWithDB(db)

	event := sampleEvent("e1")
	event.OccurredAt = now
	event.IdempotencyKey = "idem-1"
	if err := bus.Publish(context.Background(), event); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	if got := strings.Join(db.queries, "\n"); !strings.Contains(got, "INSERT INTO platform_event_outbox") {
		t.Fatalf("publish query = %s, want platform_event_outbox insert", got)
	}

	outbox := bus.Outbox()
	if len(outbox) != 1 || outbox[0].EventID != "e1" || outbox[0].Data["user_id"] != "US1" {
		t.Fatalf("Outbox() = %#v, want e1 with payload", outbox)
	}
}

func TestPostgresEventBusConsumeCheckpointLagAndReset(t *testing.T) {
	db := &fakePostgresDB{
		execTags: []pgconn.CommandTag{
			pgconn.NewCommandTag("INSERT 0 1"),
			pgconn.NewCommandTag("INSERT 0 0"),
			pgconn.NewCommandTag("INSERT 0 1"),
			pgconn.NewCommandTag("DELETE 1"),
			pgconn.NewCommandTag("DELETE 1"),
		},
		queryRows: []*fakePostgresRow{
			{values: []any{int64(2), "e2"}},
			{values: []any{int64(2), "e2"}},
			{values: []any{int64(1)}},
		},
	}
	bus := newPostgresEventBusWithDB(db)
	ctx := context.Background()

	first, err := bus.Consume(ctx, "usage", sampleEvent("e1"))
	if err != nil || !first {
		t.Fatalf("first Consume() = %v err=%v, want true", first, err)
	}
	again, err := bus.Consume(ctx, "usage", sampleEvent("e1"))
	if err != nil || again {
		t.Fatalf("duplicate Consume() = %v err=%v, want false", again, err)
	}
	bus.Checkpoint("usage")
	if lag := bus.Lag("usage"); lag != 1 {
		t.Fatalf("Lag() = %d, want 1", lag)
	}
	bus.ResetConsumer("usage")
	queries := strings.Join(db.queries, "\n")
	for _, want := range []string{"platform_event_inbox", "platform_event_checkpoints"} {
		if !strings.Contains(queries, want) {
			t.Fatalf("queries = %s, want %s", queries, want)
		}
	}
}

func TestPostgresEventBusResetConsumerEventsDoesNotClearCheckpoint(t *testing.T) {
	db := &fakePostgresDB{
		execTags: []pgconn.CommandTag{pgconn.NewCommandTag("DELETE 1")},
	}
	bus := newPostgresEventBusWithDB(db)

	bus.ResetConsumerEvents("usage", []string{"e1", "e2"})

	queries := strings.Join(db.queries, "\n")
	if !strings.Contains(queries, "platform_event_inbox") || !strings.Contains(queries, "event_id = ANY") {
		t.Fatalf("queries = %s, want targeted inbox delete", queries)
	}
	if strings.Contains(queries, "platform_event_checkpoints") {
		t.Fatalf("queries = %s, targeted reset must not clear checkpoints", queries)
	}
	if len(db.queryArgs) != 1 || len(db.queryArgs[0]) != 2 || db.queryArgs[0][0] != "usage" {
		t.Fatalf("query args = %#v, want consumer and event IDs", db.queryArgs)
	}
	eventIDs, ok := db.queryArgs[0][1].([]string)
	if !ok || len(eventIDs) != 2 || eventIDs[0] != "e1" || eventIDs[1] != "e2" {
		t.Fatalf("event id arg = %#v, want [e1 e2]", db.queryArgs[0][1])
	}
}

func TestPostgresEventRelayPublishesAndDeadLettersFailures(t *testing.T) {
	now := time.Date(2026, 6, 20, 15, 30, 0, 0, time.UTC)
	db := &fakePostgresDB{
		queryResults: []*fakePostgresRows{{rows: [][]any{
			{"e1", "UserCreated", "identity-service", "trace-1", 1, "", []byte(`{"user_id":"US1"}`), now, 0},
			{"e2", "UserCreated", "identity-service", "trace-1", 1, "", []byte(`{"user_id":"US2"}`), now, defaultRelayMaxAttempts - 1},
		}}},
		execTags: []pgconn.CommandTag{
			pgconn.NewCommandTag("UPDATE 1"),
			pgconn.NewCommandTag("UPDATE 1"),
		},
	}
	sink := &recordingEventPublisher{failures: map[string]error{"e2": errors.New("redis unavailable")}}
	bus := newPostgresEventBusWithDB(db).WithRelaySink(sink)

	result, err := bus.RelayPending(context.Background(), 10)
	if err == nil {
		t.Fatal("RelayPending() error = nil, want publish failure after marking dead-letter")
	}
	if result.Selected != 2 || result.Published != 1 || result.DeadLetter != 1 || result.Retried != 0 {
		t.Fatalf("RelayPending() result = %#v, want selected=2 published=1 dead_letter=1", result)
	}
	if len(sink.events) != 2 || sink.events[0].EventID != "e1" || sink.events[1].EventID != "e2" {
		t.Fatalf("sink events = %#v, want e1,e2", sink.events)
	}
	if got := strings.Join(db.queries, "\n"); !strings.Contains(got, "relay_status IN") || !strings.Contains(got, "published_at = now()") {
		t.Fatalf("relay queries = %s, want pending select and publish update", got)
	}
	if got := db.queryArgs[len(db.queryArgs)-1]; len(got) < 2 || got[1] != outboxRelayStatusDeadLetter {
		t.Fatalf("dead-letter update args = %#v, want status %s", got, outboxRelayStatusDeadLetter)
	}
}

type recordingEventPublisher struct {
	events   []contracts.Event
	failures map[string]error
}

func (p *recordingEventPublisher) Publish(_ context.Context, event contracts.Event) error {
	p.events = append(p.events, event)
	return p.failures[event.EventID]
}

func (p *recordingEventPublisher) Consume(context.Context, string, contracts.Event) (bool, error) {
	return true, nil
}

func (p *recordingEventPublisher) Outbox() []contracts.Event {
	return append([]contracts.Event(nil), p.events...)
}
