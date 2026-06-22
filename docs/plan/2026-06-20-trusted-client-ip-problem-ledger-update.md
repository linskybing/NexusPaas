# Trusted Client IP Problem Ledger Update

## 1. Objective

Update `problem.md` after the trusted client IP resolver slice passed code
review, local gates, Sonar Quality Gate, and live RKE2 evidence.

## 2. Background

`problem.md` still lists `Trusted client IP resolution` as a P0 GA blocker.
The implementation plan `docs/plan/2026-06-20-trusted-client-ip-resolver.md`
is now approved and records live evidence:

- shared platform resolver exported and reused by identity;
- identity login failure, captcha, cleanup, and API-token audit paths use the
  app-aware trusted proxy resolver;
- focused tests, full backend tests, quick gate, and Sonar Quality Gate passed;
- live RKE2 rollout completed on image
  `localhost:5000/nexuspaas-backend:ci-ga-ip-20260620134150`;
- live spoofed `X-Forwarded-For` login test stored `ip=127.0.0.1`, not the
  spoofed forwarded value.

## 3. Scope

- Move the trusted client IP item from `P0 Blockers Before Production/GA` to
  `Completed Or In Progress`.
- Keep remaining P0 blockers unchanged.
- Do not edit code.
- Do not claim all GA blockers are closed.

## 4. Implementation Steps

1. Add a `Trusted client IP resolution | Done | ...` row to
   `Completed Or In Progress`.
2. Remove the matching `Trusted client IP resolution` P0 blocker row.
3. Verify that the completed row exists, the P0 row is absent, and the other P0
   blocker rows remain.

## 5. Verification Plan

```sh
python3 - <<'PY'
from pathlib import Path
text = Path("problem.md").read_text()
assert "| Trusted client IP resolution | Done |" in text
p0 = text.split("## P0 Blockers Before Production/GA", 1)[1].split("## P1 Architecture Maturity", 1)[0]
assert "Trusted client IP resolution" not in p0
for blocker in [
    "Transactional outbox/inbox",
    "Typed domain data ownership",
    "Internal route auth",
    "Route collision detection",
    "Environment profiles and PDP fail-closed",
    "Reproducible toolchain",
]:
    assert blocker in p0, blocker
PY
rg -n "Trusted client IP resolution|API token indexed lookup" problem.md
git diff --check -- problem.md docs/plan/2026-06-20-trusted-client-ip-problem-ledger-update.md
```

Executed on 2026-06-20:

- Plan reviewed and approved by Reviewer Agent Mencius.
- The completed ledger row was added.
- The P0 blocker row was removed.
- The remaining P0 blocker assertions passed.
- `rg` confirmed the trusted client IP and API token completed rows.
- `git diff --check` passed for the docs-only scope.

## 6. Rollback Plan

Re-add the P0 row if reviewer or live evidence is invalidated.

## 7. Status

Status: Approved
