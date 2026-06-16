# CI Security Quality Gate

## 1. Objective

Add a reproducible backend quality gate for the Production Beta roadmap so CI
and local reviewers can run the same checks for formatting, build correctness,
tests, coverage, focused E2E, vulnerability scanning, container image scanning,
and SonarScanner Quality Gate.

## 2. Background

The Production Beta plan requires every PR to be blocked by a repeatable
quality gate instead of relying on ad-hoc local commands. The repository already
has Go unit/integration/E2E tests, a backend Dockerfile, Sonar configuration,
and Docker-backed E2E guidance. It does not yet have a GitHub Actions workflow
or a single local command that composes the required checks.

PR #2 adds the Docker E2E smoke runner and PR #3 adds the 15-service
production-beta topology. This PR is intentionally stacked on PR #3 while both
earlier PRs remain open. After PR #2 and PR #3 are squash-merged, this branch
should be rebased or retargeted before merging.

## 3. Source References

- `long-term.md`
- `problem.md`
- `AGENTS.md`
- `docs/agents/workflow.md`
- `docs/agents/planning.md`
- `docs/agents/review-checklist.md`
- `backend/docs/e2e-testing.md`
- `backend/Dockerfile`
- `backend/go.mod`
- `backend/go.sum`
- `backend/deploy/local/docker-compose.yml`
- `sonar-project.properties`
- `.github/workflows/backend-quality-gate.yml`
- `backend/scripts/ci-security-gate.sh`

## 4. Assumptions

- The PR remains stacked on `feature/production-beta-kustomize` until PR #2 and
  PR #3 are merged or their overlapping identity-store changes are resolved.
- CI runs on GitHub-hosted Ubuntu runners with Docker available.
- Docker commands use a gate-owned temporary Docker config for public image
  pulls. Trivy uses the same isolated Docker config plus a gate-owned cache and
  timeout, so local credential helpers cannot make the gate hang or fail
  nondeterministically.
- Go module commands run from `backend/` because the repository root is not a
  Go module.
- OSV scans the repository root so it can inspect workflow, Docker, and module
  manifests; `govulncheck` runs from `backend/` against Go packages.
- `SONAR_TOKEN` and `SONAR_HOST_URL` are supplied as repository secrets before
  the workflow is made a required branch-protection check.
- Local developers may run the gate without Sonar secrets by using an explicit
  skip flag, but CI must fail when required production-beta gates are missing.
- Container image scanning can scan the locally built backend image; publishing
  images, SBOM signing, and Cosign attestation are later hardening work.

## 5. Non-Goals

- Do not merge PR #2 or PR #3.
- Do not change service APIs, event contracts, or database schemas.
- Do not add new runtime dependencies to the backend binary.
- Do not introduce a new build system such as Make, Bazel, or Task.
- Do not require live Kubernetes cluster mutation.
- Do not perform broad dependency modernization. Security-driven module changes
  are limited to the dependencies and Go directive required by the scanners.
- Do not solve broad coverage debt in unrelated packages. If the new gate
  exposes the existing sub-80% total coverage blocker, only add focused tests
  for critical authorization-policy paths needed to make the gate pass.

## 6. Current Behavior

Reviewers manually run `go build`, `go vet`, `go test`, selected integration
tests, selected E2E tests, vulnerability scanners, image scanning, and
SonarScanner. The repository has no `.github/workflows` quality gate and no
single local script that standardizes command order, required environment, or
failure behavior.

## 7. Target Behavior

The repository provides:

- a GitHub Actions workflow for backend quality gates on PRs and pushes
- a local script that CI and developers both use
- Docker-backed Postgres, Redis, and fallback MinIO ports that avoid Sonar
  `localhost:9000`
- explicit coverage threshold enforcement at 80%
- focused E2E gate checks that fail when required tests skip
- full non-live E2E package execution after the focused gate
- vulnerability scanning with `govulncheck` and OSV
- backend container build plus Trivy image scan
- SonarScanner execution with Quality Gate wait enabled

## 8. Affected Domains

- CI/CD quality gate
- Security scanning
- Test orchestration
- Backend release readiness documentation

## 9. Affected Files

- `.github/workflows/backend-quality-gate.yml`
- `backend/scripts/ci-security-gate.sh`
- `backend/Dockerfile`
- `backend/go.mod`
- `backend/go.sum`
- `backend/docs/e2e-testing.md`
- `backend/internal/e2e/*_test.go`
- `backend/internal/platform/deployment_test.go`
- `backend/internal/services/authorizationpolicy/*_test.go`
- `docs/plan/2026-06-17-ci-security-quality-gate.md`

## 10. API / Contract Changes

No HTTP API, event contract, or service-to-service contract changes.

## 11. Database / Migration Changes

No database or migration changes. The gate applies existing migrations as part
of integration/E2E verification only.

## 12. Configuration Changes

Add CI-only configuration through workflow environment variables:

- isolated Postgres/Redis/MinIO ports
- `CI_GATE_DOCKER_CONFIG`
- `CI_GATE_TRIVY_CACHE_DIR`
- `TRIVY_TIMEOUT`
- `TEST_DATABASE_URL`
- `TEST_REDIS_URL`
- `TEST_EVENT_BUS_URL`
- `TEST_OBJECT_STORE_*`
- pinned scanner/tool version variables:
  - `GO_VERSION=1.25.11`
  - `GOVULNCHECK_VERSION=v1.3.0`
  - `OSV_SCANNER_VERSION=v2.0.2`
  - `TRIVY_VERSION=0.71.1`
  - `SONAR_SCANNER_VERSION=7.2.0.5079`
- Sonar host/token environment
- module metadata/dependency security updates:
  - `go 1.25.11`
  - `github.com/Azure/go-ntlmssp v0.1.1`
  - `golang.org/x/crypto v0.53.0` and its required `x/*` companion updates

No production runtime config changes.

## 13. Observability Changes

No runtime telemetry changes. CI must upload or retain coverage, scanner, and
test logs as workflow artifacts where practical so failed gates are diagnosable.

## 14. Security Considerations

- Do not commit scanner tokens, Sonar tokens, service keys, or cloud
  credentials.
- Use GitHub secrets for SonarScanner.
- Keep Docker-backed test credentials local to the CI runner.
- Fail CI when vulnerability or image scanners find blocking issues.
- Pin CI and container build toolchains to Go 1.25.11 so govulncheck does not
  fail on known standard-library vulnerabilities fixed after Go 1.25.5.
- Keep the runtime image package set upgraded during build when the image scan
  reports fixed HIGH/CRITICAL Alpine packages.
- Keep production dev-auth restrictions covered by existing tests rather than
  relaxing them for CI convenience.

## 15. Implementation Steps

1. Add `backend/scripts/ci-security-gate.sh` with strict shell options and
   subcommands or environment flags for local/CI execution. The script resolves
   `REPO_ROOT` and `BACKEND_DIR`, and every Go package command runs from
   `BACKEND_DIR`.
2. In the script, run `gofmt` check, `go vet`, `go test ./... -count=1`, and
   `go build ./...` from `backend/`.
3. Start isolated Docker-backed Postgres, Redis, and MinIO on non-Sonar ports
   when integration/E2E gates are enabled. Use a gate-owned Docker config
   directory for public image pulls.
4. Run migration apply/validate and object-store bucket provisioning through
   existing `ADMIN_TASK` entry points.
5. Run integration tests with `-tags integration`, write `coverage.out`, and
   fail when total coverage is below 80%.
6. Run the focused E2E command from `backend/docs/e2e-testing.md` and fail when
   required tests skip.
7. Run full non-live `go test -tags e2e ./internal/e2e -count=1 -v` after the
   focused E2E gate; live cluster tests may skip only through their existing
   explicit opt-in environment guards.
8. Build the backend container image and run Trivy image scan against it.
   Trivy uses a gate-owned cache/config path and finite timeout.
9. Run `govulncheck ./...` from `backend/` and `osv-scanner scan source -r .`
   from the repository root.
10. Run SonarScanner with Quality Gate wait when Sonar env is present or required.
   In CI, Sonar is required on `push`, `workflow_dispatch`, and non-fork
   `pull_request` events; missing `SONAR_TOKEN` or `SONAR_HOST_URL` must fail
   those runs with a clear error. For fork PRs where secrets are unavailable,
   the workflow records Sonar as not run and leaves branch protection to require
   the non-fork/default branch gate.
11. Add `.github/workflows/backend-quality-gate.yml` that installs pinned
    versions of Go/security tools, invokes the local script, and uploads
    coverage/log artifacts.
12. Pin `backend/Dockerfile` to the same patched Go build image used by CI and
    apply scanner-required Alpine package upgrades in the runtime stage.
13. Update `backend/go.mod` and `backend/go.sum` only as needed for the
    security scanners to stop reporting known Go directive or module
    vulnerabilities.
14. Update `backend/docs/e2e-testing.md` with the new one-command quality gate.
15. If the coverage threshold fails on existing code, add focused
    authorization-policy tests for role, policy, and assignment critical paths
    rather than lowering the threshold.
16. If the new gate exposes E2E data-isolation defects when focused and full
    non-live E2E run against the same backing services, fix only the test data
    uniqueness/cleanup issue required for repeatable gate execution.

## 16. Verification Plan

- `bash -n backend/scripts/ci-security-gate.sh`
- `ruby -e 'require "yaml"; YAML.load_file(".github/workflows/backend-quality-gate.yml")'`
- `bash backend/scripts/ci-security-gate.sh quick`
- `bash backend/scripts/ci-security-gate.sh docker`
- `bash backend/scripts/ci-security-gate.sh security`
- `bash backend/scripts/ci-security-gate.sh sonar` when local Sonar env is
  configured
- `cd backend && go test ./... -count=1`
- `cd backend && go vet ./...`
- `cd backend && go build ./...`

If security scanners or SonarScanner are not installed locally, install them in
an isolated tool directory during the script run or record the missing local
tool as a blocker.

## 17. Rollback Plan

Delete the workflow, local gate script, and related documentation, and either
revert the Dockerfile build-image pin, scanner-required Alpine runtime package
upgrade, and module security updates or explicitly retain them as separate
toolchain/dependency/image hardening decisions. This returns the repository to
manual verification without changing runtime code, APIs, or data.

## 18. Risks and Tradeoffs

- A strict CI gate may initially fail until repository secrets and scanner
  availability are configured.
- Full Docker-backed gates increase CI runtime but make readiness reproducible.
- This PR does not resolve existing service-boundary or observability debt; it
  only makes those risks visible and harder to regress.
- Stacking on PR #3 avoids editing main directly but means the PR must be
  rebased or retargeted after PR #2/#3 land.

## 19. Reviewer Checklist

- Scope is limited to CI/security quality gate orchestration and docs.
- The local script and CI workflow share the same verification path.
- Coverage threshold is enforced at 80%.
- Any coverage fix is limited to authorization-policy critical paths and does
  not lower the gate threshold.
- E2E harness fixes are limited to repeatable test-data isolation and do not
  change production runtime behavior.
- Focused E2E cannot pass by skipping required tests.
- Full non-live E2E runs after the focused gate.
- SonarScanner Quality Gate is represented and does not leak secrets.
- Security scanners are pinned or installed deterministically.
- Docker ports avoid the local Sonar `9000` port.
- Go package commands run from `backend/`.
- CI Sonar secret behavior is explicit for push, workflow dispatch, non-fork PR,
  and fork PR events.
- Rollback covers CI/docs deletion plus an explicit decision on retaining or
  reverting the toolchain/dependency/image security updates.

## 20. Status

Status: Approved
