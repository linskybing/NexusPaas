# Production Beta Kustomize Deployment

## 1. Objective

Make the backend deployment composition able to render the requested Production
Beta topology: 15 independently deployed service manifests plus gateway,
backing services, shared runtime config, and a documented runtime secret
contract.

## 2. Background

The Production Beta roadmap requires 15 service deployments rather than the
all-in-one `SERVICE_NAME=all` runtime. The repository already contains one
`k8s/deployment.yaml` per service, but the dev k3s kustomization still renders
only `platform.yaml` plus backing services. Because standard `kubectl kustomize`
does not allow a nested overlay to read resources outside its root, the
Production Beta kustomization lives at `backend/kustomization.yaml`, where all
service manifests are below the kustomization root.

## 3. Source References

- `long-term.md`
- `problem.md`
- `backend/kustomization.yaml`
- `backend/deploy/k3s/kustomization.yaml`
- `backend/deploy/k3s/runtime-config.yaml`
- `backend/deploy/k3s/production-beta/backing-secret-names.yaml`
- `backend/deploy/k3s/production-beta/runtime-config-envfrom-patch.yaml`
- `backend/*/k8s/deployment.yaml`
- `backend/identity-service/migrations/0003_login_failures_metadata.sql`
- `backend/internal/platform/deployment_test.go`
- `backend/internal/platform/store_postgres_identity.go`
- `backend/internal/platform/store_postgres_test.go`
- `backend/internal/platform/store_postgres_unit_test.go`
- `sonar-project.properties`
- `backend/docs/api-route-mapping.md`
- `backend/docs/non-functional-requirements.md`

## 4. Assumptions

- PR #2, the Docker E2E gate, is still open and not part of this branch.
- This branch is based on `origin/main` to keep the PR 2 slice independent.
- Existing service deployment manifests are the source of truth for per-service
  probes, resources, PDB, HPA, and security context.
- Production Beta may still use one shared Postgres/Redis backing service as
  transition debt; data-boundary cleanup remains a later roadmap PR.
- Runtime secrets are supplied by the cluster operator or secret manager; this
  PR documents and validates their contract but does not commit real secrets.

## 5. Non-Goals

- Do not remove the existing dev all-in-one `backend/deploy/k3s/kustomization.yaml`.
- Do not add Helm, operators, or new infrastructure controllers.
- Do not implement service mesh, mTLS, or workload identity in this slice.
- Do not change service APIs or route ownership.
- Do not change database schemas except for the additive
  `login_failures.created_at` migration required by integration-test runtime
  correctness.
- Do not run live cluster mutation unless a local k3s context is explicitly
  available and safe.

## 6. Current Behavior

`kubectl kustomize backend/deploy/k3s` renders namespace, runtime config,
Postgres, Redis, MinIO, Dex, and the monolithic `platform.yaml` runtime. It does
not render the 15 existing service manifests.

## 7. Target Behavior

A new production-beta kustomization at `backend/kustomization.yaml` renders:

- namespace and backing services
- shared runtime config with `SERVICE_URLS`
- a non-secret runtime secret contract artifact
- all 15 service manifests, including `platform-gateway`
- no all-in-one `platform.yaml`

The existing dev kustomization continues to render the all-in-one stack.

## 8. Affected Domains

- Deployment composition
- Runtime configuration
- Production Beta readiness tracking
- Kubernetes manifest policy tests

## 9. Affected Files

- `backend/kustomization.yaml`
- `backend/deploy/k3s/production-beta/backing-secret-names.yaml`
- `backend/deploy/k3s/production-beta/runtime-config-envfrom-patch.yaml`
- `backend/deploy/k3s/production-beta/runtime-config.yaml`
- `backend/deploy/k3s/production-beta/runtime-secret-contract.yaml`
- `backend/identity-service/migrations/0003_login_failures_metadata.sql`
- `backend/internal/platform/deployment_test.go`
- `backend/internal/platform/store_postgres_identity.go`
- `backend/internal/platform/store_postgres_test.go`
- `backend/internal/platform/store_postgres_unit_test.go`
- `sonar-project.properties`
- `backend/docs/non-functional-requirements.md` or a focused deployment doc if
  needed

## 10. API / Contract Changes

No HTTP API or event contract changes. The deployment contract changes by
documenting required per-service runtime secret keys and adding `SERVICE_URLS`
for owner-service HTTP contracts.

## 11. Database / Migration Changes

Add `identity-service/migrations/0003_login_failures_metadata.sql` to complete
the `login_failures` metadata columns expected by the identity-owned
PostgresStore path. The migration is additive, backfills `created_at` from
legacy `platform_records.created_at` when available, and falls back to
`updated_at`/`now()` for existing rows.

Production Beta remains on the existing shared Postgres transition model until
the data-boundary cleanup PR retires direct shared-store dependencies.

## 12. Configuration Changes

Add production-beta config values for:

- `REDIS_URL`
- `EVENT_BUS_URL`
- `JWT_AUDIENCE`
- `SERVICE_URLS`
- optional object-store endpoint defaults for the media service through its
  existing manifest

Document the required secret keys for each service runtime secret without
committing any secret values.

## 13. Observability Changes

No new telemetry code. Existing per-service `OTEL_SERVICE_NAME` and resource
attributes remain in each service manifest and become visible in the rendered
production-beta topology.

## 14. Security Considerations

- Do not commit real credentials.
- Keep production manifests on `PRODUCTION=true` and `REQUIRE_AUTH=true`.
- Preserve per-service runtime secret refs.
- Require `SERVICE_API_KEY` when `SERVICE_URLS` is configured.
- Treat scoped service keys as Production Beta transition security, with
  mTLS/workload identity left for GA hardening.

## 15. Implementation Steps

1. Add `backend/kustomization.yaml` that references k3s namespace/backing
   services, Dex, production-beta runtime config, and all 15
   `<service>/k8s/deployment.yaml` files, excluding `deploy/k3s/platform.yaml`.
2. Add production-beta shared runtime config with `SERVICE_URLS` mapping service
   names to in-cluster service URLs.
3. Add production-beta backing-service patches that replace inherited dev
   secret names with production-beta secret names.
4. Add a production-beta runtime config envFrom patch targeted at the 15
   service Deployments, keeping service-owned base manifests environment-neutral.
5. Add a non-secret `runtime-secret-contract.yaml` ConfigMap documenting the
   expected secret names and keys for operators/secret managers.
6. Add or update deployment policy tests to assert:
   - production-beta kustomization references exactly 15 service manifests
   - the all-in-one platform manifest is not included
   - each rendered service has Deployment, Service, PDB, HPA, probes, resources,
     `SERVICE_NAME`, and non-root security settings
   - `SERVICE_URLS` and `SERVICE_API_KEY` contract are present for isolated
     services
7. Update deployment docs/readiness notes as needed.
8. Fix the integration-test-discovered identity login-failures metadata gap with
   an additive migration and keep the owned-table boundary test passing.
9. Keep Sonar IaC/security analysis on Kubernetes manifests while excluding
   manifest YAML from CPD-only duplication checks, because probes, resources,
   selectors, and envFrom contracts are intentionally repeated per workload.

## 16. Verification Plan

- `kubectl kustomize backend/deploy/k3s`
- `kubectl kustomize backend`
- `cd backend && go test ./internal/platform -run Deployment -count=1`
- `cd backend && env TEST_DATABASE_URL=<postgres-url> go test -tags integration ./internal/platform -run TestPostgresStoreIdentityResourcesUseOwnedTables -count=1`
- `cd backend && go test ./... -count=1`
- `cd backend && go build ./...`
- `cd backend && go vet ./...`

If a live local k3s context is safe and available, optionally run a dry-run
server-side apply or namespace smoke. Otherwise record local cluster smoke as
not run.

## 17. Rollback Plan

Delete `backend/kustomization.yaml`, the production-beta runtime config/contract
files, and revert deployment policy tests/docs plus the Sonar CPD-only manifest
exclusion. The existing dev all-in-one kustomization remains untouched and can
continue to deploy the monolith.

The additive `login_failures.created_at` migration should not be rolled back by
dropping data-bearing metadata in a live database. If rollback is required
after applying it, roll application code back and leave the column in place, or
restore the database from a backup/snapshot taken before migration.

## 18. Risks and Tradeoffs

- This PR enables 15-service rendering but does not prove full live readiness in
  a cluster.
- Static service keys remain a Production Beta transition compromise.
- Shared database/runtime dependencies remain explicit transition debt.
- The identity login-failures migration changes schema metadata only; rollback
  should leave the column in place or restore from backup rather than dropping
  data-bearing metadata in a live environment.
- Maintaining both dev all-in-one and production-beta kustomizations creates
  duplication but keeps rollout reversible. The production-beta kustomization is
  located at `backend/` to satisfy kustomize's default load restrictions.

## 19. Reviewer Checklist

- Scope is limited to deployment composition and manifest policy.
- Production-beta renders 15 services and excludes all-in-one platform runtime.
- Config remains externalized and secrets are not committed.
- Service-to-service URL wiring is explicit.
- Rendered production-beta manifests do not reference `*-dev-*` secret names.
- Kubernetes workload safety basics are present: probes, resources, PDB, HPA,
  non-root pod/container security settings.
- Rollback is a manifest-level revert.

## 20. Status

Status: Approved
