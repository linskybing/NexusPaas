# workload-service

Category: Compute | Phase: 5

## 1. Overview

The core workload service. Responsible for ConfigFile content-addressable storage with immutable versioning, Job submit/list/detail/cancel, the Job state machine, templates, job logs metadata, and executor command orchestration. Job submission runs a saga coordinating scheduler-quota (resource reservation), image-registry (image policy), storage (mounts), and k8s-control (workload creation).

## 2. Functional Requirements

| ID | Requirement | Acceptance/Notes |
| --- | --- | --- |
| FR-WORKLOAD-01 | Support ConfigFile CRUD, listing by project, project file tree, and project version history. | /api/v1/configfiles. |
| FR-WORKLOAD-02 | ConfigFile content uses immutable versioning / content-addressable storage, deduplicating with SHA-256 blobs and keeping commit metadata. | Guarantees job reruns use the version that existed at submit time. |
| FR-WORKLOAD-03 | Support creating and destroying runtime instances from a ConfigFile, and listing instance pods. | /configfiles/{id}/instance. |
| FR-WORKLOAD-04 | Provide Job templates, Job submit, Job list, Job detail, and Job cancel. | The job plugin provides /api/v1/jobs. |
| FR-WORKLOAD-05 | Job states cover submitted, queued, running, completed, failed, cancelled, maintaining start/completion/error fields. | docs/en/features/12_jobs.md. |
| FR-WORKLOAD-11 | Support Job GPU usage, summary, timeline, and breakdown queries. | /jobs/{id}/gpu-* (read model provided by usage-observability; this service aggregates). |

## 3. Owned Data

`config_files`, `config_blobs`, `config_commits/versions`, `jobs`, `job_logs`, job templates.

## 4. Current Code/Route Mapping

- Handlers: `configfile.go`, job plugin
- Application: `application/configfile`, `application/job`
- Domain: `domain/job`
- Routes: `/api/v1/configfiles`, `/api/v1/jobs`

## 5. Job Submit Saga

```
Validate → ReserveQuota (scheduler-quota) → ResolveImage/Storage (image-registry / storage)
        → CreateK8sWorkload (k8s-control) → Commit / Release (compensation)
```

- The synchronous phase must complete validation and enqueueing within 2 seconds (NFR-PERF-02).
- Every step carries an idempotency key and a compensating action (NFR-DATA-02/03).

## 6. Events

| Direction | Event | Counterpart | Purpose |
| --- | --- | --- | --- |
| Publish | ConfigCommitted | audit, notification | Preserve immutable config versions |
| Publish | JobSubmitted/Queued/Running/Succeeded/Failed/Cancelled | usage, audit, notification, k8s-control, gateway | Status push, usage accounting, resource release |
| Subscribe | QuotaReserved / QuotaReleased | scheduler-quota | Advance the Job state machine |
| Subscribe | PVCProvisioned / StorageBound | storage | Update mountable volumes |
| Subscribe | ImageApproved / ImagePublished | image-registry | Update allow-list snapshots |
| Subscribe | GroupMembershipChanged / ProjectDeleted | org-project | Membership snapshots, project cleanup |

## 7. Non-Functional Highlights

- Config blobs/versions stay immutable; any overwrite creates a new version (NFR-DATA-04).
- Background workers such as the job dispatcher need horizontal sharding or mutual-exclusion locks (NFR-SCALE-02, NFR-AVAIL-03).
- K8s creation, image pulling, and scheduling waits are tracked via state machine/events without blocking the foreground.

## 8. Decomposition Notes

Phase 5 (final batch). May initially be co-deployed with scheduler-quota-service while keeping code and data boundaries.
