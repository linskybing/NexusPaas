# 15-Service Collaboration Smoke Gate

## 1. Objective

Add a fail-closed Docker-based smoke gate that proves the 15 independently
deployed backend services can cooperate over their service-to-service contracts,
not merely return 200 from all-in-one process endpoints.

## 2. Background

The current Docker gate starts backing services and runs an all-in-one
`SERVICE_NAME=all` runtime smoke. That validates routing, process startup,
registry shape, and per-service read-only endpoints, but it does not prove that
the production-beta 15-service topology can exchange authenticated internal
HTTP calls, propagate identity, write state, emit events, and fail closed when a
dependency or service key is wrong.

## 3. Source References

- `backend/scripts/ci-security-gate.sh`
- `backend/deploy/local/docker-compose.yml`
- `backend/deploy/k3s/production-beta/runtime-config.yaml`
- `backend/internal/e2e/cross_service_e2e_test.go`
- `backend/internal/e2e/storage_mount_plan_e2e_test.go`
- `backend/internal/platform/config.go`
- `backend/internal/platform/endpoints.go`
- `backend/docs/beta-launch-readiness.md`
- `backend/docs/operational-readiness.md`

## 4. Assumptions

- Docker Compose is available wherever the existing Docker gate is expected to
  run.
- A single shared Postgres/Redis/MinIO set is acceptable for this local/CI gate;
  this stage validates process topology and service contracts, not physical
  database-per-service isolation.
- Production-like auth is required: static API keys plus service keys, with
  `DEV_HEADER_AUTH=false`.
- Services that require Kubernetes may be process-healthy while `/readyz` fails
  without a cluster; this gate must validate collaboration workflows directly
  instead of using `/readyz` as the only signal.

## 5. Non-Goals

- Do not replace the existing backing-only local compose file.
- Do not require live Kubernetes staging, production secrets, or external beta
  traffic.
- Do not add public HTTP APIs, DB migrations, event schemas, or production
  Kubernetes manifest behavior.
- Do not treat the all-in-one runtime smoke as 15-service collaboration
  evidence.

## 6. Current Behavior

`bash backend/scripts/ci-security-gate.sh docker` runs migrations, integration
coverage, focused/full e2e, and an all-in-one `SERVICE_NAME=all` runtime smoke.
That smoke checks core endpoints, service-registry count, and one read-only
endpoint per service, allowing a false sense of safety if isolated service
containers cannot cooperate.

## 7. Target Behavior

The Docker and beta-rc gates start a production-like 15-backend-container
compose topology and run critical cross-service journeys. Any failed service
hop, missing side effect, missing event, bad service-key handling, or dependency
outage behavior fails the gate. The gate writes a collaboration smoke log and a
JSON/Markdown evidence summary.

## 8. Affected Domains

- CI/security gate orchestration
- Local Docker Compose deployment topology
- Cross-service e2e smoke coverage
- Production Beta readiness documentation and problem tracking

## 9. Affected Files

Expected changes are limited to:

- `backend/deploy/local/collaboration-smoke.compose.yml`
- `backend/internal/e2e/compose_collaboration_smoke_test.go`
- `backend/scripts/ci-security-gate.sh`
- `backend/docs/beta-launch-readiness.md`
- `backend/docs/operational-readiness.md`
- `problem.md`
- this plan file

## 10. API / Contract Changes

No public API changes. The new smoke runner uses existing public routes,
existing service-key-gated internal routes, existing records, and existing Redis
event stream behavior.

## 11. Database / Migration Changes

None.

## 12. Configuration Changes

Add CI/local-only Docker Compose configuration that supplies the existing
runtime env vars: `SERVICE_NAME`, `SERVICE_URLS`, `SERVICE_API_KEY`, `API_KEYS`,
`API_KEY_PRINCIPALS`, backing-service URLs, object-store settings, and remote
authorization-policy settings.

## 13. Observability Changes

Add gate artifacts:

- `collaboration-smoke.log`
- `collaboration-smoke-summary.json`
- `collaboration-smoke-summary.md`
- `collaboration-compose.log`

## 14. Security Considerations

- Run services with `REQUIRE_AUTH=true` and `DEV_HEADER_AUTH=false`.
- Use explicit API key principals for both smoke-user and service identities.
- Verify wrong service keys fail closed and do not write records.
- Do not commit real secrets; all compose values are local smoke defaults.

## 15. Implementation Steps

1. Add the CI/local collaboration compose file with Postgres, Redis, MinIO, and
   all 15 backend service containers using the same backend image.
2. Add a Go e2e test that is skipped unless `COMPOSE_COLLABORATION_SMOKE=1` and
   reads service URLs/backing URLs from env.
3. Implement smoke journeys for remote identity auth, workload-to-scheduler
   submit, scheduler owner-read and bad service key handling, storage mount-plan
   contract, media upload/object-store round trip, request-notification events,
   and scheduler-down fail-closed behavior.
4. Update the gate script to build the backend image, run migrations and object
   bucket setup inside the compose network, start the 15-service topology, run
   the Go smoke runner, stop scheduler-quota for the outage negative case, run
   missing-service-urls startup validation, collect logs, and fail closed.
5. Wire the new step into `docker` and therefore `beta-rc`.
6. Update docs and `problem.md` so all-in-one smoke is documented as process
   smoke, while collaboration smoke is the 15-service evidence.

## 16. Verification Plan

- `cd backend && go test -tags e2e ./internal/e2e -run TestComposeCollaborationSmoke -count=1 -v`
- `bash backend/scripts/ci-security-gate.sh quick`
- `bash backend/scripts/ci-security-gate.sh docker`
- `bash backend/scripts/ci-security-gate.sh beta-rc`
- `git diff --check`

## 17. Rollback Plan

Revert the new compose file, e2e smoke runner, gate-script changes, and
documentation/problem evidence updates. Existing unit, e2e, Docker backing, and
all-in-one runtime smoke gates remain unchanged.

## 18. Risks and Tradeoffs

- The new gate is heavier because it starts 15 backend containers plus backing
  services.
- Shared backing services do not prove physical data isolation; they prove
  service process boundaries and HTTP/service-key contracts.
- Kubernetes-dependent services may not be ready without a cluster, so the gate
  uses `/healthz` for process startup and workflow assertions for collaboration.

## 19. Reviewer Checklist

- The Docker gate fails when collaboration smoke fails.
- The topology uses 15 backend containers, not `SERVICE_NAME=all`.
- Auth is production-like and does not rely on dev header auth.
- Positive workflows validate records/events/object-store side effects.
- Negative workflows validate no writes on downstream outage or bad service key.
- No public API, schema, migration, or production manifest change is introduced.
- Artifacts clearly explain failed workflow/service hop.

## 20. Status

Status: Approved
