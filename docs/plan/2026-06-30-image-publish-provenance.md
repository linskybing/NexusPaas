# Plan + Record: Image publish supply-chain provenance enforcement

Status: Approved

_Agent-workflow record for the 2026-06-30 code change. Plan + Reviewer + Code
roles all run by Claude Code (Codex Code Agent not used) — fallback recorded per
[`workflow.md`](../agents/workflow.md)._

## Why

The user authorized code changes to resolve a tracked problem. Audit finding: the
codebase is clean and every open GA P0 is **infra-gated** (external Harbor,
8-unit cluster, live DB/Secret drills, PERF/MON load) — uncloseable by code. The
one bounded, code-addressable advance was the **image supply-chain enforcement**
dimension of the High IMG gap: extend the IMG-019 publish guard to also require
SBOM-digest + signature-ref presence.

## What

Config-gated (`IMAGE_PUBLISH_REQUIRE_PROVENANCE`, **default off**) extension to the
catalog publish guard. When on, `POST /api/v1/image-catalog/publish` rejects a
catalog image lacking an SBOM digest or signature ref (on top of the existing
digest + passing-scan checks); the published allow rule carries the promoted
`sbom_digest`/`signature` provenance refs.

Default-off is a deliberate correctness requirement: no live pipeline produces
SBOM/signature yet, so unconditional enforcement would break every existing
publish and test. Flip on once a provenance source exists.

## Files

- `backend/internal/platform/config.go` — `ImageProvenanceRequired bool` +
  env `IMAGE_PUBLISH_REQUIRE_PROVENANCE` + `ConfigFromEnv` wiring (default false).
  (Go identifier kept short so gofmt doesn't re-tab neighboring aligned lines.)
- `backend/internal/services/imageregistry/helpers.go` —
  `catalogAllowListRejection(tag, requireProvenance)` SBOM/signature checks;
  `promoteCatalogImageStatusFields` promotes `sbom_digest`/`signature`.
- `backend/internal/services/imageregistry/handler.go` — pass the flag at the
  publish call site.
- `backend/internal/services/imageregistry/handler_test.go` —
  `TestImageCatalogPublishProvenanceEnforcement` (reject no-SBOM, reject no-sig,
  accept both + promote, flag-off publishes).
- Docs: `problem.md` §2 IMG row, `gap.md` IMG note, trace-matrix IMG Evidence
  Scope — all presence-only; IMG stays `Open`.

## Out of scope (deliberate)

- Harbor-sync extraction of SBOM/signature from Harbor accessories (speculative
  shape; part of the Open live pipeline).
- Real Syft SBOM generation / Cosign signing execution — infra-gated, stays Open.
- No new dependency.

## Verification (all green this pass)

- `go build ./...` / `go vet ./...` — clean.
- `go test ./internal/services/imageregistry/... ./internal/platform/...` — green;
  new test's 4 subtests pass.
- `go test ./...` and `go test -race ./...` — green (default-off proves backward
  compatibility / no regression).
- `python3 docs/tests/verify_ga_acceptance_trace_matrix.py` — green (38 rows); IMG
  row still `Open`.
- `gofmt -l` — clean.

## Reviewer verdict

Bounded, single-service, no ADR reversal, no new dependency. Enforcement is opt-in
and default-off, so behavior is unchanged unless explicitly enabled. No open
external P0 was relabeled closed or as live GA proof — IMG stays `Open` and the
docs explicitly scope this as a presence guard pending live SBOM/sign execution.
**Status: Approved.**
