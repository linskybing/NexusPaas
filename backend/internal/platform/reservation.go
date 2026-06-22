package platform

import (
	"log/slog"
	"net/http"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
)

// reservationStateMachine encodes the quota-reservation lifecycle policy:
// which transitions are legal and which domain event a state emits. Keeping the
// rules in a dedicated type isolates the state-machine policy from the HTTP glue
// in App (finding 9) and makes every transition table-testable.
type reservationStateMachine struct{}

var reservationFSM = reservationStateMachine{}

func (reservationStateMachine) eventName(state string) string {
	switch state {
	case "reserved":
		return "QuotaReserved"
	case "committed":
		return "QuotaCommitted"
	case "released":
		return "QuotaReleased"
	default:
		return ""
	}
}

func (reservationStateMachine) transitionAllowed(current, requested string) bool {
	switch current {
	case "reserved":
		return requested == "committed" || requested == "released"
	case "committed":
		return requested == "released"
	case "released":
		return false
	default:
		return false
	}
}

func (a *App) handleReservation(r *httpRequest, route RouteSpec, state string) (int, any) {
	payload, err := DecodeMapWithError(r.Request)
	if err != nil {
		return http.StatusBadRequest, map[string]any{"message": errInvalidRequestBody}
	}
	payload["state"] = state
	payload["idempotency_key"] = firstNonEmpty(r.IdempotencyKey, newID())
	eventName := reservationFSM.eventName(state)
	var buildEvent recordEventBuilder
	if eventName != "" {
		buildEvent = func(record contracts.Record[map[string]any]) contracts.Event {
			return a.newEvent(r, eventName, reservationEventData(record.ID, state, record.Data))
		}
	}
	record, err := a.createRecordWithEvent(r, route.Resource, payload, buildEvent)
	if err != nil {
		return createErrorResponse(err)
	}
	return http.StatusCreated, record
}

func (a *App) handleReservationTransition(r *httpRequest, route RouteSpec, state string) (int, any) {
	id := pathID(r.Request, route.IDParam)
	record, ok := a.Store.Get(r.Context(), "scheduler-quota-service:reservations", id)
	if !ok {
		return http.StatusNotFound, map[string]any{"id": id}
	}
	current := asString(record.Data["state"])
	if current == state {
		return http.StatusOK, record
	}
	if !reservationFSM.transitionAllowed(current, state) {
		return http.StatusConflict, map[string]any{"id": id, "state": current, "requested": state}
	}
	eventName := reservationFSM.eventName(state)
	var buildEvent recordEventBuilder
	if eventName != "" {
		buildEvent = func(record contracts.Record[map[string]any]) contracts.Event {
			return a.newEvent(r, eventName, reservationEventData(id, state, record.Data))
		}
	}
	updated, ok, err := a.updateRecordWithEvent(r, "scheduler-quota-service:reservations", id, map[string]any{"state": state}, buildEvent)
	if err != nil || !ok {
		slog.Error("reservation state update failed", "reservation_id", id, "state", state, "error", err)
		return http.StatusInternalServerError, map[string]any{"message": "reservation update failed"}
	}
	return http.StatusOK, updated
}

func reservationEventData(id, state string, data map[string]any) map[string]any {
	event := map[string]any{"reservation_id": id, "state": state}
	for _, key := range []string{"project_id", "job_id", "plan_id", "expires_at"} {
		if value, ok := data[key]; ok {
			event[key] = value
		}
	}
	if reserved, ok := data["reserved"]; ok {
		event["reserved"] = reserved
		return event
	}
	reserved := map[string]any{}
	for _, key := range []string{"gpu", "cpu_millis", "memory_mib"} {
		if value, ok := data[key]; ok {
			reserved[key] = value
		}
	}
	if len(reserved) > 0 {
		event["reserved"] = reserved
	}
	return event
}
