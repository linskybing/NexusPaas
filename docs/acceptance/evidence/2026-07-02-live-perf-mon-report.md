# Live PERF/MON — Prometheus on kind, Alert Drill, k6 Scenarios, Drift-Job Live Run

> **KIND LOCAL — NOT EXTERNAL GA PROOF.** Per `docs/agents/workflow.md`,
> single-cluster/local evidence must NOT be described as external GA proof.
> Everything below ran on kind cluster `nexuspaas-kind-e2e` against the
> production-beta 8-unit topology built from branch `ac-completion-round`.

- Run: 2026-07-02, ~13:45–14:35 UTC+8

## 1. Prometheus deployment (MON live scrape/retention/rules)

New manifest `backend/deploy/observability/production-beta/prometheus.yaml`:
plain Prometheus v3.4.1 (operator-less path for kind and similar clusters) +
kube-state-metrics v2.13.0 (deployments/jobs collectors only). Scrape uses
pod discovery filtered to the 8 unit labels with an `X-API-Key` header from
`nexuspaas-prometheus-scrape-secret` (the `/metrics` endpoint is admin-gated).
The embedded alert rules are the SAME groups as the PrometheusRule object;
`TestPrometheusAlertRulesParity` fails the build if the two files drift.

Live results on kind:

| item | result |
| --- | --- |
| scrape targets | all 8 units + kube-state-metrics healthy (14/17 pod targets up right after a Prometheus restart; the 3 remainders were stale pre-restart pod IPs) |
| rule groups loaded | 3 groups (`availability`, `latency`, `errors`), 7 alerts |
| retention | `storage.tsdb.retention.time = 1w` (flag snapshot via `/api/v1/status/flags`) |
| cardinality snapshot | 1073 head series; top metric `nexuspaas_http_request_duration_seconds_bucket` = 455 series |

## 2. Alert firing + resolution drill (MON alerting live proof)

- 13:56:41Z — `collaboration-unit` scaled to 0 (fault injection)
- 13:57:10Z — `NexusPaasDeploymentUnavailable{deployment="collaboration-unit"}`
  **firing** (confirmed via `/api/v1/alerts`); `NexusPaasCoreAvailabilityBurn`
  also fired from rolling-restart 5xx noise in the same window
- 14:15:08Z — collaboration-unit restored (rollout complete)
- 14:33Z — `/api/v1/alerts` shows **0 active alerts** (both resolved)

## 3. OPS-019 prometheus scenario (completes the fault-injection matrix)

`SCENARIOS=prometheus backend/scripts/failure-injection-drill.sh` run
`20260702142738`: Prometheus scaled to 0 → all probed units keep serving
`/readyz` and (admin-authenticated) `/metrics` = 200 during the scrape outage
→ Prometheus restored. **PASS** — monitoring outage does not degrade the
product plane. (db / k8s-api / node-agent scenarios passed earlier the same
day, run `20260702133128` — see the OPS resilience drills report.)

## 4. k6 live scenarios (PERF-003/004/006/008)

`backend/scripts/perf/ac-live-scenarios.js`, 30 VUs × 60s per scenario, all
four scenarios concurrently, direct port-forwards to the owning units.
4xx quota/validation rejections count as correct answers; only 5xx/transport
failures and the latency budget fail the run. **k6 exit 0, all thresholds
passed:**

| scenario | AC | target | requests | p95 |
| --- | --- | --- | --- | --- |
| queue_stress | PERF-003 | compute-api `POST/GET /api/v1/jobs` | 17,278 | 2.2 ms |
| usage_query | PERF-004 | usage-observability `/api/v1/me/usage`, `/api/v1/cluster/summary` | 10,738 | 3.2 ms |
| build_load | PERF-006 | platform-io-unit `POST /api/v1/images/build/dockerfile` | 7,440 | 2.8 ms |
| k8s_control | PERF-008 | compute-control-plane `/api/v1/k8s/cluster`, `/api/v1/k8s/nodes` | 17,418 | 2.6 ms |

Total 52,874 requests at 878 rps; `http_req_failed` = 0.284% (< 1% threshold;
failures were port-forward transport resets, no service 5xx burn — the 5xx
alert stayed quiet through the run). Summary JSON:
`.tmp/nexuspaas-perf-ac-live-scenarios.json` (local artifact).

## 5. Drift→replay reconcile job — live kind run (DATA-016/018)

With the fleet's `SERVICE_TRUSTED_IDENTITIES` extended so every unit is a
trusted caller of every other unit (cross-unit read contracts; also fixed in
`kind-live-e2e.sh` for future runs):

- `usage-observability` families (`gpu_usage`, `dashboard`, `cluster`)
  converged to `projection_drift = 0` (gauge scraped by Prometheus)
- **Injected drift**: deleted the `gpu_projects` read-model row out-of-band at
  14:23:23Z → reconciler detected and rebuilt it by 14:23:33Z (≤ one 30s tick):
  `projection drift detected … drift=1` → `projection rebuild finished …
  drift_before=1 drift_after=0`; read-model row verified restored in Postgres
- org-project typed tables also proven live here: `POST /api/v1/projects`
  through tenant-unit wrote `org_projects` row `drift-drill-project` (verified
  by SQL) after migration 0002 applied in-cluster (`applied=1`, job log)

Known limitation (disclosed, tracked): in the isolated 8-unit topology,
read-model families whose *source* resources expose no cross-service read
contract (e.g. request-notification forms, scheduler live-quotas/queues as
dashboard sources; several identity/policy sources for the ide / image-access /
policy families) cannot be drift-measured remotely — a failed source read is
indistinguishable from an empty source, so those families report residual
orphan drift on kind. Co-hosted deployments (SERVICE_NAME=all) measure all
pairs fully. Follow-up: read contracts for those sources or drift-pair gating
by contract availability.
