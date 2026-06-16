package platform

import (
	"net/http"
	"testing"
)

func TestOpenAPIGeneratorGenerate(t *testing.T) {
	gen := openAPIGenerator{routes: []RouteSpec{
		{Method: http.MethodGet, Pattern: "/api/v1/widgets", OperationID: "list_widgets", Resource: "widget-service:widgets", AuthRequired: true},
		{Method: http.MethodPost, Pattern: "/api/v1/widgets", OperationID: "create_widget", Resource: "widget-service:widgets", AuthRequired: true, Admin: true, StateChanging: true},
	}}

	doc := gen.generate()
	if doc["openapi"] != "3.1.0" {
		t.Fatalf("openapi version = %v, want 3.1.0", doc["openapi"])
	}
	paths, ok := doc["paths"].(map[string]map[string]any)
	if !ok {
		t.Fatalf("paths type = %T, want map[string]map[string]any", doc["paths"])
	}
	widgets, ok := paths["/api/v1/widgets"]
	if !ok {
		t.Fatalf("missing /api/v1/widgets path: %#v", paths)
	}
	get, ok := widgets["get"].(map[string]any)
	if !ok {
		t.Fatalf("missing get operation: %#v", widgets)
	}
	if get["operationId"] != "list_widgets" {
		t.Errorf("get operationId = %v, want list_widgets", get["operationId"])
	}
	if get["x-service"] != "widget-service" {
		t.Errorf("get x-service = %v, want widget-service", get["x-service"])
	}
	if get["x-idempotent"] != true {
		t.Errorf("GET x-idempotent = %v, want true", get["x-idempotent"])
	}
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
}
