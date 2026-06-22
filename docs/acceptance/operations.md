# Operations, Reliability, and Disaster Recovery

Part of the [GA Acceptance docs](README.md).

## Required Operational Capabilities

| Capability | Requirement |
|---|---|
| Health checks | Every deployable unit has `/healthz` |
| Readiness checks | Every deployable unit has `/readyz` |
| Metrics | Every deployable unit exposes Prometheus metrics |
| Logs | Structured logs with trace/request IDs |
| Traces | OpenTelemetry instrumentation |
| Runbooks | Required for every unit |
| Rollback | Required for every unit |
| Backup | PostgreSQL, object storage, Harbor metadata, critical secrets |
| Restore | Restore drill before GA |
| GitOps | Staging and production managed through GitOps |
| Synthetic smoke | Required after deploy and rollback |
| Reconcile | Required for Kubernetes, quota, build, image, usage drift |

## Acceptance Criteria

| ID | Acceptance Criteria |
|---|---|
| OPS-001 | Every deployable unit has health, readiness, metrics, logs, traces, and owner. |
| OPS-002 | Every deployable unit has a runbook. |
| OPS-003 | Every deployable unit has rollback procedure. |
| OPS-004 | Staging environment can be rebuilt from GitOps. |
| OPS-005 | Production deployment is GitOps-managed or has equivalent release evidence. |
| OPS-006 | PostgreSQL backup and restore drill passes. |
| OPS-007 | Harbor backup and restore drill passes. |
| OPS-008 | Object storage backup and restore drill passes. |
| OPS-009 | Secret recovery procedure is documented and tested. |
| OPS-010 | K8s API temporary outage does not corrupt DB state. |
| OPS-011 | Redis/event broker temporary outage does not silently lose committed events. |
| OPS-012 | Harbor outage degrades image build/list behavior clearly. |
| OPS-013 | Prometheus outage marks telemetry stale but does not grant quota. |
| OPS-014 | Reconciler repairs Kubernetes/DB drift. |
| OPS-015 | Reconciler repairs quota/reservation drift. |
| OPS-016 | Reconciler repairs build status drift. |
| OPS-017 | Reconciler repairs image allow-list / Harbor deletion drift. |
| OPS-018 | Load test covers login, deploy, build, queue, metrics, usage query, and stream credential paths. |
| OPS-019 | Failure-injection test covers DB, event broker, Harbor, K8s API, Prometheus, and node usage-agent failure. |
| OPS-020 | No GA blocker remains open in release blocker tracking. |
