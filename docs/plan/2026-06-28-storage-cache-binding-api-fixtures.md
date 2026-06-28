# Storage CacheBinding API Fixture Parity Plan

Date: 2026-06-28
Status: Approved
Scope: storage-service external API contract fixtures only

## Objective

Bring typed external API fixture parity to the existing CacheBinding routes.
The create fixture already exists, so this slice adds list/get/update/delete
fixture coverage only. No runtime storage code changes are in scope.

## Current State

- Existing fixture:
  `backend/internal/contracts/fixtures/api/v1/storage-create-cache-binding.json`.
- Existing routes in `backend/internal/services/storage/spec.go`:
  - `GET /api/v1/projects/{id}/storage/cache-bindings` -> `cache_bindings/list`
  - `POST /api/v1/projects/{id}/storage/cache-bindings` -> `cache_bindings/create`
  - `GET /api/v1/projects/{id}/storage/cache-bindings/{cacheBindingId}` -> `cache_bindings/get`
  - `PUT /api/v1/projects/{id}/storage/cache-bindings/{cacheBindingId}` -> `cache_bindings/update`
  - `DELETE /api/v1/projects/{id}/storage/cache-bindings/{cacheBindingId}` -> `cache_bindings/delete`
- Existing handlers/tests:
  `backend/internal/services/storage/cache_bindings.go` and
  `backend/internal/services/storage/cache_binding_test.go`.

## Implementation Steps

1. Add four fixture files under
   `backend/internal/contracts/fixtures/api/v1/`:
   - `storage-list-cache-bindings.json`
   - `storage-get-cache-binding.json`
   - `storage-update-cache-binding.json`
   - `storage-delete-cache-binding.json`

2. Add those filenames and route metadata to
   `backend/internal/contracts/api_fixtures_test.go`; the contract registry has
   a fixed expected fixture file list and route map.

3. Add service-local parity checks in
   `backend/internal/services/storage/api_fixtures_test.go`.
   Prefer one small helper/table over four copied blocks. Checks must cover:
   - fixture contract name, owner, API surface, consumer, resource, and action
   - route method/path/resource/action alignment with `Spec()`
   - auth posture: user auth required, no service key, project-scoped route
   - path params: `id` for list; `id` + `cacheBindingId` for get/update/delete
   - request fields: update can change CacheBinding payload fields; list/get/delete
     should not require a request body
   - statuses: list/get/update/delete success and expected auth/not-found/errors
   - events: update/delete emit `CacheBindingChanged`; list/get emit none
   - response examples: list returns cache binding records; get/update return one
     cache binding record; delete matches the existing handler response shape:
     `{"id": "<cacheBindingId>", "deleted": true}`

4. Keep `cache_binding_test.go` focused on behavior. Do not add runtime behavior
   assertions to this fixture-only slice unless Reviewer requests one.

## Ledger Boundary

Update ledgers only after tests pass, and use bounded wording equivalent to:

> CacheBinding now has local/static typed external API fixture parity for
> list/get/update/delete alongside the existing create fixture. The evidence is
> contract/Spec parity and service-local fixture checks only; it does not prove
> live CRUD behavior, live authorization, node-local cache residency,
> DataPlanePlan runtime behavior, storage GA, Full GA, or V1 external launch
> readiness.

Ledger files, if updated:
- `gap.md`
- `problem.md`
- `docs/acceptance/gap-analysis.md`

## Verification

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

## Non-Goals

- No runtime code changes.
- No new CacheBinding route, handler, event, or storage repository behavior.
- No live kind/RKE2/Kubernetes CRUD or authorization claim.
- No live cache residency or DataPlanePlan execution claim.
- No storage GA, Full GA, or V1 external production launch claim.

## Reviewer Notes

The useful check here is parity, not another behavior test suite. Reviewer
should reject any wording or fixture example that claims more than static
external API contract coverage.

Status: Approved
