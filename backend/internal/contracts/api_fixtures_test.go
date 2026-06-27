package contracts

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestExternalAPIFixturesAreValidV1(t *testing.T) {
	fixtures := externalAPIFixtureFiles(t)
	want := []string{"image-registry-context-build.json", "image-registry-dockerfile-build.json", "image-registry-storage-build.json", "org-project-batch-delete-groups.json", "org-project-batch-delete-projects.json", "org-project-create-group.json", "org-project-create-project.json", "org-project-delete-group.json", "org-project-delete-project.json", "org-project-update-group.json", "org-project-update-project.json", "request-notification-create-form.json", "scheduler-create-network-profile.json", "scheduler-create-placement-profile.json", "storage-batch-delete-project-permissions.json", "storage-batch-update-project-permissions.json", "storage-create-benchmark-record.json", "storage-create-cache-binding.json", "storage-create-permission.json", "storage-create-profile.json", "storage-create-project-binding.json", "storage-delete-project-permission.json", "storage-list-benchmark-records.json", "storage-update-project-permission.json", "workload-cancel-job.json", "workload-commit-configfile-version.json", "workload-create-configfile.json", "workload-delete-configfile.json", "workload-get-configfile.json", "workload-patch-configfile.json", "workload-submit-job.json", "workload-update-configfile.json"}
	if !reflect.DeepEqual(fixtures, want) {
		t.Fatalf("fixture files = %v, want %v", fixtures, want)
	}

	wantRoutes := map[string]externalAPIFixtureRoute{
		"image-registry-context-build.json": {
			ownerService: "image-registry-service",
			resource:     "image-registry-service:image_builds",
			action:       "command",
			method:       http.MethodPost,
			path:         "/api/v1/images/build",
		},
		"image-registry-dockerfile-build.json": {
			ownerService: "image-registry-service",
			resource:     "image-registry-service:image_builds",
			action:       "command",
			method:       http.MethodPost,
			path:         "/api/v1/images/build/dockerfile",
		},
		"image-registry-storage-build.json": {
			ownerService: "image-registry-service",
			resource:     "image-registry-service:image_builds",
			action:       "command",
			method:       http.MethodPost,
			path:         "/api/v1/images/build/from-storage",
		},
		"org-project-create-group.json": {
			ownerService: "org-project-service",
			resource:     "org-project-service:groups",
			action:       "create",
			method:       http.MethodPost,
			path:         "/api/v1/groups",
		},
		"org-project-update-group.json": {
			ownerService: "org-project-service",
			resource:     "org-project-service:groups",
			action:       "update",
			method:       http.MethodPut,
			path:         "/api/v1/groups/{id}",
		},
		"org-project-delete-group.json": {
			ownerService: "org-project-service",
			resource:     "org-project-service:groups",
			action:       "delete",
			method:       http.MethodDelete,
			path:         "/api/v1/groups/{id}",
		},
		"org-project-batch-delete-groups.json": {
			ownerService: "org-project-service",
			resource:     "org-project-service:groups",
			action:       "batch_delete",
			method:       http.MethodDelete,
			path:         "/api/v1/groups/batch",
		},
		"org-project-create-project.json": {
			ownerService: "org-project-service",
			resource:     "org-project-service:projects",
			action:       "create",
			method:       http.MethodPost,
			path:         "/api/v1/projects",
		},
		"org-project-batch-delete-projects.json": {
			ownerService: "org-project-service",
			resource:     "org-project-service:projects",
			action:       "batch_delete",
			method:       http.MethodDelete,
			path:         "/api/v1/projects/batch",
		},
		"org-project-update-project.json": {
			ownerService: "org-project-service",
			resource:     "org-project-service:projects",
			action:       "update",
			method:       http.MethodPut,
			path:         "/api/v1/projects/{id}",
		},
		"org-project-delete-project.json": {
			ownerService: "org-project-service",
			resource:     "org-project-service:projects",
			action:       "delete",
			method:       http.MethodDelete,
			path:         "/api/v1/projects/{id}",
		},
		"request-notification-create-form.json": {
			ownerService: "request-notification-service",
			resource:     "request-notification-service:forms",
			action:       "create",
			method:       http.MethodPost,
			path:         "/api/v1/forms",
		},
		"scheduler-create-network-profile.json": {
			ownerService: "scheduler-quota-service",
			resource:     "scheduler-quota-service:network_profiles",
			action:       "create",
			method:       http.MethodPost,
			path:         "/api/v1/network-profiles",
		},
		"scheduler-create-placement-profile.json": {
			ownerService: "scheduler-quota-service",
			resource:     "scheduler-quota-service:placement_profiles",
			action:       "create",
			method:       http.MethodPost,
			path:         "/api/v1/placement-profiles",
		},
		"storage-batch-delete-project-permissions.json": {
			ownerService: "storage-service",
			resource:     "storage-service:project_storage_permissions",
			action:       "batch_delete",
			method:       http.MethodDelete,
			path:         "/api/v1/projects/{id}/storage/bindings/{pvcId}/permissions/batch",
		},
		"storage-batch-update-project-permissions.json": {
			ownerService: "storage-service",
			resource:     "storage-service:project_storage_permissions",
			action:       "batch_update",
			method:       http.MethodPut,
			path:         "/api/v1/projects/{id}/storage/bindings/{pvcId}/permissions/batch",
		},
		"storage-create-benchmark-record.json": {
			ownerService: "storage-service",
			resource:     "storage-service:storage_benchmark_records",
			action:       "create",
			method:       http.MethodPost,
			path:         "/api/v1/storage/benchmark-records",
		},
		"storage-create-cache-binding.json": {
			ownerService: "storage-service",
			resource:     "storage-service:cache_bindings",
			action:       "create",
			method:       http.MethodPost,
			path:         "/api/v1/projects/{id}/storage/cache-bindings",
		},
		"storage-create-project-binding.json": {
			ownerService: "storage-service",
			resource:     "storage-service:storage_bindings",
			action:       "create",
			method:       http.MethodPost,
			path:         "/api/v1/projects/{id}/storage/bindings",
		},
		"storage-create-permission.json": {
			ownerService: "storage-service",
			resource:     "storage-service:storage_permissions",
			action:       "create",
			method:       http.MethodPost,
			path:         "/api/v1/storage/permissions",
		},
		"storage-create-profile.json": {
			ownerService: "storage-service",
			resource:     "storage-service:storage_profiles",
			action:       "create",
			method:       http.MethodPost,
			path:         "/api/v1/storage-profiles",
		},
		"storage-list-benchmark-records.json": {
			ownerService: "storage-service",
			resource:     "storage-service:storage_benchmark_records",
			action:       "list",
			method:       http.MethodGet,
			path:         "/api/v1/storage/benchmark-records",
		},
		"storage-update-project-permission.json": {
			ownerService: "storage-service",
			resource:     "storage-service:project_storage_permissions",
			action:       "update",
			method:       http.MethodPut,
			path:         "/api/v1/projects/{id}/storage/bindings/{pvcId}/permissions",
		},
		"storage-delete-project-permission.json": {
			ownerService: "storage-service",
			resource:     "storage-service:project_storage_permissions",
			action:       "delete",
			method:       http.MethodDelete,
			path:         "/api/v1/projects/{id}/storage/bindings/{pvcId}/permissions/{userId}",
		},
		"workload-create-configfile.json": {
			ownerService: "workload-service",
			resource:     "workload-service:configfiles",
			action:       "create",
			method:       http.MethodPost,
			path:         "/api/v1/configfiles",
		},
		"workload-delete-configfile.json": {
			ownerService: "workload-service",
			resource:     "workload-service:configfiles",
			action:       "delete",
			method:       http.MethodDelete,
			path:         "/api/v1/configfiles/{id}",
		},
		"workload-get-configfile.json": {
			ownerService: "workload-service",
			resource:     "workload-service:configfiles",
			action:       "get",
			method:       http.MethodGet,
			path:         "/api/v1/configfiles/{id}",
		},
		"workload-update-configfile.json": {
			ownerService: "workload-service",
			resource:     "workload-service:configfiles",
			action:       "update",
			method:       http.MethodPut,
			path:         "/api/v1/configfiles/{id}",
		},
		"workload-patch-configfile.json": {
			ownerService: "workload-service",
			resource:     "workload-service:configfiles",
			action:       "update",
			method:       http.MethodPatch,
			path:         "/api/v1/configfiles/{id}",
		},
		"workload-cancel-job.json": {
			ownerService: "workload-service",
			resource:     "workload-service:jobs",
			action:       "command",
			method:       http.MethodPost,
			path:         "/api/v1/jobs/{id}/cancel",
		},
		"workload-commit-configfile-version.json": {
			ownerService: "workload-service",
			resource:     "workload-service:configfiles",
			action:       "config_commit",
			method:       http.MethodPost,
			path:         "/api/v1/configfiles/{id}/versions",
		},
		"workload-submit-job.json": {
			ownerService: "workload-service",
			resource:     "workload-service:jobs",
			action:       "command",
			method:       http.MethodPost,
			path:         "/api/v1/jobs",
		},
	}

	for _, name := range fixtures {
		fixture := readExternalAPIFixture(t, name)
		assertValidExternalAPIFixture(t, name, fixture)
		if got, want := externalAPIFixtureRouteFrom(fixture), wantRoutes[name]; got != want {
			t.Fatalf("%s route = %#v, want %#v", name, got, want)
		}
	}
}

func TestExternalAPICompatibilityAllowsAdditiveFields(t *testing.T) {
	for _, fixtureName := range externalAPIFixtureFiles(t) {
		t.Run(fixtureName, func(t *testing.T) {
			doc := readExternalAPIFixtureDocument(t, fixtureName)
			isNoBodyRequest := mutateFixtureDocumentForAdditiveCompatibility(doc)
			fixture := decodeExternalAPIFixtureFromDocument(t, fixtureName, doc)
			assertValidExternalAPIFixture(t, fixtureName, fixture)
			assertRequestExamplePreservedForAdditiveFields(t, isNoBodyRequest, fixture.RequestExample)
			assertResponseExamplePreservedForAdditiveFields(t, fixture.ResponseExample)
		})
	}
}

func TestExternalAPIRejectsForbiddenExampleFields(t *testing.T) {
	cases := map[string]func(*externalAPIContractFixture){
		"request_internal_id": func(fixture *externalAPIContractFixture) {
			fixture.RequestExample["internal_id"] = "42"
		},
		"request_nested_db_id": func(fixture *externalAPIContractFixture) {
			fixture.RequestExample["metadata"] = map[string]any{"db_id": 42}
		},
		"response_access_token": func(fixture *externalAPIContractFixture) {
			fixture.ResponseExample["access_token"] = "redacted"
		},
		"response_password": func(fixture *externalAPIContractFixture) {
			fixture.ResponseExample["password"] = "redacted"
		},
	}

	for _, fixtureName := range externalAPIFixtureFiles(t) {
		fixture := readExternalAPIFixture(t, fixtureName)
		isNoBodyRequest := hasNoBodyExternalAPIRequestFixture(fixture)

		for name, mutate := range cases {
			if isNoBodyRequest && (name == "request_internal_id" || name == "request_nested_db_id") {
				continue
			}

			t.Run(fixtureName+"/"+name, func(t *testing.T) {
				fixture := readExternalAPIFixture(t, fixtureName)
				mutate(&fixture)
				if err := validateExternalAPIFixture(fixture); err == nil {
					t.Fatal("validateExternalAPIFixture error = nil, want forbidden field")
				} else if !strings.Contains(err.Error(), "forbidden") {
					t.Fatalf("validateExternalAPIFixture error = %q, want forbidden key message", err.Error())
				}
			})
		}
	}
}

func TestExternalAPIAllowsReadFixtureWithoutEvents(t *testing.T) {
	fixture := readExternalAPIFixture(t, "workload-get-configfile.json")
	if len(fixture.EmitsEvents) != 0 {
		t.Fatalf("emits_events = %v, want none", fixture.EmitsEvents)
	}
	if err := validateExternalAPIFixture(fixture); err != nil {
		t.Fatalf("validateExternalAPIFixture error = %v, want nil", err)
	}
}

func TestExternalAPIRejectsStateChangingFixtureWithoutEvents(t *testing.T) {
	fixture := readExternalAPIFixture(t, "workload-create-configfile.json")
	fixture.EmitsEvents = nil
	if err := validateExternalAPIFixture(fixture); err == nil {
		t.Fatal("validateExternalAPIFixture error = nil, want empty emits_events rejection")
	} else if !strings.Contains(err.Error(), "emits_events") {
		t.Fatalf("validateExternalAPIFixture error = %q, want emits_events message", err.Error())
	}
}

func mutateFixtureDocumentForAdditiveCompatibility(doc map[string]any) bool {
	doc["new_top_level_field"] = "ignored by tolerant readers"

	request := doc["request_example"].(map[string]any)
	isNoBodyRequest := hasNoBodyExternalAPIRequestDocument(
		doc["method"].(string),
		doc["action"].(string),
		doc["path_parameters"].([]any),
		doc["required_request_fields"].([]any),
		doc["optional_request_fields"].([]any),
		request,
	)
	if !isNoBodyRequest {
		request["new_optional_request_field"] = "safe-additive-value"
	}
	doc["response_example"].(map[string]any)["new_optional_response_field"] = map[string]any{"safe_label": "api-fixture"}
	return isNoBodyRequest
}

func decodeExternalAPIFixtureFromDocument(t *testing.T, fixtureName string, doc map[string]any) externalAPIContractFixture {
	t.Helper()
	encoded, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal mutated external API fixture %s: %v", fixtureName, err)
	}
	var fixture externalAPIContractFixture
	if err := json.Unmarshal(encoded, &fixture); err != nil {
		t.Fatalf("unmarshal mutated external API fixture %s: %v", fixtureName, err)
	}
	return fixture
}

func assertRequestExamplePreservedForAdditiveFields(t *testing.T, isNoBodyRequest bool, requestExample map[string]any) {
	t.Helper()
	if isNoBodyRequest {
		if len(requestExample) != 0 {
			t.Fatalf("no-body fixture request example should remain empty")
		}
		return
	}
	if _, ok := requestExample["new_optional_request_field"]; !ok {
		t.Fatal("request additive field was not preserved")
	}
}

func assertResponseExamplePreservedForAdditiveFields(t *testing.T, responseExample map[string]any) {
	t.Helper()
	if _, ok := responseExample["new_optional_response_field"]; !ok {
		t.Fatal("response additive field was not preserved")
	}
}

func hasNoBodyExternalAPIRequestFixture(fixture externalAPIContractFixture) bool {
	return hasNoBodyExternalAPIRequest(
		fixture.Method,
		fixture.Action,
		len(fixture.PathParameters),
		len(fixture.RequiredRequestFields),
		len(fixture.OptionalRequestFields),
		len(fixture.RequestExample),
	)
}

func hasNoBodyExternalAPIRequestDocument(method string, action string, pathParameters []any, requiredRequestFields []any, optionalRequestFields []any, request map[string]any) bool {
	return hasNoBodyExternalAPIRequest(
		method,
		action,
		len(pathParameters),
		len(requiredRequestFields),
		len(optionalRequestFields),
		len(request),
	)
}

func hasNoBodyExternalAPIRequest(method string, action string, pathParameterCount int, requiredRequestFieldCount int, optionalRequestFieldCount int, requestFieldCount int) bool {
	if requiredRequestFieldCount != 0 || optionalRequestFieldCount != 0 || requestFieldCount != 0 {
		return false
	}
	if method == http.MethodDelete {
		return true
	}
	if method == http.MethodGet {
		return true
	}
	return method == http.MethodPost && action == "command" && pathParameterCount > 0
}

type externalAPIContractFixture struct {
	SchemaVersion         int                           `json:"schema_version"`
	ContractName          string                        `json:"contract_name"`
	OwnerService          string                        `json:"owner_service"`
	APISurface            string                        `json:"api_surface"`
	Consumer              string                        `json:"consumer"`
	Resource              string                        `json:"resource"`
	Action                string                        `json:"action"`
	Method                string                        `json:"method"`
	Path                  string                        `json:"path"`
	Auth                  string                        `json:"auth"`
	AuthRequired          bool                          `json:"auth_required"`
	ServiceKeyRequired    bool                          `json:"service_key_required"`
	PathParameters        []string                      `json:"path_parameters"`
	RequiredRequestFields []string                      `json:"required_request_fields"`
	OptionalRequestFields []string                      `json:"optional_request_fields"`
	RequestExample        map[string]any                `json:"request_example"`
	SuccessStatuses       []int                         `json:"success_statuses"`
	ErrorStatuses         []int                         `json:"error_statuses"`
	EmitsEvents           []string                      `json:"emits_events"`
	ResponseExample       map[string]any                `json:"response_example"`
	Compatibility         externalAPICompatibilityBlock `json:"compatibility"`
}

type externalAPICompatibilityBlock struct {
	AdditiveFields bool `json:"additive_fields"`
	TolerantReader bool `json:"tolerant_readers"`
}

type externalAPIFixtureRoute struct {
	ownerService string
	resource     string
	action       string
	method       string
	path         string
}

func externalAPIFixtureRouteFrom(fixture externalAPIContractFixture) externalAPIFixtureRoute {
	return externalAPIFixtureRoute{
		ownerService: fixture.OwnerService,
		resource:     fixture.Resource,
		action:       fixture.Action,
		method:       fixture.Method,
		path:         fixture.Path,
	}
}

func assertValidExternalAPIFixture(t *testing.T, name string, fixture externalAPIContractFixture) {
	t.Helper()
	if err := validateExternalAPIFixture(fixture); err != nil {
		t.Fatalf("%s is not a valid external API fixture: %v", name, err)
	}
}

func validateExternalAPIFixture(fixture externalAPIContractFixture) error {
	if err := validateExternalAPIRequiredMetadata(fixture); err != nil {
		return err
	}
	if err := validateExternalAPIRoute(fixture); err != nil {
		return err
	}
	if err := validateExternalAPIRequiredRequest(fixture); err != nil {
		return err
	}
	if err := validateExternalAPIStatuses(fixture); err != nil {
		return err
	}
	if len(fixture.EmitsEvents) == 0 && !allowsEmptyExternalAPIEvents(fixture) {
		return fmt.Errorf("external API fixture emits_events is empty")
	}
	if !fixture.Compatibility.AdditiveFields || !fixture.Compatibility.TolerantReader {
		return fmt.Errorf("external API fixture compatibility must allow additive fields and tolerant readers")
	}
	if err := validateExternalAPIExamplePayload("request_example", fixture.RequestExample); err != nil {
		return err
	}
	if err := validateExternalAPIExamplePayload("response_example", fixture.ResponseExample); err != nil {
		return err
	}
	return nil
}

func allowsEmptyExternalAPIEvents(fixture externalAPIContractFixture) bool {
	return fixture.Method == http.MethodGet && (fixture.Action == "get" || fixture.Action == "list")
}

func validateExternalAPIRequiredMetadata(fixture externalAPIContractFixture) error {
	missing := make([]string, 0, 12)
	if fixture.SchemaVersion == 0 {
		missing = append(missing, "schema_version")
	}
	if fixture.ContractName == "" {
		missing = append(missing, "contract_name")
	}
	if fixture.OwnerService == "" {
		missing = append(missing, "owner_service")
	}
	if fixture.APISurface == "" {
		missing = append(missing, "api_surface")
	}
	if fixture.Consumer == "" {
		missing = append(missing, "consumer")
	}
	if fixture.Resource == "" {
		missing = append(missing, "resource")
	}
	if fixture.Action == "" {
		missing = append(missing, "action")
	}
	if fixture.Method == "" {
		missing = append(missing, "method")
	}
	if fixture.Path == "" {
		missing = append(missing, "path")
	}
	if fixture.Auth == "" {
		missing = append(missing, "auth")
	}
	if fixture.RequestExample == nil {
		missing = append(missing, "request_example")
	}
	if fixture.ResponseExample == nil {
		missing = append(missing, "response_example")
	}
	if len(missing) > 0 {
		return fmt.Errorf("external API fixture missing required fields: %s", strings.Join(missing, ", "))
	}
	return nil
}

func validateExternalAPIRoute(fixture externalAPIContractFixture) error {
	if fixture.SchemaVersion != 1 {
		return fmt.Errorf("external API fixture schema_version = %d, want 1", fixture.SchemaVersion)
	}
	if fixture.APISurface != "external_rest" {
		return fmt.Errorf("external API fixture api_surface = %q, want external_rest", fixture.APISurface)
	}
	if !strings.HasPrefix(fixture.Path, "/api/v1/") || strings.HasPrefix(fixture.Path, "/internal/") {
		return fmt.Errorf("external API fixture path = %q, want external /api/v1 path", fixture.Path)
	}
	if fixture.Auth != "user" || !fixture.AuthRequired || fixture.ServiceKeyRequired {
		return fmt.Errorf("external API fixture auth = %q auth_required=%v service_key_required=%v, want user/true/false", fixture.Auth, fixture.AuthRequired, fixture.ServiceKeyRequired)
	}
	if !externalAPIMethodAllowed(fixture.Method) {
		return fmt.Errorf("external API fixture method = %q, want GET, POST, PUT, PATCH, or DELETE", fixture.Method)
	}
	for _, param := range fixture.PathParameters {
		if strings.TrimSpace(param) == "" {
			return fmt.Errorf("external API fixture path_parameters contains empty value")
		}
		if !strings.Contains(fixture.Path, "{"+param+"}") {
			return fmt.Errorf("external API fixture path parameter %q missing from path %q", param, fixture.Path)
		}
	}
	return nil
}

func externalAPIMethodAllowed(method string) bool {
	switch method {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func validateExternalAPIRequiredRequest(fixture externalAPIContractFixture) error {
	if hasNoBodyExternalAPIRequestFixture(fixture) {
		return nil
	}
	if len(fixture.RequiredRequestFields) == 0 {
		return fmt.Errorf("external API fixture required_request_fields is empty")
	}
	for _, field := range fixture.RequiredRequestFields {
		value, ok := fixture.RequestExample[field]
		if !ok {
			return fmt.Errorf("external API fixture request_example missing required field %s", field)
		}
		if text, ok := value.(string); ok && strings.TrimSpace(text) == "" {
			return fmt.Errorf("external API fixture request_example.%s is empty", field)
		}
	}
	return nil
}

func validateExternalAPIStatuses(fixture externalAPIContractFixture) error {
	if len(fixture.SuccessStatuses) == 0 {
		return fmt.Errorf("external API fixture success_statuses is empty")
	}
	for _, status := range fixture.SuccessStatuses {
		if status < 200 || status > 299 {
			return fmt.Errorf("external API fixture success status = %d, want 2xx", status)
		}
	}
	if len(fixture.ErrorStatuses) == 0 {
		return fmt.Errorf("external API fixture error_statuses is empty")
	}
	for _, status := range fixture.ErrorStatuses {
		if status < 400 {
			return fmt.Errorf("external API fixture error status = %d, want >= 400", status)
		}
	}
	return nil
}

func validateExternalAPIExamplePayload(path string, value any) error {
	switch typed := value.(type) {
	case map[string]any:
		for key, item := range typed {
			fieldPath := path + "." + key
			if forbiddenEventEnvelopePayloadKey(key) {
				return fmt.Errorf("external API example key %q is forbidden", fieldPath)
			}
			if err := validateExternalAPIExamplePayload(fieldPath, item); err != nil {
				return err
			}
		}
	case []any:
		for i, item := range typed {
			if err := validateExternalAPIExamplePayload(fmt.Sprintf("%s[%d]", path, i), item); err != nil {
				return err
			}
		}
	}
	return nil
}

func externalAPIFixtureFiles(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(externalAPIFixtureDir())
	if err != nil {
		t.Fatalf("read external API fixtures: %v", err)
	}
	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		files = append(files, entry.Name())
	}
	sort.Strings(files)
	return files
}

func readExternalAPIFixture(t *testing.T, name string) externalAPIContractFixture {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(externalAPIFixtureDir(), name))
	if err != nil {
		t.Fatalf("read external API fixture %s: %v", name, err)
	}
	var fixture externalAPIContractFixture
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatalf("unmarshal external API fixture %s: %v", name, err)
	}
	return fixture
}

func readExternalAPIFixtureDocument(t *testing.T, name string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(externalAPIFixtureDir(), name))
	if err != nil {
		t.Fatalf("read external API fixture %s: %v", name, err)
	}
	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatalf("unmarshal external API fixture document %s: %v", name, err)
	}
	return document
}

func externalAPIFixtureDir() string {
	return filepath.Join("fixtures", "api", "v1")
}
