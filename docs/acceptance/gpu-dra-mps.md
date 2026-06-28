# Core Feature Area D: GPU DRA + NVIDIA MPS

Part of the [GA Acceptance docs](README.md).

## Goal

GPU allocation uses Kubernetes Dynamic Resource Allocation as the primary
mechanism.

NVIDIA MPS is used for fractional GPU execution when the Project, Queue, Plan,
GPU type, and security policy allow it.

## Required Submit Fields

```json
{
  "gpu_count": 1,
  "sm_percentage": 50,
  "pinned_memory_limit": "8Gi",
  "device_class_name": "gpu.nvidia.com"
}
```

## DRA / MPS Behavior

When a workload requests GPU and has a GPU marker in its manifest, compute-api
and compute-control-plane must produce or reference a DRA ResourceClaimTemplate.

The effective reserved GPU is:

```text
effective_gpu = gpu_count * sm_percentage / 100
```

Example:

```text
gpu_count = 1
sm_percentage = 50
effective_gpu = 0.5
```

## MPS Isolation Policy

MPS is allowed for density and cooperative sharing.

MPS must not be represented as hard isolation across mutually untrusted tenants.

| Scenario | GPU Sharing Policy |
|---|---|
| Same user | MPS allowed |
| Same Project | MPS allowed by default if Queue permits |
| Same Group but different Projects | MPS allowed only if Group policy permits |
| Different Groups | Prefer MIG or whole GPU; MPS requires explicit platform admin approval |
| High-security tenant | MPS forbidden; use MIG or whole GPU |
| GUI streaming workload | MPS allowed if WebRTC and GPU policy permit |
| Production inference | Prefer MIG or whole GPU unless density is explicitly accepted |

## Acceptance Criteria

| ID | Acceptance Criteria |
|---|---|
| GPU-001 | A workload with `gpu_count=1` and `sm_percentage=50` creates or references a DRA ResourceClaimTemplate. |
| GPU-002 | The generated ResourceClaimTemplate contains the expected device class. |
| GPU-003 | The generated ResourceClaimTemplate contains MPS active thread percentage when requested. |
| GPU-004 | The generated ResourceClaimTemplate contains pinned memory limit when requested. |
| GPU-005 | The Pod references the generated ResourceClaimTemplate or ResourceClaim. |
| GPU-006 | Legacy `nvidia.com/gpu` marker is removed or converted so scheduling uses DRA path. |
| GPU-007 | Admission accounts fractional GPU as `gpu_count * sm_percentage / 100`. |
| GPU-008 | Invalid `sm_percentage < 1` or `sm_percentage > 100` is rejected. |
| GPU-009 | Invalid pinned memory quantity is rejected. |
| GPU-010 | Queue can restrict allowed GPU types and DeviceClasses. |
| GPU-011 | Plan can restrict total GPU count and total SM percentage. |
| GPU-012 | MPS sharing across untrusted Projects is blocked unless platform admin explicitly allows it. |
| GPU-013 | MPS caveat is visible in admin docs and user docs. |
| GPU-014 | GPU reservation is released when workload terminates. |
| GPU-015 | GPU reservation drift is detected and alerted. |
| GPU-016 | Live E2E test proves DRA ResourceClaimTemplate + MPS injection on a GPU cluster. |
| GPU-017 | GPU usage dashboard shows reserved GPU fraction and observed GPU usage separately. |
| GPU-018 | If true per-process SM usage is unavailable on the target NVIDIA stack, UI must label SM attribution as estimated or allocation-based, not measured. |

## Current Admission Caveat

`scheduler-quota-service` enforces MPS policy during submit admission. The
cross-project guard is a conservative control-plane check over active workload
records using the same active statuses as quota accounting, plus a declarative
plan-level `mps_share_project_id` / `allow_cross_project_mps` gate. It is not yet
node-level placement proof; live DRA/MPS hardware validation remains `GPU-016`.

## Current Local Control-Plane Evidence

`GPU-014` and `GPU-015` now have local control-plane coverage in the
`storage-data-path` branch slice documented by
[`2026-06-27-gpu-reservation-release-drift.md`](../plan/2026-06-27-gpu-reservation-release-drift.md):

- workload submit persists the committed scheduler reservation on the job and
  releases it if job persistence fails after commit;
- workload terminal paths release reservations for dispatch failure, lifecycle
  completion/failure, preemption, eviction, stale-resource failure, and idle
  reaping;
- scheduler-quota runs a reservation drift detector and emits
  `ReservationDriftDetected` when an active reservation has a missing or terminal
  workload record.

`GPU-017` and `GPU-018` now also have local read-model coverage in the
`storage-data-path` branch slice documented by
[`2026-06-27-gpu-usage-reserved-observed.md`](../plan/2026-06-27-gpu-usage-reserved-observed.md)
and synced by
[`2026-06-27-gpu-usage-doc-sync.md`](../plan/2026-06-27-gpu-usage-doc-sync.md):

- `/api/v1/projects/{id}/gpu-usage` keeps `used` for compatibility and adds
  `observed_gpu_pods`, `observed_gpu_source`, `reserved_gpu_fraction`,
  `reserved_gpu_source`, and `sm_attribution_source`;
- reserved GPU fraction is derived from Project GPU read-model rows, fresh
  usage-observability snapshots, then co-hosted workload job reservation data;
- MPS source labeling preserves measured source metadata and marks
  allocation-derived or unavailable true per-process SM as
  `estimated_mps_allocation` or `unavailable`, not measured.

This does not close `GPU-016`; live DRA/MPS behavior still requires evidence
from a real GPU cluster.

## Frontend-deferred note

The web frontend has been removed (backend-only phase). GPU-017 and GPU-018 are
UI-facing: their **UI rendering** is deferred until a frontend is rebuilt. The
backend satisfies the data side now — the GPU usage API exposes the reserved GPU
fraction and observed usage as **separate** fields, and labels SM attribution so
SM is never reported as measured when true per-process SM is unavailable. The
`RTC` (WebRTC GUI) acceptance area is likewise deferred with the frontend;
backend Selkies sidecar streaming dispatch remains in the codebase.

GPU-001…016 are verifiable on a GPU-less machine via the `dra-example-driver`
harness — see `backend/docs/e2e-testing.md`.
