package platform

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
)

const (
	outboxRelayStatusPending    = "pending"
	outboxRelayStatusRetry      = "retry"
	outboxRelayStatusPublished  = "published"
	outboxRelayStatusDeadLetter = "dead_letter"
	defaultRelayMaxAttempts     = 5
)

const (
	eventRelayStatusDeadLetter = outboxRelayStatusDeadLetter
	defaultEventRelayAttempts  = defaultRelayMaxAttempts
)

type eventPublisher interface {
	Publish(ctx context.Context, event contracts.Event) error
}

type PostgresEventBus struct {
	db      postgresStoreDB
	relayTo eventPublisher
}

var _ EventStream = (*PostgresEventBus)(nil)

func NewPostgresEventBus(pool *pgxpool.Pool) *PostgresEventBus {
	return &PostgresEventBus{db: pgxPoolStoreDB{pool: pool}}
}

func newPostgresEventBusWithDB(db postgresStoreDB) *PostgresEventBus {
	return &PostgresEventBus{db: db}
}

func newPostgresEventBusFromDB(db postgresStoreDB) *PostgresEventBus {
	return newPostgresEventBusWithDB(db)
}

func (b *PostgresEventBus) WithRelaySink(sink eventPublisher) *PostgresEventBus {
	b.relayTo = sink
	return b
}

func (b *PostgresEventBus) Publish(ctx context.Context, event contracts.Event) error {
	if err := validateEventMetadata(event); err != nil {
		return err
	}
	return insertOutboxEvent(ctx, b.db, event)
}

func (b *PostgresEventBus) Consume(ctx context.Context, consumer string, event contracts.Event) (bool, error) {
	if consumer == "" || event.EventID == "" {
		return false, errConsumerEventRequired
	}
	tag, err := b.db.Exec(ctx, `
		INSERT INTO platform_event_inbox (consumer, event_id)
		VALUES ($1, $2)
		ON CONFLICT (consumer, event_id) DO NOTHING`, consumer, event.EventID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (b *PostgresEventBus) ResetConsumer(consumer string) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := b.db.Exec(ctx, `DELETE FROM platform_event_inbox WHERE consumer = $1`, consumer); err != nil {
		slog.Error("postgres reset consumer inbox failed", "consumer", consumer, "error", err)
	}
	if _, err := b.db.Exec(ctx, `DELETE FROM platform_event_checkpoints WHERE consumer = $1`, consumer); err != nil {
		slog.Error("postgres reset consumer checkpoint failed", "consumer", consumer, "error", err)
	}
}

func (b *PostgresEventBus) ResetConsumerEvents(consumer string, eventIDs []string) {
	if len(eventIDs) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := b.db.Exec(ctx, `DELETE FROM platform_event_inbox WHERE consumer = $1 AND event_id = ANY($2)`, consumer, eventIDs); err != nil {
		slog.Error("postgres reset consumer events failed", "consumer", consumer, "error", err)
	}
}

func (b *PostgresEventBus) Outbox() []contracts.Event {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	rows, err := b.db.Query(ctx, `
		SELECT event_id, event_name, source, trace_id, schema_version, idempotency_key, payload, occurred_at
		FROM platform_event_outbox
		ORDER BY occurred_at, created_at, event_id`)
	if err != nil {
		slog.Error("postgres outbox read failed", "error", err)
		return nil
	}
	defer rows.Close()
	events, err := scanOutboxEvents(rows)
	if err != nil {
		slog.Error("postgres outbox scan failed", "error", err)
	}
	return events
}

func (b *PostgresEventBus) Checkpoint(consumer string) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	count, lastEventID, err := b.outboxPosition(ctx)
	if err != nil {
		slog.Error("postgres checkpoint position failed", "consumer", consumer, "error", err)
		return
	}
	if _, err := b.db.Exec(ctx, `
		INSERT INTO platform_event_checkpoints (consumer, event_count, last_event_id)
		VALUES ($1, $2, $3)
		ON CONFLICT (consumer) DO UPDATE SET
			event_count = EXCLUDED.event_count,
			last_event_id = EXCLUDED.last_event_id,
			checkpointed_at = now()`, consumer, count, lastEventID); err != nil {
		slog.Error("postgres checkpoint write failed", "consumer", consumer, "error", err)
	}
}

func (b *PostgresEventBus) Lag(consumer string) int {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	count, _, err := b.outboxPosition(ctx)
	if err != nil {
		slog.Error("postgres lag outbox position failed", "consumer", consumer, "error", err)
		return 0
	}
	var checkpoint int64
	if err := b.db.QueryRow(ctx, `SELECT event_count FROM platform_event_checkpoints WHERE consumer = $1`, consumer).Scan(&checkpoint); err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			slog.Error("postgres lag checkpoint read failed", "consumer", consumer, "error", err)
			return 0
		}
		checkpoint = 0
	}
	if checkpoint > count {
		return 0
	}
	return int(count - checkpoint)
}

func (b *PostgresEventBus) RelayPending(ctx context.Context, limit int) (eventRelayResult, error) {
	if b.relayTo == nil {
		return eventRelayResult{}, errors.New("event relay sink is not configured")
	}
	if limit <= 0 {
		limit = defaultEventRelayBatchSize
	}
	pending, err := b.pendingRelayRows(ctx, limit)
	if err != nil {
		return eventRelayResult{}, err
	}
	result := eventRelayResult{Selected: len(pending)}
	var relayErr error
	for _, row := range pending {
		delta, err := b.relayOutboxRow(ctx, row)
		result.Published += delta.Published
		result.Retried += delta.Retried
		result.DeadLetter += delta.DeadLetter
		if err != nil && relayErr == nil {
			relayErr = err
		}
	}
	return result, relayErr
}

func (b *PostgresEventBus) pendingRelayRows(ctx context.Context, limit int) ([]outboxRelayRow, error) {
	rows, err := b.db.Query(ctx, `
		SELECT event_id, event_name, source, trace_id, schema_version, idempotency_key, payload, occurred_at, relay_attempts
		FROM platform_event_outbox
		WHERE relay_status IN ($1, $2)
		ORDER BY created_at, event_id
		LIMIT $3`, outboxRelayStatusPending, outboxRelayStatusRetry, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	pending := []outboxRelayRow{}
	for rows.Next() {
		row, err := scanOutboxRelayRow(rows)
		if err != nil {
			return pending, err
		}
		pending = append(pending, row)
	}
	if err := rows.Err(); err != nil {
		return pending, err
	}
	return pending, nil
}

func (b *PostgresEventBus) relayOutboxRow(ctx context.Context, row outboxRelayRow) (eventRelayResult, error) {
	if err := b.relayTo.Publish(ctx, row.event); err != nil {
		return b.recordRelayFailure(ctx, row, err)
	}
	if err := markOutboxRelayPublished(ctx, b.db, row.event.EventID); err != nil {
		return eventRelayResult{}, err
	}
	return eventRelayResult{Published: 1}, nil
}

func (b *PostgresEventBus) recordRelayFailure(ctx context.Context, row outboxRelayRow, cause error) (eventRelayResult, error) {
	attempts := row.attempts + 1
	if err := markOutboxRelayFailed(ctx, b.db, row.event.EventID, attempts, cause); err != nil {
		return eventRelayResult{}, err
	}
	slog.Warn("event relay publish failed", "event_id", row.event.EventID, "event", row.event.Name, "attempts", attempts, "error", cause)
	if attempts >= defaultRelayMaxAttempts {
		return eventRelayResult{DeadLetter: 1}, cause
	}
	return eventRelayResult{Retried: 1}, cause
}

func (b *PostgresEventBus) outboxPosition(ctx context.Context) (int64, string, error) {
	var count int64
	var lastEventID string
	err := b.db.QueryRow(ctx, `
		SELECT count(*),
			COALESCE((
				SELECT event_id
				FROM platform_event_outbox
				ORDER BY occurred_at DESC, created_at DESC, event_id DESC
				LIMIT 1
			), '')
		FROM platform_event_outbox`).Scan(&count, &lastEventID)
	return count, lastEventID, err
}

type outboxRelayRow struct {
	event    contracts.Event
	attempts int
}

func scanOutboxRelayRow(row postgresRow) (outboxRelayRow, error) {
	event, attempts, err := scanOutboxEvent(row, true)
	return outboxRelayRow{event: event, attempts: attempts}, err
}

func scanOutboxEvents(rows postgresRows) ([]contracts.Event, error) {
	var events []contracts.Event
	for rows.Next() {
		event, _, err := scanOutboxEvent(rows, false)
		if err != nil {
			return events, err
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return events, err
	}
	return events, nil
}

func scanOutboxEvent(row postgresRow, includeAttempts bool) (contracts.Event, int, error) {
	var event contracts.Event
	var raw []byte
	attempts := 0
	dest := []any{
		&event.EventID,
		&event.Name,
		&event.Source,
		&event.TraceID,
		&event.SchemaVersion,
		&event.IdempotencyKey,
		&raw,
		&event.OccurredAt,
	}
	if includeAttempts {
		dest = append(dest, &attempts)
	}
	if err := row.Scan(dest...); err != nil {
		return contracts.Event{}, 0, err
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &event.Data); err != nil {
			return contracts.Event{}, 0, fmt.Errorf("unmarshal outbox payload: %w", err)
		}
	}
	return event, attempts, nil
}

func insertOutboxEvent(ctx context.Context, db postgresStoreExecutor, event contracts.Event) error {
	if err := validateEventMetadata(event); err != nil {
		return err
	}
	payload, err := json.Marshal(event.Data)
	if err != nil {
		return fmt.Errorf("marshal outbox payload: %w", err)
	}
	if _, err := db.Exec(ctx, `
		INSERT INTO platform_event_outbox (
			event_id, event_name, source, trace_id, schema_version,
			idempotency_key, payload, occurred_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (event_id) DO NOTHING`,
		event.EventID,
		event.Name,
		event.Source,
		event.TraceID,
		event.SchemaVersion,
		event.IdempotencyKey,
		payload,
		event.OccurredAt,
	); err != nil {
		return fmt.Errorf("insert outbox event: %w", err)
	}
	return nil
}

func markOutboxRelayPublished(ctx context.Context, db postgresStoreExecutor, eventID string) error {
	_, err := db.Exec(ctx, `
		UPDATE platform_event_outbox
		SET relay_status = $2,
			published_at = now(),
			updated_at = now(),
			last_error = NULL
		WHERE event_id = $1`, eventID, outboxRelayStatusPublished)
	return err
}

func markOutboxRelayFailed(ctx context.Context, db postgresStoreExecutor, eventID string, attempts int, cause error) error {
	status := outboxRelayStatusRetry
	if attempts >= defaultRelayMaxAttempts {
		status = outboxRelayStatusDeadLetter
	}
	_, err := db.Exec(ctx, `
		UPDATE platform_event_outbox
		SET relay_status = $2,
			relay_attempts = $3,
			last_error = $4,
			updated_at = now()
		WHERE event_id = $1`, eventID, status, attempts, cause.Error())
	return err
}
