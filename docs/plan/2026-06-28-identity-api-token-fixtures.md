# Identity API Token Fixtures

## 1. Objective

Close the next local typed API contract coverage gap by adding external REST API
v1 fixtures for the identity-service API-token lifecycle endpoints only:

- `GET /api/v1/me/api-tokens`
- `POST /api/v1/me/api-tokens`
- `DELETE /api/v1/me/api-tokens/{id}`
- `DELETE /api/v1/me/api-tokens/current`

This is fixture and contract-test coverage only. It must not change runtime
behavior, database schema, route registration, or event ownership.

## 2. Background

The shared external API fixture validator in
`backend/internal/contracts/api_fixtures_test.go` validates static v1 JSON
fixtures under `backend/internal/contracts/fixtures/api/v1/`. Identity already
has fixture coverage for the public auth/session slice, but not for the
authenticated current-user API-token lifecycle routes.

`identity.Spec()` already declares the four target routes. The handlers already
implement list/create/revoke behavior and the create/revoke paths publish
`AuditEvent` at runtime. This slice should document that existing behavior in
fixtures/tests without adding `AuditEvent` to `identity.Spec().Events` or
broadening global event producer ownership unless implementation review proves
the existing repo pattern requires it.

## 3. Source References

- `backend/internal/services/identity/spec.go`
- `backend/internal/services/identity/api_tokens.go`
- `backend/internal/services/identity/auth_repository.go`
- `backend/internal/contracts/api_fixtures_test.go`
- `backend/internal/contracts/fixtures/api/v1/*.json`
- `backend/internal/services/identity/api_fixtures_test.go`
- `docs/acceptance/gap-analysis.md`
- `gap.md`
- `problem.md`
- `docs/plan/2026-06-28-identity-api-token-fixtures.md`

## 4. Assumptions

- The four target routes are authenticated user external REST routes, not
  public routes and not service-key routes.
- Current route metadata in `identity.Spec()` is the source of truth:
  `api_tokens/list`, `api_tokens/create`, `api_tokens/delete`, and
  `api_tokens/delete_current`.
- Current response shapes are the source of truth:
  list returns an array of token metadata; create returns token metadata plus a
  one-time raw `token`; revoke endpoints return `200 OK` with no response body.
- Fixture JSON must keep `response_example` inside the current map-shaped
  fixture schema, so revoke fixtures should use an empty object unless
  implementation review finds an established no-body fixture convention.
- `POST` and `DELETE` fixtures may list existing runtime `AuditEvent` emission,
  but service-local parity should not require `AuditEvent` in
  `identity.Spec().Events` for this slice.

## 5. Non-Goals

- No runtime behavior changes.
- No DB migrations.
- No new routes.
- No changes to `identity.Spec().Routes`.
- No forced addition of `AuditEvent` to `identity.Spec().Events`.
- No global event fixture expansion or audit-compliance producer ownership work.
- No `/api/v1/me/cli-ca` fixture; it returns raw PEM outside the current JSON
  response fixture shape.
- No identity user-management, OIDC, CAPTCHA, logout, or public auth/session
  fixture expansion.

## 6. Current Behavior

- `identity.Spec()` declares:
  - `GET /api/v1/me/api-tokens`
  - `POST /api/v1/me/api-tokens`
  - `DELETE /api/v1/me/api-tokens/{id}`
  - `DELETE /api/v1/me/api-tokens/current`
- `listAPITokens` returns active token metadata with fields such as `id`,
  `name`, `token_prefix`, `expires_at`, `created_at`, and optional
  `last_used_at`.
- `createAPIToken` requires `name`, returns `201 Created`, and includes a
  one-time `token` field.
- `revokeAPIToken` and `revokeCurrentAPIToken` return `200 OK` on success and
  publish existing `AuditEvent` entries.
- The shared fixture registry does not include API-token fixture files.
- The identity service fixture parity test currently covers only the public
  auth/session fixture slice.

## 7. Target Behavior

- Four new identity API-token fixture JSON files exist under
  `backend/internal/contracts/fixtures/api/v1/`.
- The shared external API fixture test expects and validates the four new files.
- The identity service fixture parity test compares the four fixtures against
  `identity.Spec()` route metadata and fixture field expectations.
- Service parity handles existing `AuditEvent` emission as fixture metadata
  without forcing `AuditEvent` into `identity.Spec().Events`.
- `/api/v1/me/cli-ca` remains uncovered by this slice.

## 8. Affected Domains

- `identity-service`: current-user API-token external REST contract fixtures.
- Contract fixtures: shared v1 external REST fixture validation.
- Acceptance evidence docs: small local gap ledger updates only.

## 9. Affected Files

- `backend/internal/contracts/fixtures/api/v1/identity-list-api-tokens.json`
- `backend/internal/contracts/fixtures/api/v1/identity-create-api-token.json`
- `backend/internal/contracts/fixtures/api/v1/identity-revoke-api-token.json`
- `backend/internal/contracts/fixtures/api/v1/identity-revoke-current-api-token.json`
- `backend/internal/contracts/api_fixtures_test.go`
- `backend/internal/services/identity/api_fixtures_test.go`
- `docs/acceptance/gap-analysis.md`
- `gap.md`
- `problem.md`

## 10. API / Contract Changes

Add local/static fixture coverage for existing routes only:

- `identity.list_api_tokens`
  - method/path: `GET /api/v1/me/api-tokens`
  - owner/resource/action: `identity-service` /
    `identity-service:api_tokens` / `list`
  - auth: `user`, `auth_required: true`, `service_key_required: false`
  - request fields: none
  - path parameters: none
  - success status: `200`
  - emits events: none
  - response example: array-compatible token metadata represented in the
    existing fixture schema.
- `identity.create_api_token`
  - method/path: `POST /api/v1/me/api-tokens`
  - owner/resource/action: `identity-service` /
    `identity-service:api_tokens` / `create`
  - auth: `user`, `auth_required: true`, `service_key_required: false`
  - required request fields: `name`
  - path parameters: none
  - success status: `201`
  - emits events: existing runtime `AuditEvent`
  - response fields: `id`, `name`, `token_prefix`, `expires_at`,
    `created_at`, `token`
- `identity.revoke_api_token`
  - method/path: `DELETE /api/v1/me/api-tokens/{id}`
  - owner/resource/action: `identity-service` /
    `identity-service:api_tokens` / `delete`
  - auth: `user`, `auth_required: true`, `service_key_required: false`
  - path parameters: `id`
  - request fields: none
  - success status: `200`
  - emits events: existing runtime `AuditEvent`
  - response example: empty object unless existing fixture conventions support
    no-body success examples.
- `identity.revoke_current_api_token`
  - method/path: `DELETE /api/v1/me/api-tokens/current`
  - owner/resource/action: `identity-service` /
    `identity-service:api_tokens` / `delete_current`
  - auth: `user`, `auth_required: true`, `service_key_required: false`
  - path parameters: none
  - request fields: none
  - success status: `200`
  - emits events: existing runtime `AuditEvent`
  - response example: empty object unless existing fixture conventions support
    no-body success examples.

No live API behavior changes are allowed.

## 11. Database / Migration Changes

None.

The existing identity token storage and `user_api_tokens` table ownership remain
unchanged.

## 12. Configuration Changes

None.

## 13. Observability Changes

None.

The plan documents existing create/revoke `AuditEvent` emission only. It must
not add metrics, logs, traces, outbox event producers, or service event
ownership.

## 14. Security Considerations

- Fixture values must be fake and stable.
- Do not include real API tokens, token hashes, password hashes, cookies, user
  secrets, database IDs, or internal-only identifiers.
- The create response may include a fake one-time raw `token` because the
  existing endpoint returns it; keep any validator allowance scoped to this
  identity API-token create fixture.
- List/revoke fixtures must not expose `token_hash`.
- Keep `auth: user` and `auth_required: true` for all four fixtures.
- Keep `/api/v1/me/cli-ca` out of scope because raw PEM is not represented by
  the current JSON fixture shape.

## 15. Implementation Steps

1. Add the four fixture JSON files under
   `backend/internal/contracts/fixtures/api/v1/` using existing fixture style,
   `schema_version: 1`, `api_surface: external_rest`,
   `consumer: authenticated-user-client`, compatibility flags, and stable fake
   examples.
2. Update `TestExternalAPIFixturesAreValidV1` in
   `backend/internal/contracts/api_fixtures_test.go` to include the four sorted
   filenames and exact expected route metadata.
3. Add only the narrow validator support needed for these fixtures, if tests
   fail because list responses are array-shaped, create returns a raw `token`,
   or successful revoke examples are empty objects.
4. Extend `backend/internal/services/identity/api_fixtures_test.go` with a
   small API-token fixture case table that checks method, path, resource,
   action, auth posture, path parameters, required/optional request fields,
   success statuses, and expected response fields.
5. In the identity parity test, compare `AuditEvent` as fixture metadata only
   for create/revoke and do not require it in `identity.Spec().Events` unless
   the Reviewer identifies an existing required pattern.
6. Update `docs/acceptance/gap-analysis.md`, `gap.md`, and `problem.md` with
   a concise evidence note for this local typed API contract coverage slice.
7. Run `gofmt` only on changed Go test files.

## 16. Verification Plan

- `cd backend && go test ./internal/contracts -run ExternalAPI`
- `cd backend && go test ./internal/services/identity -run ExternalAPI`
- `cd backend && go test ./internal/services/identity`
- `cd backend && go test ./internal/contracts/...`
- `cd backend && go test ./...`
- `cd backend && go build ./...`
- `cd backend && make coverage`
- `cd backend && make ci-sonar`
- `git diff --check`

## 17. Rollback Plan

Remove the four identity API-token fixture JSON files, remove their expected
entries from `backend/internal/contracts/api_fixtures_test.go`, revert the
identity fixture parity test additions, and revert the three evidence doc
updates. No data rollback is required.

## 18. Risks and Tradeoffs

- The current shared fixture struct expects `response_example` as a map, while
  list returns an array and revoke returns no body. The Code Agent should make
  the smallest fixture/test adjustment that preserves existing validation.
- Adding `AuditEvent` to fixture `emits_events` can look like service event
  ownership. Keep parity checks scoped so this slice does not require
  `identity.Spec().Events` changes.
- Over-broad sensitive-field allowances could weaken fixture validation. Any
  allowance for create's fake raw `token` must be identity-create-api-token
  specific.
- Error status lists are documentation examples, not exhaustive runtime tests;
  keep them source-backed and conservative.

## 19. Reviewer Checklist

- Confirms only the four API-token lifecycle endpoints are covered.
- Confirms `/api/v1/me/cli-ca` is not included.
- Confirms no production code, route registration, DB migration, config, or
  runtime behavior change is proposed.
- Confirms fixture metadata matches `identity.Spec()` route metadata.
- Confirms create/revoke `AuditEvent` handling does not force
  `AuditEvent` into `identity.Spec().Events` unless repo pattern requires it.
- Confirms fixture examples avoid secrets and token hashes.
- Confirms verification commands include focused tests, broader backend tests,
  build, coverage, SonarScanner Quality Gate, and diff whitespace check.

## 20. Status

Status: Approved
