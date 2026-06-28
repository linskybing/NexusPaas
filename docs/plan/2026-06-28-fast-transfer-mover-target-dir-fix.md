# FastTransfer Mover Target Directory Fix

## 1. Objective

Fix the FastTransfer mover runtime defect where the live kind execution E2E
fails because rsync cannot create missing parent directories on the target PVC.

The minimal runtime change is to have the generated mover shell script create
the target directory before running rsync:

```sh
mkdir -p <target>
rsync -a --delete -- <source> <target>
```

## 2. Background

The approved test-only plan
`docs/plan/2026-06-28-fast-transfer-mover-execution-kind-e2e.md` was
implemented, but the live kind run failed after the mover Pod scheduled and
started.

Observed mover log:

```text
rsync: [Receiver] mkdir "/mnt/target/data/target" failed: No such file or directory (2)
```

`backend/internal/platform/cluster/fast_transfer_mover.go` currently builds the
mover script as:

```go
set -eu
rsync -a --delete -- <source> <target>
```

rsync creates the final directory entry only when its parent exists. In this
case `/mnt/target/data` does not exist yet, so the mover Job fails and backs
off.

## 3. Source References

- `docs/plan/2026-06-28-fast-transfer-mover-execution-kind-e2e.md`
- `backend/internal/platform/cluster/fast_transfer_mover.go`
- `backend/internal/platform/cluster/fast_transfer_mover_test.go`
- `backend/internal/e2e/fast_transfer_mover_execution_kind_e2e_test.go`
- `gap.md`
- `problem.md`
- `docs/acceptance/gap-analysis.md`

## 4. Assumptions

- The runtime failure is caused by the missing target parent directory, not by
  PVC binding, Pod scheduling, image loading, or route dispatch.
- The target path has already passed existing FastTransfer mover validation and
  normalization.
- `mkdir -p` is available in the existing mover image shell environment.
- The already-added execution kind E2E remains the correct live verification
  path.

## 5. Non-Goals

- No FastTransfer route, storage dispatch, k8s-control route, API, contract, or
  state-machine changes.
- No database, migration, deployment, or runtime config changes.
- No changes to mover PVC ownership, volume layout, image, or Kubernetes
  security posture.
- No broad rsync option changes, retry logic, progress callback work, or
  multi-file/performance claims.
- No unrelated E2E helper refactor.

## 6. Current Behavior

The mover Job mounts the source PVC read-only at `/mnt/source`, mounts the
target PVC at `/mnt/target`, then runs rsync directly into the normalized target
path. If the target path's parent directories do not already exist on the
target PVC, rsync fails with `No such file or directory`.

## 7. Target Behavior

The mover script creates the normalized target directory before invoking rsync.
The same restricted manifest properties remain true:

- no `hostPath` volumes
- no privileged container
- `AutomountServiceAccountToken=false`
- source PVC mount is read-only
- fixed shell command remains `/bin/sh -c`
- rsync command remains allowlisted as `rsync -a --delete -- <source> <target>`

## 8. Affected Domains

- `platform/cluster`: FastTransfer mover Job script generation only.
- `internal/e2e`: existing env-gated kind execution E2E is rerun as runtime
  proof.
- Acceptance ledgers: wording only if the live E2E passes.

## 9. Affected Files

Plan artifact:

- `docs/plan/2026-06-28-fast-transfer-mover-target-dir-fix.md`

Code Agent may edit only:

- `backend/internal/platform/cluster/fast_transfer_mover.go`
- `backend/internal/platform/cluster/fast_transfer_mover_test.go`

Already-added verification artifacts remain in scope but should not need
behavior changes:

- `backend/internal/e2e/fast_transfer_mover_execution_kind_e2e_test.go`
- `docs/plan/2026-06-28-fast-transfer-mover-execution-kind-e2e.md`

Ledger files may be edited only after the live execution E2E passes:

- `gap.md`
- `problem.md`
- `docs/acceptance/gap-analysis.md`

## 10. API / Contract Changes

None.

Existing routes and payloads are unchanged:

```text
POST /api/v1/projects/{project_id}/storage/transfers/fast-stage
POST /internal/k8s-control/fast-transfers/mover-jobs
```

## 11. Database / Migration Changes

None.

## 12. Configuration Changes

None.

The existing E2E env gate remains:

```text
TEST_LIVE_FAST_TRANSFER_MOVER_EXECUTION_KIND=1
KUBECONFIG=<path to disposable kind kubeconfig>
```

## 13. Observability Changes

None.

The E2E may continue to collect Pod logs/events on failure, but no application
logging, metrics, or tracing behavior changes.

## 14. Security Considerations

- Keep existing path validation and shell quoting.
- Use `mkdir -p` only on the already-normalized target path under
  `/mnt/target`.
- Do not introduce hostPath, privileged containers, service account token
  mounting, or writable source mounts.
- Do not expand allowed shell metacharacters or tool selection.

## 15. Implementation Steps

1. Update `fastTransferMoverScript` in
   `backend/internal/platform/cluster/fast_transfer_mover.go` to emit:

   ```sh
   set -eu
   mkdir -p "<target>"
   rsync -a --delete -- "<source>" "<target>"
   ```

   Reuse the existing `source` and `target` variables and existing `%q`
   quoting.
2. Update the focused cluster unit assertion in
   `backend/internal/platform/cluster/fast_transfer_mover_test.go` so it
   requires `mkdir -p "/mnt/target/data/target/"` in the restricted script and
   asserts the `mkdir -p` command appears before the `rsync` command.
3. Keep the existing assertion that the script still includes:

   ```text
   rsync -a --delete -- "/mnt/source/data/source/" "/mnt/target/data/target/"
   ```

4. Re-run the existing env-gated execution kind E2E from the approved prior
   plan.
5. Update ledger wording only after the live kind E2E passes, using the ledger
   boundaries in the prior approved plan.

## 16. Verification Plan

Acceptance criteria:

- `fastTransferMoverScript` creates the normalized target directory before
  running rsync.
- The unit test fails if `mkdir -p` is removed from the mover script or appears
  after `rsync`.
- The unit test still fails if the allowlisted rsync command is removed or
  changed.
- Existing security assertions remain intact: no hostPath, no privileged
  container, source read-only, and service account token disabled.
- The existing kind execution E2E passes and proves bytes move from source PVC
  to target PVC.
- Ledger wording is updated only after the live E2E passes and does not expand
  claims beyond kind default PVC binding, Pod scheduling, rsync execution, and
  one tiny file copied.

Focused unit check:

```bash
(cd backend && go test ./internal/platform/cluster -run FastTransferMover -count=1)
```

Env-unset E2E skip check:

```bash
(cd backend && go test -tags e2e ./internal/e2e -run FastTransferMoverExecutionKind -count=1 -v)
```

Targeted live kind run:

```bash
set -euo pipefail
export KUBECONFIG=/tmp/nexuspaas-fast-transfer-exec-e2e.kubeconfig
trap 'kind delete cluster --name nexuspaas-fast-transfer-exec-e2e --kubeconfig "$KUBECONFIG" || true' EXIT
kind create cluster --name nexuspaas-fast-transfer-exec-e2e --kubeconfig "$KUBECONFIG"
docker pull instrumentisto/rsync-ssh:alpine
docker pull busybox:1.36
kind load docker-image instrumentisto/rsync-ssh:alpine --name nexuspaas-fast-transfer-exec-e2e
kind load docker-image busybox:1.36 --name nexuspaas-fast-transfer-exec-e2e
kubectl --kubeconfig "$KUBECONFIG" get storageclass
(cd backend && TEST_LIVE_FAST_TRANSFER_MOVER_EXECUTION_KIND=1 KUBECONFIG="$KUBECONFIG" go test -tags e2e ./internal/e2e -run FastTransferMoverExecutionKind -count=1 -v)
```

Regression checks:

```bash
(cd backend && go test ./internal/services/k8scontrol -run FastTransferMover -count=1)
(cd backend && go test -tags e2e ./internal/e2e -run 'FastTransfer(MoverKindAdmission|StartMoverKindAdmission)' -count=1 -v)
(cd backend && go test ./...)
(cd backend && go build ./...)
git diff --check
(cd backend && make coverage)
(cd backend && make ci-sonar)
```

If the live kind run still fails, collect the mover Pod logs, Pod status, and
events before changing scope.

## 17. Rollback Plan

Revert the two code/test edits:

- `backend/internal/platform/cluster/fast_transfer_mover.go`
- `backend/internal/platform/cluster/fast_transfer_mover_test.go`

If ledgers were updated after a passing run, revert only the scoped ledger
wording in:

- `gap.md`
- `problem.md`
- `docs/acceptance/gap-analysis.md`

No API, database, migration, deployment, or config rollback is expected.

## 18. Risks and Tradeoffs

- If the mover image lacks `mkdir`, the fix will fail immediately; this is
  unlikely for the current Alpine-based rsync image.
- `mkdir -p` proves only target-directory creation for the existing single
  target path; it does not add broader filesystem recovery behavior.
- The live kind E2E can still be blocked by local cluster/image/storage-class
  setup. In that case, do not update ledgers.

## 19. Reviewer Checklist

- [ ] Plan file exists under `docs/plan/`.
- [ ] Runtime scope is limited to the mover script.
- [ ] Unit test requires target `mkdir -p` and the existing rsync command.
- [ ] Existing admission security assertions are preserved.
- [ ] No FastTransfer route or storage dispatch logic changes.
- [ ] No API, contract, database, migration, config, or deployment changes.
- [ ] Existing execution kind E2E is rerun.
- [ ] Ledger updates happen only after the live E2E passes.
- [ ] Verification includes focused unit, E2E skip, live kind E2E, regression
      tests, build, diff check, coverage, and Sonar.

## 20. Status

Status: Implemented and Reviewer approved
