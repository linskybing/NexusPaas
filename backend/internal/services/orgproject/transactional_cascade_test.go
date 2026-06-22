package orgproject

import (
	"context"
	"net/http"
	"testing"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

type orgScopedTxStore struct {
	*platform.Store
	projectAliases map[string]string
	ranInTx        bool
	committed      []contracts.Event
}

func (s *orgScopedTxStore) Get(ctx context.Context, resource, id string) (contracts.Record[map[string]any], bool) {
	if resource == projectsResource && s.projectAliases != nil {
		if target := s.projectAliases[id]; target != "" {
			return s.Store.Get(ctx, resource, target)
		}
	}
	return s.Store.Get(ctx, resource, id)
}

func (s *orgScopedTxStore) RunInTx(ctx context.Context, fn func(tx platform.StoreTx) error) error {
	s.ranInTx = true
	tx := &orgRecordingTx{store: s.Store}
	if err := fn(tx); err != nil {
		return err
	}
	s.committed = append(s.committed, tx.events...)
	return nil
}

func (s *orgScopedTxStore) resetTx() {
	s.ranInTx = false
	s.committed = nil
}

type orgRecordingTx struct {
	store  *platform.Store
	events []contracts.Event
}

func (tx *orgRecordingTx) Create(ctx context.Context, resource string, data map[string]any) (contracts.Record[map[string]any], error) {
	return tx.store.Create(ctx, resource, data)
}

func (tx *orgRecordingTx) Update(ctx context.Context, resource, id string, data map[string]any) (contracts.Record[map[string]any], bool, error) {
	record, ok := tx.store.Update(ctx, resource, id, data)
	return record, ok, nil
}

func (tx *orgRecordingTx) Delete(ctx context.Context, resource, id string) (bool, error) {
	return tx.store.Delete(ctx, resource, id), nil
}

func (tx *orgRecordingTx) Emit(event contracts.Event) {
	tx.events = append(tx.events, event)
}

func TestDeleteProjectUsesTransactionalCascadeAndEvent(t *testing.T) {
	app, store := newOrgScopedTxApp(t)
	createOrgRecords(t, app, projectsResource, []map[string]any{{"id": "P-STORED", "project_name": "training", "owner_id": "G1"}})
	store.projectAliases = map[string]string{"P-REQUEST": "P-STORED"}
	createOrgRecords(t, app, projectMembersResource, []map[string]any{
		{"id": "member-stored", "project_id": "P-STORED", "user_id": "U1"},
		{"id": "member-requested", "project_id": "P-REQUEST", "user_id": "U2"},
		{"id": "member-other", "project_id": "P2", "user_id": "U3"},
	})
	createOrgRecords(t, app, projectUserQuotasResource, []map[string]any{
		{"id": "quota-stored", "project_id": "P-STORED", "user_id": "U1"},
		{"id": "quota-requested", "project_id": "P-REQUEST", "user_id": "U2"},
		{"id": "quota-other", "project_id": "P2", "user_id": "U3"},
	})
	createOrgRecords(t, app, gpuClaimsResource, []map[string]any{
		{"id": "gpu-stored", "project_id": "P-STORED", "name": "claim"},
		{"id": "gpu-other", "project_id": "P2", "name": "claim"},
	})

	req := orgProjectPathRequest(http.MethodDelete, "/api/v1/projects/P-REQUEST", "", "ADMIN", "P-REQUEST")
	status, data, _ := deleteProject(app, req, platform.RouteSpec{})
	assertOrgStatus(t, status, data, http.StatusOK)

	if !store.ranInTx {
		t.Fatal("deleteProject did not use RunInTx")
	}
	if got := len(app.Events.Outbox()); got != 0 {
		t.Fatalf("app.Events published %d events, want 0 because event is tx-committed", got)
	}
	if len(store.committed) != 1 {
		t.Fatalf("committed events = %d, want 1", len(store.committed))
	}
	event := store.committed[0]
	if event.Name != "ProjectDeleted" {
		t.Fatalf("event name = %q, want ProjectDeleted", event.Name)
	}
	if event.Data["project_id"] != "P-STORED" || event.Data["project_name"] != "training" {
		t.Fatalf("ProjectDeleted payload = %#v, want deleted project data", event.Data)
	}
	assertOrgRecordMissing(t, app, projectsResource, "P-STORED")
	assertOrgRecordMissing(t, app, projectMembersResource, "member-stored")
	assertOrgRecordMissing(t, app, projectMembersResource, "member-requested")
	assertOrgRecordMissing(t, app, projectUserQuotasResource, "quota-stored")
	assertOrgRecordMissing(t, app, projectUserQuotasResource, "quota-requested")
	assertOrgRecordMissing(t, app, gpuClaimsResource, "gpu-stored")
	assertOrgRecordExists(t, app, projectMembersResource, "member-other")
	assertOrgRecordExists(t, app, projectUserQuotasResource, "quota-other")
	assertOrgRecordExists(t, app, gpuClaimsResource, "gpu-other")
}

func TestDeleteGroupUsesTransactionalCascadeAndEvent(t *testing.T) {
	app, store := newOrgScopedTxApp(t)
	createOrgRecords(t, app, groupsResource, []map[string]any{{"id": "REC1", "group_name": "vision"}})
	if _, ok := app.Store.Update(context.Background(), groupsResource, "REC1", map[string]any{"id": "G-ALIAS", "g_id": "G-ALIAS"}); !ok {
		t.Fatal("failed to create alias-backed group fixture")
	}
	createOrgRecords(t, app, userGroupsResource, []map[string]any{
		{"id": "U1:REC1", "user_id": "U1", "group_id": "REC1", "role": "user"},
		{"id": "U2:G-ALIAS", "user_id": "U2", "group_id": "G-ALIAS", "role": "user"},
		{"id": "U3:G2", "user_id": "U3", "group_id": "G2", "role": "user"},
	})

	req := orgRequest(http.MethodDelete, "/api/v1/groups/G-ALIAS", "", "ADMIN")
	req.SetPathValue("id", "G-ALIAS")
	status, data, _ := deleteGroup(app, req, platform.RouteSpec{})
	assertOrgStatus(t, status, data, http.StatusOK)

	if !store.ranInTx {
		t.Fatal("deleteGroup did not use RunInTx")
	}
	if got := len(app.Events.Outbox()); got != 0 {
		t.Fatalf("app.Events published %d events, want 0 because event is tx-committed", got)
	}
	if len(store.committed) != 1 {
		t.Fatalf("committed events = %d, want 1", len(store.committed))
	}
	event := store.committed[0]
	if event.Name != "GroupDeleted" {
		t.Fatalf("event name = %q, want GroupDeleted", event.Name)
	}
	if event.Data["id"] != "G-ALIAS" {
		t.Fatalf("GroupDeleted payload = %#v, want requested id", event.Data)
	}
	assertOrgRecordMissing(t, app, groupsResource, "REC1")
	assertOrgRecordMissing(t, app, userGroupsResource, "U1:REC1")
	assertOrgRecordMissing(t, app, userGroupsResource, "U2:G-ALIAS")
	assertOrgRecordExists(t, app, userGroupsResource, "U3:G2")
}

func TestOrgProjectAccessQuotaGPUAndPlanMutationsUseTransactionalEvents(t *testing.T) {
	app, store := newOrgScopedTxApp(t)
	app.Config.ServiceAPIKey = "service-secret"
	createOrgRecords(t, app, orgIdentityUsers, []map[string]any{
		{"id": "U1", "username": "alice"},
		{"id": "U2", "username": "bob"},
		{"id": "U3", "username": "carol"},
	})
	createOrgRecords(t, app, groupsResource, []map[string]any{{"id": "G1", "group_name": "vision", "name": "vision"}})
	createOrgRecords(t, app, projectsResource, []map[string]any{{"id": "P1", "project_name": "training", "owner_id": "G1"}})

	memberReq := orgRequest(http.MethodPost, "/api/v1/user-groups", "", "ADMIN")
	status, data, _ := createMembership(app, memberReq, "U1", "G1", "user", false)
	assertOrgStatus(t, status, data, http.StatusOK)
	assertOrgTxEvents(t, app, store, "GroupMembershipChanged")

	store.resetTx()
	status, data, _ = createMembership(app, memberReq, "U1", "G1", "manager", true)
	assertOrgStatus(t, status, data, http.StatusOK)
	assertOrgTxEvents(t, app, store, "GroupMembershipChanged")

	store.resetTx()
	removeGroupReq := orgRequest(http.MethodDelete, "/api/v1/user-groups?uid=U1&gid=G1", "", "ADMIN")
	status, data, _ = removeUserFromGroup(app, removeGroupReq, platform.RouteSpec{})
	assertOrgStatus(t, status, data, http.StatusOK)
	assertOrgTxEvents(t, app, store, "GroupMembershipChanged")

	store.resetTx()
	addProjectReq := orgProjectPathRequest(http.MethodPost, "/api/v1/projects/P1/members", `{"members":[{"user_id":"U2","role":"user"}]}`, "ADMIN", "P1")
	status, data, _ = addProjectMembers(app, addProjectReq, platform.RouteSpec{})
	assertOrgStatus(t, status, data, http.StatusOK)
	assertOrgTxEvents(t, app, store, "project_memberCreated")

	store.resetTx()
	updateMemberReq := orgProjectPathRequest(http.MethodPatch, "/api/v1/projects/P1/members/U2/role", `{"role":"manager"}`, "ADMIN", "P1")
	updateMemberReq.SetPathValue("userId", "U2")
	status, data, _ = updateProjectMemberRole(app, updateMemberReq, platform.RouteSpec{})
	assertOrgStatus(t, status, data, http.StatusOK)
	assertOrgTxEvents(t, app, store, "project_memberUpdated")

	store.resetTx()
	quotaReq := orgProjectPathRequest(http.MethodPut, "/api/v1/projects/P1/members/U2/quota", `{"gpu_limit":1,"cpu_limit":2,"memory_limit_gb":4}`, "ADMIN", "P1")
	quotaReq.SetPathValue("userId", "U2")
	status, data, _ = upsertProjectMemberQuota(app, quotaReq, platform.RouteSpec{})
	assertOrgStatus(t, status, data, http.StatusOK)
	assertOrgTxEvents(t, app, store, "UserQuotaUpdated")

	store.resetTx()
	status, data, _ = deleteProjectMemberQuota(app, quotaReq, platform.RouteSpec{})
	assertOrgStatus(t, status, data, http.StatusOK)
	assertOrgTxEvents(t, app, store, "UserQuotaDeleted")

	store.resetTx()
	removeMemberReq := orgProjectPathRequest(http.MethodDelete, "/api/v1/projects/P1/members/U2", "", "ADMIN", "P1")
	removeMemberReq.SetPathValue("userId", "U2")
	status, data, _ = removeProjectMember(app, removeMemberReq, platform.RouteSpec{})
	assertOrgStatus(t, status, data, http.StatusOK)
	assertOrgTxEvents(t, app, store, "project_memberDeleted")

	store.resetTx()
	workspaceReq := orgProjectPathRequest(http.MethodPut, "/api/v1/projects/P1/workspace-settings", `{"max_ide_runtime_seconds":3600}`, "ADMIN", "P1")
	status, data, _ = updateProjectWorkspaceSettings(app, workspaceReq, platform.RouteSpec{})
	assertOrgStatus(t, status, data, http.StatusOK)
	assertOrgTxEvents(t, app, store, "ProjectUpdated")

	store.resetTx()
	gpuReq := orgProjectPathRequest(http.MethodPost, "/api/v1/projects/P1/gpu-claims", `{"name":"gpu-a","device_class_name":"nvidia-a100","gpu_count":1,"sm_percentage":50,"vram_percentage":50}`, "ADMIN", "P1")
	status, data, _ = createGPUClaim(app, gpuReq, platform.RouteSpec{})
	assertOrgStatus(t, status, data, http.StatusCreated)
	assertOrgTxEvents(t, app, store, "GPUClaimCreated")

	store.resetTx()
	deleteGPUReq := orgProjectPathRequest(http.MethodDelete, "/api/v1/projects/P1/gpu-claims/gpu-a", "", "ADMIN", "P1")
	deleteGPUReq.SetPathValue("requestId", "gpu-a")
	status, data, _ = deleteGPUClaim(app, deleteGPUReq, platform.RouteSpec{})
	assertOrgStatus(t, status, data, http.StatusOK)
	assertOrgTxEvents(t, app, store, "GPUClaimDeleted")

	store.resetTx()
	bindReq := orgRequest(http.MethodPut, "/internal/org-project/projects/P1/plan", `{"plan_id":"plan-1"}`, "")
	bindReq.Header.Set("X-Service-Key", "service-secret")
	bindReq.SetPathValue("project_id", "P1")
	status, data, _ = bindProjectPlan(app, bindReq, platform.RouteSpec{})
	assertOrgStatus(t, status, data, http.StatusOK)
	assertOrgTxEvents(t, app, store, "ProjectUpdated")

	store.resetTx()
	clearReq := orgRequest(http.MethodDelete, "/internal/org-project/plans/plan-1/project-bindings", "", "")
	clearReq.Header.Set("X-Service-Key", "service-secret")
	clearReq.SetPathValue("plan_id", "plan-1")
	status, data, _ = clearProjectsPlan(app, clearReq, platform.RouteSpec{})
	assertOrgStatus(t, status, data, http.StatusOK)
	assertOrgTxEvents(t, app, store, "ProjectUpdated")
}

func newOrgScopedTxApp(t *testing.T) (*platform.App, *orgScopedTxStore) {
	t.Helper()
	store := &orgScopedTxStore{Store: platform.NewStore()}
	app := platform.NewApp(platform.Config{ServiceName: serviceName}, platform.WithStore(store))
	createOrgRecords(t, app, orgIdentityUsers, []map[string]any{{"id": "ADMIN", "admin_panel": true}})
	return app, store
}

func assertOrgRecordExists(t *testing.T, app *platform.App, resource, id string) {
	t.Helper()
	if _, ok := app.Store.Get(context.Background(), resource, id); !ok {
		t.Fatalf("%s/%s does not exist", resource, id)
	}
}

func assertOrgTxEvents(t *testing.T, app *platform.App, store *orgScopedTxStore, names ...string) {
	t.Helper()
	if !store.ranInTx {
		t.Fatal("mutation did not use RunInTx")
	}
	if got := len(app.Events.Outbox()); got != 0 {
		t.Fatalf("app.Events published %d events, want 0 because events are tx-committed", got)
	}
	if len(store.committed) != len(names) {
		t.Fatalf("committed events = %#v, want names %v", store.committed, names)
	}
	for i, name := range names {
		if store.committed[i].Name != name {
			t.Fatalf("event[%d].Name = %q, want %q; events=%#v", i, store.committed[i].Name, name, store.committed)
		}
	}
}
