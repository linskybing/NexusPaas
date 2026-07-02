#!/usr/bin/env bash
# Tear down the Harbor instance started by harbor-up.sh.
set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BACKEND_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

HARBOR_VERSION="${HARBOR_VERSION:-v2.15.1}"
HARBOR_CACHE_DIR="${HARBOR_CACHE_DIR:-${BACKEND_DIR}/.cache/harbor-${HARBOR_VERSION}}"

if [[ ! -d "${HARBOR_CACHE_DIR}/harbor" ]]; then
  echo "no Harbor install found at ${HARBOR_CACHE_DIR}/harbor; nothing to do"
  exit 0
fi

# See harbor-up.sh: Harbor's own containers write common/config/*/env
# unmapped (owned by root without userns-remap), so reading them back to
# resolve env_file directives needs root on a native Linux Docker Engine.
SUDO=""
[[ "$(id -u)" -eq 0 ]] || SUDO="sudo"

cd "${HARBOR_CACHE_DIR}/harbor"
${SUDO} docker compose down -v
# data_volume is a host bind mount (see harbor-up.sh), not a Docker-managed
# volume, so `down -v` does not remove it; clean it up explicitly so repeated
# local up/down cycles start from a real clean slate instead of silently
# reusing stale Postgres/secret data. Some files under data/ are root-owned
# (see above), so this also needs elevation.
${SUDO} rm -rf "${HARBOR_CACHE_DIR}/data"
