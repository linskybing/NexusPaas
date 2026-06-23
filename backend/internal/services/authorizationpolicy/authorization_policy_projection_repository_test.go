package authorizationpolicy

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

func TestAuthorizationPolicyProjectionRepositoryIdentityLifecycle(t *testing.T) {
	ctx := context.Background()
	store := platform.NewStore()
	repo := authorizationPolicyProjectionRepoFromStore(store, platform.Config{ServiceName: serviceName})

	if err := repo.UpsertIdentityUser(ctx, map[string]any{"user_id": "U1", "role_id": "R1"}); err != nil {
		t.Fatalf("UpsertIdentityUser: %v", err)
	}
	if err := repo.UpsertIdentityRole(ctx, map[string]any{"name": "R1", "capabilities": map[string]any{"adminPanel": true}}); err != nil {
		t.Fatalf("UpsertIdentityRole: %v", err)
	}
	users := repo.ListIdentityUsers(ctx)
	roles := repo.ListIdentityRoles(ctx)
	if len(users) != 1 || users[0]["id"] != "U1" || len(roles) != 1 || roles[0]["id"] != "R1" {
		t.Fatalf("identity read models users=%#v roles=%#v, want U1/R1", users, roles)
	}
	users[0]["role_id"] = "mutated"
	if again := repo.ListIdentityUsers(ctx); again[0]["role_id"] != "R1" {
		t.Fatalf("ListIdentityUsers leaked mutable row: %#v", again)
	}
	if repo.DeleteIdentityUser(ctx, map[string]any{"id": "U1", "deleted": false}) {
		t.Fatal("DeleteIdentityUser deleted=false = true, want no-op")
	}
	if !repo.DeleteIdentityUser(ctx, map[string]any{"id": "U1", "deleted": true}) {
		t.Fatal("DeleteIdentityUser deleted=true = false, want delete")
	}
	if got := repo.ListIdentityUsers(ctx); len(got) != 0 {
		t.Fatalf("identity users after delete = %#v, want empty", got)
	}
}

func TestAuthorizationPolicyProjectionRepositoryIdentitySourceFallbackGating(t *testing.T) {
	ctx := context.Background()
	store := platform.NewStore()
	if _, err := store.Create(ctx, usersResource, map[string]any{"id": "U1", "role_id": "source"}); err != nil {
		t.Fatal(err)
	}

	isolated := authorizationPolicyProjectionRepoFromStore(store, platform.Config{ServiceName: serviceName})
	if got := isolated.ListIdentityUsers(ctx); len(got) != 0 {
		t.Fatalf("isolated identity source rows = %#v, want none", got)
	}

	cohosted := authorizationPolicyProjectionRepoFromStore(store, platform.Config{ServiceName: "all"})
	if got := cohosted.ListIdentityUsers(ctx); len(got) != 1 || got[0]["role_id"] != "source" {
		t.Fatalf("cohosted identity source rows = %#v, want source user", got)
	}
	if err := cohosted.UpsertIdentityUser(ctx, map[string]any{"id": "U1", "role_id": "local"}); err != nil {
		t.Fatal(err)
	}
	got := cohosted.ListIdentityUsers(ctx)
	if len(got) != 1 || got[0]["role_id"] != "local" {
		t.Fatalf("cohosted merged identity rows = %#v, want local override", got)
	}
}

func TestAuthorizationPolicyProjectionRepositoryPolicyDataLifecycle(t *testing.T) {
	ctx := context.Background()
	store := platform.NewStore()
	repo := authorizationPolicyProjectionRepoFromStore(store, platform.Config{ServiceName: serviceName})

	if err := repo.UpsertPolicyProject(ctx, map[string]any{"project_id": "P1", "plan_id": "PL1"}); err != nil {
		t.Fatalf("UpsertPolicyProject: %v", err)
	}
	if err := repo.UpsertPolicyPlan(ctx, map[string]any{"plan_id": "PL1", "gpu_limit": 4}); err != nil {
		t.Fatalf("UpsertPolicyPlan: %v", err)
	}
	if err := repo.UpsertPolicyImageAllowList(ctx, map[string]any{"project_id": "P1", "tag_id": "T1", "image_reference": "repo/app:v1", "enabled": true}); err != nil {
		t.Fatalf("UpsertPolicyImageAllowList: %v", err)
	}
	projects := repo.ListPolicyProjects(ctx)
	if len(projects) != 1 || policyProjectID(projects[0]) != "P1" {
		t.Fatalf("ListPolicyProjects = %#v, want P1", projects)
	}
	projects[0]["plan_id"] = "mutated"
	if again := repo.ListPolicyProjects(ctx); again[0]["plan_id"] != "PL1" {
		t.Fatalf("ListPolicyProjects leaked mutable row: %#v", again)
	}
	if plan := repo.FindPolicyPlanForProject(ctx, map[string]any{"id": "P1", "plan_id": "PL1"}); policyPlanID(plan) != "PL1" {
		t.Fatalf("FindPolicyPlanForProject = %#v, want PL1", plan)
	}
	rules := repo.ListPolicyImageRulesForProject(ctx, "P1")
	if len(rules) != 1 || rules[0]["image_reference"] != "repo/app:v1" {
		t.Fatalf("ListPolicyImageRulesForProject = %#v, want repo/app:v1", rules)
	}
	if !repo.DeletePolicyImageAllowList(ctx, map[string]any{"tag_id": "T1"}) {
		t.Fatal("DeletePolicyImageAllowList by tag_id = false, want fallback delete")
	}
	if got := repo.ListPolicyImageRulesForProject(ctx, "P1"); len(got) != 0 {
		t.Fatalf("policy image rules after delete = %#v, want empty", got)
	}
	if !repo.DeletePolicyProject(ctx, map[string]any{"project_id": "P1"}) || !repo.DeletePolicyPlan(ctx, map[string]any{"plan_id": "PL1"}) {
		t.Fatal("policy project/plan delete failed")
	}
}

func TestAuthorizationPolicyProjectionRepositoryPolicyDataSourceFallbackGating(t *testing.T) {
	ctx := context.Background()
	store := platform.NewStore()
	if _, err := store.Create(ctx, policySourceProjectsResource, map[string]any{"id": "P1", "plan_id": "source"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create(ctx, policySourcePlansResource, map[string]any{"id": "PL1", "gpu_limit": 1}); err != nil {
		t.Fatal(err)
	}

	isolated := authorizationPolicyProjectionRepoFromStore(store, platform.Config{ServiceName: serviceName})
	if got := isolated.ListPolicyProjects(ctx); len(got) != 0 {
		t.Fatalf("isolated policy source rows = %#v, want none", got)
	}

	cohosted := authorizationPolicyProjectionRepoFromStore(store, platform.Config{ServiceName: "all"})
	if got := cohosted.ListPolicyProjects(ctx); len(got) != 1 || got[0]["plan_id"] != "source" {
		t.Fatalf("cohosted policy source rows = %#v, want source project", got)
	}
	if err := cohosted.UpsertPolicyProject(ctx, map[string]any{"id": "P1", "plan_id": "local"}); err != nil {
		t.Fatal(err)
	}
	got := cohosted.ListPolicyProjects(ctx)
	if len(got) != 1 || got[0]["plan_id"] != "local" {
		t.Fatalf("cohosted merged policy rows = %#v, want local override", got)
	}
}

func TestAuthorizationPolicyProjectionRepositoryConflictFallbackAndNilStore(t *testing.T) {
	ctx := context.Background()
	conflictStore := &projectionConflictStore{createErr: platform.CreateConflictError{Resource: policyDataPlansResource, ID: "PL1"}}
	repo := authorizationPolicyProjectionRepoFromStore(conflictStore, platform.Config{ServiceName: serviceName})
	if err := repo.UpsertPolicyPlan(ctx, map[string]any{"id": "PL1", "gpu_limit": 1}); err != nil {
		t.Fatalf("UpsertPolicyPlan conflict fallback: %v", err)
	}
	if conflictStore.createCalls != 1 || conflictStore.updateCalls != 2 {
		t.Fatalf("conflict calls create=%d update=%d, want 1/2", conflictStore.createCalls, conflictStore.updateCalls)
	}

	failingStore := &projectionConflictStore{createErr: errors.New("store unavailable")}
	repo = authorizationPolicyProjectionRepoFromStore(failingStore, platform.Config{ServiceName: serviceName})
	if err := repo.UpsertIdentityRole(ctx, map[string]any{"id": "R1"}); err == nil {
		t.Fatal("UpsertIdentityRole store error = nil, want error")
	}

	nilRepo := authorizationPolicyProjectionRepoFromStore(nil, platform.Config{ServiceName: serviceName})
	if got := nilRepo.ListIdentityUsers(ctx); len(got) != 0 {
		t.Fatalf("ListIdentityUsers nil store = %#v, want empty", got)
	}
	if err := nilRepo.UpsertPolicyProject(ctx, map[string]any{"id": "P1"}); err == nil {
		t.Fatal("UpsertPolicyProject nil store err = nil, want fail-closed error")
	}
	if nilRepo.DeletePolicyProject(ctx, map[string]any{"id": "P1"}) {
		t.Fatal("DeletePolicyProject nil store = true, want false")
	}
}

func TestAuthorizationPolicyProjectionRepositoryProjectionDriftDetectsMissingOrphanStaleAndSorts(t *testing.T) {
	ctx := context.Background()
	store := platform.NewStore()
	repo := authorizationPolicyProjectionRepoFromStore(store, platform.Config{ServiceName: "all"})

	createAuthorizationPolicyProjectionRecord(t, store, policySourceProjectsResource, map[string]any{"id": "P9", "project_id": "P9", "plan_id": "PL-missing-9"})
	createAuthorizationPolicyProjectionRecord(t, store, policySourceProjectsResource, map[string]any{"id": "P1", "project_id": "P1", "plan_id": "PL-clean"})
	createAuthorizationPolicyProjectionRecord(t, store, policySourceProjectsResource, map[string]any{"id": "P5", "project_id": "P5", "plan_id": "PL-source"})
	createAuthorizationPolicyProjectionRecord(t, store, policySourceProjectsResource, map[string]any{"id": "P3", "project_id": "P3", "plan_id": "PL-missing-3"})
	createAuthorizationPolicyProjectionRecord(t, store, policyDataProjectsResource, map[string]any{"id": "P7", "project_id": "P7", "plan_id": "PL-orphan-7"})
	createAuthorizationPolicyProjectionRecord(t, store, policyDataProjectsResource, map[string]any{"id": "P5", "project_id": "P5", "plan_id": "PL-local"})
	createAuthorizationPolicyProjectionRecord(t, store, policyDataProjectsResource, map[string]any{"id": "P1", "project_id": "P1", "plan_id": "PL-clean"})
	createAuthorizationPolicyProjectionRecord(t, store, policyDataProjectsResource, map[string]any{"id": "P2", "project_id": "P2", "plan_id": "PL-orphan-2"})

	report, err := repo.projectionDrift(ctx)
	if err != nil {
		t.Fatalf("projectionDrift: %v", err)
	}
	assertAuthorizationPolicyProjectionDriftIDs(t, "missing", report.Missing, policyDataProjectsResource, "P3", "P9")
	assertAuthorizationPolicyProjectionDriftIDs(t, "orphan", report.Orphan, policyDataProjectsResource, "P2", "P7")
	assertAuthorizationPolicyProjectionDriftIDs(t, "stale", report.Stale, policyDataProjectsResource, "P5")
}

func TestAuthorizationPolicyProjectionRepositoryProjectionDriftNilStoreFailsClosed(t *testing.T) {
	repo := authorizationPolicyProjectionRepoFromStore(nil, platform.Config{ServiceName: serviceName})
	if _, err := repo.projectionDrift(context.Background()); !errors.Is(err, errAuthorizationPolicyProjectionRepositoryUnavailable) {
		t.Fatalf("projectionDrift nil store error = %v, want %v", err, errAuthorizationPolicyProjectionRepositoryUnavailable)
	}
}

func TestAuthorizationPolicyProjectionRepositoryProjectionDriftPairsCoverKnownResources(t *testing.T) {
	want := map[string]string{
		policyIdentityUsers:               usersResource,
		policyIdentityRoles:               rolesResource,
		policyDataProjectsResource:        policySourceProjectsResource,
		policyDataPlansResource:           policySourcePlansResource,
		policyDataImageAllowListsResource: policySourceImageAllowListsResource,
	}
	got := map[string]string{}
	for _, pair := range authorizationPolicyProjectionDriftPairs {
		if pair.idFn == nil {
			t.Fatalf("projection drift pair %s -> %s has nil id function", pair.sourceResource, pair.localResource)
		}
		got[pair.localResource] = pair.sourceResource
	}
	if len(got) != len(want) {
		t.Fatalf("projection drift pair count = %d, want %d (%#v)", len(got), len(want), got)
	}
	for localResource, sourceResource := range want {
		if got[localResource] != sourceResource {
			t.Fatalf("projection drift pair for %s = %q, want %q", localResource, got[localResource], sourceResource)
		}
	}
}

func TestAuthorizationPolicyProjectionRepositorySourceGuardOwnsProjectionResources(t *testing.T) {
	dir := authorizationPolicyProjectionRepositoryTestDir(t)
	guard := newAuthorizationPolicyProjectionSourceGuard()
	violations, err := collectAuthorizationPolicyProjectionSourceGuardViolations(dir, guard)
	if err != nil {
		t.Fatal(err)
	}
	for _, violation := range violations {
		t.Error(violation)
	}
}

func authorizationPolicyProjectionRepositoryTestDir(t *testing.T) string {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Dir(currentFile)
}

func createAuthorizationPolicyProjectionRecord(t *testing.T, store platform.RecordStore, resource string, row map[string]any) {
	t.Helper()
	if _, err := store.Create(context.Background(), resource, row); err != nil {
		t.Fatalf("create %s/%s: %v", resource, row["id"], err)
	}
}

func assertAuthorizationPolicyProjectionDriftIDs(t *testing.T, label string, findings []authorizationPolicyProjectionDriftFinding, resource string, want ...string) {
	t.Helper()
	if len(findings) != len(want) {
		t.Fatalf("%s findings = %#v, want ids %v", label, findings, want)
	}
	for i, id := range want {
		if findings[i].LocalResource != resource || findings[i].ID != id {
			t.Fatalf("%s finding[%d] = %#v, want resource %s id %s", label, i, findings[i], resource, id)
		}
	}
}

type authorizationPolicyProjectionSourceGuard struct {
	afterStore            *regexp.Regexp
	beforeStore           *regexp.Regexp
	directProjectionStore *regexp.Regexp
}

func newAuthorizationPolicyProjectionSourceGuard() authorizationPolicyProjectionSourceGuard {
	owned := `(policyIdentityUsers|policyIdentityRoles|policyDataProjectsResource|policyDataPlansResource|policyDataImageAllowListsResource|authorization-policy-service:(?:identity_users|identity_roles|policy_projects|policy_plans|policy_image_allow_lists)|":(?:identity_users|identity_roles|policy_projects|policy_plans|policy_image_allow_lists)")`
	storeCall := `(?:Store|store)\s*\.\s*(?:Get|List|Create|Update|Delete|NextID)`
	return authorizationPolicyProjectionSourceGuard{
		afterStore:            regexp.MustCompile(storeCall + `(?s:[^\n;]*)\b` + owned + `\b`),
		beforeStore:           regexp.MustCompile(`\b` + owned + `\b(?s:[^\n;]*)` + storeCall),
		directProjectionStore: regexp.MustCompile(storeCall),
	}
}

func collectAuthorizationPolicyProjectionSourceGuardViolations(dir string, guard authorizationPolicyProjectionSourceGuard) ([]string, error) {
	var violations []string
	err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if skipAuthorizationPolicyProjectionSourceGuardFile(path, entry) {
			return nil
		}
		fileViolations, err := authorizationPolicyProjectionSourceGuardViolations(path, guard)
		if err != nil {
			return err
		}
		violations = append(violations, fileViolations...)
		return nil
	})
	return violations, err
}

func skipAuthorizationPolicyProjectionSourceGuardFile(path string, entry os.DirEntry) bool {
	if entry.IsDir() || !strings.HasSuffix(path, ".go") {
		return true
	}
	name := filepath.Base(path)
	return strings.HasSuffix(name, "_test.go") || name == "authorization_policy_projection_repository.go"
}

func authorizationPolicyProjectionSourceGuardViolations(path string, guard authorizationPolicyProjectionSourceGuard) ([]string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	name := filepath.Base(path)
	text := string(content)
	var violations []string
	if guard.afterStore.MatchString(text) || guard.beforeStore.MatchString(text) {
		violations = append(violations, path+" directly accesses authorization-policy projection resources through RecordStore; use authorizationPolicyProjectionRepository")
	}
	if isAuthorizationPolicyProjectionOrchestrator(name) && guard.directProjectionStore.MatchString(text) {
		violations = append(violations, path+" directly accesses RecordStore in projection/read-model orchestration; use authorizationPolicyProjectionRepository")
	}
	return violations, nil
}

func isAuthorizationPolicyProjectionOrchestrator(name string) bool {
	return name == "identity_projection.go" || name == "policy_data_sync.go"
}

type projectionConflictStore struct {
	createErr   error
	createCalls int
	updateCalls int
}

func (s *projectionConflictStore) Create(context.Context, string, map[string]any) (contracts.Record[map[string]any], error) {
	s.createCalls++
	return contracts.Record[map[string]any]{}, s.createErr
}

func (s *projectionConflictStore) Get(context.Context, string, string) (contracts.Record[map[string]any], bool) {
	return contracts.Record[map[string]any]{}, false
}

func (s *projectionConflictStore) List(context.Context, string) []contracts.Record[map[string]any] {
	return nil
}

func (s *projectionConflictStore) Update(_ context.Context, _ string, id string, data map[string]any) (contracts.Record[map[string]any], bool) {
	s.updateCalls++
	if s.updateCalls == 1 {
		return contracts.Record[map[string]any]{}, false
	}
	return contracts.Record[map[string]any]{ID: id, Data: data}, true
}

func (s *projectionConflictStore) Delete(context.Context, string, string) bool {
	return false
}

func (s *projectionConflictStore) NextID(string, string, int, int) string {
	return ""
}
