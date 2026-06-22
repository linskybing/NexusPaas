package workload

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/platform/cluster"
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
	record, err := jobRepository(app).CreateJobCommandWithEvent(r.Context(), app, payload, func(record contracts.Record[map[string]any]) contracts.Event {
		return buildEvent(r, "JobCancelRequested", "cancel", record.Data)
	})
	if err != nil {
		return createStatus(err), shared.ErrorData("job command could not be created"), nil
	}
	return http.StatusAccepted, record, nil
}

func listJobLogs(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	job, status, data, ok := authorizedJobRecord(app, r)
	if !ok {
		return status, data, nil
	}
	records := relatedJobRecords(app, r, job, jobLogsResource)
	records = append(records, kubernetesJobLogRecords(app, r, job)...)
	return http.StatusOK, records, nil
}

func listJobGPUUsage(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	return listJobRelatedRecords(app, r, jobGPUUsageResource)
}

func listJobRelatedRecords(app *platform.App, r *http.Request, resource string) (int, any, *platform.Degraded) {
	job, status, data, ok := authorizedJobRecord(app, r)
	if !ok {
		return status, data, nil
	}
	return http.StatusOK, relatedJobRecords(app, r, job, resource), nil
}

func relatedJobRecords(app *platform.App, r *http.Request, job contracts.Record[map[string]any], resource string) []contracts.Record[map[string]any] {
	jobID := shared.FirstNonEmpty(shared.TextValue(job.Data, "job_id", "jobId"), job.ID, pathValue(r, "id"))
	records := make([]contracts.Record[map[string]any], 0)
	for _, record := range app.Store.List(r.Context(), resource) {
		if record.ID == jobID ||
			shared.TextValue(record.Data, "job_id", "jobId") == jobID ||
			shared.TextValue(record.Data, "job_record_id", "jobRecordID") == job.ID {
			records = append(records, record)
		}
	}
	return records
}

func kubernetesJobLogRecords(app *platform.App, r *http.Request, job contracts.Record[map[string]any]) []contracts.Record[map[string]any] {
	if app == nil || app.Cluster == nil {
		return nil
	}
	jobID := shared.FirstNonEmpty(shared.TextValue(job.Data, "job_id", "jobId"), job.ID, pathValue(r, "id"))
	namespace := shared.TextValue(job.Data, "namespace", "Namespace", "pod_namespace", "podNamespace")
	projectID := jobProjectID(job)
	expectedNamespace := jobNamespace(app.Config.K8sNamespacePrefix, projectID, nil)
	if namespace == "" {
		namespace = expectedNamespace
	}
	if expectedNamespace == "" || namespace != expectedNamespace {
		return nil
	}
	if jobID == "" || namespace == "" {
		return nil
	}
	lines, err := app.Cluster.ListJobPodLogs(r.Context(), namespace, jobID, cluster.PodLogOptions{
		TailLines:     cluster.DefaultPodLogTailLines,
		LimitBytes:    cluster.DefaultPodLogLimitBytes,
		MaxPods:       cluster.DefaultPodLogMaxPods,
		MaxContainers: cluster.DefaultPodLogMaxContainers,
		MaxLines:      cluster.DefaultPodLogMaxLines,
		PodNames:      createdPodNames(job.Data),
	})
	if err != nil {
		return nil
	}
	records := make([]contracts.Record[map[string]any], 0, len(lines))
	for _, line := range lines {
		records = append(records, kubernetesJobLogRecord(job, line))
	}
	return records
}

func kubernetesJobLogRecord(job contracts.Record[map[string]any], line cluster.PodLogLine) contracts.Record[map[string]any] {
	id := strings.Join([]string{"k8s", line.Namespace, line.Pod, line.Container, strconv.Itoa(line.Line)}, ":")
	return contracts.Record[map[string]any]{
		ID: id,
		Data: map[string]any{
			"job_id":        shared.FirstNonEmpty(shared.TextValue(job.Data, "job_id", "jobId"), job.ID),
			"job_record_id": job.ID,
			"project_id":    shared.TextValue(job.Data, "project_id", "projectId"),
			"namespace":     line.Namespace,
			"pod":           line.Pod,
			"container":     line.Container,
			"line":          line.Message,
			"line_number":   line.Line,
			"message":       line.Message,
			"timestamp":     line.Timestamp,
			"source":        "kubernetes_pod_logs",
		},
	}
}

func createdPodNames(data map[string]any) []string {
	rawResources, _ := firstPresent(data, "created_resources", "createdResources", "CreatedResources")
	items, ok := rawResources.([]map[string]any)
	if !ok {
		raw, ok := rawResources.([]any)
		if !ok {
			return nil
		}
		items = make([]map[string]any, 0, len(raw))
		for _, item := range raw {
			if mapped, ok := item.(map[string]any); ok {
				items = append(items, mapped)
			}
		}
	}
	names := make([]string, 0, len(items))
	for _, item := range items {
		if strings.EqualFold(shared.TextValue(item, "kind", "Kind"), "Pod") {
			names = append(names, shared.TextValue(item, "name", "Name"))
		}
	}
	return names
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
