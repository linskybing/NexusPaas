# Migration Runner PostgreSQL Integration Evidence

## 1. Objective

Capture the missing PostgreSQL integration execution evidence for the existing
migration runner ledger/checksum/dirty-state/advisory-lock slice without
changing product code.

The Code Agent must run the already-present integration tests against the live
cluster PostgreSQL instance through a safe local connection, then update only
tracker documentation to say that PostgreSQL integration execution evidence for
migration runner dirty/checksum/adoption/advisory-lock behavior passed.

## 2. Background

`docs/plan/2026-06-22-migration-runner-ledger.md` already implemented and
reviewed the first-release migration runner code slice. The remaining tracker
gap is execution evidence for `backend/internal/platform/migrate_postgres_test.go`
with `TEST_DATABASE_URL`.

The live `nexuspaas` namespace now has a ready PostgreSQL deployment/service.
The integration tests create an isolated schema named
`nexus_migrate_<timestamp>` in the selected database, set `search_path` to that
schema, and register cleanup that runs `DROP SCHEMA IF EXISTS ... CASCADE`.

## 3. Source References

- `AGENTS.md`
- `docs/agents/planning.md`
- `docs/agents/coding-guidelines.md`
- `docs/agents/review-checklist.md`
- `docs/plan/2026-06-22-migration-runner-ledger.md`
- `backend/internal/platform/migrate_postgres_test.go`
- `problem.md` migration runner row
- `gap.md` service identity / JWT-JWKS lib / migration-runner maturity row
- `docs/acceptance/gap-analysis.md` OPS / migrations paragraph and
  Strengthen-existing row
- `backend/deploy/k3s/postgres.yaml`
- `backend/deploy/k3s/production-beta/runtime-secret-contract.yaml`

## 4. Assumptions

- The current Kubernetes context intentionally targets the live cluster used for
  this evidence.
- Namespace `nexuspaas` exists and `deployment/postgres` is ready.
- Service `postgres` exposes PostgreSQL on port `5432`.
- Secret `platform-gateway-runtime-secret` exists in namespace `nexuspaas` and
  contains key `DATABASE_URL`.
- The local machine can run `kubectl port-forward` and `go test`.
- The integration tests remain non-destructive outside their temporary
  `nexus_migrate_<timestamp>` schemas.
- The Reviewer approves using live cluster PostgreSQL for this limited evidence
  run.

## 5. Non-Goals

- No production code changes.
- No migration runner code changes.
- No SQL migration file changes.
- No Kubernetes manifest changes.
- No Secret mutation.
- No database backup/restore drill.
- No live staging migration drill.
- No schema rollback drill.
- No down-migration framework.
- No full migration-runner GA claim.
- No Full GA or first-version completion claim.

## 6. Current Behavior

Trackers say the migration runner ledger/checksum/advisory-lock/dirty-state code
slice is implemented and locally verified, but PostgreSQL integration execution
is still pending `TEST_DATABASE_URL`.

`backend/internal/platform/migrate_postgres_test.go` is guarded by
`//go:build integration`, reads `TEST_DATABASE_URL`, creates a temporary schema,
and covers:

- `TestMigrationDirtyMarkerPersistsAfterFailedSQL`
- `TestMigrationLedgerAdoptsPreLedgerDatabaseThenSkips`
- `TestMigrationChecksumMismatchStopsBeforeLaterUnits`
- `TestMigrationAdvisoryLockContentionFailsCleanly`

## 7. Target Behavior

After Code Agent execution and Reviewer approval:

- The exact integration tests above have passed against the live cluster
  PostgreSQL backing service through a safe derived `TEST_DATABASE_URL`.
- The only documented credential source is the redacted source label
  `platform-gateway-runtime-secret:DATABASE_URL`.
- Tracker docs state that PostgreSQL integration execution evidence passed for
  dirty-state persistence, first ledger adoption/skip behavior, checksum
  mismatch blocking, and advisory-lock contention.
- Tracker docs continue to say live staging migration drill, schema rollback,
  full migration-runner GA, Full GA, and first-version completion remain open.

## 8. Affected Domains

- Platform runtime evidence: migration runner PostgreSQL integration tests.
- Operations readiness tracking: tracker documentation only.
- Security operations: secret handling while deriving a local test database URL.

No service boundary, API, runtime, or deployable-unit ownership changes are
planned.

## 9. Affected Files

Plan Agent may create only:

- `docs/plan/2026-06-22-migration-runner-postgres-integration-evidence.md`

If this plan is approved, Code Agent may update only:

- `docs/plan/2026-06-22-migration-runner-postgres-integration-evidence.md`
- `gap.md`
- `problem.md`
- `docs/acceptance/gap-analysis.md`

`docs/acceptance/README.md` is not expected to change because the migration
runner pending-evidence wording is in `docs/acceptance/gap-analysis.md`, not the
README. Add it only if Reviewer identifies a directly stale migration-runner
status there.

## 10. API / Contract Changes

None.

No HTTP API, OpenAPI, event, CLI, admin-task, service-to-service, or Kubernetes
contract changes are part of this slice.

## 11. Database / Migration Changes

No database schema changes are planned by this slice.

The integration tests may create temporary schemas named
`nexus_migrate_<timestamp>` in the selected database. The tests are expected to
drop those schemas during `t.Cleanup` with:

```sql
DROP SCHEMA IF EXISTS <temporary_schema> CASCADE;
```

Do not manually drop broad schemas or tables. If a post-test leftover exists,
record the blocker and inspect only exact `nexus_migrate_%` test schemas before
any cleanup is proposed.

## 12. Configuration Changes

None.

The Code Agent may read existing Kubernetes Secret data into local shell
variables for the test process only. No environment variable, ConfigMap, Secret,
manifest, or application config file is added or changed.

## 13. Observability Changes

No product logging, metrics, or traces are changed.

Evidence recorded in docs must be limited to:

- Redacted credential source:
  `platform-gateway-runtime-secret:DATABASE_URL`
- Port-forward target:
  `nexuspaas/service/postgres:5432`
- Test command shape with `<redacted-derived-url>`, not the actual URL
- Test names
- Pass/fail result and relevant non-secret timings/counts
- Optional temporary-schema leftover count, if checked

Do not paste raw command output into docs until it has been reviewed for
secrets.

## 14. Security Considerations

Secret-safety rules are mandatory:

- Do not echo `DATABASE_URL`.
- Do not write `DATABASE_URL` or a derived URL to any tracked file.
- Do not write `DATABASE_URL` or a derived URL to temporary evidence files.
- Do not run with `set -x`.
- Do not use `tee` for commands that may contain environment values.
- Do not paste raw `kubectl get secret -o yaml/json` output into docs.
- Do not print hashes of the secret value; hashes still create unnecessary
  tracking material.
- Only report the redacted source label
  `platform-gateway-runtime-secret:DATABASE_URL`.
- If a failed command emits a URL or credential-like value, redact it before
  sharing or documenting.
- Kill the port-forward after the test and remove any local port-forward log.

## 15. Implementation Steps

1. Confirm the plan is Reviewer-approved before touching tracker docs or live
   cluster resources.
2. Preflight without secrets:

   ```bash
   kubectl config current-context
   kubectl -n nexuspaas get deploy postgres
   kubectl -n nexuspaas get svc postgres
   kubectl -n nexuspaas get secret platform-gateway-runtime-secret \
     -o jsonpath='{.data.DATABASE_URL}' >/dev/null
   ```

3. Start a local port-forward in the background and register cleanup:

   ```bash
   PF_PORT=15432
   kubectl -n nexuspaas port-forward svc/postgres "${PF_PORT}:5432" \
     >/tmp/nexuspaas-migration-runner-postgres-port-forward.log 2>&1 &
   PF_PID=$!
   trap 'kill "$PF_PID" 2>/dev/null || true; wait "$PF_PID" 2>/dev/null || true; rm -f /tmp/nexuspaas-migration-runner-postgres-port-forward.log' EXIT
   for i in $(seq 1 40); do
     (:</dev/tcp/127.0.0.1/${PF_PORT}) 2>/dev/null && break
     sleep 0.5
   done
   ```

4. Read the Secret into a local variable without printing it, then derive a
   loopback test URL for the port-forward:

   ```bash
   DATABASE_URL="$(
     kubectl -n nexuspaas get secret platform-gateway-runtime-secret \
       -o jsonpath='{.data.DATABASE_URL}' | base64 -d
   )"
   TEST_DATABASE_URL="$(
     printf '%s' "$DATABASE_URL" |
       sed -E "s#@[^/?]+(:[0-9]+)?/#@127.0.0.1:${PF_PORT}/#"
   )"
   unset DATABASE_URL
   ```

   Do not print either variable.

5. Run exactly the focused integration command from `backend`:

   ```bash
   cd backend && TEST_DATABASE_URL=<redacted-derived-url> go test -tags integration ./internal/platform -run 'TestMigration(DirtyMarkerPersistsAfterFailedSQL|LedgerAdoptsPreLedgerDatabaseThenSkips|ChecksumMismatchStopsBeforeLaterUnits|AdvisoryLockContentionFailsCleanly)' -count=1
   ```

   In the actual shell, pass the local variable instead of the redacted
   placeholder:

   ```bash
   cd backend && TEST_DATABASE_URL="$TEST_DATABASE_URL" go test -tags integration ./internal/platform -run 'TestMigration(DirtyMarkerPersistsAfterFailedSQL|LedgerAdoptsPreLedgerDatabaseThenSkips|ChecksumMismatchStopsBeforeLaterUnits|AdvisoryLockContentionFailsCleanly)' -count=1
   ```

6. Optional non-secret cleanup check, if `psql` is available inside
   `deployment/postgres`:

   ```bash
   kubectl -n nexuspaas exec deploy/postgres -- sh -lc \
     'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -tAc "select count(*) from information_schema.schemata where schema_name like '\''nexus_migrate_%'\'';"'
   ```

   Record only the count. Do not print credentials.

7. Stop the port-forward through the `trap`, or manually run:

   ```bash
   kill "$PF_PID" 2>/dev/null || true
   wait "$PF_PID" 2>/dev/null || true
   rm -f /tmp/nexuspaas-migration-runner-postgres-port-forward.log
   ```

8. Update tracker wording in `problem.md`, `gap.md`, and
   `docs/acceptance/gap-analysis.md` to replace the pending
   `TEST_DATABASE_URL` note with passed PostgreSQL integration execution
   evidence for only the four migration runner behaviors.
9. Update this plan status/evidence section with the command result, redacted
   source label, and any blocker. Do not include the secret or derived URL.
10. Request Reviewer Agent verification.

## 16. Verification Plan

Required focused integration evidence:

```bash
cd backend && TEST_DATABASE_URL=<redacted-derived-url> go test -tags integration ./internal/platform -run 'TestMigration(DirtyMarkerPersistsAfterFailedSQL|LedgerAdoptsPreLedgerDatabaseThenSkips|ChecksumMismatchStopsBeforeLaterUnits|AdvisoryLockContentionFailsCleanly)' -count=1
```

Required repository hygiene:

```bash
git diff --check
```

Required backend gates, even though the final diff should be documentation-only:

```bash
make -C backend check
make -C backend ci-sonar
```

Reviewer should verify:

- The integration command passed.
- No secret or derived URL appears in docs or git diff.
- Tracker wording claims only PostgreSQL integration execution evidence for the
  four named migration runner behaviors.
- Tracker wording does not claim live staging migration drill, schema rollback,
  full migration-runner GA, Full GA, or first-version completion.

## 17. Rollback Plan

Because this is an execution-evidence and docs-only slice:

- Stop the port-forward and remove its local log.
- If tests fail before tracker edits, leave trackers unchanged and record the
  blocker in this plan.
- If tracker edits are made incorrectly, revert only the tracker lines and plan
  evidence lines from this slice.
- If a temporary test schema is left behind, do not run broad cleanup. Inspect
  the exact `nexus_migrate_%` schema state and request Reviewer/operator
  approval before any manual drop.
- No application rollback or deployment rollback is required because no product
  code, manifest, Secret, or migration is changed.

## 18. Risks and Tradeoffs

- The test uses live cluster PostgreSQL, so secret handling and cleanup must be
  stricter than local Docker-backed runs.
- The tests are designed to isolate themselves with temporary schemas, but a
  failed cleanup could leave a `nexus_migrate_%` schema requiring manual review.
- A port-forward is simpler and safer than exposing PostgreSQL externally, but
  it depends on local `kubectl` connectivity.
- Running `make -C backend check` and `make -C backend ci-sonar` for a docs-only
  diff costs time but keeps the existing release gate discipline intact.
- This evidence proves the runner behavior against PostgreSQL. It still does not
  prove a full staging migration drill, rollback drill, expand/contract
  migration governance, or all-service typed schema maturity.

## 19. Reviewer Checklist

- [ ] Plan has exactly the required 20 sections.
- [ ] Scope is limited to plan/tracker docs and live test execution evidence.
- [ ] No product code, migration SQL, manifests, or Secrets are changed.
- [ ] Secret-safety rules are explicit and sufficient.
- [ ] Port-forward lifecycle includes startup, readiness wait, cleanup, and log
      removal.
- [ ] Integration command targets only the four requested migration tests.
- [ ] Temporary schema cleanup expectations are documented.
- [ ] Verification includes integration `go test`, `git diff --check`,
      `make -C backend check`, and `make -C backend ci-sonar`.
- [ ] Tracker update wording avoids overclaiming live staging migration drill,
      schema rollback, full migration-runner GA, Full GA, or first-version
      completion.
- [ ] Rollback is realistic for a docs-only evidence slice.

## 20. Status

Status: Blocked during Code Agent execution.

Reviewer approval was received before live execution. Preflight passed for
current Kubernetes context `default`, namespace `nexuspaas`,
`deployment/postgres` ready, `service/postgres:5432` present, and redacted
credential source `platform-gateway-runtime-secret:DATABASE_URL` readable.

The required focused integration command was executed through local
port-forward target `nexuspaas/service/postgres:5432` with an unprinted
secret-derived `TEST_DATABASE_URL`:

```bash
cd backend && TEST_DATABASE_URL=<redacted-derived-url> go test -tags integration ./internal/platform -run 'TestMigration(DirtyMarkerPersistsAfterFailedSQL|LedgerAdoptsPreLedgerDatabaseThenSkips|ChecksumMismatchStopsBeforeLaterUnits|AdvisoryLockContentionFailsCleanly)' -count=1
```

Result: failed. The observed failures indicate the PostgreSQL integration test
helper did not isolate the runner into the generated `nexus_migrate_%` schema in
this execution; later tests saw the previous dirty migration state and test
tables in the default schema:

- `TestMigrationLedgerAdoptsPreLedgerDatabaseThenSkips`: ledger table existed
  before runner.
- `TestMigrationChecksumMismatchStopsBeforeLaterUnits`: initial apply was
  blocked by dirty migration state from `identity-service` version `2`,
  `0002_fail.sql`.
- `TestMigrationAdvisoryLockContentionFailsCleanly`: ledger table existed before
  runner.

No tracker docs were updated to passed. Exact synthetic artifacts created by the
failed test attempt were cleaned from the live database:

- synthetic dirty ledger rows remaining for `identity-service` version `2`,
  `0002_fail.sql`: `0`
- synthetic public migration test tables remaining: `0`
- leftover `nexus_migrate_%` schema count: `0`

The port-forward was stopped and
`/tmp/nexuspaas-migration-runner-postgres-port-forward.log` was removed.

Next required action: revise the integration test isolation approach under a
separate Reviewer-approved plan, then rerun this evidence slice before changing
`gap.md`, `problem.md`, or `docs/acceptance/gap-analysis.md` to passed.
