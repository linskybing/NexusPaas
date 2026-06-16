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

func TestRawPermissionRepositoryPolicyLifecycle(t *testing.T) {
	ctx := context.Background()
	store := platform.NewStore()
	repo := rawPermissionRepoFromStore(store)
	aliceRead := []string{"alice", "project-1", "model", "read"}
	aliceWrite := []string{"alice", "project-1", "model", "write"}
	bobRead := []string{"bob", "project-1", "model", "read"}

	createRawPermissionPoliciesForTest(t, repo, store, ctx, aliceRead, bobRead)
	assertRawPermissionPolicyList(t, repo, ctx)
	assertRawPermissionPolicyUpdates(t, repo, ctx, aliceRead, aliceWrite, bobRead)
	assertRawPermissionPolicyAllowedAndDeleted(t, repo, ctx, aliceWrite)
}

func createRawPermissionPoliciesForTest(t *testing.T, repo rawPermissionRepository, store platform.RecordStore, ctx context.Context, aliceRead, bobRead []string) {
	t.Helper()
	created, err := repo.CreateRawPermissionPolicy(ctx, aliceRead)
	if err != nil || !created {
		t.Fatalf("CreateRawPermissionPolicy created=%v err=%v, want created", created, err)
	}
	created, err = repo.CreateRawPermissionPolicy(ctx, aliceRead)
	if err != nil || created {
		t.Fatalf("CreateRawPermissionPolicy replay created=%v err=%v, want conflict replay", created, err)
	}
	if _, err := store.Create(ctx, rawPoliciesResource, map[string]any{
		"id": rawPolicyID(bobRead),
		"v0": "bob", "v1": "project-1", "v2": "model", "v3": "read",
	}); err != nil {
		t.Fatal(err)
	}
}

func assertRawPermissionPolicyList(t *testing.T, repo rawPermissionRepository, ctx context.Context) {
	t.Helper()
	rows := repo.ListRawPermissionPolicies(ctx)
	if len(rows) != 2 || rows[0][0] != "alice" || rows[1][0] != "bob" {
		t.Fatalf("ListRawPermissionPolicies = %#v, want sorted alice/bob policies", rows)
	}
	rows[0][0] = "mutated"
	if again := repo.ListRawPermissionPolicies(ctx); again[0][0] != "alice" {
		t.Fatalf("ListRawPermissionPolicies leaked mutable row: %#v", again)
	}
}

func assertRawPermissionPolicyUpdates(t *testing.T, repo rawPermissionRepository, ctx context.Context, aliceRead, aliceWrite, bobRead []string) {
	t.Helper()
	result, err := repo.UpdateRawPermissionPolicy(ctx, aliceRead, aliceRead)
	if err != nil || !result.Found || !result.Updated || result.Conflict {
		t.Fatalf("UpdateRawPermissionPolicy same-id = %#v err=%v, want updated", result, err)
	}
	result, err = repo.UpdateRawPermissionPolicy(ctx, aliceRead, aliceWrite)
	if err != nil || !result.Found || !result.Updated || result.Conflict {
		t.Fatalf("UpdateRawPermissionPolicy new-id = %#v err=%v, want updated", result, err)
	}
	if repo.RawPermissionPolicyExists(ctx, aliceRead) || !repo.RawPermissionPolicyExists(ctx, aliceWrite) {
		t.Fatalf("policy existence old=%v new=%v, want old deleted and new present", repo.RawPermissionPolicyExists(ctx, aliceRead), repo.RawPermissionPolicyExists(ctx, aliceWrite))
	}
	result, err = repo.UpdateRawPermissionPolicy(ctx, aliceWrite, bobRead)
	if err != nil || !result.Found || !result.Conflict {
		t.Fatalf("UpdateRawPermissionPolicy conflict = %#v err=%v, want conflict", result, err)
	}
	result, err = repo.UpdateRawPermissionPolicy(ctx, []string{"missing", "project-1", "model", "read"}, aliceRead)
	if err != nil || result.Found || result.Conflict || result.Updated {
		t.Fatalf("UpdateRawPermissionPolicy missing = %#v err=%v, want not found", result, err)
	}
}

func assertRawPermissionPolicyAllowedAndDeleted(t *testing.T, repo rawPermissionRepository, ctx context.Context, aliceWrite []string) {
	t.Helper()
	allowed, err := repo.RawPermissionAllowed(ctx, aliceWrite[0], aliceWrite[1], aliceWrite[2], aliceWrite[3])
	if err != nil || !allowed {
		t.Fatalf("RawPermissionAllowed = %v err=%v, want allowed", allowed, err)
	}
	if !repo.DeleteRawPermissionPolicy(ctx, aliceWrite) || repo.DeleteRawPermissionPolicy(ctx, aliceWrite) {
		t.Fatal("DeleteRawPermissionPolicy did not delete once then report missing")
	}
}

func TestRawPermissionRepositoryGroupingLifecycle(t *testing.T) {
	ctx := context.Background()
	repo := rawPermissionRepoFromStore(platform.NewStore())
	add := map[string]string{"type": "project_member", "action": "add", "project_id": "P1", "user_id": "U1", "role": "viewer"}
	update := map[string]string{"type": "project_member", "action": "update", "project_id": "P1", "user_id": "U1", "role": "viewer"}
	remove := map[string]string{"type": "project_member", "action": "remove", "project_id": "P1", "user_id": "U1", "role": "viewer"}

	if err := repo.ApplyPermissionOperation(ctx, add); err != nil {
		t.Fatalf("ApplyPermissionOperation add: %v", err)
	}
	if err := repo.ApplyPermissionOperation(ctx, update); err != nil {
		t.Fatalf("ApplyPermissionOperation update: %v", err)
	}
	rows := repo.ListGroupingPolicies(ctx)
	if len(rows) != 1 || rows[0]["domain"] != "P1" || rows[0]["role"] != "viewer" {
		t.Fatalf("ListGroupingPolicies = %#v, want project member grouping", rows)
	}
	rows[0]["domain"] = "mutated"
	if again := repo.ListGroupingPolicies(ctx); again[0]["domain"] != "P1" {
		t.Fatalf("ListGroupingPolicies leaked mutable row: %#v", again)
	}
	if err := repo.ApplyPermissionOperation(ctx, remove); err != nil {
		t.Fatalf("ApplyPermissionOperation remove: %v", err)
	}
	if rows := repo.ListGroupingPolicies(ctx); len(rows) != 0 {
		t.Fatalf("ListGroupingPolicies after remove = %#v, want none", rows)
	}
	if err := repo.ApplyPermissionOperation(ctx, map[string]string{"type": "unsupported", "action": "add", "user_id": "U1"}); err == nil {
		t.Fatal("ApplyPermissionOperation unsupported type err = nil, want error")
	}
}

func TestRawPermissionRepositoryGroupingCreateConflictFallback(t *testing.T) {
	store := &rawPermissionConflictStore{
		createErr: platform.CreateConflictError{Resource: groupingResource, ID: "grouping"},
	}
	repo := rawPermissionRepoFromStore(store)

	if err := repo.UpsertGroupingPolicy(context.Background(), "project_member", "U1", "viewer", "P1"); err != nil {
		t.Fatalf("UpsertGroupingPolicy conflict fallback: %v", err)
	}
	if store.createCalls != 1 || store.updateCalls != 1 {
		t.Fatalf("calls create=%d update=%d, want 1/1", store.createCalls, store.updateCalls)
	}

	store = &rawPermissionConflictStore{createErr: errors.New("store unavailable")}
	repo = rawPermissionRepoFromStore(store)
	if err := repo.UpsertGroupingPolicy(context.Background(), "project_member", "U1", "viewer", "P1"); err == nil {
		t.Fatal("UpsertGroupingPolicy store error = nil, want error")
	}
}

func TestRawPermissionRepositoryNilStoreFailClosed(t *testing.T) {
	ctx := context.Background()
	repo := recordStoreRawPermissionRepository{}

	if rows := repo.ListRawPermissionPolicies(ctx); len(rows) != 0 {
		t.Fatalf("ListRawPermissionPolicies nil store = %#v, want empty", rows)
	}
	allowed, err := repo.RawPermissionAllowed(ctx, "alice", "project-1", "model", "read")
	if err != nil || allowed {
		t.Fatalf("RawPermissionAllowed nil store allowed=%v err=%v, want denied nil error", allowed, err)
	}
	if created, err := repo.CreateRawPermissionPolicy(ctx, []string{"alice", "project-1", "model", "read"}); err == nil || created {
		t.Fatalf("CreateRawPermissionPolicy nil store created=%v err=%v, want fail closed", created, err)
	}
	if result, err := repo.UpdateRawPermissionPolicy(ctx, []string{"a", "d", "o", "r"}, []string{"b", "d", "o", "r"}); err == nil || result.Found {
		t.Fatalf("UpdateRawPermissionPolicy nil store result=%#v err=%v, want fail closed", result, err)
	}
	if repo.DeleteRawPermissionPolicy(ctx, []string{"alice", "project-1", "model", "read"}) {
		t.Fatal("DeleteRawPermissionPolicy nil store = true, want false")
	}
	if err := repo.UpsertGroupingPolicy(ctx, "project_member", "U1", "viewer", "P1"); err == nil {
		t.Fatal("UpsertGroupingPolicy nil store err = nil, want error")
	}
}

func TestRawPermissionRepositorySourceGuardOwnsRawPermissionResources(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	dir := filepath.Dir(currentFile)
	owned := regexp.MustCompile(`\b(rawPoliciesResource|groupingResource)\b|authorization-policy-service:permission_(?:policies|grouping_policies)|":permission_(?:policies|grouping_policies)"`)

	err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		name := filepath.Base(path)
		if strings.HasSuffix(name, "_test.go") || name == "raw_permission_repository.go" {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if owned.Match(content) {
			t.Errorf("%s directly references raw permission resource ownership; use rawPermissionRepository", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

type rawPermissionConflictStore struct {
	createErr   error
	createCalls int
	updateCalls int
}

func (s *rawPermissionConflictStore) Create(context.Context, string, map[string]any) (contracts.Record[map[string]any], error) {
	s.createCalls++
	return contracts.Record[map[string]any]{}, s.createErr
}

func (s *rawPermissionConflictStore) Get(context.Context, string, string) (contracts.Record[map[string]any], bool) {
	return contracts.Record[map[string]any]{}, false
}

func (s *rawPermissionConflictStore) List(context.Context, string) []contracts.Record[map[string]any] {
	return nil
}

func (s *rawPermissionConflictStore) Update(_ context.Context, _ string, id string, data map[string]any) (contracts.Record[map[string]any], bool) {
	s.updateCalls++
	return contracts.Record[map[string]any]{ID: id, Data: data}, true
}

func (s *rawPermissionConflictStore) Delete(context.Context, string, string) bool {
	return false
}

func (s *rawPermissionConflictStore) NextID(string, string, int, int) string {
	return ""
}
