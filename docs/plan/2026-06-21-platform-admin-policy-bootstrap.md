# Platform Admin Policy Bootstrap

Status: Implemented; Reviewer approved

Reviewer: Final implementation review approved

## Objective

Let configured platform admin API key principals pass the existing PDP for
public API catalog routes, so live `/api/v1/projects` can reach
`org-project-service` through the gateway proxy without bypassing service-level
authorization.

## Current Evidence

- Before this slice, the gateway downstream proxy was live: `GET
  /api/v1/projects` through `platform-gateway` returned `403`, not `405`.
- Before this slice, direct `org-project-service` `GET /api/v1/projects` also
  returned `403`.
- The configured static principal was admin, but the raw-policy PDP had no exact
  policy row for the normalized admin subject, empty domain,
  `org-project-service:projects`,
  `get_org_project_service_api_v1_projects`.
- After this slice, live direct `org-project-service` and live
  `platform-gateway` requests return `HTTP 200` with an authorized empty project
  list.

## Scope

- In `authorization-policy-service` startup registration, reconcile raw
  permission policies for configured `API_KEY_PRINCIPALS` with `admin: true` or
  admin role.
- Bootstrap subjects must be `APIKeyPrincipal.normalized().ID`, matching the
  `X-User-ID` value used by request auth and `policyAllowed`.
- Export the platform principal normalization through a tiny helper or method
  such as `APIKeyPrincipal.Normalized()`, and use that helper from
  authorization-policy. Do not duplicate normalization/admin-role logic in the
  authorization-policy package.
- Mark bootstrap-created raw policy rows with metadata so they are identifiable
  as bootstrap-managed.
- On startup, delete only bootstrap-managed rows that are no longer desired for
  currently explicit admin principals. Never delete manually created policies.
- Seed only catalog routes that actually participate in PDP:
  - `AuthRequired == true`,
  - `PolicyBypass == false`,
  - skip `ServiceAuthRequired`,
  - skip `/internal/*`,
  - skip `/api/v1/internal/*`.
- Use each route's existing `Resource` and `OperationID`; do not introduce a new
  policy model.
- Keep non-admin principals unchanged.
- Keep gateway and downstream services using the normal PDP path; no route
  `PolicyBypass` and no gateway-only authorization shortcut.
- Add focused authorization-policy tests proving:
  - admin principals get public catalog raw policies,
  - non-admin principals do not,
  - admin-to-non-admin downgrade/removal deletes only bootstrap-managed rows,
  - manually created policies are preserved,
  - internal/service-auth routes are not seeded,
  - unauthenticated/public and `PolicyBypass` routes are not seeded,
  - subject uses normalized principal ID, including `user_id` fallback,
  - seeding is idempotent.
- Update this plan and `gap.md` with local and live evidence.

## Non-Goals

- No broad rewrite of RBAC or Casbin-style grouping.
- No WebRPC transport.
- No manual live-only database patch as the long-term fix.
- No OIDC browser login in this slice.

## Affected Files

- `backend/internal/services/authorizationpolicy/handler.go`
- `backend/internal/services/authorizationpolicy/seed.go`
- `backend/internal/services/authorizationpolicy/raw_permission_repository.go`
- `backend/internal/services/authorizationpolicy/*_test.go`
- `backend/internal/platform/api_key.go`
- `backend/internal/platform/routing.go`
- `backend/internal/platform/*_test.go`
- `docs/plan/2026-06-21-platform-admin-policy-bootstrap.md`
- `gap.md`

## Verification

```sh
go -C backend test ./internal/services/authorizationpolicy
go -C backend test ./internal/platform
go -C backend test ./...
bash backend/scripts/ci-security-gate.sh quick
docker build -f backend/Dockerfile -t <image> .
kubectl -n nexuspaas set image deployment/authorization-policy-service app=<image>
kubectl -n nexuspaas set image deployment/platform-gateway app=<image>
kubectl -n nexuspaas rollout status deployment/authorization-policy-service --timeout=180s
kubectl -n nexuspaas rollout status deployment/platform-gateway --timeout=180s
curl -i -H "X-API-Key: <admin-key>" http://127.0.0.1:18080/api/v1/projects
curl -i -H "X-API-Key: <admin-key>" http://127.0.0.1:18081/api/v1/projects
# or POST /api/v1/permissions/enforce proving:
# sub=admin, dom="", obj=org-project-service:projects,
# act=get_org_project_service_api_v1_projects => allowed
NEXUSPAAS_E2E_API_KEY=<admin-key> npm --prefix frontend run e2e
git diff --check
```

## Implementation Evidence

- Exported `APIKeyPrincipal.Normalized()` so bootstrap seeding uses the same
  subject semantics as request authentication and `policyAllowed`.
- Added startup reconciliation in `authorization-policy-service` for
  bootstrap-managed raw policies for explicit admin API key principals.
- Seeded only catalog routes that participate in PDP:
  authenticated, non-bypass, non-service-auth, non-internal routes with
  resource and operation metadata.
- Tagged bootstrap-created records with managed metadata and reconciled stale
  bootstrap rows without deleting manual raw policies.
- Added repository helpers for raw policy create/list/delete so seeding stays
  behind the service repository boundary.
- Local verification passed:
  - `go -C backend test ./internal/services/authorizationpolicy`
  - `go -C backend test ./internal/platform`
  - `go -C backend test ./...`
  - `bash backend/scripts/ci-security-gate.sh quick`
- Final image built and pushed:
  `localhost:5000/nexuspaas-backend:ci-ga-admin-policy-20260621020259`
  (`sha256:72c7c2ec0284b0aaec2defd277d8bfc56096e66d08724a1fb85333002b2ee38a`).
- Rolled `nexuspaas/authorization-policy-service` and
  `nexuspaas/platform-gateway`; both reached ready `1/1`.
- Live direct evidence: `GET /api/v1/projects` on `org-project-service` with
  the runtime admin API key returned `HTTP 200` and
  `{"success":true,"data":[]}`.
- Live gateway evidence: `GET /api/v1/projects` through `platform-gateway` with
  the runtime admin API key returned `HTTP 200` and
  `{"success":true,"data":[]}`.
- Live GUI E2E passed:
  `NEXUSPAAS_E2E_API_KEY=<runtime-secret> npm --prefix frontend run e2e`.
  Screenshot: `frontend/test-results/gui-live-smoke.png`.

## Reviewer Result

- Reviewer found no blocking findings and approved final implementation review.
- Reviewer reran focused regression:
  `go -C backend test ./internal/platform ./internal/services/authorizationpolicy -run 'GatewayCatalogProxy|AdminBootstrap|APIKeyPrincipalNormalized' -count=1`
  and it passed.
- Non-blocking residual risk: bootstrap reconcile currently ignores
  create/delete errors; harden in a later slice if policy-store observability is
  prioritized.

## Acceptance

- Admin API key principal can pass PDP for live `GET /api/v1/projects`.
- Live `/ui/` no longer shows `Projects unavailable` for the project selector
  when the downstream service returns a normal response.
- Non-admin static principals do not receive bootstrap policies.
- Internal/service-auth routes are not exposed by the bootstrap.
