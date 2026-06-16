# usage-observability-service

Category: Ops/Read Model | Phase: 3

## 1. Overview

The usage and dashboard read-model service. Responsible for GPU usage, resource hours, cluster summary, admin/user usage dashboards, Prometheus queries, and snapshots/retention. Read-heavy with event-driven writes (eventually consistent) — well suited for early-to-mid extraction.

## 2. Functional Requirements

| ID | Requirement | Acceptance/Notes |
| --- | --- | --- |
| FR-OBS-01 | Provide regular users with GPU/resource usage, my GPU jobs, and my request usage. | /me/usage, /me/gpu/jobs, /me/request-usage. |
| FR-OBS-02 | Provide admin usage, request usage, MPS mapping, GPU user summaries, history, and per-user job lists. | /api/v1/admin/usage, /admin/gpu/users. |
| FR-OBS-03 | Provide dashboard overview and admin dashboard summary. | /dashboard/overview, /admin/dashboard-summary. |
| FR-OBS-06 | Support GPU usage snapshots, resource hours summaries, and cleanup retention. | Config provides scan/summary/cleanup intervals and retention days. |
| FR-K8S-02 (shared) | Aggregated presentation of cluster summary, MPS mapping, node list/detail, and pod GPU usage. | Raw data collected by k8s-control; this service builds the read model. |
| FR-WORKLOAD-11 (shared) | Read model for Job GPU usage, summary, timeline, and breakdowns. | /jobs/{id}/gpu-* — workload aggregates this service's data. |

## 3. Owned Data

`job_gpu_usage_snapshots`, `pod_resource_records`, resource_hours summaries, cluster read models.

## 4. Current Code/Route Mapping

- Handlers: `usage.go`, `resource_hours.go`, `dashboard.go`, `cluster.go`
- Module: `gpuusage`
- Routes: `/api/v1/me/usage`, `/api/v1/me/gpu/jobs`, `/api/v1/me/request-usage`, `/api/v1/admin/usage`, `/api/v1/admin/request-usage`, `/api/v1/admin/gpu/users`, `/api/v1/dashboard/*`

## 5. Data Sources

- Job/IDE lifecycle events (workload, scheduler-quota, ide)
- k8s-control reconcile snapshots
- Prometheus queries (GPU metrics, MPS mapping)

## 6. Events

| Direction | Event | Purpose |
| --- | --- | --- |
| Subscribe | full Job lifecycle events, QuotaReserved/Released, QueueDepthChanged | Usage accounting and dashboards |
| Subscribe | GroupMembershipChanged / ProjectCreated/Deleted | Read-model rebuild and isolation |

## 7. Non-Functional Highlights

- Read-heavy endpoints (dashboard, usage summary, cluster summary) need cache/read models (NFR-PERF-03).
- Usage collector background workers need horizontal sharding or mutual-exclusion locks (NFR-SCALE-02).
- Snapshot scan/summary/cleanup intervals and retention days are configuration-driven (NFR-OPER-02).
- A non-core reporting service that may degrade independently without affecting the job-scheduling flow (NFR-AVAIL-01).

## 8. Decomposition Notes

Extracted in phase 3. First rebuild the read models from events + Prometheus and compare with the monolith via dual reads; after parity is verified, dashboards switch to this service.
