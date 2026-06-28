package authorizationpolicy

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestPolicyChangedProducerMatchesV1Fixture(t *testing.T) {
	fixture := readAuthorizationPolicyEventFixture(t, "policy-changed.json")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/permissions/batch", nil)
	req.Header.Set("X-Trace-ID", fixture.TraceID)
	req.Header.Set("Idempotency-Key", "idem-policy-changed-v1")
	data := cloneAuthorizationPolicyPayload(fixture.Payload)
	delete(data, "action")

	event := policyChangedEvent(req, "batch_permissions_processed", data)

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
	if event.IdempotencyKey != "idem-policy-changed-v1" {
		t.Fatalf("idempotency_key = %q, want idem-policy-changed-v1", event.IdempotencyKey)
	}
	for key, want := range fixture.Payload {
		if got, ok := event.Data[key]; !ok || !reflect.DeepEqual(got, want) {
			t.Fatalf("payload[%s] = %#v, want %#v", key, got, want)
		}
	}
}
