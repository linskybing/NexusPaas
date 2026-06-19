# NexusPaas Roadmap

## 90-Day GA Architecture Roadmap

The current long-term priority is GA architecture decomposition into 8
coarse-grained deployable units. The roadmap intentionally avoids a big-bang
15-service split.

## Day 0-15: Architecture Baseline

- Finish and review the master architecture plan, service boundaries, package
  strategy, data migration strategy, testing strategy, CI/CD governance, and
  open-source quality standard.
- Add ADRs for the 8-unit target, Outbox/Inbox/read-model migration, service
  identity direction, and deployment evidence gates.
- Update `problem.md` so GA blockers are explicit and owned.
- Keep implementation docs-only until the architecture baseline is reviewed.

## Day 16-35: Contracts And Events

- Establish versioned internal contract fixtures for owner-read and command
  APIs.
- Add event schema fixtures and producer/consumer contract tests for the core
  event envelope.
- Add Outbox/Inbox infrastructure where missing, including idempotency,
  retry/dead-letter visibility, and lag metrics.
- Start with identity, tenant, workload, scheduler, and audit events.

## Day 36-55: Data Boundary Migration

- Replace high-risk shared-store reads with owner-read APIs or read models.
- Prioritize scheduler/workload/org-project/identity/authz dependencies.
- Add drift comparison for read models before cutover.
- Block new unregistered cross-service store dependencies through architecture
  tests.

## Day 56-75: Deployable Unit Readiness

- Add staging runtime configuration for the 8 deployable units.
- Capture health, readiness, metrics, service registry, synthetic smoke,
  rollback, and redeploy evidence per unit.
- Replace static service keys in the staging GA path with Kubernetes workload
  identity or an approved equivalent where feasible.
- Make remote Sonar Quality Gate required when GitHub credentials are
  configured.

## Day 76-90: Compute Saga Stabilization

- Harden job submit as a saga: validate, reserve quota, resolve image/storage,
  create Kubernetes work, commit or release quota.
- Verify duration limit, Deployment cleanup, plan-window eviction, preemption,
  idempotency, and compensation across compute-api and compute-control-plane.
- Run Docker collaboration smoke, focused E2E, opt-in live Kubernetes E2E, and
  live staging rehearsal.
- Declare GA architecture candidate only if `problem.md` has no unaccepted
  blockers.

## Beyond 90 Days

- Evaluate whether any of the 8 units should split further based on real
  ownership, scale, release cadence, and failure isolation data.
- Add progressive delivery after staging signals are reliable.
- Complete SBOM generation, image signing, and release provenance.
- Consider service mesh only when workload identity, network policy, and
  current observability are insufficient for a concrete production need.
