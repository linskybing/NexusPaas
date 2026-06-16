package services

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

// With DEX_URL configured, the device authorization endpoint proxies to Dex's
// /device/code endpoint, and the revoke endpoint accepts a token and returns 200
// (RFC 7009) via the local denylist.
func TestOIDCDeviceProxyAndRevokeWithDex(t *testing.T) {
	var gotPath string
	dex := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"device_code":"dc-1","user_code":"UCODE"}`)
	}))
	defer dex.Close()

	app := platform.NewApp(platform.Config{ServiceName: "all", RequireAuth: false, DexURL: dex.URL})
	RegisterAll(app)

	// Device authorization proxies to Dex /device/code.
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/device_authorization?client_id=grafana", nil))
	if rec.Code != http.StatusOK || gotPath != "/device/code" {
		t.Fatalf("device authorization: status=%d dexPath=%q, want 200 /device/code", rec.Code, gotPath)
	}
	if !strings.Contains(rec.Body.String(), "device_code") {
		t.Fatalf("device authorization body did not proxy Dex response: %s", rec.Body.String())
	}

	// Revoke without a token is a bad request.
	rec = httptest.NewRecorder()
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

// Without DEX_URL, revoke and device authorization stay fail-closed (503).
func TestOIDCRevokeDeviceFailClosedWithoutDex(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: "all", RequireAuth: false})
	RegisterAll(app)

	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/oidc/revoke?token=access-1", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("revoke without provider: status=%d, want 503", rec.Code)
	}
}
