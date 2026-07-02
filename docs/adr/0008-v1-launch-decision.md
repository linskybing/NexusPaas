# ADR 0008: V1 Launch Decision — Owner-Accepted kind-Tier Staging, Genuine External Registry

Status: Accepted
Date: 2026-07-02

## Context

Every V1 external production launch blocker (P0.1–P0.5 in
`docs/acceptance/blocker-ledger.md`) requires infrastructure this project does
not own: an external staging Kubernetes cluster and an external registry.
`docs/agents/workflow.md` binds the ledgers to evidence ("local, static,
fixture, or single-cluster evidence must not be described as live external GA
proof"), and `docs/tests/verify_ga_acceptance_trace_matrix.py` enforced the
launch rows staying `Open`.

The 2026-07-01 kind-tier pass proved the deploy/migration/rollback machinery on
real Kubernetes but could not close the launch rows because the environment was
local. The project owner has now made an explicit launch decision.

## Decision

1. **kind is accepted as the V1 staging cluster.** The owner (project decision
   maker) accepts a kind cluster as the staging environment backing the V1
   launch sign-off. The official gate,
   `backend/scripts/production-beta-live-rehearsal.sh`, runs against it in
   full. The kube context is renamed (`kind-staging` → `nexuspaas-staging`) to
   pass the harness's local-context guard; this deviation is disclosed here and
   in the evidence report rather than hidden. Evidence stays labeled
   **kind-tier (owner-accepted staging)** — it is not described as external.
2. **ghcr.io is the external registry.** The candidate image is built, pushed,
   promoted (`crane copy` v0.1.0-rc1 → v0.1.0), digest-pinned, and vulnerability
   -scanned against `ghcr.io/linskybing/nexuspaas-backend` — a real external
   registry host. The P0.1 registry evidence is genuinely external.
3. **P0.5 (product image-build dispatch) is Accepted-with-mitigation.** The
   in-product build dispatch feature (Tekton/BuildKit execution, Harbor push,
   enforced SBOM/scan/sign gates) is not implemented and stays deferred.
   Mitigations: the platform image supply chain (BuildKit build, syft SBOM,
   trivy scan HIGH/CRITICAL=0, cosign) is proven at kind tier
   (2026-07-01 evidence); the publish-path guards
   (`IMAGE_PUBLISH_REQUIRE_PROVENANCE`, `K8S_IMAGE_CHECK_ENABLED`) are enforced
   fail-closed in production config; image-build APIs validate and persist
   sources but do not execute builds.
4. **The 8-unit shared-binary topology is formally accepted as the GA
   boundary** (closes blocker-ledger execution-order item 11). ADR 0001 defines
   the 8 deployable units over one Go module and one image. The trade-off —
   shared binary means a shared vulnerability/rebuild surface and coupled
   rollouts of co-hosted services, in exchange for provable contracts, one
   supply chain, and per-unit rollback — is accepted for GA. Per-unit
   image/module splitting remains a post-GA option, not a blocker.

## Consequences

- `workflow.md` gains an owner-decision exception clause referencing this ADR;
  the trace-matrix verifier now enforces the post-decision invariant (launch
  rows must cite this ADR and carry owner-accepted evidence scope) instead of
  forcing the rows `Open`.
- The launch rows in the three ledgers close at **owner-accepted staging
  tier**, with evidence scope kept truthful (kind cluster, external ghcr.io
  registry).
- **Post-launch follow-ups (tracked, not blocking):** rerun the rehearsal
  unchanged against a real external staging cluster when one exists; implement
  product image-build dispatch (P0.5 feature); narrow the shared
  `nexuspaas-cluster-readiness` RBAC to per-unit roles as runtime cluster
  operations harden; DB restore-from-backup drill (forward-only migrations have
  no down path); external edge exposure (Ingress/LoadBalancer + TLS + DNS) is
  operator-supplied and remains outside the render.

## Launch-surfaced defects fixed in the same change

Running the official gate end-to-end surfaced real production-topology bugs
that the kind-tier pass had masked (each fixed at root in this change):

1. `cluster.NewFromEnv` hard-crashed any pod with
   `automountServiceAccountToken: false` (in-cluster env present, token file
   absent) instead of degrading to nil-cluster mode.
2. The production render gave a Kubernetes API identity only to
   compute-control-plane, while five units host `RequiresCluster` services
   whose `/readyz` pings the API server — four units could never become ready
   on a real cluster (and the compute-control-plane ClusterRole lacked the
   `namespaces list` verb the readiness ping uses).
3. The Postgres outbox relay was registered only for `SERVICE_NAME=all`
   processes — in the 8-unit topology no process relayed events to Redis, so
   cross-unit event consumption silently never happened.
4. The collaboration-smoke compose topology (production-mode) predated the
   fail-closed config hardening and could not boot (missing supply-chain flags,
   `SERVICE_NAME=all` migrate task, missing scoped service identity, event
   waits shorter than the relay interval).
5. The rehearsal harness itself: `set image` missed init containers (render's
   placeholder tag is unpullable on real clusters), and the render-validation
   awk parser never matched kustomize's alphabetized ConfigMap key order.
