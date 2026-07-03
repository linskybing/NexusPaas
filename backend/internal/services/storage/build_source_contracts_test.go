package storage

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

func buildSourceAccessRequestFor(projectID, userID, storagePath, serviceKey string) *http.Request {
	body := `{"user_id":"` + userID + `","storage_path":"` + storagePath + `"}`
	req := httptest.NewRequest(http.MethodPost, "/internal/storage/projects/"+projectID+"/build-source-access", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if serviceKey != "" {
		req.Header.Set("X-Service-Key", serviceKey)
	}
	req.SetPathValue("project_id", projectID)
	return req
}

func TestBuildSourceAccessContract(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: "storage-service", HTTPAddr: ":0", ServiceAPIKey: "svc-key"})
	Register(app)
	seed := func(resource string, row map[string]any) {
		t.Helper()
		if _, err := app.Store.Create(t.Context(), resource, row); err != nil {
			t.Fatalf("seed %s: %v", resource, err)
		}
	}
	seed(projectBindingsResource, map[string]any{"id": "P1:PVC1", "project_id": "P1", "group_id": "G1", "pvc_id": "PVC1"})
	seed(storagePoliciesResource, map[string]any{"id": "G1:PVC1", "group_id": "G1", "pvc_id": "PVC1", "default_permission": "read_only"})

	code, data, _ := resolveStorageBuildSourceAccessContract(app, buildSourceAccessRequestFor("P1", "U1", "images/ctx.tar", "svc-key"), platform.RouteSpec{})
	if code != http.StatusOK {
		t.Fatalf("allowed case status=%d data=%v, want 200", code, data)
	}
	resp, ok := data.(storageBuildSourceAccessResponse)
	if !ok || !resp.Allowed || resp.PVCID != "PVC1" || resp.Permission != "read_only" {
		t.Fatalf("decision = %#v, want allowed read_only on PVC1", data)
	}

	// project without bindings → deny (allowed=false, still 200)
	code, data, _ = resolveStorageBuildSourceAccessContract(app, buildSourceAccessRequestFor("P2", "U1", "images/ctx.tar", "svc-key"), platform.RouteSpec{})
	if code != http.StatusOK {
		t.Fatalf("no-binding status=%d, want 200", code)
	}
	if resp, _ := data.(storageBuildSourceAccessResponse); resp.Allowed {
		t.Fatalf("no-binding decision = %#v, want denied", data)
	}

	// missing fields → 422
	req := httptest.NewRequest(http.MethodPost, "/internal/storage/projects/P1/build-source-access", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Service-Key", "svc-key")
	req.SetPathValue("project_id", "P1")
	code, _, _ = resolveStorageBuildSourceAccessContract(app, req, platform.RouteSpec{})
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("missing fields status=%d, want 422", code)
	}

	// bad service credential → 401
	code, _, _ = resolveStorageBuildSourceAccessContract(app, buildSourceAccessRequestFor("P1", "U1", "images/ctx.tar", "wrong-key"), platform.RouteSpec{})
	if code != http.StatusUnauthorized {
		t.Fatalf("bad service key status=%d, want 401", code)
	}
}
