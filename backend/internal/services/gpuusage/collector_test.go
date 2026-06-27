package gpuusage

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

func TestCollectGPUUsageTelemetryProducesSnapshotsAndSummary(t *testing.T) {
	app := newGPUCollectorTestApp()
	now := time.Date(2026, time.April, 5, 12, 0, 0, 0, time.UTC)
	sampleA := now.Add(-time.Minute)
	sampleB := now
	seedGPUCollectorReadModels(t, app, "succeeded")
	seedGPUCollectorClusterReadModel(t, app, sampleA, sampleB)

	stats := collectGPUUsageTelemetry(context.Background(), app, now)
	if stats.podRowsScanned != 3 || stats.snapshotsWritten != 2 || stats.snapshotsSkipped != 1 || stats.summariesComputed != 1 {
		t.Fatalf("collector stats = %#v, want scanned=3 written=2 skipped=1 summaries=1", stats)
	}

	snapshots := app.Store.List(context.Background(), snapshotsResource)
	if len(snapshots) != 2 {
		t.Fatalf("snapshots = %#v, want 2", snapshots)
	}
	first := snapshots[0].Data
	if first["job_id"] != "JGPU" || first["user_id"] != "U1" || first["project_id"] != "P1" {
		t.Fatalf("snapshot identity = %#v, want job/user/project", first)
	}
	metrics := mapValue(first, "metrics")
	if metrics["gpu_uuid"] != "GPU-a" || int64Value(metrics, "gpu_memory_bytes") != 4096 || floatValue(metrics, "gpu_sm_utilization") != 75 {
		t.Fatalf("snapshot metrics = %#v, want normalized GPU metrics", metrics)
	}
	assertEstimatedSMAttribution(t, metrics)

	summary, ok := summaryRecordForJob(context.Background(), app.Store, "JGPU")
	if !ok {
		t.Fatal("missing generated summary for terminal GPU job")
	}
	summaryMetrics := mapValue(summary.Data, "metrics")
	if intValue(summaryMetrics, "sample_count") != 2 {
		t.Fatalf("summary metrics = %#v, want sample_count=2", summaryMetrics)
	}
	if got := floatValue(summaryMetrics, "total_gpu_seconds"); got != 60 {
		t.Fatalf("total_gpu_seconds = %v, want 60 for two 50%% MPS samples one minute apart", got)
	}
	breakdowns := anySlice(summary.Data, "breakdowns")
	if len(breakdowns) != 1 {
		t.Fatalf("breakdowns = %#v, want one GPU breakdown", breakdowns)
	}

	code, data, _ := listAdminUsage(app, gpuRequest("/api/v1/admin/usage?since=2026-04-01", "ADMIN"), platform.RouteSpec{})
	if code != 200 {
		t.Fatalf("admin usage status=%d data=%#v, want 200", code, data)
	}
	rows := data.([]UserResourceUsage)
	if len(rows) != 1 || rows[0].JobID != "JGPU" || rows[0].UserID != "U1" || rows[0].ProjectName != "vision" {
		t.Fatalf("admin usage rows = %#v, want collector-produced JGPU summary", rows)
	}
}

func assertEstimatedSMAttribution(t *testing.T, metrics map[string]any) {
	t.Helper()
	if textValue(metrics, "gpu_sm_util_source") != gpuSMAttributionEstimatedMPS || intValue(metrics, "reserved_sm_percentage") != 50 {
		t.Fatalf("snapshot SM attribution = %#v, want estimated MPS allocation label", metrics)
	}
}

func TestCollectGPUUsageTelemetryUpdatesExistingSummaryByJobID(t *testing.T) {
	app := newGPUCollectorTestApp()
	now := time.Date(2026, time.April, 5, 12, 0, 0, 0, time.UTC)
	seedGPUCollectorReadModels(t, app, "completed")
	seedGPUCollectorClusterReadModel(t, app, now.Add(-time.Minute), now)
	createGPUCollectorRecord(t, app, summariesResource, map[string]any{
		"id":          "legacy-summary",
		"job_id":      "JGPU",
		"computed_at": now.Add(-time.Hour),
		"metrics":     map[string]any{"sample_count": 1},
	})

	stats := collectGPUUsageTelemetry(context.Background(), app, now)
	if stats.summariesComputed != 1 {
		t.Fatalf("summariesComputed = %d, want 1", stats.summariesComputed)
	}
	if got := app.Store.List(context.Background(), summariesResource); len(got) != 1 || got[0].ID != "legacy-summary" {
		t.Fatalf("summaries = %#v, want existing legacy record updated in place", got)
	}
}

func TestGPUUsageCollectorSanitizesHighCardinalityProcessMetrics(t *testing.T) {
	app := newGPUCollectorTestApp()
	now := time.Date(2026, time.April, 5, 12, 0, 0, 0, time.UTC)
	seedGPUCollectorReadModels(t, app, "succeeded")
	rawMetrics, deniedValues := seedHighCardinalityGPUReadModel(t, app, now)

	stats := collectGPUUsageTelemetry(context.Background(), app, now)
	if stats.podRowsScanned != 4 || stats.snapshotsWritten != 4 || stats.snapshotsSkipped != 0 || stats.summariesComputed != 1 {
		t.Fatalf("collector stats = %#v, want scanned=4 written=4 skipped=0 summaries=1", stats)
	}
	for _, metrics := range rawMetrics {
		if textValue(metrics, "pid") == "" || textValue(metrics, "container_id") == "" {
			t.Fatalf("incoming metrics mutated by sanitizer: %#v", metrics)
		}
	}

	snapshots := app.Store.List(context.Background(), snapshotsResource)
	if len(snapshots) != 4 {
		t.Fatalf("snapshots = %#v, want 4 sanitized retained snapshots", snapshots)
	}
	for _, snapshot := range snapshots {
		assertNoHighCardinalityProcessEvidence(t, "snapshot", snapshot.Data, deniedValues)
		assertSanitizedSnapshotMetrics(t, mapValue(snapshot.Data, "metrics"))
	}

	summary, ok := summaryRecordForJob(context.Background(), app.Store, "JGPU")
	if !ok {
		t.Fatal("missing generated summary for terminal GPU job")
	}
	assertNoHighCardinalityProcessEvidence(t, "summary", summary.Data, deniedValues)
	summaryMetrics := mapValue(summary.Data, "metrics")
	if intValue(summaryMetrics, "sample_count") != 4 {
		t.Fatalf("summary metrics = %#v, want sample_count=4", summaryMetrics)
	}
	breakdowns := anySlice(summary.Data, "breakdowns")
	if len(breakdowns) != 1 {
		t.Fatalf("breakdowns = %#v, want one sanitized GPU breakdown", breakdowns)
	}
	for _, breakdown := range breakdowns {
		assertNoHighCardinalityProcessEvidence(t, "breakdown", breakdown, deniedValues)
	}
}

func TestGPUUsageCollectorRetentionDeletesOnlyOldSnapshots(t *testing.T) {
	app := newGPUCollectorTestApp()
	now := time.Date(2026, time.April, 5, 12, 0, 0, 0, time.UTC)
	createGPUCollectorRecord(t, app, snapshotsResource, map[string]any{
		"id":        "old",
		"job_id":    "J1",
		"timestamp": now.Add(-48 * time.Hour),
		"metrics":   map[string]any{"gpu_uuid": "old"},
	})
	createGPUCollectorRecord(t, app, snapshotsResource, map[string]any{
		"id":        "new",
		"job_id":    "J1",
		"timestamp": now.Add(-time.Hour),
		"metrics":   map[string]any{"gpu_uuid": "new"},
	})
	createGPUCollectorRecord(t, app, summariesResource, map[string]any{
		"id":          "summary",
		"job_id":      "J1",
		"computed_at": now.Add(-48 * time.Hour),
	})

	deleted := cleanupOldGPUSnapshots(context.Background(), app.Store, now.Add(-24*time.Hour))
	if deleted != 1 {
		t.Fatalf("deleted snapshots = %d, want 1", deleted)
	}
	if _, ok := app.Store.Get(context.Background(), snapshotsResource, "old"); ok {
		t.Fatal("old snapshot still exists")
	}
	if _, ok := app.Store.Get(context.Background(), snapshotsResource, "new"); !ok {
		t.Fatal("new snapshot was deleted")
	}
	if _, ok := app.Store.Get(context.Background(), summariesResource, "summary"); !ok {
		t.Fatal("summary should be retained")
	}
}

func TestGPUUsageCollectorSkipsUntimestampedRowsWithoutDuplicating(t *testing.T) {
	app := newGPUCollectorTestApp()
	now := time.Date(2026, time.April, 5, 12, 0, 0, 0, time.UTC)
	seedGPUCollectorReadModels(t, app, "running")
	createGPUCollectorRecord(t, app, clusterReadModelsResource, map[string]any{
		"id": "untimestamped",
		"summary": map[string]any{
			"podGpuUsages": []any{
				map[string]any{
					"job_id":             "JGPU",
					"podName":            "pod-a",
					"namespace":          "project-P1",
					"node":               "gpu-node-1",
					"gpuIndex":           0,
					"gpuUuid":            "GPU-a",
					"mpsVirtualUnits":    50,
					"memoryBytes":        4096,
					"gpuSMUtilization":   75,
					"gpuMemoryUsedBytes": 2048,
				},
			},
		},
	})

	first := collectGPUUsageTelemetry(context.Background(), app, now)
	second := collectGPUUsageTelemetry(context.Background(), app, now.Add(time.Minute))
	if first.snapshotsWritten != 0 || first.snapshotsSkipped != 1 || second.snapshotsWritten != 0 || second.snapshotsSkipped != 1 {
		t.Fatalf("stats first=%#v second=%#v, want untimestamped row skipped without writes", first, second)
	}
	if got := app.Store.List(context.Background(), snapshotsResource); len(got) != 0 {
		t.Fatalf("snapshots = %#v, want none for untimestamped row across repeated collection", got)
	}
}

func TestGPUUsageCollectorSkipsProjectPodCountRowsWithoutDeviceIdentity(t *testing.T) {
	app := newGPUCollectorTestApp()
	now := time.Date(2026, time.April, 5, 12, 0, 0, 0, time.UTC)
	seedGPUCollectorReadModels(t, app, "running")
	createGPUCollectorRecord(t, app, clusterReadModelsResource, map[string]any{
		"id": "cluster",
		"summary": map[string]any{
			"podGpuUsages": []any{
				map[string]any{
					"job_id":        "JGPU",
					"project_id":    "P1",
					"user_id":       "U1",
					"namespace":     "proj-p1-alice",
					"pod_name":      "pod-count-row",
					"requested_gpu": 1.0,
					"timestamp":     now,
					"phase":         "Running",
					"active":        true,
				},
			},
		},
	})

	stats := collectGPUUsageTelemetry(context.Background(), app, now)
	if stats.podRowsScanned != 1 || stats.snapshotsWritten != 0 || stats.snapshotsSkipped != 1 || stats.summariesComputed != 0 {
		t.Fatalf("collector stats = %#v, want pod-count row scanned and skipped without snapshots", stats)
	}
	if got := app.Store.List(context.Background(), snapshotsResource); len(got) != 0 {
		t.Fatalf("snapshots = %#v, want none for project pod-count rows", got)
	}
}

func TestGPUUsageCollectorRegisteredMaintenanceTask(t *testing.T) {
	app := newGPUCollectorTestApp()
	Register(app)
	if got := app.MaintenanceTaskNames(); len(got) != 1 || got[0] != gpuCollectorTaskName {
		t.Fatalf("maintenance tasks = %v, want only %s", got, gpuCollectorTaskName)
	}
	now := time.Now().UTC()
	seedGPUCollectorReadModels(t, app, "succeeded")
	seedGPUCollectorClusterReadModel(t, app, now.Add(-time.Minute), now)
	app.RunMaintenanceOnce(context.Background(), time.Minute)
	if got := app.Store.List(context.Background(), snapshotsResource); len(got) != 2 {
		t.Fatalf("snapshots after maintenance = %#v, want 2", got)
	}
}

func newGPUCollectorTestApp() *platform.App {
	return platform.NewApp(platform.Config{
		ServiceName:               serviceName,
		HTTPAddr:                  ":0",
		GPUUsageRetentionDays:     30,
		GPUUsageSnapshotWindowMin: 10,
	})
}

func seedGPUCollectorReadModels(t *testing.T, app *platform.App, status string) {
	t.Helper()
	createGPUCollectorRecord(t, app, gpuJobsResource, map[string]any{
		"id":         "JGPU",
		"job_id":     "JGPU",
		"user_id":    "U1",
		"project_id": "P1",
		"queue_name": "gpuq",
		"status":     status,
	})
	createGPUCollectorRecord(t, app, gpuIdentityUsersResource, map[string]any{
		"id":       "U1",
		"user_id":  "U1",
		"username": "alice",
	})
	createGPUCollectorRecord(t, app, gpuIdentityUsersResource, map[string]any{
		"id":           "ADMIN",
		"user_id":      "ADMIN",
		"username":     "admin",
		"capabilities": map[string]any{"adminPanel": true},
	})
	createGPUCollectorRecord(t, app, gpuProjectsResource, map[string]any{
		"id":           "P1",
		"p_id":         "P1",
		"project_name": "vision",
	})
}

func seedGPUCollectorClusterReadModel(t *testing.T, app *platform.App, first, second time.Time) {
	t.Helper()
	createGPUCollectorRecord(t, app, clusterReadModelsResource, map[string]any{
		"id": "cluster",
		"summary": map[string]any{
			"podGpuUsages": []any{
				map[string]any{
					"job_id":             "JGPU",
					"podName":            "pod-a",
					"namespace":          "project-P1",
					"node":               "gpu-node-1",
					"gpuIndex":           0,
					"gpuUuid":            "GPU-a",
					"mpsVirtualUnits":    50,
					"timestamp":          first,
					"memoryBytes":        4096,
					"gpuSMUtilization":   75,
					"gpuMemoryUsedBytes": 2048,
					"cpuUsageCores":      2,
					"memoryUsageBytes":   1024 * 1024 * 1024,
				},
				map[string]any{
					"job_id":             "JGPU",
					"podName":            "pod-a",
					"namespace":          "project-P1",
					"node":               "gpu-node-1",
					"gpuIndex":           0,
					"gpuUuid":            "GPU-a",
					"mpsVirtualUnits":    50,
					"timestamp":          second,
					"memoryBytes":        4096,
					"gpuSMUtilization":   85,
					"gpuMemoryUsedBytes": 3072,
					"cpuUsageCores":      2,
					"memoryUsageBytes":   1024 * 1024 * 1024,
				},
				map[string]any{"podName": "incomplete", "namespace": "project-P1", "gpuIndex": 0},
			},
		},
	})
}

func seedHighCardinalityGPUReadModel(t *testing.T, app *platform.App, first time.Time) ([]map[string]any, []string) {
	t.Helper()
	rawMetrics := make([]map[string]any, 0, 4)
	deniedValues := []string{}
	rows := make([]any, 0, 4)
	for sample := range 4 {
		metrics := highCardinalityProcessMetrics(sample)
		row := map[string]any{
			"job_id":                  "JGPU",
			"podName":                 "pod-cardinality",
			"namespace":               "project-P1",
			"node":                    "gpu-node-1",
			"gpuIndex":                0,
			"gpuUuid":                 "GPU-cardinality",
			"mpsVirtualUnits":         100,
			"timestamp":               first.Add(time.Duration(sample) * time.Minute),
			"memoryBytes":             int64(8192),
			"gpuSMUtilization":        50.0 + float64(sample),
			"gpuMemoryUsedBytes":      int64(4096 + sample),
			"gpuMemUtilization":       60.0 + float64(sample),
			"gpuMemoryUsedSource":     "dcgm-rollup",
			"cpuUsageCores":           1.5,
			"memoryUsageBytes":        int64(2 * 1024 * 1024 * 1024),
			"metrics":                 metrics,
			"process_id":              fmt.Sprintf("top-process-id-%d", sample),
			"processName":             fmt.Sprintf("top-process-name-%d", sample),
			"containerID":             fmt.Sprintf("top-container-id-%d", sample),
			"podUID":                  fmt.Sprintf("top-pod-uid-%d", sample),
			"process_start_time":      fmt.Sprintf("top-process-start-%d", sample),
			"process_sample_count":    100 + sample,
			"orphan_process_count":    sample,
			"unknown_container_count": sample + 1,
		}
		rawMetrics = append(rawMetrics, metrics)
		deniedValues = append(deniedValues, deniedProcessMetricValues(metrics)...)
		deniedValues = append(deniedValues, deniedProcessMetricValues(row)...)
		rows = append(rows, row)
	}
	createGPUCollectorRecord(t, app, clusterReadModelsResource, map[string]any{
		"id": "process-cardinality",
		"summary": map[string]any{
			"podGpuUsages": rows,
		},
	})
	return rawMetrics, deniedValues
}

func highCardinalityProcessMetrics(sample int) map[string]any {
	return map[string]any{
		"pid":                     fmt.Sprintf("pid-%d", sample),
		"process_id":              fmt.Sprintf("process-id-%d", sample),
		"processId":               fmt.Sprintf("processId-%d", sample),
		"processID":               fmt.Sprintf("processID-%d", sample),
		"process_name":            fmt.Sprintf("process-name-%d", sample),
		"processName":             fmt.Sprintf("processName-%d", sample),
		"command":                 fmt.Sprintf("command-%d", sample),
		"cmd":                     fmt.Sprintf("cmd-%d", sample),
		"cmdline":                 fmt.Sprintf("cmdline-%d", sample),
		"args":                    fmt.Sprintf("args-%d", sample),
		"argv":                    fmt.Sprintf("argv-%d", sample),
		"container_id":            fmt.Sprintf("container-id-%d", sample),
		"containerId":             fmt.Sprintf("containerId-%d", sample),
		"containerID":             fmt.Sprintf("containerID-%d", sample),
		"pod_uid":                 fmt.Sprintf("pod-uid-%d", sample),
		"podUID":                  fmt.Sprintf("podUID-%d", sample),
		"podUid":                  fmt.Sprintf("podUid-%d", sample),
		"cgroup":                  fmt.Sprintf("cgroup-%d", sample),
		"cgroup_path":             fmt.Sprintf("cgroup-path-%d", sample),
		"process_start_time":      fmt.Sprintf("process-start-time-%d", sample),
		"processStartTime":        fmt.Sprintf("processStartTime-%d", sample),
		"process_sample_count":    12 + sample,
		"orphan_process_count":    2 + sample,
		"unknown_container_count": 3 + sample,
		"gpu_utilization":         0.75,
		"gpu_sm_util_source":      "dcgm-rollup",
	}
}

func deniedProcessMetricValues(metrics map[string]any) []string {
	values := []string{}
	for key, value := range metrics {
		if highCardinalityProcessMetricKey(key) {
			values = append(values, fmt.Sprint(value))
		}
	}
	return values
}

func assertSanitizedSnapshotMetrics(t *testing.T, metrics map[string]any) {
	t.Helper()
	if metrics["gpu_uuid"] != "GPU-cardinality" || int64Value(metrics, "gpu_memory_bytes") != 8192 {
		t.Fatalf("snapshot metrics = %#v, want retained normalized GPU identity and memory", metrics)
	}
	if floatValue(metrics, "gpu_sm_utilization") == 0 || textValue(metrics, "gpu_memory_used_source") != "dcgm-rollup" {
		t.Fatalf("snapshot metrics = %#v, want retained GPU utilization/source metrics", metrics)
	}
	if intValue(metrics, "process_sample_count") == 0 || intValue(metrics, "orphan_process_count") == 0 || intValue(metrics, "unknown_container_count") == 0 {
		t.Fatalf("snapshot metrics = %#v, want retained bounded aggregate process counts", metrics)
	}
}

func assertNoHighCardinalityProcessEvidence(t *testing.T, scope string, value any, deniedValues []string) {
	t.Helper()
	switch typed := value.(type) {
	case map[string]any:
		for key, item := range typed {
			if highCardinalityProcessMetricKey(key) {
				t.Fatalf("%s retained denied key %q in %#v", scope, key, typed)
			}
			assertNoHighCardinalityProcessEvidence(t, scope+"."+key, item, deniedValues)
		}
	case []any:
		for index, item := range typed {
			assertNoHighCardinalityProcessEvidence(t, fmt.Sprintf("%s[%d]", scope, index), item, deniedValues)
		}
	case string:
		for _, denied := range deniedValues {
			if strings.Contains(typed, denied) {
				t.Fatalf("%s retained denied value %q in %q", scope, denied, typed)
			}
		}
	}
}

func createGPUCollectorRecord(t *testing.T, app *platform.App, resource string, data map[string]any) {
	t.Helper()
	if _, err := app.Store.Create(context.Background(), resource, data); err != nil {
		t.Fatal(err)
	}
}
