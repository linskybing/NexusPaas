package workload

import (
	"net/http"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

func Spec() platform.ServiceSpec {
	const configFileID = "/api/v1/configfiles/{id}"
	route, id, serviceInternal, adapter := shared.Route, shared.ID, shared.ServiceInternal, shared.Adapter
	return platform.ServiceSpec{
		Name:            "workload-service",
		Category:        "compute",
		Phase:           "5",
		RequiresCluster: true,
		Description:     "Immutable ConfigFiles, job submission, job state machine, job logs, templates, and workload saga orchestration.",
		Tables:          []string{"config_files", "config_blobs", "config_commits", "jobs", "job_logs", "job_templates", "outbox", "inbox"},
		Events:          []string{"ConfigCommitted", "ConfigFileChanged", "JobSubmitted", "JobQueued", "JobRunning", "JobSucceeded", "JobFailed", "JobCancelRequested", "JobCancelled"},
		Routes: []platform.RouteSpec{
			route(http.MethodGet, "/api/v1/configfiles", "configfiles", "list"),
			route(http.MethodPost, "/api/v1/configfiles", "configfiles", "create"),
			route(http.MethodGet, configFileID, "configfiles", "get", id("id")),
			route(http.MethodPut, configFileID, "configfiles", "update", id("id")),
			route(http.MethodPatch, configFileID, "configfiles", "update", id("id")),
			route(http.MethodDelete, configFileID, "configfiles", "delete", id("id")),
			route(http.MethodPost, configFileID+"/versions", "configfiles", "config_commit", id("id")),
			route(http.MethodGet, configFileID+"/versions", "configfiles", "list_versions", id("id")),
			route(http.MethodGet, "/api/v1/configfiles/tree", "configfiles", "tree"),
			route(http.MethodGet, "/api/v1/configfiles/project/{project_id}", "configfiles", "list", id("project_id")),
			route(http.MethodGet, "/api/v1/configfiles/project/{project_id}/tree", "configfiles", "tree", id("project_id")),
			route(http.MethodGet, "/api/v1/configfiles/project/{project_id}/history", "configfiles", "list_versions", id("project_id")),
			route(http.MethodGet, "/api/v1/projects/{id}/config-files", "configfiles", "list", id("id")),
			route(http.MethodPost, configFileID+"/instance", "instances", "command", id("id"), adapter("k8s")),
			route(http.MethodDelete, configFileID+"/instance", "instances", "command", id("id"), adapter("k8s")),
			route(http.MethodGet, configFileID+"/instance/pods", "instances", "list", id("id")),
			route(http.MethodGet, "/api/v1/jobs/templates", "job_templates", "list"),
			route(http.MethodGet, "/api/v1/jobs", "jobs", "list"),
			route(http.MethodPost, "/api/v1/jobs", "jobs", "command"),
			route(http.MethodGet, "/api/v1/jobs/{id}", "jobs", "get", id("id")),
			route(http.MethodPost, "/api/v1/jobs/{id}/cancel", "jobs", "command", id("id")),
			route(http.MethodGet, "/api/v1/jobs/{id}/logs", "job_logs", "list", id("id")),
			route(http.MethodGet, "/api/v1/jobs/{id}/gpu-summary", "job_gpu_usage", "list", id("id")),
			route(http.MethodGet, "/api/v1/jobs/{id}/gpu-timeline", "job_gpu_usage", "list", id("id")),
			route(http.MethodGet, "/api/v1/jobs/{id}/gpu-breakdown", "job_gpu_usage", "list", id("id")),
			route(http.MethodPost, "/api/v1/stream/credentials", "stream_credentials", "create"),
			route(http.MethodGet, "/internal/workload/preemption-context", "preemption_context", "internal_read", serviceInternal()),
			route(http.MethodPost, "/internal/workload/jobs/{id}/preempt", "jobs", "preempt", id("id"), serviceInternal()),
			route(http.MethodPost, "/internal/workload/jobs/{id}/evict", "jobs", "evict", id("id"), serviceInternal()),
		},
	}
}
