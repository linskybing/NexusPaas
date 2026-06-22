# Harbor Outage Failure Injection Evidence

Status: Approved

## 1. Objective

Advance the next GA failure-injection gap by proving that a live Harbor
dependency outage is surfaced through the NexusPaaS product API as a clear
degraded state, then restore Harbor and update the GA ledgers with exact
evidence.

This slice targets partial OPS-012 evidence and the Harbor dependency portion of
OPS-019 only. Full OPS-012 remains open until image build/list workflows either
call Harbor directly or display/use this Harbor degraded state.

## 2. Background

Harbor is now deployed in `harbor-system` and prior slices proved Harbor backup,
restore, push, scan, and delete behavior. The current product runtime still does
not have `HARBOR_URL` set, so `/api/v1/harbor-status` currently reports
`adapter_not_configured`. That does not prove Harbor outage behavior.

The existing image-registry implementation already has the needed product
surface:

- `/api/v1/harbor-status`;
- `callHarbor`, which returns `adapter_unavailable` when the configured Harbor
  adapter cannot reach upstream;
- the shared response envelope with `degraded.adapter`, `degraded.code`,
  `degraded.message`, and `degraded.retryable`;
- the Harbor health maintenance read model.

The current image build/list routes use local records and do not directly call
the Harbor adapter. The smallest correct work is configuration plus a bounded
live outage drill, with narrow ledger wording.

## 3. Source References

- `docs/acceptance/operations.md`
- `docs/acceptance/gap-analysis.md`
- `gap.md`
- `problem.md`
- `backend/internal/services/imageregistry/handler.go`
- `backend/internal/services/imageregistry/helpers.go`
- `backend/internal/services/imageregistry/harbor_health.go`
- `backend/internal/platform/adapter.go`
- `backend/internal/platform/response.go`
- `backend/internal/platform/app.go`
- `backend/deploy/k3s/production-beta/runtime-config.yaml`

## 4. Assumptions

- Current Kubernetes context targets the live RKE2 environment.
- Namespace `nexuspaas` hosts the NexusPaaS API deployments.
- Namespace `harbor-system` hosts Harbor release `harbor`, chart
  `harbor-1.19.1`, app `2.15.1`.
- `production-beta-runtime-config` is consumed by the live `platform-gateway`
  and `image-registry-service` pods.
- `HARBOR_URL` is a non-secret 12-factor runtime configuration value.
- Static API key authentication may be used for read-only probes, but command
  output must not print key values, decoded values, hashes, or auth headers.

## 5. Non-Goals

- No new Harbor client, queue, sidecar, adapter abstraction, or retry framework.
- No product code changes unless the approved drill proves the existing API
  cannot express the outage.
- No Harbor data deletion, backup/restore mutation, registry artifact mutation,
  or credential rotation.
- No claim that full OPS-012 image build/list degradation is complete.
- No claim that DB, Redis, K8s API, Prometheus, node usage-agent, load, WebRTC,
  OIDC browser login, or full OPS-019 coverage is complete.
- No broad reapply of manifests that would overwrite live `SERVICE_URLS`.

## 6. Current Behavior

Read-only baseline on `2026-06-21`:

- `platform-gateway` consumes `platform-gateway-config`,
  `production-beta-runtime-config`, and `platform-gateway-runtime-secret`.
- `image-registry-service` consumes `image-registry-service-config`,
  `production-beta-runtime-config`, and `image-registry-service-runtime-secret`.
- Neither live ConfigMap nor relevant runtime Secret key names include
  `HARBOR_URL`.
- `GET /api/v1/harbor-status` through `platform-gateway` returns HTTP `200`
  with `degraded.code = "adapter_not_configured"`.

Harbor itself is ready:

- deployments `harbor-core`, `harbor-jobservice`, `harbor-nginx`,
  `harbor-portal`, and `harbor-registry` are `1/1`;
- statefulsets `harbor-database`, `harbor-redis`, and `harbor-trivy` are `1/1`;
- service `harbor` exists in `harbor-system` on port `80`.

## 7. Target Behavior

- `HARBOR_URL=http://harbor.harbor-system.svc.cluster.local/api/v2.0/ping` is
  present in the shared runtime ConfigMap and committed in the production-beta
  runtime manifest.
- Restarted API pods consume the new config.
- Baseline `GET /api/v1/harbor-status` returns HTTP `200` with product data
  indicating Harbor adapter success and no `degraded` envelope.
- During a bounded Harbor core outage, `GET /api/v1/harbor-status` returns HTTP
  `200` with `degraded.adapter="harbor"`, `degraded.code="adapter_unavailable"`
  or `degraded.code="circuit_open"`, and `degraded.retryable=true`.
- After restoring `harbor-core`, Harbor readiness returns to `1/1`, and
  `/api/v1/harbor-status` returns healthy again after the adapter circuit
  recovers or the API pod restarts.
- Ledgers record this as partial OPS-012 evidence: Harbor dependency outage is
  surfaced by the product API. Full image build/list outage behavior remains
  open.

## 8. Affected Domains

- Image registry / Harbor adapter boundary
- Live Kubernetes runtime configuration
- Failure-injection evidence
- GA acceptance ledgers

No microservice boundary changes are proposed.

## 9. Affected Files

- `docs/plan/2026-06-21-harbor-outage-failure-injection.md`
- `backend/deploy/k3s/production-beta/runtime-config.yaml`
- `gap.md`
- `problem.md`
- `docs/acceptance/gap-analysis.md`

## 10. API / Contract Changes

No API contract change.

The drill uses existing route:

- `GET /api/v1/harbor-status`

Expected outage response remains HTTP `200` with the existing degraded envelope.

## 11. Database / Migration Changes

None.

Optional read-only evidence may query the existing
`image-registry-service:harbor_health` read model if a maintenance pass records
the outage. The API degraded envelope is the required evidence; the read model
is supplemental only.

## 12. Configuration Changes

Repository:

- Add `HARBOR_URL:
  "http://harbor.harbor-system.svc.cluster.local/api/v2.0/ping"` to
  `backend/deploy/k3s/production-beta/runtime-config.yaml`.

Live cluster:

- Patch only key `data.HARBOR_URL` into ConfigMap
  `nexuspaas/production-beta-runtime-config`.
- Restart only deployments that need the new adapter config:
  `platform-gateway` and `image-registry-service` if present.
- Do not reapply the whole runtime ConfigMap in this slice.

Temporary outage:

- Scale `deployment/harbor-core` in `harbor-system` from `1` to `0`.
- Restore it to `1` before ledger updates.

## 13. Observability Changes

No persistent observability code/config change.

Evidence to capture:

- ConfigMap key presence for `HARBOR_URL` without secret output.
- Rollout readiness for affected API deployments.
- Harbor readiness before outage.
- Healthy `/api/v1/harbor-status` baseline.
- Read-only preflight that `http://harbor.../` and
  `http://harbor.../api/v2.0/ping` both return `200` while Harbor is healthy,
  and that the configured URL uses the API ping path so `harbor-core` outage is
  visible to the adapter.
- `harbor-core` scaled to `0`.
- Degraded `/api/v1/harbor-status` response during outage.
- `harbor-core` restored to `1` and rollout ready.
- Healthy `/api/v1/harbor-status` recovery.
- Final NexusPaaS deployment readiness and `git diff --check`.

## 14. Security Considerations

- Never print Kubernetes Secret data, decoded values, hashes, auth headers,
  Docker configs, or credentials.
- Use the existing static API key only inside local shell variables for product
  API probes.
- `HARBOR_URL` is non-secret service discovery config.
- The outage is limited to `harbor-core`; database, Redis, registry storage,
  and artifacts remain untouched.
- Restore Harbor before changing ledgers.

## 15. Implementation Steps

- [x] Capture preflight:
  - Harbor component readiness;
  - live ConfigMap key presence for `HARBOR_URL`;
  - affected API deployment envFrom references;
  - read-only `/api/v1/harbor-status` baseline showing current config gap.
- [x] Update `backend/deploy/k3s/production-beta/runtime-config.yaml` with
  `HARBOR_URL`.
- [x] Patch live `production-beta-runtime-config` with only `HARBOR_URL`.
- [x] Restart `platform-gateway` and `image-registry-service`; wait for rollout.
- [x] Probe `/api/v1/harbor-status` through `platform-gateway` and verify healthy
  Harbor adapter behavior.
- [x] Record original `harbor-core` replica count.
- [x] Scale `harbor-core` to `0`; wait until no ready `harbor-core` pod remains.
- [x] Probe `/api/v1/harbor-status` until it reports retryable Harbor degraded
  state or a bounded timeout expires.
- [x] Restore `harbor-core` to the original replica count and wait for rollout.
- [x] If the adapter circuit remains open after Harbor is ready, restart only the
  affected API deployment and re-probe healthy recovery. Record this as recovery
  after process refresh, not automatic circuit recovery. Not needed in the live
  run; recovery succeeded without process refresh.
- [x] Update `gap.md`, `problem.md`, and `docs/acceptance/gap-analysis.md` with
  exact evidence and narrow partial-OPS-012 claims.
- [x] Run scoped verification and submit to Reviewer Agent.

## 15.1 Completed Execution Evidence

Live drill stamp: `20260621200008`

Trace prefix: `ga-harbor-outage-20260621200008`

Results:

- Preflight Harbor components were ready:
  - deployments `harbor-core`, `harbor-jobservice`, `harbor-nginx`,
    `harbor-portal`, and `harbor-registry` were `1/1`;
  - statefulsets `harbor-database`, `harbor-redis`, and `harbor-trivy` were
    `1/1`.
- Live `production-beta-runtime-config` initially had `HARBOR_URL=absent`.
- `platform-gateway` and `image-registry-service` both consumed
  `production-beta-runtime-config` through `envFrom`.
- Live ConfigMap patch added only
  `HARBOR_URL=http://harbor.harbor-system.svc.cluster.local/api/v2.0/ping`.
- `platform-gateway` and `image-registry-service` rollout restarts completed.
- Healthy baseline through `platform-gateway`:
  - trace `ga-harbor-outage-20260621200008-healthy_baseline`;
  - HTTP envelope `success=true`;
  - data `status="ok"`, `adapter="harbor"`;
  - `degraded=null`.
- Outage injection:
  - original `harbor-core` replicas: `1`;
  - scaled `harbor-core` to `0`;
  - rollout showed `harbor-core 0/0`.
- Outage product probe:
  - trace `ga-harbor-outage-20260621200008-outage_1`;
  - HTTP envelope `success=true`;
  - `degraded.adapter="harbor"`;
  - `degraded.code="adapter_unavailable"`;
  - `degraded.retryable=true`;
  - data also reported `adapter="harbor"`, `degraded=true`,
    `retryable=true`.
- Recovery:
  - restored `harbor-core` to `1`;
  - `harbor-core` rollout returned to `1/1`;
  - trace `ga-harbor-outage-20260621200008-recovery_1`;
  - product probe returned data `status="ok"`, `adapter="harbor"`, and
    `degraded=null`;
  - no process-refresh restart was needed for recovery.
- Final readiness:
  - Harbor deployments `harbor-core`, `harbor-jobservice`, `harbor-nginx`,
    `harbor-portal`, and `harbor-registry` were `1/1`;
  - Harbor statefulsets `harbor-database`, `harbor-redis`, and `harbor-trivy`
    were `1/1`;
  - `platform-gateway` and `image-registry-service` were `1/1`.
- Scoped verification:
  - `git diff --check -- backend/deploy/k3s/production-beta/runtime-config.yaml
    problem.md gap.md docs/acceptance/gap-analysis.md
    docs/plan/2026-06-21-harbor-outage-failure-injection.md` passed;
  - live ConfigMap readback showed
    `HARBOR_URL=http://harbor.harbor-system.svc.cluster.local/api/v2.0/ping`;
  - final product probe
    `ga-harbor-outage-final-20260621200008` returned `success=true`,
    data `status="ok"`, `adapter="harbor"`, and `degraded=null`.

## 16. Verification Plan

Commands/checks:

- `kubectl -n harbor-system get deploy,sts,svc`
- `kubectl -n nexuspaas get cm production-beta-runtime-config -o json`
- `kubectl -n nexuspaas rollout status deploy/platform-gateway`
- `kubectl -n nexuspaas rollout status deploy/image-registry-service`
- authenticated `GET /api/v1/harbor-status` through `svc/platform-gateway`
- `kubectl -n harbor-system scale deploy/harbor-core --replicas=0`
- `kubectl -n harbor-system scale deploy/harbor-core --replicas=1`
- `kubectl -n harbor-system rollout status deploy/harbor-core`
- `git diff --check -- backend/deploy/k3s/production-beta/runtime-config.yaml problem.md gap.md docs/acceptance/gap-analysis.md docs/plan/2026-06-21-harbor-outage-failure-injection.md`

Success criteria:

- No secret value appears in output or docs.
- Healthy baseline proves the product is connected to live Harbor before outage.
- Outage response proves clear retryable Harbor dependency degradation through
  `/api/v1/harbor-status`.
- Recovery response proves Harbor returns healthy after restoration.
- Full OPS-012 image build/list behavior remains open in the ledgers.
- Final Harbor and NexusPaaS deployments are ready.

## 17. Rollback Plan

- Always restore `harbor-core` to its original replica count.
- If API rollout fails, undo the affected rollout:
  `kubectl -n nexuspaas rollout undo deploy/<name>`.
- If `HARBOR_URL` causes persistent runtime failure, remove only that key from
  the live ConfigMap and restart affected API deployments.
- Revert the manifest edit before completion if the live config proves invalid.
- Do not update ledgers unless Harbor and API deployments are healthy.

## 18. Risks and Tradeoffs

- Scaling `harbor-core` interrupts Harbor API/UI briefly. Registry/database/PV
  storage are not touched.
- The adapter circuit breaker may continue returning `circuit_open` after Harbor
  is restored. A bounded API pod restart is acceptable evidence of recovery from
  configured outage plus process refresh; record it if used.
- This proves Harbor dependency outage degradation only. It does not close full
  OPS-012 or full OPS-019.
- The live ConfigMap currently differs from the production-beta manifest in
  `SERVICE_URLS`; patching a single key avoids unrelated runtime churn.
- Harbor root `/` returns `200` through nginx/portal while healthy, so this plan
  intentionally configures the API ping path. A root URL would be weaker outage
  evidence for a core failure.

## 19. Reviewer Checklist

- [ ] Requirement fit: partial OPS-012 and Harbor part of OPS-019 only.
- [ ] Plan alignment: no product code unless a new approved plan is written.
- [ ] SOLID: uses existing adapter/response ports, no new abstraction.
- [ ] 12-Factor: Harbor endpoint is runtime config, not code or secret.
- [ ] CNCF/cloud-native: uses Kubernetes Deployment scale/rollout/readiness.
- [ ] Secret safety: no secret values, hashes, or auth headers in evidence.
- [ ] Evidence quality: healthy baseline, degraded Harbor-status outage,
  healthy recovery, and no full OPS-012 overclaim.
- [ ] Rollback: Harbor and API deployments restored before ledgers.

## 20. Status

Status: Approved
