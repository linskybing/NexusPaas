# FastTransfer Progress State Ledger Sync Plan

Date: 2026-06-28
Scope: storage-service evidence ledger sync only

## 1. Objective

Record existing storage-service evidence that the internal FastTransfer progress
callback path updates FastTransfer records and emits transition events. The
previous kind slice proved the mover can emit HTTP progress callbacks to an
in-cluster sink; this slice documents the already-present local handler proof
for storage-service state changes.

## 2. Background

The storage-service handler-level transition proof already exists in tests but
is not yet reflected in the project ledgers, so the trackers still describe
storage-service progress-state transitions as unproven.

## 3. Source References

- `backend/internal/services/storage/fast_transfer_state_test.go`
- `backend/internal/services/storage/handler.go`
- `backend/internal/services/storage/spec.go`
- `backend/internal/contracts/fixtures/events/v1/fast-transfer-progressed.json`
- `backend/internal/contracts/fixtures/events/v1/fast-transfer-completed.json`

## 4. Assumptions

- The cited tests and handler/spec entries are already implemented and passing.
- Updating evidence wording does not change runtime behavior or acceptance scope.

## 5. Non-Goals

- No FastTransfer runtime behavior changes.
- No new k8s-control or platform/cluster behavior.
- No kind run in this docs-only slice.
- No claim that the kind mover callback reaches storage-service live.
- No storage GA, Full GA, or V1 external production launch claim.

## 6. Current Behavior

- `TestFastTransferProgressTransitionsAndEvents` proves queued -> running ->
  succeeded record updates (`status`, `progress_pct`, `bytes_done`, default
  `progress_pct=100`, stored checksum), emits `FastTransferProgressed` and
  `FastTransferCompleted`, and rejects a terminal-state running update.
- `TestFastTransferProgressRejectsDecreasingProgressAndBytes` proves monotonic
  progress/bytes validation.
- `TestFastTransferProgressRequiresServiceKey`,
  `TestFastTransferProgressAcceptsScopedServiceIdentity`, and
  `TestFastTransferProgressRejectsWrongScopedAudience` prove service-key and
  scoped service-identity authorization for the callback handler.
- `handler.go` registers `pathInternalFastTransferProgress` and implements
  `updateFastTransferProgress`; `spec.go` declares the internal
  `fast_transfers/progress` route and FastTransfer transition events.
- Public event envelope fixtures exist for progressed/completed.

## 7. Target Behavior

`gap.md`, `problem.md`, and `docs/acceptance/gap-analysis.md` carry one bounded
evidence note for FastTransfer progress storage-state transitions, scoped to
local handler proof.

## 8. Affected Domains

- Repository documentation and acceptance ledgers only.

## 9. Affected Files

- `gap.md`
- `problem.md`
- `docs/acceptance/gap-analysis.md`

## 10. API / Contract Changes

None.

## 11. Database / Migration Changes

None.

## 12. Configuration Changes

None.

## 13. Observability Changes

None.

## 14. Security Considerations

No secrets or operational evidence values are added.

## 15. Implementation Steps

1. Update `gap.md` with a bounded evidence row for FastTransfer progress
   storage-state transitions.
2. Update `problem.md` with the same bounded evidence narrative.
3. Update `docs/acceptance/gap-analysis.md` with the same bounded narrative.
4. Do not edit runtime code or tests unless Reviewer requests it.

Ledger boundary wording:

> FastTransfer progress callbacks now have local/in-memory storage-service
> handler evidence for queued -> running -> succeeded record updates, monotonic
> progress/bytes checks, terminal transition rejection, service identity
> authorization, and FastTransferProgressed/FastTransferCompleted event
> emission. This does not prove live k8s-control-to-storage-service callback
> delivery, live record updates from a Kubernetes mover Job, Redis delivery,
> accurate byte accounting, checksum correctness, resume, production secret
> handling, external storage backends, multi-node behavior, performance,
> durability, storage GA, Full GA, or V1 launch readiness.

## 16. Verification Plan

Run from `backend/`:

```bash
go test ./internal/services/storage -run 'FastTransferProgress|FastTransferStart' -count=1
go test ./internal/contracts/... -count=1
```

Run from repo root:

```bash
git diff --check
```

## 17. Rollback Plan

Revert the bounded evidence wording in the three ledger files. No runtime
rollback is needed.

## 18. Risks and Tradeoffs

- The risk is overclaiming: keep the wording bounded to local handler proof and
  do not upgrade it to live Kubernetes callback proof.

## 19. Reviewer Checklist

- The useful question is whether the existing test evidence really closes the
  specific ledger wording that currently says storage-service progress-state
  transitions are unproven.
- Reviewer should reject any wording that upgrades this from local handler proof
  to live Kubernetes callback proof.

## 20. Status

Status: Approved
