#!/usr/bin/env bash
# db-backup-restore-drill.sh — repeatable backup/restore drill against a real
# Postgres (OPS: backup restore procedure evidence). Full destructive
# round-trip: exact per-table row snapshot + drill marker → pg_dump (custom
# format) → DROP DATABASE (FORCE) → prove the DB is gone → CREATE DATABASE →
# pg_restore → re-snapshot and diff → migration idempotency check
# (ADMIN_TASK=apply-migrations must apply 0 on the restored schema).
#
#   !!! LOCAL-TIER DRILL — NOT EXTERNAL GA PROOF !!!
# Per docs/agents/workflow.md, run output must be labeled by where it ran
# (local docker compose Postgres / kind). The procedure itself is
# environment-agnostic: point PG_CONTAINER at any disposable Postgres.
#
# Usage (local compose stack):
#   docker compose -f backend/deploy/local/docker-compose.yml up -d postgres
#   PG_PASSWORD=<pw> bash backend/scripts/db-backup-restore-drill.sh
#
# ponytail: docker-exec'd psql/pg_dump/pg_restore — no host Postgres client
# needed and versions always match the server. Upgrade path: point at a
# managed instance by swapping the exec wrapper for direct psql.
set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BACKEND_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

PG_CONTAINER="${PG_CONTAINER:-local-postgres-1}"
PG_USER="${PG_USER:-nexuspaas}"
PG_DB="${PG_DB:-nexuspaas}"
PG_PASSWORD="${PG_PASSWORD:-}"
RUN_ID="$(date -u '+%Y%m%d%H%M%S')"
ARTIFACT_DIR="${ARTIFACT_DIR:-${TMPDIR:-/tmp}/nexuspaas-restore-drill/${RUN_ID}}"
DUMP_FILE="${ARTIFACT_DIR}/${PG_DB}-${RUN_ID}.dump"
MARKER="restore-drill-${RUN_ID}"

fail() { echo "FAIL: $*" >&2; exit 1; }
step() { echo; echo ">> $*"; }

command -v docker >/dev/null 2>&1 || fail "docker is required"
docker inspect -f '{{.State.Running}}' "${PG_CONTAINER}" 2>/dev/null | grep -q true \
  || fail "Postgres container ${PG_CONTAINER} is not running"

psql_db()    { docker exec -i "${PG_CONTAINER}" psql -v ON_ERROR_STOP=1 -U "${PG_USER}" -d "${PG_DB}" "$@"; }
psql_admin() { docker exec -i "${PG_CONTAINER}" psql -v ON_ERROR_STOP=1 -U "${PG_USER}" -d postgres "$@"; }

# exact per-table row counts for every public table (information_schema +
# \gexec, so new tables are covered automatically)
snapshot() {
  psql_db -At <<'SQL'
SELECT format('SELECT %L || '','' || count(*) FROM %I', table_name, table_name)
FROM information_schema.tables
WHERE table_schema = 'public' AND table_type = 'BASE TABLE'
ORDER BY table_name
\gexec
SQL
}

mkdir -p "${ARTIFACT_DIR}"
echo "restore drill run ${RUN_ID}: container=${PG_CONTAINER} db=${PG_DB} artifacts=${ARTIFACT_DIR}"

step "1. seed drill marker + snapshot exact row counts (before)"
psql_db -q <<SQL
CREATE TABLE IF NOT EXISTS restore_drill_markers (
  id text PRIMARY KEY,
  created_at timestamptz NOT NULL DEFAULT now()
);
INSERT INTO restore_drill_markers (id) VALUES ('${MARKER}');
SQL
snapshot > "${ARTIFACT_DIR}/counts-before.csv"
TABLES_BEFORE="$(grep -c "" "${ARTIFACT_DIR}/counts-before.csv")"
echo "snapshot: ${TABLES_BEFORE} tables (counts-before.csv)"

step "2. pg_dump (custom format)"
docker exec "${PG_CONTAINER}" pg_dump -U "${PG_USER}" -Fc "${PG_DB}" > "${DUMP_FILE}"
echo "dump: $(du -h "${DUMP_FILE}" | cut -f1) ${DUMP_FILE}"

step "3. destroy: DROP DATABASE ${PG_DB} WITH (FORCE)"
psql_admin -q -c "DROP DATABASE ${PG_DB} WITH (FORCE);"
if psql_db -q -c "SELECT 1;" >/dev/null 2>&1; then
  fail "database still answers after DROP — destruction did not happen"
fi
echo "verified: database is gone (connection refused)"

step "4. recreate empty database + pg_restore"
psql_admin -q -c "CREATE DATABASE ${PG_DB} OWNER ${PG_USER};"
docker exec -i "${PG_CONTAINER}" pg_restore -U "${PG_USER}" -d "${PG_DB}" --no-owner < "${DUMP_FILE}"
echo "restore: pg_restore completed"

step "5. validate: row counts match + drill marker survived"
snapshot > "${ARTIFACT_DIR}/counts-after.csv"
if ! diff -u "${ARTIFACT_DIR}/counts-before.csv" "${ARTIFACT_DIR}/counts-after.csv" > "${ARTIFACT_DIR}/counts.diff"; then
  cat "${ARTIFACT_DIR}/counts.diff"
  fail "row counts differ after restore (see counts.diff)"
fi
MARKER_OK="$(psql_db -At -c "SELECT count(*) FROM restore_drill_markers WHERE id = '${MARKER}';")"
[[ "${MARKER_OK}" == "1" ]] || fail "drill marker ${MARKER} missing after restore"
echo "validated: ${TABLES_BEFORE} tables identical, marker ${MARKER} present"

step "6. migration idempotency on restored schema (ADMIN_TASK=apply-migrations)"
PG_HOST_PORT="${PG_HOST_PORT:-5432}"
DRILL_DATABASE_URL="${DRILL_DATABASE_URL:-postgres://${PG_USER}:${PG_PASSWORD}@localhost:${PG_HOST_PORT}/${PG_DB}?sslmode=disable}"
MIGRATE_OUT="$(cd "${BACKEND_DIR}" && SERVICE_NAME=all ADMIN_TASK=apply-migrations \
  DATABASE_URL="${DRILL_DATABASE_URL}" go run ./cmd/microservice 2>&1 | tee "${ARTIFACT_DIR}/migrate.log")"
echo "${MIGRATE_OUT}" | grep -q "applied=0" \
  || fail "migrations were not a no-op on the restored schema (see migrate.log)"
echo "validated: migration summary reports applied=0 (schema fully restored)"

step "cleanup: drop drill marker row"
psql_db -q -c "DELETE FROM restore_drill_markers WHERE id = '${MARKER}';"

echo
echo "RESTORE DRILL PASS — run ${RUN_ID}"
echo "  dump:      ${DUMP_FILE}"
echo "  evidence:  ${ARTIFACT_DIR} (counts-before.csv, counts-after.csv, migrate.log)"
