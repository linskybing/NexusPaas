# Project Structure Guidelines

This repository currently uses a Go modular-monolith backend with
microservice-ready boundaries. The target is 8 deployable units, not a
big-bang split into 15 physical services.

## Core Rules

* Each logical service owns its API, data model, config, tests, and deployment
  intent.
* Do not add new cross-service writable database access.
* Keep shared code small, stable, and non-business-specific.
* Split files by responsibility, not arbitrary line count.
* Prefer small duplication over premature shared abstractions.
* Every structure change must be traceable to an approved plan.

## Current Repository Shape

```text
backend/
├── cmd/microservice/                 # shared service entrypoint
├── internal/platform/                # shared runtime, middleware, routing, config
├── internal/services/<service>/      # logical service packages
├── deploy/                           # local, k3s, and production-beta manifests
├── docs/                             # backend operational and contract docs
└── scripts/                          # local and CI gates

docs/
├── adr/                              # durable architecture decisions
├── architecture/                     # long-lived architecture docs
├── agents/                           # repo workflow rules
└── plan/                             # active or unmerged implementation plans
```

## Target Deployable Units

| Unit | Logical Services |
| --- | --- |
| `platform-gateway` | `platform-gateway` |
| `iam-unit` | `identity-service`, `authorization-policy-service` |
| `tenant-unit` | `org-project-service` |
| `collaboration-unit` | `audit-compliance-service`, `request-notification-service`, `media-upload-service` |
| `platform-io-unit` | `storage-service`, `image-registry-service`, `integration-proxy-service` |
| `usage-observability` | `usage-observability-service` |
| `compute-api` | `workload-service`, `ide-service` |
| `compute-control-plane` | `scheduler-quota-service`, `k8s-control-service` |

## Service Shape

Within `backend/internal/services/<service>/`, prefer boring ownership:

```text
spec.go           # route and service registration
model.go          # typed domain records when needed
repository.go     # service-owned persistence
*_test.go         # unit, contract, and boundary tests
migrations/       # service-owned schema changes when applicable
```

Do not introduce this structure speculatively. Add files only when the service
actually owns that behavior.

## Boundary Rules

A service boundary must be based on:

* business capability;
* data ownership;
* API or event ownership;
* deployment independence;
* failure isolation;
* observability;
* rollback strategy.

Do not create a service only because a folder is large.
