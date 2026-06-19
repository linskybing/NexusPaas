# ADR 0001: GA 8 Deployable Units

Status: Accepted
Date: 2026-06-19

## Context

NexusPaas currently runs as one Go module with 15 logical services selected by
`SERVICE_NAME`. The GA roadmap targets operationally credible decomposition
without a big-bang 15-service split. The reviewed architecture baseline maps the
15 logical services into 8 coarse deployable units so contracts, data ownership,
observability, rollout, and rollback can be proven before further splitting.

The decision must preserve external `/api/v1` compatibility while the platform
moves from modular-monolith readiness to deployable-unit evidence.

## Decision

NexusPaas will use these 8 deployable units as the 90-day GA target:

| Deployable Unit | Logical Services | Primary Ownership |
| --- | --- | --- |
| `platform-gateway` | platform-gateway | Edge routing, external API compatibility, auth entry, service registry |
| `iam-unit` | identity-service, authorization-policy-service | Authentication, identity projection, API tokens, RBAC/PDP, policy bundles |
| `tenant-unit` | org-project-service | Groups, projects, membership, quota metadata, workspace settings |
| `collaboration-unit` | audit-compliance-service, request-notification-service, media-upload-service | Audit, forms, notifications, announcements, media metadata |
| `platform-io-unit` | storage-service, image-registry-service, integration-proxy-service | Storage, image governance, transfer records, external proxy integrations |
| `usage-observability` | usage-observability-service | Usage read models, GPU/resource summaries, dashboard reads |
| `compute-api` | workload-service, ide-service | User-facing job/config APIs and IDE lifecycle |
| `compute-control-plane` | scheduler-quota-service, k8s-control-service | Admission, quota, preemption, Kubernetes commands, runtime cleanup |

The single Go module may remain during the first GA milestones. The acceptance
condition is independently verifiable runtime units with explicit contracts,
owned data, health/readiness, metrics, logs, traces, synthetic smoke, rollback,
and redeploy evidence.

The platform gateway remains an edge and compatibility unit. It must not become
a hidden domain orchestrator. Owning units continue to enforce authorization and
domain invariants.

## Compatibility And Contract Requirements

- External `/api/v1` route shapes and response envelopes stay stable during the
  migration.
- Internal owner-read, command, and event contracts must be versioned before
  runtime contract changes.
- Logical-service labels must remain visible in logs, metrics, traces, service
  registry output, and evidence artifacts even when services are co-deployed in
  a coarse unit.
- A unit is not GA-ready until it has an owner, runbook, SLO, rollback plan, and
  staging evidence.

## Consequences

- Decomposition can proceed by evidence and risk instead of service count.
- Teams can harden boundaries while preserving current developer velocity and
  single-module refactoring safety.
- Future splits beyond 8 units require evidence that ownership, scale, release
  cadence, or failure isolation justify the added operational cost.
- Compute remains intentionally late because workload submit, quota, Kubernetes
  dispatch, cleanup, and preemption have the highest TOCTOU risk.

## Rejected Alternatives

| Alternative | Reason Rejected |
| --- | --- |
| Split all 15 logical services immediately | Creates too many deployment, contract, and data-consistency failure modes before evidence gates exist. |
| Keep one permanent all-in-one runtime | Does not prove rollback, failure isolation, or deployable-unit ownership for GA. |
| Put cross-domain orchestration in `platform-gateway` | Turns the gateway into a domain service and hides ownership behind edge routing. |
| Split repositories before runtime evidence | Adds coordination cost before contracts and staging gates prove the boundary. |

## Follow-up Evidence

- Add versioned internal contract fixtures and provider/consumer tests.
- Add Outbox/Inbox and read-model slices for high-risk cross-unit state.
- Capture deploy, smoke, rollback, redeploy, and post-redeploy smoke evidence for
  each unit in staging.
- Keep `problem.md` blockers open until implementation and evidence gates pass.

## Reversal

This ADR can be replaced only by a reviewed architecture ADR that proves a
smaller or larger unit set has better ownership, scale, release cadence, failure
isolation, and rollback evidence. Reversal must preserve external `/api/v1`
compatibility and must not require database restore as the rollback strategy.
