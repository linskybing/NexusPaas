# Web GUI Stream Credentials

## 1. Objective

Close the smallest useful WEB-006 and PERF-007 credential-path gap by letting
the first-party `/ui/` Workloads panel request short-lived stream/TURN
credentials for an authorized streaming job through the existing backend
REST/OpenAPI contract, then proving the same endpoint under k6 concurrency where
TURN runtime config is available.

## 2. Background

The backend already exposes `POST /api/v1/stream/credentials` in
`workload-service`. Current Web GUI evidence covers active Project selection,
ConfigFile submission, job submission, job list, job logs, cancel, Images, and
Usage, but it does not expose any browser operation for stream credentials.
`docs/acceptance/performance.md` also includes a `Stream credential p95 <= 300
ms` target and `PERF-007` WebRTC concurrency acceptance.

The existing approved "WebRPC GUI" decision for GA v1 is same-origin
REST/OpenAPI consumption. This slice does not introduce WebRPC, tRPC, gRPC, or a
parallel browser transport.

## 3. Source References

- `docs/acceptance/gap-analysis.md`
- `docs/acceptance/performance.md`
- `docs/acceptance/webrtc.md`
- `gap.md`
- `problem.md`
- `backend/internal/services/workload/stream_credentials.go`
- `backend/docs/browser-gpu-streaming.md`
- `frontend/src/api.ts`
- `frontend/src/App.tsx`
- `frontend/tests/e2e/dashboard.spec.ts`
- `backend/scripts/perf/core-read-100vu.js`
- `backend/scripts/perf/stream-credentials-100vu.js` if a separate harness is
  cleaner than extending the current read harness
- `backend/Makefile`

## 4. Assumptions

- A "WebRPC GUI" requirement is satisfied for this repository by a reviewed
  first-party GUI API contract over same-origin REST/OpenAPI until a concrete
  transport gap is proven.
- WEB-006 can be advanced by proving the GUI can request stream credentials for
  an authorized streaming job. Full direct ICE and forced TURN relay validation
  remain RTC acceptance scope.
- PERF-007 can be advanced only for the credential issuance path in this slice;
  it does not prove media egress or browser peer connectivity.
- Live TURN runtime config may need temporary, secret-safe setup for a successful
  credential issuance proof.

## 5. Non-Goals

- Do not add a new WebRPC/tRPC/gRPC transport.
- Do not build a full WebRTC peer connection client or Selkies iframe in this
  slice.
- Do not print, persist, screenshot, or document TURN passwords or shared
  secrets.
- Do not claim RTC-008 forced TURN relay, stream bitrate, packet-loss, or egress
  budget acceptance.
- Do not claim full PERF-007 WebRTC concurrency unless credential issuance and
  media path evidence both exist; this slice only targets the stream credential
  p95 sub-target.
- Do not change backend stream credential semantics unless tests prove a bug.

## 6. Current Behavior

- The GUI lists jobs and can request logs/cancel, but has no stream credential
  action.
- `frontend/src/api.ts` has no client method for `/api/v1/stream/credentials`.
- The live Playwright seeded job is currently a generic job. The stream proof
  needs a deterministic streaming job seed so GUI E2E and k6 target the same
  `streaming_session=true` job id.

## 7. Target Behavior

- Jobs that look like streaming-capable jobs expose an "Open stream" action in
  the Workloads panel.
- The GUI calls `POST /api/v1/stream/credentials` with the selected job id and a
  short UI session id.
- On success, the GUI displays non-secret connection metadata: job id, TURN URI
  count/first URI, username, TTL, expiry, and a redacted password indicator.
- On backend rejection, the GUI shows a clear error state without losing the job
  list.
- The API client tests prove request payload/header/response handling.
- UI unit tests prove the GUI does not persist stream credentials in browser
  storage, and Playwright proves live redacted credential display.

## 8. Affected Domains

- Frontend Workloads GUI.
- Workload service API consumption from the GUI.
- Live browser E2E evidence for WEB-006.

No new backend bounded context or data owner is introduced.

## 9. Affected Files

- `frontend/src/types.ts`
- `frontend/src/api.ts`
- `frontend/src/api.test.ts`
- `frontend/src/App.tsx`
- `frontend/src/App.test.tsx`
- `frontend/tests/e2e/dashboard.spec.ts`
- `backend/scripts/perf/stream-credentials-100vu.js`
- `backend/Makefile`
- `docs/plan/2026-06-22-web-gui-stream-credentials.md`
- `docs/acceptance/gap-analysis.md`
- `gap.md`
- `problem.md`

## 10. API / Contract Changes

No backend API contract change is planned.

The frontend will add a typed client wrapper for the existing
`POST /api/v1/stream/credentials` response shape. The GUI must treat the TURN
password as a sensitive transient value and must not store it.

## 11. Database / Migration Changes

None.

Live E2E seeds a synthetic Project, Queue, Plan, ConfigFile, and a deterministic
streaming job through existing APIs. Cleanup remains best-effort because
existing job cleanup routes are limited.

## 12. Configuration Changes

No committed config change is planned.

For live proof, temporary runtime setup may set `STREAM_TURN_URIS` and
`STREAM_TURN_SHARED_SECRET` in the appropriate Kubernetes runtime config/secret
without printing secret values. Any temporary patch must be restored or recorded.

## 13. Observability Changes

No new backend metrics are planned.

Live proof should record only non-secret facts: HTTP status, job id, TURN URI
count, TTL, username presence, expiry presence, and redacted password presence.

## 14. Security Considerations

- Do not store stream credentials in `localStorage`, `sessionStorage`, URL query
  strings, logs, screenshots, or docs.
- Do not display the TURN password; show a redacted indicator only.
- Keep same-origin credentials enabled so OIDC cookie auth continues to work.
- Continue relying on backend Project/job authorization for stream credential
  access.

## 15. Implementation Steps

1. Add stream credential response/request types in `frontend/src/types.ts`.
2. Add `streamCredentials(jobID, sessionID)` to `frontend/src/api.ts`.
3. Add API tests for the wrapper, same-origin credentials, JSON body, and
   response envelope handling.
4. Extend `WorkloadsPanel` with a stream state and "Open stream" action for
   streaming-capable jobs.
5. Render a compact "Stream session" status block with redacted credential
   handling and clear errors.
6. Extend frontend unit tests to cover successful stream credential request,
   error handling, and no browser storage persistence.
7. Extend Playwright seeded E2E with a deterministic streaming seed:
   - submit a ConfigFile whose content includes `streaming_session: true`,
     `stream_max_bitrate_kbps`, and stream-related metadata;
   - submit the seeded job with `streaming_session: true` and
     `stream_max_bitrate_kbps` fields through the existing job API;
   - store the seeded streaming job id in `SeedState`;
   - use the same job id for GUI stream credential proof and k6 via
     `NEXUSPAAS_PERF_STREAM_JOB_ID`.
8. Request stream credentials from the GUI for the seeded streaming job and
   print redacted route-proof metadata only.
9. Add a focused k6 harness for `POST /api/v1/stream/credentials` with endpoint
   status counters and p95 summary output.
10. Add a `make -C backend perf-k6-stream-credentials` target if a separate
   harness is used.
11. Update `gap.md`, `problem.md`, and `docs/acceptance/gap-analysis.md` with
   the result.

## 16. Verification Plan

- `npm --prefix frontend test -- --run`
- `npm --prefix frontend run build`
- `cd backend && go test ./internal/services/workload -run 'Stream|Credential' -count=1`
- `node --check backend/scripts/perf/stream-credentials-100vu.js` if added
- `make -C backend coverage`
- `make -C backend ci-sonar`
- Live browser E2E through `platform-gateway` with seeded Project/job:
  `npm --prefix frontend run e2e` with the existing live environment variables.
- The live proof must record the exact seeded streaming job id and verify that
  direct API readback shows `streaming_session=true` before GUI/k6 credential
  requests run.
- If TURN is configured live, prove `POST /api/v1/stream/credentials` returns
  200 and GUI displays redacted stream metadata.
- If TURN is configured live, run k6 with at least `100` VUs against
  `/api/v1/stream/credentials` and require p95 `<=300ms`, failure rate `0`, and
  `0` 429/5xx for the credential endpoint.
- If TURN is not configured live, record a non-passing degraded proof and keep
  WEB-006/PERF-007 open.

## 17. Rollback Plan

Revert the frontend client/UI/test changes and docs updates. No data migration or
backend API rollback is required.

If live runtime config was patched for TURN proof, restore the prior config and
secret values without printing secret material, then restart only affected
backend deployments.

## 18. Risks and Tradeoffs

- This advances WEB-006 but does not prove full WebRTC peer connectivity or TURN
  relay behavior.
- This advances PERF-007 only for credential issuance; media concurrency remains
  open.
- Displaying any credential metadata increases sensitivity, so the UI must
  redact password material and avoid storage.
- Live proof may depend on stream/TURN runtime configuration that is currently
  empty in k3s runtime config.
- Adding a full typed RPC transport would be overbuilt for this slice and would
  duplicate existing REST/OpenAPI routes.

## 19. Reviewer Checklist

| Item | Status |
|---|---|
| Requirement fit for WEB-006 GUI stream operation | Passed for credential operation; full WebRTC media/relay remains RTC scope |
| Existing REST/OpenAPI contract preserved | Passed; no new WebRPC/tRPC/gRPC transport added |
| No secret persistence or plaintext display | Passed; UI stores only redacted credential metadata and unit tests assert no browser storage persistence |
| SOLID and frontend component responsibility preserved | Passed; API wrapper, types, Workloads state, and stream status rendering stay in existing frontend boundaries |
| 12-Factor config/secrets respected | Passed; TURN URI remains config, shared secret remains Kubernetes Secret, values were not logged |
| Local frontend/backend tests defined | Passed |
| k6 stream credential p95 proof defined | Passed |
| Live E2E/k6 proof or explicit non-passing reason recorded | Passed for stream credential issuance |

## 20. Status

Status: Implemented; reviewer approved

## 21. Implementation Evidence

- Frontend added typed `streamCredentials` API consumption for
  `POST /api/v1/stream/credentials`, streaming job submission fields, an "Open
  stream" Workloads action, and a redacted stream-session metadata panel.
- The GUI never renders or stores the TURN password; it keeps only a
  `passwordIssued` boolean after redaction.
- Playwright seeded streaming jobs by setting `streaming_session=true` and
  `stream_max_bitrate_kbps=12000`, then requested stream credentials from the
  browser and recorded only redacted proof fields.
- k6 harness `backend/scripts/perf/stream-credentials-100vu.js` and Make target
  `perf-k6-stream-credentials` exercise credential issuance with per-VU API
  keys and endpoint-scoped thresholds.
- Backend image rolled live:
  `localhost:5000/nexuspaas-backend:ci-ga-web-stream-cred-20260622102018`
  (`sha256:d14aa360d5f0e4273846c88a785a2ad8cafc570613e8d892a7d9ef4407c899b1`).
- Runtime TURN config was enabled without printing secret values:
  `STREAM_TURN_URIS=turn:coturn.nexuspaas.svc.cluster.local:3478?transport=udp`
  and `STREAM_TURN_SHARED_SECRET` copied from the existing coturn static-auth
  secret into the 15 backend runtime Secrets because production profile
  validation requires the shared secret whenever TURN URIs are configured.

## 22. Verification Evidence

- `node --check backend/scripts/perf/stream-credentials-100vu.js` passed.
- `npm --prefix frontend test -- --run` passed: 2 test files, 18 tests.
- `npm --prefix frontend run build` passed.
- `cd backend && go test ./internal/services/workload -run 'Stream|Credential' -count=1`
  passed.
- `make -C backend coverage` passed with total coverage `82.1%`.
- `make -C backend ci-sonar` passed. Sonar API readback:
  `status=OK`, `new_coverage=81.8`, `new_violations=0`,
  `new_security_hotspots_reviewed=100.0`,
  `new_duplicated_lines_density=0.8262`.
- Live Playwright through `http://127.0.0.1:18080/ui/` passed with seeded
  Project `e2e-p-mqom1t1b-pa2jbl`, ConfigFile `CFG2600009`, streaming Job
  `e2e-job-mqom1t1b-pa2jbl`, `seeded_job_streaming=true`,
  `stream_credentials_status=200`, `stream_credential_uri_count=1`,
  `stream_credential_username_present=true`, and
  `stream_credential_password_issued=true`; the GUI proof records
  `stream_credential_password_redacted=true`.
  The exact Playwright job and cancel-command rows were removed after the run
  because the product has no job DELETE route yet.
- Live k6 stream-credential run passed: `100` VUs for `30s`,
  `auth_key_count=100`, `/api/v1/stream/credentials` `3000/3000` 2xx,
  failure rate `0`, p95 `22.926812599999987ms`, and `0` 429/4xx/5xx.
- Post-run cleanup restored gateway/workload runtime Secrets to
  `API_KEYS=1`, removed all `100` temporary stream k6 policy rows, deleted the
  exact k6 seed Group/Project/Queue/Plan via API (`200` each), and removed the
  exact k6 Job record. Exact seed record readback returned no rows.
- All live first-party backend deployments were `1/1` ready on
  `ci-ga-web-stream-cred-20260622102018` after cleanup.

## 23. Reviewer Decision

Reviewer `Ptolemy` initially found one proof-semantics issue: the route-proof
field named `stream_credential_password_redacted` was derived from API password
presence. The E2E proof now emits separate
`stream_credential_password_issued` and `stream_credential_password_redacted`
fields, and sets the redaction field only after the browser verifies that the
stream panel displays `redacted` and does not display the plaintext password.

Reviewer re-check found no remaining issues and approved this slice.
