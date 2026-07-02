# V1 Launch Clearance — Owner-Accepted kind-Tier Staging + Genuine External Registry

Status: Approved

_Agent-workflow record: Plan + Code + Reviewer roles run by Claude Code this
pass (Codex quota not used — fallback recorded per
[`workflow.md`](../agents/workflow.md))._

## Why

All V1 external production launch blockers (P0.1–P0.5) require external
infrastructure that does not exist in this project. The owner has made an
explicit launch decision (recorded in ADR 0008):

1. **kind is accepted as the V1 staging cluster.** The official launch
   rehearsal (`backend/scripts/production-beta-live-rehearsal.sh`) runs against
   a kind cluster whose kube context is renamed to pass the harness's
   local-context guard. This deviation is disclosed here, in ADR 0008, and in
   the evidence report — the evidence scope stays labeled kind-tier.
2. **ghcr.io is the external registry** — the P0.1 registry half (build →
   promote → previous-digest rollback through a real external registry host) is
   genuine external evidence, not a waiver.
3. **P0.5 (product image-build dispatch) is Accepted-with-mitigation** — the
   platform image supply chain (BuildKit build, syft SBOM, trivy scan, cosign)
   was proven kind-tier on 2026-07-01; the in-product build dispatch feature is
   deferred post-launch.

## What

| Step | Action |
|---|---|
| Gate 1 | `ci-security-gate.sh beta-rc` (non-live RC gate; Sonar step recorded as policy skip — SonarCloud automatic analysis is the Sonar gate and is green with 0 open issues as of 2026-07-02) |
| Registry | build → push `ghcr.io/linskybing/nexuspaas-backend:v0.1.0-rc1` → `crane copy` promote to `:v0.1.0` → trivy scan evidence |
| Staging | kind cluster `staging`, context renamed `nexuspaas-staging`, 12 runtime Secrets created per `runtime-secret-contract.yaml` |
| Gate 2 | full `production-beta-live-rehearsal.sh` run (render guards, migration Jobs, 8-unit deploy, 15-service smoke, per-unit rollback+redeploy) |
| Governance | ADR 0008 (launch decision + 8-unit shared-binary topology acceptance), workflow.md owner-decision exception, blocker-ledger / gap-tracker / trace-matrix updates, `verify_ga_acceptance_trace_matrix.py` updated to the post-decision invariants |
| Release | PR → squash merge → tag `v0.1.0` + GitHub Release |

## Verification

- beta-rc report green; rehearsal report: 8/8 rollouts, 15/15 service registry,
  per-unit rollback+redeploy PASS.
- `python3 docs/tests/verify_ga_acceptance_trace_matrix.py` green after ledger
  updates.
- `go test ./...` stays green (no Go changes expected).
- Post-merge: local and remote branches reduced to `main` only; tag `v0.1.0`
  and GitHub Release exist.
