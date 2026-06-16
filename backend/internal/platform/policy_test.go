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
		if r.Method != http.MethodPost || r.URL.Path != remotePDPEnforcePath {
			t.Fatalf("remote PDP request = %s %s, want POST %s", r.Method, r.URL.Path, remotePDPEnforcePath)
		}
		if got := r.Header.Get("X-API-Key"); got != "secret" {
			t.Fatalf("X-API-Key = %q, want service API key", got)
		}
		var payload map[string]string
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload["sub"] != "alice" || payload["dom"] != "proj" || payload["obj"] != "model" || payload["act"] != "read" {
			t.Fatalf("payload = %#v", payload)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data":    contracts.Decision{Allowed: true, Reason: "matched", Version: 7},
		})
	}))
	defer server.Close()

	decision, err := NewRemotePDP(server.URL, "secret", 0).Enforce(t.Context(), "alice", "proj", "model", "read")
	if err != nil {
		t.Fatal(err)
	}
	if !decision.Allowed || decision.Version != 7 || decision.Reason != "matched" {
		t.Fatalf("decision = %#v, want allowed envelope decision", decision)
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
