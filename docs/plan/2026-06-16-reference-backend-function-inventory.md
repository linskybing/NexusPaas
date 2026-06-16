# Reference Backend Function Inventory

## 1. Objective

Create a documentation-only function inventory at `/Users/sky/workspaces/function.md` that maps the reference backend capabilities to the target microservice catalog for future development planning.

## 2. Background

The repository contains the legacy/reference Go backend under `references/CSCC_AI_Platform_Backend` and a target microservice architecture under `backend/`. The requested output is a capability inventory, not a code migration. It must preserve the current route/function coverage while assigning each capability to the existing bounded-context service catalog.

## 3. Source References

- `references/CSCC_AI_Platform_Backend/README.md`
- `references/CSCC_AI_Platform_Backend/docs/en/02_architecture.md`
- `references/CSCC_AI_Platform_Backend/internal/api/routes/*.go`
- `references/CSCC_AI_Platform_Backend/internal/plugin/builtin/job/plugin.go`
- `references/CSCC_AI_Platform_Backend/internal/api/openapi/spec/paths/*.yaml`
- `references/CSCC_AI_Platform_Backend/internal/api/routes/background.go`
- `references/CSCC_AI_Platform_Backend/internal/domain/*`
- `references/CSCC_AI_Platform_Backend/internal/repository/*.go`
- `backend/README.md`
- `backend/docs/api-route-mapping.md`
- `backend/docs/event-contracts.md`
- `backend/docs/migration-roadmap.md`
- `backend/*-service/README.md`
- `backend/platform-gateway/README.md`

## 4. Assumptions

- `function.md` should be created at the workspace root: `/Users/sky/workspaces/function.md`.
- Existing dirty worktree changes are user-owned and must not be reverted or reformatted.
- `references/CSCC_AI_Platform_Backend` is the feature source of truth.
- Current `backend/` service docs are used only to assign target microservice boundaries.
- The document should use the existing 15-service catalog and avoid proposing new services.
- This task is documentation-only; no production code, schema, config, or deployment file changes are required.

## 5. Non-Goals

- Do not implement or refactor microservices.
- Do not modify existing source code, tests, service READMEs, deployment manifests, migrations, or route mappings.
- Do not redesign service boundaries beyond the existing 15-service catalog.
- Do not generate OpenAPI files or service contracts.
- Do not run code formatters or code generators.

## 6. Current Behavior

Reference backend functionality is spread across route files, the job plugin, OpenAPI path fragments, application/domain/repository modules, cron tasks, and existing microservice planning documents. There is no single `function.md` that lists all capabilities and maps them to target services.

## 7. Target Behavior

`function.md` provides a concise but complete function table for microservice development. It maps each capability to a target service, current routes/jobs/events, owned data, dependencies, and notes. It separately covers non-HTTP behavior such as cron tasks, collectors, dispatchers, reapers, policy sync, OIDC provider flows, JWT-only proxies, WebSockets, and audit/event obligations.

## 8. Affected Domains

- Platform gateway and edge routing
- Identity and account management
- Authorization and policy
- Organization, groups, projects, and tenancy
- Workloads, config files, jobs, scheduler, quota, and priority
- Kubernetes control and IDE workspaces
- Storage, image registry, media upload, and integration proxies
- Usage, dashboard, audit, notifications, and compliance

## 9. Affected Files

- Add `docs/plan/2026-06-16-reference-backend-function-inventory.md`
- Add `function.md`

## 10. API / Contract Changes

None. This is a documentation inventory only. Existing public `/api/v1` routes, WebSocket paths, proxy paths, OpenAPI behavior, and response contracts remain unchanged.

## 11. Database / Migration Changes

None. The inventory will mention owned data areas, but it will not create migrations, change schemas, or alter data ownership in code.

## 12. Configuration Changes

None. The inventory may mention config-driven background jobs and external dependencies, but no environment variables, ConfigMaps, Secrets, or deployment settings will change.

## 13. Observability Changes

None in runtime behavior. The inventory will document observability-related capabilities such as metrics, traces, audit events, usage collectors, dashboards, and background worker monitoring.

## 14. Security Considerations

The document must preserve security-relevant distinctions:

- Gateway/API key/JWT/User API Token entry points.
- JWT-only browser routes for proxies, WebSockets, IDE, FileBrowser, and image serving.
- Centralized authorization-policy ownership for Casbin, domain RBAC, and proxy RBAC.
- AuditEvent obligations for administrative operations, permission changes, and important Job/Storage/Image lifecycle events.
- No secrets or credentials should be added to the document.

## 15. Implementation Steps

1. Create this Draft plan file under `docs/plan/`.
2. Run Reviewer Agent plan review and revise until `Status: Approved`.
3. Create `/Users/sky/workspaces/function.md` after plan approval.
4. Build the function inventory using the existing 15-service catalog:
   - `platform-gateway`
   - `identity-service`
   - `authorization-policy-service`
   - `org-project-service`
   - `workload-service`
   - `scheduler-quota-service`
   - `k8s-control-service`
   - `ide-service`
   - `storage-service`
   - `image-registry-service`
   - `usage-observability-service`
   - `audit-compliance-service`
   - `request-notification-service`
   - `integration-proxy-service`
   - `media-upload-service`
5. Include function table columns:
   - `ID`
   - `Domain`
   - `Function`
   - `Target Microservice`
   - `Current Routes / Jobs / Events`
   - `Owned Data`
   - `Dependencies`
   - `Notes`
6. Cover these capability groups:
   - auth/session/API tokens/CLI/OIDC/users
   - RBAC/Casbin/proxy RBAC/policy sync
   - groups/user-groups/projects/members/workspace settings/GPU claims
   - configfiles/jobs/job GPU views
   - plans/queues/quota/preemption/priority
   - K8s logs/resources/WebSockets/cluster state
   - IDE lifecycle/proxy/idle reaping
   - user/group/project storage/FileBrowser/fast transfer/Longhorn validation
   - image requests/builds/catalog/Harbor governance
   - usage/dashboard/resource-hours/GPU snapshots
   - audit/report/security posture
   - forms/notifications/announcements
   - external UI proxies/VPN
   - media upload/serve
   - gateway health/metrics/OpenAPI/routing
7. Add a separate non-HTTP coverage checklist for background jobs:
   - audit cleanup
   - Harbor health checks
   - LDAP mirror sync
   - cluster resource collector
   - GPU usage collector
   - resource hours collector
   - resource quota reconciler
   - priority class sync
   - idle reaper
   - plan window reaper
   - workload runtime reaper
   - policy data sync
   - Longhorn RWX health reconciler
   - VPN usage collector
   - queue metrics collector
   - job dispatcher
8. Verify route/service/background coverage.
9. Run Reviewer Agent implementation review and address any requested documentation fixes.

## 16. Verification Plan

- Compare `function.md` rows against route scopes extracted from `references/CSCC_AI_Platform_Backend/internal/api/routes/*.go`.
- Compare job rows against `references/CSCC_AI_Platform_Backend/internal/plugin/builtin/job/plugin.go`.
- Compare route coverage against OpenAPI path fragments in `references/CSCC_AI_Platform_Backend/internal/api/openapi/spec/paths/*.yaml`.
- Confirm every route scope in `backend/docs/api-route-mapping.md` maps to at least one function row.
- Confirm every service README in `backend/` maps to at least one function row.
- Confirm background jobs from `internal/api/routes/background.go` are listed outside the HTTP API table.
- No code build is required because this is documentation-only.

## 17. Rollback Plan

Rollback is limited to removing the newly added documentation files:

- Remove `docs/plan/2026-06-16-reference-backend-function-inventory.md`
- Remove `function.md`

No runtime or data rollback is needed.

## 18. Risks and Tradeoffs

- Risk: Missing non-route behavior if only HTTP paths are inventoried. Mitigation: include background jobs, plugin routes, events, domains, repositories, and service READMEs.
- Risk: Over-splitting by controller or table. Mitigation: use existing bounded-context service catalog and capability-based grouping.
- Risk: Dirty worktree contains unrelated changes. Mitigation: only add the two approved documentation files and do not edit existing files.
- Tradeoff: The inventory is broad, so some rows group related endpoints instead of listing every method as a separate row. This keeps the document usable for microservice planning while preserving route references in each group.

## 19. Reviewer Checklist

| Category | Check |
| --- | --- |
| Requirement Fit | `function.md` maps reference backend capabilities to target microservices. |
| Scope Control | Only the plan file and `function.md` are changed. |
| Architecture | Uses existing bounded-context service catalog, not controller/table splitting. |
| API Contract | Documents current routes but changes no API contract. |
| Data Ownership | Each row identifies owned data at a capability level. |
| Config | No config changes. |
| Observability | Background jobs, audit, usage, metrics, and dashboard capabilities are covered. |
| Security | JWT-only routes, RBAC ownership, and audit obligations are preserved. |
| Testing | Documentation verification checks route, service, OpenAPI, and background coverage. |
| Rollback | Remove the two added documentation files. |
| Diff Scope | No unrelated dirty files are touched. |

## 20. Status

Status: Draft
