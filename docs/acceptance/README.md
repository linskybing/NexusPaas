# GA Target, Acceptance Criteria, and Iteration Plan

Status: Draft for GA planning.

This is the GA acceptance specification for **NexusPaaS**, split into one file
per area for progressive disclosure. NexusPaaS is the primary name; a downstream
deployment can be rebranded (for example to **CSCC AI Platform**) with no code
change — see [naming](naming.md) for the single rebrand seam.

> Faithful split: every section, model, flow, policy table, and acceptance
> criterion from the original `ac.md` lives in exactly one area doc below.
> [gap-analysis.md](gap-analysis.md) is the only doc with new (proposed) content.

## Final Product Goal

NexusPaaS is a multi-tenant Kubernetes-based AI platform for controlled
deployment, GPU sharing, GUI streaming, image building, RBAC, quota, queueing,
and usage accounting. It must allow users to deploy Kubernetes YAML safely, but
must not behave like unrestricted `kubectl apply`.

The GA target:

> A user can log in with the `nexus` CLI or Web UI, select an authorized
> Project, submit a versioned ConfigFile containing Kubernetes YAML, pass
> platform security validation, pass image allow-list validation, pass Plan /
> Queue / Quota admission, optionally request DRA + NVIDIA MPS fractional GPU
> resources, optionally enable WebRTC GUI streaming, and have all CPU, memory,
> GPU, process, container, user, project, and group usage attributed correctly.

## Repository Baseline

The existing NexusPaaS codebase already contains useful service boundaries that
should be preserved and hardened instead of rewritten. The architecture is **a
modular monolith with microservice-ready boundaries, migrating toward 8
deployable units** (full ownership map in
[`../architecture/service-boundaries.md`](../architecture/service-boundaries.md)
and [`../adr/0001-ga-8-deployable-units.md`](../adr/0001-ga-8-deployable-units.md)).

| Deployable Unit | Current Logical Services | GA Responsibility |
|---|---|---|
| `platform-gateway` | platform-gateway | Edge routing, auth entry, service registry, OpenAPI, public API compatibility |
| `iam-unit` | identity-service, authorization-policy-service | Login, sessions, users, API tokens, RBAC, PDP, policy bundles |
| `tenant-unit` | org-project-service | Groups, Projects, membership, project plan binding, group/project settings |
| `collaboration-unit` | audit-compliance-service, request-notification-service, media-upload-service | Audit, forms, notifications, media metadata |
| `platform-io-unit` | storage-service, image-registry-service, integration-proxy-service | Storage, image build, Harbor, image allow list, external proxy integrations |
| `usage-observability` | usage-observability-service, gpuusage, resourcehours | Usage snapshots, GPU usage, resource-hour summaries, dashboards |
| `compute-api` | workload-service, ide-service | ConfigFile lifecycle, job submit/list/cancel, IDE/WebRTC job lifecycle |
| `compute-control-plane` | scheduler-quota-service, k8s-control-service | Plan, Queue, Quota, admission, preemption, Kubernetes apply/delete/status |

It should not be described as fully mature microservices until these are
complete: typed domain data ownership; outbox/inbox coverage beyond the current
delivery-evidence baseline; service identity; provider abstraction; contract
tests for owner-read/command/event contracts; independent deployable-unit
readiness, rollback, and smoke evidence.

## Acceptance Areas

| Area | AC family | Doc |
|---|---|---|
| Naming and branding | NAME | [naming.md](naming.md) |
| CNCF / cloud-native adoption | CNCF | [cncf-adoption.md](cncf-adoption.md) |
| A. Kubernetes resource deployment | K8S | [k8s-deployment.md](k8s-deployment.md) |
| B. Project capability gates | CAP | [project-capabilities.md](project-capabilities.md) |
| C. Plan, Queue, Quota, Preemption | QUEUE | [plan-queue-quota.md](plan-queue-quota.md) |
| D. GPU DRA + NVIDIA MPS | GPU | [gpu-dra-mps.md](gpu-dra-mps.md) |
| E. Container/PID/process/GPU usage attribution | USAGE | [usage-attribution.md](usage-attribution.md) |
| F. WebRTC GUI workloads | RTC | [webrtc.md](webrtc.md) |
| G. Image build, Harbor, allow list | IMG | [image-build.md](image-build.md) |
| H. NexusPaaS CLI | CLI | [cli.md](cli.md) |
| I. RBAC, Group, Project, Personal Project | RBAC | [rbac.md](rbac.md) |
| J. Monitoring, usage, reporting | MON | [monitoring.md](monitoring.md) |
| Data ownership, events, contracts | DATA | [data-contracts.md](data-contracts.md) |
| Security | SEC | [security.md](security.md) |
| Operations, reliability, DR | OPS | [operations.md](operations.md) |
| Performance and scale | PERF | [performance.md](performance.md) |

## Planning References

- [coverage-matrix.md](coverage-matrix.md) — user requirement → area → AC IDs.
- [iteration-plan.md](iteration-plan.md) — milestones M0–M8 (vertical slices).
- [ga-checklist.md](ga-checklist.md) — GA release checklist + final GA definition.
- [gap-analysis.md](gap-analysis.md) — **v1-readiness review + proposed extra ACs.**
