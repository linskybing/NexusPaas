# FastTransfer Progress Callback

Date: 2026-06-28
Scope: mover Job -> storage progress callback runtime slice

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

## 2. Background

The storage internal progress endpoint and dispatch path already exist, but the
mover script never uses the `ProgressURL` it is handed, so no status callback is
emitted. This slice wires the callback using only existing pieces.

## 3. Source References

- `backend/internal/platform/service_client.go`
- `backend/internal/platform/cluster/fast_transfer_mover.go`
- `backend/internal/services/k8scontrol/fast_transfer_mover.go`
- `backend/internal/services/storage/mount_plan_contracts.go`
- `backend/internal/services/storage/fast_transfer_state_test.go`

## 4. Assumptions

- The storage internal progress endpoint already transitions queued -> running
  -> succeeded/failed via `fastTransferProgressPatchFromPayload`.
- The mover image has `wget` and `nc` but no `curl`.
- Pod env vars are an acceptable test-only credential channel for this env-gated
  v1 evidence slice.

## 5. Non-Goals

- No new public API.
- No route/spec/fixture changes unless an existing contract test proves they are
  required.
- No new dependency or image.
- No checksum, resume token, retry scheduler, reconciler, or byte counter.
- No progress streaming; just `running`, terminal `succeeded`, and best-effort
  `failed`.
- No automatic host-network discovery for kind callback tests.

## 6. Current Behavior

- Storage already exposes
  `POST /internal/storage/projects/{project_id}/transfers/{targetNamespace}/{name}/progress`,
  requiring service auth and transitioning queued -> running ->
  succeeded/failed.
- Storage dispatch already sends a `progress_callback.path` to k8s-control.
- k8s-control passes that path to `FastTransferMoverJobOptions.ProgressURL`, but
  the mover script never uses it.

## 7. Target Behavior

When both a reconstructed storage progress URL and a usable service identity are
available, the mover Job posts `running` before rsync and `succeeded` after, and
best-effort `failed` on rsync error. The storage record reaches
`status=succeeded`, `progress_pct=100`, and emits `FastTransferProgressed` /
`FastTransferCompleted`. With no callback URL/key, current copy-only behavior is
preserved.

Design decision — use existing pieces only:

- k8s-control reconstructs the expected storage progress path and joins it with
  `ServiceURLs["storage-service"]`; it does not trust arbitrary callback URLs or
  paths from the request.
- k8s-control passes callback service identity through Pod env vars using the
  existing model: `ServiceIdentityName`/`ServiceIdentityKey` first, legacy
  `ServiceAPIKey` fallback only when no scoped identity is configured.
- The generated shell script uses `wget` to POST JSON to the callback URL.
- Callback URL and credentials must not be embedded literally in shell args.

## 8. Affected Domains

- platform/cluster mover script, k8s-control mover wiring, storage progress
  endpoint auth, and (optional) e2e + acceptance ledgers.

## 9. Affected Files

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

Ledger files only after a live callback E2E passes: `gap.md`, `problem.md`,
`docs/acceptance/gap-analysis.md`.

## 10. API / Contract Changes

None. The existing internal progress endpoint and its payload are reused; no
route/spec/fixture changes unless an existing contract test proves they are
required.

## 11. Database / Migration Changes

None.

## 12. Configuration Changes

No checked-in runtime config changes. Test-only env gate
`TEST_LIVE_FAST_TRANSFER_PROGRESS_CALLBACK_KIND=1` plus
`TEST_KIND_PROGRESS_CALLBACK_HOST=<host/IP reachable from kind Pods>` for the
live host-callback variant.

## 13. Observability Changes

None.

## 14. Security Considerations

This is deliberately not production-grade secret handling. Pod env vars are
visible in the PodSpec and are acceptable only for this env-gated v1 evidence
slice; replace them with a Secret, projected token, or workload identity before
making production/security claims. k8s-control must not trust a caller-supplied
arbitrary URL; callback POST failures are best-effort and must not change rsync
success/failure semantics. Existing safety assertions remain: no `hostPath`, no
privileged container, source read-only, `AutomountServiceAccountToken=false`.

## 15. Implementation Steps

Runtime design — extend `cluster.FastTransferMoverJobOptions` with:

```go
ProgressServiceName string
ProgressServiceKey  string
```

When both `ProgressURL` and `ProgressServiceKey` are set, add Pod env vars
`NEXUSPAAS_FAST_TRANSFER_PROGRESS_URL`,
`NEXUSPAAS_FAST_TRANSFER_PROGRESS_SERVICE_NAME`, and
`NEXUSPAAS_FAST_TRANSFER_PROGRESS_KEY`. The script shape stays simple:

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
and current copy behavior is unchanged.

k8s-control wiring in `ensureFastTransferMoverJob`:

- Reconstruct the expected storage progress path from `project_id`,
  `target_namespace`, and `name`; if a request path is present, require an exact
  match to the reconstructed path.
- Reject absolute URL input and wrong internal paths by omitting callback fields,
  not by creating an arbitrary callback target.
- Build the callback URL only as `ServiceURLs["storage-service"] + expectedPath`.
- Pass scoped `ServiceIdentityName`/`ServiceIdentityKey` when configured;
  otherwise pass legacy `ServiceAPIKey` only when strict runtime checks are not
  active and no scoped trusted identity mode is configured.
- If storage URL or usable identity is absent, omit callback fields and keep
  dispatch behavior unchanged.

Ledger boundary (after a live callback E2E passes):

```text
Done for env-gated kind FastTransfer progress callback evidence only: mover Job
reports running and succeeded to storage via the existing internal progress
endpoint, and storage emits FastTransferProgressed/FastTransferCompleted. No
accurate byte accounting, checksum, resume, progress streaming, external storage
backend, multi-node, performance, durability, production-grade secret handling,
workload identity, storage GA, or Full GA claim.
```

## 16. Verification Plan

Focused cluster tests: callback-enabled mover includes env vars; shell args
include `wget`, the optional `X-Service-Name` header only when present,
`X-Service-Key`, `running` before `rsync`, and `succeeded` after; the optional
service-name header is one quoted wget argument; missing URL/key skips callback
and preserves copy behavior; shell args carry no literal credential values;
`wget` failure is ignored with `|| true` while rsync failure still exits
non-zero after a best-effort `failed`; existing safety assertions remain;
callback-disabled mover keeps working with no env vars.

Focused k8s-control tests: scoped identity yields full URL + service name + key;
legacy `ServiceAPIKey` yields URL + key with no service name; missing storage
URL/identity omits callback fields; absolute URL and wrong internal path are not
trusted; reconstructed expected path is accepted.

Focused storage tests: the internal progress endpoint accepts scoped
`X-Service-Name`/`X-Service-Key` when trusted for `storage-service`; rejects a
scoped key without the `storage-service` audience with `401`; stays
hidden/unauthorized with no identity or a wrong key; legacy `ServiceAPIKey`
stays compatible for local/dev.

Commands:

```bash
(cd backend && go test ./internal/platform/cluster -run FastTransferMover -count=1)
(cd backend && go test ./internal/services/k8scontrol -run FastTransferMover -count=1)
(cd backend && go test -tags e2e ./internal/e2e -run FastTransferProgressCallbackKind -count=1 -v)
git diff --check
```

Live kind callback (only when `TEST_KIND_PROGRESS_CALLBACK_HOST` is reachable
from kind Pods): start storage `httptest` on `0.0.0.0:0`, set
`ServiceURLs["storage-service"]` to `http://$TEST_KIND_PROGRESS_CALLBACK_HOST:<port>`,
reuse the execution E2E shape, and assert the storage record reaches
`status=succeeded`/`progress_pct=100` with `FastTransferProgressed` and
`FastTransferCompleted` in the outbox. If the kind Pod cannot reach the host
callback URL, fail with Pod events/logs and do not update ledgers.

Full gates:

```bash
(cd backend && go test ./...)
(cd backend && go build ./...)
(cd backend && make coverage)
(cd backend && make ci-sonar)
```

## 17. Rollback Plan

Revert the runtime and test edits in the section 9 files and remove the optional
e2e file. If ledgers were updated after a passing run, revert only the scoped
ledger wording. No API, database, migration, or config rollback is expected.

## 18. Risks and Tradeoffs

- Pod env-var credentials are not production-grade; the slice is env-gated v1
  evidence only.
- The live host-callback kind variant depends on a kind-Pod-reachable host
  address and stays a noted future check rather than a blocker.
- Best-effort callbacks must never alter rsync success/failure semantics.

## 19. Reviewer Checklist

- Runtime scope stays within the section 9 files; no new public API/route/spec
  unless an existing contract test forces it.
- Callback URL/path is reconstructed and not caller-trusted; credentials are not
  embedded literally in shell args.
- Existing mover security assertions are preserved.
- Ledger wording, if added, matches the bounded section 15 boundary.

## 20. Status

Focused runtime implemented and committed; the scoped-audience fix landed. The
in-cluster sink callback emission variant passed in kind (see
`2026-06-28-fast-transfer-progress-callback-kind-sink-e2e.md`). The optional live
host-callback kind variant remains a noted future check, pending an explicit
`TEST_KIND_PROGRESS_CALLBACK_HOST` reachable from kind Pods; it is not a blocker
for this slice.

Status: Approved
