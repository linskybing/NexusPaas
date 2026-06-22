package services

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

func TestAuthorizationPolicyRawPermissionPolicyCRUDAndSimulate(t *testing.T) {
	app := newTestApp()
	seedAuthorizationPolicyUsers(t, app)

	requestJSON(t, app, http.MethodPost, "/api/v1/permissions/policy", `{`, adminHeaders("ADMIN"), http.StatusBadRequest)
	requestJSON(t, app, http.MethodPost, "/api/v1/permissions/policy", `["alice","proj"]`, adminHeaders("ADMIN"), http.StatusBadRequest)
	requestJSON(t, app, http.MethodPost, "/api/v1/permissions/policy", `["alice","proj-1","model","read"]`, userHeaders("U1"), http.StatusForbidden)
	requestJSON(t, app, http.MethodPost, "/api/v1/permissions/policy", `["alice","proj-1","model","read"]`, adminHeaders("CAPONLY"), http.StatusForbidden)

	assertNoData(t, requestJSON(t, app, http.MethodPost, "/api/v1/permissions/policy", `["alice","proj-1","model","read"]`, adminHeaders("ADMIN"), http.StatusOK))
	requestJSON(t, app, http.MethodPost, "/api/v1/permissions/policy", `["alice","proj-1","model","read"]`, adminHeaders("ADMIN"), http.StatusConflict)

	policies := responseSlice(t, requestJSON(t, app, http.MethodGet, "/api/v1/permissions/policy", "", adminHeaders("ADMIN"), http.StatusOK))
	if len(policies) != 1 || policies[0].([]any)[0] != "alice" || policies[0].([]any)[3] != "read" {
		t.Fatalf("raw policies = %#v", policies)
	}

	allowed := responseMap(t, requestJSON(t, app, http.MethodPost, "/api/v1/permissions/simulate", `{"sub":"alice","dom":"proj-1","obj":"model","act":"read"}`, adminHeaders("ADMIN"), http.StatusOK))
	if allowed["allowed"] != true {
		t.Fatalf("simulate allowed = %#v", allowed)
	}
	denied := responseMap(t, requestJSON(t, app, http.MethodPost, "/api/v1/permissions/simulate", `{"sub":"alice","dom":"proj-1","obj":"model","act":"write"}`, adminHeaders("ADMIN"), http.StatusOK))
	if denied["allowed"] != false {
		t.Fatalf("simulate denied = %#v", denied)
	}
	requestJSON(t, app, http.MethodPost, "/api/v1/permissions/simulate", `{"sub":"alice","dom":"proj-1","obj":"model"}`, adminHeaders("ADMIN"), http.StatusBadRequest)
	requestJSON(t, app, http.MethodPost, "/api/v1/permissions/simulate", `{"sub":"alice","dom":"proj-1","obj":"model","act":"read"}`, userHeaders("U1"), http.StatusForbidden)

	requestJSON(t, app, http.MethodPut, "/api/v1/permissions/policy", `{}`, adminHeaders("ADMIN"), http.StatusBadRequest)
	requestJSON(t, app, http.MethodPut, "/api/v1/permissions/policy", `{"old":["alice","proj-1","missing","read"],"new":["alice","proj-1","missing","write"]}`, adminHeaders("ADMIN"), http.StatusNotFound)
	assertNoData(t, requestJSON(t, app, http.MethodPut, "/api/v1/permissions/policy", `{"old":["alice","proj-1","model","read"],"new":["alice","proj-1","model","write"]}`, adminHeaders("ADMIN"), http.StatusOK))
	allowed = responseMap(t, requestJSON(t, app, http.MethodPost, "/api/v1/permissions/simulate", `{"sub":"alice","dom":"proj-1","obj":"model","act":"write"}`, adminHeaders("ADMIN"), http.StatusOK))
	if allowed["allowed"] != true {
		t.Fatalf("updated policy did not enforce: %#v", allowed)
	}
	denied = responseMap(t, requestJSON(t, app, http.MethodPost, "/api/v1/permissions/simulate", `{"sub":"alice","dom":"proj-1","obj":"model","act":"read"}`, adminHeaders("ADMIN"), http.StatusOK))
	if denied["allowed"] != false {
		t.Fatalf("old policy still enforced: %#v", denied)
	}

	requestJSON(t, app, http.MethodDelete, "/api/v1/permissions/policy", `{}`, adminHeaders("ADMIN"), http.StatusBadRequest)
	requestJSON(t, app, http.MethodDelete, "/api/v1/permissions/policy", `["alice","proj-1","missing","write"]`, adminHeaders("ADMIN"), http.StatusNotFound)
	assertNoData(t, requestJSON(t, app, http.MethodDelete, "/api/v1/permissions/policy", `["alice","proj-1","model","write"]`, adminHeaders("ADMIN"), http.StatusOK))
	if len(app.Store.List(t.Context(), "authorization-policy-service:permission_policies")) != 0 {
		t.Fatalf("raw policy should be removed")
	}
	if countEvents(app, "PolicyChanged") < 3 {
		t.Fatalf("expected PolicyChanged events for add/update/remove")
	}
}

func TestAuthorizationPolicyEnforceEndpointRequiresServicePrincipal(t *testing.T) {
	app := platform.NewApp(platform.Config{
		ServiceName:   "all",
		HTTPAddr:      ":0",
		RequireAuth:   true,
		ServiceAPIKey: "service-key",
		APIKeys: map[string]bool{
			"service-key": true,
			"user-key":    true,
		},
		APIKeyPrincipals: map[string]platform.APIKeyPrincipal{
			"service-key": {ID: "svc:identity", Username: "identity-service", Role: "service", Scopes: []string{"authorization-policy-service:write"}},
			"user-key":    {ID: "alice", Username: "alice", Role: "user", Scopes: []string{"authorization-policy-service:write"}},
		},
		ExternalURLs: map[string]string{},
	})
	RegisterAll(app)
	body := `{"sub":"alice","dom":"proj-1","obj":"model","act":"read"}`

	requestJSON(t, app, http.MethodPost, "/api/v1/permissions/enforce", body, nil, http.StatusUnauthorized)
	requestJSON(t, app, http.MethodPost, "/api/v1/permissions/enforce", body, map[string]string{"X-API-Key": "user-key"}, http.StatusUnauthorized)
	requestJSON(t, app, http.MethodPost, "/api/v1/permissions/enforce", body, map[string]string{"X-Service-Key": "wrong"}, http.StatusUnauthorized)
	requestJSON(t, app, http.MethodPost, "/api/v1/permissions/enforce", `{"sub":"alice","dom":"proj-1","obj":"model"}`, map[string]string{"X-Service-Key": "service-key"}, http.StatusBadRequest)

	denied := responseMap(t, requestJSON(t, app, http.MethodPost, "/api/v1/permissions/enforce", body, map[string]string{"X-Service-Key": "service-key"}, http.StatusOK))
	if denied["allowed"] != false {
		t.Fatalf("enforce denied = %#v, want allowed=false", denied)
	}
	allowRawPolicy(t, app, "alice", "proj-1", "model", "read")
	allowed := responseMap(t, requestJSON(t, app, http.MethodPost, "/api/v1/permissions/enforce", body, map[string]string{"X-Service-Key": "service-key"}, http.StatusOK))
	if allowed["allowed"] != true {
		t.Fatalf("enforce allowed = %#v, want allowed=true", allowed)
	}
}

func TestAuthorizationPolicyPDPWiresPlatformAuthorization(t *testing.T) {
	app := platform.NewApp(platform.Config{
		ServiceName:  "all",
		HTTPAddr:     ":0",
		RequireAuth:  true,
		APIKeys:      map[string]bool{"user-key": true},
		ExternalURLs: map[string]string{},
		APIKeyPrincipals: map[string]platform.APIKeyPrincipal{
			"user-key": {ID: "alice", Username: "alice", Role: "user"},
		},
	})
	RegisterAll(app)
	if err := app.ValidatePolicyDecisionPoint(); err != nil {
		t.Fatalf("authorization-policy service did not install platform PDP: %v", err)
	}
	if _, err := app.Store.Create(t.Context(), "identity-service:users", map[string]any{"id": "U1", "username": "target", "role": "user", "status": "online"}); err != nil {
		t.Fatal(err)
	}

	headers := map[string]string{"X-API-Key": "user-key"}
	requestJSON(t, app, http.MethodGet, "/api/v1/users/U1", "", headers, http.StatusForbidden)

	allowPolicyForRoute(t, app, "alice", "", http.MethodGet, "/api/v1/users/{id}")

	user := responseMap(t, requestJSON(t, app, http.MethodGet, "/api/v1/users/U1", "", headers, http.StatusOK))
	if user["id"] != "U1" {
		t.Fatalf("authorized user response = %#v, want U1", user)
	}
}

func TestRemoteAuthorizationPolicyPDPAuthorizesIsolatedService(t *testing.T) {
	policyApp := platform.NewApp(platform.Config{
		ServiceName:   "authorization-policy-service",
		HTTPAddr:      ":0",
		RequireAuth:   true,
		ServiceAPIKey: "pdp-key",
		APIKeys:       map[string]bool{"pdp-key": true},
		APIKeyPrincipals: map[string]platform.APIKeyPrincipal{
			"pdp-key": {ID: "svc:identity", Username: "identity-service", Role: "service", Scopes: []string{"authorization-policy-service:write"}},
		},
		ExternalURLs: map[string]string{},
	})
	RegisterAll(policyApp)
	server := httptest.NewServer(policyApp)
	defer server.Close()

	identityApp := platform.NewApp(platform.Config{
		ServiceName:               "identity-service",
		HTTPAddr:                  ":0",
		RequireAuth:               true,
		AuthorizationPolicyURL:    server.URL,
		AuthorizationPolicyAPIKey: "pdp-key",
		APIKeys:                   map[string]bool{"user-key": true},
		APIKeyPrincipals:          map[string]platform.APIKeyPrincipal{"user-key": {ID: "alice", Username: "alice", Role: "user"}},
		ExternalURLs:              map[string]string{},
	})
	RegisterAll(identityApp)
	if err := identityApp.ValidatePolicyDecisionPoint(); err != nil {
		t.Fatalf("isolated service did not install remote PDP: %v", err)
	}
	if _, err := identityApp.Store.Create(t.Context(), "identity-service:users", map[string]any{"id": "U1", "username": "target", "role": "user", "status": "online"}); err != nil {
		t.Fatal(err)
	}

	headers := map[string]string{"X-API-Key": "user-key"}
	requestJSON(t, identityApp, http.MethodGet, "/api/v1/users/U1", "", headers, http.StatusForbidden)

	route := routeByMethodAndPattern(t, identityApp, http.MethodGet, "/api/v1/users/{id}")
	allowRawPolicy(t, policyApp, "alice", "", route.Resource, route.OperationID)
	user := responseMap(t, requestJSON(t, identityApp, http.MethodGet, "/api/v1/users/U1", "", headers, http.StatusOK))
	if user["id"] != "U1" {
		t.Fatalf("remote-PDP authorized user response = %#v, want U1", user)
	}
}

func allowPolicyForRoute(t *testing.T, app *platform.App, subject, domain, method, pattern string) {
	t.Helper()
	route := routeByMethodAndPattern(t, app, method, pattern)
	allowRawPolicy(t, app, subject, domain, route.Resource, route.OperationID)
}

func allowRawPolicy(t *testing.T, app *platform.App, subject, domain, object, action string) {
	t.Helper()
	policyID := strings.Join([]string{subject, domain, object, action}, "\x1f")
	if _, err := app.Store.Create(t.Context(), "authorization-policy-service:permission_policies", map[string]any{"id": policyID}); err != nil && !platform.IsCreateConflict(err) {
		t.Fatal(err)
	}
}

func routeByMethodAndPattern(t *testing.T, app *platform.App, method, pattern string) platform.RouteSpec {
	t.Helper()
	for _, route := range app.Routes {
		if route.Method == method && route.Pattern == pattern {
			return route
		}
	}
	t.Fatalf("missing route %s %s", method, pattern)
	return platform.RouteSpec{}
}

func TestAuthorizationPolicyPermissionBatch(t *testing.T) {
	app := newTestApp()
	seedAuthorizationPolicyUsers(t, app)

	requestJSON(t, app, http.MethodPost, "/api/v1/permissions/batch", `{}`, adminHeaders("ADMIN"), http.StatusBadRequest)
	requestJSON(t, app, http.MethodPost, "/api/v1/permissions/batch", `{"operations":[]}`, adminHeaders("ADMIN"), http.StatusBadRequest)
	requestJSON(t, app, http.MethodPost, "/api/v1/permissions/batch", `{"operations":[{"type":"project_member","action":"add","project_id":"P1","user_id":"U1","role":"user"}]}`, userHeaders("U1"), http.StatusForbidden)
	requestJSON(t, app, http.MethodPost, "/api/v1/permissions/batch", `{"operations":[{"type":"unknown_type","action":"add","user_id":"U1"}]}`, adminHeaders("ADMIN"), http.StatusInternalServerError)

	assertNoData(t, requestJSON(t, app, http.MethodPost, "/api/v1/permissions/batch", `{"operations":[{"type":"project_member","action":"add","project_id":"P1","user_id":"U1","role":"user"},{"type":"group_role","action":"add","group_id":"G1","user_id":"U2","role":"admin"}]}`, adminHeaders("ADMIN"), http.StatusOK))
	if len(app.Store.List(t.Context(), "authorization-policy-service:permission_grouping_policies")) != 2 {
		t.Fatalf("expected two grouping policies after batch add")
	}
	assertNoData(t, requestJSON(t, app, http.MethodPost, "/api/v1/permissions/batch", `{"operations":[{"type":"group_role","action":"remove","group_id":"G1","user_id":"U2","role":"admin"}]}`, adminHeaders("ADMIN"), http.StatusOK))
	groupings := app.Store.List(t.Context(), "authorization-policy-service:permission_grouping_policies")
	if len(groupings) != 1 || groupings[0].Data["type"] != "project_member" {
		t.Fatalf("grouping policies after remove = %#v", groupings)
	}
}

func TestAuthorizationPolicyProxyServices(t *testing.T) {
	app := newTestApp()
	seedAuthorizationPolicyUsers(t, app)

	requestJSON(t, app, http.MethodGet, "/api/v1/admin/proxy-rbac/services", "", nil, http.StatusUnauthorized)
	requestJSON(t, app, http.MethodGet, "/api/v1/admin/proxy-rbac/services", "", adminHeaders("forged"), http.StatusForbidden)
	requestJSON(t, app, http.MethodGet, "/api/v1/admin/proxy-rbac/services", "", adminHeaders("CAPONLY"), http.StatusForbidden)
	requestJSON(t, app, http.MethodGet, "/api/v1/admin/proxy-rbac/services", "", userHeaders("U1"), http.StatusForbidden)

	services := responseSlice(t, requestJSON(t, app, http.MethodGet, "/api/v1/admin/proxy-rbac/services", "", adminHeaders("ADMIN"), http.StatusOK))
	if len(services) != 8 || services[0].(map[string]any)["id"] != "SVC_MINIO" {
		t.Fatalf("services = %#v, want seeded proxy service definitions sorted by sort_order", services)
	}
	requestJSON(t, app, http.MethodPost, "/api/v1/admin/proxy-rbac/services", `{"id":"SVC_ATTACK","api_patterns":["/api/v1/*"],"actions":["delete"]}`, userHeaders("U1"), http.StatusForbidden)
	requestJSON(t, app, http.MethodPost, "/api/v1/admin/proxy-rbac/services", `{"id":"SVC_ATTACK","api_patterns":["/api/v1/*"],"actions":["delete"]}`, adminHeaders("ADMIN"), http.StatusMethodNotAllowed)
	if _, found := app.Store.Get(t.Context(), "authorization-policy-service:proxy_services", "SVC_ATTACK"); found {
		t.Fatal("legacy proxy service create wrote unvalidated service definition")
	}

	minio := responseMap(t, requestJSON(t, app, http.MethodGet, "/api/v1/admin/proxy-rbac/services/SVC_MINIO", "", adminHeaders("ADMIN"), http.StatusOK))
	if minio["name"] != "MinIO Console" || len(minio["actions"].([]any)) != 4 {
		t.Fatalf("minio service = %#v", minio)
	}
	requestJSON(t, app, http.MethodGet, "/api/v1/admin/proxy-rbac/services/MISSING", "", adminHeaders("ADMIN"), http.StatusNotFound)
}

func TestAuthorizationPolicyLegacyProxyRBACRoutesAreRemoved(t *testing.T) {
	app := newTestApp()
	if app.CustomHandlers["POST /api/v1/admin/proxy-rbac/services"] == nil {
		t.Fatal("canonical proxy-rbac service create handler missing")
	}
	for _, key := range []string{
		"POST /api/v1/admin/proxy-rbac/assignments",
		"GET /api/v1/admin/proxy-rbac/platform-roles",
		"POST /api/v1/admin/proxy-rbac/role-users",
	} {
		if app.CustomHandlers[key] != nil {
			t.Fatalf("%s still has a legacy custom handler", key)
		}
	}
}

func requestRemovedRoute(t *testing.T, app http.Handler, method, path, body string, headers map[string]string) {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound && rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("%s %s returned %d, want removed route status: %s", method, path, rec.Code, rec.Body.String())
	}
}

func TestAuthorizationPolicyProxyPolicyCRUD(t *testing.T) {
	app := newTestApp()
	seedAuthorizationPolicyUsers(t, app)

	requestJSON(t, app, http.MethodPost, "/api/v1/admin/proxy-rbac/policies", `{"name":"x"}`, userHeaders("U1"), http.StatusForbidden)
	requestJSON(t, app, http.MethodPost, "/api/v1/admin/proxy-rbac/policies", `{`, adminHeaders("ADMIN"), http.StatusBadRequest)
	requestJSON(t, app, http.MethodPost, "/api/v1/admin/proxy-rbac/policies", `{"description":"missing name"}`, adminHeaders("ADMIN"), http.StatusBadRequest)
	requestJSON(t, app, http.MethodPost, "/api/v1/admin/proxy-rbac/policies", `{"name":"full-proxy-access"}`, adminHeaders("ADMIN"), http.StatusBadRequest)
	policiesBefore := len(app.Store.List(t.Context(), "authorization-policy-service:proxy_policies"))
	eventsBefore := countEvents(app, "ProxyPolicyChanged")
	requestJSON(t, app, http.MethodPost, "/api/v1/admin/proxy-rbac/policies", `{"id":"PO2600001","name":"duplicate-id-policy"}`, adminHeaders("ADMIN"), http.StatusConflict)
	if policiesAfter := len(app.Store.List(t.Context(), "authorization-policy-service:proxy_policies")); policiesAfter != policiesBefore {
		t.Fatalf("policy count = %d, want unchanged %d", policiesAfter, policiesBefore)
	}
	if eventsAfter := countEvents(app, "ProxyPolicyChanged"); eventsAfter != eventsBefore {
		t.Fatalf("ProxyPolicyChanged events = %d, want unchanged %d", eventsAfter, eventsBefore)
	}

	created := responseMap(t, requestJSON(t, app, http.MethodPost, "/api/v1/admin/proxy-rbac/policies", `{"name":"science-proxy","description":"science team","rules":[{"service_id":"SVC_GRAFANA","actions":["view"]},{"service_id":"SVC_MINIO","actions":["view","create"]}]}`, adminHeaders("ADMIN"), http.StatusCreated))
	id := created["id"].(string)
	if id == "" || created["name"] != "science-proxy" || len(created["rules"].([]any)) != 2 {
		t.Fatalf("created policy = %#v", created)
	}

	policies := responseSlice(t, requestJSON(t, app, http.MethodGet, "/api/v1/admin/proxy-rbac/policies", "", adminHeaders("ADMIN"), http.StatusOK))
	if len(policies) != 3 {
		t.Fatalf("policies = %#v, want two seeded policies plus created policy", policies)
	}

	fetched := responseMap(t, requestJSON(t, app, http.MethodGet, "/api/v1/admin/proxy-rbac/policies/"+id, "", adminHeaders("ADMIN"), http.StatusOK))
	if fetched["description"] != "science team" || len(fetched["rules"].([]any)) != 2 {
		t.Fatalf("fetched policy = %#v", fetched)
	}

	updated := responseMap(t, requestJSON(t, app, http.MethodPut, "/api/v1/admin/proxy-rbac/policies/"+id, `{"name":"science-proxy-read","description":"read only"}`, adminHeaders("ADMIN"), http.StatusOK))
	if updated["name"] != "science-proxy-read" || updated["description"] != "read only" || len(updated["rules"].([]any)) != 2 {
		t.Fatalf("updated policy without rules should retain rules: %#v", updated)
	}

	updated = responseMap(t, requestJSON(t, app, http.MethodPut, "/api/v1/admin/proxy-rbac/policies/"+id, `{"description":"cleared","rules":[]}`, adminHeaders("ADMIN"), http.StatusOK))
	if updated["description"] != "cleared" || len(updated["rules"].([]any)) != 0 {
		t.Fatalf("updated policy with empty rules should clear rules: %#v", updated)
	}
	requestJSON(t, app, http.MethodPut, "/api/v1/admin/proxy-rbac/policies/MISSING", `{"description":"missing"}`, adminHeaders("ADMIN"), http.StatusNotFound)

	requestJSON(t, app, http.MethodDelete, "/api/v1/admin/proxy-rbac/policies/"+id, "", adminHeaders("ADMIN"), http.StatusOK)
	requestJSON(t, app, http.MethodGet, "/api/v1/admin/proxy-rbac/policies/"+id, "", adminHeaders("ADMIN"), http.StatusNotFound)
	if len(app.Store.List(t.Context(), "authorization-policy-service:proxy_policy_rules")) != 16 {
		t.Fatalf("policy rule cleanup left unexpected rule count")
	}
	if len(app.Events.Outbox()) == 0 {
		t.Fatal("expected ProxyPolicyChanged events for policy mutations")
	}
}

func TestAuthorizationPolicyProxyAssignments(t *testing.T) {
	app := newTestApp()
	seedAuthorizationPolicyUsers(t, app)

	assertDefaultProxyAssignment(t, app)
	policyID, assigned := createProxyAssignmentPolicyAndUserAssignment(t, app)
	assertDuplicateProxyAssignmentNoSideEffects(t, app, policyID, assigned)
	assertLegacyProxyAssignmentDoesNotSplitBrain(t, app, policyID)
	assertTargetAssignmentRemoval(t, app, policyID)
	assertBatchProxyAssignmentsAndCascade(t, app, policyID)
}

func assertDefaultProxyAssignment(t *testing.T, app *platform.App) {
	t.Helper()

	requestJSON(t, app, http.MethodGet, "/api/v1/admin/proxy-rbac/policies/PO2600001/assignments", "", userHeaders("U1"), http.StatusForbidden)
	defaultAssignments := responseSlice(t, requestJSON(t, app, http.MethodGet, "/api/v1/admin/proxy-rbac/policies/PO2600001/assignments", "", adminHeaders("ADMIN"), http.StatusOK))
	if len(defaultAssignments) != 1 {
		t.Fatalf("default assignments = %#v, want full proxy policy assigned to proxy operator", defaultAssignments)
	}
	defaultAssignment := defaultAssignments[0].(map[string]any)
	if defaultAssignment["target_id"] != "RL2600001" {
		t.Fatalf("default assignment = %#v", defaultAssignment)
	}
	if policy := defaultAssignment["policy"].(map[string]any); len(policy["rules"].([]any)) == 0 {
		t.Fatalf("default assignment should include composed policy rules: %#v", defaultAssignment)
	}
}

func createProxyAssignmentPolicyAndUserAssignment(t *testing.T, app *platform.App) (string, map[string]any) {
	t.Helper()

	createdPolicy := responseMap(t, requestJSON(t, app, http.MethodPost, "/api/v1/admin/proxy-rbac/policies", `{"name":"assignment-policy","description":"for assignment tests"}`, adminHeaders("ADMIN"), http.StatusCreated))
	policyID := createdPolicy["id"].(string)
	requestJSON(t, app, http.MethodPost, "/api/v1/admin/proxy-rbac/policies/"+policyID+"/assignments", `{"target_type":"team","target_id":"G1"}`, adminHeaders("ADMIN"), http.StatusBadRequest)
	requestJSON(t, app, http.MethodPost, "/api/v1/admin/proxy-rbac/policies/MISSING/assignments", `{"target_type":"user","target_id":"U1"}`, adminHeaders("ADMIN"), http.StatusNotFound)

	assigned := responseMap(t, requestJSON(t, app, http.MethodPost, "/api/v1/admin/proxy-rbac/policies/"+policyID+"/assignments", `{"target_type":"user","target_id":"U1"}`, adminHeaders("ADMIN"), http.StatusCreated))
	if assigned["policy_id"] != policyID || assigned["target_type"] != "user" || assigned["target_id"] != "U1" {
		t.Fatalf("assigned policy = %#v", assigned)
	}
	return policyID, assigned
}

func assertDuplicateProxyAssignmentNoSideEffects(t *testing.T, app *platform.App, policyID string, assigned map[string]any) {
	t.Helper()

	assignmentsBefore := len(app.Store.List(t.Context(), "authorization-policy-service:proxy_policy_assignments"))
	eventsBefore := countEvents(app, "ProxyPolicyChanged")
	duplicateAssigned := responseMap(t, requestJSON(t, app, http.MethodPost, "/api/v1/admin/proxy-rbac/policies/"+policyID+"/assignments", `{"target_type":"user","target_id":"U1"}`, adminHeaders("ADMIN"), http.StatusOK))
	if duplicateAssigned["id"] != assigned["id"] {
		t.Fatalf("duplicate assignment = %#v, want existing %#v", duplicateAssigned, assigned)
	}
	if assignmentsAfter := len(app.Store.List(t.Context(), "authorization-policy-service:proxy_policy_assignments")); assignmentsAfter != assignmentsBefore {
		t.Fatalf("assignment count = %d, want unchanged %d", assignmentsAfter, assignmentsBefore)
	}
	if eventsAfter := countEvents(app, "ProxyPolicyChanged"); eventsAfter != eventsBefore {
		t.Fatalf("ProxyPolicyChanged events = %d, want unchanged %d", eventsAfter, eventsBefore)
	}
}

func assertLegacyProxyAssignmentDoesNotSplitBrain(t *testing.T, app *platform.App, policyID string) {
	t.Helper()

	requestRemovedRoute(t, app, http.MethodPost, "/api/v1/admin/proxy-rbac/assignments", `{"policy_id":"`+policyID+`","target_type":"user","target_id":"U2"}`, userHeaders("U1"))
	requestRemovedRoute(t, app, http.MethodPost, "/api/v1/admin/proxy-rbac/assignments", `{"policy_id":"`+policyID+`","target_type":"user","target_id":"U2"}`, adminHeaders("ADMIN"))
	if len(app.Store.List(t.Context(), "authorization-policy-service:proxy_assignments")) != 0 {
		t.Fatal("removed legacy assignment route wrote split-brain proxy_assignments resource")
	}
}

func assertTargetAssignmentRemoval(t *testing.T, app *platform.App, policyID string) {
	t.Helper()

	targetAssignments := responseSlice(t, requestJSON(t, app, http.MethodGet, "/api/v1/admin/proxy-rbac/targets/user/U1/assignments", "", adminHeaders("ADMIN"), http.StatusOK))
	if len(targetAssignments) != 1 || targetAssignments[0].(map[string]any)["policy_id"] != policyID {
		t.Fatalf("target assignments = %#v", targetAssignments)
	}

	requestJSON(t, app, http.MethodDelete, "/api/v1/admin/proxy-rbac/policies/"+policyID+"/assignments", `{"target_type":"user","target_id":"U1"}`, adminHeaders("ADMIN"), http.StatusOK)
	targetAssignments = responseSlice(t, requestJSON(t, app, http.MethodGet, "/api/v1/admin/proxy-rbac/targets/user/U1/assignments", "", adminHeaders("ADMIN"), http.StatusOK))
	if len(targetAssignments) != 0 {
		t.Fatalf("target assignments after unassign = %#v", targetAssignments)
	}
}

func assertBatchProxyAssignmentsAndCascade(t *testing.T, app *platform.App, policyID string) {
	t.Helper()

	requestJSON(t, app, http.MethodPost, "/api/v1/admin/proxy-rbac/policies/"+policyID+"/assignments/batch", `{"assignments":[{"target_type":"team","target_id":"G1"}]}`, adminHeaders("ADMIN"), http.StatusBadRequest)
	batch := responseMap(t, requestJSON(t, app, http.MethodPost, "/api/v1/admin/proxy-rbac/policies/"+policyID+"/assignments/batch", `{"assignments":[{"target_type":"role","target_id":"RL2600001"},{"target_type":"user","target_id":"U1"}]}`, adminHeaders("ADMIN"), http.StatusOK))
	if int(batch["succeeded"].(float64)) != 2 || int(batch["failed"].(float64)) != 0 {
		t.Fatalf("batch assignment result = %#v", batch)
	}
	policyAssignments := responseSlice(t, requestJSON(t, app, http.MethodGet, "/api/v1/admin/proxy-rbac/policies/"+policyID+"/assignments", "", adminHeaders("ADMIN"), http.StatusOK))
	if len(policyAssignments) != 2 {
		t.Fatalf("policy assignments = %#v", policyAssignments)
	}
	assignedTargets := map[string]bool{}
	for _, assignment := range policyAssignments {
		item := assignment.(map[string]any)
		assignedTargets[item["target_type"].(string)+":"+item["target_id"].(string)] = true
	}
	if !assignedTargets["role:RL2600001"] || !assignedTargets["user:U1"] {
		t.Fatalf("policy assignments = %#v, want role and user batch targets", policyAssignments)
	}
	requestJSON(t, app, http.MethodDelete, "/api/v1/admin/proxy-rbac/policies/"+policyID, "", adminHeaders("ADMIN"), http.StatusOK)
	if len(app.Store.List(t.Context(), "authorization-policy-service:proxy_policy_assignments")) != 2 {
		t.Fatalf("policy delete should cascade custom assignments while preserving seeded assignments")
	}
}

func TestAuthorizationPolicyDefaultAssignmentSeedSkipsDeletedDefaults(t *testing.T) {
	app := newTestApp()
	seedAuthorizationPolicyUsers(t, app)

	requestJSON(t, app, http.MethodDelete, "/api/v1/admin/proxy-rbac/policies/PO2600001", "", adminHeaders("ADMIN"), http.StatusOK)
	assignments := responseSlice(t, requestJSON(t, app, http.MethodGet, "/api/v1/admin/proxy-rbac/targets/role/RL2600001/assignments", "", adminHeaders("ADMIN"), http.StatusOK))
	if len(assignments) != 0 {
		t.Fatalf("deleted default policy assignment was re-seeded: %#v", assignments)
	}
	viewerAssignments := responseSlice(t, requestJSON(t, app, http.MethodGet, "/api/v1/admin/proxy-rbac/targets/role/RL2600002/assignments", "", adminHeaders("ADMIN"), http.StatusOK))
	if len(viewerAssignments) != 1 || viewerAssignments[0].(map[string]any)["policy_id"] != "PO2600002" {
		t.Fatalf("remaining default assignment = %#v", viewerAssignments)
	}

	requestJSON(t, app, http.MethodDelete, "/api/v1/admin/proxy-rbac/policies/PO2600002", "", adminHeaders("ADMIN"), http.StatusOK)
	allAssignments := responseSlice(t, requestJSON(t, app, http.MethodGet, "/api/v1/admin/proxy-rbac/targets/role/RL2600002/assignments", "", adminHeaders("ADMIN"), http.StatusOK))
	if len(allAssignments) != 0 {
		t.Fatalf("deleted default policies were re-seeded after policy table became empty: %#v", allAssignments)
	}
}

func TestAuthorizationPolicyProxyRoles(t *testing.T) {
	app := newTestApp()
	seedAuthorizationPolicyUsers(t, app)

	assertProxyRoleCatalog(t, app)
	assertProxyRoleCreateValidationNoSideEffects(t, app)
	roleID := createAndUpdateProxyRole(t, app)
	assertProxyRoleUserAssignments(t, app, roleID)
	assertProxyRoleSystemRoles(t, app)
	assertProxyRoleDeleteCleansUsers(t, app, roleID)
}

func assertProxyRoleCatalog(t *testing.T, app *platform.App) {
	t.Helper()

	requestJSON(t, app, http.MethodGet, "/api/v1/admin/proxy-rbac/roles", "", userHeaders("U1"), http.StatusForbidden)
	roles := responseSlice(t, requestJSON(t, app, http.MethodGet, "/api/v1/admin/proxy-rbac/roles", "", adminHeaders("ADMIN"), http.StatusOK))
	if len(roles) != 2 || roles[0].(map[string]any)["id"] != "RL2600001" {
		t.Fatalf("roles = %#v, want seeded proxy roles sorted by name", roles)
	}
	requestRemovedRoute(t, app, http.MethodGet, "/api/v1/admin/proxy-rbac/platform-roles", "", adminHeaders("ADMIN"))
	if len(app.Store.List(t.Context(), "authorization-policy-service:platform_roles")) != 0 {
		t.Fatal("removed legacy platform-roles route read split-brain platform_roles resource")
	}
	role := responseMap(t, requestJSON(t, app, http.MethodGet, "/api/v1/admin/proxy-rbac/roles/RL2600001", "", adminHeaders("ADMIN"), http.StatusOK))
	if role["display_name"] != "Proxy Operator" {
		t.Fatalf("role = %#v", role)
	}
}

func assertProxyRoleCreateValidationNoSideEffects(t *testing.T, app *platform.App) {
	t.Helper()

	requestJSON(t, app, http.MethodPost, "/api/v1/admin/proxy-rbac/roles", `{"name":"x","display_name":"X"}`, adminHeaders("ADMIN"), http.StatusBadRequest)
	requestJSON(t, app, http.MethodPost, "/api/v1/admin/proxy-rbac/roles", `{"name":"analytics"}`, adminHeaders("ADMIN"), http.StatusBadRequest)
	requestJSON(t, app, http.MethodPost, "/api/v1/admin/proxy-rbac/roles", `{"name":"proxy-operator","display_name":"Duplicate"}`, adminHeaders("ADMIN"), http.StatusBadRequest)
	rolesBefore := len(app.Store.List(t.Context(), "authorization-policy-service:proxy_roles"))
	eventsBefore := countEvents(app, "ProxyPolicyChanged")
	requestJSON(t, app, http.MethodPost, "/api/v1/admin/proxy-rbac/roles", `{"id":"RL2600001","name":"duplicate-role-id","display_name":"Duplicate ID"}`, adminHeaders("ADMIN"), http.StatusConflict)
	if rolesAfter := len(app.Store.List(t.Context(), "authorization-policy-service:proxy_roles")); rolesAfter != rolesBefore {
		t.Fatalf("role count = %d, want unchanged %d", rolesAfter, rolesBefore)
	}
	if eventsAfter := countEvents(app, "ProxyPolicyChanged"); eventsAfter != eventsBefore {
		t.Fatalf("ProxyPolicyChanged events = %d, want unchanged %d", eventsAfter, eventsBefore)
	}
}

func createAndUpdateProxyRole(t *testing.T, app *platform.App) string {
	t.Helper()

	created := responseMap(t, requestJSON(t, app, http.MethodPost, "/api/v1/admin/proxy-rbac/roles", `{"name":"analytics","display_name":"Analytics","description":"analytics team"}`, adminHeaders("ADMIN"), http.StatusCreated))
	roleID := created["id"].(string)
	updated := responseMap(t, requestJSON(t, app, http.MethodPut, "/api/v1/admin/proxy-rbac/roles/"+roleID, `{"display_name":"Analytics Admins","description":"updated"}`, adminHeaders("ADMIN"), http.StatusOK))
	if updated["display_name"] != "Analytics Admins" || updated["description"] != "updated" {
		t.Fatalf("updated role = %#v", updated)
	}
	requestJSON(t, app, http.MethodPut, "/api/v1/admin/proxy-rbac/roles/MISSING", `{"description":"missing"}`, adminHeaders("ADMIN"), http.StatusNotFound)
	return roleID
}

func assertProxyRoleUserAssignments(t *testing.T, app *platform.App, roleID string) {
	t.Helper()

	assigned := responseMap(t, requestJSON(t, app, http.MethodPost, "/api/v1/admin/proxy-rbac/roles/"+roleID+"/users", `{"user_id":"U1"}`, adminHeaders("ADMIN"), http.StatusCreated))
	if assigned["user_id"] != "U1" || assigned["role_id"] != roleID {
		t.Fatalf("assigned role user = %#v", assigned)
	}
	assertDuplicateProxyRoleUserNoSideEffects(t, app, roleID, assigned)
	assertLegacyProxyRoleUserRouteRemoved(t, app, roleID)
	assertProxyRoleUserBatchAndRemoval(t, app, roleID)
}

func assertDuplicateProxyRoleUserNoSideEffects(t *testing.T, app *platform.App, roleID string, assigned map[string]any) {
	t.Helper()

	roleUsersBefore := len(app.Store.List(t.Context(), "authorization-policy-service:proxy_role_users"))
	eventsBefore := countEvents(app, "ProxyPolicyChanged")
	duplicateRoleUser := responseMap(t, requestJSON(t, app, http.MethodPost, "/api/v1/admin/proxy-rbac/roles/"+roleID+"/users", `{"user_id":"U1"}`, adminHeaders("ADMIN"), http.StatusOK))
	if duplicateRoleUser["id"] != assigned["id"] {
		t.Fatalf("duplicate role user = %#v, want existing %#v", duplicateRoleUser, assigned)
	}
	if roleUsersAfter := len(app.Store.List(t.Context(), "authorization-policy-service:proxy_role_users")); roleUsersAfter != roleUsersBefore {
		t.Fatalf("role user count = %d, want unchanged %d", roleUsersAfter, roleUsersBefore)
	}
	if eventsAfter := countEvents(app, "ProxyPolicyChanged"); eventsAfter != eventsBefore {
		t.Fatalf("ProxyPolicyChanged events = %d, want unchanged %d", eventsAfter, eventsBefore)
	}
}

func assertLegacyProxyRoleUserRouteRemoved(t *testing.T, app *platform.App, roleID string) {
	t.Helper()

	requestRemovedRoute(t, app, http.MethodPost, "/api/v1/admin/proxy-rbac/role-users", `{"role_id":"`+roleID+`","user_id":"U2"}`, userHeaders("U1"))
	requestRemovedRoute(t, app, http.MethodPost, "/api/v1/admin/proxy-rbac/role-users", `{"role_id":"`+roleID+`","user_id":"U2"}`, adminHeaders("ADMIN"))
	if len(app.Store.List(t.Context(), "authorization-policy-service:role_users")) != 0 {
		t.Fatal("removed legacy role-users route wrote split-brain role_users resource")
	}
}

func assertProxyRoleUserBatchAndRemoval(t *testing.T, app *platform.App, roleID string) {
	t.Helper()

	requestJSON(t, app, http.MethodPost, "/api/v1/admin/proxy-rbac/roles/"+roleID+"/users", `{}`, adminHeaders("ADMIN"), http.StatusBadRequest)
	batch := responseMap(t, requestJSON(t, app, http.MethodPost, "/api/v1/admin/proxy-rbac/roles/"+roleID+"/users/batch", `{"user_ids":["U1","U2"]}`, adminHeaders("ADMIN"), http.StatusOK))
	if int(batch["succeeded"].(float64)) != 2 || int(batch["failed"].(float64)) != 0 {
		t.Fatalf("batch role users = %#v", batch)
	}
	members := responseSlice(t, requestJSON(t, app, http.MethodGet, "/api/v1/admin/proxy-rbac/roles/"+roleID+"/users", "", adminHeaders("ADMIN"), http.StatusOK))
	if len(members) != 2 || members[0].(map[string]any)["role"].(map[string]any)["id"] != roleID {
		t.Fatalf("role members = %#v", members)
	}
	requestJSON(t, app, http.MethodDelete, "/api/v1/admin/proxy-rbac/roles/"+roleID+"/users/U1", "", adminHeaders("ADMIN"), http.StatusOK)
	members = responseSlice(t, requestJSON(t, app, http.MethodGet, "/api/v1/admin/proxy-rbac/roles/"+roleID+"/users", "", adminHeaders("ADMIN"), http.StatusOK))
	if len(members) != 1 || members[0].(map[string]any)["user_id"] != "U2" {
		t.Fatalf("role members after unassign = %#v", members)
	}
}

func assertProxyRoleSystemRoles(t *testing.T, app *platform.App) {
	t.Helper()

	systemRoles := responseSlice(t, requestJSON(t, app, http.MethodGet, "/api/v1/admin/proxy-rbac/system-roles", "", adminHeaders("ADMIN"), http.StatusOK))
	if len(systemRoles) != 2 || systemRoles[0].(map[string]any)["id"] != "RO_ADMIN" {
		t.Fatalf("system roles = %#v", systemRoles)
	}
}

func assertProxyRoleDeleteCleansUsers(t *testing.T, app *platform.App, roleID string) {
	t.Helper()

	requestJSON(t, app, http.MethodDelete, "/api/v1/admin/proxy-rbac/roles/"+roleID, "", adminHeaders("ADMIN"), http.StatusOK)
	requestJSON(t, app, http.MethodGet, "/api/v1/admin/proxy-rbac/roles/"+roleID, "", adminHeaders("ADMIN"), http.StatusNotFound)
	if len(app.Store.List(t.Context(), "authorization-policy-service:proxy_role_users")) != 0 {
		t.Fatalf("role delete should clean up role users")
	}
}

func seedAuthorizationPolicyUsers(t *testing.T, app *platform.App) {
	t.Helper()
	createRows(t, app, "identity-service:users", []map[string]any{
		{"id": "U1", "username": "alice", "role_id": "RO_USER", "capabilities": map[string]any{"adminPanel": false}},
		{"id": "ADMIN", "username": "admin", "role_id": "RO_ADMIN"},
		{"id": "CAPONLY", "username": "caponly", "role_id": "RO_USER", "capabilities": map[string]any{"adminPanel": true}},
	})
	createRows(t, app, "identity-service:roles", []map[string]any{
		{"id": "RO_ADMIN", "name": "admin", "display_name": "Admin", "capabilities": map[string]any{"adminPanel": true}},
		{"id": "RO_USER", "name": "user", "display_name": "User", "capabilities": map[string]any{"adminPanel": false}},
	})
}

func countEvents(app *platform.App, name string) int {
	count := 0
	for _, event := range app.Events.Outbox() {
		if event.Name == name {
			count++
		}
	}
	return count
}
