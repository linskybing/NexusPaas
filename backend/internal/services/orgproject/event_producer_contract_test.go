package orgproject

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
)

func TestProjectUpdatedProducerMatchesV1Fixture(t *testing.T) {
	fixture := readOrgProjectEventFixture(t, "project-updated.json")
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/projects/"+fixture.AggregateID, nil)
	req.Header.Set("X-Trace-ID", fixture.TraceID)
	req.Header.Set("Idempotency-Key", "idem-project-updated-v1")

	event := eventFor(req, fixture.EventType, fixture.Payload)

	assertOrgProjectEventMetadata(t, event, fixture, "idem-project-updated-v1")
	assertOrgProjectPayloadContains(t, event.Data, fixture.Payload)
}

func readOrgProjectEventFixture(t *testing.T, name string) contracts.EventEnvelope {
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

func assertOrgProjectEventMetadata(t *testing.T, event contracts.Event, fixture contracts.EventEnvelope, wantIdempotency string) {
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

func assertOrgProjectPayloadContains(t *testing.T, got, want map[string]any) {
	t.Helper()
	for key, wantValue := range want {
		if gotValue, ok := got[key]; !ok || !reflect.DeepEqual(gotValue, wantValue) {
			t.Fatalf("payload[%s] = %#v, want %#v", key, gotValue, wantValue)
		}
	}
}
