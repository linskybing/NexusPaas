# platform-gateway / BFF

Category: Edge | Phase: 1 (built first)

## 1. Overview

The platform's single external entry point. Responsible for Ingress, CORS, rate limiting, API versioning, JWT cookie extraction, request routing, response envelope compatibility, and the entry point for WebSocket/proxy traffic. In phase 1 it preserves the existing `/api/v1` paths and response schema so the frontend and CLI need no simultaneous rewrite, and it serves as the control point for gradually shifting traffic from the monolith to microservices.

## 2. Responsibilities

- External `/api/v1` compatibility layer: paths, OpenAPI, and the standard JSON response envelope remain unchanged (NFR-COMPAT-01/02).
- Authentication entry: extraction and initial validation of API Key (X-API-Key), JWT Bearer, httpOnly Cookie, and User API Token (JWKS provided by identity-service).
- JWT-only proxy entry: browser pop-ups, iframes, img tags, and WebSockets cannot reliably send custom API Key headers (FR-RBAC-07), so the Gateway unifies cookie/JWT validation and forwarding.
- Rate limiting, CORS, and request_id/trace_id injection and propagation.
- Routing to downstream microservices (see [docs/api-route-mapping.md](../docs/api-route-mapping.md)).
- Aggregated health/readiness and OpenAPI/Swagger exposure (external portion of FR-K8S-01).

## 3. Owned Data

Owns no core business data; only minimal route/config/cache state (e.g., JWKS cache, policy decision cache, rate-limit counters).

## 4. Current Code/Route Mapping

- All external `/api/v1` entry points
- `/metrics`, `/openapi.*`
- All proxy route entry points

## 5. Dependencies

| Dependency | Purpose |
| --- | --- |
| identity-service | Token validation via JWKS, session/token verification |
| authorization-policy-service | PDP authorization decisions, cache invalidation on PolicyChanged |
| All downstream services | Request forwarding |

## 6. Subscribed Events

| Event | Purpose |
| --- | --- |
| PolicyChanged / ProxyPolicyChanged | Invalidate RBAC caches |
| Job lifecycle events | WebSocket status push (shared with k8s-control-service) |
| AnnouncementPublished | Unread count push |

## 7. Non-Functional Highlights

- Long-lived WebSocket/proxy connections must scale independently with configurable timeouts (NFR-SCALE-01).
- Downstream calls need timeouts, retry with backoff, and circuit breakers (NFR-RES-01).
- When a downstream service is unavailable, respond with a clear degraded state without blocking unrelated domains (NFR-RES-02).

## 8. Decomposition Notes

The first service to build. Initially adopt the strangler-fig pattern: route 100% of traffic back to the monolith, then switch upstreams service by service — each switch independently rollback-able.
