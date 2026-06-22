# Route Auth And Collision Problem Ledger Update

## 1. Objective

Update `problem.md` after the route auth and collision hardening slice passed
final reviewer verification, local gates, Sonar Quality Gate, and live RKE2
service-auth evidence.

## 2. Background

`problem.md` still lists `Internal route auth` and `Route collision detection`
as P0 GA blockers. The implementation plan
`docs/plan/2026-06-20-route-auth-collision-hardening.md` is now
reviewer-verified and records:

- catalog internal routes declare central service-auth metadata;
- middleware/service auth rejects missing and wrong `X-Service-Key`;
- startup validates duplicate canonical route shapes unless they are explicit
  aliases or overrides;
- focused tests, full backend tests, vet, build, quick gate, and Sonar Quality
  Gate passed;
- live RKE2 evidence showed all 15 backend deployments started on the current
  image, and `workload-service` internal route returned 401/401/200 for
  missing/wrong/valid service key.

## 3. Scope

- Move `Internal route auth` and `Route collision detection` from
  `P0 Blockers Before Production/GA` to `Completed Or In Progress`.
- Update the existing `Route auth and collision hardening` completed row from
  `Pending merge` to `Done`.
- Keep remaining P0 blockers unchanged.
- Do not edit code.
- Do not claim all GA blockers are closed.

## 4. Implementation Steps

1. Update the completed route hardening row to `Done` with reviewer/gate/live
   evidence.
2. Remove the `Internal route auth` and `Route collision detection` P0 rows.
3. Verify that both items are absent from the P0 section and remaining P0
   blockers are still present.

## 5. Verification Plan

```sh
python3 - <<'PY'
from pathlib import Path
text = Path("problem.md").read_text()
assert "| Route auth and collision hardening | Done |" in text
p0 = text.split("## P0 Blockers Before Production/GA", 1)[1].split("## P1 Architecture Maturity", 1)[0]
for closed in ["Internal route auth", "Route collision detection"]:
    assert closed not in p0, closed
for blocker in [
    "Transactional outbox/inbox",
    "Typed domain data ownership",
    "Environment profiles and PDP fail-closed",
    "Reproducible toolchain",
]:
    assert blocker in p0, blocker
PY
rg -n "Route auth and collision hardening|Internal route auth|Route collision detection" problem.md
git diff --check -- problem.md docs/plan/2026-06-20-route-auth-collision-problem-ledger-update.md
```

Executed on 2026-06-20:

- Plan reviewed and approved by Reviewer Agent Jason.
- `Route auth and collision hardening` completed row was updated to `Done`.
- `Internal route auth` and `Route collision detection` were removed from P0.
- Remaining P0 blocker assertions passed.
- `rg` confirmed the completed row and absence of closed blockers from P0.
- `git diff --check` passed for the docs-only scope.

## 6. Rollback Plan

Re-add the P0 rows and restore the completed-row status if reviewer or live
evidence is invalidated.

## 7. Status

Status: Approved
