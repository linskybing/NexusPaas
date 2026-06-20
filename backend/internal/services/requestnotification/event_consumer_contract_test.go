package requestnotification

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

func TestRequestNotificationConsumerMatchesProjectUpdatedFixture(t *testing.T) {
	fixture := readRequestNotificationEventFixture(t, "project-updated.json")
	app := platform.NewApp(platform.Config{ServiceName: serviceName})
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	if err := projectProjectAccessEvent(app, req, requestNotificationEventFromFixture(fixture)); err != nil {
		t.Fatalf("projectProjectAccessEvent: %v", err)
	}

	projectID := fixture.Payload["project_id"].(string)
	record, ok := app.Store.Get(context.Background(), projectAccessProjects, projectID)
	if !ok {
		t.Fatalf("missing projected request notification project access %q", projectID)
	}
	if record.ID != projectID || record.Data["id"] != projectID {
		t.Fatalf("projected id = %q/%#v, want %q", record.ID, record.Data["id"], projectID)
	}
	assertRequestNotificationPayloadField(t, record.Data, fixture.Payload, "project_id")
	assertRequestNotificationPayloadField(t, record.Data, fixture.Payload, "org_id")
	assertRequestNotificationPayloadField(t, record.Data, fixture.Payload, "slug")
	assertRequestNotificationPayloadField(t, record.Data, fixture.Payload, "quota_plan_id")
	assertRequestNotificationPayloadField(t, record.Data, fixture.Payload, "member_count")
	if got := len(app.Store.List(context.Background(), orgProjectsResource)); got != 0 {
		t.Fatalf("source org projects = %d, want isolated consumer to avoid owner store", got)
	}
}

func readRequestNotificationEventFixture(t *testing.T, name string) contracts.EventEnvelope {
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

func requestNotificationEventFromFixture(fixture contracts.EventEnvelope) contracts.Event {
	return contracts.Event{
		EventID:       fixture.EventID,
		Name:          fixture.EventType,
		Source:        fixture.Producer,
		OccurredAt:    fixture.OccurredAt,
		TraceID:       fixture.TraceID,
		SchemaVersion: fixture.SchemaVersion,
		Data:          cloneRequestNotificationPayload(fixture.Payload),
	}
}

func cloneRequestNotificationPayload(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func assertRequestNotificationPayloadField(t *testing.T, got, want map[string]any, key string) {
	t.Helper()
	if !reflect.DeepEqual(got[key], want[key]) {
		t.Fatalf("projected payload[%s] = %#v, want %#v", key, got[key], want[key])
	}
}
