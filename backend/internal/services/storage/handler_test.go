package storage

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

func TestStorageGroupPermissionsAndUserStorageWorkflow(t *testing.T) {
	app := newStorageTestApp(t)
	app.Config.StorageClassOptions = []string{"standard", "fast"}

	code, data, _ := listStorageOptions(app, storageRequest(http.MethodGet, "/api/v1/storage/options", "", "U2"), platform.RouteSpec{})
	assertStorageStatus(t, code, data, http.StatusOK)
	if options := data.(map[string]any); len(options["storage_classes"].([]string)) != 2 {
		t.Fatalf("storage options = %#v, want env classes", options)
	}

	code, data, _ = listAdminGroupStorage(app, storageRequest(http.MethodGet, "/api/v1/admin/group-storage", "", "U2"), platform.RouteSpec{})
	assertStorageStatus(t, code, data, http.StatusForbidden)
	code, data, _ = listAdminGroupStorage(app, storageRequest(http.MethodGet, "/api/v1/admin/group-storage", "", "ADMIN"), platform.RouteSpec{})
	assertStorageStatus(t, code, data, http.StatusOK)

	groupReq := storagePathRequest(http.MethodGet, "/api/v1/storage/group/G1", "", "U2", "G1")
	code, data, _ = listGroupStorage(app, groupReq, platform.RouteSpec{})
	assertStorageStatus(t, code, data, http.StatusOK)
	if rows := data.([]map[string]any); len(rows) != 1 {
		t.Fatalf("group storage = %#v, want seeded PVC", rows)
	}
	code, data, _ = listMyStorages(app, storageRequest(http.MethodGet, "/api/v1/storage/my-storages", "", "U2"), platform.RouteSpec{})
	assertStorageStatus(t, code, data, http.StatusOK)
	if rows := data.([]map[string]any); len(rows) != 1 {
		t.Fatalf("my storages = %#v, want one", rows)
	}

	createReq := storagePathRequest(http.MethodPost, "/api/v1/storage/G1/storage", `{"name":"scratch","size":"20Gi","storage_class":"fast"}`, "ADMIN", "G1")
	code, data, _ = createGroupStorage(app, createReq, platform.RouteSpec{})
	assertStorageStatus(t, code, data, http.StatusCreated)
	if data.(map[string]any)["storage_class"] != "fast" {
		t.Fatalf("created storage = %#v", data)
	}
	startReq := storagePathRequest(http.MethodPost, "/api/v1/storage/G1/storage/pvc1/start", "", "U2", "G1")
	startReq.SetPathValue("pvcId", "pvc1")
	code, data, _ = startGroupStorage(app, startReq, platform.RouteSpec{})
	assertStorageStatus(t, code, data, http.StatusOK)
	if data.(map[string]any)["status"] != "running" {
		t.Fatalf("started storage = %#v", data)
	}

	permReq := storageRequest(http.MethodPost, "/api/v1/storage/permissions", `{"group_id":"G1","pvc_id":"pvc1","user_id":"U2","permission":"read_write"}`, "U1")
	code, data, _ = createStoragePermission(app, permReq, platform.RouteSpec{})
	assertStorageStatus(t, code, data, http.StatusOK)
	if data.(map[string]any)["permission"] != "read_write" {
		t.Fatalf("permission = %#v", data)
	}
	listPermReq := storageRequest(http.MethodGet, "/api/v1/storage/permissions/group/G1/pvc/pvc1/list", "", "U2")
	listPermReq.SetPathValue("group_id", "G1")
	listPermReq.SetPathValue("pvc_id", "pvc1")
	code, data, _ = listStoragePermissions(app, listPermReq, platform.RouteSpec{})
	assertStorageStatus(t, code, data, http.StatusOK)
	if permissions := data.(map[string]any)["permissions"].([]map[string]any); len(permissions) != 1 {
		t.Fatalf("permissions = %#v, want one", data)
	}

	policyReq := storageRequest(http.MethodPost, "/api/v1/storage/policies", `{"group_id":"G1","pvc_id":"pvc1","default_permission":"read_only"}`, "U1")
	code, data, _ = createStoragePolicy(app, policyReq, platform.RouteSpec{})
	assertStorageStatus(t, code, data, http.StatusOK)
	policyGetReq := storageRequest(http.MethodGet, "/api/v1/storage/permissions/group/G1/pvc/pvc1/policy", "", "U2")
	policyGetReq.SetPathValue("group_id", "G1")
	policyGetReq.SetPathValue("pvc_id", "pvc1")
	code, data, _ = getStoragePolicy(app, policyGetReq, platform.RouteSpec{})
	assertStorageStatus(t, code, data, http.StatusOK)
	if data.(map[string]any)["default_permission"] != "read_only" {
		t.Fatalf("policy = %#v", data)
	}

	batchDeleteReq := storageRequest(http.MethodDelete, "/api/v1/storage/permissions/batch", `{"group_id":"G1","items":[{"pvc_id":"pvc1","user_id":"U2"}]}`, "U1")
	code, data, _ = batchDeleteStoragePermissions(app, batchDeleteReq, platform.RouteSpec{})
	assertStorageStatus(t, code, data, http.StatusOK)
	if result := data.(map[string]any); result["succeeded"] != 1 {
		t.Fatalf("batch delete permissions = %#v", result)
	}

	batchInitReq := storageRequest(http.MethodPost, "/api/v1/admin/user-storage/batch-init", `{"usernames":["alice","bob"]}`, "ADMIN")
	code, data, _ = batchInitUserStorage(app, batchInitReq, platform.RouteSpec{})
	assertStorageStatus(t, code, data, http.StatusOK)
	if result := data.(map[string]any); result["succeeded"] != 2 {
		t.Fatalf("batch init user storage = %#v", result)
	}
	statusReq := storageRequest(http.MethodGet, "/api/v1/admin/user-storage/alice/status", "", "ADMIN")
	statusReq.SetPathValue("username", "alice")
	code, data, _ = getUserStorageStatus(app, statusReq, platform.RouteSpec{})
	assertStorageStatus(t, code, data, http.StatusOK)
	if data.(map[string]any)["status"] != "initialized" {
		t.Fatalf("user storage status = %#v", data)
	}
	expandReq := storageRequest(http.MethodPut, "/api/v1/admin/user-storage/alice/expand", `{"size":"50Gi"}`, "ADMIN")
	expandReq.SetPathValue("username", "alice")
	code, data, _ = expandUserStorage(app, expandReq, platform.RouteSpec{})
	assertStorageStatus(t, code, data, http.StatusOK)
	if data.(map[string]any)["status"] != "expanded" {
		t.Fatalf("expanded user storage = %#v", data)
	}
}

func TestStoragePermissionManagementFollowsGroupRBAC(t *testing.T) {
	t.Run("plain member cannot create permission", func(t *testing.T) {
		app := newStorageTestApp(t)

		req := storageRequest(http.MethodPost, "/api/v1/storage/permissions", `{"group_id":"G1","pvc_id":"pvc1","user_id":"U3","permission":"read_write"}`, "U2")
		code, data, _ := createStoragePermission(app, req, platform.RouteSpec{})

		assertStorageStatus(t, code, data, http.StatusForbidden)
		assertStorageRecordMissing(t, app, storagePermissionsResource, "G1:pvc1:U3")
	})

	t.Run("plain member cannot batch set permissions", func(t *testing.T) {
		app := newStorageTestApp(t)

		req := storageRequest(http.MethodPost, "/api/v1/storage/permissions/batch", `{"group_id":"G1","items":[{"pvc_id":"pvc1","user_id":"U3","permission":"read_write"}]}`, "U2")
		code, data, _ := batchSetStoragePermissions(app, req, platform.RouteSpec{})

		assertStorageStatus(t, code, data, http.StatusForbidden)
		assertStorageRecordMissing(t, app, storagePermissionsResource, "G1:pvc1:U3")
	})

	t.Run("plain member cannot batch delete permissions", func(t *testing.T) {
		app := newStorageTestApp(t)
		createStorageRecords(t, app, storagePermissionsResource, []map[string]any{
			{"id": "G1:pvc1:U3", "group_id": "G1", "pvc_id": "pvc1", "user_id": "U3", "permission": "read_only"},
		})

		req := storageRequest(http.MethodDelete, "/api/v1/storage/permissions/batch", `{"group_id":"G1","items":[{"pvc_id":"pvc1","user_id":"U3"}]}`, "U2")
		code, data, _ := batchDeleteStoragePermissions(app, req, platform.RouteSpec{})

		assertStorageStatus(t, code, data, http.StatusForbidden)
		row, ok := app.Store.Get(context.Background(), storagePermissionsResource, "G1:pvc1:U3")
		if !ok {
			t.Fatal("seeded group storage permission was deleted")
		}
		if row.Data["permission"] != "read_only" {
			t.Fatalf("seeded group storage permission = %#v, want read_only", row)
		}
	})
}

func TestStorageProjectBindingsTransfersAndValidation(t *testing.T) {
	app := newStorageTestApp(t)

	badReq := storageProjectRequest(http.MethodPost, "/api/v1/projects/P1/storage/bindings", `{`, "U3", "P1")
	code, data, _ := createProjectBinding(app, badReq, platform.RouteSpec{})
	assertStorageStatus(t, code, data, http.StatusBadRequest)
	if got := len(app.Store.List(context.Background(), projectBindingsResource)); got != 0 {
		t.Fatalf("bindings after malformed JSON = %d, want none", got)
	}

	bindReq := storageProjectRequest(http.MethodPost, "/api/v1/projects/P1/storage/bindings", `{"group_id":"G1","pvc_id":"pvc1"}`, "U3", "P1")
	code, data, _ = createProjectBinding(app, bindReq, platform.RouteSpec{})
	assertStorageStatus(t, code, data, http.StatusCreated)
	if data.(map[string]any)["pvc_id"] != "pvc1" {
		t.Fatalf("binding = %#v", data)
	}
	listReq := storageProjectRequest(http.MethodGet, "/api/v1/projects/P1/storage/bindings", "", "U2", "P1")
	code, data, _ = listProjectBindings(app, listReq, platform.RouteSpec{})
	assertStorageStatus(t, code, data, http.StatusOK)
	if rows := data.([]map[string]any); len(rows) != 1 {
		t.Fatalf("project bindings = %#v, want one", rows)
	}

	permReq := storageProjectRequest(http.MethodPut, "/api/v1/projects/P1/storage/bindings/pvc1/permissions", `{"user_id":"U2","permission":"read_only"}`, "U3", "P1")
	permReq.SetPathValue("pvcId", "pvc1")
	code, data, _ = setProjectBindingPermission(app, permReq, platform.RouteSpec{})
	assertStorageStatus(t, code, data, http.StatusOK)
	if data.(map[string]any)["permission"] != "read_only" {
		t.Fatalf("project permission = %#v", data)
	}
	listPermReq := storageProjectRequest(http.MethodGet, "/api/v1/projects/P1/storage/bindings/pvc1/permissions", "", "U2", "P1")
	listPermReq.SetPathValue("pvcId", "pvc1")
	code, data, _ = listProjectBindingPermissions(app, listPermReq, platform.RouteSpec{})
	assertStorageStatus(t, code, data, http.StatusOK)
	if rows := data.([]map[string]any); len(rows) != 1 {
		t.Fatalf("project permissions = %#v, want one", rows)
	}

	batchReq := storageProjectRequest(http.MethodPut, "/api/v1/projects/P1/storage/bindings/pvc1/permissions/batch", `{"items":[{"user_id":"U2","permission":"read_write"}]}`, "U3", "P1")
	batchReq.SetPathValue("pvcId", "pvc1")
	code, data, _ = batchSetProjectBindingPermissions(app, batchReq, platform.RouteSpec{})
	assertStorageStatus(t, code, data, http.StatusOK)
	if result := data.(map[string]any); result["succeeded"] != 1 {
		t.Fatalf("batch project permissions = %#v", result)
	}

	transferReq := storageProjectRequest(http.MethodPost, "/api/v1/projects/P1/storage/transfers/fast-stage", `{"name":"copy1","target_namespace":"project-P1"}`, "U3", "P1")
	code, data, _ = startFastTransfer(app, transferReq, platform.RouteSpec{})
	assertStorageStatus(t, code, data, http.StatusAccepted)
	getTransferReq := storageProjectRequest(http.MethodGet, "/api/v1/projects/P1/storage/transfers/project-P1/copy1", "", "U2", "P1")
	getTransferReq.SetPathValue("targetNamespace", "project-P1")
	getTransferReq.SetPathValue("name", "copy1")
	code, data, _ = getFastTransfer(app, getTransferReq, platform.RouteSpec{})
	assertStorageStatus(t, code, data, http.StatusOK)
	cancelReq := storageProjectRequest(http.MethodDelete, "/api/v1/projects/P1/storage/transfers/project-P1/copy1", "", "U3", "P1")
	cancelReq.SetPathValue("targetNamespace", "project-P1")
	cancelReq.SetPathValue("name", "copy1")
	code, data, _ = cancelFastTransfer(app, cancelReq, platform.RouteSpec{})
	assertStorageStatus(t, code, data, http.StatusOK)
	if data.(map[string]any)["status"] != "cancelled" {
		t.Fatalf("cancelled transfer = %#v", data)
	}

	deleteBindingReq := storageProjectRequest(http.MethodDelete, "/api/v1/projects/P1/storage/bindings/pvc1", "", "U3", "P1")
	deleteBindingReq.SetPathValue("requestId", "pvc1")
	code, data, _ = deleteProjectBinding(app, deleteBindingReq, platform.RouteSpec{})
	assertStorageStatus(t, code, data, http.StatusOK)
	if got := app.Store.List(context.Background(), projectPermissionsResource); len(got) != 0 {
		t.Fatalf("project permissions after binding delete = %#v, want cleanup", got)
	}
}

func TestProjectStoragePermissionManagementFollowsProjectRBAC(t *testing.T) {
	t.Run("project reader cannot set permission", func(t *testing.T) {
		app := newStorageTestApp(t)
		createProjectStorageFixtures(t, app)

		req := storageProjectRequest(http.MethodPut, "/api/v1/projects/P1/storage/bindings/pvc1/permissions", `{"user_id":"U1","permission":"read_write"}`, "U2", "P1")
		req.SetPathValue("pvcId", "pvc1")
		code, data, _ := setProjectBindingPermission(app, req, platform.RouteSpec{})

		assertStorageStatus(t, code, data, http.StatusForbidden)
		assertStorageRecordMissing(t, app, projectPermissionsResource, "P1:pvc1:U1")
	})

	t.Run("project reader cannot batch set permissions", func(t *testing.T) {
		app := newStorageTestApp(t)
		createProjectStorageFixtures(t, app)

		req := storageProjectRequest(http.MethodPut, "/api/v1/projects/P1/storage/bindings/pvc1/permissions/batch", `{"items":[{"user_id":"U1","permission":"read_write"}]}`, "U2", "P1")
		req.SetPathValue("pvcId", "pvc1")
		code, data, _ := batchSetProjectBindingPermissions(app, req, platform.RouteSpec{})

		assertStorageStatus(t, code, data, http.StatusForbidden)
		assertStorageRecordMissing(t, app, projectPermissionsResource, "P1:pvc1:U1")
	})

	t.Run("project reader cannot batch delete permissions", func(t *testing.T) {
		app := newStorageTestApp(t)
		createProjectStorageFixtures(t, app)

		req := storageProjectRequest(http.MethodDelete, "/api/v1/projects/P1/storage/bindings/pvc1/permissions/batch", `{"items":[{"user_id":"U4"}]}`, "U2", "P1")
		req.SetPathValue("pvcId", "pvc1")
		code, data, _ := batchDeleteProjectBindingPermissions(app, req, platform.RouteSpec{})

		assertStorageStatus(t, code, data, http.StatusForbidden)
		row, ok := app.Store.Get(context.Background(), projectPermissionsResource, "P1:pvc1:U4")
		if !ok {
			t.Fatal("seeded project storage permission was deleted")
		}
		if row.Data["permission"] != "read_write" {
			t.Fatalf("seeded project storage permission = %#v, want read_write", row)
		}
	})
}

func TestStorageDeletionBatchAndUserLifecycle(t *testing.T) {
	app := newStorageTestApp(t)

	stopReq := storagePathRequest(http.MethodDelete, "/api/v1/storage/G1/storage/pvc1/stop", "", "U2", "G1")
	stopReq.SetPathValue("pvcId", "pvc1")
	code, data, _ := stopGroupStorage(app, stopReq, platform.RouteSpec{})
	assertStorageStatus(t, code, data, http.StatusOK)
	assertStorageMapValue(t, data, "status", "stopped")

	batchSetReq := storageRequest(http.MethodPost, "/api/v1/storage/permissions/batch", `{"group_id":"G1","items":[{"pvc_id":"pvc1","user_id":"U2","permission":"read_only"},{"pvc_id":"pvc1"}]}`, "U1")
	code, data, _ = batchSetStoragePermissions(app, batchSetReq, platform.RouteSpec{})
	assertStorageStatus(t, code, data, http.StatusOK)
	assertStorageMapValue(t, data, "succeeded", 1)
	assertStorageMapValue(t, data, "failed", 1)

	deletePermReq := storageRequest(http.MethodDelete, "/api/v1/storage/permissions/group/G1/pvc/pvc1/user/U2", "", "U1")
	deletePermReq.SetPathValue("group_id", "G1")
	deletePermReq.SetPathValue("pvc_id", "pvc1")
	deletePermReq.SetPathValue("user_id", "U2")
	code, data, _ = deleteStoragePermission(app, deletePermReq, platform.RouteSpec{})
	assertStorageStatus(t, code, data, http.StatusOK)

	createProjectStorageFixtures(t, app)
	deleteProjectPermReq := storageProjectRequest(http.MethodDelete, "/api/v1/projects/P1/storage/bindings/pvc1/permissions/U2", "", "U3", "P1")
	deleteProjectPermReq.SetPathValue("pvcId", "pvc1")
	deleteProjectPermReq.SetPathValue("userId", "U2")
	code, data, _ = deleteProjectBindingPermission(app, deleteProjectPermReq, platform.RouteSpec{})
	assertStorageStatus(t, code, data, http.StatusOK)
	assertStorageRecordMissing(t, app, projectPermissionsResource, "P1:pvc1:U2")

	batchDeleteReq := storageProjectRequest(http.MethodDelete, "/api/v1/projects/P1/storage/bindings/pvc1/permissions/batch", `{"items":[{"user_id":"U4"}]}`, "U3", "P1")
	batchDeleteReq.SetPathValue("pvcId", "pvc1")
	code, data, _ = batchDeleteProjectBindingPermissions(app, batchDeleteReq, platform.RouteSpec{})
	assertStorageStatus(t, code, data, http.StatusOK)
	assertStorageMapValue(t, data, "succeeded", 1)

	initReq := storageRequest(http.MethodPost, "/api/v1/admin/user-storage/dana/init", `{"size":"25Gi"}`, "ADMIN")
	initReq.SetPathValue("username", "dana")
	code, data, _ = initUserStorage(app, initReq, platform.RouteSpec{})
	assertStorageStatus(t, code, data, http.StatusOK)
	assertStorageMapValue(t, data, "status", "initialized")

	statusReq := storageRequest(http.MethodPost, "/api/v1/admin/user-storage/batch-status", `{"usernames":["alice","dana","missing"]}`, "ADMIN")
	code, data, _ = batchUserStorageStatus(app, statusReq, platform.RouteSpec{})
	assertStorageStatus(t, code, data, http.StatusOK)
	assertStorageRows(t, data, 3)

	deleteUserReq := storageRequest(http.MethodDelete, "/api/v1/admin/user-storage/dana", "", "ADMIN")
	deleteUserReq.SetPathValue("username", "dana")
	code, data, _ = deleteUserStorage(app, deleteUserReq, platform.RouteSpec{})
	assertStorageStatus(t, code, data, http.StatusOK)
	assertStorageMapValue(t, data, "status", "deleted")

	createStorageRecords(t, app, storagePermissionsResource, []map[string]any{{"id": "G1:pvc1:U3", "group_id": "G1", "pvc_id": "pvc1", "user_id": "U3", "permission": "read_only"}})
	createStorageRecords(t, app, storagePoliciesResource, []map[string]any{{"id": "G1:pvc1", "group_id": "G1", "pvc_id": "pvc1", "default_permission": "read_only"}})
	deleteStorageReq := storagePathRequest(http.MethodDelete, "/api/v1/storage/G1/storage/pvc1", "", "ADMIN", "G1")
	deleteStorageReq.SetPathValue("pvcId", "pvc1")
	code, data, _ = deleteGroupStorage(app, deleteStorageReq, platform.RouteSpec{})
	assertStorageStatus(t, code, data, http.StatusOK)
	assertStorageRecordMissing(t, app, groupStorageResource, "G1:pvc1")
	assertStorageRecordMissing(t, app, storagePermissionsResource, "G1:pvc1:U3")
	assertStorageRecordMissing(t, app, storagePoliciesResource, "G1:pvc1")
}

func newStorageTestApp(t *testing.T) *platform.App {
	t.Helper()
	app := platform.NewApp(platform.Config{ServiceName: "all", HTTPAddr: ":0"})
	Register(app)
	createStorageRecords(t, app, identityUsersResource, []map[string]any{
		{"id": "ADMIN", "username": "admin", "capabilities": map[string]any{"adminPanel": true}},
		{"id": "U1", "username": "alice"},
		{"id": "U2", "username": "bob"},
		{"id": "U3", "username": "carol"},
	})
	createStorageRecords(t, app, orgProjectsResource, []map[string]any{
		{"id": "P1", "project_name": "vision", "owner_id": "G1"},
	})
	createStorageRecords(t, app, orgProjectMembersResource, []map[string]any{
		{"id": "P1:U3", "project_id": "P1", "user_id": "U3", "role": "manager"},
	})
	createStorageRecords(t, app, orgUserGroupsResource, []map[string]any{
		{"id": "U1:G1", "user_id": "U1", "group_id": "G1", "role": "admin"},
		{"id": "U2:G1", "user_id": "U2", "group_id": "G1", "role": "user"},
	})
	createStorageRecords(t, app, groupStorageResource, []map[string]any{
		{"id": "G1:pvc1", "group_id": "G1", "pvc_id": "pvc1", "name": "datasets", "size": "10Gi", "status": "created"},
	})
	return app
}

func createStorageRecords(t *testing.T, app *platform.App, resource string, rows []map[string]any) {
	t.Helper()
	for _, row := range rows {
		if _, err := app.Store.Create(context.Background(), resource, row); err != nil {
			t.Fatal(err)
		}
	}
}

func storageRequest(method, target, body, userID string) *http.Request {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if userID != "" {
		req.Header.Set("X-User-ID", userID)
	}
	return req
}

func storagePathRequest(method, target, body, userID, id string) *http.Request {
	req := storageRequest(method, target, body, userID)
	req.SetPathValue("id", id)
	return req
}

func storageProjectRequest(method, target, body, userID, projectID string) *http.Request {
	return storagePathRequest(method, target, body, userID, projectID)
}

func assertStorageStatus(t *testing.T, code int, data any, want int) {
	t.Helper()
	if code != want {
		t.Fatalf("status=%d data=%#v, want %d", code, data, want)
	}
}

func createProjectStorageFixtures(t *testing.T, app *platform.App) {
	t.Helper()
	createStorageRecords(t, app, projectBindingsResource, []map[string]any{{"id": "P1:pvc1", "project_id": "P1", "group_id": "G1", "pvc_id": "pvc1"}})
	createStorageRecords(t, app, projectPermissionsResource, []map[string]any{
		{"id": "P1:pvc1:U2", "project_id": "P1", "pvc_id": "pvc1", "user_id": "U2", "permission": "read_only"},
		{"id": "P1:pvc1:U4", "project_id": "P1", "pvc_id": "pvc1", "user_id": "U4", "permission": "read_write"},
	})
}

func assertStorageRows(t *testing.T, data any, want int) {
	t.Helper()
	rows := data.([]map[string]any)
	if len(rows) != want {
		t.Fatalf("rows = %#v, want %d", rows, want)
	}
}

func assertStorageMapValue(t *testing.T, data any, key string, want any) {
	t.Helper()
	row := data.(map[string]any)
	if row[key] != want {
		t.Fatalf("%s = %#v, want %#v in %#v", key, row[key], want, row)
	}
}

func assertStorageRecordMissing(t *testing.T, app *platform.App, resource, id string) {
	t.Helper()
	if _, ok := app.Store.Get(context.Background(), resource, id); ok {
		t.Fatalf("%s/%s unexpectedly exists", resource, id)
	}
}
