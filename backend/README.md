# NexusPaas — Microservice Architecture Documentation

Version: v0.1
Date: 2026-06-12

## 1. Purpose

This document maps the NexusPaas backend's **bounded-context decomposition**: 15
logical services defined by business semantics and data ownership.

The backend is a **Go modular monolith**, not 15 separate services. A single
`microservice` binary (`cmd/microservice`) hosts all 15 logical services as Go
packages under `internal/services/`; `SERVICE_NAME` gates which ones a given
process serves. For Production Beta those 15 logical services are packaged into
**8 deployable units** (`internal/platform/config.go`,
`deploy/k3s/production-beta/backend-units.yaml`).

### Vocabulary

- **logical service** — one of the 15 bounded contexts in §3; implemented at
  `internal/services/<pkg>/`, owns its schema under `migrations/<service>/`.
- **deployable unit** — one of the 8 physical units that host the logical
  services at runtime. The deployment **source of truth** is
  `deploy/k3s/production-beta/backend-units.yaml`.
- **package** — the Go implementation of a logical service, e.g.
  `internal/services/imageregistry/`.
- **reference manifest** — the non-production per-service deployment sketches
  under `deploy/reference/per-service/`; deployed by nothing.

### Repository layout

```
backend/
├── cmd/microservice/           # the single binary
├── internal/services/<pkg>/    # 15 logical service implementations
├── internal/platform/          # shared runtime (config, migrate, store, ...)
├── migrations/<service>/       # service-owned schema migrations
├── deploy/                     # k3s/local/hpc manifests (production source of truth)
│   └── reference/per-service/  # non-production per-service manifest sketches
├── docs/                       # backend runtime/contract/operator docs
│   └── services/<service>.md   # per-logical-service contract docs
└── Dockerfile                  # builds the shared binary
```

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
| platform-gateway | Edge | [docs/services/platform-gateway.md](docs/services/platform-gateway.md) |
| identity-service | Core | [docs/services/identity-service.md](docs/services/identity-service.md) |
| authorization-policy-service | Core | [docs/services/authorization-policy-service.md](docs/services/authorization-policy-service.md) |
| org-project-service | Core | [docs/services/org-project-service.md](docs/services/org-project-service.md) |
| workload-service | Compute | [docs/services/workload-service.md](docs/services/workload-service.md) |
| scheduler-quota-service | Compute | [docs/services/scheduler-quota-service.md](docs/services/scheduler-quota-service.md) |
| k8s-control-service | Compute/Infra | [docs/services/k8s-control-service.md](docs/services/k8s-control-service.md) |
| ide-service | Compute | [docs/services/ide-service.md](docs/services/ide-service.md) |
| storage-service | Data | [docs/services/storage-service.md](docs/services/storage-service.md) |
| image-registry-service | Supply Chain | [docs/services/image-registry-service.md](docs/services/image-registry-service.md) |
| usage-observability-service | Ops/Read Model | [docs/services/usage-observability-service.md](docs/services/usage-observability-service.md) |
| audit-compliance-service | Ops | [docs/services/audit-compliance-service.md](docs/services/audit-compliance-service.md) |
| request-notification-service | Collaboration | [docs/services/request-notification-service.md](docs/services/request-notification-service.md) |
| integration-proxy-service | Edge/Tools | [docs/services/integration-proxy-service.md](docs/services/integration-proxy-service.md) |
| media-upload-service | Support | [docs/services/media-upload-service.md](docs/services/media-upload-service.md) |

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
