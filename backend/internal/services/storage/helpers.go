package storage

import (
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

func publishEvent(app *platform.App, r *http.Request, name string, data map[string]any) {
	if err := app.Events.Publish(r.Context(), contracts.Event{
		EventID:       platform.NewUUID(),
		Name:          name,
		Source:        serviceName,
		OccurredAt:    time.Now().UTC(),
		TraceID:       firstNonEmpty(r.Header.Get("X-Trace-ID"), r.Header.Get("X-Request-ID"), platform.NewUUID()),
		SchemaVersion: 1,
		Data:          data,
	}); err != nil {
		slog.Error("storage event publish failed", "event", name, "error", err)
	}
}

func storageRows(app *platform.App, r *http.Request, resource string) []map[string]any {
	records := app.Store.List(r.Context(), resource)
	rows := make([]map[string]any, 0, len(records))
	for _, record := range records {
		row := shared.CloneMap(record.Data)
		if row["id"] == nil {
			row["id"] = record.ID
		}
		rows = append(rows, row)
	}
	return rows
}

func accessRows(app *platform.App, r *http.Request, localResource, sourceResource string) []map[string]any {
	local := storageRows(app, r, localResource)
	if !sourceCoHosted(app, sourceResource) {
		return local
	}
	source := storageRows(app, r, sourceResource)
	if len(local) == 0 {
		return source
	}
	return mergeRows(source, local)
}

func sourceCoHosted(app *platform.App, sourceResource string) bool {
	owner, _, ok := strings.Cut(sourceResource, ":")
	return ok && app.Config.AllowsService(owner)
}

func mergeRows(source, local []map[string]any) []map[string]any {
	out := make([]map[string]any, 0, len(source)+len(local))
	seen := map[string]bool{}
	for _, row := range local {
		if id := text(row, "id", "project_id", "user_id", "group_id"); id != "" {
			seen[id] = true
		}
		out = append(out, row)
	}
	for _, row := range source {
		if id := text(row, "id", "project_id", "user_id", "group_id"); id == "" || !seen[id] {
			out = append(out, row)
		}
	}
	return out
}

func requireUser(r *http.Request) (string, int, any, bool) {
	userID := strings.TrimSpace(r.Header.Get("X-User-ID"))
	if userID == "" {
		return "", http.StatusUnauthorized, shared.ErrorData("unauthorized"), false
	}
	return userID, 0, nil, true
}

func hasAdminPanel(app *platform.App, r *http.Request, userID string) bool {
	if strings.EqualFold(r.Header.Get("X-User-Role"), "admin") {
		return true
	}
	roles := accessRows(app, r, storageIdentityRolesResource, identityRolesResource)
	for _, user := range accessRows(app, r, storageIdentityUsersResource, identityUsersResource) {
		if text(user, "id", "user_id", "userId") != userID {
			continue
		}
		if grantsAdmin(user) {
			return true
		}
		roleID := text(user, "role_id", "roleId", "role")
		for _, role := range roles {
			if text(role, "id", "name") == roleID && grantsAdmin(role) {
				return true
			}
		}
	}
	return false
}

func grantsAdmin(data map[string]any) bool {
	if shared.BoolValue(data, "admin_panel", "adminPanel", "AdminPanel") {
		return true
	}
	return shared.BoolValue(shared.MapValue(data, "capabilities", "Capabilities"), "admin_panel", "adminPanel", "AdminPanel")
}

func canReadGroup(app *platform.App, r *http.Request, groupID, userID string) bool {
	return hasAdminPanel(app, r, userID) || groupRole(app, r, groupID, userID) != ""
}

func canManageGroup(app *platform.App, r *http.Request, groupID, userID string) bool {
	role := groupRole(app, r, groupID, userID)
	return hasAdminPanel(app, r, userID) || role == "admin" || role == "manager"
}

func groupRole(app *platform.App, r *http.Request, groupID, userID string) string {
	for _, member := range userGroupRows(app, r) {
		if text(member, "group_id", "groupId", "gid", "g_id") == groupID && text(member, "user_id", "userId", "uid", "u_id") == userID {
			return normalizeRole(text(member, "role"))
		}
	}
	return ""
}

func userGroupRows(app *platform.App, r *http.Request) []map[string]any {
	return accessRows(app, r, storageUserGroupsResource, orgUserGroupsResource)
}

func requireProjectRead(app *platform.App, r *http.Request, projectID, userID string) (map[string]any, int, any, bool) {
	project, found := findProject(app, r, projectID)
	if !found {
		return nil, http.StatusNotFound, shared.ErrorData("Project not found"), false
	}
	if !canReadProject(projectRole(app, r, project, userID)) {
		return nil, http.StatusForbidden, shared.ErrorData(msgProjectMember), false
	}
	return project, 0, nil, true
}

func requireProjectManager(app *platform.App, r *http.Request, projectID, userID string) (map[string]any, int, any, bool) {
	project, found := findProject(app, r, projectID)
	if !found {
		return nil, http.StatusNotFound, shared.ErrorData("Project not found"), false
	}
	if !canManageProject(projectRole(app, r, project, userID)) {
		return nil, http.StatusForbidden, shared.ErrorData(msgProjectManager), false
	}
	return project, 0, nil, true
}

func findProject(app *platform.App, r *http.Request, projectID string) (map[string]any, bool) {
	for _, project := range accessRows(app, r, storageProjectsResource, orgProjectsResource) {
		if text(project, "id", "p_id", "PID", "project_id", "projectId") == projectID {
			return project, true
		}
	}
	return nil, false
}

func projectRole(app *platform.App, r *http.Request, project map[string]any, userID string) string {
	if hasAdminPanel(app, r, userID) {
		return "admin"
	}
	if text(project, "personal_user_id", "personalUserID") == userID {
		return "admin"
	}
	if role := groupRole(app, r, text(project, "owner_id", "ownerId", "GID", "g_id"), userID); role != "" {
		return role
	}
	projectID := text(project, "id", "p_id", "PID", "project_id", "projectId")
	for _, member := range accessRows(app, r, storageProjectMembersResource, orgProjectMembersResource) {
		if text(member, "project_id", "projectId") == projectID && text(member, "user_id", "userId") == userID {
			return normalizeRole(text(member, "role"))
		}
	}
	return ""
}

func canReadProject(role string) bool {
	return role == "admin" || role == "manager" || role == "user"
}

func canManageProject(role string) bool {
	return role == "admin" || role == "manager"
}

func commandGroupStorage(app *platform.App, r *http.Request, _ platform.RouteSpec, statusValue string) (int, any, *platform.Degraded) {
	userID, status, data, ok := requireUser(r)
	if !ok {
		return status, data, nil
	}
	groupID, pvcID := groupPathID(r), pvcPathID(r)
	if !canReadGroup(app, r, groupID, userID) {
		return http.StatusForbidden, shared.ErrorData(msgGroupMemberRequired), nil
	}
	updated, ok := storageRepo(app).UpdateGroupStorageStatus(r.Context(), groupID, pvcID, statusValue, time.Now().UTC())
	if !ok {
		return http.StatusNotFound, shared.ErrorData("group storage not found"), nil
	}
	publishEvent(app, r, "GroupStorageCommanded", updated)
	return http.StatusOK, updated, nil
}

func batchStoragePermissions(app *platform.App, r *http.Request, delete bool) (int, any, *platform.Degraded) {
	userID, status, data, ok := requireUser(r)
	if !ok {
		return status, data, nil
	}
	payload, err := platform.DecodeMapWithError(r)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(msgInvalidRequestBody), nil
	}
	groupID := shared.TextValue(payload, "group_id", "groupId")
	if !canManageGroup(app, r, groupID, userID) {
		return http.StatusForbidden, shared.ErrorData(msgGroupAdminRequired), nil
	}
	result := batchResult()
	for _, item := range payloadItems(payload) {
		item["group_id"] = firstNonEmpty(shared.TextValue(item, "group_id", "groupId"), groupID)
		if delete {
			storageRepo(app).DeleteStoragePermission(
				r.Context(),
				shared.TextValue(item, "group_id", "groupId"),
				shared.TextValue(item, "pvc_id", "pvcId"),
				shared.TextValue(item, "user_id", "userId"),
			)
		} else {
			record, err := permissionRecord(item)
			if err != nil {
				result["failed"] = result["failed"].(int) + 1
				result["errors"] = append(result["errors"].([]string), err.Error())
				continue
			}
			if _, err := storageRepo(app).UpsertStoragePermission(r.Context(), record); err != nil {
				result["failed"] = result["failed"].(int) + 1
				result["errors"] = append(result["errors"].([]string), err.Error())
				continue
			}
		}
		result["succeeded"] = result["succeeded"].(int) + 1
	}
	return http.StatusOK, result, nil
}

func permissionRecord(payload map[string]any) (map[string]any, error) {
	groupID := shared.TextValue(payload, "group_id", "groupId")
	pvcID := shared.TextValue(payload, "pvc_id", "pvcId")
	userID := shared.TextValue(payload, "user_id", "userId")
	permission := normalizePermission(shared.TextValue(payload, "permission"))
	if groupID == "" || pvcID == "" || userID == "" || permission == "" {
		return nil, fmt.Errorf("group_id, pvc_id, user_id and permission are required")
	}
	return map[string]any{"id": storagePermissionID(groupID, pvcID, userID), "group_id": groupID, "pvc_id": pvcID, "user_id": userID, "permission": permission, "updated_at": time.Now().UTC()}, nil
}

func permissionsForPVC(app *platform.App, r *http.Request, groupID, pvcID string) []map[string]any {
	rows := storageRepo(app).ListStoragePermissionsForPVC(r.Context(), groupID, pvcID)
	sortRows(rows, "user_id")
	return rows
}

func projectPermissionsForPVC(app *platform.App, r *http.Request, projectID, pvcID string) []map[string]any {
	rows := storageRepo(app).ListProjectPermissionsForPVC(r.Context(), projectID, pvcID)
	sortRows(rows, "user_id")
	return rows
}

func batchProjectPermissions(app *platform.App, r *http.Request, delete bool) (int, any, *platform.Degraded) {
	userID, status, data, ok := requireUser(r)
	if !ok {
		return status, data, nil
	}
	projectID, pvcID := projectPathID(r), pvcPathID(r)
	if _, status, data, ok := requireProjectManager(app, r, projectID, userID); !ok {
		return status, data, nil
	}
	payload, err := platform.DecodeMapWithError(r)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(msgInvalidRequestBody), nil
	}
	result := batchResult()
	for _, item := range payloadItems(payload) {
		targetUserID := shared.TextValue(item, "user_id", "userId")
		if delete {
			storageRepo(app).DeleteProjectPermission(r.Context(), projectID, pvcID, targetUserID)
		} else {
			record := projectPermissionRecord(projectID, pvcID, targetUserID, normalizePermission(shared.TextValue(item, "permission")))
			if _, err := storageRepo(app).UpsertProjectPermission(r.Context(), record); err != nil {
				result["failed"] = result["failed"].(int) + 1
				result["errors"] = append(result["errors"].([]string), err.Error())
				continue
			}
		}
		result["succeeded"] = result["succeeded"].(int) + 1
	}
	return http.StatusOK, result, nil
}

func projectPermissionRecord(projectID, pvcID, userID, permission string) map[string]any {
	return map[string]any{"id": projectPermissionID(projectID, pvcID, userID), "project_id": projectID, "pvc_id": pvcID, "user_id": userID, "permission": permission, "updated_at": time.Now().UTC()}
}

func batchUserStorageCommand(app *platform.App, r *http.Request, route platform.RouteSpec, statusValue string) (int, any, *platform.Degraded) {
	userID, status, data, ok := requireUser(r)
	if !ok {
		return status, data, nil
	}
	if !hasAdminPanel(app, r, userID) {
		return http.StatusForbidden, shared.ErrorData(msgAdminRequired), nil
	}
	payload, err := platform.DecodeMapWithError(r)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(msgInvalidRequestBody), nil
	}
	result := batchResult()
	for _, username := range firstStringSlice(payload, "usernames", "users") {
		status, data, _ := userStorageCommand(app, r, route, username, statusValue)
		if status >= 400 {
			result["failed"] = result["failed"].(int) + 1
			result["errors"] = append(result["errors"].([]string), batchError(username, data))
			continue
		}
		result["succeeded"] = result["succeeded"].(int) + 1
	}
	return http.StatusOK, result, nil
}

func userStorageCommand(app *platform.App, r *http.Request, _ platform.RouteSpec, username, statusValue string) (int, any, *platform.Degraded) {
	userID, status, data, ok := requireUser(r)
	if !ok {
		return status, data, nil
	}
	if !hasAdminPanel(app, r, userID) {
		return http.StatusForbidden, shared.ErrorData(msgAdminRequired), nil
	}
	payload := platform.DecodeMap(r)
	size := firstNonEmpty(shared.TextValue(payload, "size"), shared.TextValue(payload, "quota"), "10Gi")
	record := map[string]any{"id": username, "username": username, "size": size, "status": statusValue, "updated_at": time.Now().UTC()}
	updated, err := storageRepo(app).UpsertUserStorage(r.Context(), username, record)
	if err != nil {
		return http.StatusConflict, shared.ErrorData("user storage could not be saved"), nil
	}
	publishEvent(app, r, "UserStorageChanged", updated)
	return http.StatusOK, updated, nil
}

func userStorageStatus(app *platform.App, r *http.Request, username string) map[string]any {
	return storageRepo(app).UserStorageStatus(r.Context(), username)
}

func groupStorageRows(app *platform.App, r *http.Request, groupID string) []map[string]any {
	return storageRepo(app).ListGroupStorageByGroup(r.Context(), groupID)
}

func filterRows(rows []map[string]any, key, value string) []map[string]any {
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		if text(row, key) == value {
			out = append(out, row)
		}
	}
	return out
}

func payloadItems(payload map[string]any) []map[string]any {
	out := make([]map[string]any, 0)
	if raw, ok := payload["items"].([]any); ok {
		for _, item := range raw {
			if row, ok := item.(map[string]any); ok {
				out = append(out, row)
			}
		}
	}
	if raw, ok := payload["permissions"].([]any); ok {
		for _, item := range raw {
			if row, ok := item.(map[string]any); ok {
				out = append(out, row)
			}
		}
	}
	return out
}

func optionRows(options []string) []string {
	out := make([]string, 0)
	for _, item := range options {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}

func normalizePermission(permission string) string {
	switch strings.ToLower(strings.TrimSpace(permission)) {
	case "read", "read_only", "readonly":
		return "read_only"
	case "write", "read_write", "readwrite":
		return "read_write"
	case "none", "":
		return "none"
	default:
		return strings.ToLower(strings.TrimSpace(permission))
	}
}

func normalizeRole(role string) string {
	role = strings.ToLower(strings.TrimSpace(role))
	if role == "member" {
		return "user"
	}
	return role
}

func sortRows(rows []map[string]any, keys ...string) {
	sort.Slice(rows, func(i, j int) bool {
		for _, key := range keys {
			left, right := text(rows[i], key), text(rows[j], key)
			if left != right {
				return left < right
			}
		}
		return false
	})
}

func firstStringSlice(payload map[string]any, keys ...string) []string {
	for _, key := range keys {
		if values := shared.StringSlice(payload[key]); len(values) > 0 {
			return values
		}
	}
	return nil
}

func batchResult() map[string]any {
	return map[string]any{"succeeded": 0, "failed": 0, "errors": []string{}}
}

func batchError(id string, data any) string {
	if row, ok := data.(map[string]any); ok {
		return id + ": " + shared.TextValue(row, "message")
	}
	return id + ": failed"
}

func projectPathID(r *http.Request) string {
	return strings.TrimSpace(r.PathValue("id"))
}

func groupPathID(r *http.Request) string {
	return strings.TrimSpace(firstNonEmpty(r.PathValue("id"), r.PathValue("group_id")))
}

func pvcPathID(r *http.Request) string {
	return strings.TrimSpace(firstNonEmpty(r.PathValue("pvcId"), r.PathValue("pvc_id")))
}

func groupStorageID(groupID, pvcID string) string {
	return groupID + ":" + pvcID
}

func storagePermissionID(groupID, pvcID, userID string) string {
	return groupID + ":" + pvcID + ":" + userID
}

func storagePolicyID(groupID, pvcID string) string {
	return groupID + ":" + pvcID
}

func projectBindingID(projectID, pvcID string) string {
	return projectID + ":" + pvcID
}

func projectPermissionID(projectID, pvcID, userID string) string {
	return projectID + ":" + pvcID + ":" + userID
}

func fastTransferID(projectID, namespace, name string) string {
	return projectID + ":" + namespace + ":" + name
}

func text(data map[string]any, keys ...string) string {
	return shared.TextValue(data, keys...)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
