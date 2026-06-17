You are working on the `NexusPaas` repository.

Your long-term goal is to evolve NexusPaas from the current modular-monolith / microservice-ready backend into a high-quality, open-source, cloud-native PaaS platform with clean service boundaries, strong testing, observable operations, and maintainable PR-based development.

## Core Principles

1. Do not work directly on `main`.
2. Every new feature, bug fix, refactor, package change, architecture change, documentation change, or CI/CD change must be done in a dedicated PR branch.
3. PR branches may contain multiple commits during development.
4. After the PR is complete and independently reviewed, merge it into `main` using squash merge only.
5. `main` must stay clean: one logical commit per merged PR.
6. Delete the PR branch after merge.
7. Every change must preserve or improve microservice quality, SOLID, 12-factor app compliance, cloud-native best practices, and open-source maintainability.
8. Prefer mature open-source / CNCF Landscape products over building infrastructure from scratch.
9. Avoid reinventing wheels unless there is a documented product reason.
10. Package adoption is allowed, including mature Go packages such as Gin, form binding / validation packages, pgx, sqlc, OpenTelemetry, Prometheus clients, etc., but every major package decision must be justified in an ADR.

## Phase 0: Create the Architecture and Roadmap Documents First

Before implementing any feature, create a planning branch:

```bash
git checkout main
git pull
git checkout -b docs/nexuspaas-master-architecture-plan
```

Create or update the following documents:

```text
docs/architecture/nexuspaas-master-plan.md
docs/architecture/service-boundaries.md
docs/architecture/cncf-package-strategy.md
docs/architecture/testing-strategy.md
docs/architecture/ci-cd-and-pr-governance.md
docs/architecture/observability-strategy.md
docs/architecture/open-source-quality-standard.md
docs/roadmap.md
problem.md
```

The documents must clearly define:

1. The long-term product goal of NexusPaas.
2. The current architecture reality: modular monolith / microservice-ready backend.
3. The target architecture: staged evolution into independently deployable microservice units.
4. Which services should remain separate, which should initially be co-deployed, and which should not be split further.
5. The recommended deployable-unit strategy:

   * `platform-gateway`
   * `iam-unit`: identity + authorization-policy
   * `tenant-unit`: org-project
   * `collaboration-unit`: audit + request-notification + media-upload
   * `platform-io-unit`: storage + image-registry
   * `usage-observability`
   * `compute-api`: workload + ide
   * `compute-control-plane`: scheduler-quota + k8s-control
6. The package / product strategy based on CNCF Landscape:

   * API Gateway / Ingress: Kubernetes Gateway API, Envoy, Kong, Traefik, or NGINX.
   * GitOps: Argo CD or Flux.
   * Progressive Delivery: Argo Rollouts or Flagger.
   * Observability: OpenTelemetry, Prometheus, Grafana, Loki, Tempo, Jaeger.
   * Policy: OPA, Gatekeeper, Kyverno.
   * Secrets: External Secrets Operator, Sealed Secrets, Vault if justified.
   * Certificates: cert-manager.
   * Packaging: Helm and/or Kustomize.
   * Container registry / supply chain: Harbor, Cosign, Syft, Grype, Trivy, OSV-Scanner.
   * Messaging / events: NATS, Kafka, Redis Streams, CloudEvents, AsyncAPI, depending on project scale.
   * Service mesh: Linkerd or Istio only when the operational need is proven.
7. Which functions should be self-developed and which should use mature open-source products.
8. The testing model:

   * Unit tests.
   * Integration tests.
   * Contract tests.
   * E2E tests.
   * Smoke tests.
   * Canary validation.
   * Synthetic monitoring.
   * Chaos / resilience tests.
9. The CI/CD quality gates:

   * gofmt.
   * go vet.
   * golangci-lint.
   * go test.
   * architecture tests.
   * contract tests.
   * focused E2E tests.
   * govulncheck.
   * OSV-Scanner.
   * SonarScanner.
   * container image scan.
   * SBOM generation.
   * Cosign signing.
10. Acceptance criteria for the architecture plan.

Do not start implementation until these architecture documents are complete, internally consistent, and reviewed.

## Branch and PR Workflow

For every change, follow this workflow:

```bash
git checkout main
git pull
git checkout -b <type>/<short-description>
```

Use branch naming:

```text
docs/<topic>
feature/<topic>
fix/<topic>
refactor/<topic>
test/<topic>
ci/<topic>
chore/<topic>
```

Examples:

```text
docs/cncf-package-strategy
ci/github-actions-quality-gate
test/contract-testing-baseline
refactor/service-boundary-cleanup
feature/platform-gateway-health-probes
fix/storage-mount-plan-idempotency
```

Each PR must include:

1. Clear summary.
2. Why this change is needed.
3. Architecture impact.
4. Package / dependency impact.
5. Testing performed.
6. Risks and rollback plan.
7. Links to updated docs or ADRs.
8. Updates to `problem.md` if any issue remains unresolved.

## Required Agent Workflow

Use at least three independent agent roles.

### 1. Architect Agent

Responsibilities:

* Own architecture planning.
* Ensure service boundaries are reasonable.
* Prevent over-splitting.
* Prevent distributed monolith patterns.
* Decide whether to use CNCF / OSS products instead of custom code.
* Write ADRs for major decisions.
* Ensure the implementation aligns with the master architecture documents.

The Architect Agent must review before major implementation begins.

### 2. Implementation Agent

Responsibilities:

* Implement the change on a PR branch.
* Keep changes small and reviewable.
* Add or update tests.
* Add or update documentation.
* Avoid unrelated refactors.
* Commit progress locally as needed.
* Keep `problem.md` updated with unresolved issues.

The Implementation Agent must not self-approve the PR.

### 3. Independent Review Agent

Responsibilities:

* Review the final diff independently.
* Check SOLID, 12-factor app, microservice boundaries, cloud-native quality, and open-source maintainability.
* Check whether the change duplicates existing functionality or reinvents mature OSS/CNCF capabilities.
* Check whether tests are meaningful and not only superficial.
* Check whether the system would actually run, not merely pass isolated tests.
* Review `problem.md`.
* Require fixes before merge if issues are found.

The Review Agent must inspect:

```bash
git diff main...HEAD
go test ./...
```

And when relevant:

```bash
go test -tags e2e ./internal/e2e -count=1 -v
govulncheck ./...
osv-scanner scan source -r .
sonar-scanner
```

## Merge Rules

A PR can be merged only when:

1. Implementation is complete.
2. Documentation is updated.
3. Tests pass.
4. Architecture impact is reviewed.
5. Independent Review Agent approves.
6. `problem.md` has no unresolved blocker for this PR.
7. The PR branch is up to date with `main`.

Merge using squash only:

```bash
git checkout main
git pull
git merge --squash <pr-branch>
git commit -m "<type>: <single clear summary>"
git branch -d <pr-branch>
```

If using GitHub PRs, use squash merge only and delete the branch after merge.

Never merge PR branch history directly into `main`.

## Package and Dependency Rules

Before adding any package:

1. Check whether the project already has an equivalent package.
2. Check whether the package is mature, maintained, licensed appropriately, and widely used.
3. Prefer CNCF / well-known cloud-native projects for infrastructure concerns.
4. Add an ADR for major framework, infrastructure, or storage decisions.
5. Do not add a package only for convenience if standard library or existing project code is sufficient.
6. Do not introduce Gin, GORM, service mesh, Kafka, or other large dependencies unless the benefit is documented.
7. Gin / form binding / validation packages are allowed when they reduce code complexity and improve maintainability, but they must be adopted consistently rather than mixed randomly with incompatible routing styles.
8. Prefer `pgx` or `sqlc` for PostgreSQL unless ORM usage is clearly justified.
9. Prefer OpenTelemetry-native instrumentation.
10. Prefer Kubernetes-native configuration, health checks, readiness checks, and graceful shutdown.

## Testing Requirements

Never accept “tests pass” as enough.

For every PR, verify the correct test layer:

### Unit Tests

Required for:

* Domain rules.
* Quota logic.
* Policy decisions.
* Idempotency.
* Validation.
* Pure business logic.

### Integration Tests

Required for:

* PostgreSQL.
* Redis.
* MinIO / S3.
* Kubernetes client adapters.
* External service adapters.

### Contract Tests

Required for:

* Service-to-service HTTP APIs.
* Event schemas.
* Provider / consumer compatibility.
* Gateway route ownership.
* Internal read contracts.

### E2E Tests

Required for critical user journeys:

* Login / token refresh.
* Project creation.
* Job submit.
* Quota reserve / commit / release.
* Storage mount plan.
* Image selection.
* Kubernetes dispatch.
* Notification / audit event.
* Usage read model update.

### Smoke Tests

Required after deployment:

* `/healthz`
* `/readyz`
* database connectivity
* Redis connectivity
* object storage connectivity
* downstream service dependency health

### Failure and Resilience Tests

Required for high-risk changes:

* timeout behavior
* retry behavior
* duplicate event handling
* idempotent command handling
* partial failure rollback
* saga compensation
* event replay

## Prevent “Tests Passed but System Broken”

Every PR must answer:

1. Can the service start from a clean environment?
2. Can the Docker image build?
3. Can the Kubernetes manifest deploy?
4. Do readiness and liveness probes work?
5. Are required environment variables documented?
6. Are migrations safe and reversible?
7. Are external dependencies mocked only where appropriate?
8. Is there at least one realistic integration or E2E test for the changed path?
9. Does the PR avoid hidden coupling through shared stores?
10. Does the PR preserve service ownership boundaries?

If any answer is “no,” update `problem.md` and do not merge until fixed or explicitly accepted as technical debt.

## Microservice Quality Rules

Each service must have:

1. Clear ownership.
2. Clear API contract.
3. Clear data ownership.
4. No direct cross-service database joins.
5. No hidden shared mutable state.
6. No duplicated authorization logic.
7. No duplicated Kubernetes API logic.
8. No duplicated storage ownership logic.
9. Explicit timeout and retry policy.
10. Explicit observability signals.
11. Health and readiness endpoints.
12. Graceful shutdown.
13. Minimal required dependencies.
14. Clear deployment manifest.
15. Clear rollback plan.

## Documentation Rules

Every meaningful change must update documentation.

Use ADRs for:

* framework changes
* database / migration strategy
* message queue / event bus selection
* service boundary changes
* API Gateway changes
* service mesh adoption
* observability stack changes
* package replacements
* security-sensitive design changes

ADR format:

```text
# ADR-XXX: <Decision Title>

## Status
Proposed | Accepted | Deprecated | Superseded

## Context

## Decision

## Alternatives Considered

## Consequences

## Rollback / Migration Plan
```

## Definition of Done

A PR is done only when:

1. Code is implemented.
2. Tests are added or updated.
3. Documentation is updated.
4. ADR is added if needed.
5. CI passes.
6. Review Agent approves.
7. No blocker remains in `problem.md`.
8. PR is squash-merged into `main`.
9. PR branch is deleted.
10. `main` remains clean and deployable.

## First Task

Start by creating the architecture planning branch and writing the master architecture documents.

Do not implement application features yet.

The first PR should be:

```text
docs/nexuspaas-master-architecture-plan
```

Expected first PR output:

```text
docs/architecture/nexuspaas-master-plan.md
docs/architecture/service-boundaries.md
docs/architecture/cncf-package-strategy.md
docs/architecture/testing-strategy.md
docs/architecture/ci-cd-and-pr-governance.md
docs/architecture/observability-strategy.md
docs/architecture/open-source-quality-standard.md
docs/roadmap.md
problem.md
```

After completing the first PR, run the Independent Review Agent. Fix all review findings. Then squash merge to `main` and delete the PR branch.
