package imageregistry

import (
	"net/http"
	"strings"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

const (
	serviceName = "image-registry-service"

	projectImagesResource = serviceName + ":image_allow_lists"
	imageRequestsResource = serviceName + ":image_requests"
	imageCatalogResource  = serviceName + ":container_tags"
	imageBuildsResource   = serviceName + ":image_build_jobs"
	imageSyncResource     = serviceName + ":sync_targets"

	imageProjectionConsumer     = serviceName + ":access_projection"
	imageProjectsResource       = serviceName + ":image_projects"
	imageProjectMembersResource = serviceName + ":image_project_members"
	imageUserGroupsResource     = serviceName + ":image_user_groups"
	imageIdentityUsersResource  = serviceName + ":image_identity_users"
	imageIdentityRolesResource  = serviceName + ":image_identity_roles"
	orgProjectsResource         = "org-project-service:projects"
	orgProjectMembersResource   = "org-project-service:project_members"
	orgUserGroupsResource       = "org-project-service:user_groups"
	identityUsersResource       = "identity-service:users"
	identityRolesResource       = "identity-service:roles"
	msgAdminAccessRequired      = "admin access required"
	msgInvalidRequestBody       = "invalid request body"
	msgProjectManagerAccess     = "project manager access required"
	msgProjectMemberAccess      = "project member access required"
	defaultRegistry             = "docker.io"
	defaultTag                  = "latest"
)

func Register(app *platform.App) {
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/harbor-status", getHarborStatus)
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/harbor-statistics", getHarborStatistics)
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/harbor-projects", listHarborProjects)
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/image-catalog", listCatalog)
	app.RegisterCustomHandler(http.MethodPost, "/api/v1/image-catalog/sync", syncCatalog)
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/image-catalog/sync-status", listCatalogSyncStatus)
	app.RegisterCustomHandler(http.MethodPost, "/api/v1/image-catalog/publish", publishCatalog)
	app.RegisterCustomHandler(http.MethodPost, "/api/v1/image-catalog/{id}/publish", publishCatalog)
	app.RegisterCustomHandler(http.MethodPost, "/api/v1/image-catalog/{id}/unpublish", unpublishCatalog)
	app.RegisterCustomHandler(http.MethodDelete, "/api/v1/image-catalog/publish/{ruleId}", deletePublishedRule)
	app.RegisterCustomHandler(http.MethodDelete, "/api/v1/image-catalog/{tagId}", deleteCatalogArtifact)
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/image-catalog/{tagId}/sync-status", getCatalogSyncStatus)
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/projects/{id}/images", listProjectImages)
	app.RegisterCustomHandler(http.MethodPost, "/api/v1/projects/{id}/images", requestProjectImage)
	app.RegisterCustomHandler(http.MethodDelete, "/api/v1/projects/{id}/images/{requestId}", removeProjectImage)
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/projects/{id}/image-requests", listProjectImageRequests)
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/image-requests", listImageRequests)
	app.RegisterCustomHandler(http.MethodPost, "/api/v1/image-requests", createImageRequest)
	app.RegisterCustomHandler(http.MethodPatch, "/api/v1/image-requests/{id}", updateImageRequest)
	app.RegisterCustomHandler(http.MethodPatch, "/api/v1/image-requests/batch", batchUpdateImageRequests)
	app.RegisterCustomHandler(http.MethodPut, "/api/v1/image-requests/batch/status", batchUpdateImageRequests)
	app.RegisterCustomHandler(http.MethodPut, "/api/v1/image-requests/{id}/approve", approveImageRequest)
	app.RegisterCustomHandler(http.MethodPut, "/api/v1/image-requests/{id}/reject", rejectImageRequest)
	app.RegisterCustomHandler(http.MethodPost, "/api/v1/images/build", startImageBuild)
	app.RegisterCustomHandler(http.MethodPost, "/api/v1/images/build/from-storage", startStorageImageBuild)
	app.RegisterCustomHandler(http.MethodPost, "/api/v1/images/build/dockerfile", startDockerfileImageBuild)
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/images/build/{buildId}/logs", getBuildLogs)
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/projects/{id}/builds", listProjectBuilds)
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/projects/{id}/image-builds", listProjectBuilds)
	app.RegisterCustomHandler(http.MethodDelete, "/api/v1/projects/{id}/builds/{jobName}", cancelProjectBuild)
	app.RegisterCustomHandler(http.MethodDelete, "/api/v1/projects/{id}/image-builds/{buildId}", cancelProjectBuild)
	registerHarborHealthChecks(app)
}

func getHarborStatus(app *platform.App, r *http.Request, route platform.RouteSpec) (int, any, *platform.Degraded) {
	result, degraded := callHarbor(app, r, route, "harborStatus")
	if degraded != nil {
		return http.StatusOK, result, degraded
	}
	return http.StatusOK, map[string]any{"status": "ok", "adapter": "harbor", "checked_at": time.Now().UTC()}, nil
}

func getHarborStatistics(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	return http.StatusOK, map[string]any{
		"projects":       len(uniqueHarborProjects(app, r)),
		"catalog_images": len(imageCatalogRows(app, r)),
		"allow_rules":    len(imageRows(app, r, projectImagesResource)),
		"build_jobs":     len(imageRows(app, r, imageBuildsResource)),
	}, nil
}

func listHarborProjects(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	projects := make([]map[string]any, 0)
	for _, name := range uniqueHarborProjects(app, r) {
		projects = append(projects, map[string]any{"name": name})
	}
	return http.StatusOK, projects, nil
}

func listCatalog(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	rows := imageCatalogRows(app, r)
	if projectID := strings.TrimSpace(r.URL.Query().Get("project_id")); projectID != "" {
		rows = filterRows(rows, "project_id", projectID)
	}
	sortRows(rows, "repository", "tag", "id")
	return http.StatusOK, rows, nil
}

func syncCatalog(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	userID, status, data, ok := requireUser(r)
	if !ok {
		return status, data, nil
	}
	payload, err := platform.DecodeMapWithError(r)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(msgInvalidRequestBody), nil
	}
	tagID := firstNonEmpty(shared.TextValue(payload, "tag_id", "tagId"), "catalog")
	record := map[string]any{
		"id":           tagID,
		"tag_id":       tagID,
		"status":       "sync_requested",
		"requested_by": userID,
		"updated_at":   time.Now().UTC(),
	}
	upsertRecord(app, r, imageSyncResource, tagID, record)
	publishEvent(app, r, "ImageCatalogSyncRequested", record)
	return http.StatusAccepted, record, nil
}

func listCatalogSyncStatus(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	return http.StatusOK, imageRows(app, r, imageSyncResource), nil
}

func getCatalogSyncStatus(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	tagID := firstNonEmpty(r.PathValue("tagId"), r.PathValue("id"))
	if record, found := app.Store.Get(r.Context(), imageSyncResource, tagID); found {
		return http.StatusOK, record.Data, nil
	}
	return http.StatusOK, map[string]any{"tag_id": tagID, "status": "not_synced"}, nil
}

func publishCatalog(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	userID, status, data, ok := requireUser(r)
	if !ok {
		return status, data, nil
	}
	if !hasAdminPanel(app, r, userID) {
		return http.StatusForbidden, shared.ErrorData(msgAdminAccessRequired), nil
	}
	payload, err := platform.DecodeMapWithError(r)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(msgInvalidRequestBody), nil
	}
	tagID := firstNonEmpty(r.PathValue("id"), shared.TextValue(payload, "tag_id", "tagId", "image_id", "imageId"))
	if tagID == "" {
		return http.StatusBadRequest, shared.ErrorData("tag_id is required"), nil
	}
	projectIDs := firstStringSlice(payload, "project_ids", "projectIds")
	if projectID := shared.TextValue(payload, "project_id", "projectId"); projectID != "" {
		projectIDs = append(projectIDs, projectID)
	}
	if len(projectIDs) == 0 {
		projectIDs = []string{"*"}
	}
	rules := make([]map[string]any, 0, len(projectIDs))
	for _, projectID := range projectIDs {
		rule := allowRuleFromCatalog(app, r, tagID, projectID, userID)
		record, err := app.Store.Create(r.Context(), projectImagesResource, rule)
		if platform.IsCreateConflict(err) {
			record, _ = app.Store.Update(r.Context(), projectImagesResource, ruleID(projectID, tagID), rule)
			err = nil
		}
		if err != nil {
			return http.StatusInternalServerError, shared.ErrorData("catalog publish failed"), nil
		}
		rules = append(rules, record.Data)
		publishEvent(app, r, "ImagePublished", record.Data)
	}
	return http.StatusOK, map[string]any{"rules": rules}, nil
}

func unpublishCatalog(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	userID, status, data, ok := requireUser(r)
	if !ok {
		return status, data, nil
	}
	if !hasAdminPanel(app, r, userID) {
		return http.StatusForbidden, shared.ErrorData(msgAdminAccessRequired), nil
	}
	tagID := firstNonEmpty(r.PathValue("id"), r.PathValue("tagId"))
	deleted := 0
	for _, rule := range imageRows(app, r, projectImagesResource) {
		if shared.TextValue(rule, "tag_id", "tagId") == tagID || shared.TextValue(rule, "id") == tagID {
			app.Store.Delete(r.Context(), projectImagesResource, shared.TextValue(rule, "id"))
			deleted++
		}
	}
	if deleted == 0 {
		return http.StatusNotFound, shared.ErrorData("publish rule not found"), nil
	}
	publishEvent(app, r, "ImageUnpublished", map[string]any{"tag_id": tagID, "deleted": deleted})
	return http.StatusOK, map[string]any{"deleted": deleted}, nil
}

func deletePublishedRule(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	userID, status, data, ok := requireUser(r)
	if !ok {
		return status, data, nil
	}
	if !hasAdminPanel(app, r, userID) {
		return http.StatusForbidden, shared.ErrorData(msgAdminAccessRequired), nil
	}
	ruleID := firstNonEmpty(r.PathValue("ruleId"), r.PathValue("id"))
	if !app.Store.Delete(r.Context(), projectImagesResource, ruleID) {
		return http.StatusNotFound, shared.ErrorData("publish rule not found"), nil
	}
	publishEvent(app, r, "ImageUnpublished", map[string]any{"id": ruleID})
	return http.StatusOK, nil, nil
}

func deleteCatalogArtifact(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	userID, status, data, ok := requireUser(r)
	if !ok {
		return status, data, nil
	}
	if !hasAdminPanel(app, r, userID) {
		return http.StatusForbidden, shared.ErrorData(msgAdminAccessRequired), nil
	}
	tagID := firstNonEmpty(r.PathValue("tagId"), r.PathValue("id"))
	if !app.Store.Delete(r.Context(), imageCatalogResource, tagID) {
		return http.StatusNotFound, shared.ErrorData("catalog artifact not found"), nil
	}
	for _, rule := range imageRows(app, r, projectImagesResource) {
		if shared.TextValue(rule, "tag_id", "tagId") == tagID {
			app.Store.Delete(r.Context(), projectImagesResource, shared.TextValue(rule, "id"))
		}
	}
	publishEvent(app, r, "ImageCatalogDeleted", map[string]any{"id": tagID})
	return http.StatusOK, nil, nil
}

func listProjectImages(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	userID, status, data, ok := requireUser(r)
	if !ok {
		return status, data, nil
	}
	projectID := projectPathID(r)
	if _, status, data, ok := requireProjectRead(app, r, projectID, userID); !ok {
		return status, data, nil
	}
	rows := make([]map[string]any, 0)
	for _, rule := range imageRows(app, r, projectImagesResource) {
		if !ruleEnabled(rule) {
			continue
		}
		if pid := shared.TextValue(rule, "project_id", "projectId"); pid == projectID || pid == "*" {
			rows = append(rows, enrichRuleWithCatalog(app, r, rule))
		}
	}
	sortRows(rows, "repository", "tag", "id")
	return http.StatusOK, rows, nil
}

func requestProjectImage(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	userID, status, data, ok := requireUser(r)
	if !ok {
		return status, data, nil
	}
	projectID := projectPathID(r)
	if _, status, data, ok := requireProjectManager(app, r, projectID, userID); !ok {
		return status, data, nil
	}
	payload, err := platform.DecodeMapWithError(r)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(msgInvalidRequestBody), nil
	}
	request, err := imageRequestRecord(app, r, projectID, userID, payload)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(err.Error()), nil
	}
	record, err := app.Store.Create(r.Context(), imageRequestsResource, request)
	if err != nil {
		return http.StatusInternalServerError, shared.ErrorData("image request could not be created"), nil
	}
	publishEvent(app, r, "ImageRequested", record.Data)
	return http.StatusCreated, record.Data, nil
}

func removeProjectImage(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	userID, status, data, ok := requireUser(r)
	if !ok {
		return status, data, nil
	}
	projectID := projectPathID(r)
	if _, status, data, ok := requireProjectManager(app, r, projectID, userID); !ok {
		return status, data, nil
	}
	id := firstNonEmpty(r.PathValue("requestId"), r.PathValue("image_id"))
	record, found := findProjectImageRule(app, r, projectID, id)
	if !found {
		return http.StatusNotFound, shared.ErrorData("project image not found"), nil
	}
	app.Store.Delete(r.Context(), projectImagesResource, record.ID)
	publishEvent(app, r, "ProjectImageRemoved", record.Data)
	return http.StatusOK, nil, nil
}

func listImageRequests(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	userID, status, data, ok := requireUser(r)
	if !ok {
		return status, data, nil
	}
	rows := imageRows(app, r, imageRequestsResource)
	if !hasAdminPanel(app, r, userID) {
		rows = filterRows(rows, "requested_by", userID)
	}
	rows = filterByQuery(rows, r, "status", "project_id")
	sortRows(rows, "created_at", "id")
	return http.StatusOK, rows, nil
}

func listProjectImageRequests(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	userID, status, data, ok := requireUser(r)
	if !ok {
		return status, data, nil
	}
	projectID := projectPathID(r)
	if _, status, data, ok := requireProjectRead(app, r, projectID, userID); !ok {
		return status, data, nil
	}
	rows := filterRows(imageRows(app, r, imageRequestsResource), "project_id", projectID)
	rows = filterByQuery(rows, r, "status")
	return http.StatusOK, rows, nil
}

func createImageRequest(app *platform.App, r *http.Request, route platform.RouteSpec) (int, any, *platform.Degraded) {
	payload, err := platform.DecodeMapWithError(r)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(msgInvalidRequestBody), nil
	}
	projectID := shared.TextValue(payload, "project_id", "projectId")
	if projectID == "" {
		return http.StatusBadRequest, shared.ErrorData("project_id is required"), nil
	}
	req := r.Clone(r.Context())
	req.SetPathValue("id", projectID)
	req.Body = newBody(payload)
	return requestProjectImage(app, req, route)
}

func updateImageRequest(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	userID, status, data, ok := requireUser(r)
	if !ok {
		return status, data, nil
	}
	if !hasAdminPanel(app, r, userID) {
		return http.StatusForbidden, shared.ErrorData(msgAdminAccessRequired), nil
	}
	payload, err := platform.DecodeMapWithError(r)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(msgInvalidRequestBody), nil
	}
	return setImageRequestStatus(app, r, firstNonEmpty(r.PathValue("id"), shared.TextValue(payload, "id")), shared.TextValue(payload, "status"), userID)
}

func approveImageRequest(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	userID, status, data, ok := requireUser(r)
	if !ok {
		return status, data, nil
	}
	if !hasAdminPanel(app, r, userID) {
		return http.StatusForbidden, shared.ErrorData(msgAdminAccessRequired), nil
	}
	return setImageRequestStatus(app, r, r.PathValue("id"), "approved", userID)
}

func rejectImageRequest(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	userID, status, data, ok := requireUser(r)
	if !ok {
		return status, data, nil
	}
	if !hasAdminPanel(app, r, userID) {
		return http.StatusForbidden, shared.ErrorData(msgAdminAccessRequired), nil
	}
	return setImageRequestStatus(app, r, r.PathValue("id"), "rejected", userID)
}

func batchUpdateImageRequests(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	userID, status, data, ok := requireUser(r)
	if !ok {
		return status, data, nil
	}
	if !hasAdminPanel(app, r, userID) {
		return http.StatusForbidden, shared.ErrorData(msgAdminAccessRequired), nil
	}
	payload, err := platform.DecodeMapWithError(r)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(msgInvalidRequestBody), nil
	}
	statusValue := shared.TextValue(payload, "status")
	result := batchResult()
	for _, id := range firstStringSlice(payload, "ids", "request_ids", "requestIds") {
		code, data, _ := setImageRequestStatus(app, r, id, statusValue, userID)
		if code >= 400 {
			result["failed"] = result["failed"].(int) + 1
			result["errors"] = append(result["errors"].([]string), batchError(id, data))
			continue
		}
		result["succeeded"] = result["succeeded"].(int) + 1
	}
	return http.StatusOK, result, nil
}

func startImageBuild(app *platform.App, r *http.Request, route platform.RouteSpec) (int, any, *platform.Degraded) {
	return createBuild(app, r, route, "context")
}

func startStorageImageBuild(app *platform.App, r *http.Request, route platform.RouteSpec) (int, any, *platform.Degraded) {
	return createBuild(app, r, route, "storage")
}

func startDockerfileImageBuild(app *platform.App, r *http.Request, route platform.RouteSpec) (int, any, *platform.Degraded) {
	return createBuild(app, r, route, "dockerfile")
}

func listProjectBuilds(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	userID, status, data, ok := requireUser(r)
	if !ok {
		return status, data, nil
	}
	projectID := projectPathID(r)
	if _, status, data, ok := requireProjectRead(app, r, projectID, userID); !ok {
		return status, data, nil
	}
	rows := filterRows(imageRows(app, r, imageBuildsResource), "project_id", projectID)
	sortRows(rows, "created_at", "id")
	return http.StatusOK, rows, nil
}

func getBuildLogs(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	userID, status, data, ok := requireUser(r)
	if !ok {
		return status, data, nil
	}
	buildID := firstNonEmpty(r.PathValue("buildId"), r.PathValue("jobName"))
	build, found := findBuild(app, r, buildID)
	if !found {
		return http.StatusNotFound, shared.ErrorData("build not found"), nil
	}
	projectID := shared.TextValue(build.Data, "project_id", "projectId")
	if _, status, data, ok := requireProjectRead(app, r, projectID, userID); !ok {
		return status, data, nil
	}
	logs := firstNonEmpty(shared.TextValue(build.Data, "logs"), "build logs are not available yet\n")
	return http.StatusOK, platform.RawResponse{ContentType: "text/plain; charset=utf-8", Body: []byte(logs)}, nil
}

func cancelProjectBuild(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	userID, status, data, ok := requireUser(r)
	if !ok {
		return status, data, nil
	}
	projectID := projectPathID(r)
	if _, status, data, ok := requireProjectManager(app, r, projectID, userID); !ok {
		return status, data, nil
	}
	buildID := firstNonEmpty(r.PathValue("jobName"), r.PathValue("buildId"))
	build, found := findBuild(app, r, buildID)
	if !found || shared.TextValue(build.Data, "project_id", "projectId") != projectID {
		return http.StatusNotFound, shared.ErrorData("build not found"), nil
	}
	updated, ok := app.Store.Update(r.Context(), imageBuildsResource, build.ID, map[string]any{"status": "cancelled", "updated_at": time.Now().UTC()})
	if !ok {
		return http.StatusInternalServerError, shared.ErrorData("build update failed"), nil
	}
	publishEvent(app, r, "ImageBuildCancelled", updated.Data)
	return http.StatusOK, updated.Data, nil
}

func createBuild(app *platform.App, r *http.Request, _ platform.RouteSpec, buildType string) (int, any, *platform.Degraded) {
	userID, status, data, ok := requireUser(r)
	if !ok {
		return status, data, nil
	}
	payload, err := platform.DecodeMapWithError(r)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(msgInvalidRequestBody), nil
	}
	projectID := shared.TextValue(payload, "project_id", "projectId")
	if projectID == "" {
		return http.StatusBadRequest, shared.ErrorData("project_id is required"), nil
	}
	if _, status, data, ok := requireProjectManager(app, r, projectID, userID); !ok {
		return status, data, nil
	}
	imageRef := imageReference(payload)
	if imageRef == "" {
		return http.StatusBadRequest, shared.ErrorData("image reference is required"), nil
	}
	id := firstNonEmpty(shared.TextValue(payload, "id", "job_name", "jobName", "build_id", "buildId"), app.Store.NextID(imageBuildsResource, "build-", 1, 6))
	now := time.Now().UTC()
	build := map[string]any{
		"id":              id,
		"job_name":        id,
		"build_id":        id,
		"project_id":      projectID,
		"image_reference": imageRef,
		"build_type":      buildType,
		"status":          "queued",
		"requested_by":    userID,
		"created_at":      now,
		"updated_at":      now,
		"logs":            "build queued\n",
	}
	record, err := app.Store.Create(r.Context(), imageBuildsResource, build)
	if err != nil {
		return http.StatusInternalServerError, shared.ErrorData("image build could not be created"), nil
	}
	publishEvent(app, r, "ImageBuildStarted", record.Data)
	return http.StatusAccepted, record.Data, nil
}
