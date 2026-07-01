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

cd "${HARBOR_CACHE_DIR}/harbor"
docker compose down -v
