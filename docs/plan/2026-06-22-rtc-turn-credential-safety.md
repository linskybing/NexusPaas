# RTC TURN Credential Safety

## 1. Objective

Prove the existing `POST /api/v1/stream/credentials` path satisfies the
credential-safety parts of RTC acceptance: RTC-006 short-lived TURN credentials
and RTC-007 no TURN shared-secret disclosure.

## 2. Background

The previous Web GUI stream slice proved the browser can request stream
credentials and redacts the issued password. `docs/acceptance/webrtc.md` still
lists RTC-006 and RTC-007 as WebRTC security criteria. The backend already
generates ephemeral TURN REST credentials from `STREAM_TURN_SHARED_SECRET`, so
this slice should add evidence, not a new streaming implementation.

## 3. Source References

- `docs/acceptance/webrtc.md`
- `docs/acceptance/gap-analysis.md`
- `gap.md`
- `problem.md`
- `backend/internal/services/workload/stream_credentials.go`
- `backend/internal/services/workload/stream_credentials_test.go`

## 4. Assumptions

- TURN REST username format `expiryUnix:user-session` is the accepted expiry
  mechanism for coturn-style shared-secret credentials.
- `expires_at`, `ttl_seconds`, and generated HMAC password are safe to test.
- The shared secret value itself must never appear in response payloads, docs,
  or test logs.

## 5. Non-Goals

- Do not implement direct ICE, forced TURN relay, media E2E, or RTC-008.
- Do not add a browser media client, WebRTC SDK, WebRPC/tRPC/gRPC transport, or
  dependency.
- Do not change credential API semantics unless a test exposes a bug.

## 6. Current Behavior

`streamCredentials` reads the job, verifies project access, requires an active
streaming job, requires TURN config, caps requested TTL to
`StreamTURNCredentialTTL`, returns `expires_at`, and derives the TURN password
from HMAC-SHA1 over the username. Existing tests cover a happy path cap and
invalid session states, but they do not explicitly prove expiry-window behavior,
default TTL behavior, invalid/zero TTL behavior, or response non-disclosure of
the shared secret.

## 7. Target Behavior

- Requested TTL above max is capped.
- Missing, zero, and negative requested TTL use the configured max/default TTL.
- Positive TTL below max is honored.
- `expires_at` parses as RFC3339 and falls within the expected time window.
- TURN username expiry prefix matches the returned expiry.
- TURN password equals the expected HMAC and is not the shared secret.
- The serialized response does not contain the configured shared-secret value.

## 8. Affected Domains

- Workload service stream credential tests.
- Acceptance trackers for RTC credential safety.

No new service, data owner, or deployable unit is introduced.

## 9. Affected Files

- `backend/internal/services/workload/stream_credentials_test.go`
- `docs/plan/2026-06-22-rtc-turn-credential-safety.md`
- `docs/acceptance/webrtc.md`
- `docs/acceptance/gap-analysis.md`
- `gap.md`
- `problem.md`

## 10. API / Contract Changes

None planned. This is evidence for the existing response contract.

## 11. Database / Migration Changes

None.

## 12. Configuration Changes

None committed. Existing test config sets in-memory TURN URI, shared secret,
and TTL.

## 13. Observability Changes

None. Tests must not log the shared secret.

## 14. Security Considerations

- Do not print the shared secret.
- Do not document the live shared secret.
- Assert serialized credential responses do not contain the shared-secret
  string.
- Keep this scoped to credential safety; do not claim full RTC media security.

## 15. Implementation Steps

1. Add focused table-driven test coverage in
   `backend/internal/services/workload/stream_credentials_test.go` for max cap,
   below-max TTL, missing TTL, zero TTL, and negative TTL.
2. Parse `expires_at` in tests and compare it to before/after request windows.
3. Assert username expiry prefix matches `expires_at.Unix()`.
4. Assert password equals `streamTURNPassword(secret, username)` and differs
   from the secret.
5. Assert serialized response body does not contain the shared-secret string.
6. Update `docs/acceptance/webrtc.md`, `docs/acceptance/gap-analysis.md`,
   `gap.md`, and `problem.md` with RTC-006/RTC-007 evidence and explicit
   RTC-008/media non-claim.

## 16. Verification Plan

- `cd backend && go test ./internal/services/workload -run StreamCredentials -count=1`
- `cd backend && go test ./internal/services/workload -run '^$' -count=1`
- `make -C backend lint`
- `make -C backend build`
- `git diff --check -- backend/internal/services/workload/stream_credentials_test.go docs/plan/2026-06-22-rtc-turn-credential-safety.md docs/acceptance/webrtc.md docs/acceptance/gap-analysis.md gap.md problem.md`

No frontend build is required because this slice does not touch frontend code.

Optional live proof is not required for this slice because the previous live
browser/k6 run already proved live credential issuance. If run anyway, it must
record only non-secret booleans and exact cleanup status.

## 17. Rollback Plan

Revert the test additions and tracker updates. No runtime config, deployment,
or migration rollback is needed.

## 18. Risks and Tradeoffs

- This closes credential safety evidence only; it does not prove TURN relay,
  browser peer connectivity, bitrate enforcement, session caps, egress budget,
  or media metrics.
- Tests use the existing in-memory app instead of a live coturn handshake. That
  is enough for RTC-006/RTC-007 credential generation and non-disclosure, not
  RTC-008.

## 19. Reviewer Checklist

| Item | Status |
|---|---|
| RTC-006 expiry/cap/default behavior explicitly tested | Passed |
| RTC-007 shared-secret non-disclosure explicitly tested | Passed |
| No API, DB, config, dependency, or transport expansion | Passed |
| No secret values added to docs/logs | Passed |
| Trackers avoid RTC-008/media overclaim | Passed |

## 20. Status

Status: Approved

## 21. Implementation Evidence

- Added focused RTC credential-safety tests in
  `backend/internal/services/workload/stream_credentials_test.go`.
- Updated `docs/acceptance/webrtc.md`, `docs/acceptance/gap-analysis.md`,
  `gap.md`, and `problem.md` with RTC-006/RTC-007 evidence and explicit
  RTC-008/media non-claim.

## 22. Verification Evidence

- `cd backend && go test ./internal/services/workload -run StreamCredentials -count=1`
  passed.
- `cd backend && go test ./internal/services/workload -run '^$' -count=1`
  passed.
- `cd backend && go vet ./internal/services/workload` passed.
- `gofmt -l backend/internal/services/workload/stream_credentials_test.go`
  returned no files.
- `make -C backend build` passed.
- `git diff --check -- backend/internal/services/workload/stream_credentials_test.go docs/plan/2026-06-22-rtc-turn-credential-safety.md docs/acceptance/webrtc.md docs/acceptance/gap-analysis.md gap.md problem.md`
  passed.
- `make -C backend lint` did not pass because an unrelated dirty file,
  `backend/internal/services/service_isolation_test.go`, is currently not
  gofmt-formatted. This slice did not edit that file.

## 23. Reviewer Decision

Reviewer `Dirac` approved the implementation. No blocking findings remained.
The only note was to ensure untracked docs/tracker files are included in the
final commit set.
