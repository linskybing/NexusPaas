package workload

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

type reservationReleaseFunc func(context.Context, http.Header, string) (schedulerReservationResult, error)

func schedulerReservationReleaseFuncForApp(app *platform.App) reservationReleaseFunc {
	client, err := newSchedulerReservationClient(app)
	if err != nil {
		slog.Warn("workload reservation release disabled", "error", err)
		return nil
	}
	return client.Release
}

func releaseJobReservation(ctx context.Context, release reservationReleaseFunc, headers http.Header, job map[string]any) {
	if release == nil || len(job) == 0 {
		return
	}
	reservationID := strings.TrimSpace(shared.TextValue(job, "reservation_id", "reservationId"))
	if reservationID == "" {
		return
	}
	if _, err := release(ctx, headers, reservationID); err != nil {
		slog.Warn("workload reservation release failed",
			"reservation_id", reservationID,
			"job_id", shared.TextValue(job, "job_id", "jobId", "id"),
			"error", err,
		)
	}
}

func releaseSubmittedJobReservation(ctx context.Context, app *platform.App, headers http.Header, job map[string]any) {
	releaseJobReservation(ctx, schedulerReservationReleaseFuncForApp(app), headers, job)
}

func reservationRecordID(record contracts.Record[map[string]any]) string {
	return strings.TrimSpace(shared.FirstNonEmpty(record.ID, shared.TextValue(record.Data, "id")))
}
