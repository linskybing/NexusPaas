package schedulerquota

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"strings"

	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

type admissionMPSPolicy struct {
	allowed           bool
	maxSMPercentage   int
	allowCrossProject bool
}

const (
	errInvalidPlanMPSPolicy  = "invalid plan MPS policy: %w"
	errInvalidQueueMPSPolicy = "invalid queue MPS policy: %w"
)

func enforceAdmissionMPSPlanQueuePolicy(ctx context.Context, reader admissionReader, project, plan, queue admissionRecord, queueFound bool, req submitAdmissionRequest) error {
	if !admissionMPSRequested(req) {
		return nil
	}
	policy, err := resolveAdmissionMPSPolicy(project.Data, plan.Data, queue.Data, queueFound)
	if err != nil {
		return deny(http.StatusUnprocessableEntity, err.Error())
	}
	if !policy.allowed {
		return deny(http.StatusForbidden, "MPS is not allowed by plan or queue policy")
	}
	if projectMPSForbidden(project.Data) {
		return deny(http.StatusForbidden, "MPS is forbidden by project policy")
	}
	if policy.maxSMPercentage > 0 && admissionMPSPercentage(req) > policy.maxSMPercentage {
		return deny(http.StatusForbidden, fmt.Sprintf("MPS SM percentage exceeds policy cap: requested %d, limit %d", admissionMPSPercentage(req), policy.maxSMPercentage))
	}
	if !policy.allowCrossProject && admissionHasCrossProjectMPS(ctx, reader, req) {
		return deny(http.StatusForbidden, "cross-project MPS sharing requires explicit platform policy approval")
	}
	return nil
}

func admissionMPSRequested(req submitAdmissionRequest) bool {
	if req.SMPercentage != nil && *req.SMPercentage < 100 {
		return true
	}
	return req.PinnedMemoryLimit != nil && strings.TrimSpace(*req.PinnedMemoryLimit) != ""
}

func admissionMPSPercentage(req submitAdmissionRequest) int {
	if req.SMPercentage == nil {
		return 100
	}
	return *req.SMPercentage
}

func resolveAdmissionMPSPolicy(project, plan, queue map[string]any, queueFound bool) (admissionMPSPolicy, error) {
	planAllowed, err := admissionPolicyBool(plan, true, "mps_allowed", "mpsAllowed")
	if err != nil {
		return admissionMPSPolicy{}, fmt.Errorf(errInvalidPlanMPSPolicy, err)
	}
	queueAllowed := true
	if queueFound {
		queueAllowed, err = admissionPolicyBool(queue, true, "mps_allowed", "mpsAllowed")
		if err != nil {
			return admissionMPSPolicy{}, fmt.Errorf(errInvalidQueueMPSPolicy, err)
		}
	}

	planCross, err := admissionPolicyBool(plan, false, "allow_cross_project_mps", "allowCrossProjectMps")
	if err != nil {
		return admissionMPSPolicy{}, fmt.Errorf(errInvalidPlanMPSPolicy, err)
	}
	queueCross := true
	if queueFound {
		queueCross, err = admissionPolicyBool(queue, true, "allow_cross_project_mps", "allowCrossProjectMps")
		if err != nil {
			return admissionMPSPolicy{}, fmt.Errorf(errInvalidQueueMPSPolicy, err)
		}
	}

	planCap, err := admissionPolicySMCap(plan, "max_sm_percentage_per_gpu", "maxMpsSmPercentage", "max_gpu_sm_percentage_per_job")
	if err != nil {
		return admissionMPSPolicy{}, fmt.Errorf(errInvalidPlanMPSPolicy, err)
	}
	queueCap := 0
	if queueFound {
		queueCap, err = admissionPolicySMCap(queue, "max_sm_percentage_per_gpu", "maxMpsSmPercentage", "max_gpu_sm_percentage_per_job")
		if err != nil {
			return admissionMPSPolicy{}, fmt.Errorf(errInvalidQueueMPSPolicy, err)
		}
	}
	if _, err := admissionPolicyBool(project, false, "high_security", "highSecurity", "mps_forbidden", "mpsForbidden"); err != nil {
		return admissionMPSPolicy{}, fmt.Errorf("invalid project MPS policy: %w", err)
	}

	return admissionMPSPolicy{
		allowed:           planAllowed && queueAllowed,
		maxSMPercentage:   minPositive(planCap, queueCap),
		allowCrossProject: planCross && queueCross,
	}, nil
}

func projectMPSForbidden(project map[string]any) bool {
	return shared.BoolValue(project, "high_security", "highSecurity", "mps_forbidden", "mpsForbidden")
}

func admissionPolicyBool(data map[string]any, fallback bool, keys ...string) (bool, error) {
	out := fallback
	found := false
	for _, key := range keys {
		value, ok := data[key]
		if !ok {
			continue
		}
		typed, ok := value.(bool)
		if !ok {
			return false, fmt.Errorf("%s must be a boolean", key)
		}
		if !found {
			out = typed
			found = true
		}
	}
	return out, nil
}

func admissionPolicySMCap(data map[string]any, keys ...string) (int, error) {
	out := 0
	found := false
	for _, key := range keys {
		value, ok := data[key]
		if !ok {
			continue
		}
		cap, ok := admissionPolicyInt(value)
		if !ok || cap < 1 || cap > 100 {
			return 0, fmt.Errorf("%s must be an integer from 1 to 100", key)
		}
		if !found {
			out = cap
			found = true
		}
	}
	return out, nil
}

func admissionPolicyInt(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int32:
		return int(typed), true
	case int64:
		return int(typed), true
	case float32:
		if math.IsNaN(float64(typed)) || math.IsInf(float64(typed), 0) {
			return 0, false
		}
		if typed != float32(int(typed)) {
			return 0, false
		}
		return int(typed), true
	case float64:
		if math.IsNaN(typed) || math.IsInf(typed, 0) {
			return 0, false
		}
		if math.Trunc(typed) != typed {
			return 0, false
		}
		return int(typed), true
	default:
		return 0, false
	}
}

func admissionHasCrossProjectMPS(ctx context.Context, reader admissionReader, req submitAdmissionRequest) bool {
	deviceClass := strings.TrimSpace(req.DeviceClassName)
	if deviceClass == "" {
		deviceClass = defaultDeviceClassName
	}
	for _, job := range reader.ListWorkloadJobs(ctx) {
		if !activeAdmissionStatus(shared.TextValue(job.Data, "status", "Status")) {
			continue
		}
		if shared.TextValue(job.Data, "project_id", "projectId", "ProjectID") == req.ProjectID {
			continue
		}
		if !admissionJobMPSRequested(job.Data) {
			continue
		}
		jobDeviceClass := shared.TextValue(job.Data, "device_class_name", "deviceClassName", "DeviceClassName")
		if jobDeviceClass == "" {
			jobDeviceClass = defaultDeviceClassName
		}
		if strings.EqualFold(jobDeviceClass, deviceClass) {
			return true
		}
	}
	return false
}

func admissionJobMPSRequested(data map[string]any) bool {
	sm := shared.IntValue(data, "sm_percentage", "smPercentage", "SMPercentage")
	if sm > 0 && sm < 100 {
		return true
	}
	return shared.TextValue(data, "pinned_memory_limit", "pinnedMemoryLimit", "pinned_memory", "pinnedMemory") != ""
}

func minPositive(a, b int) int {
	if a <= 0 {
		return b
	}
	if b <= 0 || a < b {
		return a
	}
	return b
}
