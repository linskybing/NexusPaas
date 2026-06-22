# Storage Mount-Plan Audit Slice

## 1. Objective

Close the `STORAGE-004` v1 gap for storage mount-plan decisions by making
successful decisions explicitly auditable and easier to correlate by Project.

## 2. Background

`docs/acceptance/gap-analysis.md` marks storage binding and mount-plan
validation as launch-blocking. The service already resolves mount plans from
storage-owned project bindings, group storage sources, and effective
permissions, and existing tests prove forged PVC source/target fields are not
trusted. The remaining narrow gap is auditability of the mount-plan decision.

## 3. Scope

- Add route metadata so generic `AuditEvent` records identify the Project for
  the internal mount-plan route.
- Publish a storage-owned sanitized domain event for successful mount-plan
  decisions.
- Add focused tests proving the decision event contains project/user/pvc
  decision data without trusting request-supplied source PVC details.
- Update the v1 checklist with storage progress.

## 4. Non-Goals

- No new storage backend, CSI integration, or PVC provisioning behavior.
- No custom audit backend. Use the existing event bus and platform audit path.
- No change to storage permission semantics.
- No Web UI, secret API, PlanAdmin, or audit-query implementation.

## 5. CNCF / Cloud-Native Fit

This slice keeps Kubernetes/PVC behavior in the existing storage service and
does not introduce a custom scheduler, CSI driver, secret store, registry, or
metrics backend. It adds audit evidence through the existing event bus and
platform route audit.

## 6. Affected Files

- `docs/plan/2026-06-20-storage-mount-plan-audit.md`
- `docs/plan/2026-06-20-v1-launch-gap-gate.md`
- `backend/internal/services/storage/spec.go`
- `backend/internal/services/storage/mount_plan_contracts.go`
- `backend/internal/services/storage/mount_plan_contracts_test.go`

## 7. Contract Changes

- Add storage domain event `StorageMountPlanResolved`.
- Event payload must include only sanitized identifiers and counts:
  `project_id`, `user_id`, `namespace`, `mount_count`,
  `manifest_mount_count`, `share_operation_count`, `pvc_ids`,
  `target_pvcs`, and `action`.
- Do not include raw manifest content, service credentials, source payload
  details, or secret values.

## 8. Implementation Steps

- [x] Mark the internal mount-plan route with `ID("project_id")`.
- [x] Publish `StorageMountPlanResolved` after successful mount-plan resolution.
- [x] Add a focused storage service test for the decision event payload.
- [x] Run focused storage tests.
- [x] Run quick gate and Sonar.
- [x] Update V1 checklist status/evidence.

## 9. Verification Plan

```sh
go -C backend test ./internal/services/storage -run 'MountPlan|Spec' -count=1
go -C backend test ./internal/services -run 'AuditEvents|RouteCoverage|CatalogStateChanging' -count=1
go -C backend test ./... -coverprofile=coverage.out -count=1
bash backend/scripts/ci-security-gate.sh quick
bash backend/scripts/ci-security-gate.sh sonar
```

Executed on 2026-06-20:

```sh
go -C backend test ./internal/services/storage -run 'MountPlan|Spec' -count=1 -v
go -C backend test ./internal/services -run 'AuditEvents|RouteCoverage|CatalogStateChanging|EventContracts|ServiceCatalog|OpenAPI' -count=1
go -C backend test ./internal/services/storage -count=1
go -C backend test ./... -coverprofile=coverage.out -count=1
bash backend/scripts/ci-security-gate.sh quick
bash backend/scripts/ci-security-gate.sh sonar
```

Result: all commands passed; SonarScanner Quality Gate passed.

## 10. Rollback Plan

Revert this slice. No schema changes or persistent-data migrations are involved.

## 11. Risks

- Additional domain event volume is one event per successful mount-plan
  decision. This is acceptable for v1 and bounded by existing gateway limits.
- Event payload must stay sanitized because storage requests may be derived from
  workload submissions.

## 12. Reviewer Checklist

| Check | Status |
|---|---|
| Requirement fit for `STORAGE-004` | Pass |
| Scope stays limited to auditability | Pass |
| Reuses existing event/audit infrastructure | Pass |
| SOLID: event payload construction is isolated | Pass |
| 12-Factor: no hard-coded environment coupling | Pass |
| Tests/build/Sonar evidence recorded | Pass |
| Risks and diff scope reviewed | Pass |

## 13. Status

Status: Implemented and reviewer-verified for this slice.

Reviewer Agent: Approved and verified. The implementation completes the narrow
auditable-decision gap without changing storage authorization semantics, PVC
lifecycle behavior, or the platform audit backend.
