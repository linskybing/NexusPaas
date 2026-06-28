# Authorization Proxy Policy Assignments API Fixtures

## 1. Objective

Add the next local/static typed contract slice for authorization-policy proxy
policy assignment administration:

- `GET /api/v1/admin/proxy-rbac/policies/{id}/assignments`
- `POST /api/v1/admin/proxy-rbac/policies/{id}/assignments`
- `DELETE /api/v1/admin/proxy-rbac/policies/{id}/assignments`

This slice adds API fixture evidence only. Typed API contract coverage remains
`Open`.

## 2. Background

`authorizationpolicy.Spec()` already declares admin-only proxy policy assignment
routes under `proxy_policy_assignments`:

- list assignments for a proxy policy, action `list`, route ID param `id`;
- assign a target to a proxy policy, action `create`, route ID param `id`;
- unassign a target from a proxy policy, action `delete`, route ID param `id`.

The handlers require admin access. List returns a `200` collection and emits no
events. Assign accepts `target_type` and `target_id`, returns `200` or `201`,
and emits `ProxyPolicyChanged` only when a new assignment is created. Unassign
accepts `target_type` and `target_id` in the request body, returns `200`, and
emits `ProxyPolicyChanged` only when an existing assignment is removed. Missing
assignment on unassign is not a `404`.

## 3. Source References

- `backend/internal/services/authorizationpolicy/spec.go`
- `backend/internal/services/authorizationpolicy/assignments.go`
- `backend/internal/services/authorizationpolicy/policies.go`
- `backend/internal/services/authorizationpolicy/authorization_policy_repository.go`
- `backend/internal/services/authorizationpolicy/api_fixtures_test.go`
- `backend/internal/contracts/api_fixtures_test.go`
- `backend/internal/contracts/fixtures/api/v1/authorization-policy-list-proxy-role-users.json`
- `backend/internal/contracts/fixtures/api/v1/authorization-policy-assign-proxy-role-user.json`
- `backend/internal/contracts/fixtures/api/v1/authorization-policy-unassign-proxy-role-user.json`
- `backend/internal/contracts/fixtures/events/v1/proxy-policy-changed.json`
- `docs/acceptance/gap-analysis.md`
- `problem.md`

## 4. Assumptions

- Branch is `storage-data-path`.
- These fixtures are local/static contract artifacts, not live route evidence.
- The existing `ProxyPolicyChanged` event fixture remains sufficient; no event
  fixture or event envelope test update is needed.
- The canonical request fields for assign and unassign are `target_type` and
  `target_id`.
- Unassign does not document `404`; missing assignment returns `200` with no
  event.
- Root `problem.md` is the requested problem ledger.

## 5. Non-Goals

- No runtime behavior changes.
- No `Spec()` route changes unless a direct fixture parity mismatch proves the
  existing metadata is wrong and Reviewer approves it.
- No handler, repository, migration, deploy, event envelope, outbox, or
  configuration changes.
- No event fixture changes.
- No GA trace matrix update or live evidence claim.
- No `POST /api/v1/admin/proxy-rbac/policies/{id}/assignments/batch` fixture.
- No `GET /api/v1/admin/proxy-rbac/targets/{type}/{id}/assignments` fixture.
- No OpenAPI generation work.
- No claim that typed API contract coverage is complete.

## 6. Current Behavior

- Shared API fixture validation has an explicit fixture filename list and route
  metadata map in `backend/internal/contracts/api_fixtures_test.go`.
- `backend/internal/services/authorizationpolicy/api_fixtures_test.go` already
  covers prior authorization-policy static API fixture slices.
- There are no external API fixtures for proxy policy assignment list, assign,
  or unassign.
- Assignment batch and target-assignment read routes are outside this slice.

## 7. Target Behavior

- The shared API fixture set includes and validates:
  - `authorization-policy-list-proxy-policy-assignments.json`
  - `authorization-policy-assign-proxy-policy.json`
  - `authorization-policy-unassign-proxy-policy.json`
- Shared route metadata maps those fixtures to the existing admin proxy policy
  assignment routes.
- Authorization-policy service-local tests verify fixture parity with
  `authorizationpolicy.Spec()`.
- Acceptance wording records only local/static fixture coverage.
- Typed API contract coverage remains `Open`.

## 8. Affected Domains

- `authorization-policy-service`: proxy policy assignment admin API contract
  fixtures and service-local parity tests.
- Shared contracts: external REST API fixture validation.
- Acceptance docs: local/static evidence wording only.

## 9. Affected Files

Add:

- `backend/internal/contracts/fixtures/api/v1/authorization-policy-list-proxy-policy-assignments.json`
- `backend/internal/contracts/fixtures/api/v1/authorization-policy-assign-proxy-policy.json`
- `backend/internal/contracts/fixtures/api/v1/authorization-policy-unassign-proxy-policy.json`

Update:

- `backend/internal/contracts/api_fixtures_test.go`
- `backend/internal/services/authorizationpolicy/api_fixtures_test.go`
- `docs/acceptance/gap-analysis.md`
- `problem.md`

Avoid:

- `backend/internal/contracts/fixtures/events/v1/proxy-policy-changed.json`
- `backend/internal/contracts/event_envelope_test.go`
- `docs/acceptance/ga-acceptance-trace-matrix.md`

## 10. API / Contract Changes

No runtime API changes.

Add these static external REST fixture contracts:

- `authorization-policy.list_proxy_policy_assignments`
  - fixture: `authorization-policy-list-proxy-policy-assignments.json`
  - owner: `authorization-policy-service`
  - surface/consumer: `external_rest` / `authenticated-user-client`
  - resource/action: `authorization-policy-service:proxy_policy_assignments` /
    `list`
  - method/path: `GET /api/v1/admin/proxy-rbac/policies/{id}/assignments`
  - auth: `user`, `auth_required: true`, `service_key_required: false`
  - path parameters: `["id"]`
  - route `IDParam`: `id`
  - required/optional request fields: `[]` / `[]`
  - request example: `{}`
  - success statuses: `[200]`
  - error statuses: `[401, 403, 404, 500]`
  - emits events: `[]`
  - response example: collection wrapper with `items`, each row containing
    fake `id`, `policy_id`, `target_type`, `target_id`, `created_at`, and a
    public nested `policy`

- `authorization-policy.assign_proxy_policy`
  - fixture: `authorization-policy-assign-proxy-policy.json`
  - owner: `authorization-policy-service`
  - surface/consumer: `external_rest` / `authenticated-user-client`
  - resource/action: `authorization-policy-service:proxy_policy_assignments` /
    `create`
  - method/path: `POST /api/v1/admin/proxy-rbac/policies/{id}/assignments`
  - auth: `user`, `auth_required: true`, `service_key_required: false`
  - path parameters: `["id"]`
  - route `IDParam`: `id`
  - required request fields: `["target_type", "target_id"]`
  - optional request fields: `[]`
  - request example: fake `target_type` and `target_id`
  - success statuses: `[200, 201]`
  - error statuses: `[400, 401, 403, 404, 409, 500]`
  - emits events: `["ProxyPolicyChanged"]`
  - response example: assignment row containing fake `id`, `policy_id`,
    `target_type`, `target_id`, `created_at`, and a public nested `policy`

- `authorization-policy.unassign_proxy_policy`
  - fixture: `authorization-policy-unassign-proxy-policy.json`
  - owner: `authorization-policy-service`
  - surface/consumer: `external_rest` / `authenticated-user-client`
  - resource/action: `authorization-policy-service:proxy_policy_assignments` /
    `delete`
  - method/path: `DELETE /api/v1/admin/proxy-rbac/policies/{id}/assignments`
  - auth: `user`, `auth_required: true`, `service_key_required: false`
  - path parameters: `["id"]`
  - route `IDParam`: `id`
  - required request fields: `["target_type", "target_id"]`
  - optional request fields: `[]`
  - request example: fake `target_type` and `target_id`
  - success statuses: `[200]`
  - error statuses: `[400, 401, 403, 500]`
  - emits events: `["ProxyPolicyChanged"]`
  - response example: `{}`

All three fixtures must set compatibility to additive fields and tolerant
readers.

## 11. Database / Migration Changes

None.

## 12. Configuration Changes

None.

## 13. Observability Changes

None.

The fixtures document existing event linkage only. Do not add logs, metrics,
traces, outbox behavior, or event emission code.

## 14. Security Considerations

- Preserve admin-only route expectations in service-local tests.
- Preserve authenticated-user posture:
  `auth: user`, `auth_required: true`, `service_key_required: false`.
- Fixture examples must use fake IDs only and no secrets, tokens, cookies,
  passwords, credentials, internal IDs, or live hostnames.
- Treat `target_id` examples as fake non-sensitive IDs.
- Do not claim live authorization enforcement from static fixture tests.

## 15. Implementation Steps

1. Add the three API fixture JSON files using the existing fixture schema and
   formatting.
2. Reuse the existing `ProxyPolicyChanged` event fixture; do not add or edit an
   event fixture.
3. Update `backend/internal/contracts/api_fixtures_test.go`:
   - add the three filenames to the expected sorted fixture list;
   - add `wantRoutes` entries for owner, resource, action, method, and path.
4. Extend `backend/internal/services/authorizationpolicy/api_fixtures_test.go`
   with a small proxy policy assignment fixture table.
5. Reuse existing fixture loading and route lookup helpers.
6. Add only the minimum helper changes needed for assignment fixtures, because
   list is read-only/no-event while assign and unassign are state-changing.
7. For each assignment fixture, assert contract name, owner service, resource,
   action, method, path, auth fields, path parameters, route `IDParam`, success
   statuses, error statuses, emitted events, admin posture, auth posture, and
   service-key posture.
8. Assert list assignments is not state-changing and emits no events.
9. Assert assign and unassign are state-changing and emit
   `ProxyPolicyChanged`.
10. Assert assign and unassign request examples include non-empty
    `target_type` and `target_id`.
11. Assert list and assign response examples include stable public assignment
    fields and a nested policy; assert unassign response example is an empty
    object.
12. Do not add cases for the batch route or target-assignment read route.
13. Update `docs/acceptance/gap-analysis.md` with one concise local/static
    paragraph for assignment list, assign, and unassign fixtures.
14. Update root `problem.md` with matching concise local/static wording and add
    these three routes to the typed API contracts row while keeping status
    `Open`.
15. Run `gofmt` on any changed Go test file.

## 16. Verification Plan

Focused checks:

- `cd backend && go test ./internal/contracts -run 'ExternalAPI|EventEnvelope'`
- `cd backend && go test ./internal/services/authorizationpolicy -run 'ExternalAPI|Spec'`
- `cd backend && go test ./internal/services/authorizationpolicy/...`
- `git diff --check`

Broader gates if the local slice passes:

- `cd backend && go test ./...`
- `cd backend && go build ./...`
- `cd backend && make coverage`
- `cd backend && make ci-sonar`

## 17. Rollback Plan

Revert only this slice:

- remove the three proxy policy assignment API fixture JSON files;
- remove their shared expected-list and route-map entries;
- remove the service-local assignment fixture test cases/helpers added for this
  slice;
- revert the matching local/static acceptance wording in
  `docs/acceptance/gap-analysis.md` and `problem.md`.

No database, migration, runtime, deploy, or configuration rollback is needed.

## 18. Risks and Tradeoffs

- Static fixtures prove contract parity with `Spec()`; they do not prove live
  handler behavior.
- `ProxyPolicyChanged` is conditional for assign and unassign, but fixtures can
  only document that the operation may emit the event.
- Unassign intentionally omits `404`; adding it would overstate current handler
  behavior.
- Keeping batch and target-assignment read routes out of scope leaves known
  API fixture gaps for later slices.

## 19. Reviewer Checklist

- Status is `Draft`.
- Scope is limited to local/static fixture parity.
- The three planned fixture files match existing spec routes exactly.
- Batch and target-assignment read routes are excluded.
- Unassign request body includes `target_type` and `target_id`.
- Unassign error statuses exclude `404`.
- No runtime, repository, migration, deploy, event fixture, event envelope, or
  GA matrix changes are planned.
- Verification commands include the requested focused checks and broader gates.
- Typed API contract coverage remains `Open`.

## 20. Status

Status: Approved
