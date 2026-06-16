-- Route identity-service durable records into identity-owned tables while
-- retaining legacy platform_records rows for rollback.

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN IF NOT EXISTS version INTEGER NOT NULL DEFAULT 1;

ALTER TABLE sessions
    ADD COLUMN IF NOT EXISTS payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN IF NOT EXISTS version INTEGER NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();

ALTER TABLE refresh_tokens
    ADD COLUMN IF NOT EXISTS payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN IF NOT EXISTS version INTEGER NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();

ALTER TABLE user_api_tokens
    ADD COLUMN IF NOT EXISTS payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN IF NOT EXISTS version INTEGER NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();

ALTER TABLE captchas
    ADD COLUMN IF NOT EXISTS payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN IF NOT EXISTS version INTEGER NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();

ALTER TABLE login_failures
    ADD COLUMN IF NOT EXISTS payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN IF NOT EXISTS version INTEGER NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT now();

CREATE TABLE IF NOT EXISTS identity_roles (
    id         TEXT        PRIMARY KEY,
    name       TEXT        NOT NULL UNIQUE,
    payload    JSONB       NOT NULL DEFAULT '{}'::jsonb,
    version    INTEGER     NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

UPDATE users
SET payload = payload || jsonb_strip_nulls(jsonb_build_object(
        'id', id,
        'username', username,
        'email', email,
        'full_name', full_name,
        'password_hash', password_hash,
        'role', role,
        'role_id', role_id,
        'system_role', system_role,
        'type', type,
        'status', status
    ));

UPDATE sessions
SET payload = payload || jsonb_strip_nulls(jsonb_build_object(
        'id', id,
        'token', token,
        'user_id', user_id,
        'expires_at', expires_at,
        'created_at', created_at
    ));

UPDATE refresh_tokens
SET payload = payload || jsonb_strip_nulls(jsonb_build_object(
        'id', id,
        'token', token,
        'user_id', user_id,
        'expires_at', expires_at,
        'created_at', created_at
    ));

UPDATE user_api_tokens
SET payload = payload || jsonb_strip_nulls(jsonb_build_object(
        'id', id,
        'user_id', user_id,
        'name', name,
        'token_hash', token_hash,
        'token_prefix', token_prefix,
        'expires_at', expires_at,
        'last_used_at', last_used_at,
        'revoked', revoked,
        'revoked_at', revoked_at,
        'created_at', created_at
    ));

UPDATE captchas
SET payload = payload || jsonb_strip_nulls(jsonb_build_object(
        'id', id,
        'answer_hash', answer_hash,
        'expires_at', expires_at,
        'created_at', created_at
    ));

UPDATE login_failures
SET payload = payload || jsonb_strip_nulls(jsonb_build_object(
        'id', id,
        'username', username,
        'ip', ip,
        'failures', failures,
        'locked_until', locked_until,
        'created_at', created_at,
        'updated_at', updated_at
    ));

INSERT INTO users (
    id, username, email, full_name, password_hash, role, role_id, system_role,
    type, status, payload, version, created_at, updated_at
)
SELECT
    id,
    COALESCE(NULLIF(payload->>'username', ''), NULLIF(payload->>'name', ''), id),
    NULLIF(payload->>'email', ''),
    NULLIF(payload->>'full_name', ''),
    COALESCE(NULLIF(payload->>'password_hash', ''), ''),
    COALESCE(NULLIF(payload->>'role', ''), 'user'),
    COALESCE(NULLIF(payload->>'role_id', ''), 'RO2600004'),
    CASE WHEN COALESCE(payload->>'system_role', '') ~ '^-?[0-9]+$' THEN (payload->>'system_role')::integer ELSE 2 END,
    COALESCE(NULLIF(payload->>'type', ''), 'origin'),
    COALESCE(NULLIF(payload->>'status', ''), 'offline'),
    payload || jsonb_build_object('id', id),
    version,
    created_at,
    updated_at
FROM platform_records
WHERE resource = 'identity-service:users'
ON CONFLICT DO NOTHING;

INSERT INTO identity_roles (id, name, payload, version, created_at, updated_at)
SELECT
    id,
    COALESCE(NULLIF(payload->>'name', ''), id),
    payload || jsonb_build_object('id', id),
    version,
    created_at,
    updated_at
FROM platform_records
WHERE resource = 'identity-service:roles'
ON CONFLICT DO NOTHING;

INSERT INTO sessions (id, user_id, token, expires_at, created_at, payload, version, updated_at)
SELECT
    id,
    payload->>'user_id',
    COALESCE(NULLIF(payload->>'token', ''), id),
    COALESCE(NULLIF(payload->>'expires_at', '')::timestamptz, created_at + interval '1 hour'),
    created_at,
    payload || jsonb_build_object('id', id, 'token', COALESCE(NULLIF(payload->>'token', ''), id)),
    version,
    updated_at
FROM platform_records
WHERE resource = 'identity-service:sessions'
  AND NULLIF(payload->>'user_id', '') IS NOT NULL
  AND EXISTS (SELECT 1 FROM users WHERE users.id = platform_records.payload->>'user_id')
ON CONFLICT DO NOTHING;

INSERT INTO refresh_tokens (id, user_id, token, expires_at, created_at, payload, version, updated_at)
SELECT
    id,
    payload->>'user_id',
    COALESCE(NULLIF(payload->>'token', ''), id),
    COALESCE(NULLIF(payload->>'expires_at', '')::timestamptz, created_at + interval '1 hour'),
    created_at,
    payload || jsonb_build_object('id', id, 'token', COALESCE(NULLIF(payload->>'token', ''), id)),
    version,
    updated_at
FROM platform_records
WHERE resource = 'identity-service:refresh_tokens'
  AND NULLIF(payload->>'user_id', '') IS NOT NULL
  AND EXISTS (SELECT 1 FROM users WHERE users.id = platform_records.payload->>'user_id')
ON CONFLICT DO NOTHING;

INSERT INTO user_api_tokens (
    id, user_id, name, token_hash, token_prefix, expires_at, last_used_at,
    revoked, revoked_at, created_at, payload, version, updated_at
)
SELECT
    id,
    payload->>'user_id',
    COALESCE(NULLIF(payload->>'name', ''), ''),
    COALESCE(NULLIF(payload->>'token_hash', ''), ''),
    COALESCE(NULLIF(payload->>'token_prefix', ''), ''),
    NULLIF(payload->>'expires_at', '')::timestamptz,
    NULLIF(payload->>'last_used_at', '')::timestamptz,
    COALESCE((payload->>'revoked')::boolean, false),
    NULLIF(payload->>'revoked_at', '')::timestamptz,
    created_at,
    (payload - 'token') || jsonb_build_object('id', id),
    version,
    updated_at
FROM platform_records
WHERE resource = 'identity-service:api_tokens'
  AND NULLIF(payload->>'user_id', '') IS NOT NULL
  AND EXISTS (SELECT 1 FROM users WHERE users.id = platform_records.payload->>'user_id')
ON CONFLICT DO NOTHING;

INSERT INTO captchas (id, answer_hash, expires_at, created_at, payload, version, updated_at)
SELECT
    id,
    COALESCE(NULLIF(payload->>'answer_hash', ''), ''),
    COALESCE(NULLIF(payload->>'expires_at', '')::timestamptz, created_at + interval '5 minutes'),
    created_at,
    payload || jsonb_build_object('id', id),
    version,
    updated_at
FROM platform_records
WHERE resource = 'identity-service:captchas'
ON CONFLICT DO NOTHING;

INSERT INTO login_failures (id, username, ip, failures, locked_until, created_at, updated_at, payload, version)
SELECT
    id,
    COALESCE(NULLIF(payload->>'username', ''), id),
    COALESCE(NULLIF(payload->>'ip', ''), ''),
    CASE WHEN COALESCE(payload->>'failures', '') ~ '^-?[0-9]+$' THEN (payload->>'failures')::integer ELSE 0 END,
    NULLIF(payload->>'locked_until', '')::timestamptz,
    created_at,
    updated_at,
    payload || jsonb_build_object('id', id),
    version
FROM platform_records
WHERE resource = 'identity-service:login_failures'
ON CONFLICT DO NOTHING;
