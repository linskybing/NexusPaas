package orgproject

import (
	"net/http"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

func listProjects(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	userID, status, data, ok := requireUser(r)
	if !ok {
		return status, data, nil
	}
	projects := visibleProjects(app, r, userID)
	if r.URL.Query().Get("page") != "" {
		return http.StatusOK, pagedRows(r, projects), nil
	}
	return http.StatusOK, projects, nil
}

func listProjectsByUser(app *platform.App, r *http.Request, route platform.RouteSpec) (int, any, *platform.Degraded) {
	return listProjects(app, r, route)
}

func createProject(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	userID, status, data, ok := requireUser(r)
	if !ok {
		return status, data, nil
	}
	if !hasAdminPanel(app, r, userID) {
		return http.StatusForbidden, shared.ErrorData(msgAdminAccessRequired), nil
	}
	payload, err := platform.DecodeMapWithError(r)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(msgInvalidRequestBody), nil
	}
	name := firstNonEmpty(shared.TextValue(payload, "project_name", "ProjectName"), shared.TextValue(payload, "name"))
	ownerID := firstNonEmpty(shared.TextValue(payload, "g_id", "gid", "GID"), shared.TextValue(payload, "group_id", "owner_id", "ownerId"))
	if name == "" || ownerID == "" {
		return http.StatusBadRequest, shared.ErrorData("project_name and g_id are required"), nil
	}
	if _, found := findGroup(app, r, ownerID); !found {
		return http.StatusNotFound, shared.ErrorData(msgGroupNotFound), nil
	}
	id := firstNonEmpty(shared.TextValue(payload, "id", "p_id", "PID"), newProjectID(app))
	now := time.Now().UTC()
	project := map[string]any{
		"id":           id,
		"p_id":         id,
		"PID":          id,
		"project_id":   id,
		"project_name": name,
		"ProjectName":  name,
		"name":         name,
		"description":  shared.TextValue(payload, "description", "Description"),
		"Description":  shared.TextValue(payload, "description", "Description"),
		"owner_id":     ownerID,
		"GID":          ownerID,
		"g_id":         ownerID,
		"path":         firstNonEmpty(shared.TextValue(payload, "path"), id),
		"created_at":   now,
		"create_at":    now,
		"created_by":   userID,
	}
	applyProjectMutableFields(project, payload, true)
	record, err := projectRepository(app).CreateProject(r.Context(), project)
	if err != nil {
		if platform.IsCreateConflict(err) {
			return http.StatusConflict, shared.ErrorData("project already exists"), nil
		}
		return http.StatusInternalServerError, shared.ErrorData("project could not be created"), nil
	}
	publishEvent(app, r, eventFor(r, "ProjectCreated", record.Data))
	return http.StatusCreated, record.Data, nil
}

func getProject(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	userID, status, data, ok := requireUser(r)
	if !ok {
		return status, data, nil
	}
	project, status, data, ok := requireProjectRead(app, r, projectPathID(r), userID)
	if !ok {
		return status, data, nil
	}
	return http.StatusOK, project, nil
}

func updateProject(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	userID, status, data, ok := requireUser(r)
	if !ok {
		return status, data, nil
	}
	if !hasAdminPanel(app, r, userID) {
		return http.StatusForbidden, shared.ErrorData(msgAdminAccessRequired), nil
	}
	projectID := projectPathID(r)
	existing, found := projectRepository(app).FindProject(r.Context(), projectID)
	if !found {
		return http.StatusNotFound, shared.ErrorData(msgProjectNotFound), nil
	}
	payload, err := platform.DecodeMapWithError(r)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(msgInvalidRequestBody), nil
	}
	if ownerID := firstNonEmpty(shared.TextValue(payload, "g_id", "gid", "GID"), shared.TextValue(payload, "group_id", "owner_id", "ownerId")); ownerID != "" {
		if _, found := findGroup(app, r, ownerID); !found {
			return http.StatusNotFound, shared.ErrorData(msgGroupNotFound), nil
		}
	}
	update := map[string]any{"updated_at": time.Now().UTC()}
	applyProjectMutableFields(update, payload, false)
	if len(update) == 1 {
		return http.StatusOK, existing.Data, nil
	}
	old, updated, ok := projectRepository(app).UpdateProject(r.Context(), projectID, update)
	if !ok {
		return http.StatusInternalServerError, shared.ErrorData("project update failed"), nil
	}
	publishEvent(app, r, eventFor(r, "ProjectUpdated", map[string]any{"old": old.Data, "new": updated.Data}))
	return http.StatusOK, updated.Data, nil
}

func deleteProject(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	userID, status, data, ok := requireUser(r)
	if !ok {
		return status, data, nil
	}
	if !hasAdminPanel(app, r, userID) {
		return http.StatusForbidden, shared.ErrorData(msgAdminAccessRequired), nil
	}
	projectID := projectPathID(r)
	project, found := findProject(app, r, projectID)
	if !found {
		return http.StatusNotFound, shared.ErrorData(msgProjectNotFound), nil
	}
	deleteProjectByID(app, r, projectID)
	publishEvent(app, r, eventFor(r, "ProjectDeleted", project))
	return http.StatusOK, nil, nil
}

func batchDeleteProjects(app *platform.App, r *http.Request, route platform.RouteSpec) (int, any, *platform.Degraded) {
	userID, status, data, ok := requireUser(r)
	if !ok {
		return status, data, nil
	}
	if !hasAdminPanel(app, r, userID) {
		return http.StatusForbidden, shared.ErrorData(msgAdminAccessRequired), nil
	}
	payload, err := platform.DecodeMapWithError(r)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(msgInvalidRequestBody), nil
	}
	result := batchResult()
	for _, id := range requestIDs(payload) {
		req := r.Clone(r.Context())
		req.SetPathValue("id", id)
		status, data, _ := deleteProject(app, req, route)
		if status >= 400 {
			result["failed"] = result["failed"].(int) + 1
			result["errors"] = append(result["errors"].([]string), batchError(id, data))
			continue
		}
		result["succeeded"] = result["succeeded"].(int) + 1
	}
	return http.StatusOK, result, nil
}

func listProjectMembers(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	userID, status, data, ok := requireUser(r)
	if !ok {
		return status, data, nil
	}
	projectID := projectPathID(r)
	if _, status, data, ok := requireProjectRead(app, r, projectID, userID); !ok {
		return status, data, nil
	}
	return http.StatusOK, mergedProjectMembers(app, r, projectID), nil
}

func projectAddMembersContext(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	userID, status, data, ok := requireUser(r)
	if !ok {
		return status, data, nil
	}
	projectID := projectPathID(r)
	project, status, data, ok := requireProjectAdmin(app, r, projectID, userID)
	if !ok {
		return status, data, nil
	}
	current := mergedProjectMembers(app, r, projectID)
	excluded := map[string]bool{}
	for _, member := range current {
		excluded[shared.TextValue(member, "user_id", "userId")] = true
	}
	available := make([]map[string]any, 0)
	for _, user := range userRows(app, r) {
		id := userIDFromMap(user)
		if id != "" && !excluded[id] && userVisible(user) {
			available = append(available, user)
		}
	}
	return http.StatusOK, map[string]any{
		"project":         project,
		"current_members": current,
		"available_users": pagedRows(r, available),
	}, nil
}

func addProjectMembers(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	userID, status, data, ok := requireUser(r)
	if !ok {
		return status, data, nil
	}
	projectID := projectPathID(r)
	if _, status, data, ok := requireProjectAdmin(app, r, projectID, userID); !ok {
		return status, data, nil
	}
	payload, err := platform.DecodeMapWithError(r)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(msgInvalidRequestBody), nil
	}
	members := projectMemberInputs(payload)
	if len(members) == 0 {
		return http.StatusBadRequest, shared.ErrorData("members are required"), nil
	}
	result := batchResult()
	for _, member := range members {
		if err := createDirectProjectMember(app, r, projectID, member.UserID, member.Role, userID); err != nil {
			result["failed"] = result["failed"].(int) + 1
			result["errors"] = append(result["errors"].([]string), member.UserID+": "+err.Error())
			continue
		}
		result["succeeded"] = result["succeeded"].(int) + 1
	}
	return http.StatusOK, result, nil
}

func batchRemoveProjectMembers(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	userID, status, data, ok := requireUser(r)
	if !ok {
		return status, data, nil
	}
	projectID := projectPathID(r)
	if _, status, data, ok := requireProjectAdmin(app, r, projectID, userID); !ok {
		return status, data, nil
	}
	payload, err := platform.DecodeMapWithError(r)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(msgInvalidRequestBody), nil
	}
	result := batchResult()
	for _, id := range requestUserIDs(payload) {
		status, data := deleteDirectProjectMember(app, r, projectID, id)
		if status >= 400 {
			result["failed"] = result["failed"].(int) + 1
			result["errors"] = append(result["errors"].([]string), batchError(id, data))
			continue
		}
		result["succeeded"] = result["succeeded"].(int) + 1
	}
	return http.StatusOK, result, nil
}

func removeProjectMember(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	userID, status, data, ok := requireUser(r)
	if !ok {
		return status, data, nil
	}
	projectID := projectPathID(r)
	if _, status, data, ok := requireProjectAdmin(app, r, projectID, userID); !ok {
		return status, data, nil
	}
	status, data = deleteDirectProjectMember(app, r, projectID, projectUserPathID(r))
	return status, data, nil
}

func updateProjectMemberRole(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	userID, status, data, ok := requireUser(r)
	if !ok {
		return status, data, nil
	}
	projectID := projectPathID(r)
	if _, status, data, ok := requireProjectAdmin(app, r, projectID, userID); !ok {
		return status, data, nil
	}
	payload, err := platform.DecodeMapWithError(r)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(msgInvalidRequestBody), nil
	}
	status, data = updateDirectProjectMemberRole(app, r, projectID, projectUserPathID(r), normalizeProjectRole(shared.TextValue(payload, "role")))
	return status, data, nil
}

func batchUpdateProjectMemberRoles(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	userID, status, data, ok := requireUser(r)
	if !ok {
		return status, data, nil
	}
	projectID := projectPathID(r)
	if _, status, data, ok := requireProjectAdmin(app, r, projectID, userID); !ok {
		return status, data, nil
	}
	payload, err := platform.DecodeMapWithError(r)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(msgInvalidRequestBody), nil
	}
	result := batchResult()
	for _, item := range roleUpdateInputs(payload) {
		status, data := updateDirectProjectMemberRole(app, r, projectID, item.UserID, item.Role)
		if status >= 400 {
			result["failed"] = result["failed"].(int) + 1
			result["errors"] = append(result["errors"].([]string), batchError(item.UserID, data))
			continue
		}
		result["succeeded"] = result["succeeded"].(int) + 1
	}
	return http.StatusOK, result, nil
}

func getProjectMemberQuota(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	actor, status, data, ok := requireUser(r)
	if !ok {
		return status, data, nil
	}
	projectID, targetUserID := projectPathID(r), projectUserPathID(r)
	if targetUserID == "" {
		return http.StatusBadRequest, shared.ErrorData("Missing user ID"), nil
	}
	project, found := findProject(app, r, projectID)
	if !found {
		return http.StatusNotFound, shared.ErrorData(msgProjectNotFound), nil
	}
	role := projectRoleForUser(app, r, project, actor)
	if actor != targetUserID && !canManageProject(role) {
		return http.StatusForbidden, shared.ErrorData("quota access requires self or project manager"), nil
	}
	if record, found := projectRepository(app).GetProjectUserQuota(r.Context(), projectID, targetUserID); found {
		return http.StatusOK, quotaDTO(record.Data), nil
	}
	return http.StatusOK, quotaDTO(nil), nil
}

func upsertProjectMemberQuota(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	actor, status, data, ok := requireUser(r)
	if !ok {
		return status, data, nil
	}
	projectID, targetUserID := projectPathID(r), projectUserPathID(r)
	if _, status, data, ok := requireProjectAdmin(app, r, projectID, actor); !ok {
		return status, data, nil
	}
	payload, err := platform.DecodeMapWithError(r)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(msgInvalidRequestBody), nil
	}
	if err := validateQuotaPayload(payload); err != nil {
		return http.StatusBadRequest, shared.ErrorData(err.Error()), nil
	}
	quota := quotaRecord(projectID, targetUserID, payload)
	return upsertQuota(app, r, quota)
}

func deleteProjectMemberQuota(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	actor, status, data, ok := requireUser(r)
	if !ok {
		return status, data, nil
	}
	projectID, targetUserID := projectPathID(r), projectUserPathID(r)
	if _, status, data, ok := requireProjectAdmin(app, r, projectID, actor); !ok {
		return status, data, nil
	}
	projectRepository(app).DeleteProjectUserQuota(r.Context(), projectID, targetUserID)
	publishEvent(app, r, eventFor(r, "UserQuotaDeleted", map[string]any{"project_id": projectID, "user_id": targetUserID}))
	return http.StatusOK, nil, nil
}

func batchSetProjectMemberQuotas(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	actor, status, data, ok := requireUser(r)
	if !ok {
		return status, data, nil
	}
	projectID := projectPathID(r)
	if _, status, data, ok := requireProjectAdmin(app, r, projectID, actor); !ok {
		return status, data, nil
	}
	payload, err := platform.DecodeMapWithError(r)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(msgInvalidRequestBody), nil
	}
	updates := quotaUpdateInputs(payload)
	for _, update := range updates {
		if err := validateQuotaPayload(update); err != nil {
			return http.StatusBadRequest, shared.ErrorData(err.Error()), nil
		}
	}
	result := batchResult()
	for _, update := range updates {
		_, data, _ := upsertQuota(app, r, quotaRecord(projectID, shared.TextValue(update, "user_id", "userId"), update))
		if message := shared.TextValue(data.(map[string]any), "message"); message != "" {
			result["failed"] = result["failed"].(int) + 1
			result["errors"] = append(result["errors"].([]string), message)
			continue
		}
		result["succeeded"] = result["succeeded"].(int) + 1
	}
	return http.StatusOK, result, nil
}

func getProjectWorkspaceSettings(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	userID, status, data, ok := requireUser(r)
	if !ok {
		return status, data, nil
	}
	project, status, data, ok := requireProjectRead(app, r, projectPathID(r), userID)
	if !ok {
		return status, data, nil
	}
	return http.StatusOK, workspaceSettings(project), nil
}

func updateProjectWorkspaceSettings(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	userID, status, data, ok := requireUser(r)
	if !ok {
		return status, data, nil
	}
	projectID := projectPathID(r)
	if _, status, data, ok := requireProjectManager(app, r, projectID, userID); !ok {
		return status, data, nil
	}
	payload, err := platform.DecodeMapWithError(r)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(msgInvalidRequestBody), nil
	}
	if !hasAny(payload, "max_ide_runtime_seconds", "MaxIDERuntimeSeconds") {
		return http.StatusBadRequest, shared.ErrorData("max_ide_runtime_seconds is required"), nil
	}
	seconds := shared.IntValue(payload, "max_ide_runtime_seconds", "MaxIDERuntimeSeconds")
	if seconds < 0 {
		return http.StatusBadRequest, shared.ErrorData("quota values must be non-negative"), nil
	}
	existing, updated, ok := projectRepository(app).UpdateWorkspaceSettings(r.Context(), projectID, seconds, time.Now().UTC())
	if !ok {
		return http.StatusInternalServerError, shared.ErrorData("workspace settings update failed"), nil
	}
	publishEvent(app, r, eventFor(r, "ProjectUpdated", map[string]any{"old": existing.Data, "new": updated.Data}))
	return http.StatusOK, updated.Data, nil
}

func listGPUClaims(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	userID, status, data, ok := requireUser(r)
	if !ok {
		return status, data, nil
	}
	projectID := projectPathID(r)
	if _, status, data, ok := requireProjectRead(app, r, projectID, userID); !ok {
		return status, data, nil
	}
	items := make([]map[string]any, 0)
	for _, claim := range groupGPURepository(app).ListGPUClaimsByProject(r.Context(), projectID) {
		if r.URL.Query().Get("scope") == "mine" && shared.TextValue(claim.Data, "user_id", "userId") != userID {
			continue
		}
		items = append(items, shared.CloneMap(claim.Data))
	}
	sortRows(items, "namespace", "name")
	return http.StatusOK, items, nil
}

func createGPUClaim(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	userID, status, data, ok := requireUser(r)
	if !ok {
		return status, data, nil
	}
	projectID := projectPathID(r)
	if _, status, data, ok := requireProjectRead(app, r, projectID, userID); !ok {
		return status, data, nil
	}
	payload, err := platform.DecodeMapWithError(r)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(msgInvalidRequestBody), nil
	}
	claim, err := gpuClaimRecord(app, r, projectID, userID, payload)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(err.Error()), nil
	}
	record, err := groupGPURepository(app).CreateGPUClaim(r.Context(), claim)
	if err != nil {
		if platform.IsCreateConflict(err) {
			return http.StatusConflict, shared.ErrorData("gpu claim already exists"), nil
		}
		return http.StatusInternalServerError, shared.ErrorData("gpu claim could not be created"), nil
	}
	publishEvent(app, r, eventFor(r, "GPUClaimCreated", record.Data))
	return http.StatusCreated, record.Data, nil
}

func deleteGPUClaim(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	userID, status, data, ok := requireUser(r)
	if !ok {
		return status, data, nil
	}
	projectID := projectPathID(r)
	project, status, data, ok := requireProjectRead(app, r, projectID, userID)
	if !ok {
		return status, data, nil
	}
	name := firstNonEmpty(r.PathValue("requestId"), r.PathValue("name"))
	record, found := findGPUClaim(app, r, projectID, name, r.URL.Query().Get("namespace"))
	if !found {
		return http.StatusNotFound, shared.ErrorData("gpu claim not found"), nil
	}
	if shared.TextValue(record.Data, "user_id", "userId") != userID && !canManageProject(projectRoleForUser(app, r, project, userID)) {
		return http.StatusForbidden, shared.ErrorData("project manager access required"), nil
	}
	deleted, _ := groupGPURepository(app).DeleteGPUClaim(r.Context(), record.ID)
	publishEvent(app, r, eventFor(r, "GPUClaimDeleted", deleted.Data))
	return http.StatusOK, nil, nil
}
