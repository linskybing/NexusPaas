# ADR 0004: Deployment Evidence Gates

Status: Accepted
Date: 2026-06-19

## Context

The GA roadmap requires the 8 deployable units to prove deploy, smoke, rollback,
and redeploy behavior before any unit is declared GA-ready. Existing local and
non-live gates provide strong Production Beta confidence, but `problem.md` still
tracks missing live staging deploy, smoke, rollback, and redeploy evidence.

This ADR records the evidence gate that future runtime and staging slices must
satisfy. It does not add manifests or run live staging in this docs-only slice.

## Decision

A deployable unit is not GA-ready until staging evidence exists for the same
candidate version and configuration that would be promoted. Evidence must prove
health, readiness, metrics, service registry visibility, logical-service smoke,
critical cross-unit journeys where applicable, rollback, redeploy, and
post-redeploy smoke.

Evidence must be attached to the PR, release notes, or an approved evidence
artifact directory. A skipped live gate requires an owner-approved reason and an
open `problem.md` blocker.

## Required Evidence Per Unit

| Evidence | Requirement |
| --- | --- |
| Candidate version | Image digest or immutable artifact ID plus runtime config source. |
| Health | `/healthz` output for the unit after rollout. |
| Readiness | `/readyz` output proving dependencies and startup checks are ready. |
| Metrics | `/metrics` availability with service/unit labels and request duration buckets. |
| Service registry | Registry output showing unit and logical-service placement. |
| Synthetic smoke | One read-only smoke endpoint per logical service in the unit. |
| Cross-unit journey | Critical workflow smoke where the unit participates in user-visible flows. |
| Rollback | Command, result, timestamp, and pre/post state for rollback. |
| Redeploy | Re-apply candidate version and prove post-redeploy smoke. |
| Correlation | Request IDs, trace IDs, timestamps, environment, and version for evidence artifacts. |

## Gate Policy

- Local gates still include `git diff --check`, full backend tests, vet, build,
  and `ci-security-gate.sh quick`.
- Contract, E2E, Docker collaboration, security scan, Sonar, and live staging
  gates are required when the slice touches their risk area.
- Docs-only ADR slices may defer live staging evidence, but must keep blockers
  open and must not claim deployable-unit readiness.
- Rollback must prefer image or config rollback plus reconciliation. Database
  restore is not the default rollback path.
- Secrets for staging evidence must come from approved secret mechanisms and must
  not be committed or pasted into PRs.

## Compatibility And Contract Requirements

- External `/api/v1` compatibility remains stable across rollout, rollback, and
  redeploy.
- Synthetic smoke must use externally supported routes or documented internal
  read-only probes. It must not depend on raw database access.
- Service registry and observability labels must preserve both deployable-unit
  and logical-service identity.
- Evidence must show stale read-model behavior when a migration slice changes
  read-model or Outbox/Inbox behavior.

## Consequences

- Future GA claims become auditable and repeatable instead of based on local
  build success alone.
- Release notes must carry enough artifact, version, and correlation data for
  reviewers and operators to reproduce the evidence.
- Live staging, remote Sonar, and supply-chain blockers remain real blockers
  until credentials and environments exist.
- Runtime slices must design rollback and redeploy before merge, not after an
  incident.

## Rejected Alternatives

| Alternative | Reason Rejected |
| --- | --- |
| Treat local tests as enough for deployable-unit readiness | Local tests do not prove staging rollout, dependency wiring, smoke, or rollback. |
| Capture deploy evidence only once for the whole backend | GA risk is unit-specific; each unit needs its own owner, smoke, and rollback evidence. |
| Use database restore as the primary rollback | Destructive restore is not acceptable as the normal rollback path for event-backed units. |
| Omit trace/request IDs from evidence | Reviewers and operators could not connect evidence to logs, metrics, or traces. |

## Follow-up Evidence

- Add staging runtime configuration for the 8 deployable units.
- Capture per-unit deploy, smoke, rollback, redeploy, and post-redeploy smoke
  artifacts.
- Make remote Sonar required when credentials are configured.
- Add SBOM and image signing gates after staging promotion becomes stable.

## Reversal

A future ADR can revise the evidence gate only if it preserves equivalent proof
of rollout, readiness, observability, smoke, rollback, redeploy, correlation, and
secret hygiene. Reversal must keep unproven staging readiness tracked in
`problem.md`.
