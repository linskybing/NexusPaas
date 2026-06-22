-- Route request-notification-service form records into service-owned typed
-- tables while retaining the legacy generic platform_records rows for rollback.
-- This is the first typed-ownership slice for this service (see
-- backend/docs/migration-roadmap.md); the full record stays in the payload
-- JSONB column, so reads reconstruct identical maps. The promoted columns exist
-- for clear ownership and indexed user/project/status queries.

CREATE TABLE IF NOT EXISTS forms (
    id         TEXT        PRIMARY KEY,
    user_id    TEXT        NOT NULL DEFAULT '',
    project_id TEXT,
    tag        TEXT        NOT NULL DEFAULT '',
    title      TEXT        NOT NULL DEFAULT '',
    status     TEXT        NOT NULL DEFAULT 'Pending',
    payload    JSONB       NOT NULL DEFAULT '{}'::jsonb,
    version    INTEGER     NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_forms_user_id ON forms (user_id);
CREATE INDEX IF NOT EXISTS idx_forms_project_id ON forms (project_id);
CREATE INDEX IF NOT EXISTS idx_forms_status ON forms (status);

CREATE TABLE IF NOT EXISTS form_messages (
    id         TEXT        PRIMARY KEY,
    form_id    TEXT        NOT NULL DEFAULT '',
    user_id    TEXT        NOT NULL DEFAULT '',
    payload    JSONB       NOT NULL DEFAULT '{}'::jsonb,
    version    INTEGER     NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_form_messages_form_id ON form_messages (form_id);

-- Backfill from the generic store. The legacy request_notification_records /
-- platform_records rows are intentionally left in place so this slice is
-- reversible (expand + dual-write phase; cutover/legacy-drop is a later slice).
INSERT INTO forms (id, user_id, project_id, tag, title, status, payload, version, created_at, updated_at)
SELECT
    id,
    COALESCE(NULLIF(payload->>'user_id', ''), NULLIF(payload->>'userId', ''), ''),
    COALESCE(NULLIF(payload->>'project_id', ''), NULLIF(payload->>'projectId', '')),
    COALESCE(NULLIF(payload->>'tag', ''), ''),
    COALESCE(NULLIF(payload->>'title', ''), ''),
    COALESCE(NULLIF(payload->>'status', ''), 'Pending'),
    payload || jsonb_build_object('id', id),
    version,
    created_at,
    updated_at
FROM platform_records
WHERE resource = 'request-notification-service:forms'
ON CONFLICT DO NOTHING;

INSERT INTO form_messages (id, form_id, user_id, payload, version, created_at, updated_at)
SELECT
    id,
    COALESCE(NULLIF(payload->>'form_id', ''), NULLIF(payload->>'formId', ''), ''),
    COALESCE(NULLIF(payload->>'user_id', ''), NULLIF(payload->>'userId', ''), ''),
    payload || jsonb_build_object('id', id),
    version,
    created_at,
    updated_at
FROM platform_records
WHERE resource = 'request-notification-service:form_messages'
ON CONFLICT DO NOTHING;
