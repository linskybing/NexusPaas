package requestnotification

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

func (s *Service) canCreateProjectForm(app *platform.App, r *http.Request, userID, projectID string, admin bool) bool {
	if projectID == "" {
		return true
	}
	if admin {
		return true
	}
	if app == nil || app.Store == nil {
		return false
	}
	syncProjectAccessReadModels(app, r)
	repo := projectAccessRepo(app)
	project, ok := projectAccessProject(repo, r.Context(), projectID)
	if !ok {
		return false
	}
	if valueFrom(project, "personal_user_id", "personalUserID") == userID {
		return true
	}
	ownerID := valueFrom(project, "owner_id", "ownerId", "group_id", "groupId")
	for _, member := range repo.ListProjectMembers(r.Context()) {
		if valueFrom(member, "project_id", "projectId") == projectID && valueFrom(member, "user_id", "userId") == userID {
			return true
		}
	}
	if ownerID == "" {
		return false
	}
	for _, member := range repo.ListUserGroups(r.Context()) {
		if valueFrom(member, "group_id", "groupId") == ownerID && valueFrom(member, "user_id", "userId") == userID && valueFrom(member, "role") != "" {
			return true
		}
	}
	return false
}

func syncProjectAccessReadModels(app *platform.App, r *http.Request) {
	if app == nil || app.Store == nil || app.Events == nil {
		return
	}
	app.RunProjection(r.Context(), projectAccessConsumer, func(event contracts.Event) error {
		return projectProjectAccessEvent(app, r, event)
	})
}

func projectProjectAccessEvent(app *platform.App, r *http.Request, event contracts.Event) error {
	resource, data, deleted, ok := projectAccessProjection(event)
	if !ok {
		return nil
	}
	repo := projectAccessRepo(app)
	if deleted {
		deleteProjectAccessReadModel(repo, r, resource, data)
		return nil
	}
	return upsertProjectAccessReadModel(repo, r, resource, data)
}

func projectAccessProjection(event contracts.Event) (string, map[string]any, bool, bool) {
	name := strings.ToLower(event.Name)
	switch name {
	case "projectcreated", "projectupdated":
		return projectAccessProjects, projectAccessEventData(event), false, true
	case "projectdeleted":
		return projectAccessProjects, projectAccessEventData(event), true, true
	case "project_membercreated", "project_memberupdated":
		return projectAccessMembers, projectAccessEventData(event), false, true
	case "project_memberdeleted":
		return projectAccessMembers, projectAccessEventData(event), true, true
	case "groupmembershipchanged":
		data, deleted := groupMembershipProjectionData(event)
		return projectAccessUserGroups, data, deleted, true
	default:
		return "", nil, false, false
	}
}

func projectAccessEventData(event contracts.Event) map[string]any {
	if next, ok := event.Data["new"].(map[string]any); ok {
		return shared.CloneMap(next)
	}
	return shared.CloneMap(event.Data)
}

func groupMembershipProjectionData(event contracts.Event) (map[string]any, bool) {
	if next, ok := event.Data["new"].(map[string]any); ok {
		return shared.CloneMap(next), false
	}
	data := projectAccessEventData(event)
	action := strings.ToLower(valueFrom(data, "action"))
	return data, action == "delete" || action == "deleted"
}

func upsertProjectAccessReadModel(repo projectAccessRepository, r *http.Request, resource string, data map[string]any) error {
	switch resource {
	case projectAccessProjects:
		return repo.UpsertProject(r.Context(), data)
	case projectAccessMembers:
		return repo.UpsertProjectMember(r.Context(), data)
	case projectAccessUserGroups:
		return repo.UpsertUserGroup(r.Context(), data)
	default:
		return nil
	}
}

func deleteProjectAccessReadModel(repo projectAccessRepository, r *http.Request, resource string, data map[string]any) bool {
	switch resource {
	case projectAccessProjects:
		return repo.DeleteProject(r.Context(), data)
	case projectAccessMembers:
		return repo.DeleteProjectMember(r.Context(), data)
	case projectAccessUserGroups:
		return repo.DeleteUserGroup(r.Context(), data)
	default:
		return false
	}
}

func projectAccessProject(repo projectAccessRepository, ctx context.Context, projectID string) (map[string]any, bool) {
	for _, project := range repo.ListProjects(ctx) {
		if valueFrom(project, "id") == projectID {
			return project, true
		}
	}
	return nil, false
}

func projectAccessRecords(app *platform.App, r *http.Request, localResource, sourceResource string) []map[string]any {
	_ = sourceResource
	repo := projectAccessRepo(app)
	switch localResource {
	case projectAccessProjects:
		return repo.ListProjects(r.Context())
	case projectAccessMembers:
		return repo.ListProjectMembers(r.Context())
	case projectAccessUserGroups:
		return repo.ListUserGroups(r.Context())
	default:
		return nil
	}
}

func validTag(tag string) bool {
	switch tag {
	case "feature", "bug", "question", "resource", "access", "other":
		return true
	default:
		return false
	}
}

func validStatus(status string) bool {
	switch status {
	case "Pending", "Processing", "Completed", "Rejected":
		return true
	default:
		return false
	}
}

func validTransition(current, next string) bool {
	allowed := map[string]map[string]bool{
		"Pending":    {"Processing": true, "Rejected": true},
		"Processing": {"Completed": true, "Rejected": true, "Pending": true},
		"Rejected":   {"Pending": true},
		"Completed":  {},
	}
	return allowed[current][next]
}

func normalizedPriority(priority string) string {
	switch priority {
	case "info", "warning", "critical":
		return priority
	default:
		return "info"
	}
}

func requireUser(r *http.Request) (userContext, int, any, bool) {
	id := currentUserID(r)
	if id == "" {
		return userContext{}, http.StatusUnauthorized, shared.ErrorData("Unauthorized"), false
	}
	admin := isAdmin(r)
	role := 0
	if admin {
		role = 1
	}
	return userContext{ID: id, Username: currentUsername(r), Admin: admin, SystemRole: role}, 0, nil, true
}

func requireAdmin(r *http.Request) (userContext, int, any, bool) {
	user, code, data, ok := requireUser(r)
	if !ok {
		return userContext{}, code, data, false
	}
	if !user.Admin {
		return userContext{}, http.StatusForbidden, shared.ErrorData("admin panel access required"), false
	}
	return user, 0, nil, true
}

func currentUserID(r *http.Request) string {
	return strings.TrimSpace(r.Header.Get("X-User-ID"))
}

func currentUsername(r *http.Request) string {
	if username := strings.TrimSpace(r.Header.Get("X-Username")); username != "" {
		return username
	}
	return currentUserID(r)
}

func isAdmin(r *http.Request) bool {
	role := strings.ToLower(r.Header.Get("X-User-Role"))
	return role == "admin" || role == "super-admin"
}

func systemRole(r *http.Request) int {
	if isAdmin(r) {
		return 1
	}
	return 0
}

func asText(value any) string {
	text, _ := value.(string)
	return strings.TrimSpace(text)
}

func asBool(value any) bool {
	if value == nil {
		return false
	}
	v, ok := value.(bool)
	return ok && v
}

func optionalString(value any) *string {
	text := asText(value)
	if text == "" {
		return nil
	}
	return &text
}

func optionalTime(value any) (*time.Time, bool) {
	text := asText(value)
	if text == "" {
		return nil, true
	}
	parsed, err := time.Parse(time.RFC3339, text)
	if err != nil {
		return nil, false
	}
	return &parsed, true
}

func stringSlice(value any) ([]string, bool) {
	raw, ok := value.([]any)
	if !ok {
		return nil, false
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		text := asText(item)
		if text != "" {
			out = append(out, text)
		}
	}
	return out, true
}

func parsePositiveInt(raw string, fallback int) int {
	parsed, err := strconv.Atoi(raw)
	if err != nil || parsed < 1 {
		return fallback
	}
	return parsed
}

func paginateForms(forms []Form, r *http.Request, maxLimit int) map[string]any {
	page := parsePositiveInt(r.URL.Query().Get("page"), 1)
	limit := parsePositiveInt(r.URL.Query().Get("limit"), 20)
	if limit > maxLimit {
		limit = maxLimit
	}
	total := len(forms)
	offset := (page - 1) * limit
	if offset > total {
		offset = total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return map[string]any{
		"list":      forms[offset:end],
		"total":     int64(total),
		"page":      page,
		"page_size": limit,
		"offset":    offset,
	}
}

func isActive(a Announcement, now time.Time) bool {
	if a.PublishedAt.After(now) {
		return false
	}
	return a.ExpiresAt == nil || a.ExpiresAt.After(now)
}

func valueFrom(data map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := asText(data[key]); value != "" {
			return value
		}
	}
	return ""
}
