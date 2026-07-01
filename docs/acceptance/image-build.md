# Core Feature Area G: Image Build, Harbor, and Image Allow List

Part of the [GA Acceptance docs](README.md).

## Goal

Users can build images only when platform admin enables image build for the
Project.

Builds must be resource-bounded, scanned, optionally signed, pushed to Harbor,
and added to the Project image allow list only after policy passes.

## GA Target Build Sources

Current implementation note: the routes and fixtures exist, but build-source
handling is not implemented yet. The image-build handlers accept JSON requests
and create queued metadata only; they do not accept multipart tar.gz/zip upload,
parse/extract archives, persist or hash Dockerfile/context content, validate
from-storage permissions, upload context objects, or dispatch a live executor.

| Source | Policy |
|---|---|
| Local Dockerfile + context via CLI | Allowed when Project build capability is enabled |
| tar.gz / zip upload | Allowed when Project build capability is enabled |
| user storage volume | Allowed if storage mount plan permits |
| group storage | Allowed if user has Group/Project access |
| project storage | Allowed if Project role permits |
| hostPath | Allowed only with explicit Project hostPath capability and prefix allow list |

## GA Target Build Flow

This is the required GA flow, not the current runtime behavior.

```text
nexus image build
  -> login and Project selection
  -> build request validates Project permission
  -> require CPU/RAM/max time
  -> scheduler-quota reserves build quota
  -> upload context to object storage or mount approved storage
  -> Tekton PipelineRun starts rootless BuildKit
  -> image built
  -> image pushed to Harbor
  -> SBOM generated
  -> vulnerability scan runs
  -> signature/attestation created if enabled
  -> image digest added to Project allow list only if policy passes
  -> audit event and notification emitted
```

## Image Allow List

Allow list must be digest-based.

Allowed:

```text
registry.nexuspaas.io/project-a/my-image@sha256:<digest>
```

Tags may be displayed, but deployment admission must resolve and enforce digest.

## Acceptance Criteria

| ID | Acceptance Criteria |
|---|---|
| IMG-001 | Project without `allow_image_build` cannot create image build. |
| IMG-002 | Build request must include CPU, RAM, and max build time. |
| IMG-003 | Build exceeding Project Plan build quota is rejected. |
| IMG-004 | CLI can build from local Dockerfile and context without manual tar.gz creation. |
| IMG-005 | Web/API can build from tar.gz or zip. |
| IMG-006 | Build from user storage is allowed only with storage permission. |
| IMG-007 | Build from group storage is allowed only with Group/Project permission. |
| IMG-008 | Build from hostPath is rejected unless hostPath capability allows the prefix. |
| IMG-009 | Build executor must not mount Docker socket. |
| IMG-010 | Default build executor is rootless BuildKit through Tekton or equivalent. |
| IMG-011 | Build logs are available and redact secrets. |
| IMG-012 | Build cancellation terminates build resources and releases quota. |
| IMG-013 | Build timeout terminates build resources and releases quota. |
| IMG-014 | Successful build pushes image to Harbor Project repository. |
| IMG-015 | Image digest is recorded after push. |
| IMG-016 | SBOM is generated or policy explicitly marks SBOM unsupported. |
| IMG-017 | Vulnerability scan result is recorded. |
| IMG-018 | Signature/attestation result is recorded if signing is enabled. |
| IMG-019 | Only passing image digest is added to Project allow list. |
| IMG-020 | ConfigFile using non-allow-listed image is rejected. |
| IMG-021 | ConfigFile using mutable tag resolves to digest before admission. |
| IMG-022 | Harbor deletion sync marks image as deleted or unavailable. |
| IMG-023 | Deleted/unavailable image cannot be used for new deployments. |
| IMG-024 | Image build status can be listed by CLI and Web UI. |
| IMG-025 | Build event includes user, Project, source type, resources, image digest, scan status, and allow-list decision. |

## Current Local Evidence

`ImageAccelerationProfile` now has local metadata and contract evidence in
`image-registry-service`:

- admin CRUD routes under `/api/v1/image-acceleration-profiles`;
- seeded defaults for `standard-overlayfs`, `estargz-gpu-prewarm`, and
  `soci-inference-prewarm`;
- `ImageAccelerationProfileChanged` event fixture and create API fixture;
- focused tests for idempotent seeding, required fields, admin guard, and event
  emission.

This is policy/metadata evidence only. It does not prove image conversion,
lazy-pull runtime support, node prewarm execution, live Harbor build execution,
SBOM/signing, or full image workflow GA.

Image-build source support is currently API-contract evidence only:

- `POST /api/v1/images/build`, `/from-storage`, and `/dockerfile` all queue
  build metadata through the same create path.
- Dockerfile/context/storage-path/build-args fields are fixture/schema shape
  only; current records do not store source content or source digests.
- tar.gz/zip upload and archive extraction are not implemented, so path
  traversal, symlink/hardlink, zip bomb, max-size, max-file-count, and checksum
  controls must be added before archive build sources can be advertised as
  working.
- Idempotency currently proves repeat/conflict semantics for queued metadata,
  but the fingerprint does not include source content or source identity.

Queued image builds now also carry local supply-chain status metadata in the
response, stored record, and `ImageBuildStarted` event:

- `image_digest=""`;
- `allow_list_decision="pending"`;
- `sbom_status="pending"`;
- `signature_status="pending"`;
- `scan_status="pending"`;
- `supply_chain_checked_at=null`.

Given a valid image build request, when it is queued, then those fields are
present with the pending/empty defaults above. Given a historical
`ImageBuildStarted` payload or image-build record without those fields, contract
and listing tests keep schema version `1` valid and treat the missing fields as
unknown. This is IMG-025 event-shape evidence only; it does not prove completed
SBOM generation, scan execution, signing, allow-list admission, live
Tekton/BuildKit/Harbor execution, or image promotion.

## Platform-image supply chain — kind-tier evidence (2026-07-01)

Distinct from the **product** image-build feature above (which remains
API-contract/metadata only), the **platform's own** container image now has a
live kind-tier supply-chain pass in
[`evidence/2026-07-01-kind-live-e2e-report.md`](evidence/2026-07-01-kind-live-e2e-report.md)
via `backend/scripts/kind-live-e2e.sh`:

- BuildKit build of `backend/Dockerfile`;
- syft SPDX SBOM;
- trivy vulnerability scan (`HIGH/CRITICAL=0` at run time);
- cosign keypair generation;
- push/promote/rollback through a local (`localhost:5000`) registry.

Per [`../agents/workflow.md`](../agents/workflow.md) this is single-cluster/local
evidence, **not external GA proof**, and it does **not** enforce SBOM/signing in
the build path, sign+push to an external registry, run the product
Tekton/BuildKit build dispatch, or implement source upload/extraction/hashing.
IMG source handling, live executor, external Harbor push, and enforced
SBOM/scan/sign gates stay OPEN.
