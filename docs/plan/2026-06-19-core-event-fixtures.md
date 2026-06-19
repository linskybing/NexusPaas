# Core Event Fixtures

## 1. Objective

Implement the first Day 16-35 Contracts And Events slice by adding versioned core event envelope fixtures and focused contract tests. The slice must give reviewers concrete, runnable evidence that identity, tenant, workload, scheduler, and audit event fixtures follow the accepted GA event envelope before any runtime Outbox/Inbox or producer migration work begins.

## 2. Background

The connector preflight for this branch read `AGENTS.md`, `docs/roadmap.md`, `problem.md`, current branch/status, and the latest merged diff. `docs/roadmap.md` says Day 16-35 starts with versioned internal contract fixtures, event schema fixtures, producer/consumer contract tests for the core event envelope, and Outbox/Inbox foundations. `problem.md` still marks contract testing as a high-priority GA blocker because internal HTTP contracts and event schemas are not all versioned provider/consumer artifacts.

Current repo evidence:

- `backend/internal/contracts/contracts.go` has the current unversioned runtime `Event` type used by the in-process event bus.
- `backend/internal/platform/events.go` already validates basic event metadata, has bounded outbox behavior, and implements consumer idempotency in memory.
- `backend/docs/event-contracts.md` lists the current event catalog and design constraints.
- `docs/adr/0002-outbox-inbox-read-models.md` accepts a canonical event envelope with `event_id`, `schema_version`, `event_type`, `producer`, `occurred_at`, `trace_id` / `request_id`, `aggregate_id`, and `payload`.
- `docs/architecture/testing-strategy.md` requires every event to have a schema fixture, producer test, consumer idempotency test, and compatibility rule.

## 3. Source References

- `AGENTS.md`
- `docs/agents/workflow.md`
- `docs/agents/planning.md`
- `docs/agents/review-checklist.md`
- `docs/agents/coding-guidelines.md`
- `docs/roadmap.md`
- `problem.md`
- `docs/adr/0002-outbox-inbox-read-models.md`
- `docs/architecture/data-migration-strategy.md`
- `docs/architecture/testing-strategy.md`
- `backend/docs/event-contracts.md`
- `backend/internal/contracts/contracts.go`
- `backend/internal/platform/events.go`
- `backend/internal/platform/events_test.go`
- Microservice architecture skill references: communication contracts, data consistency, testing/delivery, service boundaries, and review checklists.

## 4. Assumptions

- This is the first implementation slice after the Day 0-15 architecture and ADR baseline.
- The canonical v1 envelope can be added alongside the existing runtime `contracts.Event` without changing current producers.
- The fixture path should live in `backend/internal/contracts/fixtures/events/v1/` so Go tests and reviewers can inspect the versioned artifacts together.
- `request_id` is useful and should be preserved when present, but `trace_id` remains the required correlation key for v1 fixtures.
- Payloads must carry UUIDs and safe snapshots, not internal database row IDs.

## 5. Non-Goals

- Do not change external `/api/v1` routes, response envelopes, frontend behavior, or OpenAPI output.
- Do not migrate existing runtime producers from `contracts.Event` to the new canonical fixture shape in this slice.
- Do not add Outbox tables, Inbox tables, Redis stream changes, broker changes, migrations, or deployment manifests.
- Do not add owner-read or command API fixtures yet.
- Do not add E2E or live Kubernetes tests for this fixture-only slice.
- Do not add dependencies or code generators.

## 6. Current Behavior

The runtime event type has fields `event_id`, `name`, `source`, `occurred_at`, `trace_id`, `schema_version`, `idempotency_key`, and `data`. It is useful for current in-process event bus behavior, but it is not yet a versioned GA contract artifact. There are no JSON fixture files that reviewers can use to verify compatibility for identity, tenant, workload, scheduler, or audit event producers.

## 7. Target Behavior

The repository should include:

- a canonical v1 event envelope type and validator in `backend/internal/contracts`;
- five representative v1 JSON fixtures for identity, tenant, workload, scheduler, and audit events;
- focused contract tests that load every fixture, validate required metadata, verify additive compatibility, verify missing optional `request_id` compatibility, reject internal DB ID payload keys, and document the v1 compatibility behavior;
- `backend/docs/event-contracts.md` updated to point to the versioned fixtures and compatibility rules;
- `problem.md` updated to show core event envelope fixtures are started while the broader contract-testing blocker remains open.

## 8. Affected Domains

- Internal event contract artifacts.
- Contract test coverage.
- GA roadmap blocker tracking.
- Backend event-contract documentation.

## 9. Affected Files

- `docs/plan/2026-06-19-core-event-fixtures.md`
- `backend/internal/contracts/contracts.go`
- `backend/internal/contracts/event_envelope_test.go`
- `backend/internal/contracts/fixtures/events/v1/user-updated.json`
- `backend/internal/contracts/fixtures/events/v1/project-updated.json`
- `backend/internal/contracts/fixtures/events/v1/job-submitted.json`
- `backend/internal/contracts/fixtures/events/v1/quota-reserved.json`
- `backend/internal/contracts/fixtures/events/v1/audit-event.json`
- `backend/docs/event-contracts.md`
- `problem.md`

## 10. API / Contract Changes

No external API changes. This slice adds internal contract artifacts for the v1 event envelope. It does not change runtime producer output or current `/api/v1` behavior. The new fixture contract defines additive evolution, tolerant readers for unknown fields, and optional `request_id` compatibility.

## 11. Database / Migration Changes

No database or migration changes. The validator should reject internal database ID-like payload keys in fixtures, reinforcing the ADR rule that event payloads carry UUIDs and snapshots rather than fields that enable cross-service joins.

## 12. Configuration Changes

No configuration changes.

## 13. Observability Changes

No runtime observability changes. Fixtures must include `trace_id`, and tests should verify the field is required. Future producer migrations can add metrics and lag telemetry separately.

## 14. Security Considerations

Fixtures must not contain secrets, tokens, passwords, cookies, raw OIDC assertions, or private tenant data. Contract tests should reject secret-looking payload keys and internal database ID-like keys. The change must not read, print, or commit connector auth, owner password, tunnel tokens, or local metadata.

## 15. Implementation Steps

1. Add this plan with `Status: Draft`.
2. Run Reviewer Agent plan review and mark the plan approved before implementation.
3. Add a small canonical v1 `EventEnvelope` type plus validation helpers in `backend/internal/contracts/contracts.go`, using only the standard library.
4. Add five JSON fixtures under `backend/internal/contracts/fixtures/events/v1/` for identity, tenant, workload, scheduler, and audit events.
5. Add focused tests in `backend/internal/contracts/event_envelope_test.go` that load all fixtures and cover required fields, additive top-level/payload compatibility, optional `request_id`, forbidden payload keys, and stable fixture filenames.
6. Update `backend/docs/event-contracts.md` to reference the v1 fixture directory and compatibility rules.
7. Update `problem.md` to record that core event envelope fixtures now exist while owner-read/command fixtures, producer-specific tests, Outbox/Inbox infrastructure, and staging evidence remain open.
8. Run required gates and record final Reviewer Agent approval.

## 16. Verification Plan

Focused checks:

```sh
go -C backend test ./internal/contracts -count=1
go -C backend test ./internal/platform -run Event -count=1
rg -n "event_type|schema_version|producer|aggregate_id|payload|request_id" backend/internal/contracts backend/docs/event-contracts.md problem.md
```

Required gates:

```sh
git diff --check
go -C backend test ./... -count=1
go -C backend vet ./...
go -C backend build ./...
bash backend/scripts/ci-security-gate.sh quick
```

E2E, live Kubernetes, staging evidence, remote Sonar, and full security scan are not required because this slice does not change runtime cross-service behavior or deployment artifacts. The PR must state that these remain future evidence gates.

## 17. Rollback Plan

Revert the event envelope helper, fixture files, contract tests, docs update, plan file, and `problem.md` update. No runtime API, database, deployment, or configuration rollback is required.

## 18. Risks and Tradeoffs

- Adding canonical fixtures before runtime producers means current production events and future GA fixtures briefly coexist. This is intentional; runtime migration should happen only after versioned fixtures and tests exist.
- A Go validator is simpler than adding a JSON Schema dependency or generator. This keeps the slice small and avoids new toolchain risk.
- Five fixtures cover representative domains but not the full event catalog. Follow-up slices must add more fixtures and producer/consumer tests.
- The broader contract-testing blocker remains open until owner-read/command fixtures and producer/consumer coverage exist for all affected services.

## 19. Reviewer Checklist

- Requirement fit: implements the first Day 16-35 event fixture and contract-test slice.
- Scope control: does not change runtime producers, routes, DB, deployment, or CI behavior.
- Architecture: aligns with ADR 0002 and GA data migration strategy.
- API contract: preserves external `/api/v1` compatibility.
- Data ownership: fixtures avoid internal DB IDs and reinforce UUID/snapshot payloads.
- Config: no config changes.
- Observability: `trace_id` remains required in v1 fixtures.
- Security: fixture keys avoid secrets and tests reject secret/internal-ID shaped keys.
- Testing: focused contracts tests plus required gates are explicit.
- Rollback: revert-only rollback is realistic.

## 20. Status

Status: Approved

## 21. Reviewer Plan Approval

| Category | Question | Result |
| --- | --- | --- |
| Requirement Fit | Does the plan directly satisfy Day 16-35 event fixture and contract-test requirements? | Pass |
| Scope Control | Is the slice small enough and reviewable? | Pass |
| Non-Goals | Are runtime producers, DB, deployment, E2E, and unrelated refactors excluded? | Pass |
| Architecture | Does the plan align with ADR 0002 and the data migration strategy? | Pass |
| Microservice Boundary | Does it improve contract discipline before increasing service count? | Pass |
| API Contract | Does it preserve external `/api/v1` compatibility? | Pass |
| Data Ownership | Does it forbid internal DB IDs in event payload fixtures? | Pass |
| Config | Are config changes excluded? | Pass |
| Observability | Does it require `trace_id` in fixtures? | Pass |
| Security | Does it avoid secrets and reject secret-shaped payload keys? | Pass |
| Testing | Are focused and required gates concrete? | Pass |
| Rollback | Is docs/test/helper rollback realistic? | Pass |
| Simplicity | Does it avoid dependencies, generators, and broker changes? | Pass |
| Surgical Change | Are affected files limited to contracts, fixtures, docs, plan, and `problem.md`? | Pass |

Status: Approved

## 22. Reviewer Final Approval

| Category | Review Result |
| --- | --- |
| Requirement Fit | Pass: implements the first Day 16-35 core event envelope fixture slice from the roadmap. |
| Approved-Plan Alignment | Pass: changes are limited to the approved contracts, fixtures, tests, docs, plan, and `problem.md` scope. |
| SOLID / Maintainability | Pass: the validator is a small contracts-package helper using standard library parsing and recursive payload key checks. |
| 12-Factor App | Pass: no config, runtime process, backing service, or deployment behavior changed. |
| API Compatibility | Pass: no external `/api/v1` route, response, OpenAPI, or frontend contract changed. |
| Data Ownership | Pass: fixtures use UUIDs and safe snapshots; tests reject internal DB ID-like and secret-shaped payload keys. |
| Security | Pass: fixtures contain synthetic data only and no connector auth, owner password, tunnel token, local metadata, secret, token, cookie, or credential material. |
| Tests / Build | Pass: focused checks and all required local gates passed. |
| Sonar / Live Evidence | Accepted non-scope: remote Sonar, E2E, live Kubernetes, staging evidence, and full security scan were not run because this slice is contracts/fixtures/docs only and does not change runtime cross-service behavior. |
| Risk | Accepted: canonical fixtures now coexist with current runtime `contracts.Event`; producer migrations and consumer contract coverage remain future slices. |
| Diff Scope | Pass: no unrelated refactor or metadata churn detected. |

Verification evidence:

```sh
go -C backend test ./internal/contracts -count=1
# Pass: github.com/linskybing/nexuspaas/backend/internal/contracts

go -C backend test ./internal/platform -run Event -count=1
# Pass: github.com/linskybing/nexuspaas/backend/internal/platform

rg -n "event_type|schema_version|producer|aggregate_id|payload|request_id" backend/internal/contracts backend/docs/event-contracts.md problem.md
# Pass: expected envelope fields appear in contracts, fixtures, docs, and blocker tracking.

git diff --check
# Pass: no output

go -C backend test ./... -count=1
# Pass: all backend packages

go -C backend vet ./...
# Pass: no output

go -C backend build ./...
# Pass: no output

bash backend/scripts/ci-security-gate.sh quick
# Pass: Go 1.25.11 quick gate completed gofmt, vet, test, and build
```

Status: Approved

## 23. Remote CI Feedback Follow-Up

The first PR run failed `Integration and E2E` because `TestProviderConsumerContractMatrix` parses every table in `backend/docs/event-contracts.md` as an event-contract table and expects the first column to contain catalog event names. The new versioned-fixtures table initially used `Fixture` as the first column, then used backticked event names, so the parser read invalid event names.

The fix keeps the approved scope and changes only `backend/docs/event-contracts.md`: the versioned-fixtures table now uses plain event names in the first column and fixture filenames in the second column.

Additional verification after the CI feedback fix:

```sh
go -C backend test -tags e2e ./internal/e2e -run TestProviderConsumerContractMatrix -count=1 -v
# Pass: TestProviderConsumerContractMatrix

git diff --check
# Pass: no output

go -C backend test ./... -count=1
# Pass: all backend packages

go -C backend vet ./...
# Pass: no output

go -C backend build ./...
# Pass: no output

bash backend/scripts/ci-security-gate.sh quick
# Pass: Go 1.25.11 quick gate completed gofmt, vet, test, and build
```

Status: Approved
