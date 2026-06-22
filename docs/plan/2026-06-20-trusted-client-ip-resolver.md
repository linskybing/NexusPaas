# Trusted Client IP Resolver

## 1. Objective

Make identity login failure tracking, captcha gating, and API-token audit events
use the same trusted-proxy client IP resolver as platform rate limiting.

## 2. Background

`problem.md` lists trusted client IP resolution as a P0 GA blocker. The platform
rate limiter already resolves client IPs with `TRUSTED_PROXY_CIDRS`, but identity
still trusts the first `X-Forwarded-For` value directly.

## 3. Source References

- `problem.md`
- `backend/internal/platform/ratelimit.go`
- `backend/internal/platform/client_ip_test.go`
- `backend/internal/services/identity/auth.go`
- `backend/internal/services/identity/api_tokens.go`
- `backend/internal/services/identity/captcha.go`
- `backend/internal/services/identity/workflow_test.go`

## 4. Assumptions

- `TRUSTED_PROXY_CIDRS` remains the single configuration source.
- The existing rightmost-untrusted `X-Forwarded-For` behavior is the desired
  resolver semantics.
- Requests from untrusted remotes must ignore forwarded headers.

## 5. Non-Goals

- No new proxy middleware or ingress controller configuration.
- No database schema change.
- No geo/IP reputation feature.
- No change to static API-key matching or API-token format.

## 6. Current Behavior

Rate limiting uses `clientIPFromRequest(r, cfg.TrustedProxyCIDRs)`. Identity
login lockout and audit helper `requestIP` returns the first `X-Forwarded-For`
hop even when `RemoteAddr` is not trusted.

## 7. Target Behavior

Identity uses the platform resolver with `app.Config.TrustedProxyCIDRs` for
login failure IDs, captcha lookup, login failure audit events, API-token audit
events, and login failure cleanup. Untrusted remotes fall back to `RemoteAddr`.

## 8. Affected Domains

- Platform client IP resolver.
- Identity login lockout/captcha/audit.
- Identity tests for forwarded-header handling.

## 9. Affected Files

- `backend/internal/platform/ratelimit.go`
- `backend/internal/platform/client_ip_test.go`
- `backend/internal/services/identity/auth.go`
- `backend/internal/services/identity/api_tokens.go`
- `backend/internal/services/identity/captcha.go`
- `backend/internal/services/identity/workflow_test.go`

## 10. API / Contract Changes

No external HTTP API shape changes. Operational behavior changes only for
client-IP attribution behind configured trusted proxies.

## 11. Database / Migration Changes

None.

## 12. Configuration Changes

None. Existing `TRUSTED_PROXY_CIDRS` is reused.

## 13. Observability Changes

Audit `source_ip` and login failure `ip` values become consistent with rate
limiting.

## 14. Security Considerations

Untrusted clients must not spoof lockout/audit IPs with `X-Forwarded-For`.
Malformed forwarded hops must be ignored and must not panic.

## 15. Implementation Steps

1. Export the existing platform client-IP resolver as a small helper.
2. Add an app-aware identity helper that calls the platform resolver with
   `*platform.App`.
3. Update identity login, captcha, cleanup, and API-token audit call sites.
4. Update tests to cover trusted proxy, untrusted remote, and malformed header
   behavior.

## 16. Verification Plan

```sh
go -C backend test ./internal/platform -run 'ClientIP|ClientKey|RateLimit' -count=1
go -C backend test ./internal/services/identity -run 'RequestIP|Login|APIToken|Captcha' -count=1
go -C backend test ./internal/services/identity ./internal/platform -count=1
```

Executed on 2026-06-20:

```sh
go -C backend test ./internal/platform -run 'ClientIP|ClientKey|RateLimit' -count=1
go -C backend test ./internal/services/identity -run 'RequestIP|Login|APIToken|Captcha|HelperBranches' -count=1
go -C backend test ./internal/services/identity ./internal/platform -count=1
go -C backend test ./... -count=1
bash backend/scripts/ci-security-gate.sh quick
bash backend/scripts/ci-security-gate.sh sonar
```

Result: all commands passed. SonarScanner reported `QUALITY GATE STATUS:
PASSED` against `http://localhost:9000/dashboard?id=nexuspaas-backend`.

Reviewer follow-up: the platform resolver regression test was moved to
`backend/internal/platform/client_ip_test.go` so this slice can be reviewed
without pulling unrelated historical changes in `ratelimit_test.go` into scope.
The trusted-IP implementation no longer changes
`backend/internal/services/identity/handler_test.go`; the remaining diff in
that file belongs to the already-approved API-token indexed lookup slice. The
focused platform, focused identity, package-level, full backend, quick gate,
and Sonar Quality Gate checks were rerun after this scope cleanup and passed.

Live RKE2 evidence on 2026-06-20:

- Built and pushed
  `localhost:5000/nexuspaas-backend:ci-ga-ip-20260620134150`
  (`sha256:e1da11963ba9f64b2dd448bfd65fe84531a6e3b04339c42936d1746ff3c92ef9`).
- Rolled the image to the 15 NexusPaaS backend deployments in namespace
  `nexuspaas`; each rollout completed and each backend pod reported Ready on the
  new image.
- `identity-service` `/healthz` and `/readyz` returned HTTP 200.
- `platform-gateway` `/healthz` and `/readyz` returned HTTP 200.
- Sent a live `POST /api/v1/login` request to `identity-service` through
  port-forward with `X-Forwarded-For: 198.51.100.77, 203.0.113.88`.
  The request returned HTTP 401 as expected, and the typed `login_failures`
  row stored `ip=127.0.0.1`, `used_spoofed_xff=false`, proving untrusted
  forwarded headers were ignored. The test row was deleted after verification.

## 17. Rollback Plan

Revert this slice. Login lockout and identity audit IP attribution would return
to the previous direct-forwarded-header behavior.

## 18. Risks and Tradeoffs

Deployments that relied on spoofable `X-Forwarded-For` without configuring
`TRUSTED_PROXY_CIDRS` will see RemoteAddr instead. That is the safer GA default.

## 19. Reviewer Checklist

- Identity no longer directly trusts forwarded headers.
- Rate limiting and identity attribution use the same resolver.
- Existing config is reused; no new dependency or abstraction.
- Tests cover trusted and untrusted proxy paths.

## 20. Status

Status: Approved
