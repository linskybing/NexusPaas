package orgproject

import (
	"context"
	"strings"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

type orgProjectRecord struct {
	ID   string
	Data map[string]any
}

type orgProjectMemberRecord struct {
	ID   string
	Data map[string]any
}

type orgProjectQuotaRecord struct {
	ID   string
	Data map[string]any
}

type orgProjectDeleteResult struct {
	ProjectMembers int
	UserQuotas     int
	GPUClaims      int
}

type orgProjectPlanUpdate struct {
	Old orgProjectRecord
	New orgProjectRecord
}

type recordStoreOrgProjectRepository struct {
	store    platform.RecordStore
	groupGPU *recordStoreOrgProjectGroupGPURepository
}

func projectRepository(app *platform.App) *recordStoreOrgProjectRepository {
	if app == nil {
		return &recordStoreOrgProjectRepository{}
	}
	return &recordStoreOrgProjectRepository{store: app.Store, groupGPU: groupGPURepository(app)}
}

func (r recordStoreOrgProjectRepository) ListProjects(ctx context.Context) []orgProjectRecord {
	records := r.store.List(ctx, projectsResource)
	out := make([]orgProjectRecord, 0, len(records))
	for _, record := range records {
		out = append(out, projectRecordFromStore(record))
	}
	return out
}

func (r recordStoreOrgProjectRepository) FindProject(ctx context.Context, id string) (orgProjectRecord, bool) {
	id = strings.TrimSpace(id)
	if id == "" {
		return orgProjectRecord{}, false
	}
	if record, found := r.store.Get(ctx, projectsResource, id); found {
		return projectRecordFromStore(record), true
	}
	for _, project := range r.ListProjects(ctx) {
		if projectID(project.Data) == id {
			return project, true
		}
	}
	return orgProjectRecord{}, false
}

func (r recordStoreOrgProjectRepository) CreateProject(ctx context.Context, project map[string]any) (orgProjectRecord, error) {
	record, err := r.store.Create(ctx, projectsResource, shared.CloneMap(project))
	if err != nil {
		return orgProjectRecord{}, err
	}
	return projectRecordFromStore(record), nil
}

func (r recordStoreOrgProjectRepository) UpdateProject(ctx context.Context, id string, update map[string]any) (orgProjectRecord, orgProjectRecord, bool) {
	old, found := r.store.Get(ctx, projectsResource, id)
	if !found {
		return orgProjectRecord{}, orgProjectRecord{}, false
	}
	updated, ok := r.store.Update(ctx, projectsResource, id, shared.CloneMap(update))
	if !ok {
		return orgProjectRecord{}, orgProjectRecord{}, false
	}
	return projectRecordFromStore(old), projectRecordFromStore(updated), true
}

func (r recordStoreOrgProjectRepository) UpdateWorkspaceSettings(ctx context.Context, projectID string, seconds int, now time.Time) (orgProjectRecord, orgProjectRecord, bool) {
	return r.UpdateProject(ctx, projectID, map[string]any{
		"max_ide_runtime_seconds": seconds,
		"MaxIDERuntimeSeconds":    seconds,
		"updated_at":              now,
	})
}

func (r recordStoreOrgProjectRepository) DeleteProjectCascade(ctx context.Context, projectID string) (orgProjectRecord, orgProjectDeleteResult, bool) {
	project, found := r.FindProject(ctx, projectID)
	if !found {
		return orgProjectRecord{}, orgProjectDeleteResult{}, false
	}
	r.store.Delete(ctx, projectsResource, project.ID)
	result := orgProjectDeleteResult{}
	for _, resource := range []struct {
		name  string
		count *int
	}{
		{name: projectMembersResource, count: &result.ProjectMembers},
		{name: projectUserQuotasResource, count: &result.UserQuotas},
	} {
		for _, record := range r.store.List(ctx, resource.name) {
			if shared.TextValue(record.Data, "project_id", "projectId") != project.ID &&
				shared.TextValue(record.Data, "project_id", "projectId") != projectID {
				continue
			}
			if r.store.Delete(ctx, resource.name, record.ID) {
				*resource.count = *resource.count + 1
			}
		}
	}
	if r.groupGPU != nil {
		result.GPUClaims = r.groupGPU.DeleteGPUClaimsByProject(ctx, project.ID)
	}
	return project, result, true
}

func (r recordStoreOrgProjectRepository) NextProjectID() string {
	return r.store.NextID(projectsResource, "P", 1, 8)
}

func (r recordStoreOrgProjectRepository) FindDirectProjectMember(ctx context.Context, projectID, userID string) (orgProjectMemberRecord, bool) {
	for _, id := range compositeProjectUserKeys(projectID, userID) {
		if record, found := r.store.Get(ctx, projectMembersResource, id); found {
			return memberRecordFromStore(record), true
		}
	}
	for _, record := range r.store.List(ctx, projectMembersResource) {
		if shared.TextValue(record.Data, "project_id", "projectId") == projectID &&
			shared.TextValue(record.Data, "user_id", "userId") == userID {
			return memberRecordFromStore(record), true
		}
	}
	return orgProjectMemberRecord{}, false
}

func (r recordStoreOrgProjectRepository) CreateDirectProjectMember(ctx context.Context, member map[string]any) (orgProjectMemberRecord, error) {
	record, err := r.store.Create(ctx, projectMembersResource, shared.CloneMap(member))
	if err != nil {
		return orgProjectMemberRecord{}, err
	}
	return memberRecordFromStore(record), nil
}

func (r recordStoreOrgProjectRepository) UpdateDirectProjectMemberRole(ctx context.Context, projectID, userID, role string, now time.Time) (orgProjectMemberRecord, orgProjectMemberRecord, bool) {
	old, found := r.FindDirectProjectMember(ctx, projectID, userID)
	if !found {
		return orgProjectMemberRecord{}, orgProjectMemberRecord{}, false
	}
	updated, ok := r.store.Update(ctx, projectMembersResource, old.ID, map[string]any{"role": role, "updated_at": now})
	if !ok {
		return orgProjectMemberRecord{}, orgProjectMemberRecord{}, false
	}
	return old, memberRecordFromStore(updated), true
}

func (r recordStoreOrgProjectRepository) DeleteDirectProjectMemberAndQuota(ctx context.Context, projectID, userID string) (orgProjectMemberRecord, bool) {
	member, found := r.FindDirectProjectMember(ctx, projectID, userID)
	if !found {
		return orgProjectMemberRecord{}, false
	}
	r.store.Delete(ctx, projectMembersResource, member.ID)
	r.DeleteProjectUserQuota(ctx, projectID, userID)
	return member, true
}

func (r recordStoreOrgProjectRepository) GetProjectUserQuota(ctx context.Context, projectID, userID string) (orgProjectQuotaRecord, bool) {
	for _, id := range compositeProjectUserKeys(projectID, userID) {
		if record, found := r.store.Get(ctx, projectUserQuotasResource, id); found {
			return quotaRecordFromStore(record), true
		}
	}
	for _, record := range r.store.List(ctx, projectUserQuotasResource) {
		if shared.TextValue(record.Data, "project_id", "projectId") == projectID &&
			shared.TextValue(record.Data, "user_id", "userId") == userID {
			return quotaRecordFromStore(record), true
		}
	}
	return orgProjectQuotaRecord{}, false
}

func (r recordStoreOrgProjectRepository) UpsertProjectUserQuota(ctx context.Context, quota map[string]any) (orgProjectQuotaRecord, error) {
	id := shared.TextValue(quota, "id")
	if updated, ok := r.store.Update(ctx, projectUserQuotasResource, id, shared.CloneMap(quota)); ok {
		return quotaRecordFromStore(updated), nil
	}
	record, err := r.store.Create(ctx, projectUserQuotasResource, shared.CloneMap(quota))
	if err != nil {
		return orgProjectQuotaRecord{}, err
	}
	return quotaRecordFromStore(record), nil
}

func (r recordStoreOrgProjectRepository) DeleteProjectUserQuota(ctx context.Context, projectID, userID string) bool {
	quota, found := r.GetProjectUserQuota(ctx, projectID, userID)
	if !found {
		return false
	}
	return r.store.Delete(ctx, projectUserQuotasResource, quota.ID)
}

func (r recordStoreOrgProjectRepository) BindProjectPlan(ctx context.Context, projectID, planID string, now time.Time) (orgProjectRecord, orgProjectRecord, bool) {
	return r.UpdateProject(ctx, projectID, map[string]any{
		"plan_id":          planID,
		"resource_plan_id": planID,
		"updated_at":       now,
	})
}

func (r recordStoreOrgProjectRepository) ClearProjectsPlan(ctx context.Context, planID string, now time.Time) []orgProjectPlanUpdate {
	var updates []orgProjectPlanUpdate
	for _, project := range r.ListProjects(ctx) {
		if shared.TextValue(project.Data, "plan_id", "planId", "resource_plan_id", "resourcePlanId") != planID {
			continue
		}
		old, updated, ok := r.UpdateProject(ctx, project.ID, map[string]any{
			"plan_id":          "",
			"resource_plan_id": "",
			"updated_at":       now,
		})
		if !ok {
			continue
		}
		updates = append(updates, orgProjectPlanUpdate{Old: old, New: updated})
	}
	return updates
}

func projectRecordFromStore(record contracts.Record[map[string]any]) orgProjectRecord {
	return orgProjectRecord{ID: record.ID, Data: normalizeProjectRecord(record.Data, record.ID)}
}

func memberRecordFromStore(record contracts.Record[map[string]any]) orgProjectMemberRecord {
	return orgProjectMemberRecord{ID: record.ID, Data: recordDataWithID(record)}
}

func quotaRecordFromStore(record contracts.Record[map[string]any]) orgProjectQuotaRecord {
	return orgProjectQuotaRecord{ID: record.ID, Data: recordDataWithID(record)}
}

func recordDataWithID(record contracts.Record[map[string]any]) map[string]any {
	data := shared.CloneMap(record.Data)
	if data["id"] == nil {
		data["id"] = record.ID
	}
	return data
}

func compositeProjectUserKeys(projectID, userID string) []string {
	return []string{projectMemberID(projectID, userID), projectID + "/" + userID}
}
