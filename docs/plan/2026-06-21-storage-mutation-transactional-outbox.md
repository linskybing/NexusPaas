# Storage Mutation Transactional Outbox Slice

Status: Implemented and Reviewer-verified

Plan Agent: Codex

Reviewer: Boole approved the plan after revisions for the separate optional
upsert interface, `batchUserStorageCommand` per-item inheritance, and explicit
`helpers.go` scope.

Final review: Boole PASS on 2026-06-21 after the missing-delete no-event
regression coverage was added for storage and project permission deletes.

Implementation evidence (2026-06-21):

- Added exported platform event-builder aliases plus a separate optional
  transactional upsert interface and `App.UpsertRecordWithEvent`.
- Added `PostgresStore.UpsertWithEvent`, committing upsert owner writes and
  outbox inserts in one DB transaction.
- Added storage repository event-aware upsert/update/delete helpers that keep
  storage-owned resource constants inside `storage_repository.go`.
- Updated storage non-batch upsert/update/delete handlers to use event-aware
  repository helpers; `batchUserStorageCommand` inherits per-item user-storage
  coupling through `userStorageCommand`.
- Added focused platform and storage transactional mutation tests.
- Verification passed:
  - `go -C backend test ./internal/platform -run 'UpsertRecordWithEvent|CreateRecordWithEvent|WithTx' -count=1`
  - `go -C backend test ./internal/services/storage -run 'Transactional|Storage.*Workflow|ProjectBindings|DeletionBatch' -count=1`
  - `go -C backend test ./internal/platform`
  - `go -C backend test ./internal/services/storage`
  - `go -C backend test ./...`
  - `bash backend/scripts/ci-security-gate.sh quick`

## Objective

Reduce the `Transactional outbox/inbox` P0 blocker by coupling storage-service
non-batch mutation writes with their domain events through the existing
transactional outbox primitives.

This closes the open storage upsert class and the remaining non-batch storage
single-record update/delete dual-write paths. It does not claim the whole
outbox blocker is done because batch per-item coupling remains open.

## Scope

- Add a platform-level `App.UpsertRecordWithEvent` helper, with optional
  Postgres-backed transactional support and in-memory fallback behavior matching
  `CreateRecordWithEvent` / `UpdateRecordWithEvent`.
- Keep the platform port optional through a separate interface, for example
  `transactionalUpsertRecordStore`, so existing `transactionalRecordStore`
  implementations and tests do not need a new required method unless they
  support transactional upsert directly.
- Add storage repository helpers that keep resource constants inside
  `storage_repository.go`:
  - upsert-with-event helpers for storage permissions, storage policies,
    project permissions, and user storage;
  - update-with-event helpers for group-storage command status and fast
    transfer cancel;
  - delete-with-event helpers for storage permission and project permission
    single deletes.
- Update non-batch storage handlers to use the new repository helpers:
  - `createStoragePermission`
  - `createStoragePolicy`
  - `setProjectBindingPermission`
  - `userStorageCommand`
  - `commandGroupStorage`
  - `deleteStoragePermission`
  - `deleteProjectBindingPermission`
  - `cancelFastTransfer`
- `batchUserStorageCommand` intentionally inherits per-item transactional
  coupling through `userStorageCommand`; this is not a whole-batch transaction.
  The broader batch ledger remains open because storage permission batches,
  project permission batches, and other service batches still need per-item
  coupling.
- Preserve current HTTP status codes, response payloads, event names, and event
  payload shapes.
- Preserve storage source-guard expectations: handlers should not reference
  storage-owned resource constants directly.
- Update `problem.md` and `gap.md` after verification to remove the storage
  upsert/non-batch mutation class from the open outbox list.

## Non-Goals

- No whole-batch transaction in this slice.
- `batchStoragePermissions` and `batchProjectPermissions` remain explicitly
  open for a later per-item coupling slice.
- No public HTTP contract change.
- No database schema migration.
- No typed storage schema migration.
- No change to already-coupled group-storage create/delete, project-binding
  create/delete, or fast-transfer create paths.
- No live RKE2 rollout unless verification exposes runtime wiring risk. The
  existing live outbox smoke covers the foundation; this slice is code-level
  coupling.

## Source References

- `problem.md`
- `gap.md`
- `backend/internal/platform/crud.go`
- `backend/internal/platform/ports.go`
- `backend/internal/platform/store_postgres.go`
- `backend/internal/platform/crud_event_routing_test.go`
- `backend/internal/services/storage/storage_repository.go`
- `backend/internal/services/storage/handler.go`
- `backend/internal/services/storage/helpers.go`
- `backend/internal/services/storage/handler_test.go`
- `backend/internal/services/storage/storage_repository_test.go`

## Assumptions

- `App.*RecordWithEvent` is the approved single-record transactional-outbox
  pattern.
- A platform upsert helper is preferable to duplicating update-then-create
  transactional logic in storage handlers.
- Postgres `CreateWithEvent` already uses conflict-safe insert behavior; the
  upsert helper can update, create, and conflict-fallback update inside one
  transaction before inserting one outbox event.
- Batch APIs have partial-success semantics and should be fixed later by
  coupling each item independently, not by wrapping the whole request.

## Affected Files

- `backend/internal/platform/ports.go`
- `backend/internal/platform/crud.go`
- `backend/internal/platform/store_postgres.go`
- `backend/internal/platform/crud_event_routing_test.go`
- `backend/internal/services/storage/storage_repository.go`
- `backend/internal/services/storage/handler.go`
- `backend/internal/services/storage/helpers.go`
- `backend/internal/services/storage/transactional_mutation_test.go`
- `docs/plan/2026-06-21-storage-mutation-transactional-outbox.md`
- `problem.md`
- `gap.md`

## API / Contract Changes

No public API change. Existing storage endpoint responses and event payloads are
preserved.

## Database / Migration Changes

None. This slice uses the existing `platform_records` and
`platform_event_outbox` tables.

## Security / 12-Factor Notes

- Existing user/admin/group/project authorization checks remain before mutation
  writes.
- No credentials, environment variables, or hard-coded deployment config are
  added.
- Storage resource ownership remains inside storage repository helpers.
- Eventing stays behind platform ports and works with both in-memory and
  Postgres-backed runtimes.

## Implementation Steps

- [x] Add optional platform transactional upsert port and
  `App.UpsertRecordWithEvent`.
- [x] Add `PostgresStore.UpsertWithEvent` with record write and outbox insert in
  one transaction.
- [x] Add platform tests for transactional routing and fallback publish behavior
  for `UpsertRecordWithEvent`.
- [x] Add platform tests proving update-existing, create-missing,
  conflict-fallback update, fallback publish, and no event when the owner write
  fails.
- [x] Add storage repository upsert-with-event helpers.
- [x] Add storage repository update/delete-with-event helpers for the remaining
  non-batch direct-publish handlers.
- [x] Update the non-batch storage handlers listed in Scope.
- [x] Add focused storage tests with a fake transactional store proving:
  - upsert/update/delete handlers route through transactional event helpers;
  - no direct `app.Events.Publish` happens for these events;
  - one event is emitted for successful create/update/delete paths;
  - missing update/delete paths preserve current not-found/OK behavior and do
    not emit events when no owner write occurs;
  - `batchUserStorageCommand` inherits per-item coupling through
    `userStorageCommand` without becoming one whole-batch transaction;
  - storage permission and project permission batch handlers remain unchanged
    and out of scope.
- [x] Run focused platform and storage tests.
- [x] Run full backend tests and quick gate.
- [x] Update `problem.md` and `gap.md` after verification.

## Verification Plan

```sh
gofmt -w backend/internal/platform/ports.go backend/internal/platform/crud.go backend/internal/platform/store_postgres.go backend/internal/platform/crud_event_routing_test.go backend/internal/services/storage/storage_repository.go backend/internal/services/storage/handler.go backend/internal/services/storage/helpers.go backend/internal/services/storage/transactional_mutation_test.go
go -C backend test ./internal/platform -run 'UpsertRecordWithEvent|CreateRecordWithEvent|WithTx' -count=1
go -C backend test ./internal/services/storage -run 'Transactional|Storage.*Workflow|ProjectBindings|DeletionBatch' -count=1
go -C backend test ./internal/platform
go -C backend test ./internal/services/storage
go -C backend test ./...
bash backend/scripts/ci-security-gate.sh quick
git diff --check
```

## Rollback Plan

Revert this slice. Existing storage repository methods remain close to current
behavior, so rollback returns the affected handlers to direct write plus
separate publish.

## Risks and Tradeoffs

- The helper still operates on JSONB `platform_records`; typed storage data
  ownership remains a separate architecture blocker.
- Update-then-create upsert semantics keep current behavior but are not a
  replacement for a typed SQL UPSERT schema.
- This intentionally leaves batch APIs open so their partial-success semantics
  can be handled explicitly.

## Reviewer Checklist

| Check | Status |
|---|---|
| Requirement fit for transactional outbox P0 partial | Pass |
| Approved-plan alignment | Pass |
| SOLID and service ownership | Pass |
| 12-Factor compliance | Pass |
| Tests/build/quick gate | Pass |
| Ledger accuracy | Pass |
| Diff scope | Pass |
