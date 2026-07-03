package storage

import (
	"net/http"
	"strings"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

// pathInternalStorageBuildSourceAccess is the owning-service read contract that
// answers "may this user read build sources from this project's storage?".
// image-registry-service consumes it before accepting a from-storage image
// build, so source access is decided by the storage owner, not by the caller
// re-implementing permission logic.
const pathInternalStorageBuildSourceAccess = "/internal/storage/projects/{project_id}/build-source-access"

type storageBuildSourceAccessResponse struct {
	ProjectID   string `json:"project_id"`
	UserID      string `json:"user_id"`
	StoragePath string `json:"storage_path"`
	Allowed     bool   `json:"allowed"`
	Permission  string `json:"permission"`
	PVCID       string `json:"pvc_id,omitempty"`
}

func resolveStorageBuildSourceAccessContract(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	if status, data, ok := requireStorageServiceAuth(app, r); !ok {
		return status, data, nil
	}
	payload, err := platform.DecodeMapWithError(r)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(msgInvalidRequestBody), nil
	}
	projectID := strings.TrimSpace(r.PathValue("project_id"))
	userID := shared.TextValue(payload, "user_id", "userId")
	storagePath := shared.TextValue(payload, "storage_path", "storagePath")
	if projectID == "" || userID == "" || storagePath == "" {
		return http.StatusUnprocessableEntity, shared.ErrorData("project_id, user_id, and storage_path are required"), nil
	}
	resp := storageBuildSourceAccessResponse{ProjectID: projectID, UserID: userID, StoragePath: storagePath, Permission: "none"}
	// Storage paths live inside the project's bound PVCs; access is granted by
	// the first read-capable effective permission across those bindings. Paths
	// are not modeled per-binding, so the grant is project-storage-scoped.
	for _, binding := range storageRepo(app).ListProjectBindings(r.Context(), projectID) {
		pvcID := text(binding, "pvc_id", "pvcId")
		groupID := text(binding, "group_id", "groupId")
		permission := effectiveStoragePermission(app, r, projectID, groupID, pvcID, userID)
		if storagePermissionAllows(permission, true) {
			resp.Allowed = true
			resp.Permission = normalizePermission(permission)
			resp.PVCID = pvcID
			return http.StatusOK, resp, nil
		}
	}
	return http.StatusOK, resp, nil
}
