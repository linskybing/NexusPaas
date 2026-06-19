package schedulerquota

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestCommandFixturesMatchSchedulerQuotaClients(t *testing.T) {
	cases := map[string]schedulerQuotaCommandFixtureExpectation{
		"org-project-bind-project-plan.json": {
			ownerService:   orgProjectServiceName,
			method:         http.MethodPut,
			path:           bindProjectPlanPathTemplate,
			requiredFields: []string{"plan_id"},
			requestFields:  jsonFieldsOf(t, bindPlanRequest{PlanID: "plan-ga-standard"}),
			semanticKey:    []string{"project_id", "plan_id"},
		},
		"org-project-clear-plan-bindings.json": {
			ownerService:   orgProjectServiceName,
			method:         http.MethodDelete,
			path:           clearPlanBindingsPathTemplate,
			requiredFields: []string{},
			requestFields:  nil,
			semanticKey:    []string{"plan_id"},
		},
		"workload-preempt-job.json": {
			ownerService:   workloadServiceName,
			method:         http.MethodPost,
			path:           workloadPreemptJobPathTemplate,
			requiredFields: []string{"preemption_id", "reason", "cleanup"},
			requestFields: jsonFieldsOf(t, workloadPreemptRequest{
				PreemptionID:   "preempt-ga-001",
				RequesterJobID: "job-ga-priority",
				Reason:         "higher-priority workload requires capacity",
				Cleanup:        map[string]any{"action": "delete_pod"},
			}),
			semanticKey: []string{"id", "preemption_id"},
		},
		"workload-evict-job.json": {
			ownerService:   workloadServiceName,
			method:         http.MethodPost,
			path:           workloadEvictJobPathTemplate,
			requiredFields: []string{"reason"},
			requestFields:  jsonFieldsOf(t, workloadEvictRequest{Reason: "plan window expired"}),
			semanticKey:    []string{"id"},
		},
	}

	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := readSchedulerQuotaCommandFixture(t, name)
			assertSchedulerQuotaCommandFixture(t, fixture, want)
		})
	}
}

type schedulerQuotaCommandFixtureExpectation struct {
	ownerService   string
	method         string
	path           string
	requiredFields []string
	requestFields  []string
	semanticKey    []string
}

type schedulerQuotaCommandFixture struct {
	OwnerService          string   `json:"owner_service"`
	ConsumerService       string   `json:"consumer_service"`
	Method                string   `json:"method"`
	Path                  string   `json:"path"`
	ServiceKeyRequired    bool     `json:"service_key_required"`
	RequiredRequestFields []string `json:"required_request_fields"`
	Idempotency           struct {
		SemanticKey []string `json:"semantic_key"`
	} `json:"idempotency"`
}

func assertSchedulerQuotaCommandFixture(t *testing.T, fixture schedulerQuotaCommandFixture, want schedulerQuotaCommandFixtureExpectation) {
	t.Helper()
	if fixture.ConsumerService != serviceName {
		t.Fatalf("consumer_service = %q, want %q", fixture.ConsumerService, serviceName)
	}
	if fixture.OwnerService != want.ownerService {
		t.Fatalf("owner_service = %q, want %q", fixture.OwnerService, want.ownerService)
	}
	if fixture.Method != want.method || fixture.Path != want.path {
		t.Fatalf("route = %s %s, want %s %s", fixture.Method, fixture.Path, want.method, want.path)
	}
	assertSchedulerQuotaCommandFixtureFields(t, fixture, want)
}

func assertSchedulerQuotaCommandFixtureFields(t *testing.T, fixture schedulerQuotaCommandFixture, want schedulerQuotaCommandFixtureExpectation) {
	t.Helper()
	if !reflect.DeepEqual(fixture.RequiredRequestFields, want.requiredFields) {
		t.Fatalf("required_request_fields = %v, want %v", fixture.RequiredRequestFields, want.requiredFields)
	}
	for _, field := range fixture.RequiredRequestFields {
		if !containsSchedulerQuotaString(want.requestFields, field) {
			t.Fatalf("required field %q is not sent by scheduler-quota request struct fields %v", field, want.requestFields)
		}
	}
	if !reflect.DeepEqual(fixture.Idempotency.SemanticKey, want.semanticKey) {
		t.Fatalf("semantic_key = %v, want %v", fixture.Idempotency.SemanticKey, want.semanticKey)
	}
	if !fixture.ServiceKeyRequired {
		t.Fatal("service_key_required = false, want true")
	}
}

func readSchedulerQuotaCommandFixture(t *testing.T, name string) schedulerQuotaCommandFixture {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "contracts", "fixtures", "commands", "v1", name))
	if err != nil {
		t.Fatalf("read command fixture %s: %v", name, err)
	}
	var fixture schedulerQuotaCommandFixture
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatalf("unmarshal command fixture %s: %v", name, err)
	}
	return fixture
}

func jsonFieldsOf(t *testing.T, value any) []string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal request fields: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(encoded, &doc); err != nil {
		t.Fatalf("unmarshal request fields: %v", err)
	}
	fields := make([]string, 0, len(doc))
	for field := range doc {
		fields = append(fields, field)
	}
	return fields
}

func containsSchedulerQuotaString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
