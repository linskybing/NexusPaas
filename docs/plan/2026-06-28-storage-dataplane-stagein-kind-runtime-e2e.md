# Storage DataPlane Stage-In Kind Runtime E2E

Date: 2026-06-28
Status: Approved

## 1. Objective

Add a narrow env-gated kind runtime proof that workload-service DataPlane
stage-in initContainer logic can actually copy bytes from a mounted stage PVC
into a dispatcher-created scratch PVC before the application container runs.

## 2. Background

The current DataPlane evidence proves API admission, cache-hit runtime dispatch,
and dispatcher-created scratch PVC provisioning. It still does not prove the
non-cache-hit stage-in command executes in Kubernetes. A full storage-service
runtime proof is blocked by the current production PVC-share implementation:
`EnsurePVCMounted` only supports Longhorn and JuiceFS CSI-backed source volumes,
and a lightweight kind cluster does not provide those CSI drivers or NFS share
managers.

This slice should therefore prove only the workload data-path mechanics: target
stage PVC is already materialized in the workload namespace, workload dispatch
creates scratch PVC, the generated initContainer copies data from stage PVC to
scratch PVC, and the main container observes the copied data.

## 3. Source References

- `backend/internal/services/workload/dispatcher.go`
- `backend/internal/services/workload/dispatcher_dataplane.go`
- `backend/internal/services/workload/dataplane_client.go`
- `backend/internal/services/workload/job_repository.go`
- `backend/internal/platform/cluster/volume_share.go`
- `backend/internal/services/workload/dispatcher_dataplane_test.go`
- `backend/internal/e2e/storage_data_plane_cache_hit_kind_runtime_e2e_test.go`
- `gap.md`
- `problem.md`
- `docs/acceptance/gap-analysis.md`

## 4. Assumptions

- A workload-package env-gated kind test may call unexported dispatcher helpers
  directly to inject a stub `dataPlanePlanResolver`.
- The test may pre-create and populate the stage PVC because this slice is not
  proving storage-service source PVC projection.
- The stub DataPlanePlan should set scratch `StorageClassName` to empty so kind
  can use its default dynamic StorageClass.
- The stub DataPlanePlan should leave `SourceNamespace` and `SourcePVC` empty so
  `ensureDispatchDataPlanePVCMounts` skips production CSI share materialization
  while `TargetPVC`, `SourcePath`, and `ScratchPath` still drive manifest
  injection and initContainer copy.

## 5. Non-Goals

- No production behavior change unless the test exposes a small required bugfix.
- No storage-service DataPlanePlan resolver runtime proof.
- No Longhorn, JuiceFS, CephFS, local NVMe, CSI, NFS, or external storage
  backend proof.
- No `EnsurePVCMounted` behavior change or kind-only fallback in production.
- No checkpoint flush implementation.
- No quota-aware scratch sizing, cache eviction, performance, multi-node,
  storage GA, Full GA, or V1 launch readiness claim.

## 6. Current Behavior

Local tests assert the generated initContainer contains a `cp -a` stage-in
command. Existing live kind evidence covers cache-hit runtime, where no
stage-in initContainer is created. No live kind test currently proves a
non-cache-hit DataPlane stage-in initContainer copies bytes before the main
container runs.

## 7. Target Behavior

With `TEST_LIVE_WORKLOAD_DATAPLANE_STAGEIN_KIND_RUNTIME=1`, a live kind test
should:

1. Create a temporary namespace.
2. Create and populate a stage PVC with a small payload.
3. Seed a submitted workload job with a `data_plane` block.
4. Dispatch the job with a stub DataPlanePlan containing one non-cache-hit
   stage operation and a scratch claim.
5. Verify workload dispatch creates the scratch PVC.
6. Wait for the Pod to reach `Succeeded`.
7. Verify the main container saw the copied stage-in payload and wrote a marker
   under the checkpoint/scratch path.

## 8. Affected Domains

- `workload-service`: env-gated runtime test evidence for DataPlane stage-in
  initContainer behavior.
- `platform/cluster`: consumed through existing live Kubernetes client and PVC
  helper, no planned code change.
- Acceptance ledgers: bounded evidence wording after live kind pass.

## 9. Affected Files

Add:

- `backend/internal/services/workload/dispatcher_dataplane_stagein_kind_e2e_test.go`

Update after live pass:

- `gap.md`
- `problem.md`
- `docs/acceptance/gap-analysis.md`
- this plan status/evidence section

## 10. API / Contract Changes

None.

## 11. Database / Migration Changes

None.

## 12. Configuration Changes

No checked-in runtime configuration changes.

The test is opt-in through
`TEST_LIVE_WORKLOAD_DATAPLANE_STAGEIN_KIND_RUNTIME=1` and a live `KUBECONFIG`.

## 13. Observability Changes

None for runtime services. The test should emit useful failure diagnostics from
the job record and Kubernetes Pod state when dispatch or execution times out.

## 14. Security Considerations

The test uses only synthetic payload data in a temporary namespace. It must not
create Kubernetes Secrets, log credentials, or add a production fallback that
bypasses storage-service authorization. Because the DataPlanePlan is stubbed,
the ledger must explicitly say this does not prove storage-service permission
checks or source PVC trust boundaries.

## 15. Implementation Steps

1. Add an e2e build-tagged workload package test gated by
   `TEST_LIVE_WORKLOAD_DATAPLANE_STAGEIN_KIND_RUNTIME=1`.
2. Use `cluster.NewFromEnv("proj")`, create a unique namespace, and clean it up.
3. Create a small stage PVC using kind's default StorageClass and populate it
   with a helper Pod.
4. Seed `workload:jobs` with a submitted Pod manifest whose main container:
   - checks the staged payload under `/nexuspaas/scratch/datasets/dataset-v1`;
   - writes a checkpoint marker under `/nexuspaas/scratch/checkpoints`.
5. Dispatch once or in a short retry loop with
   `dispatchSubmittedWorkloadsWithStorageClients`, passing a stub
   `dataPlanePlanResolver`.
6. Assert the dispatcher-created scratch PVC exists and has `ReadWriteOnce` plus
   the default `1Gi` request.
7. Wait for the worker Pod to reach `Succeeded`.
8. Run a verify Pod mounting the scratch PVC and checking both copied payload
   and checkpoint marker.
9. Update acceptance ledgers only after the live kind test passes.

## 16. Verification Plan

Focused skip/default path:

```bash
cd backend
go test -tags e2e ./internal/services/workload -run WorkloadDataPlaneStageInKindRuntime -count=1 -v
```

Live kind:

```bash
KIND=/home/lin/go/bin/kind
CLUSTER=nexuspaas-workload-dataplane-stagein-runtime
export KUBECONFIG=/tmp/${CLUSTER}.kubeconfig
$KIND delete cluster --name "$CLUSTER" || true
$KIND create cluster --name "$CLUSTER" --kubeconfig "$KUBECONFIG" --wait 90s
$KIND load docker-image busybox:1.36 --name "$CLUSTER" || true
cd backend
TEST_LIVE_WORKLOAD_DATAPLANE_STAGEIN_KIND_RUNTIME=1 KUBECONFIG="$KUBECONFIG" \
  go test -tags e2e ./internal/services/workload -run WorkloadDataPlaneStageInKindRuntime -count=1 -v
```

Standard:

```bash
cd backend
go test ./internal/services/workload -run DataPlane -count=1
go test ./internal/contracts/... -count=1
go test ./internal/platform/cluster/... ./internal/services/workload/... ./internal/services/storage/... -count=1
go test ./... -count=1
go build ./...
cd ..
git diff --check
cd backend
go test -tags integration ./... -coverprofile=coverage.out -count=1
bash scripts/ci-security-gate.sh sonar
```

## 17. Rollback Plan

Delete the new e2e test and revert ledger/plan evidence updates. No database,
API, deployment, or production runtime rollback should be needed.

## 18. Risks and Tradeoffs

- This is intentionally not a storage-service resolver or CSI proof; ledger
  wording must stay bounded.
- Pre-creating the stage PVC is acceptable only because this slice targets the
  workload initContainer byte-copy behavior.
- The test uses kind default storage, not local NVMe. It proves Kubernetes PVC
  mount/copy mechanics, not the intended production StorageClass.
- Live kind runtime tests add elapsed time and require local Docker/kind
  availability, so the test remains env-gated.

## 19. Reviewer Checklist

- [ ] The plan does not propose production CSI/share fallback behavior.
- [ ] The test proves stage-in byte copy in Kubernetes, not storage-service
      authorization or backend mounting.
- [ ] The stub DataPlanePlan is clearly documented and does not overclaim.
- [ ] Scratch PVC is dispatcher-created, not pre-created.
- [ ] Stage PVC pre-creation is confined to test setup.
- [ ] Ledger wording keeps CSI/local NVMe/CephFS/Longhorn, checkpoint flush,
      quota-aware sizing, performance, storage GA, Full GA, and V1 launch
      readiness open.

## 20. Status

Status: Approved

## 21. Implementation Evidence

Evidence id: `2026-06-28-storage-data-plane-stagein-kind-runtime-e2e`

Completed verification:

- `go test -tags e2e ./internal/services/workload -run WorkloadDataPlaneStageInKindRuntime -count=1 -v`
  (skip path)
- `go test ./internal/services/workload -run DataPlane -count=1`
- live kind:
  `TEST_LIVE_WORKLOAD_DATAPLANE_STAGEIN_KIND_RUNTIME=1 KUBECONFIG=/tmp/nexuspaas-workload-dataplane-stagein-runtime.kubeconfig go test -tags e2e ./internal/services/workload -run WorkloadDataPlaneStageInKindRuntime -count=1 -v`

Live kind result: `PASS`, `TestWorkloadDataPlaneStageInKindRuntimeE2E`
completed in 19.18s. The test pre-populated a stage PVC, dispatched a workload
with a stub DataPlanePlan, verified dispatcher-created scratch PVC, waited for
the generated stage-in initContainer and main container to reach `Succeeded`,
and verified both the copied payload and checkpoint marker from the scratch PVC.

Reviewer planning note about cleanup traps was applied to the live kind command
used during verification.
