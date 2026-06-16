# Service Boundaries

Use this reference when deciding whether to split a system and where boundaries should be drawn.

## Boundary Principles

- Start from business capability and bounded context, not tables, controllers, UI screens, or
  deployment convenience.
- A service boundary is credible only when it can own behavior, data, release cadence, operational
  health, and a clear team interface.
- Prefer coarse boundaries while the domain is unstable. Split more finely only after change
  patterns, load, or ownership pressure proves the need.
- Functions that change together usually belong together. Splitting them creates coordination cost
  and tight coupling.
- A service is not just a code package. It is an independently deployable unit with explicit
  contracts and operational ownership.

## Readiness Check

Microservices are risky when the team lacks:

- Automated provisioning and deployment.
- Monitoring, alerting, traceability, and rollback.
- On-call ownership and incident review.
- Contract testing or compatibility discipline.
- A clear path to service-level data ownership.

If these are weak, recommend a modular monolith or a small number of coarse services while the team
builds operational maturity.

## Boundary Heuristics

Good candidates:

- Different business language or model semantics.
- Different release cadence or compliance boundary.
- Different scale, latency, availability, or data retention needs.
- Ownership by a stable team that can run it in production.
- Small contract surface with limited synchronous dependencies.

Poor candidates:

- CRUD wrappers around single tables.
- Services split by technical layer such as "frontend", "backend", and "db".
- Services that require distributed transactions for normal operation.
- Services that must be deployed together most of the time.
- Services whose primary data is owned by another service.

## Team And Ownership

- Align service ownership with durable product or platform teams.
- Require each service to publish its runtime responsibility: owner, support channel, SLO,
  dependencies, runbook, and deprecation policy.
- Avoid central teams approving every domain change. Standardize cross-cutting tooling, not domain
  behavior.
- Let teams choose implementation details only inside platform guardrails for logging, auth,
  deployment, and observability.

## Granularity Tests

Ask these before accepting a split:

- Can the service be released without coordinating most consumers?
- Can failures be isolated or degraded without taking down the workflow?
- Is the API coarse enough to avoid chatty request chains?
- Does the service own the source of truth for its state?
- Does the team understand the domain well enough to keep the boundary stable?

## Red Flags

- "One service per database table."
- "We need microservices because the monolith file count is large."
- "The gateway will contain the orchestration and business rules."
- "All services will share the same database at first, then we will fix it."
- "Every internal call is synchronous because it is simpler."
