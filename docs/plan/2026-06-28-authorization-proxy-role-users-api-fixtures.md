# Authorization Proxy Role Users API Fixtures

## 1. Objective

Add the next small local/static typed contract slice for authorization-policy
proxy role-user administration:

- `GET /api/v1/admin/proxy-rbac/roles/{id}/users`
- `POST /api/v1/admin/proxy-rbac/roles/{id}/users`
- `DELETE /api/v1/admin/proxy-rbac/roles/{id}/users/{user_id}`

This slice adds API fixture evidence only. Typed API contract coverage remains
`Open`.

## 2. Background

`authorizationpolicy.Spec()` already declares admin-only proxy role-user routes
under `proxy_role_users`:

- list users for a proxy role, action `list`, route ID param `id`;
- assign one user to a proxy role, action `create`, route ID param `id`;
- unassign one user from a proxy role, action `delete`, route ID param
  `user_id`.

The handlers require admin access. `listRoleUsers` validates the role exists
and returns `200` or `404`. `assignRoleUser` validates the role exists, decodes
JSON, requires `user_id` or `userId`, returns `201` on creation, has a
theoretical existing-member `200` path, can return `409` on conflict, and emits
`ProxyPolicyChanged` with action `role_user_assign` for the created path.
`unassignRoleUser` deletes in a transaction and emits `ProxyPolicyChanged` with
action `role_user_unassign` only when the member is found.

Existing authorization-policy fixture tests already load API fixtures from
`backend/internal/contracts/fixtures/api/v1` and compare them to
`authorizationpolicy.Spec()`.

## 3. Source References

- `backend/internal/services/authorizationpolicy/spec.go`
- `backend/internal/services/authorizationpolicy/roles.go`
- `backend/internal/services/authorizationpolicy/authorization_policy_repository.go`
- `backend/internal/services/authorizationpolicy/api_fixtures_test.go`
- `backend/internal/contracts/api_fixtures_test.go`
- `backend/internal/contracts/fixtures/api/v1/authorization-policy-create-proxy-role.json`
- `backend/internal/contracts/fixtures/api/v1/authorization-policy-delete-proxy-role.json`
- `backend/internal/contracts/fixtures/api/v1/authorization-policy-list-proxy-services.json`
- `backend/internal/contracts/fixtures/events/v1/proxy-policy-changed.json`
- `docs/acceptance/gap-analysis.md`
- `problem.md`

## 4. Assumptions

- Branch is `storage-data-path`.
- These fixtures are local/static contract artifacts, not live route evidence.
- The existing `ProxyPolicyChanged` event fixture remains sufficient; no event
  fixture or event envelope test update is needed.
- The canonical request field for assignment is `user_id`; the handler also
  accepts `userId`.
- Delete fixture errors stay conservative at `[401, 403, 500]`. Current handler
  and repository behavior do not prove a delete `404`.
- Root `problem.md` is the requested problem ledger.

## 5. Non-Goals

- No runtime code changes.
- No `Spec()` route changes unless a direct fixture parity mismatch proves the
  existing metadata is wrong and Reviewer approves it.
- No handler, repository, migration, deploy, or configuration changes.
- No event fixture, event envelope test, outbox, or live event evidence changes.
- No GA trace matrix update.
- No `POST /api/v1/admin/proxy-rbac/roles/{id}/users/batch` fixture.
- No OpenAPI generation work.
- No closure of typed API contract coverage.

## 6. Current Behavior

- Shared API fixture validation has an explicit fixture filename list and route
  metadata map in `backend/internal/contracts/api_fixtures_test.go`.
- `backend/internal/services/authorizationpolicy/api_fixtures_test.go` already
  covers prior proxy role, proxy policy, and proxy service fixture slices.
- There are no external API fixtures for proxy role-user list, assign, or
  unassign.
- `unassignRoleUser` does not explicitly return `404` for missing role or user;
  missing repository rows return no error, no event, and the handler returns
  `200`.

## 7. Target Behavior

- The shared API fixture set includes and validates:
  - `authorization-policy-list-proxy-role-users.json`
  - `authorization-policy-assign-proxy-role-user.json`
  - `authorization-policy-unassign-proxy-role-user.json`
- Shared route metadata maps those fixtures to the existing admin proxy
  role-user routes.
- Authorization-policy service-local tests verify fixture parity with
  `authorizationpolicy.Spec()`.
- The batch role-user route remains uncovered by this slice.
- Acceptance wording records only local/static fixture coverage.
- Typed API contract coverage remains `Open`.

## 8. Affected Domains

- `authorization-policy-service`: proxy role-user admin API contract fixtures
  and service-local parity tests.
- Shared contracts: external REST API fixture validation.
- Acceptance docs: local/static evidence wording only.

## 9. Affected Files

Add:

- `backend/internal/contracts/fixtures/api/v1/authorization-policy-list-proxy-role-users.json`
- `backend/internal/contracts/fixtures/api/v1/authorization-policy-assign-proxy-role-user.json`
- `backend/internal/contracts/fixtures/api/v1/authorization-policy-unassign-proxy-role-user.json`

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

- `authorization-policy.list_proxy_role_users`
  - fixture: `authorization-policy-list-proxy-role-users.json`
  - owner: `authorization-policy-service`
  - surface/consumer: `external_rest` / `authenticated-user-client`
  - resource/action: `authorization-policy-service:proxy_role_users` / `list`
  - method/path: `GET /api/v1/admin/proxy-rbac/roles/{id}/users`
  - auth: `user`, `auth_required: true`, `service_key_required: false`
  - path parameters: `["id"]`
  - route `IDParam`: `id`
  - required/optional request fields: `[]` / `[]`
  - request example: `{}`
  - success statuses: `[200]`
  - error statuses: `[401, 403, 404, 500]`
  - emits events: `[]`
  - response example: collection wrapper with `items`, each row containing
    fake `id`, `role_id`, `user_id`, `assigned_by`, `created_at`, and a public
    nested `role`

- `authorization-policy.assign_proxy_role_user`
  - fixture: `authorization-policy-assign-proxy-role-user.json`
  - owner: `authorization-policy-service`
  - surface/consumer: `external_rest` / `authenticated-user-client`
  - resource/action: `authorization-policy-service:proxy_role_users` /
    `create`
  - method/path: `POST /api/v1/admin/proxy-rbac/roles/{id}/users`
  - auth: `user`, `auth_required: true`, `service_key_required: false`
  - path parameters: `["id"]`
  - route `IDParam`: `id`
  - required request fields: `["user_id"]`
  - optional request fields: `[]`
  - request example: fake `user_id`
  - success statuses: `[200, 201]`
  - error statuses: `[400, 401, 403, 404, 409, 500]`
  - emits events: `["ProxyPolicyChanged"]`
  - response example: created-member row containing fake `id`, `role_id`,
    `user_id`, `assigned_by`, `created_at`, and a public nested `role`

- `authorization-policy.unassign_proxy_role_user`
  - fixture: `authorization-policy-unassign-proxy-role-user.json`
  - owner: `authorization-policy-service`
  - surface/consumer: `external_rest` / `authenticated-user-client`
  - resource/action: `authorization-policy-service:proxy_role_users` /
    `delete`
  - method/path:
    `DELETE /api/v1/admin/proxy-rbac/roles/{id}/users/{user_id}`
  - auth: `user`, `auth_required: true`, `service_key_required: false`
  - path parameters: `["id", "user_id"]`
  - route `IDParam`: `user_id`
  - required/optional request fields: `[]` / `[]`
  - request example: `{}`
  - success statuses: `[200]`
  - error statuses: `[401, 403, 500]`
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
- Do not claim live authorization enforcement from static fixture tests.
- Treat `assigned_by` and `user_id` examples as fake non-sensitive IDs.

## 15. Implementation Steps

1. Add the three API fixture JSON files using the existing fixture schema and
   formatting.
2. Reuse the existing `ProxyPolicyChanged` event fixture; do not add or edit an
   event fixture.
3. Update `backend/internal/contracts/api_fixtures_test.go`:
   - add the three filenames to the expected sorted fixture list;
   - add `wantRoutes` entries for owner, resource, action, method, and path.
4. Extend `backend/internal/services/authorizationpolicy/api_fixtures_test.go`
   with a small role-user fixture table.
5. Reuse existing fixture loading and route lookup helpers.
6. Add only the minimum helper changes needed for role-user fixtures, because
   list is read-only/no-event while assign and unassign are state-changing.
7. For each role-user fixture, assert contract name, owner service, resource,
   action, method, path, auth fields, path parameters, route `IDParam`, success
   statuses, error statuses, emitted events, admin posture, auth posture, and
   service-key posture.
8. Assert list role-users is not state-changing and emits no events.
9. Assert assign/unassign are state-changing and emit `ProxyPolicyChanged`.
10. Assert assignment request examples include non-empty `user_id`.
11. Assert list and assign response examples include stable public role-user
    fields and a nested role; assert unassign request/response examples are
    empty objects.
12. Do not add a case for
    `POST /api/v1/admin/proxy-rbac/roles/{id}/users/batch`.
13. Update `docs/acceptance/gap-analysis.md` with one concise local/static
    paragraph for role-user list, assign, and unassign fixtures.
14. Update root `problem.md` with matching concise local/static wording and add
    these three routes to the typed API contracts row while keeping status
    `Open`.
15. Run `gofmt` on any changed Go test file.

## 16. Verification Plan

Focused checks:

- `cd backend && go test ./internal/contracts -run ExternalAPI`
- `cd backend && go test ./internal/services/authorizationpolicy -run 'ExternalAPI|Spec'`
- `git diff --check`

Broader checks, time permitting:

- `cd backend && go test ./internal/services/authorizationpolicy/...`
- `cd backend && go test ./...`
- `cd backend && go build ./...`
- `cd backend && make coverage`
- `cd backend && make ci-sonar`

If coverage or SonarScanner is skipped, the Code Agent must say so and must not
claim Quality Gate evidence. If SonarScanner runs, report the actual Quality
Gate result.

## 17. Rollback Plan

Remove the three proxy role-user API fixture files, revert the shared contract
test expected-list/map updates, revert the authorization-policy service-local
test extension, and revert the wording updates in
`docs/acceptance/gap-analysis.md` and `problem.md`.

No data rollback is required because there are no migrations, config changes,
deploy changes, event fixture changes, or runtime behavior changes.

## 18. Risks and Tradeoffs

- Assignment has both a created `201` path and a theoretical existing-member
  `200` path, but only the created path emits `ProxyPolicyChanged`. The fixture
  should document known success statuses while using the response example for
  the created/event-emitting path.
- Delete currently does not prove a `404` for missing role or user. Do not add
  `404` to delete error statuses unless tests prove repository behavior changed.
- The batch route is adjacent to the assign route and easy to include by
  accident. Keep it explicitly out of fixture and service-local cases.
- Delete has two path parameters but route `IDParam` is `user_id`; tests should
  assert both concepts separately.
- Static fixtures can be mistaken for live evidence. Keep docs local/static and
  keep typed API contract coverage `Open`.

## 19. Reviewer Checklist

- Confirms status is `Draft` before review.
- Confirms only the new plan file changed during Plan Agent work.
- Confirms implementation scope is limited to three API fixtures, shared API
  fixture validation, one service-local test extension, and two docs.
- Confirms no runtime handlers, repositories, migrations, deploy manifests,
  event fixtures, event envelope tests, GA trace matrix, or live evidence
  changed.
- Confirms the three fixture names and route metadata match
  `authorizationpolicy.Spec()`.
- Confirms list has no events and includes `404` for missing role.
- Confirms assign requires `user_id`, allows `200`/`201`, includes `409`, and
  emits `ProxyPolicyChanged`.
- Confirms delete uses path parameters `id` and `user_id`, route `IDParam`
  `user_id`, emits `ProxyPolicyChanged`, and keeps errors `[401, 403, 500]`.
- Confirms the batch role-user route is not included.
- Confirms service-local tests assert admin-only, authenticated-user,
  no-service-key posture and correct state-changing flags.
- Confirms docs keep typed API contract coverage `Open`.
- Confirms focused tests and `git diff --check` ran, or skipped broader checks
  are explicitly reported.
- Confirms any SonarScanner Quality Gate claim is backed by an actual run.

## 20. Status

Status: Approved
