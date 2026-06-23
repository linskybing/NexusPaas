package orgproject

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

func TestCreateGroupExternalAPIFixtureMatchesSpec(t *testing.T) {
	fixture := readOrgProjectExternalAPIFixture(t, "org-project-create-group.json")
	spec := Spec()
	route, ok := findOrgProjectExternalRoute(spec.Routes, fixture.Method, fixture.Path)
	if !ok {
		t.Fatalf("route %s %s not found in Spec()", fixture.Method, fixture.Path)
	}

	assertCreateGroupExternalAPIFixtureMetadata(t, fixture, spec, route)
	assertCreateGroupExternalAPIRouteMetadata(t, route, fixture)
}

func TestUpdateGroupExternalAPIFixtureMatchesSpec(t *testing.T) {
	fixture := readOrgProjectExternalAPIFixture(t, "org-project-update-group.json")
	spec := Spec()
	route, ok := findOrgProjectExternalRoute(spec.Routes, fixture.Method, fixture.Path)
	if !ok {
		t.Fatalf("route %s %s not found in Spec()", fixture.Method, fixture.Path)
	}

	assertUpdateGroupExternalAPIFixtureMetadata(t, fixture, spec, route)
	assertUpdateGroupExternalAPIRouteMetadata(t, route, fixture)
}

func TestDeleteGroupExternalAPIFixtureMatchesSpec(t *testing.T) {
	fixture := readOrgProjectExternalAPIFixture(t, "org-project-delete-group.json")
	spec := Spec()
	route, ok := findOrgProjectExternalRoute(spec.Routes, fixture.Method, fixture.Path)
	if !ok {
		t.Fatalf("route %s %s not found in Spec()", fixture.Method, fixture.Path)
	}

	assertDeleteGroupExternalAPIFixtureMetadata(t, fixture, spec, route)
	assertDeleteGroupExternalAPIRouteMetadata(t, route, fixture)
}

func TestBatchDeleteGroupsExternalAPIFixtureMatchesSpec(t *testing.T) {
	fixture := readOrgProjectExternalAPIFixture(t, "org-project-batch-delete-groups.json")
	spec := Spec()
	route, ok := findOrgProjectExternalRoute(spec.Routes, fixture.Method, fixture.Path)
	if !ok {
		t.Fatalf("route %s %s not found in Spec()", fixture.Method, fixture.Path)
	}

	assertBatchDeleteGroupsExternalAPIFixtureMetadata(t, fixture, spec, route)
	assertBatchDeleteGroupsExternalAPIRouteMetadata(t, route, fixture)
}

func TestCreateProjectExternalAPIFixtureMatchesSpec(t *testing.T) {
	fixture := readOrgProjectExternalAPIFixture(t, "org-project-create-project.json")
	spec := Spec()
	route, ok := findOrgProjectExternalRoute(spec.Routes, fixture.Method, fixture.Path)
	if !ok {
		t.Fatalf("route %s %s not found in Spec()", fixture.Method, fixture.Path)
	}

	assertCreateProjectExternalAPIFixtureMetadata(t, fixture, spec, route)
	assertCreateProjectExternalAPIRouteMetadata(t, route, fixture)
}

func TestUpdateProjectExternalAPIFixtureMatchesSpec(t *testing.T) {
	fixture := readOrgProjectExternalAPIFixture(t, "org-project-update-project.json")
	spec := Spec()
	route, ok := findOrgProjectExternalRoute(spec.Routes, fixture.Method, fixture.Path)
	if !ok {
		t.Fatalf("route %s %s not found in Spec()", fixture.Method, fixture.Path)
	}

	assertUpdateProjectExternalAPIFixtureMetadata(t, fixture, spec, route)
	assertUpdateProjectExternalAPIRouteMetadata(t, route, fixture)
}

func TestDeleteProjectExternalAPIFixtureMatchesSpec(t *testing.T) {
	fixture := readOrgProjectExternalAPIFixture(t, "org-project-delete-project.json")
	spec := Spec()
	route, ok := findOrgProjectExternalRoute(spec.Routes, fixture.Method, fixture.Path)
	if !ok {
		t.Fatalf("route %s %s not found in Spec()", fixture.Method, fixture.Path)
	}

	assertDeleteProjectExternalAPIFixtureMetadata(t, fixture, spec, route)
	assertDeleteProjectExternalAPIRouteMetadata(t, route, fixture)
}

func TestBatchDeleteProjectsExternalAPIFixtureMatchesSpec(t *testing.T) {
	fixture := readOrgProjectExternalAPIFixture(t, "org-project-batch-delete-projects.json")
	spec := Spec()
	route, ok := findOrgProjectExternalRoute(spec.Routes, fixture.Method, fixture.Path)
	if !ok {
		t.Fatalf("route %s %s not found in Spec()", fixture.Method, fixture.Path)
	}

	assertBatchDeleteProjectsExternalAPIFixtureMetadata(t, fixture, spec, route)
	assertBatchDeleteProjectsExternalAPIRouteMetadata(t, route, fixture)
}

func assertCreateGroupExternalAPIFixtureMetadata(t *testing.T, fixture orgProjectExternalAPIFixture, spec platform.ServiceSpec, route platform.RouteSpec) {
	t.Helper()
	if fixture.ContractName != "org-project.create_group" {
		t.Fatalf("contract_name = %q, want org-project.create_group", fixture.ContractName)
	}
	if fixture.OwnerService != spec.Name {
		t.Fatalf("owner_service = %q, want %q", fixture.OwnerService, spec.Name)
	}
	if fixture.APISurface != "external_rest" {
		t.Fatalf("api_surface = %q, want external_rest", fixture.APISurface)
	}
	if fixture.Consumer != "authenticated-user-client" {
		t.Fatalf("consumer = %q, want authenticated-user-client", fixture.Consumer)
	}
	if got, want := fixture.Resource, spec.Name+":"+route.Resource; got != want {
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
	assertCreateGroupExternalAPIRequestShape(t, fixture)
	assertCreateGroupExternalAPIStatusesAndEvents(t, fixture, spec)
	assertCreateGroupExternalAPIResponseShape(t, fixture)
}

func assertCreateGroupExternalAPIRequestShape(t *testing.T, fixture orgProjectExternalAPIFixture) {
	t.Helper()
	if !reflect.DeepEqual(fixture.RequiredRequestFields, []string{"group_name"}) {
		t.Fatalf("required_request_fields = %v, want [group_name]", fixture.RequiredRequestFields)
	}
	if !reflect.DeepEqual(fixture.OptionalRequestFields, []string{"id", "gid", "g_id", "name", "description", "storage_class", "registry_profile", "allow_run_as_root", "allowed_host_paths"}) {
		t.Fatalf("optional_request_fields = %v, want approved org-project create-group fields", fixture.OptionalRequestFields)
	}
}

func assertCreateGroupExternalAPIStatusesAndEvents(t *testing.T, fixture orgProjectExternalAPIFixture, spec platform.ServiceSpec) {
	t.Helper()
	if !reflect.DeepEqual(fixture.SuccessStatuses, []int{http.StatusCreated}) {
		t.Fatalf("success_statuses = %v, want [201]", fixture.SuccessStatuses)
	}
	if !reflect.DeepEqual(fixture.ErrorStatuses, []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusConflict, http.StatusInternalServerError}) {
		t.Fatalf("error_statuses = %v, want [400 401 403 409 500]", fixture.ErrorStatuses)
	}
	if !reflect.DeepEqual(fixture.EmitsEvents, []string{"GroupCreated"}) {
		t.Fatalf("emits_events = %v, want [GroupCreated]", fixture.EmitsEvents)
	}
	if !orgProjectServiceEmitsEvent(spec, "GroupCreated") {
		t.Fatal("Spec().Events does not include GroupCreated")
	}
}

func assertCreateGroupExternalAPIResponseShape(t *testing.T, fixture orgProjectExternalAPIFixture) {
	t.Helper()
	if _, ok := fixture.ResponseExample["data"]; ok {
		t.Fatal("response_example contains record envelope data field, want createGroup record.Data shape")
	}
	if _, ok := fixture.ResponseExample["version"]; ok {
		t.Fatal("response_example contains record envelope version field, want createGroup record.Data shape")
	}
	if got, want := fixture.ResponseExample["group_name"], "ga-research"; got != want {
		t.Fatalf("response_example.group_name = %v, want %q", got, want)
	}
}

func assertCreateGroupExternalAPIRouteMetadata(t *testing.T, route platform.RouteSpec, fixture orgProjectExternalAPIFixture) {
	t.Helper()
	if got, want := route.Resource, "groups"; got != want {
		t.Fatalf("route resource = %q, want %q", got, want)
	}
	if got, want := route.Action, "create"; got != want {
		t.Fatalf("route action = %q, want %q", got, want)
	}
	if route.Method != http.MethodPost || route.Pattern != "/api/v1/groups" {
		t.Fatalf("route = %s %s, want POST /api/v1/groups", route.Method, route.Pattern)
	}
	if !route.AuthRequired {
		t.Fatal("route AuthRequired = false, want true")
	}
	if route.IDParam != "" {
		t.Fatalf("route IDParam = %q, want none", route.IDParam)
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
	if fixture.Method != route.Method || fixture.Path != route.Pattern {
		t.Fatalf("fixture route = %s %s, want %s %s", fixture.Method, fixture.Path, route.Method, route.Pattern)
	}
}

func assertUpdateGroupExternalAPIFixtureMetadata(t *testing.T, fixture orgProjectExternalAPIFixture, spec platform.ServiceSpec, route platform.RouteSpec) {
	t.Helper()
	if fixture.ContractName != "org-project.update_group" {
		t.Fatalf("contract_name = %q, want org-project.update_group", fixture.ContractName)
	}
	if fixture.OwnerService != spec.Name {
		t.Fatalf("owner_service = %q, want %q", fixture.OwnerService, spec.Name)
	}
	if fixture.APISurface != "external_rest" {
		t.Fatalf("api_surface = %q, want external_rest", fixture.APISurface)
	}
	if fixture.Consumer != "authenticated-user-client" {
		t.Fatalf("consumer = %q, want authenticated-user-client", fixture.Consumer)
	}
	if got, want := fixture.Resource, spec.Name+":"+route.Resource; got != want {
		t.Fatalf("resource = %q, want %q", got, want)
	}
	if fixture.Action != route.Action {
		t.Fatalf("action = %q, want %q", fixture.Action, route.Action)
	}
	if fixture.Auth != "user" || fixture.AuthRequired != route.AuthRequired || fixture.ServiceKeyRequired != route.ServiceAuthRequired {
		t.Fatalf("auth metadata = %q/%v/%v, want user/%v/%v", fixture.Auth, fixture.AuthRequired, fixture.ServiceKeyRequired, route.AuthRequired, route.ServiceAuthRequired)
	}
	if !reflect.DeepEqual(fixture.PathParameters, []string{"id"}) {
		t.Fatalf("fixture path_parameters = %v, want [id]", fixture.PathParameters)
	}
	assertUpdateGroupExternalAPIRequestShape(t, fixture)
	assertUpdateGroupExternalAPIStatusesAndEvents(t, fixture, spec)
	assertUpdateGroupExternalAPIResponseShape(t, fixture)
	assertOrgProjectExternalAPICompatibility(t, fixture)
}

func assertUpdateGroupExternalAPIRequestShape(t *testing.T, fixture orgProjectExternalAPIFixture) {
	t.Helper()
	if !reflect.DeepEqual(fixture.RequiredRequestFields, []string{"group_name"}) {
		t.Fatalf("required_request_fields = %v, want [group_name]", fixture.RequiredRequestFields)
	}
	if !reflect.DeepEqual(fixture.OptionalRequestFields, []string{"name", "description", "storage_class", "storageClass", "registry_profile", "registryProfile", "allow_run_as_root", "allowed_host_paths"}) {
		t.Fatalf("optional_request_fields = %v, want approved org-project update-group fields", fixture.OptionalRequestFields)
	}
	assertOrgProjectExampleText(t, fixture.RequestExample, "group_name", "ga-research-updated")
	assertOrgProjectExampleText(t, fixture.RequestExample, "description", "Updated synthetic first-release group")
	assertOrgProjectExampleText(t, fixture.RequestExample, "storage_class", "fast")
	assertOrgProjectExampleText(t, fixture.RequestExample, "registry_profile", "gpu")
	assertOrgProjectExampleBool(t, fixture.RequestExample, "allow_run_as_root", false)
	assertOrgProjectExampleArray(t, fixture.RequestExample, "allowed_host_paths", []any{"/mnt/shared/datasets"})
}

func assertUpdateGroupExternalAPIStatusesAndEvents(t *testing.T, fixture orgProjectExternalAPIFixture, spec platform.ServiceSpec) {
	t.Helper()
	if !reflect.DeepEqual(fixture.SuccessStatuses, []int{http.StatusOK}) {
		t.Fatalf("success_statuses = %v, want [200]", fixture.SuccessStatuses)
	}
	if !reflect.DeepEqual(fixture.ErrorStatuses, []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusInternalServerError}) {
		t.Fatalf("error_statuses = %v, want [400 401 403 404 500]", fixture.ErrorStatuses)
	}
	if !reflect.DeepEqual(fixture.EmitsEvents, []string{"GroupUpdated"}) {
		t.Fatalf("emits_events = %v, want [GroupUpdated]", fixture.EmitsEvents)
	}
	if !orgProjectServiceEmitsEvent(spec, "GroupUpdated") {
		t.Fatal("Spec().Events does not include GroupUpdated")
	}
}

func assertUpdateGroupExternalAPIResponseShape(t *testing.T, fixture orgProjectExternalAPIFixture) {
	t.Helper()
	if _, ok := fixture.ResponseExample["data"]; ok {
		t.Fatal("response_example contains record envelope data field, want updateGroup record.Data shape")
	}
	if _, ok := fixture.ResponseExample["version"]; ok {
		t.Fatal("response_example contains record envelope version field, want updateGroup record.Data shape")
	}
	assertOrgProjectExampleText(t, fixture.ResponseExample, "id", "group-ga-001")
	assertOrgProjectExampleText(t, fixture.ResponseExample, "group_name", "ga-research-updated")
	assertOrgProjectExampleText(t, fixture.ResponseExample, "name", "ga-research-updated")
	assertOrgProjectExampleText(t, fixture.ResponseExample, "description", "Updated synthetic first-release group")
	assertOrgProjectExampleText(t, fixture.ResponseExample, "storage_class", "fast")
	assertOrgProjectExampleText(t, fixture.ResponseExample, "registry_profile", "gpu")
	assertOrgProjectExampleBool(t, fixture.ResponseExample, "allow_run_as_root", false)
	assertOrgProjectExampleArray(t, fixture.ResponseExample, "allowed_host_paths", []any{"/mnt/shared/datasets"})
}

func assertUpdateGroupExternalAPIRouteMetadata(t *testing.T, route platform.RouteSpec, fixture orgProjectExternalAPIFixture) {
	t.Helper()
	if got, want := route.Resource, "groups"; got != want {
		t.Fatalf("route resource = %q, want %q", got, want)
	}
	if got, want := route.Action, "update"; got != want {
		t.Fatalf("route action = %q, want %q", got, want)
	}
	if route.Method != http.MethodPut || route.Pattern != "/api/v1/groups/{id}" {
		t.Fatalf("route = %s %s, want PUT /api/v1/groups/{id}", route.Method, route.Pattern)
	}
	if !route.AuthRequired {
		t.Fatal("route AuthRequired = false, want true")
	}
	if route.IDParam != "id" {
		t.Fatalf("route IDParam = %q, want id", route.IDParam)
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
	if fixture.Method != route.Method || fixture.Path != route.Pattern {
		t.Fatalf("fixture route = %s %s, want %s %s", fixture.Method, fixture.Path, route.Method, route.Pattern)
	}
}

func assertDeleteGroupExternalAPIFixtureMetadata(t *testing.T, fixture orgProjectExternalAPIFixture, spec platform.ServiceSpec, route platform.RouteSpec) {
	t.Helper()
	if fixture.ContractName != "org-project.delete_group" {
		t.Fatalf("contract_name = %q, want org-project.delete_group", fixture.ContractName)
	}
	if fixture.OwnerService != spec.Name {
		t.Fatalf("owner_service = %q, want %q", fixture.OwnerService, spec.Name)
	}
	if fixture.APISurface != "external_rest" {
		t.Fatalf("api_surface = %q, want external_rest", fixture.APISurface)
	}
	if fixture.Consumer != "authenticated-user-client" {
		t.Fatalf("consumer = %q, want authenticated-user-client", fixture.Consumer)
	}
	if got, want := fixture.Resource, spec.Name+":"+route.Resource; got != want {
		t.Fatalf("resource = %q, want %q", got, want)
	}
	if fixture.Action != route.Action {
		t.Fatalf("action = %q, want %q", fixture.Action, route.Action)
	}
	if fixture.Auth != "user" || fixture.AuthRequired != route.AuthRequired || fixture.ServiceKeyRequired != route.ServiceAuthRequired {
		t.Fatalf("auth metadata = %q/%v/%v, want user/%v/%v", fixture.Auth, fixture.AuthRequired, fixture.ServiceKeyRequired, route.AuthRequired, route.ServiceAuthRequired)
	}
	if !reflect.DeepEqual(fixture.PathParameters, []string{"id"}) {
		t.Fatalf("fixture path_parameters = %v, want [id]", fixture.PathParameters)
	}
	assertDeleteGroupExternalAPIRequestShape(t, fixture)
	assertDeleteGroupExternalAPIStatusesAndEvents(t, fixture, spec)
	assertDeleteGroupExternalAPIResponseShape(t, fixture)
	assertOrgProjectExternalAPICompatibility(t, fixture)
}

func assertDeleteGroupExternalAPIRequestShape(t *testing.T, fixture orgProjectExternalAPIFixture) {
	t.Helper()
	if got := len(fixture.RequiredRequestFields); got != 0 {
		t.Fatalf("required_request_fields count = %d (%v), want none", got, fixture.RequiredRequestFields)
	}
	if got := len(fixture.OptionalRequestFields); got != 0 {
		t.Fatalf("optional_request_fields count = %d (%v), want none", got, fixture.OptionalRequestFields)
	}
	if got := len(fixture.RequestExample); got != 0 {
		t.Fatalf("request_example field count = %d (%v), want empty deleteGroup request", got, fixture.RequestExample)
	}
}

func assertDeleteGroupExternalAPIStatusesAndEvents(t *testing.T, fixture orgProjectExternalAPIFixture, spec platform.ServiceSpec) {
	t.Helper()
	if !reflect.DeepEqual(fixture.SuccessStatuses, []int{http.StatusOK}) {
		t.Fatalf("success_statuses = %v, want [200]", fixture.SuccessStatuses)
	}
	if !reflect.DeepEqual(fixture.ErrorStatuses, []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusInternalServerError}) {
		t.Fatalf("error_statuses = %v, want [401 403 404 500]", fixture.ErrorStatuses)
	}
	if !reflect.DeepEqual(fixture.EmitsEvents, []string{"GroupDeleted"}) {
		t.Fatalf("emits_events = %v, want [GroupDeleted]", fixture.EmitsEvents)
	}
	if !orgProjectServiceEmitsEvent(spec, "GroupDeleted") {
		t.Fatal("Spec().Events does not include GroupDeleted")
	}
}

func assertDeleteGroupExternalAPIResponseShape(t *testing.T, fixture orgProjectExternalAPIFixture) {
	t.Helper()
	if len(fixture.ResponseExample) != 0 {
		t.Fatalf("response_example = %v, want empty deleteGroup response", fixture.ResponseExample)
	}
}

func assertDeleteGroupExternalAPIRouteMetadata(t *testing.T, route platform.RouteSpec, fixture orgProjectExternalAPIFixture) {
	t.Helper()
	if got, want := route.Resource, "groups"; got != want {
		t.Fatalf("route resource = %q, want %q", got, want)
	}
	if got, want := route.Action, "delete"; got != want {
		t.Fatalf("route action = %q, want %q", got, want)
	}
	if route.Method != http.MethodDelete || route.Pattern != "/api/v1/groups/{id}" {
		t.Fatalf("route = %s %s, want DELETE /api/v1/groups/{id}", route.Method, route.Pattern)
	}
	if !route.AuthRequired {
		t.Fatal("route AuthRequired = false, want true")
	}
	if route.IDParam != "id" {
		t.Fatalf("route IDParam = %q, want id", route.IDParam)
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
	if fixture.Method != route.Method || fixture.Path != route.Pattern {
		t.Fatalf("fixture route = %s %s, want %s %s", fixture.Method, fixture.Path, route.Method, route.Pattern)
	}
}

func assertBatchDeleteGroupsExternalAPIFixtureMetadata(t *testing.T, fixture orgProjectExternalAPIFixture, spec platform.ServiceSpec, route platform.RouteSpec) {
	t.Helper()
	if fixture.ContractName != "org-project.batch_delete_groups" {
		t.Fatalf("contract_name = %q, want org-project.batch_delete_groups", fixture.ContractName)
	}
	if fixture.OwnerService != spec.Name {
		t.Fatalf("owner_service = %q, want %q", fixture.OwnerService, spec.Name)
	}
	if fixture.APISurface != "external_rest" {
		t.Fatalf("api_surface = %q, want external_rest", fixture.APISurface)
	}
	if fixture.Consumer != "authenticated-user-client" {
		t.Fatalf("consumer = %q, want authenticated-user-client", fixture.Consumer)
	}
	if got, want := fixture.Resource, spec.Name+":"+route.Resource; got != want {
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
	assertBatchDeleteGroupsExternalAPIRequestShape(t, fixture)
	assertBatchDeleteGroupsExternalAPIStatusesAndEvents(t, fixture, spec)
	assertOrgProjectBatchDeleteExternalAPIResponseShape(t, fixture)
	assertOrgProjectExternalAPICompatibility(t, fixture)
}

func assertBatchDeleteGroupsExternalAPIRequestShape(t *testing.T, fixture orgProjectExternalAPIFixture) {
	t.Helper()
	if !reflect.DeepEqual(fixture.RequiredRequestFields, []string{"ids"}) {
		t.Fatalf("required_request_fields = %v, want [ids]", fixture.RequiredRequestFields)
	}
	if len(fixture.OptionalRequestFields) != 0 {
		t.Fatalf("optional_request_fields = %v, want none", fixture.OptionalRequestFields)
	}
	ids, ok := fixture.RequestExample["ids"].([]any)
	if !ok {
		t.Fatalf("request_example.ids = %T, want array", fixture.RequestExample["ids"])
	}
	if len(ids) != 2 || ids[0] != "group-ga-001" || ids[1] != "group-ga-002" {
		t.Fatalf("request_example.ids = %v, want [group-ga-001 group-ga-002]", ids)
	}
}

func assertBatchDeleteGroupsExternalAPIStatusesAndEvents(t *testing.T, fixture orgProjectExternalAPIFixture, spec platform.ServiceSpec) {
	t.Helper()
	if !reflect.DeepEqual(fixture.SuccessStatuses, []int{http.StatusOK}) {
		t.Fatalf("success_statuses = %v, want [200]", fixture.SuccessStatuses)
	}
	if !reflect.DeepEqual(fixture.ErrorStatuses, []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusInternalServerError}) {
		t.Fatalf("error_statuses = %v, want [400 401 403 500]", fixture.ErrorStatuses)
	}
	if !reflect.DeepEqual(fixture.EmitsEvents, []string{"GroupDeleted"}) {
		t.Fatalf("emits_events = %v, want [GroupDeleted]", fixture.EmitsEvents)
	}
	if !orgProjectServiceEmitsEvent(spec, "GroupDeleted") {
		t.Fatal("Spec().Events does not include GroupDeleted")
	}
}

func assertBatchDeleteGroupsExternalAPIRouteMetadata(t *testing.T, route platform.RouteSpec, fixture orgProjectExternalAPIFixture) {
	t.Helper()
	if got, want := route.Resource, "groups"; got != want {
		t.Fatalf("route resource = %q, want %q", got, want)
	}
	if got, want := route.Action, "batch_delete"; got != want {
		t.Fatalf("route action = %q, want %q", got, want)
	}
	if route.Method != http.MethodDelete || route.Pattern != "/api/v1/groups/batch" {
		t.Fatalf("route = %s %s, want DELETE /api/v1/groups/batch", route.Method, route.Pattern)
	}
	if !route.AuthRequired {
		t.Fatal("route AuthRequired = false, want true")
	}
	if route.IDParam != "" {
		t.Fatalf("route IDParam = %q, want none", route.IDParam)
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
	if fixture.Method != route.Method || fixture.Path != route.Pattern {
		t.Fatalf("fixture route = %s %s, want %s %s", fixture.Method, fixture.Path, route.Method, route.Pattern)
	}
}

func assertCreateProjectExternalAPIFixtureMetadata(t *testing.T, fixture orgProjectExternalAPIFixture, spec platform.ServiceSpec, route platform.RouteSpec) {
	t.Helper()
	if fixture.ContractName != "org-project.create_project" {
		t.Fatalf("contract_name = %q, want org-project.create_project", fixture.ContractName)
	}
	if fixture.OwnerService != spec.Name {
		t.Fatalf("owner_service = %q, want %q", fixture.OwnerService, spec.Name)
	}
	if fixture.APISurface != "external_rest" {
		t.Fatalf("api_surface = %q, want external_rest", fixture.APISurface)
	}
	if fixture.Consumer != "authenticated-user-client" {
		t.Fatalf("consumer = %q, want authenticated-user-client", fixture.Consumer)
	}
	if got, want := fixture.Resource, spec.Name+":"+route.Resource; got != want {
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
	assertCreateProjectExternalAPIRequestShape(t, fixture)
	assertCreateProjectExternalAPIStatusesAndEvents(t, fixture, spec)
	assertCreateProjectExternalAPIResponseShape(t, fixture)
}

func assertCreateProjectExternalAPIRequestShape(t *testing.T, fixture orgProjectExternalAPIFixture) {
	t.Helper()
	if !reflect.DeepEqual(fixture.RequiredRequestFields, []string{"project_name", "g_id"}) {
		t.Fatalf("required_request_fields = %v, want [project_name g_id]", fixture.RequiredRequestFields)
	}
	if !reflect.DeepEqual(fixture.OptionalRequestFields, []string{"id", "p_id", "name", "group_id", "owner_id", "description", "path", "plan_id", "personal_user_id", "max_concurrent_jobs_per_user", "max_queued_jobs_per_user", "max_job_runtime_seconds", "max_ide_runtime_seconds", "max_project_users", "allow_image_build", "allow_node_port", "allow_run_as_root", "external_network_enabled"}) {
		t.Fatalf("optional_request_fields = %v, want approved org-project create-project fields", fixture.OptionalRequestFields)
	}
}

func assertCreateProjectExternalAPIStatusesAndEvents(t *testing.T, fixture orgProjectExternalAPIFixture, spec platform.ServiceSpec) {
	t.Helper()
	if !reflect.DeepEqual(fixture.SuccessStatuses, []int{http.StatusCreated}) {
		t.Fatalf("success_statuses = %v, want [201]", fixture.SuccessStatuses)
	}
	// createProject enforces admin-panel permission in the handler through 403.
	// RouteSpec Admin remains false.
	if !reflect.DeepEqual(fixture.ErrorStatuses, []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusConflict, http.StatusInternalServerError}) {
		t.Fatalf("error_statuses = %v, want [400 401 403 404 409 500]", fixture.ErrorStatuses)
	}
	if !reflect.DeepEqual(fixture.EmitsEvents, []string{"ProjectCreated"}) {
		t.Fatalf("emits_events = %v, want [ProjectCreated]", fixture.EmitsEvents)
	}
	if !orgProjectServiceEmitsEvent(spec, "ProjectCreated") {
		t.Fatal("Spec().Events does not include ProjectCreated")
	}
}

func assertCreateProjectExternalAPIResponseShape(t *testing.T, fixture orgProjectExternalAPIFixture) {
	t.Helper()
	if _, ok := fixture.ResponseExample["data"]; ok {
		t.Fatal("response_example contains record envelope data field, want createProject record.Data shape")
	}
	if got, want := fixture.ResponseExample["project_id"], "project-ga-001"; got != want {
		t.Fatalf("response_example.project_id = %v, want %q", got, want)
	}
}

func assertCreateProjectExternalAPIRouteMetadata(t *testing.T, route platform.RouteSpec, fixture orgProjectExternalAPIFixture) {
	t.Helper()
	if got, want := route.Resource, "projects"; got != want {
		t.Fatalf("route resource = %q, want %q", got, want)
	}
	if got, want := route.Action, "create"; got != want {
		t.Fatalf("route action = %q, want %q", got, want)
	}
	if route.Method != http.MethodPost || route.Pattern != "/api/v1/projects" {
		t.Fatalf("route = %s %s, want POST /api/v1/projects", route.Method, route.Pattern)
	}
	if !route.AuthRequired {
		t.Fatal("route AuthRequired = false, want true")
	}
	if route.IDParam != "" {
		t.Fatalf("route IDParam = %q, want none", route.IDParam)
	}
	if route.Admin {
		t.Fatal("route Admin = true, want false; createProject enforces admin-panel permission in the handler")
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

func assertUpdateProjectExternalAPIFixtureMetadata(t *testing.T, fixture orgProjectExternalAPIFixture, spec platform.ServiceSpec, route platform.RouteSpec) {
	t.Helper()
	if fixture.ContractName != "org-project.update_project" {
		t.Fatalf("contract_name = %q, want org-project.update_project", fixture.ContractName)
	}
	if fixture.OwnerService != spec.Name {
		t.Fatalf("owner_service = %q, want %q", fixture.OwnerService, spec.Name)
	}
	if fixture.APISurface != "external_rest" {
		t.Fatalf("api_surface = %q, want external_rest", fixture.APISurface)
	}
	if fixture.Consumer != "authenticated-user-client" {
		t.Fatalf("consumer = %q, want authenticated-user-client", fixture.Consumer)
	}
	if got, want := fixture.Resource, spec.Name+":"+route.Resource; got != want {
		t.Fatalf("resource = %q, want %q", got, want)
	}
	if fixture.Action != route.Action {
		t.Fatalf("action = %q, want %q", fixture.Action, route.Action)
	}
	if fixture.Auth != "user" || fixture.AuthRequired != route.AuthRequired || fixture.ServiceKeyRequired != route.ServiceAuthRequired {
		t.Fatalf("auth metadata = %q/%v/%v, want user/%v/%v", fixture.Auth, fixture.AuthRequired, fixture.ServiceKeyRequired, route.AuthRequired, route.ServiceAuthRequired)
	}
	if !reflect.DeepEqual(fixture.PathParameters, []string{"id"}) {
		t.Fatalf("fixture path_parameters = %v, want [id]", fixture.PathParameters)
	}
	assertUpdateProjectExternalAPIRequestShape(t, fixture)
	assertUpdateProjectExternalAPIStatusesAndEvents(t, fixture, spec)
	assertUpdateProjectExternalAPIResponseShape(t, fixture)
	assertOrgProjectExternalAPICompatibility(t, fixture)
}

func assertUpdateProjectExternalAPIRequestShape(t *testing.T, fixture orgProjectExternalAPIFixture) {
	t.Helper()
	if !reflect.DeepEqual(fixture.RequiredRequestFields, []string{"project_name"}) {
		t.Fatalf("required_request_fields = %v, want [project_name]", fixture.RequiredRequestFields)
	}
	if !reflect.DeepEqual(fixture.OptionalRequestFields, []string{"g_id", "group_id", "owner_id", "description", "plan_id", "personal_user_id", "max_concurrent_jobs_per_user", "max_queued_jobs_per_user", "max_job_runtime_seconds", "max_ide_runtime_seconds", "max_project_users", "allow_image_build", "allow_node_port", "allow_run_as_root", "external_network_enabled"}) {
		t.Fatalf("optional_request_fields = %v, want approved org-project update-project fields", fixture.OptionalRequestFields)
	}
	assertOrgProjectExampleText(t, fixture.RequestExample, "project_name", "ga-training-updated")
	assertOrgProjectExampleText(t, fixture.RequestExample, "description", "Updated synthetic first-release project")
	assertOrgProjectExampleText(t, fixture.RequestExample, "g_id", "group-ga-001")
	assertOrgProjectExampleText(t, fixture.RequestExample, "plan_id", "plan-ga-002")
	assertOrgProjectExampleText(t, fixture.RequestExample, "personal_user_id", "user-ga-owner")
	assertOrgProjectExampleNumber(t, fixture.RequestExample, "max_concurrent_jobs_per_user", 3)
	assertOrgProjectExampleNumber(t, fixture.RequestExample, "max_queued_jobs_per_user", 10)
	assertOrgProjectExampleNumber(t, fixture.RequestExample, "max_job_runtime_seconds", 28800)
	assertOrgProjectExampleNumber(t, fixture.RequestExample, "max_ide_runtime_seconds", 18000)
	assertOrgProjectExampleNumber(t, fixture.RequestExample, "max_project_users", 30)
	assertOrgProjectExampleBool(t, fixture.RequestExample, "allow_image_build", true)
	assertOrgProjectExampleBool(t, fixture.RequestExample, "allow_node_port", false)
	assertOrgProjectExampleBool(t, fixture.RequestExample, "allow_run_as_root", false)
	assertOrgProjectExampleBool(t, fixture.RequestExample, "external_network_enabled", true)
}

func assertUpdateProjectExternalAPIStatusesAndEvents(t *testing.T, fixture orgProjectExternalAPIFixture, spec platform.ServiceSpec) {
	t.Helper()
	if !reflect.DeepEqual(fixture.SuccessStatuses, []int{http.StatusOK}) {
		t.Fatalf("success_statuses = %v, want [200]", fixture.SuccessStatuses)
	}
	if !reflect.DeepEqual(fixture.ErrorStatuses, []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusInternalServerError}) {
		t.Fatalf("error_statuses = %v, want [400 401 403 404 500]", fixture.ErrorStatuses)
	}
	if !reflect.DeepEqual(fixture.EmitsEvents, []string{"ProjectUpdated"}) {
		t.Fatalf("emits_events = %v, want [ProjectUpdated]", fixture.EmitsEvents)
	}
	if !orgProjectServiceEmitsEvent(spec, "ProjectUpdated") {
		t.Fatal("Spec().Events does not include ProjectUpdated")
	}
}

func assertUpdateProjectExternalAPIResponseShape(t *testing.T, fixture orgProjectExternalAPIFixture) {
	t.Helper()
	if _, ok := fixture.ResponseExample["data"]; ok {
		t.Fatal("response_example contains record envelope data field, want updateProject record.Data shape")
	}
	if _, ok := fixture.ResponseExample["version"]; ok {
		t.Fatal("response_example contains record envelope version field, want updateProject record.Data shape")
	}
	assertOrgProjectExampleText(t, fixture.ResponseExample, "project_id", "project-ga-001")
	assertOrgProjectExampleText(t, fixture.ResponseExample, "project_name", "ga-training-updated")
	assertOrgProjectExampleText(t, fixture.ResponseExample, "owner_id", "group-ga-001")
	assertOrgProjectExampleNumber(t, fixture.ResponseExample, "max_project_users", 30)
	assertOrgProjectExampleBool(t, fixture.ResponseExample, "external_network_enabled", true)
}

func assertUpdateProjectExternalAPIRouteMetadata(t *testing.T, route platform.RouteSpec, fixture orgProjectExternalAPIFixture) {
	t.Helper()
	if got, want := route.Resource, "projects"; got != want {
		t.Fatalf("route resource = %q, want %q", got, want)
	}
	if got, want := route.Action, "update"; got != want {
		t.Fatalf("route action = %q, want %q", got, want)
	}
	if route.Method != http.MethodPut || route.Pattern != "/api/v1/projects/{id}" {
		t.Fatalf("route = %s %s, want PUT /api/v1/projects/{id}", route.Method, route.Pattern)
	}
	if !route.AuthRequired {
		t.Fatal("route AuthRequired = false, want true")
	}
	if route.IDParam != "id" {
		t.Fatalf("route IDParam = %q, want id", route.IDParam)
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
	if fixture.Method != route.Method || fixture.Path != route.Pattern {
		t.Fatalf("fixture route = %s %s, want %s %s", fixture.Method, fixture.Path, route.Method, route.Pattern)
	}
}

func assertDeleteProjectExternalAPIFixtureMetadata(t *testing.T, fixture orgProjectExternalAPIFixture, spec platform.ServiceSpec, route platform.RouteSpec) {
	t.Helper()
	if fixture.ContractName != "org-project.delete_project" {
		t.Fatalf("contract_name = %q, want org-project.delete_project", fixture.ContractName)
	}
	if fixture.OwnerService != spec.Name {
		t.Fatalf("owner_service = %q, want %q", fixture.OwnerService, spec.Name)
	}
	if fixture.APISurface != "external_rest" {
		t.Fatalf("api_surface = %q, want external_rest", fixture.APISurface)
	}
	if fixture.Consumer != "authenticated-user-client" {
		t.Fatalf("consumer = %q, want authenticated-user-client", fixture.Consumer)
	}
	if got, want := fixture.Resource, spec.Name+":"+route.Resource; got != want {
		t.Fatalf("resource = %q, want %q", got, want)
	}
	if fixture.Action != route.Action {
		t.Fatalf("action = %q, want %q", fixture.Action, route.Action)
	}
	if fixture.Auth != "user" || fixture.AuthRequired != route.AuthRequired || fixture.ServiceKeyRequired != route.ServiceAuthRequired {
		t.Fatalf("auth metadata = %q/%v/%v, want user/%v/%v", fixture.Auth, fixture.AuthRequired, fixture.ServiceKeyRequired, route.AuthRequired, route.ServiceAuthRequired)
	}
	if !reflect.DeepEqual(fixture.PathParameters, []string{"id"}) {
		t.Fatalf("fixture path_parameters = %v, want [id]", fixture.PathParameters)
	}
	assertDeleteProjectExternalAPIRequestShape(t, fixture)
	assertDeleteProjectExternalAPIStatusesAndEvents(t, fixture, spec)
	assertDeleteProjectExternalAPIResponseShape(t, fixture)
	assertOrgProjectExternalAPICompatibility(t, fixture)
}

func assertDeleteProjectExternalAPIRequestShape(t *testing.T, fixture orgProjectExternalAPIFixture) {
	t.Helper()
	if len(fixture.RequiredRequestFields) != 0 {
		t.Fatalf("required_request_fields = %v, want none", fixture.RequiredRequestFields)
	}
	if len(fixture.OptionalRequestFields) != 0 {
		t.Fatalf("optional_request_fields = %v, want none", fixture.OptionalRequestFields)
	}
	if len(fixture.RequestExample) != 0 {
		t.Fatalf("request_example = %v, want empty no-body DELETE request", fixture.RequestExample)
	}
}

func assertDeleteProjectExternalAPIStatusesAndEvents(t *testing.T, fixture orgProjectExternalAPIFixture, spec platform.ServiceSpec) {
	t.Helper()
	if !reflect.DeepEqual(fixture.SuccessStatuses, []int{http.StatusOK}) {
		t.Fatalf("success_statuses = %v, want [200]", fixture.SuccessStatuses)
	}
	if !reflect.DeepEqual(fixture.ErrorStatuses, []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusInternalServerError}) {
		t.Fatalf("error_statuses = %v, want [401 403 404 500]", fixture.ErrorStatuses)
	}
	if !reflect.DeepEqual(fixture.EmitsEvents, []string{"ProjectDeleted"}) {
		t.Fatalf("emits_events = %v, want [ProjectDeleted]", fixture.EmitsEvents)
	}
	if !orgProjectServiceEmitsEvent(spec, "ProjectDeleted") {
		t.Fatal("Spec().Events does not include ProjectDeleted")
	}
}

func assertDeleteProjectExternalAPIResponseShape(t *testing.T, fixture orgProjectExternalAPIFixture) {
	t.Helper()
	if len(fixture.ResponseExample) != 0 {
		t.Fatalf("response_example = %v, want empty deleteProject response", fixture.ResponseExample)
	}
}

func assertDeleteProjectExternalAPIRouteMetadata(t *testing.T, route platform.RouteSpec, fixture orgProjectExternalAPIFixture) {
	t.Helper()
	if got, want := route.Resource, "projects"; got != want {
		t.Fatalf("route resource = %q, want %q", got, want)
	}
	if got, want := route.Action, "delete"; got != want {
		t.Fatalf("route action = %q, want %q", got, want)
	}
	if route.Method != http.MethodDelete || route.Pattern != "/api/v1/projects/{id}" {
		t.Fatalf("route = %s %s, want DELETE /api/v1/projects/{id}", route.Method, route.Pattern)
	}
	if !route.AuthRequired {
		t.Fatal("route AuthRequired = false, want true")
	}
	if route.IDParam != "id" {
		t.Fatalf("route IDParam = %q, want id", route.IDParam)
	}
	if route.Admin {
		t.Fatal("route Admin = true, want false; deleteProject enforces admin-panel permission in the handler")
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

func assertBatchDeleteProjectsExternalAPIFixtureMetadata(t *testing.T, fixture orgProjectExternalAPIFixture, spec platform.ServiceSpec, route platform.RouteSpec) {
	t.Helper()
	if fixture.ContractName != "org-project.batch_delete_projects" {
		t.Fatalf("contract_name = %q, want org-project.batch_delete_projects", fixture.ContractName)
	}
	if fixture.OwnerService != spec.Name {
		t.Fatalf("owner_service = %q, want %q", fixture.OwnerService, spec.Name)
	}
	if fixture.APISurface != "external_rest" {
		t.Fatalf("api_surface = %q, want external_rest", fixture.APISurface)
	}
	if fixture.Consumer != "authenticated-user-client" {
		t.Fatalf("consumer = %q, want authenticated-user-client", fixture.Consumer)
	}
	if got, want := fixture.Resource, spec.Name+":"+route.Resource; got != want {
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
	assertBatchDeleteProjectsExternalAPIRequestShape(t, fixture)
	assertBatchDeleteProjectsExternalAPIStatusesAndEvents(t, fixture, spec)
	assertOrgProjectBatchDeleteExternalAPIResponseShape(t, fixture)
	assertOrgProjectExternalAPICompatibility(t, fixture)
}

func assertBatchDeleteProjectsExternalAPIRequestShape(t *testing.T, fixture orgProjectExternalAPIFixture) {
	t.Helper()
	if !reflect.DeepEqual(fixture.RequiredRequestFields, []string{"ids"}) {
		t.Fatalf("required_request_fields = %v, want [ids]", fixture.RequiredRequestFields)
	}
	if len(fixture.OptionalRequestFields) != 0 {
		t.Fatalf("optional_request_fields = %v, want none", fixture.OptionalRequestFields)
	}
	ids, ok := fixture.RequestExample["ids"].([]any)
	if !ok {
		t.Fatalf("request_example.ids = %T, want array", fixture.RequestExample["ids"])
	}
	if len(ids) != 2 || ids[0] != "project-ga-001" || ids[1] != "project-ga-002" {
		t.Fatalf("request_example.ids = %v, want [project-ga-001 project-ga-002]", ids)
	}
}

func assertBatchDeleteProjectsExternalAPIStatusesAndEvents(t *testing.T, fixture orgProjectExternalAPIFixture, spec platform.ServiceSpec) {
	t.Helper()
	if !reflect.DeepEqual(fixture.SuccessStatuses, []int{http.StatusOK}) {
		t.Fatalf("success_statuses = %v, want [200]", fixture.SuccessStatuses)
	}
	if !reflect.DeepEqual(fixture.ErrorStatuses, []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusInternalServerError}) {
		t.Fatalf("error_statuses = %v, want [400 401 403 500]", fixture.ErrorStatuses)
	}
	if !reflect.DeepEqual(fixture.EmitsEvents, []string{"ProjectDeleted"}) {
		t.Fatalf("emits_events = %v, want [ProjectDeleted]", fixture.EmitsEvents)
	}
	if !orgProjectServiceEmitsEvent(spec, "ProjectDeleted") {
		t.Fatal("Spec().Events does not include ProjectDeleted")
	}
}

func assertOrgProjectBatchDeleteExternalAPIResponseShape(t *testing.T, fixture orgProjectExternalAPIFixture) {
	t.Helper()
	if _, ok := fixture.ResponseExample["data"]; ok {
		t.Fatal("response_example contains record envelope data field, want direct batchResult shape")
	}
	if _, ok := fixture.ResponseExample["version"]; ok {
		t.Fatal("response_example contains record envelope version field, want direct batchResult shape")
	}
	assertOrgProjectExampleNumber(t, fixture.ResponseExample, "succeeded", 2)
	assertOrgProjectExampleNumber(t, fixture.ResponseExample, "failed", 0)
	errors, ok := fixture.ResponseExample["errors"].([]any)
	if !ok {
		t.Fatalf("response_example.errors = %T, want array", fixture.ResponseExample["errors"])
	}
	if len(errors) != 0 {
		t.Fatalf("response_example.errors = %v, want empty array", errors)
	}
}

func assertBatchDeleteProjectsExternalAPIRouteMetadata(t *testing.T, route platform.RouteSpec, fixture orgProjectExternalAPIFixture) {
	t.Helper()
	if got, want := route.Resource, "projects"; got != want {
		t.Fatalf("route resource = %q, want %q", got, want)
	}
	if got, want := route.Action, "batch_delete"; got != want {
		t.Fatalf("route action = %q, want %q", got, want)
	}
	if route.Method != http.MethodDelete || route.Pattern != "/api/v1/projects/batch" {
		t.Fatalf("route = %s %s, want DELETE /api/v1/projects/batch", route.Method, route.Pattern)
	}
	if !route.AuthRequired {
		t.Fatal("route AuthRequired = false, want true")
	}
	if route.IDParam != "" {
		t.Fatalf("route IDParam = %q, want none", route.IDParam)
	}
	if route.Admin {
		t.Fatal("route Admin = true, want false; batchDeleteProjects enforces admin-panel permission in the handler")
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

func assertOrgProjectExternalAPICompatibility(t *testing.T, fixture orgProjectExternalAPIFixture) {
	t.Helper()
	if !fixture.Compatibility.AdditiveFields || !fixture.Compatibility.TolerantReaders {
		t.Fatalf("compatibility = %+v, want additive fields and tolerant readers", fixture.Compatibility)
	}
}

func assertOrgProjectExampleText(t *testing.T, example map[string]any, field string, want string) {
	t.Helper()
	if got := example[field]; got != want {
		t.Fatalf("example.%s = %v, want %q", field, got, want)
	}
}

func assertOrgProjectExampleNumber(t *testing.T, example map[string]any, field string, want float64) {
	t.Helper()
	if got := example[field]; got != want {
		t.Fatalf("example.%s = %v, want %v", field, got, want)
	}
}

func assertOrgProjectExampleBool(t *testing.T, example map[string]any, field string, want bool) {
	t.Helper()
	if got := example[field]; got != want {
		t.Fatalf("example.%s = %v, want %v", field, got, want)
	}
}

func assertOrgProjectExampleArray(t *testing.T, example map[string]any, field string, want []any) {
	t.Helper()
	if got := example[field]; !reflect.DeepEqual(got, want) {
		t.Fatalf("example.%s = %v, want %v", field, got, want)
	}
}

type orgProjectExternalAPIFixture struct {
	ContractName          string         `json:"contract_name"`
	OwnerService          string         `json:"owner_service"`
	APISurface            string         `json:"api_surface"`
	Consumer              string         `json:"consumer"`
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
	Compatibility         struct {
		AdditiveFields  bool `json:"additive_fields"`
		TolerantReaders bool `json:"tolerant_readers"`
	} `json:"compatibility"`
}

func findOrgProjectExternalRoute(routes []platform.RouteSpec, method, pattern string) (platform.RouteSpec, bool) {
	for _, route := range routes {
		if route.Method == method && route.Pattern == pattern {
			return route, true
		}
	}
	return platform.RouteSpec{}, false
}

func orgProjectServiceEmitsEvent(spec platform.ServiceSpec, event string) bool {
	for _, candidate := range spec.Events {
		if candidate == event {
			return true
		}
	}
	return false
}

func readOrgProjectExternalAPIFixture(t *testing.T, name string) orgProjectExternalAPIFixture {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "contracts", "fixtures", "api", "v1", name))
	if err != nil {
		t.Fatalf("read org-project external API fixture %s: %v", name, err)
	}
	var fixture orgProjectExternalAPIFixture
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatalf("unmarshal org-project external API fixture %s: %v", name, err)
	}
	return fixture
}
