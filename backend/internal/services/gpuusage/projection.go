package gpuusage

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"sort"
	"strings"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

const gpuProjectionConsumer = serviceName + ":gpu_usage_projection"

const gpuKeyID = "id"

var errGPUProjectionDriftUnavailable = errors.New("gpu usage projection drift unavailable")

type gpuProjectionDriftReport struct {
	Missing []gpuProjectionDriftFinding
	Orphan  []gpuProjectionDriftFinding
	Stale   []gpuProjectionDriftFinding
}

type gpuProjectionDriftFinding struct {
	SourceResource string
	LocalResource  string
	ID             string
}

type gpuProjectionDriftPair struct {
	sourceResource string
	localResource  string
	idFn           func(map[string]any) string
}

var gpuProjectionDriftPairs = []gpuProjectionDriftPair{
	{sourceResource: identityUsersResource, localResource: gpuIdentityUsersResource, idFn: func(row map[string]any) string {
		return gpuReadModelID(gpuIdentityUsersResource, row)
	}},
	{sourceResource: identityRolesResource, localResource: gpuIdentityRolesResource, idFn: func(row map[string]any) string {
		return gpuReadModelID(gpuIdentityRolesResource, row)
	}},
	{sourceResource: authorizationRolesResource, localResource: gpuAuthorizationRolesResource, idFn: func(row map[string]any) string {
		return gpuReadModelID(gpuAuthorizationRolesResource, row)
	}},
	{sourceResource: orgProjectsResource, localResource: gpuProjectsResource, idFn: func(row map[string]any) string {
		return gpuReadModelID(gpuProjectsResource, row)
	}},
	{sourceResource: workloadJobsResource, localResource: gpuJobsResource, idFn: func(row map[string]any) string {
		return gpuReadModelID(gpuJobsResource, row)
	}},
}

func gpuRecords(app *platform.App, r *http.Request, localResource, sourceResource string) []contracts.Record[map[string]any] {
	syncGPUReadModelsContext(app, r.Context())
	if app == nil || app.Store == nil {
		return nil
	}
	local := app.Store.List(r.Context(), localResource)
	if sourceResource == "" || !sourceCoHosted(app, sourceResource) {
		return local
	}
	source := app.Store.List(r.Context(), sourceResource)
	if len(local) == 0 {
		return source
	}
	return mergeGPURecords(localResource, source, local)
}

func syncGPUReadModels(app *platform.App, r *http.Request) {
	syncGPUReadModelsContext(app, r.Context())
}

func gpuRecordsContext(app *platform.App, ctx context.Context, localResource, sourceResource string) []contracts.Record[map[string]any] {
	syncGPUReadModelsContext(app, ctx)
	if app == nil || app.Store == nil {
		return nil
	}
	local := app.Store.List(ctx, localResource)
	if sourceResource == "" || !sourceCoHosted(app, sourceResource) {
		return local
	}
	source := app.Store.List(ctx, sourceResource)
	if len(local) == 0 {
		return source
	}
	return mergeGPURecords(localResource, source, local)
}

func syncGPUReadModelsContext(app *platform.App, ctx context.Context) {
	if app == nil || app.Store == nil || app.Events == nil {
		return
	}
	app.RunProjection(ctx, gpuProjectionConsumer, func(event contracts.Event) error {
		return projectGPUUsageEventContext(app, ctx, event)
	})
}

func projectGPUUsageEvent(app *platform.App, r *http.Request, event contracts.Event) error {
	return projectGPUUsageEventContext(app, r.Context(), event)
}

func projectGPUUsageEventContext(app *platform.App, ctx context.Context, event contracts.Event) error {
	resource, data, deleted, ok := gpuProjection(event)
	if !ok {
		return nil
	}
	if deleted {
		deleteGPUReadModelContext(app, ctx, resource, data)
		return nil
	}
	return upsertGPUReadModelContext(app, ctx, resource, data)
}

func gpuProjection(event contracts.Event) (string, map[string]any, bool, bool) {
	switch strings.ToLower(event.Name) {
	case "usercreated", "userupdated", "userdisabled":
		return gpuIdentityUsersResource, gpuEventData(event), false, true
	case "userdeleted":
		return gpuIdentityUsersResource, gpuEventData(event), true, true
	case "rolecreated", "roleupdated":
		return gpuIdentityRolesResource, gpuEventData(event), false, true
	case "roledeleted":
		return gpuIdentityRolesResource, gpuEventData(event), true, true
	case "projectcreated", "projectupdated":
		return gpuProjectsResource, gpuEventData(event), false, true
	case "projectdeleted":
		return gpuProjectsResource, gpuEventData(event), true, true
	case "jobsubmitted", "jobqueued", "jobrunning", "jobsucceeded", "jobfailed", "jobcancelled":
		return gpuJobProjection(event)
	case "proxypolicychanged":
		return gpuProxyPolicyProjection(event)
	default:
		return "", nil, false, false
	}
}

func gpuJobProjection(event contracts.Event) (string, map[string]any, bool, bool) {
	data := gpuEventData(event)
	if textValue(data, "status", "Status") == "" {
		data["status"] = statusForJobEvent(event.Name)
	}
	return gpuJobsResource, data, false, true
}

func gpuProxyPolicyProjection(event contracts.Event) (string, map[string]any, bool, bool) {
	data := gpuEventData(event)
	switch strings.ToLower(textValue(data, "action")) {
	case "role_create", "role_update":
		return gpuAuthorizationRolesResource, data, false, true
	case "role_delete":
		return gpuAuthorizationRolesResource, data, true, true
	default:
		return "", nil, false, false
	}
}

func statusForJobEvent(name string) string {
	switch strings.ToLower(name) {
	case "jobqueued":
		return "queued"
	case "jobrunning":
		return "running"
	case "jobsucceeded":
		return "succeeded"
	case "jobfailed":
		return "failed"
	case "jobcancelled":
		return "cancelled"
	default:
		return "submitted"
	}
}

func gpuEventData(event contracts.Event) map[string]any {
	for _, key := range []string{"new", "record", "job", "project", "user", "role"} {
		if data, ok := event.Data[key].(map[string]any); ok {
			return shared.CloneMap(data)
		}
	}
	return shared.CloneMap(event.Data)
}

func upsertGPUReadModel(app *platform.App, r *http.Request, resource string, data map[string]any) error {
	return upsertGPUReadModelContext(app, r.Context(), resource, data)
}

func upsertGPUReadModelContext(app *platform.App, ctx context.Context, resource string, data map[string]any) error {
	id := gpuReadModelID(resource, data)
	if id == "" {
		return nil
	}
	data[gpuKeyID] = id
	if _, ok := app.Store.Update(ctx, resource, id, data); ok {
		return nil
	}
	if _, err := app.Store.Create(ctx, resource, data); err != nil {
		if platform.IsCreateConflict(err) {
			if _, ok := app.Store.Update(ctx, resource, id, data); !ok {
				return fmt.Errorf("gpu usage projection conflict update missed for %s/%s", resource, id)
			}
			return nil
		}
		return fmt.Errorf("gpu usage projection create failed for %s/%s: %w", resource, id, err)
	}
	return nil
}

func deleteGPUReadModel(app *platform.App, r *http.Request, resource string, data map[string]any) {
	deleteGPUReadModelContext(app, r.Context(), resource, data)
}

func deleteGPUReadModelContext(app *platform.App, ctx context.Context, resource string, data map[string]any) {
	if deleted, ok := data["deleted"].(bool); ok && !deleted {
		return
	}
	if id := gpuReadModelID(resource, data); id != "" {
		app.Store.Delete(ctx, resource, id)
	}
}

func projectionDrift(ctx context.Context, app *platform.App) (gpuProjectionDriftReport, error) {
	var report gpuProjectionDriftReport
	if app == nil || app.Store == nil {
		return report, errGPUProjectionDriftUnavailable
	}
	for _, pair := range gpuProjectionDriftPairs {
		sourceRows := gpuProjectionDriftIndex(gpuProjectionDriftRecordMaps(ctx, app, pair.sourceResource), pair.idFn)
		localRows := gpuProjectionDriftIndex(gpuProjectionDriftRecordMaps(ctx, app, pair.localResource), pair.idFn)
		report.addGPUProjectionPairDrift(pair, sourceRows, localRows)
	}
	report.sort()
	return report, nil
}

func (r *gpuProjectionDriftReport) addGPUProjectionPairDrift(pair gpuProjectionDriftPair, sourceRows, localRows map[string]map[string]any) {
	r.addGPUProjectionMissingAndStale(pair, sourceRows, localRows)
	r.addGPUProjectionOrphans(pair, sourceRows, localRows)
}

func (r *gpuProjectionDriftReport) addGPUProjectionMissingAndStale(pair gpuProjectionDriftPair, sourceRows, localRows map[string]map[string]any) {
	for id, sourceRow := range sourceRows {
		localRow, ok := localRows[id]
		finding := gpuProjectionDriftFinding{
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

func (r *gpuProjectionDriftReport) addGPUProjectionOrphans(pair gpuProjectionDriftPair, sourceRows, localRows map[string]map[string]any) {
	for id := range localRows {
		if _, ok := sourceRows[id]; ok {
			continue
		}
		r.Orphan = append(r.Orphan, gpuProjectionDriftFinding{
			SourceResource: pair.sourceResource,
			LocalResource:  pair.localResource,
			ID:             id,
		})
	}
}

func gpuProjectionDriftRecordMaps(ctx context.Context, app *platform.App, resource string) []map[string]any {
	if app == nil || app.Store == nil {
		return nil
	}
	records := app.Store.List(ctx, resource)
	out := make([]map[string]any, 0, len(records))
	for _, record := range records {
		out = append(out, shared.CloneMap(record.Data))
	}
	return out
}

func gpuProjectionDriftIndex(rows []map[string]any, idFn func(map[string]any) string) map[string]map[string]any {
	out := map[string]map[string]any{}
	if idFn == nil {
		return out
	}
	for _, row := range rows {
		id, normalized := gpuProjectionDriftNormalize(row, idFn)
		if id == "" {
			continue
		}
		out[id] = normalized
	}
	return out
}

func gpuProjectionDriftNormalize(row map[string]any, idFn func(map[string]any) string) (string, map[string]any) {
	normalized := shared.CloneMap(row)
	id := idFn(normalized)
	if id == "" {
		return "", nil
	}
	normalized[gpuKeyID] = id
	return id, normalized
}

func (r *gpuProjectionDriftReport) sort() {
	sortGPUProjectionDriftFindings(r.Missing)
	sortGPUProjectionDriftFindings(r.Orphan)
	sortGPUProjectionDriftFindings(r.Stale)
}

func sortGPUProjectionDriftFindings(findings []gpuProjectionDriftFinding) {
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

func gpuReadModelID(resource string, data map[string]any) string {
	id := textValue(data, gpuKeyID, "ID")
	jobID := textValue(data, "job_id", "jobId", "JobID")
	name := textValue(data, "name", "Name")
	projectID := textValue(data, "project_id", "projectId", "ProjectID", "p_id", "pID", "PID")
	roleID := textValue(data, "role_id", "roleId", "RoleID")
	userID := textValue(data, "user_id", "userId", "UserID")
	switch resource {
	case gpuJobsResource:
		return shared.FirstNonBlank(id, jobID)
	case gpuProjectsResource:
		return shared.FirstNonBlank(id, projectID)
	case gpuIdentityUsersResource:
		return shared.FirstNonBlank(id, userID)
	case gpuAuthorizationRolesResource, gpuIdentityRolesResource:
		return shared.FirstNonBlank(id, roleID, name, userID)
	default:
		return shared.FirstNonBlank(id, jobID, projectID, userID, roleID, name)
	}
}

func mergeGPURecords(resource string, source, local []contracts.Record[map[string]any]) []contracts.Record[map[string]any] {
	out := make([]contracts.Record[map[string]any], 0, len(source)+len(local))
	seen := map[string]bool{}
	for _, record := range local {
		if id := gpuReadModelID(resource, record.Data); id != "" {
			seen[id] = true
		}
		out = append(out, record)
	}
	for _, record := range source {
		id := gpuReadModelID(resource, record.Data)
		if id == "" || !seen[id] {
			out = append(out, record)
		}
	}
	return out
}

func sourceCoHosted(app *platform.App, sourceResource string) bool {
	if app == nil {
		return false
	}
	owner, _, ok := strings.Cut(sourceResource, ":")
	return ok && app.Config.AllowsService(owner)
}
