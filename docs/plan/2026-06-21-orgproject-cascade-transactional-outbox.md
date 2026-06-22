# OrgProject Cascade Transactional Outbox Slice

Status: Implemented and Reviewer-verified

Reviewer: Boole approved the plan before implementation after revisions for
group alias parity, project stored/requested child matching parity, and
delete-event payload assertions.

Final review: Boole PASS on 2026-06-21. No blocking findings. Reviewer
re-ran the focused orgproject delete/cascade tests and `git diff --check` for
the reviewed slice files.

Implementation evidence (2026-06-21):

- Added `DeleteGroupCascadeTx`, membership tx-delete helper, and
  `DeleteGPUClaimsByProjectTx`.
- Added `DeleteProjectCascadeTx` that deletes project, project members, user
  quotas, and project GPU claims through `StoreTx`.
- Updated `deleteGroup` and `deleteProject` handlers to use `app.WithTx` and
  `tx.Emit`.
- Added tx-routing tests proving no separate `app.Events` publish, exactly one
  tx-committed event, event payload parity, group alias membership deletion,
  and project stored/requested child matching.
- Verification passed:
  - `go -C backend test ./internal/services/orgproject -run 'Cascade|Delete' -count=1`
  - `go -C backend test ./internal/services/orgproject`
  - `go -C backend test ./...`
  - `bash backend/scripts/ci-security-gate.sh quick`

## Objective

Reduce the `Transactional outbox/inbox` P0 blocker by coupling
org-project-service cascade deletes with their domain events in one
`App.WithTx` transaction.

This closes the explicit remaining orgproject cascade-delete class in
`problem.md`; it does not claim the whole transactional-outbox blocker is done.

## Scope

- Add transaction-aware repository helpers for:
  - Project delete cascade: project, project members, user quotas, and project
    GPU claims.
  - Group delete cascade: group and group memberships.
- Update `DELETE /api/v1/projects/{id}` and `DELETE /api/v1/groups/{id}` to use
  `app.WithTx` and `tx.Emit(...)` so the cascade and `ProjectDeleted` /
  `GroupDeleted` event commit together on Postgres.
- Keep batch delete semantics per item by continuing to route each requested id
  through the single-delete handlers.
- Keep existing non-transactional repository cascade helpers for current tests
  and non-event internal use.
- Add focused tests proving:
  - project cascade uses `RunInTx`, deletes children, emits exactly one
    `ProjectDeleted` event through the transaction, and does not separately
    publish to `app.Events`;
  - project tx cascade preserves existing child matching parity by deleting
    project members and user quotas whose `project_id` / `projectId` equals
    either the stored project record ID or the requested project ID;
  - group cascade uses `RunInTx`, deletes memberships, emits exactly one
    `GroupDeleted` event through the transaction, and does not separately
    publish to `app.Events`;
  - group tx cascade preserves existing alias behavior by deleting memberships
    for both the stored group record ID and the logical `groupID(data)` when
    they differ;
  - emitted event type and payload match existing behavior:
    `ProjectDeleted` carries the deleted project data, and `GroupDeleted`
    carries `{"id": <requested id>}`;
  - existing raw cascade repository behavior remains intact.
- Update `problem.md` and `gap.md` after verification to remove orgproject
  cascade deletes from the open transactional-outbox list.

## Non-Goals

- No new event bus, broker, or distributed transaction mechanism.
- No backend route or payload contract change.
- No typed schema migration.
- No authorization-policy single-record fixes in this slice.
- No storage `Upsert*` transactional helper in this slice.
- No batch whole-operation transaction; batch delete must keep current
  partial-success semantics and couple each item independently.
- No live RKE2 rollout unless tests expose a runtime wiring issue. The live
  outbox/PDP evidence already covers the foundation; this slice is a focused
  code-coupling fix.

## Source References

- `problem.md`
- `backend/internal/platform/tx.go`
- `backend/internal/platform/ports.go`
- `backend/internal/services/storage/storage_repository.go`
- `backend/internal/services/orgproject/handler.go`
- `backend/internal/services/orgproject/project_handlers.go`
- `backend/internal/services/orgproject/project_repository.go`
- `backend/internal/services/orgproject/org_project_group_gpu_repository.go`
- `backend/internal/services/orgproject/project_repository_test.go`
- `backend/internal/services/orgproject/org_project_group_gpu_repository_test.go`

## Assumptions

- `App.WithTx` is the approved multi-record transactional-outbox primitive.
- Repository-owned resource keys should remain inside orgproject repositories.
- Child rows are selected from the committed store, then deleted through
  `platform.StoreTx`, matching the established storage cascade pattern.
- In-memory fallback remains non-atomic for data writes but buffers events until
  the callback succeeds, matching the existing platform contract.

## Affected Files

- `backend/internal/services/orgproject/handler.go`
- `backend/internal/services/orgproject/project_handlers.go`
- `backend/internal/services/orgproject/project_repository.go`
- `backend/internal/services/orgproject/org_project_group_gpu_repository.go`
- `backend/internal/services/orgproject/project_repository_test.go`
- `backend/internal/services/orgproject/org_project_group_gpu_repository_test.go`
- `docs/plan/2026-06-21-orgproject-cascade-transactional-outbox.md`
- `problem.md`
- `gap.md`

## API / Contract Changes

No public HTTP contract change. Existing delete endpoints keep their response
status and partial-success batch behavior.

## Database / Migration Changes

None. This slice uses the existing `platform_event_outbox` and `App.WithTx`
foundation.

## Security / 12-Factor Notes

- Existing admin authorization checks remain before the transaction.
- No credentials or environment variables are added.
- Domain data ownership stays in org-project-service repositories.
- Eventing remains behind the existing platform port; no new infrastructure is
  introduced.

## Implementation Steps

- [x] Add `DeleteGroupCascadeTx` plus membership tx-delete helper.
- [x] Add `DeleteProjectCascadeTx` plus project child tx-delete helpers and GPU
  claim tx-delete helper.
- [x] Update `deleteGroup` to call `app.WithTx` and `tx.Emit(GroupDeleted)`.
- [x] Update `deleteProject` to call `app.WithTx` and `tx.Emit(ProjectDeleted)`.
- [x] Add focused tx-routing tests with a scoped fake store.
- [x] Add tx parity tests for group alias membership deletion and project
  stored-ID/requested-ID child matching.
- [x] Add event contract assertions for `ProjectDeleted` and `GroupDeleted`
  payloads.
- [x] Preserve existing repository cascade tests.
- [x] Update `problem.md` and `gap.md` remaining blocker text.

## Verification Plan

```sh
go -C backend test ./internal/services/orgproject -run 'Cascade|Delete' -count=1
go -C backend test ./internal/services/orgproject
go -C backend test ./...
bash backend/scripts/ci-security-gate.sh quick
git diff --check
```

## Rollback Plan

Revert this slice. The old cascade behavior is preserved in repository helpers,
so rollback returns handlers to raw cascade delete plus separate publish.

## Risks and Tradeoffs

- The tx helpers list child rows from the committed store before deleting them
  through `StoreTx`; this matches existing local patterns but is not a general
  serializable cascade engine.
- This removes one P0 class but leaves authorization-policy single-record ops,
  storage upserts, and batch per-item coupling open.
- Live evidence remains foundation-level unless a runtime issue appears.

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
