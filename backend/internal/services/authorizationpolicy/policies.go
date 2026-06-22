package authorizationpolicy

import (
	"net/http"
	"strings"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

func listPolicies(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	if status, data, ok := requireAdmin(app, r); !ok {
		return status, data, nil
	}
	return http.StatusOK, policyRows(app, r), nil
}

func getPolicy(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	if status, data, ok := requireAdmin(app, r); !ok {
		return status, data, nil
	}
	policy, found := findPolicy(app, r, strings.TrimSpace(r.PathValue("id")))
	if !found {
		return http.StatusNotFound, shared.ErrorData(msgPolicyNotFound), nil
	}
	return http.StatusOK, policy, nil
}

func createPolicy(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	if status, data, ok := requireAdmin(app, r); !ok {
		return status, data, nil
	}
	payload, _, err := decodePayload(r)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(err.Error()), nil
	}
	name := shared.TextValue(payload, "name")
	if len(name) < 2 || len(name) > 100 {
		return http.StatusBadRequest, shared.ErrorData("name is required and must be between 2 and 100 characters"), nil
	}
	if policyNameExists(app, r, "", name) {
		return http.StatusBadRequest, shared.ErrorData("policy name already exists"), nil
	}
	rules, err := parseRuleInputs(payload["rules"])
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(err.Error()), nil
	}
	id := shared.FirstNonEmpty(shared.TextValue(payload, "id"), authorizationPolicyRepo(app).NextProxyPolicyID(r.Context()))
	now := time.Now().UTC()
	var policy map[string]any
	err = app.WithTx(r.Context(), func(tx platform.StoreTx) error {
		created, e := authorizationPolicyRepo(app).CreateProxyPolicyTx(r.Context(), tx, map[string]any{
			"id":          id,
			"name":        name,
			"description": shared.TextValue(payload, "description"),
			"is_system":   false,
			"created_at":  now,
			"updated_at":  now,
		}, rules)
		if e != nil {
			return e
		}
		policy = created
		tx.Emit(proxyPolicyEvent(r, "create", created))
		return nil
	})
	if err != nil {
		if platform.IsCreateConflict(err) {
			if strings.Contains(err.Error(), rulesResource) {
				return http.StatusConflict, shared.ErrorData(msgPolicyRuleExists), nil
			}
			return http.StatusConflict, shared.ErrorData(msgPolicyAlreadyExists), nil
		}
		return http.StatusInternalServerError, shared.ErrorData("policy could not be created"), nil
	}
	return http.StatusCreated, policy, nil
}

func updatePolicy(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	if status, data, ok := requireAdmin(app, r); !ok {
		return status, data, nil
	}
	id := strings.TrimSpace(r.PathValue("id"))
	current, found := findPolicy(app, r, id)
	if !found {
		return http.StatusNotFound, shared.ErrorData(msgPolicyNotFound), nil
	}
	payload, present, err := decodePayload(r)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(err.Error()), nil
	}
	update, status, data, ok := policyUpdateFields(app, r, id, payload, present)
	if !ok {
		return status, data, nil
	}
	var replacement *proxyPolicyRuleReplacement
	if _, ok := present["rules"]; ok {
		rules, err := parseRuleInputs(payload["rules"])
		if err != nil {
			return http.StatusBadRequest, shared.ErrorData(err.Error()), nil
		}
		replacement = &proxyPolicyRuleReplacement{Rules: rules}
	}
	repo := authorizationPolicyRepo(app)
	var policy map[string]any
	var updated bool
	err = app.WithTx(r.Context(), func(tx platform.StoreTx) error {
		var e error
		policy, updated, e = repo.UpdateProxyPolicyTx(r.Context(), tx, id, update, replacement)
		if e != nil || !updated {
			return e
		}
		tx.Emit(proxyPolicyEvent(r, "update", map[string]any{"old": current, "new": policy}))
		return nil
	})
	if err != nil {
		if platform.IsCreateConflict(err) {
			return http.StatusConflict, shared.ErrorData(msgPolicyRuleExists), nil
		}
		return http.StatusInternalServerError, shared.ErrorData("policy rules could not be updated"), nil
	}
	if !updated {
		return http.StatusNotFound, shared.ErrorData(msgPolicyNotFound), nil
	}
	return http.StatusOK, policy, nil
}

func policyUpdateFields(app *platform.App, r *http.Request, id string, payload map[string]any, present map[string]bool) (map[string]any, int, any, bool) {
	update := map[string]any{
		"description": shared.TextValue(payload, "description"),
		"updated_at":  time.Now().UTC(),
	}
	if _, ok := present["name"]; !ok {
		return update, 0, nil, true
	}
	name := shared.TextValue(payload, "name")
	if len(name) < 2 || len(name) > 100 {
		return nil, http.StatusBadRequest, shared.ErrorData("name must be between 2 and 100 characters"), false
	}
	if policyNameExists(app, r, id, name) {
		return nil, http.StatusBadRequest, shared.ErrorData("policy name already exists"), false
	}
	update["name"] = name
	return update, 0, nil, true
}

func updatePolicyRulesIfPresent(app *platform.App, r *http.Request, id string, payload map[string]any, present map[string]bool) (int, any, bool) {
	if _, ok := present["rules"]; !ok {
		return 0, nil, true
	}
	rules, err := parseRuleInputs(payload["rules"])
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(err.Error()), false
	}
	if err := setPolicyRules(app, r, id, rules); err != nil {
		if platform.IsCreateConflict(err) {
			return http.StatusConflict, shared.ErrorData(msgPolicyRuleExists), false
		}
		return http.StatusInternalServerError, shared.ErrorData("policy rules could not be updated"), false
	}
	return 0, nil, true
}

func deletePolicy(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	if status, data, ok := requireAdmin(app, r); !ok {
		return status, data, nil
	}
	id := strings.TrimSpace(r.PathValue("id"))
	current, found := findPolicy(app, r, id)
	if !found {
		return http.StatusNotFound, shared.ErrorData(msgPolicyNotFound), nil
	}
	if err := app.WithTx(r.Context(), func(tx platform.StoreTx) error {
		if _, _, e := authorizationPolicyRepo(app).DeleteProxyPolicyCascadeTx(r.Context(), tx, id); e != nil {
			return e
		}
		tx.Emit(proxyPolicyEvent(r, "delete", current))
		return nil
	}); err != nil {
		return http.StatusInternalServerError, shared.ErrorData("policy could not be deleted"), nil
	}
	return http.StatusOK, nil, nil
}
