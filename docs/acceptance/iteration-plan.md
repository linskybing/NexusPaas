# Iteration Plan

Part of the [GA Acceptance docs](README.md).

The platform should be delivered through vertical slices.

Each milestone must produce runnable, testable, reviewable functionality.

## M0: Architecture and Safety Baseline

**Goal:** Stabilize core boundaries before adding more features.

**Scope:**
- Adopt the configured brand naming for external surfaces (default NexusPaaS).
- Keep NexusPaaS as upstream OSS name.
- Enforce 8 deployable-unit target.
- Add route collision detection.
- Centralize internal route service identity.
- Disable allow-all PDP outside local/test.
- Prevent production `SERVICE_NAME=all`.
- Define typed domain models for Project, Plan, Queue, Job, Image, Capability.
- Add transactional outbox/inbox foundation.

**Acceptance:**

| ID | Acceptance Criteria |
|---|---|
| M0-001 | Public UI/CLI docs use the configured brand names. |
| M0-002 | Route collision test passes. |
| M0-003 | Internal routes require centralized service identity. |
| M0-004 | Staging/production cannot start with allow-all PDP. |
| M0-005 | Staging/production cannot start with accidental all-in-one runtime. |
| M0-006 | Core domain model migration plan exists. |
| M0-007 | Outbox/inbox foundation has unit and integration tests. |

## M1: IAM, Group, Project, Personal Project, RBAC

**Goal:** Make user/project ownership correct before compute features.

**Scope:**
- User creation creates personal Project.
- Platform roles.
- Group roles.
- Project roles.
- Group Project inheritance.
- Project external invite.
- Admin protection.
- Route permission matrix.

**Acceptance:**

| ID | Acceptance Criteria |
|---|---|
| M1-001 | RBAC-001 to RBAC-020 pass. |
| M1-002 | API route permission matrix is generated and tested. |
| M1-003 | Platform admin cannot be removed to zero. |
| M1-004 | Project member isolation E2E passes. |

## M2: Safe ConfigFile Deployment MVP

**Goal:** Deploy safe Kubernetes YAML through NexusPaaS API.

**Scope:**
- ConfigFile immutable versions.
- YAML preflight validation.
- Resource allow list.
- Namespace isolation.
- Image allow-list check.
- k8s-control server-side dry-run and apply.
- Kyverno / ValidatingAdmissionPolicy enforcement.
- Audit events.

**Acceptance:**

| ID | Acceptance Criteria |
|---|---|
| M2-001 | K8S-001 to K8S-018 pass. |
| M2-002 | Direct Kubernetes bypass test is rejected by cluster admission. |
| M2-003 | ConfigFile replay by version works. |
| M2-004 | K8s resource cleanup is idempotent. |

## M3: Project Capability Gates

**Goal:** Control root, egress, hostPath, privileged, host namespaces, WebRTC,
and build permissions.

**Scope:**
- ProjectCapability model.
- Admin approval workflow.
- Capability expiry.
- Admission integration.
- Cluster policy integration.
- Audit.

**Acceptance:**

| ID | Acceptance Criteria |
|---|---|
| M3-001 | CAP-001 to CAP-015 pass. |
| M3-002 | Capability expiry test passes. |
| M3-003 | Platform admin approval audit evidence exists. |

## M4: Plan, Queue, Quota, Runtime, and Preemption

**Goal:** All deployments pass through Plan and Queue governance.

**Scope:**
- Plan model.
- Queue model.
- Project Plan binding.
- Runtime limit.
- Quota reserve/commit/release.
- Kueue integration.
- Preemption.
- Reconciliation.

**Acceptance:**

| ID | Acceptance Criteria |
|---|---|
| M4-001 | QUEUE-001 to QUEUE-022 pass. |
| M4-002 | Live E2E covers plan window, runtime expiry, and preemption. |
| M4-003 | Reservation drift reconciler passes failure tests. |

## M5: Image Build, Harbor, and Allow List

**Goal:** Allow controlled user image builds and enforce image allow list.

**Scope:**
- `nexus image build`.
- Rootless BuildKit through Tekton.
- Harbor push.
- Scan.
- SBOM.
- Signature/attestation if enabled.
- Digest allow list.
- Harbor deletion sync.

**Acceptance:**

| ID | Acceptance Criteria |
|---|---|
| M5-001 | IMG-001 to IMG-025 pass. |
| M5-002 | CLI build local context E2E passes. |
| M5-003 | Non-allow-listed image deployment is rejected. |
| M5-004 | Docker socket mount is impossible in build executor. |

## M6: GPU DRA, MPS, and Container/PID Usage Attribution

**Goal:** Support fractional GPU workloads and correctly attribute shared GPU
usage to users.

**Scope:**
- DRA ResourceClaimTemplate generation.
- MPS active thread percentage.
- Pinned memory limit.
- GPU reservation accounting.
- DCGM exporter deployment.
- process-exporter deployment.
- nexus-usage-agent DaemonSet.
- PID -> container -> pod -> job -> user mapping.
- MPS shared-card attribution.
- Usage rollups.

**Acceptance:**

| ID | Acceptance Criteria |
|---|---|
| M6-001 | GPU-001 to GPU-018 pass. |
| M6-002 | USAGE-001 to USAGE-037 pass. |
| M6-003 | Two users sharing one GPU through MPS are separately attributed. |
| M6-004 | process-exporter metrics are joined to container ownership. |
| M6-005 | GPU PID mapping to container ID is proven on a real GPU node. |
| M6-006 | UI clearly separates reserved, observed, estimated, and unavailable metrics. |

## M7: WebRTC GUI Workloads

**Goal:** Allow browser-operated GPU GUI workloads with controlled streaming and
egress.

**Scope:**
- Selkies or equivalent baseline image.
- Streaming ConfigFile template.
- coturn deployment.
- Short-lived TURN credentials.
- Gateway stream access.
- Stream admission.
- Stream usage attribution.
- Browser E2E.

**Acceptance:**

| ID | Acceptance Criteria |
|---|---|
| M7-001 | RTC-001 to RTC-018 pass. |
| M7-002 | Browser WebRTC test passes on GPU node. |
| M7-003 | Forced TURN relay test passes. |
| M7-004 | Stream usage appears in usage reports. |

## M8: Monitoring, Reporting, Operations, and GA Evidence

**Goal:** Move from functional platform to production-ready platform.

**Scope:**
- Real-time dashboards.
- Historical usage APIs.
- Group usage reports.
- Alerts.
- Backup/restore.
- GitOps.
- Load tests.
- Failure injection.
- Rollback drills.
- Security review.

**Acceptance:**

| ID | Acceptance Criteria |
|---|---|
| M8-001 | MON-001 to MON-020 pass. |
| M8-002 | OPS-001 to OPS-020 pass. |
| M8-003 | PERF-001 to PERF-008 pass. |
| M8-004 | Security acceptance SEC-001 to SEC-020 pass. |
| M8-005 | Full GA release checklist passes. |
