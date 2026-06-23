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

func TestParseMigrationVersion(t *testing.T) {
	tests := []struct {
		name    string
		want    int
		wantErr string
	}{
		{name: "0001_init.sql", want: 1},
		{name: "42.sql", want: 42},
		{name: "init.sql", wantErr: "must start"},
		{name: "0001init.sql", wantErr: "followed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseMigrationVersion(tt.name)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("parseMigrationVersion(%q) error = %v, want %q", tt.name, err, tt.wantErr)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("parseMigrationVersion(%q) = %d, %v; want %d, nil", tt.name, got, err, tt.want)
			}
		})
	}
}

func TestValidateServiceMigrationsInRootsRejectsBadVersions(t *testing.T) {
	t.Run("missing numeric prefix", func(t *testing.T) {
		root := t.TempDir()
		writeServiceMigration(t, root, "identity-service", "init.sql")
		if _, err := validateServiceMigrationsInRoots([]string{root}); err == nil || !strings.Contains(err.Error(), "invalid migration filename") {
			t.Fatalf("validation error = %v, want invalid filename", err)
		}
	})
	t.Run("duplicate service version", func(t *testing.T) {
		root := t.TempDir()
		writeServiceMigration(t, root, "identity-service", "0001_init.sql")
		writeServiceMigration(t, root, "identity-service", "0001_again.sql")
		if _, err := validateServiceMigrationsInRoots([]string{root}); err == nil || !strings.Contains(err.Error(), "duplicate migration version 1 for identity-service") {
			t.Fatalf("validation error = %v, want duplicate version", err)
		}
	})
}

func TestMigrationUnitsInRootsIncludesPlatformFirstThenServiceVersions(t *testing.T) {
	root := t.TempDir()
	writeAllServiceMigrations(t, root)
	writeServiceMigration(t, root, "identity-service", "0002_extra.sql")

	units, err := migrationUnitsInRoots([]string{root})
	if err != nil {
		t.Fatalf("migration units: %v", err)
	}
	if len(units) != len(serviceMigrationDirs)+2 {
		t.Fatalf("units = %d, want platform + all service migrations + identity extra", len(units))
	}
	if units[0].service != "platform" || units[0].version != 0 || units[0].filename != "schema.sql" {
		t.Fatalf("first unit = %#v, want platform schema v0", units[0])
	}
	if len(units[0].checksum) != 64 {
		t.Fatalf("platform checksum length = %d, want sha256 hex", len(units[0].checksum))
	}
	for i := 2; i < len(units); i++ {
		prev := units[i-1]
		got := units[i]
		if prev.service > got.service || (prev.service == got.service && prev.version > got.version) {
			t.Fatalf("units not ordered by service/version at %d: %#v before %#v", i, prev, got)
		}
	}
}

func TestMigrationChecksumChangesWithSQL(t *testing.T) {
	if migrationChecksum("SELECT 1;") == migrationChecksum("SELECT 2;") {
		t.Fatal("different SQL produced matching checksum")
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
