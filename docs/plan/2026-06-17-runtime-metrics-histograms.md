# Runtime Metrics Histograms

## 1. Objective

Close the Production Beta metrics granularity gap by adding Prometheus-compatible
HTTP duration histogram buckets to the existing runtime metrics sink and updating
the observability overlay to use p95 latency expressions instead of mean latency
sentinels.

## 2. Background

`problem.md` identifies that current runtime metrics expose request counts and
duration sums, but no histogram buckets. The Production Beta SLOs in
`backend/docs/operational-readiness.md` and
`docs/architecture/observability-strategy.md` require p95 latency visibility for
general reads, writes, and job-submit paths. The previous observability overlay
therefore used mean latency sentinels as a temporary fallback.

## 3. Source References

- `problem.md`: remaining `metrics granularity` issue.
- `backend/internal/platform/metrics.go`: current in-process metrics sink.
- `backend/internal/platform/metrics_rollback_admin_test.go`: current metrics
  output coverage.
- `backend/deploy/observability/production-beta/grafana-dashboard.yaml`:
  dashboard currently describes and queries mean latency.
- `backend/deploy/observability/production-beta/prometheus-rules.yaml`: latency
  alerts currently use mean read/write latency sentinels.
- `backend/docs/operational-readiness.md`: p95 Beta SLO targets.
- `docs/architecture/observability-strategy.md`: dashboard and alert strategy.

## 4. Assumptions

- The existing custom metrics sink remains acceptable for this slice; adopting
  the Prometheus Go client can be evaluated later as a larger instrumentation
  decision.
- Labels stay compatible with existing metrics: `route`, `method`, and `status`.
- Prometheus scrapes add the `service` label through the existing PodMonitor
  relabeling, so runtime metrics do not need to know Kubernetes service names.
- Histogram buckets can be fixed centrally because all HTTP routes share the
  same Production Beta latency SLO families.
- The initial bucket boundaries, in seconds, are:
  `0.005`, `0.01`, `0.025`, `0.05`, `0.1`, `0.25`, `0.3`, `0.5`, `1`, `2`,
  `5`, `10`, and `+Inf`. These cover the internal owner-read 300ms, general
  read 500ms, general write 1s, and job-submit 2s Beta SLO thresholds.

## 5. Non-Goals

- Do not replace the metrics sink with `client_golang`.
- Do not add route taxonomy, endpoint-specific SLO classification, or new
  labels beyond the current route/method/status labels plus histogram `le`.
- Do not alter request handling, auth, tracing, logging, database behavior, or
  service APIs.
- Do not activate a live Prometheus/Grafana stack or capture staging evidence.
- Do not change non-HTTP domain counters.

## 6. Current Behavior

- `/metrics` emits `nexuspaas_http_requests_total`.
- `/metrics` emits `nexuspaas_http_request_duration_seconds_sum`.
- `/metrics` does not emit `_bucket` or `_count` series, so Prometheus
  `histogram_quantile` p95 expressions cannot work.
- Dashboard and alert manifests use mean latency expressions.

## 7. Target Behavior

- `/metrics` emits `nexuspaas_http_request_duration_seconds_bucket` with
  cumulative `le` buckets for every observed route/method/status label set.
- `/metrics` emits `nexuspaas_http_request_duration_seconds_count`.
- `/metrics` uses the Prometheus histogram text exposition contract:
  `# TYPE nexuspaas_http_request_duration_seconds histogram`, cumulative
  `_bucket{route,method,status,le}`, `_sum{route,method,status}`, and
  `_count{route,method,status}` series.
- Bucket output is deterministic and ordered by label set, then by bucket
  boundary, with `+Inf` last.
- For each route/method/status label set, `_count` equals the `le="+Inf"` bucket.
- Existing request count, duration sum, rollback error-rate, and degraded
  counters remain backward-compatible.
- Dashboard and PrometheusRule latency panels/alerts use
  `histogram_quantile(0.95, sum by (..., le) (rate(..._bucket[5m])))`.
- Docs and `problem.md` reflect that runtime histogram buckets are now
  implemented, while live activation evidence remains separate.

## 8. Affected Domains

- Runtime observability.
- Production Beta dashboard and alert manifests.
- Deployment/observability policy tests.
- Launch-readiness documentation and problem tracking.

## 9. Affected Files

- `backend/internal/platform/metrics.go`
- `backend/internal/platform/metrics_rollback_admin_test.go`
- `backend/internal/platform/deployment_test.go`
- `backend/deploy/observability/production-beta/grafana-dashboard.yaml`
- `backend/deploy/observability/production-beta/prometheus-rules.yaml`
- `backend/docs/operational-readiness.md`
- `docs/architecture/observability-strategy.md`
- `problem.md`
- `docs/plan/2026-06-17-runtime-metrics-histograms.md`

## 10. API / Contract Changes

No application HTTP API changes. The `/metrics` contract expands by adding
Prometheus histogram series:

- `nexuspaas_http_request_duration_seconds_bucket{route,method,status,le}`
- `nexuspaas_http_request_duration_seconds_count{route,method,status}`

Existing metric names stay present for compatibility. The duration metric TYPE
header changes from an ad hoc `_sum` counter header to the standard base
histogram header:

```text
# TYPE nexuspaas_http_request_duration_seconds histogram
```

## 11. Database / Migration Changes

None.

## 12. Configuration Changes

None. Bucket boundaries are static code-level instrumentation policy for this
slice.

## 13. Observability Changes

- Add HTTP duration histogram buckets at `0.005`, `0.01`, `0.025`, `0.05`,
  `0.1`, `0.25`, `0.3`, `0.5`, `1`, `2`, `5`, `10`, and `+Inf` seconds.
- Update dashboard latency panel to show p95 latency from histogram buckets.
- Update PrometheusRule latency alerts from mean read/write latency to p95
  read/write latency.
- Preserve existing request count, duration sum, and degraded counter output.

## 14. Security Considerations

Metrics remain served through the existing authenticated operational `/metrics`
route in production. No secret values, credentials, or sensitive request data are
added to labels or metric values.

## 15. Implementation Steps

1. Extend `Metrics` to maintain cumulative duration bucket counts and count
   totals per route/method/status label set.
2. Keep the existing duration sum output and add `_bucket` plus `_count` series
   under the standard base histogram `# TYPE` header.
3. Add focused unit tests that parse/assert the metric contract:
   deterministic bucket order, cumulative bucket counts, `+Inf` bucket,
   `_count == +Inf bucket`, sum/count compatibility, existing
   `nexuspaas_http_requests_total`, and no loss of degraded counters.
4. Update observability dashboard and PrometheusRule latency expressions to use
   `histogram_quantile(0.95, sum by (service, le) (rate(..._bucket[5m])))` for
   the service dashboard and service-level read/write alerts.
5. Update deployment policy tests to require histogram bucket/count metrics and
   p95 alert/dashboard markers, and to reject the old `HighMean`/mean latency
   expressions.
6. Update operational readiness, observability strategy, and `problem.md`.

## 16. Verification Plan

- `gofmt -w backend/internal/platform/metrics.go backend/internal/platform/metrics_rollback_admin_test.go backend/internal/platform/deployment_test.go`
- `kubectl kustomize backend/deploy/observability/production-beta`
- Ruby YAML + embedded Grafana JSON parse for the observability overlay.
- `go test ./internal/platform -run 'Metrics|Deployment|Operational|Observability|Release|Beta' -count=1`
- `go test ./... -count=1`
- `go vet ./...`
- `go build ./...`
- `git diff --check`
- `bash backend/scripts/ci-security-gate.sh security`
- `bash backend/scripts/ci-security-gate.sh sonar`

Docker-backed E2E is not expected to be required locally because this slice
changes in-process metric exposition and static manifests, but GitHub Backend
Quality Gate will still run Docker-backed E2E after PR creation.

## 17. Rollback Plan

Revert this PR. The runtime will return to count/sum-only metrics and the
observability overlay will return to mean latency sentinels. Application API and
database state are unaffected.

## 18. Risks and Tradeoffs

- A custom histogram implementation is smaller for this PR but less feature-rich
  than `client_golang`; later adoption may still be worthwhile.
- Static buckets may need tuning after live traffic shows real distributions.
- p95 alerts become actionable only after the live Prometheus scrape overlay is
  activated in staging/production.

## 19. Reviewer Checklist

- Histogram buckets are cumulative and include `+Inf`.
- Existing metrics remain backward-compatible.
- No new sensitive labels or config values are introduced.
- Dashboard and alerts use `histogram_quantile(0.95, ...)`.
- `problem.md` removes only the metrics granularity gap and preserves live
  activation/staging blockers.
- Tests prove the metric contract rather than only checking string existence.

## 20. Status

Status: Approved
