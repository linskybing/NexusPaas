# Usage Telemetry Stale Alerting for Scheduler-Reserved Jobs

## 1. Objective

Create a local control-plane slice for `USAGE-032` / `MON-018`:
`usage-observability-service` must emit an alert when active GPU-reserved jobs have no fresh `job_gpu_usage_snapshots`, while keeping quota admission/release behavior unchanged.

## 2. Background

- `clusterread` exposes telemetry freshness metadata in `GET /api/v1/projects/{id}/gpu-usage` via `telemetry_stale`, `collected_at`, and `telemetry_age_seconds`.
- `usage-observability-service` already has a usage drift detector that processes fresh snapshot rows, emits `UsageDriftDetected`, and deduplicates via `usage_drift_alerts`.
- Existing scheduler quota behavior in `scheduler-quota-service` uses workload admission records and `required_gpu`/`reservation_payload`, not live telemetry.

## 3. Source References

- `docs/acceptance/usage-attribution.md` (`USAGE-032`, `USAGE-033`, `MON-018` context)
- `backend/internal/services/clusterread/handler.go` (`telemetry_stale` contract)
- `backend/internal/services/clusterread/workflow_test.go` (current stale/missing metadata behavior)
- `backend/internal/services/gpuusage/collector.go` (maintenance flow and detector invocation)
- `backend/internal/services/gpuusage/usage_drift.go` (snapshot scan, active-job filtering, alert persistence)
- `backend/internal/services/gpuusage/collector_test.go` (drift tests)
- `backend/internal/services/resourcehours/spec.go` (events/tables declaration)
- `backend/internal/contracts/event_envelope_test.go` and `backend/internal/contracts/fixtures/events/v1/*.json`

## 4. Assumptions

- A job is “active reserved” when it is in active workload status and has a positive GPU reservation signal (`required_gpu` or `reservation_payload.reserved.gpu`).
- Freshness for this alert is governed by the existing `GPUUsageSnapshotWindowMin` window.
- “No fresh snapshots” means **no snapshot rows within the active window** for the job, not just no usable metrics.
- Scheduler quota stays based on workload/job reservation fields; telemetry is informational only.
- Reusing `UsageDriftDetected` is an explicit compatibility decision: alert consumers
  must dispatch by a fixed `reason`/`drift_reason` value, not by introducing a
  new event type.

## 5. Non-Goals

- No changes to scheduler-quota admission/release algorithms.
- No new DB migrations.
- No quota grant or release behavior tied to telemetry presence.
- No API response shape change for `clusterread` GPU usage endpoints.
- No real node-agent remediation wiring.

## 6. Current Behavior

- `clusterread` marks telemetry freshness on API responses via `telemetry_stale` but does not emit an explicit stale-missing alert event.
- `UsageDriftDetected` currently emits only when fresh snapshots exist and material reserved-vs-observed divergence thresholds are exceeded.
- Usage drift skips stale rows in `usage_drift.go`.
- `resourcehours` already owns `UsageDriftDetected` and `usage_drift_alerts` metadata.

## 7. Target Behavior

- While collecting usage telemetry, the service emits a stale/missing alert when a project has one or more active reserved jobs with **zero fresh snapshots**.
- The alert uses the existing `UsageDriftDetected` event type with a distinct,
  fixed reason constant, so alert pipelines can remain on the existing event
  stream while dispatching by reason.
- Detection is per active reserved job:
  - if a project has three active reserved GPU jobs and one has no fresh
    snapshot, the alert emits for that missing subset;
  - if some jobs have fresh snapshots, those job IDs are omitted from
    `missing_job_ids`;
  - the alert resolves only when all active reserved jobs for that
    project/reason have fresh snapshots or are no longer active/reserved.
- Alert is deduplicated via `usage_drift_alerts` using the same persisted state model (`status`, `fingerprint`, `last_seen_at`, `resolved_at`).
- When the missing condition clears, the existing alert is resolved (no active issue) in state.
- Scheduler quota still uses workload reservations (`required_gpu` / `reservation_payload.reserved.gpu`) and remains unchanged.

## 8. Affected Domains

- `gpuusage` maintenance path (`collectGPUUsageTelemetry` and detector path)
- `resourcehours` service spec for alert contract metadata
- local event contract fixture validation
- test coverage in `gpuusage` package

## 9. Affected Files

- `backend/internal/services/gpuusage/usage_drift.go`
- `backend/internal/services/gpuusage/collector.go`
- `backend/internal/services/gpuusage/collector_test.go`
- `backend/internal/services/resourcehours/spec.go`
- `backend/internal/contracts/event_envelope_test.go` (if contract test fixtures are expanded)
- `backend/internal/contracts/fixtures/events/v1/` (optional reason-specific fixture, if added)
- `docs/acceptance/usage-attribution.md` (if plan updates acceptance evidence)

## 10. API / Contract Changes

- No external API contract changes.
- Event contract:
  - `UsageDriftDetected` (existing event type, `usage-observability-service`)
  - No new event type. The fixed reason constant is the contract-dispatch field:
    - `active_reserved_jobs_missing_fresh_snapshots`
  - The same value must be present in both `reason` and `drift_reason` payload
    fields and in the `usage_drift_alerts` alert ID/fingerprint state.
  - Suggested payload:
    - `project_id`
    - `reason`
    - `drift_reason`
    - `missing_active_reserved_jobs`
    - `missing_job_ids` (list)
    - `active_reserved_job_count`
    - `fresh_job_snapshot_window_minutes`
    - `detection_window_timestamp` / `detected_at`
    - `fresh_snapshot_rows_seen` (for visibility)
    - `first_seen_at` / `last_seen_at` (for alert hygiene)
- Keep `UsageDriftDetected` idempotency model (`scope:project:reason:fingerprint`) consistent with existing behavior.

## 11. Database / Migration Changes

- No schema migration.
- Reuse `usage_drift_alerts` (existing `resourcehours` table entry) with reason-specific IDs:
  - alert ID pattern: `project_id:reason`
- Persist/update `status`, `fingerprint`, `last_seen_at`, `resolved_at` as needed.

## 12. Configuration Changes

- No new config.
- Continue using existing `GPUUsageSnapshotWindowMin` and collector cadence.

## 13. Observability Changes

- Add one internal event stream signal on stale/missing snapshot condition.
- Add structured logs for:
  - stale/missing job count per project
  - projects with no active reserved jobs with stale signals
- Keep existing usage drift metrics unaffected.

## 14. Security Considerations

- Event payload remains ID + counters only; no user secrets.
- No auth/authz path changes.
- No changes to quota enforcement trust boundaries.

## 15. Implementation Steps

1. Extend `backend/internal/services/gpuusage/usage_drift.go`:
   - Add detector branch for active reserved jobs lacking fresh snapshots.
   - Reuse existing `usageDriftJobActive` + fresh cutoff logic.
   - Include positive reservation extraction from `required_gpu` / `reservation_payload.reserved.gpu` (fallback to existing allocation extraction if needed).
   - Build missing sets per job, not only per project, so mixed projects with
     some fresh jobs and some missing jobs still emit accurate alerts.
   - Emit an alert only when at least one active reserved job is missing fresh
     snapshots.
   - Resolve stale-missing alert when all previously missing jobs regain fresh
     samples or are no longer active/reserved.

2. Wire the branch in `backend/internal/services/gpuusage/collector.go`:
   - Keep existing material drift path unchanged.
   - Add stale/missing path in the same maintenance pass so it shares stats and execution order.

3. Add/adjust tests in `backend/internal/services/gpuusage/collector_test.go`:
   - emits alert when active reserved job has no fresh snapshot rows in window,
   - no alert when active reserved jobs have fresh rows,
   - alert still emits when a project has mixed fresh and missing active
     reserved jobs, and `missing_job_ids` contains only the missing subset,
   - no alert when active job has zero/absent reservation,
   - duplicate/noise suppression via persisted `usage_drift_alerts`,
   - resolution when telemetry becomes fresh after a missing pass.

4. Keep event contract validation passing:
   - Prefer no new fixture file because the event type is unchanged and the
     existing `usage-drift-detected.json` fixture covers the envelope/type
     contract.
   - If adding a reason-specific fixture file, update
     `backend/internal/contracts/event_envelope_test.go` and keep type→producer
     mapping stable.

5. Optional: update local acceptance evidence in `docs/acceptance/usage-attribution.md` to tie:
   - `USAGE-032` stale/missing evidence
   - existing `USAGE-029` drift evidence
   - and monitor/alert story for `MON-018`.

## 16. Verification Plan

- Targeted:

```bash
cd backend && go test ./internal/services/gpuusage -run "UsageDriftDetector|collectGPUUsageTelemetry|TestUsageDrift"
cd backend && go test ./internal/contracts -run EventEnvelopeFixturesAreValidV1
cd backend && go test ./internal/services/resourcehours -run "Spec"
cd backend && go test ./...
cd backend && go build ./...
cd backend && make coverage
cd backend && make ci-sonar
```

- Focused manual check:
  - Seed active running job with no fresh snapshot in window.
  - Seed same job with fresh snapshot in window.
  - Confirm alert lifecycle (emit → duplicate suppression → resolve).

- Regression checks:
  - `USAGE-029` material drift tests remain unchanged in expectation and pass.

## 17. Rollback Plan

- Remove the stale/missing detector branch from `detectUsageDrift`.
- Revert stats/log updates and tests.
- Keep existing drift-only behavior and all quota/admission logic untouched.

## 18. Risks

- False positives on temporary snapshot backfill delays.
- Fingerprint design must avoid flapping on per-job churn unless desired.
- Re-using `UsageDriftDetected` may require alert consumers to branch by reason.
- Reason is now a contract-dispatch field; changing it later is a compatibility
  break for alert consumers.
- Additional dedupe work might hide transient single-pass ingestion gaps; verify alert frequency expectations before external paging integration.

## 19. Reviewer Checklist

- [ ] Requirement fit: stale/missing alert exists for active reserved jobs without fresh snapshots.
- [ ] No quota admission/release behavior changes.
- [ ] `scheduler-quota` remains based on reservations/`required_gpu` path.
- [ ] Event contract reason is explicit and non-breaking (`UsageDriftDetected` reuse documented).
- [ ] Mixed projects with both fresh and missing active reserved jobs emit an
  alert only for the missing subset.
- [ ] `usage_drift_alerts` state and dedupe model is reused consistently.
- [ ] `clusterread` `telemetry_stale` semantics remain preserved and not broken.
- [ ] Tests cover emit, dedupe, suppression, and resolve paths.
- [ ] Sonar Quality Gate verification is required.

## 20. Status

Status: Approved
