# Owner-Read Contract Fixtures

## 1. Objective

Implement the next Day 16-35 Contracts And Events slice by adding versioned owner-read contract fixtures for the scheduler admission boundary. The slice must give reviewers concrete, runnable evidence for the owner-read records that scheduler-quota consumes from org-project and workload before command API fixtures or Outbox/Inbox runtime work begins.

## 2. Background

The connector preflight for this branch read `AGENTS.md`, `docs/roadmap.md`, `problem.md`, current branch/status, and the latest merged diff on `origin/main`. `docs/roadmap.md` says Day 16-35 includes versioned internal contract fixtures for owner-read and command APIs. `problem.md` still marks contract testing as a high-priority blocker because core event fixtures exist, but internal HTTP owner-read/command contracts and producer/consumer coverage are not yet all versioned artifacts.

Current repo evidence read through the connector:

- `backend/internal/services/schedulerquota/read_contracts.go` consumes owner-read resources from org-project and workload for submit-admission decisions.
- `backend/internal/platform/service_client.go` defines the current remote read contract routing for `org-project-service:projects`, `org-project-service:project_members`, `org-project-service:user_quotas`, `org-project-service:user_groups`, and `workload-service:jobs`.
- `backend/internal/services/orgproject/internal_read_contracts.go` registers service-key-gated org-project read contracts, including composite-key trailing wildcard get routes.
- `backend/internal/services/workload/internal_read_contracts_test.go` confirms workload jobs are service-key-gated and list-only.
- `backend/internal/services/schedulerquota/read_contracts_test.go` confirms isolated scheduler-quota reads owner snapshots remotely and fails closed on bad service keys.
- `backend/internal/e2e/cross_service_e2e_test.go` includes `TestSchedulerAdmissionOwnerReadContractsE2E`, which exercises scheduler admission owner-read behavior across services.
- `backend/internal/contracts` currently has versioned event fixtures, but no owner-read fixture directory.

## 3. Source References

- `AGENTS.md`
- `docs/agents/workflow.md`
- `docs/agents/planning.md`
- `docs/agents/review-checklist.md`
- `docs/agents/coding-guidelines.md`
- `docs/agents/project-structure.md`
- `docs/roadmap.md`
- `problem.md`
- `backend/docs/api-route-mapping.md`
- `backend/docs/event-contracts.md`
- `backend/internal/contracts/contracts.go`
- `backend/internal/contracts/event_envelope_test.go`
- `backend/internal/platform/read_contract.go`
- `backend/internal/platform/service_client.go`
- `backend/internal/platform/service_client_test.go`
- `backend/internal/services/schedulerquota/read_contracts.go`
- `backend/internal/services/schedulerquota/read_contracts_test.go`
- `backend/internal/services/orgproject/internal_read_contracts.go`
- `backend/internal/services/orgproject/internal_read_contracts_test.go`
- `backend/internal/services/workload/internal_read_contracts_test.go`
- `backend/internal/e2e/cross_service_e2e_test.go`
- Microservice architecture skill references: communication contracts, data consistency, testing/delivery, service boundaries, and review checklists.

## 4. Assumptions

- This slice follows the already merged core event fixture slice.
- Scheduler admission is the narrowest useful owner-read fixture boundary because it already has owner/consumer runtime tests and is called out by `problem.md` blocker tracking.
- Fixtures should live beside existing contract artifacts under `backend/internal/contracts/fixtures/owner-read/v1/`.
- The fixture shape should describe the contract metadata and example `contracts.Record[map[string]any]` records without changing runtime route handlers or external `/api/v1` behavior.
- Additive fields are allowed for owner-read records; required identity/routing fields must remain stable for scheduler admission.

## 5. Non-Goals

- Do not change external `/api/v1` routes, response envelopes, frontend behavior, or OpenAPI output.
- Do not change runtime owner-read routing, scheduler admission logic, stores, service URLs, service-key behavior, or deployable units.
- Do not add command API fixtures in this slice.
- Do not add Outbox/Inbox tables, lag metrics, retry/dead-letter runtime, migrations, Redis/broker changes, or deployment manifests.
- Do not broaden the fixture set beyond the scheduler admission owner-read dependencies.
- Do not add dependencies or code generators.

## 6. Current Behavior

Owner-read runtime behavior exists and has targeted tests, but the contracts are not represented as versioned JSON artifacts. Reviewers can inspect route maps in Go code, but there is no canonical artifact that lists owner service, consumer service, resource name, list/get path, list-only semantics, auth requirement, key shape, and representative record payload for each scheduler admission dependency.

## 7. Target Behavior

The repository should include:

- five v1 owner-read JSON fixtures for scheduler admission dependencies:
  - `org-project-projects.json`
  - `org-project-project-members.json`
  - `org-project-user-quotas.json`
  - `org-project-user-groups.json`
  - `workload-jobs.json`
- contract tests that load every fixture, assert the exact fixture list, validate required metadata, validate `schema_version == 1`, validate service-key-required metadata, validate list/get/list-only path metadata, decode records as `contracts.Record[map[string]any]`, reject secret/internal-ID-shaped fields, and verify the fixture routes align with `platform` domain read contracts;
- a focused remote-reader test that serves fixture payloads and proves `NewRemoteServiceReader` can consume the versioned fixture paths, including list-only failure for workload jobs get;
- backend docs that catalog owner-read fixtures and design constraints;
- `problem.md` updated to show scheduler admission owner-read fixtures now exist while command API fixtures, broader producer/consumer coverage, and Outbox/Inbox runtime remain open.

## 8. Affected Domains

- Internal owner-read contract artifacts.
- Cross-service scheduler admission contract test coverage.
- GA roadmap blocker tracking.
- Backend contract documentation.

## 9. Affected Files

- `docs/plan/2026-06-19-owner-read-fixtures.md`
- `backend/internal/contracts/fixtures/owner-read/v1/org-project-projects.json`
- `backend/internal/contracts/fixtures/owner-read/v1/org-project-project-members.json`
- `backend/internal/contracts/fixtures/owner-read/v1/org-project-user-quotas.json`
- `backend/internal/contracts/fixtures/owner-read/v1/org-project-user-groups.json`
- `backend/internal/contracts/fixtures/owner-read/v1/workload-jobs.json`
- `backend/internal/contracts/owner_read_fixtures_test.go`
- `backend/internal/platform/owner_read_fixtures_test.go`
- `backend/docs/owner-read-contracts.md`
- `problem.md`

## 10. API / Contract Changes

No external API changes. This slice adds versioned internal owner-read contract artifacts for the already-existing scheduler admission dependency boundary. The artifacts document current internal HTTP contracts and do not change runtime request/response behavior.

## 11. Database / Migration Changes

No database or migration changes. Fixtures use synthetic UUID-style and stable composite IDs only; tests must reject internal database ID-like or secret-shaped fields in fixture record payloads.

## 12. Configuration Changes

No configuration changes.

## 13. Observability Changes

No runtime observability changes. The fixtures document service-key-gated owner reads; future runtime Outbox/Inbox lag metrics and dead-letter visibility remain separate roadmap slices.

## 14. Security Considerations

Fixtures must not contain secrets, tokens, passwords, cookies, credentials, owner passwords, connector auth, tunnel tokens, local metadata, or raw private tenant data. The contract tests should reject secret-looking and internal-ID-looking fixture keys recursively. The fixture metadata should state `auth: service_key` and `service_key_required: true` for every owner-read contract.

## 15. Implementation Steps

1. Add this plan with `Status: Draft`.
2. Run Reviewer Agent plan review and mark the plan approved before implementation.
3. Add five versioned owner-read fixture JSON files under `backend/internal/contracts/fixtures/owner-read/v1/`.
4. Add `backend/internal/contracts/owner_read_fixtures_test.go` to validate fixture filenames, metadata, records, required fields, additive compatibility, and forbidden field names.
5. Add `backend/internal/platform/owner_read_fixtures_test.go` to verify fixture route metadata matches `domainReadContracts` and can be consumed by `NewRemoteServiceReader`.
6. Add `backend/docs/owner-read-contracts.md` documenting the scheduler admission fixture catalog and compatibility rules.
7. Update `problem.md` to record completed scheduler admission owner-read fixture coverage and remaining Day 16-35 blockers.
8. Run focused checks, required gates, and Reviewer Agent final approval.

## 16. Verification Plan

Focused checks:

```sh
go -C backend test ./internal/contracts -run OwnerRead -count=1
go -C backend test ./internal/platform -run OwnerRead -count=1
go -C backend test ./internal/services/schedulerquota -run AdmissionReader -count=1
go -C backend test ./internal/services/orgproject -run InternalReadContracts -count=1
go -C backend test ./internal/services/workload -run InternalJobsReadContract -count=1
go -C backend test -tags e2e ./internal/e2e -run TestSchedulerAdmissionOwnerReadContractsE2E -count=1 -v
rg -n "owner-read|OwnerRead|org-project-service:projects|workload-service:jobs|service_key_required" backend/internal/contracts backend/internal/platform backend/docs problem.md
```

Required gates:

```sh
git diff --check
go -C backend test ./... -count=1
go -C backend vet ./...
go -C backend build ./...
bash backend/scripts/ci-security-gate.sh quick
```

Live Kubernetes, staging evidence, remote Sonar, and full security scan are not required unless the implementation unexpectedly changes runtime behavior or deployment artifacts. If any such gate becomes relevant, run it or record the blocker explicitly.

## 17. Rollback Plan

Revert the owner-read fixture files, fixture validation tests, platform fixture test, docs update, plan file, and `problem.md` update. No runtime API, database, deployment, or configuration rollback is required.

## 18. Risks and Tradeoffs

- Fixture artifacts can drift from runtime route maps if they are not validated against `domainReadContracts`; this plan includes a platform package test specifically to prevent that drift.
- The fixture set covers scheduler admission owner-read dependencies only, not every internal read dependency in the system. This keeps the PR reviewable and leaves broader fixtures for later slices.
- The contract shape is a Go-tested JSON artifact instead of JSON Schema. This follows the existing event fixture pattern and avoids new toolchain risk.
- Workload jobs remain list-only in the fixture. This matches current runtime behavior and should be explicitly verified so future changes do not assume unsupported get semantics.

## 19. Reviewer Checklist

- Requirement fit: implements the Day 16-35 owner-read fixture requirement for a concrete scheduler admission boundary.
- Scope control: does not change runtime routes, scheduler logic, DB, deployment, config, or unrelated files.
- Architecture: aligns with owner-read/read-model migration discipline before further service split work.
- API contract: preserves external `/api/v1` compatibility.
- Data ownership: fixtures document owner service and consumer service, and avoid internal DB IDs.
- Config: no config changes.
- Observability: no runtime metrics changes in this slice.
- Security: fixtures are synthetic and service-key-gated; tests reject secret/internal-ID-shaped keys.
- Testing: focused owner-read tests, E2E owner-read check, and required gates are explicit.
- Rollback: revert-only rollback is realistic.

## 20. Status

Status: Approved

## 21. Reviewer Plan Approval

| Category | Result |
| --- | --- |
| Requirement Fit | Pass: Day 16-35 owner-read contract fixture slice is directly addressed. |
| Scope Control | Pass: limited to scheduler admission owner-read fixtures, tests, docs, and `problem.md`. |
| API Compatibility | Pass: external `/api/v1` behavior is explicitly unchanged. |
| Security | Pass: synthetic fixture data, service-key metadata, and secret/internal-ID rejection are covered. |
| Testing / Gates | Pass: focused owner-read tests, E2E owner-read check, required Go gates, and security quick gate are listed. |
| Rollback | Pass: revert-only rollback is realistic because there are no runtime, DB, config, or deployment changes. |

Status: Approved

## 22. Reviewer Final Approval

| Category | Result |
| --- | --- |
| Requirement Fit | Pass: implementation matches the approved Day 16-35 scheduler admission owner-read fixture slice. |
| Scope Control | Pass: only expected fixture, test, docs, plan, and `problem.md` files changed. No runtime route, database, config, deployment, or external `/api/v1` behavior changed. |
| Security | Pass: fixtures are synthetic, service-key-gated, and tests reject secret/internal-ID-shaped record fields. |
| Contract Coverage | Pass: contracts tests validate exact fixture list, schema, metadata, required fields, additive compatibility, and forbidden keys; platform tests bind fixture route metadata to `domainReadContracts` and exercise `NewRemoteServiceReader`. |
| Testing / Gates | Pass: focused owner-read tests, targeted owner-read E2E, full backend tests, vet, build, diff check, and quick security gate passed. |
| Documentation / Blockers | Pass: `backend/docs/owner-read-contracts.md` and `problem.md` record completed scheduler admission owner-read fixture coverage and remaining command API, broader owner-read, producer/consumer event, Outbox/Inbox, staging, and remote Sonar blockers. |
| SOLID / 12-Factor / Boundary | Pass: no new runtime abstraction, dependency, config, shared writable data, or deployable-unit change; fixtures preserve owner-service authority while documenting transitional owner-read contracts. |
| Rollback | Pass: revert-only rollback remains realistic. |

Residual risks:

- Fixture coverage is intentionally limited to scheduler admission owner-read dependencies.
- Provider-side route drift remains covered by existing service tests and E2E evidence; the new fixture drift test primarily binds to consumer-side `domainReadContracts`.
- Remote Sonar and live staging evidence remain out of scope for this slice and tracked in `problem.md`.

Status: Approved
