package imageregistry

import (
	"context"
	"net/http"
	"testing"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

type imageRegistryTxStore struct {
	*platform.Store
	createWithEvent int
	updateWithEvent int
	upsertWithEvent int
	deleteWithEvent int
	runInTx         int
	txEvents        []contracts.Event
}

func (s *imageRegistryTxStore) CreateWithEvent(ctx context.Context, resource string, data map[string]any, build platform.RecordEventBuilder) (contracts.Record[map[string]any], error) {
	s.createWithEvent++
	record, err := s.Store.Create(ctx, resource, data)
	if err == nil && build != nil {
		s.txEvents = append(s.txEvents, build(record))
	}
	return record, err
}

func (s *imageRegistryTxStore) UpdateWithEvent(ctx context.Context, resource, id string, data map[string]any, build platform.RecordEventBuilder) (contracts.Record[map[string]any], bool, error) {
	s.updateWithEvent++
	record, ok := s.Store.Update(ctx, resource, id, data)
	if ok && build != nil {
		s.txEvents = append(s.txEvents, build(record))
	}
	return record, ok, nil
}

func (s *imageRegistryTxStore) UpsertWithEvent(ctx context.Context, resource, id string, data map[string]any, build platform.RecordEventBuilder) (contracts.Record[map[string]any], error) {
	s.upsertWithEvent++
	payload := cloneImageTxMap(data)
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

func (s *imageRegistryTxStore) DeleteWithEvent(ctx context.Context, resource, id string, build platform.DeleteEventBuilder) (bool, error) {
	s.deleteWithEvent++
	deleted := s.Store.Delete(ctx, resource, id)
	if deleted && build != nil {
		s.txEvents = append(s.txEvents, build(deleted))
	}
	return deleted, nil
}

func (s *imageRegistryTxStore) RunInTx(ctx context.Context, fn func(platform.StoreTx) error) error {
	s.runInTx++
	tx := &imageRegistryRecordingTx{store: s.Store}
	if err := fn(tx); err != nil {
		return err
	}
	s.txEvents = append(s.txEvents, tx.events...)
	return nil
}

func (s *imageRegistryTxStore) resetTx() {
	s.createWithEvent = 0
	s.updateWithEvent = 0
	s.upsertWithEvent = 0
	s.deleteWithEvent = 0
	s.runInTx = 0
	s.txEvents = nil
}

type imageRegistryRecordingTx struct {
	store  *platform.Store
	events []contracts.Event
}

func (tx *imageRegistryRecordingTx) Create(ctx context.Context, resource string, data map[string]any) (contracts.Record[map[string]any], error) {
	return tx.store.Create(ctx, resource, data)
}

func (tx *imageRegistryRecordingTx) Update(ctx context.Context, resource, id string, data map[string]any) (contracts.Record[map[string]any], bool, error) {
	record, ok := tx.store.Update(ctx, resource, id, data)
	return record, ok, nil
}

func (tx *imageRegistryRecordingTx) Delete(ctx context.Context, resource, id string) (bool, error) {
	return tx.store.Delete(ctx, resource, id), nil
}

func (tx *imageRegistryRecordingTx) Emit(event contracts.Event) {
	tx.events = append(tx.events, event)
}

func TestImageRegistryCatalogMutationsUseTransactionalEvents(t *testing.T) {
	app, store := newImageRegistryTxTestApp(t)

	syncReq := imageRequest(http.MethodPost, "/api/v1/image-catalog/sync", `{"tag_id":"tag-1"}`, "U1")
	code, data, _ := syncCatalog(app, syncReq, platform.RouteSpec{})
	assertImageStatus(t, code, data, http.StatusAccepted)
	if store.upsertWithEvent != 1 {
		t.Fatalf("sync upsertWithEvent=%d, want 1", store.upsertWithEvent)
	}
	assertImageTxEvents(t, app, store, "ImageCatalogSyncRequested")

	store.resetTx()
	publishReq := imageRequest(http.MethodPost, "/api/v1/image-catalog/tag-1/publish", `{"project_id":"P1"}`, "ADMIN")
	publishReq.SetPathValue("id", "tag-1")
	code, data, _ = publishCatalog(app, publishReq, platform.RouteSpec{})
	assertImageStatus(t, code, data, http.StatusOK)
	if store.upsertWithEvent != 1 {
		t.Fatalf("publish upsertWithEvent=%d, want 1", store.upsertWithEvent)
	}
	assertImageTxEvents(t, app, store, "ImagePublished")

	store.resetTx()
	unpublishReq := imageRequest(http.MethodPost, "/api/v1/image-catalog/tag-1/unpublish", "", "ADMIN")
	unpublishReq.SetPathValue("id", "tag-1")
	code, data, _ = unpublishCatalog(app, unpublishReq, platform.RouteSpec{})
	assertImageStatus(t, code, data, http.StatusOK)
	if store.runInTx != 1 {
		t.Fatalf("unpublish runInTx=%d, want 1", store.runInTx)
	}
	assertImageTxEvents(t, app, store, "ImageUnpublished")

	createImageRecords(t, app, projectImagesResource, []map[string]any{
		{"id": "P1:tag-1", "project_id": "P1", "tag_id": "tag-1", "enabled": true},
	})
	store.resetTx()
	deleteReq := imageRequest(http.MethodDelete, "/api/v1/image-catalog/tag-1", "", "ADMIN")
	deleteReq.SetPathValue("tagId", "tag-1")
	code, data, _ = deleteCatalogArtifact(app, deleteReq, platform.RouteSpec{})
	assertImageStatus(t, code, data, http.StatusOK)
	if store.runInTx != 1 {
		t.Fatalf("deleteCatalogArtifact runInTx=%d, want 1", store.runInTx)
	}
	assertImageTxEvents(t, app, store, "ImageCatalogDeleted")
	assertImageRecordMissing(t, app, imageCatalogResource, "tag-1")
	assertImageRecordMissing(t, app, projectImagesResource, "P1:tag-1")
}

func newImageRegistryTxTestApp(t *testing.T) (*platform.App, *imageRegistryTxStore) {
	t.Helper()
	store := &imageRegistryTxStore{Store: platform.NewStore()}
	app := platform.NewApp(platform.Config{ServiceName: "all", HTTPAddr: ":0"}, platform.WithStore(store))
	Register(app)
	createImageRecords(t, app, identityUsersResource, []map[string]any{
		{"id": "ADMIN", "username": "admin", "capabilities": map[string]any{"adminPanel": true}},
		{"id": "U1", "username": "alice"},
	})
	createImageRecords(t, app, orgProjectsResource, []map[string]any{
		{"id": "P1", "project_name": "vision", "owner_id": "G1"},
	})
	createImageRecords(t, app, orgUserGroupsResource, []map[string]any{
		{"id": "U1:G1", "user_id": "U1", "group_id": "G1", "role": "admin"},
	})
	createImageRecords(t, app, imageCatalogResource, []map[string]any{
		{"id": "tag-1", "registry": "registry.local", "repository": "library/base", "tag": "1.0"},
	})
	return app, store
}

func assertImageTxEvents(t *testing.T, app *platform.App, store *imageRegistryTxStore, names ...string) {
	t.Helper()
	if got := len(app.Events.Outbox()); got != 0 {
		t.Fatalf("app.Events published %d events, want 0 because events are tx-committed", got)
	}
	if len(store.txEvents) != len(names) {
		t.Fatalf("tx events = %#v, want names %v", store.txEvents, names)
	}
	for i, name := range names {
		if store.txEvents[i].Name != name {
			t.Fatalf("event[%d].Name = %q, want %q; events=%#v", i, store.txEvents[i].Name, name, store.txEvents)
		}
	}
}

func cloneImageTxMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}
