package clusterread

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

const (
	serviceName                   = "usage-observability-service"
	clusterProjectionConsumer     = serviceName + ":cluster_projection"
	clusterIdentityRolesResource  = serviceName + ":cluster_identity_roles"
	clusterIdentityUsersResource  = serviceName + ":cluster_identity_users"
	clusterPolicyRoleAssignments  = serviceName + ":cluster_policy_role_assignments"
	clusterPolicyRolesResource    = serviceName + ":cluster_policy_roles"
	clusterProjectMembersResource = serviceName + ":cluster_project_members"
	clusterProjectsResource       = serviceName + ":cluster_projects"
	clusterReadModelResource      = serviceName + ":cluster_read_models"
	clusterUserGroupsResource     = serviceName + ":cluster_user_groups"
	authorizationRolesResource    = "authorization-policy-service:roles"
	identityRolesResource         = "identity-service:roles"
	identityUsersResource         = "identity-service:users"
	orgProjectMembersResource     = "org-project-service:project_members"
	orgProjectsResource           = "org-project-service:projects"
	orgUserGroupsResource         = "org-project-service:user_groups"
	keyAction                     = "action"
	keyCapabilities               = "capabilities"
	keyCapabilitiesTitle          = "Capabilities"
	keyDeleted                    = "deleted"
	keyGroupID                    = "group_id"
	keyGroupIDCamel               = "groupId"
	keyGroupIDTitle               = "GroupID"
	keyID                         = "id"
	keyIDTitle                    = "ID"
	keyName                       = "name"
	keyNameTitle                  = "Name"
	keyProjectID                  = "project_id"
	keyProjectIDCamel             = "projectId"
	keyProjectIDTitle             = "ProjectID"
	keyRole                       = "role"
	keyRoleTitle                  = "Role"
	keyRoleID                     = "role_id"
	keyRoleIDCamel                = "roleId"
	keyRoleIDTitle                = "RoleID"
	keyUserID                     = "user_id"
	keyUserIDCamel                = "userId"
	keyUserIDTitle                = "UserID"
)

func Register(app *platform.App) {
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/cluster/summary", getClusterSummary)
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/cluster/nodes", listClusterNodes)
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/cluster/nodes/{name}", getClusterNode)
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/cluster/gpu-usage", listPodGPUUsage)
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/projects/gpu-usage/by-user", getProjectsGPUUsageByUser)
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/projects/{id}/gpu-usage", getProjectGPUUsage)
	registerClusterResourceCollector(app)
}

func getClusterSummary(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	if currentUserID(r) == "" {
		return http.StatusUnauthorized, map[string]any{"message": "unauthorized"}, nil
	}
	return http.StatusOK, publicSummary(clusterSummary(app, r)), nil
}

func listClusterNodes(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	if status, data, ok := requireAdmin(app, r); !ok {
		return status, data, nil
	}
	return http.StatusOK, nodeList(clusterSummary(app, r)), nil
}

func getClusterNode(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	if status, data, ok := requireAdmin(app, r); !ok {
		return status, data, nil
	}
	name := strings.TrimSpace(r.PathValue("name"))
	if name == "" {
		return http.StatusBadRequest, map[string]any{"message": "node name required"}, nil
	}
	for _, node := range nodeList(clusterSummary(app, r)) {
		if textValue(node, "name", "Name") == name {
			return http.StatusOK, node, nil
		}
	}
	return http.StatusNotFound, map[string]any{"message": "node not found"}, nil
}

func listPodGPUUsage(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	if status, data, ok := requireAdmin(app, r); !ok {
		return status, data, nil
	}
	return http.StatusOK, podGPUUsages(clusterSummary(app, r)), nil
}

func getProjectsGPUUsageByUser(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	userID := currentUserID(r)
	if userID == "" {
		return http.StatusBadRequest, map[string]any{"message": "Invalid user ID"}, nil
	}
	projectIDs := visibleProjectIDs(app, r, userID)
	out := map[string]int64{}
	for projectID := range projectIDs {
		out[projectID] = 0
	}
	for _, usage := range podGPUUsages(clusterSummary(app, r)) {
		projectID := strings.TrimPrefix(textValue(usage, "namespace", "Namespace"), "project-")
		if projectID == "" || projectID == textValue(usage, "namespace", "Namespace") {
			continue
		}
		if _, ok := out[projectID]; ok {
			out[projectID]++
		}
	}
	return http.StatusOK, out, nil
}

func getProjectGPUUsage(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	userID := currentUserID(r)
	if userID == "" {
		return http.StatusUnauthorized, map[string]any{"message": "unauthorized"}, nil
	}
	projectID := strings.TrimSpace(r.PathValue("id"))
	if projectID == "" {
		return http.StatusBadRequest, map[string]any{"message": "Invalid project ID"}, nil
	}
	if !projectExists(app, r, projectID) {
		return http.StatusNotFound, map[string]any{"message": "Project not found"}, nil
	}
	if !hasAdminPanel(app, r, userID) && !isProjectMember(app, r, userID, projectID) {
		return http.StatusForbidden, map[string]any{"message": "project access required"}, nil
	}
	var used int64
	namespace := "project-" + projectID
	for _, usage := range podGPUUsages(clusterSummary(app, r)) {
		if textValue(usage, "namespace", "Namespace") == namespace {
			used++
		}
	}
	return http.StatusOK, map[string]any{"used": used}, nil
}

func requireAdmin(app *platform.App, r *http.Request) (int, any, bool) {
	userID := currentUserID(r)
	if userID == "" {
		return http.StatusUnauthorized, map[string]any{"message": "unauthorized"}, false
	}
	if !hasAdminPanel(app, r, userID) {
		return http.StatusForbidden, map[string]any{"message": "admin access required"}, false
	}
	return 0, nil, true
}

func clusterSummary(app *platform.App, r *http.Request) map[string]any {
	if app == nil || app.Store == nil {
		return emptySummary()
	}
	records := app.Store.List(r.Context(), clusterReadModelResource)
	if len(records) == 0 {
		return emptySummary()
	}
	record := records[len(records)-1]
	if summary := mapValue(record.Data, "summary", "Summary"); len(summary) > 0 {
		return shared.CloneMap(summary)
	}
	return shared.CloneMap(record.Data)
}

func emptySummary() map[string]any {
	return map[string]any{
		"nodeCount":                   0,
		"totalCpuAllocatableMilli":    0,
		"totalCpuUsedMilli":           0,
		"totalMemoryAllocatableBytes": 0,
		"totalMemoryUsedBytes":        0,
		"totalGpuAllocatable":         0,
		"totalGpuUsed":                0,
		"nodes":                       []any{},
		"podGpuUsages":                []any{},
		"deviceClasses":               []any{},
		"collectedAt":                 time.Now().UTC(),
	}
}

func publicSummary(summary map[string]any) map[string]any {
	out := shared.CloneMap(summary)
	delete(out, "nodes")
	delete(out, "Nodes")
	delete(out, "podGpuUsages")
	delete(out, "pod_gpu_usages")
	delete(out, "PodGPUUsages")
	return out
}

func nodeList(summary map[string]any) []map[string]any {
	nodes := anySlice(summary, "nodes", "Nodes")
	out := make([]map[string]any, 0, len(nodes))
	for _, raw := range nodes {
		if node, ok := raw.(map[string]any); ok {
			out = append(out, shared.CloneMap(node))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return textValue(out[i], "name", "Name") < textValue(out[j], "name", "Name")
	})
	return out
}

func podGPUUsages(summary map[string]any) []map[string]any {
	usages := anySlice(summary, "podGpuUsages", "pod_gpu_usages", "PodGPUUsages")
	out := make([]map[string]any, 0, len(usages))
	for _, raw := range usages {
		if usage, ok := raw.(map[string]any); ok {
			out = append(out, shared.CloneMap(usage))
		}
	}
	return out
}

func visibleProjectIDs(app *platform.App, r *http.Request, userID string) map[string]struct{} {
	ids := map[string]struct{}{}
	groupIDs := userGroupIDs(app, r, userID)
	projects := clusterRecords(app, r, clusterProjectsResource, orgProjectsResource)
	for _, project := range projects {
		addDirectlyVisibleProject(ids, project, groupIDs, userID)
	}
	for _, project := range projects {
		addDescendantVisibleProject(ids, project)
	}
	for _, member := range clusterRecords(app, r, clusterProjectMembersResource, orgProjectMembersResource) {
		addMemberVisibleProject(ids, member, userID)
	}
	return ids
}

func addDirectlyVisibleProject(ids map[string]struct{}, project map[string]any, groupIDs map[string]struct{}, userID string) {
	projectID := readModelID(clusterProjectsResource, project)
	if projectID == "" {
		return
	}
	if textValue(project, "personal_user_id", "personalUserID", "PersonalUserID") == userID || projectOwnedByGroup(project, groupIDs) {
		ids[projectID] = struct{}{}
	}
}

func addDescendantVisibleProject(ids map[string]struct{}, project map[string]any) {
	projectID := readModelID(clusterProjectsResource, project)
	if projectID == "" {
		return
	}
	if _, ok := ids[projectID]; ok {
		return
	}
	if projectDescendsFromVisibleProject(project, ids) {
		ids[projectID] = struct{}{}
	}
}

func addMemberVisibleProject(ids map[string]struct{}, member map[string]any, userID string) {
	if textValue(member, keyUserID, keyUserIDCamel, keyUserIDTitle) != userID {
		return
	}
	projectID := textValue(member, keyProjectID, keyProjectIDCamel, keyProjectIDTitle)
	if projectID != "" {
		ids[projectID] = struct{}{}
	}
}

func userGroupIDs(app *platform.App, r *http.Request, userID string) map[string]struct{} {
	ids := map[string]struct{}{}
	for _, member := range clusterRecords(app, r, clusterUserGroupsResource, orgUserGroupsResource) {
		if textValue(member, keyUserID, keyUserIDCamel, keyUserIDTitle) != userID {
			continue
		}
		groupID := textValue(member, keyGroupID, keyGroupIDCamel, keyGroupIDTitle, keyID, keyIDTitle)
		if groupID != "" {
			ids[groupID] = struct{}{}
		}
	}
	return ids
}

func projectOwnedByGroup(project map[string]any, groupIDs map[string]struct{}) bool {
	for _, key := range []string{"owner_id", "ownerId", "OwnerID", "GID", "group_id", "groupId", "GroupID"} {
		if _, ok := groupIDs[textValue(project, key)]; ok {
			return true
		}
	}
	return false
}

func projectDescendsFromVisibleProject(project map[string]any, visible map[string]struct{}) bool {
	parentID := textValue(project, "parent_id", "parentId", "ParentID")
	if _, ok := visible[parentID]; ok {
		return true
	}
	path := textValue(project, "path", "Path")
	if path == "" {
		return false
	}
	for projectID := range visible {
		if strings.HasPrefix(path, projectID+".") || strings.Contains(path, "."+projectID+".") {
			return true
		}
	}
	return false
}

func projectExists(app *platform.App, r *http.Request, projectID string) bool {
	for _, project := range clusterRecords(app, r, clusterProjectsResource, orgProjectsResource) {
		if readModelID(clusterProjectsResource, project) == projectID {
			return true
		}
	}
	return false
}

func isProjectMember(app *platform.App, r *http.Request, userID, projectID string) bool {
	if visible, ok := visibleProjectIDs(app, r, userID)[projectID]; ok {
		_ = visible
		return true
	}
	return false
}

func hasAdminPanel(app *platform.App, r *http.Request, userID string) bool {
	if app == nil || app.Store == nil {
		return false
	}
	syncClusterReadModels(app, r)
	identityRoles := clusterRecords(app, r, clusterIdentityRolesResource, identityRolesResource)
	policyRoles := clusterRecords(app, r, clusterPolicyRolesResource, authorizationRolesResource)
	allRoles := append([]map[string]any{}, identityRoles...)
	allRoles = append(allRoles, policyRoles...)
	if found, allowed := projectedUserAdminAccess(userID, clusterRecords(app, r, clusterIdentityUsersResource, identityUsersResource), allRoles); found {
		return allowed
	}
	return directRoleGrant(userID, allRoles) || policyRoleAssignmentGrant(userID, clusterRecords(app, r, clusterPolicyRoleAssignments, ""), policyRoles)
}

func projectedUserAdminAccess(userID string, users, roles []map[string]any) (bool, bool) {
	for _, user := range users {
		if readModelID(clusterIdentityUsersResource, user) != userID && textValue(user, keyUserID, keyUserIDCamel, keyUserIDTitle) != userID {
			continue
		}
		if recordGrantsAdminPanel(user) {
			return true, true
		}
		roleID := textValue(user, keyRoleID, keyRoleIDCamel, keyRoleIDTitle, keyRole, keyRoleTitle)
		return true, roleSetGrantsAdmin(roleID, roles)
	}
	return false, false
}

func directRoleGrant(userID string, roles []map[string]any) bool {
	for _, role := range roles {
		if textValue(role, keyUserID, keyUserIDCamel, keyUserIDTitle) == userID && recordGrantsAdminPanel(role) {
			return true
		}
	}
	return false
}

func policyRoleAssignmentGrant(userID string, assignments, roles []map[string]any) bool {
	for _, assignment := range assignments {
		if textValue(assignment, keyUserID, keyUserIDCamel, keyUserIDTitle) != userID {
			continue
		}
		roleID := textValue(assignment, keyRoleID, keyRoleIDCamel, keyRoleIDTitle)
		if roleSetGrantsAdmin(roleID, roles) {
			return true
		}
	}
	return false
}

func roleSetGrantsAdmin(roleID string, roles []map[string]any) bool {
	for _, role := range roles {
		if (readModelID(clusterPolicyRolesResource, role) == roleID || textValue(role, keyName, keyNameTitle) == roleID) && recordGrantsAdminPanel(role) {
			return true
		}
	}
	return false
}

func syncClusterReadModels(app *platform.App, r *http.Request) {
	if app == nil || app.Store == nil || app.Events == nil {
		return
	}
	app.RunProjection(r.Context(), clusterProjectionConsumer, func(event contracts.Event) error {
		return projectClusterReadEvent(app, r, event)
	})
}

func projectClusterReadEvent(app *platform.App, r *http.Request, event contracts.Event) error {
	resource, data, deleted, ok := clusterProjection(event)
	if !ok {
		return nil
	}
	if deleted {
		deleteClusterReadModel(app, r, resource, data)
		return nil
	}
	return upsertClusterReadModel(app, r, resource, data)
}

func clusterProjection(event contracts.Event) (string, map[string]any, bool, bool) {
	name := strings.ToLower(event.Name)
	switch name {
	case "usercreated", "userupdated", "userdisabled":
		return clusterIdentityUsersResource, clusterEventData(event), false, true
	case "userdeleted":
		return clusterIdentityUsersResource, clusterEventData(event), true, true
	case "rolecreated", "roleupdated":
		return clusterIdentityRolesResource, clusterEventData(event), false, true
	case "roledeleted":
		return clusterIdentityRolesResource, clusterEventData(event), true, true
	case "projectcreated", "projectupdated":
		return clusterProjectsResource, clusterEventData(event), false, true
	case "projectdeleted":
		return clusterProjectsResource, clusterEventData(event), true, true
	case "project_membercreated", "project_memberupdated":
		return clusterProjectMembersResource, clusterEventData(event), false, true
	case "project_memberdeleted":
		return clusterProjectMembersResource, clusterEventData(event), true, true
	case "groupmembershipchanged":
		data, deleted := groupMembershipProjectionData(event)
		return clusterUserGroupsResource, data, deleted, true
	case "proxypolicychanged":
		return proxyPolicyProjection(event)
	default:
		return "", nil, false, false
	}
}

func proxyPolicyProjection(event contracts.Event) (string, map[string]any, bool, bool) {
	data := clusterEventData(event)
	switch strings.ToLower(textValue(data, keyAction)) {
	case "role_create", "role_update":
		return clusterPolicyRolesResource, data, false, true
	case "role_delete":
		return clusterPolicyRolesResource, data, true, true
	case "role_user_assign":
		return clusterPolicyRoleAssignments, data, false, true
	case "role_user_unassign":
		return clusterPolicyRoleAssignments, data, true, true
	default:
		return "", nil, false, false
	}
}

func clusterEventData(event contracts.Event) map[string]any {
	if next, ok := event.Data["new"].(map[string]any); ok {
		return shared.CloneMap(next)
	}
	return shared.CloneMap(event.Data)
}

func groupMembershipProjectionData(event contracts.Event) (map[string]any, bool) {
	if next, ok := event.Data["new"].(map[string]any); ok {
		return shared.CloneMap(next), false
	}
	data := clusterEventData(event)
	action := strings.ToLower(textValue(data, keyAction))
	return data, action == "delete" || action == keyDeleted
}

func upsertClusterReadModel(app *platform.App, r *http.Request, resource string, data map[string]any) error {
	id := readModelID(resource, data)
	if id == "" {
		return nil
	}
	data[keyID] = id
	if _, ok := app.Store.Update(r.Context(), resource, id, data); ok {
		return nil
	}
	if _, err := app.Store.Create(r.Context(), resource, data); err != nil {
		if platform.IsCreateConflict(err) {
			if _, ok := app.Store.Update(r.Context(), resource, id, data); !ok {
				return fmt.Errorf("cluster projection conflict update missed for %s/%s", resource, id)
			}
			return nil
		}
		return fmt.Errorf("cluster projection create failed for %s/%s: %w", resource, id, err)
	}
	return nil
}

func deleteClusterReadModel(app *platform.App, r *http.Request, resource string, data map[string]any) {
	if deleted, ok := data[keyDeleted].(bool); ok && !deleted {
		return
	}
	if id := readModelID(resource, data); id != "" {
		app.Store.Delete(r.Context(), resource, id)
	}
}

func readModelID(resource string, data map[string]any) string {
	id := textValue(data, keyID, keyIDTitle)
	userID := textValue(data, keyUserID, keyUserIDCamel, keyUserIDTitle)
	projectID := textValue(data, keyProjectID, keyProjectIDCamel, keyProjectIDTitle, "p_id", "pID", "PID")
	groupID := textValue(data, keyGroupID, keyGroupIDCamel, keyGroupIDTitle)
	roleID := textValue(data, keyRoleID, keyRoleIDCamel, keyRoleIDTitle)
	name := textValue(data, keyName, keyNameTitle)
	switch resource {
	case clusterProjectMembersResource:
		if id == "" && projectID != "" && userID != "" {
			return projectID + ":" + userID
		}
	case clusterUserGroupsResource:
		if id == "" && userID != "" && groupID != "" {
			return userID + ":" + groupID
		}
	case clusterPolicyRoleAssignments:
		if id == "" && roleID != "" && userID != "" {
			return roleID + ":" + userID
		}
	case clusterProjectsResource:
		return shared.FirstNonBlank(id, projectID)
	case clusterIdentityRolesResource, clusterPolicyRolesResource:
		return shared.FirstNonBlank(id, roleID, name, userID)
	case clusterIdentityUsersResource:
		return shared.FirstNonBlank(id, userID)
	}
	return shared.FirstNonBlank(id, projectID, userID, groupID, roleID, name)
}

func clusterRecords(app *platform.App, r *http.Request, localResource, sourceResource string) []map[string]any {
	syncClusterReadModels(app, r)
	local := recordMaps(app, r, localResource)
	if sourceResource == "" || !sourceCoHosted(app, sourceResource) {
		return local
	}
	source := recordMaps(app, r, sourceResource)
	if len(local) == 0 {
		return source
	}
	return mergeClusterRecords(localResource, source, local)
}

func mergeClusterRecords(resource string, source, local []map[string]any) []map[string]any {
	out := make([]map[string]any, 0, len(source)+len(local))
	seen := map[string]bool{}
	for _, record := range local {
		if id := readModelID(resource, record); id != "" {
			seen[id] = true
		}
		out = append(out, record)
	}
	for _, record := range source {
		id := readModelID(resource, record)
		if id != "" && seen[id] {
			continue
		}
		out = append(out, record)
	}
	return out
}

func recordMaps(app *platform.App, r *http.Request, resource string) []map[string]any {
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

func sourceCoHosted(app *platform.App, sourceResource string) bool {
	if app == nil {
		return false
	}
	owner, _, ok := strings.Cut(sourceResource, ":")
	return ok && app.Config.AllowsService(owner)
}

func recordGrantsAdminPanel(data map[string]any) bool {
	if boolValue(data, "admin_panel", "adminPanel", "AdminPanel") {
		return true
	}
	capabilities := mapValue(data, "capabilities", "Capabilities")
	if boolValue(capabilities, "admin_panel", "adminPanel", "AdminPanel") {
		return true
	}
	return false
}

func currentUserID(r *http.Request) string {
	return strings.TrimSpace(r.Header.Get("X-User-ID"))
}

func textValue(data map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := data[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func boolValue(data map[string]any, keys ...string) bool {
	for _, key := range keys {
		switch value := data[key].(type) {
		case bool:
			return value
		case string:
			return strings.EqualFold(value, "true")
		}
	}
	return false
}

func mapValue(data map[string]any, keys ...string) map[string]any {
	for _, key := range keys {
		if value, ok := data[key].(map[string]any); ok {
			return value
		}
		if raw, ok := data[key].(string); ok && strings.TrimSpace(raw) != "" {
			var decoded map[string]any
			if json.Unmarshal([]byte(raw), &decoded) == nil {
				return decoded
			}
		}
	}
	return nil
}

func anySlice(data map[string]any, keys ...string) []any {
	for _, key := range keys {
		if value, ok := data[key].([]any); ok {
			return value
		}
	}
	return nil
}
