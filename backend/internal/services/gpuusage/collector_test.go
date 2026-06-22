package gpuusage

import (
	"context"
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

func createGPUCollectorRecord(t *testing.T, app *platform.App, resource string, data map[string]any) {
	t.Helper()
	if _, err := app.Store.Create(context.Background(), resource, data); err != nil {
		t.Fatal(err)
	}
}
