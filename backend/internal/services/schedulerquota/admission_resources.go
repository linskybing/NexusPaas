package schedulerquota

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
	"k8s.io/apimachinery/pkg/api/resource"
)

type admissionResourceFloor struct {
	gpu      float64
	cpu      float64
	memoryMB int
}

type admissionSecretPolicyViolation struct {
	ResourceName string
	ResourceKind string
	Reason       string
}

func (v admissionSecretPolicyViolation) Error() string {
	return v.Reason
}

type admissionRuntimeSocketPolicyViolation struct {
	ResourceName string
	ResourceKind string
	SocketPath   string
	Reason       string
}

func (v admissionRuntimeSocketPolicyViolation) Error() string {
	return v.Reason
}

func admissionResourceFloorFromRequest(req submitAdmissionRequest) (admissionResourceFloor, error) {
	var floor admissionResourceFloor
	if err := validateAdmissionResourceAccounting(req); err != nil {
		return floor, err
	}
	for _, payload := range req.Resources {
		if len(payload.Raw) == 0 {
			continue
		}
		var obj map[string]any
		if err := json.Unmarshal(payload.Raw, &obj); err != nil {
			return floor, fmt.Errorf("parse resource %s for submit resource policy: %w", payload.Name, err)
		}
		if err := rejectUnsupportedAdmissionKind(obj); err != nil {
			return floor, err
		}
		floor.gpu += admissionPayloadGPU(obj)
		floor.cpu += admissionPayloadCPU(obj)
		floor.memoryMB += admissionPayloadMemory(obj)
	}
	if req.GPUCount > 0 {
		smPct := 100
		if req.SMPercentage != nil {
			smPct = *req.SMPercentage
		}
		draFloor := float64(req.GPUCount) * float64(smPct) / 100.0
		if draFloor > floor.gpu {
			floor.gpu = draFloor
		}
	}
	return floor, nil
}

func validateAdmissionResourceAccounting(req submitAdmissionRequest) error {
	if req.RequiredGPU < 0 {
		return fmt.Errorf("required GPU must be non-negative")
	}
	if req.RequiredCPU < 0 {
		return fmt.Errorf("required CPU must be non-negative")
	}
	if req.RequiredMemory < 0 {
		return fmt.Errorf("required memory must be non-negative")
	}
	if req.GPUCount < 0 {
		return fmt.Errorf("DRA GPU count must be non-negative")
	}
	if req.SMPercentage != nil && (*req.SMPercentage < 1 || *req.SMPercentage > 100) {
		return fmt.Errorf("DRA SM percentage must be between 1 and 100")
	}
	if req.PinnedMemoryLimit != nil {
		if _, err := resource.ParseQuantity(*req.PinnedMemoryLimit); err != nil {
			return fmt.Errorf("invalid DRA pinned memory limit %q: %w", *req.PinnedMemoryLimit, err)
		}
	}
	return nil
}

func rejectUnsupportedAdmissionKind(obj map[string]any) error {
	if violation, found := admissionRuntimeSocketPolicyViolationFromObject(obj); found {
		return violation
	}
	kind := stringField(obj, "kind")
	switch strings.ToLower(kind) {
	case "secret":
		return admissionSecretPolicyViolation{
			ResourceName: admissionObjectName(obj),
			ResourceKind: kind,
			Reason:       rawSecretPolicyReason(),
		}
	case "cronjob", "daemonset", "statefulset", "replicaset", "replicationcontroller":
		return fmt.Errorf("unsupported workload kind %s for job submit; use Pod, Deployment, or Job", kind)
	default:
		return nil
	}
}

func admissionRuntimeSocketPolicyViolationFromRequest(req submitAdmissionRequest) (admissionRuntimeSocketPolicyViolation, bool) {
	for _, resource := range req.Resources {
		if len(resource.Raw) == 0 {
			continue
		}
		var obj map[string]any
		if json.Unmarshal(resource.Raw, &obj) != nil {
			continue
		}
		violation, found := admissionRuntimeSocketPolicyViolationFromObject(obj)
		if !found {
			continue
		}
		violation.ResourceName = shared.FirstNonEmpty(violation.ResourceName, admissionResourceName(resource))
		violation.ResourceKind = shared.FirstNonEmpty(violation.ResourceKind, admissionResourceKind(resource))
		return violation, true
	}
	return admissionRuntimeSocketPolicyViolation{}, false
}

func admissionRuntimeSocketPolicyViolationFromObject(obj map[string]any) (admissionRuntimeSocketPolicyViolation, bool) {
	kind := stringField(obj, "kind")
	for _, podSpec := range admissionResourcePodSpecs(obj) {
		if socketPath, found := shared.RuntimeSocketHostPath(podSpec); found {
			return admissionRuntimeSocketPolicyViolation{
				ResourceName: admissionObjectName(obj),
				ResourceKind: kind,
				SocketPath:   socketPath,
				Reason:       runtimeSocketPolicyReason(socketPath),
			}, true
		}
	}
	return admissionRuntimeSocketPolicyViolation{}, false
}

func admissionResourcePodSpecs(obj map[string]any) []map[string]any {
	if strings.EqualFold(stringField(obj, "kind"), "Job") {
		if tasks := admissionVolcanoTaskPodSpecs(obj); len(tasks) > 0 {
			return tasks
		}
	}
	_, podSpec, _, ok := admissionExtractPodSpec(obj)
	if !ok {
		return nil
	}
	return []map[string]any{podSpec}
}

func admissionVolcanoTaskPodSpecs(obj map[string]any) []map[string]any {
	spec, _ := obj["spec"].(map[string]any)
	tasks, _ := spec["tasks"].([]any)
	out := make([]map[string]any, 0, len(tasks))
	for _, raw := range tasks {
		task, _ := raw.(map[string]any)
		template, _ := task["template"].(map[string]any)
		podSpec, _ := template["spec"].(map[string]any)
		if podSpec != nil {
			out = append(out, podSpec)
		}
	}
	return out
}

func admissionSecretPolicyViolationFromRequest(req submitAdmissionRequest) (admissionSecretPolicyViolation, bool) {
	for _, resource := range req.Resources {
		kind := admissionResourceKind(resource)
		if !strings.EqualFold(kind, "Secret") {
			continue
		}
		return admissionSecretPolicyViolation{
			ResourceName: admissionResourceName(resource),
			ResourceKind: shared.FirstNonEmpty(kind, resource.Kind),
			Reason:       rawSecretPolicyReason(),
		}, true
	}
	return admissionSecretPolicyViolation{}, false
}

func admissionResourceKind(resource admissionResourcePayload) string {
	if obj, ok := admissionResourceRawObject(resource); ok {
		if kind := stringField(obj, "kind"); kind != "" {
			return kind
		}
	}
	return strings.TrimSpace(resource.Kind)
}

func admissionResourceName(resource admissionResourcePayload) string {
	if obj, ok := admissionResourceRawObject(resource); ok {
		if name := admissionObjectName(obj); name != "" {
			return name
		}
	}
	return strings.TrimSpace(resource.Name)
}

func admissionResourceRawObject(resource admissionResourcePayload) (map[string]any, bool) {
	var obj map[string]any
	if len(resource.Raw) == 0 || json.Unmarshal(resource.Raw, &obj) != nil {
		return nil, false
	}
	return obj, true
}

func admissionObjectName(obj map[string]any) string {
	metadata, _ := obj["metadata"].(map[string]any)
	return stringField(metadata, "name")
}

func rawSecretPolicyReason() string {
	return "raw Kubernetes Secret resources are rejected; use the platform secret API or an approved ExternalSecret profile"
}

func runtimeSocketPolicyReason(path string) string {
	return "user workloads cannot mount container runtime socket " + path
}

func admissionPayloadGPU(obj map[string]any) float64 {
	kind := stringField(obj, "kind")
	if kind == "ResourceClaim" {
		count, smPct := admissionResourceClaimGPUConfig(obj)
		return gpuFraction(count, smPct)
	}
	if kind == "ResourceClaimTemplate" {
		spec, _ := obj["spec"].(map[string]any)
		claimSpec, _ := spec["spec"].(map[string]any)
		count, smPct := admissionResourceClaimGPUConfig(map[string]any{"spec": claimSpec})
		return gpuFraction(count, smPct)
	}
	_, podSpec, replicas, ok := admissionExtractPodSpec(obj)
	if !ok {
		return 0
	}
	podGPU := sumAdmissionGPU(containersFromSpec(podSpec, "containers"))
	initGPU := maxAdmissionGPU(containersFromSpec(podSpec, "initContainers"))
	if initGPU > podGPU {
		podGPU = initGPU
	}
	return podGPU * float64(replicas)
}

func admissionPayloadCPU(obj map[string]any) float64 {
	_, podSpec, replicas, ok := admissionExtractPodSpec(obj)
	if !ok {
		return 0
	}
	podCPU := sumAdmissionCPU(containersFromSpec(podSpec, "containers"))
	initCPU := maxAdmissionCPU(containersFromSpec(podSpec, "initContainers"))
	if initCPU > podCPU {
		podCPU = initCPU
	}
	return podCPU * float64(replicas)
}

func admissionPayloadMemory(obj map[string]any) int {
	_, podSpec, replicas, ok := admissionExtractPodSpec(obj)
	if !ok {
		return 0
	}
	podMemory := sumAdmissionMemory(containersFromSpec(podSpec, "containers"))
	initMemory := maxAdmissionMemory(containersFromSpec(podSpec, "initContainers"))
	if initMemory > podMemory {
		podMemory = initMemory
	}
	return podMemory * int(replicas)
}

func admissionExtractPodSpec(obj map[string]any) (string, map[string]any, int64, bool) {
	kind := strings.ToLower(stringField(obj, "kind"))
	spec, ok := obj["spec"].(map[string]any)
	if !ok {
		return "", nil, 1, false
	}
	replicas := int64(1)
	switch kind {
	case "pod":
		return "/spec", spec, replicas, true
	case "deployment":
		replicas = getInt64(spec["replicas"], 1)
		template, ok := spec["template"].(map[string]any)
		if !ok {
			return "", nil, replicas, false
		}
		podSpec, ok := template["spec"].(map[string]any)
		return "/spec/template/spec", podSpec, replicas, ok
	case "job":
		replicas = getInt64(spec["parallelism"], 1)
		template, ok := spec["template"].(map[string]any)
		if !ok {
			return "", nil, replicas, false
		}
		podSpec, ok := template["spec"].(map[string]any)
		return "/spec/template/spec", podSpec, replicas, ok
	default:
		return "", nil, replicas, false
	}
}

func admissionResourceClaimGPUConfig(obj map[string]any) (int, int) {
	spec, _ := obj["spec"].(map[string]any)
	devices, _ := spec["devices"].(map[string]any)
	count := 0
	for _, req := range listOfMaps(devices["requests"]) {
		exactly, _ := req["exactly"].(map[string]any)
		count += int(getInt64(exactly["count"], 0))
	}
	smPct := 100
	for _, cfg := range listOfMaps(devices["config"]) {
		opaque, _ := cfg["opaque"].(map[string]any)
		params, _ := opaque["parameters"].(map[string]any)
		sharing, _ := params["sharing"].(map[string]any)
		mps, _ := sharing["mpsConfig"].(map[string]any)
		if pct := int(getInt64(mps["defaultActiveThreadPercentage"], 0)); pct > 0 {
			smPct = pct
			break
		}
	}
	return count, smPct
}

func enforceAdmissionResourceFloor(req submitAdmissionRequest, floor admissionResourceFloor) error {
	if floatBelow(req.RequiredGPU, floor.gpu) {
		return fmt.Errorf("declared GPU %.2f is lower than payload GPU %.2f", req.RequiredGPU, floor.gpu)
	}
	if floatBelow(req.RequiredCPU, floor.cpu) {
		return fmt.Errorf("declared CPU %.2f is lower than payload CPU %.2f", req.RequiredCPU, floor.cpu)
	}
	if req.RequiredMemory < floor.memoryMB {
		return fmt.Errorf("declared memory %dMi is lower than payload memory %dMi", req.RequiredMemory, floor.memoryMB)
	}
	return nil
}

func containersFromSpec(podSpec map[string]any, key string) []map[string]any {
	items, ok := podSpec[key].([]any)
	if !ok {
		return nil
	}
	containers := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if container, ok := item.(map[string]any); ok {
			containers = append(containers, container)
		}
	}
	return containers
}

func sumAdmissionGPU(containers []map[string]any) float64 {
	sum := 0.0
	for _, container := range containers {
		sum += admissionContainerGPU(container)
	}
	return sum
}

func maxAdmissionGPU(containers []map[string]any) float64 {
	maxValue := 0.0
	for _, container := range containers {
		if value := admissionContainerGPU(container); value > maxValue {
			maxValue = value
		}
	}
	return maxValue
}

func sumAdmissionCPU(containers []map[string]any) float64 {
	sum := 0.0
	for _, container := range containers {
		sum += admissionContainerCPU(container)
	}
	return sum
}

func maxAdmissionCPU(containers []map[string]any) float64 {
	maxValue := 0.0
	for _, container := range containers {
		if value := admissionContainerCPU(container); value > maxValue {
			maxValue = value
		}
	}
	return maxValue
}

func sumAdmissionMemory(containers []map[string]any) int {
	sum := 0
	for _, container := range containers {
		sum += admissionContainerMemory(container)
	}
	return sum
}

func maxAdmissionMemory(containers []map[string]any) int {
	maxValue := 0
	for _, container := range containers {
		if value := admissionContainerMemory(container); value > maxValue {
			maxValue = value
		}
	}
	return maxValue
}

func admissionContainerGPU(container map[string]any) float64 {
	resources, _ := container["resources"].(map[string]any)
	limits, _ := resources["limits"].(map[string]any)
	requests, _ := resources["requests"].(map[string]any)
	if total := sumAdmissionGPUKeys(limits); total > 0 {
		return total
	}
	return sumAdmissionGPUKeys(requests)
}

func admissionContainerCPU(container map[string]any) float64 {
	resources, _ := container["resources"].(map[string]any)
	limits, _ := resources["limits"].(map[string]any)
	requests, _ := resources["requests"].(map[string]any)
	if value := parseAdmissionCPU(limits["cpu"]); value > 0 {
		return value
	}
	return parseAdmissionCPU(requests["cpu"])
}

func admissionContainerMemory(container map[string]any) int {
	resources, _ := container["resources"].(map[string]any)
	limits, _ := resources["limits"].(map[string]any)
	requests, _ := resources["requests"].(map[string]any)
	if value := parseAdmissionMemory(limits["memory"]); value > 0 {
		return value
	}
	return parseAdmissionMemory(requests["memory"])
}

func sumAdmissionGPUKeys(resources map[string]any) float64 {
	total := 0.0
	for key, value := range resources {
		if isGPUResourceKey(key) {
			total += parseAdmissionGPU(value)
		}
	}
	return total
}

func parseAdmissionGPU(value any) float64 {
	switch typed := value.(type) {
	case string:
		text := strings.TrimSpace(strings.TrimSuffix(typed, "m"))
		parsed, err := strconvParseFloat(text)
		if err == nil {
			return parsed
		}
		if q, qerr := resource.ParseQuantity(typed); qerr == nil {
			return float64(q.Value())
		}
	case float64:
		return typed
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	}
	return 0
}

func parseAdmissionCPU(value any) float64 {
	switch typed := value.(type) {
	case string:
		q, err := resource.ParseQuantity(typed)
		if err != nil {
			return 0
		}
		return q.AsApproximateFloat64()
	case float64:
		return typed
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	}
	return 0
}

func parseAdmissionMemory(value any) int {
	switch typed := value.(type) {
	case string:
		q, err := resource.ParseQuantity(typed)
		if err != nil {
			return 0
		}
		return int(q.Value() / (1024 * 1024))
	case float64:
		return int(typed)
	case int:
		return typed
	case int64:
		return int(typed)
	}
	return 0
}

func admissionMemoryMB(data map[string]any, keys ...string) float64 {
	for _, key := range keys {
		if value, ok := data[key]; ok {
			if memory := parseAdmissionMemory(value); memory > 0 {
				return float64(memory)
			}
			if number := shared.NumberValue(map[string]any{key: value}, key); number != 0 {
				return number
			}
		}
	}
	return 0
}

func isGPUResourceKey(key string) bool {
	model := strings.TrimPrefix(key, "nvidia.com/")
	return model != key && model != ""
}

func gpuFraction(count, smPct int) float64 {
	if count < 1 {
		return 0
	}
	if smPct < 1 || smPct > 100 {
		smPct = 100
	}
	return float64(count) * float64(smPct) / 100.0
}

func floatBelow(actual, floor float64) bool {
	return actual+1e-9 < math.Max(floor, 0)
}

func strconvParseFloat(text string) (float64, error) {
	return strconv.ParseFloat(text, 64)
}
