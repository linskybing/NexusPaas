#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BACKEND_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
REPO_ROOT="$(cd "${BACKEND_DIR}/.." && pwd)"

NAMESPACE="${NAMESPACE:-nexuspaas}"
JOB_TIMEOUT="${JOB_TIMEOUT:-10m}"
ROLLOUT_TIMEOUT="${ROLLOUT_TIMEOUT:-5m}"
SMOKE_TIMEOUT_SECONDS="${SMOKE_TIMEOUT_SECONDS:-60}"
SMOKE_PORT_BASE="${SMOKE_PORT_BASE:-19080}"
RUN_ID_RAW="${LIVE_REHEARSAL_RUN_ID:-$(date -u '+%Y%m%d%H%M%S')-$$}"
RUN_ID="$(printf '%s' "${RUN_ID_RAW}" | tr '[:upper:]' '[:lower:]' | tr -c 'a-z0-9-' '-')"
ARTIFACT_DIR="${ARTIFACT_DIR:-${TMPDIR:-/tmp}/nexuspaas-production-beta-live-rehearsal/${RUN_ID}}"
RENDER_FILE="${ARTIFACT_DIR}/production-beta-render.yaml"
DEPLOY_DRY_RUN_FILE="${ARTIFACT_DIR}/production-beta-deploy-dry-run.txt"
SERVICE_LIST_FILE="${ARTIFACT_DIR}/backend-units.txt"
PREVIOUS_IMAGES_FILE="${ARTIFACT_DIR}/previous-images.tsv"
SECRET_PRESENCE_FILE="${ARTIFACT_DIR}/secret-presence.tsv"
MIGRATION_JOBS_FILE="${ARTIFACT_DIR}/migration-jobs.tsv"
ROLLOUT_FILE="${ARTIFACT_DIR}/rollouts.tsv"
SMOKE_FILE="${ARTIFACT_DIR}/smoke.tsv"
ROLLBACK_FILE="${ARTIFACT_DIR}/rollback-redeploy.tsv"
REPORT_FILE="${ARTIFACT_DIR}/production-beta-live-rehearsal-report.md"
PORT_FORWARD_PID=""

log() {
  printf '[%s] %s\n' "$(date -u '+%Y-%m-%dT%H:%M:%SZ')" "$*" >&2
}

die() {
  printf 'error: %s\n' "$*" >&2
  exit 1
}

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || die "$1 is required"
}

cleanup_port_forward() {
  if [ -n "${PORT_FORWARD_PID}" ] && kill -0 "${PORT_FORWARD_PID}" >/dev/null 2>&1; then
    kill "${PORT_FORWARD_PID}" >/dev/null 2>&1 || true
    wait "${PORT_FORWARD_PID}" >/dev/null 2>&1 || true
  fi
  PORT_FORWARD_PID=""
}

cleanup() {
  cleanup_port_forward
}
trap cleanup EXIT

require_live_opt_in() {
  if [ "${LIVE_STAGING_REHEARSAL:-}" != "1" ]; then
    die "LIVE_STAGING_REHEARSAL=1 is required before any live staging mutation"
  fi
}

require_env() {
  local name="$1"
  if [ -z "${!name:-}" ]; then
    die "${name} is required"
  fi
}

reject_local_context() {
  local context="$1"
  local lowered
  lowered="$(printf '%s' "${context}" | tr '[:upper:]' '[:lower:]')"
  case "${lowered}" in
    docker-desktop|*docker-desktop*|*docker\ desktop*|kind|kind-*|*kind-*|minikube|*minikube*|local|*local*|localhost|*localhost*|*127.0.0.1*|*::1*|*loopback*)
      die "refusing local-style kube context ${context}"
      ;;
  esac
}

registry_host_for_image() {
  local image="$1"
  case "${image}" in
    */*) printf '%s\n' "${image%%/*}" ;;
    *) return 1 ;;
  esac
}

reject_local_image_ref() {
  local image="$1"
  local host
  host="$(registry_host_for_image "${image}")" || die "${image} must include an external registry host"
  case "${host}" in
    localhost|localhost:*|127.*|0.0.0.0|0.0.0.0:*|::1|::1:*|[[]::1[]]*|host.docker.internal|*.local|local|local:*|*loopback*|kind*|minikube*)
      die "refusing local registry image ${image}"
      ;;
  esac
  case "${host}" in
    *.*|*:*) ;;
    *) die "${image} must use an explicit external registry host" ;;
  esac
}

require_candidate_image() {
  require_env CANDIDATE_IMAGE
  local digest
  digest="${CANDIDATE_IMAGE##*@sha256:}"
  [ "${digest}" != "${CANDIDATE_IMAGE}" ] \
    || die "CANDIDATE_IMAGE must be digest-pinned with @sha256:<64 lowercase hex digest>"
  printf '%s' "${digest}" | grep -Eq '^[a-f0-9]{64}$' \
    || die "CANDIDATE_IMAGE must be digest-pinned with @sha256:<64 lowercase hex digest>"
  reject_local_image_ref "${CANDIDATE_IMAGE}"
}

require_registry_evidence() {
  if [ -n "${SOURCE_IMAGE:-}" ] || [ -n "${PROMOTED_IMAGE_TAG:-}" ]; then
    require_env SOURCE_IMAGE
    require_env PROMOTED_IMAGE_TAG
    reject_local_image_ref "${PROMOTED_IMAGE_TAG}"
    need_cmd crane
    PROMOTION_MODE="crane copy"
    return
  fi
  require_env PROMOTION_EVIDENCE
  PROMOTION_MODE="operator evidence"
}

require_scan_evidence() {
  if [ -z "${REGISTRY_SCAN_STATUS:-}" ] && [ -z "${REGISTRY_SCAN_EVIDENCE:-}" ]; then
    die "REGISTRY_SCAN_STATUS or REGISTRY_SCAN_EVIDENCE is required"
  fi
}

kctl() {
  kubectl --context "${KUBE_CONTEXT}" -n "${NAMESPACE}" "$@"
}

backend_units() {
  cat <<'EOF'
platform-gateway
iam-unit
tenant-unit
collaboration-unit
platform-io-unit
usage-observability
compute-api
compute-control-plane
EOF
}

logical_services() {
  cat <<'EOF'
platform-gateway
identity-service
authorization-policy-service
org-project-service
audit-compliance-service
request-notification-service
media-upload-service
storage-service
image-registry-service
integration-proxy-service
usage-observability-service
workload-service
ide-service
scheduler-quota-service
k8s-control-service
EOF
}

required_secret_names() {
  cat <<'EOF'
postgres-password
dex-password
minio-credentials
coturn-runtime-secret
platform-gateway-runtime-secret
iam-unit-runtime-secret
tenant-unit-runtime-secret
collaboration-unit-runtime-secret
platform-io-unit-runtime-secret
usage-observability-runtime-secret
compute-api-runtime-secret
compute-control-plane-runtime-secret
EOF
}

forbidden_secret_ref_pattern() {
  cat <<'EOF'
postgres-dev-password|dex-dev-password|minio-dev-credentials|(^|[^[:alnum:]-])([[:alnum:]-]*-(dev|test|local)-[[:alnum:]-]*|placeholder-secret|sample-secret|dummy-secret|fake-secret|test-secret|local-secret|change-me|changeme)([^[:alnum:]-]|$)
EOF
}

rendered_deployment_names() {
  local render_file="$1"
  awk '
    $0 == "kind: Deployment" {
      in_deployment = 1
      name = ""
    }
    in_deployment && $0 ~ /^  name: / {
      name = $2
      gsub(/"/, "", name)
    }
    in_deployment && $0 == "---" {
      if (name != "") {
        print name
      }
      in_deployment = 0
      name = ""
    }
    END {
      if (in_deployment && name != "") {
        print name
      }
    }
  ' "${render_file}"
}

rendered_has_deployment() {
  local render_file="$1"
  local deployment="$2"
  rendered_deployment_names "${render_file}" | grep -Fxq "${deployment}"
}

rendered_service_name_for_unit() {
  local render_file="$1"
  local unit="$2"
  awk -v config_name="${unit}-config" '
    $0 == "kind: ConfigMap" {
      in_configmap = 1
      name = ""
      service_name = ""
    }
    in_configmap && $0 ~ /^  name: / {
      name = $2
      gsub(/"/, "", name)
    }
    in_configmap && $0 ~ /^  SERVICE_NAME: / {
      service_name = $2
      gsub(/"/, "", service_name)
    }
    in_configmap && $0 == "---" {
      if (name == config_name) {
        print service_name
      }
      in_configmap = 0
      name = ""
      service_name = ""
    }
    END {
      if (in_configmap && name == config_name) {
        print service_name
      }
    }
  ' "${render_file}"
}

validate_render_service_names() {
  if grep -Eq 'SERVICE_NAME:[[:space:]]*"?all"?' "${RENDER_FILE}"; then
    die "production-beta render contains forbidden SERVICE_NAME=all"
  fi

  local unit service_name
  while IFS= read -r unit; do
    [[ -n "${unit}" ]] || continue
    service_name="$(rendered_service_name_for_unit "${RENDER_FILE}" "${unit}")"
    [[ "${service_name}" = "${unit}" ]] \
      || die "production-beta render ConfigMap ${unit}-config SERVICE_NAME=${service_name:-<missing>}, want ${unit}"
  done <"${SERVICE_LIST_FILE}"
}

validate_render_secret_refs() {
  local pattern secret missing=0
  pattern="$(forbidden_secret_ref_pattern)"
  if grep -Eiq -- "${pattern}" "${RENDER_FILE}"; then
    die "production-beta render contains dev references or forbidden local/test/placeholder Secret references"
  fi

  while IFS= read -r secret; do
    [[ -n "${secret}" ]] || continue
    if ! grep -Fq -- "${secret}" "${RENDER_FILE}"; then
      missing=$((missing + 1))
    fi
  done < <(required_secret_names)
  [[ "${missing}" = "0" ]] || die "production-beta render is missing ${missing} required Secret names"
}

validate_render() {
  log "Rendering production-beta kustomization"
  (cd "${REPO_ROOT}" && kubectl kustomize backend) >"${RENDER_FILE}"
  (cd "${REPO_ROOT}" && kubectl apply --dry-run=client --validate=false -f "${RENDER_FILE}") >"${DEPLOY_DRY_RUN_FILE}"

  backend_units >"${SERVICE_LIST_FILE}"
  local expected_count rendered_count unit
  expected_count="$(wc -l <"${SERVICE_LIST_FILE}" | tr -d '[:space:]')"
  [ "${expected_count}" = "8" ] || die "backend unit list contains ${expected_count}, want 8"

  rendered_count="$(rendered_deployment_names "${RENDER_FILE}" | grep -Fxf "${SERVICE_LIST_FILE}" | wc -l | tr -d '[:space:]')"
  [ "${rendered_count}" = "8" ] || die "production-beta render contains ${rendered_count} expected backend units, want 8"

  while IFS= read -r unit; do
    [ -n "${unit}" ] || continue
    rendered_has_deployment "${RENDER_FILE}" "${unit}" \
      || die "production-beta render is missing Deployment ${unit}"
  done <"${SERVICE_LIST_FILE}"

  if rendered_has_deployment "${RENDER_FILE}" "platform"; then
    die "production-beta render contains all-in-one platform Deployment"
  fi
  validate_render_service_names
  validate_render_secret_refs
}

verify_kube_context() {
  require_env KUBE_CONTEXT
  local current_context
  current_context="$(kubectl config current-context)"
  [ "${current_context}" = "${KUBE_CONTEXT}" ] \
    || die "kubectl current-context ${current_context} does not match KUBE_CONTEXT ${KUBE_CONTEXT}"
  reject_local_context "${KUBE_CONTEXT}"
}

copy_promoted_image_if_requested() {
  if [ "${PROMOTION_MODE}" = "crane copy" ]; then
    log "Promoting image with crane copy"
    crane copy "${SOURCE_IMAGE}" "${PROMOTED_IMAGE_TAG}"
  fi
}

record_secret_presence() {
  log "Checking required Secret names"
  printf 'secret\tpresent\n' >"${SECRET_PRESENCE_FILE}"
  local secret present missing=0
  while IFS= read -r secret; do
    [ -n "${secret}" ] || continue
    if kctl get secret "${secret}" -o name >/dev/null 2>&1; then
      present="yes"
    else
      present="no"
      missing=$((missing + 1))
    fi
    printf '%s\t%s\n' "${secret}" "${present}" >>"${SECRET_PRESENCE_FILE}"
  done < <(required_secret_names)
  [ "${missing}" = "0" ] || die "${missing} required Secret names are absent; see ${SECRET_PRESENCE_FILE}"
}

record_previous_images() {
  log "Recording previous app images"
  printf 'unit\tprevious_image\n' >"${PREVIOUS_IMAGES_FILE}"
  local unit image
  while IFS= read -r unit; do
    [ -n "${unit}" ] || continue
    image="$(kctl get deployment "${unit}" -o jsonpath='{.spec.template.spec.containers[?(@.name=="app")].image}')"
    [ -n "${image}" ] || die "deployment/${unit} does not have an app container image"
    printf '%s\t%s\n' "${unit}" "${image}" >>"${PREVIOUS_IMAGES_FILE}"
  done <"${SERVICE_LIST_FILE}"
}

previous_image_for_unit() {
  local unit="$1"
  awk -F '\t' -v want="${unit}" 'NR > 1 && $1 == want {print $2}' "${PREVIOUS_IMAGES_FILE}"
}

write_migration_job_manifest() {
  local task="$1"
  local job_name="$2"
  local path="$3"
  cat >"${path}" <<EOF
apiVersion: batch/v1
kind: Job
metadata:
  name: ${job_name}
  namespace: ${NAMESPACE}
  labels:
    app.kubernetes.io/name: nexuspaas
    app.kubernetes.io/component: production-beta-live-rehearsal
spec:
  backoffLimit: 0
  template:
    metadata:
      labels:
        app.kubernetes.io/name: nexuspaas
        app.kubernetes.io/component: production-beta-live-rehearsal
    spec:
      restartPolicy: Never
      automountServiceAccountToken: false
      securityContext:
        runAsNonRoot: true
        runAsUser: 10001
        runAsGroup: 10001
        fsGroup: 10001
        seccompProfile:
          type: RuntimeDefault
      containers:
        - name: app
          image: ${CANDIDATE_IMAGE}
          imagePullPolicy: IfNotPresent
          env:
            - name: ADMIN_TASK
              value: ${task}
          envFrom:
            - configMapRef:
                name: platform-gateway-config
            - configMapRef:
                name: production-beta-runtime-config
                optional: true
            - secretRef:
                name: platform-gateway-runtime-secret
          resources:
            requests:
              cpu: 100m
              memory: 128Mi
              ephemeral-storage: 256Mi
            limits:
              cpu: 500m
              memory: 512Mi
              ephemeral-storage: 1Gi
          securityContext:
            allowPrivilegeEscalation: false
            readOnlyRootFilesystem: true
            capabilities:
              drop: ["ALL"]
EOF
}

run_migration_job() {
  local task="$1"
  local job_name="production-beta-${task}-${RUN_ID}"
  local manifest="${ARTIFACT_DIR}/${job_name}.yaml"
  write_migration_job_manifest "${task}" "${job_name}" "${manifest}"
  log "Applying migration Job ${job_name}"
  kctl apply -f "${manifest}"
  kctl wait --for=condition=complete --timeout="${JOB_TIMEOUT}" "job/${job_name}"
  printf '%s\t%s\tcomplete\n' "${task}" "${job_name}" >>"${MIGRATION_JOBS_FILE}"
}

run_migration_jobs() {
  printf 'task\tjob\tstatus\n' >"${MIGRATION_JOBS_FILE}"
  run_migration_job "apply-migrations"
  run_migration_job "validate-migrations"
}

apply_candidate_manifests() {
  log "Applying production-beta manifests"
  kctl apply -f "${RENDER_FILE}"
  local unit
  while IFS= read -r unit; do
    [ -n "${unit}" ] || continue
    kctl set image "deployment/${unit}" "app=${CANDIDATE_IMAGE}"
  done <"${SERVICE_LIST_FILE}"
}

wait_for_rollouts() {
  log "Waiting for backend unit rollouts"
  printf 'unit\tstatus\n' >"${ROLLOUT_FILE}"
  local unit
  while IFS= read -r unit; do
    [ -n "${unit}" ] || continue
    kctl rollout status "deployment/${unit}" --timeout="${ROLLOUT_TIMEOUT}"
    printf '%s\tcomplete\n' "${unit}" >>"${ROLLOUT_FILE}"
  done <"${SERVICE_LIST_FILE}"
}

start_port_forward() {
  local unit="$1"
  local port="$2"
  local log_file="${ARTIFACT_DIR}/port-forward-${unit}-${port}.log"
  cleanup_port_forward
  kctl port-forward "service/${unit}" --address 127.0.0.1 "${port}:80" >"${log_file}" 2>&1 &
  PORT_FORWARD_PID="$!"
}

wait_for_smoke_endpoint() {
  local url="$1"
  local deadline=$((SECONDS + SMOKE_TIMEOUT_SECONDS))
  while [ "${SECONDS}" -lt "${deadline}" ]; do
    if smoke_curl "${url}" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done
  return 1
}

smoke_endpoint() {
  local unit="$1"
  local port="$2"
  local endpoint="$3"
  local phase="$4"
  local body_file="${ARTIFACT_DIR}/smoke-${phase}-${unit}-${endpoint//\//_}.out"
  smoke_curl "http://127.0.0.1:${port}${endpoint}" >"${body_file}"
  printf '%s\t%s\t%s\tpass\n' "${phase}" "${unit}" "${endpoint}" >>"${SMOKE_FILE}"
}

smoke_curl() {
  local url="$1"
  local args=(-fsS)
  if [ -n "${SMOKE_API_KEY:-}" ]; then
    args+=(-H "X-API-Key: ${SMOKE_API_KEY}")
  fi
  if [ -n "${SMOKE_SERVICE_KEY:-}" ]; then
    args+=(-H "X-Service-Key: ${SMOKE_SERVICE_KEY}")
  fi
  args+=(-H "X-User-ID: live-rehearsal")
  args+=(-H "X-Username: live-rehearsal")
  args+=(-H "X-User-Role: admin")
  curl "${args[@]}" "${url}"
}

smoke_unit() {
  local unit="$1"
  local phase="$2"
  local index="$3"
  local port=$((SMOKE_PORT_BASE + index))
  start_port_forward "${unit}" "${port}"
  wait_for_smoke_endpoint "http://127.0.0.1:${port}/healthz" \
    || die "deployment/${unit} /healthz did not become reachable"
  smoke_endpoint "${unit}" "${port}" "/healthz" "${phase}"
  smoke_endpoint "${unit}" "${port}" "/readyz" "${phase}"
  smoke_endpoint "${unit}" "${port}" "/metrics" "${phase}"
  cleanup_port_forward
}

smoke_gateway_registry() {
  local phase="$1"
  local port=$((SMOKE_PORT_BASE + 80))
  local registry_file="${ARTIFACT_DIR}/service-registry-${phase}.json"
  start_port_forward "platform-gateway" "${port}"
  wait_for_smoke_endpoint "http://127.0.0.1:${port}/healthz" \
    || die "platform-gateway /healthz did not become reachable"
  smoke_endpoint "platform-gateway" "${port}" "/openapi.json" "${phase}"
  smoke_curl "http://127.0.0.1:${port}/service-registry" >"${registry_file}"
  printf '%s\t%s\t%s\tpass\n' "${phase}" "platform-gateway" "/service-registry" >>"${SMOKE_FILE}"

  local count service
  count="$(grep -Eo '"name"[[:space:]]*:' "${registry_file}" | wc -l | tr -d '[:space:]')"
  [ "${count}" = "15" ] || die "service-registry contains ${count} services, want 15"
  while IFS= read -r service; do
    [ -n "${service}" ] || continue
    grep -Fq "\"${service}\"" "${registry_file}" \
      || die "service-registry is missing ${service}"
  done < <(logical_services)
  cleanup_port_forward
}

smoke_all_units() {
  local phase="$1"
  local index=0 unit
  while IFS= read -r unit; do
    [ -n "${unit}" ] || continue
    smoke_unit "${unit}" "${phase}" "${index}"
    index=$((index + 1))
  done <"${SERVICE_LIST_FILE}"
  smoke_gateway_registry "${phase}"
}

rollback_and_redeploy_each_unit() {
  log "Rehearsing per-unit rollback and redeploy"
  printf 'unit\tphase\timage\tstatus\n' >"${ROLLBACK_FILE}"
  local index=0 unit previous_image
  while IFS= read -r unit; do
    [ -n "${unit}" ] || continue
    previous_image="$(previous_image_for_unit "${unit}")"
    [ -n "${previous_image}" ] || die "missing recorded previous image for ${unit}"

    kctl set image "deployment/${unit}" "app=${previous_image}"
    kctl rollout status "deployment/${unit}" --timeout="${ROLLOUT_TIMEOUT}"
    printf '%s\trollback\t%s\tcomplete\n' "${unit}" "${previous_image}" >>"${ROLLBACK_FILE}"
    smoke_unit "${unit}" "rollback-${unit}" "${index}"

    kctl set image "deployment/${unit}" "app=${CANDIDATE_IMAGE}"
    kctl rollout status "deployment/${unit}" --timeout="${ROLLOUT_TIMEOUT}"
    printf '%s\tredeploy\t%s\tcomplete\n' "${unit}" "${CANDIDATE_IMAGE}" >>"${ROLLBACK_FILE}"
    smoke_unit "${unit}" "redeploy-${unit}" "${index}"

    index=$((index + 1))
  done <"${SERVICE_LIST_FILE}"
}

write_report() {
  local digest="${CANDIDATE_IMAGE#*@}"
  {
    printf '# Production Beta Live Rehearsal Report\n\n'
    printf -- '- Namespace: `%s`\n' "${NAMESPACE}"
    printf -- '- Kube context: `%s`\n' "${KUBE_CONTEXT}"
    printf -- '- Candidate image: `%s`\n' "${CANDIDATE_IMAGE}"
    printf -- '- Candidate digest: `%s`\n' "${digest}"
    printf -- '- Promotion mode: `%s`\n' "${PROMOTION_MODE}"
    if [ -n "${PROMOTION_EVIDENCE:-}" ]; then
      printf -- '- Promotion evidence: `%s`\n' "${PROMOTION_EVIDENCE}"
    fi
    if [ -n "${SOURCE_IMAGE:-}" ]; then
      printf -- '- Source image: `%s`\n' "${SOURCE_IMAGE}"
      printf -- '- Promoted image tag: `%s`\n' "${PROMOTED_IMAGE_TAG}"
    fi
    if [ -n "${REGISTRY_SCAN_STATUS:-}" ]; then
      printf -- '- Registry scan status: `%s`\n' "${REGISTRY_SCAN_STATUS}"
    fi
    if [ -n "${REGISTRY_SCAN_EVIDENCE:-}" ]; then
      printf -- '- Registry scan evidence: `%s`\n' "${REGISTRY_SCAN_EVIDENCE}"
    fi
    printf -- '- Render artifact: `%s`\n' "${RENDER_FILE}"
    printf -- '- Deploy dry-run artifact: `%s`\n' "${DEPLOY_DRY_RUN_FILE}"
    printf -- '- Previous images: `%s`\n' "${PREVIOUS_IMAGES_FILE}"
    printf -- '- Secret presence: `%s`\n' "${SECRET_PRESENCE_FILE}"
    printf -- '- Migration Jobs: `%s`\n' "${MIGRATION_JOBS_FILE}"
    printf -- '- Rollouts: `%s`\n' "${ROLLOUT_FILE}"
    printf -- '- Smoke checks: `%s`\n' "${SMOKE_FILE}"
    printf -- '- Rollback and redeploy: `%s`\n' "${ROLLBACK_FILE}"
    printf '\nSuccessful harness execution still requires Reviewer acceptance before `problem.md` or `gap.md` can close live P0.2-P0.5.\n'
  } >"${REPORT_FILE}"
  log "Wrote ${REPORT_FILE}"
}

main() {
  require_live_opt_in
  need_cmd kubectl
  need_cmd curl
  need_cmd awk
  need_cmd sed
  need_cmd grep
  need_cmd sort
  need_cmd wc
  verify_kube_context
  require_candidate_image
  require_registry_evidence
  require_scan_evidence

  mkdir -p "${ARTIFACT_DIR}"
  : >"${SMOKE_FILE}"

  validate_render
  copy_promoted_image_if_requested
  record_secret_presence
  record_previous_images
  run_migration_jobs
  apply_candidate_manifests
  wait_for_rollouts
  smoke_all_units "candidate"
  rollback_and_redeploy_each_unit
  smoke_all_units "candidate-after-redeploy"
  write_report
}

main "$@"
