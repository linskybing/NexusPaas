# DATA Layer — Drift→Replay Job, Org-Project Typed Ownership, Live Authz Sweep

> **LOCAL-TIER — NOT EXTERNAL GA PROOF.** Per `docs/agents/workflow.md`, local
> evidence must not be described as external GA proof. All items below ran on
> the local backing stack (real Postgres 16 / Redis 7 / MinIO) or in-process;
> the kind-tier reconcile-job run is recorded separately once executed.

- Run: local, 2026-07-02, branch `ac-completion-round`

## 1. Drift→replay reconcile job (DATA-016/DATA-018 live-job gap)

`platform.RegisterProjectionReconciler` registers a lease-gated maintenance
task per read-model family: catch-up replay from the checkpoint → drift
measurement (missing + orphan + stale) → `projection_drift` gauge → on residual
drift: `ProjectionDriftDetected` event, consumer reset, full event-stream
replay, re-measurement, `ProjectionRebuilt` event.

Wired families (7): gpuusage, dashboard, clusterread, image-registry access,
request-notification project-access, ide-service, authorization-policy
(identity + policy-data consumers, reset together because one drift report
spans both). `storage-service` keeps drift checks only — its reads merge
co-hosted source tables directly and there is no local projection consumer to
rebuild (recorded as such, not a gap in the job).

Behavioral proof (`TestProjectionReconcilerRepairsInjectedDrift`): read model
built by catch-up → row deleted out-of-band (drift a plain catch-up cannot
repair, because the consumer already marked the event applied) → next tick
detects drift=1, publishes the drift event, resets the consumer, replays,
rebuilds the row, publishes `ProjectionRebuilt{drift_before:1, drift_after:0}`.
Clean ticks publish nothing.

## 2. Org-project typed ownership (migration 0002)

`migrations/org-project-service/0002_org_project_typed.sql` routes the two
authz-critical aggregates into typed service-owned tables with promoted,
indexed columns and full-payload JSONB (identity-0002 / image-registry-0002
pattern, expand + dual-write; legacy `platform_records` rows retained for
rollback):

- `org_projects` (project_name, owner_id, created_by)
- `org_project_members` (project_id, user_id, role; role not-blank constraint)

Applied against the real local Postgres: `applied=1` then re-run `applied=0`
(idempotent); `validate-migrations` passes at 21 additive files. Remaining
org-project resources (groups, user_groups, user_quotas, gpu_claims) stay on
`platform_records` as recorded, approved debt.

## 3. Live typed-API authz sweep (all 66 fixture families)

`TestLiveTypedAPIAuthzAcrossFixtureFamilies` (`backend/internal/e2e`) starts a
live authz-on stack (SERVICE_NAME=all, RequireAuth, real Postgres/Redis) and
replays every `api/v1` contract fixture family:

- 62 auth-required families × 3 probes each: missing credentials → 401,
  forged bearer → 401, forged API key → 401 — all fail closed with the
  platform error envelope (success=false + request_id present)
- 4 public families are reachable without credentials (a login endpoint's own
  wrong-credential 401 is distinguished from an authn-gate denial)

Result: `--- PASS` — `live authz sweep: 66 fixture families (62 auth-required
fail-closed ×3 probes, 4 public unblocked)`.
