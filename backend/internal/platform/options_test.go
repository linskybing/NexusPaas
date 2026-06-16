package platform

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
)

type fakeAdapter struct{ calls int }

func (f *fakeAdapter) Call(_ context.Context, _ string, _ bool) (contracts.AdapterResult, error) {
	f.calls++
	return contracts.AdapterResult{Code: "ok"}, nil
}

func TestOptionsOverrideDefaults(t *testing.T) {
	metrics := NewMetrics()
	limiter := NewRateLimiter(7, time.Minute)
	switches := NewRouteSwitches()
	adapter := &fakeAdapter{}
	checker := &recordingBackingChecker{}

	app := NewApp(Config{ServiceName: "all", HTTPAddr: ":0"},
		WithMetrics(metrics),
		WithRateLimiter(limiter),
		WithSwitches(switches),
		WithAdapters(map[string]contracts.ExternalAdapter{"harbor": adapter}),
		WithBackingChecker(checker),
	)

	if app.Metrics != metrics {
		t.Error("WithMetrics did not override default")
	}
	if app.Rate != limiter {
		t.Error("WithRateLimiter did not override default")
	}
	if app.Switches != switches {
		t.Error("WithSwitches did not override default")
	}
	if app.Adapters["harbor"] != adapter {
		t.Error("WithAdapters did not override injected adapter")
	}
	if app.BackingChecker != checker {
		t.Error("WithBackingChecker did not override default")
	}
	// Well-known adapters not supplied still get a default (no-op) instance.
	if app.Adapters["k8s"] == nil {
		t.Error("default adapter for k8s should still be present")
	}
}

func TestRegisterActionDispatch(t *testing.T) {
	app := NewApp(Config{ServiceName: "all", HTTPAddr: ":0"})
	called := false
	app.RegisterAction("custom_action", func(_ *httpRequest, _ RouteSpec) (int, any) {
		called = true
		return http.StatusTeapot, map[string]any{"ok": true}
	})

	req := &httpRequest{Request: httptest.NewRequest(http.MethodGet, "/x", nil), Service: "all", TraceID: "t"}
	status, _, _ := app.handleRoute(req, RouteSpec{Method: http.MethodGet, Pattern: "/x", Action: "custom_action"})

	if !called {
		t.Fatal("custom action was not dispatched")
	}
	if status != http.StatusTeapot {
		t.Fatalf("status=%d want %d", status, http.StatusTeapot)
	}
}

func TestUnknownActionFallsBackToCRUD(t *testing.T) {
	app := NewApp(Config{ServiceName: "all", HTTPAddr: ":0"})
	// An empty/unknown action must route to the CRUD default without panicking.
	req := &httpRequest{Request: httptest.NewRequest(http.MethodGet, "/api/v1/widgets", nil), Service: "all", TraceID: "t"}
	status, _, _ := app.handleRoute(req, RouteSpec{Method: http.MethodGet, Pattern: "/api/v1/widgets", Resource: "widget-service:widgets", Action: ""})
	if status == 0 {
		t.Fatal("expected CRUD fallback to produce a status")
	}
}
