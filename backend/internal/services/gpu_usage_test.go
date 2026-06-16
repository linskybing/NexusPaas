package services

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

const (
	gpuTestAdminID               = "ADMIN"
	gpuTestIdentityUsersResource = "identity-service:users"
	gpuTestIdentitySource        = "identity-service"
	gpuTestJobID                 = "J1"
	gpuTestProjectsResource      = "org-project-service:projects"
	gpuTestProjectID             = "P1"
	gpuTestProjectSource         = "org-project-service"
	gpuTestProjectedJobsResource = "usage-observability-service:gpu_jobs"
	gpuTestJobsResource          = "workload-service:jobs"
	gpuTestQueueName             = "gpuq"
	gpuTestRoleID                = "platform-admin"
	gpuTestSnapshotsResource     = "usage-observability-service:job_gpu_usage_snapshots"
	gpuTestSummariesResource     = "usage-observability-service:job_gpu_usage_summaries"
	gpuTestUsageService          = "usage-observability-service"
	gpuTestUserID                = "U1"
	gpuTestWorkloadSource        = "workload-service"
)

func TestGPUUsageWorkflow(t *testing.T) {
	app := newTestApp()
	seedGPUUsage(t, app)

	forgedAdmin := forgedGPUAdminHeaders()
	assertMyGPUUsageWorkflow(t, app)
	assertAdminGPUUsageWorkflow(t, app, forgedAdmin)
	assertGPUJobWorkflow(t, app, forgedAdmin)
	assertMPSMappingWorkflow(t, app, forgedAdmin)
}

func forgedGPUAdminHeaders() map[string]string {
	forgedAdmin := userHeaders("U1")
	forgedAdmin["X-User-Role"] = "admin"
	return forgedAdmin
}

func assertMyGPUUsageWorkflow(t *testing.T, app *platform.App) {
	t.Helper()

	requestJSON(t, app, http.MethodGet, "/api/v1/me/usage", "", nil, http.StatusUnauthorized)
	myUsage := responseSlice(t, requestJSON(t, app, http.MethodGet, "/api/v1/me/usage?since=2026-04-01", "", userHeaders("U1"), http.StatusOK))
	if len(myUsage) != 1 {
		t.Fatalf("my usage = %#v, want one current U1 row", myUsage)
	}
	myRow := myUsage[0].(map[string]any)
	if myRow["UserID"] != "U1" || myRow["Username"] != "alice" || myRow["ProjectName"] != "vision" {
		t.Fatalf("my usage row = %#v, want joined U1/project data", myRow)
	}
	if myRow["GPUHours"] != float64(2) || myRow["CPUHours"] != float64(1) || myRow["MemoryGBHours"] != float64(1) {
		t.Fatalf("my usage row = %#v, want seconds converted to hours", myRow)
	}

	oldFiltered := responseSlice(t, requestJSON(t, app, http.MethodGet, "/api/v1/me/usage?since=2026-05-01", "", userHeaders("U1"), http.StatusOK))
	if len(oldFiltered) != 0 {
		t.Fatalf("usage since 2026-05-01 = %#v, want no rows", oldFiltered)
	}
}

func assertAdminGPUUsageWorkflow(t *testing.T, app *platform.App, forgedAdmin map[string]string) {
	t.Helper()

	requestJSON(t, app, http.MethodGet, "/api/v1/admin/usage?since=2026-04-01", "", forgedAdmin, http.StatusForbidden)
	requestJSON(t, newTestApp(), http.MethodGet, "/api/v1/admin/usage?since=2026-04-01", "", forgedAdmin, http.StatusForbidden)

	adminUsage := responseSlice(t, requestJSON(t, app, http.MethodGet, "/api/v1/admin/usage?since=2026-04-01", "", adminHeaders("ADMIN"), http.StatusOK))
	if len(adminUsage) != 2 {
		t.Fatalf("admin usage = %#v, want current rows for both users", adminUsage)
	}
	first := adminUsage[0].(map[string]any)
	if first["JobID"] != "J2" || first["UserID"] != "U2" {
		t.Fatalf("admin usage order = %#v, want computed_at descending", adminUsage)
	}
}

func assertGPUJobWorkflow(t *testing.T, app *platform.App, forgedAdmin map[string]string) {
	t.Helper()

	myJobs := responseSlice(t, requestJSON(t, app, http.MethodGet, "/api/v1/me/gpu/jobs", "", userHeaders("U1"), http.StatusOK))
	if len(myJobs) != 1 || myJobs[0].(map[string]any)["job_id"] != "J1" {
		t.Fatalf("my gpu jobs = %#v, want one active U1 job", myJobs)
	}
	myJob := myJobs[0].(map[string]any)
	if myJob["project_name"] != "vision" || myJob["mps_virtual_units"] != float64(2) || myJob["gpu_memory_bytes"] != float64(2048) {
		t.Fatalf("my gpu job = %#v, want project and latest MPS allocation", myJob)
	}

	requestJSON(t, app, http.MethodGet, "/api/v1/admin/gpu/users", "", forgedAdmin, http.StatusForbidden)
	requestJSON(t, app, http.MethodGet, "/api/v1/admin/gpu/users/history", "", forgedAdmin, http.StatusForbidden)
	requestJSON(t, app, http.MethodGet, "/api/v1/admin/gpu/users/U1/jobs", "", forgedAdmin, http.StatusForbidden)
	gpuUsers := responseSlice(t, requestJSON(t, app, http.MethodGet, "/api/v1/admin/gpu/users", "", adminHeaders("ADMIN"), http.StatusOK))
	if len(gpuUsers) != 2 || gpuUsers[0].(map[string]any)["user_id"] != "U2" {
		t.Fatalf("admin gpu users = %#v, want active users ordered by MPS units", gpuUsers)
	}
	history := responseSlice(t, requestJSON(t, app, http.MethodGet, "/api/v1/admin/gpu/users/history?since=2026-04-01", "", adminHeaders("ADMIN"), http.StatusOK))
	if len(history) != 2 || history[0].(map[string]any)["total_gpu_hours"] != float64(2) {
		t.Fatalf("admin gpu user history = %#v, want summary GPU hours", history)
	}
	if history[0].(map[string]any)["total_jobs"] != float64(1) || history[0].(map[string]any)["last_job_at"] == nil {
		t.Fatalf("admin gpu user history = %#v, want job count and last job timestamp", history)
	}
	userJobs := responseSlice(t, requestJSON(t, app, http.MethodGet, "/api/v1/admin/gpu/users/U1/jobs?since=2026-04-01", "", adminHeaders("ADMIN"), http.StatusOK))
	if len(userJobs) != 1 || userJobs[0].(map[string]any)["job_id"] != "J1" {
		t.Fatalf("admin gpu user jobs = %#v, want U1 summarized job", userJobs)
	}
	if userJobs[0].(map[string]any)["queue_name"] != "gpuq" || userJobs[0].(map[string]any)["total_gpu_hours"] != float64(2) {
		t.Fatalf("admin gpu user job = %#v, want queue and summarized GPU hours", userJobs[0])
	}
}

func assertMPSMappingWorkflow(t *testing.T, app *platform.App, forgedAdmin map[string]string) {
	t.Helper()

	requestJSON(t, app, http.MethodGet, "/api/v1/cluster/mps-mapping", "", nil, http.StatusUnauthorized)
	mpsMapping := responseSlice(t, requestJSON(t, app, http.MethodGet, "/api/v1/cluster/mps-mapping", "", userHeaders("U1"), http.StatusOK))
	if len(mpsMapping) != 2 || mpsMapping[0].(map[string]any)["Node"] != "gpu-node-1" {
		t.Fatalf("cluster mps mapping = %#v, want active MPS slots", mpsMapping)
	}
	if mpsMapping[0].(map[string]any)["SMUtilization"] != float64(50) || mpsMapping[0].(map[string]any)["GPUMemoryBytes"] != float64(2048) {
		t.Fatalf("cluster mps mapping first row = %#v, want GPU metrics carried through", mpsMapping[0])
	}
	requestJSON(t, app, http.MethodGet, "/api/v1/admin/mps-mapping", "", forgedAdmin, http.StatusForbidden)
	adminMPS := responseSlice(t, requestJSON(t, app, http.MethodGet, "/api/v1/admin/mps-mapping", "", adminHeaders("ADMIN"), http.StatusOK))
	if len(adminMPS) != 2 {
		t.Fatalf("admin mps mapping = %#v, want active MPS slots", adminMPS)
	}
}

func TestGPUUsageUsesEventFedReadModelsInIsolatedService(t *testing.T) {
	app := platform.NewApp(platform.Config{
		ServiceName:  gpuTestUsageService,
		HTTPAddr:     ":0",
		APIKeys:      map[string]bool{"test-key": true},
		ExternalURLs: map[string]string{},
	})
	RegisterAll(app)
	if err := app.ValidateServiceIsolation(); err != nil {
		t.Fatalf("usage observability isolation = %v, want event-fed read models", err)
	}

	publishDashboardTestEvent(t, app, "UserCreated", gpuTestIdentitySource, map[string]any{
		"id":       gpuTestUserID,
		"username": "alice",
	})
	publishDashboardTestEvent(t, app, "UserCreated", gpuTestIdentitySource, map[string]any{
		"id":       gpuTestAdminID,
		"username": "admin",
		"role_id":  gpuTestRoleID,
	})
	publishDashboardTestEvent(t, app, "roleCreated", gpuTestIdentitySource, map[string]any{
		"id":           gpuTestRoleID,
		"name":         gpuTestRoleID,
		"capabilities": map[string]any{"adminPanel": true},
	})
	publishDashboardTestEvent(t, app, "ProjectCreated", gpuTestProjectSource, map[string]any{
		"id":           gpuTestProjectID,
		"p_id":         gpuTestProjectID,
		"project_name": "vision",
	})
	publishDashboardTestEvent(t, app, "JobRunning", gpuTestWorkloadSource, map[string]any{
		"id":         gpuTestJobID,
		"user_id":    gpuTestUserID,
		"project_id": gpuTestProjectID,
		"queue_name": gpuTestQueueName,
	})

	sampledAt := time.Date(2026, time.April, 2, 8, 0, 0, 0, time.UTC)
	createRows(t, app, gpuTestSummariesResource, []map[string]any{
		{"id": "S1", "job_id": gpuTestJobID, "computed_at": sampledAt, "metrics": map[string]any{
			"total_gpu_seconds": 7200.0,
			"first_sample_at":   sampledAt,
			"last_sample_at":    sampledAt.Add(time.Hour),
		}},
	})
	createRows(t, app, gpuTestSnapshotsResource, []map[string]any{
		{"id": "SN1", "job_id": gpuTestJobID, "pod_name": "pod-a", "pod_namespace": "ns-a", "node": "gpu-node-1", "gpu_index": 0, "mps_physical_gpu_index": 0, "mps_virtual_units": 2, "timestamp": time.Now().UTC(), "metrics": map[string]any{
			"gpu_uuid":         "GPU-a",
			"gpu_memory_bytes": 2048.0,
		}},
	})

	myUsage := responseSlice(t, requestJSON(t, app, http.MethodGet, "/api/v1/me/usage?since=2026-04-01", "", userHeaders(gpuTestUserID), http.StatusOK))
	if len(myUsage) != 1 {
		t.Fatalf("my usage = %#v, want one projected row", myUsage)
	}
	if row := myUsage[0].(map[string]any); row["Username"] != "alice" || row["ProjectName"] != "vision" {
		t.Fatalf("projected usage row = %#v, want user and project names from local read models", row)
	}

	adminJobs := responseSlice(t, requestJSON(t, app, http.MethodGet, "/api/v1/admin/gpu/users/"+gpuTestUserID+"/jobs?since=2026-04-01", "", adminHeaders(gpuTestAdminID), http.StatusOK))
	if len(adminJobs) != 1 {
		t.Fatalf("admin projected jobs = %#v, want one row", adminJobs)
	}
	if job := adminJobs[0].(map[string]any); job["queue_name"] != gpuTestQueueName {
		t.Fatalf("admin projected job = %#v, want queue from local job read model", job)
	}

	if len(app.Store.List(context.Background(), gpuTestJobsResource)) != 0 {
		t.Fatal("isolated usage test should not seed source workload jobs")
	}
	if _, ok := app.Store.Get(context.Background(), gpuTestProjectedJobsResource, gpuTestJobID); !ok {
		t.Fatal("missing local projected GPU job read model")
	}
}

func seedGPUUsage(t *testing.T, app *platform.App) {
	t.Helper()
	april2 := time.Date(2026, time.April, 2, 8, 0, 0, 0, time.UTC)
	april3 := time.Date(2026, time.April, 3, 9, 0, 0, 0, time.UTC)
	march := time.Date(2026, time.March, 20, 8, 0, 0, 0, time.UTC)
	createRows(t, app, gpuTestIdentityUsersResource, []map[string]any{
		{"id": "U1", "username": "alice", "role": "user", "capabilities": map[string]any{"adminPanel": false}},
		{"id": "U2", "username": "bob", "role": "user", "capabilities": map[string]any{"adminPanel": false}},
		{"id": "ADMIN", "username": "admin", "role": "platform-admin", "capabilities": map[string]any{"adminPanel": true}},
	})
	createRows(t, app, gpuTestProjectsResource, []map[string]any{
		{"id": "P1", "p_id": "P1", "project_name": "vision"},
		{"id": "P2", "p_id": "P2", "project_name": "language"},
	})
	createRows(t, app, gpuTestJobsResource, []map[string]any{
		{"id": "J1", "user_id": "U1", "project_id": "P1", "queue_name": "gpuq", "status": "running"},
		{"id": "J2", "user_id": "U2", "project_id": "P2", "queue_name": "gpuq", "status": "running"},
		{"id": "JOLD", "user_id": "U1", "project_id": "P1", "status": "completed"},
	})
	createRows(t, app, gpuTestSummariesResource, []map[string]any{
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
		{"id": "SOLD", "job_id": "JOLD", "computed_at": march, "metrics": map[string]any{
			"total_gpu_seconds": 3600.0,
			"total_cpu_seconds": 3600.0,
		}},
	})
	now := time.Now().UTC()
	createRows(t, app, gpuTestSnapshotsResource, []map[string]any{
		{"id": "SN1", "job_id": "J1", "user_id": "U1", "pod_name": "pod-a", "pod_namespace": "ns-a", "node": "gpu-node-1", "gpu_index": 0, "mps_physical_gpu_index": 0, "mps_virtual_units": 2, "timestamp": now.Add(-2 * time.Minute), "metrics": map[string]any{
			"gpu_uuid":              "GPU-a",
			"gpu_memory_bytes":      2048.0,
			"gpu_sm_utilization":    50.0,
			"gpu_mem_utilization":   20.0,
			"gpu_memory_used_bytes": 1024.0,
		}},
		{"id": "SN2", "job_id": "J2", "user_id": "U2", "pod_name": "pod-b", "pod_namespace": "ns-b", "node": "gpu-node-1", "gpu_index": 1, "mps_physical_gpu_index": 1, "mps_virtual_units": 4, "timestamp": now.Add(-1 * time.Minute), "metrics": map[string]any{
			"gpu_uuid":         "GPU-b",
			"gpu_memory_bytes": 4096.0,
		}},
		{"id": "SNOLD", "job_id": "J1", "user_id": "U1", "pod_name": "old", "node": "gpu-node-1", "gpu_index": 0, "mps_virtual_units": 1, "timestamp": now.Add(-30 * time.Minute), "metrics": map[string]any{
			"gpu_uuid": "GPU-old",
		}},
	})
}

func createRows(t *testing.T, app *platform.App, resource string, rows []map[string]any) {
	t.Helper()
	for _, row := range rows {
		if _, err := app.Store.Create(context.Background(), resource, row); err != nil {
			t.Fatal(err)
		}
	}
}
