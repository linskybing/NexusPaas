package services

import (
	"context"
	"net/http"
	"testing"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

const (
	ideTestAdminID                 = "ADMIN"
	ideTestBasePath                = "/api/v1/ide"
	ideTestImageRoot               = "jupyter-base-root"
	ideTestIdentityUsersResource   = "identity-service:users"
	ideTestIdentitySource          = "identity-service"
	ideTestPodName                 = "ide-u1-jupyter"
	ideTestProjectMembersResource  = "org-project-service:project_members"
	ideTestProjectID               = "P1"
	ideTestProjectSource           = "org-project-service"
	ideTestProjectsResource        = "org-project-service:projects"
	ideTestProjectedProjects       = "ide-service:ide_projects"
	ideTestProjectedProjectMembers = "ide-service:ide_project_members"
	ideTestServiceName             = "ide-service"
	ideTestUserID                  = "U1"
)

func TestIDEWorkspaceWorkflow(t *testing.T) {
	app := newTestApp()
	seedIDEData(t, app)

	images := responseSlice(t, requestJSON(t, app, http.MethodGet, "/api/v1/ide/images?ide_type=jupyter", "", nil, http.StatusOK))
	if len(images) == 0 || images[0].(map[string]any)["ide_type"] != "jupyter" {
		t.Fatalf("images = %#v, want jupyter images", images)
	}
	requestJSON(t, app, http.MethodGet, "/api/v1/ide/images?project_id=P1", "", nil, http.StatusUnauthorized)
	requestJSON(t, app, http.MethodGet, "/api/v1/ide", "", nil, http.StatusUnauthorized)
	requestJSON(t, app, http.MethodPost, "/api/v1/ide/start", "{", userHeaders("U1"), http.StatusBadRequest)
	requestJSON(t, app, http.MethodPost, "/api/v1/ide/start?gpu=bad", "", userHeaders("U1"), http.StatusBadRequest)
	requestJSON(t, app, http.MethodPost, "/api/v1/ide/start?gpu=NaN", "", userHeaders("U1"), http.StatusBadRequest)
	requestJSON(t, app, http.MethodPost, "/api/v1/ide/start?gpu=-1", "", userHeaders("U1"), http.StatusBadRequest)
	requestJSON(t, app, http.MethodPost, "/api/v1/ide/start?gpu=0", "", userHeaders("U1"), http.StatusBadRequest)
	requestJSON(t, app, http.MethodPost, "/api/v1/ide/start?project_id=P1&image_key=missing", "", userHeaders("U1"), http.StatusBadRequest)
	requestJSON(t, app, http.MethodPost, "/api/v1/ide/start?project_id=P1&image_key=jupyter-base&type=vscode", "", userHeaders("U1"), http.StatusBadRequest)
	requestJSON(t, app, http.MethodPost, "/api/v1/ide/start?project_id=P1&image_key=jupyter-base-root", "", userHeaders("U1"), http.StatusForbidden)
	requestJSON(t, app, http.MethodPost, "/api/v1/ide/start?project_id=P1&image_key=jupyter-base&executor_type=local", "", userHeaders("U1"), http.StatusForbidden)
	requestJSON(t, app, http.MethodPost, "/api/v1/ide/start?project_id=P1&image_key=jupyter-base", `{"sm_percentage":0}`, userHeaders("U1"), http.StatusBadRequest)
	requestJSON(t, app, http.MethodPost, "/api/v1/ide/start?project_id=P1&image_key=jupyter-base", `{"pinned_memory_limit":"not-a-quantity"}`, userHeaders("U1"), http.StatusBadRequest)
	requestJSON(t, app, http.MethodPost, "/api/v1/ide/start?project_id=P1&image_key=jupyter-base", `{"device_class_name":"Bad_Device"}`, userHeaders("U1"), http.StatusBadRequest)

	started := responseMap(t, requestJSON(t, app, http.MethodPost, "/api/v1/ide/start?project_id=P1&image_key=jupyter-base&gpu=0", `{"storage_ids":["S1"],"queue_name":"q1"}`, userHeaders("U1"), http.StatusOK))
	if started["status"] != "started" || started["pod_name"] != "ide-u1-jupyter" {
		t.Fatalf("started = %#v, want started pod response", started)
	}
	async := responseMap(t, requestJSON(t, app, http.MethodPost, "/api/v1/ide/start?project_id=P1&image_key=jupyter-base", `{"blocking":false}`, userHeaders("U1"), http.StatusOK))
	if async["status"] != "submitted" {
		t.Fatalf("async start = %#v, want submitted", async)
	}
	myIDEs := responseSlice(t, requestJSON(t, app, http.MethodGet, "/api/v1/ide", "", userHeaders("U1"), http.StatusOK))
	if len(myIDEs) != 1 || myIDEs[0].(map[string]any)["pod_name"] != "ide-u1-jupyter" {
		t.Fatalf("my IDEs = %#v, want own IDE", myIDEs)
	}
	requestJSON(t, app, http.MethodGet, "/api/v1/ide?project_id=P2", "", userHeaders("U1"), http.StatusForbidden)
	projectIDEs := responseSlice(t, requestJSON(t, app, http.MethodGet, "/api/v1/ide?project_id=P1", "", userHeaders("U1"), http.StatusOK))
	if len(projectIDEs) != 1 {
		t.Fatalf("project IDEs = %#v, want visible project IDE", projectIDEs)
	}
	requestJSON(t, app, http.MethodPost, "/api/v1/ide/stop?type=bad", "", userHeaders("U1"), http.StatusBadRequest)
	requestJSON(t, app, http.MethodPost, "/api/v1/ide/stop?project_id=P1&type=jupyter", "", map[string]string{"X-User-ID": "U1"}, http.StatusUnauthorized)
	requestJSON(t, app, http.MethodPost, "/api/v1/ide/stop?project_id=P1&type=jupyter", "", userHeaders("U1"), http.StatusOK)
	requestJSON(t, app, http.MethodPost, "/api/v1/ide/delete?project_id=P1&type=jupyter", "", userHeaders("U1"), http.StatusOK)
}

func TestIDEWorkspaceUsesEventFedReadModelsInIsolatedService(t *testing.T) {
	app := platform.NewApp(platform.Config{
		ServiceName:  ideTestServiceName,
		HTTPAddr:     ":0",
		APIKeys:      map[string]bool{"test-key": true},
		ExternalURLs: map[string]string{},
	})
	RegisterAll(app)
	if err := app.ValidateServiceIsolation(); err != nil {
		t.Fatalf("ide service isolation = %v, want event-fed read models", err)
	}

	publishDashboardTestEvent(t, app, "UserCreated", ideTestIdentitySource, map[string]any{
		"id":       ideTestUserID,
		"username": "alice",
	})
	publishDashboardTestEvent(t, app, "ProjectCreated", ideTestProjectSource, map[string]any{
		"id":                ideTestProjectID,
		"p_id":              ideTestProjectID,
		"project_name":      "vision",
		"allow_run_as_root": true,
	})
	publishDashboardTestEvent(t, app, "project_memberCreated", ideTestProjectSource, map[string]any{
		"project_id": ideTestProjectID,
		"user_id":    ideTestUserID,
		"role":       "manager",
	})

	images := responseSlice(t, requestJSON(t, app, http.MethodGet, ideTestBasePath+"/images?project_id="+ideTestProjectID, "", userHeaders(ideTestUserID), http.StatusOK))
	if len(images) == 0 || !containsImageKey(images, ideTestImageRoot) {
		t.Fatalf("project images = %#v, want root image allowed by local project read model", images)
	}
	started := responseMap(t, requestJSON(t, app, http.MethodPost, ideTestBasePath+"/start?project_id="+ideTestProjectID+"&image_key="+ideTestImageRoot+"&gpu=0", "", userHeaders(ideTestUserID), http.StatusOK))
	if started["pod_name"] != ideTestPodName {
		t.Fatalf("started = %#v, want projected project/member access", started)
	}
	projectIDEs := responseSlice(t, requestJSON(t, app, http.MethodGet, ideTestBasePath+"?project_id="+ideTestProjectID, "", userHeaders(ideTestUserID), http.StatusOK))
	if len(projectIDEs) != 1 {
		t.Fatalf("project IDEs = %#v, want session visible through projected member role", projectIDEs)
	}

	if len(app.Store.List(context.Background(), ideTestIdentityUsersResource)) != 0 {
		t.Fatal("isolated IDE test should not seed source identity users")
	}
	if len(app.Store.List(context.Background(), ideTestProjectsResource)) != 0 {
		t.Fatal("isolated IDE test should not seed source org projects")
	}
	if len(app.Store.List(context.Background(), ideTestProjectMembersResource)) != 0 {
		t.Fatal("isolated IDE test should not seed source project members")
	}
	if _, ok := app.Store.Get(context.Background(), ideTestProjectedProjects, ideTestProjectID); !ok {
		t.Fatal("missing local projected IDE project read model")
	}
	if _, ok := app.Store.Get(context.Background(), ideTestProjectedProjectMembers, ideTestProjectID+":"+ideTestUserID); !ok {
		t.Fatal("missing local projected IDE project member read model")
	}
}

func containsImageKey(images []any, key string) bool {
	for _, image := range images {
		if row, ok := image.(map[string]any); ok && row["key"] == key {
			return true
		}
	}
	return false
}

func seedIDEData(t *testing.T, platformApp *platform.App) {
	t.Helper()
	createRows(t, platformApp, ideTestIdentityUsersResource, []map[string]any{
		{"id": ideTestUserID, "username": "alice", "capabilities": map[string]any{"adminPanel": false}},
		{"id": ideTestAdminID, "username": "admin", "capabilities": map[string]any{"adminPanel": true}},
	})
	createRows(t, platformApp, ideTestProjectsResource, []map[string]any{
		{"id": ideTestProjectID, "p_id": ideTestProjectID, "project_name": "vision", "allow_run_as_root": false},
		{"id": "P2", "p_id": "P2", "project_name": "private"},
	})
	createRows(t, platformApp, ideTestProjectMembersResource, []map[string]any{
		{"id": "pm1", "project_id": ideTestProjectID, "user_id": ideTestUserID, "role": "user"},
	})
}
