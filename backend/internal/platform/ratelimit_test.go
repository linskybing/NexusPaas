package platform

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRateLimiterRemovesExpiredCounters(t *testing.T) {
	limiter := NewRateLimiter(2, time.Minute)
	if !limiter.Allow("stale") {
		t.Fatal("first stale key request was denied")
	}

	limiter.mu.Lock()
	limiter.counters["stale"] = rateCounter{count: 1, expiresAt: time.Now().UTC().Add(-time.Second)}
	limiter.mu.Unlock()

	if !limiter.Allow("active") {
		t.Fatal("first active key request was denied")
	}

	limiter.mu.Lock()
	defer limiter.mu.Unlock()
	if _, ok := limiter.counters["stale"]; ok {
		t.Fatalf("expired stale key was retained: %#v", limiter.counters)
	}
	if got := limiter.counters["active"].count; got != 1 {
		t.Fatalf("active count = %d, want 1", got)
	}
}

func TestRateLimiterKeepsActiveLimitBehavior(t *testing.T) {
	limiter := NewRateLimiter(2, time.Minute)
	if !limiter.Allow("client") {
		t.Fatal("first request denied")
	}
	if !limiter.Allow("client") {
		t.Fatal("second request denied")
	}
	if limiter.Allow("client") {
		t.Fatal("third request allowed, want over-limit denial")
	}
}

func TestClientKeyIgnoresXFFWithoutTrustedProxies(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "203.0.113.10:1234"
	req.Header.Set("X-Forwarded-For", "198.51.100.2")

	if got := clientKey(req, nil); got != "203.0.113.10" {
		t.Fatalf("client key = %q, want remote addr", got)
	}
}

func TestClientKeyUsesRightmostUntrustedXFFWhenRemoteIsTrusted(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "203.0.113.10:1234"
	req.Header.Set("X-Forwarded-For", "198.51.100.5, 203.0.113.20")

	if got := clientKey(req, parseTrustedProxyCIDRs("203.0.113.0/24")); got != "198.51.100.5" {
		t.Fatalf("client key = %q, want rightmost untrusted hop", got)
	}
}

func TestClientKeySkipsMalformedXFFHops(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "203.0.113.10:1234"
	req.Header.Set("X-Forwarded-For", "bad, 198.51.100.5")

	if got := clientKey(req, parseTrustedProxyCIDRs("203.0.113.0/24")); got != "198.51.100.5" {
		t.Fatalf("client key = %q, want valid untrusted hop after malformed token", got)
	}
}

func TestClientKeyFallsBackWhenOnlyTrustedOrMalformedXFF(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "203.0.113.10:1234"
	req.Header.Set("X-Forwarded-For", "198.51.100.5:abc, 203.0.113.20")

	if got := clientKey(req, parseTrustedProxyCIDRs("203.0.113.0/24")); got != "203.0.113.10" {
		t.Fatalf("client key = %q, want remote addr fallback", got)
	}
}

func TestRateLimiterPrefersVerifiedPrincipalForAuthenticatedRoutes(t *testing.T) {
	app := NewApp(Config{
		ServiceName:  "all",
		HTTPAddr:     ":0",
		RequireAuth:  true,
		ExternalURLs: map[string]string{},
	})
	app.Rate = NewRateLimiter(1, time.Minute)
	route := RouteSpec{Method: http.MethodGet, Pattern: "/limited", Resource: "test:limited", OperationID: "limited", AuthRequired: true}
	app.Routes = []RouteSpec{route}
	app.RegisterCustomHandler(http.MethodGet, "/limited", func(_ *App, _ *http.Request, _ RouteSpec) (int, any, *Degraded) {
		return http.StatusOK, map[string]any{"ok": true}, nil
	})

	expiresAt := time.Now().UTC().Add(time.Hour).Format(time.RFC3339)
	_, _ = app.Store.Create(nil, "identity-service:users", map[string]any{"id": "USER_A", "username": "a", "status": "online"})
	_, _ = app.Store.Create(nil, "identity-service:users", map[string]any{"id": "USER_B", "username": "b", "status": "online"})
	_, _ = app.Store.Create(nil, "identity-service:sessions", map[string]any{"id": "session-a", "user_id": "USER_A", "expires_at": expiresAt})
	_, _ = app.Store.Create(nil, "identity-service:sessions", map[string]any{"id": "session-b", "user_id": "USER_B", "expires_at": expiresAt})

	requestLimitedRoute(t, app, "session-a", http.StatusOK)
	requestLimitedRoute(t, app, "session-b", http.StatusOK)
	requestLimitedRoute(t, app, "session-a", http.StatusTooManyRequests)
}

func TestRateLimitedRouteReturnsRetryGuidance(t *testing.T) {
	app := NewApp(Config{
		ServiceName:  "all",
		HTTPAddr:     ":0",
		RequireAuth:  true,
		ExternalURLs: map[string]string{},
	})
	app.Rate = NewRateLimiter(0, time.Minute)
	route := RouteSpec{Method: http.MethodGet, Pattern: "/limited", Resource: "test:limited", OperationID: "limited"}
	app.Routes = []RouteSpec{route}
	app.RegisterCustomHandler(http.MethodGet, "/limited", func(_ *App, _ *http.Request, _ RouteSpec) (int, any, *Degraded) {
		return http.StatusOK, map[string]any{"ok": true}, nil
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/limited", nil)
	req.RemoteAddr = "198.51.100.10:1234"
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Retry-After"); got != "60" {
		t.Fatalf("Retry-After = %q, want 60", got)
	}
	if !strings.Contains(rec.Body.String(), "retry after 60 seconds") {
		t.Fatalf("body = %s, want retry guidance", rec.Body.String())
	}
}

func TestServiceAuthRequiredRouteBypassesUserRateLimiter(t *testing.T) {
	app := NewApp(Config{
		ServiceName:    "all",
		HTTPAddr:       ":0",
		RequireAuth:    true,
		ServiceAPIKey:  "svc-key",
		ExternalURLs:   map[string]string{},
		AllowedOrigins: map[string]bool{},
	})
	app.Rate = NewRateLimiter(0, time.Minute)
	route := RouteSpec{
		Method:              http.MethodPost,
		Pattern:             "/internal/enforce",
		Resource:            "test:internal_enforce",
		OperationID:         "internal_enforce",
		ServiceAuthRequired: true,
		PolicyBypass:        true,
	}
	app.Routes = []RouteSpec{route}
	app.RegisterCustomHandler(http.MethodPost, "/internal/enforce", func(_ *App, _ *http.Request, _ RouteSpec) (int, any, *Degraded) {
		return http.StatusOK, map[string]any{"ok": true}, nil
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/internal/enforce", nil)
	req.Header.Set(serviceKeyHeader, "svc-key")
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 service-auth bypass: %s", rec.Code, rec.Body.String())
	}
}

func TestStateChangingRouteRejectsOversizedContentLength(t *testing.T) {
	app := NewApp(Config{
		ServiceName:     "all",
		HTTPAddr:        ":0",
		RequireAuth:     true,
		ExternalURLs:    map[string]string{},
		MaxAPIBodyBytes: 8,
	})
	called := false
	route := RouteSpec{Method: http.MethodPost, Pattern: "/limited-body", Resource: "test:limited_body", OperationID: "limited_body", StateChanging: true}
	app.Routes = []RouteSpec{route}
	app.RegisterCustomHandler(http.MethodPost, "/limited-body", func(_ *App, _ *http.Request, _ RouteSpec) (int, any, *Degraded) {
		called = true
		return http.StatusOK, map[string]any{"ok": true}, nil
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/limited-body", strings.NewReader(`{"too":"large"}`))
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413: %s", rec.Code, rec.Body.String())
	}
	if called {
		t.Fatal("handler was called for oversized request")
	}
}

func requestLimitedRoute(t *testing.T, app *App, token string, want int) {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/limited", nil)
	req.RemoteAddr = "198.51.100.10:1234"
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-User-ID", "forged")
	app.ServeHTTP(rec, req)
	if rec.Code != want {
		t.Fatalf("limited route for %s returned %d, want %d: %s", token, rec.Code, want, rec.Body.String())
	}
}
