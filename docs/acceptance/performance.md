# Performance and Scale Targets

Part of the [GA Acceptance docs](README.md).

## Initial GA Targets

These are initial production targets and should be adjusted after real load
testing.

| Area | Target |
|---|---|
| Login p95 | <= 500 ms excluding external IdP delay |
| Project list p95 | <= 300 ms |
| ConfigFile submit p95 | <= 2 seconds before queue admission result |
| Manifest preflight p95 | <= 2 seconds for normal manifests |
| Queue admission p95 | <= 1 second without preemption |
| Queue admission with preemption p95 | <= 5 seconds |
| Job status p95 | <= 500 ms |
| Usage query p95 | <= 2 seconds for 30-day hourly Project report |
| Image build API p95 | <= 1 second before async build starts |
| Stream credential p95 | <= 300 ms |
| Usage attribution delay | <= 30 seconds for running containers |
| GPU process attribution delay | <= 30 seconds on GPU nodes |
| Quota release delay | <= 60 seconds after terminal state |

## Acceptance Criteria

| ID | Acceptance Criteria |
|---|---|
| PERF-001 | Load test proves target p95 latency for core APIs. |
| PERF-002 | Load test covers at least 100 concurrent users for initial GA unless revised. |
| PERF-003 | Queue stress test covers pending, admitted, preempted, and rejected workloads. |
| PERF-004 | Usage query test covers large Group with many Projects and Users. |
| PERF-005 | Metrics cardinality test verifies process/PID labels do not break Prometheus retention. |
| PERF-006 | Build concurrency test verifies quota and timeout behavior. |
| PERF-007 | WebRTC concurrency test verifies stream admission and egress budget. |
| PERF-008 | K8s-control apply throughput test verifies no uncontrolled API server pressure. |
