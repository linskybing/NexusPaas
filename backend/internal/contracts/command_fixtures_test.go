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

func TestCommandFixturesAreValidV1(t *testing.T) {
	fixtures := commandFixtureFiles(t)
	want := []string{
		"k8s-control-dispatch-fast-transfer-mover.json",
		"org-project-bind-project-plan.json",
		"org-project-clear-plan-bindings.json",
		"workload-evict-job.json",
		"workload-preempt-job.json",
	}
	if !reflect.DeepEqual(fixtures, want) {
		t.Fatalf("fixture files = %v, want %v", fixtures, want)
	}

	wantRoutes := map[string]commandFixtureRoute{
		"k8s-control-dispatch-fast-transfer-mover.json": {
			ownerService: "k8s-control-service",
			resource:     "k8s-control-service:fast_transfer_mover_jobs",
			action:       "create",
			method:       http.MethodPost,
			path:         "/internal/k8s-control/fast-transfers/mover-jobs",
		},
		"org-project-bind-project-plan.json": {
			ownerService: "org-project-service",
			resource:     "org-project-service:projects",
			action:       "bind_plan",
			method:       http.MethodPut,
			path:         "/internal/org-project/projects/{project_id}/plan",
		},
		"org-project-clear-plan-bindings.json": {
			ownerService: "org-project-service",
			resource:     "org-project-service:projects",
			action:       "clear_plan_bindings",
			method:       http.MethodDelete,
			path:         "/internal/org-project/plans/{plan_id}/project-bindings",
		},
		"workload-evict-job.json": {
			ownerService: "workload-service",
			resource:     "workload-service:jobs",
			action:       "evict",
			method:       http.MethodPost,
			path:         "/internal/workload/jobs/{id}/evict",
		},
		"workload-preempt-job.json": {
			ownerService: "workload-service",
			resource:     "workload-service:jobs",
			action:       "preempt",
			method:       http.MethodPost,
			path:         "/internal/workload/jobs/{id}/preempt",
		},
	}

	seenRoutes := make(map[string]commandFixtureRoute, len(fixtures))
	for _, name := range fixtures {
		fixture := readCommandFixture(t, name)
		assertValidCommandFixture(t, name, fixture)
		seenRoutes[name] = commandFixtureRouteFrom(fixture)
		if got, want := seenRoutes[name], wantRoutes[name]; got != want {
			t.Fatalf("%s route = %#v, want %#v", name, got, want)
		}
	}
}

func TestCommandFixtureCompatibilityAllowsAdditiveFields(t *testing.T) {
	doc := readCommandFixtureDocument(t, "org-project-bind-project-plan.json")
	doc["new_top_level_field"] = "ignored by tolerant readers"
	request := doc["request_example"].(map[string]any)
	request["new_optional_request_field"] = "safe-additive-value"
	response := doc["response_example"].(map[string]any)
	data := response["data"].(map[string]any)
	data["new_optional_response_field"] = map[string]any{"safe_label": "command-fixture"}

	encoded, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal mutated command fixture: %v", err)
	}
	var fixture commandContractFixture
	if err := json.Unmarshal(encoded, &fixture); err != nil {
		t.Fatalf("unmarshal mutated command fixture: %v", err)
	}
	assertValidCommandFixture(t, "org-project-bind-project-plan.json", fixture)
	if _, ok := fixture.RequestExample["new_optional_request_field"]; !ok {
		t.Fatal("request additive field was not preserved")
	}
	responseData := fixture.ResponseExample["data"].(map[string]any)
	if _, ok := responseData["new_optional_response_field"]; !ok {
		t.Fatal("response additive field was not preserved")
	}
}

func TestCommandFixturesRejectForbiddenExampleFields(t *testing.T) {
	cases := map[string]func(*commandContractFixture){
		"request_internal_id": func(fixture *commandContractFixture) {
			fixture.RequestExample["internal_id"] = "42"
		},
		"request_nested_db_id": func(fixture *commandContractFixture) {
			fixture.RequestExample["metadata"] = map[string]any{"db_id": 42}
		},
		"response_access_token": func(fixture *commandContractFixture) {
			fixture.ResponseExample["access_token"] = "redacted"
		},
		"response_password": func(fixture *commandContractFixture) {
			fixture.ResponseExample["password"] = "redacted"
		},
	}

	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := readCommandFixture(t, "org-project-bind-project-plan.json")
			mutate(&fixture)
			if err := validateCommandFixture(fixture); err == nil {
				t.Fatal("validateCommandFixture error = nil, want forbidden field")
			} else if !strings.Contains(err.Error(), "forbidden") {
				t.Fatalf("validateCommandFixture error = %q, want forbidden key message", err.Error())
			}
		})
	}
}

type commandContractFixture struct {
	SchemaVersion         int                       `json:"schema_version"`
	ContractName          string                    `json:"contract_name"`
	OwnerService          string                    `json:"owner_service"`
	ConsumerService       string                    `json:"consumer_service"`
	Resource              string                    `json:"resource"`
	Action                string                    `json:"action"`
	Method                string                    `json:"method"`
	Path                  string                    `json:"path"`
	Auth                  string                    `json:"auth"`
	ServiceKeyRequired    bool                      `json:"service_key_required"`
	PathParameters        []string                  `json:"path_parameters"`
	RequiredRequestFields []string                  `json:"required_request_fields"`
	RequestExample        map[string]any            `json:"request_example"`
	SuccessStatuses       []int                     `json:"success_statuses"`
	ErrorStatuses         []int                     `json:"error_statuses"`
	Idempotency           commandIdempotencyBlock   `json:"idempotency"`
	EmitsEvents           []string                  `json:"emits_events"`
	ResponseExample       map[string]any            `json:"response_example"`
	Compatibility         commandCompatibilityBlock `json:"compatibility"`
}

type commandIdempotencyBlock struct {
	Idempotent  bool     `json:"idempotent"`
	SemanticKey []string `json:"semantic_key"`
	Behavior    string   `json:"behavior"`
}

type commandCompatibilityBlock struct {
	AdditiveFields bool `json:"additive_fields"`
	TolerantReader bool `json:"tolerant_readers"`
}

type commandFixtureRoute struct {
	ownerService string
	resource     string
	action       string
	method       string
	path         string
}

func commandFixtureRouteFrom(fixture commandContractFixture) commandFixtureRoute {
	return commandFixtureRoute{
		ownerService: fixture.OwnerService,
		resource:     fixture.Resource,
		action:       fixture.Action,
		method:       fixture.Method,
		path:         fixture.Path,
	}
}

func assertValidCommandFixture(t *testing.T, name string, fixture commandContractFixture) {
	t.Helper()
	if err := validateCommandFixture(fixture); err != nil {
		t.Fatalf("%s is not a valid command fixture: %v", name, err)
	}
}

func validateCommandFixture(fixture commandContractFixture) error {
	if err := validateCommandRequiredMetadata(fixture); err != nil {
		return err
	}
	if err := validateCommandRoute(fixture); err != nil {
		return err
	}
	if err := validateCommandRequiredRequest(fixture); err != nil {
		return err
	}
	if err := validateCommandStatuses(fixture); err != nil {
		return err
	}
	if err := validateCommandIdempotency(fixture); err != nil {
		return err
	}
	if fixture.EmitsEvents == nil {
		return fmt.Errorf("command fixture emits_events must be present")
	}
	if !fixture.Compatibility.AdditiveFields || !fixture.Compatibility.TolerantReader {
		return fmt.Errorf("command fixture compatibility must allow additive fields and tolerant readers")
	}
	if err := validateCommandExamplePayload("request_example", fixture.RequestExample); err != nil {
		return err
	}
	if err := validateCommandExamplePayload("response_example", fixture.ResponseExample); err != nil {
		return err
	}
	return nil
}

func validateCommandRequiredMetadata(fixture commandContractFixture) error {
	missing := make([]string, 0, 10)
	if fixture.SchemaVersion == 0 {
		missing = append(missing, "schema_version")
	}
	if fixture.ContractName == "" {
		missing = append(missing, "contract_name")
	}
	if fixture.OwnerService == "" {
		missing = append(missing, "owner_service")
	}
	if fixture.ConsumerService == "" {
		missing = append(missing, "consumer_service")
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
		return fmt.Errorf("command fixture missing required fields: %s", strings.Join(missing, ", "))
	}
	return nil
}

func validateCommandRoute(fixture commandContractFixture) error {
	if fixture.SchemaVersion != 1 {
		return fmt.Errorf("command fixture schema_version = %d, want 1", fixture.SchemaVersion)
	}
	if fixture.Auth != "service_key" || !fixture.ServiceKeyRequired {
		return fmt.Errorf("command fixture auth = %q service_key_required=%v, want service_key/true", fixture.Auth, fixture.ServiceKeyRequired)
	}
	if fixture.ConsumerService != "scheduler-quota-service" && fixture.ConsumerService != "storage-service" {
		return fmt.Errorf("command fixture consumer_service = %q, want known internal consumer", fixture.ConsumerService)
	}
	if !strings.HasPrefix(fixture.Path, "/internal/") {
		return fmt.Errorf("command fixture path = %q, want internal path", fixture.Path)
	}
	if fixture.Method != http.MethodPut && fixture.Method != http.MethodPost && fixture.Method != http.MethodDelete {
		return fmt.Errorf("command fixture method = %q, want PUT, POST, or DELETE", fixture.Method)
	}
	for _, param := range fixture.PathParameters {
		if strings.TrimSpace(param) == "" {
			return fmt.Errorf("command fixture path_parameters contains empty value")
		}
		if !strings.Contains(fixture.Path, "{"+param+"}") {
			return fmt.Errorf("command fixture path parameter %q missing from path %q", param, fixture.Path)
		}
	}
	return nil
}

func validateCommandRequiredRequest(fixture commandContractFixture) error {
	if fixture.Method != http.MethodDelete && len(fixture.RequiredRequestFields) == 0 {
		return fmt.Errorf("command fixture required_request_fields is empty for %s", fixture.Method)
	}
	for _, field := range fixture.RequiredRequestFields {
		value, ok := fixture.RequestExample[field]
		if !ok {
			return fmt.Errorf("command fixture request_example missing required field %s", field)
		}
		if text, ok := value.(string); ok && strings.TrimSpace(text) == "" {
			return fmt.Errorf("command fixture request_example.%s is empty", field)
		}
	}
	return nil
}

func validateCommandStatuses(fixture commandContractFixture) error {
	if len(fixture.SuccessStatuses) == 0 {
		return fmt.Errorf("command fixture success_statuses is empty")
	}
	for _, status := range fixture.SuccessStatuses {
		if status < 200 || status > 299 {
			return fmt.Errorf("command fixture success status = %d, want 2xx", status)
		}
	}
	if len(fixture.ErrorStatuses) == 0 {
		return fmt.Errorf("command fixture error_statuses is empty")
	}
	for _, status := range fixture.ErrorStatuses {
		if status < 400 {
			return fmt.Errorf("command fixture error status = %d, want >= 400", status)
		}
	}
	return nil
}

func validateCommandIdempotency(fixture commandContractFixture) error {
	if !fixture.Idempotency.Idempotent {
		return fmt.Errorf("command fixture idempotency.idempotent must be true")
	}
	if len(fixture.Idempotency.SemanticKey) == 0 {
		return fmt.Errorf("command fixture idempotency.semantic_key is empty")
	}
	if strings.TrimSpace(fixture.Idempotency.Behavior) == "" {
		return fmt.Errorf("command fixture idempotency.behavior is empty")
	}
	return nil
}

func validateCommandExamplePayload(path string, value any) error {
	switch typed := value.(type) {
	case map[string]any:
		for key, item := range typed {
			fieldPath := path + "." + key
			if forbiddenEventEnvelopePayloadKey(key) {
				return fmt.Errorf("command example key %q is forbidden", fieldPath)
			}
			if err := validateCommandExamplePayload(fieldPath, item); err != nil {
				return err
			}
		}
	case []any:
		for i, item := range typed {
			if err := validateCommandExamplePayload(fmt.Sprintf("%s[%d]", path, i), item); err != nil {
				return err
			}
		}
	}
	return nil
}

func commandFixtureFiles(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(commandFixtureDir())
	if err != nil {
		t.Fatalf("read command fixtures: %v", err)
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

func readCommandFixture(t *testing.T, name string) commandContractFixture {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(commandFixtureDir(), name))
	if err != nil {
		t.Fatalf("read command fixture %s: %v", name, err)
	}
	var fixture commandContractFixture
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatalf("unmarshal command fixture %s: %v", name, err)
	}
	return fixture
}

func readCommandFixtureDocument(t *testing.T, name string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(commandFixtureDir(), name))
	if err != nil {
		t.Fatalf("read command fixture %s: %v", name, err)
	}
	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatalf("unmarshal command fixture document %s: %v", name, err)
	}
	return document
}

func commandFixtureDir() string {
	return filepath.Join("fixtures", "commands", "v1")
}
