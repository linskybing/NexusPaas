package k8scontrol

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

func TestK8sControlAdapterHandlers(t *testing.T) {
	app := newK8sControlTestApp()
	adapter := &fakeK8sAdapter{}
	app.Adapters[adapterName] = adapter

	code, data, _ := listAdminResources(app, k8sRequest(http.MethodGet, "/api/v1/admin/resources", ""), platform.RouteSpec{})
	assertK8sStatus(t, code, data, http.StatusOK)
	if adapter.lastOperation != "list_admin_resources" || !adapter.lastIdempotent {
		t.Fatalf("adapter call = %s idempotent=%v, want list_admin_resources idempotent", adapter.lastOperation, adapter.lastIdempotent)
	}

	logsReq := k8sRequest(http.MethodGet, "/api/v1/k8s/namespaces/ns1/pods/pod1/logs", "")
	logsReq.SetPathValue("ns", "ns1")
	logsReq.SetPathValue("name", "pod1")
	code, data, _ = getPodLogs(app, logsReq, platform.RouteSpec{})
	assertK8sStatus(t, code, data, http.StatusOK)
	params := data.(map[string]any)["params"].(map[string]any)
	if params["namespace"] != "ns1" || params["pod"] != "pod1" {
		t.Fatalf("pod log params = %#v, want namespace/pod", params)
	}

	deleteReq := k8sRequest(http.MethodDelete, "/api/v1/resources/ns1/deploy/app", "")
	deleteReq.SetPathValue("namespace", "ns1")
	deleteReq.SetPathValue("kind", "deploy")
	deleteReq.SetPathValue("name", "app")
	code, data, _ = deleteResource(app, deleteReq, platform.RouteSpec{})
	assertK8sStatus(t, code, data, http.StatusOK)
	if adapter.lastOperation != "delete_resource" || adapter.lastIdempotent {
		t.Fatalf("adapter call = %s idempotent=%v, want delete_resource non-idempotent", adapter.lastOperation, adapter.lastIdempotent)
	}
}

func TestK8sControlValidationAndFailClosed(t *testing.T) {
	app := newK8sControlTestApp()
	code, data, _ := listAdminResources(app, k8sRequest(http.MethodGet, "/api/v1/admin/resources", ""), platform.RouteSpec{})
	assertK8sStatus(t, code, data, http.StatusBadGateway)

	app.Adapters[adapterName] = &fakeK8sAdapter{}
	code, data, _ = startUserStorageBrowse(app, k8sRequest(http.MethodPost, "/api/v1/k8s/user-storage/browse", `{`), platform.RouteSpec{})
	assertK8sStatus(t, code, data, http.StatusBadRequest)
}

func TestK8sControlProjectAndStorageWrappers(t *testing.T) {
	app := newK8sControlTestApp()
	adapter := &fakeK8sAdapter{}
	app.Adapters[adapterName] = adapter

	for _, tc := range []k8sWrapperCase{
		{
			name:          "delete admin project",
			method:        http.MethodDelete,
			request:       k8sRequest(http.MethodDelete, "/api/v1/admin/resources/projects/P1", ""),
			call:          deleteAdminProjectResources,
			wantOperation: "delete_admin_project_resources",
			wantParams:    map[string]any{"project_id": "P1"},
		},
		{
			name:          "user storage status",
			method:        http.MethodGet,
			request:       k8sRequest(http.MethodGet, "/api/v1/k8s/user-storage/status", ""),
			call:          getUserStorageStatus,
			wantOperation: "get_user_storage_status",
		},
		{
			name:          "stop user storage browse",
			method:        http.MethodDelete,
			request:       k8sRequest(http.MethodDelete, "/api/v1/k8s/user-storage/browse", ""),
			call:          stopUserStorageBrowse,
			wantOperation: "stop_user_storage_browse",
		},
		{
			name:          "project namespaces",
			method:        http.MethodGet,
			request:       k8sRequest(http.MethodGet, "/api/v1/projects/P2/namespaces", ""),
			call:          listProjectNamespaces,
			wantOperation: "list_project_namespaces",
			wantParams:    map[string]any{"project_id": "P2"},
		},
		{
			name:          "project resources",
			method:        http.MethodGet,
			request:       k8sRequest(http.MethodGet, "/api/v1/projects/P3/resources", ""),
			call:          listProjectResources,
			wantOperation: "list_project_resources",
			wantParams:    map[string]any{"project_id": "P3"},
		},
		{
			name:          "delete project user resources",
			method:        http.MethodDelete,
			request:       k8sRequest(http.MethodDelete, "/api/v1/projects/P4/resources/U1", ""),
			call:          deleteProjectUserResources,
			wantOperation: "delete_project_user_resources",
			wantParams:    map[string]any{"project_id": "P4", "user_id": "U1"},
		},
		{
			name:          "pod events",
			method:        http.MethodGet,
			request:       k8sRequest(http.MethodGet, "/api/v1/resources/ns/pods/pod/events", ""),
			call:          listPodEvents,
			wantOperation: "list_pod_events",
			wantParams:    map[string]any{"namespace": "ns", "pod": "pod"},
		},
		{
			name:          "operation id override",
			method:        http.MethodGet,
			request:       k8sRequest(http.MethodGet, "/api/v1/k8s/user-storage/status", ""),
			route:         platform.RouteSpec{OperationID: "custom_status"},
			call:          getUserStorageStatus,
			wantOperation: "custom_status",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assertK8sWrapperCase(t, app, adapter, tc)
		})
	}
}

type k8sWrapperCase struct {
	name          string
	method        string
	request       *http.Request
	route         platform.RouteSpec
	call          func(*platform.App, *http.Request, platform.RouteSpec) (int, any, *platform.Degraded)
	wantOperation string
	wantParams    map[string]any
}

func assertK8sWrapperCase(
	t *testing.T,
	app *platform.App,
	adapter *fakeK8sAdapter,
	tc k8sWrapperCase,
) {
	t.Helper()
	setK8sPathValues(tc.request)
	code, data, _ := tc.call(app, tc.request, tc.route)
	assertK8sStatus(t, code, data, http.StatusOK)
	if adapter.lastOperation != tc.wantOperation {
		t.Fatalf("operation = %q, want %q", adapter.lastOperation, tc.wantOperation)
	}
	assertK8sParams(t, data, tc.wantParams)
}

func assertK8sParams(t *testing.T, data any, wantParams map[string]any) {
	t.Helper()
	if wantParams == nil {
		return
	}
	got := data.(map[string]any)["params"].(map[string]any)
	for key, want := range wantParams {
		if got[key] != want {
			t.Fatalf("params[%s] = %#v, want %#v in %#v", key, got[key], want, got)
		}
	}
}

func TestK8sControlAdapterErrorAndDegraded(t *testing.T) {
	app := newK8sControlTestApp()
	app.Adapters[adapterName] = &fakeK8sAdapter{err: errors.New("upstream down")}
	code, data, _ := listAdminResources(app, k8sRequest(http.MethodGet, "/api/v1/admin/resources", ""), platform.RouteSpec{})
	assertK8sStatus(t, code, data, http.StatusBadGateway)
	if !strings.Contains(data.(map[string]any)["message"].(string), "upstream down") {
		t.Fatalf("error data = %#v, want upstream down", data)
	}

	app.Adapters[adapterName] = &fakeK8sAdapter{result: contracts.AdapterResult{Adapter: adapterName, Operation: "list", Degraded: true}}
	code, data, _ = listAdminResources(app, k8sRequest(http.MethodGet, "/api/v1/admin/resources", ""), platform.RouteSpec{})
	assertK8sStatus(t, code, data, http.StatusBadGateway)
	if !data.(contracts.AdapterResult).Degraded {
		t.Fatalf("degraded data = %#v, want degraded result", data)
	}
}

type fakeK8sAdapter struct {
	lastOperation  string
	lastIdempotent bool
	result         contracts.AdapterResult
	err            error
}

func (f *fakeK8sAdapter) Call(_ context.Context, operation string, idempotent bool) (contracts.AdapterResult, error) {
	f.lastOperation = operation
	f.lastIdempotent = idempotent
	if f.err != nil {
		return contracts.AdapterResult{}, f.err
	}
	if f.result.Operation != "" || f.result.Degraded {
		return f.result, nil
	}
	return contracts.AdapterResult{Adapter: adapterName, Operation: operation, Message: "ok"}, nil
}

func newK8sControlTestApp() *platform.App {
	app := platform.NewApp(platform.Config{ServiceName: "all", HTTPAddr: ":0"})
	Register(app)
	return app
}

func k8sRequest(method, target, body string) *http.Request {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	return req
}

func setK8sPathValues(req *http.Request) {
	for key, value := range map[string]string{
		"id":        "P1",
		"userId":    "U1",
		"namespace": "ns",
		"name":      "pod",
	} {
		req.SetPathValue(key, value)
	}
	if strings.Contains(req.URL.Path, "/P2/") {
		req.SetPathValue("id", "P2")
	}
	if strings.Contains(req.URL.Path, "/P3/") {
		req.SetPathValue("id", "P3")
	}
	if strings.Contains(req.URL.Path, "/P4/") {
		req.SetPathValue("id", "P4")
	}
}

func assertK8sStatus(t *testing.T, code int, data any, want int) {
	t.Helper()
	if code != want {
		t.Fatalf("status=%d data=%#v, want %d", code, data, want)
	}
}
