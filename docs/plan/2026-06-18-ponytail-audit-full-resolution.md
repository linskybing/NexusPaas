# Ponytail Audit Full Resolution

## 1. Objective

Resolve the ponytail audit items selected by the user under the "All audit items" scope: finish the approved compatibility-route/client/helper simplification work, then remove remaining single-implementation repository/client interfaces, monolith rollback/switch plumbing, and centralized catalog ownership bloat.

## 2. Background

The first-wave plan `2026-06-18-ponytail-simplification-compat-removal.md` is approved and partially present in the working tree. The user expanded the scope to include all audit findings, including items that the first-wave plan explicitly deferred. This plan supersedes that bounded first-wave plan and must be reviewed before any production implementation continues.

## 3. Source References

- `AGENTS.md`
- `docs/agents/workflow.md`
- `docs/agents/planning.md`
- `docs/agents/review-checklist.md`
- `docs/plan/2026-06-18-ponytail-simplification-compat-removal.md`
- `backend/internal/platform/service_client.go`
- `backend/internal/platform/rollback.go`
- `backend/internal/platform/proxy.go`
- `backend/internal/services/catalog.go`
- `backend/internal/services/*/*repository*.go`
- `backend/internal/services/workload/*client.go`
- `backend/internal/services/schedulerquota/*client.go`
- `backend/internal/services/shared/text.go`

## 4. Assumptions

- "All audit items" means all prior ponytail audit findings except deleting real platform boundary ports or third-party dependencies; current inspection found no safe dependency removal.
- Existing dirty worktree changes are user or prior-task work and must not be reverted.
- Repository collapse is source-level simplification only: method behavior, nil-store behavior, resource names, record payloads, and API responses stay unchanged.
- If a candidate interface has more than one real production implementation, keep it and document why.
- The platform still needs external adapter proxying for Grafana, MinIO, PgAdmin, Longhorn, Harbor, and similar owned integrations.

## 5. Non-Goals

- No database schema or migration changes.
- No new public API surface.
- No new dependency, code generation, reflection framework, or service registry framework.
- No removal of canonical service routes.
- No removal of true boundary interfaces: `platform.RecordStore`, `ObjectStore`, `RevocationStore`, `contracts.PolicyDecisionPoint`, `WorkerLease`, `ExternalAdapter`, and `ProxyAdapter`.
- No implementation of unrelated E2E, LDAP, Kubernetes, image-build, or IDE changes beyond preserving existing dirty worktree behavior.

## 6. Current Behavior

The working tree already contains part of the first-wave simplification, including `platform.InternalJSONClient` and catalog changes. Remaining bloat includes package-private single-implementation repository/client interfaces, an empty store-dependency path, monolith rollback/switch routing, platform-forwarding proxy compatibility code, duplicated helper functions, and a centralized `services.Catalog()` that owns most service route metadata.

## 7. Target Behavior

Service-to-service JSON callers use the shared client. Legacy compatibility and monolith fallback routes are gone. Single-implementation service repositories and internal clients are concrete package-private types or small functions. Service packages own their route metadata through `Spec()` functions, while `services.Catalog()` only aggregates specs and registers handlers. Canonical external adapter proxy routes keep working.

## 8. Affected Domains

- Platform routing, adapter proxying, and route metadata.
- Service catalog registration.
- Workload and scheduler-quota internal service clients.
- Service repository wrappers across workload, identity, authorization-policy, org-project, scheduler-quota, storage, IDE, and request-notification.
- Shared helper usage.

## 9. Affected Files

Explicit production targets:

| Area | Files | Planned action |
|---|---|---|
| Shared internal JSON client | `backend/internal/platform/service_client.go` | Keep `InternalJSONClient`; finish tests/status handling as needed. |
| Monolith rollback/switch | `backend/internal/platform/rollback.go`, `proxy.go`, `proxy_stream.go`, `app.go`, `ports.go`, `config.go` | Delete route switch/rollback/monolith fallback only; keep canonical external adapter proxying. |
| Catalog aggregation | `backend/internal/services/catalog.go` | Remove empty store-dependency plumbing; aggregate package-owned specs. |
| Service specs | `backend/internal/services/{identity,authorizationpolicy,orgproject,workload,schedulerquota,k8scontrol,ideworkspace,storage,imageregistry,gpuusage,clusterread,dashboard,resourcehours,auditcompliance,requestnotification,integrationproxy,mediaupload}` | Add small `Spec() platform.ServiceSpec` only where route metadata moves out of `catalog.go`. |
| Repository interfaces | `workload/config_repository.go`, `workload/job_repository.go`, `identity/auth_repository.go`, `identity/principal_repository.go`, `authorizationpolicy/authorization_policy_repository.go`, `authorizationpolicy/raw_permission_repository.go`, `authorizationpolicy/authorization_policy_projection_repository.go`, `orgproject/project_repository.go`, `orgproject/org_project_group_gpu_repository.go`, `schedulerquota/scheduler_quota_repository.go`, `schedulerquota/scheduler_preemption_priority_repository.go`, `storage/storage_repository.go`, `ideworkspace/ide_projection_repository.go`, `requestnotification/project_access_repository.go` | Delete package-private single-implementation interfaces; constructors return concrete repositories. |
| Internal client interfaces | `workload/scheduler_admission_client.go`, `workload/scheduler_preemption_client.go`, `workload/storage_mount_client.go`, `schedulerquota/preemption_client.go`, `schedulerquota/eviction_client.go`, `schedulerquota/plan_binding_client.go` | Delete package-private single-implementation interfaces where call sites do not require a test double; otherwise replace with a tiny function type. |
| Helper duplicates | `clusterread/handler.go`, `gpuusage/helpers.go`, `gpuusage/projection.go`, `ideworkspace/handler.go`, `integrationproxy/helpers.go`, `resourcehours/handler.go`, and already-touched helper files | Replace only helpers with matching `shared` semantics; leave package-specific variants. |
| Dead code | `workload/storage_mount_client.go`, `workload/dispatcher.go`, `schedulerquota/preemption.go`, live plan-window E2E helpers | Delete `errorStorageMountPlanClient`/wrapper fallback if direct construction error handling is enough, `preemptionDemandCanBeSatisfied`, unused `ValidUntil`, and unused `livePausePodManifest`. |

Expected test targets:

- Platform routing/proxy/config tests, including removal or inversion of monolith rollback tests.
- Catalog/openapi/service-isolation tests proving canonical routes remain and compat routes are absent.
- Repository tests updated to concrete constructors without behavior changes.
- Workload and scheduler-quota client tests updated for `InternalJSONClient`.
- Route/isolation E2E contract tests listed in the verification plan.

## 10. API / Contract Changes

- Breaking public cleanup: these legacy/compat public routes are removed:
  - all-method platform gateway monolith fallback `/api/v1/{path...}`;
  - all-method storage compatibility proxy `/api/v1/storage/{id}/storage/{pvcId}/proxy/{path...}`;
  - all-method k8s user-storage proxy `/api/v1/k8s/user-storage/proxy/{path...}`;
  - all-method IDE proxy compatibility route `/api/v1/ide/proxy/{podName}/{path...}` except the canonical `GET` route if service-owned;
  - all-method integration proxy compatibility routes `/api/v1/grafana/{path...}`, `/api/v1/minio-console/{path...}`, `/api/v1/pgadmin/{path...}`, `/api/v1/longhorn/{path...}`, `/api/v1/harbor/{path...}`, and `/api/v1/harbor-gpu23/{path...}` where they were generated by `anyCompatRoutes`; canonical `GET` proxy metadata remains for Grafana, MinIO console, PgAdmin, Longhorn, and Harbor; `harbor-gpu23` is removed;
  - identity legacy OIDC compatibility paths `/oauth/token`, `/device_authorization`, `/revoke`, `/api/v1/.well-known/{path...}`, `/api/v1/keys`, `/api/v1/authorize`, `/api/v1/userinfo`, and `/api/v1/authorize/callback`;
  - authorization-policy legacy proxy-RBAC paths `POST /api/v1/admin/proxy-rbac/assignments`, `GET /api/v1/admin/proxy-rbac/platform-roles`, and `POST /api/v1/admin/proxy-rbac/role-users`.
- Canonical routes that must remain include:
  - platform gateway `/api/v1/gateway/routes` and `/api/v1/gateway/health`;
  - identity `/api/v1/login`, `/api/v1/logout`, `/api/v1/register`, `/api/v1/refresh`, `/api/v1/cli/login`, `/api/v1/me/...`, `/api/v1/users...`, and `/api/v1/oidc/...`;
  - authorization-policy `/api/v1/permissions/...` and canonical `/api/v1/admin/proxy-rbac/services|policies|roles|system-roles...`;
  - workload `/api/v1/configfiles...`, `/api/v1/jobs...`, and `/internal/workload/...`;
  - scheduler-quota `/api/v1/plans...`, `/api/v1/queues...`, and `/api/v1/internal/scheduler/...`;
  - storage, k8s-control, IDE, image-registry, usage, audit, notification, media, and integration-proxy canonical routes directly registered by their services.
- Internal HTTP contracts remain stable for scheduler admission/preemption, workload preemption/eviction/context, storage mount-plan, and org-project plan binding.
- Internal Go symbols for package-private repository/client interfaces disappear; call sites use concrete package-private structs or direct functions.

## 11. Database / Migration Changes

None.

## 12. Configuration Changes

- Remove `MONOLITH_UPSTREAM_URL` support and the default `monolith` adapter.
- Preserve `SERVICE_URLS`, `SERVICE_API_KEY`, `ADAPTER_TIMEOUT`, and external adapter URL/config handling.
- Remove dead top-level config helper wrappers only when they have no call sites.

## 13. Observability Changes

No metric, trace, or log schema changes. Removing monolith rollback/switch metrics is acceptable because the in-process rollback path is deleted. Existing external adapter degraded metrics remain.

## 14. Security Considerations

Removing compatibility routes reduces exposed public surface. Shared internal clients must preserve `X-Service-Key` and scheduler `X-API-Key` behavior. Canonical route admin/auth metadata must remain equivalent after moving route specs into service packages. Sensitive header stripping in external proxies must not change.

## 15. Implementation Steps

1. Finish first-wave cleanup already approved: verify `InternalJSONClient` coverage, internal client refactors, compat route absence, legacy handler removal, dead helper deletion, and helper consolidation.
2. Collapse package-private single-implementation repository interfaces by changing constructors to return concrete repository types and updating call sites/tests. Preserve nil-store behavior exactly.
3. Collapse package-private single-implementation internal client interfaces where no test double or second production implementation is needed. Use small function fields in tests only when required.
4. Delete monolith rollback/switch plumbing: `RollbackGate`, `RouteSwitches`, `WithSwitches`, default `monolith` adapter, `MONOLITH_UPSTREAM_URL`, and `forwardToService`. Keep canonical external adapter proxy and streaming proxy behavior.
5. Move service-owned route metadata into package-local `Spec()` functions and reduce `services.Catalog()` to aggregation plus platform gateway metadata. Do not introduce reflection or generation.
6. Delete empty `serviceStoreDependencies`/`storeDependency` plumbing and keep only owner-read dependency registration.
7. Replace duplicated helper functions only when semantics match existing `shared` helpers; leave package-specific variants in place.
8. Update or remove tests that assert deleted compat/monolith behavior. Add compact tests proving canonical routes, external proxy behavior, and service specs still work.
9. Run formatting and verification.

## 16. Verification Plan

Run from repository root:

- `go -C backend test ./internal/platform ./internal/services/workload ./internal/services/schedulerquota ./internal/services/authorizationpolicy ./internal/services/identity ./internal/services/orgproject ./internal/services/storage ./internal/services/ideworkspace ./internal/services/requestnotification -count=1`
- `go -C backend test -tags e2e ./internal/e2e -run 'TestServiceRouteIsolationContract|TestServiceIsolationValidationE2E|TestProviderConsumerContractMatrix' -count=1 -v`
- `go -C backend test ./... -count=1`
- `go -C backend vet ./...`
- `go -C backend build ./...`
- `bash backend/scripts/ci-security-gate.sh quick`
- SonarScanner command, only with user approval and configured `SONAR_TOKEN` plus `SONAR_HOST_URL`: `bash backend/scripts/ci-security-gate.sh sonar`

If Sonar credentials or user approval are unavailable, record `Not Run` with the reason. The script writes `${CI_GATE_ARTIFACT_DIR:-$TMPDIR/nexuspaas-quality-gate/<run>}/sonar-skipped.txt` when credentials are missing and Sonar is not required; if `CI_GATE_SONAR_REQUIRED=1` or CI policy requires Sonar, missing credentials are a verification failure.

## 17. Rollback Plan

Revert the full simplification commit. If only route ownership movement fails, restore centralized catalog metadata while keeping repository/interface collapse. If only external proxy behavior regresses, restore the affected proxy tests and canonical proxy path without restoring monolith fallback.

## 18. Risks and Tradeoffs

This is larger than the first-wave plan and touches many packages. The safest path is deletion-first and compile-driven: remove one interface/plumbing layer at a time, run focused tests, then continue. The main risk is accidentally removing canonical proxy behavior while deleting monolith fallback; tests must distinguish those paths.

## 19. Reviewer Checklist

- Confirm this plan supersedes the first-wave plan and explicitly includes repository interface collapse.
- Confirm true platform boundary interfaces and canonical external proxy behavior remain.
- Confirm public breaking changes are limited to legacy/compat and monolith fallback routes.
- Confirm service-owned `Spec()` functions do not introduce a new framework.
- Confirm no database migration, dependency, or unrelated E2E behavior is added.
- Confirm verification covers platform routing, catalog, affected repositories, and route isolation contracts.

## 20. Status

Status: Approved
