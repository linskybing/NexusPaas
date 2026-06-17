# Backend Gap & Code Problem Review

_Generated: 2026-06-17. Branch: `feature/scheduler-quota-boundary-cleanup`._

## 1. Summary

The backend remains a single Go module with 15 logical services selected by
`SERVICE_NAME`. This branch advances Production Beta PR 4 by retiring
scheduler-quota's remaining cross-service `storeDependencies` on org-project and
workload data. Those relationships are now explicit owner-read dependencies,
validated at startup and resolved through service-key-gated owner read contracts
when scheduler-quota runs isolated.

What changed in this branch:

- Added `RegisterOwnerReadDependencies` and extended `ValidateServiceIsolation`
  so owner-read contracts stay fail-fast without being classified as shared-store
  debt.
- Moved scheduler-quota's org-project/workload reads from
  `serviceStoreDependencies()` to `serviceOwnerReadDependencies()`.
- Routed scheduler-quota submit admission, project queue lookup, live quota
  derivation, plan-window reaping, and resource-quota reconciliation through the
  owner-aware admission reader.
- Migrated the isolated workload→scheduler admission test to stand up
  org-project/workload owner read endpoints instead of seeding foreign records
  into the scheduler store.
- Fixed the stacked CI gate's test database URL construction so Sonar no longer
  treats the local test password URI as a new secret violation.

## 2. Current Verification

| Command | Result | Notes |
| --- | --- | --- |
| `bash backend/scripts/ci-security-gate.sh quick` | Pass | gofmt, go vet, `go test ./... -count=1`, `go build ./...` |
| `bash backend/scripts/ci-security-gate.sh docker` | Pass | Postgres/Redis/MinIO healthy; migrations apply/validate; integration total coverage 80.0%; focused E2E and full non-live E2E pass |
| `bash backend/scripts/ci-security-gate.sh security` | Pass | govulncheck: no vulnerabilities; OSV: no issues; Trivy image scan: 0 vulnerabilities |
| `bash backend/scripts/ci-security-gate.sh sonar` | Pass | Sonar Quality Gate OK; new coverage 88.4%; new duplicated lines 0.86281%; new violations 0 |

## 3. Resolved In This Branch

| Area | Previous Problem | Current Evidence |
| --- | --- | --- |
| scheduler-quota data boundary | Cross-service org-project/workload reads were declared as generic store dependencies | `serviceStoreDependencies()` no longer contains scheduler-quota foreign resources; `serviceOwnerReadDependencies()` lists the five owner-read contracts |
| isolated admission runtime | Workload remote scheduler test returned 404 because scheduler had no org-project owner URL | `TestSubmitJobUsesRemoteSchedulerAdmissionWhenIsolated` passes with a real owner-read test server |
| startup isolation | Removing store dependencies could have made production startup silently accept missing owner URLs | `ValidateServiceIsolation` now validates owner-read dependencies; scheduler-quota fails without owner URLs/service key and passes with them |
| Sonar QG | New owner-read test complexity and test DB URI secret finding failed QG | Test helper refactor plus `TEST_POSTGRES_PASSWORD` URL construction; Sonar QG now passes |

## 4. Remaining Issues

| Priority | Area | Problem | Impact | Recommended Next Step |
| --- | --- | --- | --- | --- |
| High | reference parity | `references/CSCC_AI_Platform_Backend` is absent, so live reference diff cannot be performed | Reference-only behavior gaps remain unknown | Restore/provision the reference snapshot before parity-sensitive launch review |
| High | capability inventory | Planned `function.md` is still missing | No single capability parity source of truth | Execute or formally descope `docs/plan/2026-06-16-reference-backend-function-inventory.md` |
| Medium | repo hygiene | `backend/.e2e-gate/` artifacts remain untracked and not gitignored | Gate logs/service-registry output can pollute commits | Add `backend/.e2e-gate/` to `.gitignore` in a small hygiene PR |
| Medium | coverage | Several packages remain below the per-package 80% target, although integration total meets the CI gate | Per-component risk remains masked by aggregate coverage | Raise low packages, especially `cmd/microservice`, `identity`, and schedulerquota follow-up paths |
| Medium | data ownership | Scheduler-quota now uses owner-read contracts, but co-hosted mode can still read map-shaped records from the shared physical Postgres | Production Beta boundary is improved, not GA-complete | Continue toward typed DTO contracts or event-fed read models |
| Low | catalog size | `internal/services/catalog.go` remains over the 800-line soft cap | Maintainability issue | Split catalog specs by service in a later refactor |

## 5. Boundary Status

| Service | Status | Notes |
| --- | --- | --- |
| org-project-service | Improving | Remains authoritative owner for projects, project members, user groups, and user quotas; scheduler reads through owner contracts when isolated |
| workload-service | Improving | Remains authoritative owner for jobs; scheduler lists jobs through owner contract when isolated |
| scheduler-quota-service | Improving | No cross-service generic store dependencies remain; owner-read dependency config is fail-fast |

## 6. Reviewer Status

Status: Changes Required

Rationale: this branch's scheduler-quota boundary cleanup is passing local
quality, Docker-backed E2E, security scans, and Sonar Quality Gate. The repository
still has broader Production Beta blockers outside this PR: missing reference
snapshot, missing `function.md`, unignored `.e2e-gate` artifacts, per-package
coverage gaps, and remaining shared physical Postgres transition debt.
