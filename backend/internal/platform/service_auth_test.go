package platform

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestServiceAuthRequiredRouteFailsClosedWithoutConfiguredKey(t *testing.T) {
	app := NewApp(Config{ServiceName: "all", HTTPAddr: ":0"})
	registerServiceAuthRoute(app)

	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/internal/owner/o-1", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", rec.Code, rec.Body.String())
	}
}

func TestServiceAuthRequiredRouteRejectsMissingOrWrongKey(t *testing.T) {
	app := NewApp(Config{ServiceName: "all", HTTPAddr: ":0", ServiceAPIKey: "svc-key"})
	registerServiceAuthRoute(app)

	for _, key := range []string{"", "wrong"} {
		req := httptest.NewRequest(http.MethodPost, "/internal/owner/o-1", nil)
		if key != "" {
			req.Header.Set(serviceKeyHeader, key)
		}
		rec := httptest.NewRecorder()
		app.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("key %q status = %d, want 401: %s", key, rec.Code, rec.Body.String())
		}
	}
}

func TestServiceAuthRequiredRouteAllowsConfiguredKey(t *testing.T) {
	app := NewApp(Config{ServiceName: "all", HTTPAddr: ":0", ServiceAPIKey: "svc-key"})
	registerServiceAuthRoute(app)

	req := httptest.NewRequest(http.MethodPost, "/internal/owner/o-1", nil)
	req.Header.Set(serviceKeyHeader, "svc-key")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
}

func registerServiceAuthRoute(app *App) {
	app.RegisterService(ServiceSpec{
		Name: "test-service",
		Routes: []RouteSpec{{
			Method:              http.MethodPost,
			Pattern:             "/internal/owner/{id}",
			Resource:            "owners",
			Action:              "create",
			ServiceAuthRequired: true,
		}},
	})
	app.RegisterCustomHandler(http.MethodPost, "/internal/owner/{id}", func(_ *App, _ *http.Request, _ RouteSpec) (int, any, *Degraded) {
		return http.StatusOK, map[string]any{"ok": true}, nil
	})
}
