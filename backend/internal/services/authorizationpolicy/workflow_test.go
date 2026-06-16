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

func TestProxyPolicyUpdateDeleteAndRuleReplacementHandlers(t *testing.T) {
	app := newPolicyTestApp(t)

	code, data, _ := listPolicies(app, policyRequest(http.MethodGet, "/api/v1/admin/proxy-rbac/policies", "", "ADMIN"), platform.RouteSpec{})
	assertPolicyStatus(t, code, data, http.StatusOK)
	if policies := data.([]map[string]any); len(policies) < 2 {
		t.Fatalf("policies = %#v, want seeded policies", policies)
	}

	policyID := createPolicyForTest(t, app)
	updateReq := policyRequest(http.MethodPut, "/api/v1/admin/proxy-rbac/policies/"+policyID, `{"name":"science-proxy-v2","description":"science team v2","rules":[{"service_id":"SVC_HARBOR","actions":["create"]}]}`, "ADMIN")
	updateReq.SetPathValue("id", policyID)
	code, data, _ = updatePolicy(app, updateReq, platform.RouteSpec{})
	assertPolicyStatus(t, code, data, http.StatusOK)
	updated := data.(map[string]any)
	if updated["name"] != "science-proxy-v2" || updated["description"] != "science team v2" {
		t.Fatalf("updated policy = %#v, want renamed science proxy", updated)
	}
	rules := updated["rules"].([]map[string]any)
	if len(rules) != 1 || rules[0]["service_id"] != "SVC_HARBOR" {
		t.Fatalf("updated policy rules = %#v, want Harbor replacement", rules)
	}

	duplicateReq := policyRequest(http.MethodPatch, "/api/v1/admin/proxy-rbac/policies/"+policyID, `{"name":"read-only-proxy-access"}`, "ADMIN")
	duplicateReq.SetPathValue("id", policyID)
	code, data, _ = updatePolicy(app, duplicateReq, platform.RouteSpec{})
	assertPolicyStatus(t, code, data, http.StatusBadRequest)

	invalidRulesReq := policyRequest(http.MethodPatch, "/api/v1/admin/proxy-rbac/policies/"+policyID, "", "ADMIN")
	status, _, ok := updatePolicyRulesIfPresent(app, invalidRulesReq, policyID, map[string]any{"rules": "invalid"}, map[string]bool{"rules": true})
	if ok || status != http.StatusBadRequest {
		t.Fatalf("invalid rules status=%d ok=%v, want bad request", status, ok)
	}
	status, data, ok = updatePolicyRulesIfPresent(app, invalidRulesReq, policyID, map[string]any{}, map[string]bool{})
	if !ok || status != 0 || data != nil {
		t.Fatalf("absent rules status=%d ok=%v data=%#v, want passthrough", status, ok, data)
	}
	status, data, ok = updatePolicyRulesIfPresent(app, invalidRulesReq, policyID, map[string]any{
		"rules": []any{map[string]any{"service_id": "SVC_GRAFANA", "actions": []any{"view"}}},
	}, map[string]bool{"rules": true})
	if !ok || status != 0 || data != nil {
		t.Fatalf("valid rules status=%d ok=%v data=%#v, want replacement", status, ok, data)
	}

	deleteReq := policyRequest(http.MethodDelete, "/api/v1/admin/proxy-rbac/policies/"+policyID, "", "ADMIN")
	deleteReq.SetPathValue("id", policyID)
	code, data, _ = deletePolicy(app, deleteReq, platform.RouteSpec{})
	assertPolicyStatus(t, code, data, http.StatusOK)
	code, data, _ = getPolicy(app, deleteReq, platform.RouteSpec{})
	assertPolicyStatus(t, code, data, http.StatusNotFound)
}

func TestProxyPolicyBatchAndLegacyAssignmentHandlers(t *testing.T) {
	app := newPolicyTestApp(t)
	policyID := createPolicyForTest(t, app)

	emptyBatch := policyRequest(http.MethodPost, "/api/v1/admin/proxy-rbac/policies/"+policyID+"/assignments/batch", `{"assignments":[]}`, "ADMIN")
	emptyBatch.SetPathValue("id", policyID)
	code, data, _ := batchAssignPolicy(app, emptyBatch, platform.RouteSpec{})
	assertPolicyStatus(t, code, data, http.StatusBadRequest)

	batchReq := policyRequest(http.MethodPost, "/api/v1/admin/proxy-rbac/policies/"+policyID+"/assignments/batch", `{"assignments":[{"target_type":"user","target_id":"U1"},{"target_type":"role","target_id":"RL2600001"}]}`, "ADMIN")
	batchReq.SetPathValue("id", policyID)
	code, data, _ = batchAssignPolicy(app, batchReq, platform.RouteSpec{})
	assertPolicyStatus(t, code, data, http.StatusOK)
	if result := data.(map[string]any); result["succeeded"] != 2 || result["failed"] != 0 {
		t.Fatalf("batch assignment result = %#v, want two successes", result)
	}

	legacyBody := `{"policy_id":"` + policyID + `","target_type":"user","target_id":"U2"}`
	legacyReq := policyRequest(http.MethodPost, "/api/v1/admin/proxy-rbac/assignments", legacyBody, "ADMIN")
	code, data, _ = assignPolicyLegacy(app, legacyReq, platform.RouteSpec{})
	assertPolicyStatus(t, code, data, http.StatusCreated)
	code, data, _ = assignPolicyLegacy(app, policyRequest(http.MethodPost, "/api/v1/admin/proxy-rbac/assignments", legacyBody, "ADMIN"), platform.RouteSpec{})
	assertPolicyStatus(t, code, data, http.StatusOK)

	targetReq := policyRequest(http.MethodGet, "/api/v1/admin/proxy-rbac/targets/team/U2/assignments", "", "ADMIN")
	targetReq.SetPathValue("type", "team")
	targetReq.SetPathValue("id", "U2")
	code, data, _ = listTargetAssignments(app, targetReq, platform.RouteSpec{})
	assertPolicyStatus(t, code, data, http.StatusBadRequest)

	assignment := composeAssignment(app, batchReq, map[string]any{"policy_id": policyID})
	if assignment["policy"] == nil {
		t.Fatalf("composed assignment = %#v, want embedded policy", assignment)
	}
	policy := composePolicy(app, batchReq, map[string]any{"id": policyID})
	if _, ok := policy["rules"].([]map[string]any); !ok {
		t.Fatalf("composed policy = %#v, want rules", policy)
	}
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

func TestProxyRoleLegacyAndHelperHandlers(t *testing.T) {
	app := newPolicyTestApp(t)
	roleID := createRoleForTest(t, app)

	code, data, _ := listPlatformRolesLegacy(app, policyRequest(http.MethodGet, "/api/v1/admin/proxy-rbac/platform-roles", "", "ADMIN"), platform.RouteSpec{})
	assertPolicyStatus(t, code, data, http.StatusOK)
	if roles := data.([]map[string]any); len(roles) < 2 {
		t.Fatalf("legacy roles = %#v, want seeded roles", roles)
	}

	getReq := policyRequest(http.MethodGet, "/api/v1/admin/proxy-rbac/roles/"+roleID, "", "ADMIN")
	getReq.SetPathValue("id", roleID)
	code, data, _ = getRole(app, getReq, platform.RouteSpec{})
	assertPolicyStatus(t, code, data, http.StatusOK)

	missingReq := policyRequest(http.MethodGet, "/api/v1/admin/proxy-rbac/roles/missing", "", "ADMIN")
	missingReq.SetPathValue("id", "missing")
	code, data, _ = getRole(app, missingReq, platform.RouteSpec{})
	assertPolicyStatus(t, code, data, http.StatusNotFound)

	legacyBody := `{"role_id":"` + roleID + `","user_id":"U2"}`
	legacyReq := policyRequest(http.MethodPost, "/api/v1/admin/proxy-rbac/role-users", legacyBody, "ADMIN")
	code, data, _ = assignRoleUserLegacy(app, legacyReq, platform.RouteSpec{})
	assertPolicyStatus(t, code, data, http.StatusCreated)
	code, data, _ = assignRoleUserLegacy(app, policyRequest(http.MethodPost, "/api/v1/admin/proxy-rbac/role-users", legacyBody, "ADMIN"), platform.RouteSpec{})
	assertPolicyStatus(t, code, data, http.StatusOK)

	batchReq := policyRequest(http.MethodPost, "/api/v1/admin/proxy-rbac/roles/"+roleID+"/users/batch", `{"user_ids":["","U1"]}`, "ADMIN")
	batchReq.SetPathValue("id", roleID)
	code, data, _ = batchAssignRoleUsers(app, batchReq, platform.RouteSpec{})
	assertPolicyStatus(t, code, data, http.StatusOK)
	if result := data.(map[string]any); result["succeeded"] != 1 || result["failed"] != 0 {
		t.Fatalf("batch role users = %#v, want one normalized success", result)
	}
	result := map[string]any{"succeeded": 0, "failed": 0, "errors": []string{}}
	batchFailure(result, msgUserIDRequired)
	if result["failed"] != 1 || len(result["errors"].([]string)) != 1 {
		t.Fatalf("batch failure result = %#v, want one recorded error", result)
	}

	member := composeRoleUser(app, batchReq, map[string]any{"role_id": roleID, "user_id": "U1"})
	if member["role"] == nil {
		t.Fatalf("composed role member = %#v, want embedded role", member)
	}
	publishProxyPolicyChanged(app, batchReq, "role_helper", nil)
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
