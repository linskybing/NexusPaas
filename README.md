# NexusPaas

[![Go](https://img.shields.io/badge/Go-1.25-00ADD8.svg)](https://go.dev/)
[![CI](https://github.com/linskybing/nexuspaas/actions/workflows/ci.yml/badge.svg)](https://github.com/linskybing/nexuspaas/actions/workflows/ci.yml)

**NexusPaas** is an open-source, microservices-first **Platform-as-a-Service** backend
for running an AI/ML compute platform on Kubernetes — identity & access, organization and
project management, workload scheduling with quota and GPU accounting, image registry,
storage, and integration proxying, all behind a single API gateway.

> Status: **v0.1** — the backend is organized as 15 bounded-context microservices sharing
> a Go module, with k3s/Kubernetes deployment manifests and a cross-service E2E suite.

## Architecture

A single **platform-gateway** is the edge entry point; each service owns its API, domain
logic, data model, config, tests, and deployment files. Services collaborate over HTTP
read-contracts and a Redis Streams event bus, with PostgreSQL for persistence, MinIO for
object storage, and Dex for OIDC.

| Service | Responsibility |
|---|---|
| `platform-gateway` | Edge routing, auth, rate limiting, CORS |
| `identity-service` | Authentication, API tokens, OIDC |
| `authorization-policy-service` | Permission policy / PDP (Casbin) |
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

Shared infrastructure lives under [`backend/internal/platform`](backend/internal/platform)
(config, auth, observability, backing services, cluster adapters). Deeper design docs are in
[`backend/docs/`](backend/docs):
[architecture](backend/README.md) ·
[API route mapping](backend/docs/api-route-mapping.md) ·
[event contracts](backend/docs/event-contracts.md) ·
[migration roadmap](backend/docs/migration-roadmap.md) ·
[non-functional requirements](backend/docs/non-functional-requirements.md) ·
[E2E testing](backend/docs/e2e-testing.md)

## Tech stack

Go 1.25 · Gin · GORM · PostgreSQL · Redis Streams · Kubernetes (k3s) · MinIO · Dex (OIDC) ·
Casbin · Prometheus · OpenTelemetry · Harbor

## Quick start (local)

Prerequisites: Go 1.25+, Docker, Docker Compose.

```bash
# 1. Start backing services (PostgreSQL, Redis, MinIO, Dex)
docker compose -f backend/deploy/local/docker-compose.yml up -d

# 2. Build and test
cd backend
go build ./...
go test ./...

# 3. Run the platform (all services co-hosted in one process for local dev)
go run ./cmd/microservice
```

Integration tests require the backing services above:

```bash
cd backend
go test -tags integration ./...
```

For the Production Beta verification path, run the repository-owned Docker
gate. It starts isolated Postgres, Redis, and MinIO containers on non-default
ports, avoids Sonar's `localhost:9000`, runs migrations, integration coverage,
focused/full E2E, and runtime smoke checks:

```bash
backend/scripts/docker-e2e-gate.sh
```

Kubernetes (k3s) manifests are under [`backend/deploy/k3s`](backend/deploy/k3s).

## Contributing

This repository follows a structured plan → review → implement workflow; see
[AGENTS.md](AGENTS.md) and [`docs/agents/`](docs/agents) for the conventions. Before opening
a PR, make sure `go build ./...`, `go vet ./...`, `go test ./...`, and `gofmt -l .` are all
clean from `backend/`.
