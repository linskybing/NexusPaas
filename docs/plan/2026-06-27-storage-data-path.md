# P1 Storage Data Path

## 1. Objective

Make the storage data path a first-class platform concern by adding storage profiles, data-plane planning, FastTransfer lifecycle tracking, cache bindings, benchmark records, and HPC storage manifests. The implementation lands as stacked reviewable PRs on `storage-data-path`; each slice must build and test green before the next one.

## 2. Background

Training hot I/O and checkpoint writes should not default to Longhorn RWX as the live path. The current repository uses one generic `platform_records` store and declarative CRUD through `ServiceSpec.Routes`, so new storage entities are resource keys plus contracts, not per-entity SQL DDL.

## 3. Source References

- `backend/internal/platform/crud.go`: generic CRUD, required fields, field schema checks, CRUD event publication.
- `backend/internal/services/storage/spec.go`: storage-service route, table, and event contract surface.
- `backend/internal/services/storage/handler.go`: storage-service custom handler registration.
- `backend/internal/services/storage/storage_repository.go`: storage-owned record store helpers.
- `backend/internal/services/storage/mount_plan_contracts.go`: existing internal mount-plan resolver and permission checks.
- `backend/internal/services/workload/dispatcher.go`: workload dispatch flow.
- `backend/internal/services/workload/dispatcher_storage.go`: existing storage mount injection helpers.
- `backend/internal/services/workload/storage_mount_client.go`: internal storage-service client pattern.
- `backend/internal/services/workload/job_submit.go`: job submission payload preservation and idempotency pattern.
- `docs/agents/*.md`: required plan, review, coding, and project-structure workflow.

## 4. Assumptions

- Storage-service owns storage profile, transfer, cache binding, and benchmark metadata.
- Workload-service may consume storage-service internal planning contracts but must not write storage-owned records.
- `platform_records` remains the durable store for these entities.
- The first code slice is PR1. Later PRs must be implemented only after Reviewer Agent approval of the prior slice.
- Existing Longhorn and mount-plan behavior must remain backward compatible when `data_plane` is absent.

## 5. Non-Goals

- No custom byte mover in Go; transfer execution shells out through Kubernetes Jobs using proven tools.
- No Lustre, GPUDirect Storage, DAOS, Weka, or VAST profiles until hardware exists.
- No scheduler, network, or image-control-plane redesign.
- No new storage tables or migrations for entity records.
- No frontend work in this phase.

## 6. Current Behavior

- Storage entities are persisted as JSON records in `platform_records`.
- Storage-service already supports project storage bindings, permissions, mount-plan resolution, and a basic FastTransfer record path.
- Workload dispatch resolves storage mount plans and injects PVC mounts only when jobs declare storage mounts.
- Longhorn RWX can still become the mounted hot path for training I/O.

## 7. Target Behavior

- Admins can classify real storage backends through `StorageProfile` records.
- Jobs that declare `data_plane` resolve a `DataPlanePlan` that stages datasets to scratch and writes checkpoints local-first with authority-tier flush metadata.
- FastTransfer records expose a resumable lifecycle instead of a single staged state.
- Cache bindings record project/dataset scratch-cache hints.
- Benchmark records persist measurable per-profile baselines.
- HPC deployment manifests provide the StorageClasses referenced by seeded profiles.

## 8. Affected Domains

- `storage-service`: owns profiles, data-plane planning, transfer state, cache bindings, benchmarks, and storage contract events.
- `workload-service`: consumes DataPlanePlan during dispatch and injects runtime pod/VCJob resources.
- `k8s-control-service`: later transfer mover Job emission point for PR3.
- `deploy/hpc/storage`: manifests for non-dev storage classes.

## 9. Affected Files

PR1:
- `backend/internal/services/storage/spec.go`
- `backend/internal/services/storage/handler.go`
- `backend/internal/services/storage/storage_profiles.go`
- `backend/internal/services/storage/storage_profiles_test.go`
- `backend/internal/services/storage/storage_repository.go` only if the generic helpers are insufficient
- `backend/internal/contracts/fixtures/api/v1/*.json`
- `backend/internal/contracts/fixtures/events/v1/*.json`

PR2:
- `backend/internal/services/storage/data_plane_contracts.go`
- `backend/internal/services/storage/spec.go`
- `backend/internal/services/workload/dataplane_client.go`
- `backend/internal/services/workload/dispatcher_dataplane.go`
- `backend/internal/services/workload/dispatcher.go`
- `backend/internal/services/workload/job_submit.go`
- related storage/workload tests and contract fixtures

PR3:
- `backend/internal/services/storage/handler.go`
- `backend/internal/services/storage/storage_repository.go`
- `backend/internal/services/storage/spec.go`
- `backend/internal/services/storage/fast_transfer_state.go`
- related FastTransfer tests and fixtures

PR4:
- `backend/internal/services/storage/spec.go`
- `backend/internal/services/storage/data_plane_contracts.go`
- `backend/internal/services/storage/cache_binding_test.go`
- related API/event fixtures

PR5:
- `backend/internal/services/storage/spec.go`
- `backend/internal/services/storage/benchmark_record_test.go`
- related API/event fixtures

PR6:
- `backend/deploy/hpc/storage/*.yaml`
- `backend/deploy/hpc/storage/README.md`

## 10. API / Contract Changes

PR1 adds admin CRUD:
- `GET /api/v1/storage-profiles`
- `POST /api/v1/storage-profiles`
- `GET /api/v1/storage-profiles/{id}`
- `PUT /api/v1/storage-profiles/{id}`
- `DELETE /api/v1/storage-profiles/{id}`

PR2 adds internal service-key API:
- `POST /internal/storage/projects/{project_id}/data-plane-plan`

PR3 extends transfer APIs with progress/status transitions while keeping existing fast-stage/get/cancel paths.

PR4 adds project-scoped cache binding CRUD under:
- `/api/v1/projects/{id}/storage/cache-bindings`

PR5 adds benchmark record create/list under:
- `/api/v1/storage/benchmark-records`

Each new route must appear in `Spec().Routes`; each new resource/event must be represented in fixtures where existing contract gates require it.

## 11. Database / Migration Changes

No per-entity SQL DDL. New durable record resources:
- `storage-service:storage_profiles`
- `storage-service:cache_bindings`
- `storage-service:storage_benchmark_records`

FastTransfer remains the existing storage-owned transfer resource and is updated in-place.

## 12. Configuration Changes

No new required environment variables in PR1. PR2 uses the existing service URL and service API key internal client pattern. PR6 adds HPC manifests only and leaves `deploy/k3s` untouched.

## 13. Observability Changes

- Emit storage profile, data-plane, transfer, cache binding, and benchmark events through existing event helpers.
- Keep startup seed behavior idempotent and quiet unless an error occurs.
- Log-only drift warnings are allowed for profile-to-StorageClass mismatch; startup must not fail on optional class absence.

## 14. Security Considerations

- Admin-only external profile and benchmark writes.
- DataPlanePlan uses the same service-key guard as mount-plan.
- DataPlanePlan validates binding and effective permission server-side; it must not trust job-submitted source claims.
- Workload-service consumes plans but does not write storage-owned records.
- No secrets are introduced in StorageProfile payloads.

## 15. Implementation Steps

1. PR1: add `StorageProfile` CRUD and seeded defaults.
   - Add `storage_profiles` to storage `Spec().Tables`.
   - Add admin CRUD routes to `Spec().Routes`.
   - Register required fields for `storage-service:storage_profiles`.
   - Add `seedDefaultStorageProfiles(app)` called from `storage.Register`.
   - Seed `longhorn-rwx-standard`, `cephfs-rwx-authority`, `local-nvme-scratch`, and `minio-artifact`.
   - Add focused tests for idempotent seed, required-field rejection, and admin guard.

2. PR2: add DataPlanePlan resolver and workload dispatch injection.
   - Mirror mount-plan service-key contract.
   - Resolve scratch and checkpoint profiles from PR1 records.
   - Reuse project binding and permission checks.
   - Add workload internal client and dispatch resource injection.
   - Keep `data_plane` absent as a no-op.

3. PR3: upgrade FastTransfer state.
   - Add legal transitions and progress monotonicity.
   - Keep existing public transfer endpoints compatible.
   - Deduplicate on idempotency key using the existing submit hashing style.
   - Shell out through a Kubernetes Job; do not implement byte moving in storage-service.

4. PR4: add CacheBinding.
   - Add declarative project-scoped CRUD.
   - Make DataPlanePlan treat an existing matching binding as a cache hit.

5. PR5: add StorageBenchmarkRecord.
   - Add create/list CRUD.
   - Require `storage_profile`.

6. PR6: add HPC storage manifests.
   - Add only `backend/deploy/hpc/storage`.
   - Align StorageClass names with seeded profiles.

## 16. Verification Plan

Per PR:
- `cd backend && go test ./internal/services/storage/...`
- `cd backend && go test ./internal/services/workload/...` when workload files change
- `cd backend && go test ./internal/contracts/...`
- `cd backend && go test ./internal/services/...`
- `cd backend && go build ./...`
- Run SonarScanner per `CLAUDE.md` before Reviewer Agent sign-off.

Additional checks:
- PR2: assert a `data_plane` job injects scratch volume, stage-in initContainer, and checkpoint env; assert absent `data_plane` is no-op.
- PR3: assert legal transfer transitions, monotonic progress, and idempotency behavior.
- PR6: `kubectl apply --dry-run=client -f backend/deploy/hpc/storage/`

## 17. Rollback Plan

- PR1: remove the new routes, table/event entries, seed call, seed helper, tests, and fixtures. Existing storage records remain ignored if present.
- PR2: remove DataPlanePlan route/client/injection. Existing job behavior remains because `data_plane` is additive.
- PR3: keep compatibility by accepting old transfer reads; rollback removes only new transition endpoints and state helpers.
- PR4/PR5: remove declarative routes/resources/events; leftover generic records are inert.
- PR6: delete the HPC storage manifest directory.

## 18. Risks and Tradeoffs

- Generic CRUD event names may not match desired high-level `Changed` event names; use custom event publication only where contract tests require it.
- DataPlanePlan correctness depends on server-side binding and permission checks; reusing mount-plan logic keeps the trust boundary small.
- FastTransfer mover integration is the largest unknown. If k8s-control Job emission is not ready, ship the state machine and keep the mover behind an explicit stub.
- Cache hit means matching binding exists in PR4; actual node residency checks are a follow-up.

## 19. Reviewer Checklist

- Requirement fit: all requested section-9.1 surfaces are represented.
- Scope control: each PR is independently reviewable and rollbackable.
- Architecture: storage owns storage data; workload consumes internal contracts only.
- API contract: all routes/events/resources are declared in `Spec()` and fixtures.
- Data ownership: no new cross-service writable storage.
- Config: no new mandatory config in PR1.
- Observability: events are emitted for state changes.
- Security: admin and service-key guards are preserved.
- Testing: focused unit and contract tests exist for every changed behavior.
- Simplicity: no new framework, shared SDK, or byte mover implementation.

## 20. Status

Status: Approved
