package platform

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
	"strings"
)

// APIKeyPrincipal binds a static API key to an explicit service/user identity.
// Production config requires one binding per enabled API_KEYS entry.
type APIKeyPrincipal struct {
	ID       string   `json:"id"`
	UserID   string   `json:"user_id,omitempty"`
	Username string   `json:"username"`
	Name     string   `json:"name,omitempty"`
	Role     string   `json:"role"`
	Admin    bool     `json:"admin"`
	Scopes   []string `json:"scopes,omitempty"`
}

func (p APIKeyPrincipal) normalized() APIKeyPrincipal {
	p.ID = firstNonEmpty(strings.TrimSpace(p.ID), strings.TrimSpace(p.UserID))
	p.Username = firstNonEmpty(strings.TrimSpace(p.Username), strings.TrimSpace(p.Name), p.ID)
	p.Role = firstNonEmpty(strings.TrimSpace(p.Role), "service")
	p.Scopes = normalizedScopes(p.Scopes)
	if p.Admin || authRoleGrantsStaticAdmin(p.Role) {
		p.Admin = true
		p.Role = "admin"
	}
	return p
}

func (p APIKeyPrincipal) userData() map[string]any {
	p = p.normalized()
	data := map[string]any{
		"id":          p.ID,
		"username":    p.Username,
		"role":        p.Role,
		"system_role": 2,
		"status":      "online",
		"scopes":      p.Scopes,
	}
	if p.Admin {
		data["system_role"] = 0
		data["admin_panel"] = true
	}
	return data
}

func (a *App) authorizeStaticAPIKey(r *http.Request, presented string) bool {
	matched, ok := matchedAPIKey(presented, a.Config.APIKeys)
	if !ok {
		return false
	}
	if principal := a.Config.APIKeyPrincipals[matched].normalized(); principal.ID != "" {
		applyAuthHeaders(r, principal.userData())
	}
	return true
}

func apiKeyAllowed(presented string, configured map[string]bool) bool {
	_, ok := matchedAPIKey(presented, configured)
	return ok
}

func matchedAPIKey(presented string, configured map[string]bool) (string, bool) {
	if presented == "" || len(configured) == 0 {
		return "", false
	}
	presentedHash := sha256.Sum256([]byte(presented))
	matched := ""
	allowed := 0
	for key, enabled := range configured {
		keyHash := sha256.Sum256([]byte(key))
		equal := subtle.ConstantTimeCompare(presentedHash[:], keyHash[:]) & enabledMask(enabled)
		if equal == 1 {
			matched = key
		}
		allowed |= equal
	}
	return matched, allowed == 1
}

func hasEnabledAPIKey(configured map[string]bool) bool {
	for _, enabled := range configured {
		if enabled {
			return true
		}
	}
	return false
}

func normalizedScopes(scopes []string) []string {
	out := make([]string, 0, len(scopes))
	seen := map[string]bool{}
	for _, scope := range scopes {
		scope = strings.TrimSpace(scope)
		if scope == "" || seen[scope] {
			continue
		}
		seen[scope] = true
		out = append(out, scope)
	}
	return out
}

func authRoleGrantsStaticAdmin(role string) bool {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "admin", "superadmin", "root":
		return true
	default:
		return false
	}
}

func enabledMask(enabled bool) int {
	if enabled {
		return 1
	}
	return 0
}
