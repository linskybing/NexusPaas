# CNCF / Cloud-Native Adoption Principle

Part of the [GA Acceptance docs](README.md).

NexusPaaS should not rebuild mature cloud-native infrastructure.

The platform should own product-specific orchestration and governance logic,
while delegating common infrastructure functions to mature Kubernetes / CNCF
ecosystem components.

## Build vs Adopt

| Domain | NexusPaaS Owns | Adopted Component |
|---|---|---|
| Project / Group model | Yes | N/A |
| Plan / Queue entitlement | Yes | Kueue for queue execution |
| ConfigFile versioning | Yes | Kubernetes API for actual resources |
| Kubernetes admission policy | Thin integration | Kyverno + ValidatingAdmissionPolicy |
| GPU allocation | Policy and accounting | Kubernetes DRA + NVIDIA stack |
| GPU telemetry | Attribution and rollup | DCGM Exporter + node-local usage agent |
| Process telemetry | Attribution and rollup | process-exporter + node-local usage agent |
| Image build workflow | Policy and API | Tekton + rootless BuildKit |
| Registry governance | Policy and allow list | Harbor + scanner + signing |
| Metrics | Product dashboards | Prometheus Operator |
| Logs | Product views | Loki or equivalent |
| Traces | Trace correlation | OpenTelemetry Collector |
| GitOps | Release policy | Argo CD or Flux |
| Secrets | Product integration | External Secrets Operator or Vault |

## Anti-Rebuild Rules

| ID | Rule |
|---|---|
| CNCF-01 | Do not build a custom Kubernetes scheduler. Use Kueue / Kubernetes scheduler integration. |
| CNCF-02 | Do not build a custom admission-policy engine. Use Kyverno and/or ValidatingAdmissionPolicy. |
| CNCF-03 | Do not use Docker-in-Docker as the default image build path. Use rootless BuildKit through Tekton. |
| CNCF-04 | Do not build a custom metrics backend. Use Prometheus-compatible metrics and OpenTelemetry. |
| CNCF-05 | Do not build a custom registry. Use Harbor or another OCI registry provider. |
| CNCF-06 | Do not build a custom secret store. Use Kubernetes Secrets only through a managed secret workflow, External Secrets, or Vault. |
| CNCF-07 | Do not build a permanent custom event bus without transactional outbox/inbox semantics. |
