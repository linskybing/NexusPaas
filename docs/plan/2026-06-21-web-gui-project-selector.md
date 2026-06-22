# Web GUI Project Selector Slice

Status: Implemented; Reviewer approved

Reviewer: Boole approved the plan before implementation and approved the
implemented slice after final verification.

Implementation evidence (2026-06-21):

- Added project records to the frontend dashboard data path using existing
  `GET /api/v1/projects`.
- Added a Projects panel with active Project selection in component state.
- `npm --prefix frontend run test`: passed, 2 files / 9 tests, including a
  project API 405 regression that keeps the dashboard visible and renders
  "Projects unavailable".
- `npm --prefix frontend run build`: passed.
- Built and rolled live `deployment/platform-gateway` to
  `localhost:5000/nexuspaas-backend:ci-ga-web-projects-20260621013522`
  (`sha256:528f1ed02e9fd9cdc1d7c0c42392f734b54c2f37e2ebf9f80e58f89a50d30a2d`).
- Live Playwright smoke passed against `/ui/`; screenshot
  `frontend/test-results/gui-live-smoke.png` shows the Projects panel rendering
  "Projects unavailable" and a blank API key input.
- Live `GET /api/v1/projects` through the current platform-gateway-only topology
  returned `405 Method Not Allowed`; this proves the selector shell is live, but
  not live project data routing. Keep `WEB-002` partial.

## Objective

Move `WEB-002` from "not started" to a narrow, evidenced partial by letting the
first-party GUI list authorized Projects from the existing Project API and set
an active Project in browser state.

This does not claim full `WEB-002` completion until OIDC browser login and live
multi-service routing evidence prove the same flow for a real user session.

## Scope

- Reuse the existing `GET /api/v1/projects` API. Do not add a new backend route
  or WebRPC transport.
- Extend the frontend API client and dashboard hook to fetch projects after
  connection.
- Add a compact Project selector panel with active Project state.
- Show empty/unavailable states without breaking the existing operations
  dashboard.
- Extend Vitest and Playwright smoke coverage for the selector.
- Update `gap.md` and this plan with evidence after review.

## Non-Goals

- No OIDC browser login in this slice.
- No ConfigFile submission, job actions, image catalog, WebRTC launch, or usage
  views.
- No new typed RPC/WebRPC layer; the approved External GA roadmap says existing
  REST/OpenAPI remains the GUI backend contract until a proven API gap exists.
- No backend data migration or route change.

## Affected Files

- `frontend/src/types.ts`
- `frontend/src/api.ts`
- `frontend/src/useDashboardData.ts`
- `frontend/src/App.tsx`
- `frontend/src/styles.css`
- `frontend/src/api.test.ts`
- `frontend/src/App.test.tsx`
- `frontend/tests/e2e/dashboard.spec.ts`
- `docs/plan/2026-06-21-web-gui-project-selector.md`
- `docs/acceptance/gap-analysis.md`
- `gap.md`

## Security / 12-Factor Notes

- The GUI sends the existing in-memory API key/JWT header through the same API
  client; no token persistence.
- Project visibility remains enforced by the backend API/RBAC path.
- Active Project is component state only.
- Same-origin `/ui/` serving remains the default; `VITE_API_BASE_URL` stays a
  build/runtime input for non-standard smoke setups.

## Verification

```sh
npm --prefix frontend run test
npm --prefix frontend run build
NEXUSPAAS_E2E_API_KEY=<admin-key> npm --prefix frontend run e2e
git diff --check
```

Live E2E may only assert the selector if `platform-gateway` can route
`/api/v1/projects` in the current RKE2 topology. If not, record that as a live
topology gap and keep `WEB-002` partial.

## Acceptance

- Project data is fetched from `/api/v1/projects` through the existing client.
- The GUI renders Projects and lets the user choose one active Project.
- API key input still clears before screenshots.
- Existing operations dashboard remains green.
- Reviewer confirms this is a WEB-002 partial, not full Web UI parity.
