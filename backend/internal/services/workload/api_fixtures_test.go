package workload

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

func TestSubmitJobExternalAPIFixtureMatchesSpec(t *testing.T) {
	fixture := readWorkloadExternalAPIFixture(t, "workload-submit-job.json")
	spec := Spec()
	route, ok := findRoute(spec.Routes, fixture.Method, fixture.Path)
	if !ok {
		t.Fatalf("route %s %s not found in Spec()", fixture.Method, fixture.Path)
	}

	assertSubmitJobFixtureMetadata(t, fixture, spec, route)
	assertSubmitJobRouteMetadata(t, route, fixture)
}

func TestCreateConfigFileExternalAPIFixtureMatchesSpec(t *testing.T) {
	fixture := readWorkloadExternalAPIFixture(t, "workload-create-configfile.json")
	spec := Spec()
	route, ok := findRoute(spec.Routes, fixture.Method, fixture.Path)
	if !ok {
		t.Fatalf("route %s %s not found in Spec()", fixture.Method, fixture.Path)
	}

	assertCreateConfigFileFixtureMetadata(t, fixture, spec, route)
	assertCreateConfigFileRouteMetadata(t, route, fixture)
}

func TestGetConfigFileExternalAPIFixtureMatchesSpec(t *testing.T) {
	fixture := readWorkloadExternalAPIFixture(t, "workload-get-configfile.json")
	spec := Spec()
	route, ok := findRoute(spec.Routes, fixture.Method, fixture.Path)
	if !ok {
		t.Fatalf("route %s %s not found in Spec()", fixture.Method, fixture.Path)
	}

	assertGetConfigFileFixtureMetadata(t, fixture, spec, route)
	assertGetConfigFileRouteMetadata(t, route, fixture)
	assertGetConfigFileResponseExample(t, fixture.ResponseExample)
}

func TestDeleteConfigFileExternalAPIFixtureMatchesSpec(t *testing.T) {
	fixture := readWorkloadExternalAPIFixture(t, "workload-delete-configfile.json")
	spec := Spec()
	route, ok := findRoute(spec.Routes, fixture.Method, fixture.Path)
	if !ok {
		t.Fatalf("route %s %s not found in Spec()", fixture.Method, fixture.Path)
	}

	assertDeleteConfigFileFixtureMetadata(t, fixture, spec, route)
	assertDeleteConfigFileRouteMetadata(t, route, fixture)
	assertDeleteConfigFileResponseExample(t, fixture.ResponseExample)
}

func TestUpdateConfigFileExternalAPIFixtureMatchesSpec(t *testing.T) {
	fixture := readWorkloadExternalAPIFixture(t, "workload-update-configfile.json")
	spec := Spec()
	route, ok := findRoute(spec.Routes, fixture.Method, fixture.Path)
	if !ok {
		t.Fatalf("route %s %s not found in Spec()", fixture.Method, fixture.Path)
	}

	assertUpdateConfigFileFixtureMetadata(t, fixture, spec, route)
	assertUpdateConfigFileRouteMetadata(t, route, fixture)
	assertUpdateConfigFileResponseExample(t, fixture.ResponseExample)
}

func TestPatchConfigFileExternalAPIFixtureMatchesSpec(t *testing.T) {
	fixture := readWorkloadExternalAPIFixture(t, "workload-patch-configfile.json")
	spec := Spec()
	route, ok := findRoute(spec.Routes, fixture.Method, fixture.Path)
	if !ok {
		t.Fatalf("route %s %s not found in Spec()", fixture.Method, fixture.Path)
	}

	assertUpdateConfigFileFixtureMetadata(t, fixture, spec, route)
	assertUpdateConfigFileRouteMetadata(t, route, fixture)
	assertUpdateConfigFileResponseExample(t, fixture.ResponseExample)
}

func TestCommitConfigFileVersionExternalAPIFixtureMatchesSpec(t *testing.T) {
	fixture := readWorkloadExternalAPIFixture(t, "workload-commit-configfile-version.json")
	spec := Spec()
	route, ok := findRoute(spec.Routes, fixture.Method, fixture.Path)
	if !ok {
		t.Fatalf("route %s %s not found in Spec()", fixture.Method, fixture.Path)
	}

	assertCommitConfigFileVersionFixtureMetadata(t, fixture, spec, route)
	assertCommitConfigFileVersionRouteMetadata(t, route, fixture)
	assertCommitConfigFileVersionResponseExample(t, fixture.ResponseExample)
}

func TestCancelJobExternalAPIFixtureMatchesSpec(t *testing.T) {
	fixture := readWorkloadExternalAPIFixture(t, "workload-cancel-job.json")
	spec := Spec()
	route, ok := findRoute(spec.Routes, fixture.Method, fixture.Path)
	if !ok {
		t.Fatalf("route %s %s not found in Spec()", fixture.Method, fixture.Path)
	}

	assertCancelJobFixtureMetadata(t, fixture, spec, route)
	assertCancelJobRouteMetadata(t, route, fixture)
	assertCancelJobResponseExample(t, fixture.ResponseExample)
}

func assertSubmitJobFixtureMetadata(t *testing.T, fixture workloadExternalAPIFixture, spec platform.ServiceSpec, route platform.RouteSpec) {
	t.Helper()
	if fixture.OwnerService != spec.Name {
		t.Fatalf("owner_service = %q, want %q", fixture.OwnerService, spec.Name)
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
	if !reflect.DeepEqual(fixture.RequiredRequestFields, []string{"project_id", "user_id"}) {
		t.Fatalf("required_request_fields = %v, want [project_id user_id]", fixture.RequiredRequestFields)
	}
	if !reflect.DeepEqual(fixture.SuccessStatuses, []int{http.StatusCreated}) {
		t.Fatalf("success_statuses = %v, want [201]", fixture.SuccessStatuses)
	}
	if !reflect.DeepEqual(fixture.EmitsEvents, []string{"JobSubmitted"}) {
		t.Fatalf("emits_events = %v, want [JobSubmitted]", fixture.EmitsEvents)
	}
	if !serviceEmitsEvent(spec, "JobSubmitted") {
		t.Fatal("Spec().Events does not include JobSubmitted")
	}
}

func assertCreateConfigFileFixtureMetadata(t *testing.T, fixture workloadExternalAPIFixture, spec platform.ServiceSpec, route platform.RouteSpec) {
	t.Helper()
	if fixture.OwnerService != spec.Name {
		t.Fatalf("owner_service = %q, want %q", fixture.OwnerService, spec.Name)
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
	if !reflect.DeepEqual(fixture.RequiredRequestFields, []string{"project_id", "name"}) {
		t.Fatalf("required_request_fields = %v, want [project_id name]", fixture.RequiredRequestFields)
	}
	if !reflect.DeepEqual(fixture.OptionalRequestFields, []string{"id", "projectId", "filename", "path", "content", "manifest", "yaml", "config"}) {
		t.Fatalf("optional_request_fields = %v, want [id projectId filename path content manifest yaml config]", fixture.OptionalRequestFields)
	}
	if !reflect.DeepEqual(fixture.SuccessStatuses, []int{http.StatusCreated}) {
		t.Fatalf("success_statuses = %v, want [201]", fixture.SuccessStatuses)
	}
	if !reflect.DeepEqual(fixture.ErrorStatuses, []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusRequestEntityTooLarge, http.StatusUnprocessableEntity, http.StatusInternalServerError}) {
		t.Fatalf("error_statuses = %v, want [400 401 403 413 422 500]", fixture.ErrorStatuses)
	}
	if !reflect.DeepEqual(fixture.EmitsEvents, []string{"ConfigFileChanged"}) {
		t.Fatalf("emits_events = %v, want [ConfigFileChanged]", fixture.EmitsEvents)
	}
	if !serviceEmitsEvent(spec, "ConfigFileChanged") {
		t.Fatal("Spec().Events does not include ConfigFileChanged")
	}
}

func assertGetConfigFileFixtureMetadata(t *testing.T, fixture workloadExternalAPIFixture, spec platform.ServiceSpec, route platform.RouteSpec) {
	t.Helper()
	if fixture.OwnerService != spec.Name {
		t.Fatalf("owner_service = %q, want %q", fixture.OwnerService, spec.Name)
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
	if len(fixture.RequiredRequestFields) != 0 {
		t.Fatalf("required_request_fields = %v, want none", fixture.RequiredRequestFields)
	}
	if len(fixture.OptionalRequestFields) != 0 {
		t.Fatalf("optional_request_fields = %v, want none", fixture.OptionalRequestFields)
	}
	if len(fixture.RequestExample) != 0 {
		t.Fatalf("request_example = %v, want empty object", fixture.RequestExample)
	}
	if !reflect.DeepEqual(fixture.SuccessStatuses, []int{http.StatusOK}) {
		t.Fatalf("success_statuses = %v, want [200]", fixture.SuccessStatuses)
	}
	if !reflect.DeepEqual(fixture.ErrorStatuses, []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusInternalServerError}) {
		t.Fatalf("error_statuses = %v, want [401 403 404 500]", fixture.ErrorStatuses)
	}
	if len(fixture.EmitsEvents) != 0 {
		t.Fatalf("emits_events = %v, want none", fixture.EmitsEvents)
	}
}

func assertDeleteConfigFileFixtureMetadata(t *testing.T, fixture workloadExternalAPIFixture, spec platform.ServiceSpec, route platform.RouteSpec) {
	t.Helper()
	if fixture.OwnerService != spec.Name {
		t.Fatalf("owner_service = %q, want %q", fixture.OwnerService, spec.Name)
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
	if len(fixture.RequiredRequestFields) != 0 {
		t.Fatalf("required_request_fields = %v, want none", fixture.RequiredRequestFields)
	}
	if len(fixture.OptionalRequestFields) != 0 {
		t.Fatalf("optional_request_fields = %v, want none", fixture.OptionalRequestFields)
	}
	if len(fixture.RequestExample) != 0 {
		t.Fatalf("request_example = %v, want empty object", fixture.RequestExample)
	}
	if !reflect.DeepEqual(fixture.SuccessStatuses, []int{http.StatusOK}) {
		t.Fatalf("success_statuses = %v, want [200]", fixture.SuccessStatuses)
	}
	if !reflect.DeepEqual(fixture.ErrorStatuses, []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusInternalServerError}) {
		t.Fatalf("error_statuses = %v, want [401 403 404 500]", fixture.ErrorStatuses)
	}
	if !reflect.DeepEqual(fixture.EmitsEvents, []string{"ConfigFileChanged"}) {
		t.Fatalf("emits_events = %v, want [ConfigFileChanged]", fixture.EmitsEvents)
	}
	if !serviceEmitsEvent(spec, "ConfigFileChanged") {
		t.Fatal("Spec().Events does not include ConfigFileChanged")
	}
}

func assertUpdateConfigFileFixtureMetadata(t *testing.T, fixture workloadExternalAPIFixture, spec platform.ServiceSpec, route platform.RouteSpec) {
	t.Helper()
	if fixture.OwnerService != spec.Name {
		t.Fatalf("owner_service = %q, want %q", fixture.OwnerService, spec.Name)
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
	if !reflect.DeepEqual(fixture.RequiredRequestFields, []string{"content"}) {
		t.Fatalf("required_request_fields = %v, want [content]", fixture.RequiredRequestFields)
	}
	if !reflect.DeepEqual(fixture.OptionalRequestFields, []string{"name", "filename", "path", "manifest", "yaml", "config", "projectId", "project_id"}) {
		t.Fatalf("optional_request_fields = %v, want [name filename path manifest yaml config projectId project_id]", fixture.OptionalRequestFields)
	}
	if got, want := fixture.RequestExample["content"], "batch_size: 64\nlearning_rate: 0.0005\n"; got != want {
		t.Fatalf("request_example.content = %v, want %q", got, want)
	}
	if _, ok := fixture.RequestExample["project_id"]; ok {
		t.Fatal("request_example.project_id present; fixture must not imply project movement")
	}
	if _, ok := fixture.RequestExample["projectId"]; ok {
		t.Fatal("request_example.projectId present; fixture must not imply project movement")
	}
	if !reflect.DeepEqual(fixture.SuccessStatuses, []int{http.StatusOK}) {
		t.Fatalf("success_statuses = %v, want [200]", fixture.SuccessStatuses)
	}
	if !reflect.DeepEqual(fixture.ErrorStatuses, []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusRequestEntityTooLarge, http.StatusUnprocessableEntity, http.StatusInternalServerError}) {
		t.Fatalf("error_statuses = %v, want [400 401 403 404 413 422 500]", fixture.ErrorStatuses)
	}
	if !reflect.DeepEqual(fixture.EmitsEvents, []string{"ConfigFileChanged"}) {
		t.Fatalf("emits_events = %v, want [ConfigFileChanged]", fixture.EmitsEvents)
	}
	if !serviceEmitsEvent(spec, "ConfigFileChanged") {
		t.Fatal("Spec().Events does not include ConfigFileChanged")
	}
}

func assertCommitConfigFileVersionFixtureMetadata(t *testing.T, fixture workloadExternalAPIFixture, spec platform.ServiceSpec, route platform.RouteSpec) {
	t.Helper()
	if fixture.OwnerService != spec.Name {
		t.Fatalf("owner_service = %q, want %q", fixture.OwnerService, spec.Name)
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
	if !reflect.DeepEqual(fixture.RequiredRequestFields, []string{"content"}) {
		t.Fatalf("required_request_fields = %v, want [content]", fixture.RequiredRequestFields)
	}
	if !reflect.DeepEqual(fixture.OptionalRequestFields, []string{"message", "manifest", "yaml", "config"}) {
		t.Fatalf("optional_request_fields = %v, want [message manifest yaml config]", fixture.OptionalRequestFields)
	}
	if !reflect.DeepEqual(fixture.SuccessStatuses, []int{http.StatusCreated}) {
		t.Fatalf("success_statuses = %v, want [201]", fixture.SuccessStatuses)
	}
	if !reflect.DeepEqual(fixture.ErrorStatuses, []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusRequestEntityTooLarge, http.StatusUnprocessableEntity, http.StatusInternalServerError}) {
		t.Fatalf("error_statuses = %v, want [400 401 403 404 413 422 500]", fixture.ErrorStatuses)
	}
	if !reflect.DeepEqual(fixture.EmitsEvents, []string{"ConfigCommitted"}) {
		t.Fatalf("emits_events = %v, want [ConfigCommitted]", fixture.EmitsEvents)
	}
	if !serviceEmitsEvent(spec, "ConfigCommitted") {
		t.Fatal("Spec().Events does not include ConfigCommitted")
	}
	if got, want := fixture.RequestExample["content"], "kind: Job\nmetadata:\n  name: train\n"; got != want {
		t.Fatalf("request_example.content = %v, want %q", got, want)
	}
}

func assertCancelJobFixtureMetadata(t *testing.T, fixture workloadExternalAPIFixture, spec platform.ServiceSpec, route platform.RouteSpec) {
	t.Helper()
	if fixture.OwnerService != spec.Name {
		t.Fatalf("owner_service = %q, want %q", fixture.OwnerService, spec.Name)
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
	if len(fixture.RequiredRequestFields) != 0 {
		t.Fatalf("required_request_fields = %v, want none", fixture.RequiredRequestFields)
	}
	if len(fixture.OptionalRequestFields) != 0 {
		t.Fatalf("optional_request_fields = %v, want none", fixture.OptionalRequestFields)
	}
	if !reflect.DeepEqual(fixture.OptionalRequestHeaders, []string{"Idempotency-Key"}) {
		t.Fatalf("optional_request_headers = %v, want [Idempotency-Key]", fixture.OptionalRequestHeaders)
	}
	if len(fixture.RequestExample) != 0 {
		t.Fatalf("request_example = %v, want empty object", fixture.RequestExample)
	}
	if !reflect.DeepEqual(fixture.SuccessStatuses, []int{http.StatusAccepted}) {
		t.Fatalf("success_statuses = %v, want [202]", fixture.SuccessStatuses)
	}
	if !reflect.DeepEqual(fixture.ErrorStatuses, []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusConflict, http.StatusInternalServerError}) {
		t.Fatalf("error_statuses = %v, want [400 401 403 404 409 500]", fixture.ErrorStatuses)
	}
	if !reflect.DeepEqual(fixture.EmitsEvents, []string{"JobCancelRequested"}) {
		t.Fatalf("emits_events = %v, want [JobCancelRequested]", fixture.EmitsEvents)
	}
	if !serviceEmitsEvent(spec, "JobCancelRequested") {
		t.Fatal("Spec().Events does not include JobCancelRequested")
	}
}

func assertSubmitJobRouteMetadata(t *testing.T, route platform.RouteSpec, fixture workloadExternalAPIFixture) {
	t.Helper()
	assertWorkloadExternalAPIRouteMetadata(t, route, fixture, workloadRouteExpectation{
		resource: "jobs",
		action:   "command",
		method:   http.MethodPost,
		pattern:  "/api/v1/jobs",
	})
}

func assertCreateConfigFileRouteMetadata(t *testing.T, route platform.RouteSpec, fixture workloadExternalAPIFixture) {
	t.Helper()
	assertWorkloadExternalAPIRouteMetadata(t, route, fixture, workloadRouteExpectation{
		resource: "configfiles",
		action:   "create",
		method:   http.MethodPost,
		pattern:  "/api/v1/configfiles",
	})
}

func assertGetConfigFileRouteMetadata(t *testing.T, route platform.RouteSpec, fixture workloadExternalAPIFixture) {
	t.Helper()
	assertWorkloadExternalAPIRouteMetadata(t, route, fixture, workloadRouteExpectation{
		resource: "configfiles",
		action:   "get",
		method:   http.MethodGet,
		pattern:  "/api/v1/configfiles/{id}",
		idParam:  "id",
	})
}

func assertDeleteConfigFileRouteMetadata(t *testing.T, route platform.RouteSpec, fixture workloadExternalAPIFixture) {
	t.Helper()
	assertWorkloadExternalAPIRouteMetadata(t, route, fixture, workloadRouteExpectation{
		resource: "configfiles",
		action:   "delete",
		method:   http.MethodDelete,
		pattern:  "/api/v1/configfiles/{id}",
		idParam:  "id",
	})
}

func assertUpdateConfigFileRouteMetadata(t *testing.T, route platform.RouteSpec, fixture workloadExternalAPIFixture) {
	t.Helper()
	assertWorkloadExternalAPIRouteMetadata(t, route, fixture, workloadRouteExpectation{
		resource: "configfiles",
		action:   "update",
		method:   fixture.Method,
		pattern:  "/api/v1/configfiles/{id}",
		idParam:  "id",
	})
}

func assertCommitConfigFileVersionRouteMetadata(t *testing.T, route platform.RouteSpec, fixture workloadExternalAPIFixture) {
	t.Helper()
	assertWorkloadExternalAPIRouteMetadata(t, route, fixture, workloadRouteExpectation{
		resource: "configfiles",
		action:   "config_commit",
		method:   http.MethodPost,
		pattern:  "/api/v1/configfiles/{id}/versions",
		idParam:  "id",
	})
}

func assertCancelJobRouteMetadata(t *testing.T, route platform.RouteSpec, fixture workloadExternalAPIFixture) {
	t.Helper()
	assertWorkloadExternalAPIRouteMetadata(t, route, fixture, workloadRouteExpectation{
		resource: "jobs",
		action:   "command",
		method:   http.MethodPost,
		pattern:  "/api/v1/jobs/{id}/cancel",
		idParam:  "id",
	})
}

type workloadRouteExpectation struct {
	resource string
	action   string
	method   string
	pattern  string
	idParam  string
}

func assertWorkloadExternalAPIRouteMetadata(t *testing.T, route platform.RouteSpec, fixture workloadExternalAPIFixture, want workloadRouteExpectation) {
	t.Helper()
	if got, want := route.Resource, want.resource; got != want {
		t.Fatalf("route resource = %q, want %q", got, want)
	}
	if got, want := route.Action, want.action; got != want {
		t.Fatalf("route action = %q, want %q", got, want)
	}
	if route.Method != want.method || route.Pattern != want.pattern {
		t.Fatalf("route = %s %s, want %s %s", route.Method, route.Pattern, want.method, want.pattern)
	}
	if !route.AuthRequired {
		t.Fatal("route AuthRequired = false, want true")
	}
	if route.IDParam != want.idParam {
		t.Fatalf("route IDParam = %q, want %q", route.IDParam, want.idParam)
	}
	if route.Admin {
		t.Fatal("route Admin = true, want false")
	}
	if got, want := route.StateChanging, want.method != http.MethodGet; got != want {
		t.Fatalf("route StateChanging = %v, want %v", got, want)
	}
	if route.ServiceAuthRequired {
		t.Fatal("route ServiceAuthRequired = true, want false")
	}
	if fixture.Method != route.Method || fixture.Path != route.Pattern {
		t.Fatalf("fixture route = %s %s, want %s %s", fixture.Method, fixture.Path, route.Method, route.Pattern)
	}
}

func assertCancelJobResponseExample(t *testing.T, response map[string]any) {
	t.Helper()
	if response["id"] == "" {
		t.Fatalf("response_example.id = %v, want command record id", response["id"])
	}
	if response["version"] != float64(1) {
		t.Fatalf("response_example.version = %v, want 1", response["version"])
	}
	data, ok := response["data"].(map[string]any)
	if !ok {
		t.Fatalf("response_example.data = %T, want object", response["data"])
	}
	want := map[string]string{
		"job_id":       "job-ga-001",
		"status":       "accepted",
		"operation":    "workload_job_cancel",
		"requested_at": "2026-06-23T00:00:00Z",
	}
	for field, wantValue := range want {
		if got, ok := data[field].(string); !ok || got != wantValue {
			t.Fatalf("response_example.data.%s = %v, want %q", field, data[field], wantValue)
		}
	}
	for _, forbidden := range []string{
		"idempotency_key",
		"idempotencyKey",
		internalCancelIdempotencyKeyHash,
		internalCancelIdempotencyFingerprintHash,
		"key_hash",
		"fingerprint_hash",
	} {
		if _, ok := data[forbidden]; ok {
			t.Fatalf("response_example.data contains cancel idempotency material")
		}
	}
}

func assertGetConfigFileResponseExample(t *testing.T, response map[string]any) {
	t.Helper()
	if got, want := response["id"], "config-ga-001"; got != want {
		t.Fatalf("response_example.id = %v, want %q", got, want)
	}
	if response["version"] != float64(1) {
		t.Fatalf("response_example.version = %v, want 1", response["version"])
	}
	data, ok := response["data"].(map[string]any)
	if !ok {
		t.Fatalf("response_example.data = %T, want object", response["data"])
	}
	want := map[string]string{
		"id":         "config-ga-001",
		"project_id": "project-ga-001",
		"name":       "training-config.yaml",
		"content":    "batch_size: 32\nlearning_rate: 0.001\n",
		"created_at": "2026-06-22T00:00:00Z",
		"updated_at": "2026-06-22T00:00:00Z",
	}
	for field, wantValue := range want {
		if got, ok := data[field].(string); !ok || got != wantValue {
			t.Fatalf("response_example.data.%s = %v, want %q", field, data[field], wantValue)
		}
	}
}

func assertDeleteConfigFileResponseExample(t *testing.T, response map[string]any) {
	t.Helper()
	if got, want := response["id"], "config-ga-001"; got != want {
		t.Fatalf("response_example.id = %v, want %q", got, want)
	}
	if response["deleted"] != true {
		t.Fatalf("response_example.deleted = %v, want true", response["deleted"])
	}
}

func assertUpdateConfigFileResponseExample(t *testing.T, response map[string]any) {
	t.Helper()
	if got, want := response["id"], "config-ga-001"; got != want {
		t.Fatalf("response_example.id = %v, want %q", got, want)
	}
	if response["version"] != float64(2) {
		t.Fatalf("response_example.version = %v, want 2", response["version"])
	}
	data, ok := response["data"].(map[string]any)
	if !ok {
		t.Fatalf("response_example.data = %T, want object", response["data"])
	}
	want := map[string]string{
		"id":         "config-ga-001",
		"project_id": "project-ga-001",
		"name":       "training-config.yaml",
		"content":    "batch_size: 64\nlearning_rate: 0.0005\n",
		"updated_at": "2026-06-23T00:00:00Z",
	}
	for field, wantValue := range want {
		if got, ok := data[field].(string); !ok || got != wantValue {
			t.Fatalf("response_example.data.%s = %v, want %q", field, data[field], wantValue)
		}
	}
}

func assertCommitConfigFileVersionResponseExample(t *testing.T, response map[string]any) {
	t.Helper()
	if response["id"] == "" {
		t.Fatalf("response_example.id = %v, want version record id", response["id"])
	}
	if response["version"] != float64(1) {
		t.Fatalf("response_example.version = %v, want 1", response["version"])
	}
	data, ok := response["data"].(map[string]any)
	if !ok {
		t.Fatalf("response_example.data = %T, want object", response["data"])
	}
	want := map[string]string{
		"config_id":    "config-ga-001",
		"content":      "kind: Job\nmetadata:\n  name: train\n",
		"message":      "manual",
		"sha256":       "89a3cd4d85b48d3e8dceeeefbdca7ab06f6233b82a0387c7c57198413c38f47c",
		"committed_at": "2026-06-23T00:00:00Z",
	}
	for field, wantValue := range want {
		if got, ok := data[field].(string); !ok || got != wantValue {
			t.Fatalf("response_example.data.%s = %v, want %q", field, data[field], wantValue)
		}
	}
	if data["immutable"] != true {
		t.Fatalf("response_example.data.immutable = %v, want true", data["immutable"])
	}
}

type workloadExternalAPIFixture struct {
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

func findRoute(routes []platform.RouteSpec, method, pattern string) (platform.RouteSpec, bool) {
	for _, route := range routes {
		if route.Method == method && route.Pattern == pattern {
			return route, true
		}
	}
	return platform.RouteSpec{}, false
}

func serviceEmitsEvent(spec platform.ServiceSpec, event string) bool {
	for _, candidate := range spec.Events {
		if candidate == event {
			return true
		}
	}
	return false
}

func readWorkloadExternalAPIFixture(t *testing.T, name string) workloadExternalAPIFixture {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "contracts", "fixtures", "api", "v1", name))
	if err != nil {
		t.Fatalf("read workload external API fixture %s: %v", name, err)
	}
	var fixture workloadExternalAPIFixture
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatalf("unmarshal workload external API fixture %s: %v", name, err)
	}
	return fixture
}
