# Authorization Proxy Service Read API Fixtures

## 1. Objective

Add the next local/static typed contract slice for authorization-policy proxy
service read administration:

- `authorization-policy-list-proxy-services.json`
- `authorization-policy-get-proxy-service.json`
- shared external API fixture expected-list and route-map updates
- authorization-policy service-local fixture parity tests against
  `authorizationpolicy.Spec()`
- concise local/static acceptance wording in `docs/acceptance/gap-analysis.md`
  and root `problem.md`

This is contract evidence only. Typed API contract coverage remains `Open`.

## 2. Background

`authorizationpolicy.Spec()` already declares proxy service admin routes:

- `GET /api/v1/admin/proxy-rbac/services`, resource `proxy_services`, action
  `list`, admin.
- `GET /api/v1/admin/proxy-rbac/services/{id}`, resource `proxy_services`,
  action `get`, ID param `id`, admin.
- `POST /api/v1/admin/proxy-rbac/services`, resource `proxy_services`, action
  `create`, admin.

The POST/create route is intentionally excluded from this slice because
`createService` returns `405 Method Not Allowed`; proxy service definitions are
deployment-managed.

`services.go` requires admin access through `requireAdmin` for list/get. List
returns `200` with service rows. Get returns `200` for a found service and
`404` when the service ID is missing. List/get do not emit events, and the
shared external API fixture validator already allows empty `emits_events` for
GET `list` and GET `get` fixtures.

## 3. Source References

- `backend/internal/services/authorizationpolicy/spec.go`
- `backend/internal/services/authorizationpolicy/handler.go`
- `backend/internal/services/authorizationpolicy/services.go`
- `backend/internal/services/authorizationpolicy/helpers.go`
- `backend/internal/services/authorizationpolicy/authorization_policy_repository.go`
- `backend/internal/services/authorizationpolicy/seed_data.go`
- `backend/internal/contracts/api_fixtures_test.go`
- `backend/internal/services/authorizationpolicy/api_fixtures_test.go`
- `backend/internal/contracts/fixtures/api/v1/authorization-policy-create-proxy-role.json`
- `backend/internal/contracts/fixtures/api/v1/authorization-policy-update-proxy-policy.json`
- `backend/internal/contracts/fixtures/api/v1/storage-list-benchmark-records.json`
- `docs/acceptance/gap-analysis.md`
- `problem.md`

## 4. Assumptions

- Branch is `storage-data-path`.
- The prior proxy role and proxy policy fixture slices are already committed.
- The new fixtures are local/static contract artifacts, not live route proofs.
- Existing route metadata and handler behavior are correct and should be
  tested, not modified.
- The fixture schema requires an object-shaped `response_example`; list
  fixtures should follow the existing collection-fixture convention while
  documenting service row fields.
- Root `problem.md` is the requested problem ledger.
- `docs/acceptance/ga-acceptance-trace-matrix.md` stays untouched.

## 5. Non-Goals

- No runtime code changes.
- No route, handler, `Spec()`, repository, seed, or middleware changes.
- No POST/create proxy service fixture.
- No migrations, database model changes, or seed-data edits.
- No event fixture changes.
- No deploy, Kubernetes, Helm, Terraform, or CI manifest changes.
- No GA trace matrix changes.
- No live/staging evidence claims.
- No OpenAPI generation work.
- No expansion to proxy policies, proxy roles, assignments, role users, system
  roles, or raw permission policy routes.
- No closure of typed API contract coverage.

## 6. Current Behavior

- `authorizationpolicy.Spec()` declares the two target GET proxy service routes
  as admin authenticated external `/api/v1` routes.
- `Register` wires list/get custom handlers for those route patterns.
- `listServices` and `getService` both call `requireAdmin`.
- `listServices` returns `200` with proxy service rows from
  `ListProxyServices`.
- `getService` returns `200` with a service row or `404` with
  `service not found`.
- `createService` exists for the POST route but returns `405` because service
  definitions are deployment-managed.
- Shared API fixture validation has an explicit expected fixture list and route
  metadata map.
- Authorization-policy service-local fixture tests currently cover proxy role
  and proxy policy fixture parity, with mutation/event assumptions that need a
  small read-route path.

## 7. Target Behavior

- The shared API fixture set includes and validates:
  - `authorization-policy-list-proxy-services.json`
  - `authorization-policy-get-proxy-service.json`
- Shared route metadata maps those fixtures to the existing admin proxy service
  GET routes.
- Authorization-policy service-local tests cover the two proxy service read
  fixtures alongside the existing role and policy fixtures.
- The service-local read tests verify route parity, admin-only posture,
  authenticated-user/no-service-key posture, path params, GET/non-state-changing
  metadata, success/error statuses, empty event lists, request examples, and
  service row response examples.
- Acceptance wording records only local/static fixture coverage for the proxy
  service read routes.
- Typed API contract coverage remains `Open`.

## 8. Affected Domains

- `authorization-policy-service`: proxy service read API contract fixtures and
  service-local parity tests.
- Shared contracts: external REST fixture validation list and route map.
- Acceptance docs: local/static evidence wording only.

## 9. Affected Files

Add:

- `backend/internal/contracts/fixtures/api/v1/authorization-policy-list-proxy-services.json`
- `backend/internal/contracts/fixtures/api/v1/authorization-policy-get-proxy-service.json`

Update:

- `backend/internal/contracts/api_fixtures_test.go`
- `backend/internal/services/authorizationpolicy/api_fixtures_test.go`
- `docs/acceptance/gap-analysis.md`
- `problem.md`

Do not edit:

- runtime source files
- `backend/internal/contracts/fixtures/events/v1/*`
- `backend/internal/contracts/event_envelope_test.go`
- `docs/acceptance/ga-acceptance-trace-matrix.md`

## 10. API / Contract Changes

No runtime API changes.

Add these static external REST fixture contracts:

- `authorization-policy.list_proxy_services`
  - fixture: `authorization-policy-list-proxy-services.json`
  - owner: `authorization-policy-service`
  - surface/consumer: `external_rest` / `authenticated-user-client`
  - resource/action: `authorization-policy-service:proxy_services` / `list`
  - method/path: `GET /api/v1/admin/proxy-rbac/services`
  - auth: `user`, `auth_required: true`, `service_key_required: false`
  - path parameters: `[]`
  - required/optional request fields: `[]`
  - request example: `{}`
  - success statuses: `[200]`
  - error statuses: `[401, 403, 500]`
  - emits events: `[]`
  - response example: object-shaped collection containing service rows with
    fields such as `id`, `name`, `description`, `category`, `route_path`,
    `api_patterns`, `actions`, `sort_order`, `created_at`, and `updated_at`

- `authorization-policy.get_proxy_service`
  - fixture: `authorization-policy-get-proxy-service.json`
  - owner: `authorization-policy-service`
  - surface/consumer: `external_rest` / `authenticated-user-client`
  - resource/action: `authorization-policy-service:proxy_services` / `get`
  - method/path: `GET /api/v1/admin/proxy-rbac/services/{id}`
  - auth: `user`, `auth_required: true`, `service_key_required: false`
  - path parameters: `["id"]`
  - required/optional request fields: `[]`
  - request example: `{}`
  - success statuses: `[200]`
  - error statuses: `[401, 403, 404, 500]`
  - emits events: `[]`
  - response example: service row with fields such as `id`, `name`,
    `description`, `category`, `route_path`, `api_patterns`, `actions`,
    `sort_order`, `created_at`, and `updated_at`

Update shared expected maps:

- Add both filenames to `backend/internal/contracts/api_fixtures_test.go`
  `want`.
- Add `wantRoutes` entries matching the owner/resource/action/method/path
  details above.

Do not add or reference an event fixture for these GET routes.

## 11. Database / Migration Changes

None.

## 12. Configuration Changes

None.

## 13. Observability Changes

None.

The fixtures document existing no-event read behavior only. Do not add logs,
metrics, traces, outbox behavior, or event emission code.

## 14. Security Considerations

- Preserve admin-only route expectations in service-local tests.
- Preserve authenticated-user posture:
  `auth: user`, `auth_required: true`, `service_key_required: false`.
- Fixtures must use fake/static examples only and no secrets, tokens, cookies,
  credentials, internal IDs, or live hostnames.
- Do not claim live admin authorization enforcement, PDP behavior, or live
  proxy service behavior from static fixture tests.

## 15. Implementation Steps

1. Add the two API fixture JSON files using the existing fixture schema and
   formatting.
2. Use empty `emits_events` arrays for both fixtures.
3. Use empty request examples and empty required/optional request fields for
   both GET fixtures.
4. Use fake proxy service examples based on public deployment-managed service
   row fields from `defaultServices`.
5. Update `backend/internal/contracts/api_fixtures_test.go` expected filename
   list and `wantRoutes` map for the two fixtures.
6. Extend `backend/internal/services/authorizationpolicy/api_fixtures_test.go`
   with a small proxy service read fixture table.
7. Reuse existing fixture loading and route lookup helpers.
8. For read fixture assertions, require `route.Admin`, `route.AuthRequired`,
   no `route.ServiceAuthRequired`, `!route.StateChanging`, `GET` method
   parity, correct `IDParam`, expected statuses, empty events, and response row
   fields.
9. Keep existing proxy role and proxy policy mutation assertions intact.
10. Update `docs/acceptance/gap-analysis.md` with local/static wording for the
    two proxy service read fixtures only.
11. Update `problem.md` with the same scoped status note and keep typed API
    contract coverage `Open`.

## 16. Verification Plan

Code Agent should run:

```bash
go test ./backend/internal/contracts ./backend/internal/services/authorizationpolicy
```

If the repo layout requires running from `backend/`, use:

```bash
cd backend && go test ./internal/contracts ./internal/services/authorizationpolicy
```

Reviewer should verify:

- only the two planned API fixtures were added;
- no POST/create proxy service fixture was added;
- no event fixtures were added or changed;
- no runtime files were changed;
- the shared expected file list and route map include only the two new GET
  fixtures;
- service-local parity tests cover read-route no-event and non-state-changing
  expectations;
- acceptance wording stays local/static and typed API contract coverage remains
  `Open`.

## 17. Rollback Plan

Revert the two fixture files and the four planned updates:

- `backend/internal/contracts/api_fixtures_test.go`
- `backend/internal/services/authorizationpolicy/api_fixtures_test.go`
- `docs/acceptance/gap-analysis.md`
- `problem.md`

No database, config, deploy, event, or runtime rollback is needed.

## 18. Risks and Tradeoffs

- The fixture schema requires object-shaped examples, while the live list
  handler returns service rows; keep wording static and avoid claiming live
  response proof.
- Existing service-local tests are mutation-oriented; changing them too broadly
  could weaken role/policy event assertions. Keep the read path separate and
  small.
- Adding a POST/create fixture would misrepresent behavior because the handler
  returns `405`; keep it excluded.
- Acceptance wording could overstate evidence. Use only local/static typed
  contract language.

## 19. Reviewer Checklist

- [ ] Status is `Draft` before review.
- [ ] Scope is limited to the two GET proxy service API fixtures.
- [ ] POST proxy-service create is explicitly excluded due to 405
      deployment-managed behavior.
- [ ] No runtime code, routes, handlers, `Spec()`, migrations, deploy files,
      event fixtures, or GA matrix files are planned.
- [ ] Fixture metadata matches `authorizationpolicy.Spec()`.
- [ ] Service-local tests verify admin, auth, no service key, GET,
      non-state-changing, ID param, empty events, and statuses.
- [ ] Docs claim only local/static contract coverage.
- [ ] Typed API contract coverage remains `Open`.
- [ ] Verification commands are sufficient for the touched contract/test scope.

## 20. Status

Status: Approved
