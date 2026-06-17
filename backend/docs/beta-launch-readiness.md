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
   - 15 NexusPaas service deployments present
   - all-in-one `platform` deployment absent
   - no `-dev-` secret references
   - `kubectl apply --dry-run=client --validate=false`
   - rollback command plan for every service deployment
   - re-deploy client dry-run
3. Docker-backed migrations, integration coverage, focused E2E, and full
   non-live E2E.
4. non-live runtime smoke (routing/process smoke):
   - `SERVICE_NAME=all` starts on `TEST_RUNTIME_PORT` (default `18080`)
   - `/healthz`, `/readyz`, `/metrics`, `/openapi.json`, and
     `/service-registry` return 200
   - `/service-registry` lists all 15 services
   - one read-only endpoint per service returns 2xx or expected 4xx; no service
     returns 5xx
   - this proves route registration and process health only; it is not
     accepted as 15-service collaboration evidence
5. 15-service collaboration smoke:
   - starts Postgres, Redis, MinIO, and 15 independent backend service
     containers from the same backend image
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
7. SonarScanner Quality Gate when configured or required.
8. generated RC evidence report at `${ARTIFACT_DIR}/beta-rc-report.md`.

The default artifact directory is under `/tmp/nexuspaas-quality-gate/<run-id>`.
Override it with `CI_GATE_ARTIFACT_DIR` when a CI job needs to upload artifacts.

## Live Staging Rehearsal

The `beta-rc` gate is non-live by default. Before external Production Beta
traffic is allowed, run a live staging rehearsal with real staging secrets and a
throwaway or dedicated staging namespace.

The live rehearsal must prove:

- Production-beta manifests apply successfully through the chosen GitOps or
  kubectl workflow.
- Required Kubernetes Secrets or ExternalSecret-managed values exist before
  workloads start.
- Database migrations apply and validate against the staging database.
- All 15 services become ready.
- `/healthz`, `/readyz`, and `/metrics` pass for every service.
- Gateway `/openapi.json` and `/service-registry` return 200.
- The service registry lists all 15 services.
- One read-only smoke endpoint per service returns 2xx or an expected 4xx; no
  service returns 5xx.
- Critical 15-service collaboration journeys pass, including service-to-service
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
- security scan or Sonar Quality Gate failure,
- focused E2E skip/failure,
- integration coverage below 80%,
- missing production secrets or default/dev credentials in the deployment path,
- service registry missing any of the 15 services,
- any smoke endpoint returning 5xx,
- unresolved data-boundary regression that reintroduces cross-service writes or
  unvalidated shared-store reads.

## Rollback Standard

Rollback defaults to service image/config rollback, not database restore:

```sh
kubectl -n nexuspaas rollout undo deployment/<service>
```

For schema changes, use expand, dual-read/write, backfill, cutover, contract.
The staging rehearsal must capture which service was rolled back, why rollback
was safe, whether any queues/events required replay, and how re-deploy was
validated.

## Beta RC Status

A candidate can be called a Production Beta RC only after:

- `bash backend/scripts/ci-security-gate.sh beta-rc` passes,
- the live staging rehearsal above passes, and
- `problem.md` contains no unaccepted launch blockers.
