# Storage CacheBinding API Fixture Parity Plan

Date: 2026-06-28
Scope: storage-service external API contract fixtures only

## 1. Objective

Bring typed external API fixture parity to the existing CacheBinding routes.
The create fixture already exists, so this slice adds list/get/update/delete
fixture coverage only. No runtime storage code changes are in scope.

## 2. Background

CacheBinding routes and handlers exist with a single create fixture. The
remaining list/get/update/delete routes lack typed external API fixtures, so
their contract surface is unverified for parity.

## 3. Source References

- `backend/internal/services/storage/spec.go`
- `backend/internal/services/storage/cache_bindings.go`
- `backend/internal/services/storage/cache_binding_test.go`
- `backend/internal/contracts/api_fixtures_test.go`
- `backend/internal/services/storage/api_fixtures_test.go`
- Existing fixture:
  `backend/internal/contracts/fixtures/api/v1/storage-create-cache-binding.json`

## 4. Assumptions

- CacheBinding routes, handlers, and response shapes are implemented and stable.
- Adding fixtures and parity checks does not change runtime behavior.

## 5. Non-Goals

- No runtime code changes.
- No new CacheBinding route, handler, event, or storage repository behavior.
- No live kind/RKE2/Kubernetes CRUD or authorization claim.
- No live cache residency or DataPlanePlan execution claim.
- No storage GA, Full GA, or V1 external production launch claim.

## 6. Current Behavior

Existing routes in `backend/internal/services/storage/spec.go`:

- `GET /api/v1/projects/{id}/storage/cache-bindings` -> `cache_bindings/list`
- `POST /api/v1/projects/{id}/storage/cache-bindings` -> `cache_bindings/create`
- `GET /api/v1/projects/{id}/storage/cache-bindings/{cacheBindingId}` -> `cache_bindings/get`
- `PUT /api/v1/projects/{id}/storage/cache-bindings/{cacheBindingId}` -> `cache_bindings/update`
- `DELETE /api/v1/projects/{id}/storage/cache-bindings/{cacheBindingId}` -> `cache_bindings/delete`

Only the create route has a fixture today.

## 7. Target Behavior

List/get/update/delete each have typed external REST fixtures with parity checks
against `storage.Spec()` for method/path/resource/action, auth posture, path
params, request/response shapes, statuses, and events.

## 8. Affected Domains

- Repository contract fixtures and storage-service contract tests only.

## 9. Affected Files

- `backend/internal/contracts/fixtures/api/v1/storage-list-cache-bindings.json`
- `backend/internal/contracts/fixtures/api/v1/storage-get-cache-binding.json`
- `backend/internal/contracts/fixtures/api/v1/storage-update-cache-binding.json`
- `backend/internal/contracts/fixtures/api/v1/storage-delete-cache-binding.json`
- `backend/internal/contracts/api_fixtures_test.go`
- `backend/internal/services/storage/api_fixtures_test.go`
- Ledger files if updated: `gap.md`, `problem.md`,
  `docs/acceptance/gap-analysis.md`

## 10. API / Contract Changes

Static fixtures and parity checks only. Checks must cover:

- fixture contract name, owner, API surface, consumer, resource, and action
- route method/path/resource/action alignment with `Spec()`
- auth posture: user auth required, no service key, project-scoped route
- path params: `id` for list; `id` + `cacheBindingId` for get/update/delete
- request fields: update can change CacheBinding payload fields; list/get/delete
  require no request body
- statuses: list/get/update/delete success and expected auth/not-found/errors
- events: update/delete emit `CacheBindingChanged`; list/get emit none
- response examples: list returns records; get/update return one record; delete
  matches the existing handler shape `{"id": "<cacheBindingId>", "deleted": true}`

## 11. Database / Migration Changes

None.

## 12. Configuration Changes

None.

## 13. Observability Changes

None.

## 14. Security Considerations

User auth required, no service key, project-scoped routes. No secrets or
operational values are added.

## 15. Implementation Steps

1. Add the four fixture files under
   `backend/internal/contracts/fixtures/api/v1/`.
2. Add those filenames and route metadata to
   `backend/internal/contracts/api_fixtures_test.go` (fixed expected file list
   and route map).
3. Add service-local parity checks in
   `backend/internal/services/storage/api_fixtures_test.go`, preferring one
   small helper/table over four copied blocks, covering the section 10 items.
4. Keep `cache_binding_test.go` focused on behavior; do not add runtime behavior
   assertions to this fixture-only slice unless Reviewer requests one.
5. Update ledgers only after tests pass, using bounded wording equivalent to:

> CacheBinding now has local/static typed external API fixture parity for
> list/get/update/delete alongside the existing create fixture. The evidence is
> contract/Spec parity and service-local fixture checks only; it does not prove
> live CRUD behavior, live authorization, node-local cache residency,
> DataPlanePlan runtime behavior, storage GA, Full GA, or V1 external launch
> readiness.

## 16. Verification Plan

Run from `backend/`:

```bash
go test ./internal/contracts/...
go test ./internal/services/storage -run 'CacheBinding.*Fixture|CacheBinding'
go test ./internal/services/storage/...
```

Run from repo root:

```bash
git diff --check
```

## 17. Rollback Plan

Delete the four fixture files and revert the two test files plus the scoped
ledger wording. No runtime rollback is needed.

## 18. Risks and Tradeoffs

- Static fixtures can be overclaimed; bounded ledger wording keeps this to
  contract/parity evidence only.

## 19. Reviewer Checklist

- The useful check here is parity, not another behavior test suite.
- Reviewer should reject any wording or fixture example that claims more than
  static external API contract coverage.

## 20. Status

Status: Approved
