#!/usr/bin/env bash
# Install a real, pinned Harbor instance for live e2e testing (P0-1 Harbor
# catalog-sync/health evidence) — HTTP-only, loopback-bound, no scanner/
# chartmuseum/notary components, ephemeral CI/local infrastructure.
#
# Harbor's own installer (prepare + install.sh) generates its own
# docker-compose.yml from harbor.yml; it does not compose alongside an
# unrelated stack, so this is a dedicated lifecycle script rather than a
# `services:` stanza in deploy/local/docker-compose.yml — same shape as
# dra-kind-up.sh for the same reason (external tool owns its own topology).
#
# ponytail: HTTP-only + no TLS + default admin password — this Harbor never
# leaves localhost and holds nothing sensitive; a secrets layer for it would
# protect nothing real. Upgrade only if this ever needs to be reachable
# off-host.
set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BACKEND_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

HARBOR_VERSION="${HARBOR_VERSION:-v2.15.1}"
HARBOR_HTTP_PORT="${HARBOR_HTTP_PORT:-18080}"
HARBOR_ADMIN_PASSWORD="${HARBOR_ADMIN_PASSWORD:-Harbor12345}"
HARBOR_CACHE_DIR="${HARBOR_CACHE_DIR:-${BACKEND_DIR}/.cache/harbor-${HARBOR_VERSION}}"
HARBOR_TARBALL="harbor-online-installer-${HARBOR_VERSION}.tgz"
HARBOR_DOWNLOAD_URL="https://github.com/goharbor/harbor/releases/download/${HARBOR_VERSION}/${HARBOR_TARBALL}"

for tool in docker curl tar; do
  command -v "${tool}" >/dev/null 2>&1 || { echo "error: required tool not found: ${tool}" >&2; exit 1; }
done
docker info >/dev/null 2>&1 || { echo "error: docker daemon is not running" >&2; exit 1; }
docker compose version >/dev/null 2>&1 || { echo "error: docker compose plugin not found (Harbor's installer auto-detects it)" >&2; exit 1; }

# Harbor's own docs assume root/sudo for install.sh: its "prepare" container
# writes common/config/*/env unmapped (container UID 0 = host UID 0 without
# userns-remap), and docker compose then needs to read those files back
# client-side to resolve env_file directives. Docker Desktop's Linux VM masks
# this (verified locally); a native Linux Docker Engine (GitHub Actions
# runners, most Linux dev machines) enforces real host UID/GID permissions
# and hits "permission denied" as a non-root user (verified in CI). Elevate
# unless already root.
SUDO=""
[[ "$(id -u)" -eq 0 ]] || SUDO="sudo"

mkdir -p "${HARBOR_CACHE_DIR}"
if [[ ! -f "${HARBOR_CACHE_DIR}/${HARBOR_TARBALL}" ]]; then
  echo ">> downloading Harbor ${HARBOR_VERSION} online installer"
  curl -fsSL -o "${HARBOR_CACHE_DIR}/${HARBOR_TARBALL}" "${HARBOR_DOWNLOAD_URL}"
fi
if [[ ! -d "${HARBOR_CACHE_DIR}/harbor" ]]; then
  echo ">> extracting installer into ${HARBOR_CACHE_DIR}"
  tar xzf "${HARBOR_CACHE_DIR}/${HARBOR_TARBALL}" -C "${HARBOR_CACHE_DIR}"
fi

cd "${HARBOR_CACHE_DIR}/harbor"
echo ">> writing harbor.yml (HTTP-only, hostname harbor.nexuspaas.local, port ${HARBOR_HTTP_PORT})"
cp harbor.yml.tmpl harbor.yml
# Harbor's prepare step hard-rejects "127.0.0.1"/"localhost" as the configured
# hostname (verified: it errors "127.0.0.1 can not be the hostname", not just
# a documentation warning). This hostname value is only used for Harbor's own
# self-referential links/nginx server_name; actual connectivity for push/pull
# and the API in this script always goes through 127.0.0.1:${HARBOR_HTTP_PORT}
# directly by IP:port, so the string never needs to resolve via DNS.
sed -i 's/^hostname: .*/hostname: harbor.nexuspaas.local/' harbor.yml
sed -i "s/^  port: 80\$/  port: ${HARBOR_HTTP_PORT}/" harbor.yml
sed -i "s/^harbor_admin_password: .*/harbor_admin_password: ${HARBOR_ADMIN_PASSWORD}/" harbor.yml
# Harbor's default data_volume (/data) is a host path Docker Desktop's file-
# sharing sandbox may not have allow-listed; point it at our own cache dir
# instead, which is already under a path Docker can mount.
sed -i "s|^data_volume: /data\$|data_volume: ${HARBOR_CACHE_DIR}/data|" harbor.yml
# Delete the https: block entirely (verified against the real v2.15.1
# template: exactly this anchor range, 6 lines from "https:" through the
# trailing strong_ssl_ciphers comment) — Harbor's own prepare step otherwise
# expects a real certificate/private_key pair we don't want to generate for
# ephemeral test infra.
sed -i '/^https:/,/# strong_ssl_ciphers: false/d' harbor.yml

echo ">> running Harbor install.sh (no --with-trivy: scan enforcement is out of scope for this smoke lane)"
# Docker Desktop's virtiofs file sharing (macOS, and this sandbox's Linux
# Docker Desktop) can transiently fail to bind-mount a secret file that
# `prepare` just wrote moments earlier (verified: "mountpoint ... is outside
# of rootfs" on first attempt, succeeds immediately on retry with the same
# generated files) — one retry absorbs this without masking a real failure.
${SUDO} ./install.sh || { echo ">> install.sh failed, retrying once (known Docker Desktop virtiofs race)"; ${SUDO} docker compose up -d; }

echo ">> waiting for Harbor to answer /api/v2.0/ping"
ready=
for _ in $(seq 1 60); do
  if curl -fsS "http://127.0.0.1:${HARBOR_HTTP_PORT}/api/v2.0/ping" >/dev/null 2>&1; then ready=1; break; fi
  sleep 5
done
[[ "${ready}" == 1 ]] || { echo "error: Harbor did not become healthy in time" >&2; exit 1; }

cat <<EOF

Harbor ${HARBOR_VERSION} is up at http://127.0.0.1:${HARBOR_HTTP_PORT} (admin/${HARBOR_ADMIN_PASSWORD}).
Seed a project + test image, then run the live e2e:

  make -C backend harbor-seed
  make -C backend e2e-harbor

Tear it down with:

  make -C backend harbor-down
EOF
