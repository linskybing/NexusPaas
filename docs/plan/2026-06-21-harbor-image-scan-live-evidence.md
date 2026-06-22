# Harbor Image Scan Live Evidence

Status: Approved

## 1. Objective

Close the next Harbor image workflow evidence gap by enabling Harbor Trivy on the restored
static-local Harbor foundation, pushing one real OCI image to Harbor, triggering a vulnerability
scan, recording scan status through Harbor API, proving repository deletion cleanup, and updating
the GA ledgers.

## 2. Background

OPS-007 Harbor backup/restore now passes on Kubernetes static `local` PVs. The remaining Web/IMG
gap is not backup/restore; it is Harbor image workflow and scan evidence. Existing first-party GUI
evidence shows image/build rows from NexusPaaS APIs, but not Harbor-side push/delete/scan behavior.

Harbor chart `harbor-1.19.1` ships Trivy adapter support. Current live Harbor was installed with
`trivy.enabled=false` for the foundation and backup/restore slices, so this slice must enable the
official chart Trivy component before scanning.

## 3. Source References

- `docs/acceptance/image-build.md`
- `docs/acceptance/cli.md`
- `docs/acceptance/operations.md`
- `docs/acceptance/gap-analysis.md`
- `gap.md`
- `problem.md`
- Official Harbor chart values from `helm show values harbor/harbor --version 1.19.1`
- Harbor vulnerability scanning docs: https://goharbor.io/docs/2.3.0/administration/vulnerability-scanning/
- Go containerregistry `crane` project: https://github.com/google/go-containerregistry

## 4. Assumptions

- Current context is `default`.
- Harbor is healthy in namespace `harbor-system` on chart `harbor-1.19.1` / app `2.15.1`.
- Harbor uses the static local PV/PVC contract from
  `docs/plan/2026-06-21-harbor-static-local-pv-velero-drill.md`.
- A temporary `crane` pod can copy a public OCI image into Harbor over in-cluster HTTP without
  changing the host Docker daemon or external ingress.
- This is local live evidence. It does not make Harbor the external GA registry.
- Execution stamp is `20260621174500`.

## 5. Non-Goals

- Do not implement NexusPaaS image-build code paths in this slice.
- Do not claim full IMG-001..025 coverage.
- Do not claim external registry, signing, SBOM, allow-list admission, or GUI Harbor scan parity.
- Do not print, hash, or commit secret values.
- Do not delete existing Harbor backup/restore evidence resources.

## 6. Current Behavior

Pre-execution baseline: Harbor was live and recoverable, but Trivy was not deployed and the GA
ledgers still recorded missing Harbor image workflow/scan evidence.

Current resume state after approved partial execution:

- Trivy static local PV/PVC `harbor-static-trivy-20260621174500` exists and is `Bound` on
  StorageClass `nexuspaas-static-local-ops007`.
- Harbor was upgraded to revision 2 with `trivy.enabled=true` and
  `persistence.persistentVolumeClaim.trivy.existingClaim=harbor-static-trivy-20260621174500`.
- `harbor-trivy` is deployed and ready.
- No Harbor PVC uses `local-path`.
- Synthetic project `ops-img-scan-20260621174500` has already been created.
- Docker push through localhost port-forward was attempted and rejected as the wrong approach for
  this environment; no host Docker daemon configuration was changed.
- Remaining work starts at the in-cluster `crane` image copy.

## 7. Target Behavior

1. Harbor Trivy is enabled through the official Helm chart with `--reuse-values`, preserving the
   static local PVC mappings and generated credentials.
2. Trivy cache storage uses an explicitly pre-created static `local` PV/PVC, not the default
   `local-path` StorageClass.
3. Harbor remains ready after the upgrade, including `harbor-trivy`.
4. A synthetic Harbor project is created only for this drill.
5. A real container image is copied to Harbor by `crane` from a temporary Kubernetes pod.
6. Harbor API can list the repository and artifact digest.
7. A Harbor vulnerability scan is triggered for the artifact and reaches a terminal successful
   state or returns a documented scanner result for the pushed image.
8. The repository/artifact deletion path is exercised and the synthetic project is cleaned up.
9. Ledgers distinguish this as Harbor-side image workflow/scan evidence, not full NexusPaaS
   image-build/allow-list completion.

## 7.1 Trivy Static Local PV Contract

Trivy cache proof resources:

- PV/PVC: `harbor-static-trivy-20260621174500`
- Namespace: `harbor-system`
- Size: `5Gi` to match the Harbor chart default.
- Access mode: `ReadWriteOnce`
- Reclaim policy: `Retain`
- StorageClass: `nexuspaas-static-local-ops007`
- Local path:
  `/home/lin/.local/share/nexuspaas/harbor-trivy-static-local-pv-20260621174500/trivy`
- Node affinity:
  - key: `kubernetes.io/hostname`
  - operator: `In`
  - values: `sky-desktop`

Helm value:

- `persistence.persistentVolumeClaim.trivy.existingClaim=harbor-static-trivy-20260621174500`

## 8. Affected Domains

- Harbor image workflow evidence
- Harbor Trivy scanner foundation
- WEB-005 / CLI-009 / IMG scan evidence ledgers

## 9. Affected Files

- `docs/plan/2026-06-21-harbor-image-scan-live-evidence.md`
- `gap.md`
- `problem.md`
- `docs/acceptance/gap-analysis.md`

## 10. API / Contract Changes

None for NexusPaaS APIs.

## 11. Database / Migration Changes

No NexusPaaS database changes. Harbor internal metadata changes are limited to the synthetic
project/repository/artifact and are cleaned up at the end.

## 12. Configuration Changes

Live cluster changes:

- Helm upgrade of release `harbor` in namespace `harbor-system` with `trivy.enabled=true`.
- Static local Trivy PV/PVC `harbor-static-trivy-20260621174500`.
- Temporary synthetic Harbor project/repository/artifact.

No repository runtime configuration or committed secret files are changed.

## 13. Observability Changes

Evidence will be collected from:

- Helm list/status and Kubernetes readiness for Harbor and Trivy.
- `crane` copy output digest.
- Harbor API project/repository/artifact/scan status.
- Cleanup verification for repository/project deletion.

## 14. Security Considerations

- Harbor admin password must be consumed through `secretKeyRef` pods and `crane auth login
  --password-stdin`; it must not appear in command arguments, logs, docs, or files.
- `crane` auth config must live only in the temporary pod filesystem and disappear when the pod is
  deleted. Do not use the host Docker config or a long-lived Kubernetes Secret for this drill.
- Synthetic image data must be deleted after evidence.
- Trivy database downloads may use public upstream repositories from the official chart defaults;
  if the database cannot update due upstream/rate/network issues, stop and record the blocker
  instead of faking scan success.

## 15. Completed Execution Evidence Before Amendment

- Trivy PV/PVC `harbor-static-trivy-20260621174500` was created and reached `Bound`.
- `helm upgrade harbor harbor/harbor --namespace harbor-system --version 1.19.1 --reuse-values
  --set trivy.enabled=true
  --set persistence.persistentVolumeClaim.trivy.existingClaim=harbor-static-trivy-20260621174500
  --wait --timeout=15m` completed.
- Harbor release is revision 2 on chart `harbor-1.19.1` / app `2.15.1`.
- `harbor-trivy` StatefulSet is `1/1`.
- `harbor_local_path_pvc_count=0`.
- Synthetic project `ops-img-scan-20260621174500` was created with `project_create_status=201`.
- Docker localhost port-forward push was attempted, then abandoned because the host Docker daemon
  could not reliably reach the user-space port-forward/insecure registry path. Cleanup trap removed
  the temp Docker config and stopped the failed port-forward attempt.

## 15.1 Remaining Implementation Steps

1. Verify Harbor readiness, `harbor-trivy` readiness, Trivy PVC `Bound`, no Harbor PVC uses
   `local-path`, and project `ops-img-scan-20260621174500` exists.
2. Verify the pinned `crane` debug image can be pulled by creating the one-shot pod.
3. Run a one-shot pod using
   `gcr.io/go-containerregistry/crane:debug@sha256:d6146d930d9a1c78fe03ef52e8086900d42232789ca400155b6bcaca5912d807`
   with command override `["/busybox/sh", "-c", "..."]`. This digest was verified to include a
   shell and `crane version` output `c68d899782693b63b7549166812aa996f17a660c`. Inside the pod:
   - set `HOME` to a temp directory
   - use `crane auth login --insecure -u admin --password-stdin <harbor-cluster-ip>`
   - run `crane copy --insecure busybox:1.36
     <harbor-cluster-ip>/ops-img-scan-20260621174500/busybox:1.36`
   - run `crane digest --insecure
     <harbor-cluster-ip>/ops-img-scan-20260621174500/busybox:1.36`
   - print only copy success and digest
4. Delete the `crane` pod after completion.
5. Query Harbor API to record repository, artifact digest, and initial scan overview.
6. Trigger scan for the pushed artifact through Harbor API and poll until the scan state is
   terminal. Record status, severity summary if available, and scanner metadata without dumping
   large vulnerability reports.
7. Delete the repository/artifact and project through Harbor API, then verify the project is
    absent.
8. Update plan evidence, `gap.md`, `problem.md`, and `docs/acceptance/gap-analysis.md`; run
    `git diff --check`; submit to Reviewer Agent.

## 15.2 Harbor API Endpoints And Scan Criteria

All API calls use the Harbor service URL from inside the cluster and admin credentials supplied by
`secretKeyRef`.

- Create project:
  `POST /api/v2.0/projects`
  - expected HTTP: `201`
- Read project:
  `GET /api/v2.0/projects/ops-img-scan-20260621174500`
  - expected HTTP: `200`
- List repository:
  `GET /api/v2.0/projects/ops-img-scan-20260621174500/repositories`
  - expected HTTP: `200`
- Read artifact with scan overview:
  `GET /api/v2.0/projects/ops-img-scan-20260621174500/repositories/busybox/artifacts/1.36?with_scan_overview=true`
  - expected HTTP: `200`
  - record: `digest`, `tags[].name`, and the compact `scan_overview` status fields.
- Trigger scan:
  `POST /api/v2.0/projects/ops-img-scan-20260621174500/repositories/busybox/artifacts/1.36/scan`
  - expected HTTP: `202` accepted, or `200` if Harbor reports an already-completed scan.
- Poll scan overview:
  `GET /api/v2.0/projects/ops-img-scan-20260621174500/repositories/busybox/artifacts/1.36?with_scan_overview=true`
  - successful terminal statuses: `Success` or `Complete`
  - blocked statuses: `Error`, `Failed`, `Stopped`, or `Unsupported`
  - in-progress statuses: `Queued`, `Pending`, `Running`, or `Scanning`
  - record only: artifact digest, scanner name/vendor/version if present, `scan_status`,
    severity, and summary counts by severity/fixable if present.
- Read vulnerability report summary:
  `GET /api/v2.0/projects/ops-img-scan-20260621174500/repositories/busybox/artifacts/1.36/additions/vulnerabilities`
  - expected HTTP after successful scan: `200`
  - record count/summary only; do not dump the full vulnerability report.
- Delete repository:
  `DELETE /api/v2.0/projects/ops-img-scan-20260621174500/repositories/busybox`
  - expected HTTP: `200` or `404` during cleanup retry.
- Delete project:
  `DELETE /api/v2.0/projects/ops-img-scan-20260621174500`
  - expected HTTP: `200` or `404` during cleanup retry.

## 16. Verification Plan

- Harbor chart remains `harbor-1.19.1` / app `2.15.1`.
- `harbor-trivy` Deployment is available.
- Trivy PVC is `Bound` on StorageClass `nexuspaas-static-local-ops007`, and no Harbor PVC uses
  `local-path`.
- `crane` image copy to Harbor succeeds and Harbor API reports the artifact digest.
- Scan trigger returns an accepted/success status and polling reaches a terminal successful scan
  state, or the plan records a real external blocker and leaves the gap open.
- Repository/project cleanup returns success and final Harbor project list does not contain the
  synthetic project.
- Temporary `crane` pod is deleted.
- `git diff --check` passes.
- App tests/build/Sonar remain N/A unless repo code changes.

## 17. Rollback Plan

If Trivy upgrade fails:

1. Run `helm rollback harbor <previous-revision> -n harbor-system --wait --timeout=15m`.
2. Verify Harbor readiness.
3. Keep Harbor scan evidence open and record the blocker.

If `crane` image copy or scan fails:

1. Clean the synthetic project if it was created.
2. Delete any temporary `crane` or curl pods.
3. Leave Harbor running with Trivy enabled if Harbor is healthy; otherwise roll back to the prior
   revision.
4. Keep Harbor image/scan evidence open and record the blocker.

## 18. Risks and Tradeoffs

- Trivy DB update depends on upstream OCI/GitHub mirrors and can fail due network or rate limits.
- In-cluster `crane` copy proves Harbor registry behavior without external ingress/TLS readiness.
- A single `busybox` scan is evidence of scanner integration, not a complete image policy program.

## 19. Reviewer Checklist

- Uses official Harbor chart Trivy, not a custom scanner.
- Uses a real container image, not only a generic ORAS artifact.
- Secret values are not printed, hashed, or committed.
- Synthetic Harbor data and temporary pods are cleaned up.
- Ledgers do not overclaim full IMG/WEB/CLI image-build completion.

## 20. Status

Status: Approved. Reviewer Agent approved the Trivy plan before live Harbor upgrade and later
approved the amendment to resume image copy through a digest-pinned, shell-capable in-cluster
`crane` pod after Docker localhost port-forward proved unsuitable for this environment.
Implementation is complete and pending final Reviewer Agent verification.

## 21. Execution Evidence

Execution stamp: `20260621174500`.

Preflight and Trivy enablement:

- Initial preflight showed `trivy_pv_exists=false`, `trivy_pvc_exists=false`,
  `harbor_trivy_deploy_exists=false`, Harbor Deployments/StatefulSets ready, and synthetic project
  `ops-img-scan-20260621174500` absent with `scan_project_status=404`.
- Trivy static local PVC `harbor-static-trivy-20260621174500` reached `Bound` on StorageClass
  `nexuspaas-static-local-ops007`; backing PV uses reclaim policy `Retain` and local path
  `/home/lin/.local/share/nexuspaas/harbor-trivy-static-local-pv-20260621174500/trivy`.
- `helm upgrade harbor harbor/harbor --namespace harbor-system --version 1.19.1 --reuse-values
  --set trivy.enabled=true
  --set persistence.persistentVolumeClaim.trivy.existingClaim=harbor-static-trivy-20260621174500
  --wait --timeout=15m` completed.
- Harbor release is revision 2, chart `harbor-1.19.1`, app `2.15.1`.
- `harbor-trivy` StatefulSet reached `1/1`; all Harbor Deployments and StatefulSets were ready.
- `harbor_local_path_pvc_count=0`.
- Synthetic project creation returned `project_create_status=201`.

Image copy:

- Docker localhost port-forward push was attempted first and abandoned because the host Docker
  daemon could not reliably reach the user-space port-forward/insecure registry path; the cleanup
  trap removed the temp Docker config and stopped the failed port-forward attempt.
- The approved resume used one-shot pod `harbor-crane-copy-20260621174500` with digest-pinned image
  `gcr.io/go-containerregistry/crane:debug@sha256:d6146d930d9a1c78fe03ef52e8086900d42232789ca400155b6bcaca5912d807`
  and `/busybox/sh`.
- `crane auth login --insecure -u admin --password-stdin` used `secretKeyRef` and wrote credentials
  only inside the pod's `/tmp/crane-home`; the pod was deleted after copy.
- `crane copy --insecure busybox:1.36
  <harbor-cluster-ip>/ops-img-scan-20260621174500/busybox:1.36` completed.
- `crane_digest=sha256:73aaf090f3d85aa34ee199857f03fa3a95c8ede2ffd4cc2cdb5b94e566b11662`.

Scan:

- Artifact readback returned `artifact_read_status=200`,
  `artifact_digest=sha256:73aaf090f3d85aa34ee199857f03fa3a95c8ede2ffd4cc2cdb5b94e566b11662`,
  and `artifact_tags=1.36`.
- Scan trigger returned `scan_trigger_status=202`.
- Polling observed `scan_status=Running` for attempts 1 and 2, then
  `scan_terminal_status=Success` on attempt 3.
- Scan report key was `application/vnd.security.vulnerability.report; version=1.1`.
- Compact scan summary returned `severity=None`, `scan_summary_total=0`,
  `scan_summary_fixable=0`, and `vulnerability_addition_status=200`.

Cleanup and final state:

- Repository cleanup returned `repository_delete_status=200`.
- Project cleanup returned `project_delete_status=200`.
- Final synthetic project read returned `project_after_delete_status=404` and
  `scan_project_final_status=404`.
- Final Harbor API ping returned `final_ping_status=200`.
- Final Harbor release remained revision 2, chart `harbor-1.19.1`, app `2.15.1`; Harbor and Trivy
  remained ready.
- Final `harbor_local_path_pvc_count=0`.
- Temporary pods matching this scan drill were removed: `temp_pods_remaining=0`.
