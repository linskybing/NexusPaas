//go:build integration

package platform

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
)

func requireDexURL(t *testing.T) string {
	t.Helper()
	u := os.Getenv("DEX_URL")
	if u == "" {
		t.Skip("DEX_URL not set; skipping Dex OIDC integration test")
	}
	return strings.TrimRight(u, "/")
}

// dexPasswordToken obtains an id_token from Dex via the resource-owner password
// grant configured in deploy/local/dex.yaml.
func dexPasswordToken(t *testing.T, dexURL string) string {
	t.Helper()
	username, password := dexTestCredentials(t)
	form := url.Values{
		"client_id":  {"platform"},
		"grant_type": {"password"},
		"scope":      {"openid email"},
		"username":   {username},
		"password":   {password},
	}
	req, err := http.NewRequest(http.MethodPost, dexURL+"/token", strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatalf("token request: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("token call: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("token endpoint HTTP %d: %s", resp.StatusCode, body)
	}
	var out struct {
		IDToken string `json:"id_token"`
	}
	if err := json.Unmarshal(body, &out); err != nil || out.IDToken == "" {
		t.Fatalf("no id_token in response: %s", body)
	}
	return out.IDToken
}

func dexTestCredentials(t *testing.T) (string, string) {
	t.Helper()
	username := os.Getenv("DEX_TEST_USERNAME")
	password := os.Getenv("DEX_TEST_PASSWORD")
	if username == "" || password == "" {
		t.Skip("DEX_TEST_USERNAME/DEX_TEST_PASSWORD not set; skipping Dex password-grant integration test")
	}
	return username, password
}

func TestDexTokenAuthorizesProtectedRoute(t *testing.T) {
	dexURL := requireDexURL(t)
	token := dexPasswordToken(t, dexURL)

	app := NewApp(Config{
		ServiceName:  "all",
		RequireAuth:  true,
		JWKSURL:      dexURL + "/keys",
		JWTIssuer:    dexURL,
		JWTAudiences: map[string]bool{"platform": true},
	})
	app.RegisterService(ServiceSpec{Name: "demo", Routes: []RouteSpec{
		{Method: http.MethodGet, Pattern: "/api/v1/whoami", Resource: "items", Action: "", AuthRequired: true},
	}})
	server := httptest.NewServer(app)
	defer server.Close()

	// Valid Dex token is accepted.
	if status := getWhoami(t, server.URL, "Bearer "+token); status != http.StatusOK {
		t.Fatalf("with Dex token status = %d, want 200", status)
	}
	// No credentials -> 401.
	if status := getWhoami(t, server.URL, ""); status != http.StatusUnauthorized {
		t.Fatalf("no token status = %d, want 401", status)
	}
	// Garbage token -> 401.
	if status := getWhoami(t, server.URL, "Bearer not-a-jwt"); status != http.StatusUnauthorized {
		t.Fatalf("bad token status = %d, want 401", status)
	}
}

func getWhoami(t *testing.T, base, auth string) int {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, base+"/api/v1/whoami", nil)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	defer resp.Body.Close()
	_, _ = io.ReadAll(resp.Body)
	return resp.StatusCode
}
