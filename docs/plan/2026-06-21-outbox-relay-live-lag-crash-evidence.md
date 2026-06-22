# Outbox Relay Live Lag And Crash Evidence

## 1. Objective

Capture deterministic live evidence that the expanded transactional
outbox/inbox surface now has durable publish-lag and relay crash-recovery
behavior on the current RKE2 namespace.

This is an evidence-only slice. It must close only the remaining
`Transactional outbox/inbox` publish-lag/crash evidence gap in `problem.md` and
`gap.md`; broader typed data ownership remains open.

## 2. Background

The codebase already has:

- Postgres outbox/inbox tables in `platform_event_outbox`,
  `platform_event_inbox`, and `platform_event_checkpoints`;
- `PostgresEventBus.RelayPending` with `relay_status` transitions;
- Redis Streams as the relay sink when `EVENT_BUS_URL` is configured;
- `WithEventRelay` registering `event-outbox-relay` as a maintenance task;
- storage, authorization-policy, scheduler-quota, identity, org-project,
  image-registry, and workload mutations converted to `App.*RecordWithEvent`,
  `App.UpsertRecordWithEvent`, or `App.WithTx`.

The remaining P0 evidence gap is not another implementation pattern. It is live
proof that representative expanded-surface events publish through the durable
outbox, that publish lag drains, and that a pending event survives relay process
interruption.

## 3. Source References

- `problem.md`
- `gap.md`
- `docs/acceptance/gap-analysis.md`
- `backend/internal/platform/runtime.go`
- `backend/internal/platform/ports.go`
- `backend/internal/platform/events_postgres.go`
- `backend/internal/platform/events_redis.go`
- `backend/internal/platform/maintenance.go`
- `backend/internal/platform/lease_redis.go`
- `backend/internal/platform/schema.sql`
- `backend/internal/services/storage/handler.go`
- `backend/internal/services/storage/storage_repository.go`
- `backend/internal/services/storage/helpers.go`

## 4. Assumptions

- The live `nexuspaas` namespace uses `DATABASE_URL`, `REDIS_URL`, and
  `EVENT_BUS_URL`, so relay-capable backend processes wire
  `PostgresEventBus -> RedisEventBus` and register `event-outbox-relay`.
- The Redis lease key used by the generic relay task is
  `lease:maintenance:event-outbox-relay`.
- Synthetic admin requests must authenticate through the existing static API key
  path (`X-API-Key`) so production auth middleware can derive verified
  `X-User-*` headers. The command may read the key from Kubernetes Secret env
  inside the shell, but must never print the key, decoded value, hash, or request
  header. A unique `X-Trace-ID` is still used for evidence correlation.
- The storage-service route set is a representative expanded-surface sample,
  not the entire converted surface. It covers the three event-coupled mutation
  mechanisms used across the broader codebase:
  `CreateRecordWithEvent`, `UpsertRecordWithEvent`, and `App.WithTx` /
  `StoreTx.Emit`.
- The crash drill proves relay crash/restart durability for a valid pending row.
  It does not claim to prove every possible handler mid-transaction crash
  interleaving.

## 5. Non-Goals

- No product/runtime code changes.
- If a defect is found, stop execution, document the blocker, and submit a new
  implementation plan before editing code.
- No new event bus, queue, broker, worker, or alternate relay abstraction.
- No raw Kubernetes Secret value, decoded value, hash, auth header, Docker
  config, or credential output.
- No workload pod submission, GPU scheduling, Harbor promotion, external
  registry, WebRTC, OIDC browser login, performance, or off-cluster DR work.
- No claim that typed-domain data ownership or full 8-unit rollback is done.

## 6. Current Behavior

Preflight already showed:

- all 20 live deployments in `nexuspaas` are ready;
- outbox migration and validation jobs are complete;
- `storage-service` has `DATABASE_URL` in its runtime Secret and shared
  `EVENT_BUS_URL` / `REDIS_URL` in runtime ConfigMap keys;
- Postgres outbox has existing published rows and two pre-existing pending
  maintenance-style rows unrelated to this drill;
- Redis `events` stream currently has length `0`, so this drill must verify new
  append behavior by exact synthetic event ids.

## 7. Target Behavior

- API-created storage sample events appear in `platform_event_outbox` with the
  synthetic trace id, exact expected event names, `relay_status='published'`,
  non-null `published_at`, and `relay_attempts=0`.
- Synthetic event ids are present in Redis Stream `events` after relay.
- While a sentinel Redis lease holds `lease:maintenance:event-outbox-relay`, a
  manually inserted valid synthetic pending event remains pending.
- After deleting one selected relay-capable backend pod, releasing the sentinel
  lease, and waiting a bounded interval of up to 20 minutes, the pending event
  becomes published and appears in Redis. This bound covers the default
  15-minute `MAINTENANCE_INTERVAL` when the live image does not run a visible
  relay pass during pod startup.
- If natural `SET NX` acquisition repeatedly loses the expiry race to the live
  worker, the drill may deliberately replace the active relay lease with the
  sentinel for a short TTL. This is a bounded relay-outage injection, not a
  product code change. It proves pending rows survive controlled relay
  unavailability plus a relay-capable pod restart, then publish after the lease
  is released. It does not claim to identify the exact live process that
  published the row after release.
- Cleanup removes only exact synthetic platform records and exact synthetic
  outbox rows created by this drill. Redis stream entries are retained as
  append-only evidence.

## 8. Affected Domains

- Platform eventing / outbox relay
- Storage-service API mutation sample
- Live RKE2 operations in namespace `nexuspaas`
- Acceptance ledgers

No microservice extraction is proposed.

## 9. Affected Files

- `docs/plan/2026-06-21-outbox-relay-live-lag-crash-evidence.md`
- `problem.md`
- `gap.md`
- `docs/acceptance/gap-analysis.md`

No source code, Kubernetes manifests, or dependency files are in scope.

## 10. API / Contract Changes

None.

The live API calls use existing routes:

- `POST /api/v1/storage/{groupId}/storage`
- `POST /api/v1/storage/permissions`
- `DELETE /api/v1/storage/{groupId}/storage/{pvcId}`

Expected statuses:

- group storage create: `201`
- storage permission upsert: `200`
- group storage delete: `200`

## 11. Database / Migration Changes

No migration or schema change.

The drill reads:

- `platform_event_outbox`
- `platform_records`

The crash-recovery step inserts exactly one synthetic outbox row with:

- `event_id = 'ga-outbox-crash-<stamp>'`
- `event_name = 'GARelayCrashRecoveryProbe'`
- `source = 'ga-live-evidence'`
- `trace_id = 'ga-outbox-live-<stamp>'`
- `schema_version = 1`
- `idempotency_key = 'ga-outbox-crash-<stamp>'`
- `payload = '{"probe":"relay-crash-recovery","stamp":"<stamp>"}'`
- `occurred_at = now()`
- default `relay_status = 'pending'`

Cleanup deletes by exact collected ids only:

- `platform_records` rows with exact synthetic ids/resources created by the API
  sample;
- `platform_event_outbox` rows whose `event_id` is in the collected synthetic
  API event ids or equals the synthetic crash event id.

Do not delete generic checkpoint or inbox rows. Do not delete by broad consumer
prefix. Do not delete unrelated pre-existing pending rows.

## 12. Configuration Changes

None.

The drill may create or temporarily replace the Redis sentinel lease key:

- key: `lease:maintenance:event-outbox-relay`
- value: `ga-outbox-sentinel-<stamp>`
- TTL: short bounded TTL, default 120-180 seconds

The sentinel key must be deleted at the end of the drill. If deletion fails, wait
for TTL expiry and verify normal relay behavior before ledger updates.

## 13. Observability Changes

No persistent observability code/config change.

Evidence captured:

- deployment readiness and selected relay-capable deployment/pod name;
- baseline and final outbox relay status counts;
- exact synthetic event ids, event names, relay status, attempts, and
  published timestamps;
- Redis `XRANGE` or equivalent exact event-id match for each synthetic event;
- sentinel lease TTL/state before insertion, while pending, and after release;
- restart timestamp/readiness for the selected relay-capable pod;
- cleanup readback showing zero synthetic platform records and zero synthetic
  outbox rows.

## 14. Security Considerations

- Never print Kubernetes Secret data, decoded values, hashes, auth headers, or
  Docker configs.
- Only list Secret/ConfigMap names and key names if needed for preflight.
- Use the existing static API key contract for synthetic API calls, but do not
  echo request headers, key values, decoded values, or hashes in logs or final
  evidence.
- Synthetic event payload contains no credentials or user data.
- SQL cleanup is exact-id based and must not touch real production rows.

## 15. Implementation Steps

- [x] Capture preflight:
  - live deployment readiness;
  - storage-service original replica count and image;
  - relay-capable ConfigMap/Secret key names only;
  - baseline Postgres relay status counts;
  - baseline Redis `events` length.
- [x] Choose unique identifiers:
  - `stamp = YYYYMMDDHHMMSS`;
  - `trace = ga-outbox-live-<stamp>`;
  - `groupId = ga-outbox-group-<stamp>`;
  - `pvcId = ga-outbox-pvc-<stamp>`;
  - `permissionUserId = ga-outbox-user-<stamp>`;
  - `crashEventId = ga-outbox-crash-<stamp>`.
- [x] Port-forward `svc/platform-gateway` to a local ephemeral port.
- [x] Run API sample requests with the existing static API key loaded into a
  local shell variable from Kubernetes Secret data without printing it:
  - create group storage body:
    `{"id":"<pvcId>","pvc_id":"<pvcId>","name":"<pvcId>","size":"1Gi","storage_class":"standard"}`;
  - upsert storage permission body:
    `{"group_id":"<groupId>","pvc_id":"<pvcId>","user_id":"<permissionUserId>","permission":"read_only"}`;
  - delete group storage using the same `<groupId>` and `<pvcId>`.
- [x] Collect API-created event ids by exact trace id and expected names:
  - `GroupStorageCreated`;
  - `StoragePermissionChanged`;
  - `GroupStorageDeleted`.
- [x] Verify publish-lag drain for API-created events:
  - all collected API event ids reach `relay_status='published'`;
  - `published_at IS NOT NULL`;
  - `relay_attempts=0`;
  - Redis Stream `events` contains each exact event id.
- [x] Hold the relay lease with the sentinel value and short TTL:
  - first try natural `SET NX` acquisition without overwriting an active worker;
  - if live timing repeatedly loses the expiry race, replace the lease with the
    sentinel using a short TTL and record that this was controlled relay-outage
    injection.
- [x] Insert the synthetic crash event row with exact fields from section 11.
- [x] Verify the crash event stays pending while the sentinel lease is held.
- [x] Restart one selected relay-capable backend pod by deleting the pod under
  its deployment, not by scaling the deployment to zero.
- [x] Release the sentinel lease after pod deletion and before the replacement
  pod reaches readiness, so the replacement process's startup maintenance pass
  can acquire the relay lease immediately.
- [x] Wait for the selected deployment to become ready and wait up to 20 minutes
  for relay publication.
- [x] Verify the crash event is `published`, has `published_at IS NOT NULL`, and
  exists in Redis Stream `events`.
- [x] Cleanup:
  - delete exact synthetic `platform_records` rows;
  - delete exact synthetic outbox rows;
  - delete the sentinel lease key if present;
  - stop the port-forward.
- [x] Final readback:
  - all deployments ready;
  - selected deployment restored to original replica count;
  - zero synthetic platform records;
  - zero synthetic Postgres outbox rows;
  - cleanup did not delete unrelated rows; final relay status counts captured.
- [x] Update ledgers only after all evidence passes:
  - mark outbox publish-lag/crash evidence done;
  - keep typed data ownership and unrelated GA gaps open.

## 16. Verification Plan

- Live readiness:
  - `kubectl get deploy -n nexuspaas`
  - `kubectl rollout status -n nexuspaas deployment/<selected-deployment>`
- Postgres evidence:
  - relay status counts grouped by `relay_status`;
  - exact trace/event-id queries for API-created rows;
  - exact event-id query for the crash row;
  - exact cleanup count queries.
- Redis evidence:
  - `XLEN events`;
  - exact `XRANGE events - +` filtered for synthetic event ids;
  - `GET/PTTL lease:maintenance:event-outbox-relay` only for sentinel state,
    never for secret data.
- Local file checks:
  - `git diff --check -- problem.md gap.md docs/acceptance/gap-analysis.md docs/plan/2026-06-21-outbox-relay-live-lag-crash-evidence.md`

If product code changes become necessary, stop this plan and create a revised
implementation plan with focused Go tests, full build/test gates, and Reviewer
approval.

## 17. Rollback Plan

- If the port-forward is still running, stop it.
- If the sentinel lease still exists, delete it. If delete fails, wait for TTL
  expiry and verify relay status recovers.
- If a selected pod restart fails, run `kubectl rollout status` and inspect
  events/logs; restore the original deployment replica count if it changed.
- Delete only exact synthetic `platform_records` and exact synthetic outbox
  event ids collected during the drill.
- Do not delete Redis Stream entries; they are append-only evidence and have no
  product control-plane side effect.
- Do not update ledgers if any evidence or cleanup step fails.

## 18. Risks and Tradeoffs

- Storage-service is a representative sample, not exhaustive coverage of every
  converted mutation. This is acceptable because code-level coupling for the
  wider surface already has focused tests and Reviewer approval; this slice
  proves the shared relay mechanics live.
- The crash row is inserted directly to isolate relay durability. It is not a
  handler in-flight crash proof.
- Holding the relay lease briefly delays all outbox relay publication. The TTL
  and hard cleanup bound the risk.
- Restarting one relay-capable pod is a short live interruption for that
  deployment. The plan avoids scale-to-zero and restores readiness before ledger
  updates.

## 19. Reviewer Checklist

- [ ] Requirement fit: closes only expanded-surface publish-lag and relay
  crash-recovery evidence.
- [ ] Plan alignment: no product/runtime code edits under this plan.
- [ ] SOLID: uses existing `EventStream`, `eventRelay`, `StoreTx`, and
  repository/API paths; no new abstraction.
- [ ] 12-Factor: no hard-coded secrets or config; live config remains env-driven.
- [ ] CNCF/cloud-native: uses Kubernetes readiness/restart, Postgres, Redis
  Streams, and Redis-backed maintenance leases.
- [ ] Evidence quality: exact API statuses, exact event ids, relay status,
  Redis stream presence, sentinel lease behavior, and cleanup readback.
- [ ] Safety: no secret values printed, no broad cleanup predicates, no
  unrelated rows deleted, no ledger updates on partial failure.

## 20. Status

Status: Approved
