# Harbor CI/Local Smoke Lane — Live Catalog-Sync + Health Evidence

> **CI/LOCAL EPHEMERAL — NOT EXTERNAL GA PROOF.** Per `docs/agents/workflow.md`,
> local/CI-ephemeral evidence must not be described as external GA proof. This
> report proves the Harbor **health-check and catalog-sync code paths execute
> correctly against a real Harbor instance**, repeatably, via one-command
> local scripts (`make harbor-up && make harbor-seed && make e2e-harbor`) and
> a matching CI job step. It is **additive** to the prior one-off 2026-06-21
> live RKE2 Harbor evidence recorded in `gap-tracker.md`/`gap-analysis.md`
> (trace `654e8a882af7e6a2099a5cce75a8377e`), not a replacement of it. It does
> **not** prove: external registry promotion/rollback, build execution
> (Tekton/BuildKit dispatch), pushing build *output* to Harbor, or SBOM/
> signing/scan enforcement — all of those remain Open per
> `docs/acceptance/blocker-ledger.md`. This specific pass is a **local dry
> run** (this environment's Docker Desktop, not yet the GitHub Actions
> runner); the CI job wiring (`.github/workflows/backend-quality-gate.yml`,
> `integration-e2e` job) is landed in the same change but its first real CI
> execution is a separate, subsequent event.

- Run: local dry run, 2026-07-01, ~20:44–20:46 UTC+8
- Harbor: `v2.15.1`, online installer, HTTP-only (no TLS, no `--with-trivy`),
  `http://127.0.0.1:18080`
- Seed: project `nexuspaas-e2e` (public), repository `smoke:v1`, source image
  `busybox:1.36`
- Backing services: local `postgres:16-alpine` + `redis:7-alpine` (via
  `backend/deploy/local/docker-compose.yml`) + `minio` (ad hoc, CI-style raw
  `docker run`, ports 19000/19001 to avoid a local port collision — see §3)

## 1. Harbor install (harbor-up.sh)

| step | result |
| --- | --- |
| download + extract v2.15.1 online installer | pass |
| harbor.yml patch (hostname, http port, admin password, data_volume, https block removal) | pass |
| `./install.sh` (no `--with-trivy`) | pass on retry (see §3) |
| all 9 containers started (log, db, redis, registry, registryctl, portal, core, jobservice, nginx) | pass |
| `GET /api/v2.0/ping` | `Pong` |

## 2. Seed (harbor-seed.sh)

| step | result |
| --- | --- |
| `POST /api/v2.0/projects` (nexuspaas-e2e, public) | `201 Created` |
| `docker push 127.0.0.1:18080/nexuspaas-e2e/smoke:v1` | pass — digest `sha256:b7f3d86d6e84fc17718c48bcde1450807faa2d56704205c697b4bd5df7b9e29f` |

## 3. Live e2e (`make e2e-harbor`)

```
=== RUN   TestLiveHarborCatalogSyncE2E
--- PASS: TestLiveHarborCatalogSyncE2E (0.29s)
=== RUN   TestLiveHarborImageBuildE2E
--- PASS: TestLiveHarborImageBuildE2E (0.02s)
PASS
```

`TestLiveHarborCatalogSyncE2E` retrieved a real catalog record with
`digest = sha256:b7f3d86d6e84fc17718c48bcde1450807faa2d56704205c697b4bd5df7b9e29f`,
`status = available`, `deleted = false`, matching the pushed artifact exactly.
`scan_status` was correctly absent (this Harbor was installed without
`--with-trivy` by design — see `blocker-ledger.md` Feature Gap Table).
`TestLiveHarborImageBuildE2E` confirmed `/api/v1/harbor-status` reports
`adapter: harbor, status: ok` against the real instance.

Independently confirmed via direct `curl` against
`GET /api/v2.0/projects/nexuspaas-e2e/artifacts?with_tag=true&with_scan_overview=true&pageSize=100&page=1`
(bypassing the app entirely): Harbor returned the real artifact list with the
same digest, `repository_name: nexuspaas-e2e/smoke`, `tags[0].name: v1` — this
is also the live confirmation of the `pageSize` fix below.

## 4. Real bug found and fixed during this pass

`harbor_catalog_sync.go`'s `harborArtifactQuery` sent the query parameter
`page_size` (snake_case); Harbor's real API v2.0 parameter is `pageSize`
(camelCase), verified directly against `goharbor/harbor`'s `swagger.yaml`.
Real Harbor silently ignored the unrecognized `page_size` and fell back to
its own default page size, which combined with the `len(artifacts) <
harborArtifactPageSize` continuation check would have silently truncated
catalog sync to page 1 for any project with more artifacts than that
default — a class of bug only a real Harbor instance can surface, since
existing fixture-based unit tests control the full response body directly.
Fixed to `pageSize`; confirmed live via the direct `curl` call in §3 above.

## 5. Environment notes (local Docker Desktop only, not expected to recur on native Linux CI runners)

Two Docker-Desktop-specific issues surfaced and were fixed in the scripts;
neither is expected to reproduce on GitHub Actions' native Linux Docker
daemon (no VM/virtiofs layer):

- Harbor's `prepare` step transiently failed to bind-mount a just-generated
  secret file (`root.crt`) under Docker Desktop's virtiofs file sharing
  (`mountpoint ... is outside of rootfs`); succeeded immediately on retry
  with the same generated files. `harbor-up.sh` now retries `install.sh`
  once on failure.
- Harbor's default `data_volume: /data` host path was outside Docker
  Desktop's allow-listed shared paths; `harbor-up.sh` now redirects it to the
  script's own cache directory.
- Harbor rejects `127.0.0.1`/`localhost` as the configured `hostname`
  (`prepare` hard error: "127.0.0.1 can not be the hostname") — this is an
  enforced validation, not just documentation guidance as it first appeared.
  `harbor-up.sh` uses a placeholder `harbor.nexuspaas.local` value instead
  (connectivity always goes through `127.0.0.1:<port>` by IP directly, so the
  hostname string never needs to resolve).
- `alpine:3.22`'s official multi-platform Docker Hub image carries
  provenance/attestation content whose companion push failed client-side
  with a garbled, unrelated repository name on this Docker version, even
  though the registry itself accepted the actual image blob correctly (seen
  in registry logs). Switched the seed image to `busybox:1.36`, confirmed to
  push cleanly.

## 6. What remains Open

External registry promotion/rollback, build execution (Tekton/BuildKit
dispatch, Harbor push of build *output*), SBOM/signing/scan enforcement, and
any external (non-CI-ephemeral) staging Harbor instance all remain Open —
unchanged by this pass. See `docs/acceptance/blocker-ledger.md` §2 and
`docs/acceptance/ga-acceptance-trace-matrix.md`.
