package auditcompliance

import (
	"context"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

func TestCleanupOldAuditLogsDeletesOnlyExpiredRecords(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: serviceName})
	ctx := context.Background()
	now := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)

	seed := func(id string, createdAt time.Time) {
		row := map[string]any{"id": id, "action": "login", "created_at": createdAt.Format(time.RFC3339)}
		if _, err := app.Store.Create(ctx, auditLogResource, row); err != nil {
			t.Fatalf("seed %s: %v", id, err)
		}
	}
	seed("old", now.AddDate(0, 0, -40)) // older than 30d -> removed
	seed("edge", now.AddDate(0, 0, -31))
	seed("fresh", now.AddDate(0, 0, -5)) // within retention -> kept

	removed := CleanupOldAuditLogs(ctx, app.Store, 30, now)
	if removed != 2 {
		t.Fatalf("removed = %d, want 2", removed)
	}
	remaining := app.Store.List(ctx, auditLogResource)
	if len(remaining) != 1 || remaining[0].ID != "fresh" {
		t.Fatalf("remaining = %+v, want only fresh", remaining)
	}
}

func TestCleanupOldAuditLogsFallsBackToRecordTimestamp(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: serviceName})
	ctx := context.Background()
	now := time.Now().UTC()

	// No created_at field in data -> falls back to store record metadata, which is
	// "now", so nothing is older than retention and nothing is removed.
	if _, err := app.Store.Create(ctx, auditLogResource, map[string]any{"id": "x", "action": "login"}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if removed := CleanupOldAuditLogs(ctx, app.Store, 30, now); removed != 0 {
		t.Fatalf("removed = %d, want 0 (record is fresh)", removed)
	}
}

func TestCleanupOldAuditLogsDefaultsRetentionWhenUnset(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: serviceName})
	ctx := context.Background()
	now := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
	old := map[string]any{"id": "old", "created_at": now.AddDate(0, 0, -45).Format(time.RFC3339)}
	if _, err := app.Store.Create(ctx, auditLogResource, old); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if removed := CleanupOldAuditLogs(ctx, app.Store, 0, now); removed != 1 {
		t.Fatalf("removed = %d, want 1 with default 30d retention", removed)
	}
}

func TestRegisterAuditRetentionRunsViaMaintenance(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: serviceName, AuditRetentionDays: 30})
	ctx := context.Background()
	old := map[string]any{"id": "old", "created_at": time.Now().UTC().AddDate(0, 0, -90).Format(time.RFC3339)}
	if _, err := app.Store.Create(ctx, auditLogResource, old); err != nil {
		t.Fatalf("seed: %v", err)
	}
	Register(app)
	// One maintenance cycle should acquire the lease and run the retention task.
	app.RunMaintenanceOnce(ctx, time.Minute)
	if got := len(app.Store.List(ctx, auditLogResource)); got != 0 {
		t.Fatalf("expired audit log not reaped: %d remaining", got)
	}
}
