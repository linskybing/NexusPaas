# README Backend Makefile CI Local Checks

## 1. Objective

Update the root README so backend contributors can discover the new Makefile
targets for local build, lint, test, and CI quick checks.

## 2. Background

`backend/Makefile` now wraps common Go commands and the existing backend quality
gate script. The root README still documents direct `go build`, `go test`,
`go vet`, and `gofmt` commands.

## 3. Source References

- `README.md`
- `backend/Makefile`
- `backend/scripts/ci-security-gate.sh`
- `docs/agents/planning.md`
- `docs/agents/review-checklist.md`

## 4. Assumptions

- "readme" refers to the root `README.md`.
- The backend architecture README should stay focused on service decomposition.
- Docker/security/Sonar gates should be documented as optional heavier checks.

## 5. Non-Goals

- Do not change application code.
- Do not change Makefile targets.
- Do not change CI workflow behavior.
- Do not remove direct Go command compatibility.

## 6. Current Behavior

The root README tells developers to run `go build ./...`, `go test ./...`, and
manual pre-PR commands from `backend/`.

## 7. Target Behavior

The root README uses `make build`, `make test`, and `make check` as the primary
local backend workflow and lists optional heavier CI targets.

## 8. Affected Domains

Developer documentation only.

## 9. Affected Files

- `README.md`
- `docs/plan/2026-06-17-readme-backend-makefile-ci-local-checks.md`

## 10. API / Contract Changes

None.

## 11. Database / Migration Changes

None.

## 12. Configuration Changes

None.

## 13. Observability Changes

None.

## 14. Security Considerations

Security scan behavior is unchanged; the README only points readers to the
existing optional `make ci-security` target.

## 15. Implementation Steps

1. Update Quick start build/test commands to use `make build` and `make test`.
2. Add a concise local quality checks section listing `make lint`,
   `make check`, and optional heavier gates.
3. Update Contributing to use `make check` as the default pre-PR check.

## 16. Verification Plan

- `git diff --check README.md docs/plan/2026-06-17-readme-backend-makefile-ci-local-checks.md`
- `cd backend && make help`

## 17. Rollback Plan

Revert the README documentation edits and delete this plan document.

## 18. Risks and Tradeoffs

- The README becomes Makefile-first, but direct Go commands remain available
  through the Makefile and can still be run manually.
- Heavier Docker/security/Sonar targets are documented as optional because they
  require more local environment setup.

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

Reviewer decision: this is a documentation-only update aligned with the new
backend Makefile interface.

## 20. Status

Status: Approved
