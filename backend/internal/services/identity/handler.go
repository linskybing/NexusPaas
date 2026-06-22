package identity

import (
	"net/http"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

const (
	serviceName = "identity-service"

	sessionsResource      = serviceName + ":sessions"
	refreshTokensResource = serviceName + ":refresh_tokens"
	captchasResource      = serviceName + ":captchas"
	apiTokensResource     = serviceName + ":api_tokens"
	loginFailuresResource = serviceName + ":login_failures"

	defaultRoleID         = "RO2600004"
	defaultLoginMaxFailed = 5
	defaultLoginLockout   = 15 * time.Minute
	defaultAPITokenTTL    = 90 * 24 * time.Hour
	defaultAPITokenMax    = 20

	pathLogin    = "/api/v1/login"
	pathCLILogin = "/api/v1/cli/login"
	pathUserID   = "/api/v1/users/{id}"

	headerContentType = "Content-Type"
	headerUserID      = "X-User-ID"

	contentTypeJSON = "application/json"

	msgInvalidInput           = "invalid input"
	msgInvalidInputTitle      = "Invalid input"
	msgInvalidCredentials     = "Invalid credentials"
	msgAuthenticationRequired = "authentication required"
	msgAdminOnly              = "admin access required"
	msgUserNotFound           = "user not found"
	msgInvalidCaptcha         = "Invalid captcha"
	msgCaptchaRequired        = "Captcha required"
	reasonInvalidCaptcha      = "invalid_captcha"
	reasonInvalidCredentials  = "invalid_credentials"
	reasonLocked              = "locked"
	reasonCaptchaRequired     = "captcha_required"
)

var beforeLoginFailureCreate = func(*platform.App, *http.Request, string) {
	// Test hook: production login-failure creation has no pre-write side effect.
}

func Register(app *platform.App) {
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/captcha", getCaptcha)
	app.RegisterCustomHandler(http.MethodPost, pathLogin, login)
	app.RegisterCustomHandler(http.MethodPost, "/api/v1/logout", logout)
	app.RegisterCustomHandler(http.MethodPost, "/api/v1/register", register)
	app.RegisterCustomHandler(http.MethodPost, "/api/v1/refresh", refreshToken)
	app.RegisterCustomHandler(http.MethodPost, pathCLILogin, cliLogin)
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/me/api-tokens", listAPITokens)
	app.RegisterCustomHandler(http.MethodPost, "/api/v1/me/api-tokens", createAPIToken)
	app.RegisterCustomHandler(http.MethodDelete, "/api/v1/me/api-tokens/{id}", revokeAPIToken)
	app.RegisterCustomHandler(http.MethodDelete, "/api/v1/me/api-tokens/current", revokeCurrentAPIToken)
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/me/cli-ca", downloadCLICACert)
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/users", listUsers)
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/users/paging", listUsersPaging)
	app.RegisterCustomHandler(http.MethodPost, "/api/v1/users/resolve", resolveUsers)
	app.RegisterCustomHandler(http.MethodPost, "/api/v1/users/batch", batchCreateUsers)
	app.RegisterCustomHandler(http.MethodDelete, "/api/v1/users/batch", batchDeleteUsers)
	app.RegisterCustomHandler(http.MethodPut, "/api/v1/users/batch/password", batchResetPassword)
	app.RegisterCustomHandler(http.MethodPut, "/api/v1/users/batch/role", batchUpdateRole)
	app.RegisterCustomHandler(http.MethodGet, pathUserID, getUserByID)
	app.RegisterCustomHandler(http.MethodPut, pathUserID, updateUser)
	app.RegisterCustomHandler(http.MethodDelete, pathUserID, deleteUser)
	app.RegisterCustomHandler(http.MethodGet, pathUserID+"/settings", getUserSettings)
	app.RegisterCustomHandler(http.MethodPut, pathUserID+"/settings", updateUserSettings)
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/oidc/start", oidcStart)
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/oidc/login", oidcLoginForm)
	app.RegisterCustomHandler(http.MethodPost, "/api/v1/oidc/login", oidcLogin)
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/oidc/.well-known/openid-configuration", oidcProviderUnavailable)
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/oidc/jwks", oidcProviderUnavailable)
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/oidc/authorize", oidcAuthorize)
	app.RegisterCustomHandler(http.MethodPost, "/api/v1/oidc/token", oidcToken)
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/oidc/userinfo", oidcUserInfo)
	app.RegisterCustomHandler(http.MethodPost, "/api/v1/oidc/revoke", oidcRevoke)
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/oidc/callback", oidcCallback)
	app.RegisterCustomHandler(http.MethodPost, "/api/v1/oidc/callback", oidcCallback)
	registerInternalReadContracts(app)
	registerDexProxies(app)
	// Periodically prune expired sessions/refresh/API tokens (finding 1).
	registerAuthCleanup(app)
	registerLDAPMirror(app)
}
