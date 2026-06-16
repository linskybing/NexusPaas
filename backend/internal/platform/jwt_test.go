package platform

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const (
	testJWTIssuer   = "https://issuer.test"
	testJWTAudience = "platform-api"
	testJWTKeyID    = "test-key"
	testJWTSubject  = "jwt-user"
	testJWTPath     = "/jwt"
	testJWTAdmin    = "/jwt-admin"
	testAuthHeader  = "Authorization"
	testBearer      = "Bearer "
	testFieldAlg    = "alg"
	testFieldKid    = "kid"
	testFieldTyp    = "typ"
)

func TestJWTBearerAuthorizesVerifiedPrincipal(t *testing.T) {
	signer := newTestJWTSigner(t)
	jwks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(signer.jwks())
	}))
	t.Cleanup(jwks.Close)

	app := newJWTTestApp(jwks.URL)
	token := signer.token(t, map[string]any{
		jwtClaimSubject:   testJWTSubject,
		jwtClaimPreferred: testJWTSubject,
		jwtClaimEmail:     "jwt-user@example.test",
	})

	data := requestAuthTest(t, app, http.MethodGet, testJWTPath, map[string]string{
		testAuthHeader: testBearer + token,
		"X-User-ID":    "forged-admin",
		"X-User-Role":  "admin",
	}, http.StatusOK)
	if data["user_id"] != testJWTSubject || data["username"] != testJWTSubject || data["role"] != "user" {
		t.Fatalf("jwt principal was not derived from verified claims: %#v", data)
	}

	cookieData := requestAuthTest(t, app, http.MethodGet, testJWTPath, map[string]string{
		"Cookie": "nexuspaas_jwt=" + token,
	}, http.StatusOK)
	if cookieData["user_id"] != testJWTSubject {
		t.Fatalf("nexuspaas_jwt cookie was not accepted as verified JWT: %#v", cookieData)
	}
}

func TestJWTRejectsInvalidAudienceAndSignature(t *testing.T) {
	signer := newTestJWTSigner(t)
	jwks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(signer.jwks())
	}))
	t.Cleanup(jwks.Close)
	app := newJWTTestApp(jwks.URL)

	badAudience := signer.token(t, map[string]any{jwtClaimAudience: "other-api"})
	requestAuthTest(t, app, http.MethodGet, testJWTPath, map[string]string{
		testAuthHeader: testBearer + badAudience,
	}, http.StatusUnauthorized)

	tampered := tamperJWTSignature(signer.token(t, nil))
	requestAuthTest(t, app, http.MethodGet, testJWTPath, map[string]string{
		testAuthHeader: testBearer + tampered,
	}, http.StatusUnauthorized)
}

func TestJWTAdminRoleUsesPlatformAdminGate(t *testing.T) {
	signer := newTestJWTSigner(t)
	jwks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(signer.jwks())
	}))
	t.Cleanup(jwks.Close)
	app := newJWTTestApp(jwks.URL)

	userToken := signer.token(t, map[string]any{"roles": []any{"developer"}})
	requestAuthTest(t, app, http.MethodGet, testJWTAdmin, map[string]string{
		testAuthHeader: testBearer + userToken,
	}, http.StatusForbidden)

	adminToken := signer.token(t, map[string]any{"roles": []any{"platform-admin"}})
	data := requestAuthTest(t, app, http.MethodGet, testJWTAdmin, map[string]string{
		testAuthHeader: testBearer + adminToken,
	}, http.StatusOK)
	if data["user_id"] != testJWTSubject || data["role"] != "admin" {
		t.Fatalf("jwt admin role did not satisfy platform admin gate: %#v", data)
	}
}

func TestJWTVerifierSupportsES256(t *testing.T) {
	signer := newTestECDSAJWTSigner(t)
	jwks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(signer.jwks())
	}))
	t.Cleanup(jwks.Close)

	claims, err := newTestJWTVerifier(jwks.URL).Verify(context.Background(), signer.token(t, nil))
	if err != nil {
		t.Fatalf("Verify() ES256 error = %v", err)
	}
	if jwtString(claims[jwtClaimSubject]) != testJWTSubject {
		t.Fatalf("ES256 claims subject = %#v", claims[jwtClaimSubject])
	}
}

func TestJWTVerifierRejectsMalformedTokensAndClaims(t *testing.T) {
	signer := newTestJWTSigner(t)
	jwks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(signer.jwks())
	}))
	t.Cleanup(jwks.Close)
	verifier := newTestJWTVerifier(jwks.URL)

	valid := signer.token(t, nil)
	validParts := strings.Split(valid, ".")
	unsupportedAlg := encodeJSONSegment(t, jwtHeaderMap("none")) + "." + validParts[1] + "." + validParts[2]
	cases := map[string]string{
		"malformed":        "not-a-jwt",
		"unsupported alg":  unsupportedAlg,
		"missing expiry":   signer.token(t, map[string]any{jwtClaimExpiry: nil}),
		"expired":          signer.token(t, map[string]any{jwtClaimExpiry: time.Now().Add(-2 * time.Hour).Unix()}),
		"future nbf":       signer.token(t, map[string]any{jwtClaimNotBefore: time.Now().Add(5 * time.Minute).Unix()}),
		"future iat":       signer.token(t, map[string]any{jwtClaimIssuedAt: time.Now().Add(5 * time.Minute).Unix()}),
		"wrong issuer":     signer.token(t, map[string]any{jwtClaimIssuer: "https://other.test"}),
		"missing subject":  signer.token(t, map[string]any{jwtClaimSubject: ""}),
		"missing audience": signer.token(t, map[string]any{jwtClaimAudience: nil}),
	}
	for name, token := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := verifier.Verify(context.Background(), token); err == nil {
				t.Fatal("Verify() error = nil, want rejection")
			}
		})
	}
}

func TestJWTVerifierRejectsBadJWKSAndAlgorithms(t *testing.T) {
	if _, err := parseJWKS(strings.NewReader(`{"keys":[]}`)); err == nil {
		t.Fatal("parseJWKS() error = nil for empty key set")
	}
	if _, ok := parseJWK(jwkDocumentKey{KeyID: "enc-key", Use: "enc", KeyType: "RSA"}); ok {
		t.Fatal("parseJWK() accepted non-signing key")
	}
	if _, ok := parseJWK(jwkDocumentKey{KeyID: "oct-key", Use: "sig", KeyType: "oct"}); ok {
		t.Fatal("parseJWK() accepted unsupported key type")
	}
	for _, alg := range []string{"RS384", "RS512", "ES384", "ES512"} {
		if digest, _, err := jwtDigest(alg, "input"); err != nil || len(digest) == 0 {
			t.Fatalf("jwtDigest(%q) = len %d, %v", alg, len(digest), err)
		}
	}
	if _, _, err := jwtDigest("HS256", "input"); err == nil {
		t.Fatal("jwtDigest() accepted unsupported HMAC algorithm")
	}
	for _, curve := range []string{"P-384", "P-521"} {
		if jwtCurve(curve) == nil {
			t.Fatalf("jwtCurve(%q) = nil", curve)
		}
	}
	if jwtCurve("P-999") != nil {
		t.Fatal("jwtCurve() accepted unsupported curve")
	}
	if apiKeyAllowed("presented", map[string]bool{"presented": false}) {
		t.Fatal("apiKeyAllowed() accepted disabled key")
	}
}

func newJWTTestApp(jwksURL string) *App {
	app := NewApp(Config{
		ServiceName:  "all",
		HTTPAddr:     ":0",
		RequireAuth:  true,
		JWKSURL:      jwksURL,
		JWTIssuer:    testJWTIssuer,
		JWTAudiences: map[string]bool{testJWTAudience: true},
		ExternalURLs: map[string]string{},
	})
	userRoute := RouteSpec{Method: http.MethodGet, Pattern: testJWTPath, Resource: "test:jwt", Action: "read", AuthRequired: true}
	adminRoute := RouteSpec{Method: http.MethodGet, Pattern: testJWTAdmin, Resource: "test:jwt-admin", Action: "read", AuthRequired: true, Admin: true}
	app.Routes = []RouteSpec{userRoute, adminRoute}
	app.RegisterCustomHandler(http.MethodGet, testJWTPath, echoAuthHandler)
	app.RegisterCustomHandler(http.MethodGet, testJWTAdmin, echoAuthHandler)
	return app
}

func newTestJWTVerifier(jwksURL string) *jwtVerifier {
	return newJWTVerifier(Config{
		JWKSURL:      jwksURL,
		JWTIssuer:    testJWTIssuer,
		JWTAudiences: map[string]bool{testJWTAudience: true},
	})
}

type testJWTSigner struct {
	private *rsa.PrivateKey
}

func newTestJWTSigner(t *testing.T) testJWTSigner {
	t.Helper()
	private, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return testJWTSigner{private: private}
}

func (s testJWTSigner) jwks() map[string]any {
	public := s.private.PublicKey
	return map[string]any{
		"keys": []map[string]any{{
			"kty":        "RSA",
			"use":        "sig",
			testFieldAlg: "RS256",
			testFieldKid: testJWTKeyID,
			"n":          encodeBase64URL(public.N.Bytes()),
			"e":          encodeBase64URL(big.NewInt(int64(public.E)).Bytes()),
		}},
	}
}

func (s testJWTSigner) token(t *testing.T, overrides map[string]any) string {
	t.Helper()
	claims := defaultJWTClaims()
	for key, value := range overrides {
		claims[key] = value
	}
	header := jwtHeaderMap("RS256")
	signingInput := encodeJSONSegment(t, header) + "." + encodeJSONSegment(t, claims)
	digest := sha256.Sum256([]byte(signingInput))
	signature, err := rsa.SignPKCS1v15(rand.Reader, s.private, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	return signingInput + "." + encodeBase64URL(signature)
}

type testECDSAJWTSigner struct {
	private *ecdsa.PrivateKey
}

func newTestECDSAJWTSigner(t *testing.T) testECDSAJWTSigner {
	t.Helper()
	private, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return testECDSAJWTSigner{private: private}
}

func (s testECDSAJWTSigner) jwks() map[string]any {
	public := s.private.PublicKey
	return map[string]any{
		"keys": []map[string]any{{
			"kty":        "EC",
			"use":        "sig",
			testFieldAlg: "ES256",
			testFieldKid: testJWTKeyID,
			"crv":        "P-256",
			"x":          encodeBase64URL(leftPadBigInt(public.X, 32)),
			"y":          encodeBase64URL(leftPadBigInt(public.Y, 32)),
		}},
	}
}

func (s testECDSAJWTSigner) token(t *testing.T, overrides map[string]any) string {
	t.Helper()
	claims := defaultJWTClaims()
	for key, value := range overrides {
		claims[key] = value
	}
	header := jwtHeaderMap("ES256")
	signingInput := encodeJSONSegment(t, header) + "." + encodeJSONSegment(t, claims)
	digest := sha256.Sum256([]byte(signingInput))
	r, sigS, err := ecdsa.Sign(rand.Reader, s.private, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	signature := append(leftPadBigInt(r, 32), leftPadBigInt(sigS, 32)...)
	return signingInput + "." + encodeBase64URL(signature)
}

func defaultJWTClaims() map[string]any {
	return map[string]any{
		jwtClaimIssuer:   testJWTIssuer,
		jwtClaimSubject:  testJWTSubject,
		jwtClaimAudience: testJWTAudience,
		jwtClaimExpiry:   time.Now().Add(time.Hour).Unix(),
	}
}

func jwtHeaderMap(algorithm string) map[string]any {
	return map[string]any{testFieldAlg: algorithm, testFieldKid: testJWTKeyID, testFieldTyp: "JWT"}
}

func encodeJSONSegment(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encodeBase64URL(data)
}

func encodeBase64URL(value []byte) string {
	return base64.RawURLEncoding.EncodeToString(value)
}

func leftPadBigInt(value *big.Int, size int) []byte {
	raw := value.Bytes()
	if len(raw) >= size {
		return raw
	}
	out := make([]byte, size)
	copy(out[size-len(raw):], raw)
	return out
}

func tamperJWTSignature(token string) string {
	parts := strings.Split(token, ".")
	signature, err := decodeBase64URL(parts[2])
	if err != nil || len(signature) == 0 {
		return token + "tampered"
	}
	signature[0] ^= 0xff
	return parts[0] + "." + parts[1] + "." + encodeBase64URL(signature)
}
