# FastTransfer Mover Execution Kind E2E

## 1. Objective

Add one env-gated `//go:build e2e` test proving the next minimal
FastTransfer data-path slice in kind:

```text
storage fast-stage -> storage dispatch -> k8s-control mover Job -> PVC bind -> Pod schedule -> rsync runs -> one tiny file appears on target PVC
```

The proof must stay narrow. It may claim only kind default StorageClass PVC
binding, Pod scheduling, rsync command execution, and bytes moved for one tiny
file.

## 2. Background

The `storage-data-path` branch already has env-gated kind E2Es for:

- k8s-control mover Job live Kubernetes API admission.
- storage fast-stage to k8s-control mover Job live API admission.

Those tests intentionally stop before PVC binding, Pod scheduling, rsync
execution, and byte movement. This slice advances only that evidence boundary.

## 3. Source References

- `backend/internal/e2e/fast_transfer_mover_kind_admission_e2e_test.go`
- `backend/internal/e2e/fast_transfer_start_mover_kind_admission_e2e_test.go`
- `backend/internal/e2e/storage_data_plane_kind_admission_e2e_test.go`
- `backend/internal/e2e/live_user_project_plan_deploy_e2e_test.go`
- `backend/internal/platform/cluster/fast_transfer_mover.go`
- `backend/internal/services/storage/fast_transfer_dispatch.go`
- `backend/internal/services/storage/fast_transfer_state.go`
- `backend/internal/services/k8scontrol/fast_transfer_mover.go`
- `gap.md`
- `problem.md`
- `docs/acceptance/gap-analysis.md`

## 4. Assumptions

- Branch is `storage-data-path`.
- Normal test runs skip unless `TEST_LIVE_FAST_TRANSFER_MOVER_EXECUTION_KIND=1`
  is set.
- `KUBECONFIG` points at a disposable kind cluster, or `requireLiveKubeconfig`
  can set the default kubeconfig.
- The kind cluster has its default StorageClass available.
- Creating PVCs with no explicit `storageClassName` uses the kind default
  StorageClass.
- The default mover image `instrumentisto/rsync-ssh:alpine` and helper image
  `busybox:1.36` can be pulled by Docker and loaded into kind before the live
  run. If either image cannot be pulled/loaded, treat the run as environment
  blocked, do not update ledgers, and capture Pod events/logs for diagnosis.
- A single-node kind cluster is the expected local target. Multi-node kind is
  best-effort only.

## 5. Non-Goals

- No runtime code changes.
- No contracts, migrations, deployment manifests, or config defaults.
- No new FastTransfer API surface.
- No progress callback assertion.
- No multi-file, multi-directory, large-file, resume, checksum, delete-sync, or
  performance coverage.
- No external CSI/backend storage claim beyond kind default PVC binding.
- No storage GA, Full GA, or first-version launch readiness claim.
- No new helper package or generic Kubernetes test framework.

## 6. Current Behavior

- Existing fake tests prove mover Job manifest construction.
- Existing kind admission E2Es prove Job API admission and storage-to-k8s-control
  dispatch.
- No test proves the admitted mover Job can bind PVCs, schedule, run rsync, and
  move bytes in kind.

## 7. Target Behavior

With `TEST_LIVE_FAST_TRANSFER_MOVER_EXECUTION_KIND=1`, the new E2E:

1. Creates a live cluster client and disposable namespace.
2. Creates `source-pvc` and `target-pvc` in that namespace using kind's default
   StorageClass.
3. Seeds `source-pvc` with one file at `/data/source/hello.txt` using a
   short-lived `busybox:1.36` helper Pod.
4. Starts k8s-control and storage `httptest` servers using the approved
   start-to-mover admission E2E setup.
5. Posts to the storage fast-stage route.
6. Reads the returned mover Job namespace/name.
7. Waits for the mover Job to reach `Complete`.
8. Verifies source and target PVCs are `Bound`.
9. Verifies at least one mover Pod was scheduled and succeeded.
10. Verifies `target-pvc` contains `/data/target/hello.txt` with the expected
    content and byte count using another short-lived `busybox:1.36` helper Pod.

## 8. Affected Domains

- `storage-service`: existing external FastTransfer start route and dispatch.
- `k8s-control-service`: existing internal mover Job route.
- `platform/cluster`: existing mover Job manifest and rsync script.
- Kubernetes kind: default StorageClass PVC binding, Pod scheduling, Job
  execution.
- Acceptance ledgers: wording only after live proof passes.

## 9. Affected Files

Plan artifact:

- `docs/plan/2026-06-28-fast-transfer-mover-execution-kind-e2e.md`

Code Agent may add only:

- `backend/internal/e2e/fast_transfer_mover_execution_kind_e2e_test.go`

Code Agent should reuse, not edit, helpers from:

- `backend/internal/e2e/fast_transfer_mover_kind_admission_e2e_test.go`
- `backend/internal/e2e/fast_transfer_start_mover_kind_admission_e2e_test.go`

Ledger files may be edited only after the live test passes:

- `gap.md`
- `problem.md`
- `docs/acceptance/gap-analysis.md`

No production/runtime files, contracts, migrations, deployment files, `go.mod`,
or config files are in scope.

## 10. API / Contract Changes

None. The test exercises existing routes:

```text
POST /api/v1/projects/{project_id}/storage/transfers/fast-stage
POST /internal/k8s-control/fast-transfers/mover-jobs
```

Use the same payload shape as `fastTransferStartRequest`:

- `source.namespace` / `target.namespace`: disposable namespace
- `source.pvc`: `source-pvc`
- `target.pvc`: `target-pvc`
- `source.path`: `/data/source`
- `target.path`: `/data/target`
- `tool`: `rsync`
- idempotency key: synthetic per test run

## 11. Database / Migration Changes

None. Use the existing in-memory `platform.Store` seed pattern from the
start-to-mover admission E2E.

## 12. Configuration Changes

No checked-in runtime config changes.

Test-only env gate:

```text
TEST_LIVE_FAST_TRANSFER_MOVER_EXECUTION_KIND=1
KUBECONFIG=<path to disposable kind kubeconfig>
```

Do not add runtime config defaults or feature flags.

## 13. Observability Changes

None. The test may fetch helper/mover Pod logs on failure to make failures
diagnosable, but no application logs, metrics, or traces are changed.

## 14. Security Considerations

- Use a unique disposable namespace and delete it in `t.Cleanup`.
- Do not log kubeconfig content or service keys.
- Exercise storage project access and k8s-control service-key auth through HTTP.
- Keep the existing mover assertions for no `hostPath`, no privileged
  container, `AutomountServiceAccountToken=false`, and source PVC read-only.
- Helper Pods mount only one test PVC and run fixed shell commands with
  synthetic content.

## 15. Implementation Steps

1. Add `backend/internal/e2e/fast_transfer_mover_execution_kind_e2e_test.go`
   with `//go:build e2e`.
2. Add env const `fastTransferMoverExecutionKindEnv =
   "TEST_LIVE_FAST_TRANSFER_MOVER_EXECUTION_KIND"`.
3. Skip unless the env var equals `1`.
4. Call `requireLiveKubeconfig(t)`, `cluster.NewFromEnv("proj")`, and
   `cl.Ping(ctx)`.
5. Generate short DNS-safe IDs and namespace `ft-exec-<suffix>`.
6. Reuse `createFastTransferMoverAdmissionNamespace` for namespace creation and
   cleanup.
7. Create `source-pvc` and `target-pvc` with `ReadWriteOnce`, tiny storage
   request, and nil `StorageClassName` so Kubernetes applies the kind default.
8. Add local test helpers only in the new file:
   - create PVC
   - run a one-shot PVC helper Pod
   - wait for Pod `Succeeded` and scheduled `Spec.NodeName`
   - wait for PVC `Bound`
   - wait for mover Job `Complete`
   - list mover Pods by `job-name`
9. Seed source data with a helper Pod mounting `source-pvc`, creating
   `/pvc/data/source/hello.txt`, and writing a fixed no-newline string.
   The helper Pod image must be `busybox:1.36` and the command may rely only on
   `/bin/sh`, `mkdir`, `printf`, `cat`, and `wc`.
10. Wait for the seed helper Pod to succeed and `source-pvc` to bind.
11. Start k8s-control with `newFastTransferMoverAdmissionApp(cl)`.
12. Start storage with `newFastTransferStartStorageApp(store, events,
    k8sServer.URL)`.
13. Reuse `seedFastTransferStartAccess`, `fastTransferStartRequest`,
    `postFastTransferStart`, and `assertFastTransferStartRecord`.
14. POST storage fast-stage and assert `202 Accepted`, queued record, submitted
    dispatch metadata, and live mover Job namespace/name.
15. Reuse `getFastTransferMoverAdmissionJob` and
    `assertFastTransferMoverAdmissionJob` for restricted manifest assertions.
16. Wait for the mover Job to complete.
17. Assert both PVCs are `Bound` with non-empty `Spec.VolumeName`.
18. Assert at least one mover Pod has non-empty `Spec.NodeName` and
    `Status.Phase=Succeeded`.
19. Verify target data with a helper Pod mounting `target-pvc`; the helper must
    assert both exact file content and byte count at `/pvc/data/target/hello.txt`.
    Use the same `busybox:1.36` helper image and fixed shell tool set as the
    seed helper.
20. Update ledgers only after the live run passes, using the boundaries below.

Ledger wording boundaries:

- `gap.md` row status may say:
  `Done for env-gated kind default PVC binding, Pod scheduling, rsync command execution, and one tiny file copied through storage fast-stage -> k8s-control -> mover Job only; no CSI/storage GA, external storage backend, multi-node, multi-file, progress callback, performance, durability, or Full GA claim`.
- `docs/acceptance/gap-analysis.md` / `problem.md` may say:
  `Env-gated FastTransfer mover execution evidence now exists via backend/internal/e2e/fast_transfer_mover_execution_kind_e2e_test.go; it starts at storage fast-stage, dispatches to k8s-control, creates source/target PVCs with the kind default StorageClass, seeds one source file, waits for the mover Job to Complete, and verifies the target PVC contains the expected content and byte count. This proves only kind default PVC binding, Pod scheduling, rsync command execution, and bytes moved for one tiny file; it does not prove CSI/storage GA, external storage backends, multi-node behavior, multi-file sync, progress callbacks, performance, durability, or Full GA.`
- Do not delete or soften the older admission-only disclaimers unless replacing
  them with equally narrow execution wording.

## 16. Verification Plan

Acceptance criteria:

- Env-unset run skips cleanly.
- Env-set kind run starts at storage fast-stage, not direct k8s-control.
- Source and target PVCs are created in a disposable namespace using the kind
  default StorageClass.
- Source PVC binds after the seed helper Pod schedules.
- Storage response is `202 Accepted` with queued/submitted mover metadata.
- Live mover Job exists, matches the restricted manifest assertions, schedules a
  Pod, and reaches `Complete`.
- Target PVC binds and target verification helper Pod succeeds.
- Target file content and byte count match the seeded source file.
- No runtime, contract, migration, deployment, or config-default files change.
- Ledger wording, if added, stays within the boundary in section 15.

Env-unset skip:

```bash
cd backend && go test -tags e2e ./internal/e2e -run FastTransferMoverExecutionKind -count=1 -v
```

Targeted live kind run:

```bash
export KUBECONFIG=/tmp/nexuspaas-fast-transfer-exec-e2e.kubeconfig
kind create cluster --name nexuspaas-fast-transfer-exec-e2e --kubeconfig "$KUBECONFIG"
docker pull instrumentisto/rsync-ssh:alpine
docker pull busybox:1.36
kind load docker-image instrumentisto/rsync-ssh:alpine --name nexuspaas-fast-transfer-exec-e2e
kind load docker-image busybox:1.36 --name nexuspaas-fast-transfer-exec-e2e
kubectl --kubeconfig "$KUBECONFIG" get storageclass
cd backend && TEST_LIVE_FAST_TRANSFER_MOVER_EXECUTION_KIND=1 KUBECONFIG="$KUBECONFIG" go test -tags e2e ./internal/e2e -run FastTransferMoverExecutionKind -count=1 -v
kind delete cluster --name nexuspaas-fast-transfer-exec-e2e --kubeconfig "$KUBECONFIG"
```

If image pull/load or kind cluster setup fails, do not update acceptance ledgers.
If helper/mover Pods fail after scheduling, collect Pod status, events, and logs
before deciding whether this is a code defect or an environment limitation.

Focused non-live checks:

```bash
cd backend && go test ./internal/services/storage -run FastTransfer
cd backend && go test ./internal/services/k8scontrol -run FastTransferMover
cd backend && go test ./internal/platform/cluster -run FastTransferMover
cd backend && go test -tags e2e ./internal/e2e -run 'FastTransfer(MoverKindAdmission|StartMoverKindAdmission)' -count=1 -v
```

Broader gates:

```bash
cd backend && go test ./...
cd backend && go build ./...
git diff --check
cd backend && make coverage
cd backend && make ci-sonar
```

## 17. Rollback Plan

Delete:

- `backend/internal/e2e/fast_transfer_mover_execution_kind_e2e_test.go`

Revert only scoped ledger wording in:

- `gap.md`
- `problem.md`
- `docs/acceptance/gap-analysis.md`

No runtime, database, migration, contract, deployment, or config rollback is
expected.

## 18. Risks and Tradeoffs

- kind default StorageClass may be absent or not marked default in a custom
  cluster; fail clearly rather than adding fallback config.
- kind local-path provisioning commonly uses `WaitForFirstConsumer`; the test
  must bind through helper/mover Pods instead of waiting for binding before any
  consumer exists.
- The default rsync image pull can make the test slow or flaky on a cold kind
  node.
- Multi-node kind can expose local-path placement constraints; this slice only
  claims the normal lightweight kind path.
- Waiting for actual Job completion is slower than admission tests, so keep
  timeouts bounded and diagnostics concrete.

## 19. Reviewer Checklist

- [ ] Plan file exists under `docs/plan/`.
- [ ] Scope is test-only and adds one E2E file.
- [ ] Test has `//go:build e2e`.
- [ ] Test is gated by `TEST_LIVE_FAST_TRANSFER_MOVER_EXECUTION_KIND=1`.
- [ ] Test reuses existing FastTransfer admission/start helpers where possible.
- [ ] Test starts separate storage and k8s-control `httptest` servers.
- [ ] Test creates source/target PVCs with kind default StorageClass.
- [ ] Test proves PVC binding, Pod scheduling, Job completion, and target bytes.
- [ ] Test does not add runtime code, contracts, migrations, config defaults, or
      deployment files.
- [ ] Ledger wording does not overclaim CSI/storage GA, external storage,
      progress callbacks, performance, durability, or Full GA.
- [ ] Verification results include skip, live kind run when available, focused
      tests, full test/build, coverage, and Sonar.

## 20. Status

Status: Implemented and Reviewer approved
