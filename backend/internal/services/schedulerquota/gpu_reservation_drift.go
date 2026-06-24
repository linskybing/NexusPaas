package schedulerquota

import (
	"context"
	"log/slog"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

// gpuReservationDriftTolerance is the smallest reserved-vs-observed GPU gap (in
// whole GPUs) worth alerting on. Below it the difference is treated as benign
// scheduling lag rather than drift.
const gpuReservationDriftTolerance = 0.01

type gpuReservationDrift struct {
	ProjectID   string
	ReservedGPU float64
	ObservedGPU float64
	DriftGPU    float64 // reserved - observed
}

// computeGPUReservationDrift (GPU-015) compares the GPU each project reserved
// through admission with the GPU actually observed on the cluster, returning the
// projects whose gap exceeds tolerance, sorted by project id for determinism.
func computeGPUReservationDrift(reserved, observed map[string]float64, tolerance float64) []gpuReservationDrift {
	projects := make(map[string]struct{}, len(reserved)+len(observed))
	for id := range reserved {
		projects[id] = struct{}{}
	}
	for id := range observed {
		projects[id] = struct{}{}
	}
	out := make([]gpuReservationDrift, 0, len(projects))
	for id := range projects {
		drift := reserved[id] - observed[id]
		if math.Abs(drift) <= tolerance {
			continue
		}
		out = append(out, gpuReservationDrift{ProjectID: id, ReservedGPU: reserved[id], ObservedGPU: observed[id], DriftGPU: drift})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ProjectID < out[j].ProjectID })
	return out
}

// reservedGPUByProject sums the effective GPU of every active workload job per
// project using the same gpu_count * sm_percentage / 100 accounting admission
// applies.
func reservedGPUByProject(ctx context.Context, reader admissionReader) map[string]float64 {
	reserved := map[string]float64{}
	for _, job := range reader.ListWorkloadJobs(ctx) {
		status := strings.ToLower(shared.TextValue(job.Data, "status", "Status"))
		if !activeAdmissionStatus(status) {
			continue
		}
		projectID := shared.TextValue(job.Data, "project_id", "projectId", "ProjectID")
		if projectID == "" {
			continue
		}
		reserved[projectID] += jobEffectiveGPU(job.Data)
	}
	return reserved
}

func jobEffectiveGPU(job map[string]any) float64 {
	if count := shared.IntValue(job, "gpu_count", "gpuCount", "GPUCount"); count > 0 {
		smPct := 100
		if raw, ok := firstPresent(job, "sm_percentage", "smPercentage", "SMPercentage"); ok {
			if pct := int(getInt64(raw, 0)); pct > 0 {
				smPct = pct
			}
		}
		return gpuFraction(count, smPct)
	}
	return shared.NumberValue(job, "required_gpu", "requiredGpu", "RequiredGPU")
}

// observedGPUByProject sums the DRA effective GPU of every active job pod the
// cluster reports (pods carry the platform-go/dra-effective-gpu label).
func observedGPUByProject(ctx context.Context, app *platform.App, now time.Time) (map[string]float64, error) {
	observed := map[string]float64{}
	usages, err := app.Cluster.ListJobPodResourceUsage(ctx, now)
	if err != nil {
		return nil, err
	}
	for _, usage := range usages {
		if !usage.IsActive || usage.ProjectID == "" {
			continue
		}
		observed[usage.ProjectID] += usage.RequestedGPU
	}
	return observed, nil
}

// detectGPUReservationDrift gathers reserved (scheduler view) and observed
// (cluster view) GPU per project, then logs and publishes a
// GPUReservationDriftDetected event for each drifting project. A nil cluster
// (degraded mode) is a no-op, mirroring the resource quota reconciler.
func detectGPUReservationDrift(ctx context.Context, app *platform.App, reader admissionReader, now time.Time) ([]gpuReservationDrift, error) {
	if app == nil || app.Cluster == nil || app.Store == nil || reader == nil {
		return nil, nil
	}
	observed, err := observedGPUByProject(ctx, app, now)
	if err != nil {
		return nil, err
	}
	reserved := reservedGPUByProject(ctx, reader)
	drifts := computeGPUReservationDrift(reserved, observed, gpuReservationDriftTolerance)
	for _, drift := range drifts {
		slog.Warn("gpu reservation drift detected",
			"project_id", drift.ProjectID,
			"reserved_gpu", drift.ReservedGPU,
			"observed_gpu", drift.ObservedGPU,
			"drift_gpu", drift.DriftGPU)
		publishGPUReservationDrift(ctx, app, drift)
	}
	return drifts, nil
}

func publishGPUReservationDrift(ctx context.Context, app *platform.App, drift gpuReservationDrift) {
	if app == nil || app.Events == nil {
		return
	}
	_ = app.Events.Publish(ctx, contracts.Event{
		EventID:       platform.NewUUID(),
		Name:          "GPUReservationDriftDetected",
		Source:        serviceName,
		OccurredAt:    time.Now().UTC(),
		TraceID:       platform.NewUUID(),
		SchemaVersion: 1,
		Data: map[string]any{
			"project_id":   drift.ProjectID,
			"reserved_gpu": drift.ReservedGPU,
			"observed_gpu": drift.ObservedGPU,
			"drift_gpu":    drift.DriftGPU,
		},
	})
}

// registerGPUReservationDriftDetector wires drift detection as a lease-gated
// maintenance task. It runs only once StartMaintenance is called.
func registerGPUReservationDriftDetector(app *platform.App) {
	app.RegisterMaintenanceTaskForService(serviceName, "gpu-reservation-drift", func(ctx context.Context) error {
		_, err := detectGPUReservationDrift(ctx, app, newAdmissionReaderForApp(app), time.Now().UTC())
		return err
	})
}
