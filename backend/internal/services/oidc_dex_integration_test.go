//go:build integration

package services

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

// TestIdentityOIDCProxiesToDex verifies that with DEX_URL configured the identity
// OIDC compatibility endpoints serve real Dex responses instead of failing closed
// (findings 6, 30). Requires the compose Dex container and DEX_URL.
func TestIdentityOIDCProxiesToDex(t *testing.T) {
	dexURL := os.Getenv("DEX_URL")
	if dexURL == "" {
		t.Skip("DEX_URL not set; skipping Dex proxy integration test")
	}

	app := platform.NewApp(platform.Config{ServiceName: "all", RequireAuth: false, DexURL: strings.TrimRight(dexURL, "/")})
	RegisterAll(app)
	server := httptest.NewServer(app)
	defer server.Close()

	// Discovery proxies to Dex and advertises the Dex issuer.
	body, status := getBody(t, server.URL+"/api/v1/oidc/.well-known/openid-configuration")
	if status != http.StatusOK {
		t.Fatalf("discovery status = %d, want 200; body=%s", status, body)
	}
	if !strings.Contains(body, `"issuer"`) || !strings.Contains(body, "/dex") {
		t.Fatalf("discovery did not proxy Dex issuer: %s", body)
	}

	// JWKS proxies to Dex and returns signing keys.
	body, status = getBody(t, server.URL+"/api/v1/oidc/jwks")
	if status != http.StatusOK || !strings.Contains(body, `"keys"`) {
		t.Fatalf("jwks proxy status=%d body=%s", status, body)
	}
}

func getBody(t *testing.T, url string) (string, int) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("get %s: %v", url, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return string(body), resp.StatusCode
}
