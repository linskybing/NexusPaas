#!/usr/bin/env bash
# Delete the kind cluster created by dra-kind-up.sh.
set -Eeuo pipefail

DRA_CLUSTER_NAME="${DRA_CLUSTER_NAME:-dra-example-driver-cluster}"

command -v kind >/dev/null 2>&1 || { echo "error: kind not found" >&2; exit 1; }
kind delete cluster --name "${DRA_CLUSTER_NAME}"
