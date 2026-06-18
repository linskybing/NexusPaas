package workload

import (
	"net/http"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

const (
	jobLogsResource     = serviceName + ":job_logs"
	jobGPUUsageResource = serviceName + ":job_gpu_usage"
	jobCommandsResource = jobsResource + ":commands"
)

func registerJobAccessHandlers(app *platform.App) {
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/jobs", listJobs)
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/jobs/{id}", getJob)
	app.RegisterCustomHandler(http.MethodPost, "/api/v1/jobs/{id}/cancel", cancelJob)
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/jobs/{id}/logs", listJobLogs)
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/jobs/{id}/gpu-summary", listJobGPUUsage)
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/jobs/{id}/gpu-timeline", listJobGPUUsage)
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/jobs/{id}/gpu-breakdown", listJobGPUUsage)
}

func listJobs(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	projects, all, status, data, ok := authorizedWorkloadProjects(app, r)
	if !ok {
		return status, data, nil
	}
	return http.StatusOK, filterRecordsForAuthorizedProjects(jobRepository(app).ListJobs(r.Context()), projects, all), nil
}

func getJob(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	record, status, data, ok := authorizedJobRecord(app, r)
	if !ok {
		return status, data, nil
	}
	return http.StatusOK, record, nil
}

func cancelJob(app *platform.App, r *http.Request, route platform.RouteSpec) (int, any, *platform.Degraded) {
	if _, status, data, ok := authorizedJobRecord(app, r); !ok {
		return status, data, nil
	}
	payload, err := platform.DecodeMapWithError(r)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(msgInvalidBody), nil
	}
	payload["job_id"] = pathValue(r, "id")
	payload["status"] = "accepted"
	payload["operation"] = shared.FirstNonEmpty(route.OperationID, "workload_job_cancel")
	payload["idempotency_key"] = r.Header.Get("Idempotency-Key")
	payload["requested_at"] = time.Now().UTC().Format(time.RFC3339)
	record, err := app.Store.Create(r.Context(), jobCommandsResource, payload)
	if err != nil {
		return createStatus(err), shared.ErrorData("job command could not be created"), nil
	}
	publish(app, r, "JobCancelRequested", "cancel", record.Data)
	return http.StatusAccepted, record, nil
}

func listJobLogs(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	return listJobRelatedRecords(app, r, jobLogsResource)
}

func listJobGPUUsage(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	return listJobRelatedRecords(app, r, jobGPUUsageResource)
}

func listJobRelatedRecords(app *platform.App, r *http.Request, resource string) (int, any, *platform.Degraded) {
	job, status, data, ok := authorizedJobRecord(app, r)
	if !ok {
		return status, data, nil
	}
	jobID := shared.FirstNonEmpty(shared.TextValue(job.Data, "job_id", "jobId"), job.ID, pathValue(r, "id"))
	records := make([]contracts.Record[map[string]any], 0)
	for _, record := range app.Store.List(r.Context(), resource) {
		if record.ID == jobID ||
			shared.TextValue(record.Data, "job_id", "jobId") == jobID ||
			shared.TextValue(record.Data, "job_record_id", "jobRecordID") == job.ID {
			records = append(records, record)
		}
	}
	return http.StatusOK, records, nil
}

func authorizedJobRecord(app *platform.App, r *http.Request) (contracts.Record[map[string]any], int, any, bool) {
	jobs := jobRepository(app)
	if jobs == nil {
		return contracts.Record[map[string]any]{}, http.StatusInternalServerError, shared.ErrorData("job repository unavailable"), false
	}
	record, found := jobs.FindJob(r.Context(), pathValue(r, "id"))
	if !found {
		return contracts.Record[map[string]any]{}, http.StatusNotFound, shared.ErrorData("job not found"), false
	}
	if status, data, ok := requireProjectAccess(app, r, jobProjectID(record)); !ok {
		return contracts.Record[map[string]any]{}, status, data, false
	}
	return record, 0, nil, true
}

func jobProjectID(record contracts.Record[map[string]any]) string {
	return shared.TextValue(record.Data, "project_id", "projectId")
}
