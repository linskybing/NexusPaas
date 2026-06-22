package identity

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

func listAPITokens(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	user, ok := currentUser(app, r)
	if !ok {
		return http.StatusUnauthorized, map[string]any{"message": msgAuthenticationRequired}, nil
	}
	now := time.Now().UTC()
	tokens := []any{}
	for _, token := range authRepository(app).ListActiveAPITokens(r.Context(), textValue(user.Data, "id"), now) {
		tokens = append(tokens, token.Metadata())
	}
	return http.StatusOK, tokens, nil
}

func createAPIToken(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	user, ok := currentUser(app, r)
	if !ok {
		return http.StatusUnauthorized, map[string]any{"message": msgAuthenticationRequired}, nil
	}
	payload, err := decodePayload(r)
	if err != nil {
		return http.StatusBadRequest, map[string]any{"message": msgInvalidInput}, nil
	}
	name := textValue(payload, "name")
	if strings.TrimSpace(name) == "" {
		return http.StatusBadRequest, map[string]any{"message": msgInvalidInput}, nil
	}
	created, status, errData := createAPITokenForUser(app, r, textValue(user.Data, "id"), name)
	if status != http.StatusCreated {
		return status, errData, nil
	}
	return http.StatusCreated, created, nil
}

func revokeAPIToken(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	user, ok := currentUser(app, r)
	if !ok {
		return http.StatusUnauthorized, map[string]any{"message": msgAuthenticationRequired}, nil
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if !revokeTokenForUser(app, r, textValue(user.Data, "id"), id) {
		return http.StatusNotFound, map[string]any{"message": "API token not found"}, nil
	}
	publishAPITokenEvent(app, r, textValue(user.Data, "id"), "api_token_revoked", id, nil)
	return http.StatusOK, nil, nil
}

func revokeCurrentAPIToken(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	user, ok := currentUser(app, r)
	if !ok {
		return http.StatusUnauthorized, map[string]any{"message": msgAuthenticationRequired}, nil
	}
	id := strings.TrimSpace(platform.APITokenID(r))
	if id == "" {
		return http.StatusBadRequest, map[string]any{"message": "current API token required"}, nil
	}
	if !revokeTokenForUser(app, r, textValue(user.Data, "id"), id) {
		return http.StatusNotFound, map[string]any{"message": "API token not found"}, nil
	}
	publishAPITokenEvent(app, r, textValue(user.Data, "id"), "api_token_revoked", id, nil)
	return http.StatusOK, nil, nil
}

func downloadCLICACert(app *platform.App, _ *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	cert := app.Config.CLICACertPEM
	return http.StatusOK, platform.RawResponse{ContentType: "application/x-pem-file", Body: []byte(cert)}, nil
}

func createAPITokenForUser(app *platform.App, r *http.Request, userID, name string) (map[string]any, int, map[string]any) {
	tokenName, ok := normalizeAPITokenName(name)
	if !ok {
		return nil, http.StatusBadRequest, map[string]any{"message": "api token name must be 1-100 characters and must not contain control characters"}
	}
	repo := authRepository(app)
	if repo.CountActiveAPITokens(r.Context(), userID, time.Now().UTC()) >= defaultAPITokenMax {
		return nil, http.StatusTooManyRequests, map[string]any{"message": "too many active API tokens"}
	}
	now := time.Now().UTC()
	created, err := repo.CreateAPIToken(r.Context(), userID, tokenName, now, defaultAPITokenTTL)
	if err != nil {
		if !platform.IsCreateConflict(err) {
			return nil, http.StatusInternalServerError, map[string]any{"message": "api token could not be created"}
		}
		return nil, http.StatusServiceUnavailable, map[string]any{"message": "api token could not be created"}
	}
	response := created.Response()
	publishAPITokenEvent(app, r, userID, "api_token_created", created.ID, map[string]any{"name": tokenName, "expires_at": response["expires_at"]})
	return response, http.StatusCreated, nil
}

func normalizeAPITokenName(name string) (string, bool) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 100 || strings.ContainsFunc(name, unicode.IsControl) {
		return "", false
	}
	return name, true
}

func activeAPITokenCount(app *platform.App, r *http.Request, userID string) int {
	return authRepository(app).CountActiveAPITokens(r.Context(), userID, time.Now().UTC())
}

func revokeTokenForUser(app *platform.App, r *http.Request, userID, tokenID string) bool {
	token, ok := authRepository(app).RevokeAPIToken(r.Context(), userID, tokenID, time.Now().UTC())
	if !ok {
		return false
	}
	revokeCredential(app, r.Context(), "api_token", tokenID, token.ExpiresAt)
	return true
}

// revokeCredential records a credential on the distributed denylist with a TTL
// bounded by its remaining lifetime, so it is rejected on every replica until it
// would have expired anyway. A missing/invalid expiry falls back to a safe
// default window.
func revokeCredential(app *platform.App, ctx context.Context, kind, id, expiresAt string) {
	if app.Revocations == nil || id == "" {
		return
	}
	ttl := 24 * time.Hour
	if expiresAt != "" {
		if parsed, err := time.Parse(time.RFC3339, expiresAt); err == nil {
			remaining := time.Until(parsed)
			if remaining <= 0 {
				return
			}
			ttl = remaining
		}
	}
	if err := app.Revocations.Revoke(ctx, kind, id, ttl); err != nil {
		slog.Warn("credential revocation write failed", "kind", kind, "id", id, "error", err)
	}
}

func apiTokenMetadata(data map[string]any) map[string]any {
	out := map[string]any{
		"id":           textValue(data, "id"),
		"name":         textValue(data, "name"),
		"token_prefix": textValue(data, "token_prefix"),
		"expires_at":   textValue(data, "expires_at"),
		"created_at":   textValue(data, "created_at"),
	}
	if lastUsedAt := textValue(data, "last_used_at"); lastUsedAt != "" {
		out["last_used_at"] = lastUsedAt
	}
	return out
}

func tokenPrefix(token string) string {
	if len(token) <= 12 {
		return token
	}
	return token[:12]
}

func publishAPITokenEvent(app *platform.App, r *http.Request, userID, action, tokenID string, data map[string]any) {
	payload := map[string]any{
		"user_id":       userID,
		"action":        action,
		"resource_type": "auth",
		"resource":      "api_token",
		"resource_id":   tokenID,
		"success":       true,
		"source_ip":     requestIPForApp(app, r),
		"user_agent":    r.UserAgent(),
	}
	if data != nil {
		payload["data"] = data
	}
	publish(app, r, "AuditEvent", payload)
}
