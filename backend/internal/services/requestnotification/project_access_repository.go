package requestnotification

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

var errProjectAccessRepositoryUnavailable = errors.New("request notification project access repository unavailable")

type recordStoreProjectAccessRepository struct {
	store  platform.RecordStore
	config platform.Config
}

type projectAccessProjectionDriftReport struct {
	Missing []projectAccessProjectionDriftFinding
	Orphan  []projectAccessProjectionDriftFinding
	Stale   []projectAccessProjectionDriftFinding
}

type projectAccessProjectionDriftFinding struct {
	SourceResource string
	LocalResource  string
	ID             string
}

type projectAccessProjectionDriftPair struct {
	sourceResource string
	localResource  string
	idFn           func(map[string]any) string
}

var projectAccessProjectionDriftPairs = []projectAccessProjectionDriftPair{
	{
		sourceResource: orgProjectsResource,
		localResource:  projectAccessProjects,
		idFn: func(row map[string]any) string {
			return projectAccessReadModelID(projectAccessProjects, row)
		},
	},
	{
		sourceResource: orgProjectMembersResource,
		localResource:  projectAccessMembers,
		idFn: func(row map[string]any) string {
			return projectAccessReadModelID(projectAccessMembers, row)
		},
	},
	{
		sourceResource: orgUserGroupsResource,
		localResource:  projectAccessUserGroups,
		idFn: func(row map[string]any) string {
			return projectAccessReadModelID(projectAccessUserGroups, row)
		},
	},
}

func projectAccessRepo(app *platform.App) *recordStoreProjectAccessRepository {
	if app == nil {
		return &recordStoreProjectAccessRepository{}
	}
	return &recordStoreProjectAccessRepository{store: app.Store, config: app.Config}
}

func projectAccessRepoFromStore(store platform.RecordStore, config platform.Config) *recordStoreProjectAccessRepository {
	return &recordStoreProjectAccessRepository{store: store, config: config}
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

func (r recordStoreProjectAccessRepository) projectionDrift(ctx context.Context) (projectAccessProjectionDriftReport, error) {
	var report projectAccessProjectionDriftReport
	if r.store == nil {
		return report, errProjectAccessRepositoryUnavailable
	}
	for _, pair := range projectAccessProjectionDriftPairs {
		sourceRows := projectAccessProjectionDriftIndex(r.recordMaps(ctx, pair.sourceResource), pair.idFn)
		localRows := projectAccessProjectionDriftIndex(r.recordMaps(ctx, pair.localResource), pair.idFn)
		for id, sourceRow := range sourceRows {
			localRow, ok := localRows[id]
			finding := projectAccessProjectionDriftFinding{
				SourceResource: pair.sourceResource,
				LocalResource:  pair.localResource,
				ID:             id,
			}
			if !ok {
				report.Missing = append(report.Missing, finding)
				continue
			}
			if !reflect.DeepEqual(sourceRow, localRow) {
				report.Stale = append(report.Stale, finding)
			}
		}
		for id := range localRows {
			if _, ok := sourceRows[id]; ok {
				continue
			}
			report.Orphan = append(report.Orphan, projectAccessProjectionDriftFinding{
				SourceResource: pair.sourceResource,
				LocalResource:  pair.localResource,
				ID:             id,
			})
		}
	}
	report.sort()
	return report, nil
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

func projectAccessProjectionDriftIndex(rows []map[string]any, idFn func(map[string]any) string) map[string]map[string]any {
	out := map[string]map[string]any{}
	for _, row := range rows {
		id, normalized := projectAccessProjectionDriftNormalize(row, idFn)
		if id == "" {
			continue
		}
		out[id] = normalized
	}
	return out
}

func projectAccessProjectionDriftNormalize(row map[string]any, idFn func(map[string]any) string) (string, map[string]any) {
	normalized := shared.CloneMap(row)
	id := idFn(normalized)
	if id == "" {
		return "", nil
	}
	normalized["id"] = id
	return id, normalized
}

func (r *projectAccessProjectionDriftReport) sort() {
	sortProjectAccessProjectionDriftFindings(r.Missing)
	sortProjectAccessProjectionDriftFindings(r.Orphan)
	sortProjectAccessProjectionDriftFindings(r.Stale)
}

func sortProjectAccessProjectionDriftFindings(findings []projectAccessProjectionDriftFinding) {
	sort.Slice(findings, func(i, j int) bool {
		if findings[i].LocalResource != findings[j].LocalResource {
			return findings[i].LocalResource < findings[j].LocalResource
		}
		if findings[i].ID != findings[j].ID {
			return findings[i].ID < findings[j].ID
		}
		return findings[i].SourceResource < findings[j].SourceResource
	})
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
