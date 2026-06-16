package requestnotification

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

var errProjectAccessRepositoryUnavailable = errors.New("request notification project access repository unavailable")

type projectAccessRepository interface {
	UpsertProject(context.Context, map[string]any) error
	UpsertProjectMember(context.Context, map[string]any) error
	UpsertUserGroup(context.Context, map[string]any) error
	DeleteProject(context.Context, map[string]any) bool
	DeleteProjectMember(context.Context, map[string]any) bool
	DeleteUserGroup(context.Context, map[string]any) bool
	ListProjects(context.Context) []map[string]any
	ListProjectMembers(context.Context) []map[string]any
	ListUserGroups(context.Context) []map[string]any
}

type recordStoreProjectAccessRepository struct {
	store  platform.RecordStore
	config platform.Config
}

func projectAccessRepo(app *platform.App) projectAccessRepository {
	if app == nil {
		return recordStoreProjectAccessRepository{}
	}
	return recordStoreProjectAccessRepository{store: app.Store, config: app.Config}
}

func projectAccessRepoFromStore(store platform.RecordStore, config platform.Config) projectAccessRepository {
	return recordStoreProjectAccessRepository{store: store, config: config}
}

func (r recordStoreProjectAccessRepository) UpsertProject(ctx context.Context, data map[string]any) error {
	return r.upsertReadModel(ctx, projectAccessProjects, data)
}

func (r recordStoreProjectAccessRepository) UpsertProjectMember(ctx context.Context, data map[string]any) error {
	return r.upsertReadModel(ctx, projectAccessMembers, data)
}

func (r recordStoreProjectAccessRepository) UpsertUserGroup(ctx context.Context, data map[string]any) error {
	return r.upsertReadModel(ctx, projectAccessUserGroups, data)
}

func (r recordStoreProjectAccessRepository) DeleteProject(ctx context.Context, data map[string]any) bool {
	return r.deleteReadModel(ctx, projectAccessProjects, data)
}

func (r recordStoreProjectAccessRepository) DeleteProjectMember(ctx context.Context, data map[string]any) bool {
	return r.deleteReadModel(ctx, projectAccessMembers, data)
}

func (r recordStoreProjectAccessRepository) DeleteUserGroup(ctx context.Context, data map[string]any) bool {
	return r.deleteReadModel(ctx, projectAccessUserGroups, data)
}

func (r recordStoreProjectAccessRepository) ListProjects(ctx context.Context) []map[string]any {
	return r.readModelRows(ctx, projectAccessProjects, orgProjectsResource)
}

func (r recordStoreProjectAccessRepository) ListProjectMembers(ctx context.Context) []map[string]any {
	return r.readModelRows(ctx, projectAccessMembers, orgProjectMembersResource)
}

func (r recordStoreProjectAccessRepository) ListUserGroups(ctx context.Context) []map[string]any {
	return r.readModelRows(ctx, projectAccessUserGroups, orgUserGroupsResource)
}

func (r recordStoreProjectAccessRepository) upsertReadModel(ctx context.Context, resource string, data map[string]any) error {
	id := projectAccessReadModelID(resource, data)
	if id == "" {
		return nil
	}
	if r.store == nil {
		return errProjectAccessRepositoryUnavailable
	}
	next := shared.CloneMap(data)
	next["id"] = id
	if _, ok := r.store.Update(ctx, resource, id, next); ok {
		return nil
	}
	if _, err := r.store.Create(ctx, resource, next); err != nil {
		if platform.IsCreateConflict(err) {
			if _, ok := r.store.Update(ctx, resource, id, next); !ok {
				return fmt.Errorf("request notification projection conflict update missed for %s/%s", resource, id)
			}
			return nil
		}
		return fmt.Errorf("request notification projection create failed for %s/%s: %w", resource, id, err)
	}
	return nil
}

func (r recordStoreProjectAccessRepository) deleteReadModel(ctx context.Context, resource string, data map[string]any) bool {
	if deleted, ok := data["deleted"].(bool); ok && !deleted {
		return false
	}
	id := projectAccessReadModelID(resource, data)
	if r.store == nil || id == "" {
		return false
	}
	return r.store.Delete(ctx, resource, id)
}

func (r recordStoreProjectAccessRepository) readModelRows(ctx context.Context, localResource, sourceResource string) []map[string]any {
	local := r.recordMaps(ctx, localResource)
	if sourceResource == "" || !r.sourceCoHosted(sourceResource) {
		return local
	}
	source := r.recordMaps(ctx, sourceResource)
	if len(local) == 0 {
		return source
	}
	return mergeProjectAccessRecords(localResource, source, local)
}

func (r recordStoreProjectAccessRepository) recordMaps(ctx context.Context, resource string) []map[string]any {
	if r.store == nil {
		return nil
	}
	records := r.store.List(ctx, resource)
	out := make([]map[string]any, 0, len(records))
	for _, record := range records {
		out = append(out, shared.CloneMap(record.Data))
	}
	return out
}

func (r recordStoreProjectAccessRepository) sourceCoHosted(sourceResource string) bool {
	owner, _, ok := strings.Cut(sourceResource, ":")
	return ok && r.config.AllowsService(owner)
}

func projectAccessReadModelID(resource string, data map[string]any) string {
	id := valueFrom(data, "id")
	projectID := valueFrom(data, "project_id", "projectId")
	userID := valueFrom(data, "user_id", "userId")
	groupID := valueFrom(data, "group_id", "groupId")
	if resource == projectAccessMembers && id == "" && projectID != "" && userID != "" {
		return projectID + ":" + userID
	}
	if resource == projectAccessUserGroups && id == "" && userID != "" && groupID != "" {
		return userID + ":" + groupID
	}
	return shared.FirstNonEmpty(id, projectID, userID)
}

func mergeProjectAccessRecords(resource string, source, local []map[string]any) []map[string]any {
	out := make([]map[string]any, 0, len(source)+len(local))
	seen := map[string]bool{}
	for _, record := range local {
		if id := projectAccessReadModelID(resource, record); id != "" {
			seen[id] = true
		}
		out = append(out, shared.CloneMap(record))
	}
	for _, record := range source {
		id := projectAccessReadModelID(resource, record)
		if id != "" && seen[id] {
			continue
		}
		out = append(out, shared.CloneMap(record))
	}
	return out
}
