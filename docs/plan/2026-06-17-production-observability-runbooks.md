# Production Observability Runbooks

## 1. Objective

Create the first Production Beta observability and operations slice for the
15-service backend topology: a documented observability strategy, per-service
operational readiness contract, and automated documentation tests that prevent
services from shipping without SLO, dashboard, alert, runbook, rollback, and
synthetic-smoke coverage.

## 2. Background

The Production Beta roadmap requires every service to have dashboards, logs,
metrics, trace propagation, SLOs, alerts, runbooks, rollback steps, and
synthetic smoke checks before the platform can be treated as launch-ready. The
current deployment manifests already expose OpenTelemetry configuration and
Prometheus-style runtime endpoints, but the repository does not yet contain the
operator-facing contract that maps those signals to service ownership, user
journeys, alerts, incident response, and rollback.

This PR is intentionally documentation-first: it establishes the operational
contract and adds tests that keep the contract aligned with the 15 deployment
manifests. Provisioning Grafana dashboards, alert-manager rules, or synthetic
monitoring jobs is deferred to a later PR so this change remains reviewable.

## 3. Source References

- `long-term.md`
- `problem.md`
- `docs/agents/workflow.md`
- `docs/agents/planning.md`
- `docs/agents/review-checklist.md`
- `backend/docs/non-functional-requirements.md`
- `backend/docs/migration-roadmap.md`
- `backend/deploy/observability/otel-collector.yaml`
- `backend/internal/platform/deployment_test.go`
- `backend/kustomization.yaml`
- 15 service-owned `backend/*-service/README.md` and
  `backend/*-service/k8s/deployment.yaml` files

## 4. Assumptions

- This roadmap slice is stacked on
  `feature/scheduler-quota-boundary-cleanup` until earlier PRs are merged or
  this branch is retargeted.
- Production Beta still targets 15 independently deployed backend services, not
  an all-in-one runtime.
- Existing `/healthz`, `/readyz`, `/metrics`, `/openapi.json`, and
  `/service-registry` endpoints remain the runtime smoke basis.
- OpenTelemetry, Prometheus, and structured logs are the expected signal
  standards; no new vendor-specific monitoring product is introduced in this
  PR.
- Static `SERVICE_API_KEY` remains a Production Beta service-to-service
  transition mechanism; GA hardening will replace it with mTLS or workload
  identity.

## 5. Non-Goals

- Do not provision Grafana dashboards, PrometheusRule resources, Alertmanager
  routes, or synthetic monitor CronJobs in this PR.
- Do not change runtime telemetry libraries, endpoint behavior, HTTP APIs, or
  event schemas.
- Do not add database migrations.
- Do not change deployment topology beyond documenting the operational contract.
- Do not resolve all remaining Production Beta launch blockers in `problem.md`.
- Do not merge or retarget earlier stacked PRs.

## 6. Current Behavior

`backend/docs/non-functional-requirements.md` states that services must emit
structured logs, metrics, and traces, and that dashboards should cover latency,
errors, queue depth, dispatch latency, K8s API errors, storage transfers, image
builds, and audit backlog. `backend/docs/migration-roadmap.md` also lists
alerts, runbooks, and rollback as convergence acceptance criteria.

However, the repository does not yet define:

- The Production Beta observability strategy.
- A per-service mapping from critical journeys to SLOs, dashboard panels,
  alerts, runbooks, rollback, and synthetic smoke checks.
- A test that fails when a newly deployed service lacks operational readiness
  documentation.

## 7. Target Behavior

The repository contains:

- `docs/architecture/observability-strategy.md` describing the Production Beta
  telemetry model, correlation requirements, SLO classes, alert policy,
  dashboard approach, runbook expectations, and rollout/rollback feedback loop.
- `backend/docs/operational-readiness.md` defining Beta SLO targets, required
  log/metric/trace fields, alert policy, standard runbook template, synthetic
  smoke checklist, and a 15-service operations matrix.
- A platform test that derives the 15 service names from service-owned
  deployment manifests and verifies each service has operational readiness
  coverage with SLO, dashboard, alert, runbook, rollback, and synthetic-smoke
  markers.
- `backend/docs/non-functional-requirements.md` links the NFRs to the concrete
  operational readiness contract.
- `problem.md` reflects remaining launch-readiness work accurately after this
  PR.

## 8. Affected Domains

- Production Beta operations
- Observability architecture
- Service ownership and incident response
- Deployment policy tests
- Launch readiness documentation

## 9. Affected Files

- `docs/architecture/observability-strategy.md`
- `backend/docs/operational-readiness.md`
- `backend/docs/non-functional-requirements.md`
- `backend/internal/platform/deployment_test.go`
- `problem.md`
- this plan file

## 10. API / Contract Changes

No HTTP, event, or database API changes.

This PR adds a documentation contract: every service deployment represented by
`backend/*/k8s/deployment.yaml` must have an operational readiness row in
`backend/docs/operational-readiness.md` with explicit SLO, dashboard, alert,
runbook, rollback, and synthetic-smoke coverage.

## 11. Database / Migration Changes

None.

## 12. Configuration Changes

No runtime configuration changes.

The new documentation will reference existing telemetry configuration such as
`OTEL_EXPORTER_OTLP_ENDPOINT`, `OTEL_SERVICE_NAME`, trace context propagation,
and Prometheus-compatible `/metrics` endpoints.

## 13. Observability Changes

This PR defines the Production Beta observability baseline:

- Required structured log fields: service, environment, version, request_id,
  trace_id, span_id, user_id where allowed, project_id where allowed, status,
  latency, and error category.
- Required metrics: request rate, latency, errors, saturation, dependency
  health, retry/circuit-breaker state, queue depth, event lag, and domain
  workflow counters where applicable.
- Required trace behavior: accept/generate edge correlation IDs, propagate W3C
  trace context across HTTP and event boundaries, and preserve user/project
  context where safe.
- Required operations artifacts: service dashboards, SLO/burn-rate alerts,
  standard runbooks, rollback steps, and synthetic smoke checks.

## 14. Security Considerations

- Observability docs must explicitly prohibit logging secrets, bearer tokens,
  API keys, raw session cookies, file payloads, or sensitive tenant data.
- Operational runbooks must not contain real credentials.
- Service-to-service security remains required by the NFRs; this PR documents
  alerting and troubleshooting expectations but does not weaken authentication.
- Incident response must preserve auditability for admin, tenant-changing,
  job/storage/image/security operations.

## 15. Implementation Steps

1. Add `docs/architecture/observability-strategy.md` with the Production Beta
   observability model and tradeoffs.
2. Add `backend/docs/operational-readiness.md` with Beta SLOs, telemetry
   requirements, alert policy, runbook template, rollback guidance, synthetic
   smoke checklist, and the 15-service operations matrix.
3. Update `backend/docs/non-functional-requirements.md` to point NFR-OBS and
   NFR-OPER readers to the concrete operational readiness contract.
4. Add platform tests in `backend/internal/platform/deployment_test.go` that:
   - verify both observability docs exist,
   - verify core SLO/correlation/security/rollback/smoke markers,
   - derive service names from deployment manifests,
   - require every service to have one operations-matrix row, and
   - require each row to include SLO, dashboard, alert, runbook, rollback, and
     synthetic-smoke markers.
5. Update `problem.md` to show that the documentation contract is closed while
   live dashboard/alert provisioning remains a later Production Beta task.
6. Run formatting, build, test, quality-gate, security, and Sonar verification.
7. Send the implementation to the Reviewer Agent and fix any blocking findings.

## 16. Verification Plan

Run at least:

```bash
cd backend && test -z "$(gofmt -l .)"
cd backend && go test ./internal/platform -run 'Deployment|Operational' -count=1
cd backend && go test ./... -count=1
cd backend && go vet ./...
cd backend && go build ./...
bash backend/scripts/ci-security-gate.sh quick
bash backend/scripts/ci-security-gate.sh docker
bash backend/scripts/ci-security-gate.sh security
bash backend/scripts/ci-security-gate.sh sonar
git diff --check
```

If a Docker-backed or external scanner step fails for environmental reasons, the
result must be captured in the PR risk section with the exact failure.

## 17. Rollback Plan

Revert this PR. Because it is docs and tests only, rollback removes the
observability/runbook contract and its test gate without requiring database,
runtime, or deployment cleanup.

## 18. Risks and Tradeoffs

- Documentation can drift unless the test is strict enough; the new test guards
  service coverage and required readiness markers but cannot prove the future
  live dashboards are deployed.
- The per-service matrix may be high level for Beta; that is acceptable for this
  PR because it creates a baseline operators can refine as real incidents and
  dashboards mature.
- Deferring PrometheusRule/Grafana provisioning means this PR improves launch
  readiness governance, not the runtime monitoring stack itself.
- Adding too much operational detail in one PR would make review harder; this
  plan keeps the change focused on the minimum enforceable contract.

## 19. Reviewer Checklist

- The plan directly supports the Production Beta observability/runbook roadmap
  gate.
- Scope is limited to docs and tests, with no unrelated runtime refactor.
- All 15 service deployments are covered by the readiness contract.
- SLOs, dashboards, alerts, runbooks, rollback, and synthetic smoke are
  explicitly represented.
- Security-sensitive logging guidance is present.
- Verification commands are concrete and sufficient for a docs/test PR stacked
  on the current quality gate.
- Remaining live provisioning gaps are documented instead of hidden.

## 20. Status

Status: Approved
