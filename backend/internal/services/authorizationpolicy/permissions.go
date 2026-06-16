package authorizationpolicy

import (
	"net/http"
	"strings"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

func listRawPermissionPolicies(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	if status, data, ok := requireAdmin(app, r); !ok {
		return status, data, nil
	}
	return http.StatusOK, rawPermissionRepo(app).ListRawPermissionPolicies(r.Context()), nil
}

func addRawPermissionPolicy(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	policy, err := decodeRawPolicy(r)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(err.Error()), nil
	}
	if len(policy) < 4 {
		return http.StatusBadRequest, shared.ErrorData("policy must have at least 4 elements [sub, dom, obj, act]"), nil
	}
	if status, data, ok := requireAdmin(app, r); !ok {
		return status, data, nil
	}
	created, err := rawPermissionRepo(app).CreateRawPermissionPolicy(r.Context(), policy)
	if err != nil {
		return http.StatusInternalServerError, shared.ErrorData("policy could not be created"), nil
	}
	if !created {
		return http.StatusConflict, shared.ErrorData(msgPolicyAlreadyExists), nil
	}
	publishPolicyChanged(app, r, "policy_added", map[string]any{"policy": policy})
	return http.StatusOK, nil, nil
}

func updateRawPermissionPolicy(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	oldPolicy, newPolicy, err := decodeRawPolicyUpdate(r)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(err.Error()), nil
	}
	if status, data, ok := requireAdmin(app, r); !ok {
		return status, data, nil
	}
	result, err := rawPermissionRepo(app).UpdateRawPermissionPolicy(r.Context(), oldPolicy, newPolicy)
	if err != nil {
		return http.StatusInternalServerError, shared.ErrorData("policy could not be updated"), nil
	}
	if !result.Found {
		return http.StatusNotFound, shared.ErrorData(msgPolicyNotFound), nil
	}
	if result.Conflict {
		return http.StatusConflict, shared.ErrorData(msgPolicyAlreadyExists), nil
	}
	publishPolicyChanged(app, r, "policy_updated", map[string]any{"old": oldPolicy, "new": newPolicy})
	return http.StatusOK, nil, nil
}

func removeRawPermissionPolicy(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	policy, err := decodeRawPolicy(r)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(err.Error()), nil
	}
	if status, data, ok := requireAdmin(app, r); !ok {
		return status, data, nil
	}
	if !rawPermissionRepo(app).DeleteRawPermissionPolicy(r.Context(), policy) {
		return http.StatusNotFound, shared.ErrorData(msgPolicyNotFound), nil
	}
	publishPolicyChanged(app, r, "policy_removed", map[string]any{"policy": policy})
	return http.StatusOK, nil, nil
}

func batchProcessPermissions(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	operations, err := decodePermissionOperations(r)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(err.Error()), nil
	}
	if status, data, ok := requireAdmin(app, r); !ok {
		return status, data, nil
	}
	repo := rawPermissionRepo(app)
	for _, op := range operations {
		if err := repo.ApplyPermissionOperation(r.Context(), op); err != nil {
			return http.StatusInternalServerError, shared.ErrorData(err.Error()), nil
		}
	}
	publishPolicyChanged(app, r, "batch_permissions_processed", map[string]any{"operations": operations})
	return http.StatusOK, nil, nil
}

func enforcePermission(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	if !isServiceOrAdminPrincipal(r) {
		return http.StatusForbidden, shared.ErrorData("service principal is required"), nil
	}
	payload, _, err := decodePayload(r)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(msgInvalidEnforceRequest), nil
	}
	policy := permissionTuple(payload)
	if policy[0] == "" || policy[2] == "" || policy[3] == "" {
		return http.StatusBadRequest, shared.ErrorData(msgInvalidEnforceRequest), nil
	}
	decision, err := RawPolicyPDP{Policies: rawPermissionRepo(app)}.Enforce(r.Context(), policy[0], policy[1], policy[2], policy[3])
	if err != nil {
		return http.StatusInternalServerError, shared.ErrorData("policy decision failed"), nil
	}
	return http.StatusOK, decision, nil
}

func simulatePermissionEnforce(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	payload, _, err := decodePayload(r)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData("invalid simulate request: requires sub, dom, obj, act"), nil
	}
	policy := permissionTuple(payload)
	for _, value := range policy {
		if value == "" {
			return http.StatusBadRequest, shared.ErrorData("invalid simulate request: requires sub, dom, obj, act"), nil
		}
	}
	if status, data, ok := requireAdmin(app, r); !ok {
		return status, data, nil
	}
	allowed, err := rawPermissionRepo(app).RawPermissionAllowed(r.Context(), policy[0], policy[1], policy[2], policy[3])
	if err != nil {
		return http.StatusInternalServerError, shared.ErrorData("policy decision failed"), nil
	}
	return http.StatusOK, map[string]bool{"allowed": allowed}, nil
}

func permissionTuple(payload map[string]any) []string {
	return []string{
		shared.TextValue(payload, "sub"),
		shared.TextValue(payload, "dom"),
		shared.TextValue(payload, "obj"),
		shared.TextValue(payload, "act"),
	}
}

func isServiceOrAdminPrincipal(r *http.Request) bool {
	switch strings.ToLower(strings.TrimSpace(r.Header.Get("X-User-Role"))) {
	case "service", "admin", "superadmin", "root":
		return strings.TrimSpace(r.Header.Get(headerUserID)) != ""
	default:
		return false
	}
}
