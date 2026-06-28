# FastTransfer Progress Callback To Storage Kind E2E

Date: 2026-06-28
Status: Approved

## 1. Objective

Prove the next missing FastTransfer data-path link: a mover Job running in kind
can POST progress callbacks to a storage-service handler, and that handler
updates the FastTransfer record plus emits transition events.

## 2. Background

Existing evidence already proves two separate pieces:

- `backend/internal/e2e/fast_transfer_progress_callback_kind_e2e_test.go`
  proves a kind mover Job can emit progress callbacks to an in-cluster sink.
- `backend/internal/services/storage/fast_transfer_state_test.go` proves the
  storage-service progress handler updates state and emits
  `FastTransferProgressed` / `FastTransferCompleted` locally.

This slice connects those two pieces with one env-gated live kind test.

## 3. Source References

- `backend/internal/e2e/fast_transfer_progress_callback_kind_e2e_test.go`
- `backend/internal/e2e/fast_transfer_mover_execution_kind_e2e_test.go`
- `backend/internal/e2e/fast_transfer_start_mover_kind_admission_e2e_test.go`
- `backend/internal/services/storage/handler.go`
- `backend/internal/services/storage/fast_transfer_state.go`
- `backend/internal/services/k8scontrol/fast_transfer_mover.go`
- `backend/internal/platform/cluster/fast_transfer_mover.go`
- `gap.md`
- `problem.md`
- `docs/acceptance/gap-analysis.md`

## 4. Assumptions

- The live run uses a disposable lightweight kind cluster.
- The kind cluster can pull or already has `busybox:1.36` and
  `instrumentisto/rsync-ssh:alpine`.
- The test process can expose a storage-service httptest listener that is
  reachable from the kind mover Pod.
- `TEST_KIND_PROGRESS_STORAGE_CALLBACK_BASE_URL` may be required on Linux or
  non-Docker-Desktop kind setups. The best-effort fallback replaces loopback
  with `host.docker.internal`, which is not portable kind evidence.

## 5. Non-Goals

- No production deployment or real Kubernetes Service for storage-service.
- No Redis, Postgres, or durable event bus proof.
- No external CSI/backend storage proof beyond kind default PVC behavior.
- No multi-node, performance, checksum, or resume proof.
- No generic/legacy transfer route coverage.
- No storage GA, Full GA, or V1 launch readiness claim.

## 6. Current Behavior

The current kind callback test targets a sink Pod and proves only callback
emission. The current storage-service state tests target the handler directly
and prove only local in-memory state transitions. No current test proves a kind
mover callback reaches storage-service.

## 7. Target Behavior

When `TEST_LIVE_FAST_TRANSFER_PROGRESS_STORAGE_KIND=1` is set, the new E2E
starts from storage fast-stage, dispatches a mover Job through k8s-control, lets
the mover copy one tiny file, and verifies that the mover's progress callbacks
update the storage FastTransfer record to `status="succeeded"` and
`progress_pct=100`.

The same run must assert `FastTransferProgressed` and
`FastTransferCompleted` are present in the in-memory event bus.

## 8. Affected Domains

- `storage-service`: FastTransfer progress callback handler evidence.
- `k8s-control-service`: mover Job callback environment evidence.
- `platform/cluster`: mover Job execution path evidence.
- `internal/e2e`: env-gated live kind verification only.

## 9. Affected Files

Add:

- `backend/internal/e2e/fast_transfer_progress_storage_kind_e2e_test.go`

Update after the live kind test passes:

- `gap.md`
- `problem.md`
- `docs/acceptance/gap-analysis.md`
- this plan status

## 10. API / Contract Changes

No API or contract changes. The test exercises existing routes:

- `POST /api/v1/projects/{id}/storage/transfers/fast-stage`
- `POST /internal/storage/projects/{project_id}/transfers/{targetNamespace}/{name}/progress`
- `POST /internal/k8s-control/fast-transfers/mover-jobs`

## 11. Database / Migration Changes

None. The test uses the existing in-memory platform store.

## 12. Configuration Changes

The new test uses:

- `TEST_LIVE_FAST_TRANSFER_PROGRESS_STORAGE_KIND=1` to enable live kind mode.
- `KUBECONFIG` for the disposable kind cluster.
- Optional `TEST_KIND_PROGRESS_STORAGE_CALLBACK_BASE_URL` for Linux or
  non-Docker-Desktop setups where `host.docker.internal` is not reachable from
  kind Pods.

Test app config must be explicit:

- storage-service accepts `k8s-control-service` via
  `ServiceTrustedIdentities` or equivalent matching service key config.
- k8s-control sets `ServiceURLs["storage-service"]`,
  `ServiceIdentityName="k8s-control-service"`, and the matching
  `ServiceIdentityKey`.
- The test asserts the mover Job has the expected progress URL, service name,
  and key env before waiting for completion.

## 13. Observability Changes

No production observability changes. On failure, the E2E should emit mover Pod
diagnostics using existing helper diagnostics so callback reachability failures
are visible.

## 14. Security Considerations

The storage progress handler requires service auth. The E2E must not bypass
that handler or fall back to an unauthenticated sink. The configured
k8s-control service identity must match what storage-service trusts.

This does not prove production secret handling or workload identity.

## 15. Implementation Steps

1. Add `backend/internal/e2e/fast_transfer_progress_storage_kind_e2e_test.go`.
2. Reuse existing PVC, namespace, mover execution, storage app, and post
   helpers where possible.
3. Start storage-service with shared in-memory store/event bus and trusted
   k8s-control service identity.
4. Derive callback base URL from
   `TEST_KIND_PROGRESS_STORAGE_CALLBACK_BASE_URL` when set; otherwise use a
   best-effort `host.docker.internal` URL based on the storage server listener.
5. Start k8s-control with the callback base URL and service identity.
6. Start fast-stage through storage-service so the FastTransfer record exists
   before callbacks run.
7. Assert the generated mover Job contains expected callback env.
8. Wait for mover Job completion and target PVC content.
9. Poll the shared store until the FastTransfer record reaches succeeded/100.
10. Assert `FastTransferProgressed` and `FastTransferCompleted` were emitted.
11. Update ledgers only after a passing live kind run.

## 16. Verification Plan

Compile/skip:

```bash
cd backend
go test -tags e2e ./internal/e2e -run FastTransferProgressStorageKind -count=1 -v
```

Focused non-live regression:

```bash
cd backend
go test ./internal/services/storage -run 'FastTransferProgress|FastTransferStart' -count=1
go test ./internal/services/k8scontrol -run FastTransferMover -count=1
go test ./internal/platform/cluster -run FastTransferMover -count=1
```

Live kind run:

```bash
export KUBECONFIG=/tmp/nexuspaas-fast-transfer-progress-storage.kubeconfig
kind create cluster --name nexuspaas-fast-transfer-progress-storage --kubeconfig "$KUBECONFIG"
kind load docker-image busybox:1.36 --name nexuspaas-fast-transfer-progress-storage || true
kind load docker-image instrumentisto/rsync-ssh:alpine --name nexuspaas-fast-transfer-progress-storage || true
# Optional when kind Pods cannot reach host.docker.internal:
# export TEST_KIND_PROGRESS_STORAGE_CALLBACK_BASE_URL=http://<pod-reachable-host>:<port>
cd backend
TEST_LIVE_FAST_TRANSFER_PROGRESS_STORAGE_KIND=1 KUBECONFIG="$KUBECONFIG" \
  go test -tags e2e ./internal/e2e -run FastTransferProgressStorageKind -count=1 -v
```

Standard gates:

```bash
cd backend
go test ./internal/contracts/... -count=1
go test ./internal/services/storage/... -count=1
go test ./... -count=1
go build ./...
cd ..
git diff --check
```

Attempt SonarScanner after coverage if local Sonar is available. If
`http://localhost:9000` is unavailable, record it as an environment blocker.

## 17. Rollback Plan

Delete the new E2E file and revert the ledger changes. No runtime code,
database, or deployment rollback is required.

## 18. Risks and Tradeoffs

- `host.docker.internal` is a best-effort Docker Desktop/kind shortcut, not a
  portable Linux kind guarantee.
- Linux/non-Docker-Desktop runs may need
  `TEST_KIND_PROGRESS_STORAGE_CALLBACK_BASE_URL` pointing at a listener or proxy
  reachable from the kind node.
- The test remains env-gated because it depends on live Kubernetes, image pulls,
  and host networking.
- The proof is intentionally narrow and does not claim Redis/Postgres durability
  or production service identity.

## 19. Reviewer Checklist

- [ ] Plan uses only allowed status values.
- [ ] Implementation adds one env-gated E2E file only before a live pass.
- [ ] Storage callback target is storage-service progress handler, not a sink.
- [ ] k8s-control callback env includes URL, service name, and key.
- [ ] Storage-service trusts the k8s-control identity used by the mover.
- [ ] Ledgers are updated only after the live kind test passes.
- [ ] Ledger wording does not claim Redis, Postgres, durability, external
      storage backend, multi-node, performance, GA, or launch readiness.

## 20. Status

Status: Approved
