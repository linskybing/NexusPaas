# Web GUI OIDC Browser Login

## 1. Objective

Close the WEB-001 OIDC browser-login gap for the first-party `/ui/` GUI by using the existing Dex authorization-code flow, issuing the existing HttpOnly identity session cookie on callback, and proving the dashboard loads without manually entering an API key.

## 2. Background

The current GUI is live at `/ui/`, but it only connects after the user manually enters an admin API key. The backend already has Dex proxy routes, identity session cookies, and remote identity session authorization. Dex is already deployed in the k3s manifests with a public `platform` client and callback URI.

Dex documentation confirms local users plus `oauth2.passwordConnector` can issue password-grant tokens, but Dex also documents password grant as deprecated and not recommended for OAuth2 best practice. This slice will not put user passwords into the GUI. It will use browser redirect authorization code flow through the existing Dex `/auth` endpoint instead.

## 3. Source References

- `ac.md`
- `docs/acceptance/gap-analysis.md`
- `gap.md`
- `problem.md`
- `frontend/src/App.tsx`
- `frontend/src/api.ts`
- `frontend/tests/e2e/dashboard.spec.ts`
- `backend/internal/services/identity/oidc.go`
- `backend/internal/services/identity/oidc_dex.go`
- `backend/internal/services/identity/auth.go`
- `backend/internal/platform/auth.go`
- `backend/deploy/k3s/dex.yaml`
- `backend/deploy/k3s/production-beta/runtime-config.yaml`
- `backend/deploy/k3s/production-beta/backend-units.yaml`
- Dex docs: `https://dexidp.io/docs/connectors/local/`
- Dex OAuth2 docs: `https://dexidp.io/docs/configuration/oauth2/`

## 4. Assumptions

- Live browser proof can use `http://localhost:8080/ui/` because the checked-in Dex client allows `http://localhost:8080/api/v1/oidc/callback`.
- The backend can start browser OIDC through `/api/v1/oidc/start`, generate a five-minute (`Max-Age=300`) OIDC state value, store it in an HttpOnly `Path=/`, `SameSite=Lax`, secure-when-HTTPS cookie, and redirect to Dex; the callback must verify and clear that cookie before issuing a session.
- `DEX_URL` is non-secret runtime config and must stay externalized.
- Existing platform session-cookie authorization remains the source of truth after callback; no custom token parsing or new auth library is introduced in this slice.
- The GUI can keep API-key connect as a fallback for operators while adding OIDC as the normal browser path.

## 5. Non-Goals

- No password-grant GUI login.
- No PKCE hardening in this slice.
- No new OIDC client-management UI.
- No new WebRPC/tRPC/gRPC transport.
- No WebRTC, real workload log tailing, GPU telemetry, Harbor scan lifecycle, SBOM, signing, or load/perf work.
- No decoded Kubernetes Secret values in logs, docs, tests, or final output.

## 6. Current Behavior

- `/ui/` renders the operations dashboard shell.
- Dashboard API calls send `X-API-Key` only after the user enters an admin API key.
- `requestJSON` does not send browser credentials.
- `/api/v1/oidc/authorize` proxies to Dex when `DEX_URL` is configured.
- `/api/v1/oidc/callback` currently validates `code` and `state`, then returns provider-unavailable.
- Platform authorization already accepts `token` cookies for protected routes and remote identity auth can validate sessions across deployable units.

## 7. Target Behavior

- The GUI shows an OIDC sign-in action that navigates to `/api/v1/oidc/start`.
- `/api/v1/oidc/start` generates random state, stores it in a five-minute (`Max-Age=300`) HttpOnly `Path=/`, `SameSite=Lax`, secure-when-HTTPS cookie, and redirects the browser to `/api/v1/oidc/authorize`.
- The identity service proxies Dex browser paths under `/dex/{path...}` so Dex redirects, login form posts, static assets, `Set-Cookie`, `Location`, and `Cookie` semantics stay reachable through the same gateway origin.
- Dex authenticates the user and redirects to `/api/v1/oidc/callback`.
- The callback verifies the state cookie, exchanges the authorization code at Dex `/token`, maps the returned OIDC user to an existing active identity user, issues the existing identity session/refresh cookies, clears the state cookie, and redirects to `/ui/?auth=oidc`.
- GUI API requests include same-origin browser credentials and `/ui/?auth=oidc` enables dashboard loading without a manually entered API key.
- If OIDC is not configured, existing fail-closed behavior remains.

## 8. Affected Domains

- Identity service: OIDC callback and cookie issuance.
- Platform auth: reuse existing session-cookie authorization only.
- Platform gateway/Web GUI: browser sign-in path.
- Production beta config: non-secret Dex runtime value.
- E2E: live browser login evidence.

## 9. Affected Files

- `backend/internal/services/identity/oidc.go`
- `backend/internal/services/identity/oidc_dex.go`
- `backend/internal/services/identity/spec.go`
- `backend/internal/services/identity/workflow_test.go`
- `backend/internal/services/identity/handler_test.go` if callback proxy coverage needs a small assertion
- `backend/deploy/k3s/production-beta/runtime-config.yaml`
- `frontend/src/api.ts`
- `frontend/src/api.test.ts`
- `frontend/src/App.tsx`
- `frontend/src/App.test.tsx`
- `frontend/src/useDashboardData.ts`
- `frontend/tests/e2e/dashboard.spec.ts`
- `docs/acceptance/gap-analysis.md`
- `gap.md`
- `problem.md`

## 10. API / Contract Changes

- `GET|POST /api/v1/oidc/callback` changes from fail-closed stub to a configured-provider callback.
- `GET /api/v1/oidc/start` becomes a public browser start route that sets the HttpOnly state cookie and redirects to Dex authorize.
- `GET|POST /dex/{path...}` become public browser proxy routes to the configured Dex provider when `DEX_URL` is set, preserve browser-critical `Set-Cookie`, `Location`, and `Cookie` behavior, and fail closed when it is unset.
- On success it returns a redirect with existing `token` and `refresh_token` HttpOnly cookies.
- On missing `DEX_URL`, malformed callback, state mismatch, token-exchange failure, or missing ID token it returns the existing fail-closed OIDC error shape or a 400 invalid-request response.
- Existing API-key and session-cookie contracts remain unchanged.

## 11. Database / Migration Changes

No database schema or migration changes.

## 12. Configuration Changes

- Add non-secret runtime config wherever this deploy profile runs:
  - `DEX_URL`
- Existing local/k3s manifests already wire `DEX_URL`; production-beta runtime config must do the same through the shared runtime configmap.
- Do not add any secret values to configmaps or docs.

## 13. Observability Changes

- Add bounded warning logs for token-exchange failures without tokens, authorization codes, cookies, passwords, or response bodies.
- Existing request IDs, trace IDs, access logs, and auth paths remain unchanged.

## 14. Security Considerations

- Use authorization-code flow instead of GUI password grant.
- Bind the callback with backend-generated random one-time state stored in an HttpOnly SameSite cookie, and clear it after callback handling.
- Store only existing server-issued session cookies in the browser; no localStorage/sessionStorage token persistence.
- Keep SameSite=Lax and Secure behavior aligned with existing cookie helpers.
- Do not log OIDC codes, tokens, cookie values, auth headers, or decoded secret values.
- Fail closed when Dex is not configured or token exchange fails.

## 15. Implementation Steps

1. Add a small OIDC callback exchange helper in `identity/oidc.go` using stdlib `net/http`, `net/url`, and `encoding/json`.
2. Add `/api/v1/oidc/start` in identity service to generate state, set the HttpOnly state cookie, and redirect to `/api/v1/oidc/authorize`.
3. Reuse existing identity session issuance and cookie helper style to set `token`/`refresh_token`, clear the OIDC state cookie, and redirect to `/ui/?auth=oidc`.
4. Add a minimal `/dex/{path...}` browser proxy in `oidc_dex.go` and identity route metadata so gateway-origin Dex login pages work while preserving response/request headers needed by Dex browser sessions.
5. Add `credentials: "same-origin"` in the frontend request path so session cookies authorize same-origin API calls.
6. Add an OIDC sign-in button/link in `App.tsx` that simply navigates to `/api/v1/oidc/start`; no frontend state cookie generation.
7. Let the dashboard hook run when `window.location.search` contains `auth=oidc`, without sending fake API keys.
8. Add unit tests for start route, callback success/failure, Dex browser proxy header fidelity, and frontend credential behavior.
9. Add or extend Playwright live E2E so `NEXUSPAAS_E2E_AUTH_MODE=oidc` proves `/ui/` loads after OIDC login without entering an API key.
10. Add runtime config keys to production-beta config.
11. Update `gap.md`, `problem.md`, and `docs/acceptance/gap-analysis.md` with exact evidence after verification.

## 16. Verification Plan

- `go -C backend test ./internal/services/identity -count=1`
- `npm --prefix frontend test`
- `npm --prefix frontend run build`
- `go -C backend test -tags integration ./... -coverprofile=coverage.out -count=1`
- `bash backend/scripts/ci-security-gate.sh sonar`
- Build and push a new backend image containing the frontend bundle.
- Roll `platform-gateway` and `iam-unit`/`identity-service` as needed.
- Live E2E:
  - port-forward gateway on local `8080`;
  - use Playwright against `http://localhost:8080/ui/`;
  - drive Dex login using test credentials supplied through environment variables, without printing them;
  - verify callback rejects missing/mismatched state;
  - verify `token` and `refresh_token` cookie names are present without logging values;
  - verify dashboard service table, projects/workloads/images/usage panels load through cookie-authenticated API calls;
  - verify browser storage contains no API key or token.
- Playwright command:
  - `NEXUSPAAS_E2E_AUTH_MODE=oidc NEXUSPAAS_E2E_APP_PATH=http://localhost:8080/ui/ NEXUSPAAS_E2E_OIDC_USERNAME=<redacted> NEXUSPAAS_E2E_OIDC_PASSWORD=<redacted> npm --prefix frontend run e2e`

Recorded evidence (2026-06-22 Asia/Taipei):

- `go -C backend test ./internal/platform ./internal/services/identity -count=1` passed.
- `go -C backend test -tags integration ./... -coverprofile=coverage.out -count=1` passed.
- `npm --prefix frontend test -- src/api.test.ts src/App.test.tsx` passed.
- `npm --prefix frontend run build` passed.
- `bash backend/scripts/ci-security-gate.sh sonar` passed with Sonar Quality Gate `PASSED`.
- Backend image `localhost:5000/nexuspaas-backend:ci-ga-web-oidc-20260621203712` was pushed and rolled to live `identity-service` and `platform-gateway`; both Pods reported image digest `sha256:a4cc30f2f8b6b8b949c47949de186023ba61a8ad78ea52e72b927e17ea1d670b`.
- Live gateway probe on `http://localhost:8080/api/v1/oidc/start` returned `302`, a browser-visible callback `http://localhost:8080/api/v1/oidc/callback`, and an HttpOnly `SameSite=Lax` OIDC state cookie without printing cookie values.
- Live Playwright E2E passed with `NEXUSPAAS_E2E_AUTH_MODE=oidc` against `http://localhost:8080/ui/`: Dex login reached `/ui/?auth=oidc`, `token` and `refresh_token` cookie names existed, local/session storage contained no API key or token, and the dashboard Services, Outbox, Projections, Projects, Workloads, Images, Usage, and OpenAPI panels loaded through cookie-authenticated API calls.
- Live fixture note: the matching Dex user was first registered as an identity user through `POST /api/v1/register`; its role was then promoted for the admin-only dashboard proof without printing password, token, or secret values.

## 17. Rollback Plan

- Roll deployments back to the previous image digest.
- Revert the runtime-config additions if needed.
- Existing API-key GUI path remains available during and after rollback.

## 18. Risks and Tradeoffs

- Authorization-code without PKCE is acceptable for this bounded server-side callback slice because the code is exchanged by the backend and the browser only receives HttpOnly server session cookies; PKCE should be added before broader public-client hardening.
- Live proof depends on the Dex static redirect URI matching `localhost:8080`; use port-forward `8080` for this slice.
- This closes browser OIDC login for the first-party GUI, not external enterprise IdP federation UX.

## 19. Reviewer Checklist

- [x] Requirement fit: WEB-001 OIDC browser login is directly addressed.
- [x] Scope remains limited to identity callback, GUI entry, runtime config, tests, and ledgers.
- [x] SOLID: OIDC exchange stays in identity service; platform auth remains unchanged.
- [x] 12-Factor: Dex URL is config, not code.
- [x] Security: no password grant, no browser token storage, no secret logging.
- [x] Tests/build/Sonar/live E2E evidence are recorded before completion.
- [x] `gap.md`, `problem.md`, and `docs/acceptance/gap-analysis.md` are updated accurately, not falsely cleared.

## 20. Status

Status: Approved
