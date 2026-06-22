# Web GUI Workload Workflow Slice

Status: Implemented and reviewer-verified

Reviewer: Boole approved the plan before implementation after revisions for
active-Project job display filtering and live rolled `/ui/` evidence. Boole
also approved the final implementation after reviewing requirement fit,
approved-plan alignment, SOLID/12-Factor, live evidence, checklist accuracy, and
diff scope.

Implementation evidence (2026-06-21):

- Added frontend workload types, API client JSON `POST` support, ConfigFile/job
  methods, and a project-scoped Workloads panel.
- The Workloads panel lists ConfigFiles, lists active-Project jobs only, submits
  a minimal ConfigFile for the active Project, and sends job cancel requests.
- Multi-Project frontend fixture proves inactive-Project jobs are hidden.
- `npm --prefix frontend run test`: passed, 2 files / 13 tests.
- `npm --prefix frontend run build`: passed.
- `go -C backend test ./internal/services/workload`: passed.
- `go -C backend test ./...`: passed.
- `bash backend/scripts/ci-security-gate.sh quick`: passed.
- Built and pushed
  `localhost:5000/nexuspaas-backend:ci-ga-web-workloads-20260620193544`
  (`sha256:768e7c10fbec81b907601cd5f248e24f7115ace900419e6676f185331b830171`).
- Rolled `deployment/platform-gateway` container `app` to that image.
- Live `/ui/` HTML smoke passed, and JS asset
  `/ui/assets/index-BZt6oW-J.js` was fetched from the rolled gateway.
- Live Playwright smoke passed against `http://127.0.0.1:18080/ui/`; screenshot
  `frontend/test-results/gui-live-smoke.png` was refreshed.
- Live gateway API checks returned `HTTP 200` with `count=0` for
  `/api/v1/projects`, `/api/v1/configfiles`, and `/api/v1/jobs`. Keep WEB-003
  and WEB-004 partial because no real active Project data or live submit/cancel
  workflow was present.

## Objective

Move the first-party Web GUI from operations status only to a narrow,
project-scoped workload workflow surface by using the existing REST/OpenAPI
contract for ConfigFiles and jobs.

This is a partial for `WEB-003` and `WEB-004`. It does not claim full Web UI GA
parity because OIDC browser login, real live Project data, images, WebRTC,
usage views, and any separately approved WebRPC contract remain open.

## Scope

- Extend the frontend API client to support existing workload endpoints:
  - `GET /api/v1/configfiles`
  - `GET /api/v1/projects/{id}/config-files`
  - `POST /api/v1/configfiles`
  - `GET /api/v1/jobs`
  - `POST /api/v1/jobs/{id}/cancel`
- Add a compact Workloads panel under the active Project context:
  - list ConfigFiles
  - list jobs with status, filtering the authorized job list by active
    `project_id` / `projectId` for display
  - show logs link/readiness state without claiming live tailing
  - submit a minimal ConfigFile for the active Project
  - request job cancel for cancelable jobs
- Keep credentials in memory only; do not add localStorage/sessionStorage.
- Reuse the current same-origin `/ui/` deployment and `VITE_API_BASE_URL`
  fallback.
- Extend Vitest coverage for API methods, UI load states, submit, cancel, and
  no-Project disabled state.
- Add a multi-Project job fixture proving inactive-Project jobs are hidden in
  the active Project workload panel. Backend RBAC remains authoritative; this is
  only display scoping.
- Extend Playwright smoke to prove the Workloads panel is rendered in the live
  GUI path. If live Project data is absent, record the partial state honestly.
- Update `gap.md` and `docs/acceptance/gap-analysis.md` after verification.

## Non-Goals

- No backend route, database, or service-boundary changes in this slice.
- No OIDC browser login.
- No new WebRPC transport. The current approved GUI contract is REST/OpenAPI;
  the user-requested "WebRPC GUI" remains a tracked gap until a separate plan
  defines and approves that contract.
- No image catalog, WebRTC stream launch, or usage dashboard.
- No job submit UI in this slice. Job submit depends on scheduler admission and
  valid ConfigFile/cluster context; the first UI step is safe ConfigFile create,
  job list, and cancel request.

## Source References

- `docs/acceptance/gap-analysis.md`
- `gap.md`
- `docs/plan/2026-06-21-web-gui-project-selector.md`
- `frontend/src/App.tsx`
- `frontend/src/api.ts`
- `frontend/tests/e2e/dashboard.spec.ts`
- `backend/internal/services/workload/spec.go`
- `backend/internal/services/workload/handler.go`
- `backend/internal/services/workload/job_access_handlers.go`

## Assumptions

- Existing backend RBAC remains the source of truth for Project, ConfigFile, and
  job visibility.
- The GUI may render empty authorized lists as a valid live state.
- State-changing GUI requests should use existing API-key/JWT header handling
  and include JSON content type; no token persistence is needed.
- Context7's primary MCP endpoint reported an invalid API key, but the
  connector Context7 endpoint successfully returned current React, React
  Testing Library, and Playwright documentation. Frontend changes will still
  follow the repository's existing React, Vitest, and Playwright patterns.

## Affected Files

- `frontend/src/types.ts`
- `frontend/src/api.ts`
- `frontend/src/App.tsx`
- `frontend/src/styles.css`
- `frontend/src/api.test.ts`
- `frontend/src/App.test.tsx`
- `frontend/tests/e2e/dashboard.spec.ts`
- `docs/plan/2026-06-21-web-gui-workload-workflows.md`
- `docs/acceptance/gap-analysis.md`
- `gap.md`

## API / Contract Changes

No backend contract changes. The frontend consumes existing REST/OpenAPI routes.

Frontend client additions are internal TypeScript helpers around existing
routes and response envelopes.

## Data / Migration Changes

None.

## Configuration Changes

None. The UI continues to use same-origin requests by default and optional
`VITE_API_BASE_URL` for non-standard smoke setups.

## Security / 12-Factor Notes

- Backend RBAC remains authoritative; frontend filtering is display-only.
- No credentials are persisted in browser storage.
- State-changing requests are explicit user actions and send JSON through the
  existing API client.
- The slice keeps service ownership intact: workload data stays behind
  workload-service routes.
- The UI remains stateless and environment-configured, matching 12-Factor
  config expectations.

## Implementation Steps

- [x] Add workload record types and payload types.
- [x] Generalize the API client for `GET` and JSON `POST` while preserving
  envelope handling and timeout behavior.
- [x] Add ConfigFile/job client methods using existing workload routes.
- [x] Add a Workloads panel scoped by active Project with disabled controls when
  no active Project exists.
- [x] Filter displayed jobs by active Project and test that jobs from inactive
  Projects are hidden.
- [x] Add UI feedback for load errors, successful ConfigFile submission, and job
  cancel request acceptance.
- [x] Extend unit tests for API methods and GUI interactions.
- [x] Extend the live Playwright smoke assertion to include the Workloads panel.
- [x] Update `gap.md` and `gap-analysis.md` with partial WEB-003/WEB-004
  evidence and explicit remaining gaps.

## Verification Plan

```sh
npm --prefix frontend run test
npm --prefix frontend run build
go -C backend test ./internal/services/workload
go -C backend test ./...
bash backend/scripts/ci-security-gate.sh quick
git diff --check
```

For live `/ui/` evidence, the frontend build must be embedded in a new backend
image and rolled before Playwright:

```sh
docker build -t localhost:5000/nexuspaas-backend:<tag> -f backend/Dockerfile .
docker push localhost:5000/nexuspaas-backend:<tag>
kubectl -n nexuspaas set image deployment/platform-gateway app=localhost:5000/nexuspaas-backend:<tag>
kubectl -n nexuspaas rollout status deployment/platform-gateway --timeout=180s
curl -fsS http://127.0.0.1:18080/ui/ >/tmp/nexuspaas-ui.html
UI_ASSET="$(grep -o '/ui/assets/[^"]*\.js' /tmp/nexuspaas-ui.html | head -1)"
test -n "$UI_ASSET"
curl -fsS "http://127.0.0.1:18080${UI_ASSET}" >/tmp/nexuspaas-ui.js
NEXUSPAAS_E2E_API_KEY=<admin-key> npm --prefix frontend run e2e
```

If a local-only preview E2E is run instead, it must be labeled local-only and
must not be used as live GUI evidence.

If live E2E cannot submit a ConfigFile because no real active Project exists in
the live topology, the result must remain partial and the checklist must say so.

## Rollback Plan

Revert the frontend and checklist changes from this slice. No backend migration
or persisted schema change is introduced.

## Risks and Tradeoffs

- This adds a real workflow surface but still does not close full `WEB-*`.
- Live empty-list evidence is useful but weaker than evidence with real Project
  data; the checklist must stay explicit.
- Adding job submit now would expand cluster/admission blast radius; this slice
  leaves it for a dedicated plan.

## Reviewer Checklist

| Check | Status |
|---|---|
| Requirement fit for WEB-003/WEB-004 partial | Pass |
| Approved-plan alignment | Pass |
| SOLID and 12-Factor compliance | Pass |
| Existing REST/OpenAPI contract respected | Pass |
| No backend service ownership regression | Pass |
| Frontend tests/build | Pass |
| Live E2E smoke | Pass |
| Backend focused/full tests and quick gate | Pass |
| Checklist accuracy | Pass |
| Diff scope | Pass |
