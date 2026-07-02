//go:build e2e

package e2e

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLiveTypedAPIAuthzAcrossFixtureFamilies replays every api/v1 contract
// fixture family against a live authz-on stack (SERVICE_NAME=all, RequireAuth,
// real Postgres/Redis) and asserts the authentication gate behavior the
// fixtures declare: auth_required routes fail closed with 401 for missing and
// for tampered credentials, public routes are not blocked by authn, and
// denials carry the platform response envelope. Live local-tier DATA evidence
// (typed fixture field parity itself is covered by the per-service static
// fixture tests).
func TestLiveTypedAPIAuthzAcrossFixtureFamilies(t *testing.T) {
	h := newHarness(t)
	stack := h.startExtraService("authz-live-"+h.runID, "all", nil)

	fixtureDir := filepath.Join("..", "contracts", "fixtures", "api", "v1")
	entries, err := os.ReadDir(fixtureDir)
	if err != nil {
		t.Fatalf("read fixture dir: %v", err)
	}
	type fixture struct {
		ContractName       string         `json:"contract_name"`
		Method             string         `json:"method"`
		Path               string         `json:"path"`
		Auth               string         `json:"auth"`
		AuthRequired       bool           `json:"auth_required"`
		ServiceKeyRequired bool           `json:"service_key_required"`
		RequestExample     map[string]any `json:"request_example"`
	}

	client := &http.Client{}
	do := func(f fixture, decorate func(*http.Request)) (int, map[string]any) {
		t.Helper()
		path := f.Path
		for strings.Contains(path, "{") {
			start := strings.Index(path, "{")
			end := strings.Index(path[start:], "}")
			if end < 0 {
				break
			}
			path = path[:start] + "authz-e2e-probe" + path[start+end+1:]
		}
		var body *bytes.Reader
		if f.RequestExample != nil && f.Method != http.MethodGet && f.Method != http.MethodDelete {
			raw, err := json.Marshal(f.RequestExample)
			if err != nil {
				t.Fatalf("%s: marshal example: %v", f.ContractName, err)
			}
			body = bytes.NewReader(raw)
		} else {
			body = bytes.NewReader(nil)
		}
		req, err := http.NewRequest(f.Method, stack.url+path, body)
		if err != nil {
			t.Fatalf("%s: build request: %v", f.ContractName, err)
		}
		req.Header.Set("Content-Type", "application/json")
		if decorate != nil {
			decorate(req)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("%s: request: %v", f.ContractName, err)
		}
		defer resp.Body.Close()
		envelope := map[string]any{}
		_ = json.NewDecoder(resp.Body).Decode(&envelope)
		return resp.StatusCode, envelope
	}

	families, protected, public := 0, 0, 0
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(fixtureDir, entry.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", entry.Name(), err)
		}
		var f fixture
		if err := json.Unmarshal(raw, &f); err != nil {
			t.Fatalf("parse %s: %v", entry.Name(), err)
		}
		families++

		if !f.AuthRequired {
			public++
			// a public route may still return 401 from its own handler (e.g. a
			// login attempt with wrong credentials) — what it must never do is
			// deny at the authn gate ("authentication is required")
			code, envelope := do(f, nil)
			if code == http.StatusForbidden {
				t.Fatalf("%s: public route blocked by authz: %d %#v", f.ContractName, code, envelope)
			}
			if code == http.StatusUnauthorized {
				if data, _ := envelope["data"].(map[string]any); strings.Contains(strings.ToLower(asEnvelopeText(data["message"])), "authentication is required") {
					t.Fatalf("%s: public route denied by the authn gate: %#v", f.ContractName, envelope)
				}
			}
			continue
		}
		protected++

		// missing credentials fail closed with the platform error envelope
		code, envelope := do(f, nil)
		if code != http.StatusUnauthorized {
			t.Fatalf("%s: no-credential status = %d, want 401", f.ContractName, code)
		}
		if success, ok := envelope["success"].(bool); !ok || success {
			t.Fatalf("%s: 401 envelope = %#v, want success=false platform envelope", f.ContractName, envelope)
		}
		if id, _ := envelope["request_id"].(string); id == "" {
			t.Fatalf("%s: 401 envelope missing request_id: %#v", f.ContractName, envelope)
		}

		// tampered bearer token is rejected, not treated as anonymous-allowed
		code, _ = do(f, func(r *http.Request) {
			r.Header.Set("Authorization", "Bearer forged-"+h.runID)
		})
		if code != http.StatusUnauthorized {
			t.Fatalf("%s: forged-bearer status = %d, want 401", f.ContractName, code)
		}

		// forged API key is rejected
		code, _ = do(f, func(r *http.Request) {
			r.Header.Set("X-API-Key", "forged-key-"+h.runID)
		})
		if code != http.StatusUnauthorized {
			t.Fatalf("%s: forged-api-key status = %d, want 401", f.ContractName, code)
		}
	}
	if families < 60 || protected < 55 || public == 0 {
		t.Fatalf("fixture sweep looks wrong: families=%d protected=%d public=%d", families, protected, public)
	}
	t.Logf("live authz sweep: %d fixture families (%d auth-required fail-closed ×3 probes, %d public unblocked)", families, protected, public)
}

func asEnvelopeText(v any) string {
	s, _ := v.(string)
	return s
}
