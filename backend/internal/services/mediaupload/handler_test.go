package mediaupload

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

func TestUploadAndServeImageInline(t *testing.T) {
	app := platform.NewApp(platform.Config{})
	svc := NewService()

	status, data, degraded := svc.uploadImage(app, uploadRequest(t, "figure.png", "image/png", testPNG()), platform.RouteSpec{})
	if degraded != nil || status != http.StatusOK {
		t.Fatalf("upload status=%d degraded=%#v data=%#v", status, degraded, data)
	}
	key := rawMap(t, data)["key"].(string)
	if key == "" {
		t.Fatalf("upload data = %#v, want key", data)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/uploads/images/"+key, nil)
	req.Header.Set("X-User-ID", "U1")
	req.SetPathValue("key", key)
	status, data, degraded = svc.serveImage(app, req, platform.RouteSpec{})
	if degraded != nil || status != http.StatusOK {
		t.Fatalf("serve status=%d degraded=%#v data=%#v", status, degraded, data)
	}
	raw := data.(platform.RawResponse)
	if raw.ContentType != "image/png" || !bytes.Equal(raw.Body, testPNG()) {
		t.Fatalf("served image = %q/%v, want png payload", raw.ContentType, raw.Body)
	}
}

func TestUploadAndServeImageWithObjectStore(t *testing.T) {
	store := platform.NewStore()
	blobs := platform.NewMemoryObjectStore()
	app := platform.NewApp(platform.Config{}, platform.WithStore(store), platform.WithObjectStore(blobs))
	svc := NewService()

	status, data, _ := svc.uploadImage(app, uploadRequest(t, "photo.png", "image/png", testPNG()), platform.RouteSpec{})
	if status != http.StatusOK {
		t.Fatalf("upload status=%d data=%#v", status, data)
	}
	key := rawMap(t, data)["key"].(string)
	records := store.List(httptest.NewRequest(http.MethodGet, "/", nil).Context(), mediaResource)
	if len(records) != 1 {
		t.Fatalf("metadata records = %d, want 1", len(records))
	}
	if _, ok := records[0].Data["body_base64"]; ok {
		t.Fatal("object-store metadata should not contain inline body")
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/uploads/images/"+key, nil)
	req.Header.Set("X-User-ID", "U1")
	req.SetPathValue("key", key)
	status, data, _ = svc.serveImage(app, req, platform.RouteSpec{})
	if status != http.StatusOK {
		t.Fatalf("serve status=%d data=%#v", status, data)
	}
	if !bytes.Equal(data.(platform.RawResponse).Body, testPNG()) {
		t.Fatalf("served body = %v, want png payload", data.(platform.RawResponse).Body)
	}
}

func TestMediaUploadRejectsInvalidRequests(t *testing.T) {
	svc := NewService()
	app := platform.NewApp(platform.Config{})
	Register(app)

	status, data, _ := svc.uploadImage(app, httptest.NewRequest(http.MethodPost, "/upload", nil), platform.RouteSpec{})
	if status != http.StatusUnauthorized || !strings.Contains(string(data.(platform.RawResponse).Body), "Unauthorized") {
		t.Fatalf("unauthorized upload status=%d data=%#v", status, data)
	}
	req := httptest.NewRequest(http.MethodPost, "/upload", strings.NewReader("not multipart"))
	req.Header.Set("X-User-ID", "U1")
	req.Header.Set("Content-Type", "multipart/form-data; boundary=broken")
	status, data, _ = svc.uploadImage(app, req, platform.RouteSpec{})
	if status != http.StatusBadRequest {
		t.Fatalf("malformed multipart status=%d data=%#v", status, data)
	}
	status, data, _ = svc.uploadImage(&platform.App{}, uploadRequest(t, "figure.png", "image/png", testPNG()), platform.RouteSpec{})
	if status != http.StatusServiceUnavailable {
		t.Fatalf("missing store status=%d data=%#v", status, data)
	}
	status, data, _ = svc.uploadImage(app, uploadRequest(t, "note.txt", "text/plain", []byte("hello")), platform.RouteSpec{})
	if status != http.StatusBadRequest {
		t.Fatalf("text upload status=%d data=%#v", status, data)
	}
}

func TestMediaUploadHelperValidation(t *testing.T) {
	if _, err := readImageBody(failingReader{}); err == nil || !isMediaStoreFailure(err) {
		t.Fatalf("readImageBody error = %v, want media store failure", err)
	}
	if _, _, _, err := validateImagePayload("empty.png", "image/png", bytes.NewReader(nil)); err == nil {
		t.Fatal("validateImagePayload accepted empty body")
	}
	if err := validateImageKey("../secret.png"); err == nil {
		t.Fatal("validateImageKey accepted traversal key")
	}
	if err := validateImageKey("valid/key.png"); err != nil {
		t.Fatalf("validateImageKey valid key: %v", err)
	}
	if obj, ok := imageObjectFromInlineMap(map[string]any{"body_base64": "not base64", "content_type": "image/png"}); ok || obj.contentType != "" {
		t.Fatalf("invalid inline image object = %#v ok=%v, want not ok", obj, ok)
	}
	if sanitizeFilename("!!!") != "image" {
		t.Fatalf("sanitizeFilename punctuation fallback failed")
	}
	for _, value := range []any{int(1), int64(2), float64(3), "bad"} {
		_ = int64Value(value)
	}
}

func TestImageMetadataConflictAndNilStores(t *testing.T) {
	app := platform.NewApp(platform.Config{})
	ctx := httptest.NewRequest(http.MethodGet, "/", nil).Context()
	created, err := createImageMetadata(ctx, app.Store, "dup.png", imageObject{contentType: "image/png", body: testPNG(), size: int64(len(testPNG()))}, true)
	if err != nil || !created {
		t.Fatalf("createImageMetadata first created=%v err=%v", created, err)
	}
	created, err = createImageMetadata(ctx, app.Store, "dup.png", imageObject{contentType: "image/png"}, true)
	if err != nil || created {
		t.Fatalf("createImageMetadata duplicate created=%v err=%v, want conflict skip", created, err)
	}
	if err := putImageBlob(ctx, nil, "no-op.png", imageObject{}); err != nil {
		t.Fatalf("nil blob store put: %v", err)
	}
	if store, ok := mediaStore(nil); mediaBlobStore(nil) != nil || store != nil || ok {
		t.Fatal("nil app stores should be unavailable")
	}
}

func TestServeImageErrorBranchesAndEviction(t *testing.T) {
	app := platform.NewApp(platform.Config{}, platform.WithObjectStore(platform.NewMemoryObjectStore()))
	svc := NewService()

	for _, tc := range []struct {
		name string
		key  string
		user string
		want int
	}{
		{name: "empty", key: "", user: "U1", want: http.StatusBadRequest},
		{name: "invalid", key: "../secret.png", user: "U1", want: http.StatusBadRequest},
		{name: "unauthorized", key: "2026/missing.png", want: http.StatusUnauthorized},
		{name: "not_found", key: "2026/missing.png", user: "U1", want: http.StatusNotFound},
	} {
		req := httptest.NewRequest(http.MethodGet, "/images/"+tc.key, nil)
		req.SetPathValue("key", tc.key)
		req.Header.Set("X-User-ID", tc.user)
		status, _, _ := svc.serveImage(app, req, platform.RouteSpec{})
		if status != tc.want {
			t.Fatalf("%s status=%d want %d", tc.name, status, tc.want)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	for i := 0; i < maxStoredImages+2; i++ {
		key := "2026/old_" + string(rune('a'+i)) + ".png"
		obj := imageObject{contentType: "image/png", body: testPNG(), size: int64(len(testPNG()))}
		if _, err := app.Store.Create(req.Context(), mediaResource, imageMetadataToMap(key, obj, false)); err != nil {
			t.Fatal(err)
		}
		if err := app.ObjectStore.Put(req.Context(), key, obj.body, obj.contentType); err != nil {
			t.Fatal(err)
		}
	}
	evictOldestImages(req, app.Store, app.ObjectStore)
	if got := len(app.Store.List(req.Context(), mediaResource)); got != maxStoredImages {
		t.Fatalf("records after eviction = %d, want %d", got, maxStoredImages)
	}
}

func uploadRequest(t *testing.T, filename, contentType string, payload []byte) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition", `form-data; name="file"; filename="`+filename+`"`)
	header.Set("Content-Type", contentType)
	part, err := writer.CreatePart(header)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/uploads/images", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-User-ID", "U1")
	return req
}

func rawMap(t *testing.T, data any) map[string]any {
	t.Helper()
	raw := data.(platform.RawResponse)
	var payload map[string]any
	if err := json.Unmarshal(raw.Body, &payload); err != nil {
		t.Fatal(err)
	}
	return payload
}

func testPNG() []byte {
	return []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) {
	return 0, errors.New("read failed")
}

var _ io.Reader = failingReader{}
