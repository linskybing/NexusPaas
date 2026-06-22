# Complete Requirement Coverage Matrix

Part of the [GA Acceptance docs](README.md). Section numbers refer to the
original document structure; follow the per-area docs linked from the
[hub](README.md).

| User Requirement | Target Section | Required Acceptance IDs |
|---|---|---|
| Users deploy Kubernetes YAML ConfigFile in Project | Section 5 ([k8s-deployment](k8s-deployment.md)) | K8S-001 to K8S-018 |
| ConfigFile security review for multi-tenancy | Section 5, 6, 16 | K8S-005, K8S-018, CAP-001 to CAP-015, SEC-015 |
| Root requires platform admin approval | Section 6 ([project-capabilities](project-capabilities.md)) | CAP-001, CAP-002, CAP-011 |
| External network requires platform admin approval | Section 6 | CAP-004, CAP-006, CAP-011 |
| HostPath requires platform admin approval | Section 6 | CAP-007, CAP-008, CAP-009, CAP-011 |
| WebRTC GUI container through browser | Section 10 ([webrtc](webrtc.md)) | RTC-001 to RTC-018 |
| GPU uses DRA claim primarily | Section 8 ([gpu-dra-mps](gpu-dra-mps.md)) | GPU-001 to GPU-006 |
| MPS for fine-grained GPU control | Section 8, 9 | GPU-007 to GPU-018, USAGE-008 to USAGE-018 |
| All deployments go through Queue | Section 7 ([plan-queue-quota](plan-queue-quota.md)) | QUEUE-001 to QUEUE-022 |
| Queue defines max runtime | Section 7 | QUEUE-010 |
| Queue defines GPU type | Section 7 | QUEUE-004, QUEUE-005 |
| Queue defines preemptible and priority | Section 7 | QUEUE-011 to QUEUE-016 |
| High priority preempts lower preemptible Queue | Section 7 | QUEUE-011 to QUEUE-016 |
| Deployment allowed by Plan | Section 7 | QUEUE-001 to QUEUE-008 |
| Plan controls Queue/time/CPU/RAM/GPU/expiry | Section 7 | QUEUE-001 to QUEUE-008 |
| Project attaches Plan | Section 7, 13 | QUEUE-001, RBAC-related Project controls |
| Image build requires admin-enabled Project permission | Section 11 ([image-build](image-build.md)) | IMG-001 |
| Build supports Dockerfile/tar/storage/hostPath | Section 11 | IMG-004 to IMG-008 |
| Build must specify CPU/RAM/time | Section 11 | IMG-002 |
| Build pushes to Harbor | Section 11 | IMG-014 |
| Built image added to allow list after policy pass | Section 11 | IMG-019 |
| ConfigFile image must obey Project allow list | Section 11 | IMG-020, IMG-021 |
| CLI login | Section 12 ([cli](cli.md)) | CLI-001 to CLI-005 |
| CLI build from local context | Section 12 | CLI-006 to CLI-009 |
| CLI query Project build/image list | Section 12 | CLI-009 |
| Platform roles user/manager/admin | Section 13 ([rbac](rbac.md)) | RBAC-001 to RBAC-020 |
| Platform admin highest and not removable | Section 13 | RBAC-013 |
| Group/Project structure | Section 13 | RBAC-004 to RBAC-009 |
| Project can invite non-Group users | Section 13 | RBAC-007 to RBAC-009 |
| Each user has personal Project | Section 13 | RBAC-001 to RBAC-003 |
| Monitor all users' GPU usage | Section 9, 14 ([usage-attribution](usage-attribution.md), [monitoring](monitoring.md)) | USAGE-001 to USAGE-037, MON-001 to MON-020 |
| Real-time SM/memory ratio | Section 9, 14 | USAGE-012 to USAGE-018, MON-002 to MON-006 |
| Group time-range usage statistics | Section 14 | MON-007 to MON-012 |
| MPS shared GPU user attribution by container PID | Section 9 | USAGE-006 to USAGE-018, USAGE-035 to USAGE-037 |
| Process granularity using process-exporter | Section 9 | USAGE-004, USAGE-010, USAGE-023, USAGE-036 |
| Container-based ownership attribution | Section 9 | USAGE-001 to USAGE-011 |
