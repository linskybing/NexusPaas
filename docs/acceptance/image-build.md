# Core Feature Area G: Image Build, Harbor, and Image Allow List

Part of the [GA Acceptance docs](README.md).

## Goal

Users can build images only when platform admin enables image build for the
Project.

Builds must be resource-bounded, scanned, optionally signed, pushed to Harbor,
and added to the Project image allow list only after policy passes.

## Supported Build Sources

| Source | Policy |
|---|---|
| Local Dockerfile + context via CLI | Allowed when Project build capability is enabled |
| tar.gz / zip upload | Allowed when Project build capability is enabled |
| user storage volume | Allowed if storage mount plan permits |
| group storage | Allowed if user has Group/Project access |
| project storage | Allowed if Project role permits |
| hostPath | Allowed only with explicit Project hostPath capability and prefix allow list |

## Build Flow

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
