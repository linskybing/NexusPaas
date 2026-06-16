package authorizationpolicy

import (
	"net/http"
	"strings"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

func listServices(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	if status, data, ok := requireAdmin(app, r); !ok {
		return status, data, nil
	}
	return http.StatusOK, serviceRows(app, r), nil
}

func getService(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	if status, data, ok := requireAdmin(app, r); !ok {
		return status, data, nil
	}
	service, found := findService(app, r, strings.TrimSpace(r.PathValue("id")))
	if !found {
		return http.StatusNotFound, shared.ErrorData("service not found"), nil
	}
	return http.StatusOK, service, nil
}

func createService(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	if status, data, ok := requireAdmin(app, r); !ok {
		return status, data, nil
	}
	return http.StatusMethodNotAllowed, shared.ErrorData("proxy service definitions are managed by deployment configuration"), nil
}
