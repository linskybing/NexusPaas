# PR2 Supersession Coverage

## 1. Objective

Make remote PR #2 safe to supersede by preserving its remaining useful test
coverage in the current Production Beta launch-hardening stack.

## 2. Background

Remote PR #2 added the first Docker-backed verification gate. The current stack
now has the stronger `backend/scripts/ci-security-gate.sh docker` and
`beta-rc` gates, including Docker-backed migrations, integration coverage,
focused E2E, full non-live E2E, runtime smoke, security scans, Sonar, and RC
reporting. PR #2 is therefore no longer the right merge vehicle, but it also
contains small unit-test additions that are not present in the current stack.

## 3. Target Behavior

The current stack keeps PR #2's remaining test value without reintroducing the
obsolete standalone Docker gate or older problem-tracking text.

## 4. Scope

- Add Kubernetes manifest creation validation for remaining native objects and
  invalid-manifest branches.
- Add RWX PVC sharing helper tests for env parsing, mount option validation,
  JuiceFS target validation, and Longhorn endpoint failures.
- Add scheduler-quota workload eviction client tests for local and remote
  contract clients.
- Do not add `backend/scripts/docker-e2e-gate.sh`; the canonical runner remains
  `backend/scripts/ci-security-gate.sh docker`.
- Do not change runtime behavior, API contracts, deployments, or migrations.

## 5. Affected Files

- `backend/internal/platform/cluster/apply_test.go`
- `backend/internal/platform/cluster/volume_share_test.go`
- `backend/internal/services/schedulerquota/eviction_client_test.go`
- this plan file

## 6. Verification Plan

- `cd backend && gofmt -w internal/platform/cluster/apply_test.go internal/platform/cluster/volume_share_test.go internal/services/schedulerquota/eviction_client_test.go`
- `cd backend && go test ./internal/platform/cluster ./internal/services/schedulerquota -count=1`
- `cd backend && go test ./... -count=1`
- Re-run the release gate if the test additions expose any current-stack issue.

## 7. Rollback Plan

Revert this test-only patch. It does not alter runtime code, deployments,
database schemas, or persisted state.

## 8. Risks

- The tests are intentionally copied at the behavioral level from PR #2, but the
  current stack has moved since that branch. If a test no longer matches current
  behavior, update the test to the current contract rather than weakening the
  implementation.

## 9. Status

Status: Approved and verified locally
