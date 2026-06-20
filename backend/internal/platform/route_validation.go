package platform

import (
	"fmt"
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

func internalRoute(pattern string) bool {
	return strings.HasPrefix(pattern, "/internal/") || strings.HasPrefix(pattern, "/api/v1/internal/")
}
