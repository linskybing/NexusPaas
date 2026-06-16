package identity

import (
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

func listUsers(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	if status, data, ok := requireAdmin(app, r); !ok {
		return status, data, nil
	}
	return http.StatusOK, publicUsers(principalRepository(app).ListUsers(r.Context())), nil
}

func listUsersPaging(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	if status, data, ok := requireAdmin(app, r); !ok {
		return status, data, nil
	}
	users := publicUsers(principalRepository(app).ListUsers(r.Context()))
	total := len(users)
	page := positiveQueryInt(r, "page", 1)
	limit := positiveQueryInt(r, "limit", 20)
	start := (page - 1) * limit
	if start > total {
		start = total
	}
	end := start + limit
	if end > total {
		end = total
	}
	return http.StatusOK, map[string]any{"list": users[start:end], "total": total, "page": page, "limit": limit}, nil
}

func resolveUsers(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	if status, data, ok := requireAdmin(app, r); !ok {
		return status, data, nil
	}
	payload, err := decodePayload(r)
	if err != nil {
		return http.StatusBadRequest, map[string]any{"message": msgInvalidInput}, nil
	}
	resolved := []map[string]any{}
	unresolved := []string{}
	for _, identifier := range firstStringSlice(payload, "identifiers", "ids") {
		if user, ok := findUserByIdentifier(app, r, identifier); ok {
			resolved = append(resolved, publicUser(user.Data))
			continue
		}
		unresolved = append(unresolved, identifier)
	}
	return http.StatusOK, map[string]any{"resolved": resolved, "unresolved": unresolved}, nil
}

func batchCreateUsers(app *platform.App, r *http.Request, route platform.RouteSpec) (int, any, *platform.Degraded) {
	if status, data, ok := requireAdmin(app, r); !ok {
		return status, data, nil
	}
	payload, err := decodePayload(r)
	if err != nil {
		return http.StatusBadRequest, map[string]any{"message": msgInvalidInput}, nil
	}
	result := batchResult()
	for _, item := range firstMapSlice(payload, "users", "items") {
		req := cloneJSONRequest(r, item)
		status, data, _ := register(app, req, route)
		addBatchResult(result, status < 400, data)
	}
	return http.StatusOK, result, nil
}

func batchDeleteUsers(app *platform.App, r *http.Request, route platform.RouteSpec) (int, any, *platform.Degraded) {
	if status, data, ok := requireAdmin(app, r); !ok {
		return status, data, nil
	}
	ids, err := decodeIDList(r)
	if err != nil {
		return http.StatusBadRequest, map[string]any{"message": msgInvalidInput}, nil
	}
	result := batchResult()
	for _, id := range ids {
		req := r.Clone(r.Context())
		req.SetPathValue("id", id)
		status, data, _ := deleteUser(app, req, route)
		addBatchResult(result, status < 400, data)
	}
	return http.StatusOK, result, nil
}

func batchResetPassword(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	if status, data, ok := requireAdmin(app, r); !ok {
		return status, data, nil
	}
	payload, err := decodePayload(r)
	if err != nil {
		return http.StatusBadRequest, map[string]any{"message": msgInvalidInput}, nil
	}
	password := shared.TextValue(payload, "password", "new_password")
	if len(password) < 6 {
		return http.StatusBadRequest, map[string]any{"message": "password must be at least 6 characters"}, nil
	}
	result := batchResult()
	for _, id := range firstUserIDs(payload) {
		ok := resetUserPasswordWithLDAP(app, r, id, password)
		if ok {
			if updated, found := principalRepository(app).GetUser(r.Context(), id); found {
				publish(app, r, "UserUpdated", publicUser(updated.Data))
			}
		}
		addBatchResult(result, ok, map[string]any{"id": id})
	}
	return http.StatusOK, result, nil
}

func batchUpdateRole(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	if status, data, ok := requireAdmin(app, r); !ok {
		return status, data, nil
	}
	payload, err := decodePayload(r)
	if err != nil {
		return http.StatusBadRequest, map[string]any{"message": msgInvalidInput}, nil
	}
	role := shared.FirstNonEmpty(textValue(payload, "role"), roleName(intValue(payload, "system_role", 2)))
	systemRole := systemRoleFor(role, intValue(payload, "system_role", 2))
	result := batchResult()
	for _, id := range firstUserIDs(payload) {
		updated, ok := updateUserRoleWithLDAP(app, r, id, map[string]any{"role": role, "system_role": systemRole, "role_id": shared.FirstNonEmpty(textValue(payload, "role_id"), defaultRoleID)})
		if ok {
			publish(app, r, "UserUpdated", publicUser(updated))
			addBatchResult(result, true, publicUser(updated))
			continue
		}
		addBatchResult(result, false, map[string]any{"id": id})
	}
	return http.StatusOK, result, nil
}

func getUserByID(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	if status, data, ok := requireSelfOrAdmin(app, r, pathValue(r, "id")); !ok {
		return status, data, nil
	}
	user, found := principalRepository(app).GetUser(r.Context(), pathValue(r, "id"))
	if !found {
		return http.StatusNotFound, map[string]any{"message": msgUserNotFound}, nil
	}
	return http.StatusOK, publicUser(user.Data), nil
}

func updateUser(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	targetID := pathValue(r, "id")
	actor, status, data, ok := actorSelfOrAdmin(app, r, targetID)
	if !ok {
		return status, data, nil
	}
	payload, err := decodePayload(r)
	if err != nil {
		return http.StatusBadRequest, map[string]any{"message": msgInvalidInput}, nil
	}
	update := userUpdate(payload, isAdminUser(actor.Data))
	if len(update) == 0 {
		return http.StatusBadRequest, map[string]any{"message": "no updatable fields"}, nil
	}
	updated, statusCode, errData := updateUserWithLDAP(app, r, targetID, payload, update)
	if statusCode != http.StatusOK {
		return statusCode, errData, nil
	}
	publish(app, r, "UserUpdated", publicUser(updated.Data))
	return http.StatusOK, publicUser(updated.Data), nil
}

func deleteUser(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	if status, data, ok := requireAdmin(app, r); !ok {
		return status, data, nil
	}
	id := pathValue(r, "id")
	statusCode, errData := deleteUserWithLDAP(app, r, id)
	if statusCode != http.StatusOK {
		return statusCode, errData, nil
	}
	revokeUserCredentials(app, r, id)
	publish(app, r, "UserDisabled", map[string]any{"id": id, "deleted": true})
	return http.StatusOK, map[string]any{"id": id, "deleted": true}, nil
}

func getUserSettings(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	if status, data, ok := requireSelfOrAdmin(app, r, pathValue(r, "id")); !ok {
		return status, data, nil
	}
	settings, found := principalRepository(app).GetUserSettings(r.Context(), pathValue(r, "id"))
	if !found {
		return http.StatusNotFound, map[string]any{"message": msgUserNotFound}, nil
	}
	return http.StatusOK, settings, nil
}

func updateUserSettings(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	if status, data, ok := requireSelfOrAdmin(app, r, pathValue(r, "id")); !ok {
		return status, data, nil
	}
	payload, err := decodePayload(r)
	if err != nil {
		return http.StatusBadRequest, map[string]any{"message": msgInvalidInput}, nil
	}
	settings := shared.MapValue(payload, "settings")
	if len(settings) == 0 {
		settings = payload
	}
	updated, found := principalRepository(app).UpdateUserSettings(r.Context(), pathValue(r, "id"), settings, time.Now().UTC())
	if !found {
		return http.StatusNotFound, map[string]any{"message": msgUserNotFound}, nil
	}
	return http.StatusOK, updated, nil
}

func currentUser(app *platform.App, r *http.Request) (contracts.Record[map[string]any], bool) {
	userID := strings.TrimSpace(r.Header.Get(headerUserID))
	if userID == "" {
		return contracts.Record[map[string]any]{}, false
	}
	user, ok := authRepository(app).FindActiveUserByID(r.Context(), userID)
	if !ok {
		return contracts.Record[map[string]any]{}, false
	}
	return user.Record(), true
}

func findUserByUsername(app *platform.App, r *http.Request, username string) (contracts.Record[map[string]any], bool) {
	return principalRepository(app).FindUserByUsername(r.Context(), username)
}

func activeUser(user map[string]any) bool {
	status := strings.ToLower(textValue(user, "status"))
	return status == "" || status == "online" || status == "offline"
}

func publicUser(user map[string]any) map[string]any {
	systemRole := intValue(user, "system_role", 2)
	role := shared.FirstNonEmpty(textValue(user, "role"), roleName(systemRole))
	return map[string]any{
		"id":          textValue(user, "id"),
		"username":    textValue(user, "username"),
		"name":        shared.FirstNonEmpty(textValue(user, "name"), textValue(user, "full_name"), textValue(user, "username")),
		"full_name":   textValue(user, "full_name"),
		"email":       textValue(user, "email"),
		"role":        role,
		"role_id":     shared.FirstNonEmpty(textValue(user, "role_id"), defaultRoleID),
		"system_role": systemRole,
		"permissions": permissionsForRole(role, systemRole),
	}
}

func publicUsers(records []contracts.Record[map[string]any]) []map[string]any {
	users := make([]map[string]any, 0, len(records))
	for _, record := range records {
		users = append(users, publicUser(record.Data))
	}
	sort.SliceStable(users, func(i, j int) bool {
		return shared.FirstNonEmpty(textValue(users[i], "username"), textValue(users[i], "id")) <
			shared.FirstNonEmpty(textValue(users[j], "username"), textValue(users[j], "id"))
	})
	return users
}

func requireAdmin(app *platform.App, r *http.Request) (int, map[string]any, bool) {
	user, ok := currentUser(app, r)
	if !ok {
		return http.StatusUnauthorized, map[string]any{"message": msgAuthenticationRequired}, false
	}
	if !isAdminUser(user.Data) {
		return http.StatusForbidden, map[string]any{"message": msgAdminOnly}, false
	}
	return http.StatusOK, nil, true
}

func requireSelfOrAdmin(app *platform.App, r *http.Request, targetID string) (int, map[string]any, bool) {
	_, status, data, ok := actorSelfOrAdmin(app, r, targetID)
	return status, data, ok
}

func actorSelfOrAdmin(app *platform.App, r *http.Request, targetID string) (contracts.Record[map[string]any], int, map[string]any, bool) {
	user, ok := currentUser(app, r)
	if !ok {
		if platformAuthenticated(r, app) {
			return contracts.Record[map[string]any]{Data: map[string]any{"id": strings.TrimSpace(r.Header.Get(headerUserID))}}, http.StatusOK, nil, true
		}
		return contracts.Record[map[string]any]{}, http.StatusUnauthorized, map[string]any{"message": msgAuthenticationRequired}, false
	}
	if textValue(user.Data, "id") != targetID && !isAdminUser(user.Data) {
		return contracts.Record[map[string]any]{}, http.StatusForbidden, map[string]any{"message": "user self or admin access required"}, false
	}
	return user, http.StatusOK, nil, true
}

func platformAuthenticated(r *http.Request, app *platform.App) bool {
	return app.Config.RequireAuth && strings.TrimSpace(r.Header.Get(headerUserID)) != ""
}

func isAdminUser(user map[string]any) bool {
	if intValue(user, "system_role", 2) == 0 || strings.EqualFold(textValue(user, "role"), "admin") {
		return true
	}
	return shared.BoolValue(shared.MapValue(user, "capabilities", "Capabilities"), "admin_panel", "adminPanel", "AdminPanel")
}

func findUserByIdentifier(app *platform.App, r *http.Request, identifier string) (contracts.Record[map[string]any], bool) {
	identifier = strings.TrimSpace(identifier)
	if identifier == "" {
		return contracts.Record[map[string]any]{}, false
	}
	return principalRepository(app).FindUserByIdentifier(r.Context(), identifier)
}

func userUpdate(payload map[string]any, admin bool) map[string]any {
	update := map[string]any{}
	for _, key := range []string{"username", "email", "name", "full_name"} {
		if value, ok := payload[key]; ok {
			update[key] = value
		}
	}
	if password := textValue(payload, "password"); password != "" {
		update["password_hash"] = platform.HashSecret(password)
	}
	if settings := shared.MapValue(payload, "settings"); len(settings) > 0 {
		update["settings"] = settings
	}
	if admin {
		applyAdminUserUpdate(update, payload)
	}
	if len(update) > 0 {
		update["updated_at"] = time.Now().UTC().Format(time.RFC3339)
	}
	return update
}

func applyAdminUserUpdate(update, payload map[string]any) {
	for _, key := range []string{"status", "role_id", "capabilities"} {
		if value, ok := payload[key]; ok {
			update[key] = value
		}
	}
	if role := textValue(payload, "role"); role != "" {
		update["role"] = role
		update["system_role"] = systemRoleFor(role, intValue(payload, "system_role", 2))
	}
	if _, ok := payload["system_role"]; ok {
		systemRole := intValue(payload, "system_role", 2)
		update["system_role"] = systemRole
		update["role"] = shared.FirstNonEmpty(textValue(payload, "role"), roleName(systemRole))
	}
}

func systemRoleFor(role string, fallback int) int {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "admin":
		return 0
	case "manager":
		return 1
	case "user":
		return 2
	default:
		return fallback
	}
}

func revokeUserCredentials(app *platform.App, r *http.Request, userID string) {
	for _, token := range authRepository(app).RevokeAPITokensForUser(r.Context(), userID, time.Now().UTC()) {
		revokeCredential(app, r.Context(), "api_token", token.ID, token.ExpiresAt)
	}
}

func firstUserIDs(payload map[string]any) []string {
	if ids := firstStringSlice(payload, "ids", "user_ids", "userIds"); len(ids) > 0 {
		return ids
	}
	if id := shared.TextValue(payload, "id", "user_id", "userId"); id != "" {
		return []string{id}
	}
	return nil
}

func decodeIDList(r *http.Request) ([]string, error) {
	payload, err := decodePayload(r)
	if err != nil {
		return nil, err
	}
	return firstUserIDs(payload), nil
}

func permissionsForRole(role string, systemRole int) []string {
	if systemRole == 0 || strings.EqualFold(role, "admin") {
		return []string{"adminPanel"}
	}
	if systemRole == 1 || strings.EqualFold(role, "manager") {
		return []string{"projectManage"}
	}
	return []string{"selfManage"}
}

func roleName(systemRole int) string {
	switch systemRole {
	case 0:
		return "admin"
	case 1:
		return "manager"
	default:
		return "user"
	}
}
