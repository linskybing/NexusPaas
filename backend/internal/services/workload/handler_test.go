package workload

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

func TestWorkloadConfigFileWorkflow(t *testing.T) {
	app := newWorkloadTestApp()

	code, data, _ := createConfigFile(app, workloadRequest(http.MethodPost, "/api/v1/configfiles?project_id=P1", `{"id":"cfg1","name":"train.yaml","path":"jobs/train.yaml","content":"kind: Job"}`), platform.RouteSpec{})
	assertWorkloadStatus(t, code, data, http.StatusCreated)
	if data.(contracts.Record[map[string]any]).Data["project_id"] != "P1" {
		t.Fatalf("created config = %#v, want project P1", data)
	}

	getReq := workloadRequest(http.MethodGet, "/api/v1/configfiles/cfg1", "")
	getReq.SetPathValue("id", "cfg1")
	code, data, _ = getConfigFile(app, getReq, platform.RouteSpec{})
	assertWorkloadStatus(t, code, data, http.StatusOK)

	updateReq := workloadRequest(http.MethodPut, "/api/v1/configfiles/cfg1", `{"content":"kind: Pod","message":"second"}`)
	updateReq.SetPathValue("id", "cfg1")
	code, data, _ = updateConfigFile(app, updateReq, platform.RouteSpec{})
	assertWorkloadStatus(t, code, data, http.StatusOK)
	if versions := app.Store.List(context.Background(), versionsResource); len(versions) != 2 {
		t.Fatalf("versions = %d, want create and update versions", len(versions))
	}

	projectReq := workloadRequest(http.MethodGet, "/api/v1/configfiles/project/P1", "")
	projectReq.SetPathValue("project_id", "P1")
	code, data, _ = listConfigFilesByProject(app, projectReq, platform.RouteSpec{})
	assertWorkloadStatus(t, code, data, http.StatusOK)
	if records := data.([]contracts.Record[map[string]any]); len(records) != 1 || records[0].ID != "cfg1" {
		t.Fatalf("project configs = %#v, want cfg1", data)
	}

	treeReq := workloadRequest(http.MethodGet, "/api/v1/configfiles/project/P1/tree", "")
	treeReq.SetPathValue("project_id", "P1")
	code, data, _ = configFileTree(app, treeReq, platform.RouteSpec{})
	assertWorkloadStatus(t, code, data, http.StatusOK)
	if nodes := data.(map[string]any)["nodes"].([]map[string]any); len(nodes) != 1 || nodes[0]["path"] != "jobs/train.yaml" {
		t.Fatalf("tree = %#v, want config path", data)
	}

	historyReq := workloadRequest(http.MethodGet, "/api/v1/configfiles/project/P1/history", "")
	historyReq.SetPathValue("project_id", "P1")
	code, data, _ = projectConfigHistory(app, historyReq, platform.RouteSpec{})
	assertWorkloadStatus(t, code, data, http.StatusOK)
	if history := data.([]contracts.Record[map[string]any]); len(history) != 2 {
		t.Fatalf("history = %#v, want two versions", history)
	}
}

func TestWorkloadConfigInstanceCommands(t *testing.T) {
	app := newWorkloadTestApp()
	createWorkloadRecord(t, app, configsResource, map[string]any{"id": "cfg1", "project_id": "P1", "name": "train.yaml"})
	createWorkloadRecord(t, app, instancesResource, map[string]any{"id": "pod1", "config_id": "cfg1", "pod": "train-pod"})

	startReq := workloadRequest(http.MethodPost, "/api/v1/configfiles/cfg1/instance", `{"namespace":"proj-p1"}`)
	startReq.SetPathValue("id", "cfg1")
	code, data, _ := startConfigInstance(app, startReq, platform.RouteSpec{})
	assertWorkloadStatus(t, code, data, http.StatusAccepted)
	if data.(contracts.Record[map[string]any]).Data["action"] != "start" {
		t.Fatalf("start command = %#v, want action start", data)
	}

	stopReq := workloadRequest(http.MethodDelete, "/api/v1/configfiles/cfg1/instance", `{}`)
	stopReq.SetPathValue("id", "cfg1")
	code, data, _ = stopConfigInstance(app, stopReq, platform.RouteSpec{})
	assertWorkloadStatus(t, code, data, http.StatusAccepted)

	podsReq := workloadRequest(http.MethodGet, "/api/v1/configfiles/cfg1/instance/pods", "")
	podsReq.SetPathValue("id", "cfg1")
	code, data, _ = listConfigInstancePods(app, podsReq, platform.RouteSpec{})
	assertWorkloadStatus(t, code, data, http.StatusOK)
	if pods := data.([]contracts.Record[map[string]any]); len(pods) != 1 || pods[0].ID != "pod1" {
		t.Fatalf("pods = %#v, want pod1", data)
	}
}

func TestWorkloadMalformedJSONDoesNotWrite(t *testing.T) {
	app := newWorkloadTestApp()
	code, data, _ := createConfigFile(app, workloadRequest(http.MethodPost, "/api/v1/configfiles", `{`), platform.RouteSpec{})
	assertWorkloadStatus(t, code, data, http.StatusBadRequest)
	if got := len(app.Store.List(context.Background(), configsResource)); got != 0 {
		t.Fatalf("config count = %d, want 0", got)
	}
}

func newWorkloadTestApp() *platform.App {
	app := platform.NewApp(platform.Config{ServiceName: "all", HTTPAddr: ":0"})
	Register(app)
	return app
}

func workloadRequest(method, target, body string) *http.Request {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Idempotency-Key", "test-"+method+"-"+target)
	return req
}

func createWorkloadRecord(t *testing.T, app *platform.App, resource string, data map[string]any) {
	t.Helper()
	if _, err := app.Store.Create(context.Background(), resource, data); err != nil {
		t.Fatal(err)
	}
}

func assertWorkloadStatus(t *testing.T, code int, data any, want int) {
	t.Helper()
	if code != want {
		t.Fatalf("status=%d data=%#v, want %d", code, data, want)
	}
}
