package services

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

func TestAuditComplianceWorkflow(t *testing.T) {
	app := newTestApp()
	seedAuditLogs(t, app)

	requestJSON(t, app, http.MethodGet, "/api/v1/audit/logs", "", nil, http.StatusUnauthorized)
	requestJSON(t, app, http.MethodGet, "/api/v1/audit/logs", "", userHeaders("U1"), http.StatusForbidden)
	requestJSON(t, app, http.MethodGet, "/api/v1/audit/logs?start_time=bad", "", adminHeaders("ADMIN"), http.StatusBadRequest)
	requestJSON(t, app, http.MethodGet, "/api/v1/audit/logs?limit=-1", "", adminHeaders("ADMIN"), http.StatusBadRequest)

	page := responseMap(t, requestJSON(t, app, http.MethodGet, "/api/v1/audit/logs?page=1&limit=1&action=create&resource_type=job", "", adminHeaders("ADMIN"), http.StatusOK))
	if page["total"] != float64(1) || page["page_size"] != float64(1) {
		t.Fatalf("audit page = %#v, want one create/job log", page)
	}
	list := page["list"].([]any)
	if len(list) != 1 || list[0].(map[string]any)["action"] != "create" {
		t.Fatalf("audit list = %#v, want create log", list)
	}

	start := "2026-04-01T00:00:00Z"
	requestRaw(t, app, http.MethodGet, "/api/v1/audit/report?project_id=P1&start="+start, userHeaders("U2"), http.StatusForbidden)
	_, _ = app.Store.Create(context.Background(), "org-project-service:project_members", map[string]any{"id": "pm1", "project_id": "P1", "user_id": "U1"})
	rec := requestRaw(t, app, http.MethodGet, "/api/v1/audit/report?project_id=P1&start="+start, userHeaders("U1"), http.StatusOK)
	if got := rec.Header().Get("Content-Type"); got != "text/csv" {
		t.Fatalf("report content-type = %q, want text/csv", got)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Time,User,Action,Resource Type,Resource ID,Description") || !strings.Contains(body, "U1,create,job,J1,created job") {
		t.Fatalf("report body = %q, want CSV header and project log", body)
	}

	requestJSON(t, app, http.MethodGet, "/api/v1/admin/security/posture?window_days=bad", "", adminHeaders("ADMIN"), http.StatusBadRequest)
	requestJSON(t, app, http.MethodGet, "/api/v1/admin/security/posture?window_days=91", "", adminHeaders("ADMIN"), http.StatusBadRequest)
	requestJSON(t, app, http.MethodGet, "/api/v1/admin/security/posture", "", userHeaders("U1"), http.StatusForbidden)
	posture := responseMap(t, requestJSON(t, app, http.MethodGet, "/api/v1/admin/security/posture?window_days=7", "", adminHeaders("ADMIN"), http.StatusOK))
	if posture["auth_failures_24h"] != float64(3) || posture["role_changes_7d"] != float64(1) || posture["policy_mutations_7d"] != float64(1) {
		t.Fatalf("posture = %#v, want aggregated security counts", posture)
	}
	offenders := posture["top_offenders"].([]any)
	if len(offenders) == 0 || offenders[0].(map[string]any)["user_id"] != "U1" || offenders[0].(map[string]any)["failure_count"] != float64(2) {
		t.Fatalf("top offenders = %#v, want U1 first with two failures", offenders)
	}
}

func seedAuditLogs(t *testing.T, app *platform.App) {
	t.Helper()
	april := time.Date(2026, time.April, 3, 0, 0, 0, 0, time.UTC)
	may := time.Date(2026, time.May, 1, 0, 0, 0, 0, time.UTC)
	recent := time.Now().UTC().Add(-2 * time.Hour)
	week := time.Now().UTC().Add(-48 * time.Hour)
	rows := []map[string]any{
		{"id": "a1", "user_id": "U1", "project_id": "P1", "action": "create", "resource_type": "job", "resource_id": "J1", "description": "created job", "created_at": april},
		{"id": "a2", "user_id": "U2", "project_id": "P2", "action": "delete", "resource_type": "project", "resource_id": "P2", "description": "deleted project", "created_at": may},
		{"id": "a3", "user_id": "U1", "action": "login_failed", "resource_type": "auth", "resource_id": "login", "created_at": recent},
		{"id": "a4", "user_id": "U1", "action": "login_failed", "resource_type": "auth", "resource_id": "login", "created_at": recent.Add(-time.Hour)},
		{"id": "a5", "user_id": "U2", "action": "login_failed", "resource_type": "auth", "resource_id": "login", "created_at": recent.Add(-2 * time.Hour)},
		{"id": "a6", "user_id": "ADMIN", "action": "role_changed", "resource_type": "user", "resource_id": "U1", "created_at": week},
		{"id": "a7", "user_id": "ADMIN", "action": "policy_added", "resource_type": "rbac_policy", "resource_id": "policy", "created_at": week},
	}
	for _, row := range rows {
		if _, err := app.Store.Create(context.Background(), "audit-compliance-service:audit_logs", row); err != nil {
			t.Fatal(err)
		}
	}
}
