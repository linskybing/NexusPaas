package resourcehours

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

func TestUsageHandlersFilterAndSummarize(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: "usage-observability-service", HTTPAddr: ":0"})
	Register(app)
	seedUsageRows(t, app)

	req := usageRequest("/api/v1/me/request-usage?since=2026-04-01", "U1", "")
	code, data, degraded := getMyUsage(app, req, platform.RouteSpec{})
	if degraded != nil || code != http.StatusOK {
		t.Fatalf("my usage status=%d degraded=%v data=%#v, want 200", code, degraded, data)
	}
	rows := data.([]UserResourceHours)
	if len(rows) != 1 || rows[0].UserID != "U1" || rows[0].JobID != "J1" {
		t.Fatalf("my usage rows = %#v, want only U1 current row", rows)
	}

	code, data, _ = getMyUsage(app, usageRequest("/api/v1/me/request-usage", "", ""), platform.RouteSpec{})
	if code != http.StatusUnauthorized {
		t.Fatalf("anonymous my usage status=%d data=%#v, want unauthorized", code, data)
	}

	code, data, _ = getAdminUsage(app, usageRequest("/api/v1/admin/request-usage", "U1", "user"), platform.RouteSpec{})
	if code != http.StatusForbidden {
		t.Fatalf("non-admin usage status=%d data=%#v, want forbidden", code, data)
	}

	adminReq := usageRequest("/api/v1/admin/request-usage?username=ALICE&project_id=P1&finalized=false&since=2026-04-01", "ADMIN", "super-admin")
	code, data, degraded = getAdminUsage(app, adminReq, platform.RouteSpec{})
	if degraded != nil || code != http.StatusOK {
		t.Fatalf("admin usage status=%d degraded=%v data=%#v, want 200", code, degraded, data)
	}
	response := data.(AdminUsageResponse)
	assertAdminUsageSummary(t, response)
}

func TestResourceHourRowConversionAndDefaults(t *testing.T) {
	if rows := usageRows(nil, usageRequest("/", "U1", ""), time.Now()); rows != nil {
		t.Fatalf("rows with nil app = %#v, want nil", rows)
	}
	if rows := usageRows(&platform.App{}, usageRequest("/", "U1", ""), time.Now()); rows != nil {
		t.Fatalf("rows with nil store = %#v, want nil", rows)
	}

	row := rowFromMap(map[string]any{
		"UserID":          " U3 ",
		"Username":        " carol ",
		"ProjectID":       "P3",
		"ProjectName":     "Project Three",
		"JobID":           "J3",
		"CPUHours":        float32(1.5),
		"GPUHours":        int64(2),
		"MemoryGBHours":   int(8),
		"PeriodStart":     "2026-04-05",
		"PeriodEnd":       "2026-04-06T00:00:00Z",
		"IsFinalized":     true,
		"LastComputedAt":  "not-a-time",
		"lastComputedAt":  "2026-04-07T00:00:00Z",
		"ignored_numeric": "3",
	})
	if row.UserID != "U3" || row.Username != "carol" || row.ProjectID != "P3" {
		t.Fatalf("text fields = %#v, want normalized alternate-key values", row)
	}
	if row.CPUHours != 1.5 || row.GPUHours != 2 || row.MemoryGBHours != 8 {
		t.Fatalf("numeric fields = %#v, want converted values", row)
	}
	if row.PeriodStart == nil || row.PeriodEnd == nil || row.LastComputedAt.IsZero() {
		t.Fatalf("time fields = %#v, want parsed dates", row)
	}
	if !row.IsFinalized {
		t.Fatal("IsFinalized = false, want true")
	}

	since := resolveSince("bad-date")
	if since.Day() != 1 || since.Hour() != 0 || since.Minute() != 0 {
		t.Fatalf("invalid since resolved to %v, want first day of current month at midnight", since)
	}
	if got := derefTime(nil); !got.IsZero() {
		t.Fatalf("deref nil time = %v, want zero", got)
	}
}

func seedUsageRows(t *testing.T, app *platform.App) {
	t.Helper()
	ctx := context.Background()
	rows := []map[string]any{
		{
			"id":                  "rh1",
			"user_id":             "U1",
			"username":            "alice",
			"project_id":          "P1",
			"project_name":        "proj",
			"job_id":              "J1",
			"cpu_hours":           2,
			"gpu_hours":           float32(1),
			"memory_gb_hours":     int64(4),
			"period_start":        "2026-04-02T00:00:00Z",
			"period_end":          "2026-04-03",
			"is_finalized":        false,
			"last_computed_at":    time.Date(2026, time.April, 4, 0, 0, 0, 0, time.UTC),
			"unrecognized_number": "ignored",
		},
		{
			"id":           "rh2",
			"user_id":      "U2",
			"username":     "bob",
			"project_id":   "P2",
			"job_id":       "J2",
			"cpu_hours":    9.0,
			"gpu_hours":    3.0,
			"period_start": time.Date(2026, time.April, 8, 0, 0, 0, 0, time.UTC),
			"is_finalized": true,
		},
		{
			"id":           "rh3",
			"user_id":      "U1",
			"username":     "alice",
			"project_id":   "P1",
			"job_id":       "old",
			"cpu_hours":    1.0,
			"period_start": "2026-03-20T00:00:00Z",
			"is_finalized": true,
		},
	}
	for _, row := range rows {
		if _, err := app.Store.Create(ctx, resourceName, row); err != nil {
			t.Fatal(err)
		}
	}
}

func usageRequest(target, userID, role string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, target, nil)
	if userID != "" {
		req.Header.Set("X-User-ID", userID)
	}
	if role != "" {
		req.Header.Set("X-User-Role", role)
	}
	return req
}

func assertAdminUsageSummary(t *testing.T, response AdminUsageResponse) {
	t.Helper()
	if response.RowCount != 1 || len(response.Rows) != 1 {
		t.Fatalf("admin rows = %#v, want one filtered row", response.Rows)
	}
	if response.Since != "2026-04-01" {
		t.Fatalf("since = %q, want 2026-04-01", response.Since)
	}
	if response.Filters["username"] != "ALICE" || response.Filters["finalized"] != "false" {
		t.Fatalf("filters = %#v, want request filters preserved", response.Filters)
	}
	if response.Summary.UniqueUsers != 1 || response.Summary.UniqueProjects != 1 {
		t.Fatalf("summary counts = %#v, want one user and one project", response.Summary)
	}
	if response.Summary.TotalCPUHours != 2 || response.Summary.TotalGPUHours != 1 || response.Summary.TotalMemoryGBHours != 4 {
		t.Fatalf("summary totals = %#v, want totals for alice row", response.Summary)
	}
}
