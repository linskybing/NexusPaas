package platform

import (
	"context"
	"encoding/json"
	"log/slog"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
)

// deadLetterResource is the service-owned store resource that captures events a
// projection failed to apply, so a poison event neither blocks the stream nor is
// silently lost (finding 5).
const deadLetterResource = "platform:dead_letters"

// ProjectionStatus is the freshness/health snapshot of one projection consumer.
type ProjectionStatus struct {
	Consumer      string    `json:"consumer"`
	Applied       int64     `json:"applied"`
	DeadLettered  int64     `json:"dead_lettered"`
	RetryCount    int64     `json:"retry_count"`
	ReplayCount   int64     `json:"replay_count"`
	ReplayPending bool      `json:"replay_pending"`
	Lag           int       `json:"lag"`
	LastEventID   string    `json:"last_event_id,omitempty"`
	LastEventName string    `json:"last_event_name,omitempty"`
	LastEventAt   time.Time `json:"last_event_at,omitempty"`
	LastAppliedAt time.Time `json:"last_applied_at,omitempty"`
	LastReplayAt  time.Time `json:"last_replay_at,omitempty"`
}

type projectionRegistry struct {
	mu     sync.Mutex
	status map[string]*ProjectionStatus
}

func newProjectionRegistry() *projectionRegistry {
	return &projectionRegistry{status: map[string]*ProjectionStatus{}}
}

func (p *projectionRegistry) entry(consumer string) *ProjectionStatus {
	if p.status[consumer] == nil {
		p.status[consumer] = &ProjectionStatus{Consumer: consumer}
	}
	return p.status[consumer]
}

func (p *projectionRegistry) ensure(consumer string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.entry(consumer)
}

func (p *projectionRegistry) recordApplied(consumer string, event contracts.Event) {
	p.mu.Lock()
	defer p.mu.Unlock()
	status := p.entry(consumer)
	status.Applied++
	status.LastEventID = event.EventID
	status.LastEventName = event.Name
	status.LastEventAt = event.OccurredAt
	status.LastAppliedAt = time.Now().UTC()
}

func (p *projectionRegistry) recordDeadLetter(consumer string, retried bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	status := p.entry(consumer)
	status.DeadLettered++
	if retried {
		status.RetryCount++
	}
}

func (p *projectionRegistry) recordReplay(consumer string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	status := p.entry(consumer)
	status.ReplayCount++
	status.ReplayPending = true
	status.LastReplayAt = time.Now().UTC()
}

func (p *projectionRegistry) recordReplayComplete(consumer string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.entry(consumer).ReplayPending = false
}

func (p *projectionRegistry) snapshot() []ProjectionStatus {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]ProjectionStatus, 0, len(p.status))
	for _, status := range p.status {
		out = append(out, *status)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Consumer < out[j].Consumer })
	return out
}

// RunProjection consumes every outbox event for the given consumer with at-most-
// once idempotency, applies it, records freshness, and dead-letters any event the
// apply function rejects (so one poison event doesn't wedge the projection). It is
// the shared replacement for the hand-rolled iterate→Consume→apply loops.
func (a *App) RunProjection(ctx context.Context, consumer string, apply func(contracts.Event) error) {
	if a.Events == nil {
		return
	}
	a.projections.ensure(consumer)
	consumeFailed := false
	for _, event := range a.Events.Outbox() {
		processed, err := a.Events.Consume(ctx, consumer, event)
		if err != nil {
			slog.Warn("projection consume failed", "consumer", consumer, "event_id", event.EventID, "error", err)
			consumeFailed = true
			continue
		}
		if !processed {
			continue
		}
		if err := apply(event); err != nil {
			retried := a.deadLetterEvent(ctx, consumer, event, err)
			a.projections.recordDeadLetter(consumer, retried)
			continue
		}
		a.resolveDeadLetterEvent(ctx, consumer, event)
		a.projections.recordApplied(consumer, event)
	}
	if !consumeFailed {
		a.Events.Checkpoint(consumer)
		a.projections.recordReplayComplete(consumer)
	}
}

func (a *App) deadLetterEvent(ctx context.Context, consumer string, event contracts.Event, cause error) bool {
	if a.Store == nil {
		return false
	}
	id := consumer + ":" + event.EventID
	attemptCount := 1
	retried := false
	if existing, ok := a.Store.Get(ctx, deadLetterResource, id); ok {
		retried = true
		attemptCount = projectionMetadataInt(existing.Data, "attempt_count", "attemptCount")
		if attemptCount < 1 {
			attemptCount = 1
		}
		attemptCount++
	}
	failedAt := time.Now().UTC().Format(time.RFC3339)
	record := map[string]any{
		"id":             id,
		"consumer":       consumer,
		"event_id":       event.EventID,
		"event_name":     event.Name,
		"error":          cause.Error(),
		"failed_at":      failedAt,
		"last_failed_at": failedAt,
		"attempt_count":  attemptCount,
		"retry_count":    attemptCount - 1,
	}
	if _, ok := a.Store.Update(ctx, deadLetterResource, id, record); ok {
		return retried
	}
	if _, err := a.Store.Create(ctx, deadLetterResource, record); err != nil && !IsCreateConflict(err) {
		slog.Warn("dead-letter write failed", "consumer", consumer, "event_id", event.EventID, "error", err)
	}
	return retried
}

func (a *App) resolveDeadLetterEvent(ctx context.Context, consumer string, event contracts.Event) {
	if a.Store == nil {
		return
	}
	a.Store.Delete(ctx, deadLetterResource, consumer+":"+event.EventID)
}

func projectionMetadataInt(data map[string]any, keys ...string) int {
	for _, key := range keys {
		switch value := data[key].(type) {
		case int:
			return value
		case int64:
			return int(value)
		case int32:
			return int(value)
		case float64:
			return int(value)
		case float32:
			return int(value)
		case json.Number:
			if parsed, err := value.Int64(); err == nil {
				return int(parsed)
			}
		case string:
			if parsed, err := strconv.Atoi(value); err == nil {
				return parsed
			}
		}
	}
	return 0
}

// ProjectionStatuses returns the freshness/health snapshot of every projection
// consumer for the operational /projections endpoint.
func (a *App) ProjectionStatuses() []ProjectionStatus {
	statuses := a.projections.snapshot()
	if a.Events == nil {
		return statuses
	}
	for i := range statuses {
		statuses[i].Lag = a.Events.Lag(statuses[i].Consumer)
	}
	return statuses
}

// ReplayProjection releases only unresolved dead-lettered events for retry. It
// does not clear the whole consumer inbox, so already successful events are not
// applied again.
func (a *App) ReplayProjection(consumer string) {
	if a.Events == nil || a.Store == nil {
		return
	}
	var eventIDs []string
	for _, record := range a.Store.List(context.Background(), deadLetterResource) {
		if asString(record.Data["consumer"]) == consumer {
			if eventID := asString(record.Data["event_id"]); eventID != "" {
				eventIDs = append(eventIDs, eventID)
			}
		}
	}
	if len(eventIDs) == 0 {
		return
	}
	a.projections.recordReplay(consumer)
	a.Events.ResetConsumerEvents(consumer, eventIDs)
}
