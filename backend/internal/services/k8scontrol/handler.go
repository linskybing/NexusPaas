package k8scontrol

import (
	"net/http"
	"strings"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

const (
	serviceName = "k8s-control-service"
	adapterName = "k8s"
)

func Register(app *platform.App) {
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/admin/resources", listAdminResources)
	app.RegisterCustomHandler(http.MethodDelete, "/api/v1/admin/resources/projects/{id}", deleteAdminProjectResources)
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/k8s/namespaces/{ns}/pods/{name}/logs", getPodLogs)
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/k8s/user-storage/status", getUserStorageStatus)
	app.RegisterCustomHandler(http.MethodPost, "/api/v1/k8s/user-storage/browse", startUserStorageBrowse)
	app.RegisterCustomHandler(http.MethodDelete, "/api/v1/k8s/user-storage/browse", stopUserStorageBrowse)
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/projects/{id}/namespaces", listProjectNamespaces)
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/projects/{id}/resources", listProjectResources)
	app.RegisterCustomHandler(http.MethodDelete, "/api/v1/projects/{id}/resources/{userId}", deleteProjectUserResources)
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/resources/{namespace}/pods/{name}/events", listPodEvents)
	app.RegisterCustomHandler(http.MethodDelete, "/api/v1/resources/{namespace}/{kind}/{name}", deleteResource)
	app.RegisterCustomHandler(http.MethodPost, "/internal/k8s-control/fast-transfers/mover-jobs", createFastTransferMoverJob)
	registerDockerCleanup(app)
}

func listAdminResources(app *platform.App, r *http.Request, route platform.RouteSpec) (int, any, *platform.Degraded) {
	return callK8s(app, r, route, "list_admin_resources", nil)
}

func deleteAdminProjectResources(app *platform.App, r *http.Request, route platform.RouteSpec) (int, any, *platform.Degraded) {
	return callK8s(app, r, route, "delete_admin_project_resources", map[string]any{"project_id": pathValue(r, "id")})
}

func getPodLogs(app *platform.App, r *http.Request, route platform.RouteSpec) (int, any, *platform.Degraded) {
	return callK8s(app, r, route, "get_pod_logs", map[string]any{"namespace": pathValue(r, "ns"), "pod": pathValue(r, "name")})
}

func getUserStorageStatus(app *platform.App, r *http.Request, route platform.RouteSpec) (int, any, *platform.Degraded) {
	return callK8s(app, r, route, "get_user_storage_status", nil)
}

func startUserStorageBrowse(app *platform.App, r *http.Request, route platform.RouteSpec) (int, any, *platform.Degraded) {
	payload, err := platform.DecodeMapWithError(r)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData("invalid request body"), nil
	}
	return callK8s(app, r, route, "start_user_storage_browse", payload)
}

func stopUserStorageBrowse(app *platform.App, r *http.Request, route platform.RouteSpec) (int, any, *platform.Degraded) {
	return callK8s(app, r, route, "stop_user_storage_browse", nil)
}

func listProjectNamespaces(app *platform.App, r *http.Request, route platform.RouteSpec) (int, any, *platform.Degraded) {
	return callK8s(app, r, route, "list_project_namespaces", map[string]any{"project_id": pathValue(r, "id")})
}

func listProjectResources(app *platform.App, r *http.Request, route platform.RouteSpec) (int, any, *platform.Degraded) {
	return callK8s(app, r, route, "list_project_resources", map[string]any{"project_id": pathValue(r, "id")})
}

func deleteProjectUserResources(app *platform.App, r *http.Request, route platform.RouteSpec) (int, any, *platform.Degraded) {
	return callK8s(app, r, route, "delete_project_user_resources", map[string]any{"project_id": pathValue(r, "id"), "user_id": pathValue(r, "userId")})
}

func listPodEvents(app *platform.App, r *http.Request, route platform.RouteSpec) (int, any, *platform.Degraded) {
	return callK8s(app, r, route, "list_pod_events", map[string]any{"namespace": pathValue(r, "namespace"), "pod": pathValue(r, "name")})
}

func deleteResource(app *platform.App, r *http.Request, route platform.RouteSpec) (int, any, *platform.Degraded) {
	params := map[string]any{"namespace": pathValue(r, "namespace"), "kind": pathValue(r, "kind"), "name": pathValue(r, "name")}
	return callK8s(app, r, route, "delete_resource", params)
}

func callK8s(app *platform.App, r *http.Request, route platform.RouteSpec, operation string, params map[string]any) (int, any, *platform.Degraded) {
	adapter := app.Adapters[adapterName]
	if adapter == nil {
		return http.StatusBadGateway, shared.ErrorData("k8s adapter is not configured"), nil
	}
	result, err := adapter.Call(r.Context(), operationID(route, operation), r.Method == http.MethodGet)
	if err != nil {
		return http.StatusBadGateway, map[string]any{"adapter": adapterName, "operation": operation, "message": err.Error()}, nil
	}
	if result.Degraded {
		return http.StatusBadGateway, result, nil
	}
	return http.StatusOK, map[string]any{"adapter": adapterName, "operation": operation, "params": params, "result": result}, nil
}

func operationID(route platform.RouteSpec, fallback string) string {
	if strings.TrimSpace(route.OperationID) != "" {
		return route.OperationID
	}
	return fallback
}

func pathValue(r *http.Request, name string) string {
	return strings.TrimSpace(r.PathValue(name))
}
