# Fix PR33 External Sonar Gate

## 1. Objective

Fix PR #33 by changing the backend quality gate design from an in-workflow
Sonar Quality Gate job to an externally reported SonarCloud/SonarQube required
PR check. The implementation must remove the GitHub Actions dependency on
`SONAR_TOKEN` and `SONAR_HOST_URL` from
`.github/workflows/backend-quality-gate.yml`, while preserving the local
`backend/scripts/ci-security-gate.sh sonar` behavior.

## 2. Background

The current branch `feature/v1-external-deploy-gate` has an in-workflow
`sonar` job in `.github/workflows/backend-quality-gate.yml`. That job uses the
SonarSource scan action and fails trusted GitHub Actions events when
`SONAR_TOKEN` or `SONAR_HOST_URL` is unavailable. PR #33 failed because the
desired release governance model is different: SonarCloud/SonarQube should be a
separate external PR status/check enforced by branch protection, not a job
inside the backend workflow.

## 3. Source References

- `.github/workflows/backend-quality-gate.yml`
- `backend/internal/platform/deployment_test.go`
- `backend/docs/beta-launch-readiness.md`
- `backend/docs/e2e-testing.md`
- `backend/scripts/ci-security-gate.sh`
- `docs/agents/workflow.md`
- `docs/agents/planning.md`
- `docs/agents/review-checklist.md`

## 4. Assumptions

- GitHub branch protection for PR #33 can require the external
  SonarCloud/SonarQube check outside this repository diff.
- The external Sonar provider is already configured, or will be configured, to
  publish a PR check/comment for the repository.
- The backend workflow should remain responsible for workflow lint, Go checks,
  integration/E2E, govulncheck, OSV, Trivy filesystem scan, image build, and
  image scan.
- Local maintainers still need the script-driven Sonar path for manual or
  local release-candidate validation.
- No service boundary, API, data ownership, or runtime deployment behavior is
  changed by this task.

## 5. Non-Goals

- Do not modify `backend/scripts/ci-security-gate.sh` behavior.
- Do not remove `sonar-project.properties`.
- Do not add a replacement in-workflow Sonar scan through another action or
  shell command.
- Do not update `problem.md` or `gap.md`.
- Do not mark any live P0 item closed.
- Do not change application runtime code, service manifests, database schemas,
  migrations, frontend code, or deployment docs beyond the two named backend
  docs files.
- Do not configure GitHub branch protection in repository files.

## 6. Current Behavior

- `detect-changes` publishes a `sonar` output that includes workflow, backend,
  deploy, script, dependency, migration, and `sonar-project.properties` changes.
- The workflow defines a `sonar` job named `Sonar Quality Gate`.
- The `sonar` job generates Go coverage, requires `SONAR_TOKEN` and
  `SONAR_HOST_URL`, and invokes the pinned SonarSource scan action with
  `-Dsonar.qualitygate.wait=true`.
- The aggregate `quality-gate` job includes `sonar` in `needs` and prints the
  `sonar` job result.
- `backend/internal/platform/deployment_test.go` currently asserts the
  in-workflow Sonar job/action/secrets must exist.
- `backend/docs/beta-launch-readiness.md` and `backend/docs/e2e-testing.md`
  describe GitHub Actions as the runner for trusted-event Sonar enforcement.

## 7. Target Behavior

- `.github/workflows/backend-quality-gate.yml` contains no in-workflow Sonar
  Quality Gate job.
- `detect-changes.outputs.sonar` is removed.
- Any now-unused path-filter entry that exists only to drive
  `detect-changes.outputs.sonar` is removed.
- The aggregate `quality-gate` job no longer needs or reports `sonar`.
- The workflow no longer references the SonarSource scan action,
  `SONAR_TOKEN`, `SONAR_HOST_URL`, or `-Dsonar.qualitygate.wait=true`.
- `backend/internal/platform/deployment_test.go` forbids in-workflow Sonar and
  still asserts the local script keeps the `sonar` subcommand, local
  SonarScanner install validation, required-secret fail-closed logic, and RC
  report status text.
- `backend/docs/beta-launch-readiness.md` and `backend/docs/e2e-testing.md`
  describe SonarCloud/SonarQube as an external required PR check and branch
  protection gate.
- The local `backend/scripts/ci-security-gate.sh sonar` command remains
  available and unchanged.

## 8. Affected Domains

- CI workflow orchestration for backend quality gates.
- Platform deployment/readiness regression tests that inspect workflow and docs
  contracts.
- Backend release and E2E documentation.
- Repository governance around external PR checks and branch protection.

## 9. Affected Files

Implementation files for the Code Agent:

- `.github/workflows/backend-quality-gate.yml`
- `backend/internal/platform/deployment_test.go`
- `backend/docs/beta-launch-readiness.md`
- `backend/docs/e2e-testing.md`

Files explicitly not to edit:

- `backend/scripts/ci-security-gate.sh`
- `problem.md`
- `gap.md`
- `sonar-project.properties`

Plan Agent ownership for this pass:

- `docs/plan/2026-06-23-fix-pr33-external-sonar-gate.md`

## 10. API / Contract Changes

No application API changes.

The CI contract changes as follows:

- The backend workflow stops producing a `sonar` job result.
- The required Sonar result must come from the external SonarCloud/SonarQube PR
  check configured in repository branch protection.
- The local script contract remains unchanged: `backend/scripts/ci-security-gate.sh sonar`
  still runs SonarScanner when configured and still fails closed when local/CI
  policy requires Sonar credentials.

## 11. Database / Migration Changes

No database, migration, seed, or data ownership changes.

## 12. Configuration Changes

- Remove the backend workflow's dependency on repository secrets
  `SONAR_TOKEN` and `SONAR_HOST_URL`.
- Do not introduce new repository, service, or runtime configuration.
- External branch protection must require the SonarCloud/SonarQube PR check by
  its provider-published check name; that setting is outside this repository
  diff.

## 13. Observability Changes

No runtime logging, metrics, or tracing changes.

CI visibility changes:

- The aggregate backend `quality-gate` report no longer prints a `sonar` line.
- PR validation must be checked through the external SonarCloud/SonarQube check
  and any provider PR comment after the Code Agent pushes.

## 14. Security Considerations

- Removing the in-workflow Sonar job prevents PR #33 from failing solely because
  trusted Sonar secrets are unavailable to this workflow.
- Branch protection must require the external SonarCloud/SonarQube check so
  removing the workflow job does not weaken merge protection.
- The local script's fail-closed behavior remains intact for manual or CI
  contexts that explicitly require Sonar credentials.
- No secrets should be added to docs, tests, workflow logs, or plan output.

## 15. Implementation Steps

1. Confirm the worktree state and preserve unrelated user changes.
2. Edit `.github/workflows/backend-quality-gate.yml` only for the Sonar
   workflow removal:
   - remove `detect-changes.outputs.sonar`;
   - remove the now-unused `sonar_config` path filter if no other output uses
     it;
   - remove the entire `sonar` job;
   - remove `sonar` from `quality-gate.needs`;
   - remove the `echo "sonar: ..."` report line;
   - leave all non-Sonar jobs, names, permissions, and path filters unchanged.
3. Update `backend/internal/platform/deployment_test.go` in
   `TestProductionBetaReleaseCandidateGateIsDocumented`:
   - keep assertions that `backend/scripts/ci-security-gate.sh` documents and
     implements the local `sonar` subcommand;
   - keep assertions for `CI_GATE_SONAR_REQUIRED`, missing secret fail-closed
     messaging, SonarScanner cache validation, and the RC report Sonar status;
   - replace workflow assertions that require in-workflow Sonar with assertions
     that forbid `SonarSource/sonarqube-scan-action`,
     `Require Sonar secrets`, `SONAR_TOKEN`, `SONAR_HOST_URL`,
     `needs.detect-changes.outputs.sonar`, the `sonar` aggregate need/report,
     and the workflow-level `-Dsonar.qualitygate.wait=true`;
   - keep existing unrelated workflow assertions for focused E2E skip checks and
     govulncheck action replacement.
4. Update `backend/docs/beta-launch-readiness.md`:
   - keep the local `beta-rc` gate description and local Sonar behavior;
   - replace "Remote CI must run SonarScanner Quality Gate..." language with
     "SonarCloud/SonarQube is an external required PR check/branch protection
     gate";
   - make clear that the backend workflow does not require Sonar secrets;
   - keep live staging P0 language open.
5. Update `backend/docs/e2e-testing.md` similarly:
   - keep focused subcommands including the local `sonar` script example;
   - replace "GitHub Actions runs SonarScanner Quality Gate..." with external
     required PR check/branch protection language;
   - avoid saying fork PRs may skip in-workflow Sonar, because Sonar is no
     longer an in-workflow job.
6. Do not edit `backend/scripts/ci-security-gate.sh`, `problem.md`, `gap.md`,
   or `sonar-project.properties`.
7. Review the diff for accidental broad changes before handing back to the
   Reviewer Agent.

## 16. Verification Plan

Required local commands:

```sh
go -C backend test ./internal/platform -run 'ProductionBeta|ReleaseCandidate|Sonar' -count=1
bash -n backend/scripts/ci-security-gate.sh
git diff --check
```

Required reviewer spot checks:

```sh
rg -n "SonarSource/sonarqube-scan-action|Require Sonar secrets|SONAR_TOKEN|SONAR_HOST_URL|needs.detect-changes.outputs.sonar|-Dsonar.qualitygate.wait=true" .github/workflows/backend-quality-gate.yml
```

The `rg` command above should return no matches for the workflow.

After the Code Agent pushes PR #33:

- Check PR comments and checks to confirm the backend workflow no longer fails
  on missing Sonar secrets.
- Confirm the external SonarCloud/SonarQube PR check appears separately.
- Confirm branch protection treats that external SonarCloud/SonarQube check as
  required before merge.

## 17. Rollback Plan

If the external SonarCloud/SonarQube required check is not available or branch
protection cannot enforce it, revert the Code Agent's workflow, test, and docs
changes from this plan and restore the previous in-workflow Sonar job until the
external check is ready. No database or runtime rollback is required.

## 18. Risks and Tradeoffs

- The repository diff cannot prove branch protection settings; PR checks must be
  inspected after push.
- If branch protection is not updated, removing the in-workflow Sonar job could
  weaken merge enforcement.
- `sonar-project.properties` changes will no longer trigger a backend workflow
  Sonar job; the external Sonar provider must own that validation signal.
- Tests should avoid broad bans on the word `sonar` because docs and the local
  script still intentionally reference local Sonar behavior.

## 19. Reviewer Checklist

- [ ] Requirement fit: workflow Sonar job/output/aggregate dependency removed.
- [ ] Scope control: only the workflow, one platform test file, and two backend
  docs files are changed by the Code Agent.
- [ ] Non-goals: `backend/scripts/ci-security-gate.sh`, `problem.md`,
  `gap.md`, and live P0 status remain unchanged.
- [ ] Architecture: no service boundary, API, database, or runtime deployment
  behavior changes.
- [ ] Config/security: backend workflow no longer requires Sonar secrets, and
  external branch protection owns the required Sonar gate.
- [ ] Testing: required Go test, shell syntax check, diff check, and PR
  comments/checks review are documented.
- [ ] Simplicity: no replacement in-workflow Sonar implementation is added.
- [ ] Surgical change: no unrelated docs, tests, workflows, or ledgers are
  modified.

## 20. Status

Status: Draft
