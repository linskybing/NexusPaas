package imageregistry

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

func TestImageRegistryCatalogRequestsAndBuildWorkflow(t *testing.T) {
	app := newImageRegistryTestApp(t)

	code, data, degraded := getHarborStatus(app, imageRequest(http.MethodGet, "/api/v1/harbor-status", "", "U2"), platform.RouteSpec{})
	assertImageStatus(t, code, data, http.StatusOK)
	if degraded == nil || degraded.Adapter != "harbor" {
		t.Fatalf("harbor degraded = %#v, want harbor adapter degraded", degraded)
	}

	publishReq := imageRequest(http.MethodPost, "/api/v1/image-catalog/publish", `{"tag_id":"tag-1","project_id":"P1"}`, "ADMIN")
	code, data, _ = publishCatalog(app, publishReq, platform.RouteSpec{})
	assertImageStatus(t, code, data, http.StatusOK)
	if rules := data.(map[string]any)["rules"].([]map[string]any); len(rules) != 1 {
		t.Fatalf("publish result = %#v, want one rule", data)
	}

	listReq := imageProjectRequest(http.MethodGet, "/api/v1/projects/P1/images", "", "U2", "P1")
	code, data, _ = listProjectImages(app, listReq, platform.RouteSpec{})
	assertImageStatus(t, code, data, http.StatusOK)
	if images := data.([]map[string]any); len(images) != 1 || images[0]["tag_id"] != "tag-1" {
		t.Fatalf("project images = %#v, want published catalog image", images)
	}

	deniedReq := imageProjectRequest(http.MethodPost, "/api/v1/projects/P1/images", `{"image_reference":"alpine:3.19"}`, "U2", "P1")
	code, data, _ = requestProjectImage(app, deniedReq, platform.RouteSpec{})
	assertImageStatus(t, code, data, http.StatusForbidden)

	requestReq := imageProjectRequest(http.MethodPost, "/api/v1/projects/P1/images", `{"id":"IR1","image_reference":"alpine:3.19"}`, "U1", "P1")
	code, data, _ = requestProjectImage(app, requestReq, platform.RouteSpec{})
	assertImageStatus(t, code, data, http.StatusCreated)
	if request := data.(map[string]any); request["status"] != "pending" || request["requested_by"] != "U1" {
		t.Fatalf("image request = %#v, want pending request by U1", request)
	}

	approveReq := imageRequest(http.MethodPut, "/api/v1/image-requests/IR1/approve", "", "ADMIN")
	approveReq.SetPathValue("id", "IR1")
	code, data, _ = approveImageRequest(app, approveReq, platform.RouteSpec{})
	assertImageStatus(t, code, data, http.StatusOK)
	if data.(map[string]any)["status"] != "approved" {
		t.Fatalf("approved request = %#v", data)
	}

	requestsReq := imageProjectRequest(http.MethodGet, "/api/v1/projects/P1/image-requests", "", "U2", "P1")
	code, data, _ = listProjectImageRequests(app, requestsReq, platform.RouteSpec{})
	assertImageStatus(t, code, data, http.StatusOK)
	if requests := data.([]map[string]any); len(requests) != 1 {
		t.Fatalf("project image requests = %#v, want one", requests)
	}

	buildReq := imageRequest(http.MethodPost, "/api/v1/images/build/dockerfile", `{"id":"build-1","project_id":"P1","image_reference":"registry.local/team/app:dev"}`, "U1")
	code, data, _ = startDockerfileImageBuild(app, buildReq, platform.RouteSpec{})
	assertImageStatus(t, code, data, http.StatusAccepted)
	if build := data.(map[string]any); build["build_type"] != "dockerfile" || build["status"] != "queued" {
		t.Fatalf("build = %#v, want queued dockerfile build", build)
	}

	buildsReq := imageProjectRequest(http.MethodGet, "/api/v1/projects/P1/builds", "", "U2", "P1")
	code, data, _ = listProjectBuilds(app, buildsReq, platform.RouteSpec{})
	assertImageStatus(t, code, data, http.StatusOK)
	if builds := data.([]map[string]any); len(builds) != 1 {
		t.Fatalf("builds = %#v, want one", builds)
	}

	logReq := imageRequest(http.MethodGet, "/api/v1/images/build/build-1/logs", "", "U2")
	logReq.SetPathValue("buildId", "build-1")
	code, data, _ = getBuildLogs(app, logReq, platform.RouteSpec{})
	assertImageStatus(t, code, data, http.StatusOK)
	if raw := data.(platform.RawResponse); !strings.Contains(string(raw.Body), "build queued") {
		t.Fatalf("build logs = %q, want queued message", string(raw.Body))
	}

	cancelDenied := imageProjectRequest(http.MethodDelete, "/api/v1/projects/P1/builds/build-1", "", "U2", "P1")
	cancelDenied.SetPathValue("jobName", "build-1")
	code, data, _ = cancelProjectBuild(app, cancelDenied, platform.RouteSpec{})
	assertImageStatus(t, code, data, http.StatusForbidden)
	cancelReq := imageProjectRequest(http.MethodDelete, "/api/v1/projects/P1/builds/build-1", "", "U1", "P1")
	cancelReq.SetPathValue("jobName", "build-1")
	code, data, _ = cancelProjectBuild(app, cancelReq, platform.RouteSpec{})
	assertImageStatus(t, code, data, http.StatusOK)
	if data.(map[string]any)["status"] != "cancelled" {
		t.Fatalf("cancelled build = %#v", data)
	}
}

func TestImageRegistryValidationAndCatalogMutation(t *testing.T) {
	app := newImageRegistryTestApp(t)

	badReq := imageProjectRequest(http.MethodPost, "/api/v1/projects/P1/images", `{`, "U1", "P1")
	code, data, _ := requestProjectImage(app, badReq, platform.RouteSpec{})
	assertImageStatus(t, code, data, http.StatusBadRequest)
	if got := len(app.Store.List(context.Background(), imageRequestsResource)); got != 0 {
		t.Fatalf("image requests after malformed JSON = %d, want none", got)
	}

	syncReq := imageRequest(http.MethodPost, "/api/v1/image-catalog/sync", `{"tag_id":"tag-1"}`, "U1")
	code, data, _ = syncCatalog(app, syncReq, platform.RouteSpec{})
	assertImageStatus(t, code, data, http.StatusAccepted)
	statusReq := imageRequest(http.MethodGet, "/api/v1/image-catalog/tag-1/sync-status", "", "U1")
	statusReq.SetPathValue("tagId", "tag-1")
	code, data, _ = getCatalogSyncStatus(app, statusReq, platform.RouteSpec{})
	assertImageStatus(t, code, data, http.StatusOK)
	if data.(map[string]any)["status"] != "sync_requested" {
		t.Fatalf("sync status = %#v, want sync_requested", data)
	}

	deleteReq := imageRequest(http.MethodDelete, "/api/v1/image-catalog/tag-1", "", "ADMIN")
	deleteReq.SetPathValue("tagId", "tag-1")
	code, data, _ = deleteCatalogArtifact(app, deleteReq, platform.RouteSpec{})
	assertImageStatus(t, code, data, http.StatusOK)
	if _, found := app.Store.Get(context.Background(), imageCatalogResource, "tag-1"); found {
		t.Fatal("catalog artifact was not deleted")
	}
}

func TestImageRegistryCatalogListingsAndPublishDeletion(t *testing.T) {
	app := newImageRegistryTestApp(t)

	code, data, _ := getHarborStatistics(app, imageRequest(http.MethodGet, "/api/v1/harbor-statistics", "", "U2"), platform.RouteSpec{})
	assertImageStatus(t, code, data, http.StatusOK)
	assertImageMapValue(t, data, "projects", 1)

	code, data, _ = listHarborProjects(app, imageRequest(http.MethodGet, "/api/v1/harbor-projects", "", "U2"), platform.RouteSpec{})
	assertImageStatus(t, code, data, http.StatusOK)
	assertImageRows(t, data, 1)

	code, data, _ = listCatalog(app, imageRequest(http.MethodGet, "/api/v1/image-catalog", "", "U2"), platform.RouteSpec{})
	assertImageStatus(t, code, data, http.StatusOK)
	assertImageRows(t, data, 1)

	syncReq := imageRequest(http.MethodPost, "/api/v1/image-catalog/sync", `{"tag_id":"tag-1"}`, "U1")
	code, data, _ = syncCatalog(app, syncReq, platform.RouteSpec{})
	assertImageStatus(t, code, data, http.StatusAccepted)
	code, data, _ = listCatalogSyncStatus(app, imageRequest(http.MethodGet, "/api/v1/image-catalog/sync-status", "", "U1"), platform.RouteSpec{})
	assertImageStatus(t, code, data, http.StatusOK)
	assertImageRows(t, data, 1)

	publishReq := imageRequest(http.MethodPost, "/api/v1/image-catalog/tag-1/publish", `{"project_id":"P1"}`, "ADMIN")
	publishReq.SetPathValue("id", "tag-1")
	republishReq := imageRequest(http.MethodPost, "/api/v1/image-catalog/tag-1/publish", `{"project_id":"P1"}`, "ADMIN")
	republishReq.SetPathValue("id", "tag-1")
	code, data, _ = publishCatalog(app, republishReq, platform.RouteSpec{})
	assertImageStatus(t, code, data, http.StatusOK)

	unpublishReq := imageRequest(http.MethodPost, "/api/v1/image-catalog/tag-1/unpublish", "", "ADMIN")
	unpublishReq.SetPathValue("id", "tag-1")
	code, data, _ = unpublishCatalog(app, unpublishReq, platform.RouteSpec{})
	assertImageStatus(t, code, data, http.StatusOK)
	assertImageMapValue(t, data, "deleted", 1)

	code, data, _ = publishCatalog(app, publishReq, platform.RouteSpec{})
	assertImageStatus(t, code, data, http.StatusOK)
	deleteRuleReq := imageRequest(http.MethodDelete, "/api/v1/image-catalog/publish/P1:tag-1", "", "ADMIN")
	deleteRuleReq.SetPathValue("ruleId", "P1:tag-1")
	code, data, _ = deletePublishedRule(app, deleteRuleReq, platform.RouteSpec{})
	assertImageStatus(t, code, data, http.StatusOK)
}

func TestImageRegistryRequestAdministrationAndBuildTypes(t *testing.T) {
	app := newImageRegistryTestApp(t)

	createReq := imageRequest(http.MethodPost, "/api/v1/image-requests", `{"id":"IRA","project_id":"P1","image_reference":"busybox:1.36"}`, "U1")
	code, data, _ := createImageRequest(app, createReq, platform.RouteSpec{})
	assertImageStatus(t, code, data, http.StatusCreated)

	listReq := imageRequest(http.MethodGet, "/api/v1/image-requests?status=pending&project_id=P1", "", "U1")
	code, data, _ = listImageRequests(app, listReq, platform.RouteSpec{})
	assertImageStatus(t, code, data, http.StatusOK)
	assertImageRows(t, data, 1)

	updateReq := imageRequest(http.MethodPatch, "/api/v1/image-requests/IRA", `{"status":"pending"}`, "ADMIN")
	updateReq.SetPathValue("id", "IRA")
	code, data, _ = updateImageRequest(app, updateReq, platform.RouteSpec{})
	assertImageStatus(t, code, data, http.StatusOK)

	rejectReq := imageRequest(http.MethodPut, "/api/v1/image-requests/IRA/reject", "", "ADMIN")
	rejectReq.SetPathValue("id", "IRA")
	code, data, _ = rejectImageRequest(app, rejectReq, platform.RouteSpec{})
	assertImageStatus(t, code, data, http.StatusOK)
	assertImageMapValue(t, data, "status", "rejected")

	batchReq := imageRequest(http.MethodPut, "/api/v1/image-requests/batch/status", `{"ids":["IRA","missing"],"status":"approved"}`, "ADMIN")
	code, data, _ = batchUpdateImageRequests(app, batchReq, platform.RouteSpec{})
	assertImageStatus(t, code, data, http.StatusOK)
	assertImageMapValue(t, data, "succeeded", 1)
	assertImageMapValue(t, data, "failed", 1)

	removeReq := imageProjectRequest(http.MethodDelete, "/api/v1/projects/P1/images/busybox:1.36", "", "U1", "P1")
	removeReq.SetPathValue("requestId", "busybox:1.36")
	code, data, _ = removeProjectImage(app, removeReq, platform.RouteSpec{})
	assertImageStatus(t, code, data, http.StatusOK)

	contextBuild := imageRequest(http.MethodPost, "/api/v1/images/build", `{"id":"ctx-build","project_id":"P1","repository":"team/app","tag":"ctx"}`, "U1")
	code, data, _ = startImageBuild(app, contextBuild, platform.RouteSpec{})
	assertImageStatus(t, code, data, http.StatusAccepted)
	assertImageMapValue(t, data, "build_type", "context")

	storageBuild := imageRequest(http.MethodPost, "/api/v1/images/build/from-storage", `{"id":"storage-build","project_id":"P1","image_reference":"registry.local/team/app:storage"}`, "U1")
	code, data, _ = startStorageImageBuild(app, storageBuild, platform.RouteSpec{})
	assertImageStatus(t, code, data, http.StatusAccepted)
	assertImageMapValue(t, data, "build_type", "storage")
}

func TestImageRegistryProjectionUpsertAndDelete(t *testing.T) {
	app := newImageRegistryTestApp(t)
	req := imageRequest(http.MethodPost, "/internal/projections", "", "ADMIN")

	err := applyImageProjectionEvent(app, req, contracts.Event{Name: "UserUpdated", Data: map[string]any{"id": "U4", "username": "dana"}})
	assertImageNoError(t, err)
	assertImageRecordExists(t, app, imageIdentityUsersResource, "U4")

	err = applyImageProjectionEvent(app, req, contracts.Event{Name: "UserDeleted", Data: map[string]any{"id": "U4"}})
	assertImageNoError(t, err)
	assertImageRecordMissing(t, app, imageIdentityUsersResource, "U4")

	err = applyImageProjectionEvent(app, req, contracts.Event{Name: "Project_MemberCreated", Data: map[string]any{"project_id": "P1", "user_id": "U4", "role": "user"}})
	assertImageNoError(t, err)
	assertImageRecordExists(t, app, imageProjectMembersResource, "P1:U4")
}

func newImageRegistryTestApp(t *testing.T) *platform.App {
	t.Helper()
	app := platform.NewApp(platform.Config{ServiceName: "all", HTTPAddr: ":0"})
	Register(app)
	createImageRecords(t, app, identityUsersResource, []map[string]any{
		{"id": "ADMIN", "username": "admin", "capabilities": map[string]any{"adminPanel": true}},
		{"id": "U1", "username": "alice"},
		{"id": "U2", "username": "bob"},
	})
	createImageRecords(t, app, orgProjectsResource, []map[string]any{
		{"id": "P1", "project_name": "vision", "owner_id": "G1"},
	})
	createImageRecords(t, app, orgUserGroupsResource, []map[string]any{
		{"id": "U1:G1", "user_id": "U1", "group_id": "G1", "role": "admin"},
		{"id": "U2:G1", "user_id": "U2", "group_id": "G1", "role": "user"},
	})
	createImageRecords(t, app, imageCatalogResource, []map[string]any{
		{"id": "tag-1", "registry": "registry.local", "repository": "library/base", "tag": "1.0"},
	})
	return app
}

func createImageRecords(t *testing.T, app *platform.App, resource string, rows []map[string]any) {
	t.Helper()
	for _, row := range rows {
		if _, err := app.Store.Create(context.Background(), resource, row); err != nil {
			t.Fatal(err)
		}
	}
}

func imageRequest(method, target, body, userID string) *http.Request {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if userID != "" {
		req.Header.Set("X-User-ID", userID)
	}
	return req
}

func imageProjectRequest(method, target, body, userID, projectID string) *http.Request {
	req := imageRequest(method, target, body, userID)
	req.SetPathValue("id", projectID)
	return req
}

func assertImageStatus(t *testing.T, code int, data any, want int) {
	t.Helper()
	if code != want {
		t.Fatalf("status=%d data=%#v, want %d", code, data, want)
	}
}

func assertImageRows(t *testing.T, data any, want int) {
	t.Helper()
	rows := data.([]map[string]any)
	if len(rows) != want {
		t.Fatalf("rows = %#v, want %d", rows, want)
	}
}

func assertImageMapValue(t *testing.T, data any, key string, want any) {
	t.Helper()
	row := data.(map[string]any)
	if row[key] != want {
		t.Fatalf("%s = %#v, want %#v in %#v", key, row[key], want, row)
	}
}

func assertImageNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func assertImageRecordExists(t *testing.T, app *platform.App, resource, id string) {
	t.Helper()
	if _, ok := app.Store.Get(context.Background(), resource, id); !ok {
		t.Fatalf("%s/%s missing", resource, id)
	}
}

func assertImageRecordMissing(t *testing.T, app *platform.App, resource, id string) {
	t.Helper()
	if _, ok := app.Store.Get(context.Background(), resource, id); ok {
		t.Fatalf("%s/%s unexpectedly exists", resource, id)
	}
}
