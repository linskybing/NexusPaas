# DATA-017 / DATA-018 Outbox Replay Evidence Sync

## 1. Objective

Update acceptance ledgers to record existing local evidence for `DATA-017`
outbox/consumer lag observability and `DATA-018` replay/dead-letter coverage.

## 2. Background

The platform already exposes outbox, projection lag, retry, replay, and
dead-letter metrics through the internal platform observability path. The
acceptance docs still describe DATA evidence broadly, but do not call out the
focused local evidence now available for `DATA-017` and `DATA-018`.

This task is documentation-only. It records what is already implemented without
claiming live replay cutover, all-service drift coverage, typed ownership
completion, or Full DATA.

## 3. Source References

- `backend/internal/platform/outbox_inbox_metrics.go`
- `backend/internal/platform/observability_test.go`
- `docs/acceptance/data-contracts.md`
- `docs/acceptance/gap-analysis.md`
- `docs/acceptance/ga-acceptance-trace-matrix.md`

## 4. Assumptions

- Existing local tests are sufficient evidence for a local acceptance-ledger
  update.
- No runtime behavior needs to change.
- The DATA acceptance row must remain `Open` because live replay cutover,
  typed ownership, and broader data-contract work remain incomplete.

## 5. Non-Goals

- No production code changes.
- No new metrics, routes, events, migrations, or configuration.
- No live Kubernetes, Redis, or PostgreSQL replay drill.
- No all-service read-model rebuild/cutover proof.
- No Full DATA or Full GA closure claim.

## 6. Current Behavior

- `snapshotOutboxInboxMetrics` records:
  - `nexuspaas_event_outbox_events`
  - `nexuspaas_event_consumer_lag`
  - `nexuspaas_projection_applied_total`
  - `nexuspaas_projection_dead_letters_total`
  - `nexuspaas_projection_retry_total`
  - `nexuspaas_projection_replay_total`
- `TestOperationalEndpointsExposeOutboxInboxRuntimeEvidence` publishes events,
  runs successful and failed projections, calls `ReplayProjection`, and asserts
  lag, applied, dead-letter, retry, replay, and second-scrape stability.
- Acceptance docs mention DATA replay and outbox evidence, but do not clearly
  map this specific local evidence to `DATA-017` and `DATA-018`.

## 7. Target Behavior

- `docs/acceptance/data-contracts.md` includes a concise local evidence section
  for `DATA-017` and `DATA-018`.
- `docs/acceptance/gap-analysis.md` records the same evidence in the DATA gap
  summary while preserving local-only caveats.
- `docs/acceptance/ga-acceptance-trace-matrix.md` updates DATA evidence scope
  and reason text without changing classification to `Done`.

## 8. Affected Domains

- Acceptance documentation for data ownership, events, contracts, and read-model
  replay evidence.
- No service ownership boundary changes.

## 9. Affected Files

- `docs/acceptance/data-contracts.md`
- `docs/acceptance/gap-analysis.md`
- `docs/acceptance/ga-acceptance-trace-matrix.md`

## 10. API / Contract Changes

None.

## 11. Database / Migration Changes

None.

## 12. Configuration Changes

None.

## 13. Observability Changes

No new observability behavior is added. The docs will cite existing metrics and
tests for local evidence only.

## 14. Security Considerations

No security behavior changes. Documentation must avoid secrets, live endpoint
credentials, or sensitive operational data.

## 15. Implementation Steps

1. Update `docs/acceptance/data-contracts.md` under current local evidence:
   - add `DATA-017` metric evidence for outbox count and consumer lag;
   - add `DATA-018` metric/test evidence for replay, retry, and dead letters;
   - keep explicit local-only and no-Full-DATA caveats.
2. Update `docs/acceptance/gap-analysis.md` DATA paragraph:
   - mention the exact local metrics and the operational endpoint test;
   - keep live replay cutover, all-service drift jobs, and typed ownership open.
3. Update `docs/acceptance/ga-acceptance-trace-matrix.md` DATA and
   read-model replay rows:
   - add DATA-017/DATA-018 local evidence to scope/reason text;
   - keep classification `Open`.

## 16. Verification Plan

- `rg -n "DATA-017|DATA-018|nexuspaas_event_consumer_lag|nexuspaas_projection_replay_total|ReplayProjection|dead-letter" docs/acceptance`
- `cd backend && go test ./internal/platform -run "OperationalEndpointsExposeOutboxInboxRuntimeEvidence|Projection"`
- `cd backend && go test ./...`
- `cd backend && go build ./...`
- `cd backend && make coverage`
- `cd backend && make ci-sonar`
- `git diff --check`

## 17. Rollback Plan

Revert the three acceptance-doc edits. No runtime state or migration rollback is
needed.

## 18. Risks and Tradeoffs

- The main risk is overstating local/in-memory evidence as live replay cutover.
  Mitigation: every edited location must preserve the local-only caveat.
- This does not reduce the real DATA backlog; it only makes existing evidence
  traceable.
- Keeping scope docs-only avoids unnecessary code churn because the metrics and
  test already exist.

## 19. Reviewer Checklist

- Scope is docs-only and limited to the three acceptance ledgers.
- `DATA-017` evidence names outbox count and consumer lag metrics.
- `DATA-018` evidence names replay, retry, and dead-letter metrics/tests.
- DATA classification stays `Open`.
- The wording does not claim live rebuild/cutover, all-service drift jobs,
  typed ownership completion, or Full GA.
- Verification includes targeted platform tests plus full test/build/coverage
  and Sonar checks.

## 20. Status

Status: Approved
