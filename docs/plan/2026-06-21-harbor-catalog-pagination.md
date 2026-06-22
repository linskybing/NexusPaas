# Harbor Catalog Sync Bounded Pagination

## 1 Objective

Reduce false `artifact_not_found` results in explicit Harbor catalog sync by checking a bounded number of Harbor artifact-list pages before marking an existing catalog row missing.

## 2 Background

The delete-resync lifecycle slice now marks an existing catalog row `deleted=true`, `unavailable=true`, and `status="missing"` when Harbor no longer returns the artifact. The implementation currently reads one artifact-list page with `page_size=100`. Reviewer noted a non-blocking risk: if the matching artifact is on a later page, sync can incorrectly treat it as missing.

## 3 Source References

- `docs/plan/2026-06-21-harbor-delete-lifecycle-sync.md`
- `backend/internal/services/imageregistry/harbor_catalog_sync.go`
- `backend/internal/services/imageregistry/handler_test.go`
- Harbor v2 OpenAPI: `https://raw.githubusercontent.com/goharbor/harbor/main/api/v2.0/swagger.yaml`
- Harbor API Explorer docs: `https://goharbor.io/docs/2.10.0/working-with-projects/using-api-explorer/`

## 4 Assumptions

- This slice keeps the already live-proven Harbor adapter path and request model.
- Harbor list responses can be paginated with `page` and `page_size` query parameters, and may include `X-Total-Count` or `Link` headers.
- A small hard cap is acceptable for explicit sync because it avoids unbounded API calls while materially reducing false misses.
- Secret values and raw Harbor bodies must not be logged or printed.

## 5 Non-Goals

- No registry-wide crawler, watcher, queue, SDK replacement, or frontend change.
- No new API route, schema, service, dependency, or configuration variable.
- No claim of full Harbor lifecycle automation.

## 6 Current Behavior

`syncHarborCatalogTarget` calls Harbor once with `with_tag=true`, `with_scan_overview=true`, and `page_size=100`. If the target artifact is not in that first response body, the sync path records `artifact_not_found` and may mark an existing catalog row missing.

## 7 Target Behavior

`syncHarborCatalogTarget` should request page 1 first, then continue through a bounded number of pages until a matching artifact is found, Harbor returns fewer than `page_size` artifacts, or the hard page cap is reached. Adapter errors, degraded responses, and HTTP errors should preserve the current degraded status behavior.

## 8 Affected Domains

Image Registry service only.

## 9 Affected Files

- `backend/internal/services/imageregistry/harbor_catalog_sync.go`
- `backend/internal/services/imageregistry/handler_test.go`
- `docs/plan/2026-06-21-harbor-catalog-pagination.md`
- Ledgers only if live evidence is captured.

## 10 API / Contract Changes

No public API contract changes. Harbor adapter proxy requests add `page` and keep existing query keys.

## 11 Database / Migration Changes

No schema or migration changes.

## 12 Configuration Changes

No configuration changes. The page cap is a local constant in the image-registry implementation.

## 13 Observability Changes

No new logs or metrics. Existing sync status rows continue to report `synced`, `artifact_not_found`, or adapter/HTTP degraded statuses.

## 14 Security Considerations

Caller authorization and Harbor adapter credential handling remain unchanged. Tests and live verification must not print decoded secret values, auth headers, Docker config JSON, or raw Harbor response bodies.

## 15 Implementation Steps

1. Add small constants for Harbor artifact page size and maximum pages.
2. Change Harbor artifact lookup to loop pages through the existing `ProxyAdapter`.
3. Keep current degraded handling on adapter error, degraded result, and HTTP error.
4. Stop paging after a match, after a short page, or at the cap.
5. Add focused tests proving second-page match succeeds and later pages are not queried after a match.
6. Run focused tests, package tests, full integration coverage, and Sonar.
7. If live Harbor remains available, run a basic live sync smoke to ensure page query behavior does not regress existing sync.

## 16 Verification Plan

Focused tests:

```sh
go -C backend test ./internal/services/imageregistry -run 'TestImageRegistryHarborCatalogSyncFindsArtifactOnSecondPage|TestImageRegistryHarborCatalogSyncStopsPagingAfterMatch' -count=1
```

Package test:

```sh
go -C backend test ./internal/services/imageregistry -count=1
```

Full gate:

```sh
go -C backend test -tags integration ./... -coverprofile=coverage.out -count=1
bash backend/scripts/ci-security-gate.sh sonar
```

Implementation evidence captured on 2026-06-21:

- Focused tests passed:
  `go -C backend test ./internal/services/imageregistry -run 'TestImageRegistryHarborCatalogSyncFindsArtifactOnSecondPage|TestImageRegistryHarborCatalogSyncStopsPagingAfterMatch' -count=1`.
- Package tests passed:
  `go -C backend test ./internal/services/imageregistry -count=1`.
- Full integration coverage gate passed:
  `go -C backend test -tags integration ./... -coverprofile=coverage.out -count=1`.
- SonarScanner Quality Gate passed.
- Reviewer Agent approved implementation.
- No additional live ledger evidence was recorded for this slice because the
  behavioral risk is covered by focused paging tests and the previous
  delete-resync live proof remains the current deployed image.

## 17 Rollback Plan

Revert the image-registry pagination helper changes and redeploy the previous image-registry-service image. No data, schema, or config rollback is required.

## 18 Risks and Tradeoffs

- The page cap can still miss artifacts beyond the cap; this intentionally avoids unbounded calls.
- Paging increases worst-case explicit sync latency by a small bounded amount.
- Keeping the current adapter path avoids broad API-path churn but does not rework the Harbor integration into a full repository-specific crawler.

## 19 Reviewer Checklist

| Check | Status |
|---|---|
| Plan approved before code | Done |
| Scope stays in Image Registry | Done |
| No new dependency/config/route/schema | Done |
| Pagination is bounded | Done |
| Existing degraded behavior is preserved | Done |
| Tests cover second-page match and stop-after-match | Done |
| Ledgers avoid claiming full registry lifecycle automation | Done |

## 20 Status

Status: Approved
