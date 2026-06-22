# Transactional Outbox Inbox

## 1. Objective

Add the first GA-grade durable event infrastructure slice: Postgres-backed
outbox/inbox tables, transactional record write + outbox insertion for platform
write paths, idempotent consumer records, relay state, retry/dead-letter
visibility, and live evidence.

## 2. Background

`problem.md` lists `Transactional outbox/inbox` as a P0 blocker. The code already
has an `EventStream` port, in-memory events for local runs, Redis Streams for a
shared event bus, projection replay, dead-letter records, and service migration
stubs with per-service outbox/inbox table names. The missing GA piece is the
transactional boundary between a durable owner write and its emitted event.

The existing runtime also already has Postgres, Redis, pgx, Redis Streams, and a
lease-gated maintenance loop. This slice should reuse those CNCF/cloud-native
building blocks instead of adding a new broker or custom distributed protocol.

## 3. Source References

- `problem.md`
- `backend/docs/event-contracts.md`
- `backend/internal/contracts/contracts.go`
- `backend/internal/platform/ports.go`
- `backend/internal/platform/events.go`
- `backend/internal/platform/events_redis.go`
- `backend/internal/platform/projection.go`
- `backend/internal/platform/store_postgres.go`
- `backend/internal/platform/schema.sql`
- `backend/internal/platform/runtime.go`
- `backend/internal/platform/maintenance.go`
- `backend/internal/platform/crud.go`
- `backend/internal/platform/reservation.go`

## 4. Assumptions

- `contracts.Event.EventID` remains a string contract, so new platform event
  tables use `TEXT` event ids rather than requiring UUID-only event ids.
- Postgres is the system of record for the durable outbox/inbox whenever
  `DATABASE_URL` is configured.
- Redis Streams remains an optional fan-out transport when `EVENT_BUS_URL` is
  configured, but it should not replace the Postgres outbox durability boundary.
- In-memory `Store` and `EventBus` remain valid for unit tests and dependency-free
  local development.
- This is the first infrastructure slice. It does not convert every
  service-specific repository write in one large diff.

## 5. Non-Goals

- No Kafka/NATS/RabbitMQ introduction.
- No distributed transaction / two-phase commit.
- No removal of Redis Streams.
- No rewrite of all domain-specific handlers in this slice.
- No replacement of the existing projection replay/dead-letter API surface.
- No migration-runner replacement; the existing idempotent migration path is used.

## 6. Current Behavior

- `App` writes records through `RecordStore`, then separately calls
  `EventStream.Publish`.
- `RedisEventBus.Publish` appends directly to Redis Streams; this is durable
  enough for stream replay, but it is not in the same Postgres transaction as the
  owner write.
- `RunProjection` records idempotency through `EventStream.Consume` and stores
  dead-letter metadata in `RecordStore`.
- Production runtime uses backing services through `NewBackingResources`, but
  `EVENT_BUS_URL` currently selects Redis as the primary event stream.

## 7. Target Behavior

- Platform schema includes durable `platform_event_outbox`,
  `platform_event_inbox`, and `platform_event_checkpoints` tables.
- A new `PostgresEventBus` implements `EventStream` using those tables.
- When `DATABASE_URL` is configured, runtime uses the Postgres event bus as the
  primary `App.Events` implementation.
- When both `DATABASE_URL` and `EVENT_BUS_URL` are configured, a lease-gated
  maintenance relay publishes pending Postgres outbox rows to Redis Streams and
  records relay attempts, last errors, published timestamps, and dead-letter
  status.
- Platform generic write paths create/update/delete records and insert their
  domain event outbox row in one Postgres transaction when the store supports the
  transactional extension. The in-memory fallback keeps the existing behavior.
- `RunProjection` continues to use the `EventStream` port and gains durable
  Postgres-backed inbox/checkpoint semantics without service handler changes.

## 8. Affected Domains

- Platform eventing.
- Platform persistence.
- Runtime backing-resource composition.
- Generic CRUD, command, config commit, and reservation write paths.
- Projection idempotency and lag visibility.

## 9. Affected Files

- `docs/plan/2026-06-20-transactional-outbox-inbox.md`
- `backend/internal/platform/schema.sql`
- `backend/internal/platform/ports.go`
- `backend/internal/platform/events.go`
- `backend/internal/platform/events_postgres.go`
- `backend/internal/platform/event_relay.go`
- `backend/internal/platform/store_postgres.go`
- `backend/internal/platform/runtime.go`
- `backend/internal/platform/app.go`
- `backend/internal/platform/crud.go`
- `backend/internal/platform/reservation.go`
- `backend/internal/platform/config.go`
- `backend/internal/platform/config_test.go`
- `backend/internal/platform/events_postgres_test.go`
- `backend/internal/platform/store_postgres_unit_test.go`
- `backend/internal/platform/projection_test.go`
- `backend/internal/platform/runtime_test.go`
- `backend/internal/platform/schema_test.go`

## 10. API / Contract Changes

No public HTTP API change.

Internal platform contracts add:

- A narrow transactional record-store extension for write-with-event operations.
- A narrow event relay extension for durable event buses that can relay pending
  outbox rows to an optional external stream.

Existing `RecordStore` and `EventStream` methods remain compatible.

## 11. Database / Migration Changes

Extend `backend/internal/platform/schema.sql` with:

- `platform_event_outbox`
  - `event_id TEXT PRIMARY KEY`
  - `event_name TEXT NOT NULL`
  - `source TEXT NOT NULL`
  - `trace_id TEXT NOT NULL`
  - `schema_version INTEGER NOT NULL`
  - `idempotency_key TEXT NOT NULL DEFAULT ''`
  - `payload JSONB NOT NULL`
  - `occurred_at TIMESTAMPTZ NOT NULL`
  - `relay_status TEXT NOT NULL DEFAULT 'pending'`
  - `relay_attempts INTEGER NOT NULL DEFAULT 0`
  - `last_error TEXT`
  - `published_at TIMESTAMPTZ`
  - `created_at TIMESTAMPTZ NOT NULL DEFAULT now()`
  - `updated_at TIMESTAMPTZ NOT NULL DEFAULT now()`
- `platform_event_inbox`
  - `consumer TEXT NOT NULL`
  - `event_id TEXT NOT NULL`
  - `processed_at TIMESTAMPTZ NOT NULL DEFAULT now()`
  - primary key `(consumer, event_id)`
- `platform_event_checkpoints`
  - `consumer TEXT PRIMARY KEY`
  - `event_count BIGINT NOT NULL`
  - `last_event_id TEXT`
  - `checkpointed_at TIMESTAMPTZ NOT NULL DEFAULT now()`

Add indexes for outbox relay status/created order and outbox occurred order.

## 12. Configuration Changes

Add `EVENT_RELAY_BATCH_SIZE` with a conservative default. It controls how many
pending outbox rows one maintenance run relays to Redis. It is ignored when
there is no Postgres event bus or no Redis relay target.

## 13. Observability Changes

- Log relay failures with event id, event name, attempt count, and error.
- Keep `/outbox` backed by the active `EventStream.Outbox()` so admins can see
  durable events.
- Projection lag remains visible through existing projection status surfaces.
- Relay status and attempt columns are directly inspectable in Postgres for live
  evidence and operations.

## 14. Security Considerations

- Reuse existing event redaction for `/outbox`.
- Do not store secrets in new relay metadata.
- Do not expose new unauthenticated endpoints.
- The relay runs through the existing lease-gated maintenance mechanism, so only
  one replica should process a relay shard per interval.

## 15. Implementation Steps

1. Add platform event outbox/inbox/checkpoint schema and schema tests.
2. Add `PostgresEventBus` implementing `EventStream` with event validation,
   ordered outbox reads, idempotent `Consume`, durable checkpoints, lag, and
   consumer reset.
3. Add relay methods that select pending rows, publish them to an optional Redis
   sink, mark rows `published`, and record retry/dead-letter metadata on failure.
4. Wire runtime so `DATABASE_URL` creates the Postgres store and Postgres event
   bus from the same pool. If only `EVENT_BUS_URL` exists, retain the Redis bus.
   If both exist, use Postgres as primary and Redis as relay sink.
5. Register the relay as a normal maintenance task when the active event bus
   supports relay and has a sink.
6. Add a narrow transactional record-store extension implemented by
   `PostgresStore` for create/update/delete-with-event in one transaction.
7. Update platform generic CRUD, command, config commit, and reservation write
   paths to use the transactional extension when available, with the existing
   store-then-publish behavior as fallback.
8. Add focused tests for Postgres event bus behavior, transactional write
   rollback, relay success/failure state, projection idempotency, runtime wiring,
   and config parsing.
9. Run focused tests, full backend tests, quick gate, Sonar Quality Gate, and
   live RKE2 evidence.

## 16. Verification Plan

```sh
go -C backend test ./internal/platform -run 'PostgresEvent|Transactional|Projection|Runtime|Config|Schema' -count=1
go -C backend test ./internal/platform -count=1
go -C backend test ./... -count=1
bash backend/scripts/ci-security-gate.sh quick
bash backend/scripts/ci-security-gate.sh sonar
```

Live evidence:

```sh
kubectl -n nexuspaas rollout status deployment/platform-gateway
kubectl -n nexuspaas rollout status deployment/workload-service
curl http://127.0.0.1:<gateway-port>/healthz
curl http://127.0.0.1:<gateway-port>/readyz
psql "$DATABASE_URL" -c "select count(*) from platform_event_outbox"
psql "$DATABASE_URL" -c "select relay_status, count(*) from platform_event_outbox group by relay_status"
```

The live smoke should create or mutate one low-risk platform record, verify an
outbox row exists in Postgres, verify projection/inbox idempotency for a
consumer, and clean up any test record.

## 17. Rollback Plan

Revert the code wiring. The new tables are additive and can remain unused. If a
runtime issue appears, unset `DATABASE_URL` in local/dev or deploy the previous
image in production beta; existing Redis and in-memory event paths remain
backward compatible.

## 18. Risks and Tradeoffs

- This is not a full conversion of every service-specific repository write; it
  establishes the durable infrastructure and converts platform write paths first.
- Using Postgres as the primary event store adds query load to the database, but
  it avoids a second source of truth and gives the transaction boundary GA needs.
- Redis fan-out is eventually consistent from the Postgres outbox by design.
  Consumers that require owner-write atomicity must read through the Postgres
  event bus path.
- The existing migration runner is idempotent but not yet version/checksum based;
  that remains a separate P1 maturity item.

## 19. Reviewer Checklist

- Scope is small enough for one infrastructure slice.
- No new broker or unnecessary dependency is introduced.
- `EventStream` remains substitutable for in-memory, Redis-only, and Postgres
  implementations.
- Transactional extension does not force every `RecordStore` implementation to
  grow methods it cannot support.
- Runtime uses config/backing services rather than hardcoded endpoints.
- Relay is lease-gated and observable.
- Tests prove durable outbox/inbox, idempotent consumer behavior, relay retry
  state, and transaction rollback.
- Live evidence can verify rows in Postgres and health/readiness in RKE2.

## 20. Status

Status: Implemented; Reviewer approved

Reviewer Agent approval notes:

- Scope is acceptable for the first GA durable-event infrastructure slice.
- The design reuses Postgres, Redis Streams, and the existing lease-gated
  maintenance loop; no new broker or custom distributed transaction protocol is
  introduced.
- `RecordStore` and `EventStream` remain small, substitutable ports. The
  transactional write-with-event behavior is modeled as an optional extension.
- Required implementation evidence remains focused/full tests, quick gate,
  Sonar Quality Gate, and live RKE2 health/readiness plus Postgres outbox state.

Implementation evidence:

- Focused tests passed:
  `go -C backend test ./internal/platform -run 'PostgresEvent|Transactional|Projection|Runtime|Config|Schema' -count=1`.
- Platform package tests passed:
  `go -C backend test ./internal/platform -count=1`.
- Full backend tests passed:
  `go -C backend test ./... -count=1`.
- Quick gate passed:
  `bash backend/scripts/ci-security-gate.sh quick`.
- Coverage regenerated:
  `go -C backend test ./... -coverprofile=coverage.out -count=1`.
- SonarScanner Quality Gate passed.
- Migration job `nexuspaas-outbox-migrate` completed:
  `applied 18 migration unit(s) (platform schema + 17 service files)`.
- Validation job `nexuspaas-outbox-validate` completed:
  `validated 17 additive service migration files`.
- Live platform event tables exist in Postgres:
  `platform_event_outbox`, `platform_event_inbox`,
  `platform_event_checkpoints`.
- Initial outbox rollout image:
  `localhost:5000/nexuspaas-backend:ci-ga-outbox-20260620143955`
  (`sha256:56ebe7ea6318be42a932762f737478d58fa5db03f5a9975d857bdd33e568a955`).
- Final live image after PDP scope compatibility fix:
  `localhost:5000/nexuspaas-backend:ci-ga-pdp-scope-20260620163744`
  (`sha256:1817b0c42c37fe6e4d75e1155f7022084aac675dfb52857f16f7b45299b6af62`).
- All 15 backend deployments rolled out to the final live image and reported
  `1/1` ready.
- Live request-notification health and readiness returned `status: ok`.
- Live authorization-policy health and readiness returned `status: ok`.
- Live HTTP write smoke:
  `POST /api/v1/forms` returned HTTP 201 for marker
  `ga-outbox-live-20260620163919`.
- Live durable outbox evidence:
  `platform_event_outbox` contained one matching `FormCreated` event from
  `request-notification-service`, trace id
  `b37d765abc7e5682cbffa91f215dce7a`, relay status `pending`, attempts `0`.
- Outbox count increased from `17` to `20` during the live write smoke
  (`AuthorizationPolicy AuditEvent`, `FormCreated`,
  `RequestNotification AuditEvent`).
- Smoke form record and temporary raw policy rows were removed after evidence was
  captured; cleanup verification returned zero rows for all three cleanup
  checks.
