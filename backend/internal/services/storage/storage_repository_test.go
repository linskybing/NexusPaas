package storage

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

func TestStorageRepositoryGroupStorageCascadeAndCloneIsolation(t *testing.T) {
	ctx := context.Background()
	app := platform.NewApp(platform.Config{ServiceName: "all"})
	repo := storageRepo(app)

	input := map[string]any{
		"id":       groupStorageID("G1", "pvc1"),
		"group_id": "G1",
		"pvc_id":   "pvc1",
		"name":     "datasets",
		"status":   "created",
	}
	created, err := repo.CreateGroupStorage(ctx, input)
	if err != nil {
		t.Fatalf("create group storage: %v", err)
	}
	input["name"] = "mutated"
	if created["name"] != "datasets" {
		t.Fatalf("created storage was not cloned: %#v", created)
	}
	source, found := repo.FindGroupStorageSource(ctx, "G1", "pvc1")
	if !found || source["name"] != "datasets" {
		t.Fatalf("source = %#v found=%v, want cloned source", source, found)
	}

	updated, ok := repo.UpdateGroupStorageStatus(ctx, "G1", "pvc1", "running", time.Unix(10, 0).UTC())
	if !ok || updated["status"] != "running" {
		t.Fatalf("updated storage = %#v ok=%v, want running", updated, ok)
	}
	if len(repo.ListGroupStorageByGroup(ctx, "G1")) != 1 {
		t.Fatalf("group storage list = %#v, want one", repo.ListGroupStorageByGroup(ctx, "G1"))
	}

	mustUpsertStoragePermission(t, repo, map[string]any{
		"id":         storagePermissionID("G1", "pvc1", "U1"),
		"group_id":   "G1",
		"pvc_id":     "pvc1",
		"user_id":    "U1",
		"permission": "read_write",
	})
	if _, err := repo.UpsertStoragePolicy(ctx, map[string]any{
		"id":                 storagePolicyID("G1", "pvc1"),
		"group_id":           "G1",
		"pvc_id":             "pvc1",
		"default_permission": "read_only",
	}); err != nil {
		t.Fatalf("upsert policy: %v", err)
	}

	if !repo.DeleteGroupStorageCascade(ctx, "G1", "pvc1") {
		t.Fatal("delete group storage cascade = false, want true")
	}
	assertStorageRepoMissing(t, app, groupStorageResource, groupStorageID("G1", "pvc1"))
	assertStorageRepoMissing(t, app, storagePermissionsResource, storagePermissionID("G1", "pvc1", "U1"))
	assertStorageRepoMissing(t, app, storagePoliciesResource, storagePolicyID("G1", "pvc1"))
}

func TestStorageRepositoryProjectBindingCascadeAndPermissionPrecedence(t *testing.T) {
	ctx := context.Background()
	app := platform.NewApp(platform.Config{ServiceName: "all"})
	repo := storageRepo(app)

	if _, err := repo.CreateGroupStorage(ctx, map[string]any{
		"id":       groupStorageID("G1", "pvc1"),
		"group_id": "G1",
		"pvc_id":   "pvc1",
		"status":   "running",
	}); err != nil {
		t.Fatalf("create group storage: %v", err)
	}
	if _, err := repo.CreateProjectBinding(ctx, map[string]any{
		"id":         projectBindingID("P1", "pvc1"),
		"project_id": "P1",
		"group_id":   "G1",
		"pvc_id":     "pvc1",
	}); err != nil {
		t.Fatalf("create binding: %v", err)
	}
	if _, found := repo.FindProjectStorageBinding(ctx, "P1", "pvc1"); !found {
		t.Fatal("project storage binding not found")
	}
	if len(repo.ListProjectBindings(ctx, "P1")) != 1 {
		t.Fatalf("project bindings = %#v, want one", repo.ListProjectBindings(ctx, "P1"))
	}

	mustUpsertStoragePermission(t, repo, map[string]any{
		"id":         storagePermissionID("G1", "pvc1", "U1"),
		"group_id":   "G1",
		"pvc_id":     "pvc1",
		"user_id":    "U1",
		"permission": "read_write",
	})
	if _, err := repo.UpsertStoragePolicy(ctx, map[string]any{
		"id":                 storagePolicyID("G1", "pvc1"),
		"group_id":           "G1",
		"pvc_id":             "pvc1",
		"default_permission": "read_only",
	}); err != nil {
		t.Fatalf("upsert policy: %v", err)
	}
	if _, err := repo.UpsertProjectPermission(ctx, map[string]any{
		"id":         projectPermissionID("P1", "pvc1", "U1"),
		"project_id": "P1",
		"pvc_id":     "pvc1",
		"user_id":    "U1",
		"permission": "none",
	}); err != nil {
		t.Fatalf("upsert project permission: %v", err)
	}

	if got := repo.EffectiveStoragePermission(ctx, "P1", "G1", "pvc1", "U1"); got != "none" {
		t.Fatalf("effective permission = %q, want project-level none", got)
	}
	repo.DeleteProjectPermission(ctx, "P1", "pvc1", "U1")
	if got := repo.EffectiveStoragePermission(ctx, "P1", "G1", "pvc1", "U1"); got != "read_write" {
		t.Fatalf("effective permission = %q, want group permission", got)
	}
	repo.DeleteStoragePermission(ctx, "G1", "pvc1", "U1")
	if got := repo.EffectiveStoragePermission(ctx, "P1", "G1", "pvc1", "U1"); got != "read_only" {
		t.Fatalf("effective permission = %q, want policy default", got)
	}

	if _, err := repo.UpsertProjectPermission(ctx, map[string]any{
		"id":         projectPermissionID("P1", "pvc1", "U2"),
		"project_id": "P1",
		"pvc_id":     "pvc1",
		"user_id":    "U2",
		"permission": "read_only",
	}); err != nil {
		t.Fatalf("upsert project permission: %v", err)
	}
	if !repo.DeleteProjectBindingCascade(ctx, "P1", "pvc1") {
		t.Fatal("delete project binding cascade = false, want true")
	}
	assertStorageRepoMissing(t, app, projectBindingsResource, projectBindingID("P1", "pvc1"))
	assertStorageRepoMissing(t, app, projectPermissionsResource, projectPermissionID("P1", "pvc1", "U2"))
}

func TestStorageRepositoryFastTransferUserStorageAndNilStore(t *testing.T) {
	ctx := context.Background()
	app := platform.NewApp(platform.Config{ServiceName: "all"})
	repo := storageRepo(app)

	name := repo.NextFastTransferName()
	if !strings.HasPrefix(name, "transfer-") {
		t.Fatalf("next transfer name = %q, want transfer-*", name)
	}
	transfer := map[string]any{
		"id":               fastTransferID("P1", "project-P1", name),
		"project_id":       "P1",
		"target_namespace": "project-P1",
		"name":             name,
		"status":           "staged",
	}
	created, err := repo.CreateFastTransfer(ctx, transfer)
	if err != nil {
		t.Fatalf("create transfer: %v", err)
	}
	transfer["status"] = "mutated"
	found, ok := repo.GetFastTransfer(ctx, "P1", "project-P1", name)
	if !ok || found["status"] != "staged" || created["status"] != "staged" {
		t.Fatalf("transfer found=%#v ok=%v created=%#v, want staged clones", found, ok, created)
	}
	cancelled, ok := repo.CancelFastTransfer(ctx, "P1", "project-P1", name, time.Unix(20, 0).UTC())
	if !ok || cancelled["status"] != "cancelled" {
		t.Fatalf("cancelled transfer = %#v ok=%v, want cancelled", cancelled, ok)
	}

	if missing := repo.UserStorageStatus(ctx, "alice"); missing["status"] != "missing" {
		t.Fatalf("missing user storage = %#v", missing)
	}
	saved, err := repo.UpsertUserStorage(ctx, "alice", map[string]any{"username": "alice", "status": "initialized", "size": "10Gi"})
	if err != nil {
		t.Fatalf("upsert user storage: %v", err)
	}
	if saved["id"] != "alice" || repo.UserStorageStatus(ctx, "alice")["status"] != "initialized" {
		t.Fatalf("user storage saved=%#v status=%#v", saved, repo.UserStorageStatus(ctx, "alice"))
	}

	var nilRepo recordStoreStorageRepository
	if _, err := nilRepo.CreateGroupStorage(ctx, map[string]any{}); err == nil {
		t.Fatal("nil store create group storage err = nil, want error")
	}
	if rows := nilRepo.ListGroupStorage(ctx); rows != nil {
		t.Fatalf("nil store list = %#v, want nil", rows)
	}
	if _, ok := nilRepo.UpdateGroupStorageStatus(ctx, "G1", "pvc1", "running", time.Now()); ok {
		t.Fatal("nil store update = true, want false")
	}
	if got := nilRepo.EffectiveStoragePermission(ctx, "P1", "G1", "pvc1", "U1"); got != "none" {
		t.Fatalf("nil store effective permission = %q, want none", got)
	}
}

func TestStorageRepositorySourceGuard(t *testing.T) {
	dir := currentStoragePackageDir(t)
	violations, err := storageSourceGuardViolations(dir, storageRepositoryGuardTokens())
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) > 0 {
		t.Fatalf("storage-owned physical/access resources must go through storageRepository:\n%s", strings.Join(violations, "\n"))
	}
}

func currentStoragePackageDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime caller failed")
	}
	return filepath.Dir(file)
}

func storageRepositoryGuardTokens() []string {
	return []string{
		"groupStorageResource",
		"storagePermissionsResource",
		"storagePoliciesResource",
		"projectBindingsResource",
		"projectPermissionsResource",
		"userStorageResource",
		"fastTransfersResource",
		"storage-service:group_storage",
		"storage-service:storage_permissions",
		"storage-service:storage_access_policies",
		"storage-service:storage_bindings",
		"storage-service:project_storage_permissions",
		"storage-service:user_storage",
		"storage-service:fast_transfers",
	}
}

func storageSourceGuardViolations(dir string, tokens []string) ([]string, error) {
	violations := make([]string, 0)
	err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !shouldScanStorageSource(path, entry) {
			return nil
		}
		matches, err := storageGuardMatches(path, tokens)
		violations = append(violations, matches...)
		return err
	})
	return violations, err
}

func shouldScanStorageSource(path string, entry os.DirEntry) bool {
	if entry.IsDir() || filepath.Ext(path) != ".go" {
		return false
	}
	base := filepath.Base(path)
	return !strings.HasSuffix(base, "_test.go") && base != "storage_repository.go" && base != "longhorn_rwx_health.go"
}

func storageGuardMatches(path string, tokens []string) ([]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	body := string(raw)
	matches := make([]string, 0)
	for _, token := range tokens {
		if strings.Contains(body, token) {
			matches = append(matches, path+" references "+token)
		}
	}
	return matches, nil
}

func mustUpsertStoragePermission(t *testing.T, repo *recordStoreStorageRepository, data map[string]any) {
	t.Helper()
	if _, err := repo.UpsertStoragePermission(context.Background(), data); err != nil {
		t.Fatalf("upsert storage permission: %v", err)
	}
}

func assertStorageRepoMissing(t *testing.T, app *platform.App, resource, id string) {
	t.Helper()
	if _, found := app.Store.Get(context.Background(), resource, id); found {
		t.Fatalf("%s/%s unexpectedly exists", resource, id)
	}
}
