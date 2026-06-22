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
	repo := rawPermissionRepo(app)
	var created bool
	err = app.WithTx(r.Context(), func(tx platform.StoreTx) error {
		var e error
		created, e = repo.CreateRawPermissionPolicyTx(r.Context(), tx, policy)
		if e != nil || !created {
			return e
		}
		tx.Emit(policyChangedEvent(r, "policy_added", map[string]any{"policy": policy}))
		return nil
	})
	if err != nil {
		return http.StatusInternalServerError, shared.ErrorData("policy could not be created"), nil
	}
	if !created {
		return http.StatusConflict, shared.ErrorData(msgPolicyAlreadyExists), nil
	}
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
	repo := rawPermissionRepo(app)
	var result rawPermissionPolicyUpdateResult
	err = app.WithTx(r.Context(), func(tx platform.StoreTx) error {
		var e error
		result, e = repo.UpdateRawPermissionPolicyTx(r.Context(), tx, oldPolicy, newPolicy)
		if e != nil || !result.Updated {
			return e
		}
		tx.Emit(policyChangedEvent(r, "policy_updated", map[string]any{"old": oldPolicy, "new": newPolicy}))
		return nil
	})
	if err != nil {
		return http.StatusInternalServerError, shared.ErrorData("policy could not be updated"), nil
	}
	if !result.Found {
		return http.StatusNotFound, shared.ErrorData(msgPolicyNotFound), nil
	}
	if result.Conflict {
		return http.StatusConflict, shared.ErrorData(msgPolicyAlreadyExists), nil
	}
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
	var deleted bool
	if err := app.WithTx(r.Context(), func(tx platform.StoreTx) error {
		var e error
		deleted, e = rawPermissionRepo(app).DeleteRawPermissionPolicyTx(r.Context(), tx, policy)
		if e != nil || !deleted {
			return e
		}
		tx.Emit(policyChangedEvent(r, "policy_removed", map[string]any{"policy": policy}))
		return nil
	}); err != nil {
		return http.StatusInternalServerError, shared.ErrorData("policy could not be deleted"), nil
	}
	if !deleted {
		return http.StatusNotFound, shared.ErrorData(msgPolicyNotFound), nil
	}
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
		if err := app.WithTx(r.Context(), func(tx platform.StoreTx) error {
			if err := repo.ApplyPermissionOperationTx(r.Context(), tx, op); err != nil {
				return err
			}
			tx.Emit(policyChangedEvent(r, "batch_permissions_processed", map[string]any{"operation": op, "operations": []map[string]string{op}}))
			return nil
		}); err != nil {
			return http.StatusInternalServerError, shared.ErrorData(err.Error()), nil
		}
	}
	return http.StatusOK, nil, nil
}

func enforcePermission(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	serviceAuthorized := app != nil && app.ServiceRequestAuthorized(r)
	if !serviceAuthorized && !isServiceOrAdminPrincipal(r) {
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
