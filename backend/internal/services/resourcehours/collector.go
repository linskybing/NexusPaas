package resourcehours

import (
	"context"
	"log/slog"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/platform/cluster"
)

// collectResourceHours ports reference StartResourceHoursCollector into the
// usage-observability service. One maintenance pass scans active job pods, marks
// previously tracked missing pods terminated, and recomputes per-job summaries.
func collectResourceHours(ctx context.Context, cl *cluster.Client, store platform.RecordStore, now time.Time) error {
	if cl == nil || store == nil {
		return nil
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	usages, err := cl.ListJobPodResourceUsage(ctx, now)
	if err != nil {
		return err
	}
	seen := make(map[string]bool, len(usages))
	for _, usage := range usages {
		id := podUsageID(usage.JobID, usage.Namespace, usage.Name, usage.UID)
		seen[id] = true
		upsertRecord(ctx, store, podRecordsResource, id, podUsageData(id, usage))
	}
	markMissingPodRecordsTerminated(ctx, store, seen, now)
	computeResourceHourSummaries(ctx, store, now)
	return nil
}

func podUsageData(id string, usage cluster.PodResourceUsage) map[string]any {
	data := map[string]any{
		"id":                  id,
		"job_id":              usage.JobID,
		"project_id":          usage.ProjectID,
		"user_id":             usage.UserID,
		"pod_namespace":       usage.Namespace,
		"pod_name":            usage.Name,
		"pod_uid":             usage.UID,
		"requested_gpu":       usage.RequestedGPU,
		"requested_cpu":       usage.RequestedCPU,
		"requested_memory_mb": usage.RequestedMemoryMB,
		"scheduled_at":        usage.ScheduledAt,
		"last_seen_at":        usage.LastSeenAt,
		"pod_phase":           usage.Phase,
		"is_active":           usage.IsActive,
	}
	if usage.RunningAt != nil {
		data["running_at"] = *usage.RunningAt
	}
	if usage.TerminatedAt != nil {
		data["terminated_at"] = *usage.TerminatedAt
	}
	return data
}

func markMissingPodRecordsTerminated(ctx context.Context, store platform.RecordStore, seen map[string]bool, now time.Time) {
	for _, record := range store.List(ctx, podRecordsResource) {
		if seen[record.ID] || !boolValue(record.Data, "is_active", "isActive") {
			continue
		}
		if _, ok := store.Update(ctx, podRecordsResource, record.ID, map[string]any{
			"is_active":     false,
			"pod_phase":     "Missing",
			"terminated_at": now,
			"last_seen_at":  now,
		}); !ok {
			slog.Warn("resource-hours collector: failed to mark missing pod terminated", "record_id", record.ID)
		}
	}
}

func computeResourceHourSummaries(ctx context.Context, store platform.RecordStore, now time.Time) {
	grouped := map[string][]contracts.Record[map[string]any]{}
	for _, record := range store.List(ctx, podRecordsResource) {
		jobID := textValue(record.Data, "job_id", "jobId", "JobID")
		if jobID != "" {
			grouped[jobID] = append(grouped[jobID], record)
		}
	}
	for jobID, records := range grouped {
		upsertRecord(ctx, store, resourceName, jobID, summaryData(jobID, records, now))
	}
}

func summaryData(jobID string, records []contracts.Record[map[string]any], now time.Time) map[string]any {
	summary := resourceHourSummary{jobID: jobID, finalized: true}
	for _, record := range records {
		summary.add(podUsageRecordFromMap(record.Data), now)
	}
	return summary.data(now)
}

type resourceHourSummary struct {
	jobID      string
	userID     string
	projectID  string
	gpuSeconds float64
	cpuSeconds float64
	memSeconds float64
	first      *time.Time
	last       *time.Time
	finalized  bool
}

func (s *resourceHourSummary) add(record podUsageRecord, now time.Time) {
	if s.userID == "" {
		s.userID = record.userID
	}
	if s.projectID == "" {
		s.projectID = record.projectID
	}
	if record.active {
		s.finalized = false
	}
	if record.runningAt == nil || record.runningAt.IsZero() {
		return
	}
	end := record.billableEnd(now)
	if end.Before(*record.runningAt) {
		return
	}
	seconds := end.Sub(*record.runningAt).Seconds()
	s.gpuSeconds += record.gpu * seconds
	s.cpuSeconds += record.cpu * seconds
	s.memSeconds += record.memoryMB * seconds
	s.first, s.last = updateSummaryBounds(s.first, s.last, *record.runningAt, end)
}

func (s resourceHourSummary) data(now time.Time) map[string]any {
	data := map[string]any{
		"id":                   s.jobID,
		"job_id":               s.jobID,
		"user_id":              s.userID,
		"project_id":           s.projectID,
		"gpu_hours":            s.gpuSeconds / 3600,
		"cpu_hours":            s.cpuSeconds / 3600,
		"memory_gb_hours":      s.memSeconds / (1024 * 3600),
		"total_gpu_seconds":    s.gpuSeconds,
		"total_cpu_seconds":    s.cpuSeconds,
		"total_memory_seconds": s.memSeconds,
		"is_finalized":         s.finalized,
		"last_computed_at":     now,
	}
	if s.first != nil {
		data["period_start"] = *s.first
	}
	if s.last != nil {
		data["period_end"] = *s.last
	}
	return data
}

type podUsageRecord struct {
	userID    string
	projectID string
	gpu       float64
	cpu       float64
	memoryMB  float64
	runningAt *time.Time
	endedAt   *time.Time
	lastSeen  *time.Time
	active    bool
}

func podUsageRecordFromMap(data map[string]any) podUsageRecord {
	return podUsageRecord{
		userID:    textValue(data, "user_id", "userId", "UserID"),
		projectID: textValue(data, "project_id", "projectId", "ProjectID"),
		gpu:       floatValue(data, "requested_gpu", "requestedGpu", "RequestedGPU"),
		cpu:       floatValue(data, "requested_cpu", "requestedCpu", "RequestedCPU"),
		memoryMB:  floatValue(data, "requested_memory_mb", "requestedMemoryMB", "RequestedMemoryMB"),
		runningAt: timeValue(data, "running_at", "runningAt", "RunningAt"),
		endedAt:   timeValue(data, "terminated_at", "terminatedAt", "TerminatedAt"),
		lastSeen:  timeValue(data, "last_seen_at", "lastSeenAt", "LastSeenAt"),
		active:    boolValue(data, "is_active", "isActive", "IsActive"),
	}
}

func (r podUsageRecord) billableEnd(now time.Time) time.Time {
	if r.endedAt != nil && !r.endedAt.IsZero() {
		return *r.endedAt
	}
	if r.active {
		return now
	}
	if r.lastSeen != nil && !r.lastSeen.IsZero() {
		return *r.lastSeen
	}
	return now
}

func updateSummaryBounds(first, last *time.Time, start, end time.Time) (*time.Time, *time.Time) {
	if first == nil || start.Before(*first) {
		value := start
		first = &value
	}
	if last == nil || end.After(*last) {
		value := end
		last = &value
	}
	return first, last
}

func podUsageID(jobID, namespace, podName, podUID string) string {
	return jobID + "/" + namespace + "/" + podName + "/" + podUID
}

func upsertRecord(ctx context.Context, store platform.RecordStore, resource, id string, data map[string]any) {
	if _, found := store.Get(ctx, resource, id); found {
		if _, ok := store.Update(ctx, resource, id, data); !ok {
			slog.Warn("resource-hours collector: update skipped", "resource", resource, "id", id)
		}
		return
	}
	if _, err := store.Create(ctx, resource, data); err != nil {
		slog.Warn("resource-hours collector: create failed", "resource", resource, "id", id, "error", err)
	}
}

func registerResourceHoursCollector(app *platform.App) {
	app.RegisterMaintenanceTaskForService(serviceName, "resource-hours-collector", func(ctx context.Context) error {
		return collectResourceHours(ctx, app.Cluster, app.Store, time.Now().UTC())
	})
}
