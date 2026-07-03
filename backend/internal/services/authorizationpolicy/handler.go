package authorizationpolicy

import (
	"context"
	"net/http"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

const (
	serviceName                = "authorization-policy-service"
	authorizationRolesResource = serviceName + ":roles"
	servicesResource           = serviceName + ":proxy_services"
	policiesResource           = serviceName + ":proxy_policies"
	rulesResource              = serviceName + ":proxy_policy_rules"
	assignmentsResource        = serviceName + ":proxy_policy_assignments"
	platformRolesResource      = serviceName + ":proxy_roles"
	roleUsersResource          = serviceName + ":proxy_role_users"
	seedMarkersResource        = serviceName + ":seed_markers"
	identityProjectionConsumer = serviceName + ":identity_projection"

	pathRawPermissionPolicy = "/api/v1/permissions/policy"
	pathProxyRBACPolicyID   = "/api/v1/admin/proxy-rbac/policies/{id}"
	pathProxyPolicyAssign   = pathProxyRBACPolicyID + "/assignments"
	pathProxyRBACRoleID     = "/api/v1/admin/proxy-rbac/roles/{id}"

	headerUserID = "X-User-ID"

	errReadBodyFmt           = "read body: %w"
	msgInvalidPolicyFormat   = "invalid policy format"
	msgInvalidEnforceRequest = "invalid enforce request: requires sub, obj, act"
	msgInvalidBatchRequest   = "invalid batch request"
	msgPolicyAlreadyExists   = "policy already exists"
	msgPolicyRuleExists      = "policy rule already exists"
	msgPolicyNotFound        = "policy not found"
	msgRoleNotFound          = "role not found"
	msgUserIDRequired        = "user_id is required"

	seedProxyServices    = "proxy-services"
	seedProxyPolicies    = "proxy-policies"
	seedProxyRoles       = "proxy-roles"
	seedProxyAssignments = "proxy-assignments"
)

func Register(app *platform.App) {
	if app.Config.RequireAuth && app.Config.AllowsService(serviceName) {
		app.PDP = RawPolicyPDP{Policies: rawPermissionRepo(app)}
	}
	reconcileAdminBootstrapPolicies(app)
	registerAuthorizationPolicyProjectionReconciler(app)
	app.RegisterCustomHandler(http.MethodGet, pathRawPermissionPolicy, listRawPermissionPolicies)
	app.RegisterCustomHandler(http.MethodPost, pathRawPermissionPolicy, addRawPermissionPolicy)
	app.RegisterCustomHandler(http.MethodPut, pathRawPermissionPolicy, updateRawPermissionPolicy)
	app.RegisterCustomHandler(http.MethodDelete, pathRawPermissionPolicy, removeRawPermissionPolicy)
	app.RegisterCustomHandler(http.MethodPost, "/api/v1/permissions/batch", batchProcessPermissions)
	app.RegisterCustomHandler(http.MethodPost, "/api/v1/permissions/enforce", enforcePermission)
	app.RegisterCustomHandler(http.MethodPost, "/api/v1/permissions/simulate", simulatePermissionEnforce)
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/admin/proxy-rbac/services", listServices)
	app.RegisterCustomHandler(http.MethodPost, "/api/v1/admin/proxy-rbac/services", createService)
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/admin/proxy-rbac/services/{id}", getService)
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/admin/proxy-rbac/policies", listPolicies)
	app.RegisterCustomHandler(http.MethodPost, "/api/v1/admin/proxy-rbac/policies", createPolicy)
	app.RegisterCustomHandler(http.MethodGet, pathProxyRBACPolicyID, getPolicy)
	app.RegisterCustomHandler(http.MethodPut, pathProxyRBACPolicyID, updatePolicy)
	app.RegisterCustomHandler(http.MethodPatch, pathProxyRBACPolicyID, updatePolicy)
	app.RegisterCustomHandler(http.MethodDelete, pathProxyRBACPolicyID, deletePolicy)
	app.RegisterCustomHandler(http.MethodGet, pathProxyPolicyAssign, listPolicyAssignments)
	app.RegisterCustomHandler(http.MethodPost, pathProxyPolicyAssign, assignPolicy)
	app.RegisterCustomHandler(http.MethodPost, pathProxyPolicyAssign+"/batch", batchAssignPolicy)
	app.RegisterCustomHandler(http.MethodDelete, pathProxyPolicyAssign, unassignPolicy)
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/admin/proxy-rbac/targets/{type}/{id}/assignments", listTargetAssignments)
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/admin/proxy-rbac/roles", listRoles)
	app.RegisterCustomHandler(http.MethodPost, "/api/v1/admin/proxy-rbac/roles", createRole)
	app.RegisterCustomHandler(http.MethodGet, pathProxyRBACRoleID, getRole)
	app.RegisterCustomHandler(http.MethodPut, pathProxyRBACRoleID, updateRole)
	app.RegisterCustomHandler(http.MethodDelete, pathProxyRBACRoleID, deleteRole)
	app.RegisterCustomHandler(http.MethodGet, pathProxyRBACRoleID+"/users", listRoleUsers)
	app.RegisterCustomHandler(http.MethodPost, pathProxyRBACRoleID+"/users", assignRoleUser)
	app.RegisterCustomHandler(http.MethodPost, pathProxyRBACRoleID+"/users/batch", batchAssignRoleUsers)
	app.RegisterCustomHandler(http.MethodDelete, pathProxyRBACRoleID+"/users/{user_id}", unassignRoleUser)
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/admin/proxy-rbac/system-roles", listSystemRoles)
	registerPolicyDataSync(app)
}

type RawPolicyPDP struct {
	Policies rawPermissionChecker
}

func (p RawPolicyPDP) Enforce(ctx context.Context, subject, domain, object, action string) (contracts.Decision, error) {
	if p.Policies == nil {
		return contracts.Decision{Allowed: false, Reason: "authorization policy store is not configured", Version: 1}, nil
	}
	allowed, err := p.Policies.RawPermissionAllowed(ctx, subject, domain, object, action)
	if err != nil {
		return contracts.Decision{}, err
	}
	if allowed {
		return contracts.Decision{Allowed: true, Reason: "authorization-policy-service raw policy matched", Version: 1}, nil
	}
	return contracts.Decision{Allowed: false, Reason: "authorization-policy-service raw policy denied", Version: 1}, nil
}
