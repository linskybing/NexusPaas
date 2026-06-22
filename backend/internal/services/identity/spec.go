package identity

import (
	"net/http"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

func Spec() platform.ServiceSpec {
	const (
		userPath         = "/api/v1/users/{id}"
		userSettingsPath = userPath + "/settings"
	)
	route, public, id, admin := shared.Route, shared.Public, shared.ID, shared.Admin
	return platform.ServiceSpec{
		Name:        "identity-service",
		Category:    "core",
		Phase:       "4",
		Description: "Authentication, sessions, user API tokens, CLI login, LDAP/local strategies, users, roles, and OIDC provider.",
		Tables:      []string{"users", "sessions", "refresh_tokens", "user_api_tokens", "roles", "credential_audit_snapshots", "outbox", "inbox"},
		Events:      []string{"UserCreated", "UserUpdated", "UserDisabled"},
		Routes: []platform.RouteSpec{
			public(route(http.MethodPost, "/api/v1/login", "sessions", "create")),
			public(route(http.MethodPost, "/api/v1/logout", "sessions", "delete")),
			public(route(http.MethodPost, "/api/v1/register", "users", "create")),
			public(route(http.MethodPost, "/api/v1/refresh", "refresh_tokens", "create")),
			public(route(http.MethodGet, "/api/v1/captcha", "captchas", "list")),
			route(http.MethodGet, "/api/v1/me/api-tokens", "api_tokens", "list"),
			route(http.MethodPost, "/api/v1/me/api-tokens", "api_tokens", "create"),
			route(http.MethodDelete, "/api/v1/me/api-tokens/{id}", "api_tokens", "delete"),
			route(http.MethodDelete, "/api/v1/me/api-tokens/current", "api_tokens", "delete_current"),
			public(route(http.MethodPost, "/api/v1/cli/login", "cli_sessions", "create")),
			route(http.MethodGet, "/api/v1/me/cli-ca", "cli_ca", "list"),
			route(http.MethodGet, "/api/v1/users", "users", "list", admin()),
			route(http.MethodPost, "/api/v1/users", "users", "create", admin()),
			route(http.MethodGet, "/api/v1/users/paging", "users", "list_paging", admin()),
			route(http.MethodPost, "/api/v1/users/resolve", "users", "resolve", admin()),
			route(http.MethodGet, userPath, "users", "get", id("id")),
			route(http.MethodPut, userPath, "users", "update", id("id"), admin()),
			route(http.MethodPatch, userPath, "users", "update", id("id"), admin()),
			route(http.MethodDelete, userPath, "users", "delete", id("id"), admin()),
			route(http.MethodPost, "/api/v1/users/batch", "users", "batch_create", admin()),
			route(http.MethodDelete, "/api/v1/users/batch", "users", "batch_delete", admin()),
			route(http.MethodPut, "/api/v1/users/batch/password", "users", "batch_password_reset", admin()),
			route(http.MethodPost, "/api/v1/users/batch/password-reset", "users", "batch_password_reset", admin()),
			route(http.MethodPut, "/api/v1/users/batch/role", "users", "batch_role_update", admin()),
			route(http.MethodPatch, "/api/v1/users/batch/roles", "users", "batch_role_update", admin()),
			route(http.MethodGet, userSettingsPath, "user_settings", "get", id("id")),
			route(http.MethodPut, userSettingsPath, "user_settings", "update", id("id")),
			route(http.MethodPatch, userSettingsPath, "user_settings", "update", id("id")),
			public(route(http.MethodGet, "/api/v1/oidc/start", "oidc_start", "create")),
			public(route(http.MethodGet, "/api/v1/oidc/login", "oidc_login", "list")),
			public(route(http.MethodPost, "/api/v1/oidc/login", "oidc_login", "create")),
			public(route(http.MethodGet, "/api/v1/oidc/.well-known/openid-configuration", "oidc", "discovery")),
			public(route(http.MethodGet, "/api/v1/oidc/jwks", "jwks", "list")),
			public(route(http.MethodGet, "/api/v1/oidc/authorize", "oidc_authorizations", "create")),
			public(route(http.MethodPost, "/api/v1/oidc/token", "oidc_tokens", "create")),
			route(http.MethodGet, "/api/v1/oidc/userinfo", "oidc_userinfo", "list"),
			route(http.MethodPost, "/api/v1/oidc/revoke", "oidc_tokens", "delete"),
			public(route(http.MethodGet, "/api/v1/oidc/callback", "oidc_callbacks", "create")),
			public(route(http.MethodPost, "/api/v1/oidc/callback", "oidc_callbacks", "create")),
			public(route(http.MethodGet, "/dex/{path...}", "dex_browser", "browser_proxy")),
			public(route(http.MethodPost, "/dex/{path...}", "dex_browser", "browser_proxy")),
		},
	}
}
