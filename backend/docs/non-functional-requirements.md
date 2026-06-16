# Non-Functional Requirements (Platform-Wide)

All microservices must comply with the NFRs below; each service document only highlights the items that deserve special emphasis for that service.

## Security

| ID | Requirement |
| --- | --- |
| NFR-SEC-01 | All external APIs must pass an appropriate combination of API Key/JWT/User API Token/OIDC by default; internal service-to-service traffic must use mTLS or an equivalent service-mesh identity. |
| NFR-SEC-02 | Authorization logic must be centralized in the Authorization Policy Service or a verifiable shared SDK; services must not duplicate inconsistent RBAC logic. |
| NFR-SEC-03 | Secrets must never be committed to Git; manage them with Kubernetes Secret/External Secret; JWT/DB/MinIO/Harbor/LDAP/OIDC keys must be rotatable. |
| NFR-SEC-04 | JWT-only routes such as reverse proxies, WebSocket, FileBrowser, IDE, and image serving must still pass Proxy RBAC or domain RBAC. |
| NFR-SEC-05 | All administrative operations, permission changes, and important Job/Storage/Image state changes must produce audit events. |

## Availability

| ID | Requirement |
| --- | --- |
| NFR-AVAIL-01 | Core API services should target at least 99.9% availability; non-core proxy or reporting services may degrade independently and must not affect the main job-scheduling flow. |
| NFR-AVAIL-02 | Every service must provide liveness/readiness/startup probes, graceful shutdown, PDB, HPA, and resource requests/limits. |
| NFR-AVAIL-03 | Background tasks must support leader election or work sharding to avoid duplicate job dispatch or duplicate resource cleanup across replicas. |

## Performance

| ID | Requirement |
| --- | --- |
| NFR-PERF-01 | General read APIs should target p95 < 500ms; general write APIs p95 < 1s; operations against external systems (K8s/Harbor/Longhorn) must use async status queries to reduce foreground waiting. |
| NFR-PERF-02 | The synchronous phase of job submission should complete validation and enqueueing within 2 seconds; actual K8s creation, image pulling, and scheduling waits are tracked via a state machine/events. |
| NFR-PERF-03 | Read-heavy endpoints such as permissions, sessions, cluster summary, dashboard, and usage summary should use Redis/cache/read models. |

## Scalability

| ID | Requirement |
| --- | --- |
| NFR-SCALE-01 | All stateless API services must scale horizontally; long-lived WebSocket/proxy services must be independently scalable with configurable timeouts. |
| NFR-SCALE-02 | Background workers such as the job dispatcher, usage collector, audit worker, and notification worker must support horizontal sharding or mutual-exclusion locks. |

## Data Consistency

| ID | Requirement |
| --- | --- |
| NFR-DATA-01 | After microservice decomposition, cross-service database joins and cross-DB foreign keys are forbidden; cross-service references store only UUIDs and necessary snapshots. |
| NFR-DATA-02 | Cross-service flows must use Sagas, Outbox/Inbox, idempotency keys, and compensating actions — e.g., creating a Namespace/Quota after Project creation. |
| NFR-DATA-03 | Job quota reservation, queue dispatch, and preemption must use transactional reservation or arbitration by a single Scheduler service to avoid TOCTOU. |
| NFR-DATA-04 | Config blobs/versions must remain immutable; any overwrite must create a new version — old versions must never be modified. |

## Observability

| ID | Requirement |
| --- | --- |
| NFR-OBS-01 | All services must emit structured logs, Prometheus metrics, and OpenTelemetry traces, propagating trace_id/request_id/user_id/project_id. |
| NFR-OBS-02 | Cross-service dashboards must cover: API latency/error, job queue depth, dispatch latency, K8s API errors, storage transfers, image builds, and audit backlog. |

## Operability

| ID | Requirement |
| --- | --- |
| NFR-OPER-01 | Every service needs independent CI/CD, versioning, Dockerfile, Helm/Kustomize, DB migration pipeline, and rollback strategy. |
| NFR-OPER-02 | Configuration must be fully driven by environment variables/ConfigMap/Secret; production startup must never use dev default secrets. |
| NFR-OPER-03 | Database migrations must be backward-compatible: expand → dual-write/dual-read → backfill → cutover → contract. |

## Maintainability

| ID | Requirement |
| --- | --- |
| NFR-MAINT-01 | Service boundaries must follow bounded contexts and data ownership — never a one-to-one split by handler file or URL prefix. |
| NFR-MAINT-02 | Every service must provide an OpenAPI/gRPC contract, contract tests, integration tests, and at least 80% test coverage of critical paths. |

## Compatibility

| ID | Requirement |
| --- | --- |
| NFR-COMPAT-01 | In phase 1, the Gateway must preserve the current /api/v1 paths and response schema externally, so the frontend/CLI do not need a simultaneous rewrite. |
| NFR-COMPAT-02 | Internal microservice APIs may evolve, but the Gateway or adapters must preserve external compatibility and support versioning. |

## Resilience

| ID | Requirement |
| --- | --- |
| NFR-RES-01 | External system calls must have timeouts, retry with backoff, circuit breakers, and observable error codes. |
| NFR-RES-02 | When K8s/Harbor/MinIO/Longhorn are unavailable, the frontend must see a clear degraded state; unrelated domains must not be blocked. |

## Multi-Tenancy Isolation

| ID | Requirement |
| --- | --- |
| NFR-MTEN-01 | All data and K8s operations must be isolated by user/group/project namespace/domain; privilege escalation via namespace/path spoofing is forbidden. |
| NFR-MTEN-02 | Read models for usage, quota, storage permissions, and image allow-lists must be quickly invalidated or rebuilt after membership changes. |
