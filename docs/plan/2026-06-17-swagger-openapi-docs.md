# Swagger OpenAPI Docs

## 1. Objective

Expose usable Swagger/OpenAPI documentation for the backend API so operators can
inspect the route contract through Swagger UI, while preserving existing runtime
behavior and auth policy.

## 2. Background

The backend already exposes `/openapi.json`, but the generated document contains
mostly path entries and platform `x-*` metadata. It lacks standard operation
responses, parameters, security schemes, and reusable response-envelope schemas,
which makes the contract weak for Swagger UI and automated API inspection.

## 3. Source References

- `backend/internal/platform/openapi.go`
- `backend/internal/platform/endpoints.go`
- `backend/internal/platform/openapi_test.go`
- `backend/internal/services/catalog_test.go`
- `docs/agents/workflow.md`
- `docs/agents/planning.md`
- `docs/agents/review-checklist.md`
- OpenAPI 3.1 operation and security object requirements
- Swagger UI configuration for inline specs

## 4. Assumptions

- This slice delivers route-level API documentation, not complete typed
  per-endpoint request/response DTO schemas.
- `/swagger/` is an operational API documentation endpoint and remains admin-only
  when runtime auth is enabled.
- The implementation should not introduce a new OpenAPI framework or generator
  dependency.

## 5. Non-Goals

- Do not change any domain service handler behavior.
- Do not change database schemas, migrations, deployment manifests, or service
  ownership boundaries.
- Do not document every endpoint payload with fully typed schemas in this slice.
- Do not make API docs public in production.

## 6. Current Behavior

`GET /openapi.json` returns OpenAPI `3.1.0` with route paths and platform
extensions. The document does not include standard responses, reusable schemas,
path parameters, operation-level security, or a browser Swagger UI endpoint.

## 7. Target Behavior

`GET /openapi.json` returns a valid, Swagger-friendly OpenAPI 3.1 route contract.
`GET /swagger/` returns an HTML Swagger UI page with the generated spec embedded
directly in the page, so authenticated access to the documentation does not need
a second unauthenticated spec fetch.

## 8. Affected Domains

- Platform runtime operational endpoints
- OpenAPI route-contract generation
- Platform/service catalog tests covering documentation and operational auth

## 9. Affected Files

- `backend/internal/platform/openapi.go`
- `backend/internal/platform/endpoints.go`
- `backend/internal/platform/openapi_test.go`
- `backend/internal/services/catalog_test.go`
- `docs/plan/2026-06-17-swagger-openapi-docs.md`

## 10. API / Contract Changes

- Add `GET /swagger/` as an admin operational endpoint.
- Enrich `/openapi.json` with:
  - operation tags, summaries, responses, security, and parameters;
  - Swagger-safe path templates for catch-all runtime routes;
  - reusable `Envelope`, `ErrorBody`, `Degraded`, and `GenericData` schemas;
  - Bearer JWT and `X-API-Key` security schemes.

## 11. Database / Migration Changes

None.

## 12. Configuration Changes

None.

## 13. Observability Changes

No new metrics or traces. `/swagger/` uses the existing platform operational
endpoint wrapper, so request logs, request IDs, trace IDs, auth, rate limiting,
and policy checks remain consistent with `/openapi.json`.

## 14. Security Considerations

- `/swagger/` must use the same admin-only operational gate as `/openapi.json`.
- Protected routes in the OpenAPI document must declare Bearer JWT or `X-API-Key`
  security alternatives.
- Public routes must not declare operation-level security.
- The Swagger HTML must not embed secrets or runtime credentials.

## 15. Implementation Steps

1. Extend the OpenAPI generator with helper functions for Swagger-safe paths,
   path parameter extraction, operation metadata, responses, security, and
   reusable components.
2. Preserve existing platform `x-*` extensions and add `x-runtime-pattern` /
   `x-catch-all` for runtime catch-all routes.
3. Add a Swagger HTML renderer that embeds `app.OpenAPI()` as inline JSON and
   bootstraps Swagger UI from the public Swagger UI CDN.
4. Register `GET /swagger/` as an operational endpoint using the existing admin
   wrapper.
5. Expand OpenAPI unit tests for responses, parameters, security, components,
   and catch-all route rendering.
6. Expand service catalog endpoint tests for `/swagger/` content type, HTML
   contents, and operational auth behavior.

## 16. Verification Plan

- `cd backend && go test ./internal/platform ./internal/services -count=1`
- `cd backend && go test ./... -count=1`
- `cd backend && go test ./... -coverprofile=coverage.out -count=1`
- `cd backend && go vet ./...`
- `cd backend && go build ./...`
- `cd backend && bash scripts/ci-security-gate.sh quick`
- Run SonarScanner quality gate if the local environment is configured; otherwise
  record it as not run.

## 17. Rollback Plan

Revert the OpenAPI generator changes, `/swagger/` endpoint registration, tests,
and this plan file. No database, config, or deployment rollback is required.

## 18. Risks and Tradeoffs

- Route-level schemas improve Swagger usability but are less precise than fully
  typed endpoint DTOs.
- Embedding the spec avoids authenticated double-fetch problems, but relies on
  externally hosted Swagger UI assets unless a future slice vendors them.
- Catch-all path conversion must preserve the runtime pattern in extensions so
  operators can distinguish displayed Swagger paths from Go runtime routes.

## 19. Reviewer Checklist

- Requirement fit: Swagger UI exists and `/openapi.json` is more complete.
- Scope control: no domain handler, DB, deployment, or config changes.
- API contract: `/swagger/` is documented as an admin operational endpoint.
- Security: operational admin gate is preserved and OpenAPI security is accurate.
- Testing: OpenAPI generation and endpoint auth/content behavior are covered.
- Simplicity: no new framework dependency or speculative typed schema inventory.

## 20. Status

Status: Approved
