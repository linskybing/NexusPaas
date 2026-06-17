# Beta Launch Hardening

## 1. Objective

Add a reproducible Production Beta release-candidate rehearsal gate that ties
together the existing quality gates with production-beta manifest rendering,
deploy dry-run, rollback command rehearsal, re-deploy dry-run, and a generated
RC evidence report.

## 2. Background

The roadmap's final Beta hardening step requires closing or explicitly
accepting remaining blockers, rehearsing migrations/deploy/smoke/E2E/rollback/
re-deploy, and marking a Beta release candidate. The repository already has
strong quick, Docker-backed E2E, security, and Sonar gates, plus a 15-service
production-beta kustomization. What is missing is a single operator-facing gate
that proves those pieces were run together for a candidate and that the
production-beta manifests can be rendered and applied with client-side dry-run
without mutating a live cluster. The gate also needs the non-live runtime smoke
required by the roadmap: health, readiness, metrics, OpenAPI, service registry,
and one read-only endpoint per registered service without any 5xx response.

This PR creates the non-live release-candidate gate and documents the remaining
live staging rehearsal requirement. It does not declare Public GA or route real
traffic.

## 3. Source References

- `long-term.md`
- `problem.md`
- `docs/agents/workflow.md`
- `docs/agents/planning.md`
- `docs/agents/review-checklist.md`
- `backend/scripts/ci-security-gate.sh`
- `backend/docs/e2e-testing.md`
- `backend/docs/operational-readiness.md`
- `backend/docs/non-functional-requirements.md`
- `backend/kustomization.yaml`
- `backend/internal/platform/deployment_test.go`

## 4. Assumptions

- This roadmap slice is stacked on `feature/observability-runbooks` until
  earlier PRs are merged or this branch is retargeted.
- `kubectl` with bundled Kustomize is available in local and CI environments
  that run the Beta RC rehearsal gate.
- The default Beta RC gate should not mutate a live cluster or require
  production secrets.
- Live staging deploy/rollback/re-deploy remains a separate opt-in operation
  because the production-beta kustomization intentionally requires real
  Kubernetes Secrets or ExternalSecret-managed values.
- Sonar remains available locally through `SONAR_HOST_URL` and `SONAR_TOKEN` for
  this verification pass; CI may skip Sonar only under the already documented
  fork-secret policy.

## 5. Non-Goals

- Do not apply resources to a live Kubernetes cluster by default.
- Do not create or commit real secrets.
- Do not add Helm/GitOps tooling in this PR.
- Do not change runtime APIs, event schemas, or database schemas.
- Do not mark the product as Public GA or Enterprise Ready.
- Do not remove unrelated blockers such as missing reference snapshot,
  `function.md`, per-package coverage gaps, or remaining physical
  shared-Postgres transition debt.

## 6. Current Behavior

Operators can run `bash backend/scripts/ci-security-gate.sh all`, which executes
quick, Docker-backed E2E, security, and Sonar gates. Separately, they can run
`kubectl kustomize backend` manually.

There is no single command that:

- renders the production-beta 15-service kustomization,
- validates client-side deploy dry-run,
- records rollback commands for every service deployment,
- validates re-deploy dry-run,
- writes a release-candidate evidence report, and
- documents which blockers remain accepted/non-blocking versus release-blocking.

## 7. Target Behavior

`bash backend/scripts/ci-security-gate.sh beta-rc` runs:

1. quick Go quality checks,
2. production-beta manifest rehearsal,
3. Docker-backed migrations, integration coverage, focused E2E, and full
   non-live E2E,
4. non-live runtime smoke against `SERVICE_NAME=all` for `/healthz`, `/readyz`,
   `/metrics`, `/openapi.json`, `/service-registry`, and one read-only endpoint
   per service with no 5xx,
5. govulncheck, OSV, image build, and Trivy scan,
6. SonarScanner Quality Gate when configured or required, and
7. a generated `beta-rc-report.md` artifact.

The manifest rehearsal:

- runs `kubectl kustomize backend`,
- verifies the rendered output contains all 15 service deployments,
- verifies the all-in-one `platform` deployment is absent,
- verifies no `-dev-` secret names remain in the production-beta render,
- runs `kubectl apply --dry-run=client --validate=false` for deploy,
- writes `kubectl rollout undo deployment/<service> -n nexuspaas` commands for
  all 15 services, and
- runs a second client-side apply dry-run to rehearse re-deploy.

Documentation explains that this is the non-live Beta RC gate. A true external
Beta launch still requires a live staging rehearsal with real secrets,
readiness, smoke, rollback, and re-deploy evidence.

## 8. Affected Domains

- Release engineering
- Production Beta deployment readiness
- CI/security gate orchestration
- Operator documentation
- Launch blocker tracking

## 9. Affected Files

- `backend/scripts/ci-security-gate.sh`
- `backend/docs/e2e-testing.md`
- `backend/docs/beta-launch-readiness.md`
- `backend/internal/platform/deployment_test.go`
- `.gitignore`
- `problem.md`
- this plan file

## 10. API / Contract Changes

No runtime API or event contract changes.

The release engineering contract gains one CLI subcommand:

```bash
bash backend/scripts/ci-security-gate.sh beta-rc
```

## 11. Database / Migration Changes

None.

The Beta RC gate reuses the existing Docker-backed migration apply/validate
flow from the `docker` gate.

## 12. Configuration Changes

No runtime configuration changes.

The script may add gate-only environment variables for artifact locations and
optional command behavior, but it must keep defaults non-mutating and local.

## 13. Observability Changes

The generated Beta RC report records evidence paths for rendered manifests,
deploy dry-run output, rollback command plan, re-deploy dry-run output, Docker
E2E logs, security scans, and Sonar status.

No runtime telemetry changes are made.

## 14. Security Considerations

- The gate must not write real secret values.
- Production-beta rendered manifests must not reference `*-dev-*` secret names.
- The rollback command artifact must contain commands only, not credentials.
- Security scans remain part of the Beta RC gate.
- The release-readiness documentation must make live staging secrets an
  operator prerequisite rather than generating insecure defaults.

## 15. Implementation Steps

1. Extend `backend/scripts/ci-security-gate.sh` with a `beta-rc` subcommand.
2. Add a manifest rehearsal function that renders `backend/`, validates the 15
   service deployments, excludes the all-in-one platform deployment and dev
   secret references, runs deploy/re-deploy client dry-runs, and writes rollback
   commands for every service.
3. Add a Docker-backed runtime smoke phase after full non-live E2E that starts
   `SERVICE_NAME=all` on an alternate local port, checks core runtime endpoints,
   verifies `/service-registry` lists 15 services, and records per-service
   smoke status with no 5xx.
4. Generate `${ARTIFACT_DIR}/beta-rc-report.md` after all gate phases pass.
5. Update `backend/docs/e2e-testing.md` with the new `beta-rc` gate, artifact
   outputs, and live staging caveat.
6. Add `backend/docs/beta-launch-readiness.md` with the Production Beta RC
   checklist, non-live gate evidence, live staging prerequisites, rollback
   expectations, and remaining issue policy.
7. Add `.gitignore` coverage for `backend/.e2e-gate/` so local gate artifacts
   cannot pollute commits.
8. Add platform tests proving the script and docs include the Beta RC gate,
   manifest rehearsal, migration/E2E/security/Sonar coverage, runtime smoke,
   rollback, re-deploy, and live staging caveat.
9. Update `problem.md` to record that a non-live RC rehearsal gate exists while
   live staging evidence remains a blocker before external Beta traffic.

## 16. Verification Plan

Run at least:

```bash
bash -n backend/scripts/ci-security-gate.sh
cd backend && test -z "$(gofmt -l .)"
cd backend && go test ./internal/platform -run 'Deployment|Operational|Release|Beta' -count=1
bash backend/scripts/ci-security-gate.sh beta-rc
git diff --check
```

The `beta-rc` gate itself runs quick, Docker-backed E2E, security, and Sonar
phases. If Sonar is not configured, the result must be documented as skipped
only when the existing script policy permits it.

## 17. Rollback Plan

Revert this PR. Because it changes only release scripts, docs, tests, and
problem tracking, rollback removes the `beta-rc` subcommand and associated
documentation without affecting runtime services, database state, or deployed
resources.

## 18. Risks and Tradeoffs

- The default gate is non-live, so it cannot prove pod readiness, real ingress,
  or live rollback behavior by itself.
- Running `beta-rc` is intentionally heavier than `all` because it also renders
  and dry-runs production-beta manifests.
- The rollback rehearsal writes command evidence and validates re-deploy dry-run;
  it does not execute `kubectl rollout undo` without live staging context.
- Runtime smoke starts an all-in-one process, not 15 separate pods. It proves
  application-level endpoints and registry coverage while live 15-service pod
  readiness remains a staging requirement.
- Keeping live staging as an explicit remaining blocker avoids falsely claiming
  external launch readiness before real secrets and cluster evidence exist.

## 19. Reviewer Checklist

- The plan directly advances the Beta Launch Hardening roadmap item.
- The new gate is non-mutating by default and does not commit secrets.
- Manifest rehearsal covers render, deploy dry-run, 15 services, rollback
  command plan, and re-deploy dry-run.
- Runtime smoke covers health, readiness, metrics, OpenAPI, service registry,
  and one read-only endpoint per service with no 5xx.
- Existing quick/docker/security/Sonar gates remain intact.
- Documentation distinguishes non-live RC evidence from live staging launch
  approval.
- `problem.md` keeps unresolved blockers visible.
- Scope is limited to release engineering docs/tests/scripts.

## 20. Status

Status: Approved
