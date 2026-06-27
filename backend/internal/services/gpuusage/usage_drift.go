package gpuusage

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

const (
	usageDriftAlertsResource         = serviceName + ":usage_drift_alerts"
	usageDriftDetectedEventName      = "UsageDriftDetected"
	usageDriftReasonMaterialMismatch = "material_reservation_telemetry_divergence"
	usageDriftTraceIDPrefix          = "usage-drift-"
	usageDriftIdempotencyKeyScope    = "usage-drift-detector:"
	usageDriftAbsoluteThreshold      = 0.25
	usageDriftRelativeThreshold      = 0.25
)

type usageDriftProjectStats struct {
	projectID     string
	reservedGPU   float64
	reservedFound bool
	observedGPU   float64
	freshRows     int
	firstSample   time.Time
	lastSample    time.Time
}

func detectUsageDrift(ctx context.Context, app *platform.App, jobs map[string]map[string]any, now time.Time) int {
	if app == nil || app.Store == nil {
		return 0
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	windowMinutes := snapshotWindowMinutes(app.Config)
	stats := usageDriftProjectStatsForFreshSnapshots(ctx, app.Store, jobs, now, windowMinutes)
	published := 0
	for _, stat := range stats {
		if !stat.hasPositiveReservedEvidence() {
			continue
		}
		data, drifted := stat.usageDriftEventData(now, windowMinutes)
		if !drifted {
			resolveUsageDriftAlert(ctx, app.Store, stat.projectID, usageDriftReasonMaterialMismatch, now)
			continue
		}
		shouldPublish, eventData := upsertUsageDriftAlert(ctx, app.Store, data, now)
		if !shouldPublish {
			continue
		}
		if err := publishUsageDriftDetected(ctx, app.Events, eventData, now); err != nil {
			slog.Warn("usage drift event publish failed", "project_id", stat.projectID, "error", err)
			continue
		}
		published++
	}
	if len(stats) > 0 || published > 0 {
		slog.Info("usage drift detector completed", "projects_scanned", len(stats), "drifts_published", published)
	}
	return published
}

func usageDriftProjectStatsForFreshSnapshots(ctx context.Context, store platform.RecordStore, jobs map[string]map[string]any, now time.Time, windowMinutes int) []usageDriftProjectStats {
	latest := usageDriftLatestSnapshots(ctx, store, jobs, now.Add(-time.Duration(windowMinutes)*time.Minute))
	statsByProject := map[string]*usageDriftProjectStats{}
	jobReservedSeen := map[string]bool{}
	for _, row := range latest {
		if row.projectID == "" {
			continue
		}
		stat := usageDriftProjectStat(statsByProject, row.projectID)
		stat.addObserved(row)
		stat.addReservedFromSnapshot(row, jobReservedSeen)
	}
	return sortedUsageDriftProjectStats(statsByProject)
}

func usageDriftProjectStat(stats map[string]*usageDriftProjectStats, projectID string) *usageDriftProjectStats {
	stat := stats[projectID]
	if stat != nil {
		return stat
	}
	stat = &usageDriftProjectStats{projectID: projectID}
	stats[projectID] = stat
	return stat
}

func sortedUsageDriftProjectStats(statsByProject map[string]*usageDriftProjectStats) []usageDriftProjectStats {
	out := make([]usageDriftProjectStats, 0, len(statsByProject))
	for _, stat := range statsByProject {
		out = append(out, *stat)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].projectID < out[j].projectID })
	return out
}

func usageDriftLatestSnapshots(ctx context.Context, store platform.RecordStore, jobs map[string]map[string]any, cutoff time.Time) []snapshotRow {
	if store == nil {
		return nil
	}
	latest := map[string]snapshotRow{}
	for index, record := range store.List(ctx, snapshotsResource) {
		row, ok := usageDriftSnapshotRow(record, jobs)
		if !ok || (!row.timestamp.IsZero() && row.timestamp.Before(cutoff)) || !usageDriftJobActive(row.job) {
			continue
		}
		key := usageDriftSnapshotKey(index, row)
		if existing, found := latest[key]; !found || row.timestamp.After(existing.timestamp) {
			latest[key] = row
		}
	}
	out := make([]snapshotRow, 0, len(latest))
	for _, row := range latest {
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].projectID == out[j].projectID {
			if out[i].jobID == out[j].jobID {
				return out[i].timestamp.Before(out[j].timestamp)
			}
			return out[i].jobID < out[j].jobID
		}
		return out[i].projectID < out[j].projectID
	})
	return out
}

func usageDriftSnapshotRow(record contracts.Record[map[string]any], jobs map[string]map[string]any) (snapshotRow, bool) {
	data := record.Data
	metrics := mapValue(data, "metrics", "Metrics")
	timestamp := firstTime(data, metrics, "timestamp", "Timestamp")
	if timestamp.IsZero() {
		timestamp = record.CreatedAt
	}
	if timestamp.IsZero() {
		return snapshotRow{}, false
	}
	jobID := textValue(data, "job_id", "jobId", "JobID")
	job := jobs[jobID]
	projectID := shared.FirstNonBlank(textValue(data, "project_id", "projectId", "ProjectID"), textValue(job, "project_id", "projectId", "ProjectID"))
	if projectID == "" {
		return snapshotRow{}, false
	}
	return snapshotRow{
		record:    record,
		data:      data,
		metrics:   metrics,
		job:       job,
		timestamp: timestamp.UTC(),
		jobID:     jobID,
		userID:    shared.FirstNonBlank(textValue(data, "user_id", "userId", "UserID"), textValue(job, "user_id", "userId", "UserID")),
		projectID: projectID,
		gpuIndex:  intValue(data, "gpu_index", "gpuIndex", "GPUIndex"),
		gpuUUID:   shared.FirstNonBlank(textValue(data, "gpu_uuid", "gpuUUID", "GPUUUID"), textValue(metrics, "gpu_uuid", "gpuUUID", "GPUUUID")),
		podName:   textValue(data, "pod_name", "podName", "PodName"),
		namespace: textValue(data, "pod_namespace", "podNamespace", "PodNamespace"),
		mpsUnits:  intValue(data, "mps_virtual_units", "mpsVirtualUnits", "MPSVirtualUnits"),
		mpsGPUIdx: intValue(data, "mps_physical_gpu_index", "mpsPhysicalGPUIndex", "MPSPhysicalGPUIndex"),
	}, true
}

func usageDriftSnapshotKey(index int, row snapshotRow) string {
	key := strings.Join([]string{
		row.projectID,
		row.jobID,
		row.namespace,
		row.podName,
		shared.FirstNonBlank(row.gpuUUID, intKey(row.gpuIndex)),
	}, "\x00")
	if strings.Trim(key, "\x00") != "" {
		return key
	}
	return strconv.Itoa(index)
}

func usageDriftJobActive(job map[string]any) bool {
	switch strings.ToLower(textValue(job, "status", "Status")) {
	case "", "submitted", "waiting_infra", "queued", "pending", "scheduled", "dispatching", "running":
		return true
	default:
		return false
	}
}

func (s *usageDriftProjectStats) addObserved(row snapshotRow) {
	s.observedGPU += snapshotGPUUnits(row)
	s.freshRows++
	if s.firstSample.IsZero() || row.timestamp.Before(s.firstSample) {
		s.firstSample = row.timestamp
	}
	if s.lastSample.IsZero() || row.timestamp.After(s.lastSample) {
		s.lastSample = row.timestamp
	}
}

func (s *usageDriftProjectStats) addReserved(value float64) {
	if value <= 0 {
		return
	}
	s.reservedGPU += value
	s.reservedFound = true
}

func (s *usageDriftProjectStats) addReservedFromSnapshot(row snapshotRow, jobReservedSeen map[string]bool) {
	if s.addJobReservedFromSnapshot(row, jobReservedSeen) {
		return
	}
	if reserved, ok := usageDriftRowReservedGPUFraction(row); ok {
		s.addReserved(reserved)
	}
}

func (s *usageDriftProjectStats) addJobReservedFromSnapshot(row snapshotRow, jobReservedSeen map[string]bool) bool {
	if row.jobID == "" {
		return false
	}
	reserved, ok := usageDriftJobReservedGPUFraction(row.job)
	if !ok {
		return false
	}
	jobKey := row.projectID + "\x00" + row.jobID
	if !jobReservedSeen[jobKey] {
		s.addReserved(reserved)
		jobReservedSeen[jobKey] = true
	}
	return true
}

func (s usageDriftProjectStats) hasPositiveReservedEvidence() bool {
	return s.reservedFound && s.reservedGPU > 0
}

func (s usageDriftProjectStats) usageDriftEventData(detectedAt time.Time, windowMinutes int) (map[string]any, bool) {
	diff := math.Abs(s.reservedGPU - s.observedGPU)
	ratio := diff / s.reservedGPU
	if diff < usageDriftAbsoluteThreshold || ratio < usageDriftRelativeThreshold {
		return nil, false
	}
	data := map[string]any{
		"project_id":                   s.projectID,
		"reason":                       usageDriftReasonMaterialMismatch,
		"drift_reason":                 usageDriftReasonMaterialMismatch,
		"reserved_gpu_fraction":        roundUsageDriftValue(s.reservedGPU),
		"observed_gpu_fraction":        roundUsageDriftValue(s.observedGPU),
		"drift_gpu_fraction":           roundUsageDriftValue(diff),
		"drift_ratio":                  roundUsageDriftValue(ratio),
		"absolute_threshold_gpu":       usageDriftAbsoluteThreshold,
		"relative_threshold":           usageDriftRelativeThreshold,
		"snapshot_window_minutes":      windowMinutes,
		"fresh_rows_seen":              s.freshRows,
		"first_sample_at":              s.firstSample.UTC().Format(time.RFC3339),
		"last_sample_at":               s.lastSample.UTC().Format(time.RFC3339),
		"detected_at":                  detectedAt.UTC().Format(time.RFC3339),
		"basis":                        "fresh_job_gpu_usage_snapshots",
		"reserved_observed_comparison": "project_gpu_fraction",
	}
	return data, true
}

func usageDriftJobReservedGPUFraction(job map[string]any) (float64, bool) {
	if len(job) == 0 {
		return 0, false
	}
	payload := mapValue(job, "reservation_payload", "reservationPayload", "ReservationPayload")
	reserved := mapValue(payload, "reserved", "Reserved")
	if value, ok := usageDriftNumericValue(reserved, "gpu", "GPU", "gpus", "GPUs"); ok {
		return value, true
	}
	if value, ok := usageDriftNumericValue(job, "required_gpu", "requiredGPU", "requiredGpu", "RequiredGPU", "RequiredGpu"); ok {
		return value, true
	}
	return usageDriftAllocationFraction(job)
}

func usageDriftRowReservedGPUFraction(row snapshotRow) (float64, bool) {
	if value, ok := usageDriftAllocationFraction(row.data); ok {
		return value, true
	}
	if value, ok := usageDriftAllocationFraction(row.metrics); ok {
		return value, true
	}
	return 0, false
}

func usageDriftAllocationFraction(data map[string]any) (float64, bool) {
	if value, ok := usageDriftNumericValue(data, "reserved_gpu_fraction", "reservedGpuFraction", "ReservedGPUFraction"); ok {
		return value, true
	}
	if value, ok := usageDriftNumericValue(data, "dra_effective_gpu", "draEffectiveGPU", "draEffectiveGpu", "DRAEffectiveGPU", "DRAEffectiveGpu"); ok {
		return value, true
	}
	if value, ok := usageDriftNumericValue(data, "requested_gpu", "requestedGPU", "RequestedGPU"); ok {
		return value, true
	}
	gpuCount, gpuOK := usageDriftNumericValue(data, "gpu_count", "gpuCount", "GPUCount")
	smPercent, smOK := usageDriftNumericValue(data, "sm_percentage", "smPercentage", "SMPercentage")
	if gpuOK && smOK {
		return gpuCount * smPercent / 100, true
	}
	if value, ok := usageDriftNumericValue(data, "mps_virtual_units", "mpsVirtualUnits", "MPSVirtualUnits", "mpsUnits", "MPSUnits"); ok {
		return value / 100, true
	}
	return 0, false
}

func usageDriftNumericValue(data map[string]any, keys ...string) (float64, bool) {
	if data == nil {
		return 0, false
	}
	for _, key := range keys {
		switch value := data[key].(type) {
		case int:
			return float64(value), true
		case int8:
			return float64(value), true
		case int16:
			return float64(value), true
		case int32:
			return float64(value), true
		case int64:
			return float64(value), true
		case uint:
			return float64(value), true
		case uint8:
			return float64(value), true
		case uint16:
			return float64(value), true
		case uint32:
			return float64(value), true
		case uint64:
			return float64(value), true
		case float32:
			return float64(value), true
		case float64:
			return value, true
		case json.Number:
			if parsed, err := value.Float64(); err == nil {
				return parsed, true
			}
		case string:
			if parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64); err == nil {
				return parsed, true
			}
		}
	}
	return 0, false
}

func upsertUsageDriftAlert(ctx context.Context, store platform.RecordStore, data map[string]any, now time.Time) (bool, map[string]any) {
	if store == nil {
		return false, nil
	}
	alertID := usageDriftAlertID(textValue(data, "project_id"), textValue(data, "reason", "drift_reason"))
	fingerprint := usageDriftFingerprint(data)
	eventData := shared.CloneMap(data)
	eventData["alert_id"] = alertID
	eventData["fingerprint"] = fingerprint
	alertData := shared.CloneMap(eventData)
	alertData["id"] = alertID
	alertData["status"] = "active"
	alertData["last_seen_at"] = now.UTC().Format(time.RFC3339)
	if existing, ok := store.Get(ctx, usageDriftAlertsResource, alertID); ok {
		if textValue(existing.Data, "status") == "active" && textValue(existing.Data, "fingerprint") == fingerprint {
			_, _ = store.Update(ctx, usageDriftAlertsResource, alertID, map[string]any{"last_seen_at": now.UTC().Format(time.RFC3339)})
			return false, eventData
		}
		if _, ok := store.Update(ctx, usageDriftAlertsResource, alertID, alertData); ok {
			return true, eventData
		}
		return false, eventData
	}
	if _, err := store.Create(ctx, usageDriftAlertsResource, alertData); err != nil {
		if platform.IsCreateConflict(err) {
			if _, ok := store.Update(ctx, usageDriftAlertsResource, alertID, alertData); ok {
				return true, eventData
			}
		}
		slog.Warn("usage drift alert state write failed", "alert_id", alertID, "error", err)
		return false, eventData
	}
	return true, eventData
}

func resolveUsageDriftAlert(ctx context.Context, store platform.RecordStore, projectID, reason string, now time.Time) {
	if store == nil || projectID == "" {
		return
	}
	alertID := usageDriftAlertID(projectID, reason)
	existing, ok := store.Get(ctx, usageDriftAlertsResource, alertID)
	if !ok || textValue(existing.Data, "status") != "active" {
		return
	}
	_, _ = store.Update(ctx, usageDriftAlertsResource, alertID, map[string]any{
		"status":       "resolved",
		"resolved_at":  now.UTC().Format(time.RFC3339),
		"last_seen_at": now.UTC().Format(time.RFC3339),
	})
}

func usageDriftAlertID(projectID, reason string) string {
	return strings.TrimSpace(projectID) + ":" + strings.TrimSpace(reason)
}

func usageDriftFingerprint(data map[string]any) string {
	return fmt.Sprintf("%s:reserved=%.3f:observed=%.3f",
		textValue(data, "reason", "drift_reason"),
		floatValue(data, "reserved_gpu_fraction"),
		floatValue(data, "observed_gpu_fraction"),
	)
}

func publishUsageDriftDetected(ctx context.Context, events platform.EventStream, data map[string]any, detectedAt time.Time) error {
	if events == nil {
		return nil
	}
	alertID := textValue(data, "alert_id")
	fingerprint := textValue(data, "fingerprint")
	event := contracts.Event{
		EventID:        platform.NewUUID(),
		Name:           usageDriftDetectedEventName,
		Source:         serviceName,
		OccurredAt:     detectedAt.UTC(),
		TraceID:        usageDriftTraceIDPrefix + detectedAt.UTC().Format("20060102150405"),
		SchemaVersion:  1,
		IdempotencyKey: usageDriftIdempotencyKeyScope + alertID + ":" + fingerprint,
		Data:           shared.CloneMap(data),
	}
	if err := events.Publish(ctx, event); err != nil {
		return fmt.Errorf("publish usage drift event: %w", err)
	}
	return nil
}

func roundUsageDriftValue(value float64) float64 {
	return math.Round(value*1000) / 1000
}
