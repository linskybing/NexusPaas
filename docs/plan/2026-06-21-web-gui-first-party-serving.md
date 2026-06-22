# Web GUI First-Party Serving Amendment

Status: Implemented; Reviewer approved

Reviewer: Boole approved after root Docker context, CI, WEB_UI_DIR, CORS, and rollout revisions.

Implementation evidence (2026-06-21):

- Added `WEB_UI_DIR` runtime config with `/app/web` production default.
- Added platform-gateway `/ui` and `/ui/` static serving with index fallback and
  missing-asset 404 behavior covered by `backend/internal/platform/web_ui_test.go`.
- Switched `backend/Dockerfile` to repo-root context with a Node/Vite frontend
  build stage and runtime `/app/web` copy.
- Updated root `.dockerignore`, local CI gate Docker build calls, and GitHub
  backend quality gate image build context/path filters.
- Rolled live `deployment/platform-gateway` to
  `localhost:5000/nexuspaas-backend:ci-ga-web-ui-20260621011708`
  (`sha256:ad47efe0716606a009bd40cbedc43445a6c7e5259582361b1215c83fc949af16`).
- Verified live `/ui/` HTML references `/ui/assets/...`, JS asset returns 200,
  disallowed `Origin: http://127.0.0.1:4173` still has no
  `Access-Control-Allow-Origin`, and same-origin Playwright E2E passed.

## Objective

Serve the built NexusPaaS Web GUI from the platform gateway under a first-party
path so browser E2E can exercise the GUI and operational API calls from the same
origin without weakening production CORS policy.

This amends `docs/plan/2026-06-21-web-gui-foundation-live-e2e.md` after live
E2E found that direct `127.0.0.1:4173` to gateway API calls are blocked by the
intended CORS allowlist behavior.

## Scope

- Add a platform runtime static UI handler for `/ui` and `/ui/`.
- Keep operational API endpoints protected by the existing auth/admin gates.
- Keep the UI shell public; the GUI still requires an admin API key in memory to
  read privileged operational data.
- Build frontend assets into the backend image with a multi-stage Docker build.
- Switch the backend image build context to the repository root so the Dockerfile
  can copy both `backend/` and `frontend/` sources.
- Update CI/security image build calls and root `.dockerignore` so the new build
  path is reproducible without sending `frontend/node_modules`, local `dist`,
  coverage/build caches, or local secrets in the Docker context.
- Make the React app default to same-origin API calls when served from `/ui`.
- Update Playwright E2E to hit the first-party `/ui` path on the live gateway
  and use an empty API base URL.
- Update tests and evidence only for this first-party serving slice.

## Non-Goals

- No broad backend CORS relaxation.
- No OIDC browser login.
- No WebRPC transport layer.
- No Kubernetes ingress/domain work.
- No claim that `WEB-001..007` are complete.

## Affected Files

- `backend/internal/platform/endpoints.go`
- `backend/internal/platform/config.go`
- `backend/internal/platform/config_test.go`
- `backend/internal/platform/endpoints_test.go` or a focused platform test file
- `backend/Dockerfile`
- `backend/scripts/ci-security-gate.sh`
- `.dockerignore`
- `.github/workflows/backend-quality-gate.yml`
- `frontend/src/api.ts`
- `frontend/src/App.tsx`
- `frontend/src/App.test.tsx`
- `frontend/tests/e2e/dashboard.spec.ts`
- `frontend/playwright.config.ts`
- `docs/plan/2026-06-21-web-gui-first-party-serving.md`
- `docs/plan/2026-06-21-web-gui-foundation-live-e2e.md`
- `gap.md` after evidence is reviewed

## Design

- Serve a runtime static asset directory with Go standard-library `http.FileServer`
  behavior plus an `index.html` fallback. The image build copies `frontend/dist`
  into `/app/web`; local tests can point `WEB_UI_DIR` at a temp fixture.
- Serve `/ui/` static assets and route `/ui` to `/ui/`.
- Return `index.html` for missing `/ui/*` paths so client-side routing can be
  added later without another backend change.
- Keep API requests same-origin by allowing an empty API base URL in the GUI
  client. The fetch path remains `/healthz`, `/service-registry`, and related
  operational endpoints.
- The Docker build uses the existing Node/Vite toolchain to produce `frontend/dist`
  and copies those static assets into the runtime image. Runtime remains the
  existing Go service process plus static files; no extra sidecar or Node server
  is introduced.
- `backend/Dockerfile` will be built with the repo root as context. Existing CI
  image-build callers must change from `docker build <backend-dir>` to
  `docker build -f <backend-dir>/Dockerfile <repo-root>`.
- The GitHub backend quality gate image scan must use `context: .` and
  `file: backend/Dockerfile`, and its path filters must treat `frontend/**`,
  `frontend/package-lock.json`, root `.dockerignore`, and backend Dockerfile
  changes as image-scan triggers.
- `WEB_UI_DIR` defaults to `/app/web` so the production container serves the
  copied frontend bundle without an extra env var. If the directory or
  `index.html` is missing, `/ui` returns `404` rather than falling back to a fake
  UI; tests use a temp directory fixture to prove success and missing-asset
  behavior.

## Security Considerations

- No secrets are embedded in frontend assets or Docker layers.
- `NEXUSPAAS_E2E_API_KEY` remains Playwright runtime-only and is never exposed as
  a `VITE_` variable.
- Playwright automatic trace/screenshot/video remain disabled; the explicit
  dashboard screenshot is captured only after the password input is cleared.
- Serving the UI shell is not authorization by itself. Privileged data still
  requires API key/JWT auth and admin authorization through existing endpoints.

## SOLID / 12-Factor Notes

- The static UI handler stays isolated from API auth logic and does not special
  case individual frontend files.
- Build output is produced at build time; runtime config remains environment
  driven through `WEB_UI_DIR` and API base input/defaults.
- Same-origin serving keeps the production deployment model simple and avoids
  environment-specific CORS code paths.

## Verification Plan

```sh
npm --prefix frontend run test
npm --prefix frontend run build
go -C backend test ./internal/platform -run 'UI|Endpoint|Operational' -count=1
go -C backend test ./... -count=1
bash backend/scripts/ci-security-gate.sh quick
docker build -f backend/Dockerfile -t localhost:5000/nexuspaas-backend:<tag> .
docker push localhost:5000/nexuspaas-backend:<tag>
kubectl -n nexuspaas set image deployment/platform-gateway app=localhost:5000/nexuspaas-backend:<tag>
kubectl -n nexuspaas rollout status deployment/platform-gateway
kubectl -n nexuspaas port-forward svc/platform-gateway 18080:80
NEXUSPAAS_E2E_API_KEY=<admin-key> npm --prefix frontend run e2e
```

CORS regression:

```sh
curl -i -X OPTIONS \
  -H 'Origin: http://127.0.0.1:4173' \
  -H 'Access-Control-Request-Method: GET' \
  -H 'Access-Control-Request-Headers: x-api-key' \
  http://127.0.0.1:18080/service-registry
```

The response must not include `Access-Control-Allow-Origin` unless that origin
is explicitly allowed by deployment config. The same-origin `/ui` flow must pass
without changing `ALLOWED_ORIGINS`.

## Acceptance

- `GET /ui` redirects or serves the GUI entry without authentication failure.
- `GET /ui/` returns the GUI HTML from the live platform gateway.
- Playwright opens `http://127.0.0.1:18080/ui/`, clears the API key input after
  connect, reads live operational data via same-origin requests, and writes the
  reviewed screenshot artifact.
- Backend and frontend regression gates remain green.
- Reviewer confirms that CORS policy was not weakened and remaining Web GUI ACs
  stay tracked.
