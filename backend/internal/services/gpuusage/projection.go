package gpuusage

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

const gpuProjectionConsumer = serviceName + ":gpu_usage_projection"

const gpuKeyID = "id"

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
