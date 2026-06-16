package platform

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestServiceRegistryViewOmitsInternalFields(t *testing.T) {
	app := NewApp(Config{ServiceName: "all"})
	app.Services = map[string]ServiceSpec{
		"b-service": {Name: "b-service", Category: "core", Phase: "ga", Description: "internal description", Events: []string{"X"}, Tables: []string{"t"}, Routes: []RouteSpec{
			{Method: "GET", Pattern: "/api/v1/b/{id}", Resource: "b-service:items", Action: "crud", OperationID: "b_get", ExternalAdapter: "harbor", Admin: true},
		}},
		"a-service": {Name: "a-service", Category: "core", Phase: "ga", Routes: []RouteSpec{
			{Method: "POST", Pattern: "/api/v1/a", Resource: "a-service:items", Action: "crud"},
		}},
	}

	view := app.ServiceRegistryView()
	if len(view) != 2 || view[0].Name != "a-service" || view[1].Name != "b-service" {
		t.Fatalf("registry view = %#v, want sorted a-service,b-service", view)
	}

	// Serialize and confirm none of the internal fields leak through.
	blob, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	for _, leak := range []string{"resource", "action", "operation_id", "external_adapter", "harbor", "description", "internal description", "events", "tables", "auth_required", "\"admin\""} {
		if strings.Contains(string(blob), leak) {
			t.Fatalf("service-registry view leaked %q: %s", leak, blob)
		}
	}
	// The public surface (method + pattern) is still present.
	for _, want := range []string{"/api/v1/a", "/api/v1/b/{id}", "core"} {
		if !strings.Contains(string(blob), want) {
			t.Fatalf("service-registry view missing %q: %s", want, blob)
		}
	}
}

func TestCompositeBackingCheckerDispatchesAndFallsBack(t *testing.T) {
	called := map[string]bool{}
	checker := compositeBackingChecker{
		checks: map[string]func(context.Context) error{
			envDatabaseURL: func(context.Context) error { called[envDatabaseURL] = true; return nil },
			envRedisURL:    func(context.Context) error { called[envRedisURL] = true; return errors.New("redis down") },
		},
		fallback: stubChecker{err: errors.New("tcp fallback")},
	}

	if err := checker.Check(context.Background(), BackingDependency{Name: envDatabaseURL}); err != nil {
		t.Fatalf("database check = %v, want nil", err)
	}
	if !called[envDatabaseURL] {
		t.Fatal("database protocol probe was not invoked")
	}
	if err := checker.Check(context.Background(), BackingDependency{Name: envRedisURL}); err == nil || err.Error() != "redis down" {
		t.Fatalf("redis check = %v, want redis down", err)
	}
	// Unregistered dependency falls back to the TCP checker.
	if err := checker.Check(context.Background(), BackingDependency{Name: envObjectStoreURL}); err == nil || err.Error() != "tcp fallback" {
		t.Fatalf("fallback check = %v, want tcp fallback", err)
	}
}

type stubChecker struct{ err error }

func (s stubChecker) Check(context.Context, BackingDependency) error { return s.err }
