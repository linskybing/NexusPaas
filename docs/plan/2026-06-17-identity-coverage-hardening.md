# Identity Coverage Hardening

## 1. Objective

Raise `backend/internal/services/identity` package coverage above the
per-package 80% Production Beta target with focused tests for IAM credential
lifecycle behavior.

## 2. Background

The aggregate Docker-backed integration gate currently passes the 80% coverage
threshold, but `problem.md` still lists per-package coverage gaps as a remaining
Production Beta risk. A fresh package run shows `internal/services/identity` at
approximately 74.2%, below the target for a core IAM service.

## 3. Source References

- `problem.md`: remaining coverage issue calls out `identity`.
- `backend/docs/non-functional-requirements.md`: NFR-MAINT-02 requires at least
  80% coverage of critical paths.
- `backend/docs/e2e-testing.md`: internal identity read/auth contracts require
  `X-Service-Key` and form part of the focused E2E gate.
- `backend/internal/services/identity/workflow_test.go`: existing direct
  identity workflow coverage.
- `backend/internal/services/identity/handler_test.go`: existing internal
  identity read/auth contract coverage.
- `backend/internal/services/identity/api_tokens.go`: API token lifecycle and
  current-token revocation paths.
- `backend/internal/services/identity/internal_read_contracts.go`: internal
  session/API-token authorization contracts.
- `backend/internal/services/identity/oidc_dex.go`: Dex-backed OIDC revocation
  compatibility path.
- `backend/internal/services/identity/users.go`: RequireAuth self-service
  compatibility path for platform-authenticated callers.

## 4. Assumptions

- This PR is test-first and should not change production runtime behavior.
- Per-package coverage is the relevant local metric for this slice:
  `go test ./internal/services/identity -cover`.
- Existing broader service tests already cover some identity behavior from the
  parent `internal/services` package, but those tests do not improve the
  `internal/services/identity` package coverage number.
- No new dependency is needed; the standard library and existing platform test
  helpers are enough.

## 5. Non-Goals

- Do not change public identity APIs, internal contract shapes, auth policy, or
  credential storage.
- Do not add OIDC provider features, service mesh, mTLS, or workload identity.
- Do not touch database migrations or Kubernetes manifests.
- Do not resolve unrelated low-coverage packages in this PR.
- Do not claim GitHub-hosted Sonar enforcement is fixed; repository secrets are
  still a separate launch blocker.

## 6. Current Behavior

- `go test ./internal/services/identity -cover -count=1` reports about 74.2%
  statement coverage.
- Key uncovered or under-covered paths include:
  - current API token revocation via `platform.APITokenID(r)`,
  - internal API token authorization when a token is denylisted,
  - OIDC revocation parameter handling,
  - RequireAuth platform-authenticated self-service fallback,
  - API token metadata/count helper behavior.
- The behavior already works in broader service tests, but identity package
  coverage remains below the Production Beta target.

## 7. Target Behavior

- `go test ./internal/services/identity -cover -count=1` reports at least 80%.
- Tests assert meaningful IAM contracts:
  - an API token created under a session authorizes through middleware,
  - `/api/v1/me/api-tokens/current` cannot be forged with inbound headers,
  - bearer API-token revocation denylist entries are written,
  - internal identity API-token auth rejects denylisted credentials,
  - OIDC revocation accepts token form/header input and rejects missing token,
  - RequireAuth platform-authenticated self-service callers are not treated as
    anonymous when the local user projection is missing.
- `problem.md` updates only the identity coverage evidence while keeping other
  coverage and launch blockers visible.

## 8. Affected Domains

- IAM credential lifecycle tests.
- Internal identity auth contract tests.
- Production Beta readiness tracking.

## 9. Affected Files

- `backend/internal/services/identity/workflow_test.go`
- `problem.md`
- `docs/plan/2026-06-17-identity-coverage-hardening.md`

## 10. API / Contract Changes

None. Existing public and internal identity API contracts remain unchanged.

## 11. Database / Migration Changes

None.

## 12. Configuration Changes

None.

## 13. Observability Changes

None.

## 14. Security Considerations

This PR strengthens tests around credential revocation and internal
service-to-service auth. It must not introduce dev auth bypasses, test-only
production branches, plain credentials in docs, or weaker API-token handling.

## 15. Implementation Steps

1. Add identity package tests for API token middleware/current-token revocation
   using a small `platform.ServiceSpec` route registration so platform auth
   middleware sets `platform.APITokenID(r)` naturally.
2. Add identity package tests for internal API-token auth rejecting a credential
   that remains active in the store but is present in the revocation denylist.
3. Add identity package tests for OIDC revocation parameter handling from form
   and `Authorization: Bearer` header input, plus missing-token rejection.
4. Add identity package tests for RequireAuth platform-authenticated self-service
   fallback when the local identity user projection is missing.
5. Add limited helper assertions for API token metadata/count behavior if needed
   to reach the threshold, ensuring they assert no secret leakage and active-only
   counting.
6. Update `problem.md` with the new identity package coverage evidence while
   preserving the remaining low-coverage packages as unresolved.

## 16. Verification Plan

- `gofmt -w backend/internal/services/identity/workflow_test.go`
- `go test ./internal/services/identity -cover -count=1`
- `go test ./internal/services/identity -coverprofile=/tmp/identity.cover -count=1`
- `go tool cover -func=/tmp/identity.cover`
- `go test ./internal/services/identity ./internal/services -count=1`
- `go test ./... -count=1`
- `go vet ./...`
- `go build ./...`
- `git diff --check`
- `bash backend/scripts/ci-security-gate.sh security`
- `bash backend/scripts/ci-security-gate.sh sonar`

Docker-backed E2E is not expected to be required locally because this PR only
adds in-package tests and problem tracking. GitHub Backend Quality Gate will run
the Docker-backed E2E and coverage gate after PR creation.

## 17. Rollback Plan

Revert this PR. Runtime behavior is unaffected because the change only adds
tests and updates readiness tracking.

## 18. Risks and Tradeoffs

- Tests that route through platform middleware are slightly more coupled to
  route registration than direct handler tests, but that coupling is intentional
  for the current-token revocation path.
- The PR closes only the identity package coverage gap; other packages listed in
  `problem.md` remain follow-up work.
- Local Sonar can pass while GitHub-hosted Sonar remains skipped until repository
  secrets are provisioned.

## 19. Reviewer Checklist

- Tests cover IAM behavior rather than only small helper functions.
- No runtime behavior, API shape, config, migration, or deployment manifest is
  changed.
- Identity package coverage is at least 80%.
- Broader identity/service tests still pass.
- `problem.md` records the new evidence without hiding remaining blockers.

## 20. Status

Status: Approved
