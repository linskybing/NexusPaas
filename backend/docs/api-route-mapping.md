# API Route → Target Service Mapping

In phase 1, platform-gateway preserves the external `/api/v1` compatibility and forwards internally according to the table below.

| Current Route Scope | Target Service |
| --- | --- |
| /api/v1/login, /logout, /refresh, /register, /captcha, /me/api-tokens, /users, /oidc/*, /cli/login, /me/cli-ca | identity-service |
| /api/v1/permissions/*, /api/v1/admin/proxy-rbac/* | authorization-policy-service |
| /api/v1/groups, /api/v1/user-groups, /api/v1/projects, /api/v1/projects/{id}/members, /projects/{id}/workspace-settings, /projects/{id}/gpu-claims | org-project-service |
| /api/v1/configfiles, /api/v1/jobs | workload-service |
| /api/v1/plans, /api/v1/queues, quota/preemption internal APIs | scheduler-quota-service |
| /api/v1/k8s/*, /api/v1/resources/*, /api/v1/ws/* | k8s-control-service |
| /api/v1/cluster/* | split between k8s-control-service / usage-observability-service |
| /api/v1/ide/* | ide-service |
| /api/v1/storage/*, /api/v1/projects/{id}/storage/*, /api/v1/admin/user-storage/*, /api/v1/admin/group-storage | storage-service |
| /api/v1/image-requests, /api/v1/images/*, /api/v1/image-catalog, /api/v1/projects/{id}/images, /api/v1/harbor-status | image-registry-service |
| /api/v1/me/usage, /api/v1/me/gpu/jobs, /api/v1/me/request-usage, /api/v1/admin/usage, /api/v1/admin/request-usage, /api/v1/admin/gpu/users, /api/v1/dashboard/* | usage-observability-service |
| /api/v1/audit/*, /api/v1/admin/security/posture | audit-compliance-service |
| /api/v1/forms, /api/v1/notifications, /api/v1/announcements, /api/v1/admin/announcements | request-notification-service |
| /api/v1/grafana/*, /api/v1/minio-console/*, /api/v1/pgadmin/*, /api/v1/longhorn/*, /api/v1/harbor/* UI proxy, /api/v1/admin/vpn | integration-proxy-service |
| /api/v1/uploads/images/* | media-upload-service |
| /healthz, /readyz, /metrics, /openapi.* | platform-gateway (aggregated) + each service individually |
