# Batch Per-Item Transactional Outbox Slice

Status: Implemented; pending Reviewer verification (revision 5)

Plan Agent: Codex

Reviewer: Boole PASS on revision 5. Code Agent may implement exactly this
plan.

## Objective

Close the remaining `Transactional outbox/inbox` P0 blocker class by coupling
batch mutation owner writes with their domain events per item, preserving
existing partial-success behavior.

This slice also fixes custom non-batch mutations discovered during the batch
inventory so the ledger can truthfully say only explicitly standalone events
remain outside transactional owner-write coupling.

## Scope

- Authorization policy:
  - `batchAssignPolicy`: create each assignment and emit `ProxyPolicyChanged`
    in the same item transaction.
  - `batchAssignRoleUsers`: create each role-user membership and emit
    `ProxyPolicyChanged` in the same item transaction.
  - `batchProcessPermissions`: apply each grouping-policy operation and emit
    `PolicyChanged` in the same item transaction.
- Storage:
  - `batchStoragePermissions`: use storage repository event-aware upsert/delete
    helpers per item.
  - `batchProjectPermissions`: use storage repository event-aware upsert/delete
    helpers per item.
- Scheduler quota:
  - `deletePlan`: delete the plan and emit `PlanChanged` in one item
    transaction; keep post-delete org-project unbind semantics.
  - `bindPlanQueues`: update the plan queue list and emit `PlanChanged` in one
    transaction.
  - `batchDeleteQueues`: delete each queue through
    `DeleteQueueAndRemoveFromPlansTx` and emit per deleted queue in the same
    item transaction.
  - `batchDeletePlans`: delete each plan and emit per deleted plan in the same
    item transaction; keep per-deleted-plan org-project unbind semantics.
  - preemption execution successful victim record update plus `JobPreempted`.
- Identity:
  - `updateUser`: update the user and emit `UserUpdated` in the same
    transaction after LDAP preconditions pass.
  - `deleteUser`: delete the user and emit `UserDisabled` in the same
    transaction after LDAP preconditions pass; `batchDeleteUsers` inherits this
    per-item coupling.
  - `batchResetPassword`: update each user password and emit `UserUpdated` in
    the same item transaction after LDAP preconditions pass.
  - `batchUpdateRole`: update each user role and emit `UserUpdated` in the same
    item transaction after LDAP preconditions pass.
- Org-project:
  - group membership create/update/delete helpers used by group member batch
    routes.
  - direct project member create/update/delete helpers used by project member
    batch routes.
  - project member quota upsert/delete helpers used by quota batch routes.
  - project workspace settings update.
  - GPU claim create/delete.
  - internal project plan binding/clearing contracts used by scheduler-quota.
- Image registry:
  - catalog sync request upsert.
  - catalog publish rule create/update.
  - catalog unpublish rule deletes.
  - catalog artifact delete plus rule cleanup.
- Workload:
  - job submit create plus `JobSubmitted`.
  - job cancel command create plus `JobCancelRequested`.
  - config file version commit plus `ConfigCommitted`.
  - config instance command create plus `ConfigInstanceCommanded`.
- Update `problem.md` and `gap.md` after verification to remove the batch
  per-item outbox blocker or narrow it to any Reviewer-approved deferrals.
- Update `docs/acceptance/gap-analysis.md` if GA checklist/architecture status
  text changes beyond the local ledgers.

## Verified No-Op Batch Paths

- `batchUserStorageCommand` already calls `userStorageCommand`, which was
  coupled in the storage non-batch slice.
- `batchUpdateImageRequests` calls `setImageRequestStatus`, which uses
  `App.UpdateRecordWithEvent` for request status writes.
- `batchUpdateFormStatus` calls `transitionForm`, which uses
  `App.UpdateRecordWithEvent`.
- `batchDeleteGroups` calls `deleteGroup`, which uses `App.WithTx`.
- `batchDeleteProjects` calls `deleteProject`, which uses `App.WithTx`.
- Scheduler admission `SubmitAdmissionReviewed` is treated as audit/telemetry:
  the persisted review is explicitly documented as audit-only, does not mutate a
  long-lived owner aggregate for downstream state, and is therefore outside the
  owner-write + domain-event coupling class for this slice.

## Non-Goals

- No whole-request batch transaction. Partial success remains per item.
- No public HTTP response contract change.
- No event name or event payload shape change unless a focused test proves the
  old shape cannot represent a per-item event safely.
- No typed schema migration.
- No new broker, queue, or custom outbox implementation. Use the existing
  `App.*RecordWithEvent`, `App.WithTx`, `StoreTx`, and Postgres outbox support.
- No live RKE2 rollout in this slice unless tests expose runtime wiring risk or
  the Reviewer requires fresh live evidence for ledger closure. The live outbox
  foundation evidence remains valid; this slice is code-level coupling plus
  focused route tests.

## Source References

- `problem.md`
- `gap.md`
- `backend/internal/platform/tx.go`
- `backend/internal/platform/crud.go`
- `backend/internal/platform/ports.go`
- `backend/internal/services/authorizationpolicy/permissions.go`
- `backend/internal/services/authorizationpolicy/assignments.go`
- `backend/internal/services/authorizationpolicy/roles.go`
- `backend/internal/services/authorizationpolicy/raw_permission_repository.go`
- `backend/internal/services/storage/helpers.go`
- `backend/internal/services/storage/storage_repository.go`
- `backend/internal/services/schedulerquota/handler.go`
- `backend/internal/services/schedulerquota/scheduler_quota_repository.go`
- `backend/internal/services/schedulerquota/preemption.go`
- `backend/internal/services/schedulerquota/scheduler_preemption_priority_repository.go`
- `backend/internal/services/identity/users.go`
- `backend/internal/services/identity/ldap.go`
- `backend/internal/services/identity/principal_repository.go`
- `backend/internal/services/orgproject/handler.go`
- `backend/internal/services/orgproject/project_handlers.go`
- `backend/internal/services/orgproject/plan_binding_contracts.go`
- `backend/internal/services/orgproject/project_helpers.go`
- `backend/internal/services/orgproject/org_project_group_gpu_repository.go`
- `backend/internal/services/orgproject/project_repository.go`
- `backend/internal/services/imageregistry/handler.go`
- `backend/internal/services/imageregistry/helpers.go`
- `backend/internal/services/workload/job_submit.go`
- `backend/internal/services/workload/job_access_handlers.go`
- `backend/internal/services/workload/handler.go`
- `backend/internal/services/workload/job_repository.go`
- `backend/internal/services/workload/config_repository.go`

## Assumptions

- `App.WithTx` is the approved multi-record pattern for durable owner writes
  and domain events.
- In-memory fallback can still write first and publish after, matching the
  existing platform fallback contract; tests should assert transactional stores
  route through tx/event helpers and do not call direct `Events.Publish`.
- LDAP operations remain outside the local DB transaction. The local user write
  and outbox event must be coupled only after the LDAP call succeeds when LDAP
  runs before the local write. Existing compensation behavior must remain for
  flows that intentionally perform the local update before LDAP password sync.
  Tests should cover that LDAP failure does not emit an event.
- Scheduler plan unbind calls are external owner-contract calls and should stay
  after a successful local delete, preserving current fire-and-forget cleanup.
- Scheduler preemption invokes Kubernetes cleanup and workload preempt as
  external side effects. This slice couples only the scheduler-owned preemption
  record update and `JobPreempted` event after those side effects succeed; it
  does not introduce a distributed transaction with workload-service.

## Affected Files

- `backend/internal/services/authorizationpolicy/permissions.go`
- `backend/internal/services/authorizationpolicy/assignments.go`
- `backend/internal/services/authorizationpolicy/roles.go`
- `backend/internal/services/authorizationpolicy/raw_permission_repository.go`
- `backend/internal/services/authorizationpolicy/transactional_mutation_test.go`
- `backend/internal/services/storage/helpers.go`
- `backend/internal/services/storage/transactional_mutation_test.go`
- `backend/internal/services/schedulerquota/handler.go`
- `backend/internal/services/schedulerquota/scheduler_quota_repository.go`
- `backend/internal/services/schedulerquota/handler_test.go`
- `backend/internal/services/schedulerquota/preemption.go`
- `backend/internal/services/schedulerquota/scheduler_preemption_priority_repository.go`
- `backend/internal/services/schedulerquota/preemption_test.go`
- `backend/internal/services/identity/users.go`
- `backend/internal/services/identity/ldap.go`
- `backend/internal/services/identity/principal_repository.go`
- `backend/internal/services/identity/handler_test.go`
- `backend/internal/services/orgproject/handler.go`
- `backend/internal/services/orgproject/project_handlers.go`
- `backend/internal/services/orgproject/plan_binding_contracts.go`
- `backend/internal/services/orgproject/project_helpers.go`
- `backend/internal/services/orgproject/org_project_group_gpu_repository.go`
- `backend/internal/services/orgproject/project_repository.go`
- `backend/internal/services/orgproject/transactional_cascade_test.go`
- `backend/internal/services/orgproject/plan_binding_contracts_test.go`
- `backend/internal/services/imageregistry/handler.go`
- `backend/internal/services/imageregistry/helpers.go`
- `backend/internal/services/imageregistry/handler_test.go`
- `backend/internal/services/imageregistry/transactional_mutation_test.go`
- `backend/internal/services/workload/job_submit.go`
- `backend/internal/services/workload/job_access_handlers.go`
- `backend/internal/services/workload/handler.go`
- `backend/internal/services/workload/job_repository.go`
- `backend/internal/services/workload/config_repository.go`
- `backend/internal/services/workload/handler_test.go`
- `backend/internal/services/workload/job_submit_test.go`
- `backend/internal/services/workload/transactional_mutation_test.go`
- `docs/acceptance/gap-analysis.md`
- `docs/plan/2026-06-21-batch-per-item-transactional-outbox.md`
- `problem.md`
- `gap.md`

## API / Contract Changes

No public API change. Batch endpoints keep current partial-success response
shape (`succeeded`, `failed`, `errors`) and successful item counting.

Existing event names and payload shapes should be preserved where already
consumer-visible:

- authorization-policy assignment/role-user events keep existing proxy-policy
  action names.
- storage permission events keep existing storage event names.
- scheduler quota batch delete changes from one aggregate `batch_deleted` event
  to per-deleted-item `deleted` events because outbox coupling requires each
  successful owner write to carry its own committed event. The HTTP response
  remains unchanged.

## Database / Migration Changes

None. This uses existing `platform_records` and `platform_event_outbox` tables.

## Security / 12-Factor Notes

- Existing authorization checks stay before mutation writes.
- No secrets, credentials, hard-coded environment config, or deployment-specific
  settings are added.
- Service-owned resource keys stay inside repository/helper layers where that
  pattern already exists.
- No new platform dependency is introduced; the slice reuses existing
  cloud-native outbox/relay primitives.

## Implementation Steps

- [x] Add missing repository tx helpers:
  - raw permission grouping operation apply/upsert/delete tx helpers;
  - scheduler plan delete, plan queue bind, and preemption result tx helpers;
  - identity user update/delete tx helpers needed by update, delete, reset, and
    role mutation paths;
  - org-project group membership, direct project member, quota, workspace, and
    GPU claim tx helpers;
  - org-project project plan bind/clear tx helpers;
  - image-registry catalog/rule tx helpers where existing platform helpers do
    not already fit;
  - workload submitted-job, job-command, config-version, and config-instance
    command tx helpers.
- [x] Update authorization-policy batch handlers to run one `App.WithTx` per
  successful item and `tx.Emit` the matching event inside that transaction.
- [x] Update storage permission batch handlers to use event-aware repository
  helpers per item.
- [x] Update scheduler-quota custom plan mutations and queue/plan delete batch
  handlers to couple owner writes and events per item.
- [x] Update scheduler preemption successful victim handling so
  `AppendPreemptionVictim`/related record updates and `JobPreempted` commit
  through one local transaction after cleanup and workload preempt succeed.
- [x] Update identity update/delete/reset/update-role paths so local user
  mutations and `UserUpdated`/`UserDisabled` events commit together, with
  `batchDeleteUsers`, `batchResetPassword`, and `batchUpdateRole` inheriting
  per-item coupling.
- [x] Update org-project group membership, direct project member, and quota
  helpers so single and batch callers share tx-coupled owner writes/events.
- [x] Update org-project workspace settings and GPU claim create/delete paths
  to use tx-coupled owner writes/events.
- [x] Update org-project internal plan binding contracts so project plan bind
  and clear operations emit `ProjectUpdated` inside the owner-write transaction.
- [x] Update image-registry catalog sync, publish, unpublish, and artifact
  delete paths so every owner write and matching event commit together. For
  multi-rule operations, keep current partial outcome semantics by coupling each
  successful rule deletion/write independently rather than using one whole
  request transaction.
- [x] Update workload job submit/cancel and config commit/instance-command paths
  so each owner write and matching event commits together.
- [x] Add focused tests proving:
  - transactional stores receive one item transaction/event per successful item;
  - direct `app.Events.Publish` is not called on newly coupled paths;
  - failed and missing items preserve partial-success accounting and do not emit
    events;
  - already-coupled no-op batch paths remain routed through their single-item
    helpers.
  - LDAP failure paths do not emit events before or after rollback.
  - org-project non-batch helper paths newly included by this revision do not
    publish through `app.Events.Publish` directly.
  - org-project plan binding contracts commit `ProjectUpdated` events through
    `StoreTx`.
  - image-registry catalog/rule mutations do not publish directly and emit no
    events when no owner write occurs.
  - workload submit/cancel/config mutation handlers commit their events through
    transaction-aware helpers and do not publish directly.
  - scheduler preemption success emits `JobPreempted` through `StoreTx`, while
    cleanup/preflight/workload-preempt failure paths do not emit the event.
- [x] Run focused service tests, full backend tests, quick gate, and
  `git diff --check`.
- [x] Update `problem.md`, `gap.md`, and `docs/acceptance/gap-analysis.md`
  with verified evidence and remaining non-goal caveats.
- [x] Submit implementation back to Reviewer Agent.

## Implementation Evidence

- `gofmt` run over all touched implementation and test files.
- Focused transactional/batch/catalog/workload tests:
  `go -C backend test ./internal/services/authorizationpolicy ./internal/services/storage ./internal/services/schedulerquota ./internal/services/identity ./internal/services/orgproject ./internal/services/imageregistry ./internal/services/workload -run 'Transactional|Batch|Catalog|Submit|Cancel|Config|Quota|GPU|Plan|Member|LDAP|User' -count=1`
  passed.
- Full touched-service package tests:
  `go -C backend test ./internal/services/authorizationpolicy ./internal/services/storage ./internal/services/schedulerquota ./internal/services/identity ./internal/services/orgproject ./internal/services/imageregistry ./internal/services/workload -count=1`
  passed.
- Full backend tests: `go -C backend test ./... -count=1` passed.
- Diff hygiene: `git diff --check` passed.
- Quick gate: `bash backend/scripts/ci-security-gate.sh quick` passed, including
  Go version check, gofmt check, `go vet ./...`, `go test ./... -count=1`, and
  `go build ./...`.
- Reviewer verification is still pending. The implementation was submitted to
  the existing Reviewer Agent, but the agent returned no review text; a
  replacement Reviewer Agent then failed on the current usage limit before it
  could produce PASS/FAIL.

## Verification Plan

```sh
gofmt -w backend/internal/services/authorizationpolicy/permissions.go backend/internal/services/authorizationpolicy/assignments.go backend/internal/services/authorizationpolicy/roles.go backend/internal/services/authorizationpolicy/raw_permission_repository.go backend/internal/services/authorizationpolicy/transactional_mutation_test.go backend/internal/services/storage/helpers.go backend/internal/services/storage/transactional_mutation_test.go backend/internal/services/schedulerquota/handler.go backend/internal/services/schedulerquota/scheduler_quota_repository.go backend/internal/services/schedulerquota/handler_test.go backend/internal/services/schedulerquota/preemption.go backend/internal/services/schedulerquota/scheduler_preemption_priority_repository.go backend/internal/services/schedulerquota/preemption_test.go backend/internal/services/identity/users.go backend/internal/services/identity/ldap.go backend/internal/services/identity/principal_repository.go backend/internal/services/identity/handler_test.go backend/internal/services/orgproject/handler.go backend/internal/services/orgproject/project_handlers.go backend/internal/services/orgproject/plan_binding_contracts.go backend/internal/services/orgproject/project_helpers.go backend/internal/services/orgproject/org_project_group_gpu_repository.go backend/internal/services/orgproject/project_repository.go backend/internal/services/orgproject/transactional_cascade_test.go backend/internal/services/orgproject/plan_binding_contracts_test.go backend/internal/services/imageregistry/handler.go backend/internal/services/imageregistry/helpers.go backend/internal/services/imageregistry/handler_test.go backend/internal/services/imageregistry/transactional_mutation_test.go backend/internal/services/workload/job_submit.go backend/internal/services/workload/job_access_handlers.go backend/internal/services/workload/handler.go backend/internal/services/workload/job_repository.go backend/internal/services/workload/config_repository.go backend/internal/services/workload/handler_test.go backend/internal/services/workload/job_submit_test.go backend/internal/services/workload/transactional_mutation_test.go
go -C backend test ./internal/services/authorizationpolicy -run 'Transactional|Batch' -count=1
go -C backend test ./internal/services/storage -run 'Transactional|Batch|DeletionBatch' -count=1
go -C backend test ./internal/services/schedulerquota -run 'Transactional|Batch|Plan|Queue|Preemption' -count=1
go -C backend test ./internal/services/identity -run 'Batch|Transactional|User|LDAP' -count=1
go -C backend test ./internal/services/orgproject -run 'Transactional|Batch|Member|Quota|Workspace|GPU|PlanBinding' -count=1
go -C backend test ./internal/services/imageregistry -run 'Transactional|Catalog|Publish|Image' -count=1
go -C backend test ./internal/services/workload -run 'Transactional|Submit|Cancel|Config' -count=1
go -C backend test ./internal/services/authorizationpolicy ./internal/services/storage ./internal/services/schedulerquota ./internal/services/identity ./internal/services/orgproject ./internal/services/imageregistry ./internal/services/workload
go -C backend test ./...
bash backend/scripts/ci-security-gate.sh quick
git diff --check
```

## Rollback Plan

Revert this slice. Existing batch endpoints would return to their current
partial-success behavior with direct publish after owner writes.

## Risks and Tradeoffs

- This is wider than earlier single-service slices. The breadth is justified by
  the current ledger claiming the remaining outbox blocker is batch-only; the
  plan keeps each service change small and repository-local.
- Per-item transactions mean a later item failure does not roll back earlier
  successful items. That is intentional and preserves current partial-success
  semantics.
- LDAP and org-project binding calls are external side effects and cannot be
  part of the local DB transaction without distributed transactions. The plan
  keeps local durable write + event coupling correct and preserves current
  compensation/fire-and-forget behavior.

## Reviewer Checklist

| Check | Status |
|---|---|
| Requirement fit for transactional outbox P0 | Pending |
| Approved-plan alignment | Pending |
| SOLID and service ownership | Pending |
| 12-Factor compliance | Pending |
| Tests/build/quick gate plan | Pending |
| Diff scope and rollback safety | Pending |
