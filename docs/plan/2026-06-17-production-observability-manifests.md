# Production Observability Manifests

## 1. Objective

Add the smallest Production Beta observability provisioning slice: reviewable
Kubernetes/GitOps manifests for dashboards, alert rules, metrics scraping, and a
scheduled synthetic monitor covering the 15 independent backend services.

## 2. Background

`problem.md` still lists live observability provisioning as a Production Beta
gap. The repo already has the operations contract in
`backend/docs/operational-readiness.md`, the shared strategy in
`docs/architecture/observability-strategy.md`, and service deployments that
expose `/healthz`, `/readyz`, and authenticated operational endpoints such as
`/metrics`, `/openapi.json`, and `/service-registry`.

The existing root `backend/kustomization.yaml` must stay dry-run friendly for
clusters that do not have Prometheus Operator CRDs installed. Therefore this
slice will add an optional observability overlay under
`backend/deploy/observability/production-beta/` instead of inserting
`PrometheusRule` into the root production-beta topology.

## 3. Source References

- `long-term.md`: Production Beta requires synthetic monitoring, Prometheus,
  Grafana, OpenTelemetry, SLOs, alerts, rollback, and PR-sized changes.
- `problem.md`: remaining gap for dashboard, alert, and scheduled synthetic
  monitor provisioning.
- `backend/docs/operational-readiness.md`: 15-service SLO, alert, runbook, and
  smoke contract.
- `docs/architecture/observability-strategy.md`: shared backend observability
  model.
- `backend/internal/platform/metrics.go`: current Prometheus-compatible metric
  names: `nexuspaas_http_requests_total` and
  `nexuspaas_http_request_duration_seconds_sum`.
- `backend/internal/platform/endpoints.go`: operational endpoints and auth
  behavior.
- `backend/internal/platform/deployment_test.go`: deployment and operational
  readiness policy tests.

## 4. Assumptions

- Production Beta uses the existing 15-service topology in
  `backend/kustomization.yaml`.
- Prometheus Operator and Grafana sidecar conventions are acceptable optional
  targets for this repo because they align with the existing CNCF strategy.
- The live cluster will create the referenced synthetic smoke secret outside
  this repo; no secret values will be committed.
- Root production-beta render and dry-run must remain valid without Prometheus
  Operator CRDs.

## 5. Non-Goals

- Do not modify runtime request metrics or add a Prometheus client dependency.
- Do not deploy Prometheus, Grafana, Loki, Tempo, Jaeger, or a managed
  observability stack.
- Do not claim live staging evidence or live dashboard installation evidence.
- Do not add service mesh, mTLS, workload identity, or new auth behavior.
- Do not change service APIs, databases, or application logic.

## 6. Current Behavior

- The repo documents the required operational contract but does not include
  concrete dashboard, alert, scrape, or scheduled synthetic monitor manifests.
- `/metrics` emits the current application counters/sums, and production
  operational endpoints require admin-capable auth.
- `backend/scripts/ci-security-gate.sh beta-rc` renders and dry-runs only the
  root production-beta topology.

## 7. Target Behavior

- A dedicated observability overlay can be rendered with `kubectl kustomize`.
- The overlay contains:
  - a Grafana dashboard ConfigMap covering all 15 services and Beta SLOs;
  - Prometheus Operator resources for authenticated scraping and alerting;
  - a CronJob-based synthetic smoke monitor that checks core operational
    endpoints and one read-only endpoint per service without committing secrets;
  - a concise README documenting prerequisites, apply order, expected secret,
    and rollback.
- Deployment policy tests verify that all 15 services are covered and that the
  remaining launch gap is live activation/evidence, not missing manifests.

## 8. Affected Domains

- Platform operations.
- Kubernetes deployment/GitOps resources.
- Observability and launch readiness documentation.
- Deployment policy tests.

## 9. Affected Files

- `backend/deploy/observability/production-beta/kustomization.yaml`
- `backend/deploy/observability/production-beta/grafana-dashboard.yaml`
- `backend/deploy/observability/production-beta/prometheus-rules.yaml`
- `backend/deploy/observability/production-beta/synthetic-smoke.yaml`
- `backend/deploy/observability/production-beta/README.md`
- `backend/internal/platform/deployment_test.go`
- `backend/docs/operational-readiness.md`
- `docs/architecture/observability-strategy.md`
- `problem.md`

## 10. API / Contract Changes

No application API changes. The Kubernetes/GitOps contract adds an optional
observability overlay and two externally managed secrets:

- `nexuspaas-prometheus-scrape-secret` with `bearer-token`, used by the
  Prometheus Operator `authorization.credentials` contract for authenticated
  `/metrics` scraping. The token must be a valid bearer credential accepted by
  all 15 services for the admin/metrics operational route.
- `nexuspaas-synthetic-smoke-secret` with `api-key` and `service-key`, used by
  the CronJob for authenticated operational and read-only smoke requests.

## 11. Database / Migration Changes

None.

## 12. Configuration Changes

Add optional Kubernetes observability resources. No runtime environment
variables are added to application deployments. The synthetic monitor reads
credentials from Kubernetes Secret references only.

## 13. Observability Changes

Add baseline dashboard panels, alert rules, authenticated metrics scrape intent,
and scheduled synthetic smoke checks for:

- `/healthz`, `/readyz`, and `/metrics` on each service;
- `/openapi.json` and `/service-registry` on `platform-gateway`;
- one read-only smoke endpoint per service with no 5xx allowed;
- availability, latency, 5xx/error-rate, deployment availability, and synthetic
  monitor failures.

## 14. Security Considerations

- No secret values are committed.
- The CronJob uses `automountServiceAccountToken: false`, non-root container
  security context, read-only root filesystem, dropped capabilities, and
  resource requests/limits.
- Prometheus scrape credentials are supplied through
  `authorization.credentials` Secret references and sent as a bearer token to
  `/metrics`.
- The synthetic credentials are supplied through Secret references and sent only
  as platform auth headers to in-cluster service URLs.
- Metrics and operational endpoint access remains authenticated in production.

## 15. Implementation Steps

1. Create the optional observability overlay directory and kustomization.
2. Add Grafana dashboard ConfigMap with panels referencing current
   `nexuspaas_*` metrics, deployment availability, and synthetic smoke status.
3. Add Prometheus Operator PodMonitor/PrometheusRule manifests covering all 15
   services while keeping the root kustomization unchanged; the PodMonitor must
   use the external scrape bearer-token Secret and must not rely on unauthenticated
   `/metrics`.
4. Add CronJob synthetic monitor ConfigMap/script and scheduled job with
   external Secret references.
5. Add deployment policy tests that render/read the overlay and assert service,
   SLO, endpoint, scrape auth secret, synthetic secret, and security coverage.
6. Update operational readiness, observability strategy, and `problem.md` to
   distinguish provisioned manifests from remaining live activation/evidence.

## 16. Verification Plan

- `kubectl kustomize backend`
- `kubectl apply --dry-run=client --validate=false -f <(kubectl kustomize backend)`
- `kubectl kustomize backend/deploy/observability/production-beta`
- `go test ./internal/platform -run 'Deployment|Operational|Observability|Release|Beta' -count=1`
- `go vet ./...`
- `go build ./...`
- `git diff --check`

Prometheus Operator CRDs are optional for local verification, so the
observability overlay will be rendered and tested textually instead of
client-dry-run applied as part of the root production-beta gate.

## 17. Rollback Plan

Remove the observability overlay resources from GitOps or delete the overlay
directory from the branch. Runtime service behavior is unaffected because this
slice adds only optional observability manifests and docs.

## 18. Risks and Tradeoffs

- PrometheusRule and ServiceMonitor/PodMonitor resources require Prometheus
  Operator CRDs in the live cluster.
- The PodMonitor scrape auth assumes the live runtime accepts a scoped bearer
  token for the admin metrics route. If operators use static `X-API-Key` only,
  they must mint a JWT/API-token compatible bearer credential or use an external
  scrape proxy before applying the overlay.
- Dashboard queries are baseline expressions against current metric names and
  may need tuning once a real Prometheus scrape configuration adds labels.
- The CronJob depends on a correctly scoped API key/service key secret created
  by operators.
- This closes manifest provisioning, not live staging installation evidence.

## 19. Reviewer Checklist

- Scope is limited to observability manifests, tests, and docs.
- Root production-beta kustomization remains dry-run friendly without
  Prometheus Operator CRDs.
- All 15 services appear in dashboard/alert/synthetic monitor coverage.
- No secrets or default credentials are committed.
- PodMonitor uses an external bearer-token Secret for `/metrics` scraping.
- CronJob follows Kubernetes security-context expectations.
- `problem.md` accurately preserves remaining live launch blockers.

## 20. Status

Status: Approved
