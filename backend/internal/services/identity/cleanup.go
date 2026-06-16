package identity

import (
	"context"
	"log/slog"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

// CleanupExpiredAuthRecords deletes expired sessions and refresh tokens, plus
// expired or revoked API tokens, from the store. It returns the number of records
// removed. Run periodically so disused credentials do not accumulate (finding 1).
func CleanupExpiredAuthRecords(ctx context.Context, store platform.RecordStore) int {
	if store == nil {
		return 0
	}
	return newRecordStoreIdentityAuthRepository(store).CleanupExpiredAuthRecords(ctx, time.Now().UTC())
}

// registerAuthCleanup wires periodic expired-credential cleanup as a lease-gated
// maintenance task. It runs only once StartMaintenance is called from the
// composition root, so building an App in tests spawns no background work.
func registerAuthCleanup(app *platform.App) {
	app.RegisterMaintenanceTaskForService(serviceName, "identity-auth-cleanup", func(ctx context.Context) error {
		if removed := CleanupExpiredAuthRecords(ctx, app.Store); removed > 0 {
			slog.Info("auth cleanup removed expired credentials", "count", removed)
		}
		return nil
	})
}
