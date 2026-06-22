# API Token Problem Ledger Update

## 1. Objective

Update `problem.md` so the API token verification P0 no longer appears as an
open blocker after the indexed lookup implementation, tests, Sonar, reviewer
approval, and live evidence passed.

## 2. Background

`docs/plan/2026-06-20-api-token-indexed-lookup.md` is approved and implemented.
Keeping `problem.md` stale would make the GA checklist misleading.

## 3. Source References

- `problem.md`
- `docs/plan/2026-06-20-api-token-indexed-lookup.md`

## 4. Assumptions

- Only the API token row is updated.
- Other P0/P1/P2 blockers remain open.

## 5. Non-Goals

- Do not clear `problem.md`.
- Do not edit `docs/acceptance/gap-analysis.md`.
- Do not claim External GA readiness.

## 6. Current Behavior

`problem.md` still says API token verification scans token records.

## 7. Target Behavior

`problem.md` lists API token indexed lookup as completed and removes it from the
P0 blocker table.

## 8. Affected Domains

Documentation and release checklist only.

## 9. Affected Files

- `problem.md`

## 10. API / Contract Changes

None.

## 11. Database / Migration Changes

None.

## 12. Configuration Changes

None.

## 13. Observability Changes

None.

## 14. Security Considerations

The status update must not include raw tokens, secret values, or credentials.

## 15. Implementation Steps

1. Move API token verification from P0 blockers to completed status.
2. Reference the indexed lookup evidence at a high level without secrets.

## 16. Verification Plan

```sh
rg -n "API token verification|API token indexed lookup" problem.md
python3 - <<'PY'
from pathlib import Path

text = Path("problem.md").read_text()
p0 = text.split("## P0 Blockers Before Production/GA", 1)[1].split("## P1 Architecture Maturity", 1)[0]
completed = text.split("## Completed Or In Progress", 1)[1].split("## P0 Blockers Before Production/GA", 1)[0]
assert "API token verification" not in p0
assert "API token indexed lookup" in completed
for expected in [
    "Transactional outbox/inbox",
    "Typed domain data ownership",
    "Internal route auth",
    "Route collision detection",
    "Trusted client IP resolution",
    "Environment profiles and PDP fail-closed",
    "Reproducible toolchain",
]:
    assert expected in p0, expected
PY
git diff --check -- problem.md docs/plan/2026-06-20-api-token-problem-ledger-update.md
```

Executed on 2026-06-20:

```sh
python3 - <<'PY'
from pathlib import Path

text = Path("problem.md").read_text()
p0 = text.split("## P0 Blockers Before Production/GA", 1)[1].split("## P1 Architecture Maturity", 1)[0]
completed = text.split("## Completed Or In Progress", 1)[1].split("## P0 Blockers Before Production/GA", 1)[0]
assert "API token verification" not in p0
assert "API token indexed lookup" in completed
for expected in [
    "Transactional outbox/inbox",
    "Typed domain data ownership",
    "Internal route auth",
    "Route collision detection",
    "Trusted client IP resolution",
    "Environment profiles and PDP fail-closed",
    "Reproducible toolchain",
]:
    assert expected in p0, expected
PY
rg -n "API token verification|API token indexed lookup" problem.md
git diff --check -- problem.md docs/plan/2026-06-20-api-token-problem-ledger-update.md
```

Result: all checks passed. `problem.md` now lists API token indexed lookup as
done and keeps the remaining P0 blockers open.

## 17. Rollback Plan

Revert the docs-only update if the implementation is reverted.

## 18. Risks and Tradeoffs

This updates only one solved blocker; it intentionally leaves the remaining GA
blockers visible.

## 19. Reviewer Checklist

- API token blocker is no longer stale.
- No unrelated blocker is removed.
- No secrets or token values are documented.

## 20. Status

Status: Approved
