package gpuusage

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

func TestGPUUsageSummaryHandlers(t *testing.T) {
	app := newGPUUsageTestApp(t)

	code, data, _ := getMyUsage(app, gpuRequest("/api/v1/me/usage?since=2026-04-01", "U1"), platform.RouteSpec{})
	if code != http.StatusOK {
		t.Fatalf("my usage status=%d data=%#v, want 200", code, data)
	}
	myRows := data.([]UserResourceUsage)
	if len(myRows) != 1 || myRows[0].UserID != "U1" || myRows[0].ProjectName != "vision" {
		t.Fatalf("my usage rows = %#v, want U1 current joined usage", myRows)
	}
	if myRows[0].GPUHours != 2 || myRows[0].CPUHours != 1 || myRows[0].MemoryGBHours != 1 {
		t.Fatalf("my usage totals = %#v, want seconds converted to hours", myRows[0])
	}

	code, data, _ = listAdminUsage(app, gpuRequest("/api/v1/admin/usage?since=2026-04-01", "ADMIN"), platform.RouteSpec{})
	if code != http.StatusOK {
		t.Fatalf("admin usage status=%d data=%#v, want 200", code, data)
	}
	adminRows := data.([]UserResourceUsage)
	if len(adminRows) != 2 || adminRows[0].JobID != "J2" || adminRows[1].JobID != "J1" {
		t.Fatalf("admin usage rows = %#v, want computed_at descending", adminRows)
	}
}

func TestGPUUsageJobAndMappingHandlers(t *testing.T) {
	app := newGPUUsageTestApp(t)

	code, data, _ := getMyGPUJobs(app, gpuRequest("/api/v1/me/gpu/jobs", "U1"), platform.RouteSpec{})
	if code != http.StatusOK {
		t.Fatalf("my gpu jobs status=%d data=%#v, want 200", code, data)
	}
	jobs := data.([]UserMPSJob)
	if len(jobs) != 1 || jobs[0].JobID != "J1" || jobs[0].MPSVirtualUnits != 2 {
		t.Fatalf("my gpu jobs = %#v, want latest U1 active job", jobs)
	}

	code, data, _ = listAdminGPUUsers(app, gpuRequest("/api/v1/admin/gpu/users", "ADMIN"), platform.RouteSpec{})
	if code != http.StatusOK {
		t.Fatalf("admin gpu users status=%d data=%#v, want 200", code, data)
	}
	users := data.([]AdminGPUUserSummary)
	if len(users) != 2 || users[0].UserID != "U2" || users[0].TotalMPSVirtualUnits != 4 {
		t.Fatalf("admin gpu users = %#v, want active users by MPS units", users)
	}

	code, data, _ = listClusterMPSMapping(app, gpuRequest("/api/v1/cluster/mps-mapping", "U1"), platform.RouteSpec{})
	if code != http.StatusOK {
		t.Fatalf("cluster MPS status=%d data=%#v, want 200", code, data)
	}
	mapping := data.([]MPSGPUSlot)
	if len(mapping) != 2 || mapping[0].Node != "gpu-node-1" || mapping[0].SMUtilization != 50 {
		t.Fatalf("MPS mapping = %#v, want active slots with GPU metrics", mapping)
	}
}

func TestGPUUsageAdminHistoryAndJobs(t *testing.T) {
	app := newGPUUsageTestApp(t)

	code, data, _ := listAdminGPUUsersHistory(app, gpuRequest("/api/v1/admin/gpu/users/history?since=2026-04-01", "ADMIN"), platform.RouteSpec{})
	if code != http.StatusOK {
		t.Fatalf("admin gpu history status=%d data=%#v, want 200", code, data)
	}
	history := data.([]AdminGPUUserHistory)
	if len(history) != 2 || history[0].UserID != "U1" || history[0].TotalGPUHours != 2 || history[0].TotalJobs != 1 {
		t.Fatalf("admin gpu history = %#v, want U1 first with summarized job", history)
	}

	req := gpuRequest("/api/v1/admin/gpu/users/U1/jobs?since=2026-04-01", "ADMIN")
	req.SetPathValue("userId", "U1")
	code, data, _ = getAdminGPUUserJobs(app, req, platform.RouteSpec{})
	if code != http.StatusOK {
		t.Fatalf("admin gpu user jobs status=%d data=%#v, want 200", code, data)
	}
	jobs := data.([]AdminGPUUserJob)
	if len(jobs) != 1 || jobs[0].JobID != "J1" || jobs[0].QueueName != "gpuq" {
		t.Fatalf("admin gpu user jobs = %#v, want summarized U1 job", jobs)
	}
}

func TestGPUUsageAuthGuards(t *testing.T) {
	app := newGPUUsageTestApp(t)
	guards := []struct {
		name string
		call func() (int, any, *platform.Degraded)
		want int
	}{
		{name: "my usage anonymous", call: func() (int, any, *platform.Degraded) {
			return getMyUsage(app, gpuRequest("/api/v1/me/usage", ""), platform.RouteSpec{})
		}, want: http.StatusUnauthorized},
		{name: "admin usage non-admin", call: func() (int, any, *platform.Degraded) {
			return listAdminUsage(app, gpuRequest("/api/v1/admin/usage", "U1"), platform.RouteSpec{})
		}, want: http.StatusForbidden},
		{name: "my jobs anonymous", call: func() (int, any, *platform.Degraded) {
			return getMyGPUJobs(app, gpuRequest("/api/v1/me/gpu/jobs", ""), platform.RouteSpec{})
		}, want: http.StatusUnauthorized},
		{name: "admin users non-admin", call: func() (int, any, *platform.Degraded) {
			return listAdminGPUUsers(app, gpuRequest("/api/v1/admin/gpu/users", "U1"), platform.RouteSpec{})
		}, want: http.StatusForbidden},
		{name: "admin history anonymous", call: func() (int, any, *platform.Degraded) {
			return listAdminGPUUsersHistory(app, gpuRequest("/api/v1/admin/gpu/users/history", ""), platform.RouteSpec{})
		}, want: http.StatusUnauthorized},
		{name: "admin jobs missing target", call: func() (int, any, *platform.Degraded) {
			return getAdminGPUUserJobs(app, gpuRequest("/api/v1/admin/gpu/users//jobs", "ADMIN"), platform.RouteSpec{})
		}, want: http.StatusBadRequest},
		{name: "cluster mapping anonymous", call: func() (int, any, *platform.Degraded) {
			return listClusterMPSMapping(app, gpuRequest("/api/v1/cluster/mps-mapping", ""), platform.RouteSpec{})
		}, want: http.StatusUnauthorized},
		{name: "admin mapping non-admin", call: func() (int, any, *platform.Degraded) {
			return listAdminMPSMapping(app, gpuRequest("/api/v1/admin/mps-mapping", "U1"), platform.RouteSpec{})
		}, want: http.StatusForbidden},
	}
	for _, guard := range guards {
		t.Run(guard.name, func(t *testing.T) {
			code, data, _ := guard.call()
			if code != guard.want {
				t.Fatalf("status=%d data=%#v, want %d", code, data, guard.want)
			}
		})
	}
}

func TestGPUProjectionLifecycle(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: serviceName, HTTPAddr: ":0"})
	req := gpuRequest("/", "ADMIN")

	projectGPUUsageEvent(app, req, gpuEvent("UserCreated", map[string]any{"new": map[string]any{"id": "U1", "username": "alice"}}))
	projectGPUUsageEvent(app, req, gpuEvent("ProjectCreated", map[string]any{"record": map[string]any{"p_id": "P1", "project_name": "vision"}}))
	projectGPUUsageEvent(app, req, gpuEvent("JobRunning", map[string]any{"job": map[string]any{"id": "J1", "user_id": "U1", "project_id": "P1"}}))
	projectGPUUsageEvent(app, req, gpuEvent("ProxyPolicyChanged", map[string]any{"action": "role_create", "role_id": "R1", "admin_panel": true}))

	if _, ok := app.Store.Get(context.Background(), gpuIdentityUsersResource, "U1"); !ok {
		t.Fatal("projected user was not created")
	}
	job, ok := app.Store.Get(context.Background(), gpuJobsResource, "J1")
	if !ok || job.Data["status"] != "running" {
		t.Fatalf("projected job = %#v, want running job", job)
	}
	if _, ok := app.Store.Get(context.Background(), gpuAuthorizationRolesResource, "R1"); !ok {
		t.Fatal("projected authorization role was not created")
	}

	projectGPUUsageEvent(app, req, gpuEvent("UserDeleted", map[string]any{"id": "U1", "deleted": true}))
	if _, ok := app.Store.Get(context.Background(), gpuIdentityUsersResource, "U1"); ok {
		t.Fatal("deleted user read model still exists")
	}
	projectGPUUsageEvent(app, req, gpuEvent("Unknown", map[string]any{"id": "noop"}))
	if resource, _, _, ok := gpuProjection(gpuEvent("Unknown", nil)); ok || resource != "" {
		t.Fatalf("unknown projection resource=%q ok=%v, want ignored", resource, ok)
	}
}

func TestGPUHelperConversionsAndBounds(t *testing.T) {
	if got := snapshotWindowMinutes(platform.Config{GPUUsageSnapshotWindowMin: 0}); got != 10 {
		t.Fatalf("snapshot window default = %d, want 10", got)
	}
	if got := snapshotWindowMinutes(platform.Config{GPUUsageSnapshotWindowMin: -1}); got != 1 {
		t.Fatalf("snapshot window lower bound = %d, want 1", got)
	}
	if got := snapshotWindowMinutes(platform.Config{GPUUsageSnapshotWindowMin: 2000}); got != 1440 {
		t.Fatalf("snapshot window upper bound = %d, want 1440", got)
	}

	payload := map[string]any{
		"text":       json.Number("42"),
		"float":      json.Number("3.5"),
		"int":        "7",
		"bool":       "true",
		"map_string": `{"nested":true}`,
		"time":       "2026-04-05 06:07:08",
	}
	if textValue(payload, "text") != "42" || floatValue(payload, "float") != 3.5 || intValue(payload, "int") != 7 {
		t.Fatalf("numeric conversions failed for %#v", payload)
	}
	if !boolValue(payload, "bool") || !mapValue(payload, "map_string")["nested"].(bool) {
		t.Fatalf("bool/map conversions failed for %#v", payload)
	}
	if timePointerValue(payload, nil, "time") == nil || !derefTime(nil).IsZero() {
		t.Fatalf("time conversions failed for %#v", payload)
	}
	if firstNonEmpty(" ", "value") != "value" || intKey(3) != "3" {
		t.Fatal("basic helper conversions failed")
	}
}

func newGPUUsageTestApp(t *testing.T) *platform.App {
	t.Helper()
	app := platform.NewApp(platform.Config{ServiceName: "all", HTTPAddr: ":0"})
	Register(app)
	seedGPUUsageRecords(t, app)
	return app
}

func seedGPUUsageRecords(t *testing.T, app *platform.App) {
	t.Helper()
	april2 := time.Date(2026, time.April, 2, 8, 0, 0, 0, time.UTC)
	april3 := time.Date(2026, time.April, 3, 9, 0, 0, 0, time.UTC)
	createGPURecords(t, app, identityUsersResource, []map[string]any{
		{"id": "U1", "username": "alice", "role": "user"},
		{"id": "U2", "username": "bob", "role": "user"},
		{"id": "ADMIN", "username": "admin", "capabilities": map[string]any{"adminPanel": true}},
	})
	createGPURecords(t, app, orgProjectsResource, []map[string]any{
		{"id": "P1", "p_id": "P1", "project_name": "vision"},
		{"id": "P2", "p_id": "P2", "project_name": "language"},
	})
	createGPURecords(t, app, workloadJobsResource, []map[string]any{
		{"id": "J1", "user_id": "U1", "project_id": "P1", "queue_name": "gpuq", "status": "running"},
		{"id": "J2", "user_id": "U2", "project_id": "P2", "queue_name": "gpuq", "status": "running"},
	})
	createGPURecords(t, app, summariesResource, []map[string]any{
		{"id": "S1", "job_id": "J1", "computed_at": april2, "metrics": map[string]any{
			"total_gpu_seconds":       7200.0,
			"total_cpu_seconds":       3600.0,
			"total_memory_seconds_mb": 1024.0 * 3600.0,
			"first_sample_at":         april2,
			"last_sample_at":          april2.Add(time.Hour),
		}},
		{"id": "S2", "job_id": "J2", "computed_at": april3, "metrics": map[string]any{
			"total_gpu_seconds":       3600.0,
			"total_cpu_seconds":       1800.0,
			"total_memory_seconds_mb": 512.0 * 3600.0,
		}},
	})
	now := time.Now().UTC()
	createGPURecords(t, app, snapshotsResource, []map[string]any{
		{"id": "SN1", "job_id": "J1", "pod_name": "pod-a", "pod_namespace": "ns-a", "node": "gpu-node-1", "gpu_index": 0, "mps_physical_gpu_index": 0, "mps_virtual_units": 2, "timestamp": now.Add(-2 * time.Minute), "metrics": map[string]any{
			"gpu_uuid":              "GPU-a",
			"gpu_memory_bytes":      2048.0,
			"gpu_sm_utilization":    50.0,
			"gpu_memory_used_bytes": 1024.0,
		}},
		{"id": "SN2", "job_id": "J2", "pod_name": "pod-b", "pod_namespace": "ns-b", "node": "gpu-node-1", "gpu_index": 1, "mps_physical_gpu_index": 1, "mps_virtual_units": 4, "timestamp": now.Add(-1 * time.Minute), "metrics": map[string]any{
			"gpu_uuid":         "GPU-b",
			"gpu_memory_bytes": 4096.0,
		}},
		{"id": "SNOLD", "job_id": "J1", "pod_name": "old", "node": "gpu-node-1", "gpu_index": 0, "mps_virtual_units": 1, "timestamp": now.Add(-30 * time.Minute), "metrics": map[string]any{
			"gpu_uuid": "GPU-old",
		}},
	})
}

func createGPURecords(t *testing.T, app *platform.App, resource string, rows []map[string]any) {
	t.Helper()
	for _, row := range rows {
		if _, err := app.Store.Create(context.Background(), resource, row); err != nil {
			t.Fatal(err)
		}
	}
}

func gpuRequest(target, userID string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, target, nil)
	if userID != "" {
		req.Header.Set("X-User-ID", userID)
	}
	return req
}

func gpuEvent(name string, data map[string]any) contracts.Event {
	return contracts.Event{Name: name, Data: data}
}
