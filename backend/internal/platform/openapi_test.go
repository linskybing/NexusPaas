package platform

import (
	"net/http"
	"testing"
)

func TestOpenAPIGeneratorGenerate(t *testing.T) {
	gen := openAPIGenerator{routes: []RouteSpec{
		{Method: http.MethodGet, Pattern: "/api/v1/widgets/{id}", OperationID: "get_widget", Resource: "widget-service:widgets", IDParam: "id", AuthRequired: true},
		{Method: http.MethodPost, Pattern: "/api/v1/widgets/{id}", OperationID: "create_widget", Resource: "widget-service:widgets", AuthRequired: true, Admin: true, StateChanging: true},
		{Method: http.MethodGet, Pattern: "/api/v1/public", OperationID: "public_probe", Resource: "widget-service:public"},
		{Method: http.MethodPost, Pattern: "/api/v1/public", OperationID: "public_probe", Resource: "widget-service:public"},
		{Method: http.MethodGet, Pattern: "/api/v1/proxy/{path...}", OperationID: "proxy_widget", Resource: "widget-service:proxy", AuthRequired: true, Action: "proxy"},
	}}

	doc := gen.generate()
	if doc["openapi"] != "3.1.0" {
		t.Fatalf("openapi version = %v, want 3.1.0", doc["openapi"])
	}
	paths, ok := doc["paths"].(map[string]map[string]any)
	if !ok {
		t.Fatalf("paths type = %T, want map[string]map[string]any", doc["paths"])
	}
	assertWidgetPath(t, paths)
	assertPublicPath(t, paths)
	assertProxyPath(t, paths)
	assertComponents(t, doc)
}

func assertWidgetPath(t *testing.T, paths map[string]map[string]any) {
	t.Helper()
	widgets, ok := paths["/api/v1/widgets/{id}"]
	if !ok {
		t.Fatalf("missing /api/v1/widgets path: %#v", paths)
	}
	get, ok := widgets["get"].(map[string]any)
	if !ok {
		t.Fatalf("missing get operation: %#v", widgets)
	}
	if get["operationId"] != "get_widget" {
		t.Errorf("get operationId = %v, want get_widget", get["operationId"])
	}
	if get["x-service"] != "widget-service" {
		t.Errorf("get x-service = %v, want widget-service", get["x-service"])
	}
	if get["x-idempotent"] != true {
		t.Errorf("GET x-idempotent = %v, want true", get["x-idempotent"])
	}
	assertOperationResponses(t, get)
	assertOperationSecurity(t, get)
	assertPathParameter(t, get, "id", false)
	post, ok := widgets["post"].(map[string]any)
	if !ok {
		t.Fatalf("missing post operation: %#v", widgets)
	}
	if post["x-admin"] != true || post["x-stateful"] != true {
		t.Errorf("post x-admin/x-stateful = %v/%v, want true/true", post["x-admin"], post["x-stateful"])
	}
	if post["x-idempotent"] != false {
		t.Errorf("POST x-idempotent = %v, want false", post["x-idempotent"])
	}
	assertOperationResponses(t, post)
	assertOperationSecurity(t, post)
}

func assertPublicPath(t *testing.T, paths map[string]map[string]any) {
	t.Helper()
	publicPath, ok := paths["/api/v1/public"]
	if !ok {
		t.Fatalf("missing public path: %#v", paths)
	}
	publicGet := publicPath["get"].(map[string]any)
	if _, ok := publicGet["security"]; ok {
		t.Fatalf("public route unexpectedly declares security: %#v", publicGet["security"])
	}
	publicPost := publicPath["post"].(map[string]any)
	if publicPost["operationId"] != "public_probe_2" {
		t.Fatalf("duplicate operationId = %v, want public_probe_2", publicPost["operationId"])
	}
}

func assertProxyPath(t *testing.T, paths map[string]map[string]any) {
	t.Helper()
	proxyPath, ok := paths["/api/v1/proxy/{path}"]
	if !ok {
		t.Fatalf("missing Swagger-safe catch-all path: %#v", paths)
	}
	proxyGet := proxyPath["get"].(map[string]any)
	if proxyGet["x-runtime-pattern"] != "/api/v1/proxy/{path...}" {
		t.Fatalf("x-runtime-pattern = %v, want runtime catch-all pattern", proxyGet["x-runtime-pattern"])
	}
	if proxyGet["x-catch-all"] != true {
		t.Fatalf("x-catch-all = %v, want true", proxyGet["x-catch-all"])
	}
	assertPathParameter(t, proxyGet, "path", true)
}

func assertOperationResponses(t *testing.T, operation map[string]any) {
	t.Helper()
	responses, ok := operation["responses"].(map[string]any)
	if !ok {
		t.Fatalf("responses type = %T, want map[string]any", operation["responses"])
	}
	if _, ok := responses["2XX"]; !ok {
		t.Fatalf("missing 2XX response: %#v", responses)
	}
	if _, ok := responses["default"]; !ok {
		t.Fatalf("missing default response: %#v", responses)
	}
}

func assertOperationSecurity(t *testing.T, operation map[string]any) {
	t.Helper()
	security, ok := operation["security"].([]map[string][]string)
	if !ok {
		t.Fatalf("security type = %T, want []map[string][]string", operation["security"])
	}
	if len(security) != 2 {
		t.Fatalf("security entries = %d, want 2", len(security))
	}
	if _, ok := security[0]["BearerAuth"]; !ok {
		t.Fatalf("missing BearerAuth security: %#v", security)
	}
	if _, ok := security[1]["ApiKeyAuth"]; !ok {
		t.Fatalf("missing ApiKeyAuth security: %#v", security)
	}
}

func assertPathParameter(t *testing.T, operation map[string]any, name string, catchAll bool) {
	t.Helper()
	params, ok := operation["parameters"].([]map[string]any)
	if !ok {
		t.Fatalf("parameters type = %T, want []map[string]any", operation["parameters"])
	}
	for _, param := range params {
		if param["name"] != name {
			continue
		}
		if param["in"] != "path" || param["required"] != true {
			t.Fatalf("parameter %s = %#v, want required path parameter", name, param)
		}
		if got := param["x-catch-all"]; got != nil && got != catchAll {
			t.Fatalf("parameter %s x-catch-all = %v, want %v", name, got, catchAll)
		}
		if catchAll && param["x-catch-all"] != true {
			t.Fatalf("parameter %s x-catch-all = %v, want true", name, param["x-catch-all"])
		}
		return
	}
	t.Fatalf("missing path parameter %q in %#v", name, params)
}

func assertComponents(t *testing.T, doc map[string]any) {
	t.Helper()
	components, ok := doc["components"].(map[string]any)
	if !ok {
		t.Fatalf("components type = %T, want map[string]any", doc["components"])
	}
	schemas, ok := components["schemas"].(map[string]any)
	if !ok {
		t.Fatalf("schemas type = %T, want map[string]any", components["schemas"])
	}
	for _, name := range []string{"Envelope", "ErrorBody", "Degraded", "GenericData"} {
		if _, ok := schemas[name]; !ok {
			t.Fatalf("missing schema %s in %#v", name, schemas)
		}
	}
	securitySchemes, ok := components["securitySchemes"].(map[string]any)
	if !ok {
		t.Fatalf("securitySchemes type = %T, want map[string]any", components["securitySchemes"])
	}
	for _, name := range []string{"BearerAuth", "ApiKeyAuth"} {
		if _, ok := securitySchemes[name]; !ok {
			t.Fatalf("missing security scheme %s in %#v", name, securitySchemes)
		}
	}
}
