package imageregistry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
	storageservice "github.com/linskybing/nexuspaas/backend/internal/services/storage"
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

	buildReq := imageRequest(http.MethodPost, "/api/v1/images/build/dockerfile", imageBuildBody("build-1", "P1", "registry.local/team/app:dev"), "U1")
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

func TestImageCatalogPublishRequiresSupplyChainReadyCatalog(t *testing.T) {
	cases := []struct {
		name string
		row  map[string]any
	}{
		{
			name: "missing digest",
			row:  map[string]any{"id": "tag-missing-digest", "registry": "registry.local", "repository": "library/base", "tag": "1.0", "scan_status": "Success"},
		},
		{
			name: "missing scan",
			row:  map[string]any{"id": "tag-missing-scan", "registry": "registry.local", "repository": "library/base", "tag": "1.0", "digest": "sha256:missing"},
		},
		{
			name: "pending scan",
			row:  map[string]any{"id": "tag-pending-scan", "registry": "registry.local", "repository": "library/base", "tag": "1.0", "digest": "sha256:pending", "scan_status": "pending"},
		},
		{
			name: "failed scan",
			row:  map[string]any{"id": "tag-failed-scan", "registry": "registry.local", "repository": "library/base", "tag": "1.0", "digest": "sha256:failed", "scan_status": "Failed"},
		},
		{
			name: "deleted",
			row:  map[string]any{"id": "tag-deleted", "registry": "registry.local", "repository": "library/base", "tag": "1.0", "digest": "sha256:deleted", "scan_status": "Success", "deleted": true},
		},
		{
			name: "unavailable",
			row:  map[string]any{"id": "tag-unavailable", "registry": "registry.local", "repository": "library/base", "tag": "1.0", "digest": "sha256:unavailable", "scan_status": "Success", "unavailable": "true"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app := newImageRegistryTestApp(t)
			createImageRecords(t, app, imageCatalogResource, []map[string]any{tc.row})
			req := imageRequest(http.MethodPost, "/api/v1/image-catalog/publish", fmt.Sprintf(`{"tag_id":%q,"project_id":"P1"}`, tc.row["id"]), "ADMIN")

			code, data, _ := publishCatalog(app, req, platform.RouteSpec{})
			assertImageStatus(t, code, data, http.StatusConflict)
			if data.(map[string]any)["message"] == "" {
				t.Fatalf("publish rejection = %#v, want message", data)
			}
			if records := imageRows(app, req, projectImagesResource); len(records) != 0 {
				t.Fatalf("allow-list records = %#v, want none", records)
			}
			if events := imageEventsByName(app, "ImagePublished"); len(events) != 0 {
				t.Fatalf("ImagePublished events = %#v, want none", events)
			}
		})
	}
}

func TestImageCatalogPublishAllowsPassingSupplyChainCatalog(t *testing.T) {
	app := newImageRegistryTestApp(t)

	req := imageRequest(http.MethodPost, "/api/v1/image-catalog/publish", `{"tag_id":"tag-1","project_id":"P1"}`, "ADMIN")
	code, data, _ := publishCatalog(app, req, platform.RouteSpec{})
	assertImageStatus(t, code, data, http.StatusOK)

	rules := data.(map[string]any)["rules"].([]map[string]any)
	if len(rules) != 1 {
		t.Fatalf("publish result = %#v, want one rule", data)
	}
	if rules[0]["digest"] != "sha256:base" || rules[0]["scan_status"] != "Success" {
		t.Fatalf("published rule supply-chain metadata = %#v, want digest and passing scan", rules[0])
	}
	if events := imageEventsByName(app, "ImagePublished"); len(events) != 1 {
		t.Fatalf("ImagePublished events = %#v, want one", events)
	}
}

// Provenance enforcement is opt-in via IMAGE_PUBLISH_REQUIRE_PROVENANCE. When on,
// publish additionally requires SBOM-digest + signature-ref presence (on top of
// the existing digest + passing-scan checks). Default-off behavior is covered by
// TestImageCatalogPublishAllowsPassingSupplyChainCatalog above.
func TestImageCatalogPublishProvenanceEnforcement(t *testing.T) {
	t.Run("rejects missing SBOM", func(t *testing.T) {
		code, data, app := publishImageCatalogForProvenanceTest(t, true, imageCatalogProvenanceRow("prov-no-sbom", nil))
		assertImageStatus(t, code, data, http.StatusConflict)
		assertImageCatalogRejectionContains(t, data, "SBOM")
		assertImagePublishedEventCount(t, app, 0)
	})

	t.Run("rejects missing signature", func(t *testing.T) {
		row := imageCatalogProvenanceRow("prov-no-sig", map[string]any{"sbom_digest": "sha256:sbom"})
		code, data, app := publishImageCatalogForProvenanceTest(t, true, row)
		assertImageStatus(t, code, data, http.StatusConflict)
		assertImageCatalogRejectionContains(t, data, "signature")
		assertImagePublishedEventCount(t, app, 0)
	})

	t.Run("accepts full provenance and promotes refs", func(t *testing.T) {
		row := imageCatalogProvenanceRow("prov-ok", map[string]any{"sbom_digest": "sha256:sbom", "signature": "sigstore://sig"})
		code, data, app := publishImageCatalogForProvenanceTest(t, true, row)
		assertImageStatus(t, code, data, http.StatusOK)
		assertPublishedRuleProvenance(t, data)
		assertImagePublishedEventCount(t, app, 1)
	})

	t.Run("flag off publishes without provenance", func(t *testing.T) {
		code, data, _ := publishImageCatalogForProvenanceTest(t, false, imageCatalogProvenanceRow("prov-off", nil))
		assertImageStatus(t, code, data, http.StatusOK)
	})
}

func imageCatalogProvenanceRow(id string, extra map[string]any) map[string]any {
	row := map[string]any{
		"id":          id,
		"registry":    "registry.local",
		"repository":  "library/base",
		"tag":         "1.0",
		"digest":      "sha256:" + id,
		"scan_status": "Success",
	}
	for k, v := range extra {
		row[k] = v
	}
	return row
}

func publishImageCatalogForProvenanceTest(t *testing.T, requireProvenance bool, row map[string]any) (int, any, *platform.App) {
	t.Helper()
	app := newImageRegistryTestApp(t)
	app.Config.ImageProvenanceRequired = requireProvenance
	createImageRecords(t, app, imageCatalogResource, []map[string]any{row})
	// Verified provenance now also requires a build record whose pipeline
	// succeeded for the digest; seed one when the catalog row carries a
	// signature (i.e. the "full provenance" cases).
	if _, ok := row["signature"]; ok {
		createImageRecords(t, app, imageBuildsResource, []map[string]any{{
			"id":               "prov-build-" + fmt.Sprint(row["id"]),
			"image_digest":     fmt.Sprint(row["digest"]),
			"status":           "succeeded",
			"sbom_status":      "succeeded",
			"scan_status":      "passed",
			"signature_status": "signed",
		}})
	}
	req := imageRequest(http.MethodPost, "/api/v1/image-catalog/publish", fmt.Sprintf(`{"tag_id":%q,"project_id":"P1"}`, row["id"]), "ADMIN")
	code, data, _ := publishCatalog(app, req, platform.RouteSpec{})
	return code, data, app
}

func assertImageCatalogRejectionContains(t *testing.T, data any, want string) {
	t.Helper()
	msg, _ := data.(map[string]any)["message"].(string)
	if !strings.Contains(msg, want) {
		t.Fatalf("rejection message = %q, want %s requirement", msg, want)
	}
}

func assertImagePublishedEventCount(t *testing.T, app *platform.App, want int) {
	t.Helper()
	if events := imageEventsByName(app, "ImagePublished"); len(events) != want {
		t.Fatalf("ImagePublished events = %#v, want %d", events, want)
	}
}

func assertPublishedRuleProvenance(t *testing.T, data any) {
	t.Helper()
	rules := data.(map[string]any)["rules"].([]map[string]any)
	if len(rules) != 1 {
		t.Fatalf("publish result = %#v, want one rule", data)
	}
	if rules[0]["sbom_digest"] != "sha256:sbom" || rules[0]["signature"] != "sigstore://sig" {
		t.Fatalf("published rule provenance = %#v, want sbom_digest and signature promoted", rules[0])
	}
}

func TestImageBuildSupplyChainDefaultsRecordedAndEmitted(t *testing.T) {
	app := newImageRegistryTestApp(t)

	code, data, _ := startDockerfileImageBuild(app, imageRequest(http.MethodPost, "/api/v1/images/build/dockerfile", imageBuildBody("supply-chain-build", "P1", "registry.local/team/app:supply-chain"), "U1"), platform.RouteSpec{})
	assertImageStatus(t, code, data, http.StatusAccepted)
	assertImageBuildSupplyChainDefaults(t, data)

	record, found := app.Store.Get(context.Background(), imageBuildsResource, "supply-chain-build")
	if !found {
		t.Fatal("supply-chain build record missing")
	}
	assertImageBuildSupplyChainDefaults(t, record.Data)

	events := imageEventsByName(app, "ImageBuildStarted")
	if len(events) != 1 {
		t.Fatalf("ImageBuildStarted events = %#v, want one", events)
	}
	assertImageBuildSupplyChainDefaults(t, events[0].Data)
}

func TestImageBuildListingToleratesHistoricalSupplyChainFieldsMissing(t *testing.T) {
	app := newImageRegistryTestApp(t)
	createImageRecords(t, app, imageBuildsResource, []map[string]any{
		{
			"id":                     "historical-build",
			"build_id":               "historical-build",
			"job_name":               "historical-build",
			"project_id":             "P1",
			"image_reference":        "registry.local/team/app:historical",
			"build_type":             "dockerfile",
			"cpu_cores":              1.0,
			"memory_gib":             2.0,
			"max_build_time_seconds": 300,
			"status":                 "queued",
		},
	})

	code, data, _ := listProjectBuilds(app, imageProjectRequest(http.MethodGet, "/api/v1/projects/P1/builds", "", "U2", "P1"), platform.RouteSpec{})
	assertImageStatus(t, code, data, http.StatusOK)
	assertProjectBuildPayload(t, data, "historical-build")
}

func TestImageBuildLogsRedactCommonSecrets(t *testing.T) {
	app := newImageRegistryTestApp(t)
	secrets := []struct {
		name  string
		value string
	}{
		{name: "authorization bearer", value: "synthetic-bearer-value-redact-me"},
		{name: "password", value: "synthetic-password-value-redact-me"},
		{name: "passwd", value: "synthetic-passwd-value-redact-me"},
		{name: "token", value: "synthetic-token-value-redact-me"},
		{name: "api key", value: "synthetic-api-key-value-redact-me"},
		{name: "access key", value: "synthetic-access-key-value-redact-me"},
		{name: "private key", value: "synthetic-private-key-value-redact-me"},
		{name: "credential", value: "synthetic-credential-value-redact-me"},
	}
	originalLogs := strings.Join([]string{
		"step 1: pulling base image",
		"Authorization: Bearer " + secrets[0].value,
		"password=" + secrets[1].value,
		"db_passwd: '" + secrets[2].value + "'",
		"registry_token=" + secrets[3].value,
		`api_key="` + secrets[4].value + `"`,
		"access_key: " + secrets[5].value,
		"private_key=" + secrets[6].value,
		"credential: " + secrets[7].value,
		"non-secret line remains visible",
	}, "\n") + "\n"
	createImageRecords(t, app, imageBuildsResource, []map[string]any{
		{
			"id":              "redact-build",
			"build_id":        "redact-build",
			"job_name":        "redact-build",
			"project_id":      "P1",
			"image_reference": "registry.local/team/app:redact",
			"build_type":      "dockerfile",
			"status":          "running",
			"logs":            originalLogs,
		},
	})

	req := imageRequest(http.MethodGet, "/api/v1/images/build/redact-build/logs", "", "U2")
	req.SetPathValue("buildId", "redact-build")
	code, data, _ := getBuildLogs(app, req, platform.RouteSpec{})
	assertImageStatus(t, code, data, http.StatusOK)
	raw, ok := data.(platform.RawResponse)
	if !ok {
		t.Fatalf("build logs response type = %T, want platform.RawResponse", data)
	}
	if raw.ContentType != "text/plain; charset=utf-8" {
		t.Fatalf("build logs content type = %q, want text/plain; charset=utf-8", raw.ContentType)
	}
	body := string(raw.Body)
	for _, secret := range secrets {
		if strings.Contains(body, secret.value) {
			t.Fatalf("redacted build logs leaked %s value", secret.name)
		}
	}
	if count := strings.Count(body, imageBuildLogRedaction); count < len(secrets) {
		t.Fatalf("redacted build logs placeholders = %d, want at least %d", count, len(secrets))
	}
	if !strings.Contains(body, "Authorization: Bearer "+imageBuildLogRedaction) {
		t.Fatalf("redacted build logs missing authorization bearer placeholder")
	}
	if !strings.Contains(body, `api_key="`+imageBuildLogRedaction+`"`) {
		t.Fatalf("redacted build logs missing quoted key/value placeholder")
	}
	if !strings.Contains(body, "non-secret line remains visible") {
		t.Fatalf("redacted build logs removed ordinary log line")
	}

	stored, found := app.Store.Get(context.Background(), imageBuildsResource, "redact-build")
	if !found {
		t.Fatalf("stored build missing")
	}
	storedLogs, _ := stored.Data["logs"].(string)
	if storedLogs != originalLogs {
		t.Fatalf("stored build logs were mutated")
	}
	for _, secret := range secrets {
		if !strings.Contains(storedLogs, secret.value) {
			t.Fatalf("stored build logs lost original %s value", secret.name)
		}
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
	buildReq := imageRequest(http.MethodPost, "/api/v1/images/build", imageBuildBody("build-harbor-ok", "P1", "registry.local/team/app:ok"), "U1")
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

	degradedBuildReq := imageRequest(http.MethodPost, "/api/v1/images/build/dockerfile", imageBuildBody("build-harbor-down", "P1", "registry.local/team/app:down"), "U1")
	code, data, degraded = startDockerfileImageBuild(app, degradedBuildReq, buildRoute)
	assertImageStatus(t, code, data, http.StatusAccepted)
	assertHarborDegraded(t, degraded)
	assertImageMapValue(t, data, "id", "build-harbor-down")
	assertImageMapValue(t, data, "build_type", "dockerfile")
	assertImageMapValue(t, data, "status", "queued")

	storageBuildRoute := platform.RouteSpec{Method: http.MethodPost, OperationID: "imageBuildFromStorage"}
	storageBuildReq := imageRequest(http.MethodPost, "/api/v1/images/build/from-storage", imageStorageBuildBody("build-harbor-storage-down", "P1", "registry.local/team/app:storage-down"), "U1")
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

	contextBuild := imageRequest(http.MethodPost, "/api/v1/images/build", `{"id":"ctx-build","project_id":"P1","repository":"team/app","tag":"ctx","cpu":2,"memory_gb":4,"max_build_time_seconds":600}`, "U1")
	code, data, _ = startImageBuild(app, contextBuild, platform.RouteSpec{})
	assertImageStatus(t, code, data, http.StatusAccepted)
	assertImageMapValue(t, data, "build_type", "context")

	storageBuild := imageRequest(http.MethodPost, "/api/v1/images/build/from-storage", imageStorageBuildBody("storage-build", "P1", "registry.local/team/app:storage"), "U1")
	code, data, _ = startStorageImageBuild(app, storageBuild, platform.RouteSpec{})
	assertImageStatus(t, code, data, http.StatusAccepted)
	assertImageMapValue(t, data, "build_type", "storage")
}

func TestImageBuildLocalQuotaTimeoutAndConcurrencyPolicy(t *testing.T) {
	t.Run("requires build resources", func(t *testing.T) {
		cases := []struct {
			name string
			body string
		}{
			{name: "missing cpu", body: `{"id":"missing-cpu","project_id":"P1","image_reference":"registry.local/team/app:missing-cpu","memory_gib":4,"max_build_seconds":600}`},
			{name: "zero memory", body: `{"id":"zero-memory","project_id":"P1","image_reference":"registry.local/team/app:zero-memory","cpu_cores":2,"memory_gib":0,"max_build_seconds":600}`},
			{name: "negative time", body: `{"id":"negative-time","project_id":"P1","image_reference":"registry.local/team/app:negative-time","cpu_cores":2,"memory_gib":4,"max_build_seconds":-1}`},
			{name: "nonnumeric cpu", body: `{"id":"bad-cpu","project_id":"P1","image_reference":"registry.local/team/app:bad-cpu","cpu_cores":"fast","memory_gib":4,"max_build_seconds":600}`},
		}
		for _, tt := range cases {
			t.Run(tt.name, func(t *testing.T) {
				app := newImageRegistryTestApp(t)
				code, data, _ := startDockerfileImageBuild(app, imageRequest(http.MethodPost, "/api/v1/images/build/dockerfile", tt.body, "U1"), platform.RouteSpec{})
				assertImageStatus(t, code, data, http.StatusBadRequest)
				if got := len(app.Store.List(context.Background(), imageBuildsResource)); got != 0 {
					t.Fatalf("build records = %d, want none after rejected resource request", got)
				}
			})
		}
	})

	t.Run("requires project opt-in", func(t *testing.T) {
		app := newImageRegistryTestApp(t)
		updateImageProject(t, app, "P1", map[string]any{"allow_image_build": false})
		code, data, _ := startDockerfileImageBuild(app, imageRequest(http.MethodPost, "/api/v1/images/build/dockerfile", imageBuildBody("disabled", "P1", "registry.local/team/app:disabled"), "U1"), platform.RouteSpec{})
		assertImageStatus(t, code, data, http.StatusForbidden)

		createImageRecords(t, app, orgProjectsResource, []map[string]any{
			{"id": "P2", "project_name": "missing-build-policy", "owner_id": "G1"},
		})
		code, data, _ = startDockerfileImageBuild(app, imageRequest(http.MethodPost, "/api/v1/images/build/dockerfile", imageBuildBody("missing", "P2", "registry.local/team/app:missing"), "U1"), platform.RouteSpec{})
		assertImageStatus(t, code, data, http.StatusForbidden)
	})

	t.Run("enforces project limits", func(t *testing.T) {
		cases := []struct {
			name   string
			update map[string]any
			body   string
		}{
			{name: "cpu", update: map[string]any{"build_cpu_limit": 1.5}, body: imageBuildBody("cpu-limit", "P1", "registry.local/team/app:cpu-limit")},
			{name: "memory", update: map[string]any{"build_memory_gib_limit": 3.5}, body: imageBuildBody("memory-limit", "P1", "registry.local/team/app:memory-limit")},
			{name: "time", update: map[string]any{"build_time_limit_seconds": 300}, body: imageBuildBody("time-limit", "P1", "registry.local/team/app:time-limit")},
		}
		for _, tt := range cases {
			t.Run(tt.name, func(t *testing.T) {
				app := newImageRegistryTestApp(t)
				updateImageProject(t, app, "P1", tt.update)
				code, data, _ := startDockerfileImageBuild(app, imageRequest(http.MethodPost, "/api/v1/images/build/dockerfile", tt.body, "U1"), platform.RouteSpec{})
				assertImageStatus(t, code, data, http.StatusConflict)
			})
		}
	})

	t.Run("enforces active same-project concurrency only", func(t *testing.T) {
		app := newImageRegistryTestApp(t)
		updateImageProject(t, app, "P1", map[string]any{"max_running_builds": 10, "max_concurrent_builds": 4})
		seedImageBuildStatuses(t, app, "P1", "queued", "pending", "running", "building", "cancelled", "failed", "completed", "timed_out")
		seedImageBuildStatuses(t, app, "other-project", "running")
		code, data, _ := startDockerfileImageBuild(app, imageRequest(http.MethodPost, "/api/v1/images/build/dockerfile", imageBuildBody("blocked-concurrency", "P1", "registry.local/team/app:blocked"), "U1"), platform.RouteSpec{})
		assertImageStatus(t, code, data, http.StatusConflict)

		open := newImageRegistryTestApp(t)
		updateImageProject(t, open, "P1", map[string]any{"max_running_builds": 1})
		seedImageBuildStatuses(t, open, "P1", "cancelled", "canceled", "failed", "succeeded", "completed", "timed_out", "rejected")
		code, data, _ = startDockerfileImageBuild(open, imageRequest(http.MethodPost, "/api/v1/images/build/dockerfile", imageBuildBody("accepted-after-terminal", "P1", "registry.local/team/app:accepted"), "U1"), platform.RouteSpec{})
		assertImageStatus(t, code, data, http.StatusAccepted)
		assertImageMapValue(t, data, "status", "queued")
		assertImageMapValue(t, data, "cpu_cores", 2.0)
		assertImageMapValue(t, data, "memory_gib", 4.0)
		assertImageMapValue(t, data, "max_build_time_seconds", 600)
	})
}

func TestImageBuildCancelAndTimeoutReleaseActiveBuildSlotLocally(t *testing.T) {
	t.Run("cancel releases active build slot", assertImageBuildCancelReleasesActiveBuildSlotLocally)
	t.Run("timeout terminal statuses release active build slot", assertImageBuildTimeoutStatusesReleaseActiveBuildSlotLocally)
}

func assertImageBuildCancelReleasesActiveBuildSlotLocally(t *testing.T) {
	app := newImageRegistryTestApp(t)
	updateImageProject(t, app, "P1", map[string]any{"max_running_builds": 1})

	code, data, _ := startDockerfileImageBuild(app, imageRequest(http.MethodPost, "/api/v1/images/build/dockerfile", imageBuildBody("cancel-slot-first", "P1", "registry.local/team/app:cancel-first"), "U1"), platform.RouteSpec{})
	assertImageStatus(t, code, data, http.StatusAccepted)
	assertImageMapValue(t, data, "id", "cancel-slot-first")
	assertImageMapValue(t, data, "status", "queued")

	code, data, _ = startDockerfileImageBuild(app, imageRequest(http.MethodPost, "/api/v1/images/build/dockerfile", imageBuildBody("cancel-slot-blocked", "P1", "registry.local/team/app:cancel-blocked"), "U1"), platform.RouteSpec{})
	assertImageStatus(t, code, data, http.StatusConflict)

	cancelReq := imageProjectRequest(http.MethodDelete, "/api/v1/projects/P1/builds/cancel-slot-first", "", "U1", "P1")
	cancelReq.SetPathValue("jobName", "cancel-slot-first")
	code, data, _ = cancelProjectBuild(app, cancelReq, platform.RouteSpec{})
	assertImageStatus(t, code, data, http.StatusOK)
	assertImageMapValue(t, data, "id", "cancel-slot-first")
	assertImageMapValue(t, data, "status", "cancelled")

	stored, found := app.Store.Get(context.Background(), imageBuildsResource, "cancel-slot-first")
	if !found {
		t.Fatalf("cancelled build record missing")
	}
	if status, _ := stored.Data["status"].(string); status != "cancelled" {
		t.Fatalf("stored cancelled build status = %q, want cancelled", status)
	}

	cancelEvents := imageEventsByName(app, "ImageBuildCancelled")
	if len(cancelEvents) != 1 {
		t.Fatalf("ImageBuildCancelled events = %#v, want exactly one", cancelEvents)
	}
	if cancelEvents[0].Data["id"] != "cancel-slot-first" && cancelEvents[0].Data["build_id"] != "cancel-slot-first" {
		t.Fatalf("ImageBuildCancelled event data = %#v, want cancelled build id", cancelEvents[0].Data)
	}

	code, data, _ = startDockerfileImageBuild(app, imageRequest(http.MethodPost, "/api/v1/images/build/dockerfile", imageBuildBody("cancel-slot-after", "P1", "registry.local/team/app:cancel-after"), "U1"), platform.RouteSpec{})
	assertImageStatus(t, code, data, http.StatusAccepted)
	assertImageMapValue(t, data, "id", "cancel-slot-after")
	assertImageMapValue(t, data, "status", "queued")
}

func assertImageBuildTimeoutStatusesReleaseActiveBuildSlotLocally(t *testing.T) {
	app := newImageRegistryTestApp(t)
	updateImageProject(t, app, "P1", map[string]any{"max_running_builds": 1})
	seedImageBuildStatuses(t, app, "P1", "timed_out", "timeout")
	terminalStatuses := map[string]string{
		"P1-build-00-timed_out": "timed_out",
		"P1-build-01-timeout":   "timeout",
	}

	code, data, _ := startDockerfileImageBuild(app, imageRequest(http.MethodPost, "/api/v1/images/build/dockerfile", imageBuildBody("timeout-slot-after", "P1", "registry.local/team/app:timeout-after"), "U1"), platform.RouteSpec{})
	assertImageStatus(t, code, data, http.StatusAccepted)
	assertImageMapValue(t, data, "id", "timeout-slot-after")
	assertImageMapValue(t, data, "status", "queued")

	for id, want := range terminalStatuses {
		record, found := app.Store.Get(context.Background(), imageBuildsResource, id)
		if !found {
			t.Fatalf("terminal build record %s missing", id)
		}
		if status, _ := record.Data["status"].(string); status != want {
			t.Fatalf("terminal build %s status = %q, want %q", id, status, want)
		}
	}
}

func TestImageBuildIdempotencyKeyReplaysSameRequest(t *testing.T) {
	app := newImageRegistryTestApp(t)
	updateImageProject(t, app, "P1", map[string]any{"max_running_builds": 1})
	key := "image-build-idempotency-key"
	body := imageBuildBody("idem-build", "P1", "registry.local/team/app:idem")

	createReq := imageRequest(http.MethodPost, "/api/v1/images/build/dockerfile", body, "U1")
	createReq.Header.Set(idempotencyKeyHeader, key)
	code, data, _ := startDockerfileImageBuild(app, createReq, platform.RouteSpec{})
	assertImageStatus(t, code, data, http.StatusAccepted)
	assertImageMapValue(t, data, "id", "idem-build")
	assertImageMapValue(t, data, "status", "queued")
	keyHash, fingerprintHash := storedImageBuildIdempotencyHashes(t, app, "idem-build")
	assertNoImageIdempotencyMaterial(t, data, key, keyHash, fingerprintHash)

	replayReq := imageRequest(http.MethodPost, "/api/v1/images/build/dockerfile", body, "U1")
	replayReq.Header.Set(idempotencyKeyHeader, key)
	code, data, _ = startDockerfileImageBuild(app, replayReq, platform.RouteSpec{})
	assertImageStatus(t, code, data, http.StatusAccepted)
	assertImageMapValue(t, data, "id", "idem-build")
	assertImageMapValue(t, data, "status", "queued")
	assertNoImageIdempotencyMaterial(t, data, key, keyHash, fingerprintHash)

	if records := app.Store.List(context.Background(), imageBuildsResource); len(records) != 1 {
		t.Fatalf("image build records = %d, want one idempotent build", len(records))
	}
	events := imageEventsByName(app, "ImageBuildStarted")
	if len(events) != 1 {
		t.Fatalf("ImageBuildStarted events = %#v, want one", events)
	}
	if events[0].IdempotencyKey != key {
		t.Fatalf("ImageBuildStarted IdempotencyKey = %q, want synthetic test key", events[0].IdempotencyKey)
	}
	assertNoImageIdempotencyMaterial(t, events[0].Data, key, keyHash, fingerprintHash)

	listReq := imageProjectRequest(http.MethodGet, "/api/v1/projects/P1/builds", "", "U2", "P1")
	code, data, _ = listProjectBuilds(app, listReq, platform.RouteSpec{})
	assertImageStatus(t, code, data, http.StatusOK)
	assertProjectBuildPayload(t, data, "idem-build")
	assertNoImageIdempotencyMaterial(t, data, key, keyHash, fingerprintHash)

	cancelReq := imageProjectRequest(http.MethodDelete, "/api/v1/projects/P1/builds/idem-build", "", "U1", "P1")
	cancelReq.SetPathValue("jobName", "idem-build")
	code, data, _ = cancelProjectBuild(app, cancelReq, platform.RouteSpec{})
	assertImageStatus(t, code, data, http.StatusOK)
	assertImageMapValue(t, data, "status", "cancelled")
	assertNoImageIdempotencyMaterial(t, data, key, keyHash, fingerprintHash)
}

func TestImageBuildIdempotencyKeyRejectsDifferentPayload(t *testing.T) {
	app := newImageRegistryTestApp(t)
	key := "image-build-conflict-key"
	firstReq := imageRequest(http.MethodPost, "/api/v1/images/build/dockerfile", imageBuildBody("idem-conflict", "P1", "registry.local/team/app:first"), "U1")
	firstReq.Header.Set(idempotencyKeyHeader, key)
	code, data, _ := startDockerfileImageBuild(app, firstReq, platform.RouteSpec{})
	assertImageStatus(t, code, data, http.StatusAccepted)
	assertImageMapValue(t, data, "id", "idem-conflict")

	secondReq := imageRequest(http.MethodPost, "/api/v1/images/build/dockerfile", imageBuildBody("idem-conflict", "P1", "registry.local/team/app:second"), "U1")
	secondReq.Header.Set(idempotencyKeyHeader, key)
	code, data, _ = startDockerfileImageBuild(app, secondReq, platform.RouteSpec{})
	assertImageStatus(t, code, data, http.StatusConflict)

	if records := app.Store.List(context.Background(), imageBuildsResource); len(records) != 1 {
		t.Fatalf("image build records = %d, want no duplicate after idempotency conflict", len(records))
	}
	if events := imageEventsByName(app, "ImageBuildStarted"); len(events) != 1 {
		t.Fatalf("ImageBuildStarted events = %#v, want one after idempotency conflict", events)
	}
}

// P0-2: the same Idempotency-Key must 409 when only the build source changes,
// even though project/image-reference/resources are identical.
func TestImageBuildIdempotencyKeyRejectsDifferentSource(t *testing.T) {
	body := func(dockerfile, buildArgs string) string {
		return fmt.Sprintf(`{"id":"src-idem","project_id":"P1","image_reference":"registry.local/team/app:src","cpu_cores":2,"memory_gib":4,"max_build_seconds":600,"dockerfile":%q,"context":".","build_args":%s}`, dockerfile, buildArgs)
	}
	cases := []struct {
		name        string
		first, next string
	}{
		{"different dockerfile", body("FROM alpine", `{"A":"1"}`), body("FROM ubuntu", `{"A":"1"}`)},
		{"different build args", body("FROM alpine", `{"A":"1"}`), body("FROM alpine", `{"A":"2"}`)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app := newImageRegistryTestApp(t)
			const key = "image-build-source-key"
			firstReq := imageRequest(http.MethodPost, "/api/v1/images/build/dockerfile", tc.first, "U1")
			firstReq.Header.Set(idempotencyKeyHeader, key)
			code, data, _ := startDockerfileImageBuild(app, firstReq, platform.RouteSpec{})
			assertImageStatus(t, code, data, http.StatusAccepted)

			nextReq := imageRequest(http.MethodPost, "/api/v1/images/build/dockerfile", tc.next, "U1")
			nextReq.Header.Set(idempotencyKeyHeader, key)
			code, data, _ = startDockerfileImageBuild(app, nextReq, platform.RouteSpec{})
			assertImageStatus(t, code, data, http.StatusConflict)

			if records := app.Store.List(context.Background(), imageBuildsResource); len(records) != 1 {
				t.Fatalf("image build records = %d, want no duplicate after source-change conflict", len(records))
			}
		})
	}
}

// P0-2: from-storage builds are fingerprinted by storage_path — a replay with a
// different source path is a 409, an identical one replays.
func TestImageBuildIdempotencyKeyFingerprintsStorageSource(t *testing.T) {
	app := newImageRegistryTestApp(t)
	const key = "image-build-storage-key"
	body := func(path string) string {
		return fmt.Sprintf(`{"id":"stor-idem","project_id":"P1","image_reference":"registry.local/team/app:stor","cpu_cores":2,"memory_gib":4,"max_build_seconds":600,"storage_path":%q}`, path)
	}
	firstReq := imageRequest(http.MethodPost, "/api/v1/images/build/from-storage", body("images/a.tar"), "U1")
	firstReq.Header.Set(idempotencyKeyHeader, key)
	code, data, _ := startStorageImageBuild(app, firstReq, platform.RouteSpec{})
	assertImageStatus(t, code, data, http.StatusAccepted)

	replayReq := imageRequest(http.MethodPost, "/api/v1/images/build/from-storage", body("images/a.tar"), "U1")
	replayReq.Header.Set(idempotencyKeyHeader, key)
	code, data, _ = startStorageImageBuild(app, replayReq, platform.RouteSpec{})
	assertImageStatus(t, code, data, http.StatusAccepted)

	conflictReq := imageRequest(http.MethodPost, "/api/v1/images/build/from-storage", body("images/b.tar"), "U1")
	conflictReq.Header.Set(idempotencyKeyHeader, key)
	code, data, _ = startStorageImageBuild(app, conflictReq, platform.RouteSpec{})
	assertImageStatus(t, code, data, http.StatusConflict)

	if records := app.Store.List(context.Background(), imageBuildsResource); len(records) != 1 {
		t.Fatalf("image build records = %d, want one after storage-source fingerprinting", len(records))
	}
}

func TestImageBuildCancelIdempotencyKeyReplaysSameTarget(t *testing.T) {
	app := newImageRegistryTestApp(t)
	key := "image-build-cancel-idempotency-key"

	createReq := imageRequest(http.MethodPost, "/api/v1/images/build/dockerfile", imageBuildBody("cancel-idem", "P1", "registry.local/team/app:cancel-idem"), "U1")
	code, data, _ := startDockerfileImageBuild(app, createReq, platform.RouteSpec{})
	assertImageStatus(t, code, data, http.StatusAccepted)
	assertImageMapValue(t, data, "id", "cancel-idem")
	if _, ok := app.Store.Update(context.Background(), imageBuildsResource, "cancel-idem", map[string]any{"job_name": "cancel-idem-job", "build_id": "cancel-idem-build"}); !ok {
		t.Fatalf("build cancel-idem alias update failed")
	}

	cancelReq := imageProjectRequest(http.MethodDelete, "/api/v1/projects/P1/builds/cancel-idem-job", "", "U1", "P1")
	cancelReq.SetPathValue("jobName", "cancel-idem-job")
	cancelReq.Header.Set(idempotencyKeyHeader, key)
	code, data, _ = cancelProjectBuild(app, cancelReq, platform.RouteSpec{})
	assertImageStatus(t, code, data, http.StatusOK)
	assertImageMapValue(t, data, "id", "cancel-idem")
	assertImageMapValue(t, data, "status", "cancelled")
	keyHash, fingerprintHash := storedImageBuildCancelIdempotencyHashes(t, app, "cancel-idem")
	assertNoImageIdempotencyMaterial(t, data, key, keyHash, fingerprintHash)

	replayReq := imageProjectRequest(http.MethodDelete, "/api/v1/projects/P1/builds/cancel-idem-job", "", "U1", "P1")
	replayReq.SetPathValue("jobName", "cancel-idem-job")
	replayReq.Header.Set(idempotencyKeyHeader, key)
	code, data, _ = cancelProjectBuild(app, replayReq, platform.RouteSpec{})
	assertImageStatus(t, code, data, http.StatusOK)
	assertImageMapValue(t, data, "id", "cancel-idem")
	assertImageMapValue(t, data, "status", "cancelled")
	assertNoImageIdempotencyMaterial(t, data, key, keyHash, fingerprintHash)

	aliasReq := imageProjectRequest(http.MethodDelete, "/api/v1/projects/P1/image-builds/cancel-idem-build", "", "U1", "P1")
	aliasReq.SetPathValue("buildId", "cancel-idem-build")
	aliasReq.Header.Set(idempotencyKeyHeader, key)
	code, data, _ = cancelProjectBuild(app, aliasReq, platform.RouteSpec{})
	assertImageStatus(t, code, data, http.StatusOK)
	assertImageMapValue(t, data, "id", "cancel-idem")
	assertImageMapValue(t, data, "status", "cancelled")
	assertNoImageIdempotencyMaterial(t, data, key, keyHash, fingerprintHash)

	events := imageEventsByName(app, "ImageBuildCancelled")
	if len(events) != 1 {
		t.Fatalf("ImageBuildCancelled events = %#v, want one after idempotent cancel replays", events)
	}
	if events[0].IdempotencyKey != key {
		t.Fatalf("ImageBuildCancelled IdempotencyKey = %q, want synthetic test key", events[0].IdempotencyKey)
	}
	assertNoImageIdempotencyMaterial(t, events[0].Data, key, keyHash, fingerprintHash)

	listReq := imageProjectRequest(http.MethodGet, "/api/v1/projects/P1/builds", "", "U2", "P1")
	code, data, _ = listProjectBuilds(app, listReq, platform.RouteSpec{})
	assertImageStatus(t, code, data, http.StatusOK)
	assertNoImageIdempotencyMaterial(t, data, key, keyHash, fingerprintHash)
}

func TestImageBuildCancelIdempotencyKeyRejectsDifferentTarget(t *testing.T) {
	app := newImageRegistryTestApp(t)
	key := "image-build-cancel-conflict-key"

	firstReq := imageRequest(http.MethodPost, "/api/v1/images/build/dockerfile", imageBuildBody("cancel-conflict-first", "P1", "registry.local/team/app:cancel-conflict-first"), "U1")
	code, data, _ := startDockerfileImageBuild(app, firstReq, platform.RouteSpec{})
	assertImageStatus(t, code, data, http.StatusAccepted)
	secondReq := imageRequest(http.MethodPost, "/api/v1/images/build/dockerfile", imageBuildBody("cancel-conflict-second", "P1", "registry.local/team/app:cancel-conflict-second"), "U1")
	code, data, _ = startDockerfileImageBuild(app, secondReq, platform.RouteSpec{})
	assertImageStatus(t, code, data, http.StatusAccepted)

	cancelFirstReq := imageProjectRequest(http.MethodDelete, "/api/v1/projects/P1/builds/cancel-conflict-first", "", "U1", "P1")
	cancelFirstReq.SetPathValue("jobName", "cancel-conflict-first")
	cancelFirstReq.Header.Set(idempotencyKeyHeader, key)
	code, data, _ = cancelProjectBuild(app, cancelFirstReq, platform.RouteSpec{})
	assertImageStatus(t, code, data, http.StatusOK)
	assertImageMapValue(t, data, "status", "cancelled")
	keyHash, fingerprintHash := storedImageBuildCancelIdempotencyHashes(t, app, "cancel-conflict-first")
	assertNoImageIdempotencyMaterial(t, data, key, keyHash, fingerprintHash)

	cancelSecondReq := imageProjectRequest(http.MethodDelete, "/api/v1/projects/P1/builds/cancel-conflict-second", "", "U1", "P1")
	cancelSecondReq.SetPathValue("jobName", "cancel-conflict-second")
	cancelSecondReq.Header.Set(idempotencyKeyHeader, key)
	code, data, _ = cancelProjectBuild(app, cancelSecondReq, platform.RouteSpec{})
	assertImageStatus(t, code, data, http.StatusConflict)
	assertNoImageIdempotencyMaterial(t, data, key, keyHash, fingerprintHash)

	second, found := app.Store.Get(context.Background(), imageBuildsResource, "cancel-conflict-second")
	if !found {
		t.Fatalf("second build record missing")
	}
	if status, _ := second.Data["status"].(string); status == "cancelled" {
		t.Fatalf("second build was cancelled after idempotency conflict")
	}
	if events := imageEventsByName(app, "ImageBuildCancelled"); len(events) != 1 {
		t.Fatalf("ImageBuildCancelled events = %#v, want one after cancel idempotency conflict", events)
	}
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
	// ServiceAPIKey lets the co-hosted build-source-access hop authenticate via
	// the non-strict service-key fallback.
	app := platform.NewApp(platform.Config{ServiceName: "all", HTTPAddr: ":0", ServiceAPIKey: "image-test-service-key"})
	// Inline context archives are staged at create; without an object store the
	// build endpoints fail closed with 503.
	app.ObjectStore = platform.NewMemoryObjectStore()
	Register(app)
	// from-storage builds consult storage-service's build-source-access
	// contract; co-host it (spec registration wires the internal route into
	// dispatch) and seed a project binding with a read-capable default policy
	// so storage builds in these tests are authorized.
	app.RegisterService(storageservice.Spec())
	storageservice.Register(app)
	createImageRecords(t, app, "storage-service:storage_bindings", []map[string]any{
		{"id": "P1:PVC1", "project_id": "P1", "group_id": "G1", "pvc_id": "PVC1"},
	})
	createImageRecords(t, app, "storage-service:storage_access_policies", []map[string]any{
		{"id": "G1:PVC1", "group_id": "G1", "pvc_id": "PVC1", "default_permission": "read_write"},
	})
	createImageRecords(t, app, identityUsersResource, []map[string]any{
		{"id": "ADMIN", "username": "admin", "capabilities": map[string]any{"adminPanel": true}},
		{"id": "U1", "username": "alice"},
		{"id": "U2", "username": "bob"},
	})
	createImageRecords(t, app, orgProjectsResource, []map[string]any{
		{
			"id":                       "P1",
			"project_name":             "vision",
			"owner_id":                 "G1",
			"allow_image_build":        true,
			"build_cpu_limit":          8.0,
			"build_memory_gib_limit":   16.0,
			"build_time_limit_seconds": 3600,
			"max_running_builds":       10,
		},
	})
	createImageRecords(t, app, orgUserGroupsResource, []map[string]any{
		{"id": "U1:G1", "user_id": "U1", "group_id": "G1", "role": "admin"},
		{"id": "U2:G1", "user_id": "U2", "group_id": "G1", "role": "user"},
	})
	createImageRecords(t, app, imageCatalogResource, []map[string]any{
		{"id": "tag-1", "registry": "registry.local", "repository": "library/base", "tag": "1.0", "digest": "sha256:base", "scan_status": "Success"},
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

func imageBuildBody(id, projectID, imageRef string) string {
	return fmt.Sprintf(`{"id":%q,"project_id":%q,"image_reference":%q,"cpu_cores":2,"memory_gib":4,"max_build_seconds":600}`, id, projectID, imageRef)
}

func imageStorageBuildBody(id, projectID, imageRef string) string {
	return fmt.Sprintf(`{"id":%q,"project_id":%q,"image_reference":%q,"cpu_cores":2,"memory_gib":4,"max_build_seconds":600,"storage_path":"images/context.tar"}`, id, projectID, imageRef)
}

func updateImageProject(t *testing.T, app *platform.App, projectID string, data map[string]any) {
	t.Helper()
	if _, ok := app.Store.Update(context.Background(), orgProjectsResource, projectID, data); !ok {
		t.Fatalf("project %s update failed", projectID)
	}
}

func seedImageBuildStatuses(t *testing.T, app *platform.App, projectID string, statuses ...string) {
	t.Helper()
	rows := make([]map[string]any, 0, len(statuses))
	for index, status := range statuses {
		id := fmt.Sprintf("%s-build-%02d-%s", projectID, index, status)
		rows = append(rows, map[string]any{
			"id":                     id,
			"build_id":               id,
			"job_name":               id,
			"project_id":             projectID,
			"image_reference":        "registry.local/team/app:" + status,
			"build_type":             "dockerfile",
			"cpu_cores":              1.0,
			"memory_gib":             2.0,
			"max_build_time_seconds": 300,
			"status":                 status,
		})
	}
	createImageRecords(t, app, imageBuildsResource, rows)
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

func assertImageBuildSupplyChainDefaults(t *testing.T, data any) {
	t.Helper()
	row := data.(map[string]any)
	want := map[string]any{
		"image_digest":            "",
		"allow_list_decision":     "pending",
		"sbom_status":             "pending",
		"signature_status":        "pending",
		"scan_status":             "pending",
		"supply_chain_checked_at": nil,
	}
	for key, value := range want {
		if row[key] != value {
			t.Fatalf("%s = %#v, want %#v in %#v", key, row[key], value, row)
		}
	}
}

func storedImageBuildIdempotencyHashes(t *testing.T, app *platform.App, id string) (string, string) {
	t.Helper()
	record, found := app.Store.Get(context.Background(), imageBuildsResource, id)
	if !found {
		t.Fatalf("build %s not found", id)
	}
	keyHash, _ := record.Data[internalImageBuildIdempotencyKeyHash].(string)
	fingerprintHash, _ := record.Data[internalImageBuildIdempotencyFingerprintHash].(string)
	if keyHash == "" || fingerprintHash == "" {
		t.Fatalf("stored idempotency hashes missing from internal record: %#v", record.Data)
	}
	return keyHash, fingerprintHash
}

func storedImageBuildCancelIdempotencyHashes(t *testing.T, app *platform.App, id string) (string, string) {
	t.Helper()
	record, found := app.Store.Get(context.Background(), imageBuildsResource, id)
	if !found {
		t.Fatalf("build %s not found", id)
	}
	keyHash, _ := record.Data[internalImageBuildCancelIdempotencyKeyHash].(string)
	fingerprintHash, _ := record.Data[internalImageBuildCancelIdempotencyFingerprintHash].(string)
	if keyHash == "" || fingerprintHash == "" {
		t.Fatalf("stored cancel idempotency hashes missing from internal record")
	}
	return keyHash, fingerprintHash
}

func imageEventsByName(app *platform.App, name string) []contracts.Event {
	events := []contracts.Event{}
	for _, event := range app.Events.Outbox() {
		if event.Name == name {
			events = append(events, event)
		}
	}
	return events
}

func assertNoImageIdempotencyMaterial(t *testing.T, value any, forbidden ...string) {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal value for leak check: %v", err)
	}
	text := string(raw)
	for _, token := range append(forbidden,
		internalImageBuildIdempotencyKeyHash,
		internalImageBuildIdempotencyFingerprintHash,
		internalImageBuildCancelIdempotencyKeyHash,
		internalImageBuildCancelIdempotencyFingerprintHash,
		"idempotency_key_hash",
		"fingerprint_hash",
	) {
		if token != "" && strings.Contains(text, token) {
			t.Fatalf("idempotency material %q leaked in %s", token, text)
		}
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
