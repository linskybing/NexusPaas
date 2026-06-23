package ideworkspace

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

func TestIDEProjectionRepositoryLifecycle(t *testing.T) {
	ctx := context.Background()
	store := platform.NewStore()
	repo := ideProjectionRepoFromStore(store, platform.Config{ServiceName: serviceName})

	requireNoIDEProjectionError(t, repo.UpsertIdentityUser(ctx, map[string]any{"user_id": "U1", "role_id": "R1"}))
	requireNoIDEProjectionError(t, repo.UpsertIdentityRole(ctx, map[string]any{"role_id": "R1", "admin_panel": true}))
	requireNoIDEProjectionError(t, repo.UpsertPolicyRole(ctx, map[string]any{"role_id": "PR1", "admin_panel": true}))
	requireNoIDEProjectionError(t, repo.UpsertProject(ctx, map[string]any{"p_id": "P1", "allow_run_as_root": true}))
	requireNoIDEProjectionError(t, repo.UpsertProjectMember(ctx, map[string]any{"project_id": "P1", "user_id": "U1", "role": "manager"}))
	requireNoIDEProjectionError(t, repo.UpsertUserGroup(ctx, map[string]any{"user_id": "U1", "group_id": "G1"}))

	if got := repo.ListIdentityUsers(ctx); len(got) != 1 || got[0].ID != "U1" || got[0].Data["id"] != "U1" {
		t.Fatalf("identity users = %#v, want U1", got)
	}
	if got := repo.ListIdentityRoles(ctx); len(got) != 1 || got[0].ID != "R1" {
		t.Fatalf("identity roles = %#v, want R1", got)
	}
	if got := repo.ListPolicyRoles(ctx); len(got) != 1 || got[0].ID != "PR1" {
		t.Fatalf("policy roles = %#v, want PR1", got)
	}
	if got := repo.ListProjects(ctx); len(got) != 1 || got[0].ID != "P1" {
		t.Fatalf("projects = %#v, want P1", got)
	}
	if got := repo.ListProjectMembers(ctx); len(got) != 1 || got[0].ID != "P1:U1" {
		t.Fatalf("project members = %#v, want P1:U1", got)
	}
	if got := repo.ListUserGroups(ctx); len(got) != 1 || got[0].ID != "U1:G1" {
		t.Fatalf("user groups = %#v, want U1:G1", got)
	}

	if repo.DeleteUserGroup(ctx, map[string]any{"user_id": "U1", "group_id": "G1", "deleted": false}) {
		t.Fatal("DeleteUserGroup deleted=false = true, want no-op")
	}
	if !repo.DeleteUserGroup(ctx, map[string]any{"user_id": "U1", "group_id": "G1", "deleted": true}) {
		t.Fatal("DeleteUserGroup deleted=true = false, want delete")
	}
	if got := repo.ListUserGroups(ctx); len(got) != 0 {
		t.Fatalf("user groups after delete = %#v, want empty", got)
	}
}

func TestIDEProjectionRepositorySourceFallbackGating(t *testing.T) {
	ctx := context.Background()
	store := platform.NewStore()
	createIDEProjectionRecord(t, store, identityUsersResource, map[string]any{"id": "U1", "role_id": "source"})
	createIDEProjectionRecord(t, store, orgProjectsResource, map[string]any{"id": "P1", "project_name": "source"})
	createIDEProjectionRecord(t, store, authorizationRolesResource, map[string]any{"id": "ROLE1", "admin_panel": true})

	isolated := ideProjectionRepoFromStore(store, platform.Config{ServiceName: serviceName})
	if got := isolated.ListIdentityUsers(ctx); len(got) != 0 {
		t.Fatalf("isolated identity source rows = %#v, want none", got)
	}
	if got := isolated.ListProjects(ctx); len(got) != 0 {
		t.Fatalf("isolated project source rows = %#v, want none", got)
	}

	identityHosted := ideProjectionRepoFromStore(store, platform.Config{ServiceName: "identity-service"})
	if got := identityHosted.ListIdentityUsers(ctx); len(got) != 1 || got[0].Data["role_id"] != "source" {
		t.Fatalf("identity-hosted source rows = %#v, want identity source", got)
	}
	if got := identityHosted.ListProjects(ctx); len(got) != 0 {
		t.Fatalf("identity-hosted project rows = %#v, want none", got)
	}

	cohosted := ideProjectionRepoFromStore(store, platform.Config{ServiceName: "all"})
	requireNoIDEProjectionError(t, cohosted.UpsertIdentityUser(ctx, map[string]any{"id": "U1", "role_id": "local"}))
	if got := cohosted.ListIdentityUsers(ctx); len(got) != 1 || got[0].Data["role_id"] != "local" {
		t.Fatalf("cohosted merged identity rows = %#v, want local override", got)
	}
	if got := cohosted.ListProjects(ctx); len(got) != 1 || got[0].Data["project_name"] != "source" {
		t.Fatalf("cohosted project source rows = %#v, want source project", got)
	}
	if got := cohosted.ListPolicyRoles(ctx); len(got) != 1 || got[0].ID != "ROLE1" {
		t.Fatalf("cohosted policy role source rows = %#v, want ROLE1", got)
	}
}

func TestIDEProjectionRepositoryProjectionDriftDetectsMissingOrphanStaleAndSorts(t *testing.T) {
	ctx := context.Background()
	store := platform.NewStore()
	repo := ideProjectionRepoFromStore(store, platform.Config{ServiceName: "all"})

	createIDEProjectionRecord(t, store, orgProjectsResource, map[string]any{"id": "P5", "project_id": "P5", "allow_run_as_root": true})
	createIDEProjectionRecord(t, store, orgProjectsResource, map[string]any{"id": "P1", "project_id": "P1", "allow_run_as_root": false})
	createIDEProjectionRecord(t, store, orgProjectsResource, map[string]any{"id": "P3", "project_id": "P3", "allow_run_as_root": true})
	createIDEProjectionRecord(t, store, ideProjectsResource, map[string]any{"id": "P2", "project_id": "P2", "allow_run_as_root": false})
	createIDEProjectionRecord(t, store, ideProjectsResource, map[string]any{"id": "P5", "project_id": "P5", "allow_run_as_root": false})
	createIDEProjectionRecord(t, store, ideProjectsResource, map[string]any{"id": "P1", "project_id": "P1", "allow_run_as_root": false})
	createIDEProjectionRecord(t, store, identityUsersResource, map[string]any{"id": "U5", "role_id": "source"})
	createIDEProjectionRecord(t, store, identityUsersResource, map[string]any{"id": "U1", "role_id": "R1"})
	createIDEProjectionRecord(t, store, identityUsersResource, map[string]any{"id": "U9", "role_id": "R9"})
	createIDEProjectionRecord(t, store, ideIdentityUsersResource, map[string]any{"id": "U2", "role_id": "orphan"})
	createIDEProjectionRecord(t, store, ideIdentityUsersResource, map[string]any{"id": "U5", "role_id": "local"})
	createIDEProjectionRecord(t, store, ideIdentityUsersResource, map[string]any{"id": "U1", "role_id": "R1"})

	report, err := repo.projectionDrift(ctx)
	if err != nil {
		t.Fatalf("projectionDrift: %v", err)
	}
	assertIDEProjectionDriftFindings(t, "missing", report.Missing,
		ideProjectionDriftFinding{SourceResource: identityUsersResource, LocalResource: ideIdentityUsersResource, ID: "U9"},
		ideProjectionDriftFinding{SourceResource: orgProjectsResource, LocalResource: ideProjectsResource, ID: "P3"},
	)
	assertIDEProjectionDriftFindings(t, "orphan", report.Orphan,
		ideProjectionDriftFinding{SourceResource: identityUsersResource, LocalResource: ideIdentityUsersResource, ID: "U2"},
		ideProjectionDriftFinding{SourceResource: orgProjectsResource, LocalResource: ideProjectsResource, ID: "P2"},
	)
	assertIDEProjectionDriftFindings(t, "stale", report.Stale,
		ideProjectionDriftFinding{SourceResource: identityUsersResource, LocalResource: ideIdentityUsersResource, ID: "U5"},
		ideProjectionDriftFinding{SourceResource: orgProjectsResource, LocalResource: ideProjectsResource, ID: "P5"},
	)
}

func TestIDEProjectionRepositoryProjectionDriftNormalizesCanonicalID(t *testing.T) {
	ctx := context.Background()
	store := platform.NewStore()
	repo := ideProjectionRepoFromStore(store, platform.Config{ServiceName: "all"})

	createIDEProjectionRecord(t, store, identityUsersResource, map[string]any{"id": " U-normalized ", "role_id": "R1"})
	createIDEProjectionRecord(t, store, ideIdentityUsersResource, map[string]any{"id": "U-normalized", "role_id": "R1"})

	report, err := repo.projectionDrift(ctx)
	if err != nil {
		t.Fatalf("projectionDrift: %v", err)
	}
	assertIDEProjectionDriftFindings(t, "missing", report.Missing)
	assertIDEProjectionDriftFindings(t, "orphan", report.Orphan)
	assertIDEProjectionDriftFindings(t, "stale", report.Stale)
}

func TestIDEProjectionRepositoryProjectionDriftNilStoreFailsClosed(t *testing.T) {
	repo := ideProjectionRepoFromStore(nil, platform.Config{ServiceName: serviceName})
	if _, err := repo.projectionDrift(context.Background()); !errors.Is(err, errIDEProjectionRepositoryUnavailable) {
		t.Fatalf("projectionDrift nil store error = %v, want %v", err, errIDEProjectionRepositoryUnavailable)
	}
}

func TestIDEProjectionRepositoryProjectionDriftPairsCoverExpectedResources(t *testing.T) {
	want := map[string]struct {
		sourceResource string
		id             string
	}{
		ideIdentityUsersResource:  {sourceResource: identityUsersResource, id: "U-map"},
		ideIdentityRolesResource:  {sourceResource: identityRolesResource, id: "R-map"},
		idePolicyRolesResource:    {sourceResource: authorizationRolesResource, id: "PR-map"},
		ideProjectsResource:       {sourceResource: orgProjectsResource, id: "P-map"},
		ideProjectMembersResource: {sourceResource: orgProjectMembersResource, id: "P-map:U-map"},
		ideUserGroupsResource:     {sourceResource: orgUserGroupsResource, id: "U-map:G-map"},
	}
	if len(ideProjectionDriftPairs) != len(want) {
		t.Fatalf("projection drift pair count = %d, want %d", len(ideProjectionDriftPairs), len(want))
	}
	got := map[string]string{}
	for _, pair := range ideProjectionDriftPairs {
		if pair.idFn == nil {
			t.Fatalf("projection drift pair %s -> %s has nil id function", pair.sourceResource, pair.localResource)
		}
		expected, ok := want[pair.localResource]
		if !ok {
			t.Fatalf("unexpected projection drift local resource %s", pair.localResource)
		}
		got[pair.localResource] = pair.sourceResource
		row := sampleIDEProjectionDriftRow(pair.localResource)
		if id := pair.idFn(row); id != expected.id {
			t.Fatalf("projection drift id for %s = %q, want %q", pair.localResource, id, expected.id)
		}
		if id := ideReadModelID(pair.localResource, row); id != expected.id {
			t.Fatalf("ideReadModelID for %s = %q, want %q", pair.localResource, id, expected.id)
		}
	}
	for localResource, expected := range want {
		if got[localResource] != expected.sourceResource {
			t.Fatalf("projection drift pair for %s = %q, want %q", localResource, got[localResource], expected.sourceResource)
		}
	}
}

func TestIDEProjectionRepositoryCloneIsolation(t *testing.T) {
	ctx := context.Background()
	store := platform.NewStore()
	repo := ideProjectionRepoFromStore(store, platform.Config{ServiceName: serviceName})
	input := map[string]any{"id": "P1", "project_name": "original"}
	requireNoIDEProjectionError(t, repo.UpsertProject(ctx, input))

	input["project_name"] = "mutated input"
	if got := repo.ListProjects(ctx); len(got) != 1 || got[0].Data["project_name"] != "original" {
		t.Fatalf("stored project after caller mutation = %#v, want original", got)
	}
	rows := repo.ListProjects(ctx)
	rows[0].Data["project_name"] = "mutated output"
	if got := repo.ListProjects(ctx); got[0].Data["project_name"] != "original" {
		t.Fatalf("stored project after listed row mutation = %#v, want original", got)
	}
}

func TestIDEProjectionRepositoryConflictFallbackAndNilStore(t *testing.T) {
	ctx := context.Background()
	conflictStore := &ideProjectionConflictStore{createErr: platform.CreateConflictError{Resource: ideProjectsResource, ID: "P1"}}
	repo := ideProjectionRepoFromStore(conflictStore, platform.Config{ServiceName: serviceName})
	requireNoIDEProjectionError(t, repo.UpsertProject(ctx, map[string]any{"id": "P1"}))
	if conflictStore.createCalls != 1 || conflictStore.updateCalls != 2 {
		t.Fatalf("conflict calls create=%d update=%d, want 1/2", conflictStore.createCalls, conflictStore.updateCalls)
	}

	failingStore := &ideProjectionConflictStore{createErr: errors.New("store unavailable")}
	repo = ideProjectionRepoFromStore(failingStore, platform.Config{ServiceName: serviceName})
	if err := repo.UpsertProject(ctx, map[string]any{"id": "P1"}); err == nil {
		t.Fatal("UpsertProject store error = nil, want error")
	}

	nilRepo := ideProjectionRepoFromStore(nil, platform.Config{ServiceName: serviceName})
	requireNoIDEProjectionError(t, nilRepo.UpsertProject(ctx, map[string]any{}))
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

func TestIDEProjectionRepositorySourceGuard(t *testing.T) {
	dir := ideProjectionRepositoryTestDir(t)
	guard := newIDEProjectionSourceGuard()
	violations, err := collectIDEProjectionSourceGuardViolations(dir, guard)
	if err != nil {
		t.Fatal(err)
	}
	for _, violation := range violations {
		t.Error(violation)
	}
}

func createIDEProjectionRecord(t *testing.T, store platform.RecordStore, resource string, row map[string]any) {
	t.Helper()
	if _, err := store.Create(context.Background(), resource, row); err != nil {
		t.Fatal(err)
	}
}

func requireNoIDEProjectionError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func assertIDEProjectionDriftFindings(t *testing.T, label string, findings []ideProjectionDriftFinding, want ...ideProjectionDriftFinding) {
	t.Helper()
	if len(findings) != len(want) {
		t.Fatalf("%s findings = %#v, want %#v", label, findings, want)
	}
	for i := range want {
		if findings[i] != want[i] {
			t.Fatalf("%s finding[%d] = %#v, want %#v", label, i, findings[i], want[i])
		}
	}
}

func sampleIDEProjectionDriftRow(resource string) map[string]any {
	switch resource {
	case ideIdentityUsersResource:
		return map[string]any{"user_id": "U-map"}
	case ideIdentityRolesResource:
		return map[string]any{"role_id": "R-map"}
	case idePolicyRolesResource:
		return map[string]any{"role_id": "PR-map"}
	case ideProjectsResource:
		return map[string]any{"project_id": "P-map"}
	case ideProjectMembersResource:
		return map[string]any{"project_id": "P-map", "user_id": "U-map"}
	case ideUserGroupsResource:
		return map[string]any{"user_id": "U-map", "group_id": "G-map"}
	default:
		return map[string]any{}
	}
}

type ideProjectionConflictStore struct {
	createErr   error
	createCalls int
	updateCalls int
}

func (s *ideProjectionConflictStore) Create(context.Context, string, map[string]any) (contracts.Record[map[string]any], error) {
	s.createCalls++
	return contracts.Record[map[string]any]{}, s.createErr
}

func (s *ideProjectionConflictStore) Get(context.Context, string, string) (contracts.Record[map[string]any], bool) {
	return contracts.Record[map[string]any]{}, false
}

func (s *ideProjectionConflictStore) List(context.Context, string) []contracts.Record[map[string]any] {
	return nil
}

func (s *ideProjectionConflictStore) Update(_ context.Context, _ string, id string, data map[string]any) (contracts.Record[map[string]any], bool) {
	s.updateCalls++
	if s.updateCalls == 1 {
		return contracts.Record[map[string]any]{}, false
	}
	return contracts.Record[map[string]any]{ID: id, Data: data}, true
}

func (s *ideProjectionConflictStore) Delete(context.Context, string, string) bool {
	return false
}

func (s *ideProjectionConflictStore) NextID(string, string, int, int) string {
	return ""
}

func ideProjectionRepositoryTestDir(t *testing.T) string {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Dir(currentFile)
}

type ideProjectionSourceGuard struct {
	afterStore            *regexp.Regexp
	beforeStore           *regexp.Regexp
	directProjectionStore *regexp.Regexp
}

func newIDEProjectionSourceGuard() ideProjectionSourceGuard {
	owned := `(ideIdentityUsersResource|ideIdentityRolesResource|idePolicyRolesResource|ideProjectsResource|ideProjectMembersResource|ideUserGroupsResource|ide-service:(?:ide_identity_users|ide_identity_roles|ide_policy_roles|ide_projects|ide_project_members|ide_user_groups)|":(?:ide_identity_users|ide_identity_roles|ide_policy_roles|ide_projects|ide_project_members|ide_user_groups)")`
	storeCall := `(?:Store|store)\s*\.\s*(?:Get|List|Create|Update|Delete|NextID)`
	return ideProjectionSourceGuard{
		afterStore:            regexp.MustCompile(storeCall + `(?s:[^\n;]*)\b` + owned + `\b`),
		beforeStore:           regexp.MustCompile(`\b` + owned + `\b(?s:[^\n;]*)` + storeCall),
		directProjectionStore: regexp.MustCompile(storeCall),
	}
}

func collectIDEProjectionSourceGuardViolations(dir string, guard ideProjectionSourceGuard) ([]string, error) {
	var violations []string
	err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if skipIDEProjectionSourceGuardFile(path, entry) {
			return nil
		}
		fileViolations, err := ideProjectionSourceGuardViolations(path, guard)
		if err != nil {
			return err
		}
		violations = append(violations, fileViolations...)
		return nil
	})
	return violations, err
}

func skipIDEProjectionSourceGuardFile(path string, entry os.DirEntry) bool {
	if entry.IsDir() || !strings.HasSuffix(path, ".go") {
		return true
	}
	name := filepath.Base(path)
	return strings.HasSuffix(name, "_test.go") || name == "ide_projection_repository.go"
}

func ideProjectionSourceGuardViolations(path string, guard ideProjectionSourceGuard) ([]string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	name := filepath.Base(path)
	text := string(content)
	var violations []string
	if guard.afterStore.MatchString(text) || guard.beforeStore.MatchString(text) {
		violations = append(violations, path+" directly accesses IDE projection resources through RecordStore; use ideProjectionRepository")
	}
	if name == "projection.go" && guard.directProjectionStore.MatchString(text) {
		violations = append(violations, path+" directly accesses RecordStore in IDE projection orchestration; use ideProjectionRepository")
	}
	return violations, nil
}
