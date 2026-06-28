# Authorization Policy PolicyChanged Event Fixture

## 1. Objective

Add local/static event-envelope fixture coverage for the authorization-policy
`PolicyChanged` event, plus one service-local producer contract test that proves
`policyChangedEvent()` emits metadata and payload compatible with that fixture.

## 2. Background

`authorizationpolicy.Spec()` declares both `PolicyChanged` and
`ProxyPolicyChanged`. Current event-envelope coverage includes
`proxy-policy-changed.json`, but not `policy-changed.json`. The existing
batch-permissions API fixture emits `PolicyChanged` for the
`project_member`/`add` happy path, so this slice closes the matching event
fixture gap without changing runtime behavior.

## 3. Source References

- `backend/internal/services/authorizationpolicy/helpers.go`
  - `authorizationPolicyEvent()` sets event name, source, trace/idempotency,
    schema version, and payload action.
  - `policyChangedEvent()` wraps `authorizationPolicyEvent()` with
    `PolicyChanged`.
- `backend/internal/contracts/event_envelope_test.go`
  - `TestEventEnvelopeFixturesAreValidV1` has the explicit fixture filename list
    and event type to producer map.
- `backend/internal/contracts/fixtures/events/v1/proxy-policy-changed.json`
  - Existing authorization-policy event fixture shape.
- `backend/internal/contracts/fixtures/api/v1/authorization-policy-batch-permissions.json`
  - Existing API fixture that emits `PolicyChanged` for
    `project_member`/`add`.
- Existing producer contract test patterns:
  - `backend/internal/services/orgproject/event_producer_contract_test.go`
  - `backend/internal/services/identity/event_producer_contract_test.go`
  - `backend/internal/services/workload/event_producer_contract_test.go`

## 4. Assumptions

- The current branch is `storage-data-path`.
- This is a fixture/test/docs slice only.
- `authorization-policy-service` remains the producer name for
  `PolicyChanged`.
- The fixture can use stable synthetic IDs and timestamps because it is contract
  evidence, not live bus evidence.

## 5. Non-Goals

- No handler, repository, runtime, projection, or event bus changes.
- No `ProxyPolicyChanged` fixture or producer changes.
- No OpenAPI or API fixture changes.
- No kind, e2e, live event bus, or live mutation evidence.
- No DATA GA, Full GA, V1 external launch, or first-version readiness claim.

## 6. Current Behavior

- `PolicyChanged` is declared and produced by service code.
- `authorization-policy-batch-permissions.json` declares that the route emits
  `PolicyChanged`.
- Event-envelope fixture validation only knows about `ProxyPolicyChanged` for
  authorization-policy.
- There is no authorization-policy producer contract test for
  `policyChangedEvent()`.
- Acceptance docs mention `PolicyChanged` linkage but not the matching local
  event-envelope fixture evidence.

## 7. Target Behavior

- `policy-changed.json` exists under
  `backend/internal/contracts/fixtures/events/v1/`.
- `TestEventEnvelopeFixturesAreValidV1` requires that filename and maps
  `PolicyChanged` to `authorization-policy-service`.
- A service-local test verifies `policyChangedEvent()` against the fixture
  metadata and payload.
- `docs/acceptance/gap-analysis.md` and `problem.md` record local/static
  `PolicyChanged` event fixture evidence while preserving live/e2e/GA caveats.

## 8. Affected Domains

- Authorization-policy contract fixture coverage.
- Contract test coverage for event-envelope fixtures.
- Authorization-policy service-local producer contract tests.
- Acceptance/gap documentation.

No microservice boundary changes are introduced.

## 9. Affected Files

- Add:
  - `backend/internal/contracts/fixtures/events/v1/policy-changed.json`
  - `backend/internal/services/authorizationpolicy/event_producer_contract_test.go`
- Update:
  - `backend/internal/contracts/event_envelope_test.go`
  - `docs/acceptance/gap-analysis.md`
  - `problem.md`

## 10. API / Contract Changes

Add one event-envelope v1 fixture:

- `event_type`: `PolicyChanged`
- `producer`: `authorization-policy-service`
- `schema_version`: `1`
- stable metadata:
  - `event_id`: valid stable UUID
  - `occurred_at`: stable timestamp
  - `trace_id`: `trace-policy-changed-v1`
  - `request_id`: `req-policy-changed-v1`
  - `aggregate_id`: `project-ga-alpha`
- payload:
  - `action`: `batch_permissions_processed`
  - `operation`: object with `type: project_member`, `action: add`,
    `project_id: project-ga-alpha`, `user_id: user-ga-ada`, `role: viewer`
  - `operations`: array containing the same operation object

No REST API contract or OpenAPI change is in scope.

## 11. Database / Migration Changes

None.

## 12. Configuration Changes

None.

## 13. Observability Changes

None. This does not add metrics, logs, traces, outbox behavior, relay behavior,
or live event bus evidence.

## 14. Security Considerations

- Keep the fixture payload limited to public/synthetic IDs and role vocabulary.
- Do not add secrets, tokens, credentials, database IDs, internal IDs, or other
  fields rejected by event-envelope payload validation.
- The producer test should use synthetic headers only.

## 15. Implementation Steps

1. Add `backend/internal/contracts/fixtures/events/v1/policy-changed.json`.
   - Match the event-envelope v1 shape used by
     `proxy-policy-changed.json`.
   - Use `PolicyChanged`, `authorization-policy-service`, schema v1, stable
     trace/request/aggregate IDs, and the batch-permissions payload described in
     section 10.
2. Update `backend/internal/contracts/event_envelope_test.go`.
   - Insert `policy-changed.json` into the sorted expected fixture filename
     list.
   - Add `PolicyChanged: authorization-policy-service` to `wantTypes`.
3. Add
   `backend/internal/services/authorizationpolicy/event_producer_contract_test.go`.
   - Reuse the existing local producer-test pattern from org-project,
     identity, or workload.
   - Read `policy-changed.json` from the shared event fixture directory.
   - Create an `httptest` request, set `X-Trace-ID` from the fixture, and set a
     stable `Idempotency-Key`.
   - Call `policyChangedEvent(req, "batch_permissions_processed", data)` where
     `data` contains the fixture payload fields except the `action` field.
   - Assert event name, source, schema version, non-empty event ID, trace ID,
     non-zero occurred-at, idempotency key, and payload compatibility with the
     fixture.
4. Update `docs/acceptance/gap-analysis.md`.
   - Extend the authorization-policy permissions batch paragraph to mention
     local/static `PolicyChanged` event-envelope fixture and producer contract
     evidence.
   - Explicitly keep live event bus, kind/e2e, DATA GA, Full GA, and live
     mutation caveats open.
5. Update `problem.md`.
   - Mirror the same local/static event fixture evidence wording.
   - Do not broaden the claim beyond fixture and producer-test coverage.

## 16. Verification Plan

Focused checks:

```sh
cd backend && go test ./internal/contracts -run EventEnvelope
cd backend && go test ./internal/services/authorizationpolicy -run 'PolicyChangedProducer|Event'
cd backend && go test ./internal/services/authorizationpolicy/...
git diff --check
```

Broader checks before final Reviewer approval:

```sh
cd backend && go test ./...
cd backend && go build ./...
make coverage
make ci-sonar
```

## 17. Rollback Plan

Revert the new fixture, the event-envelope expected-list/map additions, the new
authorization-policy producer contract test, and the two documentation edits.
No data or configuration rollback is needed.

## 18. Risks and Tradeoffs

- The fixture payload could drift from batch-permissions behavior if it is not
  aligned with `authorization-policy-batch-permissions.json`; the producer test
  and fixture fields should use the same operation vocabulary.
- This proves local/static contract shape only. It intentionally does not prove
  live event publication, event relay, consumers, e2e behavior, or GA readiness.
- Adding one narrow producer test is enough because no runtime producer code is
  changing.

## 19. Reviewer Checklist

- Scope is limited to fixture, tests, and docs.
- `policy-changed.json` is valid event-envelope schema v1.
- Event-envelope fixture filename list remains sorted.
- `wantTypes` includes `PolicyChanged` with producer
  `authorization-policy-service`.
- The service-local producer test directly covers `policyChangedEvent()`.
- Docs call the evidence local/static and preserve live bus/e2e/Full GA caveats.
- Required focused verification commands pass or failures are documented.

## 20. Status

Status: Approved
