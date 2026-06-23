package ideworkspace

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
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

type ideProjectionDriftReport struct {
	Missing []ideProjectionDriftFinding
	Orphan  []ideProjectionDriftFinding
	Stale   []ideProjectionDriftFinding
}

type ideProjectionDriftFinding struct {
	SourceResource string
	LocalResource  string
	ID             string
}

type ideProjectionDriftPair struct {
	sourceResource string
	localResource  string
	idFn           func(map[string]any) string
}

var ideProjectionDriftPairs = []ideProjectionDriftPair{
	{sourceResource: identityUsersResource, localResource: ideIdentityUsersResource, idFn: func(row map[string]any) string {
		return ideReadModelID(ideIdentityUsersResource, row)
	}},
	{sourceResource: identityRolesResource, localResource: ideIdentityRolesResource, idFn: func(row map[string]any) string {
		return ideReadModelID(ideIdentityRolesResource, row)
	}},
	{sourceResource: authorizationRolesResource, localResource: idePolicyRolesResource, idFn: func(row map[string]any) string {
		return ideReadModelID(idePolicyRolesResource, row)
	}},
	{sourceResource: orgProjectsResource, localResource: ideProjectsResource, idFn: func(row map[string]any) string {
		return ideReadModelID(ideProjectsResource, row)
	}},
	{sourceResource: orgProjectMembersResource, localResource: ideProjectMembersResource, idFn: func(row map[string]any) string {
		return ideReadModelID(ideProjectMembersResource, row)
	}},
	{sourceResource: orgUserGroupsResource, localResource: ideUserGroupsResource, idFn: func(row map[string]any) string {
		return ideReadModelID(ideUserGroupsResource, row)
	}},
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

func (r recordStoreIDEProjectionRepository) projectionDrift(ctx context.Context) (ideProjectionDriftReport, error) {
	var report ideProjectionDriftReport
	if r.store == nil {
		return report, errIDEProjectionRepositoryUnavailable
	}
	for _, pair := range ideProjectionDriftPairs {
		sourceRows := ideProjectionDriftIndex(r.listRecords(ctx, pair.sourceResource), pair.idFn)
		localRows := ideProjectionDriftIndex(r.listRecords(ctx, pair.localResource), pair.idFn)
		for id, sourceRow := range sourceRows {
			localRow, ok := localRows[id]
			finding := ideProjectionDriftFinding{
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
			report.Orphan = append(report.Orphan, ideProjectionDriftFinding{
				SourceResource: pair.sourceResource,
				LocalResource:  pair.localResource,
				ID:             id,
			})
		}
	}
	report.sort()
	return report, nil
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

func ideProjectionDriftIndex(records []contracts.Record[map[string]any], idFn func(map[string]any) string) map[string]map[string]any {
	out := map[string]map[string]any{}
	for _, record := range records {
		id, normalized := ideProjectionDriftNormalize(record.Data, idFn)
		if id == "" {
			continue
		}
		out[id] = normalized
	}
	return out
}

func ideProjectionDriftNormalize(row map[string]any, idFn func(map[string]any) string) (string, map[string]any) {
	normalized := shared.CloneMap(row)
	id := idFn(normalized)
	if id == "" {
		return "", nil
	}
	normalized[ideKeyID] = id
	return id, normalized
}

func (r *ideProjectionDriftReport) sort() {
	sortIDEProjectionDriftFindings(r.Missing)
	sortIDEProjectionDriftFindings(r.Orphan)
	sortIDEProjectionDriftFindings(r.Stale)
}

func sortIDEProjectionDriftFindings(findings []ideProjectionDriftFinding) {
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
