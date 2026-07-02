package imageregistry

import (
	"bytes"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

func multipartContextRequest(t *testing.T, projectID, userID string, archive []byte) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("project_id", projectID); err != nil {
		t.Fatalf("write project_id: %v", err)
	}
	part, err := writer.CreateFormFile("context", "context.tar.gz")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write(archive); err != nil {
		t.Fatalf("write archive: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, pathImageBuildContextUpload, &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if userID != "" {
		req.Header.Set("X-User-ID", userID)
	}
	return req
}

func TestUploadBuildContextStagesArchiveAndBuildReferencesIt(t *testing.T) {
	app := newImageRegistryTestApp(t)
	app.ObjectStore = platform.NewMemoryObjectStore()
	archive := makeTarGz(t, []archiveEntry{{name: "Dockerfile", body: "FROM scratch\n"}})

	code, data, _ := uploadBuildContext(app, multipartContextRequest(t, "P1", "U1", archive), platform.RouteSpec{})
	assertImageStatus(t, code, data, http.StatusCreated)
	payload, ok := data.(map[string]any)
	if !ok {
		t.Fatalf("upload response = %#v, want map", data)
	}
	key, _ := payload["context_key"].(string)
	digest, _ := payload["context_digest"].(string)
	if key == "" || key != buildContextObjectKeyPrefix+digest {
		t.Fatalf("context_key = %q digest = %q, want content-addressed key", key, digest)
	}
	stored, _, found, err := app.ObjectStore.Get(t.Context(), key)
	if err != nil || !found || !bytes.Equal(stored, archive) {
		t.Fatalf("stored archive found=%v err=%v len=%d, want original bytes", found, err, len(stored))
	}

	// identical re-upload is idempotent (same key)
	code, data, _ = uploadBuildContext(app, multipartContextRequest(t, "P1", "U1", archive), platform.RouteSpec{})
	assertImageStatus(t, code, data, http.StatusCreated)

	// a build referencing the staged context records the key + source digest
	body := fmt.Sprintf(`{"id":"ctx-key-build","project_id":"P1","image_reference":"registry.local/team/app:staged","cpu_cores":2,"memory_gib":4,"max_build_seconds":600,"context_key":%q}`, key)
	code, data, _ = startImageBuild(app, imageRequest(http.MethodPost, "/api/v1/images/build", body, "U1"), platform.RouteSpec{})
	assertImageStatus(t, code, data, http.StatusAccepted)
	assertImageMapValue(t, data, "context_key", key)
	assertImageMapValue(t, data, "source_digest", digest)
}

func TestUploadBuildContextRejectsInvalidArchiveAndMissingStore(t *testing.T) {
	app := newImageRegistryTestApp(t)
	code, data, _ := uploadBuildContext(app, multipartContextRequest(t, "P1", "U1", []byte("not-an-archive")), platform.RouteSpec{})
	assertImageStatus(t, code, data, http.StatusServiceUnavailable)

	app.ObjectStore = platform.NewMemoryObjectStore()
	code, data, _ = uploadBuildContext(app, multipartContextRequest(t, "P1", "U1", []byte("not-an-archive")), platform.RouteSpec{})
	assertImageStatus(t, code, data, http.StatusBadRequest)

	// non-manager cannot stage into the project
	code, data, _ = uploadBuildContext(app, multipartContextRequest(t, "P1", "U2", makeTarGz(t, []archiveEntry{{name: "f", body: "x"}})), platform.RouteSpec{})
	assertImageStatus(t, code, data, http.StatusForbidden)
}

func TestImageBuildContextKeyFailsClosed(t *testing.T) {
	app := newImageRegistryTestApp(t)
	app.ObjectStore = platform.NewMemoryObjectStore()

	build := func(key string) (int, any) {
		body := fmt.Sprintf(`{"id":"ctx-bad","project_id":"P1","image_reference":"registry.local/team/app:x","cpu_cores":2,"memory_gib":4,"max_build_seconds":600,"context_key":%q}`, key)
		code, data, _ := startImageBuild(app, imageRequest(http.MethodPost, "/api/v1/images/build", body, "U1"), platform.RouteSpec{})
		return code, data
	}

	if code, data := build("outside/prefix"); code != http.StatusBadRequest {
		t.Fatalf("foreign key status=%d data=%v, want 400", code, data)
	}
	if code, data := build(buildContextObjectKeyPrefix + "missing"); code != http.StatusBadRequest {
		t.Fatalf("missing object status=%d data=%v, want 400", code, data)
	}
	// tampered content: stored bytes do not hash to the digest in the key
	archive := makeTarGz(t, []archiveEntry{{name: "Dockerfile", body: "FROM scratch\n"}})
	if err := app.ObjectStore.Put(t.Context(), buildContextObjectKeyPrefix+"deadbeef", archive, "application/octet-stream"); err != nil {
		t.Fatalf("seed tampered object: %v", err)
	}
	if code, data := build(buildContextObjectKeyPrefix + "deadbeef"); code != http.StatusBadRequest {
		t.Fatalf("tampered digest status=%d data=%v, want 400", code, data)
	}
}

func TestFromStorageBuildDeniedWithoutGrant(t *testing.T) {
	app := newImageRegistryTestApp(t)
	// U2 has no storage permission (policy default only covers via bindings,
	// which grant everyone read; remove the policy to prove the deny path)
	if !app.Store.Delete(t.Context(), "storage-service:storage_access_policies", "G1:PVC1") {
		t.Fatal("failed to remove seeded storage policy")
	}
	body := `{"id":"stor-denied","project_id":"P1","image_reference":"registry.local/team/app:d","cpu_cores":2,"memory_gib":4,"max_build_seconds":600,"storage_path":"images/x.tar"}`
	code, data, _ := startStorageImageBuild(app, imageRequest(http.MethodPost, "/api/v1/images/build/from-storage", body, "U1"), platform.RouteSpec{})
	assertImageStatus(t, code, data, http.StatusForbidden)

	missing := `{"id":"stor-nopath","project_id":"P1","image_reference":"registry.local/team/app:n","cpu_cores":2,"memory_gib":4,"max_build_seconds":600}`
	code, data, _ = startStorageImageBuild(app, imageRequest(http.MethodPost, "/api/v1/images/build/from-storage", missing, "U1"), platform.RouteSpec{})
	assertImageStatus(t, code, data, http.StatusBadRequest)
}
