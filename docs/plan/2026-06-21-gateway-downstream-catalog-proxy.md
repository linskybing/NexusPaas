# Gateway Downstream Catalog Proxy

Status: Implemented; Reviewer approved

Reviewer: Final implementation review approved

## Objective

Make first-party `/ui/` same-origin API calls reach existing backend services
through `platform-gateway`, starting with live `GET /api/v1/projects`.

## Current Evidence

- `org-project-service` already owns `GET /api/v1/projects`.
- Before this slice, live `platform-gateway` returned `405 Method Not Allowed`
  for that path because it only hosted `platform-gateway` routes.
- After the gateway proxy rollout, live `platform-gateway` reached the matched
  downstream route and returned `403` from the existing PDP path; the remaining
  authorization issue was handled in
  `docs/plan/2026-06-21-platform-admin-policy-bootstrap.md`.
- `production-beta` manifests already define `SERVICE_URLS`; single-service
  manifests do not.

## Scope

- Add a small gateway fallback that matches non-local `CatalogRoutes` and
  proxies them to the owning service from `SERVICE_URLS`.
- Enable the fallback only when `SERVICE_NAME=platform-gateway`, and only when
  the route owner is not locally hosted.
- Keep normal local route handling first.
- Proxy only non-internal catalog routes. Never proxy `/internal/*` or
  `/api/v1/internal/*`, regardless of `InternalPublic` or service auth flags.
- Run the matched route through the existing platform middleware before
  proxying: request IDs, CORS, body limits, authn, admin gate, rate limit, and
  policy checks.
- Preserve method, path, raw query, and request body.
- Forward only this header allowlist: `Accept`, `Authorization`,
  `Content-Type`, `Cookie`, `Idempotency-Key`, `Traceparent`, `X-API-Key`,
  `X-Request-ID`, and `X-Trace-ID`.
- Strip hop-by-hop headers and internal identity/service headers, including
  `X-Service-Key`, `X-User-ID`, `X-Username`, `X-User-Role`, `X-Admin`, and
  `X-API-Token-ID`.
- Pass through downstream status, content type, and body without double
  enveloping.
- Use the existing `AdapterTimeout` for the downstream client, bound downstream
  response reads, and return controlled gateway errors for timeout, missing
  upstream URL, or invalid upstream URL.
- Add focused platform tests for:
  - non-local public catalog route is proxied,
  - local routes take precedence over downstream proxying,
  - gateway auth denial does not call upstream,
  - downstream status, content type, and body pass through,
  - internal identity/service headers are stripped,
  - internal service-auth route is not proxied,
  - missing `SERVICE_URLS` keeps the old fallthrough behavior.
- Update single-service gateway manifest to mount
  `production-beta-runtime-config` when present, matching the 8-unit manifest.
- Update `gap.md` and this plan with test/live evidence.

## Non-Goals

- No new WebRPC transport.
- No new backend route or duplicate project handler.
- No OIDC browser login, ConfigFile/job/image/WebRTC/usage GUI work.
- No service-mesh or workload-identity migration in this slice.

## Affected Files

- `backend/internal/platform/app.go`
- `backend/internal/platform/routing.go`
- `backend/internal/platform/proxy.go`
- `backend/internal/platform/routing_test.go`
- `backend/platform-gateway/k8s/deployment.yaml`
- `docs/plan/2026-06-21-gateway-downstream-catalog-proxy.md`
- `gap.md`

## Verification

```sh
go -C backend test ./internal/platform
go -C backend test ./...
bash backend/scripts/ci-security-gate.sh quick
npm --prefix frontend run test
npm --prefix frontend run build
docker build -f backend/Dockerfile -t <image> .
kubectl -n nexuspaas set image deployment/platform-gateway app=<image>
kubectl -n nexuspaas rollout status deployment/platform-gateway --timeout=180s
curl -i -H "X-API-Key: <admin-key>" http://127.0.0.1:18080/api/v1/projects
NEXUSPAAS_E2E_API_KEY=<admin-key> npm --prefix frontend run e2e
git diff --check
```

## Implementation Evidence

- Implemented a `platform-gateway`-only downstream catalog proxy fallback that
  runs matched routes through the existing middleware, keeps local route
  precedence, forwards an explicit request-header allowlist, strips internal
  identity/service headers by omission, skips internal/service-auth routes, and
  bounds downstream response reads.
- Added optional `production-beta-runtime-config` mounting to the single-service
  `platform-gateway` deployment so `SERVICE_URLS` can be supplied consistently.
- Local verification passed:
  - `go -C backend test ./internal/platform`
  - `go -C backend test ./...`
  - `bash backend/scripts/ci-security-gate.sh quick`
  - `npm --prefix frontend run test`
  - `npm --prefix frontend run build`
- First rollout evidence:
  `localhost:5000/nexuspaas-backend:ci-ga-gateway-proxy-20260621015207`
  (`sha256:9b39af369d10fb4d0638984558bb282941e37652422048266042db937e29b4dc`)
  rolled to `nexuspaas/platform-gateway`.
- Live post-proxy evidence: `GET /api/v1/projects` through
  `platform-gateway` changed from `405 Method Not Allowed` to the downstream
  authorization result, proving gateway routing reached `org-project-service`.
- Final live evidence after the policy-bootstrap slice:
  `GET /api/v1/projects` through `platform-gateway` returned `HTTP 200` with
  `{"success":true,"data":[]}` on image
  `localhost:5000/nexuspaas-backend:ci-ga-admin-policy-20260621020259`
  (`sha256:72c7c2ec0284b0aaec2defd277d8bfc56096e66d08724a1fb85333002b2ee38a`).
- Live GUI E2E passed:
  `NEXUSPAAS_E2E_API_KEY=<runtime-secret> npm --prefix frontend run e2e`.
  Screenshot: `frontend/test-results/gui-live-smoke.png`.

## Reviewer Result

- Reviewer found no blocking findings and approved final implementation review.
- Reviewer reran focused regression:
  `go -C backend test ./internal/platform ./internal/services/authorizationpolicy -run 'GatewayCatalogProxy|AdminBootstrap|APIKeyPrincipalNormalized' -count=1`
  and it passed.
- Non-blocking residual risk: bootstrap reconcile currently ignores
  create/delete errors; harden in a later slice if policy-store observability is
  prioritized.

## Acceptance

- Live `GET /api/v1/projects` through `platform-gateway` no longer returns
  `405`.
- Live `/ui/` Project selector can render the successful project API response
  or an authorized empty list.
- Internal service-only routes stay inaccessible through the public gateway.
- Reviewer approves requirement fit, scope, SOLID, 12-Factor config, tests, and
  live E2E evidence.
