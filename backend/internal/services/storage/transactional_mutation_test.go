package storage

import (
	"context"
	"net/http"
	"testing"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

type storageTransactionalStore struct {
	*platform.Store
	createWithEvent int
	updateWithEvent int
	upsertWithEvent int
	deleteWithEvent int
	txEvents        []contracts.Event
}

func (s *storageTransactionalStore) CreateWithEvent(ctx context.Context, resource string, data map[string]any, build platform.RecordEventBuilder) (contracts.Record[map[string]any], error) {
	s.createWithEvent++
	record, err := s.Store.Create(ctx, resource, data)
	if err == nil && build != nil {
		s.txEvents = append(s.txEvents, build(record))
	}
	return record, err
}

func (s *storageTransactionalStore) UpdateWithEvent(ctx context.Context, resource, id string, data map[string]any, build platform.RecordEventBuilder) (contracts.Record[map[string]any], bool, error) {
	s.updateWithEvent++
	record, ok := s.Store.Update(ctx, resource, id, data)
	if ok && build != nil {
		s.txEvents = append(s.txEvents, build(record))
	}
	return record, ok, nil
}

func (s *storageTransactionalStore) UpsertWithEvent(ctx context.Context, resource, id string, data map[string]any, build platform.RecordEventBuilder) (contracts.Record[map[string]any], error) {
	s.upsertWithEvent++
	payload := cloneStorageTestMap(data)
	payload["id"] = id
	record, ok := s.Store.Update(ctx, resource, id, payload)
	if !ok {
		var err error
		record, err = s.Store.Create(ctx, resource, payload)
		if err != nil {
			return contracts.Record[map[string]any]{}, err
		}
	}
	if build != nil {
		s.txEvents = append(s.txEvents, build(record))
	}
	return record, nil
}

func (s *storageTransactionalStore) DeleteWithEvent(ctx context.Context, resource, id string, build platform.DeleteEventBuilder) (bool, error) {
	s.deleteWithEvent++
	deleted := s.Store.Delete(ctx, resource, id)
	if deleted && build != nil {
		s.txEvents = append(s.txEvents, build(deleted))
	}
	return deleted, nil
}

func (s *storageTransactionalStore) resetEvents() {
	s.createWithEvent = 0
	s.updateWithEvent = 0
	s.upsertWithEvent = 0
	s.deleteWithEvent = 0
	s.txEvents = nil
}

func TestStorageUpsertHandlersUseTransactionalEvents(t *testing.T) {
	app, store := newStorageTransactionalTestApp(t)

	permReq := storageRequest(http.MethodPost, "/api/v1/storage/permissions", `{"group_id":"G1","pvc_id":"pvc1","user_id":"U2","permission":"read_write"}`, "U1")
	code, data, _ := createStoragePermission(app, permReq, platform.RouteSpec{})
	assertStorageStatus(t, code, data, http.StatusOK)
	assertStorageTxEvent(t, app, store, "StoragePermissionChanged")
	if store.upsertWithEvent != 1 {
		t.Fatalf("upsertWithEvent=%d, want 1", store.upsertWithEvent)
	}

	store.resetEvents()
	policyReq := storageRequest(http.MethodPost, "/api/v1/storage/policies", `{"group_id":"G1","pvc_id":"pvc1","default_permission":"read_only"}`, "U1")
	code, data, _ = createStoragePolicy(app, policyReq, platform.RouteSpec{})
	assertStorageStatus(t, code, data, http.StatusOK)
	assertStorageTxEvent(t, app, store, "StoragePolicyChanged")

	store.resetEvents()
	projectReq := storageProjectRequest(http.MethodPut, "/api/v1/projects/P1/storage/bindings/pvc1/permissions", `{"user_id":"U2","permission":"read_only"}`, "U3", "P1")
	projectReq.SetPathValue("pvcId", "pvc1")
	code, data, _ = setProjectBindingPermission(app, projectReq, platform.RouteSpec{})
	assertStorageStatus(t, code, data, http.StatusOK)
	assertStorageTxEvent(t, app, store, "ProjectStoragePermissionChanged")

	store.resetEvents()
	userReq := storageRequest(http.MethodPost, "/api/v1/admin/user-storage/alice/init", `{"size":"25Gi"}`, "ADMIN")
	userReq.SetPathValue("username", "alice")
	code, data, _ = initUserStorage(app, userReq, platform.RouteSpec{})
	assertStorageStatus(t, code, data, http.StatusOK)
	assertStorageTxEvent(t, app, store, "UserStorageChanged")
}

func TestStorageUpdateAndDeleteHandlersUseTransactionalEvents(t *testing.T) {
	app, store := newStorageTransactionalTestApp(t)

	startReq := storagePathRequest(http.MethodPost, "/api/v1/storage/G1/storage/pvc1/start", "", "U2", "G1")
	startReq.SetPathValue("pvcId", "pvc1")
	code, data, _ := startGroupStorage(app, startReq, platform.RouteSpec{})
	assertStorageStatus(t, code, data, http.StatusOK)
	assertStorageTxEvent(t, app, store, "GroupStorageCommanded")
	if store.updateWithEvent != 1 {
		t.Fatalf("updateWithEvent=%d, want 1", store.updateWithEvent)
	}

	store.resetEvents()
	missingStart := storagePathRequest(http.MethodPost, "/api/v1/storage/G1/storage/missing/start", "", "U2", "G1")
	missingStart.SetPathValue("pvcId", "missing")
	code, data, _ = startGroupStorage(app, missingStart, platform.RouteSpec{})
	assertStorageStatus(t, code, data, http.StatusNotFound)
	assertNoStorageTxEvent(t, app, store)

	createStorageRecords(t, app, storagePermissionsResource, []map[string]any{{"id": "G1:pvc1:U2", "group_id": "G1", "pvc_id": "pvc1", "user_id": "U2", "permission": "read_only"}})
	store.resetEvents()
	deletePermReq := storageRequest(http.MethodDelete, "/api/v1/storage/permissions/group/G1/pvc/pvc1/user/U2", "", "U1")
	deletePermReq.SetPathValue("group_id", "G1")
	deletePermReq.SetPathValue("pvc_id", "pvc1")
	deletePermReq.SetPathValue("user_id", "U2")
	code, data, _ = deleteStoragePermission(app, deletePermReq, platform.RouteSpec{})
	assertStorageStatus(t, code, data, http.StatusOK)
	assertStorageTxEvent(t, app, store, "StoragePermissionChanged")

	store.resetEvents()
	code, data, _ = deleteStoragePermission(app, deletePermReq, platform.RouteSpec{})
	assertStorageStatus(t, code, data, http.StatusOK)
	assertNoStorageTxEvent(t, app, store)

	createProjectStorageFixtures(t, app)
	store.resetEvents()
	deleteProjectPermReq := storageProjectRequest(http.MethodDelete, "/api/v1/projects/P1/storage/bindings/pvc1/permissions/U2", "", "U3", "P1")
	deleteProjectPermReq.SetPathValue("pvcId", "pvc1")
	deleteProjectPermReq.SetPathValue("userId", "U2")
	code, data, _ = deleteProjectBindingPermission(app, deleteProjectPermReq, platform.RouteSpec{})
	assertStorageStatus(t, code, data, http.StatusOK)
	assertStorageTxEvent(t, app, store, "ProjectStoragePermissionChanged")

	store.resetEvents()
	code, data, _ = deleteProjectBindingPermission(app, deleteProjectPermReq, platform.RouteSpec{})
	assertStorageStatus(t, code, data, http.StatusOK)
	assertNoStorageTxEvent(t, app, store)

	transfer := map[string]any{"id": fastTransferID("P1", "project-P1", "copy1"), "project_id": "P1", "target_namespace": "project-P1", "name": "copy1", "status": "staged"}
	if _, err := storageRepo(app).CreateFastTransfer(context.Background(), transfer); err != nil {
		t.Fatal(err)
	}
	store.resetEvents()
	cancelReq := storageProjectRequest(http.MethodDelete, "/api/v1/projects/P1/storage/transfers/project-P1/copy1", "", "U3", "P1")
	cancelReq.SetPathValue("targetNamespace", "project-P1")
	cancelReq.SetPathValue("name", "copy1")
	code, data, _ = cancelFastTransfer(app, cancelReq, platform.RouteSpec{})
	assertStorageStatus(t, code, data, http.StatusOK)
	assertStorageTxEvent(t, app, store, "FastTransferChanged")
}

func TestStorageUserBatchInheritsPerItemTransactionalCoupling(t *testing.T) {
	app, store := newStorageTransactionalTestApp(t)

	batchReq := storageRequest(http.MethodPost, "/api/v1/admin/user-storage/batch-init", `{"usernames":["alice","bob"]}`, "ADMIN")
	code, data, _ := batchInitUserStorage(app, batchReq, platform.RouteSpec{})
	assertStorageStatus(t, code, data, http.StatusOK)
	if result := data.(map[string]any); result["succeeded"] != 2 || result["failed"] != 0 {
		t.Fatalf("batch result = %#v, want two successes", result)
	}
	if len(app.Events.Outbox()) != 0 {
		t.Fatalf("app.Events outbox = %#v, want no direct publish", app.Events.Outbox())
	}
	if store.upsertWithEvent != 2 || len(store.txEvents) != 2 {
		t.Fatalf("upsertWithEvent=%d txEvents=%#v, want per-item coupling", store.upsertWithEvent, store.txEvents)
	}
	for _, event := range store.txEvents {
		if event.Name != "UserStorageChanged" {
			t.Fatalf("event = %#v, want UserStorageChanged", event)
		}
	}
}

func TestStoragePermissionBatchesUsePerItemTransactionalCoupling(t *testing.T) {
	app, store := newStorageTransactionalTestApp(t)

	batchReq := storageRequest(http.MethodPost, "/api/v1/storage/permissions/batch", `{"group_id":"G1","items":[{"pvc_id":"pvc1","user_id":"U2","permission":"read_only"},{"pvc_id":"pvc1","user_id":"U3","permission":"read_write"}]}`, "U1")
	code, data, _ := batchStoragePermissions(app, batchReq, false)
	assertStorageStatus(t, code, data, http.StatusOK)
	if result := data.(map[string]any); result["succeeded"] != 2 || result["failed"] != 0 {
		t.Fatalf("storage permission batch = %#v, want two successes", result)
	}
	if store.upsertWithEvent != 2 || len(store.txEvents) != 2 {
		t.Fatalf("upsertWithEvent=%d txEvents=%#v, want two per-item events", store.upsertWithEvent, store.txEvents)
	}
	assertNoDirectStoragePublish(t, app)

	store.resetEvents()
	deleteReq := storageRequest(http.MethodDelete, "/api/v1/storage/permissions/batch", `{"group_id":"G1","items":[{"pvc_id":"pvc1","user_id":"U2"},{"pvc_id":"pvc1","user_id":"missing"}]}`, "U1")
	code, data, _ = batchStoragePermissions(app, deleteReq, true)
	assertStorageStatus(t, code, data, http.StatusOK)
	if result := data.(map[string]any); result["succeeded"] != 2 || result["failed"] != 0 {
		t.Fatalf("storage permission delete batch = %#v, want two successes", result)
	}
	if store.deleteWithEvent != 2 || len(store.txEvents) != 1 {
		t.Fatalf("deleteWithEvent=%d txEvents=%#v, want two attempts and one event", store.deleteWithEvent, store.txEvents)
	}
	assertNoDirectStoragePublish(t, app)

	store.resetEvents()
	projectReq := storageProjectRequest(http.MethodPost, "/api/v1/projects/P1/storage/bindings/pvc1/permissions/batch", `{"items":[{"user_id":"U2","permission":"read_only"},{"user_id":"U3","permission":"read_write"}]}`, "U3", "P1")
	projectReq.SetPathValue("pvcId", "pvc1")
	code, data, _ = batchProjectPermissions(app, projectReq, false)
	assertStorageStatus(t, code, data, http.StatusOK)
	if result := data.(map[string]any); result["succeeded"] != 2 || result["failed"] != 0 {
		t.Fatalf("project permission batch = %#v, want two successes", result)
	}
	if store.upsertWithEvent != 2 || len(store.txEvents) != 2 {
		t.Fatalf("project upsertWithEvent=%d txEvents=%#v, want two per-item events", store.upsertWithEvent, store.txEvents)
	}
	assertNoDirectStoragePublish(t, app)
}

func newStorageTransactionalTestApp(t *testing.T) (*platform.App, *storageTransactionalStore) {
	t.Helper()
	store := &storageTransactionalStore{Store: platform.NewStore()}
	app := platform.NewApp(platform.Config{ServiceName: "all", HTTPAddr: ":0"}, platform.WithStore(store))
	Register(app)
	createStorageRecords(t, app, identityUsersResource, []map[string]any{
		{"id": "ADMIN", "username": "admin", "capabilities": map[string]any{"adminPanel": true}},
		{"id": "U1", "username": "alice"},
		{"id": "U2", "username": "bob"},
		{"id": "U3", "username": "carol"},
	})
	createStorageRecords(t, app, orgProjectsResource, []map[string]any{
		{"id": "P1", "project_name": "vision", "owner_id": "G1"},
	})
	createStorageRecords(t, app, orgProjectMembersResource, []map[string]any{
		{"id": "P1:U3", "project_id": "P1", "user_id": "U3", "role": "manager"},
	})
	createStorageRecords(t, app, orgUserGroupsResource, []map[string]any{
		{"id": "U1:G1", "user_id": "U1", "group_id": "G1", "role": "admin"},
		{"id": "U2:G1", "user_id": "U2", "group_id": "G1", "role": "user"},
	})
	createStorageRecords(t, app, groupStorageResource, []map[string]any{
		{"id": "G1:pvc1", "group_id": "G1", "pvc_id": "pvc1", "name": "datasets", "size": "10Gi", "status": "created"},
	})
	store.resetEvents()
	return app, store
}

func assertNoDirectStoragePublish(t *testing.T, app *platform.App) {
	t.Helper()
	if len(app.Events.Outbox()) != 0 {
		t.Fatalf("app.Events outbox = %#v, want no direct publish", app.Events.Outbox())
	}
}

func assertStorageTxEvent(t *testing.T, app *platform.App, store *storageTransactionalStore, name string) {
	t.Helper()
	if len(app.Events.Outbox()) != 0 {
		t.Fatalf("app.Events outbox = %#v, want no direct publish", app.Events.Outbox())
	}
	if len(store.txEvents) != 1 || store.txEvents[0].Name != name {
		t.Fatalf("tx events = %#v, want one %s", store.txEvents, name)
	}
}

func assertNoStorageTxEvent(t *testing.T, app *platform.App, store *storageTransactionalStore) {
	t.Helper()
	if len(app.Events.Outbox()) != 0 {
		t.Fatalf("app.Events outbox = %#v, want no direct publish", app.Events.Outbox())
	}
	if len(store.txEvents) != 0 {
		t.Fatalf("tx events = %#v, want none", store.txEvents)
	}
}

func cloneStorageTestMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}
