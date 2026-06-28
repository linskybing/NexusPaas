package authorizationpolicy

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

func TestProxyRoleExternalAPIFixturesMatchSpec(t *testing.T) {
	cases := []proxyRoleFixtureCase{
		{
			name:           "create",
			fixtureName:    "authorization-policy-create-proxy-role.json",
			contractName:   "authorization-policy.create_proxy_role",
			method:         http.MethodPost,
			path:           "/api/v1/admin/proxy-rbac/roles",
			action:         "create",
			pathParameters: []string{},
			requiredFields: []string{"name", "display_name"},
			optionalFields: []string{"id", "description"},
			success:        []int{http.StatusCreated},
			errors:         []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusConflict, http.StatusInternalServerError},
			responseFields: []string{"id", "name", "display_name", "description", "created_at", "updated_at"},
		},
		{
			name:           "update",
			fixtureName:    "authorization-policy-update-proxy-role.json",
			contractName:   "authorization-policy.update_proxy_role",
			method:         http.MethodPut,
			path:           "/api/v1/admin/proxy-rbac/roles/{id}",
			action:         "update",
			idParam:        "id",
			pathParameters: []string{"id"},
			requiredFields: []string{"display_name"},
			optionalFields: []string{"description"},
			success:        []int{http.StatusOK},
			errors:         []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusInternalServerError},
			responseFields: []string{"id", "name", "display_name", "description", "created_at", "updated_at"},
		},
		{
			name:           "delete",
			fixtureName:    "authorization-policy-delete-proxy-role.json",
			contractName:   "authorization-policy.delete_proxy_role",
			method:         http.MethodDelete,
			path:           "/api/v1/admin/proxy-rbac/roles/{id}",
			action:         "delete",
			idParam:        "id",
			pathParameters: []string{"id"},
			requiredFields: []string{},
			optionalFields: []string{},
			success:        []int{http.StatusOK},
			errors:         []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusInternalServerError},
		},
	}

	spec := Spec()
	if !authorizationPolicySpecEmitsEvent(spec, "ProxyPolicyChanged") {
		t.Fatalf("spec events = %v, want ProxyPolicyChanged", spec.Events)
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			fixture := readAuthorizationPolicyExternalAPIFixture(t, tt.fixtureName)
			route, ok := findAuthorizationPolicyRoute(spec.Routes, fixture.Method, fixture.Path)
			if !ok {
				t.Fatalf("route %s %s not found in Spec()", fixture.Method, fixture.Path)
			}

			assertProxyRoleFixtureMetadata(t, fixture, spec, route, tt)
			assertProxyRoleRouteMetadata(t, route, fixture, tt)
			assertProxyRoleResponseExample(t, fixture, tt)
		})
	}
}

type proxyRoleFixtureCase struct {
	name           string
	fixtureName    string
	contractName   string
	method         string
	path           string
	action         string
	idParam        string
	pathParameters []string
	requiredFields []string
	optionalFields []string
	success        []int
	errors         []int
	responseFields []string
}

type authorizationPolicyExternalAPIFixture struct {
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
	RequestExample        map[string]any `json:"request_example"`
	SuccessStatuses       []int          `json:"success_statuses"`
	ErrorStatuses         []int          `json:"error_statuses"`
	EmitsEvents           []string       `json:"emits_events"`
	ResponseExample       map[string]any `json:"response_example"`
}

func assertProxyRoleFixtureMetadata(t *testing.T, fixture authorizationPolicyExternalAPIFixture, spec platform.ServiceSpec, route platform.RouteSpec, want proxyRoleFixtureCase) {
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
	if fixture.Action != route.Action || fixture.Action != want.action {
		t.Fatalf("action = %q, want %q", fixture.Action, want.action)
	}
	if fixture.Auth != "user" || fixture.AuthRequired != route.AuthRequired || fixture.ServiceKeyRequired != route.ServiceAuthRequired {
		t.Fatalf("auth metadata = %q/%v/%v, want user/%v/%v", fixture.Auth, fixture.AuthRequired, fixture.ServiceKeyRequired, route.AuthRequired, route.ServiceAuthRequired)
	}
	if !reflect.DeepEqual(fixture.PathParameters, want.pathParameters) {
		t.Fatalf("path_parameters = %v, want %v", fixture.PathParameters, want.pathParameters)
	}
	if !reflect.DeepEqual(fixture.RequiredRequestFields, want.requiredFields) {
		t.Fatalf("required_request_fields = %v, want %v", fixture.RequiredRequestFields, want.requiredFields)
	}
	if !reflect.DeepEqual(fixture.OptionalRequestFields, want.optionalFields) {
		t.Fatalf("optional_request_fields = %v, want %v", fixture.OptionalRequestFields, want.optionalFields)
	}
	if !reflect.DeepEqual(fixture.SuccessStatuses, want.success) {
		t.Fatalf("success_statuses = %v, want %v", fixture.SuccessStatuses, want.success)
	}
	if !reflect.DeepEqual(fixture.ErrorStatuses, want.errors) {
		t.Fatalf("error_statuses = %v, want %v", fixture.ErrorStatuses, want.errors)
	}
	if !reflect.DeepEqual(fixture.EmitsEvents, []string{"ProxyPolicyChanged"}) {
		t.Fatalf("emits_events = %v, want [ProxyPolicyChanged]", fixture.EmitsEvents)
	}
}

func assertProxyRoleRouteMetadata(t *testing.T, route platform.RouteSpec, fixture authorizationPolicyExternalAPIFixture, want proxyRoleFixtureCase) {
	t.Helper()
	if route.Method != want.method || route.Pattern != want.path {
		t.Fatalf("route = %s %s, want %s %s", route.Method, route.Pattern, want.method, want.path)
	}
	if route.Resource != "proxy_roles" || route.Action != want.action {
		t.Fatalf("route metadata = %#v, want proxy_roles/%s", route, want.action)
	}
	if route.IDParam != want.idParam {
		t.Fatalf("route IDParam = %q, want %q", route.IDParam, want.idParam)
	}
	if !route.Admin {
		t.Fatal("route Admin = false, want true")
	}
	if !route.AuthRequired {
		t.Fatal("route AuthRequired = false, want true")
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

func assertProxyRoleResponseExample(t *testing.T, fixture authorizationPolicyExternalAPIFixture, want proxyRoleFixtureCase) {
	t.Helper()
	if want.action == "delete" {
		if len(fixture.RequestExample) != 0 || len(fixture.ResponseExample) != 0 {
			t.Fatalf("delete request/response examples = %v/%v, want empty objects", fixture.RequestExample, fixture.ResponseExample)
		}
		return
	}
	for _, field := range want.requiredFields {
		if fixture.RequestExample[field] == "" {
			t.Fatalf("request_example.%s = %v, want non-empty", field, fixture.RequestExample[field])
		}
	}
	for _, field := range want.responseFields {
		if fixture.ResponseExample[field] == "" {
			t.Fatalf("response_example.%s = %v, want non-empty", field, fixture.ResponseExample[field])
		}
	}
	if value, ok := fixture.ResponseExample["is_system"].(bool); !ok || value {
		t.Fatalf("response_example.is_system = %v, want false", fixture.ResponseExample["is_system"])
	}
}

func findAuthorizationPolicyRoute(routes []platform.RouteSpec, method, pattern string) (platform.RouteSpec, bool) {
	for _, route := range routes {
		if route.Method == method && route.Pattern == pattern {
			return route, true
		}
	}
	return platform.RouteSpec{}, false
}

func authorizationPolicySpecEmitsEvent(spec platform.ServiceSpec, name string) bool {
	for _, event := range spec.Events {
		if event == name {
			return true
		}
	}
	return false
}

func readAuthorizationPolicyExternalAPIFixture(t *testing.T, name string) authorizationPolicyExternalAPIFixture {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "contracts", "fixtures", "api", "v1", name))
	if err != nil {
		t.Fatalf("read authorization-policy external API fixture %s: %v", name, err)
	}
	var fixture authorizationPolicyExternalAPIFixture
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatalf("unmarshal authorization-policy external API fixture %s: %v", name, err)
	}
	return fixture
}
