package platform

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func writeGadget(app *App, method, body string) (int, any) {
	req := &httpRequest{
		Request: httptest.NewRequest(method, "/api/v1/gadgets", strings.NewReader(body)),
		Service: "all",
		TraceID: "t",
	}
	status, data, _ := app.handleRoute(req, RouteSpec{Method: method, Pattern: "/api/v1/gadgets", Resource: "gadget-service:gadgets", Action: ""})
	return status, data
}

func newGadgetApp(t *testing.T) *App {
	t.Helper()
	app := NewApp(Config{ServiceName: "all", HTTPAddr: ":0"})
	app.RegisterFieldSchema("gadget-service:gadgets", map[string]FieldType{
		"name":    FieldString,
		"count":   FieldNumber,
		"enabled": FieldBool,
	})
	return app
}

func TestFieldSchemaAcceptsCorrectTypes(t *testing.T) {
	app := newGadgetApp(t)
	if status, _ := writeGadget(app, http.MethodPost, `{"name":"a","count":3,"enabled":true}`); status != http.StatusCreated {
		t.Fatalf("valid create status=%d want 201", status)
	}
}

func TestFieldSchemaRejectsWrongType(t *testing.T) {
	app := newGadgetApp(t)
	cases := []string{
		`{"name":3}`,
		`{"count":"three"}`,
		`{"enabled":"yes"}`,
	}
	for _, body := range cases {
		status, data := writeGadget(app, http.MethodPost, body)
		if status != http.StatusBadRequest {
			t.Fatalf("body %s status=%d want 400", body, status)
		}
		if msg, _ := data.(map[string]any)["message"].(string); !strings.Contains(msg, "invalid field type") {
			t.Fatalf("body %s message=%q want invalid field type", body, msg)
		}
	}
}

func TestFieldSchemaIgnoresAbsentAndUnregisteredFields(t *testing.T) {
	app := newGadgetApp(t)
	// "extra" is not registered; absent registered fields are fine.
	if status, _ := writeGadget(app, http.MethodPost, `{"name":"a","extra":[1,2,3]}`); status != http.StatusCreated {
		t.Fatalf("partial+extra create status=%d want 201", status)
	}
}

func TestFieldSchemaValidatesUpdatePath(t *testing.T) {
	app := newGadgetApp(t)
	if status, _ := writeGadget(app, http.MethodPut, `{"id":"g1","count":"nope"}`); status != http.StatusBadRequest {
		t.Fatalf("update wrong-type status=%d want 400", status)
	}
	if status, _ := writeGadget(app, http.MethodPut, `{"id":"g1","count":5}`); status != http.StatusOK {
		t.Fatalf("update correct-type status=%d want 200", status)
	}
}
