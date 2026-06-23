# Storage Mount-Plan Cross-Project and Cross-User Isolation Proof

Status: Approved

## 1. Objective

Add a narrow local, no-Docker proof for `STORAGE-002` showing that storage
mount-plan resolution does not reuse another Project's storage binding and does
not reuse another user's PVC permission grant.

This is a test and evidence slice only. It must not change runtime behavior,
workload dispatch behavior, Kubernetes PVC handling, deployment manifests,
fixtures, or live cluster execution.

## 2. Background

The previous approved slice
`docs/plan/2026-06-23-storage-mount-plan-authorization-proof.md` added local
in-memory `STORAGE-001` resolver tests in
`backend/internal/services/storage/mount_plan_contracts_test.go`.

Those tests prove the mount-plan resolver requires a storage-owned project
binding, a dispatch-ready group storage source, and an effective user
permission before returning a manifest mount and PVC share operation. The
remaining `STORAGE-002` gap is narrower: prove that otherwise valid records for
a different Project or different user do not authorize the current request.

## 3. Source References

- `AGENTS.md`
- `docs/agents/workflow.md`
- `docs/agents/planning.md`
- `docs/agents/coding-guidelines.md`
- `docs/agents/review-checklist.md`
- `docs/acceptance/gap-analysis.md`
- `gap.md`
- `problem.md`
- `docs/plan/2026-06-23-storage-mount-plan-authorization-proof.md`
- `backend/internal/services/storage/mount_plan_contracts.go`
- `backend/internal/services/storage/storage_repository.go`
- `backend/internal/services/storage/helpers.go`
- `backend/internal/services/storage/mount_plan_contracts_test.go`
- CodeGraph exploration for `resolveStorageMountPlan`,
  `EffectiveStoragePermission`, `FindProjectStorageBinding`,
  `projectBindingsResource`, `projectPermissionsResource`,
  `storagePermissionsResource`, `groupStorageResource`, `projectBindingID`,
  `projectPermissionID`, `storagePermissionID`, and `groupStorageID`

## 4. Assumptions

- `STORAGE-001` local/in-memory mount-plan authorization proof already exists
  and remains unchanged except for helper reuse if needed.
- `resolveStorageMountPlan` looks up project bindings with the request
  `ProjectID` and requested PVC ID.
- Missing project binding still returns HTTP `404` with
  `storage binding not found`.
- `EffectiveStoragePermission` checks permissions by exact project/PVC/user,
  then group/PVC/user, then policy default, then `none`.
- Permission denial still returns HTTP `403` with
  `storage permission denied`.
- Tests can stay in package `storage` and seed `platform.NewApp` in-memory store
  records directly using existing storage resource constants and ID helpers.
- Existing `STORAGE-001` success coverage is enough positive control for the
  correct Project/user path; this slice should not duplicate it unless Reviewer
  Agent requests that.

## 5. Non-Goals

- No runtime code changes in storage, workload, platform, shared helpers, or
  service-auth middleware.
- No new abstractions, dependencies, repository interfaces, or fixtures.
- No live Kubernetes, RKE2, Docker, PVC provisioning, CSI, namespace, or mount
  execution proof.
- No OpenAPI, route registration, middleware, deployment, Secret, environment,
  config, database migration, or frontend changes.
- No broad parser alias, HTTP contract, or audit event expansion.
- No claim that live cluster PVC isolation, namespace enforcement, CSI behavior,
  full storage GA, Full GA, or first-version readiness is proven.

## 6. Current Behavior

`resolveStorageMountPlan` resolves each requested mount by looking up the
project storage binding with `FindProjectStorageBinding(projectID, pvcID)`.
Because the lookup uses the request Project ID, an otherwise valid binding for
another Project should not be found and should fail closed as `404 storage
binding not found`.

After a binding and group source are found, the resolver calls
`EffectiveStoragePermission(projectID, groupID, pvcID, userID)`. Effective
permission checks are keyed by exact user ID at the project and group
permission levels. A grant for another user should not be reused for the
requesting user, and a writable request with no matching effective grant should
fail closed as `403 storage permission denied`.

Existing direct resolver tests already cover the valid same Project/user path,
unbound PVC denial, stopped/deleted group source denial, read-only versus
read-write permission behavior, and project-over-group permission precedence.

## 7. Target Behavior

Focused package-local tests prove these `STORAGE-002` isolation properties:

- A valid binding, source, and permission for `project-2` do not authorize a
  `project-1` mount-plan request for the same PVC and user.
- A valid binding and source for `project-1`, plus a `read_write` permission
  for `user-2`, do not authorize a writable `project-1` request from `user-1`.

The tests must assert the stable resolver status and error text for each
denial. They must not require runtime code changes.

## 8. Affected Domains

- `storage-service`: local in-memory resolver tests for Project and user
  isolation in mount-plan authorization.
- `workload-service`: consumer package regression verification only; no workload
  code changes are planned.
- Acceptance tracking docs: narrow evidence update for local/in-memory
  `STORAGE-002` isolation proof only.

No deployable service boundary, data ownership boundary, or inter-service API
contract changes are proposed.

## 9. Affected Files

Plan Agent changes only this file:

- `docs/plan/2026-06-23-storage-mount-plan-isolation-proof.md`

After Reviewer Agent approval, Code Agent may touch only:

- `backend/internal/services/storage/mount_plan_contracts_test.go`
- `gap.md`
- `problem.md`
- `docs/acceptance/gap-analysis.md`

Do not modify:

- `backend/internal/services/storage/mount_plan_contracts.go`
- `backend/internal/services/storage/storage_repository.go`
- `backend/internal/services/storage/helpers.go`
- `backend/internal/services/workload/storage_mount_client.go`
- runtime handlers, route specs, middleware, platform store code, deployment
  files, fixtures, frontend files, Kubernetes manifests, or trackers outside
  the three listed acceptance documents.

## 10. API / Contract Changes

None.

The existing internal mount-plan resolver behavior and error contract remain:

- missing request Project binding returns HTTP `404` and
  `storage binding not found`;
- insufficient effective permission returns HTTP `403` and
  `storage permission denied`;
- writable mounts require `read_write`;
- read-only mounts accept `read_only` or `read_write`.

## 11. Database / Migration Changes

None.

All proof data must use `platform.NewApp(platform.Config{...})` with the
in-memory store. No schema, migration, seed, fixture, or persistent data changes
are allowed.

## 12. Configuration Changes

None.

The tests should reuse the existing minimal local app/test setup in
`mount_plan_contracts_test.go`. Do not add environment variables, service URLs,
feature flags, or deployment configuration.

## 13. Observability Changes

None.

This slice does not change logs, metrics, traces, audit events, domain event
payloads, or service-auth behavior.

## 14. Security Considerations

- The cross-Project test must prove an unrelated Project binding is not reused
  even when PVC ID and user ID match.
- The cross-user test must prove another user's project or group grant is not
  reused for the requesting user.
- Synthetic IDs only; do not use real tenant names, credentials, Kubernetes
  tokens, service keys, or secret values.
- Assertions should check resolver status and stable error text only. Do not
  change runtime error shapes to satisfy tests.
- Tracker updates must preserve the distinction between local resolver
  authorization proof and live Kubernetes PVC isolation.

## 15. Implementation Steps

1. Open `backend/internal/services/storage/mount_plan_contracts_test.go` and
   reuse the existing package-local resolver helpers where possible:
   `newStorageMountPlanResolverApp`, `storageMountPlanResolverRequest`,
   `storageMountPlanWriteRequest`, `seedMountPlanBinding`,
   `seedMountPlanGroupSource`, `seedMountPlanProjectPermission`,
   `seedMountPlanGroupPermission`, and `assertMountPlanResolverError`.
2. Add
   `TestResolveStorageMountPlanRejectsOtherProjectBinding`.
3. In that test, seed a valid `project-2` binding for the requested PVC, a
   matching dispatch-ready group source, and a matching `read_write` permission
   for `project-2` and the same user.
4. Request the same PVC as `project-1` and `user-1`; assert HTTP `404` and
   exact error `storage binding not found`.
5. Add `TestResolveStorageMountPlanRejectsOtherUserPermission`.
6. In that test, seed a valid `project-1` binding and dispatch-ready group
   source for the requested PVC.
7. Seed a `read_write` permission for `user-2` only. Prefer project permission
   for the primary assertion; use group permission only if it keeps the helper
   reuse clearer.
8. Request the same PVC as `project-1` and `user-1`; assert HTTP `403` and
   exact error `storage permission denied`.
9. Do not add another positive-control success test unless Reviewer Agent
   requires it; the existing `STORAGE-001` success test already proves the
   correct Project/user succeeds.
10. Do not change production code. If either test fails because current runtime
    behavior reuses another Project's binding or another user's permission,
    stop and return the mismatch to Reviewer Agent instead of broadening scope.
11. Update `gap.md`, `problem.md`, and
    `docs/acceptance/gap-analysis.md` narrowly to record local/in-memory
    `STORAGE-002` cross-Project and cross-user mount-plan isolation proof.
12. Keep all broader storage gaps open, including live Kubernetes PVC
    isolation, namespace enforcement, CSI behavior, live mount execution,
    `STORAGE-003`, full storage GA, Full GA, and first-version readiness.

## 16. Verification Plan

Run from the repository root unless the command changes into `backend`:

```bash
cd backend && go test ./internal/services/storage -run 'TestResolveStorageMountPlanRejectsOtherProjectBinding|TestResolveStorageMountPlanRejectsOtherUserPermission' -count=1
cd backend && go test ./internal/services/storage -count=1
cd backend && go test ./internal/services/storage ./internal/services/workload -count=1
git diff --check
make -C backend check
make -C backend ci-sonar
```

Expected results:

- The two focused `STORAGE-002` isolation tests pass.
- Full storage package tests pass, including the existing `STORAGE-001`
  resolver proof.
- Storage plus workload package tests pass, confirming the consumer package is
  not regressed.
- `git diff --check` reports no whitespace errors.
- `make -C backend check` passes.
- `make -C backend ci-sonar` passes and the SonarScanner Quality Gate is green.

## 17. Rollback Plan

Revert only the approved implementation files for this slice:

- remove the two new resolver isolation tests and any tiny helper reuse added
  for them from `backend/internal/services/storage/mount_plan_contracts_test.go`;
- revert the narrow `STORAGE-002` tracker edits in `gap.md`, `problem.md`, and
  `docs/acceptance/gap-analysis.md`;
- remove this plan document if the slice is abandoned before approval.

No database, migration, runtime config, Kubernetes, Docker, deployment,
persistent data, or service rollback is required.

## 18. Risks and Tradeoffs

- This proves local resolver isolation only; it does not prove live Kubernetes
  PVC isolation, namespace policy enforcement, CSI behavior, or actual mount
  execution.
- Direct package-local tests are intentionally small and fast. They do not
  re-prove service-auth middleware, but that is acceptable because
  `STORAGE-002` targets resolver authorization keys.
- In-memory records can accidentally become unrealistic. Keep seeded IDs aligned
  with existing resource constants and helper IDs.
- Tracker wording is the main overclaim risk. Documentation must say
  local/in-memory `STORAGE-002` proof only and leave `STORAGE-003` and live
  storage readiness open.
- Adding extra success or parser tests would dilute the slice. Keep the diff to
  the two negative isolation cases unless Reviewer Agent requests otherwise.

## 19. Reviewer Checklist

- Plan directly targets `STORAGE-002` cross-Project and cross-user mount-plan
  isolation.
- Scope is limited to two focused resolver tests plus three tracker updates
  after approval.
- No runtime code, fixtures, deployment files, database migrations, or
  configuration changes are planned.
- Cross-Project test asserts `404 storage binding not found` when only another
  Project has the binding.
- Cross-user test asserts `403 storage permission denied` when only another
  user has the grant.
- Existing `STORAGE-001` positive proof is reused as control instead of
  duplicated.
- Tracker updates do not claim live Kubernetes PVC isolation, namespace
  enforcement, CSI behavior, `STORAGE-003`, storage GA, Full GA, or
  first-version readiness.
- Verification includes focused storage tests, full storage tests, storage plus
  workload package tests, `git diff --check`, backend check, and Sonar Quality
  Gate.

## 20. Status

Status: Approved
