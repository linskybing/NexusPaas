# GPU Usage Reserved vs Observed Attribution

## 1. Objective

Prepare the next local, non-hardware acceptance slice for `docs/acceptance/gpu-dra-mps.md`:

- **GPU-017:** usage/dashboard surfaces show reserved GPU fraction separately from observed GPU usage.
- **GPU-018:** when true per-process SM usage is unavailable, SM/process attribution is labeled as estimated/allocation-based instead of measured.

The implementation should stay in the existing split read-model/API/UI path and avoid real GPU hardware or DCGM integration:

- `clusterread` keeps serving `GET /api/v1/projects/{id}/gpu-usage`;
- `resourcehours.Spec()` keeps the route metadata for that endpoint;
- `gpuusage` keeps owning richer usage-observability snapshots, summaries, and MPS mapping/source-label behavior.

The Project GPU endpoint must not be moved to `gpuusage` in this slice.

## 2. Background

The repo currently splits this surface across packages:

- `clusterread` serves the Project GPU endpoint from the cluster read model;
- `resourcehours` declares the public route metadata;
- `gpuusage` owns `usage-observability-service` GPU snapshots/summaries and MPS mapping views.

Current local evidence already covers active-Project usage tables and a Project GPU pods summary, but that summary currently reports a single `used` value and does not clearly distinguish reserved allocation from observed usage. MPS mapping structs already carry `SMUtilizationSource` when source metadata exists, but blank source fields can still make allocation-derived SM attribution look measured.

## 3. Source References

- `docs/acceptance/gpu-dra-mps.md` (GPU-017 and GPU-018)
- `docs/acceptance/usage-attribution.md` (metric source rules and MPS attribution rules)
- `docs/acceptance/iteration-plan.md` (M6-006 reserved/observed/estimated/unavailable separation)
- `backend/internal/services/resourcehours/spec.go`
- `backend/internal/services/clusterread/handler.go`
- `backend/internal/services/clusterread/workflow_test.go`
- `backend/internal/services/gpuusage/handler.go`
- `backend/internal/services/gpuusage/helpers.go`
- `backend/internal/services/gpuusage/handler_test.go`
- `backend/internal/services/gpuusage/collector.go`
- `backend/internal/services/gpuusage/collector_test.go`
- `frontend/src/api.ts`
- `frontend/src/types.ts`
- `frontend/src/App.tsx`
- `frontend/src/App.test.tsx`
- `frontend/src/api.test.ts`

## 4. Assumptions

- Existing package ownership remains unchanged: `clusterread` owns the Project GPU endpoint implementation, `resourcehours` owns its route metadata, and `gpuusage` owns usage-observability snapshots/summaries/MPS mapping.
- No new microservice is justified; this is a contract/read-model extension in an existing bounded context.
- The route `/api/v1/projects/{id}/gpu-usage` must remain backward compatible for existing consumers that read `used`.
- Reserved GPU fraction can be derived from local read-model fields already present in this branch, with a fixed read-only source order documented below.
- Observed GPU usage in this slice means local cluster/read-model evidence such as pod GPU rows or retained snapshot metrics, not true per-process GPU SM measurement.
- If no explicit true measured SM source is present, MPS SM attribution must be labeled `estimated_mps_allocation` or equivalent allocation-based wording.

## 5. Non-Goals

- No real GPU hardware validation.
- No DCGM, NVML, Prometheus, or NVIDIA stack integration.
- No per-process PID/container attribution implementation beyond honest source labels on existing MPS/read-model surfaces.
- No billing policy change.
- No new database, queue, cache, service, or broad dashboard rewrite.
- No removal or rename of the existing `used` response field.

## 6. Current Behavior

- `frontend/src/App.tsx` fetches `/api/v1/projects/{projectID}/gpu-usage` and displays one Project GPU pods value.
- `frontend/src/types.ts` models `ProjectGPUUsage` as `{ used?: number; Used?: number }`.
- `backend/internal/services/clusterread/handler.go` returns `{"used": <pod count>}` plus telemetry metadata for `/api/v1/projects/{id}/gpu-usage`.
- `backend/internal/services/resourcehours/spec.go` declares route metadata for `/api/v1/projects/{id}/gpu-usage`.
- `backend/internal/services/gpuusage/helpers.go` returns MPS mapping rows with `SMUtilization` and `SMUtilizationSource`, but source can be empty.
- `backend/internal/services/gpuusage/collector.go` copies `gpu_sm_util_source` when present, but local fallback/derived metrics are not consistently labeled as allocation-based.

## 7. Target Behavior

- Project GPU usage response keeps `used` and adds explicit fields:
  - `observed_gpu_pods`: current observed GPU-backed pod/read-model count; this is the canonical observed field for this local slice.
  - `reserved_gpu_fraction`: sum of active reserved GPU fractions for the project.
  - `reserved_gpu_source`: e.g. `scheduler_allocation`, `mps_allocation`, or `unavailable`.
  - `observed_gpu_source`: e.g. `cluster_read_model`.
  - `sm_attribution_source`: `measured` only when true source metadata exists; otherwise `estimated_mps_allocation` for MPS allocation-derived rows or `unavailable`.
- Frontend Project GPU summary shows reserved GPU fraction and observed GPU usage as separate values.
- MPS mapping and usage-facing rows include a non-empty SM attribution label when MPS allocation data exists but measured per-process SM usage does not.
- Existing `used` consumers continue to work.

## 8. Affected Domains

- `clusterread`: Project GPU usage route implementation.
- `resourcehours`: existing route metadata for the Project GPU usage route.
- `gpuusage`: usage-observability read models, snapshots/summaries, and MPS mapping DTOs.
- Frontend operations dashboard: Project GPU summary and API types/client tests.
- Contracts/fixtures: API fixture coverage for the extended Project GPU usage response, if the current contract fixture suite supports this route.

## 9. Affected Files

Likely backend files:

- `backend/internal/services/clusterread/handler.go`
- `backend/internal/services/clusterread/workflow_test.go`
- `backend/internal/services/gpuusage/handler.go`
- `backend/internal/services/gpuusage/helpers.go`
- `backend/internal/services/gpuusage/handler_test.go`
- `backend/internal/services/gpuusage/collector.go`
- `backend/internal/services/gpuusage/collector_test.go`
- `backend/internal/services/resourcehours/spec.go` only if route metadata needs a description/resource adjustment.
- `backend/internal/contracts/fixtures/api/v1/<project-gpu-usage-fixture>.json` only if API fixtures cover this route.

Likely frontend files:

- `frontend/src/types.ts`
- `frontend/src/api.ts`
- `frontend/src/api.test.ts`
- `frontend/src/App.tsx`
- `frontend/src/App.test.tsx`

## 10. API / Contract Changes

Backward-compatible response extension for `GET /api/v1/projects/{id}/gpu-usage`.
The exact wire shape for this slice is:

```json
{
  "used": 2,
  "observed_gpu_pods": 2,
  "observed_gpu_source": "cluster_read_model",
  "reserved_gpu_fraction": 0.5,
  "reserved_gpu_source": "scheduler_allocation",
  "sm_attribution_source": "estimated_mps_allocation",
  "telemetry_stale": false,
  "collected_at": "2026-06-27T00:00:00Z"
}
```

`used` remains a compatibility alias for `observed_gpu_pods`. No
`observed_gpu_usage` field is added in this slice because its unit is ambiguous.

MPS mapping rows should add or consistently populate source fields without breaking existing names:

- keep `SMUtilization` and `SMUtilizationSource`;
- add `ReservedSMPercentage` or equivalent if needed to avoid overloading measured utilization;
- add `SMAttributionSource`/`SMAttributionMeasured` only if existing JSON output cannot express the distinction clearly.

## 11. Database / Migration Changes

None.

Use existing records/read models:

- `usage-observability-service:job_gpu_usage_snapshots`
- `usage-observability-service:job_gpu_usage_summaries`
- `usage-observability-service:gpu_jobs`
- `usage-observability-service:cluster_read_models`
- source owner-read records from `workload-service:jobs` when co-hosted/readable.

## 12. Configuration Changes

None.

## 13. Observability Changes

- No new telemetry backend.
- Preserve existing telemetry freshness metadata on project GPU usage responses.
- If a reserved fraction cannot be derived, return a source label such as `unavailable` rather than silently returning zero as measured/known data.
- Tests should assert source labels so future hardware integrations cannot regress into ambiguous wording.

## 14. Security Considerations

- Preserve current project visibility checks on `/api/v1/projects/{id}/gpu-usage`.
- Do not expose high-cardinality process identifiers, PIDs, container IDs, command lines, or pod UIDs.
- Keep admin-only MPS mapping behavior unchanged.
- Do not add frontend admin fallbacks for user/project usage.
- Source labels must not include secrets or raw provider error text.

## 15. Implementation Steps

1. Add backend helper(s) in `clusterread` to compute active Project reserved GPU fraction without moving the endpoint.
   - Source order:
     1. Existing cluster summary `pod_gpu_usages` rows filtered by Project, using explicit allocation fields if present: `reserved_gpu_fraction`, `dra_effective_gpu`, `requested_gpu`, `gpu_count * sm_percentage / 100`, then `mps_virtual_units / 100`.
     2. Existing `usage-observability-service:job_gpu_usage_snapshots` active read-model rows filtered by Project and snapshot freshness window, using the same allocation-field precedence.
     3. Co-hosted `workload-service:jobs` records only when locally available through the current store configuration; use active job statuses and `reservation_payload.reserved.gpu`, `required_gpu`, or `gpu_count * sm_percentage / 100`.
     4. If no supported source is available, return `reserved_gpu_fraction: 0` with `reserved_gpu_source: "unavailable"` rather than presenting the value as measured or known.
   - Do not add a remote owner-read client from `clusterread` in this slice.
   - Preserve existing cluster telemetry freshness metadata; snapshot fallback must respect the existing GPU usage snapshot window.

2. Extend `getProjectGPUUsage` in `backend/internal/services/clusterread/handler.go`.
   - Keep `used`.
   - Add `observed_gpu_pods`, `observed_gpu_source`, `reserved_gpu_fraction`, `reserved_gpu_source`, and `sm_attribution_source`.
   - Preserve telemetry metadata and authz behavior.

3. Make MPS SM attribution labels explicit in `gpuusage`.
   - When a row has true SM source metadata, preserve it.
   - When MPS allocation exists but measured source is missing, label source as `estimated_mps_allocation`.
   - Avoid presenting allocation-derived SM as measured utilization; add reserved/allocation fields if needed.

4. Update frontend API types and rendering.
   - Extend `ProjectGPUUsage`.
   - Change `ProjectGPUUsageSummary` to show reserved GPU fraction and observed GPU-backed pod count separately.
   - Display source/attribution labels in compact UI text without adding a large dashboard section.
   - Keep current failure isolation and no-admin-fallback behavior.

5. Add focused tests.
   - Backend clusterread test: project response includes old `used`, canonical `observed_gpu_pods`, reserved fraction, and source labels.
   - Backend gpuusage test: MPS rows with missing measured SM source are labeled `estimated_mps_allocation`.
   - Collector test if the source label belongs at snapshot normalization time.
   - Frontend API test: project GPU usage accepts the extended response.
   - Frontend App test: summary renders reserved and observed separately and displays estimated/allocation-based label.

6. Add/adjust contract fixture only if this route is already covered by the local API fixture pattern.
   - Keep fixture minimal and backward compatible.

## 16. Verification Plan

Focused commands:

```bash
cd backend && go test ./internal/services/clusterread -run "ProjectGPU|Telemetry"
cd backend && go test ./internal/services/gpuusage -run "MPS|GPUUsage|Collector"
npm --prefix frontend run test -- src/api.test.ts src/App.test.tsx
```

Broader local checks:

```bash
cd backend && go test ./internal/services/clusterread ./internal/services/gpuusage ./internal/contracts/...
cd backend && go build ./...
npm --prefix frontend run build
cd backend && make coverage
cd backend && make ci-sonar
```

Manual verification:

- Inspect `/api/v1/projects/{id}/gpu-usage` response shape and confirm `used` remains and equals `observed_gpu_pods`.
- Confirm dashboard copy separates reserved GPU fraction from observed GPU-backed pod count.
- Confirm any allocation-derived SM value is labeled `estimated_mps_allocation` or allocation-based.

## 17. Rollback Plan

- Revert the Project GPU usage response extension while keeping the existing `used` field path.
- Revert frontend Project GPU summary changes to the current single-value display.
- Revert MPS source-label additions only if they break consumers; keep tests documenting the ambiguity for a follow-up.
- Delete any added API fixture tied only to this slice.

## 18. Risks and Tradeoffs

- Reserved GPU fraction may be unavailable for older/local records. The safer behavior is an explicit `unavailable` source label, not pretending zero is measured.
- The existing Project GPU endpoint lives in `clusterread`, route metadata lives in `resourcehours`, and richer GPU snapshots live in `gpuusage`; keep those boundaries and avoid moving handlers.
- Field naming can drift between Go JSON defaults and frontend snake_case. Tests must cover the exact wire shape and must not accept `observed_gpu_usage` in place of `observed_gpu_pods`.
- This slice improves honesty of local usage surfaces, but does not prove real per-process SM measurement.
- Backward compatibility requires keeping `used`, even if new fields are clearer.

## 19. Reviewer Checklist

- [ ] Scope is limited to GPU-017 and GPU-018.
- [ ] No new service, database, hardware dependency, or DCGM integration is proposed.
- [ ] Existing ownership remains intact: endpoint in `clusterread`, route metadata in `resourcehours`, snapshots/MPS source labels in `gpuusage`.
- [ ] `/api/v1/projects/{id}/gpu-usage` keeps `used` and adds separate `observed_gpu_pods` and reserved fields.
- [ ] Frontend shows reserved GPU fraction separately from observed GPU-backed pod count.
- [ ] Allocation-derived SM attribution is labeled estimated/allocation-based.
- [ ] Tests cover backend response contract and frontend rendering.
- [ ] Security posture is unchanged: no high-cardinality process identifiers or admin fallback.
- [ ] Verification commands are runnable locally without GPU hardware.
- [ ] SonarScanner Quality Gate is run after implementation.

## 20. Status

Revised draft plan for Reviewer Agent review. No production code changes are part of this Plan Agent slice.
