# Redis Event Broker Outage Evidence

Status: Approved

## 1. Objective

Capture live OPS-011 evidence that a temporary Redis/event-broker outage does
not silently lose a committed NexusPaaS event.

This is a narrow evidence slice: prove one representative API mutation commits
to the Postgres outbox while Redis is unavailable, then publishes to Redis
Streams after Redis is restored. Full OPS-019 remains open.

## 2. Background

The platform already uses Postgres as the durable event outbox when
`DATABASE_URL` is configured. `EVENT_BUS_URL=redis://redis:6379/1` is only the
relay sink. `PostgresEventBus.Publish` inserts into `platform_event_outbox`;
`PostgresEventBus.RelayPending` later publishes pending rows to `RedisEventBus`.

Redis is also used for worker leases and rate limiting. The rate limiter fails
open on Redis errors, and maintenance leases fail closed, so a short Redis outage
should stop relay attempts without blocking ordinary API writes that only need
Postgres.

## 3. Source References

- `docs/acceptance/operations.md`
- `docs/acceptance/gap-analysis.md`
- `gap.md`
- `problem.md`
- `backend/internal/platform/runtime.go`
- `backend/internal/platform/events_postgres.go`
- `backend/internal/platform/events_redis.go`
- `backend/internal/platform/maintenance.go`
- `backend/internal/platform/lease_redis.go`
- `backend/internal/platform/ratelimit_redis.go`
- `backend/internal/services/storage/handler.go`
- `backend/deploy/k3s/redis.yaml`

## 4. Assumptions

- Namespace `nexuspaas` is the current live namespace.
- `deploy/redis` has one replica and uses `redis:7-alpine` with AOF on PVC
  `redis-data`.
- `production-beta-runtime-config` provides `REDIS_URL=redis://redis:6379/0`
  and `EVENT_BUS_URL=redis://redis:6379/1`.
- `platform-gateway` and `storage-service` use Postgres for records/outbox and
  Redis only for leases/rate/event relay.
- The current live namespace uses 15 first-party deployments, so the storage
  owner pod is `storage-service`. In the 8-unit topology the equivalent owner
  would be `platform-io-unit`, but that is not the current live target.
- API probing can use an existing static API key in local shell variables, but
  no key, decoded value, hash, auth header, Docker config, or credential may be
  printed.

## 5. Non-Goals

- No product code changes.
- No Redis data deletion, PVC deletion, Helm/chart changes, or broad manifest
  reapply.
- No claim that DB, K8s API, Harbor, Prometheus, or node usage-agent failure is
  covered.
- No claim that every event-producing route is covered.
- No long Redis outage and no deliberate dead-letter test.

## 6. Current Behavior

Read-only preflight shows:

- `deploy/redis` is `1/1`;
- `svc/redis` exposes port `6379`;
- `production-beta-runtime-config` contains:
  - `REDIS_URL=redis://redis:6379/0`;
  - `EVENT_BUS_URL=redis://redis:6379/1`;
- all 15 first-party API deployments consume `production-beta-runtime-config`;
- `platform-gateway` and `storage-service` are `1/1`.

Prior outbox evidence already proved normal publish-lag drain and controlled
relay lease interruption. It did not stop the actual Redis deployment.

## 7. Target Behavior

- Redis is scaled from its original replica count to `0` for a short bounded
  outage.
- A storage API mutation sent directly to a selected `storage-service` pod
  succeeds while Redis is unavailable.
- The mutation produces an exact `GroupStorageCreated` outbox row with the
  synthetic trace id, proving the committed event is durable in Postgres.
- While Redis is down, the exact Postgres outbox row remains `pending` or
  `retry`, not silently dropped. Redis DB1 is not queried during outage because
  Redis is intentionally unavailable.
- Redis is restored to its original replica count and becomes ready.
- A relay-capable API pod restart triggers a startup maintenance pass; the exact
  event reaches `relay_status='published'` and appears in Redis Stream `events`
  in DB1.
- If the relay lease `lease:maintenance:event-outbox-relay` survives Redis
  restart with a positive TTL and blocks immediate startup relay, record the TTL,
  wait for natural expiry in a bounded window, then restart one relay-capable API
  pod again. Do not delete or overwrite the lease for this drill.
- Exact synthetic platform records and exact synthetic Postgres outbox rows are
  cleaned up after Redis evidence is captured. Redis stream entries remain as
  append-only evidence.

## 8. Affected Domains

- Platform eventing / outbox relay
- Redis backing service / event bus sink
- Storage-service representative mutation
- Live RKE2 operations in namespace `nexuspaas`
- GA acceptance ledgers

No service boundary changes are proposed.

## 9. Affected Files

- `docs/plan/2026-06-21-redis-event-broker-outage-evidence.md`
- `gap.md`
- `problem.md`
- `docs/acceptance/gap-analysis.md`

No source code or Kubernetes manifest changes are in scope.

## 10. API / Contract Changes

None.

The drill uses existing route:

- `POST /api/v1/storage/{groupId}/storage`

Expected status:

- group storage create: `201`

## 11. Database / Migration Changes

No migration or schema change.

The drill reads:

- `platform_event_outbox`
- `platform_records`

Cleanup deletes by exact synthetic ids only:

- `platform_records` row for resource `storage-service:group_storage` and exact
  id `<groupId>:<pvcId>`;
- `platform_event_outbox` row for the exact collected `GroupStorageCreated`
  event id.

Do not delete broad trace prefixes, checkpoints, inbox rows, or unrelated
pending rows.

## 12. Configuration Changes

None.

Temporary live change:

- scale `deploy/redis` in `nexuspaas` from its original replica count to `0`;
- restore it to the original replica count before ledger updates.

## 13. Observability Changes

No persistent observability change.

Evidence to capture:

- Redis deployment/service/PVC shape and original replica count;
- `storage-service` pod selected for direct pod port-forward;
- API response status while Redis is unavailable;
- exact outbox row state while Redis is unavailable;
- Redis deployment readiness after restore;
- exact outbox row state after relay;
- Redis DB1 exact-event presence after relay;
- exact cleanup readback;
- final readiness for Redis, `platform-gateway`, and `storage-service`.

## 14. Security Considerations

- Never print Kubernetes Secret data, decoded values, hashes, auth headers,
  Docker configs, or credentials.
- Use a pod port-forward to a selected `storage-service` pod because Redis
  outage can make readiness fail and remove Service endpoints. Do not go through
  `platform-gateway`; gateway proxying would depend on downstream Service
  endpoints while Redis is down.
- Keep the Redis outage bounded; restore in a trap on any failure.
- Synthetic storage data contains no user secret.

## 15. Implementation Steps

- [x] Capture preflight:
  - Redis deployment/service/PVC and original replica count;
  - `production-beta-runtime-config` Redis keys;
  - `platform-gateway` and `storage-service` readiness;
  - baseline Redis DB1 `events` stream length.
- [x] Choose unique ids:
  - `stamp=YYYYMMDDHHMMSS`;
  - `trace=ga-redis-outage-<stamp>`;
  - `groupId=ga-redis-group-<stamp>`;
  - `pvcId=ga-redis-pvc-<stamp>`.
- [x] Select one current `storage-service` pod and start a pod port-forward.
- [x] Load one admin static API key into local shell variables without printing
  the key or request headers.
- [x] Scale `deploy/redis` to `0` and verify Redis has no ready pod.
- [x] Send storage create request through the pod port-forward:
  `{"id":"<pvcId>","pvc_id":"<pvcId>","name":"<pvcId>","size":"1Gi","storage_class":"standard"}`.
- [x] Verify HTTP `201`.
- [x] Query Postgres by exact trace id and `GroupStorageCreated`; collect the
  exact event id and relay state.
- [x] Verify the exact Postgres outbox row remains `pending` or `retry` while
  Redis is down. Do not query Redis while it is scaled to `0`.
- [x] Restore Redis to the original replica count and wait for rollout.
- [x] Restart one relay-capable API deployment, preferably `platform-gateway`,
  to trigger the startup maintenance pass after Redis is healthy.
- [x] If the exact event remains pending because the existing relay lease still
  has a positive TTL, record that TTL, wait for natural expiry up to 20 minutes,
  then restart `platform-gateway` again and continue polling. Do not delete the
  lease key.
- [x] Wait boundedly for the exact event to reach `published` and appear in
  Redis DB1.
- [x] Delete exact synthetic platform record and exact Postgres outbox row.
- [x] Verify cleanup readback and final deployment readiness.
- [x] Update `gap.md`, `problem.md`, and `docs/acceptance/gap-analysis.md` with
  narrow OPS-011 evidence.
- [x] Run scoped verification and submit to Reviewer Agent.

## 15.1 Partial Execution Before Amendment

First live attempt stamp: `20260621202250`

Trace: `ga-redis-outage-20260621202250`

Synthetic ids:

- `groupId=ga-redis-group-20260621202250`
- `pvcId=ga-redis-pvc-20260621202250`
- record id `ga-redis-group-20260621202250:ga-redis-pvc-20260621202250`
- event id `33fc697b-2cac-4715-ac04-e46097b0ea99`

Evidence already captured:

- `deploy/redis` was `1/1`, service `redis` existed, PVC `redis-data` was
  `Bound`, and DB1 stream length was `1359`.
- `storage-service` pod `storage-service-d7b67f6c7-bxmg4` was selected for
  direct pod port-forward.
- Redis was scaled to `0`; `deploy/redis` showed `0/0`.
- While Redis was unavailable, storage create returned HTTP `201` through the
  direct storage pod port-forward.
- Postgres outbox row for event
  `33fc697b-2cac-4715-ac04-e46097b0ea99` existed as
  `GroupStorageCreated|pending|0|false`.
- Redis was restored to `1/1`; `platform-gateway` was restarted.
- The event did not publish within the initial short poll window because the
  existing relay lease still had a positive TTL. Current readback after the
  failed poll showed:
  - `redis`, `platform-gateway`, and `storage-service` all `1/1`;
  - relay lease TTL `458`;
  - event still `pending|0|false`;
  - Redis DB1 exact event match count `0`;
  - exact synthetic storage record still present for continuation.

Continuation must not update ledgers until publish-after-restore and cleanup
complete.

## 15.2 Completed Execution Evidence

Resume after approved natural-lease-expiry amendment:

- Pre-resume readiness showed `redis`, `platform-gateway`, and
  `storage-service` all `1/1`.
- Relay lease TTL before waiting was `283`, so the drill waited `288` seconds
  for natural expiry.
- After the wait, the lease had been naturally reacquired by a live worker
  (`TTL=895`). No lease key was deleted or overwritten.
- `platform-gateway` was restarted and rolled out successfully.
- First post-restart publish poll showed the exact event
  `33fc697b-2cac-4715-ac04-e46097b0ea99` as `published|0|true` in Postgres and
  `redis_matches=1` in Redis DB1 Stream `events`.
- Exact cleanup deleted:
  - one `platform_records` row for resource `storage-service:group_storage` and
    id `ga-redis-group-20260621202250:ga-redis-pvc-20260621202250`;
  - one `platform_event_outbox` row for event
    `33fc697b-2cac-4715-ac04-e46097b0ea99`.
- Cleanup readback returned `records=0` and `outbox=0`.
- Final readiness showed `redis`, `platform-gateway`, and `storage-service` all
  `1/1`.
- Redis DB1 Stream `events` length after the drill was `1364`; the exact event
  remains in Redis as append-only evidence.

## 15.3 Scoped Verification Evidence

Verification after ledger updates:

- `git diff --check -- problem.md gap.md docs/acceptance/gap-analysis.md docs/plan/2026-06-21-redis-event-broker-outage-evidence.md`
  passed.
- `kubectl -n nexuspaas get deploy redis platform-gateway storage-service`
  showed all three deployments `1/1`.
- Postgres exact cleanup readback with `platform_event_outbox.event_id` returned
  `records=0` and `outbox=0` for the synthetic storage record and event
  `33fc697b-2cac-4715-ac04-e46097b0ea99`.
- Redis DB1 exact event readback returned `redis_exact_matches=1`.
- Redis DB1 Stream `events` length remained `1364`.

## 16. Verification Plan

Commands/checks:

- `kubectl -n nexuspaas get deploy redis platform-gateway storage-service`
- `kubectl -n nexuspaas get svc redis`
- `kubectl -n nexuspaas scale deploy/redis --replicas=0`
- authenticated `POST /api/v1/storage/{groupId}/storage` through selected pod
  port-forward
- Postgres exact trace/event readback
- `kubectl -n nexuspaas scale deploy/redis --replicas=<original>`
- `kubectl -n nexuspaas rollout status deploy/redis`
- `kubectl -n nexuspaas rollout restart deploy/platform-gateway`
- `git diff --check -- problem.md gap.md docs/acceptance/gap-analysis.md docs/plan/2026-06-21-redis-event-broker-outage-evidence.md`

Success criteria:

- API mutation returns `201` while Redis is unavailable.
- Exact event is durable in Postgres outbox during outage.
- Exact event remains pending/retry in Postgres while Redis is unavailable, then
  is present in Redis DB1 after Redis restore and relay.
- Exact outbox row reaches `published` after restore.
- Redis and API deployments are restored to ready.
- Full OPS-019 remains open in ledgers.

## 17. Rollback Plan

- A shell trap restores `deploy/redis` to the original replica count on any
  failure.
- If API pods remain unready after Redis restore, restart only affected API
  deployments and re-check readiness.
- If the synthetic event does not publish within the post-expiry bound, leave
  the row in Postgres for forensic inspection, document the blocker, and do not
  update ledgers.
- Do not delete non-synthetic rows.

## 18. Risks and Tradeoffs

- Redis outage affects worker leases, rate counters, revocation cache, and Redis
  Streams during the drill. The outage is short and restored before ledger
  updates.
- Service endpoints may disappear because readiness checks include Redis; using
  direct `storage-service` pod port-forward avoids conflating Service endpoint
  readiness or gateway downstream proxying with storage API behavior.
- The selected mutation is representative, not exhaustive.
- The relay publisher after restore is not claimed to be a specific pod.
- Waiting for natural relay lease expiry makes the drill longer, but avoids
  mutating the lease key and keeps the evidence closer to normal recovery.

## 19. Reviewer Checklist

- [x] Requirement fit: OPS-011 narrow evidence only.
- [x] Plan alignment: no code or manifest changes.
- [x] SOLID: uses existing outbox/event bus ports.
- [x] 12-Factor: uses configured backing services; no hardcoded credentials.
- [x] CNCF/cloud-native: uses Kubernetes Deployment scale/rollout/readiness.
- [x] Secret safety: no secret values, hashes, or auth headers in output.
- [x] Evidence quality: committed Postgres outbox pending/retry during Redis
  outage, Redis DB1 presence after restore/relay, final readiness.
- [x] Rollback: Redis restored before ledger updates.

## 20. Status

Status: Completed; Reviewer Agent final pass returned `PASS` with no concrete
findings. Residual risks: this proves one selected `storage-service`
mutation/outbox path, not every event-producing API route; full OPS-019 remains
open; Redis keeps the append-only published event after Postgres cleanup by
design.
