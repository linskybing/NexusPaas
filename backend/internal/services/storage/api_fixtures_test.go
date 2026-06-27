package storage

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

func TestCreateProjectStorageBindingExternalAPIFixtureMatchesSpec(t *testing.T) {
	fixture := readStorageExternalAPIFixture(t, "storage-create-project-binding.json")
	spec := Spec()
	route, ok := findStorageExternalRoute(spec.Routes, fixture.Method, fixture.Path)
	if !ok {
		t.Fatalf("route %s %s not found in Spec()", fixture.Method, fixture.Path)
	}

	assertCreateProjectStorageBindingExternalAPIFixtureMetadata(t, fixture, spec, route)
	assertCreateProjectStorageBindingExternalAPIRouteMetadata(t, route, fixture)
}

func TestCreateStoragePermissionExternalAPIFixtureMatchesSpec(t *testing.T) {
	fixture := readStorageExternalAPIFixture(t, "storage-create-permission.json")
	spec := Spec()
	route, ok := findStorageExternalRoute(spec.Routes, fixture.Method, fixture.Path)
	if !ok {
		t.Fatalf("route %s %s not found in Spec()", fixture.Method, fixture.Path)
	}

	assertCreateStoragePermissionExternalAPIFixtureMetadata(t, fixture, spec, route)
	assertCreateStoragePermissionExternalAPIRouteMetadata(t, route, fixture)
}

func TestCreateStorageProfileExternalAPIFixtureMatchesSpec(t *testing.T) {
	fixture := readStorageExternalAPIFixture(t, "storage-create-profile.json")
	spec := Spec()
	route, ok := findStorageExternalRoute(spec.Routes, fixture.Method, fixture.Path)
	if !ok {
		t.Fatalf("route %s %s not found in Spec()", fixture.Method, fixture.Path)
	}

	if fixture.ContractName != "storage.create_profile" {
		t.Fatalf("contract_name = %q, want storage.create_profile", fixture.ContractName)
	}
	if got, want := fixture.Resource, spec.Name+":"+route.Resource; got != want {
		t.Fatalf("resource = %q, want %q", got, want)
	}
	if fixture.Action != route.Action || route.Resource != "storage_profiles" || route.Action != "create" {
		t.Fatalf("route metadata = fixture action %q route %s/%s, want create storage_profiles", fixture.Action, route.Resource, route.Action)
	}
	if route.Method != http.MethodPost || route.Pattern != "/api/v1/storage-profiles" || !route.Admin || !route.AuthRequired || route.ServiceAuthRequired {
		t.Fatalf("route = %#v, want admin POST /api/v1/storage-profiles", route)
	}
	if !reflect.DeepEqual(fixture.RequiredRequestFields, []string{"name", "provider", "tier", "access_mode"}) {
		t.Fatalf("required_request_fields = %v, want [name provider tier access_mode]", fixture.RequiredRequestFields)
	}
	if !reflect.DeepEqual(fixture.OptionalRequestFields, []string{"id", "performance_class", "storage_class_name", "mount_mode", "mount_options", "node_selector", "topology_policy", "allow_cross_namespace", "allowed_project_scopes"}) {
		t.Fatalf("optional_request_fields = %v, want storage profile optional fields", fixture.OptionalRequestFields)
	}
	if !reflect.DeepEqual(fixture.SuccessStatuses, []int{http.StatusCreated}) {
		t.Fatalf("success_statuses = %v, want [201]", fixture.SuccessStatuses)
	}
	if !reflect.DeepEqual(fixture.EmitsEvents, []string{"StorageProfileChanged"}) || !storageServiceEmitsEvent(spec, "StorageProfileChanged") {
		t.Fatalf("emits_events = %v and spec events = %v, want StorageProfileChanged", fixture.EmitsEvents, spec.Events)
	}
	data, ok := fixture.ResponseExample["data"].(map[string]any)
	if !ok {
		t.Fatalf("response_example.data = %T, want object", fixture.ResponseExample["data"])
	}
	if got, want := data["storage_class_name"], "local-nvme-scratch"; got != want {
		t.Fatalf("response_example.data.storage_class_name = %v, want %v", got, want)
	}
}

func TestUpdateProjectStoragePermissionExternalAPIFixtureMatchesSpec(t *testing.T) {
	fixture := readStorageExternalAPIFixture(t, "storage-update-project-permission.json")
	spec := Spec()
	route, ok := findStorageExternalRoute(spec.Routes, fixture.Method, fixture.Path)
	if !ok {
		t.Fatalf("route %s %s not found in Spec()", fixture.Method, fixture.Path)
	}

	assertUpdateProjectStoragePermissionExternalAPIFixtureMetadata(t, fixture, spec, route)
	assertUpdateProjectStoragePermissionExternalAPIRouteMetadata(t, route, fixture)
}

func TestDeleteProjectStoragePermissionExternalAPIFixtureMatchesSpec(t *testing.T) {
	fixture := readStorageExternalAPIFixture(t, "storage-delete-project-permission.json")
	spec := Spec()
	route, ok := findStorageExternalRoute(spec.Routes, fixture.Method, fixture.Path)
	if !ok {
		t.Fatalf("route %s %s not found in Spec()", fixture.Method, fixture.Path)
	}

	assertDeleteProjectStoragePermissionExternalAPIFixtureMetadata(t, fixture, spec, route)
	assertDeleteProjectStoragePermissionExternalAPIRouteMetadata(t, route, fixture)
}

func TestBatchUpdateProjectStoragePermissionsExternalAPIFixtureMatchesSpec(t *testing.T) {
	fixture := readStorageExternalAPIFixture(t, "storage-batch-update-project-permissions.json")
	spec := Spec()
	route, ok := findStorageExternalRoute(spec.Routes, fixture.Method, fixture.Path)
	if !ok {
		t.Fatalf("route %s %s not found in Spec()", fixture.Method, fixture.Path)
	}

	assertBatchUpdateProjectStoragePermissionsExternalAPIFixtureMetadata(t, fixture, spec, route)
	assertBatchUpdateProjectStoragePermissionsExternalAPIRouteMetadata(t, route, fixture)
}

func TestBatchDeleteProjectStoragePermissionsExternalAPIFixtureMatchesSpec(t *testing.T) {
	fixture := readStorageExternalAPIFixture(t, "storage-batch-delete-project-permissions.json")
	spec := Spec()
	route, ok := findStorageExternalRoute(spec.Routes, fixture.Method, fixture.Path)
	if !ok {
		t.Fatalf("route %s %s not found in Spec()", fixture.Method, fixture.Path)
	}

	assertBatchDeleteProjectStoragePermissionsExternalAPIFixtureMetadata(t, fixture, spec, route)
	assertBatchDeleteProjectStoragePermissionsExternalAPIRouteMetadata(t, route, fixture)
}

func assertCreateProjectStorageBindingExternalAPIFixtureMetadata(t *testing.T, fixture storageExternalAPIFixture, spec platform.ServiceSpec, route platform.RouteSpec) {
	t.Helper()
	if fixture.ContractName != "storage.create_project_binding" {
		t.Fatalf("contract_name = %q, want storage.create_project_binding", fixture.ContractName)
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
	assertCreateProjectStorageBindingExternalAPIRequestShape(t, fixture)
	assertCreateProjectStorageBindingExternalAPIStatusesAndEvents(t, fixture, spec)
	assertCreateProjectStorageBindingExternalAPIResponseShape(t, fixture)
}

func assertCreateStoragePermissionExternalAPIFixtureMetadata(t *testing.T, fixture storageExternalAPIFixture, spec platform.ServiceSpec, route platform.RouteSpec) {
	t.Helper()
	if fixture.ContractName != "storage.create_permission" {
		t.Fatalf("contract_name = %q, want storage.create_permission", fixture.ContractName)
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
	assertCreateStoragePermissionExternalAPIRequestShape(t, fixture)
	assertCreateStoragePermissionExternalAPIStatusesAndEvents(t, fixture, spec)
	assertCreateStoragePermissionExternalAPIResponseShape(t, fixture)
}

func assertUpdateProjectStoragePermissionExternalAPIFixtureMetadata(t *testing.T, fixture storageExternalAPIFixture, spec platform.ServiceSpec, route platform.RouteSpec) {
	t.Helper()
	if fixture.ContractName != "storage.update_project_permission" {
		t.Fatalf("contract_name = %q, want storage.update_project_permission", fixture.ContractName)
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
	assertUpdateProjectStoragePermissionExternalAPIRequestShape(t, fixture)
	assertUpdateProjectStoragePermissionExternalAPIStatusesAndEvents(t, fixture, spec)
	assertUpdateProjectStoragePermissionExternalAPIResponseShape(t, fixture)
}

func assertDeleteProjectStoragePermissionExternalAPIFixtureMetadata(t *testing.T, fixture storageExternalAPIFixture, spec platform.ServiceSpec, route platform.RouteSpec) {
	t.Helper()
	if fixture.ContractName != "storage.delete_project_permission" {
		t.Fatalf("contract_name = %q, want storage.delete_project_permission", fixture.ContractName)
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
	assertDeleteProjectStoragePermissionExternalAPIRequestShape(t, fixture)
	assertDeleteProjectStoragePermissionExternalAPIStatusesAndEvents(t, fixture, spec)
	assertDeleteProjectStoragePermissionExternalAPIResponseShape(t, fixture)
}

func assertBatchUpdateProjectStoragePermissionsExternalAPIFixtureMetadata(t *testing.T, fixture storageExternalAPIFixture, spec platform.ServiceSpec, route platform.RouteSpec) {
	t.Helper()
	assertProjectStoragePermissionsBatchExternalAPIBaseMetadata(t, fixture, spec, route, "storage.batch_update_project_permissions")
	assertBatchUpdateProjectStoragePermissionsExternalAPIRequestShape(t, fixture)
	assertProjectStoragePermissionsBatchExternalAPIStatusesAndEvents(t, fixture, spec)
	assertProjectStoragePermissionsBatchExternalAPIResponseShape(t, fixture)
}

func assertBatchDeleteProjectStoragePermissionsExternalAPIFixtureMetadata(t *testing.T, fixture storageExternalAPIFixture, spec platform.ServiceSpec, route platform.RouteSpec) {
	t.Helper()
	assertProjectStoragePermissionsBatchExternalAPIBaseMetadata(t, fixture, spec, route, "storage.batch_delete_project_permissions")
	assertBatchDeleteProjectStoragePermissionsExternalAPIRequestShape(t, fixture)
	assertProjectStoragePermissionsBatchExternalAPIStatusesAndEvents(t, fixture, spec)
	assertProjectStoragePermissionsBatchExternalAPIResponseShape(t, fixture)
}

func assertProjectStoragePermissionsBatchExternalAPIBaseMetadata(t *testing.T, fixture storageExternalAPIFixture, spec platform.ServiceSpec, route platform.RouteSpec, contractName string) {
	t.Helper()
	if fixture.ContractName != contractName {
		t.Fatalf("contract_name = %q, want %s", fixture.ContractName, contractName)
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
}

func assertCreateProjectStorageBindingExternalAPIRequestShape(t *testing.T, fixture storageExternalAPIFixture) {
	t.Helper()
	if !reflect.DeepEqual(fixture.PathParameters, []string{"id"}) {
		t.Fatalf("path_parameters = %v, want [id]", fixture.PathParameters)
	}
	if !reflect.DeepEqual(fixture.RequiredRequestFields, []string{"group_id", "pvc_id"}) {
		t.Fatalf("required_request_fields = %v, want [group_id pvc_id]", fixture.RequiredRequestFields)
	}
	if len(fixture.OptionalRequestFields) != 0 {
		t.Fatalf("optional_request_fields = %v, want none", fixture.OptionalRequestFields)
	}
	if got, want := fixture.RequestExample["group_id"], "group-ga-001"; got != want {
		t.Fatalf("request_example.group_id = %v, want %q", got, want)
	}
	if got, want := fixture.RequestExample["pvc_id"], "datasets"; got != want {
		t.Fatalf("request_example.pvc_id = %v, want %q", got, want)
	}
}

func assertCreateStoragePermissionExternalAPIRequestShape(t *testing.T, fixture storageExternalAPIFixture) {
	t.Helper()
	if len(fixture.PathParameters) != 0 {
		t.Fatalf("path_parameters = %v, want none", fixture.PathParameters)
	}
	if !reflect.DeepEqual(fixture.RequiredRequestFields, []string{"group_id", "pvc_id", "user_id", "permission"}) {
		t.Fatalf("required_request_fields = %v, want [group_id pvc_id user_id permission]", fixture.RequiredRequestFields)
	}
	if len(fixture.OptionalRequestFields) != 0 {
		t.Fatalf("optional_request_fields = %v, want none", fixture.OptionalRequestFields)
	}
	want := map[string]any{
		"group_id":   "group-ga-001",
		"pvc_id":     "datasets",
		"user_id":    "user-ga-reader",
		"permission": "read_write",
	}
	for key, wantValue := range want {
		if got := fixture.RequestExample[key]; got != wantValue {
			t.Fatalf("request_example.%s = %v, want %v", key, got, wantValue)
		}
	}
}

func assertUpdateProjectStoragePermissionExternalAPIRequestShape(t *testing.T, fixture storageExternalAPIFixture) {
	t.Helper()
	if !reflect.DeepEqual(fixture.PathParameters, []string{"id", "pvcId"}) {
		t.Fatalf("path_parameters = %v, want [id pvcId]", fixture.PathParameters)
	}
	if !reflect.DeepEqual(fixture.RequiredRequestFields, []string{"user_id", "permission"}) {
		t.Fatalf("required_request_fields = %v, want [user_id permission]", fixture.RequiredRequestFields)
	}
	if len(fixture.OptionalRequestFields) != 0 {
		t.Fatalf("optional_request_fields = %v, want none", fixture.OptionalRequestFields)
	}
	want := map[string]any{
		"user_id":    "user-ga-reader",
		"permission": "read_write",
	}
	for key, wantValue := range want {
		if got := fixture.RequestExample[key]; got != wantValue {
			t.Fatalf("request_example.%s = %v, want %v", key, got, wantValue)
		}
	}
}

func assertDeleteProjectStoragePermissionExternalAPIRequestShape(t *testing.T, fixture storageExternalAPIFixture) {
	t.Helper()
	if !reflect.DeepEqual(fixture.PathParameters, []string{"id", "pvcId", "userId"}) {
		t.Fatalf("path_parameters = %v, want [id pvcId userId]", fixture.PathParameters)
	}
	if len(fixture.RequiredRequestFields) != 0 {
		t.Fatalf("required_request_fields = %v, want []", fixture.RequiredRequestFields)
	}
	if len(fixture.OptionalRequestFields) != 0 {
		t.Fatalf("optional_request_fields = %v, want none", fixture.OptionalRequestFields)
	}
	if len(fixture.RequestExample) != 0 {
		t.Fatalf("request_example = %v, want {}", fixture.RequestExample)
	}
}

func assertBatchUpdateProjectStoragePermissionsExternalAPIRequestShape(t *testing.T, fixture storageExternalAPIFixture) {
	t.Helper()
	items := assertProjectStoragePermissionsBatchExternalAPIRequestEnvelope(t, fixture)
	for i, item := range items {
		fields := projectStoragePermissionsBatchItem(t, item, i)
		assertProjectStoragePermissionsBatchItemField(t, fields, i, "user_id")
		assertProjectStoragePermissionsBatchItemField(t, fields, i, "permission")
	}
}

func assertBatchDeleteProjectStoragePermissionsExternalAPIRequestShape(t *testing.T, fixture storageExternalAPIFixture) {
	t.Helper()
	if fixture.Method != http.MethodDelete {
		t.Fatalf("method = %q, want DELETE", fixture.Method)
	}
	if len(fixture.RequiredRequestFields) == 0 && len(fixture.OptionalRequestFields) == 0 && len(fixture.RequestExample) == 0 {
		t.Fatal("batch delete fixture is modeled as no-body DELETE, want DELETE-with-body")
	}
	items := assertProjectStoragePermissionsBatchExternalAPIRequestEnvelope(t, fixture)
	for i, item := range items {
		fields := projectStoragePermissionsBatchItem(t, item, i)
		assertProjectStoragePermissionsBatchItemField(t, fields, i, "user_id")
	}
}

func assertProjectStoragePermissionsBatchExternalAPIRequestEnvelope(t *testing.T, fixture storageExternalAPIFixture) []any {
	t.Helper()
	if !reflect.DeepEqual(fixture.PathParameters, []string{"id", "pvcId"}) {
		t.Fatalf("path_parameters = %v, want [id pvcId]", fixture.PathParameters)
	}
	if !reflect.DeepEqual(fixture.RequiredRequestFields, []string{"items"}) {
		t.Fatalf("required_request_fields = %v, want [items]", fixture.RequiredRequestFields)
	}
	if len(fixture.OptionalRequestFields) != 0 {
		t.Fatalf("optional_request_fields = %v, want none", fixture.OptionalRequestFields)
	}
	if _, ok := fixture.RequestExample["permissions"]; ok {
		t.Fatal("request_example uses permissions alias, want canonical items field")
	}
	items, ok := fixture.RequestExample["items"].([]any)
	if !ok {
		t.Fatalf("request_example.items = %T, want array", fixture.RequestExample["items"])
	}
	if len(items) == 0 {
		t.Fatal("request_example.items is empty")
	}
	return items
}

func projectStoragePermissionsBatchItem(t *testing.T, item any, index int) map[string]any {
	t.Helper()
	fields, ok := item.(map[string]any)
	if !ok {
		t.Fatalf("request_example.items[%d] = %T, want object", index, item)
	}
	return fields
}

func assertProjectStoragePermissionsBatchItemField(t *testing.T, fields map[string]any, index int, field string) {
	t.Helper()
	value, ok := fields[field].(string)
	if !ok || strings.TrimSpace(value) == "" {
		t.Fatalf("request_example.items[%d].%s = %v, want non-empty string", index, field, fields[field])
	}
}

func assertCreateProjectStorageBindingExternalAPIStatusesAndEvents(t *testing.T, fixture storageExternalAPIFixture, spec platform.ServiceSpec) {
	t.Helper()
	if !reflect.DeepEqual(fixture.SuccessStatuses, []int{http.StatusCreated}) {
		t.Fatalf("success_statuses = %v, want [201]", fixture.SuccessStatuses)
	}
	if !reflect.DeepEqual(fixture.ErrorStatuses, []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusConflict, http.StatusInternalServerError}) {
		t.Fatalf("error_statuses = %v, want [400 401 403 409 500]", fixture.ErrorStatuses)
	}
	if !reflect.DeepEqual(fixture.EmitsEvents, []string{"ProjectStorageBindingChanged"}) {
		t.Fatalf("emits_events = %v, want [ProjectStorageBindingChanged]", fixture.EmitsEvents)
	}
	if !storageServiceEmitsEvent(spec, "ProjectStorageBindingChanged") {
		t.Fatal("Spec().Events does not include ProjectStorageBindingChanged")
	}
}

func assertCreateStoragePermissionExternalAPIStatusesAndEvents(t *testing.T, fixture storageExternalAPIFixture, spec platform.ServiceSpec) {
	t.Helper()
	if !reflect.DeepEqual(fixture.SuccessStatuses, []int{http.StatusOK}) {
		t.Fatalf("success_statuses = %v, want [200]", fixture.SuccessStatuses)
	}
	if !reflect.DeepEqual(fixture.ErrorStatuses, []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusConflict, http.StatusInternalServerError}) {
		t.Fatalf("error_statuses = %v, want [400 401 403 409 500]", fixture.ErrorStatuses)
	}
	if !reflect.DeepEqual(fixture.EmitsEvents, []string{"StoragePermissionChanged"}) {
		t.Fatalf("emits_events = %v, want [StoragePermissionChanged]", fixture.EmitsEvents)
	}
	if !storageServiceEmitsEvent(spec, "StoragePermissionChanged") {
		t.Fatal("Spec().Events does not include StoragePermissionChanged")
	}
}

func assertUpdateProjectStoragePermissionExternalAPIStatusesAndEvents(t *testing.T, fixture storageExternalAPIFixture, spec platform.ServiceSpec) {
	t.Helper()
	if !reflect.DeepEqual(fixture.SuccessStatuses, []int{http.StatusOK}) {
		t.Fatalf("success_statuses = %v, want [200]", fixture.SuccessStatuses)
	}
	if !reflect.DeepEqual(fixture.ErrorStatuses, []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusConflict, http.StatusInternalServerError}) {
		t.Fatalf("error_statuses = %v, want [400 401 403 409 500]", fixture.ErrorStatuses)
	}
	if !reflect.DeepEqual(fixture.EmitsEvents, []string{"ProjectStoragePermissionChanged"}) {
		t.Fatalf("emits_events = %v, want [ProjectStoragePermissionChanged]", fixture.EmitsEvents)
	}
	if !storageServiceEmitsEvent(spec, "ProjectStoragePermissionChanged") {
		t.Fatal("Spec().Events does not include ProjectStoragePermissionChanged")
	}
}

func assertDeleteProjectStoragePermissionExternalAPIStatusesAndEvents(t *testing.T, fixture storageExternalAPIFixture, spec platform.ServiceSpec) {
	t.Helper()
	if !reflect.DeepEqual(fixture.SuccessStatuses, []int{http.StatusOK}) {
		t.Fatalf("success_statuses = %v, want [200]", fixture.SuccessStatuses)
	}
	if !reflect.DeepEqual(fixture.ErrorStatuses, []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusInternalServerError}) {
		t.Fatalf("error_statuses = %v, want [401 403 404 500]", fixture.ErrorStatuses)
	}
	if !reflect.DeepEqual(fixture.EmitsEvents, []string{"ProjectStoragePermissionChanged"}) {
		t.Fatalf("emits_events = %v, want [ProjectStoragePermissionChanged]", fixture.EmitsEvents)
	}
	if !storageServiceEmitsEvent(spec, "ProjectStoragePermissionChanged") {
		t.Fatal("Spec().Events does not include ProjectStoragePermissionChanged")
	}
}

func assertProjectStoragePermissionsBatchExternalAPIStatusesAndEvents(t *testing.T, fixture storageExternalAPIFixture, spec platform.ServiceSpec) {
	t.Helper()
	if !reflect.DeepEqual(fixture.SuccessStatuses, []int{http.StatusOK}) {
		t.Fatalf("success_statuses = %v, want [200]", fixture.SuccessStatuses)
	}
	if !reflect.DeepEqual(fixture.ErrorStatuses, []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusInternalServerError}) {
		t.Fatalf("error_statuses = %v, want [400 401 403 404 500]", fixture.ErrorStatuses)
	}
	if !reflect.DeepEqual(fixture.EmitsEvents, []string{"ProjectStoragePermissionChanged"}) {
		t.Fatalf("emits_events = %v, want [ProjectStoragePermissionChanged]", fixture.EmitsEvents)
	}
	if !storageServiceEmitsEvent(spec, "ProjectStoragePermissionChanged") {
		t.Fatal("Spec().Events does not include ProjectStoragePermissionChanged")
	}
}

func assertCreateProjectStorageBindingExternalAPIResponseShape(t *testing.T, fixture storageExternalAPIFixture) {
	t.Helper()
	if _, ok := fixture.ResponseExample["data"]; ok {
		t.Fatal("response_example contains record envelope data field, want direct storage binding data")
	}
	if _, ok := fixture.ResponseExample["version"]; ok {
		t.Fatal("response_example contains record envelope version field, want direct storage binding data")
	}
	want := map[string]any{
		"id":         "project-ga-001:datasets",
		"project_id": "project-ga-001",
		"group_id":   "group-ga-001",
		"pvc_id":     "datasets",
		"created_by": "user-ga-manager",
		"created_at": "2026-06-23T00:00:00Z",
	}
	for key, wantValue := range want {
		if got := fixture.ResponseExample[key]; got != wantValue {
			t.Fatalf("response_example.%s = %v, want %v", key, got, wantValue)
		}
	}
}

func assertCreateStoragePermissionExternalAPIResponseShape(t *testing.T, fixture storageExternalAPIFixture) {
	t.Helper()
	if _, ok := fixture.ResponseExample["data"]; ok {
		t.Fatal("response_example contains record envelope data field, want direct storage permission data")
	}
	if _, ok := fixture.ResponseExample["version"]; ok {
		t.Fatal("response_example contains record envelope version field, want direct storage permission data")
	}
	want := map[string]any{
		"id":         "group-ga-001:datasets:user-ga-reader",
		"group_id":   "group-ga-001",
		"pvc_id":     "datasets",
		"user_id":    "user-ga-reader",
		"permission": "read_write",
		"updated_at": "2026-06-23T00:00:00Z",
	}
	for key, wantValue := range want {
		if got := fixture.ResponseExample[key]; got != wantValue {
			t.Fatalf("response_example.%s = %v, want %v", key, got, wantValue)
		}
	}
}

func assertUpdateProjectStoragePermissionExternalAPIResponseShape(t *testing.T, fixture storageExternalAPIFixture) {
	t.Helper()
	if _, ok := fixture.ResponseExample["data"]; ok {
		t.Fatal("response_example contains record envelope data field, want direct project storage permission data")
	}
	if _, ok := fixture.ResponseExample["version"]; ok {
		t.Fatal("response_example contains record envelope version field, want direct project storage permission data")
	}
	want := map[string]any{
		"id":         "project-ga-001:datasets:user-ga-reader",
		"project_id": "project-ga-001",
		"pvc_id":     "datasets",
		"user_id":    "user-ga-reader",
		"permission": "read_write",
		"updated_at": "2026-06-23T00:00:00Z",
	}
	for key, wantValue := range want {
		if got := fixture.ResponseExample[key]; got != wantValue {
			t.Fatalf("response_example.%s = %v, want %v", key, got, wantValue)
		}
	}
}

func assertDeleteProjectStoragePermissionExternalAPIResponseShape(t *testing.T, fixture storageExternalAPIFixture) {
	t.Helper()
	if len(fixture.ResponseExample) != 0 {
		t.Fatalf("response_example = %v, want {}", fixture.ResponseExample)
	}
}

func assertProjectStoragePermissionsBatchExternalAPIResponseShape(t *testing.T, fixture storageExternalAPIFixture) {
	t.Helper()
	if _, ok := fixture.ResponseExample["data"]; ok {
		t.Fatal("response_example contains record envelope data field, want direct batchResult data")
	}
	if _, ok := fixture.ResponseExample["version"]; ok {
		t.Fatal("response_example contains record envelope version field, want direct batchResult data")
	}
	if got, want := fixture.ResponseExample["succeeded"], float64(2); got != want {
		t.Fatalf("response_example.succeeded = %v, want %v", got, want)
	}
	if got, want := fixture.ResponseExample["failed"], float64(0); got != want {
		t.Fatalf("response_example.failed = %v, want %v", got, want)
	}
	errors, ok := fixture.ResponseExample["errors"].([]any)
	if !ok {
		t.Fatalf("response_example.errors = %T, want array", fixture.ResponseExample["errors"])
	}
	if len(errors) != 0 {
		t.Fatalf("response_example.errors = %v, want empty array", errors)
	}
}

func assertCreateProjectStorageBindingExternalAPIRouteMetadata(t *testing.T, route platform.RouteSpec, fixture storageExternalAPIFixture) {
	t.Helper()
	if got, want := route.Resource, "storage_bindings"; got != want {
		t.Fatalf("route resource = %q, want %q", got, want)
	}
	if got, want := route.Action, "create"; got != want {
		t.Fatalf("route action = %q, want %q", got, want)
	}
	if route.Method != http.MethodPost || route.Pattern != "/api/v1/projects/{id}/storage/bindings" {
		t.Fatalf("route = %s %s, want POST /api/v1/projects/{id}/storage/bindings", route.Method, route.Pattern)
	}
	if !route.AuthRequired {
		t.Fatal("route AuthRequired = false, want true")
	}
	if route.IDParam != "id" {
		t.Fatalf("route IDParam = %q, want id", route.IDParam)
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
	if route.PolicyBypass {
		t.Fatal("route PolicyBypass = true, want false")
	}
	if route.ExternalAdapter != "" {
		t.Fatalf("route ExternalAdapter = %q, want none", route.ExternalAdapter)
	}
	if fixture.Method != route.Method || fixture.Path != route.Pattern {
		t.Fatalf("fixture route = %s %s, want %s %s", fixture.Method, fixture.Path, route.Method, route.Pattern)
	}
}

func assertCreateStoragePermissionExternalAPIRouteMetadata(t *testing.T, route platform.RouteSpec, fixture storageExternalAPIFixture) {
	t.Helper()
	if got, want := route.Resource, "storage_permissions"; got != want {
		t.Fatalf("route resource = %q, want %q", got, want)
	}
	if got, want := route.Action, "create"; got != want {
		t.Fatalf("route action = %q, want %q", got, want)
	}
	if route.Method != http.MethodPost || route.Pattern != "/api/v1/storage/permissions" {
		t.Fatalf("route = %s %s, want POST /api/v1/storage/permissions", route.Method, route.Pattern)
	}
	if !route.AuthRequired {
		t.Fatal("route AuthRequired = false, want true")
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
	if route.PolicyBypass {
		t.Fatal("route PolicyBypass = true, want false")
	}
	if route.ExternalAdapter != "" {
		t.Fatalf("route ExternalAdapter = %q, want none", route.ExternalAdapter)
	}
	if fixture.Method != route.Method || fixture.Path != route.Pattern {
		t.Fatalf("fixture route = %s %s, want %s %s", fixture.Method, fixture.Path, route.Method, route.Pattern)
	}
}

func assertUpdateProjectStoragePermissionExternalAPIRouteMetadata(t *testing.T, route platform.RouteSpec, fixture storageExternalAPIFixture) {
	t.Helper()
	if got, want := route.Resource, "project_storage_permissions"; got != want {
		t.Fatalf("route resource = %q, want %q", got, want)
	}
	if got, want := route.Action, "update"; got != want {
		t.Fatalf("route action = %q, want %q", got, want)
	}
	if route.Method != http.MethodPut || route.Pattern != "/api/v1/projects/{id}/storage/bindings/{pvcId}/permissions" {
		t.Fatalf("route = %s %s, want PUT /api/v1/projects/{id}/storage/bindings/{pvcId}/permissions", route.Method, route.Pattern)
	}
	if !route.AuthRequired {
		t.Fatal("route AuthRequired = false, want true")
	}
	if route.IDParam != "pvcId" {
		t.Fatalf("route IDParam = %q, want pvcId", route.IDParam)
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
	if route.PolicyBypass {
		t.Fatal("route PolicyBypass = true, want false")
	}
	if route.ExternalAdapter != "" {
		t.Fatalf("route ExternalAdapter = %q, want none", route.ExternalAdapter)
	}
	if fixture.Method != route.Method || fixture.Path != route.Pattern {
		t.Fatalf("fixture route = %s %s, want %s %s", fixture.Method, fixture.Path, route.Method, route.Pattern)
	}
}

func assertDeleteProjectStoragePermissionExternalAPIRouteMetadata(t *testing.T, route platform.RouteSpec, fixture storageExternalAPIFixture) {
	t.Helper()
	if got, want := route.Resource, "project_storage_permissions"; got != want {
		t.Fatalf("route resource = %q, want %q", got, want)
	}
	if got, want := route.Action, "delete"; got != want {
		t.Fatalf("route action = %q, want %q", got, want)
	}
	if route.Method != http.MethodDelete || route.Pattern != "/api/v1/projects/{id}/storage/bindings/{pvcId}/permissions/{userId}" {
		t.Fatalf("route = %s %s, want DELETE /api/v1/projects/{id}/storage/bindings/{pvcId}/permissions/{userId}", route.Method, route.Pattern)
	}
	if !route.AuthRequired {
		t.Fatal("route AuthRequired = false, want true")
	}
	if route.IDParam != "userId" {
		t.Fatalf("route IDParam = %q, want userId", route.IDParam)
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
	if route.PolicyBypass {
		t.Fatal("route PolicyBypass = true, want false")
	}
	if route.ExternalAdapter != "" {
		t.Fatalf("route ExternalAdapter = %q, want none", route.ExternalAdapter)
	}
	if fixture.Method != route.Method || fixture.Path != route.Pattern {
		t.Fatalf("fixture route = %s %s, want %s %s", fixture.Method, fixture.Path, route.Method, route.Pattern)
	}
}

func assertBatchUpdateProjectStoragePermissionsExternalAPIRouteMetadata(t *testing.T, route platform.RouteSpec, fixture storageExternalAPIFixture) {
	t.Helper()
	assertProjectStoragePermissionsBatchExternalAPIRouteMetadata(t, route, fixture, http.MethodPut, "batch_update")
}

func assertBatchDeleteProjectStoragePermissionsExternalAPIRouteMetadata(t *testing.T, route platform.RouteSpec, fixture storageExternalAPIFixture) {
	t.Helper()
	assertProjectStoragePermissionsBatchExternalAPIRouteMetadata(t, route, fixture, http.MethodDelete, "batch_delete")
}

func assertProjectStoragePermissionsBatchExternalAPIRouteMetadata(t *testing.T, route platform.RouteSpec, fixture storageExternalAPIFixture, method string, action string) {
	t.Helper()
	if got, want := route.Resource, "project_storage_permissions"; got != want {
		t.Fatalf("route resource = %q, want %q", got, want)
	}
	if got, want := route.Action, action; got != want {
		t.Fatalf("route action = %q, want %q", got, want)
	}
	if route.Method != method || route.Pattern != "/api/v1/projects/{id}/storage/bindings/{pvcId}/permissions/batch" {
		t.Fatalf("route = %s %s, want %s /api/v1/projects/{id}/storage/bindings/{pvcId}/permissions/batch", route.Method, route.Pattern, method)
	}
	if !route.AuthRequired {
		t.Fatal("route AuthRequired = false, want true")
	}
	if route.IDParam != "pvcId" {
		t.Fatalf("route IDParam = %q, want pvcId", route.IDParam)
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
	if route.PolicyBypass {
		t.Fatal("route PolicyBypass = true, want false")
	}
	if route.ExternalAdapter != "" {
		t.Fatalf("route ExternalAdapter = %q, want none", route.ExternalAdapter)
	}
	if fixture.Method != route.Method || fixture.Path != route.Pattern {
		t.Fatalf("fixture route = %s %s, want %s %s", fixture.Method, fixture.Path, route.Method, route.Pattern)
	}
}

type storageExternalAPIFixture struct {
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
}

func findStorageExternalRoute(routes []platform.RouteSpec, method, pattern string) (platform.RouteSpec, bool) {
	for _, route := range routes {
		if route.Method == method && route.Pattern == pattern {
			return route, true
		}
	}
	return platform.RouteSpec{}, false
}

func storageServiceEmitsEvent(spec platform.ServiceSpec, event string) bool {
	for _, candidate := range spec.Events {
		if candidate == event {
			return true
		}
	}
	return false
}

func readStorageExternalAPIFixture(t *testing.T, name string) storageExternalAPIFixture {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "contracts", "fixtures", "api", "v1", name))
	if err != nil {
		t.Fatalf("read storage external API fixture %s: %v", name, err)
	}
	var fixture storageExternalAPIFixture
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatalf("unmarshal storage external API fixture %s: %v", name, err)
	}
	return fixture
}
