# GPU Reservation Release + Drift Detection

## 1. Objective

Close the remaining local portion of GPU DRA/MPS reservation lifecycle by wiring existing reservation FSMs into workload submit/terminal transitions and adding a scheduler-side drift detector:

- reserve + commit quota immediately after scheduler admission approval,
- persist and release reservation identity in workload jobs,
- release reserved quota on terminal statuses,
- detect and alert when committed/reserved quota no longer matches workload termination state.

This is the next local (non-hardware) slice for `docs/acceptance/gpu-dra-mps.md` and addresses **GPU-014** and **GPU-015**.

## 2. Background

- `scheduler-quota` already exposes generic reservation API paths and FSM (`reserved -> committed -> released`) in `platform/reservation.go`.
- workload submit currently only calls scheduler admission and then creates the job record directly.
- scheduler admission already reads active workload records (`submitted`, `waiting_infra`, `queued`, `running`) for quota accounting with fractional GPU.
- terminal workload transitions are already owned by workload-service:
  - `MarkDispatchFailed` -> `failed`
  - `MarkPreempted` -> `preempted`
  - `MarkEvicted` -> `evicted`
  - `ApplyLifecycleObservation` via status reconciler -> `completed`/`failed`
- there is no DRA node-level reclaim hook in this slice; drift is control-plane evidence-only.

## 3. Source References

- `docs/acceptance/gpu-dra-mps.md` (GPU-014 and GPU-015)
- `[gap.md](../../gap.md)` and `[problem.md](../../problem.md)` (roadmap and cross-service risks)
- `backend/internal/services/workload/job_submit.go`
- `backend/internal/services/workload/preemption_contracts.go`
- `backend/internal/services/workload/eviction_contracts.go`
- `backend/internal/services/workload/status_reconciler.go`
- `backend/internal/platform/reservation.go`
- `backend/internal/services/schedulerquota/spec.go`
- `backend/internal/services/schedulerquota/handler.go`
- `backend/internal/services/schedulerquota/read_contracts.go`

## 4. Assumptions

- `workload-service` owns workload/job records and terminal lifecycle transitions.
- `scheduler-quota-service` owns reservation records and state-machine transitions.
- Existing reservation FSM endpoints are sufficient; no new FSM or DDL.
- `workload-service` can continue using internal scheduler client abstractions and existing idempotent status transitions.
- `ownerRead` availability for workloads is best effort in scheduler maintenance; drift scan degrades to no-op if workload read is unavailable.

## 5. Non-Goals

- No real GPU validation/test hardware.
- No new reservation tables, fields in SQL schema, or migration scripts.
- No replacement for scheduler placement/admission decisions.
- No change to GPU accounting policy semantics beyond lifecycle enforcement.
- No service ownership inversion: workload writes remain workload-owned.

## 6. Current Behavior

- Successful submission flow:
  - build payload → admission review → persist job.
- No reservation is created/committed during submit.
- No reservation metadata is persisted on job record.
- Reservation lifecycle is not tied to job terminal transitions.
- No periodic drift audit for committed/reserved reservations exists.

## 7. Target Behavior

- On successful admission review:
  1. `scheduler-quota` `reserve` is called once with normalized request metadata.
  2. `scheduler-quota` `commit` is called before job persistence.
  3. job record stores reservation metadata (`reservation_id`, `reservation_state`, reservation payload fields).
- If commit succeeds but job persistence fails:
  - release is attempted immediately before returning the API error.
- Every workload-owned terminal transition for job states `completed`, `failed`, `preempted`, `evicted` triggers best-effort reservation release, including dispatcher failure (`failDispatchedJob` -> `MarkDispatchFailed`), lifecycle-reconciled completion/failure, preemption, eviction, and stale-job failure paths.
- Release call is idempotent by FSM semantics (repeating release is safe; already-released returns `200` with same record).
- Reservation drift scan runs as scheduler maintenance task:
  - reads committed/reserved reservations,
  - resolves associated workload status,
  - emits deterministic drift records/events when reservation state is active but job is terminal/missing.

Conflict/ordering rules:

- scheduler admission remains unchanged (no new scheduling algorithm).
- dispatcher/prepare ordering for accelerator selectors is not modified by this slice.

## 8. Affected Domains

- `workload-service`: submit pipeline + terminal status handlers.
- `scheduler-quota-service`: maintenance task and cross-service reservation reconciliation logic.
- `platform`: internal contract/client patterns only.

## 9. Affected Files

- `backend/internal/services/workload/job_submit.go` (submit reservation path + rollback)
- `backend/internal/services/workload/dispatcher.go` (release on dispatch failure)
- `backend/internal/services/workload/scheduler_reservation_client.go` (new internal scheduler client helpers for reserve/commit/release)
- `backend/internal/services/workload/scheduler_admission_client.go` (shared scheduler service constants reuse, if applicable)
- `backend/internal/services/workload/preemption_contracts.go` (release on `preempted`)
- `backend/internal/services/workload/eviction_contracts.go` (release on `evicted`)
- `backend/internal/services/workload/status_reconciler.go` (release on `completed`/`failed` transitions)
- `backend/internal/services/workload/job_submit_test.go`
- `backend/internal/services/workload/preemption_contracts_test.go`
- `backend/internal/services/workload/eviction_contracts_test.go`
- `backend/internal/services/workload/status_reconciler_test.go`
- `backend/internal/services/schedulerquota/spec.go` (add `ReservationDriftDetected` event + maintenance ownership)
- `backend/internal/services/schedulerquota/handler.go` (maintenance registration)
- `backend/internal/services/schedulerquota/read_contracts.go` (workload job lookup helpers if needed for drift)
- `backend/internal/services/schedulerquota/reservation_drift.go` (new detector module)
- `backend/internal/services/schedulerquota/reservation_drift_test.go` (new drift tests)
- `backend/internal/contracts/fixtures/events/v1/reservation-drift-detected.json` (event fixture)
- `backend/internal/services/schedulerquota/priority_class_sync_test.go` or a dedicated maintenance registration test file for task name validation (single focused test)

## 10. API / Contract Changes

No new external public API routes.

### Internal reservation data flow (new)

- `POST /api/v1/internal/quota/reservations` (reserve)
- `POST /api/v1/internal/quota/reservations/{reservationId}/commit` (commit)
- `POST /api/v1/internal/quota/reservations/{reservationId}/release` (release)

### Data model payload fields

- Workload job persistence fields:
  - `reservation_id` (string, required when admission succeeded and reservation was created)
  - `reservation_state` (string: `reserved|committed`, required when `reservation_id` exists)
  - `reservation_payload` (map, optional):
    - `required_gpu` (number)
    - `required_cpu` (number)
    - `required_memory` (number, same key/unit as existing workload job records)
    - `project_id` (string)
    - `queue_name` (string)
    - `device_class_name` (string)
  - `reservation_commit_error` (string, optional, temporary internal troubleshooting signal; should be removed/cleared on success in follow-up if needed)
- Reservation record event payload (existing) continues to carry:
  - `reservation_id`
  - `state`
  - `project_id`
  - `job_id`
  - `reserved` or explicit resource keys.
- Drift alert event payload (`ReservationDriftDetected`):
  - `reservation_id`
  - `job_id`
  - `project_id`
  - `reservation_state`
  - `job_status`
  - `job_exists`
  - `drift_reason` (`terminal_job_but_reservation_not_released` or `missing_job_for_reservation`)
  - `detected_at`

## 11. Database / Migration Changes

- No SQL DDL.
- No new repository/table types.
- Reuse existing records:
  - `scheduler-quota-service:reservations`
  - `workload-service:jobs`

## 12. Configuration Changes

- None required.
- Uses existing `service` / internal route auth config.

## 13. Observability Changes

- Keep existing `QuotaReserved`, `QuotaCommitted`, `QuotaReleased` events for lifecycle proof.
- Add required `ReservationDriftDetected` event to scheduler-quota Spec and event fixtures for explicit alerting.
- Add scheduler maintenance log entries:
  - detector run start/summary,
  - drift count,
  - skipped scans when workload owner read is unavailable.
- Ensure event/action names carry non-sensitive resource IDs and statuses only.

## 14. Security Considerations

- No user-controlled writes to reservations; all reservation calls happen inside service context after authz and admission.
- Admission failure or unavailable scheduler endpoints must block workload submit and avoid creating unreserved GPU jobs.
- Terminal release failures must be logged; no silent success.
- Drift data includes only IDs/status; no sensitive request payloads.
- No broadening of read rights: scheduler remains read-only for workload records and relies on existing owner-read contracts.

## 15. Implementation Steps

1. Add workload-to-scheduler reservation client
   - create `backend/internal/services/workload/scheduler_reservation_client.go` with typed `Reserve`, `Commit`, and `Release` calls and helper responses.
   - preserve idempotent error handling.

2. Wire submit-time reserve/commit flow in `backend/internal/services/workload/job_submit.go`
   - after `admitSubmittedJob` passes with `review` and before `CreateSubmittedJobWithEvent`:
     - reserve using normalized admission payload (`project_id`, `job_id`, resource fields, `device_class_name`),
     - commit reservation,
     - inject `reservation_id` + `reservation_state` into job payload.
   - failure compensation:
     - if reserve fails → submit returns service-unavailable-style failure and job is not created.
     - if commit fails → reserve should be released via best-effort rollback and submit fails.
     - if commit succeeded and job create fails → immediate reservation release, then return job-creation failure.
   - preserve existing idempotency behavior and admission-denial paths.

3. Add terminal release points for every workload terminal writer
   - `backend/internal/services/workload/dispatcher.go`: release on successful `MarkDispatchFailed` through `failDispatchedJob`.
   - `backend/internal/services/workload/preemption_contracts.go`: release on successful `MarkPreempted` transition.
   - `backend/internal/services/workload/eviction_contracts.go`: release on successful `MarkEvicted` transition.
   - `backend/internal/services/workload/status_reconciler.go`:
     - after successful lifecycle update to `completed`/`failed`, attempt release.
   - `backend/internal/services/workload/stale_job_reaper.go` or the equivalent stale-job failure path if present in this codebase.
   - implement a small shared helper to avoid duplicate release logic.

4. Add scheduler drift detector
   - create `backend/internal/services/schedulerquota/reservation_drift.go`:
     - new maintenance task (service-owned, lease-gated) that scans reservation records in `reserved`/`committed`.
     - map each active reservation to job in `workload-service:jobs` by `job_id`.
     - flag drift when:
       - reservation points to missing job, or
       - job exists but status is `completed|failed|preempted|evicted` and reservation state is not `released`.
    - emit `ReservationDriftDetected` event with a stable payload.
   - add `ReservationDriftDetected` to scheduler-quota Spec events and contract fixture.
   - register task in `handler.go` and include task name in maintenance registration coverage.

5. Tests and fixtures
   - workload:
     - `job_submit_test.go`: reserve+commit path persists `reservation_id`/state; submit fails with no writes when scheduler quota routes unavailable.
     - `job_submit_test.go`: duplicate job ID or created-job failure path invokes immediate release.
     - `dispatcher_test.go`, `preemption_contracts_test.go`, `eviction_contracts_test.go`, `status_reconciler_test.go`:
       - terminal transition calls release (best-effort), including repeated terminal transitions.
   - scheduler-quota:
     - `reservation_drift_test.go`: active reservation + terminal job triggers drift record/event.
     - idempotent skip when no drift (active non-terminal job).
     - maintenance task registration and no-op behavior when workload reader/deployment is degraded.
     - contract fixture test includes `ReservationDriftDetected`.

## 16. Verification Plan

- `cd backend && go test ./internal/services/workload -run "(Submit|Preemption|Evict|StatusReconciler|Drift)"`  
- `cd backend && go test ./internal/services/schedulerquota -run "Reservation|Drift|Maintenance"`  
- `cd backend && go test ./internal/contracts/...`  
- `cd backend && go test ./...`  
- `cd backend && go build ./...`  
- Manual check: `grep -n "GPU-014\\|GPU-015" docs/acceptance/gpu-dra-mps.md` and ensure expected plan status language aligns with local drift behavior.

## 17. Rollback Plan

- Remove submit-time reservation call path and restore current admission→job create ordering.
- Remove terminal release helper invocations from preemption/eviction/reconciler handlers.
- Remove `reservation_drift` module and maintenance registration.
- Delete tests tied to release/detector behavior.

## 18. Risks and Tradeoffs

- Reservation API failure path can introduce new submit failure mode if scheduler service is partitioned; this is intentional to avoid unreserved GPU grants.
- Drift scan can produce noise if scheduler maintenance has stale/partial reads; this is bounded by conservative local checks and deterministic evidence records.
- Terminal release failure creates delayed cleanup debt; drift detector covers this but cleanup is eventual.
- Workload terminal transitions and scheduler drift detector are in different services; eventual convergence relies on task cadence.
- This slice does not prove node-level MPS/GPU scheduling correctness, only control-plane consistency.

## 19. Reviewer Checklist

- [x] Scope aligned only to GPU-014 and GPU-015 in `docs/acceptance/gpu-dra-mps.md`.
- [x] Submit flow reserves and commits only after admission approval and before persistence.
- [x] `reservation_id` persisted on job record with explicit rollback behavior on job-create failure.
- [x] Release is attempted for `completed`, `failed`, `preempted`, `evicted`, dispatch failure, idle/stale reaping, and lifecycle reconciler terminal observations.
- [x] Idempotent release semantics are preserved (`state` transitions already allow same-state 200).
- [x] Drift detector is deterministic, local, and produces evidence with clear reason values.
- [x] No new SQL DDL or DRA algorithm changes.
- [x] Tests cover failure compensation and terminal-path release behavior.
- [x] Reviewer final verification complete.

## 20. Status

Plan Status: Approved
Implementation Status: Implemented and Reviewer-approved.
