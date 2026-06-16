package platform

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
)

func (a *App) handleWorkerLease(r *httpRequest) (int, any) {
	payload, err := DecodeMapWithError(r.Request)
	if err != nil {
		return http.StatusBadRequest, map[string]any{"message": errInvalidRequestBody}
	}
	worker := firstNonEmpty(asString(payload["worker"]), "worker")
	shard := firstNonEmpty(asString(payload["shard"]), "default")
	acquired, err := a.Leases.Acquire(r.Context(), worker, shard, time.Minute)
	if err != nil {
		slog.Error("worker lease acquire failed", "worker", worker, "shard", shard, "error", err)
		return http.StatusInternalServerError, map[string]any{"message": "lease acquire failed"}
	}
	return http.StatusOK, map[string]any{"worker": worker, "shard": shard, "acquired": acquired}
}

func (a *App) handleEventIngest(r *httpRequest, route RouteSpec) (int, any) {
	payload, err := DecodeMapWithError(r.Request)
	if err != nil {
		return http.StatusBadRequest, map[string]any{"message": errInvalidRequestBody}
	}
	event := contracts.Event{
		EventID:        firstNonEmpty(asString(payload["event_id"]), NewUUID()),
		Name:           firstNonEmpty(asString(payload["name"]), "AuditEvent"),
		Source:         firstNonEmpty(asString(payload["source"]), route.Resource),
		OccurredAt:     time.Now().UTC(),
		TraceID:        firstNonEmpty(asString(payload["trace_id"]), r.TraceID),
		SchemaVersion:  1,
		IdempotencyKey: firstNonEmpty(asString(payload["idempotency_key"]), r.IdempotencyKey),
		Data:           payload,
	}
	processed, err := a.Events.Consume(r.Context(), route.Resource, event)
	if err != nil {
		slog.Error("event consume failed", "event_id", event.EventID, "consumer", route.Resource, "error", err)
		return http.StatusInternalServerError, map[string]any{"message": "event consume failed"}
	}
	if processed {
		if perr := a.Events.Publish(r.Context(), event); perr != nil {
			slog.Error("event republish failed", "event_id", event.EventID, "error", perr)
		}
	}
	return http.StatusAccepted, map[string]any{"processed": processed, "event_id": event.EventID}
}
