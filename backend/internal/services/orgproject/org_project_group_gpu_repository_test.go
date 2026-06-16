package orgproject

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

func TestOrgProjectGroupGPURepositoryGroupMembershipLifecycle(t *testing.T) {
	ctx := context.Background()
	store := platform.NewStore()
	repo := groupGPURepositoryFromStore(store)

	group, err := repo.CreateGroup(ctx, map[string]any{"id": "G1", "group_name": "vision"})
	if err != nil {
		t.Fatalf("create group: %v", err)
	}
	group.Data["group_name"] = "mutated"
	if found, ok := repo.FindGroup(ctx, "G1"); !ok || found.Data["group_name"] != "vision" {
		t.Fatalf("find group = %#v ok=%v, want clone-isolated vision", found.Data, ok)
	}
	if _, err := repo.CreateGroup(ctx, map[string]any{"id": "G1"}); !platform.IsCreateConflict(err) {
		t.Fatalf("duplicate group err = %v, want create conflict", err)
	}

	old, updated, ok := repo.UpdateGroup(ctx, "G1", map[string]any{"description": "updated"})
	if !ok || old.Data["description"] != nil || updated.Data["description"] != "updated" {
		t.Fatalf("update group old=%#v updated=%#v ok=%v, want old/new split", old.Data, updated.Data, ok)
	}

	created, err := repo.CreateMembership(ctx, map[string]any{"id": membershipID("U1", "G1"), "user_id": "U1", "group_id": "G1", "role": "user"})
	if err != nil {
		t.Fatalf("create membership: %v", err)
	}
	created.Data["role"] = "mutated"
	if found, ok := repo.FindMembership(ctx, "U1", "G1"); !ok || found.Data["role"] != "user" {
		t.Fatalf("find membership = %#v ok=%v, want clone-isolated user role", found.Data, ok)
	}
	_, updatedMembership, ok := repo.UpdateMembershipRole(ctx, "U1", "G1", "manager", time.Now().UTC())
	if !ok || updatedMembership.Data["role"] != "manager" {
		t.Fatalf("update membership = %#v ok=%v, want manager", updatedMembership.Data, ok)
	}
	if deleted, ok := repo.DeleteMembership(ctx, "U1", "G1"); !ok || deleted.Data["role"] != "manager" {
		t.Fatalf("delete membership = %#v ok=%v, want deleted manager", deleted.Data, ok)
	}
	if _, ok := repo.FindMembership(ctx, "U1", "G1"); ok {
		t.Fatal("membership still exists after delete")
	}
}

func TestOrgProjectGroupGPURepositoryGroupCascadeAndLegacyMembershipKey(t *testing.T) {
	ctx := context.Background()
	store := platform.NewStore()
	repo := groupGPURepositoryFromStore(store)
	if _, err := repo.CreateGroup(ctx, map[string]any{"id": "G1", "group_name": "vision"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create(ctx, userGroupsResource, map[string]any{"id": "U1/G1", "user_id": "U1", "group_id": "G1", "role": "user"}); err != nil {
		t.Fatal(err)
	}
	if found, ok := repo.FindMembership(ctx, "U1", "G1"); !ok || found.ID != "U1/G1" {
		t.Fatalf("legacy membership find = %#v ok=%v, want slash-key record", found, ok)
	}

	deleted, memberships, ok := repo.DeleteGroupCascade(ctx, "G1")
	if !ok || deleted.ID != "G1" || memberships != 1 {
		t.Fatalf("delete group cascade deleted=%#v memberships=%d ok=%v, want G1 and one membership", deleted, memberships, ok)
	}
	if _, ok := repo.FindGroup(ctx, "G1"); ok {
		t.Fatal("group still exists after cascade delete")
	}
	if _, ok := repo.FindMembership(ctx, "U1", "G1"); ok {
		t.Fatal("membership still exists after group cascade")
	}
}

func TestOrgProjectGroupGPURepositoryAliasUpdateAndCascade(t *testing.T) {
	ctx := context.Background()
	store := platform.NewStore()
	repo := groupGPURepositoryFromStore(store)
	if _, err := store.Create(ctx, groupsResource, map[string]any{"id": "REC1", "group_name": "vision"}); err != nil {
		t.Fatal(err)
	}
	if _, ok := store.Update(ctx, groupsResource, "REC1", map[string]any{"id": "G-ALIAS", "g_id": "G-ALIAS"}); !ok {
		t.Fatal("failed to create alias-backed group fixture")
	}
	if _, err := store.Create(ctx, userGroupsResource, map[string]any{"id": "U1:G-ALIAS", "user_id": "U1", "group_id": "G-ALIAS", "role": "user"}); err != nil {
		t.Fatal(err)
	}

	old, updated, ok := repo.UpdateGroup(ctx, "G-ALIAS", map[string]any{"description": "updated"})
	if !ok || old.ID != "REC1" || updated.ID != "REC1" || updated.Data["description"] != "updated" {
		t.Fatalf("alias update old=%#v updated=%#v ok=%v, want storage record updated", old, updated, ok)
	}

	deleted, memberships, ok := repo.DeleteGroupCascade(ctx, "G-ALIAS")
	if !ok || deleted.ID != "REC1" || memberships != 1 {
		t.Fatalf("alias delete deleted=%#v memberships=%d ok=%v, want REC1 and one membership", deleted, memberships, ok)
	}
	if _, ok := repo.FindGroup(ctx, "G-ALIAS"); ok {
		t.Fatal("alias group still exists after cascade")
	}
	if _, ok := repo.FindMembership(ctx, "U1", "G-ALIAS"); ok {
		t.Fatal("alias membership still exists after cascade")
	}
}

func TestOrgProjectGroupGPURepositoryGPUClaimLifecycle(t *testing.T) {
	ctx := context.Background()
	repo := groupGPURepositoryFromStore(platform.NewStore())
	claimData := map[string]any{
		"id":         gpuClaimID("P1", "ns-a", "claim-a"),
		"project_id": "P1",
		"namespace":  "ns-a",
		"name":       "claim-a",
		"user_id":    "U1",
	}
	claim, err := repo.CreateGPUClaim(ctx, claimData)
	if err != nil {
		t.Fatalf("create gpu claim: %v", err)
	}
	claimData["name"] = "mutated"
	claim.Data["name"] = "mutated"
	if found, ok := repo.FindGPUClaim(ctx, "P1", "claim-a", "ns-a"); !ok || found.Data["name"] != "claim-a" {
		t.Fatalf("find claim = %#v ok=%v, want clone-isolated claim-a", found.Data, ok)
	}
	if claims := repo.ListGPUClaimsByProject(ctx, "P1"); len(claims) != 1 || claims[0].Data["namespace"] != "ns-a" {
		t.Fatalf("claims by project = %#v, want one ns-a claim", claims)
	}
	if _, err := repo.CreateGPUClaim(ctx, map[string]any{"id": gpuClaimID("P1", "ns-a", "claim-a")}); !platform.IsCreateConflict(err) {
		t.Fatalf("duplicate claim err = %v, want create conflict", err)
	}
	if deleted, ok := repo.DeleteGPUClaim(ctx, gpuClaimID("P1", "ns-a", "claim-a")); !ok || deleted.Data["name"] != "claim-a" {
		t.Fatalf("delete claim = %#v ok=%v, want claim-a", deleted.Data, ok)
	}
	if _, ok := repo.FindGPUClaim(ctx, "P1", "claim-a", "ns-a"); ok {
		t.Fatal("gpu claim still exists after delete")
	}
}

func TestOrgProjectGroupGPURepositoryDeleteGPUClaimsByProject(t *testing.T) {
	ctx := context.Background()
	repo := groupGPURepositoryFromStore(platform.NewStore())
	for _, claim := range []map[string]any{
		{"id": gpuClaimID("P1", "ns", "a"), "project_id": "P1", "namespace": "ns", "name": "a"},
		{"id": gpuClaimID("P1", "ns", "b"), "project_id": "P1", "namespace": "ns", "name": "b"},
		{"id": gpuClaimID("P2", "ns", "a"), "project_id": "P2", "namespace": "ns", "name": "a"},
	} {
		if _, err := repo.CreateGPUClaim(ctx, claim); err != nil {
			t.Fatal(err)
		}
	}

	if deleted := repo.DeleteGPUClaimsByProject(ctx, "P1"); deleted != 2 {
		t.Fatalf("deleted gpu claims = %d, want 2", deleted)
	}
	if claims := repo.ListGPUClaimsByProject(ctx, "P1"); len(claims) != 0 {
		t.Fatalf("P1 claims after delete = %#v, want none", claims)
	}
	if claims := repo.ListGPUClaimsByProject(ctx, "P2"); len(claims) != 1 {
		t.Fatalf("P2 claims after delete = %#v, want one preserved", claims)
	}
}

func TestOrgProjectGroupGPURepositoryNilStoreFailClosed(t *testing.T) {
	ctx := context.Background()
	repo := recordStoreOrgProjectGroupGPURepository{}
	if got := repo.NextGroupID(); got != "" {
		t.Fatalf("NextGroupID nil store = %q, want empty", got)
	}
	if groups := repo.ListGroups(ctx); len(groups) != 0 {
		t.Fatalf("ListGroups nil store = %#v, want empty", groups)
	}
	if _, err := repo.CreateGroup(ctx, map[string]any{"id": "G1"}); err == nil {
		t.Fatal("CreateGroup nil store err = nil, want fail-closed error")
	}
	if _, _, ok := repo.UpdateGroup(ctx, "G1", map[string]any{}); ok {
		t.Fatal("UpdateGroup nil store ok=true, want false")
	}
	if _, _, ok := repo.DeleteGroupCascade(ctx, "G1"); ok {
		t.Fatal("DeleteGroupCascade nil store ok=true, want false")
	}
	if _, err := repo.CreateMembership(ctx, map[string]any{"id": "U1:G1"}); err == nil {
		t.Fatal("CreateMembership nil store err = nil, want fail-closed error")
	}
	if _, err := repo.CreateGPUClaim(ctx, map[string]any{"id": "P1:ns:claim"}); err == nil {
		t.Fatal("CreateGPUClaim nil store err = nil, want fail-closed error")
	}
	if deleted := repo.DeleteGPUClaimsByProject(ctx, "P1"); deleted != 0 {
		t.Fatalf("DeleteGPUClaimsByProject nil store = %d, want 0", deleted)
	}
}

func TestOrgProjectGroupGPURepositorySourceGuardOwnsResources(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	dir := filepath.Dir(currentFile)
	owned := regexp.MustCompile(`\b(groupsResource|userGroupsResource|gpuClaimsResource)\b|org-project-service:(?:groups|user_groups|gpu_claims)|":(?:groups|user_groups|gpu_claims)"`)
	allowed := map[string]bool{
		"org_project_group_gpu_repository.go": true,
	}

	err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		name := filepath.Base(path)
		if strings.HasSuffix(name, "_test.go") || allowed[name] {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if owned.Match(content) {
			t.Errorf("%s directly references org-project group/GPU resources; use orgProjectGroupGPURepository", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
