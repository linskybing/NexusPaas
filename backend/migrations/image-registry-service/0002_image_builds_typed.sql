-- Route image-registry-service build-job records into a typed, service-owned
-- table while retaining the legacy generic platform_records rows for rollback
-- (expand + dual-write phase; cutover/legacy-drop is a later slice). The full
-- record stays in the payload JSONB column so reads reconstruct identical maps;
-- the promoted columns give clear ownership and indexed project/status queries
-- over production-critical supply-chain state. (P0-5)

CREATE TABLE IF NOT EXISTS image_build_jobs (
    id              TEXT        PRIMARY KEY,
    project_id      TEXT        NOT NULL DEFAULT '',
    image_reference TEXT        NOT NULL DEFAULT '',
    build_type      TEXT        NOT NULL DEFAULT '',
    status          TEXT        NOT NULL DEFAULT 'queued',
    requested_by    TEXT        NOT NULL DEFAULT '',
    source_digest   TEXT,
    payload         JSONB       NOT NULL DEFAULT '{}'::jsonb,
    version         INTEGER     NOT NULL DEFAULT 1,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT image_build_jobs_status_not_blank CHECK (status <> '')
);

CREATE INDEX IF NOT EXISTS idx_image_build_jobs_project_id ON image_build_jobs (project_id);
CREATE INDEX IF NOT EXISTS idx_image_build_jobs_status ON image_build_jobs (status);

-- Backfill from the generic store. Legacy platform_records rows are intentionally
-- left in place so this slice is reversible.
INSERT INTO image_build_jobs (id, project_id, image_reference, build_type, status, requested_by, source_digest, payload, version, created_at, updated_at)
SELECT
    id,
    COALESCE(NULLIF(payload->>'project_id', ''), NULLIF(payload->>'projectId', ''), ''),
    COALESCE(NULLIF(payload->>'image_reference', ''), NULLIF(payload->>'imageReference', ''), ''),
    COALESCE(NULLIF(payload->>'build_type', ''), NULLIF(payload->>'buildType', ''), ''),
    COALESCE(NULLIF(payload->>'status', ''), 'queued'),
    COALESCE(NULLIF(payload->>'requested_by', ''), NULLIF(payload->>'requestedBy', ''), ''),
    NULLIF(payload->>'source_digest', ''),
    payload || jsonb_build_object('id', id),
    version,
    created_at,
    updated_at
FROM platform_records
WHERE resource = 'image-registry-service:image_build_jobs'
ON CONFLICT DO NOTHING;
