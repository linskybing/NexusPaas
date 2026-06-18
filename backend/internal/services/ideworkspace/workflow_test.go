package ideworkspace

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

func TestIDEImageAndLifecycleHandlers(t *testing.T) {
	app := newIDETestApp(t)

	code, data, _ := listImages(app, ideRequest(http.MethodGet, "/api/v1/ide/images?ide_type=jupyter", "", ""), platform.RouteSpec{})
	if code != http.StatusOK {
		t.Fatalf("list images status=%d data=%#v, want 200", code, data)
	}
	images := data.([]Image)
	if len(images) == 0 || images[0].IDEType != "jupyter" || containsIDEImage(images, "jupyter-base-root") {
		t.Fatalf("public images = %#v, want jupyter images without root profiles", images)
	}

	code, data, _ = startIDE(app, ideRequest(http.MethodPost, "/api/v1/ide/start?project_id=P1&image_key=jupyter-base&gpu=0", `{"storage_ids":["S1"],"queue_name":"q1"}`, "U1"), platform.RouteSpec{})
	if code != http.StatusOK || data.(map[string]any)["pod_name"] != "ide-u1-jupyter" {
		t.Fatalf("start IDE status=%d data=%#v, want started pod", code, data)
	}
	code, data, _ = startIDE(app, ideRequest(http.MethodPost, "/api/v1/ide/start?project_id=P1&image_key=jupyter-base", `{"blocking":false}`, "U1"), platform.RouteSpec{})
	if code != http.StatusOK || data.(map[string]any)["status"] != "submitted" {
		t.Fatalf("async start status=%d data=%#v, want submitted", code, data)
	}

	addIDESession(t, app, "ide-u2-jupyter", "U2", "P1")
	code, data, _ = listIDEs(app, ideRequest(http.MethodGet, "/api/v1/ide", "", "U1"), platform.RouteSpec{})
	if code != http.StatusOK || len(data.([]map[string]any)) != 1 {
		t.Fatalf("own IDE list status=%d data=%#v, want only U1 session", code, data)
	}
	code, data, _ = listIDEs(app, ideRequest(http.MethodGet, "/api/v1/ide?project_id=P1", "", "U1"), platform.RouteSpec{})
	if code != http.StatusOK || len(data.([]map[string]any)) != 2 {
		t.Fatalf("project IDE list status=%d data=%#v, want manager to see project sessions", code, data)
	}

	code, data, _ = stopIDE(app, ideRequest(http.MethodPost, "/api/v1/ide/stop?project_id=P1&type=jupyter", "", "U1"), platform.RouteSpec{})
	if code != http.StatusOK || data != nil {
		t.Fatalf("stop IDE status=%d data=%#v, want 200 with no body", code, data)
	}
	code, _, _ = deleteIDE(app, ideRequest(http.MethodPost, "/api/v1/ide/delete?project_id=P1&type=jupyter", "", "U2"), platform.RouteSpec{})
	if code != http.StatusOK {
		t.Fatalf("delete IDE status=%d, want 200", code)
	}
}

func TestIDEPolicyAccessAndValidation(t *testing.T) {
	app := newIDETestApp(t)

	code, data, _ := listImages(app, ideRequest(http.MethodGet, "/api/v1/ide/images?project_id=P3", "", "U1"), platform.RouteSpec{})
	if code != http.StatusOK || !containsIDEImage(data.([]Image), "jupyter-base-root") {
		t.Fatalf("project root images status=%d data=%#v, want root profile via group-owned project", code, data)
	}
	code, data, _ = listIDEs(app, ideRequest(http.MethodGet, "/api/v1/ide?project_id=P2", "", "U1"), platform.RouteSpec{})
	if code != http.StatusForbidden {
		t.Fatalf("private project IDE list status=%d data=%#v, want forbidden", code, data)
	}

	cases := []struct {
		name   string
		target string
		body   string
		userID string
		want   int
	}{
		{name: "bad json", target: "/api/v1/ide/start", body: "{", userID: "U1", want: http.StatusBadRequest},
		{name: "bad gpu", target: "/api/v1/ide/start?gpu=bad", userID: "U1", want: http.StatusBadRequest},
		{name: "negative gpu", target: "/api/v1/ide/start?project_id=P1&gpu=-1", userID: "U1", want: http.StatusBadRequest},
		{name: "missing project", target: "/api/v1/ide/start?gpu=0", userID: "U1", want: http.StatusBadRequest},
		{name: "invalid type", target: "/api/v1/ide/start?project_id=P1&type=terminal", userID: "U1", want: http.StatusBadRequest},
		{name: "unknown image", target: "/api/v1/ide/start?project_id=P1&image_key=missing", userID: "U1", want: http.StatusBadRequest},
		{name: "incompatible image", target: "/api/v1/ide/start?project_id=P1&type=vscode&image_key=jupyter-base", userID: "U1", want: http.StatusBadRequest},
		{name: "root denied", target: "/api/v1/ide/start?project_id=P1&image_key=jupyter-base-root", userID: "U1", want: http.StatusForbidden},
		{name: "executor override denied", target: "/api/v1/ide/start?project_id=P1&image_key=jupyter-base&executor_type=local", userID: "U1", want: http.StatusForbidden},
		{name: "bad sm", target: "/api/v1/ide/start?project_id=P1&image_key=jupyter-base", body: `{"sm_percentage":0}`, userID: "U1", want: http.StatusBadRequest},
		{name: "bad memory", target: "/api/v1/ide/start?project_id=P1&image_key=jupyter-base", body: `{"pinned_memory_limit":"not-a-quantity"}`, userID: "U1", want: http.StatusBadRequest},
		{name: "bad device class", target: "/api/v1/ide/start?project_id=P1&image_key=jupyter-base", body: `{"device_class_name":"Bad_Device"}`, userID: "U1", want: http.StatusBadRequest},
		{name: "not member", target: "/api/v1/ide/start?project_id=P2&image_key=jupyter-base", userID: "U1", want: http.StatusForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, data, _ := startIDE(app, ideRequest(http.MethodPost, tc.target, tc.body, tc.userID), platform.RouteSpec{})
			if code != tc.want {
				t.Fatalf("status=%d data=%#v, want %d", code, data, tc.want)
			}
		})
	}
}

func TestIDEIdentityAndLifecycleGuards(t *testing.T) {
	app := newIDETestApp(t)

	code, data, _ := listImages(app, ideRequest(http.MethodGet, "/api/v1/ide/images?project_id=P1", "", ""), platform.RouteSpec{})
	if code != http.StatusUnauthorized {
		t.Fatalf("project images without user status=%d data=%#v, want unauthorized", code, data)
	}
	code, data, _ = listIDEs(app, ideRequest(http.MethodGet, "/api/v1/ide", "", ""), platform.RouteSpec{})
	if code != http.StatusUnauthorized {
		t.Fatalf("list IDEs without user status=%d data=%#v, want unauthorized", code, data)
	}
	code, data, _ = startIDE(app, ideRequestWithoutUsername("/api/v1/ide/start?project_id=P1&image_key=jupyter-base", "U1"), platform.RouteSpec{})
	if code != http.StatusUnauthorized {
		t.Fatalf("start without username status=%d data=%#v, want unauthorized", code, data)
	}
	code, data, _ = stopIDE(app, ideRequest(http.MethodPost, "/api/v1/ide/stop?project_id=P1&type=bad", "", "U1"), platform.RouteSpec{})
	if code != http.StatusBadRequest {
		t.Fatalf("stop bad type status=%d data=%#v, want bad request", code, data)
	}
}

func TestIDEProjectionLifecycle(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: serviceName, HTTPAddr: ":0"})
	req := ideRequest(http.MethodGet, "/", "", "ADMIN")

	projectIDEEvent(app, req, ideEvent("UserCreated", map[string]any{"new": map[string]any{"id": "U1", "username": "alice"}}))
	projectIDEEvent(app, req, ideEvent("ProjectCreated", map[string]any{"project": map[string]any{"p_id": "P1", "project_name": "vision"}}))
	projectIDEEvent(app, req, ideEvent("project_memberCreated", map[string]any{"member": map[string]any{"project_id": "P1", "user_id": "U1", "role": "manager"}}))
	projectIDEEvent(app, req, ideEvent("GroupMembershipChanged", map[string]any{"user_id": "U1", "group_id": "G1"}))
	projectIDEEvent(app, req, ideEvent("ProxyPolicyChanged", map[string]any{"action": "role_create", "role_id": "R1", "admin_panel": true}))

	assertIDEReadModelExists(t, app, ideIdentityUsersResource, "U1")
	assertIDEReadModelExists(t, app, ideProjectsResource, "P1")
	assertIDEReadModelExists(t, app, ideProjectMembersResource, "P1:U1")
	assertIDEReadModelExists(t, app, ideUserGroupsResource, "U1:G1")
	assertIDEReadModelExists(t, app, idePolicyRolesResource, "R1")

	projectIDEEvent(app, req, ideEvent("GroupMembershipChanged", map[string]any{"user_id": "U1", "group_id": "G1", "action": "delete"}))
	if _, ok := app.Store.Get(context.Background(), ideUserGroupsResource, "U1:G1"); ok {
		t.Fatal("deleted projected group membership still exists")
	}
	if resource, _, _, ok := ideProjection(ideEvent("Unknown", nil)); ok || resource != "" {
		t.Fatalf("unknown projection resource=%q ok=%v, want ignored", resource, ok)
	}
	if records := ideRecords(nil, req, ideProjectsResource, orgProjectsResource); records != nil {
		t.Fatalf("nil app records = %#v, want nil", records)
	}
}

func TestIDEHelperConversions(t *testing.T) {
	payload := map[string]any{
		"text":  " value ",
		"float": json.Number("2.5"),
		"slice": []any{"a", 2, "b"},
		"bool":  "true",
		"map":   map[string]any{"adminPanel": true},
	}
	if textValue(payload, "text") != "value" || floatValue(payload, "float") != 2.5 {
		t.Fatalf("text/float conversions failed for %#v", payload)
	}
	if got := stringSlice(payload["slice"]); len(got) != 2 || got[1] != "b" {
		t.Fatalf("string slice = %#v, want string-only values", got)
	}
	if !boolValue(payload, "bool") || !recordGrantsAdminPanel(mapValue(payload, "map")) {
		t.Fatalf("bool/map conversions failed for %#v", payload)
	}
}

func newIDETestApp(t *testing.T) *platform.App {
	t.Helper()
	app := platform.NewApp(platform.Config{ServiceName: "all", HTTPAddr: ":0"})
	Register(app)
	createIDERecords(t, app, identityUsersResource, []map[string]any{
		{"id": "U1", "username": "alice", "capabilities": map[string]any{"adminPanel": false}},
		{"id": "U2", "username": "bob", "capabilities": map[string]any{"adminPanel": false}},
		{"id": "ADMIN", "username": "admin", "capabilities": map[string]any{"adminPanel": true}},
	})
	createIDERecords(t, app, orgProjectsResource, []map[string]any{
		{"id": "P1", "p_id": "P1", "project_name": "vision", "allow_run_as_root": false},
		{"id": "P2", "p_id": "P2", "project_name": "private", "allow_run_as_root": false},
		{"id": "P3", "p_id": "P3", "owner_id": "G1", "project_name": "group", "allow_run_as_root": true},
	})
	createIDERecords(t, app, orgProjectMembersResource, []map[string]any{
		{"id": "pm1", "project_id": "P1", "user_id": "U1", "role": "manager"},
		{"id": "pm2", "project_id": "P1", "user_id": "U2", "role": "user"},
	})
	createIDERecords(t, app, orgUserGroupsResource, []map[string]any{
		{"id": "ug1", "group_id": "G1", "user_id": "U1"},
	})
	return app
}

func createIDERecords(t *testing.T, app *platform.App, resource string, rows []map[string]any) {
	t.Helper()
	for _, row := range rows {
		if _, err := app.Store.Create(context.Background(), resource, row); err != nil {
			t.Fatal(err)
		}
	}
}

func addIDESession(t *testing.T, app *platform.App, id, userID, projectID string) {
	t.Helper()
	_, err := app.Store.Create(context.Background(), sessionsResource, map[string]any{
		"id":         id,
		"pod_name":   id,
		"project_id": projectID,
		"user_id":    userID,
		"username":   strings.ToLower(userID),
		"ide_type":   "jupyter",
		"status":     "running",
	})
	if err != nil {
		t.Fatal(err)
	}
}

func ideRequest(method, target, body, userID string) *http.Request {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if userID != "" {
		req.Header.Set("X-User-ID", userID)
		req.Header.Set("X-Username", strings.ToLower(userID))
	}
	return req
}

func ideRequestWithoutUsername(target, userID string) *http.Request {
	req := ideRequest(http.MethodPost, target, "", userID)
	req.Header.Del("X-Username")
	return req
}

func containsIDEImage(images []Image, key string) bool {
	for _, image := range images {
		if image.Key == key {
			return true
		}
	}
	return false
}

func ideEvent(name string, data map[string]any) contracts.Event {
	return contracts.Event{Name: name, Data: data}
}

func assertIDEReadModelExists(t *testing.T, app *platform.App, resource, id string) {
	t.Helper()
	if _, ok := app.Store.Get(context.Background(), resource, id); !ok {
		t.Fatalf("missing read model %s %s", resource, id)
	}
}
