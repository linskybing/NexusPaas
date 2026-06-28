# AC Completion — GA Gap Tracker

_Updated: 2026-06-28 (re-verified; plan-ledger hygiene pass). Bar: **Full GA**
(`docs/acceptance/ga-checklist.md`), not just the v1 launch bar._

## First Version (V1) Status — single source of truth

- **V1 functional / Release-Candidate scope: PASSED (local).** `GATE-*`,
  `STORAGE-*`, `SECRET-*`, `AUDIT-*`, `PLANADMIN-*`, and the accepted GA families
  passed the `beta-rc` gate and a **local** RKE2 staging rollout/rollback
  rehearsal (`localhost:5000` registry, 15 deployments). See
  evidence id `2026-06-20-v1-launch-gap-gate`.
- **V1 external production launch: OPEN.** Still unproven: a real external image
  registry (Harbor is only an isolated `harbor-system` foundation, never used
  for external promotion/rollback), 8-unit topology deploy/smoke, previous-image
  rollback per unit, live external staging Secret readiness, and live staging DB
  migration/rollback drill. Static production-beta deploy-path evidence now
  proves required Secret names/keys and no local/dev/test placeholder Secret
  references in source/render only. Remote PR #33 evidence
  now shows external SonarCloud Code Analysis and Backend Quality Gate passing;
  that evidence does not close live P0.2-P0.5 or V1 external production launch.
- **Web UI (`WEB-*`) is out of V1 scope** (API/CLI-first). The existing
  `frontend/` GUI is beta/future; `WEB-*` is required only before a future Web
  UI launch.
- Throughout this file, **"first-version readiness/completion" in slice
  disclaimers means the V1 *external production launch* state above (OPEN)** — it
  does **not** mean the individual slice is unfinished.

This is the live status tracker. The verbose narrative analysis lives in
[`docs/acceptance/gap-analysis.md`](docs/acceptance/gap-analysis.md). Code-level
issues are in [`problem.md`](problem.md).

Verification basis (independent, not from the slice plans' self-attestation):
full `go test ./...`, quick gate, coverage run, focused audit retention,
scoped-query, authorization-policy transactional mutation, storage transactional
mutation, and batch per-item transactional mutation tests, live RKE2 outbox/PDP
smoke evidence, first-party `/ui/` browser smoke evidence, and live gateway
active-Project seeded E2E evidence with image-build listing (`build_count=1`),
GUI job submit/cancel route proof, GUI job logs route proof, and current-live
15 first-party backend deployment same-image rollout/undo proof, OPS-006
PostgreSQL logical backup/restore drill proof, and OPS-008 MinIO synthetic object
restore drill proof, plus OPS-009 current-live Kubernetes Secret recovery copy
drill proof for the latest verified backend images, plus static production-beta
Secret deploy-path source/render proof, plus live Harbor foundation
deploy and credential rebaseline proof with official chart `harbor-1.19.1` /
app `2.15.1`, plus Harbor static Kubernetes `local` PV replatform and OPS-007
Velero backup/restore drill proof on Velero `12.0.3` / app `1.18.1`, including
completed database/registry/jobservice PodVolumeBackups and PodVolumeRestores,
namespace/PV deletion, restored API readiness, and ORAS artifact digest/payload
verification, plus Harbor Trivy image push/scan/delete evidence using a real
`busybox:1.36` image copied by `crane` with scan status `Success`, partial
Harbor dependency outage evidence through `/api/v1/harbor-status` with trace
`ga-harbor-outage-20260621200008`, plus OPS-012 image-registry build/list
degraded-route outage evidence with trace
`ga-image-harbor-degraded-20260621212113` on image
`ci-ga-image-harbor-degraded-20260621211729`
(`sha256:21345ac6ad43db05f489bfa2ee37122b0b79873daca6f04619506c8bcef9d319`),
plus WEB-005 catalog-derived Project image status GUI/API evidence with trace
`ga-web-image-status-20260621214849` on image
`ci-ga-web-image-status-20260621214330`
(`sha256:613c1f2cefbab9f029afc580213dc619e7892753826c02367fa6ea6be129f83e`),
proving top-level `scan_status="Success"` and visible GUI state `deleted`,
plus bounded Harbor-to-catalog sync evidence with trace
`654e8a882af7e6a2099a5cce75a8377e` on image
`ci-ga-harbor-catalog-sync-reviewfix-20260621224351`
(`sha256:3730083b5b028d8a592de463892ced37b399c07bc68aef1471b9d80214168939`),
plus explicit Harbor delete-resync lifecycle evidence on image
`ci-ga-harbor-delete-lifecycle-20260621225732`
(`sha256:68b5eefe30ee0644edb4e5146523bce82ad83ea572d31fc7c7be6d5f3d1cccca`)
where `library/nexuspaas-sync:ga-harbor-delete-lifecycle-20260621225849`
synced available, Harbor delete returned `200`, exact tag lookup returned
`404`, re-sync returned `artifact_not_found`, and the catalog row became
`deleted=true` / `unavailable=true` / `status="missing"` before exact cleanup,
plus OPS-011 Redis/event-broker outage evidence with trace
`ga-redis-outage-20260621202250`, plus partial OPS-013 Prometheus/telemetry
stale and quota non-grant evidence on `ci-ga-prometheus-stale-20260621205458`,
plus WEB-001 OIDC browser-login live Playwright evidence on
`ci-ga-web-oidc-20260621203712`
(`sha256:a4cc30f2f8b6b8b949c47949de186023ba61a8ad78ea52e72b927e17ea1d670b`)
where Dex login through `platform-gateway` reached `/ui/?auth=oidc`, session
cookie names existed without logging values, browser storage had no API key or
token, and dashboard panels loaded through same-origin cookie auth, plus live
PERF-001/PERF-002 Project list k6 evidence on
`ci-ga-pdp-enforce-20260622094936` where `100` temporary principals drove
`100` VUs for `30s` with `/api/v1/projects` `1000/1000` 2xx, `0` failures,
`0` 429, p95 `3.668ms`, and `authorization-policy-service` enforce `429`
count `0`, plus WEB-006 stream credential GUI proof and PERF stream credential
issuance k6 evidence on `ci-ga-web-stream-cred-20260622102018`
(`sha256:d14aa360d5f0e4273846c88a785a2ad8cafc570613e8d892a7d9ef4407c899b1`):
live Playwright seeded streaming Job `e2e-job-mqom1t1b-pa2jbl`, proved
`stream_credentials_status=200`, `stream_credential_uri_count=1`,
`stream_credential_username_present=true`, and
`stream_credential_password_issued=true`; the GUI proof records
`stream_credential_password_redacted=true`; k6 drove `100` VUs for `30s` with
`/api/v1/stream/credentials` `3000/3000` 2xx, failure rate `0`, p95
`22.926812599999987ms`, and `0` 429/4xx/5xx, then exact temporary policies,
Secrets, and seed data were cleaned, plus WEB-007 active-Project GPU usage
live nonzero requested-GPU pod evidence on
`ci-ga-gpu-readmodel-20260622034034`
 (`sha256:2f0ebfc868a26fb59a9b3d20194756a9f8e2917b61397d50d80a16c9cde840c7`):
 seeded Project `e2e-p-mqooctn3-fammye` used fixture pod `gpu-proof` in
 namespace `gpu-e2e-p-mqooctn3-fammye` with `nvidia.com/gpu: "1"` request,
 the GUI route proof recorded `gpu_status=200`, `gpu_ok=true`, `gpu_used=1`,
 and `gpu_nonzero=true`, and the proof namespace plus temporary collector
 interval override were cleaned/restored, plus WEB-004 bounded Kubernetes
 pod-log evidence on `ci-ga-job-logs-nonempty-fix-20260622130645`
 (`sha256:fdb674beaf60e1ea052a7cbc974263b5c9fee4d39927c5980c12feb48ff2cc7e`):
 seeded Project `e2e-p-mqora84n-1y46vp` used fixture pod `log-proof`, route
 proof recorded `job_logs_status=200`, `job_logs_count=1`,
 `job_logs_nonempty=true`, and `job_logs_visible=true`, and the proof namespace
 plus temporary build/tune pods were cleaned.
Latest local SonarScanner Quality Gate passes with API readback:
`new_coverage=81.8`, `new_violations=0`,
`new_duplicated_lines_density=0.8262`. Remote PR #33 evidence also shows
external SonarCloud Code Analysis and Backend Quality Gate passing; live
P0.2-P0.5 launch evidence remains open.

## 1. Done — evidenced v1 proposed gap slices (not full GA)

| Family | Evidence | Status |
|---|---|---|
| `GATE-*` + K8S manifest cap | `backend/internal/platform/input_limits.go`, `middleware.go` (429 `Retry-After`, `MaxBytesReader`/413), `config.go` limits | Done |
| `STORAGE-001` mount-plan authorization | `storage/mount_plan_contracts_test.go` direct in-memory resolver tests for project binding, dispatch-ready group source, effective permission, and project-over-group permission precedence | Done for local/in-memory mount-plan authorization proof only |
| `STORAGE-002` mount-plan isolation | `storage/mount_plan_contracts_test.go` direct in-memory resolver tests for unrelated Project binding rejection and other-user permission denial | Done for local/in-memory cross-Project and cross-user mount-plan isolation proof only |
| `STORAGE-003` permission-management RBAC | `storage/handler_test.go` direct handler tests prove a plain group member / Project reader cannot create, batch-set, or batch-delete group/project storage permission rows, and denied deletes leave seeded rows intact | Done for local handler-level storage permission-management RBAC proof only |
| `STORAGE-004` audit | `storage/mount_plan_contracts.go` `StorageMountPlanResolved` + project-scoped AuditEvent | Done |
| Storage DataPlane dispatch API admission | evidence id `2026-06-28-storage-data-plane-kind-admission-e2e`: `backend/internal/e2e/storage_data_plane_kind_admission_e2e_test.go` | Done for env-gated live Kubernetes API admission evidence for storage DataPlane dispatch only; no CSI mount, scheduler success, local PV binding, byte mover behavior, StorageClass runtime validation, storage GA, or Full GA claim |
| Storage DataPlane cache-hit runtime in kind | evidence ids `2026-06-28-storage-data-plane-cache-hit-kind-runtime-e2e` and `2026-06-28-storage-data-plane-scratch-pvc-provisioning`: `backend/internal/e2e/storage_data_plane_cache_hit_kind_runtime_e2e_test.go`, `backend/internal/services/workload/dispatcher_dataplane.go`, `backend/internal/platform/cluster/volume_share.go` | Done for env-gated kind cache-hit runtime and scratch PVC provisioning evidence only: storage-service built a cache-hit DataPlanePlan, workload-service created the scratch PVC from that plan, dispatched a Pod using the dispatcher-created scratch PVC, the Pod reached `Succeeded`, injected checkpoint env matched the plan, a marker was written to and read back from the scratch PVC, and no stage target PVC was materialized. No stage-in byte copy, CSI/local NVMe/CephFS/Longhorn runtime, quota-aware scratch sizing, checkpoint flush, performance, multi-node behavior, storage GA, Full GA, or V1 external production launch readiness claim |
| Storage DataPlane stage-in byte copy in kind | evidence id `2026-06-28-storage-data-plane-stagein-kind-runtime-e2e`: `backend/internal/services/workload/dispatcher_dataplane_stagein_kind_e2e_test.go` | Done for env-gated workload-service kind stage-in byte-copy evidence only: a pre-populated stage PVC and stub DataPlanePlan drove workload dispatch, workload-service created the scratch PVC, the generated initContainer copied a small payload from the stage PVC into scratch, the main container read that payload and wrote a checkpoint marker, and a verify Pod read both files back from scratch. No storage-service resolver runtime, storage permission trust-boundary proof, `EnsurePVCMounted`/CSI source projection, local NVMe/CephFS/Longhorn/JuiceFS runtime, checkpoint flush to authority storage, quota-aware scratch sizing, performance, multi-node behavior, storage GA, Full GA, or V1 external production launch readiness claim |
| FastTransfer mover Job API admission | evidence id `2026-06-28-fast-transfer-mover-kind-admission-e2e`: `backend/internal/e2e/fast_transfer_mover_kind_admission_e2e_test.go` | Done for env-gated live Kubernetes API admission evidence for FastTransfer mover Job creation, repeat `already_exists`, and restricted manifest shape only; no PVC binding, Pod scheduling, rsync execution, bytes moved, progress callback, CSI, storage GA, or Full GA claim |
| FastTransfer start-to-mover API admission | evidence id `2026-06-28-fast-transfer-start-mover-kind-admission-e2e`: `backend/internal/e2e/fast_transfer_start_mover_kind_admission_e2e_test.go` | Done for env-gated live storage fast-stage-to-k8s-control mover Job admission evidence only; no PVC binding, Pod scheduling, rsync execution, bytes moved, progress callback, CSI, storage GA, or Full GA claim |
| FastTransfer mover execution in kind | evidence id `2026-06-28-fast-transfer-mover-execution-kind-e2e`: `backend/internal/e2e/fast_transfer_mover_execution_kind_e2e_test.go` | Done for env-gated kind default PVC binding, Pod scheduling, rsync command execution, and one tiny file copied through storage fast-stage -> k8s-control -> mover Job only; no CSI/storage GA, external storage backend, multi-node, multi-file, progress callback, performance, durability, or Full GA claim |
| FastTransfer progress callback emission in kind | evidence id `2026-06-28-fast-transfer-progress-callback-kind-e2e`: `backend/internal/e2e/fast_transfer_progress_callback_kind_e2e_test.go` | Done for env-gated kind FastTransfer progress callback emission evidence only: the mover Job running inside Kubernetes emitted `running` and `succeeded` HTTP POSTs to an in-cluster callback sink Service. This does not prove live k8s-control-to-storage-service callback delivery, live storage record updates, accurate byte accounting, checksum, resume, progress streaming, external storage backend, multi-node behavior, performance, durability, production-grade secret handling, workload identity, storage GA, or Full GA claim |
| FastTransfer progress storage state | evidence id `2026-06-28-fast-transfer-progress-state-ledger-sync`: `backend/internal/services/storage/fast_transfer_state_test.go`, `backend/internal/services/storage/handler.go`, `backend/internal/services/storage/spec.go`, and FastTransfer event fixtures | Done for local/in-memory storage-service handler evidence only: queued -> running -> succeeded record updates, monotonic progress/bytes checks, terminal transition rejection, scoped service identity authorization, and `FastTransferProgressed`/`FastTransferCompleted` event emission are covered. No live k8s-control-to-storage-service callback delivery, live record updates from a Kubernetes mover Job, Redis delivery, accurate byte accounting, checksum correctness, resume, production secret handling, external storage backend, multi-node behavior, performance, durability, storage GA, Full GA, or V1 launch readiness claim |
| FastTransfer progress callback-to-storage in kind | evidence id `2026-06-28-fast-transfer-progress-storage-kind-e2e`: `backend/internal/e2e/fast_transfer_progress_storage_kind_e2e_test.go` | Done for env-gated kind callback-to-storage evidence only: storage fast-stage created a mover Job, the mover copied one tiny file, the mover POSTed progress callbacks back to storage-service, the storage FastTransfer record reached `succeeded` / `progress_pct=100`, and `FastTransferProgressed` plus `FastTransferCompleted` were emitted in the in-memory event bus. No Redis delivery, durable Postgres persistence, production service identity/secret handling, accurate byte accounting, checksum correctness, resume, external storage backend, multi-node behavior, performance, durability, storage GA, Full GA, or V1 external production launch readiness claim |
| FastTransfer custom API fixtures | evidence id `2026-06-28-fast-transfer-api-fixtures`: `backend/internal/contracts/fixtures/api/v1/storage-start-fast-transfer.json`, `storage-get-fast-transfer.json`, `storage-cancel-fast-transfer.json`, `backend/internal/contracts/api_fixtures_test.go`, and `backend/internal/services/storage/api_fixtures_test.go` | Done for local/static typed external API fixture parity for custom FastTransfer fast-stage start, get, and DELETE cancel routes only. No generic/legacy transfer route coverage, live transfer execution, live authorization, live k8s-control callback delivery, bytes moved, checksum correctness, resume, external storage backend, storage GA, Full GA, or V1 external production launch readiness claim |
| Storage CacheBinding and BenchmarkRecord metadata | evidence id `2026-06-28-storage-cache-benchmark-ledger-sync`: `backend/internal/services/storage/cache_binding_test.go`, `backend/internal/services/storage/benchmark_record_test.go`, and storage API/event fixtures | Done for local/static storage metadata evidence only: CacheBinding project-manager scoped CRUD and DataPlanePlan cache-hit marking from an existing CacheBinding are covered by focused storage tests; `CacheBindingChanged` is implemented by the handler and declared in service Spec/API/event fixtures; StorageBenchmarkRecord create/list behavior, required `storage_profile`, and `StorageBenchmarkRecorded` event emission are covered by focused storage tests, with typed create/list fixture coverage in the contracts suite. No live cache residency, node-local NVMe reuse, cache eviction, live benchmark execution, fio/IOR/NCCL measurement collection, performance baselines, Kubernetes storage backend behavior, storage GA, Full GA, or V1 external production launch readiness claim |
| Storage CacheBinding typed API fixtures | evidence id `2026-06-28-storage-cache-binding-api-fixtures`: `backend/internal/contracts/fixtures/api/v1/storage-list-cache-bindings.json`, `storage-get-cache-binding.json`, `storage-update-cache-binding.json`, `storage-delete-cache-binding.json`, `backend/internal/contracts/api_fixtures_test.go`, and `backend/internal/services/storage/api_fixtures_test.go` | Done for local/static typed external API fixture parity for CacheBinding list/get/update/delete alongside the existing create fixture only. No live CRUD behavior, live authorization, node-local cache residency, DataPlanePlan runtime behavior, storage GA, Full GA, or V1 external production launch readiness claim |
| `SECRET-001..003` (v1 policy) | `schedulerquota/admission_resources.go` + `admission.go` reject raw `Secret`, safe `SecretAccessRejected`/`AuditEvent`; dispatcher defense-in-depth | Done |
| `AUDIT-001..004` | `auditcompliance/handler.go` read-time hash chain, CSV integrity columns, brand naming, project/group-scoped audit-log query RBAC with event-fed read models; `auditcompliance/cleanup.go` service-internal retention cleanup trigger | Done |
| `PLANADMIN-001..003` | `schedulerquota/handler.go` actor + old/new on Plan/Queue events | Done |
| DATA replay idempotency | `projection.go` targeted dead-letter replay + `events*.go` targeted inbox reset; tests prove replay retry does not double-apply previously successful events | Done |
| DATA-016 projection drift checks | `authorization_policy_projection_repository.go` compares raw owner/source rows with local authorization-policy projection rows for identity users/roles and policy projects/plans/image allow lists; `ide_projection_repository.go` compares raw owner/source rows with six local IDE read-model pairs for identity users/roles, policy roles, projects, project members, and user groups; `dashboard/handler.go` compares raw owner/source rows with six local dashboard read-model pairs for users, projects, project members, forms, live quotas, and queues; `clusterread/handler.go` compares raw owner/source rows with six local clusterread read-model pairs for identity users/roles, policy roles, projects, project members, and user groups; `requestnotification/project_access_repository.go` compares raw org-project source rows with three local request-notification project-access read-model pairs for projects, project members, and user groups; `gpuusage/projection.go` compares raw owner/source rows with five local GPU usage read-model pairs for identity users/roles, authorization-policy roles, org projects, and workload jobs; `imageregistry/helpers.go` compares raw owner/source rows with five local image-registry access read-model pairs for identity users/roles, org projects, project members, and user groups; focused tests cover missing, orphan, stale, clean, deterministic ordering, nil-store fail-closed behavior, canonical id normalization, projection-pair coverage, snapshot/summary exclusion for GPU usage, image-registry catalog/build/image-request/sync exclusion, and co-hosted fallback traps where applicable | Done for local/in-memory authorization-policy, IDE, dashboard, clusterread, request-notification project-access, GPU usage, and image-registry helper coverage only |
| SEC/CLI token lifecycle strengthen | `identity/auth_repository.go`, `auth.go`, internal identity auth contracts, and cleanup worker enforce session expiry, one-time refresh rotation/replay rejection, API-token expiry/revocation, and expired/revoked credential cleanup; focused handler/internal-contract tests pass | Done |

Reference: evidence id `2026-06-20-v1-launch-gap-gate`.

2026-06-28 Identity auth/session typed API local/static fixture update:
`backend/internal/contracts/fixtures/api/v1/identity-register.json`,
`identity-login.json`, `identity-refresh.json`, and `identity-cli-login.json`
now record typed external REST fixture coverage for `POST /api/v1/register`,
`POST /api/v1/login`, `POST /api/v1/refresh`, and `POST /api/v1/cli/login`.
The fixtures declare public auth posture, exact required credential fields,
success statuses, and `UserCreated` only for registration. The shared fixture
validator keeps password/refresh-token example allowances scoped to those four
identity fixtures, and the identity service parity test checks the metadata
against `identity.Spec()`. This is local/static typed external API fixture
coverage only; it does not prove live auth availability, browser cookie
behavior, OIDC/LDAP behavior, token rotation/revocation, all-critical API typed
contract coverage, DATA GA, Full GA, or V1 external production launch readiness.

2026-06-28 Identity API-token lifecycle typed API local/static fixture update:
`backend/internal/contracts/fixtures/api/v1/identity-list-api-tokens.json`,
`identity-create-api-token.json`, `identity-revoke-api-token.json`, and
`identity-revoke-current-api-token.json` now record typed external REST fixture
coverage for `GET /api/v1/me/api-tokens`, `POST /api/v1/me/api-tokens`,
`DELETE /api/v1/me/api-tokens/{id}`, and
`DELETE /api/v1/me/api-tokens/current`. The fixtures declare authenticated-user
auth posture, `id` path-parameter metadata where applicable, required create
field `name`, success statuses, list/create response fields, and `AuditEvent`
fixture metadata only for create/revoke. The shared fixture registry and
identity service parity test check this local/static contract against
`identity.Spec()` without requiring `AuditEvent` in `identity.Spec().Events`.
This does not prove live API-token lifecycle behavior, browser cookie behavior,
OIDC/LDAP behavior, all-critical API typed coverage, DATA GA, Full GA, or V1
external production launch readiness.

2026-06-23 workload local/static fixture update:
`backend/internal/contracts/fixtures/api/v1/workload-delete-configfile.json`
now records typed external REST fixture coverage for
`DELETE /api/v1/configfiles/{id}` with an empty request body, `id` path
parameter, `200 OK`, `[401, 403, 404, 500]` errors, `ConfigFileChanged`, and
`{"id":"config-ga-001","deleted":true}` response evidence. The shared fixture
registry and workload service parity test check this local/static contract
against `workload.Spec()`. This does not prove live ConfigFile deletion, project
isolation, event delivery, WEB-003/WEB-004 completion, DATA GA, Full GA, or V1
external production launch readiness.

2026-06-23 workload ConfigFile update local/static fixture update:
`backend/internal/contracts/fixtures/api/v1/workload-update-configfile.json`
now records typed external REST fixture coverage for
`PUT /api/v1/configfiles/{id}` with `id` as the path parameter, required
`content`, source-backed optional update fields, `200 OK`,
`[400, 401, 403, 404, 413, 422, 500]` errors, `ConfigFileChanged`, and an
updated ConfigFile record response. Optional `project_id`/`projectId` remain
documented only as same-Project aliases; the fixture does not imply
cross-Project ConfigFile moves, which the handler rejects with `400`.
`ConfigFileChanged` is now listed in `workload.Spec().Events` to match existing
handler emission, and create/delete/update fixture parity tests check that
metadata. This is local/static typed external API fixture coverage only; it does
not prove live ConfigFile update, project isolation, event delivery, ConfigFile
runtime rollout, WEB-003/WEB-004 completion, DATA GA, Full GA, or V1 external
production launch readiness.

2026-06-23 workload ConfigFile PATCH local/static fixture update:
`backend/internal/contracts/fixtures/api/v1/workload-patch-configfile.json`
now records typed external REST fixture coverage for
`PATCH /api/v1/configfiles/{id}` with `id` as the path parameter, required
`content`, source-backed optional update fields, `200 OK`,
`[400, 401, 403, 404, 413, 422, 500]` errors, `ConfigFileChanged`, and the
same updated ConfigFile record response evidence as the PUT update fixture. The
request example intentionally omits `project_id`/`projectId`, so it does not
imply cross-Project ConfigFile moves. The shared fixture registry and workload
service parity test check this local/static PATCH contract against
`workload.Spec()`. This is local/static typed external API fixture coverage
only; it does not prove live ConfigFile PATCH update, project isolation, event
delivery, ConfigFile runtime rollout, WEB-003/WEB-004 completion, DATA GA, Full
GA, or V1 external production launch readiness.

2026-06-23 workload ConfigFile GET local/static fixture update:
`backend/internal/contracts/fixtures/api/v1/workload-get-configfile.json`
now records typed external REST fixture coverage for
`GET /api/v1/configfiles/{id}` with `id` as the path parameter, empty request
fields/example, `200 OK`, `[401, 403, 404, 500]` errors, no emitted events, and
a public ConfigFile record response. The shared fixture validator now permits
empty `emits_events` only for GET read fixtures and still rejects empty events
for state-changing fixtures; workload parity checks the route is read-only.
This is local/static typed external API fixture coverage only; it does not
prove live ConfigFile reads, project isolation, event delivery, ConfigFile
runtime rollout, WEB-003/WEB-004 completion, DATA GA, Full GA, or V1 external
production launch readiness.

2026-06-23 STORAGE-001/STORAGE-002 local/in-memory update:
`TestResolveStorageMountPlan*` now proves mount-plan authorization decisions for
storage-owned project bindings, dispatch-ready group storage sources, effective
PVC permission, and project-permission precedence over group read-write grants.
It also proves local mount-plan isolation rejects another Project's binding and
another user's permission grant. This is resolver proof only; it does not claim
live Kubernetes mount execution, cluster PVC isolation, CSI behavior, full
storage GA, Full GA, or first-version readiness.

2026-06-23 STORAGE-003 local handler-level update:
`TestStoragePermissionManagementFollowsGroupRBAC` and
`TestProjectStoragePermissionManagementFollowsProjectRBAC` now prove plain
group member / Project reader denial for direct create/set, batch set, and
batch delete storage permission-management handlers, including no unauthorized
target-row creation and seeded-row retention after denied deletes. This is
local direct-handler RBAC proof only; it does not claim live Kubernetes PVC
isolation, namespace enforcement, storage GA, Full GA, or first-version
readiness.

2026-06-23 Storage project binding typed API local/static fixture update:
project storage binding creation now has an external REST fixture for
`POST /api/v1/projects/{id}/storage/bindings`, with `201 Created`, required
`group_id`/`pvc_id`, no optional request fields, `ProjectStorageBindingChanged`,
generic forbidden-key checks, additive/tolerant decoding, and `storage.Spec()`
parity for auth, route, path params, non-admin state-changing behavior, no
service key, no adapter, success/error statuses, direct response shape, and
event metadata. This is local/static typed external API fixture coverage only;
it does not claim live Kubernetes mount execution, cluster PVC isolation, CSI
behavior, storage GA, Full GA, or first-version completion.

2026-06-23 Storage permission creation typed API local/static fixture update:
storage permission creation now has an external REST fixture for
`POST /api/v1/storage/permissions`, with `200 OK`, required
`group_id`/`pvc_id`/`user_id`/`permission`, no optional request fields,
`StoragePermissionChanged`, generic forbidden-key checks, additive/tolerant
decoding, and `storage.Spec()` parity for auth, route, no path params,
non-admin state-changing behavior, no service key, no adapter, success/error
statuses, direct response shape, and event metadata. This is local/static typed
external API fixture coverage only; it does not claim live permission
enforcement, Kubernetes mount execution, cluster PVC isolation, namespace
enforcement, CSI behavior, storage GA, Full GA, or first-version completion.

2026-06-23 Storage project permission update typed API local/static fixture
update: project storage permission update now has an external REST fixture for
`PUT /api/v1/projects/{id}/storage/bindings/{pvcId}/permissions`, with
`200 OK`, required `user_id`/`permission`, no optional request fields,
`ProjectStoragePermissionChanged`, generic forbidden-key checks,
additive/tolerant decoding, and `storage.Spec()` parity for auth, route,
`id`/`pvcId` path params, `pvcId` route ID param, non-admin state-changing
behavior, no service key, no adapter, success/error statuses, direct project
permission response shape, and event metadata. This is local/static typed
external API fixture coverage only; it does not claim live permission
enforcement, Kubernetes mount execution, cluster PVC isolation, namespace
enforcement, CSI behavior, storage GA, Full GA, or first-version completion.

2026-06-23 Storage project permission delete typed API local/static fixture
update: project storage permission delete now has an external REST fixture for
`DELETE /api/v1/projects/{id}/storage/bindings/{pvcId}/permissions/{userId}`,
with `200 OK`, no required request fields, no optional request fields,
`ProjectStoragePermissionChanged`, and `storage.Spec()` parity for auth, route,
`id`/`pvcId`/`userId` path params, no request-body path-only semantics,
no service key, no adapter, state-changing behavior, and event metadata. This
is local/static typed external API fixture coverage only; it does not claim
live permission enforcement, Kubernetes mount execution, cluster PVC isolation,
namespace enforcement, CSI behavior, storage GA, Full GA, or first-version
completion.

2026-06-23 Storage project permission batch typed API local/static fixture
update: project storage permission batch update and batch delete now have
external REST fixtures for
`PUT /api/v1/projects/{id}/storage/bindings/{pvcId}/permissions/batch` and
`DELETE /api/v1/projects/{id}/storage/bindings/{pvcId}/permissions/batch`,
with `200 OK`, required `items` request bodies, batch result response examples,
`ProjectStoragePermissionChanged`, generic forbidden-key checks,
additive/tolerant decoding, and `storage.Spec()` parity for auth, route,
`id`/`pvcId` path params, `pvcId` route ID param, non-admin state-changing
behavior, no service key, no adapter, success/error statuses, canonical item
	fields, and event metadata. This is local/static typed external API fixture
	coverage only; it does not claim live permission enforcement, Kubernetes mount
	execution, cluster PVC isolation, namespace enforcement, CSI behavior, storage
	GA, Full GA, or first-version completion.

2026-06-23 Org-project Project update typed API local/static fixture update:
Project update now has an external REST fixture for `PUT /api/v1/projects/{id}`,
with `200 OK`, required `project_name`, source-backed mutable optional fields,
`ProjectUpdated`, generic forbidden-key checks, additive/tolerant decoding, and
`orgproject.Spec()` parity for auth, route, `id` path param, admin
state-changing behavior, no service key, success/error statuses, direct Project
response shape, and event metadata. This is local/static typed external API
fixture coverage only; it does not claim live admin authorization proof, full
Project lifecycle, tenant isolation, DATA GA, Full GA, or first-version
completion.

2026-06-23 Org-project Project delete typed API local/static fixture update:
Project delete now has an external REST fixture for
`DELETE /api/v1/projects/{id}`, with `200 OK`, no required or optional request
fields, empty request/response examples, `ProjectDeleted`, generic forbidden-key
checks, additive/tolerant decoding, and `orgproject.Spec()` parity for auth,
route, `id` path param, non-admin route metadata, state-changing behavior, no
service key, success/error statuses, and event metadata. This is local/static
typed external API fixture coverage only; it does not claim live admin
authorization proof, full Project lifecycle, tenant isolation, DATA GA, Full GA,
or first-version completion.

2026-06-23 Org-project Project batch delete typed API local/static fixture
update: Project batch delete now has an external REST fixture for
`DELETE /api/v1/projects/batch`, with `200 OK`, required top-level `ids`,
direct `succeeded`/`failed`/`errors` batch result response, `ProjectDeleted`,
generic forbidden-key checks, additive/tolerant decoding, and `orgproject.Spec()`
parity for auth, route, non-admin route metadata, state-changing behavior, no
service key, no path params, success/error statuses, canonical project IDs, and
event metadata. This is local/static typed external API fixture coverage only;
it does not claim live admin authorization proof, full Project lifecycle, tenant
isolation, DATA GA, Full GA, or first-version completion.

These rows close only the named v1/proposed slices. Remaining full-GA gaps stay
visible in Section 2.

2026-06-22 RBAC-016 local catalog-driven coverage update:
`TestRBACPublicAPIRoutesRequireAuthUnlessExplicitlyAllowed` now iterates
registered external `/api/v1/` service routes, excluding `/api/v1/internal/`
and service-auth routes; intentional public auth/OIDC entry routes must be exact
method+pattern allowlist entries with reasons, and every other scoped route is
verified to require auth and return `401` through `app.ServeHTTP` with no
credentials. This is local test coverage evidence only; it does not claim full
RBAC GA, live gateway proof, every business authorization branch, Full GA, or
first-version completion.

2026-06-22 RBAC-017 local/static metadata update: generated OpenAPI now mirrors
registered `RouteSpec` auth metadata for `x-auth`, `x-admin`,
`x-policy-bypass`, and `x-service-auth-required`, and emits user/service
security, including combined user+service requirements. Focused platform
generator and registered-route parity tests pass. This is contract metadata
evidence only; it does not claim full RBAC GA, live gateway proof, service
credential rotation, workload identity, mTLS/SPIFFE, Full GA, or first-version
completion.

2026-06-22 DATA-016 local authorization-policy drift update:
`projectionDrift` compares raw owner/source resources with local
authorization-policy read-model resources and reports missing, orphan, and
stale rows with deterministic ordering. Focused tests cover clean rows,
nil-store fail-closed behavior, and all current authorization-policy projection
pairs. This is local/in-memory repository evidence only; it does not claim a
live drift job, read-model rebuild/replay cutover, all-service DATA-016
coverage, DATA GA, Full GA, or first-version completion.

2026-06-23 DATA-016 local IDE projection drift update:
`recordStoreIDEProjectionRepository.projectionDrift` compares raw owner/source
resources with the six local IDE read-model pairs for identity users,
identity roles, authorization-policy roles, projects, project members, and user
groups. Focused in-memory tests cover missing, orphan, stale, clean,
deterministic ordering across resource pairs, canonical id normalization,
nil-store fail-closed behavior, and exact six-pair coverage. This is
local/in-memory IDE repository evidence only; it does not claim live drift
jobs, read-model rebuild/replay cutover, all-service DATA-016 coverage, DATA
GA, Full GA, or first-version readiness.

2026-06-23 DATA-016 local dashboard projection drift update:
`dashboard.projectionDrift` compares raw owner/source resources with the six
local dashboard read-model pairs for users, projects, project members, forms,
live quotas, and queues. Focused in-memory tests cover missing, orphan, stale,
clean, deterministic ordering across resource pairs, canonical id
normalization, nil app/store fail-closed behavior, exact six-pair coverage,
and the co-hosted fallback trap. This is local/in-memory dashboard helper
evidence only; it does not claim live drift jobs, read-model rebuild/replay
cutover, all-service DATA-016 coverage, DATA GA, Full GA, or first-version
readiness.

2026-06-23 DATA-016 local clusterread projection drift update:
`clusterread.projectionDrift` compares raw owner/source resources with the six
local clusterread read-model pairs for identity users, identity roles,
authorization-policy roles, projects, project members, and user groups.
Focused in-memory tests cover missing, orphan, stale, clean, deterministic
ordering across resource pairs, canonical id normalization, nil app/store
fail-closed behavior, exact six-pair coverage, excluded cluster policy role
assignments/read-model telemetry resources, and the co-hosted fallback trap.
This is local/in-memory clusterread helper evidence only; it does not claim
live drift jobs, read-model rebuild/replay cutover, all-service DATA-016
coverage, DATA GA, Full GA, or first-version readiness.

2026-06-23 DATA-016 local request-notification project-access drift update:
`recordStoreProjectAccessRepository.projectionDrift` compares raw org-project
source resources with the three local request-notification project-access
read-model pairs for projects, project members, and user groups. Focused
in-memory tests cover missing, orphan, stale, clean, deterministic ordering
across resource pairs, canonical id normalization, nil store fail-closed
behavior, exact three-pair coverage, blank-id skip behavior, source guard
coverage, and the co-hosted fallback trap. This is local/in-memory
request-notification project-access repository evidence only; it does not claim
live drift jobs, read-model rebuild/replay cutover, all-service DATA-016
coverage, DATA GA, Full GA, or first-version readiness.

2026-06-23 DATA-016 local GPU usage projection drift update:
`gpuusage.projectionDrift` compares raw owner/source resources with the five
local GPU usage read-model pairs for identity users, identity roles,
authorization-policy roles, org projects, and workload jobs. Focused
in-memory tests cover missing, orphan, stale, clean, deterministic ordering
across resource pairs, canonical id normalization for jobs/projects, nil
app/store fail-closed behavior, exact five-pair coverage, blank-id skip
behavior, snapshot/summary exclusion, and the co-hosted fallback trap. This is
local/in-memory GPU usage helper evidence only; it does not claim live drift
jobs, read-model rebuild/replay cutover, all-service DATA-016 coverage, DATA
GA, Full GA, first-version readiness, rebuild/replay cutover readiness, or
production readiness.

2026-06-23 DATA-016 local image-registry projection drift update:
`imageregistry.imageProjectionDrift` compares raw owner/source resources with
the five local image-registry access read-model pairs for identity users,
identity roles, org projects, project members, and user groups. Focused
in-memory tests cover missing, orphan, stale, clean, deterministic ordering
across resource pairs, canonical id normalization for projects/project
members, nil app/store fail-closed behavior, exact five-pair coverage,
blank-id skip behavior, catalog/build/image-request/sync exclusion, and the
co-hosted fallback trap. This is local/in-memory image-registry helper evidence
only; it does not claim live drift jobs, read-model rebuild/replay cutover,
all-service DATA-016 coverage, DATA GA, Full GA, first-version readiness,
rebuild/replay cutover readiness, or production readiness.

2026-06-23 DATA-016 local storage projection drift update:
`storage.storageProjectionDrift` compares raw owner/source resources with the
five local storage read-model pairs for identity users, identity roles,
projects, project members, and user groups. Focused in-memory tests cover
missing, orphan, stale, clean, deterministic ordering across resource pairs,
canonical id normalization, nil app/store fail-closed behavior, exact five-pair
coverage, and blank-id skip behavior. It also includes a `Config{ServiceName:"all"}`
trap proving no source fallback/merge dependency. This is local/in-memory storage
helper evidence only; it does not claim live drift jobs, read-model
rebuild/replay cutover, all-service DATA-016 coverage, DATA GA, Full GA,
first-version readiness, rebuild/replay cutover readiness, or production
readiness.

2026-06-22 Typed API local/static fixture update: request-notification
create-form now has an external REST fixture for `POST /api/v1/forms` plus
focused contracts/spec parity tests for metadata, required request fields,
forbidden example keys, additive/tolerant decoding, and route auth/action
alignment. This is local/static request-notification create-form external API
fixture coverage only; it does not claim OpenAPI-first completion, all critical
APIs, DATA GA, Full GA, or first-version completion.

2026-06-22 Image-registry typed API local/static fixture update: image build
create routes now have external REST fixtures for `POST /api/v1/images/build`,
`POST /api/v1/images/build/from-storage`, and
`POST /api/v1/images/build/dockerfile`, with `202 Accepted`, required
`project_id`/`image_reference`/resource fields, `ImageBuildStarted`, generic
all-2xx success status validation, forbidden-key checks, additive/tolerant
decoding, and `imageregistry.Spec()` parity for auth, route, path params,
state-changing, and `harbor` adapter metadata. This is local/static typed
external API fixture coverage only; it does not claim live Harbor build
execution, SBOM/signing, allow-list enforcement, image scan lifecycle, full image
workflow, Full GA, or first-version completion.

2026-06-28 Image-registry acceleration and queued supply-chain metadata update:
`ImageAccelerationProfile` now has local metadata/contract coverage through
admin CRUD routes, seeded defaults, a create API fixture, and
`ImageAccelerationProfileChanged` event fixture. Queued image builds now also
record and emit pending supply-chain status metadata in create responses, stored
records, and `ImageBuildStarted` events:
`image_digest=""`, `allow_list_decision="pending"`,
`sbom_status="pending"`, `signature_status="pending"`,
`scan_status="pending"`, and `supply_chain_checked_at=null`. Contract coverage
also validates historical `ImageBuildStarted` schema-v1 payloads without those
additive keys. This is local metadata/event-shape evidence only; it does not
claim image conversion/prewarm execution, completed SBOM generation, signing,
scan enforcement, allow-list admission, live Harbor/Tekton/BuildKit execution,
full IMG, V1 external launch, or Full GA.

2026-06-23 DATA-014 image build create idempotency local update:
image-registry build create APIs now have local deterministic optional
`Idempotency-Key` replay/conflict evidence. Same-key same-request replays return
the existing accepted build without a duplicate record or duplicate
`ImageBuildStarted` event, and same-key changed-request attempts return
`409 Conflict`. Internal matching metadata is stripped from create, list, cancel,
and event payload data. All three image build create external REST fixtures also
list `Idempotency-Key` as an optional request header, which is public/static
contract evidence only. This is local image-build-create evidence only; it does
not claim idempotency for deploy or all DATA-014
commands, live Harbor/Tekton/BuildKit execution, SBOM/signing/scan enforcement,
DATA GA, IMG GA, V1 external launch, or Full GA.

2026-06-24 DATA-014 image build cancel idempotency local update:
image-registry build cancel routes now have local deterministic optional
`Idempotency-Key` replay/conflict evidence. Same-key same-target replays,
including across the two cancel route aliases, return the existing cancelled
build without a duplicate `ImageBuildCancelled` event, and same-key
different-target attempts return `409 Conflict` without cancelling the second
build. Internal cancel matching metadata is stripped from cancel responses, list
responses, and event payload data. This is local image-build-cancel and `IMG-012`
cancel command behavior evidence only; it does not claim full DATA-014, deploy
idempotency, live executor cancellation, live Harbor/Tekton/BuildKit/Kubernetes,
full IMG/DATA, V1 external launch, or Full GA.

2026-06-24 DATA-014 workload submit idempotency local update:
workload `POST /api/v1/jobs` now has local deterministic optional
`Idempotency-Key` replay/conflict evidence. Same-key same-semantic-payload
replays return the existing submitted job with no duplicate scheduler admission,
auto-preemption, job record, or `JobSubmitted` event side effects, while same-key
different-payload attempts return `409 Conflict` before those side effects.
Internal submit matching metadata is stripped from submit, list, get, and event
payload data. This is local workload submit evidence only; it does not claim full
DATA-014, deploy idempotency, live Kubernetes apply,
full workload GA, V1 external launch, or Full GA.

2026-06-24 DATA-014 workload cancel idempotency local update:
workload `POST /api/v1/jobs/{id}/cancel` now has local deterministic optional
`Idempotency-Key` replay/conflict evidence and public/static fixture consistency.
Same-key same-semantic-command replays, including canonical-equivalent job
identifiers, return the existing accepted cancel command with no duplicate
command record or `JobCancelRequested` event; same-key different-target attempts
return `409 Conflict` without a new command or event. Internal cancel matching
metadata is stripped from cancel responses and event payload data, and the
external REST fixture lists only the optional header name. This is local workload
cancel evidence only; it does not claim full DATA-014, deploy idempotency, live
scheduler/Kubernetes cancellation, GPU evidence, full workload GA, V1 external
launch, or Full GA.

2026-06-24 DATA-014 scheduler preemption idempotency local update:
scheduler explicit preemption now has local deterministic optional
`Idempotency-Key` replay/conflict evidence. Same-key same-request replays return
the existing sanitized preemption decision without duplicate cleanup, workload
transition, or `JobPreempted` event side effects, while same-key
different-requester or different-payload attempts return `409 Conflict` before
victim selection, cleanup, workload preempt, or event emission. A generated
opaque public `preemption_id` is used in responses, workload preempt calls, and
event payload data; the key-derived store record ID and internal key/fingerprint
hashes remain private. This is local scheduler/workload preemption evidence only;
it does not claim full DATA-014, deploy idempotency, live scheduler/Kubernetes
preemption, live GPU behavior, full scheduler/workload GA, V1 external launch,
or Full GA.

2026-06-24 IMG-011 image build log redaction local update:
image-registry build log responses now have focused local deterministic evidence
for output-side redaction of Authorization bearer values and common secret-like
key/value log fields while preserving ordinary log lines and persisted
unredacted records. This is local backend response evidence only; it does not
claim live Harbor/Tekton/BuildKit logs, streaming/tailing, SBOM/signing/scan,
full image workflow closure, GPU closure, V1 external launch, or Full GA.

2026-06-24 IMG-012/IMG-013 image build active-slot release local update:
image-registry now has focused local deterministic evidence that cancellation
through the build cancel handler and timeout terminal statuses release the local
active-build concurrency slot. This is local admission/quota-slot evidence only;
it does not claim live resource termination, a real timeout controller,
executor/Kubernetes quota release, live Harbor/Tekton/BuildKit, SBOM/signing/scan,
full image workflow closure, GPU closure, V1 external launch, or Full GA.

2026-06-22 Workload typed API local/static fixture update: job submission now
has an external REST fixture for `POST /api/v1/jobs`, with `201 Created`,
required `project_id`/`user_id`, optional UI/admission/defaultable fields kept
out of direct validation requirements, `JobSubmitted`, generic forbidden-key
checks, additive/tolerant decoding, and `workload.Spec()` parity for auth,
route, path params, state-changing, success status, and event metadata. This is
local/static typed external API fixture coverage only; it does not claim live
scheduler admission, queue policy completeness, Kubernetes job execution,
logs/tailing, GPU telemetry, WEB-003/WEB-004 completion, Full GA, or
first-version completion.

2026-06-22 Workload ConfigFile typed API local/static fixture update:
ConfigFile creation now has an external REST fixture for
`POST /api/v1/configfiles`, with `201 Created`, required `project_id`/`name`,
source-backed optional aliases/payload fields, `ConfigFileChanged` as the
handler-emitted create event, generic forbidden-key checks, additive/tolerant
decoding, and `workload.Spec()` parity for auth, route, no service key/path
params, non-admin state-changing behavior, success status, and source-backed
error statuses. This is local/static typed external API fixture coverage only;
it does not claim live scheduler admission, Kubernetes job execution,
logs/tailing, GPU telemetry, WEB-003/WEB-004 completion, DATA GA, Full GA, or
first-version completion.

2026-06-23 Workload ConfigFile update typed API local/static fixture update:
ConfigFile update now has an external REST fixture for
`PUT /api/v1/configfiles/{id}`, with `200 OK`, `id` path parameter, required
`content`, source-backed optional update fields, `ConfigFileChanged`,
forbidden-key checks, additive/tolerant decoding, and `workload.Spec()` parity
for auth, route, `id` route parameter, non-admin state-changing behavior, no
service key, success/error statuses, response shape, and event metadata.
Optional `project_id`/`projectId` are same-Project aliases only and do not
indicate cross-Project moves are supported. This is local/static typed external
API fixture coverage only; it does not claim live ConfigFile update, project
isolation, event delivery, ConfigFile runtime rollout, WEB-003/WEB-004
completion, DATA GA, Full GA, or first-version completion.

2026-06-23 Workload ConfigFile GET typed API local/static fixture update:
ConfigFile read now has an external REST fixture for
`GET /api/v1/configfiles/{id}`, with `200 OK`, `id` path parameter, empty
request fields/example, no emitted events, forbidden-key checks,
additive/tolerant decoding, and `workload.Spec()` parity for auth, route, `id`
route parameter, read-only state, no service key, success/error statuses, and
response shape. This is local/static typed external API fixture coverage only;
it does not claim live ConfigFile reads, project isolation, event delivery,
ConfigFile runtime rollout, WEB-003/WEB-004 completion, DATA GA, Full GA, or
first-version completion.

2026-06-23 Workload cancel-job typed API local/static fixture update:
job cancellation now has an external REST fixture for
`POST /api/v1/jobs/{id}/cancel`, with empty `{}` request body, `id` path
parameter, `202 Accepted`, command-record response example,
`JobCancelRequested`, generic forbidden-key checks, additive/tolerant decoding,
and `workload.Spec()` parity for auth, route, `id` route parameter,
non-admin state-changing behavior, no service key, success/error statuses,
empty-body command semantics, and event metadata. `JobCancelRequested` is now
listed in `workload.Spec().Events` to match the existing handler emission. This
is local/static typed external API fixture coverage only; it does not claim
live scheduler cancellation, Kubernetes job termination, cancellation
propagation, logs/tailing, GPU telemetry, WEB-004 completion, DATA GA, Full GA,
or first-version completion.

2026-06-23 Workload ConfigFile version commit typed API local/static fixture
update: ConfigFile version commit now has an external REST fixture for
`POST /api/v1/configfiles/{id}/versions`, with `id` path parameter,
required `content`, optional `message`/`manifest`/`yaml`/`config`,
`201 Created`, version-record response metadata including `config_id`,
`content`, `message`, `sha256`, `immutable: true`, and `committed_at`,
`ConfigCommitted`, generic forbidden-key checks, additive/tolerant decoding,
and `workload.Spec()` parity for auth, route, `id` route parameter,
non-admin state-changing behavior, no service key, success/error statuses,
response shape, and event metadata. This is local/static typed external API
fixture coverage only; it does not claim live scheduler admission, Kubernetes
job execution, ConfigFile runtime rollout, logs/tailing, GPU telemetry,
WEB-003/WEB-004 completion, DATA GA, Full GA, or first-version completion.

2026-06-22 Org-project typed API local/static fixture update: Project creation
now has an external REST fixture for `POST /api/v1/projects`, with
`201 Created`, required `project_name`/`g_id`, conservative source-backed
optional fields, `ProjectCreated`, generic forbidden-key checks,
additive/tolerant decoding, and `orgproject.Spec()` parity for auth, route, no
service key/path params, non-admin route metadata, state-changing behavior,
success/error statuses, and event metadata. This is local/static typed external
API fixture coverage only; it does not claim live admin authorization proof,
full Project lifecycle, tenant isolation, DATA GA, Full GA, or first-version
completion.

2026-06-22 Org-project Group create typed API local/static fixture update:
Group creation now has an external REST fixture for `POST /api/v1/groups`, with
`201 Created`, required `group_name`, conservative source-backed optional
fields, `GroupCreated`, generic forbidden-key checks, additive/tolerant
decoding, and `orgproject.Spec()` parity for auth, route, no service key/path
params, admin route metadata, state-changing behavior, success/error statuses,
direct group response shape, and event metadata. This is local/static typed
external API fixture coverage only; it does not claim live admin authorization
proof, full Group lifecycle, tenant isolation, DATA GA, Full GA, or
first-version completion.

2026-06-23 Org-project Group update typed API local/static fixture update:
Group update now has an external REST fixture for `PUT /api/v1/groups/{id}`,
with `200 OK`, required `group_name`, source-backed mutable optional fields,
`GroupUpdated`, generic forbidden-key checks, additive/tolerant decoding, and
`orgproject.Spec()` parity for auth, route, `id` path param, admin route
metadata, state-changing behavior, success/error statuses, direct group
	response shape, and event metadata. `GroupUpdated` is now listed in
	`orgproject.Spec().Events` to match the existing handler emission. This is
	local/static typed external API fixture coverage only; it does not claim live
	admin authorization proof, full Group lifecycle, tenant isolation, DATA GA,
	Full GA, or first-version completion.

2026-06-23 Org-project Group delete typed API local/static fixture update:
Group delete now has an external REST fixture for `DELETE /api/v1/groups/{id}`,
with `200 OK`, no required or optional request fields, empty request/response
examples, `GroupDeleted`, generic forbidden-key checks, additive/tolerant
decoding, and `orgproject.Spec()` parity for auth, route, `id` path param,
admin route metadata, state-changing behavior, success/error statuses, empty
response shape, and event metadata. `GroupDeleted` is now listed in
`orgproject.Spec().Events` to match the existing handler emission. This is
local/static typed external API fixture coverage only; it does not claim live
admin authorization proof, full Group lifecycle, tenant isolation, DATA GA,
Full GA, or first-version completion.

2026-06-23 Org-project Group batch delete typed API local/static fixture
update: Group batch delete now has an external REST fixture for
`DELETE /api/v1/groups/batch`, with `200 OK`, required top-level `ids`, direct
`succeeded`/`failed`/`errors` batch result response, `GroupDeleted`, generic
forbidden-key checks, additive/tolerant decoding, and `orgproject.Spec()` parity
for auth, route, admin route metadata, state-changing behavior, no service key,
no path params, success/error statuses, canonical group IDs, and event metadata.
This is local/static typed external API fixture coverage only; it does not
claim live admin authorization proof, full Group lifecycle, tenant isolation,
DATA GA, Full GA, or first-version completion.

## 2. Incomplete for GA

| AC family | What's missing | Evidence |
|---|---|---|
| `WEB-001..007` (not a V1 blocker — Web UI out of V1 scope) | Partial — first-party `frontend/` operations GUI exists and is served by `platform-gateway` at `/ui/`; live Playwright smoke passes. WEB-001 now has live OIDC browser-login evidence: Dex login through `platform-gateway` reached `/ui/?auth=oidc`, session cookie names existed without logging values, browser storage had no API key or token, and dashboard panels loaded through same-origin cookie auth on image `ci-ga-web-oidc-20260621203712` (`sha256:a4cc30f2f8b6b8b949c47949de186023ba61a8ad78ea52e72b927e17ea1d670b`). WEB-002 has live active-Project evidence: seeded E2E created a real Group/Project through existing REST routes, connected to `/ui/`, selected the seeded Project, and proved it was present in the active selector. WEB-003/WEB-004 have partial Workloads coverage: the GUI calls existing ConfigFile/job REST routes, lists ConfigFiles, filters authorized jobs by active Project for display, submits a minimal ConfigFile and Job for the active Project, sends job cancel requests, and reaches the existing job logs route from the browser. Earlier live seeded E2E submitted ConfigFile `CFG2600007`, submitted Job `e2e-job-mqneymza-1tqckn`, displayed that job, requested logs with `job_logs_status=200` / `job_logs_count=0`, and requested cancel with command `94925e294549528a2190b3dbafd09592`. WEB-005/WEB-007 remain partial surfaces: Images lists Project images and image builds from existing image-registry routes, Usage lists current-user GPU/request usage and active-Project GPU usage from existing usage-observability routes, project GPU failures render as unavailable, and tests prove no admin-usage fallback or credential persistence. Harbor foundation now exists in `harbor-system`, and Harbor-side push/scan/delete evidence has passed with Trivy. WEB-005 catalog-derived image status display now has live API and Playwright GUI evidence under trace `ga-web-image-status-20260621214849`: a seeded Project image on `ci-ga-web-image-status-20260621214330` exposed top-level `scan_status="Success"`, `deleted=true`, `unavailable=false`, the seeded digest, and visible GUI state `deleted`. This proves UI/API display of read-model metadata. WEB-005 / IMG-024 now also has focused local frontend evidence for active-Project Dockerfile build submission through `POST /api/v1/images/build/dockerfile`, including trimmed `image_reference`, success refresh of Project images/builds, no `/admin` fallback, no browser storage persistence, and generic secret-safe submit failure text; this does not prove live Harbor build execution, SBOM/signing, allow-list enforcement, or full image workflow GA. Bounded Harbor-to-catalog sync is also live-evidenced through evidence id `2026-06-21-harbor-catalog-sync-execution`: trace `654e8a882af7e6a2099a5cce75a8377e`, image `ci-ga-harbor-catalog-sync-reviewfix-20260621224351` (`sha256:3730083b5b028d8a592de463892ced37b399c07bc68aef1471b9d80214168939`), Harbor artifact digest `sha256:73aaf090f3d85aa34ee199857f03fa3a95c8ede2ffd4cc2cdb5b94e566b11662`, sync status `synced`, `code="ok"`, and exact cleanup verified. Explicit delete-resync lifecycle is live-evidenced through evidence id `2026-06-21-harbor-delete-lifecycle-sync`: image `ci-ga-harbor-delete-lifecycle-20260621225732` (`sha256:68b5eefe30ee0644edb4e5146523bce82ad83ea572d31fc7c7be6d5f3d1cccca`), artifact `library/nexuspaas-sync:ga-harbor-delete-lifecycle-20260621225849`, Harbor delete `200`, exact tag lookup `404`, re-sync `code="artifact_not_found"`, catalog `deleted=true`, `unavailable=true`, `status="missing"`, and exact cleanup verified. The WebRPC GUI contract for GA v1 is approved as existing same-origin REST/OpenAPI consumption; no separate WebRPC/tRPC/gRPC transport is required until a concrete API gap is proven. Earlier live seeded proof had `project_count=1`, `seeded_project_present=true`, `config_file_count=1`, `job_count=3`, `seeded_job_present=true`, `job_cancel_requested=true`, `job_logs_requested=true`, `job_logs_status=200`, `job_logs_count=0`, `image_count=1`, `build_count=1`, `gpu_status=200`, and `gpu_ok=true`; the WEB-005 status proof separately verified `scan_status="Success"` and state `deleted`; a direct live route probe returned `used=0` for a seeded Project with no GPU pods, and the later GPU read-model proof recorded `gpu_status=200`, `gpu_ok=true`, `gpu_used=1`, and `gpu_nonzero=true`; the later WEB-004 bounded pod-log proof recorded `job_logs_status=200`, `job_logs_count=1`, `job_logs_nonempty=true`, and `job_logs_visible=true`. Remaining GA Web scope still lacks full WebRTC media/session operation, real workload GPU utilization/per-device telemetry evidence, live continuous log tailing/full workload status workflow evidence beyond the focused frontend REST polling slice, full usage workflow evidence, Harbor scan lifecycle synchronization and registry-wide automatic delete lifecycle beyond explicit per-tag sync/delete-resync, the full image-build/allow-list/SBOM/signing/GUI scan workflow, and full WEB AC coverage. | evidence id `2026-06-22-gpu-usage-read-model-live-proof`; evidence id `2026-06-22-web-gui-oidc-browser-login`; evidence id `2026-06-22-oidc-gateway-forwarded-origin`; evidence id `2026-06-21-web-gui-job-logs-route-proof`; evidence id `2026-06-21-web-gui-job-submit-cancel-live-e2e`; evidence id `2026-06-21-gateway-adapter-route-proxy-precedence`; evidence id `2026-06-21-image-build-live-list-evidence`; evidence id `2026-06-21-clusterread-static-admin-gpu-usage`; evidence id `2026-06-21-web-gui-active-project-live-e2e`; evidence id `2026-06-21-orgproject-static-admin-compatibility`; evidence id `2026-06-21-web-gui-foundation-live-e2e`; evidence id `2026-06-21-web-gui-first-party-serving`; evidence id `2026-06-21-web-gui-project-selector`; evidence id `2026-06-21-web-gui-workload-workflows`; evidence id `2026-06-21-web-gui-image-usage-contract`; evidence id `2026-06-21-web-gui-image-status-parity`; evidence id `2026-06-21-gateway-downstream-catalog-proxy`; evidence id `2026-06-21-platform-admin-policy-bootstrap`; evidence id `2026-06-21-harbor-foundation-live-deploy`; evidence id `2026-06-21-harbor-foundation-credential-rebaseline`; evidence id `2026-06-21-harbor-image-scan-live-evidence`; evidence id `2026-06-21-harbor-delete-lifecycle-sync`; OIDC image `localhost:5000/nexuspaas-backend:ci-ga-web-oidc-20260621203712` (`sha256:a4cc30f2f8b6b8b949c47949de186023ba61a8ad78ea52e72b927e17ea1d670b`); image-registry image `localhost:5000/nexuspaas-backend:ci-ga-harbor-delete-lifecycle-20260621225732` (`sha256:68b5eefe30ee0644edb4e5146523bce82ad83ea572d31fc7c7be6d5f3d1cccca`); gateway image `localhost:5000/nexuspaas-backend:ci-ga-web-job-logs-20260621143553` (`sha256:3111ba2be88c8b0cb4c344f172e468253c8b8c862930763d426117776ab1a824`); WEB-005 status image `localhost:5000/nexuspaas-backend:ci-ga-web-image-status-20260621214330` (`sha256:613c1f2cefbab9f029afc580213dc619e7892753826c02367fa6ea6be129f83e`); previous gateway job-submit image `localhost:5000/nexuspaas-backend:ci-ga-web-job-submit-20260621141339` (`sha256:aee156e2904e03d30dc3d671545a1b9e86e45e27092a03645427663d2544ccc4`); previous gateway proxy-adapter image `localhost:5000/nexuspaas-backend:ci-ga-gateway-proxy-adapter-20260621054757` (`sha256:3cda2888dda836a1cd197c476c31342dd7e2f6f6befe5fa7e785ab46d13bc700`); org-project image `localhost:5000/nexuspaas-backend:ci-ga-org-static-admin-20260621125516` (`sha256:7310012c13eb9ee0667ac3f27eddf839c0d13c8d53b8b1560916762158b61471`); usage-observability/image-registry image `localhost:5000/nexuspaas-backend:ci-ga-clusterread-static-admin-20260621132623` (`sha256:f6c6ab5badac315095c4ac299fb2ded4fac8c4f29ae910f65bed16eb9368a87f`) |
| E2E (ga-checklist) | Full critical **live** E2E not evidenced — focused/local E2E plus local RKE2 single-namespace smoke exist. Latest live smokes prove PDP service-key scope, HTTP form create, durable outbox row, first-party `/ui/` dashboard rendering, gateway project-list routing, OIDC browser login through Dex and HttpOnly session cookies, active Project seeded selection, live GUI ConfigFile submit, live GUI job submit/cancel, live GUI job logs route consumption with bounded non-empty rendering, Project image list rendering, image build list rendering for a seeded build, catalog-derived Project image scan/deleted state rendering in the GUI, current-user usage/request-usage API rendering, active-Project GPU usage route success for a seeded Project with no GPU pods plus nonzero requested-GPU pod visibility evidence, current-live 15 first-party backend deployment same-image rollout/undo success, OPS-006 PostgreSQL logical backup/restore drill success, OPS-008 MinIO synthetic object restore drill success, OPS-009 current-live Kubernetes Secret recovery copy drill success, live Harbor foundation deploy/credential rebaseline readiness success, OPS-007 Harbor static-local Velero backup/restore drill success, Harbor-side Trivy push/scan/delete success, OPS-011 Redis/event-broker outage evidence, partial OPS-013 Prometheus/telemetry stale and quota non-grant evidence, Harbor dependency outage evidence through `/api/v1/harbor-status`, and OPS-012 image-registry build/list degraded-route outage evidence. They still are not the full critical-path E2E suite because full WebRTC media/session launch, Harbor scan lifecycle synchronization and registry-wide automatic delete lifecycle beyond explicit per-tag sync/delete-resync, real workload GPU utilization/per-device telemetry evidence, live continuous log tailing/full workload status beyond the focused frontend REST polling slice, full usage workflow evidence, full NexusPaaS image-build/allow-list/SBOM/signing/GUI scan workflow evidence, managed/off-cluster secret recovery, PITR/off-cluster DR, target 8-unit/previous-image rollback per unit, full OPS-019 failure injection beyond the completed Redis/event-broker, Prometheus telemetry/admission, Harbor dependency/status API, and OPS-012 build/list degraded-route slices, and load/perf evidence remain open. | OIDC browser-login proof evidence id `2026-06-22-web-gui-oidc-browser-login` and evidence id `2026-06-22-oidc-gateway-forwarded-origin`, image `ci-ga-web-oidc-20260621203712` (`sha256:a4cc30f2f8b6b8b949c47949de186023ba61a8ad78ea52e72b927e17ea1d670b`), live Playwright reached `/ui/?auth=oidc`, observed only cookie names, found no browser storage API key/token, and loaded dashboard panels; `POST /api/v1/forms` -> `FormCreated` outbox row (`ga-outbox-live-20260620163919`); `platform-gateway /ui/` Playwright smoke and live `GET /api/v1/projects` through gateway on `ci-ga-admin-policy-20260621020259`; `ci-ga-web-workloads-20260620193544` rolled `/ui/` HTML/asset/Playwright smoke and live `GET /api/v1/configfiles`, `GET /api/v1/jobs` empty-list checks; `ci-ga-web-image-usage-20260621013315` rolled Images/Usage panels; gateway proxy-adapter image `ci-ga-gateway-proxy-adapter-20260621054757`; gateway job-submit image `ci-ga-web-job-submit-20260621141339`; gateway job-logs image `ci-ga-web-job-logs-20260621143553`; seeded active-Project E2E route proof `project_count=1`, `seeded_project_present=true`, `config_file_count=1`, `job_count=3`, `seeded_job_present=true`, `job_cancel_requested=true`, `job_cancel_command_id=94925e294549528a2190b3dbafd09592`, `job_logs_requested=true`, `job_logs_status=200`, `job_logs_count=0`, `image_count=1`, `build_count=1`, `gpu_status=200`, `gpu_ok=true`; WEB-005 status proof evidence id `2026-06-21-web-gui-image-status-parity` (`ga-web-image-status-20260621214849`, image `ci-ga-web-image-status-20260621214330`, digest `sha256:613c1f2cefbab9f029afc580213dc619e7892753826c02367fa6ea6be129f83e`, `scan_status="Success"`, state `deleted`); direct GPU route probe `status=200 used=0`; GPU read-model proof `gpu_status=200`, `gpu_ok=true`, `gpu_used=1`, `gpu_nonzero=true`; WEB-004 bounded pod-log proof `job_logs_status=200`, `job_logs_count=1`, `job_logs_nonempty=true`, `job_logs_visible=true`; current-live rollout/undo plan evidence id `2026-06-21-current-live-rollout-undo-evidence`; PostgreSQL restore drill evidence id `2026-06-21-postgres-backup-restore-drill`; MinIO object restore drill evidence id `2026-06-21-minio-object-restore-drill`; Kubernetes Secret recovery drill evidence id `2026-06-21-kubernetes-secret-recovery-drill`; Harbor foundation/rebaseline plans evidence id `2026-06-21-harbor-foundation-live-deploy` and evidence id `2026-06-21-harbor-foundation-credential-rebaseline`; blocked Harbor Velero attempt evidence id `2026-06-21-harbor-velero-backup-restore-drill`; Harbor static local Velero drill evidence id `2026-06-21-harbor-static-local-pv-velero-drill`; Harbor scan plan evidence id `2026-06-21-harbor-image-scan-live-evidence`; Harbor outage plan evidence id `2026-06-21-harbor-outage-failure-injection` (`ga-harbor-outage-20260621200008`); image-registry Harbor degraded build/list plan evidence id `2026-06-21-image-registry-harbor-degraded-build-list` (`ga-image-harbor-degraded-20260621212113`, image `ci-ga-image-harbor-degraded-20260621211729`, digest `sha256:21345ac6ad43db05f489bfa2ee37122b0b79873daca6f04619506c8bcef9d319`); Redis outage plan evidence id `2026-06-21-redis-event-broker-outage-evidence` (`ga-redis-outage-20260621202250`, event `33fc697b-2cac-4715-ac04-e46097b0ea99`); Prometheus stale/quota plan evidence id `2026-06-21-prometheus-stale-quota-evidence`, image `ci-ga-prometheus-stale-20260621205458` (`sha256:43a52e3875fa2ae9c0febf9a158537b7cdaeed97c7ef285f00ab9f5fee194a86`), trace `ga-prometheus-stale-20260621205959`; org-project static admin compatibility image `ci-ga-org-static-admin-20260621125516`; usage-observability/image-registry static admin compatibility image `ci-ga-clusterread-static-admin-20260621132623` |
| Backup/restore | Partial — OPS-006 PostgreSQL logical backup/restore drill passed live: `pg_dump -Fc` created a non-empty archive (`353198` bytes, SHA-256 `6c0869c3e591e9768edceee55feb80cbdf1e61e2e67b162a0b9a8bf6424a1c71`), `pg_restore --exit-on-error --single-transaction` restored into temporary DB `nexuspaas_restore_ops006_20260621150759`, restored public table count matched `81`, selected row counts matched, and temp DB/local dump cleanup passed. OPS-008 MinIO synthetic object drill passed live: object `media/ops008/20260621151838/payload.txt` was uploaded, backed up, deleted, restored, downloaded, SHA-256 matched (`dc4c12603462b5385f6cbb676cff88ba6503e1aba4b42ef10224c0b276c76d5b`), then remote object/local artifacts were cleaned. OPS-009 current-live Kubernetes Secret recovery copy drill passed for 21 selected Secrets without printing values or hashes. Static production-beta Secret deploy-path evidence proves required Secret names/keys and no local/dev/test placeholder Secret refs in source/render, without checking values. OPS-007 Harbor backup/restore now passed live after replatforming Harbor from unsupported Rancher `local-path` hostPath PVCs to Kubernetes static `local` PVs: Harbor chart `harbor-1.19.1` / app `2.15.1` restored ready, Velero chart `velero-12.0.3` / app `1.18.1` used dedicated BackupStorageLocation `ops007-static-20260621161910`, Backup `ops007-harbor-static-20260621161910` completed with `errors=0` / `warnings=0`, non-Redis PVBs completed for `database-data` (`51369080/51369080`), `registry-data` (`862/862`), and `job-logs` (`0/0`), `harbor-system` and exact static PVs were deleted, local PV directories were emptied, exact static PVs and the intentionally excluded empty Redis PVC were recreated, Restore `ops007-harbor-static-restore-20260621161910` completed with `errors=0` / `warnings=1` from Velero nodeOS detection only, matching PVRs completed, restored API ping returned `200`, read-only was unset, ORAS resolved and pulled digest `sha256:c7837e0a80dc7266b26eb197901b3ce8c3b893dc5ecb70208d13aab58dc70c46`, restored payload matched, synthetic Harbor data was cleaned, and final Harbor state was read-write with only the `library` project. Remaining GA gaps: managed/off-cluster secret recovery, Vault/External Secrets/Sealed Secrets/SOPS/KMS recovery, secret rotation/revocation, live external staging Secret object/provenance proof, PITR, off-cluster retention, versioned object restore, bucket metadata/IAM restore, encrypted backup storage, HA/off-cluster Harbor storage, and full DR. | evidence id `2026-06-21-postgres-backup-restore-drill`; evidence id `2026-06-21-minio-object-restore-drill`; evidence id `2026-06-21-kubernetes-secret-recovery-drill`; evidence id `2026-06-21-harbor-foundation-live-deploy`; evidence id `2026-06-21-harbor-foundation-credential-rebaseline`; evidence id `2026-06-21-harbor-velero-backup-restore-drill`; evidence id `2026-06-21-harbor-static-local-pv-velero-drill`; evidence id `2026-06-23-production-beta-secret-path-static-evidence`; context `default`; namespace `nexuspaas`; Harbor namespace `harbor-system`; Harbor chart `harbor-1.19.1`; Harbor app `2.15.1`; Velero chart `velero-12.0.3`; Velero app `1.18.1`; `postgres:16-alpine`; `minio/minio:RELEASE.2025-04-08T15-41-24Z`; OPS-009 stamp `20260621153156`; selected Postgres counts: `platform_records=387`, `platform_event_outbox=1193`, `users=0`, `org_project_records=0`, `workload_records=0` |
| Rollback | Partial — current live `nexuspaas` namespace has same-image Kubernetes controller rollback-path evidence for 15 first-party backend deployments: each completed serial `rollout restart`, `rollout status`, `rollout undo --to-revision=<pre_revision>`, final `rollout status`, final image equality, and final ready replicas equal desired replicas. Remaining GA rollback gaps: target 8-unit Production Beta staging rollback, previous-image rollback, schema-change rollback, backup/restore, and full external GA rollback evidence. | evidence id `2026-06-21-current-live-rollout-undo-evidence`; context `default`; namespace `nexuspaas`; all selected deployments final `1/1` ready |
| Failure injection | Partial — OPS-011 Redis/event-broker outage is evidenced live: Redis was scaled from `1` to `0`, a direct `storage-service` pod request returned HTTP `201`, exact `GroupStorageCreated` event `33fc697b-2cac-4715-ac04-e46097b0ea99` was durable in Postgres as `pending|0|false` during the outage, Redis was restored to `1/1`, natural relay-lease expiry was respected without deleting the lease, the event reached `published|0|true`, Redis DB1 retained it, and exact Postgres synthetic rows were cleaned. Partial OPS-013 Prometheus/telemetry evidence is also live: `usage-observability-service` image `ci-ga-prometheus-stale-20260621205458` returned telemetry metadata on `/api/v1/cluster/summary` and `/api/v1/projects/{id}/gpu-usage`, `/api/v1/cluster/mps` returned degraded Prometheus adapter status `adapter_not_configured`, and scheduler admission under trace `ga-prometheus-stale-20260621205959` rejected an over-quota synthetic request with HTTP `409` / `GPU quota exceeded` while Prometheus was not configured; all exact synthetic rows were cleaned. Harbor dependency/status API outage is also evidenced live: `HARBOR_URL` was added through 12-factor runtime config, `/api/v1/harbor-status` returned healthy before injection, `harbor-core` was scaled from `1` to `0`, the product API returned retryable degraded Harbor status with `degraded.code="adapter_unavailable"`, `harbor-core` was restored to `1/1`, and `/api/v1/harbor-status` returned healthy again without process refresh. OPS-012 image-registry build/list degraded behavior is now evidenced live: `image-registry-service` image `ci-ga-image-harbor-degraded-20260621211729` kept project image list, project build list, and build submission responses successful with unchanged local data while adding retryable Harbor degraded metadata during `harbor-core=0`; recovery returned no degraded envelope and exact synthetic cleanup left `cleanup_leftovers=0`. Full NexusPaaS image-build/allow-list/SBOM/signing/GUI scan workflow evidence and full OPS-019 still remain open for DB, K8s API, live Prometheus interruption, node usage-agent failure, and other fault domains. | Redis outage: evidence id `2026-06-21-redis-event-broker-outage-evidence`, trace `ga-redis-outage-20260621202250`, event `33fc697b-2cac-4715-ac04-e46097b0ea99`; Prometheus stale/quota: evidence id `2026-06-21-prometheus-stale-quota-evidence`, image `ci-ga-prometheus-stale-20260621205458`, trace `ga-prometheus-stale-20260621205959`; Harbor outage: evidence id `2026-06-21-harbor-outage-failure-injection`, trace `ga-harbor-outage-20260621200008`, configured URL `http://harbor.harbor-system.svc.cluster.local/api/v2.0/ping`; OPS-012 image-registry build/list outage: evidence id `2026-06-21-image-registry-harbor-degraded-build-list`, trace `ga-image-harbor-degraded-20260621212113`, image `ci-ga-image-harbor-degraded-20260621211729`, digest `sha256:21345ac6ad43db05f489bfa2ee37122b0b79873daca6f04619506c8bcef9d319` |
| `PERF` | Partial k6 evidence — the bounded `20` VU read smoke baseline passed after correcting endpoint selection (`30` requests per endpoint, `0` failures, total p95 `2306.00ms`) while preserving the earlier `/outbox` timeout discovery. A reusable harness exists for `PERF-001` Project list and `PERF-002` 100 concurrent users: `backend/scripts/perf/core-read-100vu.js` plus optional `make -C backend perf-k6-core-read`. Live smoke with `2` VUs / `3s` passed (`63` requests including preflight, failure rate `0`, p95 `/healthz=0.67ms`, `/readyz=1.03ms`, `/api/v1/projects=3.86ms`). Earlier live `100` VU attempts exposed the single-principal limiter problem, least-privilege scope/PDP tuple mismatches, downstream org-project static-key propagation, and remote PDP enforce `429` on `authorization-policy-service`. Those blockers are now fixed for the Project list path: image `ci-ga-pdp-enforce-20260622094936` rolled to all 15 backend deployments, `100` temporary exact-scope principals were patched into gateway and org-project only for the drill, exact raw policy rows were inserted/deleted, positive Project preflight returned `200`, negative Groups preflight returned `403`, and the live `100` VU / `30s` k6 run exited `0` with `auth_key_count=100`, `3102` total requests, failure rate `0`, total p95 `3.135ms`, `/api/v1/projects` `1000` requests / `1000` 2xx / `0` 4xx / `0` 5xx / `0` 429 / p95 `3.668ms`, and authorization-policy enforce `429` count `0`. Cleanup restored gateway/org-project Secrets to `API_KEYS=1` / `API_KEY_PRINCIPALS=1`, removed all `100` temporary policy rows, and left no port-forward. `PERF-001`/`PERF-002` Project-list read evidence is now passed. Local deterministic non-GPU `PERF-003` queue-state evidence now covers pending, admitted, preempted, and rejected workloads in `backend/internal/services/schedulerquota/queue_stress_test.go`; this is local policy/fake-client evidence only. Local deterministic `PERF-004` evidence now covers a large Group request-usage query with group filtering, since exclusion, unique user/project/group counts, and deterministic totals in `backend/internal/services/resourcehours/handler_test.go`; this is local in-memory evidence only. Local deterministic `PERF-005`/`USAGE-023` evidence now covers retained GPU snapshot metrics sanitizing process/PID/container identity keys while preserving bounded process aggregate counts and normalized GPU metrics in `backend/internal/services/gpuusage/collector_test.go`; this is local retained-storage evidence only and does not close live Prometheus retention/remote storage, real GPU PID attribution, full MON/USAGE/PERF, V1 external launch, or Full GA. Local deterministic `PERF-006`/`IMG-001`/`IMG-002`/`IMG-003` evidence now covers image build API admission for Project opt-in, required CPU/memory/time inputs, Project resource and time limits, active same-Project build concurrency, terminal status exclusion, and normalized queued build fields in `backend/internal/services/imageregistry/handler_test.go`; this is local in-memory API evidence only and does not close live Tekton/BuildKit/Harbor execution, live concurrent build load, timeout termination/quota release, SBOM/signing/scan enforcement, full IMG/PERF, V1 external launch, or Full GA. Local deterministic non-GPU `PERF-007`/`RTC-009`/`RTC-010`/`RTC-011` evidence now covers scheduler admission rejection for stream bitrate caps, active stream session caps, and active stream egress budgets, plus active non-streaming and terminal streaming exclusion from stream session/egress budgets in `backend/internal/services/schedulerquota/admission_test.go`; this is local policy evidence only and does not close browser WebRTC media, live egress traffic, real GPU/NVENC, RTC-014, RTC-017, full WebRTC GA, V1 external launch, or Full GA. Local deterministic non-GPU `PERF-008` evidence now covers workload dispatcher apply/create batch limiting per maintenance run, preserving due candidates for later runs and processing them on a subsequent fake-client run in `backend/internal/services/workload/dispatcher_test.go`; this is local dispatcher evidence only and does not close live K8s-control throughput, real API server QPS/latency, 8-unit staging, full K8S/PERF, V1 external launch, or Full GA. Live queue stress, live usage-query load, live Prometheus retention/remote storage, live build execution/load/timeout cleanup, browser media/live egress/GPU-NVENC for `PERF-007`, live K8s-control throughput/API-server QPS-latency for `PERF-008`, and DR RTO/RPO numbers remain open. | evidence id `2026-06-22-perf-live-baseline`; evidence id `2026-06-22-perf-k6-core-read-100vu`; evidence id `2026-06-22-perf-k6-multi-principal`; evidence id `2026-06-22-perf-live-multi-principal-secret-drill`; evidence id `2026-06-22-perf-live-project-list-policy-drill`; evidence id `2026-06-22-pdp-enforce-service-internal-rate-limit`; evidence id `2026-06-23-scheduler-quota-queue-stress-local-evidence`; evidence id `2026-06-23-perf004-large-group-usage-query-local-evidence`; evidence id `2026-06-23-perf005-process-metrics-cardinality-local-evidence`; evidence id `2026-06-23-perf006-image-build-quota-timeout-local-evidence`; evidence id `2026-06-23-perf007-stream-admission-egress-local-evidence`; evidence id `2026-06-23-perf008-dispatch-apply-batch-limit-local-evidence`; `docs/acceptance/performance.md` still unverified for full GA |

`PERF-004`/`PERF-005`/`PERF-006`/`PERF-007`/`PERF-008` tracker note: the large
Group request-usage evidence, retained GPU metrics sanitizer evidence, image
build API admission evidence, stream admission/egress scheduler evidence, and
dispatcher apply batch-limit evidence are local and deterministic only; full
`MON-007..009`, full `USAGE-023`, live usage-query load, live Prometheus
retention/remote storage, real GPU/per-device utilization and PID attribution,
live Tekton/BuildKit/Harbor execution, live concurrent build load, timeout
termination/quota release, SBOM/signing/scan enforcement, browser WebRTC media,
live egress traffic, real GPU/NVENC, RTC-014, RTC-017, live K8s-control
throughput, real API server QPS/latency, 8-unit staging, full K8S/PERF, V1
external launch, and Full GA remain open.

`MON-013..MON-017` tracker note: local deterministic non-GPU `/metrics`
evidence now covers workload queue pending/running/preempted/rejected counts,
image build running/failed/succeeded/timeout counts, WebRTC active sessions and
egress bitrate, ConfigFile admission rejection reasons, and Kubernetes apply
failure reasons in `backend/internal/platform/observability_test.go` and
`backend/internal/services/workload/handler_test.go`. This is local in-memory
metrics evidence only; live Prometheus scrape/retention/alerting, dashboards,
full MON/OPS/PERF, WebRTC media/live egress/GPU-NVENC, V1 external launch, and
Full GA remain open.

(K8S manifest size/document cap — **done** via `GATE-*`.)

2026-06-22 stream credential update: WEB-006 credential issuance through the
first-party GUI is now evidenced on
`ci-ga-web-stream-cred-20260622102018`
(`sha256:d14aa360d5f0e4273846c88a785a2ad8cafc570613e8d892a7d9ef4407c899b1`).
Live Playwright submitted streaming Job `e2e-job-mqom1t1b-pa2jbl` and proved
`stream_credentials_status=200`, `stream_credential_uri_count=1`,
`stream_credential_username_present=true`, and
`stream_credential_password_issued=true`; the GUI proof records
`stream_credential_password_redacted=true`. The approved WebRPC GUI contract
remains same-origin REST/OpenAPI for GA v1. This does not close full WebRTC
media session, real GPU-node streaming, or stream metrics evidence.

2026-06-23 Selkies sidecar redesign: Selkies no longer ships as a baked
standalone desktop image that users must rebuild into their app. A
`streaming_session` job now keeps the user's own app image, and workload
dispatch auto-injects the `selkies` sidecar (from `STREAM_SIDECAR_IMAGE`), the
shared `/tmp/.X11-unix` + `/dev/shm` volumes, `DISPLAY=:0` on the app
container(s), and signaling/metrics ports; the existing DRA step then wires app
and sidecar to one shared MPS claim. Injection is idempotent and submit is
rejected when `STREAM_SIDECAR_IMAGE` is unset. Unit-evidenced in
`backend/internal/services/workload/dispatcher_streaming_test.go` and
`config_test.go`. Remaining gap unchanged: live WebRTC/NVENC/forced-TURN relay
stays operator-verified on GPU hardware; `stream_resolution`/`stream_fps`/
`stream_idle_timeout_seconds`/`allow_webrtc` remain target-only.

2026-06-22 RTC credential-safety update: RTC-006/RTC-007 now have focused
backend test evidence. `stream_credentials_test.go` verifies TTL cap/default
behavior, RFC3339 `expires_at` windows, username expiry prefix matching,
HMAC-derived password generation, password not equal to the shared secret, and
serialized response non-disclosure of the shared secret. This remains credential
evidence only, not media proof.

2026-06-22 RTC-008 update: direct ICE and forced TURN relay candidate gathering
now have current live RKE2/staging GUI route-proof evidence. Seeded streaming
Job `e2e-job-mqongkuq-oov6qe` requested stream credentials through `/ui/`; the
same proof recorded `rtc_probe_environment="staging"`, `rtc_direct_ok=true`,
`rtc_direct_candidate_count=2`, `rtc_direct_candidate_types=["host"]`,
`rtc_relay_ok=true`, `rtc_relay_candidate_count=1`, and
`rtc_relay_candidate_types=["relay"]`. Runtime TURN URI config was temporarily
changed to browser-reachable `turn:127.0.0.1:3478?transport=udp` for the probe,
then restored to `turn:coturn.nexuspaas.svc.cluster.local:3478?transport=udp`.
Secret values were not printed. The same run left `job_logs_count=0`,
`job_logs_visible=false`, `gpu_status=502`, and `gpu_nonzero=false`, so real
workload log visibility and the GPU route required separate follow-up.

2026-06-22 GPU read-model update: WEB-007 active-Project GPU usage now has
live nonzero requested-GPU pod visibility evidence on
`ci-ga-gpu-readmodel-20260622034034`
(`sha256:2f0ebfc868a26fb59a9b3d20194756a9f8e2917b61397d50d80a16c9cde840c7`).
`usage-observability-service` writes Kubernetes platform job-pod GPU request
rows through the existing `ListJobPodResourceUsage` adapter, the Project GPU
route counts exact `project_id` rows with legacy namespace fallback, and the
GUI E2E harness polls bounded readiness when
`NEXUSPAAS_E2E_EXPECT_NONZERO_GPU=true`. Seeded Project
`e2e-p-mqooctn3-fammye` used fixture pod `gpu-proof` in namespace
`gpu-e2e-p-mqooctn3-fammye` with `nvidia.com/gpu: "1"` request; route
readiness reached `status=200 used=1`, and Playwright recorded
`gpu_status=200`, `gpu_ok=true`, `gpu_used=1`, and `gpu_nonzero=true`.
Cleanup proved the Project was gone, no `nexuspaas-e2e=gpu-proof` namespaces
remained, and the temporary collector interval override was removed. This
proves requested-GPU pod visibility, not per-device utilization or DCGM/MPS
telemetry.

2026-06-22 job-log update: WEB-004 bounded Kubernetes pod-log visibility now
has live non-empty GUI/API evidence on
`ci-ga-job-logs-nonempty-fix-20260622130645`
(`sha256:fdb674beaf60e1ea052a7cbc974263b5c9fee4d39927c5980c12feb48ff2cc7e`).
The workload Job logs route appends bounded Kubernetes pod logs after
authorization through the existing cluster adapter, with `TailLines=200`,
`LimitBytes=65536`, and namespace-scoped `platform-go/job-id` fallback. Seeded
Project `e2e-p-mqora84n-1y46vp` used fixture pod `log-proof` in namespace
`proj-e2e-p-mqora84n-1y46vp`; Playwright ran with
`NEXUSPAAS_E2E_EXPECT_NONEMPTY_LOGS=true` and recorded
`job_logs_status=200`, `job_logs_count=1`, `job_logs_nonempty=true`, and
`job_logs_visible=true`. Cleanup deleted the proof namespace and temporary
build/tune pods, and restored host inotify settings. This proves bounded
non-empty pod-log retrieval, not continuous tailing or a full workload status
workflow.

2026-06-22 Web UI polling update: WEB-004 continuous log/status polling is now
strengthened in `frontend/src/App.tsx` with bounded REST polling of the
selected Job's logs plus the existing active-Project workload list. Focused
frontend evidence passed with `npm --prefix frontend run test -- src/App.test.tsx`;
the test covers immediate log fetch, timer polling, job-status refresh,
failure retry, second-Job cleanup, Project switch cleanup, unmount cleanup, and
token-safe inline errors. `npm --prefix frontend run build` also passed. This
is frontend/local evidence only: Docker/live E2E, WebSocket/SSE tailing, full
workload lifecycle status, full WEB-001..007, Full GA, and first-version
completion remain unclaimed.

2026-06-22 Web UI usage update: WEB-007 frontend usage workflow is now
strengthened in `frontend/src/App.tsx` with active-Project-filtered
`/api/v1/me/usage` and `/api/v1/me/request-usage` tables, the existing
`/api/v1/projects/{projectID}/gpu-usage` Project GPU pods summary, compact
visible row/resource totals, and a Usage-local manual refresh button that
re-runs only those three usage calls. Focused frontend evidence passed with
`npm --prefix frontend run test -- src/App.test.tsx`; the test covers active
Project filtering/totals, manual refresh route counts, GPU-route failure
isolation with non-secret error text, no admin usage fallback, and no
`localStorage`/`sessionStorage` credential persistence. `npm --prefix frontend
run build` also passed. This is frontend/local evidence only: Docker/live E2E,
real per-device GPU utilization, full usage attribution GA, full WEB-001..007,
Full GA, and first-version completion remain unclaimed.

2026-06-22 performance update: stream credential issuance met the credential
p95 sub-target with k6: `100` temporary principals, `100` VUs for `30s`,
`/api/v1/stream/credentials` `3000/3000` 2xx, failure rate `0`, p95
`22.926812599999987ms`, and `0` 429/4xx/5xx. Exact temporary policy rows,
runtime Secret patches, and seed data were cleaned.

## 3. Architecture blockers backing DATA / SEC / OPS acceptance

Cross-referenced with [`problem.md`](problem.md):

| Blocker | Status |
|---|---|
| Transactional outbox/inbox | Done for current delivery evidence — tables + relay + inbox dedupe wired in `runtime.go`; single-record coupling (`App.*RecordWithEvent` / `App.UpsertRecordWithEvent`) across 7 services and multi-record coupling (`App.WithTx`/`StoreTx`/`RunInTx`) for authorizationpolicy non-batch and batch assignment/role/raw-permission mutations, schedulerquota plan/queue batch deletes and successful preemption `JobPreempted`, storage permission batches and non-batch cascades, identity user batch reset/role/delete paths, orgproject membership/project-member/quota/workspace/GPU/plan-binding paths, imageregistry catalog sync/publish/unpublish/delete, workload submit/cancel/config commit/instance command, and orgproject group/project delete cascades. Reviewer-approved live evidence: final image `localhost:5000/nexuspaas-backend:ci-ga-pdp-scope-20260620163744`, 15 backend deployments ready, PDP service-key scope fixed, HTTP 201 form create, matching `FormCreated` durable outbox row, smoke data cleaned, representative storage events `940df12e-f953-4460-bc06-3aa487209016`, `6c749a52-a980-4d9f-9f6b-834f5f6e0068`, and `a70177d7-f0da-48a8-84f3-44bee283a54f` published with `relay_attempts=0` and appeared in Redis DB1, and synthetic `ga-outbox-crash-20260621103919` stayed pending during a short sentinel relay hold, then published after relay-capable pod restart plus sentinel release and was retained in Redis DB1. Exact cleanup left zero synthetic Postgres outbox rows. This proves controlled relay unavailability plus relay-capable pod restart recovery; it does not prove handler mid-transaction crash interleavings or the exact restarted pod as publisher. |
| Typed-domain data ownership | Started |
| API-token indexed lookup | Done — token id is parsed from `nexuspaas_<token-id>_<secret>` and verification loads one indexed record; focused/full tests, quick gate, Sonar Quality Gate, reviewer approval, and live RKE2 auth evidence passed |
| Centralized trusted-IP resolver | Done — identity failure/captcha/API-token audit paths reuse the trusted-proxy resolver; focused/full tests, quick gate, Sonar Quality Gate, reviewer approval, and live spoofed-header evidence passed |
| Env profiles + PDP fail-closed | Done — explicit `APP_ENV` profiles, conflict validation, staging/production strict startup checks, production manifest profile declarations, focused/full tests, quick gate, Sonar Quality Gate, reviewer approval, and live RKE2 health/readiness evidence passed |
| Service identity / JWT-JWKS lib / migration-runner maturity | Scoped internal service identity slice done for v1; JWT/JWKS library-verifier slice done with `github.com/coreos/go-oidc/v3` replacing production custom JWK parsing and RSA/ECDSA JWT signature verification while preserving multi-audience, one-minute skew, `jti`, and role/user mapping behavior; migration runner ledger/checksum/advisory-lock/dirty-state code slice implemented with focused/full backend tests, DB-free `validate-migrations` evidence, and live PostgreSQL integration evidence for temporary-schema isolation plus dirty/checksum/adoption/lock behavior through redacted `platform-gateway-runtime-secret:DATABASE_URL`; service credential rotation/workload identity/mTLS, live staging migration drill, and full schema rollback maturity remain open |

## 4. Live cutover caveat

The V1 launch gate's "Live staging online" pass refers to this local RKE2 /
15-deployment / `localhost:5000` evidence only; external production launch
readiness is OPEN (see the First Version (V1) Status block above).

Recorded "live staging" evidence used a `localhost:5000` registry and rolled
**15 single deployments** — **not** the GA **8-unit topology** with a real
external registry. OPS "every deployable unit deployed, observed, rolled back,
recovered" is therefore unproven in its external GA form. Current live
15-deployment same-image rollout/undo evidence lowers controller-path risk, but
does not replace target 8-unit staging rollback or previous-image rollback.
Harbor now exists as an isolated ClusterIP foundation in `harbor-system`, and the
local OPS-007 Harbor backup/restore drill has passed on static Kubernetes
`local` PVs. Harbor-side push/scan/delete has also passed locally with Trivy.
Catalog-derived image scan/deleted status now has API and `/ui/` display
evidence, and bounded Harbor-to-catalog synchronization plus explicit per-tag
delete-resync lifecycle are locally proven.
Harbor is still not yet the external GA registry and has not been used for
external image promotion, rollback, or 8-unit release evidence.

## 5. Deferred (acknowledged, not GA-blocking by decision)

- NOTIF delivery guarantees (best-effort acceptable).
- IDE dedicated ACs (covered by generic ConfigFile + stream path unless IDE ships).
- i18n / accessibility (product polish).
- Billing (explicitly future).
- Web UI (`WEB-*`) is out of V1 scope (API/CLI-first); not a V1 blocker.
  Required only before a future Web UI launch.
