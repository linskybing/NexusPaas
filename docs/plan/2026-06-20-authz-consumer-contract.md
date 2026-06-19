# Authorization Policy Identity Consumer Contract

## 1. Objective

Add a focused, fixture-backed consumer contract test proving that the
`authorization-policy-service` identity projection can consume the canonical v1
`UserUpdated` event envelope and write only its local identity read model.

## 2. Background

`docs/roadmap.md` Day 16-35 calls for event schema fixtures plus producer and
consumer contract tests for the core event envelope, starting with identity
and related events. Day 36-55 also prioritizes identity/authz data-boundary
migration through owner-read APIs or read models.

`problem.md` currently records that integration-proxy, cluster-read, and
gpuusage have fixture-backed consumer coverage, while remaining consumer event
paths and broader event-fed read-model adoption remain open. The
`authorizationpolicy` package already has an identity event projection and tests
with hand-built events, but it does not yet bind that consumer path to the
canonical v1 `UserUpdated` fixture.

## 3. Source References

- `AGENTS.md`: requires the three-agent workflow and review approval before
  implementation.
- `docs/agents/planning.md`: requires this plan file and the fixed plan
  sections.
- `docs/agents/workflow.md`: requires one short feature branch and one PR per
  goal.
- `docs/roadmap.md`: Day 16-35 event contract fixtures and Day 36-55
  identity/authz read-model priorities.
- `problem.md`: remaining GA blockers include broader consumer contract tests
  and broader event-fed read-model adoption.
- `backend/internal/services/authorizationpolicy/identity_projection.go`:
  `projectPolicyIdentityEvent` maps `UserUpdated` events into
  `policyIdentityUsers`.
- `backend/internal/services/authorizationpolicy/authorization_policy_projection_repository.go`:
  `policyIdentityReadModelID` keys identity user projections by `id` or
  `user_id`, and the local resource is `authorization-policy-service:identity_users`.
- `backend/internal/services/integrationproxy/event_consumer_contract_test.go`,
  `backend/internal/services/clusterread/event_consumer_contract_test.go`, and
  `backend/internal/services/gpuusage/event_consumer_contract_test.go`: existing
  fixture-backed consumer contract test pattern.
- `backend/internal/contracts/fixtures/events/v1/user-updated.json`: canonical
  v1 fixture for the identity `UserUpdated` event.

## 4. Assumptions

- The canonical `UserUpdated` fixture remains the correct v1 identity event
  artifact for this slice.
- The authorization-policy consumer should preserve representative identity
  payload fields in its local read model.
- The consumer must stay isolated: consuming the fixture must not populate the
  identity owner store resource.
- No external `/api/v1` behavior changes are needed.

## 5. Non-Goals

- Do not change production projection logic unless the fixture test exposes a
  real defect that must be fixed to satisfy the approved plan.
- Do not add new event fixtures.
- Do not broaden authorization-policy policy-data projection coverage in this
  slice.
- Do not change database schema, migrations, deployment manifests, runtime
  config, auth behavior, or public API routes.
- Do not refactor unrelated tests or shared helpers.

## 6. Current Behavior

`authorization-policy-service` can consume identity events through
`projectPolicyIdentityEvent`, and existing package tests prove synthetic
`UserCreated`, `UserDeleted`, and role projection behavior. Existing canonical
fixture-backed consumer contract tests cover integration-proxy, cluster-read,
and gpuusage, but not authorization-policy.

## 7. Target Behavior

A new authorization-policy consumer contract test will:

- Read `backend/internal/contracts/fixtures/events/v1/user-updated.json`.
- Decode it through `contracts.DecodeEventEnvelope`.
- Convert the envelope to a `contracts.Event` without changing fixture payload
  semantics.
- Invoke `projectPolicyIdentityEvent` in an isolated
  `authorization-policy-service` app.
- Assert that `policyIdentityUsers` contains a read-model record keyed by the
  fixture `user_id`.
- Assert that representative fields such as `user_id`, `display_name`,
  `status`, and `role_ids` are preserved.
- Assert that the identity owner store resource remains empty.

## 8. Affected Domains

- Authorization policy / authz read models.
- Identity event consumer contracts.
- GA architecture blocker tracking.

No new service boundary is created. The slice strengthens the existing
identity-to-authz event boundary with contract evidence.

## 9. Affected Files

Expected files:

- `docs/plan/2026-06-20-authz-consumer-contract.md`
- `backend/internal/services/authorizationpolicy/event_consumer_contract_test.go`
- `problem.md`

## 10. API / Contract Changes

No external API changes. The only contract change is additive test evidence that
an existing internal consumer accepts the existing v1 `UserUpdated` event
fixture.

## 11. Database / Migration Changes

None. The test uses the in-memory platform store and existing read-model
resource names.

## 12. Configuration Changes

None.

## 13. Observability Changes

None. This slice is test-only plus blocker tracking and does not alter runtime
metrics, logs, traces, or projection endpoints.

## 14. Security Considerations

- No secrets or local metadata will be read, printed, committed, or added.
- The test must assert that an isolated consumer does not write to the identity
  owner store resource.
- Public authn/authz behavior remains unchanged.

## 15. Implementation Steps

1. Add `backend/internal/services/authorizationpolicy/event_consumer_contract_test.go`
   following the established consumer-contract test pattern in neighboring
   services.
2. Implement a small local fixture reader and envelope-to-event helper in that
   test file.
3. Assert the authorization-policy identity read model is keyed by fixture
   `user_id`, preserves representative payload fields, and does not populate
   `identity-service:users`.
4. Update `problem.md` GA Architecture Roadmap Update to record
   authorization-policy fixture-backed consumer coverage, latest verification,
   and remaining open blockers.
5. Run focused checks first, then all required gates.

## 16. Verification Plan

Focused verification:

```bash
go -C backend test ./internal/services/authorizationpolicy -run 'Consumer|Projection|Contract|ReadModel' -count=1
go -C backend test ./internal/contracts ./internal/services/authorizationpolicy -run 'Event|Consumer|Projection|Contract|ReadModel' -count=1
```

Required gates:

```bash
git diff --check
go -C backend test ./... -count=1
go -C backend vet ./...
go -C backend build ./...
bash backend/scripts/ci-security-gate.sh quick
```

Additional gates if feasible for this slice:

```bash
bash backend/scripts/ci-security-gate.sh security
bash backend/scripts/ci-security-gate.sh sonar
```

E2E, live Kubernetes, and staging evidence are not expected because this slice
is limited to an in-process consumer contract test and `problem.md` tracking.
If a reviewer requests broader evidence, document the result or blocker before
PR completion.

## 17. Rollback Plan

Revert the new test file and the `problem.md` update. No runtime data, schema,
configuration, deployment, or public API rollback is required.

## 18. Risks and Tradeoffs

- Risk: fixture payload shape may diverge from fields expected by older
  authorization-policy synthetic tests. Mitigation: assert the current fixture
  fields directly and avoid production changes unless a real incompatibility is
  found.
- Risk: adding another local fixture helper duplicates existing helper code.
  Tradeoff: this matches current per-package test style and avoids introducing
  a shared test abstraction for one narrow slice.
- Risk: this does not complete all remaining consumer paths. Mitigation: update
  `problem.md` to keep broader consumer coverage and read-model adoption open.

## 19. Reviewer Checklist

- Scope is limited to one consumer path and blocker tracking.
- No `/api/v1`, schema, config, deployment, or secret handling changes.
- Test uses the canonical v1 fixture and verifies local read-model isolation.
- Verification commands are concrete and include required gates.
- `problem.md` still records remaining GA blockers.

## 20. Status

Status: Approved
