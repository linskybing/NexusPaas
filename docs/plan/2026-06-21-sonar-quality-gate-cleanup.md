# Sonar Quality Gate Cleanup Slice

## 1. Objective

Make the local SonarScanner Quality Gate green again so the Web GUI image/usage
slice can receive final approval and GA ledgers can stop carrying a false
Sonar-clean claim.

## 2. Background

Reviewer blocked final approval for
`docs/plan/2026-06-21-web-gui-image-usage-contract.md` because
`bash backend/scripts/ci-security-gate.sh sonar` ran against the configured
local SonarQube server and failed:

- `new_coverage=79.6` below the 80 threshold.
- `new_violations=15` above the 0 threshold.

The Web GUI code itself is frontend-only and Sonar currently scans `backend`
only, so this cleanup targets the backend Sonar issues introduced by the
current GA worktree.

## 3. Source References

- `docs/plan/2026-06-21-web-gui-image-usage-contract.md`
- `gap.md`
- `problem.md`
- `sonar-project.properties`
- `backend/scripts/ci-security-gate.sh`
- Sonar API query:
  `/api/issues/search?componentKeys=nexuspaas-backend&resolved=false&createdAfter=2026-06-21T00:00:00%2B0800`
- Sonar API query:
  `/api/measures/component_tree?component=nexuspaas-backend&metricKeys=new_coverage,new_uncovered_lines,new_lines_to_cover`

## 4. Assumptions

- The Sonar Quality Gate must be fixed, not bypassed.
- Fixing the 15 reported new violations is enough to clear `new_violations`.
- Adding a few focused backend tests against already-changed code is enough to
  move `new_coverage` from 79.6 to at least 80.
- If Sonar reports a different issue set after fixes, update this plan evidence
  and address only the newly reported gate blockers needed for green.

## 5. Non-Goals

- No Quality Gate threshold changes.
- No broad Sonar exclusions.
- No frontend changes except keeping generated build artifacts ignored.
- No behavior changes beyond tiny refactors that preserve existing tests.
- No large cleanup of old non-new Sonar issues unless they remain gate blockers.

## 6. Current Behavior

All local Go tests, frontend tests/build, quick gate, and live `/ui/` smoke pass.
Sonar Quality Gate fails on new coverage and new violations.

## 7. Target Behavior

`bash backend/scripts/ci-security-gate.sh sonar` exits 0 and Sonar reports:

- `new_coverage >= 80`
- `new_violations = 0`
- new duplicated-line density remains under threshold

## 8. Affected Domains

- Platform shared runtime helpers.
- Audit compliance service helpers/tests.
- Authorization policy helpers.
- Image registry handler helpers.
- Org/project helpers/repository.
- Scheduler quota handlers.
- Storage helper.

No service boundary, API, database, deployment, or config ownership changes are
planned.

## 9. Affected Files

Reported new-violation files:

- `backend/internal/platform/crud.go`
- `backend/internal/platform/store_postgres.go`
- `backend/internal/services/auditcompliance/handler.go`
- `backend/internal/services/auditcompliance/handler_test.go`
- `backend/internal/services/authorizationpolicy/assignments.go`
- `backend/internal/services/authorizationpolicy/raw_permission_repository.go`
- `backend/internal/services/imageregistry/handler.go`
- `backend/internal/services/orgproject/handler.go`
- `backend/internal/services/orgproject/project_helpers.go`
- `backend/internal/services/orgproject/project_repository.go`
- `backend/internal/services/schedulerquota/handler.go`
- `backend/internal/services/storage/helpers.go`

Likely coverage-only test additions, limited to the smallest files needed:

- `backend/internal/platform/response_test.go` or existing platform tests.
- `backend/internal/platform/input_limits_test.go` or existing platform tests.
- `backend/internal/services/schedulerquota/admission_resources_test.go` or
  existing schedulerquota tests.

Checklist/docs:

- `docs/plan/2026-06-21-sonar-quality-gate-cleanup.md`
- `docs/plan/2026-06-21-web-gui-image-usage-contract.md`
- `gap.md`
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

- Do not suppress Kubernetes/security-related Sonar findings with exclusions.
- Preserve existing auth/RBAC behavior and tests.
- Do not log or expose Sonar tokens, runtime API keys, or Kubernetes secrets in
  evidence.

## 15. Implementation Steps

- [x] Remove or ignore generated frontend build-info artifacts from final status.
- [x] Fix duplicated literal issues with local constants only where Sonar
  reported them:
  - `"grouping policy update skipped"`
  - `"group not found"`
  - `"project member not found"`
- [x] Reduce cognitive complexity in the reported functions by extracting small
  private helpers or early-return blocks while preserving behavior:
  - `crud.go:308`
  - `store_postgres.go:241`
  - `auditcompliance/handler.go:484`
  - `auditcompliance/handler_test.go:28`
  - `authorizationpolicy/assignments.go:65`
  - `imageregistry/handler.go:185`
  - `imageregistry/handler.go:244`
  - `orgproject/handler.go:528`
  - `orgproject/project_repository.go:176`
  - `schedulerquota/handler.go:164`
  - `schedulerquota/handler.go:323`
  - `storage/helpers.go:219`
- [x] Add the fewest focused tests needed to raise new coverage above 80.
- [x] Run focused package tests for touched backend packages.
- [x] Run full backend tests, coverage, quick gate, Sonar gate, and diff checks.
- [x] Update `problem.md`, `gap.md`, and related plan evidence only after Sonar
  is green.
- [x] Resubmit both the Sonar cleanup and the Web GUI image/usage slice to
  Reviewer.

## 16. Verification Plan

Focused tests:

```sh
go -C backend test ./internal/platform ./internal/services/auditcompliance ./internal/services/authorizationpolicy ./internal/services/imageregistry ./internal/services/orgproject ./internal/services/schedulerquota ./internal/services/storage -count=1
```

Repository gates:

```sh
go -C backend test ./... -count=1
go -C backend test ./... -coverprofile=coverage.out -count=1
bash backend/scripts/ci-security-gate.sh quick
bash backend/scripts/ci-security-gate.sh sonar
git diff --check
```

Sonar proof:

```sh
curl -fsS -u "$SONAR_TOKEN:" \
  "$SONAR_HOST_URL/api/qualitygates/project_status?projectKey=nexuspaas-backend"
```

Implementation evidence captured on 2026-06-21:

- `go -C backend test ./internal/platform ./internal/services/auditcompliance ./internal/services/authorizationpolicy ./internal/services/imageregistry ./internal/services/orgproject ./internal/services/schedulerquota ./internal/services/storage -count=1` passed.
- Reviewer-requested org/project tx-miss regression test added and
  `go -C backend test ./internal/services/orgproject -count=1` passed.
- `go -C backend test ./... -count=1` passed.
- `go -C backend test ./... -coverprofile=coverage.out -count=1` passed.
- `bash backend/scripts/ci-security-gate.sh quick` passed.
- `bash backend/scripts/ci-security-gate.sh sonar` passed.
- Sonar API readback: `new_coverage=81.2`, `new_violations=0`,
  `new_duplicated_lines_density=0.35803`, `ignoredConditions=false`.
- `git diff --check` passed.

## 17. Rollback Plan

Revert this cleanup slice. No migrations or runtime rollouts are introduced by
this plan.

## 18. Risks and Tradeoffs

- Refactoring many reported functions in one slice is broader than ideal, but
  Sonar reports one gate-blocking set and the shortest path to green is to fix
  that exact set.
- Coverage may still fail after the first focused tests; if so, add only the
  next-smallest uncovered test target reported by Sonar.

## 19. Reviewer Checklist

| Check | Status |
|---|---|
| Requirement fit: Sonar Quality Gate green | Pass |
| Scope limited to reported gate blockers | Pass |
| No threshold/exclusion bypass | Pass |
| Behavior preserved by tests | Pass |
| SOLID compliance | Pass |
| 12-Factor compliance | Pass |
| Focused/full tests | Pass |
| Quick gate | Pass |
| Sonar Quality Gate | Pass |
| Ledger accuracy | Pass |
| Diff scope | Pass |

## 20. Status

Status: Approved (final Reviewer verification received)

Plan Agent checklist:

- [x] Current Sonar failures captured.
- [x] Non-goals reject bypassing the gate.
- [x] Verification requires a green Sonar Quality Gate.
- [x] Reviewer Agent approval received.
- [x] Code Agent implementation complete.
- [x] Reviewer Agent final implementation approval received.
