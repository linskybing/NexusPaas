# FastTransfer External API Fixtures Plan

Date: 2026-06-28
Scope: storage-service FastTransfer custom external API contract fixtures

## 1. Objective

Add typed external REST fixture parity for the existing FastTransfer custom
routes that make up the v2 lifecycle surface:

- start fast-stage transfer
- get transfer
- cancel transfer via DELETE

This is contract coverage only. Runtime behavior is already covered by focused
storage tests and kind slices; this plan does not change transfer execution.

## 2. Background

The custom FastTransfer v2 routes have handlers and `Spec()` entries but no
external API fixtures, so the typed contract surface is unverified for parity.
This slice closes that gap with static fixtures only.

## 3. Source References

- `backend/internal/services/storage/handler.go`
- `backend/internal/services/storage/spec.go`
- `backend/internal/contracts/api_fixtures_test.go`
- `backend/internal/services/storage/api_fixtures_test.go`
- Existing event fixtures: `FastTransferQueued`, `FastTransferProgressed`,
  `FastTransferCompleted`, `FastTransferFailed`

## 4. Assumptions

- The custom handler routes and their response shapes are already implemented
  and stable.
- Adding fixtures and parity checks does not change runtime behavior.

## 5. Non-Goals

- No runtime FastTransfer behavior changes.
- No mover Job, k8s-control, kind, or storage backend changes.
- No fixtures for generic/legacy transfer routes in this slice.
- No storage GA, Full GA, or V1 external production launch claim.

## 6. Current Behavior

- Existing custom handlers in `backend/internal/services/storage/handler.go`:
  - `POST /api/v1/projects/{id}/storage/transfers/fast-stage`
  - `GET /api/v1/projects/{id}/storage/transfers/{targetNamespace}/{name}`
  - `DELETE /api/v1/projects/{id}/storage/transfers/{targetNamespace}/{name}`
- Existing Spec routes in `backend/internal/services/storage/spec.go` map those
  paths to `fast_transfers`.
- No external API fixtures currently exist for storage FastTransfer routes.

## 7. Target Behavior

The three custom routes have typed external REST fixtures with parity checks
against `storage.Spec()` for method/path/resource/action, auth posture, path
params, request/response shapes, statuses, and emitted events.

## 8. Affected Domains

- Repository contract fixtures and storage-service contract tests only.

## 9. Affected Files

- `backend/internal/contracts/fixtures/api/v1/storage-start-fast-transfer.json`
- `backend/internal/contracts/fixtures/api/v1/storage-get-fast-transfer.json`
- `backend/internal/contracts/fixtures/api/v1/storage-cancel-fast-transfer.json`
- `backend/internal/contracts/api_fixtures_test.go`
- `backend/internal/services/storage/api_fixtures_test.go`
- Ledger files after tests pass: `gap.md`, `problem.md`,
  `docs/acceptance/gap-analysis.md`

## 10. API / Contract Changes

Static fixtures and parity checks only. The fixtures must enforce:

- Start fixture: route action `command`, success `202`, errors
  `400/401/403/404/409`; emits `FastTransferChanged` and `FastTransferQueued`.
- Get fixture: route action `get`, success `200`, errors `401/403/404`; emits
  no events.
- DELETE cancel fixture: route action `command`, success `200`, errors
  `401/403/404/409/500`; emits `FastTransferChanged`.
- Path params: `id` for start; `id`, `targetNamespace`, `name` for get/cancel.
- Response examples use v2 fields: `status`, `progress_pct`, `bytes_total`,
  `bytes_done`, `checksum`, `resume_token`, `idempotency_key`.

## 11. Database / Migration Changes

None.

## 12. Configuration Changes

None.

## 13. Observability Changes

None.

## 14. Security Considerations

User auth required, no service key. No secrets or operational values are added.

## 15. Implementation Steps

1. Add the three fixture files under
   `backend/internal/contracts/fixtures/api/v1/`.
2. Add those filenames and route metadata to
   `backend/internal/contracts/api_fixtures_test.go`.
3. Extend `backend/internal/services/storage/api_fixtures_test.go` with a small
   table-driven FastTransfer fixture parity test covering contract name/owner/
   surface/consumer/resource/action, route alignment with `Spec()`, auth posture,
   path params, request bodies (start optional payload/idempotency; get/cancel
   none), statuses, events, and v2 response example fields.
4. Enforce the exact route/action/status/event expectations in section 10.
5. After tests pass, update `gap.md`, `problem.md`, and
   `docs/acceptance/gap-analysis.md` with the bounded ledger wording below.
6. Keep existing runtime tests untouched unless Reviewer requests a correction.

Ledger boundary wording (after tests pass):

> FastTransfer custom external API routes now have local/static typed fixture
> parity for fast-stage start, get, and DELETE cancel. This proves contract/Spec
> parity and response-shape documentation only; it does not prove live transfer
> execution, live authorization, live k8s-control callback delivery, bytes moved,
> checksum correctness, resume, external storage backends, storage GA, Full GA,
> or V1 external production launch readiness.

Deliberate scope boundary: this slice covers the three custom FastTransfer
handler routes only. It does not cover the generic/legacy transfer routes in
`Spec()` (`GET`/`POST /api/v1/projects/{id}/storage/transfers` and
`POST /api/v1/projects/{id}/storage/transfers/{requestId}/cancel`), which need a
separate contract decision because they do not share the custom v2 response shape.

## 16. Verification Plan

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

## 17. Rollback Plan

Delete the three fixture files and revert the two test files plus the scoped
ledger wording. No runtime rollback is needed.

## 18. Risks and Tradeoffs

- Static fixtures can be overclaimed; the bounded ledger wording keeps the slice
  to contract/parity evidence only.

## 19. Reviewer Checklist

- Reviewer should reject any fixture that implies live transfer execution or
  claims the generic/legacy routes are covered.
- The useful check is that the custom v2 route contract is explicit and stays
  aligned with `Spec()` and the existing handlers.

## 20. Status

Status: Approved
