package storage

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

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
			Method:       http.MethodPost,
			Pattern:      pathInternalStorageMountPlan,
			Resource:     "mount_plans",
			Action:       "resolve",
			PolicyBypass: true,
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
