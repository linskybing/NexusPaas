# OPS Resilience Drills — Fault Injection (OPS-019) + Service-Identity Rotation

> **KIND LOCAL — NOT EXTERNAL GA PROOF.** Per `docs/agents/workflow.md`,
> single-cluster/local evidence must NOT be described as external GA proof.
> Both drills ran against the kind-deployed production-beta 8-unit topology
> (image built from branch `ac-completion-round`, `PRODUCTION: "true"` units),
> via repeatable one-command harnesses.

- Run: kind cluster `nexuspaas-kind-e2e`, namespace `nexuspaas`, 2026-07-02
- Stack: `kubectl kustomize backend` render, 8 units × 2 replicas + postgres/
  redis/minio, deployed by `backend/scripts/kind-live-e2e.sh KEEP_CLUSTER=1`

## 1. Fault-injection drill (OPS-019) — `backend/scripts/failure-injection-drill.sh`

Run `20260702133128`, all injected faults followed by verified recovery:

| scenario | step | result | detail |
| --- | --- | --- | --- |
| db | baseline-ready | pass | tenant-unit `/readyz` = 200 |
| db | outage-fails-closed | pass | postgres scaled to 0 → `/readyz` = 503 with reason `DATABASE_URL is unavailable: FATAL: terminating connection …` (fail-closed, Kubernetes stops routing) |
| db | recovery | pass | postgres restored → `/readyz` = 200 |
| k8s-api | outage-window | pass | control-plane container paused 45s, apiserver recovered |
| k8s-api | data-plane-survives | pass | container restart counts unchanged across the outage (pods kept serving) |
| k8s-api | recovery | pass | compute-control-plane `/readyz` = 200 |
| node-agent | unit-down | pass | usage-observability scaled to 0, unreachable |
| node-agent | blast-radius-contained | pass | gateway/tenant/compute-api `/readyz` = 200 throughout |
| node-agent | recovery | pass | usage-observability `/readyz` = 200 |
| prometheus | all | skipped | no prometheus deployment yet — deployed and drilled in the Live PERF/MON phase |

## 2. Service-identity dual-key rotation drill — `backend/scripts/service-identity-rotation-drill.sh`

Run `20260702133406`. Probe = `POST /internal/storage/projects/…/build-source-access`
(ServiceAuthRequired contract on platform-io-unit): **401 = identity rejected,
422 = identity accepted** (request reached contract-field validation).

| step | result | detail |
| --- | --- | --- |
| baseline-old-key-accepted | pass | probe=422 |
| baseline-forged-key-rejected | pass | probe=401 |
| window-zero-auth-failures | pass | receivers patched to `{key: NEW, previous_key: OLD}` + rolling restart of all 8 units with continuous OLD-key probe: 7 probes, **0 auth failures**, 0 infra transients |
| window-new-key-accepted / old-still-accepted | pass | both keys valid inside the window (dual-key acceptance) |
| senders-rolled-new-key-live | pass | fleet `SERVICE_IDENTITY_KEY="NEW,OLD"` (active key first) + rolling restart |
| retired-old-key-rejected | pass | receivers drop `previous_key` → OLD key = 401 |
| final-new-key-accepted | pass | probe=422 |

This is the live execution of the ADR 0003 dual-key rotation procedure added
in this branch (`ServiceTrustedIdentity.previous_key` + comma-separated
`SERVICE_IDENTITY_KEY`), demonstrating zero-downtime key rotation and
verified old-key revocation.

## 3. Related evidence

- DB backup/restore drill (destructive round-trip on real Postgres):
  `2026-07-02-db-backup-restore-drill-report.md`
- The unit-level dual-key acceptance/rejection matrix is covered by
  `TestServiceAuthDualKeyRotationWindow` and
  `TestApplyServiceIdentityHeadersPresentsFirstKeyOfRotationPair`.
