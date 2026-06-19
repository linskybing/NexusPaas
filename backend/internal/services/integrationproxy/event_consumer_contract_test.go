package integrationproxy

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

func TestIdentityAdminConsumerMatchesUserUpdatedFixture(t *testing.T) {
	fixture := readIntegrationProxyEventFixture(t, "user-updated.json")
	app := platform.NewApp(platform.Config{ServiceName: serviceName})
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	if err := projectIdentityAdminEvent(app, req, integrationProxyEventFromFixture(fixture)); err != nil {
		t.Fatalf("projectIdentityAdminEvent: %v", err)
	}

	userID := fixture.Payload["user_id"].(string)
	record, ok := app.Store.Get(context.Background(), proxyAdminUsersResource, userID)
	if !ok {
		t.Fatalf("missing projected admin user %q", userID)
	}
	if record.ID != userID || record.Data["id"] != userID {
		t.Fatalf("projected id = %q/%#v, want %q", record.ID, record.Data["id"], userID)
	}
	assertIntegrationProxyPayloadField(t, record.Data, fixture.Payload, "user_id")
	assertIntegrationProxyPayloadField(t, record.Data, fixture.Payload, "display_name")
	assertIntegrationProxyPayloadField(t, record.Data, fixture.Payload, "status")
	assertIntegrationProxyPayloadField(t, record.Data, fixture.Payload, "role_ids")
	if got := len(app.Store.List(context.Background(), identityUsersResource)); got != 0 {
		t.Fatalf("source identity users = %d, want isolated consumer to avoid owner store", got)
	}
}

func readIntegrationProxyEventFixture(t *testing.T, name string) contracts.EventEnvelope {
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

func integrationProxyEventFromFixture(fixture contracts.EventEnvelope) contracts.Event {
	return contracts.Event{
		EventID:       fixture.EventID,
		Name:          fixture.EventType,
		Source:        fixture.Producer,
		OccurredAt:    fixture.OccurredAt,
		TraceID:       fixture.TraceID,
		SchemaVersion: fixture.SchemaVersion,
		Data:          cloneIntegrationProxyPayload(fixture.Payload),
	}
}

func cloneIntegrationProxyPayload(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func assertIntegrationProxyPayloadField(t *testing.T, got, want map[string]any, key string) {
	t.Helper()
	if !reflect.DeepEqual(got[key], want[key]) {
		t.Fatalf("projected payload[%s] = %#v, want %#v", key, got[key], want[key])
	}
}
