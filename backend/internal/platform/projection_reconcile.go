package platform

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
)

// ProjectionReconcilerSpec wires one read-model family into the periodic
// drift→replay reconcile job (DATA-016/DATA-018): a lease-gated maintenance
// task measures drift between owner data and the local read model, and on
// drift resets the projection consumer(s) and replays the event stream to
// rebuild it.
type ProjectionReconcilerSpec struct {
	// Owner is the logical service that owns the read model; the task is only
	// registered when the current process hosts it.
	Owner string
	// Consumers are the projection consumers whose idempotency state is reset
	// for a rebuild (most families have one; authorization-policy splits its
	// read models across two).
	Consumers []string
	// Drift returns the current number of drifting rows (missing + orphan +
	// stale) across the family's read-model pairs.
	Drift func(ctx context.Context) (int, error)
	// Sync replays the event stream into the read model (the family's
	// RunProjection wrapper).
	Sync func(ctx context.Context)
}

// RegisterProjectionReconciler registers the drift→replay maintenance task for
// one read-model family. Every tick it first replays from the checkpoint (so
// ordinary consumer lag is not misreported as drift), then measures drift and
// exposes it as the projection_drift gauge; when drift remains it publishes
// ProjectionDriftDetected, resets the consumers, replays the stream, re-measures,
// and publishes ProjectionRebuilt with the before/after counts. Residual drift
// (rows the event stream cannot reproduce) is left for operators — while the
// drift count stays at the residual left by the last rebuild, later ticks only
// report it instead of looping a rebuild that cannot converge.
func (a *App) RegisterProjectionReconciler(spec ProjectionReconcilerSpec) {
	if len(spec.Consumers) == 0 || spec.Drift == nil || spec.Sync == nil {
		return
	}
	label := strings.Join(spec.Consumers, "+")
	residual := -1
	a.RegisterMaintenanceTaskForService(spec.Owner, "projection-reconcile:"+label, func(ctx context.Context) error {
		spec.Sync(ctx)
		before, err := spec.Drift(ctx)
		if err != nil {
			return err
		}
		a.Metrics.SetGauge("projection_drift", map[string]string{"consumer": label}, int64(before))
		if before == 0 || before == residual {
			residual = before
			return nil
		}
		slog.Warn("projection drift detected; rebuilding read model",
			"consumer", label, "drift", before)
		a.publishProjectionEvent(ctx, spec, "ProjectionDriftDetected", map[string]any{
			"consumer": label, "drift": before,
		})
		for _, consumer := range spec.Consumers {
			a.Events.ResetConsumer(consumer)
		}
		spec.Sync(ctx)
		after, err := spec.Drift(ctx)
		if err != nil {
			return err
		}
		residual = after
		a.Metrics.SetGauge("projection_drift", map[string]string{"consumer": label}, int64(after))
		a.publishProjectionEvent(ctx, spec, "ProjectionRebuilt", map[string]any{
			"consumer": label, "drift_before": before, "drift_after": after,
		})
		slog.Info("projection rebuild finished",
			"consumer", label, "drift_before", before, "drift_after", after)
		return nil
	})
}

func (a *App) publishProjectionEvent(ctx context.Context, spec ProjectionReconcilerSpec, name string, data map[string]any) {
	if a.Events == nil {
		return
	}
	if err := a.Events.Publish(ctx, contracts.Event{
		EventID:       NewUUID(),
		Name:          name,
		Source:        spec.Owner,
		OccurredAt:    time.Now().UTC(),
		TraceID:       NewUUID(),
		SchemaVersion: 1,
		Data:          data,
	}); err != nil {
		slog.Error(eventPublishFailedLogMsg, "event", name, "consumer", strings.Join(spec.Consumers, "+"), "error", err)
	}
}
