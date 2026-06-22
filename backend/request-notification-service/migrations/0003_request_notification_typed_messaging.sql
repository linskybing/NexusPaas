-- Route request-notification-service announcement and notification records into
-- service-owned typed tables, finishing this service's move off the generic
-- platform_records store (see 0002 for forms and backend/docs/migration-roadmap.md).
-- The full record stays in the payload JSONB column so reads reconstruct identical
-- maps; promoted columns exist for ownership and indexed queries. Legacy
-- platform_records rows are intentionally retained for rollback.

CREATE TABLE IF NOT EXISTS announcements (
    id           TEXT        PRIMARY KEY,
    priority     TEXT        NOT NULL DEFAULT 'info',
    is_pinned    BOOLEAN     NOT NULL DEFAULT false,
    published_at TIMESTAMPTZ,
    expires_at   TIMESTAMPTZ,
    created_by   TEXT        NOT NULL DEFAULT '',
    payload      JSONB       NOT NULL DEFAULT '{}'::jsonb,
    version      INTEGER     NOT NULL DEFAULT 1,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_announcements_priority ON announcements (priority);
CREATE INDEX IF NOT EXISTS idx_announcements_is_pinned ON announcements (is_pinned);
CREATE INDEX IF NOT EXISTS idx_announcements_published_at ON announcements (published_at);

CREATE TABLE IF NOT EXISTS announcement_reads (
    id              TEXT        PRIMARY KEY,
    announcement_id TEXT        NOT NULL DEFAULT '',
    user_id         TEXT        NOT NULL DEFAULT '',
    payload         JSONB       NOT NULL DEFAULT '{}'::jsonb,
    version         INTEGER     NOT NULL DEFAULT 1,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_announcement_reads_announcement_id ON announcement_reads (announcement_id);
CREATE INDEX IF NOT EXISTS idx_announcement_reads_user_id ON announcement_reads (user_id);

CREATE TABLE IF NOT EXISTS notifications (
    id              TEXT        PRIMARY KEY,
    user_id         TEXT        NOT NULL DEFAULT '',
    notification_id TEXT        NOT NULL DEFAULT '',
    read            BOOLEAN     NOT NULL DEFAULT false,
    payload         JSONB       NOT NULL DEFAULT '{}'::jsonb,
    version         INTEGER     NOT NULL DEFAULT 1,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_notifications_user_id ON notifications (user_id);

-- Backfill from the generic store, promoting the same JSONB keys (with camelCase
-- fallbacks) the store column builders read.
INSERT INTO announcements (id, priority, is_pinned, published_at, expires_at, created_by, payload, version, created_at, updated_at)
SELECT
    id,
    COALESCE(NULLIF(payload->>'priority', ''), 'info'),
    -- Guard the cast: malformed stored values must not abort the migration.
    CASE WHEN lower(payload->>'is_pinned') = 'true' THEN true ELSE false END,
    CASE WHEN payload->>'published_at' ~ '^\d{4}-\d{2}-\d{2}' THEN (payload->>'published_at')::timestamptz ELSE NULL END,
    CASE WHEN payload->>'expires_at' ~ '^\d{4}-\d{2}-\d{2}' THEN (payload->>'expires_at')::timestamptz ELSE NULL END,
    COALESCE(NULLIF(payload->>'created_by', ''), NULLIF(payload->>'createdBy', ''), ''),
    payload || jsonb_build_object('id', id),
    version,
    created_at,
    updated_at
FROM platform_records
WHERE resource = 'request-notification-service:announcements'
ON CONFLICT DO NOTHING;

INSERT INTO announcement_reads (id, announcement_id, user_id, payload, version, created_at, updated_at)
SELECT
    id,
    COALESCE(NULLIF(payload->>'announcement_id', ''), NULLIF(payload->>'announcementId', ''), ''),
    COALESCE(NULLIF(payload->>'user_id', ''), NULLIF(payload->>'userId', ''), ''),
    payload || jsonb_build_object('id', id),
    version,
    created_at,
    updated_at
FROM platform_records
WHERE resource = 'request-notification-service:announcement_reads'
ON CONFLICT DO NOTHING;

INSERT INTO notifications (id, user_id, notification_id, read, payload, version, created_at, updated_at)
SELECT
    id,
    COALESCE(NULLIF(payload->>'user_id', ''), NULLIF(payload->>'userId', ''), ''),
    COALESCE(NULLIF(payload->>'notification_id', ''), NULLIF(payload->>'notificationId', ''), ''),
    -- Guard the cast: malformed stored values must not abort the migration.
    CASE WHEN lower(payload->>'read') = 'true' THEN true ELSE false END,
    payload || jsonb_build_object('id', id),
    version,
    created_at,
    updated_at
FROM platform_records
WHERE resource = 'request-notification-service:notifications'
ON CONFLICT DO NOTHING;
