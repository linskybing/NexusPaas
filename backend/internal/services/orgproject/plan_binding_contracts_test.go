package orgproject

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

const testServiceKey = "svc-key"

func newBindingTestApp(t *testing.T, key string) *platform.App {
	t.Helper()
	app := platform.NewApp(platform.Config{ServiceName: serviceName, HTTPAddr: ":0", ServiceAPIKey: key})
	Register(app)
	return app
}

func serviceBindRequest(method, target, body, key string) *http.Request {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if key != "" {
		req.Header.Set("X-Service-Key", key)
	}
	return req
}

func seedProject(t *testing.T, app *platform.App, data map[string]any) {
	t.Helper()
	if _, err := app.Store.Create(context.Background(), projectsResource, data); err != nil {
		t.Fatal(err)
	}
}

func projectPlanID(t *testing.T, app *platform.App, id string) string {
	t.Helper()
	record, found := app.Store.Get(context.Background(), projectsResource, id)
	if !found {
		t.Fatalf("project %s not found", id)
	}
	return record.Data["plan_id"].(string)
}

func TestBindProjectPlanServiceAuth(t *testing.T) {
	t.Run("closed when no service key configured", func(t *testing.T) {
		app := newBindingTestApp(t, "")
		seedProject(t, app, map[string]any{"id": "proj-1"})
		req := serviceBindRequest(http.MethodPut, "/internal/org-project/projects/proj-1/plan", `{"plan_id":"p1"}`, "")
		req.SetPathValue("project_id", "proj-1")
		code, _, _ := bindProjectPlan(app, req, platform.RouteSpec{})
		if code != http.StatusNotFound {
			t.Fatalf("no-key bind status = %d, want 404", code)
		}
	})

	t.Run("unauthorized on bad key", func(t *testing.T) {
		app := newBindingTestApp(t, testServiceKey)
		seedProject(t, app, map[string]any{"id": "proj-1"})
		req := serviceBindRequest(http.MethodPut, "/internal/org-project/projects/proj-1/plan", `{"plan_id":"p1"}`, "wrong")
		req.SetPathValue("project_id", "proj-1")
		code, _, _ := bindProjectPlan(app, req, platform.RouteSpec{})
		if code != http.StatusUnauthorized {
			t.Fatalf("bad-key bind status = %d, want 401", code)
		}
	})
}

func TestBindProjectPlanSetsBinding(t *testing.T) {
	app := newBindingTestApp(t, testServiceKey)
	seedProject(t, app, map[string]any{"id": "proj-1", "name": "science"})

	req := serviceBindRequest(http.MethodPut, "/internal/org-project/projects/proj-1/plan", `{"plan_id":"p1"}`, testServiceKey)
	req.SetPathValue("project_id", "proj-1")
	code, data, _ := bindProjectPlan(app, req, platform.RouteSpec{})
	if code != http.StatusOK {
		t.Fatalf("bind status = %d data=%#v, want 200", code, data)
	}
	record := data.(contracts.Record[map[string]any])
	if record.Data["plan_id"] != "p1" || record.Data["resource_plan_id"] != "p1" {
		t.Fatalf("bound project = %#v, want plan_id/resource_plan_id p1", record.Data)
	}

	// Idempotent re-bind of the same plan succeeds.
	req2 := serviceBindRequest(http.MethodPut, "/internal/org-project/projects/proj-1/plan", `{"plan_id":"p1"}`, testServiceKey)
	req2.SetPathValue("project_id", "proj-1")
	if code, _, _ := bindProjectPlan(app, req2, platform.RouteSpec{}); code != http.StatusOK {
		t.Fatalf("idempotent re-bind status = %d, want 200", code)
	}
	if got := projectPlanID(t, app, "proj-1"); got != "p1" {
		t.Fatalf("project plan_id after re-bind = %q, want p1", got)
	}
}

func TestBindProjectPlanValidation(t *testing.T) {
	app := newBindingTestApp(t, testServiceKey)

	missingPlan := serviceBindRequest(http.MethodPut, "/internal/org-project/projects/proj-1/plan", `{}`, testServiceKey)
	missingPlan.SetPathValue("project_id", "proj-1")
	if code, _, _ := bindProjectPlan(app, missingPlan, platform.RouteSpec{}); code != http.StatusBadRequest {
		t.Fatalf("missing plan_id status = %d, want 400", code)
	}

	missingProject := serviceBindRequest(http.MethodPut, "/internal/org-project/projects/ghost/plan", `{"plan_id":"p1"}`, testServiceKey)
	missingProject.SetPathValue("project_id", "ghost")
	if code, _, _ := bindProjectPlan(app, missingProject, platform.RouteSpec{}); code != http.StatusNotFound {
		t.Fatalf("missing project status = %d, want 404", code)
	}
}

func TestClearProjectsPlanClearsMatching(t *testing.T) {
	app := newBindingTestApp(t, testServiceKey)
	seedProject(t, app, map[string]any{"id": "proj-1", "plan_id": "p1", "resource_plan_id": "p1"})
	seedProject(t, app, map[string]any{"id": "proj-2", "plan_id": "p1", "resource_plan_id": "p1"})
	seedProject(t, app, map[string]any{"id": "proj-3", "plan_id": "p2", "resource_plan_id": "p2"})

	req := serviceBindRequest(http.MethodDelete, "/internal/org-project/plans/p1/project-bindings", "", testServiceKey)
	req.SetPathValue("plan_id", "p1")
	code, data, _ := clearProjectsPlan(app, req, platform.RouteSpec{})
	if code != http.StatusOK {
		t.Fatalf("clear status = %d, want 200", code)
	}
	if cleared := data.(map[string]any)["cleared"]; cleared != 2 {
		t.Fatalf("cleared = %v, want 2", cleared)
	}
	if got := projectPlanID(t, app, "proj-1"); got != "" {
		t.Fatalf("proj-1 plan_id after clear = %q, want empty", got)
	}
	if got := projectPlanID(t, app, "proj-3"); got != "p2" {
		t.Fatalf("proj-3 plan_id changed = %q, want p2 untouched", got)
	}

	// Idempotent: clearing an unbound plan reports zero.
	req2 := serviceBindRequest(http.MethodDelete, "/internal/org-project/plans/p1/project-bindings", "", testServiceKey)
	req2.SetPathValue("plan_id", "p1")
	_, data2, _ := clearProjectsPlan(app, req2, platform.RouteSpec{})
	if cleared := data2.(map[string]any)["cleared"]; cleared != 0 {
		t.Fatalf("second clear = %v, want 0", cleared)
	}
}
