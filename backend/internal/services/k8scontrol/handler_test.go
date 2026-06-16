package k8scontrol

import (
	"context"
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

type fakeK8sAdapter struct {
	lastOperation  string
	lastIdempotent bool
}

func (f *fakeK8sAdapter) Call(_ context.Context, operation string, idempotent bool) (contracts.AdapterResult, error) {
	f.lastOperation = operation
	f.lastIdempotent = idempotent
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

func assertK8sStatus(t *testing.T, code int, data any, want int) {
	t.Helper()
	if code != want {
		t.Fatalf("status=%d data=%#v, want %d", code, data, want)
	}
}
