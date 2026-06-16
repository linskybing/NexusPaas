package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

func TestMediaUploadImageWorkflow(t *testing.T) {
	app := newTestApp()
	body, contentType := multipartUploadBody(t, "figure.png", "image/png", pngBytes())

	rec := requestMultipart(t, app, "/api/v1/uploads/images", body.Bytes(), contentType, nil, http.StatusUnauthorized)
	assertRawError(t, rec, "Unauthorized")

	emptyBody, emptyType := emptyMultipartBody(t)
	rec = requestMultipart(t, app, "/api/v1/uploads/images", emptyBody.Bytes(), emptyType, userHeaders("u1"), http.StatusBadRequest)
	assertRawError(t, rec, "file field is required")

	textBody, textType := multipartUploadBody(t, "note.txt", "text/plain", []byte("hello"))
	rec = requestMultipart(t, app, "/api/v1/uploads/images", textBody.Bytes(), textType, userHeaders("u1"), http.StatusBadRequest)
	assertRawError(t, rec, "unsupported image type")

	mismatchBody, mismatchType := multipartUploadBody(t, "figure.jpg", "image/jpeg", pngBytes())
	rec = requestMultipart(t, app, "/api/v1/uploads/images", mismatchBody.Bytes(), mismatchType, userHeaders("u1"), http.StatusBadRequest)
	assertRawError(t, rec, "image extension does not match content")

	rec = requestMultipart(t, app, "/api/v1/uploads/images", body.Bytes(), contentType, userHeaders("u1"), http.StatusOK)
	uploaded := rawJSONMap(t, rec)
	key, _ := uploaded["key"].(string)
	if key == "" || !strings.HasPrefix(uploaded["url"].(string), "/api/v1/uploads/images/") {
		t.Fatalf("upload response = %#v, want key and image url", uploaded)
	}

	requestRaw(t, app, http.MethodGet, "/api/v1/uploads/images/"+key, nil, http.StatusUnauthorized)
	requestRaw(t, app, http.MethodGet, "/api/v1/uploads/images/../secret.png", userHeaders("u1"), http.StatusBadRequest)
	rec = requestRaw(t, app, http.MethodGet, "/api/v1/uploads/images/"+key, userHeaders("u1"), http.StatusOK)
	if rec.Body.Bytes()[0] != 0x89 || rec.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("served image content-type/body = %q/%v", rec.Header().Get("Content-Type"), rec.Body.Bytes())
	}
	if got := rec.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Fatalf("Cache-Control = %q, want immutable image cache header", got)
	}
}

func TestMediaUploadEvictsOldestImagesWhenBounded(t *testing.T) {
	app := newTestApp()
	keys := []string{}
	for i := 0; i < 70; i++ {
		body, contentType := multipartUploadBody(t, fmt.Sprintf("figure-%d.png", i), "image/png", pngBytes())
		rec := requestMultipart(t, app, "/api/v1/uploads/images", body.Bytes(), contentType, userHeaders("u1"), http.StatusOK)
		keys = append(keys, rawJSONMap(t, rec)["key"].(string))
	}

	requestRaw(t, app, http.MethodGet, "/api/v1/uploads/images/"+keys[0], userHeaders("u1"), http.StatusNotFound)
	requestRaw(t, app, http.MethodGet, "/api/v1/uploads/images/"+keys[len(keys)-1], userHeaders("u1"), http.StatusOK)

	retained := 0
	for _, key := range keys {
		rec := requestRaw(t, app, http.MethodGet, "/api/v1/uploads/images/"+key, userHeaders("u1"), 0)
		if rec.Code == http.StatusOK {
			retained++
		}
	}
	if retained >= len(keys) {
		t.Fatalf("retained %d images, want bounded eviction below %d", retained, len(keys))
	}
}

func TestMediaUploadStatePersistsAcrossAppInstances(t *testing.T) {
	store := platform.NewStore()
	app1 := newTestAppWithStore(store)
	body, contentType := multipartUploadBody(t, "shared.png", "image/png", pngBytes())
	upload := requestMultipart(t, app1, "/api/v1/uploads/images", body.Bytes(), contentType, userHeaders("u1"), http.StatusOK)
	key := rawJSONMap(t, upload)["key"].(string)

	app2 := newTestAppWithStore(store)
	served := requestRaw(t, app2, http.MethodGet, "/api/v1/uploads/images/"+key, userHeaders("u1"), http.StatusOK)
	if served.Header().Get("Content-Type") != "image/png" || !bytes.Equal(served.Body.Bytes(), pngBytes()) {
		t.Fatalf("served persisted image = %q/%v, want uploaded image", served.Header().Get("Content-Type"), served.Body.Bytes())
	}
	if got := len(store.List(httptest.NewRequest(http.MethodGet, "/", nil).Context(), "media-upload-service:uploaded_media")); got != 1 {
		t.Fatalf("stored media records = %d, want 1", got)
	}
}

func requestMultipart(t *testing.T, app http.Handler, path string, body []byte, contentType string, headers map[string]string, want int) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", contentType)
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	app.ServeHTTP(rec, req)
	if want != 0 && rec.Code != want {
		t.Fatalf("POST %s returned %d, want %d: %s", path, rec.Code, want, rec.Body.String())
	}
	return rec
}

func requestRaw(t *testing.T, app http.Handler, method, path string, headers map[string]string, want int) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, nil)
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	app.ServeHTTP(rec, req)
	if want != 0 && rec.Code != want {
		t.Fatalf("%s %s returned %d, want %d: %s", method, path, rec.Code, want, rec.Body.String())
	}
	return rec
}

func rawJSONMap(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

func assertRawError(t *testing.T, rec *httptest.ResponseRecorder, contains string) {
	t.Helper()
	payload := rawJSONMap(t, rec)
	message, _ := payload["error"].(string)
	if !strings.Contains(message, contains) {
		t.Fatalf("error = %q, want containing %q", message, contains)
	}
}

func multipartUploadBody(t *testing.T, filename, contentType string, content []byte) (*bytes.Buffer, string) {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", `form-data; name="file"; filename="`+filename+`"`)
	header.Set("Content-Type", contentType)
	part, err := writer.CreatePart(header)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return body, writer.FormDataContentType()
}

func emptyMultipartBody(t *testing.T) (*bytes.Buffer, string) {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return body, writer.FormDataContentType()
}

func pngBytes() []byte {
	return []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0, 0}
}
