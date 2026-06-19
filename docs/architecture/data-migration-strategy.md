# Data Migration Strategy

## Goal

Move NexusPaas from shared physical PostgreSQL and transition owner-read
contracts toward service-owned state, versioned events, and read models without
breaking external APIs or requiring database restore as rollback.

## Ownership Model

- Each deployable unit owns its source-of-truth records.
- Other units store UUID references and necessary snapshots only.
- Shared PostgreSQL may remain during migration, but direct cross-unit reads or
  writes are technical debt with an owner and retirement gate.
- Reporting requirements must use read models, not operational schema sharing.

## Event Envelope

All cross-unit events must use a common envelope:

| Field | Requirement |
| --- | --- |
| `event_id` | Globally unique idempotency key. |
| `schema_version` | Additive schema evolution by default; breaking changes require a new version. |
| `event_type` | Past-tense fact such as `ProjectUpdated` or `JobPreempted`. |
| `producer` | Owning logical service and deployable unit. |
| `occurred_at` | Producer-side event time. |
| `trace_id` / `request_id` | Correlation across HTTP and async hops. |
| `aggregate_id` | UUID of the domain aggregate. |
| `payload` | UUIDs plus necessary snapshots; no internal DB ids that enable joins. |

## Outbox / Inbox Flow

1. The owner writes the domain change and outbox row in one local transaction.
2. A bounded relay publishes committed outbox entries to the event bus.
3. Consumers record inbox receipt before applying side effects.
4. Consumers deduplicate by `event_id` and tolerate duplicate, late, or
   out-of-order delivery.
5. Failed messages move to observable retry or dead-letter state.
6. Reconciliation jobs compare owner state, outbox state, and read model state.

## Read Model Rules

- Read models define source event types, owner, freshness target, rebuild
  procedure, and stale-data behavior.
- User-visible stale data must be labeled or bounded by documented freshness.
- Critical request-time authorization may call owner-read APIs until read-model
  freshness and invalidation are proven.
- Every read model has tests for duplicate event, missing event replay, stale
  read behavior, and rebuild from retained events.

## Migration Sequence

Every data-boundary migration uses this sequence:

| Step | Gate |
| --- | --- |
| Expand | Add new events/read models/tables without removing old paths. |
| Dual-write/read | Keep old and new paths compatible while emitting comparison evidence. |
| Backfill | Rebuild target read models and compare counts, checksums, and domain invariants. |
| Compare | Run drift checks for one operational cycle or an approved shorter Beta window. |
| Cutover | Move consumers to owner APIs or read models behind a feature/config gate. |
| Contract | Remove old shared-store paths only after rollback no longer depends on them. |

## First 90-Day Candidates

- Tenant membership and project access read models for workload, image,
  storage, request-notification, and scheduler authorization decisions.
- Scheduler quota read models for project plan/queue policy and workload
  running-state summaries.
- Workload job state events consumed by usage, audit, request-notification, and
  compute-control-plane reconciliation.
- Identity and authorization projection events for display names, user status,
  role/policy changes, and cache invalidation.

## Stop Conditions

- A migration requires two-phase commit across units.
- Rollback requires manual database restore or destructive schema rollback.
- A read model has no rebuild path.
- A shared-store dependency is added without owner approval, expiry, and a
  testable retirement condition.
- Operators cannot distinguish stale data from an incident.
