# Storage HPC Profile Manifest Drift Gate

## 1. Objective

Close the next minimal PR6 risk after commits `adfab18` and `ae7070a`: add a
repo-local test that prevents seeded `storage-service` `StorageProfile`
defaults from drifting away from `backend/deploy/hpc/storage/*.yaml`
StorageClass manifests.

The gate must be static and lightweight. It must not require live Kubernetes,
kind, `kubectl`, CSI provisioners, local PV binding, or any byte mover.

## 2. Background

PR1 seeded real storage profiles in `storage-service`; PR6 added matching HPC
StorageClass manifests under `backend/deploy/hpc/storage/`. The remaining risk
is simple drift: a default profile can be renamed or its `storage_class_name`
changed without updating the manifest, or a manifest can lose the label that
ties it back to the profile.

The user allows ultra-light kind for E2E work, but this slice does not need it.
The current machine has `kubectl` and Docker but no `kind` binary, and
`kubectl apply --dry-run=client --validate=false -f backend/deploy/hpc/storage/`
still performs API discovery and fails without an API server. A live API dry-run
belongs in a later kind-backed slice.

## 3. Source References

- `backend/internal/services/storage/storage_profiles.go`
- `backend/deploy/hpc/storage/local-nvme-storageclass.yaml`
- `backend/deploy/hpc/storage/cephfs-rwx-authority.yaml`
- `backend/deploy/hpc/storage/longhorn-rwx-standard.yaml`
- `backend/deploy/hpc/storage/README.md`
- `docs/acceptance/gap-analysis.md`
- `problem.md`

## 4. Assumptions

- The seeded defaults remain the source of truth for built-in storage profile
  IDs and `storage_class_name` values.
- Object profiles, currently `minio-artifact`, do not need a Kubernetes
  StorageClass manifest.
- All non-object seeded profiles with a non-empty `storage_class_name` must have
  exactly one matching StorageClass manifest in `backend/deploy/hpc/storage/`.
- `k8s.io/apimachinery/pkg/util/yaml` is already available through existing
  Kubernetes dependencies; no new dependency should be needed.

## 5. Non-Goals

- No live kind cluster.
- No `kubectl` invocation in tests.
- No live Kubernetes API server, discovery, admission, or dry-run dependency.
- No CSI, Rook, Longhorn, local PV, StorageClass binding, or scheduler proof.
- No byte mover, rsync/rclone/tar job, or data copy assertion.
- No runtime API, route, event, table, migration, or service behavior change.
- No Full GA claim.

## 6. Current Behavior

`storage-service` seeds four built-in profiles:

- `longhorn-rwx-standard`
- `cephfs-rwx-authority`
- `local-nvme-scratch`
- `minio-artifact`

Three matching StorageClass manifests exist for the non-object profiles. There
is no automated gate proving that the seeded profiles and YAML manifests stay in
sync.

## 7. Target Behavior

`go test` in the storage service package fails if:

- a non-object seeded profile has `storage_class_name` but no matching
  StorageClass manifest;
- a matching manifest is not `apiVersion: storage.k8s.io/v1`;
- a matching manifest is not `kind: StorageClass`;
- `metadata.name` does not equal the seeded `storage_class_name`;
- `metadata.labels["nexuspaas.io/storage-profile"]` does not equal the seeded
  profile ID.

The test must explicitly allow `minio-artifact` / object profiles to have no
StorageClass manifest.

## 8. Affected Domains

- `storage-service`: default profile and deploy manifest consistency.
- `deploy/hpc/storage`: static Kubernetes manifest hygiene.
- acceptance/problem ledgers: evidence wording only.

## 9. Affected Files

Implementation should touch only:

- `backend/internal/services/storage/storage_profiles_manifest_test.go`
- `docs/acceptance/gap-analysis.md`
- `problem.md`

Optional, only if wording is needed:

- `backend/deploy/hpc/storage/README.md`

Plan artifact:

- `docs/plan/2026-06-28-storage-hpc-profile-manifest-drift-gate.md`

## 10. API / Contract Changes

None.

No route, event, owner-read, API fixture, or command fixture changes are needed.

## 11. Database / Migration Changes

None.

The test reads seeded profile records from the in-memory platform store.

## 12. Configuration Changes

None.

Do not add kind, kubectl, cluster URL, kubeconfig, or environment-variable
requirements for this slice.

## 13. Observability Changes

None.

Documentation may record this as static drift-gate evidence only.

## 14. Security Considerations

This slice does not change runtime authorization or data access. The test helps
avoid deployment drift where a profile could point workloads to an unintended or
unreviewed StorageClass.

## 15. Implementation Steps

1. Add `backend/internal/services/storage/storage_profiles_manifest_test.go`.
2. In the test, create an in-memory `platform.App` configured for
   `storage-service`, register storage service code, and rely on startup seeding
   to populate default profiles.
3. Read seeded profiles from `storage-service:storage_profiles`.
4. Parse `backend/deploy/hpc/storage/*.yaml` with
   `k8s.io/apimachinery/pkg/util/yaml` into standard maps or
   `unstructured.Unstructured`.
5. Build a map of StorageClass manifests by `metadata.name`.
6. For each seeded profile:
   - skip profiles whose `access_mode` is `object` or whose provider/tier marks
     an object-artifact profile;
   - skip profiles with no `storage_class_name`;
   - require a matching manifest;
   - assert `apiVersion`, `kind`, `metadata.name`, and
     `metadata.labels["nexuspaas.io/storage-profile"]`.
7. Add a direct assertion that `minio-artifact` is accepted without a
   StorageClass manifest.
8. Update `docs/acceptance/gap-analysis.md` and `problem.md` after the test
   passes. Wording must say this is local/static profile-to-manifest drift-gate
   evidence only.
9. Update `backend/deploy/hpc/storage/README.md` only if a short note is needed
   to clarify that `kubectl --dry-run=client` still needs API discovery and is
   reserved for a later kind/live-API slice.

## 16. Verification Plan

Required targeted verification:

```bash
cd backend && go test ./internal/services/storage/... -run StorageProfiles.*Manifest
```

Required final gates:

```bash
cd backend && go test ./internal/services/storage/... ./internal/services/workload/...
cd backend && go test ./...
cd backend && go build ./...
git diff --check
cd backend && make coverage
cd backend && make ci-sonar
```

Do not run kind or kubectl for this slice.

## 17. Rollback Plan

- Delete `backend/internal/services/storage/storage_profiles_manifest_test.go`.
- Revert only the static drift-gate wording in
  `docs/acceptance/gap-analysis.md` and `problem.md`.
- Revert the optional README note if it was added.
- No database, manifest, cluster, or external-service rollback is needed.

## 18. Risks and Tradeoffs

- Static YAML validation cannot prove live cluster behavior. Keep docs precise.
- The test intentionally checks only seeded defaults, not admin-created custom
  profiles.
- Path assumptions can be brittle if package working directories change; keep
  the path calculation local and simple.
- This gate catches profile/manifest naming and label drift, not CSI parameter
  correctness.

## 19. Reviewer Checklist

- [ ] Plan file exists under `docs/plan/`.
- [ ] Test is repo-local and does not call `kubectl`, kind, or a Kubernetes API.
- [ ] Test seeds profiles through storage-service startup behavior, not a copied
      fixture list.
- [ ] Non-object seeded profiles with `storage_class_name` require matching
      StorageClass manifests.
- [ ] `minio-artifact` / object profile is explicitly allowed to have no
      StorageClass.
- [ ] Manifest assertions cover `apiVersion`, `kind`, `metadata.name`, and
      `nexuspaas.io/storage-profile`.
- [ ] Docs state static drift-gate evidence only and avoid live cluster claims.
- [ ] Required test/build/coverage/Sonar commands ran and results are reported.

## 20. Status

Status: Ready for Reviewer Agent
