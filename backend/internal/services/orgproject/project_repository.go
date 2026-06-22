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

// CreateProjectWithEvent persists the project and its event in one transaction
// (when the store supports it), keeping the resource key owned by this repo.
func (r recordStoreOrgProjectRepository) CreateProjectWithEvent(ctx context.Context, app *platform.App, project map[string]any, build func(contracts.Record[map[string]any]) contracts.Event) (orgProjectRecord, error) {
	record, err := app.CreateRecordWithEvent(ctx, projectsResource, shared.CloneMap(project), build)
	if err != nil {
		return orgProjectRecord{}, err
	}
	return projectRecordFromStore(record), nil
}

// UpdateProjectWithEvent is the update counterpart; the builder receives old+new.
func (r recordStoreOrgProjectRepository) UpdateProjectWithEvent(ctx context.Context, app *platform.App, id string, update map[string]any, build func(old, updated orgProjectRecord) contracts.Event) (orgProjectRecord, orgProjectRecord, bool, error) {
	old, found := r.store.Get(ctx, projectsResource, id)
	if !found {
		return orgProjectRecord{}, orgProjectRecord{}, false, nil
	}
	oldRec := projectRecordFromStore(old)
	updated, ok, err := app.UpdateRecordWithEvent(ctx, projectsResource, id, shared.CloneMap(update), func(rec contracts.Record[map[string]any]) contracts.Event {
		return build(oldRec, projectRecordFromStore(rec))
	})
	if err != nil || !ok {
		return orgProjectRecord{}, orgProjectRecord{}, false, err
	}
	return oldRec, projectRecordFromStore(updated), true, nil
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

func (r recordStoreOrgProjectRepository) UpdateWorkspaceSettingsTx(ctx context.Context, tx platform.StoreTx, projectID string, seconds int, now time.Time) (orgProjectRecord, orgProjectRecord, bool, error) {
	old, found := r.store.Get(ctx, projectsResource, projectID)
	if !found {
		return orgProjectRecord{}, orgProjectRecord{}, false, nil
	}
	updated, ok, err := tx.Update(ctx, projectsResource, projectID, map[string]any{
		"max_ide_runtime_seconds": seconds,
		"MaxIDERuntimeSeconds":    seconds,
		"updated_at":              now,
	})
	if err != nil || !ok {
		return orgProjectRecord{}, orgProjectRecord{}, false, err
	}
	return projectRecordFromStore(old), projectRecordFromStore(updated), true, nil
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

func (r recordStoreOrgProjectRepository) DeleteProjectCascadeTx(ctx context.Context, tx platform.StoreTx, projectID string) (orgProjectRecord, orgProjectDeleteResult, bool, error) {
	if r.store == nil {
		return orgProjectRecord{}, orgProjectDeleteResult{}, false, nil
	}
	project, found := r.FindProject(ctx, projectID)
	if !found {
		return orgProjectRecord{}, orgProjectDeleteResult{}, false, nil
	}
	deleted, err := tx.Delete(ctx, projectsResource, project.ID)
	if err != nil || !deleted {
		return orgProjectRecord{}, orgProjectDeleteResult{}, false, err
	}
	result := orgProjectDeleteResult{}
	if result.ProjectMembers, err = r.deleteProjectOwnedRowsTx(ctx, tx, projectMembersResource, project.ID, projectID); err != nil {
		return orgProjectRecord{}, orgProjectDeleteResult{}, false, err
	}
	if result.UserQuotas, err = r.deleteProjectOwnedRowsTx(ctx, tx, projectUserQuotasResource, project.ID, projectID); err != nil {
		return orgProjectRecord{}, orgProjectDeleteResult{}, false, err
	}
	if r.groupGPU != nil {
		deleted, err := r.groupGPU.DeleteGPUClaimsByProjectTx(ctx, tx, project.ID)
		if err != nil {
			return orgProjectRecord{}, orgProjectDeleteResult{}, false, err
		}
		result.GPUClaims = deleted
	}
	return project, result, true, nil
}

func (r recordStoreOrgProjectRepository) deleteProjectOwnedRowsTx(
	ctx context.Context,
	tx platform.StoreTx,
	resource string,
	canonicalProjectID string,
	requestedProjectID string,
) (int, error) {
	deletedRows := 0
	for _, record := range r.store.List(ctx, resource) {
		if !projectRowBelongsTo(record.Data, canonicalProjectID, requestedProjectID) {
			continue
		}
		deleted, err := tx.Delete(ctx, resource, record.ID)
		if err != nil {
			return 0, err
		}
		if deleted {
			deletedRows++
		}
	}
	return deletedRows, nil
}

func projectRowBelongsTo(row map[string]any, canonicalProjectID, requestedProjectID string) bool {
	rowProjectID := shared.TextValue(row, "project_id", "projectId")
	return rowProjectID == canonicalProjectID || rowProjectID == requestedProjectID
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

func (r recordStoreOrgProjectRepository) CreateDirectProjectMemberTx(ctx context.Context, tx platform.StoreTx, member map[string]any) (orgProjectMemberRecord, error) {
	record, err := tx.Create(ctx, projectMembersResource, shared.CloneMap(member))
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

func (r recordStoreOrgProjectRepository) UpdateDirectProjectMemberRoleTx(ctx context.Context, tx platform.StoreTx, projectID, userID, role string, now time.Time) (orgProjectMemberRecord, orgProjectMemberRecord, bool, error) {
	old, found := r.FindDirectProjectMember(ctx, projectID, userID)
	if !found {
		return orgProjectMemberRecord{}, orgProjectMemberRecord{}, false, nil
	}
	updated, ok, err := tx.Update(ctx, projectMembersResource, old.ID, map[string]any{"role": role, "updated_at": now})
	if err != nil || !ok {
		return orgProjectMemberRecord{}, orgProjectMemberRecord{}, false, err
	}
	return old, memberRecordFromStore(updated), true, nil
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

func (r recordStoreOrgProjectRepository) DeleteDirectProjectMemberAndQuotaTx(ctx context.Context, tx platform.StoreTx, projectID, userID string) (orgProjectMemberRecord, bool, error) {
	member, found := r.FindDirectProjectMember(ctx, projectID, userID)
	if !found {
		return orgProjectMemberRecord{}, false, nil
	}
	deleted, err := tx.Delete(ctx, projectMembersResource, member.ID)
	if err != nil || !deleted {
		return orgProjectMemberRecord{}, false, err
	}
	if quota, found := r.GetProjectUserQuota(ctx, projectID, userID); found {
		if _, err := tx.Delete(ctx, projectUserQuotasResource, quota.ID); err != nil {
			return orgProjectMemberRecord{}, false, err
		}
	}
	return member, true, nil
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

func (r recordStoreOrgProjectRepository) UpsertProjectUserQuotaTx(ctx context.Context, tx platform.StoreTx, quota map[string]any) (orgProjectQuotaRecord, error) {
	id := shared.TextValue(quota, "id")
	if _, found := r.store.Get(ctx, projectUserQuotasResource, id); found {
		updated, ok, err := tx.Update(ctx, projectUserQuotasResource, id, shared.CloneMap(quota))
		if err != nil || !ok {
			return orgProjectQuotaRecord{}, err
		}
		return quotaRecordFromStore(updated), nil
	}
	record, err := tx.Create(ctx, projectUserQuotasResource, shared.CloneMap(quota))
	if err != nil {
		if platform.IsCreateConflict(err) {
			updated, ok, updateErr := tx.Update(ctx, projectUserQuotasResource, id, shared.CloneMap(quota))
			if updateErr != nil || !ok {
				return orgProjectQuotaRecord{}, updateErr
			}
			return quotaRecordFromStore(updated), nil
		}
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

func (r recordStoreOrgProjectRepository) DeleteProjectUserQuotaTx(ctx context.Context, tx platform.StoreTx, projectID, userID string) (bool, error) {
	quota, found := r.GetProjectUserQuota(ctx, projectID, userID)
	if !found {
		return false, nil
	}
	return tx.Delete(ctx, projectUserQuotasResource, quota.ID)
}

func (r recordStoreOrgProjectRepository) BindProjectPlan(ctx context.Context, projectID, planID string, now time.Time) (orgProjectRecord, orgProjectRecord, bool) {
	return r.UpdateProject(ctx, projectID, map[string]any{
		"plan_id":          planID,
		"resource_plan_id": planID,
		"updated_at":       now,
	})
}

func (r recordStoreOrgProjectRepository) BindProjectPlanTx(ctx context.Context, tx platform.StoreTx, projectID, planID string, now time.Time) (orgProjectRecord, orgProjectRecord, bool, error) {
	old, found := r.store.Get(ctx, projectsResource, projectID)
	if !found {
		return orgProjectRecord{}, orgProjectRecord{}, false, nil
	}
	updated, ok, err := tx.Update(ctx, projectsResource, projectID, map[string]any{
		"plan_id":          planID,
		"resource_plan_id": planID,
		"updated_at":       now,
	})
	if err != nil || !ok {
		return orgProjectRecord{}, orgProjectRecord{}, false, err
	}
	return projectRecordFromStore(old), projectRecordFromStore(updated), true, nil
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

func (r recordStoreOrgProjectRepository) ClearProjectsPlanTx(ctx context.Context, tx platform.StoreTx, planID string, now time.Time, emit func(orgProjectPlanUpdate)) (int, error) {
	cleared := 0
	for _, project := range r.ListProjects(ctx) {
		if shared.TextValue(project.Data, "plan_id", "planId", "resource_plan_id", "resourcePlanId") != planID {
			continue
		}
		updated, ok, err := tx.Update(ctx, projectsResource, project.ID, map[string]any{
			"plan_id":          "",
			"resource_plan_id": "",
			"updated_at":       now,
		})
		if err != nil {
			return cleared, err
		}
		if !ok {
			continue
		}
		update := orgProjectPlanUpdate{Old: project, New: projectRecordFromStore(updated)}
		if emit != nil {
			emit(update)
		}
		cleared++
	}
	return cleared, nil
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
