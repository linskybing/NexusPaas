package services

import (
	"net/http"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

func TestClusterReadModelWorkflow(t *testing.T) {
	app := newTestApp()
	seedClusterReadData(t, app)

	requestJSON(t, app, http.MethodGet, "/api/v1/cluster/summary", "", nil, http.StatusUnauthorized)
	summary := responseMap(t, requestJSON(t, app, http.MethodGet, "/api/v1/cluster/summary", "", userHeaders("U1"), http.StatusOK))
	if summary["nodes"] != nil || summary["podGpuUsages"] != nil {
		t.Fatalf("cluster summary = %#v, want public summary without nodes or pod GPU usage", summary)
	}
	if summary["nodeCount"] != float64(2) || summary["totalGpuUsed"] != float64(2) {
		t.Fatalf("cluster summary = %#v, want aggregate counts", summary)
	}

	forgedAdmin := userHeaders("U1")
	forgedAdmin["X-User-Role"] = "admin"
	requestJSON(t, app, http.MethodGet, "/api/v1/cluster/nodes", "", forgedAdmin, http.StatusForbidden)
	requestJSON(t, app, http.MethodGet, "/api/v1/cluster/gpu-usage", "", forgedAdmin, http.StatusForbidden)
	requestJSON(t, app, http.MethodGet, "/api/v1/cluster/nodes/gpu-node-missing", "", adminHeaders("ADMIN"), http.StatusNotFound)

	nodes := responseSlice(t, requestJSON(t, app, http.MethodGet, "/api/v1/cluster/nodes", "", adminHeaders("ADMIN"), http.StatusOK))
	if len(nodes) != 2 || nodes[0].(map[string]any)["name"] != "cpu-node-1" {
		t.Fatalf("cluster nodes = %#v, want sorted full node list", nodes)
	}
	node := responseMap(t, requestJSON(t, app, http.MethodGet, "/api/v1/cluster/nodes/gpu-node-1", "", adminHeaders("ADMIN"), http.StatusOK))
	if node["gpuAllocatable"] != float64(2) {
		t.Fatalf("cluster node = %#v, want GPU detail", node)
	}
	podGPU := responseSlice(t, requestJSON(t, app, http.MethodGet, "/api/v1/cluster/gpu-usage", "", adminHeaders("ADMIN"), http.StatusOK))
	if len(podGPU) != 4 || podGPU[0].(map[string]any)["namespace"] != "project-P1" {
		t.Fatalf("pod GPU usage = %#v, want all pod GPU records", podGPU)
	}

	requestJSON(t, app, http.MethodGet, "/api/v1/projects/gpu-usage/by-user", "", nil, http.StatusBadRequest)
	usageByUser := responseMap(t, requestJSON(t, app, http.MethodGet, "/api/v1/projects/gpu-usage/by-user", "", userHeaders("U1"), http.StatusOK))
	if usageByUser["P1"] != float64(2) || usageByUser["P2"] != float64(1) || usageByUser["P3"] != nil {
		t.Fatalf("project GPU usage by user = %#v, want only visible projects", usageByUser)
	}
	if usageByUser["P4"] != float64(1) {
		t.Fatalf("project GPU usage by user = %#v, want group-owned project P4 included", usageByUser)
	}

	requestJSON(t, app, http.MethodGet, "/api/v1/projects/P3/gpu-usage", "", userHeaders("U1"), http.StatusForbidden)
	requestJSON(t, app, http.MethodGet, "/api/v1/projects/UNKNOWN/gpu-usage", "", adminHeaders("ADMIN"), http.StatusNotFound)
	projectUsage := responseMap(t, requestJSON(t, app, http.MethodGet, "/api/v1/projects/P1/gpu-usage", "", userHeaders("U1"), http.StatusOK))
	if projectUsage["used"] != float64(2) {
		t.Fatalf("project usage = %#v, want two P1 pod GPU records", projectUsage)
	}
	adminProjectUsage := responseMap(t, requestJSON(t, app, http.MethodGet, "/api/v1/projects/P3/gpu-usage", "", adminHeaders("ADMIN"), http.StatusOK))
	if adminProjectUsage["used"] != float64(0) {
		t.Fatalf("admin project usage = %#v, want zero for P3", adminProjectUsage)
	}
}

func seedClusterReadData(t *testing.T, app *platform.App) {
	t.Helper()
	createRows(t, app, "identity-service:users", []map[string]any{
		{"id": "U1", "username": "alice", "capabilities": map[string]any{"adminPanel": false}},
		{"id": "ADMIN", "username": "admin", "capabilities": map[string]any{"adminPanel": true}},
	})
	createRows(t, app, "org-project-service:projects", []map[string]any{
		{"id": "P1", "p_id": "P1", "project_name": "vision"},
		{"id": "P2", "p_id": "P2", "project_name": "language", "personal_user_id": "U1"},
		{"id": "P3", "p_id": "P3", "project_name": "private"},
		{"id": "P4", "p_id": "P4", "project_name": "group-visible", "owner_id": "G1"},
	})
	createRows(t, app, "org-project-service:project_members", []map[string]any{
		{"id": "pm1", "project_id": "P1", "user_id": "U1"},
	})
	createRows(t, app, "org-project-service:user_groups", []map[string]any{
		{"id": "ug1", "group_id": "G1", "user_id": "U1", "role": "member"},
	})
	createRows(t, app, "usage-observability-service:cluster_read_models", []map[string]any{
		{
			"id":                            "cluster",
			"nodeCount":                     2,
			"totalCpuAllocatableMilli":      16000,
			"totalCpuUsedMilli":             4000,
			"totalMemoryAllocatableBytes":   64 << 30,
			"totalMemoryUsedBytes":          16 << 30,
			"totalGpuAllocatable":           2.0,
			"totalGpuUsed":                  2.0,
			"deviceClasses":                 []any{"nvidia-h100"},
			"collectedAt":                   time.Now().UTC().Format(time.RFC3339),
			"unexpected_reference_passthru": "kept",
			"nodes": []any{
				map[string]any{"name": "gpu-node-1", "gpuAllocatable": 2.0, "gpuUsed": 2.0},
				map[string]any{"name": "cpu-node-1", "gpuAllocatable": 0.0, "gpuUsed": 0.0},
			},
			"podGpuUsages": []any{
				map[string]any{"podName": "pod-a", "namespace": "project-P1", "node": "gpu-node-1", "gpuIndex": 0, "gpuUuid": "GPU-a", "memoryBytes": 1024.0, "utilization": 0.5},
				map[string]any{"podName": "pod-b", "namespace": "project-P1", "node": "gpu-node-1", "gpuIndex": 1, "gpuUuid": "GPU-b", "memoryBytes": 2048.0, "utilization": 0.7},
				map[string]any{"podName": "pod-c", "namespace": "project-P2", "node": "gpu-node-1", "gpuIndex": 1, "gpuUuid": "GPU-b", "memoryBytes": 4096.0, "utilization": 0.2},
				map[string]any{"podName": "pod-d", "namespace": "project-P4", "node": "gpu-node-1", "gpuIndex": 0, "gpuUuid": "GPU-a", "memoryBytes": 1024.0, "utilization": 0.1},
			},
		},
	})
}
