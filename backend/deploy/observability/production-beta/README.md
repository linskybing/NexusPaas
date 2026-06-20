# Production Beta Observability Overlay

This overlay provisions the baseline Production Beta observability resources for
the 8-unit backend topology. It is intentionally separate from the root
`backend/kustomization.yaml` because `PodMonitor` and `PrometheusRule` require
Prometheus Operator CRDs in the target cluster.

## Prerequisites

- The root Production Beta topology has been applied from `backend/`.
- Prometheus Operator is installed and watches the `nexuspaas` namespace.
- Grafana is configured to load dashboard ConfigMaps labeled
  `grafana_dashboard: "1"`, or the dashboard JSON is imported manually.
- kube-state-metrics is available for deployment and CronJob/Job alerts.
- Operators create these secrets in `nexuspaas` before applying the overlay:

```bash
kubectl -n nexuspaas create secret generic nexuspaas-prometheus-scrape-secret \
  --from-literal=bearer-token='<valid metrics bearer token>'

kubectl -n nexuspaas create secret generic nexuspaas-synthetic-smoke-secret \
  --from-literal=api-key='<valid admin smoke api key>' \
  --from-literal=service-key='<valid internal smoke service key>'
```

Do not commit these values. The Prometheus scrape token must be accepted by all
8 backend units as a bearer credential for the admin `/metrics` route. The synthetic
smoke API key should be scoped to read operational endpoints and the listed
read-only smoke endpoints.

## Apply

```bash
kubectl kustomize backend/deploy/observability/production-beta
kubectl apply -k backend/deploy/observability/production-beta
```

## Coverage

- `PodMonitor` scrapes authenticated `/metrics` on every backend pod and relabels
  the Kubernetes `app` label into the Prometheus `service` label.
- `PrometheusRule` adds baseline alerts for scrape loss, deployment
  unavailability, core availability burn, high p95 latency, 5xx responses, and
  synthetic smoke failures.
- `nexuspaas-production-beta-dashboard` defines the Grafana dashboard for the 8
  backend units and the Beta SLO targets.
- `nexuspaas-synthetic-smoke` runs every five minutes and checks `/healthz`,
  `/readyz`, `/metrics`, gateway `/openapi.json`, gateway `/service-registry`,
  and one read-only endpoint per service. Read-only smoke endpoints may return
  expected 4xx for auth/empty data cases, but never 5xx.

## Rollback

```bash
kubectl delete -k backend/deploy/observability/production-beta
```

Deleting this overlay does not change application deployments, databases, or
runtime configuration. It removes only dashboard, scrape, alert, and scheduled
synthetic monitoring resources.
