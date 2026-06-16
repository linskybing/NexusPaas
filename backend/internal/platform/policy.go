package platform

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
)

type AllowAllPDP struct{}

func (AllowAllPDP) Enforce(_ context.Context, _, _, _, _ string) (contracts.Decision, error) {
	return contracts.Decision{Allowed: true, Reason: "local policy bundle allowed", Version: 1}, nil
}

type StaticPDP struct {
	Allowed bool
	Reason  string
}

func (p StaticPDP) Enforce(_ context.Context, _, _, _, _ string) (contracts.Decision, error) {
	return contracts.Decision{Allowed: p.Allowed, Reason: p.Reason, Version: 1}, nil
}

func (a *App) ValidatePolicyDecisionPoint() error {
	if !a.Config.RequireAuth {
		return nil
	}
	if a.PDP == nil {
		return errors.New("policy decision point is not configured")
	}
	switch a.PDP.(type) {
	case AllowAllPDP, *AllowAllPDP:
		return errors.New("policy decision point must not be AllowAllPDP when authentication is required")
	default:
		return nil
	}
}

func principalScopesAllow(r *http.Request, route RouteSpec) bool {
	user, ok := verifiedUser(r)
	if !ok {
		return true
	}
	scopes := scopeSet(userScopes(user))
	if len(scopes) == 0 || scopes[scopeWildcard] {
		return true
	}
	if scopes[scopeAdmin] && authUserIsAdmin(user) {
		return true
	}
	for _, required := range routeScopeCandidates(route) {
		if scopes[required] {
			return true
		}
	}
	return false
}

func userScopes(user map[string]any) []string {
	switch scopes := user["scopes"].(type) {
	case []string:
		return scopes
	case []any:
		out := make([]string, 0, len(scopes))
		for _, scope := range scopes {
			out = append(out, asString(scope))
		}
		return out
	case string:
		return strings.FieldsFunc(scopes, func(r rune) bool {
			return r == ',' || r == ' '
		})
	default:
		return nil
	}
}

func scopeSet(scopes []string) map[string]bool {
	set := map[string]bool{}
	for _, scope := range scopes {
		scope = strings.ToLower(strings.TrimSpace(scope))
		if scope != "" {
			set[scope] = true
		}
	}
	return set
}

func routeScopeCandidates(route RouteSpec) []string {
	action := routeScopeAction(route)
	service, resource := routeScopeParts(route.Resource)
	candidates := []string{action, scopeWildcard + ":" + action}
	if route.OperationID != "" {
		candidates = append(candidates, strings.ToLower(route.OperationID))
	}
	if route.Admin {
		candidates = append(candidates, scopeAdmin)
	}
	for _, part := range []string{service, resource} {
		if part == "" {
			continue
		}
		candidates = append(candidates, part+":"+action, part+":"+scopeWildcard)
		if route.OperationID != "" {
			candidates = append(candidates, part+":"+strings.ToLower(route.OperationID))
		}
	}
	return candidates
}

func routeScopeAction(route RouteSpec) string {
	switch route.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		if !route.StateChanging {
			return "read"
		}
	}
	return "write"
}

func routeScopeParts(resource string) (string, string) {
	parts := strings.SplitN(strings.ToLower(strings.TrimSpace(resource)), ":", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return "", parts[0]
}

const (
	scopeAdmin    = "admin"
	scopeWildcard = "*"
)
