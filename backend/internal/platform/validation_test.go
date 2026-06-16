package platform

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func postWidget(app *App, body string) (int, any) {
	req := &httpRequest{
		Request: httptest.NewRequest(http.MethodPost, "/api/v1/widgets", strings.NewReader(body)),
		Service: "all",
		TraceID: "t",
	}
	status, data, _ := app.handleRoute(req, RouteSpec{Method: http.MethodPost, Pattern: "/api/v1/widgets", Resource: "widget-service:widgets", Action: ""})
	return status, data
}

func TestRequiredFieldsRejectsMissing(t *testing.T) {
	app := NewApp(Config{ServiceName: "all", HTTPAddr: ":0"})
	app.RegisterRequiredFields("widget-service:widgets", "name")

	status, data := postWidget(app, `{}`)
	if status != http.StatusBadRequest {
		t.Fatalf("missing field status=%d want 400", status)
	}
	if msg, _ := data.(map[string]any)["message"].(string); !strings.Contains(msg, "name") {
		t.Fatalf("error message %q should name the missing field", msg)
	}

	// Whitespace-only string also counts as missing.
	if status, _ := postWidget(app, `{"name":"   "}`); status != http.StatusBadRequest {
		t.Fatalf("whitespace field status=%d want 400", status)
	}
}

func TestRequiredFieldsAllowsPresent(t *testing.T) {
	app := NewApp(Config{ServiceName: "all", HTTPAddr: ":0"})
	app.RegisterRequiredFields("widget-service:widgets", "name")

	status, _ := postWidget(app, `{"name":"gadget"}`)
	if status != http.StatusCreated {
		t.Fatalf("valid create status=%d want 201", status)
	}
}

func TestNoRequiredFieldsAcceptsAnyShape(t *testing.T) {
	app := NewApp(Config{ServiceName: "all", HTTPAddr: ":0"})
	// Unregistered resource: arbitrary valid JSON object still accepted.
	if status, _ := postWidget(app, `{"anything":1}`); status != http.StatusCreated {
		t.Fatalf("unregistered resource create status=%d want 201", status)
	}
}
