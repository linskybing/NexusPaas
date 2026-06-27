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

type recordStoreStorageRepository struct {
	store platform.RecordStore
}

func storageRepo(app *platform.App) *recordStoreStorageRepository {
	if app == nil {
		return &recordStoreStorageRepository{}
	}
	return &recordStoreStorageRepository{store: app.Store}
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

// createWithEvent persists a storage record and its event in one transaction,
// keeping resource-key ownership inside this repository (storage source guard).
func (r recordStoreStorageRepository) createWithEvent(ctx context.Context, app *platform.App, resource string, data map[string]any, build func(map[string]any) contracts.Event) (map[string]any, error) {
	if r.store == nil {
		return nil, errStorageRepositoryUnavailable
	}
	record, err := app.CreateRecordWithEvent(ctx, resource, shared.CloneMap(data), func(rec contracts.Record[map[string]any]) contracts.Event {
		return build(storageRecordMap(rec))
	})
	if err != nil {
		return nil, err
	}
	return storageRecordMap(record), nil
}

func (r recordStoreStorageRepository) CreateGroupStorageWithEvent(ctx context.Context, app *platform.App, data map[string]any, build func(map[string]any) contracts.Event) (map[string]any, error) {
	return r.createWithEvent(ctx, app, groupStorageResource, data, build)
}

func (r recordStoreStorageRepository) CreateProjectBindingWithEvent(ctx context.Context, app *platform.App, data map[string]any, build func(map[string]any) contracts.Event) (map[string]any, error) {
	return r.createWithEvent(ctx, app, projectBindingsResource, data, build)
}

func (r recordStoreStorageRepository) CreateFastTransferWithEvent(ctx context.Context, app *platform.App, data map[string]any, build func(map[string]any) contracts.Event) (map[string]any, error) {
	return r.createWithEvent(ctx, app, fastTransfersResource, data, build)
}

func (r recordStoreStorageRepository) upsertWithEvent(ctx context.Context, app *platform.App, resource, id string, data map[string]any, build func(map[string]any) contracts.Event) (map[string]any, error) {
	if r.store == nil {
		return nil, errStorageRepositoryUnavailable
	}
	record, err := app.UpsertRecordWithEvent(ctx, resource, id, shared.CloneMap(data), func(rec contracts.Record[map[string]any]) contracts.Event {
		return build(storageRecordMap(rec))
	})
	if err != nil {
		return nil, err
	}
	return storageRecordMap(record), nil
}

func (r recordStoreStorageRepository) updateWithEvent(ctx context.Context, app *platform.App, resource, id string, data map[string]any, build func(map[string]any) contracts.Event) (map[string]any, bool, error) {
	if r.store == nil {
		return nil, false, errStorageRepositoryUnavailable
	}
	record, ok, err := app.UpdateRecordWithEvent(ctx, resource, id, shared.CloneMap(data), func(rec contracts.Record[map[string]any]) contracts.Event {
		return build(storageRecordMap(rec))
	})
	if err != nil || !ok {
		return nil, ok, err
	}
	return storageRecordMap(record), true, nil
}

func (r recordStoreStorageRepository) deleteWithEvent(ctx context.Context, app *platform.App, resource, id string, build func(bool) contracts.Event) (bool, error) {
	if r.store == nil {
		return false, errStorageRepositoryUnavailable
	}
	return app.DeleteRecordWithEvent(ctx, resource, id, build)
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

func (r recordStoreStorageRepository) UpdateGroupStorageStatusWithEvent(
	ctx context.Context,
	app *platform.App,
	groupID, pvcID, status string,
	now time.Time,
	build func(map[string]any) contracts.Event,
) (map[string]any, bool, error) {
	return r.updateWithEvent(ctx, app, groupStorageResource, groupStorageID(groupID, pvcID), map[string]any{
		"status":     status,
		"updated_at": now.UTC(),
	}, build)
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

func (r recordStoreStorageRepository) UpsertStoragePermissionWithEvent(ctx context.Context, app *platform.App, data map[string]any, build func(map[string]any) contracts.Event) (map[string]any, error) {
	return r.upsertWithEvent(ctx, app, storagePermissionsResource, shared.TextValue(data, "id"), data, build)
}

func (r recordStoreStorageRepository) DeleteStoragePermission(ctx context.Context, groupID, pvcID, userID string) bool {
	return r.delete(ctx, storagePermissionsResource, storagePermissionID(groupID, pvcID, userID))
}

func (r recordStoreStorageRepository) DeleteStoragePermissionWithEvent(ctx context.Context, app *platform.App, groupID, pvcID, userID string, build func(bool) contracts.Event) (bool, error) {
	return r.deleteWithEvent(ctx, app, storagePermissionsResource, storagePermissionID(groupID, pvcID, userID), build)
}

func (r recordStoreStorageRepository) ListStoragePermissionsForPVC(ctx context.Context, groupID, pvcID string) []map[string]any {
	return r.listMatching(ctx, storagePermissionsResource, func(row map[string]any) bool {
		return text(row, "group_id") == groupID && text(row, "pvc_id") == pvcID
	})
}

func (r recordStoreStorageRepository) UpsertStoragePolicy(ctx context.Context, data map[string]any) (map[string]any, error) {
	return r.upsert(ctx, storagePoliciesResource, storagePolicyID(text(data, "group_id"), text(data, "pvc_id")), data)
}

func (r recordStoreStorageRepository) UpsertStoragePolicyWithEvent(ctx context.Context, app *platform.App, data map[string]any, build func(map[string]any) contracts.Event) (map[string]any, error) {
	return r.upsertWithEvent(ctx, app, storagePoliciesResource, storagePolicyID(text(data, "group_id"), text(data, "pvc_id")), data, build)
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

func (r recordStoreStorageRepository) UpsertProjectPermissionWithEvent(ctx context.Context, app *platform.App, data map[string]any, build func(map[string]any) contracts.Event) (map[string]any, error) {
	return r.upsertWithEvent(ctx, app, projectPermissionsResource, shared.TextValue(data, "id"), data, build)
}

func (r recordStoreStorageRepository) DeleteProjectPermission(ctx context.Context, projectID, pvcID, userID string) bool {
	return r.delete(ctx, projectPermissionsResource, projectPermissionID(projectID, pvcID, userID))
}

func (r recordStoreStorageRepository) DeleteProjectPermissionWithEvent(ctx context.Context, app *platform.App, projectID, pvcID, userID string, build func(bool) contracts.Event) (bool, error) {
	return r.deleteWithEvent(ctx, app, projectPermissionsResource, projectPermissionID(projectID, pvcID, userID), build)
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

func (r recordStoreStorageRepository) FindFastTransferByIdempotencyKeyHash(ctx context.Context, projectID, keyHash string) (map[string]any, bool) {
	if keyHash == "" {
		return nil, false
	}
	for _, row := range r.listMaps(ctx, fastTransfersResource) {
		if text(row, "project_id", "projectId") == projectID && text(row, internalFastTransferIdempotencyKeyHash) == keyHash {
			return row, true
		}
	}
	return nil, false
}

func (r recordStoreStorageRepository) UpdateFastTransferWithEvent(
	ctx context.Context,
	app *platform.App,
	projectID, namespace, name string,
	data map[string]any,
	build func(map[string]any) contracts.Event,
) (map[string]any, bool, error) {
	return r.updateWithEvent(ctx, app, fastTransfersResource, fastTransferID(projectID, namespace, name), data, build)
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

func (r recordStoreStorageRepository) CancelFastTransferWithEvent(
	ctx context.Context,
	app *platform.App,
	projectID, namespace, name string,
	now time.Time,
	build func(map[string]any) contracts.Event,
) (map[string]any, bool, error) {
	return r.updateWithEvent(ctx, app, fastTransfersResource, fastTransferID(projectID, namespace, name), map[string]any{
		"status":     "cancelled",
		"updated_at": now.UTC(),
	}, build)
}

func (r recordStoreStorageRepository) UpsertUserStorage(ctx context.Context, username string, data map[string]any) (map[string]any, error) {
	return r.upsert(ctx, userStorageResource, username, data)
}

func (r recordStoreStorageRepository) UpsertUserStorageWithEvent(ctx context.Context, app *platform.App, username string, data map[string]any, build func(map[string]any) contracts.Event) (map[string]any, error) {
	return r.upsertWithEvent(ctx, app, userStorageResource, username, data, build)
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

// deleteMatchingTx deletes matching rows through the transaction (rows are found
// from the committed store). Resource-key ownership stays in this repository.
func (r recordStoreStorageRepository) deleteMatchingTx(ctx context.Context, tx platform.StoreTx, resource string, matches func(map[string]any) bool) error {
	if r.store == nil {
		return nil
	}
	for _, record := range r.store.List(ctx, resource) {
		if matches(record.Data) {
			if _, err := tx.Delete(ctx, resource, record.ID); err != nil {
				return err
			}
		}
	}
	return nil
}

// DeleteGroupStorageCascadeTx deletes the group storage plus its permissions and
// policies in one transaction so the GroupStorageDeleted event commits atomically.
func (r recordStoreStorageRepository) DeleteGroupStorageCascadeTx(ctx context.Context, tx platform.StoreTx, groupID, pvcID string) (bool, error) {
	deleted, err := tx.Delete(ctx, groupStorageResource, groupStorageID(groupID, pvcID))
	if err != nil || !deleted {
		return false, err
	}
	if err := r.deleteMatchingTx(ctx, tx, storagePermissionsResource, func(row map[string]any) bool {
		return text(row, "group_id") == groupID && text(row, "pvc_id") == pvcID
	}); err != nil {
		return false, err
	}
	if err := r.deleteMatchingTx(ctx, tx, storagePoliciesResource, func(row map[string]any) bool {
		return text(row, "group_id") == groupID && text(row, "pvc_id") == pvcID
	}); err != nil {
		return false, err
	}
	return true, nil
}

// DeleteProjectBindingCascadeTx deletes the binding plus its project permissions
// in one transaction.
func (r recordStoreStorageRepository) DeleteProjectBindingCascadeTx(ctx context.Context, tx platform.StoreTx, projectID, pvcID string) (bool, error) {
	deleted, err := tx.Delete(ctx, projectBindingsResource, projectBindingID(projectID, pvcID))
	if err != nil || !deleted {
		return false, err
	}
	if err := r.deleteMatchingTx(ctx, tx, projectPermissionsResource, func(row map[string]any) bool {
		return text(row, "project_id") == projectID && text(row, "pvc_id") == pvcID
	}); err != nil {
		return false, err
	}
	return true, nil
}

func storageRecordMap(record contracts.Record[map[string]any]) map[string]any {
	row := shared.CloneMap(record.Data)
	if row["id"] == nil {
		row["id"] = record.ID
	}
	return row
}
