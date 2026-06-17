# Backend Makefile CI Local Checks

## 1. Objective

Add a backend Makefile that provides local developer commands for build, lint,
test, coverage, and CI-equivalent quality gates.

## 2. Background

Backend quality checks already live in `backend/scripts/ci-security-gate.sh`.
Developers currently need to remember the underlying shell commands or call the
gate script directly. A Makefile gives the backend module a small, discoverable
interface for common local checks while preserving CI parity through the
existing gate script.

## 3. Source References

- `backend/scripts/ci-security-gate.sh`
- `.github/workflows/backend-quality-gate.yml`
- `backend/go.mod`
- `README.md`
- `docs/agents/planning.md`
- `docs/agents/review-checklist.md`

## 4. Assumptions

- The Makefile belongs in `backend/Makefile`.
- `make check`, `make ci-local`, and `make ci-quick` should run the quick,
  non-Docker gate.
- Full Docker, security, Sonar, and beta RC gates should remain opt-in targets.
- Existing dirty worktree changes are unrelated and must not be reverted.

## 5. Non-Goals

- Do not change Go source code.
- Do not change CI workflow behavior.
- Do not add or remove dependencies.
- Do not change application APIs, database schemas, deployment manifests, or
  runtime configuration.
- Do not run Docker-backed or network-backed gates unless explicitly needed.

## 6. Current Behavior

Backend contributors can run `go build ./...`, `go vet ./...`,
`go test ./...`, and `bash ./scripts/ci-security-gate.sh quick` manually from
`backend/`, but there is no backend Makefile listing these entrypoints.

## 7. Target Behavior

Running `make help` from `backend/` lists supported local checks. Running
`make check`, `make ci-local`, or `make ci-quick` delegates to
`./scripts/ci-security-gate.sh quick`, which runs gofmt check, vet, tests, and
build. Heavier gates are available as explicit Make targets.

## 8. Affected Domains

Developer tooling only.

## 9. Affected Files

- `docs/plan/2026-06-17-backend-makefile-ci-local-checks.md`
- `backend/Makefile`

## 10. API / Contract Changes

No application API or service contract changes. The only new public developer
interface is the Makefile target set.

## 11. Database / Migration Changes

None.

## 12. Configuration Changes

None. The Makefile uses the existing shell script and Go toolchain behavior.

## 13. Observability Changes

None.

## 14. Security Considerations

The Makefile does not introduce secrets or new network behavior. Security scans
remain delegated to `ci-security-gate.sh security` and are opt-in through
`make ci-security` or `make ci-all`.

## 15. Implementation Steps

1. Add `backend/Makefile` with `.PHONY` targets for help, build, fmt-check,
   vet, lint, test, coverage, check, ci-local, ci-quick, ci-docker,
   ci-security, ci-sonar, ci-all, beta-rc, and clean.
2. Keep local build/lint/test targets as direct Go commands from `backend/`.
3. Delegate CI parity targets to `bash ./scripts/ci-security-gate.sh <gate>`.
4. Keep `clean` limited to removing `coverage.out`.

## 16. Verification Plan

- `cd backend && make help`
- `cd backend && make lint`
- `cd backend && make build`
- `cd backend && make test`
- `cd backend && make check`
- `cd backend && make coverage`

Optional heavier checks when Docker, network, and secrets are available:

- `cd backend && make ci-docker`
- `cd backend && make ci-security`
- `cd backend && make ci-sonar`
- `cd backend && make ci-all`

## 17. Rollback Plan

Delete `backend/Makefile` and this plan document. No runtime state, database
state, or generated application artifacts need rollback.

## 18. Risks and Tradeoffs

- `make check` duplicates the quick gate entrypoint by design; the gate script
  stays authoritative for CI parity.
- Full CI remains slower and environment-dependent, so it is exposed through
  explicit opt-in targets instead of the default local check.
- `coverage` writes `coverage.out`, which already matches Sonar coverage
  configuration.

## 19. Reviewer Checklist

| Category | Result |
|---|---|
| Requirement Fit | Pass |
| Scope Control | Pass |
| Non-Goals | Pass |
| Architecture | Pass |
| Microservice Boundary | Pass |
| API Contract | Pass |
| Data Ownership | Pass |
| Config | Pass |
| Observability | Pass |
| Security | Pass |
| Testing | Pass |
| Rollback | Pass |
| Simplicity | Pass |
| Surgical Change | Pass |

Reviewer decision: this plan is small, verifiable, and aligned with the existing
backend quality gate script.

## 20. Status

Status: Approved
