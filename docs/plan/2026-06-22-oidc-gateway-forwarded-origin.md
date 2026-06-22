# OIDC Gateway Forwarded Origin

## 1. Objective

Fix the live OIDC browser-login redirect URI when `/ui/` is reached through the platform gateway by forwarding a validated external origin and preserving downstream redirects only for identity OIDC browser routes.

## 2. Background

The OIDC browser-login slice uses `/api/v1/oidc/start` to build a Dex callback URI from the request origin. In split service deployments, the platform gateway proxies that request to the identity service. Without bounded forwarded-origin metadata, identity can see an internal downstream host and generate a callback URI Dex will reject. The same gateway proxy must also return identity/Dex 302 responses to the browser instead of following them server-side, otherwise the browser does not receive the initial state cookie and Location chain.

## 3. Source References

- `docs/plan/2026-06-22-web-gui-oidc-browser-login.md`
- `backend/internal/platform/proxy.go`
- `backend/internal/platform/routing_test.go`
- `backend/internal/services/identity/oidc.go`
- `backend/internal/services/identity/workflow_test.go`
- `backend/deploy/k3s/dex.yaml`

## 4. Assumptions

- The live proof uses the existing Dex client redirect URI `http://localhost:8080/api/v1/oidc/callback`.
- The gateway inbound `Host` header represents the browser-visible authority for port-forward and normal ingress traffic.
- A valid inbound `X-Forwarded-Proto` can be used for TLS-terminated deployments only when it is a single `http` or `https` value.
- Identity should validate forwarded origin values again before using them, because it can be deployed behind other proxies.

## 5. Non-Goals

- No generic forwarded-header or redirect-policy change for all downstream services.
- No new trusted-proxy/IP framework in this slice.
- No OIDC PKCE, client-management UI, Dex manifest rewrite, or redirect URI config change.
- No frontend changes beyond the already approved OIDC E2E update.
- No secret, token, cookie, or password values in logs or docs.

## 6. Current Behavior

- Gateway downstream proxy requests preserve a small allowlist of headers but do not set `X-Forwarded-Host` or `X-Forwarded-Proto`.
- Identity `oidcRedirectURI` reads `X-Forwarded-Host` and `X-Forwarded-Proto` without validating their shape.
- Non-OIDC gateway proxy routes currently receive no forwarded-origin headers from this proxy path.
- Gateway downstream proxy currently uses the default Go redirect behavior, which follows 302 responses before returning to the browser.

## 7. Target Behavior

- Gateway adds forwarded-origin headers only when proxying identity OIDC callback/start/authorize paths under `/api/v1/oidc/`.
- `X-Forwarded-Host` is derived from the inbound request `Host` only when it is a valid host authority.
- `X-Forwarded-Proto` uses a valid single inbound forwarded proto when present; otherwise it derives `https` from TLS and `http` from plaintext.
- Malformed, multi-value, unsupported, or empty forwarded-origin values are ignored instead of propagated.
- Identity uses forwarded host/proto only after the same validation; invalid values fall back to the request host/scheme.
- Gateway preserves 3xx responses from identity OIDC browser routes under `/api/v1/oidc/` and `/dex/`, including `Location` and `Set-Cookie`, instead of following them server-side.
- Ordinary non-OIDC downstream routes keep the current no-forwarded-origin behavior.

## 8. Affected Domains

- Platform gateway: OIDC-specific downstream proxy request metadata.
- Identity service: callback URI origin validation.
- Tests: focused platform gateway and identity OIDC coverage.

## 9. Affected Files

- `backend/internal/platform/proxy.go`
- `backend/internal/platform/routing_test.go`
- `backend/internal/services/identity/oidc.go`
- `backend/internal/services/identity/workflow_test.go`
- `docs/acceptance/gap-analysis.md`
- `gap.md`
- `problem.md`

## 10. API / Contract Changes

- No public API shape changes.
- Internal gateway-to-identity proxied OIDC requests gain validated `X-Forwarded-Host` and `X-Forwarded-Proto` headers.
- Gateway-to-identity OIDC and Dex-browser proxied redirects are returned to the browser with upstream `Location`/`Set-Cookie` headers intact.
- Non-OIDC gateway proxy requests remain unchanged.

## 11. Database / Migration Changes

No database schema or migration changes.

## 12. Configuration Changes

No new configuration. The change uses request metadata already present at the gateway.

## 13. Observability Changes

No new logs are required. Tests must not print cookie, token, or credential values.

## 14. Security Considerations

- Do not forward client-supplied `X-Forwarded-Host`; derive host from the inbound `Host` authority and validate it.
- Accept only single `http` or `https` forwarded proto values; reject multi-value and unsupported schemes.
- Keep forwarded-origin headers scoped to identity OIDC browser paths, not all services.
- Identity revalidates forwarded-origin values before constructing the redirect URI.

## 15. Implementation Steps

1. Add small validation helpers in `identity/oidc.go` for forwarded proto and host authority.
2. Update `oidcRedirectURI` to use validated forwarded values and fall back to request host/scheme on invalid or missing values.
3. Add a small gateway helper in `platform/proxy.go` that detects `identity-service` routes under `/api/v1/oidc/`.
4. Set validated `X-Forwarded-Host` and `X-Forwarded-Proto` only for those OIDC gateway proxy requests.
5. Use a no-follow HTTP client only for identity OIDC browser gateway proxy requests under `/api/v1/oidc/` and `/dex/` so 3xx responses reach the browser.
6. Add identity tests for valid forwarded origin and invalid/missing forwarded values.
7. Add gateway proxy tests proving OIDC routes receive forwarded-origin headers, OIDC redirects are preserved, and ordinary proxy routes do not gain forwarded-origin side effects.
8. Record verification results in the ledgers only after tests and live E2E complete.

## 16. Verification Plan

- `go -C backend test ./internal/services/identity -run 'TestIdentityOIDC|TestDexBrowserProxy|TestOIDCRedirectURI' -count=1`
- `go -C backend test ./internal/platform -run 'TestGatewayCatalogProxy.*Forwarded|TestGatewayCatalogProxy.*OIDCRedirect|TestGatewayCatalogProxy.*DexRedirect|TestGatewayCatalogProxyKeepsLocalRoutePrecedence' -count=1`
- `bash backend/scripts/ci-security-gate.sh sonar`
- Existing OIDC slice verification still owns full frontend, integration, Sonar, image rollout, and live Playwright proof.
- Live E2E must show OIDC login reaches `/ui/?auth=oidc`, session cookie names are present, dashboard panels load, and browser storage has no API key/token persistence.

Recorded evidence (2026-06-22 Asia/Taipei):

- `go -C backend test ./internal/platform -run 'TestGatewayCatalogProxy.*Forwarded|TestGatewayCatalogProxy.*OIDCRedirect|TestGatewayCatalogProxy.*DexRedirect|TestGatewayCatalogProxyKeepsLocalRoutePrecedence' -count=1` passed.
- `go -C backend test ./internal/platform ./internal/services/identity -count=1` passed.
- `go -C backend test -tags integration ./... -coverprofile=coverage.out -count=1` passed.
- `bash backend/scripts/ci-security-gate.sh sonar` passed with Sonar Quality Gate `PASSED`.
- Live `/api/v1/oidc/start` through `platform-gateway` returned the browser-visible callback URI `http://localhost:8080/api/v1/oidc/callback`, proving validated gateway forwarded-origin handling.
- Live `/api/v1/oidc/start` returned `302` with OIDC state cookie and Location intact, and live Playwright OIDC login reached `/ui/?auth=oidc`, proving gateway no-follow behavior for identity OIDC and Dex browser redirects.

## 17. Rollback Plan

Revert the small proxy and redirect URI validation changes. The existing API-key GUI path remains available, and OIDC would return to the previous internal-host callback behavior.

## 18. Risks and Tradeoffs

- This intentionally avoids broad trusted-proxy support; if a future ingress rewrites `Host`, a separate trusted-proxy/config slice may be needed.
- Limiting forwarded-origin metadata to `/api/v1/oidc/` reduces blast radius but does not help unrelated downstream services that may later need external origin awareness.

## 19. Reviewer Checklist

- [x] Requirement fit: unblocks OIDC callback URI generation through gateway-origin `/ui/`.
- [x] Scope is limited to OIDC gateway forwarded origin, OIDC redirect preservation, and identity redirect validation.
- [x] SOLID: proxy origin derivation stays in platform gateway; redirect URI validation stays in identity OIDC code.
- [x] 12-Factor: no hardcoded external origin and no new config.
- [x] Security: no broad forwarded-header propagation, no unsupported proto, no malformed host, no secret logging.
- [x] Tests prove OIDC route behavior, OIDC redirect preservation, and non-OIDC no-side-effect behavior.

## 20. Status

Status: Approved
