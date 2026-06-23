package imageregistry

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

func TestDockerfileBuildExternalAPIFixtureMatchesSpec(t *testing.T) {
	fixture := readDockerfileBuildExternalAPIFixture(t)
	spec := Spec()
	route, ok := findRoute(spec.Routes, fixture.Method, fixture.Path)
	if !ok {
		t.Fatalf("route %s %s not found in Spec()", fixture.Method, fixture.Path)
	}

	assertDockerfileBuildFixtureMetadata(t, fixture, spec.Name, route)
	assertDockerfileBuildRouteMetadata(t, route, fixture)
}

func assertDockerfileBuildFixtureMetadata(t *testing.T, fixture dockerfileBuildExternalAPIFixture, serviceName string, route platform.RouteSpec) {
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
	if !reflect.DeepEqual(fixture.RequiredRequestFields, []string{"project_id", "image_reference"}) {
		t.Fatalf("required_request_fields = %v, want [project_id image_reference]", fixture.RequiredRequestFields)
	}
	if !reflect.DeepEqual(fixture.SuccessStatuses, []int{http.StatusAccepted}) {
		t.Fatalf("success_statuses = %v, want [202]", fixture.SuccessStatuses)
	}
	if !reflect.DeepEqual(fixture.EmitsEvents, []string{"ImageBuildStarted"}) {
		t.Fatalf("emits_events = %v, want [ImageBuildStarted]", fixture.EmitsEvents)
	}
}

func assertDockerfileBuildRouteMetadata(t *testing.T, route platform.RouteSpec, fixture dockerfileBuildExternalAPIFixture) {
	t.Helper()
	if got, want := route.Resource, "image_builds"; got != want {
		t.Fatalf("route resource = %q, want %q", got, want)
	}
	if got, want := route.Action, "command"; got != want {
		t.Fatalf("route action = %q, want %q", got, want)
	}
	if route.Method != http.MethodPost || route.Pattern != "/api/v1/images/build/dockerfile" {
		t.Fatalf("route = %s %s, want POST /api/v1/images/build/dockerfile", route.Method, route.Pattern)
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
	if route.ExternalAdapter != "harbor" {
		t.Fatalf("route ExternalAdapter = %q, want harbor", route.ExternalAdapter)
	}
	if fixture.Method != route.Method || fixture.Path != route.Pattern {
		t.Fatalf("fixture route = %s %s, want %s %s", fixture.Method, fixture.Path, route.Method, route.Pattern)
	}
}

type dockerfileBuildExternalAPIFixture struct {
	OwnerService          string   `json:"owner_service"`
	Resource              string   `json:"resource"`
	Action                string   `json:"action"`
	Method                string   `json:"method"`
	Path                  string   `json:"path"`
	Auth                  string   `json:"auth"`
	AuthRequired          bool     `json:"auth_required"`
	ServiceKeyRequired    bool     `json:"service_key_required"`
	PathParameters        []string `json:"path_parameters"`
	RequiredRequestFields []string `json:"required_request_fields"`
	SuccessStatuses       []int    `json:"success_statuses"`
	EmitsEvents           []string `json:"emits_events"`
}

func findRoute(routes []platform.RouteSpec, method, pattern string) (platform.RouteSpec, bool) {
	for _, route := range routes {
		if route.Method == method && route.Pattern == pattern {
			return route, true
		}
	}
	return platform.RouteSpec{}, false
}

func readDockerfileBuildExternalAPIFixture(t *testing.T) dockerfileBuildExternalAPIFixture {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "contracts", "fixtures", "api", "v1", "image-registry-dockerfile-build.json"))
	if err != nil {
		t.Fatalf("read image-registry Dockerfile build external API fixture: %v", err)
	}
	var fixture dockerfileBuildExternalAPIFixture
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatalf("unmarshal image-registry Dockerfile build external API fixture: %v", err)
	}
	return fixture
}
