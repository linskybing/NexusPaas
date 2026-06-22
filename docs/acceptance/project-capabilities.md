# Core Feature Area B: Project Capability Gates

Part of the [GA Acceptance docs](README.md).

## Goal

High-risk workload permissions must be controlled by platform admin, not by user
YAML.

The following capabilities require explicit Project-level approval:

- Run container as root.
- Privileged container.
- External network egress.
- HostPath mount.
- hostIPC / hostPID / hostNetwork.
- Image build permission.
- WebRTC GUI streaming permission.
- Advanced DRA / ResourceClaimTemplate customization.

## ProjectCapability Model

```text
ProjectCapability
- id
- project_id
- allow_run_as_root
- allow_privileged
- allow_external_egress
- allowed_egress_profiles
- allow_host_path
- allowed_host_path_prefixes
- allow_host_network
- allow_host_pid
- allow_host_ipc
- allow_webrtc
- allow_image_build
- allow_custom_resource_claim_template
- starts_at
- expires_at
- approved_by
- approval_reason
- created_at
- updated_at
```

## Root Policy

`allow_run_as_root` only permits `runAsUser: 0`.

It does not imply:

- `privileged: true`
- `allowPrivilegeEscalation: true`
- Linux capabilities
- hostPath access
- host network
- host PID
- host IPC

Default even when root is allowed:

```yaml
allowPrivilegeEscalation: false
readOnlyRootFilesystem: true
capabilities:
  drop:
    - ALL
```

## External Egress Policy

Default Project network posture:

- deny ingress
- deny egress
- allow DNS only through platform policy
- allow required platform internal endpoints only

External egress must use named profiles. Example profiles:

| Profile | Allowed Destination |
|---|---|
| `dns-only` | Platform DNS only |
| `package-mirror` | Approved apt/pip/npm mirrors |
| `license-server` | Approved vendor license endpoints |
| `http-proxy` | Platform HTTP proxy only |
| `turn-only` | TURN/STUN endpoints for WebRTC |
| `internet-open` | Exceptional approval only |

## HostPath Policy

HostPath is forbidden by default. If approved, it must be prefix-scoped.

Always forbidden:

```text
/
/etc
/proc
/sys
/dev
/var/run/docker.sock
/var/lib/kubelet
/var/lib/containerd
/var/lib/rancher
/run/containerd
/run/docker.sock
```

## Acceptance Criteria

| ID | Acceptance Criteria |
|---|---|
| CAP-001 | Without `allow_run_as_root`, a workload with `runAsUser: 0` is rejected. |
| CAP-002 | With `allow_run_as_root`, root is allowed but privilege escalation remains denied unless separately approved. |
| CAP-003 | Without `allow_privileged`, `privileged: true` is rejected. |
| CAP-004 | Without `allow_external_egress`, workloads cannot reach the public internet. |
| CAP-005 | DNS must continue to work through the approved DNS policy when default-deny egress is enabled. |
| CAP-006 | Egress profiles are enforced by NetworkPolicy or equivalent CNI policy. |
| CAP-007 | Without `allow_host_path`, all hostPath mounts are rejected. |
| CAP-008 | With `allow_host_path`, only approved prefixes are allowed. |
| CAP-009 | Dangerous hostPath prefixes are always rejected even if hostPath is enabled. |
| CAP-010 | hostNetwork, hostPID, and hostIPC are separately controlled and never implied by root permission. |
| CAP-011 | Project admin cannot grant these capabilities; only platform admin can. |
| CAP-012 | Capability changes generate audit events with approver, reason, and expiry. |
| CAP-013 | Expired capability blocks new workloads immediately. |
| CAP-014 | Existing workloads using expired capability are handled by documented grace-period policy. |
| CAP-015 | Capability validation is enforced both in platform preflight and cluster admission. |
