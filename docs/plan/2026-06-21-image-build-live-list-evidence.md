# Image Build Live List Evidence Slice

## 1. Objective

Replace the remaining `build_count=0` live Web GUI evidence with a seeded live
proof that an image build created through existing APIs appears in
`GET /api/v1/projects/{id}/image-builds`.

## 2. Background

The latest seeded `/ui/` E2E proves a real active Project, ConfigFile submit,
Project image listing, and active-Project GPU usage route success. It still
records `build_count=0` after creating and cancelling an image build.

Source inspection shows `imageregistry.createBuild` persists
`image-registry-service:image_build_jobs` with `project_id`, and
`imageregistry.listProjectBuilds` filters that resource by `project_id`. Focused
local imageregistry tests pass. Live `image-registry-service` is still running
the older `ci-ga-pdp-scope-20260620163744` image while newer backend images have
already been rolled to gateway/org-project/usage-observability services.

This slice is deployment/evidence only unless live proof contradicts that
finding.

## 3. Source References

- `gap.md`
- `docs/acceptance/gap-analysis.md`
- `docs/plan/2026-06-21-web-gui-active-project-live-e2e.md`
- `docs/plan/2026-06-21-web-gui-image-usage-contract.md`
- `backend/internal/services/imageregistry/handler.go`
- `backend/internal/services/imageregistry/spec.go`
- `frontend/tests/e2e/dashboard.spec.ts`

## 4. Assumptions

- The already-pushed current backend image
  `localhost:5000/nexuspaas-backend:ci-ga-clusterread-static-admin-20260621132623`
  contains the current image-registry code because the backend Docker image is
  shared by all backend services.
- Rolling only `image-registry-service` is enough to prove this route if the old
  live image was the cause.
- If the build list remains empty after rollout, stop and record the concrete
  live failure before proposing code.

## 5. Non-Goals

- No backend source change in this slice.
- No new image build API, fake data endpoint, or frontend behavior.
- No Harbor adapter integration work.
- No claim that full image workflow GA is complete.
- No cleanup route for image requests in this slice.

## 6. Current Behavior

Seeded live E2E creates an image request, approves it, starts an image build,
and then `GET /api/v1/projects/{id}/image-builds` returns an empty list
(`build_count=0`).

## 7. Target Behavior

After rolling `image-registry-service` to the current backend image, seeded live
E2E should record `build_count>=1` for the seeded Project before cleanup. The
test should continue to clean up the created image build through the existing
DELETE route.

## 8. Affected Domains

- Image-registry live deployment evidence.
- Web GUI Images panel live evidence.
- Acceptance ledgers.

## 9. Affected Files

- `docs/plan/2026-06-21-image-build-live-list-evidence.md`
- `docs/plan/2026-06-21-web-gui-active-project-live-e2e.md`
- `gap.md`
- `docs/acceptance/gap-analysis.md`

No production source file is expected to change.

## 10. API / Contract Changes

None. Existing REST/OpenAPI routes are used unchanged.

## 11. Database / Migration Changes

None.

## 12. Configuration Changes

No source config change. Runtime deployment image for `image-registry-service`
will change to a current backend image.

## 13. Observability Changes

No runtime telemetry change. Evidence should record deployment image, route
proof, and cleanup status without logging API keys.

## 14. Security Considerations

- Do not log runtime API keys.
- Keep backend RBAC as the only authorization source.
- Do not introduce fake image/build data.

## 15. Implementation Steps

- [ ] Record the current live `image-registry-service` image.
- [ ] Retag or reuse the already-pushed current backend image for
  `image-registry-service`; do not rebuild unless the image is missing.
- [ ] Roll only `image-registry-service` and wait for rollout.
- [ ] Run seeded live Web GUI E2E.
- [ ] If `build_count>=1`, update ledgers from `build_count=0` to image-build
  listing evidence.
- [ ] If `build_count` remains `0`, record the exact live evidence and draft a
  code slice instead.

## 16. Verification Plan

Focused local:

```sh
go -C backend test ./internal/services/imageregistry -run 'Build|ImageBuild|ProjectBuild|StaticAdmin|Spoofed' -count=1
```

Live:

```sh
kubectl -n nexuspaas set image deploy/image-registry-service app=<current-backend-image>
kubectl -n nexuspaas rollout status deploy/image-registry-service --timeout=180s
NEXUSPAAS_E2E_API_KEY=<runtime-key> NEXUSPAAS_E2E_SEED_PROJECT=true NEXUSPAAS_E2E_APP_PATH=/ui/ npm --prefix frontend run e2e
git diff --check
```

Regression only if source code changes become necessary:

```sh
go -C backend test ./... -count=1
go -C backend test ./... -coverprofile=coverage.out -count=1
bash backend/scripts/ci-security-gate.sh quick
bash backend/scripts/ci-security-gate.sh sonar
npm --prefix frontend run test
npm --prefix frontend run build
```

## 17. Rollback Plan

Roll `image-registry-service` back to the previously recorded image.

## 18. Risks and Tradeoffs

- Reusing the latest backend image avoids a rebuild and proves whether live
  drift, not code, caused the evidence gap.
- The semantic image tag was created for the clusterread slice, but the backend
  image is shared. If clearer release naming is needed later, retag the same
  digest instead of rebuilding.
- This still does not prove Harbor-side build execution; it proves the platform
  API records and lists the build request.

## 19. Reviewer Checklist

| Check | Status |
|---|---|
| Requirement fit: image build list live evidence | Pending |
| No source/API/DB/config change | Pending |
| Runtime rollout scope limited to image-registry-service | Pending |
| No credential leakage | Pending |
| Focused tests | Pending |
| Live seeded E2E | Pending |
| Ledger accuracy | Pending |
| Rollback recorded | Pending |
| Diff scope | Pending |

## 20. Status

Status: Approved

Implementation evidence:

- Previous live image:
  `localhost:5000/nexuspaas-backend:ci-ga-pdp-scope-20260620163744`.
- Rolled `image-registry-service` to:
  `localhost:5000/nexuspaas-backend:ci-ga-clusterread-static-admin-20260621132623`.
- Rollout succeeded, but seeded live E2E still recorded `build_count=0`.
- Direct live probe showed `POST /api/v1/images/build` through
  `platform-gateway` returned `HTTP 200` without a build id or project id, and
  `GET /api/v1/projects/{id}/image-builds` stayed empty.
- Conclusion: this was not only image-registry deployment drift. Follow-up code
  plan: `docs/plan/2026-06-21-gateway-adapter-route-proxy-precedence.md`.
- Follow-up result: gateway proxy precedence fix rolled `platform-gateway` to
  `localhost:5000/nexuspaas-backend:ci-ga-gateway-proxy-adapter-20260621054757`
  (`sha256:3cda2888dda836a1cd197c476c31342dd7e2f6f6befe5fa7e785ab46d13bc700`);
  seeded live E2E then recorded `build_count=1` and cleaned up the image build
  with `HTTP 200`.

Plan Agent checklist:

- [x] Requirement restated.
- [x] No-code/deployment-only scope stated.
- [x] Existing REST/OpenAPI contract preserved.
- [x] Rollback path included.
- [x] Reviewer Agent approval required before rollout.
- [x] Reviewer Agent approval received.
- [x] Code Agent implementation complete.
- [ ] Reviewer Agent final implementation approval received.
