# Production Beta Launch Readiness

This document defines the evidence required before NexusPaas backend + ops can
serve early real users as a Production Beta. It complements the cross-service
E2E runbook and the operational readiness contract.

## Release Candidate Gate

Run the non-live release-candidate rehearsal from the repository root:

```sh
bash backend/scripts/ci-security-gate.sh beta-rc
```

The gate must pass these phases:

1. quick Go quality checks: gofmt, go vet, `go test ./... -count=1`, and
   `go build ./...`.
2. production-beta manifest rehearsal:
   - `kubectl kustomize backend`
   - 8 NexusPaas backend unit deployments present
   - all-in-one `platform` deployment absent
   - no `-dev-` secret references
   - `kubectl apply --dry-run=client --validate=false`
   - rollback command plan for every backend unit deployment
   - re-deploy client dry-run
   - 8 deployable-unit evidence report grouping the 15 logical deployments
3. Docker-backed migrations, integration coverage, focused E2E, and full
   non-live E2E.
4. non-live runtime smoke (routing/process smoke):
   - `SERVICE_NAME=all` starts on `TEST_RUNTIME_PORT` (default `18080`)
   - `/healthz`, `/readyz`, `/metrics`, `/openapi.json`, and
     `/service-registry` return 200
   - `/service-registry` lists all 15 logical services
   - one read-only endpoint per service returns 2xx or expected 4xx; no service
     returns 5xx
   - this proves route registration and process health only; it is not
     accepted as 8-unit collaboration evidence
5. 8-unit collaboration smoke:
   - starts Postgres, Redis, MinIO, and 8 backend unit containers from the same
     backend image
   - uses production-like `SERVICE_NAME`, `SERVICE_URLS`, `SERVICE_API_KEY`,
     static API-key principals, `REQUIRE_AUTH=true`, and
     `DEV_HEADER_AUTH=false`
   - verifies identity remote auth, workload-to-scheduler admission,
     scheduler owner-read contracts, storage mount-plan, media upload,
     request-notification events, bad service credentials, missing
     `SERVICE_URLS`, and scheduler outage fail-closed behavior
   - writes `collaboration-smoke.log`,
     `collaboration-smoke-summary.json`, and
     `collaboration-smoke-summary.md` under `${ARTIFACT_DIR}`
6. govulncheck, OSV source scan, backend image build, and Trivy image scan.
7. Local SonarScanner Quality Gate when `SONAR_TOKEN` and `SONAR_HOST_URL` are
   configured for `bash backend/scripts/ci-security-gate.sh sonar`, or
   fail-closed when local/CI policy requires Sonar.
8. generated RC evidence report at `${ARTIFACT_DIR}/beta-rc-report.md`.
   The report links `${ARTIFACT_DIR}/production-beta-deployable-units.md`,
   which maps the 8 physical backend units to the 15 logical services they
   host.

GitHub Actions does not run SonarScanner. SonarCloud/SonarQube must publish an
external required PR check and branch protection gate before merge. The backend
workflow does not require `SONAR_TOKEN` or `SONAR_HOST_URL`; the local `sonar`
script subcommand remains available for manual or release-candidate validation
with configured credentials.

The default artifact directory is under `/tmp/nexuspaas-quality-gate/<run-id>`.
Override it with `CI_GATE_ARTIFACT_DIR` when a CI job needs to upload artifacts.

## Live Staging Rehearsal

The `beta-rc` gate is non-live by default. Before external Production Beta
traffic is allowed, run a live staging rehearsal with real staging secrets and a
throwaway or dedicated staging namespace.

The operator-only harness is:

```sh
LIVE_STAGING_REHEARSAL=1 \
KUBE_CONTEXT=<real-staging-context> \
NAMESPACE=nexuspaas \
CANDIDATE_IMAGE=registry.example.com/nexuspaas/backend@sha256:<64-lowercase-hex-digest> \
PROMOTION_EVIDENCE=<promotion-evidence-url-or-path> \
REGISTRY_SCAN_STATUS=Success \
bash backend/scripts/production-beta-live-rehearsal.sh
```

It refuses Docker Desktop, kind, minikube, localhost, loopback, and other
local-style contexts; `kubectl config current-context` must exactly match
`KUBE_CONTEXT`. The candidate image must come from a non-local external
registry and must be pinned with `@sha256:<64 lowercase hex digest>`. If
operators want the harness to perform promotion, provide both `SOURCE_IMAGE`
and `PROMOTED_IMAGE_TAG`; the script then uses an already-installed `crane copy`. Otherwise
`PROMOTION_EVIDENCE` is mandatory. `REGISTRY_SCAN_STATUS` or
`REGISTRY_SCAN_EVIDENCE` is always mandatory.

The harness checks Kubernetes Secret names only, records previous app-container
images per backend unit, creates `ADMIN_TASK=apply-migrations` and
`ADMIN_TASK=validate-migrations` Jobs, applies the production-beta render, sets
the candidate image on each `deployment/<unit>`, waits for rollouts, port-forward
smokes `/healthz`, `/readyz`, and `/metrics` for all 8 backend units, verifies
gateway `/openapi.json` and `/service-registry` with all 15 logical services,
then rolls each unit back to its recorded previous image and redeploys the
candidate. The final artifact is
`${ARTIFACT_DIR}/production-beta-live-rehearsal-report.md`.

The live rehearsal must prove:

- Production-beta manifests apply successfully through the chosen GitOps or
  kubectl workflow.
- The same candidate image is promoted through a non-`localhost:5000` external
  registry with tag, digest, and scan evidence.
- Required Kubernetes Secrets or ExternalSecret-managed values exist before
  workloads start.
- Database migrations apply and validate against the staging database.
- All 8 backend units become ready.
- `/healthz`, `/readyz`, and `/metrics` pass for every backend unit.
- Gateway `/openapi.json` and `/service-registry` return 200.
- The service registry lists all 15 logical services.
- One read-only smoke endpoint per service returns 2xx or an expected 4xx; no
  service returns 5xx.
- Critical 8-unit collaboration journeys pass, including service-to-service
  auth failure cases and unavailable dependency fail-closed checks.
- Rollback command rehearsal is executed against staging workloads.
- Re-deploy returns the environment to the candidate version and repeats smoke.

## Remaining Issue Policy

Production Beta may proceed only when every `problem.md` issue is either:

- resolved with evidence,
- explicitly accepted as a non-blocking Beta risk with an owner and follow-up,
  or
- moved out of scope by product decision.

The following issue classes remain blocking for external Beta traffic unless
explicitly accepted:

- live staging deploy, smoke, rollback, and re-deploy not rehearsed,
- security scan or external SonarCloud/SonarQube Quality Gate failure,
- focused E2E skip/failure,
- integration coverage below 80%,
- missing production secrets or default/dev credentials in the deployment path,
- service registry missing any of the 15 logical services,
- any smoke endpoint returning 5xx,
- unresolved data-boundary regression that reintroduces cross-service writes or
  unvalidated shared-store reads.

## Rollback Standard

Rollback defaults to backend unit image/config rollback, not database restore:

```sh
kubectl -n nexuspaas rollout undo deployment/<unit>
```

For schema changes, use expand, dual-read/write, backfill, cutover, contract.
The staging rehearsal must capture which backend unit was rolled back, why
rollback was safe, whether any queues/events required replay, and how re-deploy
was validated.

## Beta RC Status

A candidate can be called a Production Beta RC only after:

- `bash backend/scripts/ci-security-gate.sh beta-rc` passes,
- the live staging rehearsal above passes, and
- `problem.md` contains no unaccepted launch blockers.
