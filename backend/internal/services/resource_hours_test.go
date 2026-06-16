package services

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

func TestResourceHoursUsageWorkflow(t *testing.T) {
	app := newTestApp()
	seedResourceUsage(t, app)

	requestJSON(t, app, http.MethodGet, "/api/v1/me/request-usage", "", nil, http.StatusUnauthorized)
	myUsage := responseSlice(t, requestJSON(t, app, http.MethodGet, "/api/v1/me/request-usage?since=2026-04-01", "", userHeaders("U1"), http.StatusOK))
	if len(myUsage) != 1 || myUsage[0].(map[string]any)["UserID"] != "U1" {
		t.Fatalf("my usage = %#v, want only U1 current usage", myUsage)
	}

	oldFiltered := responseSlice(t, requestJSON(t, app, http.MethodGet, "/api/v1/me/request-usage?since=2026-05-01", "", userHeaders("U1"), http.StatusOK))
	if len(oldFiltered) != 0 {
		t.Fatalf("usage since 2026-05-01 = %#v, want no rows", oldFiltered)
	}

	requestJSON(t, app, http.MethodGet, "/api/v1/admin/request-usage", "", userHeaders("U1"), http.StatusForbidden)
	adminUsage := responseMap(t, requestJSON(t, app, http.MethodGet, "/api/v1/admin/request-usage?username=ALICE&finalized=false&since=2026-04-01", "", adminHeaders("ADMIN"), http.StatusOK))
	if adminUsage["rowCount"] != float64(1) || adminUsage["since"] != "2026-04-01" {
		t.Fatalf("admin usage = %#v, want one filtered row since 2026-04-01", adminUsage)
	}
	summary := adminUsage["summary"].(map[string]any)
	if summary["totalCPUHours"] != float64(2) || summary["totalGPUHours"] != float64(1) || summary["uniqueUsers"] != float64(1) {
		t.Fatalf("summary = %#v, want totals for alice row", summary)
	}
}

func seedResourceUsage(t *testing.T, app *platform.App) {
	t.Helper()
	april := time.Date(2026, time.April, 2, 0, 0, 0, 0, time.UTC)
	march := time.Date(2026, time.March, 20, 0, 0, 0, 0, time.UTC)
	rows := []map[string]any{
		{"id": "rh1", "user_id": "U1", "username": "alice", "project_id": "P1", "project_name": "proj", "job_id": "J1", "cpu_hours": 2.0, "gpu_hours": 1.0, "memory_gb_hours": 4.0, "period_start": april, "is_finalized": false},
		{"id": "rh2", "user_id": "U2", "username": "bob", "project_id": "P2", "job_id": "J2", "cpu_hours": 9.0, "period_start": april, "is_finalized": true},
		{"id": "rh3", "user_id": "U1", "username": "alice", "project_id": "P1", "job_id": "old", "cpu_hours": 1.0, "period_start": march, "is_finalized": true},
	}
	for _, row := range rows {
		if _, err := app.Store.Create(context.Background(), "usage-observability-service:request_usage", row); err != nil {
			t.Fatal(err)
		}
	}
}
