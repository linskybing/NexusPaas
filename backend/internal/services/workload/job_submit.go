package workload

import (
	"net/http"
	"strings"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

const (
	defaultJobPrefix  = "J"
	defaultJobIDStart = 2600001
	defaultJobIDWidth = 7
)

type jobSubmitError struct {
	status  int
	message string
}

func (e jobSubmitError) Error() string {
	return e.message
}

func submitJob(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	payload, err := platform.DecodeMapWithError(r)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(msgInvalidBody), nil
	}
	jobs := jobRepository(app)
	if jobs == nil {
		return http.StatusInternalServerError, shared.ErrorData("job repository unavailable"), nil
	}
	job, admissionPayload, err := buildSubmittedJob(app, r, payload, jobs)
	if err != nil {
		submitErr := jobSubmitError{status: http.StatusBadRequest, message: err.Error()}
		if typed, ok := err.(jobSubmitError); ok {
			submitErr = typed
		}
		return submitErr.status, shared.ErrorData(submitErr.message), nil
	}
	status, review, err := reviewJobAdmission(app, r, admissionPayload)
	if err != nil {
		return http.StatusServiceUnavailable, shared.ErrorData("scheduler admission unavailable"), nil
	}
	if status >= http.StatusBadRequest || !admissionAllowed(review) {
		if status < http.StatusBadRequest {
			status = http.StatusForbidden
		}
		return status, admissionDenialData(review, "submit rejected by scheduler admission"), nil
	}
	applyAdmissionReview(job, review)
	record, err := jobs.CreateSubmittedJob(r.Context(), job)
	if err != nil {
		return createStatus(err), shared.ErrorData("job could not be submitted"), nil
	}
	publish(app, r, "JobSubmitted", "submitted", record.Data)
	return http.StatusCreated, record, nil
}

func buildSubmittedJob(app *platform.App, r *http.Request, payload map[string]any, jobs workloadJobRepository) (map[string]any, map[string]any, error) {
	submitType := shared.FirstNonEmpty(shared.TextValue(payload, "submit_type", "submitType"), "job")
	if !strings.EqualFold(submitType, "job") {
		return nil, nil, jobSubmitError{status: http.StatusBadRequest, message: "submit_type must be job"}
	}
	jobCtx, err := resolveJobSubmitContext(configRepository(app), r, payload)
	if err != nil {
		return nil, nil, err
	}
	userID := shared.FirstNonEmpty(
		shared.TextValue(payload, "user_id", "userId", "UserID"),
		strings.TrimSpace(r.Header.Get("X-User-ID")),
		strings.TrimSpace(r.Header.Get("X-Username")),
	)
	if userID == "" {
		return nil, nil, jobSubmitError{status: http.StatusBadRequest, message: "user_id is required"}
	}
	jobID := shared.FirstNonEmpty(
		shared.TextValue(payload, "job_id", "jobId", "id"),
		jobs.NextJobID(),
	)
	now := time.Now().UTC().Format(time.RFC3339)
	job := shared.CloneMap(payload)
	job["id"] = jobID
	job["job_id"] = jobID
	job["project_id"] = jobCtx.projectID
	job["user_id"] = userID
	job["status"] = "submitted"
	job["submit_type"] = "job"
	job["executor_type"] = shared.FirstNonEmpty(shared.TextValue(payload, "executor_type", "executorType"), "scheduler")
	job["namespace"] = jobNamespace(app.Config.K8sNamespacePrefix, jobCtx.projectID, payload)
	job["submission_payload"] = shared.CloneMap(payload)
	job["submitted_at"] = now
	job["created_at"] = now
	job["updated_at"] = now
	if jobCtx.configID != "" {
		job["config_id"] = jobCtx.configID
	}
	if jobCtx.configCommitID != "" {
		job["config_commit_id"] = jobCtx.configCommitID
	}
	return job, jobAdmissionPayload(job, payload), nil
}

type jobSubmitContext struct {
	projectID      string
	configID       string
	configCommitID string
}

func resolveJobSubmitContext(configs workloadConfigRepository, r *http.Request, payload map[string]any) (jobSubmitContext, error) {
	jobCtx := jobSubmitContext{
		projectID:      shared.FirstNonEmpty(shared.TextValue(payload, "project_id", "projectId", "ProjectID"), strings.TrimSpace(r.URL.Query().Get("project_id"))),
		configID:       shared.TextValue(payload, "config_id", "configId"),
		configCommitID: shared.TextValue(payload, "config_commit_id", "configCommitId", "config_version_id", "configVersionId"),
	}
	if jobCtx.configCommitID != "" {
		version, found := configs.GetVersion(r.Context(), jobCtx.configCommitID)
		if !found {
			return jobCtx, jobSubmitError{status: http.StatusNotFound, message: "config commit not found"}
		}
		jobCtx.configID = shared.FirstNonEmpty(jobCtx.configID, shared.TextValue(version.Data, "config_id", "configId"))
	}
	if jobCtx.configID != "" {
		config, found := configs.GetConfig(r.Context(), jobCtx.configID)
		if !found {
			return jobCtx, jobSubmitError{status: http.StatusNotFound, message: "config file not found"}
		}
		projectID := shared.TextValue(config.Data, "project_id", "projectId")
		if jobCtx.projectID != "" && projectID != "" && jobCtx.projectID != projectID {
			return jobCtx, jobSubmitError{status: http.StatusBadRequest, message: "project_id does not match config file"}
		}
		jobCtx.projectID = shared.FirstNonEmpty(jobCtx.projectID, projectID)
	}
	if jobCtx.projectID == "" {
		return jobCtx, jobSubmitError{status: http.StatusBadRequest, message: "project_id is required"}
	}
	return jobCtx, nil
}

func jobAdmissionPayload(job, payload map[string]any) map[string]any {
	admission := map[string]any{
		"job_id":            job["job_id"],
		"project_id":        job["project_id"],
		"user_id":           job["user_id"],
		"queue_name":        firstPayloadValue(payload, "queue_name", "queueName", "QueueName"),
		"device_class_name": firstPayloadValue(payload, "device_class_name", "deviceClassName", "DeviceClassName"),
		"required_gpu":      firstPayloadValue(payload, "required_gpu", "requiredGpu", "RequiredGPU"),
		"required_cpu":      firstPayloadValue(payload, "required_cpu", "requiredCPU", "required_cpu_cores", "RequiredCPU"),
		"required_memory":   firstPayloadValue(payload, "required_memory", "requiredMemory", "required_memory_mb", "RequiredMemory"),
		"gpu_count":         firstPayloadValue(payload, "gpu_count", "gpuCount", "GPUCount"),
		"resources":         payload["resources"],
	}
	if value, ok := firstPresent(payload, "sm_percentage", "smPercentage", "SMPercentage"); ok {
		admission["sm_percentage"] = value
	}
	return admission
}

func applyAdmissionReview(job, review map[string]any) {
	for _, key := range []string{"queue_name", "device_class_name", "required_gpu", "required_cpu", "required_memory"} {
		if value, ok := review[key]; ok {
			job[key] = value
		}
	}
	if value, ok := review["usage"]; ok {
		job["admission_usage"] = value
	}
}

func reviewJobAdmission(app *platform.App, r *http.Request, payload map[string]any) (int, map[string]any, error) {
	client, err := newSchedulerAdmissionClient(app)
	if err != nil {
		return 0, nil, err
	}
	result, err := client.Review(r.Context(), r.Header, payload)
	return result.StatusCode, result.Data, err
}

func admissionAllowed(review map[string]any) bool {
	allowed, ok := review["allowed"].(bool)
	return ok && allowed
}

func admissionDenialData(review map[string]any, fallback string) map[string]any {
	data := shared.CloneMap(review)
	if shared.TextValue(data, "reason") == "" {
		data["reason"] = fallback
	}
	if _, ok := data["allowed"]; !ok {
		data["allowed"] = false
	}
	return data
}

func jobNamespace(prefix, projectID string, payload map[string]any) string {
	if namespace := shared.TextValue(payload, "namespace"); namespace != "" {
		return namespace
	}
	if projectID == "" {
		return ""
	}
	prefix = shared.FirstNonEmpty(strings.TrimSpace(prefix), "proj")
	return strings.ToLower(prefix + "-" + projectID)
}

func firstPayloadValue(payload map[string]any, keys ...string) any {
	value, _ := firstPresent(payload, keys...)
	return value
}

func firstPresent(payload map[string]any, keys ...string) (any, bool) {
	for _, key := range keys {
		value, ok := payload[key]
		if ok && value != nil {
			return value, true
		}
	}
	return nil, false
}
