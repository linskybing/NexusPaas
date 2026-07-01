#!/usr/bin/env bash
# Seed the Harbor instance started by harbor-up.sh with one public project and
# one pushed image, so live catalog-sync (real /api/v2.0/projects/*/artifacts
# call) has real data to find. Idempotent: tolerates an already-existing
# project (409) and re-pushing the same tag.
set -Eeuo pipefail

HARBOR_HTTP_PORT="${HARBOR_HTTP_PORT:-18080}"
HARBOR_ADMIN_PASSWORD="${HARBOR_ADMIN_PASSWORD:-Harbor12345}"
HARBOR_SEED_PROJECT="${HARBOR_SEED_PROJECT:-nexuspaas-e2e}"
HARBOR_SEED_REPOSITORY="${HARBOR_SEED_REPOSITORY:-smoke}"
HARBOR_SEED_TAG="${HARBOR_SEED_TAG:-v1}"
HARBOR_HOST="127.0.0.1:${HARBOR_HTTP_PORT}"

for tool in docker curl; do
  command -v "${tool}" >/dev/null 2>&1 || { echo "error: required tool not found: ${tool}" >&2; exit 1; }
done

echo ">> creating Harbor project ${HARBOR_SEED_PROJECT} (public)"
create_status="$(curl -s -o /tmp/harbor-seed-create.json -w '%{http_code}' \
  -u "admin:${HARBOR_ADMIN_PASSWORD}" \
  -H 'Content-Type: application/json' \
  -d "{\"project_name\":\"${HARBOR_SEED_PROJECT}\",\"metadata\":{\"public\":\"true\"}}" \
  "http://${HARBOR_HOST}/api/v2.0/projects")"
case "${create_status}" in
  201) echo "   created" ;;
  409) echo "   already exists, continuing" ;;
  *) echo "error: project create returned HTTP ${create_status}: $(cat /tmp/harbor-seed-create.json)" >&2; exit 1 ;;
esac

image_ref="${HARBOR_HOST}/${HARBOR_SEED_PROJECT}/${HARBOR_SEED_REPOSITORY}:${HARBOR_SEED_TAG}"
echo ">> pushing ${image_ref}"
# busybox, not alpine: verified against a real Harbor push that alpine's
# official multi-platform image carries Docker Hub provenance/attestation
# content whose companion push fails client-side with a garbled repository
# name ("invalid repository name: nexuspaas-backend") on this Docker
# version, even though the registry itself accepts the actual image blob
# correctly (confirmed in registry logs). busybox does not trigger this.
docker pull busybox:1.36
docker tag busybox:1.36 "${image_ref}"
echo "${HARBOR_ADMIN_PASSWORD}" | docker login "${HARBOR_HOST}" -u admin --password-stdin
docker push "${image_ref}"

cat <<EOF

Seeded ${image_ref} into Harbor. Run the live e2e with:

  make -C backend e2e-harbor
EOF
