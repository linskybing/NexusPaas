# Backend Lint And Maintenance Task Unblock

## 1. Objective

Restore the backend lint gate and keep the co-hosted maintenance-task isolation
test aligned with the currently registered `harbor-catalog-sync` task.

## 2. Background

`make -C backend lint` was blocked by gofmt drift in
`backend/internal/services/service_isolation_test.go`. That same dirty slice
also contains the expected `harbor-catalog-sync` maintenance task. Current
production code registers that task through `imageregistry.Register`, so this
plan treats the expectation as part of the test alignment rather than a
format-only diff.

## 3. Source References

- `backend/Makefile`
- `backend/internal/services/service_isolation_test.go`
- `backend/internal/services/imageregistry/handler.go`
- `backend/internal/services/imageregistry/harbor_catalog_sync.go`

## 4. Assumptions

- `harbor-catalog-sync` should be present when `SERVICE_NAME=all` because
  `imageregistry.Register` calls `registerHarborCatalogSync`.
- gofmt is the repo-native formatting fix.

## 5. Non-Goals

- Do not change production service registration.
- Do not refactor maintenance-task registration.
- Do not update GA tracker status for unrelated acceptance criteria.

## 6. Current Behavior

The co-hosted app registers `harbor-catalog-sync`. The service-isolation test
expectation includes that task but was not gofmt-formatted, blocking backend
lint.

## 7. Target Behavior

The test expectation includes `harbor-catalog-sync`, the file is
gofmt-formatted, and backend lint/build pass.

## 8. Affected Domains

Backend service-isolation test coverage.

## 9. Affected Files

- `backend/internal/services/service_isolation_test.go`
- `docs/plan/2026-06-22-backend-lint-unblock.md`

## 10. API / Contract Changes

None.

## 11. Database / Migration Changes

None.

## 12. Configuration Changes

None.

## 13. Observability Changes

None.

## 14. Security Considerations

No secrets or trust-boundary behavior are touched.

## 15. Implementation Steps

1. Keep `harbor-catalog-sync` in the co-hosted maintenance-task expectation.
2. Run gofmt on `backend/internal/services/service_isolation_test.go`.
3. Do not change production code.

## 16. Verification Plan

- `test -z "$(gofmt -l backend/internal/services/service_isolation_test.go)"`
- `cd backend && go test ./internal/services -run TestRegisterAllCoHostedOwnsAllMaintenanceSideEffects -count=1`
- `make -C backend lint`
- `make -C backend build`
- `git diff --check -- backend/internal/services/service_isolation_test.go docs/plan/2026-06-22-backend-lint-unblock.md`

Sonar is not rerun for this narrow test-expectation/lint-unblock slice. The
broader GA quality gate remains covered by existing Sonar/coverage evidence and
should be rerun for release-candidate bundles.

## 17. Rollback Plan

Revert the service-isolation test diff and this plan.

## 18. Risks and Tradeoffs

The test expectation is functional. The risk is limited to service-isolation
coverage because production registration already includes `harbor-catalog-sync`.

## 19. Reviewer Checklist

| Item | Status |
|---|---|
| `harbor-catalog-sync` expectation matches current registration | Passed |
| No production/API/config/db/dependency change | Passed |
| `make -C backend lint` passes | Passed |
| `make -C backend build` passes | Passed |

## 20. Status

Status: Approved

## 21. Verification Evidence

- `test -z "$(gofmt -l backend/internal/services/service_isolation_test.go)"`
  passed.
- `cd backend && go test ./internal/services -run TestRegisterAllCoHostedOwnsAllMaintenanceSideEffects -count=1`
  passed.
- `make -C backend lint` passed.
- `make -C backend build` passed.
- `git diff --check -- backend/internal/services/service_isolation_test.go docs/plan/2026-06-22-backend-lint-unblock.md`
  passed.

## 22. Reviewer Decision

Reviewer `Mendel` requested this scope correction because the diff includes a
functional expectation for `harbor-catalog-sync`, not only formatting.
After the plan was corrected, `Mendel` approved the implementation. The only
remaining note is that untracked plan/tracker docs must be included in final
handoff/commit scope.
