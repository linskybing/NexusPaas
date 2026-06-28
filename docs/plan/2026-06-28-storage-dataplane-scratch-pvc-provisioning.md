# Storage DataPlane Scratch PVC Provisioning

Date: 2026-06-28
Status: Approved

## 1. Objective

Remove the current pre-created scratch PVC requirement for DataPlane runtime
dispatch by having workload dispatch ensure the DataPlane scratch PVC exists
before creating the Pod manifest.

## 2. Background

The cache-hit kind runtime E2E proved the injected scratch mount and checkpoint
environment work, but only after the test pre-created `scratch-{job}`. That is
not a complete platform data-path behavior: storage-service returns a scratch
claim in DataPlanePlan, and workload-service should ensure that claim exists
before dispatch.

## 3. Source References

- `backend/internal/services/workload/dispatcher.go`
- `backend/internal/services/workload/dispatcher_dataplane.go`
- `backend/internal/services/workload/dataplane_client.go`
- `backend/internal/platform/cluster/volume_share.go`
- `backend/internal/platform/cluster/volume_share_test.go`
- `backend/internal/e2e/storage_data_plane_cache_hit_kind_runtime_e2e_test.go`
- `gap.md`
- `problem.md`
- `docs/acceptance/gap-analysis.md`

## 4. Assumptions

- Scratch PVC creation belongs in dispatch-time infrastructure preparation, next
  to DataPlane stage PVC preparation.
- The storage profile's `storage_class_name` and access mode in the
  DataPlanePlan are enough to create a bounded PVC.
- A conservative default size is acceptable for this slice because quota-aware
  scratch sizing remains a separate gap.
- The kind E2E must override the seeded `local-nvme-scratch` profile's
  `storage_class_name` to an empty string so the dispatcher-created PVC can bind
  through the kind default StorageClass. Production code still uses the
  DataPlanePlan storage class value.

## 5. Non-Goals

- No local NVMe provisioner, CSI driver, CephFS, Longhorn, or real backend
  runtime proof.
- No stage-in byte copy implementation.
- No checkpoint flush implementation.
- No quota-aware sizing or eviction policy.
- No performance, multi-node, storage GA, Full GA, or V1 launch readiness claim.

## 6. Current Behavior

DataPlane dispatch injects a PVC volume using `plan.Scratch.ClaimName`, but no
production code creates that PVC. The cache-hit kind runtime test had to
pre-create it.

## 7. Target Behavior

When a DataPlanePlan includes a scratch claim, workload dispatch should create
that PVC if missing before creating workload manifests. Repeated dispatch should
treat an existing compatible PVC as okay.

The kind cache-hit runtime E2E should no longer pre-create scratch PVC and
should still reach Pod `Succeeded`.

## 8. Affected Domains

- `workload-service`: dispatch-time DataPlane scratch preparation.
- `platform/cluster`: Kubernetes PVC helper.
- `internal/e2e`: cache-hit runtime test proof.

## 9. Affected Files

Update:

- `backend/internal/platform/cluster/volume_share.go`
- `backend/internal/platform/cluster/volume_share_test.go`
- `backend/internal/services/workload/dispatcher_dataplane.go`
- `backend/internal/services/workload/dispatcher_dataplane_test.go`
- `backend/internal/e2e/storage_data_plane_cache_hit_kind_runtime_e2e_test.go`
- `gap.md`
- `problem.md`
- `docs/acceptance/gap-analysis.md`
- this plan status

## 10. API / Contract Changes

None.

## 11. Database / Migration Changes

None.

## 12. Configuration Changes

None.

## 13. Observability Changes

None.

## 14. Security Considerations

Scratch PVC creation uses the storage-service produced plan, not user-supplied
raw PVC references. This does not prove production workload identity or storage
tenant isolation beyond the existing DataPlanePlan checks.

## 15. Implementation Steps

1. Add a small cluster helper to ensure a PVC exists with namespace, name,
   access mode, optional StorageClass, and small default request size.
2. Call that helper from `ensureDispatchDataPlanePVCMounts` before stage PVC
   mount handling.
3. Keep existing compatible PVCs idempotent.
4. Override the cache-hit kind runtime E2E storage profile to use an empty
   `storage_class_name`, remove scratch PVC pre-creation, and assert the
   dispatcher-created scratch PVC exists with the expected access mode and
   storage request.
5. Add focused unit coverage for the PVC helper and dispatcher scratch path.
6. Update ledgers only after the live kind E2E passes without pre-created
   scratch PVC.

## 16. Verification Plan

Focused:

```bash
cd backend
go test ./internal/platform/cluster -run 'DataPlaneScratch|PVC' -count=1
go test ./internal/services/workload -run DataPlane -count=1
go test -tags e2e ./internal/e2e -run StorageDataPlaneCacheHitKindRuntime -count=1 -v
```

Live kind:

```bash
export KUBECONFIG=/tmp/nexuspaas-storage-dataplane-cache-hit-runtime.kubeconfig
kind create cluster --name nexuspaas-storage-dataplane-cache-hit-runtime --kubeconfig "$KUBECONFIG"
kind load docker-image busybox:1.36 --name nexuspaas-storage-dataplane-cache-hit-runtime || true
cd backend
TEST_LIVE_STORAGE_DATAPLANE_KIND_RUNTIME=1 KUBECONFIG="$KUBECONFIG" \
  go test -tags e2e ./internal/e2e -run StorageDataPlaneCacheHitKindRuntime -count=1 -v
```

Standard:

```bash
cd backend
go test ./internal/contracts/... -count=1
go test ./internal/platform/cluster/... ./internal/services/workload/... ./internal/services/storage/... -count=1
go test ./... -count=1
go build ./...
cd ..
git diff --check
bash backend/scripts/ci-security-gate.sh sonar
```

## 17. Rollback Plan

Revert the cluster helper, workload dispatch call, E2E adjustment, tests, and
ledger updates.

## 18. Risks and Tradeoffs

- The helper uses a fixed small request size; quota-aware scratch sizing stays
  open.
- Existing PVC compatibility checks stay minimal to keep the slice bounded.
- The kind proof uses an empty test profile StorageClass for default dynamic
  provisioning; it does not prove the `local-nvme-scratch` StorageClass exists
  or binds in production.
- This improves cache-hit/runtime scratch behavior but does not close stage-in
  byte copy or real backend runtime gaps.

## 19. Reviewer Checklist

- [ ] Production change is limited to dispatch-time scratch PVC creation.
- [ ] The PVC is derived from DataPlanePlan, not raw user manifest data.
- [ ] Existing PVC behavior is idempotent.
- [ ] Cache-hit kind runtime E2E no longer pre-creates scratch PVC and asserts
      the dispatcher-created PVC.
- [ ] The kind E2E explicitly uses a kind-compatible test profile StorageClass
      override without changing production defaults.
- [ ] Ledger wording does not claim stage-in copy, CSI/local NVMe runtime,
      checkpoint flush, storage GA, Full GA, or V1 readiness.

## 20. Status

Status: Approved

## 21. Implementation Evidence

Evidence id: `2026-06-28-storage-data-plane-scratch-pvc-provisioning`

Completed verification:

- `go test ./internal/platform/cluster -run 'EnsurePVC|PVC' -count=1`
- `go test ./internal/services/workload -run 'DataPlane|ScratchAccessMode' -count=1`
- `go test -tags e2e ./internal/e2e -run StorageDataPlaneCacheHitKindRuntime -count=1 -v`
  (skip path)
- live kind:
  `TEST_LIVE_STORAGE_DATAPLANE_KIND_RUNTIME=1 KUBECONFIG=/tmp/nexuspaas-storage-dataplane-cache-hit-runtime.kubeconfig go test -tags e2e ./internal/e2e -run StorageDataPlaneCacheHitKindRuntime -count=1 -v`

Live kind result: `PASS`, `TestStorageDataPlaneCacheHitKindRuntimeE2E`
completed in 15.68s with the scratch PVC created by workload dispatch instead
of pre-created by the test.

Reviewer follow-up fixed access mode normalization so profile values such as
`rwo`, `rwx`, and `rox` and Kubernetes values such as `ReadWriteMany` preserve
the DataPlanePlan access mode instead of falling back to `ReadWriteOnce`.
