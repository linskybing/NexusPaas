# ADR 0006: Course Monitoring Scope

Status: Accepted
Date: 2026-06-24

## Context

The reference platform includes a course-specific monitoring reconciler. It builds
an allow-list regex from active course plan windows, then patches course-specific
`ScrapeConfig` and `ServiceMonitor` custom resources in the `monitoring`
namespace. That reconciler is gated by
`COURSE_MONITORING_RECONCILER_ENABLED=true` and is tied to course-metrics
Prometheus/Thanos persistence for selected course projects, including targets
such as `course-cadvisor`, `course-kube-state`, and `course-mps-gpu`.

Current NexusPaaS GA acceptance requires general platform monitoring, usage
reporting, telemetry, retention, and alerting evidence. It does not define a
course-project-class product model or course-specific metric persistence
requirements.

## Decision

Course-specific monitoring reconciliation is out of scope for the current
NexusPaaS GA.

NexusPaaS will continue to track general MON GA, usage reporting, Prometheus
retention, alerting, live monitoring evidence, V1 external launch, and Full GA as
open work. This decision only resolves the scope question for the reference
course-monitoring reconciler; it does not reduce or close the current monitoring
and usage acceptance requirements.

## Consequences

- Do not add a course-monitoring reconciler for current GA.
- Do not add `COURSE_MONITORING_RECONCILER_ENABLED` or related course-monitoring
  config/env settings.
- Do not add course-specific `ScrapeConfig`, `ServiceMonitor`, Prometheus,
  Thanos, CRD, deployment, runtime, test, or live-evidence changes under this
  decision.
- Keep MON GA, usage reporting, Prometheus retention, alerting, live monitoring
  evidence, V1 external launch, and Full GA open until separately evidenced.

## Revisit Criteria

Revisit this ADR only if a future course-metrics product requirement is accepted.
That requirement must define acceptance criteria, owning service/team,
configuration and environment variables, CRD and deployment changes, tests, and
live evidence for course-project metric persistence before implementation begins.
