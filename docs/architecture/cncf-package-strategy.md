# CNCF And Package Strategy

## Principle

NexusPaas should build product-specific PaaS behavior and rely on mature
open-source or CNCF-aligned projects for generic infrastructure. New major
packages or platform products require an ADR that explains maturity, license,
operational cost, security posture, and why existing repository code or the
standard library is insufficient.

## Build In NexusPaas

| Capability | Reason |
| --- | --- |
| Tenant, project, group, and membership model | Product-specific access and quota semantics. |
| ConfigFile, job, IDE, quota, plan, and preemption workflow | Core platform domain behavior and audit requirements. |
| Image request/build governance and project allow-list policy | Product governance over Harbor/image usage. |
| Storage binding and project mount-plan rules | Product-specific connection between tenancy, compute, and storage. |
| Usage read models and GPU/resource accounting | Product-specific reporting, quota, and user visibility. |
| Audit, request, notification, and media domain records | Product workflows and compliance evidence. |

## Prefer External Products

| Concern | Preferred Direction | Notes |
| --- | --- | --- |
| Ingress/API gateway runtime | Kubernetes Gateway API, Envoy, Kong, Traefik, or NGINX | Gateway business behavior stays in NexusPaas; edge traffic management should use mature runtime components when deployed. |
| GitOps | Argo CD or Flux | Required before declaring repeatable staging/production promotion. |
| Progressive delivery | Argo Rollouts or Flagger | Add after staging evidence exists and canary health signals are reliable. |
| Observability | OpenTelemetry Collector, Prometheus, Grafana, Loki, Tempo, Jaeger | Keep vendor-neutral telemetry and avoid custom metrics pipelines. |
| Policy admission | OPA/Gatekeeper or Kyverno | Use for cluster admission and policy-as-code, not as a replacement for domain authorization. |
| Secrets | External Secrets Operator, Sealed Secrets, or Vault when justified | Static secrets in Git remain forbidden. |
| Certificates | cert-manager | Use for ingress/service certificates where needed. |
| Packaging | Kustomize first; Helm when chart reuse or parameterization justifies it | Existing manifests are Kustomize-oriented. Avoid packaging churn without operational value. |
| Registry and supply chain | Harbor, Cosign, Syft, Grype, Trivy, OSV-Scanner | Harbor remains the product registry integration; scans and signing belong in release gates. |
| Messaging/events | Redis Streams now; NATS or Kafka only when scale/retention needs prove it | Do not replace the event bus just for architecture aesthetics. |
| Service mesh | Linkerd or Istio only with proven need | Workload identity and network policy are the first GA security steps. |

## Package Adoption Rules

- Prefer the Go standard library and existing repository helpers for simple
  behavior.
- Add a package only when it removes real complexity or provides mature,
  maintained infrastructure behavior.
- Check license compatibility and maintenance activity before adoption.
- Keep shared libraries small; they must not force synchronized releases.
- Any major runtime, storage, messaging, security, or observability dependency
  needs an ADR and rollback plan.

## Near-Term Decisions

- Keep the current backend baseline: Go standard library `net/http`,
  `http.ServeMux`, `pgx`, PostgreSQL, Redis Streams, MinIO/S3-compatible
  storage, Kubernetes clients, Prometheus, and OpenTelemetry-compatible
  telemetry.
- Treat Gin, GORM, Kafka, service mesh, or a new gateway product as future
  choices that require a concrete need and ADR.
- Do not introduce Kafka, a service mesh, or a new gateway product in the first
  90 days unless a blocking scale/security requirement appears.
- Prioritize GitHub Sonar provisioning, staging GitOps evidence, and supply
  chain gates over adding new platform products.
