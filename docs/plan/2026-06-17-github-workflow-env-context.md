# GitHub Workflow Scheduling And Sonar Secret Policy Fix

## 1. Objective

Fix `.github/workflows/backend-quality-gate.yml` so GitHub Actions can schedule
the backend quality gate jobs again and so the Sonar step behaves correctly when
the repository has no Sonar secrets configured.

## 2. Background

Recent push runs for `.github/workflows/backend-quality-gate.yml` fail
immediately before any jobs are created. The latest failed main run is
`27666005151` for commit `2ecd3c4`; `gh run view` reports `jobs: []` and
`gh run view --log` reports `log not found`.

The workflow defined `${{ runner.temp }}` in workflow-level `env`. GitHub's
Contexts reference lists workflow-level `env` as allowing only `github`,
`secrets`, `inputs`, and `vars`; the `runner` context is available once a job is
running. Therefore the workflow is rejected during expression validation before
the backend quality gate job is scheduled.

After replacing `runner.temp`, GitHub run `27666820021` scheduled the backend
quality gate job and passed workflow syntax, quick, Docker-backed E2E/coverage,
and security/image scan phases. It then failed in `SonarScanner Quality Gate`
because `CI_GATE_SONAR_REQUIRED=true` while both `SONAR_TOKEN` and
`SONAR_HOST_URL` are empty in GitHub Actions. `gh secret list` and
`gh variable list` return no repository secrets or variables. Local Sonar env is
present, but `SONAR_HOST_URL` is localhost-only and cannot be used by GitHub's
hosted runner.

## 3. Source References

- `.github/workflows/backend-quality-gate.yml`
- `backend/scripts/ci-security-gate.sh`
- `problem.md`
- Failed GitHub Actions run:
  `https://github.com/linskybing/NexusPaas/actions/runs/27666005151`
- GitHub Actions Contexts reference:
  `https://docs.github.com/en/actions/reference/workflows-and-actions/contexts`

## 4. Assumptions

- The repository currently runs this workflow only on `ubuntu-latest`, so `/tmp`
  is an acceptable runner-local temporary base for the quality gate paths.
- `secrets.SONAR_TOKEN` and `secrets.SONAR_HOST_URL` can remain in
  workflow-level `env`; GitHub documents `secrets` as available there.
- Fixing scheduling may expose later runtime failures; the immediate
  no-job/no-log failure is resolved by removing `runner` from workflow-level
  `env`.
- With no repository Sonar secrets configured, GitHub CI should skip Sonar and
  upload a `sonar-skipped.txt` artifact instead of failing every run.
- If either Sonar secret is configured, the workflow should require Sonar so a
  partial or broken Sonar configuration still blocks the gate.

## 5. Non-Goals

- Do not change the backend quality gate phases or loosen pass/fail criteria.
- Do not skip Docker-backed E2E, security scans, coverage, or artifact upload.
- Do not claim GitHub-hosted Sonar Quality Gate coverage when GitHub Sonar
  secrets are absent.
- Do not modify backend runtime code, deployment manifests, or tests.
- Do not mark live staging or broader Production Beta blockers as resolved.

## 6. Current Behavior

Every push to `main` and feature branches previously created a failed workflow
run with no jobs and no logs because expression validation failed before job
scheduling. After fixing that, GitHub-hosted runs fail at the Sonar step because
the repository has no `SONAR_TOKEN` or `SONAR_HOST_URL` secrets configured.

## 7. Target Behavior

GitHub Actions accepts the workflow file and schedules the `Backend Quality
Gate` job. The gate keeps the same phases and writes artifacts under
runner-local temp directories that do not depend on unavailable workflow-level
contexts. Sonar remains required when the repository has any Sonar secret
configuration, but it is explicitly skipped with an artifact when GitHub has no
Sonar secrets configured.

## 8. Affected Domains

- CI/CD quality gate
- GitHub Actions workflow scheduling
- Production Beta launch readiness tracking

## 9. Affected Files

- `.github/workflows/backend-quality-gate.yml`
- `problem.md`

## 10. API / Contract Changes

None. CI workflow scheduling only.

## 11. Database / Migration Changes

None.

## 12. Configuration Changes

The workflow's runner-local temp path environment variables will use literal
`/tmp/...` paths instead of `${{ runner.temp }}/...` at workflow scope. The
workflow-level `CI_GATE_SONAR_REQUIRED` expression will be tied to Sonar secret
presence so unconfigured GitHub repositories do not fail every run while still
failing partial or configured Sonar gates.

## 13. Observability Changes

No runtime observability changes. CI artifact upload should start working once
the job schedules.

## 14. Security Considerations

- Do not expose secret values.
- Keep Sonar secrets in GitHub Secrets.
- Do not print secret values. The GitHub log masks secrets and currently shows
  the Sonar env values as empty.
- Keep `permissions: contents: read`.
- Do not disable any security or quality gate.

## 15. Implementation Steps

1. Replace workflow-level `${{ runner.temp }}/...` values with literal
   `/tmp/...` paths for `CI_GATE_ARTIFACT_DIR`, `CI_GATE_TOOLS_DIR`,
   `CI_GATE_DOCKER_CONFIG`, and `CI_GATE_TRIVY_CACHE_DIR`.
2. Change `CI_GATE_SONAR_REQUIRED` so it is true only when at least one Sonar
   secret is configured and the event is eligible for same-repository Sonar
   execution. This makes no-secrets repositories skip Sonar, while partial
   Sonar configuration still fails inside `run_sonar_gate`.
3. Add a short `problem.md` resolved item noting the workflow scheduling fix,
   record GitHub Sonar as skipped when secrets are absent, and preserve
   remaining launch blockers.
4. Run local syntax/verification checks.
5. Submit the implementation to Reviewer Agent.
6. Commit, push, open PR, and confirm the GitHub Actions run has at least one
   scheduled job.

## 16. Verification Plan

- `ruby -e 'require "yaml"; YAML.load_file(".github/workflows/backend-quality-gate.yml")'`
- `bash -n backend/scripts/ci-security-gate.sh`
- `rg -n 'runner\\.temp' .github/workflows/backend-quality-gate.yml` should
  return no matches.
- `rg -n 'CI_GATE_SONAR_REQUIRED' .github/workflows/backend-quality-gate.yml`
  shows the expression is based on Sonar secret presence.
- `CI_GATE_SONAR_REQUIRED=false SONAR_TOKEN= SONAR_HOST_URL= bash backend/scripts/ci-security-gate.sh sonar`
  succeeds locally and writes a skipped marker.
- `CI_GATE_SONAR_REQUIRED=true SONAR_TOKEN= SONAR_HOST_URL= bash backend/scripts/ci-security-gate.sh sonar`
  fails locally, proving explicit required/misconfigured Sonar still blocks.
- `git diff --check`
- `cd backend && go test ./internal/platform -run 'Deployment|Release|Beta' -count=1`
- After push: `gh run list --repo linskybing/NexusPaas --branch feature/github-workflow-env --limit 1 --json databaseId,status,conclusion,url`
- After push: `gh run view <run-id> --json jobs,status,conclusion` must show at
  least one job instead of `jobs: []`; if GitHub Sonar secrets remain absent,
  the Sonar step should skip rather than fail.

The full gate may take much longer than this fix. This PR's first-order success
criterion is that GitHub accepts the workflow and schedules the job.

## 17. Rollback Plan

Revert this documentation/CI workflow PR. The previous workflow failure would
return, but no runtime or data rollback is involved.

## 18. Risks and Tradeoffs

- Risk: `/tmp` is Linux-specific. Mitigation: the workflow already pins
  `runs-on: ubuntu-latest`.
- Risk: GitHub-hosted CI can pass with Sonar skipped when repository secrets are
  absent. Mitigation: record this as a remaining launch/readiness issue and
  require Sonar once secrets are configured.
- Tradeoff: Literal `/tmp` paths are less portable than `runner.temp`, but they
  are valid at workflow scope and match the current Ubuntu-only workflow.

## 19. Reviewer Checklist

| Category | Check |
| --- | --- |
| Requirement Fit | Workflow no longer uses unavailable `runner` context at workflow scope and does not fail Sonar when GitHub Sonar secrets are absent. |
| Scope Control | Only CI workflow and `problem.md` are changed. |
| Architecture | No service/runtime architecture changes. |
| Microservice Boundary | No service boundary changes. |
| API Contract | No API contract changes. |
| Data Ownership | No data ownership changes. |
| Config | CI temp path config remains external to runtime. |
| Observability | CI artifacts remain configured. |
| Security | Secrets remain in GitHub Secrets; partial Sonar configuration still fails closed. |
| Testing | Local syntax/Sonar policy checks plus pushed workflow job scheduling check. |
| Rollback | Revert PR. |
| Diff Scope | No unrelated files or `long-term.md` are included. |

## 20. Status

Status: Approved
