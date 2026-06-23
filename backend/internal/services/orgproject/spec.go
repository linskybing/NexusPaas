package orgproject

import (
	"net/http"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

func Spec() platform.ServiceSpec {
	const (
		groupID            = "/api/v1/groups/{id}"
		projectID          = "/api/v1/projects/{id}"
		projectMembers     = projectID + "/members"
		projectMemberQuota = projectMembers + "/{userId}/quota"
		userGroups         = "/api/v1/user-groups"
	)
	route, id, admin, serviceInternal := shared.Route, shared.ID, shared.Admin, shared.ServiceInternal
	return platform.ServiceSpec{
		Name:        "org-project-service",
		Category:    "core",
		Phase:       "4",
		Description: "Groups, user groups, project tree, project members, quotas, workspace settings, and GPU claims.",
		Tables:      []string{"resource_owners", "groups", "user_groups", "projects", "project_members", "user_quotas", "project_workspace_settings", "gpu_claims", "identity_users", "identity_roles", "outbox", "inbox"},
		Events:      []string{"GroupCreated", "GroupUpdated", "GroupDeleted", "GroupMembershipChanged", "ProjectCreated", "ProjectUpdated", "ProjectDeleted"},
		Routes: []platform.RouteSpec{
			route(http.MethodGet, "/api/v1/groups", "groups", "list"),
			route(http.MethodPost, "/api/v1/groups", "groups", "create", admin()),
			route(http.MethodGet, groupID, "groups", "get", id("id")),
			route(http.MethodPut, groupID, "groups", "update", id("id"), admin()),
			route(http.MethodPatch, groupID, "groups", "update", id("id"), admin()),
			route(http.MethodDelete, groupID, "groups", "delete", id("id"), admin()),
			route(http.MethodDelete, "/api/v1/groups/batch", "groups", "batch_delete", admin()),
			route(http.MethodGet, "/api/v1/admin/group-policy-options", "group_policy_options", "list", admin()),
			route(http.MethodGet, userGroups, "user_groups", "list"),
			route(http.MethodPost, userGroups, "user_groups", "create"),
			route(http.MethodPut, userGroups, "user_groups", "update"),
			route(http.MethodDelete, userGroups, "user_groups", "delete"),
			route(http.MethodPatch, userGroups+"/{id}", "user_groups", "update", id("id")),
			route(http.MethodDelete, userGroups+"/{id}", "user_groups", "delete", id("id")),
			route(http.MethodPost, "/api/v1/user-groups/batch", "user_groups", "batch_join"),
			route(http.MethodGet, userGroups+"/by-group", "user_groups", "list_by_group"),
			route(http.MethodGet, userGroups+"/by-user", "user_groups", "list_by_user"),
			route(http.MethodGet, userGroups+"/add-members-context", "user_groups", "add_members_context"),
			route(http.MethodPost, userGroups+"/resolve-add-members", "user_groups", "resolve_add_members"),
			route(http.MethodGet, userGroups+"/{group_id}/members", "user_groups", "list_members", id("group_id")),
			route(http.MethodGet, userGroups+"/{group_id}/add-members-context", "user_groups", "add_members_context", id("group_id")),
			route(http.MethodPost, userGroups+"/{group_id}/resolve-add-members", "user_groups", "resolve_add_members", id("group_id")),
			route(http.MethodGet, "/api/v1/projects", "projects", "list"),
			route(http.MethodPost, "/api/v1/projects", "projects", "create"),
			route(http.MethodGet, "/api/v1/projects/by-user", "projects", "list_by_user"),
			route(http.MethodGet, projectID, "projects", "get", id("id")),
			route(http.MethodPut, projectID, "projects", "update", id("id"), admin()),
			route(http.MethodPatch, projectID, "projects", "update", id("id")),
			route(http.MethodDelete, projectID, "projects", "delete", id("id")),
			route(http.MethodDelete, "/api/v1/projects/batch", "projects", "batch_delete"),
			route(http.MethodGet, projectMembers, "project_members", "list", id("id")),
			route(http.MethodGet, projectID+"/add-members-context", "project_members", "add_members_context", id("id")),
			route(http.MethodPost, projectMembers, "project_members", "create", id("id")),
			route(http.MethodDelete, projectMembers, "project_members", "delete", id("id")),
			route(http.MethodPut, projectMembers+"/roles", "project_members", "batch_role_update", id("id")),
			route(http.MethodPut, projectMembers+"/quotas", "member_quotas", "batch_update", id("id")),
			route(http.MethodPut, projectMembers+"/{userId}", "project_members", "update_role", id("userId")),
			route(http.MethodDelete, projectMembers+"/{userId}", "project_members", "delete", id("userId")),
			route(http.MethodPatch, projectMembers+"/{userId}/role", "project_members", "update_role", id("userId")),
			route(http.MethodPatch, projectMembers+"/batch/roles", "project_members", "batch_role_update", id("id")),
			route(http.MethodDelete, projectMembers+"/batch", "project_members", "batch_delete", id("id")),
			route(http.MethodGet, projectMemberQuota, "member_quotas", "get", id("userId")),
			route(http.MethodPut, projectMemberQuota, "member_quotas", "update", id("userId")),
			route(http.MethodDelete, projectMemberQuota, "member_quotas", "delete", id("userId")),
			route(http.MethodGet, projectID+"/workspace-settings", "workspace_settings", "get", id("id")),
			route(http.MethodPut, projectID+"/workspace-settings", "workspace_settings", "update", id("id")),
			route(http.MethodGet, projectID+"/gpu-claims", "gpu_claims", "list", id("id")),
			route(http.MethodPost, projectID+"/gpu-claims", "gpu_claims", "create", id("id")),
			route(http.MethodDelete, projectID+"/gpu-claims/{requestId}", "gpu_claims", "delete", id("requestId")),
			route(http.MethodPut, "/internal/org-project/projects/{project_id}/plan", "projects", "bind_plan", id("project_id"), serviceInternal()),
			route(http.MethodDelete, "/internal/org-project/plans/{plan_id}/project-bindings", "projects", "clear_plan_bindings", serviceInternal()),
		},
	}
}
