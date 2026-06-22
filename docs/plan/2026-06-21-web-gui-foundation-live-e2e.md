# Web GUI Foundation Live E2E

Status: Implemented; Reviewer approved

Reviewer: Boole approved after security/dependency revisions.

Implementation evidence (2026-06-21):

- Added `frontend/` Vite + React + TypeScript operations dashboard.
- Added API client, dashboard data hook, Vitest coverage, and Playwright live
  E2E.
- Live E2E passed against `platform-gateway` served from `/ui/` on RKE2:
  `NEXUSPAAS_E2E_API_KEY=<runtime-only> npm run e2e`.
- Screenshot artifact: `frontend/test-results/gui-live-smoke.png`; inspected
  manually, API key input is blank and no trace/video artifact was produced.
- Regression gates passed: `npm run test`, `npm run build`, `npm audit
  --audit-level=high`, `go -C backend test ./... -count=1`, and
  `bash backend/scripts/ci-security-gate.sh quick`.

## Objective

Add the first first-party NexusPaaS management GUI package and prove it with a
browser E2E smoke against the live RKE2 backend.

This is the Web GUI foundation slice, not full closure of `WEB-001..007`.

## Background

The External GA roadmap keeps a future first-party management Web GUI in scope,
but the repository currently has no frontend package. The backend already
exposes admin operational endpoints (`/service-registry`, `/outbox`,
`/projections`, `/openapi.json`, `/healthz`, `/readyz`) through the same auth,
admin, and policy-bypass gates used by the API surface. These are enough for a
small read-only management/evidence dashboard without adding backend APIs.

Context7 notes used for dependency choices:

- React docs recommend moving data fetching into custom hooks with loading,
  error, and cleanup/race handling.
- Vite docs support `vite build`, `vite preview`, `VITE_` environment variables,
  and `server.proxy` for development API routing.

## Scope

- Add `frontend/` as a Vite + React + TypeScript package.
- Use existing REST/OpenAPI endpoints. Do not add a WebRPC transport layer in
  this slice.
- Build a dense operations dashboard with:
  - connection panel for API base URL and admin API key kept in memory only;
  - health/readiness status;
  - service registry table;
  - redacted outbox table;
  - projection status table;
  - OpenAPI summary counts.
- Add a small API client module and React data hook with loading/error states and
  effect cleanup.
- Add Vitest unit tests for API client/hook behavior where practical.
- Add Playwright E2E that serves the built GUI and talks to a live backend base
  URL provided by env.
- Keep the root `gap.md` Web UI status incomplete; this slice only creates the
  foundation and live browser evidence path.

## Non-Goals

- No OIDC browser login yet.
- No project selection, ConfigFile submission, job management, image listing,
  WebRTC launch, or usage RBAC screens yet.
- No backend API changes.
- No WebRPC/gRPC/tRPC layer.
- No token persistence in localStorage/sessionStorage/cookies.
- No claim that `WEB-001..007` are done.

## Affected Files

- `frontend/package.json`
- `frontend/package-lock.json`
- `frontend/index.html`
- `frontend/vite.config.ts`
- `frontend/tsconfig*.json`
- `frontend/playwright.config.ts`
- `frontend/src/**`
- `frontend/tests/**`
- `docs/plan/2026-06-21-web-gui-foundation-live-e2e.md`
- `gap.md` after Reviewer-approved evidence sync only.

## Dependencies

Runtime dependencies:

- `react`, `react-dom`
- `lucide-react` for accessible toolbar/status icons.

Test/dev dependencies:

- `@vitejs/plugin-react`
- `vite`
- `typescript`
- `vitest`
- `@testing-library/react`
- `@testing-library/jest-dom`
- `jsdom`
- `@playwright/test`

## Security Considerations

- The admin API key input is held only in React component state and sent as the
  `X-API-Key` header; the app must not persist it.
- The admin API key field must use password-style masking and the Playwright
  screenshot must be captured after the key field is cleared or hidden.
- The dashboard consumes redacted `/outbox` data and must not display raw secrets
  or token-like fields.
- All privileged data remains protected by existing backend auth/admin gates.
- CORS/dev proxy config must not weaken production backend policy.

## SOLID / 12-Factor Notes

- API transport, data hooks, and presentational components are separated.
- The frontend reads API base URL from `VITE_API_BASE_URL` or a runtime form
  input; no hardcoded cluster URL.
- `NEXUSPAAS_E2E_API_KEY` is Playwright runtime-only config and must not use a
  `VITE_` prefix or be bundled into the frontend.
- Build and run are separate (`npm run build`, `npm run preview`).
- Logs remain browser/dev-server logs; no hidden sidecar service.

## Verification Plan

Local frontend:

```sh
cd frontend
npm install
npm run test
npm run build
```

Live browser smoke:

```sh
kubectl -n nexuspaas port-forward svc/platform-gateway 18080:80
cd frontend
VITE_API_BASE_URL=http://127.0.0.1:18080 \
NEXUSPAAS_E2E_API_KEY=<admin-key> \
npm run e2e
```

Backend regression gate after adding the frontend package:

```sh
go -C backend test ./... -count=1
bash backend/scripts/ci-security-gate.sh quick
```

## Acceptance

- `npm run build` produces a production frontend bundle.
- Unit tests pass.
- Playwright opens the GUI, authenticates with an in-memory API key, reads live
  health/readiness, service registry, outbox, projections, and OpenAPI summary,
  and records a screenshot artifact.
- Backend tests/quick gate remain green.
- Reviewer verifies this as GUI foundation only, with remaining `WEB-*` ACs still
  tracked.
