# Harbor Catalog Sync Execution

## 1. Objective

Move the remaining WEB-005/image-registry gap from manual catalog seeding toward
real Harbor-to-catalog synchronization by making the existing image-registry
service execute a bounded Harbor artifact metadata sync into `container_tags`.

## 2. Background

The current Project image GUI can display catalog/read-model fields, but no
production code writes `container_tags` from Harbor artifact metadata. The
existing `/api/v1/image-catalog/sync` route only records a `sync_requested`
status row.

Context7 was attempted for Harbor docs, but the configured API key is invalid.
Official Harbor docs and OpenAPI were checked instead. Harbor exposes REST API
Swagger at `/devcenter-api-2.0`, and the OpenAPI base path is `/api/v2.0`; the
artifact list paths include project-scoped artifact listing under
`/projects/{project_name_or_id}/artifacts` with query flags such as
`with_scan_overview`.

## 3. Source References

- `gap.md`
- `problem.md`
- `docs/acceptance/gap-analysis.md`
- `docs/plan/2026-06-21-web-gui-image-status-parity.md`
- `backend/internal/services/imageregistry/handler.go`
- `backend/internal/services/imageregistry/helpers.go`
- `backend/internal/services/imageregistry/harbor_health.go`
- `backend/internal/platform/adapter.go`
- `backend/internal/platform/maintenance.go`
- Official Harbor REST API docs:
  `https://goharbor.io/docs/2.10.0/working-with-projects/using-api-explorer/`
- Official Harbor OpenAPI:
  `https://github.com/goharbor/harbor/blob/main/api/v2.0/swagger.yaml`

## 4. Assumptions

- `HARBOR_URL` points at Harbor API v2.0 base or a path that can be used by the
  existing adapter/proxy rewrite config.
- The existing `harbor` adapter owns upstream credentials and must not expose
  them to callers or logs.
- Existing callers may send only `tag_id`; this must remain compatible. When a
  `tag_id` points to an existing catalog row, selectors are resolved from that
  row before declaring the sync target incomplete.
- Existing catalog rows or request payloads have enough identity fields
  (`project`, `repository`, `tag`, `digest`, or image reference) to match Harbor
  artifacts.
- This slice should improve synchronization evidence without claiming full
  image-build, SBOM, signing, or GA closure.

## 5. Non-Goals

- No new Harbor SDK/client dependency.
- No new service, controller, queue, CRD, or external scheduler.
- No bulk crawl across every Harbor project/repository.
- No SBOM/signing enforcement.
- No OIDC browser login, WebRTC, real GPU telemetry, or log tailing changes.
- No claim that `problem.md` or `gap.md` are empty.

## 6. Current Behavior

- `syncCatalog` writes only `imageSyncResource` status with
  `status="sync_requested"`.
- `listProjectImages` displays catalog-derived fields only if catalog records
  already exist.
- Harbor status checks use `adapter.Call`, which only probes a base URL.
- Path-aware Harbor access exists through the platform `ProxyAdapter` interface,
  but image-registry does not use it for catalog sync.

## 7. Target Behavior

- `POST /api/v1/image-catalog/sync` keeps returning `202`, but also attempts one
  bounded sync for the requested catalog target when the Harbor adapter supports
  proxy calls.
- The sync remains backward compatible with `{"tag_id":"..."}` calls: it first
  looks up the existing catalog row by `tag_id`, merges any selector fields from
  that row with the request payload, and only degrades when the combined target
  still lacks project/repository selectors.
- The sync fetches one Harbor artifact list for a specific project, filters by
  repository/tag/digest locally, parses digest, tag, scan overview/status,
  push/pull timestamps where available, and upserts a matching `container_tags`
  row.
- Sync status records move to `synced` or `degraded` with non-secret metadata.
- If Harbor is unavailable or the adapter lacks proxy support, the endpoint
  remains successful for local status tracking and records retryable degraded
  sync status.
- A lease-gated maintenance task retries existing `sync_requested`/`degraded`
  targets using the same helper. This uses existing platform maintenance; no new
  worker framework.

## 8. Affected Domains

- Image Registry: owns catalog/read-model sync behavior.
- Platform Adapter: reused through existing `contracts.ProxyAdapter`; no
  interface widening unless tests show it is unavoidable.
- Web GUI: benefits from already-implemented catalog field display; no frontend
  source changes planned.

## 9. Affected Files

- `backend/internal/services/imageregistry/handler.go`
- `backend/internal/services/imageregistry/helpers.go`
- `backend/internal/services/imageregistry/harbor_health.go`
- `backend/internal/services/imageregistry/handler_test.go`
- `gap.md`
- `problem.md`
- `docs/acceptance/gap-analysis.md`
- This plan file

If implementation stays small, prefer adding helpers to existing files over a
new source file.

## 10. API / Contract Changes

- Existing route stays:
  `POST /api/v1/image-catalog/sync`.
- Request payload may include existing fields plus repository selectors:
  `project`, `project_name`, `repository`, `repository_name`, `tag`, `tag_id`,
  `digest`, or `image_reference`.
- `tag_id` remains the stable sync key. Existing `{"tag_id":"tag-1"}` calls
  continue to return HTTP `202`.
- Success response/status record fields:
  - `id` and `tag_id`: requested sync key;
  - `status`: `synced`;
  - `catalog_id`: upserted catalog row id;
  - `synced_at`: UTC timestamp;
  - `degraded`: `false`;
  - `retryable`: `false`.
- Degraded response/status record fields:
  - `id` and `tag_id`: requested sync key;
  - `status`: `degraded`;
  - `degraded`: `true`;
  - `code`: one of `missing_selector`, `adapter_not_configured`,
    `adapter_unavailable`, `harbor_http_error`, `artifact_not_found`, or
    `catalog_persist_failed`;
  - `message`: non-secret operational explanation;
  - `retryable`: `true` except `missing_selector`.
- The endpoint does not return raw Harbor response bodies.
- No new public route is planned.

## 11. Database / Migration Changes

- No schema migration.
- Existing `platform_records` resources are used:
  - `image-registry-service:sync_targets`
  - `image-registry-service:container_tags`

## 12. Configuration Changes

- No new required config key.
- Use existing Harbor adapter config keys:
  - `HARBOR_URL` should point at the Harbor service root for proxy-capable
    sync.
  - `ADAPTER_CONFIG` may set Harbor `add_prefix` to `/api/v2.0`.
  - For private Harbor APIs, `ADAPTER_CONFIG` must live in the service runtime
    secret, not in a ConfigMap, and include the existing adapter `basic`,
    `bearer`, or `header` upstream auth configuration.

## 13. Observability Changes

- Reuse existing degraded metadata shape.
- Increment existing Harbor degraded metric when sync cannot reach Harbor.
- Store last sync status in `sync_targets` without credentials, auth headers, or
  raw Harbor response bodies.

## 14. Security Considerations

- Admin/user auth behavior for `syncCatalog` stays as-is for this slice.
- Do not log or return Harbor credentials, auth headers, Docker configs, decoded
  Kubernetes Secret values, or secret hashes.
- Proxy calls must use the service-owned Harbor adapter credential, not
  caller-supplied upstream auth.
- Catalog rows must not store secret-bearing response fields.

## 15. Implementation Steps

- [x] Add a small Harbor sync helper that accepts one sync target and optional
  `contracts.ProxyAdapter`.
- [x] Resolve a backward-compatible sync target from request payload plus
  existing catalog row by `tag_id`.
- [x] Build the Harbor project artifact-list path from resolved
  project/repository selectors, then filter the response by repository/tag/digest;
  if selectors are still missing, record `degraded` with
  `code="missing_selector"` instead of guessing.
- [x] Parse only the fields needed by existing UI/API:
  `digest`, `tag`, `scan_status`, `deleted`, `unavailable`, `status`, and
  timestamps when present.
- [x] Upsert one `container_tags` row idempotently while preserving unrelated
  existing catalog fields.
- [x] Update the sync status record to `synced` or `degraded`.
- [x] Register a lease-gated maintenance task that retries pending/degraded
  sync targets only when this process hosts `image-registry-service`.
- [x] Add focused backend tests for successful sync, degraded missing selector,
  degraded adapter/proxy failure, idempotent update, and maintenance retry.
- [x] Add registration tests proving the maintenance task is present for
  `image-registry-service`, absent for unrelated `SERVICE_NAME`, and executable
  via `RunMaintenanceOnce`.
- [x] Run focused image-registry tests.
- [x] Run local SonarScanner Quality Gate.
- [x] If production code changed, build/push a backend image and roll only
  `image-registry-service`.
- [x] Run bounded live proof against Harbor without printing secrets.
- [ ] Update ledgers with exact evidence and remaining gaps.
- [ ] Submit implementation to Reviewer Agent.

## 16. Verification Plan

Plan review:

- Reviewer Agent approves this plan before code changes.

Focused tests:

```sh
go -C backend test ./internal/services/imageregistry -run 'TestImageRegistryHarborCatalogSync|TestImageRegistryCatalogSync|TestImageRegistryHarborSyncMaintenance' -count=1
go -C backend test ./internal/services/imageregistry -count=1
```

Quality Gate:

```sh
bash backend/scripts/ci-security-gate.sh sonar
```

Live proof, if Harbor remains available:

- create or reuse one non-secret Harbor artifact already present from prior
  Harbor scan proof;
- call `POST /api/v1/image-catalog/sync` with explicit project/repository/tag
  selectors;
- verify `/api/v1/image-catalog/{tagId}/sync-status` becomes `synced`;
- verify `GET /api/v1/image-catalog` contains top-level digest/scan status for
  that tag;
- verify `GET /api/v1/projects/{id}/images` displays the synced fields if the
  image is published to a synthetic Project;
- clean exact synthetic rows and verify cleanup count.

No secret values, auth headers, Docker configs, decoded Kubernetes Secret values,
or secret hashes may be printed.

Actual evidence:

- Focused tests passed:
  `go -C backend test ./internal/services/imageregistry -run 'TestImageRegistryHarborCatalogSyncExecutesAndUpsertsCatalog|TestImageRegistryCatalogSyncDegradesWhenSelectorsAreMissing|TestImageRegistryHarborCatalogSyncDegradesWhenCatalogPersistFails|TestImageRegistryHarborSyncMaintenanceOwnerAndRetry|TestImageRegistryHarborSyncMaintenanceSkipsNonRetryableDegraded' -count=1`.
- Package tests passed:
  `go -C backend test ./internal/services/imageregistry -count=1`.
- Final full backend coverage passed:
  `go -C backend test -tags integration ./... -coverprofile=coverage.out -count=1`.
- Final SonarScanner Quality Gate passed. Sonar API readback:
  `new_coverage=81.6`, `new_violations=0`,
  `new_duplicated_lines_density=0.32514`, and
  `new_security_hotspots_reviewed=100.0`.
- Built and deployed final image
  `localhost:5000/nexuspaas-backend:ci-ga-harbor-catalog-sync-reviewfix-20260621224351`
  with digest
  `sha256:3730083b5b028d8a592de463892ced37b399c07bc68aef1471b9d80214168939`.
- Live Harbor artifact was pushed through a short-lived in-cluster `crane` pod:
  `library/nexuspaas-sync:ga-harbor-sync-20260621224455`, digest
  `sha256:73aaf090f3d85aa34ee199857f03fa3a95c8ede2ffd4cc2cdb5b94e566b11662`.
- Live API proof through `platform-gateway` returned HTTP `202` for
  `/api/v1/image-catalog/sync`, status `synced`, `code="ok"`,
  `degraded=false`, `retryable=false`, request/trace
  `654e8a882af7e6a2099a5cce75a8377e`.
- Live `GET /api/v1/image-catalog/{tagId}/sync-status` returned HTTP `200`,
  status `synced`, `code="ok"`, request/trace
  `4e80c3a5fc66d70aace670ac69751bdb`.
- Live `GET /api/v1/image-catalog` showed catalog id
  `harbor-sync-ga-harbor-sync-20260621224455`, repository
  `nexuspaas-sync`, tag `ga-harbor-sync-20260621224455`, digest
  `sha256:73aaf090f3d85aa34ee199857f03fa3a95c8ede2ffd4cc2cdb5b94e566b11662`,
  `deleted=false`, `unavailable=false`, and `status="available"`.
- Cleanup deleted the Harbor artifact/repository with HTTP `200`, deleted two
  exact `platform_records` rows (`container_tags` and `sync_targets`), verified
  DB remaining count `0`, API catalog matches `0`, sync lookup returned
  `not_synced`, and Harbor tag matches `0`.
- Reviewer Agent found four behavior risks; implementation now propagates
  catalog persistence failure as retryable degraded status, rejects
  repository-only targets instead of guessing, parses upstream
  `deleted`/`unavailable` booleans, and skips non-retryable degraded rows in
  maintenance retry.

## 17. Rollback Plan

- Roll back `image-registry-service` image if deployed.
- Delete exact synthetic `sync_targets`, `container_tags`, and publish rules
  created for live proof.
- Revert this slice's source/doc edits only if Reviewer rejects the approach.

## 18. Risks and Tradeoffs

- Harbor repository names can contain slashes; this slice avoids adapter
  double-escaping by using the project-level artifact list and filtering by
  repository locally.
- Harbor scan overview shape may vary by scanner/version; this slice stores a
  conservative text status and does not parse full vulnerability reports.
- Missing selector handling intentionally degrades instead of attempting a
  full Harbor crawl. A crawler can be added later if real operational need
  justifies it.
- This moves toward automatic sync but does not close the full
  image-build/allow-list/SBOM/signing/GUI scan workflow.

## 19. Reviewer Checklist

| Check | Status |
|---|---|
| Requirement fit: Harbor-to-catalog sync progress | Passed by implementation/live proof |
| Scope small and no new controller/service | Passed |
| No Harbor SDK/dependency added | Passed |
| Existing adapter/proxy and maintenance primitives reused | Passed |
| Image-registry owns catalog data changes | Passed |
| 12-factor config preserved | Passed: non-secret URL in ConfigMap, private API auth in Secret |
| Secrets not printed or stored | Passed in commands/evidence; no secret values recorded |
| Focused tests concrete | Passed |
| Live proof bounded and cleanup-safe | Passed; exact synthetic cleanup verified |
| Ledgers remain accurate and do not overclaim GA | Passed; ledgers updated with remaining GA gaps |

## 20. Status

Status: Reviewer approved after implementation and re-review.

Residual non-blocking follow-ups:

- Legacy `degraded` sync rows created before `retryable` existed may not be
  retried by maintenance unless resubmitted.
- Sync-status record persistence failures are still not surfaced separately
  from the in-memory response.
