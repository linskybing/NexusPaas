# Harbor Foundation Credential Rebaseline

Status: Approved

## 1. Objective

Rebaseline the newly created live Harbor foundation so every chart-exposed credential field used by
the minimal internal install is explicitly generated and non-default before Harbor becomes the
OPS-007 backup/restore drill target.

## 2. Background

`docs/plan/2026-06-21-harbor-foundation-live-deploy.md` installed Harbor with the official
`harbor/harbor` Helm chart version `1.19.1` / app version `2.15.1` in namespace
`harbor-system`. The release is live and reachable, but post-install review of the public chart
values showed additional chart-exposed credential fields with placeholder defaults:

- `registry.credentials.password`
- `database.internal.password`

Reviewer review also confirmed that chart `1.19.1` does not expose `redis.internal.password`;
internal Redis auth is not configured through a supported values key in this chart. This rebaseline
must not set ignored Redis values. A future external Redis design can use `redis.external.password`
if Harbor Redis auth becomes part of GA scope.

The first foundation install has no NexusPaaS integration, no external exposure, no known pushed
artifacts, and exists only as a prerequisite for later OPS-007 evidence. It is safer to delete and
recreate this just-created Harbor foundation with complete generated values than to carry chart
placeholder credentials into later GA work.

## 3. Source References

- `docs/plan/2026-06-21-harbor-foundation-live-deploy.md`
- `docs/acceptance/operations.md`
- `docs/acceptance/image-build.md`
- `gap.md`
- `problem.md`
- Harbor Helm chart values from `helm show values harbor/harbor --version 1.19.1`
- Live Harbor release and readiness evidence from `kubectl` and `helm`

## 4. Assumptions

- The current Harbor release was created by the active Harbor foundation slice and has no user data
  or NexusPaaS image workflow dependency yet.
- `harbor-system` contains only resources created for the Harbor foundation slice.
- Deleting the four Harbor PVCs created by the first install is acceptable only for this just-created
  foundation, because no backup/restore drill or registry workflow has used it yet.
- The same official chart version, `1.19.1`, remains the pinned chart for reinstall.

## 5. Non-Goals

- Do not claim OPS-007 Harbor backup/restore is complete.
- Do not configure external ingress, DNS, TLS, OIDC, robot accounts, Cosign, Trivy scan workflows,
  replication, retention, or Harbor/NexusPaaS image integration in this slice.
- Do not rotate an existing production Harbor instance. This plan applies only to the new
  foundation release created in the current session.
- Do not print, persist, hash, or commit secret values.

## 6. Current Behavior

Harbor is deployed in `harbor-system`, ready, and reachable via unauthenticated API ping. The
install used generated values for selected fields, but public chart defaults show additional
credential fields that must be explicitly provided for a GA-safe foundation.

## 7. Target Behavior

1. Reviewer explicitly approves cleanup of only the Harbor release and four Harbor PVCs created by
   the first foundation install.
2. `helm uninstall harbor -n harbor-system --wait` succeeds.
3. The exact PVCs from the first foundation install are deleted:
   - `data-harbor-redis-0`
   - `database-data-harbor-database-0`
   - `harbor-jobservice`
   - `harbor-registry`
4. A new restricted temporary values file is created with mode `600`, then removed by trap.
5. The reinstall uses `helm upgrade --install harbor harbor/harbor --version 1.19.1`.
6. The new values file explicitly sets generated non-default values for chart-supported fields:
   - `harborAdminPassword`
   - `secretKey`
   - `core.secret`
   - `core.xsrfKey`
   - `jobservice.secret`
   - `registry.secret`
   - `registry.credentials.password`
   - `database.internal.password`
7. The values file does not set unsupported Redis internal password keys.
8. Harbor returns to ready state and in-cluster `/api/v2.0/ping` returns HTTP `200`.
9. Ledgers record Harbor foundation as live with explicit credential rebaseline, while OPS-007 and
   all remaining Harbor workflow gaps stay open.

## 8. Affected Domains

- Harbor foundation security posture
- OPS-007 prerequisite foundation
- GA problem and gap tracking

## 9. Affected Files

- `docs/plan/2026-06-21-harbor-foundation-credential-rebaseline.md`
- `docs/plan/2026-06-21-harbor-foundation-live-deploy.md`
- `gap.md`
- `problem.md`

## 10. API / Contract Changes

None for NexusPaaS APIs.

## 11. Database / Migration Changes

No NexusPaaS database changes. The just-created Harbor internal database PVC is deleted and
recreated before any Harbor backup/restore or image workflow evidence depends on it.

## 12. Configuration Changes

Cluster-only Harbor resources in `harbor-system` are recreated through the official Helm chart. No
repository runtime configuration or committed secret file changes.

## 13. Observability Changes

No permanent NexusPaaS observability changes. Evidence will be collected from:

- `helm history/list/status`
- `kubectl get pvc` before and after cleanup
- `kubectl get deploy,statefulset,pod,svc,pvc`
- rollout status for Harbor deployments/statefulsets
- in-cluster Harbor API ping
- secret inventory by name/type/key count only

## 14. Security Considerations

- This rebaseline exists specifically to avoid supported chart placeholder credential carryover.
- Generate all credential values locally and write them only to a `mktemp` file with restricted
  permissions.
- Do not print Helm rendered manifests, `helm get manifest`, `helm get values`, Kubernetes Secret
  `.data`, decoded values, or value hashes.
- Record only secret names, types, and key counts.
- Do not set unsupported Redis internal password keys; if Redis auth is required later, design an
  external Redis deployment with `redis.external.password` in a separate reviewed slice.
- Delete only Harbor resources in `harbor-system`; do not touch `nexuspaas` runtime secrets.

## 15. Implementation Steps

1. Confirm the current context, release, chart/app version, namespace, and exact Harbor PVC names.
2. Confirm this is still the just-created foundation release and stop if there is evidence of
   external integration or unknown non-Harbor resources in `harbor-system`.
3. Run `helm uninstall harbor -n harbor-system --wait --timeout=10m`.
4. Delete only the exact Harbor PVC names listed in target behavior.
5. Create a restricted temporary values file with complete generated credential values and minimal
   ClusterIP settings.
6. Reinstall with `helm upgrade --install harbor harbor/harbor --version 1.19.1 -n harbor-system
   --wait --timeout=15m -f <tmp-values>`.
7. Delete the temporary values file.
8. Verify Helm release, rollout readiness, services/PVCs, secret name/type/key-count inventory, and
   in-cluster API ping.
9. Append implementation evidence to this plan and the original foundation plan.
10. Update `gap.md` and `problem.md` without closing OPS-007.

## 16. Verification Plan

- `helm uninstall` and reinstall both exit successfully.
- Only the four Harbor PVCs named in this plan are deleted.
- Reinstalled Harbor release is deployed as `harbor-1.19.1` / app `2.15.1`.
- All Harbor deployments/statefulsets are ready.
- Services and PVCs exist.
- In-cluster `/api/v2.0/ping` returns `200`.
- Temporary values file is absent after the install attempt.
- Secret inventory evidence contains names/types/key counts only.
- `git diff --check` passes after documentation updates.
- Application tests/build/Sonar are not applicable unless this slice expands into application code
  or repository manifests.

## 17. Rollback Plan

If reinstall fails:

1. Capture non-secret Helm and Kubernetes readiness diagnostics.
2. Remove the temporary values file.
3. Leave failed resources in place for inspection unless Reviewer Agent approves cleanup.
4. Update ledgers honestly: Harbor foundation is not ready and OPS-007 remains blocked.

## 18. Risks and Tradeoffs

- Deleting PVCs is destructive, but scoped to a just-created, unused Harbor foundation.
- Reinstalling now is simpler and safer than rotating internal database/registry credentials after
  data exists.
- This improves foundation hygiene but still does not provide backup/restore, external registry,
  TLS, OIDC, vulnerability scan, or image workflow evidence.

## 19. Reviewer Checklist

- Destructive cleanup is explicitly scoped to the newly created Harbor foundation only.
- The plan deletes exact PVC names, not broad cluster resources.
- The official Harbor chart remains pinned to `1.19.1`.
- Generated values cover all identified supported chart credential defaults.
- The plan does not set ignored Redis internal password keys.
- Secret values are never printed, hashed, committed, or included in evidence.
- The plan does not claim OPS-007 is complete.
- Ledgers preserve remaining GA Harbor and operations gaps.

## 20. Status

Status: Approved. Reviewer Agent approved revision 1 before uninstall/PVC deletion/reinstall.

## 21. Preflight Evidence

Non-secret checks collected before any destructive command:

- Kubernetes context: `default`.
- Helm history: release `harbor` in namespace `harbor-system` has one deployed revision:
  `harbor-1.19.1` / app `2.15.1`, description `Install complete`.
- Current Harbor PVCs:
  - `data-harbor-redis-0` — `Bound`, `local-path`, `512Mi`
  - `database-data-harbor-database-0` — `Bound`, `local-path`, `1Gi`
  - `harbor-jobservice` — `Bound`, `local-path`, `512Mi`
  - `harbor-registry` — `Bound`, `local-path`, `1Gi`
- PVC label check: `harbor-jobservice` and `harbor-registry` carry Helm/instance labels and keep
  policy; the Redis and database StatefulSet PVCs do not carry the same Helm instance labels.
  Therefore cleanup must use the exact PVC names listed in this plan, not a broad label selector.
- Namespace resource inventory contains Harbor release resources plus Kubernetes' namespace
  `kube-root-ca.crt` ConfigMap. No NexusPaaS application resources were present in
  `harbor-system`.

## 22. Implementation Evidence

Rebaseline execution was completed after Reviewer Agent approval.

Cleanup:

- `helm uninstall harbor -n harbor-system --wait --timeout=10m` succeeded.
- Helm reported that `harbor-jobservice` and `harbor-registry` PVCs were kept by resource policy.
- All four exact PVC names were deleted successfully:
  - `data-harbor-redis-0`
  - `database-data-harbor-database-0`
  - `harbor-jobservice`
  - `harbor-registry`
- Post-cleanup checks showed no Helm release, no PVCs, and only `configmap/kube-root-ca.crt`
  remaining in `harbor-system`.

Reinstall:

- Temporary values file mode: `600`.
- Helm command used the official pinned chart:
  `helm upgrade --install harbor harbor/harbor --version 1.19.1 -n harbor-system --wait --timeout=15m`.
- Helm release result:
  - release: `harbor`
  - namespace: `harbor-system`
  - revision: `1`
  - status: `deployed`
  - chart: `harbor-1.19.1`
  - app version: `2.15.1`
  - deployed at: `2026-06-21 15:59:24 +0800`
- Temporary values file cleanup: `values_file_removed=true`; a final `/tmp` lookup found no
  `harbor-rebaseline-values.*` file.

Final live readiness:

- Deployments ready: `harbor-core`, `harbor-jobservice`, `harbor-nginx`, `harbor-portal`,
  `harbor-registry` all `1/1`.
- StatefulSets ready: `harbor-database` and `harbor-redis` both `1/1`.
- Pods running:
  - `harbor-core-65cf4949c-h4246` — `1/1`
  - `harbor-database-0` — `1/1`
  - `harbor-jobservice-57c9fbf5f5-sr7zc` — `1/1` after two startup restarts
  - `harbor-nginx-f69c766d-5gkj5` — `1/1`
  - `harbor-portal-65bff5d7c6-27ftk` — `1/1`
  - `harbor-redis-0` — `1/1`
  - `harbor-registry-74895c6464-f4fd7` — `2/2`
- Services exist as ClusterIP-only services: `harbor`, `harbor-core`, `harbor-database`,
  `harbor-jobservice`, `harbor-portal`, `harbor-redis`, and `harbor-registry`.
- PVCs are bound with the expected small foundation sizes:
  - `data-harbor-redis-0` — `512Mi`
  - `database-data-harbor-database-0` — `1Gi`
  - `harbor-jobservice` — `512Mi`
  - `harbor-registry` — `1Gi`
- Rollout status succeeded for deployments `harbor-core`, `harbor-jobservice`, `harbor-nginx`,
  `harbor-portal`, `harbor-registry`, and StatefulSets `harbor-database`, `harbor-redis`.
- In-cluster API probe:
  `curl -fsS -o /dev/null -w '%{http_code}\n' http://harbor/api/v2.0/ping` returned `200` from
  one-time pod `harbor-rebaseline-curl-202606211600`; the pod was deleted.

Secret inventory evidence was limited to names, types, and key counts:

| Secret | Type | Key Count |
| --- | --- | ---: |
| `harbor-core` | `Opaque` | 8 |
| `harbor-database` | `Opaque` | 1 |
| `harbor-jobservice` | `Opaque` | 2 |
| `harbor-registry` | `Opaque` | 2 |
| `harbor-registry-htpasswd` | `Opaque` | 1 |
| `harbor-registryctl` | `Opaque` | 0 |
| `sh.helm.release.v1.harbor.v1` | `helm.sh/release.v1` | 1 |

Operator safety note: during the earlier foundation verification, one command accidentally printed
Harbor Secret base64 `.data` into transient tool output. No decoded values, hashes, or repository
files were produced. The final foundation was immediately rebaselined with new generated credential
values, and final evidence uses only the key-count inventory above.

Remaining scope after this slice:

- OPS-007 Harbor backup/restore remains open.
- Velero is not installed or tested.
- Harbor is ClusterIP-only and not integrated with NexusPaaS image workflows.
- Trivy is disabled, so Harbor scan workflows remain open.
- External DNS/TLS/OIDC/robot accounts/retention/replication/signing are not configured.
- Chart `1.19.1` does not expose internal Redis auth through a supported
  `redis.internal.password` value. If Redis auth becomes a GA requirement, use a separately
  reviewed external Redis design with `redis.external.password`.
