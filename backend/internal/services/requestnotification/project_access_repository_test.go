package requestnotification

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

func TestProjectAccessRepositoryLifecycle(t *testing.T) {
	ctx := context.Background()
	store := platform.NewStore()
	repo := projectAccessRepoFromStore(store, platform.Config{ServiceName: serviceName})

	requireNoProjectAccessError(t, repo.UpsertProject(ctx, map[string]any{"project_id": "P1", "owner_id": "G1"}))
	requireNoProjectAccessError(t, repo.UpsertProjectMember(ctx, map[string]any{"project_id": "P1", "user_id": "U1", "role": "member"}))
	requireNoProjectAccessError(t, repo.UpsertUserGroup(ctx, map[string]any{"user_id": "U1", "group_id": "G1", "role": "member"}))

	if got := repo.ListProjects(ctx); len(got) != 1 || got[0]["id"] != "P1" {
		t.Fatalf("ListProjects = %#v, want P1", got)
	}
	if got := repo.ListProjectMembers(ctx); len(got) != 1 || got[0]["id"] != "P1:U1" {
		t.Fatalf("ListProjectMembers = %#v, want P1:U1", got)
	}
	if got := repo.ListUserGroups(ctx); len(got) != 1 || got[0]["id"] != "U1:G1" {
		t.Fatalf("ListUserGroups = %#v, want U1:G1", got)
	}

	if repo.DeleteUserGroup(ctx, map[string]any{"user_id": "U1", "group_id": "G1", "deleted": false}) {
		t.Fatal("DeleteUserGroup deleted=false = true, want no-op")
	}
	if !repo.DeleteUserGroup(ctx, map[string]any{"user_id": "U1", "group_id": "G1", "deleted": true}) {
		t.Fatal("DeleteUserGroup deleted=true = false, want delete")
	}
	if got := repo.ListUserGroups(ctx); len(got) != 0 {
		t.Fatalf("ListUserGroups after delete = %#v, want empty", got)
	}
}

func TestProjectAccessRepositorySourceFallbackGating(t *testing.T) {
	ctx := context.Background()
	store := platform.NewStore()
	createProjectAccessRecord(t, store, orgProjectsResource, map[string]any{"id": "P1", "name": "source"})
	createProjectAccessRecord(t, store, orgProjectMembersResource, map[string]any{"id": "P1:U1", "project_id": "P1", "user_id": "U1"})
	createProjectAccessRecord(t, store, orgUserGroupsResource, map[string]any{"id": "U1:G1", "user_id": "U1", "group_id": "G1"})

	isolated := projectAccessRepoFromStore(store, platform.Config{ServiceName: serviceName})
	if got := isolated.ListProjects(ctx); len(got) != 0 {
		t.Fatalf("isolated source projects = %#v, want none", got)
	}
	if got := isolated.ListProjectMembers(ctx); len(got) != 0 {
		t.Fatalf("isolated source members = %#v, want none", got)
	}
	if got := isolated.ListUserGroups(ctx); len(got) != 0 {
		t.Fatalf("isolated source user groups = %#v, want none", got)
	}

	ownerHosted := projectAccessRepoFromStore(store, platform.Config{ServiceName: "org-project-service"})
	if got := ownerHosted.ListProjects(ctx); len(got) != 1 || got[0]["name"] != "source" {
		t.Fatalf("owner-hosted source projects = %#v, want source", got)
	}

	cohosted := projectAccessRepoFromStore(store, platform.Config{ServiceName: "all"})
	requireNoProjectAccessError(t, cohosted.UpsertProject(ctx, map[string]any{"id": "P1", "name": "local"}))
	if got := cohosted.ListProjects(ctx); len(got) != 1 || got[0]["name"] != "local" {
		t.Fatalf("cohosted local-over-source projects = %#v, want local override", got)
	}
	if got := cohosted.ListProjectMembers(ctx); len(got) != 1 || got[0]["id"] != "P1:U1" {
		t.Fatalf("cohosted source members = %#v, want P1:U1", got)
	}
	if got := cohosted.ListUserGroups(ctx); len(got) != 1 || got[0]["id"] != "U1:G1" {
		t.Fatalf("cohosted source user groups = %#v, want U1:G1", got)
	}
}

func TestProjectAccessRepositoryCloneIsolation(t *testing.T) {
	ctx := context.Background()
	store := platform.NewStore()
	repo := projectAccessRepoFromStore(store, platform.Config{ServiceName: serviceName})
	input := map[string]any{"id": "P1", "name": "original"}
	requireNoProjectAccessError(t, repo.UpsertProject(ctx, input))

	input["name"] = "mutated input"
	if got := repo.ListProjects(ctx); len(got) != 1 || got[0]["name"] != "original" {
		t.Fatalf("stored project after caller mutation = %#v, want original", got)
	}
	rows := repo.ListProjects(ctx)
	rows[0]["name"] = "mutated output"
	if got := repo.ListProjects(ctx); got[0]["name"] != "original" {
		t.Fatalf("stored project after listed row mutation = %#v, want original", got)
	}
}

func TestProjectAccessRepositoryConflictFallbackAndNilStore(t *testing.T) {
	ctx := context.Background()
	conflictStore := &projectAccessConflictStore{createErr: platform.CreateConflictError{Resource: projectAccessProjects, ID: "P1"}}
	repo := projectAccessRepoFromStore(conflictStore, platform.Config{ServiceName: serviceName})
	requireNoProjectAccessError(t, repo.UpsertProject(ctx, map[string]any{"id": "P1"}))
	if conflictStore.createCalls != 1 || conflictStore.updateCalls != 2 {
		t.Fatalf("conflict calls create=%d update=%d, want 1/2", conflictStore.createCalls, conflictStore.updateCalls)
	}

	failingStore := &projectAccessConflictStore{createErr: errors.New("store unavailable")}
	repo = projectAccessRepoFromStore(failingStore, platform.Config{ServiceName: serviceName})
	if err := repo.UpsertProject(ctx, map[string]any{"id": "P1"}); err == nil {
		t.Fatal("UpsertProject store error = nil, want error")
	}

	nilRepo := projectAccessRepoFromStore(nil, platform.Config{ServiceName: serviceName})
	requireNoProjectAccessError(t, nilRepo.UpsertProject(ctx, map[string]any{}))
	if err := nilRepo.UpsertProject(ctx, map[string]any{"id": "P1"}); err == nil {
		t.Fatal("UpsertProject nil store err = nil, want fail-closed error")
	}
	if got := nilRepo.ListProjects(ctx); len(got) != 0 {
		t.Fatalf("ListProjects nil store = %#v, want empty", got)
	}
	if nilRepo.DeleteProject(ctx, map[string]any{"id": "P1"}) {
		t.Fatal("DeleteProject nil store = true, want false")
	}
}

func TestProjectAccessRepositoryProjectionDriftDetectsMissingOrphanStaleCleanAndSorts(t *testing.T) {
	ctx := context.Background()
	store := platform.NewStore()
	repo := projectAccessRepoFromStore(store, platform.Config{ServiceName: "all"})

	createProjectAccessRecord(t, store, orgUserGroupsResource, map[string]any{"id": "U-missing-group:G-missing-group", "user_id": "U-missing-group", "group_id": "G-missing-group", "role": "source"})
	createProjectAccessRecord(t, store, orgProjectsResource, map[string]any{"id": "P-missing-project", "project_id": "P-missing-project", "name": "source only"})
	createProjectAccessRecord(t, store, orgProjectMembersResource, map[string]any{"id": "P-clean-member:U-clean-member", "project_id": "P-clean-member", "user_id": "U-clean-member", "role": "clean"})
	createProjectAccessRecord(t, store, orgProjectMembersResource, map[string]any{"id": "P-stale-member:U-stale-member", "project_id": "P-stale-member", "user_id": "U-stale-member", "role": "source"})
	createProjectAccessRecord(t, store, orgUserGroupsResource, map[string]any{"id": "U-stale-group:G-stale-group", "user_id": "U-stale-group", "group_id": "G-stale-group", "role": "source"})
	createProjectAccessRecord(t, store, orgProjectMembersResource, map[string]any{"id": "P-missing-member:U-missing-member", "project_id": "P-missing-member", "user_id": "U-missing-member", "role": "source"})
	createProjectAccessRecord(t, store, orgUserGroupsResource, map[string]any{"id": "U-clean-group:G-clean-group", "user_id": "U-clean-group", "group_id": "G-clean-group", "role": "clean"})
	createProjectAccessRecord(t, store, orgProjectMembersResource, map[string]any{"id": "skip-source-member"})
	clearProjectAccessRecordID(t, store, orgProjectMembersResource, "skip-source-member")

	requireNoProjectAccessError(t, repo.UpsertUserGroup(ctx, map[string]any{"user_id": "U-orphan-group", "group_id": "G-orphan-group", "role": "local"}))
	requireNoProjectAccessError(t, repo.UpsertProjectMember(ctx, map[string]any{"project_id": "P-orphan-member", "user_id": "U-orphan-member", "role": "local"}))
	requireNoProjectAccessError(t, repo.UpsertUserGroup(ctx, map[string]any{"user_id": "U-clean-group", "group_id": "G-clean-group", "role": "clean"}))
	requireNoProjectAccessError(t, repo.UpsertProjectMember(ctx, map[string]any{"project_id": "P-stale-member", "user_id": "U-stale-member", "role": "local"}))
	requireNoProjectAccessError(t, repo.UpsertUserGroup(ctx, map[string]any{"user_id": "U-stale-group", "group_id": "G-stale-group", "role": "local"}))
	requireNoProjectAccessError(t, repo.UpsertProjectMember(ctx, map[string]any{"project_id": "P-clean-member", "user_id": "U-clean-member", "role": "clean"}))
	createProjectAccessRecord(t, store, projectAccessMembers, map[string]any{"id": "skip-local-member"})
	clearProjectAccessRecordID(t, store, projectAccessMembers, "skip-local-member")

	report, err := repo.projectionDrift(ctx)
	if err != nil {
		t.Fatalf("projectionDrift: %v", err)
	}
	assertProjectAccessProjectionDriftFindings(t, "missing", report.Missing, []projectAccessProjectionDriftFinding{
		{SourceResource: orgProjectMembersResource, LocalResource: projectAccessMembers, ID: "P-missing-member:U-missing-member"},
		{SourceResource: orgProjectsResource, LocalResource: projectAccessProjects, ID: "P-missing-project"},
		{SourceResource: orgUserGroupsResource, LocalResource: projectAccessUserGroups, ID: "U-missing-group:G-missing-group"},
	})
	assertProjectAccessProjectionDriftFindings(t, "orphan", report.Orphan, []projectAccessProjectionDriftFinding{
		{SourceResource: orgProjectMembersResource, LocalResource: projectAccessMembers, ID: "P-orphan-member:U-orphan-member"},
		{SourceResource: orgUserGroupsResource, LocalResource: projectAccessUserGroups, ID: "U-orphan-group:G-orphan-group"},
	})
	assertProjectAccessProjectionDriftFindings(t, "stale", report.Stale, []projectAccessProjectionDriftFinding{
		{SourceResource: orgProjectMembersResource, LocalResource: projectAccessMembers, ID: "P-stale-member:U-stale-member"},
		{SourceResource: orgUserGroupsResource, LocalResource: projectAccessUserGroups, ID: "U-stale-group:G-stale-group"},
	})
}

func TestProjectAccessRepositoryProjectionDriftNormalizesCanonicalID(t *testing.T) {
	ctx := context.Background()
	store := platform.NewStore()
	repo := projectAccessRepoFromStore(store, platform.Config{ServiceName: "all"})

	createProjectAccessRecord(t, store, orgProjectMembersResource, map[string]any{
		"id":         "source-member-row",
		"project_id": "P-normalized",
		"user_id":    "U-normalized",
		"role":       "member",
	})
	clearProjectAccessRecordID(t, store, orgProjectMembersResource, "source-member-row")
	requireNoProjectAccessError(t, repo.UpsertProjectMember(ctx, map[string]any{
		"id":         "P-normalized:U-normalized",
		"project_id": "P-normalized",
		"user_id":    "U-normalized",
		"role":       "member",
	}))

	report, err := repo.projectionDrift(ctx)
	if err != nil {
		t.Fatalf("projectionDrift: %v", err)
	}
	assertProjectAccessProjectionDriftFindings(t, "missing", report.Missing, nil)
	assertProjectAccessProjectionDriftFindings(t, "orphan", report.Orphan, nil)
	assertProjectAccessProjectionDriftFindings(t, "stale", report.Stale, nil)
}

func TestProjectAccessRepositoryProjectionDriftNilStoreFailsClosed(t *testing.T) {
	repo := projectAccessRepoFromStore(nil, platform.Config{ServiceName: serviceName})
	if _, err := repo.projectionDrift(context.Background()); !errors.Is(err, errProjectAccessRepositoryUnavailable) {
		t.Fatalf("projectionDrift nil store error = %v, want %v", err, errProjectAccessRepositoryUnavailable)
	}
}

func TestProjectAccessRepositoryProjectionDriftPairsCoverExpectedResources(t *testing.T) {
	want := map[string]string{
		projectAccessProjects:   orgProjectsResource,
		projectAccessMembers:    orgProjectMembersResource,
		projectAccessUserGroups: orgUserGroupsResource,
	}
	got := map[string]string{}
	for _, pair := range projectAccessProjectionDriftPairs {
		if pair.idFn == nil {
			t.Fatalf("projection drift pair %s -> %s has nil id function", pair.sourceResource, pair.localResource)
		}
		got[pair.localResource] = pair.sourceResource
	}
	if len(projectAccessProjectionDriftPairs) != len(want) || !reflect.DeepEqual(got, want) {
		t.Fatalf("projection drift pairs = %#v, want %#v", got, want)
	}
}

func TestProjectAccessRepositorySourceGuard(t *testing.T) {
	dir := projectAccessRepositoryTestDir(t)
	guard := newProjectAccessSourceGuard()
	violations, err := collectProjectAccessSourceGuardViolations(dir, guard)
	if err != nil {
		t.Fatal(err)
	}
	for _, violation := range violations {
		t.Error(violation)
	}
}

func createProjectAccessRecord(t *testing.T, store platform.RecordStore, resource string, row map[string]any) {
	t.Helper()
	if _, err := store.Create(context.Background(), resource, row); err != nil {
		t.Fatal(err)
	}
}

func clearProjectAccessRecordID(t *testing.T, store platform.RecordStore, resource, recordID string) {
	t.Helper()
	if _, ok := store.Update(context.Background(), resource, recordID, map[string]any{"id": ""}); !ok {
		t.Fatalf("clear %s/%s id: record not found", resource, recordID)
	}
}

func assertProjectAccessProjectionDriftFindings(t *testing.T, label string, got, want []projectAccessProjectionDriftFinding) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s findings = %#v, want %#v", label, got, want)
	}
}

func requireNoProjectAccessError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

type projectAccessConflictStore struct {
	createErr   error
	createCalls int
	updateCalls int
}

func (s *projectAccessConflictStore) Create(context.Context, string, map[string]any) (contracts.Record[map[string]any], error) {
	s.createCalls++
	return contracts.Record[map[string]any]{}, s.createErr
}

func (s *projectAccessConflictStore) Get(context.Context, string, string) (contracts.Record[map[string]any], bool) {
	return contracts.Record[map[string]any]{}, false
}

func (s *projectAccessConflictStore) List(context.Context, string) []contracts.Record[map[string]any] {
	return nil
}

func (s *projectAccessConflictStore) Update(_ context.Context, _ string, id string, data map[string]any) (contracts.Record[map[string]any], bool) {
	s.updateCalls++
	if s.updateCalls == 1 {
		return contracts.Record[map[string]any]{}, false
	}
	return contracts.Record[map[string]any]{ID: id, Data: data}, true
}

func (s *projectAccessConflictStore) Delete(context.Context, string, string) bool {
	return false
}

func (s *projectAccessConflictStore) NextID(string, string, int, int) string {
	return ""
}

func projectAccessRepositoryTestDir(t *testing.T) string {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Dir(currentFile)
}

type projectAccessSourceGuard struct {
	afterStore  *regexp.Regexp
	beforeStore *regexp.Regexp
}

func newProjectAccessSourceGuard() projectAccessSourceGuard {
	owned := `(projectAccessProjects|projectAccessMembers|projectAccessUserGroups|request-notification-service:(?:project_access_projects|project_access_members|project_access_user_groups)|":(?:project_access_projects|project_access_members|project_access_user_groups)")`
	storeCall := `(?:Store|store)\s*\.\s*(?:Get|List|Create|Update|Delete|NextID)`
	return projectAccessSourceGuard{
		afterStore:  regexp.MustCompile(storeCall + `(?s:[^\n;]*)\b` + owned + `\b`),
		beforeStore: regexp.MustCompile(`\b` + owned + `\b(?s:[^\n;]*)` + storeCall),
	}
}

func collectProjectAccessSourceGuardViolations(dir string, guard projectAccessSourceGuard) ([]string, error) {
	var violations []string
	err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if skipProjectAccessSourceGuardFile(path, entry) {
			return nil
		}
		fileViolations, err := projectAccessSourceGuardViolations(path, guard)
		if err != nil {
			return err
		}
		violations = append(violations, fileViolations...)
		return nil
	})
	return violations, err
}

func skipProjectAccessSourceGuardFile(path string, entry os.DirEntry) bool {
	if entry.IsDir() || !strings.HasSuffix(path, ".go") {
		return true
	}
	name := filepath.Base(path)
	return strings.HasSuffix(name, "_test.go") || name == "project_access_repository.go"
}

func projectAccessSourceGuardViolations(path string, guard projectAccessSourceGuard) ([]string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if guard.afterStore.Match(content) || guard.beforeStore.Match(content) {
		return []string{path + " directly accesses request-notification project access read models through RecordStore; use projectAccessRepository"}, nil
	}
	return nil, nil
}
