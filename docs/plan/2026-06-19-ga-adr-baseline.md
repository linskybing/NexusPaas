# GA ADR Baseline

## 1. Objective

Add the missing Day 0-15 architecture decision records for the 90-day GA architecture roadmap. This slice records accepted decisions for the 8 deployable-unit target, the Outbox/Inbox and read-model migration direction, the GA service identity direction, and deployment evidence gates.

## 2. Background

The connector preflight for this branch read `AGENTS.md`, `docs/roadmap.md`, `problem.md`, the current branch/status, and the latest merged diff on `main`. `docs/roadmap.md` says Day 0-15 must add ADRs for the 8-unit target, Outbox/Inbox/read-model migration, service identity direction, and deployment evidence gates. `problem.md` still tracks GA blockers for staging evidence, data ownership, contract testing, service identity, remote Sonar, and supply chain. The merged GA architecture baseline provides master architecture, service boundary, data migration, testing, CI/CD, open-source quality, and observability docs, but there is no `docs/adr/` directory yet.

## 3. Source References

- `AGENTS.md`
- `docs/agents/workflow.md`
- `docs/agents/planning.md`
- `docs/agents/review-checklist.md`
- `docs/roadmap.md`
- `problem.md`
- `docs/architecture/nexuspaas-master-plan.md`
- `docs/architecture/service-boundaries.md`
- `docs/architecture/data-migration-strategy.md`
- `docs/architecture/testing-strategy.md`
- `docs/architecture/ci-cd-and-pr-governance.md`
- `docs/architecture/observability-strategy.md`
- Microservice architecture skill references for boundaries, contracts, data consistency, service identity, observability, testing, delivery, migration, and review checklists.

## 4. Assumptions

- The roadmap baseline merged to `main` is the current source of truth.
- ADRs should record decisions and guardrails, not duplicate every detail from the master plan.
- This is a documentation-only Day 0-15 slice.
- External `/api/v1` compatibility remains mandatory and no runtime code should change in this slice.
- Live staging evidence, Sonar provisioning, and supply-chain enforcement remain blockers until implemented in later slices.

## 5. Non-Goals

- Do not modify Go code, route registration, database schemas, migrations, Kubernetes manifests, CI workflows, or runtime configuration.
- Do not create contract fixtures, event schemas, Outbox/Inbox tables, read models, or deployment manifests in this slice.
- Do not claim GA readiness or close staging, data ownership, service identity, remote Sonar, or supply-chain blockers.
- Do not introduce new third-party dependencies or new infrastructure.
- Do not touch local metadata, connector auth, tunnel tokens, or secret files.

## 6. Current Behavior

The repository has a reviewed GA architecture baseline and roadmap, but Day 0-15 ADR coverage is absent. Reviewers can read the master plan, boundary map, and migration strategy, but they do not yet have compact ADR files that capture the accepted decisions, rejected alternatives, consequences, follow-up evidence, and rollback implications.

## 7. Target Behavior

The repository should contain an ADR baseline under `docs/adr/` with four narrow records:

1. 8 coarse-grained deployable units instead of a big-bang 15-service split.
2. Outbox/Inbox plus read models as the data-boundary migration direction.
3. Kubernetes workload identity or approved equivalent as the GA service identity direction, with static `SERVICE_API_KEY` only as a Beta fallback.
4. Deployment evidence gates for health, readiness, metrics, service registry, synthetic smoke, rollback, redeploy, and trace/request IDs before any unit is called GA-ready.

`problem.md` should also reflect that ADR coverage is now complete while the implementation and evidence blockers remain open.

## 8. Affected Domains

- Architecture decision documentation.
- GA architecture roadmap blocker tracking.
- Reviewer and PR evidence for future implementation slices.

## 9. Affected Files

- `docs/plan/2026-06-19-ga-adr-baseline.md`
- `docs/adr/0001-ga-8-deployable-units.md`
- `docs/adr/0002-outbox-inbox-read-models.md`
- `docs/adr/0003-ga-service-identity.md`
- `docs/adr/0004-deployment-evidence-gates.md`
- `problem.md`

## 10. API / Contract Changes

No runtime API or external contract changes. The ADRs must state that external `/api/v1` route shapes and response envelopes remain stable during decomposition. Future internal APIs and events must be versioned and covered by provider/consumer tests before runtime changes are made.

## 11. Database / Migration Changes

No database or migration changes. The data migration ADR must reinforce that shared PostgreSQL is temporary migration scaffolding, future migration slices use expand, dual-write/read, backfill, compare, cutover, and contract, and direct cross-unit store access needs an owner, expiry, and retirement gate.

## 12. Configuration Changes

No runtime configuration changes. The service identity ADR may document future workload identity configuration expectations, but this slice must not change manifests, secrets, CI variables, or service URL settings.

## 13. Observability Changes

No runtime observability changes. The deployment evidence ADR should document future evidence requirements for `/healthz`, `/readyz`, `/metrics`, service registry, synthetic smoke, rollback, redeploy, and correlation IDs.

## 14. Security Considerations

The ADRs must keep defense in depth explicit: gateway authentication is not sufficient, owning units enforce authorization, service-to-service calls require authenticated identity, static API keys are a transition fallback, and raw credentials must not be logged, traced, committed, or pasted.

## 15. Implementation Steps

1. Add this plan with `Status: Draft`.
2. Run Reviewer Agent plan review against `docs/agents/review-checklist.md`.
3. After approval, add `docs/adr/` and four ADR files matching the Day 0-15 roadmap bullets.
4. Keep each ADR concise and decision-focused: context, decision, consequences, rejected alternatives, compatibility, follow-up evidence, and rollback or reversal notes.
5. Update `problem.md` GA Architecture Roadmap Update to say ADR coverage is complete while implementation/evidence blockers remain open.
6. Inspect the diff for docs-only scope and no secret/local metadata additions.
7. Run the required gates and record results in this plan's final Reviewer Agent approval.
8. Commit, push, and open a PR with what/why/how/tests/risks/rollback.

## 16. Verification Plan

Required gates:

```sh
git diff --check
go -C backend test ./... -count=1
go -C backend vet ./...
go -C backend build ./...
bash backend/scripts/ci-security-gate.sh quick
```

Additional checks for this docs-only slice:

```sh
git diff --name-only origin/main...HEAD
rg -n "SERVICE_API_KEY|workload identity|Outbox|Inbox|/api/v1|rollback|redeploy" docs/adr problem.md
```

E2E, live Kubernetes, staging evidence capture, remote Sonar, and full security scan are not expected for this docs-only ADR slice. If they are not run, the PR must say so explicitly and keep the existing blockers open.

## 17. Rollback Plan

Revert the ADR files, this plan file, and the `problem.md` update. No production code, database, deployment, or runtime configuration rollback is required.

## 18. Risks and Tradeoffs

- ADRs can become stale if future implementation slices diverge; later PRs must update ADRs when decisions change.
- Documentation does not enforce architecture constraints; contract tests, event fixtures, architecture tests, and staging gates remain required follow-up work.
- Choosing workload identity before service mesh keeps GA scope smaller, but service mesh can still be revisited after workload identity, network policy, and observability are proven insufficient.
- Opening the ADR baseline as a small docs-only PR keeps the Day 0-15 review auditable without mixing in runtime changes.

## 19. Reviewer Checklist

- Requirement fit: covers all four ADRs requested by Day 0-15 roadmap.
- Scope control: docs-only, no runtime mutation.
- Non-goals: implementation, manifests, schemas, CI, and secrets are excluded.
- Architecture: aligns with 8 coarse deployable units and avoids big-bang split.
- API contract: external `/api/v1` compatibility remains explicit.
- Data ownership: Outbox/Inbox/read models and shared-store retirement are explicit.
- Security: service identity direction and secret handling are explicit.
- Observability: deployment evidence requirements are explicit.
- Testing: required local gates are listed.
- Rollback: docs-only revert is realistic.

## 20. Status

Status: Approved

## 21. Reviewer Plan Approval

| Category | Question | Result |
| --- | --- | --- |
| Requirement Fit | Does the plan directly satisfy the Day 0-15 ADR requirement? | Pass |
| Scope Control | Is the slice small enough for review? | Pass |
| Non-Goals | Are runtime changes and unrelated refactors excluded? | Pass |
| Architecture | Is the plan aligned with the 8-unit GA baseline? | Pass |
| Microservice Boundary | Are business capability, ownership, deployment, rollback, and observability considered? | Pass |
| API Contract | Does the plan preserve external `/api/v1` compatibility? | Pass |
| Data Ownership | Are shared-store retirement and Outbox/Inbox/read models explicit? | Pass |
| Config | Are runtime config changes excluded? | Pass |
| Observability | Are deployment evidence signals covered? | Pass |
| Security | Are service identity and secret handling covered? | Pass |
| Testing | Are required gates concrete? | Pass |
| Rollback | Is rollback realistic for a docs-only change? | Pass |
| Simplicity | Does the plan avoid new infrastructure and speculative abstractions? | Pass |
| Surgical Change | Are affected files limited to plan, ADRs, and `problem.md`? | Pass |

Status: Approved

## 22. Reviewer Final Approval

### Requirement Traceability

| Requirement | Evidence in Plan | Evidence in Diff | Status |
| --- | --- | --- | --- |
| Add ADR for 8-unit target | Sections 7, 15 | `docs/adr/0001-ga-8-deployable-units.md` | Pass |
| Add ADR for Outbox/Inbox and read models | Sections 7, 11, 15 | `docs/adr/0002-outbox-inbox-read-models.md` | Pass |
| Add ADR for service identity direction | Sections 7, 14, 15 | `docs/adr/0003-ga-service-identity.md` | Pass |
| Add ADR for deployment evidence gates | Sections 7, 13, 15 | `docs/adr/0004-deployment-evidence-gates.md` | Pass |
| Update blocker tracking | Sections 7, 15 | `problem.md` GA Architecture Roadmap Update | Pass |
| Preserve external `/api/v1` compatibility | Sections 10, 19 | ADR compatibility sections state no external API changes | Pass |

### SOLID Compliance

| Principle | Actual Evidence | Risk / Gap | Status |
| --- | --- | --- | --- |
| Single Responsibility | Each ADR covers one decision; `problem.md` only tracks blocker state. | None for docs-only scope. | Pass |
| Open/Closed | Runtime code and stable APIs are unchanged. | Future implementation must add tests before behavior changes. | Pass |
| Liskov Substitution | No runtime interfaces changed. | Not applicable beyond preserving compatibility requirements. | Pass |
| Interface Segregation | ADRs require versioned owner-read, command, and event contracts instead of generic shared-store access. | Contract fixtures remain future work. | Pass |
| Dependency Inversion | ADRs keep high-level ownership and migration policy independent of storage or mesh implementation details. | Concrete workload identity choice remains future work. | Pass |

### 12-Factor App Compliance

| Factor | Actual Evidence | Risk / Gap | Status |
| --- | --- | --- | --- |
| Codebase | The 8-unit ADR preserves one codebase until runtime evidence justifies more splits. | Physical extraction remains future work. | Pass |
| Dependencies | No dependencies changed. | None. | Pass |
| Config | Runtime config is unchanged; ADRs require future externalized identity/evidence config. | Staging config remains future work. | Pass |
| Backing Services | ADRs treat database, event bus, and identity providers as attached services. | Concrete Outbox/Inbox infra remains future work. | Pass |
| Build, Release, Run | Deployment evidence ADR requires candidate version/config evidence. | Live staging evidence remains open. | Pass |
| Processes | No runtime process changes. | Future units still need runtime proof. | Pass |
| Port Binding | No port behavior changed. | Not applicable to docs-only scope. | Pass |
| Concurrency | No scaling behavior changed. | Future deployable-unit scaling evidence remains open. | Pass |
| Disposability | ADRs require rollback, redeploy, and reconciliation evidence. | Evidence capture remains future work. | Pass |
| Dev/Prod Parity | ADRs keep staging evidence as a blocker before GA readiness. | Live staging environment remains required. | Pass |
| Logs | ADRs require safe service identity labels and correlation IDs. | Runtime log changes remain future work. | Pass |
| Admin Processes | No admin process changed. | Future evidence/replay jobs need one-off run guidance. | Pass |

### Verification Results

| Command | Purpose | Result | Notes |
| --- | --- | --- | --- |
| `git diff --check` | Required whitespace gate | Pass | No output. |
| `git diff --cached --check` | Include newly added ADR files in whitespace check | Pass | No output. |
| `go -C backend test ./... -count=1` | Required full backend test gate | Pass | All packages passed. |
| `go -C backend vet ./...` | Required static analysis gate | Pass | No output. |
| `go -C backend build ./...` | Required build gate | Pass | No output. |
| `bash backend/scripts/ci-security-gate.sh quick` | Required quick CI/security gate | Pass | Go 1.25.11, gofmt, vet, tests, and build passed. |
| `rg -n "SERVICE_API_KEY|workload identity|Outbox|Inbox|/api/v1|rollback|redeploy" docs/adr problem.md` | Confirm ADRs cover roadmap keywords | Pass | Expected matches found. |
| E2E / live Kubernetes / staging evidence | Not required for docs-only ADR slice | Not Run | Blockers remain open in `problem.md`. |
| Remote Sonar / full security scan | Not required for docs-only ADR slice | Not Run | Remote Sonar and supply-chain blockers remain open. |

### Required Fixes

| Priority | Issue | Required Fix | Blocking |
| --- | --- | --- | --- |
| None | No implementation issue found in this docs-only slice. | None. | No |

Status: Approved
