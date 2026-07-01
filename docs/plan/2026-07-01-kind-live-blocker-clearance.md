# Kind-Tier Live Clearance of V1 Launch Blockers

Status: Approved

_Agent-workflow record: Plan + Code + Reviewer roles run by Claude Code this
pass (Codex quota not used — fallback recorded per
[`workflow.md`](../agents/workflow.md)). This is the single active plan; all
prior completed/merged plans in this folder were deleted per the "delete when
done" convention._

## Why

`docs/acceptance/ga-acceptance-trace-matrix.md`, `gap-tracker.md`, and `blocker-ledger.md` kept
the V1 external production launch blockers `Open` with **render-only / static /
fixture** evidence — the deploy/migration/rollback/supply-chain machinery had
never executed on a real Kubernetes cluster. The user asked to actually run and
prove the blockers via **kind**, and to keep `docs/plan/` trimmed to only active
plans.

Binding constraint ([`workflow.md`](../agents/workflow.md)): *local, static,
fixture, or single-cluster evidence must not be described as external GA proof.*
kind is one local cluster, so a kind run genuinely proves the machinery executes
on real Kubernetes but the **external** adjective stays `Open`. (This supersedes
the deleted `2026-06-30-v1-external-launch-live-rehearsal.md`, whose external
harness `backend/scripts/production-beta-live-rehearsal.sh` intentionally rejects
kind and still requires a real external cluster + registry.)

## What was done

- Added `backend/scripts/kind-live-e2e.sh` (self-contained kind entrypoint;
  reuses the production-beta manifests via `kubectl kustomize backend`; leaves
  the external harness's kind-rejecting guards untouched; stamps every artifact
  `NOT EXTERNAL GA PROOF`).
- Ran it green on kind v0.32.0 / kubectl v1.36.2. Drills proven (kind-tier):
  1. **Image supply chain** — BuildKit build, syft SPDX SBOM, trivy scan
     (`HIGH/CRITICAL=0`), cosign keypair, local-registry push.
  2. **Backing services + secrets** — postgres/redis/minio live; all 12 required
     backing + per-unit runtime Secret objects created and verified present
     (names/keys only).
  3. **8-unit deploy/smoke** — all 8 production-beta units `/healthz`+`/readyz`+
     `/metrics`+`/openapi.json` = 200; 15-of-15 service-registry union.
  4. **Live DB migration** — `apply-migrations` → `validate-migrations` →
     idempotent re-apply as in-cluster Jobs against a real Postgres pod.
  5. **Per-unit rollback/redeploy** — previous-image rollback + redeploy + smoke
     for all 8 units.
  6. **Local-registry promote/rollback** — push previous, promote candidate,
     rollback-pull previous.
- Evidence: `docs/acceptance/evidence/2026-07-01-kind-live-e2e-report.md`.
- Trackers updated honestly (classifications unchanged, rows stay `Open`):
  `ga-acceptance-trace-matrix.md`, `gap-tracker.md`, `blocker-ledger.md`, `image-build.md`,
  `security.md` — each new datapoint labelled kind-tier / not-external.

## Honest residual (stays Open)

- External registry host promotion/rollback (real external Harbor GA registry).
- Live **external** staging cluster deploy, Secret provenance/rotation,
  off-cluster HA/DR.
- DB schema down-migration/restore-from-backup on external staging (the app has
  forward-only migrations; down-migration is not an app capability).
- Product image-build **dispatch** feature (Tekton/BuildKit execution, tar.gz/
  zip upload, source hashing/persistence) — not implemented; verification n/a.
- Enforced SBOM/scan/sign gates in the live build/publish path.
- Full live PERF/MON soak, browser WebRTC, real GPU (Deferred-GPU-Hardware).

## Kind-tier deviations (disclosed)

- Cluster-hosting units were granted an automounted ServiceAccount token bound
  to `cluster-admin` so the in-cluster readiness ping succeeds on a throwaway
  local cluster. Not a production posture; the SEC-016 user-workload
  `automountServiceAccountToken=false` guard is unchanged.
- `candidate` image is a retag of the baseline (same bits) so the rollback/
  redeploy drill exercises a real image-ref change + rollout, not a behavior diff.

## Verify / reproduce

```sh
KEEP_CLUSTER=0 PATH="$HOME/.local/bin:$PATH" bash backend/scripts/kind-live-e2e.sh
python3 docs/tests/verify_ga_acceptance_trace_matrix.py   # green, 38 rows
cd backend && go build ./... && go vet ./...
```

## Next (external, out of this pass)

Run `backend/scripts/production-beta-live-rehearsal.sh` against a real external
registry + external staging cluster to close the external residual above. Only
then may the V1 external production launch rows move off `Open`.
