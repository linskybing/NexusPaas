# Operational Readiness

This document is the Production Beta operations contract for the 15 independent
backend services. It turns the platform NFRs into SLO, telemetry, alert,
runbook, rollback, and synthetic-smoke expectations that can be reviewed and
tested.

## Beta SLO Targets

| Target | Requirement |
| --- | --- |
| Core API availability | >= 99.5% monthly successful requests for core user journeys |
| Job submit synchronous phase | p95 < 2s for validation, quota reservation, and enqueue handoff |
| General read latency | p95 < 500ms for list, detail, dashboard, status, and catalog endpoints |
| General write latency | p95 < 1s for tenant-changing and workflow-changing endpoints |
| Internal owner-read latency | p95 < 300ms for service-to-service read contracts |

SLO burn alerts page only when there is a clear user-visible symptom or an
imminent error-budget violation. Dependency-only alerts create tickets unless
they affect a critical journey.

## Required Telemetry

All services must emit structured logs, Prometheus-compatible metrics, and
OpenTelemetry traces.

Required log fields:

- `service`
- `environment`
- `version`
- `request_id`
- `trace_id`
- `span_id`
- `user_id` when allowed
- `project_id` when allowed
- `http.route` or operation name
- `status`
- `latency_ms`
- `error_category`

Required metrics:

- HTTP request count by route and status class.
- HTTP request duration histogram with p50, p95, and p99 dashboards.
- Error count by route and error category.
- Dependency health and degraded-state counters.
- Retry, timeout, and circuit-breaker counters.
- Queue depth, event lag, worker saturation, or collector lag where applicable.
- Domain workflow counters for job, quota, storage, image, audit, notification,
  media, usage, and integration flows.

Required trace behavior:

- Accept or create `request_id` at the platform-gateway.
- Propagate W3C `traceparent` and `tracestate` across HTTP and event hops.
- Preserve safe user/project context across downstream calls.
- Set `OTEL_SERVICE_NAME` to the service name.
- Export traces through `OTEL_EXPORTER_OTLP_ENDPOINT`.

Do not log secrets, bearer tokens, API keys, raw cookies, raw OIDC assertions,
file payloads, or sensitive tenant data.

## Alert Policy

Page immediately for sustained Production Beta symptoms:

- Core API availability below the 99.5% burn target.
- Job submit synchronous phase p95 >= 2s.
- General read p95 >= 500ms or general write p95 >= 1s on user-visible routes.
- Quota reservation, job dispatch, K8s create, storage bind, image build, audit,
  notification, or usage read-model queues are stuck.
- Production service-to-service owner reads fail closed.
- `/healthz`, `/readyz`, `/metrics`, `/openapi.json`, `/service-registry`, or a
  per-service read-only synthetic smoke endpoint returns 5xx.

Open a ticket instead of paging for slow-burn non-core degradation, dashboard
panel gaps, or capacity warnings that still have error-budget runway.

## Standard Runbook Template

Every service runbook uses this minimum sequence:

1. Confirm the active alert, affected SLO, service, version, request_id, and
   trace_id.
2. Check `/healthz`, `/readyz`, `/metrics`, pod restarts, recent rollout, and
   service-registry presence.
3. Inspect service dashboard panels for latency, errors, saturation,
   dependency health, queue depth, and event lag.
4. Follow traces from platform-gateway to the owning service and downstream
   dependencies.
5. Decide whether to degrade, roll back, replay, repair data, or escalate to a
   dependency owner.
6. If rollback is needed, use `kubectl rollout undo deployment/<service>` or
   reapply the previous GitOps revision; do not restore databases as the first
   response.
7. For event-backed workflows, replay only idempotent events or run documented
   reconciliation jobs.
8. Record customer-visible impact, mitigation, root cause, and follow-up owner.

## Synthetic Smoke Checklist

Run this checklist after deployment, rollback, migration, or dependency
maintenance:

- `GET /healthz` returns 200 for each service.
- `GET /readyz` returns 200 for each service after backing services are ready.
- `GET /metrics` returns 200 and includes service labels.
- `GET /openapi.json` returns 200 for the gateway contract.
- `GET /service-registry` returns all 15 services.
- One read-only smoke endpoint per service returns 2xx or an expected 4xx; no
  service may return 5xx.
- Capture status, latency, request_id, trace_id, and service version.

## Service Operations Matrix

| Service | Owner / Escalation | Critical Journeys | Operations Contract |
| --- | --- | --- | --- |
| platform-gateway | Edge Platform on-call -> Backend lead | API entry, auth forwarding, routing, OpenAPI, service registry | SLO: core availability >= 99.5%, general read p95 < 500ms, general write p95 < 1s. Dashboard: route RED metrics, downstream latency, auth failures, rate-limit pressure, service-registry freshness. Alerts: availability/latency burn, downstream 5xx spike, missing service-registry entry, synthetic gateway 5xx. Runbook: inspect ingress, auth headers, route table, downstream traces, and recent rollout. Rollback: undo gateway deployment or revert routing config. Synthetic: `/healthz`, `/readyz`, `/metrics`, `/openapi.json`, `/service-registry`, and a read-only route smoke. |
| identity-service | IAM on-call -> Security lead | OIDC login, token exchange, API token validation, JWKS | SLO: core availability >= 99.5%, auth read p95 < 500ms, token write p95 < 1s. Dashboard: login rate, token issuance latency, JWKS cache health, invalid token categories, DB/Redis dependency health. Alerts: login/token availability burn, JWKS failures, auth 5xx, suspicious token error spike. Runbook: verify OIDC provider, JWKS, Redis session state, DB migrations, and audit trail. Rollback: undo identity deployment while preserving backward-compatible token schema. Synthetic: health/ready/metrics plus JWKS or token metadata read smoke. |
| authorization-policy-service | IAM on-call -> Security lead | RBAC policy reads, PDP decisions, policy changes | SLO: core availability >= 99.5%, policy decision p95 < 300ms internal, general writes p95 < 1s. Dashboard: PDP allow/deny rate, decision latency, policy reloads, cache invalidations, authz dependency errors. Alerts: PDP 5xx, decision latency burn, policy cache stale, policy write failures. Runbook: inspect policy bundle/version, cache invalidation, identity/org context, and denied trace samples. Rollback: undo service deployment or reapply previous policy bundle revision. Synthetic: health/ready/metrics plus read-only policy decision smoke. |
| org-project-service | Tenant Platform on-call -> Backend lead | org/project/group CRUD, membership reads, quota metadata | SLO: core availability >= 99.5%, tenant reads p95 < 500ms, tenant writes p95 < 1s. Dashboard: project/member route RED metrics, membership cache invalidation, quota metadata reads, DB health, owner-read latency. Alerts: tenant write failures, membership owner-read failures, high 4xx drift after deploy, DB saturation. Runbook: verify DB migration, membership events, authorization decisions, and owner-read traces from scheduler/workload. Rollback: undo deployment; keep expand-phase schema compatible. Synthetic: health/ready/metrics plus project list or membership read smoke. |
| workload-service | Compute API on-call -> Compute lead | config files, job submit/list/detail/cancel, job state machine | SLO: core availability >= 99.5%, job submit synchronous p95 < 2s, job reads p95 < 500ms. Dashboard: job submit latency, state transitions, scheduler reservation latency, queue handoff, config blob errors, downstream dependency health. Alerts: submit latency burn, job 5xx, stuck state transitions, scheduler owner-read failure, quota release failure. Runbook: follow job trace across scheduler, image, storage, and k8s-control; inspect idempotency keys and compensation status. Rollback: undo workload deployment after confirming migrations are backward-compatible. Synthetic: health/ready/metrics plus read-only job/config listing smoke. |
| scheduler-quota-service | Compute Control on-call -> Compute lead | quota reserve/commit/release, queue state, plan maintenance | SLO: core availability >= 99.5%, quota reserve p95 < 300ms internal, job submit path p95 contribution < 500ms. Dashboard: reservation rate, commit/release errors, queue depth, owner-read latency, plan-window reaper status, live quota reconciliation. Alerts: reserve/commit failures, queue lag, owner-read failures, negative quota drift, reaper/reconciler stalled. Runbook: inspect owner-read contracts to org-project/workload, Redis/Event bus health, DB locks, and stale reservations. Rollback: undo scheduler deployment; run reservation reconciliation before re-enabling traffic. Synthetic: health/ready/metrics plus quota status read smoke. |
| k8s-control-service | Compute Control on-call -> Infrastructure lead | workload creation, pod/node status, namespace/quota apply | SLO: core availability >= 99.5%, K8s command enqueue p95 < 1s, read status p95 < 500ms. Dashboard: Kubernetes API latency/errors, command queue depth, reconcile lag, namespace/quota apply failures, pod status freshness. Alerts: K8s API 5xx/timeout spike, reconcile lag, command queue stuck, service account permission failure. Runbook: verify cluster API, service account RBAC, namespace state, queue workers, and workload trace IDs. Rollback: undo k8s-control deployment or pause command workers while preserving desired state. Synthetic: health/ready/metrics plus read-only cluster summary smoke. |
| ide-service | Compute API on-call -> Compute lead | IDE start/stop, session proxy state, runtime status | SLO: core availability >= 99.5% for IDE API, start write p95 < 1s before async runtime wait, reads p95 < 500ms. Dashboard: start/stop rate, session state, proxy errors, workspace dependency health, active IDE count. Alerts: start/stop 5xx, session state stuck, proxy health failure, workspace saturation. Runbook: inspect job/workload dependency, session records, proxy routes, and pod readiness traces. Rollback: undo ide deployment; keep existing sessions running when possible. Synthetic: health/ready/metrics plus read-only IDE session list smoke. |
| storage-service | Platform IO on-call -> Infrastructure lead | storage bind, transfer, PVC/FileBrowser/Longhorn/MinIO integration | SLO: storage reads p95 < 500ms, bind request p95 < 1s before async external work, degraded state visible. Dashboard: bind requests, transfer latency, external storage API errors, PVC readiness, object-store dependency health. Alerts: bind/transfer 5xx, object-store unavailable, PVC reconcile lag, storage degraded counter. Runbook: inspect MinIO/Longhorn/FileBrowser dependencies, PVC events, transfer job traces, and authorization context. Rollback: undo storage deployment; pause transfer workers before replay. Synthetic: health/ready/metrics plus read-only storage binding list smoke. |
| image-registry-service | Platform IO on-call -> Infrastructure lead | image catalog, image request, build/publish, Harbor integration | SLO: image catalog reads p95 < 500ms, image request writes p95 < 1s, build status eventually consistent. Dashboard: catalog latency, build queue depth, Harbor API errors, allow-list cache, image request state transitions. Alerts: catalog 5xx, build queue stuck, Harbor dependency failure, image policy decision errors. Runbook: inspect Harbor, build workers, allow-list snapshots, storage dependency, and request trace. Rollback: undo image-registry deployment; preserve build queue and reconcile statuses. Synthetic: health/ready/metrics plus image catalog read smoke. |
| usage-observability-service | Ops Analytics on-call -> Backend lead | usage dashboards, cluster summary, GPU/resource usage read models | SLO: dashboard reads p95 < 500ms, read-model lag within Beta threshold, non-core degradation isolated. Dashboard: usage query latency, snapshot freshness, event lag, Prometheus query errors, cache hit rate. Alerts: dashboard 5xx, read-model lag, Prometheus dependency failure, collector worker stalled. Runbook: inspect event consumers, Prometheus queries, snapshot jobs, cache state, and source service traces. Rollback: undo usage deployment; rebuild read models from events if needed. Synthetic: health/ready/metrics plus dashboard summary read smoke. |
| audit-compliance-service | Compliance on-call -> Security lead | audit event ingest, audit search, compliance retention | SLO: audit ingest write p95 < 1s, audit search p95 < 500ms for Beta datasets, backlog drains continuously. Dashboard: audit ingest rate, backlog, search latency, retention worker status, dropped/invalid event count. Alerts: audit backlog stuck, ingest 5xx, retention failure, dropped audit events. Runbook: inspect event bus, audit DB, retention job, source service event IDs, and replay safety. Rollback: undo audit deployment; replay idempotent audit events after fix. Synthetic: health/ready/metrics plus read-only audit search smoke. |
| request-notification-service | Collaboration on-call -> Backend lead | forms/requests, announcements, notification delivery | SLO: request/notification reads p95 < 500ms, writes p95 < 1s, delivery lag bounded. Dashboard: request workflow RED metrics, notification queue depth, delivery errors, announcement publish rate, worker lag. Alerts: notification queue stuck, request workflow 5xx, delivery provider failures, announcement publish failure. Runbook: inspect event consumers, delivery provider, templates, authorization context, and retry/dead-letter state. Rollback: undo deployment; replay idempotent notification jobs after fix. Synthetic: health/ready/metrics plus read-only notification/request listing smoke. |
| integration-proxy-service | Integration on-call -> Infrastructure lead | pgAdmin/FileBrowser/proxy integrations, external adapter health | SLO: proxy control reads p95 < 500ms, external adapter degradation isolated, no 5xx on control-plane smoke. Dashboard: proxy route errors, adapter dependency health, latency by integration, auth failures, circuit-breaker state. Alerts: adapter 5xx spike, proxy auth drift, circuit breaker open for critical integration, control-plane smoke 5xx. Runbook: verify gateway route, downstream adapter health, credentials source, authorization decisions, and trace path. Rollback: undo integration-proxy deployment or disable one adapter route. Synthetic: health/ready/metrics plus read-only integration status smoke. |
| media-upload-service | Collaboration on-call -> Platform IO lead | media upload, bucket provisioning, object metadata | SLO: upload metadata writes p95 < 1s, object-store dependency degraded state visible, read metadata p95 < 500ms. Dashboard: upload rate, object-store latency/errors, bucket ensure status, metadata DB errors, payload rejection categories. Alerts: upload 5xx, object-store unavailable, bucket provisioning failure, high rejected uploads after deploy. Runbook: inspect MinIO/object store, bucket contract, metadata DB, payload validation, and trace IDs. Rollback: undo media-upload deployment; keep bucket/data intact and reconcile metadata. Synthetic: health/ready/metrics plus read-only media metadata smoke. |

## Remaining Production Beta Work

This document establishes the reviewable contract. Production Beta still needs:

- Dashboard resources provisioned in Grafana or an equivalent tool.
- Alert rules provisioned in PrometheusRule or an equivalent alerting system.
- Scheduled synthetic monitoring for the 15-service topology.
- Staging rehearsal for deploy, smoke, rollback, and re-deploy.
- GA replacement of static service keys with mTLS or workload identity.
