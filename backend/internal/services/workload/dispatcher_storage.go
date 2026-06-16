package workload

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/linskybing/nexuspaas/backend/internal/platform/cluster"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type storageMountSpec struct {
	Name            string
	ClaimName       string
	MountPath       string
	ReadOnly        bool
	SubPath         string
	SourceNamespace string
	SourcePVC       string
	TargetPVC       string
}

func prepareStorageMountDispatchResources(plan storageMountPlan, resources []dispatchResource) ([]dispatchResource, error) {
	mounts := storageMountSpecsFromPlan(plan)
	if len(mounts) == 0 {
		return resources, nil
	}
	mounts = manifestStorageMounts(mounts)
	if len(mounts) == 0 {
		return resources, nil
	}
	updated := make([]dispatchResource, 0, len(resources))
	applied := false
	for _, resource := range resources {
		next, resourceApplied, err := injectStorageMountsIntoResource(resource, mounts)
		if err != nil {
			return nil, err
		}
		applied = applied || resourceApplied
		updated = append(updated, next)
	}
	if !applied {
		return nil, fmt.Errorf("%w: storage mounts require a Pod, Deployment, Job, or VCJob manifest", cluster.ErrInvalidManifest)
	}
	return updated, nil
}

func ensureDispatchPVCMounts(ctx context.Context, cl *cluster.Client, plan storageMountPlan, namespace string) error {
	for _, op := range pvcMountOpsFromPlan(plan) {
		if err := cl.EnsurePVCMounted(ctx, op.sourceNamespace, op.sourcePVC, namespace, op.targetPVC); err != nil {
			return fmt.Errorf("mount pvc %s from %s/%s into %s: %w", op.targetPVC, op.sourceNamespace, op.sourcePVC, namespace, err)
		}
	}
	return nil
}

func resolveDispatchStorageMountPlan(ctx context.Context, client storageMountPlanClient, job map[string]any, namespace string) (storageMountPlan, error) {
	selectors, err := storageMountPlanSelectorsFromJob(job)
	if err != nil {
		return storageMountPlan{}, err
	}
	if len(selectors) == 0 {
		return storageMountPlan{}, nil
	}
	if client == nil {
		return storageMountPlan{}, fmt.Errorf("storage mount plan client is not configured")
	}
	projectID := shared.TextValue(job, "project_id", "projectId")
	userID := shared.TextValue(job, "user_id", "userId")
	if projectID == "" || userID == "" {
		return storageMountPlan{}, fmt.Errorf("%w: storage mounts require project_id and user_id", cluster.ErrInvalidManifest)
	}
	return client.Resolve(ctx, projectID, storageMountPlanRequest{UserID: userID, Namespace: namespace, Mounts: selectors})
}

func storageMountPlanSelectorsFromJob(job map[string]any) ([]storageMountPlanSelector, error) {
	raw, ok := firstStorageMountValue(job)
	if !ok {
		return nil, nil
	}
	items := storageMountItems(raw)
	mounts := make([]storageMountPlanSelector, 0, len(items))
	seen := map[string]struct{}{}
	for _, item := range items {
		mount, ok, err := storageMountPlanSelectorFromMap(item)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		key := strings.Join([]string{
			mount.PVCID,
			mount.Name,
			mount.MountPath,
		}, "\x00")
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		mounts = append(mounts, mount)
	}
	return mounts, nil
}

func storageMountSpecsFromPlan(plan storageMountPlan) []storageMountSpec {
	mounts := make([]storageMountSpec, 0, len(plan.ManifestMounts))
	for _, mount := range plan.ManifestMounts {
		claimName := strings.TrimSpace(mount.ClaimName)
		if claimName == "" || strings.TrimSpace(mount.MountPath) == "" {
			continue
		}
		name := sanitizeDNSLabel(shared.FirstNonEmpty(mount.Name, claimName), "storage", 63)
		mounts = append(mounts, storageMountSpec{
			Name:      name,
			ClaimName: claimName,
			MountPath: strings.TrimSpace(mount.MountPath),
			ReadOnly:  mount.ReadOnly,
			SubPath:   strings.TrimSpace(mount.SubPath),
		})
	}
	return mounts
}

func firstStorageMountValue(job map[string]any) (any, bool) {
	keys := []string{
		"storage_mounts", "storageMounts", "StorageMounts",
		"volume_mounts", "volumeMounts", "VolumeMounts",
		"pvc_mounts", "pvcMounts", "PVCMounts",
	}
	if value, ok := firstPresent(job, keys...); ok {
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
	return firstPresent(payloadMap, keys...)
}

func storageMountItems(value any) []map[string]any {
	switch typed := value.(type) {
	case []map[string]any:
		return append([]map[string]any{}, typed...)
	case []any:
		out := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			if data, ok := item.(map[string]any); ok {
				out = append(out, data)
			}
		}
		return out
	case map[string]any:
		for _, key := range []string{"items", "mounts", "storage_mounts", "storageMounts"} {
			if nested, ok := typed[key]; ok {
				return storageMountItems(nested)
			}
		}
		return []map[string]any{typed}
	case string:
		var decoded any
		if json.Unmarshal([]byte(strings.TrimSpace(typed)), &decoded) == nil {
			return storageMountItems(decoded)
		}
	}
	return nil
}

func storageMountSpecFromMap(data map[string]any) (storageMountSpec, bool, error) {
	fields := storageMountFieldsFromMap(data)
	if !fields.present() {
		return storageMountSpec{}, false, nil
	}
	if err := fields.validate(); err != nil {
		return storageMountSpec{}, false, err
	}
	if fields.operationOnly() {
		return storageMountSpec{SourceNamespace: fields.sourceNamespace, SourcePVC: fields.sourcePVC, TargetPVC: fields.targetPVC}, true, nil
	}
	name := shared.FirstNonEmpty(
		shared.TextValue(data, "name", "Name"),
		shared.TextValue(data, "volume_name", "volumeName", "VolumeName"),
		fields.claimName,
	)
	return storageMountSpec{
		Name:            sanitizeDNSLabel(name, "storage", 63),
		ClaimName:       fields.claimName,
		MountPath:       fields.mountPath,
		ReadOnly:        shared.BoolValue(data, "read_only", "readOnly", "ReadOnly"),
		SubPath:         shared.TextValue(data, "sub_path", "subPath", "SubPath"),
		SourceNamespace: fields.sourceNamespace,
		SourcePVC:       fields.sourcePVC,
		TargetPVC:       fields.targetPVC,
	}, true, nil
}

func storageMountPlanSelectorFromMap(data map[string]any) (storageMountPlanSelector, bool, error) {
	fields := storageMountFieldsFromMap(data)
	if !fields.present() {
		return storageMountPlanSelector{}, false, nil
	}
	pvcID := shared.FirstNonEmpty(
		shared.TextValue(data, "pvc_id", "pvcId"),
		shared.TextValue(data, "pvc_name", "pvcName", "PVCName"),
		shared.TextValue(data, "claim_name", "claimName", "ClaimName"),
		fields.targetPVC,
		fields.sourcePVC,
		shared.TextValue(data, "pvc", "PVC"),
	)
	if pvcID == "" {
		return storageMountPlanSelector{}, false, fmt.Errorf("%w: storage mount requires pvc_id", cluster.ErrInvalidManifest)
	}
	return storageMountPlanSelector{
		PVCID:     pvcID,
		Name:      shared.TextValue(data, "name", "Name", "volume_name", "volumeName"),
		MountPath: fields.mountPath,
		ReadOnly:  shared.BoolValue(data, "read_only", "readOnly", "ReadOnly"),
		SubPath:   shared.TextValue(data, "sub_path", "subPath", "SubPath"),
	}, true, nil
}

type storageMountFields struct {
	sourceNamespace string
	sourcePVCRaw    string
	sourcePVC       string
	targetPVC       string
	claimName       string
	mountPath       string
}

func storageMountFieldsFromMap(data map[string]any) storageMountFields {
	sourceNamespace := shared.TextValue(data,
		"source_namespace", "sourceNamespace", "SourceNamespace",
		"source_ns", "sourceNs", "SourceNS",
	)
	sourcePVCRaw := shared.FirstNonEmpty(
		shared.TextValue(data, "source_pvc", "sourcePVC", "SourcePVC"),
		shared.TextValue(data, "source_pvc_name", "sourcePVCName", "SourcePVCName"),
		shared.TextValue(data, "source_claim_name", "sourceClaimName", "SourceClaimName"),
	)
	targetPVC := shared.FirstNonEmpty(
		shared.TextValue(data, "target_pvc", "targetPVC", "TargetPVC"),
		shared.TextValue(data, "target_pvc_name", "targetPVCName", "TargetPVCName"),
		shared.TextValue(data, "target_claim_name", "targetClaimName", "TargetClaimName"),
	)
	claimName := shared.FirstNonEmpty(
		shared.TextValue(data, "claim_name", "claimName", "ClaimName"),
		shared.TextValue(data, "pvc_name", "pvcName", "PVCName"),
		targetPVC,
		sourcePVCRaw,
		shared.TextValue(data, "pvc", "PVC"),
	)
	sourcePVC := sourcePVCRaw
	if sourcePVC == "" && sourceNamespace != "" {
		sourcePVC = claimName
	}
	if targetPVC == "" && (sourceNamespace != "" || sourcePVCRaw != "") {
		targetPVC = shared.FirstNonEmpty(claimName, sourcePVC)
	}
	if targetPVC != "" {
		claimName = targetPVC
	}
	return storageMountFields{
		sourceNamespace: sourceNamespace,
		sourcePVCRaw:    sourcePVCRaw,
		sourcePVC:       sourcePVC,
		targetPVC:       targetPVC,
		claimName:       claimName,
		mountPath:       shared.TextValue(data, "mount_path", "mountPath", "MountPath", "path", "Path"),
	}
}

func (f storageMountFields) hasSharePlan() bool {
	return f.sourceNamespace != "" || f.sourcePVCRaw != ""
}

func (f storageMountFields) present() bool {
	return f.claimName != "" || f.mountPath != "" || f.hasSharePlan()
}

func (f storageMountFields) operationOnly() bool {
	return f.mountPath == "" && f.hasSharePlan()
}

func (f storageMountFields) validate() error {
	if f.hasSharePlan() && (f.sourceNamespace == "" || f.sourcePVC == "" || f.targetPVC == "") {
		return fmt.Errorf("%w: storage PVC share requires source namespace, source PVC and target PVC", cluster.ErrInvalidManifest)
	}
	if f.operationOnly() {
		return nil
	}
	if f.claimName == "" || f.mountPath == "" {
		return fmt.Errorf("%w: storage mount requires claim name and mount path", cluster.ErrInvalidManifest)
	}
	return nil
}

type pvcMountOp struct {
	sourceNamespace string
	sourcePVC       string
	targetPVC       string
}

func manifestStorageMounts(mounts []storageMountSpec) []storageMountSpec {
	out := make([]storageMountSpec, 0, len(mounts))
	for _, mount := range mounts {
		if mount.MountPath != "" {
			out = append(out, mount)
		}
	}
	return out
}

func pvcMountOpsFromSpecs(mounts []storageMountSpec) []pvcMountOp {
	out := []pvcMountOp{}
	seen := map[string]struct{}{}
	for _, mount := range mounts {
		if mount.SourceNamespace == "" || mount.SourcePVC == "" || mount.TargetPVC == "" {
			continue
		}
		key := mount.SourceNamespace + "/" + mount.SourcePVC + ">" + mount.TargetPVC
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, pvcMountOp{sourceNamespace: mount.SourceNamespace, sourcePVC: mount.SourcePVC, targetPVC: mount.TargetPVC})
	}
	return out
}

func pvcMountOpsFromPlan(plan storageMountPlan) []pvcMountOp {
	out := []pvcMountOp{}
	seen := map[string]struct{}{}
	for _, op := range plan.PVCShareOperations {
		if op.SourceNamespace == "" || op.SourcePVC == "" || op.TargetPVC == "" {
			continue
		}
		key := op.SourceNamespace + "/" + op.SourcePVC + ">" + op.TargetPVC
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, pvcMountOp{sourceNamespace: op.SourceNamespace, sourcePVC: op.SourcePVC, targetPVC: op.TargetPVC})
	}
	return out
}

func injectStorageMountsIntoResource(resource dispatchResource, mounts []storageMountSpec) (dispatchResource, bool, error) {
	u, err := dispatchObject(resource)
	if err != nil {
		return resource, false, err
	}
	applied := false
	if isVolcanoVCJob(u) {
		taskApplied, err := injectStorageMountsIntoVCJob(u, mounts)
		if err != nil {
			return resource, false, err
		}
		applied = taskApplied
	} else {
		for _, path := range podSpecPaths(u.GetKind()) {
			specApplied, err := injectStorageMountsIntoPodSpec(u.Object, path, mounts)
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

func injectStorageMountsIntoVCJob(u *unstructured.Unstructured, mounts []storageMountSpec) (bool, error) {
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
		taskApplied, err := injectStorageMountsIntoPodSpec(task, []string{"template", "spec"}, mounts)
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

func injectStorageMountsIntoPodSpec(obj map[string]any, path []string, mounts []storageMountSpec) (bool, error) {
	applied := false
	for _, mount := range mounts {
		if err := ensureStorageVolume(obj, path, mount); err != nil {
			return false, err
		}
		mountApplied, err := ensureStorageContainerMounts(obj, path, mount)
		if err != nil {
			return false, err
		}
		applied = applied || mountApplied
	}
	return applied, nil
}

func ensureStorageVolume(obj map[string]any, path []string, mount storageMountSpec) error {
	volumesPath := append(append([]string{}, path...), "volumes")
	volumes, _, _ := unstructured.NestedSlice(obj, volumesPath...)
	for _, raw := range volumes {
		volume, ok := raw.(map[string]any)
		if !ok || shared.TextValue(volume, "name") != mount.Name {
			continue
		}
		claim, _ := volume["persistentVolumeClaim"].(map[string]any)
		if shared.TextValue(claim, "claimName", "claim_name") != mount.ClaimName {
			return fmt.Errorf("%w: storage volume %s already references a different PVC", cluster.ErrInvalidManifest, mount.Name)
		}
		return nil
	}
	volumes = append(volumes, map[string]any{
		"name": mount.Name,
		"persistentVolumeClaim": map[string]any{
			"claimName": mount.ClaimName,
		},
	})
	if err := unstructured.SetNestedSlice(obj, volumes, volumesPath...); err != nil {
		return fmt.Errorf("set storage volumes: %w", err)
	}
	return nil
}

func ensureStorageContainerMounts(obj map[string]any, path []string, mount storageMountSpec) (bool, error) {
	applied := false
	for _, key := range []string{"containers", "initContainers"} {
		containersPath := append(append([]string{}, path...), key)
		containers, found, _ := unstructured.NestedSlice(obj, containersPath...)
		if !found {
			continue
		}
		for i, raw := range containers {
			container, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			if err := ensureStorageMountInContainer(container, mount); err != nil {
				return false, err
			}
			containers[i] = container
			applied = true
		}
		if err := unstructured.SetNestedSlice(obj, containers, containersPath...); err != nil {
			return false, fmt.Errorf("set storage container mounts: %w", err)
		}
	}
	return applied, nil
}

func ensureStorageMountInContainer(container map[string]any, mount storageMountSpec) error {
	mounts, _ := container["volumeMounts"].([]any)
	for _, raw := range mounts {
		existing, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if shared.TextValue(existing, "name") == mount.Name {
			return mergeExistingStorageMount(existing, mount)
		}
		if shared.TextValue(existing, "mountPath", "mount_path") == mount.MountPath {
			return fmt.Errorf("%w: storage mount path %s is already used", cluster.ErrInvalidManifest, mount.MountPath)
		}
	}
	container["volumeMounts"] = append(mounts, storageVolumeMountMap(mount))
	return nil
}

func mergeExistingStorageMount(existing map[string]any, mount storageMountSpec) error {
	if shared.TextValue(existing, "mountPath", "mount_path") != mount.MountPath {
		return fmt.Errorf("%w: storage mount %s already uses a different mountPath", cluster.ErrInvalidManifest, mount.Name)
	}
	if mount.ReadOnly {
		existing["readOnly"] = true
	}
	return mergeExistingStorageSubPath(existing, mount)
}

func mergeExistingStorageSubPath(existing map[string]any, mount storageMountSpec) error {
	if mount.SubPath == "" {
		return nil
	}
	if existingSubPath := shared.TextValue(existing, "subPath", "sub_path"); existingSubPath != "" && existingSubPath != mount.SubPath {
		return fmt.Errorf("%w: storage mount %s already uses a different subPath", cluster.ErrInvalidManifest, mount.Name)
	}
	existing["subPath"] = mount.SubPath
	return nil
}

func storageVolumeMountMap(mount storageMountSpec) map[string]any {
	next := map[string]any{"name": mount.Name, "mountPath": mount.MountPath}
	if mount.ReadOnly {
		next["readOnly"] = true
	}
	if mount.SubPath != "" {
		next["subPath"] = mount.SubPath
	}
	return next
}

func collectDispatchPVCClaimNames(resources []dispatchResource) map[string]struct{} {
	claims := map[string]struct{}{}
	for _, resource := range resources {
		for _, claimName := range dispatchResourcePVCClaims(resource) {
			claims[claimName] = struct{}{}
		}
	}
	return claims
}

func dispatchResourcePVCClaims(resource dispatchResource) []string {
	var obj map[string]any
	if err := json.Unmarshal(resource.Raw, &obj); err != nil {
		return nil
	}
	claims := []string{}
	for _, spec := range findDispatchPodSpecs(obj) {
		claims = append(claims, podSpecPVCClaims(spec)...)
	}
	return claims
}

func podSpecPVCClaims(spec map[string]any) []string {
	volumes, ok := spec["volumes"].([]any)
	if !ok {
		return nil
	}
	claims := []string{}
	for _, raw := range volumes {
		if claimName := pvcClaimNameFromVolume(raw); claimName != "" {
			claims = append(claims, claimName)
		}
	}
	return claims
}

func pvcClaimNameFromVolume(raw any) string {
	volume, ok := raw.(map[string]any)
	if !ok {
		return ""
	}
	source, ok := volume["persistentVolumeClaim"].(map[string]any)
	if !ok {
		return ""
	}
	return shared.TextValue(source, "claimName", "claim_name")
}

func findDispatchPodSpecs(obj map[string]any) []map[string]any {
	if _, ok := obj["containers"]; ok {
		return []map[string]any{obj}
	}
	out := []map[string]any{}
	for _, key := range []string{"spec", "template", "jobTemplate"} {
		if nested, ok := obj[key].(map[string]any); ok {
			out = append(out, findDispatchPodSpecs(nested)...)
		}
	}
	if tasks, ok := obj["tasks"].([]any); ok {
		for _, raw := range tasks {
			if task, ok := raw.(map[string]any); ok {
				out = append(out, findDispatchPodSpecs(task)...)
			}
		}
	}
	return out
}
