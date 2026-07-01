# Backend Gap & Code Problem Review

_Scheduled backend gap/code audit. Branch: `main` (post-#40 merge)._
_Re-verified 2026-06-30 (independent local pass) against
`references/CSCC_AI_Platform_Backend`: `go build`/`go vet`/`go test ./...` and
`go test -race ./...` all green locally; domain-level reference parity
re-confirmed. Local-only — no live external GA evidence added this pass._

This pass independently re-ran the local toolchain on the current working tree:
`go build ./...` clean, `go vet ./...` clean, full `go test ./...` **green across
all 23 tested packages** (`cmd/microservice`, `internal/platform`,
`internal/platform/cluster`, `internal/contracts`, `internal/services`, and every
per-service package; `internal/e2e` has no test files, 24 total), and
`go test -race ./...` also **green** (new datapoint — previously "Not Run").
Reference parity re-confirmed at domain level against the provided
`references/CSCC_AI_Platform_Backend`: every reference `internal/application`
domain and every `internal/cron` reconciler maps to ported code in the current
services. No new local code-quality regression surfaced.

## 1. Summary

**Status: code-complete locally, open on live external execution.** The backend
is a single Go module (`internal/services/*`) compiled to one binary
(`cmd/microservice`) and packaged as one base image
(`nexuspaas-backend:v0.1.0`); each of the 15 logical services owns its
`Dockerfile` (FROM base), `migrations/`, `k8s/`, and `README.md`, and is selected
at runtime by `ServiceName` + `Config.AllowsService` against an 8-unit deployable
topology (`deployableUnitServices`). Code-level isolation is guarded by tests
(`service_isolation_test.go`, `source_guard_test.go`,
`service_dependency_inventory_test.go`).

**No new feature gaps vs the reference were found this pass.** All reference
`internal/application` domains (announcement, audit, cluster, configfile,
container, executor, form, gpuusage, group, ide, image, job, k8s, oidcprovider,
plan, policy, preempt, project, queue, resourcehours, storage, upload, user, vpn)
map to ported code. The reference `course_monitoring_reconciler` remains the only
deliberately out-of-scope reconciler (ADR 0006).

**Highest-risk open problems are all live-execution P0s, not code defects:**
1. No real external image registry promotion/rollback (Harbor is an isolated
   foundation only).
2. No live 8-unit staging deploy/smoke/per-unit previous-image rollback (live
   rollback evidence covers the 15-deployment namespace only).
3. No live staging DB migration/rollback drill.
4. No live external staging Secret readiness/provenance.
5. Typed external API coverage and typed ownership remain **static fixtures
   only** ("Open") — not live-authorization proven; full image-build/SBOM/
   signing/scan GA workflow and live PERF/MON also remain open.
6. Archive/image-build source support is **API contract / queued metadata only**:
   Dockerfile/context/storage-path fields are not persisted, hashed, validated, or
   dispatched; tar.gz/zip upload and archive security controls are not implemented.

The structural risk worth flagging in code terms is the **shared-binary
distributed-monolith boundary** (one module/binary/image for 15 services); it is
intentional and test-guarded, but it is the largest standing deviation from a
true microservice topology.

## 2. Backend Feature Gap Table

| Priority | Reference Capability | Current Status | Expected Service | Evidence | Required Action |
| -------- | -------------------- | -------------- | ---------------- | -------- | --------------- |
| High | External image registry promotion/rollback (Harbor) | Partial | image-registry-service | Harbor is an isolated `harbor-system` foundation; never used for external promote/rollback (`problem.md` prior evidence; `imageregistry/handler.go`) | Execute a real external Harbor build → promote → previous-image rollback drill and record evidence |
| High | Full image build workflow (BuildKit/Tekton, SBOM, signing, scan enforcement, allow-list admission) | Partial | image-registry-service | Static typed fixtures only for `POST /api/v1/images/build[/from-storage|/dockerfile]`; `ImageBuildStarted` event + queued supply-chain metadata are shape-only; local IMG-019 guard now rejects catalog publish without digest, passing scan, and available/not-deleted metadata, and (opt-in `IMAGE_PUBLISH_REQUIRE_PROVENANCE=true`) also rejects publish without SBOM-digest + signature-ref presence — presence guard only, default-off; live SBOM/sign execution unproven; scheduler-quota submit admission additionally rejects (opt-in `K8S_IMAGE_CHECK_ENABLED=true`) workload images not on the project's published allow-list via an owner-read of `image_allow_lists` (in-code defense-in-depth; external policy-engine parity + live cluster enforcement still open) | Run live build execution + SBOM/sign/scan/allow-list enforcement and capture evidence |
| High | Archive/Dockerfile/from-storage build source handling | Missing | image-registry-service | 2026-06-30 archive/HPC audit: build handlers queue JSON metadata only; no multipart tar.gz/zip upload, archive extraction, Dockerfile/context persistence or hash, storage-path permission check, source digest, or object-context upload | Implement source upload/permission/provenance pipeline before advertising these as working build sources |
| High | 8-unit live staging deploy/smoke + per-unit previous-image rollback | Partial | all (platform-gateway + 14) | 8-unit kustomize/runtime-config/compose smoke + CI gates exist; live rollback evidence is 15-deployment namespace only (`deploy/k3s`, `kustomization.yaml`) | Perform live 8-unit staging deploy/smoke and per-unit image rollback |
| High | Live staging DB migration/rollback drill | Missing | each service (owns `migrations/`) | Per-service `migrations/` present and built into image; no live staging migrate/rollback evidence | Run live staging migration + rollback drill per unit |
| High | Live external staging Secret readiness/provenance | Partial | all | Static production-beta deploy-path proof of Secret names/keys + no placeholder refs; no live Secret objects/provenance | Provision + verify live external staging Secret objects |
| Medium | Typed external REST API contract coverage (live authorization) | Partial | all | Typed API coverage repeatedly marked "Open"; current proof is static fixtures/producer tests, not live admin authz | Add live typed-API authorization evidence per route family |
| Medium | Typed per-resource ownership coverage | Partial | all | Owner-read contracts wired (`RegisterOwnerReadDependencies`); scheduler-quota and workload owner-read dependencies now have local/static `(consumer_service, resource)` contract fixtures and a guard that every registered owner-read dependency has a matching fixture; typed ownership still "Open" for live drift jobs/replay cutover | Land remaining typed-ownership coverage and live drift jobs |
| Medium | HPC storage data-plane optimization | Partial | storage-service + k8s-control-service | Storage profiles, DataPlanePlan, CacheBinding, BenchmarkRecord, HPC StorageClass manifests, and FastTransfer records exist, but maturity is **HPC storage planning layer**: stage-in is `cp -a`, mover is single-worker `rsync -a --delete`, and checksum/resume/throughput/locality/cache/benchmark feedback is not proven | Build profile-based mover strategies and live fio/IOR/mdtest/checkpoint/cache evidence before claiming HPC optimization |
| Medium | Live performance + telemetry (PERF/MON) under real scheduler/K8s load | Partial | scheduler-quota, usage-observability | Local deterministic `PERF-003..008`, `MON-013..017`, queue-stress test only; bounded k6 read smoke; live retention/alerting open | Run live PERF/MON soak + alerting/retention evidence |
| Low | `course_monitoring_reconciler` | Out of scope | n/a | ADR 0006 — deliberately excluded | None (documented decision) |
| Low | All other reference `internal/application` domains + 15 `internal/cron` reconcilers | Implemented | mapped per service | File-by-file parity re-checked; all ported and registered via `RegisterMaintenanceTaskForService` | None |

## 3. Code Problem Table

| Priority | Area | Problem | Evidence | Impact | Recommended Fix |
| -------- | ---- | ------- | -------- | ------ | --------------- |
| High | image-registry build API | Build source fields are advertised by fixtures/AC but currently ignored by the queued metadata path. Idempotency fingerprinting omits source type/content/identity, so the same key can replay a different Dockerfile/context/storage source as the same build. | `docs/acceptance/archive-image-build-hpc-storage-audit.md`; `imageregistry/handler.go` create/fingerprint path | Retry semantics, reproducibility, and source provenance are unreliable once real source inputs are accepted | Include source type, Dockerfile hash, context/archive digest, storage path/object id, build args, revision, checksum, project, user, resources, and timeout in the fingerprint |
| High | archive build security | tar.gz/zip upload is listed as a GA target, but no archive parser/extractor or security controls exist in the image-build path. | `docs/acceptance/image-build.md`; archive audit | Enabling archive upload without controls would risk path traversal, symlink/hardlink abuse, zip bombs, and unbounded extraction | Add streaming extraction with canonical path checks, link policy, compressed/uncompressed limits, file-count limits, and checksum validation before enabling upload |
| Medium | storage/FastTransfer HPC data plane | Storage is HPC-aware in metadata/manifests, not optimized in actual data movement. | `docs/acceptance/archive-image-build-hpc-storage-audit.md`; DataPlane/FastTransfer evidence | Large-file, small-file, multi-node, object-store, cache, and checkpoint workloads may be slow or unverifiable despite storage-profile labels | Add profile-driven mover strategies, checksum/resume/progress/throughput metrics, cache warmup/eviction, async checkpoint flush, and benchmark feedback |
| Medium | repo / build (`cmd/microservice`, root `Dockerfile`) | Shared-binary distributed monolith: one Go module + one binary + one base image serve all 15 services; per-service Dockerfiles only `FROM ${BASE_IMAGE}` | `Dockerfile`, `audit-compliance-service/Dockerfile`, `internal/services/catalog.go` | A change in any service recompiles/reships every service image; blast radius is repo-wide; not true independent deployability | Accept as documented 8-unit topology OR split modules/images per unit; keep isolation guard tests as the compensating control |
| Info | tests | Test LOC (~68k) exceeds product LOC (~57k); heavy intentional fixture/table duplication (Sonar CPD-excluded by design) | `internal/**/*_test.go`, `sonar.cpd.exclusions` | None functionally; review-cost only | None required; keep CPD exclusions scoped to fixtures/manifests |

_No error-swallowing, panics, or hardcoded secrets surfaced in the spot-checked
largest diffs (schedulerquota `preemption.go`, imageregistry `handler.go`,
identity `oidc*.go`)._

Resolved local hygiene: `backend/coverage.out` is ignored/untracked and no
longer tracked as a blocker. Strict staging/production runtime validation now
rejects blank/`all` `SERVICE_NAME`, and the production-beta live rehearsal
render guard also rejects `SERVICE_NAME=all` or unit ConfigMap mismatches before
live mutation.

## 4. SOLID Review

| Principle | Status | Evidence | Gap | Required Action |
| --------- | ------ | -------- | --- | --------------- |
| Single Responsibility | Pass | Each `internal/services/<svc>` owns one domain; handlers/repositories/spec split per service | Some services large (workload, schedulerquota) but cohesive | Keep file sizes within the 200–400 line norm as they grow |
| Open/Closed | Pass | Functional-options `platform.Option`, `RegisterService`/`RegisterMaintenanceTaskForService` extension seams | — | None |
| Liskov Substitution | Pass | Repository/store interfaces injected; typed-repo seams landed | — | None |
| Interface Segregation | Pass | Small per-use interfaces (Go idiom: defined where consumed) | — | None |
| Dependency Inversion | Pass | Services depend on injected store/owner-read abstractions, not concrete drivers | — | None |

## 5. 12-Factor App Review

| Factor | Status | Evidence | Gap | Required Action |
| ------ | ------ | -------- | --- | --------------- |
| Codebase | Partial | One repo, one Go module for all 15 services | Single module ≠ one-codebase-per-deployable-service | Accept (8-unit topology) or split modules |
| Dependencies | Pass | `go.mod`/`go.sum` pinned; Alpine pinned `golang:1.25.11`/`alpine:3.22` | — | None |
| Config | Pass | `ConfigFromEnv`; `.env.example`; no secrets in source | — | None |
| Backing Services | Pass | Postgres/MinIO/Redis/K8s attached via config/env | — | None |
| Build, Release, Run | Partial | Single base image; trimpath static build; per-service migrations baked in | No live release/rollback drill per unit | Run live release/rollback (P0 gap §2) |
| Processes | Pass | Stateless process; state in backing services | — | None |
| Port Binding | Pass | `HTTP_ADDR=:8080`, `EXPOSE 8080`, self-contained server | — | None |
| Concurrency | Pass | Horizontal scale via K8s replicas; leader-elected cron | Live concurrency/throughput unproven | Capture live PERF (P0 gap §2) |
| Disposability | Pass | Signal-driven graceful shutdown (`main.go` SIGINT/SIGTERM + `Shutdown`) | — | None |
| Dev/Prod Parity | Partial | docker-compose dev + kustomize prod-beta; local Sonar/CI green | Live external staging parity unproven | Capture live staging evidence |
| Logs | Pass | `slog` structured logs to stdout | — | None |
| Admin Processes | Pass | `ADMIN_TASK` env runs one-off admin tasks via same binary | — | None |

## 6. Microservice Boundary Review

| Service | Owns Code | Owns API | Owns Data | Owns Config | Owns Tests | Owns Deploy | Boundary Status | Notes |
| ------- | --------- | -------- | --------- | ----------- | ---------- | ----------- | --------------- | ----- |
| identity-service | Yes | Yes | Yes | Yes | Yes | Yes | Good | OIDC/Dex, LDAP mirror, user cleanup |
| authorization-policy-service | Yes | Yes | Yes | Yes | Yes | Yes | Good | Casbin/policy data sync |
| org-project-service | Yes | Yes | Yes | Yes | Yes | Yes | Good | project/plan admin |
| workload-service | Yes | Yes | Yes | Yes | Yes | Yes | Good | job/container/executor |
| scheduler-quota-service | Yes | Yes | Yes | Yes | Yes | Yes | Good | queue/preemption/quota/priority class |
| k8s-control-service | Yes | Yes | Yes | Yes | Yes | Yes | Good | docker cleanup, cluster control |
| ide-service | Yes | Yes | Yes | Yes | Yes | Yes | Good | IDE workspace |
| storage-service | Yes | Yes | Yes | Yes | Yes | Yes | Good | longhorn RWX health |
| image-registry-service | Yes | Partial | Yes | Yes | Yes | Yes | Risky | API/data live workflow (build/SBOM/sign/scan, external Harbor) unproven |
| usage-observability-service | Yes | Yes | Yes | Yes | Yes | Yes | Good | gpu/resource hours, metrics |
| request-notification-service | Yes | Yes | Yes | Yes | Yes | Yes | Good | announcement/notification |
| integration-proxy-service | Yes | Yes | Yes | Yes | Yes | Yes | Good | VPN usage collector, form/project-access |
| media-upload-service | Yes | Yes | Yes | Yes | Yes | Yes | Good | blob/object store owner |
| audit-compliance-service | Yes | Yes | Yes | Yes | Yes | Yes | Good | audit bus |
| platform-gateway | Yes | Yes | Partial | Yes | Yes | Yes | Risky | Owns deploy + migrations dir; edge/gateway — confirm it owns no domain data |

**Cross-cutting boundary caveat:** all services share one Go module + one binary
+ one base image. Runtime isolation is by `ServiceName`/`AllowsService` and
guarded by `service_isolation_test.go` / `source_guard_test.go` /
`service_dependency_inventory_test.go`, but they are **not independently
buildable or deployable artifacts** — the boundary is logical, not physical.

## 7. Verification Commands

| Command | Purpose | Result | Notes |
| ------- | ------- | ------ | ----- |
| `go build ./...` | Compile all packages | Pass | Clean, exit 0 |
| `go vet ./...` | Static analysis | Pass | Clean, exit 0 |
| `go test ./...` | Full unit/integration suite | Pass | 23 tested packages green; `internal/e2e` has no test files (24 total) |
| File-by-file ref parity vs `references/CSCC_AI_Platform_Backend` | Feature gap check | Pass | All `application` domains + 15 cron reconcilers ported; `course_monitoring_reconciler` out of scope (ADR 0006) |
| SonarScanner Quality Gate (local) | Code quality gate | Pass | Local scanner reached Quality Gate `PASSED` and local SECURITY issue total `0`; remote SonarCloud SECURITY cleanup still depends on supported Cloud Analysis Scope configuration or CI-based analysis because `.sonarcloud.properties` automatic-analysis wildcards are not supported |
| `go test -race ./...` | Race detection | Pass | Clean, exit 0; 23 tested packages green (`internal/e2e` no test files) this pass |
| Live 8-unit staging deploy/rollback | Release readiness | Not Run | Open P0 — requires external cluster |

## 8. Recommended Execution Order

1. **(P0, live)** External Harbor registry: real build → promote → previous-image
   rollback drill — closes the top image-registry gap.
2. **(P0, live)** 8-unit staging deploy/smoke + per-unit previous-image rollback.
3. **(P0, live)** Live staging DB migration/rollback drill per unit.
4. **(P0, live)** Live external staging Secret objects + provenance verification.
5. **(P0, live)** Full image-build GA workflow: BuildKit/Tekton execution, SBOM,
   signing, scan enforcement, allow-list admission.
6. **(P1)** Implement image-build source handling and provenance: archive upload
   security, Dockerfile/context hashing, from-storage permission checks, source
   digest persistence, and source-aware idempotency.
7. **(P1)** Turn storage profiles into a real HPC data plane: profile-based mover,
   checksum/resume/progress/throughput, cache lifecycle, checkpoint flush, and
   benchmark feedback.
8. **(P1)** Promote typed external API coverage from static fixtures to live
   authorization evidence; land typed ownership + live drift jobs.
9. **(P1)** Live PERF/MON soak under real scheduler/K8s load; retention/alerting.
10. **(P2, test hygiene)** Run `go test -race ./...` before release.
11. **(P2, structural)** Decide whether to split the shared binary/module/image
   per deployable unit, or formally accept the 8-unit shared-binary topology as
   the GA boundary (document the trade-off in an ADR).

New feature expansion should not start before items 1–5 (live P0s) are evidenced.

## 9. Reviewer Status

All reference capabilities are present in code and the full local toolchain is
green, so there are **no blocking code/feature gaps**. The remaining work is
live-execution evidence (external Harbor, 8-unit staging deploy/rollback, live DB
migration drill, live Secret readiness, full image GA workflow) plus minor code
hygiene — tracked above and unchanged in character from prior passes.

Status: Approved
