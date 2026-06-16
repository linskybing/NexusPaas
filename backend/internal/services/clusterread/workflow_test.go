package clusterread

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

func TestClusterHandlersServeSummaryNodesAndGPUUsage(t *testing.T) {
	app := newClusterHandlerApp(t)

	code, data, degraded := getClusterSummary(app, clusterRequest("/api/v1/cluster/summary", "U1"), platform.RouteSpec{})
	if degraded != nil || code != http.StatusOK {
		t.Fatalf("summary status=%d degraded=%v data=%#v, want 200", code, degraded, data)
	}
	summary := data.(map[string]any)
	if summary["nodes"] != nil || summary["podGpuUsages"] != nil || summary["nodeCount"] != 2 {
		t.Fatalf("public summary = %#v, want sanitized cluster counters", summary)
	}

	code, data, _ = listClusterNodes(app, clusterRequest("/api/v1/cluster/nodes", "ADMIN"), platform.RouteSpec{})
	if code != http.StatusOK {
		t.Fatalf("nodes status=%d data=%#v, want 200", code, data)
	}
	nodes := data.([]map[string]any)
	if len(nodes) != 2 || nodes[0]["name"] != "gpu-a" || nodes[1]["name"] != "gpu-b" {
		t.Fatalf("nodes = %#v, want sorted node list", nodes)
	}

	nodeReq := clusterRequest("/api/v1/cluster/nodes/gpu-b", "ADMIN")
	nodeReq.SetPathValue("name", "gpu-b")
	code, data, _ = getClusterNode(app, nodeReq, platform.RouteSpec{})
	if code != http.StatusOK || data.(map[string]any)["name"] != "gpu-b" {
		t.Fatalf("node status=%d data=%#v, want gpu-b", code, data)
	}

	code, data, _ = listPodGPUUsage(app, clusterRequest("/api/v1/cluster/gpu-usage", "ADMIN"), platform.RouteSpec{})
	if code != http.StatusOK || len(data.([]map[string]any)) != 3 {
		t.Fatalf("pod GPU status=%d data=%#v, want three usage rows", code, data)
	}
}

func TestClusterProjectGPUHandlersEnforceVisibility(t *testing.T) {
	app := newClusterHandlerApp(t)

	code, data, _ := getProjectsGPUUsageByUser(app, clusterRequest("/api/v1/projects/gpu-usage/by-user", "U1"), platform.RouteSpec{})
	if code != http.StatusOK {
		t.Fatalf("projects by user status=%d data=%#v, want 200", code, data)
	}
	usageByProject := data.(map[string]int64)
	if usageByProject["P1"] != 2 {
		t.Fatalf("visible project usage = %#v, want P1 with two GPU pods", usageByProject)
	}
	if _, ok := usageByProject["P2"]; ok {
		t.Fatalf("visible project usage = %#v, want inaccessible P2 omitted", usageByProject)
	}

	projectReq := clusterRequest("/api/v1/projects/P1/gpu-usage", "U1")
	projectReq.SetPathValue("id", "P1")
	code, data, _ = getProjectGPUUsage(app, projectReq, platform.RouteSpec{})
	if code != http.StatusOK || data.(map[string]any)["used"] != int64(2) {
		t.Fatalf("project P1 usage status=%d data=%#v, want used=2", code, data)
	}

	forbiddenReq := clusterRequest("/api/v1/projects/P2/gpu-usage", "U1")
	forbiddenReq.SetPathValue("id", "P2")
	code, data, _ = getProjectGPUUsage(app, forbiddenReq, platform.RouteSpec{})
	if code != http.StatusForbidden {
		t.Fatalf("project P2 user status=%d data=%#v, want forbidden", code, data)
	}

	adminReq := clusterRequest("/api/v1/projects/P2/gpu-usage", "ADMIN")
	adminReq.SetPathValue("id", "P2")
	code, data, _ = getProjectGPUUsage(app, adminReq, platform.RouteSpec{})
	if code != http.StatusOK || data.(map[string]any)["used"] != int64(1) {
		t.Fatalf("project P2 admin status=%d data=%#v, want used=1", code, data)
	}
}

func TestClusterHandlerGuards(t *testing.T) {
	app := newClusterHandlerApp(t)
	assertClusterStatus(t, getClusterSummary, app, clusterRequest("/api/v1/cluster/summary", ""), http.StatusUnauthorized)
	assertClusterStatus(t, listClusterNodes, app, clusterRequest("/api/v1/cluster/nodes", "U1"), http.StatusForbidden)
	assertClusterStatus(t, listPodGPUUsage, app, clusterRequest("/api/v1/cluster/gpu-usage", ""), http.StatusUnauthorized)
	assertClusterStatus(t, getProjectsGPUUsageByUser, app, clusterRequest("/api/v1/projects/gpu-usage/by-user", ""), http.StatusBadRequest)

	missingNameReq := clusterRequest("/api/v1/cluster/nodes/", "ADMIN")
	assertClusterStatus(t, getClusterNode, app, missingNameReq, http.StatusBadRequest)

	notFoundNodeReq := clusterRequest("/api/v1/cluster/nodes/missing", "ADMIN")
	notFoundNodeReq.SetPathValue("name", "missing")
	assertClusterStatus(t, getClusterNode, app, notFoundNodeReq, http.StatusNotFound)

	missingProjectReq := clusterRequest("/api/v1/projects//gpu-usage", "ADMIN")
	assertClusterStatus(t, getProjectGPUUsage, app, missingProjectReq, http.StatusBadRequest)

	notFoundProjectReq := clusterRequest("/api/v1/projects/P3/gpu-usage", "ADMIN")
	notFoundProjectReq.SetPathValue("id", "P3")
	assertClusterStatus(t, getProjectGPUUsage, app, notFoundProjectReq, http.StatusNotFound)
}

func TestClusterSummaryHelpersAndCoHostedFallbacks(t *testing.T) {
	req := clusterRequest("/", "ADMIN")
	if empty := clusterSummary(nil, req); empty["nodeCount"] != 0 || len(nodeList(empty)) != 0 {
		t.Fatalf("empty summary = %#v, want zero cluster state", empty)
	}
	if !recordGrantsAdminPanel(map[string]any{"Capabilities": `{"adminPanel":"true"}`}) {
		t.Fatal("recordGrantsAdminPanel should parse JSON capability maps")
	}
	if got := anySlice(map[string]any{"bad": "value"}, "bad"); got != nil {
		t.Fatalf("anySlice = %#v, want nil for non-slice", got)
	}
	if clone := cloneMap(nil); len(clone) != 0 {
		t.Fatalf("cloneMap(nil) = %#v, want empty map", clone)
	}

	visible := map[string]struct{}{"PARENT": {}}
	if !projectDescendsFromVisibleProject(map[string]any{"parent_id": "PARENT"}, visible) {
		t.Fatal("project should descend from visible parent_id")
	}
	if !projectDescendsFromVisibleProject(map[string]any{"path": "ROOT.PARENT.CHILD"}, visible) {
		t.Fatal("project should descend from visible path segment")
	}
	if projectDescendsFromVisibleProject(map[string]any{"path": ""}, visible) {
		t.Fatal("project with empty path should not descend from visible project")
	}

	app := platform.NewApp(platform.Config{ServiceName: "all", HTTPAddr: ":0"})
	createClusterRows(t, app, orgProjectsResource, []map[string]any{{"id": "SOURCE", "project_name": "source"}})
	createClusterRows(t, app, clusterProjectsResource, []map[string]any{{"id": "LOCAL", "project_name": "local"}})
	records := clusterRecords(app, req, clusterProjectsResource, orgProjectsResource)
	if len(records) != 2 {
		t.Fatalf("co-hosted records = %#v, want local and source rows", records)
	}
}

func newClusterHandlerApp(t *testing.T) *platform.App {
	t.Helper()
	app := platform.NewApp(platform.Config{ServiceName: serviceName, HTTPAddr: ":0"})
	createClusterRows(t, app, clusterIdentityUsersResource, []map[string]any{
		{"id": "ADMIN", "capabilities": map[string]any{"adminPanel": true}},
		{"id": "U1", "role_id": "user"},
	})
	createClusterRows(t, app, clusterProjectsResource, []map[string]any{
		{"id": "P1", "project_name": "vision"},
		{"id": "P2", "project_name": "language"},
	})
	createClusterRows(t, app, clusterProjectMembersResource, []map[string]any{
		{"project_id": "P1", "user_id": "U1"},
	})
	createClusterRows(t, app, clusterReadModelResource, []map[string]any{{"id": "cluster", "summary": clusterSummaryFixture()}})
	return app
}

func clusterSummaryFixture() map[string]any {
	return map[string]any{
		"nodeCount":    2,
		"totalGpuUsed": 3,
		"collectedAt":  time.Date(2026, time.April, 2, 10, 0, 0, 0, time.UTC),
		"nodes":        []any{map[string]any{"name": "gpu-b"}, map[string]any{"name": "gpu-a"}},
		"podGpuUsages": []any{map[string]any{"namespace": "project-P1"}, map[string]any{"namespace": "project-P1"}, map[string]any{"namespace": "project-P2"}},
	}
}

func createClusterRows(t *testing.T, app *platform.App, resource string, rows []map[string]any) {
	t.Helper()
	for _, row := range rows {
		if _, err := app.Store.Create(context.Background(), resource, row); err != nil {
			t.Fatal(err)
		}
	}
}

func clusterRequest(target, userID string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, target, nil)
	if userID != "" {
		req.Header.Set("X-User-ID", userID)
	}
	return req
}

func assertClusterStatus(t *testing.T, handler platform.HandlerFunc, app *platform.App, req *http.Request, want int) {
	t.Helper()
	code, data, _ := handler(app, req, platform.RouteSpec{})
	if code != want {
		t.Fatalf("%s status=%d data=%#v, want %d", req.URL.Path, code, data, want)
	}
}
