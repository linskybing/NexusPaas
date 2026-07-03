package ideworkspace

import (
	"context"
	"net/http"
	"strings"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

const (
	ideProjectionConsumer = serviceName + ":ide_projection"

	ideKeyAction    = "action"
	ideKeyDeleted   = "deleted"
	ideKeyGroupID   = "group_id"
	ideKeyGroupIDC  = "groupId"
	ideKeyID        = "id"
	ideKeyName      = "name"
	ideKeyProjectID = "project_id"
	ideKeyProjectC  = "projectId"
	ideKeyRoleID    = "role_id"
	ideKeyRoleIDC   = "roleId"
	ideKeyUserID    = "user_id"
	ideKeyUserIDC   = "userId"
)

func ideRecords(app *platform.App, r *http.Request, localResource, _ string) []contracts.Record[map[string]any] {
	syncIDEReadModels(app, r)
	repo := ideProjectionRepo(app)
	switch localResource {
	case ideIdentityUsersResource:
		return repo.ListIdentityUsers(r.Context())
	case ideIdentityRolesResource:
		return repo.ListIdentityRoles(r.Context())
	case idePolicyRolesResource:
		return repo.ListPolicyRoles(r.Context())
	case ideProjectsResource:
		return repo.ListProjects(r.Context())
	case ideProjectMembersResource:
		return repo.ListProjectMembers(r.Context())
	case ideUserGroupsResource:
		return repo.ListUserGroups(r.Context())
	default:
		return nil
	}
}

func syncIDEReadModels(app *platform.App, r *http.Request) {
	if app == nil || app.Store == nil || app.Events == nil {
		return
	}
	app.RunProjection(r.Context(), ideProjectionConsumer, func(event contracts.Event) error {
		return projectIDEEvent(app, r, event)
	})
}

func projectIDEEvent(app *platform.App, r *http.Request, event contracts.Event) error {
	resource, data, deleted, ok := ideProjection(event)
	if !ok {
		return nil
	}
	repo := ideProjectionRepo(app)
	if deleted {
		deleteIDEReadModel(repo, r, resource, data)
		return nil
	}
	return upsertIDEReadModel(repo, r, resource, data)
}

func ideProjection(event contracts.Event) (string, map[string]any, bool, bool) {
	switch strings.ToLower(event.Name) {
	case "usercreated", "userupdated", "userdisabled":
		return ideIdentityUsersResource, ideEventData(event), false, true
	case "userdeleted":
		return ideIdentityUsersResource, ideEventData(event), true, true
	case "rolecreated", "roleupdated":
		return ideIdentityRolesResource, ideEventData(event), false, true
	case "roledeleted":
		return ideIdentityRolesResource, ideEventData(event), true, true
	case "projectcreated", "projectupdated":
		return ideProjectsResource, ideEventData(event), false, true
	case "projectdeleted":
		return ideProjectsResource, ideEventData(event), true, true
	case "project_membercreated", "project_memberupdated":
		return ideProjectMembersResource, ideEventData(event), false, true
	case "project_memberdeleted":
		return ideProjectMembersResource, ideEventData(event), true, true
	case "groupmembershipchanged":
		data, deleted := ideGroupMembershipData(event)
		return ideUserGroupsResource, data, deleted, true
	case "proxypolicychanged":
		return idePolicyProjection(event)
	default:
		return "", nil, false, false
	}
}

func idePolicyProjection(event contracts.Event) (string, map[string]any, bool, bool) {
	data := ideEventData(event)
	switch strings.ToLower(textValue(data, ideKeyAction)) {
	case "role_create", "role_update":
		return idePolicyRolesResource, data, false, true
	case "role_delete":
		return idePolicyRolesResource, data, true, true
	default:
		return "", nil, false, false
	}
}

func ideGroupMembershipData(event contracts.Event) (map[string]any, bool) {
	data := ideEventData(event)
	action := strings.ToLower(textValue(data, ideKeyAction))
	return data, action == "delete" || action == ideKeyDeleted
}

func ideEventData(event contracts.Event) map[string]any {
	for _, key := range []string{"new", "record", "project", "member", "user", "role"} {
		if data, ok := event.Data[key].(map[string]any); ok {
			return shared.CloneMap(data)
		}
	}
	return shared.CloneMap(event.Data)
}

func upsertIDEReadModel(repo *recordStoreIDEProjectionRepository, r *http.Request, resource string, data map[string]any) error {
	switch resource {
	case ideIdentityUsersResource:
		return repo.UpsertIdentityUser(r.Context(), data)
	case ideIdentityRolesResource:
		return repo.UpsertIdentityRole(r.Context(), data)
	case idePolicyRolesResource:
		return repo.UpsertPolicyRole(r.Context(), data)
	case ideProjectsResource:
		return repo.UpsertProject(r.Context(), data)
	case ideProjectMembersResource:
		return repo.UpsertProjectMember(r.Context(), data)
	case ideUserGroupsResource:
		return repo.UpsertUserGroup(r.Context(), data)
	default:
		return nil
	}
}

func deleteIDEReadModel(repo *recordStoreIDEProjectionRepository, r *http.Request, resource string, data map[string]any) bool {
	switch resource {
	case ideIdentityUsersResource:
		return repo.DeleteIdentityUser(r.Context(), data)
	case ideIdentityRolesResource:
		return repo.DeleteIdentityRole(r.Context(), data)
	case idePolicyRolesResource:
		return repo.DeletePolicyRole(r.Context(), data)
	case ideProjectsResource:
		return repo.DeleteProject(r.Context(), data)
	case ideProjectMembersResource:
		return repo.DeleteProjectMember(r.Context(), data)
	case ideUserGroupsResource:
		return repo.DeleteUserGroup(r.Context(), data)
	default:
		return false
	}
}

// registerIDEProjectionReconciler wires the IDE read models into the periodic
// drift→replay reconcile job (DATA-016/DATA-018).
func registerIDEProjectionReconciler(app *platform.App) {
	app.RegisterProjectionReconciler(platform.ProjectionReconcilerSpec{
		Owner:     serviceName,
		Consumers: []string{ideProjectionConsumer},
		Drift: func(ctx context.Context) (int, error) {
			report, err := ideProjectionRepo(app).projectionDrift(ctx)
			if err != nil {
				return 0, err
			}
			return len(report.Missing) + len(report.Orphan) + len(report.Stale), nil
		},
		Sync: func(ctx context.Context) { syncIDEReadModels(app, shared.MaintenanceRequest(ctx)) },
	})
}
