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

func TestServiceAuthRequiredRouteValidatesScopedCallerAudience(t *testing.T) {
	app := NewApp(Config{
		ServiceName: "all",
		HTTPAddr:    ":0",
		ServiceTrustedIdentities: map[string]ServiceTrustedIdentity{
			"caller-service": {Key: "scoped-key", Audiences: []string{"test-service"}},
			"other-service":  {Key: "other-key", Audiences: []string{"other-service"}},
		},
	})
	registerServiceAuthRoute(app)

	cases := []struct {
		name   string
		caller string
		key    string
		want   int
	}{
		{name: "missing caller", key: "scoped-key", want: http.StatusUnauthorized},
		{name: "wrong key", caller: "caller-service", key: "wrong", want: http.StatusUnauthorized},
		{name: "wrong audience", caller: "other-service", key: "other-key", want: http.StatusUnauthorized},
		{name: "allowed", caller: "caller-service", key: "scoped-key", want: http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/internal/owner/o-1", nil)
			if tc.caller != "" {
				req.Header.Set(serviceNameHeader, tc.caller)
			}
			if tc.key != "" {
				req.Header.Set(serviceKeyHeader, tc.key)
			}
			rec := httptest.NewRecorder()
			app.ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tc.want, rec.Body.String())
			}
		})
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
