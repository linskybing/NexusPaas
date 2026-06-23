//go:build integration

package platform

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// newTestPostgresStore connects, applies migrations, and clears any rows left by
// a previous run for the given resource so tests are isolated against a
// persistent database.
func newTestPostgresStore(t *testing.T, resource string) *PostgresStore {
	t.Helper()
	url := requireTestDatabaseURL(t)
	ctx := context.Background()
	if err := ApplyMigrations(ctx, url); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(ctx, `DELETE FROM platform_records WHERE resource = $1`, resource); err != nil {
		t.Fatalf("reset records: %v", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM platform_id_seq WHERE key LIKE $1`, resource+"|%"); err != nil {
		t.Fatalf("reset seq: %v", err)
	}
	return NewPostgresStore(pool)
}

// uniqueResource isolates each test run so repeated runs against a persistent
// database do not collide.
func uniqueResource(t *testing.T) string {
	t.Helper()
	return "test-service:" + t.Name()
}

func TestPostgresStoreCRUDRoundTrip(t *testing.T) {
	resource := uniqueResource(t)
	s := newTestPostgresStore(t, resource)
	ctx := context.Background()

	created, err := s.Create(ctx, resource, map[string]any{"id": "r1", "name": "original", "count": 3})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.ID != "r1" || created.Version != 1 {
		t.Fatalf("create returned %#v", created)
	}

	got, ok := s.Get(ctx, resource, "r1")
	if !ok || got.Data["name"] != "original" {
		t.Fatalf("get = %#v ok=%v", got.Data, ok)
	}

	updated, ok := s.Update(ctx, resource, "r1", map[string]any{"name": "changed"})
	if !ok || updated.Data["name"] != "changed" || updated.Version != 2 {
		t.Fatalf("update = %#v ok=%v", updated, ok)
	}
	// Merge semantics: untouched field survives.
	if updated.Data["count"].(float64) != 3 {
		t.Fatalf("update dropped count: %#v", updated.Data)
	}

	list := s.List(ctx, resource)
	if len(list) != 1 {
		t.Fatalf("list len = %d, want 1", len(list))
	}

	if !s.Delete(ctx, resource, "r1") {
		t.Fatal("delete returned false")
	}
	if _, ok := s.Get(ctx, resource, "r1"); ok {
		t.Fatal("record still present after delete")
	}
}

func TestPostgresStorePersistsAcrossConnections(t *testing.T) {
	url := requireTestDatabaseURL(t)
	ctx := context.Background()
	if err := ApplyMigrations(ctx, url); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	resource := uniqueResource(t)

	// First "process": write a record, then drop the pool.
	pool1, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatalf("connect 1: %v", err)
	}
	if _, err := pool1.Exec(ctx, `DELETE FROM platform_records WHERE resource = $1`, resource); err != nil {
		t.Fatalf("reset: %v", err)
	}
	if _, err := NewPostgresStore(pool1).Create(ctx, resource, map[string]any{"id": "persist", "name": "kept"}); err != nil {
		t.Fatalf("create: %v", err)
	}
	pool1.Close()

	// Second "process": a fresh pool still sees the record (durability).
	pool2, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatalf("connect 2: %v", err)
	}
	defer pool2.Close()
	got, ok := NewPostgresStore(pool2).Get(ctx, resource, "persist")
	if !ok || got.Data["name"] != "kept" {
		t.Fatalf("record did not survive reconnect: %#v ok=%v", got.Data, ok)
	}
}

func TestPostgresStoreCreateConflictIntegration(t *testing.T) {
	resource := uniqueResource(t)
	s := newTestPostgresStore(t, resource)
	ctx := context.Background()

	if _, err := s.Create(ctx, resource, map[string]any{"id": "dup"}); err != nil {
		t.Fatalf("first create: %v", err)
	}
	_, err := s.Create(ctx, resource, map[string]any{"id": "dup"})
	if !IsCreateConflict(err) {
		t.Fatalf("second create err = %v, want conflict", err)
	}
}

func TestPostgresStoreNextIDMonotonicNoReuse(t *testing.T) {
	resource := uniqueResource(t)
	s := newTestPostgresStore(t, resource)
	ctx := context.Background()

	var last string
	for i := 0; i < 3; i++ {
		id := s.NextID(resource, "US", 2600001, 7)
		if _, err := s.Create(ctx, resource, map[string]any{"id": id}); err != nil {
			t.Fatalf("create %s: %v", id, err)
		}
		last = id
	}
	// Delete the highest id; the next allocation must not reuse it.
	if !s.Delete(ctx, resource, last) {
		t.Fatalf("delete %s failed", last)
	}
	next := s.NextID(resource, "US", 2600001, 7)
	if next == last {
		t.Fatalf("NextID reused deleted id %s", last)
	}
	if next != fmt.Sprintf("US%07d", 2600004) {
		t.Fatalf("NextID = %s, want US2600004", next)
	}
}

func TestPostgresStoreIdentityResourcesUseOwnedTables(t *testing.T) {
	ctx := context.Background()
	pool, store := newIdentityBoundaryStore(t, ctx)

	createIdentityBoundaryRecords(t, ctx, store)
	assertIdentityBoundaryUserUpdate(t, ctx, store)
	assertIdentityAPITokenSecretStoredHashed(t, ctx, store)
	assertIdentityBoundaryRowsBypassPlatformRecords(t, ctx, pool)
	assertIdentityOwnedTableRead(t, ctx, store)
}

func TestIdentityMigrationBackfillsFromPlatformRecords(t *testing.T) {
	ctx := context.Background()
	db := isolatedMigrationDatabase(t)
	root := t.TempDir()
	writeAllServiceMigrationsWithSQL(t, root, func(service string) string {
		if service == "identity-service" {
			return readTextFile(t, "../../identity-service/migrations/0001_init.sql")
		}
		return "-- noop\n"
	})
	if err := applyMigrationsInRoots(ctx, db.url, []string{root}); err != nil {
		t.Fatalf("apply initial migrations: %v", err)
	}
	pool := openMigrationTestPool(t, db)
	defer pool.Close()
	if _, err := pool.Exec(ctx, `
		INSERT INTO platform_records (resource, id, payload)
		VALUES ($1, $2, $3::jsonb)`,
		identityUsersResource,
		"US_BACKFILL_BOUNDARY",
		`{"id":"US_BACKFILL_BOUNDARY","username":"backfill-boundary","password_hash":"hash","status":"online","custom":"legacy"}`,
	); err != nil {
		t.Fatalf("seed platform record: %v", err)
	}
	writeServiceMigrationWithSQL(t, root, "identity-service", "0002_identity_owned_records.sql", readTextFile(t, "../../identity-service/migrations/0002_identity_owned_records.sql"))
	if err := applyMigrationsInRoots(ctx, db.url, []string{root}); err != nil {
		t.Fatalf("apply backfill migration: %v", err)
	}
	var payload string
	if err := pool.QueryRow(ctx, `SELECT payload::text FROM users WHERE id = $1`, "US_BACKFILL_BOUNDARY").Scan(&payload); err != nil {
		t.Fatalf("query backfilled user: %v", err)
	}
	if !strings.Contains(payload, "legacy") {
		t.Fatalf("backfilled payload = %s, want legacy field", payload)
	}
	var legacyCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM platform_records WHERE resource = $1 AND id = $2`, identityUsersResource, "US_BACKFILL_BOUNDARY").Scan(&legacyCount); err != nil {
		t.Fatalf("count legacy platform record: %v", err)
	}
	if legacyCount != 1 {
		t.Fatalf("legacy platform_records count = %d, want retained row", legacyCount)
	}
}

func resetIdentityBoundaryRows(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	statements := []string{
		`DELETE FROM user_api_tokens WHERE id = 'AT_BOUNDARY' OR user_id = 'US_BOUNDARY'`,
		`DELETE FROM refresh_tokens WHERE id = 'refresh-boundary' OR user_id = 'US_BOUNDARY'`,
		`DELETE FROM sessions WHERE id = 'session-boundary' OR user_id = 'US_BOUNDARY'`,
		`DELETE FROM captchas WHERE id = 'captcha-boundary'`,
		`DELETE FROM login_failures WHERE id = 'failure-boundary' OR username = 'boundary-user'`,
		`DELETE FROM identity_roles WHERE id = 'RO_BOUNDARY' OR name = 'boundary-role'`,
		`DELETE FROM users WHERE id = 'US_BOUNDARY' OR username = 'boundary-user'`,
		`DELETE FROM platform_records WHERE (resource, id) IN (
			('identity-service:users', 'US_BOUNDARY'),
			('identity-service:roles', 'RO_BOUNDARY'),
			('identity-service:sessions', 'session-boundary'),
			('identity-service:refresh_tokens', 'refresh-boundary'),
			('identity-service:api_tokens', 'AT_BOUNDARY'),
			('identity-service:captchas', 'captcha-boundary'),
			('identity-service:login_failures', 'failure-boundary')
		)`,
		`DELETE FROM platform_id_seq WHERE key IN (
			'identity-service:users|US',
			'identity-service:roles|RO',
			'identity-service:sessions|session-',
			'identity-service:refresh_tokens|refresh-',
			'identity-service:api_tokens|AT',
			'identity-service:captchas|captcha-',
			'identity-service:login_failures|failure-'
		)`,
	}
	for _, statement := range statements {
		if _, err := pool.Exec(ctx, statement); err != nil {
			t.Fatalf("reset identity boundary rows: %v", err)
		}
	}
}

func newIdentityBoundaryStore(t *testing.T, ctx context.Context) (*pgxpool.Pool, *PostgresStore) {
	t.Helper()
	url := requireTestDatabaseURL(t)
	if err := ApplyMigrations(ctx, url); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	resetIdentityBoundaryRows(t, ctx, pool)
	return pool, NewPostgresStore(pool)
}

func createIdentityBoundaryRecords(t *testing.T, ctx context.Context, store *PostgresStore) {
	t.Helper()
	records := []struct {
		name     string
		resource string
		data     map[string]any
	}{
		{"user", identityUsersResource, map[string]any{
			"id":            "US_BOUNDARY",
			"username":      "boundary-user",
			"password_hash": "hash",
			"status":        "online",
			"capabilities":  map[string]any{"adminPanel": true},
		}},
		{"role", identityRolesResource, map[string]any{
			"id":         "RO_BOUNDARY",
			"name":       "boundary-role",
			"adminPanel": true,
		}},
		{"session", identitySessionsResource, map[string]any{
			"id":         "session-boundary",
			"user_id":    "US_BOUNDARY",
			"expires_at": "2026-06-16T12:00:00Z",
		}},
		{"refresh token", identityRefreshTokens, map[string]any{
			"id":         "refresh-boundary",
			"user_id":    "US_BOUNDARY",
			"expires_at": "2026-06-16T13:00:00Z",
		}},
		{"api token", identityAPITokensResource, map[string]any{
			"id":           "AT_BOUNDARY",
			"user_id":      "US_BOUNDARY",
			"name":         "boundary",
			"token":        "raw-token-must-not-persist",
			"token_hash":   HashSecret("raw-token-must-not-persist"),
			"token_prefix": "raw",
		}},
		{"captcha", identityCaptchasResource, map[string]any{
			"id":          "captcha-boundary",
			"answer_hash": "hash",
			"expires_at":  "2026-06-16T12:00:00Z",
		}},
		{"login failure", identityLoginFailures, map[string]any{
			"id":       "failure-boundary",
			"username": "boundary-user",
			"ip":       "127.0.0.1",
			"failures": 1,
		}},
	}
	for _, record := range records {
		if _, err := store.Create(ctx, record.resource, record.data); err != nil {
			t.Fatalf("create %s: %v", record.name, err)
		}
	}
}

func assertIdentityBoundaryUserUpdate(t *testing.T, ctx context.Context, store *PostgresStore) {
	t.Helper()
	updated, ok := store.Update(ctx, identityUsersResource, "US_BOUNDARY", map[string]any{"custom": "kept"})
	if !ok || updated.Data["custom"] != "kept" {
		t.Fatalf("update user = %#v ok=%v, want custom payload retained", updated, ok)
	}
}

func assertIdentityAPITokenSecretStoredHashed(t *testing.T, ctx context.Context, store *PostgresStore) {
	t.Helper()
	token, ok := store.Get(ctx, identityAPITokensResource, "AT_BOUNDARY")
	if !ok {
		t.Fatal("api token not found")
	}
	if token.Data["token"] != nil || token.Data["token_hash"] == "" {
		t.Fatalf("api token payload = %#v, want hash only", token.Data)
	}
}

func assertIdentityBoundaryRowsBypassPlatformRecords(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	for _, resource := range identityBoundaryResources() {
		id := identityBoundaryIDForResource(resource)
		var count int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM platform_records WHERE resource = $1 AND id = $2`, resource, id).Scan(&count); err != nil {
			t.Fatalf("count platform_records for %s: %v", resource, err)
		}
		if count != 0 {
			t.Fatalf("platform_records rows for %s/%s = %d, want 0", resource, id, count)
		}
	}
}

func assertIdentityOwnedTableRead(t *testing.T, ctx context.Context, store *PostgresStore) {
	t.Helper()
	got, ok := store.Get(ctx, identityUsersResource, "US_BOUNDARY")
	if !ok || got.Data["username"] != "boundary-user" {
		t.Fatalf("get user = %#v ok=%v, want owned-table user", got, ok)
	}
}

func identityBoundaryResources() []string {
	return []string{
		identityUsersResource,
		identityRolesResource,
		identitySessionsResource,
		identityRefreshTokens,
		identityAPITokensResource,
		identityCaptchasResource,
		identityLoginFailures,
	}
}

func identityBoundaryIDForResource(resource string) string {
	switch resource {
	case identityUsersResource:
		return "US_BOUNDARY"
	case identityRolesResource:
		return "RO_BOUNDARY"
	case identitySessionsResource:
		return "session-boundary"
	case identityRefreshTokens:
		return "refresh-boundary"
	case identityAPITokensResource:
		return "AT_BOUNDARY"
	case identityCaptchasResource:
		return "captcha-boundary"
	case identityLoginFailures:
		return "failure-boundary"
	default:
		return ""
	}
}
