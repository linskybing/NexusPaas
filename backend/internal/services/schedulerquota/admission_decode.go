package schedulerquota

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

func decodeSubmitAdmissionRequest(payload map[string]any) (submitAdmissionRequest, error) {
	req := submitAdmissionRequest{
		JobID:                shared.TextValue(payload, "job_id", "jobId", "id"),
		ProjectID:            shared.TextValue(payload, "project_id", "projectId", "ProjectID"),
		UserID:               shared.TextValue(payload, "user_id", "userId", "UserID"),
		QueueName:            shared.TextValue(payload, "queue_name", "queueName", "QueueName"),
		DeviceClassName:      shared.TextValue(payload, "device_class_name", "deviceClassName", "DeviceClassName"),
		RequiredGPU:          shared.NumberValue(payload, "required_gpu", "requiredGpu", "RequiredGPU"),
		RequiredCPU:          shared.NumberValue(payload, "required_cpu", "requiredCPU", "required_cpu_cores", "RequiredCPU"),
		RequiredMemory:       int(admissionMemoryMB(payload, "required_memory", "requiredMemory", "required_memory_mb", "RequiredMemory")),
		GPUCount:             shared.IntValue(payload, "gpu_count", "gpuCount", "GPUCount"),
		StreamingSession:     shared.BoolValue(payload, "streaming_session", "streamingSession", "StreamingSession"),
		StreamMaxBitrateKbps: shared.IntValue(payload, "stream_max_bitrate_kbps", "streamMaxBitrateKbps", "StreamMaxBitrateKbps"),
		NetworkProfile:       shared.TextValue(payload, "network_profile", "networkProfile", "NetworkProfile"),
		RDMARequired:         shared.BoolValue(payload, "rdma_required", "rdmaRequired", "RDMARequired"),
		NICClass:             shared.TextValue(payload, "nic_class", "nicClass", "NICClass"),
		TopologyRequirement:  shared.TextValue(payload, "topology_requirement", "topologyRequirement", "TopologyRequirement"),
		Resources:            decodeAdmissionResources(payload["resources"]),
	}
	if rawSM, ok := firstPresent(payload, "sm_percentage", "smPercentage", "SMPercentage"); ok {
		sm := int(getInt64(rawSM, 0))
		req.SMPercentage = &sm
	}
	return req, nil
}

func decodeAdmissionResources(value any) []admissionResourcePayload {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]admissionResourcePayload, 0, len(items))
	for _, item := range items {
		data, ok := item.(map[string]any)
		if !ok {
			continue
		}
		raw := resourceRawJSON(data)
		if len(raw) == 0 {
			continue
		}
		out = append(out, admissionResourcePayload{
			Name: shared.TextValue(data, "name", "Name"),
			Kind: shared.FirstNonEmpty(
				shared.TextValue(data, "kind", "Kind"),
				kindFromRaw(raw),
			),
			Raw: raw,
		})
	}
	return out
}

func resourceRawJSON(data map[string]any) []byte {
	for _, key := range []string{"json_data", "jsonData", "json", "object", "manifest"} {
		raw, ok := data[key]
		if !ok {
			continue
		}
		if text, ok := raw.(string); ok {
			return []byte(strings.TrimSpace(text))
		}
		if b, err := json.Marshal(raw); err == nil {
			return b
		}
	}
	if shared.TextValue(data, "apiVersion") != "" || shared.TextValue(data, "kind", "Kind") != "" {
		if b, err := json.Marshal(data); err == nil {
			return b
		}
	}
	return nil
}

func kindFromRaw(raw []byte) string {
	var obj map[string]any
	if json.Unmarshal(raw, &obj) != nil {
		return ""
	}
	return stringField(obj, "kind")
}

func admissionDenied(req submitAdmissionRequest, reason string) map[string]any {
	return map[string]any{
		"allowed":    false,
		"reason":     reason,
		"job_id":     req.JobID,
		"project_id": req.ProjectID,
		"user_id":    req.UserID,
	}
}

func admissionDeniedReview(req submitAdmissionRequest, review admissionReview, reason string) map[string]any {
	review.Allowed = false
	review.Reason = reason
	data := admissionReviewData(review)
	if data["project_id"] == "" {
		data["project_id"] = req.ProjectID
	}
	if data["user_id"] == "" {
		data["user_id"] = req.UserID
	}
	if data["queue_name"] == "" {
		data["queue_name"] = req.QueueName
	}
	if req.JobID != "" {
		data["job_id"] = req.JobID
	}
	return data
}

func admissionReviewData(review admissionReview) map[string]any {
	data := map[string]any{
		"allowed":                 review.Allowed,
		"reason":                  review.Reason,
		"project_id":              review.ProjectID,
		"user_id":                 review.UserID,
		"queue_name":              review.QueueName,
		"priority_value":          review.QueuePriority,
		"preemptible":             review.QueuePreemptible,
		"is_preemptible":          review.QueuePreemptible,
		"runtime_limit_seconds":   review.RuntimeLimit,
		"max_runtime_seconds":     review.RuntimeLimit,
		"device_class_name":       review.DeviceClassName,
		"required_gpu":            review.RequiredGPU,
		"required_cpu":            review.RequiredCPU,
		"required_memory":         review.RequiredMemory,
		"streaming_session":       review.StreamingSession,
		"stream_max_bitrate_kbps": review.StreamMaxBitrateKbps,
		"usage": map[string]any{
			"project_gpu":                review.Usage.ProjectGPU,
			"project_cpu":                review.Usage.ProjectCPU,
			"project_memory_mb":          review.Usage.ProjectMemoryMB,
			"user_gpu":                   review.Usage.UserGPU,
			"user_cpu":                   review.Usage.UserCPU,
			"user_memory_mb":             review.Usage.UserMemoryMB,
			"user_running_jobs":          review.Usage.UserRunningJobs,
			"user_queued_jobs":           review.Usage.UserQueuedJobs,
			"resource_floor_gpu":         review.Usage.ResourceFloorGPU,
			"resource_floor_cpu":         review.Usage.ResourceFloorCPU,
			"floor_memory_mb":            review.Usage.FloorMemoryMB,
			"active_stream_sessions":     review.Usage.ActiveStreamSessions,
			"active_stream_bitrate_kbps": review.Usage.ActiveStreamBitrateKbps,
			"stream_egress_budget_kbps":  review.Usage.StreamEgressBudgetKbps,
		},
	}
	if review.NetworkProfile != "" {
		data["network_profile"] = review.NetworkProfile
	}
	if review.RDMARequired {
		data["rdma_required"] = true
	}
	if review.NICClass != "" {
		data["nic_class"] = review.NICClass
	}
	if review.TopologyRequirement != "" {
		data["topology_requirement"] = review.TopologyRequirement
	}
	if len(review.NetworkAnnotations) > 0 {
		data["network_annotations"] = shared.CloneMap(review.NetworkAnnotations)
	}
	if len(review.NetworkEnv) > 0 {
		data["network_env"] = shared.CloneMap(review.NetworkEnv)
	}
	return data
}

func persistAdmissionReview(ctx context.Context, repo *recordStoreSchedulerQuotaRepository, review admissionReview) {
	if repo == nil {
		return
	}
	// Admission has already succeeded; persistence is audit-only.
	repo.PersistSubmitAdmissionReview(ctx, review)
}

func stringField(data map[string]any, key string) string {
	value, _ := data[key].(string)
	return strings.TrimSpace(value)
}

func getInt64(value any, fallback int64) int64 {
	switch typed := value.(type) {
	case float64:
		return int64(typed)
	case int64:
		return typed
	case int:
		return int64(typed)
	default:
		return fallback
	}
}

func listOfMaps(value any) []map[string]any {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if data, ok := item.(map[string]any); ok {
			out = append(out, data)
		}
	}
	return out
}

func firstPresent(data map[string]any, keys ...string) (any, bool) {
	for _, key := range keys {
		value, ok := data[key]
		if ok {
			return value, true
		}
	}
	return nil, false
}

func firstValue(data map[string]any, keys ...string) any {
	value, _ := firstPresent(data, keys...)
	return value
}
