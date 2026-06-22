package workload

import (
	"net/http"
	"strings"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
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
	if status, data, ok := requireProjectAccess(app, r, shared.TextValue(job, "project_id", "projectId")); !ok {
		return status, data, nil
	}
	status, review, preemption, denial, err := admitSubmittedJob(app, r, job, admissionPayload)
	if err != nil {
		return http.StatusServiceUnavailable, shared.ErrorData("scheduler admission unavailable"), nil
	}
	if denial != nil {
		return status, denial, nil
	}
	applyAdmissionReview(job, review)
	applyAutoPreemptionResult(job, preemption)
	record, err := jobs.CreateSubmittedJobWithEvent(r.Context(), app, job, func(record contracts.Record[map[string]any]) contracts.Event {
		return buildEvent(r, "JobSubmitted", "submitted", record.Data)
	})
	if err != nil {
		return createStatus(err), shared.ErrorData("job could not be submitted"), nil
	}
	return http.StatusCreated, record, nil
}

func admitSubmittedJob(app *platform.App, r *http.Request, job, admissionPayload map[string]any) (int, map[string]any, map[string]any, map[string]any, error) {
	status, review, err := reviewJobAdmission(app, r, admissionPayload)
	if err != nil {
		return 0, nil, nil, nil, err
	}
	var preemption map[string]any
	if status >= http.StatusBadRequest || !admissionAllowed(review) {
		status, review, preemption, err = autoPreemptAndRetryAdmission(app, r, job, admissionPayload, status, review)
		if err != nil {
			return 0, nil, nil, nil, err
		}
	}
	if status < http.StatusBadRequest && admissionAllowed(review) {
		return status, review, preemption, nil, nil
	}
	if status < http.StatusBadRequest {
		status = http.StatusForbidden
	}
	denial := admissionDenialWithPreemptionData(review, preemption, "submit rejected by scheduler admission")
	return status, review, preemption, denial, nil
}

func buildSubmittedJob(app *platform.App, r *http.Request, payload map[string]any, jobs *recordStoreWorkloadJobRepository) (map[string]any, map[string]any, error) {
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

func resolveJobSubmitContext(configs *recordStoreWorkloadConfigRepository, r *http.Request, payload map[string]any) (jobSubmitContext, error) {
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
		"job_id":                  job["job_id"],
		"project_id":              job["project_id"],
		"user_id":                 job["user_id"],
		"queue_name":              firstPayloadValue(payload, "queue_name", "queueName", "QueueName"),
		"device_class_name":       firstPayloadValue(payload, "device_class_name", "deviceClassName", "DeviceClassName"),
		"required_gpu":            firstPayloadValue(payload, "required_gpu", "requiredGpu", "RequiredGPU"),
		"required_cpu":            firstPayloadValue(payload, "required_cpu", "requiredCPU", "required_cpu_cores", "RequiredCPU"),
		"required_memory":         firstPayloadValue(payload, "required_memory", "requiredMemory", "required_memory_mb", "RequiredMemory"),
		"gpu_count":               firstPayloadValue(payload, "gpu_count", "gpuCount", "GPUCount"),
		"streaming_session":       firstPayloadValue(payload, "streaming_session", "streamingSession", "StreamingSession"),
		"stream_max_bitrate_kbps": firstPayloadValue(payload, "stream_max_bitrate_kbps", "streamMaxBitrateKbps", "StreamMaxBitrateKbps"),
		"resources":               payload["resources"],
	}
	if value, ok := firstPresent(payload, "sm_percentage", "smPercentage", "SMPercentage"); ok {
		admission["sm_percentage"] = value
	}
	return admission
}

func applyAdmissionReview(job, review map[string]any) {
	for _, key := range []string{
		"queue_name",
		"priority_value",
		"preemptible",
		"is_preemptible",
		"runtime_limit_seconds",
		"max_runtime_seconds",
		"device_class_name",
		"required_gpu",
		"required_cpu",
		"required_memory",
		"streaming_session",
		"stream_max_bitrate_kbps",
	} {
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

func autoPreemptAndRetryAdmission(
	app *platform.App,
	r *http.Request,
	job, admissionPayload map[string]any,
	status int,
	review map[string]any,
) (int, map[string]any, map[string]any, error) {
	if !shouldAutoPreemptAdmissionDenial(status, review) {
		return status, review, nil, nil
	}
	payload, ok := schedulerPreemptionPayload(job, admissionPayload, review)
	if !ok {
		return status, review, nil, nil
	}
	client, err := newSchedulerPreemptionClient(app)
	if err != nil {
		return status, review, map[string]any{"status": "unavailable", "reason": err.Error()}, nil
	}
	headers := autoPreemptionHeaders(r, job)
	result, err := client.Preempt(r.Context(), headers, payload)
	if err != nil {
		return status, review, map[string]any{"status": "unavailable", "reason": err.Error()}, nil
	}
	preemption := shared.CloneMap(result.Data)
	preemption["http_status"] = result.StatusCode
	if result.StatusCode != http.StatusOK || !shared.BoolValue(preemption, "accepted") || shared.TextValue(preemption, "status") != "completed" {
		return status, admissionDenialWithPreemptionData(review, preemption, "submit rejected by scheduler admission"), preemption, nil
	}
	retryStatus, retryReview, err := reviewJobAdmission(app, r, admissionPayload)
	if err != nil {
		return retryStatus, retryReview, preemption, err
	}
	return retryStatus, retryReview, preemption, nil
}

func shouldAutoPreemptAdmissionDenial(status int, review map[string]any) bool {
	if status != http.StatusConflict {
		return false
	}
	reason := strings.ToLower(shared.TextValue(review, "reason"))
	return strings.Contains(reason, "quota exceeded") ||
		strings.Contains(reason, "resource shortage") ||
		strings.Contains(reason, "insufficient resource")
}

func schedulerPreemptionPayload(job, admissionPayload, review map[string]any) (map[string]any, bool) {
	priority := shared.IntValue(review, "priority_value", "priorityValue", "priority")
	requiredGPU := shared.NumberValue(review, "required_gpu", "requiredGpu", "RequiredGPU")
	requiredCPU := shared.NumberValue(review, "required_cpu", "requiredCPU", "required_cpu_cores", "RequiredCPU")
	if priority <= 0 || (requiredGPU <= 0 && requiredCPU <= 0) {
		return nil, false
	}
	jobID := shared.TextValue(job, "job_id", "jobId", "id")
	payload := map[string]any{
		"requester_job_id":  jobID,
		"project_id":        shared.TextValue(review, "project_id", "projectId"),
		"queue_name":        shared.FirstNonEmpty(shared.TextValue(review, "queue_name", "queueName"), shared.TextValue(admissionPayload, "queue_name", "queueName")),
		"priority_value":    priority,
		"required_gpu":      requiredGPU,
		"required_cpu":      requiredCPU,
		"device_class_name": shared.FirstNonEmpty(shared.TextValue(review, "device_class_name", "deviceClassName"), shared.TextValue(admissionPayload, "device_class_name", "deviceClassName")),
		"gpu_model":         shared.TextValue(admissionPayload, "gpu_model", "gpuModel", "dra_gpu_model", "draGPUModel"),
	}
	for key, value := range payload {
		if text, ok := value.(string); ok && text == "" {
			delete(payload, key)
		}
	}
	return payload, jobID != ""
}

func autoPreemptionHeaders(r *http.Request, job map[string]any) http.Header {
	headers := r.Header.Clone()
	jobID := shared.TextValue(job, "job_id", "jobId", "id")
	key := strings.TrimSpace(headers.Get("Idempotency-Key"))
	if key == "" {
		key = jobID
	}
	headers.Set("Idempotency-Key", "auto-preempt:"+jobID+":"+key)
	return headers
}

func applyAutoPreemptionResult(job, preemption map[string]any) {
	if len(preemption) == 0 {
		return
	}
	job["admission_preemption"] = shared.CloneMap(preemption)
	if id := shared.TextValue(preemption, "preemption_id", "id"); id != "" {
		job["admission_preemption_id"] = id
	}
	if status := shared.TextValue(preemption, "status"); status != "" {
		job["admission_preemption_status"] = status
	}
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

func admissionDenialWithPreemptionData(review, preemption map[string]any, fallback string) map[string]any {
	data := admissionDenialData(review, fallback)
	if len(preemption) > 0 {
		data["auto_preemption"] = shared.CloneMap(preemption)
	}
	return data
}

func jobNamespace(prefix, projectID string, payload map[string]any) string {
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
