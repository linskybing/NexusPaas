package platform

import (
	"fmt"
	"net/http"
	"strings"
)

func (a *App) ValidateRouteCollisions() error {
	seen := map[string]RouteSpec{}
	var gaps []string
	for _, route := range a.CatalogRoutes {
		key := route.Method + " " + canonicalPattern(route.Pattern)
		first, exists := seen[key]
		if !exists {
			seen[key] = route
			continue
		}
		switch {
		case route.Override:
			if strings.TrimSpace(route.OverrideReason) == "" {
				gaps = append(gaps, key+" override missing reason")
			}
		case strings.TrimSpace(route.AliasOf) != "":
			if canonicalPattern(route.AliasOf) != canonicalPattern(first.Pattern) {
				gaps = append(gaps, key+" alias points to "+route.AliasOf)
			}
		default:
			gaps = append(gaps, key+" duplicates "+first.Pattern)
		}
	}
	if len(gaps) > 0 {
		return fmt.Errorf("route collisions: %s", strings.Join(gaps, ", "))
	}
	return nil
}

func (a *App) ValidateInternalRouteAuth() error {
	var gaps []string
	for _, route := range a.CatalogRoutes {
		if !internalRoute(route.Pattern) || route.InternalPublic || route.ServiceAuthRequired {
			continue
		}
		gaps = append(gaps, route.Method+" "+route.Pattern)
	}
	if len(gaps) > 0 {
		return fmt.Errorf("internal routes missing service auth: %s", strings.Join(gaps, ", "))
	}
	return nil
}

func (a *App) ValidateRouteSecurity() error {
	var gaps []string
	for _, route := range a.CatalogRoutes {
		key := route.Method + " " + route.Pattern
		if externalAPIRoute(route) && !route.AuthRequired && !route.ServiceAuthRequired && !publicAPIRouteAllowed(route) {
			gaps = append(gaps, key+" public without allowlist")
		}
		if externalAPIRoute(route) && route.PolicyBypass && !route.ServiceAuthRequired && !publicAPIRouteAllowed(route) {
			gaps = append(gaps, key+" policy bypass on user-facing route")
		}
		if route.Admin && !a.adminRouteProtected(route) {
			gaps = append(gaps, key+" admin route unprotected")
		}
	}
	if len(gaps) > 0 {
		return fmt.Errorf("route security gaps: %s", strings.Join(gaps, ", "))
	}
	return nil
}

func internalRoute(pattern string) bool {
	return strings.HasPrefix(pattern, "/internal/") || strings.HasPrefix(pattern, "/api/v1/internal/")
}

func externalAPIRoute(route RouteSpec) bool {
	return strings.HasPrefix(route.Pattern, "/api/v1/") && !strings.HasPrefix(route.Pattern, "/api/v1/internal/")
}

func publicAPIRouteAllowed(route RouteSpec) bool {
	_, ok := publicAPIRouteAllowlist()[route.Method+" "+route.Pattern]
	return ok
}

func publicAPIRouteAllowlist() map[string]string {
	return map[string]string{
		http.MethodPost + " /api/v1/login":                                "session login entry point",
		http.MethodPost + " /api/v1/logout":                               "session logout entry point",
		http.MethodPost + " /api/v1/register":                             "self-service account registration entry point",
		http.MethodPost + " /api/v1/refresh":                              "refresh-token exchange entry point",
		http.MethodGet + " /api/v1/captcha":                               "captcha challenge entry point",
		http.MethodPost + " /api/v1/cli/login":                            "CLI login bootstrap entry point",
		http.MethodGet + " /api/v1/oidc/start":                            "OIDC browser authorization start",
		http.MethodGet + " /api/v1/oidc/login":                            "OIDC login form entry point",
		http.MethodPost + " /api/v1/oidc/login":                           "OIDC login submission entry point",
		http.MethodGet + " /api/v1/oidc/.well-known/openid-configuration": "OIDC discovery metadata",
		http.MethodGet + " /api/v1/oidc/jwks":                             "OIDC public signing keys",
		http.MethodGet + " /api/v1/oidc/authorize":                        "OIDC authorization endpoint",
		http.MethodPost + " /api/v1/oidc/token":                           "OIDC token endpoint",
		http.MethodGet + " /api/v1/oidc/callback":                         "OIDC browser callback endpoint",
		http.MethodPost + " /api/v1/oidc/callback":                        "OIDC form-post callback endpoint",
	}
}

func (a *App) adminRouteProtected(route RouteSpec) bool {
	if a.CustomHandlers[route.Method+" "+canonicalPattern(route.Pattern)] != nil {
		return true
	}
	if route.ServiceAuthRequired {
		return true
	}
	return a.Config.RequireAuth && route.AuthRequired
}
