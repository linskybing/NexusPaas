# Migration Runner Ledger

## 1. Objective

Move the first-release migration runner one step closer to deployable readiness
by keeping the existing embedded-SQL/pgx runner and adding the minimum audit and
safety controls it currently lacks: a platform-owned migration ledger,
per-migration version and checksum validation, a single PostgreSQL advisory
lock, and dirty-state blocking.

This closes only the first-release migration-runner maturity slice. It does not
claim full GA database migration maturity.

## 2. Background

`problem.md` still lists `Migration runner` as a P1 architecture maturity
blocker because SQL migration execution is too simple for GA auditing and
rollback. The current runner in `backend/internal/platform/migrate.go` applies
the embedded platform schema, discovers every known service `migrations/*.sql`
file, and executes all files on every run.

The smallest useful improvement is not a framework migration. Existing service
migrations are already idempotent and service-owned. The missing launch evidence
is a durable record of what ran, whether a file changed after it was recorded,
whether a previous run was left dirty, and whether two runner instances can
apply migrations concurrently.

## 3. Source References

- `problem.md` P1 Architecture Maturity, `Migration runner`
- `gap.md` rollback and release-blocker tracking
- `docs/acceptance/operations.md` (`OPS-003`, `OPS-020`)
- `docs/acceptance/ga-checklist.md` rollback and release-blocker expectations
- `backend/docs/beta-launch-readiness.md` migration and rollback standards
- `backend/docs/non-functional-requirements.md` (`NFR-OPER-01`,
  `NFR-OPER-03`)
- `docs/agents/planning.md`
- `backend/internal/platform/migrate.go`
- `backend/internal/platform/migrate_test.go`
- `backend/internal/platform/store_postgres_test.go`
- `backend/internal/platform/admin.go`
- `backend/internal/platform/schema.sql`
- `backend/*-service/migrations/*.sql`
- `backend/platform-gateway/migrations/*.sql`

## 4. Assumptions

- PostgreSQL remains the only database supported by `ApplyMigrations` for this
  release slice.
- Existing service migration files stay idempotent and backward-compatible.
- A first run against a database that predates the ledger may re-execute
  existing idempotent migrations once, then record them.
- `ADMIN_TASK=apply-migrations` is still the migration entrypoint.
- `ADMIN_TASK=validate-migrations` remains a filesystem validation task and
  does not require a database connection.
- No live staging or production database operation is authorized by this plan.

## 5. Non-Goals

- No `golang-migrate`, `goose`, Atlas, or other migration dependency unless the
  Reviewer rejects the stdlib/current-pgx approach.
- No live DB migration drill unless separately approved.
- No production DB operations.
- No broad typed-schema migration or data ownership rewrite.
- No provider abstraction or multi-database runner.
- No down-migration framework.
- No changes to existing service migration contents unless a test proves one is
  not idempotent.
- No claim that full GA migration maturity is complete.

## 6. Current Behavior

- `ApplyMigrations` opens a pgx pool with a 30-second context.
- It executes embedded `platformSchemaSQL`.
- It discovers known service migration files under deterministic roots.
- It sorts and executes every discovered `.sql` file on every run.
- `validateServiceMigrationsInRoots` requires at least one migration file per
  known service directory.
- The runner has no durable ledger, no version parsing, no checksum validation,
  no dirty-state detection, and no database-level migration lock.

## 7. Target Behavior

- The runner bootstraps a platform-owned ledger table before applying migration
  units.
- The runner acquires one PostgreSQL advisory lock for the duration of an
  `ApplyMigrations` run.
- Migration units include the embedded platform schema as `platform/schema.sql`
  and each service-owned `.sql` file.
- Service migration filenames must start with a numeric version such as
  `0001_init.sql`; duplicate versions within one service fail validation.
- Each ledger row records service, version, filename, SHA-256 checksum,
  dirty state, applied timestamp, and execution duration.
- A previously applied migration with the same checksum is skipped.
- A previously applied migration with a different checksum fails before running
  later migrations.
- Any dirty ledger row fails before running new migrations.
- A new migration is marked dirty and that marker is committed outside the SQL
  execution transaction before migration SQL starts.
- The dirty marker is cleared only after the migration SQL executes and the
  final ledger update succeeds.
- A failed migration leaves a visible dirty ledger row for operator review.
- Existing idempotent migrations remain compatible with first ledger adoption.

## 8. Affected Domains

- Platform runtime: migration runner and admin task behavior.
- Operations: deployment and rollback readiness evidence.
- Data ownership: service-owned migrations remain in each service directory.
- Security: no secrets are introduced; SQL remains trusted repository content.

No business service boundary or deployable-unit split changes are proposed.

## 9. Affected Files

Implementation is expected to touch only these files:

- `backend/internal/platform/migrate.go`
- `backend/internal/platform/migrate_test.go`
- `backend/internal/platform/store_postgres_test.go` or a new
  `backend/internal/platform/migrate_postgres_test.go`
- `backend/internal/platform/schema.sql` only if the implementer chooses to keep
  ledger bootstrap DDL beside the other platform-owned tables
- `problem.md`
- `gap.md`
- `docs/acceptance/gap-analysis.md`

`backend/internal/platform/admin.go` should not need a contract change; it is
listed as a source reference because it calls `ApplyMigrations` and
`validateServiceMigrations`.

## 10. API / Contract Changes

No public HTTP API changes.

Admin task behavior changes:

- `ADMIN_TASK=apply-migrations` becomes ledger-aware and may fail on dirty
  state, checksum mismatch, invalid migration filename, duplicate service
  version, or advisory-lock contention/timeout.
- `ADMIN_TASK=validate-migrations` additionally validates migration filename
  versions and duplicate versions, while staying database-free.

No CLI, OpenAPI, route, event, or owner-read contract changes are planned.

## 11. Database / Migration Changes

Add one platform-owned ledger table, bootstrapped by the runner before normal
migration execution:

```sql
CREATE TABLE IF NOT EXISTS platform_schema_migrations (
    service TEXT NOT NULL,
    version INTEGER NOT NULL,
    filename TEXT NOT NULL,
    checksum TEXT NOT NULL,
    dirty BOOLEAN NOT NULL DEFAULT false,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    duration_ms INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (service, version),
    CHECK (version >= 0),
    CHECK (duration_ms >= 0),
    CHECK (checksum <> ''),
    CHECK (filename <> '')
);
```

Use service `platform`, version `0`, filename `schema.sql` for the embedded
platform schema. Service-owned files use their directory name as `service` and
their numeric filename prefix as `version`.

No existing business table shape changes are part of this slice.

## 12. Configuration Changes

None.

Do not add new environment variables. Use one fixed advisory-lock key for the
database. One database should have one migration runner at a time.

## 13. Observability Changes

- Keep command output short and non-secret.
- Print a migration summary with applied, skipped, and total unit counts.
- Error messages should identify service, version, and filename for dirty state
  or checksum mismatch.
- Do not print SQL contents or database credentials.
- No Prometheus metric is required in this slice.

## 14. Security Considerations

- Use `crypto/sha256` from the Go standard library for checksums.
- Treat migration SQL as trusted repository content, not user input.
- Do not log database URLs, credentials, or SQL file contents.
- Fail closed on checksum mismatch and dirty rows.
- The advisory lock prevents concurrent runners from racing ledger and schema
  writes.

## 15. Implementation Steps

1. Add a small `migrationUnit` representation with service, version, filename,
   path, SQL body, and checksum.
2. Parse service migration versions from the leading numeric filename prefix and
   reject missing, invalid, negative, or duplicate versions per service.
3. Build the ordered migration unit list as platform schema first, then
   service-owned files in deterministic service/version order.
4. Add ledger bootstrap DDL for `platform_schema_migrations`.
5. Update `ApplyMigrations` to reserve one pgx connection, acquire a fixed
   PostgreSQL advisory lock with clean contention/timeout behavior, bootstrap
   the ledger, check for dirty rows, and apply/skip units through the ledger.
6. For a new unit, write or update its ledger row with `dirty=true` in its own
   committed operation before executing migration SQL. Do not wrap this dirty
   marker in the same transaction as the migration SQL.
7. Execute the migration SQL after the committed dirty marker exists, then clear
   `dirty=false` with the checksum and duration only after the SQL succeeds.
   If the SQL fails, leave the dirty row visible.
8. For an existing clean unit, skip execution when the checksum matches.
9. For an existing clean unit with a different checksum, return a checksum
   mismatch error without executing more units.
10. Keep `validateServiceMigrationsInRoots` database-free but extend it to catch
   bad filenames and duplicate versions.
11. Add focused unit tests for version parsing, ordering, duplicate detection,
    checksum mismatch handling, dirty-state blocking, and deterministic
    discovery.
12. Add PostgreSQL integration tests using `TEST_DATABASE_URL` that prove:
    dirty markers are committed before SQL execution and remain visible after an
    intentionally failing migration; an already-migrated database without
    `platform_schema_migrations` records ledger rows on first new-runner
    adoption and skips on the second run; and a runner fails cleanly when another
    session holds the fixed advisory lock.
13. Update `problem.md`, `gap.md`, and `docs/acceptance/gap-analysis.md` to mark
    only this migration-runner ledger slice as evidenced after Code Agent
    verification. Leave live DB drill, typed schema migrations, rollback drill,
    and full GA claims open.

## 16. Verification Plan

Run all commands from the repository root unless noted.

Focused unit tests:

```bash
cd backend && go test ./internal/platform -run 'Test.*Migration|TestValidateServiceMigrations' -count=1
```

Integration test when `TEST_DATABASE_URL` is available:

```bash
cd backend && TEST_DATABASE_URL="$TEST_DATABASE_URL" go test -tags=integration ./internal/platform -run 'Test.*Migration|TestPostgresStore' -count=1
```

Required integration coverage:

- Dirty-state persistence: create a temporary migration that fails after its
  dirty marker is written; assert the returned error and a visible
  `platform_schema_migrations.dirty=true` row after the runner exits.
- First ledger adoption: simulate a database whose schema is already present but
  `platform_schema_migrations` is absent; run the new runner once and assert
  ledger rows are recorded, then run it again and assert those rows are skipped
  rather than re-executed.
- Advisory lock contention: hold the fixed advisory lock from one PostgreSQL
  session, start a competing runner with a short context, and assert it fails or
  times out cleanly without applying migrations or leaving partial ledger rows.

Admin task validation:

```bash
cd backend && ADMIN_TASK=validate-migrations go run ./cmd/microservice
```

Admin task apply against a disposable or test database only:

```bash
cd backend && DATABASE_URL="$TEST_DATABASE_URL" ADMIN_TASK=apply-migrations go run ./cmd/microservice
```

Standard gates:

```bash
cd backend && go test ./... -count=1
cd backend && make check
cd backend && make ci-sonar
git diff --check
```

Optional Docker-backed evidence if Reviewer asks for it:

```bash
cd backend && make ci-docker
```

## 17. Rollback Plan

- If the implementation fails before deployment, revert the migration runner,
  tests, and tracker-doc updates.
- If the new runner created `platform_schema_migrations`, leaving the table in
  place is safe; the previous runner ignores it.
- If a dirty row blocks a test or staging database, inspect the named service,
  version, filename, and database state before manually clearing or deleting the
  exact ledger row. Do not automate that cleanup in application startup.
- If a newly applied service migration introduced an incompatible schema change,
  rollback follows the existing release standard: restore a known-good database
  backup or apply a reviewed forward fix. This plan does not introduce down
  migrations.
- Because service SQL must remain backward-compatible and idempotent, normal app
  rollback is still image/config rollback.

## 18. Risks and Tradeoffs

- First adoption on an existing database re-runs existing idempotent migrations
  once so the ledger can be populated.
- A dirty row may require manual operator action even when PostgreSQL rolled back
  the failed SQL transaction. This is intentional; silent retry is riskier for
  launch evidence.
- Dirty marking must be committed before migration SQL starts; otherwise a
  failed SQL transaction can erase the evidence the ledger is meant to preserve.
- A fixed advisory lock is PostgreSQL-specific, matching the current runner.
- This does not solve full schema-change rollback, expand/contract governance,
  live staging rehearsal, or typed data ownership.
- Avoiding `golang-migrate`, `goose`, or Atlas keeps the diff small but leaves
  advanced migration features for a later decision.

## 19. Reviewer Checklist

- Plan has exactly the required 20 sections and `Status: Draft`.
- No production code was changed by Plan Agent.
- Scope stays inside the existing runner and does not introduce a migration
  framework dependency.
- Ledger schema records service, version, filename, checksum, dirty state,
  timestamp, and duration.
- `ApplyMigrations` uses a database lock and fails closed on dirty or checksum
  mismatch.
- Dirty markers are committed outside the SQL execution transaction, and tests
  prove failed migration SQL leaves a dirty row.
- First ledger adoption is tested against an already-migrated database without a
  ledger, followed by a second run that skips.
- Advisory lock contention is tested by holding the fixed lock from another
  PostgreSQL session.
- `validate-migrations` remains database-free.
- Existing service migrations stay idempotent and compatible.
- Focused unit tests and PostgreSQL integration evidence are specified.
- Rollback is realistic and does not pretend down migrations exist.
- Tracker docs only claim this ledger/lock/checksum/dirty slice, not full GA.

## 20. Status

Status: Approved
