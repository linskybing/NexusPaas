package platform

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
)

func TestRequireAuthStripsClientIdentityHeadersAndEnforcesAdminRoute(t *testing.T) {
	app := newAuthTestApp()
	adminRoute := RouteSpec{Method: http.MethodGet, Pattern: "/admin", Resource: "test:admin", Action: "list", AuthRequired: true, Admin: true}
	userRoute := RouteSpec{Method: http.MethodGet, Pattern: "/user", Resource: "test:user", Action: "list", AuthRequired: true}
	app.Routes = []RouteSpec{adminRoute, userRoute}
	app.RegisterCustomHandler(http.MethodGet, "/admin", echoAuthHandler)
	app.RegisterCustomHandler(http.MethodGet, "/user", echoAuthHandler)

	requestAuthTest(t, app, http.MethodGet, "/admin", map[string]string{
		"Authorization": "Bearer user-session",
		"X-User-ID":     "admin",
		"X-User-Role":   "admin",
	}, http.StatusForbidden)

	apiKeyUser := requestAuthTest(t, app, http.MethodGet, "/user", map[string]string{
		"X-API-Key":      "test-key",
		"X-User-ID":      "admin",
		"X-User-Role":    "admin",
		"X-API-Token-ID": "forged-token",
	}, http.StatusOK)
	if apiKeyUser["user_id"] != "" || apiKeyUser["role"] != "" || apiKeyUser["api_token_id"] != "" {
		t.Fatalf("api-key request retained forged identity headers: %#v", apiKeyUser)
	}
	requestAuthTest(t, app, http.MethodGet, "/admin", map[string]string{
		"X-API-Key":   "test-key",
		"X-User-ID":   "admin",
		"X-User-Role": "admin",
	}, http.StatusForbidden)

	adminSession := requestAuthTest(t, app, http.MethodGet, "/admin", map[string]string{"Authorization": "Bearer admin-session"}, http.StatusOK)
	if adminSession["user_id"] != "admin" || adminSession["role"] != "admin" {
		t.Fatalf("admin session was not applied as verified principal: %#v", adminSession)
	}
	adminCookieWithKey := requestAuthTest(t, app, http.MethodGet, "/admin", map[string]string{
		"Cookie":    "token=admin-session",
		"X-API-Key": "test-key",
	}, http.StatusOK)
	if adminCookieWithKey["user_id"] != "admin" {
		t.Fatalf("api key took precedence over admin cookie principal: %#v", adminCookieWithKey)
	}
	capabilityAdmin := requestAuthTest(t, app, http.MethodGet, "/admin", map[string]string{"Authorization": "Bearer capability-session"}, http.StatusOK)
	if capabilityAdmin["user_id"] != "capability-admin" {
		t.Fatalf("capability admin was not accepted as verified principal: %#v", capabilityAdmin)
	}
	panelAdmin := requestAuthTest(t, app, http.MethodGet, "/admin", map[string]string{"Authorization": "Bearer panel-session"}, http.StatusOK)
	if panelAdmin["user_id"] != "panel-admin" {
		t.Fatalf("direct admin_panel user was not accepted as verified principal: %#v", panelAdmin)
	}
	adminAPI := requestAuthTest(t, app, http.MethodGet, "/admin", map[string]string{"Authorization": "Bearer " + FormatUserAPIToken("ATADMIN", "admin-secret")}, http.StatusOK)
	if adminAPI["user_id"] != "admin" || adminAPI["api_token_id"] == "" {
		t.Fatalf("admin api token was not applied as verified principal: %#v", adminAPI)
	}
}

func TestIndexedAPITokenAuthDoesNotListTokens(t *testing.T) {
	app := newAuthTestApp()
	adminRoute := RouteSpec{Method: http.MethodGet, Pattern: "/admin", Resource: "test:admin", Action: "list", AuthRequired: true, Admin: true}
	app.Routes = []RouteSpec{adminRoute}
	app.RegisterCustomHandler(http.MethodGet, "/admin", echoAuthHandler)
	spy := &authListSpyStore{RecordStore: app.Store}
	app.Store = spy
	user := requestAuthTest(t, app, http.MethodGet, "/admin", map[string]string{"Authorization": "Bearer " + FormatUserAPIToken("ATADMIN", "admin-secret")}, http.StatusOK)
	if user["api_token_id"] != "ATADMIN" {
		t.Fatalf("api token id = %#v, want ATADMIN", user["api_token_id"])
	}
	if spy.listCalls != 0 {
		t.Fatalf("api token auth called List %d times, want indexed Get only", spy.listCalls)
	}
}

func TestRemoteIdentityAuthContractsAuthorizeProtectedRoutes(t *testing.T) {
	serviceKey := "svc-" + t.Name()
	sessionToken := "session-" + t.Name()
	apiToken := "api-" + t.Name()
	paths := []string{}
	identity := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if r.Header.Get("X-Service-Key") != serviceKey {
			WriteError(w, r, http.StatusUnauthorized, "unauthorized", "service authentication is required")
			return
		}
		var payload map[string]string
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			WriteError(w, r, http.StatusBadRequest, "invalid_request", "invalid JSON")
			return
		}
		switch {
		case r.URL.Path == internalIdentitySessionAuth && payload["token"] == sessionToken:
			WriteJSON(w, r, http.StatusOK, map[string]any{"user": remoteAdminUser()})
		case r.URL.Path == internalIdentityAPITokenAuth && payload["token"] == apiToken:
			WriteJSON(w, r, http.StatusOK, map[string]any{"user": remoteAdminUser(), "api_token_id": "AT_REMOTE"})
		default:
			WriteError(w, r, http.StatusUnauthorized, "unauthorized", "credential is invalid")
		}
	}))
	defer identity.Close()

	app := NewApp(Config{
		ServiceName:   "usage-observability-service",
		HTTPAddr:      ":0",
		RequireAuth:   true,
		ServiceURLs:   map[string]string{identityServiceName: identity.URL},
		ServiceAPIKey: serviceKey,
		ExternalURLs:  map[string]string{},
	})
	adminRoute := RouteSpec{Method: http.MethodGet, Pattern: "/admin", Resource: "test:admin", Action: "list", AuthRequired: true, Admin: true}
	app.Routes = []RouteSpec{adminRoute}
	app.RegisterCustomHandler(http.MethodGet, "/admin", echoAuthHandler)

	sessionUser := requestAuthTest(t, app, http.MethodGet, "/admin", map[string]string{"Authorization": "Bearer " + sessionToken}, http.StatusOK)
	if sessionUser["user_id"] != "remote-admin" || sessionUser["role"] != "admin" {
		t.Fatalf("remote session principal = %#v, want admin", sessionUser)
	}
	apiUser := requestAuthTest(t, app, http.MethodGet, "/admin", map[string]string{"Authorization": "Bearer " + apiToken}, http.StatusOK)
	if apiUser["api_token_id"] != "AT_REMOTE" {
		t.Fatalf("remote API token principal = %#v, want token id", apiUser)
	}
	for _, path := range paths {
		if path == internalRecordsPath {
			t.Fatalf("remote identity auth used generic records fallback: paths=%#v", paths)
		}
	}
}

func TestRequireAuthFalseKeepsHeaderModeCompatibility(t *testing.T) {
	app := NewApp(Config{ServiceName: "all", HTTPAddr: ":0", DevHeaderAuth: true, ExternalURLs: map[string]string{}})
	route := RouteSpec{Method: http.MethodGet, Pattern: "/admin", Resource: "test:admin", Action: "list", AuthRequired: true, Admin: true}
	app.Routes = []RouteSpec{route}
	app.RegisterCustomHandler(http.MethodGet, "/admin", echoAuthHandler)

	data := requestAuthTest(t, app, http.MethodGet, "/admin", map[string]string{
		"X-User-ID":   "dev-admin",
		"X-User-Role": "admin",
	}, http.StatusOK)
	if data["user_id"] != "dev-admin" || data["role"] != "admin" {
		t.Fatalf("RequireAuth=false should keep header-mode compatibility: %#v", data)
	}
}

func TestAPIKeyPrincipalBindsVerifiedIdentityAndAdmin(t *testing.T) {
	app := NewApp(Config{
		ServiceName: "all",
		HTTPAddr:    ":0",
		RequireAuth: true,
		APIKeys: map[string]bool{
			"service-key": true,
			"admin-key":   true,
		},
		APIKeyPrincipals: map[string]APIKeyPrincipal{
			"service-key": {ID: "svc:catalog", Username: "catalog-bot", Role: "service", Scopes: []string{"test:read"}},
			"admin-key":   {ID: "svc:ops-admin", Username: "ops-admin", Admin: true, Scopes: []string{"admin"}},
		},
		ExternalURLs: map[string]string{},
	})
	adminRoute := RouteSpec{Method: http.MethodGet, Pattern: "/admin", Resource: "test:admin", Action: "list", AuthRequired: true, Admin: true}
	userRoute := RouteSpec{Method: http.MethodGet, Pattern: "/user", Resource: "test:user", Action: "list", AuthRequired: true}
	app.Routes = []RouteSpec{adminRoute, userRoute}
	app.RegisterCustomHandler(http.MethodGet, "/admin", echoAuthHandler)
	app.RegisterCustomHandler(http.MethodGet, "/user", echoAuthHandler)

	service := requestAuthTest(t, app, http.MethodGet, "/user", map[string]string{
		"X-API-Key":   "service-key",
		"X-User-ID":   "forged-admin",
		"X-User-Role": "admin",
	}, http.StatusOK)
	if service["user_id"] != "svc:catalog" || service["username"] != "catalog-bot" || service["role"] != "service" {
		t.Fatalf("service API key did not bind configured principal: %#v", service)
	}
	requestAuthTest(t, app, http.MethodGet, "/admin", map[string]string{"X-API-Key": "service-key"}, http.StatusForbidden)

	admin := requestAuthTest(t, app, http.MethodGet, "/admin", map[string]string{"X-API-Key": "admin-key"}, http.StatusOK)
	if admin["user_id"] != "svc:ops-admin" || admin["role"] != "admin" {
		t.Fatalf("admin API key principal did not satisfy admin gate: %#v", admin)
	}
	adminRead := requestAuthTest(t, app, http.MethodGet, "/user", map[string]string{"X-API-Key": "admin-key"}, http.StatusOK)
	if adminRead["user_id"] != "svc:ops-admin" {
		t.Fatalf("admin API key scope did not allow non-admin route: %#v", adminRead)
	}
}

func TestAPIKeyPrincipalNormalizedExportsRequestAuthSemantics(t *testing.T) {
	principal := APIKeyPrincipal{UserID: "ops-admin", Name: "Operations Admin", Role: "superadmin", Scopes: []string{"admin"}}
	normalized := principal.Normalized()

	if normalized.ID != "ops-admin" || normalized.Username != "Operations Admin" || normalized.Role != "admin" || !normalized.Admin {
		t.Fatalf("Normalized() = %#v, want request auth principal semantics", normalized)
	}
}

func TestAPIKeyPrincipalScopesRestrictRoutes(t *testing.T) {
	app := NewApp(Config{
		ServiceName: "all",
		HTTPAddr:    ":0",
		RequireAuth: true,
		APIKeys:     map[string]bool{"reader-key": true},
		APIKeyPrincipals: map[string]APIKeyPrincipal{
			"reader-key": {ID: "svc:reader", Username: "reader", Role: "service", Scopes: []string{"test:read"}},
		},
		ExternalURLs: map[string]string{},
	})
	readRoute := RouteSpec{Method: http.MethodGet, Pattern: "/records", Resource: "test:records", OperationID: "records_list", AuthRequired: true}
	writeRoute := RouteSpec{Method: http.MethodPost, Pattern: "/records", Resource: "test:records", OperationID: "records_create", AuthRequired: true, StateChanging: true}
	app.Routes = []RouteSpec{readRoute, writeRoute}
	app.RegisterCustomHandler(http.MethodGet, "/records", echoAuthHandler)
	app.RegisterCustomHandler(http.MethodPost, "/records", echoAuthHandler)

	reader := requestAuthTest(t, app, http.MethodGet, "/records", map[string]string{"X-API-Key": "reader-key"}, http.StatusOK)
	if reader["user_id"] != "svc:reader" {
		t.Fatalf("reader key was not applied as verified principal: %#v", reader)
	}
	requestAuthTest(t, app, http.MethodPost, "/records", map[string]string{"X-API-Key": "reader-key"}, http.StatusForbidden)
}

func remoteAdminUser() map[string]any {
	return map[string]any{"id": "remote-admin", "username": "remote-admin", "role": "admin", "system_role": 0, "status": "online"}
}

func newAuthTestApp() *App {
	app := NewApp(Config{
		ServiceName:  "all",
		HTTPAddr:     ":0",
		RequireAuth:  true,
		APIKeys:      map[string]bool{"test-key": true},
		ExternalURLs: map[string]string{},
	})
	now := time.Now().UTC()
	_, _ = app.Store.Create(nil, "identity-service:users", map[string]any{"id": "admin", "username": "admin", "role": "admin", "system_role": 0, "status": "online"})
	_, _ = app.Store.Create(nil, "identity-service:users", map[string]any{"id": "user", "username": "user", "role": "user", "system_role": 2, "status": "online"})
	_, _ = app.Store.Create(nil, "identity-service:users", map[string]any{"id": "capability-admin", "username": "capability-admin", "role": "user", "system_role": 2, "status": "online", "capabilities": map[string]any{"adminPanel": true}})
	_, _ = app.Store.Create(nil, "identity-service:users", map[string]any{"id": "panel-admin", "username": "panel-admin", "role": "user", "system_role": 2, "status": "online", "admin_panel": true})
	_, _ = app.Store.Create(nil, "identity-service:sessions", map[string]any{"id": "admin-session", "user_id": "admin", "expires_at": now.Add(time.Hour).Format(time.RFC3339)})
	_, _ = app.Store.Create(nil, "identity-service:sessions", map[string]any{"id": "user-session", "user_id": "user", "expires_at": now.Add(time.Hour).Format(time.RFC3339)})
	_, _ = app.Store.Create(nil, "identity-service:sessions", map[string]any{"id": "capability-session", "user_id": "capability-admin", "expires_at": now.Add(time.Hour).Format(time.RFC3339)})
	_, _ = app.Store.Create(nil, "identity-service:sessions", map[string]any{"id": "panel-session", "user_id": "panel-admin", "expires_at": now.Add(time.Hour).Format(time.RFC3339)})
	_, _ = app.Store.Create(nil, "identity-service:api_tokens", map[string]any{"id": "ATADMIN", "user_id": "admin", "token_hash": HashSecret(FormatUserAPIToken("ATADMIN", "admin-secret")), "expires_at": now.Add(time.Hour).Format(time.RFC3339), "revoked": false})
	return app
}

func echoAuthHandler(_ *App, r *http.Request, _ RouteSpec) (int, any, *Degraded) {
	return http.StatusOK, map[string]any{
		"user_id":      r.Header.Get("X-User-ID"),
		"username":     r.Header.Get("X-Username"),
		"role":         r.Header.Get("X-User-Role"),
		"api_token_id": r.Header.Get("X-API-Token-ID"),
	}, nil
}

func requestAuthTest(t *testing.T, app *App, method, path string, headers map[string]string, want int) map[string]any {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, nil)
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	app.ServeHTTP(rec, req)
	if rec.Code != want {
		t.Fatalf("%s %s returned %d, want %d: %s", method, path, rec.Code, want, rec.Body.String())
	}
	if rec.Code >= 400 {
		return nil
	}
	var env struct {
		Data map[string]any `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatal(err)
	}
	return env.Data
}

type authListSpyStore struct {
	RecordStore
	listCalls int
}

func (s *authListSpyStore) List(ctx context.Context, resource string) []contracts.Record[map[string]any] {
	s.listCalls++
	return s.RecordStore.List(ctx, resource)
}
