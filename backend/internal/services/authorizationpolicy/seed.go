package authorizationpolicy

import (
	"net/http"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

func ensureDefaultServices(app *platform.App, r *http.Request) {
	_ = authorizationPolicyRepo(app).EnsureDefaultProxyServices(r.Context())
}

func ensureDefaultPolicies(app *platform.App, r *http.Request) {
	_ = authorizationPolicyRepo(app).EnsureDefaultProxyPolicies(r.Context())
}

func ensureDefaultPlatformRoles(app *platform.App, r *http.Request) {
	_ = authorizationPolicyRepo(app).EnsureDefaultProxyRoles(r.Context())
}

func ensureDefaultAssignments(app *platform.App, r *http.Request) {
	_ = authorizationPolicyRepo(app).EnsureDefaultProxyAssignments(r.Context())
}

func policyNameExists(app *platform.App, r *http.Request, excludeID, name string) bool {
	return authorizationPolicyRepo(app).PolicyNameExists(r.Context(), excludeID, name)
}
