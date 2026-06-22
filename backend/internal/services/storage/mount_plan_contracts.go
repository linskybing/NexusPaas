package storage

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

const pathInternalStorageMountPlan = "/internal/storage/projects/{project_id}/mount-plan"

type storageMountPlanRequest struct {
	ProjectID string
	UserID    string
	Namespace string
	Mounts    []storageMountPlanRequestMount
}

type storageMountPlanRequestMount struct {
	PVCID     string
	Name      string
	MountPath string
	ReadOnly  bool
	SubPath   string
}

type storageMountPlanResponse struct {
	ProjectID          string                     `json:"project_id"`
	UserID             string                     `json:"user_id"`
	Namespace          string                     `json:"namespace"`
	ManifestMounts     []storageManifestMount     `json:"manifest_mounts"`
	PVCShareOperations []storagePVCShareOperation `json:"pvc_share_operations"`
}

type storageManifestMount struct {
	Name      string `json:"name"`
	ClaimName string `json:"claim_name"`
	MountPath string `json:"mount_path"`
	ReadOnly  bool   `json:"read_only,omitempty"`
	SubPath   string `json:"sub_path,omitempty"`
}

type storagePVCShareOperation struct {
	SourceNamespace string `json:"source_namespace"`
	SourcePVC       string `json:"source_pvc"`
	TargetPVC       string `json:"target_pvc"`
}

func resolveStorageMountPlanContract(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	if status, data, ok := requireStorageServiceAuth(app, r); !ok {
		return status, data, nil
	}
	payload, err := platform.DecodeMapWithError(r)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(msgInvalidRequestBody), nil
	}
	req, err := storageMountPlanRequestFromPayload(strings.TrimSpace(r.PathValue("project_id")), payload)
	if err != nil {
		return http.StatusUnprocessableEntity, shared.ErrorData(err.Error()), nil
	}
	plan, status, err := resolveStorageMountPlan(app, r, req)
	if err != nil {
		return status, shared.ErrorData(err.Error()), nil
	}
	publishMountPlanResolved(app, r, req, plan)
	return http.StatusOK, plan, nil
}

func requireStorageServiceAuth(app *platform.App, r *http.Request) (int, map[string]any, bool) {
	if app.Config.ServiceAPIKey == "" {
		return http.StatusNotFound, shared.ErrorData("not found"), false
	}
	if !app.ServiceRequestAuthorized(r) {
		return http.StatusUnauthorized, shared.ErrorData("service authentication is required"), false
	}
	return 0, nil, true
}

func storageMountPlanRequestFromPayload(projectID string, payload map[string]any) (storageMountPlanRequest, error) {
	req := storageMountPlanRequest{
		ProjectID: projectID,
		UserID:    shared.TextValue(payload, "user_id", "userId"),
		Namespace: shared.TextValue(payload, "namespace", "target_namespace", "targetNamespace"),
	}
	for _, item := range mountPlanPayloadItems(payload) {
		mount := storageMountPlanRequestMount{
			PVCID: shared.FirstNonEmpty(
				shared.TextValue(item, "pvc_id", "pvcId"),
				shared.TextValue(item, "pvc_name", "pvcName"),
				shared.TextValue(item, "claim_name", "claimName"),
				shared.TextValue(item, "target_pvc", "targetPVC"),
				shared.TextValue(item, "source_pvc", "sourcePVC"),
				shared.TextValue(item, "pvc", "PVC"),
			),
			Name:      shared.TextValue(item, "name", "volume_name", "volumeName"),
			MountPath: shared.TextValue(item, "mount_path", "mountPath", "path"),
			ReadOnly:  shared.BoolValue(item, "read_only", "readOnly"),
			SubPath:   shared.TextValue(item, "sub_path", "subPath"),
		}
		if mount.PVCID == "" {
			return storageMountPlanRequest{}, fmt.Errorf("pvc_id is required")
		}
		req.Mounts = append(req.Mounts, mount)
	}
	if req.ProjectID == "" || req.UserID == "" {
		return storageMountPlanRequest{}, fmt.Errorf("project_id and user_id are required")
	}
	if len(req.Mounts) == 0 {
		return req, nil
	}
	return req, nil
}

func mountPlanPayloadItems(payload map[string]any) []map[string]any {
	for _, key := range []string{"mounts", "storage_mounts", "storageMounts", "items"} {
		if raw, ok := payload[key]; ok {
			return mountPlanItems(raw)
		}
	}
	return nil
}

func mountPlanItems(raw any) []map[string]any {
	switch typed := raw.(type) {
	case []map[string]any:
		return append([]map[string]any{}, typed...)
	case []any:
		out := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			if row, ok := item.(map[string]any); ok {
				out = append(out, row)
			}
		}
		return out
	case map[string]any:
		return []map[string]any{typed}
	default:
		return nil
	}
}

func resolveStorageMountPlan(app *platform.App, r *http.Request, req storageMountPlanRequest) (storageMountPlanResponse, int, error) {
	plan := storageMountPlanResponse{ProjectID: req.ProjectID, UserID: req.UserID, Namespace: req.Namespace}
	for _, mount := range req.Mounts {
		binding, ok := findProjectStorageBinding(app, r, req.ProjectID, mount.PVCID)
		if !ok {
			return plan, http.StatusNotFound, fmt.Errorf("storage binding not found")
		}
		groupID := text(binding, "group_id", "groupId")
		source, ok := findGroupStorageSource(app, r, groupID, mount.PVCID)
		if !ok {
			return plan, http.StatusNotFound, fmt.Errorf("group storage source not found")
		}
		if !storageSourceDispatchReady(source) {
			return plan, http.StatusConflict, fmt.Errorf("group storage source is not dispatch-ready")
		}
		permission := effectiveStoragePermission(app, r, req.ProjectID, groupID, mount.PVCID, req.UserID)
		if !storagePermissionAllows(permission, mount.ReadOnly) {
			return plan, http.StatusForbidden, fmt.Errorf("storage permission denied")
		}
		sourcePVC := shared.FirstNonEmpty(text(source, "source_pvc", "sourcePVC"), text(source, "pvc_name", "pvcName"), text(source, "pvc_id", "pvcId"), mount.PVCID)
		targetPVC := shared.FirstNonEmpty(text(binding, "target_pvc", "targetPVC"), text(binding, "pvc_id", "pvcId"), mount.PVCID)
		plan.PVCShareOperations = append(plan.PVCShareOperations, storagePVCShareOperation{
			SourceNamespace: groupStorageNamespace(source, groupID),
			SourcePVC:       sourcePVC,
			TargetPVC:       targetPVC,
		})
		if mount.MountPath != "" {
			plan.ManifestMounts = append(plan.ManifestMounts, storageManifestMount{
				Name:      shared.FirstNonEmpty(mount.Name, targetPVC),
				ClaimName: targetPVC,
				MountPath: mount.MountPath,
				ReadOnly:  mount.ReadOnly,
				SubPath:   mount.SubPath,
			})
		}
	}
	return plan, http.StatusOK, nil
}

func findProjectStorageBinding(app *platform.App, r *http.Request, projectID, pvcID string) (map[string]any, bool) {
	return storageRepo(app).FindProjectStorageBinding(r.Context(), projectID, pvcID)
}

func findGroupStorageSource(app *platform.App, r *http.Request, groupID, pvcID string) (map[string]any, bool) {
	return storageRepo(app).FindGroupStorageSource(r.Context(), groupID, pvcID)
}

func storageSourceDispatchReady(source map[string]any) bool {
	switch strings.ToLower(strings.TrimSpace(text(source, "status"))) {
	case "stopped", "deleted":
		return false
	default:
		return true
	}
}

func groupStorageNamespace(source map[string]any, groupID string) string {
	return shared.FirstNonEmpty(
		text(source, "namespace", "source_namespace", "sourceNamespace", "pvc_namespace", "pvcNamespace"),
		"group-"+groupID+"-storage",
	)
}

func effectiveStoragePermission(app *platform.App, r *http.Request, projectID, groupID, pvcID, userID string) string {
	return storageRepo(app).EffectiveStoragePermission(r.Context(), projectID, groupID, pvcID, userID)
}

func storagePermissionAllows(permission string, readOnly bool) bool {
	permission = normalizePermission(permission)
	if readOnly {
		return permission == "read_only" || permission == "read_write"
	}
	return permission == "read_write"
}

func publishMountPlanResolved(app *platform.App, r *http.Request, req storageMountPlanRequest, plan storageMountPlanResponse) {
	publishEvent(app, r, "StorageMountPlanResolved", map[string]any{
		"action":                "resolved",
		"project_id":            req.ProjectID,
		"user_id":               req.UserID,
		"namespace":             req.Namespace,
		"mount_count":           len(req.Mounts),
		"manifest_mount_count":  len(plan.ManifestMounts),
		"share_operation_count": len(plan.PVCShareOperations),
		"pvc_ids":               requestedPVCIDs(req.Mounts),
		"target_pvcs":           targetPVCs(plan.PVCShareOperations),
	})
}

func requestedPVCIDs(mounts []storageMountPlanRequestMount) []string {
	out := make([]string, 0, len(mounts))
	for _, mount := range mounts {
		if mount.PVCID != "" {
			out = append(out, mount.PVCID)
		}
	}
	return out
}

func targetPVCs(ops []storagePVCShareOperation) []string {
	out := make([]string, 0, len(ops))
	for _, op := range ops {
		if op.TargetPVC != "" {
			out = append(out, op.TargetPVC)
		}
	}
	return out
}
