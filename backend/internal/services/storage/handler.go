package storage

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

const (
	serviceName = "storage-service"

	longhornRWXHealthResource = serviceName + ":longhorn_rwx_health"

	storageProjectsResource       = serviceName + ":storage_projects"
	storageProjectMembersResource = serviceName + ":storage_project_members"
	storageUserGroupsResource     = serviceName + ":storage_user_groups"
	storageIdentityUsersResource  = serviceName + ":storage_identity_users"
	storageIdentityRolesResource  = serviceName + ":storage_identity_roles"
	orgProjectsResource           = "org-project-service:projects"
	orgProjectMembersResource     = "org-project-service:project_members"
	orgUserGroupsResource         = "org-project-service:user_groups"
	identityUsersResource         = "identity-service:users"
	identityRolesResource         = "identity-service:roles"

	msgInvalidRequestBody  = "invalid request body"
	msgAdminRequired       = "admin access required"
	msgGroupMemberRequired = "group membership required"
	msgGroupAdminRequired  = "group admin access required"
	msgProjectMember       = "project member access required"
	msgProjectManager      = "project manager access required"
	msgFastTransferMissing = "fast transfer not found"

	pathProjectStorageCacheBinding = "/api/v1/projects/{id}/storage/cache-bindings/{cacheBindingId}"
)

func Register(app *platform.App) {
	app.RegisterRequiredFields(storageProfilesResource, "name", "provider", "tier", "access_mode")
	app.RegisterRequiredFields(cacheBindingsResource, "project_id", "storage_binding_id", "cache_key", "scratch_profile")
	app.RegisterRequiredFields(storageBenchmarkRecordsResource, "storage_profile")
	if err := seedDefaultStorageProfiles(app); err != nil {
		slog.Error("storage profile seed failed", "error", err)
	}
	registerLonghornRWXHealthWorker(app)
	app.RegisterCustomHandler(http.MethodPost, pathInternalStorageMountPlan, resolveStorageMountPlanContract)
	app.RegisterCustomHandler(http.MethodPost, pathInternalStorageDataPlanePlan, resolveStorageDataPlanePlanContract)
	app.RegisterCustomHandler(http.MethodPost, pathInternalFastTransferProgress, updateFastTransferProgress)
	app.RegisterCustomHandler(http.MethodPost, "/api/v1/storage-profiles", createStorageProfile)
	app.RegisterCustomHandler(http.MethodPut, "/api/v1/storage-profiles/{id}", updateStorageProfile)
	app.RegisterCustomHandler(http.MethodDelete, "/api/v1/storage-profiles/{id}", deleteStorageProfile)
	app.RegisterCustomHandler(http.MethodPost, "/api/v1/storage/benchmark-records", createStorageBenchmarkRecord)
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/storage/options", listStorageOptions)
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/admin/group-storage", listAdminGroupStorage)
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/storage/group/{id}", listGroupStorage)
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/storage/my-storages", listMyStorages)
	app.RegisterCustomHandler(http.MethodPost, "/api/v1/storage/{id}/storage", createGroupStorage)
	app.RegisterCustomHandler(http.MethodDelete, "/api/v1/storage/{id}/storage/{pvcId}", deleteGroupStorage)
	app.RegisterCustomHandler(http.MethodPost, "/api/v1/storage/{id}/storage/{pvcId}/start", startGroupStorage)
	app.RegisterCustomHandler(http.MethodDelete, "/api/v1/storage/{id}/storage/{pvcId}/stop", stopGroupStorage)
	app.RegisterCustomHandler(http.MethodPost, "/api/v1/storage/permissions", createStoragePermission)
	app.RegisterCustomHandler(http.MethodPost, "/api/v1/storage/permissions/batch", batchSetStoragePermissions)
	app.RegisterCustomHandler(http.MethodDelete, "/api/v1/storage/permissions/batch", batchDeleteStoragePermissions)
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/storage/permissions/group/{group_id}/pvc/{pvc_id}", getStoragePermission)
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/storage/permissions/group/{group_id}/pvc/{pvc_id}/list", listStoragePermissions)
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/storage/permissions/group/{group_id}/pvc/{pvc_id}/policy", getStoragePolicy)
	app.RegisterCustomHandler(http.MethodDelete, "/api/v1/storage/permissions/group/{group_id}/pvc/{pvc_id}/user/{user_id}", deleteStoragePermission)
	app.RegisterCustomHandler(http.MethodPost, "/api/v1/storage/policies", createStoragePolicy)
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/projects/{id}/storage/bindings", listProjectBindings)
	app.RegisterCustomHandler(http.MethodPost, "/api/v1/projects/{id}/storage/bindings", createProjectBinding)
	app.RegisterCustomHandler(http.MethodDelete, "/api/v1/projects/{id}/storage/bindings/{requestId}", deleteProjectBinding)
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/projects/{id}/storage/cache-bindings", listCacheBindings)
	app.RegisterCustomHandler(http.MethodPost, "/api/v1/projects/{id}/storage/cache-bindings", createCacheBinding)
	app.RegisterCustomHandler(http.MethodGet, pathProjectStorageCacheBinding, getCacheBinding)
	app.RegisterCustomHandler(http.MethodPut, pathProjectStorageCacheBinding, updateCacheBinding)
	app.RegisterCustomHandler(http.MethodDelete, pathProjectStorageCacheBinding, deleteCacheBinding)
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/projects/{id}/storage/bindings/{pvcId}/permissions", listProjectBindingPermissions)
	app.RegisterCustomHandler(http.MethodPut, "/api/v1/projects/{id}/storage/bindings/{pvcId}/permissions", setProjectBindingPermission)
	app.RegisterCustomHandler(http.MethodDelete, "/api/v1/projects/{id}/storage/bindings/{pvcId}/permissions/{userId}", deleteProjectBindingPermission)
	app.RegisterCustomHandler(http.MethodPut, "/api/v1/projects/{id}/storage/bindings/{pvcId}/permissions/batch", batchSetProjectBindingPermissions)
	app.RegisterCustomHandler(http.MethodDelete, "/api/v1/projects/{id}/storage/bindings/{pvcId}/permissions/batch", batchDeleteProjectBindingPermissions)
	app.RegisterCustomHandler(http.MethodPost, "/api/v1/projects/{id}/storage/transfers/fast-stage", startFastTransfer)
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/projects/{id}/storage/transfers/{targetNamespace}/{name}", getFastTransfer)
	app.RegisterCustomHandler(http.MethodDelete, "/api/v1/projects/{id}/storage/transfers/{targetNamespace}/{name}", cancelFastTransfer)
	app.RegisterCustomHandler(http.MethodPost, "/api/v1/admin/user-storage/batch-init", batchInitUserStorage)
	app.RegisterCustomHandler(http.MethodPost, "/api/v1/admin/user-storage/batch-status", batchUserStorageStatus)
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/admin/user-storage/{username}/status", getUserStorageStatus)
	app.RegisterCustomHandler(http.MethodPost, "/api/v1/admin/user-storage/{username}/init", initUserStorage)
	app.RegisterCustomHandler(http.MethodPut, "/api/v1/admin/user-storage/{username}/expand", expandUserStorage)
	app.RegisterCustomHandler(http.MethodDelete, "/api/v1/admin/user-storage/{username}", deleteUserStorage)
}

func listStorageOptions(app *platform.App, _ *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	return http.StatusOK, map[string]any{
		"storage_classes": optionRows(app.Config.StorageClassOptions),
		"access_modes":    []string{"ReadWriteOnce", "ReadWriteMany"},
		"permissions":     []string{"none", "read_only", "read_write"},
	}, nil
}

func listAdminGroupStorage(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	userID, status, data, ok := requireUser(r)
	if !ok {
		return status, data, nil
	}
	if !hasAdminPanel(app, r, userID) {
		return http.StatusForbidden, shared.ErrorData(msgAdminRequired), nil
	}
	return http.StatusOK, storageRepo(app).ListGroupStorage(r.Context()), nil
}

func listGroupStorage(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	userID, status, data, ok := requireUser(r)
	if !ok {
		return status, data, nil
	}
	groupID := groupPathID(r)
	if !canReadGroup(app, r, groupID, userID) {
		return http.StatusForbidden, shared.ErrorData(msgGroupMemberRequired), nil
	}
	return http.StatusOK, groupStorageRows(app, r, groupID), nil
}

func listMyStorages(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	userID, status, data, ok := requireUser(r)
	if !ok {
		return status, data, nil
	}
	rows := make([]map[string]any, 0)
	for _, membership := range userGroupRows(app, r) {
		if text(membership, "user_id", "userId", "uid", "u_id") == userID {
			rows = append(rows, groupStorageRows(app, r, text(membership, "group_id", "groupId", "gid", "g_id"))...)
		}
	}
	sortRows(rows, "group_id", "name", "id")
	return http.StatusOK, rows, nil
}

func createGroupStorage(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	userID, status, data, ok := requireUser(r)
	if !ok {
		return status, data, nil
	}
	if !hasAdminPanel(app, r, userID) {
		return http.StatusForbidden, shared.ErrorData(msgAdminRequired), nil
	}
	payload, err := platform.DecodeMapWithError(r)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(msgInvalidRequestBody), nil
	}
	groupID := groupPathID(r)
	name := shared.FirstNonBlank(shared.TextValue(payload, "name"), shared.TextValue(payload, "pvc_id", "pvcId"))
	if groupID == "" || name == "" {
		return http.StatusBadRequest, shared.ErrorData("group id and storage name are required"), nil
	}
	pvcID := shared.FirstNonBlank(shared.TextValue(payload, "id", "pvc_id", "pvcId"), name)
	now := time.Now().UTC()
	record := map[string]any{
		"id":            groupStorageID(groupID, pvcID),
		"group_id":      groupID,
		"pvc_id":        pvcID,
		"name":          name,
		"size":          shared.FirstNonBlank(shared.TextValue(payload, "size"), "10Gi"),
		"storage_class": shared.TextValue(payload, "storage_class", "storageClass"),
		"status":        "created",
		"created_at":    now,
		"updated_at":    now,
	}
	created, err := storageRepo(app).CreateGroupStorageWithEvent(r.Context(), app, record, func(data map[string]any) contracts.Event {
		return storageEvent(r, "GroupStorageCreated", data)
	})
	if err != nil {
		return http.StatusConflict, shared.ErrorData("group storage already exists"), nil
	}
	return http.StatusCreated, created, nil
}

func deleteGroupStorage(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	userID, status, data, ok := requireUser(r)
	if !ok {
		return status, data, nil
	}
	if !hasAdminPanel(app, r, userID) {
		return http.StatusForbidden, shared.ErrorData(msgAdminRequired), nil
	}
	groupID, pvcID := groupPathID(r), pvcPathID(r)
	var removed bool
	if err := app.WithTx(r.Context(), func(tx platform.StoreTx) error {
		ok, e := storageRepo(app).DeleteGroupStorageCascadeTx(r.Context(), tx, groupID, pvcID)
		if e != nil {
			return e
		}
		removed = ok
		if !ok {
			return nil
		}
		tx.Emit(storageEvent(r, "GroupStorageDeleted", map[string]any{"group_id": groupID, "pvc_id": pvcID}))
		return nil
	}); err != nil {
		return http.StatusInternalServerError, shared.ErrorData("group storage could not be deleted"), nil
	}
	if !removed {
		return http.StatusNotFound, shared.ErrorData("group storage not found"), nil
	}
	return http.StatusOK, nil, nil
}

func startGroupStorage(app *platform.App, r *http.Request, route platform.RouteSpec) (int, any, *platform.Degraded) {
	return commandGroupStorage(app, r, route, "running")
}

func stopGroupStorage(app *platform.App, r *http.Request, route platform.RouteSpec) (int, any, *platform.Degraded) {
	return commandGroupStorage(app, r, route, "stopped")
}

func createStoragePermission(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	userID, status, data, ok := requireUser(r)
	if !ok {
		return status, data, nil
	}
	payload, err := platform.DecodeMapWithError(r)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(msgInvalidRequestBody), nil
	}
	groupID := shared.TextValue(payload, "group_id", "groupId")
	if !canManageGroup(app, r, groupID, userID) {
		return http.StatusForbidden, shared.ErrorData(msgGroupAdminRequired), nil
	}
	record, err := permissionRecord(payload)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(err.Error()), nil
	}
	created, err := storageRepo(app).UpsertStoragePermissionWithEvent(r.Context(), app, record, func(data map[string]any) contracts.Event {
		return storageEvent(r, "StoragePermissionChanged", data)
	})
	if err != nil {
		return http.StatusConflict, shared.ErrorData("storage permission could not be saved"), nil
	}
	return http.StatusOK, created, nil
}

func batchSetStoragePermissions(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	return batchStoragePermissions(app, r, false)
}

func batchDeleteStoragePermissions(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	return batchStoragePermissions(app, r, true)
}

func getStoragePermission(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	userID, status, data, ok := requireUser(r)
	if !ok {
		return status, data, nil
	}
	groupID, pvcID := r.PathValue("group_id"), r.PathValue("pvc_id")
	if !canReadGroup(app, r, groupID, userID) {
		return http.StatusForbidden, shared.ErrorData(msgGroupMemberRequired), nil
	}
	return http.StatusOK, map[string]any{"group_id": groupID, "pvc_id": pvcID, "permissions": permissionsForPVC(app, r, groupID, pvcID)}, nil
}

func listStoragePermissions(app *platform.App, r *http.Request, route platform.RouteSpec) (int, any, *platform.Degraded) {
	return getStoragePermission(app, r, route)
}

func getStoragePolicy(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	userID, status, data, ok := requireUser(r)
	if !ok {
		return status, data, nil
	}
	groupID, pvcID := r.PathValue("group_id"), r.PathValue("pvc_id")
	if !canReadGroup(app, r, groupID, userID) {
		return http.StatusForbidden, shared.ErrorData(msgGroupMemberRequired), nil
	}
	if record, found := storageRepo(app).GetStoragePolicy(r.Context(), groupID, pvcID); found {
		return http.StatusOK, record, nil
	}
	return http.StatusOK, map[string]any{"group_id": groupID, "pvc_id": pvcID, "default_permission": "none"}, nil
}

func deleteStoragePermission(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	userID, status, data, ok := requireUser(r)
	if !ok {
		return status, data, nil
	}
	groupID, pvcID, targetUserID := r.PathValue("group_id"), r.PathValue("pvc_id"), r.PathValue("user_id")
	if !canManageGroup(app, r, groupID, userID) {
		return http.StatusForbidden, shared.ErrorData(msgGroupAdminRequired), nil
	}
	if _, err := storageRepo(app).DeleteStoragePermissionWithEvent(r.Context(), app, groupID, pvcID, targetUserID, func(bool) contracts.Event {
		return storageEvent(r, "StoragePermissionChanged", map[string]any{"group_id": groupID, "pvc_id": pvcID, "user_id": targetUserID, "action": "delete"})
	}); err != nil {
		return http.StatusInternalServerError, shared.ErrorData("storage permission could not be deleted"), nil
	}
	return http.StatusOK, nil, nil
}

func createStoragePolicy(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	userID, status, data, ok := requireUser(r)
	if !ok {
		return status, data, nil
	}
	payload, err := platform.DecodeMapWithError(r)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(msgInvalidRequestBody), nil
	}
	groupID := shared.TextValue(payload, "group_id", "groupId")
	if !canManageGroup(app, r, groupID, userID) {
		return http.StatusForbidden, shared.ErrorData(msgGroupAdminRequired), nil
	}
	pvcID := shared.TextValue(payload, "pvc_id", "pvcId")
	policy := map[string]any{
		"id":                 storagePolicyID(groupID, pvcID),
		"group_id":           groupID,
		"pvc_id":             pvcID,
		"default_permission": normalizePermission(shared.FirstNonBlank(shared.TextValue(payload, "default_permission", "defaultPermission"), "none")),
		"updated_at":         time.Now().UTC(),
	}
	record, err := storageRepo(app).UpsertStoragePolicyWithEvent(r.Context(), app, policy, func(data map[string]any) contracts.Event {
		return storageEvent(r, "StoragePolicyChanged", data)
	})
	if err != nil {
		return http.StatusConflict, shared.ErrorData("storage policy could not be saved"), nil
	}
	return http.StatusOK, record, nil
}

func listProjectBindings(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	userID, status, data, ok := requireUser(r)
	if !ok {
		return status, data, nil
	}
	projectID := projectPathID(r)
	if _, status, data, ok := requireProjectRead(app, r, projectID, userID); !ok {
		return status, data, nil
	}
	return http.StatusOK, storageRepo(app).ListProjectBindings(r.Context(), projectID), nil
}

func createProjectBinding(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	userID, status, data, ok := requireUser(r)
	if !ok {
		return status, data, nil
	}
	projectID := projectPathID(r)
	if _, status, data, ok := requireProjectManager(app, r, projectID, userID); !ok {
		return status, data, nil
	}
	payload, err := platform.DecodeMapWithError(r)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(msgInvalidRequestBody), nil
	}
	groupID := shared.TextValue(payload, "group_id", "groupId")
	pvcID := shared.TextValue(payload, "pvc_id", "pvcId")
	if groupID == "" || pvcID == "" {
		return http.StatusBadRequest, shared.ErrorData("group_id and pvc_id are required"), nil
	}
	record := map[string]any{"id": projectBindingID(projectID, pvcID), "project_id": projectID, "group_id": groupID, "pvc_id": pvcID, "created_by": userID, "created_at": time.Now().UTC()}
	created, err := storageRepo(app).CreateProjectBindingWithEvent(r.Context(), app, record, func(data map[string]any) contracts.Event {
		return storageEvent(r, "ProjectStorageBindingChanged", data)
	})
	if err != nil {
		return http.StatusConflict, shared.ErrorData("storage binding already exists"), nil
	}
	return http.StatusCreated, created, nil
}

func deleteProjectBinding(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	userID, status, data, ok := requireUser(r)
	if !ok {
		return status, data, nil
	}
	projectID := projectPathID(r)
	if _, status, data, ok := requireProjectManager(app, r, projectID, userID); !ok {
		return status, data, nil
	}
	pvcID := shared.FirstNonBlank(r.PathValue("requestId"), r.PathValue("pvcId"))
	var removed bool
	if err := app.WithTx(r.Context(), func(tx platform.StoreTx) error {
		ok, e := storageRepo(app).DeleteProjectBindingCascadeTx(r.Context(), tx, projectID, pvcID)
		if e != nil {
			return e
		}
		removed = ok
		if !ok {
			return nil
		}
		tx.Emit(storageEvent(r, "ProjectStorageBindingChanged", map[string]any{"project_id": projectID, "pvc_id": pvcID, "action": "delete"}))
		return nil
	}); err != nil {
		return http.StatusInternalServerError, shared.ErrorData("storage binding could not be deleted"), nil
	}
	if !removed {
		return http.StatusNotFound, shared.ErrorData("storage binding not found"), nil
	}
	return http.StatusOK, nil, nil
}

func listProjectBindingPermissions(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	userID, status, data, ok := requireUser(r)
	if !ok {
		return status, data, nil
	}
	projectID, pvcID := projectPathID(r), pvcPathID(r)
	if _, status, data, ok := requireProjectRead(app, r, projectID, userID); !ok {
		return status, data, nil
	}
	return http.StatusOK, projectPermissionsForPVC(app, r, projectID, pvcID), nil
}

func setProjectBindingPermission(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	userID, status, data, ok := requireUser(r)
	if !ok {
		return status, data, nil
	}
	projectID, pvcID := projectPathID(r), pvcPathID(r)
	if _, status, data, ok := requireProjectManager(app, r, projectID, userID); !ok {
		return status, data, nil
	}
	payload, err := platform.DecodeMapWithError(r)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(msgInvalidRequestBody), nil
	}
	targetUserID := shared.TextValue(payload, "user_id", "userId")
	permission := normalizePermission(shared.TextValue(payload, "permission"))
	record := projectPermissionRecord(projectID, pvcID, targetUserID, permission)
	created, err := storageRepo(app).UpsertProjectPermissionWithEvent(r.Context(), app, record, func(data map[string]any) contracts.Event {
		return storageEvent(r, "ProjectStoragePermissionChanged", data)
	})
	if err != nil {
		return http.StatusConflict, shared.ErrorData("project storage permission could not be saved"), nil
	}
	return http.StatusOK, created, nil
}

func deleteProjectBindingPermission(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	userID, status, data, ok := requireUser(r)
	if !ok {
		return status, data, nil
	}
	projectID, pvcID, targetUserID := projectPathID(r), pvcPathID(r), r.PathValue("userId")
	if _, status, data, ok := requireProjectManager(app, r, projectID, userID); !ok {
		return status, data, nil
	}
	if _, err := storageRepo(app).DeleteProjectPermissionWithEvent(r.Context(), app, projectID, pvcID, targetUserID, func(bool) contracts.Event {
		return storageEvent(r, "ProjectStoragePermissionChanged", map[string]any{"project_id": projectID, "pvc_id": pvcID, "user_id": targetUserID, "action": "delete"})
	}); err != nil {
		return http.StatusInternalServerError, shared.ErrorData("project storage permission could not be deleted"), nil
	}
	return http.StatusOK, nil, nil
}

func batchSetProjectBindingPermissions(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	return batchProjectPermissions(app, r, false)
}

func batchDeleteProjectBindingPermissions(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	return batchProjectPermissions(app, r, true)
}

func startFastTransfer(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	userID, status, data, ok := requireUser(r)
	if !ok {
		return status, data, nil
	}
	projectID := projectPathID(r)
	if _, status, data, ok := requireProjectManager(app, r, projectID, userID); !ok {
		return status, data, nil
	}
	payload, err := platform.DecodeMapWithError(r)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(msgInvalidRequestBody), nil
	}
	repo := storageRepo(app)
	keyHash, fingerprintHash := fastTransferIdempotencyHashes(r, payload, projectID, userID)
	if existing, found := repo.FindFastTransferByIdempotencyKeyHash(r.Context(), projectID, keyHash); found {
		if text(existing, internalFastTransferIdempotencyFingerprint) != fingerprintHash {
			return http.StatusConflict, shared.ErrorData("idempotency key is already used by a different fast transfer request"), nil
		}
		return http.StatusAccepted, existing, nil
	}
	transfer := fastTransferRecord(projectID, userID, payload, repo, time.Now().UTC())
	if keyHash != "" {
		transfer[internalFastTransferIdempotencyKeyHash] = keyHash
		transfer[internalFastTransferIdempotencyFingerprint] = fingerprintHash
	}
	record, err := repo.CreateFastTransferWithEvent(r.Context(), app, transfer, func(data map[string]any) contracts.Event {
		return storageEvent(r, fastTransferChangedEvent, fastTransferEventPayload(data, "queued"))
	})
	if err != nil {
		return http.StatusConflict, shared.ErrorData("fast transfer already exists"), nil
	}
	publishEvent(app, r, fastTransferQueuedEvent, fastTransferEventPayload(record, "queued"))
	record = dispatchFastTransferMoverJob(r.Context(), app, repo, record, time.Now().UTC())
	return http.StatusAccepted, record, nil
}

func getFastTransfer(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	userID, status, data, ok := requireUser(r)
	if !ok {
		return status, data, nil
	}
	projectID := projectPathID(r)
	if _, status, data, ok := requireProjectRead(app, r, projectID, userID); !ok {
		return status, data, nil
	}
	record, found := storageRepo(app).GetFastTransfer(r.Context(), projectID, r.PathValue("targetNamespace"), r.PathValue("name"))
	if !found {
		return http.StatusNotFound, shared.ErrorData(msgFastTransferMissing), nil
	}
	return http.StatusOK, record, nil
}

func cancelFastTransfer(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	userID, status, data, ok := requireUser(r)
	if !ok {
		return status, data, nil
	}
	projectID := projectPathID(r)
	if _, status, data, ok := requireProjectManager(app, r, projectID, userID); !ok {
		return status, data, nil
	}
	namespace, name := r.PathValue("targetNamespace"), r.PathValue("name")
	repo := storageRepo(app)
	current, found := repo.GetFastTransfer(r.Context(), projectID, namespace, name)
	if !found {
		return http.StatusNotFound, shared.ErrorData(msgFastTransferMissing), nil
	}
	patch, status, err := fastTransferCancelPatch(current, time.Now().UTC())
	if err != nil {
		return status, shared.ErrorData(err.Error()), nil
	}
	updated, ok, err := repo.UpdateFastTransferWithEvent(r.Context(), app, projectID, namespace, name, patch, func(data map[string]any) contracts.Event {
		return storageEvent(r, fastTransferChangedEvent, fastTransferEventPayload(data, "cancelled"))
	})
	if err != nil {
		return http.StatusInternalServerError, shared.ErrorData("fast transfer could not be cancelled"), nil
	}
	if !ok {
		return http.StatusNotFound, shared.ErrorData(msgFastTransferMissing), nil
	}
	return http.StatusOK, updated, nil
}

func updateFastTransferProgress(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	if status, data, ok := requireStorageServiceAuth(app, r); !ok {
		return status, data, nil
	}
	payload, err := platform.DecodeMapWithError(r)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(msgInvalidRequestBody), nil
	}
	projectID := strings.TrimSpace(r.PathValue("project_id"))
	namespace, name := r.PathValue("targetNamespace"), r.PathValue("name")
	repo := storageRepo(app)
	current, found := repo.GetFastTransfer(r.Context(), projectID, namespace, name)
	if !found {
		return http.StatusNotFound, shared.ErrorData(msgFastTransferMissing), nil
	}
	patch, eventName, status, err := fastTransferProgressPatchFromPayload(current, payload, time.Now().UTC())
	if err != nil {
		return status, shared.ErrorData(err.Error()), nil
	}
	updated, ok, err := repo.UpdateFastTransferWithEvent(r.Context(), app, projectID, namespace, name, patch, func(data map[string]any) contracts.Event {
		return storageEvent(r, fastTransferChangedEvent, fastTransferEventPayload(data, "progress"))
	})
	if err != nil {
		return http.StatusInternalServerError, shared.ErrorData("fast transfer could not be updated"), nil
	}
	if !ok {
		return http.StatusNotFound, shared.ErrorData(msgFastTransferMissing), nil
	}
	if eventName != fastTransferChangedEvent {
		publishEvent(app, r, eventName, fastTransferEventPayload(updated, shared.TextValue(updated, "status")))
	}
	return http.StatusOK, updated, nil
}

func batchInitUserStorage(app *platform.App, r *http.Request, route platform.RouteSpec) (int, any, *platform.Degraded) {
	return batchUserStorageCommand(app, r, route, "initialized")
}

func batchUserStorageStatus(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	userID, status, data, ok := requireUser(r)
	if !ok {
		return status, data, nil
	}
	if !hasAdminPanel(app, r, userID) {
		return http.StatusForbidden, shared.ErrorData(msgAdminRequired), nil
	}
	payload, err := platform.DecodeMapWithError(r)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(msgInvalidRequestBody), nil
	}
	rows := make([]map[string]any, 0)
	for _, username := range firstStringSlice(payload, "usernames", "users") {
		rows = append(rows, userStorageStatus(app, r, username))
	}
	return http.StatusOK, rows, nil
}

func getUserStorageStatus(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	userID, status, data, ok := requireUser(r)
	if !ok {
		return status, data, nil
	}
	if !hasAdminPanel(app, r, userID) {
		return http.StatusForbidden, shared.ErrorData(msgAdminRequired), nil
	}
	return http.StatusOK, userStorageStatus(app, r, r.PathValue("username")), nil
}

func initUserStorage(app *platform.App, r *http.Request, route platform.RouteSpec) (int, any, *platform.Degraded) {
	return userStorageCommand(app, r, route, r.PathValue("username"), "initialized")
}

func expandUserStorage(app *platform.App, r *http.Request, route platform.RouteSpec) (int, any, *platform.Degraded) {
	return userStorageCommand(app, r, route, r.PathValue("username"), "expanded")
}

func deleteUserStorage(app *platform.App, r *http.Request, route platform.RouteSpec) (int, any, *platform.Degraded) {
	return userStorageCommand(app, r, route, r.PathValue("username"), "deleted")
}
