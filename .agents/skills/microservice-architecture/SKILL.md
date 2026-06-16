---
name: microservice-architecture
description: >
  General microservice architecture best-practice guide for service decomposition, bounded contexts,
  team ownership, API and event contracts, database-per-service data ownership, eventual
  consistency, saga/outbox patterns, resilience, service mesh and zero-trust security, OpenTelemetry
  observability, CI/CD, contract testing, progressive delivery, and monolith-to-microservice
  modernization. Use when designing, reviewing, decomposing, migrating, or production-hardening a
  microservice system, or when checking whether microservices are appropriate.
---

# Microservice Architecture

## Overview

Use this skill to turn microservice discussion into a concrete architecture decision. Prefer
business-aligned, independently deployable services only when the organization can operate a
distributed system safely.

## Core Workflow

1. Confirm the force pushing toward microservices: independent scaling, release cadence, team
   autonomy, fault isolation, technology fit, or legacy migration.
2. Test readiness: automation, monitoring, rollback, ownership, on-call, and contract discipline
   must exist before increasing service count.
3. Draw boundaries from domain language and change cadence, then validate them against data
   ownership, transaction needs, and team ownership.
4. Choose communication per workflow: synchronous calls for immediate queries, asynchronous
   messaging for decoupling, and explicit events for state changes.
5. Design data ownership before APIs. Each service owns its schema and exposes behavior or events,
   not direct table access.
6. Add runtime controls: timeouts, bounded retries, circuit breakers, bulkheads, load shedding,
   health checks, graceful shutdown, and idempotency.
7. Add security and operations as first-class design constraints: identity propagation,
   service-to-service authentication, policy-as-code, telemetry, SLOs, and rollback paths.
8. Produce a decision record that names accepted trade-offs and rejected alternatives, including why
   a modular monolith is or is not sufficient.

## Reference Map

- Read [source-index.md](references/source-index.md) to verify source authority and topic coverage.
- Read [service-boundaries.md](references/service-boundaries.md) for bounded contexts, ownership,
  granularity, and decomposition readiness.
- Read [communication-contracts.md](references/communication-contracts.md) for REST/RPC, async
  messaging, API gateways, events, and versioning.
- Read [data-consistency.md](references/data-consistency.md) for private schemas,
  database-per-service, saga, outbox, CQRS, and read model trade-offs.
- Read [resilience-runtime.md](references/resilience-runtime.md) for timeouts, retries, circuit
  breakers, bulkheads, orchestration, and stateless runtime.
- Read [security-zero-trust.md](references/security-zero-trust.md) for gateway limits, service
  authorization, identity propagation, mTLS, and policy-as-code.
- Read [observability-operations.md](references/observability-operations.md) for logs, metrics,
  traces, correlation IDs, SLOs, and incident readiness.
- Read [testing-delivery.md](references/testing-delivery.md) for contract tests, CI/CD, rollout,
  rollback, and progressive delivery.
- Read [migration-modernization.md](references/migration-modernization.md) for modular monolith,
  strangler fig, anti-corruption layer, and extraction steps.
- Read [review-checklists.md](references/review-checklists.md) when producing an architecture review
  or readiness assessment.

## Output Expectations

- State whether microservices are justified now, later, or not for this case.
- Name each proposed service by business capability, not technology layer.
- For every service, identify owner, API/event contracts, owned data, SLOs, security boundary,
  observability signals, and rollout strategy.
- Call out distributed-system costs directly: latency, debugging, data consistency, operational
  burden, and cross-service change coordination.
- Prefer incremental migration plans over big-bang rewrites.

## Guardrails

- Do not split services only by controller, table, CRUD entity, framework module, or team
  preference.
- Do not share databases across services except as a temporary migration tactic with a retirement
  plan.
- Do not rely on an API gateway as the only authorization layer.
- Do not add synchronous call chains without latency budgets, timeout budgets, and failure behavior.
- Do not treat eventual consistency as an implementation detail; make it a product-visible contract
  where users or workflows can observe it.
