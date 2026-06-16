# Data Consistency

Use this reference when a design touches databases, ownership, transactions, events, read models, or
reporting.

## Data Ownership

- Each service owns its durable state and schema.
- Other services access that state through APIs, events, or replicated read models, not direct table
  reads.
- Treat shared databases as temporary migration scaffolding with a removal date.
- Do not split a service if the proposed boundary cannot own meaningful data.
- Prefer the storage technology that fits the service's access pattern, while staying inside
  platform support limits.

## Consistency Model

- Default to local ACID inside one service boundary.
- Across service boundaries, expect eventual consistency and design user-facing workflows around it.
- Make delays, pending states, compensation, and reconciliation visible where users or operators can
  observe them.
- Avoid distributed two-phase commit in normal microservice workflows; it couples participants and
  hurts availability.

## Saga Use

Use a saga when a business workflow spans multiple service-owned data stores and needs a reliable
end state.

- Use choreography for simple workflows with few participants and obvious event flow.
- Use orchestration when the workflow has many steps, branching, audit needs, or operational
  visibility requirements.
- Define compensating actions before implementation.
- Require idempotent participants and retry-safe commands.
- Add traceability for each saga instance, step, retry, and compensation.

## Transactional Outbox

Use the outbox pattern when a service must update its database and publish an event reliably.

- Write the domain change and outbox entry in the same local transaction.
- Publish committed outbox entries through a relay or change data capture.
- Preserve ordering where business correctness depends on it.
- Make consumers idempotent because delivery can be at-least-once.
- Monitor relay lag, failed publishes, and dead-letter volume.

## Read Models And Queries

- Use CQRS or materialized read models when query needs span multiple services.
- Build read models from events or approved APIs, not direct database coupling.
- Define freshness expectations and stale-data behavior.
- Avoid making a reporting requirement the reason to share operational schemas.

## Data Migration Guardrails

- Plan source-of-truth transfer explicitly.
- Run dual writes only as a bounded transition with reconciliation.
- Add checksums, counts, or domain invariants to prove migrated data quality.
- Retire synchronization agents after consumers move to the new source.
