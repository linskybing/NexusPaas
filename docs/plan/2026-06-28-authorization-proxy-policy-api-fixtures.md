# Authorization Proxy Policy API Fixtures

## 1. Objective

Add the next local/static typed contract slice for authorization-policy proxy
policy administration:

- three external REST API fixtures for proxy policy create, update, and delete;
- shared contract fixture expected-list/map updates;
- authorization-policy service-local fixture parity tests against
  `authorizationpolicy.Spec()`, alongside the existing proxy role fixture tests;
- concise local/static acceptance wording in `docs/acceptance/gap-analysis.md`
  and root `problem.md`.

This plan is for contract evidence only. Typed API contract coverage remains
`Open`.

## 2. Background

`authorizationpolicy.Spec()` already declares admin-only proxy policy routes:

- `POST /api/v1/admin/proxy-rbac/policies`, resource `proxy_policies`, action
  `create`;
- `PUT /api/v1/admin/proxy-rbac/policies/{id}`, resource `proxy_policies`,
  action `update`, ID param `id`;
- `DELETE /api/v1/admin/proxy-rbac/policies/{id}`, resource `proxy_policies`,
  action `delete`, ID param `id`.

The matching handlers in `policies.go` require admin access and emit
`ProxyPolicyChanged`. Create emits action `create` with the created policy map,
update emits action `update` with payload `{old,new}`, and delete emits action
`delete` with the current policy map. Delete returns `404` when the policy is
missing.

The prior slice already added proxy role create/update/delete fixtures and the
shared `proxy-policy-changed.json` event fixture. This slice should reuse that
event fixture, not add another event fixture.

## 3. Source References

- `backend/internal/services/authorizationpolicy/spec.go`
- `backend/internal/services/authorizationpolicy/policies.go`
- `backend/internal/services/authorizationpolicy/helpers.go`
- `backend/internal/contracts/api_fixtures_test.go`
- `backend/internal/contracts/fixtures/api/v1/authorization-policy-create-proxy-role.json`
- `backend/internal/contracts/fixtures/api/v1/authorization-policy-update-proxy-role.json`
- `backend/internal/contracts/fixtures/api/v1/authorization-policy-delete-proxy-role.json`
- `backend/internal/contracts/fixtures/events/v1/proxy-policy-changed.json`
- `backend/internal/services/authorizationpolicy/api_fixtures_test.go`
- `docs/acceptance/gap-analysis.md`
- `problem.md`

## 4. Assumptions

- Branch is `storage-data-path`.
- The new fixtures are local/static contract artifacts, not live route proofs.
- `ProxyPolicyChanged` remains the correct existing event name for proxy policy
  mutations.
- The existing event fixture `proxy-policy-changed.json` is sufficient event
  envelope coverage for this slice.
- Existing route metadata and handlers are correct and should be tested, not
  modified.
- Root `problem.md` is the requested problem ledger; there is no
  `docs/acceptance/problem.md` in this checkout.
- `docs/acceptance/ga-acceptance-trace-matrix.md` should remain untouched unless
  Reviewer explicitly requires it; if touched, typed API contract coverage must
  remain `Open`.

## 5. Non-Goals

- No runtime handler changes.
- No route or `Spec()` metadata changes unless a direct fixture parity mismatch
  proves current metadata is wrong and Reviewer approves the adjustment.
- No repository, persistence, migration, seed, or transaction changes.
- No deploy, Kubernetes, Helm, Terraform, or CI manifest changes.
- No new event fixture JSON unless the existing `proxy-policy-changed.json`
  cannot be reused.
- No live/staging evidence claims.
- No OpenAPI generation work.
- No expansion to proxy roles, services, assignments, role users, system roles,
  or raw permission policy routes.
- No closure of typed API contract coverage.

## 6. Current Behavior

- `authorizationpolicy.Spec()` declares proxy policy create/update/delete as
  admin authenticated external `/api/v1` routes.
- `policies.go` emits `ProxyPolicyChanged` for proxy policy create, update, and
  delete mutations.
- Shared API fixture validation has an explicit expected fixture list and route
  metadata map in `backend/internal/contracts/api_fixtures_test.go`.
- Authorization-policy service-local fixture tests currently cover the proxy
  role create/update/delete fixtures against `authorizationpolicy.Spec()`.
- There are no proxy policy external API fixtures under
  `backend/internal/contracts/fixtures/api/v1`.
- `backend/internal/contracts/fixtures/events/v1/proxy-policy-changed.json`
  already exists from the prior slice.

## 7. Target Behavior

- The shared API fixture set includes and validates:
  - `authorization-policy-create-proxy-policy.json`
  - `authorization-policy-update-proxy-policy.json`
  - `authorization-policy-delete-proxy-policy.json`
- Shared route metadata maps those fixtures to the existing admin proxy policy
  routes.
- Authorization-policy service-local tests cover these three policy fixtures in
  addition to the existing role fixtures.
- The policy fixture tests verify route parity, admin-only posture,
  authenticated-user/no-service-key posture, path params, state-changing
  metadata, success/error statuses, request examples, response examples, and
  `ProxyPolicyChanged` linkage.
- Acceptance wording records only local/static fixture coverage for these proxy
  policy admin routes and reuses the existing `ProxyPolicyChanged` envelope.
- Typed API contract coverage remains `Open`.

## 8. Affected Domains

- `authorization-policy-service`: proxy policy admin API contract fixtures and
  service-local parity tests.
- Shared contracts: external REST fixture validation.
- Acceptance docs: local/static evidence wording only.

## 9. Affected Files

Add:

- `backend/internal/contracts/fixtures/api/v1/authorization-policy-create-proxy-policy.json`
- `backend/internal/contracts/fixtures/api/v1/authorization-policy-update-proxy-policy.json`
- `backend/internal/contracts/fixtures/api/v1/authorization-policy-delete-proxy-policy.json`

Update:

- `backend/internal/contracts/api_fixtures_test.go`
- `backend/internal/services/authorizationpolicy/api_fixtures_test.go`
- `docs/acceptance/gap-analysis.md`
- `problem.md`

Avoid unless explicitly required:

- `backend/internal/contracts/fixtures/events/v1/proxy-policy-changed.json`
- `backend/internal/contracts/event_envelope_test.go`
- `docs/acceptance/ga-acceptance-trace-matrix.md`

## 10. API / Contract Changes

No runtime API changes.

Add these static external REST fixture contracts:

- `authorization-policy.create_proxy_policy`
  - fixture: `authorization-policy-create-proxy-policy.json`
  - owner: `authorization-policy-service`
  - surface/consumer: `external_rest` / `authenticated-user-client`
  - resource/action: `authorization-policy-service:proxy_policies` / `create`
  - method/path: `POST /api/v1/admin/proxy-rbac/policies`
  - auth: `user`, `auth_required: true`, `service_key_required: false`
  - path parameters: `[]`
  - required request fields: `name`
  - optional request fields: `id`, `description`, `rules`
  - request example: include fake `id`, `name`, `description`, and `rules`
    using `service_id` plus `actions`
  - success statuses: `[201]`
  - error statuses: `[400, 401, 403, 409, 500]`
  - emits events: `["ProxyPolicyChanged"]`
  - response example: public proxy policy fields such as `id`, `name`,
    `description`, `is_system`, `created_at`, `updated_at`, and `rules`

- `authorization-policy.update_proxy_policy`
  - fixture: `authorization-policy-update-proxy-policy.json`
  - owner: `authorization-policy-service`
  - surface/consumer: `external_rest` / `authenticated-user-client`
  - resource/action: `authorization-policy-service:proxy_policies` / `update`
  - method/path: `PUT /api/v1/admin/proxy-rbac/policies/{id}`
  - auth: `user`, `auth_required: true`, `service_key_required: false`
  - path parameters: `["id"]`
  - required request fields: `[]`
  - optional request fields: `name`, `description`, `rules`
  - request example: include a fake updated `description` and replacement
    `rules` using accepted `serviceId` alias plus `actions`
  - success statuses: `[200]`
  - error statuses: `[400, 401, 403, 404, 409, 500]`
  - emits events: `["ProxyPolicyChanged"]`
  - response example: public proxy policy fields such as `id`, `name`,
    `description`, `is_system`, `created_at`, `updated_at`, and `rules`

- `authorization-policy.delete_proxy_policy`
  - fixture: `authorization-policy-delete-proxy-policy.json`
  - owner: `authorization-policy-service`
  - surface/consumer: `external_rest` / `authenticated-user-client`
  - resource/action: `authorization-policy-service:proxy_policies` / `delete`
  - method/path: `DELETE /api/v1/admin/proxy-rbac/policies/{id}`
  - auth: `user`, `auth_required: true`, `service_key_required: false`
  - path parameters: `["id"]`
  - required/optional request fields: `[]`
  - request example: `{}`
  - success statuses: `[200]`
  - error statuses: `[401, 403, 404, 500]`
  - emits events: `["ProxyPolicyChanged"]`
  - response example: `{}`

Update shared expected maps:

- `backend/internal/contracts/api_fixtures_test.go`
  - add the three filenames to `want`;
  - add route map entries matching owner/resource/action/method/path above.

Do not update event fixture maps for this slice unless the existing
`proxy-policy-changed.json` is missing or invalid.

## 11. Database / Migration Changes

None.

## 12. Configuration Changes

None.

## 13. Observability Changes

None.

The API fixtures document existing event linkage only. Do not add logs, metrics,
traces, outbox behavior, or event emission code.

## 14. Security Considerations

- Preserve admin-only route expectations in service-local tests.
- Preserve authenticated-user posture:
  `auth: user`, `auth_required: true`, `service_key_required: false`.
- Fixtures must use fake IDs and no secrets, tokens, cookies, credentials,
  database IDs, internal IDs, or live hostnames.
- Rule examples should use public service IDs and action names only.
- Do not claim live authorization enforcement, PDP behavior, or admin mutation
  behavior from static fixture tests.

## 15. Implementation Steps

1. Add the three API fixture JSON files using the existing fixture schema and
   formatting.
2. Reuse `proxy-policy-changed.json`; do not add another event fixture.
3. Update `backend/internal/contracts/api_fixtures_test.go` expected filename
   list and `wantRoutes` map for the three new fixtures.
4. Update the shared fixture required-request validator so non-no-body request
   fixtures may have an empty `required_request_fields` list only when they
   explicitly declare at least one optional request field and the
   `request_example` contains at least one declared optional field. This models
   partial-update APIs without inventing false required fields.
5. Extend `backend/internal/services/authorizationpolicy/api_fixtures_test.go`
   so it covers both the existing proxy role fixtures and the new proxy policy
   fixtures.
6. Keep the service-local test small: use a table for policy fixtures, load
   each JSON fixture from
   `../../contracts/fixtures/api/v1/<fixture>.json`, find the matching
   `Spec().Routes` entry, and assert parity.
7. For each policy fixture, assert contract name, owner service, resource,
   action, method, path, auth fields, path parameters, required fields, optional
   fields, success statuses, error statuses, emitted event, route `Admin`, route
   `StateChanging`, route `IDParam`, and no service-key requirement.
8. Assert create request examples include non-empty required fields; assert
   update request examples include at least one declared optional field; assert
   create/update rule examples, when present, use valid shapes with at least one
   action.
9. Assert create/update response examples include stable public policy fields
   and rule data; assert delete request/response examples are empty objects.
10. Assert `authorizationpolicy.Spec().Events` includes `ProxyPolicyChanged`.
11. Update `docs/acceptance/gap-analysis.md` with one concise local/static
    paragraph for proxy policy create/update/delete fixtures. State that the
    existing `ProxyPolicyChanged` event fixture is reused.
12. Update root `problem.md` with matching concise local/static wording and add
    the three policy routes to the typed API contracts row.
13. Do not edit `docs/acceptance/ga-acceptance-trace-matrix.md`; if a Reviewer
    requires a trace note, leave typed API contract coverage `Open`.
14. Run `gofmt` on any changed Go test file.

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

Remove the three authorization-policy proxy policy API fixtures, revert the
shared expected-list/map updates, revert the authorization-policy service-local
test extension, and revert the wording updates in
`docs/acceptance/gap-analysis.md` and `problem.md`.

No data rollback is required because there are no migrations, config changes,
deploy changes, event fixture changes, or runtime behavior changes.

## 18. Risks and Tradeoffs

- Fixture payloads could imply live admin authorization behavior. Keep wording
  local/static and route-contract focused.
- The update handler allows partial updates, including optional rule replacement.
  The update fixture must not invent a false required field; it should be one
  representative valid payload with declared optional fields.
- The create handler tolerates omitted `rules`, so `rules` must remain optional
  even though the fixture should include a useful rule example.
- A broad helper in the service test could duplicate shared contract validation.
  Keep the test focused on `authorizationpolicy.Spec()` parity.
- Existing delete handler returns `200` with no response data and `404` when
  missing; the delete fixture should model an empty response example and include
  `404` in error statuses.

## 19. Reviewer Checklist

- Confirms scope is limited to fixture JSON, fixture validators, one
  service-local test update, and local/static doc wording.
- Confirms no runtime handlers, routes, repositories, migrations, deploy
  manifests, event fixture JSON, or live evidence claims changed.
- Confirms the three API fixture names and metadata match the route contract in
  `authorizationpolicy.Spec()`.
- Confirms service-local tests assert admin-only, state-changing, user-auth,
  no-service-key posture.
- Confirms create/update rule examples match `parseRuleInputs` accepted shapes
  (`service_id` or `serviceId` plus non-empty `actions`).
- Confirms create policy required fields match handler behavior (`name` only)
  and update policy does not invent required fields for a partial-update route.
- Confirms shared fixture validation still rejects empty non-body contracts but
  permits explicit partial-update contracts whose examples contain declared
  optional fields.
- Confirms delete includes `404` because `deletePolicy` returns not found for a
  missing policy.
- Confirms `ProxyPolicyChanged` linkage is asserted and the existing event
  fixture is reused rather than duplicated.
- Confirms typed API contract coverage remains `Open`, and
  `ga-acceptance-trace-matrix.md` is unchanged unless explicitly justified.
- Confirms focused contracts and authorizationpolicy tests ran, or skipped
  broader commands are explicitly reported.
- Confirms any SonarScanner Quality Gate claim is backed by an actual run.

## 20. Status

Status: Approved
