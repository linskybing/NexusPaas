# Core Feature Area J: Monitoring, Usage, and Reporting

Part of the [GA Acceptance docs](README.md).

## Goal

The platform must provide real-time and historical usage views.

Real-time usage is for operations and user visibility.

Historical usage is for reporting, quota review, cost allocation, and future
billing.

## Required Usage Dimensions

```text
platform
group
project
user
job
pod
container
process
gpu
queue
plan
image
build
stream session
```

## Required Usage APIs

```text
GET /api/v1/users/{user_id}/usage?from=...&to=...&step=...
GET /api/v1/projects/{project_id}/usage?from=...&to=...&step=...
GET /api/v1/groups/{group_id}/usage?from=...&to=...&step=...
GET /api/v1/jobs/{job_id}/usage?from=...&to=...&step=...
```

## Example Response

```json
{
  "project_id": "proj-123",
  "from": "2026-06-01T00:00:00Z",
  "to": "2026-06-20T00:00:00Z",
  "step": "1h",
  "totals": {
    "cpu_core_hours": 120.5,
    "memory_gib_hours": 804.0,
    "gpu_reserved_sm_hours": 56.2,
    "gpu_observed_memory_gib_hours": 211.0,
    "gpu_estimated_sm_hours": 49.8,
    "stream_session_hours": 12.0,
    "build_cpu_core_hours": 8.5
  },
  "series": []
}
```

## Acceptance Criteria

| ID | Acceptance Criteria |
|---|---|
| MON-001 | Dashboard shows all active Jobs by User, Project, and Group. |
| MON-002 | Dashboard shows GPU SM reserved ratio in real time. |
| MON-003 | Dashboard shows GPU memory usage by user/project/job when process-level data is available. |
| MON-004 | Dashboard shows metric source: reserved, observed, estimated, or unavailable. |
| MON-005 | Dashboard shows MPS clients per GPU. |
| MON-006 | Dashboard shows orphan GPU PIDs. |
| MON-007 | Group usage report supports arbitrary time range. |
| MON-008 | Project usage report supports arbitrary time range. |
| MON-009 | User usage report supports arbitrary time range. |
| MON-010 | Usage API enforces RBAC. |
| MON-011 | Usage rollups can be rebuilt. |
| MON-012 | Usage data survives workload deletion. |
| MON-013 | Metrics include queue pending/running/preempted/rejected counts. |
| MON-014 | Metrics include build running/failed/succeeded/timeout counts. |
| MON-015 | Metrics include WebRTC active session count and egress bitrate. |
| MON-016 | Metrics include ConfigFile admission rejection reason counts. |
| MON-017 | Metrics include Kubernetes apply failure counts. |
| MON-018 | Alerts exist for GPU telemetry missing, usage attribution failure, quota drift, orphan GPU PIDs, queue backlog, build failure spike, and preemption spike. |
| MON-019 | Prometheus outage does not corrupt quota accounting. |
| MON-020 | Usage-observability restart does not lose finalized usage summaries. |
