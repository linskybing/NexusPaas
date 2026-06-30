//go:build e2e

package e2e

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
)

const (
	imageCatalogResource        = "image-registry-service:container_tags"
	imageBuildsResource         = "image-registry-service:image_build_jobs"
	imageProjectsResource       = "image-registry-service:image_projects"
	imageProjectMembersResource = "image-registry-service:image_project_members"
)

func TestImageBuildGovernanceE2E(t *testing.T) {
	h := newHarness(t, identityService, imageRegistryService)
	ids := h.seedIdentityContracts()
	badToken := h.seedAPIUser("badimage"+h.runID, "bad-image-"+h.runID, false)
	h.seedImageRegistryProjectAccess(ids.userID, "manager")

	suffix := e2eSuffix(h.runID)
	catalogID := "tag-" + suffix
	requestID, rejectID := h.exerciseImageRequestReview(ids, suffix)
	h.exerciseImageCatalogPublish(ids.apiToken, suffix, catalogID)
	buildIDs := h.createImageBuilds(ids.apiToken, suffix)
	h.assertImageBuildReadCancel(ids.apiToken, buildIDs)
	h.assertImageRegistryNonMemberGuards(badToken, buildIDs[1])
	if build := h.getRecord(imageBuildsResource, buildIDs[1]); build.Data["status"] != "queued" {
		t.Fatalf("non-member cancel mutated build = %#v, want queued", build.Data)
	}
	h.requireImageGovernanceEvents(requestID, rejectID, catalogID, buildIDs)
}

func (h *e2eHarness) exerciseImageRequestReview(ids identityIDs, suffix string) (string, string) {
	h.t.Helper()
	imageRef := "registry.local/" + suffix + "/trainer:approved"
	requestID := "imgreq-" + suffix
	rejectID := "imgrej-" + suffix
	requested := h.doJSONWithBearer(imageRegistryService, http.MethodPost, "/api/v1/projects/"+h.projectID()+"/images", map[string]any{
		"id":              requestID,
		"image_reference": imageRef,
		"e2e_run_id":      h.runID,
	}, ids.apiToken, http.StatusCreated)
	h.requireEnvelopeCorrelation(requested)
	if data := requested.dataMap(h.t); data["status"] != "pending" || data["project_id"] != h.projectID() || data["requested_by"] != ids.userID {
		h.t.Fatalf("image request = %#v, want pending project request", data)
	}
	projectRequests := e2eDataRecords(h.t, h.doWithBearer(imageRegistryService, http.MethodGet, "/api/v1/projects/"+h.projectID()+"/image-requests", ids.apiToken, http.StatusOK))
	if !e2eRecordsContainID(projectRequests, requestID) {
		h.t.Fatalf("project image requests = %#v, want %s", projectRequests, requestID)
	}
	approved := h.doJSON(imageRegistryService, http.MethodPut, "/api/v1/image-requests/"+requestID+"/approve", nil, h.apiKey, http.StatusOK)
	h.requireEnvelopeCorrelation(approved)
	if data := approved.dataMap(h.t); data["status"] != "approved" || data["reviewed_by"] == "" {
		h.t.Fatalf("approved image request = %#v, want approved with reviewer", data)
	}
	h.assertProjectImageContains(ids.apiToken, "image_reference", imageRef)
	h.doJSONWithBearer(imageRegistryService, http.MethodPost, "/api/v1/projects/"+h.projectID()+"/images", map[string]any{
		"id":              rejectID,
		"image_reference": "registry.local/" + suffix + "/trainer:rejected",
		"e2e_run_id":      h.runID,
	}, ids.apiToken, http.StatusCreated)
	rejected := h.doJSON(imageRegistryService, http.MethodPut, "/api/v1/image-requests/"+rejectID+"/reject", nil, h.apiKey, http.StatusOK)
	if rejected.dataMap(h.t)["status"] != "rejected" {
		h.t.Fatalf("rejected image request = %#v, want rejected", rejected.dataMap(h.t))
	}
	return requestID, rejectID
}

func (h *e2eHarness) exerciseImageCatalogPublish(token, suffix, catalogID string) {
	h.t.Helper()
	h.createRecord(imageCatalogResource, catalogID, map[string]any{
		"tag_id":          catalogID,
		"registry":        "registry.local",
		"repository":      suffix + "/catalog",
		"tag":             "1",
		"image_reference": "registry.local/" + suffix + "/catalog:1",
		"digest":          "sha256:e2e" + suffix,
		"scan_status":     "passed",
	})
	published := h.doJSON(imageRegistryService, http.MethodPost, "/api/v1/image-catalog/publish", map[string]any{
		"tag_id":     catalogID,
		"project_id": h.projectID(),
		"e2e_run_id": h.runID,
	}, h.apiKey, http.StatusOK)
	if rules := published.dataMap(h.t)["rules"].([]any); len(rules) != 1 {
		h.t.Fatalf("catalog publish rules = %#v, want one project rule", published.dataMap(h.t)["rules"])
	}
	h.assertProjectImageContains(token, "tag_id", catalogID)
	h.doJSON(imageRegistryService, http.MethodPost, "/api/v1/image-catalog/"+catalogID+"/unpublish", nil, h.apiKey, http.StatusOK)
	projectImages := e2eDataRecords(h.t, h.doWithBearer(imageRegistryService, http.MethodGet, "/api/v1/projects/"+h.projectID()+"/images", token, http.StatusOK))
	if e2eRecordsContainDataValue(projectImages, "tag_id", catalogID) {
		h.t.Fatalf("project images after unpublish = %#v, want tag %s removed", projectImages, catalogID)
	}
}

func (h *e2eHarness) assertProjectImageContains(token, key, value string) {
	h.t.Helper()
	projectImages := e2eDataRecords(h.t, h.doWithBearer(imageRegistryService, http.MethodGet, "/api/v1/projects/"+h.projectID()+"/images", token, http.StatusOK))
	if !e2eRecordsContainDataValue(projectImages, key, value) {
		h.t.Fatalf("project images = %#v, want %s=%s", projectImages, key, value)
	}
}

func (h *e2eHarness) assertImageBuildReadCancel(token string, buildIDs []string) {
	h.t.Helper()
	logs := h.doWithBearer(imageRegistryService, http.MethodGet, "/api/v1/images/build/"+buildIDs[0]+"/logs", token, http.StatusOK)
	if !bytes.Contains(logs.Body, []byte("build queued")) || !bytes.HasPrefix([]byte(logs.Header.Get("Content-Type")), []byte("text/plain")) {
		h.t.Fatalf("build logs content-type=%q body=%q, want queued plain text", logs.Header.Get("Content-Type"), string(logs.Body))
	}
	builds := e2eDataRecords(h.t, h.doWithBearer(imageRegistryService, http.MethodGet, "/api/v1/projects/"+h.projectID()+"/builds", token, http.StatusOK))
	for _, id := range buildIDs {
		if !e2eRecordsContainID(builds, id) {
			h.t.Fatalf("project builds = %#v, want build %s", builds, id)
		}
	}
	aliasBuilds := e2eDataRecords(h.t, h.doWithBearer(imageRegistryService, http.MethodGet, "/api/v1/projects/"+h.projectID()+"/image-builds", token, http.StatusOK))
	if len(aliasBuilds) != len(builds) {
		h.t.Fatalf("project image-builds alias = %#v, want same count as builds %#v", aliasBuilds, builds)
	}
	cancelled := h.doJSONWithBearer(imageRegistryService, http.MethodDelete, "/api/v1/projects/"+h.projectID()+"/builds/"+buildIDs[0], nil, token, http.StatusOK)
	if cancelled.dataMap(h.t)["status"] != "cancelled" {
		h.t.Fatalf("cancelled build = %#v, want cancelled", cancelled.dataMap(h.t))
	}
}

func (h *e2eHarness) requireImageGovernanceEvents(requestID, rejectID, catalogID string, buildIDs []string) {
	h.t.Helper()
	h.requireEvent("ImageRequested", func(event contracts.Event) bool {
		return event.Source == imageRegistryService && event.Data["id"] == requestID
	})
	h.requireEvent("ImageApproved", func(event contracts.Event) bool {
		return event.Source == imageRegistryService && event.Data["id"] == requestID
	})
	h.requireEvent("ImageRejected", func(event contracts.Event) bool {
		return event.Source == imageRegistryService && event.Data["id"] == rejectID
	})
	h.requireEvent("ImagePublished", func(event contracts.Event) bool {
		return event.Source == imageRegistryService && event.Data["tag_id"] == catalogID
	})
	h.requireEvent("ImageUnpublished", func(event contracts.Event) bool {
		return event.Source == imageRegistryService && event.Data["tag_id"] == catalogID
	})
	for _, id := range buildIDs {
		h.requireEvent("ImageBuildStarted", func(event contracts.Event) bool {
			return event.Source == imageRegistryService && event.Data["id"] == id
		})
	}
	h.requireEvent("ImageBuildCancelled", func(event contracts.Event) bool {
		return event.Source == imageRegistryService && event.Data["id"] == buildIDs[0]
	})
}

func (h *e2eHarness) seedImageRegistryProjectAccess(userID, role string) {
	h.t.Helper()
	h.createRecord(imageProjectsResource, h.projectID(), map[string]any{
		"p_id":                     h.projectID(),
		"project_name":             "image-project-" + h.runID,
		"allow_image_build":        true,
		"build_cpu_limit":          8,
		"build_memory_gib_limit":   16,
		"build_time_limit_seconds": 3600,
		"max_running_builds":       10,
	})
	h.createRecord(imageProjectMembersResource, h.projectID()+":"+userID, map[string]any{
		"project_id": h.projectID(),
		"user_id":    userID,
		"role":       role,
	})
}

func (h *e2eHarness) createImageBuilds(token, suffix string) []string {
	h.t.Helper()
	builds := []struct {
		id      string
		path    string
		payload map[string]any
	}{
		{
			id:   "ctx-build-" + suffix,
			path: "/api/v1/images/build",
			payload: map[string]any{
				"project_id": h.projectID(),
				"repository": suffix + "/ctx",
				"tag":        "dev",
			},
		},
		{
			id:   "storage-build-" + suffix,
			path: "/api/v1/images/build/from-storage",
			payload: map[string]any{
				"project_id":      h.projectID(),
				"image_reference": "registry.local/" + suffix + "/storage:dev",
				"storage_path":    "/datasets/" + suffix,
			},
		},
		{
			id:   "dockerfile-build-" + suffix,
			path: "/api/v1/images/build/dockerfile",
			payload: map[string]any{
				"project_id":      h.projectID(),
				"image_reference": "registry.local/" + suffix + "/dockerfile:dev",
				"dockerfile":      "FROM scratch\n",
			},
		},
	}
	ids := make([]string, 0, len(builds))
	for _, build := range builds {
		build.payload["id"] = build.id
		build.payload["e2e_run_id"] = h.runID
		build.payload["cpu_cores"] = 1
		build.payload["memory_gib"] = 2
		build.payload["max_build_time_seconds"] = 300
		response := h.doJSONWithBearer(imageRegistryService, http.MethodPost, build.path, build.payload, token, http.StatusAccepted)
		data := response.dataMap(h.t)
		if data["id"] != build.id || data["status"] != "queued" || data["project_id"] != h.projectID() {
			h.t.Fatalf("image build %s = %#v, want queued project build", build.id, data)
		}
		ids = append(ids, build.id)
	}
	return ids
}

func (h *e2eHarness) assertImageRegistryNonMemberGuards(badToken, queuedBuildID string) {
	h.t.Helper()
	h.doWithBearer(imageRegistryService, http.MethodGet, "/api/v1/projects/"+h.projectID()+"/images", badToken, http.StatusForbidden)
	h.doWithBearer(imageRegistryService, http.MethodGet, "/api/v1/projects/"+h.projectID()+"/image-requests", badToken, http.StatusForbidden)
	h.doWithBearer(imageRegistryService, http.MethodGet, "/api/v1/projects/"+h.projectID()+"/builds", badToken, http.StatusForbidden)
	h.doWithBearer(imageRegistryService, http.MethodGet, "/api/v1/projects/"+h.projectID()+"/image-builds", badToken, http.StatusForbidden)
	h.doJSONWithBearer(imageRegistryService, http.MethodPost, "/api/v1/projects/"+h.projectID()+"/images", map[string]any{
		"image_reference": "registry.local/forged/app:latest",
	}, badToken, http.StatusForbidden)
	h.doWithBearer(imageRegistryService, http.MethodGet, "/api/v1/images/build/"+queuedBuildID+"/logs", badToken, http.StatusForbidden)
	h.doJSONWithBearer(imageRegistryService, http.MethodDelete, "/api/v1/projects/"+h.projectID()+"/builds/"+queuedBuildID, nil, badToken, http.StatusForbidden)
}
