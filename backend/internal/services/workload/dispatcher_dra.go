package workload

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/linskybing/nexuspaas/backend/internal/platform/cluster"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const (
	draAPIVersion                 = "resource.k8s.io/v1"
	draResourceClaimTemplateKind  = "ResourceClaimTemplate"
	draResourceClaimKind          = "ResourceClaim"
	draDefaultDeviceClassName     = "gpu.nvidia.com"
	draDefaultGPUResourceKey      = "nvidia.com/gpu"
	draPodClaimName               = "gpu"
	draLabelGPUCount              = "platform-go/dra-gpu-count"
	draLabelSMPct                 = "platform-go/dra-sm-pct"
	draLabelEffectiveGPU          = "platform-go/dra-effective-gpu"
	draLabelPinnedMem             = "platform-go/dra-pinned-mem"
	draLabelGPUModel              = "platform-go/dra-gpu-model"
	draLabelClaimName             = "platform-go/dra-claim-name"
	draLabelSharingMode           = "platform-go/dra-sharing-mode"
	draVolcanoResourceRequestAnno = "volcano.sh/resource-request"
)

type draDispatchConfig struct {
	GPUCount          int
	SMPercentage      *int
	PinnedMemoryLimit *string
	DeviceClassName   string
	GPUClaimName      string
}

func prepareDRADispatchManifests(
	job map[string]any,
	resources []dispatchResource,
	namespace string,
) ([]dispatchManifest, []dispatchResource, error) {
	cfg, err := draConfigFromJob(job)
	if err != nil {
		return nil, nil, err
	}
	if !cfg.enabled() {
		return nil, resources, nil
	}
	claimName := strings.TrimSpace(cfg.GPUClaimName)
	prefix := []dispatchManifest{}
	if claimName == "" && cfg.GPUCount > 0 && resourcesNeedTemplateDRA(resources) {
		claimName = sharedClaimTemplateName(cfg)
		raw, err := buildResourceClaimTemplateManifest(cfg, namespace, claimName)
		if err != nil {
			return nil, nil, err
		}
		prefix = append(prefix, dispatchManifest{Raw: raw})
	}
	if claimName == "" {
		return prefix, resources, nil
	}
	updated := make([]dispatchResource, 0, len(resources))
	for _, item := range resources {
		injected, err := injectDRAIntoResourceIfNeeded(item, cfg, claimName)
		if err != nil {
			return nil, nil, err
		}
		updated = append(updated, injected)
	}
	return prefix, updated, nil
}

func draConfigFromJob(job map[string]any) (draDispatchConfig, error) {
	cfg := draDispatchConfig{
		GPUCount:        shared.IntValue(job, "gpu_count", "gpuCount", "GPUCount"),
		DeviceClassName: shared.FirstNonEmpty(shared.TextValue(job, "device_class_name", "deviceClassName", "DeviceClassName"), draDefaultDeviceClassName),
		GPUClaimName:    shared.TextValue(job, "gpu_claim_name", "gpuClaimName", "GPUClaimName"),
	}
	if raw, ok := firstPresent(job, "sm_percentage", "smPercentage", "SMPercentage"); ok {
		sm := intValue(raw)
		cfg.SMPercentage = &sm
	}
	if pinned := shared.TextValue(job, "pinned_memory_limit", "pinnedMemoryLimit", "pinned_memory", "pinnedMemory"); pinned != "" {
		cfg.PinnedMemoryLimit = &pinned
	}
	return cfg, cfg.validate()
}

func (c draDispatchConfig) enabled() bool {
	return c.GPUCount > 0 || strings.TrimSpace(c.GPUClaimName) != ""
}

func jobRequestsDRA(job map[string]any) bool {
	cfg, err := draConfigFromJob(job)
	return err == nil && cfg.enabled()
}

func (c draDispatchConfig) validate() error {
	if c.GPUCount < 0 {
		return fmt.Errorf("DRA GPU count must be non-negative")
	}
	if c.SMPercentage != nil && (*c.SMPercentage < 1 || *c.SMPercentage > 100) {
		return fmt.Errorf("DRA SM percentage must be between 1 and 100")
	}
	if c.PinnedMemoryLimit != nil {
		if _, err := resource.ParseQuantity(*c.PinnedMemoryLimit); err != nil {
			return fmt.Errorf("invalid DRA pinned memory limit %q: %w", *c.PinnedMemoryLimit, err)
		}
	}
	if strings.TrimSpace(c.GPUClaimName) != "" && c.GPUCount < 1 {
		return fmt.Errorf("DRA GPU count must be at least 1 when gpu_claim_name is set")
	}
	return nil
}

func resourcesNeedTemplateDRA(resources []dispatchResource) bool {
	for _, resource := range resources {
		if resourceNeedsTemplateDRA(resource) {
			return true
		}
	}
	return false
}

func resourceNeedsTemplateDRA(resource dispatchResource) bool {
	u, err := dispatchObject(resource)
	if err != nil || resourceHasDRAClaims(u) {
		return false
	}
	return resourceHasGPUMarker(u)
}

func resourceHasDRAClaims(u *unstructured.Unstructured) bool {
	for _, path := range podSpecPaths(u.GetKind()) {
		claims, found, _ := unstructured.NestedSlice(u.Object, append(path, "resourceClaims")...)
		if found && len(claims) > 0 {
			return true
		}
	}
	return false
}

func resourceHasGPUMarker(u *unstructured.Unstructured) bool {
	for _, path := range podSpecPaths(u.GetKind()) {
		if containersHaveGPUMarker(u.Object, append(path, "containers")...) ||
			containersHaveGPUMarker(u.Object, append(path, "initContainers")...) {
			return true
		}
	}
	return false
}

func containersHaveGPUMarker(obj map[string]any, fields ...string) bool {
	containers, _, _ := unstructured.NestedSlice(obj, fields...)
	for _, raw := range containers {
		container, _ := raw.(map[string]any)
		if containerHasGPUMarker(container) {
			return true
		}
	}
	return false
}

func containerHasGPUMarker(container map[string]any) bool {
	resources, _ := container["resources"].(map[string]any)
	for _, level := range []string{"requests", "limits"} {
		values, _ := resources[level].(map[string]any)
		for key := range values {
			if isGPUResourceKey(key) {
				return true
			}
		}
	}
	return false
}

func injectDRAIntoResourceIfNeeded(resource dispatchResource, cfg draDispatchConfig, claimName string) (dispatchResource, error) {
	if !resourceNeedsTemplateDRA(resource) {
		return resource, nil
	}
	u, err := dispatchObject(resource)
	if err != nil {
		return resource, err
	}
	if err := injectDRAClaimIntoObject(u, cfg, claimName, strings.TrimSpace(cfg.GPUClaimName) != ""); err != nil {
		return resource, err
	}
	raw, err := json.Marshal(u.Object)
	if err != nil {
		return resource, fmt.Errorf(dispatcherMarshalResourceError, u.GetName(), err)
	}
	resource.Raw = raw
	return resource, nil
}

func injectDRAClaimIntoObject(u *unstructured.Unstructured, cfg draDispatchConfig, claimName string, directClaim bool) error {
	claimRef := map[string]any{"name": draPodClaimName}
	if directClaim {
		claimRef["resourceClaimName"] = claimName
	} else {
		claimRef["resourceClaimTemplateName"] = claimName
	}
	containerClaim := map[string]any{"name": draPodClaimName}
	paths := podSpecPaths(u.GetKind())
	if len(paths) == 0 {
		return fmt.Errorf("%w: unsupported DRA workload kind %s", cluster.ErrInvalidManifest, u.GetKind())
	}
	for _, path := range paths {
		claimsPath := append(path, "resourceClaims")
		claims, _, _ := unstructured.NestedSlice(u.Object, claimsPath...)
		claims = append(claims, claimRef)
		if err := unstructured.SetNestedSlice(u.Object, claims, claimsPath...); err != nil {
			return fmt.Errorf("set DRA resourceClaims: %w", err)
		}
		if err := injectContainerDRAClaims(u, containerClaim, path...); err != nil {
			return err
		}
	}
	mergeDRAMetadata(u, cfg, claimName)
	return nil
}

func injectContainerDRAClaims(u *unstructured.Unstructured, containerClaim map[string]any, path ...string) error {
	for _, key := range []string{"containers", "initContainers"} {
		containersPath := append(path, key)
		containers, found, _ := unstructured.NestedSlice(u.Object, containersPath...)
		if !found {
			continue
		}
		for i, raw := range containers {
			container, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			resources, _ := container["resources"].(map[string]any)
			if resources == nil {
				resources = map[string]any{}
			}
			claims, _ := resources["claims"].([]any)
			resources["claims"] = append(claims, containerClaim)
			stripGPUMarkerKeys(resources, "requests")
			stripGPUMarkerKeys(resources, "limits")
			container["resources"] = resources
			containers[i] = container
		}
		if err := unstructured.SetNestedSlice(u.Object, containers, containersPath...); err != nil {
			return fmt.Errorf("set DRA container claims: %w", err)
		}
	}
	return nil
}

func mergeDRAMetadata(u *unstructured.Unstructured, cfg draDispatchConfig, claimName string) {
	labels := draGPULabels(cfg, claimName)
	switch strings.ToLower(u.GetKind()) {
	case "pod":
		mergeObjectLabels(u, labels)
		setAnnotationValue(u, draVolcanoResourceRequestAnno, draResourceRequest(cfg))
	case "deployment", "job":
		mergeObjectLabels(u, labels)
		mergeNestedStringMap(u.Object, labels, "spec", "template", "metadata", "labels")
		mergeNestedStringMap(u.Object, map[string]string{draVolcanoResourceRequestAnno: draResourceRequest(cfg)}, "spec", "template", "metadata", "annotations")
	}
}

func podSpecPaths(kind string) [][]string {
	switch strings.ToLower(kind) {
	case "pod":
		return [][]string{{"spec"}}
	case "deployment", "job":
		return [][]string{{"spec", "template", "spec"}}
	default:
		return nil
	}
}

func buildResourceClaimTemplateManifest(cfg draDispatchConfig, namespace, name string) ([]byte, error) {
	devices := map[string]any{
		"requests": []any{map[string]any{
			"name": draPodClaimName,
			"exactly": map[string]any{
				"allocationMode":  "ExactCount",
				"deviceClassName": cfg.DeviceClassName,
				"count":           int64(cfg.GPUCount),
			},
		}},
	}
	if cfg.SMPercentage != nil {
		devices["config"] = []any{draMPSConfig(cfg)}
	}
	body := map[string]any{
		"apiVersion": draAPIVersion,
		"kind":       draResourceClaimTemplateKind,
		"metadata": map[string]any{
			"name":      name,
			"namespace": namespace,
			"labels":    draGPULabels(cfg, name),
		},
		"spec": map[string]any{
			"spec": map[string]any{
				"devices": devices,
			},
		},
	}
	return json.Marshal(body)
}

func draMPSConfig(cfg draDispatchConfig) map[string]any {
	mpsConfig := map[string]any{"defaultActiveThreadPercentage": int64(*cfg.SMPercentage)}
	if cfg.PinnedMemoryLimit != nil {
		mpsConfig["defaultPinnedDeviceMemoryLimit"] = *cfg.PinnedMemoryLimit
	}
	return map[string]any{
		"requests": []any{draPodClaimName},
		"opaque": map[string]any{
			"driver": "gpu.nvidia.com",
			"parameters": map[string]any{
				"apiVersion": "resource.nvidia.com/v1beta1",
				"kind":       "GpuConfig",
				"sharing": map[string]any{
					"strategy":  "MPS",
					"mpsConfig": mpsConfig,
				},
			},
		},
	}
}

func draGPULabels(cfg draDispatchConfig, claimName string) map[string]string {
	if cfg.GPUCount < 1 {
		return nil
	}
	sm := 100
	if cfg.SMPercentage != nil && *cfg.SMPercentage > 0 {
		sm = *cfg.SMPercentage
	}
	labels := map[string]string{
		draLabelGPUCount:     strconv.Itoa(cfg.GPUCount),
		draLabelSMPct:        strconv.Itoa(sm),
		draLabelEffectiveGPU: strconv.FormatFloat(float64(cfg.GPUCount)*float64(sm)/100.0, 'f', -1, 64),
		draLabelPinnedMem:    "none",
	}
	if cfg.PinnedMemoryLimit != nil && *cfg.PinnedMemoryLimit != "" {
		labels[draLabelPinnedMem] = *cfg.PinnedMemoryLimit
	}
	if model := modelFromDeviceClassName(cfg.DeviceClassName); model != "" {
		labels[draLabelGPUModel] = model
	}
	if claimName != "" {
		labels[draLabelClaimName] = claimName
		labels[draLabelSharingMode] = "shared-claim"
	}
	return labels
}

func sharedClaimTemplateName(cfg draDispatchConfig) string {
	base := "gpu-shared"
	if cfg.DeviceClassName != "" && cfg.DeviceClassName != draDefaultDeviceClassName {
		parts := strings.Split(cfg.DeviceClassName, ".")
		base = "gpu-" + parts[0]
	}
	sm := 100
	if cfg.SMPercentage != nil {
		sm = *cfg.SMPercentage
	}
	mem := "max"
	if cfg.PinnedMemoryLimit != nil {
		mem = strings.ToLower(*cfg.PinnedMemoryLimit)
	}
	return strings.ToLower(fmt.Sprintf("%s-cnt%d-sm%d-mem%s", base, cfg.GPUCount, sm, mem))
}

func draResourceRequest(cfg draDispatchConfig) string {
	return fmt.Sprintf("%s:%d", resourceKeyFromDeviceClassName(cfg.DeviceClassName), cfg.GPUCount)
}

func resourceKeyFromDeviceClassName(deviceClassName string) string {
	model := modelFromDeviceClassName(deviceClassName)
	if model == "" {
		return draDefaultGPUResourceKey
	}
	return "nvidia.com/" + model
}

func modelFromDeviceClassName(deviceClassName string) string {
	if deviceClassName == "" {
		return ""
	}
	if deviceClassName == draDefaultDeviceClassName {
		return "gpu"
	}
	if strings.Contains(deviceClassName, ".gpu.") {
		parts := strings.SplitN(deviceClassName, ".", 2)
		return parts[0]
	}
	return deviceClassName
}

func isGPUResourceKey(key string) bool {
	model := strings.TrimPrefix(key, "nvidia.com/")
	return model != key && model != ""
}

func stripGPUMarkerKeys(resources map[string]any, level string) {
	values, ok := resources[level].(map[string]any)
	if !ok {
		return
	}
	for key := range values {
		if isGPUResourceKey(key) {
			delete(values, key)
		}
	}
	if len(values) == 0 {
		delete(resources, level)
	}
}

func setAnnotationValue(u *unstructured.Unstructured, key, value string) {
	annotations := u.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations[key] = value
	u.SetAnnotations(annotations)
}

func intValue(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case json.Number:
		if parsed, err := typed.Int64(); err == nil {
			return int(parsed)
		}
	default:
		return 0
	}
	return 0
}
