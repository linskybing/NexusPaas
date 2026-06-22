# Org Project Static Admin Compatibility Slice

## 1. Objective

Let org-project admin-only handlers accept the already-authenticated static admin
API key principal so live seeded Web GUI E2E can create and clean up test
Groups/Projects through existing public REST routes.

## 2. Background

The active-project live E2E slice attempted `POST /api/v1/groups` through the
live platform gateway with the runtime admin API key. The gateway authenticated
the key and `GET /api/v1/projects` returned `HTTP 200`, but org-project returned
`HTTP 403` with `admin access required` for `POST /api/v1/groups`.

Root cause: org-project `hasAdminPanel` only accepts admin capability from the
identity read model. Other services such as image-registry already accept the
verified request role header. In production/auth-enabled requests the platform
auth middleware strips inbound identity headers before applying static API key
principal headers, so `X-User-Role=admin` is trusted only after authentication.

## 3. Source References

- `frontend/tests/e2e/dashboard.spec.ts`
- `backend/internal/platform/auth.go`
- `backend/internal/platform/middleware.go`
- `backend/internal/services/orgproject/group_helpers.go`
- `backend/internal/services/orgproject/handler_test.go`
- `backend/internal/services/imageregistry/helpers.go`
- `docs/plan/2026-06-21-web-gui-active-project-live-e2e.md`

## 4. Assumptions

- `RequireAuth=true` is the production/live path for the gateway and services.
- In auth-enabled requests, `X-User-Role=admin` is platform-authenticated, not a
  client-supplied header, because `stripInboundIdentityHeaders` runs before auth.
- Non-auth/dev direct handler tests should still require projected identity data
  for admin-panel checks.

## 5. Non-Goals

- No new seed endpoint.
- No direct database writes or migration.
- No change to project membership, quota, image, workload, or usage contracts.
- No weakening of unauthenticated or `REQUIRE_AUTH=false` admin checks.

## 6. Current Behavior

Static admin API key principals can pass gateway/PDP for read routes, but
org-project mutating admin handlers reject them unless the same principal also
exists in org-project's projected identity read model with adminPanel capability.

## 7. Target Behavior

When `app.Config.RequireAuth` is true and the request carries
`X-User-Role=admin` from platform authentication, org-project `hasAdminPanel`
returns true. When `RequireAuth` is false, `X-User-Role=admin` alone remains
insufficient.

## 8. Affected Domains

- Org/project admin authorization compatibility.
- Live Web GUI seeded E2E setup and cleanup.

## 9. Affected Files

- `backend/internal/services/orgproject/group_helpers.go`
- `backend/internal/services/orgproject/handler_test.go`
- `docs/plan/2026-06-21-orgproject-static-admin-compatibility.md`
- `docs/plan/2026-06-21-web-gui-active-project-live-e2e.md`
- `gap.md`
- `docs/acceptance/gap-analysis.md`

## 10. API / Contract Changes

None. Existing REST routes and request headers are preserved.

## 11. Database / Migration Changes

None.

## 12. Configuration Changes

None.

## 13. Observability Changes

No runtime telemetry change. Live E2E and curl evidence should record HTTP
status, seeded IDs, route proof, and cleanup status without logging API keys.

## 14. Security Considerations

- Only trust `X-User-Role=admin` when `RequireAuth=true`.
- Do not accept client-supplied identity headers in unauthenticated mode.
- Keep projected identity adminPanel support unchanged.
- Do not log secrets.

## 15. Implementation Steps

- [x] Add a small helper in org-project to detect platform-authenticated admin
  role headers only when `RequireAuth=true`.
- [x] Call that helper at the start of `hasAdminPanel`.
- [x] Add tests proving `RequireAuth=true` + `X-User-Role=admin` grants admin,
  while `RequireAuth=false` + `X-User-Role=admin` does not.
- [x] Add an integration-style org-project test through `app.ServeHTTP` proving
  a client-spoofed `X-User-Role=admin` without a valid admin static API key
  cannot create/update/delete admin-only org-project resources. Direct helper
  tests alone are insufficient for this spoofing case because the trust boundary
  is the platform middleware chain.
- [x] Rerun focused org-project tests and frontend seeded live E2E.
- [x] Update plan/evidence ledgers honestly.

## 16. Verification Plan

Focused:

```sh
go -C backend test ./internal/services/orgproject -run 'StaticAdmin|Spoofed|OrgIdentityProjection' -count=1
npm --prefix frontend run test
npm --prefix frontend run build
NEXUSPAAS_E2E_API_KEY=<runtime-key> NEXUSPAAS_E2E_SEED_PROJECT=true npm --prefix frontend run e2e
```

Regression:

```sh
go -C backend test ./... -count=1
go -C backend test ./... -coverprofile=coverage.out -count=1
bash backend/scripts/ci-security-gate.sh quick
bash backend/scripts/ci-security-gate.sh sonar
git diff --check
```

## 17. Rollback Plan

Revert this slice's helper/test/doc changes. Static admin API keys would again
need projected identity adminPanel rows for org-project mutations, and seeded
live E2E would remain blocked by `HTTP 403`.

## 18. Risks and Tradeoffs

- Trusting role headers is safe only because the auth middleware strips inbound
  identity headers when auth is required. ServeHTTP-level spoofing tests must
  lock that condition; direct helper tests are not enough for this security
  boundary.
- This keeps service-specific admin checks compatible with platform static
  principals without adding a cross-service identity projection dependency.
- It does not solve broader OIDC/login GA evidence.

## 19. Reviewer Checklist

| Check | Status |
|---|---|
| Requirement fit: unblock live seeded Web GUI E2E | Pass — live `POST /api/v1/groups` succeeds with the runtime admin API key and seeded E2E passes |
| Existing REST/OpenAPI only | Pass — no endpoint, payload, or transport change |
| No DB/migration/config change | Pass |
| Header trust limited to `RequireAuth=true` | Pass — helper and ServeHTTP spoofing tests cover the trust boundary |
| SOLID / service boundary fit | Pass — small org-project authorization compatibility helper; no cross-service projection dependency added |
| 12-Factor compatibility | Pass — behavior follows runtime auth config and does not add hard-coded credentials or environment coupling |
| Focused tests | Pass — org-project static admin, spoofing, and projection tests passed |
| Live seeded E2E | Pass — active Project `/ui/` seeded smoke passed after rollout |
| Sonar Quality Gate | Pass — local SonarScanner Quality Gate passed |
| Ledger accuracy | Pass — WEB/E2E evidence remains partial, not full GA |
| Diff scope | Pass — scoped to org-project auth helper/tests plus evidence docs |

## 20. Status

Status: Approved

Final implementation review: Approved by Reviewer Agent; no blocking findings.

Implementation evidence:

- `backend/internal/services/orgproject/group_helpers.go` now accepts
  `X-User-Role=admin` only when `app.Config.RequireAuth` is true.
- `backend/internal/services/orgproject/handler_test.go` covers helper behavior,
  spoofed-header rejection through `app.ServeHTTP`, and a valid static admin API
  key principal creating a Group through `app.ServeHTTP`.
- Rolled live `org-project-service` image:
  `localhost:5000/nexuspaas-backend:ci-ga-org-static-admin-20260621125516`
  (`sha256:7310012c13eb9ee0667ac3f27eddf839c0d13c8d53b8b1560916762158b61471`).
- Live probe after rollout: `POST /api/v1/groups` through `platform-gateway`
  with the runtime admin API key returned `HTTP 201`; cleanup DELETE returned
  `HTTP 200`.

Verification completed:

```sh
go -C backend test ./internal/services/orgproject -run 'StaticAdmin|Spoofed|OrgIdentityProjection' -count=1
go -C backend test ./internal/services/orgproject -count=1
go -C backend test ./... -count=1
go -C backend test ./... -coverprofile=coverage.out -count=1
bash backend/scripts/ci-security-gate.sh quick
bash backend/scripts/ci-security-gate.sh sonar
npm --prefix frontend run test
npm --prefix frontend run build
NEXUSPAAS_E2E_API_KEY=<runtime-key> NEXUSPAAS_E2E_SEED_PROJECT=true NEXUSPAAS_E2E_APP_PATH=/ui/ npm --prefix frontend run e2e
git diff --check
```

Plan Agent checklist:

- [x] Requirement restated.
- [x] Live blocker captured with concrete HTTP status.
- [x] Existing contract preserved.
- [x] Security condition specified.
- [x] ServeHTTP spoofing test requirement added.
- [x] Reviewer Agent approval received.
- [x] Code Agent implementation complete.
- [x] Reviewer Agent final implementation approval received.
