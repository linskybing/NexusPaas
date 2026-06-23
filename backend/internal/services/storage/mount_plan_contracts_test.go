package storage

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

func TestResolveStorageMountPlanAllowsBoundReadyReadWriteProjectPermission(t *testing.T) {
	app := newStorageMountPlanResolverApp(t)
	seedMountPlanBinding(t, app, "project-1", "group-1", "pvc-data", "project-data-claim")
	seedMountPlanGroupSource(t, app, "group-1", "pvc-data", "running")
	seedMountPlanProjectPermission(t, app, "project-1", "pvc-data", "user-1", "read_write")

	plan, status, err := resolveStorageMountPlan(app, storageMountPlanResolverRequest(), storageMountPlanRequest{
		ProjectID: "project-1",
		UserID:    "user-1",
		Namespace: "project-one",
		Mounts: []storageMountPlanRequestMount{{
			PVCID:     "pvc-data",
			Name:      "datasets",
			MountPath: "/mnt/data",
			ReadOnly:  false,
			SubPath:   "training",
		}},
	})
	if err != nil || status != http.StatusOK {
		t.Fatalf("status=%d err=%v plan=%#v, want resolved mount plan", status, err, plan)
	}
	if len(plan.ManifestMounts) != 1 {
		t.Fatalf("manifest mounts = %#v, want exactly one", plan.ManifestMounts)
	}
	mount := plan.ManifestMounts[0]
	if mount.ClaimName != "project-data-claim" || mount.MountPath != "/mnt/data" || mount.ReadOnly || mount.SubPath != "training" {
		t.Fatalf("manifest mount = %#v, want target claim and writable mount details from request/binding", mount)
	}
	if len(plan.PVCShareOperations) != 1 {
		t.Fatalf("pvc share operations = %#v, want exactly one", plan.PVCShareOperations)
	}
	share := plan.PVCShareOperations[0]
	if share.SourceNamespace != "group-1-source-ns" || share.SourcePVC != "group-1-source-pvc-data" || share.TargetPVC != "project-data-claim" {
		t.Fatalf("share operation = %#v, want storage-owned source namespace/source PVC and binding target PVC", share)
	}
}

func TestResolveStorageMountPlanRejectsOtherProjectBinding(t *testing.T) {
	app := newStorageMountPlanResolverApp(t)
	seedMountPlanBinding(t, app, "project-2", "group-1", "pvc-data", "project-data-claim")
	seedMountPlanGroupSource(t, app, "group-1", "pvc-data", "running")
	seedMountPlanProjectPermission(t, app, "project-2", "pvc-data", "user-1", "read_write")

	_, status, err := resolveStorageMountPlan(app, storageMountPlanResolverRequest(), storageMountPlanWriteRequest())
	assertMountPlanResolverError(t, status, err, http.StatusNotFound, "storage binding not found")
}

func TestResolveStorageMountPlanRejectsOtherUserPermission(t *testing.T) {
	app := newStorageMountPlanResolverApp(t)
	seedMountPlanBinding(t, app, "project-1", "group-1", "pvc-data", "project-data-claim")
	seedMountPlanGroupSource(t, app, "group-1", "pvc-data", "running")
	seedMountPlanProjectPermission(t, app, "project-1", "pvc-data", "user-2", "read_write")

	_, status, err := resolveStorageMountPlan(app, storageMountPlanResolverRequest(), storageMountPlanWriteRequest())
	assertMountPlanResolverError(t, status, err, http.StatusForbidden, "storage permission denied")
}

func TestResolveStorageMountPlanDeniesUnboundPVC(t *testing.T) {
	app := newStorageMountPlanResolverApp(t)
	seedMountPlanGroupSource(t, app, "group-1", "pvc-data", "running")
	seedMountPlanProjectPermission(t, app, "project-1", "pvc-data", "user-1", "read_write")

	_, status, err := resolveStorageMountPlan(app, storageMountPlanResolverRequest(), storageMountPlanWriteRequest())
	assertMountPlanResolverError(t, status, err, http.StatusNotFound, "storage binding not found")
}

func TestResolveStorageMountPlanDeniesStoppedOrDeletedSource(t *testing.T) {
	for _, sourceStatus := range []string{"stopped", "deleted"} {
		t.Run(sourceStatus, func(t *testing.T) {
			app := newStorageMountPlanResolverApp(t)
			seedMountPlanBinding(t, app, "project-1", "group-1", "pvc-data", "project-data-claim")
			seedMountPlanGroupSource(t, app, "group-1", "pvc-data", sourceStatus)
			seedMountPlanProjectPermission(t, app, "project-1", "pvc-data", "user-1", "read_write")

			_, status, err := resolveStorageMountPlan(app, storageMountPlanResolverRequest(), storageMountPlanWriteRequest())
			assertMountPlanResolverError(t, status, err, http.StatusConflict, "group storage source is not dispatch-ready")
		})
	}
}

func TestResolveStorageMountPlanDeniesWritableMountWithReadOnlyPermission(t *testing.T) {
	app := newStorageMountPlanResolverApp(t)
	seedMountPlanBinding(t, app, "project-1", "group-1", "pvc-data", "project-data-claim")
	seedMountPlanGroupSource(t, app, "group-1", "pvc-data", "running")
	seedMountPlanProjectPermission(t, app, "project-1", "pvc-data", "user-1", "read_only")

	_, status, err := resolveStorageMountPlan(app, storageMountPlanResolverRequest(), storageMountPlanWriteRequest())
	assertMountPlanResolverError(t, status, err, http.StatusForbidden, "storage permission denied")

	readOnlyReq := storageMountPlanWriteRequest()
	readOnlyReq.Mounts[0].ReadOnly = true
	plan, status, err := resolveStorageMountPlan(app, storageMountPlanResolverRequest(), readOnlyReq)
	if err != nil || status != http.StatusOK || len(plan.ManifestMounts) != 1 || len(plan.PVCShareOperations) != 1 {
		t.Fatalf("status=%d err=%v plan=%#v, want read-only mount allowed by read_only permission", status, err, plan)
	}
}

func TestResolveStorageMountPlanProjectPermissionOverridesGroupReadWriteToDenyWritableMount(t *testing.T) {
	app := newStorageMountPlanResolverApp(t)
	seedMountPlanBinding(t, app, "project-1", "group-1", "pvc-data", "project-data-claim")
	seedMountPlanGroupSource(t, app, "group-1", "pvc-data", "running")
	seedMountPlanGroupPermission(t, app, "group-1", "pvc-data", "user-1", "read_write")
	seedMountPlanProjectPermission(t, app, "project-1", "pvc-data", "user-1", "read_only")

	_, status, err := resolveStorageMountPlan(app, storageMountPlanResolverRequest(), storageMountPlanWriteRequest())
	assertMountPlanResolverError(t, status, err, http.StatusForbidden, "storage permission denied")
}

func TestStorageMountPlanContractResolvesProjectBindingAndPermission(t *testing.T) {
	app := newStorageMountPlanTestApp(t)
	createProjectStorageFixtures(t, app)

	code, data, errBody := postStorageMountPlan(t, app, "service-key", `{
		"user_id":"U2",
		"namespace":"proj-p1",
		"mounts":[{"pvc_id":"pvc1","mount_path":"/mnt/data","read_only":true,"sub_path":"training","name":"datasets"}]
	}`)
	if code != http.StatusOK || errBody != nil {
		t.Fatalf("status=%d error=%#v data=%#v, want resolved plan", code, errBody, data)
	}
	if data.ProjectID != "P1" || data.UserID != "U2" || data.Namespace != "proj-p1" {
		t.Fatalf("plan identity = %#v", data)
	}
	if len(data.ManifestMounts) != 1 || data.ManifestMounts[0].ClaimName != "pvc1" ||
		data.ManifestMounts[0].MountPath != "/mnt/data" || !data.ManifestMounts[0].ReadOnly ||
		data.ManifestMounts[0].SubPath != "training" {
		t.Fatalf("manifest mounts = %#v, want read-only pvc1 mount", data.ManifestMounts)
	}
	if len(data.PVCShareOperations) != 1 || data.PVCShareOperations[0].SourceNamespace != "group-G1-storage" ||
		data.PVCShareOperations[0].SourcePVC != "pvc1" || data.PVCShareOperations[0].TargetPVC != "pvc1" {
		t.Fatalf("share operations = %#v, want group source pvc1", data.PVCShareOperations)
	}
}

func TestStorageMountPlanContractPublishesAuditableDecision(t *testing.T) {
	app := newStorageMountPlanTestApp(t)
	createProjectStorageFixtures(t, app)

	code, _, errBody := postStorageMountPlan(t, app, "service-key", `{
		"user_id":"U2",
		"namespace":"proj-p1",
		"mounts":[{
			"pvc_id":"pvc1",
			"mount_path":"/mnt/data",
			"read_only":true,
			"source_namespace":"forged-storage",
			"source_pvc":"forged-source",
			"target_pvc":"forged-target"
		}]
	}`)
	if code != http.StatusOK || errBody != nil {
		t.Fatalf("status=%d error=%#v, want resolved plan", code, errBody)
	}

	assertStorageMountPlanDecisionEvent(t, requireStorageEvent(t, app, "StorageMountPlanResolved"))
	assertStorageMountPlanAuditEvent(t, requireStorageEvent(t, app, "AuditEvent"))
}

func TestStorageMountPlanContractFailsClosed(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(*platform.App)
		body      string
		wantCode  int
		wantError string
	}{
		{
			name:      "missing binding",
			body:      `{"user_id":"U2","mounts":[{"pvc_id":"missing","mount_path":"/mnt/data","read_only":true}]}`,
			wantCode:  http.StatusNotFound,
			wantError: "storage binding not found",
		},
		{
			name: "missing source",
			mutate: func(app *platform.App) {
				app.Store.Delete(context.Background(), groupStorageResource, "G1:pvc1")
			},
			body:      `{"user_id":"U2","mounts":[{"pvc_id":"pvc1","mount_path":"/mnt/data","read_only":true}]}`,
			wantCode:  http.StatusNotFound,
			wantError: "group storage source not found",
		},
		{
			name: "stopped source",
			mutate: func(app *platform.App) {
				app.Store.Update(context.Background(), groupStorageResource, "G1:pvc1", map[string]any{"status": "stopped"})
			},
			body:      `{"user_id":"U4","mounts":[{"pvc_id":"pvc1","mount_path":"/mnt/data"}]}`,
			wantCode:  http.StatusConflict,
			wantError: "not dispatch-ready",
		},
		{
			name: "deleted source",
			mutate: func(app *platform.App) {
				app.Store.Update(context.Background(), groupStorageResource, "G1:pvc1", map[string]any{"status": "deleted"})
			},
			body:      `{"user_id":"U4","mounts":[{"pvc_id":"pvc1","mount_path":"/mnt/data"}]}`,
			wantCode:  http.StatusConflict,
			wantError: "not dispatch-ready",
		},
		{
			name:      "read only cannot request write",
			body:      `{"user_id":"U2","mounts":[{"pvc_id":"pvc1","mount_path":"/mnt/data"}]}`,
			wantCode:  http.StatusForbidden,
			wantError: "permission denied",
		},
		{
			name:      "malformed request",
			body:      `{"user_id":"U2","mounts":[{"mount_path":"/mnt/data"}]}`,
			wantCode:  http.StatusUnprocessableEntity,
			wantError: "pvc_id is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := newStorageMountPlanTestApp(t)
			createProjectStorageFixtures(t, app)
			if tt.mutate != nil {
				tt.mutate(app)
			}
			code, data, errBody := postStorageMountPlan(t, app, "service-key", tt.body)
			if code != tt.wantCode || errBody == nil || !strings.Contains(errBody.Message, tt.wantError) {
				t.Fatalf("status=%d error=%#v data=%#v, want %d containing %q", code, errBody, data, tt.wantCode, tt.wantError)
			}
		})
	}
}

func TestStorageMountPlanContractAcceptsRunningAndLegacyEmptySourceStatus(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*platform.App)
	}{
		{
			name: "running",
			mutate: func(app *platform.App) {
				app.Store.Update(context.Background(), groupStorageResource, "G1:pvc1", map[string]any{"status": "running"})
			},
		},
		{
			name: "legacy omitted",
			mutate: func(app *platform.App) {
				app.Store.Delete(context.Background(), groupStorageResource, "G1:pvc1")
				createStorageRecords(t, app, groupStorageResource, []map[string]any{
					{"id": "G1:pvc1", "group_id": "G1", "pvc_id": "pvc1", "name": "datasets", "size": "10Gi"},
				})
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := newStorageMountPlanTestApp(t)
			createProjectStorageFixtures(t, app)
			tt.mutate(app)
			code, data, errBody := postStorageMountPlan(t, app, "service-key", `{
				"user_id":"U2",
				"namespace":"proj-p1",
				"mounts":[{"pvc_id":"pvc1","mount_path":"/mnt/data","read_only":true}]
			}`)
			if code != http.StatusOK || errBody != nil || len(data.ManifestMounts) != 1 || len(data.PVCShareOperations) != 1 {
				t.Fatalf("status=%d error=%#v data=%#v, want accepted storage plan", code, errBody, data)
			}
		})
	}
}

func TestStorageMountPlanContractRequiresServiceKey(t *testing.T) {
	app := newStorageMountPlanTestApp(t)
	createProjectStorageFixtures(t, app)

	code, data, errBody := postStorageMountPlan(t, app, "wrong-key", `{"user_id":"U2","mounts":[{"pvc_id":"pvc1","read_only":true}]}`)
	if code != http.StatusUnauthorized || errBody == nil {
		t.Fatalf("status=%d error=%#v data=%#v, want unauthorized", code, errBody, data)
	}
}

func newStorageMountPlanTestApp(t *testing.T) *platform.App {
	t.Helper()
	app := newStorageTestApp(t)
	app.Config.ServiceAPIKey = "service-key"
	app.RegisterService(platform.ServiceSpec{
		Name: serviceName,
		Routes: []platform.RouteSpec{{
			Method:              http.MethodPost,
			Pattern:             pathInternalStorageMountPlan,
			Resource:            "mount_plans",
			Action:              "resolve",
			IDParam:             "project_id",
			PolicyBypass:        true,
			ServiceAuthRequired: true,
			StateChanging:       true,
		}},
	})
	return app
}

func postStorageMountPlan(t *testing.T, app *platform.App, key, body string) (int, storageMountPlanResponse, *platform.ErrorBody) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/internal/storage/projects/P1/mount-plan", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("X-Service-Key", key)
	}
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	var envelope struct {
		Data  json.RawMessage     `json:"data"`
		Error *platform.ErrorBody `json:"error"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, rec.Body.String())
	}
	var data storageMountPlanResponse
	if rec.Code < http.StatusBadRequest && len(envelope.Data) > 0 {
		if err := json.Unmarshal(envelope.Data, &data); err != nil {
			t.Fatalf("decode mount plan data: %v", err)
		}
	}
	errBody := envelope.Error
	if rec.Code >= http.StatusBadRequest && errBody == nil && len(envelope.Data) > 0 {
		var payload map[string]any
		if err := json.Unmarshal(envelope.Data, &payload); err != nil {
			t.Fatalf("decode mount plan error data: %v", err)
		}
		if message, _ := payload["message"].(string); message != "" {
			errBody = &platform.ErrorBody{Code: "error", Message: message}
		}
	}
	return rec.Code, data, errBody
}

func newStorageMountPlanResolverApp(t *testing.T) *platform.App {
	t.Helper()
	return platform.NewApp(platform.Config{ServiceName: serviceName, HTTPAddr: ":0"})
}

func storageMountPlanResolverRequest() *http.Request {
	return httptest.NewRequest(http.MethodPost, "/internal/storage/projects/project-1/mount-plan", nil)
}

func storageMountPlanWriteRequest() storageMountPlanRequest {
	return storageMountPlanRequest{
		ProjectID: "project-1",
		UserID:    "user-1",
		Namespace: "project-one",
		Mounts: []storageMountPlanRequestMount{{
			PVCID:     "pvc-data",
			MountPath: "/mnt/data",
		}},
	}
}

func seedMountPlanGroupSource(t *testing.T, app *platform.App, groupID, pvcID, status string) {
	t.Helper()
	createMountPlanResolverRecord(t, app, groupStorageResource, map[string]any{
		"id":               groupStorageID(groupID, pvcID),
		"group_id":         groupID,
		"pvc_id":           pvcID,
		"source_namespace": groupID + "-source-ns",
		"source_pvc":       groupID + "-source-" + pvcID,
		"status":           status,
	})
}

func seedMountPlanBinding(t *testing.T, app *platform.App, projectID, groupID, pvcID, targetPVC string) {
	t.Helper()
	createMountPlanResolverRecord(t, app, projectBindingsResource, map[string]any{
		"id":         projectBindingID(projectID, pvcID),
		"project_id": projectID,
		"group_id":   groupID,
		"pvc_id":     pvcID,
		"target_pvc": targetPVC,
	})
}

func seedMountPlanProjectPermission(t *testing.T, app *platform.App, projectID, pvcID, userID, permission string) {
	t.Helper()
	createMountPlanResolverRecord(t, app, projectPermissionsResource, map[string]any{
		"id":         projectPermissionID(projectID, pvcID, userID),
		"project_id": projectID,
		"pvc_id":     pvcID,
		"user_id":    userID,
		"permission": permission,
	})
}

func seedMountPlanGroupPermission(t *testing.T, app *platform.App, groupID, pvcID, userID, permission string) {
	t.Helper()
	createMountPlanResolverRecord(t, app, storagePermissionsResource, map[string]any{
		"id":         storagePermissionID(groupID, pvcID, userID),
		"group_id":   groupID,
		"pvc_id":     pvcID,
		"user_id":    userID,
		"permission": permission,
	})
}

func createMountPlanResolverRecord(t *testing.T, app *platform.App, resource string, row map[string]any) {
	t.Helper()
	if _, err := app.Store.Create(context.Background(), resource, row); err != nil {
		t.Fatal(err)
	}
}

func assertMountPlanResolverError(t *testing.T, status int, err error, wantStatus int, wantMessage string) {
	t.Helper()
	if status != wantStatus || err == nil || err.Error() != wantMessage {
		t.Fatalf("status=%d err=%v, want %d %q", status, err, wantStatus, wantMessage)
	}
}

func assertStorageMountPlanDecisionEvent(t *testing.T, event contracts.Event) {
	t.Helper()
	if event.Source != serviceName || event.SchemaVersion != 1 {
		t.Fatalf("decision event metadata = %#v, want storage source/schema v1", event)
	}
	assertEventValue(t, event.Data, "project_id", "P1")
	assertEventValue(t, event.Data, "user_id", "U2")
	assertEventValue(t, event.Data, "namespace", "proj-p1")
	assertEventValue(t, event.Data, "mount_count", 1)
	assertEventValue(t, event.Data, "manifest_mount_count", 1)
	assertEventValue(t, event.Data, "share_operation_count", 1)
	assertEventStringValues(t, event.Data, "pvc_ids", []string{"pvc1"})
	assertEventStringValues(t, event.Data, "target_pvcs", []string{"pvc1"})
	assertEventKeyAbsent(t, event.Data, "source_pvc")
	assertEventKeyAbsent(t, event.Data, "source_namespace")
}

func assertStorageMountPlanAuditEvent(t *testing.T, event contracts.Event) {
	t.Helper()
	assertEventValue(t, event.Data, "resource_id", "P1")
	assertEventValue(t, event.Data, "resource_type", "mount_plans")
	assertEventValue(t, event.Data, "success", true)
}

func assertEventValue(t *testing.T, data map[string]any, key string, want any) {
	t.Helper()
	if data[key] != want {
		t.Fatalf("event[%s] = %#v, want %#v in %#v", key, data[key], want, data)
	}
}

func assertEventKeyAbsent(t *testing.T, data map[string]any, key string) {
	t.Helper()
	if _, ok := data[key]; ok {
		t.Fatalf("event leaked %s: %#v", key, data)
	}
}

func assertEventStringValues(t *testing.T, data map[string]any, key string, want []string) {
	t.Helper()
	got := stringValues(data[key])
	if !sameStrings(got, want) {
		t.Fatalf("event[%s] = %#v, want %#v in %#v", key, data[key], want, data)
	}
}

func requireStorageEvent(t *testing.T, app *platform.App, name string) contracts.Event {
	t.Helper()
	for _, event := range app.Events.Outbox() {
		if event.Name == name {
			return event
		}
	}
	t.Fatalf("missing event %s in %#v", name, app.Events.Outbox())
	return contracts.Event{}
}

func stringValues(value any) []string {
	switch typed := value.(type) {
	case []string:
		return typed
	case []any:
		values := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok {
				values = append(values, text)
			}
		}
		return values
	default:
		return nil
	}
}

func sameStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
