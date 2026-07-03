package imageregistry

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

type fakeBuildExecutor struct {
	result buildExecutionResult
	err    error
	inputs []buildExecutionInput
}

func (f *fakeBuildExecutor) Name() string { return "fake" }
func (f *fakeBuildExecutor) Execute(_ context.Context, in buildExecutionInput) (buildExecutionResult, error) {
	f.inputs = append(f.inputs, in)
	return f.result, f.err
}

func queueBuildForDispatch(t *testing.T, app *platform.App, id string) {
	t.Helper()
	body := fmt.Sprintf(`{"id":%q,"project_id":"P1","image_reference":"registry.local/team/app:%s","cpu_cores":2,"memory_gib":4,"max_build_seconds":600,"dockerfile":"FROM scratch\n"}`, id, id)
	code, data, _ := startDockerfileImageBuild(app, imageRequest(http.MethodPost, "/api/v1/images/build/dockerfile", body, "U1"), platform.RouteSpec{})
	assertImageStatus(t, code, data, http.StatusAccepted)
}

func dispatchedBuild(t *testing.T, app *platform.App, id string) map[string]any {
	t.Helper()
	record, found := app.Store.Get(t.Context(), imageBuildsResource, id)
	if !found {
		t.Fatalf("build %s not found", id)
	}
	return record.Data
}

func TestDispatchQueuedImageBuildsSuccess(t *testing.T) {
	app := newImageRegistryTestApp(t)
	queueBuildForDispatch(t, app, "disp-ok")
	executor := &fakeBuildExecutor{result: buildExecutionResult{
		ImageDigest:  "sha256:built",
		SBOMDigest:   "sha256:sbom",
		ScanPassed:   true,
		ScanSummary:  "0 HIGH/CRITICAL",
		SignatureRef: "registry.local/team/app@sha256:built",
		Logs:         []string{"[build] ok"},
	}}
	if err := dispatchQueuedImageBuilds(t.Context(), app, executor); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	data := dispatchedBuild(t, app, "disp-ok")
	for key, want := range map[string]string{
		"status": "succeeded", "image_digest": "sha256:built",
		"sbom_status": "succeeded", "scan_status": "passed", "signature_status": "signed",
	} {
		if got := shared.TextValue(data, key); got != want {
			t.Fatalf("%s = %q, want %q (data=%v)", key, got, want, data)
		}
	}
	if len(executor.inputs) != 1 || executor.inputs[0].Dockerfile == "" {
		t.Fatalf("executor inputs = %#v, want dockerfile forwarded", executor.inputs)
	}
	if !strings.Contains(shared.TextValue(data, "logs"), "[build] ok") {
		t.Fatalf("logs = %q, want executor log lines", shared.TextValue(data, "logs"))
	}
	// second tick: nothing queued
	if err := dispatchQueuedImageBuilds(t.Context(), app, executor); err != nil {
		t.Fatalf("idle dispatch: %v", err)
	}
	if len(executor.inputs) != 1 {
		t.Fatalf("executor ran %d times, want 1", len(executor.inputs))
	}
}

func TestDispatchQueuedImageBuildsScanFailureFailsClosed(t *testing.T) {
	app := newImageRegistryTestApp(t)
	queueBuildForDispatch(t, app, "disp-vuln")
	executor := &fakeBuildExecutor{result: buildExecutionResult{
		ImageDigest: "sha256:vuln",
		SBOMDigest:  "sha256:sbom",
		ScanPassed:  false,
		ScanSummary: "3 CRITICAL",
	}}
	if err := dispatchQueuedImageBuilds(t.Context(), app, executor); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	data := dispatchedBuild(t, app, "disp-vuln")
	if shared.TextValue(data, "status") != "failed" || shared.TextValue(data, "scan_status") != "failed" {
		t.Fatalf("scan-fail build = %v, want failed/failed", data)
	}
	if shared.TextValue(data, "signature_status") != "skipped" {
		t.Fatalf("signature_status = %q, want skipped (never sign a vulnerable image)", shared.TextValue(data, "signature_status"))
	}
}

func TestDispatchQueuedImageBuildsExecutorError(t *testing.T) {
	app := newImageRegistryTestApp(t)
	queueBuildForDispatch(t, app, "disp-err")
	executor := &fakeBuildExecutor{err: fmt.Errorf("docker daemon unreachable")}
	if err := dispatchQueuedImageBuilds(t.Context(), app, executor); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	data := dispatchedBuild(t, app, "disp-err")
	if shared.TextValue(data, "status") != "failed" {
		t.Fatalf("status = %q, want failed", shared.TextValue(data, "status"))
	}
	if !strings.Contains(shared.TextValue(data, "logs"), "docker daemon unreachable") {
		t.Fatalf("logs = %q, want executor error recorded", shared.TextValue(data, "logs"))
	}
}

func TestBuildDispatcherRegistrationIsConfigGated(t *testing.T) {
	disabled := platform.NewApp(platform.Config{ServiceName: "image-registry-service", HTTPAddr: ":0"})
	Register(disabled)
	for _, name := range disabled.MaintenanceTaskNames() {
		if name == "image-build-dispatcher" {
			t.Fatal("dispatcher registered without IMAGE_BUILD_EXECUTOR")
		}
	}
	enabled := platform.NewApp(platform.Config{ServiceName: "image-registry-service", HTTPAddr: ":0", ImageBuildExecutor: "docker"})
	Register(enabled)
	found := false
	for _, name := range enabled.MaintenanceTaskNames() {
		if name == "image-build-dispatcher" {
			found = true
		}
	}
	if !found {
		t.Fatal("dispatcher not registered with IMAGE_BUILD_EXECUTOR=docker")
	}
}
