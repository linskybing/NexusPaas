# Harbor Foundation Live Deploy

Status: Approved

## 1. Objective

Deploy a minimal live Harbor instance with the official Harbor Helm chart in an isolated
`harbor-system` namespace so OPS-007 can move from "no Harbor exists" to a real follow-up
backup/restore drill target.

## 2. Background

`docs/acceptance/operations.md` requires OPS-007: Harbor backup and restore drill passes. Current
live inspection found no Harbor namespace, deployment, service, pod, PVC, or Helm release. The
`nexuspaas` namespace only has the first-party `image-registry-service`, which is not Harbor and
must not be used as substitute evidence.

Official references:

- Harbor Helm chart: https://github.com/goharbor/harbor-helm
- Harbor backup/restore guidance with Velero:
  https://goharbor.io/docs/main/administration/backup-restore/

The Harbor chart available from `https://helm.goharbor.io` is `harbor/harbor` version `1.19.1`
with app version `2.15.1`. A dry-run template shows the minimal clusterIP install creates Harbor
core, portal, registry, jobservice, internal database, and Redis resources.

## 3. Source References

- `docs/acceptance/operations.md`
- `docs/acceptance/image-build.md`
- `docs/acceptance/gap-analysis.md`
- `gap.md`
- `problem.md`
- `backend/docs/e2e-testing.md`
- Harbor Helm chart metadata from `helm show chart harbor/harbor`
- Live cluster storage/ingress/namespace checks from `kubectl`

## 4. Assumptions

- The current `kubectl` context is the same live local/RKE2-style context used by prior GA
  evidence slices.
- The cluster can pull Harbor images from the public registries referenced by the chart.
- The default `local-path` StorageClass can provision the small Harbor PVCs.
- This slice deploys Harbor foundation only; the official backup/restore drill comes later.
- Helm 3 is available locally.

## 5. Non-Goals

- Do not claim OPS-007 Harbor backup/restore is complete in this slice.
- Do not integrate NexusPaaS image build flows with Harbor in this slice.
- Do not configure external DNS, public ingress, TLS, OIDC, vulnerability scanning, Cosign,
  replication, robot accounts, or production retention policies in this slice.
- Do not install Velero in this slice.
- Do not commit Harbor admin passwords or generated secrets.

## 6. Current Behavior

There is no Harbor deployment in the live cluster. `gap.md` correctly keeps Harbor restore open.
Harbor-related Web/Image ACs remain partial because only first-party image-registry routes are
exercised.

## 7. Target Behavior

An isolated Harbor release exists and is ready:

1. Helm repo `harbor` is available locally, and chart version `1.19.1` / app version `2.15.1` is
   used explicitly.
2. Namespace `harbor-system` exists.
3. A temporary values file under `/tmp` configures a minimal internal Harbor release:
   - `expose.type=clusterIP`
   - `expose.tls.enabled=false`
   - `externalURL=http://harbor.harbor-system.svc.cluster.local`
   - `trivy.enabled=false` for foundation deploy resource control
   - small `local-path` PVC sizes
   - non-default generated chart secrets/passwords
4. `helm upgrade --install harbor harbor/harbor --version 1.19.1 -n harbor-system --wait
   --timeout=15m` succeeds.
5. Harbor deployments/statefulsets become ready.
6. A non-secret API readiness probe reaches Harbor from inside the cluster.
7. Ledgers state that Harbor foundation is live, while OPS-007 backup/restore, scan workflow, and
   image-build integration remain open.

## 8. Affected Domains

- OPS-007 prerequisite foundation
- Harbor/image registry infrastructure
- GA problem and gap tracking

## 9. Affected Files

- `docs/plan/2026-06-21-harbor-foundation-live-deploy.md`
- `gap.md`
- `problem.md`

## 10. API / Contract Changes

None for NexusPaaS APIs.

## 11. Database / Migration Changes

No NexusPaaS database changes. Harbor creates its own internal database resources and PVCs inside
`harbor-system`.

## 12. Configuration Changes

No repository configuration changes. Cluster-level changes are limited to a new `harbor-system`
namespace, Harbor Helm release resources, Harbor PVCs, and Harbor-generated/configured Secrets.

## 13. Observability Changes

No permanent NexusPaaS observability changes. Evidence will be collected from:

- `helm list -n harbor-system`
- `kubectl -n harbor-system get deploy,statefulset,pod,svc,pvc`
- `kubectl -n harbor-system rollout status ...`
- Harbor non-secret readiness/API probe

The full evidence table will be appended to this plan under `## 21. Implementation Evidence`.

## 14. Security Considerations

- Generate non-default Harbor admin password and chart secrets in a `mktemp` values file under
  `/tmp` with `umask 077` or equivalent `0600` permissions; do not print values.
- Delete the temporary values file with a trap after Helm succeeds or fails.
- Do not commit Harbor values containing secrets.
- Do not run `helm template --debug`, `helm install --dry-run`, or any command that prints rendered
  Secret manifests or values. Helm dry-run/template output may include Secret data.
- Do not expose Harbor externally in this slice; use `clusterIP`.
- Do not print Kubernetes Secret `.data` or decoded values.
- Record only secret names and resource readiness, not values.

## 15. Implementation Steps

1. Confirm preflight:
   - `kubectl config current-context`
   - no existing `harbor-system` namespace
   - no existing Harbor Helm release in any namespace
   - no Harbor-owned resources or PVCs in any namespace
   - if any Harbor namespace, release, Harbor-owned resource, or Harbor PVC already exists, stop and
     return to Reviewer Agent with non-secret evidence; do not reuse or mutate it in this plan
   - default StorageClass exists
   - Helm chart metadata for `harbor/harbor`
2. Create namespace `harbor-system`.
3. Generate a temporary values file under `/tmp` using `mktemp` under `umask 077`, confirm file
   mode is `600`, and write non-default generated password/secret values plus minimal clusterIP
   settings.
4. Run `helm upgrade --install harbor harbor/harbor --version 1.19.1 -n harbor-system --wait
   --timeout=15m -f <tmp-values>`.
5. Delete the temporary values file.
6. Verify Harbor readiness:
   - `helm list -n harbor-system`
   - `kubectl -n harbor-system get deploy,statefulset,pod,svc,pvc`
   - rollout status for all Harbor deployments/statefulsets
   - in-cluster HTTP probe to Harbor `/api/v2.0/ping` or another documented unauthenticated
     readiness endpoint
7. Update `gap.md` and `problem.md`: Harbor foundation is live, but OPS-007 backup/restore, Velero
   evidence, Harbor image build push/scan/delete workflows, and full external GA registry remain
   open.
8. Submit evidence and ledger updates to Reviewer Agent.

## 16. Verification Plan

- Helm install/upgrade exits successfully.
- Harbor release appears as deployed in `helm list -n harbor-system` with chart `harbor-1.19.1`
  and app version `2.15.1`.
- All expected Harbor deployments/statefulsets are ready.
- Harbor services and PVCs exist.
- In-cluster non-secret HTTP readiness/API probe succeeds.
- Temporary values file is absent after the install attempt.
- No `helm template --debug`, `helm install --dry-run`, or other command output containing rendered
  Secret manifests was used.
- `git diff --check` passes after ledger edits.
- Not applicable for this docs/live-evidence-only + cluster foundation slice because no application
  code, tests, build scripts, or NexusPaaS manifests are changed: `go -C backend test ./...`,
  `npm --prefix frontend run test`, `npm --prefix frontend run build`, and SonarScanner. If the
  scope expands into application code or repository manifests, these gates must run.

## 17. Rollback Plan

If Harbor install fails or readiness does not converge:

1. Capture `helm status harbor -n harbor-system` and non-secret `kubectl get/describe` diagnostics.
2. Delete the temporary values file.
3. If partial resources are unusable, run `helm uninstall harbor -n harbor-system`.
4. Preserve PVCs unless Reviewer Agent explicitly approves deletion, because Harbor chart defaults
   may keep PVCs.
5. If preflight found an existing Harbor namespace/release/resource/PVC, do not run Helm commands;
   return to Reviewer Agent with non-secret evidence.
6. Update ledgers honestly: Harbor foundation failed; OPS-007 remains open.

## 18. Risks and Tradeoffs

- Harbor is heavier than prior evidence slices and adds pods/PVCs to the cluster.
- ClusterIP-only deployment is enough for follow-up in-cluster backup/restore evidence but not for
  external user registry workflows.
- Disabling Trivy keeps foundation resource use controlled but means scan workflow remains open.
- Installing Harbor is a prerequisite; it is not itself a backup/restore drill.

## 19. Reviewer Checklist

- The plan does not claim OPS-007 is closed.
- Harbor is deployed via official Helm chart, not a custom registry.
- Secrets are generated and handled without printing or committing values.
- The chart version is pinned to `1.19.1`, and evidence records chart/app versions.
- Existing Harbor namespace/release/resource/PVC conflicts cause a hard stop before mutation.
- Temporary values file permissions are restricted and the file is removed via trap.
- No secret-revealing Helm dry-run/template output is produced.
- The installation is isolated to `harbor-system`.
- Rollback/cleanup does not accidentally delete unrelated resources.
- Ledger updates preserve remaining Harbor backup/restore and image workflow gaps.

## 20. Status

Status: Approved. Reviewer Agent approved the revised plan before live Harbor install commands ran.

## 21. Implementation Evidence

This foundation slice deployed Harbor with the official pinned Helm chart and then performed a
Reviewer-approved credential rebaseline in
`docs/plan/2026-06-21-harbor-foundation-credential-rebaseline.md`.

Initial foundation install:

- Kubernetes context: `default`.
- Namespace: `harbor-system`.
- Helm chart: `harbor/harbor --version 1.19.1`.
- App version: `2.15.1`.
- Initial install succeeded and Harbor returned HTTP `200` from in-cluster
  `/api/v2.0/ping`.
- Post-install review of public chart values found additional supported credential defaults that
  needed explicit generated values before Harbor became the OPS-007 target. Reviewer Agent approved
  a rebaseline before final evidence was recorded.

Credential rebaseline:

- `helm uninstall harbor -n harbor-system --wait --timeout=10m` succeeded.
- Exact foundation PVC cleanup succeeded for:
  - `data-harbor-redis-0`
  - `database-data-harbor-database-0`
  - `harbor-jobservice`
  - `harbor-registry`
- Post-cleanup checks showed no Harbor release, no PVCs, and only `configmap/kube-root-ca.crt`
  remaining in `harbor-system`.
- Reinstall used a restricted temporary values file with mode `600`, generated non-default values
  for chart-supported credential fields, and removed the values file after Helm completed.
- Final `/tmp` lookup found no `harbor-rebaseline-values.*` file.

Final Harbor foundation state:

- Helm release: `harbor`, namespace `harbor-system`, revision `1`, status `deployed`, chart
  `harbor-1.19.1`, app version `2.15.1`, deployed at `2026-06-21 15:59:24 +0800`.
- Deployments ready: `harbor-core`, `harbor-jobservice`, `harbor-nginx`, `harbor-portal`,
  `harbor-registry` all `1/1`.
- StatefulSets ready: `harbor-database` and `harbor-redis` both `1/1`.
- ClusterIP services exist: `harbor`, `harbor-core`, `harbor-database`, `harbor-jobservice`,
  `harbor-portal`, `harbor-redis`, `harbor-registry`.
- PVCs bound:
  - `data-harbor-redis-0` — `512Mi`
  - `database-data-harbor-database-0` — `1Gi`
  - `harbor-jobservice` — `512Mi`
  - `harbor-registry` — `1Gi`
- Rollout status succeeded for deployments `harbor-core`, `harbor-jobservice`, `harbor-nginx`,
  `harbor-portal`, `harbor-registry`, and StatefulSets `harbor-database`, `harbor-redis`.
- Final in-cluster API probe from one-time pod `harbor-rebaseline-curl-202606211600` returned
  HTTP `200` for `http://harbor/api/v2.0/ping`; the pod was deleted.
- Secret inventory was recorded only by name/type/key count:
  `harbor-core` 8, `harbor-database` 1, `harbor-jobservice` 2, `harbor-registry` 2,
  `harbor-registry-htpasswd` 1, `harbor-registryctl` 0, Helm release Secret 1.

Operator safety note: an intermediate verification command accidentally printed Harbor Secret
base64 `.data` into transient tool output. No decoded values, value hashes, or repository files
were produced. The final Harbor foundation was rebaselined with new generated credential values,
and the final evidence above avoids Secret values.

Remaining gaps:

- OPS-007 Harbor backup/restore is still open.
- Velero is not installed or tested.
- Trivy is disabled, so Harbor vulnerability scan workflow evidence is still open.
- Harbor is ClusterIP-only and is not yet integrated with NexusPaaS image build/push/delete
  workflows.
- External DNS, TLS, OIDC, robot accounts, retention, replication, signing, and external registry
  promotion are not configured in this slice.
- Chart `1.19.1` does not expose internal Redis auth through a supported
  `redis.internal.password` value; external Redis with `redis.external.password` requires a
  separate reviewed design if needed.
