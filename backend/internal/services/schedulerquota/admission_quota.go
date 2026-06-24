package schedulerquota

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

func admissionProjectPlan(ctx context.Context, reader admissionReader, project contracts.Record[map[string]any], now time.Time) (contracts.Record[map[string]any], error) {
	planID := shared.TextValue(project.Data, "plan_id", "planId", "resource_plan_id", "resourcePlanId")
	if planID == "" {
		return contracts.Record[map[string]any]{}, deny(http.StatusForbidden, "project has no active resource plan")
	}
	plan, found := reader.Plan(ctx, planID)
	if !found || !admissionPlanActive(plan.Data, now) {
		return contracts.Record[map[string]any]{}, deny(http.StatusForbidden, "project has no active resource plan")
	}
	return plan, nil
}

func requireAdmissionProjectAccess(ctx context.Context, reader admissionReader, project contracts.Record[map[string]any], userID string) error {
	if userID == shared.TextValue(project.Data, "personal_user_id", "personalUserId") {
		return nil
	}
	if recordHasFields(reader.ProjectMember(ctx, projectMemberKey(project.ID, userID))) {
		return nil
	}
	for _, member := range reader.ListProjectMembers(ctx) {
		if shared.TextValue(member.Data, "project_id", "projectId") == project.ID &&
			shared.TextValue(member.Data, "user_id", "userId") == userID &&
			shared.TextValue(member.Data, "role", "Role") != "" {
			return nil
		}
	}
	ownerID := shared.TextValue(project.Data, "owner_id", "ownerId", "GID", "g_id")
	if ownerID != "" && admissionUserInGroup(ctx, reader, ownerID, userID) {
		return nil
	}
	return deny(http.StatusForbidden, "project access required for submit")
}

func admissionUserInGroup(ctx context.Context, reader admissionReader, groupID, userID string) bool {
	for _, record := range reader.ListUserGroups(ctx) {
		if shared.TextValue(record.Data, "group_id", "groupId", "GID", "g_id") == groupID &&
			shared.TextValue(record.Data, "user_id", "userId", "UID", "u_id") == userID &&
			shared.TextValue(record.Data, "role", "Role") != "" {
			return true
		}
	}
	return false
}

func admissionQueueAllowed(ctx context.Context, reader admissionReader, plan contracts.Record[map[string]any], queueName string) bool {
	if queueName == "" {
		return false
	}
	for _, queueID := range shared.StringSlice(plan.Data["queue_ids"]) {
		if queueID == queueName {
			return true
		}
		queue, found := reader.Queue(ctx, queueID)
		if found && shared.TextValue(queue.Data, "name") == queueName {
			return true
		}
	}
	return false
}

func admissionQueueByName(ctx context.Context, reader admissionReader, queueName string) (contracts.Record[map[string]any], bool) {
	if queueName == "" {
		return contracts.Record[map[string]any]{}, false
	}
	if queue, found := reader.Queue(ctx, queueName); found {
		return queue, true
	}
	for _, queue := range reader.ListQueues(ctx) {
		if queue.ID == queueName || shared.TextValue(queue.Data, "name") == queueName {
			return queue, true
		}
	}
	return contracts.Record[map[string]any]{}, false
}

func enforceAdmissionDeviceClass(plan contracts.Record[map[string]any], req *submitAdmissionRequest) error {
	if req.RequiredGPU <= 0 && req.GPUCount <= 0 {
		req.DeviceClassName = ""
		return nil
	}
	deviceClass := strings.TrimSpace(req.DeviceClassName)
	if deviceClass == "" {
		deviceClass = defaultDeviceClassName
	}
	allowed := admissionStringList(plan.Data, "allowed_gpu_models", "allowedGPUModels")
	if len(allowed) == 0 {
		return deny(http.StatusForbidden, "no GPU models allowed by plan")
	}
	for _, model := range allowed {
		if strings.EqualFold(model, deviceClass) {
			req.DeviceClassName = deviceClass
			return nil
		}
	}
	return deny(http.StatusForbidden, fmt.Sprintf("GPU model %q is not allowed by the plan", deviceClass))
}

// enforceAdmissionMPSPolicy implements GPU-012: MPS GPU sharing across projects
// is blocked unless a platform admin explicitly allows it on the plan. MPS is
// requested when the job declares an SM percentage below 100 (fractional GPU
// sharing). Same-project (single-tenant) MPS is always allowed; cross-project
// sharing is signalled by mps_share_project_id pointing at a different project.
func enforceAdmissionMPSPolicy(plan contracts.Record[map[string]any], req submitAdmissionRequest) error {
	if req.SMPercentage == nil || *req.SMPercentage >= 100 {
		return nil
	}
	if shared.BoolValue(plan.Data, "mps_forbidden", "mpsForbidden") {
		return deny(http.StatusForbidden, "MPS GPU sharing is forbidden by the plan; use MIG or a whole GPU")
	}
	share := strings.TrimSpace(req.MPSShareProjectID)
	if share == "" || share == req.ProjectID {
		return nil
	}
	if shared.BoolValue(plan.Data, "allow_cross_project_mps", "allowCrossProjectMPS") {
		return nil
	}
	return deny(http.StatusForbidden, "MPS GPU sharing across projects requires platform admin approval (plan allow_cross_project_mps)")
}

func admissionUsageForJobs(ctx context.Context, reader admissionReader, projectID, userID string, usage admissionUsage) admissionUsage {
	for _, job := range reader.ListWorkloadJobs(ctx) {
		status := strings.ToLower(shared.TextValue(job.Data, "status", "Status"))
		if !activeAdmissionStatus(status) {
			continue
		}
		if admissionJobStreamingSession(job.Data) {
			usage.ActiveStreamSessions++
			usage.ActiveStreamBitrateKbps += admissionJobStreamBitrate(job.Data)
		}
		if shared.TextValue(job.Data, "project_id", "projectId", "ProjectID") != projectID {
			continue
		}
		usage.ProjectGPU += shared.NumberValue(job.Data, "required_gpu", "requiredGpu", "RequiredGPU")
		usage.ProjectCPU += shared.NumberValue(job.Data, "required_cpu", "requiredCPU", "required_cpu_cores", "RequiredCPU")
		usage.ProjectMemoryMB += shared.IntValue(job.Data, "required_memory", "requiredMemory", "RequiredMemory")
		if shared.TextValue(job.Data, "user_id", "userId", "UserID") != userID {
			continue
		}
		usage.UserGPU += shared.NumberValue(job.Data, "required_gpu", "requiredGpu", "RequiredGPU")
		usage.UserCPU += shared.NumberValue(job.Data, "required_cpu", "requiredCPU", "required_cpu_cores", "RequiredCPU")
		usage.UserMemoryMB += shared.IntValue(job.Data, "required_memory", "requiredMemory", "RequiredMemory")
		if status == "running" {
			usage.UserRunningJobs++
		}
		if queuedAdmissionStatuses[status] {
			usage.UserQueuedJobs++
		}
	}
	return usage
}

func enforceAdmissionJobLimits(project contracts.Record[map[string]any], usage admissionUsage) error {
	maxRunning := shared.IntValue(project.Data, "max_concurrent_jobs_per_user", "maxConcurrentJobsPerUser", "MaxConcurrentJobsPerUser")
	if maxRunning > 0 && usage.UserRunningJobs >= maxRunning {
		return deny(http.StatusConflict, "max concurrent jobs exceeded")
	}
	maxQueued := shared.IntValue(project.Data, "max_queued_jobs_per_user", "maxQueuedJobsPerUser", "MaxQueuedJobsPerUser")
	if maxQueued > 0 && usage.UserQueuedJobs >= maxQueued {
		return deny(http.StatusConflict, "max queued jobs exceeded")
	}
	return nil
}

func enforceAdmissionQuota(plan contracts.Record[map[string]any], req submitAdmissionRequest, usage admissionUsage) error {
	gpuLimit := shared.NumberValue(plan.Data, "gpu_limit", "gpuLimit")
	if quotaExceeded(usage.ProjectGPU, req.RequiredGPU, gpuLimit) {
		return deny(http.StatusConflict, fmt.Sprintf("GPU quota exceeded: using %.2f, requested %.2f, limit %.2f", usage.ProjectGPU, req.RequiredGPU, gpuLimit))
	}
	cpuLimit := shared.NumberValue(plan.Data, "cpu_limit_cores", "cpuLimitCores")
	if quotaExceeded(usage.ProjectCPU, req.RequiredCPU, cpuLimit) {
		return deny(http.StatusConflict, fmt.Sprintf("CPU quota exceeded: using %.2f, requested %.2f, limit %.2f", usage.ProjectCPU, req.RequiredCPU, cpuLimit))
	}
	memLimit := shared.NumberValue(plan.Data, "memory_limit_gb", "memoryLimitGb")
	if memoryQuotaExceeded(float64(usage.ProjectMemoryMB), req.RequiredMemory, memLimit) {
		return deny(http.StatusConflict, fmt.Sprintf("Memory quota exceeded: using %.2fGB, requested %.2fGB, limit %.2fGB",
			float64(usage.ProjectMemoryMB)/1024.0, float64(req.RequiredMemory)/1024.0, memLimit))
	}
	return nil
}

func enforceAdmissionUserQuota(ctx context.Context, reader admissionReader, req submitAdmissionRequest, usage admissionUsage) error {
	quota, found := admissionUserQuota(ctx, reader, req.ProjectID, req.UserID)
	if !found {
		return nil
	}
	if quotaExceeded(usage.UserGPU, req.RequiredGPU, shared.NumberValue(quota.Data, "gpu_limit", "GPULimit")) {
		return deny(http.StatusConflict, "user GPU quota exceeded")
	}
	if quotaExceeded(usage.UserCPU, req.RequiredCPU, shared.NumberValue(quota.Data, "cpu_limit", "CPULimit")) {
		return deny(http.StatusConflict, "user CPU quota exceeded")
	}
	if memoryQuotaExceeded(float64(usage.UserMemoryMB), req.RequiredMemory, shared.NumberValue(quota.Data, "memory_limit_gb", "MemoryLimitGB")) {
		return deny(http.StatusConflict, "user Memory quota exceeded")
	}
	return nil
}

func admissionUserQuota(ctx context.Context, reader admissionReader, projectID, userID string) (contracts.Record[map[string]any], bool) {
	if quota, found := reader.UserQuota(ctx, projectMemberKey(projectID, userID)); found {
		return quota, true
	}
	for _, quota := range reader.ListUserQuotas(ctx) {
		if shared.TextValue(quota.Data, "project_id", "projectId") == projectID &&
			shared.TextValue(quota.Data, "user_id", "userId") == userID {
			return quota, true
		}
	}
	return contracts.Record[map[string]any]{}, false
}

func quotaExceeded(used, requested, limit float64) bool {
	return limit > 0 && used+requested > limit
}

func memoryQuotaExceeded(usedMB float64, requestedMB int, limitGB float64) bool {
	return limitGB > 0 && (usedMB+float64(requestedMB))/1024.0 > limitGB
}

func recordHasFields(record contracts.Record[map[string]any], found bool) bool {
	return found && len(record.Data) > 0
}

func projectMemberKey(projectID, userID string) string {
	return projectID + "/" + userID
}
