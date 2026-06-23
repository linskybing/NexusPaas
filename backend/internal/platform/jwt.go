package platform

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
)

const (
	jwtClockSkew      = time.Minute
	jwtHTTPTimeout    = 3 * time.Second
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

var jwtSupportedSigningAlgs = []string{
	oidc.RS256,
	oidc.RS384,
	oidc.RS512,
	oidc.ES256,
	oidc.ES384,
	oidc.ES512,
}

type jwtVerifier struct {
	issuer    string
	audiences map[string]bool
	verifier  *oidc.IDTokenVerifier
	now       func() time.Time
}

func newJWTVerifier(cfg Config) *jwtVerifier {
	jwksURL := strings.TrimSpace(cfg.JWKSURL)
	if jwksURL == "" {
		return nil
	}
	issuer := strings.TrimSpace(cfg.JWTIssuer)
	client := &http.Client{Timeout: jwtHTTPTimeout}
	keySet := oidc.NewRemoteKeySet(oidc.ClientContext(context.Background(), client), jwksURL)
	config := &oidc.Config{
		SupportedSigningAlgs: append([]string(nil), jwtSupportedSigningAlgs...),
		SkipClientIDCheck:    true,
		SkipExpiryCheck:      true,
	}
	return &jwtVerifier{
		issuer:    issuer,
		audiences: cloneBoolMap(cfg.JWTAudiences),
		verifier:  oidc.NewVerifier(issuer, keySet, config),
		now:       func() time.Time { return time.Now().UTC() },
	}
}

func (v *jwtVerifier) Verify(ctx context.Context, token string) (map[string]any, error) {
	idToken, err := v.verifier.Verify(ctx, token)
	if err != nil {
		return nil, err
	}
	claims := map[string]any{}
	if err := idToken.Claims(&claims); err != nil {
		return nil, errJWTInvalid
	}
	if err := v.validateClaims(claims); err != nil {
		return nil, err
	}
	return claims, nil
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

func cloneBoolMap(in map[string]bool) map[string]bool {
	out := map[string]bool{}
	for key, value := range in {
		out[key] = value
	}
	return out
}
