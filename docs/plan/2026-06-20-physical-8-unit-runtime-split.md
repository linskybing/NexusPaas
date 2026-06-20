# Physical 8 Unit Runtime Split

## 1. Objective

Move the Production Beta runtime topology from 15 logical service deployments to
the documented 8 physical deployable units, while keeping the existing single Go
module, service contracts, and external API routes.

## 2. Background

The platform currently deploys one backend container per logical service. The GA
architecture groups those logical services into 8 deployable units. The previous
P0-8 evidence slice only reported that grouping; this slice makes the runtime
and deploy manifests use it.

## 3. Source References

- `docs/architecture/service-boundaries.md`
- `docs/agents/project-structure.md`
- `backend/internal/platform/config.go`
- `backend/internal/services/catalog.go`
- `backend/kustomization.yaml`
- `backend/deploy/local/collaboration-smoke.compose.yml`
- `backend/scripts/ci-security-gate.sh`
- `backend/internal/platform/deployment_test.go`
- `backend/internal/e2e/compose_collaboration_smoke_test.go`

## 4. Assumptions

- Physical split means 8 Kubernetes/Compose backend deployments, not 8 Go
  modules or repositories.
- `SERVICE_NAME=<unit>` should host all logical services mapped to that unit.
- `SERVICE_URLS` remains keyed by logical service name, with values pointing to
  the owning unit service.
- Shared Postgres remains a temporary migration scaffold; no new cross-service
  writes are introduced.

## 5. Non-Goals

- No database-per-unit migration.
- No package extraction into separate binaries.
- No service mesh, mTLS, SPIFFE, or workload identity changes.
- No external API route changes.
- No new dependency.

## 6. Current Behavior

`Config.AllowsService` allows only `all` or an exact logical service name.
Production Beta manifests and collaboration smoke start 15 backend service
containers.

## 7. Target Behavior

The runtime accepts the 8 deployable-unit names as `SERVICE_NAME` values and
registers the logical services in that unit. Production Beta manifests and the
Docker collaboration smoke topology start 8 backend containers. Operational
checks still validate all 15 logical services through the 8 physical units.

## 8. Affected Domains

- Platform runtime service selection.
- Production Beta Kubernetes topology.
- Local/Docker collaboration smoke topology.
- CI release gate evidence.
- Readiness and architecture docs.

## 9. Affected Files

- `backend/internal/platform/config.go`
- `backend/internal/platform/config_test.go`
- `backend/internal/platform/service_isolation_test.go`
- `backend/internal/services/catalog_test.go`
- `backend/internal/e2e/compose_collaboration_smoke_test.go`
- `backend/deploy/local/collaboration-smoke.compose.yml`
- `backend/kustomization.yaml`
- `backend/deploy/k3s/production-beta/runtime-config.yaml`
- `backend/deploy/k3s/production-beta/runtime-secret-contract.yaml`
- `backend/scripts/ci-security-gate.sh`
- `backend/docs/beta-launch-readiness.md`
- `backend/docs/operational-readiness.md`

## 10. API / Contract Changes

No external API changes. Runtime configuration now accepts these `SERVICE_NAME`
values:

- `platform-gateway`
- `iam-unit`
- `tenant-unit`
- `collaboration-unit`
- `platform-io-unit`
- `usage-observability`
- `compute-api`
- `compute-control-plane`

## 11. Database / Migration Changes

None.

## 12. Configuration Changes

Production Beta `SERVICE_URLS` remains a logical-service map, but grouped
logical services point to the same physical unit URL.

## 13. Observability Changes

`OTEL_SERVICE_NAME` for grouped deployments should be the physical unit name.
Service registry output continues to list the logical services hosted by the
current process.

## 14. Security Considerations

Service-to-service authentication keeps using `SERVICE_API_KEY` for this Beta
slice. The authorization-policy unit is co-hosted with identity in `iam-unit`,
but domain authorization remains enforced in handlers/middleware.

## 15. Implementation Steps

1. Add a small unit-to-logical-service map in platform config and make
   `AllowsService` honor unit aliases.
2. Add tests proving each unit hosts the intended logical services and that
   object-store requirements follow `collaboration-unit`.
3. Replace 15-service Production Beta backend resources with 8 unit manifests or
   generated-from-existing YAML.
4. Update local collaboration smoke to start 8 unit containers while still
   routing logical service calls through logical `SERVICE_URLS`.
5. Update the CI gate to expect 8 physical deployments and 15 logical service
   registry/smoke entries.
6. Update docs to say Production Beta now runs 8 physical backend units hosting
   15 logical services.

## 16. Verification Plan

```sh
bash -n backend/scripts/ci-security-gate.sh
go -C backend test ./internal/platform -run 'Config|ServiceIsolation|Deployment' -count=1
go -C backend test ./internal/services -run 'Catalog|DeploymentArtifacts|GatewayOnly' -count=1
go -C backend test -tags e2e ./internal/e2e -run 'ComposeCollaborationSmoke|ServiceRouteIsolationContract|IsolatedRuntimeRegistrationE2E' -count=1
bash backend/scripts/ci-security-gate.sh quick
```

Full release validation remains:

```sh
bash backend/scripts/ci-security-gate.sh beta-rc
```

## 17. Rollback Plan

Revert this slice. The previous 15-service topology has no schema dependency on
the new unit aliases.

## 18. Risks and Tradeoffs

Co-hosting multiple logical services in one unit reduces process isolation
compared with 15 deployments, but matches the documented 8-unit GA direction and
lowers operational overhead. Shared database ownership remains a known GA
blocker handled by later slices.

## 19. Reviewer Checklist

- Exactly 8 backend physical deployments are rendered.
- All 15 logical services are still reachable through service registry and smoke.
- `SERVICE_URLS` logical names point to owning unit URLs.
- No external route, migration, or new dependency is introduced.
- Tests and quick gate pass.

## 20. Status

Status: Approved and implemented.

## 21. Implementation Evidence

Implemented on 2026-06-20:

- Runtime `SERVICE_NAME` aliases for the 8 physical backend units.
- Production Beta kustomize topology with 8 backend unit Deployments, Services,
  PDBs, HPAs, runtime ConfigMaps, and unit runtime Secret contracts.
- Logical `SERVICE_URLS` mapping from all 15 logical services to their owning
  physical unit URLs.
- Local Docker collaboration topology using 8 backend unit containers.
- CI/release evidence generation for 8 backend unit deployments, 15 logical
  smoke endpoints, and unit-level rollback commands.
- Production Beta observability overlay and documentation aligned to 8 physical
  backend units hosting 15 logical services.
- coturn security-context hardening required for Sonar Quality Gate pass.

Verified commands:

```sh
bash -n backend/scripts/ci-security-gate.sh
kubectl kustomize backend
kubectl apply --dry-run=client --validate=false -f /tmp/nexuspaas-production-beta-render.yaml
go -C backend test ./internal/platform -count=1
bash backend/scripts/ci-security-gate.sh quick
TEST_MINIO_PORT=19100 TEST_MINIO_CONSOLE_PORT=19101 bash backend/scripts/ci-security-gate.sh docker
TEST_MINIO_PORT=19100 TEST_MINIO_CONSOLE_PORT=19101 bash backend/scripts/ci-security-gate.sh beta-rc
```

Release-candidate artifact:

- `/tmp/nexuspaas-quality-gate/local-1259865/beta-rc-report.md`

## 22. Reviewer Verification

Reviewer Agent status: approved after implementation.

- Requirement fit: Production Beta now runs as 8 physical backend units hosting
  15 logical services; `SERVICE_NAME=all` remains available for local all-in-one
  smoke.
- Approved-plan alignment: implementation follows the approved alias,
  manifest, compose, CI evidence, and docs-only database scope.
- Service boundaries: the runtime map keeps logical service ownership explicit;
  grouped units do not introduce new shared writes or external API changes.
- 12-Factor App: configuration remains environment-driven through ConfigMaps,
  Secrets, and `SERVICE_NAME`/`SERVICE_URLS`.
- Tests/build: quick gate, Docker-backed gate, and beta-rc gate passed.
- SonarScanner Quality Gate: passed in the full beta-rc run.
- Risk status: live staging deploy/smoke/rollback evidence remains outside this
  non-live slice and is still tracked in `problem.md`.
