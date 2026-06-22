package authorizationpolicy

import (
	"context"
	"net/http"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

const (
	adminBootstrapManagedKey = "bootstrap_managed"
	adminBootstrapSourceKey  = "bootstrap_source"
	adminBootstrapSource     = "static_admin_api_key"
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

func reconcileAdminBootstrapPolicies(app *platform.App) {
	if app == nil || app.Store == nil || !app.Config.AllowsService(serviceName) {
		return
	}
	ctx := context.Background()
	repo := rawPermissionRepo(app)
	desired := adminBootstrapPolicies(app)
	for _, policy := range desired {
		if repo.RawPermissionPolicyExists(ctx, policy) {
			continue
		}
		_, _ = repo.CreateRawPermissionPolicyRecord(ctx, policy, adminBootstrapMetadata())
	}
	for _, record := range repo.ListRawPermissionPolicyRecords(ctx) {
		if !adminBootstrapManaged(record) {
			continue
		}
		id, _ := record["id"].(string)
		if _, ok := desired[id]; !ok {
			repo.DeleteRawPermissionPolicyRecord(ctx, id)
		}
	}
}

func adminBootstrapPolicies(app *platform.App) map[string][]string {
	subjects := map[string]bool{}
	for _, principal := range app.Config.APIKeyPrincipals {
		normalized := principal.Normalized()
		if normalized.ID != "" && normalized.Admin {
			subjects[normalized.ID] = true
		}
	}
	desired := map[string][]string{}
	for _, route := range app.CatalogRoutes {
		if !adminBootstrapRoute(route) {
			continue
		}
		for subject := range subjects {
			policy := []string{subject, "", route.Resource, route.OperationID}
			desired[rawPolicyID(policy)] = policy
		}
	}
	return desired
}

func adminBootstrapRoute(route platform.RouteSpec) bool {
	return route.AuthRequired &&
		!route.PolicyBypass &&
		!route.ServiceAuthRequired &&
		!platform.IsInternalRoutePattern(route.Pattern) &&
		route.Resource != "" &&
		route.OperationID != ""
}

func adminBootstrapMetadata() map[string]any {
	return map[string]any{
		adminBootstrapManagedKey: true,
		adminBootstrapSourceKey:  adminBootstrapSource,
	}
}

func adminBootstrapManaged(data map[string]any) bool {
	managed, _ := data[adminBootstrapManagedKey].(bool)
	source, _ := data[adminBootstrapSourceKey].(string)
	return managed && source == adminBootstrapSource
}

func policyNameExists(app *platform.App, r *http.Request, excludeID, name string) bool {
	return authorizationPolicyRepo(app).PolicyNameExists(r.Context(), excludeID, name)
}
