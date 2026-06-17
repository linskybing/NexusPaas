#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BACKEND_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
REPO_ROOT="$(cd "${BACKEND_DIR}/.." && pwd)"

GO_VERSION="${GO_VERSION:-1.25.11}"
GOVULNCHECK_VERSION="${GOVULNCHECK_VERSION:-v1.3.0}"
OSV_SCANNER_VERSION="${OSV_SCANNER_VERSION:-v2.0.2}"
TRIVY_VERSION="${TRIVY_VERSION:-0.71.1}"
SONAR_SCANNER_VERSION="${SONAR_SCANNER_VERSION:-7.2.0.5079}"
export GOTOOLCHAIN="${GOTOOLCHAIN:-go${GO_VERSION}}"

TEST_POSTGRES_PORT="${TEST_POSTGRES_PORT:-15432}"
TEST_REDIS_PORT="${TEST_REDIS_PORT:-16379}"
TEST_MINIO_PORT="${TEST_MINIO_PORT:-19000}"
TEST_MINIO_CONSOLE_PORT="${TEST_MINIO_CONSOLE_PORT:-19001}"
TEST_RUNTIME_PORT="${TEST_RUNTIME_PORT:-18080}"
TEST_POSTGRES_PASSWORD="${TEST_POSTGRES_PASSWORD:-nexuspaas}"
TEST_OBJECT_STORE_ACCESS_KEY="${TEST_OBJECT_STORE_ACCESS_KEY:-nexuspaas}"
TEST_OBJECT_STORE_SECRET_KEY="${TEST_OBJECT_STORE_SECRET_KEY:-nexuspaas-secret}"
TEST_OBJECT_STORE_BUCKET="${TEST_OBJECT_STORE_BUCKET:-media-e2e}"
TEST_RUNTIME_API_KEY="${TEST_RUNTIME_API_KEY:-smoke-api-key}"
TEST_RUNTIME_SERVICE_KEY="${TEST_RUNTIME_SERVICE_KEY:-smoke-service-key}"

CI_GATE_COVERAGE_THRESHOLD="${CI_GATE_COVERAGE_THRESHOLD:-80.0}"
CI_GATE_CLEANUP="${CI_GATE_CLEANUP:-1}"
TOOLS_DIR="${CI_GATE_TOOLS_DIR:-${TMPDIR:-/tmp}/nexuspaas-ci-tools}"
TOOLS_BIN="${TOOLS_DIR}/bin"
DOCKER_CONFIG_DIR="${CI_GATE_DOCKER_CONFIG:-${TOOLS_DIR}/docker-config}"
TRIVY_CACHE_DIR="${CI_GATE_TRIVY_CACHE_DIR:-${TOOLS_DIR}/trivy-cache}"
TRIVY_TIMEOUT="${TRIVY_TIMEOUT:-10m}"

RAW_RUN_ID="${CI_GATE_RUN_ID:-${GITHUB_RUN_ID:-local}-$$}"
RUN_SUFFIX="$(printf '%s' "${RAW_RUN_ID}" | tr '[:upper:]' '[:lower:]' | tr -c 'a-z0-9_.-' '-')"
ARTIFACT_DIR="${CI_GATE_ARTIFACT_DIR:-${TMPDIR:-/tmp}/nexuspaas-quality-gate/${RUN_SUFFIX}}"

POSTGRES_CONTAINER="nexuspaas-ci-postgres-${RUN_SUFFIX}"
REDIS_CONTAINER="nexuspaas-ci-redis-${RUN_SUFFIX}"
MINIO_CONTAINER="nexuspaas-ci-minio-${RUN_SUFFIX}"
BACKEND_IMAGE="${BACKEND_IMAGE:-nexuspaas-backend:ci-${RUN_SUFFIX}}"
RUNTIME_PID=""

FOCUSED_E2E_PATTERN='TestServiceRouteIsolationContract|TestServiceIsolationValidationE2E|TestIsolatedRuntimeRegistrationE2E|TestProviderConsumerContractMatrix|TestCriticalCrossServiceJourneys|TestSchedulerAdmissionOwnerReadContractsE2E|TestNonBlobIsolatedServiceIgnoresObjectStoreConfigE2E|TestStorageMountPlanContractE2E'
FOCUSED_E2E_PASS_PATTERN='^--- PASS: Test(ServiceRouteIsolationContract|ServiceIsolationValidationE2E|IsolatedRuntimeRegistrationE2E|ProviderConsumerContractMatrix|CriticalCrossServiceJourneys|SchedulerAdmissionOwnerReadContractsE2E|NonBlobIsolatedServiceIgnoresObjectStoreConfigE2E|StorageMountPlanContractE2E)'

mkdir -p "${ARTIFACT_DIR}" "${TOOLS_BIN}" "${DOCKER_CONFIG_DIR}" "${TRIVY_CACHE_DIR}"
if [ ! -f "${DOCKER_CONFIG_DIR}/config.json" ]; then
  printf '{}\n' >"${DOCKER_CONFIG_DIR}/config.json"
fi

log() {
  printf '\n[%s] %s\n' "$(date -u '+%Y-%m-%dT%H:%M:%SZ')" "$*" >&2
}

die() {
  printf 'error: %s\n' "$*" >&2
  exit 1
}

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || die "$1 is required"
}

run_backend() {
  (cd "${BACKEND_DIR}" && "$@")
}

run_repo() {
  (cd "${REPO_ROOT}" && "$@")
}

docker_cli() {
  DOCKER_CONFIG="${DOCKER_CONFIG_DIR}" docker "$@"
}

cleanup_runtime() {
  if [ -n "${RUNTIME_PID}" ] && kill -0 "${RUNTIME_PID}" >/dev/null 2>&1; then
    kill "${RUNTIME_PID}" >/dev/null 2>&1 || true
    wait "${RUNTIME_PID}" >/dev/null 2>&1 || true
  fi
  RUNTIME_PID=""
}

cleanup_containers() {
  if [ "${CI_GATE_CLEANUP}" != "1" ]; then
    log "Leaving Docker containers for inspection: ${POSTGRES_CONTAINER} ${REDIS_CONTAINER} ${MINIO_CONTAINER}"
    return
  fi
  docker_cli rm -f "${MINIO_CONTAINER}" "${REDIS_CONTAINER}" "${POSTGRES_CONTAINER}" >/dev/null 2>&1 || true
}

cleanup_gate() {
  cleanup_runtime
  cleanup_containers
}

usage() {
  cat <<'USAGE'
Usage: backend/scripts/ci-security-gate.sh <quick|docker|security|sonar|beta-rc|all>

Subcommands:
  quick     gofmt check, go vet, go test, go build from backend/
  docker    Docker-backed migrations, integration coverage, focused E2E, full non-live E2E, runtime smoke
  security  govulncheck, OSV source scan, backend image build, Trivy image scan
  sonar     SonarScanner with Quality Gate wait when configured or required
  beta-rc   quick, production-beta render/dry-run/rollback rehearsal, docker/runtime smoke, security, sonar, RC report
  all       quick, docker, security, sonar
USAGE
}

run_quick() {
  need_cmd go
  need_cmd find
  need_cmd xargs

  log "Checking Go version"
  go version
  log "Expected CI Go version: ${GO_VERSION}"

  log "Checking gofmt from backend/"
  local unformatted
  unformatted="$(cd "${BACKEND_DIR}" && find . -name '*.go' -type f -print0 | xargs -0 gofmt -l)"
  if [ -n "${unformatted}" ]; then
    printf '%s\n' "${unformatted}" >&2
    die "gofmt check failed"
  fi

  log "Running go vet ./..."
  run_backend go vet ./...

  log "Running go test ./... -count=1"
  run_backend go test ./... -count=1

  log "Running go build ./..."
  run_backend go build ./...
}

wait_for() {
  local name="$1"
  shift
  local attempt
  for attempt in $(seq 1 60); do
    if "$@" >/dev/null 2>&1; then
      log "${name} is ready"
      return 0
    fi
    sleep 2
  done
  docker_cli logs "${POSTGRES_CONTAINER}" >/tmp/nexuspaas-ci-postgres.log 2>&1 || true
  docker_cli logs "${REDIS_CONTAINER}" >/tmp/nexuspaas-ci-redis.log 2>&1 || true
  docker_cli logs "${MINIO_CONTAINER}" >/tmp/nexuspaas-ci-minio.log 2>&1 || true
  die "${name} did not become ready"
}

start_backing_services() {
  need_cmd docker
  need_cmd curl
  trap cleanup_gate EXIT

  log "Starting isolated Postgres on ${TEST_POSTGRES_PORT}"
  docker_cli run -d --rm \
    --name "${POSTGRES_CONTAINER}" \
    -p "${TEST_POSTGRES_PORT}:5432" \
    -e POSTGRES_USER=nexuspaas \
    -e "POSTGRES_PASSWORD=${TEST_POSTGRES_PASSWORD}" \
    -e POSTGRES_DB=nexuspaas \
    postgres:16-alpine >/dev/null

  log "Starting isolated Redis on ${TEST_REDIS_PORT}"
  docker_cli run -d --rm \
    --name "${REDIS_CONTAINER}" \
    -p "${TEST_REDIS_PORT}:6379" \
    redis:7-alpine \
    redis-server --appendonly no >/dev/null

  log "Starting isolated MinIO on ${TEST_MINIO_PORT}/${TEST_MINIO_CONSOLE_PORT}"
  docker_cli run -d --rm \
    --name "${MINIO_CONTAINER}" \
    -p "${TEST_MINIO_PORT}:9000" \
    -p "${TEST_MINIO_CONSOLE_PORT}:9001" \
    -e "MINIO_ROOT_USER=${TEST_OBJECT_STORE_ACCESS_KEY}" \
    -e "MINIO_ROOT_PASSWORD=${TEST_OBJECT_STORE_SECRET_KEY}" \
    minio/minio:RELEASE.2025-04-08T15-41-24Z \
    server /data --console-address :9001 >/dev/null

  wait_for "Postgres" docker_cli exec "${POSTGRES_CONTAINER}" pg_isready -U nexuspaas -d nexuspaas
  wait_for "Redis" docker_cli exec "${REDIS_CONTAINER}" redis-cli ping
  wait_for "MinIO" curl -fsS "http://127.0.0.1:${TEST_MINIO_PORT}/minio/health/live"
}

export_test_env() {
  local default_database_url
  default_database_url="postgres://nexuspaas:${TEST_POSTGRES_PASSWORD}@localhost:${TEST_POSTGRES_PORT}/nexuspaas?sslmode=disable"
  export TEST_DATABASE_URL="${TEST_DATABASE_URL:-${default_database_url}}"
  export TEST_REDIS_URL="${TEST_REDIS_URL:-redis://localhost:${TEST_REDIS_PORT}/0}"
  export TEST_EVENT_BUS_URL="${TEST_EVENT_BUS_URL:-${TEST_REDIS_URL}}"
  export TEST_OBJECT_STORE_URL="${TEST_OBJECT_STORE_URL:-http://localhost:${TEST_MINIO_PORT}}"
  export TEST_OBJECT_STORE_ACCESS_KEY
  export TEST_OBJECT_STORE_SECRET_KEY
  export TEST_OBJECT_STORE_BUCKET
}

run_admin_tasks() {
  export_test_env

  log "Applying migrations"
  run_backend env \
    DATABASE_URL="${TEST_DATABASE_URL}" \
    ADMIN_TASK=apply-migrations \
    go run ./cmd/microservice

  log "Validating migrations"
  run_backend env \
    DATABASE_URL="${TEST_DATABASE_URL}" \
    ADMIN_TASK=validate-migrations \
    go run ./cmd/microservice

  log "Ensuring E2E object-store bucket"
  run_backend env \
    SERVICE_NAME=media-upload-service \
    PRODUCTION=false \
    REQUIRE_AUTH=false \
    DEV_HEADER_AUTH=true \
    DATABASE_URL="${TEST_DATABASE_URL}" \
    REDIS_URL="${TEST_REDIS_URL}" \
    EVENT_BUS_URL="${TEST_EVENT_BUS_URL}" \
    OBJECT_STORE_URL="${TEST_OBJECT_STORE_URL}" \
    OBJECT_STORE_ACCESS_KEY="${TEST_OBJECT_STORE_ACCESS_KEY}" \
    OBJECT_STORE_SECRET_KEY="${TEST_OBJECT_STORE_SECRET_KEY}" \
    OBJECT_STORE_BUCKET="${TEST_OBJECT_STORE_BUCKET}" \
    ADMIN_TASK=ensure-object-store-bucket \
    go run ./cmd/microservice
}

enforce_coverage_threshold() {
  local coverage_file="$1"
  local total
  total="$(run_backend go tool cover -func="${coverage_file}" | awk '/^total:/ {gsub("%", "", $3); print $3}')"
  [ -n "${total}" ] || die "could not read total coverage from ${coverage_file}"
  log "Integration coverage total: ${total}% (threshold ${CI_GATE_COVERAGE_THRESHOLD}%)"
  awk -v got="${total}" -v want="${CI_GATE_COVERAGE_THRESHOLD}" 'BEGIN { exit (got + 0 >= want + 0) ? 0 : 1 }' \
    || die "coverage ${total}% is below threshold ${CI_GATE_COVERAGE_THRESHOLD}%"
}

run_integration_coverage() {
  export_test_env
  local coverage_file="${BACKEND_DIR}/coverage.out"
  local log_file="${ARTIFACT_DIR}/integration.log"

  log "Running integration tests with coverage"
  run_backend go test -tags integration ./... -coverprofile=coverage.out -count=1 2>&1 | tee "${log_file}"
  enforce_coverage_threshold "${coverage_file}"
}

run_focused_e2e() {
  export_test_env
  local log_file="${ARTIFACT_DIR}/focused-e2e.log"

  log "Running focused E2E gate"
  run_backend go test -tags e2e ./internal/e2e -run "${FOCUSED_E2E_PATTERN}" -count=1 -v 2>&1 | tee "${log_file}"
  grep -E "${FOCUSED_E2E_PASS_PATTERN}" "${log_file}" >/dev/null \
    || die "focused E2E did not emit required PASS lines"
  if grep -Eiq 'SKIP|skipping' "${log_file}"; then
    die "focused E2E skipped a required test"
  fi
}

run_full_non_live_e2e() {
  export_test_env
  local log_file="${ARTIFACT_DIR}/full-e2e.log"

  log "Running full non-live E2E package"
  run_backend go test -tags e2e ./internal/e2e -count=1 -v 2>&1 | tee "${log_file}"
}

http_status_code() {
  local code
  code="$(curl -sS -o /dev/null -w '%{http_code}' "$@" || true)"
  printf '%s' "${code:-000}"
}

wait_for_runtime() {
  local runtime_url="$1"
  local runtime_log="$2"
  local attempt
  for attempt in $(seq 1 90); do
    if [ -n "${RUNTIME_PID}" ] && ! kill -0 "${RUNTIME_PID}" >/dev/null 2>&1; then
      cat "${runtime_log}" >&2 || true
      die "runtime exited before becoming healthy"
    fi
    if curl -fsS "${runtime_url}/healthz" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done
  cat "${runtime_log}" >&2 || true
  die "runtime did not become healthy"
}

run_runtime_smoke() {
  export_test_env
  need_cmd curl

  local runtime_url="http://127.0.0.1:${TEST_RUNTIME_PORT}"
  local runtime_log="${ARTIFACT_DIR}/runtime.log"
  local smoke_log="${ARTIFACT_DIR}/runtime-smoke.log"
  local registry_file="${ARTIFACT_DIR}/service-registry.json"

  log "Starting all-in-one runtime smoke on 127.0.0.1:${TEST_RUNTIME_PORT}"
  cleanup_runtime
  (
    cd "${BACKEND_DIR}" && env \
      SERVICE_NAME=all \
      "HTTP_ADDR=127.0.0.1:${TEST_RUNTIME_PORT}" \
      REQUIRE_AUTH=false \
      DEV_HEADER_AUTH=true \
      PRODUCTION=false \
      "DATABASE_URL=${TEST_DATABASE_URL}" \
      "REDIS_URL=${TEST_REDIS_URL}" \
      "EVENT_BUS_URL=${TEST_EVENT_BUS_URL}" \
      "OBJECT_STORE_URL=${TEST_OBJECT_STORE_URL}" \
      "OBJECT_STORE_ACCESS_KEY=${TEST_OBJECT_STORE_ACCESS_KEY}" \
      "OBJECT_STORE_SECRET_KEY=${TEST_OBJECT_STORE_SECRET_KEY}" \
      "OBJECT_STORE_BUCKET=${TEST_OBJECT_STORE_BUCKET}" \
      "SERVICE_API_KEY=${TEST_RUNTIME_SERVICE_KEY}" \
      go run ./cmd/microservice
  ) >"${runtime_log}" 2>&1 &
  RUNTIME_PID="$!"

  wait_for_runtime "${runtime_url}" "${runtime_log}"

  : >"${smoke_log}"
  log "Checking runtime core endpoints"
  local endpoint code
  for endpoint in /healthz /readyz /metrics /openapi.json /service-registry; do
    code="$(http_status_code "${runtime_url}${endpoint}")"
    printf '%s %s\n' "${code}" "${endpoint}" | tee -a "${smoke_log}"
    [ "${code}" = "200" ] || die "${endpoint} returned ${code}, want 200"
  done

  curl -fsS "${runtime_url}/service-registry" >"${registry_file}"
  local service_count
  service_count="$(grep -o '"name":"' "${registry_file}" | wc -l | tr -d '[:space:]')"
  printf 'service-registry services: %s\n' "${service_count}" | tee -a "${smoke_log}"
  [ "${service_count}" = "15" ] || die "service-registry contains ${service_count} services, want 15"

  log "Checking read-only smoke endpoint for each service"
  while IFS='|' read -r service path; do
    [ -n "${service}" ] || continue
    code="$(
      http_status_code \
        -H 'X-User-ID: smoke-admin' \
        -H 'X-Username: smoke-admin' \
        -H 'X-User-Role: admin' \
        -H "X-API-Key: ${TEST_RUNTIME_API_KEY}" \
        -H "X-Service-Key: ${TEST_RUNTIME_SERVICE_KEY}" \
        "${runtime_url}${path}"
    )"
    printf '%s %-34s %s\n' "${code}" "${service}" "${path}" | tee -a "${smoke_log}"
    case "${code}" in
      000|5*) die "${service} smoke endpoint returned ${code}" ;;
    esac
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
  cleanup_runtime
}

run_docker_gate() {
  need_cmd go
  start_backing_services
  run_admin_tasks
  run_integration_coverage
  run_focused_e2e
  run_full_non_live_e2e
  run_runtime_smoke
}

service_manifest_names() {
  find "${BACKEND_DIR}" -mindepth 2 -maxdepth 3 -path '*/k8s/deployment.yaml' -type f \
    | while IFS= read -r manifest; do
      basename "$(dirname "$(dirname "${manifest}")")"
    done \
    | sort
}

rendered_has_deployment() {
  local render_file="$1"
  local deployment_name="$2"
  awk -v want="${deployment_name}" '
    $0 == "kind: Deployment" {
      in_deployment = 1
      name = ""
    }
    in_deployment && $0 ~ /^  name: / {
      name = $2
    }
    in_deployment && $0 == "---" {
      if (name == want) {
        found = 1
      }
      in_deployment = 0
      name = ""
    }
    END {
      if (in_deployment && name == want) {
        found = 1
      }
      exit found ? 0 : 1
    }
  ' "${render_file}"
}

file_sha256() {
  local path="$1"
  if command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "${path}" | awk '{print $1}'
    return
  fi
  sha256sum "${path}" | awk '{print $1}'
}

run_production_beta_manifest_rehearsal() {
  need_cmd kubectl
  need_cmd find
  need_cmd sort
  need_cmd wc

  local service_list="${ARTIFACT_DIR}/production-beta-services.txt"
  local render_file="${ARTIFACT_DIR}/production-beta-render.yaml"
  local deploy_dry_run="${ARTIFACT_DIR}/production-beta-deploy-dry-run.txt"
  local rollback_plan="${ARTIFACT_DIR}/production-beta-rollback-plan.sh"
  local redeploy_dry_run="${ARTIFACT_DIR}/production-beta-redeploy-dry-run.txt"
  local rehearsal_report="${ARTIFACT_DIR}/production-beta-manifest-rehearsal.md"

  service_manifest_names >"${service_list}"
  local service_count
  service_count="$(wc -l <"${service_list}" | tr -d '[:space:]')"
  if [ "${service_count}" != "15" ]; then
    die "production-beta service manifest count = ${service_count}, want 15"
  fi

  log "Rendering production-beta kustomization"
  run_repo kubectl kustomize backend >"${render_file}"

  local service
  while IFS= read -r service; do
    [ -n "${service}" ] || continue
    rendered_has_deployment "${render_file}" "${service}" \
      || die "production-beta render is missing Deployment ${service}"
  done <"${service_list}"

  if rendered_has_deployment "${render_file}" "platform"; then
    die "production-beta render contains all-in-one platform Deployment"
  fi
  if grep -q -- '-dev-' "${render_file}"; then
    die "production-beta render contains dev secret references"
  fi

  log "Running production-beta deploy client dry-run"
  run_repo kubectl apply --dry-run=client --validate=false -f "${render_file}" >"${deploy_dry_run}"

  log "Writing production-beta rollback command plan"
  {
    printf '#!/usr/bin/env bash\n'
    printf 'set -Eeuo pipefail\n\n'
    while IFS= read -r service; do
      [ -n "${service}" ] || continue
      printf 'kubectl -n nexuspaas rollout undo deployment/%s\n' "${service}"
    done <"${service_list}"
  } >"${rollback_plan}"
  chmod +x "${rollback_plan}"

  log "Running production-beta re-deploy client dry-run"
  run_repo kubectl apply --dry-run=client --validate=false -f "${render_file}" >"${redeploy_dry_run}"

  {
    printf '# Production Beta Manifest Rehearsal\n\n'
    printf -- '- Service deployments: %s\n' "${service_count}"
    printf -- '- Render artifact: `%s`\n' "${render_file}"
    printf -- '- Render SHA-256: `%s`\n' "$(file_sha256 "${render_file}")"
    printf -- '- Deploy dry-run artifact: `%s`\n' "${deploy_dry_run}"
    printf -- '- Rollback command plan: `%s`\n' "${rollback_plan}"
    printf -- '- Re-deploy dry-run artifact: `%s`\n' "${redeploy_dry_run}"
    printf -- '- All-in-one platform deployment absent: yes\n'
    printf -- '- Dev secret references absent: yes\n'
  } >"${rehearsal_report}"
}

install_go_tool() {
  local binary="$1"
  local module="$2"
  local version="$3"
  local target="${TOOLS_BIN}/${binary}-${version}"
  if [ ! -x "${target}" ]; then
    log "Installing ${binary} ${version}"
    GOBIN="${TOOLS_BIN}" run_backend go install "${module}@${version}" \
      || die "failed to install ${binary} ${version}"
    mv "${TOOLS_BIN}/${binary}" "${target}" \
      || die "failed to cache ${binary} ${version}"
  fi
  printf '%s\n' "${target}"
}

tool_platform() {
  local os_name arch
  os_name="$(uname -s)"
  arch="$(uname -m)"
  case "${os_name}:${arch}" in
    Linux:x86_64) printf '%s\n' "linux-x64" ;;
    Darwin:x86_64) printf '%s\n' "macosx-x64" ;;
    Darwin:arm64) printf '%s\n' "macosx-aarch64" ;;
    *) die "unsupported platform for SonarScanner: ${os_name}/${arch}" ;;
  esac
}

trivy_asset_platform() {
  local os_name arch
  os_name="$(uname -s)"
  arch="$(uname -m)"
  case "${os_name}:${arch}" in
    Linux:x86_64) printf '%s\n' "Linux-64bit" ;;
    Linux:aarch64|Linux:arm64) printf '%s\n' "Linux-ARM64" ;;
    Darwin:x86_64) printf '%s\n' "macOS-64bit" ;;
    Darwin:arm64) printf '%s\n' "macOS-ARM64" ;;
    *) die "unsupported platform for Trivy: ${os_name}/${arch}" ;;
  esac
}

install_trivy() {
  need_cmd curl
  need_cmd tar
  local platform target_dir target archive_url tmp
  platform="$(trivy_asset_platform)"
  target_dir="${TOOLS_DIR}/trivy-${TRIVY_VERSION}-${platform}"
  target="${target_dir}/trivy"
  if [ ! -x "${target}" ]; then
    log "Installing Trivy ${TRIVY_VERSION}"
    rm -rf "${target_dir}"
    mkdir -p "${target_dir}"
    tmp="${ARTIFACT_DIR}/trivy.tar.gz"
    archive_url="https://github.com/aquasecurity/trivy/releases/download/v${TRIVY_VERSION}/trivy_${TRIVY_VERSION}_${platform}.tar.gz"
    curl -fsSL "${archive_url}" -o "${tmp}" \
      || die "failed to download Trivy ${TRIVY_VERSION} from ${archive_url}"
    tar -xzf "${tmp}" -C "${target_dir}" trivy \
      || die "failed to extract Trivy ${TRIVY_VERSION}"
    chmod +x "${target}" \
      || die "failed to mark Trivy executable"
    [ -x "${target}" ] || die "Trivy archive did not contain expected binary"
  fi
  printf '%s\n' "${target}"
}

install_sonar_scanner() {
  need_cmd curl
  need_cmd unzip
  local platform target_dir target zip_url tmp extracted
  platform="$(tool_platform)"
  target_dir="${TOOLS_DIR}/sonar-scanner-${SONAR_SCANNER_VERSION}-${platform}"
  target="${target_dir}/bin/sonar-scanner"
  if [ ! -x "${target}" ]; then
    log "Installing SonarScanner ${SONAR_SCANNER_VERSION}"
    tmp="${ARTIFACT_DIR}/sonar-scanner.zip"
    zip_url="https://binaries.sonarsource.com/Distribution/sonar-scanner-cli/sonar-scanner-cli-${SONAR_SCANNER_VERSION}-${platform}.zip"
    curl -fsSL "${zip_url}" -o "${tmp}" \
      || die "failed to download SonarScanner ${SONAR_SCANNER_VERSION} from ${zip_url}"
    unzip -q "${tmp}" -d "${TOOLS_DIR}" \
      || die "failed to extract SonarScanner ${SONAR_SCANNER_VERSION}"
    extracted="${TOOLS_DIR}/sonar-scanner-${SONAR_SCANNER_VERSION}-${platform}"
    [ -x "${extracted}/bin/sonar-scanner" ] || die "SonarScanner archive did not contain expected binary"
  fi
  printf '%s\n' "${target}"
}

run_security_gate() {
  need_cmd go
  need_cmd docker

  local govulncheck_bin osv_scanner_bin trivy_bin
  govulncheck_bin="$(install_go_tool govulncheck golang.org/x/vuln/cmd/govulncheck "${GOVULNCHECK_VERSION}")"
  osv_scanner_bin="$(install_go_tool osv-scanner github.com/google/osv-scanner/v2/cmd/osv-scanner "${OSV_SCANNER_VERSION}")"
  trivy_bin="$(install_trivy)"

  log "Running govulncheck from backend/"
  run_backend "${govulncheck_bin}" ./...

  log "Running OSV source scan from repository root"
  run_repo "${osv_scanner_bin}" scan source -r .

  log "Building backend container image ${BACKEND_IMAGE}"
  docker_cli build -t "${BACKEND_IMAGE}" "${BACKEND_DIR}"

  log "Running Trivy image scan"
  DOCKER_CONFIG="${DOCKER_CONFIG_DIR}" TRIVY_CACHE_DIR="${TRIVY_CACHE_DIR}" \
    "${trivy_bin}" image --timeout "${TRIVY_TIMEOUT}" --exit-code 1 --severity HIGH,CRITICAL "${BACKEND_IMAGE}"
}

sonar_required() {
  case "${CI_GATE_SONAR_REQUIRED:-}" in
    1|true|TRUE|yes|YES) return 0 ;;
    0|false|FALSE|no|NO) return 1 ;;
  esac
  [ -n "${CI:-}" ]
}

run_sonar_gate() {
  if [ -z "${SONAR_TOKEN:-}" ] || [ -z "${SONAR_HOST_URL:-}" ]; then
    if sonar_required; then
      die "SONAR_TOKEN and SONAR_HOST_URL are required for this CI event"
    fi
    log "Skipping SonarScanner because SONAR_TOKEN or SONAR_HOST_URL is not configured"
    printf 'SonarScanner skipped: missing SONAR_TOKEN or SONAR_HOST_URL\n' >"${ARTIFACT_DIR}/sonar-skipped.txt"
    return 0
  fi

  local sonar_scanner_bin
  sonar_scanner_bin="$(install_sonar_scanner)"
  log "Running SonarScanner Quality Gate"
  run_repo "${sonar_scanner_bin}" \
    -Dsonar.qualitygate.wait=true \
    -Dsonar.host.url="${SONAR_HOST_URL}" \
    -Dsonar.token="${SONAR_TOKEN}"
}

write_beta_rc_report() {
  local report="${ARTIFACT_DIR}/beta-rc-report.md"
  local revision sonar_status
  revision="$(run_repo git rev-parse --short HEAD 2>/dev/null || printf 'unknown')"
  sonar_status="passed"
  if [ -f "${ARTIFACT_DIR}/sonar-skipped.txt" ]; then
    sonar_status="skipped: missing SONAR_TOKEN or SONAR_HOST_URL under current policy"
  fi

  {
    printf '# NexusPaas Production Beta RC Gate\n\n'
    printf -- '- Commit: `%s`\n' "${revision}"
    printf -- '- Generated: `%s`\n' "$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
    printf -- '- Artifact directory: `%s`\n\n' "${ARTIFACT_DIR}"
    printf '## Gate Results\n\n'
    printf -- '- Quick gate: passed\n'
    printf -- '- Production-beta manifest deploy dry-run: passed\n'
    printf -- '- Production-beta rollback command rehearsal: passed\n'
    printf -- '- Production-beta re-deploy dry-run: passed\n'
    printf -- '- Docker-backed migrations, integration coverage, focused E2E, full non-live E2E, and runtime smoke: passed\n'
    printf -- '- Security gate: passed\n'
    printf -- '- Sonar gate: %s\n\n' "${sonar_status}"
    printf '## Key Artifacts\n\n'
    printf -- '- Manifest rehearsal: `%s`\n' "${ARTIFACT_DIR}/production-beta-manifest-rehearsal.md"
    printf -- '- Rendered manifest: `%s`\n' "${ARTIFACT_DIR}/production-beta-render.yaml"
    printf -- '- Deploy dry-run: `%s`\n' "${ARTIFACT_DIR}/production-beta-deploy-dry-run.txt"
    printf -- '- Rollback command plan: `%s`\n' "${ARTIFACT_DIR}/production-beta-rollback-plan.sh"
    printf -- '- Re-deploy dry-run: `%s`\n' "${ARTIFACT_DIR}/production-beta-redeploy-dry-run.txt"
    printf -- '- Integration log: `%s`\n' "${ARTIFACT_DIR}/integration.log"
    printf -- '- Focused E2E log: `%s`\n' "${ARTIFACT_DIR}/focused-e2e.log"
    printf -- '- Full E2E log: `%s`\n' "${ARTIFACT_DIR}/full-e2e.log"
    printf -- '- Runtime smoke log: `%s`\n' "${ARTIFACT_DIR}/runtime-smoke.log"
    printf -- '- Runtime log: `%s`\n\n' "${ARTIFACT_DIR}/runtime.log"
    printf '## Live Staging Caveat\n\n'
    printf 'This gate is non-live by default. External Production Beta traffic still requires a live staging rehearsal with real secrets, ready pods, 15-service health/ready/metrics smoke, rollback, and re-deploy evidence.\n'
  } >"${report}"
  log "Beta RC report written to ${report}"
}

run_beta_rc_gate() {
  run_quick
  run_production_beta_manifest_rehearsal
  run_docker_gate
  run_security_gate
  run_sonar_gate
  write_beta_rc_report
}

main() {
  local command="${1:-all}"
  case "${command}" in
    quick) run_quick ;;
    docker) run_docker_gate ;;
    security) run_security_gate ;;
    sonar) run_sonar_gate ;;
    beta-rc) run_beta_rc_gate ;;
    all)
      run_quick
      run_docker_gate
      run_security_gate
      run_sonar_gate
      ;;
    -h|--help|help) usage ;;
    *) usage >&2; die "unknown command ${command}" ;;
  esac
}

main "$@"
