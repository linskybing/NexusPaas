# Request-Notification Typed Data Ownership

> **Slice 2 (this file also covers the follow-up):** the same pattern was
> extended to the service's remaining owned-write aggregates — `announcements`,
> `announcement_reads`, and `notifications` — via migration
> `request-notification-service/migrations/0003_request_notification_typed_messaging.sql`
> and additional specs/builders/tests in
> `internal/platform/store_postgres_requestnotification.go` (+ test). This retires
> generic `platform_records` storage for all of request-notification's owned
> writes. `project_access_*` stays on the generic store (read-model projection).

## 1. Objective

Close the first slice of the **Typed domain data ownership (P0)** blocker in
`problem.md` by moving the request-notification `forms` aggregate (`forms` and
`form_messages`) off the generic `platform_records` / JSONB store and onto
service-owned, typed Postgres tables, behind the same store port the handlers
already use.

## 2. Background

`backend/docs/migration-roadmap.md` requires core domains to move from generic
`platform_records` JSONB storage to typed, service-owned schemas slice by slice.
Identity is the only domain that has started: `store_postgres_identity.go` maps
identity resources to typed tables (column promotion + full-record `payload`
JSONB), dispatched per `PostgresStore` method via `identityPostgresResourceFor`,
and `identity-service/migrations/0002_identity_owned_records.sql` creates/backfills
those tables while retaining legacy rows for rollback.

Request-notification is the lowest-coupling genuine owned-write domain (roadmap
Phase 1). Its `forms` records are created/updated/batch-transitioned through
`app.CreateRecordWithEvent` / `app.UpdateRecordWithEvent` keyed by
`request-notification-service:forms`, and `form_messages` through `store.Create`.
Today both land in the generic `request_notification_records` table.

(audit-compliance was rejected as the first slice: its `audit_logs` are derived
from the AuditEvent outbox, not written to a store, so it is a projection.)

## 3. Source References

- `backend/internal/platform/store_postgres.go`
- `backend/internal/platform/store_postgres_identity.go`
- `backend/internal/services/requestnotification/handler.go`
- `backend/internal/services/requestnotification/helpers.go`
- `backend/identity-service/migrations/0002_identity_owned_records.sql`

## 4. Approach

Typed ownership is implemented at the **store layer**, not the service package:
services only hold the `RecordStore` port, so the typed table routing must live
in `PostgresStore`. The handlers stay unchanged — they already address records by
resource key, and the store routes transparently. This mirrors identity exactly.

1. **Migration** `request-notification-service/migrations/0002_request_notification_typed_forms.sql`
   creates typed `forms` and `form_messages` tables (promoted filterable columns
   + full-record `payload` JSONB + `version`/timestamps), adds indexes, and
   idempotently backfills from `platform_records`. Legacy rows are retained
   (expand + dual-write phase; cutover/legacy-drop deferred).
2. **Store spec** `internal/platform/store_postgres_requestnotification.go` maps
   the two resources to their tables with insert/update column builders, reusing
   the shared `identityPostgresResource` typed-table descriptor and column
   helpers.
3. **Open/Closed dispatch** `internal/platform/store_postgres_typed.go` adds
   `typedPostgresResourceFor`, which unions identity + request-notification
   specs; `store_postgres.go` now dispatches through it at all 13 sites instead
   of `identityPostgresResourceFor`. New typed domains register a spec file and
   are picked up without editing every store method.

## 5. Non-Goals

- No handler / API / event-shape changes; the `/api/v1/forms*`,
  `/api/v1/announcements*`, and `/api/v1/notifications*` JSON is identical.
- No cutover: legacy `platform_records` rows are not dropped and dual-write is
  not removed (later slice, after drift comparison — roadmap Phase 6).
- The `project_access_*` read-model projections stay on the generic store, and
  other domains (workload, orgproject, storage, imageregistry, scheduler/quota)
  are separate slices. (The other request-notification aggregates —
  `announcements`, `announcement_reads`, `notifications` — were completed in
  slice 2; see the note at the top of this doc.)

## 6. Current Behavior

`forms` / `form_messages` records persist as generic rows in
`request_notification_records` (JSONB). Reads scan all rows for the resource and
reconstruct from `payload`.

## 7. Target Behavior

`PostgresStore` routes `request-notification-service:forms` and `…:form_messages`
to the owned `forms` / `form_messages` tables for create/get/list/update/delete/
NextID and the transactional `*WithEvent` / `RunInTx` paths, with promoted columns
for ownership/indexing. The full record still round-trips through `payload`, so
reads are byte-for-byte equivalent. The in-memory store and handlers are
unaffected.

## 8. Affected Files

- `backend/request-notification-service/migrations/0002_request_notification_typed_forms.sql` (new)
- `backend/internal/platform/store_postgres_requestnotification.go` (new)
- `backend/internal/platform/store_postgres_typed.go` (new)
- `backend/internal/platform/store_postgres_requestnotification_test.go` (new)
- `backend/internal/platform/store_postgres.go` (dispatch lookup)
- `backend/docs/migration-roadmap.md` (status line)

## 9. API / Contract Changes

None.

## 10. Database / Migration Changes

Additive, idempotent (`IF NOT EXISTS`, `ON CONFLICT DO NOTHING`). New `forms` and
`form_messages` tables + indexes + backfill from `platform_records`. Reversible:
legacy rows retained; dropping the new tables restores prior behavior.

## 11. Tests

- `store_postgres_requestnotification_test.go`: routing of create/list/update/
  delete and form_messages to the owned tables (not `platform_records`), the
  union lookup `typedPostgresResourceFor`, and the column builders (alias keys,
  defaulted status, nullable `project_id`, present-only update columns).
- Existing `internal/services/requestnotification` and `internal/platform`
  suites continue to pass unchanged (handlers and in-memory store untouched).

## 12. Verification

From `backend/`: `go vet ./...`, `go build ./...`,
`go test -race ./internal/platform/... ./internal/services/requestnotification/... -count=1`,
full `go test ./... -count=1`, and a focused coverage report on the new files.
SonarScanner Quality Gate when `SONAR_TOKEN`/`SONAR_HOST_URL` are present.
