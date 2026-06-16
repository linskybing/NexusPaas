package services

import (
	"context"
	"net/http"
	"testing"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

func TestOrgProjectGroupWorkflow(t *testing.T) {
	app := newTestApp()
	seedOrgProjectData(t, app)
	app.Config.GroupStorageClassOptions = []string{"fast", "archive"}
	app.Config.GroupRegistryProfileOptions = []string{"default", "gpu"}

	requestJSON(t, app, http.MethodGet, "/api/v1/groups", "", nil, http.StatusUnauthorized)
	requestJSON(t, app, http.MethodPost, "/api/v1/groups", `{"group_name":"bad"}`, adminHeaders("forged"), http.StatusForbidden)
	requestJSON(t, app, http.MethodPost, "/api/v1/groups", `{"group_name":"bad","storage_class":"missing"}`, adminHeaders("ADMIN"), http.StatusBadRequest)

	created := responseMap(t, requestJSON(t, app, http.MethodPost, "/api/v1/groups", `{"id":"G3","group_name":"research","storage_class":"fast","registry_profile":"gpu","allow_run_as_root":true}`, adminHeaders("ADMIN"), http.StatusCreated))
	if created["id"] != "G3" || created["storage_class"] != "fast" {
		t.Fatalf("created group = %#v", created)
	}
	groupsBefore := len(app.Store.List(t.Context(), "org-project-service:groups"))
	eventsBefore := countEvents(app, "GroupCreated")
	requestJSON(t, app, http.MethodPost, "/api/v1/groups", `{"id":"G3","group_name":"research-duplicate","storage_class":"fast"}`, adminHeaders("ADMIN"), http.StatusConflict)
	if groupsAfter := len(app.Store.List(t.Context(), "org-project-service:groups")); groupsAfter != groupsBefore {
		t.Fatalf("group count = %d, want unchanged %d", groupsAfter, groupsBefore)
	}
	if eventsAfter := countEvents(app, "GroupCreated"); eventsAfter != eventsBefore {
		t.Fatalf("GroupCreated events = %d, want unchanged %d", eventsAfter, eventsBefore)
	}
	options := responseMap(t, requestJSON(t, app, http.MethodGet, "/api/v1/admin/group-policy-options", "", adminHeaders("ADMIN"), http.StatusOK))
	if len(options["storage_classes"].([]any)) != 2 || len(options["registry_profiles"].([]any)) != 2 {
		t.Fatalf("policy options = %#v", options)
	}

	page := responseMap(t, requestJSON(t, app, http.MethodGet, "/api/v1/groups", "", userHeaders("U2"), http.StatusOK))
	if page["total"] != float64(1) || page["list"].([]any)[0].(map[string]any)["id"] != "G1" {
		t.Fatalf("user groups = %#v, want only member group G1", page)
	}
	requestJSON(t, app, http.MethodGet, "/api/v1/groups/G2", "", userHeaders("U2"), http.StatusForbidden)
	requestJSON(t, app, http.MethodGet, "/api/v1/groups/G1", "", userHeaders("U2"), http.StatusOK)

	updated := responseMap(t, requestJSON(t, app, http.MethodPut, "/api/v1/groups/G3", `{"description":"updated","storage_class":"archive"}`, adminHeaders("ADMIN"), http.StatusOK))
	if updated["description"] != "updated" || updated["storage_class"] != "archive" {
		t.Fatalf("updated group = %#v", updated)
	}
	requestJSON(t, app, http.MethodDelete, "/api/v1/groups/batch", `{"ids":["G3"]}`, adminHeaders("ADMIN"), http.StatusOK)
	requestJSON(t, app, http.MethodGet, "/api/v1/groups/G3", "", adminHeaders("ADMIN"), http.StatusNotFound)
}

func TestOrgProjectUserGroupWorkflow(t *testing.T) {
	app := newTestApp()
	seedOrgProjectData(t, app)

	requestJSON(t, app, http.MethodPost, "/api/v1/user-groups", `{"uid":"U3","gid":"G1","role":"user"}`, userHeaders("U2"), http.StatusForbidden)
	requestJSON(t, app, http.MethodPost, "/api/v1/user-groups", `{"uid":"U3","gid":"G1","role":"admin"}`, userHeaders("U1"), http.StatusForbidden)
	requestJSON(t, app, http.MethodPost, "/api/v1/user-groups", `{"uid":"DISABLED","gid":"G1","role":"user"}`, userHeaders("U1"), http.StatusBadRequest)

	added := responseMap(t, requestJSON(t, app, http.MethodPost, "/api/v1/user-groups", `{"uid":"U3","gid":"G1","role":"user"}`, userHeaders("U1"), http.StatusOK))
	if added["user_id"] != "U3" || added["role"] != "user" {
		t.Fatalf("added membership = %#v", added)
	}
	membershipsBefore := len(app.Store.List(t.Context(), "org-project-service:user_groups"))
	eventsBefore := countEvents(app, "GroupMembershipChanged")
	requestJSON(t, app, http.MethodPost, "/api/v1/user-groups", `{"uid":"U3","gid":"G1","role":"user"}`, userHeaders("U1"), http.StatusBadRequest)
	if membershipsAfter := len(app.Store.List(t.Context(), "org-project-service:user_groups")); membershipsAfter != membershipsBefore {
		t.Fatalf("membership count = %d, want unchanged %d", membershipsAfter, membershipsBefore)
	}
	if eventsAfter := countEvents(app, "GroupMembershipChanged"); eventsAfter != eventsBefore {
		t.Fatalf("GroupMembershipChanged events = %d, want unchanged %d", eventsAfter, eventsBefore)
	}
	requestJSON(t, app, http.MethodPut, "/api/v1/user-groups", `{"uid":"U3","gid":"G1","role":"manager"}`, userHeaders("U1"), http.StatusOK)
	requestJSON(t, app, http.MethodPut, "/api/v1/user-groups", `{"uid":"U3","gid":"G1","role":"admin"}`, adminHeaders("ADMIN"), http.StatusOK)

	byGroup := responseMap(t, requestJSON(t, app, http.MethodGet, "/api/v1/user-groups/by-group?g_id=G1", "", userHeaders("U2"), http.StatusOK))
	users := byGroup["G1"].(map[string]any)["users"].([]any)
	if len(users) != 3 {
		t.Fatalf("by group = %#v, want three members", byGroup)
	}
	requestJSON(t, app, http.MethodGet, "/api/v1/user-groups/by-user?u_id=U3", "", userHeaders("U2"), http.StatusForbidden)
	byUser := responseSlice(t, requestJSON(t, app, http.MethodGet, "/api/v1/user-groups/by-user?u_id=U3", "", userHeaders("U3"), http.StatusOK))
	if len(byUser) != 1 || byUser[0].(map[string]any)["group_id"] != "G1" {
		t.Fatalf("by user = %#v", byUser)
	}

	members := responseSlice(t, requestJSON(t, app, http.MethodGet, "/api/v1/user-groups/G1/members", "", userHeaders("U2"), http.StatusOK))
	if len(members) != 3 {
		t.Fatalf("members = %#v, want three", members)
	}
	context := responseMap(t, requestJSON(t, app, http.MethodGet, "/api/v1/user-groups/G1/add-members-context", "", userHeaders("U1"), http.StatusOK))
	available := context["available_users"].(map[string]any)
	if available["total"] != float64(2) {
		t.Fatalf("add-members context = %#v, want two available visible non-members", context)
	}
	resolved := responseMap(t, requestJSON(t, app, http.MethodPost, "/api/v1/user-groups/G1/resolve-add-members", `{"identifiers":["U4","bob","missing","U4"]}`, userHeaders("U1"), http.StatusOK))
	if len(resolved["resolved"].([]any)) != 1 || len(resolved["already_members"].([]any)) != 1 || len(resolved["unresolved"].([]any)) != 1 {
		t.Fatalf("resolved = %#v", resolved)
	}

	requestJSON(t, app, http.MethodDelete, "/api/v1/user-groups?uid=U3&gid=G1", "", userHeaders("U1"), http.StatusOK)
	requestJSON(t, app, http.MethodGet, "/api/v1/user-groups?uid=U3&gid=G1", "", userHeaders("U3"), http.StatusNotFound)
}

func TestOrgProjectMalformedJSONDoesNotWrite(t *testing.T) {
	app := newTestApp()
	seedOrgProjectData(t, app)
	ctx := context.Background()

	groupsBefore := len(app.Store.List(ctx, "org-project-service:groups"))
	requestJSON(t, app, http.MethodPost, "/api/v1/groups", `{`, adminHeaders("ADMIN"), http.StatusBadRequest)
	requestJSON(t, app, http.MethodPut, "/api/v1/groups/G1", `{`, adminHeaders("ADMIN"), http.StatusBadRequest)
	requestJSON(t, app, http.MethodDelete, "/api/v1/groups/batch", `{`, adminHeaders("ADMIN"), http.StatusBadRequest)
	if groupsAfter := len(app.Store.List(ctx, "org-project-service:groups")); groupsAfter != groupsBefore {
		t.Fatalf("group count = %d, want unchanged %d", groupsAfter, groupsBefore)
	}
	group, ok := app.Store.Get(ctx, "org-project-service:groups", "G1")
	if !ok {
		t.Fatal("G1 was deleted by malformed group request")
	}
	if _, ok := group.Data["updated_at"]; ok {
		t.Fatalf("G1 was updated by malformed group request: %#v", group.Data)
	}

	membershipsBefore := len(app.Store.List(ctx, "org-project-service:user_groups"))
	for _, tc := range []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/v1/user-groups"},
		{http.MethodPut, "/api/v1/user-groups"},
		{http.MethodPost, "/api/v1/user-groups/batch"},
	} {
		requestJSON(t, app, tc.method, tc.path, `{`, adminHeaders("ADMIN"), http.StatusBadRequest)
	}
	if membershipsAfter := len(app.Store.List(ctx, "org-project-service:user_groups")); membershipsAfter != membershipsBefore {
		t.Fatalf("membership count = %d, want unchanged %d", membershipsAfter, membershipsBefore)
	}
}

func seedOrgProjectData(t *testing.T, app *platform.App) {
	t.Helper()
	createRows(t, app, "identity-service:users", []map[string]any{
		{"id": "ADMIN", "username": "admin", "email": "admin@test.local", "capabilities": map[string]any{"adminPanel": true}},
		{"id": "U1", "username": "alice", "email": "alice@test.local", "capabilities": map[string]any{"adminPanel": false}},
		{"id": "U2", "username": "bob", "email": "bob@test.local", "capabilities": map[string]any{"adminPanel": false}},
		{"id": "U3", "username": "carol", "email": "carol@test.local", "capabilities": map[string]any{"adminPanel": false}},
		{"id": "U4", "username": "dora", "email": "dora@test.local", "capabilities": map[string]any{"adminPanel": false}},
		{"id": "DISABLED", "username": "disabled", "status": "disabled", "capabilities": map[string]any{"adminPanel": false}},
	})
	createRows(t, app, "org-project-service:groups", []map[string]any{
		{"id": "G1", "group_name": "vision", "name": "vision", "description": "main"},
		{"id": "G2", "group_name": "private", "name": "private"},
	})
	createRows(t, app, "org-project-service:user_groups", []map[string]any{
		{"id": "U1:G1", "user_id": "U1", "group_id": "G1", "role": "admin"},
		{"id": "U2:G1", "user_id": "U2", "group_id": "G1", "role": "user"},
	})
}
