package ideworkspace

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

var errIDEProjectionRepositoryUnavailable = errors.New("ide projection repository unavailable")

type recordStoreIDEProjectionRepository struct {
	store  platform.RecordStore
	config platform.Config
}

func ideProjectionRepo(app *platform.App) *recordStoreIDEProjectionRepository {
	if app == nil {
		return &recordStoreIDEProjectionRepository{}
	}
	return &recordStoreIDEProjectionRepository{store: app.Store, config: app.Config}
}

func ideProjectionRepoFromStore(store platform.RecordStore, config platform.Config) *recordStoreIDEProjectionRepository {
	return &recordStoreIDEProjectionRepository{store: store, config: config}
}

func (r recordStoreIDEProjectionRepository) UpsertIdentityUser(ctx context.Context, data map[string]any) error {
	return r.upsertReadModel(ctx, ideIdentityUsersResource, data)
}

func (r recordStoreIDEProjectionRepository) UpsertIdentityRole(ctx context.Context, data map[string]any) error {
	return r.upsertReadModel(ctx, ideIdentityRolesResource, data)
}

func (r recordStoreIDEProjectionRepository) UpsertPolicyRole(ctx context.Context, data map[string]any) error {
	return r.upsertReadModel(ctx, idePolicyRolesResource, data)
}

func (r recordStoreIDEProjectionRepository) UpsertProject(ctx context.Context, data map[string]any) error {
	return r.upsertReadModel(ctx, ideProjectsResource, data)
}

func (r recordStoreIDEProjectionRepository) UpsertProjectMember(ctx context.Context, data map[string]any) error {
	return r.upsertReadModel(ctx, ideProjectMembersResource, data)
}

func (r recordStoreIDEProjectionRepository) UpsertUserGroup(ctx context.Context, data map[string]any) error {
	return r.upsertReadModel(ctx, ideUserGroupsResource, data)
}

func (r recordStoreIDEProjectionRepository) DeleteIdentityUser(ctx context.Context, data map[string]any) bool {
	return r.deleteReadModel(ctx, ideIdentityUsersResource, data)
}

func (r recordStoreIDEProjectionRepository) DeleteIdentityRole(ctx context.Context, data map[string]any) bool {
	return r.deleteReadModel(ctx, ideIdentityRolesResource, data)
}

func (r recordStoreIDEProjectionRepository) DeletePolicyRole(ctx context.Context, data map[string]any) bool {
	return r.deleteReadModel(ctx, idePolicyRolesResource, data)
}

func (r recordStoreIDEProjectionRepository) DeleteProject(ctx context.Context, data map[string]any) bool {
	return r.deleteReadModel(ctx, ideProjectsResource, data)
}

func (r recordStoreIDEProjectionRepository) DeleteProjectMember(ctx context.Context, data map[string]any) bool {
	return r.deleteReadModel(ctx, ideProjectMembersResource, data)
}

func (r recordStoreIDEProjectionRepository) DeleteUserGroup(ctx context.Context, data map[string]any) bool {
	return r.deleteReadModel(ctx, ideUserGroupsResource, data)
}

func (r recordStoreIDEProjectionRepository) ListIdentityUsers(ctx context.Context) []contracts.Record[map[string]any] {
	return r.readModelRecords(ctx, ideIdentityUsersResource, identityUsersResource)
}

func (r recordStoreIDEProjectionRepository) ListIdentityRoles(ctx context.Context) []contracts.Record[map[string]any] {
	return r.readModelRecords(ctx, ideIdentityRolesResource, identityRolesResource)
}

func (r recordStoreIDEProjectionRepository) ListPolicyRoles(ctx context.Context) []contracts.Record[map[string]any] {
	return r.readModelRecords(ctx, idePolicyRolesResource, authorizationRolesResource)
}

func (r recordStoreIDEProjectionRepository) ListProjects(ctx context.Context) []contracts.Record[map[string]any] {
	return r.readModelRecords(ctx, ideProjectsResource, orgProjectsResource)
}

func (r recordStoreIDEProjectionRepository) ListProjectMembers(ctx context.Context) []contracts.Record[map[string]any] {
	return r.readModelRecords(ctx, ideProjectMembersResource, orgProjectMembersResource)
}

func (r recordStoreIDEProjectionRepository) ListUserGroups(ctx context.Context) []contracts.Record[map[string]any] {
	return r.readModelRecords(ctx, ideUserGroupsResource, orgUserGroupsResource)
}

func (r recordStoreIDEProjectionRepository) upsertReadModel(ctx context.Context, resource string, data map[string]any) error {
	id := ideReadModelID(resource, data)
	if id == "" {
		return nil
	}
	if r.store == nil {
		return errIDEProjectionRepositoryUnavailable
	}
	next := shared.CloneMap(data)
	next[ideKeyID] = id
	if _, ok := r.store.Update(ctx, resource, id, next); ok {
		return nil
	}
	if _, err := r.store.Create(ctx, resource, next); err != nil {
		if platform.IsCreateConflict(err) {
			if _, ok := r.store.Update(ctx, resource, id, next); !ok {
				return fmt.Errorf("ide projection conflict update missed for %s/%s", resource, id)
			}
			return nil
		}
		return fmt.Errorf("ide projection create failed for %s/%s: %w", resource, id, err)
	}
	return nil
}

func (r recordStoreIDEProjectionRepository) deleteReadModel(ctx context.Context, resource string, data map[string]any) bool {
	if deleted, ok := data[ideKeyDeleted].(bool); ok && !deleted {
		return false
	}
	id := ideReadModelID(resource, data)
	if r.store == nil || id == "" {
		return false
	}
	return r.store.Delete(ctx, resource, id)
}

func (r recordStoreIDEProjectionRepository) readModelRecords(ctx context.Context, localResource, sourceResource string) []contracts.Record[map[string]any] {
	local := r.listRecords(ctx, localResource)
	if sourceResource == "" || !r.sourceCoHosted(sourceResource) {
		return local
	}
	source := r.listRecords(ctx, sourceResource)
	if len(local) == 0 {
		return source
	}
	return mergeIDERecords(localResource, source, local)
}

func (r recordStoreIDEProjectionRepository) listRecords(ctx context.Context, resource string) []contracts.Record[map[string]any] {
	if r.store == nil {
		return nil
	}
	records := r.store.List(ctx, resource)
	out := make([]contracts.Record[map[string]any], 0, len(records))
	for _, record := range records {
		out = append(out, cloneIDERecord(record))
	}
	return out
}

func (r recordStoreIDEProjectionRepository) sourceCoHosted(sourceResource string) bool {
	owner, _, ok := strings.Cut(sourceResource, ":")
	return ok && r.config.AllowsService(owner)
}

func ideReadModelID(resource string, data map[string]any) string {
	id := textValue(data, ideKeyID, "ID")
	groupID := textValue(data, ideKeyGroupID, ideKeyGroupIDC, "GroupID")
	name := textValue(data, ideKeyName, "Name")
	projectID := textValue(data, ideKeyProjectID, ideKeyProjectC, "ProjectID", "p_id", "pID", "PID")
	roleID := textValue(data, ideKeyRoleID, ideKeyRoleIDC, "RoleID")
	userID := textValue(data, ideKeyUserID, ideKeyUserIDC, "UserID")
	switch resource {
	case ideIdentityUsersResource:
		return shared.FirstNonBlank(id, userID)
	case ideIdentityRolesResource, idePolicyRolesResource:
		return shared.FirstNonBlank(id, roleID, name, userID)
	case ideProjectMembersResource:
		if id == "" && projectID != "" && userID != "" {
			return projectID + ":" + userID
		}
	case ideProjectsResource:
		return shared.FirstNonBlank(id, projectID)
	case ideUserGroupsResource:
		if id == "" && userID != "" && groupID != "" {
			return userID + ":" + groupID
		}
	}
	return shared.FirstNonBlank(id, projectID, userID, groupID, roleID, name)
}

func mergeIDERecords(resource string, source, local []contracts.Record[map[string]any]) []contracts.Record[map[string]any] {
	out := make([]contracts.Record[map[string]any], 0, len(source)+len(local))
	seen := map[string]bool{}
	for _, record := range local {
		if id := ideReadModelID(resource, record.Data); id != "" {
			seen[id] = true
		}
		out = append(out, cloneIDERecord(record))
	}
	for _, record := range source {
		id := ideReadModelID(resource, record.Data)
		if id != "" && seen[id] {
			continue
		}
		out = append(out, cloneIDERecord(record))
	}
	return out
}

func cloneIDERecord(record contracts.Record[map[string]any]) contracts.Record[map[string]any] {
	record.Data = shared.CloneMap(record.Data)
	return record
}
