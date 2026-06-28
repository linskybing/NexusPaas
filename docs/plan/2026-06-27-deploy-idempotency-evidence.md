# Deploy/Apply Retry Idempotency Evidence for DATA-014 Local Slice

## 1. Objective

Add a local DATA-014 evidence slice proving deploy/apply dispatch retries are idempotent without adding API or infrastructure changes, so a retried infra-resilient dispatch path cannot duplicate-fail or rollback resources when Kubernetes objects already exist.

## 2. Background

- `workload-service` dispatcher already executes manifest creation through `cluster.Client.CreateByJSON` and then marks the job `running` with `created_resources`.
- `cluster.CreateByJSON` currently treats existing native resources as success via `ignoreAlreadyExists`, so create retries are idempotent at the facade level.
- Current gap evidence still says DATA-014 command coverage is partial and specifically missing deploy/apply idempotency.
- This plan explicitly covers **local-only** evidence: no live Kubernetes deploy proof and no full DATA GA claim.
- Real-retry coverage should use the dispatcher retry state path (`waiting_infra` + `next_retry_at` in the past with optional `retry_count`) rather than only the direct initial `submitted` path.

## 3. Source References

- `backend/internal/services/workload/dispatcher.go` (dispatch loop + manifest handling): lines ~105-163, 165-188.
- `backend/internal/platform/cluster/apply.go` (facade + `CreateByJSON` success on `AlreadyExists`): lines ~66-131, 104-121.
- `backend/internal/services/workload/dispatcher_test.go` (existing dispatch behavior, created-resources assertions): lines ~41-97, ~139-164, ~99-136.
- `backend/internal/platform/cluster/apply_test.go` (existing native object idempotent create test for Job): lines ~24-47.
- `docs/acceptance/data-contracts.md` (`DATA-014` contract row): line ~61.
- `docs/acceptance/gap-analysis.md` (local/in-memory partial DATA-014 status and remaining open items).
- `docs/acceptance/ga-acceptance-trace-matrix.md` (DATA row `Open`, deploy coverage still open): table row for DATA.

## 4. Assumptions

- Workload dispatch retry idempotency evidence can be proven with local unit/integration-style tests only.
- Existing idempotency-key behavior and command APIs for submit/cancel/preempt/build stay unchanged.
- Existing `CreateByJSON` support for supported native kinds (`Pod`, `Deployment`, `Job`, etc.) is the current idempotency boundary.
- No new external idempotency-key semantics are introduced for deploy/apply in this slice.

## 5. Non-Goals

- No live Kubernetes deploy proof (staging/production replay, rollback, or external cluster drills).
- No API contract changes.
- No DB migrations/config changes.
- No full DATA GA closure or typed ownership rewrites.
- No live Kubernetes deploy proof; evidence is local/in-memory and scoped to DATA-014 deploy/apply retry behavior.

## 6. Current Behavior

- Duplicate dispatch retry for deploy/apply paths is not explicitly proven in local tests.
- Existing tests cover:
  - submit/cancel/preempt/build command coverage,
  - and `Cluster.CreateByJSON` idempotent behavior for a single Job kind.
- `markDispatchedJobRunning` is only test-covered for the happy path; retry (`waiting_infra`) and pre-existing-object cases are not currently explicit.

## 7. Target Behavior

- A workload record in `waiting_infra` state is retried when `next_retry_at` is in the past after partial deploy attempt and:
  - returns success because manifest resources already exist,
  - transitions to `running`,
  - persists `created_resources` with expected object identities,
  - does not go through rollback/failure path.
- Evidence specifically captures local deploy/apply idempotency and is framed as local evidence only.

## 8. Affected Domains

- `workload-service` dispatch path and `jobs` record lifecycle.
- `cluster` create facade used by dispatch.
- Existing acceptance ledgers for DATA ownership contracts (evidence tracking only).

## 9. Affected Files

- `backend/internal/services/workload/dispatcher_test.go` (new tests for retry-idempotent deployment/apply behavior).
- `backend/internal/platform/cluster/apply_test.go` (extend idempotent coverage if needed for Deployment/native kinds already used by dispatch).
- `docs/acceptance/data-contracts.md` (DATA-014 wording/row status wording constrained to local deploy/apply retry evidence).
- `docs/acceptance/gap-analysis.md` (DATA row remains local/in-memory with local-only deploy/apply idempotency scope and no full GA closure).
- `docs/acceptance/ga-acceptance-trace-matrix.md` (DATA row evidence-scope wording update to reflect this local retry evidence slice and preserve row status boundaries).

## 10. API / Contract Changes

- No API contract changes.
- No new routes, payload fields, or request headers.

## 11. Database / Migration Changes

- None.

## 12. Configuration Changes

- None.

## 13. Observability Changes

- None required for this local evidence slice.

## 14. Security Considerations

- Tests must avoid hard-coded secrets and keep resource names/project/user values non-sensitive.
- No auth/authz path changes.

## 15. Implementation Steps

1. Add a real-retry-path local test in `backend/internal/services/workload/dispatcher_test.go`:
   - Seed a job record in `waiting_infra` state with `next_retry_at` in the past and optional `retry_count` > 0.
   - Pre-create the same native manifest object in fake clientset before dispatch to force `CreateByJSON` to hit `AlreadyExists`.
   - Call dispatch path (`dispatchSubmittedWorkloads(...)`).
   - Assert no error, status `running`, `started_at` set, and `next_retry_at` cleared (and `retry_count` behavior remains as existing `markDispatchedJobRunning` semantics dictate).
   - Assert `created_resources` contains expected kinds and is not duplicated/empty.
   - Assert no transition to `waiting_infra`/`failed` and no missing rollback artifacts.

2. Add/adjust focused assertions for mixed evidence in the same file:
   - Keep a `submitted` duplicate-create case as optional secondary coverage only.
   - Keep one non-idempotent negative case (e.g., unsupported kind or transient dispatch failure) to preserve contrast.
   - Ensure created-resources assertions also cover deploy-like manifest path (Deployment/PodGroup/Pod combinations can remain future follow-up).

3. Optionally extend `backend/internal/platform/cluster/apply_test.go`:
   - Add targeted idempotency assertion for at least one additional supported kind used by dispatch (e.g., Deployment) if existing coverage is considered too narrow.

4. Update evidence docs in the same slice (no follow-up dependency):
   - `docs/acceptance/data-contracts.md`: keep DATA-014 text as local deploy/apply idempotency evidence only.
   - `docs/acceptance/gap-analysis.md`: explicitly describe this as local DATA-014 deploy/apply retry-idempotency evidence, still not Live/Full GA.
   - `docs/acceptance/ga-acceptance-trace-matrix.md`: update DATA row evidence scope to include this local deploy/apply retry evidence without changing row status to full GA.

## 16. Verification Plan

- `cd backend && go test ./internal/services/workload -run "TestDispatchSubmittedWorkload|TestDispatchSubmittedWorkloadsAppliesAtMostBatchLimitPerRun"`
- `cd backend && go test ./internal/platform/cluster -run "TestCreateByJSON"`
- If test fixture/contracts are touched: `cd backend && go test ./internal/contracts -run EventEnvelopeFixturesAreValidV1`
- `cd backend && go test ./...`
- `cd backend && go build ./...`
- `cd backend && make coverage`
- `cd backend && make ci-sonar`
- `git diff --check`

## 17. Rollback Plan

- Remove added tests and revert any doc evidence language edits.
- Keep production dispatch behavior unchanged if regression risk appears.

## 18. Risks and Tradeoffs

- Fake-client-based retry semantics may not catch provider-specific live-side races.
- This slice closes local deploy/apply idempotency evidence only; users and reviewers must not interpret it as live deploy proof.
- Over-broader resource coverage in one step could reduce signal; keep scope to one native kind + one negative control first.

## 19. Reviewer Checklist

- Objective explicitly addresses local DATA-014 deploy/apply idempotency evidence (not live deploy proof).
- No API/config/db/migration changes were introduced.
- Retry path test proves status transition to `running` and preserved `created_resources`.
- Retry case does not trigger rollback/failure path.
- Verification includes requested broad checks (`go test ./...`, `go build`, `make coverage`, `make ci-sonar`, `git diff --check`).

## 20. Status

Status: Approved
