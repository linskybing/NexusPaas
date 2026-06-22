# Gateway Adapter Route Proxy Precedence Slice

## 1. Objective

Fix the platform-gateway catalog proxy path so downstream service routes that
declare an external adapter still proxy to the owning service instead of being
short-circuited by gateway-local adapter preflight.

This should let live seeded Web GUI E2E create a real image build through
`image-registry-service` and prove `GET /api/v1/projects/{id}/image-builds`
returns the seeded build.

## 2. Background

The no-code image-registry rollout slice kept `build_count=0`. A direct live
probe showed:

- `POST /api/v1/images/build` through `platform-gateway` returned `HTTP 200`,
  but the body did not contain a build id or project id.
- `GET /api/v1/projects/{id}/image-builds` returned an empty list.
- Build logs returned an adapter/degraded-looking envelope.

Source inspection found the route-dispatch issue. `serveGatewayCatalogProxyRoute`
sets the catalog route action to `gateway_proxy`, but leaves
`RouteSpec.ExternalAdapter` intact. `handleRoute` checks external adapters before
dispatching actions unless `route.Action == "proxy"`, so gateway-local adapter
preflight returns before `handleGatewayProxy` can forward the request to
`image-registry-service`.

## 3. Source References

- `gap.md`
- `docs/acceptance/gap-analysis.md`
- `docs/plan/2026-06-21-image-build-live-list-evidence.md`
- `docs/plan/2026-06-21-web-gui-active-project-live-e2e.md`
- `backend/internal/platform/app.go`
- `backend/internal/platform/proxy.go`
- `backend/internal/platform/routing_test.go`
- `backend/internal/services/imageregistry/handler.go`
- `backend/internal/services/imageregistry/spec.go`
- `frontend/tests/e2e/dashboard.spec.ts`

## 4. Assumptions

- Gateway catalog proxy should always forward to the owning service once a route
  is selected for `gateway_proxy`.
- External adapter checks still belong in the owning service process for routes
  it handles locally.
- The seeded E2E should only mark image build creation successful when the API
  response proves a real build row was created.

## 5. Non-Goals

- No new gateway routing framework.
- No new image build API.
- No Harbor adapter implementation.
- No fake build row endpoint.
- No broad route refactor or shared abstraction.
- No claim that full image-build GA is complete.

## 6. Current Behavior

For a non-local catalog route such as `POST /api/v1/images/build`, the
platform-gateway selects the image-registry catalog route and changes its action
to `gateway_proxy`. Because the route still has `ExternalAdapter=harbor`,
`handleRoute` performs gateway-local adapter preflight and returns a degraded
`HTTP 200` envelope before calling `handleGatewayProxy`.

The E2E helper treats that `HTTP 200` as image build creation success, even
though no build row exists.

## 7. Target Behavior

- `gateway_proxy` routes skip gateway-local external adapter preflight and
  execute `handleGatewayProxy`.
- Existing local service routes still perform adapter preflight before generic
  actions when no custom handler exists.
- Seeded E2E records image build creation only when the build response contains
  a build id and project id matching the seeded Project.
- Live seeded E2E records `build_count>=1` when the routed image build is
  created by `image-registry-service`.

## 8. Affected Domains

- Platform-gateway catalog proxy dispatch.
- Image-registry live build evidence through the gateway.
- Web GUI seeded E2E evidence quality.
- Acceptance ledgers.

## 9. Affected Files

- `backend/internal/platform/app.go`
- `backend/internal/platform/routing_test.go`
- `frontend/tests/e2e/dashboard.spec.ts`
- `docs/plan/2026-06-21-gateway-adapter-route-proxy-precedence.md`
- `docs/plan/2026-06-21-image-build-live-list-evidence.md`
- `gap.md`
- `docs/acceptance/gap-analysis.md`

## 10. API / Contract Changes

None. Existing REST/OpenAPI routes and response bodies are preserved. The
gateway should forward the existing request to the existing owning service.

## 11. Database / Migration Changes

None.

## 12. Configuration Changes

No source config change. Live verification requires rolling `platform-gateway`
to the fixed backend image.

## 13. Observability Changes

No runtime logging or metric changes. Test/live evidence should record statuses,
route proof, and cleanup without logging API keys.

## 14. Security Considerations

- Do not forward stripped identity headers; keep existing gateway proxy header
  allow-list unchanged.
- Keep existing auth and RBAC checks in platform gateway and downstream service.
- Do not log secrets.
- Do not bypass adapter checks for local owning-service route handling; only
  skip gateway-local preflight for `gateway_proxy`.

## 15. Implementation Steps

- [x] In `handleRoute`, skip external adapter preflight when
  `route.Action == gatewayProxyAction`.
- [x] Add a routing test proving a gateway-catalog route with
  `ExternalAdapter=harbor` proxies to the downstream service instead of returning
  adapter degraded data.
- [x] Add a routing test proving a local owning-service route with
  `ExternalAdapter` and a non-`gateway_proxy` action still performs adapter
  preflight as before.
- [x] Tighten seeded E2E optional image build handling so degraded/adapter
  responses without a matching build id/project id are recorded as rejected, not
  as created.
- [x] Run focused platform and frontend E2E helper tests.
- [x] Record the current live `platform-gateway` image before rollout.
- [x] Build and roll `platform-gateway` to a fixed image, then run seeded live
  E2E.
- [x] Update ledgers honestly.

## 16. Verification Plan

Focused:

```sh
go -C backend test ./internal/platform -run 'GatewayCatalogProxy|Route' -count=1
npm --prefix frontend run test
```

Regression:

```sh
go -C backend test ./... -count=1
go -C backend test ./... -coverprofile=coverage.out -count=1
bash backend/scripts/ci-security-gate.sh quick
bash backend/scripts/ci-security-gate.sh sonar
npm --prefix frontend run build
git diff --check
```

If SonarScanner configuration or credentials are unavailable, record the exact
failure mode as `Not Run` in the plan and ledgers. Final GA completion remains
pending until required Sonar Quality Gate evidence is available.

Live:

```sh
kubectl -n nexuspaas get deploy platform-gateway -o jsonpath='{.spec.template.spec.containers[0].image}'
docker build -t localhost:5000/nexuspaas-backend:<tag> -f backend/Dockerfile .
docker push localhost:5000/nexuspaas-backend:<tag>
kubectl -n nexuspaas set image deploy/platform-gateway app=localhost:5000/nexuspaas-backend:<tag>
kubectl -n nexuspaas rollout status deploy/platform-gateway --timeout=180s
NEXUSPAAS_E2E_API_KEY=<runtime-key> NEXUSPAAS_E2E_SEED_PROJECT=true NEXUSPAAS_E2E_APP_PATH=/ui/ npm --prefix frontend run e2e
```

## 17. Rollback Plan

Revert this slice's platform/E2E/doc changes and roll `platform-gateway` back to
the image recorded before rollout, then verify
`kubectl -n nexuspaas rollout status deploy/platform-gateway --timeout=180s`.

## 18. Risks and Tradeoffs

- This is a one-condition dispatch fix, not a routing refactor. It deliberately
  leaves adapter preflight behavior unchanged for local service routes.
- If live `build_count` remains `0` after this fix, stop and record the new raw
  gateway/downstream evidence before changing image-registry code.

## 19. Reviewer Checklist

| Check | Status |
|---|---|
| Requirement fit: image build list live evidence | Passed |
| Gateway proxy does not run local adapter preflight | Passed |
| Local adapter route behavior preserved | Passed |
| Existing REST/OpenAPI only | Passed |
| No DB/migration/config change | Passed |
| Header/security behavior unchanged | Passed |
| Focused tests | Passed |
| Live seeded E2E | Passed |
| Sonar Quality Gate | Passed |
| Ledger accuracy | Passed |
| Diff scope | Passed |

## 20. Status

Status: Final implementation approved

Implementation evidence:

- Focused backend:
  `go -C backend test ./internal/platform -run 'GatewayCatalogProxy|Route' -count=1`
  passed.
- Frontend unit/helper tests: `npm --prefix frontend run test` passed.
- Regression passed:
  `go -C backend test ./... -count=1`,
  `go -C backend test ./... -coverprofile=coverage.out -count=1`,
  `bash backend/scripts/ci-security-gate.sh quick`,
  `bash backend/scripts/ci-security-gate.sh sonar`,
  `npm --prefix frontend run build`, and `git diff --check`.
- SonarScanner Quality Gate: `PASSED`.
- Rollback baseline before rollout:
  `localhost:5000/nexuspaas-backend:ci-ga-web-image-usage-20260621013315`,
  pod imageID
  `localhost:5000/nexuspaas-backend@sha256:d6bc9f703e57d52301d6d286eb1b42b8f8f3a906727e0c08c333be4fea834bfb`.
- Built and pushed:
  `localhost:5000/nexuspaas-backend:ci-ga-gateway-proxy-adapter-20260621054757`,
  digest
  `sha256:3cda2888dda836a1cd197c476c31342dd7e2f6f6befe5fa7e785ab46d13bc700`.
- Rolled `platform-gateway` only; rollout succeeded and pod imageID matched the
  new digest.
- Live seeded E2E passed against `http://127.0.0.1:18080/ui/`.
- Live route proof:
  `project_count=1`, `seeded_project_present=true`, `config_file_count=1`,
  `seeded_config_id=CFG2600005`, `image_count=1`,
  `seeded_image_identifier=e2e-p-mqnd9ypx-fozjbd:nexuspaas-e2e:mqnd9ypx-fozjbd`,
  `build_count=1`, `gpu_status=200`, `gpu_ok=true`.
- Cleanup evidence:
  configfile, image build, project image, project, and group deletes returned
  `HTTP 200`; the image request remains noted as a leftover because no DELETE
  route exists.

Plan Agent checklist:

- [x] Requirement restated.
- [x] Root cause captured from live evidence and source.
- [x] Scope limited to gateway dispatch and E2E evidence validation.
- [x] Existing REST/OpenAPI contract preserved.
- [x] Reviewer Agent approval required before code changes.
- [x] Reviewer Agent approval received.
- [x] Code Agent implementation complete.
- [x] Reviewer Agent final implementation approval received.
