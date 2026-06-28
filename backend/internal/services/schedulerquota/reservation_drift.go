package schedulerquota

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

const (
	reservationsResource                = serviceName + ":reservations"
	reservationDriftTaskName            = "reservation-drift-detector"
	reservationDriftDetectedEventName   = "ReservationDriftDetected"
	reservationDriftReasonJobMissing    = "missing_job_for_reservation"
	reservationDriftReasonJobTerminal   = "terminal_job_but_reservation_not_released"
	reservationDriftTraceIDPrefix       = "reservation-drift-"
	reservationDriftIdempotencyKeyScope = reservationDriftTaskName + ":"
)

var reservationDriftTerminalStatuses = map[string]bool{
	"completed": true,
	"failed":    true,
	"cancelled": true,
	"canceled":  true,
	"preempted": true,
	"evicted":   true,
}

func registerReservationDriftDetector(app *platform.App) {
	app.RegisterMaintenanceTaskForService(serviceName, reservationDriftTaskName, func(ctx context.Context) error {
		return runReservationDriftDetector(ctx, app, time.Now().UTC())
	})
}

func runReservationDriftDetector(ctx context.Context, app *platform.App, detectedAt time.Time) error {
	return runReservationDriftDetectorWithReader(ctx, app, newAdmissionReaderForApp(app), detectedAt)
}

func runReservationDriftDetectorWithReader(ctx context.Context, app *platform.App, reader admissionReader, detectedAt time.Time) error {
	if app == nil || app.Store == nil {
		return nil
	}
	if detectedAt.IsZero() {
		detectedAt = time.Now().UTC()
	}
	slog.Info("reservation drift detector started", "detected_at", detectedAt.UTC().Format(time.RFC3339))
	jobs, available := reservationDriftJobIndex(ctx, reader)
	if !available {
		slog.Warn("reservation drift detector skipped", "reason", "workload owner-read unavailable")
		return nil
	}
	activeReservations := 0
	driftCount := 0
	for _, reservation := range app.Store.List(ctx, reservationsResource) {
		state := strings.ToLower(shared.TextValue(reservation.Data, "state"))
		if state != "reserved" && state != "committed" {
			continue
		}
		activeReservations++
		drift, ok := reservationDriftData(reservation, jobs, state, detectedAt)
		if !ok {
			continue
		}
		driftCount++
		if err := publishReservationDriftDetected(ctx, app.Events, drift, detectedAt); err != nil {
			return err
		}
	}
	slog.Info("reservation drift detector completed",
		"active_reservations", activeReservations,
		"workload_jobs", len(jobs),
		"drift_count", driftCount,
	)
	return nil
}

func reservationDriftJobIndex(ctx context.Context, reader admissionReader) (map[string]admissionRecord, bool) {
	out := map[string]admissionRecord{}
	if reader == nil {
		return out, false
	}
	records, available := reservationDriftWorkloadJobs(ctx, reader)
	if !available {
		return out, false
	}
	for _, job := range records {
		for _, id := range []string{job.ID, shared.TextValue(job.Data, "job_id", "jobId", "id")} {
			id = strings.TrimSpace(id)
			if id != "" {
				out[id] = job
			}
		}
	}
	return out, true
}

func reservationDriftWorkloadJobs(ctx context.Context, reader admissionReader) ([]admissionRecord, bool) {
	if availableReader, ok := reader.(workloadJobAvailabilityReader); ok {
		return availableReader.ListWorkloadJobsAvailable(ctx)
	}
	return reader.ListWorkloadJobs(ctx), true
}

func reservationDriftData(reservation contracts.Record[map[string]any], jobs map[string]admissionRecord, state string, detectedAt time.Time) (map[string]any, bool) {
	jobID := shared.TextValue(reservation.Data, "job_id", "jobId")
	data := map[string]any{
		"reservation_id":    reservation.ID,
		"reservation_state": state,
		"project_id":        shared.TextValue(reservation.Data, "project_id", "projectId"),
		"job_id":            jobID,
		"job_exists":        false,
		"detected_at":       detectedAt.UTC().Format(time.RFC3339),
	}
	if strings.TrimSpace(jobID) == "" {
		data["drift_reason"] = reservationDriftReasonJobMissing
		data["reason"] = reservationDriftReasonJobMissing
		data["job_id_missing"] = true
		return data, true
	}
	job, found := jobs[jobID]
	if !found {
		data["drift_reason"] = reservationDriftReasonJobMissing
		data["reason"] = reservationDriftReasonJobMissing
		data["job_missing"] = true
		return data, true
	}
	data["job_exists"] = true
	status := strings.ToLower(shared.TextValue(job.Data, "status", "Status"))
	if !reservationDriftTerminalStatuses[status] {
		return nil, false
	}
	data["drift_reason"] = reservationDriftReasonJobTerminal
	data["reason"] = reservationDriftReasonJobTerminal
	data["job_status"] = status
	data["job_record_id"] = job.ID
	return data, true
}

func publishReservationDriftDetected(ctx context.Context, events platform.EventStream, data map[string]any, detectedAt time.Time) error {
	if events == nil {
		return nil
	}
	reservationID := shared.TextValue(data, "reservation_id")
	event := contracts.Event{
		EventID:        platform.NewUUID(),
		Name:           reservationDriftDetectedEventName,
		Source:         serviceName,
		OccurredAt:     detectedAt,
		TraceID:        reservationDriftTraceIDPrefix + detectedAt.Format("20060102150405"),
		SchemaVersion:  1,
		IdempotencyKey: reservationDriftIdempotencyKeyScope + reservationID + ":" + shared.TextValue(data, "drift_reason", "reason") + ":" + detectedAt.Format(time.RFC3339Nano),
		Data:           shared.CloneMap(data),
	}
	if err := events.Publish(ctx, event); err != nil {
		return fmt.Errorf("reservation drift event publish failed: %w", err)
	}
	return nil
}
