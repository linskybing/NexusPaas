package services

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// TestProxyRBACDefaultSeedingIdempotentUnderConcurrency fires many concurrent
// first requests that each trigger ensureDefaultServices and asserts the
// default set is seeded exactly once (finding 28: lazy seeding must be
// idempotent under concurrency, not double-seed).
func TestProxyRBACDefaultSeedingIdempotentUnderConcurrency(t *testing.T) {
	app := newTestApp()
	seedAuthorizationPolicyUsers(t, app)
	headers := adminHeaders("ADMIN")

	const workers = 32
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/proxy-rbac/services", strings.NewReader(""))
			for k, v := range headers {
				req.Header.Set(k, v)
			}
			app.ServeHTTP(rec, req)
		}()
	}
	wg.Wait()

	services := responseSlice(t, requestJSON(t, app, http.MethodGet, "/api/v1/admin/proxy-rbac/services", "", headers, http.StatusOK))
	if len(services) != 8 {
		t.Fatalf("expected 8 seeded services with no duplicates, got %d", len(services))
	}
}
