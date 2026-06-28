# Storage DataPlane Kind Admission E2E

## 1. Objective

Add one env-gated `//go:build e2e` test proving the storage DataPlane dispatch
path reaches a live Kubernetes API server and creates/admits the expected
objects for a Pod workload.

The proof is intentionally narrow: namespace creation, source PV/PVC seed,
target PV/PVC materialization through `EnsurePVCMounted`, Pod creation with
scratch/stage-in/checkpoint injection, and `DataPlanePlanBuilt` event emission.

## 2. Background

Existing coverage already proves the local storage DataPlanePlan contract and
fake-cluster workload dispatch injection:

- `backend/internal/e2e/storage_data_plane_plan_e2e_test.go`
- `backend/internal/e2e/storage_data_plane_dispatch_e2e_test.go`
- `backend/internal/e2e/storage_mount_plan_e2e_test.go`

Existing live E2E style is env-gated in
`backend/internal/e2e/live_user_project_plan_deploy_e2e_test.go` with
`TEST_LIVE_*` variables and `requireLiveKubeconfig`.

## 3. Source References

- `backend/internal/services/workload/dispatcher.go`
- `backend/internal/services/workload/dispatcher_dataplane.go`
- `backend/internal/services/workload/dispatcher_storage.go`
- `backend/internal/platform/cluster/volume_share.go`
- `backend/internal/e2e/live_user_project_plan_deploy_e2e_test.go`
- `backend/internal/e2e/storage_mount_plan_e2e_test.go`
- `backend/internal/e2e/storage_data_plane_plan_e2e_test.go`
- `backend/internal/e2e/storage_data_plane_dispatch_e2e_test.go`
- `docs/acceptance/gap-analysis.md`
- `gap.md`
- `problem.md`

## 4. Assumptions

- Branch is `storage-data-path`.
- The user allows ultra-lightweight kind for E2E evidence.
- Normal CI must not require kind or a live Kubernetes API server.
- A live Kubernetes API-admission proof is enough for this slice; workload
  scheduling, CSI mount success, and byte movement are separate gaps.
- Reusing existing E2E helpers is preferred over new scaffolding.

## 5. Non-Goals

- No claim that a CSI driver mounted data.
- No scheduler-success or Pod-running assertion.
- No local PV binding guarantee beyond API object creation/admission.
- No real byte mover, rsync/rclone/tar execution, or file-content assertion.
- No StorageClass runtime validation.
- No new production API, database schema, deployment manifest, or CI dependency.
- No Full GA or full storage GA closure.

## 6. Current Behavior

- DataPlanePlan and workload dispatch injection have local E2E/fake-client
  evidence.
- Storage mount-plan E2E can seed source PVC/PV and prove
  `EnsurePVCMounted` through a fake Kubernetes client.
- Live E2E tests exist, but none prove the storage DataPlane dispatch path
  creates its API objects against a real Kubernetes API server.

## 7. Target Behavior

With `TEST_LIVE_STORAGE_DATAPLANE_KIND_ADMISSION=1` and a kubeconfig pointing
at a disposable kind cluster, the new E2E:

1. Creates a unique namespace.
2. Seeds a source PV/PVC object accepted by the live API.
3. Starts storage-service and workload-service test apps using shared test
   state and service-key auth.
4. Seeds the minimum storage binding/group storage/permission records.
5. Submits or seeds one workload job with a simple Pod manifest and DataPlane
   request.
6. Runs workload dispatch once or until the API-created objects are visible.
7. Asserts the target PV/PVC exists because dispatch called
   `EnsurePVCMounted`.
8. Asserts the created Pod spec contains scratch volume/mount, stage-in
   initContainer, and checkpoint env.
9. Asserts `DataPlanePlanBuilt` was emitted.

The test may stop at API admission and object inspection. It must not wait for
the scheduler or containers.

## 8. Affected Domains

- `storage-service`: DataPlanePlan resolution and `DataPlanePlanBuilt` event.
- `workload-service`: dispatch consumption of the storage plan and Kubernetes
  object creation.
- `platform/cluster`: live `EnsurePVCMounted` API object materialization path.
- Acceptance ledgers: evidence wording only.

## 9. Affected Files

Code Agent may edit only:

- `backend/internal/e2e/storage_data_plane_kind_admission_e2e_test.go`
- `docs/acceptance/gap-analysis.md`
- `gap.md`
- `problem.md`

Plan artifact:

- `docs/plan/2026-06-28-storage-data-plane-kind-admission-e2e.md`

Runtime code changes are out of scope. If the live test exposes a real runtime
defect, stop and request Reviewer Agent approval for a narrowed fix plan.

## 10. API / Contract Changes

None. The test verifies existing internal/service behavior:

- storage DataPlanePlan internal service contract
- workload dispatch DataPlane consumption
- Kubernetes object creation through the existing cluster client

## 11. Database / Migration Changes

None. Use in-memory `platform.Store` records only.

## 12. Configuration Changes

No checked-in runtime config changes.

Test-only env gate:

- `TEST_LIVE_STORAGE_DATAPLANE_KIND_ADMISSION=1`
- `KUBECONFIG` or default `~/.kube/config`, validated through
  `requireLiveKubeconfig`

Optional local runner setup, not CI:

```bash
kind create cluster --name nexuspaas-storage-e2e
kubectl config use-context kind-nexuspaas-storage-e2e
```

If `kind` is not installed, do not add a repo dependency. Skip the live command
and run only non-live E2E/tests.

## 13. Observability Changes

No runtime observability changes. The test must inspect the in-memory outbox or
event bus for `DataPlanePlanBuilt`.

## 14. Security Considerations

- Use a disposable namespace with a unique suffix.
- Delete the namespace and seeded PVs in `t.Cleanup`.
- Do not use production clusters or shared namespaces.
- Do not log secrets or kubeconfig contents.
- Keep service-key auth in the test path; do not bypass the storage-service
  internal boundary.

## 15. Implementation Steps

1. Add `backend/internal/e2e/storage_data_plane_kind_admission_e2e_test.go`
   with `//go:build e2e`.
2. Gate the test on `TEST_LIVE_STORAGE_DATAPLANE_KIND_ADMISSION=1`; otherwise
   `t.Skip`.
3. Call `requireLiveKubeconfig(t)`, create `cluster.NewFromEnv("proj")`, and
   `Ping` the API.
4. Generate a short run suffix and namespace name; create namespace and register
   cleanup for namespace plus seeded PVs.
5. Seed one live source PV/PVC pair accepted by Kubernetes. Reuse the
   `e2eBoundPVC`/`e2eJuiceFSPV` shape where possible, adjusted only as needed
   for live API admission.
6. Start storage-service and workload-service apps using the existing E2E
   harness or the smallest shared in-memory app setup that preserves the
   storage-service HTTP/internal client boundary.
7. Seed only the required storage records:
   - `storage-service:storage_bindings`
   - `storage-service:group_storage`
   - `storage-service:project_storage_permissions`
8. Create one submitted workload job with a Pod manifest and DataPlane block
   referencing the seeded source and default profiles.
9. Run workload maintenance until the target PVC and Pod are visible or a short
   timeout expires.
10. Assert:
    - namespace exists;
    - source PV/PVC exists;
    - target PV/PVC exists through `EnsurePVCMounted`;
    - Pod exists;
    - Pod spec contains scratch/stage-in/checkpoint injection;
    - `DataPlanePlanBuilt` exists.
11. Update only the ledger lines listed below after the test passes.
    Ledger wording may only say "env-gated live Kubernetes API admission
    evidence for storage DataPlane dispatch". It must not say or imply CSI
    mount, scheduler success, local PV binding, byte mover, StorageClass
    runtime validation, storage GA, or Full GA.

## 16. Verification Plan

Non-live checks:

```bash
cd backend && go test -tags e2e ./internal/e2e -run 'DataPlane(Plan|Dispatch)'
cd backend && go test -tags e2e ./internal/e2e -run KindAdmission -count=1
cd backend && go test ./internal/services/storage/... ./internal/services/workload/... ./internal/platform/cluster/...
git diff --check
```

Optional live kind check:

```bash
kind create cluster --name nexuspaas-storage-e2e
kubectl config use-context kind-nexuspaas-storage-e2e
cd backend && TEST_LIVE_STORAGE_DATAPLANE_KIND_ADMISSION=1 go test -tags e2e ./internal/e2e -run KindAdmission -count=1 -v
kind delete cluster --name nexuspaas-storage-e2e
```

Final gates when feasible:

```bash
cd backend && go test ./...
cd backend && go build ./...
cd backend && make coverage
cd backend && make ci-sonar
```

If kind is unavailable locally, report the skipped optional live command and do
not mark the live evidence ledger closed.

## 17. Rollback Plan

- Delete `backend/internal/e2e/storage_data_plane_kind_admission_e2e_test.go`.
- Revert only the storage DataPlane/kind evidence wording in
  `docs/acceptance/gap-analysis.md`, `gap.md`, and `problem.md`.
- Delete the local kind cluster if created.

## 18. Risks and Tradeoffs

- Live API admission can be overclaimed. Keep docs explicit that this is not
  CSI mount, scheduler success, byte mover, or StorageClass runtime proof.
- Static local PV/PVC objects in kind are enough for API admission evidence, not
  real storage behavior.
- Env-gated kind keeps normal CI fast and dependency-free.
- Cleanup failures can leave PVs because PVs are cluster-scoped; name them with
  the run suffix and clean them explicitly.

## 19. Reviewer Checklist

- [ ] Plan exists under `docs/plan/`.
- [ ] Test is `//go:build e2e` and env-gated with
      `TEST_LIVE_STORAGE_DATAPLANE_KIND_ADMISSION=1`.
- [ ] Normal CI and non-live E2E do not require kind.
- [ ] Test uses live Kubernetes API via `cluster.NewFromEnv` and
      `requireLiveKubeconfig`.
- [ ] Test creates namespace, source PV/PVC, target PV/PVC, and Pod through the
      API server.
- [ ] Target PVC is created through `EnsurePVCMounted`, not direct test setup.
- [ ] Pod assertions cover scratch, stage-in, and checkpoint injection.
- [ ] `DataPlanePlanBuilt` is asserted.
- [ ] No runtime/test code beyond the listed E2E file is changed.
- [ ] Ledgers do not claim CSI mount, scheduler success, local PV binding,
      byte mover behavior, StorageClass runtime validation, storage GA, or Full
      GA.
- [ ] Verification commands and any skipped live-kind reason are reported.

## 20. Status

Status: Approved

## AC / Gap Ledger Lines To Update

After the live kind test passes, update only:

- `docs/acceptance/gap-analysis.md`: in the `STORAGE` row/evidence text, add
  exactly one scoped evidence bullet:
  - `Env-gated live Kubernetes API admission evidence for storage DataPlane dispatch exists via backend/internal/e2e/storage_data_plane_kind_admission_e2e_test.go; remaining gaps stay open for CSI mount, scheduler success, local PV binding, byte mover behavior, StorageClass runtime validation, storage GA, and Full GA.`
- `gap.md`: in the storage binding/mount-plan or storage evidence row, add only
  this evidence id and scoped note:
  - `evidence id 2026-06-28-storage-data-plane-kind-admission-e2e: env-gated live Kubernetes API admission evidence for storage DataPlane dispatch only; no CSI mount, scheduler success, local PV binding, byte mover behavior, StorageClass runtime validation, storage GA, or Full GA claim.`
- `problem.md`: in the storage DataPlane paragraph near the existing
  `DataPlanePlanBuilt`/dispatch evidence, add only this scoped sentence:
  - `An env-gated live Kubernetes API admission E2E now covers storage DataPlane dispatch object creation only; it does not prove CSI mount, scheduler success, local PV binding, byte mover behavior, StorageClass runtime validation, storage GA, or Full GA.`

Hard non-overclaim rule: ledger wording may only describe this slice as
"env-gated live Kubernetes API admission evidence for storage DataPlane
dispatch". Do not use broader phrases such as live data-plane execution, live
mount execution, storage readiness, storage GA, or GA closure.

## Code Agent Instructions

- Implement only the approved plan.
- Prefer reusing existing E2E helpers and seeded object shapes.
- Do not add a Makefile target, CI job, dependency, or runtime config.
- Keep the test one scenario; no table suite.
- If live API admission needs broad runtime changes, stop and report the
  blocker instead of expanding scope.
