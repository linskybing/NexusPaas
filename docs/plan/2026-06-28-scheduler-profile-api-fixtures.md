# Scheduler Profile API Fixtures

## 1. Objective

Add scheduler-quota service-level API fixture parity tests for the three
existing scheduler profile create fixtures:

- `POST /api/v1/accelerator-profiles`
- `POST /api/v1/network-profiles`
- `POST /api/v1/placement-profiles`

This is a local/static contract-evidence slice only. It must not add runtime
behavior, routes, handlers, migrations, deploy manifests, or live evidence
claims.

## 2. Background

The shared contract fixture set already includes the three scheduler profile API
fixtures and their matching profile-changed event fixtures. `schedulerquota.Spec()`
already declares the three admin create routes and matching events:
`AcceleratorProfileChanged`, `NetworkProfileChanged`, and
`PlacementProfileChanged`.

Other services have service-local API fixture parity tests that compare fixture
metadata against `Spec()` route metadata. Scheduler-quota currently has command
fixture parity tests, but no `api_fixtures_test.go` for these existing external
REST fixtures.

## 3. Source References

- `backend/internal/services/schedulerquota/spec.go`
- `backend/internal/services/schedulerquota/command_fixtures_test.go`
- `backend/internal/services/imageregistry/api_fixtures_test.go`
- `backend/internal/contracts/api_fixtures_test.go`
- `backend/internal/contracts/fixtures/api/v1/scheduler-create-accelerator-profile.json`
- `backend/internal/contracts/fixtures/api/v1/scheduler-create-network-profile.json`
- `backend/internal/contracts/fixtures/api/v1/scheduler-create-placement-profile.json`
- `backend/internal/contracts/fixtures/events/v1/accelerator-profile-changed.json`
- `backend/internal/contracts/fixtures/events/v1/network-profile-changed.json`
- `backend/internal/contracts/fixtures/events/v1/placement-profile-changed.json`
- `docs/acceptance/gap-analysis.md`
- `problem.md`
- `docs/acceptance/ga-acceptance-trace-matrix.md`

## 4. Assumptions

- The existing fixture files are the source of truth for this slice; the Code
  Agent should not create new scheduler profile fixture JSON unless a focused
  test proves a fixture is malformed.
- The target routes remain admin-only authenticated user routes.
- The existing event names in `schedulerquota.Spec().Events` and the fixtures
  should match exactly.
- `docs/acceptance/problem.md` does not exist in this checkout; the required
  problem ledger is root `problem.md`.
- Typed API contract coverage remains `Open`; this slice adds only local/static
  fixture parity evidence.

## 5. Non-Goals

- No runtime behavior changes.
- No new routes, route metadata changes, or handler changes.
- No database or migration changes.
- No deploy, Kubernetes, Helm, Terraform, or CI manifest changes.
- No new live/staging evidence claims.
- No OpenAPI generation work.
- No expansion beyond the three existing scheduler profile create fixtures.
- No change to the `Typed API contract coverage` matrix classification.

## 6. Current Behavior

- `schedulerquota.Spec()` declares:
  - `POST /api/v1/accelerator-profiles`, resource
    `accelerator_profiles`, action `create`, admin route.
  - `POST /api/v1/network-profiles`, resource `network_profiles`,
    action `create`, admin route.
  - `POST /api/v1/placement-profiles`, resource `placement_profiles`,
    action `create`, admin route.
- `schedulerquota.Spec().Events` includes the matching profile change events.
- The three external REST API fixtures already exist under
  `backend/internal/contracts/fixtures/api/v1/`.
- The matching event fixtures already exist under
  `backend/internal/contracts/fixtures/events/v1/`.
- There is no
  `backend/internal/services/schedulerquota/api_fixtures_test.go`.

## 7. Target Behavior

- A new schedulerquota service-local API fixture parity test loads the three
  existing create fixtures and compares them against `schedulerquota.Spec()`.
- The test verifies method, path, owner service, resource, action, auth posture,
  admin/state-changing route metadata, path parameters, success status, required
  request fields, key optional fields, emitted events, and fixture response
  shape at the level already present in the fixtures.
- Acceptance wording records this as local/static scheduler profile fixture
  parity evidence only.
- Typed API contract coverage remains `Open`.

## 8. Affected Domains

- `scheduler-quota-service`: profile create API fixture parity tests.
- Contract fixtures: existing scheduler profile external REST fixtures only.
- Acceptance evidence docs: local/static wording only.

## 9. Affected Files

- Add `backend/internal/services/schedulerquota/api_fixtures_test.go`.
- Update `docs/acceptance/gap-analysis.md`.
- Update `problem.md`.

No changes are planned for the existing fixture JSON files unless the new parity
test exposes a concrete mismatch that the Reviewer approves as in-scope.

## 10. API / Contract Changes

No runtime API changes.

The implementation should only assert local/static parity for existing
contracts:

- `scheduler.create_accelerator_profile`
  - method/path: `POST /api/v1/accelerator-profiles`
  - owner/resource/action: `scheduler-quota-service` /
    `scheduler-quota-service:accelerator_profiles` / `create`
  - auth: `user`, `auth_required: true`, `service_key_required: false`
  - route posture: admin, state-changing, no path params
  - required request fields: `name`
  - success status: `201`
  - emits event: `AcceleratorProfileChanged`
- `scheduler.create_network_profile`
  - method/path: `POST /api/v1/network-profiles`
  - owner/resource/action: `scheduler-quota-service` /
    `scheduler-quota-service:network_profiles` / `create`
  - auth: `user`, `auth_required: true`, `service_key_required: false`
  - route posture: admin, state-changing, no path params
  - required request fields: `name`, `primary_cni`
  - success status: `201`
  - emits event: `NetworkProfileChanged`
- `scheduler.create_placement_profile`
  - method/path: `POST /api/v1/placement-profiles`
  - owner/resource/action: `scheduler-quota-service` /
    `scheduler-quota-service:placement_profiles` / `create`
  - auth: `user`, `auth_required: true`, `service_key_required: false`
  - route posture: admin, state-changing, no path params
  - required request fields: `name`, `scheduler_backend`
  - success status: `201`
  - emits event: `PlacementProfileChanged`

## 11. Database / Migration Changes

None.

## 12. Configuration Changes

None.

## 13. Observability Changes

None.

The docs may mention existing event fixture parity, but the implementation must
not add logs, metrics, traces, or new outbox behavior.

## 14. Security Considerations

- Keep fixtures fake and stable; do not add secrets, credentials, tokens,
  cluster-private hostnames, real namespace names, or live IDs.
- Preserve `auth: user`, `auth_required: true`, and
  `service_key_required: false`.
- Preserve admin-only route expectations for all three create routes.
- Do not claim live authorization enforcement from this static test slice.

## 15. Implementation Steps

1. Add `backend/internal/services/schedulerquota/api_fixtures_test.go`.
2. Reuse the smallest local helper pattern from existing service fixture parity
   tests: decode fixture JSON, find the matching route in `Spec().Routes`, and
   assert metadata from a three-row case table.
3. Assert each fixture's `contract_name`, `owner_service`, `resource`, `action`,
   `method`, `path`, auth fields, empty `path_parameters`, required fields,
   `201` success status, expected event, and key response fields
   (`id`, `data`, `version`, `created_at`, `updated_at`).
4. Assert the matching route is admin, state-changing, not service-key
   required, and has no `IDParam`.
5. Assert each expected event is present in `schedulerquota.Spec().Events`.
6. Update `docs/acceptance/gap-analysis.md` with one concise paragraph saying
   scheduler accelerator/network/placement profile create fixtures now have
   service-local parity against `schedulerquota.Spec()` as local/static evidence
   only.
7. Update `problem.md` with matching concise local/static wording.
8. Do not change `docs/acceptance/ga-acceptance-trace-matrix.md` status for
   `Typed API contract coverage`; leave it `Open`.
9. Run `gofmt` on the new Go test file.

## 16. Verification Plan

Focused checks:

- `cd backend && go test ./internal/services/schedulerquota -run ExternalAPI`
- `cd backend && go test ./internal/services/schedulerquota -run Fixture`
- `cd backend && go test ./internal/contracts -run ExternalAPI`
- `cd backend && go test ./internal/contracts/...`
- `cd backend && go test ./internal/services/schedulerquota/...`

Broader checks, time permitting:

- `cd backend && go test ./...`
- `cd backend && go build ./...`
- `cd backend && make coverage`
- `cd backend && make ci-sonar`
- `git diff --check`

If `make coverage` or `make ci-sonar` is skipped for time, the Code Agent must
say so explicitly and avoid implying SonarScanner Quality Gate evidence.

## 17. Rollback Plan

Remove `backend/internal/services/schedulerquota/api_fixtures_test.go` and
revert the small wording updates in `docs/acceptance/gap-analysis.md` and
`problem.md`. No data rollback is required because there are no migrations or
runtime behavior changes.

## 18. Risks and Tradeoffs

- A too-broad helper could duplicate existing shared fixture validation. Keep
  the new test service-local and focused on `schedulerquota.Spec()` parity.
- Acceptance wording could accidentally imply live scheduler/profile behavior.
  Keep every doc update labeled local/static and do not move the matrix status.
- If an existing fixture mismatch is found, fixing that fixture may be in scope
  only when the mismatch is directly tied to the three target create contracts.

## 19. Reviewer Checklist

- Confirms scope is limited to one new service-local test file and two evidence
  wording updates unless a direct fixture mismatch is found.
- Confirms no runtime behavior, route, handler, migration, config, deployment,
  or live-evidence changes.
- Confirms the test checks all three existing create fixtures against
  `schedulerquota.Spec()`.
- Confirms required fields, success statuses, admin auth posture, route
  state-changing posture, resources/actions, and emitted events match the
  fixtures and spec.
- Confirms `Typed API contract coverage` remains `Open` in
  `docs/acceptance/ga-acceptance-trace-matrix.md`.
- Confirms focused schedulerquota and contracts tests were run, or skipped
  commands are explicitly reported.
- Confirms any SonarScanner Quality Gate statement is backed by an actual
  `make ci-sonar` run.
- Confirms diff scope is small and aligned with the approved plan.

## 20. Status

Status: Approved
