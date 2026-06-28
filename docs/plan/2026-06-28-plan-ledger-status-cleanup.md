# Plan Ledger Status Cleanup

## 1. Objective

Normalize recently added `docs/plan/` files so they follow the repository
planning template and use the required final `Status: Approved` form.

## 2. Background

The current feature branch contains implemented slices whose plan artifacts are
useful, but a few plan ledgers drifted from the repository convention:

- one plan has implementation wording instead of a standard status line;
- one docs-sync plan does not use the 20-section template;
- several section-20 status lines include extra prose.

This is documentation hygiene only. It keeps the branch easier for reviewers and
future automation to scan.

## 3. Source References

- `AGENTS.md`
- `docs/agents/planning.md`
- `docs/plan/2026-06-27-gpu-usage-reserved-observed.md`
- `docs/plan/2026-06-27-gpu-usage-doc-sync.md`
- `docs/plan/2026-06-27-gpu-reservation-release-drift.md`
- `docs/plan/2026-06-27-usage-drift-doc-sync.md`
- `docs/plan/2026-06-27-usage-telemetry-drift.md`
- `docs/plan/2026-06-27-usage-telemetry-stale-alert.md`

## 4. Assumptions

- These plan files describe work already implemented and reviewed on this branch.
- Normalizing status text does not change runtime behavior or acceptance scope.
- Existing plan content should be preserved unless it conflicts with the required
  20-section structure.

## 5. Non-Goals

- No production code changes.
- No acceptance criteria status changes.
- No new implementation work.
- No rewrite of already approved technical plans beyond formatting/status cleanup.

## 6. Current Behavior

- Most plan files already have 20 sections and a recognizable approved status.
- `2026-06-27-gpu-usage-doc-sync.md` uses a short ad hoc format.
- Some final status lines include extra words such as implementation notes,
  which makes status scanning inconsistent.

## 7. Target Behavior

- All touched plans have the 20 required section headings.
- Section 20 ends with exactly `Status: Approved`.
- Any implementation/history context is kept in body sections, not in the final
  status value.

## 8. Affected Domains

- Repository documentation and planning artifacts only.

## 9. Affected Files

- `docs/plan/2026-06-27-gpu-usage-reserved-observed.md`
- `docs/plan/2026-06-27-gpu-usage-doc-sync.md`
- `docs/plan/2026-06-27-gpu-reservation-release-drift.md`
- `docs/plan/2026-06-27-usage-drift-doc-sync.md`
- `docs/plan/2026-06-27-usage-telemetry-drift.md`
- `docs/plan/2026-06-27-usage-telemetry-stale-alert.md`

## 10. API / Contract Changes

None.

## 11. Database / Migration Changes

None.

## 12. Configuration Changes

None.

## 13. Observability Changes

None.

## 14. Security Considerations

No secrets or operational evidence values are added. This is template cleanup.

## 15. Implementation Steps

1. Convert `2026-06-27-gpu-usage-doc-sync.md` to the 20 required sections while
   preserving its existing docs-only scope and evidence caveats.
2. Update the final status lines in the listed implemented plans to the exact
   approved form.
3. Keep all runtime, acceptance, and verification claims unchanged.

## 16. Verification Plan

- Check plan section/status shape with shell text scanning.
- `git diff --check`
- `rg -n --glob '!2026-06-28-plan-ledger-status-cleanup.md' "Status: Approved by|Plan Status:|Implementation Status:|Revised draft plan" docs/plan`
- `go test ./...` from `backend/`
- `go build ./...` from `backend/`
- `make coverage` from `backend/`
- `make ci-sonar` from `backend/`

## 17. Rollback Plan

Revert the touched `docs/plan/` files. No runtime rollback is needed.

## 18. Risks and Tradeoffs

- Converting the short docs-sync plan adds lines, but it removes a persistent
  template exception.
- Touching historical plan files can look noisy; limiting the change to status
  and template normalization keeps the diff reviewable.

## 19. Reviewer Checklist

- The change is docs-only.
- All touched files remain in `docs/plan/`.
- No acceptance status or runtime behavior changes are introduced.
- Final status values are exactly `Status: Approved`.
- The short docs-sync plan has the required 20 sections.

## 20. Status

Status: Approved
