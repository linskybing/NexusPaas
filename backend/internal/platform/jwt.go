package platform

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	jwtCacheTTL       = 5 * time.Minute
	jwtClockSkew      = time.Minute
	jwtHTTPTimeout    = 3 * time.Second
	jwtMaxJWKSBytes   = 1 << 20
	jwtClaimAudience  = "aud"
	jwtClaimEmail     = "email"
	jwtClaimExpiry    = "exp"
	jwtClaimIssuer    = "iss"
	jwtClaimIssuedAt  = "iat"
	jwtClaimName      = "name"
	jwtClaimNotBefore = "nbf"
	jwtClaimPreferred = "preferred_username"
	jwtClaimSubject   = "sub"
)

var errJWTInvalid = errors.New("invalid jwt")

type jwtVerifier struct {
	jwksURL   string
	issuer    string
	audiences map[string]bool
	client    *http.Client
	now       func() time.Time
	ttl       time.Duration

	mu        sync.Mutex
	keys      map[string]jwtPublicKey
	fetchedAt time.Time
}

type jwtHeader struct {
	Algorithm string `json:"alg"`
	KeyID     string `json:"kid"`
}

type jwksDocument struct {
	Keys []jwkDocumentKey `json:"keys"`
}

type jwkDocumentKey struct {
	KeyType   string `json:"kty"`
	KeyID     string `json:"kid"`
	Algorithm string `json:"alg"`
	Use       string `json:"use"`
	Modulus   string `json:"n"`
	Exponent  string `json:"e"`
	Curve     string `json:"crv"`
	X         string `json:"x"`
	Y         string `json:"y"`
}

type jwtPublicKey struct {
	algorithm string
	value     any
}

func newJWTVerifier(cfg Config) *jwtVerifier {
	if strings.TrimSpace(cfg.JWKSURL) == "" {
		return nil
	}
	return &jwtVerifier{
		jwksURL:   strings.TrimSpace(cfg.JWKSURL),
		issuer:    strings.TrimSpace(cfg.JWTIssuer),
		audiences: cloneBoolMap(cfg.JWTAudiences),
		client:    &http.Client{Timeout: jwtHTTPTimeout},
		now:       func() time.Time { return time.Now().UTC() },
		ttl:       jwtCacheTTL,
		keys:      map[string]jwtPublicKey{},
	}
}

func (v *jwtVerifier) Verify(ctx context.Context, token string) (map[string]any, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errJWTInvalid
	}
	header, claims, err := parseJWT(parts)
	if err != nil {
		return nil, err
	}
	key, err := v.publicKey(ctx, header.KeyID, header.Algorithm)
	if err != nil {
		return nil, err
	}
	if err := verifyJWTSignature(header.Algorithm, key, parts[0]+"."+parts[1], parts[2]); err != nil {
		return nil, err
	}
	if err := v.validateClaims(claims); err != nil {
		return nil, err
	}
	return claims, nil
}

func parseJWT(parts []string) (jwtHeader, map[string]any, error) {
	headerBytes, err := decodeBase64URL(parts[0])
	if err != nil {
		return jwtHeader{}, nil, errJWTInvalid
	}
	claimBytes, err := decodeBase64URL(parts[1])
	if err != nil {
		return jwtHeader{}, nil, errJWTInvalid
	}
	var header jwtHeader
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return jwtHeader{}, nil, errJWTInvalid
	}
	claims := map[string]any{}
	if err := json.Unmarshal(claimBytes, &claims); err != nil {
		return jwtHeader{}, nil, errJWTInvalid
	}
	if !jwtAlgorithmSupported(header.Algorithm) || strings.TrimSpace(header.KeyID) == "" {
		return jwtHeader{}, nil, errJWTInvalid
	}
	return header, claims, nil
}

func (v *jwtVerifier) validateClaims(claims map[string]any) error {
	if v.issuer == "" || len(v.audiences) == 0 {
		return errors.New("jwt verifier is missing issuer or audience")
	}
	if jwtString(claims[jwtClaimIssuer]) != v.issuer {
		return errors.New("jwt issuer is not trusted")
	}
	if jwtString(claims[jwtClaimSubject]) == "" {
		return errors.New("jwt subject is required")
	}
	if !jwtAudienceAllowed(claims[jwtClaimAudience], v.audiences) {
		return errors.New("jwt audience is not trusted")
	}
	return v.validateClaimTimes(claims)
}

func (v *jwtVerifier) validateClaimTimes(claims map[string]any) error {
	now := v.now()
	expiry, ok, err := jwtNumericDate(claims[jwtClaimExpiry])
	if err != nil || !ok {
		return errors.New("jwt expiry is required")
	}
	if now.After(expiry.Add(jwtClockSkew)) {
		return errors.New("jwt is expired")
	}
	if notBefore, ok, err := jwtNumericDate(claims[jwtClaimNotBefore]); err != nil || (ok && now.Add(jwtClockSkew).Before(notBefore)) {
		return errors.New("jwt is not valid yet")
	}
	if issuedAt, ok, err := jwtNumericDate(claims[jwtClaimIssuedAt]); err != nil || (ok && issuedAt.After(now.Add(jwtClockSkew))) {
		return errors.New("jwt was issued in the future")
	}
	return nil
}

func (v *jwtVerifier) publicKey(ctx context.Context, keyID, algorithm string) (jwtPublicKey, error) {
	if key, ok := v.cachedKey(keyID, algorithm); ok {
		return key, nil
	}
	if err := v.refreshKeys(ctx); err != nil {
		return jwtPublicKey{}, err
	}
	if key, ok := v.cachedKey(keyID, algorithm); ok {
		return key, nil
	}
	return jwtPublicKey{}, fmt.Errorf("jwt signing key %q was not found", keyID)
}

func (v *jwtVerifier) cachedKey(keyID, algorithm string) (jwtPublicKey, bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	key, ok := v.keys[keyID]
	if !ok || v.now().Sub(v.fetchedAt) >= v.ttl {
		return jwtPublicKey{}, false
	}
	return key, key.algorithm == "" || key.algorithm == algorithm
}

func (v *jwtVerifier) refreshKeys(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.jwksURL, nil)
	if err != nil {
		return err
	}
	resp, err := v.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("jwks endpoint returned %d", resp.StatusCode)
	}
	keys, err := parseJWKS(io.LimitReader(resp.Body, jwtMaxJWKSBytes))
	if err != nil {
		return err
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	v.keys = keys
	v.fetchedAt = v.now()
	return nil
}

func parseJWKS(body io.Reader) (map[string]jwtPublicKey, error) {
	var doc jwksDocument
	if err := json.NewDecoder(body).Decode(&doc); err != nil {
		return nil, err
	}
	keys := map[string]jwtPublicKey{}
	for _, item := range doc.Keys {
		if key, ok := parseJWK(item); ok {
			keys[item.KeyID] = key
		}
	}
	if len(keys) == 0 {
		return nil, errors.New("jwks endpoint returned no signing keys")
	}
	return keys, nil
}

func parseJWK(item jwkDocumentKey) (jwtPublicKey, bool) {
	if item.KeyID == "" || (item.Use != "" && item.Use != "sig") {
		return jwtPublicKey{}, false
	}
	key, err := jwkValue(item)
	if err != nil {
		return jwtPublicKey{}, false
	}
	return jwtPublicKey{algorithm: item.Algorithm, value: key}, true
}

func jwkValue(item jwkDocumentKey) (any, error) {
	switch item.KeyType {
	case "RSA":
		return rsaPublicKeyFromJWK(item)
	case "EC":
		return ecdsaPublicKeyFromJWK(item)
	default:
		return nil, fmt.Errorf("unsupported jwk key type %q", item.KeyType)
	}
}

func rsaPublicKeyFromJWK(item jwkDocumentKey) (*rsa.PublicKey, error) {
	modulus, err := decodeBase64URL(item.Modulus)
	if err != nil {
		return nil, err
	}
	exponentBytes, err := decodeBase64URL(item.Exponent)
	if err != nil {
		return nil, err
	}
	exponent := 0
	for _, b := range exponentBytes {
		exponent = exponent<<8 + int(b)
	}
	if exponent == 0 || len(modulus) == 0 {
		return nil, errJWTInvalid
	}
	return &rsa.PublicKey{N: new(big.Int).SetBytes(modulus), E: exponent}, nil
}

func ecdsaPublicKeyFromJWK(item jwkDocumentKey) (*ecdsa.PublicKey, error) {
	curve := jwtCurve(item.Curve)
	if curve == nil {
		return nil, errJWTInvalid
	}
	x, err := decodeBase64URL(item.X)
	if err != nil {
		return nil, err
	}
	y, err := decodeBase64URL(item.Y)
	if err != nil {
		return nil, err
	}
	pub := &ecdsa.PublicKey{Curve: curve, X: new(big.Int).SetBytes(x), Y: new(big.Int).SetBytes(y)}
	if !curve.IsOnCurve(pub.X, pub.Y) {
		return nil, errJWTInvalid
	}
	return pub, nil
}

func verifyJWTSignature(algorithm string, key jwtPublicKey, signingInput, encodedSignature string) error {
	signature, err := decodeBase64URL(encodedSignature)
	if err != nil {
		return errJWTInvalid
	}
	digest, hashID, err := jwtDigest(algorithm, signingInput)
	if err != nil {
		return err
	}
	switch pub := key.value.(type) {
	case *rsa.PublicKey:
		return verifyRSASignature(algorithm, pub, hashID, digest, signature)
	case *ecdsa.PublicKey:
		return verifyECDSASignature(algorithm, pub, digest, signature)
	default:
		return errJWTInvalid
	}
}

func verifyRSASignature(algorithm string, pub *rsa.PublicKey, hashID crypto.Hash, digest, signature []byte) error {
	if !strings.HasPrefix(algorithm, "RS") {
		return errJWTInvalid
	}
	return rsa.VerifyPKCS1v15(pub, hashID, digest, signature)
}

func verifyECDSASignature(algorithm string, pub *ecdsa.PublicKey, digest, signature []byte) error {
	if !strings.HasPrefix(algorithm, "ES") {
		return errJWTInvalid
	}
	byteLen := (pub.Curve.Params().BitSize + 7) / 8
	if len(signature) != 2*byteLen {
		return errJWTInvalid
	}
	r := new(big.Int).SetBytes(signature[:byteLen])
	s := new(big.Int).SetBytes(signature[byteLen:])
	if !ecdsa.Verify(pub, digest, r, s) {
		return errJWTInvalid
	}
	return nil
}

func jwtDigest(algorithm, signingInput string) ([]byte, crypto.Hash, error) {
	input := []byte(signingInput)
	switch algorithm {
	case "RS256", "ES256":
		sum := sha256.Sum256(input)
		return sum[:], crypto.SHA256, nil
	case "RS384", "ES384":
		sum := sha512.Sum384(input)
		return sum[:], crypto.SHA384, nil
	case "RS512", "ES512":
		sum := sha512.Sum512(input)
		return sum[:], crypto.SHA512, nil
	default:
		return nil, 0, errJWTInvalid
	}
}

func jwtAlgorithmSupported(algorithm string) bool {
	switch algorithm {
	case "RS256", "RS384", "RS512", "ES256", "ES384", "ES512":
		return true
	default:
		return false
	}
}

func jwtCurve(name string) elliptic.Curve {
	switch name {
	case "P-256":
		return elliptic.P256()
	case "P-384":
		return elliptic.P384()
	case "P-521":
		return elliptic.P521()
	default:
		return nil
	}
}

func jwtNumericDate(value any) (time.Time, bool, error) {
	switch v := value.(type) {
	case float64:
		return time.Unix(int64(v), 0).UTC(), true, nil
	case json.Number:
		seconds, err := v.Int64()
		return time.Unix(seconds, 0).UTC(), err == nil, err
	case nil:
		return time.Time{}, false, nil
	default:
		return time.Time{}, false, errJWTInvalid
	}
}

func jwtAudienceAllowed(value any, allowed map[string]bool) bool {
	for _, audience := range jwtStrings(value) {
		if allowed[audience] {
			return true
		}
	}
	return false
}

func jwtClaimsUser(claims map[string]any) map[string]any {
	user := map[string]any{
		"id":          jwtString(claims[jwtClaimSubject]),
		"username":    jwtUsername(claims),
		"role":        jwtPrincipalRole(claims),
		"system_role": 2,
		"status":      "online",
	}
	if email := jwtString(claims[jwtClaimEmail]); email != "" {
		user["email"] = email
	}
	if jwtClaimsGrantAdmin(claims) {
		user["role"] = "admin"
		user["system_role"] = 0
		user["admin_panel"] = true
	}
	return user
}

func jwtUsername(claims map[string]any) string {
	return firstNonEmpty(
		jwtString(claims[jwtClaimPreferred]),
		jwtString(claims[jwtClaimName]),
		jwtString(claims[jwtClaimEmail]),
		jwtString(claims[jwtClaimSubject]),
	)
}

func jwtPrincipalRole(claims map[string]any) string {
	for _, claimName := range []string{"role", "roles", "groups"} {
		for _, role := range jwtStrings(claims[claimName]) {
			if role != "" {
				return role
			}
		}
	}
	return "user"
}

func jwtClaimsGrantAdmin(claims map[string]any) bool {
	if authBool(claims["admin"]) || authBool(claims["admin_panel"]) || authBool(claims["adminPanel"]) {
		return true
	}
	for _, claimName := range []string{"role", "roles", "groups"} {
		for _, role := range jwtStrings(claims[claimName]) {
			if jwtRoleGrantsAdmin(role) {
				return true
			}
		}
	}
	return false
}

func jwtRoleGrantsAdmin(role string) bool {
	switch strings.ToLower(strings.Trim(role, "/ ")) {
	case "admin", "superadmin", "root", "platform-admin":
		return true
	default:
		return false
	}
}

func jwtStrings(value any) []string {
	switch v := value.(type) {
	case string:
		return []string{v}
	case []string:
		return v
	case []any:
		return jwtStringsFromAny(v)
	default:
		return nil
	}
}

func jwtStringsFromAny(values []any) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if text := jwtString(value); text != "" {
			out = append(out, text)
		}
	}
	return out
}

func jwtString(value any) string {
	text, _ := value.(string)
	return strings.TrimSpace(text)
}

func decodeBase64URL(value string) ([]byte, error) {
	data, err := base64.RawURLEncoding.DecodeString(value)
	if err == nil {
		return data, nil
	}
	return base64.URLEncoding.DecodeString(value)
}

func cloneBoolMap(in map[string]bool) map[string]bool {
	out := map[string]bool{}
	for key, value := range in {
		out[key] = value
	}
	return out
}
