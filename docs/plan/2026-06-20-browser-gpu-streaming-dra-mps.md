# Browser GPU Graphics Streaming on DRA MPS

## 1. Objective

Package browser-operated GPU graphics sessions on the existing NexusPaas
workload job + ConfigFile path by adding the missing Selkies streaming
artifacts, TURN REST credentials, and public-internet egress guardrails.

Do not rebuild GPU sharing. Reuse the existing DRA `ResourceClaimTemplate` +
NVIDIA MPS path that already injects MPS config, accounts fractional GPU usage
at admission, and reports MPS usage telemetry.

## 2. Background

The target user workflow is Isaac Sim, Omniverse Kit, CAD/CAE, and 3D
visualization from a browser. The selected runtime shape is Selkies WebRTC +
GStreamer + NVENC, scheduled as a Kubernetes workload job. Public internet
users require TURN relay support and admission guardrails so stream sessions do
not oversubscribe a 1 Gbps egress link.

Local source review confirmed the important reuse point:

- `backend/internal/services/workload/dispatcher_dra.go` builds
  `ResourceClaimTemplate` objects with NVIDIA opaque MPS config.
- `backend/internal/services/schedulerquota/admission_resources.go` already
  accounts DRA/MPS GPU fractions as `gpu_count * sm_percentage / 100`.
- `backend/internal/services/gpuusage/collector.go` already records per-pod MPS
  usage metadata.
- `backend/internal/services/workload/job_submit.go` sends job metadata and
  manifests through scheduler admission before dispatch.
- `backend/internal/platform/proxy_stream.go` already supports streaming proxy
  upgrades for existing proxy routes, but not dynamic per-job upstreams.

External docs checked during planning:

- Kubernetes DRA is stable in current docs and workloads use
  `ResourceClaim` / `ResourceClaimTemplate` to claim devices.
- Selkies docs recommend TURN REST API authentication for multi-user
  environments and support dynamic TURN config through REST/config JSON.
- coturn supports `use-auth-secret` and `static-auth-secret`; TURN REST
  temporary credentials are `expiry:username` plus base64 HMAC-SHA1.

## 3. Source References

- `AGENTS.md`: requires Plan Agent -> Reviewer Agent approval -> Code Agent ->
  Reviewer Agent final approval.
- `docs/agents/workflow.md`, `docs/agents/planning.md`,
  `docs/agents/review-checklist.md`, `docs/agents/coding-guidelines.md`.
- `backend/internal/services/workload/dispatcher_dra.go`.
- `backend/internal/services/schedulerquota/admission.go`.
- `backend/internal/services/schedulerquota/admission_decode.go`.
- `backend/internal/services/schedulerquota/admission_quota.go`.
- `backend/internal/services/schedulerquota/admission_resources.go`.
- `backend/internal/services/workload/job_submit.go`.
- `backend/internal/services/workload/spec.go`.
- `backend/internal/services/workload/handler.go`.
- `backend/internal/platform/config.go`.
- `backend/internal/platform/dev_token.go`.
- `backend/internal/platform/proxy_stream.go`.
- `backend/internal/e2e/live_configfile_dra_e2e_test.go`.
- Kubernetes DRA docs:
  `https://kubernetes.io/docs/tasks/configure-pod-container/assign-resources/allocate-devices-dra/`.
- Selkies firewall/TURN docs:
  `https://github.com/selkies-project/selkies/blob/main/docs/firewall.md`.
- coturn TURN server docs:
  `https://github.com/coturn/coturn/blob/master/README.turnserver`.

## 4. Assumptions

- The platform stays on DRA + NVIDIA MPS, not MIG or time-slicing.
- A stream job is explicitly marked with `streaming_session: true` in the job
  submit payload.
- The generic stream template defaults to 1080p60 H.264/NVENC and a
  12,000 Kbps max bitrate.
- A flat default cap of 64 concurrent active stream sessions keeps default
  planned egress near 768 Mbps, below the 70-80% target for one 1 Gbps link.
- Operators provide real TURN DNS, public IP/LB, TLS/Ingress, and registry
  publish credentials outside this patch.
- The local implementation can add build context, manifests, templates, docs,
  and in-process tests; it cannot prove NVENC, browser WebRTC, or live TURN
  relay without a GPU cluster and published image.

## 5. Non-Goals

- Do not change `dispatcher_dra.go` unless verification finds the Selkies
  manifest cannot hit the existing GPU marker path.
- Do not add MIG, vGPU, time-slicing, or a new GPU scheduler.
- Do not add multi-egress routing, per-tier QoS, or a bandwidth scheduler.
- Do not build a per-session dynamic WSS proxy registry in this slice.
  The v1 template exposes signaling through Kubernetes Service/Ingress; a
  gateway-native dynamic proxy is a later feature if needed.
- Do not build Isaac/Omniverse-specific image templating now.
- Do not commit TURN shared secrets, generated TURN credentials, or image
  registry credentials.

## 6. Current Behavior

Users can submit ConfigFile-backed Pod/Deployment/Job manifests as workload
jobs. If the manifest has an `nvidia.com/gpu` marker and the job sets
`gpu_count`, `sm_percentage`, and `pinned_memory_limit`, workload dispatch
generates the DRA MPS `ResourceClaimTemplate` and injects the DRA claim into the
Pod spec.

There is no packaged Selkies graphics desktop image, no streaming ConfigFile
template, no coturn deployment manifest, no short-lived TURN credential
endpoint, and no stream-session egress cap in scheduler admission.

## 7. Target Behavior

Users can submit a documented Selkies ConfigFile job that:

- declares `nvidia.com/gpu: 1` so existing DRA/MPS injection runs unchanged;
- sets `gpu_count`, `sm_percentage`, `pinned_memory_limit`, and
  `device_class_name` through existing job fields;
- runs a generic Selkies GL desktop image with 1080p60 H.264/NVENC defaults;
- exposes HTTP/WSS signaling on port 8080 and metrics/health ports where the
  image supports them;
- uses short-lived TURN REST credentials instead of static pod passwords;
- is rejected by scheduler admission when the global active stream-session cap
  is exhausted or the requested bitrate exceeds the configured cap.

## 8. Affected Domains

- Workload delivery: ConfigFile/job submit template for streaming sessions.
- Scheduler quota: public-internet egress admission guardrail.
- Workload service: job-bound TURN REST credential endpoint.
- Platform config: stream/TURN/runtime limits.
- Deployment: coturn manifests under `backend/deploy/`.
- Documentation and live E2E instructions.

No new microservice boundary is introduced. `scheduler-quota-service` owns
admission policy. `workload-service` owns stream credentials because credentials
must be authorized against the caller's admitted workload job. Workload dispatch
continues to own Kubernetes workload creation.

## 9. Affected Files

- Add `docs/plan/2026-06-20-browser-gpu-streaming-dra-mps.md`.
- Add `backend/streaming/selkies-gl-desktop/Dockerfile`.
- Add `backend/streaming/selkies-gl-desktop/README.md`.
- Add `backend/workload-service/templates/selkies-gl-desktop-configfile.yaml`.
- Add `backend/deploy/k3s/coturn.yaml`.
- Update `backend/deploy/k3s/kustomization.yaml`.
- Update `backend/deploy/k3s/runtime-config.yaml`.
- Update `backend/deploy/k3s/production-beta/runtime-secret-contract.yaml`.
- Update `backend/internal/platform/config.go`.
- Update `backend/internal/platform/config_test.go`.
- Update `backend/internal/services/workload/spec.go`.
- Update `backend/internal/services/workload/handler.go`.
- Add `backend/internal/services/workload/stream_credentials.go`.
- Add `backend/internal/services/workload/stream_credentials_test.go`.
- Update `backend/internal/services/workload/job_submit.go`.
- Update `backend/internal/services/workload/job_submit_test.go`.
- Update `backend/internal/services/schedulerquota/admission.go`.
- Update `backend/internal/services/schedulerquota/admission_decode.go`.
- Update `backend/internal/services/schedulerquota/admission_quota.go`.
- Add `backend/internal/services/schedulerquota/admission_streaming.go`.
- Update `backend/internal/services/schedulerquota/admission_test.go`.
- Add or extend `backend/internal/e2e/live_configfile_dra_e2e_test.go` with a
  streaming DRA/MPS dispatch case if it can reuse the current live harness
  without needing a real browser.
- Update `backend/docs/e2e-testing.md`.
- Add `backend/docs/browser-gpu-streaming.md`.

## 10. API / Contract Changes

Add one authenticated endpoint owned by `workload-service`:

```text
POST /api/v1/stream/credentials
```

Request:

```json
{
  "job_id": "J2600001",
  "session_id": "optional-client-session-id",
  "ttl_seconds": 28800
}
```

Response:

```json
{
  "turn": {
    "uris": ["turn:turn.example.com:3478?transport=udp"],
    "username": "1780000000:user-or-session",
    "password": "base64-hmac-sha1",
    "ttl_seconds": 28800,
    "expires_at": "2026-06-20T12:00:00Z"
  },
  "job_id": "J2600001"
}
```

Authorization and validation:

- the caller must pass existing route authentication;
- `job_id` is required;
- the job must exist in the workload job repository;
- the job must have `streaming_session: true`;
- the job status must be active: `submitted`, `waiting_infra`, `queued`, or
  `running`;
- the caller must have project access to the job's `project_id`;
- requested TTL is capped by `STREAM_TURN_CREDENTIAL_TTL`.

Add job submit metadata:

- `streaming_session` / `streamingSession`: boolean.
- `stream_max_bitrate_kbps` / `streamMaxBitrateKbps`: optional positive
  integer, defaulting to platform config.

No existing response fields are removed. No signaling JWT is emitted in this
slice because no validating gateway/Selkies component exists yet. Signaling auth
for v1 is the existing gateway/Ingress SSO path; a Sec-WebSocket-Protocol token
belongs with a later dynamic stream proxy that can validate it.

## 11. Database / Migration Changes

None. Stream-session admission counts active jobs from the existing
`workload-service:jobs` record payloads, which already persist arbitrary job
metadata in the platform record store JSON payload.

The source-of-truth fields are:

- `streaming_session`: admitted boolean copied from job submit payload.
- `stream_max_bitrate_kbps`: admitted integer copied/defaulted by scheduler
  admission and preserved on the job record.
- `status`: existing job lifecycle status.

Active stream sessions are jobs with `streaming_session: true` and status
`submitted`, `waiting_infra`, `queued`, or `running`. No migration is required
because these are additive JSON payload fields in the existing job resource.

## 12. Configuration Changes

Add platform config:

- `STREAM_TURN_URIS`: comma-separated TURN URLs returned to browsers.
- `STREAM_TURN_SHARED_SECRET`: coturn REST shared secret; required when
  stream credentials are enabled in production.
- `STREAM_TURN_CREDENTIAL_TTL`: default `8h`, max `12h`.
- `STREAM_MAX_BITRATE_KBPS`: default `12000`.
- `STREAM_MAX_CONCURRENT_SESSIONS`: default `64`.
- `STREAM_EGRESS_BUDGET_KBPS`: default `800000`.

Add coturn runtime secret contract keys for:

- `TURN_REALM`
- `TURN_STATIC_AUTH_SECRET`

`STREAM_TURN_SHARED_SECRET` and coturn `TURN_STATIC_AUTH_SECRET` must be
projected from the same Kubernetes Secret value. Rotation is a coordinated
operator action: update the shared Secret, restart coturn and backend workloads,
then allow old short-lived credentials to expire.

Production config validation must reject
`STREAM_MAX_CONCURRENT_SESSIONS * STREAM_MAX_BITRATE_KBPS >
STREAM_EGRESS_BUDGET_KBPS` so cap changes cannot silently oversubscribe the
egress budget.

## 13. Observability Changes

- Expose coturn Prometheus statistics in the manifest when supported by the
  coturn image/config.
- Include stream guardrail fields in scheduler admission review usage:
  active stream sessions, active stream bitrate Kbps, and egress budget Kbps.
- Document browser-side validation with `webrtc-internals` for bitrate, relay
  ratio, and packet loss.

No new metrics library or dashboard is planned in this slice.

## 14. Security Considerations

- TURN uses REST credentials, not static per-user passwords.
- The TURN shared secret stays in Kubernetes Secrets or operator-managed secret
  stores, never in ConfigMaps or templates.
- The credentials endpoint is authenticated through existing platform route
  auth, requires a workload `job_id`, verifies caller project access, and
  refuses inactive or non-stream jobs.
- Signaling JWT is deliberately not minted in v1 because this slice has no
  validating signaling proxy. Avoid issuing bearer-looking tokens that no
  component checks.
- MPS has no hard fault isolation. Documentation must state: use MPS for
  cooperative intra-project density; use MIG or whole-GPU allocation for
  untrusted cross-tenant isolation.
- Do not log TURN passwords, shared secrets, or credential response bodies.

## 15. Implementation Steps

1. Add platform config fields, env parsing, production validation, and config
   tests for stream TURN credentials and stream caps.
2. Add the workload stream credentials endpoint using stdlib HMAC. It requires
   `job_id`, verifies the caller can access the job project, verifies the job is
   an active admitted stream session, then returns coturn credentials where
   password = base64(HMAC-SHA1(sharedSecret, username)).
3. Register `POST /api/v1/stream/credentials` in `workload.Spec()` and
   `workload.Register()`.
4. Extend workload job submit to pass `streaming_session` and
   `stream_max_bitrate_kbps` into scheduler admission and preserve the admitted
   bitrate on the job record.
5. Add scheduler admission decoding and one small streaming guardrail:
   reject active stream sessions beyond `STREAM_MAX_CONCURRENT_SESSIONS`, reject
   requested bitrate above `STREAM_MAX_BITRATE_KBPS`, and reject
   `active_stream_bitrate_kbps + requested_bitrate_kbps` above
   `STREAM_EGRESS_BUDGET_KBPS`.
   `// ponytail: flat egress cap; per-tier QoS / multi-egress when one 1Gbps link is the proven bottleneck.`
6. Count active stream jobs from existing workload job records using the current
   active admission statuses.
7. Add focused unit tests for TURN credentials, missing/wrong-owner/inactive/
   non-stream job denial, job admission payload propagation, stream cap
   rejection, bitrate cap rejection, aggregate egress budget rejection, and
   TURN secret env contract docs.
8. Add the generic Selkies GL desktop build context with documented defaults:
   1080p60, H.264/NVENC, 12 Mbps cap, no app-specific layers.
9. Add the ConfigFile manifest template: Deployment, Service, optional Ingress
   notes, `nvidia.com/gpu: 1`, `streaming_session: true` payload guidance, and
   Selkies TURN REST/config environment wiring.
10. Add coturn k3s manifest with `use-auth-secret`, `static-auth-secret` loaded
    from Secret, UDP 3478, relay port range, stdout logs, and Prometheus flag.
11. Update docs with submit instructions, DRA/MPS reuse, browser/TURN
    verification, egress limits, and MPS isolation caveat.
12. Extend the live DRA E2E only as a dispatch-level stream-template check if it
    can run without a real GPU browser session; keep full browser/WebRTC
    validation documented as an operator-run check.

## 16. Verification Plan

Run:

- `go -C backend test ./internal/platform -run 'Config' -count=1`
- `go -C backend test ./internal/services/workload -run 'Stream|Credential|TURN|Submit|Admission' -count=1`
- `go -C backend test ./internal/services/schedulerquota -run 'Admission|Stream' -count=1`
- `go -C backend test ./internal/platform ./internal/services/workload ./internal/services/schedulerquota -count=1`
- `kubectl kustomize backend/deploy/k3s`
- `kubectl kustomize backend`
- `git diff --check`
- `go -C backend test ./... -count=1`
- `go -C backend vet ./...`
- `go -C backend build ./...`
- `bash backend/scripts/ci-security-gate.sh quick`

If available, run:

- `TEST_LIVE_K8S_CONFIGFILE_DRA=1 go -C backend test -tags e2e ./internal/e2e -run '^TestLiveK8sConfigFileDRADispatchE2E$' -count=1 -v`
- Build the Selkies image locally with the documented command.
- Submit the stream ConfigFile job with `gpu_count=1`, `sm_percentage=50`, and
  `pinned_memory_limit=8Gi`; confirm the generated ResourceClaimTemplate still
  has MPS `defaultActiveThreadPercentage=50`.
- Browser check through ingress/SSO; confirm direct ICE and forced TURN relay.
- Confirm outbound bitrate is less than or equal to `STREAM_MAX_BITRATE_KBPS`.

Sonar/security gates:

- `bash backend/scripts/ci-security-gate.sh sonar`
- `bash backend/scripts/ci-security-gate.sh security`

If Sonar or security cannot run locally, final review must state the blocker and
residual risk.

## 17. Rollback Plan

Revert the new stream image context, ConfigFile template, coturn manifest, docs,
config additions, credentials endpoint, and scheduler/workload stream fields.

No database rollback is required. Existing DRA/MPS dispatch remains unchanged.
If coturn has been deployed, remove the kustomize resource or scale the
Deployment to zero and delete only the coturn Service/ConfigMap/Secret created
for this slice.

## 18. Risks and Tradeoffs

- The flat cap is intentionally crude. It is enough for a single known egress
  link and avoids building a bandwidth scheduler before measurements exist.
- Short-lived TURN credentials in pod/browser runtime must be refreshed for
  long sessions; Selkies REST/config JSON support is preferred over fixed pod
  env credentials.
- The generic GL desktop proves the platform path; Isaac/Omniverse images still
  need app-specific tags and licensing/runtime validation.
- MPS improves density but does not provide hard fault isolation. A GPU XID can
  affect co-located MPS clients.
- Live WebRTC/NVENC validation depends on GPU hardware, drivers, public DNS/LB,
  TURN reachability, and a browser; local unit tests cannot prove that path.

## 19. Reviewer Checklist

- Requirement fit: does the plan package Selkies onto the existing DRA/MPS job
  path and add TURN plus public-internet guardrails?
- Scope: is DRA/MPS reused instead of rebuilt?
- Simplicity: is the guardrail a flat session cap plus bitrate cap only?
- API contract: is the credentials endpoint documented and authenticated?
- Data ownership: does scheduler-quota own admission policy and workload-service
  own job-bound stream credentials without a new service?
- Config: are TURN secrets and caps externalized?
- Security: are static TURN passwords avoided and secrets kept out of manifests?
- Observability: are stream usage and coturn metrics considered?
- Tests: are new branch, guardrail, credential, and ownership paths covered by
  focused tests?
- Rollback: can the feature be removed without schema rollback?
- MPS caveat: is the no-hard-isolation constraint documented?

## 20. Status

Status: Approved

Reviewer Agent approved this plan for Code Agent implementation. Final review
must include requirement-fit, approved-plan alignment, SOLID, 12-Factor,
tests/build results, Sonar Quality Gate status, risks, and diff-scope evidence.
