-- Complete login_failures metadata required by identity-owned store reads.

ALTER TABLE login_failures
    ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ;

UPDATE login_failures AS failure
SET created_at = records.created_at
FROM platform_records AS records
WHERE records.resource = 'identity-service:login_failures'
  AND records.id = failure.id
  AND failure.created_at IS NULL;

UPDATE login_failures
SET created_at = COALESCE(created_at, updated_at, now())
WHERE created_at IS NULL;

ALTER TABLE login_failures
    ALTER COLUMN created_at SET DEFAULT now(),
    ALTER COLUMN created_at SET NOT NULL;
