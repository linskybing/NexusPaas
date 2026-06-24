package schedulerquota

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

const (
	submitAdmissionsResource = serviceName + ":submit_admissions"
	defaultDeviceClassName   = "gpu.nvidia.com"
	defaultQueueName         = "default-batch"
)

var activeAdmissionStatuses = map[string]bool{
	"submitted":     true,
	"waiting_infra": true,
	"queued":        true,
	"running":       true,
}

var queuedAdmissionStatuses = map[string]bool{
	"submitted":     true,
	"waiting_infra": true,
	"queued":        true,
}

type submitAdmissionRequest struct {
	JobID                  string
	ProjectID              string
	UserID                 string
	QueueName              string
	DeviceClassName        string
	RequiredGPU            float64
	RequiredCPU            float64
	RequiredMemory         int
	GPUCount               int
	SMPercentage           *int
	MPSShareProjectID      string
	StreamingSession       bool
	StreamMaxBitrateKbps   int
	StreamBitrateCapKbps   int
	StreamSessionCap       int
	StreamEgressBudgetKbps int
	Resources              []admissionResourcePayload
}

type admissionResourcePayload struct {
	Name string
	Kind string
	Raw  []byte
}

type admissionReview struct {
	Allowed              bool
	Reason               string
	ProjectID            string
	UserID               string
	QueueName            string
	QueuePriority        int
	QueuePreemptible     bool
	RuntimeLimit         int
	DeviceClassName      string
	RequiredGPU          float64
	RequiredCPU          float64
	RequiredMemory       int
	StreamingSession     bool
	StreamMaxBitrateKbps int
	Usage                admissionUsage
}

type admissionUsage struct {
	ProjectGPU              float64
	ProjectCPU              float64
	ProjectMemoryMB         int
	UserGPU                 float64
	UserCPU                 float64
	UserMemoryMB            int
	UserRunningJobs         int
	UserQueuedJobs          int
	ResourceFloorGPU        float64
	ResourceFloorCPU        float64
	FloorMemoryMB           int
	ActiveStreamSessions    int
	ActiveStreamBitrateKbps int
	StreamEgressBudgetKbps  int
}

type admissionDeny struct {
	status int
	reason string
}

func (e admissionDeny) Error() string {
	return e.reason
}

func reviewSubmitAdmission(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	payload, err := decodePayload(r)
	if err != nil {
		return platform.InputLimitStatus(err, http.StatusBadRequest), shared.ErrorData(platform.InputLimitMessage(err, msgInvalidBody)), nil
	}
	req, err := decodeSubmitAdmissionRequest(payload)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(err.Error()), nil
	}
	applyAdmissionStreamConfig(&req, app.Config)
	if req.QueueName == "" {
		req.QueueName = shared.FirstNonEmpty(strings.TrimSpace(app.Config.DefaultQueueName), defaultQueueName)
	}
	if err := validateAdmissionResourceManifestLimits(req, app.Config); err != nil {
		return platform.InputLimitStatus(err, http.StatusUnprocessableEntity), admissionDenied(req, platform.InputLimitMessage(err, err.Error())), nil
	}
	if violation, found := admissionSecretPolicyViolationFromRequest(req); found {
		publishSecretAccessRejected(app, r, req, violation)
		return http.StatusForbidden, admissionDenied(req, violation.Reason), nil
	}
	if app.Store == nil {
		return http.StatusServiceUnavailable, admissionDenied(req, "submit policy store is not configured"), nil
	}
	repo := schedulerQuotaRepositoryForApp(app)
	if repo == nil {
		return http.StatusServiceUnavailable, admissionDenied(req, "submit policy store is not configured"), nil
	}
	review, err := evaluateSubmitAdmission(r.Context(), newAdmissionReaderForApp(app), req, time.Now().UTC())
	if err != nil {
		status := http.StatusUnprocessableEntity
		var denied admissionDeny
		if errors.As(err, &denied) {
			status = denied.status
		}
		return status, admissionDeniedReview(req, review, err.Error()), nil
	}
	persistAdmissionReview(r.Context(), repo, review)
	publish(app, r, "SubmitAdmissionReviewed", "allowed", admissionReviewData(review))
	return http.StatusOK, admissionReviewData(review), nil
}

func evaluateSubmitAdmission(ctx context.Context, reader admissionReader, req submitAdmissionRequest, now time.Time) (admissionReview, error) {
	review := admissionReview{
		Allowed:              true,
		ProjectID:            req.ProjectID,
		UserID:               req.UserID,
		RequiredGPU:          req.RequiredGPU,
		RequiredCPU:          req.RequiredCPU,
		RequiredMemory:       req.RequiredMemory,
		StreamingSession:     req.StreamingSession,
		StreamMaxBitrateKbps: req.StreamMaxBitrateKbps,
	}
	if strings.TrimSpace(req.ProjectID) == "" {
		return review, deny(http.StatusBadRequest, "project id is required")
	}
	if strings.TrimSpace(req.UserID) == "" {
		return review, deny(http.StatusBadRequest, "user id is required")
	}

	project, found := reader.Project(ctx, req.ProjectID)
	if !found {
		return review, deny(http.StatusNotFound, "project not found")
	}
	plan, err := admissionProjectPlan(ctx, reader, project, now)
	if err != nil {
		return review, err
	}
	if err := requireAdmissionProjectAccess(ctx, reader, project, req.UserID); err != nil {
		return review, err
	}

	queueName := strings.TrimSpace(req.QueueName)
	if queueName == "" {
		queueName = schedulerDefaultQueueName()
	}
	if !admissionQueueAllowed(ctx, reader, plan, queueName) {
		return review, deny(http.StatusForbidden, "queue is not allowed by project plan")
	}
	review.QueueName = queueName
	if queue, found := admissionQueueByName(ctx, reader, queueName); found {
		review.QueuePriority = shared.IntValue(queue.Data, "priority_value", "priorityValue", "priority")
		review.QueuePreemptible = shared.BoolValue(queue.Data, "is_preemptible", "isPreemptible", "preemptible")
		review.RuntimeLimit = shared.IntValue(queue.Data, "max_runtime_seconds", "maxRuntimeSeconds", "runtime_limit_seconds", "runtimeLimitSeconds")
	}

	if err := enforceAdmissionGPUPolicy(plan, &req); err != nil {
		return review, err
	}
	review.DeviceClassName = req.DeviceClassName

	floor, err := admissionResourceFloorFromRequest(req)
	if err != nil {
		return review, deny(http.StatusUnprocessableEntity, err.Error())
	}
	review.Usage.ResourceFloorGPU = floor.gpu
	review.Usage.ResourceFloorCPU = floor.cpu
	review.Usage.FloorMemoryMB = floor.memoryMB
	if err := enforceAdmissionResourceFloor(req, floor); err != nil {
		return review, deny(http.StatusUnprocessableEntity, err.Error())
	}
	review.Usage = admissionUsageForJobs(ctx, reader, req.ProjectID, req.UserID, review.Usage)
	review.Usage.StreamEgressBudgetKbps = req.StreamEgressBudgetKbps
	if err := enforceAdmissionStreaming(req, review.Usage); err != nil {
		return review, err
	}
	if err := enforceAdmissionJobLimits(project, review.Usage); err != nil {
		return review, err
	}
	if err := enforceAdmissionQuota(plan, req, review.Usage); err != nil {
		return review, err
	}
	if err := enforceAdmissionUserQuota(ctx, reader, req, review.Usage); err != nil {
		return review, err
	}
	return review, nil
}

func deny(status int, reason string) error {
	return admissionDeny{status: status, reason: reason}
}

func schedulerDefaultQueueName() string {
	return defaultQueueName
}

func validateAdmissionResourceManifestLimits(req submitAdmissionRequest, cfg platform.Config) error {
	for _, resource := range req.Resources {
		if len(resource.Raw) == 0 {
			continue
		}
		if err := platform.ValidateManifestLimits(resource.Raw, cfg.EffectiveMaxConfigFileBytes(), cfg.EffectiveMaxConfigFileDocuments()); err != nil {
			return err
		}
	}
	return nil
}

func publishSecretAccessRejected(app *platform.App, r *http.Request, req submitAdmissionRequest, violation admissionSecretPolicyViolation) {
	if app == nil || app.Events == nil {
		return
	}
	data := secretAccessRejectedData(req, violation)
	publish(app, r, "SecretAccessRejected", "rejected", data)
	auditData := shared.CloneMap(data)
	auditData["action"] = "rejected"
	auditData["actor_user_id"] = shared.FirstNonEmpty(r.Header.Get("X-User-ID"), req.UserID, "anonymous")
	auditData["resource_type"] = "secret"
	auditData["resource_id"] = violation.ResourceName
	auditData["success"] = false
	auditData["description"] = violation.Reason
	_ = app.Events.Publish(r.Context(), contracts.Event{
		EventID:        platform.NewUUID(),
		Name:           "AuditEvent",
		Source:         serviceName,
		OccurredAt:     time.Now().UTC(),
		TraceID:        shared.FirstNonEmpty(r.Header.Get("X-Trace-ID"), r.Header.Get("X-Request-ID"), "scheduler-quota-local"),
		SchemaVersion:  1,
		IdempotencyKey: r.Header.Get("Idempotency-Key"),
		Data:           auditData,
	})
}

func secretAccessRejectedData(req submitAdmissionRequest, violation admissionSecretPolicyViolation) map[string]any {
	return map[string]any{
		"project_id":    req.ProjectID,
		"user_id":       req.UserID,
		"job_id":        req.JobID,
		"resource_type": "secret",
		"resource_id":   violation.ResourceName,
		"resource_kind": shared.FirstNonEmpty(violation.ResourceKind, "Secret"),
		"resource_name": violation.ResourceName,
		"reason":        violation.Reason,
		"success":       false,
	}
}
