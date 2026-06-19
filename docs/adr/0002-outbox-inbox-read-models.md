# ADR 0002: Outbox, Inbox, And Read Models

Status: Accepted
Date: 2026-06-19

## Context

The current backend still uses shared physical PostgreSQL and transition
owner-read contracts in some paths. The GA roadmap requires a migration path
that removes high-risk shared-store reads without breaking external APIs,
requiring distributed transactions, or making database restore the default
rollback strategy.

The reviewed data migration strategy defines a common event envelope,
Outbox/Inbox flow, read-model rules, and expand/contract migration sequence.
This ADR makes that direction the accepted decision for GA decomposition.

## Decision

NexusPaas will migrate cross-unit state sharing through local transactional
Outbox, idempotent Inbox, and rebuildable read models.

Owners will write domain state and an outbox row in one local transaction. A
bounded relay publishes committed outbox rows. Consumers record inbox receipt,
deduplicate by `event_id`, and apply side effects or read-model updates
idempotently. Failed messages move to observable retry or dead-letter state.
Reconciliation compares owner state, outbox state, inbox state, and read-model
state.

The shared PostgreSQL instance may remain as migration scaffolding, but it is
not permanent shared ownership. New direct cross-unit repository reads or writes
must have an owner, expiry, and testable retirement condition.

## Event Envelope Requirements

All cross-unit events use a common envelope with these required fields:

| Field | Requirement |
| --- | --- |
| `event_id` | Globally unique idempotency key. |
| `schema_version` | Additive evolution by default; breaking changes require a new version. |
| `event_type` | Past-tense domain fact. |
| `producer` | Owning logical service and deployable unit. |
| `occurred_at` | Producer-side event time. |
| `trace_id` / `request_id` | Correlation across HTTP and async hops. |
| `aggregate_id` | UUID of the domain aggregate. |
| `payload` | UUIDs plus necessary snapshots; no internal DB ids that enable joins. |

## Read Model Requirements

Each read model must document:

- source events and producer ownership;
- consumer ownership;
- freshness target and stale-data behavior;
- rebuild and replay procedure;
- drift comparison method;
- duplicate, late, missing, and out-of-order event behavior;
- rollback and reconciliation procedure.

Critical request-time authorization can use owner-read APIs until freshness,
invalidation, and drift evidence prove a read model is safe for that workflow.
Owner-read APIs are transition contracts, not a permanent substitute for data
ownership.

## Migration Sequence

Every migration slice follows this sequence:

1. Expand: add events, read models, or tables without removing old paths.
2. Dual-write/read: keep old and new paths compatible while collecting evidence.
3. Backfill: rebuild target read models and compare counts, checksums, and domain invariants.
4. Compare: run drift checks for an approved operational window.
5. Cutover: move consumers behind a feature/config gate.
6. Contract: remove old shared-store paths only after rollback no longer depends on them.

## Compatibility And Contract Requirements

- External `/api/v1` routes and response envelopes remain stable.
- Event schemas and internal owner-read/command APIs require versioned fixtures.
- Providers and consumers must have contract tests before runtime contract changes.
- Consumers must tolerate unknown fields and missing optional fields within the
  declared compatibility window.
- Every outbox relay and inbox consumer must emit publish lag, consumer lag,
  retry count, dead-letter count, replay progress, and drift metrics.

## Consequences

- The platform avoids two-phase commit across units.
- Eventual consistency becomes explicit and testable instead of hidden behind
  shared-store access.
- Rollback moves toward replay, compensation, and reconciliation instead of
  database restore.
- Follow-up slices must add concrete fixtures, storage tables, relays, inbox
  state, and tests before claiming implementation progress.

## Rejected Alternatives

| Alternative | Reason Rejected |
| --- | --- |
| Permanent shared operational database | Produces a distributed monolith and obscures data ownership. |
| Distributed transactions across units | Increases availability and operations risk for workflows that can use saga, idempotency, and compensation. |
| Synchronous owner-read for all cross-unit reads | Keeps coupling in request paths and does not support reporting or stale-data visibility. |
| Ad hoc event payloads per producer | Allows producer/consumer drift and weakens replay compatibility. |

## Follow-up Evidence

- Add event schema fixtures and compatibility tests for identity, tenant,
  workload, scheduler, and audit events.
- Add provider/consumer tests for owner-read and command APIs.
- Implement Outbox/Inbox infrastructure with idempotency, retry, dead-letter,
  lag, replay, and drift evidence.
- Update `problem.md` only when implementation and evidence blockers are reduced.

## Reversal

A future ADR may replace this decision only if it provides a simpler and safer
migration model with equivalent idempotency, compatibility, rollback,
reconciliation, and observability evidence. Reversal must not require breaking
external `/api/v1` compatibility or relying on destructive database rollback.
