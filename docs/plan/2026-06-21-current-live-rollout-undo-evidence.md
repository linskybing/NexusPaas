# Current Live Rollout Undo Evidence

Status: Approved

## 1. Objective

Capture live rollback-path evidence for the current first-party backend deployments in the
`nexuspaas` namespace by using Kubernetes-native serial `rollout restart` followed by
`rollout undo` back to the captured pre-restart revision.

## 2. Background

`gap.md` still records rollback as open because only `platform-gateway` has local rollback
evidence. The current live namespace runs the legacy 15 first-party backend service
deployments, while `backend/deploy/k3s/production-beta/backend-units.yaml` defines the target
8 physical backend units. This plan captures useful current-live controller evidence without
claiming the 8-unit Production Beta topology is fully deployed or rolled back.

## 3. Source References

- `gap.md`
- `problem.md`
- `docs/acceptance/gap-analysis.md`
- `backend/deploy/k3s/production-beta/backend-units.yaml`
- `backend/deploy/k3s/production-beta/runtime-config.yaml`
- Live `nexuspaas` namespace deployment state from `kubectl`

## 4. Assumptions

- All selected deployments are currently ready before the rehearsal starts.
- Kubernetes retains the captured pre-restart revision long enough for the immediate undo step.
- This is an evidence slice, not a user-facing feature change.

## 5. Non-Goals

- Do not change application source code.
- Do not change Kubernetes manifests or runtime config.
- Do not deploy the 8-unit Production Beta topology in this slice.
- Do not claim previous-image rollback coverage. The restart and undo use the same captured image.
- Do not exercise backing dependencies such as Postgres, Redis, MinIO, Dex, or Coturn.

## 6. Current Behavior

The live namespace has 15 first-party backend deployments plus backing dependencies. The rollback
ledger does not contain per-deployment live rollback-path evidence for these first-party
deployments, and the GA ledger still treats rollback as open.

## 7. Target Behavior

For each selected first-party backend deployment:

1. Capture `kubectl config current-context`, namespace, selected deployment list, and current
   readiness before any restart.
2. Capture current image, ready status, and deployment revision.
3. Run `kubectl rollout restart`.
4. Wait for `kubectl rollout status` to succeed.
5. Run `kubectl rollout undo --to-revision=<captured revision>`.
6. Wait for `kubectl rollout status` to succeed.
7. Record final image and ready status.

The ledgers will be updated to show this as partial rollback evidence for the current live
15-service topology, with remaining gaps clearly preserved.

## 8. Affected Domains

- OPS rollback evidence
- Live Kubernetes deployment operations
- GA problem and gap tracking

## 9. Affected Files

- `docs/plan/2026-06-21-current-live-rollout-undo-evidence.md`
- `gap.md`
- `problem.md`

## 10. API / Contract Changes

None.

## 11. Database / Migration Changes

None.

## 12. Configuration Changes

None.

## 13. Observability Changes

No permanent observability changes. Evidence will be collected from:

- `kubectl config current-context`
- `kubectl get deploy`
- `kubectl rollout status`
- deployment revision annotations
- final ready replica counts

The full evidence table will be appended to this plan under `## 21. Implementation Evidence`.
Minimum fields are: deployment, pre image, pre revision, restart revision, undo target revision,
post/final revision, final image, ready replicas, desired replicas, restart rollout status, undo
rollout status, and a note that the rehearsal is same-image Kubernetes controller rollback-path
evidence only.

## 14. Security Considerations

- Do not print Secret values.
- Do not change RBAC, NetworkPolicy, runtime secrets, or service credentials.
- Stop the serial rehearsal on the first failed rollout or undo instead of continuing into a
  degraded state.

## 15. Implementation Steps

1. Capture preflight evidence before any restart:
   - `kubectl config current-context`
   - selected namespace: `nexuspaas`
   - selected deployment list
   - readiness and image for every selected deployment
2. Confirm all selected first-party backend deployments are ready.
3. Run a serial rollout restart and undo for:
   - `audit-compliance-service`
   - `authorization-policy-service`
   - `ide-service`
   - `identity-service`
   - `image-registry-service`
   - `integration-proxy-service`
   - `k8s-control-service`
   - `media-upload-service`
   - `org-project-service`
   - `platform-gateway`
   - `request-notification-service`
   - `scheduler-quota-service`
   - `storage-service`
   - `usage-observability-service`
   - `workload-service`
4. Record per-deployment evidence in this plan's `## 21. Implementation Evidence` section:
   deployment, pre image, pre revision, restart revision, undo target revision, post/final
   revision, final image, ready replicas, desired replicas, restart rollout status, undo rollout
   status, and same-image rollback-path caveat.
5. Update `gap.md` and `problem.md` with reconciled wording: current-live 15-deployment same-image
   rollout/undo evidence exists, while 8-unit staging rollback, previous-image rollback,
   backup/restore, and full GA rollback remain open.
6. Submit the evidence and ledger updates to Reviewer Agent.

## 16. Verification Plan

- Preflight prints `kubectl config current-context`, namespace, selected deployment list, and
  readiness before any restart.
- `kubectl -n nexuspaas get deploy <name>` shows each deployment ready before starting its step.
- `kubectl -n nexuspaas rollout status deploy/<name> --timeout=180s` succeeds after restart.
- `kubectl -n nexuspaas rollout status deploy/<name> --timeout=180s` succeeds after undo.
- Final deployment image equals the captured image.
- Final ready replicas match desired replicas for each selected deployment.
- `git diff --check` passes after ledger edits.
- Not applicable for this docs/live-evidence-only slice because no application code, tests, build
  scripts, manifests, or runtime config are changed: `go -C backend test ./...`, `npm --prefix
  frontend run test`, `npm --prefix frontend run build`, and SonarScanner. If the scope expands
  beyond docs/evidence, these gates must run.

## 17. Rollback Plan

If any selected deployment fails restart or undo:

1. Stop the serial rehearsal immediately.
2. Set the deployment image back to the captured image if the live spec changed unexpectedly.
3. Run `kubectl rollout status` for that deployment.
4. Capture `kubectl describe deploy` and recent events for triage.
5. Leave subsequent deployments untouched.

## 18. Risks and Tradeoffs

- This briefly restarts pods for first-party backend services and may cause short-lived request
  retries.
- This proves the current live Kubernetes rollback controller path, not full GA rollback for the
  target 8-unit Production Beta topology.
- This does not prove rollback across schema or previous-image changes.
- Serial execution keeps blast radius low and gives a clear stop point.

## 19. Reviewer Checklist

- The plan does not overstate the evidence.
- The deployment list excludes backing dependencies.
- The plan uses Kubernetes-native rollout mechanisms instead of a custom rollback tool.
- The failure handling stops on the first degraded deployment.
- Ledger updates must preserve remaining GA gaps.
- The evidence table records that this is same-image controller rollback-path evidence, not
  previous-image or 8-unit GA rollback evidence.

## 20. Status

Status: Approved. Reviewer Agent approved the revised plan before live rollout commands ran.

## 21. Implementation Evidence

Executed on 2026-06-21T14:54:34+08:00 against `kubectl config current-context=default`,
namespace `nexuspaas`.

Selected deployments:

`audit-compliance-service authorization-policy-service ide-service identity-service image-registry-service integration-proxy-service k8s-control-service media-upload-service org-project-service platform-gateway request-notification-service scheduler-quota-service storage-service usage-observability-service workload-service`

All selected deployments passed serial same-image Kubernetes controller rollback-path rehearsal:
`rollout restart`, successful `rollout status`, `rollout undo --to-revision=<pre_revision>`,
successful `rollout status`, final image equality, and final ready replicas equal desired replicas.

| Deployment | Pre image | Pre revision | Restart revision | Undo target revision | Final revision | Final image | Ready | Desired | Restart status | Undo status | Note |
|---|---|---:|---:|---:|---:|---|---:|---:|---|---|---|
| `audit-compliance-service` | `localhost:5000/nexuspaas-backend:ci-ga-pdp-scope-20260620163744` | 8 | 9 | 8 | 10 | `localhost:5000/nexuspaas-backend:ci-ga-pdp-scope-20260620163744` | 1 | 1 | succeeded | succeeded | Same-image controller rollback-path evidence only. |
| `authorization-policy-service` | `localhost:5000/nexuspaas-backend:ci-ga-admin-policy-20260621020259` | 10 | 11 | 10 | 12 | `localhost:5000/nexuspaas-backend:ci-ga-admin-policy-20260621020259` | 1 | 1 | succeeded | succeeded | Same-image controller rollback-path evidence only. |
| `ide-service` | `localhost:5000/nexuspaas-backend:ci-ga-pdp-scope-20260620163744` | 8 | 9 | 8 | 10 | `localhost:5000/nexuspaas-backend:ci-ga-pdp-scope-20260620163744` | 1 | 1 | succeeded | succeeded | Same-image controller rollback-path evidence only. |
| `identity-service` | `localhost:5000/nexuspaas-backend:ci-ga-pdp-scope-20260620163744` | 8 | 9 | 8 | 10 | `localhost:5000/nexuspaas-backend:ci-ga-pdp-scope-20260620163744` | 1 | 1 | succeeded | succeeded | Same-image controller rollback-path evidence only. |
| `image-registry-service` | `localhost:5000/nexuspaas-backend:ci-ga-clusterread-static-admin-20260621132623` | 9 | 10 | 9 | 11 | `localhost:5000/nexuspaas-backend:ci-ga-clusterread-static-admin-20260621132623` | 1 | 1 | succeeded | succeeded | Same-image controller rollback-path evidence only. |
| `integration-proxy-service` | `localhost:5000/nexuspaas-backend:ci-ga-pdp-scope-20260620163744` | 8 | 9 | 8 | 10 | `localhost:5000/nexuspaas-backend:ci-ga-pdp-scope-20260620163744` | 1 | 1 | succeeded | succeeded | Same-image controller rollback-path evidence only. |
| `k8s-control-service` | `localhost:5000/nexuspaas-backend:ci-ga-pdp-scope-20260620163744` | 8 | 9 | 8 | 10 | `localhost:5000/nexuspaas-backend:ci-ga-pdp-scope-20260620163744` | 1 | 1 | succeeded | succeeded | Same-image controller rollback-path evidence only. |
| `media-upload-service` | `localhost:5000/nexuspaas-backend:ci-ga-pdp-scope-20260620163744` | 9 | 10 | 9 | 11 | `localhost:5000/nexuspaas-backend:ci-ga-pdp-scope-20260620163744` | 1 | 1 | succeeded | succeeded | Same-image controller rollback-path evidence only. |
| `org-project-service` | `localhost:5000/nexuspaas-backend:ci-ga-org-static-admin-20260621125516` | 9 | 10 | 9 | 11 | `localhost:5000/nexuspaas-backend:ci-ga-org-static-admin-20260621125516` | 1 | 1 | succeeded | succeeded | Same-image controller rollback-path evidence only. |
| `platform-gateway` | `localhost:5000/nexuspaas-backend:ci-ga-web-job-logs-20260621143553` | 21 | 22 | 21 | 23 | `localhost:5000/nexuspaas-backend:ci-ga-web-job-logs-20260621143553` | 1 | 1 | succeeded | succeeded | Same-image controller rollback-path evidence only. |
| `request-notification-service` | `localhost:5000/nexuspaas-backend:ci-ga-pdp-scope-20260620163744` | 7 | 8 | 7 | 9 | `localhost:5000/nexuspaas-backend:ci-ga-pdp-scope-20260620163744` | 1 | 1 | succeeded | succeeded | Same-image controller rollback-path evidence only. |
| `scheduler-quota-service` | `localhost:5000/nexuspaas-backend:ci-ga-pdp-scope-20260620163744` | 7 | 8 | 7 | 9 | `localhost:5000/nexuspaas-backend:ci-ga-pdp-scope-20260620163744` | 1 | 1 | succeeded | succeeded | Same-image controller rollback-path evidence only. |
| `storage-service` | `localhost:5000/nexuspaas-backend:ci-ga-pdp-scope-20260620163744` | 7 | 8 | 7 | 9 | `localhost:5000/nexuspaas-backend:ci-ga-pdp-scope-20260620163744` | 1 | 1 | succeeded | succeeded | Same-image controller rollback-path evidence only. |
| `usage-observability-service` | `localhost:5000/nexuspaas-backend:ci-ga-clusterread-static-admin-20260621132623` | 8 | 9 | 8 | 10 | `localhost:5000/nexuspaas-backend:ci-ga-clusterread-static-admin-20260621132623` | 1 | 1 | succeeded | succeeded | Same-image controller rollback-path evidence only. |
| `workload-service` | `localhost:5000/nexuspaas-backend:ci-ga-pdp-scope-20260620163744` | 7 | 8 | 7 | 9 | `localhost:5000/nexuspaas-backend:ci-ga-pdp-scope-20260620163744` | 1 | 1 | succeeded | succeeded | Same-image controller rollback-path evidence only. |

Final readback:

| Deployment | Ready | Desired | Image |
|---|---:|---:|---|
| `audit-compliance-service` | 1 | 1 | `localhost:5000/nexuspaas-backend:ci-ga-pdp-scope-20260620163744` |
| `authorization-policy-service` | 1 | 1 | `localhost:5000/nexuspaas-backend:ci-ga-admin-policy-20260621020259` |
| `ide-service` | 1 | 1 | `localhost:5000/nexuspaas-backend:ci-ga-pdp-scope-20260620163744` |
| `identity-service` | 1 | 1 | `localhost:5000/nexuspaas-backend:ci-ga-pdp-scope-20260620163744` |
| `image-registry-service` | 1 | 1 | `localhost:5000/nexuspaas-backend:ci-ga-clusterread-static-admin-20260621132623` |
| `integration-proxy-service` | 1 | 1 | `localhost:5000/nexuspaas-backend:ci-ga-pdp-scope-20260620163744` |
| `k8s-control-service` | 1 | 1 | `localhost:5000/nexuspaas-backend:ci-ga-pdp-scope-20260620163744` |
| `media-upload-service` | 1 | 1 | `localhost:5000/nexuspaas-backend:ci-ga-pdp-scope-20260620163744` |
| `org-project-service` | 1 | 1 | `localhost:5000/nexuspaas-backend:ci-ga-org-static-admin-20260621125516` |
| `platform-gateway` | 1 | 1 | `localhost:5000/nexuspaas-backend:ci-ga-web-job-logs-20260621143553` |
| `request-notification-service` | 1 | 1 | `localhost:5000/nexuspaas-backend:ci-ga-pdp-scope-20260620163744` |
| `scheduler-quota-service` | 1 | 1 | `localhost:5000/nexuspaas-backend:ci-ga-pdp-scope-20260620163744` |
| `storage-service` | 1 | 1 | `localhost:5000/nexuspaas-backend:ci-ga-pdp-scope-20260620163744` |
| `usage-observability-service` | 1 | 1 | `localhost:5000/nexuspaas-backend:ci-ga-clusterread-static-admin-20260621132623` |
| `workload-service` | 1 | 1 | `localhost:5000/nexuspaas-backend:ci-ga-pdp-scope-20260620163744` |

This is partial OPS evidence. It does not close 8-unit Production Beta staging rollback,
previous-image rollback, backup/restore, failure injection, or full critical-path live E2E.
