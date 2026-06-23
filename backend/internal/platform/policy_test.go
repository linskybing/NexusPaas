package platform

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
)

func TestValidatePolicyDecisionPointFlagsAllowAllWhenAuthRequired(t *testing.T) {
	app := NewApp(Config{RequireAuth: true})
	err := app.ValidatePolicyDecisionPoint()
	if err == nil || !strings.Contains(err.Error(), "AllowAllPDP") {
		t.Fatalf("ValidatePolicyDecisionPoint() = %v, want AllowAllPDP error", err)
	}
}

func TestValidatePolicyDecisionPointAllowsInjectedPDP(t *testing.T) {
	app := NewApp(Config{RequireAuth: true}, WithPDP(StaticPDP{Allowed: false, Reason: "test"}))
	if err := app.ValidatePolicyDecisionPoint(); err != nil {
		t.Fatalf("ValidatePolicyDecisionPoint() error = %v, want nil", err)
	}
}

func TestValidatePolicyDecisionPointSkipsAuthOffRuntime(t *testing.T) {
	app := NewApp(Config{RequireAuth: false})
	if err := app.ValidatePolicyDecisionPoint(); err != nil {
		t.Fatalf("ValidatePolicyDecisionPoint() error = %v, want nil when auth is disabled", err)
	}
}

func TestNewAppInstallsRemotePDPWhenConfigured(t *testing.T) {
	app := NewApp(Config{RequireAuth: true, AuthorizationPolicyURL: "http://policy.test", AuthorizationPolicyAPIKey: "secret"})
	if _, ok := app.PDP.(RemotePDP); !ok {
		t.Fatalf("PDP = %T, want RemotePDP", app.PDP)
	}
	if err := app.ValidatePolicyDecisionPoint(); err != nil {
		t.Fatalf("ValidatePolicyDecisionPoint() error = %v, want nil for remote PDP", err)
	}
}

func TestRemotePDPEnforceUsesServiceAPIKeyAndEnvelope(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertRemotePDPRequest(t, r)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data":    contracts.Decision{Allowed: true, Reason: "matched", Version: 7},
		})
	}))
	defer server.Close()

	decision, err := NewRemotePDP(server.URL, "secret", 0, Config{
		ServiceIdentityName: "compute-api",
		ServiceIdentityKey:  "scoped-secret",
	}).Enforce(t.Context(), "alice", "proj", "model", "read")
	if err != nil {
		t.Fatal(err)
	}
	if !decision.Allowed || decision.Version != 7 || decision.Reason != "matched" {
		t.Fatalf("decision = %#v, want allowed envelope decision", decision)
	}
}

func TestRemotePDPStrictRuntimeDoesNotSendLegacyServiceKey(t *testing.T) {
	var gotName, gotServiceKey, gotAPIKey string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotName = r.Header.Get(serviceNameHeader)
		gotServiceKey = r.Header.Get(serviceKeyHeader)
		gotAPIKey = r.Header.Get("X-API-Key")
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	_, _ = NewRemotePDP(server.URL, "policy-key", 0, Config{
		EnvironmentProfile: runtimeProfileStaging,
		ServiceAPIKey:      "legacy-key",
	}).Enforce(t.Context(), "alice", "proj", "model", "read")
	if gotName != "" || gotServiceKey != "" {
		t.Fatalf("strict legacy-only PDP service headers name=%q key=%q, want none", gotName, gotServiceKey)
	}
	if gotAPIKey != "policy-key" {
		t.Fatalf("X-API-Key = %q, want policy compatibility key", gotAPIKey)
	}
}

func assertRemotePDPRequest(t *testing.T, r *http.Request) {
	t.Helper()
	if r.Method != http.MethodPost || r.URL.Path != remotePDPEnforcePath {
		t.Fatalf("remote PDP request = %s %s, want POST %s", r.Method, r.URL.Path, remotePDPEnforcePath)
	}
	if got := r.Header.Get("X-Service-Name"); got != "compute-api" {
		t.Fatalf("X-Service-Name = %q, want scoped service identity", got)
	}
	if got := r.Header.Get("X-Service-Key"); got != "scoped-secret" {
		t.Fatalf("X-Service-Key = %q, want scoped service identity key", got)
	}
	if got := r.Header.Get("X-API-Key"); got != "secret" {
		t.Fatalf("X-API-Key = %q, want rolling-upgrade compatibility API key", got)
	}
	var payload map[string]string
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload["sub"] != "alice" || payload["dom"] != "proj" || payload["obj"] != "model" || payload["act"] != "read" {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestRemotePDPEnforceFailsClosedOnUpstreamError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	decision, err := NewRemotePDP(server.URL, "secret", 0).Enforce(t.Context(), "alice", "", "model", "read")
	if err == nil {
		t.Fatal("Enforce() error = nil, want upstream error")
	}
	if decision.Allowed {
		t.Fatalf("decision = %#v, want fail-closed denial", decision)
	}
}

func TestPrincipalScopesAllowRecognizesRouteActionScopes(t *testing.T) {
	route := RouteSpec{
		Method:      http.MethodPost,
		Resource:    "authorization-policy-service:permissions",
		Action:      "enforce",
		OperationID: "post_authorization_policy_service_api_v1_permissions_enforce",
	}
	adminRoute := route
	adminRoute.Admin = true

	tests := []struct {
		name  string
		user  map[string]any
		route RouteSpec
		want  bool
	}{
		{
			name:  "resource action scope",
			user:  scopedUser("svc", "service", "permissions:enforce"),
			route: route,
			want:  true,
		},
		{
			name:  "service action scope",
			user:  scopedUser("svc", "service", "authorization-policy-service:enforce"),
			route: route,
			want:  true,
		},
		{
			name:  "operation id scope still works",
			user:  scopedUser("svc", "service", "post_authorization_policy_service_api_v1_permissions_enforce"),
			route: route,
			want:  true,
		},
		{
			name:  "wildcard scope still works",
			user:  scopedUser("svc", "service", "*"),
			route: route,
			want:  true,
		},
		{
			name:  "admin scope still requires admin principal",
			user:  scopedAdmin("admin", "admin"),
			route: adminRoute,
			want:  true,
		},
		{
			name:  "unmatched scope denies",
			user:  scopedUser("svc", "service", "permissions:read"),
			route: route,
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/permissions/enforce", nil)
			setVerifiedUser(req, tt.user)
			if got := principalScopesAllow(req, tt.route); got != tt.want {
				t.Fatalf("principalScopesAllow() = %v, want %v", got, tt.want)
			}
		})
	}
}

func scopedUser(id, role string, scopes ...string) map[string]any {
	return map[string]any{
		"id":          id,
		"username":    id,
		"role":        role,
		"system_role": 2,
		"status":      "online",
		"scopes":      scopes,
	}
}

func scopedAdmin(id, role string) map[string]any {
	user := scopedUser(id, role, "admin")
	user["system_role"] = 0
	user["admin_panel"] = true
	return user
}

func TestOperationalEndpointsBypassPDPForVerifiedAdmin(t *testing.T) {
	app := NewApp(
		Config{
			RequireAuth: true,
			APIKeys:     map[string]bool{"admin-key": true},
			APIKeyPrincipals: map[string]APIKeyPrincipal{
				"admin-key": {ID: "admin", Username: "admin", Role: "admin", Admin: true},
			},
		},
		WithPDP(StaticPDP{Allowed: false, Reason: "deny all"}),
	)

	for _, path := range []string{"/metrics", "/openapi.json", "/service-registry"} {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			req.Header.Set("X-API-Key", "admin-key")
			rec := httptest.NewRecorder()

			app.Mux.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("%s status = %d body = %s, want 200 despite denying PDP", path, rec.Code, rec.Body.String())
			}
		})
	}
}

func TestOperationalEndpointsStillRequireAdminWhenPolicyBypassed(t *testing.T) {
	app := NewApp(
		Config{
			RequireAuth: true,
			APIKeys:     map[string]bool{"service-key": true},
			APIKeyPrincipals: map[string]APIKeyPrincipal{
				"service-key": {ID: "svc", Username: "svc", Role: "service"},
			},
		},
		WithPDP(StaticPDP{Allowed: true, Reason: "would allow"}),
	)
	req := httptest.NewRequest(http.MethodGet, "/service-registry", nil)
	req.Header.Set("X-API-Key", "service-key")
	rec := httptest.NewRecorder()

	app.Mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d body = %s, want admin-gate 403", rec.Code, rec.Body.String())
	}
}
