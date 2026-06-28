# ADR 0007: Provider Coupling Boundary

Status: Accepted
Date: 2026-06-28

## Context

NexusPaaS currently documents and operates against a reference stack that names
Longhorn, Harbor, MinIO, Dex, Redis Streams, and k3s. Those providers are useful
for local and reference deployments, but they must not be confused with the
portable core contracts owned by NexusPaaS services.

The provider coupling gap remains open because provider adapters, provider
replacement workflows, and live portability proof are not implemented.

## Decision

NexusPaaS separates portable core contracts from current reference providers:

| Area | Portable Core Contract | Current Reference Provider |
| --- | --- | --- |
| Storage backend | Storage records, PVC binding, permissions, mount plans, namespace isolation expectations, degraded-state reporting | Longhorn or another reviewed reference storage backend |
| OCI registry | Image request, allow-list, build governance, catalog sync, scan/status metadata, external promotion/rollback evidence | Harbor |
| Object store | Object key ownership, media/upload metadata, restore expectations, S3-compatible read/write behavior | MinIO/S3 |
| Identity provider | OIDC issuer, JWKS validation, user/session/API-token mapping, role and policy projection inputs | Dex/OIDC |
| Event transport | Event envelope transport for committed outbox rows and inbox consumers | Redis Streams behind the outbox/inbox reliability contract |
| Deploy baseline | Development/reference cluster behavior, smoke evidence, and local operator workflows | k3s/dev reference environment |

This ADR is boundary documentation only. It does not implement provider
adapters, runtime provider selection, migration tooling, configuration changes,
or live portability.

## Consequences

- Service boundaries must describe NexusPaaS-owned contracts first and provider
  names as reference implementations.
- Acceptance ledgers may cite this ADR as started boundary documentation only.
- Provider coupling remains open until implementation adapters and live
  portability evidence prove replacement paths.
- Redis Streams is not the reliability contract; durable outbox/inbox state,
  replay, idempotency, retry, dead-letter, and lag evidence remain the reliability
  boundary.
- k3s remains a development/reference deploy baseline, not the portability
  contract for production runtime targets.

## Follow-up Evidence

- Add provider adapter interfaces only when a replacement path is being built.
- Prove at least one live provider replacement or dual-provider operation before
  claiming portability.
- Keep `problem.md` and the GA acceptance matrix open until the implementation
  and evidence gates pass.

## Reversal

A future ADR may replace this boundary only with a simpler provider strategy
that preserves owned service contracts, trust boundaries, outbox/inbox
reliability, and verifiable rollback evidence.
