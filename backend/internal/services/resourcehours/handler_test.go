package resourcehours

import (
	"context"
	"fmt"
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

	adminReq := usageRequest("/api/v1/admin/request-usage?username=ALICE&group_id=G1&project_id=P1&finalized=false&since=2026-04-01", "ADMIN", "super-admin")
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
		"GroupID":         " G3 ",
		"GroupName":       " Group Three ",
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
	if row.GroupID != "G3" || row.GroupName != "Group Three" {
		t.Fatalf("group fields = %#v, want normalized alternate-key values", row)
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

func TestAdminUsageLargeGroupQueryFiltersAndSummarizes(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: "usage-observability-service", HTTPAddr: ":0"})
	Register(app)

	const (
		targetGroupID = "G-large"
		projectCount  = 10
		userCount     = 25
	)
	seedLargeGroupUsageRows(t, app, targetGroupID, projectCount, userCount)
	seedLargeGroupNoiseRows(t, app, targetGroupID)

	req := usageRequest("/api/v1/admin/request-usage?group_id=G-large&since=2026-04-01", "ADMIN", "admin")
	code, data, degraded := getAdminUsage(app, req, platform.RouteSpec{})
	if degraded != nil || code != http.StatusOK {
		t.Fatalf("admin usage status=%d degraded=%v data=%#v, want 200", code, degraded, data)
	}

	assertLargeGroupUsageResponse(t, data.(AdminUsageResponse), targetGroupID, projectCount, userCount)
}

func seedLargeGroupUsageRows(t *testing.T, app *platform.App, groupID string, projectCount, userCount int) {
	t.Helper()
	ctx := context.Background()
	for project := range projectCount {
		for user := range userCount {
			row := map[string]any{
				"id":              fmt.Sprintf("large-%02d-%02d", project, user),
				"user_id":         fmt.Sprintf("U%02d", user),
				"username":        fmt.Sprintf("user-%02d", user),
				"group_id":        groupID,
				"group_name":      "Large Group",
				"project_id":      fmt.Sprintf("P%02d", project),
				"project_name":    fmt.Sprintf("Project %02d", project),
				"job_id":          fmt.Sprintf("J-%02d-%02d", project, user),
				"cpu_hours":       1.25,
				"gpu_hours":       0.5,
				"memory_gb_hours": 2.0,
				"period_start":    "2026-04-15T00:00:00Z",
				"is_finalized":    true,
			}
			if _, err := app.Store.Create(ctx, resourceName, row); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func seedLargeGroupNoiseRows(t *testing.T, app *platform.App, targetGroupID string) {
	t.Helper()
	ctx := context.Background()
	noiseRows := []map[string]any{
		{
			"id":              "noise-other-group",
			"user_id":         "noise-user",
			"username":        "noise",
			"group_id":        "G-noise",
			"project_id":      "P-noise",
			"job_id":          "J-noise",
			"cpu_hours":       999.0,
			"gpu_hours":       999.0,
			"memory_gb_hours": 999.0,
			"period_start":    "2026-04-20T00:00:00Z",
			"is_finalized":    true,
		},
		{
			"id":              "old-target-group",
			"user_id":         "old-user",
			"username":        "old",
			"group_id":        targetGroupID,
			"project_id":      "P-old",
			"job_id":          "J-old",
			"cpu_hours":       777.0,
			"gpu_hours":       777.0,
			"memory_gb_hours": 777.0,
			"period_start":    "2026-03-31T00:00:00Z",
			"is_finalized":    true,
		},
	}
	for _, row := range noiseRows {
		if _, err := app.Store.Create(ctx, resourceName, row); err != nil {
			t.Fatal(err)
		}
	}
}

func assertLargeGroupUsageResponse(t *testing.T, response AdminUsageResponse, targetGroupID string, projectCount, userCount int) {
	t.Helper()
	const (
		expectedCPUHours     = 312.5
		expectedGPUHours     = 125.0
		expectedMemoryGBHour = 500.0
	)
	expectedRows := projectCount * userCount
	if response.RowCount != expectedRows || len(response.Rows) != expectedRows {
		t.Fatalf("row count = %d rows=%d, want %d", response.RowCount, len(response.Rows), expectedRows)
	}
	if response.Since != "2026-04-01" || response.Filters["group_id"] != targetGroupID {
		t.Fatalf("filters/since = since %q filters %#v, want target group since 2026-04-01", response.Since, response.Filters)
	}
	if response.Summary.UniqueUsers != userCount || response.Summary.UniqueProjects != projectCount || response.Summary.UniqueGroups != 1 {
		t.Fatalf("summary counts = %#v, want %d users, %d projects, one group", response.Summary, userCount, projectCount)
	}
	if response.Summary.TotalCPUHours != expectedCPUHours ||
		response.Summary.TotalGPUHours != expectedGPUHours ||
		response.Summary.TotalMemoryGBHours != expectedMemoryGBHour {
		t.Fatalf("summary totals = %#v, want cpu %.1f gpu %.1f memory %.1f", response.Summary, expectedCPUHours, expectedGPUHours, expectedMemoryGBHour)
	}
	assertLargeGroupUsageRows(t, response.Rows, targetGroupID)
}

func assertLargeGroupUsageRows(t *testing.T, rows []UserResourceHours, targetGroupID string) {
	t.Helper()
	since, err := time.Parse(dateLayout, "2026-04-01")
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row.GroupID != targetGroupID {
			t.Fatalf("row group = %#v, want only %s", row, targetGroupID)
		}
		if row.JobID == "J-old" {
			t.Fatalf("old target-group row returned: %#v", row)
		}
		if row.PeriodStart == nil || row.PeriodStart.Before(since) {
			t.Fatalf("row period_start = %#v, want not before %s", row, since.Format(dateLayout))
		}
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
			"group_id":            "G1",
			"group_name":          "Group One",
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
			"group_id":     "G2",
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
			"group_id":     "G1",
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
	if response.Filters["username"] != "ALICE" || response.Filters["group_id"] != "G1" || response.Filters["finalized"] != "false" {
		t.Fatalf("filters = %#v, want request filters preserved", response.Filters)
	}
	if response.Rows[0].GroupID != "G1" {
		t.Fatalf("admin row group = %#v, want G1", response.Rows[0])
	}
	if response.Summary.UniqueUsers != 1 || response.Summary.UniqueProjects != 1 || response.Summary.UniqueGroups != 1 {
		t.Fatalf("summary counts = %#v, want one user, one project, and one group", response.Summary)
	}
	if response.Summary.TotalCPUHours != 2 || response.Summary.TotalGPUHours != 1 || response.Summary.TotalMemoryGBHours != 4 {
		t.Fatalf("summary totals = %#v, want totals for alice row", response.Summary)
	}
}
