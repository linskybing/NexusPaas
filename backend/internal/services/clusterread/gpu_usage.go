package clusterread

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

func projectPodGPUUsages(summary map[string]any, projectID string) []map[string]any {
	out := []map[string]any{}
	for _, usage := range podGPUUsages(summary) {
		if podGPUUsageBelongsToProject(usage, projectID) {
			out = append(out, usage)
		}
	}
	return out
}

func projectReservedGPUFraction(app *platform.App, r *http.Request, projectID string, projectRows []map[string]any) (float64, string) {
	if total, ok := sumReservedGPUFraction(projectRows, gpuAllocationFraction); ok {
		return total, gpuSourceClusterAllocation
	}
	if total, ok := projectSnapshotReservedGPUFraction(app, r, projectID); ok {
		return total, gpuSourceSnapshotAllocation
	}
	if total, ok := projectWorkloadReservedGPUFraction(app, r, projectID); ok {
		return total, gpuSourceWorkloadAllocation
	}
	return 0, gpuSourceUnavailable
}

func sumReservedGPUFraction(rows []map[string]any, allocation func(map[string]any) (float64, bool)) (float64, bool) {
	var total float64
	var found bool
	for _, row := range rows {
		value, ok := allocation(row)
		if !ok {
			continue
		}
		total += value
		found = true
	}
	return total, found
}

func gpuAllocationFraction(data map[string]any) (float64, bool) {
	if value, ok := firstNumericValue(data, "reserved_gpu_fraction", "reservedGpuFraction", "ReservedGPUFraction"); ok {
		return value, true
	}
	if value, ok := firstNumericValue(data, "dra_effective_gpu", "draEffectiveGPU", "draEffectiveGpu", "DRAEffectiveGPU", "DRAEffectiveGpu"); ok {
		return value, true
	}
	if value, ok := firstNumericValue(data, "requested_gpu", "requestedGPU", "RequestedGPU"); ok {
		return value, true
	}
	gpuCount, gpuOK := firstNumericValue(data, "gpu_count", "gpuCount", "GPUCount")
	smPercent, smOK := firstNumericValue(data, "sm_percentage", "smPercentage", "SMPercentage")
	if gpuOK && smOK {
		return gpuCount * smPercent / 100, true
	}
	if value, ok := firstNumericValue(data, "mps_virtual_units", "mpsVirtualUnits", "MPSVirtualUnits", "mpsUnits", "MPSUnits"); ok {
		return value / 100, true
	}
	return 0, false
}

func projectSnapshotReservedGPUFraction(app *platform.App, r *http.Request, projectID string) (float64, bool) {
	latest := latestProjectGPUSnapshots(app, r, projectID)
	return sumReservedGPUFraction(latest, gpuAllocationFraction)
}

func latestProjectGPUSnapshots(app *platform.App, r *http.Request, projectID string) []map[string]any {
	if app == nil || app.Store == nil || r == nil {
		return nil
	}
	cutoff := time.Now().Add(-time.Duration(snapshotWindowMinutes(app.Config.GPUUsageSnapshotWindowMin)) * time.Minute)
	type snapshot struct {
		data      map[string]any
		timestamp time.Time
	}
	latest := map[string]snapshot{}
	for _, record := range app.Store.List(r.Context(), gpuUsageSnapshotsResource) {
		data := shared.CloneMap(record.Data)
		if !podGPUUsageBelongsToProject(data, projectID) {
			continue
		}
		ts := timeFromMap(data, "timestamp", "Timestamp")
		if ts.IsZero() {
			ts = record.CreatedAt
		}
		if !ts.IsZero() && ts.Before(cutoff) {
			continue
		}
		key := gpuAllocationRowKey(len(latest), data)
		if existing, ok := latest[key]; !ok || ts.After(existing.timestamp) {
			latest[key] = snapshot{data: data, timestamp: ts}
		}
	}
	out := make([]map[string]any, 0, len(latest))
	for _, row := range latest {
		out = append(out, row.data)
	}
	return out
}

func gpuAllocationRowKey(index int, row map[string]any) string {
	key := strings.Join([]string{
		textValue(row, "job_id", "jobId", "JobID"),
		textValue(row, "pod_namespace", "podNamespace", "PodNamespace", "namespace", "Namespace"),
		textValue(row, "pod_name", "podName", "PodName", "pod"),
		textValue(row, "gpu_uuid", "gpuUUID", "GPUUUID", "gpuUuid", "UUID"),
		numericText(row, "gpu_index", "gpuIndex", "GPUIndex"),
	}, "\x00")
	if strings.Trim(key, "\x00") != "" {
		return key
	}
	return strconv.Itoa(index)
}

func projectWorkloadReservedGPUFraction(app *platform.App, r *http.Request, projectID string) (float64, bool) {
	if app == nil || app.Store == nil || r == nil || !sourceCoHosted(app, workloadJobsResource) {
		return 0, false
	}
	rows := []map[string]any{}
	for _, record := range app.Store.List(r.Context(), workloadJobsResource) {
		data := shared.CloneMap(record.Data)
		if textValue(data, keyProjectID, keyProjectIDCamel, keyProjectIDTitle) != projectID || !activeWorkloadJob(data) {
			continue
		}
		rows = append(rows, data)
	}
	return sumReservedGPUFraction(rows, workloadJobAllocationFraction)
}

func activeWorkloadJob(job map[string]any) bool {
	switch strings.ToLower(textValue(job, "status", "Status")) {
	case "submitted", "waiting_infra", "queued", "running":
		return true
	default:
		return false
	}
}

func workloadJobAllocationFraction(job map[string]any) (float64, bool) {
	payload := mapValue(job, "reservation_payload", "reservationPayload", "ReservationPayload")
	reserved := mapValue(payload, "reserved", "Reserved")
	if value, ok := firstNumericValue(reserved, "gpu", "GPU", "gpus", "GPUs"); ok {
		return value, true
	}
	if value, ok := firstNumericValue(job, "required_gpu", "requiredGPU", "requiredGpu", "RequiredGPU", "RequiredGpu"); ok {
		return value, true
	}
	return gpuAllocationFraction(job)
}

func projectSMAttributionSource(app *platform.App, r *http.Request, projectID string, projectRows []map[string]any) string {
	source := aggregateSMAttributionSource(projectRows)
	if source != gpuSourceUnavailable {
		return source
	}
	return aggregateSMAttributionSource(latestProjectGPUSnapshots(app, r, projectID))
}

func aggregateSMAttributionSource(rows []map[string]any) string {
	out := gpuSourceUnavailable
	for _, row := range rows {
		source := smAttributionSource(row)
		if source == smAttributionEstimatedMPS {
			return source
		}
		if out == gpuSourceUnavailable && source != gpuSourceUnavailable {
			out = source
		}
	}
	return out
}

func smAttributionSource(data map[string]any) string {
	metrics := mapValue(data, "metrics", "Metrics")
	source := shared.FirstNonBlank(
		textValue(data, "gpu_sm_util_source", "gpuSMUtilSource", "GPUSMUtilSource"),
		textValue(metrics, "gpu_sm_util_source", "gpuSMUtilSource", "GPUSMUtilSource"),
	)
	if source != "" {
		if smSourceIsEstimated(source) {
			return smAttributionEstimatedMPS
		}
		if smSourceIsUnavailable(source) {
			if hasMPSAllocation(data, metrics) {
				return smAttributionEstimatedMPS
			}
			return gpuSourceUnavailable
		}
		return smAttributionMeasured
	}
	if hasMPSAllocation(data, metrics) {
		return smAttributionEstimatedMPS
	}
	return gpuSourceUnavailable
}

func hasMPSAllocation(data, metrics map[string]any) bool {
	if value, ok := firstNumericValue(data, "mps_virtual_units", "mpsVirtualUnits", "MPSVirtualUnits", "mpsUnits", "MPSUnits"); ok && value > 0 {
		return true
	}
	if value, ok := firstNumericValue(metrics, "mps_virtual_units", "mpsVirtualUnits", "MPSVirtualUnits", "mpsUnits", "MPSUnits"); ok && value > 0 {
		return true
	}
	return false
}

func smSourceIsEstimated(source string) bool {
	source = strings.ToLower(source)
	return strings.Contains(source, "estimated") || strings.Contains(source, "allocation") || strings.Contains(source, "mps")
}

func smSourceIsUnavailable(source string) bool {
	switch strings.ToLower(strings.TrimSpace(source)) {
	case "", "unavailable", "unknown", "none", "n/a", "na", "not_available", "not available":
		return true
	default:
		return false
	}
}

func firstNumericValue(data map[string]any, keys ...string) (float64, bool) {
	return shared.NumberValueOK(data, keys...)
}

func numericText(data map[string]any, keys ...string) string {
	value, ok := firstNumericValue(data, keys...)
	if !ok {
		return ""
	}
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func timeFromMap(data map[string]any, keys ...string) time.Time {
	if data == nil {
		return time.Time{}
	}
	for _, key := range keys {
		switch value := data[key].(type) {
		case time.Time:
			return value.UTC()
		case string:
			for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05", "2006-01-02"} {
				if parsed, err := time.Parse(layout, strings.TrimSpace(value)); err == nil {
					return parsed.UTC()
				}
			}
		}
	}
	return time.Time{}
}

func snapshotWindowMinutes(minutes int) int {
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
