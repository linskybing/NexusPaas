# Project Structure Guidelines

This repository follows a **microservices-first** structure.

Each service should be independently understandable, testable, buildable, deployable, observable, and rollbackable.

## Core Rules

* Prefer `services/<domain>-service/` for each backend service.
* Each service owns its API, data model, config, tests, and deployment files.
* Do not share writable database tables across services.
* Keep shared code minimal and non-business-specific.
* Split files by responsibility, not arbitrary line count.
* Keep most files under **400 lines**.
* Review files over **400 lines** for mixed responsibilities.
* Consider subfolders when a directory has **15–20+ files** or multiple business topics.
* Every structure change must be traceable to an approved plan.

## Repository Shape

```text
.
├── AGENTS.md
├── docs/
│   ├── agents/
│   ├── plan/
│   └── architecture/
│
├── services/
│   ├── auth-service/
│   ├── user-service/
│   ├── project-service/
│   ├── job-service/
│   ├── artifact-service/
│   └── notification-service/
│
├── packages/
│   ├── config/
│   ├── logger/
│   ├── errors/
│   └── testkit/
│
├── deploy/
│   ├── k8s/
│   ├── helm/
│   └── docker-compose/
│
├── scripts/
└── tools/
```

## Service Shape

Each service should follow this structure:

```text
services/<service-name>/
├── src/
│   ├── handler/
│   ├── usecase/
│   ├── domain/
│   ├── repository/
│   ├── dto/
│   ├── config/
│   └── main/
│
├── tests/
│   ├── unit/
│   ├── integration/
│   └── contract/
│
├── migrations/
├── deploy/
├── Dockerfile
├── README.md
└── service.yaml
```

## Layer Responsibilities

| Layer         | Responsibility                                     |
| ------------- | -------------------------------------------------- |
| `handler/`    | HTTP, RPC, CLI, or message transport input/output  |
| `usecase/`    | Application workflow and orchestration             |
| `domain/`     | Business rules, entities, value objects            |
| `repository/` | Persistence interfaces and infrastructure adapters |
| `dto/`        | Request/response and mapping objects               |
| `config/`     | Runtime config loading and validation              |
| `main/`       | Service bootstrap and dependency wiring            |

Rules:

* Handlers should be thin.
* Domain logic must not depend on handlers.
* Domain logic must not directly depend on database clients.
* Usecases coordinate business flow.
* Repositories hide persistence details.
* DTOs must not become business objects.

## Shared Packages

Use `packages/` only for stable cross-cutting code.

Allowed examples:

```text
packages/config
packages/logger
packages/errors
packages/testkit
```

Avoid sharing:

```text
business rules
domain models
repositories
service-specific DTOs
database clients with business assumptions
```

Before adding shared code, confirm:

* It is used by at least two real services.
* It has clear ownership.
* It does not create service coupling.
* Small duplication would not be simpler.

Prefer small duplication over premature shared abstraction.

## Deployment Files

Service-specific deployment files should live inside each service:

```text
services/job-service/deploy/
```

Global deployment composition may live under:

```text
deploy/
```

Use global deployment files only for local development, platform-level setup, or cross-service orchestration.

## Service Boundary Rules

A service boundary must be based on:

* Business capability
* Data ownership
* API ownership
* Deployment independence
* Failure isolation
* Observability
* Rollback strategy

Do not create a service only because a folder is large.

## Default Rule

Each service should own its code, data, config, tests, and deployment. Keep files focused, avoid generic shared folders, and make every structure change small, reviewable, and tied to the approved plan.
