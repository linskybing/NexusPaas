package auditcompliance

import (
	"context"
	"log/slog"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

// defaultRetentionDays mirrors the reference backend's audit retention default
// (references/.../internal/cron/cron.go: 30 days when unset).
const defaultRetentionDays = 30

// CleanupOldAuditLogs deletes audit_logs records whose created_at is older than
// retentionDays before now. It returns the number of records removed. This is the
// directly-testable core of the audit retention worker and is the microservice
// port of references/.../internal/application/audit.CleanupOldLogs ->
// repository.DeleteOldAuditLogs (delete WHERE created_at < cutoff).
func CleanupOldAuditLogs(ctx context.Context, store platform.RecordStore, retentionDays int, now time.Time) int {
	if store == nil {
		return 0
	}
	if retentionDays <= 0 {
		retentionDays = defaultRetentionDays
	}
	cutoff := now.UTC().AddDate(0, 0, -retentionDays)
	removed := 0
	for _, record := range store.List(ctx, auditLogResource) {
		createdAt := auditRecordCreatedAt(record.Data, record.CreatedAt)
		if createdAt.IsZero() || !createdAt.Before(cutoff) {
			continue
		}
		if store.Delete(ctx, auditLogResource, record.ID) {
			removed++
		}
	}
	return removed
}

// auditRecordCreatedAt resolves the record's effective creation time, preferring an
// explicit created_at field in the data and falling back to the store metadata
// timestamp, matching how auditLogs() reads records for the read path.
func auditRecordCreatedAt(data map[string]any, fallback time.Time) time.Time {
	if created := timeValue(data, "created_at", "createdAt"); !created.IsZero() {
		return created
	}
	return fallback
}

// registerAuditRetention wires periodic audit-log retention cleanup as a
// lease-gated maintenance task. It runs only once StartMaintenance is called from
// the composition root, so building an App in tests spawns no background work.
// This is the microservice equivalent of references/.../internal/cron.StartCleanupTask.
func registerAuditRetention(app *platform.App) {
	app.RegisterMaintenanceTaskForService(serviceName, "audit-log-retention", func(ctx context.Context) error {
		removed := CleanupOldAuditLogs(ctx, app.Store, app.Config.AuditRetentionDays, time.Now())
		if removed > 0 {
			slog.Info("audit retention removed expired logs", "count", removed, "retention_days", app.Config.AuditRetentionDays)
		}
		return nil
	})
}
