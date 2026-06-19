# Event Consumer Contract Tests

## 1. Objective

Status: Approved

Add a small Day 16-35 roadmap slice that proves selected event-fed read-model
consumers can consume the existing canonical v1 event fixtures without relying
on shared owner stores. This slice focuses on current real consumers for
identity and tenant/project facts and keeps runtime behavior unchanged unless
fixture-backed tests reveal a direct tolerant-reader gap.

## 2. Background

Connector preflight on `feature/event-consumer-contracts` found `main` clean,
synced with `origin/main`, and then created this short feature branch from the
latest `origin/main` commit `ca387e7 contracts: add event producer coverage`.

`docs/roadmap.md` Day 16-35 requires versioned internal contracts, event schema
fixtures, and producer/consumer contract tests for the core event envelope,
starting with identity, tenant, workload, scheduler, and audit events.

`problem.md` says canonical v1 event fixtures and initial producer-specific
event tests now exist, but broader consumer contract tests remain open alongside
durable relay/publish-lag evidence, retry count, replay progress, drift
comparison, and event-fed read-model adoption.

Connector code inspection found real event-fed consumers already present:

- `integration-proxy-service` projects identity events through
  `projectIdentityAdminEvent` and `identityAdminProjection`.
- `cluster-read-service` projects identity and org-project events through
  `projectClusterReadEvent` and `clusterProjection`.

## 3. Source References

- `AGENTS.md`: implementation must follow Plan Agent -> Reviewer Agent -> Code
  Agent -> Reviewer Agent approval.
- `docs/agents/planning.md`: required 20-section plan format and small,
  verifiable scope.
- `docs/agents/workflow.md`: implementation cannot begin until this plan is
  approved; final completion requires Reviewer Agent approval.
- `docs/roadmap.md`: Day 16-35 contract/event milestone.
- `problem.md`: broader consumer contract tests remain open after producer
  coverage.
- `backend/internal/contracts/fixtures/events/v1/user-updated.json`: canonical
  identity event fixture.
- `backend/internal/contracts/fixtures/events/v1/project-updated.json`:
  canonical tenant/project event fixture.
- `backend/internal/contracts/contracts.go`: `EventEnvelope` decoder and runtime
  `contracts.Event` shape.
- `backend/internal/services/integrationproxy/helpers.go`: identity admin
  read-model projection consumer.
- `backend/internal/services/clusterread/handler.go`: cluster read-model
  projection consumer.
- `backend/internal/services/integrationproxy/handler_test.go`: existing
  event-fed identity read-model tests.
- `backend/internal/services/clusterread/handler_test.go`: existing event-fed
  cluster read-model tests.
- `microservice-architecture` skill references:
  `communication-contracts.md`, `data-consistency.md`, and
  `testing-delivery.md`, which call for additive event compatibility, idempotent
  consumers, event-fed read models, and consumer-driven contract tests.

## 4. Assumptions

- Runtime producers currently publish `contracts.Event`, while fixtures are
  stored as `EventEnvelope`. Tests can convert fixture metadata/payload into the
  equivalent runtime event shape.
- This slice should validate current real consumer behavior, not introduce a
  generic contract-test framework.
- `UserUpdated` and `ProjectUpdated` are the smallest fixtures with existing
  real read-model consumers in the inspected code. Other canonical event
  fixtures can get consumer coverage in later slices when their consumers are
  identified or implemented.
- Existing projection ID helpers are intended to be tolerant readers for both
  `id` and contract-style keys such as `user_id` and `project_id`.

## 5. Non-Goals

- Do not change external `/api/v1` request paths, response schemas, status
  codes, or auth behavior.
- Do not implement durable event relay, publish-lag evidence, retry count,
  replay progress, drift comparison, or new read-model adoption in this slice.
- Do not add a broker, database table, migration, deployment manifest, service
  config, or secret.
- Do not cover every event consumer or every mutation route in one branch.
- Do not refactor projection architecture or move shared helpers across
  services.

## 6. Current Behavior

- Canonical v1 event fixtures validate as contract artifacts.
- Producer-specific tests bind the five canonical fixtures to current producer
  helpers or route producer paths.
- Integration proxy already consumes identity events into local admin read
  models and avoids direct owner-store reads in isolated mode.
- Cluster read already consumes identity and org-project events into local read
  models and avoids direct owner-store reads in isolated mode.
- Existing consumer tests use inline event payloads. They do not yet prove that
  the canonical v1 fixture payloads are accepted by the current consumers.

## 7. Target Behavior

- Integration proxy has a fixture-backed consumer contract test proving
  `UserUpdated` projects the canonical identity payload into the local admin
  users read model.
- Cluster read has fixture-backed consumer contract tests proving:
  - `UserUpdated` projects the canonical identity payload into the local cluster
    identity users read model.
  - `ProjectUpdated` projects the canonical tenant/project payload into the
    local cluster projects read model.
- Tests assert the read model uses contract IDs from `user_id` / `project_id`,
  preserves relevant fixture payload fields, and does not populate source owner
  stores in isolated service mode.
- If a fixture-backed test exposes a direct tolerant-reader gap, production
  changes are limited to minimal additive key handling in the relevant consumer
  projection helper.

## 8. Affected Domains

- Internal event consumer contract tests.
- Integration proxy identity-admin read model.
- Cluster read identity/project read models.

No new microservice boundary is introduced. This slice strengthens existing
event-fed read models that support the planned decomposition away from shared
stores.

## 9. Affected Files

Expected changes:

- `docs/plan/2026-06-20-event-consumer-contracts.md`
- `backend/internal/services/integrationproxy/event_consumer_contract_test.go`
- `backend/internal/services/clusterread/event_consumer_contract_test.go`
- `problem.md`

Only if tests reveal a direct fixture compatibility gap:

- `backend/internal/services/integrationproxy/helpers.go`
- `backend/internal/services/clusterread/handler.go`

No `.claude/`, `.codegraph/`, `.mcp.json`, tokens, owner password files, or
local metadata should be touched.

## 10. API / Contract Changes

External `/api/v1`: no request path, method, response schema, status code, or
auth behavior changes.

Internal event contracts: no fixture schema change is planned. This slice adds
consumer-side tests for existing v1 fixtures and may add tolerant-reader support
only if an existing canonical fixture cannot be consumed.

## 11. Database / Migration Changes

None. No table, index, migration, seed data, or storage ownership change is
planned.

## 12. Configuration Changes

None. No environment variable, service URL, API key, feature flag, CI setting,
or deployment config change is planned.

## 13. Observability Changes

No metrics, logs, traces, dashboards, or alert rules are planned. Existing
projection status, lag, and dead-letter visibility remain unchanged.

## 14. Security Considerations

- Use only checked-in event fixtures and in-memory test apps.
- Do not read, print, or commit secret files.
- Do not add credentials, tokens, auth headers, cookies, or owner passwords to
  event payloads or tests.
- Preserve service isolation expectations by asserting fixture consumption writes
  to local read-model resources, not owner resources.

## 15. Implementation Steps

1. Add small package-local fixture helpers that decode event fixture JSON through
   `contracts.DecodeEventEnvelope` and convert it to a runtime
   `contracts.Event`.
2. Add an integration-proxy consumer contract test for `user-updated.json` by
   calling `projectIdentityAdminEvent` and asserting the local proxy admin user
   read model is written using the fixture `user_id`.
3. Add cluster-read consumer contract tests for `user-updated.json` and
   `project-updated.json` by calling `projectClusterReadEvent` and asserting the
   local cluster read models are written using fixture `user_id` and
   `project_id`.
4. In each test, assert representative fixture payload fields are preserved and
   source owner resources remain empty in isolated service mode.
5. If a test exposes a direct fixture compatibility gap, make the smallest
   additive consumer projection change in the service-local helper only.
6. Update `problem.md` to record initial fixture-backed consumer coverage and
   keep broader consumers, durable relay/publish-lag, retry/replay, drift
   comparison, and read-model adoption open.

## 16. Verification Plan

Focused verification:

- `go -C backend test ./internal/services/integrationproxy ./internal/services/clusterread -run 'Consumer|Projection|Contract|ReadModel' -count=1`
- `go -C backend test ./internal/contracts ./internal/services/integrationproxy ./internal/services/clusterread -run 'Event|Consumer|Projection|Contract|ReadModel' -count=1`

Required gates:

- `git diff --check`
- `go -C backend test ./... -count=1`
- `go -C backend vet ./...`
- `go -C backend build ./...`
- `bash backend/scripts/ci-security-gate.sh quick`

Additional final-review gates for AGENTS compliance:

- `bash backend/scripts/ci-security-gate.sh sonar`
- `bash backend/scripts/ci-security-gate.sh security` if local tooling/runtime
  is available; otherwise record the exact blocker in `problem.md` and PR notes.

E2E, live Kubernetes, and staging evidence are not required for this slice
because the change is limited to in-process consumer contract tests and, at
most, additive tolerant-reader key handling.

## 17. Rollback Plan

Revert the new consumer contract tests, any minimal projection helper changes,
and the `problem.md` update. Since no external API, database, config, or
deployment state changes are planned, rollback is a normal git revert.

## 18. Risks and Tradeoffs

- The slice covers current real consumers for `UserUpdated` and
  `ProjectUpdated`, not every canonical event fixture or every future consumer.
- Runtime still uses `contracts.Event`; fixture conversion in tests validates
  equivalent event metadata/payload without migrating the event bus.
- Tests may reveal that some fixture fields are preserved but not yet used by
  product workflows; that is acceptable for contract coverage but not a complete
  read-model adoption proof.
- A test-only slice improves drift detection but does not address durable relay,
  replay progress, or publish-lag evidence.

## 19. Reviewer Checklist

- Plan follows `docs/agents/planning.md` required sections.
- Scope is a single Day 16-35 consumer event contract slice.
- Tests use existing canonical v1 fixtures and real consumer projection
  functions.
- No external `/api/v1`, DB, migration, config, deployment, or observability
  change is introduced.
- Service isolation remains intact: consumers write local read models and avoid
  owner-store reads.
- Verification includes required gates, Sonar, and security status.
- `problem.md` keeps remaining GA blockers explicit.

## 20. Status

Status: Approved

Reviewer Agent approved this plan for Code Agent implementation.
