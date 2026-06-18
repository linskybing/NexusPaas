package authorizationpolicy

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

func TestAuthorizationPolicyRepositorySeedsAndComposesDefaults(t *testing.T) {
	ctx := context.Background()
	app := newAuthorizationPolicyRepositoryTestApp()
	repo := authorizationPolicyRepo(app)

	if err := repo.EnsureDefaultProxyServices(ctx); err != nil {
		t.Fatalf("EnsureDefaultProxyServices: %v", err)
	}
	if err := repo.EnsureDefaultProxyPolicies(ctx); err != nil {
		t.Fatalf("EnsureDefaultProxyPolicies: %v", err)
	}
	if err := repo.EnsureDefaultProxyRoles(ctx); err != nil {
		t.Fatalf("EnsureDefaultProxyRoles: %v", err)
	}
	if err := repo.EnsureDefaultProxyAssignments(ctx); err != nil {
		t.Fatalf("EnsureDefaultProxyAssignments: %v", err)
	}

	services := repo.ListProxyServices(ctx)
	if len(services) != len(defaultServices) || services[0]["id"] != "SVC_MINIO" {
		t.Fatalf("services = %#v, want seeded services sorted by sort_order", services)
	}
	policies := repo.ListProxyPolicies(ctx)
	if len(policies) != len(defaultPolicies) {
		t.Fatalf("policies = %#v, want default policies", policies)
	}
	if rules, ok := policies[0]["rules"].([]map[string]any); !ok || len(rules) == 0 {
		t.Fatalf("default policy rules = %#v, want composed rules", policies[0]["rules"])
	}
	roles := repo.ListProxyRoles(ctx)
	if len(roles) != len(defaultPlatformRoles) || roles[0]["id"] != "RL2600001" {
		t.Fatalf("roles = %#v, want seeded roles sorted by name", roles)
	}
	assignments := repo.ListPolicyAssignments(ctx, "PO2600001")
	if len(assignments) != 1 || assignments[0]["policy"] == nil {
		t.Fatalf("assignments = %#v, want composed seeded assignment", assignments)
	}

	if got := len(app.Store.List(ctx, seedMarkersResource)); got != 4 {
		t.Fatalf("seed markers = %d, want 4", got)
	}
	_ = repo.EnsureDefaultProxyAssignments(ctx)
	if got := len(app.Store.List(ctx, seedMarkersResource)); got != 4 {
		t.Fatalf("seed markers after replay = %d, want 4", got)
	}
}

func TestAuthorizationPolicyRepositoryPolicyLifecycleAndCascade(t *testing.T) {
	ctx := context.Background()
	app := newAuthorizationPolicyRepositoryTestApp()
	repo := authorizationPolicyRepo(app)
	now := time.Date(2026, 6, 16, 7, 0, 0, 0, time.UTC)

	createRepositoryPolicyWithRule(t, repo, ctx, now)
	assertRepositoryPolicyNameLookup(t, repo, ctx)
	replaceRepositoryPolicyRules(t, app, repo, ctx, now)
	createRepositoryPolicyAssignment(t, repo, ctx)
	assertRepositoryPolicyCascadeDelete(t, app, repo, ctx)
}

func createRepositoryPolicyWithRule(t *testing.T, repo *recordStoreAuthorizationPolicyRepository, ctx context.Context, now time.Time) {
	t.Helper()
	policy, err := repo.CreateProxyPolicy(ctx, map[string]any{
		"id":          "P1",
		"name":        "analytics",
		"description": "analytics proxy",
		"is_system":   false,
		"created_at":  now,
		"updated_at":  now,
	}, []map[string]any{{"id": "R1", "service_id": "SVC_MINIO", "actions": []string{"view"}}})
	if err != nil {
		t.Fatalf("CreateProxyPolicy: %v", err)
	}
	if rules := policy["rules"].([]map[string]any); len(rules) != 1 || rules[0]["service_id"] != "SVC_MINIO" {
		t.Fatalf("created policy = %#v, want composed rule", policy)
	}
}

func assertRepositoryPolicyNameLookup(t *testing.T, repo *recordStoreAuthorizationPolicyRepository, ctx context.Context) {
	t.Helper()
	if !repo.PolicyNameExists(ctx, "", "ANALYTICS") || repo.PolicyNameExists(ctx, "P1", "analytics") {
		t.Fatal("PolicyNameExists did not respect case-insensitive duplicate and exclude id")
	}
}

func replaceRepositoryPolicyRules(t *testing.T, app *platform.App, repo *recordStoreAuthorizationPolicyRepository, ctx context.Context, now time.Time) {
	t.Helper()
	updated, ok, err := repo.UpdateProxyPolicy(ctx, "P1", map[string]any{
		"description": "updated",
		"updated_at":  now.Add(time.Minute),
	}, &proxyPolicyRuleReplacement{Rules: []map[string]any{{"id": "R2", "service_id": "SVC_HARBOR", "actions": []string{"create"}}}})
	if err != nil || !ok {
		t.Fatalf("UpdateProxyPolicy ok=%v err=%v", ok, err)
	}
	if rules := updated["rules"].([]map[string]any); len(rules) != 1 || rules[0]["id"] != "R2" {
		t.Fatalf("updated policy rules = %#v, want replacement rule R2", updated["rules"])
	}
	if _, found := app.Store.Get(ctx, rulesResource, "R1"); found {
		t.Fatal("UpdateProxyPolicy left old rule R1")
	}
}

func createRepositoryPolicyAssignment(t *testing.T, repo *recordStoreAuthorizationPolicyRepository, ctx context.Context) {
	t.Helper()
	assignment, created, err := repo.CreatePolicyAssignment(ctx, "P1", "user", "U1", "ADMIN")
	if err != nil || !created || assignment["policy"] == nil {
		t.Fatalf("CreatePolicyAssignment assignment=%#v created=%v err=%v", assignment, created, err)
	}
}

func assertRepositoryPolicyCascadeDelete(t *testing.T, app *platform.App, repo *recordStoreAuthorizationPolicyRepository, ctx context.Context) {
	t.Helper()
	deleted, found := repo.DeleteProxyPolicyCascade(ctx, "P1")
	if !found || deleted["id"] != "P1" {
		t.Fatalf("DeleteProxyPolicyCascade = %#v found=%v, want P1", deleted, found)
	}
	if _, found := app.Store.Get(ctx, policiesResource, "P1"); found {
		t.Fatal("policy P1 still exists after cascade delete")
	}
	if _, found := app.Store.Get(ctx, rulesResource, "R2"); found {
		t.Fatal("rule R2 still exists after cascade delete")
	}
	if rows := repo.ListTargetAssignments(ctx, "user", "U1"); len(rows) != 0 {
		t.Fatalf("assignments after policy cascade = %#v, want none", rows)
	}
}

func TestAuthorizationPolicyRepositoryRoleAssignmentIdempotencyAndCascade(t *testing.T) {
	ctx := context.Background()
	app := newAuthorizationPolicyRepositoryTestApp()
	repo := authorizationPolicyRepo(app)
	now := time.Date(2026, 6, 16, 7, 30, 0, 0, time.UTC)

	role := createRepositoryRole(t, repo, ctx, now)
	role["name"] = "mutated"
	assertRepositoryRoleCloneIsolation(t, repo, ctx)
	assertRepositoryRoleUserIdempotency(t, repo, ctx)
	createRepositoryPolicyForRoleAssignment(t, repo, ctx, now)
	assertRepositoryRolePolicyAssignmentIdempotency(t, repo, ctx)
	assertRepositoryRoleCascadeDelete(t, repo, ctx)
}

func createRepositoryRole(t *testing.T, repo *recordStoreAuthorizationPolicyRepository, ctx context.Context, now time.Time) map[string]any {
	t.Helper()
	role, err := repo.CreateProxyRole(ctx, map[string]any{
		"id":           "ROLE1",
		"name":         "operators",
		"display_name": "Operators",
		"is_system":    false,
		"created_at":   now,
		"updated_at":   now,
	})
	if err != nil {
		t.Fatalf("CreateProxyRole: %v", err)
	}
	return role
}

func assertRepositoryRoleCloneIsolation(t *testing.T, repo *recordStoreAuthorizationPolicyRepository, ctx context.Context) {
	t.Helper()
	gotRole, found := repo.GetProxyRole(ctx, "ROLE1")
	if !found || gotRole["name"] != "operators" {
		t.Fatalf("stored role = %#v found=%v, want clone isolation", gotRole, found)
	}
}

func assertRepositoryRoleUserIdempotency(t *testing.T, repo *recordStoreAuthorizationPolicyRepository, ctx context.Context) {
	t.Helper()
	member, created, err := repo.CreateRoleUser(ctx, "ROLE1", "U1", "ADMIN")
	if err != nil || !created || member["role"] == nil {
		t.Fatalf("CreateRoleUser member=%#v created=%v err=%v", member, created, err)
	}
	member, created, err = repo.CreateRoleUser(ctx, "ROLE1", "U1", "ADMIN")
	if err != nil || created || member["role"] == nil {
		t.Fatalf("CreateRoleUser replay member=%#v created=%v err=%v, want existing", member, created, err)
	}
}

func createRepositoryPolicyForRoleAssignment(t *testing.T, repo *recordStoreAuthorizationPolicyRepository, ctx context.Context, now time.Time) {
	t.Helper()
	policy, err := repo.CreateProxyPolicy(ctx, map[string]any{
		"id":         "P2",
		"name":       "role-policy",
		"created_at": now,
		"updated_at": now,
	}, nil)
	if err != nil || policy["id"] != "P2" {
		t.Fatalf("CreateProxyPolicy for role assignment = %#v err=%v", policy, err)
	}
}

func assertRepositoryRolePolicyAssignmentIdempotency(t *testing.T, repo *recordStoreAuthorizationPolicyRepository, ctx context.Context) {
	t.Helper()
	assignment, created, err := repo.CreatePolicyAssignment(ctx, "P2", "role", "ROLE1", "ADMIN")
	if err != nil || !created || assignment["policy"] == nil {
		t.Fatalf("CreatePolicyAssignment role target = %#v created=%v err=%v", assignment, created, err)
	}
	assignment, created, err = repo.CreatePolicyAssignment(ctx, "P2", "role", "ROLE1", "ADMIN")
	if err != nil || created || assignment["policy"] == nil {
		t.Fatalf("CreatePolicyAssignment replay = %#v created=%v err=%v, want existing", assignment, created, err)
	}
}

func assertRepositoryRoleCascadeDelete(t *testing.T, repo *recordStoreAuthorizationPolicyRepository, ctx context.Context) {
	t.Helper()
	deleted, found := repo.DeleteProxyRoleCascade(ctx, "ROLE1")
	if !found || deleted["id"] != "ROLE1" {
		t.Fatalf("DeleteProxyRoleCascade = %#v found=%v, want ROLE1", deleted, found)
	}
	if rows := repo.ListRoleUsers(ctx, "ROLE1"); len(rows) != 0 {
		t.Fatalf("role users after role cascade = %#v, want none", rows)
	}
	if rows := repo.ListTargetAssignments(ctx, "role", "ROLE1"); len(rows) != 0 {
		t.Fatalf("role target assignments after role cascade = %#v, want none", rows)
	}
}

func TestAuthorizationPolicyRepositoryNilStoreFailClosed(t *testing.T) {
	repo := recordStoreAuthorizationPolicyRepository{}
	if err := repo.EnsureDefaultProxyServices(context.Background()); err == nil {
		t.Fatal("EnsureDefaultProxyServices nil store err = nil, want fail-closed error")
	}
	if got := repo.NextProxyPolicyID(context.Background()); got != "" {
		t.Fatalf("NextProxyPolicyID nil store = %q, want empty", got)
	}
	if _, err := repo.CreateProxyPolicy(context.Background(), map[string]any{"id": "P"}, nil); err == nil {
		t.Fatal("CreateProxyPolicy nil store err = nil, want fail-closed error")
	}
	if _, _, err := repo.CreateRoleUser(context.Background(), "R", "U", "A"); err == nil {
		t.Fatal("CreateRoleUser nil store err = nil, want fail-closed error")
	}
}

func TestAuthorizationPolicyRepositorySourceGuardOwnsProxyRBACResources(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	dir := filepath.Dir(currentFile)
	owned := `(servicesResource|policiesResource|rulesResource|assignmentsResource|platformRolesResource|roleUsersResource|seedMarkersResource)`
	storeCall := `(?:Store|store)\s*\.\s*(?:Get|List|Create|Update|Delete|NextID)`
	afterStore := regexp.MustCompile(storeCall + `(?s:[^\n;]*)\b` + owned + `\b`)
	beforeStore := regexp.MustCompile(`\b` + owned + `\b(?s:[^\n;]*)` + storeCall)

	err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		name := filepath.Base(path)
		if strings.HasSuffix(name, "_test.go") || name == "authorization_policy_repository.go" {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(content)
		if afterStore.MatchString(text) || beforeStore.MatchString(text) {
			t.Errorf("%s directly accesses authorization-policy proxy RBAC resources through RecordStore; use authorizationPolicyRepository", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func newAuthorizationPolicyRepositoryTestApp() *platform.App {
	return platform.NewApp(platform.Config{ServiceName: serviceName})
}
