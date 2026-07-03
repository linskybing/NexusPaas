package imageregistry

import (
	"net/http"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

func Spec() platform.ServiceSpec {
	route, id, admin, adapter, aliasOf := shared.Route, shared.ID, shared.Admin, shared.Adapter, shared.AliasOf
	const imageAccelerationProfileID = "/api/v1/image-acceleration-profiles/{id}"
	return platform.ServiceSpec{
		Name:        "image-registry-service",
		Category:    "supply-chain",
		Phase:       "2",
		Description: "Harbor catalog sync, project image governance, build workflows, allow-list publishing, and registry health.",
		Tables:      []string{"container_repositories", "container_tags", "sync_targets", "image_allow_lists", "image_requests", "image_build_jobs", "image_acceleration_profiles", "outbox", "inbox"},
		Events:      []string{"ImageRequested", "ImageApproved", "ImageBuildStarted", "ImageBuilt", "ImagePublished", "ImageSyncFailed", "ImageAccelerationProfileChanged"},
		Routes: []platform.RouteSpec{
			route(http.MethodGet, "/api/v1/image-acceleration-profiles", "image_acceleration_profiles", "list", admin()),
			route(http.MethodPost, "/api/v1/image-acceleration-profiles", "image_acceleration_profiles", "create", admin()),
			route(http.MethodGet, imageAccelerationProfileID, "image_acceleration_profiles", "get", id("id"), admin()),
			route(http.MethodPut, imageAccelerationProfileID, "image_acceleration_profiles", "update", id("id"), admin()),
			route(http.MethodDelete, imageAccelerationProfileID, "image_acceleration_profiles", "delete", id("id"), admin()),
			route(http.MethodGet, "/api/v1/harbor-projects", "harbor_projects", "list", adapter("harbor")),
			route(http.MethodGet, "/api/v1/projects/{id}/images", "project_images", "list", id("id")),
			route(http.MethodPost, "/api/v1/projects/{id}/images", "project_images", "create", id("id")),
			route(http.MethodDelete, "/api/v1/projects/{id}/images/{requestId}", "project_images", "delete", id("requestId")),
			route(http.MethodGet, "/api/v1/projects/{id}/image-requests", "image_requests", "list", id("id")),
			route(http.MethodGet, "/api/v1/image-requests", "image_requests", "list"),
			route(http.MethodPost, "/api/v1/image-requests", "image_requests", "create"),
			route(http.MethodPut, "/api/v1/image-requests/batch/status", "image_requests", "batch_update", admin()),
			route(http.MethodPut, "/api/v1/image-requests/{id}/approve", "image_requests", "update", id("id"), admin()),
			route(http.MethodPut, "/api/v1/image-requests/{id}/reject", "image_requests", "update", id("id"), admin()),
			route(http.MethodPatch, "/api/v1/image-requests/{id}", "image_requests", "update", id("id"), admin()),
			route(http.MethodPatch, "/api/v1/image-requests/batch", "image_requests", "batch_update", admin()),
			route(http.MethodPost, "/api/v1/images/build", "image_builds", "command", adapter("harbor")),
			route(http.MethodPost, "/api/v1/images/build/context", "image_build_contexts", "command"),
			route(http.MethodPost, "/api/v1/images/build/from-storage", "image_builds", "command", adapter("harbor")),
			route(http.MethodPost, "/api/v1/images/build/dockerfile", "image_builds", "command", adapter("harbor")),
			route(http.MethodGet, "/api/v1/images/build/{jobName}/logs", "image_build_logs", "list", id("jobName"), adapter("harbor")),
			route(http.MethodGet, "/api/v1/images/build/{buildId}/logs", "image_build_logs", "list", id("buildId"), adapter("harbor"), aliasOf("/api/v1/images/build/{jobName}/logs")),
			route(http.MethodGet, "/api/v1/projects/{id}/builds", "image_builds", "list", id("id")),
			route(http.MethodDelete, "/api/v1/projects/{id}/builds/{jobName}", "image_builds", "delete", id("jobName"), adapter("harbor")),
			route(http.MethodGet, "/api/v1/projects/{id}/image-builds", "image_builds", "list", id("id")),
			route(http.MethodDelete, "/api/v1/projects/{id}/image-builds/{buildId}", "image_builds", "delete", id("buildId"), adapter("harbor")),
			route(http.MethodGet, "/api/v1/image-catalog", "image_catalog", "list", adapter("harbor")),
			route(http.MethodPost, "/api/v1/image-catalog/sync", "image_catalog", "command", adapter("harbor")),
			route(http.MethodPost, "/api/v1/image-catalog/publish", "image_catalog", "command", adapter("harbor")),
			route(http.MethodPost, "/api/v1/image-catalog/{id}/publish", "image_catalog", "command", id("id"), adapter("harbor")),
			route(http.MethodPost, "/api/v1/image-catalog/{id}/unpublish", "image_catalog", "command", id("id"), adapter("harbor")),
			route(http.MethodDelete, "/api/v1/image-catalog/publish/{ruleId}", "image_catalog", "delete", id("ruleId"), adapter("harbor")),
			route(http.MethodGet, "/api/v1/image-catalog/{tagId}/sync-status", "image_catalog", "get", id("tagId"), adapter("harbor")),
			route(http.MethodDelete, "/api/v1/image-catalog/{id}", "image_catalog", "delete", id("id"), adapter("harbor")),
			route(http.MethodGet, "/api/v1/image-catalog/sync-status", "image_catalog", "list", adapter("harbor")),
			route(http.MethodGet, "/api/v1/harbor-status", "harbor_status", "list", adapter("harbor")),
			route(http.MethodGet, "/api/v1/harbor-statistics", "harbor_statistics", "list", adapter("harbor")),
		},
	}
}
