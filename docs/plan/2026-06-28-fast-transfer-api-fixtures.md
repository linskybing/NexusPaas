# FastTransfer External API Fixtures Plan

Date: 2026-06-28
Status: Approved
Scope: storage-service FastTransfer custom external API contract fixtures

## Objective

Add typed external REST fixture parity for the existing FastTransfer custom
routes that make up the v2 lifecycle surface:

- start fast-stage transfer
- get transfer
- cancel transfer via DELETE

This is contract coverage only. Runtime behavior is already covered by focused
storage tests and kind slices; this plan does not change transfer execution.

## Current State

- Existing custom handlers in `backend/internal/services/storage/handler.go`:
  - `POST /api/v1/projects/{id}/storage/transfers/fast-stage`
  - `GET /api/v1/projects/{id}/storage/transfers/{targetNamespace}/{name}`
  - `DELETE /api/v1/projects/{id}/storage/transfers/{targetNamespace}/{name}`
- Existing Spec routes in `backend/internal/services/storage/spec.go` map those
  paths to `fast_transfers`.
- Existing event fixtures already cover:
  - `FastTransferQueued`
  - `FastTransferProgressed`
  - `FastTransferCompleted`
  - `FastTransferFailed`
- No external API fixtures currently exist for storage FastTransfer routes.

## Implementation Steps

1. Add three fixture files under
   `backend/internal/contracts/fixtures/api/v1/`:
   - `storage-start-fast-transfer.json`
   - `storage-get-fast-transfer.json`
   - `storage-cancel-fast-transfer.json`

2. Add those filenames and route metadata to
   `backend/internal/contracts/api_fixtures_test.go`.

3. Extend `backend/internal/services/storage/api_fixtures_test.go` with a small
   table-driven FastTransfer fixture parity test. Checks must cover:
   - contract name, owner, API surface, consumer, resource, action
   - route method/path/resource/action alignment with `Spec()`
   - user auth required, no service key
   - path params: `id` for start; `id`, `targetNamespace`, `name` for get/cancel
   - request body: start has optional transfer payload/idempotency fields;
     get/cancel have no request body
   - statuses: start returns `202`; get/cancel return `200`; auth, not-found,
     conflict, invalid-body, and generic error statuses match handler behavior
   - events: start emits `FastTransferChanged` and `FastTransferQueued`; get
     emits none; cancel emits `FastTransferChanged`
   - response examples use v2 fields: `status`, `progress_pct`, `bytes_total`,
     `bytes_done`, `checksum`, `resume_token`, and `idempotency_key`

4. Enforce these exact route/action/status/event expectations:
   - Start fixture: route action `command`, success `202`, errors
     `400/401/403/404/409`.
   - Get fixture: route action `get`, success `200`, errors `401/403/404`.
   - DELETE cancel fixture: route action `command`, success `200`, errors
     `401/403/404/409/500`.
   - Events: start emits `FastTransferChanged` and `FastTransferQueued`; get
     emits none; DELETE cancel emits `FastTransferChanged`.

5. After tests pass, update `gap.md`, `problem.md`, and
   `docs/acceptance/gap-analysis.md` with the bounded ledger wording below.

6. Keep existing runtime tests untouched unless Reviewer requests a correction.

## Deliberate Scope Boundary

This slice covers the three custom FastTransfer handler routes only. It does not
cover the generic/legacy transfer routes in `Spec()`:

- `GET /api/v1/projects/{id}/storage/transfers`
- `POST /api/v1/projects/{id}/storage/transfers`
- `POST /api/v1/projects/{id}/storage/transfers/{requestId}/cancel`

Those routes need a separate contract decision because they do not share the
custom handler response shape used by the v2 fast-stage/get/delete-cancel path.

## Ledger Boundary

After tests pass, update `gap.md`, `problem.md`, and
`docs/acceptance/gap-analysis.md` using bounded wording equivalent to:

> FastTransfer custom external API routes now have local/static typed fixture
> parity for fast-stage start, get, and DELETE cancel. This proves contract/Spec
> parity and response-shape documentation only; it does not prove live transfer
> execution, live authorization, live k8s-control callback delivery, bytes moved,
> checksum correctness, resume, external storage backends, storage GA, Full GA,
> or V1 external production launch readiness.

## Verification

Run from `backend/`:

```bash
go test ./internal/contracts/... -count=1
go test ./internal/services/storage -run 'FastTransfer.*Fixture|FastTransfer(Start|Progress|Cancel)' -count=1
go test ./internal/services/storage/... -count=1
```

Run from repo root:

```bash
git diff --check
```

## Non-Goals

- No runtime FastTransfer behavior changes.
- No mover Job, k8s-control, kind, or storage backend changes.
- No fixtures for generic/legacy transfer routes in this slice.
- No storage GA, Full GA, or V1 external production launch claim.

## Reviewer Notes

Reviewer should reject any fixture that implies live transfer execution or
claims that generic/legacy routes are covered. The useful check here is that the
custom v2 route contract is explicit and stays aligned with `Spec()` and the
existing handlers.

Status: Approved
