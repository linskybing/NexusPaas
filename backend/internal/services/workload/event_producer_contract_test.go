package workload

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

func TestJobSubmittedProducerMatchesV1Fixture(t *testing.T) {
	fixture := readWorkloadEventFixture(t, "job-submitted.json")
	app := platform.NewApp(platform.Config{ServiceName: serviceName, HTTPAddr: ":0"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs", nil)
	req.Header.Set("X-Trace-ID", fixture.TraceID)
	req.Header.Set("Idempotency-Key", "idem-job-submitted-v1")

	publish(app, req, fixture.EventType, "submitted", fixture.Payload)

	event := requireWorkloadProducerEvent(t, app, fixture.EventType)
	assertWorkloadEventMetadata(t, event, fixture, "idem-job-submitted-v1")
	assertWorkloadPayloadContains(t, event.Data, fixture.Payload)
	if event.Data["action"] != "submitted" {
		t.Fatalf("payload action = %#v, want submitted", event.Data["action"])
	}
}

func readWorkloadEventFixture(t *testing.T, name string) contracts.EventEnvelope {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "contracts", "fixtures", "events", "v1", name))
	if err != nil {
		t.Fatalf("read event fixture %s: %v", name, err)
	}
	fixture, err := contracts.DecodeEventEnvelope(raw)
	if err != nil {
		t.Fatalf("decode event fixture %s: %v", name, err)
	}
	return fixture
}

func requireWorkloadProducerEvent(t *testing.T, app *platform.App, name string) contracts.Event {
	t.Helper()
	for _, event := range app.Events.Outbox() {
		if event.Name == name {
			return event
		}
	}
	t.Fatalf("missing produced event %s in outbox %#v", name, app.Events.Outbox())
	return contracts.Event{}
}

func assertWorkloadEventMetadata(t *testing.T, event contracts.Event, fixture contracts.EventEnvelope, wantIdempotency string) {
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

func assertWorkloadPayloadContains(t *testing.T, got, want map[string]any) {
	t.Helper()
	for key, wantValue := range want {
		if gotValue, ok := got[key]; !ok || !reflect.DeepEqual(gotValue, wantValue) {
			t.Fatalf("payload[%s] = %#v, want %#v", key, gotValue, wantValue)
		}
	}
}
