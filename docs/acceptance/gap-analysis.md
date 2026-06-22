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

## Blocks-v1 — referenced by existing flows but has no acceptance coverage

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
`WEB-007` have partial
read-only surfaces: the Images panel calls existing Project image and
image-build routes, and live seeded E2E proves a created image build appears in
the active Project's build list. The Usage panel calls existing current-user
usage, request-usage, and active-Project GPU usage routes. Unit tests cover
image/build rows, active-Project usage filtering, forbidden Project GPU usage
rendering, no admin-usage fallback, and no browser credential persistence.
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
Remaining WEB gaps are full WebRTC media session, real workload GPU
telemetry/utilization evidence, continuous log tailing/full status workflow
evidence, full usage workflow evidence, full image-build/allow-list/SBOM/
signing/GUI scan workflow, Harbor scan lifecycle synchronization, and
registry-wide automatic delete lifecycle automation beyond explicit per-tag
sync/delete-resync. Browser-operated WebRTC sessions remain covered by the RTC
family and are not a substitute for a management Web UI.

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

| ID | Proposed Acceptance Criteria |
|---|---|
| PLANADMIN-001 (PROPOSED) | Only platform admin (or delegated platform_manager) can create/edit/expire Plans and Queues. |
| PLANADMIN-002 (PROPOSED) | Plan/Queue mutations produce audit events with actor and before/after values. |
| PLANADMIN-003 (PROPOSED) | Attaching/detaching a Project Plan is authorized and audited; a Project always resolves to at most one active Plan. |

### GATE-* — gateway abuse controls and manifest size bound
Boundaries mention "coarse rate limits" but there is no AC, and §5 preflight
parses multi-document YAML with no size bound (a DoS vector). A public first
launch needs basic protection.

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
  Put numeric RTO/RPO targets on them after the first load test.
- **i18n / accessibility ACs** for the Web UI — product polish.
- **Billing** — explicitly future in the monitoring goal.

## Strengthen-existing — small additions to current families (not new families)

- **K8S:** add a manifest size / document-count cap AC (also see GATE-002) so the
  preflight DoS surface is closed inside the K8S family too.
- **SEC / CLI:** add an explicit session + refresh-token expiry/rotation AC.
  Implementation evidence now covers one-time refresh rotation/replay rejection,
  session expiry, internal expired/revoked credential rejection, and cleanup of
  expired/revoked credentials.
- **DATA:** replay idempotency has implementation evidence: projection replay
  retries only unresolved dead-letter events and tests assert that successful
  events are not double-applied. Transactional outbox coupling now includes the
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
  `409` / `GPU quota exceeded` while Prometheus was not configured. Harbor
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
| Blocks-v1 | STORAGE-*, SECRET-*, AUDIT-*, PLANADMIN-*, GATE-* |
| Defer | NOTIF, IDE, DR RTO/RPO numbers, i18n/a11y, billing |
| Conditional | WEB-* blocks any release that advertises the NexusPaaS management Web UI as GA; the current `/ui/` implementation is partial and must stay labeled as such until the WEB ACs pass. |
| Strengthen-existing | K8S manifest size cap, SEC/CLI token lifecycle, OPS-011 Redis/event-broker outage evidence, partial OPS-013 Prometheus/telemetry stale and quota non-grant evidence, Harbor dependency/status outage evidence, OPS-012 image-registry build/list degraded-route evidence, PERF stream credential issuance p95 evidence |

The original spec covers the **compute, GPU, queue, RBAC, image, and security
core** thoroughly. The launch-blocking gaps are the **supporting surfaces every
real deployment touches** — Web UI, storage, secrets, audit query, Plan/Queue
administration, and basic gateway abuse limits.
