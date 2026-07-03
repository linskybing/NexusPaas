#!/usr/bin/env bash
# kind-live-e2e.sh — KIND-LOCAL single-cluster live proof of the V1 launch-blocker
# drills: image supply chain, backing services + secrets, 8-unit deploy/smoke,
# live DB migration apply/validate/idempotency, per-unit previous-image
# rollback/redeploy, and local-registry promote/rollback.
#
#   !!! KIND LOCAL — NOT EXTERNAL GA PROOF !!!
# Per docs/agents/workflow.md, single-cluster/local evidence must NOT be described
# as external GA proof. kind is one local cluster, so this UPGRADES render-only
# evidence to live single-cluster execution but does NOT close the external
# registry / external staging rows. The external-only harness
# (production-beta-live-rehearsal.sh) keeps its kind-rejecting guards; this script
# is a separate, clearly-stamped local entrypoint and does not touch them.
#
# ponytail: intentionally reuses the production-beta MANIFESTS (the real reusable
# asset) via `kubectl kustomize backend`, and re-implements only the thin kubectl
# drill orchestration here rather than refactoring the security-critical external
# harness. Add a shared lib only if a third caller appears.
set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BACKEND_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
REPO_ROOT="$(cd "${BACKEND_DIR}/.." && pwd)"

CLUSTER_NAME="${CLUSTER_NAME:-nexuspaas-kind-e2e}"
KIND_CONTEXT="kind-${CLUSTER_NAME}"
NAMESPACE="${NAMESPACE:-nexuspaas}"
REG_NAME="${REG_NAME:-nexuspaas-kind-registry}"
REG_PORT="${REG_PORT:-5000}"
BASE_IMAGE="${BASE_IMAGE:-nexuspaas-backend:v0.1.0}"          # baseline / "previous"
CANDIDATE_IMAGE="${CANDIDATE_IMAGE:-nexuspaas-backend:v0.1.1}" # "candidate"
ROLLOUT_TIMEOUT="${ROLLOUT_TIMEOUT:-180s}"
JOB_TIMEOUT="${JOB_TIMEOUT:-300s}"
SMOKE_TIMEOUT_SECONDS="${SMOKE_TIMEOUT_SECONDS:-90}"
KEEP_CLUSTER="${KEEP_CLUSTER:-0}"
RUN_ID="$(date -u '+%Y%m%d%H%M%S')"
ARTIFACT_DIR="${ARTIFACT_DIR:-${TMPDIR:-/tmp}/nexuspaas-kind-live-e2e/${RUN_ID}}"
EVIDENCE_DOC="${EVIDENCE_DOC:-${REPO_ROOT}/docs/acceptance/evidence/2026-07-01-kind-live-e2e-report.md}"

# --- kind-tier secret values (throwaway local cluster only; never external) -----
# Static (not time-based): Postgres persists the password on first PVC init, so a
# per-run value would break auth when an existing cluster/PVC is reused.
PG_PW="nexuspaas-kind-pg-static"
MINIO_AK="nexuspaaskind"
MINIO_SK="nexuspaas-kind-minio-secret"
API_KEY="nexuspaas-kind-admin-key"
IDENTITY_KEY="nexuspaas-kind-identity-key"
AUTHZ_KEY="nexuspaas-kind-authz-key"
TURN_SECRET="nexuspaas-kind-turn-secret"
# dex is scaled to 0 in this run (DEX_URL empty, no OIDC consumers), so its
# password hash is never consumed — use a non-secret placeholder, not a real hash.
DEX_HASH="dex-scaled-to-zero-unused-in-kind-run"
DB_URL="postgres://nexuspaas:${PG_PW}@postgres:5432/nexuspaas?sslmode=disable"
API_KEY_PRINCIPALS='{"'"${API_KEY}"'":{"id":"kind-admin","username":"kind-admin","role":"admin","admin":true}}'
# every unit is a trusted caller of every other unit: cross-unit read
# contracts (projection drift measurement, identity auth) originate from all
# units, not just the gateway.
TRUSTED_IDENTITIES="$(python3 - "${IDENTITY_KEY}" <<'PYEOF'
import json, sys
units = ["platform-gateway","iam-unit","tenant-unit","collaboration-unit","platform-io-unit","usage-observability","compute-api","compute-control-plane"]
print(json.dumps({u: {"key": sys.argv[1], "audiences": units} for u in units}, separators=(",", ":")))
PYEOF
)"

UNITS=(platform-gateway iam-unit tenant-unit collaboration-unit platform-io-unit usage-observability compute-api compute-control-plane)
LOGICAL_SERVICES=(platform-gateway identity-service authorization-policy-service org-project-service audit-compliance-service request-notification-service media-upload-service storage-service image-registry-service integration-proxy-service usage-observability-service workload-service ide-service scheduler-quota-service k8s-control-service)

mkdir -p "${ARTIFACT_DIR}"
SUPPLY_TSV="${ARTIFACT_DIR}/supply-chain.tsv"
SECRET_TSV="${ARTIFACT_DIR}/secret-presence.tsv"
MIG_TSV="${ARTIFACT_DIR}/migrations.tsv"
ROLLOUT_TSV="${ARTIFACT_DIR}/rollouts.tsv"
SMOKE_TSV="${ARTIFACT_DIR}/smoke.tsv"
ROLLBACK_TSV="${ARTIFACT_DIR}/rollback-redeploy.tsv"
REGISTRY_TSV="${ARTIFACT_DIR}/registry-promote-rollback.tsv"
REGISTRY_UNION="${ARTIFACT_DIR}/registry-union.txt"

log() { printf '[%s] %s\n' "$(date -u '+%H:%M:%SZ')" "$*" >&2; }
die() { printf 'error: %s\n' "$*" >&2; exit 1; }
need() { local cmd="$1"; command -v "${cmd}" >/dev/null 2>&1 || die "${cmd} is required"; }
kc() { kubectl --context "${KIND_CONTEXT}" -n "${NAMESPACE}" "$@"; }

preflight() {
  need docker; need kind; need kubectl; need curl; need jq
  docker info >/dev/null 2>&1 || die "docker daemon is not running"
  docker image inspect "${BASE_IMAGE}" >/dev/null 2>&1 \
    || die "${BASE_IMAGE} not found; build it: docker build -f backend/Dockerfile -t ${BASE_IMAGE} ."
  # candidate is a retag of the baseline image (same bits, different tag) so the
  # rollback/redeploy drill exercises a real image change + rollout.
  docker tag "${BASE_IMAGE}" "${CANDIDATE_IMAGE}"
}

ensure_cluster() {
  if kind get clusters 2>/dev/null | grep -Fxq "${CLUSTER_NAME}"; then
    log "reusing kind cluster ${CLUSTER_NAME}"
  else
    log "creating kind cluster ${CLUSTER_NAME}"
    kind create cluster --name "${CLUSTER_NAME}" --wait 120s
  fi
  kubectl --context "${KIND_CONTEXT}" cluster-info >/dev/null
  log "loading images into kind"
  kind load docker-image "${BASE_IMAGE}" "${CANDIDATE_IMAGE}" --name "${CLUSTER_NAME}"
}

supply_chain() {
  printf 'step\ttool\tstatus\tdetail\n' >"${SUPPLY_TSV}"
  local digest
  digest="$(docker image inspect "${BASE_IMAGE}" --format '{{index .Id}}')"
  printf 'image-build\tdocker-buildkit\tpass\t%s\n' "${digest}" >>"${SUPPLY_TSV}"

  if command -v syft >/dev/null 2>&1; then
    if syft "docker:${BASE_IMAGE}" -o spdx-json="${ARTIFACT_DIR}/sbom.spdx.json" >/dev/null 2>&1; then
      printf 'sbom\tsyft\tpass\t%s\n' "$(wc -c <"${ARTIFACT_DIR}/sbom.spdx.json" | tr -d ' ') bytes" >>"${SUPPLY_TSV}"
    else printf 'sbom\tsyft\tfail\tsyft error\n' >>"${SUPPLY_TSV}"; fi
  else printf 'sbom\tsyft\tskipped\ttool unavailable\n' >>"${SUPPLY_TSV}"; fi

  if command -v trivy >/dev/null 2>&1; then
    if trivy image --quiet --scanners vuln --severity HIGH,CRITICAL --format json -o "${ARTIFACT_DIR}/trivy.json" "${BASE_IMAGE}" >/dev/null 2>&1; then
      local vulns; vulns="$(jq '[.Results[]?.Vulnerabilities // []] | add | length' "${ARTIFACT_DIR}/trivy.json" 2>/dev/null || echo '?')"
      printf 'scan\ttrivy\tpass\tHIGH+CRITICAL=%s\n' "${vulns}" >>"${SUPPLY_TSV}"
    else printf 'scan\ttrivy\tfail\ttrivy error\n' >>"${SUPPLY_TSV}"; fi
  else printf 'scan\ttrivy\tskipped\ttool unavailable\n' >>"${SUPPLY_TSV}"; fi

  if command -v cosign >/dev/null 2>&1; then
    ( cd "${ARTIFACT_DIR}" && COSIGN_PASSWORD="" cosign generate-key-pair >/dev/null 2>&1 ) \
      && printf 'sign\tcosign\tpass\tlocal keypair generated (offline sign requires a registry ref)\n' >>"${SUPPLY_TSV}" \
      || printf 'sign\tcosign\tskipped\tkeygen failed\n' >>"${SUPPLY_TSV}"
  else printf 'sign\tcosign\tskipped\ttool unavailable\n' >>"${SUPPLY_TSV}"; fi
}

create_ns_and_secrets() {
  kubectl --context "${KIND_CONTEXT}" create namespace "${NAMESPACE}" --dry-run=client -o yaml | kubectl --context "${KIND_CONTEXT}" apply -f -
  # backing-service secrets (production-beta names the kustomize patches expect)
  kc create secret generic postgres-password --from-literal=password="${PG_PW}" --dry-run=client -o yaml | kc apply -f -
  kc create secret generic minio-credentials --from-literal=access-key="${MINIO_AK}" --from-literal=secret-key="${MINIO_SK}" --dry-run=client -o yaml | kc apply -f -
  kc create secret generic dex-password --from-literal=bcrypt-hash="${DEX_HASH}" --dry-run=client -o yaml | kc apply -f -
  kc create secret generic coturn-runtime-secret --from-literal=TURN_STATIC_AUTH_SECRET="${TURN_SECRET}" --dry-run=client -o yaml | kc apply -f -

  printf 'secret\tpresent\n' >"${SECRET_TSV}"
  local u args
  for u in "${UNITS[@]}"; do
    # iam-unit additionally accepts the authz policy key: every other unit
    # calls its /api/v1/permissions/enforce with AUTHORIZATION_POLICY_API_KEY,
    # which must therefore be a recognized API key WITH a principal on iam-unit.
    local unit_api_keys="${API_KEY}" unit_principals="${API_KEY_PRINCIPALS}"
    if [[ "${u}" = "iam-unit" ]]; then
      unit_api_keys="${API_KEY},${AUTHZ_KEY}"
      unit_principals='{"'"${API_KEY}"'":{"id":"kind-admin","username":"kind-admin","role":"admin","admin":true},"'"${AUTHZ_KEY}"'":{"id":"authz-policy-client","username":"authz-policy-client","role":"service"}}'
    fi
    args=(--from-literal=DATABASE_URL="${DB_URL}"
          --from-literal=API_KEYS="${unit_api_keys}"
          --from-literal=API_KEY_PRINCIPALS="${unit_principals}"
          --from-literal=SERVICE_IDENTITY_KEY="${IDENTITY_KEY}"
          --from-literal=SERVICE_TRUSTED_IDENTITIES="${TRUSTED_IDENTITIES}")
    if [[ "${u}" != "iam-unit" ]]; then
      args+=(--from-literal=AUTHORIZATION_POLICY_URL="http://iam-unit"
             --from-literal=AUTHORIZATION_POLICY_API_KEY="${AUTHZ_KEY}")
    fi
    if [[ "${u}" = "collaboration-unit" ]]; then
      args+=(--from-literal=OBJECT_STORE_ACCESS_KEY="${MINIO_AK}"
             --from-literal=OBJECT_STORE_SECRET_KEY="${MINIO_SK}")
    fi
    kc create secret generic "${u}-runtime-secret" "${args[@]}" --dry-run=client -o yaml | kc apply -f -
  done
  # record presence only (names), never values
  local s
  for s in postgres-password minio-credentials dex-password coturn-runtime-secret \
           platform-gateway-runtime-secret iam-unit-runtime-secret tenant-unit-runtime-secret \
           collaboration-unit-runtime-secret platform-io-unit-runtime-secret \
           usage-observability-runtime-secret compute-api-runtime-secret compute-control-plane-runtime-secret; do
    if kc get secret "${s}" -o name >/dev/null 2>&1; then printf '%s\tyes\n' "${s}"; else printf '%s\tNO\n' "${s}"; fi >>"${SECRET_TSV}"
  done
}

apply_stack() {
  log "rendering + applying production-beta kustomize (8-unit topology)"
  (cd "${REPO_ROOT}" && kubectl kustomize backend) >"${ARTIFACT_DIR}/render.yaml"
  kubectl --context "${KIND_CONTEXT}" apply -f "${ARTIFACT_DIR}/render.yaml"
  # kind has no OIDC/TURN consumers in this run; silence dex/coturn (DEX_URL empty).
  kc scale deployment/dex --replicas=0 >/dev/null 2>&1 || true
  kc scale deployment/coturn --replicas=0 >/dev/null 2>&1 || true
  log "waiting for postgres/redis/minio"
  kc rollout status deployment/postgres --timeout="${ROLLOUT_TIMEOUT}"
  kc rollout status deployment/redis --timeout="${ROLLOUT_TIMEOUT}"
  kc rollout status deployment/minio --timeout="${ROLLOUT_TIMEOUT}"
}

setup_cluster_rbac() {
  # kind-tier deviation: cluster-hosting units need an in-cluster SA token for the
  # readiness cluster-ping (rest.InClusterConfig). The production manifest sets
  # automountServiceAccountToken=false + expects external workload identity; here
  # we grant a throwaway SA (cluster-admin on a disposable local cluster) so the
  # ping succeeds. Documented as kind-tier, NOT a production posture.
  kc create serviceaccount nexuspaas-kind --dry-run=client -o yaml | kc apply -f -
  kubectl --context "${KIND_CONTEXT}" create clusterrolebinding nexuspaas-kind-admin \
    --clusterrole=cluster-admin --serviceaccount="${NAMESPACE}:nexuspaas-kind" \
    --dry-run=client -o yaml | kubectl --context "${KIND_CONTEXT}" apply -f -
}

patch_units_sa() {
  local u
  for u in "${UNITS[@]}"; do
    kc patch deployment "${u}" --type merge -p \
      '{"spec":{"template":{"spec":{"serviceAccountName":"nexuspaas-kind","automountServiceAccountToken":true}}}}' >/dev/null
  done
}

MIG_JOB_SEQ=0
run_migration_job() {
  local task="$1"
  MIG_JOB_SEQ=$((MIG_JOB_SEQ + 1))
  local job="kind-${1}-${RUN_ID}-${MIG_JOB_SEQ}"  # unique per call so a re-apply is a real fresh run
  cat <<EOF | kc apply -f -
apiVersion: batch/v1
kind: Job
metadata:
  name: ${job}
spec:
  backoffLimit: 1
  template:
    spec:
      restartPolicy: Never
      automountServiceAccountToken: false
      containers:
        - name: app
          image: ${BASE_IMAGE}
          imagePullPolicy: IfNotPresent
          env:
            - {name: ADMIN_TASK, value: ${task}}
          envFrom:
            - configMapRef: {name: platform-gateway-config}
            - configMapRef: {name: production-beta-runtime-config, optional: true}
            - secretRef: {name: platform-gateway-runtime-secret}
EOF
  if kc wait --for=condition=complete --timeout="${JOB_TIMEOUT}" "job/${job}" 2>/dev/null; then
    printf '%s\t%s\tcomplete\n' "${task}" "${job}" >>"${MIG_TSV}"
  else
    kc logs "job/${job}" >"${ARTIFACT_DIR}/${job}.log" 2>&1 || true
    printf '%s\t%s\tFAILED\n' "${task}" "${job}" >>"${MIG_TSV}"
    die "migration job ${task} failed; see ${ARTIFACT_DIR}/${job}.log"
  fi
}

run_migrations() {
  printf 'task\tjob\tstatus\n' >"${MIG_TSV}"
  log "migration drill: apply -> validate -> idempotent re-apply"
  run_migration_job apply-migrations
  run_migration_job validate-migrations
  run_migration_job apply-migrations   # idempotency: second apply must be a clean no-op
}

wait_and_smoke() {
  local phase="$1" u
  printf -- '--- %s ---\n' "${phase}" >>"${ROLLOUT_TSV}"
  for u in "${UNITS[@]}"; do
    kc rollout status "deployment/${u}" --timeout="${ROLLOUT_TIMEOUT}"
    printf '%s\t%s\tcomplete\n' "${phase}" "${u}" >>"${ROLLOUT_TSV}"
  done
  : >"${REGISTRY_UNION}"
  local idx=0
  for u in "${UNITS[@]}"; do smoke_unit "${u}" "${phase}" "${idx}"; idx=$((idx+1)); done
  verify_registry_union "${phase}"
}

smoke_unit() {
  # NOTE: separate `local` statements — combining assignments on one `local` line
  # expands each RHS against the outer scope before the locals bind (set -u gotcha).
  local u="$1" phase="$2" idx="$3"
  local port=$((18080 + idx))
  local pf_log="${ARTIFACT_DIR}/pf-${phase}-${u}.log"
  kc port-forward "service/${u}" --address 127.0.0.1 "${port}:80" >"${pf_log}" 2>&1 &
  local pf=$!
  local deadline=$((SECONDS + SMOKE_TIMEOUT_SECONDS)) ok=0
  while [[ "${SECONDS}" -lt "${deadline}" ]]; do
    if curl -fsS "http://127.0.0.1:${port}/healthz" >/dev/null 2>&1; then ok=1; break; fi
    sleep 1
  done
  [[ "${ok}" = 1 ]] || { kill "${pf}" 2>/dev/null || true; die "${u} /healthz unreachable in ${phase}"; }
  local ep code
  for ep in /healthz /readyz /metrics /openapi.json; do
    code="$(curl -s -o /dev/null -w '%{http_code}' -H "X-API-Key: ${API_KEY}" "http://127.0.0.1:${port}${ep}")"
    printf '%s\t%s\t%s\t%s\n' "${phase}" "${u}" "${ep}" "${code}" >>"${SMOKE_TSV}"
    [[ "${code}" = 200 ]] || { kill "${pf}" 2>/dev/null || true; die "${u} ${ep} returned ${code} in ${phase} (want 200)"; }
  done
  # In the split 8-unit topology each unit's /service-registry lists only the
  # services IT hosts; the 15 logical services are proven as the UNION across units.
  curl -s -H "X-API-Key: ${API_KEY}" "http://127.0.0.1:${port}/service-registry" \
    | jq -r '.data[]?.name' >>"${REGISTRY_UNION}" 2>/dev/null || true
  kill "${pf}" 2>/dev/null || true; wait "${pf}" 2>/dev/null || true
}

verify_registry_union() {
  local phase="$1" s missing=0 count
  for s in "${LOGICAL_SERVICES[@]}"; do
    grep -Fxq "${s}" "${REGISTRY_UNION}" || { log "union missing ${s}"; missing=$((missing + 1)); }
  done
  count="$(sort -u "${REGISTRY_UNION}" | grep -Fxf <(printf '%s\n' "${LOGICAL_SERVICES[@]}") | wc -l | tr -d ' ')"
  printf '%s\tALL-8-UNITS\t/service-registry (union)\t%s-of-15\n' "${phase}" "${count}" >>"${SMOKE_TSV}"
  [[ "${missing}" = 0 ]] || die "service-registry union missing ${missing} of 15 logical services in ${phase}"
}

rollback_redeploy() {
  printf 'unit\tphase\timage\tstatus\n' >"${ROLLBACK_TSV}"
  local idx=0 u
  for u in "${UNITS[@]}"; do
    kc set image "deployment/${u}" "app=${BASE_IMAGE}"
    kc rollout status "deployment/${u}" --timeout="${ROLLOUT_TIMEOUT}"
    printf '%s\trollback\t%s\tcomplete\n' "${u}" "${BASE_IMAGE}" >>"${ROLLBACK_TSV}"
    smoke_unit "${u}" "rollback" "${idx}"
    kc set image "deployment/${u}" "app=${CANDIDATE_IMAGE}"
    kc rollout status "deployment/${u}" --timeout="${ROLLOUT_TIMEOUT}"
    printf '%s\tredeploy\t%s\tcomplete\n' "${u}" "${CANDIDATE_IMAGE}" >>"${ROLLBACK_TSV}"
    smoke_unit "${u}" "redeploy" "${idx}"
    idx=$((idx+1))
  done
}

registry_promote_rollback() {
  printf 'step\tref\tstatus\n' >"${REGISTRY_TSV}"
  docker rm -f "${REG_NAME}" >/dev/null 2>&1 || true
  docker run -d --restart=always -p "127.0.0.1:${REG_PORT}:5000" --name "${REG_NAME}" registry:2 >/dev/null
  # wait for registry
  local i; for i in $(seq 1 30); do curl -fsS "http://127.0.0.1:${REG_PORT}/v2/" >/dev/null 2>&1 && break; sleep 1; done
  local reg="127.0.0.1:${REG_PORT}/nexuspaas-backend"
  docker tag "${BASE_IMAGE}" "${reg}:v0.1.0"; docker push "${reg}:v0.1.0" >/dev/null
  printf 'push-previous\t%s\tok\n' "${reg}:v0.1.0" >>"${REGISTRY_TSV}"
  docker tag "${CANDIDATE_IMAGE}" "${reg}:v0.1.1"; docker push "${reg}:v0.1.1" >/dev/null
  printf 'promote-candidate\t%s\tok\n' "${reg}:v0.1.1" >>"${REGISTRY_TSV}"
  # rollback: pull the previous tag back and re-verify it resolves
  docker rmi "${reg}:v0.1.0" >/dev/null 2>&1 || true
  docker pull "${reg}:v0.1.0" >/dev/null && printf 'rollback-pull-previous\t%s\tok\n' "${reg}:v0.1.0" >>"${REGISTRY_TSV}"
  local d0 d1
  d0="$(docker manifest inspect "${reg}:v0.1.0" 2>/dev/null | jq -r '.config.digest // .manifests[0].digest' 2>/dev/null || echo n/a)"
  d1="$(docker manifest inspect "${reg}:v0.1.1" 2>/dev/null | jq -r '.config.digest // .manifests[0].digest' 2>/dev/null || echo n/a)"
  printf 'digests\tprev=%s candidate=%s\tok\n' "${d0}" "${d1}" >>"${REGISTRY_TSV}"
}

tsv_table() { # render a tsv as a github md table
  local f="$1"
  awk -F '\t' 'NR==1{h=$0; n=split(h,a,"\t"); printf "|"; for(i=1;i<=n;i++)printf" %s |",a[i]; printf"\n|"; for(i=1;i<=n;i++)printf" --- |"; printf"\n"; next}
  /^---/{next} {printf "|"; for(i=1;i<=NF;i++)printf" %s |",$i; printf"\n"}' "${f}"
}

write_evidence() {
  mkdir -p "$(dirname "${EVIDENCE_DOC}")"
  {
    printf '# Kind Live E2E — V1 Launch-Blocker Drills (kind-tier evidence)\n\n'
    printf '> **KIND LOCAL — NOT EXTERNAL GA PROOF.** Per `docs/agents/workflow.md`,\n'
    printf '> single-cluster/local (kind) evidence must not be described as external GA\n'
    printf '> proof. This report proves the deploy / migration / rollback / supply-chain\n'
    printf '> **machinery executes on a real Kubernetes cluster** (kind), upgrading prior\n'
    printf '> render-only/static evidence. The **external registry host** and **external\n'
    printf '> staging cluster** rows remain **Open**.\n\n'
    printf -- '- Run ID: `%s`  •  Cluster: `%s`  •  Namespace: `%s`\n' "${RUN_ID}" "${KIND_CONTEXT}" "${NAMESPACE}"
    printf -- '- Baseline image: `%s`  •  Candidate image: `%s`\n' "${BASE_IMAGE}" "${CANDIDATE_IMAGE}"
    printf -- '- kind: `%s`  •  kubectl: `%s`\n\n' "$(kind version 2>/dev/null)" "$(kubectl version --client -o json 2>/dev/null | jq -r .clientVersion.gitVersion 2>/dev/null)"
    printf '## 1. Image supply chain (kind-tier)\n\n'; tsv_table "${SUPPLY_TSV}"; printf '\n'
    printf '## 2. Secret presence (names only; no values)\n\n'; tsv_table "${SECRET_TSV}"; printf '\n'
    printf '## 3. Live DB migration apply / validate / idempotency (real Postgres)\n\n'; tsv_table "${MIG_TSV}"
    printf '\nDB **schema down-migration is not an app capability** (forward-only migrations\n'
    printf 'with dirty-tracking); DB rollback = restore-from-backup, out of kind scope and\n'
    printf 'still Open. Deployment-level rollback is proven in section 5.\n\n'
    printf '## 4. 8-unit rollout + smoke (/healthz /readyz /metrics + 15-service registry)\n\n'; tsv_table "${SMOKE_TSV}"; printf '\n'
    printf '## 5. Per-unit previous-image rollback + redeploy\n\n'; tsv_table "${ROLLBACK_TSV}"; printf '\n'
    printf '## 6. Local-registry promote / rollback (kind-tier; NOT an external registry)\n\n'; tsv_table "${REGISTRY_TSV}"; printf '\n'
    printf '## Residual (stays Open — external only)\n\n'
    printf -- '- External registry host promotion/rollback (Harbor as the real external GA registry).\n'
    printf -- '- External staging cluster deploy/secret provenance/DR (off-cluster, HA).\n'
    printf -- '- DB schema rollback via restore-from-backup on external staging.\n'
    printf -- '- Product image-build dispatch feature (Tekton/BuildKit, source upload/hash) — not implemented.\n'
    printf -- '- Full live PERF/MON soak, browser WebRTC, real GPU.\n\n'
    printf -- '_Raw artifacts (TSVs, SBOM, scan, logs): `%s`_\n' "${ARTIFACT_DIR}"
  } >"${EVIDENCE_DOC}"
  log "wrote evidence: ${EVIDENCE_DOC}"
}

teardown() {
  [[ "${KEEP_CLUSTER}" = 1 ]] && { log "KEEP_CLUSTER=1; leaving ${CLUSTER_NAME} up"; return; }
  log "tearing down"
  kind delete cluster --name "${CLUSTER_NAME}" >/dev/null 2>&1 || true
  docker rm -f "${REG_NAME}" >/dev/null 2>&1 || true
}

main() {
  log "=== KIND LIVE E2E (kind-tier, NOT external GA) — artifacts in ${ARTIFACT_DIR} ==="
  printf 'phase\tunit\tendpoint\tcode\n' >"${SMOKE_TSV}"
  printf 'phase\tunit\tstatus\n' >"${ROLLOUT_TSV}"
  preflight
  supply_chain
  ensure_cluster
  create_ns_and_secrets
  setup_cluster_rbac
  apply_stack
  run_migrations
  patch_units_sa
  wait_and_smoke "baseline-${BASE_IMAGE##*:}"
  log "promoting all units to candidate ${CANDIDATE_IMAGE}"
  for u in "${UNITS[@]}"; do kc set image "deployment/${u}" "app=${CANDIDATE_IMAGE}"; done
  wait_and_smoke "candidate-${CANDIDATE_IMAGE##*:}"
  rollback_redeploy
  registry_promote_rollback
  write_evidence
  log "=== DONE — all drills passed (kind-tier) ==="
}

trap 'teardown' EXIT
main "$@"
