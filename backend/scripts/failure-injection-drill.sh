#!/usr/bin/env bash
# failure-injection-drill.sh — repeatable OPS-019 fault-injection harness against
# the kind-deployed 8-unit stack (kind-live-e2e.sh KEEP_CLUSTER=1 must have run
# first). Injects real faults and asserts the documented degradation contract,
# then restores and asserts recovery:
#
#   db          scale postgres→0    → /readyz fails closed (503) on DB-dependent
#                                     units; restore → 200
#   k8s-api     docker-pause the kind control-plane for K8S_OUTAGE_SECONDS
#                                   → data plane survives (no container restarts),
#                                     units answer /readyz 200 after recovery
#   node-agent  scale usage-observability→0
#                                   → blast radius contained: other units stay
#                                     ready while the unit is gone; restore → 200
#   prometheus  scale prometheus→0  → product units unaffected (/readyz + /metrics
#                                     stay 200); restore (skipped if no
#                                     prometheus deployment is present)
#
#   !!! KIND LOCAL — NOT EXTERNAL GA PROOF !!!
# Per docs/agents/workflow.md, single-cluster/local evidence must NOT be
# described as external GA proof.
set -Eeuo pipefail

CLUSTER_NAME="${CLUSTER_NAME:-nexuspaas-kind-e2e}"
KIND_CONTEXT="kind-${CLUSTER_NAME}"
NAMESPACE="${NAMESPACE:-nexuspaas}"
SCENARIOS="${SCENARIOS:-db k8s-api node-agent prometheus}"
K8S_OUTAGE_SECONDS="${K8S_OUTAGE_SECONDS:-45}"
API_KEY="${API_KEY:-nexuspaas-kind-admin-key}"
RUN_ID="$(date -u '+%Y%m%d%H%M%S')"
ARTIFACT_DIR="${ARTIFACT_DIR:-${TMPDIR:-/tmp}/nexuspaas-failure-injection/${RUN_ID}}"
RESULT_TSV="${ARTIFACT_DIR}/results.tsv"
UNITS=(platform-gateway iam-unit tenant-unit collaboration-unit platform-io-unit usage-observability compute-api compute-control-plane)

log() { printf '[%s] %s\n' "$(date -u '+%H:%M:%SZ')" "$*" >&2; }
die() { printf 'error: %s\n' "$*" >&2; exit 1; }
kc() { kubectl --context "${KIND_CONTEXT}" -n "${NAMESPACE}" "$@"; }
record() { printf '%s\t%s\t%s\t%s\n' "$1" "$2" "$3" "$4" >>"${RESULT_TSV}"; log "[$1] $2 → $3 ($4)"; }

# curl a unit endpoint through a short-lived port-forward; prints the HTTP code
# (000 when the forward/connection fails, which is itself a valid observation).
probe() {
  local unit="$1" path="$2" out code pf
  out="${ARTIFACT_DIR}/pf-${unit}.log"
  kubectl --context "${KIND_CONTEXT}" -n "${NAMESPACE}" port-forward "deploy/${unit}" :8080 >"${out}" 2>&1 &
  pf=$!
  local port="" i
  for i in $(seq 1 40); do
    port="$(sed -n 's/^Forwarding from 127\.0\.0\.1:\([0-9]*\).*/\1/p' "${out}" | head -1)"
    [[ -n "${port}" ]] && break
    kill -0 "${pf}" 2>/dev/null || break
    sleep 0.25
  done
  if [[ -z "${port}" ]]; then echo "000"; kill "${pf}" 2>/dev/null || true; wait "${pf}" 2>/dev/null || true; return; fi
  code="$(curl -s -o "${ARTIFACT_DIR}/last-body.json" -w '%{http_code}' --max-time 10 -H "X-API-Key: ${API_KEY}" "http://127.0.0.1:${port}${path}" || echo "000")"
  kill "${pf}" 2>/dev/null || true; wait "${pf}" 2>/dev/null || true
  echo "${code}"
}

# poll until probe returns the wanted code (accepts a regex like "503|000")
wait_code() {
  local unit="$1" path="$2" want="$3" timeout="${4:-90}" code deadline
  deadline=$(( $(date +%s) + timeout ))
  while true; do
    code="$(probe "${unit}" "${path}")"
    if [[ "${code}" =~ ^(${want})$ ]]; then echo "${code}"; return 0; fi
    if (( $(date +%s) > deadline )); then echo "${code}"; return 1; fi
    sleep 3
  done
}

scenario_db() {
  local code
  code="$(wait_code tenant-unit /readyz 200 60)" || die "db: baseline tenant-unit /readyz=${code}, want 200"
  record db baseline-ready "pass" "tenant-unit /readyz=200"

  kc scale deployment/postgres --replicas=0 >/dev/null
  code="$(wait_code tenant-unit /readyz 503 120)" || die "db: tenant-unit /readyz=${code} with postgres down, want 503 fail-closed"
  local reason; reason="$(tr -d '\n' <"${ARTIFACT_DIR}/last-body.json" | head -c 200)"
  record db outage-fails-closed "pass" "readyz=503 body=${reason}"

  kc scale deployment/postgres --replicas=1 >/dev/null
  kc rollout status deployment/postgres --timeout=180s >/dev/null
  code="$(wait_code tenant-unit /readyz 200 180)" || die "db: tenant-unit did not recover, /readyz=${code}"
  record db recovery "pass" "readyz=200 after postgres restore"
}

scenario_k8s_api() {
  local cp="${CLUSTER_NAME}-control-plane" before after code
  before="$(kc get pods -o jsonpath='{range .items[*]}{.metadata.name}={.status.containerStatuses[0].restartCount}{"\n"}{end}' | sort)"
  log "pausing ${cp} for ${K8S_OUTAGE_SECONDS}s (control-plane outage)"
  docker pause "${cp}" >/dev/null
  sleep "${K8S_OUTAGE_SECONDS}"
  docker unpause "${cp}" >/dev/null
  local i
  for i in $(seq 1 60); do
    kubectl --context "${KIND_CONTEXT}" get nodes >/dev/null 2>&1 && break
    sleep 2
  done
  kubectl --context "${KIND_CONTEXT}" get nodes >/dev/null 2>&1 || die "k8s-api: control plane did not recover"
  record k8s-api outage-window "pass" "${K8S_OUTAGE_SECONDS}s pause + apiserver recovered"

  after="$(kc get pods -o jsonpath='{range .items[*]}{.metadata.name}={.status.containerStatuses[0].restartCount}{"\n"}{end}' | sort)"
  [[ "${before}" == "${after}" ]] || die "k8s-api: restart counts changed during outage: $(diff <(echo "${before}") <(echo "${after}") | tr '\n' ' ')"
  record k8s-api data-plane-survives "pass" "container restart counts unchanged"

  code="$(wait_code compute-control-plane /readyz 200 120)" || die "k8s-api: compute-control-plane /readyz=${code} after recovery"
  record k8s-api recovery "pass" "compute-control-plane readyz=200"
}

scenario_node_agent() {
  local code
  kc scale deployment/usage-observability --replicas=0 >/dev/null
  kc wait --for=delete pod -l app=usage-observability --timeout=120s >/dev/null 2>&1 || true
  code="$(probe usage-observability /readyz)"
  [[ "${code}" == "000" ]] || die "node-agent: usage-observability still answers (${code}) after scale-to-zero"
  record node-agent unit-down "pass" "usage-observability unreachable (000)"
  local u
  for u in platform-gateway tenant-unit compute-api; do
    code="$(wait_code "${u}" /readyz 200 60)" || die "node-agent: ${u} /readyz=${code} while usage-observability down, want 200"
  done
  record node-agent blast-radius-contained "pass" "gateway/tenant/compute-api readyz=200 during outage"

  kc scale deployment/usage-observability --replicas=1 >/dev/null
  kc rollout status deployment/usage-observability --timeout=180s >/dev/null
  code="$(wait_code usage-observability /readyz 200 120)" || die "node-agent: usage-observability did not recover (${code})"
  record node-agent recovery "pass" "usage-observability readyz=200"
}

scenario_prometheus() {
  if ! kc get deployment/prometheus >/dev/null 2>&1; then
    record prometheus all "skipped" "no prometheus deployment in namespace ${NAMESPACE}"
    return
  fi
  local code
  kc scale deployment/prometheus --replicas=0 >/dev/null
  kc wait --for=delete pod -l app=prometheus --timeout=120s >/dev/null 2>&1 || true
  for u in platform-gateway usage-observability; do
    code="$(wait_code "${u}" /readyz 200 60)" || die "prometheus: ${u} /readyz=${code} during scrape outage, want 200"
    code="$(probe "${u}" /metrics)"
    [[ "${code}" == "200" ]] || die "prometheus: ${u} /metrics=${code} during scrape outage, want 200"
  done
  record prometheus scrape-outage-tolerated "pass" "units serve /readyz + /metrics with prometheus down"

  kc scale deployment/prometheus --replicas=1 >/dev/null
  kc rollout status deployment/prometheus --timeout=180s >/dev/null
  record prometheus recovery "pass" "prometheus scaled back"
}

command -v kubectl >/dev/null || die "kubectl is required"
command -v docker >/dev/null || die "docker is required"
kubectl --context "${KIND_CONTEXT}" get ns "${NAMESPACE}" >/dev/null 2>&1 \
  || die "namespace ${NAMESPACE} not found on ${KIND_CONTEXT}; run kind-live-e2e.sh KEEP_CLUSTER=1 first"
mkdir -p "${ARTIFACT_DIR}"
printf 'scenario\tstep\tresult\tdetail\n' >"${RESULT_TSV}"
log "failure-injection drill ${RUN_ID}: scenarios=[${SCENARIOS}] artifacts=${ARTIFACT_DIR}"

for s in ${SCENARIOS}; do
  case "${s}" in
    db)         scenario_db ;;
    k8s-api)    scenario_k8s_api ;;
    node-agent) scenario_node_agent ;;
    prometheus) scenario_prometheus ;;
    *) die "unknown scenario ${s}" ;;
  esac
done

echo
echo "FAILURE-INJECTION DRILL PASS — run ${RUN_ID}"
column -t -s $'\t' "${RESULT_TSV}"
echo "evidence: ${RESULT_TSV}"
