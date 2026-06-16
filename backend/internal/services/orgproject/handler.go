package orgproject

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

// publishEvent emits a domain event, logging instead of silently dropping a
// publish failure (finding 12: never discard Store/Events errors).
func publishEvent(app *platform.App, r *http.Request, event contracts.Event) {
	if err := app.Events.Publish(r.Context(), event); err != nil {
		slog.Error("orgproject event publish failed", "event", event.Name, "error", err)
	}
}

const (
	serviceName               = "org-project-service"
	projectsResource          = serviceName + ":projects"
	projectMembersResource    = serviceName + ":project_members"
	projectUserQuotasResource = serviceName + ":user_quotas"
	identityConsumer          = serviceName + ":identity_projection"
	orgIdentityUsers          = serviceName + ":identity_users"
	orgIdentityRoles          = serviceName + ":identity_roles"
	usersResource             = "identity-service:users"
	rolesResource             = "identity-service:roles"

	pathProjectID          = "/api/v1/projects/{id}"
	pathProjectMembers     = pathProjectID + "/members"
	pathProjectMemberQuota = pathProjectMembers + "/{userId}/quota"
	pathGroupID            = "/api/v1/groups/{id}"
	pathUserGroups         = "/api/v1/user-groups"

	msgAdminAccessRequired = "admin access required"
	msgInvalidRequestBody  = "invalid request body"
	msgGroupAdminRequired  = "group admin access required"
	msgGroupMembership     = "group membership required"
	msgGroupNotFound       = "Group not found"
	msgProjectNotFound     = "Project not found"
)

var allowedGroupRoles = map[string]bool{"admin": true, "manager": true, "user": true}
var allowedProjectRoles = map[string]bool{"admin": true, "manager": true, "user": true}

func Register(app *platform.App) {
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/groups", listGroups)
	app.RegisterCustomHandler(http.MethodPost, "/api/v1/groups", createGroup)
	app.RegisterCustomHandler(http.MethodGet, pathGroupID, getGroup)
	app.RegisterCustomHandler(http.MethodPut, pathGroupID, updateGroup)
	app.RegisterCustomHandler(http.MethodPatch, pathGroupID, updateGroup)
	app.RegisterCustomHandler(http.MethodDelete, pathGroupID, deleteGroup)
	app.RegisterCustomHandler(http.MethodDelete, "/api/v1/groups/batch", batchDeleteGroups)
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/admin/group-policy-options", groupPolicyOptions)

	app.RegisterCustomHandler(http.MethodGet, pathUserGroups, getUserGroup)
	app.RegisterCustomHandler(http.MethodPost, pathUserGroups, addUserToGroup)
	app.RegisterCustomHandler(http.MethodPut, pathUserGroups, updateUserGroup)
	app.RegisterCustomHandler(http.MethodDelete, pathUserGroups, removeUserFromGroup)
	app.RegisterCustomHandler(http.MethodPost, "/api/v1/user-groups/batch", batchAddMembers)
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/user-groups/by-group", userGroupsByGroup)
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/user-groups/by-user", userGroupsByUser)
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/user-groups/{group_id}/members", groupMembers)
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/user-groups/{group_id}/add-members-context", addMembersContext)
	app.RegisterCustomHandler(http.MethodPost, "/api/v1/user-groups/{group_id}/resolve-add-members", resolveAddMembers)

	app.RegisterCustomHandler(http.MethodGet, "/api/v1/projects", listProjects)
	app.RegisterCustomHandler(http.MethodPost, "/api/v1/projects", createProject)
	app.RegisterCustomHandler(http.MethodDelete, "/api/v1/projects/batch", batchDeleteProjects)
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/projects/by-user", listProjectsByUser)
	app.RegisterCustomHandler(http.MethodGet, pathProjectID, getProject)
	app.RegisterCustomHandler(http.MethodPut, pathProjectID, updateProject)
	app.RegisterCustomHandler(http.MethodPatch, pathProjectID, updateProject)
	app.RegisterCustomHandler(http.MethodDelete, pathProjectID, deleteProject)
	app.RegisterCustomHandler(http.MethodGet, pathProjectMembers, listProjectMembers)
	app.RegisterCustomHandler(http.MethodGet, pathProjectID+"/add-members-context", projectAddMembersContext)
	app.RegisterCustomHandler(http.MethodPost, pathProjectMembers, addProjectMembers)
	app.RegisterCustomHandler(http.MethodDelete, pathProjectMembers, batchRemoveProjectMembers)
	app.RegisterCustomHandler(http.MethodPut, pathProjectMembers+"/roles", batchUpdateProjectMemberRoles)
	app.RegisterCustomHandler(http.MethodPut, pathProjectMembers+"/quotas", batchSetProjectMemberQuotas)
	app.RegisterCustomHandler(http.MethodPut, pathProjectMembers+"/{userId}", updateProjectMemberRole)
	app.RegisterCustomHandler(http.MethodPatch, pathProjectMembers+"/{userId}/role", updateProjectMemberRole)
	app.RegisterCustomHandler(http.MethodDelete, pathProjectMembers+"/{userId}", removeProjectMember)
	app.RegisterCustomHandler(http.MethodGet, pathProjectMemberQuota, getProjectMemberQuota)
	app.RegisterCustomHandler(http.MethodPut, pathProjectMemberQuota, upsertProjectMemberQuota)
	app.RegisterCustomHandler(http.MethodDelete, pathProjectMemberQuota, deleteProjectMemberQuota)
	app.RegisterCustomHandler(http.MethodGet, pathProjectID+"/workspace-settings", getProjectWorkspaceSettings)
	app.RegisterCustomHandler(http.MethodPut, pathProjectID+"/workspace-settings", updateProjectWorkspaceSettings)
	app.RegisterCustomHandler(http.MethodGet, pathProjectID+"/gpu-claims", listGPUClaims)
	app.RegisterCustomHandler(http.MethodPost, pathProjectID+"/gpu-claims", createGPUClaim)
	app.RegisterCustomHandler(http.MethodDelete, pathProjectID+"/gpu-claims/{requestId}", deleteGPUClaim)

	// Service-key-gated internal command contract: lets plan-owning services
	// (scheduler-quota) apply/clear project plan bindings without writing the
	// org-project-owned project aggregate directly (problem.md #2).
	app.RegisterCustomHandler(http.MethodPut, pathBindProjectPlan, bindProjectPlan)
	app.RegisterCustomHandler(http.MethodDelete, pathClearPlanBindings, clearProjectsPlan)

	// Service-key-gated read contracts for cross-service consumers (problem.md #3).
	registerInternalReadContracts(app)
}

func listGroups(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	userID, status, data, ok := requireUser(r)
	if !ok {
		return status, data, nil
	}
	all := groupRows(app, r)
	visible := make([]map[string]any, 0, len(all))
	for _, group := range all {
		groupID := groupID(group)
		if hasAdminPanel(app, r, userID) || isGroupMember(app, r, userID, groupID) {
			visible = append(visible, group)
		}
	}
	return http.StatusOK, pagedRows(r, visible), nil
}

func getGroup(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	userID, status, data, ok := requireUser(r)
	if !ok {
		return status, data, nil
	}
	id := strings.TrimSpace(r.PathValue("id"))
	group, found := findGroup(app, r, id)
	if !found {
		return http.StatusNotFound, shared.ErrorData(msgGroupNotFound), nil
	}
	if !hasAdminPanel(app, r, userID) && !isGroupMember(app, r, userID, id) {
		return http.StatusForbidden, shared.ErrorData(msgGroupMembership), nil
	}
	return http.StatusOK, group, nil
}

func groupPolicyOptions(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	userID, status, data, ok := requireUser(r)
	if !ok {
		return status, data, nil
	}
	if !hasAdminPanel(app, r, userID) {
		return http.StatusForbidden, shared.ErrorData(msgAdminAccessRequired), nil
	}
	storageClasses := optionRows(app.Config.GroupStorageClassOptions)
	registryProfiles := optionRows(app.Config.GroupRegistryProfileOptions)
	return http.StatusOK, map[string]any{
		"storage_classes":            storageClasses,
		"registry_profiles":          registryProfiles,
		"storageClassOptions":        storageClasses,
		"registryProfileOptions":     registryProfiles,
		"allowRunAsRootConfigurable": true,
	}, nil
}

func createGroup(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
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
	name := firstNonEmpty(shared.TextValue(payload, "group_name", "groupName"), shared.TextValue(payload, "name"))
	if name == "" {
		return http.StatusBadRequest, shared.ErrorData("group_name is required"), nil
	}
	if err := validateGroupPolicies(app, payload); err != nil {
		return http.StatusBadRequest, shared.ErrorData(err.Error()), nil
	}
	for _, group := range groupRows(app, r) {
		if strings.EqualFold(shared.TextValue(group, "group_name", "groupName", "name"), name) {
			return http.StatusBadRequest, shared.ErrorData("group already exists"), nil
		}
	}
	id := firstNonEmpty(shared.TextValue(payload, "id", "gid", "g_id"), newGroupID(app, r))
	now := time.Now().UTC()
	group := map[string]any{
		"id":                 id,
		"group_name":         name,
		"name":               name,
		"description":        shared.TextValue(payload, "description"),
		"storage_class":      optionalText(payload, "storage_class", "storageClass"),
		"registry_profile":   optionalText(payload, "registry_profile", "registryProfile"),
		"allow_run_as_root":  shared.BoolValue(payload, "allow_run_as_root", "allowRunAsRoot"),
		"allowed_host_paths": normalizedHostPaths(payload),
		"created_at":         now,
	}
	record, err := groupGPURepository(app).CreateGroup(r.Context(), group)
	if err != nil {
		if platform.IsCreateConflict(err) {
			return http.StatusConflict, shared.ErrorData("group already exists"), nil
		}
		return http.StatusInternalServerError, shared.ErrorData("group could not be created"), nil
	}
	publishEvent(app, r, eventFor(r, "GroupCreated", record.Data))
	return http.StatusCreated, record.Data, nil
}

func updateGroup(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	userID, status, data, ok := requireUser(r)
	if !ok {
		return status, data, nil
	}
	if !hasAdminPanel(app, r, userID) {
		return http.StatusForbidden, shared.ErrorData(msgAdminAccessRequired), nil
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if _, found := groupGPURepository(app).FindGroup(r.Context(), id); !found {
		return http.StatusNotFound, shared.ErrorData("group not found"), nil
	}
	payload, err := platform.DecodeMapWithError(r)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(msgInvalidRequestBody), nil
	}
	if err := validateGroupPolicies(app, payload); err != nil {
		return http.StatusBadRequest, shared.ErrorData(err.Error()), nil
	}
	update := map[string]any{}
	if name := firstNonEmpty(shared.TextValue(payload, "group_name", "groupName"), shared.TextValue(payload, "name")); name != "" {
		update["group_name"] = name
		update["name"] = name
	}
	for _, key := range []string{"description", "storage_class", "registry_profile", "allow_run_as_root", "allowed_host_paths"} {
		if value, ok := payload[key]; ok {
			update[key] = value
		}
	}
	if value, ok := payload["storageClass"]; ok {
		update["storage_class"] = normalizeOptional(value)
	}
	if value, ok := payload["registryProfile"]; ok {
		update["registry_profile"] = normalizeOptional(value)
	}
	update["updated_at"] = time.Now().UTC()
	old, updated, ok := groupGPURepository(app).UpdateGroup(r.Context(), id, update)
	if !ok {
		return http.StatusInternalServerError, shared.ErrorData("group update failed"), nil
	}
	publishEvent(app, r, eventFor(r, "GroupUpdated", map[string]any{"old": old.Data, "new": updated.Data}))
	return http.StatusOK, updated.Data, nil
}

func deleteGroup(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	userID, status, data, ok := requireUser(r)
	if !ok {
		return status, data, nil
	}
	if !hasAdminPanel(app, r, userID) {
		return http.StatusForbidden, shared.ErrorData(msgAdminAccessRequired), nil
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if _, _, found := groupGPURepository(app).DeleteGroupCascade(r.Context(), id); !found {
		return http.StatusNotFound, shared.ErrorData("group not found"), nil
	}
	publishEvent(app, r, eventFor(r, "GroupDeleted", map[string]any{"id": id}))
	return http.StatusOK, nil, nil
}

func batchDeleteGroups(app *platform.App, r *http.Request, route platform.RouteSpec) (int, any, *platform.Degraded) {
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
	ids := shared.StringSlice(payload["ids"])
	result := map[string]any{"succeeded": 0, "failed": 0, "errors": []string{}}
	for _, id := range ids {
		req := r.Clone(r.Context())
		req.SetPathValue("id", id)
		status, data, _ := deleteGroup(app, req, route)
		if status >= 400 {
			result["failed"] = result["failed"].(int) + 1
			result["errors"] = append(result["errors"].([]string), fmt.Sprintf("%s: %s", id, shared.TextValue(data.(map[string]any), "message")))
			continue
		}
		result["succeeded"] = result["succeeded"].(int) + 1
	}
	return http.StatusOK, result, nil
}

func getUserGroup(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	userID, status, data, ok := requireUser(r)
	if !ok {
		return status, data, nil
	}
	uid := firstNonEmpty(r.URL.Query().Get("u_id"), r.URL.Query().Get("uid"), r.URL.Query().Get("user_id"))
	gid := firstNonEmpty(r.URL.Query().Get("g_id"), r.URL.Query().Get("gid"), r.URL.Query().Get("group_id"))
	if uid == "" || gid == "" {
		return http.StatusOK, []any{}, nil
	}
	if userID != uid && !hasAdminPanel(app, r, userID) && !isGroupMember(app, r, userID, gid) {
		return http.StatusForbidden, shared.ErrorData(msgGroupMembership), nil
	}
	membership, found := findUserGroup(app, r, uid, gid)
	if !found {
		return http.StatusNotFound, shared.ErrorData("Not found"), nil
	}
	return http.StatusOK, decorateMembership(app, r, membership), nil
}

func addUserToGroup(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	payload, err := platform.DecodeMapWithError(r)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(msgInvalidRequestBody), nil
	}
	uid := firstNonEmpty(shared.TextValue(payload, "uid", "u_id"), shared.TextValue(payload, "user_id", "userId"))
	gid := firstNonEmpty(shared.TextValue(payload, "gid", "g_id"), shared.TextValue(payload, "group_id", "groupId"))
	role := normalizeRole(firstNonEmpty(shared.TextValue(payload, "role"), "user"))
	return createMembership(app, r, uid, gid, role, false)
}

func updateUserGroup(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	payload, err := platform.DecodeMapWithError(r)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(msgInvalidRequestBody), nil
	}
	uid := firstNonEmpty(shared.TextValue(payload, "uid", "u_id"), shared.TextValue(payload, "user_id", "userId"))
	gid := firstNonEmpty(shared.TextValue(payload, "gid", "g_id"), shared.TextValue(payload, "group_id", "groupId"))
	role := normalizeRole(shared.TextValue(payload, "role"))
	return createMembership(app, r, uid, gid, role, true)
}

func removeUserFromGroup(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	actor, status, data, ok := requireUser(r)
	if !ok {
		return status, data, nil
	}
	uid := firstNonEmpty(r.URL.Query().Get("uid"), r.URL.Query().Get("u_id"), r.URL.Query().Get("user_id"))
	gid := firstNonEmpty(r.URL.Query().Get("gid"), r.URL.Query().Get("g_id"), r.URL.Query().Get("group_id"))
	if uid == "" || gid == "" {
		return http.StatusBadRequest, shared.ErrorData("uid and gid are required"), nil
	}
	if !isGroupAdmin(app, r, actor, gid) {
		return http.StatusForbidden, shared.ErrorData(msgGroupAdminRequired), nil
	}
	_, found := groupGPURepository(app).FindMembership(r.Context(), uid, gid)
	if !found {
		return http.StatusNotFound, shared.ErrorData("Not found"), nil
	}
	groupGPURepository(app).DeleteMembership(r.Context(), uid, gid)
	publishEvent(app, r, eventFor(r, "GroupMembershipChanged", map[string]any{"user_id": uid, "group_id": gid, "action": "delete"}))
	return http.StatusOK, nil, nil
}

func batchAddMembers(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	payload, err := platform.DecodeMapWithError(r)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(msgInvalidRequestBody), nil
	}
	gid := firstNonEmpty(shared.TextValue(payload, "gid", "g_id"), shared.TextValue(payload, "group_id", "groupId"))
	role := normalizeRole(shared.TextValue(payload, "role"))
	userIDs := shared.StringSlice(firstNonNil(payload["user_ids"], payload["userIds"]))
	if gid == "" || role == "" || len(userIDs) == 0 {
		return http.StatusBadRequest, shared.ErrorData("gid, role and user_ids are required"), nil
	}
	result := map[string]any{"succeeded": 0, "failed": 0, "errors": []string{}}
	for _, uid := range userIDs {
		status, data, _ := createMembership(app, r, uid, gid, role, false)
		if status >= 400 {
			result["failed"] = result["failed"].(int) + 1
			result["errors"] = append(result["errors"].([]string), fmt.Sprintf("%s: %s", uid, shared.TextValue(data.(map[string]any), "message")))
			continue
		}
		result["succeeded"] = result["succeeded"].(int) + 1
	}
	return http.StatusOK, result, nil
}

func userGroupsByGroup(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	userID, status, data, ok := requireUser(r)
	if !ok {
		return status, data, nil
	}
	gid := firstNonEmpty(r.URL.Query().Get("g_id"), r.URL.Query().Get("gid"), r.URL.Query().Get("group_id"))
	if gid == "" {
		return http.StatusBadRequest, shared.ErrorData("Missing g_id"), nil
	}
	if !isGroupMember(app, r, userID, gid) && !hasAdminPanel(app, r, userID) {
		return http.StatusForbidden, shared.ErrorData(msgGroupMembership), nil
	}
	return http.StatusOK, formatByGroup(app, r, gid), nil
}

func userGroupsByUser(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	userID, status, data, ok := requireUser(r)
	if !ok {
		return status, data, nil
	}
	uid := firstNonEmpty(r.URL.Query().Get("u_id"), r.URL.Query().Get("uid"), r.URL.Query().Get("user_id"))
	if uid == "" {
		return http.StatusBadRequest, shared.ErrorData("Missing u_id"), nil
	}
	if uid != userID && !hasAdminPanel(app, r, userID) {
		return http.StatusForbidden, shared.ErrorData("user self or admin access required"), nil
	}
	out := []map[string]any{}
	for _, membership := range membershipRows(app, r) {
		if shared.TextValue(membership, "user_id", "userId", "uid", "u_id") == uid {
			out = append(out, decorateMembership(app, r, membership))
		}
	}
	return http.StatusOK, out, nil
}

func groupMembers(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	userID, status, data, ok := requireUser(r)
	if !ok {
		return status, data, nil
	}
	gid := strings.TrimSpace(r.PathValue("group_id"))
	if !isGroupMember(app, r, userID, gid) && !hasAdminPanel(app, r, userID) {
		return http.StatusForbidden, shared.ErrorData(msgGroupMembership), nil
	}
	return http.StatusOK, membersForGroup(app, r, gid), nil
}

func addMembersContext(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	userID, status, data, ok := requireUser(r)
	if !ok {
		return status, data, nil
	}
	gid := strings.TrimSpace(r.PathValue("group_id"))
	if !isGroupAdmin(app, r, userID, gid) {
		return http.StatusForbidden, shared.ErrorData(msgGroupAdminRequired), nil
	}
	group, found := findGroup(app, r, gid)
	if !found {
		return http.StatusNotFound, shared.ErrorData(msgGroupNotFound), nil
	}
	current := membersForGroup(app, r, gid)
	excluded := map[string]bool{}
	for _, member := range current {
		excluded[shared.TextValue(member, "user_id", "userId")] = true
	}
	available := []map[string]any{}
	for _, user := range userRows(app, r) {
		id := userIDFromMap(user)
		if id != "" && !excluded[id] && userVisible(user) {
			available = append(available, user)
		}
	}
	return http.StatusOK, map[string]any{
		"group":           group,
		"current_members": current,
		"available_users": pagedRows(r, available),
	}, nil
}

func resolveAddMembers(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	userID, status, data, ok := requireUser(r)
	if !ok {
		return status, data, nil
	}
	gid := strings.TrimSpace(r.PathValue("group_id"))
	if !isGroupAdmin(app, r, userID, gid) {
		return http.StatusForbidden, shared.ErrorData(msgGroupAdminRequired), nil
	}
	if _, found := findGroup(app, r, gid); !found {
		return http.StatusNotFound, shared.ErrorData(msgGroupNotFound), nil
	}
	payload := platform.DecodeMap(r)
	identifiers := normalizeIdentifiers(shared.StringSlice(payload["identifiers"]))
	already := map[string]bool{}
	for _, member := range membershipRows(app, r) {
		if shared.TextValue(member, "group_id", "groupId", "gid", "g_id") == gid {
			already[shared.TextValue(member, "user_id", "userId", "uid", "u_id")] = true
		}
	}
	users := userLookup(app, r)
	result := map[string]any{"resolved": []map[string]any{}, "unresolved": []string{}, "already_members": []map[string]any{}}
	seenUsers := map[string]bool{}
	for _, identifier := range identifiers {
		user, found := users[strings.ToLower(identifier)]
		if !found || !userVisible(user) {
			result["unresolved"] = append(result["unresolved"].([]string), identifier)
			continue
		}
		id := userIDFromMap(user)
		if seenUsers[id] {
			continue
		}
		seenUsers[id] = true
		if already[id] {
			result["already_members"] = append(result["already_members"].([]map[string]any), user)
			continue
		}
		result["resolved"] = append(result["resolved"].([]map[string]any), user)
	}
	return http.StatusOK, result, nil
}

func createMembership(app *platform.App, r *http.Request, uid, gid, role string, upsert bool) (int, any, *platform.Degraded) {
	actor, status, data, ok := requireUser(r)
	if !ok {
		return status, data, nil
	}
	if status, data, ok := validateMembershipRequest(app, r, actor, uid, gid, role); !ok {
		return status, data, nil
	}
	id := membershipID(uid, gid)
	if existing, found := groupGPURepository(app).FindMembership(r.Context(), uid, gid); found {
		if !upsert {
			return http.StatusBadRequest, shared.ErrorData("user group already exists"), nil
		}
		_, updated, ok := groupGPURepository(app).UpdateMembershipRole(r.Context(), uid, gid, role, time.Now().UTC())
		if !ok {
			return http.StatusInternalServerError, shared.ErrorData("user group update failed"), nil
		}
		publishEvent(app, r, eventFor(r, "GroupMembershipChanged", map[string]any{"old": existing.Data, "new": updated.Data}))
		return http.StatusOK, decorateMembership(app, r, updated.Data), nil
	}
	record, err := groupGPURepository(app).CreateMembership(r.Context(), map[string]any{
		"id":         id,
		"user_id":    uid,
		"group_id":   gid,
		"role":       role,
		"created_at": time.Now().UTC(),
		"updated_at": time.Now().UTC(),
	})
	if err != nil {
		if platform.IsCreateConflict(err) {
			return http.StatusConflict, shared.ErrorData("user group already exists"), nil
		}
		return http.StatusInternalServerError, shared.ErrorData("user group could not be created"), nil
	}
	publishEvent(app, r, eventFor(r, "GroupMembershipChanged", map[string]any{"user_id": uid, "group_id": gid, "role": role, "action": "create"}))
	return http.StatusOK, decorateMembership(app, r, record.Data), nil
}

func validateMembershipRequest(app *platform.App, r *http.Request, actor, uid, gid, role string) (int, any, bool) {
	if uid == "" || gid == "" || role == "" {
		return http.StatusBadRequest, shared.ErrorData("uid, gid and role are required"), false
	}
	if !allowedGroupRoles[role] {
		return http.StatusBadRequest, shared.ErrorData("role must be admin, manager or user"), false
	}
	if role == "admin" && !hasAdminPanel(app, r, actor) {
		return http.StatusForbidden, shared.ErrorData("DB-backed admin panel access required"), false
	}
	if !isGroupAdmin(app, r, actor, gid) {
		return http.StatusForbidden, shared.ErrorData(msgGroupAdminRequired), false
	}
	if _, found := findGroup(app, r, gid); !found {
		return http.StatusNotFound, shared.ErrorData(msgGroupNotFound), false
	}
	user, found := findUser(app, r, uid)
	if !found || !userVisible(user) {
		return http.StatusBadRequest, shared.ErrorData("user not assignable"), false
	}
	return 0, nil, true
}
