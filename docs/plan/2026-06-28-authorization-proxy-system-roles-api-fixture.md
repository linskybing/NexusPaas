# Authorization Proxy System Roles API Fixture

## 1. Objective

Add the next local/static typed contract slice for authorization-policy proxy
system role reads:

- `GET /api/v1/admin/proxy-rbac/system-roles`

This slice adds one external REST API fixture and parity tests only. Typed API
contract coverage remains `Open`.

## 2. Background

`authorizationpolicy.Spec()` already declares an admin-only proxy system roles
read route under `proxy_system_roles`, action `list`, with no route ID param.

`listSystemRoles` requires admin access, reads projected identity role rows via
`policyIdentityRecords(app, r, policyIdentityRoles, rolesResource)`, sorts by
role name, returns `200` with rows, and emits no events. The handler has no
`400` or `404` path; `401`, `403`, and generic fixture-level `500` remain the
expected error statuses.

## 3. Source References

- `backend/internal/services/authorizationpolicy/spec.go`
- `backend/internal/services/authorizationpolicy/roles.go`
- `backend/internal/services/authorizationpolicy/helpers.go`
- `backend/internal/contracts/api_fixtures_test.go`
- `backend/internal/services/authorizationpolicy/api_fixtures_test.go`
- `backend/internal/contracts/fixtures/api/v1/authorization-policy-list-proxy-target-assignments.json`
- `backend/internal/contracts/fixtures/api/v1/authorization-policy-list-proxy-role-users.json`
- `docs/acceptance/gap-analysis.md`
- `problem.md`

## 4. Assumptions

- Branch is `storage-data-path`.
- The code facts supplied by CodeGraph are current for this slice.
- This is local/static fixture parity only, not live route evidence.
- The response row shape should assert only stable public identity role fields,
  especially `id` and `name`.
- Root `problem.md` is the requested problem ledger.

## 5. Non-Goals

- No runtime behavior changes.
- No `Spec()`, handler, repository, migration, deploy, event fixture, event
  envelope, GA matrix, or OpenAPI generation changes.
- No live auth evidence and no kind/e2e cluster evidence.
- No expansion to proxy policies, proxy roles, role users, policy assignments,
  target assignments, services, or raw permission policy routes.
- No closure of typed API contract coverage.

## 6. Current Behavior

- Shared API fixture validation uses an explicit expected fixture list and
  route metadata map in `backend/internal/contracts/api_fixtures_test.go`.
- `backend/internal/services/authorizationpolicy/api_fixtures_test.go` covers
  earlier authorization-policy fixture slices.
- There is no external API fixture for listing proxy system roles.
- The handler has `401`, `403`, `500`, and `200` paths, but no `400` or `404`
  path.

## 7. Target Behavior

- The shared API fixture set includes and validates:
  - `authorization-policy-list-proxy-system-roles.json`
- Shared route metadata maps the fixture to the existing admin proxy system
  roles route.
- Authorization-policy service-local tests verify fixture parity with
  `authorizationpolicy.Spec()`.
- Acceptance wording records only local/static fixture coverage.
- Typed API contract coverage remains `Open`.

## 8. Affected Domains

- `authorization-policy-service`: proxy system roles read API contract fixture
  and service-local parity tests.
- Shared contracts: external REST API fixture validation.
- Acceptance docs: local/static evidence wording only.

## 9. Affected Files

Add:

- `backend/internal/contracts/fixtures/api/v1/authorization-policy-list-proxy-system-roles.json`

Update:

- `backend/internal/contracts/api_fixtures_test.go`
- `backend/internal/services/authorizationpolicy/api_fixtures_test.go`
- `docs/acceptance/gap-analysis.md`
- `problem.md`

Avoid:

- runtime source files
- `backend/internal/contracts/fixtures/events/v1/*`
- `backend/internal/contracts/event_envelope_test.go`
- `docs/acceptance/ga-acceptance-trace-matrix.md`

## 10. API / Contract Changes

No runtime API changes.

Add this static external REST fixture contract:

- `authorization-policy.list_proxy_system_roles`
  - fixture: `authorization-policy-list-proxy-system-roles.json`
  - owner: `authorization-policy-service`
  - surface/consumer: `external_rest` / `authenticated-user-client`
  - resource/action:
    `authorization-policy-service:proxy_system_roles` / `list`
  - method/path: `GET /api/v1/admin/proxy-rbac/system-roles`
  - auth: `user`, `auth_required: true`, `service_key_required: false`
  - path parameters: `[]`
  - route `IDParam`: empty
  - required/optional request fields: `[]` / `[]`
  - request example: `{}`
  - success statuses: `[200]`
  - error statuses: `[401, 403, 500]`
  - emits events: `[]`
  - response example: collection wrapper with `items`, each row containing a
    fake public identity role row with at least `id` and `name`; optional
    public fields such as `description` or `display_name` may be included only
    if consistent with existing projected role fixtures

The fixture must use fake IDs only and compatibility settings consistent with
existing additive-field, tolerant-reader fixtures. Tests should assert only
stable `id` and `name` response fields.

## 11. Database / Migration Changes

None.

## 12. Configuration Changes

None.

## 13. Observability Changes

None.

The fixture documents existing no-event read behavior only. Do not add logs,
metrics, traces, outbox behavior, or event emission code.

## 14. Security Considerations

- Preserve admin-only route expectations in service-local tests.
- Preserve authenticated-user posture:
  `auth: user`, `auth_required: true`, `service_key_required: false`.
- Fixture examples must use fake IDs only and no secrets, tokens, cookies,
  passwords, credentials, internal IDs, or live hostnames.
- Do not claim live authorization enforcement from static fixture tests.

## 15. Implementation Steps

1. Add the API fixture JSON file using the existing fixture schema and
   formatting.
2. Use empty `emits_events`, empty request fields, and `{}` as the request
   example.
3. Use a collection response wrapper with one fake public role row containing
   at least `id` and `name`.
4. Update `backend/internal/contracts/api_fixtures_test.go`:
   - add the filename to the expected sorted fixture list;
   - add the `wantRoutes` entry for owner, resource, action, method, and path.
5. Extend `backend/internal/services/authorizationpolicy/api_fixtures_test.go`
   with one proxy system roles read fixture case.
6. Reuse existing fixture loading and route lookup helpers.
7. Assert contract name, owner, resource, action, method, path, auth fields,
   empty path parameters, empty route `IDParam`, statuses, empty events, admin
   posture, authenticated-user posture, no service-key posture, and
   non-state-changing GET behavior.
8. Assert response examples include stable public role fields `id` and `name`
   only.
9. Do not add an event fixture or event envelope test change.
10. Update `docs/acceptance/gap-analysis.md` with concise local/static wording
    for this system roles read fixture.
11. Update root `problem.md` with matching concise local/static wording and add
    this route to the typed API contracts row while keeping status `Open`.
12. Run `gofmt` on any changed Go test file.

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

- remove `authorization-policy-list-proxy-system-roles.json`;
- remove its shared expected-list and route-map entries;
- remove the service-local system roles fixture test case/helpers added for
  this slice;
- revert matching local/static wording in `docs/acceptance/gap-analysis.md`
  and `problem.md`.

No database, migration, runtime, deploy, event, or configuration rollback is
needed.

## 18. Risks and Tradeoffs

- Static fixtures prove metadata parity with `Spec()`; they do not prove live
  handler behavior.
- The handler intentionally has no `400` or `404` path; adding either would
  overstate current behavior.
- Keeping response assertions to `id` and `name` avoids pinning unstable
  projected identity role fields.

## 19. Reviewer Checklist

- [ ] Status is `Draft`.
- [ ] Scope is limited to one local/static proxy system roles read fixture.
- [ ] Fixture metadata matches the existing `Spec()` route exactly.
- [ ] Error statuses include `401`, `403`, and `500`, and exclude `400` and
      `404`.
- [ ] `emits_events` is empty and no event fixture or event envelope change is
      planned.
- [ ] Response assertions are limited to stable public `id` and `name` fields.
- [ ] No runtime, repository, migration, deploy, GA matrix, OpenAPI, or live
      evidence work is planned.
- [ ] Docs claim only local/static contract coverage.
- [ ] Typed API contract coverage remains `Open`.
- [ ] Verification commands include the requested focused checks and broader
      gates.

## 20. Status

Status: Approved
