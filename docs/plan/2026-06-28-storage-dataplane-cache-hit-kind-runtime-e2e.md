# Storage DataPlane Cache-Hit Kind Runtime E2E

Date: 2026-06-28
Status: Approved

## 1. Objective

Prove the next missing Storage DataPlane dispatch layer: a workload with
`data_plane` can be dispatched into kind and its Pod can reach `Succeeded` while
using the injected scratch PVC and checkpoint environment.

## 2. Background

Current evidence proves DataPlane API admission and manifest creation in kind,
but the ledger still says no scheduler success, local PV/PVC runtime binding, or
runtime behavior proof. The existing admission E2E intentionally uses a
JuiceFS-like CSI source PV, which is fine for API admission but cannot be run to
completion in a plain kind cluster without the CSI driver.

This slice adds a narrower cache-hit runtime proof that avoids external CSI by
skipping stage source mounts and running only against a pre-created kind default
scratch PVC.

## 3. Source References

- `backend/internal/e2e/storage_data_plane_kind_admission_e2e_test.go`
- `backend/internal/e2e/storage_data_plane_dispatch_e2e_test.go`
- `backend/internal/e2e/storage_data_plane_plan_e2e_test.go`
- `backend/internal/services/storage/data_plane_contracts.go`
- `backend/internal/services/workload/dispatcher_dataplane.go`
- `gap.md`
- `problem.md`
- `docs/acceptance/gap-analysis.md`

## 4. Assumptions

- The live run uses a disposable lightweight kind cluster.
- The cluster can pull or already has `busybox:1.36`.
- A pre-created scratch PVC using the kind default StorageClass is acceptable
  for this evidence slice because production scratch PVC provisioning remains a
  separate gap.
- A CacheBinding row is valid enough to force the resolver's `cache_hit=true`
  path and skip source PVC mounting.

## 5. Non-Goals

- No CSI driver, CephFS, JuiceFS, Longhorn, local NVMe, or real StorageClass
  runtime proof.
- No stage-in byte copy proof.
- No checkpoint flush proof.
- No scratch PVC auto-provisioning implementation.
- No scheduler performance, multi-node, durability, Redis/Postgres, storage GA,
  Full GA, or V1 launch readiness claim.

## 6. Current Behavior

`TestStorageDataPlaneKindAdmissionE2E` waits until the namespace, source/target
PVCs, target PV, and Pod object exist, then asserts manifest wiring. It does not
wait for the Pod to schedule or succeed.

## 7. Target Behavior

When `TEST_LIVE_STORAGE_DATAPLANE_KIND_RUNTIME=1` is set, the new E2E should:

1. Seed the same storage/project permission records plus a matching
   CacheBinding.
2. Pre-create the scratch PVC named by the DataPlane resolver.
3. Dispatch a Pod whose container verifies `CHECKPOINT_DIR`,
   `NEXUSPAAS_CHECKPOINT_FLUSH_TARGET`,
   `NEXUSPAAS_CHECKPOINT_WRITE_POLICY`, and writes a marker under scratch.
4. Wait for the Pod to reach `Succeeded`.
5. Assert `DataPlanePlanBuilt` was emitted and the built plan took the
   cache-hit path.

## 8. Affected Domains

- `internal/e2e`: env-gated live kind evidence only.
- `storage-service`: DataPlane plan event evidence only.
- `workload-service`: dispatcher runtime evidence only.

## 9. Affected Files

Add:

- `backend/internal/e2e/storage_data_plane_cache_hit_kind_runtime_e2e_test.go`

Update after the live kind test passes:

- `gap.md`
- `problem.md`
- `docs/acceptance/gap-analysis.md`
- this plan status

## 10. API / Contract Changes

None. The test exercises existing service internals and dispatch behavior.

## 11. Database / Migration Changes

None. The test uses the existing in-memory platform store.

## 12. Configuration Changes

The test is gated by:

- `TEST_LIVE_STORAGE_DATAPLANE_KIND_RUNTIME=1`
- `KUBECONFIG`

## 13. Observability Changes

No production observability changes. The test should log Pod status and recent
container state on timeout through small local helpers.

## 14. Security Considerations

The test reuses storage-service permission records and does not bypass
DataPlanePlan authorization. It does not prove production workload identity or
secret handling.

## 15. Implementation Steps

1. Add a new env-gated kind E2E file.
2. Reuse existing DataPlane storage/workload app helpers.
3. Seed a unique Project/Group/User/Job and CacheBinding.
4. Pre-create the scratch PVC in the workload namespace.
5. Create a submitted workload record with a BusyBox Pod command that verifies
   injected checkpoint env and scratch writeability.
6. Run workload maintenance until the Pod exists.
7. Wait for the Pod phase to become `Succeeded`.
8. Assert `DataPlanePlanBuilt` and cache-hit evidence.
9. Update ledgers only after a passing live kind run.

## 16. Verification Plan

Compile/skip:

```bash
cd backend
go test -tags e2e ./internal/e2e -run StorageDataPlaneCacheHitKindRuntime -count=1 -v
```

Live kind run:

```bash
export KUBECONFIG=/tmp/nexuspaas-storage-dataplane-cache-hit-runtime.kubeconfig
kind create cluster --name nexuspaas-storage-dataplane-cache-hit-runtime --kubeconfig "$KUBECONFIG"
kind load docker-image busybox:1.36 --name nexuspaas-storage-dataplane-cache-hit-runtime || true
cd backend
TEST_LIVE_STORAGE_DATAPLANE_KIND_RUNTIME=1 KUBECONFIG="$KUBECONFIG" \
  go test -tags e2e ./internal/e2e -run StorageDataPlaneCacheHitKindRuntime -count=1 -v
```

Standard gates:

```bash
cd backend
go test ./internal/contracts/... -count=1
go test ./internal/services/storage/... ./internal/services/workload/... -count=1
go test ./... -count=1
go build ./...
cd ..
git diff --check
bash backend/scripts/ci-security-gate.sh sonar
```

## 17. Rollback Plan

Delete the new E2E file and revert the ledger updates. No runtime code,
database, or deployment rollback is required.

## 18. Risks and Tradeoffs

- This proves only cache-hit runtime dispatch, not stage-in byte copying.
- Pre-created scratch PVC keeps the proof small and honest; automatic scratch
  provisioning remains a separate gap.
- Plain kind scheduling may still depend on pulling `busybox:1.36`.

## 19. Reviewer Checklist

- [ ] The plan stays limited to env-gated E2E evidence and ledgers.
- [ ] No production runtime code changes.
- [ ] Cache-hit wording does not imply stage-in byte copy.
- [ ] Scratch PVC is explicitly pre-created by the test.
- [ ] Ledger wording does not claim CSI, local NVMe, checkpoint flush,
      durability, storage GA, Full GA, or V1 readiness.

## 20. Status

Status: Approved
