# DATA Replay Idempotency

Status: Implemented; Reviewer approved

Reviewer: Final implementation review approved

## Objective

Close the `Strengthen: DATA replay idempotency` gap by proving projection
replay does not double-apply events that already succeeded.

## Current Evidence

- `RunProjection` already deduplicates duplicate delivery through
  `EventStream.Consume`.
- Before this slice, `ReplayProjection` reset the whole consumer inbox, so an
  operator retry for one dead-letter could re-apply every previously successful
  event for that consumer.
- This slice changes replay retry to release only unresolved dead-letter event
  IDs and resolve the dead-letter record after successful retry.

## Scope

- Add a targeted inbox reset operation for specific event IDs.
- Keep the existing full `ResetConsumer` primitive for future full rebuild
  workflows, but stop using it for ordinary projection replay.
- Change `ReplayProjection` to derive event IDs only from that consumer's
  unresolved `platform:dead_letters` records and release only those event IDs
  from inbox state.
- `ReplayProjection` must not fall back to full `ResetConsumer`.
- Targeted inbox release must not clear checkpoints.
- When a previously dead-lettered event later applies successfully, resolve the
  `(consumer,event_id)` dead-letter record by deleting it or marking it
  resolved, so later replay requests cannot reselect it.
- Preserve dead-letter retry behavior.
- Prove with tests that:
  - successful events are not applied again during replay,
  - dead-lettered events can still be retried,
  - after a dead-lettered event succeeds, a second replay does not re-apply the
    formerly dead-lettered event,
  - resolved dead-letter records are not selected for replay,
  - targeted reset works for in-memory, Redis, and Postgres event streams.
- Update `gap.md` and `docs/acceptance/gap-analysis.md` after tests pass.

## Non-Goals

- No new event bus dependency or broker.
- No read-model rebuild subsystem.
- No live cluster rollout in this slice unless the local checks expose a runtime
  change that needs live evidence.
- No attempt to close all remaining DATA blockers.

## Affected Files

- `backend/internal/platform/ports.go`
- `backend/internal/platform/events.go`
- `backend/internal/platform/events_redis.go`
- `backend/internal/platform/events_postgres.go`
- `backend/internal/platform/projection.go`
- `backend/internal/platform/*_test.go`
- `docs/plan/2026-06-21-data-replay-idempotency.md`
- `gap.md`
- `docs/acceptance/gap-analysis.md`

## Verification

```sh
go -C backend test ./internal/platform -run 'Projection|EventBus|PostgresEventBus|RedisEventBus' -count=1
go -C backend test ./internal/platform
go -C backend test ./...
bash backend/scripts/ci-security-gate.sh quick
git diff --check
```

## Implementation Evidence

- Added `EventStream.ResetConsumerEvents(consumer, eventIDs)` for targeted
  inbox release without clearing checkpoints.
- Implemented targeted release for in-memory, Redis, and Postgres event
  streams.
- Changed `ReplayProjection` to derive replay IDs from unresolved
  `platform:dead_letters` records for that consumer and not call full
  `ResetConsumer`.
- Changed successful projection apply to resolve/delete any matching
  `(consumer,event_id)` dead-letter record.
- Added regression coverage for:
  - replay retry does not re-apply previously successful events,
  - a successfully retried dead-letter is not selected by a later replay,
  - dead-letter retry behavior remains intact,
  - targeted inbox release preserves checkpoints in in-memory, Redis, and
    Postgres streams.
- Verification passed:
  - `go -C backend test ./internal/platform -run 'Projection|EventBus|PostgresEventBus|RedisEventBus' -count=1`
  - `go -C backend test ./internal/platform`
  - `go -C backend test ./...`
  - `bash backend/scripts/ci-security-gate.sh quick`
  - `git diff --check`

## Reviewer Result

- Reviewer found no blocking findings and approved final implementation review.
- Reviewer reran:
  - `go -C backend test ./internal/platform -run 'Projection|EventBus|PostgresEventBus|RedisEventBus' -count=1`
  - `go -C backend test ./internal/platform -count=1`
- Non-blocking residual risk: stale dead-letter rows from older builds are
  still treated as unresolved; migration/cleanup was not in this approved slice.

## Acceptance

- Replaying a projection retry does not double-apply previously successful
  events.
- Dead-letter retries still work.
- Reviewer approves requirement fit, plan alignment, SOLID, 12-Factor, tests,
  and diff scope.
