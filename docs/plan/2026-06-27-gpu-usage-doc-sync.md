# GPU Usage Evidence Doc Sync

## Objective

Synchronize acceptance and gap documentation with the local evidence added in
commit `83a6f9b usage: split reserved and observed gpu attribution`.

This is a docs-only slice. It records that `GPU-017` and `GPU-018` now have
local control-plane/UI evidence, and that `USAGE-015`, `USAGE-016`,
`USAGE-031`, and `M6-006` have partial local evidence. It must not claim live
GPU hardware, DCGM, NVML, PID-to-container, or process telemetry E2E proof.

## Background

The implementation slice added:

- Project GPU response fields from `clusterread`:
  `used`, `observed_gpu_pods`, `observed_gpu_source`,
  `reserved_gpu_fraction`, `reserved_gpu_source`, and
  `sm_attribution_source`;
- reserved-fraction resolution from cluster summary rows, fresh GPU usage
  snapshots, then co-hosted workload jobs;
- MPS SM attribution labels in `gpuusage`, including
  `estimated_mps_allocation` for allocation-derived SM and unavailable-source
  handling;
- frontend Usage panel fields for Observed GPU pods, Reserved GPU fraction, and
  SM attribution;
- passing verification:
  `go test ./...`, `go build ./...`,
  `npm test -- --run src/api.test.ts src/App.test.tsx`, `npm run build`,
  `git diff --check`, `make coverage`, and `make ci-sonar`.

## Scope

Update these docs only:

- `docs/acceptance/gpu-dra-mps.md`
- `docs/acceptance/usage-attribution.md`
- `docs/acceptance/gap-analysis.md`

Do not edit code, tests, fixtures, generated assets, or deployment manifests.
Do not update `docs/acceptance/iteration-plan.md` unless a reviewer requests a
direct milestone wording change.

## Required Documentation Changes

1. In `gpu-dra-mps.md`, extend the current local evidence section to include
   `GPU-017` and `GPU-018`.
   - Reference the plan
     `docs/plan/2026-06-27-gpu-usage-reserved-observed.md`.
   - State exactly what local evidence exists: response fields, frontend
     rendering, MPS source labels, and tests.
   - Preserve the caveat that `GPU-016` remains open until live DRA/MPS GPU
     cluster validation exists.

2. In `usage-attribution.md`, add a short "Current Local Evidence" section
   after the acceptance table.
   - Mark `USAGE-015`, `USAGE-016`, and `USAGE-031` as partially covered by
     local read-model/UI evidence.
   - State that `USAGE-013`, `USAGE-014`, `USAGE-017`, `USAGE-018`, and
     `USAGE-035` to `USAGE-037` still need real node/process/GPU evidence.
   - Avoid marking the usage-attribution family complete.

3. In `gap-analysis.md`, update the WEB-007 paragraph.
   - Replace "Project GPU pods summary" wording with the new separated
     observed/reserved/source-label summary.
   - Keep the existing frontend/local caveat.
   - Update the remaining WEB gaps to say real per-device/per-process GPU
     utilization and live usage attribution remain open.

## Acceptance Criteria

- Documentation reflects the current branch behavior without claiming hardware
  evidence that does not exist.
- The docs point reviewers to the implementation plan and verification command
  set.
- The remaining open gaps are explicit: `GPU-016`, real PID/container/GPU
  process telemetry, DCGM/NVML/process-exporter deployment evidence, and live
  MPS multi-user E2E.
- `git diff --check` passes.

## Verification

Run:

```bash
git diff --check
rg "Project GPU pods summary|GPU-017|GPU-018|USAGE-015|M6-006" docs/acceptance
```

Optional sanity check:

```bash
git diff -- docs/acceptance/gpu-dra-mps.md docs/acceptance/usage-attribution.md docs/acceptance/gap-analysis.md
```
