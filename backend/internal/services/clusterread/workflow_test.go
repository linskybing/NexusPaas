package clusterread

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/resourcehours"
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
	if summary["telemetry_stale"] != false || summary["collected_at"] == "" {
		t.Fatalf("public summary telemetry = %#v, want fresh telemetry metadata", summary)
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
	projectUsage := data.(map[string]any)
	if code != http.StatusOK || projectUsage["used"] != int64(2) {
		t.Fatalf("project P1 usage status=%d data=%#v, want used=2", code, data)
	}
	if projectUsage["telemetry_stale"] != false {
		t.Fatalf("project P1 telemetry = %#v, want fresh", projectUsage)
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

func TestClusterTelemetryMetadataMarksStaleAndMissingSnapshots(t *testing.T) {
	staleApp := newClusterHandlerAppWithSummary(t, clusterSummaryFixture(time.Now().UTC().Add(-3*time.Minute)))
	staleReq := clusterRequest("/api/v1/projects/P1/gpu-usage", "U1")
	staleReq.SetPathValue("id", "P1")
	code, data, _ := getProjectGPUUsage(staleApp, staleReq, platform.RouteSpec{})
	staleUsage := data.(map[string]any)
	if code != http.StatusOK || staleUsage["telemetry_stale"] != true {
		t.Fatalf("stale usage status=%d data=%#v, want telemetry_stale=true", code, data)
	}

	missing := clusterSummaryFixture(time.Now().UTC())
	delete(missing, "collectedAt")
	missingApp := newClusterHandlerAppWithSummary(t, missing)
	code, data, _ = getClusterSummary(missingApp, clusterRequest("/api/v1/cluster/summary", "U1"), platform.RouteSpec{})
	missingSummary := data.(map[string]any)
	if code != http.StatusOK || missingSummary["telemetry_stale"] != true || missingSummary["collected_at"] != "" {
		t.Fatalf("missing telemetry status=%d data=%#v, want missing timestamp marked stale", code, data)
	}
}

func TestSpoofedAdminRoleHeaderCannotReadProjectGPUUsageThroughServeHTTP(t *testing.T) {
	app := newStaticAdminClusterHTTPApp(t)

	noKey := serveProjectGPUUsage(app, map[string]string{"X-User-Role": "admin"}, "P2")
	if noKey.Code != http.StatusUnauthorized {
		t.Fatalf("spoofed admin header without API key status = %d, want 401: %s", noKey.Code, noKey.Body.String())
	}

	reader := serveProjectGPUUsage(app, map[string]string{"X-API-Key": "reader-key", "X-User-Role": "admin"}, "P2")
	if reader.Code != http.StatusForbidden {
		t.Fatalf("spoofed admin header with reader key status = %d, want 403: %s", reader.Code, reader.Body.String())
	}
}

func TestStaticAdminAPIKeyPrincipalCanReadProjectGPUUsageThroughServeHTTP(t *testing.T) {
	app := newStaticAdminClusterHTTPApp(t)

	rec := serveProjectGPUUsage(app, map[string]string{"X-API-Key": "admin-key"}, "P2")
	if rec.Code != http.StatusOK {
		t.Fatalf("static admin project GPU usage status = %d, want 200: %s", rec.Code, rec.Body.String())
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
	return newClusterHandlerAppWithSummary(t, clusterSummaryFixture(time.Now().UTC()))
}

func newClusterHandlerAppWithSummary(t *testing.T, summary map[string]any) *platform.App {
	t.Helper()
	app := platform.NewApp(platform.Config{ServiceName: serviceName, HTTPAddr: ":0", MaintenanceInterval: time.Minute})
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
	createClusterRows(t, app, clusterReadModelResource, []map[string]any{{"id": "cluster", "summary": summary}})
	return app
}

func newStaticAdminClusterHTTPApp(t *testing.T) *platform.App {
	t.Helper()
	app := platform.NewApp(platform.Config{
		ServiceName:  serviceName,
		HTTPAddr:     ":0",
		RequireAuth:  true,
		APIKeys:      map[string]bool{"admin-key": true, "reader-key": true},
		ExternalURLs: map[string]string{},
		APIKeyPrincipals: map[string]platform.APIKeyPrincipal{
			"admin-key":  {ID: "ops-admin", Username: "ops-admin", Admin: true},
			"reader-key": {ID: "ops-reader", Username: "ops-reader", Role: "user"},
		},
	})
	app.RegisterService(resourcehours.Spec())
	Register(app)
	createClusterRows(t, app, clusterProjectsResource, []map[string]any{{"id": "P2", "project_name": "language"}})
	createClusterRows(t, app, clusterReadModelResource, []map[string]any{{"id": "cluster", "summary": clusterSummaryFixture(time.Now().UTC())}})
	return app
}

func clusterSummaryFixture(collectedAt time.Time) map[string]any {
	return map[string]any{
		"nodeCount":    2,
		"totalGpuUsed": 3,
		"collectedAt":  collectedAt,
		"nodes":        []any{map[string]any{"name": "gpu-b"}, map[string]any{"name": "gpu-a"}},
		"podGpuUsages": []any{
			map[string]any{"project_id": "P1", "namespace": "proj-p1-alice"},
			map[string]any{"namespace": "project-P1"},
			map[string]any{"project_id": "P2", "namespace": "proj-p2-admin"},
		},
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

func serveProjectGPUUsage(app *platform.App, headers map[string]string, projectID string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+projectID+"/gpu-usage", nil)
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	return rec
}

func assertClusterStatus(t *testing.T, handler platform.HandlerFunc, app *platform.App, req *http.Request, want int) {
	t.Helper()
	code, data, _ := handler(app, req, platform.RouteSpec{})
	if code != want {
		t.Fatalf("%s status=%d data=%#v, want %d", req.URL.Path, code, data, want)
	}
}
