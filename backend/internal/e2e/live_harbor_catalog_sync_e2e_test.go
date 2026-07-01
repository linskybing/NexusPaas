//go:build e2e

package e2e

import (
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

// TestLiveHarborCatalogSyncE2E exercises the real Harbor artifact-list API
// (/api/v2.0/projects/{project}/artifacts) end to end: seed a project+image
// with `make harbor-up && make harbor-seed`, then run this via `make
// e2e-harbor`. This is additive to the prior one-off 2026-06-21 live RKE2
// Harbor evidence recorded in docs/acceptance/gap-tracker.md — it proves the
// same code path repeatably, in CI and locally, against an ephemeral
// CI/local Harbor instance. It does not prove external registry promotion,
// rollback, or build execution (Tekton/BuildKit/SBOM/scan/sign remain
// unimplemented — see docs/acceptance/blocker-ledger.md).
func TestLiveHarborCatalogSyncE2E(t *testing.T) {
	if strings.TrimSpace(os.Getenv("TEST_LIVE_HARBOR_IMAGE_BUILD")) != "1" {
		t.Skip("set TEST_LIVE_HARBOR_IMAGE_BUILD=1 to run live Harbor catalog-sync e2e")
	}
	harborURL := strings.TrimSpace(os.Getenv("HARBOR_URL"))
	if harborURL == "" {
		t.Skip("HARBOR_URL is required for live Harbor catalog-sync e2e")
	}
	adminPassword := envDefault("HARBOR_ADMIN_PASSWORD", "Harbor12345")
	project := envDefault("HARBOR_SEED_PROJECT", "nexuspaas-e2e")
	repository := envDefault("HARBOR_SEED_REPOSITORY", "smoke")
	tag := envDefault("HARBOR_SEED_TAG", "v1")

	h := newHarness(t)
	imageRegistry := h.startExtraServiceWithConfig("image-registry-live-harbor-sync-"+h.runID, imageRegistryService, nil, func(cfg *platform.Config) {
		cfg.ExternalURLs = map[string]string{"harbor": harborURL}
		cfg.AdapterConfigs = map[string]platform.AdapterConfig{
			"harbor": {
				AddPrefix: "/api/v2.0",
				Auth: platform.AdapterAuthConfig{
					Type:     "basic",
					Username: "admin",
					Password: adminPassword,
				},
			},
		}
	})

	tagID := "harbor-live-sync-" + h.runID
	resp := h.doURLJSON(imageRegistry.url, http.MethodPost, "/api/v1/image-catalog/sync", map[string]any{
		"tag_id":     tagID,
		"project":    project,
		"repository": project + "/" + repository,
		"tag":        tag,
	}, h.apiKey, http.StatusAccepted)
	h.requireEnvelopeCorrelation(resp)
	data := resp.dataMap(t)
	if data["status"] != "synced" {
		t.Fatalf("live Harbor catalog sync status = %#v, want synced (data=%#v)", data["status"], data)
	}
	catalogID, _ := data["catalog_id"].(string)
	if catalogID == "" {
		t.Fatalf("live Harbor catalog sync missing catalog_id: %#v", data)
	}

	catalog := h.getRecord(imageCatalogResource, catalogID)
	digest, _ := catalog.Data["digest"].(string)
	if digest == "" || !strings.HasPrefix(digest, "sha256:") {
		t.Fatalf("live Harbor catalog digest = %#v, want a real sha256 digest from the seeded push", catalog.Data["digest"])
	}
	if catalog.Data["status"] != "available" || catalog.Data["deleted"] == true {
		t.Fatalf("live Harbor catalog record = %#v, want available/not-deleted", catalog.Data)
	}
	// scan_status is intentionally not hard-asserted: this CI/local Harbor is
	// installed without --with-trivy (leaner/faster by design), so
	// scan_overview is empty and harborScanStatus() leaves the field unset.
	t.Logf("live Harbor catalog scan_status = %#v (Trivy not installed in this Harbor by design)", catalog.Data["scan_status"])
}
