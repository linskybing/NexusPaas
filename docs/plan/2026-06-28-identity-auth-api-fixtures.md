# Identity Auth API Fixtures

## 1. Objective

Close the next local typed API contract coverage gap by adding external REST API
v1 fixtures for these identity-service auth/session entrypoints only:

- `POST /api/v1/register`
- `POST /api/v1/login`
- `POST /api/v1/refresh`
- `POST /api/v1/cli/login`

This is fixture and contract-test coverage only. It must not change runtime
behavior, database schema, or route registration.

## 2. Background

The repo already has a shared external API fixture validator under
`backend/internal/contracts/api_fixtures_test.go` and service-local parity tests
for several services. The current fixture set covers selected request,
image-registry, workload, org-project, scheduler, and storage routes, but not
the identity auth/session entrypoints listed above.

Identity already registers the target routes in
`backend/internal/services/identity/handler.go`, implements their current
behavior in `backend/internal/services/identity/auth.go`, and declares their
route metadata in `backend/internal/services/identity/spec.go`.

## 3. Source References

- `backend/internal/contracts/api_fixtures_test.go`
- `backend/internal/contracts/fixtures/api/v1/*.json`
- `backend/internal/services/identity/handler.go`
- `backend/internal/services/identity/auth.go`
- `backend/internal/services/identity/spec.go`
- `backend/internal/services/identity/workflow_test.go`
- Existing service fixture parity patterns such as
  `backend/internal/services/imageregistry/api_fixtures_test.go`
- `docs/acceptance/gap-analysis.md`
- `gap.md`
- `problem.md`

## 4. Assumptions

- These four auth/session routes are intentionally public external routes in
  `identity.Spec()`, even though they create users, sessions, refresh tokens, or
  API tokens.
- The existing success shapes are the source of truth:
  - register returns `200 OK` with an empty body and emits `UserCreated`.
  - login returns `200 OK` with `token`, `refresh_token`, and `user`.
  - refresh returns `200 OK` with `token` and `refresh_token`.
  - CLI login returns `200 OK` with `token`, `token_id`, `expires_at`, and
    `user`.
- Existing fixture validation may need a narrow allowance for public external
  API fixtures and for successful credential issuance routes that do not emit a
  domain event.
- The user-mentioned `docs/acceptance/gap.md` and
  `docs/acceptance/problem.md` do not exist in this checkout; the matching
  evidence ledgers are root-level `gap.md` and `problem.md`.

## 5. Non-Goals

- No behavior changes.
- No new routes.
- No DB migrations.
- No auth, cookie, token, password, CAPTCHA, LDAP, OIDC, or CLI-login logic
  changes.
- No frontend changes.
- No OpenAPI generation work.
- No expansion to identity user-management, OIDC, CAPTCHA, logout, or API-token
  management routes.

## 6. Current Behavior

- `identity.Register` registers all four target routes.
- `identity.Spec()` declares all four target routes as public external routes.
- Existing workflow tests exercise register, login, refresh-token rotation, and
  CLI login behavior directly.
- The shared external API fixture registry does not list identity auth/session
  fixture files.
- The shared external API validator currently assumes user-authenticated
  external fixtures and rejects state-changing fixtures without `emits_events`
  except read-only GET fixtures.

## 7. Target Behavior

- Four new identity auth/session fixture JSON files exist under
  `backend/internal/contracts/fixtures/api/v1/`.
- The shared fixture registry expects those files and validates their route
  metadata, payload examples, status lists, compatibility flags, and forbidden
  example fields.
- A focused identity service fixture parity test checks those fixtures against
  `identity.Spec()` for method, path, public auth posture, route resource,
  action, path parameters, and relevant event expectations.
- Acceptance evidence docs are updated only if the Code Agent updates evidence.

## 8. Affected Domains

- `identity-service`: auth/session external API contract documentation and
  service-local fixture parity.
- Contract fixtures: shared external REST v1 fixture validation.
- Acceptance evidence docs: optional evidence ledger updates.

## 9. Affected Files

- `backend/internal/contracts/fixtures/api/v1/identity-register.json`
- `backend/internal/contracts/fixtures/api/v1/identity-login.json`
- `backend/internal/contracts/fixtures/api/v1/identity-refresh.json`
- `backend/internal/contracts/fixtures/api/v1/identity-cli-login.json`
- `backend/internal/contracts/api_fixtures_test.go`
- `backend/internal/services/identity/api_fixtures_test.go`
- `docs/acceptance/gap-analysis.md`
- `gap.md`
- `problem.md`

Do not create or modify `docs/acceptance/gap.md` or
`docs/acceptance/problem.md` unless those files are added separately by an
approved docs task.

## 10. API / Contract Changes

Add local/static fixture coverage for existing routes only:

- `POST /api/v1/register`
  - owner: `identity-service`
  - resource: `identity-service:users`
  - action: `create`
  - public auth fixture
  - required request fields: `username`, `password`
  - optional request fields: `email`, `full_name`, `name`
  - success status: `200`
  - event: `UserCreated`
- `POST /api/v1/login`
  - owner: `identity-service`
  - resource: `identity-service:sessions`
  - action: `create`
  - public auth fixture
  - required request fields: `username`, `password`
  - optional request fields: `captcha_id`, `captcha_answer`
  - success status: `200`
  - no new event
- `POST /api/v1/refresh`
  - owner: `identity-service`
  - resource: `identity-service:refresh_tokens`
  - action: `create`
  - public auth fixture
  - required request fields: `refresh_token`
  - success status: `200`
  - no new event
- `POST /api/v1/cli/login`
  - owner: `identity-service`
  - resource: `identity-service:cli_sessions`
  - action: `create`
  - public auth fixture
  - required request fields: `username`, `password`
  - optional request fields: `name`
  - success status: `200`
  - no new event

No runtime API changes are allowed.

## 11. Database / Migration Changes

None.

Existing generic record resources remain unchanged:

- `identity-service:sessions`
- `identity-service:refresh_tokens`
- `identity-service:api_tokens`
- identity user records declared through the existing identity principal
  repository/spec metadata.

## 12. Configuration Changes

None.

## 13. Observability Changes

None.

The plan must not add metrics, logs, traces, or outbox events. Fixture metadata
may document the existing `UserCreated` event for registration only.

## 14. Security Considerations

- Fixtures must not contain real secrets, password hashes, access tokens, refresh
  tokens, API token hashes, cookies, or internal IDs.
- Example token values must be obviously fake and contract-shaped only.
- The shared forbidden-field validation must continue rejecting sensitive keys
  such as passwords and raw access-token field names where applicable. If
  response examples need credential fields because that is the public API, keep
  the allowance narrow to the identity auth/session fixtures and avoid relaxing
  unrelated fixtures.
- Public route fixture support must be scoped so authenticated service fixtures
  still require user auth and no service key.

## 15. Implementation Steps

1. Add the four identity fixture JSON files under
   `backend/internal/contracts/fixtures/api/v1/` using existing fixture style,
   stable fake values, compatibility flags, success statuses, and source-backed
   error status lists.
2. Update `TestExternalAPIFixturesAreValidV1` in
   `backend/internal/contracts/api_fixtures_test.go` to include the four new
   sorted filenames and expected route metadata.
3. Add the smallest validator change needed for public identity auth/session
   fixtures and their existing credential response fields. Keep the allowance
   data-driven or fixture-scoped, not global.
4. Add `backend/internal/services/identity/api_fixtures_test.go` to load the
   four fixtures and compare them with `identity.Spec()` route metadata,
   including public route posture and `UserCreated` only for register.
5. Update `docs/acceptance/gap-analysis.md`, root `gap.md`, and root
   `problem.md` only if evidence is updated by the Code Agent.
6. Run formatting if needed for Go test files only.

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

Remove the four identity fixture JSON files, remove their expected entries from
`backend/internal/contracts/api_fixtures_test.go`, remove
`backend/internal/services/identity/api_fixtures_test.go`, and revert any
acceptance evidence text updates. No data rollback is required because there are
no migrations or runtime behavior changes.

## 18. Risks and Tradeoffs

- Public auth/session fixtures are different from the existing authenticated
  external fixture set, so over-broad validator relaxation could hide real
  fixture mistakes. Keep exceptions narrow.
- Login, refresh, and CLI login return credential-shaped fields by design.
  Fixture validation must document that contract without permitting sensitive
  fields in unrelated fixtures.
- This closes only a local/static typed contract gap. It does not prove live auth
  availability, session security, browser cookie behavior, LDAP/OIDC behavior,
  or full identity GA readiness.

## 19. Reviewer Checklist

- Scope is limited to the four requested identity auth/session entrypoints.
- No production code, DB migrations, routes, or behavior changes are proposed.
- Fixture names are sorted and registered in the shared fixture allowlist.
- Public auth fixture handling is narrowly scoped.
- Credential response field allowances are narrowly scoped to existing identity
  auth/session responses.
- Identity service parity test checks fixture metadata against `identity.Spec()`.
- Acceptance docs, if changed, clearly say this is local/static fixture evidence
  only.
- Verification commands include targeted tests, full backend tests/build,
  coverage, SonarScanner, and diff whitespace checks.

## 20. Status

Status: Approved
