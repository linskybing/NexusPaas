# Environment Profile And PDP Fail-Closed

## 1. Objective

Close the P0 blocker that still depends on a production boolean by adding an
explicit runtime profile and making staging/production fail closed when the PDP
is not real.

## 2. Background

`problem.md` lists `Environment profiles and PDP fail-closed` as a P0 GA
blocker. Today `Config` uses `PRODUCTION` as the main mode switch and
`EnvironmentName()` maps only to `production` or `development`. Startup checks
also fail hard only when `cfg.Production` is true, so a staging deployment can
accidentally run with warning-only checks and an `AllowAllPDP`.

The code already has a good strict production validation surface. The smallest
safe GA slice is to add an explicit profile on top of that surface rather than
rewriting config.

## 3. Source References

- `problem.md`
- `backend/internal/platform/config.go`
- `backend/internal/platform/config_test.go`
- `backend/internal/platform/policy.go`
- `backend/cmd/microservice/main.go`
- `backend/deploy/k3s/production-beta/backend-units.yaml`
- `backend/internal/platform/deployment_test.go`

## 4. Assumptions

- `APP_ENV` is the primary explicit profile env var.
- Valid profiles are `local`, `test`, `dev`, `staging`, and `production`.
- `PRODUCTION` remains supported for compatibility, but `APP_ENV` is the
  user-facing profile source.
- `staging` and `production` are strict runtime profiles.
- `production` profile implies the existing production validation behavior.

## 5. Non-Goals

- No new policy engine, service mesh, SPIFFE, or workload identity.
- No removal of the legacy `PRODUCTION` env var in this slice.
- No broad manifest topology rewrite.
- No change to normal route PDP enforcement semantics.

## 6. Current Behavior

- `EnvironmentName()` returns only `production` or `development`.
- Non-production startup checks warn instead of failing.
- `PRODUCTION=false` plus no remote/local real PDP can still start with
  `AllowAllPDP` outside production.
- Production Beta manifests do not declare an explicit profile separate from
  `PRODUCTION`.

## 7. Target Behavior

- `Config` has an explicit profile from `APP_ENV` with allowed values:
  `local`, `test`, `dev`, `staging`, `production`.
- Missing `APP_ENV` falls back to `PRODUCTION` for compatibility:
  `production` when `PRODUCTION=true`, otherwise `dev`.
- Invalid explicit profiles fail validation.
- `staging` and `production` use strict startup checks, including PDP
  fail-closed behavior.
- Production Beta manifests set `APP_ENV: "production"`.

## 8. Affected Domains

- Platform configuration parsing and validation.
- Microservice startup check mode selection.
- Production Beta deployment manifests and manifest tests.

## 9. Affected Files

- `docs/plan/2026-06-20-environment-profile-pdp-fail-closed.md`
- `backend/internal/platform/config.go`
- `backend/internal/platform/config_test.go`
- `backend/cmd/microservice/main.go`
- `backend/deploy/k3s/production-beta/backend-units.yaml`
- `backend/internal/platform/deployment_test.go`

## 10. API / Contract Changes

No HTTP API changes. Operational config contract adds `APP_ENV`.

## 11. Database / Migration Changes

None.

## 12. Configuration Changes

Add `APP_ENV` with allowed values `local`, `test`, `dev`, `staging`, and
`production`. Keep `PRODUCTION` as a compatibility switch during migration.

## 13. Observability Changes

OpenTelemetry deployment environment uses the explicit profile value.

## 14. Security Considerations

Staging and production must not run with `AllowAllPDP` when auth is required.
Invalid or conflicting profile settings must fail closed rather than silently
downgrading validation mode.

## 15. Implementation Steps

1. Add `EnvironmentProfile` and `APP_ENV` parsing/normalization.
2. Add helpers for effective profile and strict runtime mode.
3. Use strict runtime mode in config validation and startup checks.
4. Add config tests for allowed profiles, invalid profiles, legacy fallback,
   production compatibility, and staging PDP fail-closed startup behavior.
5. Add `APP_ENV: "production"` to Production Beta backend manifests and update
   manifest tests.
6. Run focused tests, full backend tests, quick gate, Sonar Quality Gate, and
   live RKE2 rollout/smoke.

## 16. Verification Plan

```sh
go -C backend test ./internal/platform -run 'Config.*Environment|Policy|Production|Deployment' -count=1
go -C backend test ./cmd/microservice -run 'Startup|Policy|Environment' -count=1
go -C backend test ./internal/platform ./cmd/microservice -count=1
go -C backend test ./... -count=1
bash backend/scripts/ci-security-gate.sh quick
bash backend/scripts/ci-security-gate.sh sonar
```

Live evidence:

```sh
kubectl -n nexuspaas get configmap <backend-config> -o yaml
kubectl -n nexuspaas rollout restart deployment/<backend-service>
kubectl -n nexuspaas rollout status deployment/<backend-service>
curl http://127.0.0.1:<port>/healthz
curl http://127.0.0.1:<port>/readyz
```

## 17. Rollback Plan

Revert this slice. Existing `PRODUCTION`-only behavior remains compatible, so
rollback does not require data migration.

## 18. Risks and Tradeoffs

Keeping `PRODUCTION` avoids a breaking deployment migration but temporarily
leaves two mode indicators. The validator should make conflicting strictness
obvious while allowing `APP_ENV=staging` with production-grade validation.

## 19. Reviewer Checklist

- Explicit profile exists and validates allowed values.
- Staging and production fail closed for missing/allow-all PDP.
- Legacy `PRODUCTION` deployments still behave as before.
- Production Beta manifests declare `APP_ENV`.
- Tests and live evidence cover the new strict path.

## 20. Status

Status: Approved

Implementation state: implemented; pending Reviewer Agent final verification.

Reviewer Agent plan approval: approved after adding an explicit conflict rule
for mixed `APP_ENV` and `PRODUCTION` settings.

Implementation summary:

- Added `Config.EnvironmentProfile`, `APP_ENV` parsing, effective profile
  helpers, production profile detection, and strict runtime mode detection.
- Kept legacy `PRODUCTION` compatibility while rejecting explicit conflicting
  `APP_ENV` and `PRODUCTION` values.
- Switched production validation and microservice startup checks to strict
  runtime/profile helpers, so `staging` and `production` fail closed.
- Added manifest coverage requiring production backend configmaps to declare
  `APP_ENV: "production"`.
- Added `APP_ENV: "production"` to Production Beta backend configmaps and the
  generated service deployment configmaps.

Verification completed:

```sh
go -C backend test ./internal/platform -run 'Config.*Environment|MalformedStaging|ValidateProductionGuards|Deployment|Policy|Production' -count=1
go -C backend test ./cmd/microservice -run 'Startup|Policy|Environment' -count=1
git diff --check -- backend/internal/platform/config.go backend/internal/platform/config_test.go backend/internal/platform/deployment_test.go backend/cmd/microservice/main.go backend/cmd/microservice/main_test.go backend/deploy/k3s/production-beta/backend-units.yaml backend/*/k8s/deployment.yaml
go -C backend test ./internal/platform ./cmd/microservice -count=1
kubectl kustomize backend
go -C backend test ./... -count=1
bash backend/scripts/ci-security-gate.sh quick
bash backend/scripts/ci-security-gate.sh sonar
```

SonarScanner result: `QUALITY GATE STATUS: PASSED`.

Live RKE2 evidence:

- Built and pushed
  `localhost:5000/nexuspaas-backend:ci-ga-env-20260620140631`.
- Image digest:
  `sha256:a1df56d826f38eb69f3472075786fdda5058393c956694eb1d39ebad58362c4b`.
- Patched 15 live backend configmaps in namespace `nexuspaas` with
  `APP_ENV=production`.
- Rolled 15 backend deployments to the new image.
- Verified backend pod readiness: `ready=15 total=15`.
- Verified production configmap profile coverage:
  `app_env_production=15 production_configmaps=15`.
- Verified running container env:
  `identity APP_ENV=production PRODUCTION=true`.
- Verified running container env:
  `gateway APP_ENV=production PRODUCTION=true`.
- Verified gateway live health:
  `GET /healthz` returned `{"success":true,"data":{"status":"ok"}}`.
- Verified gateway live readiness:
  `GET /readyz` returned `{"success":true,"data":{"status":"ok"}}`.
