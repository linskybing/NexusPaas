package identity

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

func TestIdentityRegisterLoginRefreshLogoutDirect(t *testing.T) {
	app := newIdentityTestApp()

	code, data, _ := register(app, identityRequest(http.MethodPost, "/api/v1/register", `{"username":"al","password":"short"}`), platform.RouteSpec{})
	assertIdentityStatus(t, code, data, http.StatusBadRequest)
	code, data, _ = register(app, identityRequest(http.MethodPost, "/api/v1/register", `{"username":"alice","password":"correct-password","email":"alice@example.com","full_name":"Alice A"}`), platform.RouteSpec{})
	assertIdentityStatus(t, code, data, http.StatusOK)
	code, data, _ = register(app, identityRequest(http.MethodPost, "/api/v1/register", `{"username":"alice","password":"correct-password"}`), platform.RouteSpec{})
	assertIdentityStatus(t, code, data, http.StatusConflict)

	code, data, _ = login(app, identityRequest(http.MethodPost, pathLogin, `{"username":"alice","password":"wrong-password"}`), platform.RouteSpec{})
	assertIdentityStatus(t, code, data, http.StatusUnauthorized)
	code, data, _ = login(app, identityRequest(http.MethodPost, pathLogin, `{"username":"alice","password":"correct-password"}`), platform.RouteSpec{})
	assertIdentityStatus(t, code, data, http.StatusOK)
	loginData := identityRawData(t, data)
	token := loginData["token"].(string)
	refresh := loginData["refresh_token"].(string)
	if !strings.HasPrefix(token, "access.US") || !strings.HasPrefix(refresh, "refresh.") {
		t.Fatalf("login data = %#v, want issued access and refresh tokens", loginData)
	}

	code, data, _ = refreshToken(app, identityRequest(http.MethodPost, "/api/v1/refresh", `{}`), platform.RouteSpec{})
	assertIdentityStatus(t, code, data, http.StatusBadRequest)
	code, data, _ = refreshToken(app, identityRequest(http.MethodPost, "/api/v1/refresh", `{"refresh_token":"`+refresh+`"}`), platform.RouteSpec{})
	assertIdentityStatus(t, code, data, http.StatusOK)
	refreshed := identityRawData(t, data)
	if refreshed["token"] == token || refreshed["refresh_token"] == refresh {
		t.Fatalf("refreshed tokens = %#v, want rotation", refreshed)
	}
	code, data, _ = refreshToken(app, identityRequest(http.MethodPost, "/api/v1/refresh", `{"refresh_token":"`+refresh+`"}`), platform.RouteSpec{})
	assertIdentityStatus(t, code, data, http.StatusUnauthorized)
	rotatedToken := refreshed["token"].(string)
	rotatedRefresh := refreshed["refresh_token"].(string)
	code, data, _ = refreshToken(app, identityRequest(http.MethodPost, "/api/v1/refresh", `{"refresh_token":"`+rotatedRefresh+`"}`), platform.RouteSpec{})
	assertIdentityStatus(t, code, data, http.StatusOK)
	refreshed = identityRawData(t, data)
	if refreshed["token"] == rotatedToken || refreshed["refresh_token"] == rotatedRefresh {
		t.Fatalf("second refresh tokens = %#v, want another rotation", refreshed)
	}

	logoutReq := identityRequest(http.MethodPost, "/api/v1/logout", "")
	logoutReq.AddCookie(&http.Cookie{Name: "token", Value: refreshed["token"].(string)})
	logoutReq.AddCookie(&http.Cookie{Name: "refresh_token", Value: refreshed["refresh_token"].(string)})
	code, data, _ = logout(app, logoutReq, platform.RouteSpec{})
	assertIdentityStatus(t, code, data, http.StatusOK)
	user, ok := app.Store.Get(context.Background(), usersResource, "US2600001")
	if !ok || user.Data["status"] != "offline" {
		t.Fatalf("user after logout = %#v found=%v, want offline", user.Data, ok)
	}
}

func TestIdentityCaptchaAndCLILoginDirect(t *testing.T) {
	app := newIdentityTestApp()
	code, data, _ := register(app, identityRequest(http.MethodPost, "/api/v1/register", `{"username":"bob","password":"correct-password"}`), platform.RouteSpec{})
	assertIdentityStatus(t, code, data, http.StatusOK)

	code, data, _ = getCaptcha(app, identityRequest(http.MethodGet, "/api/v1/captcha", ""), platform.RouteSpec{})
	assertIdentityStatus(t, code, data, http.StatusOK)
	captcha := data.(map[string]any)
	if captcha["answer"] == "" || !strings.HasPrefix(captcha["image"].(string), "data:image/png;base64,") {
		t.Fatalf("captcha = %#v, want answer and data URL image", captcha)
	}
	badCaptchaBody := `{"username":"bob","password":"correct-password","captcha_id":"` + captcha["captcha_id"].(string) + `","captcha_answer":"0000"}`
	code, data, _ = login(app, identityRequest(http.MethodPost, pathLogin, badCaptchaBody), platform.RouteSpec{})
	assertIdentityStatus(t, code, data, http.StatusUnauthorized)

	code, data, _ = getCaptcha(app, identityRequest(http.MethodGet, "/api/v1/captcha", ""), platform.RouteSpec{})
	assertIdentityStatus(t, code, data, http.StatusOK)
	captcha = data.(map[string]any)
	goodCaptchaBody := `{"username":"bob","password":"correct-password","captcha_id":"` + captcha["captcha_id"].(string) + `","captcha_answer":"` + captcha["answer"].(string) + `"}`
	code, data, _ = login(app, identityRequest(http.MethodPost, pathLogin, goodCaptchaBody), platform.RouteSpec{})
	assertIdentityStatus(t, code, data, http.StatusOK)

	code, data, _ = cliLogin(app, identityRequest(http.MethodPost, pathCLILogin, `{"username":"bob"}`), platform.RouteSpec{})
	assertIdentityStatus(t, code, data, http.StatusBadRequest)
	code, data, _ = cliLogin(app, identityRequest(http.MethodPost, pathCLILogin, `{"username":"bob","password":"correct-password","name":"laptop"}`), platform.RouteSpec{})
	assertIdentityStatus(t, code, data, http.StatusOK)
	if !strings.HasPrefix(data.(map[string]any)["token"].(string), "nexuspaas_") {
		t.Fatalf("cli login = %#v, want API token", data)
	}
}

func TestIdentityAPITokenLifecycleDirect(t *testing.T) {
	app := newIdentityTestApp()
	seedIdentityUser(t, app, "US1", "dana")

	code, data, _ := listAPITokens(app, identityUserRequest(http.MethodGet, "/api/v1/me/api-tokens", "", ""), platform.RouteSpec{})
	assertIdentityStatus(t, code, data, http.StatusUnauthorized)
	code, data, _ = createAPIToken(app, identityUserRequest(http.MethodPost, "/api/v1/me/api-tokens", `{}`, "US1"), platform.RouteSpec{})
	assertIdentityStatus(t, code, data, http.StatusBadRequest)
	code, data, _ = createAPIToken(app, identityUserRequest(http.MethodPost, "/api/v1/me/api-tokens", `{"name":"notebook"}`, "US1"), platform.RouteSpec{})
	assertIdentityStatus(t, code, data, http.StatusCreated)
	created := data.(map[string]any)
	tokenID := created["id"].(string)
	if !strings.HasPrefix(created["token"].(string), "nexuspaas_") || created["token_hash"] != nil {
		t.Fatalf("created api token = %#v, want raw token without hash leak", created)
	}

	code, data, _ = listAPITokens(app, identityUserRequest(http.MethodGet, "/api/v1/me/api-tokens", "", "US1"), platform.RouteSpec{})
	assertIdentityStatus(t, code, data, http.StatusOK)
	if tokens := data.([]any); len(tokens) != 1 || tokens[0].(map[string]any)["id"] != tokenID {
		t.Fatalf("listed tokens = %#v, want created token metadata", data)
	}
	revokeReq := identityUserRequest(http.MethodDelete, "/api/v1/me/api-tokens/"+tokenID, "", "US1")
	revokeReq.SetPathValue("id", tokenID)
	code, data, _ = revokeAPIToken(app, revokeReq, platform.RouteSpec{})
	assertIdentityStatus(t, code, data, http.StatusOK)
	code, data, _ = revokeAPIToken(app, revokeReq, platform.RouteSpec{})
	assertIdentityStatus(t, code, data, http.StatusNotFound)
}

func TestIdentityAPITokenCurrentRevocationThroughMiddleware(t *testing.T) {
	serviceKey := testServiceKey(t)
	app := newRoutedIdentityAuthTestApp(serviceKey)
	seedIdentityUser(t, app, "US1", "dana")
	seedIdentitySession(t, app, "session-1", "US1")

	created := identityResponseMap(t, serveIdentityHTTP(t, app, http.MethodPost, "/api/v1/me/api-tokens", `{"name":"notebook"}`, map[string]string{"Cookie": "token=session-1"}, http.StatusCreated))
	rawToken := created["token"].(string)
	tokenID := created["id"].(string)
	if rawToken == "" || tokenID == "" || created["token_hash"] != nil {
		t.Fatalf("created api token = %#v, want raw token without hash leak", created)
	}
	if got := activeAPITokenCount(app, identityRequest(http.MethodGet, "/api/v1/me/api-tokens", ""), "US1"); got != 1 {
		t.Fatalf("active API token count = %d, want 1", got)
	}

	serveIdentityHTTP(t, app, http.MethodGet, "/api/v1/me/api-tokens", "", map[string]string{"Authorization": "Bearer " + rawToken}, http.StatusOK)
	postInternalIdentityAuth(t, app, serviceKey, "/internal/identity/auth/api-token", rawToken, http.StatusOK)

	// The current-token endpoint must rely on the platform-authenticated API token
	// context, not a caller-supplied header.
	serveIdentityHTTP(t, app, http.MethodDelete, "/api/v1/me/api-tokens/current", "", map[string]string{
		"Cookie":         "token=session-1",
		"X-API-Token-ID": tokenID,
	}, http.StatusBadRequest)

	serveIdentityHTTP(t, app, http.MethodDelete, "/api/v1/me/api-tokens/current", "", map[string]string{"Authorization": "Bearer " + rawToken}, http.StatusOK)
	stored, ok := app.Store.Get(context.Background(), apiTokensResource, tokenID)
	if !ok || stored.Data["revoked"] != true || stored.Data["revoked_at"] == nil {
		t.Fatalf("revoked token record = %#v found=%v, want revoked metadata", stored.Data, ok)
	}
	if revoked, err := app.Revocations.IsRevoked(context.Background(), "api_token", tokenID); err != nil || !revoked {
		t.Fatalf("revocation denylist token=%s revoked=%v err=%v, want revoked", tokenID, revoked, err)
	}
	serveIdentityHTTP(t, app, http.MethodGet, "/api/v1/me/api-tokens", "", map[string]string{"Authorization": "Bearer " + rawToken}, http.StatusUnauthorized)
}

func TestIdentityInternalAPITokenAuthRejectsDenylistedCredential(t *testing.T) {
	serviceKey := testServiceKey(t)
	app := newRoutedIdentityAuthTestApp(serviceKey)
	seedIdentityUser(t, app, "US1", "dana")
	seedIdentitySession(t, app, "session-1", "US1")

	created := identityResponseMap(t, serveIdentityHTTP(t, app, http.MethodPost, "/api/v1/me/api-tokens", `{"name":"automation"}`, map[string]string{"Cookie": "token=session-1"}, http.StatusCreated))
	rawToken := created["token"].(string)
	tokenID := created["id"].(string)
	if err := app.Revocations.Revoke(context.Background(), "api_token", tokenID, time.Hour); err != nil {
		t.Fatal(err)
	}

	postInternalIdentityAuth(t, app, serviceKey, "/internal/identity/auth/api-token", rawToken, http.StatusUnauthorized)
	serveIdentityHTTP(t, app, http.MethodGet, "/api/v1/me/api-tokens", "", map[string]string{"Authorization": "Bearer " + rawToken}, http.StatusUnauthorized)
	stored, ok := app.Store.Get(context.Background(), apiTokensResource, tokenID)
	if !ok || stored.Data["revoked"] == true {
		t.Fatalf("stored token = %#v found=%v, want active store record rejected only by denylist", stored.Data, ok)
	}
}

func TestIdentityOIDCRevokeViaDenylistParameters(t *testing.T) {
	app := newIdentityTestApp()

	code, data, degraded := oidcRevokeViaDenylist(app, identityRequest(http.MethodPost, "/api/v1/oidc/revoke", ""), platform.RouteSpec{})
	if degraded != nil || code != http.StatusBadRequest {
		t.Fatalf("missing token revoke status=%d degraded=%v data=%#v, want 400", code, degraded, data)
	}

	for _, tc := range []struct {
		name string
		req  *http.Request
	}{
		{name: "form token", req: identityFormRequest(http.MethodPost, "/api/v1/oidc/revoke", "token=opaque-token")},
		{name: "bearer token", req: func() *http.Request {
			req := identityRequest(http.MethodPost, "/api/v1/oidc/revoke", "")
			req.Header.Set("Authorization", "Bearer opaque-token")
			return req
		}()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, data, degraded := oidcRevokeViaDenylist(app, tc.req, platform.RouteSpec{})
			if degraded != nil || code != http.StatusOK {
				t.Fatalf("status=%d degraded=%v data=%#v, want 200", code, degraded, data)
			}
			raw := data.(platform.RawResponse)
			if !strings.Contains(string(raw.Body), `"revoked":true`) {
				t.Fatalf("body = %s, want revoked response", raw.Body)
			}
		})
	}
}

func TestIdentityRequireAuthSelfServiceFallbackForPlatformAuthenticatedUser(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: serviceName, RequireAuth: true})

	anonymousReq := identityRequest(http.MethodPut, "/api/v1/users/REMOTE/settings", `{"settings":{"theme":"dark"}}`)
	anonymousReq.SetPathValue("id", "REMOTE")
	code, data, _ := updateUserSettings(app, anonymousReq, platform.RouteSpec{})
	assertIdentityStatus(t, code, data, http.StatusUnauthorized)

	platformReq := identityUserRequest(http.MethodPut, "/api/v1/users/REMOTE/settings", `{"settings":{"theme":"dark"}}`, "REMOTE")
	platformReq.SetPathValue("id", "REMOTE")
	code, data, _ = updateUserSettings(app, platformReq, platform.RouteSpec{})
	assertIdentityStatus(t, code, data, http.StatusNotFound)
	if got := data.(map[string]any)["message"]; got != msgUserNotFound {
		t.Fatalf("fallback response = %#v, want user-not-found after platform auth", data)
	}
}

func TestIdentityRegisterWiresDexProxyAndAuthCleanup(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: "all", DexURL: "http://127.0.0.1:1", ExternalURLs: map[string]string{}})
	Register(app)

	if app.CustomHandlers["POST /api/v1/oidc/revoke"] == nil {
		t.Fatalf("dex revoke proxy handlers not registered: %#v", app.CustomHandlers)
	}
	if app.CustomHandlers["POST /api/v1/oidc/token"] == nil {
		t.Fatalf("dex token proxy handler not registered: %#v", app.CustomHandlers)
	}
	if app.CustomHandlers["POST /revoke"] != nil || app.CustomHandlers["POST /oauth/token"] != nil || app.CustomHandlers["POST /device_authorization"] != nil {
		t.Fatalf("legacy dex proxy handlers registered: %#v", app.CustomHandlers)
	}
	if !containsString(app.MaintenanceTaskNames(), "identity-auth-cleanup") {
		t.Fatalf("maintenance tasks = %v, want identity-auth-cleanup", app.MaintenanceTaskNames())
	}

	if _, err := app.Store.Create(context.Background(), sessionsResource, map[string]any{
		"id":         "expired-session",
		"user_id":    "US1",
		"expires_at": time.Now().UTC().Add(-time.Minute).Format(time.RFC3339),
	}); err != nil {
		t.Fatal(err)
	}
	app.RunMaintenanceOnce(context.Background(), time.Minute)
	if _, ok := app.Store.Get(context.Background(), sessionsResource, "expired-session"); ok {
		t.Fatal("registered auth cleanup did not remove expired session")
	}
}

func TestIdentityAPITokenQuotaAndValidationBranchesDirect(t *testing.T) {
	app := newIdentityTestApp()
	seedIdentityUser(t, app, "US1", "dana")
	req := identityUserRequest(http.MethodPost, "/api/v1/me/api-tokens", "", "US1")

	if _, status, data := createAPITokenForUser(app, req, "US1", "bad\nname"); status != http.StatusBadRequest || data["message"] == "" {
		t.Fatalf("invalid token name status=%d data=%#v, want 400", status, data)
	}
	for i := 0; i < defaultAPITokenMax; i++ {
		if _, status, data := createAPITokenForUser(app, req, "US1", "token-"+string(rune('a'+i))); status != http.StatusCreated {
			t.Fatalf("token %d status=%d data=%#v, want created", i, status, data)
		}
	}
	if _, status, data := createAPITokenForUser(app, req, "US1", "overflow"); status != http.StatusTooManyRequests || data["message"] == "" {
		t.Fatalf("quota overflow status=%d data=%#v, want 429", status, data)
	}
}

func TestIdentityLoginCaptchaAndLockoutEdgeBranchesDirect(t *testing.T) {
	app := newIdentityTestApp()
	seedIdentityUser(t, app, "US1", "dana")
	req := identityRequest(http.MethodPost, pathLogin, `{"username":"dana","password":"correct-password"}`)
	id := loginFailureID("dana", requestIPForApp(app, req))
	if _, err := app.Store.Create(context.Background(), loginFailuresResource, map[string]any{
		"id":       id,
		"username": "dana",
		"ip":       requestIPForApp(app, req),
		"failures": defaultLoginMaxFailed,
	}); err != nil {
		t.Fatal(err)
	}
	code, data, _ := login(app, req, platform.RouteSpec{})
	assertIdentityStatus(t, code, data, http.StatusUnauthorized)
	if data.(map[string]any)["message"] != msgCaptchaRequired {
		t.Fatalf("captcha-required response = %#v", data)
	}

	lockReq := identityRequest(http.MethodPost, pathLogin, `{"username":"locked","password":"correct-password"}`)
	lockID := loginFailureID("locked", requestIPForApp(app, lockReq))
	if _, err := app.Store.Create(context.Background(), loginFailuresResource, map[string]any{
		"id":           lockID,
		"username":     "locked",
		"ip":           requestIPForApp(app, lockReq),
		"failures":     defaultLoginMaxFailed + 1,
		"locked_until": "not-a-time",
	}); err != nil {
		t.Fatal(err)
	}
	if !loginLocked(app, lockReq, "locked") {
		t.Fatal("invalid locked_until should fail closed as locked")
	}
	if _, ok := app.Store.Update(context.Background(), loginFailuresResource, lockID, map[string]any{"locked_until": time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)}); !ok {
		t.Fatal("failed to update lockout record")
	}
	if loginLocked(app, lockReq, "locked") {
		t.Fatal("expired locked_until should not remain locked")
	}
	if _, ok := app.Store.Get(context.Background(), loginFailuresResource, lockID); ok {
		t.Fatal("expired lockout record should be cleared")
	}
}

func TestIdentityAdminEdgeBranchesAndCredentialRevocationDirect(t *testing.T) {
	app := newIdentityAdminTestApp(t)

	otherReq := identityAdminRequest(http.MethodGet, "/api/v1/users/ADMIN", "", "U1")
	otherReq.SetPathValue("id", "ADMIN")
	code, data, _ := getUserByID(app, otherReq, platform.RouteSpec{})
	assertIdentityAdminStatus(t, code, data, http.StatusForbidden)

	missingReq := identityAdminRequest(http.MethodGet, "/api/v1/users/missing", "", "ADMIN")
	missingReq.SetPathValue("id", "missing")
	code, data, _ = getUserByID(app, missingReq, platform.RouteSpec{})
	assertIdentityAdminStatus(t, code, data, http.StatusNotFound)

	badJSONReq := identityAdminRequest(http.MethodPut, "/api/v1/users/U1", `{bad`, "ADMIN")
	badJSONReq.SetPathValue("id", "U1")
	code, data, _ = updateUser(app, badJSONReq, platform.RouteSpec{})
	assertIdentityAdminStatus(t, code, data, http.StatusBadRequest)

	emptyUpdateReq := identityAdminRequest(http.MethodPut, "/api/v1/users/U1", `{"unknown":true}`, "U1")
	emptyUpdateReq.SetPathValue("id", "U1")
	code, data, _ = updateUser(app, emptyUpdateReq, platform.RouteSpec{})
	assertIdentityAdminStatus(t, code, data, http.StatusBadRequest)

	adminUpdateReq := identityAdminRequest(http.MethodPut, "/api/v1/users/U1", `{"status":"disabled","system_role":1,"capabilities":{"adminPanel":true}}`, "ADMIN")
	adminUpdateReq.SetPathValue("id", "U1")
	code, data, _ = updateUser(app, adminUpdateReq, platform.RouteSpec{})
	assertIdentityAdminStatus(t, code, data, http.StatusOK)
	updated, _ := app.Store.Get(context.Background(), usersResource, "U1")
	if updated.Data["status"] != "disabled" || updated.Data["system_role"] != 1 || updated.Data["role"] != "manager" {
		t.Fatalf("admin update = %#v, want status plus manager role", updated.Data)
	}

	code, data, _ = batchResetPassword(app, identityAdminRequest(http.MethodPut, "/api/v1/users/batch/password", `{"ids":["U1"],"password":"123"}`, "ADMIN"), platform.RouteSpec{})
	assertIdentityAdminStatus(t, code, data, http.StatusBadRequest)
	code, data, _ = batchUpdateRole(app, identityAdminRequest(http.MethodPut, "/api/v1/users/batch/role", `{bad`, "ADMIN"), platform.RouteSpec{})
	assertIdentityAdminStatus(t, code, data, http.StatusBadRequest)
	code, data, _ = batchDeleteUsers(app, identityAdminRequest(http.MethodDelete, "/api/v1/users/batch", `{bad`, "ADMIN"), platform.RouteSpec{})
	assertIdentityAdminStatus(t, code, data, http.StatusBadRequest)

	deleteApp := newIdentityAdminTestApp(t)
	if _, err := deleteApp.Store.Create(context.Background(), apiTokensResource, map[string]any{
		"id":         "AT1",
		"user_id":    "U1",
		"name":       "ci",
		"token_hash": platform.HashSecret("nexuspaas_delete"),
		"expires_at": time.Now().UTC().Add(time.Hour).Format(time.RFC3339),
		"revoked":    false,
	}); err != nil {
		t.Fatal(err)
	}
	deleteReq := identityAdminRequest(http.MethodDelete, "/api/v1/users/U1", "", "ADMIN")
	deleteReq.SetPathValue("id", "U1")
	code, data, _ = deleteUser(deleteApp, deleteReq, platform.RouteSpec{})
	assertIdentityAdminStatus(t, code, data, http.StatusOK)
	if revoked, err := deleteApp.Revocations.IsRevoked(context.Background(), "api_token", "AT1"); err != nil || !revoked {
		t.Fatalf("deleted user api token revoked=%v err=%v, want revoked", revoked, err)
	}
}

func TestIdentityPrincipalRepositoryNilAndNextIDBranchesDirect(t *testing.T) {
	nilRepo := principalRepository(nil)
	if nilRepo.UserResourceName() != usersResource || nilRepo.RoleResourceName() != rolesResource {
		t.Fatalf("resource names = %q/%q", nilRepo.UserResourceName(), nilRepo.RoleResourceName())
	}
	if nilRepo.NextUserID() != "" || len(nilRepo.ListUsers(context.Background())) != 0 {
		t.Fatalf("nil repo should be inert")
	}
	if _, ok := nilRepo.GetUser(context.Background(), "US1"); ok {
		t.Fatal("nil repo returned a user")
	}

	app := newIdentityTestApp()
	if id := nextID(app, identityRequest(http.MethodGet, "/", ""), usersResource, "US", 2600001); id != "US2600001" {
		t.Fatalf("nextID = %q, want US2600001", id)
	}
}

func TestIdentityAPITokenMetadataAndExpiryHelpersDirect(t *testing.T) {
	metadata := apiTokenMetadata(map[string]any{
		"id":           "AT1",
		"name":         "ci",
		"token_prefix": "nexuspaas_abc",
		"token_hash":   "must-not-leak",
		"expires_at":   "soon",
		"created_at":   "now",
		"last_used_at": "later",
	})
	if metadata["last_used_at"] != "later" || metadata["token_hash"] != nil {
		t.Fatalf("metadata = %#v, want last_used_at without token_hash", metadata)
	}
	if tokenExpired(map[string]any{}) {
		t.Fatal("token without expiry should not be expired")
	}
	if !tokenExpired(map[string]any{"expires_at": "not-a-time"}) {
		t.Fatal("invalid expiry should be treated as expired")
	}
}

func TestIdentityGenericHelperBranchesDirect(t *testing.T) {
	if got := intValue(map[string]any{"n": int64(7)}, "n", 0); got != 7 {
		t.Fatalf("int64 intValue = %d, want 7", got)
	}
	if got := intValue(map[string]any{"n": json.Number("8")}, "n", 0); got != 8 {
		t.Fatalf("json.Number intValue = %d, want 8", got)
	}
	if got := textValue(map[string]any{"m": time.January}, "m"); got != "January" {
		t.Fatalf("Stringer textValue = %q, want January", got)
	}
	if maps := mapSlice([]any{map[string]any{"id": "one"}, "skip"}); len(maps) != 1 || maps[0]["id"] != "one" {
		t.Fatalf("mapSlice = %#v, want one map", maps)
	}
	if ids := firstUserIDs(map[string]any{"user_id": "US1"}); len(ids) != 1 || ids[0] != "US1" {
		t.Fatalf("firstUserIDs = %#v, want US1", ids)
	}
}

func TestIdentityUserPagingAndSettingsDirect(t *testing.T) {
	app := newIdentityTestApp()
	seedIdentityUser(t, app, "ADMIN", "admin")
	seedIdentityUser(t, app, "US1", "alice")
	_, _ = app.Store.Update(context.Background(), usersResource, "ADMIN", map[string]any{"role": "admin", "system_role": 0})

	code, data, _ := listUsersPaging(app, identityUserRequest(http.MethodGet, "/api/v1/users?page=2&limit=1", "", "ADMIN"), platform.RouteSpec{})
	assertIdentityStatus(t, code, data, http.StatusOK)
	page := data.(map[string]any)
	if page["total"] != 2 || page["page"] != 2 || len(page["list"].([]map[string]any)) != 1 {
		t.Fatalf("paged users = %#v, want second page with one user", page)
	}

	settingsReq := identityUserRequest(http.MethodGet, "/api/v1/users/US1/settings", "", "US1")
	settingsReq.SetPathValue("id", "US1")
	code, data, _ = getUserSettings(app, settingsReq, platform.RouteSpec{})
	assertIdentityStatus(t, code, data, http.StatusOK)
	if len(data.(map[string]any)) != 0 {
		t.Fatalf("initial settings = %#v, want empty", data)
	}

	updateReq := identityUserRequest(http.MethodPut, "/api/v1/users/US1/settings", `{"theme":"dark"}`, "US1")
	updateReq.SetPathValue("id", "US1")
	code, data, _ = updateUserSettings(app, updateReq, platform.RouteSpec{})
	assertIdentityStatus(t, code, data, http.StatusOK)
	if data.(map[string]any)["theme"] != "dark" {
		t.Fatalf("updated settings = %#v, want theme", data)
	}
}

func TestIdentityOIDCFailClosedDirect(t *testing.T) {
	app := newIdentityTestApp()
	cases := []struct {
		name   string
		call   func() (int, any, *platform.Degraded)
		status int
	}{
		{name: "login form missing", call: func() (int, any, *platform.Degraded) {
			return oidcLoginForm(app, identityRequest(http.MethodGet, "/api/v1/oidc/login", ""), platform.RouteSpec{})
		}, status: http.StatusBadRequest},
		{name: "login post valid", call: func() (int, any, *platform.Degraded) {
			return oidcLogin(app, identityRequest(http.MethodPost, "/api/v1/oidc/login", `{"auth_request_id":"req","username":"alice","password":"secret"}`), platform.RouteSpec{})
		}, status: http.StatusServiceUnavailable},
		{name: "token form valid", call: func() (int, any, *platform.Degraded) {
			req := identityFormRequest(http.MethodPost, "/api/v1/oidc/token", "grant_type=authorization_code&client_id=grafana&code=code-1")
			return oidcToken(app, req, platform.RouteSpec{})
		}, status: http.StatusServiceUnavailable},
		{name: "revoke missing token", call: func() (int, any, *platform.Degraded) {
			return oidcRevoke(app, identityRequest(http.MethodPost, "/api/v1/oidc/revoke", `{}`), platform.RouteSpec{})
		}, status: http.StatusBadRequest},
		{name: "authorize valid", call: func() (int, any, *platform.Degraded) {
			return oidcAuthorize(app, identityRequest(http.MethodGet, "/api/v1/oidc/authorize?client_id=grafana&response_type=code&redirect_uri=https://grafana.example/callback", ""), platform.RouteSpec{})
		}, status: http.StatusServiceUnavailable},
		{name: "userinfo bearer", call: func() (int, any, *platform.Degraded) {
			req := identityRequest(http.MethodGet, "/api/v1/oidc/userinfo", "")
			req.Header.Set("Authorization", "Bearer access-1")
			return oidcUserInfo(app, req, platform.RouteSpec{})
		}, status: http.StatusServiceUnavailable},
		{name: "callback valid", call: func() (int, any, *platform.Degraded) {
			return oidcCallback(app, identityRequest(http.MethodGet, "/api/v1/oidc/callback?code=code-1&state=state-1", ""), platform.RouteSpec{})
		}, status: http.StatusServiceUnavailable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, data, degraded := tc.call()
			if degraded != nil || code != tc.status {
				t.Fatalf("status=%d degraded=%v data=%#v, want %d", code, degraded, data, tc.status)
			}
		})
	}
}

func TestIdentityOIDCCallbackRejectsInvalidState(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: "all", DexURL: "http://dex.test/dex"})
	Register(app)
	req := identityRequest(http.MethodGet, "/api/v1/oidc/callback?code=code-1&state=state-1", "")

	code, data, degraded := oidcCallback(app, req, platform.RouteSpec{})
	if degraded != nil || code != http.StatusBadRequest {
		t.Fatalf("callback status=%d degraded=%v data=%#v, want 400", code, degraded, data)
	}
}

func TestIdentityOIDCStartIssuesStateCookie(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: "all", DexURL: "http://dex.test/dex"})
	Register(app)
	req := identityRequest(http.MethodGet, "/api/v1/oidc/start", "")
	req.Host = "console.test"

	code, data, degraded := oidcStart(app, req, platform.RouteSpec{})
	if degraded != nil || code != http.StatusFound {
		t.Fatalf("start status=%d degraded=%v data=%#v, want 302", code, degraded, data)
	}
	raw := data.(platform.RawResponse)
	location := raw.Headers["Location"]
	if !strings.HasPrefix(location, "/api/v1/oidc/authorize?") {
		t.Fatalf("Location = %q, want authorize path", location)
	}
	parsed, err := url.Parse(location)
	if err != nil {
		t.Fatalf("parse Location: %v", err)
	}
	state := parsed.Query().Get("state")
	if state == "" || parsed.Query().Get("redirect_uri") != "http://console.test/api/v1/oidc/callback" {
		t.Fatalf("authorize query = %q, want state and redirect_uri", parsed.RawQuery)
	}
	cookies := strings.Join(raw.HeaderValues["Set-Cookie"], "\n")
	for _, want := range []string{oidcStateCookieName + "=", "HttpOnly", "Path=/", "Max-Age=300", "SameSite=Lax"} {
		if !strings.Contains(cookies, want) {
			t.Fatalf("state cookie attributes = %q, missing %s", cookies, want)
		}
	}
}

func TestOIDCRedirectURIUsesValidatedForwardedOrigin(t *testing.T) {
	req := identityRequest(http.MethodGet, "/api/v1/oidc/start", "")
	req.Host = "identity-service:8080"
	req.Header.Set("X-Forwarded-Host", "localhost:8080")
	req.Header.Set("X-Forwarded-Proto", "https")

	if got, want := oidcRedirectURI(req), "https://localhost:8080/api/v1/oidc/callback"; got != want {
		t.Fatalf("redirect_uri = %q, want %q", got, want)
	}
}

func TestOIDCRedirectURIRejectsInvalidForwardedOrigin(t *testing.T) {
	req := identityRequest(http.MethodGet, "/api/v1/oidc/start", "")
	req.Host = "identity-service:8080"
	req.Header.Add("X-Forwarded-Host", "localhost:8080")
	req.Header.Add("X-Forwarded-Host", "evil.test")
	req.Header.Set("X-Forwarded-Proto", "ftp")

	if got, want := oidcRedirectURI(req), "http://identity-service:8080/api/v1/oidc/callback"; got != want {
		t.Fatalf("redirect_uri = %q, want %q", got, want)
	}
}

func TestIdentityOIDCCallbackIssuesSessionCookies(t *testing.T) {
	dex := newOIDCTestTokenServer(t)
	defer dex.Close()
	app := platform.NewApp(platform.Config{ServiceName: "all", DexURL: dex.URL})
	Register(app)
	seedIdentityUser(t, app, "US1", "admin")
	req := identityRequest(http.MethodGet, "/api/v1/oidc/callback?code=code-1&state=state-1", "")
	req.Host = "console.test"
	req.AddCookie(&http.Cookie{Name: oidcStateCookieName, Value: "state-1"})

	code, data, degraded := oidcCallback(app, req, platform.RouteSpec{})
	if degraded != nil || code != http.StatusFound {
		t.Fatalf("callback status=%d degraded=%v data=%#v, want 302", code, degraded, data)
	}
	raw := data.(platform.RawResponse)
	if raw.Headers["Location"] != "/ui/?auth=oidc" {
		t.Fatalf("Location = %q, want /ui/?auth=oidc", raw.Headers["Location"])
	}
	cookies := strings.Join(raw.HeaderValues["Set-Cookie"], "\n")
	for _, name := range []string{"token=", "refresh_token=", oidcStateCookieName + "="} {
		if !strings.Contains(cookies, name) {
			t.Fatalf("Set-Cookie = %q, want %s", cookies, name)
		}
	}
	foundSession := false
	for _, record := range app.Store.List(context.Background(), sessionsResource) {
		if record.Data["user_id"] == "US1" {
			foundSession = true
		}
	}
	if !foundSession {
		t.Fatal("callback did not persist an identity session for US1")
	}
}

func newOIDCTestTokenServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertOIDCTestTokenRequest(t, r)
		w.Header().Set(headerContentType, "application/json")
		_, _ = w.Write([]byte(`{"id_token":"` + oidcTestIDToken(map[string]any{"preferred_username": "admin"}) + `"}`))
	}))
}

func assertOIDCTestTokenRequest(t *testing.T, r *http.Request) {
	t.Helper()
	if r.Method != http.MethodPost || r.URL.Path != "/token" {
		t.Fatalf("dex request = %s %s, want POST /token", r.Method, r.URL.Path)
	}
	if err := r.ParseForm(); err != nil {
		t.Fatalf("parse dex form: %v", err)
	}
	if r.FormValue("grant_type") != "authorization_code" || r.FormValue("client_id") != "platform" || r.FormValue("code") != "code-1" {
		t.Fatalf("dex form = %#v, want authorization code request", r.Form)
	}
	if r.FormValue("redirect_uri") != "http://console.test/api/v1/oidc/callback" {
		t.Fatalf("redirect_uri = %q", r.FormValue("redirect_uri"))
	}
}

func TestIdentityHelperBranchesDirect(t *testing.T) {
	_, trustedProxy, err := net.ParseCIDR("198.51.100.0/24")
	if err != nil {
		t.Fatal(err)
	}
	app := platform.NewApp(platform.Config{
		ServiceName:       serviceName,
		TrustedProxyCIDRs: []*net.IPNet{trustedProxy},
	})
	req := identityRequest(http.MethodGet, "https://example.test", "")
	req.RemoteAddr = "203.0.113.200:1234"
	req.Header.Set("X-Forwarded-For", "203.0.113.10, 198.51.100.5")
	if requestIPForApp(app, req) != "203.0.113.200" {
		t.Fatalf("untrusted requestIP = %q, want remote addr", requestIPForApp(app, req))
	}
	req.RemoteAddr = "198.51.100.5:1234"
	if requestIPForApp(app, req) != "203.0.113.10" {
		t.Fatalf("trusted requestIP = %q, want rightmost untrusted forwarded IP", requestIPForApp(app, req))
	}
	if tokenPrefix("short") != "short" || len(tokenPrefix("nexuspaas_abcdefghijklmnop")) != 12 {
		t.Fatal("tokenPrefix did not preserve/trim expected values")
	}
	if normalize, ok := normalizeAPITokenName(" notebook "); !ok || normalize != "notebook" {
		t.Fatalf("normalized API token name = %q ok=%v", normalize, ok)
	}
	if _, ok := normalizeAPITokenName("\n"); ok {
		t.Fatal("control-only API token name was accepted")
	}
	if !passwordMatches(map[string]any{"password": "legacy"}, "legacy") {
		t.Fatal("legacy plain password verification failed")
	}
	if !expiredAt(map[string]any{"expires_at": time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)}, time.Now().UTC()) {
		t.Fatal("expiredAt did not detect past expiry")
	}
	if roleName(0) != "admin" || len(permissionsForRole("manager", 1)) != 1 || boolValue(map[string]any{"enabled": "true"}, "enabled") != true {
		t.Fatal("role/permission/bool helper branch failed")
	}
}

func oidcTestIDToken(claims map[string]any) string {
	header, _ := json.Marshal(map[string]any{"alg": "none"})
	payload, _ := json.Marshal(claims)
	return base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload) + "."
}

func newIdentityTestApp() *platform.App {
	app := platform.NewApp(platform.Config{ServiceName: "all", HTTPAddr: ":0", ExternalURLs: map[string]string{}})
	Register(app)
	return app
}

func newRoutedIdentityAuthTestApp(serviceKey string) *platform.App {
	app := platform.NewApp(platform.Config{
		ServiceName:   "all",
		HTTPAddr:      ":0",
		RequireAuth:   true,
		ServiceAPIKey: serviceKey,
		ExternalURLs:  map[string]string{},
	})
	Register(app)
	app.RegisterService(platform.ServiceSpec{Name: serviceName, Routes: []platform.RouteSpec{
		{Method: http.MethodGet, Pattern: "/api/v1/me/api-tokens", Resource: "api_tokens", AuthRequired: true},
		{Method: http.MethodPost, Pattern: "/api/v1/me/api-tokens", Resource: "api_tokens", AuthRequired: true},
		{Method: http.MethodDelete, Pattern: "/api/v1/me/api-tokens/current", Resource: "api_tokens", AuthRequired: true},
	}})
	return app
}

func seedIdentityUser(t *testing.T, app *platform.App, id, username string) {
	t.Helper()
	if _, err := app.Store.Create(context.Background(), usersResource, map[string]any{
		"id":            id,
		"username":      username,
		"password_hash": platform.HashSecret("correct-password"),
		"status":        "offline",
		"system_role":   2,
		"role":          "user",
	}); err != nil {
		t.Fatal(err)
	}
}

func seedIdentitySession(t *testing.T, app *platform.App, token, userID string) {
	t.Helper()
	if _, err := app.Store.Create(context.Background(), sessionsResource, map[string]any{
		"id":         token,
		"token":      token,
		"user_id":    userID,
		"created_at": time.Now().UTC().Format(time.RFC3339),
		"expires_at": time.Now().UTC().Add(time.Hour).Format(time.RFC3339),
	}); err != nil {
		t.Fatal(err)
	}
}

func serveIdentityHTTP(t *testing.T, app *platform.App, method, target, body string, headers map[string]string, want int) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := identityRequest(method, target, body)
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	app.ServeHTTP(rec, req)
	if rec.Code != want {
		t.Fatalf("%s %s status = %d, want %d; body=%s", method, target, rec.Code, want, rec.Body.String())
	}
	return rec
}

func identityRequest(method, target, body string) *http.Request {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	return req
}

func identityFormRequest(method, target, body string) *http.Request {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return req
}

func identityUserRequest(method, target, body, userID string) *http.Request {
	req := identityRequest(method, target, body)
	if userID != "" {
		req.Header.Set(headerUserID, userID)
	}
	return req
}

func identityResponseMap(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var envelope struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	return envelope.Data
}

func identityRawData(t *testing.T, data any) map[string]any {
	t.Helper()
	raw := data.(platform.RawResponse)
	var envelope struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(raw.Body, &envelope); err != nil {
		t.Fatal(err)
	}
	return envelope.Data
}

func assertIdentityStatus(t *testing.T, code int, data any, want int) {
	t.Helper()
	if code != want {
		t.Fatalf("status=%d data=%#v, want %d", code, data, want)
	}
}
