package gpuusage

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

func usageRows(app *platform.App, r *http.Request, since time.Time) []usageRow {
	if app == nil || app.Store == nil {
		return nil
	}
	jobs := indexRecords(gpuRecords(app, r, gpuJobsResource, workloadJobsResource))
	projects := indexProjectRecords(gpuRecords(app, r, gpuProjectsResource, orgProjectsResource))
	users := indexRecords(gpuRecords(app, r, gpuIdentityUsersResource, identityUsersResource))

	records := app.Store.List(r.Context(), summariesResource)
	rows := make([]usageRow, 0, len(records))
	for _, record := range records {
		row := usageRowFromRecord(record, jobs, projects, users)
		if !row.computedAt.IsZero() && row.computedAt.Before(since) {
			continue
		}
		rows = append(rows, row)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		return rows[i].computedAt.After(rows[j].computedAt)
	})
	return rows
}

func latestActiveSnapshots(app *platform.App, r *http.Request) map[string]snapshotRow {
	latest := map[string]snapshotRow{}
	for _, row := range snapshotRows(app, r, activeSnapshotCutoff(app.Config)) {
		key := row.jobID + "\x00" + intKey(row.gpuIndex)
		if existing, ok := latest[key]; !ok || row.timestamp.After(existing.timestamp) {
			latest[key] = row
		}
	}
	return latest
}

func runningUserJobs(app *platform.App, r *http.Request, userID string, summarizedJobs map[string]struct{}) []AdminGPUUserJob {
	grouped := map[string][]snapshotRow{}
	for _, row := range snapshotRows(app, r, activeSnapshotCutoff(app.Config)) {
		if row.userID != userID || !strings.EqualFold(textValue(row.job, "status", "Status"), "running") {
			continue
		}
		if _, ok := summarizedJobs[row.jobID]; ok {
			continue
		}
		grouped[row.jobID] = append(grouped[row.jobID], row)
	}
	out := []AdminGPUUserJob{}
	for jobID, rows := range grouped {
		if len(rows) == 0 {
			continue
		}
		sort.Slice(rows, func(i, j int) bool { return rows[i].timestamp.Before(rows[j].timestamp) })
		start := rows[0].timestamp
		end := rows[len(rows)-1].timestamp
		gpus := map[int]struct{}{}
		for _, row := range rows {
			gpus[row.gpuIndex] = struct{}{}
		}
		spanHours := end.Sub(start).Hours() * float64(len(gpus))
		out = append(out, AdminGPUUserJob{
			JobID:         jobID,
			QueueName:     textValue(rows[0].job, "queue_name", "queueName", "QueueName"),
			ProjectName:   textValue(rows[0].project, "project_name", "projectName", "name", "Name"),
			TotalGPUHours: spanHours,
			PeriodStart:   &start,
		})
	}
	return out
}

func mpsMapping(app *platform.App, r *http.Request) []MPSGPUSlot {
	latest := map[string]snapshotRow{}
	for _, row := range snapshotRows(app, r, activeSnapshotCutoff(app.Config)) {
		if row.mpsUnits <= 0 {
			continue
		}
		key := textValue(row.data, "node", "Node") + "\x00" + intKey(row.mpsGPUIdx) + "\x00" + row.gpuUUID + "\x00" + row.jobID + "\x00" + row.namespace + "\x00" + row.podName
		if existing, ok := latest[key]; !ok || row.timestamp.After(existing.timestamp) {
			latest[key] = row
		}
	}
	out := make([]MPSGPUSlot, 0, len(latest))
	for _, row := range latest {
		out = append(out, MPSGPUSlot{
			Node:                 textValue(row.data, "node", "Node"),
			PhysicalGPUIndex:     row.mpsGPUIdx,
			GPUUUID:              row.gpuUUID,
			JobID:                row.jobID,
			PodName:              row.podName,
			PodNamespace:         row.namespace,
			MPSVirtualUnits:      row.mpsUnits,
			GPUMemoryBytes:       row.memoryByte,
			SMUtilization:        floatValue(row.metrics, "gpu_sm_utilization", "gpuSMUtilization"),
			SMUtilizationSource:  textValue(row.metrics, "gpu_sm_util_source", "gpuSMUtilSource"),
			MemUtilization:       floatValue(row.metrics, "gpu_mem_utilization", "gpuMemUtilization"),
			MemUtilizationSource: textValue(row.metrics, "gpu_mem_util_source", "gpuMemUtilSource"),
			MemoryUsedBytes:      int64Value(row.metrics, "gpu_memory_used_bytes", "gpuMemoryUsedBytes"),
			MemoryUsedSource:     textValue(row.metrics, "gpu_memory_used_source", "gpuMemoryUsedSource"),
			EncUtilization:       floatValue(row.metrics, "gpu_enc_utilization", "gpuEncUtilization"),
			DecUtilization:       floatValue(row.metrics, "gpu_dec_utilization", "gpuDecUtilization"),
			PowerUsageWatts:      floatValue(row.metrics, "gpu_power_usage_watts", "gpuPowerUsageWatts"),
			Timestamp:            row.timestamp,
			UserID:               row.userID,
			Username:             textValue(row.user, "username", "Username"),
			ProjectName:          textValue(row.project, "project_name", "projectName", "name", "Name"),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Node == out[j].Node {
			if out[i].PhysicalGPUIndex == out[j].PhysicalGPUIndex {
				return out[i].JobID < out[j].JobID
			}
			return out[i].PhysicalGPUIndex < out[j].PhysicalGPUIndex
		}
		return out[i].Node < out[j].Node
	})
	return out
}

func snapshotRows(app *platform.App, r *http.Request, since time.Time) []snapshotRow {
	if app == nil || app.Store == nil {
		return nil
	}
	jobs := indexRecords(gpuRecords(app, r, gpuJobsResource, workloadJobsResource))
	projects := indexProjectRecords(gpuRecords(app, r, gpuProjectsResource, orgProjectsResource))
	users := indexRecords(gpuRecords(app, r, gpuIdentityUsersResource, identityUsersResource))
	rows := []snapshotRow{}
	for _, record := range app.Store.List(r.Context(), snapshotsResource) {
		data := record.Data
		metrics := mapValue(data, "metrics", "Metrics")
		timestamp := firstTime(data, metrics, "timestamp", "Timestamp")
		if timestamp.IsZero() {
			timestamp = record.CreatedAt
		}
		if !timestamp.IsZero() && timestamp.Before(since) {
			continue
		}
		jobID := textValue(data, "job_id", "jobId", "JobID")
		job := jobs[jobID]
		userID := shared.FirstNonBlank(textValue(data, "user_id", "userId", "UserID"), textValue(job, "user_id", "userId", "UserID"))
		projectID := shared.FirstNonBlank(textValue(data, "project_id", "projectId", "ProjectID"), textValue(job, "project_id", "projectId", "ProjectID"))
		gpuUUID := shared.FirstNonBlank(textValue(data, "gpu_uuid", "gpuUUID", "GPUUUID"), textValue(metrics, "gpu_uuid", "gpuUUID", "GPUUUID"))
		gpuIndex := intValue(data, "gpu_index", "gpuIndex", "GPUIndex")
		rows = append(rows, snapshotRow{
			record:     record,
			data:       data,
			metrics:    metrics,
			job:        job,
			user:       users[userID],
			project:    projects[projectID],
			timestamp:  timestamp,
			jobID:      jobID,
			userID:     userID,
			projectID:  projectID,
			gpuIndex:   gpuIndex,
			gpuUUID:    gpuUUID,
			podName:    textValue(data, "pod_name", "podName", "PodName"),
			namespace:  textValue(data, "pod_namespace", "podNamespace", "PodNamespace"),
			mpsUnits:   intValue(data, "mps_virtual_units", "mpsVirtualUnits", "MPSVirtualUnits"),
			mpsGPUIdx:  intValue(data, "mps_physical_gpu_index", "mpsPhysicalGPUIndex", "MPSPhysicalGPUIndex"),
			memoryByte: int64Value(metrics, "gpu_memory_bytes", "gpuMemoryBytes", "GPUMemoryBytes"),
		})
	}
	return rows
}

func usageRowFromRecord(record contracts.Record[map[string]any], jobs, projects, users map[string]map[string]any) usageRow {
	data := record.Data
	metrics := mapValue(data, "metrics", "Metrics")
	jobID := textValue(data, "job_id", "jobId", "JobID")
	job := jobs[jobID]
	userID := shared.FirstNonBlank(textValue(data, "user_id", "userId", "UserID"), textValue(job, "user_id", "userId", "UserID"))
	projectID := shared.FirstNonBlank(textValue(data, "project_id", "projectId", "ProjectID"), textValue(job, "project_id", "projectId", "ProjectID"))
	user := users[userID]
	project := projects[projectID]

	gpuSeconds := firstFloat(data, metrics, "total_gpu_seconds", "totalGPUSeconds", "TotalGPUSeconds")
	cpuSeconds := firstFloat(data, metrics, "total_cpu_seconds", "totalCPUSeconds", "TotalCPUSeconds")
	memorySecondsMB := firstFloat(data, metrics, "total_memory_seconds_mb", "totalMemorySecondsMB", "TotalMemorySecondsMB")
	gpuHours := gpuSeconds / 3600.0
	cpuHours := cpuSeconds / 3600.0
	memoryGBHours := memorySecondsMB / (1024.0 * 3600.0)
	if gpuSeconds == 0 {
		gpuHours = floatValue(data, "gpu_hours", "gpuHours", "GPUHours")
	}
	if cpuSeconds == 0 {
		cpuHours = floatValue(data, "cpu_hours", "cpuHours", "CPUHours")
	}
	if memorySecondsMB == 0 {
		memoryGBHours = floatValue(data, "memory_gb_hours", "memoryGBHours", "MemoryGBHours")
	}

	computedAt := firstTime(data, metrics, "computed_at", "computedAt", "ComputedAt")
	if computedAt.IsZero() {
		computedAt = record.CreatedAt
	}
	periodStart := timePointerValue(data, metrics, "first_sample_at", "period_start", "periodStart", "PeriodStart")
	periodEnd := timePointerValue(data, metrics, "last_sample_at", "period_end", "periodEnd", "PeriodEnd")
	return usageRow{
		UserResourceUsage: UserResourceUsage{
			UserID:        userID,
			Username:      shared.FirstNonBlank(textValue(data, "username", "Username"), textValue(user, "username", "Username")),
			ProjectID:     projectID,
			ProjectName:   shared.FirstNonBlank(textValue(data, "project_name", "projectName", "ProjectName"), textValue(project, "project_name", "projectName", "name", "Name")),
			JobID:         jobID,
			CPUHours:      cpuHours,
			GPUHours:      gpuHours,
			MemoryGBHours: memoryGBHours,
			PeriodStart:   periodStart,
			PeriodEnd:     periodEnd,
		},
		computedAt: computedAt,
	}
}

func hasAdminPanel(app *platform.App, r *http.Request, userID string) bool {
	users := gpuRecords(app, r, gpuIdentityUsersResource, identityUsersResource)
	roles := append(
		gpuRecords(app, r, gpuIdentityRolesResource, identityRolesResource),
		gpuRecords(app, r, gpuAuthorizationRolesResource, authorizationRolesResource)...,
	)
	for _, user := range users {
		if recordID(user) != userID && textValue(user.Data, "user_id", "userId", "UserID") != userID {
			continue
		}
		if recordGrantsAdminPanel(user.Data) {
			return true
		}
		roleID := textValue(user.Data, "role_id", "roleId", "RoleID", "role", "Role")
		return roleGrantsAdminPanel(roles, roleID)
	}
	for _, role := range roles {
		if directRoleGrantsUserAdmin(role, userID) {
			return true
		}
	}
	return false
}

func roleGrantsAdminPanel(roles []contracts.Record[map[string]any], roleID string) bool {
	for _, role := range roles {
		if recordID(role) == roleID || textValue(role.Data, "name", "Name") == roleID {
			return recordGrantsAdminPanel(role.Data)
		}
	}
	return false
}

func directRoleGrantsUserAdmin(role contracts.Record[map[string]any], userID string) bool {
	return textValue(role.Data, "user_id", "userId", "UserID") == userID && recordGrantsAdminPanel(role.Data)
}

func recordGrantsAdminPanel(data map[string]any) bool {
	if boolValue(data, "admin_panel", "adminPanel", "AdminPanel") {
		return true
	}
	capabilities := mapValue(data, "capabilities", "Capabilities")
	if boolValue(capabilities, "admin_panel", "adminPanel", "AdminPanel") {
		return true
	}
	return false
}

func indexRecords(records []contracts.Record[map[string]any]) map[string]map[string]any {
	index := map[string]map[string]any{}
	for _, record := range records {
		for _, key := range []string{recordID(record), textValue(record.Data, "id", "ID"), textValue(record.Data, "job_id", "jobId", "JobID"), textValue(record.Data, "user_id", "userId", "UserID")} {
			if key != "" {
				index[key] = record.Data
			}
		}
	}
	return index
}

func indexProjectRecords(records []contracts.Record[map[string]any]) map[string]map[string]any {
	index := indexRecords(records)
	for _, record := range records {
		for _, key := range []string{textValue(record.Data, "p_id", "pID", "PID", "project_id", "projectId", "ProjectID")} {
			if key != "" {
				index[key] = record.Data
			}
		}
	}
	return index
}

func recordID(record contracts.Record[map[string]any]) string {
	return shared.FirstNonBlank(record.ID, textValue(record.Data, "id", "ID"))
}

func currentUserID(r *http.Request) string {
	return strings.TrimSpace(r.Header.Get("X-User-ID"))
}

func startOfMonth(now time.Time) time.Time {
	return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
}

func textValue(data map[string]any, keys ...string) string {
	for _, key := range keys {
		switch value := data[key].(type) {
		case string:
			if strings.TrimSpace(value) != "" {
				return strings.TrimSpace(value)
			}
		case json.Number:
			if value.String() != "" {
				return value.String()
			}
		}
	}
	return ""
}

func floatValue(data map[string]any, keys ...string) float64 {
	for _, key := range keys {
		switch value := data[key].(type) {
		case float64:
			return value
		case float32:
			return float64(value)
		case int:
			return float64(value)
		case int64:
			return float64(value)
		case json.Number:
			if parsed, err := value.Float64(); err == nil {
				return parsed
			}
		}
	}
	return 0
}

func intValue(data map[string]any, keys ...string) int {
	return int(int64Value(data, keys...))
}

func int64Value(data map[string]any, keys ...string) int64 {
	for _, key := range keys {
		switch value := data[key].(type) {
		case int:
			return int64(value)
		case int64:
			return value
		case int32:
			return int64(value)
		case float64:
			return int64(value)
		case float32:
			return int64(value)
		case json.Number:
			if parsed, err := value.Int64(); err == nil {
				return parsed
			}
		case string:
			if parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64); err == nil {
				return parsed
			}
		}
	}
	return 0
}

func boolValue(data map[string]any, keys ...string) bool {
	for _, key := range keys {
		switch value := data[key].(type) {
		case bool:
			return value
		case string:
			return strings.EqualFold(value, "true")
		}
	}
	return false
}

func mapValue(data map[string]any, keys ...string) map[string]any {
	for _, key := range keys {
		if value, ok := data[key].(map[string]any); ok {
			return value
		}
		if raw, ok := data[key].(string); ok && strings.TrimSpace(raw) != "" {
			var decoded map[string]any
			if json.Unmarshal([]byte(raw), &decoded) == nil {
				return decoded
			}
		}
	}
	return map[string]any{}
}

func firstFloat(primary, secondary map[string]any, keys ...string) float64 {
	if value := floatValue(primary, keys...); value != 0 {
		return value
	}
	return floatValue(secondary, keys...)
}

func firstTime(primary, secondary map[string]any, keys ...string) time.Time {
	if value := derefTime(timePointerValue(primary, nil, keys...)); !value.IsZero() {
		return value
	}
	return derefTime(timePointerValue(secondary, nil, keys...))
}

func timePointerValue(primary, secondary map[string]any, keys ...string) *time.Time {
	for _, source := range []map[string]any{primary, secondary} {
		if source == nil {
			continue
		}
		for _, key := range keys {
			if parsed := timePointerFromValue(source[key]); parsed != nil {
				return parsed
			}
		}
	}
	return nil
}

func timePointerFromValue(value any) *time.Time {
	switch typed := value.(type) {
	case time.Time:
		return &typed
	case *time.Time:
		return typed
	case string:
		return parseTimePointer(typed)
	default:
		return nil
	}
}

func parseTimePointer(value string) *time.Time {
	if value == "" {
		return nil
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05", "2006-01-02"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return &parsed
		}
	}
	return nil
}

func derefTime(value *time.Time) time.Time {
	if value == nil {
		return time.Time{}
	}
	return *value
}

func intKey(value int) string {
	return strconv.Itoa(value)
}

func findRecord(records []contracts.Record[map[string]any], id string) map[string]any {
	for _, record := range records {
		if recordID(record) == id || textValue(record.Data, "job_id", "jobId", "JobID", "user_id", "userId", "UserID") == id {
			return record.Data
		}
	}
	return map[string]any{}
}

func comparableJobTime(job AdminGPUUserJob) time.Time {
	if job.PeriodEnd != nil {
		return *job.PeriodEnd
	}
	if job.PeriodStart != nil {
		return *job.PeriodStart
	}
	return time.Time{}
}

func activeSnapshotCutoff(cfg platform.Config) time.Time {
	return time.Now().Add(-time.Duration(snapshotWindowMinutes(cfg)) * time.Minute)
}

func snapshotWindowMinutes(cfg platform.Config) int {
	minutes := cfg.GPUUsageSnapshotWindowMin
	if minutes == 0 {
		minutes = 10
	}
	if minutes < 1 {
		return 1
	}
	if minutes > 1440 {
		return 1440
	}
	return minutes
}
