package services

import (
	"bytes"
	"encoding/base64"
	"image/png"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

func TestIdentityRegisterLoginRefreshLogout(t *testing.T) {
	app := newTestApp()

	registerIdentityUsersForAuthTest(t, app)
	token, refresh := loginAliceForAuthTest(t, app)
	refreshed := refreshAliceSessionForAuthTest(t, app, token, refresh)
	logoutAliceSessionForAuthTest(t, app, refreshed)
	assertLoginLockoutAuditForTrudy(t, app)
}

func registerIdentityUsersForAuthTest(t *testing.T, app *platform.App) {
	t.Helper()

	requestJSON(t, app, http.MethodPost, "/api/v1/register", `{bad`, nil, http.StatusBadRequest)
	requestJSON(t, app, http.MethodPost, "/api/v1/register", `{"username":"al","password":"short"}`, nil, http.StatusBadRequest)
	assertNoData(t, requestJSON(t, app, http.MethodPost, "/api/v1/register", `{"username":"alice","password":"correct-password","email":"alice@example.com","full_name":"Alice A"}`, nil, http.StatusOK))
	requestJSON(t, app, http.MethodPost, "/api/v1/register", `{"username":"alice","password":"correct-password"}`, nil, http.StatusConflict)
	assertNoData(t, requestJSON(t, app, http.MethodPost, "/api/v1/register", `{"id":"US2600001","username":"mallory","password":"correct-password","system_role":0,"role_id":"RO_ADMIN","status":"online"}`, nil, http.StatusOK))
	mallory := responseMap(t, requestJSON(t, app, http.MethodPost, "/api/v1/login", `{"username":"mallory","password":"correct-password"}`, nil, http.StatusOK))
	malloryUser := mallory["user"].(map[string]any)
	if malloryUser["role"] == "admin" || malloryUser["system_role"] == float64(0) || containsValue(malloryUser["permissions"], "adminPanel") {
		t.Fatalf("public register accepted privileged fields: %#v", malloryUser)
	}
	malloryRecord, ok := app.Store.Get(nil, "identity-service:users", "US2600002")
	if !ok || !strings.HasPrefix(malloryRecord.Data["password_hash"].(string), "pbkdf2-sha256:") {
		t.Fatalf("registered user hash = %#v, found=%v", malloryRecord.Data, ok)
	}
	aliceRecord, ok := app.Store.Get(nil, "identity-service:users", "US2600001")
	if !ok || aliceRecord.Data["username"] != "alice" {
		t.Fatalf("caller-supplied id overwrote alice: %#v, found=%v", aliceRecord.Data, ok)
	}
}

func loginAliceForAuthTest(t *testing.T, app *platform.App) (string, string) {
	t.Helper()

	requestJSON(t, app, http.MethodPost, "/api/v1/login", `{bad`, nil, http.StatusBadRequest)
	requestJSON(t, app, http.MethodPost, "/api/v1/login", `{"username":"alice"}`, nil, http.StatusBadRequest)
	requestJSON(t, app, http.MethodPost, "/api/v1/login", `{"username":"alice","password":"wrong-password"}`, nil, http.StatusUnauthorized)
	login := requestJSON(t, app, http.MethodPost, "/api/v1/login", `{"username":"alice","password":"correct-password"}`, nil, http.StatusOK)
	loginData := responseMap(t, login)
	token := loginData["token"].(string)
	refresh := loginData["refresh_token"].(string)
	if !strings.HasPrefix(token, "access.US") || !strings.HasPrefix(refresh, "refresh.") {
		t.Fatalf("login tokens = %#v", loginData)
	}
	cookies := strings.Join(login.Header().Values("Set-Cookie"), "\n")
	if !strings.Contains(cookies, "token="+token) || !strings.Contains(cookies, "refresh_token="+refresh) || !strings.Contains(cookies, "HttpOnly") {
		t.Fatalf("login cookies = %q", cookies)
	}
	userRecord, ok := app.Store.Get(nil, "identity-service:users", "US2600001")
	if !ok || userRecord.Data["status"] != "online" {
		t.Fatalf("user after login = %#v, found=%v", userRecord.Data, ok)
	}
	return token, refresh
}

func refreshAliceSessionForAuthTest(t *testing.T, app *platform.App, token, refresh string) map[string]any {
	t.Helper()

	requestJSON(t, app, http.MethodPost, "/api/v1/refresh", `{}`, nil, http.StatusBadRequest)
	requestJSON(t, app, http.MethodPost, "/api/v1/refresh", `{"refresh_token":"missing"}`, nil, http.StatusUnauthorized)
	refreshed := responseMap(t, requestJSON(t, app, http.MethodPost, "/api/v1/refresh", `{"refresh_token":"`+refresh+`"}`, nil, http.StatusOK))
	if refreshed["token"] == token || refreshed["refresh_token"] == refresh {
		t.Fatalf("refresh did not rotate tokens: %#v", refreshed)
	}
	requestJSON(t, app, http.MethodPost, "/api/v1/refresh", `{"refresh_token":"`+refresh+`"}`, nil, http.StatusUnauthorized)
	return refreshed
}

func logoutAliceSessionForAuthTest(t *testing.T, app *platform.App, refreshed map[string]any) {
	t.Helper()

	logout := requestJSON(t, app, http.MethodPost, "/api/v1/logout", ``, map[string]string{"Cookie": "token=" + refreshed["token"].(string) + "; refresh_token=" + refreshed["refresh_token"].(string)}, http.StatusOK)
	assertNoData(t, logout)
	clearCookies := strings.Join(logout.Header().Values("Set-Cookie"), "\n")
	if !strings.Contains(clearCookies, "token=;") || !strings.Contains(clearCookies, "refresh_token=;") || !strings.Contains(clearCookies, "Max-Age=0") {
		t.Fatalf("logout cookies = %q", clearCookies)
	}
	userRecord, ok := app.Store.Get(nil, "identity-service:users", "US2600001")
	if !ok || userRecord.Data["status"] != "offline" {
		t.Fatalf("user after logout = %#v, found=%v", userRecord.Data, ok)
	}
}

func assertLoginLockoutAuditForTrudy(t *testing.T, app *platform.App) {
	t.Helper()

	assertNoData(t, requestJSON(t, app, http.MethodPost, "/api/v1/register", `{"username":"trudy","password":"correct-password"}`, nil, http.StatusOK))
	for i := 0; i < 5; i++ {
		requestJSON(t, app, http.MethodPost, "/api/v1/login", `{"username":"trudy","password":"wrong-password"}`, map[string]string{"X-Forwarded-For": "203.0.113.9"}, http.StatusUnauthorized)
	}
	requestJSON(t, app, http.MethodPost, "/api/v1/login", `{"username":"trudy","password":"correct-password"}`, map[string]string{"X-Forwarded-For": "203.0.113.9"}, http.StatusUnauthorized)
	if got := countLoginFailureAudits(app); got < 5 {
		t.Fatalf("login_failed audit count = %d, want at least 5", got)
	}
}

func TestIdentityCaptchaAndCLILogin(t *testing.T) {
	app := newTestApp()
	assertNoData(t, requestJSON(t, app, http.MethodPost, "/api/v1/register", `{"username":"bob","password":"correct-password"}`, nil, http.StatusOK))

	captcha := responseMap(t, requestJSON(t, app, http.MethodGet, "/api/v1/captcha", "", nil, http.StatusOK))
	captchaID := captcha["captcha_id"].(string)
	answer := captcha["answer"].(string)
	if captchaID == "" || answer == "" || !strings.HasPrefix(captcha["image"].(string), "data:image/png;base64,") {
		t.Fatalf("captcha = %#v", captcha)
	}
	decodeCaptchaPNG(t, captcha["image"])
	requestJSON(t, app, http.MethodPost, "/api/v1/login", `{"username":"bob","password":"correct-password","captcha_id":"`+captchaID+`","captcha_answer":"0000"}`, nil, http.StatusUnauthorized)
	captcha = responseMap(t, requestJSON(t, app, http.MethodGet, "/api/v1/captcha", "", nil, http.StatusOK))
	captchaID = captcha["captcha_id"].(string)
	answer = captcha["answer"].(string)
	requestJSON(t, app, http.MethodPost, "/api/v1/login", `{"username":"bob","password":"correct-password","captcha_id":"`+captchaID+`","captcha_answer":"`+answer+`"}`, nil, http.StatusOK)

	requestJSON(t, app, http.MethodPost, "/api/v1/cli/login", `{"username":"bob"}`, nil, http.StatusBadRequest)
	requestJSON(t, app, http.MethodPost, "/api/v1/cli/login", `{"username":"bob","password":"wrong"}`, nil, http.StatusUnauthorized)
	cli := responseMap(t, requestJSON(t, app, http.MethodPost, "/api/v1/cli/login", `{"username":"bob","password":"correct-password","name":"laptop"}`, nil, http.StatusOK))
	if !strings.HasPrefix(cli["token"].(string), "nexuspaas_") || !strings.HasPrefix(cli["token_id"].(string), "AT") {
		t.Fatalf("cli login = %#v", cli)
	}
	if len(app.Store.List(nil, "identity-service:api_tokens")) != 1 {
		t.Fatalf("api token count = %d, want 1", len(app.Store.List(nil, "identity-service:api_tokens")))
	}
}

func TestIdentityProductionCaptchaAndIssuedTokensAuthorize(t *testing.T) {
	prodApp := platform.NewApp(platform.Config{ServiceName: "all", HTTPAddr: ":0", Production: true, ExternalURLs: map[string]string{}})
	RegisterAll(prodApp)
	captcha := responseMap(t, requestJSON(t, prodApp, http.MethodGet, "/api/v1/captcha", "", nil, http.StatusOK))
	if _, ok := captcha["answer"]; ok {
		t.Fatalf("production captcha exposed answer: %#v", captcha)
	}
	rawCaptcha := decodeCaptchaPNG(t, captcha["image"])
	if bytes.Contains(rawCaptcha, []byte("captcha:")) {
		t.Fatalf("production captcha image contains plaintext answer marker")
	}

	authApp := platform.NewApp(platform.Config{ServiceName: "all", HTTPAddr: ":0", RequireAuth: true, ExternalURLs: map[string]string{}})
	RegisterAll(authApp)
	requestJSON(t, authApp, http.MethodGet, "/api/v1/users", "", nil, http.StatusUnauthorized)
	requestJSON(t, authApp, http.MethodGet, "/api/v1/users", "", map[string]string{"Authorization": "Bearer forged"}, http.StatusUnauthorized)
	assertNoData(t, requestJSON(t, authApp, http.MethodPost, "/api/v1/register", `{"username":"carol","password":"correct-password"}`, nil, http.StatusOK))

	login := responseMap(t, requestJSON(t, authApp, http.MethodPost, "/api/v1/login", `{"username":"carol","password":"correct-password"}`, nil, http.StatusOK))
	token := login["token"].(string)
	allowPolicyForRoute(t, authApp, "US2600001", "", http.MethodGet, "/api/v1/me/api-tokens")
	requestJSON(t, authApp, http.MethodGet, "/api/v1/users", "", map[string]string{"Cookie": "token=" + token}, http.StatusForbidden)
	requestJSON(t, authApp, http.MethodGet, "/api/v1/me/api-tokens", "", map[string]string{"Cookie": "token=" + token}, http.StatusOK)
	requestJSON(t, authApp, http.MethodGet, "/api/v1/me/api-tokens", "", map[string]string{"Authorization": "Bearer " + token}, http.StatusOK)

	cli := responseMap(t, requestJSON(t, authApp, http.MethodPost, "/api/v1/cli/login", `{"username":"carol","password":"correct-password","name":"ci"}`, nil, http.StatusOK))
	requestJSON(t, authApp, http.MethodGet, "/api/v1/me/api-tokens", "", map[string]string{"Authorization": "Bearer " + cli["token"].(string)}, http.StatusOK)
}

func TestIdentityAPITokenLifecycle(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: "all", HTTPAddr: ":0", RequireAuth: true, ExternalURLs: map[string]string{}})
	RegisterAll(app)
	requestJSON(t, app, http.MethodGet, "/api/v1/me/api-tokens", "", nil, http.StatusUnauthorized)

	assertNoData(t, requestJSON(t, app, http.MethodPost, "/api/v1/register", `{"username":"dana","password":"correct-password"}`, nil, http.StatusOK))
	danaLogin := responseMap(t, requestJSON(t, app, http.MethodPost, "/api/v1/login", `{"username":"dana","password":"correct-password"}`, nil, http.StatusOK))
	danaSession := danaLogin["token"].(string)
	danaCookie := map[string]string{"Cookie": "token=" + danaSession}
	allowPolicyForRoute(t, app, "US2600001", "", http.MethodGet, "/api/v1/me/api-tokens")
	allowPolicyForRoute(t, app, "US2600001", "", http.MethodPost, "/api/v1/me/api-tokens")
	allowPolicyForRoute(t, app, "US2600001", "", http.MethodDelete, "/api/v1/me/api-tokens/current")
	allowPolicyForRoute(t, app, "US2600001", "", http.MethodDelete, "/api/v1/me/api-tokens/{id}")

	requestJSON(t, app, http.MethodPost, "/api/v1/me/api-tokens", `{}`, danaCookie, http.StatusBadRequest)
	created := responseMap(t, requestJSON(t, app, http.MethodPost, "/api/v1/me/api-tokens", `{"name":"notebook","user_id":"US9999999","token_hash":"plain:owned"}`, danaCookie, http.StatusCreated))
	rawToken := created["token"].(string)
	tokenID := created["id"].(string)
	if !strings.HasPrefix(rawToken, "nexuspaas_") || !strings.HasPrefix(tokenID, "AT") || created["token_hash"] != nil {
		t.Fatalf("created api token response = %#v", created)
	}
	stored, ok := app.Store.Get(nil, "identity-service:api_tokens", tokenID)
	if !ok || stored.Data["user_id"] != "US2600001" || stored.Data["token_hash"] == "plain:owned" {
		t.Fatalf("stored api token ignored scope/hash incorrectly: %#v found=%v", stored.Data, ok)
	}
	requestJSON(t, app, http.MethodGet, "/api/v1/me/api-tokens", "", map[string]string{"Authorization": "Bearer owned"}, http.StatusUnauthorized)
	requestJSON(t, app, http.MethodGet, "/api/v1/me/api-tokens", "", map[string]string{"Authorization": "Bearer " + rawToken}, http.StatusOK)

	list := responseSlice(t, requestJSON(t, app, http.MethodGet, "/api/v1/me/api-tokens", "", danaCookie, http.StatusOK))
	if len(list) != 1 {
		t.Fatalf("api token list length = %d, want 1: %#v", len(list), list)
	}
	listed := list[0].(map[string]any)
	if listed["id"] != tokenID || listed["token"] != nil || listed["token_hash"] != nil {
		t.Fatalf("api token metadata leaked secret or wrong token: %#v", listed)
	}

	requestJSON(t, app, http.MethodDelete, "/api/v1/me/api-tokens/current", "", danaCookie, http.StatusBadRequest)
	requestJSON(t, app, http.MethodDelete, "/api/v1/me/api-tokens/current", "", map[string]string{"Cookie": "token=" + danaSession, "X-API-Token-ID": tokenID}, http.StatusBadRequest)
	requestJSON(t, app, http.MethodDelete, "/api/v1/me/api-tokens/current", "", map[string]string{"Authorization": "Bearer " + rawToken}, http.StatusOK)
	requestJSON(t, app, http.MethodGet, "/api/v1/me/api-tokens", "", map[string]string{"Authorization": "Bearer " + rawToken}, http.StatusUnauthorized)

	second := responseMap(t, requestJSON(t, app, http.MethodPost, "/api/v1/me/api-tokens", `{"name":"second"}`, danaCookie, http.StatusCreated))
	secondID := second["id"].(string)
	assertNoData(t, requestJSON(t, app, http.MethodPost, "/api/v1/register", `{"username":"erin","password":"correct-password"}`, nil, http.StatusOK))
	erinLogin := responseMap(t, requestJSON(t, app, http.MethodPost, "/api/v1/login", `{"username":"erin","password":"correct-password"}`, nil, http.StatusOK))
	allowPolicyForRoute(t, app, "US2600002", "", http.MethodDelete, "/api/v1/me/api-tokens/{id}")
	requestJSON(t, app, http.MethodDelete, "/api/v1/me/api-tokens/"+secondID, "", map[string]string{"Cookie": "token=" + erinLogin["token"].(string)}, http.StatusNotFound)
	requestJSON(t, app, http.MethodDelete, "/api/v1/me/api-tokens/"+secondID, "", danaCookie, http.StatusOK)
	requestJSON(t, app, http.MethodGet, "/api/v1/me/api-tokens", "", map[string]string{"Authorization": "Bearer " + second["token"].(string)}, http.StatusUnauthorized)
}

func TestIdentityCaptchaExpiryAndOneTimeUse(t *testing.T) {
	app := newTestApp()
	assertNoData(t, requestJSON(t, app, http.MethodPost, "/api/v1/register", `{"username":"eve","password":"correct-password"}`, nil, http.StatusOK))

	expired := responseMap(t, requestJSON(t, app, http.MethodGet, "/api/v1/captcha", "", nil, http.StatusOK))
	expiredID := expired["captcha_id"].(string)
	_, _ = app.Store.Update(nil, "identity-service:captchas", expiredID, map[string]any{"expires_at": time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)})
	requestJSON(t, app, http.MethodPost, "/api/v1/login", `{"username":"eve","password":"correct-password","captcha_id":"`+expiredID+`","captcha_answer":"`+expired["answer"].(string)+`"}`, nil, http.StatusUnauthorized)
	if _, ok := app.Store.Get(nil, "identity-service:captchas", expiredID); ok {
		t.Fatalf("expired captcha %s was not consumed", expiredID)
	}

	captcha := responseMap(t, requestJSON(t, app, http.MethodGet, "/api/v1/captcha", "", nil, http.StatusOK))
	captchaID := captcha["captcha_id"].(string)
	answer := captcha["answer"].(string)
	requestJSON(t, app, http.MethodPost, "/api/v1/login", `{"username":"eve","password":"correct-password","captcha_id":"`+captchaID+`","captcha_answer":"`+answer+`"}`, nil, http.StatusOK)
	requestJSON(t, app, http.MethodPost, "/api/v1/login", `{"username":"eve","password":"correct-password","captcha_id":"`+captchaID+`","captcha_answer":"`+answer+`"}`, nil, http.StatusUnauthorized)
}

func TestIdentityCaptchaRequiredAfterFailureThreshold(t *testing.T) {
	app := newTestApp()
	assertNoData(t, requestJSON(t, app, http.MethodPost, "/api/v1/register", `{"username":"frank","password":"correct-password"}`, nil, http.StatusOK))
	for i := 0; i < 5; i++ {
		requestJSON(t, app, http.MethodPost, "/api/v1/login", `{"username":"frank","password":"wrong-password"}`, nil, http.StatusUnauthorized)
	}
	failures, ok := loginFailureForUser(app, "frank")
	if !ok || failures["failures"] != 5 || failures["locked_until"] != nil {
		t.Fatalf("threshold failures = %#v, found=%v; want count 5 without lock", failures, ok)
	}
	captcha := responseMap(t, requestJSON(t, app, http.MethodGet, "/api/v1/captcha", "", nil, http.StatusOK))
	requestJSON(t, app, http.MethodPost, "/api/v1/login", `{"username":"frank","password":"correct-password","captcha_id":"`+captcha["captcha_id"].(string)+`","captcha_answer":"`+captcha["answer"].(string)+`"}`, nil, http.StatusOK)
	if failures, ok := loginFailureForUser(app, "frank"); ok {
		t.Fatalf("successful captcha login did not clear failures: %#v", failures)
	}

	assertNoData(t, requestJSON(t, app, http.MethodPost, "/api/v1/register", `{"username":"grace","password":"correct-password"}`, nil, http.StatusOK))
	for i := 0; i < 5; i++ {
		requestJSON(t, app, http.MethodPost, "/api/v1/login", `{"username":"grace","password":"wrong-password"}`, nil, http.StatusUnauthorized)
	}
	requestJSON(t, app, http.MethodPost, "/api/v1/login", `{"username":"grace","password":"correct-password"}`, nil, http.StatusUnauthorized)
	failures, ok = loginFailureForUser(app, "grace")
	if !ok || failures["failures"] != 6 || failures["locked_until"] == nil {
		t.Fatalf("omitted threshold captcha failures = %#v, found=%v; want count 6 with lock", failures, ok)
	}
	captcha = responseMap(t, requestJSON(t, app, http.MethodGet, "/api/v1/captcha", "", nil, http.StatusOK))
	requestJSON(t, app, http.MethodPost, "/api/v1/login", `{"username":"grace","password":"correct-password","captcha_id":"`+captcha["captcha_id"].(string)+`","captcha_answer":"`+captcha["answer"].(string)+`"}`, nil, http.StatusUnauthorized)
}

func TestIdentityOIDCRoutesFailClosed(t *testing.T) {
	app := newTestApp()
	cases := []struct {
		name    string
		method  string
		path    string
		body    string
		headers map[string]string
		want    int
	}{
		{name: "login form missing auth request", method: http.MethodGet, path: "/api/v1/oidc/login", want: http.StatusBadRequest},
		{name: "login form provider missing", method: http.MethodGet, path: "/api/v1/oidc/login?auth_request_id=req-1", want: http.StatusServiceUnavailable},
		{name: "login post invalid", method: http.MethodPost, path: "/api/v1/oidc/login", body: `{}`, want: http.StatusBadRequest},
		{name: "login post provider missing", method: http.MethodPost, path: "/api/v1/oidc/login", body: `{"auth_request_id":"req-1","username":"alice","password":"secret"}`, want: http.StatusServiceUnavailable},
		{name: "legacy token missing grant", method: http.MethodPost, path: "/oauth/token", body: `{}`, want: http.StatusBadRequest},
		{name: "legacy token missing provider", method: http.MethodPost, path: "/oauth/token?grant_type=authorization_code&client_id=grafana&code=code-1", want: http.StatusServiceUnavailable},
		{name: "prefixed token missing provider", method: http.MethodPost, path: "/api/v1/oidc/token?grant_type=refresh_token&client_id=grafana&refresh_token=refresh-1", want: http.StatusServiceUnavailable},
		{name: "legacy revoke missing token", method: http.MethodPost, path: "/revoke", body: `{}`, want: http.StatusBadRequest},
		{name: "legacy revoke missing provider", method: http.MethodPost, path: "/revoke?token=access-1", want: http.StatusServiceUnavailable},
		{name: "prefixed revoke missing provider", method: http.MethodPost, path: "/api/v1/oidc/revoke?token=access-1", want: http.StatusServiceUnavailable},
		{name: "device authorization missing client", method: http.MethodPost, path: "/device_authorization", body: `{}`, want: http.StatusBadRequest},
		{name: "device authorization missing provider", method: http.MethodPost, path: "/device_authorization?client_id=grafana", want: http.StatusServiceUnavailable},
		{name: "unknown well known", method: http.MethodGet, path: "/api/v1/.well-known/unknown", want: http.StatusNotFound},
		{name: "well known missing provider", method: http.MethodGet, path: "/api/v1/.well-known/openid-configuration", want: http.StatusServiceUnavailable},
		{name: "prefixed discovery missing provider", method: http.MethodGet, path: "/api/v1/oidc/.well-known/openid-configuration", want: http.StatusServiceUnavailable},
		{name: "legacy jwks missing provider", method: http.MethodGet, path: "/api/v1/keys", want: http.StatusServiceUnavailable},
		{name: "prefixed jwks missing provider", method: http.MethodGet, path: "/api/v1/oidc/jwks", want: http.StatusServiceUnavailable},
		{name: "legacy authorize missing params", method: http.MethodGet, path: "/api/v1/authorize", want: http.StatusBadRequest},
		{name: "legacy authorize missing provider", method: http.MethodGet, path: "/api/v1/authorize?client_id=grafana&response_type=code&redirect_uri=https://grafana.example/callback", want: http.StatusServiceUnavailable},
		{name: "prefixed authorize missing provider", method: http.MethodGet, path: "/api/v1/oidc/authorize?client_id=grafana&response_type=code&redirect_uri=https://grafana.example/callback", want: http.StatusServiceUnavailable},
		{name: "legacy userinfo missing bearer", method: http.MethodGet, path: "/api/v1/userinfo", want: http.StatusUnauthorized},
		{name: "legacy userinfo missing provider", method: http.MethodGet, path: "/api/v1/userinfo", headers: map[string]string{"Authorization": "Bearer access-1"}, want: http.StatusServiceUnavailable},
		{name: "prefixed userinfo missing provider", method: http.MethodGet, path: "/api/v1/oidc/userinfo", headers: map[string]string{"Authorization": "Bearer access-1"}, want: http.StatusServiceUnavailable},
		{name: "legacy callback missing params", method: http.MethodGet, path: "/api/v1/authorize/callback", want: http.StatusBadRequest},
		{name: "legacy callback missing provider", method: http.MethodGet, path: "/api/v1/authorize/callback?code=code-1&state=state-1", want: http.StatusServiceUnavailable},
		{name: "prefixed callback missing provider", method: http.MethodGet, path: "/api/v1/oidc/callback?code=code-1&state=state-1", want: http.StatusServiceUnavailable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			requestJSON(t, app, tc.method, tc.path, tc.body, tc.headers, tc.want)
		})
	}

	for _, resource := range []string{
		"identity-service:oidc",
		"identity-service:jwks",
		"identity-service:oidc_login",
		"identity-service:oidc_keys",
		"identity-service:oidc_tokens",
		"identity-service:oidc_devices",
		"identity-service:oidc_userinfo",
		"identity-service:oidc_callbacks",
		"identity-service:oidc_authorize",
		"identity-service:oidc_authorizations",
		"identity-service:oidc_authorize_callback",
	} {
		if got := len(app.Store.List(nil, resource)); got != 0 {
			t.Fatalf("%s records = %d, want 0", resource, got)
		}
	}
}

func containsValue(values any, want string) bool {
	for _, value := range values.([]any) {
		if value == want {
			return true
		}
	}
	return false
}

func countLoginFailureAudits(app *platform.App) int {
	count := 0
	for _, event := range app.Events.Outbox() {
		if event.Name == "AuditEvent" && event.Data["action"] == "login_failed" {
			count++
		}
	}
	return count
}

func decodeCaptchaPNG(t *testing.T, value any) []byte {
	t.Helper()
	imageText, ok := value.(string)
	if !ok {
		t.Fatalf("captcha image type = %T, want string", value)
	}
	const prefix = "data:image/png;base64,"
	if !strings.HasPrefix(imageText, prefix) {
		t.Fatalf("captcha image prefix = %q", imageText)
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(imageText, prefix))
	if err != nil {
		t.Fatalf("captcha base64 decode failed: %v", err)
	}
	if !bytes.HasPrefix(raw, []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}) {
		t.Fatalf("captcha is not a PNG payload")
	}
	if _, err := png.Decode(bytes.NewReader(raw)); err != nil {
		t.Fatalf("captcha PNG decode failed: %v", err)
	}
	return raw
}

func loginFailureForUser(app *platform.App, username string) (map[string]any, bool) {
	for _, record := range app.Store.List(nil, "identity-service:login_failures") {
		if record.Data["username"] == strings.ToLower(username) {
			return record.Data, true
		}
	}
	return nil, false
}
