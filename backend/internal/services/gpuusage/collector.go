package gpuusage

import (
	"context"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

const (
	clusterReadModelsResource = serviceName + ":cluster_read_models"
	gpuCollectorTaskName      = "gpu-usage-telemetry-collector"
)

type gpuCollectorStats struct {
	clusterReadModelFound bool
	podRowsScanned        int
	snapshotsWritten      int
	snapshotsSkipped      int
	summariesComputed     int
	snapshotsDeleted      int
}

type normalizedPodGPUUsage struct {
	jobID          string
	userID         string
	projectID      string
	namespace      string
	podName        string
	node           string
	gpuUUID        string
	gpuIndex       int
	mpsPhysicalIdx int
	mpsUnits       int
	timestamp      time.Time
	metrics        map[string]any
}

func collectGPUUsageTelemetry(ctx context.Context, app *platform.App, now time.Time) gpuCollectorStats {
	stats := gpuCollectorStats{}
	if app == nil || app.Store == nil {
		return stats
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	syncGPUReadModelsContext(app, ctx)
	jobs := indexRecords(gpuRecordsContext(app, ctx, gpuJobsResource, workloadJobsResource))

	summary, found := latestClusterGPUReadModel(ctx, app.Store)
	stats.clusterReadModelFound = found
	for _, usage := range podGPUUsageRows(summary) {
		stats.podRowsScanned++
		row, ok := normalizePodGPUUsage(usage, jobs)
		if !ok {
			stats.snapshotsSkipped++
			continue
		}
		if upsertGPUSnapshot(ctx, app.Store, row) {
			stats.snapshotsWritten++
		}
	}
	stats.summariesComputed = computeGPUSummaries(ctx, app.Store, jobs, now)
	stats.snapshotsDeleted = cleanupOldGPUSnapshots(ctx, app.Store, retentionCutoff(now, app.Config.GPUUsageRetentionDays))
	slog.Info("gpu usage collector completed",
		"cluster_read_model_found", stats.clusterReadModelFound,
		"pod_gpu_rows_scanned", stats.podRowsScanned,
		"snapshots_written", stats.snapshotsWritten,
		"snapshots_skipped", stats.snapshotsSkipped,
		"summaries_computed", stats.summariesComputed,
		"snapshots_deleted", stats.snapshotsDeleted,
	)
	return stats
}

func latestClusterGPUReadModel(ctx context.Context, store platform.RecordStore) (map[string]any, bool) {
	if store == nil {
		return nil, false
	}
	records := store.List(ctx, clusterReadModelsResource)
	if len(records) == 0 {
		return nil, false
	}
	record := records[len(records)-1]
	if summary := mapValue(record.Data, "summary", "Summary"); len(summary) > 0 {
		return summary, true
	}
	return record.Data, true
}

func podGPUUsageRows(summary map[string]any) []map[string]any {
	rawRows := anySlice(summary, "podGpuUsages", "pod_gpu_usages", "PodGPUUsages")
	rows := make([]map[string]any, 0, len(rawRows))
	for _, raw := range rawRows {
		if row, ok := raw.(map[string]any); ok {
			rows = append(rows, row)
		}
	}
	return rows
}

func normalizePodGPUUsage(data map[string]any, jobs map[string]map[string]any) (normalizedPodGPUUsage, bool) {
	jobID := textValue(data, "job_id", "jobId", "JobID")
	job := jobs[jobID]
	namespace := textValue(data, "pod_namespace", "podNamespace", "PodNamespace", "namespace", "Namespace")
	podName := textValue(data, "pod_name", "podName", "PodName", "pod")
	timestamp := firstTime(data, nil, "timestamp", "Timestamp", "sampled_at", "sampledAt", "SampledAt", "collected_at", "collectedAt", "CollectedAt")
	gpuUUID := textValue(data, "gpu_uuid", "gpuUUID", "GPUUUID", "gpuUuid", "UUID")
	gpuIndex := intValue(data, "gpu_index", "gpuIndex", "GPUIndex")
	if jobID == "" || namespace == "" || podName == "" || timestamp.IsZero() || (gpuUUID == "" && !hasAnyKey(data, "gpu_index", "gpuIndex", "GPUIndex")) {
		return normalizedPodGPUUsage{}, false
	}
	metrics := gpuSnapshotMetrics(data, gpuUUID)
	return normalizedPodGPUUsage{
		jobID:          jobID,
		userID:         shared.FirstNonBlank(textValue(data, "user_id", "userId", "UserID"), textValue(job, "user_id", "userId", "UserID")),
		projectID:      shared.FirstNonBlank(textValue(data, "project_id", "projectId", "ProjectID"), textValue(job, "project_id", "projectId", "ProjectID")),
		namespace:      namespace,
		podName:        podName,
		node:           textValue(data, "node", "Node", "nodeName", "NodeName"),
		gpuUUID:        gpuUUID,
		gpuIndex:       gpuIndex,
		mpsPhysicalIdx: intValue(data, "mps_physical_gpu_index", "mpsPhysicalGPUIndex", "MPSPhysicalGPUIndex"),
		mpsUnits:       intValue(data, "mps_virtual_units", "mpsVirtualUnits", "MPSVirtualUnits", "mpsUnits", "MPSUnits"),
		timestamp:      timestamp.UTC(),
		metrics:        metrics,
	}, true
}

func gpuSnapshotMetrics(data map[string]any, gpuUUID string) map[string]any {
	metrics := mapValue(data, "metrics", "Metrics")
	out := sanitizeRetainedGPUMetrics(metrics)
	if gpuUUID != "" {
		out["gpu_uuid"] = gpuUUID
	}
	copyMetric(out, data, "gpu_memory_bytes", "gpu_memory_bytes", "gpuMemoryBytes", "GPUMemoryBytes", "memoryBytes", "memory_bytes")
	copyMetric(out, data, "gpu_utilization", "gpu_utilization", "gpuUtilization", "GPUUtilization", "utilization")
	copyMetric(out, data, "gpu_sm_utilization", "gpu_sm_utilization", "gpuSMUtilization", "GPUSMUtilization", "smUtilization", "sm_utilization")
	if _, ok := out["gpu_sm_utilization"]; !ok {
		if value := floatValue(out, "gpu_utilization"); value != 0 {
			out["gpu_sm_utilization"] = normalizedUtilizationPercent(value)
		}
	}
	copyMetric(out, data, "gpu_mem_utilization", "gpu_mem_utilization", "gpuMemUtilization", "GPUMemUtilization", "memUtilization", "memoryUtilization")
	copyMetric(out, data, "gpu_memory_used_bytes", "gpu_memory_used_bytes", "gpuMemoryUsedBytes", "GPUMemoryUsedBytes", "memoryUsedBytes")
	copyMetric(out, data, "gpu_enc_utilization", "gpu_enc_utilization", "gpuEncUtilization", "GPUEncUtilization")
	copyMetric(out, data, "gpu_dec_utilization", "gpu_dec_utilization", "gpuDecUtilization", "GPUDecUtilization")
	copyMetric(out, data, "gpu_power_usage_watts", "gpu_power_usage_watts", "gpuPowerUsageWatts", "GPUPowerUsageWatts", "powerUsageWatts")
	copyMetric(out, data, "cpu_usage_cores", "cpu_usage_cores", "cpuUsageCores", "CPUUsageCores")
	copyMetric(out, data, "memory_usage_bytes", "memory_usage_bytes", "memoryUsageBytes", "MemoryUsageBytes")
	copyTextMetric(out, data, "gpu_sm_util_source", "gpu_sm_util_source", "gpuSMUtilSource", "GPUSMUtilSource")
	copyTextMetric(out, data, "gpu_mem_util_source", "gpu_mem_util_source", "gpuMemUtilSource", "GPUMemUtilSource")
	copyTextMetric(out, data, "gpu_memory_used_source", "gpu_memory_used_source", "gpuMemoryUsedSource", "GPUMemoryUsedSource")
	if units := mpsVirtualUnits(data, metrics); units > 0 {
		if _, ok := out["gpu_sm_util_source"]; !ok {
			out["gpu_sm_util_source"] = gpuSMAttributionEstimatedMPS
		}
		if _, ok := out["reserved_sm_percentage"]; !ok {
			out["reserved_sm_percentage"] = units
		}
	}
	return out
}

func sanitizeRetainedGPUMetrics(metrics map[string]any) map[string]any {
	out := make(map[string]any, len(metrics))
	for key, value := range metrics {
		if highCardinalityProcessMetricKey(key) {
			continue
		}
		out[key] = value
	}
	return out
}

func highCardinalityProcessMetricKey(key string) bool {
	switch canonicalProcessMetricKey(key) {
	case "pid",
		"processid",
		"processname",
		"command",
		"cmd",
		"cmdline",
		"args",
		"argv",
		"containerid",
		"poduid",
		"cgroup",
		"cgrouppath",
		"processstarttime":
		return true
	default:
		return false
	}
}

func canonicalProcessMetricKey(key string) string {
	key = strings.TrimSpace(strings.ToLower(key))
	key = strings.ReplaceAll(key, "_", "")
	key = strings.ReplaceAll(key, "-", "")
	return key
}

func copyMetric(out, data map[string]any, target string, keys ...string) {
	if _, ok := out[target]; ok {
		return
	}
	if value := floatValue(data, keys...); value != 0 {
		out[target] = value
	}
}

func copyTextMetric(out, data map[string]any, target string, keys ...string) {
	if _, ok := out[target]; ok {
		return
	}
	if value := textValue(data, keys...); value != "" {
		out[target] = value
	}
}

func normalizedUtilizationPercent(value float64) float64 {
	if value > 0 && value <= 1 {
		return value * 100
	}
	return value
}

func upsertGPUSnapshot(ctx context.Context, store platform.RecordStore, row normalizedPodGPUUsage) bool {
	id := gpuSnapshotID(row)
	data := map[string]any{
		"id":                     id,
		"job_id":                 row.jobID,
		"user_id":                row.userID,
		"project_id":             row.projectID,
		"pod_name":               row.podName,
		"pod_namespace":          row.namespace,
		"node":                   row.node,
		"gpu_index":              row.gpuIndex,
		"mps_physical_gpu_index": row.mpsPhysicalIdx,
		"mps_virtual_units":      row.mpsUnits,
		"timestamp":              row.timestamp,
		"metrics":                row.metrics,
	}
	if _, ok := store.Update(ctx, snapshotsResource, id, data); ok {
		return true
	}
	if _, err := store.Create(ctx, snapshotsResource, data); err != nil {
		if platform.IsCreateConflict(err) {
			_, ok := store.Update(ctx, snapshotsResource, id, data)
			return ok
		}
		slog.Warn("gpu usage collector: snapshot create failed", "snapshot_id", id, "error", err)
		return false
	}
	return true
}

func gpuSnapshotID(row normalizedPodGPUUsage) string {
	gpuKey := row.gpuUUID
	if gpuKey == "" {
		gpuKey = intKey(row.gpuIndex)
	}
	return row.jobID + "/" + row.namespace + "/" + row.podName + "/" + intKey(row.gpuIndex) + "/" + gpuKey + "/" + row.timestamp.Format("20060102150405.000000000")
}

func computeGPUSummaries(ctx context.Context, store platform.RecordStore, jobs map[string]map[string]any, now time.Time) int {
	if store == nil {
		return 0
	}
	written := 0
	for jobID, job := range terminalGPUJobs(jobs) {
		snapshots := snapshotsForJob(ctx, store, jobID)
		if !shouldComputeGPUSummary(ctx, store, jobID, snapshots) {
			continue
		}
		if writeGPUSummary(ctx, store, jobID, gpuSummaryData(jobID, job, snapshots, now)) {
			written++
		}
	}
	return written
}

func shouldComputeGPUSummary(ctx context.Context, store platform.RecordStore, jobID string, snapshots []snapshotRow) bool {
	return len(snapshots) > 0 && summaryNeedsUpdate(ctx, store, jobID, snapshots)
}

func writeGPUSummary(ctx context.Context, store platform.RecordStore, jobID string, data map[string]any) bool {
	summaryID := jobID
	if existing, ok := summaryRecordForJob(ctx, store, jobID); ok {
		summaryID = existing.ID
	}
	if _, ok := store.Update(ctx, summariesResource, summaryID, data); ok {
		return true
	}
	if _, err := store.Create(ctx, summariesResource, data); err != nil {
		return handleGPUSummaryCreateError(ctx, store, jobID, data, err)
	}
	return true
}

func handleGPUSummaryCreateError(ctx context.Context, store platform.RecordStore, jobID string, data map[string]any, err error) bool {
	if !platform.IsCreateConflict(err) {
		slog.Warn("gpu usage collector: summary create failed", "job_id", jobID, "error", err)
		return false
	}
	_, ok := store.Update(ctx, summariesResource, jobID, data)
	return ok
}

func terminalGPUJobs(jobs map[string]map[string]any) map[string]map[string]any {
	out := map[string]map[string]any{}
	for jobID, job := range jobs {
		switch strings.ToLower(textValue(job, "status", "Status")) {
		case "succeeded", "completed", "failed", "cancelled", "canceled":
			if jobID != "" {
				out[jobID] = job
			}
		}
	}
	return out
}

func snapshotsForJob(ctx context.Context, store platform.RecordStore, jobID string) []snapshotRow {
	var rows []snapshotRow
	for _, record := range store.List(ctx, snapshotsResource) {
		data := record.Data
		metrics := mapValue(data, "metrics", "Metrics")
		timestamp := firstTime(data, metrics, "timestamp", "Timestamp")
		if timestamp.IsZero() {
			timestamp = record.CreatedAt
		}
		if textValue(data, "job_id", "jobId", "JobID") != jobID || timestamp.IsZero() {
			continue
		}
		rows = append(rows, snapshotRow{
			record:     record,
			data:       data,
			metrics:    metrics,
			timestamp:  timestamp,
			jobID:      jobID,
			userID:     textValue(data, "user_id", "userId", "UserID"),
			projectID:  textValue(data, "project_id", "projectId", "ProjectID"),
			gpuIndex:   intValue(data, "gpu_index", "gpuIndex", "GPUIndex"),
			gpuUUID:    shared.FirstNonBlank(textValue(data, "gpu_uuid", "gpuUUID", "GPUUUID"), textValue(metrics, "gpu_uuid", "gpuUUID", "GPUUUID")),
			podName:    textValue(data, "pod_name", "podName", "PodName"),
			namespace:  textValue(data, "pod_namespace", "podNamespace", "PodNamespace"),
			mpsUnits:   intValue(data, "mps_virtual_units", "mpsVirtualUnits", "MPSVirtualUnits"),
			mpsGPUIdx:  intValue(data, "mps_physical_gpu_index", "mpsPhysicalGPUIndex", "MPSPhysicalGPUIndex"),
			memoryByte: int64Value(metrics, "gpu_memory_bytes", "gpuMemoryBytes", "GPUMemoryBytes"),
		})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].timestamp.Before(rows[j].timestamp) })
	return rows
}

func summaryNeedsUpdate(ctx context.Context, store platform.RecordStore, jobID string, snapshots []snapshotRow) bool {
	summary, ok := summaryRecordForJob(ctx, store, jobID)
	if !ok {
		return true
	}
	computedAt := firstTime(summary.Data, mapValue(summary.Data, "metrics", "Metrics"), "computed_at", "computedAt", "ComputedAt")
	if computedAt.IsZero() {
		computedAt = summary.UpdatedAt
	}
	latest := snapshots[len(snapshots)-1].timestamp
	return latest.After(computedAt)
}

func summaryRecordForJob(ctx context.Context, store platform.RecordStore, jobID string) (contracts.Record[map[string]any], bool) {
	if byID, ok := store.Get(ctx, summariesResource, jobID); ok {
		return byID, true
	}
	for _, record := range store.List(ctx, summariesResource) {
		if textValue(record.Data, "job_id", "jobId", "JobID") == jobID {
			return record, true
		}
	}
	return contracts.Record[map[string]any]{}, false
}

func gpuSummaryData(jobID string, job map[string]any, snapshots []snapshotRow, now time.Time) map[string]any {
	metrics, breakdowns := gpuSummaryMetrics(snapshots)
	data := map[string]any{
		"id":          jobID,
		"job_id":      jobID,
		"user_id":     shared.FirstNonBlank(textValue(job, "user_id", "userId", "UserID"), snapshots[0].userID),
		"project_id":  shared.FirstNonBlank(textValue(job, "project_id", "projectId", "ProjectID"), snapshots[0].projectID),
		"computed_at": now,
		"metrics":     metrics,
		"breakdowns":  breakdowns,
	}
	return data
}

func gpuSummaryMetrics(snapshots []snapshotRow) (map[string]any, []any) {
	first := snapshots[0].timestamp
	last := snapshots[len(snapshots)-1].timestamp
	timeSlots := map[time.Time]map[string]float64{}
	breakdowns := map[string]*gpuBreakdownAccum{}
	var sumUtil, sumCPU, sumSM, sumMemUtil, sumEnc, sumDec float64
	var sumMem, peakMem, sumMemBytes, peakMemUsed int64
	for _, snap := range snapshots {
		metrics := snap.metrics
		if snap.timestamp.Before(first) {
			first = snap.timestamp
		}
		if snap.timestamp.After(last) {
			last = snap.timestamp
		}
		mem := int64Value(metrics, "gpu_memory_bytes", "gpuMemoryBytes", "GPUMemoryBytes")
		memUsed := int64Value(metrics, "gpu_memory_used_bytes", "gpuMemoryUsedBytes", "GPUMemoryUsedBytes")
		if mem > peakMem {
			peakMem = mem
		}
		if memUsed > peakMemUsed {
			peakMemUsed = memUsed
		}
		sumUtil += floatValue(metrics, "gpu_utilization", "gpuUtilization", "GPUUtilization")
		sumMem += mem
		sumCPU += floatValue(metrics, "cpu_usage_cores", "cpuUsageCores", "CPUUsageCores")
		sumMemBytes += int64Value(metrics, "memory_usage_bytes", "memoryUsageBytes", "MemoryUsageBytes")
		sumSM += floatValue(metrics, "gpu_sm_utilization", "gpuSMUtilization", "GPUSMUtilization")
		sumMemUtil += floatValue(metrics, "gpu_mem_utilization", "gpuMemUtilization", "GPUMemUtilization")
		sumEnc += floatValue(metrics, "gpu_enc_utilization", "gpuEncUtilization", "GPUEncUtilization")
		sumDec += floatValue(metrics, "gpu_dec_utilization", "gpuDecUtilization", "GPUDecUtilization")
		bucket := snap.timestamp.Truncate(time.Second)
		slot := timeSlots[bucket]
		if slot == nil {
			slot = map[string]float64{}
			timeSlots[bucket] = slot
		}
		unitKey := snap.namespace + "/" + snap.podName + ":" + intKey(snap.gpuIndex) + ":" + snap.gpuUUID
		if units := snapshotGPUUnits(snap); units > slot[unitKey] {
			slot[unitKey] = units
		}
		key := snap.gpuUUID + ":" + intKey(snap.gpuIndex)
		if breakdowns[key] == nil {
			breakdowns[key] = &gpuBreakdownAccum{uuid: snap.gpuUUID, index: snap.gpuIndex, node: textValue(snap.data, "node", "Node")}
		}
		breakdowns[key].add(snap, memUsed)
	}
	totalGPUSeconds := totalGPUSeconds(timeSlots)
	sampleCount := len(snapshots)
	avgCPU := averageFloat(sumCPU, sampleCount)
	avgMemoryMB := averageMemoryMB(sumMemBytes, sampleCount)
	metrics := map[string]any{
		"total_gpu_seconds":       totalGPUSeconds,
		"peak_memory_bytes":       peakMem,
		"avg_utilization":         averageFloat(sumUtil, sampleCount),
		"avg_memory_bytes":        averageInt64(sumMem, sampleCount),
		"sample_count":            sampleCount,
		"first_sample_at":         first,
		"last_sample_at":          last,
		"total_cpu_seconds":       totalGPUSeconds * avgCPU,
		"total_memory_seconds_mb": totalGPUSeconds * avgMemoryMB,
		"avg_sm_utilization":      averageFloat(sumSM, sampleCount),
		"avg_mem_utilization":     averageFloat(sumMemUtil, sampleCount),
		"peak_memory_used_bytes":  peakMemUsed,
		"avg_enc_utilization":     averageFloat(sumEnc, sampleCount),
		"avg_dec_utilization":     averageFloat(sumDec, sampleCount),
	}
	return metrics, breakdownEntries(breakdowns, totalGPUSeconds, sampleCount)
}

type gpuBreakdownAccum struct {
	uuid        string
	index       int
	node        string
	count       int
	sumSM       float64
	sumMemUsed  int64
	peakMemUsed int64
}

func (a *gpuBreakdownAccum) add(row snapshotRow, memUsed int64) {
	a.count++
	a.sumSM += floatValue(row.metrics, "gpu_sm_utilization", "gpuSMUtilization", "GPUSMUtilization")
	a.sumMemUsed += memUsed
	if memUsed > a.peakMemUsed {
		a.peakMemUsed = memUsed
	}
}

func snapshotGPUUnits(row snapshotRow) float64 {
	if row.mpsUnits > 0 {
		return float64(row.mpsUnits) / 100.0
	}
	return 1
}

func totalGPUSeconds(slots map[time.Time]map[string]float64) float64 {
	buckets := make([]time.Time, 0, len(slots))
	for bucket := range slots {
		buckets = append(buckets, bucket)
	}
	sort.Slice(buckets, func(i, j int) bool { return buckets[i].Before(buckets[j]) })
	interval := medianInterval(buckets)
	var total float64
	for i, bucket := range buckets {
		dt := interval
		if i+1 < len(buckets) {
			if next := buckets[i+1].Sub(bucket); next > 0 {
				dt = next
			}
		}
		var units float64
		for _, value := range slots[bucket] {
			units += value
		}
		total += dt.Seconds() * units
	}
	return total
}

func medianInterval(buckets []time.Time) time.Duration {
	if len(buckets) < 2 {
		return time.Second
	}
	intervals := make([]time.Duration, 0, len(buckets)-1)
	for i := 1; i < len(buckets); i++ {
		if diff := buckets[i].Sub(buckets[i-1]); diff > 0 {
			intervals = append(intervals, diff)
		}
	}
	if len(intervals) == 0 {
		return time.Second
	}
	sort.Slice(intervals, func(i, j int) bool { return intervals[i] < intervals[j] })
	return intervals[len(intervals)/2]
}

func breakdownEntries(acc map[string]*gpuBreakdownAccum, totalGPUSeconds float64, sampleCount int) []any {
	keys := make([]string, 0, len(acc))
	for key := range acc {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]any, 0, len(keys))
	for _, key := range keys {
		item := acc[key]
		entry := map[string]any{
			"gpu_uuid":            item.uuid,
			"gpu_index":           item.index,
			"node":                item.node,
			"sample_count":        item.count,
			"avg_sm_utilization":  averageFloat(item.sumSM, item.count),
			"avg_mem_used_bytes":  averageInt64(item.sumMemUsed, item.count),
			"peak_mem_used_bytes": item.peakMemUsed,
			"total_gpu_seconds":   proportionalSeconds(totalGPUSeconds, item.count, sampleCount),
		}
		out = append(out, entry)
	}
	return out
}

func proportionalSeconds(total float64, count, sampleCount int) float64 {
	if sampleCount == 0 {
		return 0
	}
	return total * float64(count) / float64(sampleCount)
}

func averageFloat(sum float64, count int) float64 {
	if count == 0 {
		return 0
	}
	return sum / float64(count)
}

func averageInt64(sum int64, count int) int64 {
	if count == 0 {
		return 0
	}
	return sum / int64(count)
}

func averageMemoryMB(sumBytes int64, count int) float64 {
	if count == 0 {
		return 0
	}
	return float64(sumBytes) / float64(count) / (1024 * 1024)
}

func cleanupOldGPUSnapshots(ctx context.Context, store platform.RecordStore, cutoff time.Time) int {
	if store == nil || cutoff.IsZero() {
		return 0
	}
	deleted := 0
	for _, record := range store.List(ctx, snapshotsResource) {
		timestamp := firstTime(record.Data, mapValue(record.Data, "metrics", "Metrics"), "timestamp", "Timestamp")
		if timestamp.IsZero() {
			timestamp = record.CreatedAt
		}
		if timestamp.Before(cutoff) && store.Delete(ctx, snapshotsResource, record.ID) {
			deleted++
		}
	}
	return deleted
}

func retentionCutoff(now time.Time, days int) time.Time {
	if days <= 0 {
		days = 30
	}
	return now.Add(-time.Duration(days) * 24 * time.Hour)
}

func anySlice(data map[string]any, keys ...string) []any {
	for _, key := range keys {
		switch value := data[key].(type) {
		case []any:
			return value
		case []map[string]any:
			out := make([]any, 0, len(value))
			for _, item := range value {
				out = append(out, item)
			}
			return out
		}
	}
	return nil
}

func hasAnyKey(data map[string]any, keys ...string) bool {
	for _, key := range keys {
		if _, ok := data[key]; ok {
			return true
		}
	}
	return false
}

func registerGPUUsageCollector(app *platform.App) {
	app.RegisterMaintenanceTaskForService(serviceName, gpuCollectorTaskName, func(ctx context.Context) error {
		collectGPUUsageTelemetry(ctx, app, time.Now().UTC())
		return nil
	})
}
