package services

import (
	"strings"
	"testing"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

// TestCatalogResourceOwnership asserts that every resource key referenced by a
// route resolves to a registered service (the "<service>:<resource>" prefix is
// a known service name). This is the contract test called for by findings 22
// and 27: it catches split-brain / misrouted resource keys where a route points
// at a service that does not exist, before they reach production.
func TestCatalogResourceOwnership(t *testing.T) {
	app := newTestApp()

	owners := map[string]bool{}
	for name := range app.Services {
		owners[name] = true
	}

	checked := 0
	for _, route := range app.CatalogRoutes {
		resource := route.Resource
		if resource == "" || !strings.Contains(resource, ":") {
			continue // action-only routes (proxy/command) carry no owned resource
		}
		owner := resource[:strings.Index(resource, ":")]
		checked++
		if !owners[owner] {
			t.Errorf("route %s %s references resource %q whose owner %q is not a registered service",
				route.Method, route.Pattern, resource, owner)
		}
	}
	if checked == 0 {
		t.Fatal("no resource-bearing routes were checked; ownership test is not exercising the catalog")
	}
}

// TestRegisterAllAdminCoverageInProduction proves the full registered catalog
// passes the production admin-coverage startup check (finding 26): every
// state-changing admin route is covered by a custom handler or the platform
// admin gate when RequireAuth is on.
func TestRegisterAllAdminCoverageInProduction(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: "all", HTTPAddr: ":0", RequireAuth: true})
	RegisterAll(app)
	if err := app.ValidateAdminCoverage(); err != nil {
		t.Fatalf("production admin-coverage gaps in registered catalog: %v", err)
	}
}

// TestCatalogStateChangingRoutesHaveResourceOrAction guards against routes that
// mutate state but neither name a resource nor an action, which would silently
// fall through generic CRUD with an empty resource key.
func TestCatalogStateChangingRoutesHaveResourceOrAction(t *testing.T) {
	app := newTestApp()
	for _, route := range app.CatalogRoutes {
		if !route.StateChanging {
			continue
		}
		if route.Resource == "" && route.Action == "" {
			// A custom handler may still own it; only flag if nothing handles it.
			if !hasCustomHandler(app, route) {
				t.Errorf("state-changing route %s %s has no resource, action, or custom handler",
					route.Method, route.Pattern)
			}
		}
	}
}

func hasCustomHandler(app *platform.App, route platform.RouteSpec) bool {
	_, ok := app.CustomHandlers[route.Method+" "+canonicalPatternForTest(route.Pattern)]
	return ok
}

// canonicalPatternForTest mirrors the platform's pattern canonicalization for
// the subset used by registered routes (already canonical in the catalog).
func canonicalPatternForTest(pattern string) string { return pattern }
