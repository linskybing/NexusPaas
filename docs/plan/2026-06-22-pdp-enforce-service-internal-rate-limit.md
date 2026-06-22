# PDP Enforce Service-Internal Rate Limit Fix

Status: Approved

Date: 2026-06-22

## Requirement

Close the live k6 blocker where `/api/v1/projects` passes preflight but fails under 100 VU because the gateway remote PDP calls to `authorization-policy-service` hit `/api/v1/permissions/enforce` rate limits.

## Evidence

- 100-key Project preflight passed with exact `get_org-project-service_api_v1_projects` operation scope.
- k6 100 VU / 10s result: `/healthz` and `/readyz` were 100% 2xx, `/api/v1/projects` had 199 2xx and 67 4xx, 0 direct 429.
- `authorization-policy-service` logs during the same window showed `/api/v1/permissions/enforce` returning 429 for `authorization-policy-client`.
- Current source shows `/api/v1/permissions/enforce` is `PolicyBypass` but not `ServiceInternal`, and the middleware applies `ratelimit` after authn/service-auth and before policy.

## Scope

Affected files:

- `backend/internal/platform/policy_remote.go`
- `backend/internal/platform/middleware.go`
- `backend/internal/platform/ratelimit_test.go`
- `backend/internal/platform/policy_remote_test.go` if no current coverage exists
- `backend/internal/services/authorizationpolicy/permissions.go`
- `backend/internal/services/authorizationpolicy/spec.go`
- `backend/internal/services/authorizationpolicy/spec_test.go`
- `backend/scripts/perf/core-read-100vu.js`
- `sonar-project.properties`
- `docs/plan/2026-06-22-perf-live-project-list-policy-drill.md`
- `gap.md`
- `problem.md`

## Plan

1. Make remote PDP service-auth compatible.
   - Send the configured remote PDP API key in `X-Service-Key`.
   - Keep `X-API-Key` during the transition so mixed old/new deployments continue to work during rolling update.

2. Mark `/api/v1/permissions/enforce` as service-internal.
   - Change the route option from `PolicyBypass()` to `ServiceInternal()`.
   - Preserve policy bypass semantics while requiring service-to-service auth.
   - Update the enforce handler to accept a request that already carries the configured service-to-service key, without requiring RemotePDP to synthesize user principal headers.

3. Exclude service-authenticated internal routes from the general user/IP rate limiter.
   - Add a small predicate around the `ratelimit` guard.
   - Only skip when `route.ServiceAuthRequired` is true; public/user routes remain limited.

4. Add focused tests.
   - Assert `RemotePDP` sends `X-Service-Key` and preserves `X-API-Key`.
   - Assert service-authenticated internal routes bypass the limiter after service-auth succeeds.
   - Assert ordinary authenticated routes still receive 429 when the limiter denies.
   - Assert authorization-policy spec marks `/api/v1/permissions/enforce` as `ServiceAuthRequired`, `PolicyBypass`, and not `AuthRequired`.
   - Update existing authorization-policy enforce workflow tests that currently call the route with only `X-API-Key`.

5. Verify locally and live.
   - `cd backend && go test ./internal/platform ./internal/services/authorizationpolicy -count=1`
   - `make -C backend coverage`
   - `make -C backend ci-sonar`
   - Roll out the affected backend image/manifests if the local change is part of the active live deployment flow.
   - Repeat the 100-key k6 Project drill with exact operation scope.
   - Check `authorization-policy-service` logs/metrics for the drill window and confirm `/api/v1/permissions/enforce` no longer emits 429.
   - Confirm cleanup: temporary DB policies removed, gateway/org-project Secrets restored, no port-forward remains.

6. Update checklists.
   - Record the exact k6 result and log finding in the perf drill plan.
   - Update `gap.md` and `problem.md` with pass/fail state based on the live k6 result.

## Non-Goals

- Do not raise global external user rate limits.
- Do not disable rate limiting for normal authenticated user routes.
- Do not change PDP policy tuple semantics.
- Do not introduce a new authorization engine or cache in this slice.
- Do not exclude runtime backend service code from coverage to satisfy Sonar; only the k6 performance harness may be excluded from coverage accounting while staying in source analysis.

## Security

The enforce endpoint becomes stricter for internal use because it requires `X-Service-Key`. The compatibility `X-API-Key` header is retained only to avoid rolling-update breakage with older authorization-policy pods; remove it in a later cleanup once all supported deployments use `ServiceInternal` enforce.

## Rollback

Revert the four code areas and redeploy. The previous behavior is fail-closed and safe but will continue to fail the 100 VU PDP hot path under current limits.

## Checklist

- [x] Live failure source identified as authorization-policy-service enforce 429.
- [x] Reviewer approves this revised plan.
- [x] Code Agent implements only this plan.
- [x] Local targeted tests pass.
- [x] Live k6 100 VU drill rerun.
- [x] Drill plan, `gap.md`, and `problem.md` updated.
- [x] Reviewer approves implementation.

## Implementation Evidence

- Updated `RemotePDP` to send `X-Service-Key` and retain `X-API-Key` for rolling upgrade compatibility.
- Marked `/api/v1/permissions/enforce` as service-internal in the authorization-policy service spec.
- Updated `enforcePermission` to accept a configured service-to-service key without synthesized user principal headers.
- Added `rateLimitApplies` so `ServiceAuthRequired` routes bypass the general user/IP limiter after service-auth succeeds.
- Added/updated focused tests for RemotePDP headers, service-auth limiter bypass, enforce route metadata, and service-key enforce handling.
- Kept the k6 harness in Sonar source analysis and excluded `backend/scripts/perf/**` only from coverage accounting because it is live load-test tooling, not runtime service code.
- Local targeted test passed:
  `cd backend && go test ./internal/platform ./internal/services/authorizationpolicy -count=1`.
- Expanded targeted test passed:
  `cd backend && go test ./internal/platform ./internal/services/authorizationpolicy ./internal/services -count=1`.
- Full coverage passed:
  `make -C backend coverage`, total statement coverage `82.1%`.
- SonarScanner Quality Gate passed:
  `new_coverage=81.8`, `new_violations=0`, `new_security_hotspots_reviewed=100.0`, `new_duplicated_lines_density=0.5217`.
- Built and pushed image:
  `localhost:5000/nexuspaas-backend:ci-ga-pdp-enforce-20260622094936`.
- Rolled all 15 live backend deployments to the same image; each reported `1/1` ready.
- Live k6 drill passed:
  `auth_key_count=100`, `total_requests=3102`, `total_failure_rate=0`, total p95 `3.135ms`.
- Endpoint results:
  `/healthz` `1000` requests / `1000` 2xx / p95 `0.818ms`;
  `/readyz` `1000` requests / `1000` 2xx / p95 `1.283ms`;
  `/api/v1/projects` `1000` requests / `1000` 2xx / `0` 4xx / `0` 5xx / `0` 429 / p95 `3.668ms`.
- Observability check passed:
  `authorization_policy_enforce_429_since_start=0`.
- Cleanup passed:
  `deleted_count=100`, `post_delete_count=0`, gateway/org-project runtime Secrets restored to `API_KEYS=1` and `API_KEY_PRINCIPALS=1`, no temporary DB policies remained, and no port-forward process remained.
- Final Reviewer Agent implementation review approved with no blocking fixes.
  Low-priority follow-ups recorded: remove temporary `X-API-Key` compatibility on the enforce path after all supported deployments use `ServiceInternal`, and keep future middleware changes tightly scoped.
