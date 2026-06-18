package workload

import (
	"net/http"
	"strings"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

const (
	orgProjectsResource       = "org-project-service:projects"
	orgProjectMembersResource = "org-project-service:project_members"
	msgProjectAccessDenied    = "project access denied"
)

func requireProjectAccess(app *platform.App, r *http.Request, projectID string) (int, any, bool) {
	if !workloadProjectAccessEnforced(app) {
		return 0, nil, true
	}
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return http.StatusBadRequest, shared.ErrorData("project_id is required"), false
	}
	if _, found := findWorkloadProject(app, r, projectID); !found {
		return http.StatusForbidden, shared.ErrorData(msgProjectAccessDenied), false
	}
	userID := workloadAuthenticatedUserID(r)
	if userID == "" {
		return http.StatusForbidden, shared.ErrorData(msgProjectAccessDenied), false
	}
	if workloadRequestIsAdmin(r) {
		return 0, nil, true
	}
	if workloadProjectMember(app, r, projectID, userID) {
		return 0, nil, true
	}
	return http.StatusForbidden, shared.ErrorData(msgProjectAccessDenied), false
}

func authorizedWorkloadProjects(app *platform.App, r *http.Request) (map[string]bool, bool, int, any, bool) {
	if !workloadProjectAccessEnforced(app) || workloadRequestIsAdmin(r) {
		return nil, true, 0, nil, true
	}
	userID := workloadAuthenticatedUserID(r)
	if userID == "" {
		return nil, false, http.StatusForbidden, shared.ErrorData(msgProjectAccessDenied), false
	}
	projects := map[string]bool{}
	for _, record := range app.Store.List(r.Context(), orgProjectMembersResource) {
		if shared.TextValue(record.Data, "user_id", "userId") != userID {
			continue
		}
		projectID := shared.TextValue(record.Data, "project_id", "projectId")
		if projectID != "" {
			projects[projectID] = true
		}
	}
	return projects, false, 0, nil, true
}

func filterRecordsForAuthorizedProjects(records []contracts.Record[map[string]any], projects map[string]bool, all bool) []contracts.Record[map[string]any] {
	if all {
		return records
	}
	filtered := make([]contracts.Record[map[string]any], 0, len(records))
	for _, record := range records {
		if projects[shared.TextValue(record.Data, "project_id", "projectId")] {
			filtered = append(filtered, record)
		}
	}
	return filtered
}

func findWorkloadProject(app *platform.App, r *http.Request, projectID string) (contracts.Record[map[string]any], bool) {
	if app == nil || app.Store == nil || projectID == "" {
		return contracts.Record[map[string]any]{}, false
	}
	if record, found := app.Store.Get(r.Context(), orgProjectsResource, projectID); found {
		return record, true
	}
	for _, record := range app.Store.List(r.Context(), orgProjectsResource) {
		if shared.TextValue(record.Data, "id", "ID", "p_id", "PID", "project_id", "projectId") == projectID {
			return record, true
		}
	}
	return contracts.Record[map[string]any]{}, false
}

func workloadProjectMember(app *platform.App, r *http.Request, projectID, userID string) bool {
	if app == nil || app.Store == nil || projectID == "" || userID == "" {
		return false
	}
	for _, id := range []string{projectID + ":" + userID, projectID + "/" + userID} {
		if record, found := app.Store.Get(r.Context(), orgProjectMembersResource, id); found && memberRecordMatches(record, projectID, userID) {
			return true
		}
	}
	for _, record := range app.Store.List(r.Context(), orgProjectMembersResource) {
		if memberRecordMatches(record, projectID, userID) {
			return true
		}
	}
	return false
}

func memberRecordMatches(record contracts.Record[map[string]any], projectID, userID string) bool {
	return shared.TextValue(record.Data, "project_id", "projectId") == projectID &&
		shared.TextValue(record.Data, "user_id", "userId") == userID
}

func workloadProjectAccessEnforced(app *platform.App) bool {
	return app != nil && app.Config.RequireAuth
}

func workloadAuthenticatedUserID(r *http.Request) string {
	if r == nil {
		return ""
	}
	return strings.TrimSpace(r.Header.Get("X-User-ID"))
}

func workloadRequestIsAdmin(r *http.Request) bool {
	if r == nil {
		return false
	}
	role := strings.ToLower(strings.TrimSpace(r.Header.Get("X-User-Role")))
	if role == "admin" || role == "superadmin" || role == "root" {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Admin")), "true")
}
