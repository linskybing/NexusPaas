# Outbox Inbox Runtime Foundation

## 1. Objective

Advance the Day 16-35 GA roadmap by turning the existing Outbox/Inbox and
projection runtime into an auditable foundation with explicit consumer lag,
dead-letter, and projection progress signals. Keep the change small: expose the
state already maintained by `EventStream` and `RunProjection`, add focused tests,
and update blocker tracking.

## 2. Background

Connector preflight found `docs/roadmap.md` still lists Outbox/Inbox foundation
work for Day 16-35, including idempotency, retry/dead-letter visibility, lag
metrics, and replay evidence. `problem.md` says event fixtures, owner-read
fixtures, and command fixtures now exist, but Outbox/Inbox runtime infrastructure
remains open. The codebase already has an in-memory `EventBus`, Redis-backed
`RedisEventBus`, `/outbox`, `/projections`, and `RunProjection`; the missing
piece for this slice is a reviewable runtime contract that surfaces consumer lag
and dead-letter/projection counters through existing operational endpoints.

## 3. Source References

- `AGENTS.md`: requires the Plan Agent -> Reviewer Agent -> Code Agent ->
  Reviewer Agent workflow and small PRs on feature branches.
- `docs/roadmap.md`: Day 16-35 requires Outbox/Inbox foundations with
  idempotency, retry/dead-letter visibility, lag metrics, and replay evidence.
- `problem.md`: GA Architecture Remaining Issues still list data ownership and
  contract/runtime infrastructure work after the fixture PRs.
- `docs/adr/0002-outbox-inbox-read-models.md`: accepted direction for local
  outbox, inbox idempotency, dead-letter handling, lag, replay, and drift
  visibility.
- `docs/architecture/observability-strategy.md`: queue/event lag must be visible
  enough to alert on workflows blocked by event lag.
- `backend/internal/platform/events.go`: in-memory `EventBus` already records
  outbox events, consumer inbox idempotency, checkpoints, and lag.
- `backend/internal/platform/events_redis.go`: Redis stream event bus already
  persists event IDs, inbox idempotency keys, checkpoints, and lag.
- `backend/internal/platform/projection.go`: `RunProjection` already deduplicates
  consumers and records projection/dead-letter status.
- `backend/internal/platform/endpoints.go`: `/outbox`, `/projections`, and
  `/metrics` are existing authenticated operational endpoints.
- `backend/internal/platform/metrics.go`: current metrics sink supports HTTP
  histograms and unlabeled counters, but not labeled Outbox/Inbox gauges.

## 4. Assumptions

- This slice should build on the existing `EventStream` interface instead of
  replacing it with a broker, relay worker, or database schema migration.
- The current operational endpoints are the right review surface for this
  foundation; `/api/v1` external behavior must remain unchanged.
- Consumer lag can use the existing `EventStream.Lag(consumer)` checkpoint model.
- Projection dead-letter and applied counts are process-local runtime evidence;
  persistent replay/relay history can be added in a later broader slice.
- Metrics labels only need safe operational identifiers such as projection
  consumer names. Event payloads and IDs must not be exported as metric labels.

## 5. Non-Goals

- Do not add Kafka, NATS, new Redis topology, background relay workers, or new
  infrastructure.
- Do not change public `/api/v1` routes, request/response schemas, auth, or
  CORS behavior.
- Do not add database migrations or alter service-owned tables.
- Do not convert every service to event-fed read models in this PR.
- Do not add producer-specific or consumer-specific contract fixtures; those are
  separate follow-up roadmap slices.
- Do not capture live staging evidence or modify Kubernetes deployment manifests.

## 6. Current Behavior

- `EventBus` and `RedisEventBus` publish events and deduplicate consumers by
  `(consumer, event_id)`.
- `EventStream.Checkpoint` and `EventStream.Lag` exist, but `RunProjection` does
  not currently advance checkpoints after successful projection passes.
- `/outbox` returns redacted events.
- `/projections` returns projection applied/dead-letter counts and last event
  metadata, but not event-stream lag.
- `/metrics` exposes HTTP histograms and generic unlabeled counters; it does not
  expose outbox depth, consumer lag, or projection dead-letter totals.

## 7. Target Behavior

- Successful `RunProjection` passes advance the consumer checkpoint after the
  outbox scan completes without consume errors, making lag observable through the
  existing `EventStream` contract.
- A consume error does not advance the checkpoint, so lag remains visible rather
  than being hidden by a partial pass.
- `ProjectionStatuses()` enriches each projection status with current
  `lag` from `EventStream.Lag(consumer)`.
- `/projections` keeps the existing status fields and adds additive `lag` data;
  existing clients remain compatible.
- `/metrics` sets an Outbox/Inbox runtime snapshot immediately before serving
  metrics. Scraping `/metrics` must not increment labeled projection counters or
  otherwise mutate totals; snapshot series mirror current runtime state.
- `/metrics` emits Prometheus text series for:
  - `nexuspaas_event_outbox_events` gauge
  - `nexuspaas_event_consumer_lag{consumer="..."}` gauge
  - `nexuspaas_projection_applied_total{consumer="..."}` counter
  - `nexuspaas_projection_dead_letters_total{consumer="..."}` counter
- Metric output remains deterministic and does not include event payloads,
  event IDs, trace IDs, user IDs, API keys, or tokens. Consumer label values must
  be Prometheus-escaped or skipped if they cannot be rendered safely.

## 8. Affected Domains

- Platform event runtime.
- Projection/read-model runtime foundation.
- Runtime observability and operational evidence.
- GA roadmap and blocker tracking.

## 9. Affected Files

Expected files:

- `backend/internal/platform/metrics.go`
- `backend/internal/platform/projection.go`
- `backend/internal/platform/endpoints.go`
- `backend/internal/platform/projection_test.go`
- `backend/internal/platform/metrics_rollback_admin_test.go` or
  `backend/internal/platform/observability_test.go`
- `problem.md`
- `docs/plan/2026-06-19-outbox-inbox-foundation.md`

Optional only if implementation needs a small helper split:

- `backend/internal/platform/outbox_inbox_metrics.go`

## 10. API / Contract Changes

No external `/api/v1` API changes.

Operational contract changes are additive:

- `/projections` adds a `lag` integer field per consumer.
- `/metrics` adds the Prometheus series listed in the target behavior section.

Existing `/outbox`, `/projections`, and `/metrics` paths remain authenticated
operational endpoints in production via the existing route registration.

## 11. Database / Migration Changes

None.

This PR does not change service-owned schemas or the existing service migration
files. Dead-letter persistence continues to use the existing
`platform:dead_letters` store resource.

## 12. Configuration Changes

None.

No environment variables, service URLs, secrets, deployment manifests, or
service catalog entries should change.

## 13. Observability Changes

- Extend the in-process metrics sink with a narrowly scoped way to set labeled
  gauges and labeled counters from runtime snapshots.
- Set an Outbox/Inbox runtime snapshot immediately before `/metrics` is served;
  repeated scrapes must not increment values.
- Emit deterministic Prometheus text output for outbox depth, consumer lag,
  projection applied totals, and projection dead-letter totals.
- Add lag to `/projections` JSON so reviewers can inspect state without parsing
  Prometheus output.

## 14. Security Considerations

- Metrics labels must be limited to safe operational consumer names.
- Do not export event IDs, trace IDs, idempotency keys, user IDs, tenant IDs,
  project IDs, payload fields, tokens, cookies, or service keys as metric labels.
- `/metrics` and `/projections` stay behind the existing authenticated admin
  operational route in production.
- No secret files or local metadata are read, printed, staged, or committed.

## 15. Implementation Steps

1. Add minimal labeled metric support to `Metrics` with deterministic label
   formatting, Prometheus label-value escaping, and explicit metric kind (`gauge`
   or `counter`) for runtime snapshot series.
2. Add a small App helper that sets current outbox length and projection statuses
   into metrics using safe labels. The helper must replace snapshot values, not
   increment them per scrape.
3. Call the snapshot helper from the existing `/metrics` operational handler
   immediately before `rawHTTPResponse(app.Metrics, r)`.
4. Update `ProjectionStatus` and `ProjectionStatuses()` to include current
   `lag` from `EventStream.Lag(consumer)`.
5. Update `RunProjection` so it advances the checkpoint only after a full scan
   completes without consume errors. Leave checkpoint unchanged on consume
   errors so lag remains visible.
6. Add focused platform tests for:
   - projection checkpoint advancement and lag returning to zero after a clean
     projection pass;
   - lag staying visible when consume fails;
   - `/projections` including additive `lag` data;
   - `/metrics` exposing outbox depth, consumer lag, applied totals, and
     dead-letter totals with safe deterministic labels.
7. Update `problem.md` to move only the runtime lag/dead-letter/projection
   visibility portion from open to implemented. Preserve unimplemented ADR 0002
   gaps explicitly: retry count, replay progress, drift metrics/comparison,
   durable relay/publish lag evidence, broader producer/consumer contract tests,
   read-model adoption, staging evidence, service identity, remote Sonar, and
   supply chain.

## 16. Verification Plan

Focused checks:

- `gofmt -w backend/internal/platform/metrics.go backend/internal/platform/projection.go backend/internal/platform/endpoints.go backend/internal/platform/projection_test.go backend/internal/platform/metrics_rollback_admin_test.go backend/internal/platform/observability_test.go`
- `go -C backend test ./internal/platform -run 'Projection|Outbox|Metrics|Observability' -count=1`

Required gates for this roadmap slice:

- `git diff --check`
- `go -C backend test ./... -count=1`
- `go -C backend vet ./...`
- `go -C backend build ./...`
- `bash backend/scripts/ci-security-gate.sh quick`

Sonar/security follow-up gates:

- Run `bash backend/scripts/ci-security-gate.sh security` if the quick gate is
  clean and the local scanner dependencies are available.
- Run `bash backend/scripts/ci-security-gate.sh sonar` if the local Sonar service
  and credentials are available. If unavailable, document the blocker and rely
  on the required quick gate plus PR checks.

No E2E, live Kubernetes, or staging evidence is required because this slice only
changes in-process operational observability and platform unit tests.

## 17. Rollback Plan

Revert this PR. The runtime will return to existing `/outbox`, `/projections`,
and HTTP-only `/metrics` behavior. Application APIs, database state, deployment
manifests, and service configuration are unaffected.

## 18. Risks and Tradeoffs

- The metric snapshot is process-local and reflects the current service process,
  not a durable historical event relay. This is acceptable for the foundation
  slice but does not close ADR 0002 gaps for retry count, replay progress, drift
  metrics/comparison, durable relay/publish lag evidence, or broader
  producer/consumer contract tests.
- Advancing checkpoints only after clean scans is conservative; a consume error
  can keep lag higher until a later successful pass.
- Labeled metric support adds a small generic capability to the custom metrics
  sink, but it is constrained to Prometheus exposition and avoids replacing the
  sink with a new dependency.
- Projection `lag` is additive JSON, so existing clients should continue to
  decode the response.

## 19. Reviewer Checklist

- Scope stays limited to existing Outbox/Inbox/projection runtime visibility.
- No `/api/v1`, DB migration, deployment manifest, auth, or config changes.
- `RunProjection` checkpoint behavior does not hide consume failures.
- `/projections` lag is additive and deterministic.
- `/metrics` includes safe labels only and does not expose payloads or IDs.
- Tests prove idempotency/lag/dead-letter visibility rather than checking only
  implementation details.
- `problem.md` updates only the completed runtime visibility piece and keeps the
  remaining roadmap blockers explicit.

## 20. Status

Status: Approved

Reviewer Agent approved after requiring snapshot set semantics, Prometheus label
escaping, repo-root verification commands, and explicit preservation of the
remaining ADR 0002 gaps.
