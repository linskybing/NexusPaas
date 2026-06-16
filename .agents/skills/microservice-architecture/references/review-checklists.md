# Review Checklists

Use this reference to produce architecture reviews, design comments, or readiness assessments.

## Quick Verdict

Classify the proposal:

- Green: microservices are justified, boundaries are stable, operations are mature, data ownership
  is clear, and failure modes are designed.
- Yellow: the goal is valid, but boundaries, contracts, data, or operations need tightening before
  implementation.
- Red: the proposal creates a distributed monolith, premature split, shared database dependency, or
  unowned operational burden.

## Architecture Checklist

- Business capability and bounded context are named.
- Service owner and on-call path are defined.
- API and event contracts are versioned.
- Owned data and source-of-truth rules are explicit.
- Cross-service consistency model is documented.
- Synchronous calls have timeout, retry, and fallback behavior.
- Async flows have idempotency, ordering, replay, and dead-letter handling.
- Security is enforced beyond the gateway.
- Telemetry covers logs, metrics, traces, and correlation IDs.
- Deployment, rollback, and migration paths are routine.

## Boundary Review

- What changes together?
- What must scale independently?
- What team owns the service for its full lifetime?
- What data cannot be accessed directly by other services?
- What would force two services to deploy together?
- What user or business workflow crosses the boundary?

## Data Review

- Which service owns each aggregate or entity?
- Are direct database reads or writes crossing service boundaries?
- What consistency delays can users observe?
- Which operations require compensation?
- How are duplicate, late, or out-of-order events handled?
- How is reconciliation performed?

## Security Review

- What identity is authenticated at the edge?
- What identity is propagated internally?
- How does the callee authorize the caller and user context?
- Are service credentials short-lived and scoped?
- Which policies are code-reviewed and tested?
- Which secrets or PII could appear in logs?

## Operational Review

- What SLO does this service support?
- Which alerts page the owner?
- How does the service behave when each dependency fails?
- How is a bad deployment rolled back?
- What dashboard shows health by user journey?
- What runbook handles replay, compensation, or repair?

## Red-Flag Phrases

- "We will share the database temporarily" with no removal plan.
- "The gateway owns the workflow."
- "Retries will make it reliable" without idempotency or limits.
- "Eventually consistent" without user-visible behavior.
- "The platform team will operate all services" without domain ownership.
- "We need microservices so teams can move faster" without contract discipline.
