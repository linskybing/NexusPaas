#!/usr/bin/env bash
set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
BACKEND_DIR="${ROOT_DIR}/backend"
LOG_DIR="${E2E_LOG_DIR:-${BACKEND_DIR}/.e2e-gate}"

POSTGRES_PORT="${E2E_POSTGRES_PORT:-15433}"
REDIS_PORT="${E2E_REDIS_PORT:-16379}"
MINIO_PORT="${E2E_MINIO_PORT:-19000}"
MINIO_CONSOLE_PORT="${E2E_MINIO_CONSOLE_PORT:-19001}"
RUNTIME_PORT="${E2E_RUNTIME_PORT:-18080}"

CONTAINER_PREFIX="${E2E_CONTAINER_PREFIX:-nexuspaas-e2e-gate}"
POSTGRES_CONTAINER="${CONTAINER_PREFIX}-postgres"
REDIS_CONTAINER="${CONTAINER_PREFIX}-redis"
MINIO_CONTAINER="${CONTAINER_PREFIX}-minio"

E2E_RUN_ID="${E2E_RUN_ID:-e2e-$(date +%Y%m%d%H%M%S)}"
POSTGRES_USER="${E2E_POSTGRES_USER:-nexuspaas}"
POSTGRES_PASSWORD="${E2E_POSTGRES_PASSWORD:-pg-${E2E_RUN_ID}}"
OBJECT_ACCESS_KEY="${TEST_OBJECT_STORE_ACCESS_KEY:-nexuspaas}"
OBJECT_SECRET_KEY="${TEST_OBJECT_STORE_SECRET_KEY:-minio-${E2E_RUN_ID}}"
OBJECT_BUCKET="${TEST_OBJECT_STORE_BUCKET:-media-e2e}"
E2E_API_KEY="${E2E_API_KEY:-api-${E2E_RUN_ID}}"
E2E_SERVICE_API_KEY="${E2E_SERVICE_API_KEY:-svc-${E2E_RUN_ID}}"
E2E_MIN_COVERAGE="${E2E_MIN_COVERAGE:-80.0}"
E2E_RUN_FULL="${E2E_RUN_FULL:-true}"
E2E_KEEP_CONTAINERS="${E2E_KEEP_CONTAINERS:-false}"

DATABASE_URL="postgres://${POSTGRES_USER}:${POSTGRES_PASSWORD}@127.0.0.1:${POSTGRES_PORT}/nexuspaas?sslmode=disable"
REDIS_URL="redis://127.0.0.1:${REDIS_PORT}/0"
OBJECT_STORE_URL="http://127.0.0.1:${MINIO_PORT}"
RUNTIME_URL="http://127.0.0.1:${RUNTIME_PORT}"

RUNTIME_PID=""

log() {
  printf '\n==> %s\n' "$*"
}

fail() {
  printf 'ERROR: %s\n' "$*" >&2
  exit 1
}

has_command() {
  command -v "$1" >/dev/null 2>&1
}

cleanup_runtime() {
  if [[ -n "${RUNTIME_PID}" ]] && kill -0 "${RUNTIME_PID}" >/dev/null 2>&1; then
    kill "${RUNTIME_PID}" >/dev/null 2>&1 || true
    wait "${RUNTIME_PID}" >/dev/null 2>&1 || true
  fi
}

cleanup_containers() {
  if [[ "${E2E_KEEP_CONTAINERS}" == "true" ]]; then
    log "Keeping Docker containers because E2E_KEEP_CONTAINERS=true"
    return
  fi

  for name in "${POSTGRES_CONTAINER}" "${REDIS_CONTAINER}" "${MINIO_CONTAINER}"; do
    if docker inspect "${name}" >/dev/null 2>&1; then
      label="$(docker inspect -f '{{ index .Config.Labels "nexuspaas.e2e-gate" }}' "${name}" 2>/dev/null || true)"
      if [[ "${label}" == "true" ]]; then
        docker rm -f "${name}" >/dev/null 2>&1 || true
      fi
    fi
  done
}

cleanup() {
  cleanup_runtime
  cleanup_containers
}

trap cleanup EXIT

require_commands() {
  for cmd in docker go curl awk grep sed seq tr; do
    has_command "${cmd}" || fail "required command not found: ${cmd}"
  done
}

require_port_free() {
  local port="$1"
  local label="$2"
  if has_command lsof && lsof -nP -iTCP:"${port}" -sTCP:LISTEN >/dev/null 2>&1; then
    fail "${label} port ${port} is already in use; override with E2E_*_PORT or stop the listener"
  fi
}

remove_owned_container_if_present() {
  local name="$1"
  if ! docker inspect "${name}" >/dev/null 2>&1; then
    return
  fi

  local label
  label="$(docker inspect -f '{{ index .Config.Labels "nexuspaas.e2e-gate" }}' "${name}" 2>/dev/null || true)"
  if [[ "${label}" != "true" ]]; then
    fail "container ${name} already exists and is not owned by this gate"
  fi

  docker rm -f "${name}" >/dev/null
}

preflight() {
  require_commands

  docker info >/dev/null 2>&1 || fail "Docker daemon is not reachable"

  remove_owned_container_if_present "${POSTGRES_CONTAINER}"
  remove_owned_container_if_present "${REDIS_CONTAINER}"
  remove_owned_container_if_present "${MINIO_CONTAINER}"

  require_port_free "${POSTGRES_PORT}" "Postgres"
  require_port_free "${REDIS_PORT}" "Redis"
  require_port_free "${MINIO_PORT}" "MinIO API"
  require_port_free "${MINIO_CONSOLE_PORT}" "MinIO console"
  require_port_free "${RUNTIME_PORT}" "runtime"

  mkdir -p "${LOG_DIR}"
}

start_postgres() {
  log "Starting isolated Postgres on 127.0.0.1:${POSTGRES_PORT}"
  remove_owned_container_if_present "${POSTGRES_CONTAINER}"
  docker run -d \
    --name "${POSTGRES_CONTAINER}" \
    --label nexuspaas.e2e-gate=true \
    -p "127.0.0.1:${POSTGRES_PORT}:5432" \
    -e "POSTGRES_USER=${POSTGRES_USER}" \
    -e "POSTGRES_PASSWORD=${POSTGRES_PASSWORD}" \
    -e POSTGRES_DB=nexuspaas \
    postgres:16-alpine >/dev/null

  for _ in $(seq 1 60); do
    if docker exec "${POSTGRES_CONTAINER}" pg_isready -U "${POSTGRES_USER}" -d nexuspaas >/dev/null 2>&1; then
      return
    fi
    sleep 1
  done

  docker logs "${POSTGRES_CONTAINER}" >&2 || true
  fail "Postgres did not become ready"
}

start_redis() {
  log "Starting isolated Redis on 127.0.0.1:${REDIS_PORT}"
  remove_owned_container_if_present "${REDIS_CONTAINER}"
  docker run -d \
    --name "${REDIS_CONTAINER}" \
    --label nexuspaas.e2e-gate=true \
    -p "127.0.0.1:${REDIS_PORT}:6379" \
    redis:7-alpine \
    redis-server --appendonly yes >/dev/null

  for _ in $(seq 1 60); do
    if docker exec "${REDIS_CONTAINER}" redis-cli ping >/dev/null 2>&1; then
      return
    fi
    sleep 1
  done

  docker logs "${REDIS_CONTAINER}" >&2 || true
  fail "Redis did not become ready"
}

start_minio() {
  log "Starting isolated MinIO on 127.0.0.1:${MINIO_PORT}/${MINIO_CONSOLE_PORT}"
  remove_owned_container_if_present "${MINIO_CONTAINER}"
  docker run -d \
    --name "${MINIO_CONTAINER}" \
    --label nexuspaas.e2e-gate=true \
    -p "127.0.0.1:${MINIO_PORT}:9000" \
    -p "127.0.0.1:${MINIO_CONSOLE_PORT}:9001" \
    -e "MINIO_ROOT_USER=${OBJECT_ACCESS_KEY}" \
    -e "MINIO_ROOT_PASSWORD=${OBJECT_SECRET_KEY}" \
    minio/minio:RELEASE.2025-04-08T15-41-24Z \
    server /data --console-address :9001 >/dev/null

  for _ in $(seq 1 60); do
    if curl -fsS "${OBJECT_STORE_URL}/minio/health/ready" >/dev/null 2>&1; then
      return
    fi
    sleep 1
  done

  docker logs "${MINIO_CONTAINER}" >&2 || true
  fail "MinIO did not become ready"
}

start_backing_services() {
  start_postgres
  start_redis
  start_minio
}

reset_postgres_and_redis() {
  log "Resetting Postgres and Redis for an isolated full E2E phase"
  docker rm -f "${POSTGRES_CONTAINER}" "${REDIS_CONTAINER}" >/dev/null 2>&1 || true
  start_postgres
  start_redis
}

run_backend() {
  (
    cd "${BACKEND_DIR}"
    env "$@" go run ./cmd/microservice
  )
}

common_test_env() {
  env \
    "TEST_DATABASE_URL=${DATABASE_URL}" \
    "TEST_REDIS_URL=${REDIS_URL}" \
    "TEST_EVENT_BUS_URL=${REDIS_URL}" \
    "TEST_OBJECT_STORE_URL=${OBJECT_STORE_URL}" \
    "TEST_OBJECT_STORE_ACCESS_KEY=${OBJECT_ACCESS_KEY}" \
    "TEST_OBJECT_STORE_SECRET_KEY=${OBJECT_SECRET_KEY}" \
    "TEST_OBJECT_STORE_BUCKET=${OBJECT_BUCKET}" \
    "E2E_API_KEY=${E2E_API_KEY}" \
    "E2E_SERVICE_API_KEY=${E2E_SERVICE_API_KEY}" \
    "$@"
}

apply_and_validate_migrations() {
  log "Applying migrations"
  run_backend ADMIN_TASK=apply-migrations "DATABASE_URL=${DATABASE_URL}"

  log "Validating migrations"
  run_backend ADMIN_TASK=validate-migrations "DATABASE_URL=${DATABASE_URL}"
}

ensure_media_bucket() {
  log "Ensuring media bucket ${OBJECT_BUCKET}"
  run_backend \
    SERVICE_NAME=media-upload-service \
    PRODUCTION=false \
    REQUIRE_AUTH=false \
    DEV_HEADER_AUTH=true \
    "OBJECT_STORE_URL=${OBJECT_STORE_URL}" \
    "OBJECT_STORE_ACCESS_KEY=${OBJECT_ACCESS_KEY}" \
    "OBJECT_STORE_SECRET_KEY=${OBJECT_SECRET_KEY}" \
    "OBJECT_STORE_BUCKET=${OBJECT_BUCKET}" \
    ADMIN_TASK=ensure-object-store-bucket
}

run_integration_coverage() {
  log "Running integration tests with coverage"
  (
    cd "${BACKEND_DIR}"
    common_test_env go test -tags integration ./... -coverprofile=coverage.out -count=1
  )

  local coverage
  coverage="$(
    awk 'NR > 1 { total += $2; if ($3 > 0) covered += $2 } END { if (total == 0) { print "0.00" } else { printf "%.2f", (covered / total) * 100 } }' \
      "${BACKEND_DIR}/coverage.out"
  )"
  printf 'Aggregate coverage: %s%% (required >= %s%%)\n' "${coverage}" "${E2E_MIN_COVERAGE}"

  awk -v got="${coverage}" -v want="${E2E_MIN_COVERAGE}" 'BEGIN { exit !((got + 0) >= (want + 0)) }' \
    || fail "coverage ${coverage}% is below ${E2E_MIN_COVERAGE}%"
}

run_focused_e2e() {
  log "Running focused cross-service E2E gate"
  local log_file="${LOG_DIR}/focused-e2e.log"
  local pattern='TestServiceRouteIsolationContract|TestServiceIsolationValidationE2E|TestIsolatedRuntimeRegistrationE2E|TestProviderConsumerContractMatrix|TestCriticalCrossServiceJourneys|TestSchedulerAdmissionOwnerReadContractsE2E|TestNonBlobIsolatedServiceIgnoresObjectStoreConfigE2E|TestStorageMountPlanContractE2E'

  (
    cd "${BACKEND_DIR}"
    common_test_env "E2E_RUN_ID=${E2E_RUN_ID}-focused" \
      go test -tags e2e ./internal/e2e -run "${pattern}" -count=1 -v
  ) | tee "${log_file}"

  for test_name in \
    TestServiceRouteIsolationContract \
    TestServiceIsolationValidationE2E \
    TestIsolatedRuntimeRegistrationE2E \
    TestProviderConsumerContractMatrix \
    TestCriticalCrossServiceJourneys \
    TestSchedulerAdmissionOwnerReadContractsE2E \
    TestNonBlobIsolatedServiceIgnoresObjectStoreConfigE2E \
    TestStorageMountPlanContractE2E
  do
    grep -E "^--- PASS: ${test_name}( |$)" "${log_file}" >/dev/null \
      || fail "focused E2E did not show PASS for ${test_name}"
  done

  if grep -E 'SKIP|skipping|Skipping' "${log_file}" >/dev/null; then
    fail "focused E2E gate skipped a required test"
  fi
}

run_full_e2e() {
  if [[ "${E2E_RUN_FULL}" == "false" ]]; then
    log "Skipping full E2E because E2E_RUN_FULL=false"
    return
  fi

  reset_postgres_and_redis
  apply_and_validate_migrations
  ensure_media_bucket

  log "Running full non-live E2E package"
  local log_file="${LOG_DIR}/full-e2e.log"
  (
    cd "${BACKEND_DIR}"
    common_test_env "E2E_RUN_ID=${E2E_RUN_ID}-full" \
      go test -tags e2e ./internal/e2e -count=1 -v
  ) | tee "${log_file}"
}

http_code() {
  local code
  code="$(curl -sS -o /dev/null -w '%{http_code}' "$@" || true)"
  printf '%s' "${code:-000}"
}

wait_for_runtime() {
  for _ in $(seq 1 90); do
    if [[ -n "${RUNTIME_PID}" ]] && ! kill -0 "${RUNTIME_PID}" >/dev/null 2>&1; then
      cat "${LOG_DIR}/runtime.log" >&2 || true
      fail "runtime exited before becoming ready"
    fi
    if curl -fsS "${RUNTIME_URL}/healthz" >/dev/null 2>&1; then
      return
    fi
    sleep 1
  done

  cat "${LOG_DIR}/runtime.log" >&2 || true
  fail "runtime did not become healthy"
}

run_runtime_smoke() {
  log "Starting all-in-one runtime for smoke checks on 127.0.0.1:${RUNTIME_PORT}"
  cleanup_runtime
  (
    cd "${BACKEND_DIR}"
    env \
      SERVICE_NAME=all \
      "HTTP_ADDR=127.0.0.1:${RUNTIME_PORT}" \
      REQUIRE_AUTH=false \
      DEV_HEADER_AUTH=true \
      PRODUCTION=false \
      "DATABASE_URL=${DATABASE_URL}" \
      "REDIS_URL=${REDIS_URL}" \
      "EVENT_BUS_URL=${REDIS_URL}" \
      "OBJECT_STORE_URL=${OBJECT_STORE_URL}" \
      "OBJECT_STORE_ACCESS_KEY=${OBJECT_ACCESS_KEY}" \
      "OBJECT_STORE_SECRET_KEY=${OBJECT_SECRET_KEY}" \
      "OBJECT_STORE_BUCKET=${OBJECT_BUCKET}" \
      go run ./cmd/microservice
  ) >"${LOG_DIR}/runtime.log" 2>&1 &
  RUNTIME_PID="$!"

  wait_for_runtime

  log "Checking platform runtime endpoints"
  for endpoint in /healthz /readyz /metrics /openapi.json /service-registry; do
    local code
    code="$(http_code "${RUNTIME_URL}${endpoint}")"
    printf '%s %s\n' "${code}" "${endpoint}"
    [[ "${code}" == "200" ]] || fail "${endpoint} returned ${code}, want 200"
  done

  local registry_file="${LOG_DIR}/service-registry.json"
  curl -fsS "${RUNTIME_URL}/service-registry" > "${registry_file}"
  local service_count
  service_count="$(grep -o '"name":"' "${registry_file}" | wc -l | tr -d ' ')"
  printf 'service-registry services: %s\n' "${service_count}"
  [[ "${service_count}" == "15" ]] || fail "service-registry contains ${service_count} services, want 15"

  log "Checking 15 service smoke endpoints"
  while IFS='|' read -r service path; do
    [[ -n "${service}" ]] || continue
    local code
    code="$(
      http_code \
        -H 'X-User-ID: smoke-admin' \
        -H 'X-Username: smoke-admin' \
        -H 'X-User-Role: admin' \
        -H "X-API-Key: ${E2E_API_KEY}" \
        -H "X-Service-Key: ${E2E_SERVICE_API_KEY}" \
        "${RUNTIME_URL}${path}"
    )"
    printf '%s %-34s %s\n' "${code}" "${service}" "${path}"
    if [[ "${code}" == "000" || "${code}" == 5* ]]; then
      fail "${service} smoke endpoint returned ${code}"
    fi
  done <<'EOF'
audit-compliance-service|/api/v1/audit/logs
authorization-policy-service|/api/v1/permissions/policies
ide-service|/api/v1/ide
identity-service|/api/v1/users
image-registry-service|/api/v1/image-catalog
integration-proxy-service|/api/v1/admin/vpn
k8s-control-service|/api/v1/resources
media-upload-service|/api/v1/uploads/images/nonexistent-smoke.png
org-project-service|/api/v1/projects
platform-gateway|/api/v1/gateway/health
request-notification-service|/api/v1/forms
scheduler-quota-service|/api/v1/plans
storage-service|/api/v1/storage/options
usage-observability-service|/api/v1/admin/usage
workload-service|/api/v1/jobs
EOF
}

main() {
  log "NexusPaas Docker E2E gate"
  printf 'Logs: %s\n' "${LOG_DIR}"
  printf 'Coverage profile: %s\n' "${BACKEND_DIR}/coverage.out"
  printf 'Ports: postgres=%s redis=%s minio=%s minio-console=%s runtime=%s\n' \
    "${POSTGRES_PORT}" "${REDIS_PORT}" "${MINIO_PORT}" "${MINIO_CONSOLE_PORT}" "${RUNTIME_PORT}"

  preflight
  start_backing_services
  apply_and_validate_migrations
  ensure_media_bucket
  run_integration_coverage
  run_focused_e2e
  run_full_e2e
  run_runtime_smoke

  log "Docker E2E gate passed"
}

main "$@"
