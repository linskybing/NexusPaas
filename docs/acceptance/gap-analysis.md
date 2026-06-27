# Gap Analysis — Is the GA Spec Complete for a Launchable v1?

Part of the [GA Acceptance docs](README.md).

> **This is the only doc with new content.** Everything else under
> `docs/acceptance/` is a faithful migration of the original spec. The acceptance
> criteria proposed here are `(PROPOSED — not yet accepted)` and are NOT part of
> the GA checklist until a Reviewer approves them per the
> [three-agent workflow](../agents/workflow.md).

Bar used for this review: **可上線第一版標準 — the standard for a launchable first
version**, not a perfect platform. Each gap is tagged **[Blocks-v1]** (a launched
product would be incomplete or unsafe without it) or **[Defer]** (real, but can
follow the first launch).

## Blocks-v1 — proposed coverage for flows the original spec did not fully cover

### WEB-* — Web UI parity
The GA goal and final definition both say "CLI **or Web UI**", and several ACs
mention the Web UI (IMG-024, K8S-006), but only the CLI has its own acceptance
family. A first launch that advertises a Web UI needs minimum UI ACs.

**Current implementation status (2026-06-22):** a first-party operations Web GUI
is now served by `platform-gateway` at `/ui/`, but `WEB-*` remains incomplete.
The approved GUI API contract for GA v1 is the existing same-origin
REST/OpenAPI backend contract; no separate WebRPC/tRPC/gRPC transport is
required until a concrete API gap is proven. `WEB-001` now has live OIDC browser
login evidence on image `ci-ga-web-oidc-20260621203712`
(`sha256:a4cc30f2f8b6b8b949c47949de186023ba61a8ad78ea52e72b927e17ea1d670b`):
Playwright drove Dex login through `platform-gateway` at
`http://localhost:8080/ui/`, reached `/ui/?auth=oidc`, observed `token` and
`refresh_token` cookie names without logging values, verified browser
local/session storage contains no API key or token, and loaded the dashboard
through same-origin cookie-authenticated API calls. A focused live gateway probe
also proved `/api/v1/oidc/start` returns a browser-visible callback
`http://localhost:8080/api/v1/oidc/callback` plus an HttpOnly `SameSite=Lax`
state cookie through the gateway. `WEB-002` has live active-Project evidence:
seeded Playwright E2E creates a real Group/Project through existing REST routes,
connects to `/ui/`, verifies the active Project selector, and confirms the
seeded Project appears in the live GUI. `WEB-003` and `WEB-004` are
also partial: the Workloads panel calls existing ConfigFile and job REST routes,
lists ConfigFiles, filters authorized jobs by active Project for display,
submits a minimal ConfigFile and Job, displays the submitted job, reaches the
job logs route, and sends a job cancel request. Earlier live seeded E2E
submitted ConfigFile `CFG2600007`, submitted Job `e2e-job-mqneymza-1tqckn`,
requested logs with `job_logs_status=200` / `job_logs_count=0`, and requested
cancel with command `94925e294549528a2190b3dbafd09592`; the later bounded
pod-log proof below closes the non-empty log evidence gap. `WEB-005` and
`WEB-007` have partial surfaces: the Images panel calls existing Project image
and image-build routes, and live seeded E2E proves a created image build appears
in the active Project's build list. WEB-005 / IMG-024 now also has focused local
frontend evidence for active-Project Dockerfile build submission through
`POST /api/v1/images/build/dockerfile`, including trimmed `image_reference`,
success refresh of Project images/builds, no `/admin` fallback, no browser
storage persistence, and generic secret-safe submit failure text. The Usage
panel calls existing current-user
usage, request-usage, and active-Project GPU usage routes. Unit tests cover
image/build rows, active-Project usage filtering, forbidden Project GPU usage
rendering, no admin-usage fallback, and no browser credential persistence.
Image-registry Dockerfile build submission now also has local/static typed
external REST fixture coverage for `POST /api/v1/images/build/dockerfile`: the
fixture uses `202 Accepted`, requires `project_id` and `image_reference`, emits
only `ImageBuildStarted`, and is checked against `imageregistry.Spec()` for
auth, route, path params, state-changing, and `harbor` adapter metadata. This is
static contract evidence only; it does not prove live Harbor build execution,
SBOM/signing, allow-list enforcement, image scan lifecycle, or full image
workflow GA.
Image-registry profile and queued-event metadata now also have local/static
evidence: `ImageAccelerationProfile` admin CRUD, seeded defaults, create API
fixture, and `ImageAccelerationProfileChanged` event fixture are covered, and
queued image builds include pending supply-chain status fields in the response,
stored record, and `ImageBuildStarted` event while historical records/events
without those fields remain tolerated. This is metadata and event-shape evidence
only; it does not prove image conversion/prewarm execution, completed SBOM
generation, signing, scan enforcement, allow-list admission, live Harbor build
execution, or full image workflow GA.
Identity auth/session entrypoints now also have local/static typed external REST
fixture coverage for `POST /api/v1/register`, `POST /api/v1/login`,
`POST /api/v1/refresh`, and `POST /api/v1/cli/login`: the fixtures declare
public auth posture, exact required credential fields, success statuses, and
`UserCreated` only for registration, and are checked against `identity.Spec()`.
The shared fixture validator keeps credential-shaped request/response field
allowances scoped to those four public identity fixtures. This is static
contract evidence only; it does not prove live auth availability, cookie/browser
behavior, OIDC/LDAP behavior, rotation, revocation, all-critical API typed
coverage, DATA GA, Full GA, or first-version readiness.
Workload job submission now also has local/static typed external REST fixture
coverage for `POST /api/v1/jobs`: the fixture uses `201 Created`, requires
`project_id` and `user_id`, keeps queue/resource/config/streaming fields as
optional UI/admission/defaultable payload fields, emits only `JobSubmitted`,
and is checked against `workload.Spec()` for auth, route, path params,
state-changing, success status, and event metadata. This is static contract
evidence only; it does not prove live scheduler admission, queue policy
completeness, Kubernetes job execution, logs/tailing, GPU telemetry,
WEB-003/WEB-004 completion, or full workload GA.
Workload ConfigFile creation now also has local/static typed external REST
fixture coverage for `POST /api/v1/configfiles`: the fixture uses `201 Created`,
requires `project_id` and `name`, keeps accepted aliases/payload fields
optional, emits `ConfigFileChanged` as handler-emitted create evidence, and is
checked against `workload.Spec()` for auth, route, no service key/path params,
non-admin state-changing behavior, success status, and source-backed error
statuses. This is static contract evidence only; it does not prove live
scheduler admission, Kubernetes job execution, logs/tailing, GPU telemetry,
WEB-003/WEB-004 completion, DATA GA, Full GA, or first-version readiness.
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
Harbor-side push/scan/delete evidence now exists outside the GUI: official
Harbor Trivy scanned a real `busybox:1.36` image copied by `crane`, reached scan
status `Success`, and the synthetic Harbor project/repository were deleted.
WEB-005 catalog-derived image status display now also has live API and
Playwright GUI evidence under trace `ga-web-image-status-20260621214849`: a
seeded Project image on `ci-ga-web-image-status-20260621214330`
(`sha256:613c1f2cefbab9f029afc580213dc619e7892753826c02367fa6ea6be129f83e`)
exposed top-level `scan_status="Success"`, `deleted=true`,
`unavailable=false`, the seeded digest, and visible GUI state `deleted`. This
proves UI/API display of catalog/read-model metadata. Bounded automatic
Harbor-to-catalog synchronization now also has live API evidence under trace
`654e8a882af7e6a2099a5cce75a8377e`: a real Harbor artifact
`library/nexuspaas-sync:ga-harbor-sync-20260621224455` with digest
`sha256:73aaf090f3d85aa34ee199857f03fa3a95c8ede2ffd4cc2cdb5b94e566b11662`
was synced through `/api/v1/image-catalog/sync` into `container_tags` as
`status="available"`, `degraded=false`, and `code="ok"`, then exact Harbor and
platform synthetic rows were cleaned. Explicit Harbor delete-resync lifecycle
now also has live API evidence on
`ci-ga-harbor-delete-lifecycle-20260621225732`
(`sha256:68b5eefe30ee0644edb4e5146523bce82ad83ea572d31fc7c7be6d5f3d1cccca`):
temporary artifact
`library/nexuspaas-sync:ga-harbor-delete-lifecycle-20260621225849` first synced
available with digest
`sha256:73aaf090f3d85aa34ee199857f03fa3a95c8ede2ffd4cc2cdb5b94e566b11662`,
then Harbor delete returned `200`, exact tag lookup returned `404`, re-sync
returned `status="degraded"`, `code="artifact_not_found"`, `retryable=true`,
and the existing catalog row reported `deleted=true`, `unavailable=true`, and
`status="missing"` before exact cleanup left zero platform rows and an empty
synthetic Harbor repository.
WEB-006 stream credential operation now has first-party GUI and live API
evidence on `ci-ga-web-stream-cred-20260622102018`
(`sha256:d14aa360d5f0e4273846c88a785a2ad8cafc570613e8d892a7d9ef4407c899b1`):
the Workloads panel submitted a streaming job, exposed an `Open stream` action
for that job, called the existing same-origin
`POST /api/v1/stream/credentials` contract, and displayed only redacted stream
metadata. Live Playwright seeded streaming Job `e2e-job-mqom1t1b-pa2jbl`
recorded `seeded_job_streaming=true`, `stream_credentials_status=200`,
`stream_credential_uri_count=1`,
`stream_credential_username_present=true`, and
`stream_credential_password_issued=true`; the GUI proof records
`stream_credential_password_redacted=true`. TURN runtime config was enabled
through Kubernetes config/Secrets without logging secret values.
RTC-006/RTC-007 credential-safety evidence now also exists in focused backend
tests: TURN credential TTLs are capped/defaulted, `expires_at` is RFC3339 and
matches the username expiry prefix, generated passwords are HMAC-derived and not
the shared secret, and serialized responses do not contain the configured shared
secret.
RTC-008 direct ICE and forced TURN relay candidate gathering now also has
current live RKE2/staging GUI route-proof evidence. Seeded streaming Job
`e2e-job-mqongkuq-oov6qe` requested stream credentials through the GUI; the same
proof recorded `rtc_probe_environment="staging"`, `rtc_direct_ok=true`,
`rtc_direct_candidate_count=2`, `rtc_direct_candidate_types=["host"]`,
`rtc_relay_ok=true`, `rtc_relay_candidate_count=1`, and
`rtc_relay_candidate_types=["relay"]`. The proof temporarily used browser-
reachable `turn:127.0.0.1:3478?transport=udp`, restored runtime config to the
cluster TURN URI after the run, and did not log secret values. The same run
recorded `job_logs_status=200`, `job_logs_count=0`, `job_logs_visible=false`,
`gpu_status=502`, and `gpu_nonzero=false`; those earlier job-log and GPU route
gaps have since been addressed by the workload pod-log and usage-observability
read-model slices below.
WEB-007 active-Project GPU usage now has live nonzero GUI/API evidence on
`ci-ga-gpu-readmodel-20260622034034`
(`sha256:2f0ebfc868a26fb59a9b3d20194756a9f8e2917b61397d50d80a16c9cde840c7`):
`usage-observability-service` now writes Kubernetes platform job-pod GPU
request rows into `podGpuUsages` through the existing
`ListJobPodResourceUsage` adapter, the Project GPU route counts `project_id`
with namespace fallback, and Playwright ran with
`NEXUSPAAS_E2E_EXPECT_NONZERO_GPU=true`. Seeded Project
`e2e-p-mqooctn3-fammye` used a short-lived Kubernetes fixture pod
`gpu-proof` in namespace `gpu-e2e-p-mqooctn3-fammye` with
`nvidia.com/gpu: "1"` request; route readiness reached `status=200 used=1`
on monitor attempt `2`, and the final GUI route proof recorded
`gpu_status=200`, `gpu_ok=true`, `gpu_used=1`, and `gpu_nonzero=true`.
The fixture namespace was deleted, the seeded Project returned `404` after API
cleanup, and the temporary `MAINTENANCE_INTERVAL=5s` runtime override was
removed. This proves nonzero requested-GPU pod visibility, not per-device GPU
utilization.
WEB-004 bounded Kubernetes pod logs now also has live GUI/API evidence on
`ci-ga-job-logs-nonempty-fix-20260622130645`
(`sha256:fdb674beaf60e1ea052a7cbc974263b5c9fee4d39927c5980c12feb48ff2cc7e`).
Seeded Project `e2e-p-mqora84n-1y46vp` used fixture pod `log-proof` in
namespace `proj-e2e-p-mqora84n-1y46vp`, emitted
`nexuspaas-log-proof-e2e-p-mqora84n-1y46vp`, and the live GUI route proof
recorded `job_logs_status=200`, `job_logs_count=1`,
`job_logs_nonempty=true`, and `job_logs_visible=true`; the proof namespace and
temporary build/tune pods were cleaned. This proves bounded non-empty pod-log
retrieval, not continuous tailing or a full workload status workflow.
WEB-004 continuous log/status polling is now strengthened in the first-party
frontend with bounded REST polling of selected Job logs and active-Project
workload status. Focused Vitest evidence passed with `npm --prefix frontend run
test -- src/App.test.tsx`, covering immediate fetch, timer polling, status
refresh, retry after failed poll, stale Job/Project cleanup, unmount cleanup,
and token-safe inline errors; `npm --prefix frontend run build` also passed.
This is frontend/local evidence only; Docker/live E2E, WebSocket/SSE tailing,
full workload lifecycle status, and full WEB coverage remain open.
WEB-007 frontend usage workflow is now strengthened in `frontend/src/App.tsx`
with active-Project-filtered current-user usage and request-usage tables,
compact visible row/resource totals, separated Project GPU observed pods,
reserved GPU fraction, SM attribution source, and a Usage-local manual refresh
button that re-runs only `/api/v1/me/usage`, `/api/v1/me/request-usage`, and
`/api/v1/projects/{projectID}/gpu-usage`. Focused App/API test evidence passed,
covering filtering/totals, manual refresh route counts, GPU-route failure
isolation with non-secret text, no admin usage fallback, no
`localStorage`/`sessionStorage` credential persistence, backward-compatible
`used` handling, and estimated/allocation-based SM attribution display;
`npm --prefix frontend run build` also passed. Backend local evidence now
separates `observed_gpu_pods` from `reserved_gpu_fraction` and labels
allocation-derived or unavailable true per-process SM as
`estimated_mps_allocation` or `unavailable`, not measured. This is
frontend/local read-model evidence only; live usage attribution, real
per-device/per-process GPU utilization, full WEB coverage, Full GA, and
first-version completion remain open.
Remaining WEB gaps are full WebRTC media session, real workload GPU
telemetry/utilization evidence, live continuous log tailing/full status workflow
evidence beyond the focused REST polling slice, live usage attribution beyond
the focused WEB-007 frontend workflow, PID/container/GPU process telemetry
evidence, full image-build/allow-list/SBOM/signing/GUI scan workflow, Harbor
scan lifecycle synchronization, and registry-wide automatic delete lifecycle
automation beyond explicit per-tag sync/delete-resync.
Browser-operated WebRTC sessions remain covered by the RTC family and are not a
substitute for a management Web UI.

| ID | Proposed Acceptance Criteria |
|---|---|
| WEB-001 (PROPOSED) | A user can log in through the Web UI (OIDC) without handling tokens manually. |
| WEB-002 (PROPOSED) | Web UI lists only authorized Projects and can set an active Project. |
| WEB-003 (PROPOSED) | Web UI can submit a ConfigFile and display the machine-readable admission result and rejection reasons. |
| WEB-004 (PROPOSED) | Web UI can list jobs, show status, tail logs, and cancel a job. |
| WEB-005 (PROPOSED) | Web UI can list Project images with Harbor/deleted/scan status. |
| WEB-006 (PROPOSED) | Web UI can open a WebRTC stream session for an authorized workload. |
| WEB-007 (PROPOSED) | Web UI usage views enforce the same RBAC as the usage API and never expose tokens/secrets. |

### STORAGE-* — storage binding and mount-plan validation
The deploy flow (`storage-service mount-plan check`) and build flow (user/group/
project storage) both depend on storage, but no AC defines mount-plan
validation, storage isolation, or PVC approval — yet workloads cannot run
without it.

**Current implementation status (2026-06-23):** `STORAGE-004` mount-plan audit
evidence exists, `STORAGE-001` now has local/in-memory resolver proof for
storage-owned project bindings, dispatch-ready group storage sources, effective
PVC permission, and project-permission precedence, and `STORAGE-002` now has
local/in-memory resolver proof for unrelated Project binding rejection and
other-user permission denial. `STORAGE-003` now has local handler-level
permission-management RBAC proof for plain group member / Project reader denial
across direct create/set, batch set, and batch delete group/project storage
permission rows, including no unauthorized target-row creation and seeded-row
retention after denied deletes. Project storage binding creation now also has
local/static external REST fixture coverage for
`POST /api/v1/projects/{id}/storage/bindings`, checked against `storage.Spec()`
for route/auth/state/service-auth/adapter metadata, request fields, statuses,
direct response shape, and `ProjectStorageBindingChanged` metadata. Storage
permission creation now also has local/static external REST fixture coverage
for `POST /api/v1/storage/permissions`, checked against `storage.Spec()` for
route/auth/state/service-auth/adapter metadata, no path params, required
request fields, statuses, direct permission response shape, and
`StoragePermissionChanged` metadata. Project storage permission update now also
has local/static typed external REST fixture coverage for
`PUT /api/v1/projects/{id}/storage/bindings/{pvcId}/permissions`, checked
against `storage.Spec()` for route/auth/state/service-auth/adapter metadata,
path params, required request fields, statuses, direct project permission
response shape, and `ProjectStoragePermissionChanged` metadata. It also now has
local/in-memory DATA-016 storage projection drift helper coverage for identity
users, identity roles, projects, project members, and user groups with
missing/orphan/stale/clean reporting, deterministic ordering, blank-ID skip,
canonical-id normalization, and nil app/store fail-closed checks. This does not
prove live permission enforcement, live Kubernetes mount execution, cluster PVC
isolation, namespace enforcement, CSI behavior, full storage GA, Full GA, or
first-version readiness. The broader storage isolation and mount validation
criteria below still should not be treated as fully proven unless `gap.md`
records explicit evidence for those slices.

Storage project permission delete now also has local/static external REST
fixture coverage for
`DELETE /api/v1/projects/{id}/storage/bindings/{pvcId}/permissions/{userId}`,
with no required request fields, no optional request fields, `200 OK`,
configured error statuses, and `ProjectStoragePermissionChanged`; it is also
checked against `storage.Spec()` for auth, no request body semantics, path
params `id`/`pvcId`/`userId`, `userId` route ID param, no service key, no
adapter, state-changing behavior, and event metadata. This is not live
permission enforcement, live Kubernetes mount execution, cluster PVC isolation,
namespace enforcement, CSI behavior, storage GA, Full GA, or first-version
readiness.

| ID | Proposed Acceptance Criteria |
|---|---|
| STORAGE-001 (PROPOSED) | A workload may mount only platform-approved PVCs / user / group / project storage resolved by the mount-plan; anything else is rejected. |
| STORAGE-002 (PROPOSED) | A user cannot mount another user's or unrelated Project's storage. |
| STORAGE-003 (PROPOSED) | Group/project storage access follows the same RBAC as the owning Project/Group. |
| STORAGE-004 (PROPOSED) | Mount-plan decisions are produced before quota reservation and are auditable. |

### SECRET-* — platform secret API
`Secret` resources are routed through "platform secret API or ExternalSecret
profile" (resource policy), but that API has no acceptance criteria, and secret
handling is security-critical at launch.

**Current implementation status (2026-06-22):** the proposed `SECRET-001..003`
v1 policy slice has implementation evidence: raw Kubernetes `Secret` YAML is
rejected by default through admission policy, secret-access rejection is audited
without secret values, and dispatcher defense-in-depth exists. This does not
close managed/off-cluster secret recovery, rotation/revocation, or full DR
evidence in the GA tracker.

| ID | Proposed Acceptance Criteria |
|---|---|
| SECRET-001 (PROPOSED) | Users create/reference secrets only through the platform secret API or an approved ExternalSecret profile; raw `Secret` YAML is rejected by default. |
| SECRET-002 (PROPOSED) | Secret values are never returned in plaintext through API, CLI, Web UI, or logs. |
| SECRET-003 (PROPOSED) | Secret access is Project-scoped and audited (create, read-reference, delete). |

### AUDIT-* — audit retention, immutability, and query
Audit events are required across nearly every family, but nothing defines audit
immutability, retention, or who may query/export — a multi-tenant launch needs
this to be trustworthy.

**Current implementation status (2026-06-21):** audit logs now have a
tamper-evident read-time hash chain, CSV integrity columns, configured product
branding, project/group-scoped audit-log query RBAC for project and group
admins, and an enforced service-internal retention cleanup trigger. The proposed
`AUDIT-*` family has implementation evidence; the broader GA tracker still has
non-audit blockers.

| ID | Proposed Acceptance Criteria |
|---|---|
| AUDIT-001 (PROPOSED) | Audit events are append-only / tamper-evident and cannot be edited or deleted through normal APIs. |
| AUDIT-002 (PROPOSED) | Audit query enforces RBAC: platform_auditor/admin cluster-wide, Project/Group admin scoped to their domain. |
| AUDIT-003 (PROPOSED) | A documented audit retention period exists and is enforced. |
| AUDIT-004 (PROPOSED) | Exported audit reports use the configured brand naming (consistent with NAME-08). |

### PLANADMIN-* — Plan and Queue lifecycle
Plan and Queue models exist and gate everything, but no AC says who creates/edits
them or that those mutations are controlled and audited.

**Current implementation status (2026-06-22):** the proposed `PLANADMIN-001..003`
slice has implementation evidence for controlled Plan/Queue mutations and actor
plus old/new audit data. This closes the proposed v1 slice, not every remaining
GA operations or runtime-isolation gap.

| ID | Proposed Acceptance Criteria |
|---|---|
| PLANADMIN-001 (PROPOSED) | Only platform admin (or delegated platform_manager) can create/edit/expire Plans and Queues. |
| PLANADMIN-002 (PROPOSED) | Plan/Queue mutations produce audit events with actor and before/after values. |
| PLANADMIN-003 (PROPOSED) | Attaching/detaching a Project Plan is authorized and audited; a Project always resolves to at most one active Plan. |

### GATE-* — gateway abuse controls and manifest size bound
Boundaries mention "coarse rate limits" but there is no AC, and §5 preflight
parses multi-document YAML with no size bound (a DoS vector). A public first
launch needs basic protection.

**Current implementation status (2026-06-22):** the proposed `GATE-*` slice and
K8S manifest size/document-count cap have implementation evidence for rate
limits, request-body bounds, and pre-parse manifest limits. Broader performance
and load evidence remains tracked separately under PERF.

| ID | Proposed Acceptance Criteria |
|---|---|
| GATE-001 (PROPOSED) | Gateway enforces per-principal rate limits and returns 429 with retry guidance. |
| GATE-002 (PROPOSED) | ConfigFile/manifest submissions have an enforced max byte size and max document count; oversize input is rejected before parsing cost. |
| GATE-003 (PROPOSED) | Request body size limits apply to upload/build-context and API payload paths. |

## Defer-post-v1 — real, but not launch-blocking

- **NOTIF-*** notification delivery guarantees — collaboration-unit exists;
  best-effort notification is acceptable for v1.
- **IDE dedicated ACs** separate from WebRTC — only needed if the IDE workload
  ships in v1; otherwise it is covered by the generic ConfigFile + stream path.
- **DR numeric RTO/RPO targets** — OPS-006..009 already require restore drills.
  As of 2026-06-21, OPS-006 PostgreSQL restore, OPS-008 MinIO object restore,
  OPS-009 Kubernetes Secret recovery copy, and OPS-007 Harbor Velero
  backup/restore on static Kubernetes `local` PVs have live drill evidence.
  This is drill evidence only; managed/off-cluster DR, PITR, HA storage, backup
  retention, and numeric RTO/RPO targets remain open in the Full GA tracker.
- **i18n / accessibility ACs** for the Web UI — product polish.
- **Billing** — explicitly future in the monitoring goal.

## Strengthen-existing — small additions to current families (not new families)

- **K8S:** add a manifest size / document-count cap AC (also see GATE-002) so the
  preflight DoS surface is closed inside the K8S family too.
- **SEC / CLI:** add an explicit session + refresh-token expiry/rotation AC.
  Implementation evidence now covers one-time refresh rotation/replay rejection,
  session expiry, internal expired/revoked credential rejection, and cleanup of
  expired/revoked credentials.
- **SEC:** scoped internal service identity now has v1 implementation evidence.
  Internal callers send `X-Service-Name` + `X-Service-Key`; receivers validate
  trusted caller keys and target audiences; strict staging/production config
  rejects legacy-only remote service auth. This does not close future credential
  rotation, workload identity, mTLS/SPIFFE, or remaining live migration
  drill/full rollback maturity gaps. The JWT/JWKS library-verifier slice is
  separately implemented with `github.com/coreos/go-oidc/v3`; it closes only the
  custom JWT/JWKS parsing and signature-verification replacement.
- **DATA:** replay idempotency has implementation evidence: projection replay
  retries only unresolved dead-letter events and tests assert that successful
  events are not double-applied. `DATA-017` / `DATA-018` now also have local
  operational endpoint evidence: `TestOperationalEndpointsExposeOutboxInboxRuntimeEvidence`
  publishes three events, runs read-model and dead-letter projections, calls
  `ReplayProjection`, and asserts `nexuspaas_event_outbox_events`,
  `nexuspaas_event_consumer_lag`, `nexuspaas_projection_applied_total`,
  `nexuspaas_projection_dead_letters_total`,
  `nexuspaas_projection_retry_total`, and
  `nexuspaas_projection_replay_total` samples, including second-scrape
  stability. This is local/in-memory metrics evidence only; live replay
  cutover, all-service rebuild, and typed ownership completion remain open.
  Transactional outbox coupling now includes the
  batch per-item/custom mutation paths across authorizationpolicy, storage,
  schedulerquota, identity, orgproject, imageregistry, and workload. Live
  publish-lag evidence now covers a representative expanded storage surface:
  `GroupStorageCreated` (`940df12e-f953-4460-bc06-3aa487209016`),
  `StoragePermissionChanged` (`6c749a52-a980-4d9f-9f6b-834f5f6e0068`), and
  `GroupStorageDeleted` (`a70177d7-f0da-48a8-84f3-44bee283a54f`) reached
  `published` with `relay_attempts=0` and were found in Redis DB1. Relay
  recovery evidence also passed for synthetic
  `ga-outbox-crash-20260621103919`: it stayed pending during a short sentinel
  relay lease hold, a relay-capable pod was restarted, the sentinel was
  released, the row reached `published|0|true`, Redis DB1 retained the event,
  and exact Postgres cleanup left zero synthetic outbox rows. This proves
  controlled relay unavailability plus relay-capable pod restart recovery; it
  does not claim handler mid-transaction crash interleavings or the exact
  restarted pod as publisher.
- **OPS / migrations:** the existing pgx migration runner now has a first-
  release ledger/checksum/advisory-lock/dirty-state implementation slice.
  `validate-migrations` remains DB-free and focused/full backend tests pass.
  PostgreSQL integration evidence now passes against live cluster PostgreSQL
  through redacted `platform-gateway-runtime-secret:DATABASE_URL` port-forward
  execution for temporary-schema isolation, dirty-state persistence, checksum
  mismatch blocking, first ledger adoption/skip, and advisory-lock contention.
  This still does not claim live staging migration drill, schema-change
  rollback, full migration-runner GA, Full GA, or first-version completion.
- **OPS:** OPS-011 Redis/event-broker outage evidence now exists: Redis was
  scaled from `1` to `0`, a direct `storage-service` pod request returned HTTP
  `201`, exact `GroupStorageCreated` event
  `33fc697b-2cac-4715-ac04-e46097b0ea99` was durable in Postgres as
  `pending|0|false` during the outage, Redis was restored to `1/1`, natural
  relay-lease expiry was respected without deleting the lease, the event
  reached `published|0|true`, Redis DB1 retained it, and exact Postgres
  synthetic rows were cleaned. Partial OPS-013 Prometheus/telemetry evidence
  also exists: cluster/project GPU read APIs now expose `telemetry_stale`,
  `telemetry_age_seconds`, and `collected_at`, live `/api/v1/cluster/mps`
  returned degraded Prometheus adapter status `adapter_not_configured`, and
  live scheduler admission rejected an over-quota synthetic request with HTTP
  `409` / `GPU quota exceeded` while Prometheus was not configured. Local
  USAGE-029 evidence also exists: usage-observability now emits
  `UsageDriftDetected` for material reserved-vs-observed GPU telemetry drift,
  persists `usage_drift_alerts` to suppress repeated equivalent alerts, and
  skips missing/zero reserved evidence so telemetry absence does not grant
  extra quota or create invalid drift ratios. Local USAGE-032 / MON-018 evidence
  also exists after commit `eb5cd16`: active reserved GPU jobs without fresh
  `job_gpu_usage_snapshots` emit `UsageDriftDetected` with
  `reason`/`drift_reason="active_reserved_jobs_missing_fresh_snapshots"`,
  repeated equivalent alerts are deduped and later resolved through
  `usage_drift_alerts`, and mixed projects report only the stale/missing subset
  in `missing_job_ids`. This alert path is informational and does not change
  quota admission, quota grants, or quota release behavior. This is local
  control-plane evidence only; live GPU/process telemetry, live node-agent
  failure proof, and full MON completion remain open under the usage and
  monitoring rows. Harbor
  dependency failure-injection evidence also exists: runtime `HARBOR_URL`
  points at the Harbor API ping path, healthy `/api/v1/harbor-status` was
  proven, `harbor-core` was scaled from `1` to `0`, the product API returned
  retryable degraded Harbor status with `degraded.code="adapter_unavailable"`
  under trace `ga-harbor-outage-20260621200008`, and recovery returned healthy
  after `harbor-core` was restored to `1/1`. OPS-012 image-registry build/list
  degraded behavior is now evidenced live under trace
  `ga-image-harbor-degraded-20260621212113`: project image list, project build
  list, and build submission preserved successful statuses/local data while
  adding retryable Harbor degraded metadata during `harbor-core=0`, then
  recovered with no degraded envelope after Harbor returned. This closes the
  OPS-012 build/list degraded-route proof only; full NexusPaaS
  image-build/allow-list/SBOM/signing/GUI scan workflow evidence and full
  OPS-019 DB, K8s API, live Prometheus interruption, node usage-agent, and other
  fault-domain evidence remain open.
- **PERF:** Project list load evidence now exists for `PERF-001`/`PERF-002`:
  image `ci-ga-pdp-enforce-20260622094936` ran `100` temporary exact-scope
  principals at `100` VUs for `30s`; `/api/v1/projects` returned `1000/1000`
  2xx with `0` failures, `0` 429, and p95 `3.668ms`; authorization-policy
  enforce emitted `0` 429 during the drill, and exact temporary DB policies plus
  Secret patches were cleaned. Stream credential issuance evidence now also
  exists for the `PERF-007` credential-path sub-target on
  `ci-ga-web-stream-cred-20260622102018`: `100` temporary principals drove
  `100` VUs for `30s`, `/api/v1/stream/credentials` returned `3000/3000` 2xx
  with failure rate `0`, `0` 429/4xx/5xx, and p95 `22.926812599999987ms`; exact
  temporary policy rows, API key Secret patches, and seed data were cleaned.
  Remaining performance evidence gaps are queue stress, large usage-query load,
  metrics cardinality, build concurrency, full WebRTC media concurrency,
  K8s-control throughput, and numeric DR RTO/RPO.

## Summary

| Tag | Items |
|---|---|
| Originally proposed Blocks-v1 families | STORAGE-*, SECRET-*, AUDIT-*, PLANADMIN-*, GATE-* |
| Now evidenced v1 slices | SECRET-001..003, AUDIT-001..004, PLANADMIN-001..003, GATE-*, K8S manifest cap, STORAGE-001 local/in-memory mount-plan authorization proof, STORAGE-002 local/in-memory cross-Project and cross-user mount-plan isolation proof, STORAGE-003 local handler-level permission-management RBAC proof, STORAGE-004 audit, org-project project update/delete/batch-delete typed external fixture coverage, org-project group update/delete/batch-delete typed external fixture coverage, and storage project permission delete and batch typed external fixture coverage |
| Still not proven as Full GA | Remaining STORAGE live isolation/mount-execution/namespace-enforcement slices, WEB full coverage, DR/rollback, full OPS-019, full image workflow, full usage workflow, full WebRTC media/session, telemetry, and remaining PERF evidence |
| Defer | NOTIF, IDE, DR RTO/RPO numbers, i18n/a11y, billing |
| Conditional | WEB-* blocks any release that advertises the NexusPaaS management Web UI as GA; the current `/ui/` implementation is partial and must stay labeled as such until the WEB ACs pass. |
| Strengthen-existing | K8S manifest size cap, SEC/CLI token lifecycle, RBAC-016 local catalog-driven public route auth coverage, RBAC-017 local/static OpenAPI auth metadata parity, DATA-014 local deterministic image build create/cancel, workload submit/cancel, scheduler preemption, and workload deploy/apply retry-idempotency evidence, DATA-016 local/in-memory authorization-policy projection drift-check coverage, DATA-016 local/in-memory IDE projection drift-check coverage for six IDE read-model pairs, DATA-016 local/in-memory dashboard projection drift-check coverage for six dashboard read-model pairs, DATA-016 local/in-memory clusterread projection drift-check coverage for six clusterread read-model pairs, DATA-016 local/in-memory request-notification project-access drift-check coverage for three project-access read-model pairs, DATA-016 local/in-memory GPU usage projection drift-check coverage for five GPU usage read-model pairs, DATA-016 local/in-memory image-registry projection drift-check coverage for five image-registry access read-model pairs, DATA-016 local/in-memory storage projection drift-check coverage for five storage read-model pairs, DATA-017/DATA-018 local outbox/consumer-lag/replay/dead-letter metrics evidence, STORAGE-001 local/in-memory mount-plan authorization proof, STORAGE-002 local/in-memory cross-Project and cross-user mount-plan isolation proof, STORAGE-003 local handler-level permission-management RBAC proof, identity auth/session local/static external API fixture coverage, request-notification create-form local/static external API fixture coverage, image-registry Dockerfile build local/static external API fixture coverage, workload submit-job local/static external API fixture coverage, workload create-configfile/update-configfile/delete-configfile local/static external API fixture coverage with ConfigFileChanged event metadata repair, workload cancel-job local/static external API fixture coverage, org-project create-project local/static external API fixture coverage, org-project project update/delete/batch-delete local/static external API fixture coverage, org-project create-group/update-group/delete-group/batch-delete-group local/static external API fixture coverage, storage permission create local/static external API fixture coverage, storage project permission delete local/static external API fixture coverage, storage project permission batch update/delete local/static external API fixture coverage, migration-runner ledger/checksum/lock/dirty code slice with live PostgreSQL temporary-schema/dirty/checksum/adoption/lock evidence, OPS-011 Redis/event-broker outage evidence, partial OPS-013 Prometheus/telemetry stale and quota non-grant evidence, Harbor dependency/status outage evidence, OPS-012 image-registry build/list degraded-route evidence, PERF stream credential issuance p95 evidence |

The original spec covers the **compute, GPU, queue, RBAC, image, and security
core** thoroughly. The original v1 gap review identified the **supporting
surfaces every real deployment touches** — Web UI, storage, secrets, audit
query, Plan/Queue administration, and basic gateway abuse limits. Several of
those proposed slices now have evidence, but that evidence must not be read as
Full GA completion; the remaining GA gaps are tracked in `gap.md`.

DATA-014 deploy/apply retry evidence now exists locally: the workload dispatcher
selects a `waiting_infra` job whose `next_retry_at` is due, retries manifest
creation against a pre-existing Kubernetes Job, relies on
`cluster.Client.CreateByJSON` treating `AlreadyExists` as success, and marks the
job `running` with a single `created_resources` entry. This is local
control-plane/fake-client evidence only; it does not prove live Kubernetes
deploy replay, live rollback behavior, typed ownership completion, or Full GA.

RBAC-016 now has local catalog-driven public route coverage:
`TestRBACPublicAPIRoutesRequireAuthUnlessExplicitlyAllowed` checks registered
external `/api/v1/` routes, excluding `/api/v1/internal/` and service-auth
routes. Intentional public auth/OIDC entry routes require exact method+pattern
allowlist reasons; all other scoped routes must require auth and return `401`
through `app.ServeHTTP` with no credentials. This is not live gateway proof,
every business authorization branch, full RBAC GA, Full GA, or first-version
completion.

RBAC-017 now has local/static OpenAPI metadata parity evidence: generated
operations mirror `RouteSpec` auth flags, include service-auth metadata and
schemes for `X-Service-Name`/`X-Service-Key`, omit public-route security, and
model combined user+service auth as requiring both categories. This is not live
gateway proof, service credential rotation, workload identity, mTLS/SPIFFE,
full RBAC GA, Full GA, or first-version completion.

DATA-016 now has local/in-memory authorization-policy, IDE, dashboard,
clusterread, request-notification project-access, GPU usage, and image-registry
projection drift-check coverage: the authorization-policy
repository compares raw owner/source resources with local authorization-policy
read-model resources for identity users/roles and policy projects/plans/image
allow lists, the IDE repository compares raw owner/source resources with six
local IDE read-model pairs for identity users/roles, policy roles, projects,
project members, and user groups, the dashboard helper compares raw
owner/source resources with six local dashboard read-model pairs for users,
projects, project members, forms, live quotas, and queues, and the clusterread
helper compares raw owner/source resources with six local clusterread
read-model pairs for identity users/roles, policy roles, projects, project
members, and user groups, and the request-notification project-access
repository compares raw org-project source resources with three local
request-notification project-access read-model pairs for projects, project
members, and user groups, and the GPU usage helper compares raw owner/source
resources with five local GPU usage read-model pairs for identity users,
identity roles, authorization-policy roles, org projects, and workload jobs,
the storage helper compares raw owner/source resources with five local storage
read-model pairs for identity users, identity roles, projects, project members,
and user groups, and the image-registry helper compares raw owner/source
resources with five
local image-registry access read-model pairs for identity users, identity
roles, org projects, project members, and user groups.
Focused tests cover missing, orphan, stale, clean, deterministic ordering,
canonical id normalization, nil app/store fail-closed behavior,
projection-pair coverage, GPU usage snapshot/summary exclusion,
request-notification source guard coverage, image-registry
catalog/build/image-request/sync exclusion, excluded clusterread
policy-assignment and read-model telemetry resources, blank-id skip behavior,
storage canonical id normalization coverage, dashboard/clusterread/
request-notification/GPU usage/image-registry
co-hosted fallback-trap coverage. This is local/in-memory helper evidence only, not a
live drift job, read-model rebuild/replay cutover, all-service DATA-016
coverage, DATA GA, Full GA, first-version readiness, rebuild/replay cutover
readiness, or production readiness.

Identity auth/session routes now have local/static external API fixture coverage
for `POST /api/v1/register`, `POST /api/v1/login`, `POST /api/v1/refresh`, and
`POST /api/v1/cli/login`: the fixture validator checks public auth metadata,
exact required credential fields, narrowly-scoped credential example field
allowances, additive/tolerant decoding, success statuses, and `UserCreated`
only for registration, while the service parity test checks the fixtures
against `identity.Spec()`. This is not live auth availability, browser cookie
behavior, OIDC/LDAP behavior, token rotation/revocation proof, all-critical-API
typed contract coverage, DATA GA, Full GA, or first-version completion.

Request-notification create-form now has local/static external API fixture
coverage for `POST /api/v1/forms`: the fixture validator checks metadata,
required request fields, forbidden example keys, additive/tolerant decoding,
and request/response example shape, while the service parity test checks the
fixture against `requestnotification.Spec()`. This is not OpenAPI-first
completion, all-critical-API typed contract coverage, DATA GA, Full GA, or
first-version completion.

Image-registry Dockerfile build submission now has local/static external API
fixture coverage for `POST /api/v1/images/build/dockerfile`: the fixture
validator accepts non-empty all-2xx success statuses, the fixture uses `202`,
required request fields are `project_id` and `image_reference`, forbidden
example keys and additive/tolerant decoding are covered for all external API
fixtures, and the service parity test checks `imageregistry.Spec()` metadata
including user auth, no service key, no path params, state-changing behavior,
`harbor` adapter metadata, and `ImageBuildStarted`. This is not live Harbor
build execution, SBOM/signing, allow-list enforcement, image scan lifecycle,
full image workflow, Full GA, or first-version completion.
Image-registry profile and supply-chain status metadata now also have local
evidence: `ImageAccelerationProfile` CRUD/seed/fixture/event coverage exists,
and queued image builds expose `image_digest=""`,
`allow_list_decision="pending"`, `sbom_status="pending"`,
`signature_status="pending"`, `scan_status="pending"`, and
`supply_chain_checked_at=null` in create responses, stored records, and
`ImageBuildStarted` events. Contract coverage also validates that historical
`ImageBuildStarted` payloads without those additive keys remain schema-v1
compatible. This remains local metadata/contract evidence only and does not
close live SBOM/signing/scan, allow-list enforcement, image conversion/prewarm,
live Harbor/Tekton/BuildKit execution, full IMG, V1 external launch, or Full GA.

Workload job submission now has local/static external API fixture coverage for
`POST /api/v1/jobs`: the fixture validator checks metadata, exact required
request fields `project_id` and `user_id`, forbidden example keys,
additive/tolerant decoding, `201 Created`, and `JobSubmitted`, while the
service parity test checks `workload.Spec()` metadata including user auth, no
service key, no path params/ID param, non-admin state-changing route behavior,
success status, and event metadata. Queue/resource/config/streaming fields are
optional UI/admission/defaultable payload examples only. This is not live
scheduler admission, queue policy completeness, Kubernetes job execution,
logs/tailing, GPU telemetry, WEB-003/WEB-004 completion, Full GA, or
first-version completion.

Workload ConfigFile creation now has local/static external API fixture coverage
for `POST /api/v1/configfiles`: the fixture validator checks metadata, exact
required request fields `project_id` and `name`, forbidden example keys,
additive/tolerant decoding, `201 Created`, source-backed error statuses, and
`ConfigFileChanged` as handler-emitted create evidence, while the service parity
test checks `workload.Spec()` metadata including user auth, no service key, no
path params/ID param, non-admin state-changing route behavior, and success
status. Accepted aliases and payload fields remain optional. This is not live
scheduler admission, Kubernetes job execution, logs/tailing, GPU telemetry,
WEB-003/WEB-004 completion, DATA GA, Full GA, or first-version completion.

Workload ConfigFile deletion now has local/static external API fixture coverage
for `DELETE /api/v1/configfiles/{id}`: the fixture validator checks metadata,
exact path parameter `id`, empty required and optional request fields, empty
request object, forbidden example keys, additive/tolerant decoding, `200 OK`,
source-backed error statuses, and `ConfigFileChanged` as handler-emitted delete
evidence, while the service parity test checks `workload.Spec()` metadata
including user auth, no service key, `id` route parameter/ID param, non-admin
state-changing route behavior, response shape, and event metadata. This is not
live ConfigFile deletion, project isolation, event delivery, scheduler
admission, Kubernetes job execution, ConfigFile runtime rollout, logs/tailing,
GPU telemetry, WEB-003/WEB-004 completion, DATA GA, Full GA, or first-version
completion.

Workload ConfigFile update now has local/static external API fixture coverage
for `PUT /api/v1/configfiles/{id}`: the fixture validator checks metadata,
exact path parameter `id`, exact required request field `content`, optional
`name`/`filename`/`path`/`manifest`/`yaml`/`config` and same-Project
`projectId`/`project_id` aliases, forbidden example keys, additive/tolerant
decoding, `200 OK`, source-backed error statuses, and `ConfigFileChanged` as
handler-emitted update evidence, while the service parity test checks
`workload.Spec()` metadata including user auth, no service key, `id` route
parameter/ID param, non-admin state-changing route behavior, response shape,
and event metadata. The optional project aliases do not imply cross-Project
ConfigFile moves are supported. This is not live ConfigFile update, project
isolation, event delivery, ConfigFile runtime rollout, logs/tailing, GPU
telemetry, WEB-003/WEB-004 completion, DATA GA, Full GA, or first-version
completion.

Workload ConfigFile read now has local/static external API fixture coverage for
`GET /api/v1/configfiles/{id}`: the fixture validator checks metadata, exact
path parameter `id`, empty required and optional request fields, empty request
object, forbidden example keys, additive/tolerant decoding, `200 OK`,
source-backed error statuses, and no emitted events, while the service parity
test checks `workload.Spec()` metadata including user auth, no service key,
`id` route parameter/ID param, non-admin read-only route behavior, response
shape, and no events. This is not live ConfigFile read proof, project
isolation, event delivery, ConfigFile runtime rollout, logs/tailing, GPU
telemetry, WEB-003/WEB-004 completion, DATA GA, Full GA, or first-version
completion.

Workload ConfigFile version commit now has local/static external API fixture
coverage for `POST /api/v1/configfiles/{id}/versions`: the fixture validator
checks metadata, exact path parameter `id`, exact required request field
`content`, optional `message`/`manifest`/`yaml`/`config`, forbidden example
keys, additive/tolerant decoding, `201 Created`, source-backed error statuses,
and `ConfigCommitted`, while the service parity test checks `workload.Spec()`
metadata including user auth, no service key, `id` route parameter/ID param,
non-admin state-changing route behavior, response shape, and event metadata.
This is not live scheduler admission, Kubernetes job execution, ConfigFile
runtime rollout, logs/tailing, GPU telemetry, WEB-003/WEB-004 completion, DATA
GA, Full GA, or first-version completion.

Org-project Project creation now has local/static external API fixture coverage
for `POST /api/v1/projects`: the fixture validator checks metadata, exact
required request fields `project_name` and `g_id`, forbidden example keys,
additive/tolerant decoding, `201 Created`, source-backed error statuses, and
`ProjectCreated`, while the service parity test checks `orgproject.Spec()`
metadata including user auth, no service key, no path params/ID param,
non-admin route metadata, state-changing route behavior, and event metadata.
Conservative aliases and policy/quota fields remain optional. This is not live
admin authorization proof, full Project lifecycle, tenant isolation, DATA GA,
Full GA, or first-version completion.

Org-project Project update now has local/static external API fixture coverage
for `PUT /api/v1/projects/{id}`: the fixture validator checks metadata, exact
path param `id`, required request field `project_name`, mutable optional
Project fields, forbidden example keys, additive/tolerant decoding, `200 OK`,
configured error statuses, and `ProjectUpdated`, while the service parity test
checks `orgproject.Spec()` metadata including user auth, admin route metadata,
state-changing route behavior, no service key, direct Project response shape,
and event metadata. This is not live admin authorization proof, full Project
lifecycle, tenant isolation, DATA GA, Full GA, or first-version completion.

Org-project Project delete now has local/static external API fixture coverage
for `DELETE /api/v1/projects/{id}`: the fixture validator checks metadata,
exact path param `id`, no-body DELETE request shape, empty response example,
forbidden example keys, additive/tolerant decoding, `200 OK`, configured error
statuses, and `ProjectDeleted`, while the service parity test checks
`orgproject.Spec()` metadata including user auth, non-admin route metadata,
state-changing route behavior, no service key, and event metadata. This is not
live admin authorization proof, full Project lifecycle, tenant isolation, DATA
GA, Full GA, or first-version completion.

Org-project Project batch delete now has local/static external API fixture
coverage for `DELETE /api/v1/projects/batch`: the fixture validator checks
metadata, no path params, required top-level `ids`, forbidden example keys,
additive/tolerant decoding, `200 OK`, configured error statuses, and
`ProjectDeleted`, while the service parity test checks `orgproject.Spec()`
metadata including user auth, non-admin route metadata, state-changing route
behavior, no service key, canonical project IDs, and direct
`succeeded`/`failed`/`errors` batch result response shape. This is not live
admin authorization proof, full Project lifecycle, tenant isolation, DATA GA,
Full GA, or first-version completion.

Org-project Group creation now has local/static external API fixture coverage
for `POST /api/v1/groups`: the fixture validator checks metadata, exact
required request field `group_name`, forbidden example keys, additive/tolerant
decoding, `201 Created`, source-backed error statuses, and `GroupCreated`,
while the service parity test checks `orgproject.Spec()` metadata including
user auth, no service key, no path params/ID param, admin route metadata,
state-changing route behavior, direct group response shape, and event metadata.
Conservative aliases and policy fields remain optional. This is not live admin
authorization proof, full Group lifecycle, tenant isolation, DATA GA, Full GA,
or first-version completion.

Org-project Group update now has local/static external API fixture coverage for
`PUT /api/v1/groups/{id}`: the fixture validator checks metadata, exact path
param `id`, required request field `group_name`, mutable optional Group fields,
forbidden example keys, additive/tolerant decoding, `200 OK`, configured error
statuses, and `GroupUpdated`, while the service parity test checks
`orgproject.Spec()` metadata including user auth, admin route metadata,
state-changing route behavior, no service key, direct group response shape, and
event metadata. `GroupUpdated` is now also listed in `orgproject.Spec().Events`
to match the existing handler emission. This is not live admin authorization
proof, full Group lifecycle, tenant isolation, DATA GA, Full GA, or
first-version completion.

Org-project Group delete now has local/static external API fixture coverage for
`DELETE /api/v1/groups/{id}`: the fixture validator checks metadata, exact path
param `id`, no-body DELETE request shape, empty response example, forbidden
example keys, additive/tolerant decoding, `200 OK`, configured error statuses,
and `GroupDeleted`, while the service parity test checks `orgproject.Spec()`
metadata including user auth, admin route metadata, state-changing route
behavior, no service key, empty response shape, and event metadata.
`GroupDeleted` is now also listed in `orgproject.Spec().Events` to match the
existing handler emission. This is not live admin authorization proof, full
Group lifecycle, tenant isolation, DATA GA, Full GA, or first-version
completion.

Org-project Group batch delete now has local/static external API fixture
coverage for `DELETE /api/v1/groups/batch`: the fixture validator checks
metadata, DELETE-with-body request shape with required top-level `ids`,
forbidden example keys, additive/tolerant decoding, `200 OK`, configured error
statuses, and `GroupDeleted`, while the service parity test checks
`orgproject.Spec()` metadata including user auth, admin route metadata,
state-changing route behavior, no service key/path params, canonical group IDs,
direct batch result response shape, and event metadata. This is not live admin
authorization proof, full Group lifecycle, tenant isolation, DATA GA, Full GA,
or first-version completion.

Storage permission creation now has local/static external API fixture coverage
for `POST /api/v1/storage/permissions`: the fixture validator checks metadata,
exact required request fields `group_id`, `pvc_id`, `user_id`, and
`permission`, forbidden example keys, additive/tolerant decoding, `200 OK`,
configured error statuses, and `StoragePermissionChanged`, while the service
parity test checks `storage.Spec()` metadata including user auth, no service
key, no path params/ID param, non-admin state-changing route behavior, no
adapter, direct permission response shape, and event metadata. This is not live
permission enforcement, Kubernetes mount execution, cluster PVC isolation,
namespace enforcement, CSI behavior, storage GA, Full GA, or first-version
completion.

Storage project permission update now has local/static external API fixture
coverage for
`PUT /api/v1/projects/{id}/storage/bindings/{pvcId}/permissions`: the fixture
validator checks metadata, exact path params `id` and `pvcId`, required request
fields `user_id` and `permission`, no optional fields, forbidden example keys,
additive/tolerant decoding, `200 OK`, configured error statuses, and
`ProjectStoragePermissionChanged`, while the service parity test checks
`storage.Spec()` metadata including user auth, no service key, `pvcId` route ID
param, non-admin state-changing route behavior, no adapter, direct project
permission response shape, and event metadata. This is not live permission
enforcement, Kubernetes mount execution, cluster PVC isolation, namespace
enforcement, CSI behavior, storage GA, Full GA, or first-version completion.

Storage project permission delete now has local/static external API fixture
coverage for
`DELETE /api/v1/projects/{id}/storage/bindings/{pvcId}/permissions/{userId}`: the
fixture validator checks metadata, exact path params `id`, `pvcId`, and
`userId`, no required/optional request fields, forbidden example keys,
additive/tolerant decoding, `200 OK`, configured error statuses, and
`ProjectStoragePermissionChanged`, while the service parity test checks
`storage.Spec()` metadata including user auth, no request body, `userId` route ID
param, non-admin state-changing route behavior, no service key, no adapter, and
event metadata. This is not live permission enforcement, Kubernetes mount
execution, cluster PVC isolation, namespace enforcement, CSI behavior, storage
GA, Full GA, or first-version completion.

Storage project permission batch update and batch delete now have local/static
external API fixture coverage for
`PUT /api/v1/projects/{id}/storage/bindings/{pvcId}/permissions/batch` and
`DELETE /api/v1/projects/{id}/storage/bindings/{pvcId}/permissions/batch`: the
fixture validator checks metadata, exact path params `id` and `pvcId`, required
top-level `items` request bodies, forbidden example keys, additive/tolerant
decoding, `200 OK`, configured error statuses, and
`ProjectStoragePermissionChanged`, while the service parity tests check
`storage.Spec()` metadata including user auth, `pvcId` route ID param,
non-admin state-changing route behavior, no service key, no adapter, canonical
item fields, direct `succeeded`/`failed`/`errors` batch result responses, and
event metadata. This is not live permission enforcement, Kubernetes mount
execution, cluster PVC isolation, namespace enforcement, CSI behavior, storage
GA, Full GA, or first-version completion.
