# Scheduler Quota Boundary Cleanup

## 1. Objective

Retire scheduler-quota's remaining shared-store dependency declarations on
org-project and workload data while preserving fail-fast production startup and
runtime behavior through explicit owner read contracts.

## 2. Background

The Production Beta roadmap identifies scheduler-quota as the next data-boundary
blocker: it reads org-project project/member/quota/group data and workload job
data to evaluate submit admission, derive live quota responses, and run quota
maintenance. The provider services already expose service-key-gated internal
read contracts, and isolated runtimes can route read-only access through
`SERVICE_URLS` plus `SERVICE_API_KEY`. The remaining gap is that catalog still
models these reads as `storeDependencies`, which keeps them classified as
shared-store transition debt.

## 3. Source References

- `long-term.md`
- `problem.md`
- `docs/agents/workflow.md`
- `docs/agents/planning.md`
- `docs/agents/review-checklist.md`
- `backend/internal/platform/service_isolation.go`
- `backend/internal/platform/service_client.go`
- `backend/internal/platform/read_contract.go`
- `backend/internal/services/catalog.go`
- `backend/internal/services/service_isolation_test.go`
- `backend/internal/services/service_dependency_inventory_test.go`
- `backend/internal/services/schedulerquota/read_contracts.go`
- `backend/internal/services/schedulerquota/admission.go`
- `backend/internal/services/schedulerquota/handler.go`
- `backend/internal/services/schedulerquota/plan_window_reaper.go`
- `backend/internal/services/schedulerquota/resource_quota_reconciler.go`
- `backend/internal/e2e/cross_service_e2e_test.go`

## 4. Assumptions

- This roadmap slice is stacked on the predecessor CI/security quality-gate
  branch `feature/ci-security-quality-gate` until that branch is merged or this
  branch is retargeted.
- Org-project and workload remain the authoritative owners for their data.
- The existing internal read contracts are sufficient for the scheduler-quota
  reads in this slice.
- Production Beta may use the current scoped `SERVICE_API_KEY` for internal
  reads; mTLS/workload identity remains GA hardening.
- Maintenance tasks may still no-op in degraded local mode when owner reads are
  unavailable, but production startup must fail when required owner read config
  is absent.

## 5. Non-Goals

- Do not change public HTTP API shape or event schema.
- Do not add database migrations or move physical tables.
- Do not build event-fed read models in this PR.
- Do not remove unrelated shared-store/cohosted-only fallback classifications
  from other services.
- Do not introduce service mesh, mTLS, or workload identity.
- Do not merge or retarget earlier stacked PRs.

## 6. Current Behavior

`serviceStoreDependencies()` declares scheduler-quota dependencies on:

- `org-project-service:projects`
- `org-project-service:project_members`
- `org-project-service:user_groups`
- `org-project-service:user_quotas`
- `workload-service:jobs`

`ValidateServiceIsolation` uses those declarations to fail isolated production
startup unless matching owner URLs and a service key are configured. Runtime
reads currently flow through `app.Store`, which may be decorated by
`crossServiceStore` in isolated mode, but the catalog still describes the
relationship as shared-store debt.

## 7. Target Behavior

Scheduler-quota declares these relationships as owner read dependencies, not
generic store dependencies. Production startup still fails without the required
owner URLs/service key or without a registered domain read contract.

Scheduler-quota runtime code performs org-project/workload reads through a
small owner-read boundary that uses local co-hosted reads when the owner is in
process and service-to-service read contracts when the owner is isolated.
Scheduler-owned plan/queue/live-quota data continues to use the scheduler
repository.

## 8. Affected Domains

- Scheduler quota admission and quota maintenance
- Platform service isolation validation
- Service dependency inventory
- Microservice data-boundary documentation

## 9. Affected Files

- `backend/internal/platform/app.go`
- `backend/internal/platform/service_isolation.go`
- `backend/internal/platform/service_isolation_test.go`
- `backend/internal/services/catalog.go`
- `backend/internal/services/service_isolation_test.go`
- `backend/internal/services/service_dependency_inventory_test.go`
- `backend/internal/services/schedulerquota/read_contracts.go`
- `backend/internal/services/schedulerquota/admission.go`
- `backend/internal/services/schedulerquota/handler.go`
- `backend/internal/services/schedulerquota/plan_window_reaper.go`
- `backend/internal/services/schedulerquota/resource_quota_reconciler.go`
- `backend/internal/services/schedulerquota/admission_test.go`
- `backend/internal/services/schedulerquota/handler_test.go`
- `backend/internal/services/schedulerquota/plan_window_reaper_test.go`
- `backend/internal/services/schedulerquota/resource_quota_reconciler_test.go`
- `backend/internal/services/schedulerquota/read_contracts_test.go`
- `backend/internal/services/workload/job_submit_test.go`
- `backend/scripts/ci-security-gate.sh` (only if the stacked quality gate
  exposes a new Sonar blocker that prevents this PR from reaching a clean gate)
- `problem.md`
- this plan file

## 10. API / Contract Changes

No public API changes. No new provider endpoints are expected.

Internally, platform gains an owner-read dependency registration API used for
startup validation. It reuses existing domain read contracts and
`SERVICE_URLS`/`SERVICE_API_KEY`; it does not expose a generic records fallback.

## 11. Database / Migration Changes

None.

## 12. Configuration Changes

No new environment variables. Existing isolated scheduler-quota deployments
must keep:

- `SERVICE_URLS` entries for `org-project-service` and `workload-service`
- `SERVICE_API_KEY`

## 13. Observability Changes

Keep existing logging behavior for owner-read failures. Do not add new metrics
in this slice. The service-to-service HTTP client already uses the platform's
OpenTelemetry transport.

## 14. Security Considerations

- Internal owner reads remain service-key-gated.
- Missing or wrong `SERVICE_API_KEY` must fail closed.
- Do not commit any service key or secret value.
- Removing store dependency declarations must not permit a production isolated
  scheduler-quota process to start without owner-read auth/config.

## 15. Implementation Steps

1. Add platform support for owner-read dependencies alongside existing generic
   store dependencies. `ValidateServiceIsolation` must validate both, but report
   generic store dependencies distinctly from owner read dependencies.
2. Register scheduler-quota's org-project/workload relationships through the new
   owner-read dependency list and remove them from `serviceStoreDependencies()`.
3. Update service dependency inventory tests so owner-read dependencies classify
   cross-service resource literals without treating them as shared-store debt.
4. Add/adjust isolation tests proving:
   - scheduler-quota has no cross-service `storeDependencies`
   - scheduler-quota has the expected owner-read dependencies
   - isolated scheduler-quota fails without owner URLs/service key
   - isolated scheduler-quota passes with org-project/workload owner contracts
   - an unrelated identity URL does not satisfy scheduler-quota dependencies
5. Add a scheduler-quota owner-read boundary that reads foreign resources via
   owner contracts when the owner is isolated and uses local store reads only
   when the owner is co-hosted.
6. Route submit admission, project queue lookup, live quota derivation, plan
   window reaping, and resource quota reconciliation through that boundary.
7. Keep scheduler-owned reads/writes on the scheduler repository.
8. Update the workload remote-scheduler submit test so the scheduler under test
   reaches org-project/workload data through owner read endpoints instead of
   same-process seeded foreign records.
9. If SonarScanner fails on a new violation inherited from the stacked
   CI/security quality-gate branch, apply the smallest gate-script fix needed
   for this branch to pass Sonar without weakening the quality gate.
10. Run `gofmt` on touched Go files after implementation.
11. Update `problem.md` so the scheduler-quota shared-store blocker is marked
   resolved or narrowed, while preserving unrelated blockers.

## 16. Verification Plan

- `cd backend && test -z "$(gofmt -l internal/platform/app.go internal/platform/service_isolation.go internal/platform/service_isolation_test.go internal/services/catalog.go internal/services/service_isolation_test.go internal/services/service_dependency_inventory_test.go internal/services/schedulerquota/*.go)"`
- `cd backend && go test ./internal/platform -run 'ServiceIsolation|RemoteServiceReader|CrossServiceStore' -count=1`
- `cd backend && go test ./internal/services -run 'ServiceIsolation|ServiceResourceConstants|ServiceStoreDependency|SchedulerQuota' -count=1`
- `cd backend && go test ./internal/services/schedulerquota -count=1`
- `cd backend && go test ./internal/services/workload -run TestSubmitJobUsesRemoteSchedulerAdmissionWhenIsolated -count=1`
- `cd backend && go test -tags e2e ./internal/e2e -run TestSchedulerAdmissionOwnerReadContractsE2E -count=1 -v`
- `bash backend/scripts/ci-security-gate.sh quick`
- `bash backend/scripts/ci-security-gate.sh docker`
- If local scanner/Sonar config remains available from PR #4:
  - `bash backend/scripts/ci-security-gate.sh security`
  - `bash backend/scripts/ci-security-gate.sh sonar`

## 17. Rollback Plan

Revert the owner-read dependency registration API, restore scheduler-quota's
entries in `serviceStoreDependencies()`, and revert scheduler-quota call sites
to their previous store-backed reader. This returns startup validation and
runtime behavior to the prior shared-store transition model without schema or
API rollback.

## 18. Risks and Tradeoffs

- This keeps synchronous owner HTTP reads as a Production Beta compromise; event
  read models remain the preferred GA direction for hot paths and maintenance.
- The owner-read boundary still handles map-shaped records because the current
  contracts expose `platform.RecordStore` record envelopes. Typed DTO contracts
  can be introduced later without changing the external provider endpoints.
- Failing owner reads currently look like missing data to existing scheduler
  policy paths. This preserves behavior but may need richer 503 handling in a
  future reliability hardening PR.
- The PR intentionally leaves unrelated co-hosted-only fallback debt untouched.

## 19. Reviewer Checklist

- Scope is limited to scheduler-quota owner-read boundary cleanup.
- No public API or migration is introduced.
- Production isolated startup still fails without required owner-read config.
- Scheduler-quota no longer registers cross-service generic store dependencies.
- Existing owner read contracts remain service-key-gated and fail closed.
- Tests prove both startup validation and runtime owner-read behavior.
- `problem.md` accurately reflects remaining launch blockers.

## 20. Status

Status: Draft
