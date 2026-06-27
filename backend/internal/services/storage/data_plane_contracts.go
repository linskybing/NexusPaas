package storage

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

const (
	pathInternalStorageDataPlanePlan = "/internal/storage/projects/{project_id}/data-plane-plan"

	defaultScratchProfileID         = "local-nvme-scratch"
	defaultCheckpointFlushProfileID = "cephfs-rwx-authority"
	defaultScratchMountPath         = "/nexuspaas/scratch"
	defaultStageSourceBasePath      = "/nexuspaas/stage-in"
	defaultCheckpointWritePolicy    = "local-first-async-flush"
	defaultDataPlaneScratchVolume   = "nexuspaas-scratch"
	dataPlanePlanBuiltEvent         = "DataPlanePlanBuilt"
)

type storageDataPlanePlanRequest struct {
	ProjectID       string
	JobID           string
	UserID          string
	Namespace       string
	DatasetSources  []storageDataPlaneDatasetSource
	ScratchProfile  string
	Checkpoint      storageDataPlaneCheckpointRequest
	RetainLocalLast int
}

type storageDataPlaneDatasetSource struct {
	StorageBindingID string
	Mode             string
	CacheKey         string
}

type storageDataPlaneCheckpointRequest struct {
	FlushTargetProfile string
	WritePolicy        string
	RetainLocalLastN   int
}

type storageDataPlanePlan struct {
	ProjectID         string
	JobID             string
	Namespace         string
	Scratch           storageDataPlaneScratchPlan
	StageInOperations []storageDataPlaneStageInOperation
	Checkpoint        storageDataPlaneCheckpointPlan
}

type storageDataPlaneScratchPlan struct {
	ProfileID        string
	StorageClassName string
	VolumeName       string
	ClaimName        string
	MountPath        string
	AccessMode       string
}

type storageDataPlaneStageInOperation struct {
	StorageBindingID string
	CacheKey         string
	CacheHit         bool
	SourceNamespace  string
	SourcePVC        string
	TargetPVC        string
	VolumeName       string
	SourcePath       string
	ScratchPath      string
}

type storageDataPlaneCheckpointPlan struct {
	FlushTargetProfileID string
	StorageClassName     string
	WritePolicy          string
	LocalPath            string
	FlushTargetPath      string
	RetainLocalLastN     int
}

func resolveStorageDataPlanePlanContract(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	if status, data, ok := requireStorageServiceAuth(app, r); !ok {
		return status, data, nil
	}
	payload, err := platform.DecodeMapWithError(r)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(msgInvalidRequestBody), nil
	}
	req, err := storageDataPlanePlanRequestFromPayload(strings.TrimSpace(r.PathValue("project_id")), payload)
	if err != nil {
		return http.StatusUnprocessableEntity, shared.ErrorData(err.Error()), nil
	}
	plan, status, err := resolveStorageDataPlanePlan(r.Context(), app, r, req)
	if err != nil {
		return status, shared.ErrorData(err.Error()), nil
	}
	publishEvent(app, r, dataPlanePlanBuiltEvent, map[string]any{
		"action":               "built",
		"project_id":           plan.ProjectID,
		"job_id":               plan.JobID,
		"namespace":            plan.Namespace,
		"scratch_profile":      plan.Scratch.ProfileID,
		"checkpoint_profile":   plan.Checkpoint.FlushTargetProfileID,
		"dataset_source_count": len(plan.StageInOperations),
	})
	return http.StatusOK, storageDataPlanePlanPayload(plan), nil
}

func storageDataPlanePlanRequestFromPayload(projectID string, payload map[string]any) (storageDataPlanePlanRequest, error) {
	req := storageDataPlanePlanRequest{
		ProjectID:      firstText(projectID, shared.TextValue(payload, "project_id"), shared.TextValue(payload, "projectId")),
		JobID:          firstText(shared.TextValue(payload, "job_id"), shared.TextValue(payload, "jobId")),
		UserID:         firstText(shared.TextValue(payload, "user_id"), shared.TextValue(payload, "userId")),
		Namespace:      firstText(shared.TextValue(payload, "namespace"), shared.TextValue(payload, "target_namespace"), shared.TextValue(payload, "targetNamespace")),
		ScratchProfile: firstText(shared.TextValue(payload, "scratch_profile"), shared.TextValue(payload, "scratchProfile"), defaultScratchProfileID),
		Checkpoint: storageDataPlaneCheckpointRequest{
			FlushTargetProfile: firstText(defaultCheckpointFlushProfileID),
			WritePolicy:        defaultCheckpointWritePolicy,
		},
	}
	if req.ProjectID == "" {
		return req, fmt.Errorf("project_id is required")
	}
	if req.UserID == "" {
		return req, fmt.Errorf("user_id is required")
	}
	req.DatasetSources = storageDataPlaneDatasetSources(payload)
	req.Checkpoint = storageDataPlaneCheckpointFromPayload(payload)
	return req, nil
}

func storageDataPlaneDatasetSources(payload map[string]any) []storageDataPlaneDatasetSource {
	raw := firstAny(payload["dataset_sources"], payload["datasetSources"], payload["sources"])
	items, _ := raw.([]any)
	if len(items) == 0 {
		return nil
	}
	sources := make([]storageDataPlaneDatasetSource, 0, len(items))
	for _, item := range items {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		bindingID := firstText(
			shared.TextValue(entry, "storage_binding_id"),
			shared.TextValue(entry, "storageBindingId"),
			shared.TextValue(entry, "binding_id"),
			shared.TextValue(entry, "bindingId"),
			shared.TextValue(entry, "pvc_id"),
			shared.TextValue(entry, "pvcId"),
		)
		if bindingID == "" {
			continue
		}
		sources = append(sources, storageDataPlaneDatasetSource{
			StorageBindingID: bindingID,
			Mode:             firstText(shared.TextValue(entry, "mode"), "readOnly"),
			CacheKey:         firstText(shared.TextValue(entry, "cache_key"), shared.TextValue(entry, "cacheKey")),
		})
	}
	return sources
}

func storageDataPlaneCheckpointFromPayload(payload map[string]any) storageDataPlaneCheckpointRequest {
	req := storageDataPlaneCheckpointRequest{
		FlushTargetProfile: defaultCheckpointFlushProfileID,
		WritePolicy:        defaultCheckpointWritePolicy,
	}
	raw := firstAny(payload["checkpoint"])
	checkpoint, _ := raw.(map[string]any)
	if checkpoint == nil {
		return req
	}
	req.FlushTargetProfile = firstText(shared.TextValue(checkpoint, "flush_target_profile"), shared.TextValue(checkpoint, "flushTargetProfile"), req.FlushTargetProfile)
	req.WritePolicy = firstText(shared.TextValue(checkpoint, "write_policy"), shared.TextValue(checkpoint, "writePolicy"), req.WritePolicy)
	req.RetainLocalLastN = shared.IntValue(checkpoint, "retain_local_last_n")
	if req.RetainLocalLastN == 0 {
		req.RetainLocalLastN = shared.IntValue(checkpoint, "retainLocalLastN")
	}
	return req
}

func resolveStorageDataPlanePlan(ctx context.Context, app *platform.App, r *http.Request, req storageDataPlanePlanRequest) (storageDataPlanePlan, int, error) {
	scratchProfile, err := getStorageProfilePayload(ctx, app, req.ScratchProfile)
	if err != nil {
		return storageDataPlanePlan{}, http.StatusUnprocessableEntity, err
	}
	checkpointProfile, err := getStorageProfilePayload(ctx, app, req.Checkpoint.FlushTargetProfile)
	if err != nil {
		return storageDataPlanePlan{}, http.StatusUnprocessableEntity, err
	}

	identity := storagePlanName(firstText(req.JobID, req.ProjectID), "job")
	plan := storageDataPlanePlan{
		ProjectID: req.ProjectID,
		JobID:     req.JobID,
		Namespace: req.Namespace,
		Scratch: storageDataPlaneScratchPlan{
			ProfileID:        req.ScratchProfile,
			StorageClassName: shared.TextValue(scratchProfile, "storage_class_name"),
			VolumeName:       defaultDataPlaneScratchVolume,
			ClaimName:        "scratch-" + identity,
			MountPath:        defaultScratchMountPath,
			AccessMode:       shared.TextValue(scratchProfile, "access_mode"),
		},
		Checkpoint: storageDataPlaneCheckpointPlan{
			FlushTargetProfileID: req.Checkpoint.FlushTargetProfile,
			StorageClassName:     shared.TextValue(checkpointProfile, "storage_class_name"),
			WritePolicy:          req.Checkpoint.WritePolicy,
			LocalPath:            defaultScratchMountPath + "/checkpoints",
			FlushTargetPath:      "/checkpoints/" + identity,
			RetainLocalLastN:     req.Checkpoint.RetainLocalLastN,
		},
	}

	for _, source := range req.DatasetSources {
		op, status, err := resolveStorageDataPlaneStageIn(app, r, req, source)
		if err != nil {
			return storageDataPlanePlan{}, status, err
		}
		plan.StageInOperations = append(plan.StageInOperations, op)
	}
	return plan, http.StatusOK, nil
}

func resolveStorageDataPlaneStageIn(app *platform.App, r *http.Request, req storageDataPlanePlanRequest, source storageDataPlaneDatasetSource) (storageDataPlaneStageInOperation, int, error) {
	binding, ok := findProjectStorageBinding(app, r, req.ProjectID, source.StorageBindingID)
	if !ok {
		return storageDataPlaneStageInOperation{}, http.StatusNotFound, fmt.Errorf("storage binding not found")
	}
	groupID := text(binding, "group_id", "groupId")
	pvcID := shared.FirstNonEmpty(text(binding, "source_pvc_id", "sourcePvcId"), text(binding, "pvc_id", "pvcId"), source.StorageBindingID)
	if groupID == "" || pvcID == "" {
		return storageDataPlaneStageInOperation{}, http.StatusNotFound, fmt.Errorf("binding %s has no source group PVC", source.StorageBindingID)
	}
	sourcePVC, ok := findGroupStorageSource(app, r, groupID, pvcID)
	if !ok {
		return storageDataPlaneStageInOperation{}, http.StatusNotFound, fmt.Errorf("group storage source not found")
	}
	if !storageSourceDispatchReady(sourcePVC) {
		return storageDataPlaneStageInOperation{}, http.StatusConflict, fmt.Errorf("group storage source is not dispatch-ready")
	}
	permission := effectiveStoragePermission(app, r, req.ProjectID, groupID, pvcID, req.UserID)
	if !storagePermissionAllows(permission, true) {
		return storageDataPlaneStageInOperation{}, http.StatusForbidden, fmt.Errorf("storage permission denied")
	}
	targetPVC := shared.FirstNonEmpty(text(binding, "target_pvc", "targetPVC"), text(binding, "pvc_id", "pvcId"), source.StorageBindingID)
	label := storagePlanName(firstText(source.CacheKey, source.StorageBindingID), "dataset")
	return storageDataPlaneStageInOperation{
		StorageBindingID: source.StorageBindingID,
		CacheKey:         source.CacheKey,
		CacheHit:         false,
		SourceNamespace:  groupStorageNamespace(sourcePVC, groupID),
		SourcePVC:        shared.FirstNonEmpty(text(sourcePVC, "source_pvc", "sourcePVC"), text(sourcePVC, "pvc_name", "pvcName"), text(sourcePVC, "pvc_id", "pvcId"), pvcID),
		TargetPVC:        targetPVC,
		VolumeName:       "stage-" + label,
		SourcePath:       defaultStageSourceBasePath + "/" + label,
		ScratchPath:      defaultScratchMountPath + "/datasets/" + label,
	}, http.StatusOK, nil
}

func getStorageProfilePayload(ctx context.Context, app *platform.App, profileID string) (map[string]any, error) {
	if strings.TrimSpace(profileID) == "" {
		return nil, fmt.Errorf("storage profile is required")
	}
	rec, ok := app.Store.Get(ctx, storageProfilesResource, profileID)
	if !ok {
		return nil, fmt.Errorf("storage profile %q not found", profileID)
	}
	return rec.Data, nil
}

func storageDataPlanePlanPayload(plan storageDataPlanePlan) map[string]any {
	stageOps := make([]any, 0, len(plan.StageInOperations))
	for _, op := range plan.StageInOperations {
		stageOps = append(stageOps, map[string]any{
			"storage_binding_id": op.StorageBindingID,
			"cache_key":          op.CacheKey,
			"cache_hit":          op.CacheHit,
			"source_namespace":   op.SourceNamespace,
			"source_pvc":         op.SourcePVC,
			"target_pvc":         op.TargetPVC,
			"volume_name":        op.VolumeName,
			"source_path":        op.SourcePath,
			"scratch_path":       op.ScratchPath,
		})
	}
	return map[string]any{
		"project_id": plan.ProjectID,
		"job_id":     plan.JobID,
		"namespace":  plan.Namespace,
		"scratch": map[string]any{
			"profile_id":         plan.Scratch.ProfileID,
			"storage_class_name": plan.Scratch.StorageClassName,
			"volume_name":        plan.Scratch.VolumeName,
			"claim_name":         plan.Scratch.ClaimName,
			"mount_path":         plan.Scratch.MountPath,
			"access_mode":        plan.Scratch.AccessMode,
		},
		"stage_in_operations": stageOps,
		"checkpoint": map[string]any{
			"flush_target_profile_id": plan.Checkpoint.FlushTargetProfileID,
			"storage_class_name":      plan.Checkpoint.StorageClassName,
			"write_policy":            plan.Checkpoint.WritePolicy,
			"local_path":              plan.Checkpoint.LocalPath,
			"flush_target_path":       plan.Checkpoint.FlushTargetPath,
			"retain_local_last_n":     plan.Checkpoint.RetainLocalLastN,
		},
	}
}

func firstText(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func firstAny(values ...any) any {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func storagePlanName(value, fallback string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		valid := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if valid {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	name := strings.Trim(b.String(), "-")
	if name == "" {
		name = fallback
	}
	if len(name) > 63 {
		name = strings.Trim(name[:63], "-")
	}
	return name
}
