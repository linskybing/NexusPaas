# Problem Main Readiness Update

## 1. Objective

Update `problem.md` after the Production Beta readiness stack was squash-merged
into `main`, so the repository's primary problem review reflects the current
mainline evidence and remaining launch blockers.

## 2. Background

PRs #3 through #7 have been squash-merged into `main`, PR #2 was closed as
superseded after its remaining useful tests were preserved, and the final
`beta-rc` gate passed on main commit `d01fc55`. `problem.md` still describes a
feature branch and includes stale review wording from before the merge.

## 3. Scope

- Update the generated branch/status text to refer to `main`.
- Update verification rows to reflect the final main `beta-rc` evidence:
  integration coverage 80.5%, 15-service runtime smoke with no 5xx, Trivy clean,
  and Sonar Quality Gate passed on `d01fc55`.
- Remove stale references to unignored `.e2e-gate` artifacts.
- Keep unresolved launch blockers visible, especially live staging rehearsal,
  missing reference snapshot, missing `function.md`, live observability
  provisioning, per-package coverage gaps, and remaining shared physical
  Postgres transition debt.
- Do not change code, deployment manifests, scripts, or runtime behavior.

## 4. Affected Files

- `problem.md`
- this plan file

## 5. Verification Plan

- Review the markdown diff for factual accuracy against the latest merge and
  `beta-rc` report.
- `git diff --check`

## 6. Rollback Plan

Revert this docs-only patch. It does not affect runtime behavior, deployment,
database state, or CI gates.

## 7. Status

Status: Approved and verified locally
