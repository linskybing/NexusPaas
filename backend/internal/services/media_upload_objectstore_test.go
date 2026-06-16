package services

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

func newTestAppWithObjectStore(store platform.RecordStore, blobs platform.ObjectStore) *platform.App {
	app := platform.NewApp(
		platform.Config{ServiceName: "all", HTTPAddr: ":0", APIKeys: map[string]bool{"test-key": true}, ExternalURLs: map[string]string{}},
		platform.WithStore(store),
		platform.WithObjectStore(blobs),
	)
	RegisterAll(app)
	return app
}

// When an ObjectStore is configured, the image blob is stored there and the
// RecordStore record holds metadata only (no inline base64 body).
func TestMediaUploadUsesObjectStoreForBlobs(t *testing.T) {
	store := platform.NewStore()
	blobs := platform.NewMemoryObjectStore()
	app := newTestAppWithObjectStore(store, blobs)

	body, contentType := multipartUploadBody(t, "photo.png", "image/png", pngBytes())
	rec := requestMultipart(t, app, "/api/v1/uploads/images", body.Bytes(), contentType, userHeaders("u1"), http.StatusOK)
	key := rawJSONMap(t, rec)["key"].(string)

	ctx := httptest.NewRequest(http.MethodGet, "/", nil).Context()
	records := store.List(ctx, "media-upload-service:uploaded_media")
	if len(records) != 1 {
		t.Fatalf("metadata records = %d, want 1", len(records))
	}
	if _, hasInline := records[0].Data["body_base64"]; hasInline {
		t.Fatalf("metadata record should not carry inline body_base64 when an object store is configured")
	}
	if _, _, found, err := blobs.Get(ctx, key); err != nil || !found {
		t.Fatalf("blob for %q not found in object store (err=%v)", key, err)
	}

	served := requestRaw(t, app, http.MethodGet, "/api/v1/uploads/images/"+key, userHeaders("u1"), http.StatusOK)
	if served.Header().Get("Content-Type") != "image/png" || !bytes.Equal(served.Body.Bytes(), pngBytes()) {
		t.Fatalf("served image = %q/%v, want uploaded png", served.Header().Get("Content-Type"), served.Body.Bytes())
	}
}

// A shared object store + record store makes uploads visible across app
// instances even though blobs no longer live inline in the record store.
func TestMediaUploadObjectStoreVisibleAcrossInstances(t *testing.T) {
	store := platform.NewStore()
	blobs := platform.NewMemoryObjectStore()
	app1 := newTestAppWithObjectStore(store, blobs)

	body, contentType := multipartUploadBody(t, "shared.png", "image/png", pngBytes())
	upload := requestMultipart(t, app1, "/api/v1/uploads/images", body.Bytes(), contentType, userHeaders("u1"), http.StatusOK)
	key := rawJSONMap(t, upload)["key"].(string)

	app2 := newTestAppWithObjectStore(store, blobs)
	served := requestRaw(t, app2, http.MethodGet, "/api/v1/uploads/images/"+key, userHeaders("u1"), http.StatusOK)
	if !bytes.Equal(served.Body.Bytes(), pngBytes()) {
		t.Fatalf("served persisted blob = %v, want uploaded png", served.Body.Bytes())
	}
}
