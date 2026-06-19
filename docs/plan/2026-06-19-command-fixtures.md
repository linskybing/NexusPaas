# Internal Command Contract Fixtures

## 1. Objective

Implement the next Day 16-35 Contracts And Events slice by adding versioned internal command contract fixtures for the scheduler/compute boundary. The slice should give reviewers concrete artifacts for the service-key-gated commands that scheduler-quota already sends to org-project and workload before broader command APIs or Outbox/Inbox runtime work continues.

## 2. Background

Connector preflight for this branch verified DevSpace runtime, read `AGENTS.md`, the detailed agent workflow/planning/review/coding/project-structure rules, `docs/roadmap.md`, `problem.md`, current branch/status, and latest commits on `origin/main`. `main` was clean and synced at `b0ddfa668efe020ae2556d2537cc35bed377776d` before creating `feature/command-fixtures`.

`docs/roadmap.md` says Day 16-35 must establish versioned internal contract fixtures for owner-read and command APIs, add event schema fixtures, and add Outbox/Inbox infrastructure where missing. `problem.md` now says core event envelope v1 fixtures and scheduler admission owner-read fixtures exist, but command API contracts, broader owner-read coverage, producer/consumer event coverage, and Outbox/Inbox runtime infrastructure remain open.

Current repo evidence read through the connector:

- `backend/internal/contracts` contains event envelope fixtures and owner-read fixtures, but no command fixture directory.
- `backend/internal/services/schedulerquota/plan_binding_client.go` sends service-key internal commands to org-project:
  - `PUT /internal/org-project/projects/{project_id}/plan`
  - `DELETE /internal/org-project/plans/{plan_id}/project-bindings`
- `backend/internal/services/orgproject/plan_binding_contracts.go` owns those project-plan writes and documents why scheduler-quota must not write project records directly.
- `backend/internal/services/schedulerquota/preemption_client.go` sends `POST /internal/workload/jobs/{id}/preempt` to workload.
- `backend/internal/services/schedulerquota/eviction_client.go` sends `POST /internal/workload/jobs/{id}/evict` to workload.
- `backend/internal/services/workload/handler.go` registers the workload preempt and evict internal routes.
- Existing focused tests cover owner behavior: `orgproject/plan_binding_contracts_test.go`, `workload/preemption_contracts_test.go`, and `workload/eviction_contracts_test.go`.
- Existing E2E coverage includes `TestPlanBindingOwnerContractE2E` and `TestSchedulerPreemptionEngineE2E`.

Microservice architecture skill guidance used for this plan: treat contracts as versioned artifacts, prefer additive compatibility and tolerant readers, keep data writes inside the owner service, avoid distributed transactions, require idempotent participants for cross-service workflows, and add consumer/provider contract checks before decomposition.

## 3. Source References

- `AGENTS.md`
- `docs/agents/workflow.md`
- `docs/agents/planning.md`
- `docs/agents/review-checklist.md`
- `docs/agents/coding-guidelines.md`
- `docs/agents/project-structure.md`
- `docs/roadmap.md`
- `problem.md`
- `backend/docs/event-contracts.md`
- `backend/docs/owner-read-contracts.md`
- `backend/internal/contracts/contracts.go`
- `backend/internal/contracts/event_envelope_test.go`
- `backend/internal/contracts/owner_read_fixtures_test.go`
- `backend/internal/services/schedulerquota/plan_binding_client.go`
- `backend/internal/services/schedulerquota/preemption_client.go`
- `backend/internal/services/schedulerquota/eviction_client.go`
- `backend/internal/services/orgproject/plan_binding_contracts.go`
- `backend/internal/services/orgproject/plan_binding_contracts_test.go`
- `backend/internal/services/orgproject/spec.go`
- `backend/internal/services/workload/handler.go`
- `backend/internal/services/workload/preemption_contracts_test.go`
- `backend/internal/services/workload/eviction_contracts_test.go`
- `backend/internal/services/workload/spec.go`
- `backend/internal/e2e/plan_binding_owner_contract_e2e_test.go`
- `backend/internal/e2e/scheduler_preemption_e2e_test.go`
- Microservice architecture skill references: communication contracts, data consistency, testing/delivery, and review checklists.

## 4. Assumptions

- This slice follows the merged core event fixture and scheduler admission owner-read fixture slices.
- The smallest useful command fixture boundary is scheduler/compute owner-write commands because those commands already have consumer clients, owner handlers, service-key auth, idempotency behavior, and focused tests.
- Fixtures should live beside existing contract artifacts under `backend/internal/contracts/fixtures/commands/v1/`.
- The fixture shape should describe command metadata, route, owner service, consumer service, auth, idempotency semantics, path parameters, request fields, success/error statuses, emitted events, and representative synthetic payloads.
- This slice should not change runtime behavior; existing service tests and E2E coverage already prove the owner handlers and callers work.

## 5. Non-Goals

- Do not change external `/api/v1` routes, response envelopes, frontend behavior, or OpenAPI output.
- Do not change runtime command handlers, scheduler clients, service URL resolution, service-key behavior, stores, events, or deployable units.
- Do not add command fixtures for every internal command in the backend; limit this PR to scheduler/compute boundary commands.
- Do not add owner-read fixtures, new event fixtures, producer-specific event tests, or consumer-driven event tests in this slice.
- Do not add Outbox/Inbox tables, migrations, Redis/broker changes, relay workers, lag metrics, retry/dead-letter runtime, or deployment manifests.
- Do not add dependencies, code generators, JSON schema tooling, or new shared SDK abstractions.

## 6. Current Behavior

Runtime command behavior exists and has focused tests, but command contracts are not represented as versioned JSON artifacts. Reviewers can inspect Go constants and tests, but there is no canonical artifact that records method/path, owner/consumer, service-key requirement, idempotency behavior, request/response fields, and failure statuses for scheduler-quota commands crossing into org-project and workload.

## 7. Target Behavior

The repository should include:

- four v1 command JSON fixtures:
  - `org-project-bind-project-plan.json`
  - `org-project-clear-plan-bindings.json`
  - `workload-preempt-job.json`
  - `workload-evict-job.json`
- a contracts package test that loads every command fixture, asserts the exact fixture list, validates `schema_version == 1`, service-key metadata, method/path shape, path parameter coverage, required request fields, success/error status metadata, idempotency semantics, emitted event metadata, synthetic payloads, additive compatibility, and recursively rejects secret/internal-ID-shaped fields;
- focused service tests that bind fixture route metadata to the existing owner route constants/registrations and consumer client path templates, so fixture drift fails locally;
- backend docs that catalog command fixtures and compatibility rules;
- `problem.md` updated to show scheduler/compute command fixtures now exist while broader command coverage, producer/consumer event tests, and Outbox/Inbox runtime remain open.

## 8. Affected Domains

- Internal command contract artifacts.
- Scheduler-quota to org-project project-plan owner-write boundary.
- Scheduler-quota to workload preemption/eviction owner-write boundary.
- GA roadmap blocker tracking.
- Backend contract documentation.

## 9. Affected Files

- `docs/plan/2026-06-19-command-fixtures.md`
- `backend/internal/contracts/fixtures/commands/v1/org-project-bind-project-plan.json`
- `backend/internal/contracts/fixtures/commands/v1/org-project-clear-plan-bindings.json`
- `backend/internal/contracts/fixtures/commands/v1/workload-preempt-job.json`
- `backend/internal/contracts/fixtures/commands/v1/workload-evict-job.json`
- `backend/internal/contracts/command_fixtures_test.go`
- `backend/internal/services/orgproject/command_fixtures_test.go`
- `backend/internal/services/workload/command_fixtures_test.go`
- `backend/internal/services/schedulerquota/command_fixtures_test.go`
- `backend/docs/internal-command-contracts.md`
- `problem.md`

## 10. API / Contract Changes

No external API changes. This slice adds versioned internal command contract artifacts for already-existing service-key-gated internal HTTP commands. The artifacts document current internal behavior and do not change runtime request/response behavior.

## 11. Database / Migration Changes

No database or migration changes. Fixtures use synthetic IDs only. Tests should reject secret-looking and internal database ID-like fixture keys recursively.

## 12. Configuration Changes

No configuration changes.

## 13. Observability Changes

No runtime observability changes. Fixture metadata should record expected emitted domain events where they already exist, but this slice does not add metrics, logs, traces, or event publishing behavior.

## 14. Security Considerations

Fixtures must not contain secrets, tokens, passwords, cookies, credentials, owner passwords, connector auth, tunnel tokens, local metadata, or private tenant data. Every command fixture must state `auth: service_key` and `service_key_required: true`. The tests should reject secret-looking and internal-ID-looking keys in request/response examples. Runtime service-key behavior is not changed.

## 15. Implementation Steps

1. Add this plan with `Status: Draft`.
2. Run Reviewer Agent plan review and revise until approved.
3. Add four v1 command fixture JSON files under `backend/internal/contracts/fixtures/commands/v1/`.
4. Add `backend/internal/contracts/command_fixtures_test.go` to validate fixture filenames, metadata, paths, request/response examples, statuses, idempotency, emitted events, additive compatibility, and forbidden field names.
5. Add focused service tests:
   - org-project fixture route metadata matches `pathBindProjectPlan` and `pathClearPlanBindings` and owner behavior tests remain aligned.
   - workload fixture route metadata matches the preempt/evict registered internal routes.
   - scheduler-quota fixture route metadata matches `bindProjectPlanPathTemplate`, `clearPlanBindingsPathTemplate`, `workloadPreemptJobPathTemplate`, and `workloadEvictJobPathTemplate`.
6. Add `backend/docs/internal-command-contracts.md` documenting the fixture catalog and compatibility rules.
7. Update `problem.md` to record completed scheduler/compute command fixture coverage and remaining Day 16-35 blockers.
8. Run focused checks, required gates, and Reviewer Agent final approval.

## 16. Verification Plan

Focused checks:

```sh
go -C backend test ./internal/contracts -run Command -count=1
go -C backend test ./internal/services/orgproject -run 'Command|PlanBinding' -count=1
go -C backend test ./internal/services/workload -run 'Command|Preempt|Evict' -count=1
go -C backend test ./internal/services/schedulerquota -run 'Command|PlanBinding|Preempt|Evict' -count=1
go -C backend test -tags e2e ./internal/e2e -run 'TestPlanBindingOwnerContractE2E|TestSchedulerPreemptionEngineE2E' -count=1 -v
rg -n "commands/v1|CommandFixture|org-project-bind-project-plan|workload-preempt-job|service_key_required" backend/internal/contracts backend/internal/services backend/docs problem.md
```

Required gates:

```sh
git diff --check
go -C backend test ./... -count=1
go -C backend vet ./...
go -C backend build ./...
bash backend/scripts/ci-security-gate.sh quick
bash backend/scripts/ci-security-gate.sh sonar
```

SonarScanner Quality Gate must be run for this slice. If Sonar credentials or a reachable endpoint are unavailable, record the explicit blocker or documented skip-policy result in the plan final approval section, PR body, and `problem.md` as appropriate. Live Kubernetes, staging evidence, and full security scan are not required unless implementation unexpectedly changes runtime behavior or deployment artifacts. If any such gate becomes relevant, run it or record the blocker explicitly.

## 17. Rollback Plan

Revert the command fixture files, fixture validation tests, focused service fixture tests, docs update, plan file, and `problem.md` update. No runtime API, database, deployment, or configuration rollback is required.

## 18. Risks and Tradeoffs

- Fixture coverage is intentionally limited to scheduler/compute command boundaries, not every internal command. This keeps the PR reviewable and leaves broader command coverage for later slices.
- The fixture shape is a Go-tested JSON artifact instead of JSON Schema. This follows the existing event and owner-read fixture pattern and avoids adding toolchain risk.
- Route drift is guarded through focused package tests rather than a shared command registry. This avoids adding runtime abstractions only for test fixtures.
- Emitted event metadata in fixtures documents current owner behavior but does not add producer/consumer event compatibility tests; those remain a separate blocker.

## 19. Reviewer Checklist

- Requirement fit: implements the Day 16-35 command fixture requirement for a concrete scheduler/compute boundary.
- Scope control: does not change runtime handlers, clients, DB, deployment, config, external `/api/v1`, or unrelated files.
- Architecture: reinforces owner-write discipline before further service split work.
- API contract: preserves external `/api/v1` compatibility and documents internal command contracts only.
- Data ownership: fixtures keep project writes inside org-project and job writes inside workload.
- Config: no config changes.
- Observability: no runtime metrics/log/trace changes.
- Security: fixtures are synthetic and service-key-gated; tests reject secret/internal-ID-shaped keys.
- Testing: focused command fixture tests, owner/consumer route drift tests, targeted E2E, and required gates are explicit.
- Rollback: revert-only rollback is realistic.

## 20. Status

Status: Approved

## 21. Reviewer Plan Approval

| Category | Result |
| --- | --- |
| Requirement Fit | Pass: directly implements the Day 16-35 command fixture requirement for a concrete scheduler/compute boundary. |
| Scope Control / Non-Goals | Pass: limited to fixtures, tests, docs, plan, and `problem.md`; runtime handlers, DB, config, deployment, and external `/api/v1` stay unchanged. |
| Architecture / Boundary | Pass: reinforces owner-write discipline for org-project-owned project bindings and workload-owned job state. |
| API / DB / Config | Pass: documents internal contracts only; no external API, migration, or config change. |
| Observability / Security | Pass: no runtime observability change; fixtures are synthetic and service-key-gated. |
| Testing / Gates | Pass: focused command tests, route drift tests, targeted E2E, required Go gates, quick gate, and Sonar gate are explicit. |
| Rollback / Simplicity | Pass: revert-only rollback and no new shared runtime abstraction. |

Status: Approved

## 22. Final Verification Evidence

| Command | Result | Notes |
| --- | --- | --- |
| `go -C backend test ./internal/contracts -run Command -count=1` | Pass | Command fixture catalog, additive compatibility, and forbidden-field validation. |
| `go -C backend test ./internal/services/orgproject -run 'Command|PlanBinding' -count=1` | Pass | Org-project command fixtures match owner route constants and plan-binding behavior. |
| `go -C backend test ./internal/services/workload -run 'Command|Preempt|Evict' -count=1` | Pass | Workload command fixtures match preempt/evict route registrations and existing command behavior. |
| `go -C backend test ./internal/services/schedulerquota -run 'Command|PlanBinding|Preempt|Evict' -count=1` | Pass | Scheduler-quota client path templates and request structs match command fixture metadata. |
| `go -C backend test ./internal/contracts ./internal/platform ./internal/services/orgproject ./internal/services/workload ./internal/services/schedulerquota -run 'Command|OwnerRead|RemoteServiceReaderConsumesOwnerReadFixtures|PlanBinding|Preempt|Evict' -count=1` | Pass | Re-run after Sonar complexity refactor. |
| `TEST_MINIO_PORT=19100 TEST_MINIO_CONSOLE_PORT=19101 bash backend/scripts/ci-security-gate.sh docker` | Pass | Docker-backed integration/E2E/runtime smoke/collaboration smoke passed; evidence in local quality-gate artifact `local-76394`. |
| Docker gate full E2E log check | Pass / Skip | `TestPlanBindingOwnerContractE2E` passed; live `TestSchedulerPreemptionEngineE2E` skipped because `TEST_LIVE_K8S_PREEMPTION=1` was not enabled. |
| `git diff --check` | Pass | Connector run produced no whitespace findings. |
| `go -C backend test ./... -count=1` | Pass | Full backend package test suite passed after final refactor. |
| `go -C backend vet ./...` | Pass | Static Go vet check passed. |
| `go -C backend build ./...` | Pass | Full backend build passed. |
| `bash backend/scripts/ci-security-gate.sh quick` | Pass | Gofmt, vet, full tests, and build passed through the gate wrapper. |
| `bash backend/scripts/ci-security-gate.sh sonar` | Pass | Sonar Quality Gate passed; connector API confirmed `new-code-issues=0`. |

Sonar initially reported six `go:S3776` cognitive-complexity findings in the command fixture tests and owner-read fixture tests still inside the Sonar new-code window. The final implementation fixes them by extracting validation/assertion helpers without changing runtime behavior, fixture payloads, database schema, deployment manifests, config, or external `/api/v1` contracts.

## 23. Reviewer Final Approval

Reviewer Agent status: FINAL APPROVED.

Summary: the change fits `docs/roadmap.md` Day 16-35 and the approved plan, adds v1 internal command fixtures for scheduler-quota owner-write commands into org-project and workload, preserves external `/api/v1`, runtime handlers, DB, config, and deployments, keeps fixtures synthetic and service-key-gated, and records sufficient focused, full, Docker, quick, and Sonar evidence. Remaining broader command/event/Outbox/staging risks stay tracked in `problem.md`, and rollback remains revert-only.
