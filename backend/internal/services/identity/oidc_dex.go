package identity

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/platform"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// dexClient forwards canonical OIDC requests to the configured Dex provider.
var dexClient = &http.Client{Timeout: 10 * time.Second, Transport: otelhttp.NewTransport(http.DefaultTransport)}

// registerDexProxies replaces the fail-closed OIDC handlers with reverse proxies
// to a real Dex provider when DEX_URL is configured. Tokens issued by Dex are
// verified across every service via JWKS, so this turns the local endpoints into
// a real provider-backed surface.
// When DEX_URL is unset the fail-closed handlers registered earlier remain.
func registerDexProxies(app *platform.App) {
	if strings.TrimSpace(app.Config.DexURL) == "" {
		return
	}
	routes := []struct {
		method  string
		pattern string
		dexPath string
	}{
		{http.MethodGet, "/api/v1/oidc/.well-known/openid-configuration", "/.well-known/openid-configuration"},
		{http.MethodGet, "/api/v1/oidc/jwks", "/keys"},
		{http.MethodGet, "/api/v1/oidc/authorize", "/auth"},
		{http.MethodPost, "/api/v1/oidc/token", "/token"},
		{http.MethodGet, "/api/v1/oidc/userinfo", "/userinfo"},
		{http.MethodPost, "/api/v1/oidc/userinfo", "/userinfo"},
	}
	for _, route := range routes {
		app.RegisterCustomHandler(route.method, route.pattern, dexProxy(route.dexPath))
	}
	// Dex exposes no RFC 7009 revocation endpoint, so token revocation is handled
	// locally by denylisting the JWT's jti across replicas (findings 6, 30, 1).
	app.RegisterCustomHandler(http.MethodPost, "/api/v1/oidc/revoke", oidcRevokeViaDenylist)
}

// oidcRevokeViaDenylist implements OAuth 2.0 token revocation (RFC 7009) by adding
// the presented bearer token to the distributed revocation denylist. Per the RFC
// it returns 200 regardless of whether the token was valid, to avoid leaking token
// state, while requiring the token parameter to be present.
func oidcRevokeViaDenylist(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	token := revocationTokenParam(r)
	if token == "" {
		return rawJSON(r, http.StatusBadRequest, map[string]any{"error": "invalid_request", "reason": "token_required"}, nil)
	}
	app.RevokeBearer(r.Context(), token)
	return rawJSON(r, http.StatusOK, map[string]any{"revoked": true}, nil)
}

func revocationTokenParam(r *http.Request) string {
	if err := r.ParseForm(); err == nil {
		if token := strings.TrimSpace(r.PostFormValue("token")); token != "" {
			return token
		}
	}
	if header := strings.TrimSpace(r.Header.Get("Authorization")); strings.HasPrefix(header, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
	}
	return ""
}

func dexProxy(dexPath string) platform.HandlerFunc {
	return func(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
		base := strings.TrimSpace(app.Config.DexURL)
		if base == "" {
			return oidcProviderUnavailable(app, r, platform.RouteSpec{})
		}
		req, status, data := newDexProviderRequest(r, base, dexPath)
		if status != 0 {
			return status, data, nil
		}
		return sendDexProviderRequest(req)
	}
}

func newDexProviderRequest(r *http.Request, base, dexPath string) (*http.Request, int, any) {
	body, status, data := dexRequestBody(r)
	if status != 0 {
		return nil, status, data
	}
	req, err := http.NewRequestWithContext(r.Context(), r.Method, dexTargetURL(base, dexPath, r.URL.RawQuery), body)
	if err != nil {
		return nil, http.StatusBadGateway, map[string]any{"message": "OIDC provider request failed", "reason": "oidc_provider_unreachable"}
	}
	copyDexHeaders(r.Header, req.Header)
	return req, 0, nil
}

func dexTargetURL(base, dexPath, rawQuery string) string {
	target := strings.TrimRight(base, "/") + dexPath
	if rawQuery != "" {
		return target + "?" + rawQuery
	}
	return target
}

func dexRequestBody(r *http.Request) (io.Reader, int, any) {
	if r.Body == nil {
		return nil, 0, nil
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	_ = r.Body.Close()
	if err != nil {
		return nil, http.StatusBadRequest, map[string]any{"message": "OIDC provider request body could not be read", "reason": "invalid_request_body"}
	}
	return bytes.NewReader(raw), 0, nil
}

func copyDexHeaders(src http.Header, dst http.Header) {
	for _, header := range []string{"Authorization", "Content-Type", "Accept"} {
		if value := src.Get(header); value != "" {
			dst.Set(header, value)
		}
	}
}

func sendDexProviderRequest(req *http.Request) (int, any, *platform.Degraded) {
	resp, err := dexClient.Do(req)
	if err != nil {
		return http.StatusBadGateway, map[string]any{"message": "OIDC provider unreachable", "reason": "oidc_provider_unreachable"}, nil
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return http.StatusBadGateway, map[string]any{"message": "OIDC provider response could not be read", "reason": "oidc_provider_unreachable"}, nil
	}
	return resp.StatusCode, platform.RawResponse{
		ContentType: resp.Header.Get("Content-Type"),
		Body:        respBody,
	}, nil
}
