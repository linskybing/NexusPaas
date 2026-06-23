# Core Feature Area F: WebRTC GUI Workloads

Part of the [GA Acceptance docs](README.md).

## Goal

Users can enable WebRTC for GUI workloads so applications such as desktop GUI,
simulation, CAD, Omniverse-like tools, or visualization containers can be
operated through a browser.

Existing repository direction with Selkies, DRA/MPS, coturn, and streaming
admission should be preserved and productized.

## WebRTC Deployment Flow

```text
User submits ConfigFile (their own app image) with streaming_session=true
  -> Project capability check: allow_webrtc                      [target]
  -> Plan/Queue streaming limit check
  -> GPU/DRA/MPS admission if GPU requested
  -> Dispatch auto-injects the Selkies sidecar + shared display/GPU claim
  -> Workload deployed
  -> User opens NexusPaaS stream URL
  -> workload-service issues short-lived TURN credentials
  -> Gateway authorizes browser session
  -> Usage agent attributes CPU/RAM/GPU/process/stream usage
```

## Submit Fields

Implemented (enforced today):

```json
{ "streaming_session": true, "stream_max_bitrate_kbps": 12000 }
```

Target (not yet implemented — tracked as gaps, do not assume enforced):
`stream_resolution`, `stream_fps`, `stream_idle_timeout_seconds`, and the
`allow_webrtc` Project capability. The sidecar currently defaults to 1080p60.

## Streaming Security Rules

| Rule | Required Behavior |
|---|---|
| Auth | Only authorized Project members can open the stream. |
| TURN credentials | Short-lived credentials only. No shared TURN secret exposed to users. |
| Gateway | Browser access must go through NexusPaaS Gateway or approved ingress path. |
| NodePort | User workloads cannot expose arbitrary NodePort. |
| Egress | TURN egress must be controlled by egress profile. |
| Bitrate | Requested bitrate must pass admission. |
| Session cap | Active stream sessions must be limited by Plan/Queue. |
| Idle timeout | Idle sessions are terminated or suspended. |
| Audit | Stream open, credential issue, and stream close are audited. |

## Acceptance Criteria

| ID | Acceptance Criteria |
|---|---|
| RTC-001 | Project without `allow_webrtc` cannot start streaming workload. |
| RTC-002 | Project with `allow_webrtc` can start approved streaming ConfigFile. |
| RTC-003 | Browser user can open GUI session without Kubernetes credentials. |
| RTC-004 | Non-member cannot open stream URL. |
| RTC-005 | Non-owner cannot request stream credentials unless Project role allows it. |
| RTC-006 | TURN credentials are short-lived and expire correctly. |
| RTC-007 | TURN shared secret is never returned to client. |
| RTC-008 | Direct ICE and forced TURN relay are tested in staging. |
| RTC-009 | Stream bitrate above cap is rejected. |
| RTC-010 | Stream session above Project/Queue cap is rejected. |
| RTC-011 | Stream egress budget is enforced. |
| RTC-012 | Idle timeout terminates or suspends session. |
| RTC-013 | Stream usage is attributed to user/project/group. |
| RTC-014 | Stream GPU usage is attributed through container/PID mapping. |
| RTC-015 | Streaming workload cannot bypass image allow list. |
| RTC-016 | Streaming workload cannot open external egress except through approved profile. |
| RTC-017 | Browser GPU streaming E2E passes on a real GPU node. |
| RTC-018 | WebRTC metrics include session count, bitrate, packet loss if available, TURN relay ratio, and disconnect count. |

## Current Evidence

As of 2026-06-22, RTC-006/RTC-007 have focused credential-safety evidence in
`backend/internal/services/workload/stream_credentials_test.go`: requested TURN
credential TTLs are capped/defaulted, `expires_at` is RFC3339 and matches the
username expiry prefix, generated passwords are HMAC-derived and not the shared
secret, and serialized responses do not contain the configured shared secret.

RTC-008 now has current live RKE2/staging route-proof evidence from the
first-party GUI E2E harness. Playwright ran with
`NEXUSPAAS_E2E_ENVIRONMENT=staging`, submitted seeded streaming Job
`e2e-job-mqongkuq-oov6qe`, requested stream credentials through the GUI, then
used browser-native `RTCPeerConnection` candidate gathering for both direct ICE
and forced TURN relay. The single route-proof JSON recorded
`rtc_probe_environment="staging"`, `rtc_direct_ok=true`,
`rtc_direct_candidate_count=2`, `rtc_direct_candidate_types=["host"]`,
`rtc_relay_ok=true`, `rtc_relay_candidate_count=1`, and
`rtc_relay_candidate_types=["relay"]`. TURN URI config was temporarily changed
to browser-reachable `turn:127.0.0.1:3478?transport=udp` for the proof, then
restored to `turn:coturn.nexuspaas.svc.cluster.local:3478?transport=udp`;
secret values were not printed.

A separate WEB/usage proof now demonstrates nonzero requested-GPU pod
visibility through the first-party GUI and Project GPU route on
`ci-ga-gpu-readmodel-20260622034034`: seeded Project
`e2e-p-mqooctn3-fammye` reported `gpu_status=200`, `gpu_used=1`, and
`gpu_nonzero=true` for a Kubernetes fixture pod requesting
`nvidia.com/gpu: "1"`.

Selkies now attaches as an **auto-injected sidecar**, not a baked image: a
`streaming_session` job keeps the user's own app image, and dispatch injects the
`selkies` container, shared `/tmp/.X11-unix` + `/dev/shm` volumes, `DISPLAY=:0`,
and a shared MPS claim. This injection is unit-evidenced in
`backend/internal/services/workload/dispatcher_streaming_test.go` (sidecar
present, app image unchanged, idempotent, sidecar image required).

This does not prove browser media E2E, real GPU-node streaming, egress budget
enforcement, per-device GPU utilization telemetry, or stream metrics.
