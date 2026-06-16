package authorizationpolicy

import (
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

func listRoles(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	if status, data, ok := requireAdmin(app, r); !ok {
		return status, data, nil
	}
	return http.StatusOK, roleRows(app, r), nil
}

func listPlatformRolesLegacy(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	return listRoles(app, r, platform.RouteSpec{})
}

func getRole(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	if status, data, ok := requireAdmin(app, r); !ok {
		return status, data, nil
	}
	role, found := findPlatformRole(app, r, strings.TrimSpace(r.PathValue("id")))
	if !found {
		return http.StatusNotFound, shared.ErrorData(msgRoleNotFound), nil
	}
	return http.StatusOK, role, nil
}

func createRole(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	if status, data, ok := requireAdmin(app, r); !ok {
		return status, data, nil
	}
	payload, _, err := decodePayload(r)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(err.Error()), nil
	}
	name := shared.TextValue(payload, "name")
	displayName := shared.TextValue(payload, "display_name", "displayName")
	if len(name) < 2 || len(name) > 100 {
		return http.StatusBadRequest, shared.ErrorData("name is required and must be between 2 and 100 characters"), nil
	}
	if displayName == "" {
		return http.StatusBadRequest, shared.ErrorData("display_name is required"), nil
	}
	if roleNameExists(app, r, "", name) {
		return http.StatusBadRequest, shared.ErrorData("role name already exists"), nil
	}
	now := time.Now().UTC()
	role, err := authorizationPolicyRepo(app).CreateProxyRole(r.Context(), map[string]any{
		"id":           shared.FirstNonEmpty(shared.TextValue(payload, "id"), authorizationPolicyRepo(app).NextProxyRoleID(r.Context())),
		"name":         name,
		"display_name": displayName,
		"description":  shared.TextValue(payload, "description"),
		"is_system":    false,
		"created_at":   now,
		"updated_at":   now,
	})
	if err != nil {
		if platform.IsCreateConflict(err) {
			return http.StatusConflict, shared.ErrorData("role already exists"), nil
		}
		return http.StatusInternalServerError, shared.ErrorData("role could not be created"), nil
	}
	publishProxyPolicyChanged(app, r, "role_create", role)
	return http.StatusCreated, role, nil
}

func updateRole(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	if status, data, ok := requireAdmin(app, r); !ok {
		return status, data, nil
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if _, found := findPlatformRole(app, r, id); !found {
		return http.StatusNotFound, shared.ErrorData(msgRoleNotFound), nil
	}
	payload, present, err := decodePayload(r)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(err.Error()), nil
	}
	update := map[string]any{"updated_at": time.Now().UTC()}
	if _, ok := present["display_name"]; ok {
		if displayName := shared.TextValue(payload, "display_name", "displayName"); displayName != "" {
			update["display_name"] = displayName
		}
	}
	if _, ok := present["description"]; ok {
		update["description"] = shared.TextValue(payload, "description")
	}
	role, ok := authorizationPolicyRepo(app).UpdateProxyRole(r.Context(), id, update)
	if !ok {
		return http.StatusNotFound, shared.ErrorData(msgRoleNotFound), nil
	}
	publishProxyPolicyChanged(app, r, "role_update", role)
	return http.StatusOK, role, nil
}

func deleteRole(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	if status, data, ok := requireAdmin(app, r); !ok {
		return status, data, nil
	}
	id := strings.TrimSpace(r.PathValue("id"))
	current, _ := authorizationPolicyRepo(app).DeleteProxyRoleCascade(r.Context(), id)
	publishProxyPolicyChanged(app, r, "role_delete", current)
	return http.StatusOK, nil, nil
}

func listRoleUsers(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	if status, data, ok := requireAdmin(app, r); !ok {
		return status, data, nil
	}
	roleID := strings.TrimSpace(r.PathValue("id"))
	if _, found := findPlatformRole(app, r, roleID); !found {
		return http.StatusNotFound, shared.ErrorData(msgRoleNotFound), nil
	}
	return http.StatusOK, authorizationPolicyRepo(app).ListRoleUsers(r.Context(), roleID), nil
}

func assignRoleUser(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	if status, data, ok := requireAdmin(app, r); !ok {
		return status, data, nil
	}
	roleID := strings.TrimSpace(r.PathValue("id"))
	if _, found := findPlatformRole(app, r, roleID); !found {
		return http.StatusNotFound, shared.ErrorData(msgRoleNotFound), nil
	}
	payload, _, err := decodePayload(r)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(err.Error()), nil
	}
	userID := shared.TextValue(payload, "user_id", "userId")
	if userID == "" {
		return http.StatusBadRequest, shared.ErrorData(msgUserIDRequired), nil
	}
	member, created, err := createRoleUser(app, r, roleID, userID, r.Header.Get(headerUserID))
	if err != nil {
		if platform.IsCreateConflict(err) {
			return http.StatusConflict, shared.ErrorData("role user already exists"), nil
		}
		return http.StatusInternalServerError, shared.ErrorData("role user could not be created"), nil
	}
	if created {
		publishProxyPolicyChanged(app, r, "role_user_assign", member)
		return http.StatusCreated, member, nil
	}
	return http.StatusOK, member, nil
}

func assignRoleUserLegacy(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	if status, data, ok := requireAdmin(app, r); !ok {
		return status, data, nil
	}
	payload, _, err := decodePayload(r)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(err.Error()), nil
	}
	roleID := shared.TextValue(payload, "role_id", "roleId")
	if roleID == "" {
		return http.StatusBadRequest, shared.ErrorData("role_id is required"), nil
	}
	if _, found := findPlatformRole(app, r, roleID); !found {
		return http.StatusNotFound, shared.ErrorData(msgRoleNotFound), nil
	}
	userID := shared.TextValue(payload, "user_id", "userId")
	if userID == "" {
		return http.StatusBadRequest, shared.ErrorData(msgUserIDRequired), nil
	}
	member, created, err := createRoleUser(app, r, roleID, userID, r.Header.Get(headerUserID))
	if err != nil {
		if platform.IsCreateConflict(err) {
			return http.StatusConflict, shared.ErrorData("role user already exists"), nil
		}
		return http.StatusInternalServerError, shared.ErrorData("role user could not be created"), nil
	}
	if created {
		publishProxyPolicyChanged(app, r, "role_user_assign", member)
		return http.StatusCreated, member, nil
	}
	return http.StatusOK, member, nil
}

func batchAssignRoleUsers(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	if status, data, ok := requireAdmin(app, r); !ok {
		return status, data, nil
	}
	roleID := strings.TrimSpace(r.PathValue("id"))
	if _, found := findPlatformRole(app, r, roleID); !found {
		return http.StatusNotFound, shared.ErrorData(msgRoleNotFound), nil
	}
	payload, _, err := decodePayload(r)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(err.Error()), nil
	}
	userIDs := shared.StringSlice(payload["user_ids"])
	if len(userIDs) == 0 || len(userIDs) > 100 {
		return http.StatusBadRequest, shared.ErrorData("user_ids is required and must contain 1 to 100 items"), nil
	}
	result := map[string]any{"succeeded": 0, "failed": 0, "errors": []string{}}
	for _, userID := range userIDs {
		if strings.TrimSpace(userID) == "" {
			batchFailure(result, msgUserIDRequired)
			continue
		}
		member, created, err := createRoleUser(app, r, roleID, userID, r.Header.Get(headerUserID))
		if err != nil {
			batchFailure(result, err.Error())
			continue
		}
		if created {
			publishProxyPolicyChanged(app, r, "role_user_assign", member)
		}
		result["succeeded"] = result["succeeded"].(int) + 1
	}
	return http.StatusOK, result, nil
}

func unassignRoleUser(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	if status, data, ok := requireAdmin(app, r); !ok {
		return status, data, nil
	}
	roleID := strings.TrimSpace(r.PathValue("id"))
	userID := strings.TrimSpace(r.PathValue("user_id"))
	if member, found := authorizationPolicyRepo(app).UnassignRoleUser(r.Context(), roleID, userID); found {
		publishProxyPolicyChanged(app, r, "role_user_unassign", member)
	}
	return http.StatusOK, nil, nil
}

func listSystemRoles(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	if status, data, ok := requireAdmin(app, r); !ok {
		return status, data, nil
	}
	rows := policyIdentityRecords(app, r, policyIdentityRoles, rolesResource)
	sort.Slice(rows, func(i, j int) bool {
		return shared.TextValue(rows[i], "name", "Name") < shared.TextValue(rows[j], "name", "Name")
	})
	return http.StatusOK, rows, nil
}
