# GA Release Checklist and Final GA Definition

Part of the [GA Acceptance docs](README.md).

## GA Release Checklist

NexusPaaS may be declared GA only when all required categories pass.

| Category | Required Result |
|---|---|
| Naming | Configured external naming complete |
| Kubernetes deployment | K8S acceptance complete |
| Project capability | CAP acceptance complete |
| Queue/Plan/Quota | QUEUE acceptance complete |
| GPU DRA/MPS | GPU acceptance complete |
| Usage attribution | USAGE acceptance complete |
| WebRTC | RTC acceptance complete |
| Image build | IMG acceptance complete |
| CLI | CLI acceptance complete |
| RBAC | RBAC acceptance complete |
| Monitoring | MON acceptance complete |
| Data contracts | DATA acceptance complete |
| Security | SEC acceptance complete |
| Operations | OPS acceptance complete |
| Performance | PERF acceptance complete |
| Documentation | User/admin/operator docs complete |
| E2E | Critical live E2E tests complete |
| Backup/restore | Restore drill complete |
| Rollback | Rollback drill complete |
| Failure injection | Failure tests complete |
| Release blockers | No unresolved GA blocker |

## Final GA Definition

NexusPaaS is GA-ready when:

> A user can use the `nexus` CLI or Web UI to log in, select an authorized
> Project, build or select an allow-listed image, submit a versioned Kubernetes
> ConfigFile, pass security and capability validation, pass Plan/Queue/Quota
> admission, deploy through k8s-control-service, optionally use DRA + MPS
> fractional GPU, optionally open a WebRTC GUI session in the browser, and have
> all CPU, memory, GPU, process, container, user, Project, and Group usage
> attributed correctly, with audit, monitoring, rollback, backup, and security
> evidence complete.

The platform must prove that:

- Users cannot escape their Project.
- Users cannot bypass image allow lists.
- Users cannot self-grant root, hostPath, external egress, privileged, or host namespace access.
- Queue and Plan rules are always enforced.
- High-priority preemption works only against lower-priority preemptible workloads.
- MPS shared GPU workloads are separated by container/PID ownership.
- process-exporter and GPU process sampling support process-level attribution.
- Billing-grade GPU accounting is based on reliable reservation or validated measured metrics.
- UI never mislabels estimated MPS usage as measured actual usage.
- Group and Project usage can be queried over time ranges.
- Every critical action is audited.
- Every deployable unit can be deployed, observed, rolled back, and recovered.
