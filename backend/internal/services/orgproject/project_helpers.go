package orgproject

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

type projectMemberInput struct {
	UserID string
	Role   string
}

func visibleProjects(app *platform.App, r *http.Request, userID string) []map[string]any {
	rows := projectRows(app, r)
	visible := make([]map[string]any, 0, len(rows))
	for _, project := range rows {
		if canReadProject(projectRoleForUser(app, r, project, userID)) {
			visible = append(visible, project)
		}
	}
	sortRows(visible, "project_name", "id")
	return visible
}

func projectRows(app *platform.App, r *http.Request) []map[string]any {
	records := projectRepository(app).ListProjects(r.Context())
	out := make([]map[string]any, 0, len(records))
	for _, record := range records {
		out = append(out, shared.CloneMap(record.Data))
	}
	return out
}

func findProject(app *platform.App, r *http.Request, id string) (map[string]any, bool) {
	record, found := projectRepository(app).FindProject(r.Context(), id)
	if !found {
		return nil, false
	}
	return shared.CloneMap(record.Data), true
}

func normalizeProjectRecord(data map[string]any, id string) map[string]any {
	project := shared.CloneMap(data)
	id = shared.FirstNonBlank(id, projectID(project))
	name := shared.FirstNonBlank(shared.TextValue(project, "project_name", "ProjectName"), shared.TextValue(project, "name"))
	ownerID := shared.FirstNonBlank(shared.TextValue(project, "owner_id", "ownerId"), shared.TextValue(project, "g_id", "gid", "GID"))
	if id != "" {
		project["id"] = id
		project["p_id"] = id
		project["PID"] = id
		project["project_id"] = id
	}
	if name != "" {
		project["project_name"] = name
		project["ProjectName"] = name
		project["name"] = name
	}
	if ownerID != "" {
		project["owner_id"] = ownerID
		project["GID"] = ownerID
		project["g_id"] = ownerID
	}
	return project
}

func projectRoleForUser(app *platform.App, r *http.Request, project map[string]any, userID string) string {
	if hasAdminPanel(app, r, userID) {
		return "admin"
	}
	projectID := projectID(project)
	if shared.TextValue(project, "personal_user_id", "personalUserID") == userID {
		return "admin"
	}
	if ownerID := shared.TextValue(project, "owner_id", "ownerId", "GID", "g_id"); ownerID != "" {
		if membership, found := findUserGroup(app, r, userID, ownerID); found {
			return normalizeProjectRole(shared.TextValue(membership, "role"))
		}
	}
	if member, found := findProjectMember(app, r, projectID, userID); found {
		return normalizeProjectRole(shared.TextValue(member, "role"))
	}
	return ""
}

func requireProjectRead(app *platform.App, r *http.Request, projectID, userID string) (map[string]any, int, any, bool) {
	project, status, data, ok := requireExistingProject(app, r, projectID)
	if !ok {
		return nil, status, data, false
	}
	if !canReadProject(projectRoleForUser(app, r, project, userID)) {
		return nil, http.StatusForbidden, shared.ErrorData("project member access required"), false
	}
	return project, 0, nil, true
}

func requireProjectManager(app *platform.App, r *http.Request, projectID, userID string) (map[string]any, int, any, bool) {
	project, status, data, ok := requireExistingProject(app, r, projectID)
	if !ok {
		return nil, status, data, false
	}
	if !canManageProject(projectRoleForUser(app, r, project, userID)) {
		return nil, http.StatusForbidden, shared.ErrorData("Project manager access required"), false
	}
	return project, 0, nil, true
}

func requireProjectAdmin(app *platform.App, r *http.Request, projectID, userID string) (map[string]any, int, any, bool) {
	project, status, data, ok := requireExistingProject(app, r, projectID)
	if !ok {
		return nil, status, data, false
	}
	if projectRoleForUser(app, r, project, userID) != "admin" {
		return nil, http.StatusForbidden, shared.ErrorData("Project admin access required"), false
	}
	return project, 0, nil, true
}

func requireExistingProject(app *platform.App, r *http.Request, projectID string) (map[string]any, int, any, bool) {
	project, found := findProject(app, r, projectID)
	if !found {
		return nil, http.StatusNotFound, shared.ErrorData(msgProjectNotFound), false
	}
	return project, 0, nil, true
}

func canReadProject(role string) bool {
	return role == "admin" || role == "manager" || role == "user"
}

func canManageProject(role string) bool {
	return role == "admin" || role == "manager"
}

func mergedProjectMembers(app *platform.App, r *http.Request, projectID string) []map[string]any {
	project, _ := findProject(app, r, projectID)
	out := make([]map[string]any, 0)
	seen := map[string]bool{}
	if ownerID := shared.TextValue(project, "owner_id", "ownerId", "GID", "g_id"); ownerID != "" {
		for _, membership := range membershipRows(app, r) {
			if shared.TextValue(membership, "group_id", "groupId", "gid", "g_id") != ownerID {
				continue
			}
			member := decorateProjectMember(app, r, projectID, membership, "group")
			out = append(out, member)
			seen[shared.TextValue(member, "user_id")] = true
		}
	}
	for _, member := range listMaps(app, r, projectMembersResource) {
		uid := shared.TextValue(member, "user_id", "userId")
		if shared.TextValue(member, "project_id", "projectId") == projectID && !seen[uid] {
			out = append(out, decorateProjectMember(app, r, projectID, member, "direct"))
		}
	}
	sortRows(out, "username", "user_id")
	return out
}

func decorateProjectMember(app *platform.App, r *http.Request, projectID string, member map[string]any, source string) map[string]any {
	out := shared.CloneMap(member)
	uid := shared.TextValue(out, "user_id", "userId", "uid", "u_id")
	out["user_id"] = uid
	out["project_id"] = projectID
	out["role"] = normalizeProjectRole(shared.FirstNonBlank(shared.TextValue(out, "role"), "user"))
	out["source"] = source
	if user, found := findUser(app, r, uid); found {
		out["username"] = shared.TextValue(user, "username", "Username")
		out["full_name"] = shared.TextValue(user, "full_name", "fullName", "FullName")
		out["email"] = shared.TextValue(user, "email", "Email")
	}
	return out
}

func createDirectProjectMember(app *platform.App, r *http.Request, projectID, targetUserID, role, actor string) error {
	targetUserID = strings.TrimSpace(targetUserID)
	role = normalizeProjectRole(role)
	if targetUserID == "" || !allowedProjectRoles[role] {
		return fmt.Errorf("user_id and valid role are required")
	}
	if _, found := findUser(app, r, targetUserID); !found {
		return fmt.Errorf("user not assignable")
	}
	project, _ := findProject(app, r, projectID)
	if ownerID := shared.TextValue(project, "owner_id", "ownerId", "GID", "g_id"); ownerID != "" && isGroupMember(app, r, targetUserID, ownerID) {
		return fmt.Errorf("group member inherits project role from owning group")
	}
	now := time.Now().UTC()
	data := map[string]any{
		"id":         projectMemberID(projectID, targetUserID),
		"project_id": projectID,
		"user_id":    targetUserID,
		"role":       role,
		"added_by":   actor,
		"created_at": now,
		"updated_at": now,
	}
	record, err := projectRepository(app).CreateDirectProjectMember(r.Context(), data)
	if err != nil {
		if platform.IsCreateConflict(err) {
			return fmt.Errorf("project member already exists")
		}
		return fmt.Errorf("project member could not be created")
	}
	publishEvent(app, r, eventFor(r, "project_memberCreated", record.Data))
	return nil
}

func deleteDirectProjectMember(app *platform.App, r *http.Request, projectID, targetUserID string) (int, any) {
	if targetUserID == "" {
		return http.StatusBadRequest, shared.ErrorData("Missing user ID")
	}
	project, _ := findProject(app, r, projectID)
	if ownerID := shared.TextValue(project, "owner_id", "ownerId", "GID", "g_id"); ownerID != "" && isGroupMember(app, r, targetUserID, ownerID) {
		return http.StatusForbidden, shared.ErrorData("Cannot remove group members from project; remove them from the group instead")
	}
	member, found := findProjectMemberRecord(app, r, projectID, targetUserID)
	if !found {
		return http.StatusNotFound, shared.ErrorData("project member not found")
	}
	projectRepository(app).DeleteDirectProjectMemberAndQuota(r.Context(), projectID, targetUserID)
	publishEvent(app, r, eventFor(r, "project_memberDeleted", member.Data))
	return http.StatusOK, nil
}

func updateDirectProjectMemberRole(app *platform.App, r *http.Request, projectID, targetUserID, role string) (int, any) {
	role = normalizeProjectRole(role)
	if targetUserID == "" || !allowedProjectRoles[role] {
		return http.StatusBadRequest, shared.ErrorData("invalid role: must be admin, manager, or user")
	}
	project, _ := findProject(app, r, projectID)
	if ownerID := shared.TextValue(project, "owner_id", "ownerId", "GID", "g_id"); ownerID != "" && isGroupMember(app, r, targetUserID, ownerID) {
		return http.StatusForbidden, shared.ErrorData("Cannot update group members in project; update their group role instead")
	}
	member, found := findProjectMemberRecord(app, r, projectID, targetUserID)
	if !found {
		return http.StatusNotFound, shared.ErrorData("project member not found")
	}
	_, updated, ok := projectRepository(app).UpdateDirectProjectMemberRole(r.Context(), projectID, targetUserID, role, time.Now().UTC())
	if !ok {
		return http.StatusInternalServerError, shared.ErrorData("project member update failed")
	}
	publishEvent(app, r, eventFor(r, "project_memberUpdated", map[string]any{"old": member.Data, "new": updated.Data}))
	return http.StatusOK, updated.Data
}

func findProjectMember(app *platform.App, r *http.Request, projectID, userID string) (map[string]any, bool) {
	record, found := findProjectMemberRecord(app, r, projectID, userID)
	if !found {
		return nil, false
	}
	return shared.CloneMap(record.Data), true
}

func findProjectMemberRecord(app *platform.App, r *http.Request, projectID, userID string) (platformRecord, bool) {
	record, found := projectRepository(app).FindDirectProjectMember(r.Context(), projectID, userID)
	if !found {
		return platformRecord{}, false
	}
	return platformRecord{ID: record.ID, Data: record.Data}, true
}

type platformRecord struct {
	ID   string
	Data map[string]any
}

func upsertQuota(app *platform.App, r *http.Request, quota map[string]any) (int, any, *platform.Degraded) {
	id := shared.TextValue(quota, "id")
	if id == "" || shared.TextValue(quota, "user_id") == "" {
		return http.StatusBadRequest, shared.ErrorData("user_id is required"), nil
	}
	record, err := projectRepository(app).UpsertProjectUserQuota(r.Context(), quota)
	if err != nil {
		return http.StatusInternalServerError, shared.ErrorData("quota update failed"), nil
	}
	publishEvent(app, r, eventFor(r, "UserQuotaUpdated", record.Data))
	return http.StatusOK, quotaDTO(record.Data), nil
}

func quotaRecord(projectID, userID string, payload map[string]any) map[string]any {
	return map[string]any{
		"id":              projectQuotaID(projectID, userID),
		"project_id":      projectID,
		"user_id":         userID,
		"gpu_limit":       shared.NumberValue(payload, "gpu_limit", "GPULimit"),
		"cpu_limit":       shared.NumberValue(payload, "cpu_limit", "CPULimit"),
		"memory_limit_gb": shared.NumberValue(payload, "memory_limit_gb", "MemoryLimitGB"),
		"updated_at":      time.Now().UTC(),
	}
}

func quotaDTO(data map[string]any) map[string]any {
	return map[string]any{
		"gpu_limit":       shared.NumberValue(data, "gpu_limit", "GPULimit"),
		"cpu_limit":       shared.NumberValue(data, "cpu_limit", "CPULimit"),
		"memory_limit_gb": shared.NumberValue(data, "memory_limit_gb", "MemoryLimitGB"),
	}
}

func validateQuotaPayload(payload map[string]any) error {
	for _, key := range []string{"gpu_limit", "cpu_limit", "memory_limit_gb", "GPULimit", "CPULimit", "MemoryLimitGB"} {
		if shared.NumberValue(payload, key) < 0 {
			return fmt.Errorf("quota values must be non-negative")
		}
	}
	return nil
}

func gpuClaimRecord(app *platform.App, r *http.Request, projectID, userID string, payload map[string]any) (map[string]any, error) {
	name := strings.TrimSpace(shared.TextValue(payload, "name"))
	deviceClass := strings.TrimSpace(shared.TextValue(payload, "device_class_name", "deviceClassName"))
	gpuCount := shared.IntValue(payload, "gpu_count", "gpuCount")
	sm := shared.IntValue(payload, "sm_percentage", "smPercentage")
	vram := shared.IntValue(payload, "vram_percentage", "vramPercentage")
	policy := shared.FirstNonBlank(shared.TextValue(payload, "vram_policy", "vramPolicy"), "elastic")
	if name == "" || !dnsLabelLike(name) {
		return nil, fmt.Errorf("name must be a valid DNS label")
	}
	if deviceClass == "" {
		return nil, fmt.Errorf("device_class_name is required")
	}
	if gpuCount < 1 {
		return nil, fmt.Errorf("gpu_count must be at least 1")
	}
	if sm < 1 || sm > 100 {
		return nil, fmt.Errorf("sm_percentage must be between 1 and 100")
	}
	if vram < 1 || vram > 100 {
		return nil, fmt.Errorf("vram_percentage must be between 1 and 100")
	}
	if policy != "elastic" && policy != "hard_cap" {
		return nil, fmt.Errorf("vram_policy must be elastic or hard_cap")
	}
	username := userID
	if user, found := findUser(app, r, userID); found {
		username = shared.FirstNonBlank(shared.TextValue(user, "username"), userID)
	}
	namespace := shared.FirstNonBlank(shared.TextValue(payload, "namespace"), "project-"+safeK8sPart(projectID)+"-"+safeK8sPart(username))
	now := time.Now().UTC()
	return map[string]any{
		"id":                gpuClaimID(projectID, namespace, name),
		"name":              name,
		"namespace":         namespace,
		"project_id":        projectID,
		"user_id":           userID,
		"username":          username,
		"device_class_name": deviceClass,
		"gpu_count":         gpuCount,
		"sm_percentage":     sm,
		"effective_gpu":     float64(gpuCount) * float64(sm) / 100,
		"vram_policy":       policy,
		"vram_percentage":   vram,
		"status":            "created",
		"created_at":        now,
	}, nil
}

func findGPUClaim(app *platform.App, r *http.Request, projectID, name, namespace string) (platformRecord, bool) {
	record, found := groupGPURepository(app).FindGPUClaim(r.Context(), projectID, name, namespace)
	if !found {
		return platformRecord{}, false
	}
	return platformRecord{ID: record.ID, Data: record.Data}, true
}

func applyProjectMutableFields(out, payload map[string]any, create bool) {
	if name := shared.FirstNonBlank(shared.TextValue(payload, "project_name", "ProjectName"), shared.TextValue(payload, "name")); name != "" {
		out["project_name"] = name
		out["ProjectName"] = name
		out["name"] = name
	}
	if description, ok := firstPresentText(payload, "description", "Description"); ok || create {
		out["description"] = description
		out["Description"] = description
	}
	if ownerID := shared.FirstNonBlank(shared.TextValue(payload, "g_id", "gid", "GID"), shared.TextValue(payload, "group_id", "owner_id", "ownerId")); ownerID != "" {
		out["owner_id"] = ownerID
		out["GID"] = ownerID
		out["g_id"] = ownerID
	}
	if planID, ok := firstPresentText(payload, "plan_id", "planId"); ok {
		out["plan_id"] = nullableText(planID)
	}
	if personalUserID, ok := firstPresentText(payload, "personal_user_id", "personalUserID"); ok {
		out["personal_user_id"] = nullableText(personalUserID)
	}
	for _, field := range projectIntFields() {
		if hasAny(payload, field.snake, field.camel) {
			value := shared.IntValue(payload, field.snake, field.camel)
			out[field.snake] = value
			out[field.camel] = value
		} else if create {
			out[field.snake] = 0
			out[field.camel] = 0
		}
	}
	for _, field := range projectBoolFields() {
		if hasAny(payload, field.snake, field.camel) {
			value := shared.BoolValue(payload, field.snake, field.camel)
			out[field.snake] = value
			out[field.camel] = value
		} else if create {
			out[field.snake] = false
			out[field.camel] = false
		}
	}
}

type projectField struct {
	snake string
	camel string
}

func projectIntFields() []projectField {
	return []projectField{
		{"max_concurrent_jobs_per_user", "MaxConcurrentJobsPerUser"},
		{"max_queued_jobs_per_user", "MaxQueuedJobsPerUser"},
		{"max_job_runtime_seconds", "MaxJobRuntimeSeconds"},
		{"max_ide_runtime_seconds", "MaxIDERuntimeSeconds"},
		{"max_project_users", "MaxProjectUsers"},
	}
}

func projectBoolFields() []projectField {
	return []projectField{
		{"allow_image_build", "AllowImageBuild"},
		{"allow_node_port", "AllowNodePort"},
		{"allow_run_as_root", "AllowRunAsRoot"},
		{"external_network_enabled", "ExternalNetworkEnabled"},
	}
}

func projectMemberInputs(payload map[string]any) []projectMemberInput {
	out := make([]projectMemberInput, 0)
	if raw, ok := payload["members"].([]any); ok {
		for _, item := range raw {
			row, ok := item.(map[string]any)
			if !ok {
				continue
			}
			out = append(out, projectMemberInput{
				UserID: shared.TextValue(row, "user_id", "userId", "uid", "id"),
				Role:   normalizeProjectRole(shared.FirstNonBlank(shared.TextValue(row, "role"), "user")),
			})
		}
		return out
	}
	role := normalizeProjectRole(shared.FirstNonBlank(shared.TextValue(payload, "role"), "user"))
	for _, uid := range requestUserIDs(payload) {
		out = append(out, projectMemberInput{UserID: uid, Role: role})
	}
	return out
}

func roleUpdateInputs(payload map[string]any) []projectMemberInput {
	out := make([]projectMemberInput, 0)
	if raw, ok := payload["updates"].([]any); ok {
		for _, item := range raw {
			row, ok := item.(map[string]any)
			if !ok {
				continue
			}
			out = append(out, projectMemberInput{
				UserID: shared.TextValue(row, "user_id", "userId", "uid", "id"),
				Role:   normalizeProjectRole(shared.TextValue(row, "role")),
			})
		}
	}
	return out
}

func quotaUpdateInputs(payload map[string]any) []map[string]any {
	out := make([]map[string]any, 0)
	if raw, ok := payload["updates"].([]any); ok {
		for _, item := range raw {
			if row, ok := item.(map[string]any); ok {
				out = append(out, row)
			}
		}
	}
	return out
}

func requestIDs(payload map[string]any) []string {
	return firstStringSlice(payload, "ids", "project_ids", "projectIds")
}

func requestUserIDs(payload map[string]any) []string {
	return firstStringSlice(payload, "user_ids", "userIds", "ids")
}

func firstStringSlice(payload map[string]any, keys ...string) []string {
	for _, key := range keys {
		if ids := shared.StringSlice(payload[key]); len(ids) > 0 {
			return ids
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

func deleteProjectByID(app *platform.App, r *http.Request, projectID string) {
	projectRepository(app).DeleteProjectCascade(r.Context(), projectID)
}

func workspaceSettings(project map[string]any) map[string]any {
	return map[string]any{"max_ide_runtime_seconds": shared.IntValue(project, "max_ide_runtime_seconds", "MaxIDERuntimeSeconds")}
}

func normalizeProjectRole(role string) string {
	role = normalizeRole(role)
	if role == "member" {
		return "user"
	}
	return role
}

func sortRows(rows []map[string]any, keys ...string) {
	sort.Slice(rows, func(i, j int) bool {
		for _, key := range keys {
			left := shared.TextValue(rows[i], key)
			right := shared.TextValue(rows[j], key)
			if left != right {
				return left < right
			}
		}
		return false
	})
}

func projectPathID(r *http.Request) string {
	return strings.TrimSpace(r.PathValue("id"))
}

func projectUserPathID(r *http.Request) string {
	return strings.TrimSpace(shared.FirstNonBlank(r.PathValue("userId"), r.PathValue("user_id")))
}

func projectID(project map[string]any) string {
	return shared.TextValue(project, "id", "ID", "p_id", "PID", "project_id", "projectId")
}

func projectMemberID(projectID, userID string) string {
	return projectID + ":" + userID
}

func projectQuotaID(projectID, userID string) string {
	return projectID + ":" + userID
}

func gpuClaimID(projectID, namespace, name string) string {
	return projectID + ":" + namespace + ":" + name
}

func newProjectID(app *platform.App) string {
	return projectRepository(app).NextProjectID()
}

func hasAny(data map[string]any, keys ...string) bool {
	for _, key := range keys {
		if _, ok := data[key]; ok {
			return true
		}
	}
	return false
}

func firstPresentText(data map[string]any, keys ...string) (string, bool) {
	for _, key := range keys {
		if value, ok := data[key]; ok {
			return normalizeOptional(value), true
		}
	}
	return "", false
}

func nullableText(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return strings.TrimSpace(value)
}

func dnsLabelLike(value string) bool {
	if len(value) == 0 || len(value) > 63 || value[0] == '-' || value[len(value)-1] == '-' {
		return false
	}
	for _, ch := range value {
		if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '-' {
			continue
		}
		return false
	}
	return true
}

func safeK8sPart(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	lastDash := false
	for _, ch := range value {
		ok := (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9')
		if ok {
			b.WriteRune(ch)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "user"
	}
	return out
}
