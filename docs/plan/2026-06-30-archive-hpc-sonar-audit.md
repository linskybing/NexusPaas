# Archive/Image-Build, HPC Storage, and Sonar Security Audit

## 1. Objective

Produce a detailed, verifiable audit report on archive/image-build documentation
and code drift plus HPC storage maturity, then remediate every currently visible
SonarCloud SECURITY issue for `linskybing_NexusPaas`:

- hardcoded password embedded in `TEST_DATABASE_URL` in
  `.github/workflows/backend-quality-gate.yml`;
- unpinned GitHub Actions in `.github/workflows/backend-quality-gate.yml`;
- coturn `hostNetwork: true` finding in `backend/deploy/k3s/coturn.yaml` and
  the corresponding Sonar exclusion/risk record;
- Dockerfile default-root findings.

This plan is the Plan Agent artifact only. The current workflow fallback is:
Plan Agent = Codex subagent, Reviewer Agent = Codex subagent, Code Agent =
Codex main agent because Claude Code is unavailable in this environment.

## 2. Background

The repository requires a three-agent workflow: a Plan Agent writes this plan, a
Reviewer Agent approves or rejects it, and the Code Agent implements only the
approved plan. No production code is changed by this planning step.

The audit work is needed because image-build acceptance docs now describe
archive upload, storage-backed builds, Harbor, supply-chain status, and
allow-list behavior, while the repo also contains a read-only reference backend
under `references/CSCC_AI_Platform_Backend/` and current product code under
`backend/internal/services/imageregistry` and `backend/internal/services/storage`.
The report must separate implemented evidence from open maturity gaps.

The SonarCloud work is needed because open SECURITY findings remain visible even
though some local files already contain compensating controls or exclusions. The
implementation must close real issues in source/configuration and explicitly
document any intentional risk acceptance such as coturn host networking.

## 3. Source References

- `AGENTS.md`
- `docs/agents/planning.md`
- `docs/agents/workflow.md`
- `docs/agents/review-checklist.md`
- `.github/workflows/backend-quality-gate.yml`
- `sonar-project.properties`
- `backend/deploy/k3s/coturn.yaml`
- `backend/Dockerfile`
- `backend/streaming/selkies-gl-desktop/Dockerfile`
- `docs/acceptance/image-build.md`
- `docs/acceptance/gap-analysis.md`
- `docs/acceptance/ga-acceptance-trace-matrix.md`
- `docs/acceptance/cli.md`
- `docs/acceptance/cncf-adoption.md`
- `backend/deploy/hpc/storage/README.md`
- `backend/deploy/hpc/storage/local-nvme-storageclass.yaml`
- `backend/deploy/hpc/storage/cephfs-rwx-authority.yaml`
- `backend/deploy/hpc/storage/longhorn-rwx-standard.yaml`
- `backend/internal/services/storage/storage_profiles.go`
- `backend/internal/services/storage/storage_profiles_manifest_test.go`
- `backend/internal/services/storage/data_plane_contracts.go`
- `backend/internal/services/imageregistry/handler.go`
- `backend/internal/services/imageregistry/spec.go`
- `backend/internal/e2e/image_build_governance_e2e_test.go`
- `backend/internal/e2e/live_harbor_image_build_e2e_test.go`
- `backend/internal/contracts/fixtures/events/v1/image-build-started.json`
- `references/CSCC_AI_Platform_Backend/internal/api/handlers/image/image_handler_build.go`
- `references/CSCC_AI_Platform_Backend/internal/api/handlers/image/image_build_limits.go`
- `references/CSCC_AI_Platform_Backend/internal/domain/image/model_build_request.go`

## 4. Assumptions

- The user-supplied SonarCloud issue list is the source of truth for the open
  SECURITY findings to close.
- The implementation has access to the SonarCloud UI or scanner output needed
  to verify that `linskybing_NexusPaas` no longer shows these SECURITY issues.
- Commit-SHA pinning is acceptable for all actions and reusable workflows in
  `.github/workflows/backend-quality-gate.yml`.
- CI test database credentials are non-production, but Sonar must still not see
  a literal password embedded in a connection URL.
- coturn host networking is intentional unless the audit proves TURN relay
  requirements are satisfied without it.
- `references/CSCC_AI_Platform_Backend/` is read-only reference material for the
  audit and should not be edited.
- No new microservice, API, database table, queue, or external infrastructure is
  required.

## 5. Non-Goals

- Do not implement live Tekton, BuildKit, Harbor, SBOM, signing, or image
  promotion behavior.
- Do not claim image-build GA readiness from local/static evidence alone.
- Do not implement new storage CSI drivers, provisioners, backup systems, or
  live cluster storage changes.
- Do not rewrite coturn deployment architecture unless host networking is proven
  unnecessary.
- Do not add new CI dependencies just to lint YAML or pin actions.
- Do not edit unrelated user changes.
- Do not modify production code during the Plan Agent step.

## 6. Current Behavior

- `docs/plan/2026-06-30-archive-hpc-sonar-audit.md` did not exist before this
  planning step.
- `docs/acceptance/image-build.md` lists archive upload, storage-backed builds,
  rootless BuildKit/Tekton, Harbor push, SBOM, scan, signing, and digest
  allow-list targets. Its current evidence section explicitly marks queued
  supply-chain metadata as local contract evidence, not completed live workflow
  proof.
- `docs/acceptance/ga-acceptance-trace-matrix.md` keeps IMG, STORAGE, SEC, OPS,
  and related launch rows open where live execution or maturity evidence is
  missing.
- `backend/deploy/hpc/storage/README.md` documents three HPC storage classes:
  `local-nvme-scratch`, `cephfs-rwx-authority`, and
  `longhorn-rwx-standard`. Current evidence is local/static and does not prove
  live CSI behavior, PVC isolation, namespace enforcement, durability, or
  production DR.
- `.github/workflows/backend-quality-gate.yml` currently embeds
  `postgres://nexuspaas:nexuspaas@127.0.0.1:15432/nexuspaas?sslmode=disable`
  in `TEST_DATABASE_URL`.
- `.github/workflows/backend-quality-gate.yml` uses tag-pinned actions such as
  `actions/checkout@v4`, `actions/setup-go@v5`, `dorny/paths-filter@v4`,
  `reviewdog/action-actionlint@v1`, `actions/upload-artifact@v4`,
  `google/osv-scanner-action/...@v2.3.8`,
  `aquasecurity/trivy-action@v0.36.0`,
  `docker/setup-buildx-action@v3`, and `docker/build-push-action@v6`.
- `backend/deploy/k3s/coturn.yaml` uses `hostNetwork: true`,
  `dnsPolicy: ClusterFirstWithHostNet`, disabled service-account automount, a
  non-root container user, read-only root filesystem, seccomp, and dropped
  capabilities.
- `sonar-project.properties` currently excludes
  `backend/deploy/k3s/coturn.yaml`, `backend/Dockerfile`, and
  `backend/streaming/**`, with a coturn host-network rationale comment.
- `backend/Dockerfile` already sets `USER app:app` in the final runtime stage.
- `backend/streaming/selkies-gl-desktop/Dockerfile` has no explicit `USER`.

## 7. Target Behavior

- A new audit report exists at
  `docs/acceptance/archive-image-build-hpc-storage-audit.md` with tables for:
  documented claim, source reference, code/deployment evidence, current status,
  drift or maturity gap, severity, owner domain, and recommended next step.
- The audit report distinguishes:
  local/static evidence, contract evidence, env-gated evidence, live evidence,
  accepted risk, and open GA blockers.
- Image-build docs and trace rows are corrected only where the audit finds
  overclaiming or stale wording.
- The CI workflow no longer contains a database URL with a literal password.
  Test jobs still receive a valid `TEST_DATABASE_URL` derived from CI-only
  values.
- Every `uses:` entry in `.github/workflows/backend-quality-gate.yml` is pinned
  to a full immutable commit SHA.
- The coturn host-network finding is resolved by the smallest correct action:
  keep `hostNetwork` with a clear Sonar exclusion and documented compensating
  controls if it is still required, or remove it only if TURN behavior remains
  correct without host networking.
- Dockerfile default-root findings are resolved by explicit final-stage
  non-root users or by narrowly scoped, documented exclusions only where the
  runtime genuinely requires root.
- SonarCloud for `linskybing_NexusPaas` shows no remaining open SECURITY issues
  for the four listed categories after the next analysis.

## 8. Affected Domains

- CI and supply-chain security: GitHub Actions workflow pinning, CI-only test
  database URL composition, Trivy/OSV/actionlint path behavior.
- Static analysis configuration: Sonar project exclusions, project key/analysis
  path validation, SECURITY issue closure evidence.
- Kubernetes deployment security: coturn host networking risk and compensating
  controls.
- Container runtime hardening: backend and Selkies runtime user declarations.
- Image-build evidence: archive upload, storage-backed build, Harbor, scan,
  signing, allow-list, and event/contract maturity audit only.
- HPC storage evidence: StorageProfile-to-StorageClass mapping, local/static
  tests, data-plane contracts, and live maturity gaps.

## 9. Affected Files

Plan artifact in this step:

- `docs/plan/2026-06-30-archive-hpc-sonar-audit.md`

Expected implementation files after Reviewer approval:

- `docs/acceptance/archive-image-build-hpc-storage-audit.md` (new audit report)
- `docs/acceptance/image-build.md` (only if audit finds stale or overclaimed
  evidence wording)
- `docs/acceptance/gap-analysis.md` (only if audit findings require traceable
  wording updates)
- `docs/acceptance/ga-acceptance-trace-matrix.md` (only if status/evidence
  wording is stale)
- `.github/workflows/backend-quality-gate.yml`
- `sonar-project.properties`
- `backend/deploy/k3s/coturn.yaml` (only if host networking can be removed or a
  manifest-level comment is required)
- `backend/Dockerfile` (only if the Sonar finding is not stale and needs a
  source-visible hardening adjustment)
- `backend/streaming/selkies-gl-desktop/Dockerfile`

Audit-only source files, not expected to be edited:

- `backend/internal/services/imageregistry/**`
- `backend/internal/services/storage/**`
- `backend/internal/contracts/fixtures/**`
- `backend/internal/e2e/*image_build*`
- `backend/deploy/hpc/storage/*.yaml`
- `references/CSCC_AI_Platform_Backend/**`

## 10. API / Contract Changes

None expected. This work should not add, remove, or alter HTTP routes, event
schemas, CLI contracts, OpenAPI fixtures, storage contracts, or image-build
payload shapes.

If the audit finds documentation that contradicts an existing contract fixture,
the implementation should update documentation only unless the Reviewer
approves a separate product-contract plan.

## 11. Database / Migration Changes

None. No SQL migrations, platform record migrations, backfills, or data
ownership changes are in scope.

## 12. Configuration Changes

- Update `.github/workflows/backend-quality-gate.yml` so the test database URL
  is built from non-literal CI values instead of embedding
  `nexuspaas:nexuspaas` in the URL.
- Pin every GitHub Action and reusable workflow reference in
  `.github/workflows/backend-quality-gate.yml` to a full commit SHA.
- Keep or adjust `sonar-project.properties` exclusions so intentional
  host-network and unavoidable runtime-root exceptions are narrow, documented,
  and effective for the SonarCloud project actually being analyzed.
- Add explicit final-stage non-root user declarations to Dockerfiles where
  runtime compatibility allows it.

## 13. Observability Changes

- Add the audit report as durable repo evidence with commands, source paths,
  and Sonar issue status references.
- Record SonarCloud verification status in the audit report by issue category:
  fixed in code, excluded with justification, stale/closed after rescan, or
  still open with blocker.
- No runtime metric, log, trace, dashboard, or alerting changes are required.

## 14. Security Considerations

- Removing the hardcoded password from `TEST_DATABASE_URL` reduces accidental
  credential-pattern exposure even for local CI-only credentials.
- SHA pinning reduces action supply-chain risk but makes action upgrades more
  manual. The pinned SHA list must preserve the intended upstream action tags in
  comments or audit evidence so future updates are reviewable.
- coturn host networking exposes a host namespace and must remain justified by
  TURN relay behavior. If kept, compensating controls must remain present:
  service-account automount disabled, non-root UID/GID, read-only root
  filesystem, seccomp `RuntimeDefault`, dropped Linux capabilities, bounded UDP
  relay port range, secret-backed TURN auth, and no CLI.
- Dockerfile root remediation must not break runtime startup or force writable
  paths into insecure permissions. If Selkies requires root for a specific
  component, the report must name that component and keep the exclusion narrow.
- Do not move CI credentials to repository secrets unless they are real secrets.
  Generated or CI-only values are preferable for ephemeral local services.
- Do not hide broad deployment trees from Sonar to close one finding.

## 15. Implementation Steps

1. Reviewer Agent reviews this plan against
   `docs/agents/review-checklist.md`. Code Agent starts only after approval.
2. Create `docs/acceptance/archive-image-build-hpc-storage-audit.md` with:
   claim/evidence tables for archive image builds, storage-backed image builds,
   supply-chain status, Harbor/allow-list behavior, and HPC storage maturity;
   explicit statuses for local/static, contract, env-gated, live, accepted-risk,
   and open; and source links for every finding.
3. Compare image-build docs against current implementation evidence:
   `docs/acceptance/image-build.md`,
   `docs/acceptance/gap-analysis.md`,
   `docs/acceptance/ga-acceptance-trace-matrix.md`,
   `backend/internal/services/imageregistry/**`, image-build fixtures, image
   build E2E tests, and the read-only reference archive handlers under
   `references/CSCC_AI_Platform_Backend/**`.
4. Compare HPC storage docs/manifests against current storage-service evidence:
   seeded profiles, `backend/deploy/hpc/storage/*.yaml`,
   `storage_profiles_manifest_test.go`, data-plane contracts, and E2E/static
   tests. Mark live CSI, PVC isolation, namespace enforcement, durability,
   performance, and DR as open unless there is direct evidence.
5. Update acceptance docs only where the audit proves wording is stale or
   overclaims readiness. Keep edits small and do not change product behavior.
6. Remediate the workflow database URL:
   remove the literal password-bearing URL from global env, generate or compose
   the CI-only password without embedding it in the URL, pass the same values to
   the Postgres service and test steps, and verify integration tests still
   connect.
7. Pin every `uses:` reference in
   `.github/workflows/backend-quality-gate.yml` to a full commit SHA. Include
   enough local evidence in the audit report to map each SHA back to the
   intended upstream tag/version.
8. Resolve the coturn Sonar finding:
   first verify whether `hostNetwork` is still required for the deployed TURN
   relay path. If required, keep the manifest behavior, keep the Sonar exclusion
   narrow, and document the accepted risk plus compensating controls. If not
   required, remove `hostNetwork`, update `dnsPolicy`, preserve service
   exposure, and verify manifests.
9. Resolve Dockerfile default-root findings:
   keep `backend/Dockerfile` final runtime non-root behavior; add or verify an
   explicit non-root runtime user for Selkies if compatible; otherwise keep a
   narrow Sonar exclusion with a named runtime-root reason and reviewer
   approval.
10. Run the verification commands below. Record any unavailable external tool
    explicitly in the audit report.
11. Reviewer Agent performs final review for requirement fit, approved-plan
    alignment, SOLID, 12-Factor compliance, tests/build, SonarScanner Quality
    Gate status, risks, and diff scope.

## 16. Verification Plan

Required local/static checks:

```sh
git diff --check
git diff --name-only
rg -n 'nexuspaas:nexuspaas@|TEST_DATABASE_URL: .*://.*:.*@' .github/workflows/backend-quality-gate.yml
perl -ne 'if (/uses:\s+\S+@([^\s#]+)/ && $1 !~ /^[0-9a-f]{40}$/) { print; $bad=1 } END { exit($bad || 0) }' .github/workflows/backend-quality-gate.yml
rg -n '^USER ' backend/Dockerfile backend/streaming/selkies-gl-desktop/Dockerfile
rg -n 'hostNetwork: true|backend/deploy/k3s/coturn.yaml|backend/streaming' backend/deploy/k3s/coturn.yaml sonar-project.properties
```

Expected results:

- the hardcoded-URL `rg` command prints no password-bearing `TEST_DATABASE_URL`
  matches;
- the `perl` command exits zero and prints no unpinned `uses:` lines;
- each scanned runtime Dockerfile either has an explicit non-root `USER` or is
  narrowly excluded with documented reviewer-approved rationale;
- coturn host networking is either removed and manifest-validated, or still
  present with a narrow Sonar exclusion and audit rationale.

Workflow/deployment checks:

```sh
actionlint .github/workflows/backend-quality-gate.yml
kubectl apply --dry-run=client -f backend/deploy/k3s/coturn.yaml
kubectl apply --dry-run=client -f backend/deploy/hpc/storage/
```

If `actionlint` or `kubectl` is unavailable locally, record that fact and rely
on the GitHub workflow lint job or a reviewer-provided environment before merge.

Backend checks:

```sh
cd backend && go test ./internal/services/storage -run 'StorageProfilesHPCStorageClassManifests|DataPlane' -count=1
cd backend && go test ./internal/services/imageregistry ./internal/contracts/... -count=1
cd backend && go test ./... -count=1
cd backend && go build ./...
cd backend && make ci-sonar
```

SonarCloud checks:

- Run or observe the SonarCloud analysis for `linskybing_NexusPaas`.
- Confirm no open SECURITY findings remain for:
  hardcoded `TEST_DATABASE_URL` password, unpinned GitHub Actions, coturn
  `hostNetwork`, and Dockerfile default-root.
- Add the analysis date, project key, and issue status summary to the audit
  report.

## 17. Rollback Plan

- Revert documentation-only audit/report changes by removing
  `docs/acceptance/archive-image-build-hpc-storage-audit.md` and restoring any
  touched acceptance docs.
- Revert `.github/workflows/backend-quality-gate.yml` if CI database connection
  composition or action pinning breaks the workflow. If reverting action pins,
  keep the Sonar issue open and record the blocker; do not silently restore
  unpinned actions as complete.
- Revert Dockerfile `USER` changes if image build or runtime smoke tests fail,
  then either implement the minimum writable-path fix or document a
  reviewer-approved narrow exclusion.
- Revert coturn manifest changes if TURN relay validation fails. If
  `hostNetwork` is restored, retain the explicit risk rationale and Sonar
  exclusion.
- No database rollback is needed.

## 18. Risks and Tradeoffs

- SHA-pinned actions improve supply-chain security but require manual upgrade
  work when upstream tags move.
- Removing a hardcoded CI database URL must preserve service/test env parity or
  integration tests may fail to connect to Postgres.
- coturn may legitimately require host networking for UDP relay behavior. A
  code-only removal would be smaller but riskier than an accepted, documented
  exception if live behavior depends on host networking.
- Selkies may inherit runtime assumptions from its base image. Adding `USER`
  without testing could break startup, GPU access, or writable paths.
- The audit can identify drift and maturity gaps, but it does not close live
  image-build or HPC storage GA blockers by itself.
- SonarCloud may retain stale issues until the next successful analysis. The
  implementation must distinguish source remediation from remote issue refresh.

## 19. Reviewer Checklist

- [ ] Fallback assignment is recorded: Plan Agent = Codex subagent, Reviewer
  Agent = Codex subagent, Code Agent = Codex main agent.
- [ ] Audit report separates implemented evidence from open image-build and HPC
  storage maturity gaps.
- [ ] `references/CSCC_AI_Platform_Backend/**` is used as read-only evidence
  only.
- [ ] No unrelated production refactor or new abstraction is introduced.
- [ ] `TEST_DATABASE_URL` no longer embeds a literal password.
- [ ] Every GitHub Action/reusable workflow in
  `.github/workflows/backend-quality-gate.yml` is pinned to a full commit SHA.
- [ ] coturn `hostNetwork` is either removed with validation or retained with a
  narrow Sonar exclusion, compensating controls, and explicit risk acceptance.
- [ ] Dockerfile default-root findings are fixed with explicit final-stage
  non-root users or narrowly justified exclusions.
- [ ] Verification commands were run or unavailable tools were explicitly
  recorded.
- [ ] SonarCloud `linskybing_NexusPaas` SECURITY issues are closed or the
  remaining blocker is documented before final approval.
- [ ] Diff scope is limited to the approved affected files.

## 20. Status

Status: Approved
