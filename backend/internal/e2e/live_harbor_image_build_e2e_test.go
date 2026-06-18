//go:build e2e

package e2e

import (
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

func TestLiveHarborImageBuildE2E(t *testing.T) {
	if strings.TrimSpace(os.Getenv("TEST_LIVE_HARBOR_IMAGE_BUILD")) != "1" {
		t.Skip("set TEST_LIVE_HARBOR_IMAGE_BUILD=1 to run live Harbor adapter boundary e2e")
	}
	harborURL := strings.TrimSpace(os.Getenv("HARBOR_URL"))
	if harborURL == "" {
		t.Skip("HARBOR_URL is required for live Harbor adapter boundary e2e")
	}
	h := newHarness(t)
	imageRegistry := h.startExtraServiceWithConfig("image-registry-live-harbor-"+h.runID, imageRegistryService, nil, func(cfg *platform.Config) {
		cfg.ExternalURLs = map[string]string{"harbor": harborURL}
	})

	status := h.doURLJSON(imageRegistry.url, http.MethodGet, "/api/v1/harbor-status", nil, h.apiKey, http.StatusOK)
	h.requireEnvelopeCorrelation(status)
	data := status.dataMap(t)
	if data["adapter"] != "harbor" || data["status"] != "ok" {
		t.Fatalf("live Harbor status = %#v, want existing harbor adapter success boundary", data)
	}
}
