# Sonar Quality Gate Remediation

## 1. Objective

Bring the current `nexuspaas-backend` Sonar Quality Gate to passing by fixing the issues reported by the latest authorized SonarScanner run and raising new-code coverage above the configured 80% threshold.

## 2. Background

The user explicitly approved uploading Sonar analysis data. `sonar-scanner -Dsonar.qualitygate.wait=true` completed analysis upload but failed the Quality Gate.

Latest Quality Gate failures:

- `new_coverage`: 62.0%, threshold 80%.
- `new_violations`: 6, threshold 0.

Latest unresolved Sonar issues:

- `go:S3776`: high cognitive complexity in `backend/internal/e2e/image_build_governance_e2e_test.go`.
- `go:S107` and `go:S3776`: too many parameters and cognitive complexity in `backend/internal/e2e/live_configfile_dra_e2e_test.go`.
- `go:S3776`: high cognitive complexity in `backend/internal/e2e/workload_configfile_lifecycle_e2e_test.go`.
- `go:S1192`: duplicated `"X-Service-Key"` literal in `backend/internal/platform/service_client.go`.
- `go:S1192`: duplicated `"/settings"` literal in `backend/internal/services/identity/spec.go`.
- `secrets:S6698`: hard-coded PostgreSQL password in `backend/deploy/local/docker-compose.yml`.

## 3. Source References

- `sonar-project.properties`
- `.scannerwork/report-task.txt`
- `backend/coverage.out`
- `backend/internal/e2e/image_build_governance_e2e_test.go`
- `backend/internal/e2e/live_configfile_dra_e2e_test.go`
- `backend/internal/e2e/workload_configfile_lifecycle_e2e_test.go`
- `backend/internal/platform/service_client.go`
- `backend/internal/services/identity/spec.go`
- `backend/deploy/local/docker-compose.yml`
- `docs/agents/workflow.md`
- `docs/agents/planning.md`

## 4. Assumptions

- The Quality Gate is evaluated against new code since the previous project version baseline.
- The E2E files are included as tests by Sonar but still counted for maintainability rules.
- Fixing the reported maintainability issues and adding focused tests for low-covered new production helpers should be enough to pass `new_violations` and `new_coverage`.
- The local Docker Compose password is dev-only, but Sonar treats committed default passwords as vulnerabilities; replacing it with a required environment variable is acceptable.

## 5. Non-Goals

- Do not perform broad style rewrites or speculative optimization.
- Do not change public API routes, database schema, service ownership, or Kubernetes behavior.
- Do not suppress Sonar issues without code/config remediation unless a false positive is proven.
- Do not alter unrelated dirty worktree changes.

## 6. Current Behavior

- Sonar Quality Gate fails.
- E2E tests contain long scenario functions that exceed Sonar cognitive-complexity thresholds.
- The DRA live E2E validation helper takes 8 parameters.
- Repeated string literals trigger Sonar duplicate-literal rules.
- Local Docker Compose contains committed default PostgreSQL passwords.
- New-code coverage is below 80%.

## 7. Target Behavior

- Sonar Quality Gate passes with `new_coverage >= 80%`, `new_violations = 0`, and no blocker vulnerability.
- E2E scenario behavior remains unchanged but is decomposed into small helper functions.
- Service key and identity settings path literals are centralized.
- Local Docker Compose requires password values from environment instead of committing defaults.
- Focused tests cover new low-coverage production helpers without adding heavy fixtures.

## 8. Affected Domains

- E2E test harness.
- Platform internal JSON/service client.
- Identity service catalog metadata.
- Local development backing services.
- Sonar/CI verification.

## 9. Affected Files

- `backend/internal/e2e/image_build_governance_e2e_test.go`
- `backend/internal/e2e/live_configfile_dra_e2e_test.go`
- `backend/internal/e2e/workload_configfile_lifecycle_e2e_test.go`
- `backend/internal/platform/service_client.go`
- `backend/internal/platform/internal_json_client_test.go`
- `backend/internal/services/identity/spec.go`
- `backend/internal/services/catalog_test.go` or package-level spec tests as needed.
- `backend/internal/services/shared/spec.go`
- `backend/internal/services/shared/shared_test.go`
- `backend/deploy/local/docker-compose.yml`
- `backend/docs/e2e-testing.md` if local compose environment examples need updating.

## 10. API / Contract Changes

No public API contract changes.

Test helper signatures may change inside E2E test files only.

## 11. Database / Migration Changes

No database or migration changes.

## 12. Configuration Changes

`backend/deploy/local/docker-compose.yml` will no longer commit a default PostgreSQL password. Local users must provide a password environment variable when starting the compose stack.

Documentation examples will be updated if needed so local setup remains explicit.

## 13. Observability Changes

No runtime observability changes.

SonarScanner will be rerun after remediation and the Quality Gate result will be reported.

## 14. Security Considerations

- Remove committed PostgreSQL password defaults from local Docker Compose.
- Do not print `SONAR_TOKEN` or other secrets.
- Do not weaken Sonar secret scanning or exclude the affected compose file from security analysis.

## 15. Implementation Steps

1. Refactor the three Sonar-flagged E2E functions into smaller helpers while preserving assertions and route coverage.
2. Replace the 8-parameter DRA helper with a small options struct and split validation branches enough to satisfy complexity.
3. Add a platform constant for `"X-Service-Key"` and reuse it in the internal JSON client.
4. Add an identity spec constant for the user settings path and reuse it.
5. Replace local Docker Compose default PostgreSQL password literals with required environment references and update local setup docs/examples.
6. Add focused tests for new low-coverage production helper files introduced by the audit cleanup, prioritizing `shared/spec.go` and service `Spec()` metadata aggregation.
7. Regenerate `backend/coverage.out`, rerun SonarScanner with `sonar.qualitygate.wait=true`, and iterate only on remaining reported issues.

## 16. Verification Plan

- `go test ./... -coverprofile=coverage.out`
- `go test -tags e2e ./internal/e2e -run 'TestImageBuildGovernanceE2E|TestLiveK8sConfigFileDRADispatchE2E|TestWorkloadConfigFileLifecycleE2E' -count=1 -v`
- `go vet ./...`
- `go build ./...`
- `bash backend/scripts/ci-security-gate.sh quick`
- `sonar-scanner -Dsonar.qualitygate.wait=true`

## 17. Rollback Plan

Revert the Sonar remediation patch. The previous behavior remains available because no database, API, or runtime contract migration is involved.

## 18. Risks and Tradeoffs

- Refactoring E2E tests may hide scenario flow if over-split; keep helper names concrete and local to the test file.
- Requiring an explicit local Postgres password is slightly less convenient but avoids committed secret defaults.
- New-code coverage may still fail if Sonar's baseline includes other changed files; if so, add narrow tests for the exact remaining low-coverage new files rather than broad exclusions.

## 19. Reviewer Checklist

- Plan scope matches the latest Sonar failures.
- No unrelated optimization or broad refactor is included.
- Security issue is remediated without suppressing the rule.
- Coverage remediation uses tests, not exclusions.
- Verification includes a final Sonar Quality Gate wait.

## 20. Status

Status: Approved
