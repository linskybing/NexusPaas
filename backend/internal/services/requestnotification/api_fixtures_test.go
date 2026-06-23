package requestnotification

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

func TestCreateFormExternalAPIFixtureMatchesSpec(t *testing.T) {
	fixture := readCreateFormExternalAPIFixture(t)
	spec := Spec()
	route, ok := findRoute(spec.Routes, fixture.Method, fixture.Path)
	if !ok {
		t.Fatalf("route %s %s not found in Spec()", fixture.Method, fixture.Path)
	}

	assertCreateFormFixtureMetadata(t, fixture, spec.Name, route)
	assertCreateFormRouteMetadata(t, route, fixture)
}

func assertCreateFormFixtureMetadata(t *testing.T, fixture createFormExternalAPIFixture, serviceName string, route platform.RouteSpec) {
	t.Helper()
	if fixture.OwnerService != serviceName {
		t.Fatalf("owner_service = %q, want %q", fixture.OwnerService, serviceName)
	}
	if got, want := fixture.Resource, serviceName+":"+route.Resource; got != want {
		t.Fatalf("resource = %q, want %q", got, want)
	}
	if fixture.Action != route.Action {
		t.Fatalf("action = %q, want %q", fixture.Action, route.Action)
	}
	if fixture.Auth != "user" || fixture.AuthRequired != route.AuthRequired || fixture.ServiceKeyRequired != route.ServiceAuthRequired {
		t.Fatalf("auth metadata = %q/%v/%v, want user/%v/%v", fixture.Auth, fixture.AuthRequired, fixture.ServiceKeyRequired, route.AuthRequired, route.ServiceAuthRequired)
	}
	if len(fixture.PathParameters) != 0 {
		t.Fatalf("fixture path_parameters = %v, want none", fixture.PathParameters)
	}
	if !reflect.DeepEqual(fixture.EmitsEvents, []string{"FormCreated"}) {
		t.Fatalf("emits_events = %v, want [FormCreated]", fixture.EmitsEvents)
	}
}

func assertCreateFormRouteMetadata(t *testing.T, route platform.RouteSpec, fixture createFormExternalAPIFixture) {
	t.Helper()
	if got, want := route.Resource, "forms"; got != want {
		t.Fatalf("route resource = %q, want %q", got, want)
	}
	if got, want := route.Action, "create"; got != want {
		t.Fatalf("route action = %q, want %q", got, want)
	}
	if route.Method != http.MethodPost || route.Pattern != "/api/v1/forms" {
		t.Fatalf("route = %s %s, want POST /api/v1/forms", route.Method, route.Pattern)
	}
	if route.IDParam != "" {
		t.Fatalf("route IDParam = %q, want none", route.IDParam)
	}
	if route.Admin {
		t.Fatal("route Admin = true, want false")
	}
	if !route.StateChanging {
		t.Fatal("route StateChanging = false, want true")
	}
	if route.ServiceAuthRequired {
		t.Fatal("route ServiceAuthRequired = true, want false")
	}
	if fixture.Method != route.Method || fixture.Path != route.Pattern {
		t.Fatalf("fixture route = %s %s, want %s %s", fixture.Method, fixture.Path, route.Method, route.Pattern)
	}
}

type createFormExternalAPIFixture struct {
	OwnerService       string   `json:"owner_service"`
	Resource           string   `json:"resource"`
	Action             string   `json:"action"`
	Method             string   `json:"method"`
	Path               string   `json:"path"`
	Auth               string   `json:"auth"`
	AuthRequired       bool     `json:"auth_required"`
	ServiceKeyRequired bool     `json:"service_key_required"`
	PathParameters     []string `json:"path_parameters"`
	EmitsEvents        []string `json:"emits_events"`
}

func findRoute(routes []platform.RouteSpec, method, pattern string) (platform.RouteSpec, bool) {
	for _, route := range routes {
		if route.Method == method && route.Pattern == pattern {
			return route, true
		}
	}
	return platform.RouteSpec{}, false
}

func readCreateFormExternalAPIFixture(t *testing.T) createFormExternalAPIFixture {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "contracts", "fixtures", "api", "v1", "request-notification-create-form.json"))
	if err != nil {
		t.Fatalf("read request-notification create-form external API fixture: %v", err)
	}
	var fixture createFormExternalAPIFixture
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatalf("unmarshal request-notification create-form external API fixture: %v", err)
	}
	return fixture
}
