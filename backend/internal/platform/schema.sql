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
