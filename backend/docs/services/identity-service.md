# identity-service

Category: Core | Phase: 4

## 1. Overview

The authentication and account center. Responsible for login/logout/refresh, JWT/JWKS, User API Tokens, CLI login/CA, LDAP and local-credential strategies, user and system role management, and acting as the platform's OIDC Identity Provider (SSO for external services such as Grafana, MinIO, Harbor, and pgAdmin).

## 2. Functional Requirements

| ID | Requirement | Acceptance/Notes |
| --- | --- | --- |
| FR-AUTH-01 | Provide login, logout, registration, refresh token, and CAPTCHA APIs, supporting both JWT Bearer Token and httpOnly Cookie for browser/CLI scenarios. | /api/v1/login, /logout, /register, /refresh, /captcha. |
| FR-AUTH-02 | Support frontend platform API Key validation and user API Tokens for CLI or automation access. | Browser API uses X-API-Key + JWT; CLI may use revocable nexuspaas_ user API tokens. |
| FR-AUTH-03 | Provide personal API Token creation, listing, revocation, and revoke-current-token. | /api/v1/me/api-tokens. |
| FR-AUTH-04 | Support CLI login and CLI CA certificate download; CLI login must have failure-count and lockout mechanisms. | /api/v1/cli/login, /api/v1/me/cli-ca. |
| FR-AUTH-05 | Support local credentials and LDAP identity sources; when LDAP is enabled, try LDAP first while keeping the local DB as admin and fallback login source. | Derived from the auth Resolver and LDAP/LocalDB strategies. |
| FR-AUTH-06 | Act as an OIDC Identity Provider with discovery, JWKS, authorize, token, userinfo, revoke, and login callback. | SSO for Grafana, MinIO, Harbor, pgAdmin, etc. |
| FR-AUTH-07 | Provide user profiles, paginated lists, batch create, batch delete, batch password reset, batch role updates, user resolution, and personal settings management. | /api/v1/users and /api/v1/users/{id}/settings. |
| FR-AUTH-08 | Maintain platform system roles, user status, role capabilities, and a reserved admin account; support admin password initialization and forced reset. | Derived from users, roles, config seed/migration. |

## 3. Owned Data

`users`, `sessions`, `refresh_tokens`, `user_api_tokens`, `roles`, credential audit snapshots.

## 4. Current Code/Route Mapping

- Handlers: `auth.go`, `user.go`, `me.go`, `oidc.go`
- Domain: `domain/auth`, `domain/user`, `domain/role`
- Routes: `/api/v1/login`, `/logout`, `/refresh`, `/register`, `/captcha`, `/me/api-tokens`, `/users`, `/oidc/*`, `/cli/login`, `/me/cli-ca`

## 5. External Interfaces

- REST/gRPC: `ValidateToken`, `GetUser`, `ListUsers`
- JWKS endpoint so the Gateway and other services can validate tokens locally

## 6. Published Events

| Event | Subscribers | Purpose |
| --- | --- | --- |
| UserCreated / UserUpdated / UserDisabled | authorization-policy, org-project, audit-compliance, request-notification | Sync display names, role/status caches, and audit |

## 7. Non-Functional Highlights

- JWT/LDAP/OIDC keys must be rotatable (NFR-SEC-03).
- Login, token, and session endpoints are read-heavy and need Redis caching (NFR-PERF-03).
- All account-management operations must produce AuditEvents (NFR-SEC-05).

## 8. Decomposition Notes

Phase 4, together with authorization-policy-service — higher risk: requires dual writes, cache invalidation, and contract tests. May be co-deployed with Authz initially while keeping code and data boundaries.
