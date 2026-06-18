# Core User Journey E2E Coverage

## 1. Objective

Add missing end-to-end coverage for core user journeys: ConfigFile lifecycle plus deployment readback, ConfigFile-backed DRA dispatch, image build governance, and IDE lifecycle project access. Use the existing layered E2E model: harness-backed cross-service tests by default, opt-in live tests only for Kubernetes or external Harbor dependencies.

## 2. Background

The backend already has E2E coverage for service isolation, identity contracts, scheduler admission, storage mount-plan, GPU telemetry, LDAP/project/plan minimal K8s deploy, plan-window/runtime cleanup, priority class sync, policy ConfigMaps, Longhorn health, Docker cleanup, media upload, and notification/audit events. The uncovered user-facing paths are concentrated around DRA deployment, image build governance, IDE lifecycle, and full ConfigFile lifecycle around deploy/cancel/readback.

## 3. Source References

- `backend/internal/e2e/harness_test.go`
- `backend/internal/e2e/cross_service_e2e_test.go`
- `backend/internal/e2e/live_user_project_plan_deploy_e2e_test.go`
- `backend/internal/e2e/live_plan_window_duration_preemption_e2e_test.go`
- `backend/internal/services/workload/dispatcher_test.go`
- `backend/internal/services/imageregistry/handler_test.go`
- `backend/internal/services/ideworkspace/workflow_test.go`

## 4. Assumptions

- "Complete E2E" means core user journeys are covered at the highest practical layer, not every route running against live external systems.
- Docker Desktop Kubernetes may be mutated only by opt-in live tests using unique namespaces/resources and cleanup.
- Live DRA may skip with an explicit reason when the cluster lacks `resource.k8s.io` APIs or a compatible DRA driver; fake-client/unit DRA coverage remains required.
- Image build governance uses existing record-backed build routes; no fake or new production Harbor adapter is needed for queue/log/list/cancel behavior. Real Harbor remains opt-in for adapter boundary smoke only.

## 5. Non-Goals

- Do not change production route shapes, database schema, or business behavior.
- Do not implement a real image builder, real Harbor build pipeline, or new DRA driver support.
- Do not require OIDC/Dex authorization-code live flow in this pass.
- Do not make all E2E tests depend on live Kubernetes, LDAP, Harbor, Longhorn, or external proxy tools.

## 6. Current Behavior

Existing E2E tests prove several cross-service and live Kubernetes paths, but the required gate does not include image build governance or IDE lifecycle. DRA dispatch is covered by workload fake-client/unit tests but not by a live opt-in E2E. ConfigFile live deploy currently proves create plus minimal Job submit; it does not fully exercise update, version commit, tree/history, config-commit submit, cancel/log/GPU read guards, and non-member readback denial.

## 7. Target Behavior

- Harness E2E includes image request/build governance and IDE lifecycle project access.
- Workload E2E includes ConfigFile update/version/history/tree plus config-commit based job submit and access guard readbacks.
- Opt-in live Kubernetes E2E includes ConfigFile-backed DRA Pod dispatch and verifies ResourceClaimTemplate/Pod DRA wiring when the cluster supports DRA.
- Optional live Harbor E2E verifies the configured Harbor adapter boundary without becoming part of the default gate.

## 8. Affected Domains

- Workload service E2E coverage for ConfigFile, job submit, dispatcher, project access, and DRA.
- Image registry service E2E coverage for image requests, catalog governance, build queue/log/list/cancel, and Harbor adapter boundary.
- IDE service E2E coverage for project membership, lifecycle, validation, and projections.
- E2E documentation and gate commands.

## 9. Affected Files

- `backend/internal/e2e/image_build_governance_e2e_test.go`
- `backend/internal/e2e/ide_lifecycle_project_access_e2e_test.go`
- `backend/internal/e2e/workload_configfile_lifecycle_e2e_test.go`
- `backend/internal/e2e/live_configfile_dra_e2e_test.go`
- `backend/internal/e2e/live_harbor_image_build_e2e_test.go`, only if existing Harbor adapter configuration can support an opt-in smoke without production changes.
- `backend/docs/e2e-testing.md`
- Focused service unit tests only if an uncovered helper branch is needed to make E2E assertions stable.

## 10. API / Contract Changes

No public API changes. New tests must use existing routes and contracts:

- Workload ConfigFile member-allowed/non-member-denied routes: `GET/POST /api/v1/configfiles`, `GET/PUT/PATCH/DELETE /api/v1/configfiles/{id}`, `POST/GET /api/v1/configfiles/{id}/versions`, `GET /api/v1/configfiles/tree`, `GET /api/v1/configfiles/project/{project_id}`, `GET /api/v1/configfiles/project/{project_id}/tree`, `GET /api/v1/configfiles/project/{project_id}/history`, `GET /api/v1/projects/{id}/config-files`, `POST/DELETE /api/v1/configfiles/{id}/instance`, and `GET /api/v1/configfiles/{id}/instance/pods`.
- Workload job member-allowed/non-member-denied readback routes: `POST /api/v1/jobs`, `GET /api/v1/jobs/{id}`, `POST /api/v1/jobs/{id}/cancel`, `GET /api/v1/jobs/{id}/logs`, `GET /api/v1/jobs/{id}/gpu-summary`, `GET /api/v1/jobs/{id}/gpu-timeline`, and `GET /api/v1/jobs/{id}/gpu-breakdown`.
- Image registry: member reads project images/builds; project manager/admin creates/cancels builds; non-member is denied on `GET/POST /api/v1/projects/{id}/images`, `GET /api/v1/projects/{id}/image-requests`, `GET /api/v1/projects/{id}/builds`, `GET /api/v1/projects/{id}/image-builds`, and cancel routes. Admin-only governance remains on `/api/v1/image-requests*` approve/reject/batch and `/api/v1/image-catalog*` publish/unpublish/delete.
- IDE: project member can use `/api/v1/ide`, `/api/v1/ide/images`, `/api/v1/ide/start`, `/api/v1/ide/stop`, and `/api/v1/ide/delete`; non-member project start is denied; invalid root/executor/type/image inputs fail with existing 400/403 responses.

## 11. Database / Migration Changes

No schema or migration changes. Tests may seed existing record-store resources with E2E run IDs and must clean up through the existing harness cleanup.

## 12. Configuration Changes

- Add `TEST_LIVE_K8S_CONFIGFILE_DRA=1` for the opt-in live DRA E2E.
- Add `TEST_LIVE_HARBOR_IMAGE_BUILD=1` for optional live Harbor boundary smoke, only if a local Harbor URL/credentials are configured through existing adapter config. The default image build governance E2E must not require Harbor.
- Keep existing backing-service environment variables documented in `backend/docs/e2e-testing.md`.

## 13. Observability Changes

No production observability changes. Tests should assert relevant events where already emitted: `JobSubmitted`, `SubmitAdmissionReviewed`, `ImageRequested`, `ImageApproved`, `ImageBuildStarted`, and IDE lifecycle events when present.

## 14. Security Considerations

- E2E tests must verify non-member project access is denied for ConfigFile/job/IDE/image project routes.
- Tests must not log secrets, session tokens, API tokens, LDAP credentials, or Harbor credentials.
- Live tests must delete only unique E2E-marked namespaces/resources.

## 15. Implementation Steps

1. Add this plan and obtain Reviewer Agent approval before implementation.
2. Add harness E2E `TestImageBuildGovernanceE2E` using existing record-backed image registry routes only. Cover project image request, admin approve/reject, catalog publish/unpublish/list, context/from-storage/dockerfile build creation, build logs, project build list, cancel, and event persistence. Do not add fake Harbor adapter abstractions for this default test.
3. Add harness E2E `TestIDELifecycleProjectAccessE2E`. Seed identity/project records, verify member start/list/stop/delete, non-member denial, invalid image/type/root/executor validation, and projection/list visibility.
4. Extend workload harness/live coverage for ConfigFile lifecycle: create, update, commit version, list versions, tree/history, submit job by `config_commit_id`, and verify non-member read/deploy/cancel/log/GPU readbacks fail closed without writes.
5. Add opt-in live `TestLiveK8sConfigFileDRADispatchE2E`. Before mutation, discover DRA support by checking the API resource list for `resource.k8s.io/v1` `resourceclaimtemplates` and required dynamic client support. If unsupported, skip with a reason that names the missing API/resource. If supported, submit a ConfigFile-backed DRA Pod job with `gpu_count`, `sm_percentage`, `pinned_memory_limit`, and `device_class_name`; dispatch once; verify ResourceClaimTemplate and Pod DRA resourceClaims/labels/annotations; cleanup the unique namespace even on failure.
6. Add optional `TestLiveHarborImageBuildE2E` only if existing adapter configuration makes it possible without adding new production code. This test should be limited to Harbor status/projects/catalog adapter boundary behavior and must not claim to perform a real container build unless the existing production code already does so.
7. Update `backend/docs/e2e-testing.md` with the new required and optional commands.

## 16. Verification Plan

Focused service tests:

```sh
go -C backend test ./internal/services/workload ./internal/services/imageregistry ./internal/services/ideworkspace ./internal/services/schedulerquota ./internal/services/orgproject -count=1
```

Required E2E gate:

```sh
go -C backend test -tags e2e ./internal/e2e -run 'TestServiceRouteIsolationContract|TestServiceIsolationValidationE2E|TestIsolatedRuntimeRegistrationE2E|TestNonBlobIsolatedServiceIgnoresObjectStoreConfigE2E|TestProviderConsumerContractMatrix|TestCriticalCrossServiceJourneys|TestSchedulerAdmissionOwnerReadContractsE2E|TestStorageMountPlanContractE2E|TestImageBuildGovernanceE2E|TestIDELifecycleProjectAccessE2E|TestWorkloadConfigFileLifecycleE2E' -count=1 -v
```

Live/optional:

```sh
TEST_LIVE_K8S_CONFIGFILE_DRA=1 go -C backend test -tags e2e ./internal/e2e -run '^TestLiveK8sConfigFileDRADispatchE2E$' -count=1 -v
TEST_LIVE_HARBOR_IMAGE_BUILD=1 go -C backend test -tags e2e ./internal/e2e -run '^TestLiveHarborImageBuildE2E$' -count=1 -v
```

General gates:

```sh
go -C backend test ./... -count=1
bash backend/scripts/ci-security-gate.sh quick
```

## 17. Rollback Plan

Remove the added E2E test files/sections and E2E documentation updates. No production code or database migrations should need rollback.

## 18. Risks and Tradeoffs

- Live DRA is cluster-dependent and may skip on Docker Desktop clusters without DRA API resources or drivers; the skip must happen before creating namespace/resources.
- Image build governance E2E proves record-backed governance and build queue lifecycle, not actual container image building.
- Expanding E2E gates increases runtime; keep only harness tests in the required gate and leave live external tests opt-in.

## 19. Reviewer Checklist

- Requirement fit: P0 gaps are covered or explicitly documented as optional/live-dependent.
- Scope: no production behavior, route, or schema changes unless Reviewer approves a necessary testability fix.
- Microservice boundaries: tests use public/internal service contracts and do not depend on unauthorized shared-table reads.
- Security: non-member and bad credential paths fail closed.
- Verification: focused service tests, required E2E gate, full tests, and quick CI gate are reported.

## 20. Status

Status: Approved
