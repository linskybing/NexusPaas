# Plan + Record: Re-verify & refresh `ac.md` / `gap.md` / `problem.md`

Status: Approved

_Agent-workflow record for the 2026-06-30 independent re-verification pass of the
GA-clearance trackers. Plan Agent + Reviewer Agent + Code Agent roles all run by
Claude Code this pass (Codex quota not used) — fallback recorded per
[`workflow.md`](../agents/workflow.md)._

## Why

`ac.md`, `gap.md`, and `problem.md` are living GA-clearance trackers last updated
in merged commit `#40`. The user requested another independent re-verification +
refresh pass against the current code, with the reference repo provided locally.
Constraints: **local-only** (open external P0s stay Open), and the
[`workflow.md`](../agents/workflow.md) evidence rule — local/static/fixture
evidence must never be described as live external GA proof.

## What was verified (this pass)

| Check | Result |
|---|---|
| `go build ./...` | clean (exit 0) |
| `go vet ./...` | clean (exit 0) |
| `go test ./...` | green — 23 tested packages; `internal/e2e` no test files (24 total) |
| `go test -race ./...` | **green (exit 0)** — new datapoint; trackers previously listed "Not Run" |
| `python3 docs/tests/verify_ga_acceptance_trace_matrix.py` | green (38 rows) — **was failing before the fix below** |
| Domain-level reference parity vs `references/CSCC_AI_Platform_Backend` | holds — every `internal/application` domain + `internal/cron` reconciler maps to a ported service; `course_monitoring_reconciler` out of scope (ADR 0006) |
| Sonar | prior local `nexuspaas-backend` report only — **not re-run** this pass |

## Drift found & corrected

1. **Trace-matrix self-validation failure (the one real defect):** the `RTC`
   row carried classification `Deferred-Frontend`, a value defined nowhere in the
   doc's own Status Taxonomy and rejected by the verify script (allowed:
   `Done`/`Open`/`Deferred-GPU-Hardware`). Introduced in `#36`, committed broken.
   Fix: reclassified `RTC` → **`Open`** (consistent with the frontend-dependent
   `WEB` family; frontend-removed deferral stays in the Reason column; GPU
   browser-media proof already tracked in the `Deferred-GPU-Hardware` row). No new
   one-row taxonomy value invented.
2. **`problem.md` stale branch ref:** `feature/ga-ac-clearance` (merged) →
   `main`; header re-stamped with this pass's results incl. `-race`.
3. **`gap.md` stale present-tense frontend claim:** frontend was removed in `#36`
   (`git ls-files frontend` = 0; `App.tsx` absent — on-disk `frontend/` is
   untracked leftover). Clarified the Web UI bullet that all `WEB-*`/`frontend/`
   evidence below is **historical (pre-removal)**, not current in-repo state.
4. Package-count precision in `problem.md` (`24` → `23 tested + e2e no-tests`).

## Not changed (deliberate)

- `ac.md` — intentional pointer to `docs/acceptance/`; both link targets exist.
- Open external P0s (external Harbor promote/rollback, 8-unit staging
  deploy/smoke/rollback, live staging DB migration/rollback, live external Secret
  provenance, full image-build SBOM/sign/scan GA, live PERF/MON) — **stay Open**;
  no local evidence promoted to live GA proof.
- The large `WEB-*` / `frontend/` historical evidence blocks — flagged as
  historical upfront rather than rewritten.

## Files touched

- `docs/acceptance/ga-acceptance-trace-matrix.md` — RTC classification fix.
- `problem.md` — branch ref, header re-stamp, §1 + §7 toolchain/`-race`/parity.
- `gap.md` — date re-stamp, frontend-removed clarification.

## Reviewer verdict

Diff is surgical (3 tracker files; date/branch/result/drift corrections only). No
open external P0 was relabeled closed or as live GA proof. Toolchain, `-race`, and
the trace-matrix verify script are all green. **Status: Approved.**
