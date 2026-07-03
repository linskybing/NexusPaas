# Live Image-Build Pipeline — Dispatcher + Supply-Chain + Provenance-Gate Evidence

> **LOCAL EPHEMERAL — NOT EXTERNAL GA PROOF.** Per `docs/agents/workflow.md`,
> local evidence must not be described as external GA proof. This report proves
> the **product image-build workflow executes end to end against a real local
> Harbor**: build created through the public API, picked up by the lease-gated
> dispatcher maintenance task, executed by the `docker` build executor
> (docker build → Harbor push → syft SBOM → trivy scan → cosign sign), and the
> resulting digest admitted through the verified-provenance publish gate while
> a forged catalog row is rejected. It does **not** prove: in-cluster BuildKit
> Job execution (tracked follow-up, ADR 0008 / blocker-ledger §8), external
> registry promotion, or GPU build nodes.

- Run: local, 2026-07-02, ~21:06 UTC+8, branch `ac-completion-round`
- Test: `TestLiveImageBuildPipelineE2E`
  (`backend/internal/e2e/live_image_build_pipeline_e2e_test.go`, env-gated by
  `TEST_LIVE_IMAGE_BUILD_PIPELINE=1`)
- Harbor: `v2.15.1` via `backend/scripts/harbor-up.sh`, HTTP loopback
  `127.0.0.1:18080`, project `library`
- Backing services: `postgres:16-alpine`, `redis:7-alpine`, `minio` (RELEASE
  2025-04-08, host port 19000 to avoid the local SonarQube collision on 9000)
  via `backend/deploy/local/docker-compose.yml`
- Toolchain: docker 29.5.3 (Docker Desktop), syft, trivy, cosign v3.1.1

## 1. Pipeline stages (all live, one test run)

| stage | mechanism | result |
| --- | --- | --- |
| build create | `POST /api/v1/images/build/dockerfile` (project manager authz, image_projects read model) | `202`, record `status=queued` |
| dispatch | `image-build-dispatcher` maintenance task (`RunMaintenanceOnce`, lease-gated in runtime) | picks oldest queued build, `status=building`, `ImageBuildStarted` event |
| build | `docker build` (busybox:1.36 + evidence layer) in executor temp workdir | pass |
| push | `docker push` to `127.0.0.1:18080/library/nexuspaas-build-e2e` | pass, digest-pinned via `RepoDigests` |
| SBOM | `syft docker:<ref> -o spdx-json` | `sbom_status=succeeded`, sha256 SBOM digest recorded |
| scan | `trivy image --scanners vuln --severity HIGH,CRITICAL --exit-code 1` | 0 findings, `scan_status=passed` |
| sign | `cosign sign --use-signing-config=false --tlog-upload=false --key <ephemeral>` on digest-pinned ref | `signature_status=signed`, signature pushed to Harbor |
| registry proof | `docker pull <repo>@<digest>` | pass (manifest exists in Harbor) |
| provenance gate (reject) | `POST /api/v1/image-catalog/publish` with forged catalog row (`sha256:deadbeef`, fake SBOM/signature metadata) | `409` "no verified build provenance for catalog image digest" |
| provenance gate (admit) | same publish with the pipeline-built digest | `200`, allow-list rules issued |

Test verdict: `--- PASS: TestLiveImageBuildPipelineE2E (3.70s)`.

## 2. Harbor-side artifact evidence (registry API, `with_accessory=true`)

Final passing run (tag `1782997615`):

```
sha256:240608766a6207f6064566a07f6fd19d876bca703a002b9cc6771c2188d34ed9  IMAGE  [1782997615]
  accessory: signature.cosign sha256:925cdb38df263fcc059e05c4e8fa3ded13786d4968398a316158b976099ff82b
```

The cosign signature is stored in Harbor as an OCI accessory of the built
image — signing happened against the real registry, not a local stub.

## 3. Fail-closed behavior covered by unit suites (same commit)

- scan failure ⇒ build `failed`, `scan_status=failed`, `signature_status=skipped`
  (a vulnerable image is never signed) — `TestDispatchQueuedImageBuildsScanFailureFailsClosed`
- executor infrastructure error ⇒ build `failed` with error in logs —
  `TestDispatchQueuedImageBuildsExecutorError`
- dispatcher registration is config-gated on `IMAGE_BUILD_EXECUTOR` —
  `TestBuildDispatcherRegistrationIsConfigGated`
- context-key staging is fail-closed (foreign prefix / missing object /
  tampered bytes rejected) — `buildcontext_upload_test.go`
- from-storage builds require a read-capable storage grant (storage-service
  owner contract), otherwise `403` — `build_source_contracts_test.go`

## 4. Notes / deviations

- cosign v3 defaults to a Sigstore signing config that rejects
  `--tlog-upload=false`; the executor passes `--use-signing-config=false`
  (key-based signing against a private registry, no transparency log).
- Harbor `install.sh` hit the known Docker Desktop virtiofs mount race once;
  the script's built-in retry absorbed it (same behavior documented in the
  2026-07-01 Harbor smoke report).
- The dispatcher is driven synchronously in the test via `RunMaintenanceOnce`
  — the identical code path the runtime maintenance loop invokes on its
  interval; only the trigger differs (deterministic test vs timer).
