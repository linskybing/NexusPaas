package imageregistry

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

// buildContextObjectKeyPrefix namespaces staged build-context archives inside
// the shared object-store bucket. Keys are content-addressed by the archive's
// deterministic digest, so re-uploading identical content is an idempotent
// overwrite and a context_key can be verified against the bytes it names.
const buildContextObjectKeyPrefix = "build-contexts/"

const pathImageBuildContextUpload = "/api/v1/images/build/context"

// uploadBuildContext stages a build-context archive via multipart upload into
// the object store and returns the content-addressed context_key. This is the
// large-context transport; the inline base64 context_archive field remains for
// small payloads. The archive is fully validated (path traversal, zip bombs,
// entry caps) before a single byte is persisted.
func uploadBuildContext(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	userID, status, data, ok := requireUser(r)
	if !ok {
		return status, data, nil
	}
	if app.ObjectStore == nil {
		return http.StatusServiceUnavailable, shared.ErrorData("object store is not configured"), nil
	}
	// Archive validation needs random access (zip central directory), so the
	// bounded body is buffered; multipart still avoids the base64 size penalty
	// and keeps the archive out of the JSON record path.
	r.Body = http.MaxBytesReader(nil, r.Body, int64(maxBuildContextArchiveBytes)+(1<<20))
	if err := r.ParseMultipartForm(4 << 20); err != nil {
		return http.StatusBadRequest, shared.ErrorData("multipart form is invalid or exceeds the archive size limit"), nil
	}
	defer func() {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
	}()
	projectID := strings.TrimSpace(r.FormValue("project_id"))
	if projectID == "" {
		return http.StatusBadRequest, shared.ErrorData("project_id is required"), nil
	}
	if _, status, data, ok := requireProjectManager(app, r, projectID, userID); !ok {
		return status, data, nil
	}
	file, _, err := r.FormFile("context")
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData("context archive file is required"), nil
	}
	defer file.Close()
	archive, err := io.ReadAll(io.LimitReader(file, int64(maxBuildContextArchiveBytes)+1))
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData("context archive could not be read"), nil
	}
	if len(archive) > maxBuildContextArchiveBytes {
		return http.StatusBadRequest, shared.ErrorData("context archive exceeds the size limit"), nil
	}
	info, err := validateBuildContextArchive(archive)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(err.Error()), nil
	}
	key := buildContextObjectKeyPrefix + info.Digest
	if err := app.ObjectStore.PutStream(r.Context(), key, bytes.NewReader(archive), int64(len(archive)), "application/octet-stream"); err != nil {
		return http.StatusInternalServerError, shared.ErrorData("context archive could not be stored"), nil
	}
	return http.StatusCreated, map[string]any{
		"context_key":    key,
		"context_digest": info.Digest,
		"size_bytes":     len(archive),
		"project_id":     projectID,
		"uploaded_by":    userID,
	}, nil
}

// imageBuildContextKeyDigest resolves an optional context_key reference to a
// previously staged archive: the object must exist and its bytes must still
// hash to the digest embedded in the key (fail closed on tampering or loss).
func imageBuildContextKeyDigest(app *platform.App, r *http.Request, payload map[string]any) (string, string, int, any) {
	key := strings.TrimSpace(shared.TextValue(payload, "context_key", "contextKey"))
	if key == "" {
		return "", "", 0, nil
	}
	if !strings.HasPrefix(key, buildContextObjectKeyPrefix) {
		return "", "", http.StatusBadRequest, shared.ErrorData("context_key must reference a staged build context")
	}
	if app.ObjectStore == nil {
		return "", "", http.StatusServiceUnavailable, shared.ErrorData("object store is not configured")
	}
	_, digest, err := stagedBuildContextArchive(r.Context(), app, key)
	if err != nil {
		return "", "", http.StatusBadRequest, shared.ErrorData(err.Error())
	}
	return key, digest, 0, nil
}

func stagedBuildContextArchive(ctx context.Context, app *platform.App, key string) ([]byte, string, error) {
	archive, _, found, err := app.ObjectStore.Get(ctx, key)
	if err != nil {
		return nil, "", fmt.Errorf("staged build context could not be read")
	}
	if !found {
		return nil, "", fmt.Errorf("staged build context was not found")
	}
	info, err := validateBuildContextArchive(archive)
	if err != nil {
		return nil, "", fmt.Errorf("staged build context is invalid: %s", err.Error())
	}
	if buildContextObjectKeyPrefix+info.Digest != key {
		return nil, "", fmt.Errorf("staged build context digest does not match its key")
	}
	return archive, info.Digest, nil
}
