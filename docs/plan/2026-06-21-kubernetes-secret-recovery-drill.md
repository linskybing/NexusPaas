# Kubernetes Secret Recovery Drill

Status: Approved

## 1. Objective

Capture live OPS-009 evidence that current Kubernetes runtime Secrets can be recovered without
disclosing secret values by recreating temporary copy Secrets from selected source Secrets,
verifying `.data` equality internally, and deleting every temporary copy.

## 2. Background

`docs/acceptance/operations.md` requires OPS-009: secret recovery procedure is documented and
tested. The current live `nexuspaas` namespace stores runtime credentials as Kubernetes `Secret`
objects. The production-beta runtime secret contract says operators should create Kubernetes
Secrets or ExternalSecret-managed Secrets before applying the kustomization and that secret values
must not be placed in the documentation ConfigMap.

Official documentation reference:

- Kubernetes Secrets: https://kubernetes.io/docs/concepts/configuration/secret/

Kubernetes documentation notes that Secret `.data` values are base64-encoded, which is not a
confidentiality boundary. This drill must therefore avoid printing `.data`, decoded values, or
hashes derived from low-entropy secret material.

## 3. Source References

- `docs/acceptance/operations.md`
- `docs/acceptance/cncf-adoption.md`
- `backend/deploy/k3s/production-beta/runtime-secret-contract.yaml`
- Live `production-beta-runtime-secret-contract` ConfigMap
- Live `nexuspaas` deployment secret references from `kubectl`
- `gap.md`
- `problem.md`

## 4. Assumptions

- The current `kubectl` context is the same live local/RKE2-style context used by prior GA
  evidence slices.
- `jq` is available locally for structured JSON transformation.
- The selected source Secrets exist before the drill starts.
- Temporary copy Secrets are not referenced by any workload.
- This is a current-live Kubernetes Secret recovery drill, not a managed external secret-store
  recovery drill.

## 5. Non-Goals

- Do not print Secret `.data`, decoded values, or hashes of secret values.
- Do not delete or mutate any source Secret.
- Do not restart workloads.
- Do not claim Vault, External Secrets Operator, Sealed Secrets, SOPS, KMS, off-cluster backup,
  rotation, revocation, or full GA secret-management maturity.
- Do not fix the current live Dex deployment's `*-dev-*` secret references in this slice.

## 6. Current Behavior

OPS-009 is not evidenced in the GA ledgers. The live namespace has deployment-referenced runtime
Secrets, and read-only inventory confirms their names and key names. A prior exploratory command in
this session accidentally printed base64 `.data`; no repository file was written with those values.
This plan explicitly forbids that pattern and uses key-only inventory plus internal equality checks.

## 7. Target Behavior

For each selected source Secret:

1. Create a temporary copy Secret named `ops009-<timestamp>-<index>` from the source Secret's
   `type` and `.data` fields through a `jq | kubectl create -f -` pipeline.
2. Label and annotate the copy with drill metadata and the source Secret name.
3. Verify the copy Secret's `.data` equals the source Secret's `.data` using `jq -e` without
   printing either value.
4. Record only source name, temporary copy name, key names, key count, equality result, and cleanup
   result.
5. Delete the temporary copy Secret.
6. Confirm no `nexuspaas.io/ops009-stamp=<timestamp>` Secrets remain.

Ledgers will mark OPS-009 as current-live Kubernetes Secret recovery copy evidence while preserving
managed/off-cluster secret recovery gaps.

## 8. Affected Domains

- OPS-009 secret recovery procedure
- Kubernetes runtime Secret operations
- GA problem and gap tracking

## 9. Affected Files

- `docs/plan/2026-06-21-kubernetes-secret-recovery-drill.md`
- `gap.md`
- `problem.md`

## 10. API / Contract Changes

None.

## 11. Database / Migration Changes

None.

## 12. Configuration Changes

No persistent configuration changes. Temporary copy Secrets are created and deleted in the live
namespace.

## 13. Observability Changes

No permanent observability changes. Evidence will be collected from:

- `kubectl config current-context`
- `kubectl -n nexuspaas get deploy -o json` secret-reference inventory
- `kubectl -n nexuspaas get secret -o json` key-only inventory
- `jq -e` equality checks that do not print `.data`
- cleanup readbacks for the temporary Secret label selector

The full evidence table will be appended to this plan under `## 21. Implementation Evidence`.

## 14. Security Considerations

- Do not run commands that print `.data`, such as raw `kubectl get secret -o yaml/json`, unfiltered
  custom-columns on `.data`, or decoded value commands.
- Do not print secret-value hashes; hashes of local/dev default secrets can be brute-forced.
- Do not write Secret manifests to repo files.
- Keep Secret manifests inside local process pipelines only.
- Use `kubectl create -f -`, not client-side `kubectl apply`, so Kubernetes does not add a
  `kubectl.kubernetes.io/last-applied-configuration` annotation containing `.data`.
- Label every temporary copy Secret with `nexuspaas.io/ops009-drill=true` and
  `nexuspaas.io/ops009-stamp=<timestamp>` for exact cleanup.
- Delete temporary copy Secrets even on failure.

## 15. Implementation Steps

1. Capture preflight:
   - `kubectl config current-context`
   - namespace: `nexuspaas`
   - local `jq --version`
   - live deployment secret-reference inventory with Secret names only
   - selected source Secret key-only inventory with key names and key counts only
2. Select current-live critical Secrets:
   - deployment-referenced runtime Secrets
   - `postgres-password`
   - `minio-credentials`
   - `coturn-runtime-secret`
   - live Dex-referenced `dex-dev-password` and `postgres-dev-password`
   - contract-present `dex-password`
3. Abort if any selected source Secret is missing or if any temporary copy Secret for the timestamp
   already exists.
4. For each selected source Secret, build a minimal Secret JSON object:
   - preserve `apiVersion`, `kind`, `type`, and `.data`
   - set `metadata.name` to `ops009-<timestamp>-<index>`
   - set `metadata.namespace` to `nexuspaas`
   - set labels/annotations for drill stamp and source Secret name
   - remove all source metadata except namespace-derived context
5. Create the temporary copy Secret with `kubectl create -f -`; do not use client-side
   `kubectl apply`.
6. Compare source and copy `.data` with `jq -e` and suppress output.
7. Verify the temporary copy Secret does not have a
   `kubectl.kubernetes.io/last-applied-configuration` annotation.
8. Record key names/count, `data_equal=true`, and `last_applied_annotation=false`; do not record
   values or hashes.
9. Delete the temporary copy Secret.
10. Confirm no temporary copy Secrets remain for the drill stamp.
11. Update `gap.md` and `problem.md`: OPS-009 current-live Kubernetes Secret recovery copy drill is
    evidenced; Vault/ExternalSecrets/off-cluster backup, rotation/revocation, removal of live dev
    secret references, and full GA secret-management maturity remain open.
12. Submit the evidence and ledger updates to Reviewer Agent.

## 16. Verification Plan

- Preflight prints context, namespace, `jq` version, deployment secret-reference names, and selected
  key-only inventory.
- No command output includes Secret `.data`, decoded values, or value hashes.
- Every temporary copy Secret is created successfully.
- Every copy Secret `.data` equals its source Secret `.data` by a suppressed `jq -e` equality check.
- Every temporary copy Secret lacks `kubectl.kubernetes.io/last-applied-configuration`.
- Every temporary copy Secret is deleted.
- Final label-selector readback returns zero temporary copy Secrets.
- Source Secrets still exist after the drill.
- `git diff --check` passes after ledger edits.
- Not applicable for this docs/live-evidence-only slice because no application code, tests, build
  scripts, manifests, or runtime config are changed: `go -C backend test ./...`, `npm --prefix
  frontend run test`, `npm --prefix frontend run build`, and SonarScanner. If the scope expands
  beyond docs/evidence, these gates must run.

## 17. Rollback Plan

If any copy/apply/equality/delete step fails:

1. Stop the drill.
2. Delete temporary Secrets matching `nexuspaas.io/ops009-stamp=<timestamp>`.
3. Verify source Secrets still exist.
4. Capture only the failing stage, source Secret name, copy Secret name, key names, and non-secret
   error output.
5. Do not delete or mutate any source Secret.

## 18. Risks and Tradeoffs

- The drill temporarily duplicates real secret material inside the same namespace; labels and
  immediate cleanup limit exposure duration.
- This proves Kubernetes Secret object recovery mechanics for current live Secrets, not external
  secret manager restore, off-cluster backup, rotation, revocation, or KMS-backed recovery.
- Current live Dex references `dex-dev-password` and `postgres-dev-password`; this drill can prove
  recovery mechanics for those referenced Secrets but does not make the live topology GA-grade.

## 19. Reviewer Checklist

- The plan never prints Secret values, `.data`, decoded values, or value hashes.
- The plan never deletes or mutates source Secrets.
- Temporary copy Secret names are bounded and exact.
- Temporary copy Secrets are labeled for cleanup and verified absent.
- Temporary copy Secrets are created with `kubectl create -f -`, not client-side apply, and have no
  last-applied annotation.
- The evidence contract is enough to verify current-live OPS-009 recovery mechanics.
- Ledger updates preserve managed/off-cluster secret recovery and live dev-secret-reference gaps.

## 20. Status

Status: Approved. Reviewer Agent approved the revised plan before live Secret copy commands ran.

## 21. Implementation Evidence

Executed on 2026-06-21 against `kubectl config current-context=default`, namespace
`nexuspaas`, with `jq-1.8.1`.

The drill selected 21 current-live critical Secrets, created one temporary copy Secret per source
with `kubectl create -f -`, verified `.data` equality with suppressed command output, verified the
temporary copy did not have `kubectl.kubernetes.io/last-applied-configuration`, deleted each
temporary copy, and confirmed no `nexuspaas.io/ops009-stamp=20260621153156` or
`nexuspaas.io/ops009-drill=true` Secrets remained. Independent readback confirmed all 21 source
Secrets still existed.

No Secret `.data`, decoded values, or value hashes were recorded in this evidence.

| Source Secret | Temporary Copy | Key Count | Keys | Data Equal | Last-Applied Annotation | Copy Deleted |
|---|---|---:|---|---|---|---|
| `audit-compliance-service-runtime-secret` | `ops009-20260621153156-01` | 6 | `API_KEYS,API_KEY_PRINCIPALS,AUTHORIZATION_POLICY_API_KEY,AUTHORIZATION_POLICY_URL,DATABASE_URL,SERVICE_API_KEY` | true | false | true |
| `authorization-policy-service-runtime-secret` | `ops009-20260621153156-02` | 4 | `API_KEYS,API_KEY_PRINCIPALS,DATABASE_URL,SERVICE_API_KEY` | true | false | true |
| `coturn-runtime-secret` | `ops009-20260621153156-03` | 2 | `TURN_REALM,TURN_STATIC_AUTH_SECRET` | true | false | true |
| `dex-dev-password` | `ops009-20260621153156-04` | 1 | `bcrypt-hash` | true | false | true |
| `dex-password` | `ops009-20260621153156-05` | 1 | `bcrypt-hash` | true | false | true |
| `ide-service-runtime-secret` | `ops009-20260621153156-06` | 6 | `API_KEYS,API_KEY_PRINCIPALS,AUTHORIZATION_POLICY_API_KEY,AUTHORIZATION_POLICY_URL,DATABASE_URL,SERVICE_API_KEY` | true | false | true |
| `identity-service-runtime-secret` | `ops009-20260621153156-07` | 6 | `API_KEYS,API_KEY_PRINCIPALS,AUTHORIZATION_POLICY_API_KEY,AUTHORIZATION_POLICY_URL,DATABASE_URL,SERVICE_API_KEY` | true | false | true |
| `image-registry-service-runtime-secret` | `ops009-20260621153156-08` | 6 | `API_KEYS,API_KEY_PRINCIPALS,AUTHORIZATION_POLICY_API_KEY,AUTHORIZATION_POLICY_URL,DATABASE_URL,SERVICE_API_KEY` | true | false | true |
| `integration-proxy-service-runtime-secret` | `ops009-20260621153156-09` | 6 | `API_KEYS,API_KEY_PRINCIPALS,AUTHORIZATION_POLICY_API_KEY,AUTHORIZATION_POLICY_URL,DATABASE_URL,SERVICE_API_KEY` | true | false | true |
| `k8s-control-service-runtime-secret` | `ops009-20260621153156-10` | 6 | `API_KEYS,API_KEY_PRINCIPALS,AUTHORIZATION_POLICY_API_KEY,AUTHORIZATION_POLICY_URL,DATABASE_URL,SERVICE_API_KEY` | true | false | true |
| `media-upload-service-runtime-secret` | `ops009-20260621153156-11` | 10 | `API_KEYS,API_KEY_PRINCIPALS,AUTHORIZATION_POLICY_API_KEY,AUTHORIZATION_POLICY_URL,DATABASE_URL,EVENT_BUS_URL,OBJECT_STORE_ACCESS_KEY,OBJECT_STORE_SECRET_KEY,REDIS_URL,SERVICE_API_KEY` | true | false | true |
| `minio-credentials` | `ops009-20260621153156-12` | 2 | `access-key,secret-key` | true | false | true |
| `org-project-service-runtime-secret` | `ops009-20260621153156-13` | 6 | `API_KEYS,API_KEY_PRINCIPALS,AUTHORIZATION_POLICY_API_KEY,AUTHORIZATION_POLICY_URL,DATABASE_URL,SERVICE_API_KEY` | true | false | true |
| `platform-gateway-runtime-secret` | `ops009-20260621153156-14` | 6 | `API_KEYS,API_KEY_PRINCIPALS,AUTHORIZATION_POLICY_API_KEY,AUTHORIZATION_POLICY_URL,DATABASE_URL,SERVICE_API_KEY` | true | false | true |
| `postgres-dev-password` | `ops009-20260621153156-15` | 1 | `password` | true | false | true |
| `postgres-password` | `ops009-20260621153156-16` | 1 | `password` | true | false | true |
| `request-notification-service-runtime-secret` | `ops009-20260621153156-17` | 6 | `API_KEYS,API_KEY_PRINCIPALS,AUTHORIZATION_POLICY_API_KEY,AUTHORIZATION_POLICY_URL,DATABASE_URL,SERVICE_API_KEY` | true | false | true |
| `scheduler-quota-service-runtime-secret` | `ops009-20260621153156-18` | 6 | `API_KEYS,API_KEY_PRINCIPALS,AUTHORIZATION_POLICY_API_KEY,AUTHORIZATION_POLICY_URL,DATABASE_URL,SERVICE_API_KEY` | true | false | true |
| `storage-service-runtime-secret` | `ops009-20260621153156-19` | 6 | `API_KEYS,API_KEY_PRINCIPALS,AUTHORIZATION_POLICY_API_KEY,AUTHORIZATION_POLICY_URL,DATABASE_URL,SERVICE_API_KEY` | true | false | true |
| `usage-observability-service-runtime-secret` | `ops009-20260621153156-20` | 6 | `API_KEYS,API_KEY_PRINCIPALS,AUTHORIZATION_POLICY_API_KEY,AUTHORIZATION_POLICY_URL,DATABASE_URL,SERVICE_API_KEY` | true | false | true |
| `workload-service-runtime-secret` | `ops009-20260621153156-21` | 6 | `API_KEYS,API_KEY_PRINCIPALS,AUTHORIZATION_POLICY_API_KEY,AUTHORIZATION_POLICY_URL,DATABASE_URL,SERVICE_API_KEY` | true | false | true |

Final cleanup readback:

- temporary Secrets with `nexuspaas.io/ops009-stamp=20260621153156`: `0`
- temporary Secrets with `nexuspaas.io/ops009-drill=true`: `0`
- selected source Secrets still present: `21`

This closes current-live Kubernetes Secret recovery copy evidence only. Vault/External Secrets
Operator/Sealed Secrets/SOPS/KMS/off-cluster backup, rotation, revocation, removal of current live
`*-dev-*` secret references, and full GA secret-management maturity remain open.
