package platform

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// A verified JWT carrying a jti is rejected once RevokeBearer denylists that jti,
// giving OIDC token revocation even though Dex exposes no revocation endpoint.
func TestJWTDeniedAfterRevokeBearer(t *testing.T) {
	signer := newTestJWTSigner(t)
	jwks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(signer.jwks())
	}))
	t.Cleanup(jwks.Close)

	app := newJWTTestApp(jwks.URL)
	token := signer.token(t, map[string]any{
		jwtClaimSubject: testJWTSubject,
		"jti":           "jti-123",
	})
	auth := map[string]string{testAuthHeader: testBearer + token}

	requestAuthTest(t, app, http.MethodGet, testJWTPath, auth, http.StatusOK)

	if !app.RevokeBearer(context.Background(), token) {
		t.Fatal("RevokeBearer should accept a valid jwt carrying a jti")
	}
	requestAuthTest(t, app, http.MethodGet, testJWTPath, auth, http.StatusUnauthorized)
}

func TestRevokeBearerRejectsNonVerifiableToken(t *testing.T) {
	app := NewApp(Config{RequireAuth: true})
	if app.RevokeBearer(context.Background(), "not-a-jwt") {
		t.Fatal("RevokeBearer must return false without a verifiable jwt")
	}
}
