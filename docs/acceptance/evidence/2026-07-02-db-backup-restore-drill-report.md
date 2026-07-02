# DB Backup/Restore Drill — Full Destructive Round-Trip Evidence

> **LOCAL-TIER DRILL — NOT EXTERNAL GA PROOF.** Per `docs/agents/workflow.md`,
> local evidence must not be described as external GA proof. This report proves
> the **backup/restore procedure executes end to end against a real Postgres**
> via the repeatable one-command harness
> `backend/scripts/db-backup-restore-drill.sh`. It does not prove managed-DB
> (PITR/WAL) restore on external infrastructure — that remains an external-tier
> follow-up.

- Run: local, 2026-07-02, run id `20260702131529`, branch `ac-completion-round`
- Target: `postgres:16-alpine` from `backend/deploy/local/docker-compose.yml`
  (container `local-postgres-1`), database `nexuspaas` with the full 21-unit
  migration set applied (78 public tables)
- Harness: `PG_PASSWORD=… bash backend/scripts/db-backup-restore-drill.sh`

## Drill steps (all pass, single run)

| step | mechanism | result |
| --- | --- | --- |
| marker + snapshot | drill-owned `restore_drill_markers` row + exact `count(*)` per public table (`information_schema` + `\gexec`) | 78 tables snapshotted |
| backup | `pg_dump -Fc` (custom format) | 124K dump |
| destroy | `DROP DATABASE nexuspaas WITH (FORCE)` | verified: connection refused after drop |
| restore | `CREATE DATABASE` + `pg_restore --no-owner` | completed |
| data validation | re-snapshot + `diff` of per-table counts; marker row lookup | 78/78 tables identical, marker present |
| schema validation | `ADMIN_TASK=apply-migrations` on the restored DB | `migration summary: applied=0 skipped=21 total=21` (restore reproduced the full schema; migrations are a no-op) |

Artifacts (per run, machine-local): `counts-before.csv`, `counts-after.csv`,
`counts.diff` (empty), `migrate.log`, and the `.dump` file under
`/tmp/nexuspaas-restore-drill/<run-id>/`.

## Notes

- The drill is destructive by design and self-contained: it owns its marker
  table, drops only the target database, and restores it from the dump it just
  took. Point `PG_CONTAINER`/`PG_HOST_PORT`/`DRILL_DATABASE_URL` at any
  disposable Postgres to re-run.
- Forward-only migrations remain the schema-change policy; this drill is the
  documented rollback path (restore-from-backup), which is why the
  apply-migrations no-op check matters: a restored dump needs no schema repair.
