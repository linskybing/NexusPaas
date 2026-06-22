# Harbor Delete Lifecycle Sync

## 1 Objective

Close the next small WEB-005/image-registry gap by making an explicit Harbor
catalog sync mark an existing catalog row as deleted and unavailable when Harbor
no longer returns the artifact.

## 2 Background

The previous Harbor catalog sync slice proved that `POST
/api/v1/image-catalog/sync` can import Harbor artifact metadata into the
image-registry `container_tags` read model. The remaining acceptance ledgers
still list Harbor delete/scan lifecycle work because the current bounded sync
does not update an already-imported catalog row after the upstream artifact is
deleted.

## 3 Source References

- `gap.md`
- `problem.md`
- `docs/acceptance/gap-analysis.md`
- `docs/plan/2026-06-21-harbor-catalog-sync-execution.md`
- `backend/internal/services/imageregistry/harbor_catalog_sync.go`
- `backend/internal/services/imageregistry/handler_test.go`
- Harbor API Explorer docs:
  `https://goharbor.io/docs/2.10.0/working-with-projects/using-api-explorer/`
- Harbor v2 OpenAPI:
  `https://github.com/goharbor/harbor/blob/main/api/v2.0/swagger.yaml`

## 4 Assumptions

- Explicit re-sync is the trigger for this slice; there is no Harbor event
  watcher, crawler, queue, or periodic repository-wide scan in scope.
- An existing catalog row proves the platform previously knew the artifact
  identity, so a later Harbor artifact miss should make that row honest for the
  GUI by marking it missing.
- A missing artifact without an existing catalog row should not create a
  synthetic deleted row.
- Raw Harbor response bodies, credentials, Kubernetes Secret values, auth
  headers, Docker config JSON, and secret hashes must not be logged or printed.

## 5 Non-Goals

- No Harbor event watcher, crawler, SDK replacement, queue worker, or new route.
- No frontend change.
- No SBOM, signing, vulnerability parsing, scan summary UI, or registry-wide
  lifecycle automation.
- No attempt to close every WEB or GA acceptance gap in this slice.

## 6 Current Behavior

- A found Harbor artifact upserts `container_tags` as available.
- A missing Harbor artifact records a `sync_targets` row with
  `status="degraded"` and `code="artifact_not_found"`.
- Existing `container_tags` rows are not updated to reflect Harbor deletion, so
  the GUI-facing catalog can remain stale after a proven upstream delete.

## 7 Target Behavior

- If a sync target has an existing catalog row and Harbor returns no matching
  artifact, update that catalog row:
  - `deleted=true`
  - `unavailable=true`
  - `status="missing"`
  - `updated_at=<sync time>`
- Keep sync status explicit and non-secret:
  - `status="degraded"`
  - `code="artifact_not_found"`
  - `retryable=true`
- If no catalog row exists, preserve the current degraded-only behavior.

## 8 Affected Domains

- Image Registry service only.
- Platform record store read models owned by Image Registry:
  `image-registry-service:container_tags` and
  `image-registry-service:sync_targets`.

## 9 Affected Files

- `backend/internal/services/imageregistry/harbor_catalog_sync.go`
- `backend/internal/services/imageregistry/handler_test.go`
- `docs/acceptance/gap-analysis.md`
- `gap.md`
- `problem.md`
- This plan file

## 10 API / Contract Changes

- Reuse the existing `POST /api/v1/image-catalog/sync` route and request shape.
- Reuse the existing catalog read contract; after explicit re-sync an existing
  row may now report `deleted=true`, `unavailable=true`, and `status="missing"`.
- No new public endpoint, dependency, service, or HTTP status contract.

## 11 Database / Migration Changes

- No schema migration.
- Continue using existing `platform_records` rows for `container_tags` and
  `sync_targets`.

## 12 Configuration Changes

- No new configuration.
- Continue using the existing Harbor adapter/runtime configuration.

## 13 Observability Changes

- Reuse the existing sync status row for operator visibility.
- Do not add logs that include raw Harbor bodies or secrets.
- Keep `artifact_not_found` degraded and retryable because a miss can still be
  caused by transient permissions, replication, or query timing.

## 14 Security Considerations

- Caller authorization stays unchanged on the existing sync endpoint.
- No credential format or Secret contract changes.
- The implementation only updates image-registry-owned read model records.
- Live testing must not print decoded secret values, auth headers, Docker config
  JSON, or secret hashes.

## 15 Implementation Steps

1. Add a small helper in `harbor_catalog_sync.go` that updates an existing
   catalog row to `deleted=true`, `unavailable=true`, and `status="missing"`.
2. In the Harbor artifact-not-found branch, call the helper before recording the
   degraded sync status.
3. If the missing-row update fails, record `catalog_persist_failed` as the
   degraded sync status, matching the previous catalog persistence behavior.
4. If no catalog row exists, leave the current degraded-only behavior unchanged.
5. Add focused unit tests for both branches: existing row becomes missing, and
   unknown row remains degraded-only.
6. Update acceptance ledgers only after evidence exists, and do not overclaim
   full Harbor delete/scan lifecycle automation.

## 16 Verification Plan

Focused tests:

```sh
go -C backend test ./internal/services/imageregistry -run 'TestImageRegistryHarborCatalogSyncMarksMissingCatalogDeleted|TestImageRegistryHarborCatalogSyncDegradesWhenMissingCatalogUpdateFails|TestImageRegistryHarborCatalogSyncMissingWithoutCatalogStaysDegradedOnly' -count=1
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

Live proof, if Harbor remains available:

1. Push one temporary Harbor artifact.
2. Sync it and verify catalog `status="available"`.
3. Delete the Harbor artifact.
4. Sync the same tag again and verify catalog `deleted=true`,
   `unavailable=true`, and `status="missing"`.
5. Delete exact synthetic platform rows and verify zero leftovers.

Implementation evidence captured on 2026-06-21:

- Focused tests passed:
  `go -C backend test ./internal/services/imageregistry -run 'TestImageRegistryHarborCatalogSyncMarksMissingCatalogDeleted|TestImageRegistryHarborCatalogSyncDegradesWhenMissingCatalogUpdateFails|TestImageRegistryHarborCatalogSyncMissingWithoutCatalogStaysDegradedOnly' -count=1`.
- Package tests passed:
  `go -C backend test ./internal/services/imageregistry -count=1`.
- Full integration coverage gate passed:
  `go -C backend test -tags integration ./... -coverprofile=coverage.out -count=1`.
- SonarScanner Quality Gate passed.
- Reviewer Agent approved implementation.
- Live proof ran on image
  `localhost:5000/nexuspaas-backend:ci-ga-harbor-delete-lifecycle-20260621225732`
  (`sha256:68b5eefe30ee0644edb4e5146523bce82ad83ea572d31fc7c7be6d5f3d1cccca`).
- Temporary Harbor artifact
  `library/nexuspaas-sync:ga-harbor-delete-lifecycle-20260621225849`
  synced available with digest
  `sha256:73aaf090f3d85aa34ee199857f03fa3a95c8ede2ffd4cc2cdb5b94e566b11662`.
- Harbor delete returned `200`, exact tag lookup returned `404`, re-sync returned
  `status="degraded"`, `code="artifact_not_found"`, `retryable=true`, and the
  catalog row reported `deleted=true`, `unavailable=true`, `status="missing"`.
- Cleanup deleted 2 exact platform rows, API catalog leftovers were `0`, sync
  status returned `not_synced`, Harbor repository artifacts were empty, and
  port-forward listeners were closed.

## 17 Rollback Plan

- Roll back `image-registry-service` to the previous deployed image.
- Delete only the exact synthetic `container_tags` and `sync_targets` rows used
  by live proof.
- No database migration rollback or configuration rollback is required.

## 18 Risks and Tradeoffs

- This is explicit-sync lifecycle handling, not automatic Harbor lifecycle
  automation.
- An artifact miss can represent a transient registry issue, so the catalog row
  is marked unavailable for UI honesty while sync status remains degraded and
  retryable.
- Not creating deleted rows for unknown artifacts avoids inventing catalog data
  that the platform never previously imported.
- The change keeps responsibilities local to Image Registry and avoids new
  abstractions or dependencies.

## 19 Reviewer Checklist

| Check | Status |
|---|---|
| Plan approved before code | Done |
| Scope stays inside Image Registry | Done |
| SOLID responsibilities remain local and small | Done |
| 12-Factor config contract is unchanged | Done |
| No new dependency, service, route, or schema | Done |
| Existing adapter and record store reused | Done |
| Missing artifact updates only existing catalog rows | Done |
| Unknown missing artifact remains degraded-only | Done |
| Tests prove deleted/unavailable lifecycle | Done |
| Live proof cleanup exact and non-secret | Done |
| Ledgers do not overclaim full WEB/GA closure | Done |

## 20 Status

Status: Approved
