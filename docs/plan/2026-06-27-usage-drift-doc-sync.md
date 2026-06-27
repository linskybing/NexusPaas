# Usage-Drift Documentation Sync (Post 9a75152)

## 1. Objective

Create a docs-only plan to synchronize acceptance and gap evidence after commit
`9a75152` so `USAGE-029` records local evidence for:

- `UsageDriftDetected` emission,
- persisted `usage_drift_alerts` de-duplication, and
- the passing implementation evidence that supports Sonar/test expectations.

## 2. Background

Commit `9a75152` adds drift-alerting behavior in `usage-observability-service`
for material scheduler reservation vs observed snapshot divergence:

- `UsageDriftDetected` event contract and fixture,
- deduplication in persisted `usage_drift_alerts`,
- drift tests in `backend/internal/services/gpuusage/collector_test.go`,
- `resourcehours` contract/spec updates and event fixture validation updates.

Current docs still list `USAGE-029` in acceptance but do not document this local
evidence in a dedicated, test-backed way.

## 3. Source References

- `backend/internal/services/gpuusage/collector.go`
- `backend/internal/services/gpuusage/collector_test.go`
- `backend/internal/services/gpuusage/usage_drift.go`
- `backend/internal/contracts/event_envelope_test.go`
- `backend/internal/contracts/fixtures/events/v1/usage-drift-detected.json`
- `backend/internal/services/resourcehours/spec.go`
- `docs/acceptance/usage-attribution.md`
- `docs/acceptance/gap-analysis.md`
- `docs/plan/2026-06-27-usage-telemetry-drift.md`

## 4. Assumptions

- No code changes are to be made in this task.
- The implementation from `9a75152` is already present on the branch.
- Tests/commands listed in Section 16 are expected to pass when run on the same
  branch state.

## 5. Non-Goals

- No runtime behavior changes, new endpoints, migrations, or config changes.
- No GA matrix edits unless explicitly approved by the Reviewer during handoff.
- No promises of live GPU hardware telemetry proof; only local evidence is documented.

## 6. Current Behavior

- `USAGE-029` exists in acceptance criteria, but its local evidence is not yet
  captured in the acceptance evidence section.
- `gap-analysis.md` currently references only generic usage-read-model and live GUI
  status evidence, without naming `UsageDriftDetected`/`usage_drift_alerts`.

## 7. Target Behavior

- `docs/acceptance/usage-attribution.md` should explicitly state that `USAGE-029`
  has local evidence through the post-9a75152 drift path:
  - material reservation/telemetry mismatch detection, and
  - `UsageDriftDetected` event generation.
- The same file should call out deduplication via persisted `usage_drift_alerts`
  (active/ resolved handling, fingerprint-based repeat suppression).
- `docs/acceptance/gap-analysis.md` should include this same evidence in the usage
  coverage text (and keep non-committal language where live/production proof is
  still absent).
- If needed, optionally extend
  `docs/acceptance/ga-acceptance-trace-matrix.md` usage row evidence scope to
  mention this local docs-backed control-plane signal.

## 8. Affected Domains

- Documentation ownership for usage acceptance and gap evidence.

## 9. Affected Files

- **Required**:
  - `docs/acceptance/usage-attribution.md`
  - `docs/acceptance/gap-analysis.md`
- **Optional if requested by Reviewer**:
  - `docs/acceptance/ga-acceptance-trace-matrix.md`

## 10. API / Contract Changes

None in this task. Documentation should reference existing contracts/events:

- `UsageDriftDetected` (`usage-observability-service`)
- `usage_drift_alerts` internal record key

## 11. Database / Migration Changes

None in this task.

## 12. Configuration Changes

None in this task.

## 13. Observability Changes

None in this task (already implemented in code). Document event evidence and the
dedupe mechanics; do not add telemetry guidance.

## 14. Security Considerations

- Keep documentation accurate and non-sensitive:
  - No secrets, tokens, or raw tenant identifiers beyond already published test artifacts.
  - Do not infer or claim security/compliance outcomes not present in evidence.

## 15. Implementation Steps

1. Update `docs/acceptance/usage-attribution.md`:
   - In `USAGE-029` evidence area, add a concise local evidence paragraph.
   - Include:
     - material drift scenario,
     - `UsageDriftDetected` event emission,
     - persisted `usage_drift_alerts` dedupe and `resolve` behavior,
     - test anchors (e.g., `TestUsageDriftDetectorEmitsAndDedupesMaterialDrift`).
   - Add a reference to
     `../plan/2026-06-27-usage-telemetry-drift.md`.

2. Update `docs/acceptance/gap-analysis.md`:
   - Add `USAGE-029`-relevant local evidence in the usage-related narrative.
   - Keep the distinction between local evidence and unproven live GPU hardware
     evidence explicit.

3. Optional: Update `docs/acceptance/ga-acceptance-trace-matrix.md` usage row scope
   only if reviewer requires end-to-end trace consistency.

4. Post-edit sanity:
   - Run `git diff --check`.
   - Run evidence presence checks from Section 16.

## 16. Verification Plan

- Doc checks:
  - `rg "USAGE-029|UsageDriftDetected|usage_drift_alerts" docs/acceptance/usage-attribution.md docs/acceptance/gap-analysis.md`
  - `git diff --check`
- Code-behavior evidence validation (reference only, not modified by this task):
  - `cd backend && go test ./internal/services/gpuusage -run "UsageDriftDetector"`
  - `cd backend && go test ./internal/contracts -run "EventEnvelopeFixturesAreValidV1"`
  - `cd backend && go test ./internal/contracts -run "TestEventEnvelopeFixturesAreValidV1"`
  - `cd backend && make ci-sonar`

## 17. Rollback Plan

- Revert doc changes in the above files if evidence wording is judged too
  specific or if contract names/test anchors change.

## 18. Risks and Tradeoffs

- Overstating evidence scope as production-grade usage is an acceptance risk.
  - Mitigation: phrase all additions as "local evidence" and separate from live proofs.
- Test-name drift: test function names may change in future refactors.
  - Mitigation: anchor on behavior descriptors (`UsageDriftDetected`, `usage_drift_alerts`)
    in addition to test names.
- GA matrix update scope creep.
  - Mitigation: mark as optional and only apply with explicit reviewer buy-in.

## 19. Reviewer Checklist

- [ ] Scope is docs-only with no file edits outside `docs/`.
- [ ] `USAGE-029` local evidence is explicit in `usage-attribution.md`.
- [ ] Deduplication via persisted `usage_drift_alerts` is documented.
- [ ] `UsageDriftDetected` contract/event naming is accurate and consistent.
- [ ] `gap-analysis.md` adds the same evidence without claiming unavailable live proof.
- [ ] Tests/Sonar verification commands are listed.
- [ ] Risks include over-claiming and stale wording rollback handling.

## 20. Status

Status: Approved by Reviewer Agent and implemented
