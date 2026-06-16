# Backend Production Beta Problem Review

> Refreshed 2026-06-17 for the Docker E2E gate PR. Current branch:
> `test/docker-e2e-gate` based on `origin/main = 150f1ad feat: identity
> data-boundary first slice`.

## Summary

NexusPaas backend is functionally strong enough to enter Production Beta
hardening, but it is not ready for formal beta launch until verification,
deployment, security, observability, and remaining service-boundary gaps are
made reproducible and reviewable.

This PR addresses the first process blocker by adding a repository-owned
Docker-backed verification gate:

- isolated Postgres/Redis/MinIO on non-default ports, avoiding Sonar `9000`
- migration apply/validate, including a fixed identity `login_failures`
  owned-table `created_at` backfill path caught by the gate
- media bucket provisioning
- integration coverage with `coverage.out`
- focused cross-service E2E with no required-test skips
- full non-live E2E
- runtime smoke for `/healthz`, `/readyz`, `/metrics`, `/openapi.json`,
  `/service-registry`, and one endpoint per registered service

## Current Open Problems

| Priority | Area | Problem | Status | Required Action |
| --- | --- | --- | --- | --- |
| Closed | Verification | DB-backed coverage and E2E previously required manual Docker setup. | Closed in this PR | `backend/scripts/docker-e2e-gate.sh` now runs migrations, integration coverage, focused/full E2E, and runtime smoke on isolated Docker backing services. Wire it into CI in a later PR. |
| Medium | Security tooling | `govulncheck`, `osv-scanner`, and local container scanners (`trivy`/`grype`/`syft`) are not installed in this workstation. | Open | Add pinned security scan tooling to the CI/security gate PR. |
| Medium | Reference parity | `references/` is gitignored and absent, so parity/OpenAPI diffs are not reproducible from a clean checkout. | Open | Vendor or deterministically fetch a pinned reference snapshot before parity tests. |
| Medium | Deployment | k3s kustomization still deploys all-in-one `platform.yaml`, not the requested 15-service Production Beta shape. | Open | Add production-beta kustomization that includes the 15 service manifests and service URL wiring. |
| Medium | Service boundaries | scheduler-quota still has transition dependencies on org-project/workload owned data. | Open | Replace shared-store reads with owner read contracts, events, or read models. |
| Low/Medium | Coverage | Some packages remain below ideal package-local coverage even when aggregate coverage passes. | Open | Add focused tests for `cmd/microservice`, identity, image registry, k8s control, and workload paths. |
| Low | Maintainability | `internal/services/catalog.go` and `internal/platform/config.go` exceed the 800-line guideline. | Open | Split by domain/concern after higher-priority launch gates are stable. |

## Production Beta Gate Status

| Gate | Status | Notes |
| --- | --- | --- |
| Core API build/test | Passing | `gofmt -l .`, `go build ./...`, `go vet ./...`, and `go test ./... -count=1` passed locally. |
| Docker-backed integration/E2E/runtime smoke | Passing | Local run passed with aggregate integration coverage 80.29%, required focused E2E PASS, full non-live E2E PASS, 15 registered services, and no smoke endpoint 5xx. |
| 15-service Kubernetes deployment | Not ready | Current k3s root still targets all-in-one runtime. |
| Security scans and Sonar QG | Partial | SonarScanner Quality Gate passed against local SonarQube with `SONAR_TOKEN`; govulncheck, OSV, and container scanners were unavailable locally. |
| Observability/runbooks/SLOs | Not ready | Requires follow-up Production Observability PR. |
| Remaining data-boundary cleanup | Not ready | scheduler-quota owner-read dependencies remain. |

## Reviewer Status

Status: Approved for this PR

The Docker gate PR closes the first reproducibility slice. Production Beta
remains blocked on 15-service deployment, CI/security gates,
observability/runbooks, and remaining cross-service data-boundary work.
