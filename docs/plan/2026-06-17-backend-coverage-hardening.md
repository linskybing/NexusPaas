# Backend Coverage Hardening

## 1. Objective

Raise every currently sub-80% backend Go package to at least 80.0% package
coverage while preserving public APIs, database schema, deployment manifests,
and runtime contracts.

## 2. Background

The Production Beta readiness stack now passes the aggregate quality gate, but
several packages remain below the per-package 80% target called out in
`problem.md`. This slice hardens package-local confidence with focused behavior
tests and one small command composition-root refactor for testability.

## 3. Source References

- `problem.md`
- `docs/agents/workflow.md`
- `docs/agents/planning.md`
- `docs/agents/review-checklist.md`
- `backend/docs/beta-launch-readiness.md`
- `backend/internal/platform`
- `backend/internal/services`
- `backend/cmd/microservice`

## 4. Assumptions

- This branch does not need staging cluster access, production secrets, GitHub
  repository secrets, or a restored reference backend snapshot.
- Existing focused package tests are the right place to add behavior coverage.
- `cmd/microservice` can gain an unexported test seam as long as `main()` keeps
  the same production behavior.

## 5. Non-Goals

- Do not change public HTTP APIs, event schemas, database schema, migrations, or
  Kubernetes manifests.
- Do not solve live staging evidence, GitHub Sonar provisioning, reference
  parity, data-boundary read models, or catalog splitting.
- Do not delete production code or weaken gates to improve coverage.

## 6. Current Behavior

`go test ./... -coverprofile=/tmp/nexuspaas-coverage.out -count=1` passes, but
these packages are below 80% package coverage:

- `cmd/microservice`: 15.4%
- `internal/platform`: 79.5%
- `internal/services/gpuusage`: 79.7%
- `internal/services/imageregistry`: 76.3%
- `internal/services/integrationproxy`: 78.3%
- `internal/services/k8scontrol`: 76.3%
- `internal/services/requestnotification`: 78.9%
- `internal/services/storage`: 79.1%
- `internal/services/workload`: 77.7%

## 7. Target Behavior

Every package listed above reports at least 80.0% coverage through focused,
package-local tests. The command entrypoint remains behaviorally equivalent, but
its startup path is testable without invoking `os.Exit`, binding fixed ports, or
requiring real backing services.

## 8. Affected Domains

- Backend command startup and graceful shutdown coverage
- Platform helper and response coverage
- Service package behavior coverage for GPU usage, image registry, integration
  proxy, k8s control, request notification, storage, and workload
- Production Beta quality evidence in `problem.md`

## 9. Affected Files

Expected changes are limited to:

- `backend/cmd/microservice/main.go`
- `_test.go` files under the nine low-coverage packages
- `problem.md`
- this plan file

## 10. API / Contract Changes

No public API or wire-contract changes. The only production-code shape change is
an unexported command startup seam used by tests.

## 11. Database / Migration Changes

None.

## 12. Configuration Changes

None.

## 13. Observability Changes

No runtime observability behavior changes. Tests may assert existing health,
response, or shutdown behavior.

## 14. Security Considerations

- Preserve service-key and auth behavior in all tests.
- Do not commit secrets or staging credentials.
- Do not bypass production startup validation in the command path.

## 15. Implementation Steps

1. Create the implementation branch `test/backend-coverage-hardening`.
2. Refactor `cmd/microservice/main.go` so `main()` calls an unexported
   `runMicroservice(ctx, deps) int`; inject config loading, admin task, tracing,
   backing resources, app construction, service registration, server listen,
   signal/shutdown, and exit handling for tests.
3. Add command tests for invalid config, admin task failure/success, tracing or
   backing setup failure, startup validation failure, listen failure, successful
   signal-driven shutdown, and tracing shutdown logging.
4. Add focused tests in the eight low service/platform packages for existing
   helper, handler, projection, client, dispatch, and error behavior.
5. Keep non-command production code unchanged unless a tiny test seam is needed
   and reviewed as part of this plan.
6. Update `problem.md` with the new per-package coverage evidence and verification
   commands.

## 16. Verification Plan

- `cd backend && go test ./cmd/microservice ./internal/platform ./internal/services/gpuusage ./internal/services/imageregistry ./internal/services/integrationproxy ./internal/services/k8scontrol ./internal/services/requestnotification ./internal/services/storage ./internal/services/workload -coverprofile=/tmp/nexuspaas-low-coverage.out -count=1`
- `cd backend && go test ./... -coverprofile=/tmp/nexuspaas-coverage.out -count=1`
- `cd backend && go vet ./...`
- `cd backend && go build ./...`
- `bash backend/scripts/ci-security-gate.sh quick`
- `bash backend/scripts/ci-security-gate.sh sonar`
- `git diff --check`

## 17. Rollback Plan

Revert this branch's test additions, `cmd/microservice` startup seam, and
`problem.md` coverage update. Since no schema, config, or public contract changes
are introduced, rollback is a normal Git revert with no runtime migration.

## 18. Risks and Tradeoffs

- Package coverage can fluctuate when unrelated code is added later; tests should
  target meaningful behavior rather than brittle line coverage.
- `cmd/microservice` startup refactoring must preserve production exit behavior.
- Full `beta-rc` remains outside scope because this branch is a local coverage
  hardening slice.

## 19. Reviewer Checklist

- Scope is limited to coverage hardening and the approved command startup seam.
- Each formerly sub-80 package reports at least 80.0% coverage.
- No public API, DB, config, or manifest changes are present.
- Tests cover meaningful behavior, including error paths, rather than only
  calling helpers for coverage.
- `problem.md` reflects the new coverage evidence and remaining launch blockers.
- SOLID and 12-Factor compliance are preserved.
- Sonar Quality Gate passes or any inability to run it is explicitly documented.

## 20. Status

Status: Approved
