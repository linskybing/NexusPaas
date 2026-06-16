package imageregistry

import (
	"context"
	"log/slog"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

const (
	harborHealthResource  = serviceName + ":harbor_health"
	harborHealthOperation = "harborHealth"
)

// checkHarborHealth probes Harbor admin connectivity through the harbor adapter and
// records the latest result in the harbor_health read model. It is the microservice
// port of reference cron.StartHarborHealthChecks + image.CheckHarborAdminConnectivity:
// the cron loop logged connectivity; here the lease-gated maintenance loop persists
// a single health record so operators (and future /harbor-status reads) see the
// last probe outcome.
//
// A nil adapter or store (degraded mode, harbor not configured) is a no-op so a
// transient absence never overwrites the last good result with a false negative.
func checkHarborHealth(ctx context.Context, adapter contracts.ExternalAdapter, store platform.RecordStore, now time.Time) error {
	if adapter == nil || store == nil {
		return nil
	}
	probeCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	result, err := adapter.Call(probeCtx, harborHealthOperation, true)
	healthy := err == nil && !result.Degraded
	record := map[string]any{
		"healthy":    healthy,
		"checked_at": now,
		"code":       result.Code,
		"message":    result.Message,
	}
	if err != nil {
		record["message"] = err.Error()
		slog.Warn("harbor health check failed", "error", err)
	} else if !healthy {
		slog.Warn("harbor health check degraded", "code", result.Code, "message", result.Message)
	}
	upsertHarborHealth(ctx, store, record)
	return nil
}

// upsertHarborHealth keeps a single harbor_health record, updating the newest in
// place so the table does not grow unbounded across ticks.
func upsertHarborHealth(ctx context.Context, store platform.RecordStore, data map[string]any) {
	records := store.List(ctx, harborHealthResource)
	if len(records) > 0 {
		if _, ok := store.Update(ctx, harborHealthResource, records[len(records)-1].ID, data); ok {
			return
		}
	}
	_, _ = store.Create(ctx, harborHealthResource, data)
}

// registerHarborHealthChecks wires the Harbor health probe as a lease-gated
// maintenance task. It runs only once StartMaintenance is called.
func registerHarborHealthChecks(app *platform.App) {
	app.RegisterMaintenanceTaskForService(serviceName, "harbor-health", func(ctx context.Context) error {
		return checkHarborHealth(ctx, app.Adapters["harbor"], app.Store, time.Now().UTC())
	})
}
