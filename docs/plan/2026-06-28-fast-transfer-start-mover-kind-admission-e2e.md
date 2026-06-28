# FastTransfer Start-to-Mover Live Admission E2E

## 1. Objective

Add one env-gated `//go:build e2e` test proving the external storage-service
FastTransfer start route dispatches through the service boundary to
k8s-control and results in a live Kubernetes API-admitted mover `Job`.

The proof is narrow: storage HTTP start route -> storage internal dispatch
client -> k8s-control HTTP internal route -> Kubernetes Job object admission.
It must not claim PVC binding, Pod scheduling, rsync execution, bytes moved,
progress callback behavior, CSI, storage GA, or Full GA.

## 2. Background

Committed slices on `storage-data-path`:

- `49148c6 storage: dispatch fast transfer mover jobs` added storage
  FastTransfer dispatch to k8s-control and additive dispatch metadata.
- `4b814dd storage: add fast transfer mover kind admission e2e` added an
  env-gated kind E2E proving k8s-control can create the restricted mover Job
  and repeat returns `already_exists`.

Remaining gap:

- No live proof starts at
  `POST /api/v1/projects/{id}/storage/transfers/fast-stage` and crosses the
  storage-service to k8s-control service boundary before live Job admission.

## 3. Source References

- `backend/internal/services/storage/handler.go`
- `backend/internal/services/storage/fast_transfer_dispatch.go`
- `backend/internal/services/storage/fast_transfer_state.go`
- `backend/internal/services/storage/fast_transfer_dispatch_test.go`
- `backend/internal/services/k8scontrol/fast_transfer_mover.go`
- `backend/internal/platform/cluster/fast_transfer_mover.go`
- `backend/internal/e2e/fast_transfer_mover_kind_admission_e2e_test.go`
- `backend/internal/e2e/storage_data_plane_kind_admission_e2e_test.go`
- `backend/internal/e2e/harness_test.go`
- `docs/acceptance/gap-analysis.md`
- `gap.md`
- `problem.md`

## 4. Assumptions

- Normal CI must skip this test unless the explicit live env var is set.
- The k8s-control mover-kind E2E helpers live in package `e2e` and may be
  reused directly if they remain unexported but same-package.
- Storage can be started with in-memory `platform.Store` and `EventBus`.
- Storage test app auth is deliberately handler-scoped, not platform-auth
  coverage: use `RequireAuth: false` and send `X-User-ID` for the manager user
  so `requireUser` and project-manager checks still execute.
- The storage test app must run as `ServiceName: "storage-service"`, not
  `ServiceName: "all"`, and should use `ServiceFallbackDisabled: true` so the
  k8s-control dispatch crosses the configured `ServiceURLs` HTTP boundary.
- The live proof should use a real `httptest` server for storage-service and a
  separate `httptest` server for k8s-control so dispatch crosses HTTP.

## 5. Non-Goals

- No runtime code changes.
- No new service contract or route.
- No PVC binding, PV creation, CSI, StorageClass, or scheduler assertion.
- No Pod Running/Completed wait.
- No rsync execution, byte movement, checksum, or file-content assertion.
- No progress callback assertion.
- No broad FastTransfer lifecycle test beyond queued start and dispatch.
- No storage GA or Full GA claim.

## 6. Current Behavior

- Storage start dispatch has focused fake/local tests.
- k8s-control mover Job admission has an env-gated live kind test.
- The combined storage start-to-k8s-control-to-live-Kubernetes path lacks live
  E2E evidence.

## 7. Target Behavior

With `TEST_LIVE_FAST_TRANSFER_START_MOVER_KIND_ADMISSION=1` and a kubeconfig
pointing at a disposable kind cluster, the new E2E:

1. Skips cleanly when the env var is unset.
2. Calls `requireLiveKubeconfig(t)`.
3. Creates a live cluster client with `cluster.NewFromEnv("proj")`.
4. Calls `cl.Ping(ctx)`.
5. Creates a disposable namespace and registers cleanup.
6. Starts a k8s-control app/server with the live cluster and service auth.
7. Starts a storage-service app/server with seeded project manager access and
   `ServiceURLs["k8s-control-service"]` pointing at the k8s-control server.
8. Posts to the real storage HTTP route:

```text
POST /api/v1/projects/{project_id}/storage/transfers/fast-stage
```

9. Asserts the storage response is `202 Accepted` and contains:
   - `status=queued`
   - `dispatch_status=submitted`
   - `mover_job_namespace`
   - `mover_job_name`
10. Reads the live Kubernetes Job and verifies the restricted manifest shape.
11. Repeats the same fast-stage request with the same idempotency key and
    payload, then asserts the existing record is returned and no duplicate Job
    exists.

## 8. Affected Domains

- `storage-service`: external FastTransfer start route and dispatch metadata.
- `k8s-control-service`: internal mover Job command route.
- `platform/cluster`: live Kubernetes Job API admission.
- Acceptance ledgers: scoped evidence wording only if updated.

## 9. Affected Files

Code Agent may edit only:

- `backend/internal/e2e/fast_transfer_start_mover_kind_admission_e2e_test.go`
- `docs/acceptance/gap-analysis.md` only if scoped evidence is recorded
- `gap.md` only if scoped evidence is recorded
- `problem.md` only if scoped evidence is recorded

Plan artifact:

- `docs/plan/2026-06-28-fast-transfer-start-mover-kind-admission-e2e.md`

Runtime code changes are out of scope. If the test exposes a runtime defect,
stop and request Reviewer Agent approval for a separate fix plan.

## 10. API / Contract Changes

None. The test exercises existing routes:

```text
POST /api/v1/projects/{project_id}/storage/transfers/fast-stage
POST /internal/k8s-control/fast-transfers/mover-jobs
```

Storage request payload should include synthetic DNS/path-safe values:

- `name`
- `target_namespace`
- `source.namespace`, `source.pvc`, `source.path`
- `target.namespace`, `target.pvc`, `target.path`
- `tool=rsync`
- `idempotency_key` or `Idempotency-Key` header

## 11. Database / Migration Changes

None. Use in-memory store records only.

## 12. Configuration Changes

No checked-in runtime config changes.

Test-only env gate:

```text
TEST_LIVE_FAST_TRANSFER_START_MOVER_KIND_ADMISSION=1
KUBECONFIG=<path to kind kubeconfig>
```

The storage test app must set:

- `ServiceName: "storage-service"`
- `RequireAuth: false`; send `X-User-ID` on the storage route request. This is
  handler/project-access evidence, not platform authentication evidence.
- `ServiceFallbackDisabled: true`
- `ServiceURLs["k8s-control-service"] = <k8s-control httptest URL>`
- `ServiceAPIKey` or scoped `ServiceIdentityName`/`ServiceIdentityKey` so
  `platform.NewInternalJSONClient` sends service auth

The k8s-control test app must set matching service auth and use
`platform.WithCluster(cl)`.

## 13. Observability Changes

None.

## 14. Security Considerations

- Use a unique disposable namespace and delete it in `t.Cleanup`.
- Do not log kubeconfig content, service keys, or credentials.
- Use synthetic project/user IDs and safe absolute paths.
- Exercise storage user/project authorization and k8s-control service-key auth.
- Assert the live Job has no privileged container and no hostPath volume.

## 15. Implementation Steps

1. Add `backend/internal/e2e/fast_transfer_start_mover_kind_admission_e2e_test.go`
   with `//go:build e2e`.
2. Gate on `TEST_LIVE_FAST_TRANSFER_START_MOVER_KIND_ADMISSION=1`; otherwise
   `t.Skip`.
3. Call `requireLiveKubeconfig(t)`, `cluster.NewFromEnv("proj")`, and
   `cl.Ping(ctx)`.
4. Generate short DNS-safe IDs for project, user, transfer, namespace, and PVCs.
5. Create the disposable namespace and register cleanup.
6. Start a k8s-control `httptest` server using the same service-auth/live-cluster
   setup as `fast_transfer_mover_kind_admission_e2e_test.go`.
7. Start a storage-service `httptest` server with a shared in-memory store/event
   bus and remote k8s-control `ServiceURLs` entry.
8. Register storage-service routes/specs as needed by existing E2E/app patterns.
9. Seed only required storage-local projection records for project manager
   access:
   - `storage-service:storage_projects`
   - `storage-service:storage_project_members` with manager role
   Do not rely only on `org-project-service:*` rows; storage access checks read
   local projections when the storage service is isolated.
10. POST the storage fast-stage route over HTTP with the manager user header and
    the JSON payload.
11. Decode the platform response envelope and assert `202 Accepted`.
12. Assert returned record has `queued` status and submitted dispatch metadata.
13. Assert `FastTransferQueued` exists in the event bus/outbox if existing E2E
    helpers make this cheap; otherwise leave event assertion optional.
14. Read the live Job using `BatchV1().Jobs(namespace).Get`.
15. Reuse `assertFastTransferMoverAdmissionJob` and
    `assertFastTransferMoverAdmissionJobCount` if appropriate; otherwise add the
    same local assertions.
16. Repeat the same fast-stage request with the same idempotency key and same
    payload.
17. Assert the repeat returns `202 Accepted`, the same transfer ID/job name, and
    exactly one Job for that transfer.
18. Update acceptance ledgers only after the live test passes, with wording
    limited to env-gated live storage fast-stage-to-k8s-control mover Job
    admission evidence.

## 16. Acceptance Criteria

- Env-unset E2E skips cleanly.
- Env-set live kind E2E starts at the storage HTTP fast-stage route.
- Storage dispatch crosses to a separate k8s-control HTTP server.
- Storage response is `202 Accepted`, `status=queued`, and
  `dispatch_status=submitted`.
- Response includes live mover Job namespace/name.
- Live Kubernetes Job exists and matches the restricted mover manifest.
- Same idempotency key/payload returns existing record and does not create a
  duplicate Job, unless Reviewer approves making this optional after a concrete
  complexity finding.
- No runtime code is changed.
- Docs, if changed, avoid overclaiming beyond live API admission.

## 17. Verification Plan

Env-unset skip:

```bash
cd backend && go test -tags e2e ./internal/e2e -run FastTransferStartMoverKindAdmission -count=1 -v
```

Targeted live kind run:

```bash
kind create cluster --name nexuspaas-fast-transfer-start-e2e
kubectl config use-context kind-nexuspaas-fast-transfer-start-e2e
cd backend && TEST_LIVE_FAST_TRANSFER_START_MOVER_KIND_ADMISSION=1 go test -tags e2e ./internal/e2e -run FastTransferStartMoverKindAdmission -count=1 -v
kind delete cluster --name nexuspaas-fast-transfer-start-e2e
```

Focused non-live tests:

```bash
cd backend && go test ./internal/services/storage -run FastTransfer
cd backend && go test ./internal/services/k8scontrol -run FastTransferMover
cd backend && go test ./internal/platform/cluster -run FastTransferMover
cd backend && go test -tags e2e ./internal/e2e -run FastTransferMoverKindAdmission -count=1
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

- `backend/internal/e2e/fast_transfer_start_mover_kind_admission_e2e_test.go`

Revert only scoped wording changes in:

- `docs/acceptance/gap-analysis.md`
- `gap.md`
- `problem.md`

No runtime, database, migration, contract, or deployment rollback is expected.

## 19. Risks and Tradeoffs

- Live kind evidence is environment-sensitive, so normal CI must rely on clean
  skip behavior and existing fake-client tests.
- This proves API object admission only; a Kubernetes Job can be admitted and
  still fail scheduling or execution.
- Reusing mover-kind assertion helpers keeps this slice small; duplicate local
  assertions are acceptable only if reuse becomes awkward.
- Header-mode auth setup is intentionally limited to handler/project-access
  evidence; seed storage-local project membership and exercise the storage
  route rather than calling the handler directly.

## 20. Reviewer Checklist

- [ ] Plan file exists under `docs/plan/`.
- [ ] Test file is `//go:build e2e`.
- [ ] Test is gated by `TEST_LIVE_FAST_TRANSFER_START_MOVER_KIND_ADMISSION=1`.
- [ ] Test uses `requireLiveKubeconfig(t)`, `cluster.NewFromEnv("proj")`, and
      `cl.Ping`.
- [ ] Test uses separate storage-service and k8s-control `httptest` servers.
- [ ] Storage calls k8s-control through `ServiceURLs` and service auth.
- [ ] Test exercises the real storage HTTP route, not `startFastTransfer`
      directly.
- [ ] Storage-local project manager access records are seeded.
- [ ] Storage app uses `ServiceName: "storage-service"`, not `all`, and
      `ServiceFallbackDisabled: true` with a k8s-control `ServiceURLs` entry.
- [ ] Response asserts queued state and submitted dispatch metadata.
- [ ] Live Job manifest restrictions are verified.
- [ ] Idempotency replay does not create a duplicate Job, or any omission is
      explicitly justified and approved.
- [ ] No runtime code is changed.
- [ ] Docs, if changed, claim only env-gated live start-to-mover Job admission.
- [ ] Verification results include skip, live kind when available, focused
      tests, full test/build, coverage, and Sonar.

## 21. Status

Status: Approved

Approved by Reviewer Agent with constraints: test-only, storage app
`ServiceName: "storage-service"` with `RequireAuth: false` and
`ServiceFallbackDisabled: true`, storage-local projection seeds only, separate
k8s-control HTTP server with service auth, optional kind env gate, temporary
kubeconfig for live verification, and no execution/GA overclaims.
