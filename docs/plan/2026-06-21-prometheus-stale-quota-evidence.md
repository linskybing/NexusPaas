# Prometheus Stale Telemetry And Quota Evidence

Status: Completed after Reviewer approval and final pass.

## Objective

Close the smallest honest OPS-013 slice: telemetry read APIs expose stale
cluster/GPU data clearly, and scheduler admission does not use missing or stale
Prometheus telemetry to grant quota.

This is not full OPS-019 Prometheus failure-injection coverage until live
Prometheus is installed and can be deliberately interrupted.

## Current State

- `PROMETHEUS_URL` is absent from `production-beta-runtime-config`.
- Prometheus adapter routes exist through route metadata:
  - `/api/v1/cluster/mps`
  - `/api/v1/grafana/{path...}`
  - `/api/v1/internal/usage/snapshots`
- `clusterread` serves `/api/v1/cluster/summary`,
  `/api/v1/projects/gpu-usage/by-user`, and
  `/api/v1/projects/{id}/gpu-usage` from `cluster_read_models`.
- `cluster_read_models.summary.collectedAt` exists, but the read responses do
  not currently mark old snapshots as stale.
- `schedulerquota` admission uses active workload job records and plan/user
  quotas, not Prometheus, when deciding whether quota is exceeded.

## Non-Goals

- Do not install Prometheus or Prometheus Operator.
- Do not add a new telemetry store, sidecar, controller, dependency, or config
  knob.
- Do not claim full OPS-019 Prometheus failure-injection coverage.
- Do not change quota math.

## Proposed Change

Use the existing cluster snapshot timestamp and existing
`Config.MaintenanceInterval` to mark stale telemetry in read responses:

- Add small helper functions in `clusterread`:
  - read `collectedAt` from the summary;
  - compute stale when the timestamp is missing or older than
    `2 * MaintenanceInterval`;
  - return `telemetry_stale`, `telemetry_age_seconds`, and `collected_at`.
- Apply that metadata to:
  - `/api/v1/cluster/summary`;
  - `/api/v1/projects/{id}/gpu-usage`.
- Keep `/api/v1/projects/gpu-usage/by-user` as its existing bare
  `map[projectId]usedCount` response to avoid mixing metadata fields into a
  count map.
- Keep existing `used` values unchanged so clients can display last-known data
  while seeing that it is stale.
- Add focused tests in `clusterread` for fresh, older-than-window, and missing
  `collectedAt` summaries with `MaintenanceInterval` set explicitly.
- Reuse the existing schedulerquota quota-exceeded test as code evidence that
  quota is not granted from telemetry absence.

## API Contract

- `/api/v1/cluster/summary` keeps the existing summary fields and adds:
  - `telemetry_stale: bool`;
  - `telemetry_age_seconds: number`;
  - `collected_at: string`.
- `/api/v1/projects/{id}/gpu-usage` keeps `used` and adds the same telemetry
  metadata fields.
- `/api/v1/projects/gpu-usage/by-user` is unchanged and remains a bare
  project-id-to-count map.

## Files

- `backend/internal/services/clusterread/handler.go`
- `backend/internal/services/clusterread/workflow_test.go`
- `docs/plan/2026-06-21-prometheus-stale-quota-evidence.md`
- `gap.md`
- `problem.md`
- `docs/acceptance/gap-analysis.md`

## Rollback

- Revert the small `clusterread` response metadata change and tests.
- No database migration or runtime config cleanup is needed.
- Ledgers must be updated only after tests and live readback pass.

## Verification

- `go -C backend test ./internal/services/clusterread ./internal/services/schedulerquota -count=1`
- Live read-only evidence after rollout:
  - `PROMETHEUS_URL` remains absent or points to the tested Prometheus target;
  - `/api/v1/projects/{seeded-project}/gpu-usage` returns HTTP `200` with
    `telemetry_stale` and `telemetry_age_seconds`;
  - `/api/v1/cluster/summary` returns HTTP `200` with `telemetry_stale` and
    `telemetry_age_seconds`;
  - `/api/v1/cluster/mps` returns a degraded adapter response when Prometheus is
    unavailable or not configured;
  - an admission request that would exceed active job quota is rejected with
    quota exceeded while Prometheus remains unavailable/not configured.
- `git diff --check` on touched files.

## Execution Evidence

Reviewer approved the revised plan after API-contract and verification-command
changes. Final Reviewer pass returned `PASS` with no findings.

Implemented:

- `clusterread` now adds `telemetry_stale`, `telemetry_age_seconds`, and
  `collected_at` to `/api/v1/cluster/summary` and
  `/api/v1/projects/{id}/gpu-usage`.
- `/api/v1/projects/gpu-usage/by-user` remains the existing bare
  project-id-to-count map.
- Tests cover fresh telemetry, older-than-`2 * MaintenanceInterval`, and
  missing `collectedAt`.

Verification:

- `go -C backend test ./internal/services/clusterread ./internal/services/schedulerquota -count=1`
  passed.
- Built and pushed
  `localhost:5000/nexuspaas-backend:ci-ga-prometheus-stale-20260621205458`
  with digest
  `sha256:43a52e3875fa2ae9c0febf9a158537b7cdaeed97c7ef285f00ab9f5fee194a86`.
- Rolled out `usage-observability-service`; final pod image ID matched the new
  digest and rollout completed successfully.
- Live `/api/v1/cluster/summary` returned HTTP `200` with
  `telemetry_stale=false`, `telemetry_age_seconds=225`,
  `collected_at=2026-06-21T12:54:01Z`, and `nodeCount=1`.
- Live `/api/v1/projects/{synthetic}/gpu-usage` returned HTTP `200` with
  `telemetry_stale=false`, `telemetry_age_seconds=225`,
  `collected_at=2026-06-21T12:54:01Z`, and `used=0`; the exact synthetic
  `usage-observability-service:cluster_projects` row was deleted and cleanup
  readback returned `0`.
- Live `/api/v1/cluster/mps` returned HTTP `200` with degraded Prometheus
  adapter status `adapter_not_configured`.
- Live scheduler admission with exact synthetic owner-read rows returned HTTP
  `409` and reason
  `GPU quota exceeded: using 1.50, requested 1.00, limit 2.00` while
  Prometheus remained unavailable/not configured. The five exact synthetic
  rows were deleted and cleanup readback returned `0`.
- Final readiness showed `usage-observability-service`,
  `scheduler-quota-service`, and `platform-gateway` all `1/1`.
- Prefix cleanup readback returned `ops013_project_route_leftovers=0` and
  `ops013_quota_leftovers=0`.

## Acceptance Boundary

Ledgers are updated only as partial OPS-013 evidence: live Prometheus was not
installed or deliberately interrupted, so full OPS-019 remains open for
Prometheus.

## Reviewer Checklist

- [x] Requirement fit and acceptance boundary are honest.
- [x] No new dependency/config surface.
- [x] SOLID: stale calculation stays inside cluster read behavior; quota code is
  not coupled to telemetry.
- [x] 12-Factor: runtime config remains environment-driven.
- [x] CNCF/cloud-native: live proof uses Kubernetes rollout/readiness and
  Prometheus-adapter route behavior.
- [x] Tests cover stale telemetry marker and quota non-grant.
