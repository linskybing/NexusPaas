# Web GUI Image Status Parity

## 1. Objective

Materially advance `WEB-005` by making Project image list API rows expose
catalog-derived image digest, scan status, deleted, unavailable, and status
fields at the top level consumed by the first-party Web GUI, then prove the GUI
renders those fields with focused tests and a bounded live UI/API route proof.

## 2. Background

The first-party operations GUI already renders Project image columns for
`Image`, `Digest`, `Scan`, and `State`. The frontend reads top-level
`scan_status` / `scanStatus`, `deleted`, `unavailable`, and `status` from each
Project image row. The backend `GET /api/v1/projects/{id}/images` route returns
allow-list rows and attaches the matching catalog row only under nested
`catalog`; it does not guarantee those display fields at the top level.

Explorer review found the smallest backend hook is `enrichRuleWithCatalog`.
This slice normalizes already-known catalog/read-model fields into the API row.
It does not add a new Harbor client, scanner sync worker, or real-time Harbor
artifact reconciliation.

## 3. Source References

- `docs/acceptance/gap-analysis.md`
- `docs/acceptance/cli.md`
- `docs/acceptance/image-build.md`
- `gap.md`
- `problem.md`
- `docs/plan/2026-06-21-harbor-image-scan-live-evidence.md`
- `backend/internal/services/imageregistry/handler.go`
- `backend/internal/services/imageregistry/helpers.go`
- `backend/internal/services/imageregistry/handler_test.go`
- `frontend/src/App.tsx`
- `frontend/src/App.test.tsx`
- `frontend/src/api.ts`
- `frontend/src/api.test.ts`
- `frontend/src/types.ts`
- `frontend/tests/e2e/dashboard.spec.ts`

## 4. Assumptions

- `frontend/` remains the first-party Web GUI served by `platform-gateway` at
  `/ui/`.
- The approved GA v1 WebRPC contract remains same-origin REST/OpenAPI
  consumption; no separate WebRPC/tRPC/gRPC transport is required for this
  slice.
- Catalog/read-model rows may contain Harbor-derived fields after a sync or
  controlled seed. This slice only normalizes fields that are already present.
- Live proof may seed exact synthetic catalog and allow-list rows directly in
  the current live database when no public API exists to create catalog rows
  with Harbor scan/deleted metadata.
- Runtime API keys and database credentials may be held in local shell variables
  only and must not be printed.

## 5. Non-Goals

- No new Harbor SDK/client, scanner poller, queue worker, controller, or
  registry reconciliation loop.
- No claim that Harbor scan/delete metadata is automatically synchronized into
  `container_tags`; that remains a future sync/projection slice.
- No change to route auth/RBAC, Project image allow-list semantics, or build
  execution behavior.
- No new database table or migration.
- No visual redesign of the GUI.
- No full `WEB-005`, full image-build, SBOM/signing, or full GA closure claim.

## 6. Current Behavior

- `ImageTable` already renders `Scan` via top-level `scan_status` /
  `scanStatus`.
- `ImageTable` already renders `State` with precedence `deleted`,
  `unavailable`, then `status`, otherwise `available`.
- `ProjectImageRecord` already has the needed type fields.
- `GET /api/v1/projects/{id}/images` returns allow-list rows and nested
  `catalog`, but does not promote catalog scan/deleted/status/digest fields to
  the top level.
- Existing frontend tests cover the Images panel broadly but do not assert scan
  or state rendering.

## 7. Target Behavior

- Project image API rows preserve their existing shape and nested `catalog`.
- When the matching catalog row includes `digest`, `scan_status` / `scanStatus`,
  `deleted`, `unavailable`, or `status`, the API response exposes canonical
  top-level fields for GUI consumption.
- Allow-list row values keep precedence over catalog values when both are
  present, so a Project-specific policy state is not overwritten by catalog
  metadata.
- The GUI renders seeded scan/deleted/unavailable/status values from the API
  without storing credentials or requiring a new transport.

## 8. Affected Domains

- Image registry service Project image read contract.
- First-party Web GUI image table rendering tests.
- Acceptance ledgers for `WEB-005` partial status.

No microservice ownership boundary changes are introduced.

## 9. Affected Files

- `backend/internal/services/imageregistry/helpers.go`
- `backend/internal/services/imageregistry/handler_test.go`
- `frontend/src/App.test.tsx`
- `frontend/src/api.test.ts`
- `frontend/tests/e2e/dashboard.spec.ts`
- `docs/plan/2026-06-21-web-gui-image-status-parity.md`
- `gap.md`
- `problem.md`
- `docs/acceptance/gap-analysis.md`

## 10. API / Contract Changes

No route changes.

The `GET /api/v1/projects/{id}/images` response remains an array of Project
image rows. New behavior is additive and normalizes already-known fields to the
top level:

- `digest`
- `scan_status`
- `deleted`
- `unavailable`
- `status`

The nested `catalog` row remains available for compatibility.

## 11. Database / Migration Changes

None.

Live proof may insert exact synthetic rows into existing resources and must
delete them afterward:

- `image-registry-service:container_tags`
- `image-registry-service:image_allow_lists`
- related exact synthetic `org-project-service` rows if needed for UI selection

## 12. Configuration Changes

None.

## 13. Observability Changes

No new telemetry backend is added. Live evidence will record only:

- image tag/digest if a backend rebuild is needed;
- seeded Project/image identifiers;
- API row fields (`scan_status`, `deleted`, `unavailable`, `status`, `digest`);
- GUI visible table cells;
- cleanup leftover count.

## 14. Security Considerations

- Keep existing Project image route auth/RBAC.
- Do not expose Harbor credentials, API keys, auth headers, database passwords,
  Kubernetes Secret values, decoded values, hashes, Docker configs, or scanner
  credentials.
- Synthetic live rows must be exact and cleaned by deterministic IDs.
- The GUI must continue clearing the API key input and must not persist
  credentials.

## 15. Implementation Steps

- [x] Add a small helper that promotes selected catalog image-status fields to a
  Project image row only when the top-level row does not already define them.
- [x] Wire that helper into `enrichRuleWithCatalog` while preserving nested
  `catalog`.
- [x] Add backend tests proving Project image rows include digest, scan status,
  deleted/unavailable/status fields from catalog metadata and preserve row
  precedence.
- [x] Strengthen frontend unit/API tests to assert Images table `Scan`/`State`
  rendering and Project image API payload preservation.
- [x] Add a deterministic Playwright assertion for seeded Project image scan and
  state cells, gated by explicit live-test environment variables so normal smoke
  runs are not tied to synthetic catalog metadata.
- [x] Run focused backend tests.
- [x] Run frontend unit tests.
- [x] Run local SonarScanner Quality Gate.
- [x] If production code changed, build and push a timestamped backend image and
  roll only the needed service(s).
- [x] Run a bounded live UI/API route proof against `/ui/` and
  `/api/v1/projects/{id}/images`.
- [x] Update ledgers with exact evidence and remaining gaps.
- [x] Submit implementation to Reviewer Agent.

## 15.1 Completed Execution Evidence

Plan review:

- Reviewer Agent approved the plan after clarifying the frontend test command
  and deterministic, environment-gated Playwright proof.

Code:

- Added catalog-status promotion in `enrichRuleWithCatalog` through
  `promoteCatalogImageStatusFields`.
- Preserved existing row precedence: top-level Project image row values are not
  overwritten by catalog values.
- Canonicalized promoted `deleted` and `unavailable` string booleans into Go
  booleans.
- Added backend coverage for digest, scan status, deleted/unavailable/status
  promotion and nested `catalog` preservation.
- Added frontend unit/API coverage for Images table `Scan`/`State` cells and
  Project image payload preservation.
- Added Playwright expected Project/image status assertions gated by
  `NEXUSPAAS_E2E_PROJECT_ID`, `NEXUSPAAS_E2E_IMAGE_SCAN_STATUS`, and
  `NEXUSPAAS_E2E_IMAGE_STATE`.

Tests:

```sh
go -C backend test ./internal/services/imageregistry -run 'CatalogRequestsAndBuildWorkflow|BuildAndListRoutesSurfaceHarborDegradedAdditively|ProjectImagesPromoteCatalogStatusFields' -count=1
go -C backend test ./internal/services/imageregistry -count=1
npm --prefix frontend test
npm --prefix frontend run build
```

All passed.

Quality Gate:

```sh
bash backend/scripts/ci-security-gate.sh sonar
```

Passed with `QUALITY GATE STATUS: PASSED`.

Image and rollout:

- Built and pushed
  `localhost:5000/nexuspaas-backend:ci-ga-web-image-status-20260621214330`.
- Registry digest:
  `sha256:613c1f2cefbab9f029afc580213dc619e7892753826c02367fa6ea6be129f83e`.
- Rolled only `image-registry-service`.
- Ready pod:
  `image-registry-service-57f688dff4-pxbxc` with the new digest.

Live proof:

- Trace: `ga-web-image-status-20260621214849`.
- Synthetic Group/Project creation returned HTTP `201` / `201`.
- Seeded exact image-registry catalog and allow-list rows with:
  - `scan_status="Success"`;
  - `deleted=true`;
  - `unavailable=false`;
  - digest
    `sha256:613c1f2cefbab9f029afc580213dc619e7892753826c02367fa6ea6be129f83e`.
- `GET /api/v1/projects/{id}/images` returned success `true`, count `1`, and
  top-level `scan_status="Success"`, `deleted=true`, `unavailable=false`,
  computed state `deleted`, and the seeded digest.
- Playwright live GUI proof passed:
  `npx playwright test tests/e2e/dashboard.spec.ts --project=chromium` with
  `NEXUSPAAS_E2E_PROJECT_ID`, `NEXUSPAAS_E2E_IMAGE_SCAN_STATUS=Success`, and
  `NEXUSPAAS_E2E_IMAGE_STATE=deleted`.
- Initial cleanup detected two usage-observability projection rows for the
  synthetic Project; exact follow-up cleanup removed them.
- Final synthetic cleanup count: `0`.

Ledger updates:

- Updated `gap.md`, `problem.md`, and `docs/acceptance/gap-analysis.md` to
  record the WEB-005 catalog-derived API/GUI status proof and to keep automatic
  Harbor-to-catalog synchronization, full image-build/allow-list/SBOM/signing
  GUI scan workflow, OIDC browser login, WebRTC, real GPU telemetry, real log
  tailing, 8-unit rollback, DR, and load/perf gaps open.

## 16. Verification Plan

Backend focused tests:

```sh
go -C backend test ./internal/services/imageregistry -run 'CatalogRequestsAndBuildWorkflow|BuildAndListRoutesSurfaceHarborDegradedAdditively|ProjectImagesPromoteCatalogStatusFields' -count=1
go -C backend test ./internal/services/imageregistry -count=1
```

Frontend tests:

```sh
npm --prefix frontend test
```

Quality Gate:

```sh
bash backend/scripts/ci-security-gate.sh sonar
```

Pre-ledger hygiene:

```sh
git diff --check -- backend/internal/services/imageregistry/helpers.go backend/internal/services/imageregistry/handler_test.go frontend/src/App.test.tsx frontend/src/api.test.ts frontend/tests/e2e/dashboard.spec.ts gap.md problem.md docs/acceptance/gap-analysis.md docs/plan/2026-06-21-web-gui-image-status-parity.md
```

Live proof:

- seed exact Project/catalog/allow-list rows with `scan_status`, `digest`,
  `deleted`, `unavailable`, and/or `status`;
- verify `GET /api/v1/projects/{id}/images` exposes top-level fields;
- open `/ui/`, select the seeded Project, and verify visible scan/state cells
  through Playwright using expected-value environment variables;
- delete exact synthetic rows and verify no leftovers.

## 17. Rollback Plan

- Roll back the service image if a backend image is deployed.
- Remove exact synthetic rows inserted for live proof.
- Revert only this slice's source/ledger edits if Reviewer Agent rejects the
  implementation.

## 18. Risks and Tradeoffs

- This can only display fields present in the catalog/read model. It does not
  create authoritative Harbor synchronization.
- If future Harbor sync emits alternate boolean representations, frontend
  `boolValue` may need a separate normalization slice. This slice will keep
  boolean values canonical.
- Top-level row precedence avoids overwriting Project-specific policy state but
  means stale allow-list metadata can hide fresher catalog metadata if both are
  populated differently.

## 19. Reviewer Checklist

| Check | Status |
|---|---|
| Requirement fit: WEB-005 scan/deleted/status display parity | Pass |
| Scope limited to normalization/tests/proof | Pass |
| Existing REST routes and GUI transport preserved | Pass |
| No new Harbor client or infrastructure | Pass |
| SOLID and microservice ownership preserved | Pass |
| 12-factor config preserved | Pass |
| Secrets not printed | Pass |
| Focused tests concrete | Pass |
| Live proof concrete and cleanup-safe | Pass |
| Ledgers remain accurate | Pass |

Reviewer Agent result:

- Reviewer Agent `Rawls` returned `PASS — no blocking findings`.
- Residual risks are accepted and tracked as remaining gaps: this is
  catalog/read-model display parity only, the Playwright status assertion is
  env-gated, and malformed catalog boolean strings beyond canonical true/false
  are not yet covered by a full matrix.

## 20. Status

Status: Reviewer-approved partial WEB-005 completion. Full WEB-005/GA remains
open for automatic Harbor-to-catalog synchronization, full image-build /
allow-list / SBOM / signing / GUI scan workflow, OIDC browser login, WebRTC,
real GPU/log-tail evidence, 8-unit rollback, DR, failure injection, and
load/perf evidence.
