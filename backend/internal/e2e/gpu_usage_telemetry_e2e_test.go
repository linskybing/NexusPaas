//go:build e2e

package e2e

import (
	"net/http"
	"testing"
	"time"
)

func TestGPUUsageTelemetryCollectorE2E(t *testing.T) {
	h := newHarness(t, usageObservabilityService)
	now := time.Now().UTC().Truncate(time.Second)
	userID := "gpuuser" + h.runID
	projectID := "gpuproject" + h.runID
	jobID := "gpujob" + h.runID

	h.seedGPUUsageReadModels(userID, projectID, jobID, now)
	h.services[usageObservabilityService].app.RunMaintenanceOnce(h.ctx, time.Second)

	h.assertGPUCollectorSnapshots(jobID, userID, projectID)
	h.assertGPUCollectorSummary(jobID)
	h.assertGPUUsageAPIs(jobID, userID, projectID)
}

func (h *e2eHarness) assertGPUCollectorSnapshots(jobID, userID, projectID string) {
	h.t.Helper()
	matchingSnapshots := 0
	for _, snapshot := range h.listRecords(gpuSnapshotsResource) {
		if snapshot.Data["job_id"] != jobID {
			continue
		}
		matchingSnapshots++
		h.assertGPUCollectorSnapshot(jobID, userID, projectID, snapshot.Data)
	}
	if matchingSnapshots != 2 {
		h.t.Fatalf("matching snapshots = %d, want 2", matchingSnapshots)
	}
}

func (h *e2eHarness) assertGPUCollectorSnapshot(jobID, userID, projectID string, data map[string]any) {
	h.t.Helper()
	metrics, ok := data["metrics"].(map[string]any)
	if !ok {
		h.t.Fatalf("snapshot metrics = %#v, want object", data["metrics"])
	}
	if metrics["gpu_uuid"] != "GPU-"+h.runID || metrics["gpu_memory_bytes"] == nil || metrics["gpu_sm_utilization"] == nil {
		h.t.Fatalf("snapshot metrics = %#v, want normalized GPU memory and SM metrics", metrics)
	}
	if data["job_id"] != jobID {
		h.t.Fatalf("snapshot job = %#v, want %s", data["job_id"], jobID)
	}
	if data["user_id"] != userID || data["project_id"] != projectID {
		h.t.Fatalf("snapshot identity = %#v, want user/project", data)
	}
}

func (h *e2eHarness) assertGPUCollectorSummary(jobID string) {
	h.t.Helper()
	summary, ok := h.findRecordDataBy(gpuSummariesResource, "job_id", jobID)
	if !ok {
		h.t.Fatalf("missing GPU summary for job %s", jobID)
	}
	metrics, ok := summary["metrics"].(map[string]any)
	if !ok {
		h.t.Fatalf("summary metrics = %#v, want object", summary["metrics"])
	}
	if got := numberValue(metrics["total_gpu_seconds"]); got <= 0 {
		h.t.Fatalf("total_gpu_seconds = %v, want > 0", got)
	}
	if got := numberValue(metrics["sample_count"]); got != 2 {
		h.t.Fatalf("sample_count = %v, want 2", got)
	}
}

func (h *e2eHarness) assertGPUUsageAPIs(jobID, userID, projectID string) {
	h.t.Helper()
	adminUsage := h.do(h.newRequest(usageObservabilityService, http.MethodGet, "/api/v1/admin/usage?since=2026-01-01", nil, h.apiKey), http.StatusOK)
	usageRows := responseDataSlice(h.t, adminUsage)
	if !containsUsageRow(usageRows, jobID, userID, projectID) {
		h.t.Fatalf("admin usage rows = %#v, want collector-produced job/user/project row", usageRows)
	}

	adminJobs := h.do(h.newRequest(usageObservabilityService, http.MethodGet, "/api/v1/admin/gpu/users/"+userID+"/jobs?since=2026-01-01", nil, h.apiKey), http.StatusOK)
	jobRows := responseDataSlice(h.t, adminJobs)
	if !containsGPUJobRow(jobRows, jobID) {
		h.t.Fatalf("admin GPU job rows = %#v, want summarized job %s", jobRows, jobID)
	}
}

func (h *e2eHarness) seedGPUUsageReadModels(userID, projectID, jobID string, now time.Time) {
	h.t.Helper()
	h.createRecord(gpuIdentityUsersResource, "admin-"+h.runID, map[string]any{
		"user_id":      "admin-" + h.runID,
		"username":     "admin-" + h.runID,
		"capabilities": map[string]any{"adminPanel": true},
	})
	h.createRecord(gpuIdentityUsersResource, userID, map[string]any{
		"user_id":  userID,
		"username": "gpu-user-" + h.runID,
	})
	h.createRecord(gpuProjectsResource, projectID, map[string]any{
		"p_id":         projectID,
		"project_name": "gpu-project-" + h.runID,
	})
	h.createRecord(gpuJobsResource, jobID, map[string]any{
		"job_id":     jobID,
		"user_id":    userID,
		"project_id": projectID,
		"queue_name": "gpuq-" + h.runID,
		"status":     "succeeded",
	})
	h.createRecord(gpuReadModelsResource, "cluster-"+h.runID, map[string]any{
		"summary": map[string]any{
			"podGpuUsages": []any{
				gpuPodUsageRow(h, userID, projectID, jobID, now.Add(-time.Minute), 65, 4096),
				gpuPodUsageRow(h, userID, projectID, jobID, now, 85, 8192),
				map[string]any{
					"podName":   "incomplete-" + h.runID,
					"namespace": "project-" + projectID,
					"gpuIndex":  0,
				},
			},
		},
	})
}

func gpuPodUsageRow(h *e2eHarness, userID, projectID, jobID string, sampledAt time.Time, smUtilization float64, memoryUsed int64) map[string]any {
	return map[string]any{
		"job_id":              jobID,
		"user_id":             userID,
		"project_id":          projectID,
		"podName":             "trainer-" + h.runID,
		"namespace":           "project-" + projectID,
		"node":                "gpu-node-" + h.runID,
		"gpuIndex":            0,
		"gpuUuid":             "GPU-" + h.runID,
		"mpsPhysicalGPUIndex": 0,
		"mpsVirtualUnits":     50,
		"timestamp":           sampledAt.Format(time.RFC3339Nano),
		"memoryBytes":         16384,
		"gpuSMUtilization":    smUtilization,
		"gpuMemoryUsedBytes":  memoryUsed,
		"cpuUsageCores":       2,
		"memoryUsageBytes":    2 * 1024 * 1024 * 1024,
		"e2e_run_id":          h.runID,
	}
}

func (h *e2eHarness) findRecordDataBy(resource, key, value string) (map[string]any, bool) {
	h.t.Helper()
	for _, record := range h.listRecords(resource) {
		if record.Data[key] == value {
			return record.Data, true
		}
	}
	return nil, false
}

func responseDataSlice(t *testing.T, resp testResponse) []any {
	t.Helper()
	data, ok := resp.envelope(t)["data"].([]any)
	if !ok {
		t.Fatalf("response data = %#v, want array", resp.envelope(t)["data"])
	}
	return data
}

func containsUsageRow(rows []any, jobID, userID, projectID string) bool {
	for _, row := range rows {
		data, ok := row.(map[string]any)
		if !ok {
			continue
		}
		if textAny(data["JobID"]) == jobID && textAny(data["UserID"]) == userID && textAny(data["ProjectID"]) == projectID {
			return true
		}
	}
	return false
}

func containsGPUJobRow(rows []any, jobID string) bool {
	for _, row := range rows {
		data, ok := row.(map[string]any)
		if !ok {
			continue
		}
		if textAny(data["job_id"]) == jobID || textAny(data["JobID"]) == jobID {
			return true
		}
	}
	return false
}

func numberValue(value any) float64 {
	switch typed := value.(type) {
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	case float64:
		return typed
	case float32:
		return float64(typed)
	default:
		return 0
	}
}

func textAny(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	return ""
}
