package services

import (
	"net/http"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/auditcompliance"
	"github.com/linskybing/nexuspaas/backend/internal/services/authorizationpolicy"
	"github.com/linskybing/nexuspaas/backend/internal/services/clusterread"
	"github.com/linskybing/nexuspaas/backend/internal/services/dashboard"
	"github.com/linskybing/nexuspaas/backend/internal/services/gpuusage"
	"github.com/linskybing/nexuspaas/backend/internal/services/identity"
	"github.com/linskybing/nexuspaas/backend/internal/services/ideworkspace"
	"github.com/linskybing/nexuspaas/backend/internal/services/imageregistry"
	"github.com/linskybing/nexuspaas/backend/internal/services/integrationproxy"
	"github.com/linskybing/nexuspaas/backend/internal/services/k8scontrol"
	"github.com/linskybing/nexuspaas/backend/internal/services/mediaupload"
	"github.com/linskybing/nexuspaas/backend/internal/services/orgproject"
	"github.com/linskybing/nexuspaas/backend/internal/services/requestnotification"
	"github.com/linskybing/nexuspaas/backend/internal/services/resourcehours"
	"github.com/linskybing/nexuspaas/backend/internal/services/schedulerquota"
	storageservice "github.com/linskybing/nexuspaas/backend/internal/services/storage"
	"github.com/linskybing/nexuspaas/backend/internal/services/workload"
)

const (
	serviceIdentity            = "identity-service"
	serviceAuthorizationPolicy = "authorization-policy-service"
	serviceOrgProject          = "org-project-service"
	serviceWorkload            = "workload-service"
	serviceSchedulerQuota      = "scheduler-quota-service"
	serviceK8sControl          = "k8s-control-service"
	serviceIDE                 = "ide-service"
	serviceStorage             = "storage-service"
	serviceImageRegistry       = "image-registry-service"
	serviceUsageObservability  = "usage-observability-service"
	serviceRequestNotification = "request-notification-service"
	serviceIntegrationProxy    = "integration-proxy-service"
	serviceMediaUpload         = "media-upload-service"
	serviceAuditCompliance     = "audit-compliance-service"

	catalogPathPermissionPolicy       = "/api/v1/permissions/policy"
	catalogPathProxyRBACPolicyID      = "/api/v1/admin/proxy-rbac/policies/{id}"
	catalogPathProxyPolicyAssignments = catalogPathProxyRBACPolicyID + "/assignments"
	catalogPathProxyRBACRoleID        = "/api/v1/admin/proxy-rbac/roles/{id}"
	catalogPathGroupID                = "/api/v1/groups/{id}"
	catalogPathProjectID              = "/api/v1/projects/{id}"
	catalogPathProjectMembers         = catalogPathProjectID + "/members"
	catalogPathProjectMemberQuota     = catalogPathProjectMembers + "/{userId}/quota"
	catalogPathUserGroups             = "/api/v1/user-groups"
	catalogPathQueueID                = "/api/v1/queues/{id}"
	catalogPathPlanID                 = "/api/v1/plans/{id}"
	catalogPathUserID                 = "/api/v1/users/{id}"
	catalogPathUserSettings           = catalogPathUserID + "/settings"
	catalogPathConfigFileID           = "/api/v1/configfiles/{id}"
	catalogPathCompatProxy            = "/api/v1/{path...}"
)

func RegisterAll(app *platform.App) {
	for _, spec := range Catalog() {
		app.RegisterService(spec)
	}
	registerStoreDependencies(app)
	for _, registration := range serviceRegistrations() {
		if app.Config.AllowsService(registration.owner) {
			registration.register(app)
		}
	}
}

type serviceRegistration struct {
	owner    string
	register func(*platform.App)
}

type storeDependency struct {
	service  string
	resource string
	access   []string
}

const (
	storeAccessGet    = "Get"
	storeAccessList   = "List"
	storeAccessUpdate = "Update"
)

func registerStoreDependencies(app *platform.App) {
	for _, dependency := range serviceStoreDependencies() {
		app.RegisterStoreDependencies(dependency.service, dependency.resource)
	}
}

func serviceStoreDependencies() []storeDependency {
	return []storeDependency{
		{
			service:  serviceSchedulerQuota,
			resource: serviceOrgProject + ":project_members",
			access:   []string{storeAccessGet, storeAccessList},
		},
		{
			// Read-only: project plan-binding writes now go through the
			// org-project internal binding contract (problem.md #2), so
			// scheduler-quota no longer needs Update on the project aggregate.
			// The remaining Get/List back the out-of-scope plan/quota reads.
			service:  serviceSchedulerQuota,
			resource: serviceOrgProject + ":projects",
			access:   []string{storeAccessGet, storeAccessList},
		},
		{
			service:  serviceSchedulerQuota,
			resource: serviceOrgProject + ":user_groups",
			access:   []string{storeAccessList},
		},
		{
			service:  serviceSchedulerQuota,
			resource: serviceOrgProject + ":user_quotas",
			access:   []string{storeAccessGet, storeAccessList},
		},
		{
			service:  serviceSchedulerQuota,
			resource: serviceWorkload + ":jobs",
			access:   []string{storeAccessList},
		},
	}
}

func serviceRegistrations() []serviceRegistration {
	return []serviceRegistration{
		{owner: serviceRequestNotification, register: requestnotification.Register},
		{owner: serviceMediaUpload, register: mediaupload.Register},
		{owner: serviceUsageObservability, register: resourcehours.Register},
		{owner: serviceSchedulerQuota, register: schedulerquota.Register},
		{owner: serviceWorkload, register: workload.Register},
		{owner: serviceK8sControl, register: k8scontrol.Register},
		{owner: serviceStorage, register: storageservice.Register},
		{owner: serviceUsageObservability, register: gpuusage.Register},
		{owner: serviceUsageObservability, register: clusterread.Register},
		{owner: serviceIDE, register: ideworkspace.Register},
		{owner: serviceImageRegistry, register: imageregistry.Register},
		{owner: serviceAuditCompliance, register: auditcompliance.Register},
		{owner: serviceUsageObservability, register: dashboard.Register},
		{owner: serviceIntegrationProxy, register: integrationproxy.Register},
		{owner: serviceOrgProject, register: orgproject.Register},
		{owner: serviceAuthorizationPolicy, register: authorizationpolicy.Register},
		{owner: serviceIdentity, register: identity.Register},
	}
}

func Catalog() []platform.ServiceSpec {
	specs := []platform.ServiceSpec{
		platformGateway(),
		identityService(),
		authorizationPolicyService(),
		orgProjectService(),
		workloadService(),
		schedulerQuotaService(),
		k8sControlService(),
		ideService(),
		storageService(),
		imageRegistryService(),
		usageObservabilityService(),
		auditComplianceService(),
		requestNotificationService(),
		integrationProxyService(),
		mediaUploadService(),
	}
	for i := range specs {
		specs[i].Routes = append(specs[i].Routes, referenceCompatRoutes(specs[i].Name)...)
	}
	return specs
}

func referenceCompatRoutes(service string) []platform.RouteSpec {
	switch service {
	case serviceAuthorizationPolicy:
		return []platform.RouteSpec{
			route(http.MethodGet, catalogPathPermissionPolicy, "policies", "list"),
			route(http.MethodPost, catalogPathPermissionPolicy, "policies", "create"),
			route(http.MethodPut, catalogPathPermissionPolicy, "policies", "update"),
			route(http.MethodDelete, catalogPathPermissionPolicy, "policies", "delete"),
			route(http.MethodPost, "/api/v1/permissions/batch", "policies", "batch"),
			route(http.MethodGet, "/api/v1/admin/proxy-rbac/services/{id}", "proxy_services", "get", id("id"), admin()),
			route(http.MethodGet, catalogPathProxyRBACPolicyID, "proxy_policies", "get", id("id"), admin()),
			route(http.MethodPut, catalogPathProxyRBACPolicyID, "proxy_policies", "update", id("id"), admin()),
			route(http.MethodGet, catalogPathProxyPolicyAssignments, "proxy_policy_assignments", "list", id("id"), admin()),
			route(http.MethodPost, catalogPathProxyPolicyAssignments, "proxy_policy_assignments", "create", id("id"), admin()),
			route(http.MethodPost, catalogPathProxyPolicyAssignments+"/batch", "proxy_policy_assignments", "batch", id("id"), admin()),
			route(http.MethodDelete, catalogPathProxyPolicyAssignments, "proxy_policy_assignments", "delete", id("id"), admin()),
			route(http.MethodGet, "/api/v1/admin/proxy-rbac/targets/{type}/{id}/assignments", "proxy_target_assignments", "list", id("id"), admin()),
			route(http.MethodGet, "/api/v1/admin/proxy-rbac/roles", "proxy_roles", "list", admin()),
			route(http.MethodPost, "/api/v1/admin/proxy-rbac/roles", "proxy_roles", "create", admin()),
			route(http.MethodGet, catalogPathProxyRBACRoleID, "proxy_roles", "get", id("id"), admin()),
			route(http.MethodPut, catalogPathProxyRBACRoleID, "proxy_roles", "update", id("id"), admin()),
			route(http.MethodDelete, catalogPathProxyRBACRoleID, "proxy_roles", "delete", id("id"), admin()),
			route(http.MethodGet, catalogPathProxyRBACRoleID+"/users", "proxy_role_users", "list", id("id"), admin()),
			route(http.MethodPost, catalogPathProxyRBACRoleID+"/users", "proxy_role_users", "create", id("id"), admin()),
			route(http.MethodPost, catalogPathProxyRBACRoleID+"/users/batch", "proxy_role_users", "batch", id("id"), admin()),
			route(http.MethodDelete, catalogPathProxyRBACRoleID+"/users/{user_id}", "proxy_role_users", "delete", id("user_id"), admin()),
			route(http.MethodGet, "/api/v1/admin/proxy-rbac/system-roles", "proxy_system_roles", "list", admin()),
		}
	case serviceOrgProject:
		return []platform.RouteSpec{
			route(http.MethodGet, catalogPathGroupID, "groups", "get", id("id")),
			route(http.MethodPut, catalogPathGroupID, "groups", "update", id("id"), admin()),
			route(http.MethodGet, "/api/v1/admin/group-policy-options", "group_policy_options", "list", admin()),
			route(http.MethodGet, "/api/v1/projects/by-user", "projects", "list_by_user"),
			route(http.MethodPut, catalogPathProjectID, "projects", "update", id("id"), admin()),
			route(http.MethodGet, catalogPathProjectID+"/add-members-context", "project_members", "add_members_context", id("id")),
			route(http.MethodDelete, catalogPathProjectID+"/gpu-claims/{name}", "gpu_claims", "delete", id("name")),
			route(http.MethodDelete, catalogPathProjectMembers, "project_members", "delete"),
			route(http.MethodPut, catalogPathProjectMembers+"/roles", "project_members", "batch_role_update", id("id")),
			route(http.MethodPut, catalogPathProjectMembers+"/quotas", "member_quotas", "batch_update", id("id")),
			route(http.MethodPut, catalogPathProjectMembers+"/{userId}", "project_members", "update_role", id("userId")),
			route(http.MethodDelete, catalogPathUserGroups, "user_groups", "delete"),
			route(http.MethodPut, catalogPathUserGroups, "user_groups", "update"),
			route(http.MethodGet, catalogPathUserGroups+"/by-group", "user_groups", "list_by_group"),
			route(http.MethodGet, catalogPathUserGroups+"/by-user", "user_groups", "list_by_user"),
			route(http.MethodGet, catalogPathUserGroups+"/{group_id}/add-members-context", "user_groups", "add_members_context", id("group_id")),
			route(http.MethodPost, catalogPathUserGroups+"/{group_id}/resolve-add-members", "user_groups", "resolve_add_members", id("group_id")),
			route(http.MethodGet, catalogPathUserGroups+"/{group_id}/members", "user_groups", "list_members", id("group_id")),
		}
	case serviceStorage:
		routes := []platform.RouteSpec{
			route(http.MethodGet, "/api/v1/storage/options", "storage_options", "list"),
			route(http.MethodGet, "/api/v1/storage/group/{id}", "group_storage", "list", id("id")),
			route(http.MethodGet, "/api/v1/storage/my-storages", "group_storage", "list_my"),
			route(http.MethodPost, "/api/v1/storage/{id}/storage", "group_storage", "create", id("id"), admin(), adapter("minio")),
			route(http.MethodDelete, "/api/v1/storage/{id}/storage/{pvcId}", "group_storage", "delete", id("pvcId"), admin(), adapter("minio")),
			route(http.MethodPost, "/api/v1/storage/{id}/storage/{pvcId}/start", "filebrowser", "command", id("pvcId"), adapter("minio")),
			route(http.MethodDelete, "/api/v1/storage/{id}/storage/{pvcId}/stop", "filebrowser", "command", id("pvcId"), adapter("minio")),
			route(http.MethodDelete, "/api/v1/storage/permissions/batch", "storage_permissions", "batch_delete"),
			route(http.MethodGet, "/api/v1/storage/permissions/group/{group_id}/pvc/{pvc_id}", "storage_permissions", "get", id("pvc_id")),
			route(http.MethodGet, "/api/v1/storage/permissions/group/{group_id}/pvc/{pvc_id}/policy", "storage_access_policies", "get", id("pvc_id")),
			route(http.MethodGet, "/api/v1/storage/permissions/group/{group_id}/pvc/{pvc_id}/list", "storage_permissions", "list", id("pvc_id")),
			route(http.MethodDelete, "/api/v1/storage/permissions/group/{group_id}/pvc/{pvc_id}/user/{user_id}", "storage_permissions", "delete", id("user_id")),
			route(http.MethodPost, "/api/v1/storage/policies", "storage_access_policies", "create"),
			route(http.MethodDelete, "/api/v1/projects/{id}/storage/bindings/{pvcId}", "storage_bindings", "delete", id("pvcId")),
			route(http.MethodGet, "/api/v1/projects/{id}/storage/bindings/{pvcId}/permissions", "project_storage_permissions", "list", id("pvcId")),
			route(http.MethodPut, "/api/v1/projects/{id}/storage/bindings/{pvcId}/permissions", "project_storage_permissions", "update", id("pvcId")),
			route(http.MethodDelete, "/api/v1/projects/{id}/storage/bindings/{pvcId}/permissions/{userId}", "project_storage_permissions", "delete", id("userId")),
			route(http.MethodPut, "/api/v1/projects/{id}/storage/bindings/{pvcId}/permissions/batch", "project_storage_permissions", "batch_update", id("pvcId")),
			route(http.MethodDelete, "/api/v1/projects/{id}/storage/bindings/{pvcId}/permissions/batch", "project_storage_permissions", "batch_delete", id("pvcId")),
			route(http.MethodPost, "/api/v1/projects/{id}/storage/transfers/fast-stage", "fast_transfers", "command", id("id"), adapter("minio")),
			route(http.MethodGet, "/api/v1/projects/{id}/storage/transfers/{targetNamespace}/{name}", "fast_transfers", "get", id("name")),
			route(http.MethodDelete, "/api/v1/projects/{id}/storage/transfers/{targetNamespace}/{name}", "fast_transfers", "command", id("name"), adapter("minio")),
			route(http.MethodPost, "/api/v1/admin/user-storage/batch-init", "user_storage", "command", admin(), adapter("minio")),
			route(http.MethodPost, "/api/v1/admin/user-storage/batch-status", "user_storage", "command", admin(), adapter("minio")),
			route(http.MethodGet, "/api/v1/admin/user-storage/{username}/status", "user_storage", "get", id("username"), admin(), adapter("minio")),
			route(http.MethodPost, "/api/v1/admin/user-storage/{username}/init", "user_storage", "command", id("username"), admin(), adapter("minio")),
			route(http.MethodPut, "/api/v1/admin/user-storage/{username}/expand", "user_storage", "command", id("username"), admin(), adapter("minio")),
			route(http.MethodDelete, "/api/v1/admin/user-storage/{username}", "user_storage", "command", id("username"), admin(), adapter("minio")),
		}
		routes = append(routes, anyCompatRoutes("/api/v1/storage/{id}/storage/{pvcId}/proxy/{path...}", "filebrowser_proxy", "proxy", id("pvcId"), adapter("minio"))...)
		return routes
	case serviceImageRegistry:
		return []platform.RouteSpec{
			route(http.MethodGet, "/api/v1/harbor-projects", "harbor_projects", "list", adapter("harbor")),
			route(http.MethodPut, "/api/v1/image-requests/batch/status", "image_requests", "batch_update", admin()),
			route(http.MethodPut, "/api/v1/image-requests/{id}/approve", "image_requests", "update", id("id"), admin()),
			route(http.MethodPut, "/api/v1/image-requests/{id}/reject", "image_requests", "update", id("id"), admin()),
			route(http.MethodGet, "/api/v1/images/build/{jobName}/logs", "image_build_logs", "list", id("jobName"), adapter("harbor")),
			route(http.MethodPost, "/api/v1/image-catalog/publish", "image_catalog", "command", adapter("harbor")),
			route(http.MethodDelete, "/api/v1/image-catalog/publish/{ruleId}", "image_catalog", "delete", id("ruleId"), adapter("harbor")),
			route(http.MethodDelete, "/api/v1/image-catalog/{tagId}", "image_catalog", "delete", id("tagId"), adapter("harbor")),
			route(http.MethodGet, "/api/v1/image-catalog/{tagId}/sync-status", "image_catalog", "get", id("tagId"), adapter("harbor")),
			route(http.MethodGet, "/api/v1/projects/{id}/image-requests", "image_requests", "list", id("id")),
			route(http.MethodGet, "/api/v1/projects/{id}/builds", "image_builds", "list", id("id")),
			route(http.MethodDelete, "/api/v1/projects/{id}/builds/{jobName}", "image_builds", "delete", id("jobName"), adapter("harbor")),
			route(http.MethodDelete, "/api/v1/projects/{id}/images/{image_id}", "project_images", "delete", id("image_id")),
		}
	case serviceUsageObservability:
		return []platform.RouteSpec{
			route(http.MethodGet, "/api/v1/cluster/gpu-usage", "cluster_gpu_usage", "list", admin()),
			route(http.MethodGet, "/api/v1/cluster/mps-mapping", "mps_read_models", "list"),
			route(http.MethodGet, "/api/v1/cluster/nodes", "cluster_nodes", "list", admin()),
			route(http.MethodGet, "/api/v1/cluster/nodes/{name}", "cluster_nodes", "get", id("name"), admin()),
			route(http.MethodGet, "/api/v1/admin/mps-mapping", "mps_read_models", "list", admin()),
			route(http.MethodGet, "/api/v1/admin/gpu/users/history", "gpu_users", "list_history", admin()),
			route(http.MethodGet, "/api/v1/admin/gpu/users/{userId}/jobs", "gpu_jobs", "list_by_user", id("userId"), admin()),
			route(http.MethodGet, "/api/v1/projects/gpu-usage/by-user", "project_gpu_usage", "list_by_user"),
			route(http.MethodGet, "/api/v1/projects/{id}/gpu-usage", "project_gpu_usage", "get", id("id")),
		}
	case serviceK8sControl:
		routes := []platform.RouteSpec{
			route(http.MethodGet, "/api/v1/k8s/namespaces/{ns}/pods/{name}/logs", "pod_logs", "list", id("name"), adapter("k8s")),
			route(http.MethodGet, "/api/v1/k8s/user-storage/status", "user_storage", "get", adapter("k8s")),
			route(http.MethodPost, "/api/v1/k8s/user-storage/browse", "user_storage", "command", adapter("k8s")),
			route(http.MethodDelete, "/api/v1/k8s/user-storage/browse", "user_storage", "command", adapter("k8s")),
			route(http.MethodGet, "/api/v1/ws/watch/{namespace}", "ws_namespace_watch", "proxy", id("namespace"), adapter("k8s")),
			route(http.MethodGet, "/api/v1/ws/watch-project/{projectId}", "ws_project_watch", "proxy", id("projectId"), adapter("k8s")),
			route(http.MethodGet, "/api/v1/ws/job-status/{id}", "ws_job_status", "proxy", id("id"), adapter("k8s")),
			route(http.MethodGet, "/api/v1/admin/resources", "resources", "list", admin(), adapter("k8s")),
			route(http.MethodDelete, "/api/v1/admin/resources/projects/{id}", "project_resources", "command", id("id"), admin(), adapter("k8s")),
			route(http.MethodGet, "/api/v1/projects/{id}/namespaces", "project_namespaces", "list", id("id"), adapter("k8s")),
			route(http.MethodDelete, "/api/v1/projects/{id}/resources/{userId}", "project_resources", "command", id("userId"), adapter("k8s")),
			route(http.MethodGet, "/api/v1/resources/{namespace}/pods/{name}/events", "pod_events", "list", id("name"), adapter("k8s")),
			route(http.MethodDelete, "/api/v1/resources/{namespace}/{kind}/{name}", "resources", "command", id("name"), adapter("k8s")),
		}
		routes = append(routes, anyCompatRoutes("/api/v1/k8s/user-storage/proxy/{path...}", "user_storage_proxy", "proxy", adapter("k8s"))...)
		return routes
	case serviceSchedulerQuota:
		return []platform.RouteSpec{
			route(http.MethodGet, catalogPathQueueID, "queues", "get", id("id"), admin()),
			route(http.MethodPut, catalogPathQueueID, "queues", "update", id("id"), admin()),
			route(http.MethodDelete, "/api/v1/queues/batch", "queues", "batch_delete", admin()),
			route(http.MethodGet, "/api/v1/queues/project/{project_id}", "queues", "list_by_project", id("project_id")),
			route(http.MethodGet, catalogPathPlanID, "plans", "get", id("id"), admin()),
			route(http.MethodPut, catalogPathPlanID, "plans", "update", id("id"), admin()),
			route(http.MethodDelete, "/api/v1/plans/batch", "plans", "batch_delete", admin()),
			route(http.MethodPut, "/api/v1/plans/bind/{project_id}", "plans", "bind_project", id("project_id"), admin()),
			route(http.MethodGet, catalogPathPlanID+"/queues", "plan_queues", "list", id("id"), admin()),
			route(http.MethodPut, catalogPathPlanID+"/queues", "plan_queues", "update", id("id"), admin()),
			route(http.MethodGet, catalogPathProjectID+"/quota/live", "live_quotas", "get", id("id")),
		}
	case serviceIdentity:
		routes := []platform.RouteSpec{
			public(route(http.MethodGet, "/api/v1/oidc/login", "oidc_login", "get")),
			public(route(http.MethodPost, "/api/v1/oidc/login", "oidc_login", "create")),
			public(route(http.MethodPost, "/oauth/token", "oidc_tokens", "create")),
			public(route(http.MethodPost, "/revoke", "oidc_tokens", "delete")),
			public(route(http.MethodPost, "/device_authorization", "oidc_devices", "create")),
			route(http.MethodGet, "/api/v1/users/paging", "users", "list_paging", admin()),
			route(http.MethodPost, "/api/v1/users/resolve", "users", "resolve", admin()),
			route(http.MethodPut, catalogPathUserID, "users", "update", id("id"), admin()),
			route(http.MethodPut, catalogPathUserSettings, "user_settings", "update", id("id")),
			route(http.MethodPut, "/api/v1/users/batch/role", "users", "batch_role_update", admin()),
			route(http.MethodPut, "/api/v1/users/batch/password", "users", "batch_password_reset", admin()),
		}
		routes = append(routes, publicRouteSet(anyCompatRoutes("/api/v1/.well-known/{path...}", "oidc", "proxy"))...)
		routes = append(routes, publicRouteSet(anyCompatRoutes("/api/v1/keys", "oidc_keys", "proxy"))...)
		routes = append(routes, publicRouteSet(anyCompatRoutes("/api/v1/authorize", "oidc_authorize", "proxy"))...)
		routes = append(routes, publicRouteSet(anyCompatRoutes("/api/v1/userinfo", "oidc_userinfo", "proxy"))...)
		routes = append(routes, publicRouteSet(anyCompatRoutes("/api/v1/authorize/callback", "oidc_authorize_callback", "proxy"))...)
		return routes
	case serviceWorkload:
		return []platform.RouteSpec{
			route(http.MethodPut, catalogPathConfigFileID, "configfiles", "update", id("id")),
			route(http.MethodGet, "/api/v1/configfiles/project/{project_id}", "configfiles", "list", id("project_id")),
			route(http.MethodGet, "/api/v1/configfiles/project/{project_id}/tree", "configfiles", "tree", id("project_id")),
			route(http.MethodGet, "/api/v1/configfiles/project/{project_id}/history", "configfiles", "list_versions", id("project_id")),
			route(http.MethodGet, "/api/v1/projects/{id}/config-files", "configfiles", "list", id("id")),
		}
	case serviceRequestNotification:
		return nil
	case serviceIDE:
		routes := []platform.RouteSpec{
			route(http.MethodPost, "/api/v1/ide/start", "ide_sessions", "command"),
			route(http.MethodPost, "/api/v1/ide/stop", "ide_sessions", "command"),
			route(http.MethodPost, "/api/v1/ide/delete", "ide_sessions", "command"),
		}
		routes = append(routes, anyCompatRoutes("/api/v1/ide/proxy/{podName}/{path...}", "ide_proxy", "proxy", id("podName"), adapter("k8s"))...)
		return routes
	case serviceIntegrationProxy:
		routes := []platform.RouteSpec{
			route(http.MethodGet, "/api/v1/admin/vpn/clients", "vpn_clients", "list", admin()),
			route(http.MethodGet, "/api/v1/admin/vpn/usage", "vpn_usage", "list", admin()),
			route(http.MethodDelete, "/api/v1/admin/vpn/clients/{cn}", "vpn_clients", "command", id("cn"), admin()),
		}
		routes = append(routes, anyCompatRoutes("/api/v1/grafana/{path...}", "grafana_proxy", "proxy", adapter("prometheus"))...)
		routes = append(routes, anyCompatRoutes("/api/v1/minio-console/{path...}", "minio_proxy", "proxy", adapter("minio"))...)
		routes = append(routes, anyCompatRoutes("/api/v1/pgadmin/{path...}", "pgadmin_proxy", "proxy", adapter("pgadmin"))...)
		routes = append(routes, anyCompatRoutes("/api/v1/longhorn/{path...}", "longhorn_proxy", "proxy", adapter("longhorn"))...)
		routes = append(routes, anyCompatRoutes("/api/v1/harbor/{path...}", "harbor_ui_proxy", "proxy", adapter("harbor"))...)
		routes = append(routes, anyCompatRoutes("/api/v1/harbor-gpu23/{path...}", "harbor_gpu23_proxy", "proxy", adapter("harbor"))...)
		return routes
	case serviceMediaUpload:
		return nil
	default:
		return nil
	}
}

func platformGateway() platform.ServiceSpec {
	return platform.ServiceSpec{
		Name:        "platform-gateway",
		Category:    "edge",
		Phase:       "1",
		Description: "External /api/v1 compatibility layer, auth entry, rate limiting, route mapping, and degraded downstream proxy.",
		Tables:      []string{"route_config_cache", "jwks_cache", "rate_limit_counters"},
		Events:      []string{"PolicyChanged", "ProxyPolicyChanged", "AnnouncementPublished"},
		Routes: []platform.RouteSpec{
			route(http.MethodGet, "/api/v1/gateway/routes", "routes", "list"),
			route(http.MethodGet, "/api/v1/gateway/health", "health", "list"),
			route(http.MethodGet, catalogPathCompatProxy, "compat_proxy", "proxy", adapter("monolith")),
			route(http.MethodPost, catalogPathCompatProxy, "compat_proxy", "proxy", adapter("monolith")),
			route(http.MethodPut, catalogPathCompatProxy, "compat_proxy", "proxy", adapter("monolith")),
			route(http.MethodPatch, catalogPathCompatProxy, "compat_proxy", "proxy", adapter("monolith")),
			route(http.MethodDelete, catalogPathCompatProxy, "compat_proxy", "proxy", adapter("monolith")),
		},
	}
}

func identityService() platform.ServiceSpec {
	return platform.ServiceSpec{
		Name:        serviceIdentity,
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
			route(http.MethodGet, catalogPathUserID, "users", "get", id("id")),
			route(http.MethodPatch, catalogPathUserID, "users", "update", id("id"), admin()),
			route(http.MethodDelete, catalogPathUserID, "users", "delete", id("id"), admin()),
			route(http.MethodPost, "/api/v1/users/batch", "users", "batch_create", admin()),
			route(http.MethodDelete, "/api/v1/users/batch", "users", "batch_delete", admin()),
			route(http.MethodPost, "/api/v1/users/batch/password-reset", "users", "batch_password_reset", admin()),
			route(http.MethodPatch, "/api/v1/users/batch/roles", "users", "batch_role_update", admin()),
			route(http.MethodGet, catalogPathUserSettings, "user_settings", "get", id("id")),
			route(http.MethodPatch, catalogPathUserSettings, "user_settings", "update", id("id")),
			public(route(http.MethodGet, "/api/v1/oidc/.well-known/openid-configuration", "oidc", "discovery")),
			public(route(http.MethodGet, "/api/v1/oidc/jwks", "jwks", "list")),
			public(route(http.MethodGet, "/api/v1/oidc/authorize", "oidc_authorizations", "create")),
			public(route(http.MethodPost, "/api/v1/oidc/token", "oidc_tokens", "create")),
			route(http.MethodGet, "/api/v1/oidc/userinfo", "oidc_userinfo", "list"),
			route(http.MethodPost, "/api/v1/oidc/revoke", "oidc_tokens", "delete"),
			public(route(http.MethodGet, "/api/v1/oidc/callback", "oidc_callbacks", "create")),
			public(route(http.MethodPost, "/api/v1/oidc/callback", "oidc_callbacks", "create")),
		},
	}
}

func authorizationPolicyService() platform.ServiceSpec {
	return platform.ServiceSpec{
		Name:            serviceAuthorizationPolicy,
		Category:        "core",
		Phase:           "4",
		RequiresCluster: true,
		Description:     "Central PDP, Casbin/domain RBAC, proxy RBAC, policy simulation, signed policy bundles, and policy sync.",
		Tables:          []string{"casbin_rule", "policies", "policy_rules", "policy_assignments", "platform_roles", "user_platform_roles", "service_definitions", "identity_users", "identity_roles", "outbox", "inbox"},
		Events:          []string{"PolicyChanged", "ProxyPolicyChanged"},
		Routes: []platform.RouteSpec{
			route(http.MethodPost, "/api/v1/permissions/enforce", "decisions", "create", policyBypass()),
			route(http.MethodPost, "/api/v1/permissions/simulate", "decisions", "simulate", admin()),
			route(http.MethodGet, "/api/v1/permissions/policies", "policies", "list", admin()),
			route(http.MethodPost, "/api/v1/permissions/policies", "policies", "create", admin()),
			route(http.MethodPatch, "/api/v1/permissions/policies/{id}", "policies", "update", id("id"), admin()),
			route(http.MethodDelete, "/api/v1/permissions/policies/{id}", "policies", "delete", id("id"), admin()),
			route(http.MethodPost, "/api/v1/permissions/policies/batch", "policies", "batch", admin()),
			route(http.MethodGet, "/api/v1/admin/proxy-rbac/services", "proxy_services", "list", admin()),
			route(http.MethodPost, "/api/v1/admin/proxy-rbac/services", "proxy_services", "create", admin()),
			route(http.MethodGet, "/api/v1/admin/proxy-rbac/policies", "proxy_policies", "list", admin()),
			route(http.MethodPost, "/api/v1/admin/proxy-rbac/policies", "proxy_policies", "create", admin()),
			route(http.MethodPatch, catalogPathProxyRBACPolicyID, "proxy_policies", "update", id("id"), admin()),
			route(http.MethodDelete, catalogPathProxyRBACPolicyID, "proxy_policies", "delete", id("id"), admin()),
			route(http.MethodPost, "/api/v1/admin/proxy-rbac/assignments", "proxy_assignments", "create", admin()),
			route(http.MethodGet, "/api/v1/admin/proxy-rbac/platform-roles", "platform_roles", "list", admin()),
			route(http.MethodPost, "/api/v1/admin/proxy-rbac/role-users", "role_users", "create", admin()),
		},
	}
}

func orgProjectService() platform.ServiceSpec {
	return platform.ServiceSpec{
		Name:        serviceOrgProject,
		Category:    "core",
		Phase:       "4",
		Description: "Groups, user groups, project tree, project members, quotas, workspace settings, and GPU claims.",
		Tables:      []string{"resource_owners", "groups", "user_groups", "projects", "project_members", "user_quotas", "project_workspace_settings", "gpu_claims", "identity_users", "identity_roles", "outbox", "inbox"},
		Events:      []string{"GroupCreated", "GroupMembershipChanged", "ProjectCreated", "ProjectUpdated", "ProjectDeleted"},
		Routes: []platform.RouteSpec{
			route(http.MethodGet, "/api/v1/groups", "groups", "list"),
			route(http.MethodPost, "/api/v1/groups", "groups", "create", admin()),
			route(http.MethodPatch, catalogPathGroupID, "groups", "update", id("id"), admin()),
			route(http.MethodDelete, catalogPathGroupID, "groups", "delete", id("id"), admin()),
			route(http.MethodDelete, "/api/v1/groups/batch", "groups", "batch_delete", admin()),
			route(http.MethodGet, catalogPathUserGroups, "user_groups", "list"),
			route(http.MethodPost, catalogPathUserGroups, "user_groups", "create"),
			route(http.MethodPatch, catalogPathUserGroups+"/{id}", "user_groups", "update", id("id")),
			route(http.MethodDelete, catalogPathUserGroups+"/{id}", "user_groups", "delete", id("id")),
			route(http.MethodPost, "/api/v1/user-groups/batch", "user_groups", "batch_join"),
			route(http.MethodGet, catalogPathUserGroups+"/add-members-context", "user_groups", "add_members_context"),
			route(http.MethodPost, catalogPathUserGroups+"/resolve-add-members", "user_groups", "resolve_add_members"),
			route(http.MethodGet, "/api/v1/projects", "projects", "list"),
			route(http.MethodPost, "/api/v1/projects", "projects", "create"),
			route(http.MethodGet, catalogPathProjectID, "projects", "get", id("id")),
			route(http.MethodPatch, catalogPathProjectID, "projects", "update", id("id")),
			route(http.MethodDelete, catalogPathProjectID, "projects", "delete", id("id")),
			route(http.MethodDelete, "/api/v1/projects/batch", "projects", "batch_delete"),
			route(http.MethodGet, catalogPathProjectMembers, "project_members", "list", id("id")),
			route(http.MethodPost, catalogPathProjectMembers, "project_members", "create", id("id")),
			route(http.MethodDelete, catalogPathProjectMembers+"/{userId}", "project_members", "delete", id("userId")),
			route(http.MethodPatch, catalogPathProjectMembers+"/{userId}/role", "project_members", "update_role", id("userId")),
			route(http.MethodPatch, catalogPathProjectMembers+"/batch/roles", "project_members", "batch_role_update", id("id")),
			route(http.MethodDelete, catalogPathProjectMembers+"/batch", "project_members", "batch_delete", id("id")),
			route(http.MethodGet, catalogPathProjectMemberQuota, "member_quotas", "get", id("userId")),
			route(http.MethodPut, catalogPathProjectMemberQuota, "member_quotas", "update", id("userId")),
			route(http.MethodDelete, catalogPathProjectMemberQuota, "member_quotas", "delete", id("userId")),
			route(http.MethodGet, catalogPathProjectID+"/workspace-settings", "workspace_settings", "get", id("id")),
			route(http.MethodPut, catalogPathProjectID+"/workspace-settings", "workspace_settings", "update", id("id")),
			route(http.MethodGet, catalogPathProjectID+"/gpu-claims", "gpu_claims", "list", id("id")),
			route(http.MethodPost, catalogPathProjectID+"/gpu-claims", "gpu_claims", "create", id("id")),
			route(http.MethodDelete, catalogPathProjectID+"/gpu-claims/{requestId}", "gpu_claims", "delete", id("requestId")),
			route(http.MethodPut, "/internal/org-project/projects/{project_id}/plan", "projects", "bind_plan", id("project_id"), serviceInternal()),
			route(http.MethodDelete, "/internal/org-project/plans/{plan_id}/project-bindings", "projects", "clear_plan_bindings", serviceInternal()),
		},
	}
}

func workloadService() platform.ServiceSpec {
	return platform.ServiceSpec{
		Name:            serviceWorkload,
		Category:        "compute",
		Phase:           "5",
		RequiresCluster: true,
		Description:     "Immutable ConfigFiles, job submission, job state machine, job logs, templates, and workload saga orchestration.",
		Tables:          []string{"config_files", "config_blobs", "config_commits", "jobs", "job_logs", "job_templates", "outbox", "inbox"},
		Events:          []string{"ConfigCommitted", "JobSubmitted", "JobQueued", "JobRunning", "JobSucceeded", "JobFailed", "JobCancelled"},
		Routes: []platform.RouteSpec{
			route(http.MethodGet, "/api/v1/configfiles", "configfiles", "list"),
			route(http.MethodPost, "/api/v1/configfiles", "configfiles", "create"),
			route(http.MethodGet, catalogPathConfigFileID, "configfiles", "get", id("id")),
			route(http.MethodPatch, catalogPathConfigFileID, "configfiles", "update", id("id")),
			route(http.MethodDelete, catalogPathConfigFileID, "configfiles", "delete", id("id")),
			route(http.MethodPost, catalogPathConfigFileID+"/versions", "configfiles", "config_commit", id("id")),
			route(http.MethodGet, catalogPathConfigFileID+"/versions", "configfiles", "list_versions", id("id")),
			route(http.MethodGet, "/api/v1/configfiles/tree", "configfiles", "tree"),
			route(http.MethodPost, catalogPathConfigFileID+"/instance", "instances", "command", id("id"), adapter("k8s")),
			route(http.MethodDelete, catalogPathConfigFileID+"/instance", "instances", "command", id("id"), adapter("k8s")),
			route(http.MethodGet, catalogPathConfigFileID+"/instance/pods", "instances", "list", id("id")),
			route(http.MethodGet, "/api/v1/jobs/templates", "job_templates", "list"),
			route(http.MethodGet, "/api/v1/jobs", "jobs", "list"),
			route(http.MethodPost, "/api/v1/jobs", "jobs", "command"),
			route(http.MethodGet, "/api/v1/jobs/{id}", "jobs", "get", id("id")),
			route(http.MethodPost, "/api/v1/jobs/{id}/cancel", "jobs", "command", id("id")),
			route(http.MethodGet, "/api/v1/jobs/{id}/logs", "job_logs", "list", id("id")),
			route(http.MethodGet, "/api/v1/jobs/{id}/gpu-summary", "job_gpu_usage", "list", id("id")),
			route(http.MethodGet, "/api/v1/jobs/{id}/gpu-timeline", "job_gpu_usage", "list", id("id")),
			route(http.MethodGet, "/api/v1/jobs/{id}/gpu-breakdown", "job_gpu_usage", "list", id("id")),
			route(http.MethodGet, "/internal/workload/preemption-context", "preemption_context", "internal_read", serviceInternal()),
			route(http.MethodPost, "/internal/workload/jobs/{id}/preempt", "jobs", "preempt", id("id"), serviceInternal()),
			route(http.MethodPost, "/internal/workload/jobs/{id}/evict", "jobs", "evict", id("id"), serviceInternal()),
		},
	}
}

func schedulerQuotaService() platform.ServiceSpec {
	return platform.ServiceSpec{
		Name:            serviceSchedulerQuota,
		Category:        "compute",
		Phase:           "5",
		RequiresCluster: true,
		Description:     "Resource plans, queues, quota reservation, priority, preemption, and queue dispatch arbitration.",
		Tables:          []string{"plans", "queues", "resource_quotas", "submit_admissions", "priority_classes", "reservations", "preemption_records", "gpu_claim_snapshots", "outbox", "inbox"},
		Events:          []string{"PlanChanged", "QuotaReserved", "QuotaCommitted", "QuotaReleased", "SubmitAdmissionReviewed", "QueueDepthChanged", "JobPreempted", "PriorityClassSyncCompleted"},
		Routes: []platform.RouteSpec{
			route(http.MethodGet, "/api/v1/plans", "plans", "list"),
			route(http.MethodPost, "/api/v1/plans", "plans", "create", admin()),
			route(http.MethodPatch, catalogPathPlanID, "plans", "update", id("id"), admin()),
			route(http.MethodDelete, catalogPathPlanID, "plans", "delete", id("id"), admin()),
			route(http.MethodGet, "/api/v1/queues", "queues", "list"),
			route(http.MethodPost, "/api/v1/queues", "queues", "create", admin()),
			route(http.MethodPatch, catalogPathQueueID, "queues", "update", id("id"), admin()),
			route(http.MethodDelete, catalogPathQueueID, "queues", "delete", id("id"), admin()),
			route(http.MethodPost, "/api/v1/internal/quota/reservations", "reservations", "quota_reserve"),
			route(http.MethodPost, "/api/v1/internal/quota/reservations/{reservationId}/commit", "reservations", "quota_commit", id("reservationId")),
			route(http.MethodPost, "/api/v1/internal/quota/reservations/{reservationId}/release", "reservations", "quota_release", id("reservationId")),
			route(http.MethodPost, "/api/v1/internal/scheduler/admission", "submit_admissions", "review"),
			route(http.MethodPost, "/api/v1/internal/scheduler/preemptions", "preemptions", "command"),
			route(http.MethodPost, "/api/v1/internal/workers/leases", "worker_leases", "worker_lease"),
		},
	}
}

func k8sControlService() platform.ServiceSpec {
	return platform.ServiceSpec{
		Name:            serviceK8sControl,
		Category:        "compute-infra",
		Phase:           "5",
		RequiresCluster: true,
		Description:     "Single Kubernetes API adapter, resource snapshots, command/status APIs, pod logs/events, and watch contracts.",
		Tables:          []string{"k8s_operations", "namespace_mappings", "pod_snapshots", "resource_snapshots", "outbox", "inbox"},
		Events:          []string{"ResourceSnapshotRecorded", "NamespaceCreated", "NamespaceDeleted"},
		Routes: []platform.RouteSpec{
			route(http.MethodGet, "/api/v1/k8s/cluster", "cluster_snapshots", "list", adapter("k8s")),
			route(http.MethodGet, "/api/v1/k8s/nodes", "nodes", "list", adapter("k8s")),
			route(http.MethodGet, "/api/v1/k8s/nodes/{id}", "nodes", "get", id("id"), adapter("k8s")),
			route(http.MethodGet, "/api/v1/k8s/pods/{id}/logs", "pod_logs", "list", id("id"), adapter("k8s")),
			route(http.MethodGet, "/api/v1/k8s/pods/{id}/events", "pod_events", "list", id("id"), adapter("k8s")),
			route(http.MethodDelete, "/api/v1/k8s/resources/{id}", "resources", "command", id("id"), adapter("k8s")),
			route(http.MethodGet, "/api/v1/resources", "resources", "list", adapter("k8s")),
			route(http.MethodDelete, "/api/v1/resources/{id}", "resources", "command", id("id"), adapter("k8s")),
			route(http.MethodGet, "/api/v1/projects/{id}/resources", "project_resources", "list", id("id"), adapter("k8s")),
			route(http.MethodPost, "/api/v1/projects/{id}/resources/cleanup", "project_resources", "command", id("id"), adapter("k8s")),
			route(http.MethodGet, "/api/v1/ws/exec", "ws_exec", "proxy", adapter("k8s")),
			route(http.MethodGet, "/api/v1/ws/namespace-watch", "ws_namespace_watch", "proxy", adapter("k8s")),
			route(http.MethodGet, "/api/v1/ws/pod-logs", "ws_pod_logs", "proxy", adapter("k8s")),
			route(http.MethodGet, "/api/v1/ws/project-watch", "ws_project_watch", "proxy", adapter("k8s")),
			route(http.MethodGet, "/api/v1/ws/job-status", "ws_job_status", "proxy", adapter("k8s")),
			route(http.MethodGet, "/api/v1/ws/storage-status", "ws_storage_status", "proxy", adapter("k8s")),
		},
	}
}

func ideService() platform.ServiceSpec {
	return platform.ServiceSpec{
		Name:        serviceIDE,
		Category:    "compute",
		Phase:       "5",
		Description: "Interactive workspace lifecycle, IDE proxy, activity tracking, idle reaping, and image listing.",
		Tables:      []string{"ide_sessions", "workspace_activity", "pod_mappings", "ide_identity_roles", "ide_identity_users", "ide_policy_roles", "ide_project_members", "ide_projects", "ide_user_groups", "outbox", "inbox"},
		Events:      []string{"IDEStarted", "IDEStopped", "IDEDeleted", "IDEIdleReaped"},
		Routes: []platform.RouteSpec{
			route(http.MethodGet, "/api/v1/ide", "ide_sessions", "list"),
			route(http.MethodPost, "/api/v1/ide", "ide_sessions", "command", adapter("k8s")),
			route(http.MethodGet, "/api/v1/ide/images", "ide_images", "list"),
			route(http.MethodPost, "/api/v1/ide/{id}/stop", "ide_sessions", "command", id("id"), adapter("k8s")),
			route(http.MethodDelete, "/api/v1/ide/{id}", "ide_sessions", "command", id("id"), adapter("k8s")),
			route(http.MethodPost, "/api/v1/ide/{id}/activity", "ide_activity", "create", id("id")),
			route(http.MethodPost, "/api/v1/ide/reap-idle", "ide_sessions", "command"),
			route(http.MethodGet, "/api/v1/ide/proxy/{podName}/{path...}", "ide_proxy", "proxy", id("podName"), adapter("k8s")),
		},
	}
}

func storageService() platform.ServiceSpec {
	return platform.ServiceSpec{
		Name:            serviceStorage,
		Category:        "data",
		Phase:           "2",
		RequiresCluster: true,
		Description:     "User/group storage, PVC lifecycle, FileBrowser, permissions, project bindings, fast-stage transfer, and Longhorn RWX health.",
		Tables:          []string{"storages", "group_storage_permissions", "access_policies", "project_storage_bindings", "fast_transfer_records", "longhorn_rwx_health", "outbox", "inbox"},
		Events:          []string{"PVCProvisioned", "StorageBound", "StoragePermissionChanged", "FastTransferCompleted", "LonghornRWXHealthChecked"},
		Routes: []platform.RouteSpec{
			route(http.MethodPost, "/internal/storage/projects/{project_id}/mount-plan", "mount_plans", "resolve", serviceInternal()),
			route(http.MethodGet, "/api/v1/admin/user-storage", "user_storage", "list", admin(), adapter("minio")),
			route(http.MethodPost, "/api/v1/admin/user-storage/{id}/init", "user_storage", "command", id("id"), admin(), adapter("minio")),
			route(http.MethodPost, "/api/v1/admin/user-storage/{id}/expand", "user_storage", "command", id("id"), admin(), adapter("minio")),
			route(http.MethodDelete, "/api/v1/admin/user-storage/{id}", "user_storage", "command", id("id"), admin(), adapter("minio")),
			route(http.MethodGet, "/api/v1/k8s/user-storage/{id}", "user_storage", "get", id("id"), adapter("minio")),
			route(http.MethodPost, "/api/v1/storage/filebrowser/{id}/open", "filebrowser", "command", id("id"), adapter("minio")),
			route(http.MethodPost, "/api/v1/storage/filebrowser/{id}/stop", "filebrowser", "command", id("id"), adapter("minio")),
			route(http.MethodGet, "/api/v1/storage/filebrowser/{id}/proxy/{path...}", "filebrowser_proxy", "proxy", id("id"), adapter("minio")),
			route(http.MethodGet, "/api/v1/admin/group-storage", "group_storage", "list", admin(), adapter("minio")),
			route(http.MethodPost, "/api/v1/admin/group-storage", "group_storage", "create", admin(), adapter("minio")),
			route(http.MethodDelete, "/api/v1/admin/group-storage/{id}", "group_storage", "delete", id("id"), admin(), adapter("minio")),
			route(http.MethodGet, "/api/v1/storage/permissions", "storage_permissions", "list"),
			route(http.MethodPost, "/api/v1/storage/permissions", "storage_permissions", "create"),
			route(http.MethodPost, "/api/v1/storage/permissions/batch", "storage_permissions", "batch_set"),
			route(http.MethodDelete, "/api/v1/storage/permissions/{id}", "storage_permissions", "delete", id("id")),
			route(http.MethodGet, "/api/v1/projects/{id}/storage/bindings", "storage_bindings", "list", id("id")),
			route(http.MethodPost, "/api/v1/projects/{id}/storage/bindings", "storage_bindings", "create", id("id")),
			route(http.MethodDelete, "/api/v1/projects/{id}/storage/bindings/{requestId}", "storage_bindings", "delete", id("requestId")),
			route(http.MethodGet, "/api/v1/projects/{id}/storage/permissions", "project_storage_permissions", "list", id("id")),
			route(http.MethodPost, "/api/v1/projects/{id}/storage/permissions", "project_storage_permissions", "create", id("id")),
			route(http.MethodDelete, "/api/v1/projects/{id}/storage/permissions/{requestId}", "project_storage_permissions", "delete", id("requestId")),
			route(http.MethodGet, "/api/v1/projects/{id}/storage/transfers", "fast_transfers", "list", id("id")),
			route(http.MethodPost, "/api/v1/projects/{id}/storage/transfers", "fast_transfers", "command", id("id"), adapter("minio")),
			route(http.MethodPost, "/api/v1/projects/{id}/storage/transfers/{requestId}/cancel", "fast_transfers", "command", id("requestId"), adapter("minio")),
		},
	}
}

func imageRegistryService() platform.ServiceSpec {
	return platform.ServiceSpec{
		Name:        serviceImageRegistry,
		Category:    "supply-chain",
		Phase:       "2",
		Description: "Image requests, allow-lists, builds, catalog sync/publish, Harbor API integration, and governance state.",
		Tables:      []string{"container_repositories", "container_tags", "sync_targets", "image_allow_lists", "image_requests", "image_build_jobs", "outbox", "inbox"},
		Events:      []string{"ImageRequested", "ImageApproved", "ImageBuildStarted", "ImageBuilt", "ImagePublished", "ImageSyncFailed"},
		Routes: []platform.RouteSpec{
			route(http.MethodGet, "/api/v1/projects/{id}/images", "project_images", "list", id("id")),
			route(http.MethodPost, "/api/v1/projects/{id}/images", "project_images", "create", id("id")),
			route(http.MethodDelete, "/api/v1/projects/{id}/images/{requestId}", "project_images", "delete", id("requestId")),
			route(http.MethodGet, "/api/v1/image-requests", "image_requests", "list"),
			route(http.MethodPost, "/api/v1/image-requests", "image_requests", "create"),
			route(http.MethodPatch, "/api/v1/image-requests/{id}", "image_requests", "update", id("id"), admin()),
			route(http.MethodPatch, "/api/v1/image-requests/batch", "image_requests", "batch_update", admin()),
			route(http.MethodPost, "/api/v1/images/build", "image_builds", "command", adapter("harbor")),
			route(http.MethodPost, "/api/v1/images/build/from-storage", "image_builds", "command", adapter("harbor")),
			route(http.MethodPost, "/api/v1/images/build/dockerfile", "image_builds", "command", adapter("harbor")),
			route(http.MethodGet, "/api/v1/images/build/{buildId}/logs", "image_build_logs", "list", id("buildId"), adapter("harbor")),
			route(http.MethodGet, "/api/v1/projects/{id}/image-builds", "image_builds", "list", id("id")),
			route(http.MethodDelete, "/api/v1/projects/{id}/image-builds/{buildId}", "image_builds", "delete", id("buildId"), adapter("harbor")),
			route(http.MethodGet, "/api/v1/image-catalog", "image_catalog", "list", adapter("harbor")),
			route(http.MethodPost, "/api/v1/image-catalog/sync", "image_catalog", "command", adapter("harbor")),
			route(http.MethodPost, "/api/v1/image-catalog/{id}/publish", "image_catalog", "command", id("id"), adapter("harbor")),
			route(http.MethodPost, "/api/v1/image-catalog/{id}/unpublish", "image_catalog", "command", id("id"), adapter("harbor")),
			route(http.MethodDelete, "/api/v1/image-catalog/{id}", "image_catalog", "delete", id("id"), adapter("harbor")),
			route(http.MethodGet, "/api/v1/image-catalog/sync-status", "image_catalog", "list", adapter("harbor")),
			route(http.MethodGet, "/api/v1/harbor-status", "harbor_status", "list", adapter("harbor")),
			route(http.MethodGet, "/api/v1/harbor-statistics", "harbor_statistics", "list", adapter("harbor")),
		},
	}
}

func usageObservabilityService() platform.ServiceSpec {
	return platform.ServiceSpec{
		Name:            serviceUsageObservability,
		Category:        "ops-read-model",
		Phase:           "3",
		RequiresCluster: true,
		Description:     "GPU usage, resource hours, cluster summary, dashboards, Prometheus queries, snapshots, and retention cleanup.",
		Tables:          []string{"job_gpu_usage_snapshots", "job_gpu_usage_summaries", "pod_resource_records", "resource_hour_summaries", "gpu_authorization_roles", "gpu_identity_roles", "gpu_identity_users", "gpu_jobs", "gpu_projects", "cluster_read_models", "cluster_identity_users", "cluster_identity_roles", "cluster_policy_roles", "cluster_policy_role_assignments", "cluster_projects", "cluster_project_members", "cluster_user_groups", "dashboard_users", "dashboard_projects", "dashboard_project_members", "dashboard_forms", "dashboard_live_quotas", "dashboard_queues", "outbox", "inbox"},
		Events:          []string{"UsageSnapshotRecorded", "ResourceHoursSummarized"},
		Routes: []platform.RouteSpec{
			route(http.MethodGet, "/api/v1/me/usage", "usage", "list"),
			route(http.MethodGet, "/api/v1/me/gpu/jobs", "gpu_jobs", "list"),
			route(http.MethodGet, "/api/v1/me/request-usage", "request_usage", "list"),
			route(http.MethodGet, "/api/v1/admin/usage", "admin_usage", "list", admin()),
			route(http.MethodGet, "/api/v1/admin/request-usage", "admin_request_usage", "list", admin()),
			route(http.MethodGet, "/api/v1/admin/gpu/users", "gpu_users", "list", admin()),
			route(http.MethodGet, "/api/v1/dashboard/overview", "dashboard", "list"),
			route(http.MethodGet, "/api/v1/admin/dashboard-summary", "dashboard", "list", admin()),
			route(http.MethodGet, "/api/v1/cluster/summary", "cluster_read_models", "list"),
			route(http.MethodGet, "/api/v1/cluster/mps", "mps_read_models", "list", adapter("prometheus")),
			route(http.MethodGet, "/api/v1/resource-hours", "resource_hours", "list"),
			route(http.MethodPost, "/api/v1/internal/usage/snapshots", "usage_snapshots", "command", adapter("prometheus")),
			route(http.MethodPost, "/api/v1/internal/usage/cleanup", "usage_retention", "command"),
		},
	}
}

func auditComplianceService() platform.ServiceSpec {
	return platform.ServiceSpec{
		Name:        serviceAuditCompliance,
		Category:    "ops",
		Phase:       "1",
		Description: "Audit event ingestion, audit logs, project audit reports, security posture, retention, and cleanup.",
		Tables:      []string{"audit_logs", "security_findings", "security_reports", "event_ingestion_offsets", "project_report_members", "outbox", "inbox"},
		Events:      []string{"AuditEvent"},
		Routes: []platform.RouteSpec{
			route(http.MethodPost, "/api/v1/audit/events", "audit_events", "event_ingest"),
			route(http.MethodGet, "/api/v1/audit/logs", "audit_logs", "list", admin()),
			route(http.MethodGet, "/api/v1/audit/report", "audit_reports", "list"),
			route(http.MethodGet, "/api/v1/admin/security/posture", "security_posture", "list", admin()),
			route(http.MethodPost, "/api/v1/internal/audit/cleanup", "audit_retention", "command", admin()),
		},
	}
}

func requestNotificationService() platform.ServiceSpec {
	return platform.ServiceSpec{
		Name:        serviceRequestNotification,
		Category:    "collaboration",
		Phase:       "1",
		Description: "Forms, form messages, notifications, announcements, unread counts, and mark-read state.",
		Tables:      []string{"forms", "form_messages", "notifications", "announcements", "announcement_reads", "project_access_projects", "project_access_members", "project_access_user_groups", "outbox", "inbox"},
		Events:      []string{"FormCreated", "FormUpdated", "NotificationRequested", "AnnouncementPublished"},
		Routes: []platform.RouteSpec{
			route(http.MethodGet, "/api/v1/forms", "forms", "list"),
			route(http.MethodPost, "/api/v1/forms", "forms", "create"),
			route(http.MethodGet, "/api/v1/forms/my", "forms", "list_my"),
			route(http.MethodGet, "/api/v1/forms/{id}", "forms", "get", id("id")),
			route(http.MethodPut, "/api/v1/forms/{id}", "forms", "update", id("id"), admin()),
			route(http.MethodPut, "/api/v1/forms/{id}/status", "forms", "update", id("id"), admin()),
			route(http.MethodPut, "/api/v1/forms/batch/status", "forms", "batch_status", admin()),
			route(http.MethodGet, "/api/v1/forms/{id}/messages", "form_messages", "list", id("id")),
			route(http.MethodPost, "/api/v1/forms/{id}/messages", "form_messages", "create", id("id")),
			route(http.MethodPut, "/api/v1/notifications/{id}/read", "notifications", "update", id("id")),
			route(http.MethodPut, "/api/v1/notifications/read-all", "notifications", "update"),
			route(http.MethodDelete, "/api/v1/notifications/clear-all", "notifications", "delete"),
			route(http.MethodGet, "/api/v1/announcements/active", "announcements", "list"),
			route(http.MethodGet, "/api/v1/announcements/unread-count", "announcement_reads", "list"),
			route(http.MethodGet, "/api/v1/announcements/{id}", "announcements", "get", id("id")),
			route(http.MethodPut, "/api/v1/announcements/{id}/read", "announcement_reads", "create", id("id")),
			route(http.MethodGet, "/api/v1/admin/announcements", "announcements", "list", admin()),
			route(http.MethodPost, "/api/v1/admin/announcements", "announcements", "create", admin()),
			route(http.MethodPut, "/api/v1/admin/announcements/{id}", "announcements", "update", id("id"), admin()),
			route(http.MethodDelete, "/api/v1/admin/announcements/{id}", "announcements", "delete", id("id"), admin()),
		},
	}
}

func integrationProxyService() platform.ServiceSpec {
	return platform.ServiceSpec{
		Name:        serviceIntegrationProxy,
		Category:    "edge-tools",
		Phase:       "2",
		Description: "External tool UI proxies, SSO callbacks, proxy auth-check, and VPN administration.",
		Tables:      []string{"proxy_sessions", "proxy_cache", "vpn_clients", "vpn_usage_sessions", "admin_users", "admin_roles", "outbox", "inbox"},
		Events:      []string{"ProxySessionStarted", "ProxySessionTerminated"},
		Routes: []platform.RouteSpec{
			route(http.MethodGet, "/api/v1/grafana/{path...}", "grafana_proxy", "proxy", adapter("prometheus")),
			route(http.MethodGet, "/api/v1/minio-console-sso", "minio_sso", "proxy"),
			route(http.MethodGet, "/api/v1/minio-console/{path...}", "minio_proxy", "proxy", adapter("minio")),
			route(http.MethodGet, "/api/v1/pgadmin-sso", "pgadmin_sso", "proxy"),
			route(http.MethodGet, "/api/v1/pgadmin-auth-check", "pgadmin_auth_check", "proxy"),
			route(http.MethodGet, "/api/v1/pgadmin/{path...}", "pgadmin_proxy", "proxy", adapter("pgadmin")),
			route(http.MethodGet, "/api/v1/longhorn/{path...}", "longhorn_proxy", "proxy", adapter("longhorn")),
			route(http.MethodGet, "/api/v1/harbor/{path...}", "harbor_ui_proxy", "proxy", adapter("harbor")),
			route(http.MethodGet, "/api/v1/admin/vpn", "vpn_clients", "list", admin()),
			route(http.MethodPost, "/api/v1/admin/vpn/{id}/disconnect", "vpn_clients", "command", id("id"), admin()),
		},
	}
}

func mediaUploadService() platform.ServiceSpec {
	return platform.ServiceSpec{
		Name:        serviceMediaUpload,
		Category:    "support",
		Phase:       "1",
		Description: "Image uploads, JWT-only image serving, MinIO bucket abstraction, checksum, and owner references.",
		Tables:      []string{"uploaded_media", "outbox", "inbox"},
		Events:      []string{"MediaUploaded", "MediaDeleted"},
		Routes: []platform.RouteSpec{
			route(http.MethodPost, "/api/v1/uploads/images", "uploaded_media", "create"),
			route(http.MethodGet, "/api/v1/uploads/images/{key...}", "uploaded_media", "get", id("key")),
		},
	}
}

func route(method, pattern, resource, action string, opts ...func(*platform.RouteSpec)) platform.RouteSpec {
	spec := platform.RouteSpec{
		Method:        method,
		Pattern:       pattern,
		Resource:      resource,
		Action:        action,
		AuthRequired:  true,
		StateChanging: method != http.MethodGet,
	}
	for _, opt := range opts {
		opt(&spec)
	}
	return spec
}

func anyCompatRoutes(pattern, resource, action string, opts ...func(*platform.RouteSpec)) []platform.RouteSpec {
	methods := []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete}
	routes := make([]platform.RouteSpec, 0, len(methods))
	for _, method := range methods {
		routes = append(routes, route(method, pattern, resource, action, opts...))
	}
	return routes
}

func public(spec platform.RouteSpec) platform.RouteSpec {
	spec.AuthRequired = false
	return spec
}

func publicRouteSet(routes []platform.RouteSpec) []platform.RouteSpec {
	for i := range routes {
		routes[i] = public(routes[i])
	}
	return routes
}

func id(name string) func(*platform.RouteSpec) {
	return func(spec *platform.RouteSpec) {
		spec.IDParam = name
	}
}

func admin() func(*platform.RouteSpec) {
	return func(spec *platform.RouteSpec) {
		spec.Admin = true
	}
}

func serviceInternal() func(*platform.RouteSpec) {
	return func(spec *platform.RouteSpec) {
		spec.AuthRequired = false
		spec.PolicyBypass = true
	}
}

func adapter(name string) func(*platform.RouteSpec) {
	return func(spec *platform.RouteSpec) {
		spec.ExternalAdapter = name
	}
}

func policyBypass() func(*platform.RouteSpec) {
	return func(spec *platform.RouteSpec) {
		spec.PolicyBypass = true
	}
}
