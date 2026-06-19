# GA Architecture Roadmap

## 1. Objective

Document the 90-day GA architecture roadmap for moving NexusPaas from a microservice-ready modular monolith toward verified coarse-grained deployable units. The first implementation slice is documentation only: create the master architecture plan, service boundary map, data migration strategy, testing strategy, CI/CD governance, roadmap, and update unresolved risks in `problem.md`.

## 2. Background

NexusPaas already has a 15-service logical catalog, Production Beta manifests, quality gates, operational readiness docs, opt-in live Kubernetes E2E tests, and core journey E2E coverage. Opt-in live tests are not the same as captured live staging evidence; the live staging deploy/smoke/rollback rehearsal remains a documented blocker. The remaining long-term goal is not a big-bang split into 15 physical services. The selected direction is a 90-day GA architecture effort centered on 8 deployable units, Outbox/Inbox plus read models, stable external `/api/v1` contracts, staging evidence, and contract-driven migration away from shared-store coupling.

## 3. Source References

- `long-term.md`
- `problem.md`
- `README.md`
- `function.md`
- `backend/docs/migration-roadmap.md`
- `backend/docs/non-functional-requirements.md`
- `backend/docs/operational-readiness.md`
- `backend/docs/beta-launch-readiness.md`
- Microservice architecture skill references: service boundaries, communication contracts, data consistency, resilience/runtime, security/zero trust, observability/operations, testing/delivery, and migration/modernization.

## 4. Assumptions

- The target is 90 days, not a 6- or 12-month transformation.
- The GA architecture success target is 8 coarse deployable units: `platform-gateway`, `iam-unit`, `tenant-unit`, `collaboration-unit`, `platform-io-unit`, `usage-observability`, `compute-api`, and `compute-control-plane`.
- Outbox/Inbox and read models are the preferred data-boundary strategy.
- The current PR #14 baseline is accepted as the starting point.
- The unavailable `references/CSCC_AI_Platform_Backend` snapshot remains a parity risk, not a blocker to writing the GA roadmap.

## 5. Non-Goals

- Do not modify production Go code, route registration, database schema, migrations, Kubernetes manifests, or CI workflow behavior in this slice.
- Do not physically split services or introduce new infrastructure.
- Do not add new third-party dependencies.
- Do not claim GA completion; document the plan, acceptance gates, and remaining evidence requirements.

## 6. Current Behavior

The repository documents Production Beta readiness, 15 logical services, current capability inventory, operational readiness, non-functional requirements, and migration phases. It does not yet contain a single GA master architecture plan with the chosen 8 deployable-unit target, event/read-model migration plan, explicit 90-day milestones, and CI/CD governance for GA architecture decomposition.

## 7. Target Behavior

The repository should contain a concise architecture documentation set that makes the GA path decision-complete:

- why NexusPaas should remain a modular monolith while moving toward coarse deployable units;
- how the 15 logical services map to 8 deployable units;
- which unit owns which data, contracts, SLOs, security boundary, rollout, and rollback behavior;
- how Outbox/Inbox/read models replace shared-store access;
- how tests, contract gates, staging evidence, and release governance prove readiness;
- which risks remain in `problem.md`.

## 8. Affected Domains

- Architecture documentation.
- Microservice boundary and data ownership documentation.
- Testing and release governance documentation.
- Project roadmap and unresolved risk tracking.

## 9. Affected Files

- `docs/architecture/nexuspaas-master-plan.md`
- `docs/architecture/service-boundaries.md`
- `docs/architecture/cncf-package-strategy.md`
- `docs/architecture/data-migration-strategy.md`
- `docs/architecture/testing-strategy.md`
- `docs/architecture/ci-cd-and-pr-governance.md`
- `docs/architecture/open-source-quality-standard.md`
- `docs/architecture/observability-strategy.md`
- `docs/roadmap.md`
- `problem.md`

## 10. API / Contract Changes

No runtime API changes. The documentation must state that external `/api/v1` routes and response envelopes remain stable during the 90-day GA migration. Internal contract expectations are documentation-only in this slice: versioned owner-read/command APIs, versioned event schemas, tolerant readers, idempotent consumers, and explicit producer/consumer ownership.

## 11. Database / Migration Changes

No schema or migration changes. The documentation must require future database migration slices to use expand, dual-write/read, backfill, compare, cutover, and contract. Shared PostgreSQL must be documented as bounded migration scaffolding with retirement gates, not permanent shared ownership.

## 12. Configuration Changes

No runtime configuration changes. The documentation must define future deployable-unit configuration expectations: unit-level service URLs, readiness, rollback, migration ownership, service identity, and staging evidence requirements.

## 13. Observability Changes

No runtime observability changes. The documentation must align future GA gates with the existing operational readiness contract: logs, metrics, traces, request/trace correlation, SLOs, dashboards, alerts, synthetic smoke, rollback, and staging evidence.

## 14. Security Considerations

The roadmap must preserve defense in depth: gateway authentication is not sufficient, service-to-service calls require authenticated service identity, domain authorization remains in owning services, static `SERVICE_API_KEY` is Beta fallback only, and GA should prefer Kubernetes workload identity or equivalent before considering service mesh/mTLS.

## 15. Implementation Steps

1. Add this plan with `Status: Draft`.
2. Obtain Reviewer Agent approval before editing the target architecture documents.
3. Add the GA master architecture plan with 8 deployable units, 90-day milestones, success gates, and rejected alternatives.
4. Add service boundary documentation mapping 15 logical services to the 8 units with owners, data, APIs/events, SLOs, security, observability, rollout, and rollback.
5. Add the CNCF/package strategy that identifies mature external products to prefer for gateway, GitOps, observability, policy, secrets, certificates, supply chain, messaging, and service mesh.
6. Add the data migration strategy for Outbox/Inbox, read models, event envelope, reconciliation, stale data behavior, and shared-store retirement gates.
7. Add the GA testing strategy for unit, integration, contract, E2E, live staging, failure-mode, and evidence gates.
8. Add CI/CD and PR governance for branch/PR discipline, required gates, staging evidence, Sonar remote gate, supply-chain checks, and rollback evidence.
9. Add the open-source quality standard covering maintainability, licensing, security posture, contribution hygiene, and release evidence.
10. Update the existing observability strategy with GA-specific expectations for 8 deployable units, staging evidence, service identity, and read-model/event-lag telemetry.
11. Add `docs/roadmap.md` with Day 0-15, Day 16-35, Day 36-55, Day 56-75, and Day 76-90 milestones.
12. Update `problem.md` to reflect GA architecture risks and current blockers without removing existing unresolved risks.

## 16. Verification Plan

Documentation and syntax checks:

```sh
git diff --check
```

Backend sanity checks:

```sh
go -C backend test ./internal/platform -run 'Deployment|Operational|Release|Beta' -count=1
go -C backend test ./... -count=1
go -C backend vet ./...
go -C backend build ./...
bash backend/scripts/ci-security-gate.sh quick
```

Reviewer verification should confirm requirement fit, approved-plan alignment, SOLID/12-factor implications, microservice boundary quality, test plan clarity, Sonar status expectations, risks, and diff scope.

## 17. Rollback Plan

Revert the added architecture and roadmap documentation plus the `problem.md` update. No production code, schema, or deployment rollback is required.

## 18. Risks and Tradeoffs

- A documentation-only slice does not itself enforce architecture gates; follow-up PRs must add enforcement tests and CI integration.
- An 8-unit target reduces operational risk compared with 15 independent services, but still requires disciplined contracts and staging evidence.
- Outbox/read-model migration introduces eventual consistency that must be documented per user-visible workflow before implementation.
- GitHub Sonar and staging evidence may remain blocked by credentials or environment availability.

## 19. Reviewer Checklist

- Requirement fit: the plan implements the requested 90-day GA architecture roadmap slice.
- Scope: documentation only, no production code or deployment mutation.
- Architecture: avoids big-bang service split and uses coarse deployable units.
- Data ownership: shared database is temporary migration scaffolding with retirement gates.
- Security: service identity and authorization are not delegated only to the gateway.
- Testing: verification commands and future GA gates are explicit.
- Rollback: documentation-only rollback is realistic.

## 20. Status

Status: Approved

## 21. Reviewer Final Approval

Status: Approved

- Requirement fit: the implementation documents the Day 0-15 GA architecture baseline, including the 8-unit target, boundary map, data migration strategy, test strategy, CI/CD governance, open-source quality standard, roadmap, and blocker tracking.
- Approved-plan alignment: the final diff stays within documentation, planning, observability strategy, roadmap, and `problem.md` tracking files.
- Architecture verdict: Yellow/ready for incremental follow-up. The proposal keeps a modular monolith while moving toward coarse deployable units, and it treats shared PostgreSQL, owner-read contracts, staging evidence, service identity, and contract testing as explicit blockers rather than completed work.
- SOLID and 12-factor impact: no runtime code changes; future work is constrained toward owned data, externalized config, stateless deployable units, observability, and rollback evidence.
- Verification: `git diff --check`, `go -C backend test ./... -count=1`, `go -C backend vet ./...`, `go -C backend build ./...`, and `bash backend/scripts/ci-security-gate.sh quick` passed locally.
- Security and secrets: local metadata, owner tokens, tunnel tokens, and direct owner identity values are not part of the PR.
- Risks: this is a documentation baseline only. Follow-up PRs must add ADRs, contract fixtures/tests, Outbox/Inbox implementation, read-model drift checks, staging evidence, remote Sonar enforcement, and supply-chain gates.
- Diff scope: no production Go code, database schema, Kubernetes manifests, CI workflow behavior, or external `/api/v1` contract changed.
