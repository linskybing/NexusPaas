package platform

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
)

func TestEventBusOutboxIsBoundedAndLagSafe(t *testing.T) {
	bus := NewEventBus()
	bus.Checkpoint("early")

	total := defaultOutboxLimit + 5
	for i := 0; i < total; i++ {
		if err := bus.Publish(context.Background(), testEvent(i)); err != nil {
			t.Fatal(err)
		}
	}

	outbox := bus.Outbox()
	if len(outbox) != defaultOutboxLimit {
		t.Fatalf("outbox length = %d, want %d", len(outbox), defaultOutboxLimit)
	}
	if outbox[0].EventID != "event-5" {
		t.Fatalf("oldest retained event = %q, want event-5", outbox[0].EventID)
	}
	if lag := bus.Lag("early"); lag < 0 || lag > defaultOutboxLimit {
		t.Fatalf("early lag = %d, want bounded non-negative lag", lag)
	}

	bus.Checkpoint("consumer")
	for i := total; i < total+3; i++ {
		if err := bus.Publish(context.Background(), testEvent(i)); err != nil {
			t.Fatal(err)
		}
	}
	if lag := bus.Lag("consumer"); lag != 3 {
		t.Fatalf("lag after post-checkpoint publishes = %d, want 3", lag)
	}
}

func TestEventBusPublishRejectsInvalidMetadata(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*contracts.Event)
	}{
		{name: "missing name", mutate: func(event *contracts.Event) { event.Name = "" }},
		{name: "missing source", mutate: func(event *contracts.Event) { event.Source = "" }},
		{name: "missing event id", mutate: func(event *contracts.Event) { event.EventID = "" }},
		{name: "missing trace id", mutate: func(event *contracts.Event) { event.TraceID = "" }},
		{name: "missing schema version", mutate: func(event *contracts.Event) { event.SchemaVersion = 0 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			event := testEvent(1)
			tc.mutate(&event)
			if err := NewEventBus().Publish(context.Background(), event); err == nil {
				t.Fatal("Publish() error = nil, want invalid metadata error")
			}
		})
	}
}

func TestEventBusConsumeRejectsInvalidTokens(t *testing.T) {
	bus := NewEventBus()
	if _, err := bus.Consume(context.Background(), "", testEvent(1)); err == nil {
		t.Fatal("Consume() with empty consumer error = nil, want error")
	}
	event := testEvent(2)
	event.EventID = ""
	if _, err := bus.Consume(context.Background(), "consumer", event); err == nil {
		t.Fatal("Consume() with empty event id error = nil, want error")
	}
}

func TestEventBusConsumeIdempotencyByConsumer(t *testing.T) {
	bus := NewEventBus()
	event := testEvent(1)
	processed, err := bus.Consume(context.Background(), "consumer-a", event)
	if err != nil || !processed {
		t.Fatalf("first Consume() = (%v, %v), want processed nil-error", processed, err)
	}
	processed, err = bus.Consume(context.Background(), "consumer-a", event)
	if err != nil || processed {
		t.Fatalf("second Consume() = (%v, %v), want duplicate nil-error", processed, err)
	}
	processed, err = bus.Consume(context.Background(), "consumer-b", event)
	if err != nil || !processed {
		t.Fatalf("different consumer Consume() = (%v, %v), want processed nil-error", processed, err)
	}
}

func TestRedactedOutboxHidesSensitivePayloadFields(t *testing.T) {
	event := testEvent(1)
	event.Data = map[string]any{
		"username":     "alice",
		"access_token": "secret-token",
		"headers": map[string]any{
			"Authorization": "Bearer secret",
			"X-Request-ID":  "req-1",
		},
		"messages": []any{
			map[string]any{"password": "secret-password", "body": "hello"},
		},
		"metadata": map[string]string{
			"api_key": "secret-key",
			"region":  "tw",
		},
	}

	redacted := redactedOutbox([]contracts.Event{event})
	data := redacted[0].Data
	if data["access_token"] != redactedValue {
		t.Fatalf("access token was not redacted: %#v", data)
	}
	headers := data["headers"].(map[string]any)
	if headers["Authorization"] != redactedValue || headers["X-Request-ID"] != "req-1" {
		t.Fatalf("headers redaction = %#v", headers)
	}
	message := data["messages"].([]any)[0].(map[string]any)
	if message["password"] != redactedValue || message["body"] != "hello" {
		t.Fatalf("nested message redaction = %#v", message)
	}
	metadata := data["metadata"].(map[string]string)
	if metadata["api_key"] != redactedValue || metadata["region"] != "tw" {
		t.Fatalf("metadata redaction = %#v", metadata)
	}
	if event.Data["access_token"] != "secret-token" {
		t.Fatalf("redaction mutated original event data: %#v", event.Data)
	}
}

func testEvent(i int) contracts.Event {
	return contracts.Event{
		EventID:       fmt.Sprintf("event-%d", i),
		Name:          "TestEvent",
		Source:        "platform-test",
		OccurredAt:    time.Now().UTC(),
		TraceID:       "trace",
		SchemaVersion: 1,
	}
}
