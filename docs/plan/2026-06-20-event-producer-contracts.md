# Event Producer Contract Tests

## 1. Objective

Status: Approved

Add a small Day 16-35 roadmap slice that proves the current runtime event producers for the canonical v1 event fixtures emit fixture-compatible event metadata and payload keys, while preserving external `/api/v1` behavior and existing event consumer compatibility.

## 2. Background

Connector preflight on `feature/event-producer-contracts` found the branch clean except for this plan file, based on `origin/main` at `33ac6ff platform: expose outbox inbox runtime evidence`.

`docs/roadmap.md` Day 16-35 requires versioned event schema fixtures and producer/consumer contract tests for the core event envelope, starting with identity, tenant, workload, scheduler, and audit events.

`problem.md` says the previous Day 16-35 slices added core event envelope fixtures, owner-read fixtures, command fixtures, and Outbox/Inbox visibility. It still lists producer-specific event tests, broader consumer contract tests, durable relay/publish-lag evidence, replay progress, and read-model adoption as open.

The first Reviewer Agent pass requested this plan revision because the previous version did not follow `docs/agents/planning.md`, used generic platform publisher coverage where real route producers were needed, and did not explicitly address Sonar status.

## 3. Source References

- `AGENTS.md`: implementation must follow Plan Agent -> Reviewer Agent -> Code Agent -> Reviewer Agent approval.
- `docs/agents/planning.md`: required 20-section plan format and small, verifiable scope.
- `docs/agents/workflow.md`: implementation cannot begin until this plan is approved; final completion requires Reviewer Agent approval.
- `docs/roadmap.md`: Day 16-35 contract/event milestone.
- `problem.md`: producer-specific event tests remain open after the existing fixture and Outbox/Inbox slices.
- `backend/internal/contracts/fixtures/events/v1/*.json`: canonical v1 fixtures for `UserUpdated`, `ProjectUpdated`, `JobSubmitted`, `QuotaReserved`, and `AuditEvent`.
- `backend/internal/contracts/event_envelope_test.go`: fixture validation and fixture helper patterns.
- `backend/internal/services/identity/helpers.go`: identity `publish` helper emits `contracts.Event` from `identity-service`.
- `backend/internal/services/orgproject/group_helpers.go`: org-project `eventFor` helper emits `contracts.Event` from `org-project-service`.
- `backend/internal/services/workload/handler.go`: workload `publish` helper emits `contracts.Event` from `workload-service`.
- `backend/internal/platform/reservation.go`: real quota reservation producer path currently emits `QuotaReserved` with minimal payload.
- `backend/internal/platform/app.go`: real route audit producer path currently emits `AuditEvent` with legacy audit keys.

## 4. Assumptions

- Current runtime producers emit legacy `contracts.Event` values, while fixtures define canonical envelope fields. This slice should compare equivalent metadata fields instead of migrating the whole runtime event bus to `EventEnvelope`.
- Existing event consumers may depend on current keys such as `reservation_id`, `state`, `resource`, and `success`; this slice may add keys but must not remove or rename existing keys.
- Fixture payload values are stable review artifacts. Tests can use fixture payloads as sample inputs for helpers, but route-level tests should assert fixture-required keys are present and semantically mapped rather than require every literal fixture value when runtime-generated IDs and paths differ.
- No new service boundary is being introduced.

## 5. Non-Goals

- Do not change external `/api/v1` request or response payloads.
- Do not implement broader consumer contract tests in this slice.
- Do not implement durable relay, retry count, replay progress, drift comparison, or event-fed read-model adoption.
- Do not refactor routing, storage, or event bus architecture.
- Do not add new infrastructure, config, database tables, migrations, deployment files, or secrets.

## 6. Current Behavior

- Event envelope fixtures exist and validate as standalone artifacts.
- Service-level producer helpers emit metadata such as event id, name, source, trace id, schema version, idempotency key, and data, but there are no fixture-backed producer contract tests.
- `handleReservation` emits `QuotaReserved` with `reservation_id` and `state`, but it does not preserve fixture-relevant reservation context such as `project_id`, `job_id`, `plan_id`, `reserved`, or `expires_at` in the event payload.
- `publishAudit` emits legacy audit keys such as `user_id`, `action`, `resource`, `success`, `source_ip`, `project_id`, and `group_id`, but it does not expose fixture-style audit keys such as `audit_event_id`, `actor_user_id`, `resource_type`, `resource_id`, `outcome`, `source_service`, and `request_path`.

## 7. Target Behavior

- Producer contract tests bind the five existing v1 fixtures to real producer helpers or route producer paths:
  - `UserUpdated` from identity `publish`.
  - `ProjectUpdated` from org-project `eventFor`.
  - `JobSubmitted` from workload `publish`.
  - `QuotaReserved` from platform `handleReservation` on the scheduler-quota reservation route.
  - `AuditEvent` from platform `publishAudit` on a scheduler-quota route context.
- Tests fail if event name/source/schema metadata drift away from fixture expectations.
- Tests fail if fixture-required payload keys are not present in produced event data when the producer receives fixture-compatible input.
- Runtime event payload changes, if needed, are additive only and keep existing consumer keys intact.

## 8. Affected Domains

- Internal event contracts.
- Scheduler-quota reservation event emission.
- Platform audit event emission.
- Identity, org-project, and workload producer helper tests.

No new microservice boundary is introduced. The slice strengthens the existing service-owned producer contract evidence for the decomposition roadmap.

## 9. Affected Files

Expected changes:

- `docs/plan/2026-06-20-event-producer-contracts.md`
- `backend/internal/services/identity/event_producer_contract_test.go`
- `backend/internal/services/orgproject/event_producer_contract_test.go`
- `backend/internal/services/workload/event_producer_contract_test.go`
- `backend/internal/platform/event_producer_contract_test.go`
- `backend/internal/platform/reservation.go`
- `backend/internal/platform/app.go`
- `problem.md`

No `.claude/`, `.codegraph/`, `.mcp.json`, tokens, owner password files, or local metadata should be touched.

## 10. API / Contract Changes

External `/api/v1`: no request path, method, response schema, status code, or auth behavior changes.

Internal event contract: additive payload coverage only.

- `QuotaReserved` keeps existing `reservation_id` and `state`, and may add fixture-compatible keys from the reservation request/record such as `project_id`, `job_id`, `plan_id`, `reserved`, and `expires_at`.
- `AuditEvent` keeps existing `user_id`, `action`, `resource`, `success`, `source_ip`, `project_id`, and `group_id`, and may add fixture-compatible keys such as `audit_event_id`, `actor_user_id`, `resource_type`, `resource_id`, `outcome`, `source_service`, and `request_path`.
- `UserUpdated`, `ProjectUpdated`, and `JobSubmitted` producer helper tests should not require runtime code changes unless tests reveal a direct metadata/payload drift.

## 11. Database / Migration Changes

None. No table, index, migration, seed data, or storage ownership change is planned.

## 12. Configuration Changes

None. No environment variable, service URL, API key, feature flag, CI config, or deployment config change is planned.

## 13. Observability Changes

No metrics, logs, traces, dashboards, or alert rules are planned. This slice improves event payload observability by adding contract-tested context to scheduler-quota reservation and audit events only.

## 14. Security Considerations

- Do not include secrets, tokens, cookies, passwords, API keys, owner credentials, or auth headers in event payloads or test fixtures.
- Preserve existing audit fields while adding normalized audit context for contract consumers.
- Use existing fixture payload validation to avoid forbidden payload keys.
- Avoid reading or printing secret files.

## 15. Implementation Steps

1. Add small fixture-loading helpers in producer test files or reuse existing package-local fixture decoding patterns without moving production code.
2. Add identity producer contract test for `publish(app, req, "UserUpdated", fixture.Payload)` and assert name, source, schema version, event id, trace id, occurred timestamp, idempotency key, and fixture payload key preservation.
3. Add org-project producer contract test for `eventFor(req, "ProjectUpdated", fixture.Payload)` with equivalent assertions.
4. Add workload producer contract test for `publish(app, req, "JobSubmitted", "submitted", fixture.Payload)` with equivalent assertions, allowing workload's additive `action` field.
5. Add platform route producer contract tests for `QuotaReserved` by invoking `handleReservation` with fixture-compatible request data and asserting the emitted event keeps existing keys plus fixture-required reservation context.
6. Add platform audit producer contract test by invoking `publishAudit` with a scheduler-quota route context and asserting legacy keys plus fixture-style audit keys are present.
7. If tests fail because `QuotaReserved` or `AuditEvent` lacks fixture-required context, make minimal additive changes in `backend/internal/platform/reservation.go` and/or `backend/internal/platform/app.go` only.
8. Update `problem.md` to mark initial producer-specific event contract coverage as added for the five canonical v1 fixtures, and keep broader route-level producer coverage, consumer contract coverage, durable relay/publish-lag, retry/replay, drift comparison, and read-model adoption open.

## 16. Verification Plan

Focused verification:

- `go -C backend test ./internal/contracts ./internal/platform ./internal/services/identity ./internal/services/orgproject ./internal/services/workload -run 'Event|Producer|Contract|Reservation|Audit' -count=1`

Required gates:

- `git diff --check`
- `go -C backend test ./... -count=1`
- `go -C backend vet ./...`
- `go -C backend build ./...`
- `bash backend/scripts/ci-security-gate.sh quick`

Additional final-review gates for AGENTS compliance:

- `bash backend/scripts/ci-security-gate.sh sonar`
- `bash backend/scripts/ci-security-gate.sh security` if local tooling/runtime is available; otherwise record the exact blocker in `problem.md` and final PR notes.

E2E, live Kubernetes, and staging evidence are not required for this slice because the change is limited to in-process producer contract tests and additive event payload context.

## 17. Rollback Plan

Revert the new producer contract tests, the additive event payload changes in platform files, and the `problem.md` update. Since no external API, database, config, or deployment state changes are planned, rollback is a normal git revert.

## 18. Risks and Tradeoffs

- The tests cover first canonical producers, not every mutation route that can emit the same event names. Broader route-level producer coverage remains open.
- Additive event keys may expose new data to existing internal event consumers. The chosen keys must be non-secret operational context already present in request or route data.
- Current runtime still uses `contracts.Event`, not `EventEnvelope`; this slice deliberately validates equivalent metadata rather than performing an event bus migration.
- Running Sonar/security gates can depend on local services and tooling. Any unavailable gate must be documented with exact command output and residual risk.

## 19. Reviewer Checklist

- Plan follows `docs/agents/planning.md` required sections.
- Scope is a single Day 16-35 event producer contract slice.
- No external `/api/v1` compatibility change is introduced.
- DB, migration, config, deployment, and observability boundaries are explicitly unchanged.
- Runtime changes are additive event payload changes only.
- Tests exercise real producer helpers or route producer paths, not only generic publisher pass-through.
- Required gates and Sonar status are included in verification.
- `problem.md` will retain broader remaining blockers.

## 20. Status

Status: Approved

Reviewer Agent approved this plan for Code Agent implementation.
