package authorizationpolicy

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

func decodePayload(r *http.Request) (map[string]any, map[string]bool, error) {
	raw, err := readRequestBody(r)
	if err != nil {
		return nil, nil, err
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		return map[string]any{}, map[string]bool{}, nil
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, nil, fmt.Errorf("invalid JSON body")
	}
	present := map[string]bool{}
	for key := range payload {
		present[key] = true
	}
	return payload, present, nil
}

func parseRuleInputs(value any) ([]map[string]any, error) {
	if value == nil {
		return nil, nil
	}
	items, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("rules must be an array")
	}
	rules := make([]map[string]any, 0, len(items))
	for i, item := range items {
		raw, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("rules[%d] must be an object", i)
		}
		serviceID := shared.TextValue(raw, "service_id", "serviceId")
		actions := shared.StringSlice(raw["actions"])
		if serviceID == "" || len(actions) == 0 {
			return nil, fmt.Errorf("rules[%d] requires service_id and at least one action", i)
		}
		rules = append(rules, map[string]any{
			"service_id": serviceID,
			"actions":    actions,
		})
	}
	return rules, nil
}

func setPolicyRules(app *platform.App, r *http.Request, policyID string, rules []map[string]any) error {
	repo := authorizationPolicyRepo(app)
	return repo.replacePolicyRules(r.Context(), policyID, rules)
}

func assignmentTarget(payload map[string]any) (string, string, error) {
	targetType := shared.TextValue(payload, "target_type", "targetType")
	targetID := shared.TextValue(payload, "target_id", "targetId")
	if !validTargetType(targetType) {
		return "", "", fmt.Errorf("target_type must be role or user")
	}
	if targetID == "" {
		return "", "", fmt.Errorf("target_id is required")
	}
	return targetType, targetID, nil
}

func validTargetType(targetType string) bool {
	return targetType == "role" || targetType == "user"
}

func createPolicyAssignment(app *platform.App, r *http.Request, policyID, targetType, targetID, assignedBy string) (map[string]any, bool, error) {
	return authorizationPolicyRepo(app).CreatePolicyAssignment(r.Context(), policyID, targetType, targetID, assignedBy)
}

func assignmentRowsForPolicy(app *platform.App, r *http.Request, policyID string) []map[string]any {
	return authorizationPolicyRepo(app).ListPolicyAssignments(r.Context(), policyID)
}

func composeAssignment(app *platform.App, r *http.Request, data map[string]any) map[string]any {
	row := shared.CloneMap(data)
	policyID := shared.TextValue(row, "policy_id", "policyId")
	if policy, found := findPolicy(app, r, policyID); found {
		row["policy"] = policy
	}
	return row
}

func sortAssignments(rows []map[string]any) {
	sort.Slice(rows, func(i, j int) bool {
		left := shared.TextValue(rows[i], "policy_id", "policyId") + "|" + shared.TextValue(rows[i], "target_type", "targetType") + "|" + shared.TextValue(rows[i], "target_id", "targetId")
		right := shared.TextValue(rows[j], "policy_id", "policyId") + "|" + shared.TextValue(rows[j], "target_type", "targetType") + "|" + shared.TextValue(rows[j], "target_id", "targetId")
		return left < right
	})
}

func roleRows(app *platform.App, r *http.Request) []map[string]any {
	return authorizationPolicyRepo(app).ListProxyRoles(r.Context())
}

func findPlatformRole(app *platform.App, r *http.Request, id string) (map[string]any, bool) {
	return authorizationPolicyRepo(app).GetProxyRole(r.Context(), id)
}

func roleNameExists(app *platform.App, r *http.Request, excludeID, name string) bool {
	return authorizationPolicyRepo(app).RoleNameExists(r.Context(), excludeID, name)
}

func createRoleUser(app *platform.App, r *http.Request, roleID, userID, assignedBy string) (map[string]any, bool, error) {
	return authorizationPolicyRepo(app).CreateRoleUser(r.Context(), roleID, userID, assignedBy)
}

func composeRoleUser(app *platform.App, r *http.Request, data map[string]any) map[string]any {
	row := shared.CloneMap(data)
	if role, found := findPlatformRole(app, r, shared.TextValue(row, "role_id", "roleId")); found {
		row["role"] = role
	}
	return row
}

func batchFailure(result map[string]any, message string) {
	result["failed"] = result["failed"].(int) + 1
	result["errors"] = append(result["errors"].([]string), message)
}

func serviceRows(app *platform.App, r *http.Request) []map[string]any {
	return authorizationPolicyRepo(app).ListProxyServices(r.Context())
}

func policyRows(app *platform.App, r *http.Request) []map[string]any {
	return authorizationPolicyRepo(app).ListProxyPolicies(r.Context())
}

func findService(app *platform.App, r *http.Request, id string) (map[string]any, bool) {
	return authorizationPolicyRepo(app).GetProxyService(r.Context(), id)
}

func findPolicy(app *platform.App, r *http.Request, id string) (map[string]any, bool) {
	return authorizationPolicyRepo(app).GetProxyPolicy(r.Context(), id)
}

func composePolicy(app *platform.App, r *http.Request, policy map[string]any) map[string]any {
	return authorizationPolicyRepo(app).composePolicy(r.Context(), shared.CloneMap(policy))
}

func requireAdmin(app *platform.App, r *http.Request) (int, any, bool) {
	userID := strings.TrimSpace(r.Header.Get(headerUserID))
	if userID == "" {
		return http.StatusUnauthorized, shared.ErrorData("unauthorized"), false
	}
	if !hasAdminPanel(app, r, userID) {
		return http.StatusForbidden, shared.ErrorData("Only administrators can perform this operation"), false
	}
	return 0, nil, true
}

func hasAdminPanel(app *platform.App, r *http.Request, userID string) bool {
	roles := policyIdentityRecords(app, r, policyIdentityRoles, rolesResource)
	roles = append(roles, listMaps(app, r, authorizationRolesResource)...)
	for _, user := range policyIdentityRecords(app, r, policyIdentityUsers, usersResource) {
		if policyIdentityReadModelID(policyIdentityUsers, user) != userID && shared.TextValue(user, identityKeyUserID, identityKeyUserIDCamel, identityKeyUserIDTitle) != userID {
			continue
		}
		roleID := shared.TextValue(user, "role_id", "roleId", "RoleID", "role", "Role")
		for _, role := range roles {
			if (policyIdentityReadModelID(policyIdentityRoles, role) == roleID || shared.TextValue(role, identityKeyName, identityKeyNameTitle) == roleID) && recordGrantsAdminPanel(role) {
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

func authorizationPolicyEvent(r *http.Request, name, action string, data map[string]any) contracts.Event {
	if data == nil {
		data = map[string]any{}
	}
	payload := shared.CloneMap(data)
	payload["action"] = action
	return contracts.Event{
		EventID:        platform.NewUUID(),
		Name:           name,
		Source:         serviceName,
		OccurredAt:     time.Now().UTC(),
		TraceID:        shared.FirstNonEmpty(r.Header.Get("X-Trace-ID"), r.Header.Get("X-Request-ID"), platform.NewUUID()),
		SchemaVersion:  1,
		IdempotencyKey: r.Header.Get("Idempotency-Key"),
		Data:           payload,
	}
}

func proxyPolicyEvent(r *http.Request, action string, data map[string]any) contracts.Event {
	return authorizationPolicyEvent(r, "ProxyPolicyChanged", action, data)
}

func policyChangedEvent(r *http.Request, action string, data map[string]any) contracts.Event {
	return authorizationPolicyEvent(r, "PolicyChanged", action, data)
}

func publishProxyPolicyChanged(app *platform.App, r *http.Request, action string, data map[string]any) {
	if err := app.Events.Publish(r.Context(), proxyPolicyEvent(r, action, data)); err != nil {
		slog.Error("authorization-policy event publish failed", "event", "ProxyPolicyChanged", "error", err)
	}
}

func publishPolicyChanged(app *platform.App, r *http.Request, action string, data map[string]any) {
	if err := app.Events.Publish(r.Context(), policyChangedEvent(r, action, data)); err != nil {
		slog.Error("authorization-policy event publish failed", "event", "PolicyChanged", "error", err)
	}
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
