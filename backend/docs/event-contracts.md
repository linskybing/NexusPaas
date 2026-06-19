# Core Event Contracts

Cross-service state synchronization uses an event bus. All events are published via the Outbox/Inbox pattern (at-least-once delivery + idempotency keys). Event payloads carry only UUIDs and necessary snapshots; internal primary keys that would enable cross-service joins are forbidden.

## Event Catalog

| Event | Publisher | Main Subscribers | Purpose |
| --- | --- | --- | --- |
| UserCreated / UserUpdated / UserDisabled | identity-service | authorization-policy, org-project, audit-compliance, request-notification | Sync display names, role/status caches, and audit. |
| GroupCreated / GroupMembershipChanged | org-project-service | authorization-policy, image-registry, storage, workload, usage-observability | Update membership read models, image access, storage permissions. |
| ProjectCreated / ProjectUpdated / ProjectDeleted | org-project-service | k8s-control, scheduler-quota, storage, image-registry, usage-observability, audit-compliance | Create namespaces/quotas/read models; deletion runs as a saga. |
| PolicyChanged / ProxyPolicyChanged | authorization-policy-service | platform-gateway, integration-proxy, k8s-control, audit-compliance | Invalidate RBAC caches, sync ConfigMaps. |
| ConfigCommitted | workload-service | audit-compliance, request-notification | Preserve immutable config versions. |
| JobSubmitted / JobQueued / JobRunning / JobSucceeded / JobFailed / JobCancelled / JobPreempted | workload-service / scheduler-quota-service | usage-observability, audit-compliance, request-notification, k8s-control, platform-gateway | Status push, usage accounting, resource release. |
| PlanChanged | scheduler-quota-service | authorization-policy, workload, usage-observability | Sync runtime quota/plan read models such as policy-data ConfigMaps and dispatch views. |
| QuotaReserved / QuotaCommitted / QuotaReleased / SubmitAdmissionReviewed / QueueDepthChanged / PriorityClassSyncCompleted | scheduler-quota-service | workload, usage-observability, audit-compliance | Support dashboards, dispatch, quota state, admission evidence, and Kubernetes priority-class sync evidence. |
| ResourceSnapshotRecorded / NamespaceCreated / NamespaceDeleted | k8s-control-service | workload, scheduler-quota, usage-observability, audit-compliance | Publish cluster resource and namespace lifecycle snapshots. |
| IDEStarted / IDEStopped / IDEDeleted / IDEIdleReaped | ide-service | audit-compliance, request-notification, usage-observability | Track workspace lifecycle and idle cleanup outcomes. |
| PVCProvisioned / StorageBound / StoragePermissionChanged / FastTransferCompleted / LonghornRWXHealthChecked | storage-service | workload, k8s-control, audit-compliance, request-notification, usage-observability | Update mountable volumes, notify users, and publish storage health evidence. |
| ImageRequested / ImageApproved / ImageBuildStarted / ImageBuilt / ImagePublished / ImageSyncFailed | image-registry-service | workload, audit-compliance, request-notification, org-project | Update allow lists and build status. |
| UsageSnapshotRecorded / ResourceHoursSummarized | usage-observability-service | audit-compliance, dashboard/read-model consumers | Publish usage snapshots and accounting summaries. |
| AuditEvent | all services | audit-compliance-service | Published via outbox, at-least-once delivery. |
| FormCreated / FormUpdated / NotificationRequested / AnnouncementPublished | request-notification-service / any service | request-notification-service / platform-gateway / audit-compliance | In-app request workflows, notifications, announcements, and unread counts. |
| ProxySessionStarted / ProxySessionTerminated | integration-proxy-service | audit-compliance, usage-observability | Record external tool proxy session lifecycle. |
| MediaUploaded / MediaDeleted | media-upload-service | audit-compliance, request-notification | Track uploaded media object lifecycle. |

## Versioned Fixtures

Canonical v1 event envelope fixtures live under `backend/internal/contracts/fixtures/events/v1/` and are validated by `backend/internal/contracts` tests. The initial fixture set covers the first GA contract slice:

| Event | Fixture | Producer | Representative Boundary |
| --- | --- | --- | --- |
| UserUpdated | `user-updated.json` | `identity-service` | IAM identity snapshot for downstream read models |
| ProjectUpdated | `project-updated.json` | `org-project-service` | Tenant/project ownership and quota-plan snapshot |
| JobSubmitted | `job-submitted.json` | `workload-service` | Compute API admission request snapshot |
| QuotaReserved | `quota-reserved.json` | `scheduler-quota-service` | Scheduler/quota reservation state |
| AuditEvent | `audit-event.json` | `scheduler-quota-service` | Mandatory audit trail event |

## Design Constraints

- V1 fixtures must carry `event_id` (UUID), `schema_version`, `event_type`, `producer`, `occurred_at`, `trace_id`, `aggregate_id`, and `payload`.
- `request_id` is optional; consumers must keep `trace_id` as the required correlation key and tolerate missing `request_id`.
- Subscribers must process idempotently (Inbox deduplication).
- Event schema evolution is additive-only; consumers must tolerate unknown top-level fields and additive payload fields. Breaking changes require a new versioned topic or envelope version.
- Payloads must carry UUIDs and safe snapshots, not internal database row IDs, raw primary keys, secrets, tokens, cookies, credentials, or private tenant data.
- AuditEvent is mandatory: all administrative operations, permission changes, and important Job/Storage/Image state changes must publish one (NFR-SEC-05).
