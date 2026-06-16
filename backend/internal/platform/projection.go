package platform

import (
	"context"
	"log/slog"
	"sort"
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
	LastEventID   string    `json:"last_event_id,omitempty"`
	LastEventName string    `json:"last_event_name,omitempty"`
	LastEventAt   time.Time `json:"last_event_at,omitempty"`
	LastAppliedAt time.Time `json:"last_applied_at,omitempty"`
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

func (p *projectionRegistry) recordDeadLetter(consumer string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.entry(consumer).DeadLettered++
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
	for _, event := range a.Events.Outbox() {
		processed, err := a.Events.Consume(ctx, consumer, event)
		if err != nil {
			slog.Warn("projection consume failed", "consumer", consumer, "event_id", event.EventID, "error", err)
			continue
		}
		if !processed {
			continue
		}
		if err := apply(event); err != nil {
			a.deadLetterEvent(ctx, consumer, event, err)
			a.projections.recordDeadLetter(consumer)
			continue
		}
		a.projections.recordApplied(consumer, event)
	}
}

func (a *App) deadLetterEvent(ctx context.Context, consumer string, event contracts.Event, cause error) {
	if a.Store == nil {
		return
	}
	id := consumer + ":" + event.EventID
	record := map[string]any{
		"id":         id,
		"consumer":   consumer,
		"event_id":   event.EventID,
		"event_name": event.Name,
		"error":      cause.Error(),
		"failed_at":  time.Now().UTC().Format(time.RFC3339),
	}
	if _, ok := a.Store.Update(ctx, deadLetterResource, id, record); ok {
		return
	}
	if _, err := a.Store.Create(ctx, deadLetterResource, record); err != nil && !IsCreateConflict(err) {
		slog.Warn("dead-letter write failed", "consumer", consumer, "event_id", event.EventID, "error", err)
	}
}

// ProjectionStatuses returns the freshness/health snapshot of every projection
// consumer for the operational /projections endpoint.
func (a *App) ProjectionStatuses() []ProjectionStatus {
	return a.projections.snapshot()
}

// ReplayProjection resets a consumer's idempotency state so its events are
// re-applied on the next projection run (e.g. after a bug fix or to recover from
// dead-lettered events). It is the operator-facing replay primitive.
func (a *App) ReplayProjection(consumer string) {
	if a.Events != nil {
		a.Events.ResetConsumer(consumer)
	}
}
