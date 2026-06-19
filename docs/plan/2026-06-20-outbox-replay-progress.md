# Outbox Replay Progress

## 1. Objective

Status: Approved

Add a small Day 16-35 Outbox/Inbox maturity slice that makes projection replay
and retry progress auditable. The slice should expose additive replay and retry
metadata through the existing `/projections` operational endpoint, Prometheus
metrics, and dead-letter records so operators can see when a replay was
requested and whether a previously dead-lettered event failed again.

## 2. Background

Connector preflight on `main` verified the DevSpace runtime, re-read
`AGENTS.md`, `docs/roadmap.md`, and `problem.md`, confirmed `main` was clean and
synced with `origin/main`, then created `feature/outbox-replay-progress` from
commit `64d425d contracts: add event consumer coverage`.

`docs/roadmap.md` Day 16-35 requires Outbox/Inbox infrastructure where missing,
including idempotency, retry/dead-letter visibility, and lag metrics.

`problem.md` says the current runtime visibility foundation already exposes
projection lag, outbox depth, applied totals, and dead-letter totals, but retry
count and replay progress remain open alongside durable relay/publish-lag
evidence, drift comparison, and broader read-model adoption.

Connector code inspection found the current platform has:

- `EventBus` / `RedisEventBus` idempotency and checkpoint/lag primitives.
- `App.RunProjection` for shared projection application and dead-letter writes.
- `App.ReplayProjection` to reset a consumer for replay.
- `/projections` returning `ProjectionStatus`.
- `/metrics` exporting outbox depth, consumer lag, applied totals, and
  dead-letter totals.

The missing evidence is not a new broker or new deployment topology. The
smallest useful next step is operator-visible replay and retry metadata for the
existing projection runtime.

## 3. Source References

- `AGENTS.md`: implementation must follow Plan Agent, Reviewer Agent, Code
  Agent, Reviewer Agent final approval.
- `docs/agents/planning.md`: required 20-section plan format and small
  verifiable scope.
- `docs/agents/workflow.md`: implementation cannot begin until plan approval;
  completion requires final Reviewer approval.
- `docs/roadmap.md`: Day 16-35 Outbox/Inbox infrastructure and visibility
  requirements.
- `problem.md`: retry count and replay progress remain open blockers for
  Outbox/Inbox maturity.
- `backend/internal/platform/projection.go`: current projection status,
  dead-letter write, and replay primitive.
- `backend/internal/platform/outbox_inbox_metrics.go`: current metrics snapshot.
- `backend/internal/platform/projection_test.go`: focused projection behavior
  tests.
- `backend/internal/platform/observability_test.go`: `/projections` and
  `/metrics` operational evidence tests.
- `backend/internal/platform/events.go`: in-memory outbox/inbox checkpoint and
  replay reset behavior.
- `backend/internal/platform/events_redis.go`: Redis stream implementation of
  the same EventStream port.
- `microservice-architecture` references:
  `communication-contracts.md`, `data-consistency.md`, and
  `testing-delivery.md`, which call for idempotent consumers, poison-message
  handling, retry/replay visibility, and tests for duplicate/failure behavior.

## 4. Assumptions

- Replay in this codebase means `App.ReplayProjection(consumer)` resets the
  consumer inbox/checkpoint state so existing outbox events can be re-applied on
  a later `RunProjection` pass.
- Retry count for this slice means repeated failed apply attempts for the same
  `(consumer, event_id)` dead-letter after replay, not automatic background
  broker redelivery.
- `/projections` is an operational endpoint, not an external `/api/v1`
  customer contract. Adding JSON fields is acceptable and backward-compatible.
- Existing `Lag` already shows replay progress after `ReplayProjection` because
  resetting the checkpoint makes all retained outbox events pending again.
  Replay metadata should make that state explicit.
- Durable relay/publish-lag evidence, drift comparison, and new read-model
  adoption are separate follow-up slices.

## 5. Non-Goals

- Do not add a new message broker, queue worker, Redis consumer-group protocol,
  database migration, deployment manifest, or service config.
- Do not change external `/api/v1` request paths, response schemas, auth
  behavior, or status codes.
- Do not introduce automatic retry scheduling in this slice.
- Do not change producer event fixtures or event schema versions.
- Do not refactor service projections or migrate new read models.
- Do not solve durable relay/publish-lag evidence or drift comparison in this
  branch.

## 6. Current Behavior

- `RunProjection` uses `EventStream.Consume` to make per-consumer application
  idempotent.
- A failed projection apply writes a record under `platform:dead_letters` and
  increments `ProjectionStatus.DeadLettered`.
- `ProjectionStatus` exposes consumer, applied count, dead-letter count, lag,
  last event metadata, and last applied timestamp.
- `ReplayProjection` resets the consumer's inbox/checkpoint state but does not
  update projection status, so operators cannot distinguish ordinary lag from a
  requested replay.
- If a dead-lettered event is replayed and fails again, the dead-letter record
  is overwritten without an attempt/retry count.
- Metrics expose outbox depth, consumer lag, applied totals, and dead-letter
  totals only.

## 7. Target Behavior

- `ProjectionStatus` includes additive replay metadata:
  - `replay_count`
  - `replay_pending`
  - `last_replay_at`
- `ProjectionStatus` includes additive retry metadata:
  - `retry_count`
- `ReplayProjection(consumer)` records the replay request in the projection
  registry and keeps `replay_pending` true until a projection pass completes
  without a consume failure.
- Dead-letter records include additive attempt metadata:
  - `attempt_count`
  - `retry_count`
  - `last_failed_at`
- Replaying a previously dead-lettered event that fails again increments the
  status retry count and updates the same dead-letter record with the new
  attempt/retry counts.
- `/metrics` exports per-consumer replay and retry totals alongside the
  existing applied/dead-letter/lag metrics.
- Existing lag, dead-letter, idempotency, and replay behavior continue to pass.

## 8. Affected Domains

- Platform Outbox/Inbox projection runtime.
- Operational `/projections` status.
- Prometheus runtime metrics.
- Platform tests that prove replay/retry evidence.

No new service boundary is introduced. This slice strengthens the shared
projection runtime that supports the planned move away from shared stores and
toward event-fed read models.

## 9. Affected Files

Expected changes:

- `docs/plan/2026-06-20-outbox-replay-progress.md`
- `backend/internal/platform/projection.go`
- `backend/internal/platform/outbox_inbox_metrics.go`
- `backend/internal/platform/projection_test.go`
- `backend/internal/platform/observability_test.go`
- `problem.md`

No `.claude/`, `.codegraph/`, `.mcp.json`, token, owner password, local
metadata, deployment manifest, or migration file should be touched.

## 10. API / Contract Changes

External `/api/v1`: none.

Operational endpoint `/projections`: additive JSON fields only:
`retry_count`, `replay_count`, `replay_pending`, and `last_replay_at`.

Metrics: additive Prometheus metric names for projection replay and retry
totals. Existing metric names and labels remain unchanged.

Internal dead-letter records: additive keys only. Existing keys such as
`consumer`, `event_id`, `event_name`, `error`, and `failed_at` remain.

## 11. Database / Migration Changes

None. The in-memory/default record store and existing Postgres store can hold
the additive map fields without a schema migration.

## 12. Configuration Changes

None. No environment variable, service URL, feature flag, API key, CI setting,
or deployment configuration is planned.

## 13. Observability Changes

- Add projection replay total metric per consumer.
- Add projection retry total metric per consumer.
- Keep existing outbox depth, consumer lag, projection applied total, and
  projection dead-letter total metrics unchanged.
- Extend `/projections` to show replay count, replay pending state, last replay
  timestamp, and retry count.

## 14. Security Considerations

- Do not write event payloads into dead-letter records in this slice. Continue
  storing only metadata and the error string.
- Do not read, print, or commit secret files.
- Keep `/outbox` redaction behavior unchanged.
- Do not add credentials, tokens, auth headers, cookies, or owner passwords to
  metrics or status payloads.

## 15. Implementation Steps

1. Extend `ProjectionStatus` with replay and retry fields.
2. Add projection registry helpers to record replay requests, completed replay
   passes, dead-letter attempts, and retry totals.
3. Update `ReplayProjection` to record replay metadata before resetting the
   consumer state.
4. Update `RunProjection` so a completed consume pass clears replay pending, and
   a consume failure keeps replay pending visible.
5. Update `deadLetterEvent` to preserve existing metadata while adding
   `attempt_count`, `retry_count`, and `last_failed_at`. Use the existing
   dead-letter record, when present, to increment attempts.
6. Add retry and replay metrics in `outbox_inbox_metrics.go`.
7. Add focused projection tests proving:
   - replay sets status replay metadata and makes lag visible before rerun;
   - successful rerun clears replay pending;
   - a second failed replay updates dead-letter attempt/retry counts and status
     retry count.
8. Extend operational endpoint/metrics tests to assert the new additive fields
   and metrics.
9. Update `problem.md` to record this slice's evidence and keep durable
   relay/publish-lag, drift comparison, and read-model adoption open.

## 16. Verification Plan

Focused verification:

- `go -C backend test ./internal/platform -run 'Projection|Outbox|Metrics|Observability' -count=1`

Required gates:

- `git diff --check`
- `go -C backend test ./... -count=1`
- `go -C backend vet ./...`
- `go -C backend build ./...`
- `bash backend/scripts/ci-security-gate.sh quick`

Additional final-review gates:

- `bash backend/scripts/ci-security-gate.sh sonar`
- `bash backend/scripts/ci-security-gate.sh security` if local tooling/runtime
  is available; otherwise record the exact blocker in `problem.md` and PR notes.

E2E, live Kubernetes, and staging evidence are not required for this slice
because it is limited to in-process projection runtime metadata, operational
status, and metrics.

## 17. Rollback Plan

Revert the new projection status fields, metrics, dead-letter attempt metadata,
tests, and `problem.md` update. Since no external `/api/v1`, database schema,
deployment, or configuration changes are planned, rollback is a normal git
revert.

## 18. Risks and Tradeoffs

- Retry count here is explicitly failed replay attempts for the same
  dead-lettered event, not automatic broker retries.
- Replay progress is exposed through replay metadata plus existing lag; this
  does not add a durable relay worker or consumer-group protocol.
- Additional `/projections` fields are backward-compatible, but internal
  dashboards may choose to start depending on them after this PR.
- Metrics are process-local for the in-memory runtime and Redis-backed where the
  injected EventStream backs lag/outbox state. Cross-replica aggregation remains
  a deployment concern.

## 19. Reviewer Checklist

- Plan follows `docs/agents/planning.md` required sections.
- Scope is a single Day 16-35 Outbox/Inbox maturity slice.
- No external `/api/v1`, DB migration, deployment, or config change is
  introduced.
- Replay and retry metadata is additive and does not weaken existing
  idempotency/dead-letter behavior.
- Tests cover replay progress, retry count, operational endpoint fields, and
  metrics.
- Verification includes required gates, Sonar, and security status.
- `problem.md` keeps remaining GA blockers explicit.

## 20. Status

Status: Approved

Reviewer Agent approved this plan for Code Agent implementation.
