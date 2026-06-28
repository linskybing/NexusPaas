# Image Evidence Ledger Sync for Accepted Slice Alignment

This is a documentation-alignment-only plan. It records already-merged local
evidence in stale ledgers and does not add, change, or imply runtime behavior.

## 1. Objective

Align stale status-ledger docs (`gap.md`, `problem.md`, and `docs/acceptance/gap-analysis.md`) with the approved evidence in `docs/acceptance/image-build.md` and `docs/acceptance/ga-acceptance-trace-matrix.md` for:

- `ImageAccelerationProfile` metadata/contract evidence
- queued `ImageBuildStarted` supply-chain status metadata

No code changes are required; this is a docs-ledger synchronization plan.

## 2. Background

The acceptance docs currently diverge: image-acceleration metadata and queued supply-chain status language does not consistently reflect the already-approved status-slice scope. The gap/ledger/problem docs still imply mixed or stale claims while the canonical evidence documents reflect the active slice.

## 3. Source References

- `docs/acceptance/image-build.md`
- `docs/acceptance/ga-acceptance-trace-matrix.md`
- `docs/acceptance/gap-analysis.md`
- `gap.md`
- `problem.md`

## 4. Assumptions

- The two image-registry slices are approved and should be treated as the current source of truth.
- No implementation or API behavior changes are requested.
- This plan must not create new evidence; it may only cite local evidence already
  present in accepted code, tests, fixtures, and acceptance docs.
- `IMG`, `Supply-chain SBOM/signing`, `V1 external launch`, and `Full GA` remain open per instruction.
- Documentation-only updates must avoid stating any completed live SBOM generation, live signing integration, or runtime scan execution where only queued metadata/status is intended.

## 5. Non-Goals

- No code edits in service, tests, contracts, CI, or build pipeline.
- No introduction of new evidence about runtime SBOM signing, scanning, or digest/publish enforcement.
- No closure of IMG, supply-chain SBOM/signing, V1 external launch, or Full GA status.
- No changes to acceptance behavior beyond text/table alignment and evidence wording.
- No changes to `docs/acceptance/image-build.md` or
  `docs/acceptance/ga-acceptance-trace-matrix.md`; those files are the current
  source text to align against.

## 6. Current Behavior

- `docs/acceptance/gap-analysis.md` and `gap.md` contain outdated wording about image-ledger status coverage.
- `problem.md` retains stale wording that can imply scope beyond queued/status metadata.
- `docs/acceptance/image-build.md` and `docs/acceptance/ga-acceptance-trace-matrix.md` already describe the accepted scope and should be treated as authoritative.

## 7. Target Behavior

- The three ledger docs consistently state that image-ledger alignment is at metadata/contract level plus queued `ImageBuildStarted` supply-chain status metadata.
- Wording explicitly keeps IMG, supply-chain SBOM/signing, V1 external launch, and Full GA status as open.
- No section in target docs claims implemented live SBOM generation, signing workflows, or scan gates.
- Any contradictory or stale statements in `gap.md`, `problem.md`, and `docs/acceptance/gap-analysis.md` are removed or reframed.
- Final-state measurable checks:
  - all three target docs mention `ImageAccelerationProfile` evidence;
  - all three target docs mention queued `ImageBuildStarted` supply-chain status
    metadata or equivalent pending supply-chain defaults;
  - all three target docs keep live SBOM/signing/scan execution and full image
    workflow open.

## 8. Affected Domains

- Documentation taxonomy for acceptance evidence ledgering and trace-matrix alignment.
- No operational, API, storage, or CI-runtime domains are modified.

## 9. Affected Files

- `docs/acceptance/gap-analysis.md`
- `gap.md`
- `problem.md`
- `docs/plan/2026-06-28-image-evidence-ledger-sync.md`

## 10. API / Contract Changes

None. This is a docs-only ledger-sync plan.

## 11. Database / Migration Changes

None.

## 12. Configuration Changes

None.

## 13. Observability Changes

None.

## 14. Security Considerations

No security-impacting behavior changes; no secrets, credentials, signing keys, or pipeline trust boundaries are modified.

## 15. Implementation Steps

1. Update `docs/acceptance/gap-analysis.md` only where image-build fixture or
   IMG evidence wording is stale. Add `ImageAccelerationProfile` and queued
   `ImageBuildStarted` supply-chain status metadata language while keeping live
   SBOM/signing/scan open.
2. Update `gap.md` only in existing image-build ledger paragraphs. Add the same
   evidence wording and explicitly preserve open statuses for IMG,
   supply-chain SBOM/signing, V1 external launch, and Full GA.
3. Update `problem.md` only in existing image-build/typed-contract/supply-chain
   ledger wording. Add aligned, non-contradictory phrasing and no regression to
   claimed live SBOM/signing/scan execution.
4. Do not edit code, contract fixtures, build scripts, or the already-aligned
   acceptance source docs.
5. Re-run the verification checks listed below and capture results as the
   acceptance of doc alignment.

## 16. Verification Plan

- `rg -n "ImageAccelerationProfile|supply-chain|ImageBuildStarted" docs/acceptance/gap-analysis.md gap.md problem.md`
- `rg -n "live SBOM|completed SBOM|completed signing|completed scan|full image workflow" docs/acceptance/gap-analysis.md gap.md problem.md`
- `git diff -- docs/acceptance/gap-analysis.md gap.md problem.md`
- `git diff --check`
- Optional repo-policy pass-through checks if time budget allows:
- `cd backend && go test ./...`
  - `cd backend && go build ./...`
  - `cd backend && make coverage`
  - `cd backend && make ci-sonar`

## 17. Rollback Plan

- Revert wording edits in the three target docs if alignment is found inaccurate.
- Keep all authoritative acceptance files unchanged unless separately approved.

## 18. Risks and Tradeoffs

- Language-only updates can still be misread as implementation scope if wording is not precise; all replacements should explicitly label remaining items as open.
- `go test ./...`, `go build ./...`, coverage, and Sonar commands are noisy and may fail for unrelated pre-existing repo issues; doc scope remains valid if plan acceptance evidence checks pass.
- This does not solve SBOM generation, image signing, scan enforcement,
  allow-list admission, live Harbor/Tekton/BuildKit execution, image promotion,
  or any launch blocker; those are intentionally deferred because this slice
  only corrects ledger drift after already-merged local evidence.

## 19. Reviewer Checklist

- [ ] `gap.md` wording aligned with `docs/acceptance/image-build.md` and `docs/acceptance/ga-acceptance-trace-matrix.md`.
- [ ] `problem.md` contains no stale or over-claiming statements about implemented live SBOM/signing/scan behavior.
- [ ] `docs/acceptance/gap-analysis.md` reflects both accepted slices:
  - `ImageAccelerationProfile` metadata/contract evidence
  - queued `ImageBuildStarted` supply-chain status metadata
- [ ] IMG, supply-chain SBOM/signing, V1 external launch, and Full GA are explicitly marked as open.
- [ ] No claims of completed live SBOM/signing/scan are present after edits.
- [ ] No files outside `docs/acceptance/gap-analysis.md`, `gap.md`,
  `problem.md`, and this plan are modified.
- [ ] Required `rg`, target diff, and `git diff --check` checks completed;
  optional repo-policy pass-through checks reported if run.

## 20. Status

Status: Approved
