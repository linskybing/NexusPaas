package workload

import (
	"net/http"
	"strings"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

const jobStatusEvicted = "evicted"

// workloadEvictJob transitions an active workload job to the terminal "evicted"
// status. It is the workload-owned status-write counterpart to the scheduler-quota
// plan-window reaper: the reaper performs the cluster-side cleanup, then calls this
// service-key-protected internal contract so that workload — the owner of job state —
// records the transition. This mirrors workloadPreemptJob and keeps cross-service job
// writes out of scheduler-quota (problem.md issue #2).
//
// Unlike preemption (which only acts on preemptible/running jobs), plan-window
// eviction applies to any non-terminal job, matching the reference reaper's
// UpdateStatusIfActive semantics. The transition is idempotent and never overwrites
// an existing terminal status.
func workloadEvictJob(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	if status, data, ok := requireWorkloadServiceAuth(app, r); !ok {
		return status, data, nil
	}
	payload, err := platform.DecodeMapWithError(r)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(msgInvalidBody), nil
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		return http.StatusBadRequest, shared.ErrorData("job id is required"), nil
	}
	record, found := findWorkloadJob(app, r, id)
	if !found {
		return http.StatusNotFound, shared.ErrorData("job not found"), nil
	}
	currentStatus := currentJobStatus(record.Data)
	if currentStatus == jobStatusEvicted {
		return http.StatusOK, record, nil
	}
	if terminalJobStatuses[currentStatus] {
		return http.StatusConflict, shared.ErrorData("job is not active for eviction"), nil
	}
	now := time.Now().UTC()
	updated, ok := jobRepository(app).MarkEvicted(r.Context(), record.ID, jobEvictionUpdate{
		Reason:      shared.TextValue(payload, "reason"),
		EvictedAt:   now,
		CompletedAt: now,
	})
	if !ok {
		return http.StatusInternalServerError, shared.ErrorData("job eviction status update failed"), nil
	}
	releaseSubmittedJobReservation(r.Context(), app, r.Header, updated.Data)
	return http.StatusOK, updated, nil
}
