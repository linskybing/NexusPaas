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

func TestImageBuildCreateExternalAPIFixturesMatchSpec(t *testing.T) {
	cases := []imageBuildCreateFixtureCase{
		{
			name:         "context",
			fixtureName:  "image-registry-context-build.json",
			contractName: "image-registry.context_build",
			path:         "/api/v1/images/build",
			buildType:    "context",
			optionalFields: []string{
				"context",
				"build_args",
				"tag",
				"registry",
				"repository",
				"cpu",
				"memory_gb",
				"max_build_seconds",
			},
		},
		{
			name:         "dockerfile",
			fixtureName:  "image-registry-dockerfile-build.json",
			contractName: "image-registry.dockerfile_build",
			path:         "/api/v1/images/build/dockerfile",
			buildType:    "dockerfile",
			optionalFields: []string{
				"dockerfile",
				"context",
				"build_args",
				"tag",
				"registry",
				"repository",
				"cpu",
				"memory_gb",
				"max_build_seconds",
			},
		},
		{
			name:         "storage",
			fixtureName:  "image-registry-storage-build.json",
			contractName: "image-registry.storage_build",
			path:         "/api/v1/images/build/from-storage",
			buildType:    "storage",
			// storage_path became required with the from-storage source
			// permission gate (IMG-002).
			requiredExtras: []string{"storage_path"},
			optionalFields: []string{
				"tag",
				"registry",
				"repository",
				"cpu",
				"memory_gb",
				"max_build_seconds",
			},
		},
	}

	spec := Spec()
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			fixture := readImageBuildCreateExternalAPIFixture(t, tt.fixtureName)
			route, ok := findRoute(spec.Routes, fixture.Method, fixture.Path)
			if !ok {
				t.Fatalf("route %s %s not found in Spec()", fixture.Method, fixture.Path)
			}

			assertImageBuildCreateFixtureMetadata(t, fixture, tt, spec.Name, route)
			assertImageBuildCreateRouteMetadata(t, route, fixture, tt.path)
		})
	}
}

func TestImageAccelerationProfileExternalAPIFixtureMatchesSpec(t *testing.T) {
	spec := Spec()
	fixture := readImageBuildCreateExternalAPIFixture(t, "image-registry-create-acceleration-profile.json")
	route, ok := findRoute(spec.Routes, fixture.Method, fixture.Path)
	if !ok {
		t.Fatalf("route %s %s not found in Spec()", fixture.Method, fixture.Path)
	}
	if fixture.ContractName != "image-registry.create_acceleration_profile" {
		t.Fatalf("contract_name = %q, want image-registry.create_acceleration_profile", fixture.ContractName)
	}
	if fixture.OwnerService != spec.Name {
		t.Fatalf("owner_service = %q, want %q", fixture.OwnerService, spec.Name)
	}
	if got, want := fixture.Resource, spec.Name+":"+route.Resource; got != want {
		t.Fatalf("resource = %q, want %q", got, want)
	}
	if route.Method != http.MethodPost || route.Pattern != "/api/v1/image-acceleration-profiles" ||
		route.Resource != "image_acceleration_profiles" || route.Action != "create" || !route.Admin {
		t.Fatalf("route metadata = %#v, want admin create image_acceleration_profiles", route)
	}
	if fixture.Auth != "user" || !fixture.AuthRequired || fixture.ServiceKeyRequired {
		t.Fatalf("auth metadata = %q/%v/%v, want user/true/false", fixture.Auth, fixture.AuthRequired, fixture.ServiceKeyRequired)
	}
	if !reflect.DeepEqual(fixture.RequiredRequestFields, []string{"name", "snapshotter", "prewarm_policy"}) {
		t.Fatalf("required_request_fields = %v, want name/snapshotter/prewarm_policy", fixture.RequiredRequestFields)
	}
	assertFixtureStringsContainAll(t, "optional_request_fields", fixture.OptionalRequestFields, []string{"id", "conversion_required", "allowed_for_projects"})
	assertFixtureIntsContainAll(t, "success_statuses", fixture.SuccessStatuses, []int{http.StatusCreated})
	assertFixtureStringsContainAll(t, "emits_events", fixture.EmitsEvents, []string{"ImageAccelerationProfileChanged"})
	if !imageRegistrySpecEmitsEvent(spec, "ImageAccelerationProfileChanged") {
		t.Fatalf("spec events = %v, want ImageAccelerationProfileChanged", spec.Events)
	}
	assertFixtureStringField(t, "request_example", fixture.RequestExample, "snapshotter", "stargz")
	assertFixtureStringField(t, "request_example", fixture.RequestExample, "prewarm_policy", "nodepool-based")
}

func assertImageBuildCreateFixtureMetadata(t *testing.T, fixture imageBuildCreateExternalAPIFixture, want imageBuildCreateFixtureCase, serviceName string, route platform.RouteSpec) {
	t.Helper()
	if fixture.ContractName != want.contractName {
		t.Fatalf("contract_name = %q, want %q", fixture.ContractName, want.contractName)
	}
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
	requiredFields := []string{"project_id", "image_reference", "cpu_cores", "memory_gib", "max_build_time_seconds"}
	requiredFields = append(requiredFields, want.requiredExtras...)
	if !reflect.DeepEqual(fixture.RequiredRequestFields, requiredFields) {
		t.Fatalf("required_request_fields = %v, want %v", fixture.RequiredRequestFields, requiredFields)
	}
	assertFixtureStringsContainAll(t, "optional_request_fields", fixture.OptionalRequestFields, want.optionalFields)
	assertFixtureStringsContainAll(t, "optional_request_headers", fixture.OptionalRequestHeaders, []string{"Idempotency-Key"})
	assertFixtureIntsContainAll(t, "error_statuses", fixture.ErrorStatuses, []int{http.StatusConflict})
	assertImageBuildResourceExample(t, "request_example", fixture.RequestExample)
	assertImageBuildResourceExample(t, "response_example", fixture.ResponseExample)
	assertImageBuildSupplyChainResponseExample(t, fixture.ResponseExample)
	assertFixtureStringField(t, "response_example", fixture.ResponseExample, "build_type", want.buildType)
	if !reflect.DeepEqual(fixture.SuccessStatuses, []int{http.StatusAccepted}) {
		t.Fatalf("success_statuses = %v, want [202]", fixture.SuccessStatuses)
	}
	if !reflect.DeepEqual(fixture.EmitsEvents, []string{"ImageBuildStarted"}) {
		t.Fatalf("emits_events = %v, want [ImageBuildStarted]", fixture.EmitsEvents)
	}
}

func assertImageBuildCreateRouteMetadata(t *testing.T, route platform.RouteSpec, fixture imageBuildCreateExternalAPIFixture, path string) {
	t.Helper()
	if got, want := route.Resource, "image_builds"; got != want {
		t.Fatalf("route resource = %q, want %q", got, want)
	}
	if got, want := route.Action, "command"; got != want {
		t.Fatalf("route action = %q, want %q", got, want)
	}
	if route.Method != http.MethodPost || route.Pattern != path {
		t.Fatalf("route = %s %s, want POST %s", route.Method, route.Pattern, path)
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

type imageBuildCreateFixtureCase struct {
	name           string
	fixtureName    string
	contractName   string
	path           string
	buildType      string
	requiredExtras []string
	optionalFields []string
}

type imageBuildCreateExternalAPIFixture struct {
	ContractName           string         `json:"contract_name"`
	OwnerService           string         `json:"owner_service"`
	Resource               string         `json:"resource"`
	Action                 string         `json:"action"`
	Method                 string         `json:"method"`
	Path                   string         `json:"path"`
	Auth                   string         `json:"auth"`
	AuthRequired           bool           `json:"auth_required"`
	ServiceKeyRequired     bool           `json:"service_key_required"`
	PathParameters         []string       `json:"path_parameters"`
	RequiredRequestFields  []string       `json:"required_request_fields"`
	OptionalRequestFields  []string       `json:"optional_request_fields"`
	OptionalRequestHeaders []string       `json:"optional_request_headers"`
	RequestExample         map[string]any `json:"request_example"`
	SuccessStatuses        []int          `json:"success_statuses"`
	ErrorStatuses          []int          `json:"error_statuses"`
	EmitsEvents            []string       `json:"emits_events"`
	ResponseExample        map[string]any `json:"response_example"`
}

func assertFixtureStringsContainAll(t *testing.T, field string, got, want []string) {
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

func assertFixtureIntsContainAll(t *testing.T, field string, got, want []int) {
	t.Helper()
	present := make(map[int]bool, len(got))
	for _, value := range got {
		present[value] = true
	}
	for _, value := range want {
		if !present[value] {
			t.Fatalf("%s = %v, missing %d", field, got, value)
		}
	}
}

func assertImageBuildResourceExample(t *testing.T, field string, example map[string]any) {
	t.Helper()
	assertFixtureNumberField(t, field, example, "cpu_cores", 2)
	assertFixtureNumberField(t, field, example, "memory_gib", 4)
	assertFixtureNumberField(t, field, example, "max_build_time_seconds", 600)
}

func assertImageBuildSupplyChainResponseExample(t *testing.T, example map[string]any) {
	t.Helper()
	assertFixtureStringField(t, "response_example", example, "image_digest", "")
	assertFixtureStringField(t, "response_example", example, "allow_list_decision", "pending")
	assertFixtureStringField(t, "response_example", example, "sbom_status", "pending")
	assertFixtureStringField(t, "response_example", example, "signature_status", "pending")
	assertFixtureStringField(t, "response_example", example, "scan_status", "pending")
	if value, ok := example["supply_chain_checked_at"]; !ok || value != nil {
		t.Fatalf("response_example.supply_chain_checked_at = %v, want null", value)
	}
}

func assertFixtureNumberField(t *testing.T, field string, data map[string]any, key string, want float64) {
	t.Helper()
	got, ok := data[key].(float64)
	if !ok || got != want {
		t.Fatalf("%s.%s = %v, want %v", field, key, data[key], want)
	}
}

func assertFixtureStringField(t *testing.T, field string, data map[string]any, key string, want string) {
	t.Helper()
	got, ok := data[key].(string)
	if !ok || got != want {
		t.Fatalf("%s.%s = %v, want %q", field, key, data[key], want)
	}
}

func findRoute(routes []platform.RouteSpec, method, pattern string) (platform.RouteSpec, bool) {
	for _, route := range routes {
		if route.Method == method && route.Pattern == pattern {
			return route, true
		}
	}
	return platform.RouteSpec{}, false
}

func imageRegistrySpecEmitsEvent(spec platform.ServiceSpec, name string) bool {
	for _, event := range spec.Events {
		if event == name {
			return true
		}
	}
	return false
}

func readImageBuildCreateExternalAPIFixture(t *testing.T, name string) imageBuildCreateExternalAPIFixture {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "contracts", "fixtures", "api", "v1", name))
	if err != nil {
		t.Fatalf("read image-registry build external API fixture %s: %v", name, err)
	}
	var fixture imageBuildCreateExternalAPIFixture
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatalf("unmarshal image-registry build external API fixture %s: %v", name, err)
	}
	return fixture
}
