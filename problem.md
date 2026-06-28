# NexusPaaS Current Architecture Blockers

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
  references in source/render only. Remote PR #33 evidence now shows external
  SonarCloud Code Analysis and Backend Quality Gate passing; that evidence does
  not close live P0.2-P0.5 or V1 external production launch.
- **Web UI (`WEB-*`) is out of V1 scope** (API/CLI-first). The existing
  `frontend/` GUI is beta/future; `WEB-*` is required only before a future Web
  UI launch.
- Throughout this file, **"first-version readiness/completion" in slice
  disclaimers means the V1 *external production launch* state above (OPEN)** — it
  does **not** mean the individual slice is unfinished.

_Updated: 2026-06-24 (re-verified, scheduled backend gap/code review;
independently re-confirmed this pass — `go build ./...`, `go vet ./...`, and
full `go test ./...` all green across all 24 packages on the current working
tree, including uncommitted PERF/MON/DATA-014/IMG work; cron parity re-checked
file-by-file vs `references/CSCC_AI_Platform_Backend/internal/cron` — 15
reconcilers ported, `course_monitoring_reconciler` deliberately out of scope per
ADR 0006; spot-checked the largest uncommitted diffs (schedulerquota
`preemption.go` public/internal id separation, imageregistry `handler.go`, no
error swallowing or panics) — no local code-quality regression surfaced).
Branch: `feature/ga-gap-clearance`. This pass re-ran the local toolchain on the
current working tree (incl. uncommitted PERF-003..008, MON-013..017, partial
DATA-014, IMG-011/012/013, image-build create contract fixtures, the new
`monitoring_metrics.go` low-cardinality metric source, and the new local
schedulerquota `queue_stress_test.go`): `go build
./...` clean, `go vet ./...` clean, and full `go test ./...` green across all 24
packages (`cmd/microservice`, `internal/platform`, `internal/services`, and
every service package). Reference parity vs `references/CSCC_AI_Platform_Backend`
re-checked file-by-file this pass: every `internal/cron` reconciler is ported —
priority_class (`schedulerquota/priority_class_sync.go` + `cluster/priority_class.go`),
longhorn_rwx (`storage/longhorn_rwx_health.go` + `cluster/longhorn_rwx.go`),
docker_cleanup (`k8scontrol/docker_cleanup.go`), gpu_usage
(`gpuusage/collector.go`), vpn_usage (`integrationproxy/vpn_usage_collector.go`),
cluster_resources (`clusterread/cluster_resource_collector.go`), ldap_mirror
(`identity/ldap.go`), user_delete_cleanup (`identity/cleanup.go`),
resource_hours (`resourcehours`), resource_quota (`schedulerquota`),
harbor_health, idle_reaper, plan_window_reaper, workload_runtime_reaper, and
policy_data_sync (`authorizationpolicy/policy_data_sync.go`) — all registered via
the per-service maintenance-task framework (`internal/platform/maintenance.go` +
`RegisterMaintenanceTaskForService`); the reference course_monitoring_reconciler
is the only one deliberately out of scope (ADR 0006). No new untracked reference
capability surfaced this pass. New this pass: a local deterministic schedulerquota
queue-stress test now exercises pending/admitted/preempted/rejected workloads
(`queue_stress_test.go`); this is local evidence only — live queue stress under
real scheduler/Kubernetes load remains an open P0/PERF item. All remaining open items
are the already-tracked live-execution P0 blockers (external Harbor registry,
8-unit staging deploy/rollback, live staging DB migration/rollback drill, live
external Secret readiness) plus the GA family tail — none are local code-quality
regressions. Prior basis retained below. Branch `ga-web-gate`. Full
`go test ./...`, quick gate, coverage run, focused transactional batch/per-item
tests, local SonarScanner Quality Gate, and live RKE2 outbox/PDP/Web GUI
active-Project seeded image-build plus job submit/cancel/log-route smoke
evidence plus WEB-001 OIDC browser-login Playwright evidence on
`ci-ga-web-oidc-20260621203712`
(`sha256:a4cc30f2f8b6b8b949c47949de186023ba61a8ad78ea52e72b927e17ea1d670b`)
plus current-live 15-deployment same-image rollout/undo evidence are green.
OPS-006 PostgreSQL logical backup/restore drill, OPS-008 MinIO
synthetic object restore drill, and OPS-009 current-live Kubernetes Secret
recovery copy drill are also evidenced. Latest Sonar API status:
`new_coverage=81.8`,
`new_violations=0`, `new_duplicated_lines_density=0.8262`. Live Harbor
foundation deploy and credential rebaseline are also evidenced with official
chart `harbor-1.19.1` / app `2.15.1`. Velero `12.0.3` / app `1.18.1` is
installed and healthy. OPS-007 Harbor backup/restore has passed on Kubernetes
static `local` PVs with completed non-Redis PodVolumeBackups/PodVolumeRestores,
namespace/PV deletion, restored Harbor readiness, and ORAS artifact
digest/payload verification. Harbor-side image push/scan/delete also has live
evidence through official Trivy and a real `busybox:1.36` image copied by
`crane`, with scan status `Success`. Harbor dependency outage evidence also
passed through `/api/v1/harbor-status` with trace
`ga-harbor-outage-20260621200008`, and OPS-012 image-registry build/list
degraded-route outage evidence passed with trace
`ga-image-harbor-degraded-20260621212113` on image
`ci-ga-image-harbor-degraded-20260621211729`
(`sha256:21345ac6ad43db05f489bfa2ee37122b0b79873daca6f04619506c8bcef9d319`).
WEB-005 catalog-derived Project image status display also has live API and
Playwright GUI evidence with trace `ga-web-image-status-20260621214849` on
image `ci-ga-web-image-status-20260621214330`
(`sha256:613c1f2cefbab9f029afc580213dc619e7892753826c02367fa6ea6be129f83e`),
proving top-level `scan_status="Success"` and visible state `deleted` for a
seeded Project image. WEB-005 / IMG-024 frontend image build submission now has
focused local evidence: `frontend/src/App.test.tsx` covers
`POST /api/v1/images/build/dockerfile` with the active Project id and trimmed
`image_reference`, success refresh of Project images/builds, no `/admin`
fallback, no browser storage persistence, and generic secret-safe submit
failure text. This is frontend REST workflow evidence only; it does not prove
live Harbor build execution, SBOM/signing, allow-list enforcement, or full
image workflow GA. Image-registry image build create routes now also have
local/static typed external REST fixture coverage for `POST /api/v1/images/build`,
`POST /api/v1/images/build/from-storage`, and
`POST /api/v1/images/build/dockerfile`: the fixtures use `202 Accepted`, require
`project_id`, `image_reference`, and resource fields, emit only
`ImageBuildStarted`, and are checked against `imageregistry.Spec()` for auth,
route, path params, state-changing, and `harbor` adapter metadata. This is
static contract evidence only; it does not prove live Harbor build execution,
SBOM/signing, allow-list enforcement, image scan lifecycle, or full image
workflow GA. Image-registry image acceleration and queued supply-chain metadata
now also have local evidence: `ImageAccelerationProfile` admin CRUD, seeded
defaults, create fixture, and `ImageAccelerationProfileChanged` event fixture
exist, and queued image builds expose pending supply-chain status fields in the
create response, stored record, and `ImageBuildStarted` event while historical
payloads without those additive fields remain schema-v1 compatible. This is
metadata/event-shape evidence only and does not prove image conversion/prewarm,
completed SBOM generation, signing, scan enforcement, allow-list admission, live
Harbor/Tekton/BuildKit execution, or full image workflow GA.
Scheduler accelerator, network, and placement profile creation now also have
local/static external REST fixture parity against `schedulerquota.Spec()` for
the three existing profile create fixtures and matching service event names.
This is static contract evidence only and does not prove live scheduler/profile
behavior, Kubernetes placement, full typed API coverage, V1 external launch, or
Full GA. Authorization-policy proxy role create/update/delete now also have
local/static external REST fixture parity against `authorizationpolicy.Spec()`
for admin route metadata, authenticated-user/no-service-key posture,
path params, success/error statuses, and `ProxyPolicyChanged` linkage; the
event fixture uses role-map public fields plus `action`. This is static
contract evidence only and does not prove live admin authorization, live proxy
role mutation behavior, full typed API coverage, V1 external launch, or Full
GA. Authorization-policy proxy policy create/update/delete now also have
local/static external REST fixture parity against `authorizationpolicy.Spec()`
for admin route metadata, authenticated-user/no-service-key posture,
path params, partial-update optional request fields, success/error statuses,
and `ProxyPolicyChanged` linkage; the existing `ProxyPolicyChanged` event
fixture is reused. Typed API coverage remains Open, and this does not prove
live admin authorization, live proxy policy mutation behavior, DATA GA, V1
external launch, or Full GA. Authorization-policy proxy service list/get now
also have local/static external REST fixture parity against
`authorizationpolicy.Spec()` for admin route metadata,
authenticated-user/no-service-key posture, path params, read-only/no-event
behavior, success/error statuses, and service-row response examples. Typed API
coverage remains Open, and this does not prove live admin authorization, live
proxy service behavior, DATA GA, or Full GA.
Authorization-policy proxy role-user list/assign/unassign now also have
local/static external REST fixture parity against `authorizationpolicy.Spec()`
for admin route metadata, authenticated-user/no-service-key posture,
path params, delete route `IDParam` `user_id`, read-only/no-event list
behavior, assign/unassign `ProxyPolicyChanged` linkage, success/error statuses,
and role-user response examples with nested public role data. Typed API
coverage remains Open, and this does not prove live admin authorization, live
role-user mutation behavior, DATA GA, V1 external launch, or Full GA.
Authorization-policy proxy policy assignment list/assign/unassign now also
have local/static external REST fixture parity against
`authorizationpolicy.Spec()` for admin route metadata,
authenticated-user/no-service-key posture, path params, read-only/no-event list
behavior, assign/unassign `ProxyPolicyChanged` linkage, success/error statuses,
`target_type`/`target_id` request examples, assignment response examples with
nested public policy data, and unassign's empty response without a `404` error
status. Typed API coverage remains Open, and this does not prove live admin
authorization, live proxy policy assignment mutation behavior, DATA GA, V1
external launch, or Full GA. Image build create and cancel routes now also have partial local
deterministic `DATA-014` evidence for optional `Idempotency-Key` replay/conflict
behavior: create covers same-request replay and changed-request conflict, while
cancel covers same-target replay across both cancel route aliases and
different-target conflict without duplicate `ImageBuildCancelled` events. This
is local image-build command evidence only and does not prove full DATA-014,
deploy idempotency, live executor cancellation, live Harbor/Tekton/BuildKit or
Kubernetes execution, full IMG/DATA, V1 external launch, or Full GA. Identity
auth/session routes now also have local/static typed external REST fixture
coverage for `POST /api/v1/register`, `POST /api/v1/login`,
`POST /api/v1/refresh`, and `POST /api/v1/cli/login`: the fixtures declare
public auth posture, exact required credential fields, success statuses, and
`UserCreated` only for registration, with credential-shaped example allowances
scoped to those four identity fixtures and checked against `identity.Spec()`.
This is local/static contract evidence only and does not prove live auth
availability, browser cookie behavior, OIDC/LDAP behavior, token
rotation/revocation, all-critical API typed coverage, DATA GA, Full GA, or V1
external launch readiness. Workload submit now also has partial local
deterministic `DATA-014` evidence for
`POST /api/v1/jobs`: same-key same-semantic-payload replays return the existing
submitted job without duplicate scheduler admission, auto-preemption, job record,
or `JobSubmitted` event side effects, and same-key different-payload attempts
return `409 Conflict` before those side effects. This is local workload submit
evidence only and does not prove full DATA-014, deploy idempotency, live
Kubernetes apply, full workload GA, V1 external launch, or Full GA. Workload
cancel now also has partial local deterministic `DATA-014` evidence
for `POST /api/v1/jobs/{id}/cancel`: same-key same-semantic-command replays,
including canonical-equivalent job identifiers, return the existing accepted
cancel command without duplicate command record or `JobCancelRequested` event
side effects, and same-key different-target attempts return `409 Conflict`
before those side effects. The workload cancel fixture now lists only the
optional `Idempotency-Key` header name, and internal cancel matching metadata is
stripped from cancel responses and event payload data. This is local workload
cancel evidence only and does not prove full DATA-014, deploy idempotency, live
scheduler/Kubernetes cancellation, GPU evidence, full workload GA, V1 external
launch, or Full GA. Scheduler explicit preemption now also has partial local
deterministic `DATA-014` evidence for scheduler preemption: same-key
same-request replays return the existing sanitized decision without duplicate
cleanup, workload transition, or `JobPreempted` event side effects, and same-key
different-requester or different-payload attempts return `409 Conflict` before
victim selection, cleanup, workload preempt, or event emission. Responses,
workload preempt calls, and event payload data use a generated public
`preemption_id`, while key-derived store record IDs and internal
key/fingerprint hashes remain private. This is local scheduler/workload
preemption evidence only and does not prove full DATA-014, deploy idempotency,
live scheduler/Kubernetes preemption, live GPU behavior, full scheduler/workload
GA, V1 external launch, or Full GA. Image build
logs now also have local deterministic `IMG-011`
response redaction evidence for common Authorization bearer and secret-like
key/value patterns while preserving ordinary log lines and stored logs; this
does not prove live Harbor/Tekton/BuildKit logs, streaming/tailing,
SBOM/signing/scan, or full image workflow GA. Image build cancellation and
timeout terminal states now also have local deterministic `IMG-012`/`IMG-013`
active-build slot release evidence; this does not prove live resource
termination, a real timeout controller, executor/Kubernetes quota release,
live Harbor/Tekton/BuildKit, SBOM/signing/scan, GPU closure, or full image
workflow GA. Workload job
submission now also has local/static typed external REST fixture
coverage for `POST /api/v1/jobs`: the fixture uses `201 Created`, requires
`project_id` and `user_id`, keeps queue/resource/config/streaming fields as
optional UI/admission/defaultable payload fields, emits only `JobSubmitted`,
and is checked against `workload.Spec()` for auth, route, path params,
state-changing, success status, and event metadata. This is static contract
evidence only; it does not prove live scheduler admission, queue policy
completeness, Kubernetes job execution, logs/tailing, GPU telemetry,
WEB-003/WEB-004 completion, or full workload GA. Workload ConfigFile creation
now also has local/static typed external REST fixture coverage for
`POST /api/v1/configfiles`: the fixture uses `201 Created`, requires
`project_id` and `name`, keeps accepted aliases/payload fields optional, emits
`ConfigFileChanged` as handler-emitted create evidence, and is checked against
`workload.Spec()` for auth, route, no service key/path params, non-admin
state-changing behavior, success status, and source-backed error statuses. This
is static contract evidence only; it does not prove live scheduler admission,
Kubernetes job execution, logs/tailing, GPU telemetry, WEB-003/WEB-004
completion, DATA GA, Full GA, or first-version readiness.
Workload ConfigFile deletion now also has local/static typed external REST
fixture coverage for `DELETE /api/v1/configfiles/{id}`: the fixture uses empty
`{}` body, `id` path parameter, `200 OK`, emits `ConfigFileChanged`, models the
direct delete response `{"id":"config-ga-001","deleted":true}`, and is checked
against `workload.Spec()` for auth, route, ID param, no service key, non-admin
state-changing behavior, success/error statuses, empty-body semantics, response
shape, and event metadata. This is static contract evidence only; it does not
prove live ConfigFile deletion, project isolation, event delivery, scheduler
admission, Kubernetes job execution, ConfigFile runtime rollout, WEB-003/WEB-004
completion, DATA GA, Full GA, or first-version readiness.
Workload ConfigFile update now also has local/static typed external REST
fixture coverage for `PUT /api/v1/configfiles/{id}`: the fixture uses `200 OK`,
requires `content`, carries `id` as the only path parameter, keeps
`name`/`filename`/`path`/`manifest`/`yaml`/`config` and same-Project
`projectId`/`project_id` aliases optional, emits `ConfigFileChanged`, models
the updated ConfigFile record response, and is checked against `workload.Spec()`
for auth, route, ID param, no service key, non-admin state-changing behavior,
success/error statuses, response shape, and event metadata. `ConfigFileChanged`
is also listed in `workload.Spec().Events` to match existing handler emission.
The optional project aliases do not imply cross-Project moves are supported;
the handler rejects those with `400`. This is static contract evidence only; it
does not prove live ConfigFile update, project isolation, event delivery,
ConfigFile runtime rollout, WEB-003/WEB-004 completion, DATA GA, Full GA, or
first-version readiness.
Workload ConfigFile PATCH update now also has local/static typed external REST
fixture coverage for `PATCH /api/v1/configfiles/{id}`: the fixture uses
`200 OK`, requires `content`, carries `id` as the only path parameter, keeps the
same optional update fields as the PUT fixture, emits `ConfigFileChanged`,
models the updated ConfigFile record response, and is checked against
`workload.Spec()` for auth, route, ID param, no service key, non-admin
state-changing behavior, success/error statuses, response shape, and event
metadata. Its request example omits `project_id` and `projectId`, so it does
not imply cross-Project ConfigFile moves. This is static contract evidence
only; it does not prove live ConfigFile PATCH update, project isolation, event
delivery, ConfigFile runtime rollout, WEB-003/WEB-004 completion, DATA GA, Full
GA, or first-version readiness.
Workload ConfigFile read now also has local/static typed external REST fixture
coverage for `GET /api/v1/configfiles/{id}`: the fixture uses empty `{}`
request body, `id` path parameter, `200 OK`, emits no events, models the public
ConfigFile record response, and is checked against `workload.Spec()` for auth,
route, ID param, no service key, non-admin read-only behavior, success/error
statuses, empty-body semantics, response shape, and no events. This is static
contract evidence only; it does not prove live ConfigFile reads, project
isolation, event delivery, ConfigFile runtime rollout, WEB-003/WEB-004
completion, DATA GA, Full GA, or first-version readiness.
Workload job cancellation now also has local/static typed external REST fixture
coverage for `POST /api/v1/jobs/{id}/cancel`: the fixture uses empty `{}` body,
`id` path parameter, `202 Accepted`, emits `JobCancelRequested`, models the
returned command record, and is checked against `workload.Spec()` for auth,
route, ID param, no service key, non-admin state-changing behavior,
success/error statuses, empty-body command semantics, response shape, and event
metadata. `JobCancelRequested` is also listed in `workload.Spec().Events` to
match the existing handler emission. This is static contract evidence only; it
does not prove live scheduler cancellation, Kubernetes job termination,
cancellation propagation, logs/tailing, GPU telemetry, WEB-004 completion, DATA
GA, Full GA, or first-version readiness.
Workload ConfigFile version commit now also has local/static typed external
REST fixture coverage for `POST /api/v1/configfiles/{id}/versions`: the
fixture uses `201 Created`, requires `content`, carries `id` as the only path
parameter, preserves optional `message`/`manifest`/`yaml`/`config` fields, emits
`ConfigCommitted`, models the returned immutable version record with
`config_id`, `content`, `message`, `sha256`, and `committed_at`, and is checked
against `workload.Spec()` for auth, route, ID param, no service key, non-admin
state-changing behavior, success/error statuses, response shape, and event
metadata. This is static contract evidence only; it does not prove live
scheduler admission, Kubernetes job execution, ConfigFile runtime rollout,
logs/tailing, GPU telemetry, WEB-003/WEB-004 completion, DATA GA, Full GA, or
first-version readiness.
Org-project Project creation now also has local/static typed external REST
fixture coverage for `POST /api/v1/projects`: the fixture uses `201 Created`,
requires `project_name` and `g_id`, keeps conservative source-backed aliases
and policy/quota fields optional, emits only `ProjectCreated`, and is checked
against `orgproject.Spec()` for auth, route, no service key/path params,
non-admin route metadata, state-changing behavior, success/error statuses, and
event metadata. This is static contract evidence only; it does not prove live
admin authorization, full Project lifecycle, tenant isolation, DATA GA, Full
GA, or first-version readiness. Org-project Project update now also has
local/static typed external REST fixture coverage for `PUT /api/v1/projects/{id}`:
the fixture uses `200 OK`, requires `project_name`, keeps source-backed mutable
Project fields optional, emits only `ProjectUpdated`, and is checked against
`orgproject.Spec()` for auth, route, `id` path param, admin route metadata,
state-changing behavior, success/error statuses, direct Project response shape,
and event metadata. This is static contract evidence only; it does not prove
live admin authorization, full Project lifecycle, tenant isolation, DATA GA,
Full GA, or first-version readiness. Org-project Project delete now also has
local/static typed external REST fixture coverage for `DELETE /api/v1/projects/{id}`:
the fixture uses `200 OK`, has no required or optional request fields, keeps
empty request/response examples, emits only `ProjectDeleted`, and is checked
against `orgproject.Spec()` for auth, route, `id` path param, non-admin route
metadata, state-changing behavior, success/error statuses, and event metadata.
This is static contract evidence only; it does not prove live admin
authorization, full Project lifecycle, tenant isolation, DATA GA, Full GA, or
first-version readiness. Org-project Group creation now also has
local/static typed external REST fixture coverage for `POST /api/v1/groups`:
the fixture uses `201 Created`, requires `group_name`, keeps conservative
source-backed aliases and policy fields optional, emits only `GroupCreated`,
and is checked against `orgproject.Spec()` for auth, route, no service key/path
params, admin route metadata, state-changing behavior, success/error statuses,
direct group response shape, and event metadata. This is static contract
evidence only; it does not prove live admin authorization, full Group
lifecycle, tenant isolation, DATA GA, Full GA, or first-version readiness.
Org-project Group update now also has local/static typed external REST fixture
coverage for `PUT /api/v1/groups/{id}`: the fixture uses `200 OK`, requires
`group_name`, keeps source-backed mutable Group fields optional, emits only
`GroupUpdated`, and is checked against `orgproject.Spec()` for auth, route,
`id` path param, admin route metadata, state-changing behavior, success/error
statuses, direct group response shape, and event metadata. `GroupUpdated` is
also now listed in `orgproject.Spec().Events` to match the existing handler
emission. This is static contract evidence only; it does not prove live admin
authorization, full Group lifecycle, tenant isolation, DATA GA, Full GA, or
first-version readiness. Org-project Group delete now also has local/static
typed external REST fixture coverage for `DELETE /api/v1/groups/{id}`: the
fixture uses `200 OK`, has no required or optional request fields, keeps empty
request/response examples, emits only `GroupDeleted`, and is checked against
`orgproject.Spec()` for auth, route, `id` path param, admin route metadata,
state-changing behavior, success/error statuses, empty response shape, and
event metadata. `GroupDeleted` is also now listed in `orgproject.Spec().Events`
to match the existing handler emission. This is static contract evidence only;
it does not prove live admin authorization, full Group lifecycle, tenant
isolation, DATA GA, Full GA, or first-version readiness.
Org-project Group batch delete now also has local/static typed external REST
fixture coverage for `DELETE /api/v1/groups/batch`: the fixture uses `200 OK`,
requires top-level `ids`, keeps direct `succeeded`/`failed`/`errors` response
shape, emits only `GroupDeleted`, and is checked against `orgproject.Spec()` for
auth, route, admin route metadata, state-changing behavior, no service key/path
params, success/error statuses, canonical group IDs, and event metadata. This is
static contract evidence only; it does not prove live admin authorization, full
Group lifecycle, tenant isolation, DATA GA, Full GA, or first-version readiness.

Storage mount-plan authorization and isolation now have local/in-memory
`STORAGE-001` and `STORAGE-002` proof: direct resolver tests cover
storage-owned project binding lookup, dispatch-ready group storage sources,
effective PVC permission, project-level `read_only` precedence over group
`read_write` for writable mount denial, unrelated Project binding rejection,
and other-user permission denial. This is mount-plan authorization/isolation
proof only; it does not prove live Kubernetes mount execution, cluster PVC
isolation, CSI behavior, full storage GA, Full GA, or first-version readiness.

Storage permission management now has local handler-level `STORAGE-003` RBAC
proof: direct handler tests cover plain group member / Project reader denial
for direct create/set, batch set, and batch delete group/project storage
permission rows, with assertions that denied creates/sets do not create target
rows and denied deletes leave seeded rows intact. This does not prove live
Kubernetes PVC isolation, namespace enforcement, full storage GA, Full GA, or
first-version readiness.

Storage project binding creation now also has local/static typed external REST
fixture coverage for `POST /api/v1/projects/{id}/storage/bindings`: the
fixture uses `201 Created`, requires `group_id` and `pvc_id`, has no optional
request fields, emits only `ProjectStorageBindingChanged`, and is checked
against `storage.Spec()` for auth, route, path params, non-admin
state-changing behavior, no service key, no adapter, success/error statuses,
direct response shape, and event metadata. This is static contract evidence
only; it does not prove live Kubernetes mount execution, cluster PVC isolation,
CSI behavior, full storage GA, Full GA, or first-version readiness.

Storage permission creation now also has local/static typed external REST
fixture coverage for `POST /api/v1/storage/permissions`: the fixture uses
`200 OK`, requires `group_id`, `pvc_id`, `user_id`, and `permission`, has no
optional request fields, emits only `StoragePermissionChanged`, and is checked
against `storage.Spec()` for auth, route, no path params, non-admin
state-changing behavior, no service key, no adapter, success/error statuses,
direct permission response shape, and event metadata. This is static contract
evidence only; it does not prove live permission enforcement, Kubernetes mount
execution, cluster PVC isolation, namespace enforcement, CSI behavior, full
storage GA, Full GA, or first-version readiness.

Storage project permission update now also has local/static typed external REST
fixture coverage for
`PUT /api/v1/projects/{id}/storage/bindings/{pvcId}/permissions`: the fixture
uses `200 OK`, requires `user_id` and `permission`, has no optional request
fields, emits only `ProjectStoragePermissionChanged`, and is checked against
`storage.Spec()` for auth, route, `id`/`pvcId` path params, `pvcId` route ID
param, non-admin state-changing behavior, no service key, no adapter,
success/error statuses, direct project permission response shape, and event
metadata. This is static contract evidence only; it does not prove live
permission enforcement, Kubernetes mount execution, cluster PVC isolation,
namespace enforcement, CSI behavior, full storage GA, Full GA, or first-version
readiness.

Storage project permission delete now also has local/static typed external REST
fixture coverage for
`DELETE /api/v1/projects/{id}/storage/bindings/{pvcId}/permissions/{userId}`:
the fixture uses `200 OK`, has no required or optional request fields, has an
empty request/response example, emits only `ProjectStoragePermissionChanged`,
and is checked against `storage.Spec()` for auth, route, `id`/`pvcId`/`userId`
path params, `userId` route ID param, non-admin state-changing behavior,
no service key, no adapter, success/error statuses, and event metadata. This is
static contract evidence only; it does not prove live permission enforcement,
Kubernetes mount execution, cluster PVC isolation, namespace enforcement, CSI
behavior, full storage GA, Full GA, or first-version readiness.

Storage project permission batch update and batch delete now also have
local/static typed external REST fixture coverage for
`PUT /api/v1/projects/{id}/storage/bindings/{pvcId}/permissions/batch` and
`DELETE /api/v1/projects/{id}/storage/bindings/{pvcId}/permissions/batch`: the
fixtures use `200 OK`, require canonical top-level `items` request bodies,
model direct `succeeded`/`failed`/`errors` batch result responses, emit only
`ProjectStoragePermissionChanged`, and are checked against `storage.Spec()` for
auth, route, `id`/`pvcId` path params, `pvcId` route ID param, non-admin
state-changing behavior, no service key, no adapter, success/error statuses,
canonical item fields, and event metadata. This is static contract evidence
only; it does not prove live permission enforcement, Kubernetes mount
execution, cluster PVC isolation, namespace enforcement, CSI behavior, full
storage GA, Full GA, or first-version readiness.

Storage projection visibility now also has local/in-memory DATA-016 coverage:
`storage.storageProjectionDrift` compares raw owner/source resources with local
storage projection rows for identity users, identity roles, projects, project
members, and user groups. Focused coverage includes missing/orphan/stale,
deterministic sort, blank-ID skip, canonical-id normalization, nil app/store
fail-closed behavior, exact five-pair coverage, and `Config{ServiceName:"all"}`
fallback-trap verification. This remains helper-only in-memory evidence and does
not close live drift, read-model rebuild/replay cutover, all-service
DATA-016, DATA GA, Full GA, or production readiness.

Bounded Harbor-to-catalog synchronization also has live API
evidence with trace `654e8a882af7e6a2099a5cce75a8377e` on
`ci-ga-harbor-catalog-sync-reviewfix-20260621224351`
(`sha256:3730083b5b028d8a592de463892ced37b399c07bc68aef1471b9d80214168939`):
real Harbor artifact `library/nexuspaas-sync:ga-harbor-sync-20260621224455`
(`sha256:73aaf090f3d85aa34ee199857f03fa3a95c8ede2ffd4cc2cdb5b94e566b11662`)
synced into `container_tags` as `status="available"` / `code="ok"`, and exact
Harbor/platform synthetic cleanup was verified. Explicit Harbor delete-resync
lifecycle now has live API evidence on
`ci-ga-harbor-delete-lifecycle-20260621225732`
(`sha256:68b5eefe30ee0644edb4e5146523bce82ad83ea572d31fc7c7be6d5f3d1cccca`):
temporary artifact
`library/nexuspaas-sync:ga-harbor-delete-lifecycle-20260621225849` synced
available with digest
`sha256:73aaf090f3d85aa34ee199857f03fa3a95c8ede2ffd4cc2cdb5b94e566b11662`,
Harbor delete returned `200`, exact tag lookup returned `404`, re-sync returned
`code="artifact_not_found"`, and the existing catalog row became
`deleted=true`, `unavailable=true`, and `status="missing"` before exact cleanup.
OPS-011 Redis/event-broker outage evidence also passed with trace
`ga-redis-outage-20260621202250`: a committed
`GroupStorageCreated` event stayed durable in Postgres while Redis was scaled
to zero, then published to Redis DB1 after restore and natural relay-lease
expiry. Partial OPS-013 Prometheus/telemetry stale and quota non-grant evidence
also passed on image `ci-ga-prometheus-stale-20260621205458`: stale metadata is
served on cluster/project GPU read APIs, the Prometheus adapter degrades clearly
when not configured, and scheduler admission still rejects over-quota requests.
The first PERF live read baseline is also recorded: an initial run exposed
`/outbox`/port-forward instability with repeated `5s` timeouts and `4.2857%`
total failure rate, then a corrected retained-counter rerun using k6 global
iteration selection passed the bounded smoke baseline with `20` VUs, `210`
iterations, `0` failures, and total p95 `2306.00ms`. A reusable k6
`100` VU core-read harness now exists and live Project list `100` VU evidence
has passed on `ci-ga-pdp-enforce-20260622094936`: `100` temporary exact-scope
principals ran for `30s`, `/api/v1/projects` returned `1000/1000` 2xx with
`0` failures, `0` 429, p95 `3.668ms`, and authorization-policy enforce `429`
count `0`; exact temporary DB policies and Secret patches were cleaned. WEB-006
stream credential GUI proof and the PERF stream credential issuance p95
sub-target also passed on `ci-ga-web-stream-cred-20260622102018`
(`sha256:d14aa360d5f0e4273846c88a785a2ad8cafc570613e8d892a7d9ef4407c899b1`):
live Playwright proved issued credential plus redacted GUI display for streaming
Job `e2e-job-mqom1t1b-pa2jbl`, and k6 returned
`/api/v1/stream/credentials`
`3000/3000` 2xx with p95 `22.926812599999987ms`, failure rate `0`, and `0`
429/4xx/5xx; exact temporary policies, Secrets, and seed data were cleaned.
RTC-006/RTC-007 credential-safety tests now also prove TURN TTL cap/default
behavior, RFC3339 expiry windows, username expiry prefix matching, HMAC-derived
passwords, password not equal to the shared secret, and serialized response
non-disclosure of the shared secret. RTC-008 direct ICE and forced TURN relay
candidate gathering now has current live RKE2/staging GUI route-proof evidence:
seeded streaming Job `e2e-job-mqongkuq-oov6qe` recorded
`rtc_probe_environment="staging"`, `rtc_direct_ok=true`,
`rtc_direct_candidate_types=["host"]`, `rtc_relay_ok=true`, and
`rtc_relay_candidate_types=["relay"]`; temporary browser-reachable TURN config
was restored and secret values were not printed. Streaming now attaches Selkies
as an auto-injected sidecar (`STREAM_SIDECAR_IMAGE`) on the user's own pod, so
GUI workloads stream without rebuilding the app image; live WebRTC/NVENC remains
operator-verified on GPU hardware.
WEB-007 active-Project GPU usage now also has live nonzero requested-GPU pod
evidence on `ci-ga-gpu-readmodel-20260622034034`
(`sha256:2f0ebfc868a26fb59a9b3d20194756a9f8e2917b61397d50d80a16c9cde840c7`):
seeded Project `e2e-p-mqooctn3-fammye` used fixture pod `gpu-proof` with
`nvidia.com/gpu: "1"` request and the GUI route proof recorded
`gpu_status=200`, `gpu_ok=true`, `gpu_used=1`, and `gpu_nonzero=true`; exact
Project/namespace cleanup and collector interval restore were verified.
WEB-004 bounded Kubernetes pod logs now also has live non-empty GUI/API evidence
on `ci-ga-job-logs-nonempty-fix-20260622130645`
(`sha256:fdb674beaf60e1ea052a7cbc974263b5c9fee4d39927c5980c12feb48ff2cc7e`):
seeded Project `e2e-p-mqora84n-1y46vp` used fixture pod `log-proof`, the pod
emitted `nexuspaas-log-proof-e2e-p-mqora84n-1y46vp`, and route proof recorded
`job_logs_status=200`, `job_logs_count=1`, `job_logs_nonempty=true`, and
`job_logs_visible=true`; exact proof namespace/build-pod cleanup and inotify
restore were verified.
WEB-004 continuous log/status polling now has focused frontend evidence:
`frontend/src/App.tsx` polls selected Job logs and active-Project workload
status through existing REST calls, and `npm --prefix frontend run test -- src/App.test.tsx`
passed coverage for timer polling, retry after failure,
second-Job cleanup, Project switch cleanup, unmount cleanup, and token-safe
inline errors; `npm --prefix frontend run build` also passed. This is local
frontend evidence only; Docker/live E2E and full workload lifecycle status
remain open.
WEB-007 frontend usage workflow now has focused local evidence:
`frontend/src/App.tsx` shows active-Project-filtered current-user usage and
request-usage tables, compact row/resource totals, Project GPU pods summary
from the existing Project route, and a Usage-local refresh button that re-runs
only `/api/v1/me/usage`, `/api/v1/me/request-usage`, and
`/api/v1/projects/{projectID}/gpu-usage`. The focused App test passed coverage
for filtering/totals, manual refresh, GPU-route failure isolation with
non-secret text, no admin usage fallback, and no browser-storage credential
persistence; `npm --prefix frontend run build` also passed. This is local
frontend evidence only; live usage attribution and per-device GPU utilization
remain open.
This is still local evidence, not HA, external registry, off-cluster DR, 8-unit
staging rollback, full OPS-019 failure-injection coverage, full WebRTC media
session, live continuous log tailing/full workload status beyond the focused
frontend REST polling slice, live usage attribution beyond the focused WEB-007
frontend workflow, full image workflow, per-device GPU utilization telemetry,
or full PERF-003..008 acceptance._

## Current Architecture Reality

NexusPaaS is currently a Go modular monolith with service-boundary awareness.
The backend has 15 logical bounded-context services in one Go module and one
shared platform runtime. Production Beta now has an 8-physical-unit runtime
topology that hosts those 15 logical services; the remaining GA target is
clearer data ownership, reliable event delivery, stronger service identity, and
production-grade operational evidence.

Do not describe the current backend as mature production-grade microservices.
The accurate description is:

> Go-based modular monolith with service boundaries and an 8-unit Production
> Beta runtime topology; GA-grade microservice isolation is still in progress.

## Completed Or In Progress

| Area | Status | Notes |
| --- | --- | --- |
| 8-unit GA direction | Done | ADRs and architecture docs define the target deployable units. |
| Physical 8-unit runtime split | Done | Production Beta kustomize, runtime config, local compose smoke, and CI gates define/use 8 backend units hosting 15 logical services. Live rollback evidence is currently for the 15-deployment namespace only, so 8-unit staging rollback remains open. |
| Identity data boundary | Started | Identity-owned records and migrations exist; other core domains still need typed ownership work. |
| Contract fixtures | Started | Core event, owner-read, command, producer, and consumer fixture coverage exists for initial slices. Local/static typed external REST fixtures now include the already committed identity auth/session entrypoints plus the current-user identity API-token lifecycle slice. |
| Projection visibility | Started | Projection lag, retry, replay, dead-letter visibility, local/in-memory authorization-policy drift comparison, local/in-memory IDE drift comparison for six IDE read-model pairs, local/in-memory dashboard drift comparison for six dashboard read-model pairs, local/in-memory clusterread drift comparison for six clusterread read-model pairs, local/in-memory request-notification project-access drift comparison for three project-access read-model pairs, local/in-memory GPU usage drift comparison for five GPU usage read-model pairs, local/in-memory image-registry drift comparison for five image-registry access read-model pairs, and local/in-memory storage drift comparison for five storage read-model pairs now exist. Durable transactional outbox delivery evidence is tracked as done in the next row; remaining projection work is live drift jobs, read-model rebuild/replay cutover, all-service DATA-016 coverage, DATA GA, Full GA, and first-version readiness evidence. |
| Transactional outbox/inbox | Done for delivery evidence | Outbox/inbox tables + relay + inbox dedupe wired in `runtime.go`. Single-record coupling via `App.*RecordWithEvent` and `App.UpsertRecordWithEvent`, plus multi-record coupling via `App.WithTx` (`StoreTx`/`RunInTx`), now cover single-record sites in 7 services plus non-batch authorizationpolicy mutations (role create/update, policy rule replacement update, policy assignments, role users, raw permissions), authorizationpolicy policy/role cascades, schedulerquota queue cascade, storage non-batch upsert/update/delete mutations and cascades, orgproject group/project delete cascades, and batch per-item/custom mutation coverage across authorizationpolicy, storage, schedulerquota, identity, orgproject, imageregistry, and workload. Reviewer-approved live RKE2 evidence captured final image `localhost:5000/nexuspaas-backend:ci-ga-pdp-scope-20260620163744` (`sha256:1817b0c42c37fe6e4d75e1155f7022084aac675dfb52857f16f7b45299b6af62`), 15 backend deployments ready, PDP service-key scope check, HTTP 201 `POST /api/v1/forms`, and matching `FormCreated` row in `platform_event_outbox`. Expanded-surface publish-lag evidence passed with storage API events `940df12e-f953-4460-bc06-3aa487209016`, `6c749a52-a980-4d9f-9f6b-834f5f6e0068`, and `a70177d7-f0da-48a8-84f3-44bee283a54f` reaching `published` / `relay_attempts=0` and appearing in Redis DB1. Relay recovery evidence passed for `ga-outbox-crash-20260621103919`: the row stayed pending under a short sentinel relay lease, a relay-capable pod restart occurred, the sentinel was released, the row reached `published|0|true`, Redis DB1 retained the event, and exact cleanup left zero synthetic outbox rows. This proves controlled relay unavailability plus relay-capable pod restart recovery, not handler mid-transaction crash interleavings or the exact restarted pod as publisher. |
| Route auth and collision hardening | Done | Centralized internal-route service auth and route collision validators are implemented, wired into startup checks, covered by focused/full tests, vet, build, quick gate, Sonar Quality Gate, reviewer approval, and live RKE2 missing/wrong/valid service-key evidence. |
| Public route RBAC test coverage | Done for local/catalog RBAC-016 coverage | `TestRBACPublicAPIRoutesRequireAuthUnlessExplicitlyAllowed` now covers registered external `/api/v1/` service routes, excluding `/api/v1/internal/` and service-auth routes. Intentional public auth/OIDC entry routes require exact method+pattern allowlist reasons, and every other scoped route is verified to require auth and return `401` through `app.ServeHTTP` with no credentials. This is local test evidence only; it does not claim full RBAC GA, live gateway proof, every business authorization branch, Full GA, or first-version completion. |
| OpenAPI auth metadata | Done for local/static RBAC-017 parity | Generated OpenAPI now mirrors `RouteSpec` auth metadata for `x-auth`, `x-admin`, `x-policy-bypass`, and `x-service-auth-required`, and emits user/service security including combined user+service requirements. Focused generator and registered-route parity tests pass. This is contract metadata evidence only; it does not claim live gateway proof, service credential rotation, workload identity, mTLS/SPIFFE, full RBAC GA, Full GA, or first-version completion. |
| API token indexed lookup | Done | User API tokens now use `nexuspaas_<token-id>_<secret>` format; local and identity verification parse the id, load one record, verify one full-token hash, pass focused/full Go tests, quick gate, Sonar Quality Gate, reviewer approval, and live RKE2 identity auth evidence. |
| Trusted client IP resolution | Done | Identity login failures, captcha checks, cleanup, and API-token audit events now reuse the platform trusted-proxy resolver; focused/full Go tests, quick gate, Sonar Quality Gate, reviewer approval, RKE2 rollout, health checks, and live spoofed `X-Forwarded-For` evidence passed. |
| Environment profiles and PDP fail-closed | Done | Runtime config now supports explicit `APP_ENV` profiles (`local`, `test`, `dev`, `staging`, `production`), preserves legacy `PRODUCTION` fallback, rejects invalid/conflicting mode settings, uses strict startup/PDP checks for staging/production, declares `APP_ENV: "production"` in production backend manifests, and passed focused/full tests, quick gate, Sonar Quality Gate, reviewer approval, and live RKE2 rollout health/readiness evidence. |
| Production Beta operations | Partial | Non-live gates and docs exist, static production-beta Secret deploy-path source/render proof now shows required Secret names/keys and no local/dev/test placeholder references without exposing values, current live 15 first-party deployments have same-image Kubernetes rollout/undo evidence, OPS-006 PostgreSQL logical backup/restore drill passed, OPS-008 MinIO synthetic object restore drill passed, OPS-009 current-live Kubernetes Secret recovery copy drill passed, OPS-007 Harbor backup/restore passed after moving Harbor from unsupported Rancher `local-path` hostPath PVCs to Kubernetes static `local` PVs, Harbor-side push/scan/delete passed with official Trivy and `crane`, OPS-011 Redis/event-broker outage evidence passed (`ga-redis-outage-20260621202250`, event `33fc697b-2cac-4715-ac04-e46097b0ea99`), partial OPS-013 Prometheus/telemetry stale and quota non-grant evidence passed (`ci-ga-prometheus-stale-20260621205458`, trace `ga-prometheus-stale-20260621205959`), Harbor dependency outage evidence passed through `/api/v1/harbor-status` (`ga-harbor-outage-20260621200008`), OPS-012 image-registry build/list degraded-route outage evidence passed (`ga-image-harbor-degraded-20260621212113`, image `ci-ga-image-harbor-degraded-20260621211729`, digest `sha256:21345ac6ad43db05f489bfa2ee37122b0b79873daca6f04619506c8bcef9d319`), a bounded PERF read smoke baseline passed after correcting k6 endpoint selection (`20` VUs / `210` iterations / `30` requests per endpoint / `0` failures / total p95 `2306.00ms`), Project list `100` VU live k6 evidence now passes on `ci-ga-pdp-enforce-20260622094936` (`100` temporary principals, `30s`, `/api/v1/projects` `1000/1000` 2xx, `0` failures, `0` 429, p95 `3.668ms`, enforce `429` count `0`, exact cleanup verified), and local deterministic `PERF-003..008`, `MON-013..017`, partial `DATA-014` image-build create/cancel, workload-submit/cancel, and scheduler-preemption idempotency, and all three image build create fixture optional-header evidence are recorded in the ledgers. Live 8-unit staging deploy/smoke/previous-image rollback/redeploy, live external staging Secret objects/provenance, full NexusPaaS image-build/allow-list/SBOM/signing/GUI scan workflow evidence, managed/off-cluster secret recovery, PITR/off-cluster DR, HA/off-cluster Harbor storage, full OPS-019 failure injection, live Prometheus telemetry/retention/alerting, live build execution/cleanup, live K8s-control throughput, full DATA-014 command coverage, typed ownership, live drift jobs, and Full GA remain open. |

Additional Web/performance evidence: WEB-006 stream credential operation and
the stream credential p95 sub-target passed on
`ci-ga-web-stream-cred-20260622102018`
(`sha256:d14aa360d5f0e4273846c88a785a2ad8cafc570613e8d892a7d9ef4407c899b1`).
RTC-008 direct/relay candidate gathering also passed in the GUI E2E harness.
WEB-007 active-Project GPU usage has a live nonzero requested-GPU pod proof on
`ci-ga-gpu-readmodel-20260622034034` with `gpu_status=200`, `gpu_used=1`, and
`gpu_nonzero=true`. This does not close full WebRTC media session, per-device
GPU utilization telemetry, live continuous log tailing/full workload status
beyond the focused frontend REST polling slice, full usage workflow, full image
workflow, or the remaining PERF families.

Additional local non-GPU PERF/MON/DATA/IMG evidence: deterministic local coverage
now exists for `PERF-003` queue states, `PERF-004` large Group request-usage
queries, `PERF-005`/`USAGE-023` retained GPU-metric cardinality sanitization,
`PERF-006` image-build quota/timeout/concurrency admission, `PERF-007` stream
admission/egress policy, `PERF-008` dispatcher batch limiting, `MON-013` through
`MON-017` low-cardinality `/metrics` snapshots, partial `DATA-014`
image-build create/cancel, workload-submit/cancel, and scheduler-preemption
idempotency, and all three image
build create fixture optional-header contract evidence for `Idempotency-Key`, and
`IMG-011` build-log response redaction, plus `IMG-012`/`IMG-013` active-build
slot release after cancellation and timeout terminal states. This remains local
deterministic and static contract evidence only; live queue stress, usage-query load,
Prometheus retention/alerting, live build execution/cleanup, live resource
termination, real timeout control, executor/Kubernetes quota release, deploy
idempotency, live scheduler/Kubernetes preemption, live
Harbor/Tekton/BuildKit/Kubernetes cancellation, live build log
streaming/tailing, browser media/live egress, K8s-control throughput, full
DATA-014 command coverage, typed ownership, live drift jobs, V1 external launch,
and Full GA remain open.

Additional image-registry evidence: bounded Harbor-to-catalog sync now uses the
existing Harbor adapter/proxy and image-registry maintenance task to upsert one
artifact into `container_tags`; live proof passed on
`ci-ga-harbor-catalog-sync-reviewfix-20260621224351`
(`sha256:3730083b5b028d8a592de463892ced37b399c07bc68aef1471b9d80214168939`)
with trace `654e8a882af7e6a2099a5cce75a8377e`, and exact synthetic cleanup
left no Harbor artifact or platform read-model rows. Explicit delete-resync
proof passed on `ci-ga-harbor-delete-lifecycle-20260621225732`
(`sha256:68b5eefe30ee0644edb4e5146523bce82ad83ea572d31fc7c7be6d5f3d1cccca`):
the same bounded sync route first imported a real Harbor tag as `available`,
then after Harbor API delete and exact `404` confirmation re-sync marked the
existing catalog row `deleted=true`, `unavailable=true`, and `status="missing"`;
cleanup deleted 2 exact platform rows and left no API catalog leftovers.

## P0 Blockers Before Production/GA

| Area | Current Problem | Next Step |
| --- | --- | --- |
| Transactional outbox/inbox (service publishes) | Resolved for current delivery evidence. Single-record coupling via `App.*RecordWithEvent` / `App.UpsertRecordWithEvent` and **multi-record coupling via `App.WithTx`** (`tx.go`; `StoreTx` port + `PostgresStore.RunInTx`) — multiple writes plus events commit in one tx (in-memory fallback publishes after the owner write). **Verified foundation:** focused/full tests, quick gate, Sonar Quality Gate, live migration/validation jobs, 15-deployment rollout, PDP service-key scope fix, HTTP 201 `POST /api/v1/forms`, and matching durable `FormCreated` row. **Coupled:** generic CRUD + single-record sites in workload/imageregistry/orgproject/schedulerquota/requestnotification/storage/identity; authorizationpolicy non-batch and batch assignment/role/raw-permission mutations; schedulerquota plan/queue batch deletes, queue binding, and successful preemption `JobPreempted`; storage permission batches; identity user batch reset/role/delete paths; orgproject membership/project-member/quota/workspace/GPU/plan-binding paths; imageregistry catalog sync/publish/unpublish/delete; workload submit/cancel/config commit/instance command. **Live relay evidence:** representative storage events reached `published` with `relay_attempts=0` and were found in Redis DB1; synthetic `ga-outbox-crash-20260621103919` survived controlled relay unavailability plus relay-capable pod restart and then published after sentinel release. | No remaining outbox delivery-evidence action. Broader typed ownership remains tracked separately below. |
| Typed domain data ownership | Core domains still rely too heavily on generic `platform_records` / JSONB payloads. | Move identity, tenant/project, workload, scheduler/quota, storage, registry/build, and billing-related data to typed schemas and repositories slice by slice. |
| Reproducible toolchain | Local quick, Docker-backed, manifest rehearsal, 8-unit collaboration gates, full backend coverage run, and local Sonar Quality Gate are green. Remote PR #33 external SonarCloud Code Analysis and Backend Quality Gate evidence is passing; live external staging evidence remains open. Latest Sonar API status: `new_coverage=81.8`, `new_violations=0`, `new_duplicated_lines_density=0.8262`. | Keep local quick/Sonar/Docker-backed collaboration and PR Sonar/Backend Quality Gate evidence green, then capture live staging evidence per deployable unit. |
| Harbor DR storage maturity | OPS-007 local drill now passes on Kubernetes static `local` PVs, but this storage is single-node and manually pre-provisioned. The runbook must recreate matching static PVs before restore readiness and recreate an empty Redis PVC because Redis is intentionally excluded as cache. This proves Harbor backup/restore mechanics for local evidence, not production-grade HA/off-cluster DR. | Keep static local PV evidence as the OPS-007 drill proof, then move the GA DR path to Longhorn/CSI snapshots or another reviewed HA/off-cluster design with encrypted backup storage and retention evidence before claiming production-grade DR. |

## P1 Architecture Maturity

| Area | Current Problem | Next Step |
| --- | --- | --- |
| Service identity | First-release scoped per-service HTTP credentials are implemented for internal callers: `X-Service-Name` + `X-Service-Key`, trusted caller keys, audience checks, shared client/PDP/remote-identity header injection, and strict staging/production config failure for legacy-only remote service auth. `SERVICE_API_KEY` remains local/dev/test compatibility only. | Keep this scoped-key slice as v1 evidence; add rotation and move the later GA path to workload identity, mTLS, or SPIFFE/SPIRE. |
| JWT/JWKS verification | Done for the first-release library-verifier slice: production JWT verification now uses `github.com/coreos/go-oidc/v3` for compact JWT/JWKS/signature handling with an explicit RS*/ES* algorithm allow-list, while local checks preserve the existing trusted-audience map, required `sub`/`exp`, one-minute `exp`/`nbf`/future-`iat` skew, `jti` revocation, and user/role mapping. | Keep this slice as v1 evidence; broader live OIDC/provider proof, service credential rotation, workload identity, mTLS/SPIFFE, and full GA security maturity remain separate gaps. |
| Migration runner | Started — the existing pgx runner now has a first-release ledger/checksum/advisory-lock/dirty-state implementation slice with DB-free validation, focused/full backend tests, and live PostgreSQL integration evidence through redacted `platform-gateway-runtime-secret:DATABASE_URL` for temporary-schema isolation, dirty persistence, checksum mismatch blocking, first ledger adoption/skip, and lock contention. | Keep live staging migration/rollback drill, full schema-change rollback evidence, and broader migration-runner GA maturity open. |
| Provider coupling | Started boundary documentation in [ADR 0007](docs/adr/0007-provider-coupling-boundary.md): portable core contracts are separated from Longhorn/reference storage, Harbor, MinIO/S3, Dex/OIDC, Redis Streams, and k3s/dev reference providers. Implementation adapters and live portability proof remain open. | Implement provider adapters or replacement paths and prove live portability before claiming portability. |
| Typed API contracts | Started for local/static external REST fixtures: request-notification create-form has fixture validation and spec parity coverage for `POST /api/v1/forms`, image-registry image build create routes have fixture validation plus `imageregistry.Spec()` parity coverage for `POST /api/v1/images/build`, `POST /api/v1/images/build/from-storage`, and `POST /api/v1/images/build/dockerfile`, image-registry acceleration profile creation has fixture/event coverage, queued `ImageBuildStarted` supply-chain status metadata has event fixture and historical schema-v1 compatibility coverage, identity auth/session routes have fixture validation plus `identity.Spec()` parity coverage for `POST /api/v1/register`, `POST /api/v1/login`, `POST /api/v1/refresh`, and `POST /api/v1/cli/login`, identity API-token lifecycle routes have fixture validation plus `identity.Spec()` parity coverage for `GET /api/v1/me/api-tokens`, `POST /api/v1/me/api-tokens`, `DELETE /api/v1/me/api-tokens/{id}`, and `DELETE /api/v1/me/api-tokens/current`, workload job submission has fixture validation plus `workload.Spec()` parity coverage for `POST /api/v1/jobs`, workload ConfigFile creation/update has fixture validation plus `workload.Spec()` parity coverage for `POST /api/v1/configfiles` and `PUT /api/v1/configfiles/{id}` with a `ConfigFileChanged` service event metadata repair, workload job cancellation has fixture validation plus `workload.Spec()` parity coverage for `POST /api/v1/jobs/{id}/cancel` and a `JobCancelRequested` service event metadata repair, org-project Project creation has fixture validation plus `orgproject.Spec()` parity coverage for `POST /api/v1/projects`, org-project Project update has fixture validation plus `orgproject.Spec()` parity coverage for `PUT /api/v1/projects/{id}`, org-project Project delete has fixture validation plus `orgproject.Spec()` parity coverage for `DELETE /api/v1/projects/{id}`, org-project Project batch delete has fixture validation plus `orgproject.Spec()` parity coverage for `DELETE /api/v1/projects/batch`, org-project Group creation has fixture validation plus `orgproject.Spec()` parity coverage for `POST /api/v1/groups`, org-project Group update has fixture validation plus `orgproject.Spec()` parity coverage for `PUT /api/v1/groups/{id}` and a `GroupUpdated` service event metadata repair, org-project Group delete has fixture validation plus `orgproject.Spec()` parity coverage for `DELETE /api/v1/groups/{id}` and a `GroupDeleted` service event metadata repair, org-project Group batch delete has fixture validation plus `orgproject.Spec()` parity coverage for `DELETE /api/v1/groups/batch`, storage permission creation has fixture validation plus `storage.Spec()` parity coverage for `POST /api/v1/storage/permissions`, storage project permission update has fixture validation plus `storage.Spec()` parity coverage for `PUT /api/v1/projects/{id}/storage/bindings/{pvcId}/permissions`, storage project permission delete has fixture validation plus `storage.Spec()` parity coverage for `DELETE /api/v1/projects/{id}/storage/bindings/{pvcId}/permissions/{userId}`, storage project permission batch update has fixture validation plus `storage.Spec()` parity coverage for `PUT /api/v1/projects/{id}/storage/bindings/{pvcId}/permissions/batch`, storage project permission batch delete has fixture validation plus `storage.Spec()` parity coverage for `DELETE /api/v1/projects/{id}/storage/bindings/{pvcId}/permissions/batch`, authorization-policy proxy role create/update/delete has fixture validation plus `authorizationpolicy.Spec()` parity coverage for `POST /api/v1/admin/proxy-rbac/roles`, `PUT /api/v1/admin/proxy-rbac/roles/{id}`, and `DELETE /api/v1/admin/proxy-rbac/roles/{id}` with `ProxyPolicyChanged` event fixture coverage, authorization-policy proxy policy create/update/delete has fixture validation plus `authorizationpolicy.Spec()` parity coverage for `POST /api/v1/admin/proxy-rbac/policies`, `PUT /api/v1/admin/proxy-rbac/policies/{id}`, and `DELETE /api/v1/admin/proxy-rbac/policies/{id}` while reusing the existing `ProxyPolicyChanged` event fixture, authorization-policy proxy service list/get has fixture validation plus `authorizationpolicy.Spec()` parity coverage for `GET /api/v1/admin/proxy-rbac/services` and `GET /api/v1/admin/proxy-rbac/services/{id}` with no emitted events, authorization-policy proxy role-user list/assign/unassign has fixture validation plus `authorizationpolicy.Spec()` parity coverage for `GET /api/v1/admin/proxy-rbac/roles/{id}/users`, `POST /api/v1/admin/proxy-rbac/roles/{id}/users`, and `DELETE /api/v1/admin/proxy-rbac/roles/{id}/users/{user_id}` with list emitting no events and assign/unassign reusing `ProxyPolicyChanged`, and authorization-policy proxy policy assignment list/assign/unassign has fixture validation plus `authorizationpolicy.Spec()` parity coverage for `GET`, `POST`, and `DELETE /api/v1/admin/proxy-rbac/policies/{id}/assignments` with list emitting no events, assign/unassign reusing `ProxyPolicyChanged`, and unassign intentionally excluding `404`. Broader critical APIs still rely on route specs/generated OpenAPI instead of typed request/response contracts. | Continue moving critical APIs toward OpenAPI-first or explicit typed DTO contracts with fixtures; do not treat these fixtures as DATA GA, live auth/API-token lifecycle proof, live ConfigFile update proof, live admin authorization proof, live proxy role mutation proof, live proxy policy mutation proof, live proxy service behavior proof, live role-user mutation proof, live proxy policy assignment mutation proof, full Project/Group lifecycle, tenant isolation, live scheduler cancellation, Kubernetes job termination, cancellation propagation, live scheduler/Kubernetes execution, live permission enforcement, Kubernetes mount execution, cluster PVC isolation, namespace enforcement, CSI behavior, full workload workflow, completed image conversion/prewarm, completed SBOM/signing/scan enforcement, full image workflow, first-version readiness, or Full GA coverage. |
| Read-model drift and replay | Replay idempotency now has focused evidence: dead-letter replay retries only unresolved events and does not double-apply successful events. Authorization-policy, IDE, dashboard, clusterread, request-notification project-access, GPU usage, and image-registry local/in-memory drift comparisons now cover raw owner/source vs local projection missing, orphan, and stale rows for their scoped read-model pairs. GPU usage evidence covers the five identity/role/project/job pairs, deterministic ordering, canonical id normalization, nil app/store fail-closed behavior, exact pair coverage, snapshot/summary exclusion, blank-id skip behavior, and the co-hosted fallback trap. Image-registry evidence covers the five identity/role/project/member/group access read-model pairs, deterministic ordering, canonical id normalization, nil app/store fail-closed behavior, exact pair coverage, catalog/build/image-request/sync exclusion, blank-id skip behavior, and the co-hosted fallback trap. Live drift jobs, all-service DATA-016 coverage, and rebuild/cutover evidence are still not enough for cutover; this does not claim DATA GA, Full GA, first-version readiness, rebuild/replay cutover readiness, or production readiness. | Add live drift jobs, all-service DATA-016 coverage, and rebuild/cutover proof before retiring owner-read/shared-store paths. |
| Per-unit runtime isolation | Non-live 8-unit runtime isolation is proven locally, and current live 15 first-party deployments now have same-image rollout/undo evidence. Deployable-unit RBAC, network policy, migration ownership, target 8-unit staging rollback, previous-image rollback, and live staging evidence are not fully proven. | Capture staging deploy, smoke, previous-image rollback, and redeploy evidence for each of the 8 units. |

## P2 Documentation And Tooling

| Area | Current Problem | Next Step |
| --- | --- | --- |
| Documentation alignment | Older docs described the project as microservices-first and listed non-current runtime dependencies. | Keep README, roadmap, architecture docs, and backend docs aligned with current reality. |
| CI script size | The central security gate is useful but large. | Split checks only when there is real maintenance pain; keep the top-level script as the orchestrator. |
| Service ownership docs | Service-level ownership is partially documented across several files. | Consolidate owner, API, data, config, test, and deployment responsibility per deployable unit. |
| Provider ADRs | Provider abstraction is a target but not yet documented as concrete ADRs. | Add ADRs when replacing or abstracting current reference-stack assumptions. |
| Supply chain | SBOM generation and image signing are GA goals but not enforced. Queued image builds now expose pending supply-chain metadata and event-shape evidence, but that is not completed SBOM/signing/scan enforcement. | Add Syft/Cosign or equivalent after staging promotion is stable. |
| Course-monitoring scope | Decided by ADR 0006: the reference course-monitoring reconciler is out of scope for current NexusPaaS GA. | Done for scope decision only; MON GA, usage reporting, Prometheus retention, alerting, live monitoring evidence, V1 external launch, and Full GA remain open. |
| Remote Sonar | PR #33 external `SonarCloud Code Analysis` and Backend Quality Gate evidence is passing. | Keep the external SonarCloud PR check required by repository policy; live P0.2-P0.5 staging evidence remains open. |

## Preserved Direction

- Keep the modular monolith while boundaries are being proven.
- Keep the 8 deployable-unit target instead of forcing a 15-way split.
- Keep the reference distribution as k3s + Longhorn + Harbor + MinIO + Dex +
  Redis Streams until provider abstractions are justified by concrete needs.
- Prefer deleting stale docs and consolidating status over adding more planning
  files.

## Reviewer Status

This file is the maintained NexusPaaS backend gap/code-problem record, kept in
the team's narrative format aligned with `gap.md` rather than the generic
scheduled-task template (the same substance — feature gaps, code problems,
SOLID/12-Factor posture, microservice boundaries, and verification — is covered
in the sections above and in `gap.md`).

This pass found **no local code-quality regression and no newly-surfaced
unported reference capability.** All open items are the already-tracked
live-execution P0/GA blockers — external Harbor registry promotion/rollback,
8-unit staging deploy/smoke/previous-image rollback, live staging DB
migration/rollback drill, live external Secret readiness, typed domain
ownership, live drift jobs, full image-build/SBOM/signing workflow, and the
remaining PERF/OPS families — none of which are code defects in the current
tree.

Verification this pass (current working tree, branch `feature/ga-gap-clearance`):

| Command | Purpose | Result |
| --- | --- | --- |
| `go build ./...` | Compile all packages | Pass |
| `go vet ./...` | Static checks | Pass |
| `go test ./...` | Full unit/contract suite (24 pkgs) | Pass |
| Cron parity diff vs reference | Confirm reconciler ports | Pass (15/15 in scope; `course_monitoring` out per ADR 0006) |

Live external staging / SonarCloud PR gate evidence (P0.2–P0.5, V1 external
launch) remains open and is not re-run here — it requires the live environment,
not local toolchain.

Status: Approved
