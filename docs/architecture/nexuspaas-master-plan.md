# NexusPaas GA Architecture Master Plan

## Purpose

NexusPaas is moving from a microservice-ready modular monolith toward GA-grade,
coarse-grained deployable units. The goal is operationally credible service
boundaries, not maximum service count. The platform should keep the current
developer velocity and external API stability while proving data ownership,
contract compatibility, staged rollout, rollback, and observability in a
production-like staging environment.

## Current State

The backend is one Go module with 15 logical services selected at runtime by
`SERVICE_NAME`. The repository already contains:

- a platform gateway and 15 bounded-context service packages;
- Production Beta Kubernetes manifests and service registry behavior;
- operational readiness, non-functional requirements, and event-contract docs;
- quality gates for tests, build, security scans, and Sonar when configured;
- opt-in live E2E coverage for LDAP, Kubernetes deploy, runtime expiry, and
  preemption.

This is a sound modular-monolith baseline. It is not yet GA-complete
microservices because shared physical PostgreSQL and transition owner-read
contracts still exist, live staging evidence is missing, and deployable-unit
rollback/evidence is not yet enforced as a release gate.

## Target Architecture

The 90-day target is 8 coarse deployable units. They map the 15 logical services
into units that can be deployed, observed, rolled back, and evolved with lower
operational risk than a 15-way physical split.

| Deployable Unit | Logical Services | Primary Capability |
| --- | --- | --- |
| `platform-gateway` | platform-gateway | Edge routing, external API compatibility, auth entry, service registry |
| `iam-unit` | identity-service, authorization-policy-service | Authentication, identity projection, API tokens, RBAC/PDP, policy bundles |
| `tenant-unit` | org-project-service | Groups, projects, membership, quota metadata, workspace settings |
| `collaboration-unit` | audit-compliance-service, request-notification-service, media-upload-service | Audit, forms, notifications, announcements, media metadata |
| `platform-io-unit` | storage-service, image-registry-service, integration-proxy-service | Storage, Harbor/image governance, external proxy integrations |
| `usage-observability` | usage-observability-service | Usage read models, GPU/resource summaries, dashboard reads |
| `compute-api` | workload-service, ide-service | Config files, job API, IDE lifecycle, user-facing compute commands |
| `compute-control-plane` | scheduler-quota-service, k8s-control-service | Admission, quota, preemption, Kubernetes commands, runtime cleanup |

## Architecture Decisions

- Keep external `/api/v1` route shapes and the standard response envelope
  stable through the GA migration.
- Keep the single Go module initially. The first GA milestone is independently
  verifiable runtime units, not separate repositories.
- Prefer Outbox/Inbox and read models over direct cross-service table access.
- Keep synchronous owner-read APIs only for latency-sensitive validation paths,
  and retire them when event-fed read models are complete and verified.
- Treat the shared database as temporary migration scaffolding. It must not be
  used to justify new cross-service repository dependencies.
- Split compute last. Workload, quota, Kubernetes, runtime expiry, and
  preemption are the highest TOCTOU-risk workflows.
- Use Kubernetes workload identity or an equivalent service identity for the GA
  service-to-service path. Static `SERVICE_API_KEY` remains a Beta fallback.
- Add service mesh or mTLS only after the team can operate the added complexity
  and has a concrete security or traffic-management need.

## 90-Day Outcomes

By Day 90, a GA architecture candidate is acceptable only if:

- the 8 deployable units have staging manifests or runtime configuration,
  independent readiness, synthetic smoke, rollback, and redeploy evidence;
- core internal contracts have provider/consumer tests and versioned schemas;
- Outbox/Inbox event flow exists for the highest-risk cross-service state;
- read models replace high-risk shared-store reads in scheduler, workload,
  org-project, identity, and authorization paths;
- compute saga behavior is documented and tested for reserve, commit, release,
  dispatch, cleanup, plan windows, duration limits, and preemption;
- GitHub-hosted quality gates require tests, build, scans, and Sonar when
  credentials are configured;
- `problem.md` contains no unaccepted GA architecture blocker.

## Rejected Alternatives

| Alternative | Reason Rejected |
| --- | --- |
| Split all 15 services immediately | Too much operational and data-consistency risk for 90 days. It increases failure modes before contract and staging discipline are mature. |
| Keep a permanent shared database | Produces a distributed monolith and makes ownership, rollback, and incident triage ambiguous. |
| Put orchestration in the gateway | Turns the gateway into a domain service and creates hidden coupling across all units. |
| Use distributed transactions | Availability and operability costs are too high; sagas, idempotency, and compensation are the selected model. |
| Add service mesh first | The current risk is contract/data maturity, not mesh traffic management. Workload identity can deliver the first GA security step with less operational cost. |

## Acceptance Gates

- Architecture docs and ADRs are reviewed before implementation PRs.
- Every new internal API or event has a named producer, consumer, schema
  version, compatibility rule, and contract test.
- Every migration slice has expand, dual-read/write, backfill, compare,
  cutover, contract, and rollback evidence.
- Every deployable unit has health/readiness, metrics, traces, logs, runbook,
  owner, rollback, and synthetic smoke coverage.
- E2E tests remain focused on critical user journeys; lower-level contract and
  integration tests carry most cross-service confidence.
