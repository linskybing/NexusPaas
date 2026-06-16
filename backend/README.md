# NexusPaas — Microservice Architecture Documentation

Version: v0.1
Date: 2026-06-12

## 1. Purpose

This directory decomposes the existing modular-monolith Go/Gin backend (1,464 Go files, 229 OpenAPI paths, 36 handler modules) into 15 microservices based on bounded contexts and data ownership. Each microservice has its own folder and requirements document.

## 2. Current State Assessment

The current backend is a "modular monolith", not a set of microservices: it already uses API / Application / Domain / Repository layering with clear module boundaries, but a single Gin process registers most routes, and modules collaborate through a shared repository container and a single PostgreSQL database.

**Core design principle: do NOT split every handler or URL prefix into its own service.** The decomposition is driven by:

- Bounded context (business semantic boundaries)
- Data ownership (each service owns its own schema/database)
- Transactional consistency (quota reservation, dispatch, etc. need a single arbiter)
- External system integration (K8s, Harbor, MinIO, Longhorn, LDAP)
- Scaling characteristics (long-lived proxy connections, background workers, read-heavy read models)

## 3. Service Catalog

| Service | Category | Documentation |
| --- | --- | --- |
| platform-gateway | Edge | [platform-gateway/README.md](platform-gateway/README.md) |
| identity-service | Core | [identity-service/README.md](identity-service/README.md) |
| authorization-policy-service | Core | [authorization-policy-service/README.md](authorization-policy-service/README.md) |
| org-project-service | Core | [org-project-service/README.md](org-project-service/README.md) |
| workload-service | Compute | [workload-service/README.md](workload-service/README.md) |
| scheduler-quota-service | Compute | [scheduler-quota-service/README.md](scheduler-quota-service/README.md) |
| k8s-control-service | Compute/Infra | [k8s-control-service/README.md](k8s-control-service/README.md) |
| ide-service | Compute | [ide-service/README.md](ide-service/README.md) |
| storage-service | Data | [storage-service/README.md](storage-service/README.md) |
| image-registry-service | Supply Chain | [image-registry-service/README.md](image-registry-service/README.md) |
| usage-observability-service | Ops/Read Model | [usage-observability-service/README.md](usage-observability-service/README.md) |
| audit-compliance-service | Ops | [audit-compliance-service/README.md](audit-compliance-service/README.md) |
| request-notification-service | Collaboration | [request-notification-service/README.md](request-notification-service/README.md) |
| integration-proxy-service | Edge/Tools | [integration-proxy-service/README.md](integration-proxy-service/README.md) |
| media-upload-service | Support | [media-upload-service/README.md](media-upload-service/README.md) |

## 4. Shared Documents

| Document | Content |
| --- | --- |
| [docs/non-functional-requirements.md](docs/non-functional-requirements.md) | Platform-wide non-functional requirements (security, availability, performance, consistency, observability, etc.) |
| [docs/event-contracts.md](docs/event-contracts.md) | Cross-service event contracts (publishers, subscribers, purpose) |
| [docs/migration-roadmap.md](docs/migration-roadmap.md) | Decomposition phases, risks and mitigations, cutover acceptance criteria |
| [docs/api-route-mapping.md](docs/api-route-mapping.md) | Mapping of existing /api/v1 routes → target services |

## 5. Target Architecture Principles

- **platform-gateway is the single external entry point.** In phase 1 it preserves the existing `/api/v1` paths and response schema (NFR-COMPAT-01).
- **Each service owns its own database or schema.** Cross-service DB joins and cross-DB foreign keys are forbidden; cross-service references store only UUIDs and necessary snapshots (NFR-DATA-01).
- **Cross-service commands use REST/gRPC**; cross-service state synchronization uses an event bus with Outbox/Inbox (NFR-DATA-02).
- **Authorization is centralized in authorization-policy-service** (PDP API + SDK); services must not duplicate inconsistent RBAC logic (NFR-SEC-02).
- **All Kubernetes operations are centralized in k8s-control-service**; no other service may call the Kubernetes API directly.

## 6. Technology Stack

Go, Gin, GORM, PostgreSQL, Redis, Kubernetes, Prometheus, OpenTelemetry, Casbin, OIDC, Harbor, MinIO, Longhorn, Volcano/VCJob.

## 7. Conclusion

The best decomposition is a bounded-context split into "Gateway + core platform services + compute control plane + data/storage/image services + ops/collaboration services". In the short term, do not split all handlers at once; instead, first establish boundary governance with events, data ownership, service API contracts, and the Gateway. In the mid term, extract the low-coupling and external-integration services. Finally, split the workload/scheduler/k8s control plane — the parts with the highest transactional and consistency risk.
