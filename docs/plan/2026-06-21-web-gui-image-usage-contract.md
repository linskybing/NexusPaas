# Web GUI Image Usage Contract Slice

## 1. Objective

Move the first-party Web GUI closer to External GA by adding narrow,
project-scoped image and usage read surfaces and by settling the current
WebRPC GUI requirement as an explicit GUI API contract decision.

For this slice, the Web GUI continues to consume existing REST/OpenAPI routes.
No new WebRPC, tRPC, gRPC, or parallel RPC transport is introduced unless this
plan review finds a concrete API gap that existing endpoints cannot satisfy.

This targets partial evidence for `WEB-005` and `WEB-007`; it does not close
full Web GUI GA.

## 2. Background

Existing approved Web slices created a Vite/React first-party GUI served by
`platform-gateway` at `/ui/`, added authorized Project selection, and added a
Workloads panel for ConfigFiles and jobs.

The live tracker still marks Web incomplete because the GUI lacks image,
WebRTC, usage, OIDC login, real active-Project live mutation evidence, and a
separately approved WebRPC GUI contract if that remains required. The External
GA roadmap already states that REST/OpenAPI is the current GUI backend contract
until a proven API gap exists.

This slice keeps that direction and adds the smallest useful missing GUI read
surfaces backed by existing service-owned APIs.

## 3. Source References

- `gap.md`
- `problem.md`
- `docs/acceptance/gap-analysis.md`
- `docs/acceptance/image-build.md`
- `docs/acceptance/monitoring.md`
- `docs/acceptance/webrtc.md`
- `docs/plan/2026-06-20-external-ga-web-gui-live-e2e.md`
- `docs/plan/2026-06-21-web-gui-foundation-live-e2e.md`
- `docs/plan/2026-06-21-web-gui-project-selector.md`
- `docs/plan/2026-06-21-web-gui-workload-workflows.md`
- `frontend/src/api.ts`
- `frontend/src/types.ts`
- `frontend/src/App.tsx`
- `frontend/src/api.test.ts`
- `frontend/src/App.test.tsx`
- `frontend/tests/e2e/dashboard.spec.ts`
- `backend/internal/services/imageregistry/spec.go`
- `backend/internal/services/imageregistry/handler.go`
- `backend/internal/services/resourcehours/spec.go`
- `backend/internal/services/gpuusage/handler.go`
- `backend/internal/services/workload/spec.go`
- `backend/internal/services/workload/stream_credentials.go`

## 4. Assumptions

- "WebRPC GUI" means a first-party GUI with a reviewed backend API contract for
  browser workflows, not necessarily a new RPC transport.
- Existing REST/OpenAPI routes are acceptable for GA v1 unless a concrete
  browser workflow cannot be implemented against them.
- Backend RBAC and service-owned routes remain authoritative. Frontend filtering
  is display-only.
- The local agent may not have live active Project data, image rows, usage rows,
  TURN configuration, or a running streaming workload. Missing live data must be
  recorded as partial evidence, not waived.
- Context7 will be used before implementation if React, Playwright, Vite, or
  Testing Library syntax or behavior needs current documentation.

## 5. Non-Goals

- No OIDC browser login in this slice.
- No new WebRPC/tRPC/gRPC layer.
- No backend route, database, migration, deployment, or service-boundary change.
- No image build submission, image approval, catalog publish/unpublish, or
  destructive image action from the GUI.
- No full WebRTC browser session launch or TURN credential issuance UI in this
  slice. WebRTC remains a separate live E2E slice because it needs a real
  streaming job, TURN config, and browser session evidence.
- No claim that `WEB-001..007`, RTC, MON, IMG, or External GA are complete.
- No persistence of API keys, JWTs, TURN credentials, or secrets in browser
  storage.

## 6. Current Behavior

The GUI can connect with an in-memory API key, show operations status, list
Projects, set an active Project, list active-Project ConfigFiles, list jobs
filtered by active Project, submit a minimal ConfigFile, and request job cancel.

It cannot display Project image allow-list/build status or usage views. The
tracker therefore keeps `WEB-005` and `WEB-007` open. The WebRPC GUI requirement
is also still mentioned as a tracked gap even though the approved roadmap uses
REST/OpenAPI as the current GUI contract.

## 7. Target Behavior

- The frontend API client exposes typed helpers for:
  - `GET /api/v1/projects/{id}/images`
  - `GET /api/v1/projects/{id}/image-builds`
  - `GET /api/v1/me/usage`
  - `GET /api/v1/me/request-usage`
  - `GET /api/v1/projects/{id}/gpu-usage`
- The active Project view adds compact read-only panels:
  - Images: allow-listed images with repository/tag/digest/deleted/scan/build
    status when present.
  - Usage: current user's usage/request usage and active Project GPU usage when
    the authorized API returns data.
- Empty, unavailable, and forbidden states render without breaking the dashboard.
- The GUI contract is documented in the plan and ledgers as REST/OpenAPI for GA
  v1. A new WebRPC transport remains a non-goal until a concrete API gap is
  approved.
- `gap.md` and `docs/acceptance/gap-analysis.md` are updated honestly: this is
  partial evidence for `WEB-005` and `WEB-007`, not full Web closure.

## 8. Affected Domains

- Frontend management GUI.
- Image registry read APIs consumed by the GUI.
- Usage-observability read APIs consumed by the GUI.
- Acceptance ledgers and plan checklist.

Service ownership remains unchanged:

- image data stays behind `image-registry-service` routes.
- usage data stays behind `usage-observability-service` routes.
- workload WebRTC credentials stay behind `workload-service` and are not changed
  in this slice.

## 9. Affected Files

- `frontend/src/types.ts`
- `frontend/src/api.ts`
- `frontend/src/App.tsx`
- `frontend/src/styles.css`
- `frontend/.gitignore`
- `frontend/src/api.test.ts`
- `frontend/src/App.test.tsx`
- `frontend/tests/e2e/dashboard.spec.ts`
- `docs/plan/2026-06-21-web-gui-image-usage-contract.md`
- `gap.md`
- `docs/acceptance/gap-analysis.md`

No backend source file should change unless implementation discovers that an
existing endpoint cannot be called safely from the GUI; that would require plan
revision and Reviewer approval before coding.

## 10. API / Contract Changes

No backend API change is planned.

Frontend client additions are internal TypeScript helpers around existing
REST/OpenAPI routes. The approved GUI API contract for this slice is:

- same-origin `/ui/` frontend;
- existing authenticated REST/OpenAPI routes;
- existing `X-API-Key` or JWT header handling through the shared API client;
- backend RBAC as the source of truth.

This explicitly rejects a new WebRPC transport for this slice because it would
duplicate existing reviewed routes without a proven API gap.

## 11. Database / Migration Changes

None.

## 12. Configuration Changes

None.

The GUI continues to use same-origin API calls by default and optional
`VITE_API_BASE_URL` for non-standard local smoke setups. Runtime test secrets
remain outside bundled `VITE_` variables.

## 13. Observability Changes

None in backend runtime.

Frontend test and live evidence should include:

- unit test output;
- production frontend build output;
- Playwright smoke artifact for `/ui/`;
- live HTTP status/count evidence for image and usage routes when a live gateway
  and runtime-only API key are available.

## 14. Security Considerations

- No credentials or tokens are persisted in localStorage, sessionStorage, cookies,
  IndexedDB, or build-time variables.
- Usage and image visibility remain backend-enforced by existing RBAC.
- The UI must not render raw secret-like fields if usage or image payloads carry
  unexpected keys. Show only known display fields.
- Project GPU usage failures must render as unavailable/forbidden state instead
  of falling back to broader admin endpoints.
- WebRTC TURN credentials are not requested in this slice, so no short-lived
  credential material can appear in screenshots or logs.

## 15. Implementation Steps

- [x] Add image, image build, usage, request usage, and Project GPU usage record
  types using permissive payload shapes that match existing API records without
  over-modeling every backend JSONB field.
- [x] Add API client methods for the existing image and usage routes.
- [x] Add a combined active-Project read model helper only if it reduces
  duplicated request orchestration in the UI; otherwise call the new methods
  directly from the panel. The implementation kept requests inside the panels
  because that was the smaller shape.
- [x] Add an Images panel under active Project context with empty and unavailable
  states.
- [x] Add a Usage panel under active Project context with empty and unavailable
  states.
- [x] Keep panels disabled when no active Project exists or Project API is
  unavailable.
- [x] Extend API client tests for the new endpoints and envelope handling.
- [x] Extend App tests for image rows, build rows, usage rows, forbidden/empty
  states, and no credential persistence.
- [x] Extend Playwright smoke to assert the Images and Usage panels render in the
  live `/ui/` path.
- [x] Run verification commands and record results in this plan.
- [x] Update `gap.md` and `docs/acceptance/gap-analysis.md` with partial
  `WEB-005`/`WEB-007` evidence and the REST/OpenAPI WebRPC contract decision.
- [x] Submit implementation to Reviewer Agent for final review.

## 16. Verification Plan

Focused frontend:

```sh
npm --prefix frontend run test
npm --prefix frontend run build
```

Focused backend regression for consumed service contracts:

```sh
go -C backend test ./internal/services/imageregistry ./internal/services/gpuusage ./internal/services/resourcehours ./internal/services/workload -count=1
```

Repository gates:

```sh
go -C backend test ./... -count=1
bash backend/scripts/ci-security-gate.sh quick
bash backend/scripts/ci-security-gate.sh sonar
git diff --check
```

If SonarScanner cannot run because local Sonar configuration or credentials are
unavailable, record `Not Run` with the concrete missing configuration and keep
final completion pending any required Quality Gate evidence.

Live `/ui/` evidence when a local RKE2 gateway and runtime-only admin key are
available:

```sh
docker build -t localhost:5000/nexuspaas-backend:<tag> -f backend/Dockerfile .
docker push localhost:5000/nexuspaas-backend:<tag>
kubectl -n nexuspaas set image deployment/platform-gateway app=localhost:5000/nexuspaas-backend:<tag>
kubectl -n nexuspaas rollout status deployment/platform-gateway --timeout=180s
curl -fsS http://127.0.0.1:18080/ui/ >/tmp/nexuspaas-ui.html
NEXUSPAAS_E2E_API_KEY=<admin-key> npm --prefix frontend run e2e
```

Live image/usage route evidence must be attempted after rollout when a gateway
and runtime-only key are available. Capture HTTP status and item count for each
route; if no active Project exists, record `No active Project` with the project
list response count and keep the slice partial. If a live gateway or key is not
available, record `Not Run` with the concrete missing prerequisite instead of
claiming live WEB-005/WEB-007 evidence.

```sh
curl -fsS -H "X-API-Key: <admin-key>" http://127.0.0.1:18080/api/v1/projects
curl -fsS -H "X-API-Key: <admin-key>" http://127.0.0.1:18080/api/v1/projects/<id>/images
curl -fsS -H "X-API-Key: <admin-key>" http://127.0.0.1:18080/api/v1/projects/<id>/image-builds
curl -fsS -H "X-API-Key: <admin-key>" http://127.0.0.1:18080/api/v1/me/usage
curl -fsS -H "X-API-Key: <admin-key>" http://127.0.0.1:18080/api/v1/me/request-usage
curl -fsS -H "X-API-Key: <admin-key>" http://127.0.0.1:18080/api/v1/projects/<id>/gpu-usage
```

If no active Project exists, live evidence must say so and this slice remains a
GUI shell/rendering partial.

## 17. Rollback Plan

Revert this slice's frontend and ledger changes. No backend route, migration, or
runtime configuration change is expected. If the frontend bundle is already
rolled, roll `platform-gateway` back to the previously verified image.

## 18. Risks and Tradeoffs

- Read-only image/usage panels do not satisfy full IMG, MON, RTC, or WEB
  acceptance by themselves.
- Live empty-list evidence is weaker than real Project data. The ledger must
  keep this explicit.
- A new WebRPC transport could provide typed browser contracts, but adding it now
  would duplicate existing REST/OpenAPI routes and expand security/test scope
  without a proven gap.
- Usage payloads are JSONB/read-model shaped. The UI should display known fields
  conservatively rather than inventing a broad data abstraction.

## 19. Reviewer Checklist

| Check | Status |
|---|---|
| Requirement fit for Web GUI image and usage partial | Pass |
| WebRPC GUI requirement handled without wheel-rebuilding | Pass |
| Scope limited to existing REST/OpenAPI consumption | Pass |
| No backend service ownership regression | Pass |
| SOLID compliance | Pass |
| 12-Factor compliance | Pass |
| Frontend tests and build | Pass |
| Backend focused/full tests and quick gate | Pass |
| SonarScanner Quality Gate | Pass — cleanup slice reports `new_coverage=81.2`, `new_violations=0` |
| Live E2E attempted and recorded honestly | Pass |
| `problem.md` / `gap.md` accuracy | Pass |
| Diff scope | Pass |

## 20. Status

Status: Approved

Reviewer Agent Harvey approved the revised plan after Sonar Quality Gate and
live evidence requirements were made explicit.

Implementation evidence (2026-06-21):

- Added frontend image, build, usage, request-usage, and Project GPU usage types
  and API client methods for existing REST/OpenAPI endpoints only.
- Added read-only Images and Usage panels under the active Project context.
- Added `*.tsbuildinfo` to `frontend/.gitignore` and removed the generated
  `frontend/tsconfig.tsbuildinfo` artifact.
- Project GPU usage failures render as unavailable and do not fall back to admin
  usage routes.
- The WebRPC GUI requirement is handled as an approved first-party GUI API
  contract over existing REST/OpenAPI routes; no new transport was added.
- `npm --prefix frontend run test`: passed, 2 files / 15 tests.
- `npm --prefix frontend run build`: passed.
- Focused backend service tests passed:
  `go -C backend test ./internal/services/imageregistry ./internal/services/gpuusage ./internal/services/resourcehours ./internal/services/workload -count=1`.
- `go -C backend test ./... -count=1`: passed.
- `go -C backend test ./... -coverprofile=coverage.out -count=1`: passed.
- `bash backend/scripts/ci-security-gate.sh quick`: passed.
- `bash backend/scripts/ci-security-gate.sh sonar`: initially failed after a
  real analysis upload; the dedicated cleanup slice now passes the same gate.
  Latest Sonar API readback: `new_coverage=81.2`, `new_violations=0`,
  `new_duplicated_lines_density=0.35803`.
- Built and pushed
  `localhost:5000/nexuspaas-backend:ci-ga-web-image-usage-20260621013315`
  (`sha256:d6bc9f703e57d52301d6d286eb1b42b8f8f3a906727e0c08c333be4fea834bfb`)
  and rolled `deployment/platform-gateway`.
- Live `/ui/` HTML and JS asset fetch passed for `/ui/assets/index-Ca_csg9P.js`.
- Live Playwright smoke passed against `http://127.0.0.1:18080/ui/` with the
  runtime-only API key and refreshed `frontend/test-results/gui-live-smoke.png`.
- Live route checks returned `HTTP 200 count=0` for `/api/v1/projects`,
  `/api/v1/me/usage`, and `/api/v1/me/request-usage`. No active Project existed,
  so project-scoped `/images`, `/image-builds`, and `/gpu-usage` route checks
  were recorded as `No active Project` rather than claimed as full live data
  evidence.

Plan Agent checklist:

- [x] Requirements restated.
- [x] Existing REST/OpenAPI contract preferred over new transport.
- [x] Scope kept to image/usage read surfaces.
- [x] Non-goals documented.
- [x] Verification, Sonar gate handling, live evidence, and rollback documented.
- [x] Reviewer Agent approval received.
- [x] Code Agent implementation complete.
- [x] Reviewer Agent final implementation approval received.
