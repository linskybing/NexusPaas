package workload

import (
	"net/http"
	"strings"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

const jobStatusPreempted = "preempted"

var preemptibleJobStatuses = map[string]bool{
	jobStatusRunning:    true,
	"partial-preempted": true,
}

var terminalJobStatuses = map[string]bool{
	jobStatusCompleted: true,
	jobStatusFailed:    true,
	"cancelled":        true,
	jobStatusPreempted: true,
	"evicted":          true,
}

func workloadPreemptionContext(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	if status, data, ok := requireWorkloadServiceAuth(app, r); !ok {
		return status, data, nil
	}
	requesterID := strings.TrimSpace(r.URL.Query().Get("requester_job_id"))
	response := map[string]any{"candidates": workloadPreemptionCandidates(app, r)}
	if requesterID != "" {
		record, found := findWorkloadJob(app, r, requesterID)
		if !found {
			return http.StatusNotFound, shared.ErrorData("requester job not found"), nil
		}
		response["requester"] = workloadPreemptionJobData(record)
	}
	return http.StatusOK, response, nil
}

func workloadPreemptJob(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	if status, data, ok := requireWorkloadServiceAuth(app, r); !ok {
		return status, data, nil
	}
	payload, err := platform.DecodeMapWithError(r)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(msgInvalidBody), nil
	}
	id := strings.TrimSpace(r.PathValue("id"))
	preemptionID := shared.TextValue(payload, "preemption_id", "preemptionId")
	if id == "" || preemptionID == "" {
		return http.StatusBadRequest, shared.ErrorData("job id and preemption_id are required"), nil
	}
	record, found := findWorkloadJob(app, r, id)
	if !found {
		return http.StatusNotFound, shared.ErrorData("job not found"), nil
	}
	currentStatus := currentJobStatus(record.Data)
	currentPreemptionID := shared.TextValue(record.Data, "preemption_record_id", "preemptionRecordId")
	if currentStatus == jobStatusPreempted {
		if currentPreemptionID == preemptionID {
			return http.StatusOK, record, nil
		}
		return http.StatusConflict, shared.ErrorData("job is already preempted by another preemption record"), nil
	}
	if terminalJobStatuses[currentStatus] || !preemptibleJobStatuses[currentStatus] {
		return http.StatusConflict, shared.ErrorData("job is not active for preemption"), nil
	}
	now := time.Now().UTC()
	updated, ok := jobRepository(app).MarkPreempted(r.Context(), record.ID, jobPreemptionUpdate{
		PreemptionID: preemptionID,
		RequesterID:  shared.TextValue(payload, "requester_job_id", "requesterJobId"),
		Reason:       shared.TextValue(payload, "reason"),
		Cleanup:      shared.MapValue(payload, "cleanup"),
		PreemptedAt:  now,
		CompletedAt:  now,
	})
	if !ok {
		return http.StatusInternalServerError, shared.ErrorData("job preemption status update failed"), nil
	}
	releaseSubmittedJobReservation(r.Context(), app, r.Header, updated.Data)
	return http.StatusOK, updated, nil
}

func requireWorkloadServiceAuth(app *platform.App, r *http.Request) (int, map[string]any, bool) {
	if app.Config.ServiceAPIKey == "" {
		return http.StatusNotFound, shared.ErrorData("not found"), false
	}
	if !app.ServiceRequestAuthorized(r) {
		return http.StatusUnauthorized, shared.ErrorData("service authentication is required"), false
	}
	return 0, nil, true
}

func workloadPreemptionCandidates(app *platform.App, r *http.Request) []map[string]any {
	records := jobRepository(app).ListPreemptionCandidates(r.Context())
	candidates := make([]map[string]any, 0, len(records))
	for _, record := range records {
		candidates = append(candidates, workloadPreemptionJobData(record))
	}
	return candidates
}

func findWorkloadJob(app *platform.App, r *http.Request, id string) (contracts.Record[map[string]any], bool) {
	return jobRepository(app).FindJob(r.Context(), id)
}

func workloadPreemptionJobData(record contracts.Record[map[string]any]) map[string]any {
	data := record.Data
	createdAt := shared.FirstNonEmpty(
		shared.TextValue(data, "created_at", "createdAt", "submitted_at", "submittedAt"),
		record.CreatedAt.UTC().Format(time.RFC3339),
	)
	jobID := shared.FirstNonEmpty(shared.TextValue(data, "job_id", "jobId"), record.ID)
	return map[string]any{
		"id":                   record.ID,
		"job_id":               jobID,
		"project_id":           shared.TextValue(data, "project_id", "projectId"),
		"user_id":              shared.TextValue(data, "user_id", "userId"),
		"queue_name":           shared.TextValue(data, "queue_name", "queueName"),
		"namespace":            shared.TextValue(data, "namespace", "Namespace", "pod_namespace", "podNamespace"),
		"status":               currentJobStatus(data),
		"priority_value":       jobPriority(data),
		"preemptible":          workloadJobPreemptible(data),
		"required_gpu":         shared.NumberValue(data, "required_gpu", "requiredGpu", "RequiredGPU"),
		"required_cpu":         shared.NumberValue(data, "required_cpu", "requiredCPU", "required_cpu_cores", "RequiredCPU"),
		"required_memory":      shared.IntValue(data, "required_memory", "requiredMemory", "required_memory_mb", "RequiredMemory"),
		"device_class_name":    shared.TextValue(data, "device_class_name", "deviceClassName", "DeviceClassName"),
		"gpu_model":            shared.TextValue(data, "gpu_model", "gpuModel", "dra_gpu_model", "draGPUModel"),
		"created_at":           createdAt,
		"preemption_record_id": shared.TextValue(data, "preemption_record_id", "preemptionRecordId"),
	}
}

func workloadJobPreemptible(data map[string]any) bool {
	if shared.BoolValue(data, "is_preemptible", "isPreemptible", "preemptible") {
		return true
	}
	priority := jobPriority(data)
	return priority > 0 && priority < 10000
}
