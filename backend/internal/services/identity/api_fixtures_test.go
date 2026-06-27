package identity

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

func TestIdentityAuthExternalAPIFixturesMatchSpec(t *testing.T) {
	cases := []identityAuthExternalAPIFixtureCase{
		{
			fixtureName:    "identity-register.json",
			contractName:   "identity.register",
			path:           "/api/v1/register",
			resource:       "users",
			requiredFields: []string{"username", "password"},
			optionalFields: []string{"email", "full_name", "name"},
			emitsEvents:    []string{"UserCreated"},
		},
		{
			fixtureName:    "identity-login.json",
			contractName:   "identity.login",
			path:           "/api/v1/login",
			resource:       "sessions",
			requiredFields: []string{"username", "password"},
			optionalFields: []string{"captcha_id", "captcha_answer"},
			emitsEvents:    []string{},
		},
		{
			fixtureName:    "identity-refresh.json",
			contractName:   "identity.refresh",
			path:           "/api/v1/refresh",
			resource:       "refresh_tokens",
			requiredFields: []string{"refresh_token"},
			optionalFields: []string{},
			emitsEvents:    []string{},
		},
		{
			fixtureName:    "identity-cli-login.json",
			contractName:   "identity.cli_login",
			path:           "/api/v1/cli/login",
			resource:       "cli_sessions",
			requiredFields: []string{"username", "password"},
			optionalFields: []string{"name"},
			emitsEvents:    []string{},
		},
	}

	spec := Spec()
	for _, tt := range cases {
		t.Run(tt.fixtureName, func(t *testing.T) {
			fixture := readIdentityAuthExternalAPIFixture(t, tt.fixtureName)
			route, ok := findIdentityAuthRoute(spec.Routes, fixture.Method, fixture.Path)
			if !ok {
				t.Fatalf("route %s %s not found in Spec()", fixture.Method, fixture.Path)
			}

			assertIdentityAuthFixtureMetadata(t, fixture, tt, spec, route)
			assertIdentityAuthRouteMetadata(t, route, tt)
		})
	}
}

type identityAuthExternalAPIFixtureCase struct {
	fixtureName    string
	contractName   string
	path           string
	resource       string
	requiredFields []string
	optionalFields []string
	emitsEvents    []string
}

type identityAuthExternalAPIFixture struct {
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
	RequestExample        map[string]any `json:"request_example"`
	ResponseExample       map[string]any `json:"response_example"`
}

func assertIdentityAuthFixtureMetadata(t *testing.T, fixture identityAuthExternalAPIFixture, want identityAuthExternalAPIFixtureCase, spec platform.ServiceSpec, route platform.RouteSpec) {
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
	if fixture.Auth != "public" || fixture.AuthRequired || fixture.ServiceKeyRequired {
		t.Fatalf("auth metadata = %q/%v/%v, want public/false/false", fixture.Auth, fixture.AuthRequired, fixture.ServiceKeyRequired)
	}
	if len(fixture.PathParameters) != 0 {
		t.Fatalf("path_parameters = %v, want none", fixture.PathParameters)
	}
	if !reflect.DeepEqual(fixture.RequiredRequestFields, want.requiredFields) {
		t.Fatalf("required_request_fields = %v, want %v", fixture.RequiredRequestFields, want.requiredFields)
	}
	if !reflect.DeepEqual(fixture.OptionalRequestFields, want.optionalFields) {
		t.Fatalf("optional_request_fields = %v, want %v", fixture.OptionalRequestFields, want.optionalFields)
	}
	if !reflect.DeepEqual(fixture.SuccessStatuses, []int{http.StatusOK}) {
		t.Fatalf("success_statuses = %v, want [200]", fixture.SuccessStatuses)
	}
	if !reflect.DeepEqual(fixture.EmitsEvents, want.emitsEvents) {
		t.Fatalf("emits_events = %v, want %v", fixture.EmitsEvents, want.emitsEvents)
	}
	if len(want.emitsEvents) > 0 && !identitySpecEmitsEvent(spec, want.emitsEvents[0]) {
		t.Fatalf("spec events = %v, want %s", spec.Events, want.emitsEvents[0])
	}
	if fixture.RequestExample == nil || fixture.ResponseExample == nil {
		t.Fatal("request_example and response_example must be present")
	}
}

func assertIdentityAuthRouteMetadata(t *testing.T, route platform.RouteSpec, want identityAuthExternalAPIFixtureCase) {
	t.Helper()
	if route.Method != http.MethodPost || route.Pattern != want.path {
		t.Fatalf("route = %s %s, want POST %s", route.Method, route.Pattern, want.path)
	}
	if route.Resource != want.resource || route.Action != "create" {
		t.Fatalf("route metadata = %s/%s, want %s/create", route.Resource, route.Action, want.resource)
	}
	if route.AuthRequired || route.ServiceAuthRequired || route.Admin {
		t.Fatalf("route auth metadata = auth_required:%v service_auth_required:%v admin:%v, want public", route.AuthRequired, route.ServiceAuthRequired, route.Admin)
	}
	if !route.StateChanging {
		t.Fatal("route StateChanging = false, want true")
	}
	if route.IDParam != "" {
		t.Fatalf("route IDParam = %q, want none", route.IDParam)
	}
}

func findIdentityAuthRoute(routes []platform.RouteSpec, method, pattern string) (platform.RouteSpec, bool) {
	for _, route := range routes {
		if route.Method == method && route.Pattern == pattern {
			return route, true
		}
	}
	return platform.RouteSpec{}, false
}

func identitySpecEmitsEvent(spec platform.ServiceSpec, name string) bool {
	for _, event := range spec.Events {
		if event == name {
			return true
		}
	}
	return false
}

func readIdentityAuthExternalAPIFixture(t *testing.T, name string) identityAuthExternalAPIFixture {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "contracts", "fixtures", "api", "v1", name))
	if err != nil {
		t.Fatalf("read identity external API fixture %s: %v", name, err)
	}
	var fixture identityAuthExternalAPIFixture
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatalf("unmarshal identity external API fixture %s: %v", name, err)
	}
	return fixture
}
