# NexusPaaS Architecture Remediation Roadmap

The current goal is to turn the Go modular monolith into 8 credible deployable
units without doing a big-bang 15-service split. `blocker-ledger.md` is the blocker
ledger; this roadmap is the implementation order.

## P0: Before Production/GA

1. Merge route auth and collision hardening.
   - Centralize internal route auth.
   - Fail startup on undeclared same-shape route collisions.
   - Keep route/OpenAPI/internal-auth tests in CI.

2. Implement reliable transactional events.
   - Write owner state and outbox events in the same database transaction.
   - Add durable relay publishing, inbox dedupe, retry, dead-letter, and publish
     lag evidence.
   - Keep Redis Streams as a provider, not the reliability boundary.

3. Move core data away from generic JSON records.
   - Continue from the identity first slice.
   - Prioritize tenant/project, workload, scheduler/quota, storage bindings,
     registry/build records, and billing-related data if added.
   - Require an ADR for any new core domain stored in `platform_records`.

4. Harden auth and request trust boundaries.
   - Replace API token full-table scans with token ID lookup.
   - Use one trusted proxy-aware client IP resolver everywhere.
   - Add explicit `local`, `test`, `dev`, `staging`, and `production`
     environment profiles.
   - Fail closed in staging/production when the PDP is missing.

## P1: Architecture Maturity

1. Replace Beta service keys with workload identity or scoped rotatable
   per-service credentials.
2. Replace or wrap custom JWT/JWKS verification with a mature Go library.
3. Adopt or harden the migration runner with version, checksum, lock, and dirty
   state tracking.
4. Abstract Longhorn, Harbor, MinIO, Dex, Redis Streams, and k3s behind provider
   contracts only where replacement is a real requirement.
5. Move critical APIs toward typed contracts and generated/fixture-backed
   compatibility tests.
6. Add read-model drift comparison and replay evidence before retiring
   shared-store or owner-read paths.
7. Capture staging deploy, smoke, rollback, and redeploy evidence for each of
   the 8 deployable units.

## P2: Documentation And Tooling

1. Keep README, `blocker-ledger.md`, architecture docs, and backend docs aligned with
   the current implementation.
2. Split the large CI/security script only when a check becomes hard to run or
   debug independently.
3. Consolidate service ownership docs around the 8 deployable units.
4. Add provider ADRs when replacing reference-stack assumptions.
5. Add SBOM generation, image signing, and release provenance after staging
   promotion is stable.

## Acceptance Rule

NexusPaaS can be called a GA microservice architecture only when `blocker-ledger.md`
has no unaccepted P0 blockers, the 8 deployable units have staging evidence, and
core data/event boundaries no longer depend on undocumented shared-store paths.
