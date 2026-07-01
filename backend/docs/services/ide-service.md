# ide-service

Category: Compute | Phase: 5 (may initially co-deploy with workload-service)

## 1. Overview

The interactive IDE workspace service. Responsible for the lifecycle of Jupyter/VS Code/TensorBoard workspaces (start, stop, delete), the IDE proxy, activity tracking, idle reaping, and the IDE image list. IDE startup is governed by project membership, quota, image, PVC mount, priority, and scheduler controls.

## 2. Functional Requirements

| ID | Requirement | Acceptance/Notes |
| --- | --- | --- |
| FR-IDE-01 | Support listing IDEs, listing available IDE images, and starting, stopping, and deleting IDE workspaces. | /api/v1/ide. |
| FR-IDE-02 | IDE startup is governed by project membership, quota, image, PVC mount, priority, and scheduler controls. | IDE runs through the executor and PVCMountPlanner. |
| FR-IDE-03 | The IDE proxy uses JWT-only + Proxy RBAC, supporting browsers opening Jupyter/VS Code directly. | /api/v1/ide/proxy/{podName}/*. |
| FR-IDE-04 | Track IDE activity, supporting idle reaping and resource reclamation. | Derived from IDE activity and cron docs. |

## 3. Owned Data

`ide_sessions`, workspace activity, pod mapping; may reference jobs or use an independent workspace table.

## 4. Current Code/Route Mapping

- Handlers: `ide.go`, `handlers/executor/ide_*`
- Application: `application/ide`
- Routes: `/api/v1/ide/*`, `/api/v1/ide/proxy/{podName}/*`

## 5. Dependencies

| Dependency | Purpose |
| --- | --- |
| org-project-service | Project membership checks (event snapshots + synchronous query for critical operations) |
| scheduler-quota-service | Quota Reserve/Commit/Release |
| image-registry-service | IDE image allow-list |
| storage-service | PVC mount resolution |
| k8s-control-service | Workspace pod creation/deletion |
| authorization-policy-service | Proxy RBAC decisions |

## 6. Events

| Direction | Event | Purpose |
| --- | --- | --- |
| Publish | IDE lifecycle events (analogous to Job lifecycle events) | Usage accounting, audit, notification |
| Subscribe | StoragePermissionChanged, GroupMembershipChanged | Invalidate mount/membership snapshots |

## 7. Non-Functional Highlights

- The IDE proxy is a JWT-only long-lived connection, needing independent scaling and timeout settings (NFR-SCALE-01, NFR-SEC-04).
- The idle reaper needs leader election to avoid duplicate reclamation (NFR-AVAIL-03).
- IDE start/stop must have saga state, compensation, and retry strategies (acceptance criterion).

## 8. Decomposition Notes

May initially co-deploy with workload-service (sharing the executor and saga infrastructure), then split off later because long-lived connection/proxy scaling characteristics differ.
