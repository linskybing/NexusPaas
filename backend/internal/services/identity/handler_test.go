package identity

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

func TestInternalIdentityReadContractsRequireServiceAuth(t *testing.T) {
	serviceKey := testServiceKey(t)
	app := platform.NewApp(platform.Config{ServiceName: serviceName, ServiceAPIKey: serviceKey})
	Register(app)
	if _, err := app.Store.Create(context.Background(), usersResource, map[string]any{
		"id":            "US1",
		"username":      "alice",
		"status":        "online",
		"password_hash": "must-not-leak",
		"token_hash":    "must-not-leak",
		"token":         "must-not-leak",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Store.Create(context.Background(), rolesResource, map[string]any{"id": "RO1", "name": "admin"}); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/internal/identity/users", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated users list status = %d, want 401; body=%s", rec.Code, rec.Body.String())
	}

	for _, tc := range []struct {
		path string
		want string
	}{
		{path: "/internal/identity/users", want: "alice"},
		{path: "/internal/identity/users/US1", want: "alice"},
		{path: "/internal/identity/roles", want: "admin"},
		{path: "/internal/identity/roles/RO1", want: "admin"},
	} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		req.Header.Set("X-Service-Key", serviceKey)
		app.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d, want 200; body=%s", tc.path, rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), tc.want) {
			t.Fatalf("%s body = %s, want %q", tc.path, rec.Body.String(), tc.want)
		}
		if strings.HasPrefix(tc.path, "/internal/identity/users") {
			assertNoCredentialLeak(t, tc.path, rec.Body.String())
		}
	}
}

func assertNoCredentialLeak(t *testing.T, path, body string) {
	t.Helper()
	for _, leak := range []string{"password_hash", "token_hash", `"token":`, "must-not-leak"} {
		if strings.Contains(body, leak) {
			t.Fatalf("%s body = %s, leaked credential field/value %q", path, body, leak)
		}
	}
}

func TestInternalIdentityAuthContractsVerifyCredentials(t *testing.T) {
	serviceKey := testServiceKey(t)
	rawAPIToken := "nexuspaas_" + strings.ReplaceAll(t.Name(), "/", "_")
	app := platform.NewApp(platform.Config{ServiceName: serviceName, ServiceAPIKey: serviceKey})
	Register(app)
	if _, err := app.Store.Create(context.Background(), usersResource, map[string]any{
		"id":            "US1",
		"username":      "alice",
		"role":          "admin",
		"system_role":   0,
		"status":        "online",
		"password_hash": "must-not-leak",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Store.Create(context.Background(), sessionsResource, map[string]any{
		"id":         "session-1",
		"user_id":    "US1",
		"expires_at": time.Now().UTC().Add(time.Hour).Format(time.RFC3339),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Store.Create(context.Background(), apiTokensResource, map[string]any{
		"id":         "AT1",
		"user_id":    "US1",
		"token_hash": platform.HashSecret(rawAPIToken),
		"expires_at": time.Now().UTC().Add(time.Hour).Format(time.RFC3339),
		"revoked":    false,
	}); err != nil {
		t.Fatal(err)
	}

	sessionBody := postInternalIdentityAuth(t, app, serviceKey, "/internal/identity/auth/session", "session-1", http.StatusOK)
	if !strings.Contains(sessionBody, `"username":"alice"`) || strings.Contains(sessionBody, "password_hash") {
		t.Fatalf("session auth body = %s, want sanitized verified user", sessionBody)
	}

	apiBody := postInternalIdentityAuth(t, app, serviceKey, "/internal/identity/auth/api-token", rawAPIToken, http.StatusOK)
	if !strings.Contains(apiBody, `"api_token_id":"AT1"`) || strings.Contains(apiBody, "token_hash") {
		t.Fatalf("api token auth body = %s, want token id without hash leak", apiBody)
	}
	tokenRecord, ok := app.Store.Get(context.Background(), apiTokensResource, "AT1")
	if !ok || tokenRecord.Data["last_used_at"] == nil {
		t.Fatalf("api token record = %#v, want identity-owned last_used_at update", tokenRecord)
	}

	postInternalIdentityAuth(t, app, serviceKey, "/internal/identity/auth/session", "missing", http.StatusUnauthorized)
	postInternalIdentityAuth(t, app, "wrong-"+serviceKey, "/internal/identity/auth/api-token", rawAPIToken, http.StatusUnauthorized)
}

func TestInternalIdentityReadContractsAreDisabledWithoutServiceKey(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: serviceName})
	Register(app)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/internal/identity/users", nil)
	req.Header.Set("X-Service-Key", testServiceKey(t))
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 when SERVICE_API_KEY is unset; body=%s", rec.Code, rec.Body.String())
	}
}

func TestIdentityUserAdministrationHandlers(t *testing.T) {
	app := newIdentityAdminTestApp(t)

	assertIdentityAdminListAuth(t, app)
	assertIdentityAdminSelfService(t, app)
	assertIdentityAdminBatchOperations(t, app)
	assertIdentityCLICertDownload(t, app)
}

func assertIdentityAdminListAuth(t *testing.T, app *platform.App) {
	t.Helper()
	code, data, _ := listUsers(app, identityAdminRequest(http.MethodGet, "/api/v1/users", "", ""), platform.RouteSpec{})
	assertIdentityAdminStatus(t, code, data, http.StatusUnauthorized)
	code, data, _ = listUsers(app, identityAdminRequest(http.MethodGet, "/api/v1/users", "", "U1"), platform.RouteSpec{})
	assertIdentityAdminStatus(t, code, data, http.StatusForbidden)
	code, data, _ = listUsers(app, identityAdminRequest(http.MethodGet, "/api/v1/users", "", "ADMIN"), platform.RouteSpec{})
	assertIdentityAdminStatus(t, code, data, http.StatusOK)
	users := data.([]map[string]any)
	if len(users) != 2 || users[0]["password_hash"] != nil {
		t.Fatalf("users = %#v, want sanitized public users", users)
	}
}

func assertIdentityAdminSelfService(t *testing.T, app *platform.App) {
	t.Helper()
	getReq := identityAdminRequest(http.MethodGet, "/api/v1/users/U1", "", "U1")
	getReq.SetPathValue("id", "U1")
	code, data, _ := getUserByID(app, getReq, platform.RouteSpec{})
	assertIdentityAdminStatus(t, code, data, http.StatusOK)
	if data.(map[string]any)["username"] != "alice" {
		t.Fatalf("user = %#v, want alice", data)
	}

	updateReq := identityAdminRequest(http.MethodPut, "/api/v1/users/U1", `{"name":"Alice Updated","role":"admin"}`, "U1")
	updateReq.SetPathValue("id", "U1")
	code, data, _ = updateUser(app, updateReq, platform.RouteSpec{})
	assertIdentityAdminStatus(t, code, data, http.StatusOK)
	if got := data.(map[string]any); got["name"] != "Alice Updated" || got["role"] == "admin" {
		t.Fatalf("self update = %#v, want name change without role escalation", got)
	}

	settingsReq := identityAdminRequest(http.MethodPut, "/api/v1/users/U1/settings", `{"settings":{"theme":"dark"}}`, "U1")
	settingsReq.SetPathValue("id", "U1")
	code, data, _ = updateUserSettings(app, settingsReq, platform.RouteSpec{})
	assertIdentityAdminStatus(t, code, data, http.StatusOK)
	if data.(map[string]any)["theme"] != "dark" {
		t.Fatalf("settings = %#v, want theme", data)
	}
}

func assertIdentityAdminBatchOperations(t *testing.T, app *platform.App) {
	t.Helper()
	resolveReq := identityAdminRequest(http.MethodPost, "/api/v1/users/resolve", `{"identifiers":["alice","missing"]}`, "ADMIN")
	code, data, _ := resolveUsers(app, resolveReq, platform.RouteSpec{})
	assertIdentityAdminStatus(t, code, data, http.StatusOK)
	resolved := data.(map[string]any)
	if len(resolved["resolved"].([]map[string]any)) != 1 || len(resolved["unresolved"].([]string)) != 1 {
		t.Fatalf("resolved = %#v, want one resolved and one unresolved", resolved)
	}

	roleReq := identityAdminRequest(http.MethodPut, "/api/v1/users/batch/role", `{"ids":["U1"],"role":"manager"}`, "ADMIN")
	code, data, _ = batchUpdateRole(app, roleReq, platform.RouteSpec{})
	assertIdentityAdminStatus(t, code, data, http.StatusOK)
	updated, _ := app.Store.Get(context.Background(), usersResource, "U1")
	if updated.Data["role"] != "manager" || updated.Data["system_role"] != 1 {
		t.Fatalf("role update = %#v, want manager", updated.Data)
	}

	passwordReq := identityAdminRequest(http.MethodPut, "/api/v1/users/batch/password", `{"ids":["U1"],"password":"new-password"}`, "ADMIN")
	code, data, _ = batchResetPassword(app, passwordReq, platform.RouteSpec{})
	assertIdentityAdminStatus(t, code, data, http.StatusOK)
	updated, _ = app.Store.Get(context.Background(), usersResource, "U1")
	if !strings.HasPrefix(updated.Data["password_hash"].(string), "pbkdf2-sha256:") {
		t.Fatalf("password hash = %#v, want hashed", updated.Data["password_hash"])
	}

	createReq := identityAdminRequest(http.MethodPost, "/api/v1/users/batch", `{"users":[{"username":"bob","password":"secret1","email":"bob@test.local"}]}`, "ADMIN")
	code, data, _ = batchCreateUsers(app, createReq, platform.RouteSpec{})
	assertIdentityAdminStatus(t, code, data, http.StatusOK)
	if data.(map[string]any)["succeeded"] != 1 {
		t.Fatalf("batch create = %#v, want success", data)
	}

	deleteReq := identityAdminRequest(http.MethodDelete, "/api/v1/users/batch", `{"ids":["U1","missing"]}`, "ADMIN")
	code, data, _ = batchDeleteUsers(app, deleteReq, platform.RouteSpec{})
	assertIdentityAdminStatus(t, code, data, http.StatusOK)
	if result := data.(map[string]any); result["succeeded"] != 1 || result["failed"] != 1 {
		t.Fatalf("batch delete = %#v, want one success and one failure", result)
	}
}

func assertIdentityCLICertDownload(t *testing.T, app *platform.App) {
	t.Helper()
	app.Config.CLICACertPEM = "test-ca"
	code, data, _ := downloadCLICACert(app, identityAdminRequest(http.MethodGet, "/api/v1/me/cli-ca", "", "U1"), platform.RouteSpec{})
	assertIdentityAdminStatus(t, code, data, http.StatusOK)
	if raw := data.(platform.RawResponse); raw.ContentType != "application/x-pem-file" || string(raw.Body) != "test-ca" {
		t.Fatalf("cli ca = %#v", raw)
	}
}

func postInternalIdentityAuth(t *testing.T, app *platform.App, serviceKey, path, token string, want int) string {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"token":"`+token+`"}`))
	req.Header.Set("X-Service-Key", serviceKey)
	req.Header.Set("Content-Type", "application/json")
	app.ServeHTTP(rec, req)
	if rec.Code != want {
		t.Fatalf("%s status = %d, want %d; body=%s", path, rec.Code, want, rec.Body.String())
	}
	return rec.Body.String()
}

func newIdentityAdminTestApp(t *testing.T) *platform.App {
	t.Helper()
	app := platform.NewApp(platform.Config{ServiceName: serviceName})
	Register(app)
	createIdentityRecord(t, app, usersResource, map[string]any{"id": "ADMIN", "username": "admin", "role": "admin", "system_role": 0, "status": "online"})
	createIdentityRecord(t, app, usersResource, map[string]any{"id": "U1", "username": "alice", "email": "alice@test.local", "role": "user", "system_role": 2, "status": "online", "password_hash": "old"})
	return app
}

func identityAdminRequest(method, target, body, userID string) *http.Request {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if userID != "" {
		req.Header.Set(headerUserID, userID)
	}
	return req
}

func createIdentityRecord(t *testing.T, app *platform.App, resource string, data map[string]any) {
	t.Helper()
	if _, err := app.Store.Create(context.Background(), resource, data); err != nil {
		t.Fatal(err)
	}
}

func assertIdentityAdminStatus(t *testing.T, code int, data any, want int) {
	t.Helper()
	if code != want {
		t.Fatalf("status=%d data=%#v, want %d", code, data, want)
	}
}

func testServiceKey(t *testing.T) string {
	t.Helper()
	return "svc-" + t.Name()
}

func TestRecordLoginFailureCreateConflictRecoversByUpdating(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: "all", HTTPAddr: ":0", ExternalURLs: map[string]string{}})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/login", nil)
	req.Header.Set("X-Forwarded-For", "203.0.113.10")
	id := loginFailureID("alice", requestIP(req))
	originalHook := beforeLoginFailureCreate
	beforeLoginFailureCreate = func(app *platform.App, r *http.Request, id string) {
		_, _ = app.Store.Create(r.Context(), loginFailuresResource, map[string]any{
			"id":         id,
			"username":   "alice",
			"ip":         requestIP(r),
			"failures":   1,
			"updated_at": "before",
		})
	}
	defer func() { beforeLoginFailureCreate = originalHook }()

	recordLoginFailure(app, req, "alice")

	record, ok := app.Store.Get(req.Context(), loginFailuresResource, id)
	if !ok {
		t.Fatal("login failure record missing")
	}
	if got := intValue(record.Data, "failures", 0); got != 2 {
		t.Fatalf("failures = %d, want recovered increment to 2; data=%#v", got, record.Data)
	}
}

func TestDexProxyForwardsConfiguredOIDCEndpoint(t *testing.T) {
	dex := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/token" || r.URL.RawQuery != "foo=bar" {
			t.Fatalf("dex request = %s %s?%s", r.Method, r.URL.Path, r.URL.RawQuery)
		}
		if r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
			t.Fatalf("Content-Type = %q", r.Header.Get("Content-Type"))
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if string(body) != "grant_type=password" {
			t.Fatalf("body = %q", body)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer dex.Close()

	app := platform.NewApp(platform.Config{DexURL: dex.URL})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/oidc/token?foo=bar", strings.NewReader("grant_type=password"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	status, data, degraded := dexProxy("/token")(app, req, platform.RouteSpec{})
	if degraded != nil {
		t.Fatalf("degraded = %#v, want nil", degraded)
	}
	if status != http.StatusCreated {
		t.Fatalf("status = %d, want 201", status)
	}
	raw, ok := data.(platform.RawResponse)
	if !ok {
		t.Fatalf("data = %T, want RawResponse", data)
	}
	if raw.ContentType != "application/json" || string(raw.Body) != `{"ok":true}` {
		t.Fatalf("raw response = %#v", raw)
	}
}

func TestDexProxyFallsBackWhenDexURLUnset(t *testing.T) {
	app := platform.NewApp(platform.Config{})
	status, data, degraded := dexProxy("/token")(app, httptest.NewRequest(http.MethodPost, "/api/v1/oidc/token", nil), platform.RouteSpec{})
	if degraded != nil {
		t.Fatalf("degraded = %#v, want nil", degraded)
	}
	if status != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", status)
	}
	got, ok := data.(map[string]any)
	if !ok {
		t.Fatalf("data = %T, want map response", data)
	}
	if got["reason"] != "oidc_provider_not_configured" {
		t.Fatalf("response = %#v", got)
	}
}
