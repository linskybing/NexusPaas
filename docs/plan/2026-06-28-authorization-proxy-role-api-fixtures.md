# Authorization Proxy Role API Fixtures

## 1. Objective

Add the next local/static typed contract slice for authorization-policy proxy
role administration:

- three external REST API fixtures for proxy role create, update, and delete;
- one `ProxyPolicyChanged` event envelope fixture;
- shared contract fixture expected-list/map updates;
- authorization-policy service-local fixture parity tests against
  `authorizationpolicy.Spec()`;
- concise local/static acceptance wording in `docs/acceptance/gap-analysis.md`
  and root `problem.md`.

This plan is for contract evidence only. Typed API contract coverage remains
`Open`.

## 2. Background

`authorizationpolicy.Spec()` already declares admin-only proxy role routes:

- `POST /api/v1/admin/proxy-rbac/roles`, resource `proxy_roles`, action
  `create`;
- `PUT /api/v1/admin/proxy-rbac/roles/{id}`, resource `proxy_roles`, action
  `update`, ID param `id`;
- `DELETE /api/v1/admin/proxy-rbac/roles/{id}`, resource `proxy_roles`, action
  `delete`, ID param `id`.

The matching handlers in `roles.go` require admin access and emit
`ProxyPolicyChanged` with payload actions `role_create`, `role_update`, and
`role_delete`. `authorizationpolicy.Spec().Events` already includes
`ProxyPolicyChanged`, but the shared event fixture set has no
`proxy-policy-changed.json` yet, and authorization-policy has no external API
fixtures under `backend/internal/contracts/fixtures/api/v1`.

## 3. Source References

- `backend/internal/services/authorizationpolicy/spec.go`
- `backend/internal/services/authorizationpolicy/roles.go`
- `backend/internal/services/authorizationpolicy/helpers.go`
- `backend/internal/services/authorizationpolicy/handler.go`
- `backend/internal/services/authorizationpolicy/spec_test.go`
- `backend/internal/contracts/api_fixtures_test.go`
- `backend/internal/contracts/event_envelope_test.go`
- `backend/internal/contracts/fixtures/api/v1/*.json`
- `backend/internal/contracts/fixtures/events/v1/*.json`
- `backend/internal/services/imageregistry/api_fixtures_test.go`
- `backend/internal/services/workload/api_fixtures_test.go`
- `docs/acceptance/gap-analysis.md`
- `problem.md`
- `docs/acceptance/ga-acceptance-trace-matrix.md`

## 4. Assumptions

- Branch is `storage-data-path`.
- The new fixtures are local/static contract artifacts, not live route proofs.
- `ProxyPolicyChanged` stays in `authorizationpolicy.Spec().Events`; no event
  name change is needed.
- Existing route metadata is correct and should be tested, not modified.
- Root `problem.md` is the requested problem ledger; there is no
  `docs/acceptance/problem.md` in this checkout.
- `docs/acceptance/ga-acceptance-trace-matrix.md` should not be edited unless a
  Reviewer explicitly requires it; if touched, typed API contract coverage must
  remain `Open`.

## 5. Non-Goals

- No runtime handler changes.
- No route or `Spec()` metadata changes unless a direct test mismatch proves the
  current metadata is wrong and Reviewer approves the adjustment.
- No repository, persistence, migration, seed, or transaction changes.
- No deploy, Kubernetes, Helm, Terraform, or CI manifest changes.
- No live/staging evidence claims.
- No OpenAPI generation work.
- No expansion to proxy policies, services, assignments, role users, or raw
  permission policy routes.
- No closure of typed API contract coverage.

## 6. Current Behavior

- `authorizationpolicy.Spec()` declares proxy role create/update/delete routes
  as admin authenticated external `/api/v1` routes.
- `roles.go` emits `ProxyPolicyChanged` through `proxyPolicyEvent()` for role
  create/update/delete mutations.
- Shared API fixture validation has an explicit expected fixture list and route
  metadata map in `backend/internal/contracts/api_fixtures_test.go`.
- Shared event envelope validation has an explicit expected fixture list and
  event type/producer map in `backend/internal/contracts/event_envelope_test.go`.
- There are no authorization-policy external API fixtures in
  `backend/internal/contracts/fixtures/api/v1`.
- There is no
  `backend/internal/contracts/fixtures/events/v1/proxy-policy-changed.json`.
- There is no
  `backend/internal/services/authorizationpolicy/api_fixtures_test.go`.

## 7. Target Behavior

- The shared API fixture set includes and validates:
  - `authorization-policy-create-proxy-role.json`
  - `authorization-policy-update-proxy-role.json`
  - `authorization-policy-delete-proxy-role.json`
- The shared event fixture set includes and validates:
  - `proxy-policy-changed.json`
- Authorization-policy service-local tests load the three API fixtures and
  compare them to `authorizationpolicy.Spec()` route and event metadata.
- Acceptance wording records only local/static fixture coverage for these proxy
  role admin routes and the `ProxyPolicyChanged` envelope.
- Typed API contract coverage remains `Open`.

## 8. Affected Domains

- `authorization-policy-service`: proxy role admin API contract fixtures and
  service-local parity tests.
- Shared contracts: external REST fixture validation and event envelope fixture
  validation.
- Acceptance docs: local/static evidence wording only.

## 9. Affected Files

Add:

- `backend/internal/contracts/fixtures/api/v1/authorization-policy-create-proxy-role.json`
- `backend/internal/contracts/fixtures/api/v1/authorization-policy-update-proxy-role.json`
- `backend/internal/contracts/fixtures/api/v1/authorization-policy-delete-proxy-role.json`
- `backend/internal/contracts/fixtures/events/v1/proxy-policy-changed.json`
- `backend/internal/services/authorizationpolicy/api_fixtures_test.go`

Update:

- `backend/internal/contracts/api_fixtures_test.go`
- `backend/internal/contracts/event_envelope_test.go`
- `docs/acceptance/gap-analysis.md`
- `problem.md`

Avoid unless explicitly required:

- `docs/acceptance/ga-acceptance-trace-matrix.md`

## 10. API / Contract Changes

No runtime API changes.

Add these static external REST fixture contracts:

- `authorization-policy.create_proxy_role`
  - fixture: `authorization-policy-create-proxy-role.json`
  - owner: `authorization-policy-service`
  - surface/consumer: `external_rest` / `authenticated-user-client`
  - resource/action: `authorization-policy-service:proxy_roles` / `create`
  - method/path: `POST /api/v1/admin/proxy-rbac/roles`
  - auth: `user`, `auth_required: true`, `service_key_required: false`
  - path parameters: `[]`
  - required request fields: `name`, `display_name`
  - optional request fields: `id`, `description`
  - success statuses: `[201]`
  - error statuses: include `400`, `401`, `403`, `409`, `500`
  - emits events: `["ProxyPolicyChanged"]`
  - response example: public proxy role fields such as `id`, `name`,
    `display_name`, `description`, `is_system`, `created_at`, `updated_at`

- `authorization-policy.update_proxy_role`
  - fixture: `authorization-policy-update-proxy-role.json`
  - owner: `authorization-policy-service`
  - surface/consumer: `external_rest` / `authenticated-user-client`
  - resource/action: `authorization-policy-service:proxy_roles` / `update`
  - method/path: `PUT /api/v1/admin/proxy-rbac/roles/{id}`
  - auth: `user`, `auth_required: true`, `service_key_required: false`
  - path parameters: `["id"]`
  - required request fields: `display_name`
  - optional request fields: `description`
  - success statuses: `[200]`
  - error statuses: include `400`, `401`, `403`, `404`, `500`
  - emits events: `["ProxyPolicyChanged"]`
  - response example: public proxy role fields such as `id`, `name`,
    `display_name`, `description`, `is_system`, `created_at`, `updated_at`

- `authorization-policy.delete_proxy_role`
  - fixture: `authorization-policy-delete-proxy-role.json`
  - owner: `authorization-policy-service`
  - surface/consumer: `external_rest` / `authenticated-user-client`
  - resource/action: `authorization-policy-service:proxy_roles` / `delete`
  - method/path: `DELETE /api/v1/admin/proxy-rbac/roles/{id}`
  - auth: `user`, `auth_required: true`, `service_key_required: false`
  - path parameters: `["id"]`
  - required/optional request fields: `[]`
  - request example: `{}`
  - success statuses: `[200]`
  - error statuses: include `401`, `403`, `500`
  - emits events: `["ProxyPolicyChanged"]`
  - response example: `{}`

Add event envelope fixture `proxy-policy-changed.json`:

- `schema_version`: `1`
- `event_type`: `ProxyPolicyChanged`
- `producer`: `authorization-policy-service`
- `aggregate_id`: stable fake proxy role ID, for example
  `proxy-role-ga-auditor`
- payload should be secret-free and public-contract shaped, for example:
  `id`, `name`, `display_name`, `description`, `is_system`, and `action`
  with an action value from handler behavior such as `role_update`.

Update shared expected maps:

- `backend/internal/contracts/api_fixtures_test.go`
  - add the three filenames to `want`;
  - add route map entries matching owner/resource/action/method/path above.
- `backend/internal/contracts/event_envelope_test.go`
  - add `proxy-policy-changed.json` to `want`;
  - add `"ProxyPolicyChanged": "authorization-policy-service"` to `wantTypes`.

## 11. Database / Migration Changes

None.

## 12. Configuration Changes

None.

## 13. Observability Changes

None.

The event fixture documents an existing event shape only. Do not add logs,
metrics, traces, outbox behavior, or event emission code.

## 14. Security Considerations

- Preserve admin-only route expectations in service-local tests.
- Preserve authenticated-user posture:
  `auth: user`, `auth_required: true`, `service_key_required: false`.
- Fixtures must use fake IDs and no secrets, tokens, cookies, credentials,
  database IDs, internal IDs, or live hostnames.
- Event payload must avoid fields rejected by the envelope validator, including
  secret/token/password/internal ID shaped keys.
- Do not claim live authorization enforcement or PDP behavior from static
  fixture tests.

## 15. Implementation Steps

1. Add the three API fixture JSON files using existing fixture schema and
   formatting.
2. Add `proxy-policy-changed.json` using the existing event envelope schema and
   a payload aligned with `proxyPolicyEvent()` role mutation payloads.
3. Update `backend/internal/contracts/api_fixtures_test.go` expected filename
   list and `wantRoutes` map for the three new fixtures.
4. Update `backend/internal/contracts/event_envelope_test.go` expected filename
   list and `wantTypes` map for `ProxyPolicyChanged`.
5. Add `backend/internal/services/authorizationpolicy/api_fixtures_test.go`.
6. In the service-local test, use a small table for the three fixtures and load
   each JSON fixture directly from
   `../../contracts/fixtures/api/v1/<fixture>.json`, matching existing service
   test patterns.
7. For each fixture, find the matching route in `Spec().Routes` and assert:
   contract name, owner service, resource, action, method, path, auth fields,
   path parameters, required fields, success status, emitted event, response
   example presence/empty-delete semantics, route `Admin`, route
   `StateChanging`, route `IDParam`, and no service-key requirement.
8. Assert `authorizationpolicy.Spec().Events` includes `ProxyPolicyChanged`.
9. Update `docs/acceptance/gap-analysis.md` with one concise local/static
   paragraph for proxy role create/update/delete fixtures and the
   `ProxyPolicyChanged` envelope. Keep all live behavior disclaimers explicit.
10. Update root `problem.md` with matching concise local/static wording.
11. Do not edit `docs/acceptance/ga-acceptance-trace-matrix.md`; if a Reviewer
    requires a trace note, leave typed API contract coverage `Open`.
12. Run `gofmt` on the new Go test file.

## 16. Verification Plan

Focused checks:

- `cd backend && go test ./internal/contracts -run 'ExternalAPI|EventEnvelope'`
- `cd backend && go test ./internal/services/authorizationpolicy -run 'ExternalAPI|Spec'`
- `cd backend && go test ./internal/services/authorizationpolicy/...`
- `git diff --check`

Broader checks, time permitting:

- `cd backend && go test ./...`
- `cd backend && go build ./...`
- `cd backend && make coverage`
- `cd backend && make ci-sonar`

If coverage or SonarScanner is skipped, the Code Agent must say so and must not
claim Quality Gate evidence. If SonarScanner runs, report the actual Quality
Gate result.

## 17. Rollback Plan

Remove the three authorization-policy API fixtures, remove
`proxy-policy-changed.json`, revert the shared expected-list/map updates, remove
`backend/internal/services/authorizationpolicy/api_fixtures_test.go`, and revert
the wording updates in `docs/acceptance/gap-analysis.md` and `problem.md`.

No data rollback is required because there are no migrations, config changes,
deploy changes, or runtime behavior changes.

## 18. Risks and Tradeoffs

- Fixture payloads could imply live admin authorization behavior. Keep wording
  local/static and route-contract focused.
- The event fixture covers one representative `ProxyPolicyChanged` envelope,
  not all role actions. The API fixtures cover event name linkage for create,
  update, and delete.
- A broad helper in the service test could duplicate shared contract validation.
  Keep the test focused on `authorizationpolicy.Spec()` parity.
- Existing delete handler currently returns `200` with no response data; the
  delete fixture should model an empty response example rather than inventing a
  deletion body.

## 19. Reviewer Checklist

- Confirms scope is limited to fixture JSON, fixture validators, one
  service-local test file, and local/static doc wording.
- Confirms no runtime handlers, routes, repositories, migrations, deploy
  manifests, or live evidence claims changed.
- Confirms the three API fixture names and metadata match the route contract in
  `authorizationpolicy.Spec()`.
- Confirms service-local tests assert admin-only, state-changing, user-auth,
  no-service-key posture.
- Confirms `ProxyPolicyChanged` exists in the event fixture list/type map and
  in `authorizationpolicy.Spec().Events`.
- Confirms event payload uses fake public fields and passes forbidden-key
  validation.
- Confirms typed API contract coverage remains `Open`, and
  `ga-acceptance-trace-matrix.md` is unchanged unless explicitly justified.
- Confirms focused contracts and authorizationpolicy tests ran, or skipped
  broader commands are explicitly reported.
- Confirms any SonarScanner Quality Gate claim is backed by an actual run.

## 20. Status

Status: Approved
