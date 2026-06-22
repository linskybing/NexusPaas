# AuthorizationPolicy Mutation Transactional Outbox Slice

Status: Implemented and Reviewer-verified

Plan Agent: Codex

Reviewer: Boole approved the plan after revisions for proxy policy rule
replacement, ID-changing raw permission updates, and explicit out-of-scope
already-coupled/non-mutating handlers.

Final review: Boole PASS on 2026-06-21. No blocking findings. Reviewer
re-ran focused authorization-policy mutation tests and scoped diff whitespace
checks.

Implementation evidence (2026-06-21):

- Added transaction-aware repository helpers for proxy roles, proxy policy
  update/rule replacement, policy assignments, role-user assignments, and raw
  permission policy create/update/delete.
- Updated non-batch authorization-policy handlers to write through
  `app.WithTx` and emit `ProxyPolicyChanged` / `PolicyChanged` through
  `tx.Emit`.
- Added focused fake-transaction-store tests proving tx routing, no direct
  `app.Events` publish, event count/action payloads, idempotent replay
  behavior, proxy policy rule replacement, and raw permission rename/conflict /
  not-found behavior.
- Verification passed:
  - `go -C backend test ./internal/services/authorizationpolicy -run 'Transactional|RawPermission|ProxyPolicy|ProxyRole|Assignment' -count=1`
  - `go -C backend test ./internal/services/authorizationpolicy`
  - `go -C backend test ./...`
  - `bash backend/scripts/ci-security-gate.sh quick`

## Objective

Reduce the `Transactional outbox/inbox` P0 blocker by coupling
authorization-policy-service non-batch mutation writes with their domain events
through the existing transactional outbox primitives.

This closes the open non-batch authorization-policy mutation class in
`problem.md`. It does not claim the whole outbox blocker is done because
storage upserts and batch per-item coupling remain open.

## Scope

- Keep using the existing platform `App.WithTx` / `StoreTx` primitives; no new
  broker, event bus, or transaction framework.
- Add transaction-aware repository helpers for:
  - proxy role create/update;
  - proxy policy update with optional rule replacement;
  - proxy policy assignment create/delete;
  - proxy role-user assignment create/delete;
  - raw permission policy create/update/delete.
- Update non-batch handlers to emit `ProxyPolicyChanged` / `PolicyChanged`
  inside the same transaction as their owner write.
- For proxy policy update with `rules` present, delete old rules and create
  replacement rules through `StoreTx`, emit exactly one tx
  `ProxyPolicyChanged` event with action `update`, and preserve the existing
  `{"old": ..., "new": ...}` response/event payload shape where `new.rules`
  contains the replacement rules.
- For raw permission policy update where the policy ID changes, create the new
  row and delete the old row through `StoreTx`, emit exactly one tx
  `PolicyChanged` event with action `policy_updated`, and preserve conflict /
  not-found behavior without emitting an event.
- Preserve idempotent create semantics:
  - existing proxy assignment or role-user assignment returns HTTP 200 and does
    not emit a duplicate event;
  - raw permission duplicate create still returns HTTP 409;
  - raw permission update conflict still returns HTTP 409.
- Preserve current response statuses and payload shapes.
- Keep repository ownership of authorization-policy resource constants.
- Update `problem.md` and `gap.md` after verification to remove
  authorization-policy non-batch mutations from the open outbox list.

## Non-Goals

- No whole-batch transaction for batch APIs.
- No batch per-item coupling in this slice; `batchProcessPermissions`,
  `batchAssignPolicy`, and `batchAssignRoleUsers` remain explicitly open.
- No churn to already-coupled proxy policy create/delete or proxy role delete;
  `createPolicy`, `deletePolicy`, and `deleteRole` already use `App.WithTx`.
- No change to `createService`; it returns HTTP 405 and does not mutate.
- No storage `Upsert*` helper in this slice.
- No typed schema migration.
- No public HTTP contract change.
- No live RKE2 rollout unless verification exposes runtime wiring risk. The
  existing live outbox smoke covers the foundation; this slice is code-level
  coupling.

## Source References

- `problem.md`
- `gap.md`
- `backend/internal/platform/tx.go`
- `backend/internal/platform/crud.go`
- `backend/internal/services/authorizationpolicy/permissions.go`
- `backend/internal/services/authorizationpolicy/policies.go`
- `backend/internal/services/authorizationpolicy/assignments.go`
- `backend/internal/services/authorizationpolicy/roles.go`
- `backend/internal/services/authorizationpolicy/authorization_policy_repository.go`
- `backend/internal/services/authorizationpolicy/raw_permission_repository.go`
- `backend/internal/services/authorizationpolicy/workflow_test.go`
- `backend/internal/services/authorizationpolicy/authorization_policy_repository_test.go`
- `backend/internal/services/authorizationpolicy/raw_permission_repository_test.go`

## Assumptions

- `App.WithTx` is the approved multi-record transactional-outbox primitive for
  handlers that need to coordinate writes and emitted events.
- Reading current child rows from the committed store before deleting/replacing
  them through `StoreTx` is acceptable and matches the existing storage and
  orgproject cascade pattern.
- In-memory tests only prove event routing and post-callback publish behavior;
  Postgres provides the real atomic commit guarantee.
- Batch APIs have partial-success semantics and should be fixed later by
  coupling each item independently, not by wrapping the whole request.

## Affected Files

- `backend/internal/services/authorizationpolicy/permissions.go`
- `backend/internal/services/authorizationpolicy/policies.go`
- `backend/internal/services/authorizationpolicy/assignments.go`
- `backend/internal/services/authorizationpolicy/roles.go`
- `backend/internal/services/authorizationpolicy/authorization_policy_repository.go`
- `backend/internal/services/authorizationpolicy/raw_permission_repository.go`
- `backend/internal/services/authorizationpolicy/transactional_mutation_test.go`
- `docs/plan/2026-06-21-authorizationpolicy-mutation-transactional-outbox.md`
- `problem.md`
- `gap.md`

## API / Contract Changes

None. Existing endpoints, status codes, response payloads, event names, and
event `action` values are preserved.

## Database / Migration Changes

None. This slice uses the existing `platform_event_outbox` schema and
`App.WithTx` foundation.

## Security / 12-Factor Notes

- Existing admin and service-principal checks remain before mutation writes.
- No credentials, environment variables, or hard-coded deployment config are
  added.
- Authorization-policy resource ownership remains inside repository files.
- Eventing stays behind the platform port; the service remains portable across
  in-memory and Postgres-backed runtimes.

## Implementation Steps

- [x] Add tx repository helpers for proxy role create/update.
- [x] Add tx repository helper for proxy policy update plus optional rule
  replacement.
- [x] Add tx repository helpers for policy assignment create/delete.
- [x] Add tx repository helpers for role-user assignment create/delete.
- [x] Add tx repository helpers for raw permission policy create/update/delete.
- [x] Update non-batch handlers to call `app.WithTx` and `tx.Emit`.
- [x] Add focused tests with a fake transactional store proving:
  - handlers route through `RunInTx`;
  - no direct `app.Events.Publish` happens for the mutated event;
  - exactly one tx event is emitted for successful non-idempotent mutations;
  - `updatePolicy` with replacement `rules` deletes old rules, creates
    replacement rules through `StoreTx`, emits one `ProxyPolicyChanged` action
    `update`, and preserves the `old`/`new` payload shape with replacement
    rules in `new`;
  - ID-changing raw permission update creates the new row, deletes the old row,
    emits one `PolicyChanged` action `policy_updated`, and leaves conflict /
    not-found paths event-free with existing rows preserved;
  - idempotent assignment replay does not emit a duplicate event;
  - raw permission create/update/delete preserve conflict/not-found behavior.
- [x] Run focused authorization-policy tests.
- [x] Run full backend tests and quick gate.
- [x] Update `problem.md` and `gap.md` after verification.

## Verification Plan

```sh
gofmt -w backend/internal/services/authorizationpolicy/*.go
go -C backend test ./internal/services/authorizationpolicy -run 'Transactional|RawPermission|ProxyPolicy|ProxyRole|Assignment' -count=1
go -C backend test ./internal/services/authorizationpolicy
go -C backend test ./...
bash backend/scripts/ci-security-gate.sh quick
git diff --check
```

## Rollback Plan

Revert this slice. Existing repository methods and handler behavior are kept
close to the current shape, so rollback returns authorization-policy non-batch
mutations to direct write plus separate publish.

## Risks and Tradeoffs

- The tx helpers still use JSONB `platform_records`; typed ownership remains a
  separate architecture blocker.
- This intentionally leaves batch APIs open so their partial-success semantics
  can be handled explicitly.
- Seeding paths continue using non-event repository methods because default seed
  rows are bootstrap state, not user-requested domain mutations.

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
