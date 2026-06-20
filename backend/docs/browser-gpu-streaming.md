# Browser GPU Streaming

NexusPaas streams GPU graphics sessions by running a Selkies desktop as a normal
workload job. GPU sharing is not a new path: the submitted manifest keeps an
`nvidia.com/gpu: "1"` marker and workload dispatch reuses the existing DRA
`ResourceClaimTemplate` + NVIDIA MPS injection.

## Job Shape

1. Build and publish `backend/streaming/selkies-gl-desktop`.
2. Create a ConfigFile from
   `backend/workload-service/templates/selkies-gl-desktop-configfile.yaml`.
3. Submit the job with the normal workload API fields:

```json
{
  "config_id": "selkies-gl-desktop",
  "queue_name": "default-batch",
  "gpu_count": 1,
  "sm_percentage": 50,
  "pinned_memory_limit": "8Gi",
  "device_class_name": "gpu.nvidia.com",
  "streaming_session": true,
  "stream_max_bitrate_kbps": 12000
}
```

`stream_max_bitrate_kbps` defaults to `STREAM_MAX_BITRATE_KBPS` when omitted.
For the default config, one stream is admitted at 12 Mbps and the flat
concurrency cap is 64 sessions, keeping planned egress at 768 Mbps.

## TURN

Deploy `backend/deploy/k3s/coturn.yaml` on a node with public UDP reachability.
The manifest uses coturn REST shared-secret auth:

- coturn reads `TURN_STATIC_AUTH_SECRET` from `coturn-runtime-secret`;
- workload-service reads `STREAM_TURN_SHARED_SECRET` from its runtime secret;
- both values must be the same secret;
- `STREAM_TURN_URIS` controls the TURN URIs returned to browsers.

Create the coturn secret:

```sh
kubectl -n nexuspaas create secret generic coturn-runtime-secret \
  --from-literal=TURN_STATIC_AUTH_SECRET='<shared secret>' \
  --from-literal=TURN_REALM='turn.example.com'
```

Set `STREAM_TURN_URIS`, for example:

```text
turn:turn.example.com:3478?transport=udp
```

Then put the same shared secret in `workload-service-runtime-secret` as
`STREAM_TURN_SHARED_SECRET`.

Clients request short-lived credentials from:

```text
POST /api/v1/stream/credentials
```

The caller must be authenticated, must have access to the job project, and the
job must be an active `streaming_session`.

## Egress Guardrail

The scheduler admission guardrail is intentionally flat:

- reject a stream whose requested bitrate exceeds `STREAM_MAX_BITRATE_KBPS`;
- reject when active stream sessions reach `STREAM_MAX_CONCURRENT_SESSIONS`;
- reject when active stream bitrate plus the requested bitrate exceeds
  `STREAM_EGRESS_BUDGET_KBPS`.

The default is 64 sessions x 12 Mbps = 768 Mbps under an 800 Mbps budget. That
is the v1 public-internet protection for one 1 Gbps egress link. Multi-egress,
per-tier QoS, and codec fleet optimization are separate projects.

## Verification

Dispatch-level DRA/MPS check:

```sh
TEST_LIVE_K8S_CONFIGFILE_DRA=1 \
  go test -tags e2e ./internal/e2e -run '^TestLiveK8sConfigFileDRADispatchE2E$' -count=1 -v
```

Runtime checks on a GPU cluster:

- submit the Selkies job with `gpu_count=1`, `sm_percentage=50`, and
  `pinned_memory_limit=8Gi`;
- confirm the applied ResourceClaimTemplate has MPS
  `defaultActiveThreadPercentage=50`;
- open the session through gateway/Ingress auth;
- confirm WebRTC direct ICE, then force TURN and confirm relay fallback;
- check browser `webrtc-internals` for bitrate, relay ratio, and packet loss;
- verify outbound bitrate stays at or below `STREAM_MAX_BITRATE_KBPS`;
- confirm gpuusage telemetry reports MPS units for the stream pod.

## Isolation Caveat

NVIDIA MPS does not provide hard fault isolation between clients on one GPU. A
GPU fault can affect co-located MPS sessions. NexusPaas limits the memory
dimension with `pinned_memory_limit`, but MPS is still best for cooperative
intra-project density. Use MIG or whole-GPU allocation for untrusted tenants or
hard cross-tenant isolation.
