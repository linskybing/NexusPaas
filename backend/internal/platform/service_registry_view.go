package platform

import "sort"

// ServiceRegistryEntry is the public, minimal view of a registered service exposed
// by GET /service-registry. It deliberately omits internal RouteSpec fields
// (resource names, action types, operation ids, adapter bindings, auth/admin flags,
// event names, table names) so the operational endpoint does not leak the internal
// wiring of every service to verified admins (finding 13).
type ServiceRegistryEntry struct {
	Name     string                 `json:"name"`
	Category string                 `json:"category"`
	Phase    string                 `json:"phase"`
	Routes   []ServiceRegistryRoute `json:"routes"`
}

// ServiceRegistryRoute exposes only the HTTP surface (method + pattern) of a route.
type ServiceRegistryRoute struct {
	Method  string `json:"method"`
	Pattern string `json:"pattern"`
}

// ServiceRegistryView returns the minimal, sorted registry view served by the
// /service-registry operational endpoint.
func (a *App) ServiceRegistryView() []ServiceRegistryEntry {
	entries := make([]ServiceRegistryEntry, 0, len(a.Services))
	for _, spec := range a.Services {
		routes := make([]ServiceRegistryRoute, 0, len(spec.Routes))
		for _, route := range spec.Routes {
			routes = append(routes, ServiceRegistryRoute{Method: route.Method, Pattern: route.Pattern})
		}
		sort.Slice(routes, func(i, j int) bool {
			if routes[i].Pattern != routes[j].Pattern {
				return routes[i].Pattern < routes[j].Pattern
			}
			return routes[i].Method < routes[j].Method
		})
		entries = append(entries, ServiceRegistryEntry{
			Name:     spec.Name,
			Category: spec.Category,
			Phase:    spec.Phase,
			Routes:   routes,
		})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	return entries
}
