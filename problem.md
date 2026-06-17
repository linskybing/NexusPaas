# Backend Gap & Code Problem Review

_Generated: 2026-06-17. Branch: `main`._

## 1. Summary

The backend remains a single Go module with 15 logical services selected by
`SERVICE_NAME`. Main contains the Production Beta readiness stack through PR #7,
and the current function-inventory branch adds the missing current-backend
capability inventory. The stack adds 15-service production-beta manifests,
CI/security gates, scheduler-quota owner-read boundary cleanup, operational
readiness docs, and a non-live release-candidate rehearsal gate. The gate ties
quick checks, production-beta manifest render/deploy dry-run, rollback command
planning, re-deploy dry-run, Docker-backed E2E, runtime smoke, security scans,
Sonar, and an RC evidence report into one repeatable command.

What changed in the current stacked work:

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
- Added `docs/architecture/observability-strategy.md` to define the Production
  Beta telemetry, SLO, alert, dashboard, runbook, rollback, and synthetic-smoke
  strategy.
- Added `backend/docs/operational-readiness.md` with the 15-service operations
  matrix and Beta SLO targets.
- Added `TestProductionOperationalReadinessDocsCoverAllServices` so every
  service deployment must have operational readiness documentation before the
  platform deployment policy tests pass.
- Linked the non-functional requirements to the new observability and
  operational readiness contract.
- Added `backend/scripts/ci-security-gate.sh beta-rc` to run the non-live
  release-candidate rehearsal.
- Added `backend/docs/beta-launch-readiness.md` to define RC evidence, live
  staging prerequisites, remaining issue policy, and rollback expectations.
- Added runtime smoke to the Docker-backed gate: core endpoints,
  `/service-registry`, and one read-only endpoint for each of the 15 services
  must avoid 5xx.
- Added `.gitignore` coverage for `backend/.e2e-gate/` local artifacts.
- Preserved the remaining useful test coverage from superseded PR #2 before
  closing it: Kubernetes native-object apply branches, RWX/Longhorn
  volume-share helpers, and scheduler-quota workload eviction client contracts.
- Added `function.md` as the current-backend capability inventory across the
  15-service catalog, route/event evidence, owned data, dependencies,
  background workers, and remaining parity risks.

## 2. Current Verification

| Command | Result | Notes |
| --- | --- | --- |
| `go test ./internal/platform -run 'Deployment\|Operational\|Release\|Beta' -count=1` | Pass | Deployment hardening tests plus operational readiness and Beta RC docs/script guards |
| `bash backend/scripts/ci-security-gate.sh quick` | Pass | gofmt, go vet, `go test ./... -count=1`, `go build ./...` |
| `bash backend/scripts/ci-security-gate.sh docker` | Pass | Postgres/Redis/MinIO healthy; migrations apply/validate; integration total coverage 80.5%; focused E2E, full non-live E2E, and runtime smoke pass |
| `bash backend/scripts/ci-security-gate.sh security` | Pass | govulncheck: no vulnerabilities; OSV: no issues; Trivy image scan: 0 vulnerabilities |
| `bash backend/scripts/ci-security-gate.sh sonar` | Pass | Sonar Quality Gate OK |
| `bash backend/scripts/ci-security-gate.sh beta-rc` | Pass | Passed on main commit `fa1d041`; quick checks, production-beta manifest render/deploy dry-run, rollback plan, re-deploy dry-run, Docker E2E, runtime smoke, security scans, Sonar, and RC report all passed; runtime smoke verified core endpoints 200, 15 registered services, and no per-service smoke 5xx |
| `git diff --check` | Pass | Function inventory documentation diff has no whitespace errors |
| `test -f function.md` plus per-service/per-worker `rg` checks | Pass | `function.md` covers all 15 services, required background workers, and explicitly states reference parity is unverified |
| `go test ./internal/platform ./internal/services -count=1` | Pass | Relevant backend platform/service tests still pass for this docs-only branch |
| `go vet ./...` | Pass | Static Go vet check passes |
| `go build ./...` | Pass | Backend packages still build |

## 3. Resolved In This Branch

| Area | Previous Problem | Current Evidence |
| --- | --- | --- |
| scheduler-quota data boundary | Cross-service org-project/workload reads were declared as generic store dependencies | `serviceStoreDependencies()` no longer contains scheduler-quota foreign resources; `serviceOwnerReadDependencies()` lists the five owner-read contracts |
| isolated admission runtime | Workload remote scheduler test returned 404 because scheduler had no org-project owner URL | `TestSubmitJobUsesRemoteSchedulerAdmissionWhenIsolated` passes with a real owner-read test server |
| startup isolation | Removing store dependencies could have made production startup silently accept missing owner URLs | `ValidateServiceIsolation` now validates owner-read dependencies; scheduler-quota fails without owner URLs/service key and passes with them |
| Sonar QG | New owner-read test complexity and test DB URI secret finding failed QG | Test helper refactor plus `TEST_POSTGRES_PASSWORD` URL construction; Sonar QG now passes |
| operations contract | 15-service Production Beta SLOs, dashboards, alerts, runbooks, rollback, and synthetic smoke were roadmap requirements but not documented as an enforceable contract | `backend/docs/operational-readiness.md` defines the service matrix; `TestProductionOperationalReadinessDocsCoverAllServices` verifies every deployment has coverage |
| observability strategy | Trace/log/metric correlation and alert/runbook strategy existed only as NFR bullets | `docs/architecture/observability-strategy.md` defines the shared OpenTelemetry/Prometheus/logging model and links to the backend operations contract |
| release rehearsal | Launch readiness required a repeatable RC rehearsal, but operators only had separate quick/docker/security/Sonar commands | `backend/scripts/ci-security-gate.sh beta-rc` runs quick checks, manifest render/deploy dry-run, rollback plan, re-deploy dry-run, Docker E2E, runtime smoke, security scans, Sonar, and RC report generation |
| capability inventory | `function.md` was missing, leaving launch reviewers without a single current-backend capability inventory | `function.md` now maps current capabilities to the 15-service catalog, route/job/event evidence, owned data, dependencies, background workers, and explicit reference-parity limits |

## 4. Remaining Issues

| Priority | Area | Problem | Impact | Recommended Next Step |
| --- | --- | --- | --- | --- |
| High | reference parity | `references/CSCC_AI_Platform_Backend` is absent, so live reference diff cannot be performed | Reference-only behavior gaps remain unknown | Restore/provision the reference snapshot before parity-sensitive launch review |
| Medium | coverage | Several packages remain below the per-package 80% target, although integration total meets the CI gate | Per-component risk remains masked by aggregate coverage | Raise low packages, especially `cmd/microservice`, `identity`, and schedulerquota follow-up paths |
| Medium | live observability provisioning | Operational readiness docs exist, but Grafana dashboards, PrometheusRule alerts, and scheduled synthetic monitors are not yet provisioned | Operators have a tested contract but not the final live monitoring resources | Implement dashboard/alert/synthetic monitor manifests or GitOps resources in the next observability hardening slice |
| Medium | live staging rehearsal | The non-live Beta RC gate exists, but a real staging deploy/readiness/smoke/rollback/re-deploy rehearsal has not been captured | External Beta traffic remains blocked until real cluster evidence exists or the risk is explicitly accepted | Run the live staging checklist in `backend/docs/beta-launch-readiness.md` with real staging secrets |
| Medium | data ownership | Scheduler-quota now uses owner-read contracts, but co-hosted mode can still read map-shaped records from the shared physical Postgres | Production Beta boundary is improved, not GA-complete | Continue toward typed DTO contracts or event-fed read models |
| Low | catalog size | `internal/services/catalog.go` remains over the 800-line soft cap | Maintainability issue | Split catalog specs by service in a later refactor |

## 5. Boundary Status

| Service | Status | Notes |
| --- | --- | --- |
| org-project-service | Improving | Remains authoritative owner for projects, project members, user groups, and user quotas; scheduler reads through owner contracts when isolated |
| workload-service | Improving | Remains authoritative owner for jobs; scheduler lists jobs through owner contract when isolated |
| scheduler-quota-service | Improving | No cross-service generic store dependencies remain; owner-read dependency config is fail-fast |
| all 15 deployment services | Improving | Each service now has a documented SLO/dashboard/alert/runbook/rollback/synthetic-smoke row guarded by platform tests |

## 6. Reviewer Status

Status: Non-live Production Beta RC gate passed on main; external Beta traffic
is still blocked pending live staging evidence or explicit risk acceptance.

Rationale: main's scheduler-quota boundary cleanup, observability/runbook
contract, production-beta manifest rehearsal, Docker-backed E2E, security scans,
Sonar Quality Gate, and `beta-rc` rehearsal all pass. The repository still has
broader Production Beta launch blockers: missing reference snapshot,
per-package coverage gaps, missing live dashboard/alert
provisioning, missing live staging rehearsal evidence, and remaining shared
physical Postgres transition debt.
