# GPU Usage Evidence Doc Sync

## 1. Objective

Synchronize acceptance and gap documentation with the local evidence added in
commit `83a6f9b usage: split reserved and observed gpu attribution`.

## 2. Background

The implementation slice added:

- Project GPU response fields from `clusterread`: `used`,
  `observed_gpu_pods`, `observed_gpu_source`, `reserved_gpu_fraction`,
  `reserved_gpu_source`, and `sm_attribution_source`;
- reserved-fraction resolution from cluster summary rows, fresh GPU usage
  snapshots, then co-hosted workload jobs;
- MPS SM attribution labels in `gpuusage`, including
  `estimated_mps_allocation` for allocation-derived SM and unavailable-source
  handling;
- frontend Usage panel fields for observed GPU pods, reserved GPU fraction, and
  SM attribution;
- passing verification: `go test ./...`, `go build ./...`,
  `npm test -- --run src/api.test.ts src/App.test.tsx`, `npm run build`,
  `git diff --check`, `make coverage`, and `make ci-sonar`.

## 3. Source References

- `docs/acceptance/gpu-dra-mps.md`
- `docs/acceptance/usage-attribution.md`
- `docs/acceptance/gap-analysis.md`
- `docs/plan/2026-06-27-gpu-usage-reserved-observed.md`
- commit `83a6f9b usage: split reserved and observed gpu attribution`

## 4. Assumptions

- The implementation from commit `83a6f9b` is already present.
- This task edits documentation only.
- Local control-plane/UI evidence is useful, but it is not hardware evidence.

## 5. Non-Goals

- No code, test, fixture, generated asset, or deployment manifest changes.
- No live GPU hardware, DCGM, NVML, PID-to-container, or process telemetry E2E
  proof.
- No usage-attribution family completion claim.
- No update to `docs/acceptance/iteration-plan.md` unless a reviewer requests a
  direct milestone wording change.

## 6. Current Behavior

- Acceptance docs describe GPU usage and MPS attribution requirements.
- They do not fully reflect the local reserved-versus-observed UI/API evidence
  from commit `83a6f9b`.
- Existing wording can still sound like a single Project GPU pods summary.

## 7. Target Behavior

- `gpu-dra-mps.md` records `GPU-017` and `GPU-018` local evidence.
- `usage-attribution.md` records partial local evidence for `USAGE-015`,
  `USAGE-016`, `USAGE-031`, and `M6-006`.
- `gap-analysis.md` describes the separated observed/reserved/source-label
  summary and preserves live GPU caveats.

## 8. Affected Domains

- Acceptance documentation for GPU/DRA/MPS and usage attribution.
- No service ownership or runtime domain changes.

## 9. Affected Files

- `docs/acceptance/gpu-dra-mps.md`
- `docs/acceptance/usage-attribution.md`
- `docs/acceptance/gap-analysis.md`

## 10. API / Contract Changes

None in this docs-sync task. It documents the already implemented
backward-compatible Project GPU usage response extension.

## 11. Database / Migration Changes

None.

## 12. Configuration Changes

None.

## 13. Observability Changes

None in this docs-sync task. It documents existing source-label behavior for
reserved, observed, estimated, and unavailable GPU usage evidence.

## 14. Security Considerations

The docs must not add secrets, high-cardinality process identifiers, PID data,
container IDs, or admin-only fallback claims.

## 15. Implementation Steps

1. In `gpu-dra-mps.md`, extend the current local evidence section to include
   `GPU-017` and `GPU-018`.
   - Reference `docs/plan/2026-06-27-gpu-usage-reserved-observed.md`.
   - State the local response fields, frontend rendering, MPS source labels, and
     tests.
   - Preserve the caveat that `GPU-016` remains open until live DRA/MPS GPU
     cluster validation exists.
2. In `usage-attribution.md`, add a short Current Local Evidence section after
   the acceptance table.
   - Mark `USAGE-015`, `USAGE-016`, and `USAGE-031` as partially covered by
     local read-model/UI evidence.
   - State that `USAGE-013`, `USAGE-014`, `USAGE-017`, `USAGE-018`, and
     `USAGE-035` through `USAGE-037` still need real node/process/GPU evidence.
   - Avoid marking the usage-attribution family complete.
3. In `gap-analysis.md`, update the WEB-007 paragraph.
   - Replace "Project GPU pods summary" wording with the new separated
     observed/reserved/source-label summary.
   - Keep the existing frontend/local caveat.
   - Keep real per-device/per-process GPU utilization and live usage attribution
     open.

## 16. Verification Plan

- `git diff --check`
- `rg "Project GPU pods summary|GPU-017|GPU-018|USAGE-015|M6-006" docs/acceptance`
- Optional sanity check:
  `git diff -- docs/acceptance/gpu-dra-mps.md docs/acceptance/usage-attribution.md docs/acceptance/gap-analysis.md`

## 17. Rollback Plan

Revert the three acceptance-doc edits if wording overstates local evidence or
incorrectly implies hardware validation.

## 18. Risks and Tradeoffs

- Overstating local evidence as hardware proof is the main risk.
- Keeping docs scoped to existing behavior avoids another code slice.
- The usage family remains incomplete until real GPU/process telemetry evidence
  exists.

## 19. Reviewer Checklist

- Scope is docs-only.
- `GPU-017` and `GPU-018` evidence is local-only.
- `USAGE-015`, `USAGE-016`, `USAGE-031`, and `M6-006` are described as partial
  local evidence only.
- Live GPU, DCGM, NVML, process telemetry, PID/container mapping, and MPS
  multi-user E2E gaps remain open.
- `git diff --check` passes.

## 20. Status

Status: Approved
