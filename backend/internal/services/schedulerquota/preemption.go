package schedulerquota

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/platform/cluster"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

const (
	defaultMaxPreemptions = 5

	internalPreemptionIdempotencyKeyHash = "internal_preemption_idempotency_key_hash"
	internalPreemptionFingerprintHash    = "internal_preemption_fingerprint_hash"
	internalPreemptionRecordID           = "internal_preemption_record_id"
)

type preemptionRequest struct {
	IdempotencyKey string
	RequesterJobID string
	ProjectID      string
	QueueName      string
	DeviceClass    string
	GPUModel       string
	PriorityValue  int
	RequiredGPU    float64
	RequiredCPU    float64
	MaxPreemptions int
	Override       bool
	Fingerprint    string
}

type preemptionTarget struct {
	workloadJobSnapshot
}

func handlePreemption(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	payload, err := decodePayload(r)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(msgInvalidBody), nil
	}
	req, err := decodePreemptionRequest(app, r, payload)
	if err != nil {
		return preemptionErrorStatus(err), shared.ErrorData(err.Error()), nil
	}
	repo := schedulerPreemptionPriorityRepositoryForApp(app)
	if repo == nil {
		return http.StatusInternalServerError, shared.ErrorData("preemption repository unavailable"), nil
	}
	recordID := preemptionRecordID(req.IdempotencyKey)
	if record, found := repo.FindPreemptionRecord(r.Context(), recordID); found {
		return replayPreemptionRecord(record.Data, req)
	}
	record, err := repo.CreatePreemptionRecord(r.Context(), initialPreemptionRecord(recordID, req))
	if err != nil {
		if platform.IsCreateConflict(err) {
			existing, _ := repo.FindPreemptionRecord(r.Context(), recordID)
			return replayPreemptionRecord(existing.Data, req)
		}
		return http.StatusInternalServerError, shared.ErrorData("preemption record could not be created"), nil
	}
	client, err := newWorkloadPreemptionClient(app)
	if err != nil {
		data := finishPreemptionRecord(r.Context(), repo, record.ID, "preflight_failed", map[string]any{"reason": err.Error()})
		return http.StatusServiceUnavailable, data, nil
	}
	ctx, err := client.Context(r.Context(), req)
	if err != nil {
		data := finishPreemptionRecord(r.Context(), repo, record.ID, "preflight_failed", map[string]any{"reason": err.Error()})
		return http.StatusServiceUnavailable, data, nil
	}
	resolved, err := trustedPreemptionRequest(app, r, req, ctx)
	if err != nil {
		data := finishPreemptionRecord(r.Context(), repo, record.ID, "preflight_failed", map[string]any{"reason": err.Error()})
		return preemptionErrorStatus(err), data, nil
	}
	req = resolved
	if ctx.Requester != nil && ctx.Requester.Preemptible {
		data := finishPreemptionRecord(r.Context(), repo, record.ID, "no_victims", map[string]any{"reason": "preemptible requester cannot preempt"})
		return http.StatusOK, data, nil
	}
	victims, freedGPU, freedCPU := selectPreemptionVictims(app, req, ctx.Candidates)
	if len(victims) == 0 {
		data := finishPreemptionRecord(r.Context(), repo, record.ID, "no_victims", map[string]any{"reason": "insufficient eligible preemptible resources"})
		return http.StatusOK, data, nil
	}
	if !app.Cluster.Configured() {
		data := finishPreemptionRecord(r.Context(), repo, record.ID, "preflight_failed", map[string]any{"reason": "cluster client is not configured"})
		return http.StatusServiceUnavailable, data, nil
	}
	if err := preflightPreemptionVictims(app, r, client, req, victims); err != nil {
		data := finishPreemptionRecord(r.Context(), repo, record.ID, "preflight_failed", map[string]any{"reason": err.Error()})
		return preemptionErrorStatus(err), data, nil
	}
	status, data := executePreemption(preemptionExecution{
		app:                app,
		request:            r,
		client:             client,
		repo:               repo,
		recordID:           record.ID,
		publicPreemptionID: shared.TextValue(record.Data, "preemption_id", "preemptionId"),
		req:                req,
		victims:            victims,
		freedGPU:           freedGPU,
		freedCPU:           freedCPU,
	})
	return status, data, nil
}

func decodePreemptionRequest(app *platform.App, r *http.Request, payload map[string]any) (preemptionRequest, error) {
	req := preemptionRequest{
		IdempotencyKey: strings.TrimSpace(r.Header.Get("Idempotency-Key")),
		RequesterJobID: shared.FirstNonEmpty(shared.TextValue(payload, "requester_job_id", "requesterJobId"), shared.TextValue(payload, "job_id", "jobId")),
		ProjectID:      shared.TextValue(payload, "project_id", "projectId"),
		QueueName:      shared.TextValue(payload, "queue_name", "queueName"),
		DeviceClass:    shared.TextValue(payload, "device_class_name", "deviceClassName"),
		GPUModel:       shared.TextValue(payload, "gpu_model", "gpuModel"),
		PriorityValue:  shared.IntValue(payload, "priority_value", "priorityValue", "priority"),
		RequiredGPU:    shared.NumberValue(payload, "required_gpu", "requiredGpu", "RequiredGPU"),
		RequiredCPU:    shared.NumberValue(payload, "required_cpu", "requiredCPU", "required_cpu_cores", "RequiredCPU"),
		MaxPreemptions: shared.IntValue(payload, "max_preemptions", "maxPreemptions"),
		Override:       explicitPreemptionOverride(payload),
	}
	if req.IdempotencyKey == "" && req.RequesterJobID != "" {
		req.IdempotencyKey = "requester_job_id:" + req.RequesterJobID
	}
	if req.IdempotencyKey == "" {
		return req, preemptionHTTPError{status: http.StatusBadRequest, message: "Idempotency-Key or requester_job_id is required"}
	}
	if req.Override && !authorizedPreemptionOverride(app, r) {
		return req, preemptionHTTPError{status: http.StatusForbidden, message: "administrator or system principal is required for explicit preemption override"}
	}
	req.MaxPreemptions = clampMaxPreemptions(req.MaxPreemptions)
	req.Fingerprint = preemptionFingerprint(req)
	return req, nil
}

func explicitPreemptionOverride(payload map[string]any) bool {
	return shared.IntValue(payload, "priority_value", "priorityValue", "priority") > 0 ||
		shared.NumberValue(payload, "required_gpu", "requiredGpu", "RequiredGPU") > 0 ||
		shared.NumberValue(payload, "required_cpu", "requiredCPU", "required_cpu_cores", "RequiredCPU") > 0
}

func authorizedPreemptionOverride(app *platform.App, r *http.Request) bool {
	if app.ServiceRequestAuthorized(r) {
		return true
	}
	if !app.Config.RequireAuth {
		return false
	}
	role := strings.ToLower(strings.TrimSpace(r.Header.Get("X-User-Role")))
	switch role {
	case "admin", "superadmin", "root", "system":
		return true
	default:
		return false
	}
}

func trustedPreemptionRequest(app *platform.App, r *http.Request, req preemptionRequest, ctx workloadPreemptionContext) (preemptionRequest, error) {
	if ctx.Requester == nil {
		if req.Override && authorizedPreemptionOverride(app, r) {
			if req.PriorityValue <= 0 || (req.RequiredGPU <= 0 && req.RequiredCPU <= 0) {
				return req, preemptionHTTPError{status: http.StatusUnprocessableEntity, message: "priority and GPU or CPU demand are required"}
			}
			return req, nil
		}
		return req, preemptionHTTPError{status: http.StatusUnprocessableEntity, message: "trusted requester job context is required"}
	}
	requester := ctx.Requester
	req.RequesterJobID = shared.FirstNonEmpty(req.RequesterJobID, requester.JobID, requester.ID)
	req.ProjectID = shared.FirstNonEmpty(requester.ProjectID, req.ProjectID)
	req.QueueName = shared.FirstNonEmpty(requester.QueueName, req.QueueName)
	req.DeviceClass = shared.FirstNonEmpty(requester.DeviceClassName, req.DeviceClass)
	req.GPUModel = shared.FirstNonEmpty(requester.GPUModel, req.GPUModel)
	req.PriorityValue = requester.PriorityValue
	req.RequiredGPU = requester.RequiredGPU
	req.RequiredCPU = requester.RequiredCPU
	if req.PriorityValue <= 0 || (req.RequiredGPU <= 0 && req.RequiredCPU <= 0) {
		return req, preemptionHTTPError{status: http.StatusUnprocessableEntity, message: "trusted requester priority and GPU or CPU demand are required"}
	}
	return req, nil
}

func selectPreemptionVictims(app *platform.App, req preemptionRequest, candidates []workloadJobSnapshot) ([]preemptionTarget, float64, float64) {
	eligible := make([]preemptionTarget, 0, len(candidates))
	for _, candidate := range candidates {
		if !candidateEligibleForPreemption(app, req, candidate) {
			continue
		}
		eligible = append(eligible, preemptionTarget{workloadJobSnapshot: candidate})
	}
	sort.SliceStable(eligible, func(i, j int) bool {
		return preemptionTargetPreferred(eligible[i], eligible[j])
	})
	return selectMinimalPreemptionVictims(req, eligible)
}

func candidateEligibleForPreemption(app *platform.App, req preemptionRequest, candidate workloadJobSnapshot) bool {
	if candidate.JobID == "" || candidate.JobID == req.RequesterJobID || candidate.ID == req.RequesterJobID {
		return false
	}
	if req.ProjectID != "" && candidate.ProjectID != req.ProjectID {
		return false
	}
	if !preemptibleWorkloadStatus(candidate.Status) || candidate.PriorityValue >= req.PriorityValue {
		return false
	}
	if !candidate.Preemptible && !schedulerQueuePreemptible(app, candidate.QueueName) {
		return false
	}
	if req.DeviceClass != "" && !strings.EqualFold(req.DeviceClass, candidate.DeviceClassName) {
		return false
	}
	if req.GPUModel != "" && !strings.EqualFold(req.GPUModel, candidate.GPUModel) {
		return false
	}
	if req.RequiredGPU <= 0 && candidate.RequiredCPU <= 0 {
		return false
	}
	return candidate.RequiredGPU > 0 || candidate.RequiredCPU > 0
}

func preflightPreemptionVictims(app *platform.App, r *http.Request, client internalWorkloadPreemptionClient, req preemptionRequest, victims []preemptionTarget) error {
	ctx, err := client.Context(r.Context(), req)
	if err != nil {
		return preemptionHTTPError{status: http.StatusServiceUnavailable, message: err.Error()}
	}
	current := map[string]workloadJobSnapshot{}
	for _, candidate := range ctx.Candidates {
		current[candidate.JobID] = candidate
		current[candidate.ID] = candidate
	}
	for _, victim := range victims {
		latest, found := current[victim.JobID]
		if !found {
			return preemptionHTTPError{status: http.StatusConflict, message: "selected victim is no longer available"}
		}
		if latest.Namespace == "" || latest.JobID == "" {
			return preemptionHTTPError{status: http.StatusConflict, message: "selected victim namespace and job id are required"}
		}
		if !candidateEligibleForPreemption(app, req, latest) {
			return preemptionHTTPError{status: http.StatusConflict, message: "selected victim is no longer eligible"}
		}
	}
	return nil
}

type preemptionExecution struct {
	app                *platform.App
	request            *http.Request
	client             internalWorkloadPreemptionClient
	repo               *recordStoreSchedulerPreemptionPriorityRepository
	recordID           string
	publicPreemptionID string
	req                preemptionRequest
	victims            []preemptionTarget
	freedGPU           float64
	freedCPU           float64
}

func executePreemption(exec preemptionExecution) (int, map[string]any) {
	completed := make([]map[string]any, 0, len(exec.victims))
	reason := fmt.Sprintf("preempted for requester %s priority %d", exec.req.RequesterJobID, exec.req.PriorityValue)
	for _, victim := range exec.victims {
		cleanup, err := exec.app.Cluster.CleanupJobResources(exec.request.Context(), victim.Namespace, victim.JobID)
		cleanupData := cleanupResultData(cleanup)
		victimData := victimResultData(victim, cleanupData, err)
		if err != nil {
			appendVictimResult(exec.request.Context(), exec.repo, exec.recordID, victimData)
			data := finishPreemptionRecord(exec.request.Context(), exec.repo, exec.recordID, "partial_failure", map[string]any{
				"reason":  err.Error(),
				"victims": preemptionRecordVictims(exec.request.Context(), exec.repo, exec.recordID),
			})
			return http.StatusServiceUnavailable, data
		}
		if cleanup.Total() == 0 {
			appendVictimResult(exec.request.Context(), exec.repo, exec.recordID, victimData)
			data := finishPreemptionRecord(exec.request.Context(), exec.repo, exec.recordID, "preflight_failed", map[string]any{
				"reason":  "selected victim has no Kubernetes resources to clean up",
				"victims": preemptionRecordVictims(exec.request.Context(), exec.repo, exec.recordID),
			})
			return http.StatusConflict, data
		}
		_, err = exec.client.Preempt(exec.request.Context(), victim.ID, workloadPreemptRequest{
			PreemptionID:   exec.publicPreemptionID,
			RequesterJobID: exec.req.RequesterJobID,
			Reason:         reason,
			Cleanup:        cleanupData,
		})
		if err != nil {
			appendVictimResult(exec.request.Context(), exec.repo, exec.recordID, victimData)
			data := finishPreemptionRecord(exec.request.Context(), exec.repo, exec.recordID, "failed", map[string]any{
				"reason":  err.Error(),
				"victims": preemptionRecordVictims(exec.request.Context(), exec.repo, exec.recordID),
			})
			return http.StatusBadGateway, data
		}
		completed = append(completed, victimData)
		eventData := map[string]any{
			"preemption_id":    exec.publicPreemptionID,
			"requester_job_id": exec.req.RequesterJobID,
			"victim_job_id":    victim.JobID,
			"project_id":       victim.ProjectID,
			"namespace":        victim.Namespace,
			"reason":           reason,
			"cleanup":          cleanupData,
		}
		if err := exec.app.WithTx(exec.request.Context(), func(tx platform.StoreTx) error {
			if err := exec.repo.AppendPreemptionVictimTx(exec.request.Context(), tx, exec.recordID, victimData); err != nil {
				return err
			}
			event := schedulerEvent(exec.request, "JobPreempted", "preempted", eventData)
			event.IdempotencyKey = ""
			tx.Emit(event)
			return nil
		}); err != nil {
			data := finishPreemptionRecord(exec.request.Context(), exec.repo, exec.recordID, "failed", map[string]any{
				"reason":  err.Error(),
				"victims": preemptionRecordVictims(exec.request.Context(), exec.repo, exec.recordID),
			})
			return http.StatusInternalServerError, data
		}
	}
	data := finishPreemptionRecord(exec.request.Context(), exec.repo, exec.recordID, "completed", map[string]any{
		"accepted":  true,
		"reason":    reason,
		"victims":   completed,
		"freed_gpu": exec.freedGPU,
		"freed_cpu": exec.freedCPU,
	})
	return http.StatusOK, data
}

func initialPreemptionRecord(id string, req preemptionRequest) map[string]any {
	return map[string]any{
		"id":                                 id,
		internalPreemptionRecordID:           id,
		"preemption_id":                      platform.NewUUID(),
		"status":                             "in_progress",
		"accepted":                           false,
		internalPreemptionIdempotencyKeyHash: preemptionHash(req.IdempotencyKey),
		internalPreemptionFingerprintHash:    req.Fingerprint,
		"requester_job_id":                   req.RequesterJobID,
		"project_id":                         req.ProjectID,
		"queue_name":                         req.QueueName,
		"required_gpu":                       req.RequiredGPU,
		"required_cpu":                       req.RequiredCPU,
		"max_preemptions":                    req.MaxPreemptions,
		"started_at":                         time.Now().UTC().Format(time.RFC3339),
	}
}

func finishPreemptionRecord(
	ctx context.Context,
	repo *recordStoreSchedulerPreemptionPriorityRepository,
	id, status string,
	updates map[string]any,
) map[string]any {
	if repo == nil {
		update := shared.CloneMap(updates)
		update["status"] = status
		update["completed_at"] = time.Now().UTC().Format(time.RFC3339)
		return publicPreemptionRecordData(update)
	}
	return publicPreemptionRecordData(repo.FinishPreemptionRecord(ctx, id, status, updates, time.Now().UTC()))
}

func appendVictimResult(ctx context.Context, repo *recordStoreSchedulerPreemptionPriorityRepository, id string, victim map[string]any) {
	if repo == nil {
		return
	}
	repo.AppendPreemptionVictim(ctx, id, victim)
}

func preemptionRecordVictims(ctx context.Context, repo *recordStoreSchedulerPreemptionPriorityRepository, id string) []map[string]any {
	if repo == nil {
		return nil
	}
	return repo.PreemptionRecordVictims(ctx, id)
}

func replayPreemptionRecord(data map[string]any, req preemptionRequest) (int, any, *platform.Degraded) {
	fingerprintHash := shared.FirstNonEmpty(shared.TextValue(data, internalPreemptionFingerprintHash), shared.TextValue(data, "fingerprint"))
	if fingerprintHash != req.Fingerprint {
		return http.StatusConflict, shared.ErrorData("idempotency key is already used by a different preemption request"), nil
	}
	if shared.TextValue(data, "status") == "in_progress" {
		return http.StatusConflict, shared.ErrorData("preemption request is already in progress"), nil
	}
	return http.StatusOK, publicPreemptionRecordData(data), nil
}

func preemptionRecordID(key string) string {
	sum := sha256.Sum256([]byte(key))
	return "PRE-" + hex.EncodeToString(sum[:8])
}

func preemptionHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func publicPreemptionRecordData(data map[string]any) map[string]any {
	out := shared.CloneMap(data)
	publicID := shared.TextValue(out, "preemption_id", "preemptionId")
	privateID := shared.TextValue(out, "id")
	if publicID != "" && publicID != privateID {
		out["id"] = publicID
	} else {
		delete(out, "id")
		delete(out, "preemption_id")
	}
	delete(out, "idempotency_key")
	delete(out, "idempotencyKey")
	delete(out, "fingerprint")
	delete(out, internalPreemptionIdempotencyKeyHash)
	delete(out, internalPreemptionFingerprintHash)
	delete(out, internalPreemptionRecordID)
	return out
}

func preemptionFingerprint(req preemptionRequest) string {
	body := map[string]any{
		"requester_job_id": req.RequesterJobID,
		"project_id":       req.ProjectID,
		"queue_name":       req.QueueName,
		"device_class":     req.DeviceClass,
		"gpu_model":        req.GPUModel,
		"priority_value":   req.PriorityValue,
		"required_gpu":     req.RequiredGPU,
		"required_cpu":     req.RequiredCPU,
		"max_preemptions":  req.MaxPreemptions,
	}
	raw, _ := json.Marshal(body)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func (req preemptionRequest) contextQuery() url.Values {
	query := url.Values{}
	if req.RequesterJobID != "" && !req.Override {
		query.Set("requester_job_id", req.RequesterJobID)
	}
	if req.ProjectID != "" {
		query.Set("project_id", req.ProjectID)
	}
	if req.QueueName != "" {
		query.Set("queue_name", req.QueueName)
	}
	if req.DeviceClass != "" {
		query.Set("device_class_name", req.DeviceClass)
	}
	if req.GPUModel != "" {
		query.Set("gpu_model", req.GPUModel)
	}
	return query
}

func preemptibleWorkloadStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "running", "partial-preempted":
		return true
	default:
		return false
	}
}

func schedulerQueuePreemptible(app *platform.App, queueName string) bool {
	if queueName == "" {
		return false
	}
	repo := schedulerQuotaRepositoryForApp(app)
	if repo == nil {
		return false
	}
	queue, found := repo.FindQueueByNameOrID(context.Background(), queueName)
	return found && shared.BoolValue(queue.Data, "is_preemptible", "isPreemptible", "preemptible")
}

func preemptionListOfMaps(value any) []map[string]any {
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
	default:
		return nil
	}
}

func preemptionDemandSatisfied(req preemptionRequest, freedGPU, freedCPU float64) bool {
	gpuOK := req.RequiredGPU <= 0 || freedGPU >= req.RequiredGPU
	cpuOK := req.RequiredCPU <= 0 || freedCPU >= req.RequiredCPU
	if req.RequiredGPU > 0 && gpuOK {
		cpuOK = true
	}
	return gpuOK && cpuOK
}

func clampMaxPreemptions(value int) int {
	if value <= 0 || value > defaultMaxPreemptions {
		return defaultMaxPreemptions
	}
	return value
}

func selectMinimalPreemptionVictims(req preemptionRequest, eligible []preemptionTarget) ([]preemptionTarget, float64, float64) {
	maxVictims := req.MaxPreemptions
	if maxVictims > len(eligible) {
		maxVictims = len(eligible)
	}
	if maxVictims <= 0 {
		return nil, 0, 0
	}
	for targetSize := 1; targetSize <= maxVictims; targetSize++ {
		selection := make([]preemptionTarget, 0, targetSize)
		if victims, freedGPU, freedCPU, ok := findPreemptionVictimCombination(req, eligible, selection, 0, targetSize, 0, 0); ok {
			return victims, freedGPU, freedCPU
		}
	}
	return nil, 0, 0
}

func findPreemptionVictimCombination(
	req preemptionRequest,
	eligible []preemptionTarget,
	selection []preemptionTarget,
	start, remaining int,
	freedGPU, freedCPU float64,
) ([]preemptionTarget, float64, float64, bool) {
	if remaining == 0 {
		if preemptionDemandSatisfied(req, freedGPU, freedCPU) {
			return append([]preemptionTarget{}, selection...), freedGPU, freedCPU, true
		}
		return nil, 0, 0, false
	}
	if len(eligible)-start < remaining {
		return nil, 0, 0, false
	}
	for i := start; i <= len(eligible)-remaining; i++ {
		candidate := eligible[i]
		selection = append(selection, candidate)
		victims, gpu, cpu, ok := findPreemptionVictimCombination(
			req,
			eligible,
			selection,
			i+1,
			remaining-1,
			freedGPU+candidate.RequiredGPU,
			freedCPU+candidate.RequiredCPU,
		)
		if ok {
			return victims, gpu, cpu, true
		}
		selection = selection[:len(selection)-1]
	}
	return nil, 0, 0, false
}

func preemptionTargetPreferred(left, right preemptionTarget) bool {
	if left.PriorityValue != right.PriorityValue {
		return left.PriorityValue < right.PriorityValue
	}
	leftTime := parsePreemptionTime(left.CreatedAt)
	rightTime := parsePreemptionTime(right.CreatedAt)
	if !leftTime.Equal(rightTime) {
		return leftTime.After(rightTime)
	}
	return left.JobID < right.JobID
}

func parsePreemptionTime(value string) time.Time {
	if parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(value)); err == nil {
		return parsed
	}
	return time.Time{}
}

func cleanupResultData(result cluster.CleanupResult) map[string]any {
	return map[string]any{
		"pods":         result.Pods,
		"deployments":  result.Deployments,
		"statefulsets": result.StatefulSets,
		"services":     result.Services,
		"jobs":         result.Jobs,
		"vcjobs":       result.VCJobs,
		"podgroups":    result.PodGroups,
		"configmaps":   result.ConfigMaps,
		"secrets":      result.Secrets,
		"ingresses":    result.Ingresses,
		"total":        result.Total(),
	}
}

func victimResultData(victim preemptionTarget, cleanup map[string]any, cleanupErr error) map[string]any {
	data := map[string]any{
		"job_id":         victim.JobID,
		"record_id":      victim.ID,
		"project_id":     victim.ProjectID,
		"queue_name":     victim.QueueName,
		"namespace":      victim.Namespace,
		"priority_value": victim.PriorityValue,
		"required_gpu":   victim.RequiredGPU,
		"required_cpu":   victim.RequiredCPU,
		"cleanup":        cleanup,
	}
	if cleanupErr != nil {
		data["cleanup_error"] = cleanupErr.Error()
	}
	return data
}

func workloadSnapshotFromData(id string, data map[string]any) workloadJobSnapshot {
	return workloadJobSnapshot{
		ID:                 shared.FirstNonEmpty(shared.TextValue(data, "id"), id),
		JobID:              shared.FirstNonEmpty(shared.TextValue(data, "job_id", "jobId"), id),
		ProjectID:          shared.TextValue(data, "project_id", "projectId"),
		UserID:             shared.TextValue(data, "user_id", "userId"),
		QueueName:          shared.TextValue(data, "queue_name", "queueName"),
		Namespace:          shared.TextValue(data, "namespace", "Namespace"),
		Status:             shared.TextValue(data, "status", "Status"),
		PriorityValue:      shared.IntValue(data, "priority_value", "priorityValue", "priority"),
		Preemptible:        shared.BoolValue(data, "preemptible", "is_preemptible", "isPreemptible"),
		RequiredGPU:        shared.NumberValue(data, "required_gpu", "requiredGpu", "RequiredGPU"),
		RequiredCPU:        shared.NumberValue(data, "required_cpu", "requiredCPU", "required_cpu_cores", "RequiredCPU"),
		RequiredMemory:     shared.IntValue(data, "required_memory", "requiredMemory", "required_memory_mb", "RequiredMemory"),
		DeviceClassName:    shared.TextValue(data, "device_class_name", "deviceClassName"),
		GPUModel:           shared.TextValue(data, "gpu_model", "gpuModel"),
		CreatedAt:          shared.TextValue(data, "created_at", "createdAt"),
		PreemptionRecordID: shared.TextValue(data, "preemption_record_id", "preemptionRecordId"),
	}
}

type preemptionHTTPError struct {
	status  int
	message string
}

func (e preemptionHTTPError) Error() string {
	return e.message
}

func preemptionErrorStatus(err error) int {
	if typed, ok := err.(preemptionHTTPError); ok {
		return typed.status
	}
	return http.StatusInternalServerError
}
