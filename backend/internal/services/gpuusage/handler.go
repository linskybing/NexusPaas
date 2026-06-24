package gpuusage

import (
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

const (
	serviceName                   = "usage-observability-service"
	authorizationRolesResource    = "authorization-policy-service:roles"
	gpuAuthorizationRolesResource = serviceName + ":gpu_authorization_roles"
	gpuIdentityRolesResource      = serviceName + ":gpu_identity_roles"
	gpuIdentityUsersResource      = serviceName + ":gpu_identity_users"
	gpuJobsResource               = serviceName + ":gpu_jobs"
	gpuProjectsResource           = serviceName + ":gpu_projects"
	identityRolesResource         = "identity-service:roles"
	identityUsersResource         = "identity-service:users"
	orgProjectsResource           = "org-project-service:projects"
	snapshotsResource             = serviceName + ":job_gpu_usage_snapshots"
	summariesResource             = serviceName + ":job_gpu_usage_summaries"
	workloadJobsResource          = "workload-service:jobs"
	dateLayout                    = "2006-01-02"
	msgAdminAccessRequired        = "admin access required"
)

type UserResourceUsage struct {
	UserID        string
	Username      string
	ProjectID     string
	ProjectName   string
	JobID         string
	CPUHours      float64
	GPUHours      float64
	MemoryGBHours float64
	PeriodStart   *time.Time
	PeriodEnd     *time.Time
}

type usageRow struct {
	UserResourceUsage
	computedAt time.Time
}

type UserMPSJob struct {
	JobID           string    `json:"job_id"`
	ProjectName     string    `json:"project_name"`
	QueueName       string    `json:"queue_name"`
	PodName         string    `json:"pod_name"`
	Node            string    `json:"node"`
	GPUUUID         string    `json:"gpu_uuid"`
	GPUIndex        int       `json:"gpu_index"`
	MPSVirtualUnits int       `json:"mps_virtual_units"`
	GPUMemoryBytes  int64     `json:"gpu_memory_bytes"`
	Timestamp       time.Time `json:"timestamp"`
}

type MPSGPUSlot struct {
	Node                 string
	PhysicalGPUIndex     int
	GPUUUID              string
	JobID                string
	PodName              string
	PodNamespace         string
	MPSVirtualUnits      int
	GPUMemoryBytes       int64
	SMUtilization        float64
	SMUtilizationSource  string
	SMAttribution        string
	MemUtilization       float64
	MemUtilizationSource string
	MemoryUsedBytes      int64
	MemoryUsedSource     string
	EncUtilization       float64
	DecUtilization       float64
	PowerUsageWatts      float64
	Timestamp            time.Time
	UserID               string
	Username             string
	ProjectName          string
}

type AdminGPUUserSummary struct {
	UserID               string    `json:"user_id"`
	Username             string    `json:"username"`
	ActiveJobs           int       `json:"active_jobs"`
	TotalMPSVirtualUnits int       `json:"total_mps_virtual_units"`
	TotalGPUUnits        float64   `json:"total_gpu_units"`
	TotalGPUMemoryBytes  int64     `json:"total_gpu_memory_bytes"`
	LastSeenAt           time.Time `json:"last_seen_at"`
}

type AdminGPUUserHistory struct {
	UserID        string     `json:"user_id"`
	Username      string     `json:"username"`
	TotalGPUHours float64    `json:"total_gpu_hours"`
	TotalJobs     int        `json:"total_jobs"`
	LastJobAt     *time.Time `json:"last_job_at"`
}

type AdminGPUUserJob struct {
	JobID         string     `json:"job_id"`
	QueueName     string     `json:"queue_name"`
	ProjectName   string     `json:"project_name"`
	TotalGPUHours float64    `json:"total_gpu_hours"`
	PeriodStart   *time.Time `json:"period_start"`
	PeriodEnd     *time.Time `json:"period_end"`
}

type snapshotRow struct {
	record     contracts.Record[map[string]any]
	data       map[string]any
	metrics    map[string]any
	job        map[string]any
	user       map[string]any
	project    map[string]any
	timestamp  time.Time
	jobID      string
	userID     string
	projectID  string
	gpuIndex   int
	gpuUUID    string
	podName    string
	namespace  string
	mpsUnits   int
	mpsGPUIdx  int
	memoryByte int64
}

func Register(app *platform.App) {
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/me/usage", getMyUsage)
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/admin/usage", listAdminUsage)
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/me/gpu/jobs", getMyGPUJobs)
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/admin/gpu/users", listAdminGPUUsers)
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/admin/gpu/users/history", listAdminGPUUsersHistory)
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/admin/gpu/users/{userId}/jobs", getAdminGPUUserJobs)
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/cluster/mps-mapping", listClusterMPSMapping)
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/admin/mps-mapping", listAdminMPSMapping)
	registerGPUUsageCollector(app)
}

func getMyUsage(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	userID := currentUserID(r)
	if userID == "" {
		return http.StatusUnauthorized, map[string]any{"message": "unauthorized"}, nil
	}

	since := startOfMonth(time.Now())
	if raw := strings.TrimSpace(r.URL.Query().Get("since")); raw != "" {
		if parsed, err := time.Parse(dateLayout, raw); err == nil {
			since = parsed
		}
	}

	rows := usageRows(app, r, since)
	result := make([]UserResourceUsage, 0, len(rows))
	for _, row := range rows {
		if row.UserID == userID {
			result = append(result, row.UserResourceUsage)
		}
	}
	return http.StatusOK, result, nil
}

func listAdminUsage(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	userID := currentUserID(r)
	if userID == "" {
		return http.StatusUnauthorized, map[string]any{"message": "unauthorized"}, nil
	}
	if !hasAdminPanel(app, r, userID) {
		return http.StatusForbidden, map[string]any{"message": msgAdminAccessRequired}, nil
	}

	since := time.Now().AddDate(0, -1, 0)
	if raw := strings.TrimSpace(r.URL.Query().Get("since")); raw != "" {
		if parsed, err := time.Parse(dateLayout, raw); err == nil {
			since = parsed
		}
	}

	rows := usageRows(app, r, since)
	result := make([]UserResourceUsage, 0, len(rows))
	for _, row := range rows {
		result = append(result, row.UserResourceUsage)
	}
	return http.StatusOK, result, nil
}

func getMyGPUJobs(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	userID := currentUserID(r)
	if userID == "" {
		return http.StatusUnauthorized, map[string]any{"message": "unauthorized"}, nil
	}

	latest := map[string]snapshotRow{}
	for _, row := range snapshotRows(app, r, activeSnapshotCutoff(app.Config)) {
		if row.userID != userID {
			continue
		}
		key := row.jobID + "\x00" + row.podName + "\x00" + row.gpuUUID
		if existing, ok := latest[key]; !ok || row.timestamp.After(existing.timestamp) {
			latest[key] = row
		}
	}
	out := make([]UserMPSJob, 0, len(latest))
	for _, row := range latest {
		out = append(out, UserMPSJob{
			JobID:           row.jobID,
			ProjectName:     textValue(row.project, "project_name", "projectName", "name", "Name"),
			QueueName:       textValue(row.job, "queue_name", "queueName", "QueueName"),
			PodName:         row.podName,
			Node:            textValue(row.data, "node", "Node"),
			GPUUUID:         row.gpuUUID,
			GPUIndex:        row.gpuIndex,
			MPSVirtualUnits: row.mpsUnits,
			GPUMemoryBytes:  row.memoryByte,
			Timestamp:       row.timestamp,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].JobID == out[j].JobID {
			if out[i].PodName == out[j].PodName {
				return out[i].GPUUUID < out[j].GPUUUID
			}
			return out[i].PodName < out[j].PodName
		}
		return out[i].JobID < out[j].JobID
	})
	return http.StatusOK, out, nil
}

func listAdminGPUUsers(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	userID := currentUserID(r)
	if userID == "" {
		return http.StatusUnauthorized, map[string]any{"message": "unauthorized"}, nil
	}
	if !hasAdminPanel(app, r, userID) {
		return http.StatusForbidden, map[string]any{"message": msgAdminAccessRequired}, nil
	}

	latest := latestActiveSnapshots(app, r)
	summaries := map[string]AdminGPUUserSummary{}
	for _, row := range latest {
		if !strings.EqualFold(textValue(row.job, "status", "Status"), "running") {
			continue
		}
		summary := summaries[row.userID]
		summary.UserID = row.userID
		summary.Username = textValue(row.user, "username", "Username")
		summary.TotalMPSVirtualUnits += row.mpsUnits
		summary.TotalGPUUnits += float64(row.mpsUnits)
		summary.TotalGPUMemoryBytes += row.memoryByte
		if row.timestamp.After(summary.LastSeenAt) {
			summary.LastSeenAt = row.timestamp
		}
		summaries[row.userID] = summary
	}
	activeJobs := map[string]map[string]struct{}{}
	for _, row := range latest {
		if !strings.EqualFold(textValue(row.job, "status", "Status"), "running") {
			continue
		}
		if activeJobs[row.userID] == nil {
			activeJobs[row.userID] = map[string]struct{}{}
		}
		activeJobs[row.userID][row.jobID] = struct{}{}
	}
	out := make([]AdminGPUUserSummary, 0, len(summaries))
	for userID, summary := range summaries {
		summary.ActiveJobs = len(activeJobs[userID])
		out = append(out, summary)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].TotalMPSVirtualUnits == out[j].TotalMPSVirtualUnits {
			return out[i].Username < out[j].Username
		}
		return out[i].TotalMPSVirtualUnits > out[j].TotalMPSVirtualUnits
	})
	return http.StatusOK, out, nil
}

func listAdminGPUUsersHistory(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	userID := currentUserID(r)
	if userID == "" {
		return http.StatusUnauthorized, map[string]any{"message": "unauthorized"}, nil
	}
	if !hasAdminPanel(app, r, userID) {
		return http.StatusForbidden, map[string]any{"message": msgAdminAccessRequired}, nil
	}

	since := time.Now().AddDate(0, -1, 0)
	if raw := strings.TrimSpace(r.URL.Query().Get("since")); raw != "" {
		if parsed, err := time.Parse(dateLayout, raw); err == nil {
			since = parsed
		}
	}

	history := map[string]AdminGPUUserHistory{}
	seenJobs := map[string]map[string]struct{}{}
	for _, row := range usageRows(app, r, since) {
		entry := history[row.UserID]
		entry.UserID = row.UserID
		entry.Username = row.Username
		entry.TotalGPUHours += row.GPUHours
		if seenJobs[row.UserID] == nil {
			seenJobs[row.UserID] = map[string]struct{}{}
		}
		seenJobs[row.UserID][row.JobID] = struct{}{}
		last := row.PeriodEnd
		if last != nil && (entry.LastJobAt == nil || last.After(*entry.LastJobAt)) {
			value := *last
			entry.LastJobAt = &value
		}
		history[row.UserID] = entry
	}
	out := make([]AdminGPUUserHistory, 0, len(history))
	for userID, entry := range history {
		entry.TotalJobs = len(seenJobs[userID])
		out = append(out, entry)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].TotalGPUHours > out[j].TotalGPUHours })
	return http.StatusOK, out, nil
}

func getAdminGPUUserJobs(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	actorID := currentUserID(r)
	if actorID == "" {
		return http.StatusUnauthorized, map[string]any{"message": "unauthorized"}, nil
	}
	if !hasAdminPanel(app, r, actorID) {
		return http.StatusForbidden, map[string]any{"message": msgAdminAccessRequired}, nil
	}
	targetUserID := strings.TrimSpace(r.PathValue("userId"))
	if targetUserID == "" {
		return http.StatusBadRequest, map[string]any{"message": "user ID required"}, nil
	}

	since := time.Now().AddDate(0, -1, 0)
	if raw := strings.TrimSpace(r.URL.Query().Get("since")); raw != "" {
		if parsed, err := time.Parse(dateLayout, raw); err == nil {
			since = parsed
		}
	}

	out := []AdminGPUUserJob{}
	summarizedJobs := map[string]struct{}{}
	for _, row := range usageRows(app, r, since) {
		if row.UserID != targetUserID {
			continue
		}
		summarizedJobs[row.JobID] = struct{}{}
		job := findRecord(gpuRecords(app, r, gpuJobsResource, workloadJobsResource), row.JobID)
		out = append(out, AdminGPUUserJob{
			JobID:         row.JobID,
			QueueName:     textValue(job, "queue_name", "queueName", "QueueName"),
			ProjectName:   row.ProjectName,
			TotalGPUHours: row.GPUHours,
			PeriodStart:   row.PeriodStart,
			PeriodEnd:     row.PeriodEnd,
		})
	}
	out = append(out, runningUserJobs(app, r, targetUserID, summarizedJobs)...)
	sort.Slice(out, func(i, j int) bool {
		return comparableJobTime(out[i]).After(comparableJobTime(out[j]))
	})
	return http.StatusOK, out, nil
}

func listClusterMPSMapping(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	if currentUserID(r) == "" {
		return http.StatusUnauthorized, map[string]any{"message": "unauthorized"}, nil
	}
	return http.StatusOK, mpsMapping(app, r), nil
}

func listAdminMPSMapping(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	userID := currentUserID(r)
	if userID == "" {
		return http.StatusUnauthorized, map[string]any{"message": "unauthorized"}, nil
	}
	if !hasAdminPanel(app, r, userID) {
		return http.StatusForbidden, map[string]any{"message": msgAdminAccessRequired}, nil
	}
	return http.StatusOK, mpsMapping(app, r), nil
}
