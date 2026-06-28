# FastTransfer Progress Callback Kind Sink E2E

## Objective

Add lightweight kind evidence that the FastTransfer mover Job actually emits
progress callback HTTP POSTs from inside Kubernetes:

```text
mover Job -> in-cluster callback sink Service
```

This is a narrow runtime proof for callback emission only. It does not prove
storage record transitions, storage-service reachability from kind Pods,
accurate byte progress, checksum, resume, performance, CSI, storage GA, or Full
GA.

## Design

- Reuse the existing FastTransfer mover execution kind helpers for namespace,
  PVCs, source seeding, mover Job completion, target content verification, and
  diagnostics.
- Add a tiny BusyBox callback sink Pod and ClusterIP Service in the same test
  namespace. The sink must be Ready before mover dispatch and must return
  `HTTP/1.1 200 OK` for each POST while appending the request body to stdout,
  so the mover's blocking `wget` callback cannot hang on an incomplete response.
- Configure the k8s-control test app with
  `ServiceURLs["storage-service"] = http://<sink-service>.<namespace>.svc.cluster.local:8080`
  plus scoped callback identity env fields. This is test-only callback wiring
  evidence, not production-grade secret handling or workload identity.
- Submit the existing internal mover Job request with the expected storage
  progress path.
- Wait for the mover Job to complete, then poll the sink Pod logs for both:
  `{"status":"running","progress_pct":1}` and
  `{"status":"succeeded","progress_pct":100}`.

The sink intentionally records raw HTTP requests only. It does not emulate
storage-service business logic.
Ledger wording must say this proves only in-cluster callback emission;
storage-service progress-state transition evidence remains pending.

## Files

- `backend/internal/e2e/fast_transfer_progress_callback_kind_e2e_test.go`
- `docs/plan/2026-06-28-fast-transfer-progress-callback-kind-sink-e2e.md`
- Ledger files only after the live kind test passes:
  - `gap.md`
  - `problem.md`
  - `docs/acceptance/gap-analysis.md`

Exact ledger boundary:

```text
Done for env-gated kind FastTransfer progress callback emission evidence only:
the mover Job running inside Kubernetes emitted running and succeeded HTTP POSTs
to an in-cluster callback sink Service. This does not prove storage-service
progress-state transitions, storage record updates, storage events, accurate byte
accounting, checksum, resume, progress streaming, external storage backend,
multi-node behavior, performance, durability, production-grade secret handling,
workload identity, storage GA, or Full GA.
```

## Verification

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

## Reviewer Status

Approved for implementation. Scope is intentionally limited to live callback
emission evidence through an in-cluster sink and must not be worded as
storage-service progress-state evidence.

## Status

Implemented as an env-gated e2e harness. Focused compile/skip passes. Live kind
passed on `nexuspaas-fast-transfer-progress-sink-e2e3` using
`kindest/node:v1.31.0` and in-cluster image pulls after local `kind load
docker-image` failed for the rsync image. The dedicated kind cluster was deleted
after the run. Ledger evidence is scoped to in-cluster callback emission only.
