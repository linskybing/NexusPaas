# FastTransfer Progress State Ledger Sync Plan

Date: 2026-06-28
Status: Approved
Scope: storage-service evidence ledger sync only

## Objective

Record existing storage-service evidence that the internal FastTransfer progress
callback path updates FastTransfer records and emits transition events. The
previous kind slice proved the mover can emit HTTP progress callbacks to an
in-cluster sink; this slice documents the already-present local handler proof
for storage-service state changes.

## Current Evidence

- `backend/internal/services/storage/fast_transfer_state_test.go`
  - `TestFastTransferProgressTransitionsAndEvents` proves:
    - queued transfer -> running progress update mutates `status`,
      `progress_pct`, and `bytes_done`
    - running transfer -> succeeded mutates `status`, defaults
      `progress_pct` to `100`, stores checksum, and emits
      `FastTransferCompleted`
    - terminal succeeded transfer rejects a later running update with conflict
    - `FastTransferProgressed` is emitted for the running transition
  - `TestFastTransferProgressRejectsDecreasingProgressAndBytes` proves
    monotonic progress/bytes validation.
  - `TestFastTransferProgressRequiresServiceKey`,
    `TestFastTransferProgressAcceptsScopedServiceIdentity`, and
    `TestFastTransferProgressRejectsWrongScopedAudience` prove service-key and
    scoped service-identity authorization for the callback handler.
- `backend/internal/services/storage/handler.go` registers
  `pathInternalFastTransferProgress` and implements
  `updateFastTransferProgress`.
- `backend/internal/services/storage/spec.go` declares the internal
  `fast_transfers/progress` route and FastTransfer transition events.
- `backend/internal/contracts/fixtures/events/v1/fast-transfer-progressed.json`
  and `fast-transfer-completed.json` cover the public event envelope fixtures.

## Implementation Steps

1. Update `gap.md` with a bounded evidence row for FastTransfer progress
   storage-state transitions.
2. Update `problem.md` with the same bounded evidence narrative.
3. Update `docs/acceptance/gap-analysis.md` with the same bounded evidence
   narrative.
4. Do not edit runtime code or tests unless Reviewer requests it.

## Ledger Boundary

Use wording equivalent to:

> FastTransfer progress callbacks now have local/in-memory storage-service
> handler evidence for queued -> running -> succeeded record updates,
> monotonic progress/bytes checks, terminal transition rejection, service
> identity authorization, and FastTransferProgressed/FastTransferCompleted
> event emission. This does not prove live k8s-control-to-storage-service
> callback delivery, live record updates from a Kubernetes mover Job, Redis
> delivery, accurate byte accounting, checksum correctness, resume, production
> secret handling, external storage backends, multi-node behavior, performance,
> durability, storage GA, Full GA, or V1 launch readiness.

## Verification

Run from `backend/`:

```bash
go test ./internal/services/storage -run 'FastTransferProgress|FastTransferStart' -count=1
go test ./internal/contracts/... -count=1
```

Run from repo root:

```bash
git diff --check
```

## Non-Goals

- No FastTransfer runtime behavior changes.
- No new k8s-control or platform/cluster behavior.
- No kind run in this docs-only slice.
- No claim that the kind mover callback reaches storage-service live.
- No storage GA, Full GA, or V1 external production launch claim.

## Reviewer Notes

The useful question is whether the existing test evidence really closes the
specific ledger wording that currently says storage-service progress-state
transitions are unproven. Reviewer should reject any wording that upgrades this
from local handler proof to live Kubernetes callback proof.

Status: Approved
