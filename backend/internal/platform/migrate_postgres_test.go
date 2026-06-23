//go:build integration

package platform

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestMigrationDirtyMarkerPersistsAfterFailedSQL(t *testing.T) {
	ctx := context.Background()
	db := isolatedMigrationDatabase(t)
	root := t.TempDir()
	writeAllServiceMigrationsWithSQL(t, root, func(string) string {
		return "-- noop\n"
	})
	writeServiceMigrationWithSQL(t, root, "identity-service", "0002_fail.sql", "SELECT * FROM missing_migration_table;\n")

	err := applyMigrationsInRoots(ctx, db.url, []string{root})
	if err == nil || !strings.Contains(err.Error(), "apply migration service=identity-service version=2") {
		t.Fatalf("apply error = %v, want failed identity migration", err)
	}

	pool := openMigrationTestPool(t, db)
	defer pool.Close()
	var dirty bool
	if err := pool.QueryRow(ctx, `
SELECT dirty
FROM platform_schema_migrations
WHERE service = 'identity-service' AND version = 2`).Scan(&dirty); err != nil {
		t.Fatalf("query dirty row: %v", err)
	}
	if !dirty {
		t.Fatal("failed migration row dirty = false, want true")
	}

	err = applyMigrationsInRoots(ctx, db.url, []string{root})
	if err == nil || !strings.Contains(err.Error(), "dirty migration blocks run") {
		t.Fatalf("second apply error = %v, want dirty-state block", err)
	}
}

func TestMigrationLedgerAdoptsPreLedgerDatabaseThenSkips(t *testing.T) {
	ctx := context.Background()
	db := isolatedMigrationDatabase(t)
	root := t.TempDir()
	writeAllServiceMigrationsWithSQL(t, root, func(service string) string {
		return fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (id TEXT PRIMARY KEY);\n", migrationTestTableName(service))
	})

	pool := openMigrationTestPool(t, db)
	defer pool.Close()
	if _, err := pool.Exec(ctx, platformSchemaSQL); err != nil {
		t.Fatalf("seed platform schema: %v", err)
	}
	for _, service := range serviceMigrationDirs {
		sqlBytes, err := os.ReadFile(filepath.Join(root, service, "migrations", "0001_init.sql"))
		if err != nil {
			t.Fatalf("read seed migration: %v", err)
		}
		if _, err := pool.Exec(ctx, string(sqlBytes)); err != nil {
			t.Fatalf("seed service migration %s: %v", service, err)
		}
	}
	assertLedgerAbsent(t, ctx, pool)

	if err := applyMigrationsInRoots(ctx, db.url, []string{root}); err != nil {
		t.Fatalf("first ledger adoption apply: %v", err)
	}
	assertLedgerRows(t, ctx, pool, len(serviceMigrationDirs)+1, 0)
	firstFingerprint := migrationLedgerFingerprint(t, ctx, pool)

	time.Sleep(20 * time.Millisecond)
	if err := applyMigrationsInRoots(ctx, db.url, []string{root}); err != nil {
		t.Fatalf("second ledger apply: %v", err)
	}
	if got := migrationLedgerFingerprint(t, ctx, pool); got != firstFingerprint {
		t.Fatalf("second apply changed ledger rows\nfirst: %s\nsecond: %s", firstFingerprint, got)
	}
}

func TestMigrationChecksumMismatchStopsBeforeLaterUnits(t *testing.T) {
	ctx := context.Background()
	db := isolatedMigrationDatabase(t)
	root := t.TempDir()
	writeAllServiceMigrationsWithSQL(t, root, func(string) string {
		return "-- noop\n"
	})

	if err := applyMigrationsInRoots(ctx, db.url, []string{root}); err != nil {
		t.Fatalf("initial apply: %v", err)
	}
	writeServiceMigrationWithSQL(t, root, "identity-service", "0001_init.sql", "-- changed\n")
	writeServiceMigrationWithSQL(t, root, "identity-service", "0002_later.sql", "CREATE TABLE checksum_later (id TEXT PRIMARY KEY);\n")

	err := applyMigrationsInRoots(ctx, db.url, []string{root})
	if err == nil || !strings.Contains(err.Error(), "migration checksum mismatch: service=identity-service version=1") {
		t.Fatalf("second apply error = %v, want checksum mismatch", err)
	}

	pool := openMigrationTestPool(t, db)
	defer pool.Close()
	var laterRows int
	if err := pool.QueryRow(ctx, `
SELECT count(*)::int
FROM platform_schema_migrations
WHERE service = 'identity-service' AND version = 2`).Scan(&laterRows); err != nil {
		t.Fatalf("count later migration ledger rows: %v", err)
	}
	if laterRows != 0 {
		t.Fatalf("later migration ledger rows = %d, want 0", laterRows)
	}
}

func TestMigrationAdvisoryLockContentionFailsCleanly(t *testing.T) {
	ctx := context.Background()
	db := isolatedMigrationDatabase(t)
	root := t.TempDir()
	writeAllServiceMigrationsWithSQL(t, root, func(string) string {
		return "-- noop\n"
	})
	pool := openMigrationTestPool(t, db)
	defer pool.Close()
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire lock holder: %v", err)
	}
	defer conn.Release()
	defer conn.Exec(context.Background(), `SELECT pg_advisory_unlock($1)`, migrationAdvisoryLockKey)

	var locked bool
	if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, migrationAdvisoryLockKey).Scan(&locked); err != nil {
		t.Fatalf("hold advisory lock: %v", err)
	}
	if !locked {
		t.Fatal("test could not acquire advisory lock")
	}

	err = applyMigrationsInRoots(ctx, db.url, []string{root})
	if err == nil || !strings.Contains(err.Error(), "migration advisory lock is already held") {
		t.Fatalf("apply error = %v, want advisory lock contention", err)
	}
	assertLedgerAbsent(t, ctx, pool)
}

func TestMigrationTemporarySchemaSurvivesNewPoolsAndRunner(t *testing.T) {
	ctx := context.Background()
	db := isolatedMigrationDatabase(t)
	pool := openMigrationTestPool(t, db)
	defer pool.Close()

	root := t.TempDir()
	writeAllServiceMigrationsWithSQL(t, root, func(string) string {
		return migrationCurrentSchemaAssertionSQL(db.schema)
	})

	if err := applyMigrationsInRoots(ctx, db.url, []string{root}); err != nil {
		t.Fatalf("apply schema assertion migration: %v", err)
	}
	assertLedgerRows(t, ctx, pool, len(serviceMigrationDirs)+1, 0)
}

type migrationTestDatabase struct {
	url    string
	schema string
}

func isolatedMigrationDatabase(t *testing.T) migrationTestDatabase {
	t.Helper()
	baseURL := requireTestDatabaseURL(t)
	ctx := context.Background()
	adminPool, err := pgxpool.New(ctx, baseURL)
	if err != nil {
		t.Fatalf("connect admin database: %v", err)
	}
	schema := fmt.Sprintf("nexus_migrate_%d", time.Now().UnixNano())
	requireValidMigrationTestSchema(t, schema)
	if _, err := adminPool.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		adminPool.Close()
		t.Fatalf("create test schema: %v", err)
	}
	t.Cleanup(func() {
		_, _ = adminPool.Exec(context.Background(), "DROP SCHEMA IF EXISTS "+schema+" CASCADE")
		adminPool.Close()
	})

	testURL := migrationTestDatabaseURL(t, baseURL, schema)
	return migrationTestDatabase{url: testURL, schema: schema}
}

func migrationTestDatabaseURL(t *testing.T, baseURL string, schema string) string {
	t.Helper()
	requireValidMigrationTestSchema(t, schema)
	if parsed, err := url.Parse(baseURL); err == nil && (parsed.Scheme == "postgres" || parsed.Scheme == "postgresql") {
		query := parsed.Query()
		query.Set("search_path", schema)
		parsed.RawQuery = query.Encode()
		testURL := parsed.String()
		assertMigrationTestURLSearchPath(t, testURL, schema)
		return testURL
	}

	testURL := strings.TrimSpace(baseURL) + " search_path=" + schema
	assertMigrationTestURLSearchPath(t, testURL, schema)
	return testURL
}

func requireValidMigrationTestSchema(t *testing.T, schema string) {
	t.Helper()
	if !strings.HasPrefix(schema, "nexus_migrate_") || len(schema) == len("nexus_migrate_") {
		t.Fatalf("invalid migration test schema: %s", schema)
	}
	for _, r := range schema {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			continue
		}
		t.Fatalf("invalid migration test schema: %s", schema)
	}
}

func assertMigrationTestURLSearchPath(t *testing.T, testURL string, schema string) {
	t.Helper()
	cfg, err := pgxpool.ParseConfig(testURL)
	if err != nil {
		t.Fatal("parse isolated database url with test search_path")
	}
	if got := cfg.ConnConfig.RuntimeParams["search_path"]; got != schema {
		t.Fatalf("isolated database url search_path = %q, want %q", got, schema)
	}
}

func openMigrationTestPool(t *testing.T, db migrationTestDatabase) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, db.url)
	if err != nil {
		t.Fatalf("connect isolated database: %v", err)
	}
	assertCurrentSchema(t, ctx, pool, db.schema)
	return pool
}

func assertCurrentSchema(t *testing.T, ctx context.Context, pool *pgxpool.Pool, want string) {
	t.Helper()
	var got string
	if err := pool.QueryRow(ctx, `SELECT current_schema()`).Scan(&got); err != nil {
		t.Fatalf("query current schema: %v", err)
	}
	if got != want {
		t.Fatalf("current_schema() = %q, want %q", got, want)
	}
}

func migrationCurrentSchemaAssertionSQL(schema string) string {
	return fmt.Sprintf(`
DO $$
BEGIN
    IF current_schema() <> '%s' THEN
        RAISE EXCEPTION 'unexpected migration schema %%', current_schema();
    END IF;
END
$$;
`, schema)
}

func writeAllServiceMigrationsWithSQL(t *testing.T, root string, sqlFor func(service string) string) {
	t.Helper()
	for _, service := range serviceMigrationDirs {
		writeServiceMigrationWithSQL(t, root, service, "0001_init.sql", sqlFor(service))
	}
}

func writeServiceMigrationWithSQL(t *testing.T, root, service, name, sql string) {
	t.Helper()
	writeTestFile(t, filepath.Join(root, service, "migrations", name), sql)
}

func migrationTestTableName(service string) string {
	return strings.ReplaceAll(service, "-", "_") + "_migration_test"
}

func assertLedgerAbsent(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	var table sql.NullString
	if err := pool.QueryRow(ctx, `SELECT to_regclass('platform_schema_migrations')::text`).Scan(&table); err != nil {
		t.Fatalf("check ledger table: %v", err)
	}
	if table.Valid && table.String != "" {
		t.Fatalf("ledger table exists before runner: %s", table.String)
	}
}

func assertLedgerRows(t *testing.T, ctx context.Context, pool *pgxpool.Pool, wantTotal, wantDirty int) {
	t.Helper()
	var total int
	var dirty int
	if err := pool.QueryRow(ctx, `
SELECT count(*)::int, count(*) FILTER (WHERE dirty)::int
FROM platform_schema_migrations`).Scan(&total, &dirty); err != nil {
		t.Fatalf("count ledger rows: %v", err)
	}
	if total != wantTotal || dirty != wantDirty {
		t.Fatalf("ledger rows total=%d dirty=%d, want total=%d dirty=%d", total, dirty, wantTotal, wantDirty)
	}
}

func migrationLedgerFingerprint(t *testing.T, ctx context.Context, pool *pgxpool.Pool) string {
	t.Helper()
	var fingerprint string
	if err := pool.QueryRow(ctx, `
SELECT string_agg(
    service || ':' || version::text || ':' || filename || ':' || checksum || ':' ||
    dirty::text || ':' || applied_at::text || ':' || duration_ms::text,
    ',' ORDER BY service, version
)
FROM platform_schema_migrations`).Scan(&fingerprint); err != nil {
		t.Fatalf("fingerprint ledger: %v", err)
	}
	return fingerprint
}
