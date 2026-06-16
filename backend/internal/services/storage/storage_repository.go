package storage

import (
	"context"
	"errors"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

const (
	groupStorageResource       = serviceName + ":group_storage"
	storagePermissionsResource = serviceName + ":storage_permissions"
	storagePoliciesResource    = serviceName + ":storage_access_policies"
	projectBindingsResource    = serviceName + ":storage_bindings"
	projectPermissionsResource = serviceName + ":project_storage_permissions"
	fastTransfersResource      = serviceName + ":fast_transfers"
	userStorageResource        = serviceName + ":user_storage"
)

var errStorageRepositoryUnavailable = errors.New("storage repository unavailable")

type storageRepository interface {
	ListGroupStorage(context.Context) []map[string]any
	ListGroupStorageByGroup(context.Context, string) []map[string]any
	CreateGroupStorage(context.Context, map[string]any) (map[string]any, error)
	UpdateGroupStorageStatus(context.Context, string, string, string, time.Time) (map[string]any, bool)
	DeleteGroupStorageCascade(context.Context, string, string) bool
	FindGroupStorageSource(context.Context, string, string) (map[string]any, bool)

	UpsertStoragePermission(context.Context, map[string]any) (map[string]any, error)
	DeleteStoragePermission(context.Context, string, string, string) bool
	ListStoragePermissionsForPVC(context.Context, string, string) []map[string]any
	UpsertStoragePolicy(context.Context, map[string]any) (map[string]any, error)
	GetStoragePolicy(context.Context, string, string) (map[string]any, bool)

	ListProjectBindings(context.Context, string) []map[string]any
	CreateProjectBinding(context.Context, map[string]any) (map[string]any, error)
	DeleteProjectBindingCascade(context.Context, string, string) bool
	FindProjectStorageBinding(context.Context, string, string) (map[string]any, bool)
	UpsertProjectPermission(context.Context, map[string]any) (map[string]any, error)
	DeleteProjectPermission(context.Context, string, string, string) bool
	ListProjectPermissionsForPVC(context.Context, string, string) []map[string]any

	NextFastTransferName() string
	CreateFastTransfer(context.Context, map[string]any) (map[string]any, error)
	GetFastTransfer(context.Context, string, string, string) (map[string]any, bool)
	CancelFastTransfer(context.Context, string, string, string, time.Time) (map[string]any, bool)

	UpsertUserStorage(context.Context, string, map[string]any) (map[string]any, error)
	UserStorageStatus(context.Context, string) map[string]any

	EffectiveStoragePermission(context.Context, string, string, string, string) string
}

type recordStoreStorageRepository struct {
	store platform.RecordStore
}

func storageRepo(app *platform.App) storageRepository {
	if app == nil {
		return recordStoreStorageRepository{}
	}
	return recordStoreStorageRepository{store: app.Store}
}

func (r recordStoreStorageRepository) ListGroupStorage(ctx context.Context) []map[string]any {
	return r.listMaps(ctx, groupStorageResource)
}

func (r recordStoreStorageRepository) ListGroupStorageByGroup(ctx context.Context, groupID string) []map[string]any {
	return filterRows(r.ListGroupStorage(ctx), "group_id", groupID)
}

func (r recordStoreStorageRepository) CreateGroupStorage(ctx context.Context, data map[string]any) (map[string]any, error) {
	return r.create(ctx, groupStorageResource, data)
}

func (r recordStoreStorageRepository) UpdateGroupStorageStatus(
	ctx context.Context,
	groupID, pvcID, status string,
	now time.Time,
) (map[string]any, bool) {
	return r.update(ctx, groupStorageResource, groupStorageID(groupID, pvcID), map[string]any{
		"status":     status,
		"updated_at": now.UTC(),
	})
}

func (r recordStoreStorageRepository) DeleteGroupStorageCascade(ctx context.Context, groupID, pvcID string) bool {
	if !r.delete(ctx, groupStorageResource, groupStorageID(groupID, pvcID)) {
		return false
	}
	r.deleteMatching(ctx, storagePermissionsResource, func(row map[string]any) bool {
		return text(row, "group_id") == groupID && text(row, "pvc_id") == pvcID
	})
	r.deleteMatching(ctx, storagePoliciesResource, func(row map[string]any) bool {
		return text(row, "group_id") == groupID && text(row, "pvc_id") == pvcID
	})
	return true
}

func (r recordStoreStorageRepository) FindGroupStorageSource(ctx context.Context, groupID, pvcID string) (map[string]any, bool) {
	return r.get(ctx, groupStorageResource, groupStorageID(groupID, pvcID))
}

func (r recordStoreStorageRepository) UpsertStoragePermission(ctx context.Context, data map[string]any) (map[string]any, error) {
	return r.upsert(ctx, storagePermissionsResource, shared.TextValue(data, "id"), data)
}

func (r recordStoreStorageRepository) DeleteStoragePermission(ctx context.Context, groupID, pvcID, userID string) bool {
	return r.delete(ctx, storagePermissionsResource, storagePermissionID(groupID, pvcID, userID))
}

func (r recordStoreStorageRepository) ListStoragePermissionsForPVC(ctx context.Context, groupID, pvcID string) []map[string]any {
	return r.listMatching(ctx, storagePermissionsResource, func(row map[string]any) bool {
		return text(row, "group_id") == groupID && text(row, "pvc_id") == pvcID
	})
}

func (r recordStoreStorageRepository) UpsertStoragePolicy(ctx context.Context, data map[string]any) (map[string]any, error) {
	return r.upsert(ctx, storagePoliciesResource, storagePolicyID(text(data, "group_id"), text(data, "pvc_id")), data)
}

func (r recordStoreStorageRepository) GetStoragePolicy(ctx context.Context, groupID, pvcID string) (map[string]any, bool) {
	return r.get(ctx, storagePoliciesResource, storagePolicyID(groupID, pvcID))
}

func (r recordStoreStorageRepository) ListProjectBindings(ctx context.Context, projectID string) []map[string]any {
	return r.listMatching(ctx, projectBindingsResource, func(row map[string]any) bool {
		return text(row, "project_id") == projectID
	})
}

func (r recordStoreStorageRepository) CreateProjectBinding(ctx context.Context, data map[string]any) (map[string]any, error) {
	return r.create(ctx, projectBindingsResource, data)
}

func (r recordStoreStorageRepository) DeleteProjectBindingCascade(ctx context.Context, projectID, pvcID string) bool {
	if !r.delete(ctx, projectBindingsResource, projectBindingID(projectID, pvcID)) {
		return false
	}
	r.deleteMatching(ctx, projectPermissionsResource, func(row map[string]any) bool {
		return text(row, "project_id") == projectID && text(row, "pvc_id") == pvcID
	})
	return true
}

func (r recordStoreStorageRepository) FindProjectStorageBinding(ctx context.Context, projectID, pvcID string) (map[string]any, bool) {
	return r.get(ctx, projectBindingsResource, projectBindingID(projectID, pvcID))
}

func (r recordStoreStorageRepository) UpsertProjectPermission(ctx context.Context, data map[string]any) (map[string]any, error) {
	return r.upsert(ctx, projectPermissionsResource, shared.TextValue(data, "id"), data)
}

func (r recordStoreStorageRepository) DeleteProjectPermission(ctx context.Context, projectID, pvcID, userID string) bool {
	return r.delete(ctx, projectPermissionsResource, projectPermissionID(projectID, pvcID, userID))
}

func (r recordStoreStorageRepository) ListProjectPermissionsForPVC(ctx context.Context, projectID, pvcID string) []map[string]any {
	return r.listMatching(ctx, projectPermissionsResource, func(row map[string]any) bool {
		return text(row, "project_id") == projectID && text(row, "pvc_id") == pvcID
	})
}

func (r recordStoreStorageRepository) NextFastTransferName() string {
	if r.store == nil {
		return ""
	}
	return r.store.NextID(fastTransfersResource, "transfer-", 1, 5)
}

func (r recordStoreStorageRepository) CreateFastTransfer(ctx context.Context, data map[string]any) (map[string]any, error) {
	return r.create(ctx, fastTransfersResource, data)
}

func (r recordStoreStorageRepository) GetFastTransfer(ctx context.Context, projectID, namespace, name string) (map[string]any, bool) {
	return r.get(ctx, fastTransfersResource, fastTransferID(projectID, namespace, name))
}

func (r recordStoreStorageRepository) CancelFastTransfer(
	ctx context.Context,
	projectID, namespace, name string,
	now time.Time,
) (map[string]any, bool) {
	return r.update(ctx, fastTransfersResource, fastTransferID(projectID, namespace, name), map[string]any{
		"status":     "cancelled",
		"updated_at": now.UTC(),
	})
}

func (r recordStoreStorageRepository) UpsertUserStorage(ctx context.Context, username string, data map[string]any) (map[string]any, error) {
	return r.upsert(ctx, userStorageResource, username, data)
}

func (r recordStoreStorageRepository) UserStorageStatus(ctx context.Context, username string) map[string]any {
	if record, found := r.get(ctx, userStorageResource, username); found {
		return record
	}
	return map[string]any{"id": username, "username": username, "status": "missing"}
}

func (r recordStoreStorageRepository) EffectiveStoragePermission(
	ctx context.Context,
	projectID, groupID, pvcID, userID string,
) string {
	if record, found := r.get(ctx, projectPermissionsResource, projectPermissionID(projectID, pvcID, userID)); found {
		return normalizePermission(text(record, "permission"))
	}
	if record, found := r.get(ctx, storagePermissionsResource, storagePermissionID(groupID, pvcID, userID)); found {
		return normalizePermission(text(record, "permission"))
	}
	if record, found := r.get(ctx, storagePoliciesResource, storagePolicyID(groupID, pvcID)); found {
		return normalizePermission(text(record, "default_permission", "defaultPermission"))
	}
	return "none"
}

func (r recordStoreStorageRepository) create(ctx context.Context, resource string, data map[string]any) (map[string]any, error) {
	if r.store == nil {
		return nil, errStorageRepositoryUnavailable
	}
	record, err := r.store.Create(ctx, resource, shared.CloneMap(data))
	if err != nil {
		return nil, err
	}
	return storageRecordMap(record), nil
}

func (r recordStoreStorageRepository) get(ctx context.Context, resource, id string) (map[string]any, bool) {
	if r.store == nil {
		return nil, false
	}
	record, found := r.store.Get(ctx, resource, id)
	if !found {
		return nil, false
	}
	return storageRecordMap(record), true
}

func (r recordStoreStorageRepository) update(ctx context.Context, resource, id string, data map[string]any) (map[string]any, bool) {
	if r.store == nil {
		return nil, false
	}
	record, ok := r.store.Update(ctx, resource, id, shared.CloneMap(data))
	if !ok {
		return nil, false
	}
	return storageRecordMap(record), true
}

func (r recordStoreStorageRepository) upsert(ctx context.Context, resource, id string, data map[string]any) (map[string]any, error) {
	if r.store == nil {
		return nil, errStorageRepositoryUnavailable
	}
	payload := shared.CloneMap(data)
	payload["id"] = id
	if updated, ok := r.store.Update(ctx, resource, id, payload); ok {
		return storageRecordMap(updated), nil
	}
	record, err := r.store.Create(ctx, resource, payload)
	if err != nil {
		if platform.IsCreateConflict(err) {
			if updated, ok := r.store.Update(ctx, resource, id, payload); ok {
				return storageRecordMap(updated), nil
			}
		}
		return nil, err
	}
	return storageRecordMap(record), nil
}

func (r recordStoreStorageRepository) delete(ctx context.Context, resource, id string) bool {
	if r.store == nil {
		return false
	}
	return r.store.Delete(ctx, resource, id)
}

func (r recordStoreStorageRepository) listMaps(ctx context.Context, resource string) []map[string]any {
	if r.store == nil {
		return nil
	}
	records := r.store.List(ctx, resource)
	rows := make([]map[string]any, 0, len(records))
	for _, record := range records {
		rows = append(rows, storageRecordMap(record))
	}
	return rows
}

func (r recordStoreStorageRepository) listMatching(
	ctx context.Context,
	resource string,
	matches func(map[string]any) bool,
) []map[string]any {
	rows := make([]map[string]any, 0)
	for _, row := range r.listMaps(ctx, resource) {
		if matches(row) {
			rows = append(rows, row)
		}
	}
	return rows
}

func (r recordStoreStorageRepository) deleteMatching(
	ctx context.Context,
	resource string,
	matches func(map[string]any) bool,
) int {
	deleted := 0
	if r.store == nil {
		return deleted
	}
	for _, record := range r.store.List(ctx, resource) {
		if matches(record.Data) && r.store.Delete(ctx, resource, record.ID) {
			deleted++
		}
	}
	return deleted
}

func storageRecordMap(record contracts.Record[map[string]any]) map[string]any {
	row := shared.CloneMap(record.Data)
	if row["id"] == nil {
		row["id"] = record.ID
	}
	return row
}
