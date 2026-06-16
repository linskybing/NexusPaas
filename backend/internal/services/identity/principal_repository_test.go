package identity

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

func TestIdentityPrincipalRepositoryUserCreateLookupClone(t *testing.T) {
	ctx := context.Background()
	store := platform.NewStore()
	repo := principalRepositoryFromStore(store)

	if id := repo.NextUserID(); id != "US2600001" {
		t.Fatalf("NextUserID() = %q, want US2600001", id)
	}

	userData := map[string]any{
		"id":       "US1",
		"username": "alice",
		"email":    "alice@test.local",
		"status":   "offline",
		"settings": map[string]any{"theme": "light"},
	}
	created, err := repo.CreateUser(ctx, userData)
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
	userData["username"] = "mutated"
	created.Data["username"] = "mutated"

	found, ok := repo.GetUser(ctx, "US1")
	if !ok || found.Data["username"] != "alice" {
		t.Fatalf("GetUser() = %#v ok=%v, want clone-isolated alice", found.Data, ok)
	}
	if _, ok := repo.FindUserByUsername(ctx, "ALICE"); !ok {
		t.Fatal("FindUserByUsername() did not match case-insensitive username")
	}
	for _, identifier := range []string{"US1", "alice", "alice@test.local"} {
		assertPrincipalIdentifier(t, repo, ctx, identifier)
	}
	if _, err := repo.CreateUser(ctx, map[string]any{"id": "US1"}); !platform.IsCreateConflict(err) {
		t.Fatalf("duplicate CreateUser() err = %v, want create conflict", err)
	}
}

func TestIdentityPrincipalRepositoryUserUpdateSettingsDelete(t *testing.T) {
	ctx := context.Background()
	repo := principalRepositoryFromStore(platform.NewStore())
	if _, err := repo.CreateUser(ctx, map[string]any{
		"id":       "US1",
		"username": "alice",
		"status":   "offline",
		"settings": map[string]any{"theme": "light"},
	}); err != nil {
		t.Fatal(err)
	}

	updated, ok := repo.UpdateUser(ctx, "US1", map[string]any{"full_name": "Alice A"})
	if !ok || updated.Data["full_name"] != "Alice A" {
		t.Fatalf("UpdateUser() = %#v ok=%v, want Alice A", updated.Data, ok)
	}
	if !repo.SetUserStatus(ctx, "US1", "online") {
		t.Fatal("SetUserStatus() = false, want true")
	}
	if settings, ok := repo.GetUserSettings(ctx, "US1"); !ok || settings["theme"] != "light" {
		t.Fatalf("GetUserSettings() = %#v ok=%v, want light", settings, ok)
	}
	settings, ok := repo.UpdateUserSettings(ctx, "US1", map[string]any{"theme": "dark"}, time.Unix(1700000000, 0).UTC())
	if !ok || settings["theme"] != "dark" {
		t.Fatalf("UpdateUserSettings() = %#v ok=%v, want dark", settings, ok)
	}
	settings["theme"] = "mutated"
	storedSettings, ok := repo.GetUserSettings(ctx, "US1")
	if !ok || storedSettings["theme"] != "dark" {
		t.Fatalf("settings mutated through return value: %#v ok=%v", storedSettings, ok)
	}

	if !repo.DeleteUser(ctx, "US1") {
		t.Fatal("DeleteUser() = false, want true")
	}
	if _, ok := repo.GetUser(ctx, "US1"); ok {
		t.Fatal("user still exists after DeleteUser()")
	}
}

func assertPrincipalIdentifier(t *testing.T, repo identityPrincipalRepository, ctx context.Context, identifier string) {
	t.Helper()
	if _, ok := repo.FindUserByIdentifier(ctx, identifier); !ok {
		t.Fatalf("FindUserByIdentifier(%q) = false, want true", identifier)
	}
}

func TestIdentityPrincipalRepositoryRoles(t *testing.T) {
	ctx := context.Background()
	store := platform.NewStore()
	repo := principalRepositoryFromStore(store)
	if _, err := store.Create(ctx, rolesResource, map[string]any{"id": "RO1", "name": "admin", "sort_order": 1}); err != nil {
		t.Fatal(err)
	}

	roles := repo.ListRoles(ctx)
	if len(roles) != 1 || roles[0].Data["name"] != "admin" {
		t.Fatalf("ListRoles() = %#v, want one admin role", roles)
	}
	roles[0].Data["name"] = "mutated"
	role, ok := repo.GetRole(ctx, "RO1")
	if !ok || role.Data["name"] != "admin" {
		t.Fatalf("GetRole() = %#v ok=%v, want clone-isolated admin", role.Data, ok)
	}
	if _, ok := repo.GetRole(ctx, "missing"); ok {
		t.Fatal("GetRole(missing) = true, want false")
	}
}

func TestIdentityPrincipalRepositoryNilStoreFailClosed(t *testing.T) {
	ctx := context.Background()
	repo := recordStoreIdentityPrincipalRepository{}
	if got := repo.NextUserID(); got != "" {
		t.Fatalf("NextUserID nil store = %q, want empty", got)
	}
	if users := repo.ListUsers(ctx); len(users) != 0 {
		t.Fatalf("ListUsers nil store = %#v, want none", users)
	}
	if _, ok := repo.GetUser(ctx, "US1"); ok {
		t.Fatal("GetUser nil store ok=true, want false")
	}
	if _, err := repo.CreateUser(ctx, map[string]any{"id": "US1"}); err == nil {
		t.Fatal("CreateUser nil store err=nil, want fail-closed error")
	}
	if _, ok := repo.UpdateUser(ctx, "US1", map[string]any{"status": "online"}); ok {
		t.Fatal("UpdateUser nil store ok=true, want false")
	}
	if repo.DeleteUser(ctx, "US1") {
		t.Fatal("DeleteUser nil store = true, want false")
	}
	if repo.SetUserStatus(ctx, "US1", "online") {
		t.Fatal("SetUserStatus nil store = true, want false")
	}
	if _, ok := repo.UpdateUserSettings(ctx, "US1", map[string]any{}, time.Now()); ok {
		t.Fatal("UpdateUserSettings nil store ok=true, want false")
	}
	if roles := repo.ListRoles(ctx); len(roles) != 0 {
		t.Fatalf("ListRoles nil store = %#v, want none", roles)
	}
	if _, ok := repo.GetRole(ctx, "RO1"); ok {
		t.Fatal("GetRole nil store ok=true, want false")
	}
}

func TestIdentityPrincipalRepositorySourceGuardOwnsResources(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	dir := filepath.Dir(currentFile)
	owned := regexp.MustCompile(`\b(usersResource|rolesResource)\b|identity-service:(?:users|roles)`)
	allowed := map[string]bool{
		"principal_repository.go": true,
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
			t.Errorf("%s directly references identity users/roles resources; use identityPrincipalRepository", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
