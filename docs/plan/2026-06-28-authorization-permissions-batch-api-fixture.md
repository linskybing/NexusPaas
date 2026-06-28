# Authorization Permissions Batch API Fixture

## 1. Objective

Add the next local/static typed contract slice for authorization-policy
permissions batch processing:

- repair `authorizationpolicy.Spec()` metadata so
  `POST /api/v1/permissions/batch` is marked admin-only, matching its handler;
- add one external REST API fixture for the permissions batch route;
- update shared and service-local fixture tests;
- record concise local/static acceptance wording in
  `docs/acceptance/gap-analysis.md` and root `problem.md`.

This plan is for contract parity and one Spec metadata repair only. Typed API
contract coverage remains `Open`.

## 2. Background

`authorizationpolicy.Spec()` currently declares
`POST /api/v1/permissions/batch` as resource `policies`, action `batch`, but the
route is missing `admin()` metadata. The handler
`batchProcessPermissions()` requires admin access through `requireAdmin()`, so
the Spec under-states the route's real authorization posture.

`batchProcessPermissions()` decodes a JSON object with an `operations` array.
Each operation requires `type`, `action`, and `user_id`/`userId`, with optional
`project_id`/`projectId`, `group_id`/`groupId`, and `role`. Bad decode or
invalid request returns `400`; failed admin auth returns `401`/`403`; repository
transaction errors return `500`; success returns `200` with an empty response
body. Each operation calls `ApplyPermissionOperationTx()` and emits
`PolicyChanged` with action `batch_permissions_processed`.

The permissions batch request example must deliberately use operation
vocabulary accepted by `ApplyPermissionOperationTx()`:
`type: "project_member"` and `action: "add"`.

Existing API fixture schema supports object `request_example` values. Raw
permission policy CRUD routes consume raw JSON arrays, so those routes stay out
of this slice.

## 3. Source References

- `backend/internal/services/authorizationpolicy/spec.go`
- `backend/internal/services/authorizationpolicy/permissions.go`
- `backend/internal/services/authorizationpolicy/helpers.go`
- `backend/internal/services/authorizationpolicy/spec_test.go`
- `backend/internal/services/authorizationpolicy/api_fixtures_test.go`
- `backend/internal/contracts/api_fixtures_test.go`
- `backend/internal/contracts/fixtures/api/v1/*.json`
- `docs/acceptance/gap-analysis.md`
- `problem.md`

## 4. Assumptions

- Branch is `storage-data-path`.
- The new fixture is local/static contract evidence, not live route proof.
- `PolicyChanged` event-envelope fixture coverage is a separate remaining gap;
  this slice records the API route's emitted event name only.
- The route handler and repository behavior are correct and must not change.
- Root `problem.md` is the requested problem ledger.
- `docs/acceptance/ga-acceptance-trace-matrix.md` remains untouched unless a
  Reviewer explicitly requires it; if touched, typed API contract coverage must
  remain `Open`.

## 5. Non-Goals

- No runtime handler, repository, transaction, or event emission behavior
  changes.
- No raw `/api/v1/permissions/policy` CRUD fixtures because those handlers
  consume raw JSON arrays while API fixture `request_example` currently supports
  objects.
- No service-internal `/api/v1/permissions/enforce` fixture.
- No simulate fixture.
- No deploy, migration, event fixture, event envelope, GA matrix, OpenAPI, live
  evidence, kind/e2e cluster, or broader contract closure work.

## 6. Current Behavior

- `authorizationpolicy.Spec()` declares
  `POST /api/v1/permissions/batch` with resource `policies`, action `batch`,
  auth required, state changing, and no service-key requirement.
- The same Spec route is missing `Admin: true` metadata even though
  `batchProcessPermissions()` calls `requireAdmin()`.
- There is no external REST API fixture for the permissions batch route.
- Shared API fixture validation has an explicit expected fixture list and route
  metadata map in `backend/internal/contracts/api_fixtures_test.go`.
- Authorization-policy service-local fixture tests already cover existing
  authorization-policy fixtures against `authorizationpolicy.Spec()`.

## 7. Target Behavior

- `POST /api/v1/permissions/batch` has Spec metadata:
  `Admin: true`, `AuthRequired: true`, `ServiceAuthRequired: false`, and
  `StateChanging: true`.
- Shared API fixtures include
  `authorization-policy-batch-permissions.json`.
- Authorization-policy service-local tests verify the new fixture against
  `authorizationpolicy.Spec()`.
- The fixture records an authenticated user external REST contract that emits
  `PolicyChanged` and has object-shaped request and response examples.
- Acceptance wording records only local/static fixture coverage and the Spec
  admin metadata repair.

## 8. Affected Domains

- `authorization-policy-service`: route metadata repair, permissions batch API
  fixture, and service-local fixture parity tests.
- Shared contracts: external REST fixture validation.
- Acceptance docs: local/static evidence wording only.

## 9. Affected Files

Add:

- `backend/internal/contracts/fixtures/api/v1/authorization-policy-batch-permissions.json`

Update:

- `backend/internal/services/authorizationpolicy/spec.go`
- `backend/internal/contracts/api_fixtures_test.go`
- `backend/internal/services/authorizationpolicy/api_fixtures_test.go`
- `docs/acceptance/gap-analysis.md`
- `problem.md`

Avoid:

- `backend/internal/services/authorizationpolicy/permissions.go`
- `backend/internal/contracts/fixtures/events/v1/policy-changed.json`
- `backend/internal/contracts/event_envelope_test.go`
- `docs/acceptance/ga-acceptance-trace-matrix.md`

## 10. API / Contract Changes

No runtime API behavior changes.

Repair existing Spec metadata:

- `POST /api/v1/permissions/batch`
  - add `admin()` to the route declaration only.

Add static external REST fixture:

- `authorization-policy.batch_permissions`
  - fixture: `authorization-policy-batch-permissions.json`
  - owner: `authorization-policy-service`
  - API surface / consumer: `external_rest` / `authenticated-user-client`
  - resource/action: `authorization-policy-service:policies` / `batch`
  - method/path: `POST /api/v1/permissions/batch`
  - auth: `user`
  - `auth_required`: `true`
  - `service_key_required`: `false`
  - path parameters: `[]`
  - required request fields: `["operations"]`
  - optional request fields: `[]`
  - request example:

    ```json
    {
      "operations": [
        {
          "type": "project_member",
          "action": "add",
          "project_id": "project-ga-alpha",
          "user_id": "user-ga-ada",
          "role": "viewer"
        }
      ]
    }
    ```

  - success statuses: `[200]`
  - error statuses: `[400, 401, 403, 500]`
  - emits events: `["PolicyChanged"]`
  - response example: `{}`

Update shared expected maps in `backend/internal/contracts/api_fixtures_test.go`
for the new filename and route metadata. Do not update event fixture maps; the
missing `PolicyChanged` event-envelope fixture is intentionally left for the
next event-contract slice.

## 11. Database / Migration Changes

None.

## 12. Configuration Changes

None.

## 13. Observability Changes

None.

## 14. Security Considerations

- The Spec metadata repair closes a contract mismatch by marking the route as
  admin-only, matching `requireAdmin()` in the handler.
- The fixture must keep `auth_required: true` and `service_key_required: false`
  to match authenticated user admin access, not service-key access.
- The request example must use fake IDs only and contain no credentials,
  secrets, tokens, or real personal data.

## 15. Implementation Steps

1. Update `backend/internal/services/authorizationpolicy/spec.go` by adding
   `admin()` only to the `POST /api/v1/permissions/batch` route declaration.
2. Add
   `backend/internal/contracts/fixtures/api/v1/authorization-policy-batch-permissions.json`
   with the exact contract metadata, statuses, events, request example, and
   empty response example listed in this plan.
3. Update `backend/internal/contracts/api_fixtures_test.go` to include the new
   fixture in the expected external API fixture list and route metadata map.
4. Update `backend/internal/services/authorizationpolicy/api_fixtures_test.go`
   with a focused batch permissions case that verifies:
   - route metadata: `Admin true`, `AuthRequired true`,
     `ServiceAuthRequired false`, `StateChanging true`;
   - fixture metadata, statuses, and `PolicyChanged` event linkage;
   - `request_example.operations` is a non-empty array;
   - the first operation has `type: "project_member"`,
     `action: "add"`, and `user_id: "user-ga-ada"`;
   - the operation includes `project_id: "project-ga-alpha"` and
     `role: "viewer"` for the project-member add example.
5. Update `docs/acceptance/gap-analysis.md` to record local/static coverage for
   the permissions batch fixture and Spec admin metadata repair while keeping
   broader typed API contract coverage `Open`.
6. Update `problem.md` with the same concise local/static evidence note and
   remaining non-goals.

## 16. Verification Plan

Required for this slice:

```sh
cd backend && go test ./internal/contracts -run 'ExternalAPI|EventEnvelope'
cd backend && go test ./internal/services/authorizationpolicy -run 'ExternalAPI|Spec'
cd backend && go test ./internal/services/authorizationpolicy/...
git diff --check
```

Broader checks if the local slice passes:

```sh
cd backend && go test ./...
cd backend && go build ./...
cd backend && make coverage
cd backend && make ci-sonar
```

## 17. Rollback Plan

Revert the five planned edits:

- remove
  `backend/internal/contracts/fixtures/api/v1/authorization-policy-batch-permissions.json`;
- remove the new shared fixture expected-list/map entries;
- remove the new authorization-policy service-local test case/helpers;
- remove `admin()` from the batch route only if rolling back this metadata
  repair is explicitly desired;
- remove the matching `docs/acceptance/gap-analysis.md` and `problem.md`
  evidence notes.

## 18. Risks and Tradeoffs

- Adding `admin()` may expose tests or tooling that previously relied on the
  incorrect non-admin Spec metadata; that should be treated as the mismatch this
  slice repairs.
- The empty response fixture uses `{}` even though the handler returns nil on
  success; this follows existing fixture convention for empty object responses.
- Raw permission policy CRUD routes remain uncovered because supporting raw JSON
  array request examples would require fixture schema work outside this slice.

## 19. Reviewer Checklist

- Plan scope matches only the permissions batch fixture plus one Spec admin
  metadata repair.
- `spec.go` change is limited to adding `admin()` on
  `POST /api/v1/permissions/batch`.
- Fixture metadata exactly matches this plan, including contract name,
  owner, resource/action, statuses, events, and object examples.
- Tests verify route admin/auth/state-changing metadata and request example
  shape.
- `PolicyChanged` event-envelope fixture work remains out of scope; no event
  fixture or event envelope map is changed.
- No handler, repository, migration, deploy, OpenAPI, or live evidence files are
  changed.
- Verification commands and results are recorded by Code Agent.
- Diff is small, service-owned, SOLID-aligned, 12-Factor-compatible,
  SonarScanner-clean, and scoped to local/static contract parity.

## 20. Status

Status: Approved
