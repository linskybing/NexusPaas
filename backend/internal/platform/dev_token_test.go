package platform

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func devApp() *App {
	return NewApp(Config{RequireAuth: true, DevAuthSigningKey: "dev-secret-key"})
}

func TestDevTokenSignVerifyRoundTrip(t *testing.T) {
	signer := newDevTokenSigner(Config{DevAuthSigningKey: "k"})
	if signer == nil {
		t.Fatal("signer should be non-nil for non-production with key")
	}
	token := signer.sign(devTokenClaims{Subject: "u1", Role: "admin", Admin: true, Expires: nowUnixPlus(3600)})
	claims, err := signer.verify(token)
	if err != nil || claims.Subject != "u1" || !claims.Admin {
		t.Fatalf("verify = %#v err=%v, want u1/admin", claims, err)
	}
}

func TestDevTokenSignerNilInProductionOrWithoutKey(t *testing.T) {
	if newDevTokenSigner(Config{Production: true, DevAuthSigningKey: "k"}) != nil {
		t.Fatal("signer must be nil in production")
	}
	if newDevTokenSigner(Config{}) != nil {
		t.Fatal("signer must be nil without a key")
	}
}

func TestDevTokenRejectsTamperedAndExpired(t *testing.T) {
	signer := newDevTokenSigner(Config{DevAuthSigningKey: "k"})
	valid := signer.sign(devTokenClaims{Subject: "u1", Expires: nowUnixPlus(3600)})

	// Tamper the payload but keep the old signature.
	tampered := "ZXZpbA." + strings.SplitN(valid, ".", 2)[1]
	if _, err := signer.verify(tampered); err == nil {
		t.Fatal("tampered token must fail verification")
	}
	// Wrong key cannot verify.
	other := newDevTokenSigner(Config{DevAuthSigningKey: "different"})
	if _, err := other.verify(valid); err == nil {
		t.Fatal("token signed with a different key must fail")
	}
	// Expired token rejected.
	expired := signer.sign(devTokenClaims{Subject: "u1", Expires: nowUnixPlus(-10)})
	if _, err := signer.verify(expired); err == nil {
		t.Fatal("expired token must fail verification")
	}
}

func TestAuthorizeDevTokenSetsPrincipalAndDeniesForgedHeaders(t *testing.T) {
	app := devApp()
	token := app.devTokenSigner.sign(devTokenClaims{Subject: "dev1", Role: "admin", Admin: true, Expires: nowUnixPlus(3600)})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/x", nil)
	// A forged identity header must be ignored; identity comes from the token.
	req.Header.Set("X-User-ID", "attacker")
	req.Header.Set("Authorization", "Bearer "+token)

	if !app.authorized(req, RouteSpec{AuthRequired: true}) {
		t.Fatal("valid dev token should authorize")
	}
	if got := req.Header.Get("X-User-ID"); got != "dev1" {
		t.Fatalf("X-User-ID = %q, want dev1 (forged header must not survive)", got)
	}
}

func TestDevTokenMintEndpoint(t *testing.T) {
	app := devApp()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/dev/token", strings.NewReader(`{"username":"alice","role":"admin","admin":true}`))
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("mint status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	token := out.Data.Token
	claims, err := app.devTokenSigner.verify(token)
	if err != nil || claims.Subject != "alice" || !claims.Admin {
		t.Fatalf("minted token claims = %#v err=%v, want alice/admin", claims, err)
	}
}

func TestDevTokenEndpointAbsentWhenDisabled(t *testing.T) {
	app := NewApp(Config{RequireAuth: true, APIKeys: map[string]bool{"k": true}})
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/dev/token", strings.NewReader("{}")))
	if rec.Code == http.StatusOK {
		t.Fatal("dev token endpoint must not exist without a dev signing key")
	}
}

func TestDevAuthSigningKeyRejectedInProduction(t *testing.T) {
	cfg := withRuntimeDefaults(withProductionBacking(Config{
		Production: true, RequireAuth: true, APIKeys: map[string]bool{testAPIKey: true},
		APIKeyPrincipals:  map[string]APIKeyPrincipal{testAPIKey: {ID: "svc"}},
		DevAuthSigningKey: "leak",
	}))
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "DEV_AUTH_SIGNING_KEY") {
		t.Fatalf("Validate() = %v, want DEV_AUTH_SIGNING_KEY production rejection", err)
	}
}

func nowUnixPlus(seconds int64) int64 {
	return time.Now().Unix() + seconds
}
