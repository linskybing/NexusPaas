# CI/CD And PR Governance

## Branch And PR Policy

- Every architecture, feature, refactor, CI, or documentation goal uses a
  dedicated branch and PR.
- PRs must explain what changed, why it changed, and how it was implemented.
- Large goals start with an approved `docs/plan/` document.
- Squash merge only. `main` should keep one logical commit per PR.
- Delete merged local branches after confirming the branch was pushed or merged.

## Required Review Roles

- Plan Agent writes the implementation plan.
- Reviewer Agent approves the plan before implementation.
- Code Agent implements only the approved plan.
- Reviewer Agent verifies the final diff, tests, SOLID/12-factor alignment,
  microservice quality, risks, and scope before completion.

## Quality Gates

Every GA architecture PR should report:

```sh
git diff --check
go -C backend test ./... -count=1
go -C backend vet ./...
go -C backend build ./...
bash backend/scripts/ci-security-gate.sh quick
```

Depending on scope, the PR should also run:

- focused service package tests;
- provider/consumer contract tests;
- E2E tests for affected user journeys;
- Docker collaboration smoke;
- govulncheck, OSV, Trivy, and image build scans;
- SonarScanner Quality Gate.

GitHub-hosted Sonar fails closed for push, workflow dispatch, and
same-repository pull requests when `SONAR_TOKEN` or `SONAR_HOST_URL` is missing.
Fork pull requests may skip Sonar because GitHub does not expose trusted
repository secrets to forked workflows.

## Staging Promotion

A candidate can move toward GA staging only when:

- the same artifact is promoted rather than rebuilt per environment;
- secrets come from Kubernetes Secret, ExternalSecret, Sealed Secret, Vault, or
  an approved equivalent;
- database migrations are backward-compatible and have rollback guidance;
- each deployable unit has health, readiness, metrics, logs, traces, smoke, and
  rollback evidence;
- rollout and rollback evidence is attached to the release notes or evidence
  artifact directory.

## Supply Chain

- Dependencies, images, IaC, and policy definitions are scanned.
- SBOM generation and Cosign signing are GA goals for release artifacts.
- Exceptions require owner, expiry date, compensating control, and tracking in
  `blocker-ledger.md` or the release evidence.

## Rollback Standard

- Prefer service image/config rollback.
- Do not use database restore as the default rollback path.
- Event-backed workflows must document replay, compensation, and reconciliation.
- Compute-control rollback must reconcile reservations, queues, and runtime
  cleanup before traffic is fully restored.

## Merge Blockers

- New direct cross-service repository/store dependency.
- Unversioned internal API or event schema.
- Destructive migration without expand/contract plan.
- Missing owner/runbook/SLO for a deployable unit touched by the PR.
- Security scan or Sonar failure without explicit risk acceptance.
- Live staging evidence missing for milestones that claim deployable-unit
  readiness.
