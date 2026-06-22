package platform

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestValidateRouteCollisionsRejectsDuplicateCanonicalRoutePatterns(t *testing.T) {
	app := NewApp(Config{ServiceName: "all", HTTPAddr: ":0"})
	app.RegisterService(ServiceSpec{
		Name: "test-service",
		Routes: []RouteSpec{
			{Method: http.MethodGet, Pattern: "/api/v1/items/{id}", Resource: "items", Action: "get", IDParam: "id"},
			{Method: http.MethodGet, Pattern: "/api/v1/items/{item}", Resource: "duplicate_items", Action: "get", IDParam: "item"},
		},
	})

	if len(app.Routes) != 1 {
		t.Fatalf("route count = %d, want one canonical route", len(app.Routes))
	}
	if app.Routes[0].Resource != "test-service:items" {
		t.Fatalf("registered route = %#v, want first route to win", app.Routes[0])
	}
	if len(app.CatalogRoutes) != 2 {
		t.Fatalf("catalog route count = %d, want full catalog retained", len(app.CatalogRoutes))
	}
	if err := app.ValidateRouteCollisions(); err == nil {
		t.Fatal("ValidateRouteCollisions succeeded, want duplicate route error")
	}
}

func TestValidateRouteCollisionsAcceptsAlias(t *testing.T) {
	app := NewApp(Config{ServiceName: "all", HTTPAddr: ":0"})
	app.RegisterService(ServiceSpec{
		Name: "test-service",
		Routes: []RouteSpec{
			{Method: http.MethodGet, Pattern: "/api/v1/items/{id}", Resource: "items", Action: "get", IDParam: "id"},
			{Method: http.MethodGet, Pattern: "/api/v1/items/{item}", Resource: "items_alias", Action: "get", IDParam: "item", AliasOf: "/api/v1/items/{id}"},
		},
	})

	if err := app.ValidateRouteCollisions(); err != nil {
		t.Fatalf("ValidateRouteCollisions alias = %v, want nil", err)
	}
	if len(app.Routes) != 1 || app.Routes[0].Resource != "test-service:items" {
		t.Fatalf("registered routes = %#v, want alias catalog-only", app.Routes)
	}
}

func TestRegisterServiceAllowsExplicitRouteOverride(t *testing.T) {
	app := NewApp(Config{ServiceName: "all", HTTPAddr: ":0"})
	app.RegisterService(ServiceSpec{
		Name: "test-service",
		Routes: []RouteSpec{
			{Method: http.MethodGet, Pattern: "/api/v1/items/{id}", Resource: "items", Action: "get", IDParam: "id"},
			{Method: http.MethodGet, Pattern: "/api/v1/items/{item}", Resource: "override_items", Action: "get", IDParam: "item", Override: true, OverrideReason: "replace generated route"},
		},
	})

	if err := app.ValidateRouteCollisions(); err != nil {
		t.Fatalf("ValidateRouteCollisions override = %v, want nil", err)
	}
	if len(app.Routes) != 1 || app.Routes[0].Resource != "test-service:override_items" {
		t.Fatalf("registered routes = %#v, want override route", app.Routes)
	}
}

func TestValidateRouteCollisionsRejectsOverrideWithoutReason(t *testing.T) {
	app := NewApp(Config{ServiceName: "all", HTTPAddr: ":0"})
	app.RegisterService(ServiceSpec{
		Name: "test-service",
		Routes: []RouteSpec{
			{Method: http.MethodGet, Pattern: "/api/v1/items/{id}", Resource: "items", Action: "get", IDParam: "id"},
			{Method: http.MethodGet, Pattern: "/api/v1/items/{item}", Resource: "override_items", Action: "get", IDParam: "item", Override: true},
		},
	})

	if err := app.ValidateRouteCollisions(); err == nil {
		t.Fatal("ValidateRouteCollisions succeeded, want override reason error")
	}
}

func TestRegisterServiceKeepsDistinctMethodPatternPairs(t *testing.T) {
	app := NewApp(Config{ServiceName: "all", HTTPAddr: ":0"})
	app.RegisterService(ServiceSpec{
		Name: "test-service",
		Routes: []RouteSpec{
			{Method: http.MethodGet, Pattern: "/api/v1/items/{id}", Resource: "items", Action: "get", IDParam: "id"},
			{Method: http.MethodPost, Pattern: "/api/v1/items/{item}", Resource: "items", Action: "create", IDParam: "item"},
		},
	})

	if len(app.Routes) != 2 {
		t.Fatalf("route count = %d, want GET and POST retained", len(app.Routes))
	}
}

func TestValidateInternalRouteAuthRequiresServiceAuth(t *testing.T) {
	app := NewApp(Config{ServiceName: "all", HTTPAddr: ":0"})
	app.RegisterService(ServiceSpec{
		Name: "test-service",
		Routes: []RouteSpec{
			{Method: http.MethodPost, Pattern: "/internal/items", Resource: "items", Action: "create", AuthRequired: true},
		},
	})

	if err := app.ValidateInternalRouteAuth(); err == nil {
		t.Fatal("ValidateInternalRouteAuth succeeded, want missing service auth error")
	}
}

func TestValidateInternalRouteAuthAllowsServiceAuthOrExplicitPublic(t *testing.T) {
	app := NewApp(Config{ServiceName: "all", HTTPAddr: ":0"})
	app.RegisterService(ServiceSpec{
		Name: "test-service",
		Routes: []RouteSpec{
			{Method: http.MethodPost, Pattern: "/internal/items", Resource: "items", Action: "create", ServiceAuthRequired: true},
			{Method: http.MethodGet, Pattern: "/api/v1/internal/health", Resource: "health", Action: "list", InternalPublic: true},
		},
	})

	if err := app.ValidateInternalRouteAuth(); err != nil {
		t.Fatalf("ValidateInternalRouteAuth = %v, want nil", err)
	}
}

func TestServeServiceRouteUsesBucketIndexAndSpecificity(t *testing.T) {
	app := NewApp(Config{ServiceName: "all", HTTPAddr: ":0"})
	app.RegisterService(ServiceSpec{
		Name: "test-service",
		Routes: []RouteSpec{
			{Method: http.MethodGet, Pattern: "/api/v1/groups/{id}", Resource: "groups", Action: "get", IDParam: "id"},
			{Method: http.MethodGet, Pattern: "/api/v1/projects/{id}", Resource: "projects", Action: "get", IDParam: "id"},
			{Method: http.MethodGet, Pattern: "/api/v1/{path...}", Resource: "compat_proxy", Action: "proxy"},
		},
	})
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/groups/{id}", routeEchoHandler)
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/{path...}", routeEchoHandler)

	candidates := app.routeCandidates(http.MethodGet, "/api/v1/groups/g1")
	if len(candidates) != 2 {
		t.Fatalf("candidate count = %d, want concrete group + wildcard: %#v", len(candidates), candidates)
	}
	for _, route := range candidates {
		if route.Resource == "test-service:projects" {
			t.Fatalf("candidate set included cross-bucket project route: %#v", candidates)
		}
	}

	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/groups/g1", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	data := responseData(t, rec)
	if data["resource"] != "test-service:groups" || data["id"] != "g1" {
		t.Fatalf("response data = %#v, want concrete group route", data)
	}
}

func TestServeServiceRouteFallsThroughForUnrelatedBucket(t *testing.T) {
	app := NewApp(Config{ServiceName: "all", HTTPAddr: ":0"})
	app.RegisterService(ServiceSpec{
		Name: "test-service",
		Routes: []RouteSpec{
			{Method: http.MethodGet, Pattern: "/api/v1/projects/{id}", Resource: "projects", Action: "get", IDParam: "id"},
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/groups/g1", nil)
	if app.serveServiceRoute(httptest.NewRecorder(), req) {
		t.Fatal("serveServiceRoute matched unrelated bucket, want false")
	}
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/groups/g1", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want mux 405 fallthrough", rec.Code)
	}
}

func TestGatewayCatalogProxyRoutesNonLocalPublicCatalogRoute(t *testing.T) {
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/projects" || r.URL.RawQuery != "page=1" {
			t.Fatalf("upstream request = %s %s?%s", r.Method, r.URL.Path, r.URL.RawQuery)
		}
		if r.Header.Get("X-Api-Key") != "adminkey" || r.Header.Get("X-Request-Id") != "req-1" {
			t.Fatalf("forwarded headers = %#v", r.Header)
		}
		for _, key := range []string{"X-Service-Key", "X-User-Id", "X-Username", "X-User-Role", "X-Admin", "X-Api-Token-Id"} {
			if r.Header.Get(key) != "" {
				t.Fatalf("%s was forwarded", key)
			}
		}
		w.Header().Set("Content-Type", "application/vnd.nexuspaas+json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"success":true,"data":[{"id":"P1"}]}`))
	}))
	defer upstream.Close()

	app := NewApp(Config{
		ServiceName: "platform-gateway",
		RequireAuth: false,
		ServiceURLs: map[string]string{"org-project-service": upstream.URL},
	})
	registerGatewayProxyTestCatalog(app)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects?page=1", nil)
	req.Header.Set("X-API-Key", "adminkey")
	req.Header.Set("X-Request-ID", "req-1")
	req.Header.Set("X-Service-Key", "service-secret")
	req.Header.Set("X-User-ID", "spoofed")
	req.Header.Set("X-Username", "spoofed")
	req.Header.Set("X-User-Role", "admin")
	req.Header.Set("X-Admin", "true")
	req.Header.Set("X-API-Token-ID", "AT1")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202: %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Content-Type") != "application/vnd.nexuspaas+json" {
		t.Fatalf("content-type = %q", rec.Header().Get("Content-Type"))
	}
	if strings.TrimSpace(rec.Body.String()) != `{"success":true,"data":[{"id":"P1"}]}` {
		t.Fatalf("body = %s", rec.Body.String())
	}
	if calls != 1 {
		t.Fatalf("upstream calls = %d, want 1", calls)
	}
}

func TestGatewayCatalogProxyPreservesMethodQueryBodyAndPassesThroughResponse(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/projects" || r.URL.RawQuery != "dry_run=1" || string(body) != `{"name":"p"}` {
			t.Fatalf("upstream request = %s %s?%s body=%s", r.Method, r.URL.Path, r.URL.RawQuery, string(body))
		}
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"success":false,"error":{"code":"conflict"}}`))
	}))
	defer upstream.Close()

	app := NewApp(Config{
		ServiceName: "platform-gateway",
		RequireAuth: false,
		ServiceURLs: map[string]string{"org-project-service": upstream.URL},
	})
	registerGatewayProxyTestCatalog(app)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects?dry_run=1", strings.NewReader(`{"name":"p"}`))
	req.Header.Set("Content-Type", "application/json")
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409: %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Content-Type") != "application/problem+json" {
		t.Fatalf("content-type = %q", rec.Header().Get("Content-Type"))
	}
	if strings.TrimSpace(rec.Body.String()) != `{"success":false,"error":{"code":"conflict"}}` {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestGatewayCatalogProxySkipsLocalExternalAdapterPreflight(t *testing.T) {
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/images/build" || string(body) != `{"id":"B1","project_id":"P1"}` {
			t.Fatalf("upstream request = %s %s body=%s", r.Method, r.URL.Path, string(body))
		}
		w.Header().Set("Content-Type", "application/vnd.nexuspaas+json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"success":true,"data":{"id":"B1","project_id":"P1"}}`))
	}))
	defer upstream.Close()

	app := NewApp(Config{
		ServiceName: "platform-gateway",
		RequireAuth: false,
		ServiceURLs: map[string]string{"image-registry-service": upstream.URL},
	})
	app.RegisterService(ServiceSpec{
		Name: "platform-gateway",
		Routes: []RouteSpec{{
			Method: http.MethodGet, Pattern: "/api/v1/gateway/health", Resource: "health", Action: "list", AuthRequired: false,
		}},
	})
	app.RegisterService(ServiceSpec{
		Name: "image-registry-service",
		Routes: []RouteSpec{{
			Method: http.MethodPost, Pattern: "/api/v1/images/build", Resource: "image_build_jobs", Action: "command", AuthRequired: false, ExternalAdapter: "harbor",
		}},
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/images/build", strings.NewReader(`{"id":"B1","project_id":"P1"}`))
	req.Header.Set("Content-Type", "application/json")
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202: %s", rec.Code, rec.Body.String())
	}
	if strings.TrimSpace(rec.Body.String()) != `{"success":true,"data":{"id":"B1","project_id":"P1"}}` {
		t.Fatalf("body = %s", rec.Body.String())
	}
	if calls != 1 {
		t.Fatalf("upstream calls = %d, want 1", calls)
	}
	if app.Metrics.Counter("harbor_degraded") != 0 {
		t.Fatalf("harbor degraded counter = %d, want 0", app.Metrics.Counter("harbor_degraded"))
	}
}

func TestLocalExternalAdapterRouteStillRunsPreflight(t *testing.T) {
	adapter := &fakeAdapter{}
	app := NewApp(Config{ServiceName: "image-registry-service", RequireAuth: false})
	app.Adapters["harbor"] = adapter
	app.RegisterAction("local_test", func(_ *httpRequest, _ RouteSpec) (int, any) {
		return http.StatusOK, map[string]any{"ok": true}
	})
	app.RegisterService(ServiceSpec{
		Name: "image-registry-service",
		Routes: []RouteSpec{{
			Method: http.MethodGet, Pattern: "/api/v1/local-adapter-check", Resource: "checks", Action: "local_test", AuthRequired: false, ExternalAdapter: "harbor",
		}},
	})

	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/local-adapter-check", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if adapter.calls != 1 {
		t.Fatalf("adapter calls = %d, want 1", adapter.calls)
	}
}

func TestGatewayCatalogProxyKeepsLocalRoutePrecedence(t *testing.T) {
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls++ }))
	defer upstream.Close()

	app := NewApp(Config{
		ServiceName: "platform-gateway",
		RequireAuth: false,
		ServiceURLs: map[string]string{"org-project-service": upstream.URL},
	})
	app.RegisterService(ServiceSpec{
		Name: "platform-gateway",
		Routes: []RouteSpec{{
			Method: http.MethodGet, Pattern: "/api/v1/projects", Resource: "projects", Action: "list", AuthRequired: false,
		}},
	})
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/projects", routeEchoHandler)
	app.RegisterService(ServiceSpec{
		Name: "org-project-service",
		Routes: []RouteSpec{{
			Method: http.MethodGet, Pattern: "/api/v1/projects", Resource: "projects", Action: "list", AuthRequired: true,
		}},
	})

	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if data := responseData(t, rec); data["resource"] != "platform-gateway:projects" {
		t.Fatalf("response data = %#v, want local route", data)
	}
	if calls != 0 {
		t.Fatalf("upstream calls = %d, want 0", calls)
	}
}

func TestGatewayCatalogProxyAuthDenialDoesNotCallUpstream(t *testing.T) {
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls++ }))
	defer upstream.Close()

	app := NewApp(Config{
		ServiceName: "platform-gateway",
		RequireAuth: true,
		ServiceURLs: map[string]string{"org-project-service": upstream.URL},
	})
	registerGatewayProxyTestCatalog(app)

	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401: %s", rec.Code, rec.Body.String())
	}
	if calls != 0 {
		t.Fatalf("upstream calls = %d, want 0", calls)
	}
}

func TestGatewayCatalogProxyMissingServiceURLsKeepsFallthrough(t *testing.T) {
	app := NewApp(Config{ServiceName: "platform-gateway", RequireAuth: false})
	registerGatewayProxyTestCatalog(app)

	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil))

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want mux fallthrough 405", rec.Code)
	}
}

func TestGatewayCatalogProxyMissingOwnerURLReturnsGatewayError(t *testing.T) {
	app := NewApp(Config{
		ServiceName: "platform-gateway",
		RequireAuth: false,
		ServiceURLs: map[string]string{"identity-service": "http://identity-service"},
	})
	registerGatewayProxyTestCatalog(app)

	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil))

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "downstream service URL is not configured") {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestGatewayCatalogProxySetsForwardedOriginForOIDCRoutes(t *testing.T) {
	var gotHost, gotProto string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHost = r.Header.Get("X-Forwarded-Host")
		gotProto = r.Header.Get("X-Forwarded-Proto")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()

	app := NewApp(Config{
		ServiceName: "platform-gateway",
		RequireAuth: false,
		ServiceURLs: map[string]string{"identity-service": upstream.URL},
	})
	app.RegisterService(ServiceSpec{
		Name: "identity-service",
		Routes: []RouteSpec{{
			Method: http.MethodGet, Pattern: "/api/v1/oidc/start", Resource: "oidc", Action: "start", AuthRequired: false,
		}},
	})

	req := httptest.NewRequest(http.MethodGet, "http://localhost:8080/api/v1/oidc/start", nil)
	req.Host = "localhost:8080"
	req.Header.Set("X-Forwarded-Host", "evil.test")
	req.Header.Set("X-Forwarded-Proto", "https")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204: %s", rec.Code, rec.Body.String())
	}
	if gotHost != "localhost:8080" {
		t.Fatalf("X-Forwarded-Host = %q, want gateway host", gotHost)
	}
	if gotProto != "https" {
		t.Fatalf("X-Forwarded-Proto = %q, want https", gotProto)
	}
}

func TestGatewayCatalogProxyPreservesOIDCRedirects(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/oidc/start" {
			t.Fatalf("upstream path = %s, want /api/v1/oidc/start", r.URL.Path)
		}
		w.Header().Set("Location", "/api/v1/oidc/authorize?state=opaque")
		w.Header().Add("Set-Cookie", "nexuspaas_oidc_state=opaque; Path=/; HttpOnly; SameSite=Lax")
		w.WriteHeader(http.StatusFound)
	}))
	defer upstream.Close()

	app := NewApp(Config{
		ServiceName: "platform-gateway",
		RequireAuth: false,
		ServiceURLs: map[string]string{"identity-service": upstream.URL},
	})
	app.RegisterService(ServiceSpec{
		Name: "identity-service",
		Routes: []RouteSpec{{
			Method: http.MethodGet, Pattern: "/api/v1/oidc/start", Resource: "oidc", Action: "start", AuthRequired: false,
		}},
	})

	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "http://localhost:8080/api/v1/oidc/start", nil))

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302: %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Location") != "/api/v1/oidc/authorize?state=opaque" {
		t.Fatalf("Location = %q", rec.Header().Get("Location"))
	}
	if got := rec.Header().Values("Set-Cookie"); len(got) != 1 || !strings.Contains(got[0], "nexuspaas_oidc_state=") {
		t.Fatalf("Set-Cookie = %#v, want OIDC state cookie", got)
	}
}

func TestGatewayCatalogProxyPreservesDexRedirects(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/dex/auth/local" {
			t.Fatalf("upstream path = %s, want /dex/auth/local", r.URL.Path)
		}
		if r.Header.Get("X-Forwarded-Host") != "" || r.Header.Get("X-Forwarded-Proto") != "" {
			t.Fatalf("forwarded origin leaked to Dex route: %#v", r.Header)
		}
		w.Header().Set("Location", "/dex/auth/local/login?state=opaque")
		w.Header().Add("Set-Cookie", "dex-session=opaque; Path=/dex; HttpOnly")
		w.WriteHeader(http.StatusFound)
	}))
	defer upstream.Close()

	app := NewApp(Config{
		ServiceName: "platform-gateway",
		RequireAuth: false,
		ServiceURLs: map[string]string{"identity-service": upstream.URL},
	})
	app.RegisterService(ServiceSpec{
		Name: "identity-service",
		Routes: []RouteSpec{{
			Method: http.MethodGet, Pattern: "/dex/{path...}", Resource: "dex_browser", Action: "browser_proxy", AuthRequired: false,
		}},
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost:8080/dex/auth/local", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302: %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Location") != "/dex/auth/local/login?state=opaque" {
		t.Fatalf("Location = %q", rec.Header().Get("Location"))
	}
	if got := rec.Header().Values("Set-Cookie"); len(got) != 1 || !strings.Contains(got[0], "dex-session=") {
		t.Fatalf("Set-Cookie = %#v, want Dex session cookie", got)
	}
}

func TestGatewayCatalogProxyDoesNotSetForwardedOriginForOrdinaryRoutes(t *testing.T) {
	var gotHost, gotProto string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHost = r.Header.Get("X-Forwarded-Host")
		gotProto = r.Header.Get("X-Forwarded-Proto")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()

	app := NewApp(Config{
		ServiceName: "platform-gateway",
		RequireAuth: false,
		ServiceURLs: map[string]string{"org-project-service": upstream.URL},
	})
	registerGatewayProxyTestCatalog(app)

	req := httptest.NewRequest(http.MethodGet, "http://localhost:8080/api/v1/projects", nil)
	req.Host = "localhost:8080"
	req.Header.Set("X-Forwarded-Proto", "https")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204: %s", rec.Code, rec.Body.String())
	}
	if gotHost != "" || gotProto != "" {
		t.Fatalf("forwarded origin headers = host %q proto %q, want none", gotHost, gotProto)
	}
}

func TestGatewayCatalogProxyDoesNotExposeInternalRoutes(t *testing.T) {
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls++ }))
	defer upstream.Close()

	app := NewApp(Config{
		ServiceName: "platform-gateway",
		RequireAuth: false,
		ServiceURLs: map[string]string{"org-project-service": upstream.URL},
	})
	app.RegisterService(ServiceSpec{
		Name: "org-project-service",
		Routes: []RouteSpec{
			{Method: http.MethodGet, Pattern: "/internal/org-project/projects", Resource: "projects", Action: "list", AuthRequired: false},
			{Method: http.MethodGet, Pattern: "/api/v1/internal/org-project/projects", Resource: "projects", Action: "list", AuthRequired: false},
		},
	})

	for _, path := range []string{"/internal/org-project/projects", "/api/v1/internal/org-project/projects"} {
		rec := httptest.NewRecorder()
		app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code == http.StatusOK {
			t.Fatalf("%s returned 200, want fallthrough", path)
		}
	}
	if calls != 0 {
		t.Fatalf("upstream calls = %d, want 0", calls)
	}
}

func TestServeServiceRouteRebuildsIndexForSameLengthRouteMutation(t *testing.T) {
	app := NewApp(Config{ServiceName: "all", HTTPAddr: ":0"})
	app.Routes = []RouteSpec{{Method: http.MethodGet, Pattern: "/api/v1/old/{id}", Resource: "test-service:old", Action: "get", IDParam: "id"}}
	app.rebuildRouteIndex()
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/new/{id}", routeEchoHandler)
	app.Routes[0] = RouteSpec{Method: http.MethodGet, Pattern: "/api/v1/new/{id}", Resource: "test-service:new", Action: "get", IDParam: "id"}

	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/new/n1", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 after same-length route mutation: %s", rec.Code, rec.Body.String())
	}
	data := responseData(t, rec)
	if data["resource"] != "test-service:new" || data["id"] != "n1" {
		t.Fatalf("response data = %#v, want rebuilt new route", data)
	}
}

func TestHandleReservationTransitionStateMachine(t *testing.T) {
	app := NewApp(Config{ServiceName: "all", HTTPAddr: ":0"})
	route := RouteSpec{IDParam: "reservationId"}
	cases := []struct {
		name      string
		current   string
		requested string
		want      int
		wantState string
	}{
		{name: "reserved noop", current: "reserved", requested: "reserved", want: http.StatusOK, wantState: "reserved"},
		{name: "reserved commit", current: "reserved", requested: "committed", want: http.StatusOK, wantState: "committed"},
		{name: "reserved release", current: "reserved", requested: "released", want: http.StatusOK, wantState: "released"},
		{name: "committed release", current: "committed", requested: "released", want: http.StatusOK, wantState: "released"},
		{name: "released noop", current: "released", requested: "released", want: http.StatusOK, wantState: "released"},
		{name: "released commit conflict", current: "released", requested: "committed", want: http.StatusConflict, wantState: "released"},
		{name: "committed reserved conflict", current: "committed", requested: "reserved", want: http.StatusConflict, wantState: "committed"},
		{name: "unknown commit conflict", current: "unknown", requested: "committed", want: http.StatusConflict, wantState: "unknown"},
		{name: "unknown release conflict", current: "unknown", requested: "released", want: http.StatusConflict, wantState: "unknown"},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			id := "res-" + strconv.Itoa(i)
			_, _ = app.Store.Create(context.Background(), "scheduler-quota-service:reservations", map[string]any{"id": id, "state": tc.current})
			status, _ := app.handleReservationTransition(testHTTPRequest(id), route, tc.requested)
			if status != tc.want {
				t.Fatalf("status = %d, want %d", status, tc.want)
			}
			record, ok := app.Store.Get(context.Background(), "scheduler-quota-service:reservations", id)
			if !ok {
				t.Fatalf("reservation %s missing", id)
			}
			if record.Data["state"] != tc.wantState {
				t.Fatalf("state = %v, want %s", record.Data["state"], tc.wantState)
			}
		})
	}

	status, _ := app.handleReservationTransition(testHTTPRequest("missing"), route, "committed")
	if status != http.StatusNotFound {
		t.Fatalf("missing reservation status = %d, want 404", status)
	}
}

func TestHandleReservationTransitionPublishesStateEvents(t *testing.T) {
	app := NewApp(Config{ServiceName: "all", HTTPAddr: ":0"})
	route := RouteSpec{IDParam: "reservationId"}
	_, _ = app.Store.Create(context.Background(), "scheduler-quota-service:reservations", map[string]any{"id": "commit-res", "state": "reserved"})
	status, _ := app.handleReservationTransition(testHTTPRequest("commit-res"), route, "committed")
	if status != http.StatusOK {
		t.Fatalf("commit status = %d, want 200", status)
	}
	assertPlatformEventCount(t, app, "QuotaCommitted", 1)
	status, _ = app.handleReservationTransition(testHTTPRequest("commit-res"), route, "committed")
	if status != http.StatusOK {
		t.Fatalf("same-state commit status = %d, want 200", status)
	}
	assertPlatformEventCount(t, app, "QuotaCommitted", 1)

	_, _ = app.Store.Create(context.Background(), "scheduler-quota-service:reservations", map[string]any{"id": "release-res", "state": "reserved"})
	status, _ = app.handleReservationTransition(testHTTPRequest("release-res"), route, "released")
	if status != http.StatusOK {
		t.Fatalf("release status = %d, want 200", status)
	}
	assertPlatformEventCount(t, app, "QuotaReleased", 1)
	status, _ = app.handleReservationTransition(testHTTPRequest("release-res"), route, "committed")
	if status != http.StatusConflict {
		t.Fatalf("released->committed status = %d, want 409", status)
	}
	assertPlatformEventCount(t, app, "QuotaCommitted", 1)
}

func TestHandleCRUDPostDuplicateIDConflictHasNoMutationOrEvent(t *testing.T) {
	app := NewApp(Config{ServiceName: "all", HTTPAddr: ":0"})
	route := RouteSpec{Resource: "test-service:records", Action: "create"}
	_, _ = app.Store.Create(context.Background(), route.Resource, map[string]any{"id": "r1", "name": "original"})
	outboxBefore := len(app.Events.Outbox())

	status, _ := app.handleCRUD(testJSONHTTPRequest(http.MethodPost, `{"id":"r1","name":"replacement"}`), route)
	if status != http.StatusConflict {
		t.Fatalf("duplicate POST status = %d, want 409", status)
	}
	record, ok := app.Store.Get(context.Background(), route.Resource, "r1")
	if !ok || record.Data["name"] != "original" {
		t.Fatalf("duplicate POST mutated record = %#v, found=%v", record.Data, ok)
	}
	if got := len(app.Events.Outbox()); got != outboxBefore {
		t.Fatalf("outbox length = %d, want unchanged %d", got, outboxBefore)
	}
}

func TestHandleCRUDUpsertFallbackDuplicateIDConflictHasNoMutationOrEvent(t *testing.T) {
	app := NewApp(Config{ServiceName: "all", HTTPAddr: ":0"})
	route := RouteSpec{Resource: "test-service:records", Action: "update"}
	originalHook := beforeCRUDFallbackCreate
	beforeCRUDFallbackCreate = func(app *App, r *httpRequest, route RouteSpec, payload map[string]any) {
		_, _ = app.Store.Create(r.Context(), route.Resource, map[string]any{"id": payload["id"], "name": "concurrent"})
	}
	defer func() { beforeCRUDFallbackCreate = originalHook }()
	outboxBefore := len(app.Events.Outbox())

	status, _ := app.handleCRUD(testJSONHTTPRequest(http.MethodPut, `{"id":"r-race","name":"replacement"}`), route)
	if status != http.StatusConflict {
		t.Fatalf("duplicate fallback PUT status = %d, want 409", status)
	}
	record, ok := app.Store.Get(context.Background(), route.Resource, "r-race")
	if !ok || record.Data["name"] != "concurrent" {
		t.Fatalf("fallback duplicate mutated record = %#v, found=%v", record.Data, ok)
	}
	if got := len(app.Events.Outbox()); got != outboxBefore {
		t.Fatalf("outbox length = %d, want unchanged %d", got, outboxBefore)
	}
}

func TestHandleCommandDuplicateIDConflictHasNoMutationOrEvent(t *testing.T) {
	app := NewApp(Config{ServiceName: "all", HTTPAddr: ":0"})
	route := RouteSpec{Resource: "workload-service:jobs", Action: "submit"}
	resource := route.Resource + ":commands"
	_, _ = app.Store.Create(context.Background(), resource, map[string]any{"id": "cmd-dup", "name": "original"})
	outboxBefore := len(app.Events.Outbox())

	status, _ := app.handleCommand(testJSONHTTPRequest(http.MethodPost, `{"id":"cmd-dup","name":"replacement"}`), route)
	if status != http.StatusConflict {
		t.Fatalf("duplicate command status = %d, want 409", status)
	}
	record, ok := app.Store.Get(context.Background(), resource, "cmd-dup")
	if !ok || record.Data["name"] != "original" {
		t.Fatalf("duplicate command mutated record = %#v, found=%v", record.Data, ok)
	}
	if got := len(app.Events.Outbox()); got != outboxBefore {
		t.Fatalf("outbox length = %d, want unchanged %d", got, outboxBefore)
	}
}

func TestHandleConfigCommitDuplicateIDConflictHasNoMutationOrEvent(t *testing.T) {
	app := NewApp(Config{ServiceName: "all", HTTPAddr: ":0"})
	route := RouteSpec{Resource: "workload-service:configfiles", Action: "commit"}
	resource := route.Resource + ":versions"
	_, _ = app.Store.Create(context.Background(), resource, map[string]any{"id": "ver-dup", "content": "original"})
	outboxBefore := len(app.Events.Outbox())

	status, _ := app.handleConfigCommit(testJSONHTTPRequest(http.MethodPost, `{"id":"ver-dup","content":"replacement"}`), route)
	if status != http.StatusConflict {
		t.Fatalf("duplicate config commit status = %d, want 409", status)
	}
	record, ok := app.Store.Get(context.Background(), resource, "ver-dup")
	if !ok || record.Data["content"] != "original" {
		t.Fatalf("duplicate config commit mutated record = %#v, found=%v", record.Data, ok)
	}
	if got := len(app.Events.Outbox()); got != outboxBefore {
		t.Fatalf("outbox length = %d, want unchanged %d", got, outboxBefore)
	}
}

func TestHandleReservationDuplicateIDConflictHasNoMutationOrEvent(t *testing.T) {
	app := NewApp(Config{ServiceName: "all", HTTPAddr: ":0"})
	route := RouteSpec{Resource: "scheduler-quota-service:reservations", Action: "reserve"}
	_, _ = app.Store.Create(context.Background(), route.Resource, map[string]any{"id": "res-dup", "state": "original"})
	outboxBefore := len(app.Events.Outbox())

	status, _ := app.handleReservation(testJSONHTTPRequest(http.MethodPost, `{"id":"res-dup"}`), route, "reserved")
	if status != http.StatusConflict {
		t.Fatalf("duplicate reservation status = %d, want 409", status)
	}
	record, ok := app.Store.Get(context.Background(), route.Resource, "res-dup")
	if !ok || record.Data["state"] != "original" {
		t.Fatalf("duplicate reservation mutated record = %#v, found=%v", record.Data, ok)
	}
	if got := len(app.Events.Outbox()); got != outboxBefore {
		t.Fatalf("outbox length = %d, want unchanged %d", got, outboxBefore)
	}
}

func testHTTPRequest(reservationID string) *httpRequest {
	req := httptest.NewRequest(http.MethodPost, "/reservation/"+reservationID, nil)
	req.SetPathValue("reservationId", reservationID)
	return &httpRequest{Request: req, Service: "test", TraceID: "trace"}
}

func testJSONHTTPRequest(method, body string) *httpRequest {
	req := httptest.NewRequest(method, "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return &httpRequest{Request: req, Service: "test", TraceID: "trace", IdempotencyKey: "test-key"}
}

func assertPlatformEventCount(t *testing.T, app *App, name string, want int) {
	t.Helper()
	got := 0
	for _, event := range app.Events.Outbox() {
		if event.Name == name {
			got++
		}
	}
	if got != want {
		t.Fatalf("%s event count = %d, want %d", name, got, want)
	}
}

func routeEchoHandler(_ *App, r *http.Request, route RouteSpec) (int, any, *Degraded) {
	return http.StatusOK, map[string]any{
		"resource": route.Resource,
		"id":       r.PathValue("id"),
	}, nil
}

func registerGatewayProxyTestCatalog(app *App) {
	app.RegisterService(ServiceSpec{
		Name: "platform-gateway",
		Routes: []RouteSpec{{
			Method: http.MethodGet, Pattern: "/api/v1/gateway/health", Resource: "health", Action: "list", AuthRequired: false,
		}},
	})
	app.RegisterService(ServiceSpec{
		Name: "org-project-service",
		Routes: []RouteSpec{
			{Method: http.MethodGet, Pattern: "/api/v1/projects", Resource: "projects", Action: "list", AuthRequired: true},
			{Method: http.MethodPost, Pattern: "/api/v1/projects", Resource: "projects", Action: "create", AuthRequired: true},
			{Method: http.MethodGet, Pattern: "/internal/org-project/projects", Resource: "projects", Action: "list", ServiceAuthRequired: true},
		},
	})
}

func responseData(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var env struct {
		Data map[string]any `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatal(err)
	}
	return env.Data
}
