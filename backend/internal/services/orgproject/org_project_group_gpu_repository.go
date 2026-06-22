package orgproject

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

const (
	groupsResource     = serviceName + ":groups"
	userGroupsResource = serviceName + ":user_groups"
	gpuClaimsResource  = serviceName + ":gpu_claims"
)

var errOrgProjectGroupGPUStoreUnavailable = errors.New("org-project group/gpu repository unavailable")

type orgProjectGroupRecord struct {
	ID   string
	Data map[string]any
}

type orgProjectMembershipRecord struct {
	ID   string
	Data map[string]any
}

type orgProjectGPUClaimRecord struct {
	ID   string
	Data map[string]any
}

type recordStoreOrgProjectGroupGPURepository struct {
	store platform.RecordStore
}

func groupGPURepository(app *platform.App) *recordStoreOrgProjectGroupGPURepository {
	if app == nil {
		return &recordStoreOrgProjectGroupGPURepository{}
	}
	return groupGPURepositoryFromStore(app.Store)
}

func groupGPURepositoryFromStore(store platform.RecordStore) *recordStoreOrgProjectGroupGPURepository {
	return &recordStoreOrgProjectGroupGPURepository{store: store}
}

func registerGroupGPUReadContracts(app *platform.App) {
	app.RegisterReadContract(userGroupsResource, "/internal/org-project/user-groups", "/internal/org-project/user-groups/{id...}")
}

func (r recordStoreOrgProjectGroupGPURepository) ListGroups(ctx context.Context) []orgProjectGroupRecord {
	if r.store == nil {
		return nil
	}
	records := r.store.List(ctx, groupsResource)
	out := make([]orgProjectGroupRecord, 0, len(records))
	for _, record := range records {
		out = append(out, groupRecordFromStore(record))
	}
	return out
}

func (r recordStoreOrgProjectGroupGPURepository) FindGroup(ctx context.Context, id string) (orgProjectGroupRecord, bool) {
	if r.store == nil {
		return orgProjectGroupRecord{}, false
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return orgProjectGroupRecord{}, false
	}
	if record, found := r.store.Get(ctx, groupsResource, id); found {
		return groupRecordFromStore(record), true
	}
	for _, group := range r.ListGroups(ctx) {
		if groupID(group.Data) == id {
			return group, true
		}
	}
	return orgProjectGroupRecord{}, false
}

func (r recordStoreOrgProjectGroupGPURepository) CreateGroup(ctx context.Context, group map[string]any) (orgProjectGroupRecord, error) {
	if r.store == nil {
		return orgProjectGroupRecord{}, errOrgProjectGroupGPUStoreUnavailable
	}
	record, err := r.store.Create(ctx, groupsResource, shared.CloneMap(group))
	if err != nil {
		return orgProjectGroupRecord{}, err
	}
	return groupRecordFromStore(record), nil
}

// CreateGroupWithEvent persists the group and its event in one transaction.
func (r recordStoreOrgProjectGroupGPURepository) CreateGroupWithEvent(ctx context.Context, app *platform.App, group map[string]any, build func(contracts.Record[map[string]any]) contracts.Event) (orgProjectGroupRecord, error) {
	if r.store == nil {
		return orgProjectGroupRecord{}, errOrgProjectGroupGPUStoreUnavailable
	}
	record, err := app.CreateRecordWithEvent(ctx, groupsResource, shared.CloneMap(group), build)
	if err != nil {
		return orgProjectGroupRecord{}, err
	}
	return groupRecordFromStore(record), nil
}

// UpdateGroupWithEvent resolves the group id, then commits update+event in one tx.
func (r recordStoreOrgProjectGroupGPURepository) UpdateGroupWithEvent(ctx context.Context, app *platform.App, id string, update map[string]any, build func(old, updated orgProjectGroupRecord) contracts.Event) (orgProjectGroupRecord, orgProjectGroupRecord, bool, error) {
	if r.store == nil {
		return orgProjectGroupRecord{}, orgProjectGroupRecord{}, false, nil
	}
	group, found := r.FindGroup(ctx, id)
	if !found {
		return orgProjectGroupRecord{}, orgProjectGroupRecord{}, false, nil
	}
	old, found := r.store.Get(ctx, groupsResource, group.ID)
	if !found {
		return orgProjectGroupRecord{}, orgProjectGroupRecord{}, false, nil
	}
	oldRec := groupRecordFromStore(old)
	updated, ok, err := app.UpdateRecordWithEvent(ctx, groupsResource, group.ID, shared.CloneMap(update), func(rec contracts.Record[map[string]any]) contracts.Event {
		return build(oldRec, groupRecordFromStore(rec))
	})
	if err != nil || !ok {
		return orgProjectGroupRecord{}, orgProjectGroupRecord{}, false, err
	}
	return oldRec, groupRecordFromStore(updated), true, nil
}

func (r recordStoreOrgProjectGroupGPURepository) UpdateGroup(ctx context.Context, id string, update map[string]any) (orgProjectGroupRecord, orgProjectGroupRecord, bool) {
	if r.store == nil {
		return orgProjectGroupRecord{}, orgProjectGroupRecord{}, false
	}
	group, found := r.FindGroup(ctx, id)
	if !found {
		return orgProjectGroupRecord{}, orgProjectGroupRecord{}, false
	}
	old, found := r.store.Get(ctx, groupsResource, group.ID)
	if !found {
		return orgProjectGroupRecord{}, orgProjectGroupRecord{}, false
	}
	updated, ok := r.store.Update(ctx, groupsResource, group.ID, shared.CloneMap(update))
	if !ok {
		return orgProjectGroupRecord{}, orgProjectGroupRecord{}, false
	}
	return groupRecordFromStore(old), groupRecordFromStore(updated), true
}

func (r recordStoreOrgProjectGroupGPURepository) DeleteGroupCascade(ctx context.Context, id string) (orgProjectGroupRecord, int, bool) {
	if r.store == nil {
		return orgProjectGroupRecord{}, 0, false
	}
	group, found := r.FindGroup(ctx, id)
	if !found {
		return orgProjectGroupRecord{}, 0, false
	}
	if !r.store.Delete(ctx, groupsResource, group.ID) {
		return orgProjectGroupRecord{}, 0, false
	}
	deletedMemberships := r.DeleteMembershipsByGroup(ctx, group.ID)
	if logicalID := groupID(group.Data); logicalID != "" && logicalID != group.ID {
		deletedMemberships += r.DeleteMembershipsByGroup(ctx, logicalID)
	}
	return group, deletedMemberships, true
}

func (r recordStoreOrgProjectGroupGPURepository) DeleteGroupCascadeTx(ctx context.Context, tx platform.StoreTx, id string) (orgProjectGroupRecord, int, bool, error) {
	if r.store == nil {
		return orgProjectGroupRecord{}, 0, false, nil
	}
	group, found := r.FindGroup(ctx, id)
	if !found {
		return orgProjectGroupRecord{}, 0, false, nil
	}
	deleted, err := tx.Delete(ctx, groupsResource, group.ID)
	if err != nil || !deleted {
		return orgProjectGroupRecord{}, 0, false, err
	}
	deletedMemberships, err := r.deleteMembershipsByGroupTx(ctx, tx, group.ID)
	if err != nil {
		return orgProjectGroupRecord{}, 0, false, err
	}
	if logicalID := groupID(group.Data); logicalID != "" && logicalID != group.ID {
		logicalDeleted, err := r.deleteMembershipsByGroupTx(ctx, tx, logicalID)
		if err != nil {
			return orgProjectGroupRecord{}, 0, false, err
		}
		deletedMemberships += logicalDeleted
	}
	return group, deletedMemberships, true, nil
}

func (r recordStoreOrgProjectGroupGPURepository) NextGroupID() string {
	if r.store == nil {
		return ""
	}
	return r.store.NextID(groupsResource, "G", 1, 7)
}

func (r recordStoreOrgProjectGroupGPURepository) ListMemberships(ctx context.Context) []orgProjectMembershipRecord {
	if r.store == nil {
		return nil
	}
	records := r.store.List(ctx, userGroupsResource)
	out := make([]orgProjectMembershipRecord, 0, len(records))
	for _, record := range records {
		out = append(out, membershipRecordFromStore(record))
	}
	return out
}

func (r recordStoreOrgProjectGroupGPURepository) FindMembership(ctx context.Context, userID, groupID string) (orgProjectMembershipRecord, bool) {
	if r.store == nil {
		return orgProjectMembershipRecord{}, false
	}
	for _, id := range membershipKeys(userID, groupID) {
		if record, found := r.store.Get(ctx, userGroupsResource, id); found {
			return membershipRecordFromStore(record), true
		}
	}
	for _, record := range r.store.List(ctx, userGroupsResource) {
		if shared.TextValue(record.Data, "user_id", "userId", "uid", "u_id") == userID &&
			shared.TextValue(record.Data, "group_id", "groupId", "gid", "g_id") == groupID {
			return membershipRecordFromStore(record), true
		}
	}
	return orgProjectMembershipRecord{}, false
}

func (r recordStoreOrgProjectGroupGPURepository) CreateMembership(ctx context.Context, membership map[string]any) (orgProjectMembershipRecord, error) {
	if r.store == nil {
		return orgProjectMembershipRecord{}, errOrgProjectGroupGPUStoreUnavailable
	}
	record, err := r.store.Create(ctx, userGroupsResource, shared.CloneMap(membership))
	if err != nil {
		return orgProjectMembershipRecord{}, err
	}
	return membershipRecordFromStore(record), nil
}

func (r recordStoreOrgProjectGroupGPURepository) CreateMembershipTx(ctx context.Context, tx platform.StoreTx, membership map[string]any) (orgProjectMembershipRecord, error) {
	if r.store == nil {
		return orgProjectMembershipRecord{}, errOrgProjectGroupGPUStoreUnavailable
	}
	record, err := tx.Create(ctx, userGroupsResource, shared.CloneMap(membership))
	if err != nil {
		return orgProjectMembershipRecord{}, err
	}
	return membershipRecordFromStore(record), nil
}

func (r recordStoreOrgProjectGroupGPURepository) UpdateMembershipRole(ctx context.Context, userID, groupID, role string, now time.Time) (orgProjectMembershipRecord, orgProjectMembershipRecord, bool) {
	if r.store == nil {
		return orgProjectMembershipRecord{}, orgProjectMembershipRecord{}, false
	}
	old, found := r.FindMembership(ctx, userID, groupID)
	if !found {
		return orgProjectMembershipRecord{}, orgProjectMembershipRecord{}, false
	}
	updated, ok := r.store.Update(ctx, userGroupsResource, old.ID, map[string]any{"role": role, "updated_at": now})
	if !ok {
		return orgProjectMembershipRecord{}, orgProjectMembershipRecord{}, false
	}
	return old, membershipRecordFromStore(updated), true
}

func (r recordStoreOrgProjectGroupGPURepository) UpdateMembershipRoleTx(ctx context.Context, tx platform.StoreTx, userID, groupID, role string, now time.Time) (orgProjectMembershipRecord, orgProjectMembershipRecord, bool, error) {
	if r.store == nil {
		return orgProjectMembershipRecord{}, orgProjectMembershipRecord{}, false, nil
	}
	old, found := r.FindMembership(ctx, userID, groupID)
	if !found {
		return orgProjectMembershipRecord{}, orgProjectMembershipRecord{}, false, nil
	}
	updated, ok, err := tx.Update(ctx, userGroupsResource, old.ID, map[string]any{"role": role, "updated_at": now})
	if err != nil || !ok {
		return orgProjectMembershipRecord{}, orgProjectMembershipRecord{}, false, err
	}
	return old, membershipRecordFromStore(updated), true, nil
}

func (r recordStoreOrgProjectGroupGPURepository) DeleteMembership(ctx context.Context, userID, groupID string) (orgProjectMembershipRecord, bool) {
	if r.store == nil {
		return orgProjectMembershipRecord{}, false
	}
	membership, found := r.FindMembership(ctx, userID, groupID)
	if !found {
		return orgProjectMembershipRecord{}, false
	}
	if !r.store.Delete(ctx, userGroupsResource, membership.ID) {
		return orgProjectMembershipRecord{}, false
	}
	return membership, true
}

func (r recordStoreOrgProjectGroupGPURepository) DeleteMembershipTx(ctx context.Context, tx platform.StoreTx, userID, groupID string) (orgProjectMembershipRecord, bool, error) {
	if r.store == nil {
		return orgProjectMembershipRecord{}, false, nil
	}
	membership, found := r.FindMembership(ctx, userID, groupID)
	if !found {
		return orgProjectMembershipRecord{}, false, nil
	}
	deleted, err := tx.Delete(ctx, userGroupsResource, membership.ID)
	if err != nil || !deleted {
		return orgProjectMembershipRecord{}, false, err
	}
	return membership, true, nil
}

func (r recordStoreOrgProjectGroupGPURepository) DeleteMembershipsByGroup(ctx context.Context, groupID string) int {
	if r.store == nil {
		return 0
	}
	count := 0
	for _, record := range r.store.List(ctx, userGroupsResource) {
		if shared.TextValue(record.Data, "group_id", "groupId", "gid", "g_id") != groupID {
			continue
		}
		if r.store.Delete(ctx, userGroupsResource, record.ID) {
			count++
		}
	}
	return count
}

func (r recordStoreOrgProjectGroupGPURepository) deleteMembershipsByGroupTx(ctx context.Context, tx platform.StoreTx, groupID string) (int, error) {
	if r.store == nil {
		return 0, nil
	}
	count := 0
	for _, record := range r.store.List(ctx, userGroupsResource) {
		if shared.TextValue(record.Data, "group_id", "groupId", "gid", "g_id") != groupID {
			continue
		}
		deleted, err := tx.Delete(ctx, userGroupsResource, record.ID)
		if err != nil {
			return 0, err
		}
		if deleted {
			count++
		}
	}
	return count, nil
}

func (r recordStoreOrgProjectGroupGPURepository) ListGPUClaimsByProject(ctx context.Context, projectID string) []orgProjectGPUClaimRecord {
	if r.store == nil {
		return nil
	}
	out := []orgProjectGPUClaimRecord{}
	for _, record := range r.store.List(ctx, gpuClaimsResource) {
		if shared.TextValue(record.Data, "project_id", "projectId") == projectID {
			out = append(out, gpuClaimRecordFromStore(record))
		}
	}
	return out
}

func (r recordStoreOrgProjectGroupGPURepository) FindGPUClaim(ctx context.Context, projectID, name, namespace string) (orgProjectGPUClaimRecord, bool) {
	if r.store == nil {
		return orgProjectGPUClaimRecord{}, false
	}
	if strings.TrimSpace(namespace) != "" {
		if record, found := r.store.Get(ctx, gpuClaimsResource, gpuClaimID(projectID, namespace, name)); found {
			return gpuClaimRecordFromStore(record), true
		}
	}
	for _, record := range r.store.List(ctx, gpuClaimsResource) {
		if shared.TextValue(record.Data, "project_id", "projectId") == projectID &&
			shared.TextValue(record.Data, "name") == name {
			return gpuClaimRecordFromStore(record), true
		}
	}
	return orgProjectGPUClaimRecord{}, false
}

func (r recordStoreOrgProjectGroupGPURepository) CreateGPUClaim(ctx context.Context, claim map[string]any) (orgProjectGPUClaimRecord, error) {
	if r.store == nil {
		return orgProjectGPUClaimRecord{}, errOrgProjectGroupGPUStoreUnavailable
	}
	record, err := r.store.Create(ctx, gpuClaimsResource, shared.CloneMap(claim))
	if err != nil {
		return orgProjectGPUClaimRecord{}, err
	}
	return gpuClaimRecordFromStore(record), nil
}

func (r recordStoreOrgProjectGroupGPURepository) CreateGPUClaimTx(ctx context.Context, tx platform.StoreTx, claim map[string]any) (orgProjectGPUClaimRecord, error) {
	if r.store == nil {
		return orgProjectGPUClaimRecord{}, errOrgProjectGroupGPUStoreUnavailable
	}
	record, err := tx.Create(ctx, gpuClaimsResource, shared.CloneMap(claim))
	if err != nil {
		return orgProjectGPUClaimRecord{}, err
	}
	return gpuClaimRecordFromStore(record), nil
}

func (r recordStoreOrgProjectGroupGPURepository) DeleteGPUClaim(ctx context.Context, id string) (orgProjectGPUClaimRecord, bool) {
	if r.store == nil || strings.TrimSpace(id) == "" {
		return orgProjectGPUClaimRecord{}, false
	}
	record, found := r.store.Get(ctx, gpuClaimsResource, id)
	if !found {
		return orgProjectGPUClaimRecord{}, false
	}
	if !r.store.Delete(ctx, gpuClaimsResource, id) {
		return orgProjectGPUClaimRecord{}, false
	}
	return gpuClaimRecordFromStore(record), true
}

func (r recordStoreOrgProjectGroupGPURepository) DeleteGPUClaimTx(ctx context.Context, tx platform.StoreTx, id string) (orgProjectGPUClaimRecord, bool, error) {
	if r.store == nil || strings.TrimSpace(id) == "" {
		return orgProjectGPUClaimRecord{}, false, nil
	}
	record, found := r.store.Get(ctx, gpuClaimsResource, id)
	if !found {
		return orgProjectGPUClaimRecord{}, false, nil
	}
	deleted, err := tx.Delete(ctx, gpuClaimsResource, id)
	if err != nil || !deleted {
		return orgProjectGPUClaimRecord{}, false, err
	}
	return gpuClaimRecordFromStore(record), true, nil
}

func (r recordStoreOrgProjectGroupGPURepository) DeleteGPUClaimsByProject(ctx context.Context, projectID string) int {
	if r.store == nil {
		return 0
	}
	count := 0
	for _, record := range r.store.List(ctx, gpuClaimsResource) {
		if shared.TextValue(record.Data, "project_id", "projectId") != projectID {
			continue
		}
		if r.store.Delete(ctx, gpuClaimsResource, record.ID) {
			count++
		}
	}
	return count
}

func (r recordStoreOrgProjectGroupGPURepository) DeleteGPUClaimsByProjectTx(ctx context.Context, tx platform.StoreTx, projectID string) (int, error) {
	if r.store == nil {
		return 0, nil
	}
	count := 0
	for _, record := range r.store.List(ctx, gpuClaimsResource) {
		if shared.TextValue(record.Data, "project_id", "projectId") != projectID {
			continue
		}
		deleted, err := tx.Delete(ctx, gpuClaimsResource, record.ID)
		if err != nil {
			return 0, err
		}
		if deleted {
			count++
		}
	}
	return count, nil
}

func groupRecordFromStore(record contracts.Record[map[string]any]) orgProjectGroupRecord {
	return orgProjectGroupRecord{ID: record.ID, Data: recordDataWithID(record)}
}

func membershipRecordFromStore(record contracts.Record[map[string]any]) orgProjectMembershipRecord {
	return orgProjectMembershipRecord{ID: record.ID, Data: recordDataWithID(record)}
}

func gpuClaimRecordFromStore(record contracts.Record[map[string]any]) orgProjectGPUClaimRecord {
	return orgProjectGPUClaimRecord{ID: record.ID, Data: recordDataWithID(record)}
}

func membershipKeys(userID, groupID string) []string {
	return []string{membershipID(userID, groupID), userID + "/" + groupID}
}
