package orgproject

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

func groupRows(app *platform.App, r *http.Request) []map[string]any {
	records := groupGPURepository(app).ListGroups(r.Context())
	out := make([]map[string]any, 0, len(records))
	for _, record := range records {
		out = append(out, shared.CloneMap(record.Data))
	}
	return out
}

func membershipRows(app *platform.App, r *http.Request) []map[string]any {
	records := groupGPURepository(app).ListMemberships(r.Context())
	out := make([]map[string]any, 0, len(records))
	for _, record := range records {
		out = append(out, shared.CloneMap(record.Data))
	}
	return out
}

func userRows(app *platform.App, r *http.Request) []map[string]any {
	return orgIdentityRows(app, r, orgIdentityUsers, usersResource)
}

func listMaps(app *platform.App, r *http.Request, resource string) []map[string]any {
	records := app.Store.List(r.Context(), resource)
	out := make([]map[string]any, 0, len(records))
	for _, record := range records {
		row := shared.CloneMap(record.Data)
		if row["id"] == nil {
			row["id"] = record.ID
		}
		out = append(out, row)
	}
	return out
}

func findGroup(app *platform.App, r *http.Request, id string) (map[string]any, bool) {
	record, found := groupGPURepository(app).FindGroup(r.Context(), id)
	if !found {
		return nil, false
	}
	return shared.CloneMap(record.Data), true
}

func findUser(app *platform.App, r *http.Request, id string) (map[string]any, bool) {
	for _, user := range userRows(app, r) {
		if userIDFromMap(user) == id || strings.EqualFold(shared.TextValue(user, "username", "email"), id) {
			return user, true
		}
	}
	return nil, false
}

func findUserGroup(app *platform.App, r *http.Request, uid, gid string) (map[string]any, bool) {
	record, found := findUserGroupRecord(app, r, uid, gid)
	if !found {
		return nil, false
	}
	return shared.CloneMap(record.Data), true
}

func findUserGroupRecord(app *platform.App, r *http.Request, uid, gid string) (contracts.Record[map[string]any], bool) {
	record, found := groupGPURepository(app).FindMembership(r.Context(), uid, gid)
	if !found {
		return contracts.Record[map[string]any]{}, false
	}
	return contracts.Record[map[string]any]{ID: record.ID, Data: record.Data}, true
}

func isGroupMember(app *platform.App, r *http.Request, uid, gid string) bool {
	_, found := findUserGroup(app, r, uid, gid)
	return found
}

func isGroupAdmin(app *platform.App, r *http.Request, uid, gid string) bool {
	if hasAdminPanel(app, r, uid) {
		return true
	}
	membership, found := findUserGroup(app, r, uid, gid)
	return found && shared.TextValue(membership, "role") == "admin"
}

func hasAdminPanel(app *platform.App, r *http.Request, userID string) bool {
	roles := orgIdentityRows(app, r, orgIdentityRoles, rolesResource)
	for _, user := range orgIdentityRows(app, r, orgIdentityUsers, usersResource) {
		if orgIdentityID(orgIdentityUsers, user) != userID && shared.TextValue(user, "user_id", "userId", "UserID") != userID {
			continue
		}
		if recordGrantsAdminPanel(user) {
			return true
		}
		roleID := shared.TextValue(user, "role_id", "roleId", "RoleID", "role", "Role")
		for _, role := range roles {
			if (orgIdentityID(orgIdentityRoles, role) == roleID || shared.TextValue(role, "name", "Name") == roleID) && recordGrantsAdminPanel(role) {
				return true
			}
		}
		return false
	}
	return false
}

func recordGrantsAdminPanel(data map[string]any) bool {
	if shared.BoolValue(data, "admin_panel", "adminPanel", "AdminPanel") {
		return true
	}
	return shared.BoolValue(shared.MapValue(data, "capabilities", "Capabilities"), "admin_panel", "adminPanel", "AdminPanel")
}

func membersForGroup(app *platform.App, r *http.Request, gid string) []map[string]any {
	out := []map[string]any{}
	for _, membership := range membershipRows(app, r) {
		if shared.TextValue(membership, "group_id", "groupId", "gid", "g_id") == gid {
			out = append(out, decorateMembership(app, r, membership))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return shared.TextValue(out[i], "username") < shared.TextValue(out[j], "username")
	})
	return out
}

func decorateMembership(app *platform.App, r *http.Request, membership map[string]any) map[string]any {
	out := shared.CloneMap(membership)
	uid := shared.TextValue(out, "user_id", "userId", "uid", "u_id")
	gid := shared.TextValue(out, "group_id", "groupId", "gid", "g_id")
	if user, found := findUser(app, r, uid); found {
		out["username"] = shared.TextValue(user, "username", "Username")
		out["email"] = shared.TextValue(user, "email", "Email")
	}
	if group, found := findGroup(app, r, gid); found {
		out["group_name"] = shared.TextValue(group, "group_name", "groupName", "name")
	}
	out["user_id"] = uid
	out["group_id"] = gid
	return out
}

func formatByGroup(app *platform.App, r *http.Request, gid string) map[string]map[string]any {
	groupName := ""
	if group, found := findGroup(app, r, gid); found {
		groupName = shared.TextValue(group, "group_name", "groupName", "name")
	}
	return map[string]map[string]any{
		gid: {
			"group_id":   gid,
			"group_name": groupName,
			"users":      membersForGroup(app, r, gid),
		},
	}
}

func userLookup(app *platform.App, r *http.Request) map[string]map[string]any {
	out := map[string]map[string]any{}
	for _, user := range userRows(app, r) {
		for _, key := range []string{userIDFromMap(user), shared.TextValue(user, "username"), shared.TextValue(user, "email")} {
			key = strings.ToLower(strings.TrimSpace(key))
			if key != "" {
				out[key] = user
			}
		}
	}
	return out
}

func validateGroupPolicies(app *platform.App, payload map[string]any) error {
	if err := validateOption("storage class", optionalText(payload, "storage_class", "storageClass"), app.Config.GroupStorageClassOptions); err != nil {
		return err
	}
	if err := validateOption("registry profile", optionalText(payload, "registry_profile", "registryProfile"), app.Config.GroupRegistryProfileOptions); err != nil {
		return err
	}
	return nil
}

func validateOption(label, value string, allowed []string) error {
	value = strings.TrimSpace(value)
	if value == "" || len(allowed) == 0 {
		return nil
	}
	for _, option := range allowed {
		if strings.EqualFold(option, value) {
			return nil
		}
	}
	return fmt.Errorf("invalid group policy option: %s %s is not configured", label, value)
}

func optionRows(options []string) []map[string]any {
	rows := make([]map[string]any, 0, len(options))
	for _, option := range options {
		rows = append(rows, map[string]any{"name": option, "value": option})
	}
	return rows
}

func pagedRows(r *http.Request, rows []map[string]any) map[string]any {
	page := positiveInt(r.URL.Query().Get("page"), 1)
	limit := positiveInt(shared.FirstNonBlank(r.URL.Query().Get("limit"), r.URL.Query().Get("page_size")), 20)
	start := (page - 1) * limit
	if start > len(rows) {
		start = len(rows)
	}
	end := start + limit
	if end > len(rows) {
		end = len(rows)
	}
	return map[string]any{"list": rows[start:end], "total": len(rows), "page": page, "page_size": limit}
}

func requireUser(r *http.Request) (string, int, any, bool) {
	userID := strings.TrimSpace(r.Header.Get("X-User-ID"))
	if userID == "" {
		return "", http.StatusUnauthorized, shared.ErrorData("unauthorized"), false
	}
	return userID, 0, nil, true
}

func userVisible(user map[string]any) bool {
	switch strings.ToLower(shared.TextValue(user, "status", "Status")) {
	case "disabled", "deleted", "delete":
		return false
	default:
		return true
	}
}

func userIDFromMap(user map[string]any) string {
	return shared.TextValue(user, "id", "ID", "user_id", "userId", "UID", "uid")
}

func groupID(group map[string]any) string {
	return shared.TextValue(group, "id", "ID", "gid", "g_id", "group_id", "groupId")
}

func membershipID(uid, gid string) string {
	return uid + ":" + gid
}

func normalizeRole(role string) string {
	return strings.ToLower(strings.TrimSpace(role))
}

func normalizeIdentifiers(input []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, raw := range input {
		item := strings.TrimSpace(raw)
		key := strings.ToLower(item)
		if item == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, item)
	}
	return out
}

func normalizedHostPaths(payload map[string]any) []any {
	if value, ok := payload["allowed_host_paths"].([]any); ok {
		return value
	}
	if value, ok := payload["allowedHostPaths"].([]any); ok {
		return value
	}
	return []any{}
}

func newGroupID(app *platform.App, _ *http.Request) string {
	return groupGPURepository(app).NextGroupID()
}

func eventFor(r *http.Request, name string, data map[string]any) contracts.Event {
	return contracts.Event{
		EventID:        platform.NewUUID(),
		Name:           name,
		Source:         "org-project-service",
		OccurredAt:     time.Now().UTC(),
		TraceID:        shared.FirstNonBlank(r.Header.Get("X-Trace-ID"), r.Header.Get("X-Request-ID"), platform.NewUUID()),
		SchemaVersion:  1,
		IdempotencyKey: r.Header.Get("Idempotency-Key"),
		Data:           data,
	}
}

func positiveInt(raw string, fallback int) int {
	var n int
	for _, ch := range raw {
		if ch < '0' || ch > '9' {
			return fallback
		}
		n = n*10 + int(ch-'0')
	}
	if n <= 0 {
		return fallback
	}
	return n
}

func firstNonNil(values ...any) any {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func optionalText(data map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := data[key]; ok {
			return normalizeOptional(value)
		}
	}
	return ""
}

func normalizeOptional(value any) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}
