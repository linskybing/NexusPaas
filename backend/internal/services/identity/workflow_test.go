package identity

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
			req := identityFormRequest(http.MethodPost, "/oauth/token", "grant_type=authorization_code&client_id=grafana&code=code-1")
			return oidcToken(app, req, platform.RouteSpec{})
		}, status: http.StatusServiceUnavailable},
		{name: "revoke missing token", call: func() (int, any, *platform.Degraded) {
			return oidcRevoke(app, identityRequest(http.MethodPost, "/revoke", `{}`), platform.RouteSpec{})
		}, status: http.StatusBadRequest},
		{name: "device auth valid", call: func() (int, any, *platform.Degraded) {
			return oidcDeviceAuthorization(app, identityRequest(http.MethodPost, "/device_authorization?client_id=grafana", ""), platform.RouteSpec{})
		}, status: http.StatusServiceUnavailable},
		{name: "well-known valid", call: func() (int, any, *platform.Degraded) {
			req := identityRequest(http.MethodGet, "/api/v1/.well-known/openid-configuration", "")
			req.SetPathValue("path", "openid-configuration")
			return oidcWellKnown(app, req, platform.RouteSpec{})
		}, status: http.StatusServiceUnavailable},
		{name: "authorize valid", call: func() (int, any, *platform.Degraded) {
			return oidcAuthorize(app, identityRequest(http.MethodGet, "/api/v1/authorize?client_id=grafana&response_type=code&redirect_uri=https://grafana.example/callback", ""), platform.RouteSpec{})
		}, status: http.StatusServiceUnavailable},
		{name: "userinfo bearer", call: func() (int, any, *platform.Degraded) {
			req := identityRequest(http.MethodGet, "/api/v1/userinfo", "")
			req.Header.Set("Authorization", "Bearer access-1")
			return oidcUserInfo(app, req, platform.RouteSpec{})
		}, status: http.StatusServiceUnavailable},
		{name: "callback valid", call: func() (int, any, *platform.Degraded) {
			return oidcCallback(app, identityRequest(http.MethodGet, "/api/v1/authorize/callback?code=code-1&state=state-1", ""), platform.RouteSpec{})
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

func TestIdentityHelperBranchesDirect(t *testing.T) {
	req := identityRequest(http.MethodGet, "https://example.test", "")
	req.Header.Set("X-Forwarded-For", "203.0.113.10, 198.51.100.5")
	if requestIP(req) != "203.0.113.10" {
		t.Fatalf("requestIP = %q, want first forwarded IP", requestIP(req))
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

func newIdentityTestApp() *platform.App {
	app := platform.NewApp(platform.Config{ServiceName: "all", HTTPAddr: ":0", ExternalURLs: map[string]string{}})
	Register(app)
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
