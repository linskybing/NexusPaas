package auditcompliance

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

func TestAuditComplianceConsumerMatchesAuditEventFixture(t *testing.T) {
	fixture := readAuditComplianceEventFixture(t, "audit-event.json")
	app := platform.NewApp(platform.Config{ServiceName: serviceName})
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	if err := app.Events.Publish(context.Background(), auditComplianceEventFromFixture(fixture)); err != nil {
		t.Fatalf("publish audit event fixture: %v", err)
	}

	logs := auditLogs(app, req)
	if len(logs) != 1 {
		t.Fatalf("audit logs = %d, want 1", len(logs))
	}

	entry := logs[0]
	assertAuditComplianceLogMatchesFixture(t, entry, fixture)

	rows := RecentAuditLogMaps(app, req, 1)
	if len(rows) != 1 {
		t.Fatalf("recent audit rows = %d, want 1", len(rows))
	}
	assertAuditComplianceRowMatchesFixture(t, rows[0], fixture)

	if got := len(app.Store.List(context.Background(), auditLogResource)); got != 0 {
		t.Fatalf("audit owner-store logs = %d, want isolated consumer to avoid owner store", got)
	}
}

func readAuditComplianceEventFixture(t *testing.T, name string) contracts.EventEnvelope {
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

func auditComplianceEventFromFixture(fixture contracts.EventEnvelope) contracts.Event {
	return contracts.Event{
		EventID:       fixture.EventID,
		Name:          fixture.EventType,
		Source:        fixture.Producer,
		OccurredAt:    fixture.OccurredAt,
		TraceID:       fixture.TraceID,
		SchemaVersion: fixture.SchemaVersion,
		Data:          cloneAuditCompliancePayload(fixture.Payload),
	}
}

func cloneAuditCompliancePayload(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func assertAuditComplianceLogMatchesFixture(t *testing.T, got AuditLog, fixture contracts.EventEnvelope) {
	t.Helper()
	if got.ID != fixture.Payload["audit_event_id"] {
		t.Fatalf("audit log id = %q, want %#v", got.ID, fixture.Payload["audit_event_id"])
	}
	if got.UserID != fixture.Payload["actor_user_id"] {
		t.Fatalf("audit log user_id = %q, want %#v", got.UserID, fixture.Payload["actor_user_id"])
	}
	if got.Action != fixture.Payload["action"] {
		t.Fatalf("audit log action = %q, want %#v", got.Action, fixture.Payload["action"])
	}
	if got.ResourceType != fixture.Payload["resource_type"] {
		t.Fatalf("audit log resource_type = %q, want %#v", got.ResourceType, fixture.Payload["resource_type"])
	}
	if got.ResourceID != fixture.Payload["resource_id"] {
		t.Fatalf("audit log resource_id = %q, want %#v", got.ResourceID, fixture.Payload["resource_id"])
	}
	if !got.CreatedAt.Equal(fixture.OccurredAt) {
		t.Fatalf("audit log created_at = %s, want %s", got.CreatedAt.Format(time.RFC3339), fixture.OccurredAt.Format(time.RFC3339))
	}
}

func assertAuditComplianceRowMatchesFixture(t *testing.T, got map[string]any, fixture contracts.EventEnvelope) {
	t.Helper()
	if got["id"] != fixture.Payload["audit_event_id"] {
		t.Fatalf("recent audit row id = %#v, want %#v", got["id"], fixture.Payload["audit_event_id"])
	}
	if got["user_id"] != fixture.Payload["actor_user_id"] {
		t.Fatalf("recent audit row user_id = %#v, want %#v", got["user_id"], fixture.Payload["actor_user_id"])
	}
	if got["action"] != fixture.Payload["action"] {
		t.Fatalf("recent audit row action = %#v, want %#v", got["action"], fixture.Payload["action"])
	}
	if got["resource_type"] != fixture.Payload["resource_type"] {
		t.Fatalf("recent audit row resource_type = %#v, want %#v", got["resource_type"], fixture.Payload["resource_type"])
	}
	if got["resource_id"] != fixture.Payload["resource_id"] {
		t.Fatalf("recent audit row resource_id = %#v, want %#v", got["resource_id"], fixture.Payload["resource_id"])
	}
	if got["created_at"] != fixture.OccurredAt.Format(time.RFC3339) {
		t.Fatalf("recent audit row created_at = %#v, want %s", got["created_at"], fixture.OccurredAt.Format(time.RFC3339))
	}
}
