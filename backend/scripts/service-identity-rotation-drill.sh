#!/usr/bin/env bash
# service-identity-rotation-drill.sh — live SERVICE_IDENTITY dual-key rotation
# drill against the kind-deployed 8-unit stack (kind-live-e2e.sh KEEP_CLUSTER=1
# must have run first). Proves the ADR 0003 zero-downtime rotation procedure:
#
#   1. window   receivers accept {key: NEW, previous_key: OLD} — rolling restart
#               of all units with a continuous OLD-key probe: zero auth failures
#   2. senders  fleet SERVICE_IDENTITY_KEY becomes "NEW,OLD" (active key first)
#   3. retire   receivers drop previous_key — OLD key now rejected (401),
#               NEW key accepted
#
# The probe hits a ServiceAuthRequired internal contract
# (POST /internal/storage/projects/{id}/build-source-access on platform-io-unit)
# where the status code separates the outcomes: 401 = service identity
# rejected, 422 = identity accepted (contract-field validation reached).
#
#   !!! KIND LOCAL — NOT EXTERNAL GA PROOF !!!
set -Eeuo pipefail

CLUSTER_NAME="${CLUSTER_NAME:-nexuspaas-kind-e2e}"
KIND_CONTEXT="kind-${CLUSTER_NAME}"
NAMESPACE="${NAMESPACE:-nexuspaas}"
RUN_ID="$(date -u '+%Y%m%d%H%M%S')"
ARTIFACT_DIR="${ARTIFACT_DIR:-${TMPDIR:-/tmp}/nexuspaas-identity-rotation/${RUN_ID}}"
RESULT_TSV="${ARTIFACT_DIR}/results.tsv"
PROBE_LOG="${ARTIFACT_DIR}/window-probes.log"
UNITS=(platform-gateway iam-unit tenant-unit collaboration-unit platform-io-unit usage-observability compute-api compute-control-plane)
PROBE_PATH="/internal/storage/projects/rotation-drill/build-source-access"

log() { printf '[%s] %s\n' "$(date -u '+%H:%M:%SZ')" "$*" >&2; }
die() { printf 'error: %s\n' "$*" >&2; exit 1; }
kc() { kubectl --context "${KIND_CONTEXT}" -n "${NAMESPACE}" "$@"; }
record() { printf '%s\t%s\t%s\n' "$1" "$2" "$3" >>"${RESULT_TSV}"; log "[$1] $2 ($3)"; }

secret_value() { kc get secret "$1" -o jsonpath="{.data.$2}" | base64 -d; }

# probe platform-io-unit's ServiceAuthRequired storage contract with the given
# service key; prints HTTP code (401 rejected / 422 identity accepted / 000 infra)
probe() {
  local key="$1" out="${ARTIFACT_DIR}/pf-probe.log" pf port="" i code
  kubectl --context "${KIND_CONTEXT}" -n "${NAMESPACE}" port-forward deploy/platform-io-unit :8080 >"${out}" 2>&1 &
  pf=$!
  for i in $(seq 1 40); do
    port="$(sed -n 's/^Forwarding from 127\.0\.0\.1:\([0-9]*\).*/\1/p' "${out}" | head -1)"
    [[ -n "${port}" ]] && break
    kill -0 "${pf}" 2>/dev/null || break
    sleep 0.25
  done
  if [[ -z "${port}" ]]; then echo "000"; kill "${pf}" 2>/dev/null || true; wait "${pf}" 2>/dev/null || true; return; fi
  code="$(curl -s -o /dev/null -w '%{http_code}' --max-time 8 -X POST "http://127.0.0.1:${port}${PROBE_PATH}" \
    -H "X-Service-Name: platform-gateway" -H "X-Service-Key: ${key}" \
    -H 'Content-Type: application/json' -d '{}' || echo "000")"
  kill "${pf}" 2>/dev/null || true; wait "${pf}" 2>/dev/null || true
  echo "${code}"
}

expect_probe() {
  local key="$1" want="$2" label="$3" code deadline
  deadline=$(( $(date +%s) + 120 ))
  while true; do
    code="$(probe "${key}")"
    [[ "${code}" == "${want}" ]] && { record "${label}" "pass" "probe=${code}"; return 0; }
    (( $(date +%s) > deadline )) && die "${label}: probe=${code}, want ${want}"
    sleep 3
  done
}

patch_trusted() {
  local json="$1" u
  for u in "${UNITS[@]}"; do
    kc patch secret "${u}-runtime-secret" --type merge \
      -p "$(jq -cn --arg v "${json}" '{stringData:{SERVICE_TRUSTED_IDENTITIES:$v}}')" >/dev/null
  done
}

rolling_restart_all() {
  local u
  for u in "${UNITS[@]}"; do kc rollout restart "deployment/${u}" >/dev/null; done
  for u in "${UNITS[@]}"; do kc rollout status "deployment/${u}" --timeout=300s >/dev/null; done
}

command -v kubectl >/dev/null || die "kubectl is required"
command -v jq >/dev/null || die "jq is required"
kubectl --context "${KIND_CONTEXT}" get ns "${NAMESPACE}" >/dev/null 2>&1 \
  || die "namespace ${NAMESPACE} not found on ${KIND_CONTEXT}; run kind-live-e2e.sh KEEP_CLUSTER=1 first"
mkdir -p "${ARTIFACT_DIR}"
printf 'step\tresult\tdetail\n' >"${RESULT_TSV}"

OLD_KEY="$(secret_value platform-gateway-runtime-secret SERVICE_IDENTITY_KEY)"
# senders may already carry a "new,old" pair from a previous run; the active key is first
OLD_KEY="${OLD_KEY%%,*}"
NEW_KEY="rotated-${RUN_ID}"
TRUSTED_CURRENT="$(secret_value platform-gateway-runtime-secret SERVICE_TRUSTED_IDENTITIES)"
TRUSTED_WINDOW="$(jq -c --arg new "${NEW_KEY}" --arg old "${OLD_KEY}" \
  '.["platform-gateway"] |= (.key = $new | .previous_key = $old)' <<<"${TRUSTED_CURRENT}")"
TRUSTED_FINAL="$(jq -c --arg new "${NEW_KEY}" \
  '.["platform-gateway"] |= ((.key = $new) | del(.previous_key))' <<<"${TRUSTED_CURRENT}")"
log "rotation drill ${RUN_ID}: OLD=${OLD_KEY:0:8}… NEW=${NEW_KEY} artifacts=${ARTIFACT_DIR}"

log "step 0: baseline — OLD key accepted, forged key rejected"
expect_probe "${OLD_KEY}" 422 "baseline-old-key-accepted"
expect_probe "forged-${RUN_ID}" 401 "baseline-forged-key-rejected"

log "step 1: rotation window — receivers accept NEW + previous OLD, rolling restart with continuous OLD-key probe"
patch_trusted "${TRUSTED_WINDOW}"
: >"${PROBE_LOG}"
( while true; do printf '%s %s\n' "$(date -u '+%H:%M:%S')" "$(probe "${OLD_KEY}")" >>"${PROBE_LOG}"; sleep 1; done ) &
PROBE_PID=$!
rolling_restart_all
kill "${PROBE_PID}" 2>/dev/null || true; wait "${PROBE_PID}" 2>/dev/null || true
TOTAL="$(grep -c "" "${PROBE_LOG}")"
AUTH_FAILS="$(awk '$2 == "401" || $2 == "403"' "${PROBE_LOG}" | grep -c "" || true)"
INFRA="$(awk '$2 == "000"' "${PROBE_LOG}" | grep -c "" || true)"
(( TOTAL >= 10 )) || die "window probe collected only ${TOTAL} samples"
(( AUTH_FAILS == 0 )) || die "window probe saw ${AUTH_FAILS} auth failures during rolling restart (see ${PROBE_LOG})"
record "window-zero-auth-failures" "pass" "${TOTAL} probes, 0 auth failures, ${INFRA} infra-transient"
expect_probe "${NEW_KEY}" 422 "window-new-key-accepted"
expect_probe "${OLD_KEY}" 422 "window-old-key-still-accepted"

log "step 2: senders — fleet SERVICE_IDENTITY_KEY becomes 'NEW,OLD' (active first), rolling restart"
for u in "${UNITS[@]}"; do
  kc patch secret "${u}-runtime-secret" --type merge \
    -p "$(jq -cn --arg v "${NEW_KEY},${OLD_KEY}" '{stringData:{SERVICE_IDENTITY_KEY:$v}}')" >/dev/null
done
rolling_restart_all
expect_probe "${NEW_KEY}" 422 "senders-rolled-new-key-live"

log "step 3: retire OLD — receivers drop previous_key, rolling restart"
patch_trusted "${TRUSTED_FINAL}"
for u in "${UNITS[@]}"; do
  kc patch secret "${u}-runtime-secret" --type merge \
    -p "$(jq -cn --arg v "${NEW_KEY}" '{stringData:{SERVICE_IDENTITY_KEY:$v}}')" >/dev/null
done
rolling_restart_all
expect_probe "${OLD_KEY}" 401 "retired-old-key-rejected"
expect_probe "${NEW_KEY}" 422 "final-new-key-accepted"

echo
echo "IDENTITY ROTATION DRILL PASS — run ${RUN_ID}"
column -t -s $'\t' "${RESULT_TSV}"
echo "evidence: ${RESULT_TSV} + ${PROBE_LOG}"
