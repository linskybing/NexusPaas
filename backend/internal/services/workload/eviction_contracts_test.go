package workload

import (
	"context"
	"net/http"
	"testing"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

func newWorkloadEvictionTestApp(serviceKey string) *platform.App {
	app := platform.NewApp(platform.Config{ServiceName: "all", HTTPAddr: ":0", ServiceAPIKey: serviceKey})
	app.RegisterService(platform.ServiceSpec{Name: serviceName, Routes: []platform.RouteSpec{
		{Method: http.MethodPost, Pattern: "/internal/workload/jobs/{id}/evict", Resource: "jobs", Action: "evict", AuthRequired: false},
	}})
	registerSchedulerReservationRoutes(app)
	Register(app)
	return app
}

func TestWorkloadEvictRequiresServiceAuth(t *testing.T) {
	app := newWorkloadEvictionTestApp("svc-key")
	createWorkloadRecord(t, app, jobsResource, map[string]any{"id": "j1", "job_id": "j1", "status": "running"})

	serveWorkloadPreemption(t, app, http.MethodPost, "/internal/workload/jobs/j1/evict", `{"reason":"x"}`, "", http.StatusUnauthorized)
	serveWorkloadPreemption(t, app, http.MethodPost, "/internal/workload/jobs/j1/evict", `{"reason":"x"}`, "svc-key", http.StatusOK)
}

func TestWorkloadEvictTransitionIsIdempotent(t *testing.T) {
	app := newWorkloadEvictionTestApp("svc-key")
	createWorkloadRecord(t, app, testSchedulerReservationsResource, map[string]any{
		"id": "res-evict", "job_id": "j1", "project_id": "P1", "state": "committed",
	})
	createWorkloadRecord(t, app, jobsResource, map[string]any{"id": "j1", "job_id": "j1", "status": "queued", "reservation_id": "res-evict"})
	body := `{"reason":"Plan window expired"}`

	serveWorkloadPreemption(t, app, http.MethodPost, "/internal/workload/jobs/j1/evict", body, "svc-key", http.StatusOK)
	serveWorkloadPreemption(t, app, http.MethodPost, "/internal/workload/jobs/j1/evict", body, "svc-key", http.StatusOK)

	record, _ := app.Store.Get(context.Background(), jobsResource, "j1")
	if record.Data["status"] != jobStatusEvicted {
		t.Fatalf("evicted record = %#v, want status evicted", record.Data)
	}
	if record.Data["status_reason"] != "Plan window expired" {
		t.Fatalf("status_reason = %v, want plan window reason", record.Data["status_reason"])
	}
	reservation, _ := app.Store.Get(context.Background(), testSchedulerReservationsResource, "res-evict")
	if reservation.Data["state"] != "released" {
		t.Fatalf("evicted reservation = %#v, want released", reservation.Data)
	}
}

func TestWorkloadEvictProtectsTerminalStatus(t *testing.T) {
	app := newWorkloadEvictionTestApp("svc-key")
	createWorkloadRecord(t, app, jobsResource, map[string]any{"id": "done", "job_id": "done", "status": "completed"})

	serveWorkloadPreemption(t, app, http.MethodPost, "/internal/workload/jobs/done/evict", `{"reason":"x"}`, "svc-key", http.StatusConflict)

	record, _ := app.Store.Get(context.Background(), jobsResource, "done")
	if record.Data["status"] != "completed" {
		t.Fatalf("terminal job mutated: %#v", record.Data)
	}
}

func TestWorkloadEvictUnknownJobIsNotFound(t *testing.T) {
	app := newWorkloadEvictionTestApp("svc-key")
	serveWorkloadPreemption(t, app, http.MethodPost, "/internal/workload/jobs/missing/evict", `{"reason":"x"}`, "svc-key", http.StatusNotFound)
}

func TestWorkloadEvictedStatusIsTerminalForReconcilers(t *testing.T) {
	if statusReconcilerLiveStatuses[jobStatusEvicted] {
		t.Fatal("status reconciler must not treat evicted as live")
	}
	if activeJobStatuses[jobStatusEvicted] {
		t.Fatal("runtime reaper must not overwrite evicted terminal jobs")
	}
	if !terminalJobStatuses[jobStatusEvicted] {
		t.Fatal("evicted must be a terminal job status")
	}
}
