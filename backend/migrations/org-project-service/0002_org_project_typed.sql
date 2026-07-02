-- Route org-project-service project + membership records into typed,
-- service-owned tables while retaining the legacy generic platform_records
-- rows for rollback (expand + dual-write phase; cutover/legacy-drop is a later
-- slice, same shape as identity 0002 / image-registry 0002). Projects and
-- members are the authz-critical aggregates every other service reads; the
-- remaining org-project resources (groups, user_groups, user_quotas,
-- gpu_claims) stay on platform_records as recorded debt.

CREATE TABLE IF NOT EXISTS org_projects (
    id           TEXT        PRIMARY KEY,
    project_name TEXT        NOT NULL DEFAULT '',
    owner_id     TEXT        NOT NULL DEFAULT '',
    created_by   TEXT        NOT NULL DEFAULT '',
    payload      JSONB       NOT NULL DEFAULT '{}'::jsonb,
    version      INTEGER     NOT NULL DEFAULT 1,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_org_projects_owner_id ON org_projects (owner_id);

CREATE TABLE IF NOT EXISTS org_project_members (
    id         TEXT        PRIMARY KEY,
    project_id TEXT        NOT NULL DEFAULT '',
    user_id    TEXT        NOT NULL DEFAULT '',
    role       TEXT        NOT NULL DEFAULT 'user',
    payload    JSONB       NOT NULL DEFAULT '{}'::jsonb,
    version    INTEGER     NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT org_project_members_role_not_blank CHECK (role <> '')
);

CREATE INDEX IF NOT EXISTS idx_org_project_members_project_id ON org_project_members (project_id);
CREATE INDEX IF NOT EXISTS idx_org_project_members_user_id ON org_project_members (user_id);

-- Backfill from the generic store. Legacy platform_records rows are
-- intentionally left in place so this slice is reversible.
INSERT INTO org_projects (id, project_name, owner_id, created_by, payload, version, created_at, updated_at)
SELECT
    id,
    COALESCE(NULLIF(payload->>'project_name', ''), NULLIF(payload->>'ProjectName', ''), NULLIF(payload->>'name', ''), ''),
    COALESCE(NULLIF(payload->>'owner_id', ''), NULLIF(payload->>'g_id', ''), NULLIF(payload->>'GID', ''), ''),
    COALESCE(NULLIF(payload->>'created_by', ''), NULLIF(payload->>'createdBy', ''), ''),
    payload || jsonb_build_object('id', id),
    version,
    created_at,
    updated_at
FROM platform_records
WHERE resource = 'org-project-service:projects'
ON CONFLICT DO NOTHING;

INSERT INTO org_project_members (id, project_id, user_id, role, payload, version, created_at, updated_at)
SELECT
    id,
    COALESCE(NULLIF(payload->>'project_id', ''), NULLIF(payload->>'projectId', ''), ''),
    COALESCE(NULLIF(payload->>'user_id', ''), NULLIF(payload->>'userId', ''), ''),
    COALESCE(NULLIF(payload->>'role', ''), 'user'),
    payload || jsonb_build_object('id', id),
    version,
    created_at,
    updated_at
FROM platform_records
WHERE resource = 'org-project-service:project_members'
ON CONFLICT DO NOTHING;
