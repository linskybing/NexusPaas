package dashboard

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/auditcompliance"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

const (
	serviceName                     = "usage-observability-service"
	dashboardProjectionConsumer     = serviceName + ":dashboard_projection"
	auditLogsResource               = "audit-compliance-service:audit_logs"
	clusterReadModelsResource       = serviceName + ":cluster_read_models"
	dashboardFormsResource          = serviceName + ":dashboard_forms"
	dashboardLiveQuotasResource     = serviceName + ":dashboard_live_quotas"
	dashboardProjectMembersResource = serviceName + ":dashboard_project_members"
	dashboardProjectsResource       = serviceName + ":dashboard_projects"
	dashboardQueuesResource         = serviceName + ":dashboard_queues"
	dashboardUsersResource          = serviceName + ":dashboard_users"
	identityUsersResource           = "identity-service:users"
	orgProjectMembersResource       = "org-project-service:project_members"
	orgProjectsResource             = "org-project-service:projects"
	requestFormsResource            = "request-notification-service:forms"
	schedulerLiveQuotasResource     = "scheduler-quota-service:live_quotas"
	schedulerQueuesResource         = "scheduler-quota-service:queues"
)

var errDashboardProjectionDriftUnavailable = errors.New("dashboard projection drift unavailable")

type dashboardProjectionDriftReport struct {
	Missing []dashboardProjectionDriftFinding
	Orphan  []dashboardProjectionDriftFinding
	Stale   []dashboardProjectionDriftFinding
}

type dashboardProjectionDriftFinding struct {
	SourceResource string
	LocalResource  string
	ID             string
}

type dashboardProjectionDriftPair struct {
	sourceResource string
	localResource  string
	idFn           func(map[string]any) string
}

var dashboardProjectionDriftPairs = []dashboardProjectionDriftPair{
	{sourceResource: identityUsersResource, localResource: dashboardUsersResource, idFn: func(row map[string]any) string {
		return readModelID(dashboardUsersResource, row)
	}},
	{sourceResource: orgProjectsResource, localResource: dashboardProjectsResource, idFn: func(row map[string]any) string {
		return readModelID(dashboardProjectsResource, row)
	}},
	{sourceResource: orgProjectMembersResource, localResource: dashboardProjectMembersResource, idFn: func(row map[string]any) string {
		return readModelID(dashboardProjectMembersResource, row)
	}},
	{sourceResource: requestFormsResource, localResource: dashboardFormsResource, idFn: func(row map[string]any) string {
		return readModelID(dashboardFormsResource, row)
	}},
	{sourceResource: schedulerLiveQuotasResource, localResource: dashboardLiveQuotasResource, idFn: func(row map[string]any) string {
		return readModelID(dashboardLiveQuotasResource, row)
	}},
	{sourceResource: schedulerQueuesResource, localResource: dashboardQueuesResource, idFn: func(row map[string]any) string {
		return readModelID(dashboardQueuesResource, row)
	}},
}

func Register(app *platform.App) {
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/dashboard/overview", getOverview)
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/admin/dashboard-summary", getAdminSummary)
	registerDashboardProjectionReconciler(app)
}

func getOverview(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	syncDashboardReadModels(app, r)
	userID, username, code, data, ok := requireNamedUser(app, r)
	if !ok {
		return code, data, nil
	}
	projects := projectsForUser(app, r, userID)
	projectIDs := map[string]bool{}
	for _, project := range projects {
		projectIDs[shared.TextValue(project, "id")] = true
	}
	activities := formsForUser(app, r, userID, 10)
	return http.StatusOK, map[string]any{
		"projects":              projects,
		"activities":            activities,
		"clusterSummary":        publicClusterSummary(clusterSummary(app, r)),
		"projectQuotaLiveById":  quotaMap(app, r, projectIDs, username),
		"preemptibleQueueCount": preemptibleQueueCount(app, r),
	}, nil
}

func getAdminSummary(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	syncDashboardReadModels(app, r)
	if _, _, code, data, ok := requireNamedUser(app, r); !ok {
		return code, data, nil
	}
	if !isAdmin(r) {
		return http.StatusForbidden, shared.ErrorData("admin access required"), nil
	}
	return http.StatusOK, map[string]any{
		"totalUsers":           int64(len(userRecords(app, r))),
		"clusterSummary":       clusterSummary(app, r),
		"pendingRequestsCount": pendingForms(app, r),
		"recentLogs":           recentAuditLogs(app, r, 5),
	}, nil
}

func requireNamedUser(app *platform.App, r *http.Request) (string, string, int, any, bool) {
	userID := strings.TrimSpace(r.Header.Get("X-User-ID"))
	username := strings.TrimSpace(r.Header.Get("X-Username"))
	if userID == "" || username == "" {
		return "", "", http.StatusUnauthorized, shared.ErrorData("Unauthorized"), false
	}
	users := userRecords(app, r)
	for _, user := range users {
		if shared.TextValue(user, "id", "user_id", "userId") != userID {
			continue
		}
		status := strings.ToLower(shared.TextValue(user, "status"))
		if status == "disabled" || status == "delete" || status == "deleted" {
			return "", "", http.StatusUnauthorized, shared.ErrorData("account disabled"), false
		}
		return userID, username, 0, nil, true
	}
	return "", "", http.StatusNotFound, shared.ErrorData("User not found"), false
}

func projectsForUser(app *platform.App, r *http.Request, userID string) []map[string]any {
	projectIDs := map[string]bool{}
	for _, member := range projectMemberRecords(app, r) {
		if shared.TextValue(member, "user_id", "userId") == userID {
			projectIDs[shared.TextValue(member, "project_id", "projectId")] = true
		}
	}
	out := []map[string]any{}
	for _, project := range projectRecords(app, r) {
		if projectIDs[shared.TextValue(project, "id")] || shared.TextValue(project, "personal_user_id", "personalUserID") == userID {
			out = append(out, project)
		}
	}
	return out
}

func formsForUser(app *platform.App, r *http.Request, userID string, limit int) []map[string]any {
	out := []map[string]any{}
	for _, form := range formRecords(app, r) {
		if shared.TextValue(form, "user_id", "userId") == userID {
			out = append(out, form)
		}
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func clusterSummary(app *platform.App, r *http.Request) map[string]any {
	records := listRecords(app, r, clusterReadModelsResource)
	if len(records) == 0 {
		return map[string]any{"nodes": []any{}, "podGpuUsages": []any{}}
	}
	return records[0]
}

func publicClusterSummary(summary map[string]any) map[string]any {
	out := shared.CloneMap(summary)
	rawNodes, _ := out["nodes"].([]any)
	publicNodes := make([]any, 0, len(rawNodes))
	gpuIndex := 1
	for _, raw := range rawNodes {
		node, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		gpu := shared.NumberValue(node, "gpuAllocatable", "gpu_allocatable", "GPUAllocatable")
		if gpu <= 0 {
			continue
		}
		safe := map[string]any{}
		for key, value := range node {
			switch key {
			case "cpuAllocatable", "cpu_allocatable", "memoryAllocatable", "memory_allocatable", "gpuDevices", "gpu_devices", "gpuDeviceSMStatuses", "gpu_device_sm_statuses":
				continue
			default:
				safe[key] = value
			}
		}
		safe["name"] = "GPU node " + strconv.Itoa(gpuIndex)
		publicNodes = append(publicNodes, safe)
		gpuIndex++
	}
	out["nodes"] = publicNodes
	out["podGpuUsages"] = nil
	return out
}

func quotaMap(app *platform.App, r *http.Request, projectIDs map[string]bool, _ string) map[string]any {
	out := map[string]any{}
	for _, quota := range liveQuotaRecords(app, r) {
		projectID := shared.TextValue(quota, "project_id", "projectId")
		if projectIDs[projectID] {
			out[projectID] = quota
		}
	}
	return out
}

func preemptibleQueueCount(app *platform.App, r *http.Request) int {
	count := 0
	for _, queue := range queueRecords(app, r) {
		if shared.BoolValue(queue, "is_preemptible", "isPreemptible") {
			count++
		}
	}
	return count
}

func pendingForms(app *platform.App, r *http.Request) int {
	count := 0
	for _, form := range formRecords(app, r) {
		if shared.TextValue(form, "status") == "Pending" {
			count++
		}
	}
	return count
}

func recentAuditLogs(app *platform.App, r *http.Request, limit int) []map[string]any {
	// Use the audit service's shared reader (outbox + store) instead of a raw
	// store-key read so emitted audit events are reflected (finding 32).
	return auditcompliance.RecentAuditLogMaps(app, r, limit)
}

func userRecords(app *platform.App, r *http.Request) []map[string]any {
	return dashboardRecords(app, r, dashboardUsersResource, identityUsersResource)
}

func projectRecords(app *platform.App, r *http.Request) []map[string]any {
	return dashboardRecords(app, r, dashboardProjectsResource, orgProjectsResource)
}

func projectMemberRecords(app *platform.App, r *http.Request) []map[string]any {
	return dashboardRecords(app, r, dashboardProjectMembersResource, orgProjectMembersResource)
}

func formRecords(app *platform.App, r *http.Request) []map[string]any {
	return dashboardRecords(app, r, dashboardFormsResource, requestFormsResource)
}

func liveQuotaRecords(app *platform.App, r *http.Request) []map[string]any {
	return dashboardRecords(app, r, dashboardLiveQuotasResource, schedulerLiveQuotasResource)
}

func queueRecords(app *platform.App, r *http.Request) []map[string]any {
	return dashboardRecords(app, r, dashboardQueuesResource, schedulerQueuesResource)
}

func dashboardRecords(app *platform.App, r *http.Request, localResource, sourceResource string) []map[string]any {
	local := listRecords(app, r, localResource)
	if len(local) > 0 || !sourceCoHosted(app, sourceResource) {
		return local
	}
	return listRecords(app, r, sourceResource)
}

func listRecords(app *platform.App, r *http.Request, resource string) []map[string]any {
	if app == nil || app.Store == nil {
		return nil
	}
	records := app.Store.List(r.Context(), resource)
	out := make([]map[string]any, 0, len(records))
	for _, record := range records {
		out = append(out, shared.CloneMap(record.Data))
	}
	return out
}

func projectionDrift(app *platform.App, r *http.Request) (dashboardProjectionDriftReport, error) {
	var report dashboardProjectionDriftReport
	if app == nil || app.Store == nil {
		return report, errDashboardProjectionDriftUnavailable
	}
	for _, pair := range dashboardProjectionDriftPairs {
		sourceRows := dashboardProjectionDriftIndex(listRecords(app, r, pair.sourceResource), pair.idFn)
		localRows := dashboardProjectionDriftIndex(listRecords(app, r, pair.localResource), pair.idFn)
		report.addDashboardProjectionPairDrift(pair, sourceRows, localRows)
	}
	report.sort()
	return report, nil
}

func (r *dashboardProjectionDriftReport) addDashboardProjectionPairDrift(pair dashboardProjectionDriftPair, sourceRows, localRows map[string]map[string]any) {
	r.addDashboardProjectionMissingAndStale(pair, sourceRows, localRows)
	r.addDashboardProjectionOrphans(pair, sourceRows, localRows)
}

func (r *dashboardProjectionDriftReport) addDashboardProjectionMissingAndStale(pair dashboardProjectionDriftPair, sourceRows, localRows map[string]map[string]any) {
	for id, sourceRow := range sourceRows {
		localRow, ok := localRows[id]
		finding := dashboardProjectionDriftFinding{
			SourceResource: pair.sourceResource,
			LocalResource:  pair.localResource,
			ID:             id,
		}
		if !ok {
			r.Missing = append(r.Missing, finding)
			continue
		}
		if !reflect.DeepEqual(sourceRow, localRow) {
			r.Stale = append(r.Stale, finding)
		}
	}
}

func (r *dashboardProjectionDriftReport) addDashboardProjectionOrphans(pair dashboardProjectionDriftPair, sourceRows, localRows map[string]map[string]any) {
	for id := range localRows {
		if _, ok := sourceRows[id]; ok {
			continue
		}
		r.Orphan = append(r.Orphan, dashboardProjectionDriftFinding{
			SourceResource: pair.sourceResource,
			LocalResource:  pair.localResource,
			ID:             id,
		})
	}
}

func dashboardProjectionDriftIndex(rows []map[string]any, idFn func(map[string]any) string) map[string]map[string]any {
	out := map[string]map[string]any{}
	for _, row := range rows {
		id, normalized := dashboardProjectionDriftNormalize(row, idFn)
		if id == "" {
			continue
		}
		out[id] = normalized
	}
	return out
}

func dashboardProjectionDriftNormalize(row map[string]any, idFn func(map[string]any) string) (string, map[string]any) {
	normalized := shared.CloneMap(row)
	id := idFn(normalized)
	if id == "" {
		return "", nil
	}
	normalized["id"] = id
	return id, normalized
}

func (r *dashboardProjectionDriftReport) sort() {
	sortDashboardProjectionDriftFindings(r.Missing)
	sortDashboardProjectionDriftFindings(r.Orphan)
	sortDashboardProjectionDriftFindings(r.Stale)
}

func sortDashboardProjectionDriftFindings(findings []dashboardProjectionDriftFinding) {
	sort.Slice(findings, func(i, j int) bool {
		if findings[i].LocalResource != findings[j].LocalResource {
			return findings[i].LocalResource < findings[j].LocalResource
		}
		if findings[i].ID != findings[j].ID {
			return findings[i].ID < findings[j].ID
		}
		return findings[i].SourceResource < findings[j].SourceResource
	})
}

func syncDashboardReadModels(app *platform.App, r *http.Request) {
	if app == nil || app.Store == nil || app.Events == nil {
		return
	}
	app.RunProjection(r.Context(), dashboardProjectionConsumer, func(event contracts.Event) error {
		return projectDashboardEvent(app, r, event)
	})
}

func projectDashboardEvent(app *platform.App, r *http.Request, event contracts.Event) error {
	resource, data, deleted, ok := dashboardProjection(event)
	if !ok {
		return nil
	}
	if deleted {
		deleteReadModel(app, r, resource, data)
		return nil
	}
	return upsertReadModel(app, r, resource, data)
}

func dashboardProjection(event contracts.Event) (string, map[string]any, bool, bool) {
	name := strings.ToLower(event.Name)
	switch name {
	case "usercreated", "userupdated", "userdisabled":
		return dashboardUsersResource, eventData(event), false, true
	case "userdeleted":
		return dashboardUsersResource, eventData(event), true, true
	case "projectcreated", "projectupdated":
		return dashboardProjectsResource, eventData(event), false, true
	case "projectdeleted":
		return dashboardProjectsResource, eventData(event), true, true
	case "project_membercreated", "project_memberupdated":
		return dashboardProjectMembersResource, eventData(event), false, true
	case "project_memberdeleted":
		return dashboardProjectMembersResource, eventData(event), true, true
	case "formcreated", "formupdated":
		return dashboardFormsResource, eventData(event), false, true
	case "formdeleted":
		return dashboardFormsResource, eventData(event), true, true
	case "live_quotacreated", "live_quotaupdated":
		return dashboardLiveQuotasResource, eventData(event), false, true
	case "live_quotadeleted":
		return dashboardLiveQuotasResource, eventData(event), true, true
	case "queuecreated", "queueupdated":
		return dashboardQueuesResource, eventData(event), false, true
	case "queuedeleted":
		return dashboardQueuesResource, eventData(event), true, true
	default:
		return "", nil, false, false
	}
}

func eventData(event contracts.Event) map[string]any {
	if next, ok := event.Data["new"].(map[string]any); ok {
		return shared.CloneMap(next)
	}
	return shared.CloneMap(event.Data)
}

func upsertReadModel(app *platform.App, r *http.Request, resource string, data map[string]any) error {
	id := readModelID(resource, data)
	if id == "" {
		return nil
	}
	data["id"] = id
	if _, ok := app.Store.Update(r.Context(), resource, id, data); ok {
		return nil
	}
	if _, err := app.Store.Create(r.Context(), resource, data); err != nil {
		if platform.IsCreateConflict(err) {
			if _, ok := app.Store.Update(r.Context(), resource, id, data); !ok {
				return fmt.Errorf("dashboard projection conflict update missed for %s/%s", resource, id)
			}
			return nil
		}
		return fmt.Errorf("dashboard projection create failed for %s/%s: %w", resource, id, err)
	}
	return nil
}

func deleteReadModel(app *platform.App, r *http.Request, resource string, data map[string]any) {
	if deleted, ok := data["deleted"].(bool); ok && !deleted {
		return
	}
	if id := readModelID(resource, data); id != "" {
		app.Store.Delete(r.Context(), resource, id)
	}
}

func readModelID(resource string, data map[string]any) string {
	id := shared.TextValue(data, "id")
	projectID := shared.TextValue(data, "project_id", "projectId")
	userID := shared.TextValue(data, "user_id", "userId")
	if resource == dashboardProjectMembersResource && id == "" && projectID != "" && userID != "" {
		return projectID + ":" + userID
	}
	return shared.FirstNonEmpty(id, userID, projectID)
}

func sourceCoHosted(app *platform.App, sourceResource string) bool {
	if app == nil {
		return false
	}
	owner, _, ok := strings.Cut(sourceResource, ":")
	return ok && app.Config.AllowsService(owner)
}

func isAdmin(r *http.Request) bool {
	role := strings.ToLower(r.Header.Get("X-User-Role"))
	return role == "admin" || role == "super-admin"
}

// registerDashboardProjectionReconciler wires the dashboard read models into
// the periodic drift→replay reconcile job (DATA-016/DATA-018).
func registerDashboardProjectionReconciler(app *platform.App) {
	app.RegisterProjectionReconciler(platform.ProjectionReconcilerSpec{
		Owner:     serviceName,
		Consumers: []string{dashboardProjectionConsumer},
		Drift: func(ctx context.Context) (int, error) {
			report, err := projectionDrift(app, shared.MaintenanceRequest(ctx))
			if err != nil {
				return 0, err
			}
			return len(report.Missing) + len(report.Orphan) + len(report.Stale), nil
		},
		Sync: func(ctx context.Context) { syncDashboardReadModels(app, shared.MaintenanceRequest(ctx)) },
	})
}
