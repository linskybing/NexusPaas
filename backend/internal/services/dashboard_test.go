package services

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

func TestDashboardOverviewAndAdminSummary(t *testing.T) {
	app := newTestApp()
	seedDashboardData(t, app)

	requestJSON(t, app, http.MethodGet, "/api/v1/dashboard/overview", "", map[string]string{"X-User-ID": "U1"}, http.StatusUnauthorized)
	requestJSON(t, app, http.MethodGet, "/api/v1/dashboard/overview", "", userHeaders("missing"), http.StatusNotFound)
	requestJSON(t, app, http.MethodGet, "/api/v1/dashboard/overview", "", userHeaders("disabled"), http.StatusUnauthorized)

	overview := responseMap(t, requestJSON(t, app, http.MethodGet, "/api/v1/dashboard/overview", "", userHeaders("U1"), http.StatusOK))
	if len(overview["projects"].([]any)) != 1 || len(overview["activities"].([]any)) != 1 {
		t.Fatalf("overview = %#v, want one project and one activity", overview)
	}
	if overview["preemptibleQueueCount"] != float64(1) {
		t.Fatalf("preemptibleQueueCount = %v, want 1", overview["preemptibleQueueCount"])
	}
	cluster := overview["clusterSummary"].(map[string]any)
	nodes := cluster["nodes"].([]any)
	if len(nodes) != 1 || nodes[0].(map[string]any)["name"] != "GPU node 1" {
		t.Fatalf("public cluster nodes = %#v, want sanitized single GPU node", nodes)
	}
	if cluster["podGpuUsages"] != nil {
		t.Fatalf("podGpuUsages = %#v, want nil in public dashboard", cluster["podGpuUsages"])
	}

	requestJSON(t, app, http.MethodGet, "/api/v1/admin/dashboard-summary", "", userHeaders("U1"), http.StatusForbidden)
	summary := responseMap(t, requestJSON(t, app, http.MethodGet, "/api/v1/admin/dashboard-summary", "", adminHeaders("ADMIN"), http.StatusOK))
	if summary["totalUsers"] != float64(3) || summary["pendingRequestsCount"] != float64(1) {
		t.Fatalf("admin summary = %#v, want totalUsers 3 pending 1", summary)
	}
	if len(summary["recentLogs"].([]any)) != 2 {
		t.Fatalf("recentLogs = %#v, want two audit logs", summary["recentLogs"])
	}
}

func TestDashboardOverviewAndAdminSummaryWithEmptyUserStore(t *testing.T) {
	app := newTestApp()

	rec := requestJSON(t, app, http.MethodGet, "/api/v1/dashboard/overview", "", userHeaders("U1"), http.StatusNotFound)
	assertResponseDataMessage(t, rec, "User not found")

	rec = requestJSON(t, app, http.MethodGet, "/api/v1/admin/dashboard-summary", "", adminHeaders("ADMIN"), http.StatusNotFound)
	assertResponseDataMessage(t, rec, "User not found")
}

func TestDashboardProjectQuotaLiveUsesCatalogResourceKey(t *testing.T) {
	app := newTestApp()
	seedDashboardData(t, app)
	ctx := context.Background()

	staleQuota := map[string]any{
		"id":              "P1",
		"project_id":      "P1",
		"gpu_limit":       2.0,
		"gpu_used":        1.0,
		"source_resource": "stale",
	}
	if _, err := app.Store.Create(ctx, "scheduler-quota-service:project_quota_live", staleQuota); err != nil {
		t.Fatal(err)
	}

	overview := responseMap(t, requestJSON(t, app, http.MethodGet, "/api/v1/dashboard/overview", "", userHeaders("U1"), http.StatusOK))
	quotas := overview["projectQuotaLiveById"].(map[string]any)
	if len(quotas) != 0 {
		t.Fatalf("projectQuotaLiveById = %#v, want empty when only stale resource key is seeded", quotas)
	}
	requestJSON(t, app, http.MethodGet, "/api/v1/projects/P1/quota/live", "", userHeaders("U1"), http.StatusNotFound)

	liveQuota := map[string]any{
		"id":              "P1",
		"project_id":      "P1",
		"gpu_limit":       4.0,
		"gpu_used":        3.0,
		"source_resource": "live",
	}
	if _, err := app.Store.Create(ctx, "scheduler-quota-service:live_quotas", liveQuota); err != nil {
		t.Fatal(err)
	}

	routeQuota := responseMap(t, requestJSON(t, app, http.MethodGet, "/api/v1/projects/P1/quota/live", "", userHeaders("U1"), http.StatusOK))
	routeData := routeQuota["data"].(map[string]any)
	if routeData["source_resource"] != "live" {
		t.Fatalf("route quota data = %#v, want live resource data", routeData)
	}

	overview = responseMap(t, requestJSON(t, app, http.MethodGet, "/api/v1/dashboard/overview", "", userHeaders("U1"), http.StatusOK))
	quotas = overview["projectQuotaLiveById"].(map[string]any)
	rawQuota, ok := quotas["P1"].(map[string]any)
	if !ok {
		t.Fatalf("projectQuotaLiveById = %#v, want quota for P1", quotas)
	}
	if rawQuota["source_resource"] != "live" || rawQuota["gpu_limit"] != float64(4) {
		t.Fatalf("dashboard quota = %#v, want live resource data", rawQuota)
	}
}

func TestDashboardOverviewUsesEventFedReadModelsWhenIsolated(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: "usage-observability-service", HTTPAddr: ":0", APIKeys: map[string]bool{"test-key": true}, ExternalURLs: map[string]string{}})
	RegisterAll(app)
	ctx := context.Background()
	if _, err := app.Store.Create(ctx, "usage-observability-service:cluster_read_models", map[string]any{
		"id":    "cluster",
		"nodes": []any{map[string]any{"name": "gpu-prod-1", "gpuAllocatable": 2.0}},
	}); err != nil {
		t.Fatal(err)
	}
	publishDashboardTestEvent(t, app, "UserCreated", "identity-service", map[string]any{"id": "U1", "username": "alice", "status": "online"})
	publishDashboardTestEvent(t, app, "UserCreated", "identity-service", map[string]any{"id": "ADMIN", "username": "admin", "status": "online"})
	publishDashboardTestEvent(t, app, "ProjectCreated", "org-project-service", map[string]any{"id": "P1", "name": "Projected project"})
	publishDashboardTestEvent(t, app, "project_memberCreated", "org-project-service", map[string]any{"id": "pm1", "project_id": "P1", "user_id": "U1"})
	publishDashboardTestEvent(t, app, "FormCreated", "request-notification-service", map[string]any{"id": "F1", "user_id": "U1", "title": "Need GPU", "status": "Pending"})
	publishDashboardTestEvent(t, app, "queueCreated", "scheduler-quota-service", map[string]any{"id": "q1", "is_preemptible": true})
	publishDashboardTestEvent(t, app, "live_quotaCreated", "scheduler-quota-service", map[string]any{"id": "P1", "project_id": "P1", "gpu_limit": 4.0})
	publishDashboardTestEvent(t, app, "AuditEvent", "request-notification-service", map[string]any{"id": "audit1", "user_id": "U1", "action": "create", "resource_type": "forms", "success": true})

	overview := responseMap(t, requestJSON(t, app, http.MethodGet, "/api/v1/dashboard/overview", "", userHeaders("U1"), http.StatusOK))
	if len(overview["projects"].([]any)) != 1 || len(overview["activities"].([]any)) != 1 {
		t.Fatalf("projected overview = %#v, want project and activity from local read models", overview)
	}
	if overview["preemptibleQueueCount"] != float64(1) {
		t.Fatalf("preemptibleQueueCount = %#v, want 1", overview["preemptibleQueueCount"])
	}
	if quota := overview["projectQuotaLiveById"].(map[string]any)["P1"].(map[string]any); quota["gpu_limit"] != float64(4) {
		t.Fatalf("projected quota = %#v, want event-fed quota", quota)
	}

	summary := responseMap(t, requestJSON(t, app, http.MethodGet, "/api/v1/admin/dashboard-summary", "", adminHeaders("ADMIN"), http.StatusOK))
	if summary["totalUsers"] != float64(2) || summary["pendingRequestsCount"] != float64(1) {
		t.Fatalf("projected admin summary = %#v, want users and pending forms from local read models", summary)
	}
	if got := len(app.Store.List(ctx, "identity-service:users")); got != 0 {
		t.Fatalf("identity source records = %d, want dashboard to avoid remote owner store", got)
	}
}

func TestDashboardReadModelProjectionUsesCompositeMembershipKey(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: "usage-observability-service", HTTPAddr: ":0", APIKeys: map[string]bool{"test-key": true}, ExternalURLs: map[string]string{}})
	RegisterAll(app)

	publishDashboardTestEvent(t, app, "UserCreated", "identity-service", map[string]any{"id": "U1", "username": "alice", "status": "online"})
	publishDashboardTestEvent(t, app, "UserCreated", "identity-service", map[string]any{"id": "U2", "username": "bob", "status": "online"})
	publishDashboardTestEvent(t, app, "ProjectCreated", "org-project-service", map[string]any{"id": "P1", "name": "Projected project"})
	publishDashboardTestEvent(t, app, "project_memberCreated", "org-project-service", map[string]any{"project_id": "P1", "user_id": "U1"})
	publishDashboardTestEvent(t, app, "project_memberCreated", "org-project-service", map[string]any{"project_id": "P1", "user_id": "U2"})

	requestJSON(t, app, http.MethodGet, "/api/v1/dashboard/overview", "", userHeaders("U1"), http.StatusOK)
	members := app.Store.List(context.Background(), "usage-observability-service:dashboard_project_members")
	if len(members) != 2 {
		t.Fatalf("projected members = %#v, want two distinct records", members)
	}
	if _, ok := app.Store.Get(context.Background(), "usage-observability-service:dashboard_project_members", "P1:U1"); !ok {
		t.Fatal("missing projected P1:U1 membership")
	}
	if _, ok := app.Store.Get(context.Background(), "usage-observability-service:dashboard_project_members", "P1:U2"); !ok {
		t.Fatal("missing projected P1:U2 membership")
	}
}

func assertResponseDataMessage(t *testing.T, recBody *httptest.ResponseRecorder, want string) {
	t.Helper()
	var env struct {
		Success bool           `json:"success"`
		Data    map[string]any `json:"data"`
	}
	if err := json.NewDecoder(recBody.Body).Decode(&env); err != nil {
		t.Fatal(err)
	}
	if env.Success {
		t.Fatal("response was successful, want failure")
	}
	if env.Data["message"] != want {
		t.Fatalf("data.message = %#v, want %q", env.Data["message"], want)
	}
}

func publishDashboardTestEvent(t *testing.T, app *platform.App, name, source string, data map[string]any) {
	t.Helper()
	if err := app.Events.Publish(context.Background(), contracts.Event{
		EventID:       platform.NewUUID(),
		Name:          name,
		Source:        source,
		OccurredAt:    time.Now().UTC(),
		TraceID:       platform.NewUUID(),
		SchemaVersion: 1,
		Data:          data,
	}); err != nil {
		t.Fatal(err)
	}
}

func seedDashboardData(t *testing.T, app *platform.App) {
	t.Helper()
	ctx := context.Background()
	rows := []struct {
		resource string
		data     map[string]any
	}{
		{"identity-service:users", map[string]any{"id": "U1", "username": "alice", "status": "online"}},
		{"identity-service:users", map[string]any{"id": "disabled", "username": "disabled", "status": "disabled"}},
		{"identity-service:users", map[string]any{"id": "ADMIN", "username": "admin", "status": "online"}},
		{"org-project-service:projects", map[string]any{"id": "P1", "name": "Project 1"}},
		{"org-project-service:project_members", map[string]any{"id": "pm1", "project_id": "P1", "user_id": "U1"}},
		{"request-notification-service:forms", map[string]any{"id": "F1", "user_id": "U1", "title": "Need GPU", "status": "Pending"}},
		{"request-notification-service:forms", map[string]any{"id": "F2", "user_id": "U2", "title": "Done", "status": "Completed"}},
		{"scheduler-quota-service:queues", map[string]any{"id": "q1", "name": "preempt", "is_preemptible": true}},
		{"scheduler-quota-service:queues", map[string]any{"id": "q2", "name": "regular", "is_preemptible": false}},
		{"usage-observability-service:cluster_read_models", map[string]any{
			"id": "cluster",
			"nodes": []any{
				map[string]any{"name": "gpu-prod-1", "gpuAllocatable": 2.0, "cpuAllocatable": 64.0, "memoryAllocatable": 128.0},
				map[string]any{"name": "cpu-prod-1", "gpuAllocatable": 0.0, "cpuAllocatable": 32.0},
			},
			"podGpuUsages": []any{map[string]any{"pod": "secret"}},
		}},
		{"audit-compliance-service:audit_logs", map[string]any{"id": "a1", "user_id": "U1", "action": "login_failed", "resource_type": "auth", "created_at": time.Now().UTC().Format(time.RFC3339)}},
		{"audit-compliance-service:audit_logs", map[string]any{"id": "a2", "user_id": "ADMIN", "action": "policy_added", "resource_type": "rbac_policy", "created_at": time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)}},
	}
	for _, row := range rows {
		if _, err := app.Store.Create(ctx, row.resource, row.data); err != nil {
			t.Fatal(err)
		}
	}
}
