# AC Gap Problem Sync

## 1. Objective

Create a docs-only implementation plan to sync `problem.md`, `gap.md`, and the
limited acceptance docs with
`docs/acceptance/archive-image-build-hpc-storage-audit.md` as the source of
truth.

The implementation must clarify that archive/image-build support is currently
metadata-only/API-contract evidence, that image-build idempotency lacks source
fingerprinting, that HPC storage is a planning-layer maturity area with limited
basic data-plane evidence, and that Sonar is locally clean while remote
SonarCloud status still has unresolved scope/configuration conditions.

## 2. Background

The audit dated 2026-06-30 found that current tracker language can still read as
more complete than the implementation. The requested follow-up is only a tracker
wording sync, not feature work.

Agent fallback record: Claude is unavailable. Codex subagent is acting as Plan
Agent for this file. Main Codex will act as Code Agent only after plan approval.
Another Codex subagent will act as Reviewer Agent for plan and implementation
review.

## 3. Source References

- `docs/acceptance/archive-image-build-hpc-storage-audit.md`
- `problem.md`
- `gap.md`
- `docs/acceptance/ga-acceptance-trace-matrix.md`
- `docs/acceptance/image-build.md`
- `docs/agents/workflow.md`
- `docs/agents/planning.md`

## 4. Assumptions

- The audit file is accepted as the source of truth for this sync.
- Existing dirty worktree changes are user-owned and must not be reverted.
- Because `problem.md`, `gap.md`, and
  `docs/acceptance/ga-acceptance-trace-matrix.md` are already dirty in the
  current worktree, implementation must preserve the current dirty baseline and
  patch only the additional audit-sync wording required by this plan.
- "AC" scope means only `docs/acceptance/ga-acceptance-trace-matrix.md` and
  `docs/acceptance/image-build.md`.
- Local/static/fixture evidence must remain visibly separate from live external
  GA evidence.

## 5. Non-Goals

- No product code changes.
- No tests, fixtures, migrations, manifests, CI workflows, or runtime config
  changes.
- No broader `docs/acceptance/` rewrite.
- No claim that archive upload, BuildKit/Tekton execution, Harbor push, SBOM,
  scan, signing, full allow-list workflow, HPC optimized mover, or remote
  SonarCloud cleanup is complete.

## 6. Current Behavior

The trackers already mark several related areas open, but wording is uneven:

- `problem.md` does not yet prominently carry the audit's metadata-only
  archive/image-build API-contract gaps, missing source fingerprint gap, HPC
  storage planning-layer maturity, and local-vs-remote Sonar state.
- `gap.md` image-build and storage rows contain partial evidence, but should use
  matching V1/Full GA blocker wording and clearer FastTransfer/HPC limits.
- `docs/acceptance/image-build.md` still presents supported sources and build
  flow as target behavior without enough current-status separation.
- `docs/acceptance/ga-acceptance-trace-matrix.md` has an IMG row and STORAGE row
  that need audit-aligned wording, not a broad acceptance-doc rewrite.

## 7. Target Behavior

After implementation:

- `problem.md` states that archive/image-build source handling is currently
  metadata-only/API-contract evidence: JSON build request routes and fixtures
  exist, but source content is not uploaded, unpacked, hashed, validated, saved,
  or dispatched to a live executor.
- `problem.md` states that image-build idempotency fingerprinting omits source
  identity/content: Dockerfile content, context/archive digest, storage path or
  object identity, build args, source revision, and checksum.
- `problem.md` states that HPC storage maturity is best described as a planning
  layer plus basic data-plane evidence, not production-grade HPC optimization.
- `problem.md` states that local SonarScanner status is clean, while remote
  SonarCloud cleanup still depends on UI Analysis Scope or CI-based analysis for
  remaining automatic-analysis exclusions.
- `gap.md` uses matching V1 external production launch and Full GA blocker
  wording for image-build/archive/storage gaps.
- `gap.md` updates the IMG row to distinguish route/fixture/queued metadata and
  local guards from missing live executor, source handling, source
  fingerprinting, SBOM/scan/sign, Harbor push, and full allow-list workflow.
- `gap.md` updates STORAGE/FastTransfer/HPC wording to call out planning-layer
  storage profiles, basic `cp -a` stage-in, single-worker `rsync -a --delete`
  mover, limited progress callbacks, and missing checksum/resume/throughput/
  benchmark/cache/checkpoint optimization.
- `docs/acceptance/ga-acceptance-trace-matrix.md` updates only the IMG and
  STORAGE-related wording needed to match the audit.
- `docs/acceptance/image-build.md` updates only AC/source/current-evidence
  wording needed to mark the build flow as GA target and current implementation
  as metadata/API-contract evidence.

## 8. Affected Domains

- Documentation and GA tracker state only.
- Relevant functional areas: image-registry/image build, storage/FastTransfer/
  HPC storage, Sonar evidence tracking.
- No backend microservice code boundary changes.

## 9. Affected Files

Implementation may edit only:

- `problem.md`
- `gap.md`
- `docs/acceptance/ga-acceptance-trace-matrix.md`
- `docs/acceptance/image-build.md`

Implementation must not edit:

- product code under `backend/`
- tests or fixtures
- CI workflows
- other `docs/acceptance/*` files
- existing unrelated dirty files

## 10. API / Contract Changes

None. The plan changes documentation only.

The docs should explicitly say the current build routes and fixtures are
contract/metadata evidence, not proof of source upload, source extraction, source
hashing, live execution, or completed supply-chain policy.

## 11. Database / Migration Changes

None.

## 12. Configuration Changes

None.

Sonar wording may mention configuration status, but no Sonar configuration file,
workflow, or project setting is changed by this task.

## 13. Observability Changes

None.

The docs may clarify that image-build logs/cancel/status and FastTransfer
progress are limited evidence today and do not prove full live execution,
resource termination, byte accounting, throughput, checksum, or resume behavior.

## 14. Security Considerations

- Keep archive upload security gaps open: path traversal, symlink/hardlink
  policy, zip bomb controls, max archive size, max file count, and checksum
  validation are not implemented.
- Keep from-storage trust-boundary gaps open: permission and mount-plan checks
  must be proven before executor use.
- Keep supply-chain gaps open: SBOM, scan, signing/attestation, digest allow-list
  workflow, and Harbor push are not live-complete.
- Keep Sonar remote-status wording precise: local scanner clean does not equal
  remote automatic-analysis closure.

## 15. Implementation Steps

1. Before editing, inspect `git status --short` and the current diff for the
   four allowed files. Treat existing hunks as user-owned baseline; do not
   reset, checkout, reformat, or rewrite whole files.
2. In `problem.md`, add or revise concise gap language in the summary, feature
   gap table, verification/Sonar section, and recommended execution order as
   needed. Use the audit wording without adding new claims.
3. In `gap.md`, revise the V1/Full GA status language and the IMG, STORAGE,
   FastTransfer, and HPC-related entries to match the audit's blocker wording.
4. In `docs/acceptance/ga-acceptance-trace-matrix.md`, update only the IMG row
   and STORAGE/FastTransfer/HPC evidence wording necessary to reflect the audit.
5. In `docs/acceptance/image-build.md`, change "Supported Build Sources" and
   build-flow wording so it reads as GA target behavior, then add a current
   implementation note that the current API is metadata-only/contract evidence.
6. Review the diff for scope. Remove only Code Agent-added unrelated churn; do
   not remove pre-existing dirty hunks.

## 16. Verification Plan

Run:

```text
git diff --check
git diff -- problem.md gap.md docs/acceptance/ga-acceptance-trace-matrix.md docs/acceptance/image-build.md
python3 docs/tests/verify_ga_acceptance_trace_matrix.py
rg -n "metadata-only|API-contract|source fingerprint|fingerprint|HPC storage planning layer|local Sonar|remote SonarCloud|single-worker|rsync -a --delete|cp -a|BuildKit|Tekton|Harbor push|SBOM|signature|allow-list" problem.md gap.md docs/acceptance/ga-acceptance-trace-matrix.md docs/acceptance/image-build.md
```

Go tests are not required because the approved scope is docs-only. If the Code
Agent touches product code despite this plan, it must stop and request review
before continuing.

## 17. Rollback Plan

Rollback must reverse only the Code Agent's own audit-sync hunks. Do not revert
whole files, because several allowed target files already have user-owned dirty
changes in the current baseline. If the Code Agent cannot isolate its own hunks,
it must stop and request Reviewer guidance instead of using `git checkout`,
`git restore`, or a whole-file rewrite.

## 18. Risks and Tradeoffs

- Risk: overly broad acceptance-doc edits create review noise. Mitigation: only
  edit the two named AC files and only the relevant rows/sections.
- Risk: wording accidentally closes live GA blockers. Mitigation: keep every
  local/static/fixture/current-route statement scoped and keep live external
  evidence requirements open.
- Tradeoff: this plan does not restructure the trackers. Minimal wording sync is
  enough for the requested source-of-truth alignment.

## 19. Reviewer Checklist

- Plan edits only this file.
- Implementation edits only the four affected docs.
- Audit remains the source of truth.
- V1 external production launch remains Open.
- Full GA remains Open.
- IMG wording distinguishes metadata/API-contract evidence from live build
  execution.
- Storage wording distinguishes planning layer/basic copy evidence from HPC
  optimization.
- Sonar wording distinguishes local clean status from remote cleanup conditions.
- Verification commands pass or failures are recorded with exact output.

## 20. Status

Status: Approved
