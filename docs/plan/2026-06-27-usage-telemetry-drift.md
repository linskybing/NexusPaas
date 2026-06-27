# Usage Drift Alerting for Reserved vs Observed GPU Telemetry

## 1. Objective

For `USAGE-029`, add drift alerting in `usage-observability-service` so it emits a `UsageDriftDetected` event when a project’s reserved GPU allocation materially diverges from fresh observed GPU telemetry.

This is a local, control-plane evidence slice that reuses the existing `gpuusage` collector path and `resourcehours` service specification metadata.

## 2. Background

The acceptance criterion currently requires an alert for drift between reservation state and observed telemetry:

- `docs/acceptance/usage-attribution.md` (line: `USAGE-029`)

Current implementation already computes:

- `GET /api/v1/projects/{id}/gpu-usage` (in `clusterread`) separates `observed_gpu_pods`, `reserved_gpu_fraction`, and `sm_attribution_source`.
- `gpuusage` collector writes fresh snapshots into `job_gpu_usage_snapshots` and summaries into `job_gpu_usage_summaries`.
- No event is emitted today when reservation and telemetry diverge.

## 3. Source References

- `backend/internal/services/gpuusage/collector.go`
- `backend/internal/services/gpuusage/collector_test.go`
- `backend/internal/services/clusterread/gpu_usage.go`
- `backend/internal/services/resourcehours/spec.go`
- `backend/internal/contracts/fixtures/events/v1/reservation-drift-detected.json` (reference pattern)
- `backend/internal/contracts/event_envelope_test.go`
- `backend/internal/contracts/fixtures/events/v1/*.json`

## 4. Assumptions

- “Fresh” observed telemetry means rows from `job_gpu_usage_snapshots` with timestamps inside `GPUUsageSnapshotWindowMin`.
- “Material” divergence is based on a deterministic threshold applied to the difference between reserved and observed fractional GPU counts.
- Drift detection only evaluates projects with both a positive reserved GPU fraction and fresh observed telemetry. Missing/zero reserved evidence is skipped so the detector does not create divide-by-zero ratios or false positives.
- Equivalent drift alerts must be de-duplicated through persisted internal alert state, not only `contracts.Event.IdempotencyKey`, because the in-memory event stream does not suppress repeated events by idempotency key.
- No immediate remediation is required in this slice; only alerting via event emission.
- No SQL DDL is required. If state is needed, use a new generic-store resource key under `platform_records`.
- `usage-observability-service` owns snapshot and summary records and can publish internal events via `app.Events`.

## 5. Non-Goals

- No new public HTTP API routes.
- No automatic quota rollback/release on drift.
- No owner-read writes in `gpuusage` beyond snapshot-consumption reads.
- No new hardware-level telemetry source or node-side GPU collector changes.
- No cross-service service ownership changes.

## 6. Current Behavior

- `gpuusage` collector normalizes pod rows and writes snapshots each maintenance tick.
- `clusterread` endpoint already calculates observed pods and reserved fraction with fallbacks (`clusterReadModel -> snapshots -> workload jobs`).
- `resourcehours` event list currently declares only `UsageSnapshotRecorded` and `ResourceHoursSummarized`.
- Contract fixture validation (`backend/internal/contracts/event_envelope_test.go`) has fixed event-file and event-type maps.

## 7. Target Behavior

Add drift detection in the `gpuusage` maintenance flow:

- For each project with active fresh telemetry rows:
  - compute `reserved_fraction` from the row-level allocation fields (`reserved_gpu_fraction`, `dra_effective_gpu`, `requested_gpu`, `gpu_count`/`sm_percentage`, `mps_virtual_units`).
  - compute `observed_fraction` from fresh rows (using existing snapshot unit logic: 1 GPU by default, `mps_virtual_units / 100` when present).
  - skip the project when reserved evidence is absent or `reserved_fraction <= 0`.
  - compare with a fixed materiality gate: absolute diff `>= 0.25` GPU **and** relative diff `>= 25%` of reserved GPU.
- Persist alert state in an internal `usage_drift_alerts` generic resource keyed by `project_id + reason`.
  - Store `status`, `fingerprint`, `reserved_gpu_fraction`, `observed_gpu_fraction`, `detected_at`, and `last_seen_at`.
  - Fingerprint uses deterministic rounded numeric values and reason, not volatile sample timestamps.
  - Emit `UsageDriftDetected` only when the alert is new or the fingerprint changes.
  - If drift falls below the gate, mark an existing alert resolved without emitting a new event.
- emit at most one `UsageDriftDetected` event per project/reason per collector pass when the gate is exceeded and persisted alert state says it is new or changed.
- event payload includes project id, reserved value, observed value, drift value, drift ratio, basis/window, sample timestamps, and a compact reason string.

Acceptance criteria (plan-level):

- when reserved and observed are materially different, a `UsageDriftDetected` event is published.
- no event when reserved evidence is missing/zero, telemetry is missing, or telemetry is outside the freshness window for a project.
- no event when the divergence is below materiality gate.
- duplicate equivalent drift across repeated collector passes is coalesced by `usage_drift_alerts`.
- runtime event metadata includes `OccurredAt`, `TraceID`, schema version, and a stable idempotency key even though local de-duplication is enforced by persisted alert state.

## 8. Affected Domains

- `gpuusage` maintenance/collector domain for metric drift detection.
- `resourcehours` event contract metadata.
- Internal contract fixtures and envelope validation.

## 9. Affected Files

Likely touched:

- `backend/internal/services/gpuusage/collector.go` (or split helper file, e.g. `backend/internal/services/gpuusage/drift_detector.go`)
- `backend/internal/services/gpuusage/collector_test.go`
- `backend/internal/services/resourcehours/spec.go`
- `backend/internal/contracts/fixtures/events/v1/usage-drift-detected.json`
- `backend/internal/contracts/event_envelope_test.go`

## 10. API / Contract Changes

- No new external API contract.
- Add internal event contract: `UsageDriftDetected` (producer: `usage-observability-service`) to:
  - `backend/internal/services/resourcehours/spec.go` (`Spec().Events`)
  - `backend/internal/services/resourcehours/spec.go` (`Spec().Tables`) for internal `usage_drift_alerts`
  - `backend/internal/contracts/fixtures/events/v1/usage-drift-detected.json`

Suggested event payload fields:

- `project_id`
- `reserved_gpu_fraction`
- `observed_gpu_fraction`
- `drift_gpu_fraction`
- `drift_ratio`
- `snapshot_window_minutes`
- `fresh_rows_seen`
- `detected_at`
- `reason` (string, e.g. `material_reservation_telemetry_divergence`)

## 11. Database / Migration Changes

No SQL migration expected. Reuse:

- `usage-observability-service:job_gpu_usage_snapshots` (for freshness and observed value)
- `usage-observability-service:cluster_read_models` (if needed for reservation fallback/source attribution)
- Existing record store APIs only.

Add one generic-store resource key:

- `usage-observability-service:usage_drift_alerts`

Payload fields:

- `id`
- `project_id`
- `reason`
- `status` (`active` or `resolved`)
- `fingerprint`
- `reserved_gpu_fraction`
- `observed_gpu_fraction`
- `drift_gpu_fraction`
- `drift_ratio`
- `snapshot_window_minutes`
- `detected_at`
- `last_seen_at`
- `resolved_at`

## 12. Configuration Changes

None initially.

Keep thresholds and gating constants local in `gpuusage` collector code unless threshold tuning is needed later.

## 13. Observability Changes

- Add collector logs when drift scan starts/completes and when drift is detected.
- Emit `UsageDriftDetected` as first-class signal for alerting pipelines.
- Keep event payload compact to avoid leaking request/credential context.

## 14. Security Considerations

- `UsageDriftDetected` contains IDs and float metrics only.
- No additional privileges or secret material.
- Preserve existing role/visibility checks for user-facing `/api/v1/projects/{id}/gpu-usage` (read path unchanged).

## 15. Implementation Steps

1. Add drift detection logic in `gpuusage` maintenance flow.
   - Implement helper to compute fresh project telemetry stats (reserved + observed).
   - Reuse existing snapshot freshness window from `app.Config.GPUUsageSnapshotWindowMin`.
   - Reuse the same allocation field precedence as `clusterread` where possible: `reserved_gpu_fraction`, `dra_effective_gpu`, `requested_gpu`, `gpu_count * sm_percentage / 100`, `mps_virtual_units / 100`.
   - Skip rows/projects with no positive reserved fraction.
   - Keep detection deterministic and bounded; skip projects without recent snapshots.

2. Emit `UsageDriftDetected` events from maintenance task.
   - Add typed event payload with reason + threshold + drift values.
   - Before publishing, upsert `usage_drift_alerts` and only publish when alert state is new or the deterministic fingerprint changed.
   - Attach a stable idempotency key for downstream consumers, but do not rely on it for local de-duplication.
   - Register/trigger from existing maintenance task path in `Register`.

3. Update service contract declarations.
   - Add `UsageDriftDetected` to `resourcehours.Spec().Events`.
   - Add `usage_drift_alerts` to `resourcehours.Spec().Tables`.
   - Add fixture `backend/internal/contracts/fixtures/events/v1/usage-drift-detected.json`.
   - Update fixture validation expectations (`event_envelope_test.go`) for file list and type->producer map.

4. Add focused tests.
   - New/updated `collector_test.go` cases for:
     - drift emitted when material divergence exists,
     - repeated collector pass with unchanged drift does not emit a duplicate event,
     - runtime event data has expected payload, `OccurredAt`, `TraceID`, schema version, and idempotency key,
     - no event when below threshold,
     - no event when reserved fraction is missing/zero,
     - no event when snapshots are stale/outside window.
   - Ensure event fixture decode/validation test stays green with new fixture.

## 16. Verification Plan

- Targeted:

  ```bash
  cd backend && go test ./internal/services/gpuusage -run "Drift|Collector"
  cd backend && go test ./internal/contracts -run EventEnvelopeFixturesAreValidV1
  cd backend && go test ./internal/services/resourcehours
  ```

- Broader:

  ```bash
  cd backend && go test ./...
  cd backend && go build ./...
  cd backend && make ci-sonar
  ```

- Manual spot-check:
  - Run a collector-maintenance tick with seeded snapshot rows and confirm `UsageDriftDetected` appears on the event stream, while aligned reserved/observed data does not.

## 17. Rollback Plan

- Remove drift emission function and callsites from collector.
- Revert `resourcehours` event/table list and fixture additions.
- Stop writing `usage_drift_alerts`; existing generic records can be ignored or cleaned by a follow-up retention task if needed.
- Keep snapshot collection and read-model paths unchanged.

## 18. Risks and Tradeoffs

- False positives can occur if a project is intentionally over-reserved but not fully active.
- Divergence interpretation depends on snapshot field completeness (`reserved_gpu_fraction` and `mps_virtual_units` may be missing); fallback behavior needs a conservative default.
- Zero/missing reserved evidence is intentionally skipped in this slice; stale/missing telemetry behavior remains under `USAGE-032` and `USAGE-033`.
- Event volume risk if thresholds are too low; enforce a strict materiality gate and coalescing logic first.
- Adding fixture/contract assertions may fail unrelated event fixture ordering if the fixture set changes.

## 19. Reviewer Checklist

- [ ] Target behavior maps directly to `USAGE-029`.
- [ ] Plan scope is limited to drift signal emission; no unrelated service/domain changes.
- [ ] Divergence is measured on fresh snapshots only.
- [ ] Missing/zero reserved evidence is skipped, with no divide-by-zero ratio.
- [ ] Equivalent repeated drift is suppressed through persisted `usage_drift_alerts` state.
- [ ] `resourcehours` event contract is updated for `UsageDriftDetected`.
- [ ] `resourcehours` table metadata includes `usage_drift_alerts`.
- [ ] Event fixture is added and `event_envelope_test.go` is updated accordingly.
- [ ] Runtime tests verify event payload and metadata, not only fixture shape.
- [ ] Verification commands are concrete and runnable.
- [ ] Rollback path is explicit and limited.

## 20. Status

Plan Status: Approved by Reviewer Agent after revision
