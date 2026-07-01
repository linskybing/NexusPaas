package platform

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRateLimitClassification(t *testing.T) {
	cases := []struct {
		method, pattern, want string
	}{
		{http.MethodPost, "/api/v1/images/build", rateClassBuild},
		{http.MethodPost, "/api/v1/images/build/from-storage", rateClassBuild},
		{http.MethodPost, "/api/v1/projects/{id}/storage/transfers", rateClassTransfer},
		{http.MethodPost, "/api/v1/jobs", rateClassWorkload},
		{http.MethodPost, "/api/v1/jobs/{id}/cancel", rateClassWorkload},
		{http.MethodGet, "/api/v1/jobs/{id}", rateClassDefault}, // reads are not workload-submit
		{http.MethodPost, "/api/v1/login", rateClassAuth},
		{http.MethodPost, "/api/v1/me/api-tokens", rateClassAuth},
		{http.MethodGet, "/api/v1/projects/{id}", rateClassDefault},
	}
	for _, tc := range cases {
		got := rateLimitClass(RouteSpec{Method: tc.method, Pattern: tc.pattern})
		if got != tc.want {
			t.Fatalf("rateLimitClass(%s %s) = %q, want %q", tc.method, tc.pattern, got, tc.want)
		}
	}
}

func TestSpecialRateLimitPolicyIsTighterThanDefault(t *testing.T) {
	for _, class := range []string{rateClassBuild, rateClassTransfer, rateClassWorkload, rateClassAuth} {
		rule, special := specialRateLimit(class)
		if !special {
			t.Fatalf("class %q should have a dedicated quota", class)
		}
		if rule.limit >= defaultRateLimit {
			t.Fatalf("class %q limit %d must be tighter than default %d", class, rule.limit, defaultRateLimit)
		}
		if rule.window != time.Minute {
			t.Fatalf("class %q window %v must be one minute to match Retry-After", class, rule.window)
		}
	}
	if _, special := specialRateLimit(rateClassDefault); special {
		t.Fatal("default class must not be special (uses the limiter's configured cap)")
	}
}

func TestRateLimiterAllowWithinIsPerCallLimitAndPerKey(t *testing.T) {
	limiter := NewRateLimiter(600, time.Minute) // generous default is irrelevant to AllowWithin
	if !limiter.AllowWithin("build|user:a", 1, time.Minute) {
		t.Fatal("first request under limit 1 must be allowed")
	}
	if limiter.AllowWithin("build|user:a", 1, time.Minute) {
		t.Fatal("second request over limit 1 must be rejected")
	}
	// A different class key has its own independent budget.
	if !limiter.AllowWithin("default|user:a", 1, time.Minute) {
		t.Fatal("a different class key must not share the build budget")
	}
}

func TestRateLimitRejectionEmitsClassMetric(t *testing.T) {
	app := NewApp(Config{ServiceName: "all", HTTPAddr: ":0", RequireAuth: true, ExternalURLs: map[string]string{}})
	app.Rate = NewRateLimiter(0, time.Minute) // reject everything
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
		t.Fatalf("status = %d, want 429", rec.Code)
	}

	metricsRec := httptest.NewRecorder()
	app.Metrics.ServeHTTP(metricsRec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if !strings.Contains(metricsRec.Body.String(), `rate_limit_rejected_total{class="default"}`) {
		t.Fatalf("metrics missing rate_limit_rejected_total class counter:\n%s", metricsRec.Body.String())
	}
}
