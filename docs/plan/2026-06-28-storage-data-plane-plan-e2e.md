# Storage DataPlanePlan E2E Contract Case

## 1. Objective

Add the next P1 storage verification slice: a lightweight `internal/e2e`
contract test for the storage-service internal DataPlanePlan endpoint.

The required implementation adds
`backend/internal/e2e/storage_data_plane_plan_e2e_test.go` with
`//go:build e2e`, runs without Postgres, Redis, MinIO, kind, or a live
Kubernetes cluster, and records local/lightweight evidence in the acceptance
ledgers after the test exists.

## 2. Background

The approved P1 storage data-path plan includes the internal storage
DataPlanePlan contract and an E2E case alongside the existing mount-plan E2E.
The implementation appears present: storage profiles, data-plane resolver,
workload data-plane client/injection, cache bindings, benchmark records,
FastTransfer state, and HPC manifests already exist.

The current `storage_mount_plan_e2e_test.go` proves the adjacent mount-plan
contract, but it uses the shared `newHarness`, which requires
`TEST_DATABASE_URL`, Redis, and MinIO. This slice should be smaller: use
`platform.NewApp`, the default in-memory store/event bus or an explicitly shared
in-memory store, and `httptest` only where an internal service URL is needed.

## 3. Source References

- `backend/internal/services/storage/spec.go`
  - declares `POST /internal/storage/projects/{project_id}/data-plane-plan`
  - declares `DataPlanePlanBuilt`
- `backend/internal/services/storage/data_plane_contracts.go`
  - `resolveStorageDataPlanePlanContract`
  - `storageDataPlanePlanPayload`
  - `resolveStorageDataPlaneStageIn`
- `backend/internal/services/storage/mount_plan_contracts.go`
  - `requireStorageServiceAuth`
  - storage-owned binding/source/permission lookup pattern
- `backend/internal/services/storage/storage_profiles.go`
  - seeded defaults including `local-nvme-scratch` and
    `cephfs-rwx-authority`
- `backend/internal/e2e/storage_mount_plan_e2e_test.go`
  - existing adjacent E2E style and storage seed shape
- `backend/internal/e2e/harness_test.go`
  - shows why the shared harness is too heavy for this slice
- `backend/internal/platform/app.go`, `store.go`, `events.go`, `ports.go`
  - in-memory `NewApp`, `NewStore`, `NewEventBus`, `WithStore`,
    `WithEventBus`
- `backend/internal/services/workload/dispatcher_dataplane_test.go`
  - expected pod-level scratch, stage-in initContainer, and checkpoint env
    behavior if the optional dispatch assertion stays small
- `docs/acceptance/gap-analysis.md`
- `problem.md`

## 4. Assumptions

- The current branch is `storage-data-path`.
- Storage-service registration seeds the default storage profiles when
  `ServiceName` allows `storage-service`.
- The endpoint remains service-key protected through `X-Service-Key` and
  `Config.ServiceAPIKey`.
- Local in-memory evidence is acceptable for this slice.
- A live kind/Kubernetes proof is useful later but is not part of this slice.
- The test may create small local seed helpers in package `e2e` because
  service-local storage helpers are not exported.

## 5. Non-Goals

- No runtime code changes unless the E2E exposes a real blocking bug.
- No kind cluster creation or live Kubernetes dependency.
- No deploy manifest changes.
- No API fixture changes.
- No broad storage refactor.
- No byte mover, CSI, live StorageClass, or real data-copy proof.
- No Full GA or storage GA closure claim.

## 6. Current Behavior

- The storage DataPlanePlan endpoint and event are declared in
  `storage.Spec()`.
- Storage default profiles are seeded by storage-service registration.
- The resolver builds scratch, stage-in, and checkpoint sections from
  storage-owned records.
- The adjacent mount-plan E2E covers service-key behavior and storage-owned
  source details, but depends on external services through `newHarness`.
- Workload data-plane injection has focused package tests, but there is no
  lightweight `internal/e2e` contract case for the storage internal endpoint.

## 7. Target Behavior

- `go test -tags e2e ./internal/e2e -run DataPlanePlan` runs locally without
  `TEST_DATABASE_URL`, Redis, MinIO, kind, or a live cluster.
- A wrong service key returns `401`.
- A valid request to
  `/internal/storage/projects/{project_id}/data-plane-plan` returns:
  - scratch profile `local-nvme-scratch`;
  - a scratch claim derived from the job/project identity;
  - one stage-in operation using storage-owned binding/source details;
  - checkpoint authority profile `cephfs-rwx-authority`;
  - a `DataPlanePlanBuilt` event in the storage app outbox.
- If it stays small, the same test file may also exercise workload dispatch
  through an `httptest` storage-service URL and fake cluster, asserting the
  created Pod has scratch volume/mount, stage-in initContainer, and checkpoint
  env. If this materially increases scope, leave dispatch E2E as a follow-up.
- Acceptance docs record this as local/lightweight contract evidence only.

## 8. Affected Domains

- `storage-service`: internal DataPlanePlan contract, storage-owned source
  resolution, seeded profile defaults, outbox event evidence.
- `workload-service`: optional consumer integration only if it remains a small
  fake-cluster assertion.
- Acceptance documentation: local evidence status only.

## 9. Affected Files

Implementation slice:

- `backend/internal/e2e/storage_data_plane_plan_e2e_test.go`
- `docs/acceptance/gap-analysis.md`
- `problem.md`

Plan artifact:

- `docs/plan/2026-06-28-storage-data-plane-plan-e2e.md`

No other files are expected. Runtime files may change only if the new E2E
exposes a real defect and Reviewer Agent approves the fix scope.

## 10. API / Contract Changes

None intended. The test verifies the existing internal contract:

- `POST /internal/storage/projects/{project_id}/data-plane-plan`
- service auth through `X-Service-Key`
- response envelope `data.scratch`, `data.stage_in_operations`, and
  `data.checkpoint`
- outbox event `DataPlanePlanBuilt`

## 11. Database / Migration Changes

None. The test must use the in-memory `platform.Store` and create only test
records for:

- `storage-service:storage_bindings`
- `storage-service:group_storage`
- `storage-service:project_storage_permissions`

Default `storage-service:storage_profiles` should come from storage-service
registration.

## 12. Configuration Changes

None. Test-only app config should be local to the new E2E file:

- `ServiceName: "storage-service"` for the storage app
- `RequireAuth: true`
- `ServiceAPIKey: <test service key>`
- minimal `APIKeys` only if needed by existing platform auth plumbing
- `ServiceURLs` only for the optional workload dispatch path

## 13. Observability Changes

No runtime observability changes. The test must assert that the storage app
outbox contains `DataPlanePlanBuilt` with storage-service source metadata and
data matching the project/job/scratch/checkpoint shape.

## 14. Security Considerations

- The wrong-key request must return `401`.
- The valid request must use `X-Service-Key`, not a user API key.
- The test must prove the resolver uses storage-owned binding/source records,
  not caller-forged source PVC details.
- No secrets, credentials, or external service endpoints are introduced.

## 15. Implementation Steps

1. Add `backend/internal/e2e/storage_data_plane_plan_e2e_test.go` with
   `//go:build e2e` and package `e2e`.
2. Build a tiny local app helper in that file:
   - create one `platform.NewStore()` and `platform.NewEventBus()`;
   - create `platform.NewApp(storage config, platform.WithStore(store),
     platform.WithEventBus(events))`;
   - call `services.RegisterAll(app)`;
   - do not call `newHarness`.
3. Add local seed helpers for one project, group, user, binding, group storage
   source, and read permission. Keep the seed payload close to the existing
   mount-plan E2E fields:
   - binding has `project_id`, `group_id`, `pvc_id`, `target_pvc`;
   - group source has `group_id`, `pvc_id`, `status: running`, `namespace`,
     `source_pvc`;
   - permission has `project_id`, `pvc_id`, `user_id`,
     `permission: read_only`.
4. Send the wrong-key request to the storage app through `httptest` and assert
   `401`.
5. Send the valid request with `X-Service-Key` and assert the response envelope:
   - `scratch.profile_id == "local-nvme-scratch"`;
   - `scratch.storage_class_name == "local-nvme-scratch"`;
   - `scratch.claim_name` is present and derived from the job/project identity;
   - `len(stage_in_operations) == 1`;
   - stage-in source namespace/source PVC/target PVC equal the seeded
     storage-owned records;
   - `checkpoint.flush_target_profile_id == "cephfs-rwx-authority"`;
   - `checkpoint.storage_class_name == "cephfs-rwx-authority"`.
6. Assert the storage event bus outbox contains `DataPlanePlanBuilt` with
   `project_id`, `job_id`, `scratch_profile`, `checkpoint_profile`, and
   `dataset_source_count == 1`.
7. Optional, only if still small: start the storage app with `httptest.Server`,
   create a workload app sharing the same in-memory store and a fake cluster,
   seed a submitted `data_plane` job with a simple Pod manifest, run workload
   maintenance once, and assert the created Pod has:
   - scratch PVC volume and mount;
   - stage-in initContainer;
   - checkpoint env vars.
   If this needs broad helpers or runtime changes, skip it and add a follow-up
   note in the docs evidence wording.
8. Update `docs/acceptance/gap-analysis.md` and `problem.md` after the test is
   implemented:
   - record local/lightweight `internal/e2e` DataPlanePlan evidence;
   - explicitly state this is not live kind, real Kubernetes, CSI,
     byte-mover, or Full GA evidence.

## 16. Verification Plan

Required for the implementation review:

- `cd backend && go test -tags e2e ./internal/e2e -run DataPlanePlan`
- `cd backend && go test ./internal/services/storage/... ./internal/services/workload/...`
- `cd backend && go test ./...`
- `cd backend && go build ./...`
- `git diff --check`
- `cd backend && make coverage`
- `cd backend && make ci-sonar`

If optional dispatch E2E is included, the targeted e2e command must prove both
the endpoint assertions and workload fake-cluster assertions.

## 17. Rollback Plan

- Delete `backend/internal/e2e/storage_data_plane_plan_e2e_test.go`.
- Revert only the DataPlanePlan evidence wording in
  `docs/acceptance/gap-analysis.md` and `problem.md`.
- If any runtime fix was approved because the test exposed a real bug, revert
  that fix separately with its focused test evidence.

No database, migration, manifest, or external service state rollback is needed.

## 18. Risks and Tradeoffs

- In-memory E2E does not prove live Kubernetes, CSI, StorageClass binding, or
  byte movement. That is intentional for this P1 verification slice.
- Reusing the full e2e harness would prove more topology but would reintroduce
  external dependencies; the lightweight helper is the smaller contract check.
- Optional workload dispatch coverage could make the slice noisy. Keep the
  storage endpoint case mandatory and defer dispatch E2E if it stops being a
  small fake-cluster addition.
- Documentation wording can overclaim. Keep every evidence update explicitly
  local/lightweight and leave kind/live storage as a gated follow-up.

## 19. Reviewer Checklist

- [ ] New E2E file exists under `backend/internal/e2e/` with `//go:build e2e`.
- [ ] Test does not call `newHarness` and does not require
      `TEST_DATABASE_URL`, Redis, MinIO, kind, or live Kubernetes.
- [ ] Wrong service key returns `401`.
- [ ] Good request validates scratch, stage-in, checkpoint, and
      `DataPlanePlanBuilt` outbox event.
- [ ] Seeded storage-owned records, not caller-provided source details, drive
      the stage-in operation.
- [ ] Optional workload dispatch check, if present, uses only `httptest`,
      shared in-memory store, and fake cluster.
- [ ] No runtime code changed unless a real defect was found and explained.
- [ ] `docs/acceptance/gap-analysis.md` and `problem.md` state local/lightweight
      evidence only and do not claim live kind, CSI, byte mover, Full GA, or
      real Kubernetes proof.
- [ ] Required test/build/coverage/Sonar commands are run and results reported.

## 20. Status

Status: Approved
