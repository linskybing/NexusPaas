//go:build integration

package platform

import (
	"context"
	"fmt"
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
