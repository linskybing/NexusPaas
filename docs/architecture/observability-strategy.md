# Observability Strategy

## Purpose

NexusPaas Production Beta runs on 8 physical backend units hosting 15 logical
services.
The observability goal is to let operators answer four questions during a user
incident:

1. Which user journey is affected?
2. Which service owns the failing behavior?
3. Which dependency, queue, or external system is causing the symptom?
4. Which rollback or repair path is safe?

The concrete per-service operating contract lives in
`../../backend/docs/operational-readiness.md`. This strategy explains the shared
model behind that contract.

## Scope

This strategy covers backend and operations readiness for Production Beta:

- Structured application logs.
- Prometheus-compatible metrics.
- OpenTelemetry traces.
- Service and journey SLOs.
- Dashboards and alerts.
- Runbooks, rollback, and synthetic smoke.

It does not select a hosted vendor. Production Beta should stay compatible with
OpenTelemetry Collector, Prometheus, Grafana, Loki, Tempo, Jaeger, or equivalent
CNCF-aligned components.

## Signal Model

Every backend unit must emit the same three signal families while preserving
logical-service labels for route and workflow ownership.

### Logs

Logs are event streams, not files to manage inside containers. Each log event
must include:

- `service`
- `environment`
- `version`
- `request_id`
- `trace_id`
- `span_id` when available
- `user_id` when allowed
- `project_id` when allowed
- `http.route` or operation name
- `status`
- `latency_ms`
- `error_category` for failures

Services must not log secrets, bearer tokens, API keys, raw cookies, file
payloads, raw OIDC assertions, or sensitive tenant data.

### Metrics

Every service dashboard starts with RED and saturation signals:

- Request rate by route and status class.
- Error rate by route, status class, and error category.
- Duration histogram with p50, p95, and p99.
- Saturation for workers, queues, connection pools, and external API limits.
- Dependency health and retry/circuit-breaker state.
- Domain counters for service-specific workflows.

The usage-observability service also tracks product-facing read model lag, GPU
usage snapshot freshness, and dashboard query latency.

### Traces

The platform-gateway accepts or generates a correlation ID at the edge and
propagates W3C Trace Context across HTTP and event boundaries. Every service
keeps `traceparent`, `tracestate`, `request_id`, and safe user/project context
when making downstream calls or publishing events.

`OTEL_EXPORTER_OTLP_ENDPOINT` is the deployment contract for trace export.
`OTEL_SERVICE_NAME` must match the service name in the Kubernetes deployment and
the service registry.

## SLO Classes

Production Beta SLOs are intentionally modest but measurable.

| SLO Class | Target | Applies To |
| --- | --- | --- |
| Core API availability | >= 99.5% monthly successful requests | platform-gateway, identity, authorization, org-project, workload, scheduler-quota, k8s-control |
| Job submit synchronous phase | p95 < 2s | workload submit, scheduler quota reserve, k8s-control enqueue handoff |
| General read latency | p95 < 500ms | list, detail, dashboard, catalog, status, policy read endpoints |
| General write latency | p95 < 1s | tenant-changing writes, policy writes, quota writes, requests, notifications, uploads metadata |
| Internal owner-read latency | p95 < 300ms | service-to-service read contracts used by isolated services |

SLOs are user-journey oriented. Infrastructure-only alerts are useful for
triage, but paging starts from product symptoms: availability burn, latency
burn, queue lag, rejected job submissions, failed storage/image operations, or
audit backlog.

## Dashboard Model

Each service owns one service dashboard and participates in one or more journey
dashboards. A service dashboard must include:

- Request rate, errors, and latency.
- Saturation and worker/queue depth where applicable.
- Dependency health and degraded-state counters.
- Recent deploy version and pod restart count.
- Trace exemplars or links by route.
- Top error categories with request IDs.

Journey dashboards aggregate services around critical flows:

- Login and token issuance.
- Authorization and policy decisions.
- Org, project, and group administration.
- Job submit, queue, run, cancel, and quota release.
- IDE start and stop.
- Storage binding and transfer.
- Image catalog, request, and build.
- Media upload.
- Notification, form, and audit delivery.
- Usage dashboard and cluster summaries.
- Integration proxy health.

## Alert Policy

Alerts are split into page and ticket severity.

Page when:

- Core API availability burn threatens the 99.5% Beta target.
- p95 job submit synchronous phase exceeds 2s for a sustained window.
- General reads exceed 500ms p95 or writes exceed 1s p95 for user-visible
  routes.
- Queue/event lag blocks job, audit, notification, usage, or cleanup workflows.
- Service-to-service owner reads fail closed for production traffic.
- Storage, image, K8s, OIDC, Redis, Postgres, or object-store dependencies cause
  user-visible degraded state.

Create a ticket when:

- A non-core service is degraded but critical job submission remains healthy.
- Error budget burn is slow and no user-visible outage is active.
- A dashboard panel has missing data but synthetic smoke still passes.

Every alert must have an action path in
`../../backend/docs/operational-readiness.md`.

## Runbooks And Rollback

Every service runbook must include:

- Owner and escalation path.
- Critical user journeys.
- Dependencies and failure modes.
- First five triage checks.
- Safe rollback command or rollout undo path.
- Data repair, replay, or reconciliation steps when applicable.
- Customer/support communication notes for visible degradation.
- Post-incident follow-up checklist.

Rollback should prefer rolling back the service image or config that introduced
the issue. Database rollback by restore is not an acceptable default path.
Schema changes must follow expand, dual-read/write, backfill, cutover, contract.

## Synthetic Smoke

Synthetic smoke runs against the deployed topology, not only the all-in-one
binary. The baseline check is:

- `/healthz`
- `/readyz`
- `/metrics`
- `/openapi.json`
- `/service-registry`
- One read-only smoke endpoint per service

Smoke failures should record request ID, trace ID, status code, latency, and the
service-registry snapshot so operators can determine whether the issue is
routing, startup, auth, dependency health, or service behavior.

The Production Beta baseline overlay is
`backend/deploy/observability/production-beta`. It contains a Grafana dashboard
ConfigMap, Prometheus Operator `PodMonitor` and `PrometheusRule` resources, and
a Kubernetes `CronJob` synthetic monitor. The overlay is separate from the root
backend kustomization because Prometheus Operator CRDs are optional
infrastructure prerequisites and should not break the root non-live dry-run
gate. The PodMonitor uses `nexuspaas-prometheus-scrape-secret` for bearer-token
scrape auth; the CronJob uses `nexuspaas-synthetic-smoke-secret` for smoke
credentials. Runtime HTTP duration metrics include Prometheus histogram buckets,
so p95 dashboard panels and alert rules use `histogram_quantile`.

## GA Architecture Extension

The Production Beta topology groups the 15 logical services into 8 deployable
units. Observability must preserve both views:

- unit-level dashboards for deployment, rollback, saturation, and dependency
  health;
- logical-service labels for route, event, and domain workflow ownership;
- journey dashboards for login, authorization, tenant management, job submit,
  queue/dispatch, IDE, storage, image, usage, audit, notification, media, and
  proxy flows.

Every Outbox/Inbox and read-model migration must add telemetry for publish lag,
consumer lag, retry count, dead-letter count, replay progress, and drift
comparison. Operators must be able to tell whether stale data is within a
documented freshness target or is an incident.

Each of the 8 deployable units must capture staging evidence before it can be
called GA-ready:

- applied version and configuration source;
- `/healthz`, `/readyz`, `/metrics`, and service-registry evidence;
- one read-only synthetic smoke endpoint for every logical service in the unit;
- rollback, redeploy, and post-redeploy smoke evidence;
- trace IDs and request IDs for critical cross-unit journeys.

Static service keys are a Production Beta transition. GA observability should
make the active service identity visible through safe labels or trace
attributes, without logging raw credentials or user tokens.

## Production Beta Gaps

This strategy closes the documentation, testable contract, and baseline
observability manifest gap. The following remain launch-readiness work:

- Activate the overlay in a live cluster and capture dashboard, alert, scrape,
  and CronJob evidence.
- Replace static service keys with mTLS or workload identity before GA.
- Exercise rollback and incident runbooks in staging.
