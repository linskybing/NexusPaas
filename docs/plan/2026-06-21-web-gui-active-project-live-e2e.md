# Web GUI Active Project Live E2E Slice

## 1. Objective

Replace the current empty-list Web GUI live evidence with a repeatable live
browser smoke that creates a real Project through existing APIs, proves the
first-party `/ui/` selects it, and exercises project-scoped read panels.

This is partial WEB/E2E evidence only. It does not close OIDC login, WebRTC, or
full GA.

## 2. Background

`gap.md` records that live `/ui/` evidence had `project_count=0`, so
project-scoped image/build/GPU routes were `No active Project`. Existing backend
routes can create a Group and Project, request/approve an image, start an image
build, and submit a ConfigFile without adding a new API or fake-data mode.

Context7 reference used for this slice: Playwright `/microsoft/playwright.dev`
documents using the `request` fixture / API request context to set up backend
preconditions before UI assertions, and recommends user-facing locators with
auto-waiting.

## 3. Source References

- `gap.md`
- `docs/acceptance/gap-analysis.md`
- `docs/plan/2026-06-21-web-gui-image-usage-contract.md`
- `frontend/tests/e2e/dashboard.spec.ts`
- `frontend/src/App.tsx`
- `frontend/src/api.ts`
- `backend/internal/services/orgproject/project_handlers.go`
- `backend/internal/services/orgproject/handler.go`
- `backend/internal/services/imageregistry/handler.go`
- `backend/internal/services/imageregistry/helpers.go`

## 4. Assumptions

- The runtime API key used for live E2E has enough privilege to create and clean
  up test Group/Project rows.
- Existing APIs are enough for this evidence. If an API rejects optional image
  seed data, record the rejection and keep the core Project/ConfigFile evidence.
- The live gateway remains reachable at the configured Playwright app path.

## 5. Non-Goals

- No backend API, database, migration, or deployment change.
- No new WebRPC/tRPC/gRPC transport.
- No fake fixture endpoint.
- No OIDC browser login.
- No WebRTC session launch.
- No destructive image or workload operation except best-effort cleanup of
  records created by this smoke through existing owner APIs.

## 6. Current Behavior

The existing live GUI smoke proves `/ui/` renders and the gateway returns empty
authorized lists for Projects and current-user usage. It does not prove active
Project selection, project-scoped image/build/GPU routes with a real Project, or
live ConfigFile submit through the GUI.

## 7. Target Behavior

When `NEXUSPAAS_E2E_SEED_PROJECT=true` and `NEXUSPAAS_E2E_API_KEY` are set,
`frontend/tests/e2e/dashboard.spec.ts` should:

- seed a uniquely named Group and Project through existing REST routes;
- seed a Project image request, approve it, and start one image build through
  existing REST routes when those APIs accept the runtime data;
- connect to `/ui/` with the runtime API key;
- assert the seeded Project appears in the Active project selector;
- assert Workloads, Images, and Usage panels render with that active Project ID;
- submit one ConfigFile through the UI and assert it appears in the ConfigFiles
  table;
- leave no API key in the key input after connect;
- track created IDs and best-effort clean up created owner records through
  existing APIs after the test;
- record explicit leftover evidence for any seeded owner record that has no
  existing cleanup API or whose cleanup API rejects the request.

If seeding is disabled, the existing smoke remains unchanged.

## 8. Affected Domains

- Frontend live E2E test coverage.
- Org/project API usage for live test setup.
- Workload ConfigFile API usage through the GUI.
- Image-registry API usage for optional Project image/build evidence.
- Acceptance ledgers.

## 9. Affected Files

- `frontend/tests/e2e/dashboard.spec.ts`
- `docs/plan/2026-06-21-web-gui-active-project-live-e2e.md`
- `gap.md`
- `docs/acceptance/gap-analysis.md`

## 10. API / Contract Changes

None. The smoke uses existing REST/OpenAPI routes and the existing first-party
GUI contract.

## 11. Database / Migration Changes

None.

## 12. Configuration Changes

No runtime config change.

The E2E test reads optional environment flags:

- `NEXUSPAAS_E2E_SEED_PROJECT=true`
- existing `NEXUSPAAS_E2E_API_KEY`
- existing `NEXUSPAAS_E2E_APP_PATH`
- existing `VITE_API_BASE_URL`

## 13. Observability Changes

None in backend runtime. The test should log created IDs and cleanup leftovers
to Playwright output for evidence.

## 14. Security Considerations

- Do not log the runtime API key.
- Do not persist credentials in browser storage.
- Use only the shared GUI key entry and existing backend RBAC.
- Use unique test IDs so any leftover records are identifiable.

## 15. Implementation Steps

- [x] Keep the existing unseeded live smoke path unchanged.
- [x] Add seeded setup helpers in `frontend/tests/e2e/dashboard.spec.ts` using
  Playwright's `request` fixture.
- [x] Track created Group, Project, ConfigFile, image request, image rule, and
  image build IDs when the APIs return them.
- [x] Add UI assertions for seeded Project selection, project-scoped panel
  rendering, ConfigFile submit, and cleared API key input.
- [x] Add best-effort cleanup for tracked IDs through existing owner APIs, or
  log explicit leftover evidence when no cleanup API exists.
- [x] Run verification and update ledgers honestly.

## 16. Verification Plan

Focused live:

```sh
NEXUSPAAS_E2E_API_KEY=<runtime-key> \
NEXUSPAAS_E2E_SEED_PROJECT=true \
npm --prefix frontend run e2e
```

Regression:

```sh
npm --prefix frontend run test
npm --prefix frontend run build
go -C backend test ./... -count=1
go -C backend test ./... -coverprofile=coverage.out -count=1
bash backend/scripts/ci-security-gate.sh quick
bash backend/scripts/ci-security-gate.sh sonar
git diff --check
```

If SonarScanner is unavailable, record the concrete missing prerequisite as
`Not Run`; final completion remains pending if the Quality Gate is required for
the slice review.

Live route proof after smoke:

```sh
curl -fsS -H "X-API-Key: <runtime-key>" http://127.0.0.1:18080/api/v1/projects
curl -fsS -H "X-API-Key: <runtime-key>" http://127.0.0.1:18080/api/v1/projects/<id>/images
curl -fsS -H "X-API-Key: <runtime-key>" http://127.0.0.1:18080/api/v1/projects/<id>/image-builds
curl -fsS -H "X-API-Key: <runtime-key>" http://127.0.0.1:18080/api/v1/projects/<id>/gpu-usage
```

## 17. Rollback Plan

Revert this slice's E2E and ledger changes. Clean up any test records whose IDs
were logged by the failed seeded smoke.

## 18. Risks and Tradeoffs

- The runtime key may not have admin/project-manager privileges. If so, record
  the exact HTTP failure and do not claim active Project evidence.
- Cleanup is best-effort and must include every seeded owner record ID known to
  the smoke. Generated IDs use a unique prefix so API-rejected leftovers are easy
  to identify and delete.
- Image request/build seeding may fail if live RBAC or downstream policy rejects
  it. The core evidence remains active Project + ConfigFile + project-scoped
  route rendering; rejected image seeding must be recorded honestly.

## 19. Reviewer Checklist

| Check | Status |
|---|---|
| Requirement fit: active Project live GUI evidence | Pass — seeded live E2E selected a real Project and submitted a ConfigFile through `/ui/` |
| Existing REST/OpenAPI only | Pass — no new API/transport introduced |
| No backend contract or DB change | Pass for this slice; separate org-project auth compatibility slice had no API/DB/config change |
| Cleanup/leftover evidence covers seeded owner records | Pass with caveat — Group, Project, ConfigFile, Project image, and image build cleaned; image request has no DELETE route |
| No credential leakage | Pass — test logs IDs/status only, not the API key |
| Frontend tests/build | Pass — `npm --prefix frontend run test`, `npm --prefix frontend run build`, and seeded Playwright E2E passed |
| Backend full tests/coverage/quick gate | Pass — `go -C backend test ./...`, coverage run, and quick gate passed |
| Sonar Quality Gate | Pass — local SonarScanner Quality Gate passed |
| Ledgers accurate and partial WEB/E2E label preserved | Pass — gap ledgers updated as partial, not full GA |
| Diff scope | Pass — scoped to live E2E plus evidence/ledger files |

## 20. Status

Status: Approved

Final implementation review: Approved by Reviewer Agent; no blocking findings.

Implementation evidence:

- `frontend/tests/e2e/dashboard.spec.ts` now keeps the default unseeded smoke and
  adds an opt-in seeded path behind `NEXUSPAAS_E2E_SEED_PROJECT=true`.
- Live seeded command passed:

```sh
NEXUSPAAS_E2E_API_KEY=<runtime-key> \
NEXUSPAAS_E2E_SEED_PROJECT=true \
NEXUSPAAS_E2E_APP_PATH=/ui/ \
npm --prefix frontend run e2e
```

- Live route proof from the passing run:

```json
{
  "project_id": "e2e-p-mqnbnezg-xgp1qi",
  "project_count": 1,
  "seeded_project_present": true,
  "config_file_count": 1,
  "seeded_config_id": "CFG2600002",
  "image_count": 1,
  "seeded_image_identifier": "e2e-p-mqnbnezg-xgp1qi:nexuspaas-e2e:mqnbnezg-xgp1qi",
  "build_count": 0,
  "gpu_status": 403,
  "gpu_ok": false
}
```

- Cleanup evidence from the passing run: `CFG2600002`, image build
  `e2e-build-e2e-p-mqnbnezg-xgp1qi`, Project image
  `e2e-p-mqnbnezg-xgp1qi:nexuspaas-e2e:mqnbnezg-xgp1qi`, Project
  `e2e-p-mqnbnezg-xgp1qi`, and Group `e2e-g-mqnbnezg-xgp1qi` returned cleanup
  success. The image request route has no DELETE handler, so the test records
  that leftover explicitly.
- Partial evidence caveats: image build create/cancel was exercised, but the
  post-run build list returned `build_count=0`; active-Project GPU usage returned
  `HTTP 403`, so the GUI proof is unavailable-state rendering rather than
  successful GPU usage data.
- Follow-up: `docs/plan/2026-06-21-clusterread-static-admin-gpu-usage.md`
  rolled `usage-observability-service` and the same seeded live E2E now records
  `gpu_status=200`, `gpu_ok=true`; a direct live probe returned `used=0` for a
  seeded Project with no GPU pods.
- Follow-up: `docs/plan/2026-06-21-gateway-adapter-route-proxy-precedence.md`
  rolled `platform-gateway` to
  `localhost:5000/nexuspaas-backend:ci-ga-gateway-proxy-adapter-20260621054757`
  and the seeded live E2E now records `build_count=1` with the image build
  cleanup returning `HTTP 200`.
- Follow-up:
  `docs/plan/2026-06-21-web-gui-job-submit-cancel-live-e2e.md` rolled
  `platform-gateway` to
  `localhost:5000/nexuspaas-backend:ci-ga-web-job-submit-20260621141339` and
  the seeded live E2E now records `seeded_job_present=true`,
  `job_cancel_requested=true`, and cleanup of ConfigFile, image build, Project
  image, Plan, Queue, Project, and Group returning `HTTP 200`.

Plan Agent checklist:

- [x] Requirement restated.
- [x] Existing REST/OpenAPI contract preserved.
- [x] Cleanup/leftover evidence requirement included.
- [x] Verification includes Sonar gate handling.
- [x] Reviewer Agent approval received.
- [x] Code Agent implementation complete.
- [x] Reviewer Agent final implementation approval received.
