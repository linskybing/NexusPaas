package dashboard

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

func TestRegisterUsesEventFedReadModelsInsteadOfExternalStoreDependencies(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: serviceName})
	Register(app)

	if err := app.ValidateServiceIsolation(); err != nil {
		t.Fatalf("dashboard should use local event-fed read models, got isolation error: %v", err)
	}
}

func TestDashboardHandlersUseProjectedReadModels(t *testing.T) {
	app := projectedDashboardApp(t)

	req := dashboardRequest("U1", "alice", "user")
	code, data, degraded := getOverview(app, req, platform.RouteSpec{})
	if degraded != nil || code != http.StatusOK {
		t.Fatalf("overview status=%d degraded=%v data=%#v, want 200", code, degraded, data)
	}
	assertProjectedOverview(t, data.(map[string]any))

	adminReq := dashboardRequest("ADMIN", "admin", "admin")
	code, data, degraded = getAdminSummary(app, adminReq, platform.RouteSpec{})
	if degraded != nil || code != http.StatusOK {
		t.Fatalf("admin status=%d degraded=%v data=%#v, want 200", code, degraded, data)
	}
	assertProjectedAdminSummary(t, data.(map[string]any))
	if _, ok := app.Store.Get(context.Background(), dashboardProjectMembersResource, "P1:U1"); !ok {
		t.Fatal("missing composite-key projected membership")
	}
	if got := len(app.Store.List(context.Background(), identityUsersResource)); got != 0 {
		t.Fatalf("source identity records = %d, want isolated dashboard to avoid owner store", got)
	}
}

func projectedDashboardApp(t *testing.T) *platform.App {
	t.Helper()
	app := platform.NewApp(platform.Config{ServiceName: serviceName, HTTPAddr: ":0"})
	ctx := context.Background()
	if _, err := app.Store.Create(ctx, clusterReadModelsResource, map[string]any{
		"id": "cluster",
		"nodes": []any{
			map[string]any{"name": "gpu-prod-1", "gpuAllocatable": 2.0, "cpuAllocatable": 64.0},
			map[string]any{"name": "cpu-prod-1", "gpuAllocatable": 0.0},
		},
		"podGpuUsages": []any{map[string]any{"pod": "private"}},
	}); err != nil {
		t.Fatal(err)
	}
	publishTestEvent(t, app, "UserCreated", map[string]any{"id": "U1", "username": "alice", "status": "online"})
	publishTestEvent(t, app, "UserCreated", map[string]any{"id": "ADMIN", "username": "admin", "status": "online"})
	publishTestEvent(t, app, "ProjectCreated", map[string]any{"id": "P1", "name": "Projected project"})
	publishTestEvent(t, app, "project_memberCreated", map[string]any{"project_id": "P1", "user_id": "U1"})
	publishTestEvent(t, app, "FormCreated", map[string]any{"id": "F1", "user_id": "U1", "status": "Pending", "title": "Need GPU"})
	publishTestEvent(t, app, "live_quotaCreated", map[string]any{"id": "P1", "project_id": "P1", "gpu_limit": 4.0})
	publishTestEvent(t, app, "queueCreated", map[string]any{"id": "q1", "is_preemptible": true})
	return app
}

func assertProjectedOverview(t *testing.T, overview map[string]any) {
	t.Helper()
	if len(overview["projects"].([]map[string]any)) != 1 || len(overview["activities"].([]map[string]any)) != 1 {
		t.Fatalf("overview = %#v, want projected project and activity", overview)
	}
	if overview["preemptibleQueueCount"] != 1 {
		t.Fatalf("preemptibleQueueCount = %#v, want 1", overview["preemptibleQueueCount"])
	}
	cluster := overview["clusterSummary"].(map[string]any)
	if cluster["podGpuUsages"] != nil {
		t.Fatalf("podGpuUsages = %#v, want nil", cluster["podGpuUsages"])
	}
	nodes := cluster["nodes"].([]any)
	if len(nodes) != 1 || nodes[0].(map[string]any)["name"] != "GPU node 1" {
		t.Fatalf("public nodes = %#v, want sanitized GPU node", nodes)
	}
	quota := overview["projectQuotaLiveById"].(map[string]any)["P1"].(map[string]any)
	if quota["gpu_limit"] != 4.0 {
		t.Fatalf("quota = %#v, want projected live quota", quota)
	}
}

func assertProjectedAdminSummary(t *testing.T, summary map[string]any) {
	t.Helper()
	if summary["totalUsers"] != int64(2) {
		t.Fatalf("totalUsers = %#v, want 2", summary["totalUsers"])
	}
	if summary["pendingRequestsCount"] != 1 {
		t.Fatalf("pendingRequestsCount = %#v, want 1", summary["pendingRequestsCount"])
	}
}

func TestDashboardProjectionDeletesAndFallsBackWhenCoHosted(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: "all", HTTPAddr: ":0"})
	ctx := context.Background()
	req := dashboardRequest("U1", "alice", "user")
	if _, err := app.Store.Create(ctx, identityUsersResource, map[string]any{"id": "U1", "username": "source"}); err != nil {
		t.Fatal(err)
	}
	if users := userRecords(app, req); len(users) != 1 || users[0]["username"] != "source" {
		t.Fatalf("fallback users = %#v, want co-hosted source records", users)
	}

	projectDashboardEvent(app, req, contracts.Event{
		EventID:       platform.NewUUID(),
		Name:          "UserCreated",
		Source:        "identity-service",
		OccurredAt:    time.Now().UTC(),
		TraceID:       platform.NewUUID(),
		SchemaVersion: 1,
		Data:          map[string]any{"new": map[string]any{"id": "U2", "username": "projected"}},
	})
	if users := userRecords(app, req); len(users) != 1 || users[0]["username"] != "projected" {
		t.Fatalf("projected users = %#v, want local read model to override fallback", users)
	}
	projectDashboardEvent(app, req, contracts.Event{
		EventID:       platform.NewUUID(),
		Name:          "UserDeleted",
		Source:        "identity-service",
		OccurredAt:    time.Now().UTC(),
		TraceID:       platform.NewUUID(),
		SchemaVersion: 1,
		Data:          map[string]any{"id": "U2", "deleted": true},
	})
	if _, ok := app.Store.Get(ctx, dashboardUsersResource, "U2"); ok {
		t.Fatal("projected user was not deleted")
	}
	projectDashboardEvent(app, req, contracts.Event{Name: "UnknownEvent", Data: map[string]any{"id": "noop"}})
	deleteReadModel(app, req, dashboardUsersResource, map[string]any{"id": "U1", "deleted": false})
}

func dashboardRequest(userID, username, role string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/overview", nil)
	req.Header.Set("X-User-ID", userID)
	req.Header.Set("X-Username", username)
	req.Header.Set("X-User-Role", role)
	return req
}

func publishTestEvent(t *testing.T, app *platform.App, name string, data map[string]any) {
	t.Helper()
	if err := app.Events.Publish(context.Background(), contracts.Event{
		EventID:       platform.NewUUID(),
		Name:          name,
		Source:        "test",
		OccurredAt:    time.Now().UTC(),
		TraceID:       platform.NewUUID(),
		SchemaVersion: 1,
		Data:          data,
	}); err != nil {
		t.Fatal(err)
	}
}
