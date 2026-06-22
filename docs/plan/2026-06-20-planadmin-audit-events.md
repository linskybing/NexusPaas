# PlanAdmin Audit Event Slice

## 1. Objective

Close the core `PLANADMIN-*` launch gap for Plan/Queue lifecycle auditability by
ensuring scheduler-quota Plan/Queue mutation events include actor and
before/after values.

## 2. Background

The scheduler-quota service already owns Plans and Queues, route metadata marks
Plan/Queue mutations admin-only, and org-project owns the Project plan binding
with a single `plan_id`/`resource_plan_id` update contract. Existing events
(`PlanChanged`, `QueueChanged`, `ProjectUpdated`) prove changes happened, but
Plan/Queue events do not consistently include actor and before/after values.

## 3. Scope

- Add actor metadata to scheduler-quota domain events from request headers.
- Publish `old`/`new` values for create/update/delete Plan and Queue mutations.
- Publish `old`/`new` values for Plan queue binding.
- Keep Project plan binding through org-project owner contract; do not write
  Project aggregates from scheduler-quota.
- Add focused tests for Plan/Queue before/after event payloads.

## 4. Non-Goals

- No scheduler, quota, or admission algorithm changes.
- No schema migration.
- No new audit backend or event broker.
- No Web UI, secret API, or audit-query implementation.

## 5. CNCF / Cloud-Native Fit

This slice reuses the existing event bus and service-owned data boundaries. It
does not introduce a custom scheduler, queueing system, policy engine, metrics
backend, or secret store.

## 6. Affected Files

- `docs/plan/2026-06-20-planadmin-audit-events.md`
- `docs/plan/2026-06-20-v1-launch-gap-gate.md`
- `backend/internal/services/schedulerquota/handler.go`
- `backend/internal/services/schedulerquota/handler_test.go`

## 7. Contract Changes

`PlanChanged` and `QueueChanged` events for primary lifecycle mutations include:

- `action`
- `actor_user_id`
- `old`
- `new`

The `old` value is `nil` for create. The `new` value is a deletion marker for
delete. Existing top-level fields are preserved where useful.

## 8. Implementation Steps

- [x] Add scheduler event helpers for actor and old/new payloads.
- [x] Update Plan/Queue create/update/delete/bind-queues event publishing.
- [x] Add focused handler tests for actor plus old/new values.
- [x] Run focused scheduler tests.
- [x] Run quick gate and Sonar.
- [x] Update V1 checklist status/evidence.

## 9. Verification Plan

```sh
go -C backend test ./internal/services/schedulerquota -run 'QueueAndPlanWorkflow|ReadUpdateAndBatchPlanHandlers|PlanAdmin' -count=1
go -C backend test ./internal/services -run 'CatalogStateChanging|RegisterAllAdminCoverage' -count=1
go -C backend test ./... -coverprofile=coverage.out -count=1
bash backend/scripts/ci-security-gate.sh quick
bash backend/scripts/ci-security-gate.sh sonar
```

Executed on 2026-06-20:

```sh
go -C backend test ./internal/services/schedulerquota -run 'QueueAndPlanWorkflow|ReadUpdateAndBatchPlanHandlers|PlanAdmin' -count=1 -v
go -C backend test ./internal/services -run 'CatalogStateChanging|RegisterAllAdminCoverage|RouteCoverage|AuditEvents|EventContracts' -count=1
go -C backend test ./internal/services/schedulerquota -count=1
go -C backend test ./... -coverprofile=coverage.out -count=1
bash backend/scripts/ci-security-gate.sh quick
bash backend/scripts/ci-security-gate.sh sonar
```

Result: all commands passed; SonarScanner Quality Gate passed.

## 10. Rollback Plan

Revert this slice. No schema or data migration is involved.

## 11. Risks

- Events become slightly larger because they include before/after maps.
- Event payloads must remain free of secrets. Plan/Queue records currently carry
  resource and queue metadata, not credentials.

## 12. Reviewer Checklist

| Check | Status |
|---|---|
| Requirement fit for `PLANADMIN-001..003` audit evidence | Pass |
| Scope stays limited to event payload hardening | Pass |
| Reuses existing service/event infrastructure | Pass |
| SOLID: payload construction is isolated | Pass |
| 12-Factor: no environment coupling | Pass |
| Tests/build/Sonar evidence recorded | Pass |
| Risks and diff scope reviewed | Pass |

## 13. Status

Status: Implemented and reviewer-verified for this slice.

Reviewer Agent: Approved and verified. The implementation improves Plan/Queue
mutation evidence while keeping scheduler-quota ownership and org-project
Project ownership intact.
