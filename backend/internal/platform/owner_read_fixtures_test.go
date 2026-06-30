package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
)

func TestOwnerReadFixturesMatchDomainReadContracts(t *testing.T) {
	fixtures := ownerReadPlatformFixtures(t)
	wantFiles := []string{
		"org-project-project-members.json",
		"org-project-projects.json",
		"org-project-user-groups.json",
		"org-project-user-quotas.json",
		"workload-jobs.json",
		"workload-org-project-project-members.json",
		"workload-org-project-projects.json",
	}
	if got := ownerReadPlatformFixtureNames(fixtures); !reflect.DeepEqual(got, wantFiles) {
		t.Fatalf("owner-read fixture files = %v, want %v", got, wantFiles)
	}

	gotContracts := make([]string, 0, len(fixtures))
	for _, fixture := range fixtures {
		contract, ok := domainReadContracts[fixture.Resource]
		if !ok {
			t.Fatalf("%s resource %q has no domainReadContracts entry", fixture.FileName, fixture.Resource)
		}
		if owner := resourceOwner(fixture.Resource); owner != fixture.OwnerService {
			t.Fatalf("%s owner = %q from resource, want %q", fixture.FileName, owner, fixture.OwnerService)
		}
		if contract.listPath != fixture.ListPath {
			t.Fatalf("%s list_path = %q, domain contract has %q", fixture.FileName, fixture.ListPath, contract.listPath)
		}
		if contract.getPath != fixture.GetPath {
			t.Fatalf("%s get_path = %q, domain contract has %q", fixture.FileName, fixture.GetPath, contract.getPath)
		}
		if fixture.ListOnly != (contract.getPath == "") {
			t.Fatalf("%s list_only = %v, domain get_path = %q", fixture.FileName, fixture.ListOnly, contract.getPath)
		}
		gotContracts = append(gotContracts, fixture.ConsumerService+" -> "+fixture.Resource)
	}
	sort.Strings(gotContracts)
	wantContracts := []string{
		"scheduler-quota-service -> org-project-service:project_members",
		"scheduler-quota-service -> org-project-service:projects",
		"scheduler-quota-service -> org-project-service:user_groups",
		"scheduler-quota-service -> org-project-service:user_quotas",
		"scheduler-quota-service -> workload-service:jobs",
		"workload-service -> org-project-service:project_members",
		"workload-service -> org-project-service:projects",
	}
	if !reflect.DeepEqual(gotContracts, wantContracts) {
		t.Fatalf("owner-read fixture contracts = %v, want %v", gotContracts, wantContracts)
	}
}

func TestRemoteServiceReaderConsumesOwnerReadFixtures(t *testing.T) {
	fixtures := ownerReadPlatformFixtures(t)
	serviceKey := "svc-owner-read-fixtures"
	var calls []string
	server := newOwnerReadFixtureServer(fixtures, serviceKey, &calls)
	t.Cleanup(server.Close)

	reader := newOwnerReadFixtureReader(server.URL, serviceKey)

	for _, fixture := range fixtures {
		assertRemoteReaderListConsumesFixture(t, reader, fixture)
		if fixture.ListOnly {
			assertRemoteReaderRejectsListOnlyGet(t, reader, fixture)
			continue
		}
		assertRemoteReaderGetConsumesFixture(t, reader, fixture)
	}

	if want := len(fixtures)*2 - 1; len(calls) != want {
		t.Fatalf("remote reader calls = %v, want %d list/get calls", calls, want)
	}

	assertRemoteReaderRejectsBadServiceKey(t, server.URL, serviceKey)
}

type ownerReadPlatformFixture struct {
	FileName           string
	SchemaVersion      int                                `json:"schema_version"`
	OwnerService       string                             `json:"owner_service"`
	ConsumerService    string                             `json:"consumer_service"`
	Resource           string                             `json:"resource"`
	Auth               string                             `json:"auth"`
	ServiceKeyRequired bool                               `json:"service_key_required"`
	ListPath           string                             `json:"list_path"`
	GetPath            string                             `json:"get_path,omitempty"`
	ListOnly           bool                               `json:"list_only"`
	Records            []contracts.Record[map[string]any] `json:"records"`
}

func (fixture ownerReadPlatformFixture) payloadForPath(path string) (any, bool) {
	if path == fixture.ListPath {
		return fixture.Records, true
	}
	if fixture.GetPath == "" || len(fixture.Records) == 0 {
		return nil, false
	}
	getPath := strings.ReplaceAll(fixture.GetPath, "{id}", fixture.Records[0].ID)
	if path == getPath {
		return fixture.Records[0], true
	}
	return nil, false
}

func newOwnerReadFixtureServer(fixtures []ownerReadPlatformFixture, serviceKey string, calls *[]string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*calls = append(*calls, r.URL.Path)
		if r.Header.Get(serviceKeyHeader) != serviceKey {
			WriteError(w, r, http.StatusUnauthorized, "unauthorized", "service authentication is required")
			return
		}
		for _, fixture := range fixtures {
			if payload, ok := fixture.payloadForPath(r.URL.Path); ok {
				WriteJSON(w, r, http.StatusOK, payload)
				return
			}
		}
		WriteError(w, r, http.StatusNotFound, "not_found", "unexpected owner-read path")
	}))
}

func newOwnerReadFixtureReader(serverURL, serviceKey string) *RemoteServiceReader {
	return NewRemoteServiceReader(Config{
		ServiceURLs: map[string]string{
			"org-project-service": serverURL,
			"workload-service":    serverURL,
		},
		ServiceAPIKey: serviceKey,
	})
}

func assertRemoteReaderListConsumesFixture(t *testing.T, reader *RemoteServiceReader, fixture ownerReadPlatformFixture) {
	t.Helper()
	records, err := reader.List(context.Background(), fixture.Resource)
	if err != nil {
		t.Fatalf("List(%s): %v", fixture.Resource, err)
	}
	if len(records) != 1 || records[0].ID != fixture.Records[0].ID {
		t.Fatalf("List(%s) = %#v, want fixture record %q", fixture.Resource, records, fixture.Records[0].ID)
	}
}

func assertRemoteReaderRejectsListOnlyGet(t *testing.T, reader *RemoteServiceReader, fixture ownerReadPlatformFixture) {
	t.Helper()
	if _, ok, err := reader.Get(context.Background(), fixture.Resource, fixture.Records[0].ID); err == nil || ok {
		t.Fatalf("Get(%s) ok=%v err=%v, want list-only failure", fixture.Resource, ok, err)
	}
}

func assertRemoteReaderGetConsumesFixture(t *testing.T, reader *RemoteServiceReader, fixture ownerReadPlatformFixture) {
	t.Helper()
	record, ok, err := reader.Get(context.Background(), fixture.Resource, fixture.Records[0].ID)
	if err != nil || !ok || record.ID != fixture.Records[0].ID {
		t.Fatalf("Get(%s) = %#v ok=%v err=%v, want fixture record", fixture.Resource, record, ok, err)
	}
}

func assertRemoteReaderRejectsBadServiceKey(t *testing.T, serverURL, serviceKey string) {
	t.Helper()
	badReader := NewRemoteServiceReader(Config{
		ServiceURLs:   map[string]string{"org-project-service": serverURL},
		ServiceAPIKey: serviceKey + "-wrong",
	})
	if _, err := badReader.List(context.Background(), "org-project-service:projects"); err == nil {
		t.Fatal("List with bad service key error = nil, want unauthorized error")
	}
}

func ownerReadPlatformFixtures(t *testing.T) []ownerReadPlatformFixture {
	t.Helper()
	entries, err := os.ReadDir(ownerReadPlatformFixtureDir())
	if err != nil {
		t.Fatalf("read owner-read fixtures: %v", err)
	}
	fixtures := make([]ownerReadPlatformFixture, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(ownerReadPlatformFixtureDir(), entry.Name()))
		if err != nil {
			t.Fatalf("read owner-read fixture %s: %v", entry.Name(), err)
		}
		var fixture ownerReadPlatformFixture
		if err := json.Unmarshal(raw, &fixture); err != nil {
			t.Fatalf("unmarshal owner-read fixture %s: %v", entry.Name(), err)
		}
		fixture.FileName = entry.Name()
		if err := validateOwnerReadPlatformFixture(fixture); err != nil {
			t.Fatalf("%s is not a valid owner-read platform fixture: %v", entry.Name(), err)
		}
		fixtures = append(fixtures, fixture)
	}
	sort.Slice(fixtures, func(i, j int) bool { return fixtures[i].FileName < fixtures[j].FileName })
	return fixtures
}

func validateOwnerReadPlatformFixture(fixture ownerReadPlatformFixture) error {
	if fixture.SchemaVersion != 1 {
		return fmt.Errorf("schema_version = %d, want 1", fixture.SchemaVersion)
	}
	if fixture.Auth != "service_key" || !fixture.ServiceKeyRequired {
		return fmt.Errorf("auth = %q service_key_required=%v, want service_key/true", fixture.Auth, fixture.ServiceKeyRequired)
	}
	if fixture.OwnerService == "" || fixture.ConsumerService == "" || fixture.Resource == "" || fixture.ListPath == "" {
		return fmt.Errorf("owner_service, consumer_service, resource, and list_path are required")
	}
	if len(fixture.Records) != 1 {
		return fmt.Errorf("records length = %d, want 1", len(fixture.Records))
	}
	if fixture.Records[0].ID == "" {
		return fmt.Errorf("record id is required")
	}
	return nil
}

func ownerReadPlatformFixtureNames(fixtures []ownerReadPlatformFixture) []string {
	names := make([]string, 0, len(fixtures))
	for _, fixture := range fixtures {
		names = append(names, fixture.FileName)
	}
	return names
}

func ownerReadPlatformFixtureDir() string {
	return filepath.Join("..", "contracts", "fixtures", "owner-read", "v1")
}
