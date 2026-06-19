# Request Notification Project Consumer Contract

## 1. Objective

Add fixture-backed consumer contract coverage for the request-notification project access projection consuming the canonical v1 `ProjectUpdated` event fixture.

This slice advances the Day 16-35 contract/event roadmap and the Day 36-55 read-model migration roadmap by proving that request-notification can consume the org-project boundary event into its own local project access read model without writing to the org-project owner store.

## 2. Background

`docs/roadmap.md` requires versioned internal event fixtures, producer/consumer contract tests, and read-model migration before GA decomposition. `problem.md` currently records that fixture-backed consumer tests exist for integration-proxy, cluster-read, gpuusage, and authorization-policy, while remaining consumer event paths and broader event-fed read-model adoption are still open.

The request-notification service already projects org-project events through `projectProjectAccessEvent` and stores local access data in `request-notification-service:project_access_projects`. Existing request-notification tests use synthetic events and records, so this service boundary is not yet bound to the canonical `backend/internal/contracts/fixtures/events/v1/project-updated.json` artifact.

## 3. Source References

- `AGENTS.md`: requires Plan Agent, Reviewer Agent approval, Code Agent implementation, and Reviewer final approval.
- `docs/agents/workflow.md`: requires one feature branch per goal, approved plan before implementation, PR with what/why/how, and squash merge.
- `docs/agents/planning.md`: requires small scope, affected files, verification commands, rollback, and explicit non-goals.
- `docs/agents/review-checklist.md`: requires Reviewer verification of plan fit, implementation alignment, SOLID, 12-Factor, tests/build, risks, and diff scope.
- `docs/agents/coding-guidelines.md`: requires surgical changes, no unrelated refactors, no weakened tests, and contract tests for service boundary changes.
- `docs/agents/project-structure.md`: requires each service to own its code, data, config, tests, deployment, observability, and rollback strategy.
- `docs/roadmap.md`: Day 16-35 requires event schema fixtures and producer/consumer contract tests; Day 36-55 requires owner-read/read-model migration for high-risk shared-store reads.
- `problem.md`: remaining GA blockers include broader owner-read/command coverage, remaining consumer event paths, durable relay/read-model slices, and event-fed read-model adoption.
- `backend/internal/services/requestnotification/helpers.go`: `projectProjectAccessEvent` handles `ProjectUpdated` through `projectAccessProjection` and `upsertProjectAccessReadModel`.
- `backend/internal/services/requestnotification/project_access_repository.go`: `UpsertProject` persists local read models keyed by `project_id` when no explicit `id` exists.
- `backend/internal/services/requestnotification/handler.go`: defines `projectAccessProjects` and `orgProjectsResource` resource names.
- `backend/internal/contracts/fixtures/events/v1/project-updated.json`: canonical v1 `ProjectUpdated` event fixture produced by org-project-service.
- `backend/internal/services/clusterread/event_consumer_contract_test.go`: existing fixture-backed consumer contract test pattern for `ProjectUpdated`.

## 4. Assumptions

- The canonical `ProjectUpdated` fixture remains the source artifact for this slice; no fixture schema changes are needed.
- `projectProjectAccessEvent` is the correct in-process projection entrypoint to test the consumer mapping without starting HTTP routing or background workers.
- The isolated request-notification app should store projected data only in request-notification read-model resources and should not populate `org-project-service:projects`.
- No external `/api/v1` behavior should change.

## 5. Non-Goals

- Do not change event schema fixtures.
- Do not add new runtime projection behavior.
- Do not change request-notification HTTP APIs.
- Do not change database schema, migrations, deployment manifests, service config, or observability metrics.
- Do not add membership or group membership fixture-backed coverage in this slice.
- Do not retire shared physical PostgreSQL or implement durable relay/drift comparison in this slice.
- Do not refactor request-notification repositories, handlers, or helper structure.

## 6. Current Behavior

Request-notification can project project access records from events into local read models, but the current tests use synthetic event payloads. The service does not yet have a consumer contract test that decodes the canonical v1 `ProjectUpdated` event fixture and proves the local read model preserves important org-project payload fields.

## 7. Target Behavior

A new focused request-notification consumer contract test decodes `project-updated.json`, converts the envelope into a `contracts.Event`, runs `projectProjectAccessEvent`, and verifies:

- `request-notification-service:project_access_projects` receives a record keyed by fixture `project_id`.
- The stored record ID and `Data["id"]` match fixture `project_id`.
- Representative fixture fields are preserved: `project_id`, `org_id`, `slug`, `quota_plan_id`, and `member_count`.
- The isolated consumer does not write to `org-project-service:projects`.

## 8. Affected Domains

- Request-notification service consumer contract tests.
- Org-project to request-notification event/read-model boundary evidence.
- GA roadmap blocker tracking in `problem.md`.

## 9. Affected Files

Expected changed files:

- `docs/plan/2026-06-20-request-project-contract.md`
- `backend/internal/services/requestnotification/event_consumer_contract_test.go`
- `problem.md`

## 10. API / Contract Changes

No external API changes. No event fixture schema changes.

This adds consumer contract coverage against the existing v1 `ProjectUpdated` fixture and existing in-process request-notification projection contract.

## 11. Database / Migration Changes

None.

The test uses the existing in-memory `platform.RecordStore` through `platform.NewApp`.

## 12. Configuration Changes

None.

The test uses `platform.Config{ServiceName: serviceName}` to exercise isolated request-notification behavior.

## 13. Observability Changes

None.

This is test and documentation evidence only. Existing projection metrics and runtime observability are unchanged.

## 14. Security Considerations

- No secrets, tokens, owner passwords, local metadata, `.claude/`, `.codegraph/`, or `.mcp.json` files are read, written, staged, or committed.
- The test asserts isolated consumer behavior by checking that the org-project owner resource is not populated.
- No auth or authorization runtime behavior changes.

## 15. Implementation Steps

1. Add `backend/internal/services/requestnotification/event_consumer_contract_test.go` following the existing consumer contract style used by cluster-read.
2. Decode `backend/internal/contracts/fixtures/events/v1/project-updated.json` with `contracts.DecodeEventEnvelope`.
3. Convert the fixture into `contracts.Event` with cloned payload data.
4. Run `projectProjectAccessEvent` against a `platform.NewApp(platform.Config{ServiceName: serviceName})` and a minimal `httptest` request.
5. Assert the local project access read model is keyed by fixture `project_id` and preserves representative payload fields.
6. Assert the isolated app did not write to `orgProjectsResource`.
7. Update `problem.md` to record this slice's evidence, remaining blockers, and next steps.
8. Run gofmt and the verification plan.

## 16. Verification Plan

Focused verification before final review:

- `gofmt -w backend/internal/services/requestnotification/event_consumer_contract_test.go`
- `go -C backend test ./internal/services/requestnotification -run 'Consumer|Projection|Contract|ReadModel|ProjectAccess' -count=1`
- `go -C backend test ./internal/contracts ./internal/services/requestnotification -run 'Event|Consumer|Projection|Contract|ReadModel|ProjectAccess' -count=1`

Required local gates before PR:

- `git diff --check`
- `go -C backend test ./... -count=1`
- `go -C backend vet ./...`
- `go -C backend build ./...`
- `bash backend/scripts/ci-security-gate.sh quick`

Additional security and Sonar evidence when locally available:

- `bash backend/scripts/ci-security-gate.sh security`
- `bash backend/scripts/ci-security-gate.sh sonar`

E2E, live Kubernetes, and staging evidence are not required for this slice because it is limited to in-process consumer contract coverage and blocker tracking.

## 17. Rollback Plan

Revert the feature commit or remove the new request-notification consumer contract test plus the associated `problem.md` update. Runtime behavior is unchanged, so rollback does not require data migration, config rollback, or deployment changes.

## 18. Risks and Tradeoffs

- This covers only the `ProjectUpdated` project access path, not project member or group membership events. That keeps the slice small; those paths remain explicit follow-up work.
- The test calls the in-process projection function instead of running a full outbox/inbox worker. Existing platform projection tests cover worker mechanics; this slice validates payload compatibility at the service boundary.
- The test preserves map-based payload assertions because existing contract fixtures decode into `map[string]any` and the local repository currently stores record data as maps.

## 19. Reviewer Checklist

- Requirement fit: plan advances remaining consumer event path coverage from `problem.md` and Day 16-35/36-55 roadmap items.
- Scope control: only plan, one focused test file, and `problem.md` update are expected.
- API compatibility: no `/api/v1` changes.
- Data ownership: projected data stays in request-notification local read-model resource; owner store remains untouched.
- Config/security: no config or secret changes.
- Testing: focused package tests plus required gates are defined.
- Rollback: revert the commit without runtime migration.

## 20. Status

Status: Approved
