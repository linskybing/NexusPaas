# V1 External Deployment P0.1 CI/Sonar Gate Closure

## 1. Objective

Close the repository-side P0.1 gate for V1 external deployment by making
SonarScanner reproducible locally and fail-closed in trusted GitHub Actions
events. Live staging P0 items remain open until real staging kube context,
external registry credentials, production secrets, and Sonar repository secrets
are available.

## 2. Scope

This plan covers only repo-side release gate behavior:

- SonarScanner install cache validation and reinstall on partial/corrupt cache.
- GitHub Actions Sonar enforcement for push, workflow dispatch, and
  same-repository pull requests.
- Fork pull request Sonar skip only because trusted repository secrets are not
  exposed to forked workflows.
- Text-level regression tests and release documentation for the required
  behavior.
- Sonar Quality Gate remediation required to make the gate actually pass.

This plan does not claim closure for external registry promotion, live staging
8-unit deploy/smoke/rollback/redeploy, production secrets live path, or staging
database migration/rollback drills.

## 3. Affected Files

- `.github/workflows/backend-quality-gate.yml`
- `backend/scripts/ci-security-gate.sh`
- `backend/internal/platform/deployment_test.go`
- `backend/internal/platform/store_postgres_test.go`
- `backend/internal/services/schedulerquota/admission_test.go`
- `backend/deploy/k3s/production-beta/runtime-config.yaml`
- `backend/deploy/k3s/production-beta/runtime-secret-contract.yaml`
- `backend/deploy/local/docker-compose.yml`
- `backend/workload-service/templates/selkies-gl-desktop-configfile.yaml`
- `backend/docs/beta-launch-readiness.md`
- `backend/docs/e2e-testing.md`
- `docs/adr/0004-deployment-evidence-gates.md`
- `docs/architecture/ci-cd-and-pr-governance.md`
- `docs/architecture/nexuspaas-master-plan.md`
- `docs/architecture/open-source-quality-standard.md`
- `sonar-project.properties`
- `docs/plan/2026-06-23-v1-external-deploy-p0.md`

## 4. Implementation Steps

1. Update `install_sonar_scanner()` so a cached SonarScanner install is valid
   only when `bin/sonar-scanner` exists and `lib/*.jar` is present. Delete and
   reinstall incomplete cache directories.
2. Keep local Sonar optional unless `CI_GATE_SONAR_REQUIRED=true` or CI policy
   requires it. Missing `SONAR_TOKEN` or `SONAR_HOST_URL` must fail under
   required policy.
3. Update GitHub Actions so Sonar secrets are required for push,
   workflow_dispatch, and same-repository pull requests. Fork pull requests may
   skip Sonar because repository secrets are unavailable.
4. Add text-level deployment tests that assert the script, workflow, and docs
   preserve fail-closed Sonar behavior.
5. Keep production-beta render free of insecure external service defaults:
   `DEX_URL` and `HARBOR_URL` stay empty in the base and must be supplied by
   staging/production overlays with TLS endpoints.
6. Remove local/default secret literals from docs and local examples where they
   would weaken release-gate evidence.
7. Keep Sonar exclusions narrow: reference snapshots, Docker metadata, the
   external Selkies image recipe, dev-only local runtime config, and the
   documented coturn host-network exception.
8. Update release, E2E, ADR, and architecture docs to match remote Sonar
   enforcement and live staging evidence requirements.

## 5. Verification Plan

Run:

```sh
go -C backend test ./internal/platform -run 'ProductionBeta|ReleaseCandidate|Sonar' -count=1
go -C backend test ./internal/services/schedulerquota -run 'SubmitAdmissionStreamingGuardrails' -count=1
bash backend/scripts/ci-security-gate.sh quick
env -u SONAR_TOKEN -u SONAR_HOST_URL CI_GATE_SONAR_REQUIRED=true bash backend/scripts/ci-security-gate.sh sonar
CI_GATE_SONAR_REQUIRED=true bash backend/scripts/ci-security-gate.sh sonar
kubectl kustomize backend
```

The required-missing-secrets Sonar command must fail with the expected
fail-closed error. The configured Sonar command must pass the Quality Gate.
The kustomize render must include the 8 backend units, exclude the all-in-one
`platform` deployment, and contain no `-dev-` references.

## 6. Rollback Plan

Revert the scoped files above. No public API, route shape, database schema, or
runtime migration semantics change is introduced by this plan.

## 7. Live P0s Still Open

- P0.2 External registry promotion/rollback.
- P0.3 Production secrets live path.
- P0.4 Staging migration apply/validate/rollback drill.
- P0.5 8-unit live staging deploy/smoke/rollback/redeploy.

These require a real staging kube context, external registry credentials, real
runtime secrets, and remote GitHub/Sonar credentials before `problem.md` and
`gap.md` can move those items out of P0.

## 8. Status

Status: Approved after implementation review. Repo-side P0.1 is implemented;
live P0.2-P0.5 remain open pending external staging evidence.
