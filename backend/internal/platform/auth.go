package platform

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
)

type apiTokenIDContextKey struct{}
type verifiedUserContextKey struct{}

func (a *App) authorized(r *http.Request, route RouteSpec) bool {
	r.Header.Del(headerAPITokenID)
	if a.Config.RequireAuth {
		stripInboundIdentityHeaders(r)
	}
	if !a.Config.RequireAuth || !route.AuthRequired {
		return true
	}
	return a.authorizeBearerHeader(r) || a.authorizeAuthCookies(r) || a.authorizeDevCookie(r) || a.authorizeStaticAPIKey(r, r.Header.Get("X-API-Key"))
}

func (a *App) authorizeBearerHeader(r *http.Request) bool {
	if header := strings.TrimSpace(r.Header.Get("Authorization")); strings.HasPrefix(header, "Bearer ") {
		token := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
		return token != "" && (a.authorizeSessionToken(r, token) || a.authorizeAPIToken(r, token) || a.authorizeDevToken(r, token) || a.authorizeJWT(r, token))
	}
	return false
}

func (a *App) authorizeAuthCookies(r *http.Request) bool {
	for _, name := range []string{"token", "nexuspaas_jwt"} {
		if cookie, err := r.Cookie(name); err == nil && cookie.Value != "" && (a.authorizeSessionToken(r, cookie.Value) || a.authorizeJWT(r, cookie.Value)) {
			return true
		}
	}
	return false
}

func (a *App) authorizeDevCookie(r *http.Request) bool {
	if cookie, err := r.Cookie("dev_token"); err == nil && cookie.Value != "" && a.authorizeDevToken(r, cookie.Value) {
		return true
	}
	return false
}

func (a *App) authorizeSessionToken(r *http.Request, token string) bool {
	if a.remoteIdentityAuthEnabled() {
		return a.authorizeRemoteSessionToken(r, token)
	}
	session, ok := a.Store.Get(r.Context(), "identity-service:sessions", token)
	if !ok || tokenExpired(session.Data) || authBool(session.Data["revoked"]) {
		return false
	}
	if a.tokenRevoked(r.Context(), "session", token) {
		return false
	}
	user, ok := a.Store.Get(r.Context(), "identity-service:users", asString(session.Data["user_id"]))
	if !ok || !authActiveUser(user.Data) {
		return false
	}
	applyAuthHeaders(r, user.Data)
	return true
}

func (a *App) authorizeAPIToken(r *http.Request, token string) bool {
	if a.remoteIdentityAuthEnabled() {
		return a.authorizeRemoteAPIToken(r, token)
	}
	tokenID, ok := ParseUserAPITokenID(token)
	if !ok {
		return false
	}
	record, ok := a.Store.Get(r.Context(), "identity-service:api_tokens", tokenID)
	if !ok || authBool(record.Data["revoked"]) || tokenExpired(record.Data) {
		return false
	}
	if !VerifySecret(asString(record.Data["token_hash"]), token) {
		return false
	}
	if a.tokenRevoked(r.Context(), "api_token", record.ID) {
		return false
	}
	user, ok := a.Store.Get(r.Context(), "identity-service:users", asString(record.Data["user_id"]))
	if !ok || !authActiveUser(user.Data) {
		return false
	}
	applyAuthHeaders(r, user.Data)
	setAPITokenID(r, record.ID)
	r.Header.Set(headerAPITokenID, record.ID)
	if _, ok := a.Store.Update(r.Context(), "identity-service:api_tokens", record.ID, map[string]any{"last_used_at": time.Now().UTC().Format(time.RFC3339)}); !ok {
		slog.Warn("api token last_used_at update skipped", "token_id", record.ID)
	}
	return true
}

// tokenRevoked reports whether a credential has been explicitly revoked via the
// distributed denylist. A backend error fails open (treats the credential as not
// revoked) so a transient revocation-store outage cannot reject every valid
// credential; the error is logged for visibility.
func (a *App) tokenRevoked(ctx context.Context, kind, id string) bool {
	if a.Revocations == nil {
		return false
	}
	revoked, err := a.Revocations.IsRevoked(ctx, kind, id)
	if err != nil {
		slog.Warn("revocation check failed", "kind", kind, "error", err)
		return false
	}
	return revoked
}

// authorizeDevToken verifies a signed local development token and, on success,
// derives the request principal from its verified claims. It is only active in
// non-production runs that configure DEV_AUTH_SIGNING_KEY; otherwise the signer is
// nil and this always denies. Unlike DEV_HEADER_AUTH it never trusts raw client
// identity headers (finding 2).
func (a *App) authorizeDevToken(r *http.Request, token string) bool {
	if a.devTokenSigner == nil {
		return false
	}
	claims, err := a.devTokenSigner.verify(token)
	if err != nil {
		slog.Debug("dev token authorization failed", "error", err)
		return false
	}
	applyAuthHeaders(r, devClaimsUser(claims))
	return true
}

func (a *App) authorizeJWT(r *http.Request, token string) bool {
	if a.jwtVerifier == nil {
		return false
	}
	claims, err := a.jwtVerifier.Verify(r.Context(), token)
	if err != nil {
		slog.Debug("jwt authorization failed", "error", err)
		return false
	}
	if jti := jwtString(claims["jti"]); jti != "" && a.tokenRevoked(r.Context(), "jwt", jti) {
		return false
	}
	applyAuthHeaders(r, jwtClaimsUser(claims))
	return true
}

// RevokeBearer denylists a verifiable JWT by its jti so every replica rejects it
// before its natural expiry. It is the platform-side of OIDC token revocation,
// since Dex exposes no RFC 7009 revocation endpoint. Returns false when the token
// is not a verifiable JWT carrying a jti.
func (a *App) RevokeBearer(ctx context.Context, token string) bool {
	if a.jwtVerifier == nil {
		return false
	}
	claims, err := a.jwtVerifier.Verify(ctx, token)
	if err != nil {
		return false
	}
	jti := jwtString(claims["jti"])
	if jti == "" {
		return false
	}
	ttl := time.Hour
	if expiry, ok, _ := jwtNumericDate(claims["exp"]); ok {
		remaining := time.Until(expiry)
		if remaining <= 0 {
			return true
		}
		ttl = remaining
	}
	if err := a.Revocations.Revoke(ctx, "jwt", jti, ttl); err != nil {
		slog.Warn("jwt revocation write failed", "error", err)
	}
	return true
}

func applyAuthHeaders(r *http.Request, user map[string]any) {
	setVerifiedUser(r, user)
	r.Header.Set(headerUserID, asString(user["id"]))
	r.Header.Set(headerUsername, firstNonEmpty(asString(user["username"]), asString(user["name"])))
	r.Header.Set(headerUserRole, firstNonEmpty(asString(user["role"]), authRoleName(authInt(user["system_role"], 2))))
}

func stripInboundIdentityHeaders(r *http.Request) {
	for _, name := range []string{headerUserID, headerUsername, headerUserRole, headerAdmin} {
		r.Header.Del(name)
	}
}

func setVerifiedUser(r *http.Request, user map[string]any) {
	*r = *r.WithContext(context.WithValue(r.Context(), verifiedUserContextKey{}, cloneMap(user)))
}

func verifiedUser(r *http.Request) (map[string]any, bool) {
	user, ok := r.Context().Value(verifiedUserContextKey{}).(map[string]any)
	return user, ok
}

func (a *App) adminAllowed(r *http.Request) bool {
	user, ok := verifiedUser(r)
	if !ok {
		return false
	}
	if authUserIsAdmin(user) || authRoleGrantsAdmin(user) {
		return true
	}
	roleID := firstNonEmpty(asString(user["role_id"]), asString(user["roleId"]), asString(user["role"]))
	if roleID == "" {
		return false
	}
	for _, role := range a.Store.List(r.Context(), "identity-service:roles") {
		if recordMatchesRole(role, roleID) && authRoleGrantsAdmin(role.Data) {
			return true
		}
	}
	return false
}

func authUserIsAdmin(user map[string]any) bool {
	if authInt(user["system_role"], 2) == 0 {
		return true
	}
	role := strings.ToLower(firstNonEmpty(asString(user["role"]), asString(user["role_id"]), asString(user["roleId"])))
	return role == "admin" || role == "superadmin" || role == "root"
}

func recordMatchesRole(record contracts.Record[map[string]any], roleID string) bool {
	return record.ID == roleID || asString(record.Data["id"]) == roleID || asString(record.Data["name"]) == roleID
}

func authRoleGrantsAdmin(data map[string]any) bool {
	if authBool(data["admin_panel"]) || authBool(data["adminPanel"]) || authBool(data["AdminPanel"]) {
		return true
	}
	if capabilities, ok := data["capabilities"].(map[string]any); ok {
		return authBool(capabilities["admin_panel"]) || authBool(capabilities["adminPanel"]) || authBool(capabilities["AdminPanel"])
	}
	return false
}

func setAPITokenID(r *http.Request, id string) {
	*r = *r.WithContext(context.WithValue(r.Context(), apiTokenIDContextKey{}, id))
}

func APITokenID(r *http.Request) string {
	id, _ := r.Context().Value(apiTokenIDContextKey{}).(string)
	return id
}

// RecordExpired reports whether a record's expires_at timestamp lies in the past.
// Records without an expires_at are treated as non-expiring. Exposed for periodic
// credential cleanup.
func RecordExpired(data map[string]any) bool {
	return tokenExpired(data)
}

func tokenExpired(data map[string]any) bool {
	expiresAt := asString(data["expires_at"])
	if expiresAt == "" {
		return false
	}
	parsed, err := time.Parse(time.RFC3339, expiresAt)
	return err != nil || time.Now().UTC().After(parsed)
}

func authActiveUser(user map[string]any) bool {
	status := strings.ToLower(asString(user["status"]))
	return status == "" || status == "online" || status == "offline"
}

func authRoleName(systemRole int) string {
	switch systemRole {
	case 0:
		return "admin"
	case 1:
		return "manager"
	default:
		return "user"
	}
}

func authInt(value any, fallback int) int {
	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	default:
		return fallback
	}
}

func authBool(value any) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		return strings.EqualFold(v, "true")
	default:
		return false
	}
}

func (a *App) policyAllowed(r *http.Request, route RouteSpec) bool {
	if !principalScopesAllow(r, route) {
		return false
	}
	if route.PolicyBypass {
		return true
	}
	decision, err := a.PDP.Enforce(r.Context(), firstNonEmpty(r.Header.Get(headerUserID), "anonymous"), r.URL.Query().Get("project_id"), route.Resource, route.OperationID)
	return err == nil && decision.Allowed
}
