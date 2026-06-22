-- Unified record table backing PostgresStore (RecordStore port).
-- The runtime uses arbitrary string ids (e.g. US2600004, session tokens), so id
-- is TEXT and the primary key is (resource, id): the same id may exist under
-- different resource keys, mirroring the in-memory store's data[resource][id].
CREATE TABLE IF NOT EXISTS platform_records (
    resource   TEXT        NOT NULL,
    id         TEXT        NOT NULL,
    payload    JSONB       NOT NULL,
    version    INTEGER     NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (resource, id)
);

CREATE INDEX IF NOT EXISTS idx_platform_records_resource_created
    ON platform_records (resource, created_at);

-- Monotonic high-water mark per (resource|prefix) so NextID never reuses an id
-- after the highest record is deleted (matches the in-memory store's seq map).
CREATE TABLE IF NOT EXISTS platform_id_seq (
    key   TEXT   PRIMARY KEY,
    value BIGINT NOT NULL
);

-- Durable event outbox backing the transactional Outbox/Inbox pattern. Event ids
-- remain TEXT because contracts.Event.EventID is a string and existing fixtures
-- use UUID-shaped strings by convention, not by storage type.
CREATE TABLE IF NOT EXISTS platform_event_outbox (
    event_id        TEXT        PRIMARY KEY,
    event_name      TEXT        NOT NULL,
    source          TEXT        NOT NULL,
    trace_id        TEXT        NOT NULL,
    schema_version  INTEGER     NOT NULL,
    idempotency_key TEXT        NOT NULL DEFAULT '',
    payload         JSONB       NOT NULL,
    occurred_at     TIMESTAMPTZ NOT NULL,
    relay_status    TEXT        NOT NULL DEFAULT 'pending',
    relay_attempts  INTEGER     NOT NULL DEFAULT 0,
    last_error      TEXT,
    published_at    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (relay_status IN ('pending', 'retry', 'published', 'dead_letter')),
    CHECK (schema_version > 0),
    CHECK (relay_attempts >= 0)
);

CREATE INDEX IF NOT EXISTS idx_platform_event_outbox_relay
    ON platform_event_outbox (relay_status, created_at, event_id);

CREATE INDEX IF NOT EXISTS idx_platform_event_outbox_occurred
    ON platform_event_outbox (occurred_at, created_at, event_id);

CREATE TABLE IF NOT EXISTS platform_event_inbox (
    consumer     TEXT        NOT NULL,
    event_id     TEXT        NOT NULL,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (consumer, event_id)
);

CREATE TABLE IF NOT EXISTS platform_event_checkpoints (
    consumer        TEXT        PRIMARY KEY,
    event_count     BIGINT      NOT NULL,
    last_event_id   TEXT,
    checkpointed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (event_count >= 0)
);
