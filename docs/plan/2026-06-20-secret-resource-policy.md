# Secret Resource Policy Slice

## 1. Objective

Close the v1 `SECRET-*` raw Kubernetes Secret gap by rejecting user-submitted
raw `Secret` resources by default, emitting safe audit evidence, and preserving
the CNCF-approved direction of External Secrets/Vault instead of a custom secret
store.

## 2. Background

The acceptance docs say Kubernetes `Secret` resources are discouraged and should
be allowed only through a platform secret API or approved ExternalSecret
profile. The current scheduler admission resource policy blocks some unsupported
workload controllers but does not reject raw `Secret` manifests. The cluster
facade can still create Secrets, which should remain available for internal
controlled deployment/runtime-secret contracts.

## 3. Source References

- `docs/acceptance/gap-analysis.md`
- `docs/acceptance/cncf-adoption.md`
- `docs/acceptance/k8s-deployment.md`
- `backend/internal/services/schedulerquota/admission.go`
- `backend/internal/services/schedulerquota/admission_resources.go`
- `backend/internal/services/schedulerquota/admission_test.go`
- `backend/internal/services/workload/dispatcher.go`
- `backend/internal/services/workload/dispatcher_test.go`
- `backend/internal/platform/events.go`

## 4. Assumptions

- For v1, the approved secret workflow is External Secrets/Vault/Kubernetes
  managed runtime secret contracts; a custom NexusPaaS secret store is a
  non-goal.
- User-submitted job resources must not include raw Kubernetes `Secret`
  manifests.
- Internal platform deployment code may still need Kubernetes Secret support
  through the cluster facade.

## 5. Non-Goals

- No custom secret CRUD service, encryption subsystem, or Vault client.
- No database migration.
- No removal of Kubernetes Secret support from internal cluster apply code.
- No Web UI secret management.

## 6. Current Behavior

- Scheduler admission accepts resource payloads with kind `Secret` unless another
  quota/resource rule rejects them.
- Dispatcher can eventually call the cluster facade with a raw `Secret`
  resource if admission is bypassed.
- Secret-related event redaction exists, but raw Secret rejection is not audited.

## 7. Target Behavior

- Scheduler admission rejects raw Kubernetes `Secret` resources with a clear
  policy reason before quota reservation.
- Rejection response does not include plaintext secret values.
- Rejection emits a safe `SecretAccessRejected` event plus `AuditEvent` metadata
  with project/user/resource kind/name/reason and no raw manifest.
- Workload dispatcher rejects raw `Secret` resources as defense in depth.
- Existing ExternalSecret/Vault direction remains documented; no custom secret
  store is introduced.

## 8. Affected Domains

- Scheduler admission resource policy.
- Workload dispatcher resource preparation.
- Audit event evidence for rejected secret submissions.

## 9. Affected Files

- `docs/plan/2026-06-20-secret-resource-policy.md`
- `docs/plan/2026-06-20-v1-launch-gap-gate.md`
- `backend/internal/services/schedulerquota/admission.go`
- `backend/internal/services/schedulerquota/admission_resources.go`
- `backend/internal/services/schedulerquota/admission_test.go`
- `backend/internal/services/workload/dispatcher.go`
- `backend/internal/services/workload/dispatcher_test.go`

## 10. API / Contract Changes

- Scheduler admission may return `403` for raw Kubernetes `Secret` resources.
- New events:
  - `SecretAccessRejected`
  - `AuditEvent` with `resource_type=secret`, `success=false`, and safe
    resource metadata.

## 11. Database / Migration Changes

None.

## 12. Configuration Changes

None. Raw `Secret` rejection is the default v1 policy.

## 13. Observability Changes

Rejected raw Secret submissions become visible through the existing event/audit
stream.

## 14. Security Considerations

- Do not log, return, or publish raw Secret manifest contents, `data`, or
  `stringData`.
- Keep platform/runtime secret creation on the internal cluster facade intact.
- This complements, but does not replace, Kubernetes admission policy such as
  Kyverno or ValidatingAdmissionPolicy in a real cluster.

## 15. Implementation Steps

- [x] Add raw Secret policy validation to scheduler admission resources.
- [x] Publish safe rejection domain/audit events from scheduler admission.
- [x] Add dispatcher defense-in-depth rejection for raw Secret resources.
- [x] Add tests for rejection status, no plaintext leakage, event evidence, and
  dispatcher block.
- [x] Run focused scheduler/workload tests, full backend tests, quick gate, and
  Sonar.
- [x] Update V1 checklist evidence.

## 16. Verification Plan

```sh
go -C backend test ./internal/services/schedulerquota -run 'Secret|Admission' -count=1
go -C backend test ./internal/services/workload -run 'Secret|Dispatch' -count=1
go -C backend test ./internal/services -run 'AuditEvents|EventContracts|CatalogStateChanging' -count=1
go -C backend test ./... -coverprofile=coverage.out -count=1
bash backend/scripts/ci-security-gate.sh quick
bash backend/scripts/ci-security-gate.sh sonar
```

Executed on 2026-06-20:

```sh
go -C backend test ./internal/services/schedulerquota -run 'Secret|Admission' -count=1 -v
go -C backend test ./internal/services/workload -run 'Secret|DispatchResources' -count=1 -v
go -C backend test ./internal/services/schedulerquota -count=1
go -C backend test ./internal/services/workload -count=1
go -C backend test ./internal/services -run 'AuditEvents|EventContracts|CatalogStateChanging|RegisterAllAdminCoverage|RouteCoverage' -count=1
go -C backend test ./... -coverprofile=coverage.out -count=1
bash backend/scripts/ci-security-gate.sh quick
bash backend/scripts/ci-security-gate.sh sonar
```

Result: all commands passed; SonarScanner Quality Gate passed.

## 17. Rollback Plan

Revert this slice. No schema or persistent data migration is involved.

## 18. Risks and Tradeoffs

- Users who previously submitted raw Secret manifests must migrate to the
  approved External Secrets/Vault/runtime secret workflow.
- This is policy enforcement, not secret storage. A richer secret API can be
  added later if the product chooses to own that capability.

## 19. Reviewer Checklist

| Check | Status |
|---|---|
| Requirement fit for `SECRET-001..003` v1 safety evidence | Pass |
| Scope avoids custom secret store | Pass |
| CNCF fit: External Secrets/Vault direction preserved | Pass |
| SOLID: policy validation and audit publishing are isolated | Pass |
| 12-Factor: no new env coupling | Pass |
| Security: no plaintext secret leakage | Pass |
| Verification plan is concrete | Pass |
| Rollback is realistic | Pass |
| Simplicity / no over-engineering | Pass |

## 20. Status

Status: Implemented and reviewer-verified for this slice.

Reviewer Agent: Approved and verified. The implementation rejects raw
Kubernetes `Secret` manifests through scheduler admission and dispatcher
defense-in-depth, emits safe `SecretAccessRejected` and `AuditEvent` metadata,
does not return or publish plaintext secret values, avoids a custom secret
store, and passes focused, full, quick, and Sonar verification.
