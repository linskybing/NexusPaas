# AC Completion Round — Image Build, Ops Resilience, Live PERF/MON, DATA

Status: Approved

_Agent-workflow record: Plan + Code + Reviewer roles run by Claude Code this
pass (Codex quota not used — fallback recorded per
[`workflow.md`](../agents/workflow.md))._

## Why

The v0.1.0 release was withdrawn by the owner ("still missing a lot"). The
remaining machine-feasible acceptance-criteria work is exactly
`blocker-ledger.md` §8 items 6–9. The owner selected all four blocks in one
round, delivered on a single branch/PR. All new evidence is kind/local-tier and
labeled truthfully per `workflow.md` / ADR 0008; external-infra (C), GPU (D),
and frontend (E) classes remain out of scope. No release/tag is created.

## What

| Phase | Scope |
|---|---|
| 1 | Image-build source: `ObjectStore.PutStream`, multipart `POST /api/v1/images/build/context` (streamed, validated), `context_key` build reference (base64 path retained, additive), from-storage permission checks |
| 2 | Image-build dispatch: executor abstraction (local docker executor for live evidence; in-cluster BuildKit Job manifest for production wiring), lease-gated dispatch loop consuming `queued` builds, syft SBOM → trivy scan gate (fail-closed) → cosign sign, `IMAGE_PUBLISH_REQUIRE_PROVENANCE` upgraded from presence-only to verified, live Harbor E2E incl. scan-fail rejection and allow-list admission |
| 3 | Ops resilience: `db-backup-restore-drill.sh` (pg_dump→destroy→restore→validate), SERVICE_IDENTITY dual-key rotation (`previous_key` overlap), `failure-injection-drill.sh` (DB / K8s-API / Prometheus interruption / node agent) |
| 4 | DATA: live typed-API authorization evidence over api/v1 fixture families, org-project-service typed-table migration (identity pattern), drift→replay reconcile job (projectionDrift → event + ResetConsumer rebuild) with kind evidence |
| 5 | Live PERF/MON: Prometheus deployment manifest + kind deploy with existing rules/dashboard, alert firing proof, k6 PERF-003/004/006/008 live runs |
| 6 | Ledgers/evidence updates (kind/local-tier labeling), verifier + race + beta-rc green, reviewer pass, PR, squash merge |

## Verification

- Per phase: `go build/vet/test` plus env-gated live e2e where applicable.
- Round close: `ci-security-gate.sh beta-rc` exit 0;
  `verify_ga_acceptance_trace_matrix.py` green; `go test -race ./...` green.
- Post-merge: SonarCloud Quality Gate stays OK with 0 open issues.

## Execution record (2026-07-02)

All six phases executed on branch `ac-completion-round`; every live claim is
kind/local-tier and labeled as such in the evidence reports:

| Phase | Outcome | Evidence |
|---|---|---|
| 1 | Done — PutStream, multipart context upload, context_key fail-closed staging, from-storage permission gate | unit suites; commit `41e4e17` |
| 2 | Done — dispatcher + docker executor + verified provenance gate; live pipeline E2E PASS against real Harbor (build→push→SBOM→scan→sign→pull→publish 409/200) | `evidence/2026-07-02-live-image-build-pipeline-report.md` |
| 3 | Done — destructive restore drill PASS (78 tables), dual-key rotation live drill PASS (0 auth failures), OPS-019 matrix PASS (db/k8s-api/node-agent/prometheus) | `evidence/2026-07-02-db-backup-restore-drill-report.md`, `evidence/2026-07-02-ops-resilience-drills-report.md` |
| 4 | Done — drift→replay reconcile job (7 families) with live kind injected-drift auto-repair, org-project typed migration 0002, live 66-family authz sweep | `evidence/2026-07-02-data-layer-report.md` |
| 5 | Done — Prometheus + kube-state-metrics on kind, alert fire+resolve drill, k6 PERF-003/004/006/008 all green | `evidence/2026-07-02-live-perf-mon-report.md` |
| 6 | Verifier 38 rows green; `go test -race ./...` green (23 pkgs); beta-rc gate + reviewer pass + PR recorded below | this doc, PR description |

Deviations from plan: the in-cluster BuildKit Job executor (Phase 2 second
executor) was NOT implemented — the docker executor carries the live evidence
as the plan's pre-approved fallback, and the BuildKit executor is recorded in
the ledgers as the tracked production follow-up. storage-service has drift
checks but no local projection consumer, so it is documented rather than wired
into the reconcile job. Codex was unavailable this round; Claude Code executed
all three agent roles (fallback recorded per AGENTS.md).

## Reviewer pass (2026-07-03)

`/code-review medium` over `main...ac-completion-round` (8 finder angles +
verification; Claude Code as Reviewer Agent, Codex unavailable — fallback per
AGENTS.md). 7 findings confirmed and fixed in commits `4b97bbb` + follow-up:

1. **prometheus.yaml scrape job lacked a `namespace` relabel** — `up{namespace="nexuspaas"}`
   in `NexusPaasServiceUnscraped` matched zero series on the operator-less
   path; the scrape-blindness watchdog could never fire. Fixed (relabel added;
   rule files unchanged, parity test still pins both paths).
2. **Inline context archive silently skipped when no object store** — build
   queued then failed late at the executor with an empty context; now fails
   closed at create with 503.
3. **Projection reconciler looped non-convergent rebuilds** — residual drift
   triggered a full consumer reset + stream replay every tick, contradicting
   its own doc comment; now rebuilds once per residual value and only reports
   after (regression test added).
4. **Staged build context downloaded twice per dispatch** — validate+fetch
   collapsed into one object-store read.
5. **`sbom_status` left stale (`pending`) on partial executor failure** — now
   recorded as succeeded with its digest when the SBOM was produced.
6. **`dispatcherEvent` duplicated `registryEvent`** — replaced with
   `registryEvent(shared.MaintenanceRequest(ctx), …)`.
7. **`imageBuildTimeout` re-implemented `shared.IntValue`** — replaced.

Also cleared the local SonarQube quality gate for the branch: 5× go:S3776
cognitive-complexity refactors (createBuild, docker executor Execute, three
test files split into helpers), kubernetes:S6897/S6870 ephemeral-storage
requests/limits added; 2× kubernetes:S6865 on the Prometheus/KSM pod specs
resolved as **Accepted** with justification (both ServiceAccounts require the
API token for discovery and are least-privilege RBAC-bound in the same
manifest; the analyzer cannot link the binding — same rationale class as the
accepted S6431, see the 2026-07-02 cleanup plan doc).

Known-accepted debt (reported, intentionally not changed this round):
`oldestQueuedBuild`/`buildProvenanceRejection` list whole resources (needs a
filtered-list store API); the drift-count sum is repeated across the seven
reconciler wirings (distinct per-service report types); `storage_path` became
required on the from-storage v1 endpoint (approved plan behavior — the field
was previously accepted-but-meaningless; fixtures/parity updated in the same
change).
