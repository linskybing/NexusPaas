package storage

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/platform/cluster"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

const (
	longhornRWXHealthTask   = "longhorn-rwx-health"
	longhornRWXHealthRecord = "latest"
	longhornRWXHealthEvent  = "LonghornRWXHealthChecked"
)

func registerLonghornRWXHealthWorker(app *platform.App) {
	app.RegisterMaintenanceTaskForService(serviceName, longhornRWXHealthTask, func(ctx context.Context) error {
		return runLonghornRWXHealth(ctx, app)
	})
}

func runLonghornRWXHealth(ctx context.Context, app *platform.App) error {
	summary := longhornRWXSummary(ctx, app)
	data := longhornRWXSummaryData(app.Config, summary)
	if err := persistLonghornRWXSummary(ctx, app, data); err != nil {
		return err
	}
	if err := publishLonghornRWXSummary(ctx, app, data); err != nil {
		return err
	}
	logLonghornRWXSummary(summary)
	return nil
}

func longhornRWXSummary(ctx context.Context, app *platform.App) cluster.LonghornRWXSummary {
	if app.Cluster == nil {
		return cluster.LonghornRWXSummary{
			Namespace: app.Config.LonghornNamespace,
			Degraded:  true,
			Failed:    1,
			Error:     "cluster client unavailable",
			Results: []cluster.LonghornRWXVolumeStatus{{
				Namespace: app.Config.LonghornNamespace,
				Error:     "cluster client unavailable",
			}},
		}
	}
	return app.Cluster.ReconcileLonghornRWXVolumes(ctx, cluster.LonghornRWXOptions{
		Namespace:          app.Config.LonghornNamespace,
		AutoRepairEnabled:  app.Config.LonghornRWXAutoRepair,
		RepairCooldown:     app.Config.LonghornRWXRepairCooldown,
		SnapshotWarnLimit:  app.Config.LonghornRWXSnapshotWarn,
		SnapshotBlockLimit: app.Config.LonghornRWXSnapshotBlock,
	})
}

func longhornRWXSummaryData(cfg platform.Config, summary cluster.LonghornRWXSummary) map[string]any {
	checkedAt := time.Now().UTC().Format(time.RFC3339)
	return map[string]any{
		"id":                         longhornRWXHealthRecord,
		"checked_at":                 checkedAt,
		"longhorn_namespace":         shared.FirstNonBlank(summary.Namespace, cfg.LonghornNamespace),
		"auto_repair_enabled":        cfg.LonghornRWXAutoRepair,
		"repair_cooldown_seconds":    int(cfg.LonghornRWXRepairCooldown.Seconds()),
		"snapshot_warn_limit":        cfg.LonghornRWXSnapshotWarn,
		"snapshot_block_limit":       cfg.LonghornRWXSnapshotBlock,
		"volumes_checked":            summary.Checked,
		"unavailable_count":          summary.Unavailable,
		"unhealthy_count":            summary.Unhealthy,
		"endpoint_unavailable_count": summary.EndpointUnavailable,
		"snapshot_warn_count":        summary.SnapshotWarning,
		"snapshot_block_count":       summary.SnapshotBlocked,
		"repair_attempted_count":     summary.RepairAttempted,
		"repair_succeeded_count":     summary.RepairSucceeded,
		"repair_skipped_count":       summary.RepairSkipped,
		"failed_count":               summary.Failed,
		"degraded":                   summary.Degraded,
		"error":                      summary.Error,
		"results":                    longhornRWXVolumeRows(summary.Results),
	}
}

func longhornRWXVolumeRows(results []cluster.LonghornRWXVolumeStatus) []map[string]any {
	rows := make([]map[string]any, 0, len(results))
	for _, result := range results {
		rows = append(rows, map[string]any{
			"volume":           result.Volume,
			"namespace":        result.Namespace,
			"robustness":       result.Robustness,
			"endpoint_mode":    result.EndpointMode,
			"endpoint_ready":   result.EndpointReady,
			"available":        result.Available,
			"active_snapshots": result.ActiveSnapshots,
			"snapshot_warning": result.SnapshotWarning,
			"snapshot_blocked": result.SnapshotBlocked,
			"active_consumers": result.ActiveConsumers,
			"in_cooldown":      result.InCooldown,
			"repaired":         result.Repaired,
			"repair_action":    result.RepairAction,
			"skipped":          result.Skipped,
			"error":            result.Error,
		})
	}
	return rows
}

func persistLonghornRWXSummary(ctx context.Context, app *platform.App, data map[string]any) error {
	if updated, ok := app.Store.Update(ctx, longhornRWXHealthResource, longhornRWXHealthRecord, data); ok {
		_ = updated
		return nil
	}
	if _, err := app.Store.Create(ctx, longhornRWXHealthResource, data); err != nil {
		if platform.IsCreateConflict(err) {
			if _, ok := app.Store.Update(ctx, longhornRWXHealthResource, longhornRWXHealthRecord, data); ok {
				return nil
			}
		}
		return fmt.Errorf("persist Longhorn RWX health summary: %w", err)
	}
	return nil
}

func publishLonghornRWXSummary(ctx context.Context, app *platform.App, data map[string]any) error {
	if err := app.Events.Publish(ctx, contracts.Event{
		EventID:       platform.NewUUID(),
		Name:          longhornRWXHealthEvent,
		Source:        serviceName,
		OccurredAt:    time.Now().UTC(),
		TraceID:       platform.NewUUID(),
		SchemaVersion: 1,
		Data:          shared.CloneMap(data),
	}); err != nil {
		return fmt.Errorf("publish Longhorn RWX health event: %w", err)
	}
	return nil
}

func logLonghornRWXSummary(summary cluster.LonghornRWXSummary) {
	if summary.Degraded || summary.Unavailable > 0 || summary.Failed > 0 || summary.RepairSucceeded > 0 {
		slog.Warn("Longhorn RWX health check completed",
			"namespace", summary.Namespace,
			"checked", summary.Checked,
			"degraded", summary.Degraded,
			"unavailable", summary.Unavailable,
			"failed", summary.Failed,
			"repair_succeeded", summary.RepairSucceeded,
			"error", summary.Error)
	}
}
