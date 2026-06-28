# Provider Coupling Boundary Documentation

## 1. Objective

Document the provider coupling boundary for the next docs-only slice: separate
portable NexusPaaS core contracts from current reference-stack providers without
claiming provider adapters are implemented.

## 2. Background

`problem.md` and `docs/acceptance/ga-acceptance-trace-matrix.md` both keep
provider coupling open because Longhorn, Harbor, MinIO, Dex, Redis Streams, and
k3s remain reference-stack assumptions. The required next step is architecture
documentation that names the portable contracts and the current provider
bindings before any portability implementation is claimed.

## 3. Source References

- `AGENTS.md`
- `docs/agents/planning.md`
- `docs/agents/workflow.md`
- `docs/architecture/service-boundaries.md`
- `docs/roadmap.md`
- `docs/acceptance/ga-acceptance-trace-matrix.md`
- `problem.md`
- Existing ADR style under `docs/adr/`

## 4. Assumptions

- The slice is documentation only.
- The branch remains `storage-data-path`.
- Current reference providers stay valid defaults for local/reference operation.
- The provider coupling gap must remain `Open` or `Started`, not `Done`.
- No provider adapter code, deployment manifests, or runtime configuration are
  changed in this slice.

## 5. Non-Goals

- No production code edits.
- No tests, mocks, adapters, SDK interfaces, or runtime provider abstractions.
- No Kubernetes manifest, Helm, Secret, or environment variable changes.
- No claim of portability, GA readiness, or provider replacement support.
- No change to Harbor external registry promotion, Harbor DR, or live staging
  evidence status.

## 6. Current Behavior

- The architecture docs mention concrete providers in service boundaries.
- The acceptance and problem ledgers list provider coupling as `Open`.
- The portable core contracts for storage, registry, object store, identity,
  event bus, and deployment baseline are not described in one reviewed boundary
  document.

## 7. Target Behavior

- A new ADR documents the provider boundary:
  - Longhorn or other storage backends implement storage/PVC/mount-plan
    contracts.
  - Harbor implements OCI registry/catalog/build governance contracts.
  - MinIO implements S3-compatible object storage contracts.
  - Dex implements OIDC identity provider contracts.
  - Redis Streams implements event transport only; durable reliability remains
    the outbox/inbox contract.
  - k3s remains the development/reference deploy baseline, not the portability
    contract.
- `docs/architecture/service-boundaries.md` points readers to the ADR instead
  of treating provider names as core boundaries.
- `docs/acceptance/ga-acceptance-trace-matrix.md` and `problem.md` point to the
  ADR while keeping provider coupling open/started.

## 8. Affected Domains

- Architecture documentation.
- GA acceptance ledgers.
- Problem/blocker ledger.

## 9. Affected Files

- Add `docs/adr/0007-provider-coupling-boundary.md`.
- Update `docs/architecture/service-boundaries.md`.
- Update `docs/acceptance/ga-acceptance-trace-matrix.md`.
- Update `problem.md`.
- `docs/plan/2026-06-28-provider-coupling-boundary-docs.md`.

## 10. API / Contract Changes

Documentation-only contract clarification. No runtime API, OpenAPI, event
schema, or provider interface changes are implemented.

## 11. Database / Migration Changes

None.

## 12. Configuration Changes

None.

## 13. Observability Changes

None. The ADR may name future adapter health and degraded-state expectations,
but no metric, alert, or dashboard is added.

## 14. Security Considerations

The ADR must preserve trust-boundary wording for identity, object storage,
registry, and event transport. It must not introduce secrets, endpoints, or
credential examples.

## 15. Implementation Steps

1. Add `docs/adr/0007-provider-coupling-boundary.md` in the existing ADR style
   with `Status: Accepted` and `Date: 2026-06-28`. The ADR body must state that
   this decision is boundary documentation only and does not implement provider
   adapters. Keep provider coupling `Open`/`Started` only in `problem.md` and
   the acceptance matrix, not as the ADR status.
2. In the ADR, define the portable core contract for each provider area and the
   current reference implementation:
   - storage backend: Longhorn/reference storage;
   - OCI registry: Harbor;
   - object store: MinIO/S3;
   - identity provider: Dex/OIDC;
   - event bus: Redis Streams as transport behind outbox/inbox reliability;
   - deploy baseline: k3s/dev reference environment.
3. Update `docs/architecture/service-boundaries.md` to link to the ADR and
   reword provider mentions as reference implementations for owned contracts.
4. Update the Provider coupling row in
   `docs/acceptance/ga-acceptance-trace-matrix.md` to include the ADR as
   evidence of started boundary documentation while keeping status `Open`.
5. Update the Provider coupling row in `problem.md` to point at the ADR and
   state that implementation adapters and live portability proof remain open.

## 16. Verification Plan

Acceptance criteria:

- The diff is docs-only.
- The new ADR separates portable core contracts from the six current reference
  providers named in the requirement.
- The acceptance trace matrix Provider coupling row remains `Open`.
- The `problem.md` Provider coupling entry remains open/started in wording and
  does not claim portability is done.
- No code, manifests, migrations, or runtime config are edited.

Verification commands:

```bash
git diff --check
! git diff --name-only | rg -v '^(docs/plan/2026-06-28-provider-coupling-boundary-docs.md|docs/adr/0007-provider-coupling-boundary.md|docs/architecture/service-boundaries.md|docs/acceptance/ga-acceptance-trace-matrix.md|problem.md)$'
rg -n "Provider coupling|0007-provider-coupling-boundary|Longhorn|Harbor|MinIO|Dex|Redis Streams|k3s" docs/adr docs/architecture/service-boundaries.md docs/acceptance/ga-acceptance-trace-matrix.md problem.md
rg -n '^\| Provider coupling \|.*\| Open \|' docs/acceptance/ga-acceptance-trace-matrix.md
```

No backend test/build/Sonar command is required for this docs-only slice.

## 17. Rollback Plan

Revert the new ADR and the three documentation reference updates. No runtime
rollback is needed.

## 18. Risks and Tradeoffs

- The ADR improves reviewability but does not reduce runtime coupling yet.
- Naming contracts now may require later refinement when real provider adapters
  are implemented.
- Keeping the gap open avoids overstating architecture maturity.

## 19. Reviewer Checklist

- The plan is docs-only and scoped to provider boundary documentation.
- The new ADR covers all six required provider areas.
- Ledger updates link to the ADR.
- Provider coupling remains `Open`/`Started`, not `Done`.
- No code, config, deployment, database, or test files are included.
- Verification commands prove docs-only scope and status wording.

## 20. Status

Status: Approved
