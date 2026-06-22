package authorizationpolicy

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

type authorizationPolicyTxStore struct {
	*platform.Store
	runInTx  int
	creates  []txRecordWrite
	updates  []txRecordWrite
	deletes  []txRecordDelete
	txEvents []contracts.Event
}

type txRecordWrite struct {
	resource string
	id       string
}

type txRecordDelete struct {
	resource string
	id       string
}

func (s *authorizationPolicyTxStore) RunInTx(ctx context.Context, fn func(platform.StoreTx) error) error {
	s.runInTx++
	return fn(&authorizationPolicyRecordingTx{store: s})
}

func (s *authorizationPolicyTxStore) resetTx() {
	s.runInTx = 0
	s.creates = nil
	s.updates = nil
	s.deletes = nil
	s.txEvents = nil
}

type authorizationPolicyRecordingTx struct {
	store *authorizationPolicyTxStore
}

func (tx *authorizationPolicyRecordingTx) Create(ctx context.Context, resource string, data map[string]any) (contracts.Record[map[string]any], error) {
	record, err := tx.store.Store.Create(ctx, resource, data)
	if err == nil {
		tx.store.creates = append(tx.store.creates, txRecordWrite{resource: resource, id: record.ID})
	}
	return record, err
}

func (tx *authorizationPolicyRecordingTx) Update(ctx context.Context, resource, id string, data map[string]any) (contracts.Record[map[string]any], bool, error) {
	record, ok := tx.store.Store.Update(ctx, resource, id, data)
	if ok {
		tx.store.updates = append(tx.store.updates, txRecordWrite{resource: resource, id: id})
	}
	return record, ok, nil
}

func (tx *authorizationPolicyRecordingTx) Delete(ctx context.Context, resource, id string) (bool, error) {
	deleted := tx.store.Store.Delete(ctx, resource, id)
	if deleted {
		tx.store.deletes = append(tx.store.deletes, txRecordDelete{resource: resource, id: id})
	}
	return deleted, nil
}

func (tx *authorizationPolicyRecordingTx) Emit(event contracts.Event) {
	tx.store.txEvents = append(tx.store.txEvents, event)
}

func TestAuthorizationPolicyRoleMutationsUseTransactionalEvents(t *testing.T) {
	app, store := newAuthorizationPolicyTxTestApp(t)

	code, data, _ := createRole(app, policyRequest(http.MethodPost, "/api/v1/admin/proxy-rbac/roles", `{"name":"analytics","display_name":"Analytics"}`, "ADMIN"), platform.RouteSpec{})
	assertPolicyStatus(t, code, data, http.StatusCreated)
	assertTxEvent(t, app, store, "ProxyPolicyChanged", "role_create")
	roleID := data.(map[string]any)["id"].(string)
	assertTxCreated(t, store, platformRolesResource, roleID)

	store.resetTx()
	updateReq := policyRequest(http.MethodPut, "/api/v1/admin/proxy-rbac/roles/"+roleID, `{"display_name":"Analytics Admins"}`, "ADMIN")
	updateReq.SetPathValue("id", roleID)
	code, data, _ = updateRole(app, updateReq, platform.RouteSpec{})
	assertPolicyStatus(t, code, data, http.StatusOK)
	assertTxEvent(t, app, store, "ProxyPolicyChanged", "role_update")
	assertTxUpdated(t, store, platformRolesResource, roleID)

	store.resetTx()
	assignReq := policyRequest(http.MethodPost, "/api/v1/admin/proxy-rbac/roles/"+roleID+"/users", `{"user_id":"U1"}`, "ADMIN")
	assignReq.SetPathValue("id", roleID)
	code, data, _ = assignRoleUser(app, assignReq, platform.RouteSpec{})
	assertPolicyStatus(t, code, data, http.StatusCreated)
	assertTxEvent(t, app, store, "ProxyPolicyChanged", "role_user_assign")

	store.resetTx()
	replayReq := policyRequest(http.MethodPost, "/api/v1/admin/proxy-rbac/roles/"+roleID+"/users", `{"user_id":"U1"}`, "ADMIN")
	replayReq.SetPathValue("id", roleID)
	code, data, _ = assignRoleUser(app, replayReq, platform.RouteSpec{})
	assertPolicyStatus(t, code, data, http.StatusOK)
	assertNoTxEvent(t, app, store)

	store.resetTx()
	unassignReq := policyRequest(http.MethodDelete, "/api/v1/admin/proxy-rbac/roles/"+roleID+"/users/U1", "", "ADMIN")
	unassignReq.SetPathValue("id", roleID)
	unassignReq.SetPathValue("user_id", "U1")
	code, data, _ = unassignRoleUser(app, unassignReq, platform.RouteSpec{})
	assertPolicyStatus(t, code, data, http.StatusOK)
	assertTxEvent(t, app, store, "ProxyPolicyChanged", "role_user_unassign")
}

func TestAuthorizationPolicyAssignmentMutationsUseTransactionalEvents(t *testing.T) {
	app, store := newAuthorizationPolicyTxTestApp(t)
	policyID := seedProxyPolicyForTxTest(t, app, "PO_TX_ASSIGN")

	assignReq := policyRequest(http.MethodPost, "/api/v1/admin/proxy-rbac/policies/"+policyID+"/assignments", `{"target_type":"user","target_id":"U1"}`, "ADMIN")
	assignReq.SetPathValue("id", policyID)
	code, data, _ := assignPolicy(app, assignReq, platform.RouteSpec{})
	assertPolicyStatus(t, code, data, http.StatusCreated)
	assertTxEvent(t, app, store, "ProxyPolicyChanged", "assign")

	store.resetTx()
	replayReq := policyRequest(http.MethodPost, "/api/v1/admin/proxy-rbac/policies/"+policyID+"/assignments", `{"target_type":"user","target_id":"U1"}`, "ADMIN")
	replayReq.SetPathValue("id", policyID)
	code, data, _ = assignPolicy(app, replayReq, platform.RouteSpec{})
	assertPolicyStatus(t, code, data, http.StatusOK)
	assertNoTxEvent(t, app, store)

	store.resetTx()
	unassignReq := policyRequest(http.MethodDelete, "/api/v1/admin/proxy-rbac/policies/"+policyID+"/assignments", `{"target_type":"user","target_id":"U1"}`, "ADMIN")
	unassignReq.SetPathValue("id", policyID)
	code, data, _ = unassignPolicy(app, unassignReq, platform.RouteSpec{})
	assertPolicyStatus(t, code, data, http.StatusOK)
	assertTxEvent(t, app, store, "ProxyPolicyChanged", "unassign")
}

func TestAuthorizationPolicyUpdatePolicyRulesUseTransactionalEvent(t *testing.T) {
	app, store := newAuthorizationPolicyTxTestApp(t)
	policyID := seedProxyPolicyForTxTest(t, app, "PO_TX_RULES")
	if _, err := app.Store.Create(context.Background(), rulesResource, map[string]any{
		"id":         "PR_OLD",
		"policy_id":  policyID,
		"service_id": "SVC_MINIO",
		"actions":    []string{"view"},
	}); err != nil {
		t.Fatal(err)
	}
	store.resetTx()

	req := policyRequest(http.MethodPut, "/api/v1/admin/proxy-rbac/policies/"+policyID, `{"name":"tx-policy-updated","rules":[{"service_id":"SVC_HARBOR","actions":["create"]}]}`, "ADMIN")
	req.SetPathValue("id", policyID)
	code, data, _ := updatePolicy(app, req, platform.RouteSpec{})
	assertPolicyStatus(t, code, data, http.StatusOK)
	assertTxEvent(t, app, store, "ProxyPolicyChanged", "update")
	assertTxDeleted(t, store, rulesResource, "PR_OLD")
	assertTxCreatedResource(t, store, rulesResource)
	if _, found := app.Store.Get(context.Background(), rulesResource, "PR_OLD"); found {
		t.Fatal("old policy rule still exists after transactional replacement")
	}

	event := store.txEvents[0]
	oldPayload := event.Data["old"].(map[string]any)
	newPayload := event.Data["new"].(map[string]any)
	if oldPayload["id"] != policyID || newPayload["id"] != policyID {
		t.Fatalf("update event old/new payload = %#v", event.Data)
	}
	newRules := newPayload["rules"].([]map[string]any)
	if len(newRules) != 1 || newRules[0]["service_id"] != "SVC_HARBOR" {
		t.Fatalf("new policy rules = %#v, want replacement SVC_HARBOR rule", newRules)
	}
}

func TestRawPermissionMutationsUseTransactionalEvents(t *testing.T) {
	app, store := newAuthorizationPolicyTxTestApp(t)
	aliceRead := `["alice","project-1","model","read"]`
	aliceWrite := `["alice","project-1","model","write"]`
	bobReadPolicy := []string{"bob", "project-1", "model", "read"}

	code, data, _ := addRawPermissionPolicy(app, policyRequest(http.MethodPost, pathRawPermissionPolicy, aliceRead, "ADMIN"), platform.RouteSpec{})
	assertPolicyStatus(t, code, data, http.StatusOK)
	assertTxEvent(t, app, store, "PolicyChanged", "policy_added")

	store.resetTx()
	code, data, _ = addRawPermissionPolicy(app, policyRequest(http.MethodPost, pathRawPermissionPolicy, aliceRead, "ADMIN"), platform.RouteSpec{})
	assertPolicyStatus(t, code, data, http.StatusConflict)
	assertNoTxEvent(t, app, store)

	store.resetTx()
	updateBody := `{"old":["alice","project-1","model","read"],"new":["alice","project-1","model","write"]}`
	code, data, _ = updateRawPermissionPolicy(app, policyRequest(http.MethodPut, pathRawPermissionPolicy, updateBody, "ADMIN"), platform.RouteSpec{})
	assertPolicyStatus(t, code, data, http.StatusOK)
	assertTxEvent(t, app, store, "PolicyChanged", "policy_updated")
	assertTxCreatedResource(t, store, rawPoliciesResource)
	assertTxDeleted(t, store, rawPoliciesResource, rawPolicyID([]string{"alice", "project-1", "model", "read"}))
	if rawPermissionRepo(app).RawPermissionPolicyExists(context.Background(), []string{"alice", "project-1", "model", "read"}) {
		t.Fatal("old raw permission policy still exists after transactional rename")
	}
	if !rawPermissionRepo(app).RawPermissionPolicyExists(context.Background(), []string{"alice", "project-1", "model", "write"}) {
		t.Fatal("new raw permission policy missing after transactional rename")
	}

	if created, err := rawPermissionRepo(app).CreateRawPermissionPolicy(context.Background(), bobReadPolicy); err != nil || !created {
		t.Fatalf("seed conflicting raw policy created=%v err=%v", created, err)
	}
	store.resetTx()
	conflictBody := `{"old":["alice","project-1","model","write"],"new":["bob","project-1","model","read"]}`
	code, data, _ = updateRawPermissionPolicy(app, policyRequest(http.MethodPut, pathRawPermissionPolicy, conflictBody, "ADMIN"), platform.RouteSpec{})
	assertPolicyStatus(t, code, data, http.StatusConflict)
	assertNoTxEvent(t, app, store)
	if !rawPermissionRepo(app).RawPermissionPolicyExists(context.Background(), bobReadPolicy) {
		t.Fatal("conflict path removed existing raw permission policy")
	}

	store.resetTx()
	missingBody := `{"old":["missing","project-1","model","read"],"new":["nobody","project-1","model","write"]}`
	code, data, _ = updateRawPermissionPolicy(app, policyRequest(http.MethodPut, pathRawPermissionPolicy, missingBody, "ADMIN"), platform.RouteSpec{})
	assertPolicyStatus(t, code, data, http.StatusNotFound)
	assertNoTxEvent(t, app, store)

	store.resetTx()
	code, data, _ = removeRawPermissionPolicy(app, policyRequest(http.MethodDelete, pathRawPermissionPolicy, aliceWrite, "ADMIN"), platform.RouteSpec{})
	assertPolicyStatus(t, code, data, http.StatusOK)
	assertTxEvent(t, app, store, "PolicyChanged", "policy_removed")
}

func TestAuthorizationPolicyBatchMutationsUsePerItemTransactionalEvents(t *testing.T) {
	app, store := newAuthorizationPolicyTxTestApp(t)
	policyID := seedProxyPolicyForTxTest(t, app, "PO_TX_BATCH")
	role, err := authorizationPolicyRepo(app).CreateProxyRole(context.Background(), map[string]any{
		"id":           "RL_TX_BATCH",
		"name":         "batch-role",
		"display_name": "Batch Role",
	})
	if err != nil {
		t.Fatal(err)
	}
	roleID := role["id"].(string)

	store.resetTx()
	assignReq := policyRequest(http.MethodPost, "/api/v1/admin/proxy-rbac/policies/"+policyID+"/assignments/batch", `{"assignments":[{"target_type":"user","target_id":"U1"},{"target_type":"role","target_id":"RL_TX_BATCH"}]}`, "ADMIN")
	assignReq.SetPathValue("id", policyID)
	code, data, _ := batchAssignPolicy(app, assignReq, platform.RouteSpec{})
	assertPolicyStatus(t, code, data, http.StatusOK)
	assertTxEventCount(t, app, store, "ProxyPolicyChanged", "assign", 2)

	store.resetTx()
	roleReq := policyRequest(http.MethodPost, "/api/v1/admin/proxy-rbac/roles/"+roleID+"/users/batch", `{"user_ids":["U1","ADMIN"]}`, "ADMIN")
	roleReq.SetPathValue("id", roleID)
	code, data, _ = batchAssignRoleUsers(app, roleReq, platform.RouteSpec{})
	assertPolicyStatus(t, code, data, http.StatusOK)
	assertTxEventCount(t, app, store, "ProxyPolicyChanged", "role_user_assign", 2)

	store.resetTx()
	permissionReq := policyRequest(http.MethodPost, "/api/v1/admin/permissions/batch", `{"operations":[{"type":"project_member","action":"add","project_id":"P1","user_id":"U1","role":"manager"},{"type":"group_role","action":"add","group_id":"G1","user_id":"U1","role":"admin"}]}`, "ADMIN")
	code, data, _ = batchProcessPermissions(app, permissionReq, platform.RouteSpec{})
	assertPolicyStatus(t, code, data, http.StatusOK)
	assertTxEventCount(t, app, store, "PolicyChanged", "batch_permissions_processed", 2)
}

func newAuthorizationPolicyTxTestApp(t *testing.T) (*platform.App, *authorizationPolicyTxStore) {
	t.Helper()
	store := &authorizationPolicyTxStore{Store: platform.NewStore()}
	app := platform.NewApp(platform.Config{ServiceName: "all", HTTPAddr: ":0"}, platform.WithStore(store))
	Register(app)
	createPolicyRecords(t, app, usersResource, []map[string]any{
		{"id": "U1", "username": "alice", "role_id": "RO_USER"},
		{"id": "ADMIN", "username": "admin", "role_id": "RO_ADMIN"},
	})
	createPolicyRecords(t, app, rolesResource, []map[string]any{
		{"id": "RO_ADMIN", "name": "admin", "capabilities": map[string]any{"adminPanel": true}},
		{"id": "RO_USER", "name": "user", "capabilities": map[string]any{"adminPanel": false}},
	})
	store.resetTx()
	return app, store
}

func seedProxyPolicyForTxTest(t *testing.T, app *platform.App, id string) string {
	t.Helper()
	now := time.Now().UTC()
	policy, err := authorizationPolicyRepo(app).CreateProxyPolicy(context.Background(), map[string]any{
		"id":          id,
		"name":        id,
		"description": "transactional test policy",
		"is_system":   false,
		"created_at":  now,
		"updated_at":  now,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return policy["id"].(string)
}

func assertTxEventCount(t *testing.T, app *platform.App, store *authorizationPolicyTxStore, name, action string, want int) {
	t.Helper()
	if store.runInTx != want {
		t.Fatalf("RunInTx calls = %d, want %d", store.runInTx, want)
	}
	if len(app.Events.Outbox()) != 0 {
		t.Fatalf("app.Events outbox = %#v, want no direct publish", app.Events.Outbox())
	}
	if len(store.txEvents) != want {
		t.Fatalf("tx events = %#v, want %d", store.txEvents, want)
	}
	for _, event := range store.txEvents {
		if event.Name != name || event.Data["action"] != action {
			t.Fatalf("tx event name/action = %s/%v, want %s/%s", event.Name, event.Data["action"], name, action)
		}
	}
}

func assertTxEvent(t *testing.T, app *platform.App, store *authorizationPolicyTxStore, name, action string) {
	t.Helper()
	if store.runInTx != 1 {
		t.Fatalf("RunInTx calls = %d, want 1", store.runInTx)
	}
	if len(app.Events.Outbox()) != 0 {
		t.Fatalf("app.Events outbox = %#v, want no direct publish", app.Events.Outbox())
	}
	if len(store.txEvents) != 1 {
		t.Fatalf("tx events = %#v, want one", store.txEvents)
	}
	event := store.txEvents[0]
	if event.Name != name || event.Data["action"] != action {
		t.Fatalf("tx event name/action = %s/%v, want %s/%s", event.Name, event.Data["action"], name, action)
	}
}

func assertNoTxEvent(t *testing.T, app *platform.App, store *authorizationPolicyTxStore) {
	t.Helper()
	if store.runInTx != 1 {
		t.Fatalf("RunInTx calls = %d, want 1", store.runInTx)
	}
	if len(app.Events.Outbox()) != 0 {
		t.Fatalf("app.Events outbox = %#v, want no direct publish", app.Events.Outbox())
	}
	if len(store.txEvents) != 0 {
		t.Fatalf("tx events = %#v, want none", store.txEvents)
	}
}

func assertTxCreated(t *testing.T, store *authorizationPolicyTxStore, resource, id string) {
	t.Helper()
	for _, write := range store.creates {
		if write.resource == resource && write.id == id {
			return
		}
	}
	t.Fatalf("tx creates = %#v, want %s/%s", store.creates, resource, id)
}

func assertTxCreatedResource(t *testing.T, store *authorizationPolicyTxStore, resource string) {
	t.Helper()
	for _, write := range store.creates {
		if write.resource == resource {
			return
		}
	}
	t.Fatalf("tx creates = %#v, want resource %s", store.creates, resource)
}

func assertTxUpdated(t *testing.T, store *authorizationPolicyTxStore, resource, id string) {
	t.Helper()
	for _, write := range store.updates {
		if write.resource == resource && write.id == id {
			return
		}
	}
	t.Fatalf("tx updates = %#v, want %s/%s", store.updates, resource, id)
}

func assertTxDeleted(t *testing.T, store *authorizationPolicyTxStore, resource, id string) {
	t.Helper()
	for _, deleted := range store.deletes {
		if deleted.resource == resource && deleted.id == id {
			return
		}
	}
	t.Fatalf("tx deletes = %#v, want %s/%s", store.deletes, resource, id)
}
