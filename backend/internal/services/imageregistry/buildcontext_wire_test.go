package imageregistry

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"testing"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

func b64BuildContext(t *testing.T, entries []archiveEntry) string {
	t.Helper()
	return base64.StdEncoding.EncodeToString(makeTarGz(t, entries))
}

func dockerfileBuildBodyWithArchive(archiveB64 string) string {
	return fmt.Sprintf(`{"id":"ctx-arch","project_id":"P1","image_reference":"registry.local/team/app:ctx","cpu_cores":2,"memory_gib":4,"max_build_seconds":600,"dockerfile":"FROM scratch","context_archive":%q}`, archiveB64)
}

// P0-1: a valid inline context archive is validated, content-addressed, and its
// digest persisted on the build record.
func TestImageBuildValidatesInlineContextArchive(t *testing.T) {
	app := newImageRegistryTestApp(t)
	archive := b64BuildContext(t, []archiveEntry{{name: "Dockerfile", body: "FROM scratch"}, {name: "main.go", body: "package main"}})

	req := imageRequest(http.MethodPost, "/api/v1/images/build/dockerfile", dockerfileBuildBodyWithArchive(archive), "U1")
	code, data, _ := startDockerfileImageBuild(app, req, platform.RouteSpec{})
	assertImageStatus(t, code, data, http.StatusAccepted)

	records := app.Store.List(context.Background(), imageBuildsResource)
	if len(records) != 1 {
		t.Fatalf("records = %d, want one build", len(records))
	}
	if digest := shared.TextValue(records[0].Data, "source_digest"); len(digest) < 8 || digest[:7] != "sha256:" {
		t.Fatalf("source_digest = %q, want sha256: content digest", digest)
	}
}

// P0-1: a malicious archive (path traversal) is rejected at submit with a 400.
func TestImageBuildRejectsMaliciousContextArchive(t *testing.T) {
	app := newImageRegistryTestApp(t)
	archive := b64BuildContext(t, []archiveEntry{{name: "../escape", body: "x"}})

	req := imageRequest(http.MethodPost, "/api/v1/images/build/dockerfile", dockerfileBuildBodyWithArchive(archive), "U1")
	code, data, _ := startDockerfileImageBuild(app, req, platform.RouteSpec{})
	assertImageStatus(t, code, data, http.StatusBadRequest)

	if records := app.Store.List(context.Background(), imageBuildsResource); len(records) != 0 {
		t.Fatalf("records = %d, want no build created for malicious archive", len(records))
	}
}

// P0-1 + P0-2: the same Idempotency-Key with a different context archive is a 409.
func TestImageBuildContextArchiveFeedsIdempotencyFingerprint(t *testing.T) {
	app := newImageRegistryTestApp(t)
	const key = "image-build-archive-key"
	first := b64BuildContext(t, []archiveEntry{{name: "Dockerfile", body: "FROM scratch"}})
	second := b64BuildContext(t, []archiveEntry{{name: "Dockerfile", body: "FROM alpine"}})

	firstReq := imageRequest(http.MethodPost, "/api/v1/images/build/dockerfile", dockerfileBuildBodyWithArchive(first), "U1")
	firstReq.Header.Set(idempotencyKeyHeader, key)
	code, data, _ := startDockerfileImageBuild(app, firstReq, platform.RouteSpec{})
	assertImageStatus(t, code, data, http.StatusAccepted)

	secondReq := imageRequest(http.MethodPost, "/api/v1/images/build/dockerfile", dockerfileBuildBodyWithArchive(second), "U1")
	secondReq.Header.Set(idempotencyKeyHeader, key)
	code, data, _ = startDockerfileImageBuild(app, secondReq, platform.RouteSpec{})
	assertImageStatus(t, code, data, http.StatusConflict)
}
