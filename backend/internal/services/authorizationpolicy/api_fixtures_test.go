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
	cases := []proxyRBACFixtureCase{
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

	assertProxyRBACExternalAPIFixturesMatchSpec(t, cases)
}

func TestProxyPolicyExternalAPIFixturesMatchSpec(t *testing.T) {
	cases := []proxyRBACFixtureCase{
		{
			name:           "create",
			fixtureName:    "authorization-policy-create-proxy-policy.json",
			contractName:   "authorization-policy.create_proxy_policy",
			method:         http.MethodPost,
			path:           "/api/v1/admin/proxy-rbac/policies",
			resource:       "proxy_policies",
			action:         "create",
			pathParameters: []string{},
			requiredFields: []string{"name"},
			optionalFields: []string{"id", "description", "rules"},
			success:        []int{http.StatusCreated},
			errors:         []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusConflict, http.StatusInternalServerError},
			responseFields: []string{"id", "name", "description", "created_at", "updated_at"},
			expectRules:    true,
		},
		{
			name:           "update",
			fixtureName:    "authorization-policy-update-proxy-policy.json",
			contractName:   "authorization-policy.update_proxy_policy",
			method:         http.MethodPut,
			path:           "/api/v1/admin/proxy-rbac/policies/{id}",
			resource:       "proxy_policies",
			action:         "update",
			idParam:        "id",
			pathParameters: []string{"id"},
			requiredFields: []string{},
			optionalFields: []string{"name", "description", "rules"},
			success:        []int{http.StatusOK},
			errors:         []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusConflict, http.StatusInternalServerError},
			responseFields: []string{"id", "name", "description", "created_at", "updated_at"},
			expectRules:    true,
		},
		{
			name:           "delete",
			fixtureName:    "authorization-policy-delete-proxy-policy.json",
			contractName:   "authorization-policy.delete_proxy_policy",
			method:         http.MethodDelete,
			path:           "/api/v1/admin/proxy-rbac/policies/{id}",
			resource:       "proxy_policies",
			action:         "delete",
			idParam:        "id",
			pathParameters: []string{"id"},
			requiredFields: []string{},
			optionalFields: []string{},
			success:        []int{http.StatusOK},
			errors:         []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusInternalServerError},
		},
	}

	assertProxyRBACExternalAPIFixturesMatchSpec(t, cases)
}

func TestProxyServiceReadExternalAPIFixturesMatchSpec(t *testing.T) {
	cases := []proxyServiceReadFixtureCase{
		{
			name:           "list",
			fixtureName:    "authorization-policy-list-proxy-services.json",
			contractName:   "authorization-policy.list_proxy_services",
			path:           "/api/v1/admin/proxy-rbac/services",
			action:         "list",
			pathParameters: []string{},
			errors:         []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusInternalServerError},
			collection:     true,
		},
		{
			name:           "get",
			fixtureName:    "authorization-policy-get-proxy-service.json",
			contractName:   "authorization-policy.get_proxy_service",
			path:           "/api/v1/admin/proxy-rbac/services/{id}",
			action:         "get",
			idParam:        "id",
			pathParameters: []string{"id"},
			errors:         []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusInternalServerError},
		},
	}

	assertProxyServiceReadExternalAPIFixturesMatchSpec(t, cases)
}

func TestProxyRoleUserExternalAPIFixturesMatchSpec(t *testing.T) {
	cases := []proxyRoleUserFixtureCase{
		{
			name:           "list",
			fixtureName:    "authorization-policy-list-proxy-role-users.json",
			contractName:   "authorization-policy.list_proxy_role_users",
			method:         http.MethodGet,
			path:           "/api/v1/admin/proxy-rbac/roles/{id}/users",
			action:         "list",
			idParam:        "id",
			pathParameters: []string{"id"},
			requiredFields: []string{},
			success:        []int{http.StatusOK},
			errors:         []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusInternalServerError},
			emitsEvents:    []string{},
			collection:     true,
		},
		{
			name:           "assign",
			fixtureName:    "authorization-policy-assign-proxy-role-user.json",
			contractName:   "authorization-policy.assign_proxy_role_user",
			method:         http.MethodPost,
			path:           "/api/v1/admin/proxy-rbac/roles/{id}/users",
			action:         "create",
			idParam:        "id",
			pathParameters: []string{"id"},
			requiredFields: []string{"user_id"},
			success:        []int{http.StatusOK, http.StatusCreated},
			errors:         []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusConflict, http.StatusInternalServerError},
			emitsEvents:    []string{"ProxyPolicyChanged"},
			stateChanging:  true,
		},
		{
			name:           "unassign",
			fixtureName:    "authorization-policy-unassign-proxy-role-user.json",
			contractName:   "authorization-policy.unassign_proxy_role_user",
			method:         http.MethodDelete,
			path:           "/api/v1/admin/proxy-rbac/roles/{id}/users/{user_id}",
			action:         "delete",
			idParam:        "user_id",
			pathParameters: []string{"id", "user_id"},
			requiredFields: []string{},
			success:        []int{http.StatusOK},
			errors:         []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusInternalServerError},
			emitsEvents:    []string{"ProxyPolicyChanged"},
			stateChanging:  true,
		},
	}

	assertProxyRoleUserExternalAPIFixturesMatchSpec(t, cases)
}

func TestProxySystemRoleExternalAPIFixturesMatchSpec(t *testing.T) {
	cases := []proxyRoleUserFixtureCase{
		{
			name:           "list",
			fixtureName:    "authorization-policy-list-proxy-system-roles.json",
			contractName:   "authorization-policy.list_proxy_system_roles",
			method:         http.MethodGet,
			path:           "/api/v1/admin/proxy-rbac/system-roles",
			resource:       "proxy_system_roles",
			action:         "list",
			pathParameters: []string{},
			requiredFields: []string{},
			success:        []int{http.StatusOK},
			errors:         []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusInternalServerError},
			emitsEvents:    []string{},
			collection:     true,
		},
	}

	assertProxyRoleUserExternalAPIFixturesMatchSpec(t, cases)
}

func TestProxyPolicyAssignmentExternalAPIFixturesMatchSpec(t *testing.T) {
	cases := []proxyRoleUserFixtureCase{
		{
			name:           "list",
			fixtureName:    "authorization-policy-list-proxy-policy-assignments.json",
			contractName:   "authorization-policy.list_proxy_policy_assignments",
			method:         http.MethodGet,
			path:           "/api/v1/admin/proxy-rbac/policies/{id}/assignments",
			resource:       "proxy_policy_assignments",
			action:         "list",
			idParam:        "id",
			pathParameters: []string{"id"},
			requiredFields: []string{},
			success:        []int{http.StatusOK},
			errors:         []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusInternalServerError},
			emitsEvents:    []string{},
			collection:     true,
		},
		{
			name:           "list-target",
			fixtureName:    "authorization-policy-list-proxy-target-assignments.json",
			contractName:   "authorization-policy.list_proxy_target_assignments",
			method:         http.MethodGet,
			path:           "/api/v1/admin/proxy-rbac/targets/{type}/{id}/assignments",
			resource:       "proxy_target_assignments",
			action:         "list",
			idParam:        "id",
			pathParameters: []string{"type", "id"},
			requiredFields: []string{},
			success:        []int{http.StatusOK},
			errors:         []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusInternalServerError},
			emitsEvents:    []string{},
			collection:     true,
		},
		{
			name:           "assign",
			fixtureName:    "authorization-policy-assign-proxy-policy.json",
			contractName:   "authorization-policy.assign_proxy_policy",
			method:         http.MethodPost,
			path:           "/api/v1/admin/proxy-rbac/policies/{id}/assignments",
			resource:       "proxy_policy_assignments",
			action:         "create",
			idParam:        "id",
			pathParameters: []string{"id"},
			requiredFields: []string{"target_type", "target_id"},
			success:        []int{http.StatusOK, http.StatusCreated},
			errors:         []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusConflict, http.StatusInternalServerError},
			emitsEvents:    []string{"ProxyPolicyChanged"},
			stateChanging:  true,
		},
		{
			name:           "unassign",
			fixtureName:    "authorization-policy-unassign-proxy-policy.json",
			contractName:   "authorization-policy.unassign_proxy_policy",
			method:         http.MethodDelete,
			path:           "/api/v1/admin/proxy-rbac/policies/{id}/assignments",
			resource:       "proxy_policy_assignments",
			action:         "delete",
			idParam:        "id",
			pathParameters: []string{"id"},
			requiredFields: []string{"target_type", "target_id"},
			success:        []int{http.StatusOK},
			errors:         []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusInternalServerError},
			emitsEvents:    []string{"ProxyPolicyChanged"},
			stateChanging:  true,
		},
	}

	assertProxyRoleUserExternalAPIFixturesMatchSpec(t, cases)
}

func assertProxyRBACExternalAPIFixturesMatchSpec(t *testing.T, cases []proxyRBACFixtureCase) {
	t.Helper()
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

			assertProxyRBACFixtureMetadata(t, fixture, spec, route, tt)
			assertProxyRBACRouteMetadata(t, route, fixture, tt)
			assertProxyRBACExamplePayloads(t, fixture, tt)
		})
	}
}

func assertProxyServiceReadExternalAPIFixturesMatchSpec(t *testing.T, cases []proxyServiceReadFixtureCase) {
	t.Helper()
	spec := Spec()

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			fixture := readAuthorizationPolicyExternalAPIFixture(t, tt.fixtureName)
			route, ok := findAuthorizationPolicyRoute(spec.Routes, fixture.Method, fixture.Path)
			if !ok {
				t.Fatalf("route %s %s not found in Spec()", fixture.Method, fixture.Path)
			}

			assertProxyServiceReadFixtureMetadata(t, fixture, spec, route, tt)
			assertProxyServiceReadRouteMetadata(t, route, fixture, tt)
			assertProxyServiceReadExamples(t, fixture, tt)
		})
	}
}

func assertProxyRoleUserExternalAPIFixturesMatchSpec(t *testing.T, cases []proxyRoleUserFixtureCase) {
	t.Helper()
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

			assertProxyRoleUserFixtureMetadata(t, fixture, spec, route, tt)
			assertProxyRoleUserRouteMetadata(t, route, fixture, tt)
			assertProxyRoleUserExamples(t, fixture, tt)
		})
	}
}

type proxyRBACFixtureCase struct {
	name           string
	fixtureName    string
	contractName   string
	method         string
	path           string
	resource       string
	action         string
	idParam        string
	pathParameters []string
	requiredFields []string
	optionalFields []string
	success        []int
	errors         []int
	responseFields []string
	expectRules    bool
}

type proxyServiceReadFixtureCase struct {
	name           string
	fixtureName    string
	contractName   string
	path           string
	action         string
	idParam        string
	pathParameters []string
	errors         []int
	collection     bool
}

type proxyRoleUserFixtureCase struct {
	name           string
	fixtureName    string
	contractName   string
	method         string
	path           string
	resource       string
	action         string
	idParam        string
	pathParameters []string
	requiredFields []string
	success        []int
	errors         []int
	emitsEvents    []string
	stateChanging  bool
	collection     bool
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

func assertProxyServiceReadFixtureMetadata(t *testing.T, fixture authorizationPolicyExternalAPIFixture, spec platform.ServiceSpec, route platform.RouteSpec, want proxyServiceReadFixtureCase) {
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
	if fixture.Auth != "user" || !fixture.AuthRequired || fixture.ServiceKeyRequired {
		t.Fatalf("auth metadata = %q/%v/%v, want user/true/false", fixture.Auth, fixture.AuthRequired, fixture.ServiceKeyRequired)
	}
	if !reflect.DeepEqual(fixture.PathParameters, want.pathParameters) {
		t.Fatalf("path_parameters = %v, want %v", fixture.PathParameters, want.pathParameters)
	}
	if len(fixture.RequiredRequestFields) != 0 || len(fixture.OptionalRequestFields) != 0 {
		t.Fatalf("request fields = %v/%v, want empty", fixture.RequiredRequestFields, fixture.OptionalRequestFields)
	}
	if !reflect.DeepEqual(fixture.SuccessStatuses, []int{http.StatusOK}) {
		t.Fatalf("success_statuses = %v, want [200]", fixture.SuccessStatuses)
	}
	if !reflect.DeepEqual(fixture.ErrorStatuses, want.errors) {
		t.Fatalf("error_statuses = %v, want %v", fixture.ErrorStatuses, want.errors)
	}
	if len(fixture.EmitsEvents) != 0 {
		t.Fatalf("emits_events = %v, want none", fixture.EmitsEvents)
	}
}

func assertProxyServiceReadRouteMetadata(t *testing.T, route platform.RouteSpec, fixture authorizationPolicyExternalAPIFixture, want proxyServiceReadFixtureCase) {
	t.Helper()
	if route.Method != http.MethodGet || route.Pattern != want.path {
		t.Fatalf("route = %s %s, want GET %s", route.Method, route.Pattern, want.path)
	}
	if route.Resource != "proxy_services" || route.Action != want.action {
		t.Fatalf("route metadata = %#v, want proxy_services/%s", route, want.action)
	}
	if route.IDParam != want.idParam {
		t.Fatalf("route IDParam = %q, want %q", route.IDParam, want.idParam)
	}
	if !route.Admin || !route.AuthRequired || route.ServiceAuthRequired || route.StateChanging {
		t.Fatalf("route auth/change metadata = admin:%v auth:%v service:%v state:%v, want true/true/false/false", route.Admin, route.AuthRequired, route.ServiceAuthRequired, route.StateChanging)
	}
	if fixture.Method != route.Method || fixture.Path != route.Pattern {
		t.Fatalf("fixture route = %s %s, want %s %s", fixture.Method, fixture.Path, route.Method, route.Pattern)
	}
}

func assertProxyServiceReadExamples(t *testing.T, fixture authorizationPolicyExternalAPIFixture, want proxyServiceReadFixtureCase) {
	t.Helper()
	if len(fixture.RequestExample) != 0 {
		t.Fatalf("request_example = %v, want empty object", fixture.RequestExample)
	}
	row := fixture.ResponseExample
	if want.collection {
		row = firstResponseItem(t, fixture.ResponseExample)
	}
	assertProxyServiceReadResponseRow(t, row)
}

func assertProxyRoleUserFixtureMetadata(t *testing.T, fixture authorizationPolicyExternalAPIFixture, spec platform.ServiceSpec, route platform.RouteSpec, want proxyRoleUserFixtureCase) {
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
	if fixture.Auth != "user" || !fixture.AuthRequired || fixture.ServiceKeyRequired {
		t.Fatalf("auth metadata = %q/%v/%v, want user/true/false", fixture.Auth, fixture.AuthRequired, fixture.ServiceKeyRequired)
	}
	if !reflect.DeepEqual(fixture.PathParameters, want.pathParameters) {
		t.Fatalf("path_parameters = %v, want %v", fixture.PathParameters, want.pathParameters)
	}
	if !reflect.DeepEqual(fixture.RequiredRequestFields, want.requiredFields) {
		t.Fatalf("required_request_fields = %v, want %v", fixture.RequiredRequestFields, want.requiredFields)
	}
	if len(fixture.OptionalRequestFields) != 0 {
		t.Fatalf("optional_request_fields = %v, want empty", fixture.OptionalRequestFields)
	}
	if !reflect.DeepEqual(fixture.SuccessStatuses, want.success) {
		t.Fatalf("success_statuses = %v, want %v", fixture.SuccessStatuses, want.success)
	}
	if !reflect.DeepEqual(fixture.ErrorStatuses, want.errors) {
		t.Fatalf("error_statuses = %v, want %v", fixture.ErrorStatuses, want.errors)
	}
	if !reflect.DeepEqual(fixture.EmitsEvents, want.emitsEvents) {
		t.Fatalf("emits_events = %v, want %v", fixture.EmitsEvents, want.emitsEvents)
	}
}

func assertProxyRoleUserRouteMetadata(t *testing.T, route platform.RouteSpec, fixture authorizationPolicyExternalAPIFixture, want proxyRoleUserFixtureCase) {
	t.Helper()
	if route.Method != want.method || route.Pattern != want.path {
		t.Fatalf("route = %s %s, want %s %s", route.Method, route.Pattern, want.method, want.path)
	}
	resource := want.resource
	if resource == "" {
		resource = "proxy_role_users"
	}
	if route.Resource != resource || route.Action != want.action {
		t.Fatalf("route metadata = %#v, want %s/%s", route, resource, want.action)
	}
	if route.IDParam != want.idParam {
		t.Fatalf("route IDParam = %q, want %q", route.IDParam, want.idParam)
	}
	if !route.Admin || !route.AuthRequired || route.ServiceAuthRequired || route.StateChanging != want.stateChanging {
		t.Fatalf("route auth/change metadata = admin:%v auth:%v service:%v state:%v, want true/true/false/%v", route.Admin, route.AuthRequired, route.ServiceAuthRequired, route.StateChanging, want.stateChanging)
	}
	if fixture.Method != route.Method || fixture.Path != route.Pattern {
		t.Fatalf("fixture route = %s %s, want %s %s", fixture.Method, fixture.Path, route.Method, route.Pattern)
	}
}

func assertProxyRoleUserExamples(t *testing.T, fixture authorizationPolicyExternalAPIFixture, want proxyRoleUserFixtureCase) {
	t.Helper()
	if want.action == "delete" {
		assertRequiredRequestFields(t, fixture, want.requiredFields)
		if len(want.requiredFields) == 0 && len(fixture.RequestExample) != 0 {
			t.Fatalf("delete request example = %v, want empty object", fixture.RequestExample)
		}
		if len(fixture.ResponseExample) != 0 {
			t.Fatalf("delete response example = %v, want empty object", fixture.ResponseExample)
		}
		return
	}
	if len(want.requiredFields) == 0 && len(fixture.RequestExample) != 0 {
		t.Fatalf("request_example = %v, want empty object", fixture.RequestExample)
	}
	assertRequiredRequestFields(t, fixture, want.requiredFields)

	row := fixture.ResponseExample
	if want.collection {
		row = firstResponseItem(t, fixture.ResponseExample)
	}
	if want.resource == "proxy_system_roles" {
		assertProxySystemRoleResponseRow(t, row)
	} else if want.resource == "proxy_policy_assignments" || want.resource == "proxy_target_assignments" {
		assertProxyPolicyAssignmentResponseRow(t, row)
	} else {
		assertProxyRoleUserResponseRow(t, row)
	}
}

func assertProxyRBACFixtureMetadata(t *testing.T, fixture authorizationPolicyExternalAPIFixture, spec platform.ServiceSpec, route platform.RouteSpec, want proxyRBACFixtureCase) {
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

func firstResponseItem(t *testing.T, response map[string]any) map[string]any {
	t.Helper()
	items, ok := response["items"].([]any)
	if !ok || len(items) == 0 {
		t.Fatalf("response_example.items = %v, want non-empty array", response["items"])
	}
	row, ok := items[0].(map[string]any)
	if !ok {
		t.Fatalf("response_example.items[0] = %v, want object", items[0])
	}
	return row
}

func assertProxyServiceReadResponseRow(t *testing.T, row map[string]any) {
	t.Helper()
	assertResponseStringFields(t, row, []string{"id", "name", "description", "category", "route_path", "created_at", "updated_at"})
	assertResponseStringArray(t, row, "api_patterns")
	assertResponseStringArray(t, row, "actions")
	if _, ok := row["sort_order"].(float64); !ok {
		t.Fatalf("response row sort_order = %v, want number", row["sort_order"])
	}
}

func assertResponseStringFields(t *testing.T, row map[string]any, fields []string) {
	t.Helper()
	for _, field := range fields {
		value, ok := row[field].(string)
		if !ok || value == "" {
			t.Fatalf("response row %s = %v, want non-empty string", field, row[field])
		}
	}
}

func assertResponseStringArray(t *testing.T, row map[string]any, field string) {
	t.Helper()
	values, ok := row[field].([]any)
	if !ok || len(values) == 0 {
		t.Fatalf("response row %s = %v, want non-empty string array", field, row[field])
	}
	for i, value := range values {
		text, ok := value.(string)
		if !ok || text == "" {
			t.Fatalf("response row %s[%d] = %v, want non-empty string", field, i, value)
		}
	}
}

func assertProxyRoleUserResponseRow(t *testing.T, row map[string]any) {
	t.Helper()
	assertResponseStringFields(t, row, []string{"id", "role_id", "user_id", "assigned_by", "created_at"})
	role, ok := row["role"].(map[string]any)
	if !ok {
		t.Fatalf("response row role = %v, want object", row["role"])
	}
	assertResponseStringFields(t, role, []string{"id", "name", "display_name", "description", "created_at", "updated_at"})
	if _, ok := role["is_system"].(bool); !ok {
		t.Fatalf("response row role.is_system = %v, want bool", role["is_system"])
	}
}

func assertProxyPolicyAssignmentResponseRow(t *testing.T, row map[string]any) {
	t.Helper()
	assertResponseStringFields(t, row, []string{"id", "policy_id", "target_type", "target_id", "created_at"})
	policy, ok := row["policy"].(map[string]any)
	if !ok {
		t.Fatalf("response row policy = %v, want object", row["policy"])
	}
	assertResponseStringFields(t, policy, []string{"id", "name", "description", "created_at", "updated_at"})
	if _, ok := policy["is_system"].(bool); !ok {
		t.Fatalf("response row policy.is_system = %v, want bool", policy["is_system"])
	}
}

func assertProxySystemRoleResponseRow(t *testing.T, row map[string]any) {
	t.Helper()
	assertResponseStringFields(t, row, []string{"id", "name"})
}

func assertProxyRBACRouteMetadata(t *testing.T, route platform.RouteSpec, fixture authorizationPolicyExternalAPIFixture, want proxyRBACFixtureCase) {
	t.Helper()
	if route.Method != want.method || route.Pattern != want.path {
		t.Fatalf("route = %s %s, want %s %s", route.Method, route.Pattern, want.method, want.path)
	}
	resource := want.resource
	if resource == "" {
		resource = "proxy_roles"
	}
	if route.Resource != resource || route.Action != want.action {
		t.Fatalf("route metadata = %#v, want %s/%s", route, resource, want.action)
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

func assertProxyRBACExamplePayloads(t *testing.T, fixture authorizationPolicyExternalAPIFixture, want proxyRBACFixtureCase) {
	t.Helper()
	if assertDeleteFixtureExamples(t, fixture, want) {
		return
	}
	assertRequiredRequestFields(t, fixture, want.requiredFields)
	assertOptionalRequestFieldForPartialFixture(t, fixture, want)
	assertResponseFields(t, fixture, want.responseFields)
	assertResponseIsNotSystemPolicy(t, fixture)
	assertProxyPolicyRuleExamples(t, fixture, want)
}

func assertDeleteFixtureExamples(t *testing.T, fixture authorizationPolicyExternalAPIFixture, want proxyRBACFixtureCase) bool {
	t.Helper()
	if want.action != "delete" {
		return false
	}
	if len(fixture.RequestExample) != 0 || len(fixture.ResponseExample) != 0 {
		t.Fatalf("delete request/response examples = %v/%v, want empty objects", fixture.RequestExample, fixture.ResponseExample)
	}
	return true
}

func assertRequiredRequestFields(t *testing.T, fixture authorizationPolicyExternalAPIFixture, fields []string) {
	t.Helper()
	for _, field := range fields {
		value, ok := fixture.RequestExample[field].(string)
		if !ok || value == "" {
			t.Fatalf("request_example.%s = %v, want non-empty", field, fixture.RequestExample[field])
		}
	}
}

func assertOptionalRequestFieldForPartialFixture(t *testing.T, fixture authorizationPolicyExternalAPIFixture, want proxyRBACFixtureCase) {
	t.Helper()
	if len(want.requiredFields) == 0 && len(want.optionalFields) > 0 {
		assertFixtureContainsDeclaredOptionalRequestField(t, fixture)
	}
}

func assertResponseFields(t *testing.T, fixture authorizationPolicyExternalAPIFixture, fields []string) {
	t.Helper()
	for _, field := range fields {
		value, ok := fixture.ResponseExample[field].(string)
		if !ok || value == "" {
			t.Fatalf("response_example.%s = %v, want non-empty", field, fixture.ResponseExample[field])
		}
	}
}

func assertResponseIsNotSystemPolicy(t *testing.T, fixture authorizationPolicyExternalAPIFixture) {
	t.Helper()
	if value, ok := fixture.ResponseExample["is_system"].(bool); !ok || value {
		t.Fatalf("response_example.is_system = %v, want false", fixture.ResponseExample["is_system"])
	}
}

func assertProxyPolicyRuleExamples(t *testing.T, fixture authorizationPolicyExternalAPIFixture, want proxyRBACFixtureCase) {
	t.Helper()
	if want.expectRules {
		assertProxyPolicyRules(t, "request_example.rules", fixture.RequestExample["rules"])
		assertProxyPolicyRules(t, "response_example.rules", fixture.ResponseExample["rules"])
	}
}

func assertFixtureContainsDeclaredOptionalRequestField(t *testing.T, fixture authorizationPolicyExternalAPIFixture) {
	t.Helper()
	for _, field := range fixture.OptionalRequestFields {
		if _, ok := fixture.RequestExample[field]; ok {
			return
		}
	}
	t.Fatalf("request_example = %v, want at least one declared optional field from %v", fixture.RequestExample, fixture.OptionalRequestFields)
}

func assertProxyPolicyRules(t *testing.T, path string, value any) {
	t.Helper()
	rules, ok := value.([]any)
	if !ok || len(rules) == 0 {
		t.Fatalf("%s = %v, want non-empty rule array", path, value)
	}
	for i, item := range rules {
		assertProxyPolicyRule(t, path, i, item)
	}
}

func assertProxyPolicyRule(t *testing.T, path string, index int, item any) {
	t.Helper()
	rule, ok := item.(map[string]any)
	if !ok {
		t.Fatalf("%s[%d] = %v, want object", path, index, item)
	}
	if proxyPolicyRuleServiceID(rule) == "" {
		t.Fatalf("%s[%d] missing service_id/serviceId", path, index)
	}
	assertProxyPolicyRuleActions(t, path, index, rule["actions"])
}

func proxyPolicyRuleServiceID(rule map[string]any) string {
	serviceID, _ := rule["service_id"].(string)
	if serviceID != "" {
		return serviceID
	}
	serviceID, _ = rule["serviceId"].(string)
	return serviceID
}

func assertProxyPolicyRuleActions(t *testing.T, path string, index int, value any) {
	t.Helper()
	actions, ok := value.([]any)
	if !ok || len(actions) == 0 {
		t.Fatalf("%s[%d].actions = %v, want non-empty array", path, index, value)
	}
	for j, action := range actions {
		assertProxyPolicyRuleAction(t, path, index, j, action)
	}
}

func assertProxyPolicyRuleAction(t *testing.T, path string, ruleIndex int, actionIndex int, value any) {
	t.Helper()
	text, ok := value.(string)
	if !ok || text == "" {
		t.Fatalf("%s[%d].actions[%d] = %v, want non-empty string", path, ruleIndex, actionIndex, value)
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
