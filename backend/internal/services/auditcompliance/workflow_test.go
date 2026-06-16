package auditcompliance

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

func TestAuditLogHandlerFiltersAndPaginates(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: serviceName, HTTPAddr: ":0"})
	seedAuditRows(t, app)

	code, data, degraded := getAuditLogs(app, auditRequest("/api/v1/audit/logs?user_id=alice&resource_type=auth&action=login_failed&limit=1", "ADMIN", "admin"), platform.RouteSpec{})
	if degraded != nil || code != http.StatusOK {
		t.Fatalf("audit logs status=%d degraded=%v data=%#v, want 200", code, degraded, data)
	}
	response := data.(map[string]any)
	logs := response["list"].([]AuditLog)
	if response["total"] != int64(2) || len(logs) != 1 || logs[0].UserID != "alice" {
		t.Fatalf("paginated audit response = %#v, want first alice login failure", response)
	}
	if response["page"].(int) != 1 || response["page_size"].(int) != 1 {
		t.Fatalf("pagination metadata = %#v, want first page with size 1", response)
	}

	code, data, _ = getAuditLogs(app, auditRequest("/api/v1/audit/logs", "", "admin"), platform.RouteSpec{})
	if code != http.StatusUnauthorized {
		t.Fatalf("anonymous audit status=%d data=%#v, want unauthorized", code, data)
	}
	code, data, _ = getAuditLogs(app, auditRequest("/api/v1/audit/logs", "U1", "user"), platform.RouteSpec{})
	if code != http.StatusForbidden {
		t.Fatalf("user audit status=%d data=%#v, want forbidden", code, data)
	}
	code, data, _ = getAuditLogs(app, auditRequest("/api/v1/audit/logs?start_time=bad-date", "ADMIN", "admin"), platform.RouteSpec{})
	if code != http.StatusBadRequest {
		t.Fatalf("bad query status=%d data=%#v, want bad request", code, data)
	}
}

func TestProjectReportDownloadUsesProjectedMembership(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: serviceName, HTTPAddr: ":0"})
	seedAuditRows(t, app)
	publishProjectMemberTestEvent(t, app, "project_memberCreated", map[string]any{"project_id": "P1", "user_id": "U1"})

	target := "/api/v1/audit/report?project_id=P1&start=2026-04-01T00:00:00Z&end=2026-04-03T00:00:00Z"
	code, data, degraded := downloadProjectReport(app, auditRequest(target, "U1", ""), platform.RouteSpec{})
	if degraded != nil || code != http.StatusOK {
		t.Fatalf("report status=%d degraded=%v data=%#v, want 200", code, degraded, data)
	}
	raw := data.(platform.RawResponse)
	body := string(raw.Body)
	if raw.ContentType != "text/csv" || !strings.Contains(raw.Headers["Content-Disposition"], "audit_report_P1.csv") {
		t.Fatalf("raw response headers = %#v contentType=%q, want CSV attachment", raw.Headers, raw.ContentType)
	}
	if !strings.Contains(body, "login_failed") || strings.Contains(body, "P2") {
		t.Fatalf("CSV body = %q, want only matching project rows", body)
	}

	code, data, _ = downloadProjectReport(app, auditRequest("/api/v1/audit/report?project_id=P1&start=2026-04-01T00:00:00Z", "U2", ""), platform.RouteSpec{})
	if code != http.StatusForbidden {
		t.Fatalf("non-member report status=%d data=%#v, want forbidden", code, data)
	}
	code, data, _ = downloadProjectReport(app, auditRequest("/api/v1/audit/report?start=2026-04-01T00:00:00Z", "U1", ""), platform.RouteSpec{})
	if code != http.StatusBadRequest {
		t.Fatalf("missing project report status=%d data=%#v, want bad request", code, data)
	}
}

func TestSecurityPostureSummarizesRecentRiskSignals(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: serviceName, HTTPAddr: ":0"})
	now := time.Now().UTC()
	createAuditRow(t, app, auditRow{id: "fail-1", userID: "alice", projectID: "P1", action: "login_failed", resourceType: "auth", resourceID: "browser", createdAt: now.Add(-1 * time.Hour)})
	createAuditRow(t, app, auditRow{id: "fail-2", userID: "alice", projectID: "P1", action: "login_failed", resourceType: "auth", resourceID: "cli", createdAt: now.Add(-2 * time.Hour)})
	createAuditRow(t, app, auditRow{id: "role-1", userID: "admin", projectID: "P1", action: "role_changed", resourceType: "role", resourceID: "R1", createdAt: now.Add(-3 * time.Hour)})
	createAuditRow(t, app, auditRow{id: "policy-1", userID: "admin", projectID: "P1", action: "policy_removed", resourceType: "policy", resourceID: "POL1", createdAt: now.Add(-4 * time.Hour)})
	createAuditRow(t, app, auditRow{id: "old-fail", userID: "bob", projectID: "P2", action: "login_failed", resourceType: "auth", resourceID: "browser", createdAt: now.AddDate(0, 0, -10)})

	code, data, degraded := getSecurityPosture(app, auditRequest("/api/v1/admin/security/posture?window_days=7", "ADMIN", "super-admin"), platform.RouteSpec{})
	if degraded != nil || code != http.StatusOK {
		t.Fatalf("posture status=%d degraded=%v data=%#v, want 200", code, degraded, data)
	}
	posture := data.(SecurityPosture)
	if posture.AuthFailures24h != 2 || posture.RoleChanges7d != 1 || posture.PolicyMutations7d != 1 {
		t.Fatalf("posture counters = %#v, want recent failures, role changes, and policy mutations", posture)
	}
	if len(posture.TopOffenders) == 0 || posture.TopOffenders[0].UserID != "alice" || posture.TopOffenders[0].FailureCount != 2 {
		t.Fatalf("top offenders = %#v, want alice first with two failures", posture.TopOffenders)
	}
	if len(posture.RecentEvents) != 5 {
		t.Fatalf("recent events = %#v, want all retained security events", posture.RecentEvents)
	}

	assertPostureGuard(t, app, "/api/v1/admin/security/posture", "", "admin", http.StatusUnauthorized)
	assertPostureGuard(t, app, "/api/v1/admin/security/posture", "U1", "user", http.StatusForbidden)
	assertPostureGuard(t, app, "/api/v1/admin/security/posture?window_days=0", "ADMIN", "admin", http.StatusBadRequest)
	assertPostureGuard(t, app, "/api/v1/admin/security/posture?window_days=91", "ADMIN", "admin", http.StatusBadRequest)
}

func TestAuditHelpersComposeMapsAndDefaults(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: serviceName, HTTPAddr: ":0"})
	publishAuditEvent(t, app, "event-1", time.Date(2026, time.April, 3, 10, 0, 0, 0, time.UTC), map[string]any{
		"user_id":       "event-user",
		"action":        "policy_added",
		"resource":      "policy",
		"resource_id":   "POL2",
		"description":   "policy created",
		"source_ip":     "127.0.0.1",
		"user_agent":    "agent",
		"project_id":    "P3",
		"ignored_value": "ignored",
	})
	createAuditRow(t, app, auditRow{id: "store-1", userID: "store-user", action: "logout", resourceType: "auth", resourceID: "session", createdAt: time.Date(2026, time.April, 2, 10, 0, 0, 0, time.UTC)})

	maps := RecentAuditLogMaps(app, auditRequest("/", "ADMIN", "admin"), 1)
	if len(maps) != 1 || maps[0]["id"] != "event-1" || maps[0]["project_id"] != "P3" {
		t.Fatalf("recent maps = %#v, want limited newest event with project", maps)
	}
	if all := RecentAuditLogMaps(nil, auditRequest("/", "ADMIN", "admin"), 10); len(all) != 0 {
		t.Fatalf("nil app maps = %#v, want empty", all)
	}

	log := logFromMap("fallback", map[string]any{
		"userId":       "U3",
		"projectId":    " P4 ",
		"resourceType": "dataset",
		"resourceId":   "D1",
		"createdAt":    "2026-04-04T12:00:00Z",
		"oldData":      map[string]any{"status": "old"},
		"newData":      map[string]any{"status": "new"},
		"description":  "changed",
	}, time.Time{})
	if log.ID != "fallback" || log.ProjectID == nil || *log.ProjectID != "P4" || log.OldData["status"] != "old" {
		t.Fatalf("mapped log = %#v, want alternate keys and maps converted", log)
	}

	logs := []AuditLog{{ID: "a"}, {ID: "b"}, {ID: "c"}}
	if got := pageLogs(logs, 0, 1); len(got) != 2 || got[0].ID != "b" {
		t.Fatalf("page limit zero = %#v, want remaining rows", got)
	}
	if got := pageLogs(logs, 5, 10); len(got) != 0 {
		t.Fatalf("page beyond end = %#v, want empty", got)
	}
	if !paginationRequested(auditRequest("/?page_size=50", "ADMIN", "admin")) {
		t.Fatal("paginationRequested = false, want page_size to request pagination")
	}
	if timeValue(map[string]any{"bad": "not-a-time"}, "bad").IsZero() != true {
		t.Fatal("invalid timeValue should return zero time")
	}
}

func seedAuditRows(t *testing.T, app *platform.App) {
	t.Helper()
	createAuditRow(t, app, auditRow{id: "store-login", userID: "alice", projectID: "P1", action: "login_failed", resourceType: "auth", resourceID: "browser", createdAt: time.Date(2026, time.April, 2, 10, 0, 0, 0, time.UTC)})
	createAuditRow(t, app, auditRow{id: "store-role", userID: "alice", projectID: "P1", action: "role_changed", resourceType: "role", resourceID: "R1", createdAt: time.Date(2026, time.April, 2, 9, 0, 0, 0, time.UTC)})
	createAuditRow(t, app, auditRow{id: "store-other", userID: "bob", projectID: "P2", action: "login_failed", resourceType: "auth", resourceID: "browser", createdAt: time.Date(2026, time.April, 2, 8, 0, 0, 0, time.UTC)})
	publishAuditEvent(t, app, "event-login", time.Date(2026, time.April, 2, 11, 0, 0, 0, time.UTC), map[string]any{
		"user_id":       "alice",
		"project_id":    "P1",
		"action":        "login_failed",
		"resource_type": "auth",
		"resource_id":   "cli",
		"description":   "bad password",
	})
}

type auditRow struct {
	id           string
	userID       string
	projectID    string
	action       string
	resourceType string
	resourceID   string
	createdAt    time.Time
}

func createAuditRow(t *testing.T, app *platform.App, entry auditRow) {
	t.Helper()
	row := map[string]any{
		"id":            entry.id,
		"user_id":       entry.userID,
		"action":        entry.action,
		"resource_type": entry.resourceType,
		"resource_id":   entry.resourceID,
		"description":   entry.action + " description",
		"created_at":    entry.createdAt,
	}
	if entry.projectID != "" {
		row["project_id"] = entry.projectID
	}
	if _, err := app.Store.Create(context.Background(), auditLogResource, row); err != nil {
		t.Fatal(err)
	}
}

func publishAuditEvent(t *testing.T, app *platform.App, id string, occurredAt time.Time, data map[string]any) {
	t.Helper()
	if err := app.Events.Publish(context.Background(), contracts.Event{
		EventID:       id,
		Name:          "AuditEvent",
		Source:        serviceName,
		OccurredAt:    occurredAt,
		TraceID:       platform.NewUUID(),
		SchemaVersion: 1,
		Data:          data,
	}); err != nil {
		t.Fatal(err)
	}
}

func auditRequest(target, userID, role string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, target, nil)
	if userID != "" {
		req.Header.Set("X-User-ID", userID)
	}
	if role != "" {
		req.Header.Set("X-User-Role", role)
	}
	return req
}

func assertPostureGuard(t *testing.T, app *platform.App, target, userID, role string, want int) {
	t.Helper()
	code, data, _ := getSecurityPosture(app, auditRequest(target, userID, role), platform.RouteSpec{})
	if code != want {
		t.Fatalf("posture guard %s status=%d data=%#v, want %d", target, code, data, want)
	}
}
