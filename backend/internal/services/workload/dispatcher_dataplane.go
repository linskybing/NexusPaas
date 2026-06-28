package workload

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/linskybing/nexuspaas/backend/internal/platform/cluster"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const (
	dataPlaneStageInContainerName = "nexuspaas-stage-in"
	// ponytail: keep stage-in as a shell initContainer; FastTransfer v2 owns the real byte-mover.
	dataPlaneStageInImage    = "busybox:1.36"
	checkpointDirEnv         = "CHECKPOINT_DIR"
	checkpointFlushTargetEnv = "NEXUSPAAS_CHECKPOINT_FLUSH_TARGET"
	checkpointWritePolicyEnv = "NEXUSPAAS_CHECKPOINT_WRITE_POLICY"
)

func resolveDispatchDataPlanePlan(ctx context.Context, client dataPlanePlanResolver, job map[string]any, namespace string) (dataPlanePlan, error) {
	spec, ok := dataPlaneSpecFromJob(job)
	if !ok {
		return dataPlanePlan{}, nil
	}
	if client == nil {
		return dataPlanePlan{}, fmt.Errorf("data plane plan client is not configured")
	}
	projectID := shared.TextValue(job, "project_id", "projectId")
	userID := shared.TextValue(job, "user_id", "userId")
	if projectID == "" || userID == "" {
		return dataPlanePlan{}, fmt.Errorf("%w: data_plane requires project_id and user_id", cluster.ErrInvalidManifest)
	}
	return client(ctx, projectID, dataPlanePlanRequest{
		JobID:          shared.TextValue(job, "id", "job_id", "jobId"),
		UserID:         userID,
		Namespace:      namespace,
		DatasetSources: spec.DatasetSources,
		ScratchProfile: spec.ScratchProfile,
		Checkpoint:     spec.Checkpoint,
	})
}

func prepareDataPlaneDispatchResources(plan dataPlanePlan, resources []dispatchResource) ([]dispatchResource, error) {
	if dataPlanePlanEmpty(plan) {
		return resources, nil
	}
	updated := make([]dispatchResource, 0, len(resources))
	applied := false
	for _, resource := range resources {
		next, resourceApplied, err := injectDataPlaneIntoResource(resource, plan)
		if err != nil {
			return nil, err
		}
		applied = applied || resourceApplied
		updated = append(updated, next)
	}
	if !applied {
		return nil, fmt.Errorf("%w: data_plane requires a Pod, Deployment, Job, or VCJob manifest", cluster.ErrInvalidManifest)
	}
	return updated, nil
}

func ensureDispatchDataPlanePVCMounts(ctx context.Context, cl *cluster.Client, plan dataPlanePlan, namespace string) error {
	if err := ensureDispatchDataPlaneScratchPVC(ctx, cl, plan, namespace); err != nil {
		return err
	}
	for _, op := range plan.StageInOperations {
		if op.CacheHit || op.SourceNamespace == "" || op.SourcePVC == "" || op.TargetPVC == "" {
			continue
		}
		if err := cl.EnsurePVCMounted(ctx, op.SourceNamespace, op.SourcePVC, namespace, op.TargetPVC); err != nil {
			return fmt.Errorf("mount data plane pvc %s from %s/%s into %s: %w", op.TargetPVC, op.SourceNamespace, op.SourcePVC, namespace, err)
		}
	}
	return nil
}

func ensureDispatchDataPlaneScratchPVC(ctx context.Context, cl *cluster.Client, plan dataPlanePlan, namespace string) error {
	if plan.Scratch.ClaimName == "" {
		return nil
	}
	return cl.EnsurePVC(ctx, cluster.PVCSpec{
		Namespace:        namespace,
		Name:             plan.Scratch.ClaimName,
		StorageClassName: plan.Scratch.StorageClassName,
		AccessMode:       dataPlaneScratchAccessMode(plan.Scratch.AccessMode),
	})
}

func dataPlaneScratchAccessMode(mode string) corev1.PersistentVolumeAccessMode {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "rwx", strings.ToLower(string(corev1.ReadWriteMany)):
		return corev1.ReadWriteMany
	case "rox", strings.ToLower(string(corev1.ReadOnlyMany)):
		return corev1.ReadOnlyMany
	default:
		return corev1.ReadWriteOnce
	}
}

type dispatchDataPlaneSpec struct {
	DatasetSources []dataPlaneDatasetSource
	ScratchProfile string
	Checkpoint     dataPlaneCheckpointSpec
}

func dataPlaneSpecFromJob(job map[string]any) (dispatchDataPlaneSpec, bool) {
	raw, ok := firstDataPlaneValue(job)
	if !ok {
		return dispatchDataPlaneSpec{}, false
	}
	body, ok := raw.(map[string]any)
	if !ok {
		return dispatchDataPlaneSpec{}, false
	}
	return dispatchDataPlaneSpec{
		DatasetSources: dataPlaneDatasetSourcesFromValue(firstAnyValue(body, "dataset_sources", "datasetSources", "sources")),
		ScratchProfile: shared.TextValue(body, "scratch_profile", "scratchProfile"),
		Checkpoint:     dataPlaneCheckpointFromValue(firstAnyValue(body, "checkpoint")),
	}, true
}

func firstDataPlaneValue(job map[string]any) (any, bool) {
	if value, ok := firstPresent(job, "data_plane", "dataPlane", "DataPlane"); ok {
		return value, true
	}
	payload, ok := firstPresent(job, "submission_payload", "submissionPayload", "SubmissionPayload")
	if !ok {
		return nil, false
	}
	payloadMap, ok := payload.(map[string]any)
	if !ok {
		return nil, false
	}
	return firstPresent(payloadMap, "data_plane", "dataPlane", "DataPlane")
}

func dataPlaneDatasetSourcesFromValue(raw any) []dataPlaneDatasetSource {
	items := storageMountItems(raw)
	out := make([]dataPlaneDatasetSource, 0, len(items))
	seen := map[string]struct{}{}
	for _, item := range items {
		bindingID := shared.FirstNonEmpty(
			shared.TextValue(item, "storage_binding_id", "storageBindingId"),
			shared.TextValue(item, "binding_id", "bindingId"),
			shared.TextValue(item, "pvc_id", "pvcId"),
		)
		if bindingID == "" {
			continue
		}
		key := strings.Join([]string{bindingID, shared.TextValue(item, "cache_key", "cacheKey")}, "\x00")
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, dataPlaneDatasetSource{
			StorageBindingID: bindingID,
			Mode:             shared.FirstNonEmpty(shared.TextValue(item, "mode"), "readOnly"),
			CacheKey:         shared.TextValue(item, "cache_key", "cacheKey"),
		})
	}
	return out
}

func dataPlaneCheckpointFromValue(raw any) dataPlaneCheckpointSpec {
	body, _ := raw.(map[string]any)
	if body == nil {
		return dataPlaneCheckpointSpec{}
	}
	return dataPlaneCheckpointSpec{
		FlushTargetProfile: shared.TextValue(body, "flush_target_profile", "flushTargetProfile"),
		WritePolicy:        shared.TextValue(body, "write_policy", "writePolicy"),
		RetainLocalLastN:   shared.IntValue(body, "retain_local_last_n", "retainLocalLastN"),
	}
}

func firstAnyValue(data map[string]any, keys ...string) any {
	for _, key := range keys {
		if value, ok := data[key]; ok {
			return value
		}
	}
	return nil
}

func dataPlanePlanEmpty(plan dataPlanePlan) bool {
	return plan.Scratch.ClaimName == "" && len(plan.StageInOperations) == 0 && plan.Checkpoint.LocalPath == ""
}

func injectDataPlaneIntoResource(resource dispatchResource, plan dataPlanePlan) (dispatchResource, bool, error) {
	u, err := dispatchObject(resource)
	if err != nil {
		return resource, false, err
	}
	applied := false
	if isVolcanoVCJob(u) {
		taskApplied, err := injectDataPlaneIntoVCJob(u, plan)
		if err != nil {
			return resource, false, err
		}
		applied = taskApplied
	} else {
		for _, path := range podSpecPaths(u.GetKind()) {
			specApplied, err := injectDataPlaneIntoPodSpec(u.Object, path, plan)
			if err != nil {
				return resource, false, err
			}
			applied = applied || specApplied
		}
	}
	if !applied {
		return resource, false, nil
	}
	raw, err := json.Marshal(u.Object)
	if err != nil {
		return resource, false, fmt.Errorf(dispatcherMarshalResourceError, u.GetName(), err)
	}
	resource.Raw = raw
	return resource, true, nil
}

func injectDataPlaneIntoVCJob(u *unstructured.Unstructured, plan dataPlanePlan) (bool, error) {
	tasks, found, _ := unstructured.NestedSlice(u.Object, "spec", "tasks")
	if !found {
		return false, nil
	}
	applied := false
	for i := range tasks {
		task, ok := tasks[i].(map[string]any)
		if !ok {
			continue
		}
		taskApplied, err := injectDataPlaneIntoPodSpec(task, []string{"template", "spec"}, plan)
		if err != nil {
			return false, err
		}
		if taskApplied {
			tasks[i] = task
			applied = true
		}
	}
	if applied {
		_ = unstructured.SetNestedSlice(u.Object, tasks, "spec", "tasks")
	}
	return applied, nil
}

func injectDataPlaneIntoPodSpec(obj map[string]any, path []string, plan dataPlanePlan) (bool, error) {
	scratch := dataPlaneScratchMount(plan)
	if scratch.ClaimName == "" || scratch.MountPath == "" {
		return false, fmt.Errorf("%w: data_plane scratch volume is incomplete", cluster.ErrInvalidManifest)
	}
	if err := ensureStorageVolume(obj, path, scratch); err != nil {
		return false, err
	}
	for _, mount := range dataPlaneStageSourceMounts(plan) {
		if err := ensureStorageVolume(obj, path, mount); err != nil {
			return false, err
		}
	}
	applied, err := ensureDataPlaneAppContainers(obj, path, scratch, plan.Checkpoint)
	if err != nil {
		return false, err
	}
	initApplied, err := ensureDataPlaneStageInInitContainer(obj, path, scratch, plan.StageInOperations)
	if err != nil {
		return false, err
	}
	return applied || initApplied, nil
}

func dataPlaneScratchMount(plan dataPlanePlan) storageMountSpec {
	return storageMountSpec{
		Name:      sanitizeDNSLabel(shared.FirstNonEmpty(plan.Scratch.VolumeName, "nexuspaas-scratch"), "scratch", 63),
		ClaimName: strings.TrimSpace(plan.Scratch.ClaimName),
		MountPath: strings.TrimSpace(plan.Scratch.MountPath),
	}
}

func dataPlaneStageSourceMounts(plan dataPlanePlan) []storageMountSpec {
	mounts := make([]storageMountSpec, 0, len(plan.StageInOperations))
	seen := map[string]struct{}{}
	for _, op := range plan.StageInOperations {
		if op.CacheHit || op.TargetPVC == "" || op.SourcePath == "" {
			continue
		}
		name := sanitizeDNSLabel(shared.FirstNonEmpty(op.VolumeName, "stage-"+op.StorageBindingID), "stage", 63)
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		mounts = append(mounts, storageMountSpec{Name: name, ClaimName: op.TargetPVC, MountPath: op.SourcePath, ReadOnly: true})
	}
	return mounts
}

func ensureDataPlaneAppContainers(obj map[string]any, path []string, scratch storageMountSpec, checkpoint dataPlaneCheckpointPlan) (bool, error) {
	containersPath := childPath(path, "containers")
	containers, found, _ := unstructured.NestedSlice(obj, containersPath...)
	if !found {
		return false, nil
	}
	for i, raw := range containers {
		container, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if err := ensureStorageMountInContainer(container, scratch); err != nil {
			return false, err
		}
		ensureCheckpointEnv(container, checkpoint)
		containers[i] = container
	}
	if err := unstructured.SetNestedSlice(obj, containers, containersPath...); err != nil {
		return false, fmt.Errorf("set data plane containers: %w", err)
	}
	return len(containers) > 0, nil
}

func ensureCheckpointEnv(container map[string]any, checkpoint dataPlaneCheckpointPlan) {
	if checkpoint.LocalPath != "" {
		addNamedItem(container, "env", map[string]any{"name": checkpointDirEnv, "value": checkpoint.LocalPath})
	}
	if checkpoint.FlushTargetPath != "" {
		addNamedItem(container, "env", map[string]any{"name": checkpointFlushTargetEnv, "value": checkpoint.FlushTargetPath})
	}
	if checkpoint.WritePolicy != "" {
		addNamedItem(container, "env", map[string]any{"name": checkpointWritePolicyEnv, "value": checkpoint.WritePolicy})
	}
}

func ensureDataPlaneStageInInitContainer(obj map[string]any, path []string, scratch storageMountSpec, ops []dataPlaneStageInOperation) (bool, error) {
	command := dataPlaneStageInCommand(ops)
	if command == "" {
		return false, nil
	}
	initContainersPath := childPath(path, "initContainers")
	initContainers, _, _ := unstructured.NestedSlice(obj, initContainersPath...)
	if sliceHasNamedItem(initContainers, dataPlaneStageInContainerName) {
		return false, nil
	}
	init := map[string]any{
		"name":         dataPlaneStageInContainerName,
		"image":        dataPlaneStageInImage,
		"command":      []any{"sh", "-c"},
		"args":         []any{command},
		"volumeMounts": dataPlaneStageInVolumeMounts(scratch, ops),
	}
	if len(init["volumeMounts"].([]any)) == 0 {
		return false, nil
	}
	initContainers = append(initContainers, init)
	if err := unstructured.SetNestedSlice(obj, initContainers, initContainersPath...); err != nil {
		return false, fmt.Errorf("set data plane initContainers: %w", err)
	}
	return true, nil
}

func dataPlaneStageInVolumeMounts(scratch storageMountSpec, ops []dataPlaneStageInOperation) []any {
	mounts := []any{storageVolumeMountMap(scratch)}
	seen := map[string]struct{}{scratch.Name: {}}
	for _, op := range ops {
		if op.CacheHit || op.TargetPVC == "" || op.SourcePath == "" {
			continue
		}
		name := sanitizeDNSLabel(shared.FirstNonEmpty(op.VolumeName, "stage-"+op.StorageBindingID), "stage", 63)
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		mounts = append(mounts, storageVolumeMountMap(storageMountSpec{Name: name, MountPath: op.SourcePath, ReadOnly: true}))
	}
	return mounts
}

func dataPlaneStageInCommand(ops []dataPlaneStageInOperation) string {
	lines := []string{"set -e"}
	for _, op := range ops {
		if op.CacheHit || op.SourcePath == "" || op.ScratchPath == "" {
			continue
		}
		source := shellQuote(strings.TrimRight(op.SourcePath, "/") + "/.")
		target := shellQuote(strings.TrimRight(op.ScratchPath, "/") + "/")
		lines = append(lines, "mkdir -p "+shellQuote(op.ScratchPath))
		lines = append(lines, "cp -a "+source+" "+target)
	}
	if len(lines) == 1 {
		return ""
	}
	return strings.Join(lines, "\n")
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
