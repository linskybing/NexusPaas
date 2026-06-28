# FastTransfer Mover Job Dispatch Contract

## 1. Objective

Close the next narrow FastTransfer v2 gap by adding a service-internal control-plane contract from `storage-service` to `k8s-control-service` that asks k8s-control to create one Kubernetes mover `Job` for a queued FastTransfer record.

The implementation must keep FastTransfer state ownership in storage-service and Kubernetes Job ownership in k8s-control-service. `startFastTransfer` must still return `202 Accepted` after creating or finding the storage-owned record, even when mover dispatch is not configured or fails.

## 2. Background

Current state:

- `backend/internal/services/storage/handler.go` registers `POST /api/v1/projects/{id}/storage/transfers/fast-stage`.
- `startFastTransfer` creates a queued record, emits `FastTransferChanged` and `FastTransferQueued`, and returns the record.
- `backend/internal/services/storage/fast_transfer_state.go` owns statuses `queued`, `running`, `succeeded`, `failed`, `cancelled`, plus progress updates through `/internal/storage/projects/{project_id}/transfers/{targetNamespace}/{name}/progress`.
- `backend/internal/services/k8scontrol/handler.go` exposes adapter-style/resource handlers only.
- `backend/internal/platform/cluster/docker_cleanup.go` shows the existing cluster facade pattern for Kubernetes batch resources with fake-client tests.
- `backend/internal/platform/service_client.go` provides `platform.NewInternalJSONClient` for local/remote service-to-service JSON calls.

This slice adds dispatch intent only. It does not prove that bytes moved or that Kubernetes scheduled/runs the Job.

## 3. Source References

- `backend/internal/services/storage/handler.go`
- `backend/internal/services/storage/fast_transfer_state.go`
- `backend/internal/services/storage/storage_repository.go`
- `backend/internal/services/storage/handler_test.go`
- `backend/internal/services/storage/fast_transfer_state_test.go`
- `backend/internal/services/k8scontrol/handler.go`
- `backend/internal/services/k8scontrol/spec.go`
- `backend/internal/services/k8scontrol/handler_test.go`
- `backend/internal/platform/cluster/docker_cleanup.go`
- `backend/internal/platform/cluster/docker_cleanup_test.go`
- `backend/internal/platform/service_client.go`
- `backend/internal/contracts/command_fixtures_test.go`
- `backend/docs/internal-command-contracts.md`

## 4. Assumptions

- `SERVICE_URLS` is the existing configuration surface for remote service calls; storage-service should call owner `k8s-control-service` through `platform.NewInternalJSONClient`.
- Local co-hosted `SERVICE_NAME=all` behavior may use the same internal client local path.
- The mover container image is runtime configuration owned by k8s-control-service. Add the smallest config knob needed if no existing image setting fits.
- The initial request shape can be additive and map-based where existing FastTransfer payload fields are still settling.
- The Kubernetes Job can use PVC names and paths supplied by storage-service without validating live PVC existence in this slice.

## 5. Non-Goals

- No Go byte mover.
- No live byte-copy assertion.
- No CSI mount, StorageClass binding, scheduler success, kind requirement, storage GA, or Full GA claim.
- No new storage state machine statuses.
- No database migration unless implementation discovers the existing schemaless record store cannot persist additive dispatch fields.
- No broad k8s-control adapter rewrite.
- No generic Job framework beyond the one FastTransfer mover Job contract.

## 6. Current Behavior

`startFastTransfer`:

1. Authenticates the project manager.
2. Decodes the request.
3. Computes idempotency key and fingerprint.
4. Returns the existing record for repeated same-key/same-fingerprint requests.
5. Creates a queued FastTransfer record for new requests.
6. Emits queue events.
7. Does not dispatch Kubernetes work.

k8s-control-service has no internal FastTransfer mover route and no cluster facade method for creating a mover `Job`.

## 7. Target Behavior

New request flow:

1. `startFastTransfer` keeps current validation, idempotency, record creation, and event behavior.
2. If the idempotency lookup returns an existing record, return it without dispatching.
3. If a new queued record is created and k8s-control is configured, storage-service calls k8s-control through `platform.NewInternalJSONClient`.
4. k8s-control validates service-key auth and request fields, builds a Kubernetes `batch/v1 Job`, and creates it through the cluster facade.
5. The Job runs a shell command that invokes a proven tool such as `rsync`, `rclone`, or `tar`; do not implement copy logic in Go.
6. storage-service records additive dispatch metadata where available:
   - `dispatch_status`: `not_configured`, `submitted`, `unavailable`, or `failed`
   - `dispatch_error`: blank or a short failure string
   - `mover_job_namespace`
   - `mover_job_name`
7. Dispatch failure does not fail `startFastTransfer`; response remains `202 Accepted` with the queued record plus dispatch metadata.

## 8. Affected Domains

- `storage-service`: FastTransfer record owner, state machine owner, dispatch client consumer.
- `k8s-control-service`: Kubernetes Job manifest owner and cluster API caller.
- `platform`: configuration only if a mover image/env setting is needed.
- `contracts`: internal command fixture for the new service-to-service write contract.

## 9. Affected Files

Expected runtime files:

- `backend/internal/services/k8scontrol/handler.go`
- `backend/internal/services/k8scontrol/spec.go`
- `backend/internal/services/k8scontrol/fast_transfer_mover.go`
- `backend/internal/platform/cluster/fast_transfer_mover.go`
- `backend/internal/services/storage/handler.go`
- `backend/internal/services/storage/fast_transfer_dispatch.go`
- `backend/internal/platform/config.go` only if adding `FAST_TRANSFER_MOVER_IMAGE`

Expected test files:

- `backend/internal/platform/cluster/fast_transfer_mover_test.go`
- `backend/internal/services/k8scontrol/fast_transfer_mover_test.go`
- `backend/internal/services/storage/fast_transfer_dispatch_test.go` or focused additions to `backend/internal/services/storage/handler_test.go`
- `backend/internal/contracts/command_fixtures_test.go`
- `backend/internal/services/k8scontrol/spec_test.go`

Expected fixture/docs files:

- `backend/internal/contracts/fixtures/commands/v1/k8s-control-dispatch-fast-transfer-mover.json`
- `backend/docs/internal-command-contracts.md`
- `docs/acceptance/gap-analysis.md` only if the repo gate requires acceptance ledger wording for this slice
- `problem.md` only if the repo gate requires acceptance ledger wording for this slice

Do not edit unrelated runtime/test files.

## 10. API / Contract Changes

Add one service-key-gated internal command route:

```text
POST /internal/k8s-control/fast-transfers/mover-jobs
Owner: k8s-control-service
Consumer: storage-service
Auth: scoped X-Service-Name/X-Service-Key or legacy X-Service-Key
```

Request shape:

```json
{
  "project_id": "P1",
  "transfer_id": "P1:project-P1:copy1",
  "target_namespace": "project-P1",
  "name": "copy1",
  "source": {
    "namespace": "project-P1",
    "pvc": "dataset-pvc",
    "path": "/data/source"
  },
  "target": {
    "namespace": "project-P1",
    "pvc": "scratch-pvc",
    "path": "/data/target"
  },
  "tool": "rsync",
  "progress_callback": {
    "path": "/internal/storage/projects/P1/transfers/project-P1/copy1/progress"
  }
}
```

Response shape:

```json
{
  "namespace": "project-P1",
  "name": "fast-transfer-copy1",
  "action": "created"
}
```

Allowed actions:

- `created`
- `already_exists`
- `invalid`
- `degraded`
- `failed`

Contract rules:

- The command is idempotent by Kubernetes Job identity: same namespace/name returns `already_exists` only when the existing Job has matching NexusPaaS ownership labels/annotations for the same `transfer_id`.
- A conflicting existing Job must not be mutated.
- Request and response readers must tolerate additive fields.
- Fixture must be added under `backend/internal/contracts/fixtures/commands/v1/` and registered in `backend/internal/contracts/command_fixtures_test.go`.
- Add the route to `k8scontrol.Spec()` using `shared.ServiceInternal()`.

## 11. Database / Migration Changes

No migration is planned.

FastTransfer records are map-backed in the current repository layer, so dispatch metadata should be additive fields on the existing record. If Code Agent finds a strict schema/migration requirement in the active backing store, stop and ask Reviewer Agent to approve a narrowed migration plan before editing migrations.

## 12. Configuration Changes

Use existing storage-service configuration:

- `SERVICE_URLS` must include `k8s-control-service=<base-url>` for remote dispatch.
- A sendable service identity must be configured: scoped `SERVICE_IDENTITY_NAME` + `SERVICE_IDENTITY_KEY`, or legacy `SERVICE_API_KEY`.

Add only if needed for k8s-control Job image selection:

- `FAST_TRANSFER_MOVER_IMAGE`

Do not add feature flags unless required by tests or existing config validation. Absence of a k8s-control URL or sendable service identity means dispatch is `not_configured` and `startFastTransfer` remains accepted.

## 13. Observability Changes

Add concise structured logs only:

- storage-service logs dispatch result/failure with `transfer_id`, `project_id`, `dispatch_status`, and no paths containing secrets.
- k8s-control logs Job create result with namespace/name/action/reason.

No new metrics are required for this narrow contract slice.

## 14. Security Considerations

- Internal k8s-control route must require service-key auth. Use the same header and validation style as existing internal command/read contracts.
- Do not accept privileged containers, hostPath mounts, raw command override, or arbitrary shell from storage-service.
- Build the command from validated structured fields. The Job may use `/bin/sh -c`, but inputs must be constrained/quoted enough to prevent shell injection.
- Do not include credentials, tokens, passwords, raw service keys, or local host paths in fixtures or logs.
- Job manifest must set `AutomountServiceAccountToken: false` unless a later live Kubernetes slice proves a service account is required.

## 15. Implementation Steps

1. Add the cluster facade helper in `backend/internal/platform/cluster/fast_transfer_mover.go`.
   - Define minimal options/result structs.
   - Validate namespace/name/PVC/path/tool.
   - Build one `batch/v1 Job` with two PVC volumes and one container.
   - Container command invokes a proven tool (`rsync` by default) through a shell script.
   - Create the Job through `Clientset().BatchV1().Jobs(namespace).Create`.
   - Return `already_exists` only for matching managed transfer ownership.
2. Add k8s-control handler in `backend/internal/services/k8scontrol/fast_transfer_mover.go`.
   - Decode JSON.
   - Require service auth.
   - Map request to cluster facade options.
   - Return `201 Created` for created, `200 OK` for already exists, `422` for invalid, `502` for degraded/failed.
3. Register the route in `backend/internal/services/k8scontrol/handler.go`.
4. Add route metadata to `backend/internal/services/k8scontrol/spec.go` with `shared.ServiceInternal()`.
5. Add storage dispatch client in `backend/internal/services/storage/fast_transfer_dispatch.go`.
   - Use `platform.NewInternalJSONClient(app, "k8s-control-service")`.
   - Treat missing `SERVICE_URLS["k8s-control-service"]` or missing sendable service identity as `not_configured` when k8s-control is not co-hosted.
   - Convert non-2xx/transport errors to dispatch metadata, not handler failure.
6. Update `startFastTransfer` only after successful new record creation.
   - Do not dispatch for existing idempotency record.
   - Apply additive dispatch metadata to the newly created record before returning.
   - Preserve current events and response status.
7. Add the command fixture and update fixture allowlist/docs.
8. Add focused tests.

## 16. Verification Plan

Focused tests:

```text
cd backend && go test ./internal/platform/cluster -run FastTransferMover
cd backend && go test ./internal/services/k8scontrol -run FastTransferMover
cd backend && go test ./internal/services/storage -run 'FastTransfer|ProjectBindingsTransfers'
cd backend && go test ./internal/contracts -run CommandFixtures
```

Broader local gates:

```text
cd backend && go test ./internal/platform/... ./internal/services/k8scontrol/... ./internal/services/storage/... ./internal/contracts/...
cd backend && go test ./...
cd backend && go build ./...
git diff --check
```

Repo final gates expected by Reviewer Agent:

```text
cd backend && make coverage
cd backend && make ci-sonar
```

Do not require kind for normal verification. A live kind test is out of scope for this slice.

## 17. Rollback Plan

Revert:

- new k8s-control internal route/handler/spec entry;
- new cluster FastTransfer mover helper;
- storage dispatch client and `startFastTransfer` dispatch call;
- command fixture and fixture allowlist/docs entries;
- tests added for this slice;
- any optional acceptance wording.

No database rollback is expected.

## 18. Risks and Tradeoffs

- Fake Kubernetes tests prove manifest creation, not scheduling or byte movement.
- Shelling out to `rsync`/`rclone`/`tar` is intentional; writing a Go mover would expand scope and ownership.
- Adding dispatch metadata to the record is the smallest backward-compatible status surface. Do not create a separate dispatch table in this slice.
- Job idempotency must be tied to the storage transfer identity; otherwise retries can create duplicate movers.
- If the initial payload lacks enough source/target fields to build a safe mover Job, Code Agent should return a clear dispatch failure while preserving `202 Accepted`, not invent storage resolution logic in k8s-control.

## 19. Reviewer Checklist

- [ ] Plan file exists under `docs/plan/`.
- [ ] k8s-control owns Kubernetes Job creation; storage-service owns FastTransfer records/state.
- [ ] `startFastTransfer` still returns `202 Accepted` if dispatch is unavailable.
- [ ] Same idempotency key/same fingerprint returns existing record and does not dispatch again.
- [ ] k8s-control route is service-key-gated and registered in `Spec()`.
- [ ] Command fixture is added and validated if the route is added.
- [ ] Job manifest shells out to a proven tool; no Go byte mover is introduced.
- [ ] Tests cover Job manifest creation, configured dispatch call, no-op/not-configured behavior, and no duplicate idempotent dispatch.
- [ ] Verification commands and results are reported.
- [ ] No claim is made for live bytes moved, CSI mount, scheduler success, storage GA, or Full GA.
- [ ] SonarScanner Quality Gate result is reported by Reviewer Agent.

## 20. Status

Status: Approved

Approved by Reviewer Agent for Code Agent implementation.

## Code Agent Instructions

Implement only this approved plan. Keep the diff surgical. If a required change falls outside the files or behavior listed here, stop and ask Reviewer Agent for a scope decision before editing.
