# Open Source Quality Standard

## Goal

NexusPaas should be understandable, buildable, testable, and operable by an
external contributor without private context. Open-source quality is treated as
part of architecture quality because unclear ownership, hidden dependencies, or
unreviewed security exceptions make microservices unsafe to operate.

## Repository Standard

- The README explains product purpose, architecture, quick start, and quality
  checks.
- Service READMEs identify responsibility, owned data, dependencies, operations,
  and local test commands.
- Architecture docs record accepted tradeoffs and rejected alternatives.
- `blocker-ledger.md` tracks unresolved blockers, accepted risks, and follow-ups.
- Plans under `docs/plan/` document significant work before implementation.

## Code And Design Standard

- Keep changes simple, surgical, and aligned with service ownership.
- Prefer existing helpers and standard library behavior before adding packages.
- Keep shared platform code small and stable.
- Avoid speculative abstractions and framework churn.
- Preserve external API compatibility unless a versioned migration is approved.

## Security Standard

- No secrets, raw tokens, cookies, credentials, OIDC assertions, or sensitive
  payloads in source or logs.
- Auth and authorization are enforced at gateway and owning service boundaries.
- Internal service credentials are scoped and rotatable.
- Dependency, vulnerability, filesystem, image, and IaC scans are part of the
  release evidence.
- Security exceptions require owner, expiry, and compensating controls.

## Licensing Standard

- The repository license must be clearly discoverable before any public GA
  release.
- New dependencies must have licenses compatible with the NexusPaas project
  license and intended distribution model.
- Copyleft, source-available, commercial, or unclear licenses require an ADR
  and explicit maintainer approval before adoption.
- Required attribution, NOTICE, copyright, and third-party license files must be
  preserved in source and release artifacts.
- License exceptions need an owner, reason, expiry or review date, and
  documented compensating controls.
- Dependency license review belongs in the same PR that introduces or upgrades a
  major package.

## Test And Evidence Standard

- Critical packages and workflows need meaningful tests, not only superficial
  coverage.
- E2E tests cover user journeys; contract and integration tests cover service
  boundaries.
- Live tests are opt-in and must use unique resources with cleanup.
- Staging evidence is required before claiming deployable-unit readiness.
- Sonar Quality Gate must pass locally and in GitHub Actions for trusted
  events; fork pull requests may skip only because trusted repository secrets
  are unavailable.

## Contribution Standard

- One goal per branch and PR.
- PR descriptions include what, why, how, tests, risks, rollback, and links to
  docs/ADRs.
- Reviewers check requirement fit, microservice boundaries, 12-factor behavior,
  security, observability, tests, and rollback.
- Merge by squash only.

## Release Standard

- Release artifacts should be reproducible and promoted across environments.
- SBOM and image signing are GA goals.
- Rollback procedures are documented before release.
- Operational docs identify owner, SLO, dashboard, alert, runbook, and smoke
  coverage for each deployable unit.
