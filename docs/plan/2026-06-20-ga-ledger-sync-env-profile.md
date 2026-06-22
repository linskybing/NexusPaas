# GA Ledger Sync Environment Profile

## 1. Objective

Synchronize the GA checklist docs after the approved environment profile and
PDP fail-closed implementation, without changing production code.

## 2. Background

The environment profile/PDP slice was implemented, verified, rolled out to the
live RKE2 namespace, and approved by Reviewer Agent. `problem.md` and `gap.md`
still list the item as open/partial, and `gap.md` also still has stale statuses
for the already-approved API token indexed lookup and trusted client IP slices.

## 3. Source References

- `problem.md`
- `gap.md`
- `docs/plan/2026-06-20-environment-profile-pdp-fail-closed.md`
- `docs/plan/2026-06-20-api-token-problem-ledger-update.md`
- `docs/plan/2026-06-20-trusted-client-ip-problem-ledger-update.md`

## 4. Assumptions

- This is documentation/status bookkeeping only.
- `Transactional outbox/inbox`, typed data ownership, reproducible toolchain,
  Web GUI, and live GA E2E remain open unless separately completed.

## 5. Non-Goals

- No code, manifest, migration, or test changes.
- No claim that all GA acceptance criteria now pass.
- No deletion of remaining open blockers.

## 6. Current Behavior

- `problem.md` lists environment profiles and PDP fail-closed as a P0 blocker.
- `gap.md` lists API-token indexed lookup as open, centralized trusted-IP
  resolver as open, and env profiles/PDP fail-closed as partial.

## 7. Target Behavior

- `problem.md` records environment profiles/PDP fail-closed as done with
  reviewer/live evidence and removes it from P0 blockers.
- `gap.md` records API-token indexed lookup, centralized trusted-IP resolver,
  and env profiles/PDP fail-closed as done.
- Remaining open blockers remain visible.

## 8. Affected Domains

- GA status tracking.
- Acceptance checklist evidence.

## 9. Affected Files

- `docs/plan/2026-06-20-ga-ledger-sync-env-profile.md`
- `problem.md`
- `gap.md`

## 10. API / Contract Changes

None.

## 11. Database / Migration Changes

None.

## 12. Configuration Changes

None.

## 13. Observability Changes

None.

## 14. Security Considerations

This sync must not hide remaining security or reliability blockers. It only
updates items with completed implementation and reviewer evidence.

## 15. Implementation Steps

1. Update `problem.md` completed-status table and remove the environment profile
   row from the P0 blocker table.
2. Update `gap.md` architecture blocker statuses for API token indexed lookup,
   trusted client IP resolver, and env profiles/PDP fail-closed.
3. Run markdown/diff sanity checks.

## 16. Verification Plan

```sh
git diff --check -- problem.md gap.md docs/plan/2026-06-20-ga-ledger-sync-env-profile.md
rg -n "API-token indexed lookup|Centralized trusted-IP resolver|Env profiles|Environment profiles" problem.md gap.md
```

## 17. Rollback Plan

Revert this docs-only sync if Reviewer Agent finds that a status was overstated.

## 18. Risks and Tradeoffs

The main risk is overclaiming. Keep remaining blockers open and include evidence
language only for completed slices.

## 19. Reviewer Checklist

- Status updates match implemented and approved work.
- Remaining open blockers are preserved.
- No production code is changed.
- No GA-wide completion is claimed.

## 20. Status

Status: Approved
