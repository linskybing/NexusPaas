package schedulerquota

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/orgproject"
)

const testServiceKey = "svc-key"

// registerOrgProjectBindingRoutes installs the catalog RouteSpecs for the
// org-project internal binding contract so app.ServeHTTP can dispatch them. It
// mirrors how preemption_test registers the workload internal routes without
// importing the services catalog package (which would create an import cycle).
func registerOrgProjectBindingRoutes(app *platform.App) {
	app.RegisterService(platform.ServiceSpec{Name: orgProjectServiceName, Routes: []platform.RouteSpec{
		{Method: http.MethodPut, Pattern: bindProjectPlanPathTemplate, Resource: "projects", Action: "bind_plan", AuthRequired: false},
		{Method: http.MethodDelete, Pattern: clearPlanBindingsPathTemplate, Resource: "projects", Action: "clear_plan_bindings", AuthRequired: false},
	}})
}

// newCoHostedBindingApp builds an app hosting both scheduler-quota and
// org-project with a service key, so the local binding client routes through the
// org-project internal contract via app.ServeHTTP.
func newCoHostedBindingApp(t *testing.T) *platform.App {
	t.Helper()
	app := platform.NewApp(platform.Config{ServiceName: "all", HTTPAddr: ":0", ServiceAPIKey: testServiceKey})
	registerOrgProjectBindingRoutes(app)
	Register(app)
	orgproject.Register(app)
	return app
}

func TestOrgProjectBindingClientLocal(t *testing.T) {
	app := newCoHostedBindingApp(t)
	ctx := context.Background()
	if _, err := app.Store.Create(ctx, projectsResource, map[string]any{"id": "proj-1"}); err != nil {
		t.Fatal(err)
	}
	client, err := newOrgProjectBindingClient(app)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	if _, ok := client.(localOrgProjectBindingClient); !ok {
		t.Fatalf("co-hosted client = %T, want local", client)
	}

	if err := client.BindPlan(ctx, "proj-1", "p1"); err != nil {
		t.Fatalf("BindPlan: %v", err)
	}
	record, _ := app.Store.Get(ctx, projectsResource, "proj-1")
	if record.Data["plan_id"] != "p1" {
		t.Fatalf("project after BindPlan = %#v, want plan_id p1", record.Data)
	}

	if err := client.BindPlan(ctx, "ghost", "p1"); !errors.Is(err, errProjectNotFound) {
		t.Fatalf("BindPlan missing project err = %v, want errProjectNotFound", err)
	}

	if err := client.ClearPlanBindings(ctx, "p1"); err != nil {
		t.Fatalf("ClearPlanBindings: %v", err)
	}
	cleared, _ := app.Store.Get(ctx, projectsResource, "proj-1")
	if cleared.Data["plan_id"] != "" {
		t.Fatalf("project after clear = %#v, want empty plan_id", cleared.Data)
	}
}

func TestOrgProjectBindingClientRemote(t *testing.T) {
	owner := platform.NewApp(platform.Config{ServiceName: orgProjectServiceName, HTTPAddr: ":0", ServiceAPIKey: testServiceKey})
	registerOrgProjectBindingRoutes(owner)
	orgproject.Register(owner)
	if _, err := owner.Store.Create(context.Background(), projectsResource, map[string]any{"id": "proj-1"}); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(owner)
	defer server.Close()

	consumer := platform.NewApp(platform.Config{
		ServiceName:   serviceName,
		HTTPAddr:      ":0",
		ServiceAPIKey: testServiceKey,
		ServiceURLs:   map[string]string{orgProjectServiceName: server.URL},
	})
	client, err := newOrgProjectBindingClient(consumer)
	if err != nil {
		t.Fatalf("new remote client: %v", err)
	}
	if _, ok := client.(httpOrgProjectBindingClient); !ok {
		t.Fatalf("isolated client = %T, want http", client)
	}

	if err := client.BindPlan(context.Background(), "proj-1", "p9"); err != nil {
		t.Fatalf("remote BindPlan: %v", err)
	}
	record, _ := owner.Store.Get(context.Background(), projectsResource, "proj-1")
	if record.Data["plan_id"] != "p9" {
		t.Fatalf("remote bound project = %#v, want plan_id p9", record.Data)
	}

	if err := client.BindPlan(context.Background(), "ghost", "p9"); !errors.Is(err, errProjectNotFound) {
		t.Fatalf("remote BindPlan missing project err = %v, want errProjectNotFound", err)
	}
}

func TestOrgProjectBindingClientIsolatedRequiresConfig(t *testing.T) {
	// Isolated scheduler-quota with no org-project URL must fail closed, never
	// silently fall back to a local write.
	app := platform.NewApp(platform.Config{ServiceName: serviceName, HTTPAddr: ":0", ServiceAPIKey: testServiceKey})
	if _, err := newOrgProjectBindingClient(app); err == nil {
		t.Fatal("expected error when org-project URL is unconfigured in isolated mode")
	}
}
