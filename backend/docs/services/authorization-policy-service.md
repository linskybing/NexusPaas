# authorization-policy-service

Category: Core | Phase: 4

## 1. Overview

The platform's centralized authorization center (PDP — Policy Decision Point). Responsible for Casbin/domain RBAC, Proxy RBAC, policy simulation, service definitions, platform proxy roles, and policy sync. Provides an SDK + PDP API consumed by the Gateway and all other services; duplicating inconsistent RBAC logic in individual services is forbidden.

## 2. Functional Requirements

| ID | Requirement | Acceptance/Notes |
| --- | --- | --- |
| FR-RBAC-01 | Use domain-based RBAC for authorization decisions across Projects, Groups, global administration, and external proxied services. | The Casbin middleware must extract the domain from path/query/body. |
| FR-RBAC-02 | Support super-admin/admin/manager/user plus group/project-scoped roles, with permission inheritance along parent-child project paths. | Project hierarchy uses PostgreSQL ltree. |
| FR-RBAC-03 | Allow administrators to query, add, update, and delete Casbin policies, with batch operations and simulate-enforce. | /api/v1/permissions/*. |
| FR-RBAC-04 | Provide Proxy RBAC for external proxied services: service definitions, policies, policy assignments, platform proxy roles, role-user assignments, and system role queries. | /api/v1/admin/proxy-rbac/*. |
| FR-RBAC-05 | Ensure default path policies exist at startup and repair missing roles for existing users. | Derived from the RegisterRoutesWithServices initialization flow. |
| FR-RBAC-06 | Sync authorization policy changes to Kubernetes ConfigMaps or other runtime authorization data sources, so admission/proxy never uses stale rules. | Derived from PolicySyncer and policy_data_sync. |
| FR-RBAC-07 | Proxy routes must support JWT-only access, because browser pop-ups, iframes, img tags, and WebSockets cannot reliably send custom API Key headers. | Needed by Grafana, Harbor, MinIO, pgAdmin, Longhorn, IDE, and FileBrowser. |

## 3. Owned Data

`casbin_rule`, `policies`, `policy_rules`, `policy_assignments`, `platform_roles`, `user_platform_roles`, `service_definitions`.

## 4. Current Code/Route Mapping

- Handlers: `permission.go`, `policy.go`
- Middleware: `middleware/casbin`, `middleware/proxy_rbac`
- Sync: `authzsync`
- Routes: `/api/v1/permissions/*`, `/api/v1/admin/proxy-rbac/*`

## 5. External Interfaces

- PDP API: `Enforce(subject, domain, object, action)`, `SimulateEnforce`
- Shared SDK: same-version authorization logic embedded in the Gateway and services (with local cache + PolicyChanged invalidation)

## 6. Events

| Direction | Event | Counterpart | Purpose |
| --- | --- | --- | --- |
| Publish | PolicyChanged / ProxyPolicyChanged | gateway, integration-proxy, k8s-control, audit-compliance | Invalidate RBAC caches, sync ConfigMaps |
| Subscribe | UserCreated/Updated/Disabled | identity-service | Sync user role/status caches |
| Subscribe | GroupMembershipChanged, ProjectCreated/Deleted | org-project-service | Update domain membership read models |

## 7. Non-Functional Highlights

- Authorization logic centralized here or in a verifiable shared SDK (NFR-SEC-02) — the core mitigation for "RBAC rule drift".
- Authorization decisions are a high-frequency hot path requiring cache/read models (NFR-PERF-03).
- All policy changes must be audited and traceable (NFR-SEC-05).

## 8. Decomposition Notes

A highly shared service. Recommended dual mode — SDK + PDP API: hot paths use local SDK decisions (signed policy bundles), while administration and simulation go through the API.
