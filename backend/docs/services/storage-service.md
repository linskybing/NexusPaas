# storage-service

Category: Data | Phase: 2

## 1. Overview

The storage governance center. Responsible for User/Group storage, PVC lifecycle, storage options, FileBrowser, storage permissions, project bindings, fast-stage transfers, and Longhorn/NFS policy validation. Interacts closely with k8s-control, org-project, and authorization-policy — a good candidate for early extraction.

## 2. Functional Requirements

| ID | Requirement | Acceptance/Notes |
| --- | --- | --- |
| FR-STORAGE-01 | Support personal user storage status check, initialization, expansion, deletion, open browsing, stop browsing, and proxy. | /admin/user-storage and /k8s/user-storage. |
| FR-STORAGE-02 | Support group storage option queries, group storage lists, my-group storage lists, and group PVC creation/deletion. | Creation/deletion is a platform-admin permission. |
| FR-STORAGE-03 | Support FileBrowser start/stop/proxy, re-validating user, group, and policy permissions at proxy time. | The proxy uses JWT-only. |
| FR-STORAGE-04 | Support group PVC permission setting, batch set, batch revoke, effective personal permission queries, listings, and access policies. | /storage/permissions. |
| FR-STORAGE-05 | Support binding group PVCs to projects, unbinding, and listing project-bound storage. | /projects/{id}/storage/bindings. |
| FR-STORAGE-06 | Support project-level permissions on project-bound storage, including query, set, revoke, and batch operations. | Fine-grained project storage permissions. |
| FR-STORAGE-07 | Support fast-stage transfer, quickly moving shared group PVC data to a project-local fast cache PVC, with transfer query and cancellation. | /projects/{id}/storage/transfers. |
| FR-STORAGE-08 | Validate storage class, Longhorn RWX/NFS mount options, fast-data node selector, checksum mode, and image pinning. | Derived from storage config validation. |

## 3. Owned Data

storages/PVCs, `group_storage_permissions`, `access_policies`, `project_storage_bindings`, fast_transfer records.

## 4. Current Code/Route Mapping

- Handlers: `storage.go`, `project_storage.go`, `admin_storage.go`
- Application: `application/storage`
- Domain: `domain/storage`
- Routes: `/api/v1/storage/*`, `/api/v1/projects/{id}/storage/*`, `/api/v1/admin/user-storage/*`, `/api/v1/admin/group-storage`

## 5. Dependencies

| Dependency | Purpose |
| --- | --- |
| k8s-control-service | Actual PVC/FileBrowser pod creation and deletion (async command + status) |
| org-project-service | Group/project membership snapshots and binding validation |
| authorization-policy-service | RBAC decisions for FileBrowser proxy and storage permissions |

## 6. Events

| Direction | Event | Counterpart | Purpose |
| --- | --- | --- | --- |
| Publish | PVCProvisioned / StorageBound / StoragePermissionChanged / FastTransferCompleted | workload, k8s-control, audit, notification | Update mountable volumes and notify users |
| Subscribe | GroupMembershipChanged / ProjectDeleted | org-project | Invalidate permission read models, clean up bindings |

## 7. Non-Functional Highlights

- The FileBrowser proxy is JWT-only and must re-validate permissions at proxy time (NFR-SEC-04).
- PVC operations use async command + status so a slow Longhorn never drags down the foreground (NFR-PERF-01, NFR-RES-02).
- Storage bind/fast-transfer must have saga state, compensation, and retry strategies (acceptance criterion).
- Storage permission read models must be quickly invalidated or rebuilt after membership changes (NFR-MTEN-02).

## 8. Decomposition Notes

Extracted in phase 2. Fast-stage transfer is a long-running operation needing progress query and cancellation; its state machine is persisted in this service.
