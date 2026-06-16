# Decomposition Phases, Risks, and Acceptance Criteria

## 1. Decomposition Phases

| Phase | Goal | Main Work | Rationale |
| --- | --- | --- | --- |
| 0. Boundary preparation | Keep the monolith deployed, but finish module-boundary and data-boundary cleanup first. | Establish internal contracts; forbid new features from using Repos across modules directly; introduce outbox/inbox; complete OpenAPI contracts; build a route-to-service map; add owner columns and event tables to DB schemas. | Validates microservice boundaries without touching the frontend. |
| 1. Gateway and low-coupling services | Build platform-gateway first; extract audit-compliance, request-notification, and media-upload. | Gateway preserves /api/v1; all services emit AuditEvent; forms/announcements/notifications go through service APIs; image uploads move to the media service. | Lowest transactional coupling — easiest wins. |
| 2. External-integration services | Extract integration-proxy, image-registry, and storage-service. | Harbor/MinIO/pgAdmin/Longhorn/Storage/FileBrowser are managed by dedicated services; the core monolith calls service APIs instead; introduce async status. | Reduces the risk of external-system timeouts dragging down the core API. |
| 3. Read models and usage | Extract usage-observability-service. | Build read models for GPU/resource hours/cluster summary from events and Prometheus; dashboards query the usage service. | Read-heavy and eventually consistent — well suited for extraction. |
| 4. IAM, Authz, Org Project | Extract identity-service, authorization-policy-service, and org-project-service. | Gateway validates tokens via JWKS; Authz PDP/SDK unifies authorization; membership/project events build per-service read models; remove direct DB joins. | Higher risk — requires dual writes, cache invalidation, and contract tests. |
| 5. Workload, Scheduler, K8s Control, IDE | Split the most critical compute control plane last. | Job submit becomes a saga: Validate → ReserveQuota → ResolveImage/Storage → CreateK8sWorkload → Commit/Release; centralize K8s operations in k8s-control-service. | Requires strict handling of TOCTOU, retries, compensation, and state machines. |
| 6. Convergence and governance | Remove legacy monolith routes and shared-database dependencies. | Turn off dual writes; drop legacy tables/foreign keys; complete SLOs, on-call runbooks, chaos testing, and security scanning. | Produces real microservices instead of a distributed monolith. |

## 2. Major Risks and Mitigations

| Risk | Severity | Mitigation |
| --- | --- | --- |
| Shared database produces a distributed monolith | High | Each service owns its own schema/database; forbid cross-service joins; sync read models via events. |
| Job quota / dispatch TOCTOU | High | scheduler-quota-service provides Reserve/Commit/Release; every resource consumption reserves before dispatch. |
| RBAC rule drift | High | Centralized PDP or signed policy bundles; Gateway/services use the same SDK version; PolicyChanged events invalidate caches. |
| Stale caches after Project/Group membership changes | Medium-High | MembershipChanged events + short TTL + enforced version numbers; critical operations query Org/Authz synchronously. |
| Slow or unavailable external APIs (K8s/Harbor/Longhorn) | Medium-High | Switch to async command + status; timeout/retry/circuit breaker; service degradation. |
| Proxy/WebSocket behavior vs. browser Cookie/API Key limitations | Medium | Keep JWT-only proxy entries; the Gateway unifies cookie/JWT validation and forwarding. |
| Service outages during data migration | High | expand/dual-write/backfill/cutover/contract; every stage is rollback-able; add data-comparison tooling. |
| Operational cost explosion from too many services | Medium | Phase the rollout; initially co-deploy Workload+Scheduler, IAM+Authz, Request+Media while keeping code and data boundaries. |

## 3. Cutover Acceptance Criteria

- The Gateway externally preserves the existing /api/v1 routes, OpenAPI, and standard JSON response envelope; the frontend and CLI need no big-bang rewrite.
- No service reads or writes another service's database directly; cross-service references are resolved via UUIDs, necessary snapshots, and event synchronization.
- Every authorization decision is traceable: token validation, RBAC decisions, proxy policies, and project/group domain extraction all have audit or debug traces.
- Core flows — Job submit/cancel, IDE start/stop, Storage bind/fast-transfer, Image build/publish — all have saga state, compensation, and retry strategies.
- Every service has readiness/liveness, metrics, traces, structured logs, alerts, runbooks, and rollback.
- Before the legacy monolith is shut down, dual-write data comparison and event-lag monitoring pass continuously for at least one full operational cycle.
