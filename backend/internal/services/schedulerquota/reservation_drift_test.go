package schedulerquota

import (
	"context"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

func TestReservationDriftDetectorEmitsForTerminalOrMissingJobs(t *testing.T) {
	ctx := context.Background()
	app := newSchedulerQuotaTestApp()
	now := time.Date(2026, 6, 27, 9, 10, 0, 0, time.UTC)
	createSchedulerRecord(t, app, reservationsResource, map[string]any{
		"id": "res-terminal", "job_id": "J-terminal", "project_id": "P1", "state": "committed",
	})
	createSchedulerRecord(t, app, reservationsResource, map[string]any{
		"id": "res-running", "job_id": "J-running", "project_id": "P1", "state": "committed",
	})
	createSchedulerRecord(t, app, reservationsResource, map[string]any{
		"id": "res-released", "job_id": "J-missing", "project_id": "P1", "state": "released",
	})
	createSchedulerRecord(t, app, workloadJobsResource, map[string]any{
		"id": "J-terminal", "job_id": "J-terminal", "project_id": "P1", "status": "completed",
	})
	createSchedulerRecord(t, app, workloadJobsResource, map[string]any{
		"id": "J-running", "job_id": "J-running", "project_id": "P1", "status": "running",
	})

	if err := runReservationDriftDetector(ctx, app, now); err != nil {
		t.Fatal(err)
	}

	events := schedulerEventsByName(app, reservationDriftDetectedEventName)
	if len(events) != 1 {
		t.Fatalf("drift events = %#v, want one terminal-job event", events)
	}
	data := events[0].Data
	if data["reservation_id"] != "res-terminal" || data["drift_reason"] != reservationDriftReasonJobTerminal || data["job_exists"] != true || data["job_status"] != "completed" {
		t.Fatalf("drift event data = %#v, want terminal reservation drift", data)
	}
}

func TestReservationDriftDetectorEmitsForMissingJob(t *testing.T) {
	ctx := context.Background()
	app := newSchedulerQuotaTestApp()
	now := time.Date(2026, 6, 27, 9, 11, 0, 0, time.UTC)
	createSchedulerRecord(t, app, reservationsResource, map[string]any{
		"id": "res-missing", "job_id": "J-missing", "project_id": "P1", "state": "reserved",
	})

	if err := runReservationDriftDetector(ctx, app, now); err != nil {
		t.Fatal(err)
	}

	events := schedulerEventsByName(app, reservationDriftDetectedEventName)
	if len(events) != 1 {
		t.Fatalf("drift events = %#v, want one missing-job event", events)
	}
	if events[0].Data["reservation_id"] != "res-missing" || events[0].Data["drift_reason"] != reservationDriftReasonJobMissing || events[0].Data["job_exists"] != false {
		t.Fatalf("missing drift event data = %#v, want missing job drift", events[0].Data)
	}
}

func TestReservationDriftDetectorSkipsWhenWorkloadJobsUnavailable(t *testing.T) {
	ctx := context.Background()
	app := newSchedulerQuotaTestApp()
	now := time.Date(2026, 6, 27, 9, 12, 0, 0, time.UTC)
	createSchedulerRecord(t, app, reservationsResource, map[string]any{
		"id": "res-active", "job_id": "J-unreadable", "project_id": "P1", "state": "committed",
	})
	reader := unavailableWorkloadJobsReader{
		storeAdmissionReader: storeAdmissionReader{store: app.Store, scheduler: schedulerQuotaRepositoryForApp(app)},
	}

	if err := runReservationDriftDetectorWithReader(ctx, app, reader, now); err != nil {
		t.Fatal(err)
	}

	if events := schedulerEventsByName(app, reservationDriftDetectedEventName); len(events) != 0 {
		t.Fatalf("drift events = %#v, want no-op when workload jobs are unavailable", events)
	}
}

type unavailableWorkloadJobsReader struct {
	storeAdmissionReader
}

func (r unavailableWorkloadJobsReader) ListWorkloadJobsAvailable(context.Context) ([]admissionRecord, bool) {
	return nil, false
}

func schedulerEventsByName(app *platform.App, name string) []contracts.Event {
	events := []contracts.Event{}
	for _, event := range app.Events.Outbox() {
		if event.Name == name {
			events = append(events, event)
		}
	}
	return events
}
