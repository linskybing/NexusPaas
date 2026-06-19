package contracts

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestOwnerReadFixturesAreValidV1(t *testing.T) {
	fixtures := ownerReadFixtureFiles(t)
	want := []string{
		"org-project-project-members.json",
		"org-project-projects.json",
		"org-project-user-groups.json",
		"org-project-user-quotas.json",
		"workload-jobs.json",
	}
	if !reflect.DeepEqual(fixtures, want) {
		t.Fatalf("fixture files = %v, want %v", fixtures, want)
	}

	wantResources := map[string]ownerReadFixtureRoute{
		"org-project-project-members.json": {
			ownerService: "org-project-service",
			resource:     "org-project-service:project_members",
			listPath:     "/internal/org-project/project-members",
			getPath:      "/internal/org-project/project-members/{id}",
		},
		"org-project-projects.json": {
			ownerService: "org-project-service",
			resource:     "org-project-service:projects",
			listPath:     "/internal/org-project/projects",
			getPath:      "/internal/org-project/projects/{id}",
		},
		"org-project-user-groups.json": {
			ownerService: "org-project-service",
			resource:     "org-project-service:user_groups",
			listPath:     "/internal/org-project/user-groups",
			getPath:      "/internal/org-project/user-groups/{id}",
		},
		"org-project-user-quotas.json": {
			ownerService: "org-project-service",
			resource:     "org-project-service:user_quotas",
			listPath:     "/internal/org-project/user-quotas",
			getPath:      "/internal/org-project/user-quotas/{id}",
		},
		"workload-jobs.json": {
			ownerService: "workload-service",
			resource:     "workload-service:jobs",
			listPath:     "/internal/workload/jobs",
			listOnly:     true,
		},
	}

	seenResources := make(map[string]string, len(fixtures))
	for _, name := range fixtures {
		fixture := readOwnerReadFixture(t, name)
		assertValidOwnerReadFixture(t, name, fixture)
		route := wantResources[name]
		if got := ownerReadFixtureRouteFrom(fixture); got != route {
			t.Fatalf("%s route = %#v, want %#v", name, got, route)
		}
		seenResources[fixture.Resource] = fixture.OwnerService
	}

	wantSeen := map[string]string{
		"org-project-service:project_members": "org-project-service",
		"org-project-service:projects":        "org-project-service",
		"org-project-service:user_groups":     "org-project-service",
		"org-project-service:user_quotas":     "org-project-service",
		"workload-service:jobs":               "workload-service",
	}
	if !reflect.DeepEqual(seenResources, wantSeen) {
		t.Fatalf("fixture resource owners = %v, want %v", seenResources, wantSeen)
	}
}

func TestOwnerReadFixtureCompatibilityAllowsAdditiveFields(t *testing.T) {
	doc := readOwnerReadFixtureDocument(t, "org-project-projects.json")
	doc["new_top_level_field"] = "ignored by tolerant readers"
	records := doc["records"].([]any)
	first := records[0].(map[string]any)
	data := first["data"].(map[string]any)
	data["new_optional_snapshot"] = map[string]any{"safe_label": "owner-read-fixture"}

	encoded, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal mutated fixture: %v", err)
	}
	var fixture ownerReadContractFixture
	if err := json.Unmarshal(encoded, &fixture); err != nil {
		t.Fatalf("unmarshal mutated fixture: %v", err)
	}
	assertValidOwnerReadFixture(t, "org-project-projects.json", fixture)
	if _, ok := fixture.Records[0].Data["new_optional_snapshot"]; !ok {
		t.Fatalf("record additive field was not preserved")
	}
}

func TestOwnerReadFixturesRejectForbiddenRecordFields(t *testing.T) {
	cases := map[string]map[string]any{
		"internal_id": {
			"internal_id": "42",
		},
		"db_id_nested": {
			"record": map[string]any{"db_id": 42},
		},
		"access_token": {
			"access_token": "redacted",
		},
		"password": {
			"password": "redacted",
		},
	}

	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := readOwnerReadFixture(t, "org-project-projects.json")
			for key, value := range payload {
				fixture.Records[0].Data[key] = value
			}
			if err := validateOwnerReadFixture(fixture); err == nil {
				t.Fatal("validateOwnerReadFixture error = nil, want forbidden field")
			} else if !strings.Contains(err.Error(), "forbidden") {
				t.Fatalf("validateOwnerReadFixture error = %q, want forbidden key message", err.Error())
			}
		})
	}
}

type ownerReadContractFixture struct {
	SchemaVersion        int                         `json:"schema_version"`
	ContractName         string                      `json:"contract_name"`
	OwnerService         string                      `json:"owner_service"`
	ConsumerService      string                      `json:"consumer_service"`
	Resource             string                      `json:"resource"`
	Auth                 string                      `json:"auth"`
	ServiceKeyRequired   bool                        `json:"service_key_required"`
	ListPath             string                      `json:"list_path"`
	GetPath              string                      `json:"get_path,omitempty"`
	ListOnly             bool                        `json:"list_only"`
	KeyShape             string                      `json:"key_shape"`
	RequiredRecordFields []string                    `json:"required_record_fields"`
	Records              []Record[map[string]any]    `json:"records"`
	Compatibility        ownerReadCompatibilityBlock `json:"compatibility"`
}

type ownerReadCompatibilityBlock struct {
	AdditiveFields bool `json:"additive_fields"`
	TolerantReader bool `json:"tolerant_readers"`
}

type ownerReadFixtureRoute struct {
	ownerService string
	resource     string
	listPath     string
	getPath      string
	listOnly     bool
}

func ownerReadFixtureRouteFrom(fixture ownerReadContractFixture) ownerReadFixtureRoute {
	return ownerReadFixtureRoute{
		ownerService: fixture.OwnerService,
		resource:     fixture.Resource,
		listPath:     fixture.ListPath,
		getPath:      fixture.GetPath,
		listOnly:     fixture.ListOnly,
	}
}

func assertValidOwnerReadFixture(t *testing.T, name string, fixture ownerReadContractFixture) {
	t.Helper()
	if err := validateOwnerReadFixture(fixture); err != nil {
		t.Fatalf("%s is not a valid owner-read fixture: %v", name, err)
	}
}

func validateOwnerReadFixture(fixture ownerReadContractFixture) error {
	if err := validateOwnerReadRequiredMetadata(fixture); err != nil {
		return err
	}
	if err := validateOwnerReadRoute(fixture); err != nil {
		return err
	}
	if !fixture.Compatibility.AdditiveFields || !fixture.Compatibility.TolerantReader {
		return fmt.Errorf("owner-read fixture compatibility must allow additive fields and tolerant readers")
	}
	if len(fixture.RequiredRecordFields) == 0 {
		return fmt.Errorf("owner-read fixture required_record_fields is empty")
	}
	if err := validateOwnerReadRecords(fixture.Records, fixture.RequiredRecordFields); err != nil {
		return err
	}
	return nil
}

func validateOwnerReadRequiredMetadata(fixture ownerReadContractFixture) error {
	missing := make([]string, 0, 8)
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
	if fixture.Auth == "" {
		missing = append(missing, "auth")
	}
	if fixture.ListPath == "" {
		missing = append(missing, "list_path")
	}
	if fixture.KeyShape == "" {
		missing = append(missing, "key_shape")
	}
	if len(missing) > 0 {
		return fmt.Errorf("owner-read fixture missing required fields: %s", strings.Join(missing, ", "))
	}
	return nil
}

func validateOwnerReadRoute(fixture ownerReadContractFixture) error {
	if fixture.SchemaVersion != 1 {
		return fmt.Errorf("owner-read fixture schema_version = %d, want 1", fixture.SchemaVersion)
	}
	if fixture.Auth != "service_key" || !fixture.ServiceKeyRequired {
		return fmt.Errorf("owner-read fixture auth = %q service_key_required=%v, want service_key/true", fixture.Auth, fixture.ServiceKeyRequired)
	}
	if fixture.ConsumerService != "scheduler-quota-service" {
		return fmt.Errorf("owner-read fixture consumer_service = %q, want scheduler-quota-service", fixture.ConsumerService)
	}
	if !strings.HasPrefix(fixture.ListPath, "/internal/") {
		return fmt.Errorf("owner-read fixture list_path = %q, want internal path", fixture.ListPath)
	}
	if fixture.ListOnly && fixture.GetPath != "" {
		return fmt.Errorf("owner-read fixture list_only has get_path %q", fixture.GetPath)
	}
	if !fixture.ListOnly && fixture.GetPath == "" {
		return fmt.Errorf("owner-read fixture get_path is required for non-list-only contract")
	}
	return nil
}

func validateOwnerReadRecords(records []Record[map[string]any], requiredFields []string) error {
	if len(records) == 0 {
		return fmt.Errorf("owner-read fixture records is empty")
	}
	for index, record := range records {
		if err := validateOwnerReadRecord(index, record, requiredFields); err != nil {
			return err
		}
	}
	return nil
}

func validateOwnerReadRecord(index int, record Record[map[string]any], requiredFields []string) error {
	if record.ID == "" {
		return fmt.Errorf("owner-read fixture records[%d].id is empty", index)
	}
	if record.Version <= 0 {
		return fmt.Errorf("owner-read fixture records[%d].version = %d, want positive", index, record.Version)
	}
	if record.CreatedAt.IsZero() || record.UpdatedAt.IsZero() {
		return fmt.Errorf("owner-read fixture records[%d] missing timestamps", index)
	}
	if len(record.Data) == 0 {
		return fmt.Errorf("owner-read fixture records[%d].data is empty", index)
	}
	if dataID, ok := record.Data["id"].(string); !ok || dataID != record.ID {
		return fmt.Errorf("owner-read fixture records[%d].data.id = %#v, want %q", index, record.Data["id"], record.ID)
	}
	if err := validateOwnerReadRecordRequiredFields(index, record.Data, requiredFields); err != nil {
		return err
	}
	return validateOwnerReadRecordPayload(fmt.Sprintf("records[%d].data", index), record.Data)
}

func validateOwnerReadRecordRequiredFields(index int, data map[string]any, requiredFields []string) error {
	for _, field := range requiredFields {
		if text, ok := data[field].(string); ok && strings.TrimSpace(text) == "" {
			return fmt.Errorf("owner-read fixture records[%d].data.%s is empty", index, field)
		}
		if _, ok := data[field]; !ok {
			return fmt.Errorf("owner-read fixture records[%d].data missing %s", index, field)
		}
	}
	return nil
}

func validateOwnerReadRecordPayload(path string, value any) error {
	switch typed := value.(type) {
	case map[string]any:
		for key, item := range typed {
			fieldPath := path + "." + key
			if forbiddenEventEnvelopePayloadKey(key) {
				return fmt.Errorf("owner-read record key %q is forbidden", fieldPath)
			}
			if err := validateOwnerReadRecordPayload(fieldPath, item); err != nil {
				return err
			}
		}
	case []any:
		for i, item := range typed {
			if err := validateOwnerReadRecordPayload(fmt.Sprintf("%s[%d]", path, i), item); err != nil {
				return err
			}
		}
	}
	return nil
}

func ownerReadFixtureFiles(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(ownerReadFixtureDir())
	if err != nil {
		t.Fatalf("read owner-read fixtures: %v", err)
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

func readOwnerReadFixture(t *testing.T, name string) ownerReadContractFixture {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(ownerReadFixtureDir(), name))
	if err != nil {
		t.Fatalf("read owner-read fixture %s: %v", name, err)
	}
	var fixture ownerReadContractFixture
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatalf("unmarshal owner-read fixture %s: %v", name, err)
	}
	return fixture
}

func readOwnerReadFixtureDocument(t *testing.T, name string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(ownerReadFixtureDir(), name))
	if err != nil {
		t.Fatalf("read owner-read fixture %s: %v", name, err)
	}
	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatalf("unmarshal owner-read fixture document %s: %v", name, err)
	}
	return document
}

func ownerReadFixtureDir() string {
	return filepath.Join("fixtures", "owner-read", "v1")
}
