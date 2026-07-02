# V1 Launch Rehearsal — Official `production-beta-live-rehearsal.sh` Run (2026-07-02)

**Tier disclosure (ADR 0008):** staging cluster = **kind (owner-accepted)** —
kube context renamed `kind-staging` → `nexuspaas-staging` to pass the harness's
local-context guard, disclosed here and in
[ADR 0008](../../adr/0008-v1-launch-decision.md). Registry evidence is
**genuinely external** (ghcr.io). This report must not be cited as
external-staging proof; the external-cluster rerun is a tracked post-launch
follow-up.

## Gate 1 — non-live `beta-rc` gate (same day, exit 0)

`backend/scripts/ci-security-gate.sh beta-rc` green end-to-end: quick gate
(gofmt/vet/test/build), production-beta manifest dry-run + rollback-plan
rehearsal, Docker-backed migrations + integration coverage 82.8% (threshold
80), focused + full non-live E2E, 15-service routing smoke, **8-unit
production-mode collaboration smoke** (first green run since the fail-closed
hardening), security gate (govulncheck, OSV, image build, trivy
HIGH/CRITICAL=0). Sonar step recorded as policy skip — the Sonar gate for this
repo is SonarCloud automatic analysis, green with **0 open issues** and Quality
Gate OK as of 2026-07-02.

## Gate 2 run parameters

| Item | Value |
| --- | --- |
| Harness | `backend/scripts/production-beta-live-rehearsal.sh`, exit 0 |
| Kube context | `nexuspaas-staging` (kind v0.32.0, k8s v1.36.1, owner-accepted staging) |
| Namespace | `nexuspaas` |
| Candidate image | `ghcr.io/linskybing/nexuspaas-backend@sha256:d94ca9471c5637b33bea8352216501662061b012edcea615834b4fd11cab7125` |
| Baseline (previous) image | `ghcr.io/linskybing/nexuspaas-backend:v0.1.0-rc1@sha256:d94ca947…` (distinct ref → real per-unit rollouts) |
| Promotion | `crane copy` `v0.1.0-rc1` → `v0.1.0` on ghcr.io (external registry host) |
| Registry scan | trivy 0.71.1, HIGH/CRITICAL = 0, `--exit-code 1` clean |
| Smoke auth | production mode (`REQUIRE_AUTH=true`), API key + scoped service identity |

## Stage results (all pass, fail-closed harness)

1. **Render validation** — `kubectl kustomize backend`: exactly 8 unit
   Deployments, per-unit `SERVICE_NAME` correct, no `SERVICE_NAME=all`, no
   all-in-one Deployment, no dev/placeholder secret refs, client dry-run clean.
2. **Registry promotion** — crane copy to `ghcr.io/linskybing/nexuspaas-backend:v0.1.0`, digest-pinned candidate.
3. **Secret readiness** — all 12 required Secret objects present in the live
   cluster (names only, values never recorded): postgres-password,
   dex-password, minio-credentials, coturn-runtime-secret + 8 per-unit
   runtime secrets.
4. **Previous images recorded** — per-unit rollback targets captured before any
   mutation.
5. **DB migration Jobs** — `apply-migrations` → `validate-migrations` as
   in-cluster Jobs against the live staging Postgres: both `complete`.
6. **8-unit candidate rollout** — all 8 units rolled out to the digest-pinned
   candidate (init containers included).
7. **Smoke (candidate)** — `/healthz` + `/readyz` + `/metrics` per unit = 200;
   gateway `/openapi.json` = 200; **15-of-15 logical-service registry union**
   across the 8 units.
8. **Per-unit rollback + redeploy** — for each of the 8 units: rollback to the
   recorded previous image → rollout complete → re-smoke; redeploy candidate →
   rollout complete → re-smoke. 16/16 transitions `complete` (real image-ref
   changes, verified distinct baseline/candidate refs).
9. **Smoke (candidate-after-redeploy)** — repeated full smoke incl. 15-of-15
   registry union.

## Launch-surfaced defects (fixed in this change, see ADR 0008 §"defects")

This was the first end-to-end execution of the official harness; it surfaced
and forced root-cause fixes for: `cluster.NewFromEnv` crash under
`automountServiceAccountToken: false`; missing ServiceAccounts/readiness RBAC
for 4 of 5 cluster-requiring units (+ missing `namespaces list` verb);
the Postgres outbox relay never registering in isolated units; the
collaboration-smoke compose topology predating fail-closed config; `set image`
missing init containers; and the render-validation parser never matching
kustomize's key order. Each fix is covered by updated guard tests.

## Reproduction

Staging bootstrap (namespace, 12 generated Secrets, ghcr pull secret on all
unit ServiceAccounts, render apply, baseline images, initial migration Job),
then:

```sh
LIVE_STAGING_REHEARSAL=1 KUBE_CONTEXT=nexuspaas-staging NAMESPACE=nexuspaas \
CANDIDATE_IMAGE=ghcr.io/linskybing/nexuspaas-backend@sha256:<digest> \
SOURCE_IMAGE=ghcr.io/linskybing/nexuspaas-backend:v0.1.0-rc1 \
PROMOTED_IMAGE_TAG=ghcr.io/linskybing/nexuspaas-backend:v0.1.0 \
REGISTRY_SCAN_STATUS="Success (trivy, HIGH/CRITICAL=0)" \
REGISTRY_SCAN_EVIDENCE=<trivy-report-path> \
SMOKE_API_KEY=<api-key> SMOKE_SERVICE_KEY=<identity-key> \
bash backend/scripts/production-beta-live-rehearsal.sh
```
