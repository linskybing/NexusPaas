package platform

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestPostgresStoreCRUDWithQueryLayer(t *testing.T) {
	now := time.Date(2026, 6, 13, 18, 0, 0, 0, time.UTC)
	later := now.Add(time.Minute)
	db := &fakePostgresDB{
		queryRows: []*fakePostgresRow{
			{values: []any{"r1", []byte(`{"id":"r1","name":"original","count":3}`), 1, now, now}},
			{values: []any{"r1", []byte(`{"id":"r1","name":"original","count":3}`), 1, now, now}},
			{values: []any{"r1", []byte(`{"id":"r1","name":"changed","count":3}`), 2, now, later}},
		},
		queryResults: []*fakePostgresRows{
			{rows: [][]any{{"r1", []byte(`{"id":"r1","name":"original","count":3}`), 1, now, now}}},
		},
		execTags: []pgconn.CommandTag{pgconn.NewCommandTag("DELETE 1")},
	}
	store := &PostgresStore{db: db}
	ctx := context.Background()

	created, err := store.Create(ctx, "svc:records", map[string]any{"id": "r1", "name": "original", "count": 3})
	if err != nil {
		t.Fatal(err)
	}
	if created.ID != "r1" || created.Version != 1 || created.Data["name"] != "original" {
		t.Fatalf("created record = %#v", created)
	}

	got, ok := store.Get(ctx, "svc:records", "r1")
	if !ok || got.Data["count"].(float64) != 3 {
		t.Fatalf("got record = %#v ok=%v", got, ok)
	}

	listed := store.List(ctx, "svc:records")
	if len(listed) != 1 || listed[0].ID != "r1" {
		t.Fatalf("listed records = %#v", listed)
	}

	updated, ok := store.Update(ctx, "svc:records", "r1", map[string]any{"name": "changed"})
	if !ok || updated.Version != 2 || updated.Data["name"] != "changed" {
		t.Fatalf("updated record = %#v ok=%v", updated, ok)
	}
	if !store.Delete(ctx, "svc:records", "r1") {
		t.Fatal("delete returned false")
	}
}

func TestPostgresStoreCreateConflict(t *testing.T) {
	store := &PostgresStore{db: &fakePostgresDB{
		queryRows: []*fakePostgresRow{{err: pgx.ErrNoRows}},
	}}

	_, err := store.Create(context.Background(), "svc:records", map[string]any{"id": "dup"})
	if !IsCreateConflict(err) {
		t.Fatalf("Create error = %v, want create conflict", err)
	}
}

func TestPostgresStoreListChecksRowsError(t *testing.T) {
	store := &PostgresStore{db: &fakePostgresDB{
		queryResults: []*fakePostgresRows{{
			rows: [][]any{{"r1", []byte(`{"id":"r1"}`), 1, time.Now().UTC(), time.Now().UTC()}},
			err:  errors.New("network read failed"),
		}},
	}}

	records := store.List(context.Background(), "svc:records")
	if len(records) != 1 {
		t.Fatalf("records = %#v, want partial row before rows.Err", records)
	}
}

func TestPostgresStoreNextIDUsesRecordsAndHighWaterMark(t *testing.T) {
	tx := &fakePostgresTx{
		fakePostgresDB: fakePostgresDB{
			queryResults: []*fakePostgresRows{{
				rows: [][]any{{"US2600002"}, {"US2600005"}, {"USnot-a-number"}},
			}},
			queryRows: []*fakePostgresRow{
				{values: []any{int64(2600007)}},
				{values: []any{false}},
			},
			execTags: []pgconn.CommandTag{
				pgconn.NewCommandTag("SELECT 1"),
				pgconn.NewCommandTag("INSERT 0 1"),
			},
		},
	}
	store := &PostgresStore{db: &fakePostgresDB{tx: tx}}

	got := store.NextID("svc:users", "US", 2600001, 7)
	if got != "US2600008" {
		t.Fatalf("NextID = %q, want US2600008", got)
	}
	if !tx.committed {
		t.Fatal("transaction was not committed")
	}
	if !tx.rolledBack {
		t.Fatal("deferred rollback should still run after commit")
	}
}

func TestNewBackingResourcesInjectsPostgresStoreWhenDatabaseURLIsConfigured(t *testing.T) {
	backing, err := NewBackingResources(context.Background(), Config{
		DatabaseURL: "postgres://user:pass@localhost:1/db?sslmode=disable",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer backing.Close()

	app := NewApp(Config{ServiceName: "all"}, backing.Options...)
	if _, ok := app.Store.(*PostgresStore); !ok {
		t.Fatalf("store = %T, want *PostgresStore", app.Store)
	}
}

func TestNewBackingResourcesKeepsDefaultsWithoutDatabaseURL(t *testing.T) {
	backing, err := NewBackingResources(context.Background(), Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer backing.Close()
	if len(backing.Options) != 0 {
		t.Fatalf("options = %d, want none", len(backing.Options))
	}
}

type fakePostgresDB struct {
	execTags     []pgconn.CommandTag
	execErrs     []error
	queryRows    []*fakePostgresRow
	queryResults []*fakePostgresRows
	tx           *fakePostgresTx
}

func (f *fakePostgresDB) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	if len(f.execErrs) > 0 {
		err := f.execErrs[0]
		f.execErrs = f.execErrs[1:]
		if err != nil {
			return pgconn.CommandTag{}, err
		}
	}
	if len(f.execTags) == 0 {
		return pgconn.NewCommandTag(""), nil
	}
	tag := f.execTags[0]
	f.execTags = f.execTags[1:]
	return tag, nil
}

func (f *fakePostgresDB) Query(context.Context, string, ...any) (postgresRows, error) {
	if len(f.queryResults) == 0 {
		return nil, errors.New("unexpected Query")
	}
	rows := f.queryResults[0]
	f.queryResults = f.queryResults[1:]
	return rows, nil
}

func (f *fakePostgresDB) QueryRow(context.Context, string, ...any) postgresRow {
	if len(f.queryRows) == 0 {
		return &fakePostgresRow{err: errors.New("unexpected QueryRow")}
	}
	row := f.queryRows[0]
	f.queryRows = f.queryRows[1:]
	return row
}

func (f *fakePostgresDB) Begin(context.Context) (postgresStoreTx, error) {
	if f.tx == nil {
		return nil, errors.New("unexpected Begin")
	}
	return f.tx, nil
}

type fakePostgresTx struct {
	fakePostgresDB
	committed  bool
	rolledBack bool
}

func (f *fakePostgresTx) Commit(context.Context) error {
	f.committed = true
	return nil
}

func (f *fakePostgresTx) Rollback(context.Context) error {
	f.rolledBack = true
	return nil
}

type fakePostgresRow struct {
	values []any
	err    error
}

func (r *fakePostgresRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	return scanFakePostgresValues(dest, r.values)
}

type fakePostgresRows struct {
	rows   [][]any
	index  int
	err    error
	closed bool
}

func (r *fakePostgresRows) Close() {
	r.closed = true
}

func (r *fakePostgresRows) Err() error {
	return r.err
}

func (r *fakePostgresRows) Next() bool {
	if r.index >= len(r.rows) {
		r.closed = true
		return false
	}
	r.index++
	return true
}

func (r *fakePostgresRows) Scan(dest ...any) error {
	if r.index == 0 || r.index > len(r.rows) {
		return errors.New("Scan called without current row")
	}
	return scanFakePostgresValues(dest, r.rows[r.index-1])
}

func scanFakePostgresValues(dest []any, values []any) error {
	if len(dest) != len(values) {
		return fmt.Errorf("scan dest count=%d values=%d", len(dest), len(values))
	}
	for i := range dest {
		if err := assignFakePostgresValue(dest[i], values[i]); err != nil {
			return fmt.Errorf("scan column %d: %w", i, err)
		}
	}
	return nil
}

func assignFakePostgresValue(dest, value any) error {
	switch ptr := dest.(type) {
	case *string:
		*ptr = value.(string)
	case *[]byte:
		*ptr = append((*ptr)[:0], value.([]byte)...)
	case *int:
		*ptr = value.(int)
	case *int64:
		*ptr = value.(int64)
	case *time.Time:
		*ptr = value.(time.Time)
	case *bool:
		*ptr = value.(bool)
	default:
		return fmt.Errorf("unsupported dest %s", reflect.TypeOf(dest))
	}
	return nil
}
