# Data Ownership, Events, and Contracts

Part of the [GA Acceptance docs](README.md). Related: ADR 0002 (outbox/inbox &
read models) under `docs/adr/`.

## Goal

The platform must avoid becoming a distributed monolith.

Cross-unit state sharing must use typed contracts, owner-read APIs, commands,
events, and read models.

## Required Event Pattern

All cross-unit domain events use a common envelope:

```json
{
  "event_id": "uuid",
  "schema_version": "v1",
  "event_type": "JobSubmitted",
  "producer": "compute-api.workload-service",
  "occurred_at": "2026-06-20T00:00:00Z",
  "trace_id": "trace-id",
  "request_id": "request-id",
  "aggregate_id": "job-id",
  "payload": {}
}
```

## Outbox / Inbox Requirement

Owner writes must persist domain state and outbox row in the same transaction.

Consumers must deduplicate through inbox state.

Minimum tables:

```text
outbox_events
inbox_events
```

## Acceptance Criteria

| ID | Acceptance Criteria |
|---|---|
| DATA-001 | Core domain models use typed tables/repositories, not only generic JSON records. |
| DATA-002 | New core domain data cannot be added to generic record store without ADR and migration plan. |
| DATA-003 | Domain write and outbox event are committed in the same transaction. |
| DATA-004 | Outbox relay publishes committed events with retry. |
| DATA-005 | Inbox deduplicates by consumer and event ID. |
| DATA-006 | Failed messages have retry state and dead-letter state. |
| DATA-007 | Read models document freshness target and stale-data behavior. |
| DATA-008 | Read models can be rebuilt from events or owner snapshots. |
| DATA-009 | Event schemas are versioned. |
| DATA-010 | Event consumers tolerate unknown additive fields. |
| DATA-011 | Breaking event changes require new schema version. |
| DATA-012 | Owner-read APIs have provider and consumer contract tests. |
| DATA-013 | Command APIs are idempotent. |
| DATA-014 | Submit, cancel, preempt, build, and deploy APIs support idempotency keys. |
| DATA-015 | Cross-service direct repository access is forbidden unless explicitly documented as temporary migration debt. |
| DATA-016 | Drift checks compare owner data and read-model data. |
| DATA-017 | Outbox publish lag and consumer lag are observable. |
| DATA-018 | Event replay procedure is documented and tested. |
