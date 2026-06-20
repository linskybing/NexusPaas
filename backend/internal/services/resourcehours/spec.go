package resourcehours

import (
	"net/http"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

func Spec() platform.ServiceSpec {
	route, id, admin, adapter, serviceInternal := shared.Route, shared.ID, shared.Admin, shared.Adapter, shared.ServiceInternal
	return platform.ServiceSpec{
		Name:            "usage-observability-service",
		Category:        "ops-read-model",
		Phase:           "3",
		RequiresCluster: true,
		Description:     "GPU usage, resource hours, cluster summary, dashboards, Prometheus queries, snapshots, and retention cleanup.",
		Tables:          []string{"job_gpu_usage_snapshots", "job_gpu_usage_summaries", "pod_resource_records", "resource_hour_summaries", "gpu_authorization_roles", "gpu_identity_roles", "gpu_identity_users", "gpu_jobs", "gpu_projects", "cluster_read_models", "cluster_identity_users", "cluster_identity_roles", "cluster_policy_roles", "cluster_policy_role_assignments", "cluster_projects", "cluster_project_members", "cluster_user_groups", "dashboard_users", "dashboard_projects", "dashboard_project_members", "dashboard_forms", "dashboard_live_quotas", "dashboard_queues", "outbox", "inbox"},
		Events:          []string{"UsageSnapshotRecorded", "ResourceHoursSummarized"},
		Routes: []platform.RouteSpec{
			route(http.MethodGet, "/api/v1/me/usage", "usage", "list"),
			route(http.MethodGet, "/api/v1/me/gpu/jobs", "gpu_jobs", "list"),
			route(http.MethodGet, "/api/v1/me/request-usage", "request_usage", "list"),
			route(http.MethodGet, "/api/v1/admin/usage", "admin_usage", "list", admin()),
			route(http.MethodGet, "/api/v1/admin/request-usage", "admin_request_usage", "list", admin()),
			route(http.MethodGet, "/api/v1/admin/gpu/users", "gpu_users", "list", admin()),
			route(http.MethodGet, "/api/v1/admin/gpu/users/history", "gpu_users", "list_history", admin()),
			route(http.MethodGet, "/api/v1/admin/gpu/users/{userId}/jobs", "gpu_jobs", "list_by_user", id("userId"), admin()),
			route(http.MethodGet, "/api/v1/dashboard/overview", "dashboard", "list"),
			route(http.MethodGet, "/api/v1/admin/dashboard-summary", "dashboard", "list", admin()),
			route(http.MethodGet, "/api/v1/cluster/summary", "cluster_read_models", "list"),
			route(http.MethodGet, "/api/v1/cluster/gpu-usage", "cluster_gpu_usage", "list", admin()),
			route(http.MethodGet, "/api/v1/cluster/mps-mapping", "mps_read_models", "list"),
			route(http.MethodGet, "/api/v1/cluster/mps", "mps_read_models", "list", adapter("prometheus")),
			route(http.MethodGet, "/api/v1/cluster/nodes", "cluster_nodes", "list", admin()),
			route(http.MethodGet, "/api/v1/cluster/nodes/{name}", "cluster_nodes", "get", id("name"), admin()),
			route(http.MethodGet, "/api/v1/admin/mps-mapping", "mps_read_models", "list", admin()),
			route(http.MethodGet, "/api/v1/projects/gpu-usage/by-user", "project_gpu_usage", "list_by_user"),
			route(http.MethodGet, "/api/v1/projects/{id}/gpu-usage", "project_gpu_usage", "get", id("id")),
			route(http.MethodGet, "/api/v1/resource-hours", "resource_hours", "list"),
			route(http.MethodPost, "/api/v1/internal/usage/snapshots", "usage_snapshots", "command", adapter("prometheus"), serviceInternal()),
			route(http.MethodPost, "/api/v1/internal/usage/cleanup", "usage_retention", "command", serviceInternal()),
		},
	}
}
