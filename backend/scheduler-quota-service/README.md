# scheduler-quota-service

Category: Compute | Phase: 5 (recommended to split last)

## 1. Overview

The resource scheduling and quota arbitration center — the core of job consistency. Responsible for Plans, Queues, quota reservation, priority, preemption, dispatch policies, SKIP LOCKED/queue workers, and GPU/DRA/MPS resource claims. Provides transactional `Reserve/Commit/Release` APIs: every resource consumption must reserve before dispatch, eliminating TOCTOU.

## 2. Functional Requirements

| ID | Requirement | Acceptance/Notes |
| --- | --- | --- |
| FR-WORKLOAD-06 | Support scheduler mode and local mode; scheduler mode controls workloads via Volcano/VCJob or a scheduler executor. | SchedulerConfig.EXECUTOR_MODE. |
| FR-WORKLOAD-07 | Control job queuing and dispatch using queue, priority, preemptible, TTL, and max-concurrent policies. | plans/queues and scheduler config. |
| FR-WORKLOAD-08 | Support Resource Plan and Queue CRUD, Project-to-Plan binding, and Plan-to-Queue binding. | /api/v1/plans, /api/v1/queues. |
| FR-WORKLOAD-09 | Support GPU quota and CPU/Memory/GPU resource checks with transactional reservation or pre-scheduling checks that prevent TOCTOU. | Currently uses DB constraints, SKIP LOCKED, and the quota service. |
| FR-WORKLOAD-10 | Support preemption strategies, including SQL-based and K8s-live strategies; in Volcano mode, defer to the scheduler's native preemption. | preemption plugin. |

## 3. Owned Data

`plans`, `queues`, `resource_quotas`, `priority_classes`, `reservations`, preemption records, gpu_claim snapshots.

## 4. Current Code/Route Mapping

- Handlers: `plan.go`, `queue.go`
- Plugins: priority, preemptor, resource quota/migrations
- Routes: `/api/v1/plans`, `/api/v1/queues`, quota/preemption internal APIs

## 5. External Interfaces

- `Reserve(project, resources) → reservation_id`: transactional resource reservation
- `Commit(reservation_id)`: confirm consumption after successful dispatch
- `Release(reservation_id)`: release on failure/cancellation/completion
- Plan/Queue management REST APIs

## 6. Events

| Direction | Event | Counterpart | Purpose |
| --- | --- | --- | --- |
| Publish | QuotaReserved / QuotaReleased / QueueDepthChanged | workload, usage | Dashboards and dispatch |
| Publish | JobPreempted | workload, usage, audit, notification | Preemption notification and resource release |
| Subscribe | ProjectCreated / ProjectDeleted | org-project | Create/reclaim project quotas |
| Subscribe | Job terminal-state events | workload | Release reserved resources |

## 7. Non-Functional Highlights

- Quota reservation, queue dispatch, and preemption require transactional reservation or single-arbiter control (NFR-DATA-03) — this service IS that arbiter.
- Dispatch workers need leader election or sharding to avoid duplicate dispatch (NFR-AVAIL-03).
- Queue depth and dispatch latency are core monitoring metrics (NFR-OBS-02).

## 8. Decomposition Notes

Recommended to split last, since it is the core of job consistency. Before extraction, first introduce the Reserve/Commit/Release interface inside the monolith to ensure a single call path.
