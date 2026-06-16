# integration-proxy-service

Category: Edge/Tools | Phase: 2

## 1. Overview

The external-tool UI proxy service. Responsible for reverse-proxying Grafana, MinIO Console, pgAdmin, Longhorn, and the Harbor UI, plus SSO callback adapters, proxy auth-check, and VPN administration. Can independently tune timeouts, streaming, and iframe/cookie behavior, reducing the risk of external-system timeouts dragging down the core API.

## 2. Functional Requirements

| ID | Requirement | Acceptance/Notes |
| --- | --- | --- |
| FR-PROXY-01 | Proxy Grafana with Proxy RBAC controlling service access. | /grafana/*. |
| FR-PROXY-02 | Proxy the MinIO Console with SSO login support. | /minio-console-sso, /minio-console/*. |
| FR-PROXY-03 | Proxy pgAdmin with SSO login and nginx auth-check support. | /pgadmin-sso, /pgadmin-auth-check, /pgadmin/*. |
| FR-PROXY-04 | Proxy the Longhorn UI using Proxy RBAC. | /longhorn/*. |
| FR-PROXY-05 | Provide VPN client list, usage, and disconnect-client administration. | /admin/vpn. |
| FR-IMAGE-07 (shared) | Harbor UI reverse proxy (the admin API integration stays in image-registry-service). | /harbor/* UI proxy. |

## 3. Owned Data

Proxy sessions/caches; **owns no core policy** — all access decisions come from authorization-policy-service.

## 4. Current Code/Route Mapping

- Handlers: `grafana.go`, `minio.go`, `pgadmin.go`, `longhorn.go`, parts of `harbor.go`
- Routes: `/api/v1/grafana/*`, `/api/v1/minio-console/*`, `/api/v1/pgadmin/*`, `/api/v1/longhorn/*`, `/api/v1/harbor/*` (UI), `/api/v1/admin/vpn`

## 5. Dependencies

| Dependency | Purpose |
| --- | --- |
| identity-service | OIDC SSO (Grafana/MinIO/pgAdmin/Harbor authenticate against the platform IdP) |
| authorization-policy-service | Proxy RBAC decisions (PDP/SDK) |
| platform-gateway | JWT-only proxy entry and cookie/JWT validation/forwarding |

## 6. Subscribed Events

| Event | Purpose |
| --- | --- |
| ProxyPolicyChanged | Invalidate proxy RBAC caches |
| UserDisabled | Immediately terminate that user's proxy sessions |

## 7. Non-Functional Highlights

- All proxy routes are JWT-only but must still pass Proxy RBAC (NFR-SEC-04, FR-RBAC-07).
- Long-lived/streaming connections need independent timeout settings and horizontal scaling (NFR-SCALE-01).
- When an external tool is unavailable, respond with a clear degraded state without affecting the core API (NFR-RES-02, NFR-AVAIL-01).

## 8. Decomposition Notes

Extracted in phase 2. Browser iframe/cookie behavior is complex (pop-ups, img tags, and WebSockets cannot send custom headers); keep the JWT-only entry and coordinate unified validation with the Gateway.
