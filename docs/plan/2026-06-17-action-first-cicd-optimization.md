# Action-First CI/CD Optimization

## 1. Objective

Replace the backend quality gate workflow's script-driven orchestration with an
action-first GitHub Actions design that avoids running expensive gates for
documentation-only changes and avoids custom downloads for tools that already
have maintained public actions.

## 2. Background

The current backend quality gate is a single serial job that invokes
`backend/scripts/ci-security-gate.sh` for quick checks, Docker-backed tests,
security scanning, image scanning, and SonarScanner. That script also downloads
general-purpose tools that are already available as public GitHub Actions. This
makes small changes, especially documentation-only pull requests, pay for slow
Docker, scanner, and Sonar work.

## 3. Source References

- `AGENTS.md`
- `docs/agents/workflow.md`
- `docs/agents/planning.md`
- `.github/workflows/backend-quality-gate.yml`
- `backend/scripts/ci-security-gate.sh`
- `backend/Dockerfile`
- `backend/go.mod`
- `backend/go.sum`
- `sonar-project.properties`

## 4. Assumptions

- Branch protection will require only the final `quality-gate` job.
- GitHub-hosted Ubuntu runners provide Docker and support service containers.
- Fork pull requests do not have Sonar secrets and should skip Sonar safely.
- The existing local gate script may remain for local or release-candidate use,
  but it must not be the main CI orchestration path.

## 5. Non-Goals

- Do not rewrite backend services, APIs, contracts, or tests.
- Do not add per-service microservice matrices in this change.
- Do not remove the legacy local gate script.
- Do not add new repository-level deployment automation.

## 6. Current Behavior

The workflow runs one `backend-quality-gate` job for PRs and pushes to `main`
and `feature/**`. That job checks out code, sets up Go, validates shell/YAML,
then calls the local gate script for quick, Docker, security, and Sonar gates in
sequence.

## 7. Target Behavior

The workflow first classifies changed paths, then runs only the jobs relevant to
those changes. Documentation-only pull requests run only change detection and
the final aggregate gate. Backend Go changes run quick Go checks. Dependency,
Docker, deploy, or CI changes trigger security and image scanning. Integration
and E2E tests run only for backend runtime or deployment risk.

## 8. Affected Domains

- CI/CD workflow orchestration
- Security scanning
- Docker image scanning
- Backend integration and E2E test execution
- Sonar quality gate execution

## 9. Affected Files

- `.github/workflows/backend-quality-gate.yml`
- `docs/plan/2026-06-17-action-first-cicd-optimization.md`

## 10. API / Contract Changes

No public API, event contract, or service-to-service contract changes.

## 11. Database / Migration Changes

No schema or migration changes. Existing migrations are only exercised by tests
when integration/E2E gates run.

## 12. Configuration Changes

The workflow adds `workflow_dispatch.full_gate` and replaces custom CI tool
version/download variables with public action usage. Test backing-service
environment variables remain CI-only.

## 13. Observability Changes

The workflow keeps uploaded coverage and E2E artifacts for diagnosability. No
runtime logs, metrics, or traces change.

## 14. Security Considerations

Use least-privilege job permissions where practical. Keep Sonar secrets in
GitHub secrets. Use public scanner actions for govulncheck, OSV, Trivy, Docker
Buildx, and Sonar instead of custom download logic. Do not expose secrets to
fork pull requests.

## 15. Implementation Steps

1. Add path classification with `dorny/paths-filter@v4`.
2. Add conditional jobs for workflow lint, quick Go checks, integration/E2E,
   govulncheck, OSV, Trivy filesystem scan, Docker build/image scan, and Sonar.
3. Use GitHub Actions service containers for Postgres, Redis, and MinIO in the
   integration/E2E job.
4. Use `docker/setup-buildx-action` and `docker/build-push-action` with
   `type=gha` cache for image builds.
5. Add a final always-running `quality-gate` job that fails on failed or
   cancelled required jobs and passes when optional jobs are skipped.
6. Remove workflow calls to `backend/scripts/ci-security-gate.sh`.

## 16. Verification Plan

- `ruby -e 'require "yaml"; YAML.load_file(".github/workflows/backend-quality-gate.yml")'`
- `bash -n backend/scripts/ci-security-gate.sh`
- Inspect the workflow for any remaining `ci-security-gate.sh` invocations.
- Review path filters for docs-only, Go-only, Docker/deploy, and CI changes.

## 17. Rollback Plan

Revert `.github/workflows/backend-quality-gate.yml` to the previous single-job
script-driven workflow and remove this plan file. No runtime application state
or database state needs rollback.

## 18. Risks and Tradeoffs

The action-first workflow has more jobs, but each job has a narrower purpose and
can be skipped safely. Integration/E2E commands still require small shell blocks
because they are project-specific test invocations, not generic tool download or
scanner installation logic.

## 19. Reviewer Checklist

- Workflow uses public actions for general CI/CD capabilities.
- Documentation-only changes do not run backend quick, Docker, security, Sonar,
  or E2E jobs.
- The final `quality-gate` job is suitable as the required branch check.
- No new large shell script is introduced.
- The legacy local script is not invoked by the workflow.

## 20. Status

Status: Approved
