# Core Feature Area I: RBAC, Group, Project, and Personal Project

Part of the [GA Acceptance docs](README.md).

## Goal

The platform supports both platform-level roles and Project/Group roles.

Every user gets a personal Project on creation.

Group Projects inherit Group defaults but can have Project-specific overrides.

Project admins/managers can invite users outside the Group, but those users gain
only Project-scoped access.

## Platform Roles

| Role | Description |
|---|---|
| `platform_user` | Normal user |
| `platform_manager` | Operational manager with delegated admin permissions |
| `platform_admin` | Highest platform authority |
| `platform_auditor` | Read-only audit and usage role |
| `platform_system` | Service account / automation role |

`platform_admin` is protected. Rules:

- At least one enabled platform admin must always exist.
- The last platform admin cannot be deleted.
- The last platform admin cannot be disabled.
- The last platform admin cannot have the admin role removed.
- Regular API calls cannot weaken platform admin's core permission set.

## Group Roles

| Role | Description |
|---|---|
| `group_admin` | Manage Group settings, members, defaults |
| `group_manager` | Manage Projects and normal members |
| `group_user` | Use Group resources according to Project membership |
| `group_viewer` | Read-only |

## Project Roles

| Role | Description |
|---|---|
| `project_admin` | Manage Project settings, invite members, submit workloads |
| `project_manager` | Manage workloads and members within limits |
| `project_user` | Submit workloads and use allowed resources |
| `project_viewer` | Read-only |
| `project_external_user` | Invited non-Group member with scoped access |

## Personal Project

On user creation:

```text
create user
  -> create personal Project
  -> attach default personal Plan
  -> assign user as project_admin
  -> emit audit event
```

## Group Project Inheritance

Effective Project policy is computed from:

```text
platform defaults
+ group defaults
+ project overrides
+ project capabilities
+ attached plan
```

## Acceptance Criteria

| ID | Acceptance Criteria |
|---|---|
| RBAC-001 | Creating a user automatically creates a personal Project. |
| RBAC-002 | Personal Project has default personal Plan attached. |
| RBAC-003 | User is project_admin of their personal Project. |
| RBAC-004 | Group admin can create Group Project. |
| RBAC-005 | Group Project inherits Group defaults. |
| RBAC-006 | Group default changes can recompute effective Project policy. |
| RBAC-007 | Project admin can invite non-Group user. |
| RBAC-008 | Invited non-Group user gains only Project-scoped access. |
| RBAC-009 | Non-Group invited user does not gain Group-wide storage or admin access. |
| RBAC-010 | Project manager cannot grant platform role. |
| RBAC-011 | Project admin cannot grant high-risk Project capabilities. |
| RBAC-012 | Only platform admin can grant root, hostPath, external egress, privileged, hostPID, hostIPC, hostNetwork, image build, and WebRTC capability. |
| RBAC-013 | Last platform admin cannot be removed, disabled, or demoted. |
| RBAC-014 | All role changes produce audit events. |
| RBAC-015 | All Project invitations produce audit events. |
| RBAC-016 | RBAC tests cover every public API route. |
| RBAC-017 | OpenAPI security metadata matches actual middleware authorization behavior. |
| RBAC-018 | Project member cannot list workloads from unrelated Project. |
| RBAC-019 | Group admin can view Group usage across Projects. |
| RBAC-020 | Platform auditor can view audit/usage but cannot mutate platform state. |
