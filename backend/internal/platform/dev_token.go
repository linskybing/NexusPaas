package platform

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
)

// devTokenSigner mints and verifies short-lived, HMAC-SHA256-signed local
// development tokens. They are the secure replacement for DEV_HEADER_AUTH: in
// non-production a developer authenticates with a signed token whose claims are
// trusted only after the signature verifies, so handlers never trust raw client
// identity headers (finding 2). The signer is nil in production and when no
// DEV_AUTH_SIGNING_KEY is configured.
type devTokenSigner struct {
	key []byte
}

type devTokenClaims struct {
	Subject  string `json:"sub"`
	Username string `json:"username,omitempty"`
	Role     string `json:"role,omitempty"`
	Admin    bool   `json:"admin,omitempty"`
	Expires  int64  `json:"exp"`
}

func newDevTokenSigner(cfg Config) *devTokenSigner {
	if cfg.Production || strings.TrimSpace(cfg.DevAuthSigningKey) == "" {
		return nil
	}
	return &devTokenSigner{key: []byte(cfg.DevAuthSigningKey)}
}

func (s *devTokenSigner) sign(claims devTokenClaims) string {
	payload, _ := json.Marshal(claims)
	body := base64.RawURLEncoding.EncodeToString(payload)
	return body + "." + s.mac(body)
}

func (s *devTokenSigner) mac(body string) string {
	mac := hmac.New(sha256.New, s.key)
	mac.Write([]byte(body))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (s *devTokenSigner) verify(token string) (devTokenClaims, error) {
	parts := strings.Split(strings.TrimSpace(token), ".")
	if len(parts) != 2 || parts[0] == "" {
		return devTokenClaims{}, errors.New("malformed dev token")
	}
	if !hmac.Equal([]byte(s.mac(parts[0])), []byte(parts[1])) {
		return devTokenClaims{}, errors.New("invalid dev token signature")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return devTokenClaims{}, err
	}
	var claims devTokenClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return devTokenClaims{}, err
	}
	if claims.Subject == "" {
		return devTokenClaims{}, errors.New("dev token missing subject")
	}
	if claims.Expires == 0 || time.Now().Unix() > claims.Expires {
		return devTokenClaims{}, errors.New("dev token expired")
	}
	return claims, nil
}

// handleDevToken mints a signed local development token. It is registered only in
// non-production runs with DEV_AUTH_SIGNING_KEY set, and is the bootstrap a
// developer uses to obtain a token instead of forging identity headers.
func (a *App) handleDevToken(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username   string `json:"username"`
		Role       string `json:"role"`
		Admin      bool   `json:"admin"`
		TTLSeconds int    `json:"ttl_seconds"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	username := strings.TrimSpace(body.Username)
	if username == "" {
		username = "dev"
	}
	ttl := time.Duration(body.TTLSeconds) * time.Second
	if ttl <= 0 || ttl > 12*time.Hour {
		ttl = time.Hour
	}
	expires := time.Now().Add(ttl)
	token := a.devTokenSigner.sign(devTokenClaims{
		Subject:  username,
		Username: username,
		Role:     firstNonEmpty(body.Role, "user"),
		Admin:    body.Admin,
		Expires:  expires.Unix(),
	})
	WriteJSON(w, r, http.StatusOK, map[string]any{
		"token":      token,
		"token_type": "Bearer",
		"expires_at": expires.UTC().Format(time.RFC3339),
	})
}

// devClaimsUser maps verified dev-token claims onto the user identity shape the
// rest of the runtime consumes.
func devClaimsUser(c devTokenClaims) map[string]any {
	user := map[string]any{
		"id":       c.Subject,
		"username": firstNonEmpty(c.Username, c.Subject),
		"role":     firstNonEmpty(c.Role, "user"),
	}
	if c.Admin {
		user["system_role"] = 0
		user["admin_panel"] = true
	}
	return user
}
