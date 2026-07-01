# NexusPaas

[![Go](https://img.shields.io/badge/Go-1.25-00ADD8.svg)](https://go.dev/)
[![Backend Quality Gate](https://github.com/linskybing/nexuspaas/actions/workflows/backend-quality-gate.yml/badge.svg)](https://github.com/linskybing/nexuspaas/actions/workflows/backend-quality-gate.yml)

**NexusPaas** is an open-source **Platform-as-a-Service** backend for running an
AI/ML compute platform on Kubernetes: identity and access, tenant/project
management, workload scheduling with quota and GPU accounting, image registry,
storage, and integration proxying behind one API surface.

> Status: **v0.1** — the backend is a Go-based modular monolith with
> service-boundary awareness. It has 15 logical bounded-context services in one
> Go module; Production Beta manifests and local smoke tests now run those
> services as 8 physical backend units while GA isolation hardening continues.

## Architecture

Current runtime:

- Go standard library `net/http` / `http.ServeMux`, custom route registry,
  middleware, policy helpers, and OpenAPI generation.
- PostgreSQL via `pgx`, Redis Streams/in-memory events, Kubernetes `client-go`,
  MinIO/S3-compatible object storage, Dex/OIDC integration, and Prometheus /
  OpenTelemetry instrumentation.
- Local development can co-host all logical services in one process with
  `SERVICE_NAME=all`; staging and production hardening is moving toward explicit
  deployable-unit selection.
- Production Beta can run 8 physical backend units, each using `SERVICE_NAME`
  for its deployable-unit alias while `SERVICE_URLS` still maps logical service
  names to the owning unit URL.

The reference distribution remains k3s + Longhorn + Harbor + MinIO + Dex +
Redis Streams. Those products are defaults for the packaged distribution, not
intended as permanent hard requirements in the platform core.

The current 15 logical services are:

| Service | Responsibility |
|---|---|
| `platform-gateway` | Edge routing, auth, rate limiting, CORS |
| `identity-service` | Authentication, API tokens, OIDC |
| `authorization-policy-service` | Permission policy / PDP |
| `org-project-service` | Organizations, projects, groups, GPU repos |
| `workload-service` | Job submission and lifecycle |
| `scheduler-quota-service` | Scheduling, preemption, quota reservations |
| `k8s-control-service` | Kubernetes cluster control |
| `image-registry-service` | Container image registry (Harbor) |
| `storage-service` | Volumes, Longhorn RWX health |
| `media-upload-service` | Media blobs in object storage |
| `integration-proxy-service` | Upstream integrations, VPN usage |
| `ide-service` | IDE workspaces |
| `request-notification-service` | Request/notification state |
| `usage-observability-service` | Usage telemetry, dashboards |
| `audit-compliance-service` | Audit reporting |

The Production Beta / GA deployment direction is 8 deployable units:

| Deployable Unit | Logical Services |
|---|---|
| `platform-gateway` | `platform-gateway` |
| `iam-unit` | `identity-service`, `authorization-policy-service` |
| `tenant-unit` | `org-project-service` |
| `collaboration-unit` | `audit-compliance-service`, `request-notification-service`, `media-upload-service` |
| `platform-io-unit` | `storage-service`, `image-registry-service`, `integration-proxy-service` |
| `usage-observability` | `usage-observability-service` |
| `compute-api` | `workload-service`, `ide-service` |
| `compute-control-plane` | `scheduler-quota-service`, `k8s-control-service` |

Shared infrastructure lives under [`backend/internal/platform`](backend/internal/platform)
(config, auth, observability, backing services, cluster adapters). The current
blocker ledger is [`blocker-ledger.md`](docs/acceptance/blocker-ledger.md), the remediation sequence is
[`docs/roadmap.md`](docs/roadmap.md), and deeper backend docs are in
[`backend/docs/`](backend/docs):
[architecture](backend/README.md) ·
[API route mapping](backend/docs/api-route-mapping.md) ·
[event contracts](backend/docs/event-contracts.md) ·
[migration roadmap](backend/docs/migration-roadmap.md) ·
[non-functional requirements](backend/docs/non-functional-requirements.md) ·
[E2E testing](backend/docs/e2e-testing.md)

## Tech stack

Go 1.25 · `net/http` · `pgx` · PostgreSQL · Redis Streams · Kubernetes
`client-go` · MinIO/S3-compatible object storage · Dex/OIDC integration ·
Prometheus · OpenTelemetry · Harbor integration

## Quick start (local)

Prerequisites: Go 1.25+, Docker, Docker Compose.

```bash
# 1. Start backing services (PostgreSQL, Redis, MinIO, Dex)
docker compose -f backend/deploy/local/docker-compose.yml up -d

# 2. Build and test
cd backend
make build
make test

# 3. Run the platform (all services co-hosted in one process for local dev)
go run ./cmd/microservice
```

Integration tests require the backing services above:

```bash
cd backend
go test -tags integration ./...
```

Kubernetes (k3s) manifests are under [`backend/deploy/k3s`](backend/deploy/k3s).

## Local quality checks

Backend developer checks are exposed through [`backend/Makefile`](backend/Makefile):

```bash
cd backend
make lint    # gofmt check + go vet
make check   # quick local CI gate: gofmt, vet, tests, build
```

Use `make help` to list all targets. Heavier CI-equivalent gates are available
when Docker, network access, and any required secrets are configured:
`make ci-docker`, `make ci-security`, `make ci-sonar`, and `make ci-all`.

## Contributing

This repository follows a structured plan → review → implement workflow; see
[AGENTS.md](AGENTS.md) and [`docs/agents/`](docs/agents) for the conventions. Before opening
a PR, run `make check` from `backend/` and address any formatting, vet, test, or
build failures.
