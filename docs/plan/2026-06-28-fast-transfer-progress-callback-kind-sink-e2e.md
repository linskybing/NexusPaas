# FastTransfer Progress Callback Kind Sink E2E

Date: 2026-06-28
Scope: env-gated kind e2e harness for in-cluster callback emission only

## 1. Objective

Add lightweight kind evidence that the FastTransfer mover Job actually emits
progress callback HTTP POSTs from inside Kubernetes:

```text
mover Job -> in-cluster callback sink Service
```

This is a narrow runtime proof for callback emission only. It does not prove
storage record transitions, storage-service reachability from kind Pods,
accurate byte progress, checksum, resume, performance, CSI, storage GA, or Full
GA.

## 2. Background

The mover Job is expected to POST progress callbacks, but there was no kind
evidence that it actually does so from inside Kubernetes. This slice adds an
in-cluster sink and asserts the mover's running/succeeded POST bodies arrive.
Storage-service progress-state transition evidence is tracked separately and
remains out of scope here.

## 3. Source References

- `backend/internal/e2e/fast_transfer_mover_execution_kind_e2e_test.go`
  (reused helpers for namespace, PVCs, source seeding, mover Job completion,
  target content verification, diagnostics)
- `backend/internal/e2e/fast_transfer_progress_callback_kind_e2e_test.go`
  (new test)

## 4. Assumptions

- The reused execution-kind helpers are stable.
- A BusyBox callback sink Pod/Service can become Ready before mover dispatch.
- The mover's blocking `wget` callback requires a complete `200` response from
  the sink to avoid hanging.

## 5. Non-Goals

- No storage-service progress-state transition evidence.
- No production-grade secret handling or workload identity claim.
- No accurate byte progress, checksum, resume, performance, CSI, storage GA, or
  Full GA claim.

## 6. Current Behavior

The mover execution-kind test proves bytes move, but nothing asserts the mover
emits progress callbacks from inside the cluster.

## 7. Target Behavior

An env-gated kind test creates an in-cluster BusyBox callback sink, dispatches a
mover Job wired to that sink, waits for completion, and confirms the sink logged
both `{"status":"running","progress_pct":1}` and
`{"status":"succeeded","progress_pct":100}` plus the service identity header.

## 8. Affected Domains

- E2E test harness and acceptance ledgers only.

## 9. Affected Files

- `backend/internal/e2e/fast_transfer_progress_callback_kind_e2e_test.go`
- `docs/plan/2026-06-28-fast-transfer-progress-callback-kind-sink-e2e.md`
- Ledger files only after the live kind test passes: `gap.md`, `problem.md`,
  `docs/acceptance/gap-analysis.md`

## 10. API / Contract Changes

None.

## 11. Database / Migration Changes

None.

## 12. Configuration Changes

Test-only callback wiring:

```text
ServiceURLs["storage-service"] = http://<sink-service>.<namespace>.svc.cluster.local:8080
TEST_LIVE_FAST_TRANSFER_PROGRESS_CALLBACK_KIND=1   # live-kind gate
```

No checked-in runtime config changes.

## 13. Observability Changes

None. The sink records raw HTTP request bodies for test assertions only.

## 14. Security Considerations

This is test-only callback wiring evidence, not production-grade secret handling
or workload identity. The sink intentionally records raw HTTP requests only and
does not emulate storage-service business logic.

## 15. Implementation Steps

1. Reuse the existing FastTransfer mover execution kind helpers for namespace,
   PVCs, source seeding, mover Job completion, target content verification, and
   diagnostics.
2. Add a tiny BusyBox callback sink Pod and ClusterIP Service in the same test
   namespace; the sink must be Ready before mover dispatch and return
   `HTTP/1.1 200 OK` for each POST while appending the request body to stdout.
3. Configure the k8s-control test app with the sink `ServiceURLs` entry plus
   scoped callback identity env fields.
4. Submit the existing internal mover Job request with the expected storage
   progress path.
5. Wait for the mover Job to complete, then poll the sink Pod logs for both
   `{"status":"running","progress_pct":1}` and
   `{"status":"succeeded","progress_pct":100}`.
6. Update ledgers only after the live kind test passes, with exact boundary:

```text
Done for env-gated kind FastTransfer progress callback emission evidence only:
the mover Job running inside Kubernetes emitted running and succeeded HTTP POSTs
to an in-cluster callback sink Service. This does not prove storage-service
progress-state transitions, storage record updates, storage events, accurate byte
accounting, checksum, resume, progress streaming, external storage backend,
multi-node behavior, performance, durability, production-grade secret handling,
workload identity, storage GA, or Full GA.
```

## 16. Verification Plan

Focused:

```bash
(cd backend && go test -tags e2e ./internal/e2e -run FastTransferProgressCallbackKind -count=1 -v)
```

Live kind:

```bash
set -euo pipefail
export KUBECONFIG=/tmp/nexuspaas-fast-transfer-progress-sink-e2e.kubeconfig
trap 'kind delete cluster --name nexuspaas-fast-transfer-progress-sink-e2e --kubeconfig "$KUBECONFIG" || true' EXIT
kind create cluster --name nexuspaas-fast-transfer-progress-sink-e2e --kubeconfig "$KUBECONFIG"
docker pull instrumentisto/rsync-ssh:alpine
docker pull busybox:1.36
kind load docker-image instrumentisto/rsync-ssh:alpine --name nexuspaas-fast-transfer-progress-sink-e2e
kind load docker-image busybox:1.36 --name nexuspaas-fast-transfer-progress-sink-e2e
(cd backend && TEST_LIVE_FAST_TRANSFER_PROGRESS_CALLBACK_KIND=1 KUBECONFIG="$KUBECONFIG" go test -tags e2e ./internal/e2e -run FastTransferProgressCallbackKind -count=1 -v)
```

Full gates:

```bash
(cd backend && go test ./...)
(cd backend && go build ./...)
(cd backend && make coverage)
(cd backend && make ci-sonar)
```

## 17. Rollback Plan

Delete `backend/internal/e2e/fast_transfer_progress_callback_kind_e2e_test.go`
and revert any scoped ledger wording. No runtime rollback is needed.

## 18. Risks and Tradeoffs

- The mover's blocking `wget` callback can hang on an incomplete sink response;
  the sink must always return a complete `200`.
- Local `kind load docker-image` can fail for the rsync image; in-cluster image
  pulls are the fallback.
- Evidence must stay scoped to in-cluster callback emission, not storage-service
  progress-state.

## 19. Reviewer Checklist

- Scope is intentionally limited to live callback emission evidence through an
  in-cluster sink and must not be worded as storage-service progress-state
  evidence.
- Ledger wording must match the bounded boundary in section 15.

## 20. Status

Implemented as an env-gated e2e harness. Focused compile/skip passes. Live kind
passed on `nexuspaas-fast-transfer-progress-sink-e2e3` using
`kindest/node:v1.31.0` and in-cluster image pulls after local `kind load
docker-image` failed for the rsync image; the dedicated kind cluster was deleted
after the run. Ledger evidence is scoped to in-cluster callback emission only.

Status: Approved
