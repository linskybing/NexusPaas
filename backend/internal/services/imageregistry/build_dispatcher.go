package imageregistry

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

// defaultImageBuildTimeout bounds a build whose record carries no (or an
// unusable) max_build_time_seconds; the per-build value wins when present.
const defaultImageBuildTimeout = 30 * time.Minute

// registerBuildDispatcher wires the queued-build consumer as a lease-gated
// maintenance task (same pattern as workload-dispatcher). Without a configured
// executor the task is not registered and builds stay queued — dispatch is an
// explicit deployment capability, not an ambient side effect.
func registerBuildDispatcher(app *platform.App) {
	executor := newBuildExecutorFromConfig(app.Config)
	if executor == nil {
		return
	}
	app.RegisterMaintenanceTaskForService(serviceName, "image-build-dispatcher", func(ctx context.Context) error {
		return dispatchQueuedImageBuilds(ctx, app, executor)
	})
}

// dispatchQueuedImageBuilds runs the oldest queued build to completion. One
// build per tick bounds the work a single maintenance cycle can do; the lease
// already guarantees only one replica dispatches at a time.
func dispatchQueuedImageBuilds(ctx context.Context, app *platform.App, executor buildExecutor) error {
	build, ok := oldestQueuedBuild(ctx, app)
	if !ok {
		return nil
	}
	return runImageBuild(ctx, app, executor, build)
}

func oldestQueuedBuild(ctx context.Context, app *platform.App) (contracts.Record[map[string]any], bool) {
	records := app.Store.List(ctx, imageBuildsResource)
	queued := records[:0]
	for _, record := range records {
		if strings.EqualFold(shared.TextValue(record.Data, "status"), "queued") {
			queued = append(queued, record)
		}
	}
	if len(queued) == 0 {
		return contracts.Record[map[string]any]{}, false
	}
	sort.Slice(queued, func(i, j int) bool { return queued[i].CreatedAt.Before(queued[j].CreatedAt) })
	return queued[0], true
}

func runImageBuild(ctx context.Context, app *platform.App, executor buildExecutor, build contracts.Record[map[string]any]) error {
	buildID := build.ID
	if _, ok, err := app.UpdateRecordWithEvent(ctx, imageBuildsResource, buildID, map[string]any{
		"status":     "building",
		"executor":   executor.Name(),
		"updated_at": time.Now().UTC(),
		"logs":       shared.TextValue(build.Data, "logs") + "build dispatched to executor " + executor.Name() + "\n",
	}, func(rec contracts.Record[map[string]any]) contracts.Event {
		return registryEvent(shared.MaintenanceRequest(ctx), "ImageBuildStarted", rec.Data)
	}); err != nil || !ok {
		return fmt.Errorf("mark build %s building: %w", buildID, err)
	}

	input, inputErr := buildExecutionInputFor(ctx, app, build.Data)
	var result buildExecutionResult
	var execErr error
	if inputErr != nil {
		execErr = inputErr
	} else {
		execCtx, cancel := context.WithTimeout(ctx, imageBuildTimeout(build.Data))
		defer cancel()
		result, execErr = executor.Execute(execCtx, input)
	}

	update := buildResultUpdate(build.Data, result, execErr)
	if _, ok, err := app.UpdateRecordWithEvent(ctx, imageBuildsResource, buildID, update, func(rec contracts.Record[map[string]any]) contracts.Event {
		return registryEvent(shared.MaintenanceRequest(ctx), "ImageBuilt", rec.Data)
	}); err != nil || !ok {
		return fmt.Errorf("record build %s result: %w", buildID, err)
	}
	if execErr != nil {
		slog.Error("image build failed", "build_id", buildID, "executor", executor.Name(), "error", execErr)
	}
	return nil
}

// buildExecutionInputFor resolves the build record's source references back
// into executor input: staged context (context_key), inline archive digest
// (re-fetched from the record's stored payload is not possible — inline
// archives are not persisted), and inline dockerfile.
func buildExecutionInputFor(ctx context.Context, app *platform.App, data map[string]any) (buildExecutionInput, error) {
	input := buildExecutionInput{
		BuildID:        shared.TextValue(data, "id"),
		ImageReference: shared.TextValue(data, "image_reference"),
		Dockerfile:     shared.TextValue(data, "dockerfile"),
	}
	if args, ok := data["build_args"].(map[string]any); ok {
		input.BuildArgs = args
	}
	if key := shared.TextValue(data, "context_key"); key != "" {
		if app.ObjectStore == nil {
			return input, fmt.Errorf("build references a staged context but no object store is configured")
		}
		archive, _, err := stagedBuildContextArchive(ctx, app, key)
		if err != nil {
			return input, err
		}
		input.ContextArchive = archive
	}
	return input, nil
}

func imageBuildTimeout(data map[string]any) time.Duration {
	if seconds := shared.IntValue(data, "max_build_time_seconds"); seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	return defaultImageBuildTimeout
}

// buildResultUpdate maps a pipeline outcome onto the build record's
// supply-chain fields. Every terminal state stamps supply_chain_checked_at so
// the publish gate can distinguish "checked and failed" from "never checked".
func buildResultUpdate(data map[string]any, result buildExecutionResult, execErr error) map[string]any {
	now := time.Now().UTC()
	logs := shared.TextValue(data, "logs")
	for _, line := range result.Logs {
		logs += line + "\n"
	}
	update := map[string]any{
		"updated_at":              now,
		"supply_chain_checked_at": now,
		"logs":                    logs,
	}
	switch {
	case execErr != nil:
		update["status"] = "failed"
		update["logs"] = logs + "build failed: " + execErr.Error() + "\n"
		if result.SBOMDigest == "" {
			update["sbom_status"] = "failed"
		} else {
			update["sbom_status"] = "succeeded"
			update["sbom_digest"] = result.SBOMDigest
		}
		update["scan_status"] = failIfPending(shared.TextValue(data, "scan_status"))
		update["signature_status"] = failIfPending(shared.TextValue(data, "signature_status"))
	case !result.ScanPassed:
		update["status"] = "failed"
		update["image_digest"] = result.ImageDigest
		update["sbom_status"] = "succeeded"
		update["sbom_digest"] = result.SBOMDigest
		update["scan_status"] = "failed"
		update["scan_summary"] = result.ScanSummary
		update["signature_status"] = "skipped"
		update["logs"] = logs + "build failed: image scan found HIGH/CRITICAL vulnerabilities\n"
	default:
		update["status"] = "succeeded"
		update["image_digest"] = result.ImageDigest
		update["sbom_status"] = "succeeded"
		update["sbom_digest"] = result.SBOMDigest
		update["scan_status"] = "passed"
		update["scan_summary"] = result.ScanSummary
		update["signature_status"] = "signed"
		update["signature_ref"] = result.SignatureRef
	}
	return update
}

func failIfPending(status string) string {
	if strings.EqualFold(strings.TrimSpace(status), "pending") || status == "" {
		return "failed"
	}
	return status
}
