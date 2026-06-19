package platform

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
)

func TestQuotaReservedProducerMatchesV1Fixture(t *testing.T) {
	fixture := readPlatformEventFixture(t, "quota-reserved.json")
	app := NewApp(Config{ServiceName: "all", HTTPAddr: ":0"})
	payload := clonePlatformFixturePayload(fixture.Payload)
	payload["id"] = payload["reservation_id"]
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal reservation payload: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/internal/quota/reservations", bytes.NewReader(raw))
	req.Header.Set(headerContentType, "application/json")
	httpReq := &httpRequest{
		Request:        req,
		Service:        fixture.Producer,
		TraceID:        fixture.TraceID,
		IdempotencyKey: "idem-quota-reserved-v1",
	}
	route := RouteSpec{Resource: "scheduler-quota-service:reservations", Action: "quota_reserve"}

	status, _ := app.handleReservation(httpReq, route, "reserved")
	if status != http.StatusCreated {
		t.Fatalf("handleReservation status = %d, want %d", status, http.StatusCreated)
	}

	event := requirePlatformProducerEvent(t, app, fixture.EventType)
	assertPlatformEventMetadata(t, event, fixture, "idem-quota-reserved-v1")
	assertPlatformPayloadContains(t, event.Data, fixture.Payload)
	if event.Data["state"] != "reserved" {
		t.Fatalf("payload state = %#v, want reserved", event.Data["state"])
	}
}

func TestAuditEventProducerMatchesV1Fixture(t *testing.T) {
	fixture := readPlatformEventFixture(t, "audit-event.json")
	app := NewApp(Config{ServiceName: "all", HTTPAddr: ":0"})
	payload := fixture.Payload
	req := httptest.NewRequest(http.MethodPost, payload["request_path"].(string), nil)
	req.SetPathValue("reservationId", payload["resource_id"].(string))
	req.Header.Set(headerUserID, payload["actor_user_id"].(string))
	httpReq := &httpRequest{
		Request: req,
		Service: fixture.Producer,
		TraceID: fixture.TraceID,
	}
	route := RouteSpec{
		Method:      http.MethodPost,
		Pattern:     "/api/v1/internal/quota/reservations/{reservationId}",
		OperationID: payload["action"].(string),
		Resource:    "scheduler-quota-service:quota_reservation",
		IDParam:     "reservationId",
	}

	app.publishAudit(httpReq, route, true)

	event := requirePlatformProducerEvent(t, app, fixture.EventType)
	assertPlatformEventMetadata(t, event, fixture, "")
	for _, key := range []string{"actor_user_id", "action", "resource_type", "resource_id", "outcome", "source_service", "request_path"} {
		if got, want := event.Data[key], payload[key]; !reflect.DeepEqual(got, want) {
			t.Fatalf("payload[%s] = %#v, want %#v", key, got, want)
		}
	}
	if event.Data["audit_event_id"] == "" {
		t.Fatal("payload audit_event_id is empty")
	}
	if event.Data["user_id"] != payload["actor_user_id"] {
		t.Fatalf("legacy user_id = %#v, want actor_user_id %#v", event.Data["user_id"], payload["actor_user_id"])
	}
	if event.Data["success"] != true {
		t.Fatalf("legacy success = %#v, want true", event.Data["success"])
	}
}

func readPlatformEventFixture(t *testing.T, name string) contracts.EventEnvelope {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "contracts", "fixtures", "events", "v1", name))
	if err != nil {
		t.Fatalf("read event fixture %s: %v", name, err)
	}
	fixture, err := contracts.DecodeEventEnvelope(raw)
	if err != nil {
		t.Fatalf("decode event fixture %s: %v", name, err)
	}
	return fixture
}

func clonePlatformFixturePayload(in map[string]any) map[string]any {
	out := make(map[string]any, len(in)+1)
	for key, value := range in {
		out[key] = value
	}
	return out
}

func requirePlatformProducerEvent(t *testing.T, app *App, name string) contracts.Event {
	t.Helper()
	for _, event := range app.Events.Outbox() {
		if event.Name == name {
			return event
		}
	}
	t.Fatalf("missing produced event %s in outbox %#v", name, app.Events.Outbox())
	return contracts.Event{}
}

func assertPlatformEventMetadata(t *testing.T, event contracts.Event, fixture contracts.EventEnvelope, wantIdempotency string) {
	t.Helper()
	if event.Name != fixture.EventType {
		t.Fatalf("event name = %q, want %q", event.Name, fixture.EventType)
	}
	if event.Source != fixture.Producer {
		t.Fatalf("event source = %q, want %q", event.Source, fixture.Producer)
	}
	if event.SchemaVersion != fixture.SchemaVersion {
		t.Fatalf("schema version = %d, want %d", event.SchemaVersion, fixture.SchemaVersion)
	}
	if event.EventID == "" {
		t.Fatal("event_id is empty")
	}
	if event.TraceID != fixture.TraceID {
		t.Fatalf("trace_id = %q, want %q", event.TraceID, fixture.TraceID)
	}
	if event.OccurredAt.IsZero() {
		t.Fatal("occurred_at is zero")
	}
	if event.IdempotencyKey != wantIdempotency {
		t.Fatalf("idempotency_key = %q, want %q", event.IdempotencyKey, wantIdempotency)
	}
}

func assertPlatformPayloadContains(t *testing.T, got, want map[string]any) {
	t.Helper()
	for key, wantValue := range want {
		if gotValue, ok := got[key]; !ok || !reflect.DeepEqual(gotValue, wantValue) {
			t.Fatalf("payload[%s] = %#v, want %#v", key, gotValue, wantValue)
		}
	}
}
