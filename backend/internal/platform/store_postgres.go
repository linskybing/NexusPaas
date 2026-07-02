package platform

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
)

// PostgresStore implements RecordStore against the unified platform_records
// table (see schema.sql). It gives every service durable, restart-surviving,
// multi-replica-shared storage behind the same port the in-memory Store
// satisfies (findings 4, 7, 13). Payloads are stored as JSONB; numbers and
// timestamps therefore round-trip through JSON, which the handler helpers
// (asString/intValue/numberValue) already tolerate.
type PostgresStore struct {
	db postgresStoreDB
}

// NewPostgresStore returns a RecordStore backed by the given pool.
func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{db: pgxPoolStoreDB{pool: pool}}
}

func (s *PostgresStore) Create(ctx context.Context, resource string, data map[string]any) (contracts.Record[map[string]any], error) {
	if spec, ok := typedPostgresResourceFor(resource); ok {
		return s.createIdentityRecord(ctx, spec, data)
	}
	return createPostgresRecord(ctx, s.db, resource, data)
}

func (s *PostgresStore) CreateWithEvent(ctx context.Context, resource string, data map[string]any, buildEvent recordEventBuilder) (contracts.Record[map[string]any], error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return contracts.Record[map[string]any]{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // rollback after commit is a no-op

	var record contracts.Record[map[string]any]
	if spec, ok := typedPostgresResourceFor(resource); ok {
		record, err = createIdentityRecordIn(ctx, tx, spec, data)
	} else {
		record, err = createPostgresRecord(ctx, tx, resource, data)
	}
	if err != nil {
		return contracts.Record[map[string]any]{}, err
	}
	if buildEvent != nil {
		if err := insertOutboxEvent(ctx, tx, buildEvent(record)); err != nil {
			return contracts.Record[map[string]any]{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return contracts.Record[map[string]any]{}, err
	}
	return record, nil
}

func createPostgresRecord(ctx context.Context, db postgresStoreExecutor, resource string, data map[string]any) (contracts.Record[map[string]any], error) {
	id, _ := data["id"].(string)
	if id == "" {
		id = newID()
		data["id"] = id
	}
	payload, err := json.Marshal(cloneMap(data))
	if err != nil {
		return contracts.Record[map[string]any]{}, fmt.Errorf("marshal payload: %w", err)
	}
	var rec contracts.Record[map[string]any]
	var raw []byte
	row := db.QueryRow(ctx, `
		INSERT INTO platform_records (resource, id, payload)
		VALUES ($1, $2, $3)
		ON CONFLICT (resource, id) DO NOTHING
		RETURNING id, payload, version, created_at, updated_at`,
		resource, id, payload)
	if err := row.Scan(&rec.ID, &raw, &rec.Version, &rec.CreatedAt, &rec.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return contracts.Record[map[string]any]{}, CreateConflictError{Resource: resource, ID: id}
		}
		return contracts.Record[map[string]any]{}, fmt.Errorf("insert record: %w", err)
	}
	if err := json.Unmarshal(raw, &rec.Data); err != nil {
		return contracts.Record[map[string]any]{}, fmt.Errorf("unmarshal payload: %w", err)
	}
	return rec, nil
}

func (s *PostgresStore) Get(ctx context.Context, resource, id string) (contracts.Record[map[string]any], bool) {
	if spec, ok := typedPostgresResourceFor(resource); ok {
		return s.getIdentityRecord(ctx, spec, id)
	}
	var rec contracts.Record[map[string]any]
	var raw []byte
	row := s.db.QueryRow(ctx, `
		SELECT id, payload, version, created_at, updated_at
		FROM platform_records WHERE resource = $1 AND id = $2`, resource, id)
	if err := row.Scan(&rec.ID, &raw, &rec.Version, &rec.CreatedAt, &rec.UpdatedAt); err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			slog.Error("postgres get failed", "resource", resource, "id", id, "error", err)
		}
		return contracts.Record[map[string]any]{}, false
	}
	if err := json.Unmarshal(raw, &rec.Data); err != nil {
		slog.Error("postgres get unmarshal failed", "resource", resource, "id", id, "error", err)
		return contracts.Record[map[string]any]{}, false
	}
	return rec, true
}

func (s *PostgresStore) List(ctx context.Context, resource string) []contracts.Record[map[string]any] {
	if spec, ok := typedPostgresResourceFor(resource); ok {
		return s.listIdentityRecords(ctx, spec)
	}
	rows, err := s.db.Query(ctx, `
		SELECT id, payload, version, created_at, updated_at
		FROM platform_records WHERE resource = $1 ORDER BY created_at, id`, resource)
	if err != nil {
		slog.Error("postgres list failed", "resource", resource, "error", err)
		return nil
	}
	defer rows.Close()
	records := []contracts.Record[map[string]any]{}
	for rows.Next() {
		var rec contracts.Record[map[string]any]
		var raw []byte
		if err := rows.Scan(&rec.ID, &raw, &rec.Version, &rec.CreatedAt, &rec.UpdatedAt); err != nil {
			slog.Error("postgres list scan failed", "resource", resource, "error", err)
			return records
		}
		if err := json.Unmarshal(raw, &rec.Data); err != nil {
			slog.Error("postgres list unmarshal failed", "resource", resource, "error", err)
			continue
		}
		records = append(records, rec)
	}
	if err := rows.Err(); err != nil {
		slog.Error("postgres list rows failed", "resource", resource, "error", err)
		return records
	}
	return records
}

func (s *PostgresStore) Update(ctx context.Context, resource, id string, data map[string]any) (contracts.Record[map[string]any], bool) {
	if spec, ok := typedPostgresResourceFor(resource); ok {
		return s.updateIdentityRecord(ctx, spec, id, data)
	}
	record, ok, err := updatePostgresRecord(ctx, s.db, resource, id, data)
	if err != nil {
		slog.Error("postgres update failed", "resource", resource, "id", id, "error", err)
		return contracts.Record[map[string]any]{}, false
	}
	return record, ok
}

func (s *PostgresStore) UpdateWithEvent(ctx context.Context, resource, id string, data map[string]any, buildEvent recordEventBuilder) (contracts.Record[map[string]any], bool, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return contracts.Record[map[string]any]{}, false, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // rollback after commit is a no-op

	var record contracts.Record[map[string]any]
	var ok bool
	if spec, identity := typedPostgresResourceFor(resource); identity {
		record, ok, err = updateIdentityRecordIn(ctx, tx, spec, id, data)
	} else {
		record, ok, err = updatePostgresRecord(ctx, tx, resource, id, data)
	}
	if err != nil || !ok {
		return record, ok, err
	}
	if buildEvent != nil {
		if err := insertOutboxEvent(ctx, tx, buildEvent(record)); err != nil {
			return contracts.Record[map[string]any]{}, false, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return contracts.Record[map[string]any]{}, false, err
	}
	return record, true, nil
}

func (s *PostgresStore) UpsertWithEvent(ctx context.Context, resource, id string, data map[string]any, buildEvent recordEventBuilder) (contracts.Record[map[string]any], error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return contracts.Record[map[string]any]{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // rollback after commit is a no-op

	record, err := upsertRecordIn(ctx, tx, resource, id, data)
	if err != nil {
		return contracts.Record[map[string]any]{}, err
	}
	if buildEvent != nil {
		if err := insertOutboxEvent(ctx, tx, buildEvent(record)); err != nil {
			return contracts.Record[map[string]any]{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return contracts.Record[map[string]any]{}, err
	}
	return record, nil
}

func updatePostgresRecord(ctx context.Context, db postgresStoreExecutor, resource, id string, data map[string]any) (contracts.Record[map[string]any], bool, error) {
	patch, err := json.Marshal(cloneMap(data))
	if err != nil {
		return contracts.Record[map[string]any]{}, false, fmt.Errorf("marshal patch: %w", err)
	}
	var rec contracts.Record[map[string]any]
	var raw []byte
	// Merge the patch into the stored JSONB, bump version, refresh updated_at.
	row := db.QueryRow(ctx, `
		UPDATE platform_records
		SET payload = payload || $3::jsonb, version = version + 1, updated_at = now()
		WHERE resource = $1 AND id = $2
		RETURNING id, payload, version, created_at, updated_at`,
		resource, id, patch)
	if err := row.Scan(&rec.ID, &raw, &rec.Version, &rec.CreatedAt, &rec.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return contracts.Record[map[string]any]{}, false, nil
		}
		return contracts.Record[map[string]any]{}, false, fmt.Errorf("update record: %w", err)
	}
	if err := json.Unmarshal(raw, &rec.Data); err != nil {
		return contracts.Record[map[string]any]{}, false, fmt.Errorf("unmarshal payload: %w", err)
	}
	return rec, true, nil
}

func upsertRecordIn(ctx context.Context, db postgresStoreExecutor, resource, id string, data map[string]any) (contracts.Record[map[string]any], error) {
	payload := cloneMap(data)
	if id == "" {
		id, _ = payload["id"].(string)
	}
	if id == "" {
		id = newID()
	}
	payload["id"] = id
	if spec, ok := typedPostgresResourceFor(resource); ok {
		return upsertIdentityRecordIn(ctx, db, spec, id, payload)
	}
	return upsertPlatformRecordIn(ctx, db, resource, id, payload)
}

func upsertIdentityRecordIn(
	ctx context.Context,
	db postgresStoreExecutor,
	spec identityPostgresResource,
	id string,
	payload map[string]any,
) (contracts.Record[map[string]any], error) {
	record, updated, err := updateIdentityRecordIn(ctx, db, spec, id, payload)
	if err != nil || updated {
		return record, err
	}
	record, err = createIdentityRecordIn(ctx, db, spec, payload)
	if err == nil {
		return record, nil
	}
	if !IsCreateConflict(err) {
		return contracts.Record[map[string]any]{}, err
	}
	if updatedRecord, updated, updateErr := updateIdentityRecordIn(ctx, db, spec, id, payload); updateErr != nil || updated {
		return updatedRecord, updateErr
	}
	return contracts.Record[map[string]any]{}, err
}

func upsertPlatformRecordIn(
	ctx context.Context,
	db postgresStoreExecutor,
	resource string,
	id string,
	payload map[string]any,
) (contracts.Record[map[string]any], error) {
	record, updated, err := updatePostgresRecord(ctx, db, resource, id, payload)
	if err != nil || updated {
		return record, err
	}
	record, err = createPostgresRecord(ctx, db, resource, payload)
	if err == nil {
		return record, nil
	}
	if !IsCreateConflict(err) {
		return contracts.Record[map[string]any]{}, err
	}
	if updatedRecord, updated, updateErr := updatePostgresRecord(ctx, db, resource, id, payload); updateErr != nil || updated {
		return updatedRecord, updateErr
	}
	return contracts.Record[map[string]any]{}, err
}

func (s *PostgresStore) Delete(ctx context.Context, resource, id string) bool {
	if spec, ok := typedPostgresResourceFor(resource); ok {
		return s.deleteIdentityRecord(ctx, spec, id)
	}
	deleted, err := deletePostgresRecord(ctx, s.db, resource, id)
	if err != nil {
		slog.Error("postgres delete failed", "resource", resource, "id", id, "error", err)
		return false
	}
	return deleted
}

func (s *PostgresStore) DeleteWithEvent(ctx context.Context, resource, id string, buildEvent deleteEventBuilder) (bool, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // rollback after commit is a no-op

	var deleted bool
	if spec, ok := typedPostgresResourceFor(resource); ok {
		deleted, err = deleteIdentityRecordIn(ctx, tx, spec, id)
	} else {
		deleted, err = deletePostgresRecord(ctx, tx, resource, id)
	}
	if err != nil || !deleted {
		return deleted, err
	}
	if buildEvent != nil {
		if err := insertOutboxEvent(ctx, tx, buildEvent(deleted)); err != nil {
			return false, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return true, nil
}

func deletePostgresRecord(ctx context.Context, db postgresStoreExecutor, resource, id string) (bool, error) {
	tag, err := db.Exec(ctx, `DELETE FROM platform_records WHERE resource = $1 AND id = $2`, resource, id)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// RunInTx runs fn inside one database transaction: every write fn does through the
// StoreTx and every event it Emits commit together, or all roll back. This is the
// multi-record counterpart to CreateWithEvent and lets a cascade (parent + child
// deletes, or a record + its sibling rows) stay atomic with its outbox event.
func (s *PostgresStore) RunInTx(ctx context.Context, fn func(tx StoreTx) error) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // rollback after commit is a no-op

	adapter := &postgresStoreTxAdapter{exec: tx}
	if err := fn(adapter); err != nil {
		return err
	}
	for _, event := range adapter.events {
		if err := insertOutboxEvent(ctx, tx, event); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// postgresStoreTxAdapter exposes the in-transaction record helpers as a StoreTx,
// dispatching identity-owned resources to the typed identity tables exactly like
// the single-record *WithEvent methods do.
type postgresStoreTxAdapter struct {
	exec   postgresStoreExecutor
	events []contracts.Event
}

func (a *postgresStoreTxAdapter) Create(ctx context.Context, resource string, data map[string]any) (contracts.Record[map[string]any], error) {
	if spec, ok := typedPostgresResourceFor(resource); ok {
		return createIdentityRecordIn(ctx, a.exec, spec, data)
	}
	return createPostgresRecord(ctx, a.exec, resource, data)
}

func (a *postgresStoreTxAdapter) Update(ctx context.Context, resource, id string, data map[string]any) (contracts.Record[map[string]any], bool, error) {
	if spec, ok := typedPostgresResourceFor(resource); ok {
		return updateIdentityRecordIn(ctx, a.exec, spec, id, data)
	}
	return updatePostgresRecord(ctx, a.exec, resource, id, data)
}

func (a *postgresStoreTxAdapter) Delete(ctx context.Context, resource, id string) (bool, error) {
	if spec, ok := typedPostgresResourceFor(resource); ok {
		return deleteIdentityRecordIn(ctx, a.exec, spec, id)
	}
	return deletePostgresRecord(ctx, a.exec, resource, id)
}

func (a *postgresStoreTxAdapter) Emit(event contracts.Event) {
	a.events = append(a.events, event)
}

// NextID mirrors Store.NextID: collision-free, monotonic per (resource,prefix),
// never reused after the highest record is deleted. A transaction-scoped
// advisory lock serialises concurrent allocators across processes.
func (s *PostgresStore) NextID(resource, prefix string, base, width int) string {
	if spec, ok := typedPostgresResourceFor(resource); ok {
		return s.nextIdentityID(spec, prefix, base, width)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	key := resource + "|" + prefix

	id, err := s.allocateNextID(ctx, resource, prefix, key, base, width)
	if err != nil {
		slog.Error("postgres NextID failed; using fallback", "resource", resource, "prefix", prefix, "error", err)
		return fmt.Sprintf("%s%d", prefix, time.Now().UTC().UnixNano())
	}
	return id
}

func (s *PostgresStore) allocateNextID(ctx context.Context, resource, prefix, key string, base, width int) (string, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // rollback after commit is a no-op

	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1)::bigint)`, key); err != nil {
		return "", err
	}

	maxN, err := maxExistingID(ctx, tx, resource, prefix, base)
	if err != nil {
		return "", err
	}
	maxN, err = maxCachedID(ctx, tx, key, maxN)
	if err != nil {
		return "", err
	}
	id, maxN, err := nextAvailableID(ctx, tx, resource, prefix, maxN, width)
	if err != nil {
		return "", err
	}
	if err := saveIDHighWater(ctx, tx, key, maxN); err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return id, nil
}

func maxExistingID(ctx context.Context, tx postgresStoreTx, resource, prefix string, base int) (int, error) {
	maxN := base - 1
	rows, err := tx.Query(ctx, `SELECT id FROM platform_records WHERE resource = $1 AND id LIKE $2`, resource, prefix+"%")
	if err != nil {
		return 0, err
	}
	for rows.Next() {
		var existing string
		if err := rows.Scan(&existing); err != nil {
			rows.Close()
			return 0, err
		}
		var n int
		if _, err := fmt.Sscanf(strings.TrimPrefix(existing, prefix), "%d", &n); err == nil && n > maxN {
			maxN = n
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	return maxN, nil
}

func maxCachedID(ctx context.Context, tx postgresStoreTx, key string, maxN int) (int, error) {
	var cached int64
	if err := tx.QueryRow(ctx, `SELECT value FROM platform_id_seq WHERE key = $1`, key).Scan(&cached); err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return 0, err
		}
	} else if cached > int64(maxN) {
		maxN = int(cached)
	}
	return maxN, nil
}

func nextAvailableID(ctx context.Context, tx postgresStoreTx, resource, prefix string, maxN, width int) (string, int, error) {
	var id string
	for {
		maxN++
		if width > 0 {
			id = fmt.Sprintf("%s%0*d", prefix, width, maxN)
		} else {
			id = fmt.Sprintf("%s%d", prefix, maxN)
		}
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM platform_records WHERE resource = $1 AND id = $2)`, resource, id).Scan(&exists); err != nil {
			return "", 0, err
		}
		if !exists {
			break
		}
	}
	return id, maxN, nil
}

func saveIDHighWater(ctx context.Context, tx postgresStoreTx, key string, maxN int) error {
	if _, err := tx.Exec(ctx, `
		INSERT INTO platform_id_seq (key, value) VALUES ($1, $2)
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`, key, maxN); err != nil {
		return err
	}
	return nil
}

type postgresStoreExecutor interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (postgresRows, error)
	QueryRow(ctx context.Context, sql string, args ...any) postgresRowScanner
}

type postgresStoreDB interface {
	postgresStoreExecutor
	Begin(ctx context.Context) (postgresStoreTx, error)
}

type postgresStoreTx interface {
	postgresStoreExecutor
	Commit(ctx context.Context) error
	Rollback(ctx context.Context) error
}

type postgresRowScanner interface {
	Scan(dest ...any) error
}

type postgresRows interface {
	Close()
	Err() error
	Next() bool
	Scan(dest ...any) error
}

type pgxPoolStoreDB struct {
	pool *pgxpool.Pool
}

func (d pgxPoolStoreDB) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	return d.pool.Exec(ctx, sql, args...)
}

func (d pgxPoolStoreDB) Query(ctx context.Context, sql string, args ...any) (postgresRows, error) {
	return d.pool.Query(ctx, sql, args...)
}

func (d pgxPoolStoreDB) QueryRow(ctx context.Context, sql string, args ...any) postgresRowScanner {
	return d.pool.QueryRow(ctx, sql, args...)
}

func (d pgxPoolStoreDB) Begin(ctx context.Context) (postgresStoreTx, error) {
	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	return pgxStoreTx{tx: tx}, nil
}

type pgxStoreTx struct {
	tx pgx.Tx
}

func (t pgxStoreTx) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	return t.tx.Exec(ctx, sql, args...)
}

func (t pgxStoreTx) Query(ctx context.Context, sql string, args ...any) (postgresRows, error) {
	return t.tx.Query(ctx, sql, args...)
}

func (t pgxStoreTx) QueryRow(ctx context.Context, sql string, args ...any) postgresRowScanner {
	return t.tx.QueryRow(ctx, sql, args...)
}

func (t pgxStoreTx) Commit(ctx context.Context) error {
	return t.tx.Commit(ctx)
}

func (t pgxStoreTx) Rollback(ctx context.Context) error {
	return t.tx.Rollback(ctx)
}
