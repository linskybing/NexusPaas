# PostgreSQL Backup Restore Drill

Status: Approved

## 1. Objective

Capture live OPS-006 evidence that the `nexuspaas` PostgreSQL database can be backed up and
restored by creating a `pg_dump -Fc` archive, restoring it into a temporary database, validating
schema and selected row counts, and cleaning up the temporary restore target.

## 2. Background

`gap.md` still records Backup/restore as open because no restore drill has been evidenced. The
live `nexuspaas` namespace contains a `postgres` deployment using `postgres:16-alpine`,
`postgres-data` PVC, and a `postgres-password` Secret reference. The pod already contains
`pg_dump`, `pg_restore`, `createdb`, `dropdb`, and `psql`, so this slice can use mature
PostgreSQL tools instead of adding a custom backup mechanism.

Official documentation references:

- PostgreSQL `pg_dump`: https://www.postgresql.org/docs/current/app-pgdump.html
- PostgreSQL `pg_restore`: https://www.postgresql.org/docs/current/app-pgrestore.html
- PostgreSQL SQL dump restore guidance: https://www.postgresql.org/docs/current/backup-dump.html
- Kubernetes `kubectl exec`: https://kubernetes.io/docs/reference/kubectl/generated/kubectl_exec/

## 3. Source References

- `docs/acceptance/operations.md`
- `docs/acceptance/ga-checklist.md`
- `gap.md`
- `problem.md`
- Live `nexuspaas` namespace `postgres` deployment and pod state from `kubectl`

## 4. Assumptions

- The current `kubectl` context is the same live local/RKE2-style context used by prior GA
  evidence slices.
- The `postgres` deployment is ready before the drill starts.
- The live system is in a quiet maintenance window for this short drill; selected source row counts
  must be stable immediately before and after `pg_dump`.
- The `POSTGRES_USER` role can create and drop a temporary database.
- The backup archive is handled as sensitive operational data and is not committed.

## 5. Non-Goals

- Do not restore over the live `nexuspaas` database.
- Do not change application source code, manifests, runtime config, or secrets.
- Do not add a new backup product in this slice.
- Do not claim OPS-007 Harbor, OPS-008 object storage, OPS-009 secret recovery, PITR, off-cluster
  retention, encrypted backup storage, or full DR coverage.

## 6. Current Behavior

The live database is reachable and has 81 public tables. No documented live restore drill evidence
exists in the GA ledgers, so `Backup/restore` remains open.

## 7. Target Behavior

The live PostgreSQL database has a successful restore-drill record:

1. Preflight captures context, namespace, pod readiness, PostgreSQL tool availability, current
   database/user names, source database size, local `/tmp` free space, PGDATA/PVC free space,
   public table count, and selected source row counts without printing Secret values.
2. `pg_dump -Fc` creates a local archive under `/tmp`.
3. Source row counts are checked again after the dump; abort if the selected counts changed.
4. The archive byte size and SHA-256 digest are recorded.
5. Space guards confirm local `/tmp` and PGDATA/PVC free space before restore.
6. A temporary database named with an OPS-006 timestamp is created.
7. `pg_restore --exit-on-error --single-transaction -d "$restore_db"` restores the archive into the
   temporary database. `--create` and `--clean` are prohibited for this drill.
8. Restored public table count and selected row counts match the stable source counts.
9. The temporary database and local archive are deleted.
10. Ledgers mark OPS-006 as evidenced while preserving OPS-007..009 and broader DR gaps.

## 8. Affected Domains

- OPS-006 PostgreSQL backup and restore drill
- GA problem and gap tracking
- Live database operations

## 9. Affected Files

- `docs/plan/2026-06-21-postgres-backup-restore-drill.md`
- `gap.md`
- `problem.md`

## 10. API / Contract Changes

None.

## 11. Database / Migration Changes

No schema or data changes to the live database. The drill creates and drops one temporary restore
database in the same PostgreSQL instance.

## 12. Configuration Changes

None.

## 13. Observability Changes

No permanent observability changes. Evidence will be collected from:

- `kubectl config current-context`
- `kubectl -n nexuspaas get deploy postgres`
- `kubectl -n nexuspaas exec deploy/postgres -- ...`
- local archive byte size and SHA-256 digest
- source database size and free-space guards
- source and restored table/row count comparisons

The full evidence table will be appended to this plan under `## 21. Implementation Evidence`.

## 14. Security Considerations

- Do not print or copy Secret values.
- Treat the local dump file as sensitive data.
- Store the dump only under `/tmp` for the duration of the drill.
- Delete the temporary database and local dump after validation.
- Do not commit backup artifacts.
- Do not use `kubectl cp`; `kubectl exec` streams the dump/restore payload and avoids tar
  assumptions.
- Do not use `pg_restore --create` or `pg_restore --clean`.

## 15. Implementation Steps

1. Capture preflight:
   - `kubectl config current-context`
   - namespace: `nexuspaas`
   - `postgres` deployment readiness and image
   - PostgreSQL tool availability inside `deploy/postgres`
   - current database/user names
   - source database size from `pg_database_size(current_database())`
   - PGDATA/PVC free bytes from `df -Pk /var/lib/postgresql/data`
   - local `/tmp` free bytes from `df -Pk /tmp`
   - source public table count
   - selected source row counts for `platform_records`, `platform_event_outbox`, `users`,
     `org_project_records`, and `workload_records`
   - abort if local `/tmp` free bytes are less than twice the source database size plus 128 MiB
   - abort if PGDATA/PVC free bytes are less than twice the source database size plus 128 MiB
2. Create a timestamped local dump path under `/tmp`, for example
   `/tmp/nexuspaas-ops006-YYYYMMDDHHMMSS.dump`.
3. Stream a custom-format dump to the local path:
   `kubectl -n nexuspaas exec deploy/postgres -- sh -lc 'pg_dump -Fc -U "$POSTGRES_USER" "$POSTGRES_DB"' > "$dump_path"`
4. Re-read selected source row counts immediately after `pg_dump`; abort if they changed from the
   pre-dump counts.
5. Record dump byte size and SHA-256 digest.
6. Re-check local `/tmp` and PGDATA/PVC free bytes; abort if local free bytes are less than the
   dump size plus 128 MiB or PGDATA/PVC free bytes are less than twice the dump size plus 128 MiB.
7. Create a timestamped temporary restore database.
8. Restore the archive into the temporary database via `kubectl exec -i`:
   `pg_restore --exit-on-error --single-transaction -U "$POSTGRES_USER" -d "$restore_db"`
   Do not pass `--create` or `--clean`.
9. Compare restored public table count and selected row counts against the stable source counts.
10. Drop the temporary restore database.
11. Delete the local dump file.
12. Update `gap.md` and `problem.md`: OPS-006 PostgreSQL restore drill is evidenced; OPS-007,
    OPS-008, OPS-009, PITR/off-cluster retention/encrypted backup storage/full DR remain open.
13. Submit the evidence and ledger updates to Reviewer Agent.

## 16. Verification Plan

- Preflight shows the selected context, namespace, ready `postgres` deployment, and available
  PostgreSQL tools.
- Preflight records source database size, local `/tmp` free bytes, and PGDATA/PVC free bytes.
- The drill aborts before restore if free-space guards are not satisfied.
- `pg_dump -Fc` exits successfully and produces a non-empty archive.
- Selected source row counts are stable immediately before and after `pg_dump`.
- SHA-256 digest is recorded without storing the dump in git.
- Temporary database creation succeeds.
- `pg_restore --exit-on-error --single-transaction -d "$restore_db"` exits successfully without
  `--create` or `--clean`.
- Restored table count equals source table count.
- Selected restored row counts equal source row counts.
- Temporary database is absent after cleanup.
- Local dump file is absent after cleanup.
- `git diff --check` passes after ledger edits.
- Not applicable for this docs/live-evidence-only slice because no application code, tests, build
  scripts, manifests, or runtime config are changed: `go -C backend test ./...`, `npm --prefix
  frontend run test`, `npm --prefix frontend run build`, and SonarScanner. If the scope expands
  beyond docs/evidence, these gates must run.

## 17. Rollback Plan

If dump, restore, or validation fails:

1. Stop the drill.
2. Drop the temporary restore database if it was created.
3. Delete the local dump file if it exists.
4. Capture the failing command stage and non-secret error output.
5. Do not retry with destructive live-database actions.

## 18. Risks and Tradeoffs

- `pg_dump` reads the live database and can add load, but the local database is small, the drill
  requires stable selected counts, and it avoids writes to the live `nexuspaas` database.
- Restoring into a temporary database consumes space on the same PostgreSQL PVC; preflight guards
  reduce but do not eliminate capacity risk.
- Restoring into the same PostgreSQL instance proves logical backup/restore mechanics, not
  off-cluster disaster recovery.
- Local `/tmp` storage proves a backup artifact can be produced and consumed, but not retention,
  encryption, rotation, or remote object-store durability.
- The drill covers OPS-006 only; Harbor, object storage, and secret recovery remain separate work.

## 19. Reviewer Checklist

- The drill never restores over the live database.
- The plan uses standard PostgreSQL backup/restore tools instead of custom backup code.
- The evidence contract is specific enough to verify OPS-006.
- Secret values and dump contents are not printed or committed.
- Ledger updates preserve OPS-007..009 and broader DR gaps.
- Free-space guards and stable source row-count checks run before restore.
- `pg_restore` uses `--exit-on-error --single-transaction -d "$restore_db"` and does not use
  `--create` or `--clean`.

## 20. Status

Status: Approved. Reviewer Agent approved the revised plan before live database commands ran.

## 21. Implementation Evidence

Executed on 2026-06-21 against `kubectl config current-context=default`, namespace
`nexuspaas`.

| Check | Evidence | Result |
|---|---|---|
| PostgreSQL deployment | image `postgres:16-alpine`; ready `1/1` | Pass |
| Tool availability | `pg_dump pg_restore createdb dropdb psql` present in `deploy/postgres` | Pass |
| Database/user | `nexuspaas|nexuspaas` | Pass |
| Source database size | `14408727` bytes | Pass |
| PGDATA/PVC free space before dump | `611863531520` bytes | Pass |
| Local `/tmp` free space before dump | `11948277760` bytes | Pass |
| Restore safety threshold | `163035182` bytes | Pass |
| Source public table count | `81` | Pass |
| Source selected counts stable | pre/post `pg_dump` counts matched | Pass |
| Dump artifact | local path `/tmp/nexuspaas-ops006-20260621150759.dump`; deleted after drill | Pass |
| Dump size | `353198` bytes | Pass |
| Dump SHA-256 | `6c0869c3e591e9768edceee55feb80cbdf1e61e2e67b162a0b9a8bf6424a1c71` | Pass |
| PGDATA/PVC free space before restore | `611863072768` bytes | Pass |
| Local `/tmp` free space before restore | `11947921408` bytes | Pass |
| Temporary restore database | `nexuspaas_restore_ops006_20260621150759` | Pass |
| Restore command semantics | `pg_restore --exit-on-error --single-transaction -U "$POSTGRES_USER" -d "$restore_db"`; no `--create`; no `--clean` | Pass |
| Restored public table count | `81` | Pass |
| Selected row counts | restored counts matched source counts | Pass |
| Temporary database cleanup | no `nexuspaas_restore_ops006_%` databases remained | Pass |
| Local dump cleanup | dump file absent after cleanup | Pass |

Selected source/restored counts:

| Table | Count |
|---|---:|
| `org_project_records` | 0 |
| `platform_event_outbox` | 1193 |
| `platform_records` | 387 |
| `users` | 0 |
| `workload_records` | 0 |

This closes live OPS-006 PostgreSQL logical backup/restore drill evidence only. OPS-007 Harbor
restore, OPS-008 object storage restore, OPS-009 secret recovery, PITR, off-cluster retention,
encrypted backup storage, and full DR remain open.
