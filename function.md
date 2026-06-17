# NexusPaas Current Backend Function Inventory

Generated: 2026-06-17

This inventory maps the current NexusPaas backend capabilities to the 15 target
Production Beta services. It is a launch-readiness and planning artifact, not a
runtime contract change.

Reference parity is unverified: `references/CSCC_AI_Platform_Backend` is absent
from the current worktree. This file closes the missing current-backend
capability inventory gap only. It does not prove that the current backend has
full parity with the unavailable reference snapshot.

## Evidence Base

- `backend/README.md`
- `backend/docs/api-route-mapping.md`
- `backend/docs/event-contracts.md`
- `backend/docs/non-functional-requirements.md`
- `backend/docs/operational-readiness.md`
- `backend/internal/services/catalog.go`
- `backend/*-service/README.md`
- `backend/platform-gateway/README.md`

## Service Catalog

| Service | Launch Role | Primary Ownership |
| --- | --- | --- |
| platform-gateway | Edge entry | External `/api/v1` compatibility, request routing, auth entry, OpenAPI, service registry |
| identity-service | Core IAM | Login, sessions, refresh tokens, user API tokens, users, roles, OIDC provider |
| authorization-policy-service | Core IAM | PDP, Casbin/domain RBAC, proxy RBAC, policy bundles, policy data sync |
| org-project-service | Tenant platform | Groups, user groups, projects, members, workspace settings, GPU claims, tenant quota metadata |
| workload-service | Compute API | Config files, job submit/list/detail/cancel, job state machine, job logs, dispatcher handoff |
| scheduler-quota-service | Compute control | Plans, queues, quota reserve/commit/release, priority, preemption, quota reconciliation |
| k8s-control-service | Infrastructure control | Kubernetes resource adapter, namespace/resource commands, pod logs/events, WebSockets |
| ide-service | Compute API | IDE session lifecycle, IDE image list, proxy, activity tracking, idle reaping |
| storage-service | Platform IO | User/group/project storage, PVC/FileBrowser lifecycle, storage permissions, fast transfer |
| image-registry-service | Supply chain | Image requests, allow lists, image builds, catalog sync/publish, Harbor API governance |
| usage-observability-service | Ops read model | Usage, dashboard, cluster summaries, GPU snapshots, resource hours |
| audit-compliance-service | Ops compliance | Audit ingestion/search, project reports, security posture, retention cleanup |
| request-notification-service | Collaboration | Forms, form messages, notifications, announcements, unread/read state |
| integration-proxy-service | Edge integrations | Grafana/MinIO/pgAdmin/Longhorn/Harbor UI proxies, auth-check, VPN administration |
| media-upload-service | Collaboration support | Image upload, object bucket abstraction, image serving, media metadata |

## Capability Inventory

| ID | Domain | Function | Target Microservice | Current Routes / Jobs / Events | Owned Data | Dependencies | Notes |
| --- | --- | --- | --- | --- | --- | --- | --- |
| CAP-GW-01 | Gateway | External API compatibility, route dispatch, service registry, health, readiness, metrics, OpenAPI | platform-gateway | `/healthz`, `/readyz`, `/metrics`, `/openapi.json`, `/openapi.yaml`, `/service-registry`, external `/api/v1` routes | Route config, JWKS/policy caches, rate-limit state | identity-service, authorization-policy-service, all downstream services | Single external entry point; injects request and trace context; no core business data ownership |
| CAP-GW-02 | Gateway | JWT-only browser/proxy/WebSocket entry for paths that cannot send custom API key headers | platform-gateway | Proxy, image, IDE, FileBrowser, and WebSocket entry points | Proxy/session cache only | identity-service, authorization-policy-service, integration-proxy-service, ide-service, storage-service, media-upload-service | Must preserve API key, JWT bearer, cookie, and user API token distinctions |
| CAP-ID-01 | Identity | Login, logout, registration, CAPTCHA, refresh token, session lifecycle | identity-service | `/api/v1/login`, `/logout`, `/register`, `/refresh`, `/captcha`; publishes `UserCreated`, `UserUpdated`, `UserDisabled` | `users`, `sessions`, `refresh_tokens`, roles and credential audit snapshots | Redis, PostgreSQL, audit-compliance-service, authorization-policy-service | Account-management operations are audit-relevant |
| CAP-ID-02 | Identity | User API tokens, CLI login, CLI CA, personal token lifecycle | identity-service | `/api/v1/me/api-tokens`, `/api/v1/cli/login`, `/api/v1/me/cli-ca` | `user_api_tokens`, token hash/prefix metadata | audit-compliance-service, authorization-policy-service | User API tokens must stay revocable and never log raw token values |
| CAP-ID-03 | Identity | User, role, status, settings, local credential, LDAP strategy, OIDC provider | identity-service | `/api/v1/users`, `/api/v1/users/{id}/settings`, `/api/v1/oidc/*`, JWKS/discovery/token/userinfo/revoke flows | `users`, `roles`, credential metadata | LDAP/OIDC config, authorization-policy-service, integration-proxy-service | OIDC supports external tool SSO; LDAP mirror sync remains a parity item to verify against the missing reference snapshot |
| CAP-AUTHZ-01 | Authorization | Domain RBAC, Casbin policy management, simulation, default policy repair | authorization-policy-service | `/api/v1/permissions/*`; publishes `PolicyChanged` | `casbin_rule`, policies, policy rules, policy assignments | identity-service, org-project-service, audit-compliance-service | Centralized PDP/SDK avoids RBAC rule drift |
| CAP-AUTHZ-02 | Authorization | Proxy RBAC, platform proxy roles, service definitions, signed policy bundles | authorization-policy-service | `/api/v1/admin/proxy-rbac/*`; publishes `ProxyPolicyChanged`; policy data sync worker | Proxy policies, platform roles, service definitions, policy bundle metadata | platform-gateway, integration-proxy-service, k8s-control-service | Proxy policy changes must be audited and invalidate caches |
| CAP-ORG-01 | Tenancy | Group and user-group lifecycle, membership lookup, group policy options | org-project-service | `/api/v1/groups`, `/api/v1/user-groups`, `/api/v1/admin/group-policy-options`; publishes `GroupCreated`, `GroupMembershipChanged` | Groups, user groups, membership snapshots | identity-service, authorization-policy-service, downstream read-model consumers | Membership changes drive storage/image/usage/workload read models |
| CAP-ORG-02 | Tenancy | Project tree, project members, project quotas, workspace settings, GPU claims | org-project-service | `/api/v1/projects`, `/api/v1/projects/{id}/members`, `/workspace-settings`, `/gpu-claims`; publishes `ProjectCreated`, `ProjectUpdated`, `ProjectDeleted` | Projects, project members, user quotas, workspace settings, GPU claims | k8s-control-service, scheduler-quota-service, storage-service, image-registry-service, usage-observability-service | Project deletion is a saga; cross-service writes go through APIs/events |
| CAP-ORG-03 | Tenancy | Internal owner-read contracts for projects, members, quotas, user groups, plan binding | org-project-service | `/internal/org-project/projects`, `/internal/org-project/project-members`, `/internal/org-project/user-quotas`, `/internal/org-project/user-groups`, plan binding/clear contracts | Same org-project owned records | scheduler-quota-service, workload-service, request-notification-service | Service-to-service access requires scoped service key in Beta |
| CAP-WORK-01 | Workload | ConfigFile CRUD, immutable content/versioning, project file tree, runtime instance creation | workload-service | `/api/v1/configfiles`, `/configfiles/{id}/instance`; publishes `ConfigCommitted` | `config_files`, config blobs, commits/versions | storage-service, k8s-control-service, audit-compliance-service | Config versions must remain immutable for repeatable job reruns |
| CAP-WORK-02 | Workload | Job templates, submit/list/detail/cancel, state machine, logs metadata | workload-service | `/api/v1/jobs`; publishes `JobSubmitted`, `JobQueued`, `JobRunning`, `JobSucceeded`, `JobFailed`, `JobCancelled` | Jobs, job logs, job templates | scheduler-quota-service, image-registry-service, storage-service, k8s-control-service, usage-observability-service | Submit path is a saga with quota reserve, image/storage resolution, K8s create, commit/release compensation |
| CAP-WORK-03 | Workload | Job GPU usage read views and status aggregation | workload-service | `/api/v1/jobs/{id}/gpu-*` style read models; subscribes to usage snapshots | Job identity/status; usage data is read-model owned elsewhere | usage-observability-service | Workload aggregates usage data but does not own GPU usage snapshots |
| CAP-SCHED-01 | Scheduling | Plans, queues, project-to-plan, plan-to-queue, priority classes | scheduler-quota-service | `/api/v1/plans`, `/api/v1/queues`; publishes `PlanChanged`, `PriorityClassSyncCompleted` | Plans, queues, priority classes, project plan bindings/read models | org-project-service, authorization-policy-service, workload-service | Queue and plan state are the quota arbiter's data boundary |
| CAP-SCHED-02 | Scheduling | Quota reserve/commit/release, admission review, preemption, queue depth | scheduler-quota-service | Internal quota/preemption APIs; events `QuotaReserved`, `QuotaCommitted`, `QuotaReleased`, `SubmitAdmissionReviewed`, `QueueDepthChanged`, `JobPreempted` | Reservations, resource quotas, preemption records, GPU claim snapshots | workload-service, org-project-service owner reads, k8s-control-service | Scheduler-quota no longer declares org/workload shared-store dependencies in isolated mode |
| CAP-SCHED-03 | Scheduling | Resource quota reconciler, queue metrics collector, plan window reaper | scheduler-quota-service | Maintenance workers: resource quota reconciler, queue metrics collector, plan window reaper, priority class sync | Reconciliation evidence, quota state, queue metrics | k8s-control-service, workload-service, org-project-service | Negative quota drift, stalled queues, and owner-read failure are alert-worthy |
| CAP-K8S-01 | Kubernetes control | Cluster summary, resource snapshots, namespace and workload resource reads | k8s-control-service | `/api/v1/k8s/*`, `/api/v1/resources/*`, `/api/v1/projects/{id}/resources`, `/api/v1/cluster/*` shared presentation | K8s operation records, namespace mapping, pod/resource snapshots | Kubernetes API, usage-observability-service | Centralizes Kubernetes API access; no other service should call K8s directly after extraction |
| CAP-K8S-02 | Kubernetes control | Pod logs/events, delete commands, project cleanup, WebSocket exec/watch/log streams | k8s-control-service | `/api/v1/ws/*`, pod logs/events, cleanup routes; publishes `ResourceSnapshotRecorded`, `NamespaceCreated`, `NamespaceDeleted` | Command records and snapshots | workload-service, scheduler-quota-service, audit-compliance-service | WebSockets need JWT/cookie path support and trace propagation |
| CAP-K8S-03 | Kubernetes control | Cluster resource collector and workload runtime reaper support | k8s-control-service | Background cluster resource collector; workload runtime reaper support through cluster cleanup primitives | Resource snapshots, cleanup evidence | workload-service, scheduler-quota-service, usage-observability-service | Collector/reaper behavior must be idempotent in degraded cluster mode |
| CAP-IDE-01 | IDE | IDE session list, image list, start, stop, delete, activity tracking | ide-service | `/api/v1/ide/*`; publishes `IDEStarted`, `IDEStopped`, `IDEDeleted`, `IDEIdleReaped` | `ide_sessions`, workspace activity, pod mapping | workload-service, scheduler-quota-service, image-registry-service, storage-service, k8s-control-service | IDE start is governed by project, quota, image, PVC, and scheduler controls |
| CAP-IDE-02 | IDE | IDE proxy and idle reaper | ide-service | `/api/v1/ide/proxy/{podName}/*`; idle reaper worker | Session activity and proxy state | platform-gateway, authorization-policy-service, k8s-control-service | Long-lived proxy paths are JWT-only and must re-check authorization |
| CAP-STOR-01 | Storage | User/group storage, PVC lifecycle, storage options, FileBrowser start/stop/proxy | storage-service | `/api/v1/storage/*`, `/api/v1/admin/user-storage/*`, `/api/v1/admin/group-storage` | Storage/PVC records, FileBrowser state, access policies | k8s-control-service, authorization-policy-service, org-project-service | FileBrowser proxy is JWT-only and must revalidate permissions |
| CAP-STOR-02 | Storage | Project storage binding, project permissions, fast-stage transfer | storage-service | `/api/v1/projects/{id}/storage/*`, transfer query/cancel routes; publishes `StorageBound`, `StoragePermissionChanged`, `FastTransferCompleted` | Project storage bindings, permission records, fast transfer records | org-project-service, workload-service, audit-compliance-service, request-notification-service | Bind/transfer operations are saga candidates and audit-relevant |
| CAP-STOR-03 | Storage | Longhorn RWX health reconciler and storage class validation | storage-service | Longhorn RWX health reconciler, storage validation helpers; publishes `LonghornRWXHealthChecked` | Storage health evidence and validation state | Longhorn/NFS, k8s-control-service | Storage dependency degradation must be visible without blocking unrelated domains |
| CAP-IMG-01 | Image | Project image allow lists and image requests/review | image-registry-service | `/api/v1/projects/{id}/images`, `/api/v1/image-requests`; events `ImageRequested`, `ImageApproved` | Image allow lists, image requests | org-project-service, authorization-policy-service, audit-compliance-service | Request/review and allow-list changes are audit-relevant |
| CAP-IMG-02 | Image | Image builds from archive, storage, Dockerfile, logs, status | image-registry-service | `/api/v1/images/build/*`, build logs/status; publishes `ImageBuildStarted`, `ImageBuilt`, `ImageSyncFailed` | Image build jobs and artifact metadata | Harbor, storage-service, k8s-control-service, request-notification-service | Long-running build workflow should remain async and resumable |
| CAP-IMG-03 | Image | Image catalog sync, publish, unpublish, delete, Harbor API governance, Harbor health | image-registry-service | `/api/v1/image-catalog`, `/api/v1/harbor-status`; Harbor health checks; publishes `ImagePublished` | Repositories, tags, sync targets, catalog state | Harbor primary/GPU23 lanes, authorization-policy-service | Harbor UI proxy belongs to integration-proxy-service; Harbor API governance stays here |
| CAP-USAGE-01 | Usage | User/admin usage, request usage, GPU user summaries, job GPU views | usage-observability-service | `/api/v1/me/usage`, `/me/gpu/jobs`, `/me/request-usage`, `/admin/usage`, `/admin/request-usage`, `/admin/gpu/users` | Usage snapshots, request usage, job GPU read models | workload-service, scheduler-quota-service, Prometheus | Read-model lag is acceptable only within Beta thresholds |
| CAP-USAGE-02 | Usage | Dashboard overview/admin summary, cluster summary presentation | usage-observability-service | `/api/v1/dashboard/*`, `/api/v1/admin/dashboard-summary`, cluster read models | Dashboard/read-model records | k8s-control-service, Prometheus, event bus | Non-core reporting degradation must not break job submission |
| CAP-USAGE-03 | Usage | GPU usage collector, resource hours collector, resource-hour cleanup | usage-observability-service | GPU usage collector, resource hours collector, `ResourceSnapshotRecorded`, `UsageSnapshotRecorded`, `ResourceHoursSummarized` | GPU snapshots, pod resource records, resource hour summaries | k8s-control-service, workload-service, scheduler-quota-service | Collector lag and Prometheus query failure need alerts |
| CAP-AUDIT-01 | Audit | AuditEvent ingestion, audit search/logs, project audit reports | audit-compliance-service | `/api/v1/audit/*`; subscribes to `AuditEvent` from all services | `audit_logs`, ingestion offsets, compliance reports | All services, event bus | Admin, permission, tenant, job, storage, image, and integration changes must emit audit evidence |
| CAP-AUDIT-02 | Audit | Security posture and audit cleanup | audit-compliance-service | `/api/v1/admin/security/posture`; audit cleanup worker | Security findings/reports, retention state | authorization-policy-service, identity-service, storage backend | Retention is configuration-driven; dropped audit events are launch blockers |
| CAP-COMM-01 | Collaboration | Forms/requests, form messages, status updates | request-notification-service | `/api/v1/forms`, `/api/v1/forms/{id}/messages`, batch status routes | Forms and form messages | org-project-service, identity-service, media-upload-service, audit-compliance-service | Forms can carry project/media references but do not own raw media |
| CAP-COMM-02 | Collaboration | Notifications, announcements, unread counts, mark-read state | request-notification-service | `/api/v1/notifications`, `/api/v1/announcements`, `/api/v1/admin/announcements`; events `NotificationRequested`, `AnnouncementPublished` | Notifications, announcements, announcement reads | identity-service, platform-gateway, audit-compliance-service | Notification workers must be idempotent and replay-safe |
| CAP-PROXY-01 | Integrations | Grafana, MinIO Console, pgAdmin, Longhorn, Harbor UI proxy and SSO/auth-check adapters | integration-proxy-service | `/api/v1/grafana/*`, `/minio-console/*`, `/pgadmin/*`, `/longhorn/*`, `/harbor/*`, SSO/auth-check routes; events `ProxySessionStarted`, `ProxySessionTerminated` | Proxy session/cache state only | platform-gateway, identity-service, authorization-policy-service, external tools | Owns no core policy; proxy authorization comes from authz service |
| CAP-PROXY-02 | Integrations | VPN administration and VPN usage collector | integration-proxy-service | `/api/v1/admin/vpn`; VPN usage collector | VPN session/usage snapshots | VPN backend, audit-compliance-service, usage-observability-service | External adapter degradation should not affect core APIs |
| CAP-MEDIA-01 | Media | Image upload, object store bucket abstraction, checksum, metadata | media-upload-service | `POST /api/v1/uploads/images`; publishes `MediaUploaded` | Uploaded media metadata, object keys, checksums | MinIO/S3-compatible object store, request-notification-service, audit-compliance-service | Upload payloads and raw object data must not be logged |
| CAP-MEDIA-02 | Media | JWT-only image serving and media deletion lifecycle | media-upload-service | `GET /api/v1/uploads/images/{key...}`; publishes `MediaDeleted` when deletion is implemented | Media metadata and owner references | platform-gateway, identity-service, object store | Expected 404 for nonexistent media is acceptable smoke behavior |

## Non-HTTP And Background Coverage

| Worker / Process | Owning Service | Evidence / Trigger | Launch Note |
| --- | --- | --- | --- |
| audit cleanup | audit-compliance-service | Retention cleanup worker and audit-compliance README | Must be configuration-driven and idempotent |
| Harbor health | image-registry-service | Harbor health checks and `/api/v1/harbor-status` | Dependency failures must surface as degraded state |
| LDAP mirror | identity-service | LDAP strategy documented; mirror sync is not verified without reference snapshot | Keep as reference parity follow-up until snapshot is restored |
| cluster resource collector | k8s-control-service | Cluster collector publishes resource snapshots | Must tolerate nil/degraded Kubernetes client |
| GPU usage collector | usage-observability-service | GPU usage snapshots and Prometheus query read models | Collector lag is operationally significant |
| resource hours collector | usage-observability-service | Resource hours summary worker | Feeds usage and billing-style dashboards |
| resource quota reconciler | scheduler-quota-service | Reconciles plan/quota state to Kubernetes limits | Run before or after scheduler rollback when quota drift is suspected |
| priority class sync | scheduler-quota-service | Syncs scheduler priority classes and emits evidence event | Needed for queue/priority correctness |
| idle reaper | ide-service | IDE idle reaper and workload idle cleanup behavior | Must avoid duplicate cleanup in multi-replica mode |
| plan window reaper | scheduler-quota-service | Plan window reaper | Must release or close expired plan windows predictably |
| workload runtime reaper | workload-service | Runtime-limited resource cleanup support | Applies runtime expiry to active workload resources |
| policy data sync | authorization-policy-service | Syncs policy data and signed policy bundles | Keeps gateway/proxy/admission policy fresh |
| Longhorn RWX | storage-service | Longhorn RWX health reconciler and volume-share helpers | RWX dependency degradation must not hide storage risk |
| VPN usage collector | integration-proxy-service | VPN usage collector | Feeds usage/audit views for external access |
| queue metrics collector | scheduler-quota-service | Queue depth and scheduler metrics | Alert on stuck queue or dispatch lag |
| job dispatcher | workload-service | Job dispatcher handoff to scheduler/K8s control | Must use idempotency keys and quota compensation |

## Cross-Service Contract Notes

- Gateway authentication is not the only security boundary. Internal owner-read
  and command contracts must require scoped service-to-service credentials for
  Production Beta.
- Cross-service writes must use API commands or events. Shared database reads
  remain transition debt unless registered as explicit owner-read contracts.
- Every event should carry `event_id`, `occurred_at`, `trace_id`, and
  `schema_version`, and subscribers must be idempotent.
- AuditEvent is mandatory for administrative operations, permission changes,
  tenant-changing operations, and important job, quota, storage, image, media,
  and integration lifecycle events.
- All services must emit logs, metrics, and traces with service, environment,
  version, request_id, trace_id, route/operation, status, latency, and safe
  tenant/user context where allowed.

## Remaining Gaps

- Reference parity remains unverified because the reference snapshot is absent.
- Live staging deploy/readiness/smoke/rollback/re-deploy evidence is still
  required before external Production Beta traffic.
- Live Grafana dashboards, PrometheusRule alerts, and scheduled synthetic
  monitors still need provisioning evidence.
- Some packages remain below the per-package 80 percent coverage target even
  though the aggregate integration gate is above 80 percent.
- Shared physical PostgreSQL remains a transition debt until every service has
  independent storage or formally documented owner-read/read-model contracts.
