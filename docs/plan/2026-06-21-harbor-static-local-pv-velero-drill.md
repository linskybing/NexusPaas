# Harbor Static Local PV Velero Drill

Status: Approved

## 1. Objective

Unblock OPS-007 by moving the live Harbor foundation from Rancher `local-path` hostPath PVCs to
Kubernetes static `local` PersistentVolumes, then rerun the Velero Harbor backup/restore drill with
verified non-Redis PVC data backup, namespace deletion, restore, and ORAS artifact verification.

## 2. Background

The first OPS-007 Velero attempt installed Velero successfully but stopped before deleting
`harbor-system` because Velero file-system backup skipped Harbor PVC data. The reason was storage
type, not Harbor or Velero readiness: Rancher `local-path` presents the mounted PVCs as `hostPath`,
and Velero file-system backup does not support `hostPath`.

Official Velero file-system backup docs state that `hostPath` volumes are unsupported but the
Kubernetes `local` volume type is supported. Kubernetes local PersistentVolumes are static
pre-provisioned volumes with node affinity. This plan uses that standard Kubernetes storage type as
a local, non-HA live-drill bridge. It does not claim off-cluster DR or production-grade storage HA.

## 3. Source References

- `docs/acceptance/operations.md`
- `docs/plan/2026-06-21-harbor-velero-backup-restore-drill.md`
- `docs/plan/2026-06-21-harbor-foundation-credential-rebaseline.md`
- Velero file-system backup docs: https://velero.io/docs/main/file-system-backup/
- Kubernetes PersistentVolume docs: https://kubernetes.io/docs/concepts/storage/persistent-volumes/
- Local static provisioner reference: https://github.com/kubernetes-sigs/sig-storage-local-static-provisioner

## 4. Assumptions

- Current context is `default`.
- The cluster is single-node (`sky-desktop`), so static local PV node affinity can target that node.
- `/home/lin/.local/share/nexuspaas/harbor-static-local-pv-20260621161910` can be created and
  managed by the current user for local live evidence.
- Harbor is still ClusterIP-only foundation and has no user image workflow dependency.
- The synthetic OPS-007 project from the failed attempt was cleaned up.
- Velero remains installed and healthy with node-agent and an available MinIO BackupStorageLocation.
- Execution stamp for this plan is `20260621161910`.

## 5. Non-Goals

- Do not claim HA storage, off-cluster DR, encrypted backup storage, or production retention.
- Do not install Longhorn, OpenEBS, NFS, or host packages in this slice.
- Do not use Rancher `local-path` for Harbor PVCs after replatform.
- Do not print or commit secret values.
- Do not delete `harbor-system` until a new Velero backup has completed with non-Redis Harbor
  volume data protected.

## 6. Current Behavior

Pre-execution baseline: Harbor and Velero were healthy, but OPS-007 remained open because Harbor
PVCs used Rancher `local-path`, which exposed `hostPath` volumes that could not satisfy Velero
file-system backup volume gates.

Final state after this plan: Harbor is restored and ready on Kubernetes static `local` PV/PVCs,
OPS-007 local Harbor backup/restore drill has passed with non-Redis PodVolumeBackup and
PodVolumeRestore evidence, and the remaining storage caveat is HA/off-cluster DR maturity rather
than the earlier `local-path` blocker.

## 7. Target Behavior

1. A scratch namespace proves Velero can back up and restore a Kubernetes static `local` PV on this
   node; if this proof fails, stop before mutating Harbor. Scratch restore must include a completed
   PodVolumeRestore and payload verification.
2. Harbor is reinstalled with official chart `harbor/harbor --version 1.19.1` / app `2.15.1`,
   using pre-created static local PV/PVC claims for:
   - registry storage
   - jobservice logs
   - database
   - Redis
3. Harbor credentials are regenerated through restricted temp values as before.
4. A new unique Velero BackupStorageLocation bucket is created for the static-local attempt.
5. A seeded Harbor OCI artifact is pushed with ORAS using `--plain-http` and `--password-stdin`.
6. Harbor read-only mode is enabled before backup, Redis volume is excluded per Harbor docs, and a
   Velero Backup reaches `Completed` with zero errors and completed non-Redis PodVolumeBackups for
   database, jobservice, and registry data.
7. Only after the volume gate passes, `harbor-system` is deleted, the exact static Harbor PVs are
   deleted, and the local PV directories are emptied to simulate data loss.
8. Exact static Harbor PVs are recreated from the contract before Velero Restore is allowed to
   wait for workload readiness; because Redis is intentionally excluded from data backup, an empty
   same-name Redis PVC is recreated after restore if Velero does not restore it. Velero Restore
   reaches `Completed` with zero errors and expected PodVolumeRestores complete.
9. Restored Harbor becomes ready, read-only is unset, and ORAS verifies the seeded artifact digest.
10. Ledgers mark OPS-007 complete only if the full restore and artifact verification pass.

## 7.1 Static Local PV Contract

Scratch proof resources:

- Namespace: `ops007-local-pv-proof-20260621161910`
- StorageClass: `nexuspaas-static-local-ops007`
  - provisioner: `kubernetes.io/no-provisioner`
  - volumeBindingMode: `WaitForFirstConsumer`
- PV/PVC: `ops007-local-pv-proof-20260621161910`
- Size: `128Mi`
- Access mode: `ReadWriteOnce`
- Reclaim policy: `Retain`
- Local path:
  `/home/lin/.local/share/nexuspaas/ops007-local-pv-proof-20260621161910/data`
- Node affinity:
  - key: `kubernetes.io/hostname`
  - operator: `In`
  - values: `sky-desktop`

Harbor static local PV resources:

| Purpose | PV/PVC Name | Size | Local Path |
| --- | --- | ---: | --- |
| Registry | `harbor-static-registry-20260621161910` | `1Gi` | `/home/lin/.local/share/nexuspaas/harbor-static-local-pv-20260621161910/registry` |
| Jobservice logs | `harbor-static-jobservice-20260621161910` | `512Mi` | `/home/lin/.local/share/nexuspaas/harbor-static-local-pv-20260621161910/jobservice` |
| Database | `harbor-static-database-20260621161910` | `1Gi` | `/home/lin/.local/share/nexuspaas/harbor-static-local-pv-20260621161910/database` |
| Redis | `harbor-static-redis-20260621161910` | `512Mi` | `/home/lin/.local/share/nexuspaas/harbor-static-local-pv-20260621161910/redis` |

All Harbor static local PVs use:

- StorageClass: `nexuspaas-static-local-ops007`
- Access mode: `ReadWriteOnce`
- Reclaim policy: `Retain`
- Node affinity:
  - key: `kubernetes.io/hostname`
  - operator: `In`
  - values: `sky-desktop`

Directory rules:

- Create directories with `install -d`.
- Set mode `0777` for this local evidence environment so Harbor containers can write and the
  current user can later empty directories during disaster simulation.
- Do not store these directories in the repository.

Harbor Helm `existingClaim` mappings:

- `persistence.persistentVolumeClaim.registry.existingClaim=harbor-static-registry-20260621161910`
- `persistence.persistentVolumeClaim.jobservice.jobLog.existingClaim=harbor-static-jobservice-20260621161910`
- `persistence.persistentVolumeClaim.database.existingClaim=harbor-static-database-20260621161910`
- `persistence.persistentVolumeClaim.redis.existingClaim=harbor-static-redis-20260621161910`

Current Harbor PVCs eligible for deletion only after scratch proof and final preflight pass:

- `data-harbor-redis-0`
- `database-data-harbor-database-0`
- `harbor-jobservice`
- `harbor-registry`

Final Velero storage isolation:

- Bucket: `velero-harbor-static-20260621161910`
- BackupStorageLocation: `ops007-static-20260621161910`
- Backup CR: `ops007-harbor-static-20260621161910`
- Restore CR: `ops007-harbor-static-restore-20260621161910`
- The final Backup CR must explicitly set `storageLocation: ops007-static-20260621161910`.

## 8. Affected Domains

- OPS-007 Harbor backup/restore
- Harbor persistence foundation
- Velero backup/restore evidence
- GA problem/gap ledgers

## 9. Affected Files

- `docs/plan/2026-06-21-harbor-static-local-pv-velero-drill.md`
- `gap.md`
- `problem.md`

## 10. API / Contract Changes

None for NexusPaaS APIs.

## 11. Database / Migration Changes

No NexusPaaS database changes. Harbor internal database data is recreated on new Harbor static
local PV storage, then protected and restored through Velero file-system backup.

## 12. Configuration Changes

Live cluster changes:

- New scratch proof namespace/PV/PVC/pod/backup/restore resources, then cleanup.
- New static local Harbor PV directories under `/home/lin/.local/share/nexuspaas/...`.
- New Harbor static local PV/PVC resources.
- Harbor release reinstall using `existingClaim` values for all Harbor persistence.
- New unique MinIO bucket and Velero BackupStorageLocation for the static-local drill.

No repository runtime config or committed secret files are changed.

## 13. Observability Changes

Evidence will be collected from:

- scratch local-PV backup/restore status
- Helm list/status for Harbor
- Kubernetes Harbor readiness resources
- Velero Backup/Restore/PodVolumeBackup/PodVolumeRestore CRs
- ORAS push/fetch/pull digest output

## 14. Security Considerations

- Temp values files for Harbor credentials must be mode `600` and removed.
- Harbor and MinIO credentials are consumed via `secretKeyRef` or restricted temp files only.
- Do not run `helm get values`, `helm get manifest`, or print Kubernetes Secret `.data`.
- Static local PV directories are a local evidence bridge, not production HA storage.
- Velero backups include restored Harbor Secrets; encrypted/off-cluster backup remains open.

## 15. Implementation Steps

1. Scratch proof:
   - create one static `local` PV directory under `/home/lin/.local/share/nexuspaas/ops007-local-pv-proof-20260621161910`
   - create namespace/PV/PVC and a running pod that mounts the PVC
   - write a known payload into the mounted PVC with `kubectl exec` so restore cannot be
     accidentally proven by a pod startup command
   - create a Velero backup using file-system backup
   - require a completed PodVolumeBackup for the local PV
   - delete namespace/PV and empty the directory
   - recreate the exact static local PV before waiting for restore readiness because
     `kubernetes.io/no-provisioner` does not dynamically create PVs
   - restore and verify completed PodVolumeRestore plus payload
   - clean up scratch resources
2. Harbor replatform preflight:
   - verify Harbor has no `ops007-*` synthetic project
   - verify Harbor has only expected default project state, with no unexpected non-default
     projects, repositories, artifacts, robot accounts, or NexusPaaS image-build integration
     evidence; if any unexpected Harbor data exists, stop before uninstall
   - verify the only current Harbor PVCs eligible for deletion are exactly
     `data-harbor-redis-0`, `database-data-harbor-database-0`, `harbor-jobservice`, and
     `harbor-registry`
   - verify Velero is healthy
   - verify current Harbor is still read-write
3. Uninstall current Harbor and delete exact current `local-path` PVCs after Reviewer-approved
   scratch proof success.
4. Create four static local PV directories and PV/PVC pairs for Harbor.
5. Reinstall Harbor with generated credentials and `existingClaim` mappings.
6. Create unique MinIO bucket/BackupStorageLocation for the final static-local attempt. Hard-stop
   if bucket `velero-harbor-static-20260621161910` exists or is non-empty, if BSL
   `ops007-static-20260621161910` already exists, or if same-stamp Backup/Restore/PVB/PVR
   resources exist.
7. Seed Harbor project/artifact with ORAS.
8. Set read-only true and apply Redis exclusion labels.
9. Create Velero Backup and require non-Redis Harbor volume PVB completion.
10. Delete `harbor-system`, delete exact static PVs, and empty the local PV directories.
11. Recreate the exact Harbor static PVs from section 7.1, restore from Velero, recreate the empty
    Redis PVC if it was excluded from backup, and require PVR completion.
12. Verify Harbor readiness and restored artifact digest; unset read-only and clean synthetic data.
13. Update plan evidence, `gap.md`, and `problem.md`; submit to Reviewer Agent.

## 16. Verification Plan

- Scratch static local PV backup and restore passes, including completed PodVolumeBackup,
  completed PodVolumeRestore, and payload verification.
- Harbor release is deployed and ready after static local PV reinstall.
- Harbor PVCs no longer use Rancher `local-path`; PV specs use Kubernetes `local`.
- Harbor preflight found no unexpected projects, repositories, artifacts, robot accounts, or
  integration data before reinstall.
- Backup completes with zero errors.
- PodVolumeBackups include non-Redis Harbor volumes and complete successfully.
- Restore completes with zero errors.
- PodVolumeRestores for non-Redis Harbor volumes complete successfully.
- Restored Harbor API ping returns `200`.
- ORAS can fetch/pull the seeded digest after restore.
- `git diff --check` passes after documentation updates.
- App tests/build/Sonar remain N/A unless repo manifests or app code are changed.

## 17. Rollback Plan

If scratch local PV proof fails:

1. Clean scratch resources.
2. Do not mutate Harbor.
3. Keep OPS-007 open and record the blocker.

If Harbor reinstall fails:

1. Keep static PV directories for inspection.
2. Reinstall the prior Harbor foundation using the credential rebaseline plan if needed.
3. Keep OPS-007 open.

If backup gate fails:

1. Unset Harbor read-only.
2. Do not delete `harbor-system`.
3. Keep Harbor running and record the blocker.

If restore fails after namespace deletion:

1. Do not delete Velero backup data.
2. If restored PVCs are Pending because static PVs are absent, recreate the exact PVs from section
   7.1 before treating the restore as failed.
3. If Redis remains Pending because its excluded PVC was not restored, recreate the exact Redis PVC
   from section 7.1 as empty cache storage before treating the restore as failed.
4. Attempt one non-destructive restore retry if the error is transient.
5. If still failed, redeploy Harbor foundation and keep OPS-007 open.

## 18. Risks and Tradeoffs

- Static local PV is single-node and not HA.
- Local paths under the user home are acceptable for this live evidence environment but not a
  production storage recommendation.
- Restore runbooks must pre-provision matching static local PVs before workload readiness can
  recover; Kubernetes `local` PVs are not dynamically provisioned.
- Because Redis is intentionally excluded as cache, the restore runbook must recreate its static PVC
  as empty storage so the restored Redis StatefulSet can schedule.
- Reinstalling Harbor is destructive to current Harbor data; current Harbor is foundation-only and
  synthetic OPS-007 data was cleaned up.
- This can close OPS-007 drill mechanics if restore passes, but off-cluster DR and storage maturity
  remain open.

## 19. Reviewer Checklist

- Scratch local PV proof hard-stops before Harbor mutation if unsupported.
- Harbor does not remain on Rancher `local-path`.
- The plan uses Kubernetes `local` PV and Velero FSB, not custom tarball backup.
- `harbor-system` deletion occurs only after non-Redis Harbor PVBs complete.
- Secret values are not printed, hashed, or committed.
- OPS-007 is marked complete only after restore and artifact verification.
- The plan preserves broader DR/storage limitations.

## 20. Status

Status: Approved. Reviewer Agent approved the revised plan before scratch proof, then approved the
scratch-evidence amendment that adds the static-PV restore pre-provisioning requirement before
Harbor replatform. Implementation is complete and pending final Reviewer Agent verification.

## 21. Scratch Local PV Evidence

Execution stamp: `20260621161910`.

- Initial scratch attempt with a completed writer pod was intentionally discarded because Velero
  skipped pod volume backup for the already completed pod.
- Second scratch backup used pod `ops007-local-pv-proof-sleeper-20260621161910`, which only slept;
  payload `ops007-local-pv-proof-20260621161910` was written by `kubectl exec`.
- Backup `ops007-local-pv-proof-20260621161910` completed with `errors=0`, `warnings=0`.
- PodVolumeBackup
  `ops007-local-pv-proof-20260621161910-67nzk` completed for pod
  `ops007-local-pv-proof-sleeper-20260621161910`, volume `data`, `bytesDone=37`,
  `totalBytes=37`.
- After namespace/PV deletion and local path emptying, Restore
  `ops007-local-pv-proof-restore-20260621161910` stayed in progress while the restored PVC was
  Pending because no static PV existed. Recreating PV
  `ops007-local-pv-proof-20260621161910` from the contract bound the PVC.
- Restore `ops007-local-pv-proof-restore-20260621161910` completed with `errors=0`,
  `warnings=1`; the warning was Velero PVR controller nodeOS detection with an empty node name, not
  a data-copy failure.
- PodVolumeRestore
  `ops007-local-pv-proof-restore-20260621161910-5dfgf` completed for pod
  `ops007-local-pv-proof-sleeper-20260621161910`, volume `data`, `bytesDone=37`,
  `totalBytes=37`.
- Restored payload readback returned `ops007-local-pv-proof-20260621161910`.
- Scratch cleanup removed the proof namespace, PV, Backup, Restore, and local-path contents;
  `scratch_namespace_exists=false`, `scratch_pv_exists=false`, `scratch_backup_exists=false`,
  `scratch_restore_exists=false`, and `scratch_path_file_count=0`.

## 22. Harbor Static Local PV Execution Evidence

Execution stamp: `20260621161910`.

Preflight:

- Harbor foundation before replatform was ready on official chart `harbor-1.19.1` / app `2.15.1`.
- Harbor API preflight returned `harbor_read_only=false`, `project_count=1`,
  `project=library`, `repo_count=0`, and `robot_count=0`.
- Current Harbor PVCs were exactly the approved deletion set and all used `local-path`:
  `data-harbor-redis-0`, `database-data-harbor-database-0`, `harbor-jobservice`, and
  `harbor-registry`.
- Final resources did not preexist:
  `harbor_static_bucket_exists=false`, `ops007_static_bsl_exists=false`,
  `ops007_static_backup_exists=false`, and `ops007_static_restore_exists=false`.

Replatform:

- `helm uninstall harbor -n harbor-system --wait --timeout=10m` completed.
- Exact old PVC deletion completed and `old_harbor_pvc_count=0`.
- Static local PVCs
  `harbor-static-registry-20260621161910`,
  `harbor-static-jobservice-20260621161910`,
  `harbor-static-database-20260621161910`, and
  `harbor-static-redis-20260621161910` all reached `Bound` on StorageClass
  `nexuspaas-static-local-ops007`; backing PVs use `Retain`.
- Harbor was reinstalled with chart `harbor-1.19.1` / app `2.15.1` and generated credentials via a
  mode-`600` temp values file; `temp_values_removed=true`.
- Restored static Harbor foundation readiness passed:
  `harbor-core`, `harbor-jobservice`, `harbor-nginx`, `harbor-portal`, `harbor-registry`,
  `harbor-database`, and `harbor-redis` all reached `1/1`.
- In-cluster Harbor API ping after static reinstall returned `harbor_ping_status=200`.
- Dedicated bucket `velero-harbor-static-20260621161910` was created through a MinIO client pod
  using `secretKeyRef`; BackupStorageLocation `ops007-static-20260621161910` reached `Available`.

Seed and backup:

- Synthetic Harbor project `ops007-static-20260621161910` was created with
  `project_create_status=201`.
- ORAS `v1.3.0` pushed and resolved artifact
  `ops007-static-20260621161910/payload:20260621161910` over plain HTTP using
  `--password-stdin`; baseline digest:
  `sha256:c7837e0a80dc7266b26eb197901b3ce8c3b893dc5ecb70208d13aab58dc70c46`.
- Harbor read-only was set before backup: `readonly_set_status=200`, `read_only.value=true`.
- Redis exclusion labels were applied to pod, PVC, and PV:
  `redis_pod_exclude=true`, `redis_pvc_exclude=true`, `redis_pv_exclude=true`.
- Backup `ops007-harbor-static-20260621161910` completed with `errors=0`, `warnings=0`, and
  `storageLocation=ops007-static-20260621161910`.
- Completed PodVolumeBackups:
  - `harbor-database-0` / `database-data`: `51369080/51369080`
  - `harbor-registry-7bfb6dc49b-gqwz9` / `registry-data`: `862/862`
  - `harbor-jobservice-6ff6795f95-fbmwv` / `job-logs`: `0/0`
  - additional ephemeral volumes `shm-volume` and `psc` completed.

Disaster simulation and restore:

- `harbor-system` namespace was deleted, exact static Harbor PVs were deleted, and all four local
  data directories were emptied:
  `path_count_registry=0`, `path_count_jobservice=0`, `path_count_database=0`,
  `path_count_redis=0`.
- Exact static Harbor PVs were recreated before restore; all were initially `Available`.
- Restore `ops007-harbor-static-restore-20260621161910` completed with `errors=0`,
  `warnings=1`. The warning was Velero PVR nodeOS detection with an empty node name, not a data
  copy failure.
- Completed PodVolumeRestores:
  - `harbor-database-0` / `database-data`: `51369080/51369080`
  - `harbor-registry-7bfb6dc49b-gqwz9` / `registry-data`: `862/862`
  - `harbor-jobservice-6ff6795f95-fbmwv` / `job-logs`: `0/0`
  - additional ephemeral volumes `shm-volume` and `psc` completed.
- Redis PVC was not restored because it was intentionally excluded. The runbook recreated empty PVC
  `harbor-static-redis-20260621161910`, which bound to the precreated Redis PV and allowed
  `harbor-redis` to schedule.
- Final Harbor readiness passed with all five Deployments and both StatefulSets `1/1`; all four
  static PVCs were `Bound` and all four static PVs were `Bound` with reclaim policy `Retain`.

Post-restore verification and cleanup:

- Restored API ping returned `final_ping_status=200`.
- Harbor read-only was unset: `readonly_unset_status=200`, `read_only.value=false`.
- ORAS restored digest verification returned
  `restored_resolved_digest=sha256:c7837e0a80dc7266b26eb197901b3ce8c3b893dc5ecb70208d13aab58dc70c46`
  and `restored_payload_ok=true`.
- Synthetic data cleanup returned `repository_delete_status=200`,
  `project_delete_status=200`, and `synthetic_project_present=false`.
- Final Harbor API state returned `final_ping_status=200`, `read_only.value=false`, and only
  `project=library`.
- Dedicated backup bucket currently has `bucket_object_count=49`; the Velero backup is retained
  under its `24h0m0s` TTL for evidence.
- No temporary pods from this drill remained in `harbor-system`, `nexuspaas`, or `default`.
