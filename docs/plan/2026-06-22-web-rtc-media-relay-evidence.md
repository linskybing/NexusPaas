# Web RTC Media Relay Evidence

## 1. Objective

Close the next smallest WEB/RTC evidence gap by extending the existing live
Playwright GUI proof to record:

- browser-native direct ICE candidate gathering;
- browser-native forced TURN relay candidate gathering using the already issued
  short-lived TURN credentials;
- non-empty job log tail visibility when a live fixture provides logs;
- nonzero Project GPU usage visibility when a live fixture provides GPU usage.

This slice must not introduce a new media stack, WebRPC transport, backend API,
database schema, or permanent test-only route.

## 2. Background

`WEB-006` credential issuance through the first-party GUI is now evidenced, but
`RTC-008` still lacks browser ICE proof. The WEB tracker also still calls out
real workload log tailing/full status and nonzero GPU usage evidence. The
existing GUI already calls the required APIs:

- `POST /api/v1/stream/credentials`
- `GET /api/v1/jobs/{id}/logs`
- `GET /api/v1/projects/{id}/gpu-usage`

The remaining gap is evidence quality, not a need for a new product surface.

## 3. Source References

- `docs/acceptance/gap-analysis.md`
- `docs/acceptance/webrtc.md`
- `gap.md`
- `problem.md`
- `frontend/src/App.tsx`
- `frontend/src/App.test.tsx`
- `frontend/src/api.ts`
- `frontend/tests/e2e/dashboard.spec.ts`
- `backend/internal/services/workload/stream_credentials.go`
- `backend/internal/services/workload/job_access_handlers.go`
- `backend/internal/services/clusterread/handler.go`
- Official Playwright docs for `page.evaluate()` and `page.waitForResponse()`
- MDN/WebRTC docs for `RTCPeerConnection` and ICE server configuration

Context7 was attempted for Playwright docs but the configured Context7 API key
was invalid, so official Playwright and MDN documentation were used as the
fallback source.

## 4. Assumptions

- GA v1 WebRPC GUI remains the approved same-origin REST/OpenAPI GUI contract.
- Browser ICE probing can use native `RTCPeerConnection` from Playwright
  `page.evaluate()`; no JavaScript WebRTC library is needed.
- Live forced TURN relay proof requires a browser-reachable TURN URI. If the
  current cluster TURN URI is service-internal only, the live run must
  temporarily use a browser-reachable URI, then restore config without printing
  secret values.
- RTC-008 can be marked passed only when the probe runs against a named staging
  environment and the proof records that staging context in the same route-proof
  JSON as both direct and forced-relay ICE results. Local preview or developer
  laptop runs are rehearsal evidence only.
- Non-empty logs and nonzero GPU usage are environment data requirements. The
  E2E harness can enforce them when explicitly requested, but it must not fake
  them in production code.

## 5. Non-Goals

- Do not add Selkies, Janus, mediasoup, LiveKit, WebRTC gateway code, or a
  custom signaling server.
- Do not add a new backend API, debug route, database table, or migration.
- Do not persist or print TURN passwords or shared secrets.
- Do not claim RTC-017 real GPU browser streaming unless a real GPU node media
  session is separately proven.
- Do not claim PERF-003..008 from this slice.
- Do not rewrite the GUI layout or API client style.

## 6. Current Behavior

- The GUI can request stream credentials and shows redacted TURN metadata.
- The Playwright proof records stream credential status, URI count, username
  presence, password issuance, and GUI redaction.
- The Playwright proof records `job_logs_count`, but the latest live run had
  `0` logs and did not assert visible non-empty tail content.
- The Playwright proof records GPU route status only; it does not record or
  assert `used > 0`.
- No browser ICE candidate probe exists for direct or forced relay paths.

## 7. Target Behavior

- When `NEXUSPAAS_E2E_RTC_ICE_PROBE=true`, Playwright gathers direct ICE
  candidates in the browser and requires at least one direct candidate.
- When `NEXUSPAAS_E2E_RTC_ICE_PROBE=true`, Playwright gathers candidates with
  `iceTransportPolicy: "relay"` and the issued TURN credentials, then requires
  at least one relay candidate.
- The proof prints only safe metadata: booleans/counts/candidate types, never
  TURN passwords.
- The route proof includes `job_logs_nonempty`, `job_logs_visible`, `gpu_used`,
  and `gpu_nonzero`.
- The same route proof includes `rtc_probe_environment`, `rtc_direct_ok`,
  `rtc_direct_candidate_count`, `rtc_direct_candidate_types`, `rtc_relay_ok`,
  `rtc_relay_candidate_count`, and `rtc_relay_candidate_types`. The RTC-008
  success condition is `rtc_probe_environment="staging"`, `rtc_direct_ok=true`,
  and `rtc_relay_ok=true` in one run.
- Optional enforcement flags can make live E2E fail when log/GPU fixtures are
  absent:
  - `NEXUSPAAS_E2E_EXPECT_NONEMPTY_LOGS=true`
  - `NEXUSPAAS_E2E_EXPECT_NONZERO_GPU=true`
- Unit tests assert the GUI renders the nonzero Project GPU pod count, not just
  the heading.

## 8. Affected Domains

- Frontend E2E proof harness.
- Frontend Usage panel test coverage.
- Workload stream credential evidence.
- Usage-observability read-model evidence.

No new service boundary is introduced. Ownership remains:

- Workload service owns job/log/stream credential APIs.
- Usage-observability service owns Project GPU usage read APIs.
- Frontend owns browser proof and rendering tests.

## 9. Affected Files

- `frontend/tests/e2e/dashboard.spec.ts`
- `frontend/src/App.test.tsx`
- `docs/plan/2026-06-22-web-rtc-media-relay-evidence.md`
- After live proof only: `docs/acceptance/webrtc.md`,
  `docs/acceptance/gap-analysis.md`, `gap.md`, `problem.md`

## 10. API / Contract Changes

None.

The E2E harness consumes the existing stream credential, job logs, and Project
GPU usage contracts.

## 11. Database / Migration Changes

None.

If live evidence needs synthetic environment data, use an explicit one-off
admin fixture against existing owned resources and exact cleanup. Do not commit
schema changes or production fixture routes.

## 12. Configuration Changes

No committed config changes.

Live RTC-008 proof may temporarily require:

- browser-reachable `STREAM_TURN_URIS`, preferably TURN/TCP if local
  port-forwarding is used;
- existing `STREAM_TURN_SHARED_SECRET` kept in Kubernetes Secrets;
- restoration of previous config/Secrets after proof.

## 13. Observability Changes

No runtime telemetry changes.

Playwright proof output will add safe fields for ICE candidate counts/types,
log visibility, and GPU usage count.

## 14. Security Considerations

- TURN password stays in browser memory only long enough to run the optional
  ICE probe.
- Proof logs must not print password, shared secret, full credential payloads,
  or browser storage contents.
- Same-origin credential behavior remains unchanged.
- Backend authorization remains the source of truth for stream credential,
  logs, and GPU usage access.

## 15. Implementation Steps

1. Add E2E environment flags in `frontend/tests/e2e/dashboard.spec.ts` for RTC
   ICE probing, staging proof context, non-empty log enforcement, and nonzero
   GPU enforcement. The RTC probe flag must require an explicit
   `NEXUSPAAS_E2E_ENVIRONMENT=staging` value before it can produce passing
   RTC-008 evidence.
2. Extend `SeedState` with safe proof fields for direct ICE, relay ICE, log
   visibility, and GPU `used`.
3. After the stream credential UI proof, optionally call a browser-native
   `RTCPeerConnection` probe through `page.evaluate()`:
   - direct mode: no TURN credentials, default ICE policy;
   - relay mode: issued TURN URI/username/password, `iceTransportPolicy:
     "relay"`;
   - return only candidate counts/types and completion status.
4. Parse job log response rows and, when non-empty, assert at least one log line
   is visible in the GUI table.
5. Parse Project GPU usage response and record `used`; enforce `>0` only when
   the explicit environment flag is enabled.
6. Extend route proof JSON with the new safe fields, including staging context
   and both direct/relay ICE results in the same logged object.
7. Strengthen `frontend/src/App.test.tsx` to assert the Project GPU pods value
   `2` is rendered in the existing nonzero fixture.
8. Run focused frontend tests and build.
9. If live TURN/log/GPU fixtures are available, run live Playwright with
   enforcement enabled and update acceptance trackers with evidence.
10. If live fixtures are not available, update only the plan evidence and keep
    the corresponding acceptance rows open.

## 16. Verification Plan

- `npm --prefix frontend test -- --run`
- `npm --prefix frontend run build`
- `npm --prefix frontend run e2e` against local preview or live gateway with
  default optional enforcement disabled.
- Live RTC-008 proof command with enforcement enabled once TURN is browser
  reachable in staging:
  `NEXUSPAAS_E2E_ENVIRONMENT=staging NEXUSPAAS_E2E_STREAM_CREDENTIALS=true NEXUSPAAS_E2E_RTC_ICE_PROBE=true npm --prefix frontend run e2e`
- Passing RTC-008 evidence requires one route-proof JSON line from that staging
  run containing `rtc_probe_environment="staging"`,
  `rtc_direct_ok=true`, and `rtc_relay_ok=true`.
- Live WEB log/GPU proof command once fixtures exist:
  `NEXUSPAAS_E2E_EXPECT_NONEMPTY_LOGS=true NEXUSPAAS_E2E_EXPECT_NONZERO_GPU=true npm --prefix frontend run e2e`
- If frontend files changed only, backend quick gate is not required for the
  implementation diff. Before tracker closure, keep the already refreshed
  backend gate evidence: `make -C backend check`, `make -C backend coverage`,
  and `make -C backend ci-sonar`.

## 17. Rollback Plan

Revert the Playwright harness and frontend unit-test changes.

If live runtime TURN config was temporarily patched, restore the previous
config/Secret values without printing secret material and roll affected backend
pods back to ready state.

## 18. Risks and Tradeoffs

- ICE candidate gathering can be browser/network dependent. The probe is
  opt-in so normal smoke runs remain stable.
- Direct ICE proof may gather mDNS-obfuscated host candidates; candidate type is
  enough for this acceptance proof and does not expose IPs.
- Synthetic DB fixtures can prove GUI/read API behavior but not real workload
  telemetry. They must be labeled as synthetic if used.
- Full real GPU media streaming remains a separate RTC-017 slice.

## 19. Reviewer Checklist

| Item | Status |
|---|---|
| Requirement fit for RTC-008 / WEB evidence | Passed |
| Scope limited to existing GUI/E2E proof harness | Passed |
| No backend API/schema/transport added | Passed |
| No secret logging or persistence | Passed |
| SOLID boundaries preserved | Passed |
| 12-Factor config respected | Passed |
| Verification commands concrete | Passed |
| Live evidence rules avoid false completion claims | Passed |

## 20. Status

Status: Approved

## 21. Implementation Evidence

- `frontend/tests/e2e/dashboard.spec.ts` now adds explicit proof flags:
  `NEXUSPAAS_E2E_ENVIRONMENT`, `NEXUSPAAS_E2E_RTC_ICE_PROBE`,
  `NEXUSPAAS_E2E_EXPECT_NONEMPTY_LOGS`, and
  `NEXUSPAAS_E2E_EXPECT_NONZERO_GPU`.
- The seeded route proof now emits one JSON object containing log visibility,
  GPU usage, and RTC fields:
  `job_logs_nonempty`, `job_logs_visible`, `gpu_used`, `gpu_nonzero`,
  `rtc_probe_environment`, `rtc_direct_ok`, `rtc_direct_candidate_count`,
  `rtc_direct_candidate_types`, `rtc_relay_ok`,
  `rtc_relay_candidate_count`, and `rtc_relay_candidate_types`.
- The optional RTC probe uses browser-native `RTCPeerConnection` from
  Playwright `page.evaluate()` and returns only candidate counts/types and
  booleans. It does not log TURN passwords, shared secrets, or raw candidates.
- `frontend/src/App.test.tsx` now asserts the nonzero Project GPU pod count
  (`2`) is rendered, not just the summary label.

## 22. Verification Evidence

- `npm --prefix frontend test -- --run` passed: 2 files, 18 tests.
- `npm --prefix frontend run build` passed.
- `npm --prefix frontend run e2e` passed with 1 skipped test because no live
  E2E API key/OIDC credentials were provided in this local run.
- Current live RKE2/staging GUI RTC proof passed through
  `http://127.0.0.1:18080/ui/` with
  `NEXUSPAAS_E2E_ENVIRONMENT=staging`,
  `NEXUSPAAS_E2E_STREAM_CREDENTIALS=true`, and
  `NEXUSPAAS_E2E_RTC_ICE_PROBE=true`.
- The proof seeded Project `e2e-p-mqongkuq-oov6qe`, ConfigFile `CFG2600010`,
  and streaming Job `e2e-job-mqongkuq-oov6qe`; stream credentials returned
  `stream_credentials_status=200`, `stream_credential_uri_count=1`,
  `stream_credential_password_issued=true`, and
  `stream_credential_password_redacted=true`.
- The same route-proof JSON recorded `rtc_probe_environment="staging"`,
  `rtc_direct_ok=true`, `rtc_direct_candidate_count=2`,
  `rtc_direct_candidate_types=["host"]`, `rtc_relay_ok=true`,
  `rtc_relay_candidate_count=1`, and `rtc_relay_candidate_types=["relay"]`.
- For the proof only, `STREAM_TURN_URIS` was temporarily changed to
  browser-reachable `turn:127.0.0.1:3478?transport=udp`; it was restored to
  `turn:coturn.nexuspaas.svc.cluster.local:3478?transport=udp`, and
  `workload-service` rolled out ready after restore.
- Exact leftover readback for Job `e2e-job-mqongkuq-oov6qe`, cancel command
  `3afd9c4e324c737f669ed517dc16f152`, and image request
  `e2e-img-e2e-p-mqongkuq-oov6qe` returned zero rows after cleanup/cascade.

RTC-008 direct/relay candidate gathering is now evidenced. WEB log/GPU evidence
remains open because the same live run recorded `job_logs_count=0`,
`job_logs_visible=false`, `gpu_status=502`, and `gpu_nonzero=false`.
