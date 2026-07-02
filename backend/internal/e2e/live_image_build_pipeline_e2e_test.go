//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

// TestLiveImageBuildPipelineE2E drives the product image-build workflow end to
// end against a real local Harbor: create build via API → dispatcher executes
// (docker build → Harbor push → syft SBOM → trivy scan → cosign sign) →
// verified-provenance publish gate. Live local-tier evidence for
// blocker-ledger §8 items 5+6 (NOT external GA proof).
//
// Requires: TEST_LIVE_IMAGE_BUILD_PIPELINE=1, HARBOR_URL, HARBOR_ADMIN_PASSWORD,
// docker/syft/trivy/cosign on PATH, and a docker login to the Harbor host.
func TestLiveImageBuildPipelineE2E(t *testing.T) {
	if strings.TrimSpace(os.Getenv("TEST_LIVE_IMAGE_BUILD_PIPELINE")) != "1" {
		t.Skip("set TEST_LIVE_IMAGE_BUILD_PIPELINE=1 to run the live image-build pipeline e2e")
	}
	harborURL := strings.TrimSpace(os.Getenv("HARBOR_URL"))
	if harborURL == "" {
		t.Skip("HARBOR_URL is required")
	}
	for _, tool := range []string{"docker", "syft", "trivy", "cosign"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s is required on PATH for the live pipeline e2e", tool)
		}
	}
	harborHost := strings.TrimPrefix(strings.TrimPrefix(harborURL, "http://"), "https://")
	project := strings.TrimSpace(os.Getenv("HARBOR_SEED_PROJECT"))
	if project == "" {
		project = "library"
	}
	imageRef := fmt.Sprintf("%s/%s/nexuspaas-build-e2e:%d", harborHost, project, time.Now().Unix())

	h := newHarness(t)
	imageRegistrySvc := h.startExtraServiceWithConfig("image-registry-build-pipeline-"+h.runID, imageRegistryService, nil, func(cfg *platform.Config) {
		cfg.ExternalURLs = map[string]string{"harbor": harborURL}
		cfg.ImageBuildExecutor = "docker"
		cfg.ImageProvenanceRequired = true
	})

	// project + manager membership for the harness admin user (image-registry
	// reads its own image_projects/image_project_members read model)
	adminUser := "admin-" + h.runID
	h.createRecord(imageProjectsResource, "build-e2e-project", map[string]any{
		"p_id": "build-e2e-project", "project_name": "build-e2e",
		"allow_image_build": true, "build_cpu_limit": 8, "build_memory_gib_limit": 16,
		"build_time_limit_seconds": 3600, "max_running_builds": 5,
	})
	h.createRecord(imageProjectMembersResource, "build-e2e-project:"+adminUser, map[string]any{
		"project_id": "build-e2e-project", "user_id": adminUser, "role": "manager",
	})

	build := h.doURLJSON(imageRegistrySvc.url, http.MethodPost, "/api/v1/images/build/dockerfile", map[string]any{
		"id":                     "live-pipeline-" + h.runID,
		"project_id":             "build-e2e-project",
		"image_reference":        imageRef,
		"cpu_cores":              2,
		"memory_gib":             4,
		"max_build_time_seconds": 900,
		"dockerfile":             "FROM busybox:1.36\nRUN echo nexuspaas-live-build > /evidence.txt\n",
	}, h.apiKey, http.StatusAccepted).dataMap(t)
	if build["status"] != "queued" {
		t.Fatalf("build create = %#v, want queued", build)
	}
	buildID, _ := build["id"].(string)

	// The dispatcher maintenance task is lease-gated and interval-driven; run
	// the registered tasks synchronously until the build reaches a terminal
	// state (same mechanism the runtime uses, deterministic for the test).
	deadline := time.Now().Add(15 * time.Minute)
	var final map[string]any
	for {
		imageRegistrySvc.app.RunMaintenanceOnce(context.Background(), time.Minute)
		record := h.getRecord(imageBuildsResource, buildID)
		status, _ := record.Data["status"].(string)
		if status == "succeeded" || status == "failed" {
			final = record.Data
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("build did not reach a terminal state; last=%v", record.Data)
		}
		time.Sleep(2 * time.Second)
	}

	logs, _ := final["logs"].(string)
	if final["status"] != "succeeded" {
		t.Fatalf("build status = %v, want succeeded; logs:\n%s", final["status"], logs)
	}
	for field, want := range map[string]string{
		"sbom_status": "succeeded", "scan_status": "passed", "signature_status": "signed",
	} {
		if got, _ := final[field].(string); got != want {
			t.Fatalf("%s = %q, want %q; logs:\n%s", field, got, want, logs)
		}
	}
	digest, _ := final["image_digest"].(string)
	if !strings.HasPrefix(digest, "sha256:") {
		t.Fatalf("image_digest = %q, want sha256-pinned", digest)
	}

	// image really exists in Harbor (registry v2 manifest HEAD via docker pull)
	repo := imageRef
	if at := strings.LastIndex(repo, ":"); at > strings.LastIndex(repo, "/") {
		repo = repo[:at]
	}
	pull := exec.Command("docker", "pull", repo+"@"+digest)
	if out, err := pull.CombinedOutput(); err != nil {
		t.Fatalf("pull built image from Harbor: %v\n%s", err, out)
	}

	// verified-provenance publish gate: a catalog row whose digest has no
	// verified build record is rejected; the built digest passes.
	h.createRecord(imageCatalogResource, "cat-unverified-"+h.runID, map[string]any{
		"registry": harborHost, "repository": project + "/nexuspaas-build-e2e",
		"tag": "unverified", "digest": "sha256:deadbeef", "scan_status": "Success",
		"sbom_digest": "sha256:sbom", "signature": "sigstore://forged",
	})
	rejected := h.doURLJSON(imageRegistrySvc.url, http.MethodPost, "/api/v1/image-catalog/publish", map[string]any{
		"tag_id": "cat-unverified-" + h.runID, "project_id": "build-e2e-project",
	}, h.apiKey, http.StatusConflict).dataMap(t)
	if msg, _ := rejected["message"].(string); !strings.Contains(msg, "provenance") {
		t.Fatalf("unverified publish rejection = %#v, want provenance rejection", rejected)
	}

	h.createRecord(imageCatalogResource, "cat-verified-"+h.runID, map[string]any{
		"registry": harborHost, "repository": project + "/nexuspaas-build-e2e",
		"tag": "verified", "digest": digest, "scan_status": "Success",
		"sbom_digest": final["sbom_digest"], "signature": final["signature_ref"],
	})
	published := h.doURLJSON(imageRegistrySvc.url, http.MethodPost, "/api/v1/image-catalog/publish", map[string]any{
		"tag_id": "cat-verified-" + h.runID, "project_id": "build-e2e-project",
	}, h.apiKey, http.StatusOK).dataMap(t)
	if published["rules"] == nil {
		t.Fatalf("verified publish = %#v, want allow-list rules", published)
	}
}
