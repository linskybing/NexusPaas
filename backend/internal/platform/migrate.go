package platform

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
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
	path     string
	service  string
	version  int
	filename string
}

type migrationUnit struct {
	service  string
	version  int
	filename string
	path     string
	sql      string
	checksum string
}

type dirtyMigration struct {
	service  string
	version  int
	filename string
}

const migrationAdvisoryLockKey int64 = 569842761603

const migrationLedgerDDL = `
CREATE TABLE IF NOT EXISTS platform_schema_migrations (
    service TEXT NOT NULL,
    version INTEGER NOT NULL,
    filename TEXT NOT NULL,
    checksum TEXT NOT NULL,
    dirty BOOLEAN NOT NULL DEFAULT false,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    duration_ms INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (service, version),
    CHECK (version >= 0),
    CHECK (duration_ms >= 0),
    CHECK (checksum <> ''),
    CHECK (filename <> '')
);
`

// ApplyMigrations connects to databaseURL and applies the embedded platform
// schema plus service-owned migrations through the platform migration ledger.
func ApplyMigrations(ctx context.Context, databaseURL string) error {
	return applyMigrationsInRoots(ctx, databaseURL, defaultMigrationRoots())
}

func applyMigrationsInRoots(ctx context.Context, databaseURL string, roots []string) error {
	if strings.TrimSpace(databaseURL) == "" {
		return fmt.Errorf("DATABASE_URL is required for apply-migrations")
	}
	units, err := migrationUnitsInRoots(roots)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return fmt.Errorf("connect database: %w", err)
	}
	defer pool.Close()

	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire migration connection: %w", err)
	}
	defer conn.Release()

	locked, err := acquireMigrationAdvisoryLock(ctx, conn)
	if err != nil {
		return err
	}
	if !locked {
		return fmt.Errorf("migration advisory lock is already held")
	}
	defer releaseMigrationAdvisoryLock(conn)

	if _, err := conn.Exec(ctx, migrationLedgerDDL); err != nil {
		return fmt.Errorf("bootstrap migration ledger: %w", err)
	}
	if dirty, err := findDirtyMigration(ctx, conn); err != nil {
		return err
	} else if dirty != nil {
		return fmt.Errorf("dirty migration blocks run: service=%s version=%d filename=%s", dirty.service, dirty.version, dirty.filename)
	}

	applied := 0
	skipped := 0
	for _, unit := range units {
		unitApplied, err := applyMigrationUnit(ctx, conn, unit)
		if err != nil {
			return err
		}
		if unitApplied {
			applied++
		} else {
			skipped++
		}
	}
	fmt.Printf("migration summary: applied=%d skipped=%d total=%d\n", applied, skipped, len(units))
	return nil
}

func validateServiceMigrations() ([]string, error) {
	return validateServiceMigrationsInRoots(defaultMigrationRoots())
}

func validateServiceMigrationsInRoots(roots []string) ([]string, error) {
	migrations, foundByService, err := discoverServiceMigrationMetadataInRoots(roots)
	if err != nil {
		return nil, err
	}
	if err := validateDiscoveredServiceMigrations(migrations, foundByService); err != nil {
		return nil, err
	}
	return discoveredMigrationResults(migrations), nil
}

func validateDiscoveredServiceMigrations(migrations []discoveredMigration, foundByService map[string]bool) error {
	if len(migrations) == 0 {
		return fmt.Errorf("no service migration files found")
	}
	var missing []string
	for _, dir := range serviceMigrationDirs {
		if !foundByService[dir] {
			missing = append(missing, dir)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing service migration files for: %s", strings.Join(missing, ", "))
	}
	return nil
}

func discoverServiceMigrationsInRoots(roots []string) ([]string, map[string]bool, error) {
	migrations, foundByService, err := discoverServiceMigrationMetadataInRoots(roots)
	if err != nil {
		return nil, nil, err
	}
	return discoveredMigrationResults(migrations), foundByService, nil
}

func discoverServiceMigrationMetadataInRoots(roots []string) ([]discoveredMigration, map[string]bool, error) {
	seen := map[string]discoveredMigration{}
	versionsByService := map[string]map[int]string{}
	for _, root := range canonicalMigrationRoots(roots) {
		if err := discoverServiceMigrationsInRoot(root, seen, versionsByService); err != nil {
			return nil, nil, err
		}
	}
	migrations := discoveredMigrationItems(seen)
	return migrations, discoveredServices(migrations), nil
}

func discoverServiceMigrationsInRoot(root string, seen map[string]discoveredMigration, versionsByService map[string]map[int]string) error {
	for _, serviceDir := range serviceMigrationDirs {
		migrationDir := filepath.Join(root, "migrations", serviceDir)
		if err := discoverServiceMigrationDir(migrationDir, serviceDir, seen, versionsByService); err != nil {
			return err
		}
	}
	return nil
}

func discoverServiceMigrationDir(migrationDir string, serviceDir string, seen map[string]discoveredMigration, versionsByService map[string]map[int]string) error {
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
		key := migrationDedupeKey(path)
		if _, ok := seen[key]; ok {
			continue
		}
		version, err := parseMigrationVersion(entry.Name())
		if err != nil {
			return fmt.Errorf("invalid migration filename %s: %w", path, err)
		}
		if versionsByService[serviceDir] == nil {
			versionsByService[serviceDir] = map[int]string{}
		}
		if existing := versionsByService[serviceDir][version]; existing != "" {
			return fmt.Errorf("duplicate migration version %d for %s: %s and %s", version, serviceDir, existing, path)
		}
		versionsByService[serviceDir][version] = path
		seen[key] = discoveredMigration{
			path:     path,
			service:  serviceDir,
			version:  version,
			filename: entry.Name(),
		}
	}
	return nil
}

func discoveredMigrationItems(seen map[string]discoveredMigration) []discoveredMigration {
	items := make([]discoveredMigration, 0, len(seen))
	for _, item := range seen {
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].service != items[j].service {
			return items[i].service < items[j].service
		}
		if items[i].version != items[j].version {
			return items[i].version < items[j].version
		}
		return filepath.ToSlash(items[i].path) < filepath.ToSlash(items[j].path)
	})
	return items
}

func discoveredMigrationResults(items []discoveredMigration) []string {
	files := make([]string, 0, len(items))
	for _, item := range items {
		files = append(files, item.path)
	}
	return files
}

func discoveredServices(items []discoveredMigration) map[string]bool {
	foundByService := map[string]bool{}
	for _, item := range items {
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

func migrationUnitsInRoots(roots []string) ([]migrationUnit, error) {
	migrations, foundByService, err := discoverServiceMigrationMetadataInRoots(roots)
	if err != nil {
		return nil, err
	}
	if err := validateDiscoveredServiceMigrations(migrations, foundByService); err != nil {
		return nil, err
	}
	units := []migrationUnit{platformMigrationUnit()}
	for _, migration := range migrations {
		sqlBytes, err := os.ReadFile(migration.path)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", migration.path, err)
		}
		sql := string(sqlBytes)
		units = append(units, migrationUnit{
			service:  migration.service,
			version:  migration.version,
			filename: migration.filename,
			path:     migration.path,
			sql:      sql,
			checksum: migrationChecksum(sql),
		})
	}
	return units, nil
}

func platformMigrationUnit() migrationUnit {
	return migrationUnit{
		service:  "platform",
		version:  0,
		filename: "schema.sql",
		path:     "schema.sql",
		sql:      platformSchemaSQL,
		checksum: migrationChecksum(platformSchemaSQL),
	}
}

func parseMigrationVersion(filename string) (int, error) {
	base := filepath.Base(filename)
	digits := 0
	for digits < len(base) && base[digits] >= '0' && base[digits] <= '9' {
		digits++
	}
	if digits == 0 {
		return 0, fmt.Errorf("must start with a numeric version")
	}
	if digits < len(base) && base[digits] != '_' && base[digits:] != ".sql" {
		return 0, fmt.Errorf("numeric version must be followed by '_' or '.sql'")
	}
	version, err := strconv.Atoi(base[:digits])
	if err != nil || version < 0 {
		return 0, fmt.Errorf("invalid numeric version")
	}
	return version, nil
}

func migrationChecksum(sql string) string {
	sum := sha256.Sum256([]byte(sql))
	return hex.EncodeToString(sum[:])
}

func acquireMigrationAdvisoryLock(ctx context.Context, conn *pgxpool.Conn) (bool, error) {
	var locked bool
	if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, migrationAdvisoryLockKey).Scan(&locked); err != nil {
		return false, fmt.Errorf("acquire migration advisory lock: %w", err)
	}
	return locked, nil
}

func releaseMigrationAdvisoryLock(conn *pgxpool.Conn) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, _ = conn.Exec(ctx, `SELECT pg_advisory_unlock($1)`, migrationAdvisoryLockKey)
}

func findDirtyMigration(ctx context.Context, conn *pgxpool.Conn) (*dirtyMigration, error) {
	var dirty dirtyMigration
	err := conn.QueryRow(ctx, `
SELECT service, version, filename
FROM platform_schema_migrations
WHERE dirty = true
ORDER BY service, version
LIMIT 1`).Scan(&dirty.service, &dirty.version, &dirty.filename)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("check dirty migrations: %w", err)
	}
	return &dirty, nil
}

func applyMigrationUnit(ctx context.Context, conn *pgxpool.Conn, unit migrationUnit) (bool, error) {
	applied, err := migrationAlreadyApplied(ctx, conn, unit)
	if err != nil {
		return false, err
	}
	if applied {
		return false, nil
	}
	if err := markMigrationDirty(ctx, conn, unit); err != nil {
		return false, err
	}
	start := time.Now()
	tx, err := conn.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("begin migration transaction service=%s version=%d filename=%s: %w", unit.service, unit.version, unit.filename, err)
	}
	if _, err := tx.Exec(ctx, unit.sql); err != nil {
		_ = tx.Rollback(context.Background())
		return false, fmt.Errorf("apply migration service=%s version=%d filename=%s: %w", unit.service, unit.version, unit.filename, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit migration service=%s version=%d filename=%s: %w", unit.service, unit.version, unit.filename, err)
	}
	durationMS := time.Since(start).Milliseconds()
	tag, err := conn.Exec(ctx, `
UPDATE platform_schema_migrations
SET dirty = false, checksum = $3, filename = $4, applied_at = now(), duration_ms = $5
WHERE service = $1 AND version = $2`, unit.service, unit.version, unit.checksum, unit.filename, durationMS)
	if err != nil {
		return false, fmt.Errorf("clear dirty migration service=%s version=%d filename=%s: %w", unit.service, unit.version, unit.filename, err)
	}
	if tag.RowsAffected() != 1 {
		return false, fmt.Errorf("clear dirty migration service=%s version=%d filename=%s: ledger row not updated", unit.service, unit.version, unit.filename)
	}
	return true, nil
}

func migrationAlreadyApplied(ctx context.Context, conn *pgxpool.Conn, unit migrationUnit) (bool, error) {
	var checksum string
	var dirty bool
	var filename string
	err := conn.QueryRow(ctx, `
SELECT checksum, dirty, filename
FROM platform_schema_migrations
WHERE service = $1 AND version = $2`, unit.service, unit.version).Scan(&checksum, &dirty, &filename)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read migration ledger service=%s version=%d filename=%s: %w", unit.service, unit.version, unit.filename, err)
	}
	if dirty {
		return false, fmt.Errorf("dirty migration blocks run: service=%s version=%d filename=%s", unit.service, unit.version, filename)
	}
	if checksum != unit.checksum {
		return false, fmt.Errorf("migration checksum mismatch: service=%s version=%d filename=%s", unit.service, unit.version, unit.filename)
	}
	return true, nil
}

func markMigrationDirty(ctx context.Context, conn *pgxpool.Conn, unit migrationUnit) error {
	if _, err := conn.Exec(ctx, `
INSERT INTO platform_schema_migrations (service, version, filename, checksum, dirty, applied_at, duration_ms)
VALUES ($1, $2, $3, $4, true, now(), 0)`, unit.service, unit.version, unit.filename, unit.checksum); err != nil {
		return fmt.Errorf("mark migration dirty service=%s version=%d filename=%s: %w", unit.service, unit.version, unit.filename, err)
	}
	return nil
}
