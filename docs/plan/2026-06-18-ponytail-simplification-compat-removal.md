# Ponytail Simplification + Compat Route Removal

## 1. Objective

Implement the first ponytail simplification wave: consolidate duplicated internal service-client plumbing, delete low-risk dead helpers, replace duplicated local helper code where semantics already match shared helpers, and remove compatibility route registration.

## 2. Background

The ponytail audit found repeated local/HTTP service-client implementations, helper duplicates, dead config/test helpers, and legacy/compat route surfaces. The repository already has canonical service handlers and internal contracts, so this pass should shorten plumbing without changing owned data models.

This pass is deliberately bounded. Repository interface consolidation, persistence model changes, and new external APIs are out of scope.

## 3. Source References

- `AGENTS.md`
- `docs/agents/workflow.md`
- `docs/agents/planning.md`
- `docs/agents/review-checklist.md`
- `backend/internal/services/catalog.go`
- `backend/internal/platform/service_client.go`
- `backend/internal/services/workload/storage_mount_client.go`
- `backend/internal/services/workload/scheduler_admission_client.go`
- `backend/internal/services/workload/scheduler_preemption_client.go`
- `backend/internal/services/schedulerquota/preemption_client.go`
- `backend/internal/services/schedulerquota/eviction_client.go`
- `backend/internal/services/schedulerquota/plan_binding_client.go`
- `backend/internal/services/authorizationpolicy/handler.go`
- `backend/internal/services/authorizationpolicy/assignments.go`
- `backend/internal/services/authorizationpolicy/roles.go`
- `backend/internal/services/identity/handler.go`
- `backend/internal/services/identity/oidc_dex.go`
- `backend/internal/services/shared/text.go`
- `backend/internal/services/shared/maps.go`

## 4. Assumptions

- "All Compat Routes" means remove `referenceCompatRoutes`, `anyCompatRoutes`, `publicRouteSet`, platform gateway `/api/v1/{path...}` monolith proxy route specs, and legacy-only custom handlers tied to those removed routes.
- Canonical handlers directly registered by service packages remain available.
- Any directly registered custom handler route whose metadata currently exists only in `referenceCompatRoutes` must be moved into the owning `ServiceSpec` before `referenceCompatRoutes` is deleted.
- Existing dirty worktree changes are user or prior-task work and must not be reverted.
- Repository interface consolidation is deferred to a separate plan.
- The work must happen on a short feature branch and be delivered through a PR because this is a breaking public route cleanup.

## 5. Non-Goals

- No repository interface collapse.
- No database schema or migration changes.
- No behavior change to internal service contracts.
- No removal of canonical feature endpoints.
- No frontend or deployment manifest changes.
- No new third-party dependency.

## 6. Current Behavior

Several services each implement their own local/HTTP client pair for internal contracts. `Catalog()` appends `referenceCompatRoutes()` to service route metadata, and the platform gateway service advertises catch-all monolith proxy routes. Some legacy proxy-RBAC and OIDC compatibility handlers are registered solely to support removed compatibility paths.

Several packages keep local `firstNonEmpty`, `textValue`, `mapValue`, and `cloneMap` helpers despite shared equivalents in `internal/services/shared`.

## 7. Target Behavior

Internal clients share one platform-level JSON service caller. Catalog metadata exposes only canonical service routes. Legacy/compat paths no longer appear in catalog/openapi/route metadata and legacy-only custom handlers are not registered. Helper duplicates are replaced only where semantics match. Dead helpers are deleted.

## 8. Affected Domains

- Platform service-to-service plumbing.
- Service catalog and OpenAPI/route metadata.
- Authorization policy legacy proxy-RBAC compatibility.
- Identity Dex/OIDC compatibility proxy registration.
- Workload and scheduler-quota internal clients.
- Shared helper usage.

## 9. Affected Files

Production files expected to change:

- `backend/internal/platform/service_client.go`
- `backend/internal/services/catalog.go`
- `backend/internal/services/workload/storage_mount_client.go`
- `backend/internal/services/workload/scheduler_admission_client.go`
- `backend/internal/services/workload/scheduler_preemption_client.go`
- `backend/internal/services/schedulerquota/preemption_client.go`
- `backend/internal/services/schedulerquota/eviction_client.go`
- `backend/internal/services/schedulerquota/plan_binding_client.go`
- `backend/internal/services/authorizationpolicy/handler.go`
- `backend/internal/services/authorizationpolicy/assignments.go`
- `backend/internal/services/authorizationpolicy/roles.go`
- `backend/internal/services/identity/handler.go`
- `backend/internal/services/identity/oidc_dex.go`
- `backend/internal/platform/config.go`
- `backend/internal/services/workload/dispatcher.go`
- `backend/internal/services/schedulerquota/preemption.go`
- `backend/internal/e2e/live_plan_window_duration_preemption_e2e_test.go`

Helper files may change only when the local helper has matching semantics with `shared` helpers:

- `backend/internal/services/shared/text.go`
- `backend/internal/services/shared/maps.go`
- `backend/internal/services/clusterread/handler.go`
- `backend/internal/services/gpuusage/helpers.go`
- `backend/internal/services/gpuusage/projection.go`
- `backend/internal/services/ideworkspace/handler.go`
- `backend/internal/services/imageregistry/helpers.go`
- `backend/internal/services/integrationproxy/helpers.go`
- `backend/internal/services/orgproject/group_helpers.go`
- `backend/internal/services/storage/helpers.go`
- `backend/internal/services/auditcompliance/handler.go`
- `backend/internal/services/resourcehours/handler.go`
- `backend/internal/services/k8scontrol/docker_cleanup.go`

Tests expected to change or be added:

- `backend/internal/platform/internal_json_client_test.go`
- `backend/internal/platform/openapi_test.go`
- `backend/internal/platform/forward_test.go`
- `backend/internal/platform/routing_test.go`
- `backend/internal/services/catalog_test.go`
- `backend/internal/services/authorization_policy_test.go`
- `backend/internal/services/authorizationpolicy/workflow_test.go`
- `backend/internal/services/identity_auth_test.go`
- `backend/internal/services/identity/workflow_test.go`
- `backend/internal/services/identity/handler_test.go`
- `backend/internal/services/oidc_dex_integration_test.go`
- `backend/internal/services/oidc_revoke_device_test.go`
- `backend/internal/services/workload/storage_mount_client_test.go`
- `backend/internal/services/workload/scheduler_admission_client_test.go`
- `backend/internal/services/workload/scheduler_preemption_client_test.go`
- `backend/internal/services/schedulerquota/preemption_client_test.go`
- `backend/internal/services/schedulerquota/eviction_client_test.go`
- `backend/internal/services/schedulerquota/plan_binding_client_test.go`

## 10. API / Contract Changes

Breaking public compatibility removal:

- Remove all `referenceCompatRoutes()` catalog appending after canonical metadata is moved to owning service specs.
- Delete `anyCompatRoutes()` and `publicRouteSet()` after their callers are removed.
- Remove platform gateway route specs for all methods on `/api/v1/{path...}` with `ExternalAdapter: monolith`.
- Remove storage compatibility metadata for `/api/v1/storage/{id}/storage/{pvcId}/proxy/{path...}`. Keep canonical storage metadata for `GET /api/v1/storage/filebrowser/{id}/proxy/{path...}`.
- Remove k8s user-storage compatibility metadata for `/api/v1/k8s/user-storage/proxy/{path...}`. Keep canonical k8s storage status/browse routes already in `k8sControlService()`.
- Remove all-method IDE compatibility metadata generated by `anyCompatRoutes` for `/api/v1/ide/proxy/{podName}/{path...}`. Keep the canonical `GET /api/v1/ide/proxy/{podName}/{path...}` route already in `ideService()`.
- Remove all-method integration-proxy compatibility metadata generated by `anyCompatRoutes` for `/api/v1/grafana/{path...}`, `/api/v1/minio-console/{path...}`, `/api/v1/pgadmin/{path...}`, `/api/v1/longhorn/{path...}`, `/api/v1/harbor/{path...}`, and `/api/v1/harbor-gpu23/{path...}`. Keep canonical `GET` metadata already in `integrationProxyService()` for Grafana, MinIO console, PgAdmin, Longhorn, and Harbor. `harbor-gpu23` has no canonical owning route and is removed.
- Remove identity legacy OIDC public route metadata and handlers for `/oauth/token`, `/device_authorization`, `/revoke`, `/api/v1/.well-known/{path...}`, `/api/v1/keys`, `/api/v1/authorize`, `/api/v1/userinfo`, and `/api/v1/authorize/callback`.
- Remove authorization-policy legacy proxy-RBAC handlers for `POST /api/v1/admin/proxy-rbac/assignments`, `GET /api/v1/admin/proxy-rbac/platform-roles`, and `POST /api/v1/admin/proxy-rbac/role-users`.

Canonical metadata that must remain or be moved into owning service specs before deletion:

- Platform gateway canonical routes `/api/v1/gateway/routes` and `/api/v1/gateway/health`.
- Authorization-policy raw permission routes `/api/v1/permissions/policy`, `/api/v1/permissions/batch`, `/api/v1/permissions/enforce`, and `/api/v1/permissions/simulate`.
- Authorization-policy proxy-RBAC canonical routes under `/api/v1/admin/proxy-rbac/services`, `/services/{id}`, `/policies`, `/policies/{id}`, `/policies/{id}/assignments`, `/policies/{id}/assignments/batch`, `/targets/{type}/{id}/assignments`, `/roles`, `/roles/{id}`, `/roles/{id}/users`, `/roles/{id}/users/batch`, and `/system-roles`.
- Identity auth/user routes under `/api/v1/login`, `/api/v1/logout`, `/api/v1/register`, `/api/v1/refresh`, `/api/v1/cli/login`, `/api/v1/me/...`, and `/api/v1/users...`, including direct registered legacy-shaped but still canonical user-admin routes such as `/api/v1/users/paging`, `/api/v1/users/resolve`, `/api/v1/users/batch/password`, and `/api/v1/users/batch/role`.
- Canonical OIDC routes under `/api/v1/oidc/...`, including discovery, JWKS, authorize, token, userinfo, revoke, and callback.
- Existing service-owned custom handlers that are not route-compat passthroughs, including integration-proxy SSO/admin handlers and k8s/workload resource handlers.

Stable internal contracts:

- Scheduler admission and preemption internal endpoints keep the same paths, request bodies, response envelopes, and status expectations.
- Workload preemption, eviction, and context internal endpoints remain stable.
- Storage mount-plan and org-project plan binding internal endpoints remain stable.

## 11. Database / Migration Changes

None.

## 12. Configuration Changes

No new configuration.

Existing values continue to drive remote internal clients:

- `SERVICE_URLS`
- `SERVICE_API_KEY`
- `ADAPTER_TIMEOUT`

Unused top-level platform config helpers `envBool`, `envDuration`, and `envInt` in `backend/internal/platform/config.go` are removed only if they have no call sites.

## 13. Observability Changes

No metric or tracing schema changes. Shared HTTP client should preserve existing request behavior; local calls still go through `app.ServeHTTP` and existing middleware logging.

The shared client must preserve caller-supplied propagation headers, including `X-Request-ID`, `X-Trace-ID`, `Traceparent`, and `Idempotency-Key`, when the wrapper passes them.

## 14. Security Considerations

Internal remote calls must continue sending `X-Service-Key`. Scheduler clients must also preserve existing `X-API-Key` behavior. Local co-hosted calls must keep current service-key authorization behavior. Removing catch-all and legacy compatibility routes reduces exposed public surface. Do not weaken canonical auth/admin flags.

## 15. Implementation Steps

1. Add `platform.InternalJSONClient` in `backend/internal/platform/service_client.go`.
2. Cover the shared client with `backend/internal/platform/internal_json_client_test.go`.
3. Refactor workload storage mount, scheduler admission, scheduler preemption, scheduler-quota workload preemption, workload eviction, and org-project plan binding clients to use the shared client while preserving typed request/response/status mapping.
4. Move canonical authorization-policy and identity route metadata that currently lives only in `referenceCompatRoutes()` into `authorizationPolicyService()` and `identityService()`.
5. Remove `referenceCompatRoutes()` append from `Catalog()`, delete `referenceCompatRoutes`, delete `anyCompatRoutes`, delete `publicRouteSet`, and remove now-unused compatibility constants.
6. Remove platform gateway catch-all monolith proxy route specs while keeping canonical gateway route specs.
7. Remove authorization-policy legacy registrations from `Register`, then delete `assignPolicyLegacy`, `listPlatformRolesLegacy`, and `assignRoleUserLegacy` plus tests that directly exercise those legacy functions/routes.
8. Stop registering identity legacy OIDC paths in `identity.Register` and `registerDexProxies`, while keeping canonical `/api/v1/oidc/...` handlers and canonical Dex proxy behavior.
9. Replace duplicated helper functions with `shared` helpers only where return semantics match. Add `shared.FirstNonBlank` only if needed to replace trimmed `firstNonEmpty` copies without changing whitespace semantics.
10. Delete dead helpers: top-level platform config env parsers, `appStorageMountPlanClient`, `preemptionDemandCanBeSatisfied`, unused `ValidUntil` field, and `livePausePodManifest`.
11. Update catalog/openapi/platform/service tests to assert removed compat routes are absent and canonical routes still exist.
12. Run formatting and verification.

`platform.InternalJSONClient` proposed public shape:

```go
type InternalJSONClient struct {
	// fields unexported
}

type InternalJSONRequest struct {
	Method        string
	Path          string
	Query         url.Values
	Headers       http.Header
	Body          any
	Response      any
	ResponseLimit int64
}

type InternalJSONResponse struct {
	StatusCode int
	Header     http.Header
}

func NewInternalJSONClient(app *App, owner string) InternalJSONClient
func (c InternalJSONClient) Do(ctx context.Context, req InternalJSONRequest) (InternalJSONResponse, error)
```

`InternalJSONClient` invariants:

- Local mode is used when the owner service is co-hosted. It uses `httptest.NewRequestWithContext` and `app.ServeHTTP`.
- Remote mode is used when the owner service is not co-hosted. It resolves base URL from `SERVICE_URLS`.
- Remote mode uses `ADAPTER_TIMEOUT`, standard `http.Client`, and the same JSON body as local mode.
- `Path` is a path, not a raw URL. `Query` is encoded with `url.Values.Encode`; callers do not concatenate unescaped query strings.
- Remote URL construction preserves the configured base path and appends the request path without swallowing escaped path/query data.
- `X-Service-Key` is sent when `SERVICE_API_KEY` is configured.
- Scheduler wrappers continue sending `X-API-Key` in addition to `X-Service-Key`.
- Caller-supplied headers are copied before auth headers are added, so wrapper-provided trace/idempotency headers survive.
- The default response cap is 8 MiB. Wrappers may pass a smaller `ResponseLimit` to preserve existing stricter behavior, such as scheduler admission/preemption 1 MiB caps.
- Successful JSON responses decode the repository-standard `{data,error}` envelope into `Response`.
- Envelope error payloads become errors only for wrappers whose old behavior treated them as transport/application errors. Scheduler admission keeps its current behavior of mapping denial payloads and non-2xx statuses into admission results instead of returning a generic client error.
- Route-specific status handling remains in service clients. For example, org-project plan binding keeps project-not-found handling, workload clients keep invalid-manifest/status mapping, and scheduler clients keep quota/admission status semantics.

Helper replacement rules:

- `textValue(data, keys...)` copies that trim string values become `shared.TextValue`.
- `mapValue(data, keys...)` copies that accept `map[string]any` and JSON object strings become `shared.MapValue`.
- `cloneMap(map[string]any)` copies that return an empty non-nil map for nil become `shared.CloneMap`.
- `firstNonEmpty(values...)` copies that do not trim become `shared.FirstNonEmpty`.
- Trimmed `firstNonEmpty(values...)` copies become `shared.FirstNonBlank` if a shared helper is still needed after local simplification.
- Do not replace package-specific helper variants, including helpers whose inputs are not `map[string]any`, variants that intentionally preserve whitespace, or test harness helpers when replacement would make the test less readable.

Dead-code deletions are limited to:

- `envBool`, `envDuration`, and `envInt` in `backend/internal/platform/config.go`.
- `appStorageMountPlanClient` in `backend/internal/services/workload/storage_mount_client.go`.
- `preemptionDemandCanBeSatisfied` and its pre-scan call in `backend/internal/services/schedulerquota/preemption.go`.
- `ValidUntil` in the live plan-window E2E seed struct when no scenario sets it.
- `livePausePodManifest` in the live plan-window E2E when no caller uses it.

## 16. Verification Plan

Run from repository root:

- `go -C backend test ./internal/platform ./internal/services/workload ./internal/services/schedulerquota ./internal/services/authorizationpolicy ./internal/services/identity -count=1`
- `go -C backend test ./... -count=1`
- `go -C backend vet ./...`
- `go -C backend build ./...`
- `bash backend/scripts/ci-security-gate.sh quick`

SonarScanner:

- Run `bash backend/scripts/ci-security-gate.sh sonar` only if the user approves exporting repository analysis data and required Sonar credentials/environment variables are present.
- If credentials are missing or the user does not approve, record Sonar as not run with the reason.

## 17. Rollback Plan

Revert the simplification commit. If only route removal causes compatibility fallout, restore `referenceCompatRoutes`, platform gateway catch-all route specs, and legacy-only handler registrations while keeping the internal client consolidation if tests remain green.

## 18. Risks and Tradeoffs

Removing all compatibility routes is a deliberate breaking public API change. Tests that assert legacy paths must be removed or inverted. Shared internal-client plumbing reduces duplication but concentrates behavior, so package tests must cover local and remote paths.

The lowest-risk implementation path is deletion-first: remove compatibility route registration and dead helpers before broad helper consolidation. If helper replacement starts spreading beyond the files listed above, stop and leave the extra duplicate for a separate cleanup.

## 19. Reviewer Checklist

- Plan approval before code changes.
- Confirm scope excludes repository interface consolidation.
- Confirm removed routes are legacy/compat only and canonical handlers remain.
- Confirm canonical route metadata currently stranded in `referenceCompatRoutes()` is moved into the owning service spec before `referenceCompatRoutes()` is deleted.
- Confirm internal route contracts remain stable.
- Confirm service-key and scheduler `X-API-Key` authorization behavior is preserved.
- Confirm affected files and tests are explicit enough to review.
- Confirm focused, full, and quick gates pass or blockers are documented.
- Confirm SonarScanner is run only with user-approved credentials/environment.

## 20. Status

Status: Approved
