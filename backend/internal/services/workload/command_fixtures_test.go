package workload

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

func TestCommandFixturesMatchWorkloadRoutes(t *testing.T) {
	cases := map[string]workloadCommandFixtureExpectation{
		"workload-preempt-job.json": {
			path:           "/internal/workload/jobs/{id}/preempt",
			method:         http.MethodPost,
			action:         "preempt",
			pathParameters: []string{"id"},
			requiredFields: []string{"preemption_id", "reason", "cleanup"},
		},
		"workload-evict-job.json": {
			path:           "/internal/workload/jobs/{id}/evict",
			method:         http.MethodPost,
			action:         "evict",
			pathParameters: []string{"id"},
			requiredFields: []string{"reason"},
		},
	}

	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := readWorkloadCommandFixture(t, name)
			assertWorkloadCommandFixture(t, fixture, want)
			route := findWorkloadCommandRoute(t, fixture.Method, fixture.Path)
			assertWorkloadInternalCommandRoute(t, route, fixture)
		})
	}
}

type workloadCommandFixtureExpectation struct {
	path           string
	method         string
	action         string
	pathParameters []string
	requiredFields []string
}

type workloadCommandFixture struct {
	OwnerService          string   `json:"owner_service"`
	ConsumerService       string   `json:"consumer_service"`
	Resource              string   `json:"resource"`
	Action                string   `json:"action"`
	Method                string   `json:"method"`
	Path                  string   `json:"path"`
	ServiceKeyRequired    bool     `json:"service_key_required"`
	PathParameters        []string `json:"path_parameters"`
	RequiredRequestFields []string `json:"required_request_fields"`
	EmitsEvents           []string `json:"emits_events"`
}

func assertWorkloadCommandFixture(t *testing.T, fixture workloadCommandFixture, want workloadCommandFixtureExpectation) {
	t.Helper()
	if fixture.OwnerService != serviceName {
		t.Fatalf("owner_service = %q, want %q", fixture.OwnerService, serviceName)
	}
	if fixture.ConsumerService != "scheduler-quota-service" {
		t.Fatalf("consumer_service = %q, want scheduler-quota-service", fixture.ConsumerService)
	}
	if fixture.Resource != jobsResource {
		t.Fatalf("resource = %q, want %q", fixture.Resource, jobsResource)
	}
	assertWorkloadCommandFixtureShape(t, fixture, want)
}

func assertWorkloadCommandFixtureShape(t *testing.T, fixture workloadCommandFixture, want workloadCommandFixtureExpectation) {
	t.Helper()
	if fixture.Method != want.method || fixture.Path != want.path || fixture.Action != want.action {
		t.Fatalf("route = %s %s %s, want %s %s %s", fixture.Method, fixture.Path, fixture.Action, want.method, want.path, want.action)
	}
	if !fixture.ServiceKeyRequired {
		t.Fatal("service_key_required = false, want true")
	}
	if !reflect.DeepEqual(fixture.PathParameters, want.pathParameters) {
		t.Fatalf("path_parameters = %v, want %v", fixture.PathParameters, want.pathParameters)
	}
	if !reflect.DeepEqual(fixture.RequiredRequestFields, want.requiredFields) {
		t.Fatalf("required_request_fields = %v, want %v", fixture.RequiredRequestFields, want.requiredFields)
	}
	if len(fixture.EmitsEvents) != 0 {
		t.Fatalf("emits_events = %v, want no emitted events for current workload command handlers", fixture.EmitsEvents)
	}
}

func readWorkloadCommandFixture(t *testing.T, name string) workloadCommandFixture {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "contracts", "fixtures", "commands", "v1", name))
	if err != nil {
		t.Fatalf("read command fixture %s: %v", name, err)
	}
	var fixture workloadCommandFixture
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatalf("unmarshal command fixture %s: %v", name, err)
	}
	return fixture
}

func findWorkloadCommandRoute(t *testing.T, method, path string) platform.RouteSpec {
	t.Helper()
	for _, route := range Spec().Routes {
		if route.Method == method && route.Pattern == path {
			return route
		}
	}
	t.Fatalf("route %s %s not found in workload spec", method, path)
	return platform.RouteSpec{}
}

func assertWorkloadInternalCommandRoute(t *testing.T, route platform.RouteSpec, fixture workloadCommandFixture) {
	t.Helper()
	if route.Resource != strings.TrimPrefix(fixture.Resource, fixture.OwnerService+":") {
		t.Fatalf("route resource = %q, want fixture resource %q", route.Resource, fixture.Resource)
	}
	if route.Action != fixture.Action {
		t.Fatalf("route action = %q, want %q", route.Action, fixture.Action)
	}
	if route.AuthRequired {
		t.Fatal("route AuthRequired = true, want service-internal route to bypass user auth")
	}
	if !route.PolicyBypass {
		t.Fatal("route PolicyBypass = false, want service-internal policy bypass")
	}
	if !route.StateChanging {
		t.Fatal("route StateChanging = false, want command route to be state-changing")
	}
	for _, param := range fixture.PathParameters {
		if !strings.Contains(route.Pattern, "{"+param+"}") {
			t.Fatalf("route pattern %q does not contain path parameter %q", route.Pattern, param)
		}
	}
}
