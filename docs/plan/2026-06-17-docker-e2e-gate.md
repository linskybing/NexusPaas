# Docker E2E Gate

## 1. Objective

Make the backend's Postgres/Redis/MinIO-backed integration, E2E, coverage, and runtime smoke checks reproducible from a clean local checkout with one repository-owned command.

## 2. Background

The Production Beta roadmap requires a repeatable verification gate before moving to 15 independently deployed services. Manual Docker testing has already proven the backend can run with real backing services, but the process is not encoded as a stable command and still depends on developer memory.

## 3. Source References

- `long-term.md`
- `problem.md`
- `backend/docs/e2e-testing.md`
- `backend/deploy/local/docker-compose.yml`
- `sonar-project.properties`

## 4. Assumptions

- Sonar continues to use `localhost:9000`, so the gate must not bind MinIO to port `9000`.
- Docker is available locally.
- The gate may start and remove containers that it creates, but must not remove compose volumes or unrelated containers.
- `long-term.md` remains untracked and is not part of this PR.

## 5. Non-Goals

- Do not add GitHub Actions in this slice.
- Do not solve Sonar authentication or Quality Gate readback.
- Do not change service APIs, schemas, or deployment topology.
- Do not implement the 15-service kustomize rollout.

## 6. Current Behavior

Developers must manually start backing services, choose alternate MinIO ports when Sonar owns `9000`, run migrations, run integration tests, run E2E tests, and start an all-in-one runtime for smoke checks.

## 7. Target Behavior

`backend/scripts/docker-e2e-gate.sh` starts isolated Docker backing services on non-conflicting default ports, runs migrations, provisions the media bucket, runs integration coverage, runs required E2E gates, smoke-tests the runtime, and cleans up its own resources.

## 8. Affected Domains

- Backend verification
- Local developer workflow
- Production Beta readiness tracking

## 9. Affected Files

- `backend/scripts/docker-e2e-gate.sh`
- `backend/docs/e2e-testing.md`
- `README.md`
- `problem.md`
- `backend/identity-service/migrations/0002_identity_owned_records.sql`

## 10. API / Contract Changes

No service API or wire contract changes.

## 11. Database / Migration Changes

The gate executes `apply-migrations` and `validate-migrations` against an
isolated test database. During implementation it exposed that
`login_failures` lacked the owned-table `created_at` column/backfill used by
the identity store, so this slice includes the small migration correction needed
to keep a clean database bootstrappable.

## 12. Configuration Changes

Add script-level environment overrides for ports, coverage threshold, E2E run ID, and optional full-E2E execution. Defaults must avoid Sonar's `9000`.

## 13. Observability Changes

The gate prints clear step names and runtime smoke results, including status codes for service endpoints.

## 14. Security Considerations

The gate uses local-only static test credentials and scoped container names. It must not print real secrets and must not require production credentials.

## 15. Implementation Steps

1. Add `backend/scripts/docker-e2e-gate.sh`.
2. Run isolated Postgres, Redis, and MinIO containers using default ports `15433`, `16379`, `19000`, and `19001`.
3. Apply and validate migrations, then provision `media-e2e`.
4. Run `go test -tags integration ./... -coverprofile=coverage.out -count=1` and enforce aggregate coverage `>= 80.0%`.
5. Run the focused cross-service E2E gate and reject skipped required tests.
6. Run full non-live E2E unless explicitly disabled.
7. Start `SERVICE_NAME=all` on `127.0.0.1:18080`, assert platform endpoints are `200`, assert registry count is `15`, and assert 15 service smoke endpoints have no `5xx`.
8. Fix the identity `login_failures` owned-table migration gap caught by the
   gate.
9. Update docs and `problem.md`.

## 16. Verification Plan

- `bash -n backend/scripts/docker-e2e-gate.sh`
- `gofmt -l backend`
- `cd backend && go build ./...`
- `cd backend && go vet ./...`
- `cd backend && go test ./... -count=1`
- `backend/scripts/docker-e2e-gate.sh`

Run security and Sonar tools if locally available; otherwise record exact not-run reasons.

## 17. Rollback Plan

Delete the script and revert documentation/problem updates. The script removes its own temporary containers and does not create schema changes outside the isolated test database.

## 18. Risks and Tradeoffs

- Full E2E is slower than the focused gate, but catches non-live regressions before Production Beta.
- Shell scripting is intentionally used to avoid adding a new dependency.
- Local Docker availability remains required until the CI slice lands.

## 19. Reviewer Checklist

- Scope is limited to verification reproducibility.
- No service behavior changes.
- Script cleans up only its own containers/processes.
- Coverage gate is real and uses the generated profile.
- Runtime smoke validates all 15 services without depending on Sonar port `9000`.

## 20. Status

Status: Approved and verified locally
