# FastTransfer Mover Kind Admission E2E

## 1. Objective

Add one env-gated `//go:build e2e` test proving the FastTransfer mover Job
contract reaches a live Kubernetes API server and creates the expected
`batch/v1 Job` object through k8s-control, with repeat requests returning the
idempotent `already_exists` response.

The proof is intentionally narrow: Kubernetes API admission and Job manifest
shape only. It must not claim PVC binding, Pod scheduling, rsync execution,
progress callbacks, bytes moved, CSI, storage GA, or Full GA.

## 2. Background

Latest commit `49148c6 storage: dispatch fast transfer mover jobs` added:

- `POST /internal/k8s-control/fast-transfers/mover-jobs`
- k8s-control `shared.ServiceInternal()` route metadata
- `cluster.EnsureFastTransferMoverJob`
- restricted rsync `batch/v1 Job` creation with PVC-only volumes, source
  read-only, target writable, `RestartPolicy=Never`, and
  `AutomountServiceAccountToken=false`

Current tests use fake Kubernetes only. Reviewer scoped the previous slice to
no kind. This slice adds optional live kind evidence using the existing pattern
from `backend/internal/e2e/storage_data_plane_kind_admission_e2e_test.go`.

## 3. Source References

- `backend/internal/platform/cluster/fast_transfer_mover.go`
- `backend/internal/platform/cluster/fast_transfer_mover_test.go`
- `backend/internal/services/k8scontrol/fast_transfer_mover.go`
- `backend/internal/services/k8scontrol/fast_transfer_mover_test.go`
- `backend/internal/services/k8scontrol/handler.go`
- `backend/internal/services/k8scontrol/spec.go`
- `backend/internal/e2e/storage_data_plane_kind_admission_e2e_test.go`
- `backend/internal/e2e/harness_test.go`
- `docs/acceptance/gap-analysis.md`
- `problem.md`

## 4. Assumptions

- Branch is `storage-data-path`.
- Normal test runs must skip without a live kubeconfig and explicit env var.
- `kind` is allowed only for this optional live E2E verification.
- Kubernetes Job object creation is enough for this slice.
- Placeholder PVC objects are optional because this test does not prove binding
  or scheduling.

## 5. Non-Goals

- No production/runtime code changes.
- No storage-service FastTransfer start flow test in this slice.
- No PVC binding, PV creation, CSI, StorageClass, or scheduler assertion.
- No Pod Running/Completed wait.
- No rsync execution or file-content/byte-movement assertion.
- No progress callback assertion.
- No storage GA or Full GA claim.
- No CI requirement for kind.

## 6. Current Behavior

- Fake-client tests prove k8s-control and the cluster facade build the restricted
  mover Job.
- No live E2E proves the mover Job is accepted by a real Kubernetes API server.

## 7. Target Behavior

With `TEST_LIVE_FAST_TRANSFER_MOVER_KIND_ADMISSION=1` and a kubeconfig pointing
at a disposable kind cluster, the new E2E:

1. Calls `requireLiveKubeconfig(t)`.
2. Creates a real cluster client with `cluster.NewFromEnv("proj")`.
3. Pings the live Kubernetes API.
4. Creates a unique disposable namespace.
5. Optionally creates placeholder source/target PVC objects in that namespace.
6. Starts a k8s-control app with the live cluster client and service-key auth.
7. Posts to `/internal/k8s-control/fast-transfers/mover-jobs` with valid
   same-namespace source/target PVC names and safe paths.
8. Asserts HTTP `201 Created` for first create.
9. Reads the live Job from Kubernetes and verifies the manifest.
10. Repeats the POST and asserts HTTP `200 OK` with `action=already_exists`
    and still exactly one Job for that transfer.

## 8. Affected Domains

- `k8s-control-service`: internal mover Job command route.
- `platform/cluster`: live API use of `EnsureFastTransferMoverJob`.
- E2E acceptance evidence: optional kind-only proof wording.

## 9. Affected Files

Code Agent may edit only:

- `backend/internal/e2e/fast_transfer_mover_kind_admission_e2e_test.go`
- `docs/acceptance/gap-analysis.md` only if acceptance ledger wording is needed
- `gap.md` only for the scoped Done-table evidence row approved by Reviewer
- `problem.md` only if acceptance ledger wording is needed

Plan artifact:

- `docs/plan/2026-06-28-fast-transfer-mover-kind-admission-e2e.md`

Runtime code changes are out of scope. If this test exposes a real runtime
defect, stop and request Reviewer Agent approval for a narrowed fix plan.

## 10. API / Contract Changes

None. The test exercises the existing internal command:

```text
POST /internal/k8s-control/fast-transfers/mover-jobs
```

The request should use synthetic values such as:

- `project_id`: generated short ID
- `transfer_id`: generated short transfer identity
- `target_namespace`: disposable namespace
- `name`: `copy-<suffix>` or equivalent DNS-safe value
- `source.namespace` and `target.namespace`: same disposable namespace
- `source.pvc` and `target.pvc`: DNS-safe placeholder PVC names
- `source.path` and `target.path`: safe absolute paths
- `tool`: `rsync`

## 11. Database / Migration Changes

None.

## 12. Configuration Changes

No checked-in runtime config changes.

Test-only env gate:

```text
TEST_LIVE_FAST_TRANSFER_MOVER_KIND_ADMISSION=1
KUBECONFIG=<path to kind kubeconfig>
```

Optional local setup:

```bash
kind create cluster --name nexuspaas-fast-transfer-e2e
kubectl config use-context kind-nexuspaas-fast-transfer-e2e
```

## 13. Observability Changes

None.

## 14. Security Considerations

- Use a disposable namespace with a unique suffix.
- Delete namespace and any placeholder PVCs/Jobs in `t.Cleanup`.
- Do not log kubeconfig content or service keys.
- Exercise service-key auth; do not call the handler directly.
- Assert no privileged container and no hostPath volume.

## 15. Implementation Steps

1. Add `backend/internal/e2e/fast_transfer_mover_kind_admission_e2e_test.go`
   with `//go:build e2e`.
2. Gate the test on `TEST_LIVE_FAST_TRANSFER_MOVER_KIND_ADMISSION=1`;
   otherwise `t.Skip`.
3. Call `requireLiveKubeconfig(t)`, `cluster.NewFromEnv("proj")`, and
   `cl.Ping(ctx)`.
4. Create a short suffix and namespace such as `ft-mover-<suffix>`.
5. Create the namespace and register cleanup.
6. Optionally create source/target PVC API objects if that makes the Job
   manifest easier to inspect; do not wait for binding.
7. Start a k8s-control `platform.App` with:
   - `ServiceName: "k8s-control-service"`
   - `RequireAuth: true`
   - `ServiceAPIKey` or scoped service identity matching existing service-auth
     test patterns
   - `platform.WithCluster(cl)`
8. Register k8s-control routes and serve the app through `httptest`.
9. POST the internal mover request over HTTP with `X-Service-Key`.
10. Assert first response status is `201 Created` and action is `created`.
11. Read the Job from `BatchV1().Jobs(namespace).Get`.
12. Assert:
    - Job namespace/name match the response.
    - Managed labels include platform/backend, platform part, mover component,
      and k8s-control owner.
    - Annotations include managed resource and the transfer ID.
    - Exactly one container named `fast-transfer-mover`.
    - Command is `/bin/sh -c`.
    - Args include `set -eu` and `rsync -a --delete --`.
    - Volumes are PVC-only, one source and one target.
    - Source PVC volume and source mount are read-only.
    - Target PVC volume and mount are writable.
    - `RestartPolicy=Never`.
    - `AutomountServiceAccountToken=false`.
    - No container is privileged.
    - No `hostPath` volume exists.
13. Repeat the POST and assert status `200 OK`, action `already_exists`, and no
    duplicate Job.
14. Update acceptance/problem wording only after the live test passes, and only
    with scoped language: env-gated live Kubernetes API admission evidence for
    FastTransfer mover Job creation.

## 16. Acceptance Criteria

- Env-unset E2E skips cleanly.
- Env-set live kind E2E creates or reuses one mover Job through the k8s-control
  HTTP route.
- Live Job manifest matches the restricted fake-client contract.
- Idempotent repeat returns `already_exists` and does not create a duplicate.
- Docs, if edited, explicitly avoid claims about PVC binding, scheduling, rsync
  execution, bytes moved, progress callback, CSI, storage GA, or Full GA.

## 17. Verification Plan

Env-unset skip:

```bash
cd backend && go test -tags e2e ./internal/e2e -run FastTransferMoverKindAdmission -count=1 -v
```

Targeted live kind run:

```bash
kind create cluster --name nexuspaas-fast-transfer-e2e
kubectl config use-context kind-nexuspaas-fast-transfer-e2e
cd backend && TEST_LIVE_FAST_TRANSFER_MOVER_KIND_ADMISSION=1 go test -tags e2e ./internal/e2e -run FastTransferMoverKindAdmission -count=1 -v
kind delete cluster --name nexuspaas-fast-transfer-e2e
```

Focused non-live tests:

```bash
cd backend && go test ./internal/platform/cluster -run FastTransferMover
cd backend && go test ./internal/services/k8scontrol -run FastTransferMover
cd backend && go test ./internal/services/storage -run FastTransfer
```

Broader gates:

```bash
cd backend && go test ./...
cd backend && go build ./...
git diff --check
cd backend && make coverage
cd backend && make ci-sonar
```

## 18. Rollback Plan

Delete:

- `backend/internal/e2e/fast_transfer_mover_kind_admission_e2e_test.go`

Revert only scoped wording changes in:

- `docs/acceptance/gap-analysis.md`
- `problem.md`

No runtime, database, migration, or deployment rollback is expected.

## 19. Risks and Tradeoffs

- Live kind coverage is optional and environment-sensitive, so normal CI must
  rely on the skip path plus fake-client tests.
- Kubernetes API admission does not prove a Job can run.
- Placeholder PVCs can make the object graph clearer but must not be described
  as bound storage.
- Keeping the slice at k8s-control avoids retesting storage dispatch already
  covered by the prior contract slice.

## 20. Reviewer Checklist

- [ ] Plan file exists under `docs/plan/`.
- [ ] Test file is `//go:build e2e`.
- [ ] Test is gated by `TEST_LIVE_FAST_TRANSFER_MOVER_KIND_ADMISSION=1`.
- [ ] Test uses `requireLiveKubeconfig(t)`, `cluster.NewFromEnv("proj")`, and
      `cl.Ping`.
- [ ] Test calls the k8s-control HTTP route with service-key auth.
- [ ] Test creates and cleans a disposable namespace.
- [ ] Test asserts live Job labels, annotations, command/script, PVC-only
      volumes, source read-only, target writable, `RestartPolicy=Never`,
      `AutomountServiceAccountToken=false`, no privileged container, and no
      hostPath.
- [ ] Repeat POST proves idempotent `already_exists` without duplicate Job.
- [ ] No runtime code is changed.
- [ ] Docs, if changed, claim only env-gated live Kubernetes API admission.
- [ ] Verification results include skip, live kind when available, focused
      tests, full test/build, coverage, and Sonar.

## 21. Status

Status: Approved

Approved by Reviewer Agent with constraints: test-only, route-level HTTP
exercise, optional kind env gate, no placeholder PVCs unless required, no
runtime changes, and no storage execution claims.
