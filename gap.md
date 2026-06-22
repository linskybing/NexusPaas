# AC Completion — GA Gap Tracker

_Updated: 2026-06-22 (re-verified). Bar: **Full GA**
(`docs/acceptance/ga-checklist.md`), not just the v1 launch bar._

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
drill proof for the latest verified backend images, plus live Harbor foundation
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
`new_duplicated_lines_density=0.8262`.

## 1. Done — v1 launch-gap families (code-complete + tests green)

| Family | Evidence | Status |
|---|---|---|
| `GATE-*` + K8S manifest cap | `backend/internal/platform/input_limits.go`, `middleware.go` (429 `Retry-After`, `MaxBytesReader`/413), `config.go` limits | Done |
| `STORAGE-004` audit | `storage/mount_plan_contracts.go` `StorageMountPlanResolved` + project-scoped AuditEvent | Done |
| `SECRET-001..003` (v1 policy) | `schedulerquota/admission_resources.go` + `admission.go` reject raw `Secret`, safe `SecretAccessRejected`/`AuditEvent`; dispatcher defense-in-depth | Done |
| `AUDIT-001..004` | `auditcompliance/handler.go` read-time hash chain, CSV integrity columns, brand naming, project/group-scoped audit-log query RBAC with event-fed read models; `auditcompliance/cleanup.go` service-internal retention cleanup trigger | Done |
| `PLANADMIN-001..003` | `schedulerquota/handler.go` actor + old/new on Plan/Queue events | Done |
| DATA replay idempotency | `projection.go` targeted dead-letter replay + `events*.go` targeted inbox reset; tests prove replay retry does not double-apply previously successful events | Done |
| SEC/CLI token lifecycle strengthen | `identity/auth_repository.go`, `auth.go`, internal identity auth contracts, and cleanup worker enforce session expiry, one-time refresh rotation/replay rejection, API-token expiry/revocation, and expired/revoked credential cleanup; focused handler/internal-contract tests pass | Done |

Reference: `docs/plan/2026-06-20-v1-launch-gap-gate.md`.

## 2. Incomplete for GA

| AC family | What's missing | Evidence |
|---|---|---|
| `WEB-001..007` | Partial — first-party `frontend/` operations GUI exists and is served by `platform-gateway` at `/ui/`; live Playwright smoke passes. WEB-001 now has live OIDC browser-login evidence: Dex login through `platform-gateway` reached `/ui/?auth=oidc`, session cookie names existed without logging values, browser storage had no API key or token, and dashboard panels loaded through same-origin cookie auth on image `ci-ga-web-oidc-20260621203712` (`sha256:a4cc30f2f8b6b8b949c47949de186023ba61a8ad78ea52e72b927e17ea1d670b`). WEB-002 has live active-Project evidence: seeded E2E created a real Group/Project through existing REST routes, connected to `/ui/`, selected the seeded Project, and proved it was present in the active selector. WEB-003/WEB-004 have partial Workloads coverage: the GUI calls existing ConfigFile/job REST routes, lists ConfigFiles, filters authorized jobs by active Project for display, submits a minimal ConfigFile and Job for the active Project, sends job cancel requests, and reaches the existing job logs route from the browser. Earlier live seeded E2E submitted ConfigFile `CFG2600007`, submitted Job `e2e-job-mqneymza-1tqckn`, displayed that job, requested logs with `job_logs_status=200` / `job_logs_count=0`, and requested cancel with command `94925e294549528a2190b3dbafd09592`. WEB-005/WEB-007 remain partial read-only surfaces: Images lists Project images and image builds from existing image-registry routes, Usage lists current-user GPU/request usage and active-Project GPU usage from existing usage-observability routes, project GPU failures render as unavailable, and tests prove no admin-usage fallback or credential persistence. Harbor foundation now exists in `harbor-system`, and Harbor-side push/scan/delete evidence has passed with Trivy. WEB-005 catalog-derived image status display now has live API and Playwright GUI evidence under trace `ga-web-image-status-20260621214849`: a seeded Project image on `ci-ga-web-image-status-20260621214330` exposed top-level `scan_status="Success"`, `deleted=true`, `unavailable=false`, the seeded digest, and visible GUI state `deleted`. This proves UI/API display of read-model metadata. Bounded Harbor-to-catalog sync is also live-evidenced through `docs/plan/2026-06-21-harbor-catalog-sync-execution.md`: trace `654e8a882af7e6a2099a5cce75a8377e`, image `ci-ga-harbor-catalog-sync-reviewfix-20260621224351` (`sha256:3730083b5b028d8a592de463892ced37b399c07bc68aef1471b9d80214168939`), Harbor artifact digest `sha256:73aaf090f3d85aa34ee199857f03fa3a95c8ede2ffd4cc2cdb5b94e566b11662`, sync status `synced`, `code="ok"`, and exact cleanup verified. Explicit delete-resync lifecycle is live-evidenced through `docs/plan/2026-06-21-harbor-delete-lifecycle-sync.md`: image `ci-ga-harbor-delete-lifecycle-20260621225732` (`sha256:68b5eefe30ee0644edb4e5146523bce82ad83ea572d31fc7c7be6d5f3d1cccca`), artifact `library/nexuspaas-sync:ga-harbor-delete-lifecycle-20260621225849`, Harbor delete `200`, exact tag lookup `404`, re-sync `code="artifact_not_found"`, catalog `deleted=true`, `unavailable=true`, `status="missing"`, and exact cleanup verified. The WebRPC GUI contract for GA v1 is approved as existing same-origin REST/OpenAPI consumption; no separate WebRPC/tRPC/gRPC transport is required until a concrete API gap is proven. Earlier live seeded proof had `project_count=1`, `seeded_project_present=true`, `config_file_count=1`, `job_count=3`, `seeded_job_present=true`, `job_cancel_requested=true`, `job_logs_requested=true`, `job_logs_status=200`, `job_logs_count=0`, `image_count=1`, `build_count=1`, `gpu_status=200`, and `gpu_ok=true`; the WEB-005 status proof separately verified `scan_status="Success"` and state `deleted`; a direct live route probe returned `used=0` for a seeded Project with no GPU pods, and the later GPU read-model proof recorded `gpu_status=200`, `gpu_ok=true`, `gpu_used=1`, and `gpu_nonzero=true`; the later WEB-004 bounded pod-log proof recorded `job_logs_status=200`, `job_logs_count=1`, `job_logs_nonempty=true`, and `job_logs_visible=true`. Remaining GA Web scope still lacks WebRTC browser operation, real workload GPU utilization/per-device telemetry evidence, continuous log tailing/full status workflow evidence, Harbor scan lifecycle synchronization and registry-wide automatic delete lifecycle beyond explicit per-tag sync/delete-resync, the full image-build/allow-list/SBOM/signing/GUI scan workflow, and full WEB AC coverage. | `docs/plan/2026-06-22-gpu-usage-read-model-live-proof.md`; `docs/plan/2026-06-22-web-gui-oidc-browser-login.md`; `docs/plan/2026-06-22-oidc-gateway-forwarded-origin.md`; `docs/plan/2026-06-21-web-gui-job-logs-route-proof.md`; `docs/plan/2026-06-21-web-gui-job-submit-cancel-live-e2e.md`; `docs/plan/2026-06-21-gateway-adapter-route-proxy-precedence.md`; `docs/plan/2026-06-21-image-build-live-list-evidence.md`; `docs/plan/2026-06-21-clusterread-static-admin-gpu-usage.md`; `docs/plan/2026-06-21-web-gui-active-project-live-e2e.md`; `docs/plan/2026-06-21-orgproject-static-admin-compatibility.md`; `docs/plan/2026-06-21-web-gui-foundation-live-e2e.md`; `docs/plan/2026-06-21-web-gui-first-party-serving.md`; `docs/plan/2026-06-21-web-gui-project-selector.md`; `docs/plan/2026-06-21-web-gui-workload-workflows.md`; `docs/plan/2026-06-21-web-gui-image-usage-contract.md`; `docs/plan/2026-06-21-web-gui-image-status-parity.md`; `docs/plan/2026-06-21-gateway-downstream-catalog-proxy.md`; `docs/plan/2026-06-21-platform-admin-policy-bootstrap.md`; `docs/plan/2026-06-21-harbor-foundation-live-deploy.md`; `docs/plan/2026-06-21-harbor-foundation-credential-rebaseline.md`; `docs/plan/2026-06-21-harbor-image-scan-live-evidence.md`; `docs/plan/2026-06-21-harbor-delete-lifecycle-sync.md`; OIDC image `localhost:5000/nexuspaas-backend:ci-ga-web-oidc-20260621203712` (`sha256:a4cc30f2f8b6b8b949c47949de186023ba61a8ad78ea52e72b927e17ea1d670b`); image-registry image `localhost:5000/nexuspaas-backend:ci-ga-harbor-delete-lifecycle-20260621225732` (`sha256:68b5eefe30ee0644edb4e5146523bce82ad83ea572d31fc7c7be6d5f3d1cccca`); gateway image `localhost:5000/nexuspaas-backend:ci-ga-web-job-logs-20260621143553` (`sha256:3111ba2be88c8b0cb4c344f172e468253c8b8c862930763d426117776ab1a824`); WEB-005 status image `localhost:5000/nexuspaas-backend:ci-ga-web-image-status-20260621214330` (`sha256:613c1f2cefbab9f029afc580213dc619e7892753826c02367fa6ea6be129f83e`); previous gateway job-submit image `localhost:5000/nexuspaas-backend:ci-ga-web-job-submit-20260621141339` (`sha256:aee156e2904e03d30dc3d671545a1b9e86e45e27092a03645427663d2544ccc4`); previous gateway proxy-adapter image `localhost:5000/nexuspaas-backend:ci-ga-gateway-proxy-adapter-20260621054757` (`sha256:3cda2888dda836a1cd197c476c31342dd7e2f6f6befe5fa7e785ab46d13bc700`); org-project image `localhost:5000/nexuspaas-backend:ci-ga-org-static-admin-20260621125516` (`sha256:7310012c13eb9ee0667ac3f27eddf839c0d13c8d53b8b1560916762158b61471`); usage-observability/image-registry image `localhost:5000/nexuspaas-backend:ci-ga-clusterread-static-admin-20260621132623` (`sha256:f6c6ab5badac315095c4ac299fb2ded4fac8c4f29ae910f65bed16eb9368a87f`) |
| E2E (ga-checklist) | Full critical **live** E2E not evidenced — focused/local E2E plus local RKE2 single-namespace smoke exist. Latest live smokes prove PDP service-key scope, HTTP form create, durable outbox row, first-party `/ui/` dashboard rendering, gateway project-list routing, OIDC browser login through Dex and HttpOnly session cookies, active Project seeded selection, live GUI ConfigFile submit, live GUI job submit/cancel, live GUI job logs route consumption with bounded non-empty rendering, Project image list rendering, image build list rendering for a seeded build, catalog-derived Project image scan/deleted state rendering in the GUI, current-user usage/request-usage API rendering, active-Project GPU usage route success for a seeded Project with no GPU pods plus nonzero requested-GPU pod visibility evidence, current-live 15 first-party backend deployment same-image rollout/undo success, OPS-006 PostgreSQL logical backup/restore drill success, OPS-008 MinIO synthetic object restore drill success, OPS-009 current-live Kubernetes Secret recovery copy drill success, live Harbor foundation deploy/credential rebaseline readiness success, OPS-007 Harbor static-local Velero backup/restore drill success, Harbor-side Trivy push/scan/delete success, OPS-011 Redis/event-broker outage evidence, partial OPS-013 Prometheus/telemetry stale and quota non-grant evidence, Harbor dependency outage evidence through `/api/v1/harbor-status`, and OPS-012 image-registry build/list degraded-route outage evidence. They still are not the full critical-path E2E suite because WebRTC session launch, Harbor scan lifecycle synchronization and registry-wide automatic delete lifecycle beyond explicit per-tag sync/delete-resync, real workload GPU utilization/per-device telemetry evidence, continuous log tailing/full workload status, full NexusPaaS image-build/allow-list/SBOM/signing/GUI scan workflow evidence, managed/off-cluster secret recovery, PITR/off-cluster DR, target 8-unit/previous-image rollback per unit, full failure injection beyond the completed Redis/event-broker, Prometheus telemetry/admission, Harbor dependency/status API, and OPS-012 build/list degraded-route slices, and load/perf evidence remain open. | OIDC browser-login proof `docs/plan/2026-06-22-web-gui-oidc-browser-login.md` and `docs/plan/2026-06-22-oidc-gateway-forwarded-origin.md`, image `ci-ga-web-oidc-20260621203712` (`sha256:a4cc30f2f8b6b8b949c47949de186023ba61a8ad78ea52e72b927e17ea1d670b`), live Playwright reached `/ui/?auth=oidc`, observed only cookie names, found no browser storage API key/token, and loaded dashboard panels; `POST /api/v1/forms` -> `FormCreated` outbox row (`ga-outbox-live-20260620163919`); `platform-gateway /ui/` Playwright smoke and live `GET /api/v1/projects` through gateway on `ci-ga-admin-policy-20260621020259`; `ci-ga-web-workloads-20260620193544` rolled `/ui/` HTML/asset/Playwright smoke and live `GET /api/v1/configfiles`, `GET /api/v1/jobs` empty-list checks; `ci-ga-web-image-usage-20260621013315` rolled Images/Usage panels; gateway proxy-adapter image `ci-ga-gateway-proxy-adapter-20260621054757`; gateway job-submit image `ci-ga-web-job-submit-20260621141339`; gateway job-logs image `ci-ga-web-job-logs-20260621143553`; seeded active-Project E2E route proof `project_count=1`, `seeded_project_present=true`, `config_file_count=1`, `job_count=3`, `seeded_job_present=true`, `job_cancel_requested=true`, `job_cancel_command_id=94925e294549528a2190b3dbafd09592`, `job_logs_requested=true`, `job_logs_status=200`, `job_logs_count=0`, `image_count=1`, `build_count=1`, `gpu_status=200`, `gpu_ok=true`; WEB-005 status proof `docs/plan/2026-06-21-web-gui-image-status-parity.md` (`ga-web-image-status-20260621214849`, image `ci-ga-web-image-status-20260621214330`, digest `sha256:613c1f2cefbab9f029afc580213dc619e7892753826c02367fa6ea6be129f83e`, `scan_status="Success"`, state `deleted`); direct GPU route probe `status=200 used=0`; GPU read-model proof `gpu_status=200`, `gpu_ok=true`, `gpu_used=1`, `gpu_nonzero=true`; WEB-004 bounded pod-log proof `job_logs_status=200`, `job_logs_count=1`, `job_logs_nonempty=true`, `job_logs_visible=true`; current-live rollout/undo plan `docs/plan/2026-06-21-current-live-rollout-undo-evidence.md`; PostgreSQL restore drill `docs/plan/2026-06-21-postgres-backup-restore-drill.md`; MinIO object restore drill `docs/plan/2026-06-21-minio-object-restore-drill.md`; Kubernetes Secret recovery drill `docs/plan/2026-06-21-kubernetes-secret-recovery-drill.md`; Harbor foundation/rebaseline plans `docs/plan/2026-06-21-harbor-foundation-live-deploy.md` and `docs/plan/2026-06-21-harbor-foundation-credential-rebaseline.md`; blocked Harbor Velero attempt `docs/plan/2026-06-21-harbor-velero-backup-restore-drill.md`; Harbor static local Velero drill `docs/plan/2026-06-21-harbor-static-local-pv-velero-drill.md`; Harbor scan plan `docs/plan/2026-06-21-harbor-image-scan-live-evidence.md`; Harbor outage plan `docs/plan/2026-06-21-harbor-outage-failure-injection.md` (`ga-harbor-outage-20260621200008`); image-registry Harbor degraded build/list plan `docs/plan/2026-06-21-image-registry-harbor-degraded-build-list.md` (`ga-image-harbor-degraded-20260621212113`, image `ci-ga-image-harbor-degraded-20260621211729`, digest `sha256:21345ac6ad43db05f489bfa2ee37122b0b79873daca6f04619506c8bcef9d319`); Redis outage plan `docs/plan/2026-06-21-redis-event-broker-outage-evidence.md` (`ga-redis-outage-20260621202250`, event `33fc697b-2cac-4715-ac04-e46097b0ea99`); Prometheus stale/quota plan `docs/plan/2026-06-21-prometheus-stale-quota-evidence.md`, image `ci-ga-prometheus-stale-20260621205458` (`sha256:43a52e3875fa2ae9c0febf9a158537b7cdaeed97c7ef285f00ab9f5fee194a86`), trace `ga-prometheus-stale-20260621205959`; org-project static admin compatibility image `ci-ga-org-static-admin-20260621125516`; usage-observability/image-registry static admin compatibility image `ci-ga-clusterread-static-admin-20260621132623` |
| Backup/restore | Partial — OPS-006 PostgreSQL logical backup/restore drill passed live: `pg_dump -Fc` created a non-empty archive (`353198` bytes, SHA-256 `6c0869c3e591e9768edceee55feb80cbdf1e61e2e67b162a0b9a8bf6424a1c71`), `pg_restore --exit-on-error --single-transaction` restored into temporary DB `nexuspaas_restore_ops006_20260621150759`, restored public table count matched `81`, selected row counts matched, and temp DB/local dump cleanup passed. OPS-008 MinIO synthetic object drill passed live: object `media/ops008/20260621151838/payload.txt` was uploaded, backed up, deleted, restored, downloaded, SHA-256 matched (`dc4c12603462b5385f6cbb676cff88ba6503e1aba4b42ef10224c0b276c76d5b`), then remote object/local artifacts were cleaned. OPS-009 current-live Kubernetes Secret recovery copy drill passed for 21 selected Secrets without printing values or hashes. OPS-007 Harbor backup/restore now passed live after replatforming Harbor from unsupported Rancher `local-path` hostPath PVCs to Kubernetes static `local` PVs: Harbor chart `harbor-1.19.1` / app `2.15.1` restored ready, Velero chart `velero-12.0.3` / app `1.18.1` used dedicated BackupStorageLocation `ops007-static-20260621161910`, Backup `ops007-harbor-static-20260621161910` completed with `errors=0` / `warnings=0`, non-Redis PVBs completed for `database-data` (`51369080/51369080`), `registry-data` (`862/862`), and `job-logs` (`0/0`), `harbor-system` and exact static PVs were deleted, local PV directories were emptied, exact static PVs and the intentionally excluded empty Redis PVC were recreated, Restore `ops007-harbor-static-restore-20260621161910` completed with `errors=0` / `warnings=1` from Velero nodeOS detection only, matching PVRs completed, restored API ping returned `200`, read-only was unset, ORAS resolved and pulled digest `sha256:c7837e0a80dc7266b26eb197901b3ce8c3b893dc5ecb70208d13aab58dc70c46`, restored payload matched, synthetic Harbor data was cleaned, and final Harbor state was read-write with only the `library` project. Remaining GA gaps: managed/off-cluster secret recovery, Vault/External Secrets/Sealed Secrets/SOPS/KMS recovery, secret rotation/revocation, live `*-dev-*` secret-reference removal, PITR, off-cluster retention, versioned object restore, bucket metadata/IAM restore, encrypted backup storage, HA/off-cluster Harbor storage, and full DR. | `docs/plan/2026-06-21-postgres-backup-restore-drill.md`; `docs/plan/2026-06-21-minio-object-restore-drill.md`; `docs/plan/2026-06-21-kubernetes-secret-recovery-drill.md`; `docs/plan/2026-06-21-harbor-foundation-live-deploy.md`; `docs/plan/2026-06-21-harbor-foundation-credential-rebaseline.md`; `docs/plan/2026-06-21-harbor-velero-backup-restore-drill.md`; `docs/plan/2026-06-21-harbor-static-local-pv-velero-drill.md`; context `default`; namespace `nexuspaas`; Harbor namespace `harbor-system`; Harbor chart `harbor-1.19.1`; Harbor app `2.15.1`; Velero chart `velero-12.0.3`; Velero app `1.18.1`; `postgres:16-alpine`; `minio/minio:RELEASE.2025-04-08T15-41-24Z`; OPS-009 stamp `20260621153156`; selected Postgres counts: `platform_records=387`, `platform_event_outbox=1193`, `users=0`, `org_project_records=0`, `workload_records=0` |
| Rollback | Partial — current live `nexuspaas` namespace has same-image Kubernetes controller rollback-path evidence for 15 first-party backend deployments: each completed serial `rollout restart`, `rollout status`, `rollout undo --to-revision=<pre_revision>`, final `rollout status`, final image equality, and final ready replicas equal desired replicas. Remaining GA rollback gaps: target 8-unit Production Beta staging rollback, previous-image rollback, schema-change rollback, backup/restore, and full external GA rollback evidence. | `docs/plan/2026-06-21-current-live-rollout-undo-evidence.md`; context `default`; namespace `nexuspaas`; all selected deployments final `1/1` ready |
| Failure injection | Partial — OPS-011 Redis/event-broker outage is evidenced live: Redis was scaled from `1` to `0`, a direct `storage-service` pod request returned HTTP `201`, exact `GroupStorageCreated` event `33fc697b-2cac-4715-ac04-e46097b0ea99` was durable in Postgres as `pending|0|false` during the outage, Redis was restored to `1/1`, natural relay-lease expiry was respected without deleting the lease, the event reached `published|0|true`, Redis DB1 retained it, and exact Postgres synthetic rows were cleaned. Partial OPS-013 Prometheus/telemetry evidence is also live: `usage-observability-service` image `ci-ga-prometheus-stale-20260621205458` returned telemetry metadata on `/api/v1/cluster/summary` and `/api/v1/projects/{id}/gpu-usage`, `/api/v1/cluster/mps` returned degraded Prometheus adapter status `adapter_not_configured`, and scheduler admission under trace `ga-prometheus-stale-20260621205959` rejected an over-quota synthetic request with HTTP `409` / `GPU quota exceeded` while Prometheus was not configured; all exact synthetic rows were cleaned. Harbor dependency/status API outage is also evidenced live: `HARBOR_URL` was added through 12-factor runtime config, `/api/v1/harbor-status` returned healthy before injection, `harbor-core` was scaled from `1` to `0`, the product API returned retryable degraded Harbor status with `degraded.code="adapter_unavailable"`, `harbor-core` was restored to `1/1`, and `/api/v1/harbor-status` returned healthy again without process refresh. OPS-012 image-registry build/list degraded behavior is now evidenced live: `image-registry-service` image `ci-ga-image-harbor-degraded-20260621211729` kept project image list, project build list, and build submission responses successful with unchanged local data while adding retryable Harbor degraded metadata during `harbor-core=0`; recovery returned no degraded envelope and exact synthetic cleanup left `cleanup_leftovers=0`. Full NexusPaaS image-build/allow-list/SBOM/signing/GUI scan workflow evidence and full OPS-019 still remain open for DB, K8s API, live Prometheus interruption, node usage-agent failure, and other fault domains. | Redis outage: `docs/plan/2026-06-21-redis-event-broker-outage-evidence.md`, trace `ga-redis-outage-20260621202250`, event `33fc697b-2cac-4715-ac04-e46097b0ea99`; Prometheus stale/quota: `docs/plan/2026-06-21-prometheus-stale-quota-evidence.md`, image `ci-ga-prometheus-stale-20260621205458`, trace `ga-prometheus-stale-20260621205959`; Harbor outage: `docs/plan/2026-06-21-harbor-outage-failure-injection.md`, trace `ga-harbor-outage-20260621200008`, configured URL `http://harbor.harbor-system.svc.cluster.local/api/v2.0/ping`; OPS-012 image-registry build/list outage: `docs/plan/2026-06-21-image-registry-harbor-degraded-build-list.md`, trace `ga-image-harbor-degraded-20260621212113`, image `ci-ga-image-harbor-degraded-20260621211729`, digest `sha256:21345ac6ad43db05f489bfa2ee37122b0b79873daca6f04619506c8bcef9d319` |
| `PERF` | Partial k6 evidence — the bounded `20` VU read smoke baseline passed after correcting endpoint selection (`30` requests per endpoint, `0` failures, total p95 `2306.00ms`) while preserving the earlier `/outbox` timeout discovery. A reusable harness exists for `PERF-001` Project list and `PERF-002` 100 concurrent users: `backend/scripts/perf/core-read-100vu.js` plus optional `make -C backend perf-k6-core-read`. Live smoke with `2` VUs / `3s` passed (`63` requests including preflight, failure rate `0`, p95 `/healthz=0.67ms`, `/readyz=1.03ms`, `/api/v1/projects=3.86ms`). Earlier live `100` VU attempts exposed the single-principal limiter problem, least-privilege scope/PDP tuple mismatches, downstream org-project static-key propagation, and remote PDP enforce `429` on `authorization-policy-service`. Those blockers are now fixed for the Project list path: image `ci-ga-pdp-enforce-20260622094936` rolled to all 15 backend deployments, `100` temporary exact-scope principals were patched into gateway and org-project only for the drill, exact raw policy rows were inserted/deleted, positive Project preflight returned `200`, negative Groups preflight returned `403`, and the live `100` VU / `30s` k6 run exited `0` with `auth_key_count=100`, `3102` total requests, failure rate `0`, total p95 `3.135ms`, `/api/v1/projects` `1000` requests / `1000` 2xx / `0` 4xx / `0` 5xx / `0` 429 / p95 `3.668ms`, and authorization-policy enforce `429` count `0`. Cleanup restored gateway/org-project Secrets to `API_KEYS=1` / `API_KEY_PRINCIPALS=1`, removed all `100` temporary policy rows, and left no port-forward. `PERF-001`/`PERF-002` Project-list read evidence is now passed; `PERF-003..008` remain open: no queue stress, no large usage query, no metrics cardinality, no build concurrency, no WebRTC concurrency, and no K8s-control throughput proof. DR RTO/RPO numbers remain deferred. | `docs/plan/2026-06-22-perf-live-baseline.md`; `docs/plan/2026-06-22-perf-k6-core-read-100vu.md`; `docs/plan/2026-06-22-perf-k6-multi-principal.md`; `docs/plan/2026-06-22-perf-live-multi-principal-secret-drill.md`; `docs/plan/2026-06-22-perf-live-project-list-policy-drill.md`; `docs/plan/2026-06-22-pdp-enforce-service-internal-rate-limit.md`; `docs/acceptance/performance.md` still unverified for full GA |

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
| Service identity / JWT-JWKS lib / migration-runner maturity | Open |

## 4. Live cutover caveat

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
