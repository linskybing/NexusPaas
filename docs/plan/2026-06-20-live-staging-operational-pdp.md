# Live Staging Operational PDP Fix

## 1. Objective

Make the live Kubernetes smoke for authenticated operational endpoints pass when
the platform gateway uses a remote authorization-policy PDP.

## 2. Background

The local `beta-rc` gate passed, but the live `nexuspaas` namespace returned
`403` for authenticated `/metrics`, `/openapi.json`, and `/service-registry`.
Logs showed authentication succeeded as the admin user, then the gateway called
`authorization-policy-service` and the raw PDP denied because no route-level raw
permission seed exists for `platform-runtime:*` operational resources.

Operational endpoints are already:

- authenticated,
- admin-only, and
- owned by the runtime rather than a tenant/domain service.

They should not require user-managed raw permission rows to be available before
basic live smoke can inspect readiness, metrics, OpenAPI, and service registry.

## 3. Scope

- Set `PolicyBypass` only on platform-runtime operational routes.
- Keep authn, admin gate, rate limiting, request logging, and response envelopes
  unchanged.
- Do not bypass PDP for normal service/domain routes.
- Do not add a custom policy engine or seed mechanism.

## 4. Affected Files

- `backend/internal/platform/endpoints.go`
- `backend/internal/platform/policy_test.go`
- `docs/plan/2026-06-20-v1-launch-gap-gate.md`

## 5. Implementation Steps

- [x] Confirm live namespace failure mode with authenticated smoke and service
  logs.
- [x] Reviewer Agent approves this narrow route metadata fix.
- [x] Mark operational routes as `PolicyBypass`.
- [x] Add regression coverage proving admin operational routes work even when a
  configured PDP denies normal policy decisions.
- [x] Run focused platform tests.
- [x] Run quick/Sonar gates after the code change.
- [x] Re-run live smoke for `/healthz`, `/readyz`, `/metrics`, `/openapi.json`,
  and `/service-registry`.
- [x] Update V1 checklist with release and live evidence.

## 6. Verification Plan

```sh
go -C backend test ./internal/platform -run 'Operational|Policy|RemotePDP' -count=1
go -C backend test ./internal/platform -count=1
bash backend/scripts/ci-security-gate.sh quick
bash backend/scripts/ci-security-gate.sh sonar
```

Live smoke after deployment/config refresh:

```sh
curl -H "X-API-Key: <admin key>" http://127.0.0.1:18081/metrics
curl -H "X-API-Key: <admin key>" http://127.0.0.1:18081/openapi.json
curl -H "X-API-Key: <admin key>" http://127.0.0.1:18081/service-registry
```

Executed on 2026-06-20:

```sh
go -C backend test ./internal/platform -run 'Operational|Policy|RemotePDP' -count=1 -v
go -C backend test ./internal/platform -count=1
bash backend/scripts/ci-security-gate.sh quick
bash backend/scripts/ci-security-gate.sh sonar
TEST_MINIO_PORT=29000 TEST_MINIO_CONSOLE_PORT=29001 bash backend/scripts/ci-security-gate.sh beta-rc
```

Result: all commands passed. The final `beta-rc` wrote
`/tmp/nexuspaas-quality-gate/local-1710124/beta-rc-report.md`.

Live RKE2 staging evidence:

- Pushed `localhost:5000/nexuspaas-backend:ci-local-1710124` to the local
  registry and verified RKE2 can pull it with `nexuspaas-image-pull-test`.
- Rolled all 15 existing backend deployments in namespace `nexuspaas` to the RC
  image; every backend deployment reported `1/1` ready.
- `/healthz`, `/readyz`, authenticated `/metrics`, and authenticated
  `/openapi.json` passed through `http://127.0.0.1:18081`.
- Every backend service returned `200` for its isolated `/service-registry`;
  the union of those registry views covered all 15 logical services.
- Rehearsed `kubectl -n nexuspaas rollout undo deployment/platform-gateway`,
  then re-deployed the RC image and repeated gateway smoke.

## 7. Reviewer Checklist

| Check | Status |
|---|---|
| Requirement fit: live smoke can inspect authenticated operational surfaces | Approved |
| Scope: only platform-runtime operational routes bypass remote PDP | Approved |
| SOLID: route authorization metadata owns the decision; middleware stays unchanged | Approved |
| 12-Factor: no hard-coded environment-specific policy rows | Approved |
| CNCF/cloud-native: no new custom auth/policy component | Approved |
| Tests and quality gates | Pass |
| Live smoke evidence | Pass |

## 8. Status

Status: Implemented and reviewer-verified.

Reviewer Agent: Approved and verified. This is the smallest correction that
preserves the existing auth/admin controls while avoiding a bootstrap dependency
on raw authorization rows for core operational inspection endpoints. Focused
tests, quick gate, Sonar Quality Gate, full `beta-rc`, live rollout, smoke,
registry-union evidence, rollback, and re-deploy all passed.
