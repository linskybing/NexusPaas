package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

const (
	identityServiceName           = "identity-service"
	internalIdentitySessionAuth   = "/internal/identity/auth/session"
	internalIdentityAPITokenAuth  = "/internal/identity/auth/api-token"
	maxIdentityAuthResponseBytes  = 1 << 20
	identityAuthContentTypeHeader = "application/json"
)

type identityAuthResult struct {
	User       map[string]any `json:"user"`
	APITokenID string         `json:"api_token_id,omitempty"`
}

func (a *App) remoteIdentityAuthEnabled() bool {
	return !a.Config.AllowsService(identityServiceName) &&
		strings.TrimSpace(a.Config.ServiceURLs[identityServiceName]) != "" &&
		strings.TrimSpace(a.Config.ServiceAPIKey) != ""
}

func (a *App) authorizeRemoteSessionToken(r *http.Request, token string) bool {
	result, ok := a.remoteIdentityAuth(r.Context(), internalIdentitySessionAuth, token)
	if !ok {
		return false
	}
	applyAuthHeaders(r, result.User)
	return true
}

func (a *App) authorizeRemoteAPIToken(r *http.Request, token string) bool {
	result, ok := a.remoteIdentityAuth(r.Context(), internalIdentityAPITokenAuth, token)
	if !ok || strings.TrimSpace(result.APITokenID) == "" {
		return false
	}
	applyAuthHeaders(r, result.User)
	setAPITokenID(r, result.APITokenID)
	r.Header.Set(headerAPITokenID, result.APITokenID)
	return true
}

func (a *App) remoteIdentityAuth(ctx context.Context, requestPath, token string) (identityAuthResult, bool) {
	endpoint, err := serviceEndpoint(a.Config.ServiceURLs[identityServiceName], requestPath, "")
	if err != nil {
		slog.Warn("identity auth endpoint invalid", "path", requestPath, "error", err)
		return identityAuthResult{}, false
	}
	body, err := json.Marshal(map[string]string{"token": token})
	if err != nil {
		return identityAuthResult{}, false
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		slog.Warn("identity auth request build failed", "path", requestPath, "error", err)
		return identityAuthResult{}, false
	}
	req.Header.Set("X-Service-Key", a.Config.ServiceAPIKey)
	req.Header.Set(headerContentType, identityAuthContentTypeHeader)

	client := a.identityAuthHTTPClient()
	resp, err := client.Do(req)
	if err != nil {
		slog.Warn("identity auth request failed", "path", requestPath, "error", err)
		return identityAuthResult{}, false
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusNotFound {
		return identityAuthResult{}, false
	}
	if resp.StatusCode != http.StatusOK {
		slog.Warn("identity auth request returned non-OK", "path", requestPath, "status", resp.StatusCode)
		return identityAuthResult{}, false
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxIdentityAuthResponseBytes))
	if err != nil {
		slog.Warn("identity auth response read failed", "path", requestPath, "error", err)
		return identityAuthResult{}, false
	}
	var result identityAuthResult
	if err := decodeEnvelopeData(raw, &result); err != nil {
		slog.Warn("identity auth response decode failed", "path", requestPath, "error", err)
		return identityAuthResult{}, false
	}
	if asString(result.User["id"]) == "" {
		return identityAuthResult{}, false
	}
	return result, true
}

func (a *App) identityAuthHTTPClient() *http.Client {
	timeout := a.Config.AdapterTimeout
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	return &http.Client{Timeout: timeout, Transport: otelhttp.NewTransport(http.DefaultTransport)}
}
