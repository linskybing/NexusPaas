# org-project-service

Category: Core | Phase: 4

## 1. Overview

The organization and multi-tenancy governance center. Responsible for Groups, UserGroups, the hierarchical Project tree (ltree), Project members, workspace settings, project/user quota metadata, and resource owner references. Other services store only project_id/group_id/user_id snapshots and synchronize membership read models via events.

## 2. Functional Requirements

| ID | Requirement | Acceptance/Notes |
| --- | --- | --- |
| FR-ORG-01 | Support group CRUD, batch delete, and group governance policy management. | Group create/update/delete is primarily a platform-admin permission. |
| FR-ORG-02 | Support adding/removing users to/from groups, batch joins, in-group role updates, and membership queries by group or by user. | /api/v1/user-groups. |
| FR-ORG-03 | Support candidate lookup and resolution before adding group members, preventing duplicate or unauthorized joins. | add-members-context and resolve-add-members. |
| FR-ORG-04 | Support project CRUD, batch delete, list projects by user, project detail queries, GPU usage, and live quota. | /api/v1/projects. |
| FR-ORG-05 | Support a hierarchical project tree, representing organization/project relations with global UUIDs and paths. | README and architecture docs describe ltree. |
| FR-ORG-06 | Support project member listing, add, remove, batch removal, batch role updates, and individual role updates. | Managed by project admins. |
| FR-ORG-07 | Support per-member project quota query, create/update, and delete. | /projects/{id}/members/{userId}/quota. |
| FR-ORG-08 | Support workspace runtime cap or workspace settings adjustments, managed by project managers/admins. | /projects/{id}/workspace-settings. |
| FR-ORG-09 | Support Project GPU claim listing, creation, and deletion for project-level management of Kubernetes DRA/MPS/GPU resources. | /projects/{id}/gpu-claims. |

## 3. Owned Data

`resource_owners`, `groups`, `user_groups`, `projects`, `project_members`, `user_quotas`, project workspace settings.

## 4. Current Code/Route Mapping

- Handlers: `group.go`, `user_group.go`, `project.go`
- Domain: `domain/group`, `domain/project`
- Routes: `/api/v1/groups`, `/api/v1/user-groups`, `/api/v1/projects`, `/api/v1/projects/{id}/members`, `/projects/{id}/workspace-settings`, `/projects/{id}/gpu-claims`

## 5. Published Events

| Event | Subscribers | Purpose |
| --- | --- | --- |
| GroupCreated / GroupMembershipChanged | authz, image-registry, storage, workload, usage | Update membership read models, image access, storage permissions |
| ProjectCreated / ProjectUpdated / ProjectDeleted | k8s-control, scheduler-quota, storage, image-registry, usage, audit | Create namespaces/quotas/read models; deletion runs as a saga |

## 6. Cross-Service Flows (Sagas)

- **Project creation**: create project record → publish ProjectCreated → k8s-control creates the namespace → scheduler-quota creates the quota → any failed step triggers compensation (NFR-DATA-02).
- **Project deletion**: reverse saga; must wait for workload/storage/image cleanup to finish.

## 7. Non-Functional Highlights

- After membership changes, downstream read models (usage, quota, storage permissions, image allow-lists) must be quickly invalidated or rebuilt (NFR-MTEN-02).
- GPU usage and live quota queries delegate to the read models of usage-observability and scheduler-quota — never direct joins.

## 8. Decomposition Notes

Extracted in phase 4 together with IAM/Authz. All references to projects/groups from other services use UUIDs + event snapshots; cross-service DB joins are forbidden.
