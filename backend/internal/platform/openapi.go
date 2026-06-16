package platform

import (
	"net/http"
	"strings"
)

// openAPIGenerator builds an OpenAPI 3.1 document from a route table. It is a
// pure function of the routes it is given, so it can be unit-tested in isolation
// from the rest of the platform runtime (finding 9: extract OpenAPI generation
// out of the App god object).
type openAPIGenerator struct {
	routes []RouteSpec
}

func (g openAPIGenerator) generate() map[string]any {
	paths := map[string]map[string]any{}
	for _, route := range g.routes {
		if paths[route.Pattern] == nil {
			paths[route.Pattern] = map[string]any{}
		}
		paths[route.Pattern][strings.ToLower(route.Method)] = map[string]any{
			"operationId":     route.OperationID,
			"x-service":       strings.Split(route.Resource, ":")[0],
			"x-auth":          route.AuthRequired,
			"x-admin":         route.Admin,
			"x-stateful":      route.StateChanging,
			"x-policy-bypass": route.PolicyBypass,
			"x-resource":      route.Resource,
			"x-adapter":       route.ExternalAdapter,
			"x-idempotent":    route.Method == http.MethodGet || route.Action == "quota_commit" || route.Action == "quota_release",
		}
	}
	return map[string]any{
		"openapi": "3.1.0",
		"info": map[string]any{
			"title":   "NexusPaas Microservices",
			"version": "0.1.0",
		},
		"paths": paths,
	}
}

// OpenAPI returns the runtime OpenAPI document for the registered routes.
func (a *App) OpenAPI() map[string]any {
	return openAPIGenerator{routes: a.Routes}.generate()
}
