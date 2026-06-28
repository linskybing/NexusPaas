package k8scontrol

import (
	"net/http"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

func Spec() platform.ServiceSpec {
	route, id, admin, adapter := shared.Route, shared.ID, shared.Admin, shared.Adapter
	return platform.ServiceSpec{
		Name:            "k8s-control-service",
		Category:        "compute-infra",
		Phase:           "5",
		RequiresCluster: true,
		Description:     "Single Kubernetes API adapter, resource snapshots, command/status APIs, pod logs/events, and watch contracts.",
		Tables:          []string{"k8s_operations", "namespace_mappings", "pod_snapshots", "resource_snapshots", "outbox", "inbox"},
		Events:          []string{"ResourceSnapshotRecorded", "NamespaceCreated", "NamespaceDeleted"},
		Routes: []platform.RouteSpec{
			route(http.MethodGet, "/api/v1/k8s/cluster", "cluster_snapshots", "list", adapter("k8s")),
			route(http.MethodGet, "/api/v1/k8s/nodes", "nodes", "list", adapter("k8s")),
			route(http.MethodGet, "/api/v1/k8s/nodes/{id}", "nodes", "get", id("id"), adapter("k8s")),
			route(http.MethodGet, "/api/v1/k8s/namespaces/{ns}/pods/{name}/logs", "pod_logs", "list", id("name"), adapter("k8s")),
			route(http.MethodGet, "/api/v1/k8s/pods/{id}/logs", "pod_logs", "list", id("id"), adapter("k8s")),
			route(http.MethodGet, "/api/v1/k8s/pods/{id}/events", "pod_events", "list", id("id"), adapter("k8s")),
			route(http.MethodGet, "/api/v1/k8s/user-storage/status", "user_storage", "get", adapter("k8s")),
			route(http.MethodPost, "/api/v1/k8s/user-storage/browse", "user_storage", "command", adapter("k8s")),
			route(http.MethodDelete, "/api/v1/k8s/user-storage/browse", "user_storage", "command", adapter("k8s")),
			route(http.MethodDelete, "/api/v1/k8s/resources/{id}", "resources", "command", id("id"), adapter("k8s")),
			route(http.MethodGet, "/api/v1/resources", "resources", "list", adapter("k8s")),
			route(http.MethodDelete, "/api/v1/resources/{id}", "resources", "command", id("id"), adapter("k8s")),
			route(http.MethodGet, "/api/v1/admin/resources", "resources", "list", admin(), adapter("k8s")),
			route(http.MethodDelete, "/api/v1/admin/resources/projects/{id}", "project_resources", "command", id("id"), admin(), adapter("k8s")),
			route(http.MethodGet, "/api/v1/projects/{id}/namespaces", "project_namespaces", "list", id("id"), adapter("k8s")),
			route(http.MethodGet, "/api/v1/projects/{id}/resources", "project_resources", "list", id("id"), adapter("k8s")),
			route(http.MethodDelete, "/api/v1/projects/{id}/resources/{userId}", "project_resources", "command", id("userId"), adapter("k8s")),
			route(http.MethodPost, "/api/v1/projects/{id}/resources/cleanup", "project_resources", "command", id("id"), adapter("k8s")),
			route(http.MethodGet, "/api/v1/resources/{namespace}/pods/{name}/events", "pod_events", "list", id("name"), adapter("k8s")),
			route(http.MethodDelete, "/api/v1/resources/{namespace}/{kind}/{name}", "resources", "command", id("name"), adapter("k8s")),
			route(http.MethodPost, "/internal/k8s-control/fast-transfers/mover-jobs", "fast_transfer_mover_jobs", "create", shared.ServiceInternal()),
			route(http.MethodGet, "/api/v1/ws/exec", "ws_exec", "proxy", adapter("k8s")),
			route(http.MethodGet, "/api/v1/ws/watch/{namespace}", "ws_namespace_watch", "proxy", id("namespace"), adapter("k8s")),
			route(http.MethodGet, "/api/v1/ws/watch-project/{projectId}", "ws_project_watch", "proxy", id("projectId"), adapter("k8s")),
			route(http.MethodGet, "/api/v1/ws/job-status/{id}", "ws_job_status", "proxy", id("id"), adapter("k8s")),
			route(http.MethodGet, "/api/v1/ws/namespace-watch", "ws_namespace_watch", "proxy", adapter("k8s")),
			route(http.MethodGet, "/api/v1/ws/pod-logs", "ws_pod_logs", "proxy", adapter("k8s")),
			route(http.MethodGet, "/api/v1/ws/project-watch", "ws_project_watch", "proxy", adapter("k8s")),
			route(http.MethodGet, "/api/v1/ws/job-status", "ws_job_status", "proxy", adapter("k8s")),
			route(http.MethodGet, "/api/v1/ws/storage-status", "ws_storage_status", "proxy", adapter("k8s")),
		},
	}
}
