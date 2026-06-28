package schedulerquota

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/platform/cluster"
	"k8s.io/client-go/kubernetes/fake"
)

func TestComputeGPUReservationDrift(t *testing.T) {
	reserved := map[string]float64{"P1": 1.0, "P2": 0.5, "P3": 0.005}
	observed := map[string]float64{"P1": 1.0, "P2": 0.0, "P4": 0.7}

	got := computeGPUReservationDrift(reserved, observed, gpuReservationDriftTolerance)

	// P1 matches, P3 is within tolerance; only P2 (+0.5) and P4 (-0.7) drift.
	if len(got) != 2 {
		t.Fatalf("drift = %#v, want 2 findings (P2, P4)", got)
	}
	if got[0].ProjectID != "P2" || math.Abs(got[0].DriftGPU-0.5) > 1e-9 {
		t.Fatalf("got[0] = %#v, want P2 drift +0.5", got[0])
	}
	if got[1].ProjectID != "P4" || math.Abs(got[1].DriftGPU+0.7) > 1e-9 {
		t.Fatalf("got[1] = %#v, want P4 drift -0.7", got[1])
	}
}

func TestDetectGPUReservationDriftEmitsEvent(t *testing.T) {
	// No job pods on the cluster => observed GPU is 0.
	app := newSchedulerQuotaTestApp()
	app.Cluster = cluster.New(fake.NewSimpleClientset(), "proj")
	// One active job reserves 1.0 effective GPU (1 GPU @ 100% SM).
	createSchedulerRecord(t, app, workloadJobsResource, map[string]any{
		"id": "J1", "project_id": "P1", "user_id": "U1", "status": "running", "gpu_count": 1, "sm_percentage": 100,
	})
	// A terminated job must not contribute to reserved GPU.
	createSchedulerRecord(t, app, workloadJobsResource, map[string]any{
		"id": "J2", "project_id": "P1", "user_id": "U1", "status": "succeeded", "gpu_count": 4,
	})

	drifts, err := detectGPUReservationDrift(context.Background(), app, newAdmissionReaderForApp(app), time.Now().UTC())
	if err != nil {
		t.Fatalf("detect drift: %v", err)
	}
	if len(drifts) != 1 || drifts[0].ProjectID != "P1" || math.Abs(drifts[0].ReservedGPU-1.0) > 1e-9 || drifts[0].ObservedGPU != 0 {
		t.Fatalf("drifts = %#v, want one P1 reserved 1.0 observed 0", drifts)
	}

	found := false
	for _, event := range app.Events.Outbox() {
		if event.Name == "GPUReservationDriftDetected" {
			found = true
			if event.Data["project_id"] != "P1" {
				t.Fatalf("drift event = %#v, want project_id P1", event.Data)
			}
		}
	}
	if !found {
		t.Fatalf("events = %#v, want a GPUReservationDriftDetected event", app.Events.Outbox())
	}
}

func TestDetectGPUReservationDriftDegradedNoop(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: serviceName})
	drifts, err := detectGPUReservationDrift(context.Background(), app, newAdmissionReaderForApp(app), time.Now().UTC())
	if err != nil || drifts != nil {
		t.Fatalf("degraded detect = (%#v, %v), want (nil, nil)", drifts, err)
	}
}
