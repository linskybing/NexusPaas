package authorizationpolicy

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

func listPolicyAssignments(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	if status, data, ok := requireAdmin(app, r); !ok {
		return status, data, nil
	}
	policyID := strings.TrimSpace(r.PathValue("id"))
	ensureDefaultAssignments(app, r)
	if _, found := findPolicy(app, r, policyID); !found {
		return http.StatusNotFound, shared.ErrorData(msgPolicyNotFound), nil
	}
	return http.StatusOK, assignmentRowsForPolicy(app, r, policyID), nil
}

func assignPolicy(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	if status, data, ok := requireAdmin(app, r); !ok {
		return status, data, nil
	}
	policyID := strings.TrimSpace(r.PathValue("id"))
	ensureDefaultAssignments(app, r)
	if _, found := findPolicy(app, r, policyID); !found {
		return http.StatusNotFound, shared.ErrorData(msgPolicyNotFound), nil
	}
	payload, _, err := decodePayload(r)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(err.Error()), nil
	}
	targetType, targetID, err := assignmentTarget(payload)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(err.Error()), nil
	}
	assignment, created, err := createPolicyAssignment(app, r, policyID, targetType, targetID, r.Header.Get(headerUserID))
	if err != nil {
		if platform.IsCreateConflict(err) {
			return http.StatusConflict, shared.ErrorData("assignment already exists"), nil
		}
		return http.StatusInternalServerError, shared.ErrorData("assignment could not be created"), nil
	}
	if created {
		publishProxyPolicyChanged(app, r, "assign", assignment)
		return http.StatusCreated, assignment, nil
	}
	return http.StatusOK, assignment, nil
}

func batchAssignPolicy(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	if status, data, ok := requireAdmin(app, r); !ok {
		return status, data, nil
	}
	policyID := strings.TrimSpace(r.PathValue("id"))
	ensureDefaultAssignments(app, r)
	if _, found := findPolicy(app, r, policyID); !found {
		return http.StatusNotFound, shared.ErrorData(msgPolicyNotFound), nil
	}
	payload, _, err := decodePayload(r)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(err.Error()), nil
	}
	items, ok := payload["assignments"].([]any)
	if !ok || len(items) == 0 || len(items) > 100 {
		return http.StatusBadRequest, shared.ErrorData("assignments is required and must contain 1 to 100 items"), nil
	}
	assignments := make([]map[string]string, 0, len(items))
	for i, item := range items {
		raw, ok := item.(map[string]any)
		if !ok {
			return http.StatusBadRequest, shared.ErrorData(fmt.Sprintf("assignments[%d] must be an object", i)), nil
		}
		targetType, targetID, err := assignmentTarget(raw)
		if err != nil {
			return http.StatusBadRequest, shared.ErrorData(fmt.Sprintf("assignments[%d]: %s", i, err.Error())), nil
		}
		assignments = append(assignments, map[string]string{"target_type": targetType, "target_id": targetID})
	}
	result := map[string]any{"succeeded": 0, "failed": 0, "errors": []string{}}
	for _, item := range assignments {
		targetType := item["target_type"]
		targetID := item["target_id"]
		assignment, created, err := createPolicyAssignment(app, r, policyID, targetType, targetID, r.Header.Get(headerUserID))
		if err != nil {
			batchFailure(result, err.Error())
			continue
		}
		if created {
			publishProxyPolicyChanged(app, r, "assign", assignment)
		}
		result["succeeded"] = result["succeeded"].(int) + 1
	}
	return http.StatusOK, result, nil
}

func unassignPolicy(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	if status, data, ok := requireAdmin(app, r); !ok {
		return status, data, nil
	}
	policyID := strings.TrimSpace(r.PathValue("id"))
	ensureDefaultAssignments(app, r)
	payload, _, err := decodePayload(r)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(err.Error()), nil
	}
	targetType, targetID, err := assignmentTarget(payload)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(err.Error()), nil
	}
	if assignment, found := authorizationPolicyRepo(app).UnassignPolicy(r.Context(), policyID, targetType, targetID); found {
		publishProxyPolicyChanged(app, r, "unassign", assignment)
	}
	return http.StatusOK, nil, nil
}

func listTargetAssignments(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	if status, data, ok := requireAdmin(app, r); !ok {
		return status, data, nil
	}
	targetType := strings.TrimSpace(r.PathValue("type"))
	targetID := strings.TrimSpace(r.PathValue("id"))
	if !validTargetType(targetType) || targetID == "" {
		return http.StatusBadRequest, shared.ErrorData("target type must be role or user and target id is required"), nil
	}
	ensureDefaultAssignments(app, r)
	return http.StatusOK, authorizationPolicyRepo(app).ListTargetAssignments(r.Context(), targetType, targetID), nil
}
