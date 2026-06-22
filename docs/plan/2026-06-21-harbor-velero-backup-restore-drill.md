# Harbor Velero Backup Restore Drill

Status: Approved

## 1. Objective

Complete OPS-007 with a live Harbor backup/restore drill using CNCF/cloud-native tooling:
Velero backs up the rebaselined Harbor foundation, the Harbor namespace is deleted to simulate a
disaster, Velero restores it, and a seeded OCI artifact is pulled from the restored Harbor registry.

## 2. Background

`docs/acceptance/operations.md` requires:

- `OPS-007`: Harbor backup and restore drill passes.

Harbor foundation is now live in `harbor-system` with official chart `harbor-1.19.1` / app
`2.15.1` after credential rebaseline. At the start of this plan, there was no Velero installation
in the cluster. The existing MinIO service in `nexuspaas` can act as a local S3-compatible backup
store for a live drill.

Official Harbor guidance for Helm-deployed Harbor uses Velero to back up Kubernetes resources and
PersistentVolume data for Harbor's internal database, registry, jobservice, and Trivy, while
excluding Redis data. The Harbor docs state the backup is crash-consistent, not
application-consistent, and call out Redis/session limitations. The live foundation has Trivy
disabled, so this drill covers the internal database, registry storage, jobservice logs, Kubernetes
resources, and Secrets restored from backup.

## 3. Source References

- `docs/acceptance/operations.md`
- `docs/acceptance/image-build.md`
- `gap.md`
- `problem.md`
- `docs/plan/2026-06-21-harbor-foundation-live-deploy.md`
- `docs/plan/2026-06-21-harbor-foundation-credential-rebaseline.md`
- Harbor backup/restore docs: https://goharbor.io/docs/main/administration/backup-restore/
- Velero MinIO quick-start docs: https://velero.io/docs/main/contributions/minio/
- Velero install customization docs: https://velero.io/docs/main/customize-installation/
- Velero Helm chart metadata from `helm show chart vmware-tanzu/velero`
- Velero AWS plugin compatibility/release notes for Velero `1.18.x`

## 4. Assumptions

- The current Kubernetes context is `default`.
- `harbor-system` contains only the Harbor foundation resources created for GA evidence.
- Harbor remains ClusterIP-only and has no NexusPaaS image build integration or user workload
  dependency yet.
- Existing MinIO in namespace `nexuspaas` is acceptable as an in-cluster S3-compatible backup store
  for this OPS-007 drill.
- Off-cluster backup retention, encrypted backup storage, and full DR are separate open GA gaps.
- The cluster can pull `velero/velero`, `velero/velero-plugin-for-aws`, `minio/mc`,
  `curlimages/curl`, and `ghcr.io/oras-project/oras`.

## 5. Non-Goals

- Do not claim off-cluster DR, PITR, encrypted backup storage, or production retention is complete.
- Do not enable external Harbor ingress, DNS, TLS, OIDC, robot accounts, retention, replication,
  signing, or Trivy scanning.
- Do not integrate NexusPaaS image build flows with Harbor in this slice.
- Do not print MinIO credentials, Harbor admin password, Kubernetes Secret `.data`, decoded values,
  or value hashes.
- Do not hand-roll a custom backup script as the primary evidence path.

## 6. Baseline And Current Behavior

Pre-execution baseline: Harbor was live and ready, but OPS-007 remained open because no Harbor
backup/restore drill had been performed. Velero was not installed and the `velero` namespace did
not exist.

Current safe-stop state after this plan's execution: Velero is installed and healthy, the
BackupStorageLocation is available, Harbor remains live and ready, and OPS-007 remains open because
Rancher `local-path` exposes Harbor PVCs as unsupported `hostPath` volumes for Velero
file-system backup.

## 7. Target Behavior

1. Velero is installed with the official Helm chart:
   - chart: `vmware-tanzu/velero` version `12.0.3`
   - app: `1.18.1`
   - AWS/S3 plugin: `velero/velero-plugin-for-aws:v1.14.1`
   - namespace: `velero`
   - `deployNodeAgent=true`
   - `snapshotsEnabled=false`
   - `configuration.defaultVolumesToFsBackup=true`
2. Velero BackupStorageLocation points to existing MinIO:
   - bucket: `velero-harbor-ops007-<stamp>`
   - endpoint: `http://minio.nexuspaas.svc.cluster.local:9000`
   - provider: `aws`
   - `s3ForcePathStyle=true`
3. A temporary Harbor project/repository/artifact is seeded:
   - project prefix: `ops007-`
   - repository: `evidence`
   - tag: `seed`
   - pushed by ORAS through Harbor's HTTP ClusterIP service using `--plain-http` and admin
     password from existing `harbor-core` Secret via `secretKeyRef` and `--password-stdin`.
4. Harbor is set to repository read-only mode through the Harbor configuration API before backup.
5. Redis pod/PVC/PV are labeled `velero.io/exclude-from-backup=true` per Harbor docs.
6. A Velero Backup CR backs up namespace `harbor-system` with file-system backup enabled and reaches
   phase `Completed` with no reported errors.
7. Disaster simulation deletes namespace `harbor-system` after backup completes.
8. A Velero Restore CR restores the backup and reaches phase `Completed` with no reported errors.
9. Restored Harbor becomes ready again, in-cluster `/api/v2.0/ping` returns `200`, read-only is
   unset, and ORAS can pull or fetch the seeded artifact from restored Harbor using `--plain-http`.
10. Synthetic Harbor project/artifact and temporary probe pods are cleaned up after verification.
11. Ledgers mark OPS-007 Harbor backup/restore drill passed, while off-cluster DR and remaining
    Harbor image workflow/scan gaps stay open.

## 8. Affected Domains

- OPS-007 Harbor backup/restore
- Velero backup infrastructure
- Harbor live foundation
- GA problem and gap tracking

## 9. Affected Files

- `docs/plan/2026-06-21-harbor-velero-backup-restore-drill.md`
- `gap.md`
- `problem.md`

## 10. API / Contract Changes

None for NexusPaaS APIs.

## 11. Database / Migration Changes

No NexusPaaS database changes. Harbor's internal PostgreSQL PVC is backed up and restored by Velero
file-system backup as part of the Harbor namespace.

## 12. Configuration Changes

Live cluster changes:

- New `velero` namespace, Velero CRDs, Velero deployment, node-agent DaemonSet, ServiceAccount/RBAC,
  BackupStorageLocation, and backup repository resources.
- New MinIO bucket `velero-harbor-ops007-<stamp>`.
- Harbor Redis pod/PVC/PV receives the Velero exclude label.
- Temporary Harbor read-only configuration is set before backup and unset after restore.

No repository runtime configuration or committed secret files are changed.

## 13. Observability Changes

Evidence will be collected from:

- `helm list -n velero`
- `kubectl -n velero get deploy,daemonset,pod,backupstoragelocation`
- `kubectl -n velero get backup,restore,podvolumebackup,podvolumerestore`
- `kubectl -n harbor-system get deploy,statefulset,pod,svc,pvc`
- Harbor API status-only probes
- ORAS artifact push/pull digest output

## 14. Security Considerations

- MinIO and Harbor credentials are consumed through Kubernetes `secretKeyRef` or restricted
  temporary Helm values only; no secret value output is allowed.
- The Velero credentials values file must use `mktemp`, mode `600`, and trap cleanup.
- Do not run `helm get values`, `helm get manifest`, or Kubernetes commands that print Secret
  `.data`.
- Velero backups include Kubernetes Secrets required to restore Harbor. This is acceptable for the
  drill but not sufficient for GA encrypted/off-cluster backup maturity.
- Temporary pods that mount credentials must be deleted after completion.
- Backup object storage is in-cluster MinIO, so this does not prove off-cluster disaster recovery.

## 15. Implementation Steps

1. Confirm preflight:
   - context `default`
   - Harbor release ready in `harbor-system`
   - no existing `velero` namespace, Velero release, Velero CRDs, BackupStorageLocation, or
     node-agent resources; if any existing Velero install is found, stop and return to Reviewer
     Agent with non-secret evidence before mutating it
   - MinIO service and `minio-credentials` key names exist without printing values
2. Create unique MinIO bucket `velero-harbor-ops007-<stamp>` with a temporary `minio/mc` pod that
   uses `secretKeyRef` to existing MinIO credentials. If the bucket already exists or contains
   objects, stop and return to Reviewer Agent before reusing it.
3. Install Velero with a restricted temporary values file:
   - chart `vmware-tanzu/velero --version 12.0.3`
   - app `1.18.1`
   - AWS plugin `velero/velero-plugin-for-aws:v1.14.1`
   - BSL configured for MinIO path-style S3
   - node-agent enabled and snapshots disabled
4. Wait for Velero deployment, node-agent, and BackupStorageLocation readiness.
5. Seed Harbor:
   - create project `ops007-<stamp>` through a temporary curl pod using
     `harbor-core` `HARBOR_ADMIN_PASSWORD` via `secretKeyRef`
   - push a tiny OCI artifact to `harbor/ops007-<stamp>/evidence:seed` with a temporary ORAS pod
     using `oras login --plain-http --username admin --password-stdin harbor`,
     `oras push --plain-http ...`, and no password in command arguments
   - verify the artifact digest before backup
6. Set Harbor read-only through the configuration API and verify status code only.
7. Label Redis pod/PVC/PV for Velero backup exclusion.
8. Create Backup CR `ops007-harbor-<stamp>` for namespace `harbor-system` with
   `defaultVolumesToFsBackup=true` and `snapshotVolumes=false`.
9. Wait for Backup phase `Completed`; require zero reported backup errors and collect
   PodVolumeBackup status for non-Redis Harbor PVC volume data. Redis volume must be excluded.
10. Delete namespace `harbor-system` to simulate disaster.
11. Create Restore CR `ops007-harbor-restore-<stamp>` from the backup.
12. Wait for Restore phase `Completed`; require zero reported restore errors and collect
   PodVolumeRestore status for non-Redis Harbor PVC volume data.
13. Wait for restored Harbor readiness, then unset read-only.
14. Verify restored artifact via `oras pull --plain-http` or `oras manifest fetch --plain-http`
   and record digest/count evidence.
15. Clean up temporary pods and synthetic Harbor project after verification.
16. Update this plan, `gap.md`, and `problem.md`; submit to Reviewer Agent.

## 16. Verification Plan

- Velero Helm release is deployed with chart `12.0.3` / app `1.18.1`.
- BackupStorageLocation is available.
- Node-agent DaemonSet is ready.
- Seeded Harbor artifact push succeeds before backup using ORAS `--plain-http` and
  `--password-stdin`.
- Backup CR reaches `Completed` with zero reported errors.
- Expected non-Redis Harbor PodVolumeBackup records complete successfully, and Redis volume is
  excluded.
- `harbor-system` namespace deletion succeeds after backup.
- Restore CR reaches `Completed` with zero reported errors.
- Expected non-Redis Harbor PodVolumeRestore records complete successfully.
- Restored Harbor deploys/sts become ready.
- Restored Harbor API ping returns `200`.
- ORAS can pull/fetch the seeded artifact after restore using `--plain-http`.
- Temporary credential-bearing pods are deleted.
- `git diff --check` passes after documentation updates.
- Application tests/build/Sonar are not applicable unless this slice expands into app code or
  repository manifests.

## 17. Rollback Plan

If Velero install fails:

1. Capture non-secret Helm/Kubernetes diagnostics.
2. Delete restricted temp values file.
3. If no backup/restore started, uninstall Velero only after Reviewer Agent approval or leave it for
   inspection.

If backup fails before namespace deletion:

1. Unset Harbor read-only.
2. Delete temporary probe pods.
3. Keep Harbor running and mark OPS-007 still open.

If restore fails after namespace deletion:

1. Do not delete Velero backup data.
2. Re-run restore once if failure is clearly transient and non-destructive.
3. If restore remains failed, use the existing Helm foundation plan to redeploy Harbor and mark
   OPS-007 failed/open with evidence.

## 18. Risks and Tradeoffs

- This slice deliberately deletes `harbor-system` after a successful backup. The namespace currently
  hosts only the just-created Harbor foundation and is not integrated with NexusPaaS workflows.
- In-cluster MinIO proves the mechanics of backup/restore but not off-cluster survivability.
- File-system backup is portable for local-path storage but may be slower than CSI snapshots.
- Harbor's Redis data is excluded per official guidance; sessions/tasks may be lost after restore.
- Velero backups include restored Harbor Secrets; encrypted/off-cluster backup policy remains open.

## 19. Reviewer Checklist

- The plan uses Velero/MinIO/ORAS instead of custom backup code.
- Velero chart/plugin versions are pinned.
- The plan performs a true restore after deleting `harbor-system`.
- The plan verifies a real seeded OCI artifact after restore with ORAS `--plain-http`.
- Backup and restore success gates include completed non-Redis PodVolumeBackup/Restore records and
  zero reported Velero errors.
- Existing Velero resources or non-empty/reused backup bucket conflicts hard-stop before mutation.
- Redis exclusion follows Harbor documentation.
- Secret values are never printed, hashed, or committed.
- OPS-007 can be marked complete only if backup and restore both complete and artifact verification
  passes.
- Off-cluster DR, encrypted backup storage, Harbor scan workflows, and image build integration
  remain open.

## 20. Status

Status: Approved. Reviewer Agent approved revision 1 before installing Velero or deleting
`harbor-system`; execution safely stopped before namespace deletion because the current
`local-path` StorageClass exposes Harbor PVCs as `hostPath`, and Velero file-system backup skips
hostPath volumes.

## 21. Implementation Evidence

Execution stamp: `20260621161910`.

Completed safely:

- Preflight context: `default`.
- No existing Velero namespace, release, or Velero CRDs were present before install.
- Harbor was ready in `harbor-system`.
- MinIO service `nexuspaas/minio` and Secret `minio-credentials` key names
  `access-key,secret-key` existed; values were not printed.
- Created unique MinIO bucket `velero-harbor-ops007-20260621161910`; initial object count was `0`.
- Installed Velero with official chart:
  - release: `velero`
  - namespace: `velero`
  - chart: `velero-12.0.3`
  - app version: `1.18.1`
  - AWS plugin: `velero/velero-plugin-for-aws:v1.14.1`
  - node-agent: ready `1/1`
  - BackupStorageLocation `default`: `Available`
- Restricted Velero values file mode was `600`; final `/tmp` lookup found no
  `velero-ops007-values.*` file.
- Created Harbor project `ops007-20260621161910`: HTTP `201`.
- ORAS pushed `harbor/ops007-20260621161910/evidence:seed` using `--plain-http` and
  `--password-stdin`.
- Baseline artifact digest:
  `sha256:a43acf6cd7b0a6748ce689ba4220a37d1c1864627ac6118d3f93da3705c0a506`.
- Harbor read-only set before backup:
  `readonly_set_status=200`, `read_only.value=true`.
- Redis exclusion labels were applied to:
  - pod `harbor-redis-0`
  - PVC `data-harbor-redis-0`
  - PV `pvc-fc1d7935-a0f8-4fe2-8ee7-8aba0afc862d`
- Velero Backup CR `ops007-harbor-20260621161910` reached phase `Completed` with `errors=0`.

Gate failure:

- Backup reported `warnings=3`.
- Expected Harbor PVC data volumes did **not** complete file-system backup:
  - `database-data` / PVC `database-data-harbor-database-0`
  - `job-logs` / PVC `harbor-jobservice`
  - `registry-data` / PVC `harbor-registry`
- Velero backup logs showed each was skipped because the current `local-path` provisioner presents
  the mounted PVC as `hostPath`, which Velero file-system backup does not support:
  - `Volume database-data ... is a hostPath volume which is not supported for pod volume backup`
  - `Volume job-logs ... is a hostPath volume which is not supported for pod volume backup`
  - `Volume registry-data ... is a hostPath volume which is not supported for pod volume backup`
- Only two PodVolumeBackup records completed, neither sufficient for OPS-007 Harbor restore:
  - `psc` from `harbor-core-65cf4949c-h4246`, `0` bytes
  - `shm-volume` from `harbor-database-0`, `1260352` bytes

Safety actions after gate failure:

- `harbor-system` namespace was **not** deleted.
- No Velero Restore CR was created.
- Harbor read-only was unset:
  `readonly_unset_status=200`, `read_only.value=false`.
- Synthetic Harbor repository/project cleanup succeeded:
  - `repository_delete_status=200`
  - `project_delete_status=200`
- Invalid Velero backup was deleted with `velero backup delete ... --confirm`; backup CR removal
  verified with `backup_deleted=true`.
- Bucket `velero-harbor-ops007-20260621161910` was emptied after deleting the invalid backup;
  final `bucket_object_count=0`.
- No `ops007` temporary pods remained in `harbor-system` or `nexuspaas`.
- Harbor remained healthy:
  - all Harbor deployments ready `1/1`
  - StatefulSets `harbor-database` and `harbor-redis` ready `1/1`
  - all four Harbor PVCs still `Bound`
- Velero remained installed and healthy for the next corrected attempt:
  - deployment `velero` ready `1/1`
  - DaemonSet `node-agent` ready `1/1`
  - BackupStorageLocation `default` `Available`

Conclusion:

- OPS-007 is **not complete**.
- The approved destructive step was correctly skipped.
- Next slice must move Harbor persistence away from Rancher `local-path` hostPath volumes or use
  another Harbor backup architecture that gives Velero-supported PVC data protection. Viable
  directions include Longhorn/CSI snapshots, a supported local/NFS volume provisioner, or a reviewed
  Harbor storage redesign with registry/object storage and database backup semantics.
