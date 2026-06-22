# Route Auth And Collision Hardening

## 1. Objective

Add one P0 hardening slice that centralizes service authentication for catalog internal routes and prevents silent route collisions.

## 2. Background

The architecture review identified two production risks: internal routes can depend on each handler remembering `X-Service-Key`, and duplicate canonical route shapes can silently shadow later routes. The current runtime is still a modular monolith, so the smallest useful fix is startup/runtime validation in the shared platform layer.

## 3. Source References

- `backend/internal/platform/types.go`
- `backend/internal/platform/app.go`
- `backend/internal/platform/middleware.go`
- `backend/internal/platform/service_client.go`
- `backend/internal/platform/routing.go`
- `backend/internal/services/shared/spec.go`
- `backend/internal/services/*/spec.go`
- `backend/cmd/microservice/main.go`

## 4. Assumptions

- Static `SERVICE_API_KEY` remains the beta service-to-service mechanism.
- External `/api/v1` customer routes remain compatible.
- Raw `Mux.HandleFunc` internal handlers stay allowed only when they call `AuthorizeServiceRequest`.

## 5. Non-Goals

- No workload identity, mTLS, SPIFFE, JWT library replacement, API token prefix lookup, provider abstraction, or typed data migration.
- No broad route rewrite or service extraction.

## 6. Current Behavior

`RegisterService` silently skips duplicate method plus canonical path routes. Some catalog internal routes are regular authenticated routes, while others use handler-level service-key checks.

## 7. Target Behavior

Catalog internal routes explicitly declare service authentication, middleware enforces `X-Service-Key`, and startup validation reports duplicate canonical routes unless they are intentional aliases or overrides.

## 8. Affected Domains

- Platform route registry and middleware.
- Service catalog route metadata.
- Startup safety checks.

## 9. Affected Files

- `backend/internal/platform/types.go`
- `backend/internal/platform/app.go`
- `backend/internal/platform/middleware.go`
- `backend/internal/platform/route_validation.go`
- `backend/internal/platform/routing_test.go`
- `backend/internal/platform/service_auth_test.go`
- `backend/internal/services/shared/spec.go`
- Selected service `spec.go` files with internal routes.
- `backend/cmd/microservice/main.go`

## 10. API / Contract Changes

No external customer API changes. Internal catalog routes now require `X-Service-Key` and return `404` when `SERVICE_API_KEY` is unset or `401` when the key is missing or wrong.

## 11. Database / Migration Changes

None.

## 12. Configuration Changes

None. Existing `SERVICE_API_KEY` is reused.

## 13. Observability Changes

Denied internal requests use the existing request log and span denial attribute.

## 14. Security Considerations

Internal routes fail closed when no service key is configured. Intentional route aliases/overrides must be explicit in code, so a typo cannot silently replace a protected handler.

## 15. Implementation Steps

1. Add `ServiceAuthRequired`, `InternalPublic`, `AliasOf`, `Override`, and `OverrideReason` to `RouteSpec`.
2. Update `shared.ServiceInternal()` and add a small alias option helper.
3. Add a `service-auth` guard to middleware.
4. Add route collision and internal route auth validators.
5. Wire validators into microservice startup checks.
6. Mark catalog internal routes with service-auth metadata and mark intentional storage aliases.
7. Add focused platform and service catalog tests.

## 16. Verification Plan

```sh
go -C backend test ./internal/platform -run 'Route|Internal|ServiceAuth|Admin|Policy' -count=1
go -C backend test ./internal/services -run 'Catalog|Internal|Command|Contract' -count=1
go -C backend test ./... -count=1
go -C backend vet ./...
go -C backend build ./...
bash backend/scripts/ci-security-gate.sh quick
```

Executed on 2026-06-20:

```sh
go -C backend test ./internal/platform -run 'Route|Internal|ServiceAuth|Admin|Policy' -count=1
go -C backend test ./internal/services -run 'Catalog|Internal|Command|Contract' -count=1
go -C backend test ./... -count=1
go -C backend vet ./...
go -C backend build ./...
bash backend/scripts/ci-security-gate.sh quick
bash backend/scripts/ci-security-gate.sh sonar
```

Result: all commands passed. SonarScanner reported `QUALITY GATE STATUS:
PASSED`.

Live RKE2 evidence on 2026-06-20:

- The current live backend image
  `localhost:5000/nexuspaas-backend:ci-ga-ip-20260620134150` includes this
  route hardening code and successfully started all 15 backend deployments in
  namespace `nexuspaas`, so production startup checks accepted the current
  catalog for route collisions and internal route auth metadata.
- `GET /internal/workload/preemption-context` on `workload-service` through
  port-forward returned HTTP 401 without `X-Service-Key`.
- The same request returned HTTP 401 with a wrong `X-Service-Key`.
- The same request returned HTTP 200 with the live service key and standard
  response envelope `{"success":true,"data":{"candidates":[]},...}`.

## 17. Rollback Plan

Revert this branch. No schema, deployment manifest, or persistent data changes are involved.

## 18. Risks and Tradeoffs

Static service keys remain a beta compromise. This slice reduces the chance of forgotten checks but does not provide per-service identity or rotation.

## 19. Reviewer Checklist

- Internal catalog routes are centrally service-authenticated.
- Duplicate canonical routes are rejected unless explicitly documented.
- Existing public APIs remain compatible.
- Tests cover missing, wrong, and valid service keys.

## 20. Status

Status: Implemented and reviewer-verified.

Reviewer Agent: Approved and verified. The implementation matches the approved
plan, passes focused/full Go tests, vet, build, quick gate, Sonar Quality Gate,
and live RKE2 internal-route service-auth evidence. Residual static shared
service key risk is documented and accepted for this beta hardening slice.
