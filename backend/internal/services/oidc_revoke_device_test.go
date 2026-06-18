package services

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

// With DEX_URL configured, the canonical revoke endpoint accepts a token and
// returns 200 (RFC 7009) via the local denylist.
func TestOIDCRevokeWithDex(t *testing.T) {
	dex := httptest.NewServer(http.NotFoundHandler())
	defer dex.Close()

	app := platform.NewApp(platform.Config{ServiceName: "all", RequireAuth: false, DexURL: dex.URL})
	RegisterAll(app)

	// Revoke without a token is a bad request.
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/oidc/revoke", strings.NewReader("")))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("revoke without token: status=%d, want 400", rec.Code)
	}

	// Revoke with a token returns 200 regardless of token validity (RFC 7009).
	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/oidc/revoke", strings.NewReader("token=some-access-token"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("revoke with token: status=%d, want 200: %s", rec.Code, rec.Body.String())
	}
}

// Without DEX_URL, revoke stays fail-closed (503).
func TestOIDCRevokeFailClosedWithoutDex(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: "all", RequireAuth: false})
	RegisterAll(app)

	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/oidc/revoke?token=access-1", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("revoke without provider: status=%d, want 503", rec.Code)
	}
}
