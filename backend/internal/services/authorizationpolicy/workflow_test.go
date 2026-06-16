package authorizationpolicy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

func TestRawPermissionHandlers(t *testing.T) {
	app := newPolicyTestApp(t)
	policy := `["alice","project-1","model","read"]`

	code, data, _ := addRawPermissionPolicy(app, policyRequest(http.MethodPost, pathRawPermissionPolicy, policy, "ADMIN"), platform.RouteSpec{})
	assertPolicyStatus(t, code, data, http.StatusOK)
	code, data, _ = addRawPermissionPolicy(app, policyRequest(http.MethodPost, pathRawPermissionPolicy, policy, "ADMIN"), platform.RouteSpec{})
	assertPolicyStatus(t, code, data, http.StatusConflict)

	code, data, _ = listRawPermissionPolicies(app, policyRequest(http.MethodGet, pathRawPermissionPolicy, "", "ADMIN"), platform.RouteSpec{})
	assertPolicyStatus(t, code, data, http.StatusOK)
	if rows := data.([][]string); len(rows) != 1 || rows[0][0] != "alice" {
		t.Fatalf("raw policy rows = %#v, want alice policy", rows)
	}

	code, data, _ = simulatePermissionEnforce(app, policyRequest(http.MethodPost, "/api/v1/permissions/simulate", `{"sub":"alice","dom":"project-1","obj":"model","act":"read"}`, "ADMIN"), platform.RouteSpec{})
	assertPolicyStatus(t, code, data, http.StatusOK)
	if !data.(map[string]bool)["allowed"] {
		t.Fatalf("simulate decision = %#v, want allowed", data)
	}

	updateBody := `{"old":["alice","project-1","model","read"],"new":["alice","project-1","model","write"]}`
	code, data, _ = updateRawPermissionPolicy(app, policyRequest(http.MethodPut, pathRawPermissionPolicy, updateBody, "ADMIN"), platform.RouteSpec{})
	assertPolicyStatus(t, code, data, http.StatusOK)

	enforceReq := policyRequest(http.MethodPost, "/api/v1/permissions/enforce", `{"sub":"alice","dom":"project-1","obj":"model","act":"write"}`, "svc:identity")
	enforceReq.Header.Set("X-User-Role", "service")
	code, data, _ = enforcePermission(app, enforceReq, platform.RouteSpec{})
	assertPolicyStatus(t, code, data, http.StatusOK)
	if !data.(contracts.Decision).Allowed {
		t.Fatalf("enforce decision = %#v, want allowed", data)
	}

	batch := `{"operations":[{"type":"project_member","action":"add","project_id":"P1","user_id":"U1","role":"user"},{"type":"group_role","action":"remove","group_id":"G1","user_id":"U1","role":"admin"}]}`
	code, data, _ = batchProcessPermissions(app, policyRequest(http.MethodPost, "/api/v1/permissions/batch", batch, "ADMIN"), platform.RouteSpec{})
	assertPolicyStatus(t, code, data, http.StatusOK)
	if got := len(rawPermissionRepo(app).ListGroupingPolicies(context.Background())); got != 1 {
		t.Fatalf("grouping policies = %d, want one add operation result", got)
	}

	code, data, _ = removeRawPermissionPolicy(app, policyRequest(http.MethodDelete, pathRawPermissionPolicy, `["alice","project-1","model","write"]`, "ADMIN"), platform.RouteSpec{})
	assertPolicyStatus(t, code, data, http.StatusOK)
	if countPolicyEvents(app, "PolicyChanged") < 4 {
		t.Fatal("expected PolicyChanged events for raw policy mutations")
	}
}

func TestProxyServicePolicyAndAssignmentHandlers(t *testing.T) {
	app := newPolicyTestApp(t)
	req := policyRequest(http.MethodGet, "/api/v1/admin/proxy-rbac/services", "", "ADMIN")

	code, data, _ := listServices(app, req, platform.RouteSpec{})
	assertPolicyStatus(t, code, data, http.StatusOK)
	if services := data.([]map[string]any); len(services) == 0 || services[0]["id"] != "SVC_MINIO" {
		t.Fatalf("services = %#v, want seeded services sorted by sort_order", data)
	}
	serviceReq := policyRequest(http.MethodGet, "/api/v1/admin/proxy-rbac/services/SVC_MINIO", "", "ADMIN")
	serviceReq.SetPathValue("id", "SVC_MINIO")
	code, data, _ = getService(app, serviceReq, platform.RouteSpec{})
	assertPolicyStatus(t, code, data, http.StatusOK)
	code, data, _ = createService(app, policyRequest(http.MethodPost, "/api/v1/admin/proxy-rbac/services", `{}`, "ADMIN"), platform.RouteSpec{})
	assertPolicyStatus(t, code, data, http.StatusMethodNotAllowed)

	policyID := createPolicyForTest(t, app)
	policyReq := policyRequest(http.MethodGet, "/api/v1/admin/proxy-rbac/policies/"+policyID, "", "ADMIN")
	policyReq.SetPathValue("id", policyID)
	code, data, _ = getPolicy(app, policyReq, platform.RouteSpec{})
	assertPolicyStatus(t, code, data, http.StatusOK)

	assignReq := policyRequest(http.MethodPost, "/api/v1/admin/proxy-rbac/policies/"+policyID+"/assignments", `{"target_type":"user","target_id":"U1"}`, "ADMIN")
	assignReq.SetPathValue("id", policyID)
	code, data, _ = assignPolicy(app, assignReq, platform.RouteSpec{})
	assertPolicyStatus(t, code, data, http.StatusCreated)
	assignment := data.(map[string]any)
	if assignment["policy_id"] != policyID || assignment["target_id"] != "U1" {
		t.Fatalf("assignment = %#v, want policy assigned to U1", assignment)
	}

	listReq := policyRequest(http.MethodGet, "/api/v1/admin/proxy-rbac/policies/"+policyID+"/assignments", "", "ADMIN")
	listReq.SetPathValue("id", policyID)
	code, data, _ = listPolicyAssignments(app, listReq, platform.RouteSpec{})
	assertPolicyStatus(t, code, data, http.StatusOK)
	if rows := data.([]map[string]any); len(rows) != 1 || rows[0]["policy"] == nil {
		t.Fatalf("policy assignments = %#v, want composed policy", rows)
	}

	targetReq := policyRequest(http.MethodGet, "/api/v1/admin/proxy-rbac/targets/user/U1/assignments", "", "ADMIN")
	targetReq.SetPathValue("type", "user")
	targetReq.SetPathValue("id", "U1")
	code, data, _ = listTargetAssignments(app, targetReq, platform.RouteSpec{})
	assertPolicyStatus(t, code, data, http.StatusOK)

	unassignReq := policyRequest(http.MethodDelete, "/api/v1/admin/proxy-rbac/policies/"+policyID+"/assignments", `{"target_type":"user","target_id":"U1"}`, "ADMIN")
	unassignReq.SetPathValue("id", policyID)
	code, data, _ = unassignPolicy(app, unassignReq, platform.RouteSpec{})
	assertPolicyStatus(t, code, data, http.StatusOK)
}

func TestProxyRoleHandlers(t *testing.T) {
	app := newPolicyTestApp(t)

	code, data, _ := listRoles(app, policyRequest(http.MethodGet, "/api/v1/admin/proxy-rbac/roles", "", "ADMIN"), platform.RouteSpec{})
	assertPolicyStatus(t, code, data, http.StatusOK)
	if roles := data.([]map[string]any); len(roles) != 2 || roles[0]["id"] != "RL2600001" {
		t.Fatalf("roles = %#v, want seeded roles", roles)
	}

	roleID := createRoleForTest(t, app)
	updateReq := policyRequest(http.MethodPut, "/api/v1/admin/proxy-rbac/roles/"+roleID, `{"display_name":"Analytics Admins","description":"updated"}`, "ADMIN")
	updateReq.SetPathValue("id", roleID)
	code, data, _ = updateRole(app, updateReq, platform.RouteSpec{})
	assertPolicyStatus(t, code, data, http.StatusOK)
	if data.(map[string]any)["display_name"] != "Analytics Admins" {
		t.Fatalf("updated role = %#v, want display name change", data)
	}

	assignReq := policyRequest(http.MethodPost, "/api/v1/admin/proxy-rbac/roles/"+roleID+"/users", `{"user_id":"U1"}`, "ADMIN")
	assignReq.SetPathValue("id", roleID)
	code, data, _ = assignRoleUser(app, assignReq, platform.RouteSpec{})
	assertPolicyStatus(t, code, data, http.StatusCreated)
	if data.(map[string]any)["user_id"] != "U1" {
		t.Fatalf("role user = %#v, want U1", data)
	}

	batchReq := policyRequest(http.MethodPost, "/api/v1/admin/proxy-rbac/roles/"+roleID+"/users/batch", `{"user_ids":["U1","U2"]}`, "ADMIN")
	batchReq.SetPathValue("id", roleID)
	code, data, _ = batchAssignRoleUsers(app, batchReq, platform.RouteSpec{})
	assertPolicyStatus(t, code, data, http.StatusOK)
	if result := data.(map[string]any); result["succeeded"] != 2 || result["failed"] != 0 {
		t.Fatalf("batch role users = %#v, want two successes", result)
	}

	listReq := policyRequest(http.MethodGet, "/api/v1/admin/proxy-rbac/roles/"+roleID+"/users", "", "ADMIN")
	listReq.SetPathValue("id", roleID)
	code, data, _ = listRoleUsers(app, listReq, platform.RouteSpec{})
	assertPolicyStatus(t, code, data, http.StatusOK)
	if members := data.([]map[string]any); len(members) != 2 || members[0]["role"] == nil {
		t.Fatalf("role members = %#v, want composed role users", members)
	}

	systemReq := policyRequest(http.MethodGet, "/api/v1/admin/proxy-rbac/system-roles", "", "ADMIN")
	code, data, _ = listSystemRoles(app, systemReq, platform.RouteSpec{})
	assertPolicyStatus(t, code, data, http.StatusOK)
	if systemRoles := data.([]map[string]any); len(systemRoles) != 2 {
		t.Fatalf("system roles = %#v, want source identity roles", systemRoles)
	}

	unassignReq := policyRequest(http.MethodDelete, "/api/v1/admin/proxy-rbac/roles/"+roleID+"/users/U1", "", "ADMIN")
	unassignReq.SetPathValue("id", roleID)
	unassignReq.SetPathValue("user_id", "U1")
	code, data, _ = unassignRoleUser(app, unassignReq, platform.RouteSpec{})
	assertPolicyStatus(t, code, data, http.StatusOK)

	deleteReq := policyRequest(http.MethodDelete, "/api/v1/admin/proxy-rbac/roles/"+roleID, "", "ADMIN")
	deleteReq.SetPathValue("id", roleID)
	code, data, _ = deleteRole(app, deleteReq, platform.RouteSpec{})
	assertPolicyStatus(t, code, data, http.StatusOK)
}

func TestProxyPolicyMutationHandlers(t *testing.T) {
	app := newPolicyTestApp(t)
	policyID := createPolicyForTest(t, app)

	code, data, _ := listPolicies(app, policyRequest(http.MethodGet, "/api/v1/admin/proxy-rbac/policies", "", "ADMIN"), platform.RouteSpec{})
	assertPolicyStatus(t, code, data, http.StatusOK)
	if policies := data.([]map[string]any); len(policies) == 0 {
		t.Fatal("listPolicies returned no policies")
	}

	updateReq := policyRequest(http.MethodPut, "/api/v1/admin/proxy-rbac/policies/"+policyID, `{"name":"science-proxy-updated","description":"updated","rules":[{"service_id":"SVC_HARBOR","actions":["view"]}]}`, "ADMIN")
	updateReq.SetPathValue("id", policyID)
	code, data, _ = updatePolicy(app, updateReq, platform.RouteSpec{})
	assertPolicyStatus(t, code, data, http.StatusOK)
	updated := data.(map[string]any)
	if updated["name"] != "science-proxy-updated" {
		t.Fatalf("updated policy = %#v, want renamed policy", updated)
	}

	if status, data, ok := updatePolicyRulesIfPresent(app, updateReq, policyID, map[string]any{}, map[string]bool{}); !ok || status != 0 || data != nil {
		t.Fatalf("rules absent status=%d data=%#v ok=%v, want no-op success", status, data, ok)
	}
	badRules := map[string]any{"rules": []any{map[string]any{"service_id": "SVC_MINIO"}}}
	if status, _, ok := updatePolicyRulesIfPresent(app, updateReq, policyID, badRules, map[string]bool{"rules": true}); ok || status != http.StatusBadRequest {
		t.Fatalf("bad rules status=%d ok=%v, want bad request", status, ok)
	}
	validRules := map[string]any{"rules": []any{map[string]any{"service_id": "SVC_MINIO", "actions": []any{"view"}}}}
	if status, data, ok := updatePolicyRulesIfPresent(app, updateReq, policyID, validRules, map[string]bool{"rules": true}); !ok || status != 0 || data != nil {
		t.Fatalf("valid rules status=%d data=%#v ok=%v, want success", status, data, ok)
	}

	secondBody := `{"name":"data-proxy","description":"data","rules":[{"service_id":"SVC_MINIO","actions":["view"]}]}`
	code, data, _ = createPolicy(app, policyRequest(http.MethodPost, "/api/v1/admin/proxy-rbac/policies", secondBody, "ADMIN"), platform.RouteSpec{})
	assertPolicyStatus(t, code, data, http.StatusCreated)
	secondID := data.(map[string]any)["id"].(string)

	dupReq := policyRequest(http.MethodPut, "/api/v1/admin/proxy-rbac/policies/"+secondID, `{"name":"science-proxy-updated"}`, "ADMIN")
	dupReq.SetPathValue("id", secondID)
	code, data, _ = updatePolicy(app, dupReq, platform.RouteSpec{})
	assertPolicyStatus(t, code, data, http.StatusBadRequest)

	missingReq := policyRequest(http.MethodPut, "/api/v1/admin/proxy-rbac/policies/PO_MISSING", `{"name":"missing"}`, "ADMIN")
	missingReq.SetPathValue("id", "PO_MISSING")
	code, data, _ = updatePolicy(app, missingReq, platform.RouteSpec{})
	assertPolicyStatus(t, code, data, http.StatusNotFound)

	deleteReq := policyRequest(http.MethodDelete, "/api/v1/admin/proxy-rbac/policies/"+policyID, "", "ADMIN")
	deleteReq.SetPathValue("id", policyID)
	code, data, _ = deletePolicy(app, deleteReq, platform.RouteSpec{})
	assertPolicyStatus(t, code, data, http.StatusOK)

	getReq := policyRequest(http.MethodGet, "/api/v1/admin/proxy-rbac/policies/"+policyID, "", "ADMIN")
	getReq.SetPathValue("id", policyID)
	code, data, _ = getPolicy(app, getReq, platform.RouteSpec{})
	assertPolicyStatus(t, code, data, http.StatusNotFound)
}

func TestProxyPolicyBatchAndLegacyAssignmentHandlers(t *testing.T) {
	app := newPolicyTestApp(t)
	policyID := createPolicyForTest(t, app)
	roleID := createRoleForTest(t, app)

	batchBody := `{"assignments":[{"target_type":"user","target_id":"U1"},{"target_type":"role","target_id":"` + roleID + `"}]}`
	batchReq := policyRequest(http.MethodPost, "/api/v1/admin/proxy-rbac/policies/"+policyID+"/assignments/batch", batchBody, "ADMIN")
	batchReq.SetPathValue("id", policyID)
	code, data, _ := batchAssignPolicy(app, batchReq, platform.RouteSpec{})
	assertPolicyStatus(t, code, data, http.StatusOK)
	if result := data.(map[string]any); result["succeeded"] != 2 || result["failed"] != 0 {
		t.Fatalf("batch assignments = %#v, want two successes", result)
	}

	legacyReq := policyRequest(http.MethodPost, "/api/v1/admin/proxy-rbac/assignments", `{"policy_id":"`+policyID+`","target_type":"user","target_id":"U2"}`, "ADMIN")
	code, data, _ = assignPolicyLegacy(app, legacyReq, platform.RouteSpec{})
	assertPolicyStatus(t, code, data, http.StatusCreated)
	code, data, _ = assignPolicyLegacy(app, policyRequest(http.MethodPost, "/api/v1/admin/proxy-rbac/assignments", `{"policy_id":"`+policyID+`","target_type":"user","target_id":"U2"}`, "ADMIN"), platform.RouteSpec{})
	assertPolicyStatus(t, code, data, http.StatusOK)

	code, data, _ = assignPolicyLegacy(app, policyRequest(http.MethodPost, "/api/v1/admin/proxy-rbac/assignments", `{"target_type":"user","target_id":"U3"}`, "ADMIN"), platform.RouteSpec{})
	assertPolicyStatus(t, code, data, http.StatusBadRequest)
	code, data, _ = assignPolicyLegacy(app, policyRequest(http.MethodPost, "/api/v1/admin/proxy-rbac/assignments", `{"policy_id":"PO_MISSING","target_type":"user","target_id":"U3"}`, "ADMIN"), platform.RouteSpec{})
	assertPolicyStatus(t, code, data, http.StatusNotFound)

	invalidBatch := policyRequest(http.MethodPost, "/api/v1/admin/proxy-rbac/policies/"+policyID+"/assignments/batch", `{"assignments":["bad"]}`, "ADMIN")
	invalidBatch.SetPathValue("id", policyID)
	code, data, _ = batchAssignPolicy(app, invalidBatch, platform.RouteSpec{})
	assertPolicyStatus(t, code, data, http.StatusBadRequest)

	invalidTarget := policyRequest(http.MethodGet, "/api/v1/admin/proxy-rbac/targets/team/T1/assignments", "", "ADMIN")
	invalidTarget.SetPathValue("type", "team")
	invalidTarget.SetPathValue("id", "T1")
	code, data, _ = listTargetAssignments(app, invalidTarget, platform.RouteSpec{})
	assertPolicyStatus(t, code, data, http.StatusBadRequest)
}

func TestProxyRoleLegacyAndFailureHandlers(t *testing.T) {
	app := newPolicyTestApp(t)
	roleID := createRoleForTest(t, app)

	code, data, _ := listPlatformRolesLegacy(app, policyRequest(http.MethodGet, "/api/v1/admin/proxy-rbac/roles", "", "ADMIN"), platform.RouteSpec{})
	assertPolicyStatus(t, code, data, http.StatusOK)

	getReq := policyRequest(http.MethodGet, "/api/v1/admin/proxy-rbac/roles/"+roleID, "", "ADMIN")
	getReq.SetPathValue("id", roleID)
	code, data, _ = getRole(app, getReq, platform.RouteSpec{})
	assertPolicyStatus(t, code, data, http.StatusOK)

	missingReq := policyRequest(http.MethodGet, "/api/v1/admin/proxy-rbac/roles/RL_MISSING", "", "ADMIN")
	missingReq.SetPathValue("id", "RL_MISSING")
	code, data, _ = getRole(app, missingReq, platform.RouteSpec{})
	assertPolicyStatus(t, code, data, http.StatusNotFound)

	code, data, _ = createRole(app, policyRequest(http.MethodPost, "/api/v1/admin/proxy-rbac/roles", `{"name":"xy"}`, "ADMIN"), platform.RouteSpec{})
	assertPolicyStatus(t, code, data, http.StatusBadRequest)
	code, data, _ = createRole(app, policyRequest(http.MethodPost, "/api/v1/admin/proxy-rbac/roles", `{"name":"analytics","display_name":"Duplicate"}`, "ADMIN"), platform.RouteSpec{})
	assertPolicyStatus(t, code, data, http.StatusBadRequest)

	legacyReq := policyRequest(http.MethodPost, "/api/v1/admin/proxy-rbac/role-users", `{"role_id":"`+roleID+`","user_id":"U2"}`, "ADMIN")
	code, data, _ = assignRoleUserLegacy(app, legacyReq, platform.RouteSpec{})
	assertPolicyStatus(t, code, data, http.StatusCreated)
	code, data, _ = assignRoleUserLegacy(app, policyRequest(http.MethodPost, "/api/v1/admin/proxy-rbac/role-users", `{"role_id":"`+roleID+`","user_id":"U2"}`, "ADMIN"), platform.RouteSpec{})
	assertPolicyStatus(t, code, data, http.StatusOK)

	code, data, _ = assignRoleUserLegacy(app, policyRequest(http.MethodPost, "/api/v1/admin/proxy-rbac/role-users", `{"user_id":"U1"}`, "ADMIN"), platform.RouteSpec{})
	assertPolicyStatus(t, code, data, http.StatusBadRequest)
	code, data, _ = assignRoleUserLegacy(app, policyRequest(http.MethodPost, "/api/v1/admin/proxy-rbac/role-users", `{"role_id":"RL_MISSING","user_id":"U1"}`, "ADMIN"), platform.RouteSpec{})
	assertPolicyStatus(t, code, data, http.StatusNotFound)

	batchReq := policyRequest(http.MethodPost, "/api/v1/admin/proxy-rbac/roles/"+roleID+"/users/batch", `{"user_ids":["","U1"]}`, "ADMIN")
	batchReq.SetPathValue("id", roleID)
	code, data, _ = batchAssignRoleUsers(app, batchReq, platform.RouteSpec{})
	assertPolicyStatus(t, code, data, http.StatusOK)
	if result := data.(map[string]any); result["succeeded"] != 1 || result["failed"] != 0 {
		t.Fatalf("batch role users = %#v, want one filtered success", result)
	}
}

func TestAuthorizationPolicyHelperBranches(t *testing.T) {
	app := newPolicyTestApp(t)
	req := policyRequest(http.MethodGet, "/", "", "ADMIN")
	policyID := createPolicyForTest(t, app)
	roleID := createRoleForTest(t, app)
	ensureDefaultServices(app, req)
	ensureDefaultPolicies(app, req)
	ensureDefaultPlatformRoles(app, req)

	if _, err := parseRuleInputs("bad"); err == nil {
		t.Fatal("parseRuleInputs accepted non-array")
	}
	if _, err := parseRuleInputs([]any{"bad"}); err == nil {
		t.Fatal("parseRuleInputs accepted non-object item")
	}
	if _, err := parseRuleInputs([]any{map[string]any{"service_id": "SVC_MINIO"}}); err == nil {
		t.Fatal("parseRuleInputs accepted missing actions")
	}

	assignment, _, err := createPolicyAssignment(app, req, policyID, "user", "U1", "ADMIN")
	if err != nil {
		t.Fatal(err)
	}
	if row := composeAssignment(app, req, assignment); row["policy"] == nil {
		t.Fatalf("composeAssignment = %#v, want policy", row)
	}
	member, _, err := createRoleUser(app, req, roleID, "U1", "ADMIN")
	if err != nil {
		t.Fatal(err)
	}
	if row := composeRoleUser(app, req, member); row["role"] == nil {
		t.Fatalf("composeRoleUser = %#v, want role", row)
	}
	if row := composePolicy(app, req, map[string]any{"id": policyID, "name": "science-proxy"}); row["rules"] == nil {
		t.Fatalf("composePolicy = %#v, want rules", row)
	}

	rows := []map[string]any{
		{"policy_id": "B", "target_type": "user", "target_id": "2"},
		{"policy_id": "A", "target_type": "role", "target_id": "1"},
	}
	sortAssignments(rows)
	if rows[0]["policy_id"] != "A" {
		t.Fatalf("sortAssignments = %#v, want A first", rows)
	}
	result := map[string]any{"succeeded": 0, "failed": 0, "errors": []string{}}
	batchFailure(result, "boom")
	if result["failed"] != 1 || len(result["errors"].([]string)) != 1 {
		t.Fatalf("batchFailure = %#v, want one failure", result)
	}
	if !recordGrantsAdminPanel(map[string]any{"admin_panel": true}) {
		t.Fatal("recordGrantsAdminPanel denied direct admin_panel")
	}
	if recordGrantsAdminPanel(map[string]any{"capabilities": map[string]any{"adminPanel": false}}) {
		t.Fatal("recordGrantsAdminPanel allowed false capability")
	}
}

func TestRawPolicyDecodeValidationBranches(t *testing.T) {
	policy, err := decodeRawPolicy(policyRequest(http.MethodPost, pathRawPermissionPolicy, `[" alice "," project "," model "," read "]`, "ADMIN"))
	if err != nil {
		t.Fatal(err)
	}
	if policy[0] != "alice" || policy[3] != "read" {
		t.Fatalf("decodeRawPolicy = %#v, want trimmed tuple", policy)
	}
	if _, err := decodeRawPolicy(policyRequest(http.MethodPost, pathRawPermissionPolicy, `{}`, "ADMIN")); err == nil {
		t.Fatal("decodeRawPolicy accepted object payload")
	}
}

func TestRawPolicyUpdateDecodeValidationBranches(t *testing.T) {
	oldPolicy, newPolicy, err := decodeRawPolicyUpdate(policyRequest(http.MethodPut, pathRawPermissionPolicy, `{"old":["alice","p1","model","read"],"new":["alice","p1","model","write"]}`, "ADMIN"))
	if err != nil {
		t.Fatal(err)
	}
	if oldPolicy[3] != "read" || newPolicy[3] != "write" {
		t.Fatalf("decodeRawPolicyUpdate old=%#v new=%#v", oldPolicy, newPolicy)
	}
	if _, _, err := decodeRawPolicyUpdate(policyRequest(http.MethodPut, pathRawPermissionPolicy, `{}`, "ADMIN")); err == nil {
		t.Fatal("decodeRawPolicyUpdate accepted missing tuples")
	}
	if _, _, err := decodeRawPolicyUpdate(policyRequest(http.MethodPut, pathRawPermissionPolicy, `{`, "ADMIN")); err == nil {
		t.Fatal("decodeRawPolicyUpdate accepted invalid JSON")
	}
}

func TestPermissionOperationsDecodeValidationBranches(t *testing.T) {
	ops, err := decodePermissionOperations(policyRequest(http.MethodPost, "/api/v1/permissions/batch", `{"operations":[{"type":"project_member","action":"add","project_id":"P1","user_id":"U1","role":"viewer"}]}`, "ADMIN"))
	if err != nil {
		t.Fatal(err)
	}
	if len(ops) != 1 || ops[0]["project_id"] != "P1" {
		t.Fatalf("decodePermissionOperations = %#v, want project operation", ops)
	}
	for _, body := range []string{`{}`, `{"operations":["bad"]}`, `{"operations":[{"type":"project_member","action":"add"}]}`} {
		if _, err := decodePermissionOperations(policyRequest(http.MethodPost, "/api/v1/permissions/batch", body, "ADMIN")); err == nil {
			t.Fatalf("decodePermissionOperations accepted %s", body)
		}
	}
}

func TestRawPolicyRecordAndSliceBranches(t *testing.T) {
	record := rawPolicyRecord([]string{"alice", "p1", "model"})
	if record["sub"] != nil {
		t.Fatalf("rawPolicyRecord short tuple = %#v, want no sub/dom/obj/act aliases", record)
	}
	if got := policySlice([]any{" alice ", 2}); len(got) != 2 || got[0] != "alice" || got[1] != "2" {
		t.Fatalf("policySlice []any = %#v", got)
	}
	if got := policySlice("bad"); got != nil {
		t.Fatalf("policySlice default = %#v, want nil", got)
	}
}

func TestAuthorizationPolicyRepositoryUnavailableBranches(t *testing.T) {
	ctx := context.Background()
	repo := recordStoreAuthorizationPolicyRepository{}

	if err := repo.EnsureDefaultProxyServices(ctx); err == nil {
		t.Fatal("EnsureDefaultProxyServices without store returned nil")
	}
	if err := repo.EnsureDefaultProxyPolicies(ctx); err == nil {
		t.Fatal("EnsureDefaultProxyPolicies without store returned nil")
	}
	if err := repo.EnsureDefaultProxyAssignments(ctx); err == nil {
		t.Fatal("EnsureDefaultProxyAssignments without store returned nil")
	}
	if err := repo.EnsureDefaultProxyRoles(ctx); err == nil {
		t.Fatal("EnsureDefaultProxyRoles without store returned nil")
	}
	if _, err := repo.CreateProxyPolicy(ctx, map[string]any{"id": "PO1"}, nil); err == nil {
		t.Fatal("CreateProxyPolicy without store returned nil")
	}
	if _, ok, err := repo.UpdateProxyPolicy(ctx, "PO1", map[string]any{"name": "n"}, nil); err == nil || ok {
		t.Fatalf("UpdateProxyPolicy without store ok=%v err=%v, want error", ok, err)
	}
	if _, err := repo.CreateProxyRole(ctx, map[string]any{"id": "RL1"}); err == nil {
		t.Fatal("CreateProxyRole without store returned nil")
	}
	if _, _, err := repo.CreatePolicyAssignment(ctx, "PO1", "user", "U1", "ADMIN"); err == nil {
		t.Fatal("CreatePolicyAssignment without store returned nil")
	}
	if _, _, err := repo.CreateRoleUser(ctx, "RL1", "U1", "ADMIN"); err == nil {
		t.Fatal("CreateRoleUser without store returned nil")
	}
	if got := repo.NextProxyPolicyID(ctx); got != "" {
		t.Fatalf("NextProxyPolicyID without store = %q, want empty", got)
	}
}

func TestAuthorizationPolicyGuardFailures(t *testing.T) {
	app := newPolicyTestApp(t)
	guardCases := []struct {
		name string
		call func() (int, any, *platform.Degraded)
		want int
	}{
		{name: "services anonymous", call: func() (int, any, *platform.Degraded) {
			return listServices(app, policyRequest(http.MethodGet, "/api/v1/admin/proxy-rbac/services", "", ""), platform.RouteSpec{})
		}, want: http.StatusUnauthorized},
		{name: "policy bad JSON", call: func() (int, any, *platform.Degraded) {
			return createPolicy(app, policyRequest(http.MethodPost, "/api/v1/admin/proxy-rbac/policies", `{`, "ADMIN"), platform.RouteSpec{})
		}, want: http.StatusBadRequest},
		{name: "assignment bad target", call: func() (int, any, *platform.Degraded) {
			req := policyRequest(http.MethodPost, "/api/v1/admin/proxy-rbac/policies/PO2600001/assignments", `{"target_type":"team","target_id":"G1"}`, "ADMIN")
			req.SetPathValue("id", "PO2600001")
			return assignPolicy(app, req, platform.RouteSpec{})
		}, want: http.StatusBadRequest},
		{name: "role missing user", call: func() (int, any, *platform.Degraded) {
			req := policyRequest(http.MethodPost, "/api/v1/admin/proxy-rbac/roles/RL2600001/users", `{}`, "ADMIN")
			req.SetPathValue("id", "RL2600001")
			return assignRoleUser(app, req, platform.RouteSpec{})
		}, want: http.StatusBadRequest},
		{name: "enforce non-service", call: func() (int, any, *platform.Degraded) {
			return enforcePermission(app, policyRequest(http.MethodPost, "/api/v1/permissions/enforce", `{"sub":"alice","obj":"model","act":"read"}`, "U1"), platform.RouteSpec{})
		}, want: http.StatusForbidden},
	}
	for _, tc := range guardCases {
		t.Run(tc.name, func(t *testing.T) {
			code, data, degraded := tc.call()
			if degraded != nil || code != tc.want {
				t.Fatalf("status=%d degraded=%v data=%#v, want %d", code, degraded, data, tc.want)
			}
		})
	}
}

func createPolicyForTest(t *testing.T, app *platform.App) string {
	t.Helper()
	body := `{"name":"science-proxy","description":"science team","rules":[{"service_id":"SVC_MINIO","actions":["view","create"]}]}`
	code, data, _ := createPolicy(app, policyRequest(http.MethodPost, "/api/v1/admin/proxy-rbac/policies", body, "ADMIN"), platform.RouteSpec{})
	assertPolicyStatus(t, code, data, http.StatusCreated)
	return data.(map[string]any)["id"].(string)
}

func createRoleForTest(t *testing.T, app *platform.App) string {
	t.Helper()
	body := `{"name":"analytics","display_name":"Analytics","description":"analytics team"}`
	code, data, _ := createRole(app, policyRequest(http.MethodPost, "/api/v1/admin/proxy-rbac/roles", body, "ADMIN"), platform.RouteSpec{})
	assertPolicyStatus(t, code, data, http.StatusCreated)
	return data.(map[string]any)["id"].(string)
}

func newPolicyTestApp(t *testing.T) *platform.App {
	t.Helper()
	app := platform.NewApp(platform.Config{ServiceName: "all", HTTPAddr: ":0"})
	Register(app)
	createPolicyRecords(t, app, usersResource, []map[string]any{
		{"id": "U1", "username": "alice", "role_id": "RO_USER"},
		{"id": "U2", "username": "bob", "role_id": "RO_USER"},
		{"id": "ADMIN", "username": "admin", "role_id": "RO_ADMIN"},
	})
	createPolicyRecords(t, app, rolesResource, []map[string]any{
		{"id": "RO_ADMIN", "name": "admin", "capabilities": map[string]any{"adminPanel": true}},
		{"id": "RO_USER", "name": "user", "capabilities": map[string]any{"adminPanel": false}},
	})
	return app
}

func createPolicyRecords(t *testing.T, app *platform.App, resource string, rows []map[string]any) {
	t.Helper()
	for _, row := range rows {
		if _, err := app.Store.Create(context.Background(), resource, row); err != nil {
			t.Fatal(err)
		}
	}
}

func policyRequest(method, target, body, userID string) *http.Request {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if userID != "" {
		req.Header.Set(headerUserID, userID)
	}
	return req
}

func assertPolicyStatus(t *testing.T, code int, data any, want int) {
	t.Helper()
	if code != want {
		t.Fatalf("status=%d data=%#v, want %d", code, data, want)
	}
}

func countPolicyEvents(app *platform.App, name string) int {
	count := 0
	for _, event := range app.Events.Outbox() {
		if event.Name == name {
			count++
		}
	}
	return count
}
