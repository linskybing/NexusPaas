# USAGE-032 / MON-018 Docs Sync

## 1. Objective

Create a docs-only sync plan for `USAGE-032` / `MON-018` after commit
`eb5cd16`, updating acceptance and gap docs to record control-plane evidence for
stale/missing telemetry alerts.

## 2. Background

Commit `eb5cd16` updates usage attribution monitoring behavior so active reserved
GPU jobs with no fresh snapshots produce `UsageDriftDetected` telemetry alerts
with dedicated reasoning for stale/missing snapshot conditions.

The existing `USAGE-029` evidence remains valid as adjacent local drift evidence;
this task is specifically scoped to stale/missing alert coverage and related
documentation.

## 3. Source References

- `docs/acceptance/usage-attribution.md`
- `docs/acceptance/gap-analysis.md`
- `docs/acceptance/ga-acceptance-trace-matrix.md` (optional only if reviewer requests)
- `docs/plan/2026-06-27-usage-telemetry-stale-alert.md`
- `docs/plan/2026-06-27-usage-telemetry-drift.md` (adjacent context for `USAGE-029`)
- `backend/internal/services/gpuusage/collector_test.go` (test basis for stale/missing behavior)
- `backend/internal/contracts/event_envelope_test.go`
- `backend/internal/contracts/fixtures/events/v1/usage-drift-detected.json`

## 4. Assumptions

- The implementation changes in `eb5cd16` are already present on the branch.
- This task edits only docs; no source logic will be changed.
- `docs/acceptance/usage-attribution.md` and `docs/acceptance/gap-analysis.md`
  are the canonical target locations for local MON/usage evidence text.

## 5. Non-Goals

- No code changes, config changes, migrations, or test code edits.
- No claim of live GPU/node-agent E2E proof.
- No full MON completion claim or GA completion claim.
- No runtime behavior change to quota, admission, or release processes.

## 6. Current Behavior

- Current docs already contain local `USAGE-029` material-drift evidence and some
  usage monitoring notes.
- `USAGE-032` / `MON-018` stale-missing alert details are not yet fully
  represented in the acceptance and gap ledger wording required after `eb5cd16`.

## 7. Target Behavior

- `usage-attribution.md` must explicitly document `USAGE-032` local evidence:
  - `UsageDriftDetected` with `reason` and `drift_reason`
    `active_reserved_jobs_missing_fresh_snapshots`,
  - active reserved jobs without fresh `job_gpu_usage_snapshots`.
- Documentation must state `usage_drift_alerts` dedupe and resolution behavior
  for repeated stale-missing conditions.
- `missing_job_ids` must be documented as project-level output containing only
  missing jobs when some jobs in the same project are fresh.
- Explicitly document that stale/missing alerting does **not** change quota,
  admission, or release behavior.
- Add a visible note that evidence is local control-plane only and does not imply
  live GPU/node-agent proof or complete MON.

## 8. Affected Domains

Acceptance documentation for usage monitoring and gap reporting (`USAGE-032` /
`MON-018` and adjacent usage-monitoring context).

## 9. Affected Files

- Required:
  - `docs/acceptance/usage-attribution.md`
  - `docs/acceptance/gap-analysis.md`
- Optional (only if later requested by reviewer):
  - `docs/acceptance/ga-acceptance-trace-matrix.md`

## 10. API / Contract Changes

None.

## 11. Database / Migration Changes

None.

## 12. Configuration Changes

None.

## 13. Observability Changes

None in this task. This task documents existing observability behavior from
`eb5cd16`:
- reasoned `UsageDriftDetected` alerts,
- alert dedupe/resolution state in `usage_drift_alerts`,
- stale/missing snapshot semantics.

## 14. Security Considerations

No security model changes.
Only non-sensitive evidence language is added; no secrets or operational
controls are introduced.

## 15. Implementation Steps

1. Update `docs/acceptance/usage-attribution.md`:
   - In `USAGE-032` / `MON-018` evidence section, add local evidence bullets for
     `UsageDriftDetected` + `active_reserved_jobs_missing_fresh_snapshots`.
   - Document dedupe/resolution via `usage_drift_alerts`.
   - Document mixed-project output with `missing_job_ids` containing only missing jobs.
   - Add explicit caveat text: no quota/admission/release changes and no live
     GPU/node-agent claim.
   - Optionally reference adjacent `USAGE-029` evidence and this plan for context.

2. Update `docs/acceptance/gap-analysis.md`:
   - Add/adjust usage and monitoring gap wording to align with item 1 and keep
     local-only framing.

3. Pause on any `ga-acceptance-trace-matrix.md` edits unless reviewer explicitly
   requests a trace-matrix follow-up.

## 16. Verification Plan

- Confirm edited docs mention:
  - `active_reserved_jobs_missing_fresh_snapshots`,
  - `usage_drift_alerts`,
  - `missing_job_ids` subset behavior,
  - unchanged quota/admission/release behavior,
  - explicit local-only and no full-MON caveat.
- Record validation evidence that branch checks passed:
  - `cd backend && go test ./internal/services/gpuusage -run "UsageDrift|collectGPUUsageTelemetry|TestUsageDrift"`
  - `cd backend && go test ./internal/contracts -run EventEnvelopeFixturesAreValidV1`
  - `cd backend && go test ./...`
  - `cd backend && go build ./...`
  - `cd backend && make coverage`
  - `cd backend && make ci-sonar`

## 17. Rollback Plan

Remove doc edits from both target acceptance/gap files if wording overstates local
evidence scope or incorrectly asserts runtime behavior.

## 18. Risks and Tradeoffs

- Overstating live coverage from control-plane evidence.
  - Mitigation: keep explicit local-only caveats and avoid MON completion wording.
- Drifting wording with adjacent `USAGE-029` evidence and new `USAGE-032` wording.
  - Mitigation: explicitly label adjacency and keep section scope tight.
- Missing alignment in optional trace matrix if later required.
  - Mitigation: keep as optional follow-up to avoid scope creep.

## 19. Reviewer Checklist

- [ ] Section structure exactly matches required 20-section planning format.
- [ ] Plan is docs-only and scoped to approval-target docs.
- [ ] Alignment is to `USAGE-032` / `MON-018`; `USAGE-029` is only adjacent context.
- [ ] Evidence includes committed reason/drift_reason
  `active_reserved_jobs_missing_fresh_snapshots`.
- [ ] Evidence includes `usage_drift_alerts` dedupe and resolution behavior.
- [ ] Evidence includes mixed-project `missing_job_ids` subset semantics.
- [ ] Plan explicitly states no quota/admission/release change.
- [ ] Plan explicitly states local control-plane-only evidence and avoids live GPU/node-agent or full MON claims.
- [ ] Verification commands list tests/build/coverage/Sonar checks.
- [ ] Diff scope is limited to docs targets.

## 20. Status

Status: Approved
