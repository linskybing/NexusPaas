package mediaupload

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

const (
	mediaResource       = "media-upload-service:uploaded_media"
	maxImageBytes       = 10 << 20
	maxStoredImages     = 64
	maxStoredImageBytes = 64 << 20
	uploadStoreError    = "upload: store: %w"
	mimeImageJPEG       = "image/jpeg"
	msgInvalidImageKey  = "invalid image key"
)

var allowedMIME = map[string]string{
	mimeImageJPEG: ".jpg",
	"image/png":   ".png",
	"image/gif":   ".gif",
	"image/webp":  ".webp",
}

type Service struct{}

type imageObject struct {
	contentType string
	body        []byte
	size        int64
}

func Register(app *platform.App) {
	svc := NewService()
	app.RegisterCustomHandler(http.MethodPost, "/api/v1/uploads/images", svc.uploadImage)
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/uploads/images/{key...}", svc.serveImage)
}

func NewService() *Service {
	return &Service{}
}

func (s *Service) uploadImage(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	if currentUserID(r) == "" {
		return rawJSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
	}
	store, ok := mediaStore(app)
	if !ok {
		return rawJSON(http.StatusServiceUnavailable, map[string]string{"error": "media store unavailable"})
	}
	if err := r.ParseMultipartForm(maxImageBytes + 1024); err != nil {
		return rawJSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		return rawJSON(http.StatusBadRequest, map[string]string{"error": "file field is required"})
	}
	defer file.Close()

	key, err := s.storeImage(app, store, r, header.Filename, header.Header.Get("Content-Type"), header.Size, file)
	if err != nil {
		if isMediaStoreFailure(err) {
			return rawJSON(http.StatusInternalServerError, map[string]string{"error": "image could not be stored"})
		}
		return rawJSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	return rawJSON(http.StatusOK, map[string]string{
		"key": key,
		"url": "/api/v1/uploads/images/" + key,
	})
}

func (s *Service) serveImage(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	key := r.PathValue("key")
	if key == "" || key == "/" {
		return rawJSON(http.StatusBadRequest, map[string]string{"error": "key is required"})
	}
	key = strings.TrimPrefix(key, "/")
	if err := validateImageKey(key); err != nil {
		return rawJSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	if currentUserID(r) == "" {
		return rawJSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
	}

	store, ok := mediaStore(app)
	if !ok {
		return rawJSON(http.StatusServiceUnavailable, map[string]string{"error": "media store unavailable"})
	}
	obj, ok := getImageObject(app, r, store, key)
	if !ok {
		return rawJSON(http.StatusNotFound, map[string]string{"error": "image not found"})
	}
	return http.StatusOK, platform.RawResponse{
		ContentType: obj.contentType,
		Headers: map[string]string{
			"Cache-Control": "public, max-age=31536000, immutable",
		},
		Body: append([]byte(nil), obj.body...),
	}, nil
}

func (s *Service) storeImage(app *platform.App, store platform.RecordStore, req *http.Request, filename, contentType string, size int64, r io.Reader) (string, error) {
	if size > maxImageBytes {
		return "", fmt.Errorf("image exceeds 10 MiB limit")
	}
	if size < 0 {
		return "", fmt.Errorf("invalid image size")
	}
	ext, detectedContentType, reader, err := validateImagePayload(filename, contentType, r)
	if err != nil {
		return "", err
	}
	body, err := readImageBody(reader)
	if err != nil {
		return "", err
	}
	obj := imageObject{contentType: detectedContentType, body: body, size: int64(len(body))}
	blobStore := mediaBlobStore(app)
	for attempts := 0; attempts < 3; attempts++ {
		key := buildKey(filename, ext)
		created, err := createImageMetadata(req.Context(), store, key, obj, blobStore == nil)
		if err != nil {
			return "", err
		}
		if !created {
			continue
		}
		if err := putImageBlob(req.Context(), blobStore, key, obj); err != nil {
			store.Delete(req.Context(), mediaResource, key)
			return "", err
		}
		evictOldestImages(req, store, blobStore)
		return key, nil
	}
	return "", fmt.Errorf("could not allocate unique image key")
}

func readImageBody(reader io.Reader) ([]byte, error) {
	body, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf(uploadStoreError, err)
	}
	if int64(len(body)) > maxStoredImageBytes {
		return nil, fmt.Errorf("image exceeds media storage limit")
	}
	return body, nil
}

func createImageMetadata(ctx context.Context, store platform.RecordStore, key string, obj imageObject, inline bool) (bool, error) {
	if _, err := store.Create(ctx, mediaResource, imageMetadataToMap(key, obj, inline)); err != nil {
		if platform.IsCreateConflict(err) {
			return false, nil
		}
		return false, fmt.Errorf(uploadStoreError, err)
	}
	return true, nil
}

func putImageBlob(ctx context.Context, blobStore platform.ObjectStore, key string, obj imageObject) error {
	if blobStore == nil {
		return nil
	}
	if err := blobStore.Put(ctx, key, obj.body, obj.contentType); err != nil {
		return fmt.Errorf(uploadStoreError, err)
	}
	return nil
}

func isMediaStoreFailure(err error) bool {
	if err == nil {
		return false
	}
	message := err.Error()
	return strings.HasPrefix(message, "upload: store:") || strings.Contains(message, "unique image key")
}

func validateImagePayload(filename, contentType string, r io.Reader) (string, string, io.Reader, error) {
	if r == nil {
		return "", "", nil, fmt.Errorf("image payload is required")
	}
	head := make([]byte, 512)
	n, err := io.ReadFull(r, head)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return "", "", nil, fmt.Errorf("cannot read image header: %w", err)
	}
	head = head[:n]
	if n == 0 {
		return "", "", nil, fmt.Errorf("image payload is required")
	}
	detectedContentType := strings.ToLower(http.DetectContentType(head))
	ext, ok := allowedMIME[detectedContentType]
	if !ok {
		return "", "", nil, fmt.Errorf("unsupported image type: %s", contentType)
	}
	if filenameExt := strings.ToLower(path.Ext(filename)); filenameExt != "" {
		if filenameContentType, valid := extensionValid(filenameExt); !valid || filenameContentType != detectedContentType {
			return "", "", nil, fmt.Errorf("image extension does not match content")
		}
	}
	return ext, detectedContentType, io.MultiReader(bytes.NewReader(head), r), nil
}

func buildKey(filename, ext string) string {
	return fmt.Sprintf("%d/%s_%s%s", time.Now().Year(), sanitizeFilename(filename), randomSuffix(), ext)
}

func sanitizeFilename(name string) string {
	base := strings.TrimSuffix(name, path.Ext(name))
	base = strings.ToLower(base)
	var b strings.Builder
	prev := byte('_')
	for i := 0; i < len(base); i++ {
		c := base[i]
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
			b.WriteByte(c)
			prev = c
		} else if prev != '_' {
			b.WriteByte('_')
			prev = '_'
		}
	}
	result := strings.Trim(b.String(), "_")
	if len(result) > 50 {
		result = result[:50]
	}
	if result == "" {
		result = "image"
	}
	return result
}

func extensionValid(ext string) (string, bool) {
	allowed := map[string]string{
		".jpg":  mimeImageJPEG,
		".jpeg": mimeImageJPEG,
		".png":  "image/png",
		".gif":  "image/gif",
		".webp": "image/webp",
	}
	ct, ok := allowed[ext]
	return ct, ok
}

func validateImageKey(key string) error {
	if key == "" || strings.HasPrefix(key, "/") || strings.Contains(key, "\\") {
		return fmt.Errorf(msgInvalidImageKey)
	}
	for _, segment := range strings.Split(key, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return fmt.Errorf(msgInvalidImageKey)
		}
	}
	if _, ok := extensionValid(strings.ToLower(path.Ext(key))); !ok {
		return fmt.Errorf(msgInvalidImageKey)
	}
	return nil
}

func randomSuffix() string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%08x", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

func currentUserID(r *http.Request) string {
	return strings.TrimSpace(r.Header.Get("X-User-ID"))
}

func mediaStore(app *platform.App) (platform.RecordStore, bool) {
	if app == nil || app.Store == nil {
		return nil, false
	}
	return app.Store, true
}

// mediaBlobStore returns the configured ObjectStore for image blobs, or nil when
// none is configured (in which case blobs are kept inline in the RecordStore for
// local no-dependency runs).
func mediaBlobStore(app *platform.App) platform.ObjectStore {
	if app == nil {
		return nil
	}
	return app.ObjectStore
}

// imageMetadataToMap builds the RecordStore metadata record. The blob body is
// stored inline (base64) only when no object store backs the blobs; otherwise the
// bytes live in the ObjectStore and the record holds metadata alone.
func imageMetadataToMap(key string, obj imageObject, inlineBody bool) map[string]any {
	record := map[string]any{
		"id":           key,
		"key":          key,
		"content_type": obj.contentType,
		"size":         obj.size,
	}
	if inlineBody {
		record["body_base64"] = base64.StdEncoding.EncodeToString(obj.body)
	}
	return record
}

func getImageObject(app *platform.App, r *http.Request, store platform.RecordStore, key string) (imageObject, bool) {
	record, ok := store.Get(r.Context(), mediaResource, key)
	if !ok {
		return imageObject{}, false
	}
	meta := imageMetadataFromMap(record.Data)
	if blobStore := mediaBlobStore(app); blobStore != nil {
		body, contentType, found, err := blobStore.Get(r.Context(), key)
		if err != nil || !found {
			return imageObject{}, false
		}
		if contentType != "" {
			meta.contentType = contentType
		}
		meta.body = body
		meta.size = int64(len(body))
		return meta, meta.contentType != ""
	}
	return imageObjectFromInlineMap(record.Data)
}

// imageMetadataFromMap reads the content type and size metadata without requiring
// an inline blob (used on the object-store path).
func imageMetadataFromMap(data map[string]any) imageObject {
	return imageObject{
		contentType: textValue(data["content_type"]),
		size:        int64Value(data["size"]),
	}
}

func imageObjectFromInlineMap(data map[string]any) (imageObject, bool) {
	body, err := base64.StdEncoding.DecodeString(textValue(data["body_base64"]))
	if err != nil {
		return imageObject{}, false
	}
	obj := imageObject{
		contentType: textValue(data["content_type"]),
		body:        body,
		size:        int64Value(data["size"]),
	}
	if obj.size == 0 {
		obj.size = int64(len(body))
	}
	return obj, obj.contentType != "" && obj.size >= 0
}

func evictOldestImages(r *http.Request, store platform.RecordStore, blobStore platform.ObjectStore) {
	records := store.List(r.Context(), mediaResource)
	totalBytes := int64(0)
	for _, record := range records {
		totalBytes += int64Value(record.Data["size"])
	}
	for (len(records) > maxStoredImages || totalBytes > maxStoredImageBytes) && len(records) > 0 {
		record := records[0]
		records = records[1:]
		totalBytes -= int64Value(record.Data["size"])
		store.Delete(r.Context(), mediaResource, record.ID)
		if blobStore != nil {
			blobStore.Delete(r.Context(), record.ID)
		}
	}
}

func textValue(value any) string {
	text, _ := value.(string)
	return text
}

func int64Value(value any) int64 {
	switch typed := value.(type) {
	case int:
		return int64(typed)
	case int64:
		return typed
	case float64:
		return int64(typed)
	default:
		return 0
	}
}

func rawJSON(status int, payload any) (int, platform.RawResponse, *platform.Degraded) {
	body, _ := json.Marshal(payload)
	body = append(body, '\n')
	return status, platform.RawResponse{ContentType: "application/json", Body: body}, nil
}
