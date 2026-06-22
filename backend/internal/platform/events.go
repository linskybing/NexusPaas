package platform

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"

	"go.opentelemetry.io/otel/trace"
)

const (
	defaultOutboxLimit       = 1000
	eventPublishFailedLogMsg = "event publish failed"
)

var errConsumerEventRequired = errors.New("consumer and event_id are required")

type EventBus struct {
	mu          sync.RWMutex
	outbox      []contracts.Event
	inbox       map[string]map[string]bool
	checkpoints map[string]int
}

func NewEventBus() *EventBus {
	return &EventBus{inbox: map[string]map[string]bool{}, checkpoints: map[string]int{}}
}

func (b *EventBus) Publish(_ context.Context, event contracts.Event) error {
	if err := validateEventMetadata(event); err != nil {
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.outbox = append(b.outbox, event)
	b.trimOutboxLocked()
	return nil
}

func (b *EventBus) Consume(_ context.Context, consumer string, event contracts.Event) (bool, error) {
	if consumer == "" || event.EventID == "" {
		return false, errConsumerEventRequired
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.inbox[consumer] == nil {
		b.inbox[consumer] = map[string]bool{}
	}
	if b.inbox[consumer][event.EventID] {
		return false, nil
	}
	b.inbox[consumer][event.EventID] = true
	return true, nil
}

func (b *EventBus) ResetConsumer(consumer string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.inbox, consumer)
	delete(b.checkpoints, consumer)
}

func (b *EventBus) ResetConsumerEvents(consumer string, eventIDs []string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, eventID := range eventIDs {
		delete(b.inbox[consumer], eventID)
	}
}

func (b *EventBus) Outbox() []contracts.Event {
	b.mu.RLock()
	defer b.mu.RUnlock()
	events := make([]contracts.Event, len(b.outbox))
	copy(events, b.outbox)
	return events
}

func redactedOutbox(events []contracts.Event) []contracts.Event {
	out := make([]contracts.Event, len(events))
	for i, event := range events {
		out[i] = event
		out[i].Data = redactEventMap(event.Data)
	}
	return out
}

func redactEventMap(data map[string]any) map[string]any {
	if data == nil {
		return nil
	}
	redacted := make(map[string]any, len(data))
	for key, value := range data {
		if sensitiveEventKey(key) {
			redacted[key] = redactedValue
			continue
		}
		redacted[key] = redactEventValue(value)
	}
	return redacted
}

func redactEventValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return redactEventMap(typed)
	case map[string]string:
		redacted := make(map[string]string, len(typed))
		for key, value := range typed {
			if sensitiveEventKey(key) {
				redacted[key] = redactedValue
			} else {
				redacted[key] = value
			}
		}
		return redacted
	case []any:
		redacted := make([]any, len(typed))
		for i, item := range typed {
			redacted[i] = redactEventValue(item)
		}
		return redacted
	default:
		return value
	}
}

func sensitiveEventKey(key string) bool {
	normalized := strings.NewReplacer("-", "", "_", "", " ", "").Replace(strings.ToLower(key))
	for _, token := range sensitiveEventKeyTokens {
		if strings.Contains(normalized, token) {
			return true
		}
	}
	return false
}

const redactedValue = "[REDACTED]"

var sensitiveEventKeyTokens = []string{
	"accesstoken",
	"apikey",
	"authorization",
	"cookie",
	"credential",
	"jwt",
	"password",
	"privatekey",
	"refreshtoken",
	"secret",
	"sessiontoken",
	"tokenhash",
}

func (b *EventBus) Checkpoint(consumer string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.checkpoints[consumer] = len(b.outbox)
}

func (b *EventBus) Lag(consumer string) int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	checkpoint := b.checkpoints[consumer]
	if checkpoint > len(b.outbox) {
		return 0
	}
	return len(b.outbox) - checkpoint
}

func (b *EventBus) trimOutboxLocked() {
	if defaultOutboxLimit <= 0 || len(b.outbox) <= defaultOutboxLimit {
		return
	}
	dropped := len(b.outbox) - defaultOutboxLimit
	b.outbox = append([]contracts.Event(nil), b.outbox[dropped:]...)
	for consumer, checkpoint := range b.checkpoints {
		switch {
		case checkpoint <= dropped:
			b.checkpoints[consumer] = 0
		default:
			b.checkpoints[consumer] = checkpoint - dropped
		}
		if b.checkpoints[consumer] > len(b.outbox) {
			b.checkpoints[consumer] = len(b.outbox)
		}
	}
}

func validateEventMetadata(event contracts.Event) error {
	if event.Name == "" || event.Source == "" || event.EventID == "" || event.TraceID == "" || event.SchemaVersion == 0 {
		return errors.New("event metadata is incomplete")
	}
	return nil
}

func (a *App) publishEvent(r *httpRequest, name string, data map[string]any) {
	if err := a.Events.Publish(r.Context(), a.newEvent(r, name, data)); err != nil {
		slog.Error(eventPublishFailedLogMsg, "event", name, "service", r.Service, "trace_id", traceIDFromRequest(r), "error", err)
	}
}

func (a *App) newEvent(r *httpRequest, name string, data map[string]any) contracts.Event {
	return contracts.Event{
		EventID:        NewUUID(),
		Name:           name,
		Source:         r.Service,
		OccurredAt:     time.Now().UTC(),
		TraceID:        traceIDFromRequest(r),
		SchemaVersion:  1,
		IdempotencyKey: r.IdempotencyKey,
		Data:           data,
	}
}

func traceIDFromRequest(r *httpRequest) string {
	traceID := r.TraceID
	if sc := trace.SpanContextFromContext(r.Context()); sc.HasTraceID() {
		traceID = sc.TraceID().String()
	}
	return traceID
}
