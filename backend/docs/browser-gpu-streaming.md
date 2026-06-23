# Browser GPU Streaming

NexusPaas streams a GPU graphics session by **auto-injecting a Selkies sidecar**
into the user's own workload pod. The user submits their normal app image with
`streaming_session: true`; the platform attaches the sidecar (X display server +
NVENC capture + WebRTC signaling) at dispatch. The app image is never rebuilt to
embed Selkies.

GPU sharing is unchanged: the manifest keeps an `nvidia.com/gpu: "1"` marker and
dispatch reuses the existing DRA `ResourceClaimTemplate` + NVIDIA MPS injection.
Because DRA wiring runs after sidecar injection, both the app container and the
sidecar reference the **same** shared MPS claim.

## Submit a Streaming Job

1. Operator sets `STREAM_SIDECAR_IMAGE` to the published sidecar image
   (`backend/streaming/selkies-gl-desktop`).
2. Submit a ConfigFile whose pod runs **your** app image with an
   `nvidia.com/gpu` marker (see
   `backend/workload-service/templates/selkies-gl-desktop-configfile.yaml`).
3. Submit the job with the normal workload fields:

```json
{
  "config_id": "my-gpu-app",
  "queue_name": "default-batch",
  "gpu_count": 1,
  "sm_percentage": 50,
  "pinned_memory_limit": "8Gi",
  "device_class_name": "gpu.nvidia.com",
  "streaming_session": true,
  "stream_max_bitrate_kbps": 12000
}
```

At dispatch the platform injects, into each pod-bearing resource:
- a `selkies` sidecar container from `STREAM_SIDECAR_IMAGE`, signaling on `:8080`
  and metrics on `:9090`;
- shared `emptyDir` volumes mounted on both containers: `/tmp/.X11-unix` (X
  socket) and `/dev/shm`;
- `DISPLAY=:0` on the app container(s) so they render into the sidecar's display.

Injection is skipped if the pod already defines a `selkies` container.
`stream_max_bitrate_kbps` defaults to `STREAM_MAX_BITRATE_KBPS` when omitted.
Submitting `streaming_session: true` while `STREAM_SIDECAR_IMAGE` is unset is
rejected at submit.

## TURN

Deploy `backend/deploy/k3s/coturn.yaml` on a node with public UDP reachability.
coturn uses REST shared-secret auth:

- coturn reads `TURN_STATIC_AUTH_SECRET` from `coturn-runtime-secret`;
- workload-service reads `STREAM_TURN_SHARED_SECRET` from its runtime secret;
- both must be the same value;
- `STREAM_TURN_URIS` controls the TURN URIs returned to browsers.

```sh
kubectl -n nexuspaas create secret generic coturn-runtime-secret \
  --from-literal=TURN_STATIC_AUTH_SECRET='<shared secret>' \
  --from-literal=TURN_REALM='turn.example.com'
```

Clients request short-lived credentials from `POST /api/v1/stream/credentials`.
The caller must be authenticated, have access to the job project, and the job
must be an active `streaming_session`.

## Egress Guardrail

Scheduler admission applies a flat cap:

- reject a stream whose requested bitrate exceeds `STREAM_MAX_BITRATE_KBPS`;
- reject when active sessions reach `STREAM_MAX_CONCURRENT_SESSIONS`;
- reject when active + requested bitrate exceeds `STREAM_EGRESS_BUDGET_KBPS`.

The default 64 sessions x 12 Mbps = 768 Mbps protects one 1 Gbps egress link.
Multi-egress, per-tier QoS, and codec optimization are separate projects.

## Verification

Dispatch-level DRA/MPS + sidecar check:

```sh
TEST_LIVE_K8S_CONFIGFILE_DRA=1 \
  go test -tags e2e ./internal/e2e -run '^TestLiveK8sConfigFileDRADispatchE2E$' -count=1 -v
```

On a GPU cluster:
- submit a streaming job; confirm the dispatched pod has the injected `selkies`
  sidecar, shared `/tmp/.X11-unix` + `/dev/shm`, `DISPLAY=:0` on the app, and a
  single shared MPS `ResourceClaimTemplate` (`defaultActiveThreadPercentage=50`);
- open the session through gateway/Ingress auth; confirm direct ICE, then force
  TURN and confirm relay fallback;
- check `webrtc-internals` for bitrate, relay ratio, packet loss; confirm
  outbound bitrate ≤ `STREAM_MAX_BITRATE_KBPS`;
- confirm gpuusage telemetry reports MPS units for the pod.

## Isolation Caveat

NVIDIA MPS does not provide hard fault isolation between clients on one GPU. A
GPU fault can affect co-located MPS sessions (including the app + its own
sidecar). `pinned_memory_limit` bounds only the memory dimension. Use MIG or
whole-GPU allocation for untrusted tenants or hard cross-tenant isolation.
