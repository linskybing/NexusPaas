# NexusPaaS Current Architecture Blockers

_Updated: 2026-06-20. Branch: `route-auth-hardening`._

## Current Architecture Reality

NexusPaaS is currently a Go modular monolith with service-boundary awareness.
The backend has 15 logical bounded-context services in one Go module and one
shared platform runtime. Production Beta now has an 8-physical-unit runtime
topology that hosts those 15 logical services; the remaining GA target is
clearer data ownership, reliable event delivery, stronger service identity, and
production-grade operational evidence.

Do not describe the current backend as mature production-grade microservices.
The accurate description is:

> Go-based modular monolith with service boundaries and an 8-unit Production
> Beta runtime topology; GA-grade microservice isolation is still in progress.

## Completed Or In Progress

| Area | Status | Notes |
| --- | --- | --- |
| 8-unit GA direction | Done | ADRs and architecture docs define the target deployable units. |
| Physical 8-unit runtime split | Done | Production Beta kustomize, runtime config, rollback evidence, local compose smoke, and CI gates use 8 backend units hosting 15 logical services. |
| Identity data boundary | Started | Identity-owned records and migrations exist; other core domains still need typed ownership work. |
| Contract fixtures | Started | Core event, owner-read, command, producer, and consumer fixture coverage exists for initial slices. |
| Projection visibility | Started | Projection lag, retry, replay, and dead-letter visibility exist; durable transactional delivery is still open. |
| Route auth and collision hardening | Pending merge | Current branch adds centralized internal-route auth validation and strict route collision checks. |
| Production Beta operations | Partial | Non-live gates and docs exist; live staging evidence remains open. |

## P0 Blockers Before Production/GA

| Area | Current Problem | Next Step |
| --- | --- | --- |
| Transactional outbox/inbox | Domain writes and event publishing are not yet guaranteed in the same database transaction, and durable relay/inbox semantics are incomplete. | Add Postgres-backed outbox/inbox tables, relay worker, idempotent consumer records, retry/dead-letter handling, and publish-lag evidence. |
| Typed domain data ownership | Core domains still rely too heavily on generic `platform_records` / JSONB payloads. | Move identity, tenant/project, workload, scheduler/quota, storage, registry/build, and billing-related data to typed schemas and repositories slice by slice. |
| Internal route auth | Handler-level service-auth checks are easy to forget. | Merge the current centralized internal-route auth validation and middleware hardening. |
| Route collision detection | Same-shape routes can silently override each other without explicit alias/override metadata. | Merge the current route collision validator and keep CI coverage. |
| API token verification | Token auth still scans stored token records and verifies hashes until one matches. | Use token IDs/prefixes so verification performs one indexed lookup and one hash check. |
| Trusted client IP resolution | Some paths use trusted proxy CIDR logic while identity login handling still trusts forwarded headers too directly. | Add one platform client-IP resolver and use it for rate limits, login failures, audit logs, and security events. |
| Environment profiles and PDP fail-closed | Config still depends on a production boolean instead of explicit `local`, `test`, `dev`, `staging`, and `production` profiles. | Add explicit environment profiles and fail startup in staging/production without a real PDP. |
| Reproducible toolchain | Local quick, Docker-backed, manifest rehearsal, and 8-unit collaboration gates exist; GA still needs remote CI and live staging evidence. | Keep `backend/scripts/ci-security-gate.sh quick` and Docker-backed collaboration evidence green, then capture live staging evidence per deployable unit. |

## P1 Architecture Maturity

| Area | Current Problem | Next Step |
| --- | --- | --- |
| Service identity | `SERVICE_API_KEY` remains the Beta service-to-service fallback. | Move the GA path to Kubernetes workload identity, mTLS, SPIFFE/SPIRE, or scoped per-service keys with rotation. |
| JWT/JWKS verification | Security-sensitive token verification is mostly custom. | Replace or wrap it with a mature Go OIDC/JWT/JWK library through an ADR-backed slice. |
| Migration runner | SQL migration execution is still too simple for GA auditing and rollback. | Adopt `golang-migrate`, `goose`, Atlas, or add version/checksum/locking/dirty-state support. |
| Provider coupling | Longhorn, Harbor, MinIO, Dex, Redis Streams, and k3s are still reference-stack assumptions in several places. | Separate core contracts from provider implementations before claiming portability. |
| Typed API contracts | Custom route specs and generated OpenAPI help, but do not replace typed request/response contracts. | Move critical APIs toward OpenAPI-first or explicit typed DTO contracts with fixtures. |
| Read-model drift and replay | Some visibility exists, but drift comparison and replay evidence are not yet enough for cutover. | Add projection drift checks before retiring owner-read/shared-store paths. |
| Per-unit runtime isolation | Non-live 8-unit runtime isolation is proven locally, but deployable-unit RBAC, network policy, migration ownership, and live staging evidence are not fully proven. | Capture staging deploy, smoke, rollback, and redeploy evidence for each of the 8 units. |

## P2 Documentation And Tooling

| Area | Current Problem | Next Step |
| --- | --- | --- |
| Documentation alignment | Older docs described the project as microservices-first and listed non-current runtime dependencies. | Keep README, roadmap, architecture docs, and backend docs aligned with current reality. |
| CI script size | The central security gate is useful but large. | Split checks only when there is real maintenance pain; keep the top-level script as the orchestrator. |
| Service ownership docs | Service-level ownership is partially documented across several files. | Consolidate owner, API, data, config, test, and deployment responsibility per deployable unit. |
| Provider ADRs | Provider abstraction is a target but not yet documented as concrete ADRs. | Add ADRs when replacing or abstracting current reference-stack assumptions. |
| Supply chain | SBOM generation and image signing are GA goals but not enforced. | Add Syft/Cosign or equivalent after staging promotion is stable. |
| Remote Sonar | GitHub-hosted Sonar still depends on repository secrets. | Provision reachable Sonar credentials and make the remote gate required when configured. |

## Preserved Direction

- Keep the modular monolith while boundaries are being proven.
- Keep the 8 deployable-unit target instead of forcing a 15-way split.
- Keep the reference distribution as k3s + Longhorn + Harbor + MinIO + Dex +
  Redis Streams until provider abstractions are justified by concrete needs.
- Prefer deleting stale docs and consolidating status over adding more planning
  files.
