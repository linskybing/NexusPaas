package imageregistry

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
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
	idempotencyKeyHeader        = "Idempotency-Key"

	internalImageBuildIdempotencyKeyHash               = "internal_idempotency_key_hash"
	internalImageBuildIdempotencyFingerprintHash       = "internal_idempotency_fingerprint_hash"
	internalImageBuildCancelIdempotencyKeyHash         = "internal_cancel_idempotency_key_hash"
	internalImageBuildCancelIdempotencyFingerprintHash = "internal_cancel_idempotency_fingerprint_hash"
)

type imageBuildResources struct {
	cpuCores            float64
	memoryGiB           float64
	maxBuildTimeSeconds int
}

func Register(app *platform.App) {
	if err := seedDefaultImageAccelerationProfiles(app); err != nil {
		panic(err)
	}
	app.RegisterCustomHandler(http.MethodPost, "/api/v1/image-acceleration-profiles", createImageAccelerationProfile)
	app.RegisterCustomHandler(http.MethodPut, "/api/v1/image-acceleration-profiles/{id}", updateImageAccelerationProfile)
	app.RegisterCustomHandler(http.MethodDelete, "/api/v1/image-acceleration-profiles/{id}", deleteImageAccelerationProfile)
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
	registerHarborCatalogSync(app)
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
	tagID := shared.FirstNonBlank(shared.TextValue(payload, "tag_id", "tagId"), "catalog")
	record := map[string]any{
		"id":           tagID,
		"tag_id":       tagID,
		"status":       "sync_requested",
		"requested_by": userID,
		"updated_at":   time.Now().UTC(),
	}
	updated, err := app.UpsertRecordWithEvent(r.Context(), imageSyncResource, tagID, record, func(rec contracts.Record[map[string]any]) contracts.Event {
		return registryEvent(r, "ImageCatalogSyncRequested", rec.Data)
	})
	if err != nil {
		return http.StatusInternalServerError, shared.ErrorData("catalog sync request failed"), nil
	}
	return http.StatusAccepted, syncHarborCatalogTarget(r.Context(), app, tagID, updated.Data, payload, time.Now().UTC()), nil
}

func listCatalogSyncStatus(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	return http.StatusOK, imageRows(app, r, imageSyncResource), nil
}

func getCatalogSyncStatus(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	tagID := shared.FirstNonBlank(r.PathValue("tagId"), r.PathValue("id"))
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
	tagID := shared.FirstNonBlank(r.PathValue("id"), shared.TextValue(payload, "tag_id", "tagId", "image_id", "imageId"))
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
		record, err := app.UpsertRecordWithEvent(r.Context(), projectImagesResource, ruleID(projectID, tagID), rule, func(rec contracts.Record[map[string]any]) contracts.Event {
			return registryEvent(r, "ImagePublished", rec.Data)
		})
		if err != nil {
			return http.StatusInternalServerError, shared.ErrorData("catalog publish failed"), nil
		}
		rules = append(rules, record.Data)
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
	tagID := shared.FirstNonBlank(r.PathValue("id"), r.PathValue("tagId"))
	deleted, err := unpublishCatalogRules(app, r, tagID)
	if err != nil {
		return http.StatusInternalServerError, shared.ErrorData("catalog unpublish failed"), nil
	}
	if deleted == 0 {
		return http.StatusNotFound, shared.ErrorData("publish rule not found"), nil
	}
	return http.StatusOK, map[string]any{"deleted": deleted}, nil
}

func unpublishCatalogRules(app *platform.App, r *http.Request, tagID string) (int, error) {
	deleted := 0
	for _, rule := range imageRows(app, r, projectImagesResource) {
		if !catalogRuleMatchesTag(rule, tagID) {
			continue
		}
		removed, err := deletePublishedCatalogRule(app, r, shared.TextValue(rule, "id"), tagID)
		if err != nil {
			return 0, err
		}
		if removed {
			deleted++
		}
	}
	return deleted, nil
}

func catalogRuleMatchesTag(rule map[string]any, tagID string) bool {
	return shared.TextValue(rule, "tag_id", "tagId") == tagID ||
		shared.TextValue(rule, "id") == tagID
}

func deletePublishedCatalogRule(app *platform.App, r *http.Request, ruleID, tagID string) (bool, error) {
	removed := false
	err := app.WithTx(r.Context(), func(tx platform.StoreTx) error {
		ok, err := tx.Delete(r.Context(), projectImagesResource, ruleID)
		if err != nil {
			return err
		}
		removed = ok
		if ok {
			tx.Emit(registryEvent(r, "ImageUnpublished", map[string]any{"id": ruleID, "tag_id": tagID}))
		}
		return nil
	})
	return removed, err
}

func deletePublishedRule(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	userID, status, data, ok := requireUser(r)
	if !ok {
		return status, data, nil
	}
	if !hasAdminPanel(app, r, userID) {
		return http.StatusForbidden, shared.ErrorData(msgAdminAccessRequired), nil
	}
	ruleID := shared.FirstNonBlank(r.PathValue("ruleId"), r.PathValue("id"))
	deletedRule, err := app.DeleteRecordWithEvent(r.Context(), projectImagesResource, ruleID, func(bool) contracts.Event {
		return registryEvent(r, "ImageUnpublished", map[string]any{"id": ruleID})
	})
	if err != nil {
		return http.StatusInternalServerError, shared.ErrorData("publish rule could not be deleted"), nil
	}
	if !deletedRule {
		return http.StatusNotFound, shared.ErrorData("publish rule not found"), nil
	}
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
	tagID := shared.FirstNonBlank(r.PathValue("tagId"), r.PathValue("id"))
	deleted, err := deleteCatalogArtifactCascade(app, r, tagID)
	if err != nil {
		return http.StatusInternalServerError, shared.ErrorData("catalog artifact delete failed"), nil
	}
	if !deleted {
		return http.StatusNotFound, shared.ErrorData("catalog artifact not found"), nil
	}
	return http.StatusOK, nil, nil
}

func deleteCatalogArtifactCascade(app *platform.App, r *http.Request, tagID string) (bool, error) {
	deleted := false
	err := app.WithTx(r.Context(), func(tx platform.StoreTx) error {
		var err error
		deleted, err = tx.Delete(r.Context(), imageCatalogResource, tagID)
		if err != nil || !deleted {
			return err
		}
		if err := deleteProjectImageRulesForTagTx(app, r, tx, tagID); err != nil {
			return err
		}
		tx.Emit(registryEvent(r, "ImageCatalogDeleted", map[string]any{"id": tagID}))
		return nil
	})
	return deleted, err
}

func deleteProjectImageRulesForTagTx(app *platform.App, r *http.Request, tx platform.StoreTx, tagID string) error {
	for _, rule := range imageRowsWithoutSync(app, r, projectImagesResource) {
		if shared.TextValue(rule, "tag_id", "tagId") != tagID {
			continue
		}
		if _, err := tx.Delete(r.Context(), projectImagesResource, shared.TextValue(rule, "id")); err != nil {
			return err
		}
	}
	return nil
}

func listProjectImages(app *platform.App, r *http.Request, route platform.RouteSpec) (int, any, *platform.Degraded) {
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
	return http.StatusOK, rows, harborDegraded(app, r, route, "listProjectImages")
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
	record, err := app.CreateRecordWithEvent(r.Context(), imageRequestsResource, request, func(rec contracts.Record[map[string]any]) contracts.Event {
		return registryEvent(r, "ImageRequested", rec.Data)
	})
	if err != nil {
		return http.StatusInternalServerError, shared.ErrorData("image request could not be created"), nil
	}
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
	id := shared.FirstNonBlank(r.PathValue("requestId"), r.PathValue("image_id"))
	record, found := findProjectImageRule(app, r, projectID, id)
	if !found {
		return http.StatusNotFound, shared.ErrorData("project image not found"), nil
	}
	if _, err := app.DeleteRecordWithEvent(r.Context(), projectImagesResource, record.ID, func(bool) contracts.Event {
		return registryEvent(r, "ProjectImageRemoved", record.Data)
	}); err != nil {
		return http.StatusInternalServerError, shared.ErrorData("project image could not be removed"), nil
	}
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
	return setImageRequestStatus(app, r, shared.FirstNonBlank(r.PathValue("id"), shared.TextValue(payload, "id")), shared.TextValue(payload, "status"), userID)
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

func listProjectBuilds(app *platform.App, r *http.Request, route platform.RouteSpec) (int, any, *platform.Degraded) {
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
	return http.StatusOK, rows, harborDegraded(app, r, route, "listProjectBuilds")
}

func getBuildLogs(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	userID, status, data, ok := requireUser(r)
	if !ok {
		return status, data, nil
	}
	buildID := shared.FirstNonBlank(r.PathValue("buildId"), r.PathValue("jobName"))
	build, found := findBuild(app, r, buildID)
	if !found {
		return http.StatusNotFound, shared.ErrorData("build not found"), nil
	}
	projectID := shared.TextValue(build.Data, "project_id", "projectId")
	if _, status, data, ok := requireProjectRead(app, r, projectID, userID); !ok {
		return status, data, nil
	}
	logs := shared.FirstNonBlank(shared.TextValue(build.Data, "logs"), "build logs are not available yet\n")
	return http.StatusOK, platform.RawResponse{ContentType: "text/plain; charset=utf-8", Body: []byte(redactImageBuildLogs(logs))}, nil
}

const imageBuildLogRedaction = "[REDACTED]"

var (
	imageBuildLogBearerRE = regexp.MustCompile(`(?i)\b(authorization\s*:\s*bearer\s+)([^\s"'\\]+)`)
	imageBuildLogSecretRE = regexp.MustCompile(`(?i)\b([A-Za-z0-9_.-]*(?:password|passwd|secret|token|api_key|apikey|access_key|private_key|credential)[A-Za-z0-9_.-]*)(\s*[:=]\s*)(?:"([^"\r\n]*)"|'([^'\r\n]*)'|([^\s,;]+))`)
)

func redactImageBuildLogs(logs string) string {
	redacted := imageBuildLogBearerRE.ReplaceAllString(logs, `${1}`+imageBuildLogRedaction)
	return imageBuildLogSecretRE.ReplaceAllStringFunc(redacted, func(match string) string {
		parts := imageBuildLogSecretRE.FindStringSubmatch(match)
		if len(parts) == 0 {
			return match
		}
		value := imageBuildLogRedaction
		switch {
		case parts[3] != "":
			value = `"` + imageBuildLogRedaction + `"`
		case parts[4] != "":
			value = `'` + imageBuildLogRedaction + `'`
		}
		return parts[1] + parts[2] + value
	})
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
	buildID := shared.FirstNonBlank(r.PathValue("jobName"), r.PathValue("buildId"))
	build, found := findBuild(app, r, buildID)
	if !found || shared.TextValue(build.Data, "project_id", "projectId") != projectID {
		return http.StatusNotFound, shared.ErrorData("build not found"), nil
	}
	keyHash, fingerprintHash := imageBuildCancelIdempotencyHashes(r, userID, projectID, build)
	if status, data := imageBuildCancelIdempotencyResult(app, r, keyHash, fingerprintHash); status != 0 {
		return status, data, nil
	}
	update := map[string]any{"status": "cancelled", "updated_at": time.Now().UTC()}
	if keyHash != "" {
		update[internalImageBuildCancelIdempotencyKeyHash] = keyHash
		update[internalImageBuildCancelIdempotencyFingerprintHash] = fingerprintHash
	}
	updated, ok, err := app.UpdateRecordWithEvent(r.Context(), imageBuildsResource, build.ID, update, func(rec contracts.Record[map[string]any]) contracts.Event {
		return registryEvent(r, "ImageBuildCancelled", rec.Data)
	})
	if err != nil || !ok {
		return http.StatusInternalServerError, shared.ErrorData("build update failed"), nil
	}
	return http.StatusOK, publicImageBuildData(updated.Data), nil
}

func createBuild(app *platform.App, r *http.Request, route platform.RouteSpec, buildType string) (int, any, *platform.Degraded) {
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
	project, status, data, ok := requireProjectManager(app, r, projectID, userID)
	if !ok {
		return status, data, nil
	}
	imageRef := imageReference(payload)
	if imageRef == "" {
		return http.StatusBadRequest, shared.ErrorData("image reference is required"), nil
	}
	resources, ok := imageBuildResourcesFromPayload(payload)
	if !ok {
		return http.StatusBadRequest, shared.ErrorData("cpu_cores, memory_gib, and max_build_time_seconds are required"), nil
	}
	if status, data, ok := admitImageBuildPolicy(project, resources); !ok {
		return status, data, nil
	}
	requestedID := imageBuildRequestedID(payload)
	keyHash, fingerprintHash := imageBuildIdempotencyHashes(r, imageBuildIdempotencyFingerprint{
		RequestedID:         requestedID,
		UserID:              userID,
		BuildType:           buildType,
		ProjectID:           projectID,
		ImageReference:      imageRef,
		CPU:                 resources.cpuCores,
		MemoryGiB:           resources.memoryGiB,
		MaxBuildTimeSeconds: resources.maxBuildTimeSeconds,
	})
	if status, data, degraded := imageBuildIdempotencyResult(app, r, route, keyHash, fingerprintHash); status != 0 {
		return status, data, degraded
	}
	if status, data, ok := admitImageBuildConcurrency(app, r, projectID, project); !ok {
		return status, data, nil
	}
	id := shared.FirstNonBlank(requestedID, app.Store.NextID(imageBuildsResource, "build-", 1, 6))
	now := time.Now().UTC()
	build := map[string]any{
		"id":                     id,
		"job_name":               id,
		"build_id":               id,
		"project_id":             projectID,
		"image_reference":        imageRef,
		"build_type":             buildType,
		"cpu_cores":              resources.cpuCores,
		"memory_gib":             resources.memoryGiB,
		"max_build_time_seconds": resources.maxBuildTimeSeconds,
		"status":                 "queued",
		"requested_by":           userID,
		"created_at":             now,
		"updated_at":             now,
		"logs":                   "build queued\n",
	}
	if keyHash != "" {
		build[internalImageBuildIdempotencyKeyHash] = keyHash
		build[internalImageBuildIdempotencyFingerprintHash] = fingerprintHash
	}
	record, err := app.CreateRecordWithEvent(r.Context(), imageBuildsResource, build, func(rec contracts.Record[map[string]any]) contracts.Event {
		return registryEvent(r, "ImageBuildStarted", rec.Data)
	})
	if err != nil {
		return http.StatusInternalServerError, shared.ErrorData("image build could not be created"), nil
	}
	return http.StatusAccepted, publicImageBuildData(record.Data), harborDegraded(app, r, route, "createBuild")
}

func imageBuildIdempotencyResult(app *platform.App, r *http.Request, route platform.RouteSpec, keyHash, fingerprintHash string) (int, any, *platform.Degraded) {
	if keyHash == "" {
		return 0, nil, nil
	}
	existing, found := findImageBuildByIdempotencyKeyHash(app, r, keyHash)
	if !found {
		return 0, nil, nil
	}
	if shared.TextValue(existing.Data, internalImageBuildIdempotencyFingerprintHash) != fingerprintHash {
		return http.StatusConflict, shared.ErrorData("idempotency key is already used by a different image build request"), nil
	}
	return http.StatusAccepted, publicImageBuildData(existing.Data), harborDegraded(app, r, route, "createBuild")
}

func imageBuildResourcesFromPayload(payload map[string]any) (imageBuildResources, bool) {
	cpu, ok := positiveNumberValue(payload, "cpu_cores", "cpu")
	if !ok {
		return imageBuildResources{}, false
	}
	memory, ok := positiveNumberValue(payload, "memory_gib", "memory_gb")
	if !ok {
		return imageBuildResources{}, false
	}
	maxBuildSeconds, ok := positiveNumberValue(payload, "max_build_seconds", "max_build_time_seconds")
	if !ok {
		return imageBuildResources{}, false
	}
	maxBuildTimeSeconds := int(maxBuildSeconds)
	if maxBuildTimeSeconds <= 0 || float64(maxBuildTimeSeconds) != maxBuildSeconds {
		return imageBuildResources{}, false
	}
	return imageBuildResources{
		cpuCores:            cpu,
		memoryGiB:           memory,
		maxBuildTimeSeconds: maxBuildTimeSeconds,
	}, true
}

func admitImageBuild(app *platform.App, r *http.Request, projectID string, project map[string]any, resources imageBuildResources) (int, any, bool) {
	if status, data, ok := admitImageBuildPolicy(project, resources); !ok {
		return status, data, false
	}
	return admitImageBuildConcurrency(app, r, projectID, project)
}

func admitImageBuildPolicy(project map[string]any, resources imageBuildResources) (int, any, bool) {
	if !shared.BoolValue(project, "allow_image_build", "allowImageBuild") {
		return http.StatusForbidden, shared.ErrorData("image builds are not enabled for this project"), false
	}
	if exceededBuildLimit(resources.cpuCores, project, "build_cpu_limit") {
		return http.StatusConflict, shared.ErrorData("build CPU request exceeds project limit"), false
	}
	if exceededBuildLimit(resources.memoryGiB, project, "build_memory_gib_limit") {
		return http.StatusConflict, shared.ErrorData("build memory request exceeds project limit"), false
	}
	if exceededBuildLimit(float64(resources.maxBuildTimeSeconds), project, "build_time_limit_seconds") {
		return http.StatusConflict, shared.ErrorData("build time request exceeds project limit"), false
	}
	return 0, nil, true
}

func admitImageBuildConcurrency(app *platform.App, r *http.Request, projectID string, project map[string]any) (int, any, bool) {
	if exceededActiveBuildLimit(app, r, projectID, project) {
		return http.StatusConflict, shared.ErrorData("project active image build limit reached"), false
	}
	return 0, nil, true
}

type imageBuildIdempotencyFingerprint struct {
	RequestedID         string  `json:"requested_id"`
	UserID              string  `json:"user_id"`
	BuildType           string  `json:"build_type"`
	ProjectID           string  `json:"project_id"`
	ImageReference      string  `json:"image_reference"`
	CPU                 float64 `json:"cpu_cores"`
	MemoryGiB           float64 `json:"memory_gib"`
	MaxBuildTimeSeconds int     `json:"max_build_time_seconds"`
}

type imageBuildCancelIdempotencyFingerprint struct {
	Command   string `json:"command"`
	UserID    string `json:"user_id"`
	ProjectID string `json:"project_id"`
	RecordID  string `json:"record_id"`
	ID        string `json:"id"`
	JobName   string `json:"job_name"`
	BuildID   string `json:"build_id"`
}

func imageBuildRequestedID(payload map[string]any) string {
	return shared.FirstNonBlank(shared.TextValue(payload, "id", "job_name", "jobName", "build_id", "buildId"))
}

func imageBuildIdempotencyHashes(r *http.Request, fingerprint imageBuildIdempotencyFingerprint) (string, string) {
	key := strings.TrimSpace(r.Header.Get(idempotencyKeyHeader))
	if key == "" {
		return "", ""
	}
	rawFingerprint, _ := json.Marshal(fingerprint)
	return imageBuildHash(key), imageBuildHash(string(rawFingerprint))
}

func imageBuildCancelIdempotencyHashes(r *http.Request, userID, projectID string, build imageRecord) (string, string) {
	key := strings.TrimSpace(r.Header.Get(idempotencyKeyHeader))
	if key == "" {
		return "", ""
	}
	fingerprint := imageBuildCancelIdempotencyFingerprint{
		Command:   "image_build_cancel",
		UserID:    userID,
		ProjectID: projectID,
		RecordID:  build.ID,
		ID:        shared.FirstNonBlank(shared.TextValue(build.Data, "id"), build.ID),
		JobName:   shared.FirstNonBlank(shared.TextValue(build.Data, "job_name", "jobName"), build.ID),
		BuildID:   shared.FirstNonBlank(shared.TextValue(build.Data, "build_id", "buildId"), build.ID),
	}
	rawFingerprint, _ := json.Marshal(fingerprint)
	return imageBuildHash(key), imageBuildHash(string(rawFingerprint))
}

func imageBuildHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func findImageBuildByIdempotencyKeyHash(app *platform.App, r *http.Request, keyHash string) (imageRecord, bool) {
	if app == nil || app.Store == nil || keyHash == "" {
		return imageRecord{}, false
	}
	for _, record := range app.Store.List(r.Context(), imageBuildsResource) {
		if shared.TextValue(record.Data, internalImageBuildIdempotencyKeyHash) == keyHash {
			return imageRecord{ID: record.ID, Data: record.Data}, true
		}
	}
	return imageRecord{}, false
}

func imageBuildCancelIdempotencyResult(app *platform.App, r *http.Request, keyHash, fingerprintHash string) (int, any) {
	if keyHash == "" {
		return 0, nil
	}
	existing, found := findImageBuildByCancelIdempotencyKeyHash(app, r, keyHash)
	if !found {
		return 0, nil
	}
	if shared.TextValue(existing.Data, internalImageBuildCancelIdempotencyFingerprintHash) != fingerprintHash {
		return http.StatusConflict, shared.ErrorData("idempotency key is already used by a different image build cancel request")
	}
	return http.StatusOK, publicImageBuildData(existing.Data)
}

func findImageBuildByCancelIdempotencyKeyHash(app *platform.App, r *http.Request, keyHash string) (imageRecord, bool) {
	if app == nil || app.Store == nil || keyHash == "" {
		return imageRecord{}, false
	}
	for _, record := range app.Store.List(r.Context(), imageBuildsResource) {
		if shared.TextValue(record.Data, internalImageBuildCancelIdempotencyKeyHash) == keyHash {
			return imageRecord{ID: record.ID, Data: record.Data}, true
		}
	}
	return imageRecord{}, false
}

func publicImageBuildData(data map[string]any) map[string]any {
	out := shared.CloneMap(data)
	delete(out, internalImageBuildIdempotencyKeyHash)
	delete(out, internalImageBuildIdempotencyFingerprintHash)
	delete(out, internalImageBuildCancelIdempotencyKeyHash)
	delete(out, internalImageBuildCancelIdempotencyFingerprintHash)
	return out
}

func exceededBuildLimit(requested float64, project map[string]any, keys ...string) bool {
	limit, ok := optionalNumberValue(project, keys...)
	return ok && requested > limit
}

func exceededActiveBuildLimit(app *platform.App, r *http.Request, projectID string, project map[string]any) bool {
	limit, ok := activeBuildLimit(project)
	return ok && activeProjectImageBuilds(app, r, projectID) >= limit
}

func activeBuildLimit(project map[string]any) (int, bool) {
	limit, ok := optionalNumberValue(project, "max_running_builds")
	if concurrent, found := optionalNumberValue(project, "max_concurrent_builds"); found && (!ok || concurrent < limit) {
		limit = concurrent
		ok = true
	}
	return int(limit), ok
}

func activeProjectImageBuilds(app *platform.App, r *http.Request, projectID string) int {
	if app == nil || app.Store == nil {
		return 0
	}
	active := 0
	for _, record := range app.Store.List(r.Context(), imageBuildsResource) {
		if shared.TextValue(record.Data, "project_id", "projectId") != projectID {
			continue
		}
		if activeImageBuildStatus(shared.TextValue(record.Data, "status")) {
			active++
		}
	}
	return active
}

func activeImageBuildStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "queued", "pending", "running", "building":
		return true
	default:
		return false
	}
}

func positiveNumberValue(data map[string]any, keys ...string) (float64, bool) {
	value, ok := optionalNumberValue(data, keys...)
	return value, ok && value > 0
}

func optionalNumberValue(data map[string]any, keys ...string) (float64, bool) {
	for _, key := range keys {
		value, exists := data[key]
		if !exists {
			continue
		}
		if number, ok := numberValue(value); ok {
			return number, true
		}
		return 0, false
	}
	return 0, false
}

func numberValue(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}
