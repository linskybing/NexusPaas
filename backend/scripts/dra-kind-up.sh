#!/usr/bin/env bash
# Bring up a GPU-less kind cluster with the upstream dra-example-driver so the
# live DRA E2E (TestLiveK8sConfigFileDRADispatchE2E) can run on a machine that
# has NO NVIDIA GPU.
#
# The example driver advertises *simulated* GPUs via a `gpu.example.com`
# DeviceClass + ResourceSlices. That is all the live test needs: it asserts on
# the generated ResourceClaimTemplate/Pod object shape against a real
# `resource.k8s.io/v1` API server, not on real GPU scheduling.
#
# ponytail: reuse the upstream demo scripts verbatim — we do not reinvent the
# kind config, DRA feature gates, or the driver image build.
set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BACKEND_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

DRA_EXAMPLE_REPO="${DRA_EXAMPLE_REPO:-https://github.com/kubernetes-sigs/dra-example-driver.git}"
DRA_EXAMPLE_REF="${DRA_EXAMPLE_REF:-v0.3.0}"   # v0.3.0 ships kindest/node:v1.36.0 (GA resource.k8s.io/v1)
DRA_EXAMPLE_DIR="${DRA_EXAMPLE_DIR:-${BACKEND_DIR}/.cache/dra-example-driver}"
DRA_DEVICE_CLASS="${DRA_DEVICE_CLASS:-gpu.example.com}"
DRA_NAMESPACE="${DRA_NAMESPACE:-dra-example-driver}"

for tool in docker kind kubectl helm git; do
  command -v "${tool}" >/dev/null 2>&1 || { echo "error: required tool not found: ${tool}" >&2; exit 1; }
done
docker info >/dev/null 2>&1 || { echo "error: docker daemon is not running" >&2; exit 1; }

if [[ ! -d "${DRA_EXAMPLE_DIR}/.git" ]]; then
  echo ">> cloning dra-example-driver ${DRA_EXAMPLE_REF} into ${DRA_EXAMPLE_DIR}"
  git clone --depth 1 --branch "${DRA_EXAMPLE_REF}" "${DRA_EXAMPLE_REPO}" "${DRA_EXAMPLE_DIR}"
fi

cd "${DRA_EXAMPLE_DIR}"
echo ">> creating kind cluster (Kubernetes >= 1.34 for GA resource.k8s.io/v1)"
./demo/clusters/kind/create-cluster.sh
echo ">> building the example driver image and loading it into kind"
./demo/build-driver.sh
echo ">> installing the driver via helm into namespace ${DRA_NAMESPACE}"
helm upgrade -i --create-namespace --namespace "${DRA_NAMESPACE}" \
  dra-example-driver deployments/helm/dra-example-driver

echo ">> waiting for DeviceClass ${DRA_DEVICE_CLASS}"
ready=
for _ in $(seq 1 60); do
  if kubectl get deviceclass "${DRA_DEVICE_CLASS}" >/dev/null 2>&1; then ready=1; break; fi
  sleep 5
done
[[ "${ready}" == 1 ]] || { echo "error: DeviceClass ${DRA_DEVICE_CLASS} did not appear in time" >&2; exit 1; }
kubectl wait --for=condition=Ready pods -n "${DRA_NAMESPACE}" --all --timeout=180s || true

cat <<EOF

DRA fake-GPU cluster is ready (DeviceClass ${DRA_DEVICE_CLASS}).
kubectl is now pointed at the kind cluster. Run the live DRA E2E with:

  make -C backend dra-e2e

Tear the cluster down with:

  make -C backend dra-down
EOF
