# Security Requirements

Part of the [GA Acceptance docs](README.md).

## Security Baseline

| Domain | Required Policy |
|---|---|
| Authentication | OIDC/JWT or approved enterprise identity |
| Authorization | Platform RBAC + Project RBAC + PDP |
| Service identity | Workload identity, mTLS, or per-service scoped credentials |
| Pod security | Restricted by default |
| Network | Default-deny Project namespace |
| Image | Digest allow list, scan, SBOM, signature/attestation where enabled |
| Secrets | External Secrets or Vault-backed process |
| Audit | All privileged actions audited |
| Admin protection | Last platform admin cannot be removed |
| Internal APIs | Service identity required |
| Gateway | Does not replace domain authorization |

## Acceptance Criteria

| ID | Acceptance Criteria |
|---|---|
| SEC-001 | Production cannot run with allow-all PDP. |
| SEC-002 | Staging cannot run with allow-all PDP unless explicitly approved for a temporary test environment. |
| SEC-003 | Production cannot default to `SERVICE_NAME=all`. |
| SEC-004 | Internal routes require centralized service identity middleware. |
| SEC-005 | Static shared `SERVICE_API_KEY` is not accepted as final GA service identity. |
| SEC-006 | API token lookup does not scan all token hashes. |
| SEC-007 | API token format includes token ID or prefix for indexed lookup. |
| SEC-008 | Trusted proxy logic is used consistently for client IP extraction. |
| SEC-009 | Login failure tracking cannot be bypassed by spoofed X-Forwarded-For. |
| SEC-010 | JWT/JWKS validation uses a mature library or has formal security review. |
| SEC-011 | Password hashing uses Argon2id or has an approved migration plan. |
| SEC-012 | Legacy plain/raw password hash verification is disabled in production. |
| SEC-013 | Secrets are not committed to Git. |
| SEC-014 | Image build logs redact secrets. |
| SEC-015 | HostPath, privileged, hostNetwork, hostPID, hostIPC are forbidden unless Project capability allows them. |
| SEC-016 | User workloads cannot access Kubernetes service account tokens unless explicitly required and scoped. |
| SEC-017 | User workloads cannot mount container runtime sockets. |
| SEC-018 | Admission bypass attempts are tested. |
| SEC-019 | RBAC bypass attempts are tested. |
| SEC-020 | Security runbook covers credential rotation, compromised Project, malicious image, and orphan workload. |

## Scoped Evidence Notes

- 2026-06-30 SEC-016 local/static dispatcher evidence: workload dispatch now
  forces `automountServiceAccountToken=false` on user workload PodSpecs for
  native Pod, Job, Deployment, Volcano VCJob tasks, synthesized VCJob templates,
  and Volcano fallback Pods. This does not close workload identity, mTLS, live
  staging security, SEC GA, or Full GA.
- 2026-06-30 SEC-017 local/static admission and dispatcher evidence: scheduler
  admission and workload dispatch now reject user workload PodSpecs mounting
  known Docker, containerd, or CRI-O runtime sockets through `hostPath`,
  including native Pod, Job, Deployment, Volcano VCJob tasks, synthesized VCJob
  templates, and Volcano fallback Pods. This does not close workload identity,
  mTLS, live staging security, SEC GA, or Full GA.
- 2026-06-30 SEC-018 local/static admission evidence: scheduler admission now
  parses raw manifest kind/name before trusting explicit resource metadata, so
  spoofed safe metadata cannot hide raw Secrets, runtime socket hostPath mounts,
  or unsupported workload kinds. This does not close workload identity, mTLS,
  live staging security, SEC GA, or Full GA.
- 2026-06-30 SEC-019 local/static route-catalog evidence: platform route
  validation now rejects RBAC bypass-prone metadata, including non-allowlisted
  public external API routes, user-facing policy bypass routes, and unprotected
  admin routes. The full registered catalog is checked in production-auth mode.
  This does not close live authorization, workload identity, mTLS, live staging
  security, SEC GA, or Full GA.
- 2026-07-01 kind-tier launch drill secret evidence + disclosed deviation
  ([`evidence/2026-07-01-kind-live-e2e-report.md`](evidence/2026-07-01-kind-live-e2e-report.md)):
  all required backing and per-unit runtime Secret objects were created and
  verified present by name/key (no values printed) and loaded into the live
  8-unit deployment. **Deviation (kind-tier only):** the platform units that host
  cluster-dependent services were granted an automounted ServiceAccount token
  bound to `cluster-admin` so the in-cluster readiness ping succeeds on a
  throwaway local cluster. This is a platform-unit convenience for kind and is
  **not** a production posture — production uses least-privilege workload
  identity, and the SEC-016 `automountServiceAccountToken=false` guard for
  **user workloads** is unchanged. External Secret provenance/rotation, workload
  identity, and mTLS remain open.
