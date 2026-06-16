package platform

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed schema.sql
var platformSchemaSQL string

var serviceMigrationDirs = []string{
	"audit-compliance-service",
	"authorization-policy-service",
	"ide-service",
	"identity-service",
	"image-registry-service",
	"integration-proxy-service",
	"k8s-control-service",
	"media-upload-service",
	"org-project-service",
	"platform-gateway",
	"request-notification-service",
	"scheduler-quota-service",
	"storage-service",
	"usage-observability-service",
	"workload-service",
}

type discoveredMigration struct {
	path    string
	service string
}

// ApplyMigrations connects to databaseURL and applies the embedded platform
// schema (the table backing PostgresStore). It then applies every service-owned
// migration found under deterministic migration roots used by local dev, tests,
// and the runtime image. All statements are idempotent (`CREATE TABLE IF NOT
// EXISTS`), so re-running is safe.
func ApplyMigrations(ctx context.Context, databaseURL string) error {
	if strings.TrimSpace(databaseURL) == "" {
		return fmt.Errorf("DATABASE_URL is required for apply-migrations")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return fmt.Errorf("connect database: %w", err)
	}
	defer pool.Close()

	if _, err := pool.Exec(ctx, platformSchemaSQL); err != nil {
		return fmt.Errorf("apply platform schema: %w", err)
	}
	applied := 1

	files, err := validateServiceMigrationsInRoots(defaultMigrationRoots())
	if err != nil {
		return err
	}
	for _, path := range files {
		sql, readErr := os.ReadFile(path)
		if readErr != nil {
			return fmt.Errorf("read %s: %w", path, readErr)
		}
		if _, execErr := pool.Exec(ctx, string(sql)); execErr != nil {
			return fmt.Errorf("apply %s: %w", path, execErr)
		}
		applied++
	}
	fmt.Printf("applied %d migration unit(s) (platform schema + %d service files)\n", applied, len(files))
	return nil
}

func validateServiceMigrations() ([]string, error) {
	return validateServiceMigrationsInRoots(defaultMigrationRoots())
}

func validateServiceMigrationsInRoots(roots []string) ([]string, error) {
	files, foundByService, err := discoverServiceMigrationsInRoots(roots)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no service migration files found")
	}
	var missing []string
	for _, dir := range serviceMigrationDirs {
		if !foundByService[dir] {
			missing = append(missing, dir)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("missing service migration files for: %s", strings.Join(missing, ", "))
	}
	return files, nil
}

func discoverServiceMigrationsInRoots(roots []string) ([]string, map[string]bool, error) {
	seen := map[string]discoveredMigration{}
	for _, root := range canonicalMigrationRoots(roots) {
		if err := discoverServiceMigrationsInRoot(root, seen); err != nil {
			return nil, nil, err
		}
	}
	return discoveredMigrationResults(seen), discoveredServices(seen), nil
}

func discoverServiceMigrationsInRoot(root string, seen map[string]discoveredMigration) error {
	for _, serviceDir := range serviceMigrationDirs {
		migrationDir := filepath.Join(root, serviceDir, "migrations")
		if err := discoverServiceMigrationDir(migrationDir, serviceDir, seen); err != nil {
			return err
		}
	}
	return nil
}

func discoverServiceMigrationDir(migrationDir string, serviceDir string, seen map[string]discoveredMigration) error {
	entries, err := os.ReadDir(migrationDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read migration dir %s: %w", migrationDir, err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		path := filepath.Join(migrationDir, entry.Name())
		seen[migrationDedupeKey(path)] = discoveredMigration{path: path, service: serviceDir}
	}
	return nil
}

func discoveredMigrationResults(seen map[string]discoveredMigration) []string {
	files := make([]string, 0, len(seen))
	for _, item := range seen {
		files = append(files, item.path)
	}
	sort.Slice(files, func(i, j int) bool {
		return filepath.ToSlash(files[i]) < filepath.ToSlash(files[j])
	})
	return files
}

func discoveredServices(seen map[string]discoveredMigration) map[string]bool {
	foundByService := map[string]bool{}
	for _, item := range seen {
		foundByService[item.service] = true
	}
	return foundByService
}

func defaultMigrationRoots() []string {
	return []string{".", "..", "../..", "/app"}
}

func canonicalMigrationRoots(roots []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(roots))
	for _, root := range roots {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		key := migrationDedupeKey(root)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, root)
	}
	return out
}

func migrationDedupeKey(path string) string {
	if abs, err := filepath.Abs(path); err == nil {
		return filepath.Clean(abs)
	}
	return filepath.Clean(path)
}
