package schedulerquota

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

func TestSchedulerProfileExternalAPIFixturesMatchSpec(t *testing.T) {
	cases := []schedulerProfileFixtureCase{
		{
			name:           "accelerator",
			fixtureName:    "scheduler-create-accelerator-profile.json",
			contractName:   "scheduler.create_accelerator_profile",
			path:           "/api/v1/accelerator-profiles",
			resource:       "accelerator_profiles",
			requiredFields: []string{"name"},
			optionalFields: []string{"id", "enabled", "node_selector", "allowed_device_class_name"},
			event:          "AcceleratorProfileChanged",
		},
		{
			name:           "network",
			fixtureName:    "scheduler-create-network-profile.json",
			contractName:   "scheduler.create_network_profile",
			path:           "/api/v1/network-profiles",
			resource:       "network_profiles",
			requiredFields: []string{"name", "primary_cni"},
			optionalFields: []string{"id", "secondary_network", "rdma_enabled", "network_env"},
			event:          "NetworkProfileChanged",
		},
		{
			name:           "placement",
			fixtureName:    "scheduler-create-placement-profile.json",
			contractName:   "scheduler.create_placement_profile",
			path:           "/api/v1/placement-profiles",
			resource:       "placement_profiles",
			requiredFields: []string{"name", "scheduler_backend"},
			optionalFields: []string{"id", "scheduler_name", "gang_enabled", "queue_label_key"},
			event:          "PlacementProfileChanged",
		},
	}

	spec := Spec()
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			fixture := readSchedulerProfileExternalAPIFixture(t, tt.fixtureName)
			route, ok := findSchedulerProfileRoute(spec.Routes, fixture.Method, fixture.Path)
			if !ok {
				t.Fatalf("route %s %s not found in Spec()", fixture.Method, fixture.Path)
			}

			assertSchedulerProfileFixture(t, fixture, tt, spec, route)
			assertSchedulerProfileRoute(t, route, tt)
		})
	}
}

type schedulerProfileFixtureCase struct {
	name           string
	fixtureName    string
	contractName   string
	path           string
	resource       string
	requiredFields []string
	optionalFields []string
	event          string
}

type schedulerProfileExternalAPIFixture struct {
	ContractName          string         `json:"contract_name"`
	OwnerService          string         `json:"owner_service"`
	Resource              string         `json:"resource"`
	Action                string         `json:"action"`
	Method                string         `json:"method"`
	Path                  string         `json:"path"`
	Auth                  string         `json:"auth"`
	AuthRequired          bool           `json:"auth_required"`
	ServiceKeyRequired    bool           `json:"service_key_required"`
	PathParameters        []string       `json:"path_parameters"`
	RequiredRequestFields []string       `json:"required_request_fields"`
	OptionalRequestFields []string       `json:"optional_request_fields"`
	SuccessStatuses       []int          `json:"success_statuses"`
	EmitsEvents           []string       `json:"emits_events"`
	ResponseExample       map[string]any `json:"response_example"`
}

func assertSchedulerProfileFixture(t *testing.T, fixture schedulerProfileExternalAPIFixture, want schedulerProfileFixtureCase, spec platform.ServiceSpec, route platform.RouteSpec) {
	t.Helper()
	assertSchedulerProfileFixtureIdentity(t, fixture, want, spec, route)
	assertSchedulerProfileFixtureAuth(t, fixture, route)
	assertSchedulerProfileFixtureFields(t, fixture, want)
	assertSchedulerProfileFixtureEvent(t, fixture, want, spec)
	assertSchedulerProfileResponseKeys(t, fixture.ResponseExample)
}

func assertSchedulerProfileFixtureIdentity(t *testing.T, fixture schedulerProfileExternalAPIFixture, want schedulerProfileFixtureCase, spec platform.ServiceSpec, route platform.RouteSpec) {
	t.Helper()
	if fixture.ContractName != want.contractName {
		t.Fatalf("contract_name = %q, want %q", fixture.ContractName, want.contractName)
	}
	if fixture.OwnerService != spec.Name {
		t.Fatalf("owner_service = %q, want %q", fixture.OwnerService, spec.Name)
	}
	if got, want := fixture.Resource, spec.Name+":"+route.Resource; got != want {
		t.Fatalf("resource = %q, want %q", got, want)
	}
	if fixture.Action != route.Action {
		t.Fatalf("action = %q, want %q", fixture.Action, route.Action)
	}
	if fixture.Method != route.Method || fixture.Path != route.Pattern {
		t.Fatalf("fixture route = %s %s, want %s %s", fixture.Method, fixture.Path, route.Method, route.Pattern)
	}
}

func assertSchedulerProfileFixtureAuth(t *testing.T, fixture schedulerProfileExternalAPIFixture, route platform.RouteSpec) {
	t.Helper()
	if fixture.Auth != "user" || !fixture.AuthRequired || fixture.ServiceKeyRequired ||
		fixture.AuthRequired != route.AuthRequired || fixture.ServiceKeyRequired != route.ServiceAuthRequired {
		t.Fatalf("auth metadata = %q/%v/%v, want user/%v/%v", fixture.Auth, fixture.AuthRequired, fixture.ServiceKeyRequired, route.AuthRequired, route.ServiceAuthRequired)
	}
	if len(fixture.PathParameters) != 0 {
		t.Fatalf("path_parameters = %v, want none", fixture.PathParameters)
	}
}

func assertSchedulerProfileFixtureFields(t *testing.T, fixture schedulerProfileExternalAPIFixture, want schedulerProfileFixtureCase) {
	t.Helper()
	if !reflect.DeepEqual(fixture.RequiredRequestFields, want.requiredFields) {
		t.Fatalf("required_request_fields = %v, want %v", fixture.RequiredRequestFields, want.requiredFields)
	}
	assertSchedulerProfileStringsContainAll(t, "optional_request_fields", fixture.OptionalRequestFields, want.optionalFields)
	if !reflect.DeepEqual(fixture.SuccessStatuses, []int{http.StatusCreated}) {
		t.Fatalf("success_statuses = %v, want [201]", fixture.SuccessStatuses)
	}
}

func assertSchedulerProfileFixtureEvent(t *testing.T, fixture schedulerProfileExternalAPIFixture, want schedulerProfileFixtureCase, spec platform.ServiceSpec) {
	t.Helper()
	if !reflect.DeepEqual(fixture.EmitsEvents, []string{want.event}) {
		t.Fatalf("emits_events = %v, want [%s]", fixture.EmitsEvents, want.event)
	}
	if !schedulerQuotaSpecEmitsEvent(spec, want.event) {
		t.Fatalf("spec events = %v, want %s", spec.Events, want.event)
	}
}

func assertSchedulerProfileRoute(t *testing.T, route platform.RouteSpec, want schedulerProfileFixtureCase) {
	t.Helper()
	if route.Method != http.MethodPost || route.Pattern != want.path {
		t.Fatalf("route = %s %s, want POST %s", route.Method, route.Pattern, want.path)
	}
	if route.Resource != want.resource || route.Action != "create" {
		t.Fatalf("route metadata = %s/%s, want %s/create", route.Resource, route.Action, want.resource)
	}
	if !route.Admin {
		t.Fatal("route Admin = false, want true")
	}
	if !route.StateChanging {
		t.Fatal("route StateChanging = false, want true")
	}
	if route.ServiceAuthRequired {
		t.Fatal("route ServiceAuthRequired = true, want false")
	}
	if route.IDParam != "" {
		t.Fatalf("route IDParam = %q, want none", route.IDParam)
	}
}

func assertSchedulerProfileResponseKeys(t *testing.T, response map[string]any) {
	t.Helper()
	for _, key := range []string{"id", "data", "version", "created_at", "updated_at"} {
		if _, ok := response[key]; !ok {
			t.Fatalf("response_example missing %q", key)
		}
	}
}

func assertSchedulerProfileStringsContainAll(t *testing.T, field string, got, want []string) {
	t.Helper()
	present := make(map[string]bool, len(got))
	for _, value := range got {
		present[value] = true
	}
	for _, value := range want {
		if !present[value] {
			t.Fatalf("%s = %v, missing %q", field, got, value)
		}
	}
}

func readSchedulerProfileExternalAPIFixture(t *testing.T, name string) schedulerProfileExternalAPIFixture {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "contracts", "fixtures", "api", "v1", name))
	if err != nil {
		t.Fatalf("read scheduler profile external API fixture %s: %v", name, err)
	}
	var fixture schedulerProfileExternalAPIFixture
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatalf("unmarshal scheduler profile external API fixture %s: %v", name, err)
	}
	return fixture
}

func findSchedulerProfileRoute(routes []platform.RouteSpec, method, pattern string) (platform.RouteSpec, bool) {
	for _, route := range routes {
		if route.Method == method && route.Pattern == pattern {
			return route, true
		}
	}
	return platform.RouteSpec{}, false
}

func schedulerQuotaSpecEmitsEvent(spec platform.ServiceSpec, name string) bool {
	for _, event := range spec.Events {
		if event == name {
			return true
		}
	}
	return false
}
