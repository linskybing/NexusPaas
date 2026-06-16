package platform

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestDiscoverServiceMigrationsInRootsIsDeterministicScopedAndSorted(t *testing.T) {
	root := t.TempDir()
	writeServiceMigration(t, root, "identity-service", "0002_extra.sql")
	writeServiceMigration(t, root, "identity-service", "0001_init.sql")
	writeServiceMigration(t, root, "workload-service", "0001_init.sql")
	writeTestFile(t, filepath.Join(root, "other", "migrations", "0001_init.sql"), "-- ignored\n")
	writeTestFile(t, filepath.Join(root, "identity-service", "not-migrations", "0001_init.sql"), "-- ignored\n")

	files, foundByService, err := discoverServiceMigrationsInRoots([]string{root})
	if err != nil {
		t.Fatalf("discover service migrations: %v", err)
	}
	wantSuffixes := []string{
		"identity-service/migrations/0001_init.sql",
		"identity-service/migrations/0002_extra.sql",
		"workload-service/migrations/0001_init.sql",
	}
	assertPathSuffixes(t, files, wantSuffixes)
	if !foundByService["identity-service"] || !foundByService["workload-service"] {
		t.Fatalf("found services = %#v, want identity-service and workload-service", foundByService)
	}
	for _, file := range files {
		if strings.Contains(filepath.ToSlash(file), "other/migrations") {
			t.Fatalf("unexpected arbitrary migration path discovered: %s", file)
		}
	}
}

func TestDiscoverServiceMigrationsInRootsDeduplicatesRoots(t *testing.T) {
	root := t.TempDir()
	writeServiceMigration(t, root, "identity-service", "0001_init.sql")

	files, _, err := discoverServiceMigrationsInRoots([]string{root, root, filepath.Clean(root)})
	if err != nil {
		t.Fatalf("discover service migrations: %v", err)
	}
	assertPathSuffixes(t, files, []string{"identity-service/migrations/0001_init.sql"})
}

func TestValidateServiceMigrationsInRootsRequiresAllKnownServices(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		if _, err := validateServiceMigrationsInRoots([]string{t.TempDir()}); err == nil || !strings.Contains(err.Error(), "no service migration files found") {
			t.Fatalf("empty validation error = %v, want no files", err)
		}
	})
	t.Run("partial", func(t *testing.T) {
		root := t.TempDir()
		writeServiceMigration(t, root, "identity-service", "0001_init.sql")
		if _, err := validateServiceMigrationsInRoots([]string{root}); err == nil || !strings.Contains(err.Error(), "missing service migration files") {
			t.Fatalf("partial validation error = %v, want missing service dirs", err)
		}
	})
	t.Run("complete", func(t *testing.T) {
		root := t.TempDir()
		writeAllServiceMigrations(t, root)
		files, err := validateServiceMigrationsInRoots([]string{root})
		if err != nil {
			t.Fatalf("complete validation error = %v, want nil", err)
		}
		if len(files) != len(serviceMigrationDirs) {
			t.Fatalf("validated files = %d, want %d", len(files), len(serviceMigrationDirs))
		}
	})
}

func assertPathSuffixes(t *testing.T, files, suffixes []string) {
	t.Helper()
	got := make([]string, 0, len(files))
	for _, file := range files {
		got = append(got, filepath.ToSlash(file))
	}
	for i, suffix := range suffixes {
		suffixes[i] = filepath.ToSlash(suffix)
	}
	if len(got) != len(suffixes) {
		t.Fatalf("files = %v, want suffixes %v", got, suffixes)
	}
	if !slices.IsSorted(got) {
		t.Fatalf("files are not sorted: %v", got)
	}
	for i, suffix := range suffixes {
		if !strings.HasSuffix(got[i], suffix) {
			t.Fatalf("file[%d] = %s, want suffix %s (all files %v)", i, got[i], suffix, got)
		}
	}
}

func writeAllServiceMigrations(t *testing.T, root string) {
	t.Helper()
	for _, serviceDir := range serviceMigrationDirs {
		writeServiceMigration(t, root, serviceDir, "0001_init.sql")
	}
}

func writeServiceMigration(t *testing.T, root, serviceDir, name string) {
	t.Helper()
	writeTestFile(t, filepath.Join(root, serviceDir, "migrations", name), "-- migration\n")
}

func writeTestFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
