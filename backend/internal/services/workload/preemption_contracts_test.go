package workload

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

func TestWorkloadPreemptionContextRequiresServiceAuth(t *testing.T) {
	app := newWorkloadPreemptionTestApp("svc-key")
	createWorkloadRecord(t, app, jobsResource, map[string]any{
		"id": "j1", "job_id": "j1", "status": "running", "priority_value": 1000, "preemptible": true,
	})

	serveWorkloadPreemption(t, app, http.MethodGet, "/internal/workload/preemption-context", "", "", http.StatusUnauthorized)
	rec := serveWorkloadPreemption(t, app, http.MethodGet, "/internal/workload/preemption-context", "", "svc-key", http.StatusOK)
	data := workloadPreemptionData(t, rec)
	candidates, ok := data["candidates"].([]any)
	if !ok || len(candidates) != 1 {
		t.Fatalf("context data = %#v, want one candidate", data)
	}
}

func TestWorkloadPreemptStatusTransitionIsIdempotent(t *testing.T) {
	app := newWorkloadPreemptionTestApp("svc-key")
	createWorkloadRecord(t, app, testSchedulerReservationsResource, map[string]any{
		"id": "res-preempt", "job_id": "j1", "project_id": "P1", "state": "committed",
	})
	createWorkloadRecord(t, app, jobsResource, map[string]any{
		"id": "j1", "job_id": "j1", "status": "running", "priority_value": 1000, "preemptible": true, "reservation_id": "res-preempt",
	})
	body := `{"preemption_id":"pre-1","requester_job_id":"requester","reason":"test","cleanup":{"pods":1}}`

	serveWorkloadPreemption(t, app, http.MethodPost, "/internal/workload/jobs/j1/preempt", body, "svc-key", http.StatusOK)
	serveWorkloadPreemption(t, app, http.MethodPost, "/internal/workload/jobs/j1/preempt", body, "svc-key", http.StatusOK)

	record, _ := app.Store.Get(context.Background(), jobsResource, "j1")
	if record.Data["status"] != "preempted" || record.Data["preemption_record_id"] != "pre-1" {
		t.Fatalf("preempted record = %#v, want idempotent preempted transition", record.Data)
	}
	reservation, _ := app.Store.Get(context.Background(), testSchedulerReservationsResource, "res-preempt")
	if reservation.Data["state"] != "released" {
		t.Fatalf("preempted reservation = %#v, want released", reservation.Data)
	}
	serveWorkloadPreemption(t, app, http.MethodPost, "/internal/workload/jobs/j1/preempt", strings.ReplaceAll(body, "pre-1", "pre-2"), "svc-key", http.StatusConflict)
}

func TestWorkloadPreemptedStatusIsTerminalForReconcilers(t *testing.T) {
	if statusReconcilerLiveStatuses[jobStatusPreempted] {
		t.Fatal("status reconciler must not treat preempted as live")
	}
	if activeJobStatuses[jobStatusPreempted] {
		t.Fatal("runtime reaper must not overwrite preempted terminal jobs")
	}
}

func newWorkloadPreemptionTestApp(serviceKey string) *platform.App {
	app := platform.NewApp(platform.Config{ServiceName: "all", HTTPAddr: ":0", ServiceAPIKey: serviceKey})
	app.RegisterService(platform.ServiceSpec{Name: serviceName, Routes: []platform.RouteSpec{
		{Method: http.MethodGet, Pattern: "/internal/workload/preemption-context", Resource: "preemption_context", Action: "internal_read", AuthRequired: false},
		{Method: http.MethodPost, Pattern: "/internal/workload/jobs/{id}/preempt", Resource: "jobs", Action: "preempt", AuthRequired: false},
	}})
	registerSchedulerReservationRoutes(app)
	Register(app)
	return app
}

func serveWorkloadPreemption(t *testing.T, app http.Handler, method, target, body, serviceKey string, want int) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if serviceKey != "" {
		req.Header.Set("X-Service-Key", serviceKey)
	}
	app.ServeHTTP(rec, req)
	if rec.Code != want {
		t.Fatalf("%s %s returned %d, want %d: %s", method, target, rec.Code, want, rec.Body.String())
	}
	return rec
}

func workloadPreemptionData(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&envelope); err != nil {
		t.Fatal(err)
	}
	var data map[string]any
	if err := json.Unmarshal(envelope.Data, &data); err != nil {
		t.Fatal(err)
	}
	return data
}
