# Storage Mount-Plan Authorization Proof

Status: Approved

## 1. Objective

Add a narrow local, no-Docker proof for `STORAGE-001` showing that storage
mount-plan resolution authorizes requested workload mounts only through
storage-owned project bindings, dispatch-ready group storage sources, and the
effective user permission for that PVC.

This is a test and evidence slice. It does not change storage runtime behavior,
workload dispatch behavior, Kubernetes PVC handling, deployment manifests, or
live cluster execution.

## 2. Background

The storage service already exposes an internal service-auth mount-plan contract:
`POST /internal/storage/projects/{project_id}/mount-plan`. CodeGraph source
review shows that successful resolution is based on storage-owned records rather
than request-supplied PVC source details.

Existing tracker wording still treats storage as a v1 gap family. `STORAGE-004`
audit evidence exists, but broader storage isolation and mount validation are
not fully proven. The next useful first-version slice is a focused in-memory
authorization proof for the existing mount-plan resolver.

## 3. Source References

- `AGENTS.md`
- `docs/agents/workflow.md`
- `docs/agents/planning.md`
- `docs/agents/coding-guidelines.md`
- `docs/agents/review-checklist.md`
- `docs/acceptance/gap-analysis.md`
- `gap.md`
- `problem.md`
- `backend/internal/services/storage/mount_plan_contracts.go`
- `backend/internal/services/storage/storage_repository.go`
- `backend/internal/services/storage/helpers.go`
- `backend/internal/services/workload/storage_mount_client.go`
- `docs/plan/2026-06-20-storage-mount-plan-audit.md`
- CodeGraph exploration for `resolveStorageMountPlan`,
  `storageMountPlanRequestFromPayload`, `EffectiveStoragePermission`,
  `groupStorageID`, and `projectBindingID`

## 4. Assumptions

- `storage-service` owns group storage, project storage bindings, storage
  permissions, storage access policies, and project storage permissions.
- Local in-memory tests are sufficient for this `STORAGE-001` proof because the
  requirement is authorization decision behavior, not live PVC mounting.
- The Code Agent may call package-local helpers directly from storage package
  tests instead of creating service-auth HTTP tests.
- Records can be seeded directly into `app.Store` using existing storage
  resource constants and ID helpers.
- `EffectiveStoragePermission` precedence is project permission first, then
  group permission, then storage policy default, then `none`.
- `stopped` and `deleted` group storage sources are not dispatch-ready.
- Tracker updates after implementation must describe only local/in-memory
  mount-plan authorization evidence.

## 5. Non-Goals

- No runtime behavior changes in storage, workload, platform, or shared helpers.
- No new abstractions, dependencies, repository interfaces, or service split.
- No live Kubernetes, RKE2, PVC provisioning, CSI, namespace, or mount execution
  test.
- No Docker or docker-compose usage.
- No service-auth HTTP integration test unless the Code Agent finds direct
  helper tests cannot cover the requirement.
- No OpenAPI, route registration, middleware, deployment, Secret, config,
  database migration, or fixture changes.
- No frontend or workload scheduler behavior changes.
- No claim that storage isolation is complete in a live cluster.
- No claim of full storage GA, Full GA, or first-version readiness.

## 6. Current Behavior

`resolveStorageMountPlanContract` decodes the internal service-auth request,
validates the payload, calls `resolveStorageMountPlan`, publishes
`StorageMountPlanResolved` on success, and returns HTTP `200` with the plan.

`storageMountPlanRequestFromPayload` requires the project ID from the path and a
user ID from the payload. It accepts namespace aliases, several mount list keys,
and PVC selector aliases including `pvc_id`, `pvcId`, `pvc_name`, `pvcName`,
`claim_name`, `claimName`, `target_pvc`, `targetPVC`, `source_pvc`,
`sourcePVC`, `pvc`, and `PVC`. Each mount requires one PVC selector.

`resolveStorageMountPlan` currently checks each requested mount by looking up:

- `FindProjectStorageBinding(projectID, pvcID)`, otherwise `404 storage binding
  not found`.
- `FindGroupStorageSource(groupID, pvcID)`, otherwise `404 group storage source
  not found`.
- dispatch-ready source status, otherwise `409 group storage source is not
  dispatch-ready`.
- `EffectiveStoragePermission(projectID, groupID, pvcID, userID)`, with
  read-only mounts allowing `read_only` or `read_write`, writable mounts
  requiring `read_write`, otherwise `403 storage permission denied`.

On success, the resolver returns manifest mounts using the target claim and PVC
share operations using the source namespace/PVC plus target PVC from
storage-owned records.

There is not yet a focused mount-plan authorization test matrix proving the
success and denial cases for `STORAGE-001`.

## 7. Target Behavior

Focused storage package tests prove that a requested workload mount resolves
only when all of these are true:

- the project has a storage binding for the requested PVC;
- the bound group has a matching group storage source;
- the group storage source is dispatch-ready;
- the effective user permission allows the requested read/write mode.

The success test must assert the returned plan contains exactly the expected
manifest mount and PVC share operation from storage-owned source namespace,
source PVC, and target PVC records.

Denial tests must prove unbound PVCs, non-dispatch-ready sources, and
insufficient permissions fail with the expected HTTP status codes. One
precedence test must prove project-level permission precedence over group or
policy permission for writable mount decisions.

## 8. Affected Domains

- `storage-service`: in-memory authorization proof for mount-plan resolution.
- `workload-service`: consumer contract regression only through package tests;
  no workload code changes are expected.
- Acceptance tracking docs: narrow evidence update for local STORAGE-001
  mount-plan authorization proof.

No deployable service boundary or data ownership boundary changes are proposed.

## 9. Affected Files

Code Agent may touch only these implementation files for this slice:

- `backend/internal/services/storage/mount_plan_contracts_test.go`
- `gap.md`
- `problem.md`
- `docs/acceptance/gap-analysis.md`
- `docs/plan/2026-06-23-storage-mount-plan-authorization-proof.md`

Do not modify:

- `backend/internal/services/storage/mount_plan_contracts.go`
- `backend/internal/services/storage/storage_repository.go`
- `backend/internal/services/storage/helpers.go`
- `backend/internal/services/workload/storage_mount_client.go`
- runtime handlers, route specs, middleware, platform store code, deployment
  files, fixtures, frontend files, or Kubernetes manifests.

## 10. API / Contract Changes

No runtime API or service contract changes.

The existing internal mount-plan response contract remains:

- `project_id`
- `user_id`
- `namespace`
- `manifest_mounts`
- `pvc_share_operations`

Tests may add local helper data that models these storage-owned records:

- group storage source with group ID, PVC ID/name, namespace/source namespace,
  source PVC/name, and status;
- project storage binding with project ID, group ID, PVC ID, and target PVC;
- project storage permission, group storage permission, or storage access policy
  records with read-only or read-write permission values.

## 11. Database / Migration Changes

None.

All proof data must use `platform.NewApp(platform.Config{...})` with the
in-memory store. No schema, migration, seed, fixture, or persistent data changes
are allowed.

## 12. Configuration Changes

None.

Tests should use minimal local `platform.Config` values needed to construct the
app and storage service context. Do not add environment variables or deployment
configuration.

## 13. Observability Changes

None.

This slice does not change logs, metrics, traces, audit events, or domain event
payloads. Existing `StorageMountPlanResolved` behavior remains unchanged.

## 14. Security Considerations

- Tests must prove request-supplied PVC selectors do not bypass project binding
  or group source lookup.
- Tests must prove writeable mounts cannot be resolved with only `read_only` or
  default read-only access.
- Permission precedence must be explicit so a broader group or policy grant does
  not accidentally override a restrictive project-level permission, or so a
  project-level read-write grant is proven to override narrower lower-precedence
  permissions. Choose one clear assertion and name it accordingly.
- Test data must be synthetic and must not include real tenant IDs, credentials,
  Kubernetes tokens, service keys, or secret values.
- Error assertions should check status and stable error text only; no runtime
  error shape changes are allowed.

## 15. Implementation Steps

1. Add or extend `backend/internal/services/storage/mount_plan_contracts_test.go`
   in package `storage`.
2. Add small test helpers only if they remove repeated setup in this one test
   file, for example:
   - create a local in-memory `platform.App`;
   - create a request with context for direct resolver calls;
   - seed a group storage source record;
   - seed a project storage binding record;
   - seed project, group, or policy permission records.
3. Seed storage records directly through `app.Store` using existing resource
   constants and ID helpers from `storage_repository.go` and `helpers.go`.
4. Add
   `TestResolveStorageMountPlanAllowsBoundReadyReadWriteProjectPermission`.
   It must seed:
   - one project binding for the requested PVC;
   - one dispatch-ready group storage source for the same PVC;
   - one project-level `read_write` permission for the user.
   It must request one writable mount and assert:
   - status `200`;
   - one manifest mount;
   - one PVC share operation;
   - manifest claim name is the storage-owned target PVC;
   - share operation source namespace and source PVC come from the group source;
   - share operation target PVC comes from the project binding.
5. Add `TestResolveStorageMountPlanDeniesUnboundPVC`. It must omit the project
   binding for the requested PVC and assert status `404` plus `storage binding
   not found`.
6. Add `TestResolveStorageMountPlanDeniesStoppedOrDeletedSource`. It must cover
   `stopped` and `deleted` group source statuses, preferably table-driven, and
   assert status `409` plus `group storage source is not dispatch-ready`.
7. Add `TestResolveStorageMountPlanDeniesWritableMountWithReadOnlyPermission`.
   It must seed an otherwise valid binding/source and only `read_only`
   permission or policy access, request a writable mount, and assert status
   `403` plus `storage permission denied`.
8. In the same read-only permission test, add one bounded assertion that a
   read-only mount with read-only permission succeeds if it remains concise.
   Split it into a separate test only if the table becomes hard to read.
9. Add one precedence test. Prefer
   `TestResolveStorageMountPlanProjectPermissionOverridesGroupReadWriteToDenyWritableMount`:
   seed group `read_write` permission plus project `read_only` permission for
   the same project/PVC/user, request a writable mount, and assert status `403`.
   If Code Agent finds the opposite assertion clearer, use project `read_write`
   over group/policy `read_only` and assert status `200`; keep only one
   precedence test.
10. Optionally add
    `TestStorageMountPlanRequestFromPayloadAcceptsDocumentedAliases` if it stays
    small. It should cover the documented mount list and PVC selector aliases
    without expanding into broad request parsing coverage.
11. Do not add HTTP service-auth tests unless direct package-local tests cannot
    reach the required behavior.
12. Do not modify production code to make tests pass. If tests expose a behavior
    mismatch, stop and return the mismatch to Reviewer Agent before expanding
    scope.
13. After focused tests pass, update `gap.md`, `problem.md`, and
    `docs/acceptance/gap-analysis.md` with narrow wording only:
    local/in-memory `STORAGE-001` mount-plan authorization proof exists for
    project binding, dispatch-ready group source, effective permission, and
    permission precedence.
14. Keep all broader storage gaps open, including live Kubernetes PVC isolation,
    live mount execution, cluster namespace enforcement, CSI behavior, full
    storage GA, Full GA, and first-version readiness.

## 16. Verification Plan

Run from the repository root unless the command changes into `backend`:

```bash
cd backend && go test ./internal/services/storage -run 'TestResolveStorageMountPlan|TestStorageMountPlanRequestFromPayload' -count=1
cd backend && go test ./internal/services/storage -count=1
cd backend && go test ./internal/services/storage ./internal/services/workload -count=1
git diff --check
make -C backend check
make -C backend ci-sonar
```

Expected results:

- Focused mount-plan authorization tests pass.
- Full storage package tests pass.
- Storage plus workload package tests pass, confirming the consumer package is
  not regressed.
- `git diff --check` reports no whitespace errors.
- `make -C backend check` passes.
- `make -C backend ci-sonar` passes and the SonarScanner Quality Gate is
  green.

## 17. Rollback Plan

Revert only this slice's files:

- remove the new or changed mount-plan authorization tests from
  `backend/internal/services/storage/mount_plan_contracts_test.go`;
- revert the narrow tracker edits in `gap.md`, `problem.md`, and
  `docs/acceptance/gap-analysis.md`;
- restore this plan document to its prior reviewed status if needed.

No database, migration, runtime config, Kubernetes, Docker, deployment, or
persistent data rollback is required.

## 18. Risks and Tradeoffs

- This proves local resolver authorization behavior, not live Kubernetes PVC
  isolation or actual mount execution.
- Direct package-local tests are intentionally cheaper than service-auth HTTP
  tests, but they do not prove middleware behavior. That is acceptable for this
  `STORAGE-001` slice because the target is resolver authorization.
- In-memory store setup can accidentally diverge from realistic records. Keep
  seeded keys aligned with existing storage resource constants and ID helpers.
- Over-testing parser aliases could dilute the authorization proof. Keep parser
  coverage to one concise test or omit it.
- Tracker wording can overstate readiness. The docs must say local/in-memory
  mount-plan authorization proof only.

## 19. Reviewer Checklist

- Plan remains limited to local/no-Docker `STORAGE-001` authorization evidence.
- Only test and tracker files are planned for implementation; no runtime code
  changes are allowed.
- Success case proves storage-owned source namespace, source PVC, target PVC,
  target claim, and mount path behavior.
- Denial cases cover unbound PVC, stopped/deleted group source, and writable
  mount with insufficient permission.
- Permission precedence is covered by one clear project-vs-group/policy
  assertion.
- Any parser alias coverage is small and does not broaden into HTTP
  service-auth testing.
- Tracker updates remain narrow and do not claim live Kubernetes mount
  execution, cluster PVC isolation, storage GA, Full GA, or first-version
  readiness.
- Verification includes focused storage tests, full storage tests, storage plus
  workload package tests, `git diff --check`, backend check, and Sonar Quality
  Gate.

## 20. Status

Status: Approved
