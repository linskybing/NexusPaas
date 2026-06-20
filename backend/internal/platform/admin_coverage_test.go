package platform

import (
	"net/http"
	"testing"
)

func adminRoute() RouteSpec {
	return RouteSpec{
		Method:        http.MethodPost,
		Pattern:       "/api/v1/admin/widget",
		Admin:         true,
		StateChanging: true,
		AuthRequired:  true,
	}
}

func TestValidateAdminCoverageFlagsUnprotected(t *testing.T) {
	app := NewApp(Config{ServiceName: "all"}) // RequireAuth false: platform gate inactive
	app.CatalogRoutes = append(app.CatalogRoutes, adminRoute())
	if err := app.ValidateAdminCoverage(); err == nil {
		t.Fatal("expected a gap: admin route with no custom handler and platform gate inactive")
	}
}

func TestValidateAdminCoverageCoveredByPlatformGate(t *testing.T) {
	app := NewApp(Config{ServiceName: "all", RequireAuth: true})
	app.CatalogRoutes = append(app.CatalogRoutes, adminRoute())
	if err := app.ValidateAdminCoverage(); err != nil {
		t.Fatalf("platform admin gate should cover the route: %v", err)
	}
}

func TestValidateAdminCoverageCoveredByCustomHandler(t *testing.T) {
	app := NewApp(Config{ServiceName: "all"}) // RequireAuth false
	app.CatalogRoutes = append(app.CatalogRoutes, adminRoute())
	app.RegisterCustomHandler(http.MethodPost, "/api/v1/admin/widget", func(_ *App, _ *http.Request, _ RouteSpec) (int, any, *Degraded) {
		return http.StatusOK, nil, nil
	})
	if err := app.ValidateAdminCoverage(); err != nil {
		t.Fatalf("custom handler should cover the route: %v", err)
	}
}

func TestValidateAdminCoverageCoveredByServiceAuth(t *testing.T) {
	app := NewApp(Config{ServiceName: "all"})
	route := adminRoute()
	route.Pattern = "/api/v1/internal/admin/widget"
	route.AuthRequired = false
	route.ServiceAuthRequired = true
	app.CatalogRoutes = append(app.CatalogRoutes, route)
	if err := app.ValidateAdminCoverage(); err != nil {
		t.Fatalf("service auth should cover internal admin route: %v", err)
	}
}
