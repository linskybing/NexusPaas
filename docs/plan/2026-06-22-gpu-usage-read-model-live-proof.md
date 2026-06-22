# GPU Usage Read Model Live Proof

## 1. Objective

Close the next WEB evidence gap by making `GET /api/v1/projects/{id}/gpu-usage`
report live Kubernetes job-pod GPU usage through the existing
usage-observability read model, then prove the first-party GUI can display a
nonzero value in live E2E.

This slice must reuse the existing Kubernetes adapter and read-model pattern. It
must not add a fake GPU route, test-only production fixture, new service, schema
migration, or custom metrics stack.

## 2. Background

The latest live GUI route proof showed the Project GPU route is reachable, but
`used` remained `0`. Code inspection found the route counts `podGpuUsages` from
the cluster read model, while `collectClusterResources` currently writes
`podGpuUsages: []`.

The backend already has a CNCF-native Kubernetes adapter,
`cluster.Client.ListJobPodResourceUsage`, which lists platform job pods by label
and extracts project/job/user IDs plus requested GPU. The read model should use
that adapter instead of inventing a second pod scanner.

## 3. Source References

- `docs/acceptance/gap-analysis.md`
- `docs/acceptance/webrtc.md`
- `gap.md`
- `problem.md`
- `frontend/tests/e2e/dashboard.spec.ts`
- `backend/internal/services/clusterread/cluster_resource_collector.go`
- `backend/internal/services/clusterread/cluster_resource_collector_test.go`
- `backend/internal/services/clusterread/handler.go`
- `backend/internal/services/clusterread/workflow_test.go`
- `backend/internal/platform/cluster/resource_usage.go`
- Official Grafana k6 docs for thresholds/checks/env driven load scripts:
  - https://grafana.com/docs/k6/latest/using-k6/thresholds/
  - https://grafana.com/docs/k6/latest/using-k6/checks/
  - https://grafana.com/docs/k6/latest/using-k6/environment-variables/

Context7 was attempted for k6 docs but the configured API key was invalid, so
official Grafana k6 documentation was used as fallback.

## 4. Assumptions

- Project GPU usage should count platform job pods with positive GPU request or
  effective DRA GPU labels.
- The stable ownership key is the platform project label
  `platform-go/project-id`. Namespace matching is only a backward-compatible
  fallback for older summary rows.
- A pending GPU pod is acceptable for live evidence if Kubernetes records it as
  an active platform job pod with a GPU request; the product metric is requested
  GPU pods, not confirmed GPU device utilization.
- Live proof may create a short-lived Kubernetes pod in a staging namespace and
  must delete it afterward. It must not print secrets.
- k6 remains useful for API performance gates, but this slice's primary
  acceptance proof is live Playwright GUI evidence with
  `NEXUSPAAS_E2E_EXPECT_NONZERO_GPU=true`.

## 5. Non-Goals

- Do not add Prometheus, DCGM, DRA ResourceSlice parsing, or GPU utilization
  metrics in this slice.
- Do not change the public API shape of `/api/v1/projects/{id}/gpu-usage`.
- Do not persist synthetic GPU data in production stores.
- Do not loosen authorization, service ownership, or gateway routing.
- Do not claim real GPU media streaming from this evidence.

## 6. Current Behavior

- `collectClusterResources` stores node capacity/requests and an empty
  `podGpuUsages` array.
- `/api/v1/projects/{id}/gpu-usage` counts rows by hard-coded namespace
  `project-{id}`.
- Live gateway and usage-observability calls can return 200, but `used` is `0`.
- The GUI E2E harness can optionally fail when GPU usage is not nonzero.

## 7. Target Behavior

- `collectClusterResources` calls `ListJobPodResourceUsage` after collecting node
  summary and writes `podGpuUsages` rows for active platform job pods with
  requested GPU greater than zero.
- Each row includes safe, non-secret fields needed by existing consumers:
  `job_id`, `project_id`, `user_id`, `namespace`, `pod_name`,
  `requested_gpu`, `timestamp`, `phase`, and `active`.
- The rows are explicitly project GPU pod-count rows, not per-device/MPS/DCGM
  telemetry rows. They intentionally do not invent `gpu_uuid`, `gpu_index`,
  `mps_virtual_units`, or GPU utilization metrics because the current
  Kubernetes adapter does not source those fields.
- `/api/v1/projects/{id}/gpu-usage` counts rows whose `project_id` matches the
  requested project, falling back to namespace matching for legacy rows.
- Existing public summary sanitization remains unchanged.
- The live GUI proof can create a staging GPU pod fixture, wait for collector
  refresh/read-model visibility, run Playwright with
  `NEXUSPAAS_E2E_EXPECT_NONZERO_GPU=true`, and cleanup the pod.
- When `NEXUSPAAS_E2E_EXPECT_NONZERO_GPU=true`, the Playwright route proof may
  poll the existing GPU usage route briefly before failing. This is test-harness
  readiness behavior only; the product API and UI remain unchanged.

## 8. Affected Domains

- Usage-observability cluster read model.
- Kubernetes adapter consumption.
- Existing GPU usage telemetry collector compatibility:
  `backend/internal/services/gpuusage/collector.go` reads `podGpuUsages` for
  per-device snapshots and should continue to skip rows that lack device
  identity. This slice must not turn project pod-count rows into synthetic MPS
  snapshots.
- Frontend live E2E evidence only; no UI behavior change is expected.

No new service boundary is introduced. Usage-observability remains the owner of
cluster/GPU read APIs; the Kubernetes adapter remains infrastructure.

## 9. Affected Files

- `backend/internal/services/clusterread/cluster_resource_collector.go`
- `backend/internal/services/clusterread/cluster_resource_collector_test.go`
- `backend/internal/services/clusterread/handler.go`
- `backend/internal/services/clusterread/workflow_test.go`
- `backend/internal/services/gpuusage/collector_test.go`
- `frontend/tests/e2e/dashboard.spec.ts`
- After live proof only: `docs/acceptance/webrtc.md`,
  `docs/acceptance/gap-analysis.md`, `gap.md`, `problem.md`

## 10. API / Contract Changes

No public API path or response shape change.

The meaning of `used` becomes the intended live value: count of active platform
job GPU pod rows for the project.

## 11. Database / Migration Changes

None. The existing single cluster read-model record stores the additional
`podGpuUsages` array.

## 12. Configuration Changes

No committed config change.

Live proof may temporarily create/delete Kubernetes resources in a staging
namespace. Any non-secret runtime observation such as route status, pod name, and
cleanup status may be recorded; secrets must not be printed.

## 13. Observability Changes

No new telemetry backend. Existing collector logs should naturally show
successful maintenance execution. E2E route proof already records `gpu_used` and
`gpu_nonzero`.

## 14. Security Considerations

- Do not expose pod internals beyond existing non-secret platform labels and
  resource-request metadata.
- Preserve auth checks in `getProjectGPUUsage`.
- Do not print API keys, service keys, database URLs, TURN secrets, or pod
  environment values during live proof.

## 15. Implementation Steps

1. Extend `collectClusterResources` to obtain `ListJobPodResourceUsage(ctx, now)`
   from the existing cluster client and pass the rows into `summaryMap`.
2. Add a small conversion helper that emits one project pod-count
   `podGpuUsages` row per active platform job pod with `RequestedGPU > 0`. Use
   existing adapter fields only; do not inspect arbitrary containers twice and
   do not synthesize per-device telemetry fields.
3. Update `getProjectGPUUsage` and `getProjectsGPUUsageByUser` to count by
   `project_id` first and namespace fallback second.
4. Update clusterread collector tests to assert `podGpuUsages` is populated from
   the fake Kubernetes client and terminal/non-GPU pods are excluded.
5. Update workflow tests so at least one fixture row proves `project_id`
   matching works without the legacy `project-{id}` namespace.
6. Add/extend `gpuusage` collector compatibility coverage so project pod-count
   rows without GPU device identity are scanned/skipped without writing
   snapshots or summaries.
7. Run focused backend tests for `clusterread`, `gpuusage`, and `cluster`
   packages.
8. If live enforced E2E proves the GPU fixture can become ready only after the
   current single route read, extend `frontend/tests/e2e/dashboard.spec.ts` so
   `NEXUSPAAS_E2E_EXPECT_NONZERO_GPU=true` polls
   `/api/v1/projects/{id}/gpu-usage` for a short bounded window before failing.
   The exact test contract is:
   - at most 10 attempts;
   - 1 second between attempts;
   - retry only when the response is HTTP 200 and `used == 0`;
   - stop immediately on HTTP non-200 or `used > 0`;
   - after the final attempt, fail if the final result is not HTTP 200 or
     `used <= 0`.
   The proof must still print the final route status/count and must not fake
   values.
9. Run `make -C backend lint`, `make -C backend build`, and
   `make -C backend ci-sonar`.
10. Build and deploy a live backend image for the changed service.
11. Create a short-lived staging GPU pod fixture with platform labels, wait for
   usage-observability read-model refresh, run Playwright with
   `NEXUSPAAS_E2E_EXPECT_NONZERO_GPU=true`, then delete the fixture and verify
   cleanup.
12. Update the acceptance trackers only with observed evidence.

## 16. Verification Plan

- `go test ./internal/services/clusterread ./internal/services/gpuusage ./internal/platform/cluster`
- `make -C backend lint`
- `make -C backend build`
- `make -C backend ci-sonar`
- If E2E harness changed: `npm --prefix frontend test -- --run` and
  `npm --prefix frontend run build`
- Live route proof:
  - `GET /api/v1/projects/{id}/gpu-usage` returns 200 and `used > 0` through
    platform-gateway for the staging fixture.
  - `NEXUSPAAS_E2E_EXPECT_NONZERO_GPU=true npm --prefix frontend run e2e`
    records `gpu_status=200`, `gpu_used > 0`, and `gpu_nonzero=true`.
- Optional follow-up performance proof may use k6 against the GPU route after
  the live route is stable; k6 docs confirm thresholds and checks are the right
  failure mechanism for API load gates.

## 17. Rollback Plan

- Revert the collector/helper and project-counting code changes.
- Revert the associated tests and tracker updates if live proof is invalidated.
- Redeploy the previous known-good backend image:
  `localhost:5000/nexuspaas-backend:ci-ga-web-stream-cred-20260622102018`.
- Delete any live staging GPU pod fixture and verify no `diag-*` or
  `e2e-*` Kubernetes resources remain.
- No database migration rollback is needed because this slice only updates the
  existing single cluster read-model record.

## 18. Risks and Tradeoffs

- **Cold read-model latency:** first request after deployment can still wait on
  projection/collector freshness. This slice does not solve gateway timeout
  context hygiene; that remains a separate narrow slice.
- **Pending pod semantics:** a GPU-requesting platform pod may count before it
  reaches Running if Kubernetes still reports it as active. This matches the
  current product label "GPU pods" and avoids claiming device utilization.
- **Telemetry scope:** rows do not include per-device UUID/index/utilization, so
  the existing GPU telemetry collector should skip them. That preserves honesty
  but means MPS/DCGM history still requires a later real telemetry adapter.
- **Kubernetes fixture risk:** live proof needs a temporary staging pod with GPU
  request labels. Cleanup must be exact and verified.
- **E2E timing risk:** the collector is asynchronous. A bounded E2E route poll
  is acceptable only under the explicit nonzero enforcement flag. It is capped
  at 10 attempts with 1 second between attempts and retries only HTTP 200 /
  `used == 0`; route errors fail instead of being hidden.

## 19. Reviewer Checklist

| Category | Plan Evidence | Status |
|---|---|---|
| Requirement fit | Sections 1, 6, 7, 16 limit this to Project GPU usage live evidence. | Ready for review |
| Scope control | Sections 5, 9, 15 exclude new APIs, schemas, telemetry stacks, and UI rewrites. | Ready for review |
| SOLID | Section 15 reuses `ListJobPodResourceUsage` and adds a conversion helper rather than another pod scanner. | Ready for review |
| 12-Factor | Sections 12, 14, 17 avoid committed environment config and secrets. | Ready for review |
| Microservice boundary | Sections 8, 10, 11 keep usage-observability as read-model/API owner. | Ready for review |
| Tests/build/Sonar | Section 16 includes focused tests, lint, build, Sonar, and live E2E. | Ready for review |
| Diff scope | Section 9 lists the full intended write set plus tracker-only updates after proof. | Ready for review |

## 20. Status

Status: Approved

Reviewer Agent approved the backend read-model plan. A first live enforced E2E
run then showed the GPU fixture route became ready after Playwright's single
GPU read, so this amendment adds a bounded E2E readiness poll under the existing
explicit enforcement flag. Reviewer Agent approved the amendment after the
polling contract was made deterministic.

## 21. Implementation Evidence

Implemented the approved read-model slice:

- `collectClusterResources` now reuses `cluster.Client.ListJobPodResourceUsage`
  and writes active GPU-requesting platform job pods into `podGpuUsages`.
- The new row shape is request metadata only: no synthetic GPU UUID, index, MPS,
  or utilization fields.
- Project GPU usage now counts by `project_id` first and falls back to legacy
  `project-{id}` namespace rows.
- The GPU telemetry collector compatibility test proves project pod-count rows
  without device identity are scanned/skipped and do not create MPS/DCGM
  snapshots.
- The Playwright proof harness now polls the GPU route only when
  `NEXUSPAAS_E2E_EXPECT_NONZERO_GPU=true`, with the approved 10 attempts / 1s /
  retry-only-200-and-zero contract.
- Reviewer Agent approved the implementation and noted one non-blocking
  diagnostic improvement: emit the final route-proof JSON before failing a
  nonzero GPU assertion. The harness now logs final `gpu_status` / `gpu_used`
  values before throwing, so failed live runs still leave proof context.

Verification passed:

- `go test ./internal/services/clusterread ./internal/services/gpuusage ./internal/platform/cluster`
- `make -C backend lint`
- `make -C backend build`
- `npm --prefix frontend test -- --run`
- `npm --prefix frontend run build`
- after the reviewer diagnostic improvement, reran
  `npm --prefix frontend test -- --run` and `npm --prefix frontend run build`
- `make -C backend ci-sonar`
  - first post-fix Quality Gate: `PASSED`
  - final rerun after frontend harness change: `PASSED`
  - one intermediate rerun failed from a transient Sonar JS/TS analyzer
    WebSocket execution error, not a Quality Gate failure; immediate rerun
    passed.

Live evidence:

- Built and pushed
  `localhost:5000/nexuspaas-backend:ci-ga-gpu-readmodel-20260622034034`
  with digest
  `sha256:2f0ebfc868a26fb59a9b3d20194756a9f8e2917b61397d50d80a16c9cde840c7`.
- Rolled only `usage-observability-service` to that image; final deployment
  reported `1/1` ready.
- Temporarily set `MAINTENANCE_INTERVAL=5s` and cleared only the
  `cluster-resource-collector` maintenance lease to accelerate fixture proof;
  removed that runtime env afterward and rolled the service back to default
  interval.
- Live Playwright enforced proof with
  `NEXUSPAAS_E2E_EXPECT_NONZERO_GPU=true` seeded Project
  `e2e-p-mqooctn3-fammye`, created a short-lived Kubernetes fixture pod
  `gpu-proof` in namespace `gpu-e2e-p-mqooctn3-fammye` with platform labels and
  `nvidia.com/gpu: "1"` request, and recorded:
  - `gpu_status=200`
  - `gpu_ok=true`
  - `gpu_used=1`
  - `gpu_nonzero=true`
  - monitor route readiness `status=200 used=1 attempt=2`
- Cleanup evidence:
  - E2E API cleanup returned HTTP `200` for ConfigFile, image build, Project
    image, Plan, Queue, Project, and Group.
  - Follow-up API readback for the seeded Project returned HTTP `404`.
  - Fixture namespace count was `0`.
  - `MAINTENANCE_INTERVAL` env count on `usage-observability-service` was `0`.

The live proof does not claim full WebRTC media streaming, real GPU utilization,
per-device MPS/DCGM telemetry, or non-empty job log tailing.
