# NexusPaaS Current Architecture Blockers

_Updated: 2026-06-22 (re-verified). Branch: `ga-web-gate`. Full
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
seeded Project image. Bounded Harbor-to-catalog synchronization also has live
API evidence with trace `654e8a882af7e6a2099a5cce75a8377e` on
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
was restored and secret values were not printed.
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
This is still local evidence, not HA, external registry, off-cluster DR, 8-unit
staging rollback, full failure-injection coverage, full WebRTC media session,
continuous log tailing/full workload status, per-device GPU utilization
telemetry, or full
PERF-003..008 acceptance._

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
| Contract fixtures | Started | Core event, owner-read, command, producer, and consumer fixture coverage exists for initial slices. |
| Projection visibility | Started | Projection lag, retry, replay, and dead-letter visibility exist; durable transactional delivery is still open. |
| Transactional outbox/inbox | Done for delivery evidence | Outbox/inbox tables + relay + inbox dedupe wired in `runtime.go`. Single-record coupling via `App.*RecordWithEvent` and `App.UpsertRecordWithEvent`, plus multi-record coupling via `App.WithTx` (`StoreTx`/`RunInTx`), now cover single-record sites in 7 services plus non-batch authorizationpolicy mutations (role create/update, policy rule replacement update, policy assignments, role users, raw permissions), authorizationpolicy policy/role cascades, schedulerquota queue cascade, storage non-batch upsert/update/delete mutations and cascades, orgproject group/project delete cascades, and batch per-item/custom mutation coverage across authorizationpolicy, storage, schedulerquota, identity, orgproject, imageregistry, and workload. Reviewer-approved live RKE2 evidence captured final image `localhost:5000/nexuspaas-backend:ci-ga-pdp-scope-20260620163744` (`sha256:1817b0c42c37fe6e4d75e1155f7022084aac675dfb52857f16f7b45299b6af62`), 15 backend deployments ready, PDP service-key scope check, HTTP 201 `POST /api/v1/forms`, and matching `FormCreated` row in `platform_event_outbox`. Expanded-surface publish-lag evidence passed with storage API events `940df12e-f953-4460-bc06-3aa487209016`, `6c749a52-a980-4d9f-9f6b-834f5f6e0068`, and `a70177d7-f0da-48a8-84f3-44bee283a54f` reaching `published` / `relay_attempts=0` and appearing in Redis DB1. Relay recovery evidence passed for `ga-outbox-crash-20260621103919`: the row stayed pending under a short sentinel relay lease, a relay-capable pod restart occurred, the sentinel was released, the row reached `published|0|true`, Redis DB1 retained the event, and exact cleanup left zero synthetic outbox rows. This proves controlled relay unavailability plus relay-capable pod restart recovery, not handler mid-transaction crash interleavings or the exact restarted pod as publisher. |
| Route auth and collision hardening | Done | Centralized internal-route service auth and route collision validators are implemented, wired into startup checks, covered by focused/full tests, vet, build, quick gate, Sonar Quality Gate, reviewer approval, and live RKE2 missing/wrong/valid service-key evidence. |
| API token indexed lookup | Done | User API tokens now use `nexuspaas_<token-id>_<secret>` format; local and identity verification parse the id, load one record, verify one full-token hash, pass focused/full Go tests, quick gate, Sonar Quality Gate, reviewer approval, and live RKE2 identity auth evidence. |
| Trusted client IP resolution | Done | Identity login failures, captcha checks, cleanup, and API-token audit events now reuse the platform trusted-proxy resolver; focused/full Go tests, quick gate, Sonar Quality Gate, reviewer approval, RKE2 rollout, health checks, and live spoofed `X-Forwarded-For` evidence passed. |
| Environment profiles and PDP fail-closed | Done | Runtime config now supports explicit `APP_ENV` profiles (`local`, `test`, `dev`, `staging`, `production`), preserves legacy `PRODUCTION` fallback, rejects invalid/conflicting mode settings, uses strict startup/PDP checks for staging/production, declares `APP_ENV: "production"` in production backend manifests, and passed focused/full tests, quick gate, Sonar Quality Gate, reviewer approval, and live RKE2 rollout health/readiness evidence. |
| Production Beta operations | Partial | Non-live gates and docs exist, current live 15 first-party deployments have same-image Kubernetes rollout/undo evidence, OPS-006 PostgreSQL logical backup/restore drill passed, OPS-008 MinIO synthetic object restore drill passed, OPS-009 current-live Kubernetes Secret recovery copy drill passed, OPS-007 Harbor backup/restore passed after moving Harbor from unsupported Rancher `local-path` hostPath PVCs to Kubernetes static `local` PVs, Harbor-side push/scan/delete passed with official Trivy and `crane`, OPS-011 Redis/event-broker outage evidence passed (`ga-redis-outage-20260621202250`, event `33fc697b-2cac-4715-ac04-e46097b0ea99`), partial OPS-013 Prometheus/telemetry stale and quota non-grant evidence passed (`ci-ga-prometheus-stale-20260621205458`, trace `ga-prometheus-stale-20260621205959`), Harbor dependency outage evidence passed through `/api/v1/harbor-status` (`ga-harbor-outage-20260621200008`), OPS-012 image-registry build/list degraded-route outage evidence passed (`ga-image-harbor-degraded-20260621212113`, image `ci-ga-image-harbor-degraded-20260621211729`, digest `sha256:21345ac6ad43db05f489bfa2ee37122b0b79873daca6f04619506c8bcef9d319`), a bounded PERF read smoke baseline passed after correcting k6 endpoint selection (`20` VUs / `210` iterations / `30` requests per endpoint / `0` failures / total p95 `2306.00ms`), and Project list `100` VU live k6 evidence now passes on `ci-ga-pdp-enforce-20260622094936` (`100` temporary principals, `30s`, `/api/v1/projects` `1000/1000` 2xx, `0` failures, `0` 429, p95 `3.668ms`, enforce `429` count `0`, exact cleanup verified). Live 8-unit staging deploy/smoke/previous-image rollback/redeploy, full NexusPaaS image-build/allow-list/SBOM/signing/GUI scan workflow evidence, managed/off-cluster secret recovery, PITR/off-cluster DR, HA/off-cluster Harbor storage, full failure injection beyond the Redis/event-broker, Prometheus telemetry/admission, Harbor dependency/status API, OPS-012 build/list degraded-route slices, and PERF-003..008 evidence remain open. |

Additional Web/performance evidence: WEB-006 stream credential operation and
the stream credential p95 sub-target passed on
`ci-ga-web-stream-cred-20260622102018`
(`sha256:d14aa360d5f0e4273846c88a785a2ad8cafc570613e8d892a7d9ef4407c899b1`).
RTC-008 direct/relay candidate gathering also passed in the GUI E2E harness.
WEB-007 active-Project GPU usage has a live nonzero requested-GPU pod proof on
`ci-ga-gpu-readmodel-20260622034034` with `gpu_status=200`, `gpu_used=1`, and
`gpu_nonzero=true`. This does not close full WebRTC media session, per-device
GPU utilization telemetry, continuous log tailing/full workload status, or the
remaining PERF families.

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
| Reproducible toolchain | Local quick, Docker-backed, manifest rehearsal, 8-unit collaboration gates, full backend coverage run, and local Sonar Quality Gate are green. Remote CI and live external staging evidence remain open. Latest Sonar API status: `new_coverage=81.8`, `new_violations=0`, `new_duplicated_lines_density=0.8262`. | Keep local quick/Sonar/Docker-backed collaboration evidence green, provision remote CI/Sonar secrets, then capture live staging evidence per deployable unit. |
| Harbor DR storage maturity | OPS-007 local drill now passes on Kubernetes static `local` PVs, but this storage is single-node and manually pre-provisioned. The runbook must recreate matching static PVs before restore readiness and recreate an empty Redis PVC because Redis is intentionally excluded as cache. This proves Harbor backup/restore mechanics for local evidence, not production-grade HA/off-cluster DR. | Keep static local PV evidence as the OPS-007 drill proof, then move the GA DR path to Longhorn/CSI snapshots or another reviewed HA/off-cluster design with encrypted backup storage and retention evidence before claiming production-grade DR. |

## P1 Architecture Maturity

| Area | Current Problem | Next Step |
| --- | --- | --- |
| Service identity | `SERVICE_API_KEY` remains the Beta service-to-service fallback. | Move the GA path to Kubernetes workload identity, mTLS, SPIFFE/SPIRE, or scoped per-service keys with rotation. |
| JWT/JWKS verification | Security-sensitive token verification is mostly custom. | Replace or wrap it with a mature Go OIDC/JWT/JWK library through an ADR-backed slice. |
| Migration runner | SQL migration execution is still too simple for GA auditing and rollback. | Adopt `golang-migrate`, `goose`, Atlas, or add version/checksum/locking/dirty-state support. |
| Provider coupling | Longhorn, Harbor, MinIO, Dex, Redis Streams, and k3s are still reference-stack assumptions in several places. | Separate core contracts from provider implementations before claiming portability. |
| Typed API contracts | Custom route specs and generated OpenAPI help, but do not replace typed request/response contracts. | Move critical APIs toward OpenAPI-first or explicit typed DTO contracts with fixtures. |
| Read-model drift and replay | Replay idempotency now has focused evidence: dead-letter replay retries only unresolved events and does not double-apply successful events. Drift comparison and read-model rebuild evidence are still not enough for cutover. | Add projection drift checks before retiring owner-read/shared-store paths. |
| Per-unit runtime isolation | Non-live 8-unit runtime isolation is proven locally, and current live 15 first-party deployments now have same-image rollout/undo evidence. Deployable-unit RBAC, network policy, migration ownership, target 8-unit staging rollback, previous-image rollback, and live staging evidence are not fully proven. | Capture staging deploy, smoke, previous-image rollback, and redeploy evidence for each of the 8 units. |

## P2 Documentation And Tooling

| Area | Current Problem | Next Step |
| --- | --- | --- |
| Documentation alignment | Older docs described the project as microservices-first and listed non-current runtime dependencies. | Keep README, roadmap, architecture docs, and backend docs aligned with current reality. |
| CI script size | The central security gate is useful but large. | Split checks only when there is real maintenance pain; keep the top-level script as the orchestrator. |
| Service ownership docs | Service-level ownership is partially documented across several files. | Consolidate owner, API, data, config, test, and deployment responsibility per deployable unit. |
| Provider ADRs | Provider abstraction is a target but not yet documented as concrete ADRs. | Add ADRs when replacing or abstracting current reference-stack assumptions. |
| Supply chain | SBOM generation and image signing are GA goals but not enforced. | Add Syft/Cosign or equivalent after staging promotion is stable. |
| Remote Sonar | GitHub-hosted Sonar still depends on repository secrets. | Provision reachable Sonar credentials and make the remote gate required when configured. |

## Preserved Direction

- Keep the modular monolith while boundaries are being proven.
- Keep the 8 deployable-unit target instead of forcing a 15-way split.
- Keep the reference distribution as k3s + Longhorn + Harbor + MinIO + Dex +
  Redis Streams until provider abstractions are justified by concrete needs.
- Prefer deleting stale docs and consolidating status over adding more planning
  files.
