# Identity Token Lifecycle Evidence

Status: Implemented; Reviewer approved

Reviewer: Final implementation review approved

## Objective

Close the `Strengthen: SEC/CLI token lifecycle` gap by proving the existing
identity implementation enforces session expiry, refresh-token rotation/replay
rejection, API-token expiry/revocation, and credential cleanup at handler and
internal-contract boundaries. This is an evidence-only slice; if any focused
test exposes a production enforcement gap, implementation must stop and a
revised plan must be reviewed before production code changes.

## Current Evidence

- `gap.md` marks session + refresh-token expiry/rotation as not implemented.
- The repository layer already creates access sessions and refresh tokens with
  `expires_at`, rejects expired sessions, consumes/deletes refresh tokens, and
  cleans expired session/refresh/API-token records.
- `/api/v1/refresh` already calls `ConsumeRefreshToken` and then issues a new
  session pair.
- Internal identity auth contracts already call `FindValidSession` and
  `FindActiveAPITokenByRaw`, but current tests only cover happy-path
  authorization plus missing/invalid credentials.
- CLI API-token create/list/revoke behavior and indexed lookup already have
  focused tests.

## Scope

- Add focused tests proving:
  - `/api/v1/refresh` rotates both access and refresh tokens,
  - replaying the old refresh token after rotation returns `401`,
  - the new refresh token remains usable for one subsequent rotation,
  - internal session auth rejects expired sessions,
  - internal API-token auth rejects expired and revoked API tokens.
- Reuse the existing `cleanup_test.go` coverage proving auth cleanup removes
  expired session/refresh/API-token rows and revoked API tokens while retaining
  live credentials; do not modify cleanup behavior in this slice.
- Keep existing TTLs and token formats unchanged.
- Update `gap.md`, `docs/acceptance/gap-analysis.md`, and this plan with
  evidence if tests pass.
- If a focused test exposes a real enforcement gap, stop and write a revised
  production-fix plan for Reviewer approval before changing implementation code.

## Non-Goals

- No OIDC/JWKS library replacement in this slice.
- No service-to-service identity replacement for `SERVICE_API_KEY`.
- No API-token format changes.
- No new dependency.
- No production code changes.
- No live rollout unless local checks expose runtime risk that needs live API
  evidence.

## Affected Files

- `backend/internal/services/identity/workflow_test.go`
- `backend/internal/services/identity/handler_test.go`
- `backend/internal/services/identity/cleanup_test.go` (existing coverage reused
  unchanged)
- `docs/plan/2026-06-21-identity-token-lifecycle-evidence.md`
- `gap.md`
- `docs/acceptance/gap-analysis.md`

## Verification

```sh
go -C backend test ./internal/services/identity -run 'TokenLifecycle|RegisterLoginRefreshLogout|InternalIdentityAuthContracts|CleanupExpiredAuthRecords' -count=1
go -C backend test ./internal/services/identity
go -C backend test ./...
bash backend/scripts/ci-security-gate.sh quick
git diff --check
```

## Implementation Evidence

- Added handler-level refresh lifecycle evidence:
  - login issues access and refresh tokens,
  - `/api/v1/refresh` rotates both tokens,
  - replaying the old refresh token returns `401`,
  - the newly rotated refresh token can be used once for another rotation.
- Added internal identity auth contract evidence:
  - expired sessions return `401`,
  - expired API tokens return `401`,
  - revoked API tokens return `401`,
  - valid API tokens still update `last_used_at` without leaking token hashes.
- Reused existing cleanup coverage proving expired sessions, expired refresh
  tokens, expired API tokens, and revoked API tokens are pruned while live
  credentials remain.
- No production code was changed.
- Updated `gap.md` and `docs/acceptance/gap-analysis.md` so SEC/CLI token
  lifecycle is no longer listed as not implemented.
- Verification passed:
  - `go -C backend test ./internal/services/identity -run 'TokenLifecycle|RegisterLoginRefreshLogout|InternalIdentityAuthContracts|CleanupExpiredAuthRecords' -count=1`
  - `go -C backend test ./internal/services/identity`
  - `go -C backend test ./...`
  - `bash backend/scripts/ci-security-gate.sh quick`
  - `git diff --check`

## Reviewer Result

- Reviewer found no blocking findings and approved final implementation review.
- Reviewer confirmed no production code changed.
- Reviewer reran:
  - `go -C backend test ./internal/services/identity -run 'TokenLifecycle|RegisterLoginRefreshLogout|InternalIdentityAuthContracts|CleanupExpiredAuthRecords' -count=1`
  - `go -C backend test ./internal/services/identity`
  - `go -C backend test ./...`
  - `bash backend/scripts/ci-security-gate.sh quick`
  - `git diff --check`

## Acceptance

- Refresh tokens are one-time-use and rotate on refresh.
- Expired sessions and expired/revoked API tokens are rejected by internal auth
  contracts.
- Expired/revoked credential cleanup is covered by tests.
- `gap.md` no longer lists SEC/CLI token lifecycle as not implemented.
