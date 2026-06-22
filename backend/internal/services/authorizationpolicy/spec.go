package authorizationpolicy

import (
	"net/http"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

func Spec() platform.ServiceSpec {
	const (
		permissionPolicy       = "/api/v1/permissions/policy"
		proxyRBACPolicyID      = "/api/v1/admin/proxy-rbac/policies/{id}"
		proxyPolicyAssignments = proxyRBACPolicyID + "/assignments"
		proxyRBACRoleID        = "/api/v1/admin/proxy-rbac/roles/{id}"
	)
	route, id, admin, serviceInternal := shared.Route, shared.ID, shared.Admin, shared.ServiceInternal
	return platform.ServiceSpec{
		Name:            "authorization-policy-service",
		Category:        "core",
		Phase:           "4",
		RequiresCluster: true,
		Description:     "Central PDP, Casbin/domain RBAC, proxy RBAC, policy simulation, signed policy bundles, and policy sync.",
		Tables:          []string{"casbin_rule", "policies", "policy_rules", "policy_assignments", "platform_roles", "user_platform_roles", "service_definitions", "identity_users", "identity_roles", "outbox", "inbox"},
		Events:          []string{"PolicyChanged", "ProxyPolicyChanged"},
		Routes: []platform.RouteSpec{
			route(http.MethodGet, permissionPolicy, "policies", "list"),
			route(http.MethodPost, permissionPolicy, "policies", "create"),
			route(http.MethodPut, permissionPolicy, "policies", "update"),
			route(http.MethodDelete, permissionPolicy, "policies", "delete"),
			route(http.MethodPost, "/api/v1/permissions/batch", "policies", "batch"),
			route(http.MethodPost, "/api/v1/permissions/enforce", "permissions", "enforce", serviceInternal()),
			route(http.MethodPost, "/api/v1/permissions/simulate", "decisions", "simulate", admin()),
			route(http.MethodGet, "/api/v1/permissions/policies", "policies", "list", admin()),
			route(http.MethodPost, "/api/v1/permissions/policies", "policies", "create", admin()),
			route(http.MethodPatch, "/api/v1/permissions/policies/{id}", "policies", "update", id("id"), admin()),
			route(http.MethodDelete, "/api/v1/permissions/policies/{id}", "policies", "delete", id("id"), admin()),
			route(http.MethodPost, "/api/v1/permissions/policies/batch", "policies", "batch", admin()),
			route(http.MethodGet, "/api/v1/admin/proxy-rbac/services", "proxy_services", "list", admin()),
			route(http.MethodPost, "/api/v1/admin/proxy-rbac/services", "proxy_services", "create", admin()),
			route(http.MethodGet, "/api/v1/admin/proxy-rbac/services/{id}", "proxy_services", "get", id("id"), admin()),
			route(http.MethodGet, "/api/v1/admin/proxy-rbac/policies", "proxy_policies", "list", admin()),
			route(http.MethodPost, "/api/v1/admin/proxy-rbac/policies", "proxy_policies", "create", admin()),
			route(http.MethodGet, proxyRBACPolicyID, "proxy_policies", "get", id("id"), admin()),
			route(http.MethodPut, proxyRBACPolicyID, "proxy_policies", "update", id("id"), admin()),
			route(http.MethodPatch, proxyRBACPolicyID, "proxy_policies", "update", id("id"), admin()),
			route(http.MethodDelete, proxyRBACPolicyID, "proxy_policies", "delete", id("id"), admin()),
			route(http.MethodGet, proxyPolicyAssignments, "proxy_policy_assignments", "list", id("id"), admin()),
			route(http.MethodPost, proxyPolicyAssignments, "proxy_policy_assignments", "create", id("id"), admin()),
			route(http.MethodPost, proxyPolicyAssignments+"/batch", "proxy_policy_assignments", "batch", id("id"), admin()),
			route(http.MethodDelete, proxyPolicyAssignments, "proxy_policy_assignments", "delete", id("id"), admin()),
			route(http.MethodGet, "/api/v1/admin/proxy-rbac/targets/{type}/{id}/assignments", "proxy_target_assignments", "list", id("id"), admin()),
			route(http.MethodGet, "/api/v1/admin/proxy-rbac/roles", "proxy_roles", "list", admin()),
			route(http.MethodPost, "/api/v1/admin/proxy-rbac/roles", "proxy_roles", "create", admin()),
			route(http.MethodGet, proxyRBACRoleID, "proxy_roles", "get", id("id"), admin()),
			route(http.MethodPut, proxyRBACRoleID, "proxy_roles", "update", id("id"), admin()),
			route(http.MethodDelete, proxyRBACRoleID, "proxy_roles", "delete", id("id"), admin()),
			route(http.MethodGet, proxyRBACRoleID+"/users", "proxy_role_users", "list", id("id"), admin()),
			route(http.MethodPost, proxyRBACRoleID+"/users", "proxy_role_users", "create", id("id"), admin()),
			route(http.MethodPost, proxyRBACRoleID+"/users/batch", "proxy_role_users", "batch", id("id"), admin()),
			route(http.MethodDelete, proxyRBACRoleID+"/users/{user_id}", "proxy_role_users", "delete", id("user_id"), admin()),
			route(http.MethodGet, "/api/v1/admin/proxy-rbac/system-roles", "proxy_system_roles", "list", admin()),
		},
	}
}
