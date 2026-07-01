# Per-service reference manifests (NON-PRODUCTION)

These `<service>/deployment.yaml` files are **reference only** — a sketch of how
each of the 15 logical services *could* be deployed if the modular monolith were
ever split into standalone services. They are deployed by nothing.

The **source of truth** for Production Beta is the 8 deployable units in
[`deploy/k3s/production-beta/backend-units.yaml`](../../k3s/production-beta/backend-units.yaml),
wired through [`backend/kustomization.yaml`](../../../kustomization.yaml). A single
`microservice` binary hosts the 15 logical services, gated by `SERVICE_NAME`
(see `internal/platform/config.go`).

Do not treat anything in this directory as the running configuration.
