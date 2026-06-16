# Identity Data Boundary First Slice

## 1. Objective

Move `identity-service` durable data off the generic `platform_records` JSONB table as the first data-boundary implementation slice, while preserving current API behavior and the modular-monolith runtime.

## 2. Background

The architecture review identifies `platform_records` as the highest-priority blocker to real microservice data ownership. The repository already has identity service migrations and internal identity read/auth contracts, but runtime persistence still goes through `platform.RecordStore` and `PostgresStore`.

## 3. Source References

- `backend/internal/platform/store_postgres.go`
- `backend/internal/platform/schema.sql`
- `backend/identity-service/migrations/0001_init.sql`
- `backend/internal/services/identity/principal_repository.go`
- `backend/internal/services/identity/auth_repository.go`
- `backend/internal/services/identity/internal_read_contracts.go`
- `backend/docs/migration-roadmap.md`
- `backend/docs/non-functional-requirements.md`

## 4. Assumptions

- This is the first slice only; full removal of `platform_records` across all services is out of scope.
- In-memory `platform.Store` remains unchanged for no-DB local tests.
- Existing `/api/v1/*` and `/internal/identity/*` response shapes remain compatible.
- Existing legacy `platform_records` rows must remain available for rollback.

## 5. Non-Goals

- No service runtime extraction.
- No service mesh, mTLS, JWT/OIDC library replacement, or Prometheus rewrite.
- No broad refactor of all 15 services.
- No deletion of `platform_records`.

## 6. Current Behavior

With `DATABASE_URL`, `NewBackingResources` injects `PostgresStore`, and all resources, including `identity-service:users`, sessions, API tokens, and roles, persist in `platform_records`.

## 7. Target Behavior

With `DATABASE_URL`, identity-owned resources persist in identity-owned tables; unmigrated resources continue using `platform_records`. External and internal identity contracts continue to work without frontend changes. The implementation remains a transitional `PostgresStore` adapter only; identity service handlers and repositories are not rewritten in this slice.

## 8. Affected Domains

- `identity-service`: users, roles, sessions, refresh tokens, API tokens, captchas, login failures.
- `platform`: transitional persistence adapter and migration runner only.

## 9. Affected Files

- Add `backend/identity-service/migrations/0002_identity_owned_records.sql`.
- Update `backend/internal/platform/store_postgres.go` or a new adjacent file for identity resource routing.
- Update/add focused tests under `backend/internal/platform` and `backend/internal/services/identity`.
- Update `backend/docs/migration-roadmap.md` only if implementation needs to document this as Phase 0 progress.

## 10. API / Contract Changes

No public API changes. Preserve:

- `/api/v1/login`, `/logout`, `/refresh`, `/me/api-tokens`, `/users`
- `/internal/identity/users`, `/internal/identity/users/{id}`, `/internal/identity/roles`, `/internal/identity/auth/session`, `/internal/identity/auth/api-token`

## 11. Database / Migration Changes

Add an idempotent `0002_identity_owned_records.sql` migration with this exact mapping:

| Resource | Destination table | Primary key | Unique constraints | Notes |
| --- | --- | --- | --- | --- |
| `identity-service:users` | `users` | `id TEXT` | existing `username UNIQUE` | Existing table; add `payload JSONB`, `version INTEGER`, preserve existing `updated_at`. |
| `identity-service:roles` | `identity_roles` | `id TEXT` | `name UNIQUE` | New table; stores role/admin capability fields in `payload`. |
| `identity-service:sessions` | `sessions` | `id TEXT` | existing `token UNIQUE` | Existing table; add `payload`, `version`, `updated_at`; `id` and `token` both use the record ID/token. |
| `identity-service:refresh_tokens` | `refresh_tokens` | `id TEXT` | existing `token UNIQUE` | Existing table; add `payload`, `version`, `updated_at`; `id` and `token` both use the record ID/token. |
| `identity-service:api_tokens` | `user_api_tokens` | `id TEXT` | none new | Existing table; add `payload`, `version`, `updated_at`; never persist raw token values. |
| `identity-service:captchas` | `captchas` | `id TEXT` | none new | Existing table; add `payload`, `version`, `updated_at`. |
| `identity-service:login_failures` | `login_failures` | `id TEXT` | existing `idx_login_failures_username_ip` stays an index, not unique | Existing table; add `payload`, `version`, preserve existing `updated_at`. |

Migration details:

- Add only missing columns with `ALTER TABLE ... ADD COLUMN IF NOT EXISTS`.
- Backfill from `platform_records` into the destination tables with `ON CONFLICT DO NOTHING`.
- For missing typed columns, derive values from `payload` with current handler keys and safe defaults used by `0001_init.sql`.
- Leave `platform_records` rows untouched for rollback.
- Leave existing `identity_records` rows untouched. That table is not the runtime source for these resources today and is not used as a destination in this slice.

## 12. Configuration Changes

No new runtime config. `DATABASE_URL` remains the switch for Postgres-backed persistence.

## 13. Observability Changes

No new metrics. Add structured warning logs only for identity table routing/backfill failures that would otherwise silently fall back or skip data.

## 14. Security Considerations

- Preserve hash-only API token storage.
- Do not log raw tokens, password hashes, captcha answers, or LDAP credentials.
- Keep internal identity auth endpoints protected by `X-Service-Key` as today; stronger service identity is a later plan.

## 15. Implementation Steps

1. Add the `0002` identity migration exactly as described in section 11.
2. Add a small resource router to `PostgresStore`: identity resources use identity-owned table operations; all other resources keep the current `platform_records` operations.
3. Preserve `contracts.Record[map[string]any]` semantics: `ID`, merged `Data`, `Version`, `CreatedAt`, and `UpdatedAt` must be reconstructed from typed columns plus `payload`.
4. Implement table-specific CRUD only in the platform Postgres adapter. Do not rewrite identity handlers, public routes, or the in-memory store.
5. Preserve unknown fields by merging typed columns and `payload` on read, and by storing the full map in `payload` on create/update.
6. Map duplicate primary-key and unique-constraint errors to `CreateConflictError` for identity resources.
7. Implement `NextID` for identity resources using the destination table IDs plus the existing `platform_id_seq` high-water mark; non-identity resources keep the current implementation.
8. Add focused tests for identity routing and existing identity repository behavior.
9. Extend source guards only if new raw identity literals are introduced outside the approved platform adapter or identity repositories.

Acceptance criteria per resource:

- `Create`, `Get`, `List`, `Update`, and `Delete` work for every mapped identity resource.
- New writes for mapped identity resources do not insert `platform_records` rows.
- `identity-service:api_tokens` stores `token_hash` and metadata, never a raw token.
- `identity-service:users` and `identity-service:roles` retain admin/capability fields through `payload`.
- Non-identity resources still use `platform_records`.

## 16. Verification Plan

Required focused test cases:

- Migration discovery still includes `identity-service/migrations/0002_identity_owned_records.sql`.
- Identity CRUD through `PostgresStore` writes destination tables and leaves `platform_records` empty for those resources.
- Non-identity CRUD through `PostgresStore` still writes `platform_records`.
- Identity read/list/update/delete parity preserves `Record` fields and unknown payload fields.
- Identity `NextID` scans destination table IDs and does not reuse deleted high-water IDs.
- Duplicate identity create returns `CreateConflictError`.
- Migration backfills representative legacy `platform_records` rows into identity destination tables without deleting legacy rows.
- Existing identity repository tests still pass unchanged.

Run from `backend`:

- `go test ./internal/platform ./internal/services/identity`
- `go test ./internal/services/...`
- `go test ./...`
- With Postgres available: `TEST_DATABASE_URL=<postgres-url> go test ./internal/platform ./internal/services/identity ./internal/e2e -run 'Identity|CrossService'`
- `ADMIN_TASK=validate-migrations DATABASE_URL=<postgres-url> go run ./cmd/microservice`
- Run SonarScanner Quality Gate if credentials/config are available; otherwise record as Not Run with reason.

## 17. Rollback Plan

The migration is additive and legacy rows are retained, but post-cutover identity writes live only in identity-owned tables. Rollback therefore requires one of these explicit actions before reverting routing:

- Preferred: run a copy-back SQL script from identity destination tables into `platform_records` using `ON CONFLICT (resource, id) DO UPDATE`, then revert the routing change.
- Emergency: revert routing without copy-back only if losing post-cutover identity writes is explicitly accepted for that environment.

Do not drop identity-owned tables during rollback.

## 18. Risks and Tradeoffs

- Existing duplicate usernames or token rows may conflict with destination table constraints during backfill; the migration uses `ON CONFLICT DO NOTHING` and preserves legacy rows for manual reconciliation.
- This keeps a transitional platform-level adapter, so it is not the final clean architecture.
- It intentionally prioritizes a safe data cutover over service runtime extraction.
- Rollback is operationally heavier than before because post-cutover writes require copy-back to restore the legacy path.

## 19. Reviewer Checklist

- Requirement fit: first report recommendation is addressed.
- Scope: limited to identity durable data and contracts.
- Data ownership: identity resources no longer write new durable rows to `platform_records`.
- API compatibility: existing public/internal identity tests pass.
- Rollback: additive migration and legacy rows retained.
- SOLID/12-Factor: no new shared mutable database ownership beyond the explicit transitional adapter.

## 20. Status

Status: Approved
