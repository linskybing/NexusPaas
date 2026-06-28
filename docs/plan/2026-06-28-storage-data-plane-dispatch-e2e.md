# Storage DataPlanePlan Dispatch E2E Contract

Date: 2026-06-28
Scope: lightweight cross-service dispatch contract E2E only

## 1. Objective

Add the next minimal P1 verification slice after the storage internal
DataPlanePlan E2E: prove workload-service dispatch consumes the storage-service
DataPlanePlan through the internal service boundary and injects the resolved
data-path pieces into the Kubernetes manifest prepared for dispatch.

This slice adds only lightweight `internal/e2e` evidence. It runs with in-memory
platform state, `httptest`, and the fake cluster client. kind is allowed but
intentionally not used here, because the immediate acceptance gap is the
cross-service dispatch contract; kind is reserved for later live mount, CSI,
StorageClass, and byte-mover slices.

## 2. Background

Already-committed evidence:

- `backend/internal/e2e/storage_data_plane_plan_e2e_test.go` covers the
  storage-service internal DataPlanePlan route, service-key auth, storage-owned
  stage-in resolution, and `DataPlanePlanBuilt`.
- `backend/internal/services/workload/dispatcher_dataplane_test.go` covers the
  local workload injection helpers for scratch volume, stage-in initContainer,
  and checkpoint env.

Remaining gap: there is no lightweight cross-service E2E proving the workload
dispatch path calls storage-service for a DataPlanePlan and applies that returned
plan to a dispatch manifest. Gap docs must still avoid claiming live data-plane
execution is proven; this slice closes only the fake-cluster dispatch contract.

## 3. Source References

- `backend/internal/e2e/storage_data_plane_plan_e2e_test.go`
- `backend/internal/services/workload/dispatcher_dataplane_test.go`
- `backend/internal/services/storage` (resolver + `DataPlanePlanBuilt`)
- `backend/internal/services/workload` (dispatch maintenance path)

## 4. Assumptions

- The storage-service DataPlanePlan route and workload dispatch injection helpers
  are already implemented.
- The fake cluster client can expose the created Pod for assertions.
- Shared in-memory store/event bus is acceptable test plumbing as long as the
  service-call boundary is exercised over HTTP.

## 5. Non-Goals

- No live kind cluster in this slice.
- No live Kubernetes API server, CSI provisioner, StorageClass binding, local
  PV, CephFS, Longhorn, or object storage dependency.
- No real byte mover, rsync/rclone/tar job, or data-copy assertion.
- No production code changes unless the new E2E exposes a real blocking defect
  and Reviewer Agent approves the fix scope.
- No new API, event, route, table, migration, or fixture contract.
- No Full GA, live storage, or live data-plane execution claim.

## 6. Current Behavior

The storage-service route and workload injection helpers are each independently
tested, but no E2E exercises the workload-to-storage dispatch contract end to end
through the internal service client.

## 7. Target Behavior

One `//go:build e2e` test (suggested
`TestWorkloadDataPlaneDispatchConsumesStoragePlanE2E`) proves workload dispatch
calls storage-service over its `httptest` URL, applies the returned DataPlanePlan
to the dispatch manifest, and that storage emitted `DataPlanePlanBuilt` — all
without kind or live infrastructure.

## 8. Affected Domains

- Cross-service E2E test and acceptance ledgers only.

## 9. Affected Files

Code Agent should touch only:

- `backend/internal/e2e/storage_data_plane_dispatch_e2e_test.go`
- `docs/acceptance/gap-analysis.md`
- `problem.md`

Any runtime file change is out of scope unless the test reveals a real defect.
If that happens, Code Agent must stop, describe the defect, and ask Reviewer
Agent to approve a narrowed runtime-fix scope before editing product code.

## 10. API / Contract Changes

None.

## 11. Database / Migration Changes

None.

## 12. Configuration Changes

None. The test uses no external env vars.

## 13. Observability Changes

None.

## 14. Security Considerations

The test exercises storage project access and workload-to-storage service-key
auth through HTTP. It must not call storage resolver functions directly or
duplicate storage-owned resolution logic inside workload assertions.

## 15. Implementation Steps

1. Create shared in-memory platform state: one `platform.NewStore()`, one
   `platform.NewEventBus()`, no `newHarness`, no external env vars.
2. Start a storage-service `platform.App` with `httptest`
   (`ServiceName: "storage-service"`, `RequireAuth: true`, a test `ServiceAPIKey`,
   `services.RegisterAll(storageApp)` so default storage profiles seed).
3. Seed only the storage records for one authorized dataset source:
   `storage_bindings`, `group_storage`, `project_storage_permissions`.
4. Start a workload-service `platform.App` sharing the same store and event bus,
   configured with `ServiceName: "workload-service"`,
   `ServiceURLs["storage-service"] = storage httptest URL`, the same
   `ServiceAPIKey`, and a fake cluster client.
5. Seed one submitted workload job with `project_id`, `user_id`, a simple Pod
   manifest, and a `data_plane` block referencing the storage binding, scratch
   profile `local-nvme-scratch`, and checkpoint target `cephfs-rwx-authority`.
6. Run the existing workload dispatch maintenance path once.
7. Assert the fake-cluster-created Pod contains scratch PVC volume + mount,
   stage-in source PVC volume + stage-in initContainer, and checkpoint env
   (scratch checkpoint directory + local-first write policy).
8. Assert storage-service emitted `DataPlanePlanBuilt`.
9. Update acceptance ledgers only after the test passes, keeping the remaining
   gap explicit: no live kind/Kubernetes execution yet; no CSI/StorageClass/local
   PV binding proof yet; no real byte mover proof yet; no Full GA claim. The
   wording must say this slice adds local/lightweight cross-service dispatch
   contract evidence.

## 16. Verification Plan

Required targeted verification:

```bash
cd backend && go test -tags e2e ./internal/e2e -run DataPlaneDispatch
cd backend && go test ./internal/services/storage/... ./internal/services/workload/...
```

Required final gates:

```bash
cd backend && go test ./...
cd backend && go build ./...
git diff --check
cd backend && make coverage
cd backend && make ci-sonar
```

Do not run kind for this slice. If the fake-cluster test cannot be expressed
without broad helpers or product-code churn, Code Agent should stop and report
the blocker instead of expanding the slice.

## 17. Rollback Plan

- Delete `backend/internal/e2e/storage_data_plane_dispatch_e2e_test.go`.
- Revert only the dispatch-evidence wording in
  `docs/acceptance/gap-analysis.md` and `problem.md`.
- No database, migration, manifest, cluster, or external-service rollback is
  needed.

## 18. Risks and Tradeoffs

- Fake-cluster evidence can be overclaimed. Keep documentation precise.
- Shared in-memory store is acceptable test plumbing, but workload assertions
  must still prove the service-call boundary by using the storage-service
  `httptest` URL.
- The existing dispatch maintenance path may need non-obvious seed fields. Keep
  the fixture minimal and copy only fields needed by existing code.
- If the fake cluster API cannot expose the created Pod cleanly, prefer one
  focused helper in the test file over product-code scaffolding.

## 19. Reviewer Checklist

- [ ] Plan file exists under `docs/plan/`.
- [ ] New E2E file is `//go:build e2e` and does not call `newHarness`.
- [ ] Test uses `httptest` storage-service URL and workload internal client
      behavior, not direct storage resolver calls.
- [ ] Test runs without `TEST_DATABASE_URL`, Redis, MinIO, kind, live
      Kubernetes, CSI, or byte mover.
- [ ] Fake-cluster Pod has scratch volume/mount, stage-in initContainer, and
      checkpoint env derived from the storage-service DataPlanePlan.
- [ ] Storage outbox/event bus contains `DataPlanePlanBuilt`.
- [ ] No runtime code changed unless Reviewer approved a defect fix.
- [ ] Docs update states local/lightweight dispatch evidence only.
- [ ] Required test/build/coverage/Sonar commands ran and results are reported.

## 20. Status

Status: Approved
