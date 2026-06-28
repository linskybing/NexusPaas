# FastTransfer Progress Callback

## 1. Objective

Add the smallest useful FastTransfer mover status callback path:

```text
mover Job -> POST storage progress endpoint -> fast_transfer_records status
```

The target proof is narrow: when a mover Job is given a reachable storage
callback URL and service key, it reports `running` before rsync and `succeeded`
after rsync. A failed rsync reports `failed` best-effort before exiting non-zero.

This does not claim resume, checksum, accurate byte accounting, performance,
multi-node behavior, or storage GA.

## 2. Current State

- Storage already exposes:
  `POST /internal/storage/projects/{project_id}/transfers/{targetNamespace}/{name}/progress`
- The handler requires service auth and already transitions queued -> running ->
  succeeded/failed via `fastTransferProgressPatchFromPayload`.
- Storage dispatch already sends a `progress_callback.path` to k8s-control.
- k8s-control passes that path to `FastTransferMoverJobOptions.ProgressURL`,
  but the mover script never uses it.
- The mover image has `wget` and `nc`, but no `curl`.

## 3. Decision

Use existing pieces only.

- k8s-control reconstructs the expected storage progress path and joins it with
  `ServiceURLs["storage-service"]`; it does not trust arbitrary callback URLs or
  paths from the request.
- k8s-control passes callback service identity through Pod environment
  variables using the existing service identity model:
  `ServiceIdentityName`/`ServiceIdentityKey` first, legacy `ServiceAPIKey`
  fallback only when no scoped identity is configured.
- The generated shell script uses `wget` to POST JSON to the callback URL.
- Callback URL and credentials must not be embedded literally in shell args.
- If no callback URL/key is available, preserve the current copy-only behavior.

This is deliberately not production-grade secret handling. Pod env vars are
visible in the PodSpec and are acceptable only for this env-gated v1 evidence
slice. Replace them with a Secret, projected token, or workload identity before
making production/security claims.

## 4. Non-Goals

- No new public API.
- No route/spec/fixture changes unless an existing contract test proves they are
  required.
- No new dependency or image.
- No checksum, resume token, retry scheduler, reconciler, or byte counter.
- No progress streaming; just `running`, terminal `succeeded`, and best-effort
  `failed`.
- No automatic host-network discovery for kind callback tests.

## 5. Affected Files

Runtime:

- `backend/internal/platform/service_client.go`
- `backend/internal/platform/cluster/fast_transfer_mover.go`
- `backend/internal/platform/cluster/fast_transfer_mover_test.go`
- `backend/internal/services/k8scontrol/fast_transfer_mover.go`
- `backend/internal/services/k8scontrol/fast_transfer_mover_test.go`
- `backend/internal/services/storage/mount_plan_contracts.go`
- `backend/internal/services/storage/fast_transfer_state_test.go` or the nearest
  focused storage FastTransfer test file

E2E only if live callback reachability is available:

- `backend/internal/e2e/fast_transfer_progress_callback_kind_e2e_test.go`

Ledger files only after a live callback E2E passes:

- `gap.md`
- `problem.md`
- `docs/acceptance/gap-analysis.md`

Plan artifact:

- `docs/plan/2026-06-28-fast-transfer-progress-callback.md`

## 6. Runtime Design

Extend `cluster.FastTransferMoverJobOptions` with:

```go
ProgressServiceName string
ProgressServiceKey string
```

When both `ProgressURL` and `ProgressServiceKey` are set, add Pod env vars:

```text
NEXUSPAAS_FAST_TRANSFER_PROGRESS_URL
NEXUSPAAS_FAST_TRANSFER_PROGRESS_SERVICE_NAME
NEXUSPAAS_FAST_TRANSFER_PROGRESS_KEY
```

The script shape should stay simple:

```sh
set -eu
post_progress() {
  [ -n "${NEXUSPAAS_FAST_TRANSFER_PROGRESS_URL:-}" ] || return 0
  [ -n "${NEXUSPAAS_FAST_TRANSFER_PROGRESS_KEY:-}" ] || return 0
  if [ -n "${NEXUSPAAS_FAST_TRANSFER_PROGRESS_SERVICE_NAME:-}" ]; then
    wget -q -O- \
      --header "Content-Type: application/json" \
      --header "X-Service-Name: ${NEXUSPAAS_FAST_TRANSFER_PROGRESS_SERVICE_NAME}" \
      --header "X-Service-Key: ${NEXUSPAAS_FAST_TRANSFER_PROGRESS_KEY}" \
      --post-data "$1" \
      "$NEXUSPAAS_FAST_TRANSFER_PROGRESS_URL" >/dev/null || true
  else
    wget -q -O- \
      --header "Content-Type: application/json" \
      --header "X-Service-Key: ${NEXUSPAAS_FAST_TRANSFER_PROGRESS_KEY}" \
      --post-data "$1" \
      "$NEXUSPAAS_FAST_TRANSFER_PROGRESS_URL" >/dev/null || true
  fi
}
post_progress '{"status":"running","progress_pct":1}'
mkdir -p "<target>"
set +e
rsync -a --delete -- "<source>" "<target>"
code=$?
set -e
if [ "$code" -ne 0 ]; then
  post_progress '{"status":"failed","error":"rsync failed"}' || true
  exit "$code"
fi
post_progress '{"status":"succeeded","progress_pct":100}'
```

If callback URL or key env vars are absent, `post_progress` returns immediately
and current copy behavior remains unchanged. Callback POST failures are
best-effort and must not change rsync success/failure semantics.

## 7. k8s-Control Wiring

In `ensureFastTransferMoverJob`:

- Reconstruct the expected storage progress path from `project_id`,
  `target_namespace`, and `name`.
- If a request path is present, require it to exactly match the reconstructed
  expected path.
- Reject absolute URL input and wrong internal paths by omitting callback fields,
  not by creating an arbitrary callback target.
- Build the callback URL only as `ServiceURLs["storage-service"] + expectedPath`.
- Pass scoped service identity when configured:
  `ServiceIdentityName` and `ServiceIdentityKey`.
- Otherwise, pass legacy `ServiceAPIKey` only when strict runtime checks are not
  active and no scoped trusted identity mode is configured.
- If storage URL or usable service identity is absent, omit progress callback
  fields and keep dispatch behavior unchanged.

Do not trust a caller-supplied arbitrary URL in this slice.

## 8. Test Plan

Focused cluster tests:

- Callback-enabled mover includes env vars for URL, optional service name, and
  key.
- Shell args include `wget`, `X-Service-Name: ${...}` only when the service-name
  env var is present, `X-Service-Key: ${...}`, `running` before `rsync`, and
  `succeeded` after `rsync`.
- Optional service-name header is passed as one quoted wget argument, not by
  expanding an unquoted variable.
- Missing URL or missing key skips callback and preserves copy behavior.
- Shell args do not contain literal credential values.
- `wget` failure is ignored with `|| true`, while rsync failure still exits
  non-zero after a best-effort `failed` callback.
- Existing safety assertions remain: no `hostPath`, no privileged container,
  source read-only, `AutomountServiceAccountToken=false`.
- Callback-disabled mover keeps working with no env vars.

Focused k8s-control tests:

- With `ServiceURLs["storage-service"]` plus scoped
  `ServiceIdentityName`/`ServiceIdentityKey`, cluster options receive a full
  storage progress URL, service name, and callback key.
- With legacy `ServiceAPIKey` and no scoped identity mode, cluster options
  receive callback URL and key with no service name.
- Without storage base URL or usable identity, mover options omit callback
  fields.
- Absolute URL input in `progress_callback.path` is not trusted.
- Wrong internal path is not trusted.
- Reconstructed expected path is accepted.

Focused storage tests:

- The internal FastTransfer progress endpoint accepts scoped
  `X-Service-Name`/`X-Service-Key` when `ServiceTrustedIdentities` trusts the
  caller for `storage-service`.
- A scoped identity with the right key but without `storage-service` audience is
  rejected with 401.
- The same endpoint remains hidden/unauthorized when no service identity is
  configured or the scoped key is wrong.
- Legacy `ServiceAPIKey` behavior stays compatible for local/dev.

Live kind E2E, only when callback host reachability is configured:

- Gate with `TEST_LIVE_FAST_TRANSFER_PROGRESS_CALLBACK_KIND=1`.
- Require `TEST_KIND_PROGRESS_CALLBACK_HOST=<host/IP reachable from kind Pods>`.
- Start storage `httptest` on `0.0.0.0:0`; build callback base URL as
  `http://$TEST_KIND_PROGRESS_CALLBACK_HOST:<port>`.
- Start k8s-control with `ServiceURLs["storage-service"]` set to that base URL.
- Reuse the existing execution E2E shape: source/target PVCs, seed file, storage
  fast-stage, mover Job Complete, target content check.
- Assert the storage record reaches `status=succeeded`, `progress_pct=100`.
- Assert outbox contains `FastTransferProgressed` and `FastTransferCompleted`.
- Assert no production-grade secret handling/workload identity claim is made.

If the kind Pod cannot reach the host callback URL, fail the live test with Pod
events/logs and do not update ledgers.

## 9. Verification Commands

Focused:

```bash
(cd backend && go test ./internal/platform/cluster -run FastTransferMover -count=1)
(cd backend && go test ./internal/services/k8scontrol -run FastTransferMover -count=1)
(cd backend && go test -tags e2e ./internal/e2e -run FastTransferProgressCallbackKind -count=1 -v)
git diff --check
```

Live kind callback, when host callback address is known:

```bash
set -euo pipefail
export KUBECONFIG=/tmp/nexuspaas-fast-transfer-progress-e2e.kubeconfig
export TEST_KIND_PROGRESS_CALLBACK_HOST=<host/IP reachable from kind Pods>
trap 'kind delete cluster --name nexuspaas-fast-transfer-progress-e2e --kubeconfig "$KUBECONFIG" || true' EXIT
kind create cluster --name nexuspaas-fast-transfer-progress-e2e --kubeconfig "$KUBECONFIG"
docker pull instrumentisto/rsync-ssh:alpine
docker pull busybox:1.36
kind load docker-image instrumentisto/rsync-ssh:alpine --name nexuspaas-fast-transfer-progress-e2e || docker exec nexuspaas-fast-transfer-progress-e2e-control-plane crictl pull docker.io/instrumentisto/rsync-ssh:alpine
kind load docker-image busybox:1.36 --name nexuspaas-fast-transfer-progress-e2e || docker exec nexuspaas-fast-transfer-progress-e2e-control-plane crictl pull docker.io/library/busybox:1.36
(cd backend && TEST_LIVE_FAST_TRANSFER_PROGRESS_CALLBACK_KIND=1 KUBECONFIG="$KUBECONFIG" go test -tags e2e ./internal/e2e -run FastTransferProgressCallbackKind -count=1 -v)
```

Full gates:

```bash
(cd backend && go test ./...)
(cd backend && go build ./...)
(cd backend && make coverage)
(cd backend && make ci-sonar)
```

## 10. Ledger Boundary

Update ledgers only after the live callback E2E passes. Wording may say:

```text
Done for env-gated kind FastTransfer progress callback evidence only: mover Job
reports running and succeeded to storage via the existing internal progress
endpoint, and storage emits FastTransferProgressed/FastTransferCompleted. No
accurate byte accounting, checksum, resume, progress streaming, external storage
backend, multi-node, performance, durability, production-grade secret handling,
workload identity, storage GA, or Full GA claim.
```

## 11. Status

Status: Focused runtime implemented; final Reviewer verification pending after
the scoped audience fix. Live kind callback evidence remains pending until an explicit
`TEST_KIND_PROGRESS_CALLBACK_HOST` is reachable from kind Pods.
