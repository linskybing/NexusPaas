package imageregistry

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

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

func TestImageRegistryBuildAndListRoutesSurfaceHarborDegradedAdditively(t *testing.T) {
	app := newImageRegistryTestApp(t)
	app.Adapters["harbor"] = fakeImageHarborAdapter{result: contracts.AdapterResult{Adapter: "harbor"}}

	publishReq := imageRequest(http.MethodPost, "/api/v1/image-catalog/publish", `{"tag_id":"tag-1","project_id":"P1"}`, "ADMIN")
	code, data, degraded := publishCatalog(app, publishReq, platform.RouteSpec{})
	assertImageStatus(t, code, data, http.StatusOK)
	assertImageNoDegraded(t, degraded)

	buildRoute := platform.RouteSpec{Method: http.MethodPost, OperationID: "imageBuild"}
	buildReq := imageRequest(http.MethodPost, "/api/v1/images/build", `{"id":"build-harbor-ok","project_id":"P1","image_reference":"registry.local/team/app:ok"}`, "U1")
	code, data, degraded = startImageBuild(app, buildReq, buildRoute)
	assertImageStatus(t, code, data, http.StatusAccepted)
	assertImageNoDegraded(t, degraded)
	assertImageMapValue(t, data, "id", "build-harbor-ok")
	assertImageMapValue(t, data, "status", "queued")

	imageListRoute := platform.RouteSpec{Method: http.MethodGet, OperationID: "projectImages"}
	code, data, degraded = listProjectImages(app, imageProjectRequest(http.MethodGet, "/api/v1/projects/P1/images", "", "U2", "P1"), imageListRoute)
	assertImageStatus(t, code, data, http.StatusOK)
	assertImageNoDegraded(t, degraded)
	assertProjectImagePayload(t, data, "tag-1")

	buildListRoute := platform.RouteSpec{Method: http.MethodGet, OperationID: "projectBuilds"}
	code, data, degraded = listProjectBuilds(app, imageProjectRequest(http.MethodGet, "/api/v1/projects/P1/builds", "", "U2", "P1"), buildListRoute)
	assertImageStatus(t, code, data, http.StatusOK)
	assertImageNoDegraded(t, degraded)
	assertProjectBuildPayload(t, data, "build-harbor-ok")

	app.Adapters["harbor"] = fakeImageHarborAdapter{
		result: contracts.AdapterResult{Adapter: "harbor", Degraded: true, Code: "adapter_unavailable", Retryable: true},
	}

	code, data, degraded = listProjectImages(app, imageProjectRequest(http.MethodGet, "/api/v1/projects/P1/images", "", "U2", "P1"), imageListRoute)
	assertImageStatus(t, code, data, http.StatusOK)
	assertHarborDegraded(t, degraded)
	assertProjectImagePayload(t, data, "tag-1")

	code, data, degraded = listProjectBuilds(app, imageProjectRequest(http.MethodGet, "/api/v1/projects/P1/builds", "", "U2", "P1"), buildListRoute)
	assertImageStatus(t, code, data, http.StatusOK)
	assertHarborDegraded(t, degraded)
	assertProjectBuildPayload(t, data, "build-harbor-ok")

	aliasBuildListRoute := platform.RouteSpec{Method: http.MethodGet, OperationID: "projectImageBuilds"}
	code, data, degraded = listProjectBuilds(app, imageProjectRequest(http.MethodGet, "/api/v1/projects/P1/image-builds", "", "U2", "P1"), aliasBuildListRoute)
	assertImageStatus(t, code, data, http.StatusOK)
	assertHarborDegraded(t, degraded)
	assertProjectBuildPayload(t, data, "build-harbor-ok")

	degradedBuildReq := imageRequest(http.MethodPost, "/api/v1/images/build/dockerfile", `{"id":"build-harbor-down","project_id":"P1","image_reference":"registry.local/team/app:down"}`, "U1")
	code, data, degraded = startDockerfileImageBuild(app, degradedBuildReq, buildRoute)
	assertImageStatus(t, code, data, http.StatusAccepted)
	assertHarborDegraded(t, degraded)
	assertImageMapValue(t, data, "id", "build-harbor-down")
	assertImageMapValue(t, data, "build_type", "dockerfile")
	assertImageMapValue(t, data, "status", "queued")

	storageBuildRoute := platform.RouteSpec{Method: http.MethodPost, OperationID: "imageBuildFromStorage"}
	storageBuildReq := imageRequest(http.MethodPost, "/api/v1/images/build/from-storage", `{"id":"build-harbor-storage-down","project_id":"P1","image_reference":"registry.local/team/app:storage-down"}`, "U1")
	code, data, degraded = startStorageImageBuild(app, storageBuildReq, storageBuildRoute)
	assertImageStatus(t, code, data, http.StatusAccepted)
	assertHarborDegraded(t, degraded)
	assertImageMapValue(t, data, "id", "build-harbor-storage-down")
	assertImageMapValue(t, data, "build_type", "storage")
	assertImageMapValue(t, data, "status", "queued")
}

func TestProjectImagesPromoteCatalogStatusFields(t *testing.T) {
	app := newImageRegistryTestApp(t)
	app.Adapters["harbor"] = fakeImageHarborAdapter{result: contracts.AdapterResult{Adapter: "harbor"}}
	createImageRecords(t, app, imageCatalogResource, []map[string]any{
		{
			"id":          "tag-status",
			"registry":    "registry.local",
			"repository":  "library/status",
			"tag":         "2.0",
			"digest":      "sha256:feedface",
			"scanStatus":  "Success",
			"deleted":     "TRUE",
			"unavailable": "false",
			"status":      "catalog-available",
		},
	})
	createImageRecords(t, app, projectImagesResource, []map[string]any{
		{
			"id":              ruleID("P1", "tag-status"),
			"project_id":      "P1",
			"tag_id":          "tag-status",
			"repository":      "library/status",
			"tag":             "2.0",
			"image_reference": "registry.local/library/status:2.0",
			"enabled":         true,
			"status":          "project-allowed",
		},
	})

	code, data, degraded := listProjectImages(app, imageProjectRequest(http.MethodGet, "/api/v1/projects/P1/images", "", "U2", "P1"), platform.RouteSpec{Method: http.MethodGet, OperationID: "projectImages"})
	assertImageStatus(t, code, data, http.StatusOK)
	assertImageNoDegraded(t, degraded)
	images := data.([]map[string]any)
	if len(images) != 1 {
		t.Fatalf("project images = %#v, want one status-enriched image", images)
	}
	image := images[0]
	if image["digest"] != "sha256:feedface" || image["scan_status"] != "Success" {
		t.Fatalf("project image scan metadata = %#v, want digest and scan_status promoted", image)
	}
	if image["deleted"] != true || image["unavailable"] != false {
		t.Fatalf("project image availability metadata = %#v, want canonical deleted/unavailable booleans", image)
	}
	if image["status"] != "project-allowed" {
		t.Fatalf("project image status = %#v, want Project row precedence over catalog status", image["status"])
	}
	if catalog, ok := image["catalog"].(map[string]any); !ok || catalog["status"] != "catalog-available" {
		t.Fatalf("project image catalog = %#v, want nested catalog preserved", image["catalog"])
	}
}

func TestImageRegistryHarborCatalogSyncExecutesAndUpsertsCatalog(t *testing.T) {
	app := newImageRegistryTestApp(t)
	adapter := &fakeImageHarborProxyAdapter{
		response: contracts.AdapterProxyResponse{
			StatusCode: http.StatusOK,
			Body: []byte(`[
				{
						"digest":"sha256:abc123",
						"repository_name":"library/base",
						"deleted":true,
						"unavailable":"true",
						"tags":[{"name":"1.0"}],
						"scan_overview":{"application/vnd.security.vulnerability.report; version=1.1":{"scan_status":"Success"}},
						"push_time":"2026-06-21T12:00:00Z"
				}
			]`),
		},
		result: contracts.AdapterResult{Adapter: "harbor", Operation: harborCatalogSyncOperation},
	}
	app.Adapters["harbor"] = adapter

	code, data, _ := syncCatalog(app, imageRequest(http.MethodPost, "/api/v1/image-catalog/sync", `{"tag_id":"tag-live","project":"library","repository":"base","tag":"1.0"}`, "U1"), platform.RouteSpec{})
	assertImageStatus(t, code, data, http.StatusAccepted)
	status := data.(map[string]any)
	if status["status"] != "synced" || status["degraded"] != false || status["retryable"] != false {
		t.Fatalf("sync status = %#v, want synced non-degraded", status)
	}
	if status["code"] != "ok" {
		t.Fatalf("sync code = %#v, want ok", status["code"])
	}
	if status["catalog_id"] != "tag-live" {
		t.Fatalf("catalog_id = %#v, want tag-live", status["catalog_id"])
	}
	record, found := app.Store.Get(context.Background(), imageCatalogResource, "tag-live")
	if !found {
		t.Fatal("synced catalog record missing")
	}
	if record.Data["digest"] != "sha256:abc123" || record.Data["scan_status"] != "Success" {
		t.Fatalf("catalog record = %#v, want Harbor digest and scan status", record.Data)
	}
	if record.Data["deleted"] != true || record.Data["unavailable"] != true || record.Data["status"] != "available" {
		t.Fatalf("catalog availability = %#v, want upstream deleted/unavailable parsed", record.Data)
	}
	if len(adapter.requests) != 1 {
		t.Fatalf("proxy requests = %#v, want one", adapter.requests)
	}
	if adapter.requests[0].Path != "/projects/library/artifacts" {
		t.Fatalf("proxy path = %q", adapter.requests[0].Path)
	}
	if !strings.Contains(adapter.requests[0].RawQuery, "with_scan_overview=true") {
		t.Fatalf("proxy query = %q, want scan overview", adapter.requests[0].RawQuery)
	}
}

func TestImageRegistryHarborCatalogSyncFindsArtifactOnSecondPage(t *testing.T) {
	app := newImageRegistryTestApp(t)
	firstPage := make([]map[string]any, 0, harborArtifactPageSize)
	for i := 0; i < harborArtifactPageSize; i++ {
		firstPage = append(firstPage, map[string]any{
			"digest":          "sha256:first",
			"repository_name": "library/base",
			"tags":            []any{map[string]any{"name": "not-target"}},
		})
	}
	adapter := &fakeImageHarborProxyAdapter{
		responses: []contracts.AdapterProxyResponse{
			{StatusCode: http.StatusOK, Body: mustImageJSON(t, firstPage)},
			{StatusCode: http.StatusOK, Body: []byte(`[{"digest":"sha256:second","repository_name":"library/base","tags":[{"name":"1.0"}]}]`)},
		},
		result: contracts.AdapterResult{Adapter: "harbor", Operation: harborCatalogSyncOperation},
	}
	app.Adapters["harbor"] = adapter

	code, data, _ := syncCatalog(app, imageRequest(http.MethodPost, "/api/v1/image-catalog/sync", `{"tag_id":"tag-page","project":"library","repository":"base","tag":"1.0"}`, "U1"), platform.RouteSpec{})
	assertImageStatus(t, code, data, http.StatusAccepted)
	status := data.(map[string]any)
	if status["status"] != "synced" || status["code"] != "ok" {
		t.Fatalf("sync status = %#v, want second-page synced", status)
	}
	record, found := app.Store.Get(context.Background(), imageCatalogResource, "tag-page")
	if !found || record.Data["digest"] != "sha256:second" {
		t.Fatalf("catalog record = %#v found=%v, want second page digest", record.Data, found)
	}
	if len(adapter.requests) != 2 {
		t.Fatalf("proxy requests = %#v, want two pages", adapter.requests)
	}
	assertImageQueryValue(t, adapter.requests[0].RawQuery, "page", "1")
	assertImageQueryValue(t, adapter.requests[1].RawQuery, "page", "2")
}

func TestImageRegistryHarborCatalogSyncStopsPagingAfterMatch(t *testing.T) {
	app := newImageRegistryTestApp(t)
	adapter := &fakeImageHarborProxyAdapter{
		responses: []contracts.AdapterProxyResponse{
			{StatusCode: http.StatusOK, Body: []byte(`[{"digest":"sha256:first","repository_name":"library/base","tags":[{"name":"1.0"}]}]`)},
			{StatusCode: http.StatusOK, Body: []byte(`[{"digest":"sha256:unexpected","repository_name":"library/base","tags":[{"name":"1.0"}]}]`)},
		},
		result: contracts.AdapterResult{Adapter: "harbor", Operation: harborCatalogSyncOperation},
	}
	app.Adapters["harbor"] = adapter

	code, data, _ := syncCatalog(app, imageRequest(http.MethodPost, "/api/v1/image-catalog/sync", `{"tag_id":"tag-stop","project":"library","repository":"base","tag":"1.0"}`, "U1"), platform.RouteSpec{})
	assertImageStatus(t, code, data, http.StatusAccepted)
	status := data.(map[string]any)
	if status["status"] != "synced" || status["code"] != "ok" {
		t.Fatalf("sync status = %#v, want first-page synced", status)
	}
	if len(adapter.requests) != 1 {
		t.Fatalf("proxy requests = %#v, want stop after first page match", adapter.requests)
	}
	assertImageQueryValue(t, adapter.requests[0].RawQuery, "page", "1")
}

func TestImageRegistryCatalogSyncDegradesWhenSelectorsAreMissing(t *testing.T) {
	app := newImageRegistryTestApp(t)

	code, data, _ := syncCatalog(app, imageRequest(http.MethodPost, "/api/v1/image-catalog/sync", `{"tag_id":"missing"}`, "U1"), platform.RouteSpec{})
	assertImageStatus(t, code, data, http.StatusAccepted)
	status := data.(map[string]any)
	if status["status"] != "degraded" || status["code"] != "missing_selector" || status["retryable"] != false {
		t.Fatalf("sync status = %#v, want missing_selector degraded", status)
	}

	code, data, _ = syncCatalog(app, imageRequest(http.MethodPost, "/api/v1/image-catalog/sync", `{"tag_id":"repo-only","project":"library","repository":"base"}`, "U1"), platform.RouteSpec{})
	assertImageStatus(t, code, data, http.StatusAccepted)
	status = data.(map[string]any)
	if status["status"] != "degraded" || status["code"] != "missing_selector" || status["retryable"] != false {
		t.Fatalf("repository-only sync status = %#v, want missing_selector degraded", status)
	}
}

func TestImageRegistryHarborCatalogSyncDegradesWhenAdapterIsMissing(t *testing.T) {
	app := newImageRegistryTestApp(t)

	code, data, _ := syncCatalog(app, imageRequest(http.MethodPost, "/api/v1/image-catalog/sync", `{"tag_id":"tag-1"}`, "U1"), platform.RouteSpec{})
	assertImageStatus(t, code, data, http.StatusAccepted)
	status := data.(map[string]any)
	if status["status"] != "degraded" || status["code"] != "adapter_not_configured" || status["retryable"] != true {
		t.Fatalf("sync status = %#v, want adapter_not_configured degraded", status)
	}
}

func TestImageRegistryHarborCatalogSyncDegradesWhenCatalogPersistFails(t *testing.T) {
	app := newImageRegistryTestApp(t)
	app.Store = failingImageCatalogCreateStore{RecordStore: app.Store}
	app.Adapters["harbor"] = &fakeImageHarborProxyAdapter{
		response: contracts.AdapterProxyResponse{
			StatusCode: http.StatusOK,
			Body:       []byte(`[{"digest":"sha256:abc123","repository_name":"library/base","tags":[{"name":"1.0"}]}]`),
		},
		result: contracts.AdapterResult{Adapter: "harbor", Operation: harborCatalogSyncOperation},
	}

	code, data, _ := syncCatalog(app, imageRequest(http.MethodPost, "/api/v1/image-catalog/sync", `{"tag_id":"new-tag","project":"library","repository":"base","tag":"1.0"}`, "U1"), platform.RouteSpec{})
	assertImageStatus(t, code, data, http.StatusAccepted)
	status := data.(map[string]any)
	if status["status"] != "degraded" || status["code"] != "catalog_persist_failed" || status["retryable"] != true {
		t.Fatalf("sync status = %#v, want catalog_persist_failed degraded", status)
	}
}

func TestImageRegistryHarborCatalogSyncMarksMissingCatalogDeleted(t *testing.T) {
	app := newImageRegistryTestApp(t)
	app.Adapters["harbor"] = &fakeImageHarborProxyAdapter{
		response: contracts.AdapterProxyResponse{
			StatusCode: http.StatusOK,
			Body:       []byte(`[{"digest":"sha256:abc123","repository_name":"library/base","tags":[{"name":"2.0"}]}]`),
		},
		result: contracts.AdapterResult{Adapter: "harbor", Operation: harborCatalogSyncOperation},
	}

	code, data, _ := syncCatalog(app, imageRequest(http.MethodPost, "/api/v1/image-catalog/sync", `{"tag_id":"tag-1","project":"library","repository":"base","tag":"1.0"}`, "U1"), platform.RouteSpec{})
	assertImageStatus(t, code, data, http.StatusAccepted)
	status := data.(map[string]any)
	if status["status"] != "degraded" || status["code"] != "artifact_not_found" || status["retryable"] != true {
		t.Fatalf("sync status = %#v, want artifact_not_found degraded", status)
	}

	record, found := app.Store.Get(context.Background(), imageCatalogResource, "tag-1")
	if !found {
		t.Fatal("expected catalog row to remain and be marked missing")
	}
	if record.Data["deleted"] != true || record.Data["unavailable"] != true || record.Data["status"] != "missing" {
		t.Fatalf("catalog record = %#v, want deleted=true unavailable=true status=missing", record.Data)
	}
}

func TestImageRegistryHarborCatalogSyncDegradesWhenMissingCatalogUpdateFails(t *testing.T) {
	app := newImageRegistryTestApp(t)
	app.Store = failingImageCatalogUpdateStore{RecordStore: app.Store}
	app.Adapters["harbor"] = &fakeImageHarborProxyAdapter{
		response: contracts.AdapterProxyResponse{
			StatusCode: http.StatusOK,
			Body:       []byte(`[{"digest":"sha256:abc123","repository_name":"library/base","tags":[{"name":"2.0"}]}]`),
		},
		result: contracts.AdapterResult{Adapter: "harbor", Operation: harborCatalogSyncOperation},
	}

	code, data, _ := syncCatalog(app, imageRequest(http.MethodPost, "/api/v1/image-catalog/sync", `{"tag_id":"tag-1","project":"library","repository":"base","tag":"1.0"}`, "U1"), platform.RouteSpec{})
	assertImageStatus(t, code, data, http.StatusAccepted)
	status := data.(map[string]any)
	if status["status"] != "degraded" || status["code"] != "catalog_persist_failed" || status["retryable"] != true {
		t.Fatalf("sync status = %#v, want catalog_persist_failed degraded", status)
	}
}

func TestImageRegistryHarborCatalogSyncMissingWithoutCatalogStaysDegradedOnly(t *testing.T) {
	app := newImageRegistryTestApp(t)
	app.Adapters["harbor"] = &fakeImageHarborProxyAdapter{
		response: contracts.AdapterProxyResponse{
			StatusCode: http.StatusOK,
			Body:       []byte(`[{"digest":"sha256:abc123","repository_name":"library/base","tags":[{"name":"2.0"}]}]`),
		},
		result: contracts.AdapterResult{Adapter: "harbor", Operation: harborCatalogSyncOperation},
	}

	code, data, _ := syncCatalog(app, imageRequest(http.MethodPost, "/api/v1/image-catalog/sync", `{"tag_id":"unknown-tag","project":"library","repository":"base","tag":"1.0"}`, "U1"), platform.RouteSpec{})
	assertImageStatus(t, code, data, http.StatusAccepted)
	status := data.(map[string]any)
	if status["status"] != "degraded" || status["code"] != "artifact_not_found" || status["retryable"] != true {
		t.Fatalf("sync status = %#v, want artifact_not_found degraded", status)
	}

	if _, found := app.Store.Get(context.Background(), imageCatalogResource, "unknown-tag"); found {
		t.Fatalf("unexpected catalog row for missing tag")
	}
}

func TestImageRegistryHarborSyncMaintenanceOwnerAndRetry(t *testing.T) {
	other := platform.NewApp(platform.Config{ServiceName: "workload-service", HTTPAddr: ":0"})
	Register(other)
	if containsImageTask(other.MaintenanceTaskNames(), harborCatalogSyncTaskName) {
		t.Fatalf("unowned maintenance tasks = %v, want no %s", other.MaintenanceTaskNames(), harborCatalogSyncTaskName)
	}

	app := newImageRegistryTestApp(t)
	if !containsImageTask(app.MaintenanceTaskNames(), harborCatalogSyncTaskName) {
		t.Fatalf("maintenance tasks = %v, want %s", app.MaintenanceTaskNames(), harborCatalogSyncTaskName)
	}
	app.Adapters["harbor"] = &fakeImageHarborProxyAdapter{
		response: contracts.AdapterProxyResponse{
			StatusCode: http.StatusOK,
			Body:       []byte(`[{"digest":"sha256:retry","repository_name":"library/base","tags":[{"name":"1.0"}],"scan_overview":{"report":{"scan_status":"Success"}}}]`),
		},
		result: contracts.AdapterResult{Adapter: "harbor", Operation: harborCatalogSyncOperation},
	}
	createImageRecords(t, app, imageSyncResource, []map[string]any{{"id": "tag-1", "tag_id": "tag-1", "status": "sync_requested"}})

	app.RunMaintenanceOnce(context.Background(), time.Second)
	record, found := app.Store.Get(context.Background(), imageSyncResource, "tag-1")
	if !found || record.Data["status"] != "synced" {
		t.Fatalf("sync status record = %#v found=%v, want synced", record.Data, found)
	}
	catalog, found := app.Store.Get(context.Background(), imageCatalogResource, "tag-1")
	if !found || catalog.Data["digest"] != "sha256:retry" {
		t.Fatalf("catalog record = %#v found=%v, want retry digest", catalog.Data, found)
	}
}

func TestImageRegistryHarborSyncMaintenanceSkipsNonRetryableDegraded(t *testing.T) {
	app := newImageRegistryTestApp(t)
	adapter := &fakeImageHarborProxyAdapter{
		response: contracts.AdapterProxyResponse{
			StatusCode: http.StatusOK,
			Body:       []byte(`[{"digest":"sha256:retry","repository_name":"library/base","tags":[{"name":"1.0"}]}]`),
		},
		result: contracts.AdapterResult{Adapter: "harbor", Operation: harborCatalogSyncOperation},
	}
	app.Adapters["harbor"] = adapter
	createImageRecords(t, app, imageSyncResource, []map[string]any{{
		"id":         "terminal",
		"tag_id":     "terminal",
		"project":    "library",
		"repository": "base",
		"tag":        "1.0",
		"status":     "degraded",
		"retryable":  false,
	}})

	app.RunMaintenanceOnce(context.Background(), time.Second)
	if len(adapter.requests) != 0 {
		t.Fatalf("proxy requests = %#v, want non-retryable degraded status skipped", adapter.requests)
	}
	record, found := app.Store.Get(context.Background(), imageSyncResource, "terminal")
	if !found || record.Data["status"] != "degraded" {
		t.Fatalf("sync status record = %#v found=%v, want unchanged degraded", record.Data, found)
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
	if data.(map[string]any)["status"] != "degraded" {
		t.Fatalf("sync status = %#v, want degraded without Harbor adapter", data)
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

func assertImageNoDegraded(t *testing.T, degraded *platform.Degraded) {
	t.Helper()
	if degraded != nil {
		t.Fatalf("degraded = %#v, want nil", degraded)
	}
}

func assertHarborDegraded(t *testing.T, degraded *platform.Degraded) {
	t.Helper()
	if degraded == nil || degraded.Adapter != "harbor" || degraded.Code != "adapter_unavailable" || !degraded.Retryable {
		t.Fatalf("degraded = %#v, want retryable harbor adapter_unavailable", degraded)
	}
}

func assertProjectImagePayload(t *testing.T, data any, tagID string) {
	t.Helper()
	images := data.([]map[string]any)
	if len(images) != 1 || images[0]["tag_id"] != tagID || images[0]["project_id"] != "P1" {
		t.Fatalf("project images = %#v, want one unchanged image for tag %s", images, tagID)
	}
}

func assertProjectBuildPayload(t *testing.T, data any, buildID string) {
	t.Helper()
	builds := data.([]map[string]any)
	if len(builds) != 1 || builds[0]["id"] != buildID || builds[0]["project_id"] != "P1" || builds[0]["status"] != "queued" {
		t.Fatalf("project builds = %#v, want one unchanged queued build %s", builds, buildID)
	}
}

func assertImageQueryValue(t *testing.T, rawQuery, key, want string) {
	t.Helper()
	values, err := url.ParseQuery(rawQuery)
	if err != nil {
		t.Fatalf("query %q parse failed: %v", rawQuery, err)
	}
	if got := values.Get(key); got != want {
		t.Fatalf("query %q %s = %q, want %q", rawQuery, key, got, want)
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

func mustImageJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json marshal failed: %v", err)
	}
	return data
}

type fakeImageHarborProxyAdapter struct {
	response  contracts.AdapterProxyResponse
	responses []contracts.AdapterProxyResponse
	result    contracts.AdapterResult
	err       error
	requests  []contracts.AdapterProxyRequest
}

type failingImageCatalogCreateStore struct {
	platform.RecordStore
}

type failingImageCatalogUpdateStore struct {
	platform.RecordStore
}

func (s failingImageCatalogCreateStore) Create(ctx context.Context, resource string, data map[string]any) (contracts.Record[map[string]any], error) {
	if resource == imageCatalogResource {
		return contracts.Record[map[string]any]{}, errors.New("catalog create blocked")
	}
	return s.RecordStore.Create(ctx, resource, data)
}

func (s failingImageCatalogUpdateStore) Update(ctx context.Context, resource, id string, data map[string]any) (contracts.Record[map[string]any], bool) {
	if resource == imageCatalogResource {
		return contracts.Record[map[string]any]{}, false
	}
	return s.RecordStore.Update(ctx, resource, id, data)
}

func (f *fakeImageHarborProxyAdapter) Call(context.Context, string, bool) (contracts.AdapterResult, error) {
	return contracts.AdapterResult{Adapter: "harbor"}, nil
}

func (f *fakeImageHarborProxyAdapter) Proxy(_ context.Context, req contracts.AdapterProxyRequest) (contracts.AdapterProxyResponse, contracts.AdapterResult, error) {
	f.requests = append(f.requests, req)
	if len(f.responses) >= len(f.requests) {
		return f.responses[len(f.requests)-1], f.result, f.err
	}
	return f.response, f.result, f.err
}

func containsImageTask(tasks []string, want string) bool {
	for _, task := range tasks {
		if task == want {
			return true
		}
	}
	return false
}
