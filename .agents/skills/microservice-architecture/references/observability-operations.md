# Observability And Operations

Use this reference when designing telemetry, SLOs, alerts, runbooks, or incident readiness.

## Telemetry Signals

- Collect traces for request paths across service boundaries.
- Collect metrics for availability, latency, traffic, errors, saturation, queue depth, retry rate,
  and dependency health.
- Collect structured logs for discrete events and debugging context.
- Use OpenTelemetry concepts and semantic conventions when possible to reduce vendor lock-in.
- Preserve trace context across HTTP calls, RPC, message publishing, and message consumption.

## Correlation

- Generate or accept a correlation ID at the edge.
- Propagate correlation and trace context through every service and async hop.
- Include correlation ID, trace ID, span ID, service name, version, environment, and tenant/user
  context where allowed.
- Do not log sensitive payloads just to improve traceability.

## SLO Design

- Define user-facing SLOs for critical journeys.
- Map each service to the journeys it supports.
- Add dependency SLOs for external APIs, brokers, databases, and queues.
- Use error budgets to guide release pace and reliability investment.
- Separate platform health alerts from product/business impact alerts.

## Dashboards And Alerts

- Build dashboards per service and per journey.
- Alert on symptoms users feel before paging on internal causes.
- Include burn-rate alerts for SLOs where possible.
- Track saturation and queue lag before hard failures.
- Avoid paging on noisy metrics without an action path.

## Runbooks

Each production service should have:

- Owner and escalation path.
- Deployment and rollback commands.
- Dependency map and failure modes.
- Common alert explanations.
- Data repair or replay instructions where applicable.
- Known limits and capacity assumptions.

## Operational Red Flags

- No distributed trace for cross-service workflows.
- Logs cannot connect a user action to downstream calls.
- Alerts page a team that does not own the service.
- Dashboards show infrastructure only, not user journeys.
- Operators cannot tell whether stale data is expected or a bug.
