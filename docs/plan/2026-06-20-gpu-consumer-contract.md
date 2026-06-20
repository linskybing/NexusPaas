# GPU Usage Consumer Contract Coverage

## 1. Objective

Add a small Day 16-35 contract-testing slice that binds the existing
`gpuusage` event-fed projection consumer to the canonical v1 `JobSubmitted`
event fixture.

## 2. Background

Connector preflight on 2026-06-20 showed a clean `main...origin/main` at
`a08c206` and a healthy DevSpace runtime: the required `devspace-mcp` and
`cloudflared-devspace-mcp` screen sessions were present, and public `/mcp`
returned the expected OAuth 401.

`docs/roadmap.md` says Day 16-35 must establish event schema fixtures and
producer/consumer contract tests for core events. `problem.md` says core v1
fixtures, initial producer tests, and initial consumer tests for
integration-proxy and cluster-read exist, but remaining consumer event coverage
is still open.

The existing `gpuusage` projection already consumes workload job events,
including `JobSubmitted`, into local read models. This slice makes that
consumer relationship explicit and fixture-backed before later data-boundary
cutover work.

## 3. Source References

- `AGENTS.md`: requires Plan Agent -> Reviewer Agent approval -> Code Agent ->
  Reviewer Agent final approval.
- `docs/roadmap.md`: Day 16-35 requires versioned event fixtures and
  producer/consumer contract tests.
- `problem.md`: remaining high-priority contract-testing issue includes
  remaining consumer event coverage.
- `backend/internal/contracts/fixtures/events/v1/job-submitted.json`: canonical
  v1 event fixture for workload `JobSubmitted`.
- `backend/internal/services/gpuusage/projection.go`: existing consumer handles
  `JobSubmitted` via `gpuProjection` and writes `gpuJobsResource`.
- `backend/internal/services/clusterread/event_consumer_contract_test.go` and
  `backend/internal/services/integrationproxy/event_consumer_contract_test.go`:
  local precedent for fixture-backed consumer contract tests.
- Microservice architecture guidance: prefer event-fed read models over direct
  database coupling and keep contract checks in CI.

## 4. Assumptions

- `JobSubmitted` remains a canonical v1 event produced by workload-service.
- `gpuusage` should keep consuming the event into its local job read model.
- The consumer contract can be proved with a focused in-process test because no
  API behavior, broker wiring, database schema, or deployment manifest changes
  are planned.

## 5. Non-Goals

- Do not add new event fixture files.
- Do not change the `JobSubmitted` fixture shape.
- Do not add a durable relay, broker, or publish-lag implementation.
- Do not alter `/api/v1` routes or response bodies.
- Do not migrate any additional shared-store reads in this slice.
- Do not add staging, Kubernetes, or live E2E evidence.

## 6. Current Behavior

`gpuusage` has projection code and unit tests for synthetic events, but it does
not yet have a consumer contract test that decodes the canonical
`job-submitted.json` fixture and proves the real projection preserves required
payload fields in the local job read model.

## 7. Target Behavior

The `gpuusage` package has a fixture-backed consumer contract test that:

- decodes `backend/internal/contracts/fixtures/events/v1/job-submitted.json`;
- converts it to the current internal `contracts.Event` shape using the same
  tolerant-reader pattern as existing consumer tests;
- invokes the real `projectGPUUsageEvent` projection path;
- asserts the local GPU job read model is keyed by `job_id`;
- asserts representative fixture payload fields are preserved;
- asserts the isolated consumer does not populate the workload owner store.

## 8. Affected Domains

- Workload events: `JobSubmitted` v1 fixture.
- Usage/observability unit: `gpuusage` local read model consumer.
- Contract testing: consumer-side fixture coverage.

## 9. Affected Files

- Add `backend/internal/services/gpuusage/event_consumer_contract_test.go`.
- Update `problem.md` with the new evidence and remaining blockers.
- Keep this plan under `docs/plan/2026-06-20-gpu-consumer-contract.md`.

## 10. API / Contract Changes

No public API changes. No event fixture shape changes. This is an additive test
that locks the existing consumer behavior to the existing v1 `JobSubmitted`
contract.

## 11. Database / Migration Changes

None. The test uses the existing in-memory platform store and existing local
read-model resource names.

## 12. Configuration Changes

None.

## 13. Observability Changes

No runtime observability changes. The new test is evidence that one additional
event-fed read model can be contract-verified before GA data-boundary migration.

## 14. Security Considerations

No secrets, credentials, or auth behavior changes. The test must not read or
print local secret files. The fixture payload does not contain tokens,
passwords, or owner credentials.

## 15. Implementation Steps

1. Add `backend/internal/services/gpuusage/event_consumer_contract_test.go`.
2. Implement a local fixture reader using `contracts.DecodeEventEnvelope`.
3. Add a fixture-to-`contracts.Event` helper that preserves event ID, type,
   producer, occurred timestamp, trace ID, schema version, and payload.
4. Add `TestGPUUsageConsumerMatchesJobSubmittedFixture`.
5. Assert the projected `gpuJobsResource` record uses fixture `job_id` as both
   record ID and local `id`.
6. Assert `job_id`, `project_id`, `user_id`, `config_commit_id`, `image_ref`,
   and `requested_resources` are preserved.
7. Assert `status` defaults to `submitted` for `JobSubmitted`.
8. Assert isolated `workloadJobsResource` remains empty.
9. Update `problem.md` to list the focused test result and reduce the remaining
   consumer coverage blocker accordingly.

## 16. Verification Plan

Run:

- `go -C backend test ./internal/services/gpuusage -run 'Consumer|Projection|Contract|ReadModel' -count=1`
- `go -C backend test ./internal/contracts ./internal/services/gpuusage -run 'Event|Consumer|Projection|Contract|ReadModel' -count=1`
- `git diff --check`
- `go -C backend test ./... -count=1`
- `go -C backend vet ./...`
- `go -C backend build ./...`
- `bash backend/scripts/ci-security-gate.sh quick`

If feasible after the required gates, also run:

- `bash backend/scripts/ci-security-gate.sh sonar`
- `bash backend/scripts/ci-security-gate.sh security`

No E2E, live Kubernetes, or staging evidence is required because this slice is
an additive in-process consumer contract test with no runtime deployment change.

## 17. Rollback Plan

Revert the new test file and the `problem.md` update. No migration, config, or
runtime rollback is required.

## 18. Risks and Tradeoffs

- This improves consumer contract evidence but does not complete all remaining
  consumer events.
- The test intentionally uses the current internal `contracts.Event` shape
  instead of introducing a new shared fixture SDK.
- Durable relay, publish-lag evidence, drift comparison, and broader read-model
  adoption remain separate roadmap slices.

## 19. Reviewer Checklist

- Requirement fit: does the slice address Day 16-35 consumer contract coverage?
- Scope: are changes limited to one consumer test, this plan, and `problem.md`?
- API compatibility: are `/api/v1` and event fixture shapes unchanged?
- Data ownership: does the test prove `gpuusage` writes a local read model
  without populating workload owner resources?
- Security: no secrets or credentials are read, logged, or committed.
- Verification: required gates are concrete and sufficient for this scope.
- Simplicity: no new frameworks, abstractions, configs, migrations, or
  deployment files.

## 20. Status

Status: Approved

Reviewer Agent approved this plan for Code Agent implementation. Final review
must include Sonar and security evidence, or an explicit blocker and residual
risk if either gate cannot run.
