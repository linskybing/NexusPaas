package authorizationpolicy

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

type policyAssignmentTarget struct {
	TargetType string
	TargetID   string
}

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
	repo := authorizationPolicyRepo(app)
	var assignment map[string]any
	var created bool
	err = app.WithTx(r.Context(), func(tx platform.StoreTx) error {
		var e error
		assignment, created, e = repo.CreatePolicyAssignmentTx(r.Context(), tx, policyID, targetType, targetID, r.Header.Get(headerUserID))
		if e != nil || !created {
			return e
		}
		tx.Emit(proxyPolicyEvent(r, "assign", assignment))
		return nil
	})
	if err != nil {
		if platform.IsCreateConflict(err) {
			return http.StatusConflict, shared.ErrorData("assignment already exists"), nil
		}
		return http.StatusInternalServerError, shared.ErrorData("assignment could not be created"), nil
	}
	if created {
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
	assignments, err := parseBatchPolicyAssignments(payload)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(err.Error()), nil
	}
	result := map[string]any{"succeeded": 0, "failed": 0, "errors": []string{}}
	repo := authorizationPolicyRepo(app)
	for _, item := range assignments {
		if err := createBatchPolicyAssignment(app, r, repo, policyID, item); err != nil {
			batchFailure(result, err.Error())
			continue
		}
		result["succeeded"] = result["succeeded"].(int) + 1
	}
	return http.StatusOK, result, nil
}

func parseBatchPolicyAssignments(payload map[string]any) ([]policyAssignmentTarget, error) {
	items, ok := payload["assignments"].([]any)
	if !ok || len(items) == 0 || len(items) > 100 {
		return nil, fmt.Errorf("assignments is required and must contain 1 to 100 items")
	}
	assignments := make([]policyAssignmentTarget, 0, len(items))
	for i, item := range items {
		raw, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("assignments[%d] must be an object", i)
		}
		targetType, targetID, err := assignmentTarget(raw)
		if err != nil {
			return nil, fmt.Errorf("assignments[%d]: %s", i, err.Error())
		}
		assignments = append(assignments, policyAssignmentTarget{TargetType: targetType, TargetID: targetID})
	}
	return assignments, nil
}

func createBatchPolicyAssignment(
	app *platform.App,
	r *http.Request,
	repo *recordStoreAuthorizationPolicyRepository,
	policyID string,
	target policyAssignmentTarget,
) error {
	var assignment map[string]any
	var created bool
	return app.WithTx(r.Context(), func(tx platform.StoreTx) error {
		var err error
		assignment, created, err = repo.CreatePolicyAssignmentTx(
			r.Context(),
			tx,
			policyID,
			target.TargetType,
			target.TargetID,
			r.Header.Get(headerUserID),
		)
		if err != nil || !created {
			return err
		}
		tx.Emit(proxyPolicyEvent(r, "assign", assignment))
		return nil
	})
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
	var assignment map[string]any
	var found bool
	if err := app.WithTx(r.Context(), func(tx platform.StoreTx) error {
		var e error
		assignment, found, e = authorizationPolicyRepo(app).UnassignPolicyTx(r.Context(), tx, policyID, targetType, targetID)
		if e != nil || !found {
			return e
		}
		tx.Emit(proxyPolicyEvent(r, "unassign", assignment))
		return nil
	}); err != nil {
		return http.StatusInternalServerError, shared.ErrorData("assignment could not be deleted"), nil
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
