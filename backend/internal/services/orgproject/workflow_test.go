package orgproject

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

func TestOrgProjectGroupHandlers(t *testing.T) {
	app := newOrgProjectTestApp(t)
	app.Config.GroupStorageClassOptions = []string{"fast", "archive"}
	app.Config.GroupRegistryProfileOptions = []string{"default", "gpu"}

	code, data, _ := listGroups(app, orgRequest(http.MethodGet, "/api/v1/groups", "", ""), platform.RouteSpec{})
	assertOrgStatus(t, code, data, http.StatusUnauthorized)
	code, data, _ = createGroup(app, orgRequest(http.MethodPost, "/api/v1/groups", `{"id":"G3","group_name":"research","storage_class":"fast","registry_profile":"gpu","allow_run_as_root":true}`, "ADMIN"), platform.RouteSpec{})
	assertOrgStatus(t, code, data, http.StatusCreated)
	if data.(map[string]any)["id"] != "G3" || data.(map[string]any)["storage_class"] != "fast" {
		t.Fatalf("created group = %#v, want G3 with fast storage", data)
	}

	code, data, _ = groupPolicyOptions(app, orgRequest(http.MethodGet, "/api/v1/admin/group-policy-options", "", "ADMIN"), platform.RouteSpec{})
	assertOrgStatus(t, code, data, http.StatusOK)
	if options := data.(map[string]any); len(options["storage_classes"].([]map[string]any)) != 2 {
		t.Fatalf("policy options = %#v, want configured storage classes", options)
	}
	code, data, _ = listGroups(app, orgRequest(http.MethodGet, "/api/v1/groups?page=1&limit=1", "", "U2"), platform.RouteSpec{})
	assertOrgStatus(t, code, data, http.StatusOK)
	if page := data.(map[string]any); page["total"] != 1 || len(page["list"].([]map[string]any)) != 1 {
		t.Fatalf("visible groups = %#v, want only member group", page)
	}

	getReq := orgRequest(http.MethodGet, "/api/v1/groups/G1", "", "U2")
	getReq.SetPathValue("id", "G1")
	code, data, _ = getGroup(app, getReq, platform.RouteSpec{})
	assertOrgStatus(t, code, data, http.StatusOK)

	updateReq := orgRequest(http.MethodPut, "/api/v1/groups/G3", `{"description":"updated","storageClass":"archive"}`, "ADMIN")
	updateReq.SetPathValue("id", "G3")
	code, data, _ = updateGroup(app, updateReq, platform.RouteSpec{})
	assertOrgStatus(t, code, data, http.StatusOK)
	if data.(map[string]any)["description"] != "updated" || data.(map[string]any)["storage_class"] != "archive" {
		t.Fatalf("updated group = %#v, want updated fields", data)
	}

	code, data, _ = batchDeleteGroups(app, orgRequest(http.MethodDelete, "/api/v1/groups/batch", `{"ids":["G3"]}`, "ADMIN"), platform.RouteSpec{})
	assertOrgStatus(t, code, data, http.StatusOK)
	if result := data.(map[string]any); result["succeeded"] != 1 || result["failed"] != 0 {
		t.Fatalf("batch delete = %#v, want one success", result)
	}
}

func TestOrgProjectMembershipHandlers(t *testing.T) {
	app := newOrgProjectTestApp(t)

	code, data, _ := addUserToGroup(app, orgRequest(http.MethodPost, "/api/v1/user-groups", `{"uid":"U3","gid":"G1","role":"user"}`, "U1"), platform.RouteSpec{})
	assertOrgStatus(t, code, data, http.StatusOK)
	if data.(map[string]any)["user_id"] != "U3" {
		t.Fatalf("created membership = %#v, want U3", data)
	}
	code, data, _ = updateUserGroup(app, orgRequest(http.MethodPut, "/api/v1/user-groups", `{"uid":"U3","gid":"G1","role":"manager"}`, "U1"), platform.RouteSpec{})
	assertOrgStatus(t, code, data, http.StatusOK)
	code, data, _ = updateUserGroup(app, orgRequest(http.MethodPut, "/api/v1/user-groups", `{"uid":"U3","gid":"G1","role":"admin"}`, "ADMIN"), platform.RouteSpec{})
	assertOrgStatus(t, code, data, http.StatusOK)

	code, data, _ = getUserGroup(app, orgRequest(http.MethodGet, "/api/v1/user-groups?uid=U3&gid=G1", "", "U3"), platform.RouteSpec{})
	assertOrgStatus(t, code, data, http.StatusOK)
	if data.(map[string]any)["group_name"] != "vision" {
		t.Fatalf("decorated membership = %#v, want group name", data)
	}
	code, data, _ = userGroupsByGroup(app, orgRequest(http.MethodGet, "/api/v1/user-groups/by-group?g_id=G1", "", "U2"), platform.RouteSpec{})
	assertOrgStatus(t, code, data, http.StatusOK)
	if users := data.(map[string]map[string]any)["G1"]["users"].([]map[string]any); len(users) != 3 {
		t.Fatalf("members by group = %#v, want three", users)
	}
	code, data, _ = userGroupsByUser(app, orgRequest(http.MethodGet, "/api/v1/user-groups/by-user?u_id=U3", "", "U3"), platform.RouteSpec{})
	assertOrgStatus(t, code, data, http.StatusOK)
	if len(data.([]map[string]any)) != 1 {
		t.Fatalf("memberships by user = %#v, want one", data)
	}

	membersReq := orgRequest(http.MethodGet, "/api/v1/user-groups/G1/members", "", "U2")
	membersReq.SetPathValue("group_id", "G1")
	code, data, _ = groupMembers(app, membersReq, platform.RouteSpec{})
	assertOrgStatus(t, code, data, http.StatusOK)
	if len(data.([]map[string]any)) != 3 {
		t.Fatalf("group members = %#v, want three", data)
	}

	contextReq := orgRequest(http.MethodGet, "/api/v1/user-groups/G1/add-members-context", "", "U1")
	contextReq.SetPathValue("group_id", "G1")
	code, data, _ = addMembersContext(app, contextReq, platform.RouteSpec{})
	assertOrgStatus(t, code, data, http.StatusOK)
	if available := data.(map[string]any)["available_users"].(map[string]any); available["total"] != 2 {
		t.Fatalf("add-members context = %#v, want two available users", data)
	}

	resolveReq := orgRequest(http.MethodPost, "/api/v1/user-groups/G1/resolve-add-members", `{"identifiers":["U4","bob","missing","U4"]}`, "U1")
	resolveReq.SetPathValue("group_id", "G1")
	code, data, _ = resolveAddMembers(app, resolveReq, platform.RouteSpec{})
	assertOrgStatus(t, code, data, http.StatusOK)
	result := data.(map[string]any)
	if len(result["resolved"].([]map[string]any)) != 1 || len(result["already_members"].([]map[string]any)) != 1 || len(result["unresolved"].([]string)) != 1 {
		t.Fatalf("resolve result = %#v, want resolved/already/unresolved split", result)
	}

	removeReq := orgRequest(http.MethodDelete, "/api/v1/user-groups?uid=U3&gid=G1", "", "U1")
	code, data, _ = removeUserFromGroup(app, removeReq, platform.RouteSpec{})
	assertOrgStatus(t, code, data, http.StatusOK)
}

func TestOrgProjectGuardAndHelperBranches(t *testing.T) {
	app := newOrgProjectTestApp(t)
	app.Config.GroupStorageClassOptions = []string{"fast"}

	code, data, _ := createGroup(app, orgRequest(http.MethodPost, "/api/v1/groups", `{`, "ADMIN"), platform.RouteSpec{})
	assertOrgStatus(t, code, data, http.StatusBadRequest)
	code, data, _ = createGroup(app, orgRequest(http.MethodPost, "/api/v1/groups", `{"group_name":"bad","storage_class":"missing"}`, "ADMIN"), platform.RouteSpec{})
	assertOrgStatus(t, code, data, http.StatusBadRequest)
	code, data, _ = addUserToGroup(app, orgRequest(http.MethodPost, "/api/v1/user-groups", `{"uid":"DISABLED","gid":"G1","role":"user"}`, "U1"), platform.RouteSpec{})
	assertOrgStatus(t, code, data, http.StatusBadRequest)
	code, data, _ = addUserToGroup(app, orgRequest(http.MethodPost, "/api/v1/user-groups", `{"uid":"U3","gid":"G1","role":"admin"}`, "U1"), platform.RouteSpec{})
	assertOrgStatus(t, code, data, http.StatusForbidden)
	code, data, _ = userGroupsByUser(app, orgRequest(http.MethodGet, "/api/v1/user-groups/by-user?u_id=U3", "", "U2"), platform.RouteSpec{})
	assertOrgStatus(t, code, data, http.StatusForbidden)

	if positiveInt("bad", 7) != 7 || positiveInt("0", 7) != 7 || positiveInt("12", 7) != 12 {
		t.Fatal("positiveInt fallback/parsing failed")
	}
	if got := normalizeIdentifiers([]string{" U1 ", "u1", "", "U2"}); len(got) != 2 {
		t.Fatalf("normalized identifiers = %#v, want unique non-empty values", got)
	}
	if paths := normalizedHostPaths(map[string]any{"allowedHostPaths": []any{"/data"}}); len(paths) != 1 {
		t.Fatalf("host paths = %#v, want camel-case value", paths)
	}
	if optionalText(map[string]any{"storageClass": " fast "}, "storage_class", "storageClass") != "fast" {
		t.Fatal("optionalText did not normalize camel-case value")
	}
}

func TestOrgProjectBatchAddAndParserHelpers(t *testing.T) {
	app := newOrgProjectTestApp(t)

	batchReq := orgRequest(http.MethodPost, "/api/v1/user-groups/batch", `{"gid":"G1","role":"user","user_ids":["U3","DISABLED"]}`, "U1")
	code, data, _ := batchAddMembers(app, batchReq, platform.RouteSpec{})
	assertOrgStatus(t, code, data, http.StatusOK)
	assertOrgMapValue(t, data, "succeeded", 1)
	assertOrgMapValue(t, data, "failed", 1)

	assertOrgStringSlice(t, requestIDs(map[string]any{"projectIds": []any{"P1", "P2"}}), 2)
	assertOrgStringSlice(t, requestUserIDs(map[string]any{"ids": []any{"U1"}}), 1)
	assertOrgStringSlice(t, firstStringSlice(map[string]any{"users": []any{"U1", "U2"}}, "missing", "users"), 2)
	if firstNonNil(nil, nil) != nil || firstNonNil(nil, "fallback") != "fallback" {
		t.Fatal("firstNonNil did not return the first non-nil value")
	}
	if nullableText("  ") != nil || nullableText(" value ") != "value" {
		t.Fatal("nullableText did not normalize blank and non-blank values")
	}
}

func TestOrgProjectProjectMemberQuotaAndGPUClaimWorkflow(t *testing.T) {
	app := newOrgProjectTestApp(t)

	code, data, _ := listProjects(app, orgRequest(http.MethodGet, "/api/v1/projects", "", ""), platform.RouteSpec{})
	assertOrgStatus(t, code, data, http.StatusUnauthorized)
	code, data, _ = createProject(app, orgRequest(http.MethodPost, "/api/v1/projects", `{"id":"P1","project_name":"trainer","g_id":"G1","max_ide_runtime_seconds":7200,"external_network_enabled":true}`, "ADMIN"), platform.RouteSpec{})
	assertOrgStatus(t, code, data, http.StatusCreated)
	if project := data.(map[string]any); project["id"] != "P1" || project["owner_id"] != "G1" {
		t.Fatalf("created project = %#v, want P1 owned by G1", project)
	}

	code, data, _ = listProjects(app, orgRequest(http.MethodGet, "/api/v1/projects", "", "U2"), platform.RouteSpec{})
	assertOrgStatus(t, code, data, http.StatusOK)
	if projects := data.([]map[string]any); len(projects) != 1 || projects[0]["id"] != "P1" {
		t.Fatalf("visible projects = %#v, want P1", projects)
	}
	getReq := orgProjectPathRequest(http.MethodGet, "/api/v1/projects/P1", "", "U3", "P1")
	code, data, _ = getProject(app, getReq, platform.RouteSpec{})
	assertOrgStatus(t, code, data, http.StatusForbidden)

	addReq := orgProjectPathRequest(http.MethodPost, "/api/v1/projects/P1/members", `{"members":[{"user_id":"U3","role":"manager"}]}`, "U1", "P1")
	code, data, _ = addProjectMembers(app, addReq, platform.RouteSpec{})
	assertOrgStatus(t, code, data, http.StatusOK)
	if result := data.(map[string]any); result["succeeded"] != 1 || result["failed"] != 0 {
		t.Fatalf("add members result = %#v, want one success", result)
	}

	membersReq := orgProjectPathRequest(http.MethodGet, "/api/v1/projects/P1/members", "", "U2", "P1")
	code, data, _ = listProjectMembers(app, membersReq, platform.RouteSpec{})
	assertOrgStatus(t, code, data, http.StatusOK)
	if members := data.([]map[string]any); len(members) != 3 {
		t.Fatalf("project members = %#v, want owner-group members plus direct member", members)
	}

	workspaceReq := orgProjectPathRequest(http.MethodPut, "/api/v1/projects/P1/workspace-settings", `{"max_ide_runtime_seconds":3600}`, "U3", "P1")
	code, data, _ = updateProjectWorkspaceSettings(app, workspaceReq, platform.RouteSpec{})
	assertOrgStatus(t, code, data, http.StatusOK)
	if data.(map[string]any)["max_ide_runtime_seconds"] != 3600 {
		t.Fatalf("workspace update = %#v, want runtime cap", data)
	}

	roleReq := orgProjectPathRequest(http.MethodPut, "/api/v1/projects/P1/members/U3", `{"role":"user"}`, "U1", "P1")
	roleReq.SetPathValue("userId", "U3")
	code, data, _ = updateProjectMemberRole(app, roleReq, platform.RouteSpec{})
	assertOrgStatus(t, code, data, http.StatusOK)

	quotaBatchReq := orgProjectPathRequest(http.MethodPut, "/api/v1/projects/P1/members/quotas", `{"updates":[{"user_id":"U3","gpu_limit":1.5,"cpu_limit":4,"memory_limit_gb":16}]}`, "U1", "P1")
	code, data, _ = batchSetProjectMemberQuotas(app, quotaBatchReq, platform.RouteSpec{})
	assertOrgStatus(t, code, data, http.StatusOK)
	if result := data.(map[string]any); result["succeeded"] != 1 {
		t.Fatalf("quota batch result = %#v, want one success", result)
	}
	quotaReq := orgProjectPathRequest(http.MethodGet, "/api/v1/projects/P1/members/U3/quota", "", "U3", "P1")
	quotaReq.SetPathValue("userId", "U3")
	code, data, _ = getProjectMemberQuota(app, quotaReq, platform.RouteSpec{})
	assertOrgStatus(t, code, data, http.StatusOK)
	if quota := data.(map[string]any); quota["gpu_limit"] != 1.5 {
		t.Fatalf("quota = %#v, want gpu limit", quota)
	}

	removeGroupReq := orgProjectPathRequest(http.MethodDelete, "/api/v1/projects/P1/members/U2", "", "U1", "P1")
	removeGroupReq.SetPathValue("userId", "U2")
	code, data, _ = removeProjectMember(app, removeGroupReq, platform.RouteSpec{})
	assertOrgStatus(t, code, data, http.StatusForbidden)
	removeDirectReq := orgProjectPathRequest(http.MethodDelete, "/api/v1/projects/P1/members/U3", "", "U1", "P1")
	removeDirectReq.SetPathValue("userId", "U3")
	code, data, _ = removeProjectMember(app, removeDirectReq, platform.RouteSpec{})
	assertOrgStatus(t, code, data, http.StatusOK)
}

func TestOrgProjectGPUClaimAndProjectDeleteCleanup(t *testing.T) {
	app := newOrgProjectTestApp(t)
	code, data, _ := createProject(app, orgRequest(http.MethodPost, "/api/v1/projects", `{"id":"P2","project_name":"claims","g_id":"G1"}`, "ADMIN"), platform.RouteSpec{})
	assertOrgStatus(t, code, data, http.StatusCreated)

	claimBody := `{"name":"claim-a","device_class_name":"gpu.nvidia.com","gpu_count":2,"sm_percentage":50,"vram_policy":"elastic","vram_percentage":80}`
	createReq := orgProjectPathRequest(http.MethodPost, "/api/v1/projects/P2/gpu-claims", claimBody, "U2", "P2")
	code, data, _ = createGPUClaim(app, createReq, platform.RouteSpec{})
	assertOrgStatus(t, code, data, http.StatusCreated)
	if claim := data.(map[string]any); claim["effective_gpu"] != float64(1) || claim["user_id"] != "U2" {
		t.Fatalf("created claim = %#v, want U2 effective GPU 1", claim)
	}
	listReq := orgProjectPathRequest(http.MethodGet, "/api/v1/projects/P2/gpu-claims?scope=mine", "", "U2", "P2")
	code, data, _ = listGPUClaims(app, listReq, platform.RouteSpec{})
	assertOrgStatus(t, code, data, http.StatusOK)
	if claims := data.([]map[string]any); len(claims) != 1 {
		t.Fatalf("claims = %#v, want one mine claim", claims)
	}
	deleteClaimReq := orgProjectPathRequest(http.MethodDelete, "/api/v1/projects/P2/gpu-claims/claim-a", "", "U1", "P2")
	deleteClaimReq.SetPathValue("requestId", "claim-a")
	code, data, _ = deleteGPUClaim(app, deleteClaimReq, platform.RouteSpec{})
	assertOrgStatus(t, code, data, http.StatusOK)

	addReq := orgProjectPathRequest(http.MethodPost, "/api/v1/projects/P2/members", `{"members":[{"user_id":"U3","role":"user"}]}`, "U1", "P2")
	code, data, _ = addProjectMembers(app, addReq, platform.RouteSpec{})
	assertOrgStatus(t, code, data, http.StatusOK)
	quotaReq := orgProjectPathRequest(http.MethodPut, "/api/v1/projects/P2/members/U3/quota", `{"gpu_limit":1}`, "U1", "P2")
	quotaReq.SetPathValue("userId", "U3")
	code, data, _ = upsertProjectMemberQuota(app, quotaReq, platform.RouteSpec{})
	assertOrgStatus(t, code, data, http.StatusOK)

	deleteProjectReq := orgProjectPathRequest(http.MethodDelete, "/api/v1/projects/P2", "", "ADMIN", "P2")
	code, data, _ = deleteProject(app, deleteProjectReq, platform.RouteSpec{})
	assertOrgStatus(t, code, data, http.StatusOK)
	if _, found := app.Store.Get(context.Background(), projectsResource, "P2"); found {
		t.Fatal("project was not deleted")
	}
	if got := app.Store.List(context.Background(), projectMembersResource); len(got) != 0 {
		t.Fatalf("project members after delete = %#v, want cleanup", got)
	}
	if got := app.Store.List(context.Background(), projectUserQuotasResource); len(got) != 0 {
		t.Fatalf("project quotas after delete = %#v, want cleanup", got)
	}
}

func TestOrgProjectBatchUpdateAndDeleteWorkflows(t *testing.T) {
	app := newOrgProjectTestApp(t)
	createOrgProjectForTest(t, app, "P3", "batch")
	createOrgProjectForTest(t, app, "P4", "cleanup")

	updateReq := orgProjectPathRequest(http.MethodPatch, "/api/v1/projects/P3", `{"description":"updated","allow_image_build":true}`, "ADMIN", "P3")
	code, data, _ := updateProject(app, updateReq, platform.RouteSpec{})
	assertOrgStatus(t, code, data, http.StatusOK)
	assertOrgMapValue(t, data, "description", "updated")

	code, data, _ = listProjectsByUser(app, orgRequest(http.MethodGet, "/api/v1/projects/by-user", "", "U2"), platform.RouteSpec{})
	assertOrgStatus(t, code, data, http.StatusOK)
	assertOrgRows(t, data, 2)

	contextReq := orgProjectPathRequest(http.MethodGet, "/api/v1/projects/P3/add-members-context", "", "U1", "P3")
	code, data, _ = projectAddMembersContext(app, contextReq, platform.RouteSpec{})
	assertOrgStatus(t, code, data, http.StatusOK)

	addReq := orgProjectPathRequest(http.MethodPost, "/api/v1/projects/P3/members", `{"user_ids":["U3","U4"],"role":"user"}`, "U1", "P3")
	code, data, _ = addProjectMembers(app, addReq, platform.RouteSpec{})
	assertOrgStatus(t, code, data, http.StatusOK)
	assertOrgMapValue(t, data, "succeeded", 2)

	roleReq := orgProjectPathRequest(http.MethodPut, "/api/v1/projects/P3/members/roles", `{"updates":[{"user_id":"U3","role":"manager"},{"user_id":"missing","role":"user"}]}`, "U1", "P3")
	code, data, _ = batchUpdateProjectMemberRoles(app, roleReq, platform.RouteSpec{})
	assertOrgStatus(t, code, data, http.StatusOK)
	assertOrgMapValue(t, data, "succeeded", 1)
	assertOrgMapValue(t, data, "failed", 1)

	workspaceReq := orgProjectPathRequest(http.MethodGet, "/api/v1/projects/P3/workspace-settings", "", "U3", "P3")
	code, data, _ = getProjectWorkspaceSettings(app, workspaceReq, platform.RouteSpec{})
	assertOrgStatus(t, code, data, http.StatusOK)
	assertOrgMapValue(t, data, "max_ide_runtime_seconds", 0)

	quotaReq := orgProjectPathRequest(http.MethodPut, "/api/v1/projects/P3/members/U3/quota", `{"gpu_limit":2}`, "U1", "P3")
	quotaReq.SetPathValue("userId", "U3")
	code, data, _ = upsertProjectMemberQuota(app, quotaReq, platform.RouteSpec{})
	assertOrgStatus(t, code, data, http.StatusOK)
	deleteQuotaReq := orgProjectPathRequest(http.MethodDelete, "/api/v1/projects/P3/members/U3/quota", "", "U1", "P3")
	deleteQuotaReq.SetPathValue("userId", "U3")
	code, data, _ = deleteProjectMemberQuota(app, deleteQuotaReq, platform.RouteSpec{})
	assertOrgStatus(t, code, data, http.StatusOK)
	assertOrgRecordMissing(t, app, projectUserQuotasResource, "P3:U3")

	removeReq := orgProjectPathRequest(http.MethodDelete, "/api/v1/projects/P3/members", `{"user_ids":["U4","missing"]}`, "U1", "P3")
	code, data, _ = batchRemoveProjectMembers(app, removeReq, platform.RouteSpec{})
	assertOrgStatus(t, code, data, http.StatusOK)
	assertOrgMapValue(t, data, "succeeded", 1)
	assertOrgMapValue(t, data, "failed", 1)

	batchDeleteReq := orgRequest(http.MethodDelete, "/api/v1/projects/batch", `{"ids":["P4","missing"]}`, "ADMIN")
	code, data, _ = batchDeleteProjects(app, batchDeleteReq, platform.RouteSpec{})
	assertOrgStatus(t, code, data, http.StatusOK)
	assertOrgMapValue(t, data, "succeeded", 1)
	assertOrgMapValue(t, data, "failed", 1)
	assertOrgRecordMissing(t, app, projectsResource, "P4")
}

func newOrgProjectTestApp(t *testing.T) *platform.App {
	t.Helper()
	app := platform.NewApp(platform.Config{ServiceName: "all", HTTPAddr: ":0"})
	Register(app)
	createOrgRecords(t, app, usersResource, []map[string]any{
		{"id": "ADMIN", "username": "admin", "email": "admin@test.local", "capabilities": map[string]any{"adminPanel": true}},
		{"id": "U1", "username": "alice", "email": "alice@test.local", "capabilities": map[string]any{"adminPanel": false}},
		{"id": "U2", "username": "bob", "email": "bob@test.local", "capabilities": map[string]any{"adminPanel": false}},
		{"id": "U3", "username": "carol", "email": "carol@test.local", "capabilities": map[string]any{"adminPanel": false}},
		{"id": "U4", "username": "dora", "email": "dora@test.local", "capabilities": map[string]any{"adminPanel": false}},
		{"id": "DISABLED", "username": "disabled", "status": "disabled"},
	})
	createOrgRecords(t, app, groupsResource, []map[string]any{
		{"id": "G1", "group_name": "vision", "name": "vision", "description": "main"},
		{"id": "G2", "group_name": "private", "name": "private"},
	})
	createOrgRecords(t, app, userGroupsResource, []map[string]any{
		{"id": "U1:G1", "user_id": "U1", "group_id": "G1", "role": "admin"},
		{"id": "U2:G1", "user_id": "U2", "group_id": "G1", "role": "user"},
	})
	return app
}

func createOrgRecords(t *testing.T, app *platform.App, resource string, rows []map[string]any) {
	t.Helper()
	for _, row := range rows {
		if _, err := app.Store.Create(context.Background(), resource, row); err != nil {
			t.Fatal(err)
		}
	}
}

func orgRequest(method, target, body, userID string) *http.Request {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if userID != "" {
		req.Header.Set("X-User-ID", userID)
	}
	return req
}

func orgProjectPathRequest(method, target, body, userID, projectID string) *http.Request {
	req := orgRequest(method, target, body, userID)
	req.SetPathValue("id", projectID)
	return req
}

func assertOrgStatus(t *testing.T, code int, data any, want int) {
	t.Helper()
	if code != want {
		t.Fatalf("status=%d data=%#v, want %d", code, data, want)
	}
}

func createOrgProjectForTest(t *testing.T, app *platform.App, id, name string) {
	t.Helper()
	createOrgRecords(t, app, projectsResource, []map[string]any{{"id": id, "project_name": name, "owner_id": "G1"}})
}

func assertOrgRows(t *testing.T, data any, want int) {
	t.Helper()
	rows := data.([]map[string]any)
	if len(rows) != want {
		t.Fatalf("rows = %#v, want %d", rows, want)
	}
}

func assertOrgStringSlice(t *testing.T, data []string, want int) {
	t.Helper()
	if len(data) != want {
		t.Fatalf("slice = %#v, want %d items", data, want)
	}
}

func assertOrgMapValue(t *testing.T, data any, key string, want any) {
	t.Helper()
	row := data.(map[string]any)
	if row[key] != want {
		t.Fatalf("%s = %#v, want %#v in %#v", key, row[key], want, row)
	}
}

func assertOrgRecordMissing(t *testing.T, app *platform.App, resource, id string) {
	t.Helper()
	if _, ok := app.Store.Get(context.Background(), resource, id); ok {
		t.Fatalf("%s/%s unexpectedly exists", resource, id)
	}
}
