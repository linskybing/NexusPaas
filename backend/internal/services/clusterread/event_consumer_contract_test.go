package clusterread

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

func TestClusterReadConsumerMatchesUserUpdatedFixture(t *testing.T) {
	fixture := readClusterReadEventFixture(t, "user-updated.json")
	app := platform.NewApp(platform.Config{ServiceName: serviceName})
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	if err := projectClusterReadEvent(app, req, clusterReadEventFromFixture(fixture)); err != nil {
		t.Fatalf("projectClusterReadEvent: %v", err)
	}

	userID := fixture.Payload["user_id"].(string)
	record, ok := app.Store.Get(context.Background(), clusterIdentityUsersResource, userID)
	if !ok {
		t.Fatalf("missing projected cluster identity user %q", userID)
	}
	if record.ID != userID || record.Data["id"] != userID {
		t.Fatalf("projected id = %q/%#v, want %q", record.ID, record.Data["id"], userID)
	}
	assertClusterReadPayloadField(t, record.Data, fixture.Payload, "user_id")
	assertClusterReadPayloadField(t, record.Data, fixture.Payload, "display_name")
	assertClusterReadPayloadField(t, record.Data, fixture.Payload, "status")
	assertClusterReadPayloadField(t, record.Data, fixture.Payload, "role_ids")
	if got := len(app.Store.List(context.Background(), identityUsersResource)); got != 0 {
		t.Fatalf("source identity users = %d, want isolated consumer to avoid owner store", got)
	}
}

func TestClusterReadConsumerMatchesProjectUpdatedFixture(t *testing.T) {
	fixture := readClusterReadEventFixture(t, "project-updated.json")
	app := platform.NewApp(platform.Config{ServiceName: serviceName})
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	if err := projectClusterReadEvent(app, req, clusterReadEventFromFixture(fixture)); err != nil {
		t.Fatalf("projectClusterReadEvent: %v", err)
	}

	projectID := fixture.Payload["project_id"].(string)
	record, ok := app.Store.Get(context.Background(), clusterProjectsResource, projectID)
	if !ok {
		t.Fatalf("missing projected cluster project %q", projectID)
	}
	if record.ID != projectID || record.Data["id"] != projectID {
		t.Fatalf("projected id = %q/%#v, want %q", record.ID, record.Data["id"], projectID)
	}
	assertClusterReadPayloadField(t, record.Data, fixture.Payload, "project_id")
	assertClusterReadPayloadField(t, record.Data, fixture.Payload, "org_id")
	assertClusterReadPayloadField(t, record.Data, fixture.Payload, "slug")
	assertClusterReadPayloadField(t, record.Data, fixture.Payload, "quota_plan_id")
	assertClusterReadPayloadField(t, record.Data, fixture.Payload, "member_count")
	if got := len(app.Store.List(context.Background(), orgProjectsResource)); got != 0 {
		t.Fatalf("source org projects = %d, want isolated consumer to avoid owner store", got)
	}
}

func readClusterReadEventFixture(t *testing.T, name string) contracts.EventEnvelope {
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

func clusterReadEventFromFixture(fixture contracts.EventEnvelope) contracts.Event {
	return contracts.Event{
		EventID:       fixture.EventID,
		Name:          fixture.EventType,
		Source:        fixture.Producer,
		OccurredAt:    fixture.OccurredAt,
		TraceID:       fixture.TraceID,
		SchemaVersion: fixture.SchemaVersion,
		Data:          cloneClusterReadPayload(fixture.Payload),
	}
}

func cloneClusterReadPayload(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func assertClusterReadPayloadField(t *testing.T, got, want map[string]any, key string) {
	t.Helper()
	if !reflect.DeepEqual(got[key], want[key]) {
		t.Fatalf("projected payload[%s] = %#v, want %#v", key, got[key], want[key])
	}
}
