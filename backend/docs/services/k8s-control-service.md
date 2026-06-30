# k8s-control-service

Category: Compute/Infra | Phase: 5

## 1. Overview

The single Kubernetes API adapter. Responsible for namespace/resource operations, pod logs/events, exec/watch WebSockets, resource cleanup, and the K8s reconciler. **No other service may call Kubernetes directly** — all K8s commands are centralized here, running with minimized RBAC.

## 2. Functional Requirements

| ID | Requirement | Acceptance/Notes |
| --- | --- | --- |
| FR-K8S-01 | Provide health/readiness, metrics, and OpenAPI/Swagger. | /healthz, /readyz, /metrics, /openapi.* (every service must also expose its own). |
| FR-K8S-02 | Provide cluster summary, MPS mapping, node list/detail, and pod GPU usage. | /cluster (split with usage-observability: this service collects, the usage service aggregates and presents). |
| FR-K8S-03 | Provide pod logs, pod events, namespaced resource deletion, project resources, and project namespaces. | /k8s, /resources, /projects/{id}/resources. |
| FR-K8S-04 | Support admin project resource cleanup and user resource cleanup. | Prevents orphaned K8s resources. |
| FR-K8S-05 | Support WebSocket exec, namespace watch, pod logs stream, project watch, job status watch, and storage status watch. | /api/v1/ws/*. |
| FR-K8S-06 | Sync K8s reality into the database and caches via background workers/reconcilers. | resource reconciler, quota reconciler, cluster collector. |

## 3. Owned Data

K8s operation records, namespace mapping read models, pod/resource snapshots/caches.

## 4. Current Code/Route Mapping

- Handlers: `k8s.go`, `resource.go`, `websocket.go`, `handlers/resource`
- Application: `application/k8s`
- Routes: `/api/v1/k8s/*`, `/api/v1/resources/*`, `/api/v1/ws/*`, parts of `/api/v1/cluster/*`

## 5. External Interfaces

- Command APIs: CreateWorkload, DeleteResource, CreateNamespace, CleanupProjectResources (async command + status, NFR-PERF-01)
- Query APIs: pod logs/events, node/cluster snapshots
- WebSocket: exec, watch, logs stream (JWT-only + Proxy RBAC, NFR-SEC-04)

## 6. Events

| Direction | Event | Counterpart | Purpose |
| --- | --- | --- | --- |
| Subscribe | ProjectCreated / ProjectDeleted | org-project | Namespace creation/cleanup (saga participant) |
| Subscribe | PolicyChanged | authz | Sync ConfigMaps, invalidate RBAC caches |
| Subscribe | Job lifecycle events | workload | Resource release and watch pushes |
| Publish | reconcile results (resource state snapshots) | usage, workload | Sync K8s reality |

## 7. Non-Functional Highlights

- K8s API calls need timeouts, retry with backoff, and circuit breakers (NFR-RES-01); clear degradation when K8s is unavailable (NFR-RES-02).
- Reconcilers need leader election to avoid duplicate cleanup across replicas (NFR-AVAIL-03).
- All operations are isolated by namespace/domain; namespace/path spoofing must be impossible (NFR-MTEN-01).
- Long-lived WebSockets must scale independently with configurable timeouts (NFR-SCALE-01).

## 8. Decomposition Notes

Phase 5. Before extraction, inventory every code path that uses client-go directly and migrate all of them to this service's API; consolidate service-account permissions into this single service.
