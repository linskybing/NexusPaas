# Backend Gap & Code Problem Review

_Generated: 2026-06-17. Branch: `main`._

## 1. Summary

The backend remains a single Go module with 15 logical services selected by
`SERVICE_NAME`. Main contains the Production Beta readiness stack through PR #12:
15-service production-beta manifests, CI/security gates, scheduler-quota
owner-read boundary cleanup, operational readiness docs, a non-live
release-candidate rehearsal gate, and the current-backend capability inventory.
The gate ties quick checks, production-beta manifest render/deploy dry-run,
rollback command planning, re-deploy dry-run, Docker-backed E2E, runtime smoke,
security scans, Sonar, and an RC evidence report into one repeatable command.

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
- Fixed the GitHub Actions workflow-level environment configuration so the
  backend quality gate no longer references the unavailable `runner` context
  before jobs are scheduled.
- Adjusted the GitHub-hosted Sonar policy so repositories with no configured
  Sonar secrets skip Sonar explicitly instead of failing every workflow before
  a reachable Sonar endpoint/token exists.
- Added `backend/deploy/observability/production-beta` with baseline Grafana
  dashboard, authenticated Prometheus PodMonitor/PrometheusRule alerts, and a
  scheduled synthetic smoke CronJob for the 15-service topology.
- Added runtime HTTP duration histogram buckets and updated the observability
  overlay latency dashboard/alerts to use p95 `histogram_quantile` queries.
- Added focused identity package tests for API-token middleware revocation,
  internal API-token denylist enforcement, Dex/OIDC revocation registration,
  auth cleanup registration, login captcha/lockout edges, admin credential
  revocation, and helper/repository branches. `internal/services/identity` now
  meets the per-package 80% target locally.
- Hardened backend coverage for the remaining previously sub-80 packages without
  changing public API, database schema, deployment manifests, or runtime
  contracts. The formerly low packages now report: `cmd/microservice` 90.5%,
  `internal/platform` 80.2%, `internal/services/gpuusage` 83.4%,
  `internal/services/imageregistry` 81.2%,
  `internal/services/integrationproxy` 87.1%,
  `internal/services/k8scontrol` 91.5%,
  `internal/services/requestnotification` 80.6%,
  `internal/services/storage` 82.2%, and `internal/services/workload` 81.4%.
- Added a Docker Compose collaboration-smoke topology for Postgres, Redis,
  MinIO, and 15 independent backend service containers. The Docker gate now
  fails closed on missing scheduler owner-read `SERVICE_URLS`, bad service
  credentials, scheduler outage, and cross-service workflow mismatches instead
  of accepting only all-in-one routing smoke.

## 2. Current Verification

| Command | Result | Notes |
| --- | --- | --- |
| `go test ./internal/platform -run 'Deployment\|Operational\|Release\|Beta' -count=1` | Pass | Deployment hardening tests plus operational readiness and Beta RC docs/script guards |
| `bash backend/scripts/ci-security-gate.sh quick` | Pass | gofmt, go vet, `go test ./... -count=1`, `go build ./...` |
| `bash backend/scripts/ci-security-gate.sh docker` | Pass | Postgres/Redis/MinIO healthy; migrations apply/validate; integration total coverage 82.7%; focused E2E, full non-live E2E, all-in-one routing smoke, and 15-service collaboration smoke pass |
| `bash backend/scripts/ci-security-gate.sh security` | Pass | govulncheck: no vulnerabilities; OSV: no issues; Trivy image scan: 0 vulnerabilities |
| `bash backend/scripts/ci-security-gate.sh sonar` | Pass | Sonar Quality Gate OK |
| `bash backend/scripts/ci-security-gate.sh beta-rc` | Pass | Passed on commit `0aac41a`; quick checks, production-beta manifest render/deploy dry-run, rollback plan, re-deploy dry-run, Docker E2E, routing/process smoke, 15-service collaboration smoke, security scans, Sonar, and RC report all passed |
| `git diff --check` | Pass | Current branch diff has no whitespace errors |
| `test -f function.md` plus per-service/per-worker `rg` checks | Pass | `function.md` covers all 15 services, required background workers, and explicitly states reference parity is unverified |
| `go test ./internal/platform ./internal/services -count=1` | Pass | Relevant backend platform/service tests still pass for this docs-only branch |
| `go vet ./...` | Pass | Static Go vet check passes |
| `go build ./...` | Pass | Backend packages still build |
| `ruby -e 'require "yaml"; YAML.load_file(".github/workflows/backend-quality-gate.yml")'` | Pass | Workflow YAML parses locally |
| `bash -n backend/scripts/ci-security-gate.sh` | Pass | Quality gate script syntax remains valid |
| `! rg -n 'runner\\.temp' .github/workflows/backend-quality-gate.yml` | Pass | Workflow-level env no longer references the unavailable `runner` context |
| `CI_GATE_SONAR_REQUIRED=false SONAR_TOKEN= SONAR_HOST_URL= bash backend/scripts/ci-security-gate.sh sonar` | Pass | GitHub no-secrets policy skips Sonar explicitly |
| `CI_GATE_SONAR_REQUIRED=true SONAR_TOKEN= SONAR_HOST_URL= bash backend/scripts/ci-security-gate.sh sonar` | Expected fail | Explicitly required or partially configured Sonar still fails closed when credentials are missing |
| `kubectl kustomize backend` | Pass | Root Production Beta topology still renders without optional observability CRDs |
| `kubectl apply --dry-run=client --validate=false -f <rendered backend>` | Pass | Root Production Beta dry-run remains valid without Prometheus Operator CRDs |
| `kubectl kustomize backend/deploy/observability/production-beta` | Pass | Optional Grafana/Prometheus Operator/CronJob observability overlay renders |
| `ruby -e 'require "yaml"; require "json"; ...'` | Pass | Observability YAML parses and embedded Grafana dashboard JSON parses |
| `go test ./internal/platform -run 'Deployment\|Operational\|Observability\|Release\|Beta' -count=1` | Pass | Deployment policy tests verify observability overlay coverage and secret contracts |
| `go test ./... -count=1` | Pass | Full backend unit package suite passes after manifest/docs/test changes |
| `bash backend/scripts/ci-security-gate.sh security` | Pass | govulncheck no vulnerabilities; OSV no issues; backend image build passes; Trivy reports 0 vulnerabilities |
| `bash backend/scripts/ci-security-gate.sh sonar` | Pass | Local SonarScanner Quality Gate passed against `http://localhost:9000/dashboard?id=nexuspaas-backend` |
| `go test ./internal/platform -run 'Metrics\|Deployment\|Operational\|Observability\|Release\|Beta' -count=1` | Pass | Histogram metric contract plus deployment/observability policy tests pass |
| `kubectl kustomize backend/deploy/observability/production-beta` plus YAML/JSON parse | Pass | p95 dashboard/PrometheusRule overlay still renders and embedded dashboard JSON parses |
| `go test ./... -count=1` | Pass | Full backend unit package suite passes after histogram and p95 overlay changes |
| `go vet ./...` | Pass | Static Go vet check passes after histogram changes |
| `go build ./...` | Pass | Backend packages still build after histogram changes |
| `git diff --check` | Pass | Histogram branch diff has no whitespace errors |
| `bash backend/scripts/ci-security-gate.sh security` | Pass | govulncheck no vulnerabilities; OSV no issues; backend image build passes; Trivy reports 0 vulnerabilities |
| `bash backend/scripts/ci-security-gate.sh sonar` | Pass | Local SonarScanner Quality Gate passed against `http://localhost:9000/dashboard?id=nexuspaas-backend` |
| `go test ./internal/services/identity -coverprofile=/tmp/identity.cover -count=1` | Pass | `internal/services/identity` coverage is 80.3% |
| `go tool cover -func=/tmp/identity.cover` | Pass | Identity package total coverage reports 80.3% |
| `go test ./cmd/microservice ./internal/platform ./internal/services/gpuusage ./internal/services/imageregistry ./internal/services/integrationproxy ./internal/services/k8scontrol ./internal/services/requestnotification ./internal/services/storage ./internal/services/workload -coverprofile=/tmp/nexuspaas-low-coverage.out -count=1` | Pass | Formerly sub-80 packages are now `cmd/microservice` 90.5%, `internal/platform` 80.2%, `gpuusage` 83.4%, `imageregistry` 81.2%, `integrationproxy` 87.1%, `k8scontrol` 91.5%, `requestnotification` 80.6%, `storage` 82.2%, and `workload` 81.4% |
| `go test ./... -coverprofile=/tmp/nexuspaas-coverage.out -count=1` | Pass | Full backend coverage suite passes; all tested packages report at least 80.0% coverage |
| `go test -tags e2e ./internal/e2e -run TestComposeCollaborationSmoke -count=1 -v` | Pass | Build-tagged collaboration runner compiles; skips unless `COMPOSE_COLLABORATION_SMOKE=1` is set by the Docker gate |
| `/var/folders/xl/4ctb0b7j68z74pg5pc0h9wmh0000gn/T/nexuspaas-quality-gate/local-4661/beta-rc-report.md` | Pass | Run `local4661` produced the RC report after quick, production-beta dry-runs, Docker E2E, routing/process smoke, 15-service collaboration smoke, security scans, and Sonar all passed |
| `/var/folders/xl/4ctb0b7j68z74pg5pc0h9wmh0000gn/T/nexuspaas-quality-gate/local-4661/collaboration-smoke-summary.md` | Pass | Run `local4661` verified 15 isolated registries, identity remote auth ignoring forged `X-User-ID`, workload->scheduler admission, scheduler owner-read and bad credential failures, storage mount-plan, media upload, request-notification domain/audit events, and scheduler outage fail-closed behavior |

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
| GitHub workflow scheduling | `.github/workflows/backend-quality-gate.yml` used `${{ runner.temp }}` in workflow-level `env`, so GitHub failed the run before creating jobs or logs | CI temp paths now use literal `/tmp/...` values that are valid at workflow scope for the existing `ubuntu-latest` job |
| GitHub Sonar no-secrets handling | GitHub-hosted runs failed at Sonar because the repository has no `SONAR_TOKEN` or `SONAR_HOST_URL` secrets | The workflow now skips Sonar only when no Sonar secrets are configured; any partial or configured Sonar setup remains fail-closed |
| observability baseline provisioning | Dashboard, alert, authenticated scrape, and scheduled synthetic monitor resources were documented but not provisioned | `backend/deploy/observability/production-beta` now renders an optional Grafana/Prometheus Operator/CronJob overlay covering all 15 services without committing secrets |
| metrics granularity | Runtime metrics exposed request counts and duration sums but no histogram buckets | `/metrics` now emits Prometheus-compatible HTTP duration buckets/count/sum; dashboard and alert rules use p95 `histogram_quantile` |
| identity package coverage | `internal/services/identity` was below the per-package 80% target for a core IAM service | Focused identity tests now cover API-token current revocation, denylist behavior, OIDC/Dex revocation registration, auth cleanup, login edge cases, admin credential revocation, and helper/repository branches; local coverage is 80.3% |
| backend package coverage | Nine backend packages were below the per-package 80% target: `cmd/microservice`, `internal/platform`, `gpuusage`, `imageregistry`, `integrationproxy`, `k8scontrol`, `requestnotification`, `storage`, and `workload` | A focused coverage run now reports all nine at or above 80.0%: 90.5%, 80.2%, 83.4%, 81.2%, 87.1%, 91.5%, 80.6%, 82.2%, and 81.4%, respectively |
| 15-service collaboration evidence | The previous Docker smoke could prove `SERVICE_NAME=all` routing/process health but not independent service cooperation | `ci-security-gate.sh docker` now starts 15 backend containers with production-like service URLs, service keys, and auth settings, then verifies critical state-changing cross-service workflows and fail-closed negative paths |

## 4. Remaining Issues

| Priority | Area | Problem | Impact | Recommended Next Step |
| --- | --- | --- | --- | --- |
| High | reference parity | `references/CSCC_AI_Platform_Backend` is absent, so live reference diff cannot be performed | Reference-only behavior gaps remain unknown | Restore/provision the reference snapshot before parity-sensitive launch review |
| Medium | live observability activation | Baseline Grafana, PodMonitor, PrometheusRule, and synthetic CronJob manifests exist, but live cluster activation evidence has not been captured | Operators have provisionable resources but not proof that dashboards, alerts, scrape auth, and scheduled smoke are working in staging | Apply `backend/deploy/observability/production-beta` in staging with real secrets and capture dashboard/alert/CronJob evidence |
| Medium | live staging rehearsal | The non-live Beta RC gate exists, but a real staging deploy/readiness/smoke/rollback/re-deploy rehearsal has not been captured | External Beta traffic remains blocked until real cluster evidence exists or the risk is explicitly accepted | Run the live staging checklist in `backend/docs/beta-launch-readiness.md` with real staging secrets |
| Medium | GitHub Sonar provisioning | GitHub repository has no `SONAR_TOKEN` or `SONAR_HOST_URL` secrets, so hosted CI skips Sonar even though local Sonar evidence exists | Remote PR checks do not enforce Sonar Quality Gate until a GitHub-reachable Sonar endpoint/token is configured | Add SonarCloud or reachable SonarQube secrets and rerun the workflow with Sonar required |
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

Status: Non-live Beta RC gate now includes and passes the 15-service
collaboration smoke; external Beta traffic is still blocked pending live
staging evidence or explicit risk acceptance.

Rationale: the local Docker gate now proves both the older all-in-one
routing/process smoke and independent 15-service collaboration workflows across
real containers. The repository still has broader Production Beta launch
blockers: missing reference snapshot, missing GitHub Sonar provisioning, missing
live dashboard/alert provisioning, missing live staging rehearsal evidence, and
remaining shared physical Postgres transition debt.

## 7. GA Architecture Roadmap Update

_Updated: 2026-06-20. Branch: `feature/authz-consumer-contract`._

The 90-day GA architecture direction is now documented as a staged move from the
current modular monolith to 8 coarse deployable units:

- `platform-gateway`
- `iam-unit`
- `tenant-unit`
- `collaboration-unit`
- `platform-io-unit`
- `usage-observability`
- `compute-api`
- `compute-control-plane`

The accepted direction avoids a big-bang 15-service split. It prioritizes
Outbox/Inbox, read models, versioned internal contracts, deployable-unit
staging evidence, and compute saga hardening.

The Day 0-15 ADR baseline is now recorded under `docs/adr/` for the 8-unit
target, Outbox/Inbox and read-model migration, GA service identity direction,
and deployment evidence gates. These ADRs close the decision-record gap only;
implementation, contract fixtures, staging evidence, and security hardening
remain open until the follow-up slices produce test and runtime evidence.

The Day 16-35 contract/runtime work now includes canonical v1 core event
envelope fixtures under `backend/internal/contracts/fixtures/events/v1/`,
scheduler admission owner-read fixtures under
`backend/internal/contracts/fixtures/owner-read/v1/`, and scheduler/compute
internal command fixtures under
`backend/internal/contracts/fixtures/commands/v1/`, all guarded by focused
contract and drift tests. This gives reviewers versioned event-envelope evidence
for identity, tenant, workload, scheduler, and audit events; owner-read HTTP
contract artifacts for scheduler admission dependencies; and command artifacts
for scheduler-quota writes into org-project and workload owners.

The Outbox/Inbox runtime visibility foundation now exposes projection lag on
`/projections`, set-based Prometheus metrics for outbox depth, consumer lag,
projection applied totals, and projection dead-letter totals, and checkpoint
behavior that keeps lag visible when consumer intake fails. Focused platform
tests pass for projection lag/checkpoint behavior, dead-letter visibility,
runtime metric snapshot semantics, and escaped consumer metric labels:
`go -C backend test ./internal/platform -run 'Projection|Outbox|Metrics|Observability' -count=1`.

Projection replay and retry progress are now visible through additive
`/projections` fields, Prometheus metrics, and dead-letter metadata. A replay
request records `replay_count`, `replay_pending`, and `last_replay_at`, while
replayed poison events update the existing dead-letter record with
`attempt_count`, `retry_count`, and `last_failed_at`; metrics now expose
per-consumer replay and retry totals. Focused platform tests pass:
`go -C backend test ./internal/platform -run 'Projection|Outbox|Metrics|Observability' -count=1`.

Initial producer-specific event contract coverage now binds the five canonical
v1 event fixtures to real producer helpers or route producer paths:
`UserUpdated` from identity, `ProjectUpdated` from org-project, `JobSubmitted`
from workload, `QuotaReserved` from the scheduler-quota reservation route, and
`AuditEvent` from the platform audit hook. The platform reservation and audit
events keep existing consumer keys while adding fixture-compatible context such
as reservation details and normalized audit resource/outcome fields. Focused
producer tests pass:
`go -C backend test ./internal/contracts ./internal/platform ./internal/services/identity ./internal/services/orgproject ./internal/services/workload -run 'Event|Producer|Contract|Reservation|Audit' -count=1`.

Initial fixture-backed consumer contract coverage now binds current real
event-fed read-model consumers to canonical v1 fixtures: integration-proxy
consumes `UserUpdated` into its local admin users read model, and cluster-read
consumes `UserUpdated` plus `ProjectUpdated` into local identity/project read
models. The tests assert contract IDs from `user_id` and `project_id`, preserve
representative fixture payload fields, and keep isolated consumers from
populating owner-store resources. Focused consumer tests pass:
`go -C backend test ./internal/contracts ./internal/services/integrationproxy ./internal/services/clusterread -run 'Event|Consumer|Projection|Contract|ReadModel' -count=1`.

GPU usage consumer contract coverage now binds the existing usage-observability
read-model projection to the canonical v1 `JobSubmitted` fixture. The test
asserts the GPU job read model is keyed by fixture `job_id`, preserves
representative workload payload fields including requested resources, defaults
`JobSubmitted` status to `submitted`, and keeps the isolated consumer from
populating the workload owner store. Focused consumer tests pass:
`go -C backend test ./internal/contracts ./internal/services/gpuusage -run 'Event|Consumer|Projection|Contract|ReadModel' -count=1`.

Authorization-policy identity consumer contract coverage now binds the existing
authz read-model projection to the canonical v1 `UserUpdated` fixture. The test
asserts the policy identity users read model is keyed by fixture `user_id`,
preserves representative identity payload fields, and keeps the isolated
consumer from populating the identity owner store. Focused consumer tests pass:
`go -C backend test ./internal/contracts ./internal/services/authorizationpolicy -run 'Event|Consumer|Projection|Contract|ReadModel' -count=1`.

Latest local verification for this slice:

- `go -C backend test ./internal/services/authorizationpolicy -run 'Consumer|Projection|Contract|ReadModel' -count=1`:
  Pass.
- `go -C backend test ./internal/contracts ./internal/services/authorizationpolicy -run 'Event|Consumer|Projection|Contract|ReadModel' -count=1`:
  Pass.
- `git diff --check`: Pass.
- `go -C backend test ./... -count=1`: Pass.
- `go -C backend vet ./...`: Pass.
- `go -C backend build ./...`: Pass.
- `bash backend/scripts/ci-security-gate.sh quick`: Pass.
- `bash backend/scripts/ci-security-gate.sh security`: Pass; govulncheck and
  OSV found no issues, Docker image build succeeded, and Trivy reported 0
  vulnerabilities.
- `bash backend/scripts/ci-security-gate.sh sonar`: Pass; local SonarScanner
  Quality Gate passed for `nexuspaas-backend`.

No E2E, live Kubernetes, or staging evidence was required for this slice because
the change is limited to an in-process fixture-backed consumer contract test and
roadmap blocker tracking.

Broader command coverage, broader owner-read coverage, broader route-level
producer coverage, remaining consumer contract tests for other canonical
consumer paths, durable relay/publish-lag evidence, drift metrics/comparison,
and broader event-fed read-model adoption remain open.

### GA Architecture Remaining Issues

| Priority | Area | Problem | Impact | Recommended Next Step |
| --- | --- | --- | --- | --- |
| High | staging evidence | The 8 deployable units do not yet have captured live staging deploy, smoke, rollback, and redeploy evidence | The roadmap is documented but cannot be declared GA-ready | Build staging runtime config and capture evidence unit by unit |
| High | data ownership | Shared physical PostgreSQL and transition owner-read contracts remain; Outbox/Inbox runtime visibility exists but read-model adoption is not complete | Cross-unit boundaries are not yet GA-complete | Add durable relay/read-model slices and retire high-risk shared-store reads |
| High | contract testing | Core event envelope v1 fixtures, initial producer-specific event tests, fixture-backed consumer tests for integration-proxy, cluster-read, gpuusage, and authorization-policy, scheduler admission owner-read fixtures, scheduler/compute command fixtures, and runtime visibility tests exist, but broader owner-read/command coverage, broader route-level producer coverage, and remaining consumer event paths are not yet all versioned artifacts | Consumers can drift silently during decomposition | Add remaining owner-read/command fixtures, broader producer tests, and remaining consumer contract tests before changing internal contracts |
| High | Outbox/Inbox maturity | Runtime lag/dead-letter/projection visibility plus replay/retry progress exists, but drift metrics/comparison, durable relay/publish-lag evidence, and event-fed read-model adoption remain open | ADR 0002 cannot be declared complete and service cutovers still need stronger evidence | Add durable relay, drift comparison, and read-model adoption slices before retiring shared-store reads |
| Medium | service identity | Static `SERVICE_API_KEY` remains the Production Beta service-to-service auth fallback | GA security posture depends on rotatable workload identity or equivalent | Introduce Kubernetes workload identity or approved equivalent in staging |
| Medium | remote Sonar gate | GitHub-hosted Sonar still depends on repository secrets being configured | Remote PRs may not enforce Sonar even when local evidence exists | Provision reachable Sonar credentials and make the remote gate required |
| Medium | supply chain | SBOM generation and image signing are GA goals but not yet enforced | Release provenance remains incomplete | Add SBOM and Cosign signing after staging promotion is stable |
