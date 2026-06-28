# Service Boundaries

## Boundary Policy

NexusPaas boundaries are based on business capability, data ownership,
operational responsibility, and release independence. A logical service can
remain co-deployed while its contracts and data ownership are hardened. A
deployable unit is accepted only when it owns its runtime responsibility,
publishes explicit contracts, has rollback evidence, and can fail without
turning unrelated domains into an outage.

Provider names are reference implementations for owned NexusPaaS contracts, not
the contracts themselves. ADR 0007 defines the provider coupling boundary for
Longhorn/reference storage, Harbor, MinIO/S3, Dex/OIDC, Redis Streams, and the
k3s/dev baseline: [Provider Coupling Boundary](../adr/0007-provider-coupling-boundary.md).

## Deployable Unit Map

| Unit | Owned Capabilities | Source-of-Truth Data | Contracts | Primary Consumers | SLO / Operations |
| --- | --- | --- | --- | --- | --- |
| `platform-gateway` | Edge routing, auth entry, service registry, OpenAPI, coarse rate limits | Route config, gateway caches, rate-limit state | External `/api/v1`, `/healthz`, `/readyz`, `/metrics`, `/service-registry`, OpenAPI | All clients and browser proxy paths | Core API availability, route RED metrics, downstream latency, rollback by gateway deployment |
| `iam-unit` | Login, sessions, OIDC, API tokens, users, roles, RBAC/PDP, policy bundles | Users, sessions, refresh tokens, API token metadata, policy rules, policy assignments | JWKS, token validation, PDP decisions, policy events, identity events | Gateway, tenant, compute, proxy, audit | Auth availability, PDP latency, policy reload health, credential rotation, rollback with compatible token/policy schema |
| `tenant-unit` | Groups, projects, project members, user groups, tenant quota metadata, workspace settings | Projects, members, groups, user quotas, GPU claims, workspace settings | Project/member owner reads, membership events, project lifecycle events | Compute, scheduler, storage, image, usage, authz | Tenant read/write latency, membership freshness, owner-read latency, rollback with expand-phase schema |
| `collaboration-unit` | Audit ingestion/search, forms, notifications, announcements, media metadata | Audit logs, notification state, forms/messages, announcement reads, media metadata | AuditEvent ingestion, notification commands/events, media upload/read contracts | All services, frontend, support tooling | Audit backlog, notification lag, media object-store health, replay-safe workers |
| `platform-io-unit` | Storage binding, fast transfer reference integrations, image requests/builds/catalog, OCI registry governance, external proxies | Storage records, permissions, transfer records, image requests, image catalog/build state, proxy sessions | Mount-plan API, image allow-list/build APIs, proxy auth-check, storage/image events | Compute, tenant, audit, request-notification, users | Storage/image read/write SLOs, external dependency degraded state, worker queues, rollback with reconcile |
| `usage-observability` | Usage dashboards, GPU/resource summaries, cluster usage read models | Usage snapshots, resource-hour summaries, dashboard cache/read models | Usage read APIs, snapshot events, dashboard summaries | Users, admins, audit, compute | Dashboard p95, read-model lag, Prometheus dependency health, rebuildable read models |
| `compute-api` | ConfigFile lifecycle, job submit/list/cancel, IDE lifecycle and proxy state | Config files, immutable commits, jobs, job logs metadata, IDE sessions | Job commands/events, config events, IDE lifecycle events, scheduler admission calls | Users, scheduler, k8s-control, usage, audit | Job submit sync p95, state transition health, IDE lifecycle, compensation visibility |
| `compute-control-plane` | Queue/plan/quota, priority, preemption, Kubernetes command/status, runtime cleanup | Plans, queues, reservations, quota state, preemption records, K8s command/snapshot records | Admission/reserve/commit/release, K8s command/status, resource snapshot events | Compute API, tenant, usage, audit, operators | Quota reserve p95, queue lag, K8s API health, reaper/reconciler status, rollback with reservation reconciliation |

## Logical Service Placement

| Logical Service | Unit | Boundary Notes |
| --- | --- | --- |
| platform-gateway | `platform-gateway` | Owns edge policy and routing, not domain workflow orchestration. |
| identity-service | `iam-unit` | Owns credentials, sessions, users, API tokens, OIDC, LDAP projection. |
| authorization-policy-service | `iam-unit` | Owns PDP, RBAC, proxy policy, signed policy bundles. |
| org-project-service | `tenant-unit` | Owns tenant hierarchy, membership, and project quota metadata. |
| audit-compliance-service | `collaboration-unit` | Owns audit ingestion/search and retention. |
| request-notification-service | `collaboration-unit` | Owns forms, notifications, announcements, unread state. |
| media-upload-service | `collaboration-unit` | Owns media metadata and object keys; object store is an attached resource. |
| storage-service | `platform-io-unit` | Owns storage/PVC/FileBrowser records and mount-plan contracts. |
| image-registry-service | `platform-io-unit` | Owns image request, allow-list, build, catalog, and OCI registry governance; Harbor is the current reference implementation. |
| integration-proxy-service | `platform-io-unit` | Owns external proxy sessions and adapter health, not domain authorization policy. |
| usage-observability-service | `usage-observability` | Owns read models, snapshots, resource-hour summaries, dashboard queries. |
| workload-service | `compute-api` | Owns job/config API state and user-facing job lifecycle. |
| ide-service | `compute-api` | Owns IDE session lifecycle and activity tracking. |
| scheduler-quota-service | `compute-control-plane` | Owns quota arbitration, queue policy, plan windows, preemption. |
| k8s-control-service | `compute-control-plane` | Owns Kubernetes API commands, snapshots, namespace/resource control. |

## Cross-Boundary Rules

- Cross-service writes use commands or events, not direct repository calls.
- Cross-service reads use owner-read APIs only when a request-time decision
  needs fresh data; otherwise use event-fed read models with freshness targets.
- Every event consumer must be idempotent and record inbox processing evidence.
- Every synchronous call has a timeout, error category, retry policy, and
  fail-closed or degraded behavior.
- Gateway checks are not enough. Each owning unit enforces domain authorization.
- Rollback must be service image/config rollback plus reconciliation, not
  database restore as the default response.
