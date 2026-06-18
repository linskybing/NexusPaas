package platform

import (
	"encoding/json"
	"fmt"
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
	seenOperationIDs := map[string]int{}
	for _, route := range g.routes {
		path, catchAll := openAPIPath(route.Pattern)
		if paths[path] == nil {
			paths[path] = map[string]any{}
		}
		operationID := uniqueOpenAPIOperationID(route, seenOperationIDs)
		operation := map[string]any{
			"tags":            []string{routeServiceName(route)},
			"summary":         routeSummary(route),
			"operationId":     operationID,
			"parameters":      pathParameters(route.Pattern),
			"responses":       routeResponses(),
			"x-service":       routeServiceName(route),
			"x-auth":          route.AuthRequired,
			"x-admin":         route.Admin,
			"x-stateful":      route.StateChanging,
			"x-policy-bypass": route.PolicyBypass,
			"x-resource":      route.Resource,
			"x-adapter":       route.ExternalAdapter,
			"x-idempotent":    route.Method == http.MethodGet || route.Action == "quota_commit" || route.Action == "quota_release",
		}
		if catchAll {
			operation["x-runtime-pattern"] = route.Pattern
			operation["x-catch-all"] = true
		}
		if route.AuthRequired {
			operation["security"] = []map[string][]string{
				{"BearerAuth": {}},
				{"ApiKeyAuth": {}},
			}
		}
		paths[path][strings.ToLower(route.Method)] = operation
	}
	return map[string]any{
		"openapi": "3.1.0",
		"info": map[string]any{
			"title":   "NexusPaas Microservices",
			"version": "0.1.0",
		},
		"paths":      paths,
		"components": openAPIComponents(),
	}
}

// OpenAPI returns the runtime OpenAPI document for the registered routes.
func (a *App) OpenAPI() map[string]any {
	return openAPIGenerator{routes: a.Routes}.generate()
}

func (a *App) SwaggerHTML() RawResponse {
	spec, err := json.Marshal(a.OpenAPI())
	if err != nil {
		spec = []byte(`{"openapi":"3.1.0","info":{"title":"NexusPaas Microservices","version":"0.1.0"},"paths":{}}`)
	}
	html := fmt.Sprintf(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>NexusPaaS API Docs</title>
  <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css">
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
  <script>
    window.addEventListener("load", function () {
      SwaggerUIBundle({
        spec: %s,
        dom_id: "#swagger-ui",
        deepLinking: true,
        presets: [SwaggerUIBundle.presets.apis],
        layout: "BaseLayout"
      });
    });
  </script>
</body>
</html>`, string(spec))
	return RawResponse{
		ContentType: "text/html; charset=utf-8",
		Body:        []byte(html),
	}
}

func openAPIPath(pattern string) (string, bool) {
	segments := strings.Split(pattern, "/")
	catchAll := false
	for i, segment := range segments {
		if !strings.HasPrefix(segment, "{") || !strings.HasSuffix(segment, "}") {
			continue
		}
		name := strings.TrimSuffix(strings.TrimPrefix(strings.TrimSuffix(segment, "}"), "{"), "...")
		if name == "" || name == segment {
			continue
		}
		if strings.HasSuffix(strings.TrimSuffix(segment, "}"), "...") {
			catchAll = true
			segments[i] = "{" + name + "}"
		}
	}
	return strings.Join(segments, "/"), catchAll
}

func pathParameters(pattern string) []map[string]any {
	var params []map[string]any
	for _, segment := range strings.Split(pattern, "/") {
		if !strings.HasPrefix(segment, "{") || !strings.HasSuffix(segment, "}") {
			continue
		}
		raw := strings.TrimSuffix(strings.TrimPrefix(segment, "{"), "}")
		catchAll := strings.HasSuffix(raw, "...")
		name := strings.TrimSuffix(raw, "...")
		if name == "" {
			continue
		}
		param := map[string]any{
			"name":        name,
			"in":          "path",
			"required":    true,
			"description": "Path parameter from the runtime route template.",
			"schema": map[string]any{
				"type": "string",
			},
		}
		if catchAll {
			param["x-catch-all"] = true
		}
		params = append(params, param)
	}
	return params
}

func routeResponses() map[string]any {
	return map[string]any{
		"2XX": map[string]any{
			"description": "Successful response wrapped in the platform response envelope.",
			"content": map[string]any{
				contentTypeJSON: map[string]any{
					"schema": map[string]any{
						"$ref": "#/components/schemas/Envelope",
					},
				},
			},
		},
		"default": map[string]any{
			"description": "Error response wrapped in the platform response envelope.",
			"content": map[string]any{
				contentTypeJSON: map[string]any{
					"schema": map[string]any{
						"$ref": "#/components/schemas/Envelope",
					},
				},
			},
		},
	}
}

func openAPIComponents() map[string]any {
	return map[string]any{
		"securitySchemes": map[string]any{
			"BearerAuth": map[string]any{
				"type":         "http",
				"scheme":       "bearer",
				"bearerFormat": "JWT",
			},
			"ApiKeyAuth": map[string]any{
				"type": "apiKey",
				"in":   "header",
				"name": "X-API-Key",
			},
		},
		"schemas": map[string]any{
			"Envelope": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"success": map[string]any{"type": "boolean"},
					"data":    map[string]any{"$ref": "#/components/schemas/GenericData"},
					"error":   map[string]any{"$ref": "#/components/schemas/ErrorBody"},
					"degraded": map[string]any{
						"$ref": "#/components/schemas/Degraded",
					},
					"request_id": map[string]any{"type": "string"},
					"trace_id":   map[string]any{"type": "string"},
				},
				"required": []string{"success", "request_id", "trace_id"},
			},
			"ErrorBody": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"code":    map[string]any{"type": "string"},
					"message": map[string]any{"type": "string"},
				},
				"required": []string{"code", "message"},
			},
			"Degraded": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"adapter":   map[string]any{"type": "string"},
					"code":      map[string]any{"type": "string"},
					"message":   map[string]any{"type": "string"},
					"retryable": map[string]any{"type": "boolean"},
				},
				"required": []string{"adapter", "code", "message", "retryable"},
			},
			"GenericData": map[string]any{
				"description": "Route-specific response payload. This route-level contract intentionally leaves domain payloads generic.",
			},
		},
	}
}

func routeServiceName(route RouteSpec) string {
	if before, _, ok := strings.Cut(route.Resource, ":"); ok && before != "" {
		return before
	}
	return route.Resource
}

func routeSummary(route RouteSpec) string {
	return route.Method + " " + route.Pattern
}

func uniqueOpenAPIOperationID(route RouteSpec, seen map[string]int) string {
	id := strings.TrimSpace(route.OperationID)
	if id == "" {
		id = operationID(routeServiceName(route), route.Method, route.Pattern)
	}
	seen[id]++
	if seen[id] == 1 {
		return id
	}
	return fmt.Sprintf("%s_%d", id, seen[id])
}
