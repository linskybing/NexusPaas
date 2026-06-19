# GA Testing Strategy

## Test Pyramid

GA architecture confidence comes from layered tests, not more live E2E alone.

| Layer | Purpose | Gate |
| --- | --- | --- |
| Unit | Domain logic, validation, policy, mappers, state transitions | Required in package tests. |
| Integration | Owned DB, Redis/event bus, object store, Kubernetes/Harbor adapters | Required for services that own the boundary. |
| Contract | Internal HTTP APIs, events, provider/consumer compatibility | Required before any cross-unit contract changes. |
| E2E | Critical user journeys across units | Required for core journeys only. |
| Live staging | Deploy, smoke, rollback, redeploy, external dependencies | Required for GA evidence, opt-in locally. |
| Failure mode | Timeout, retry, duplicate event, stale read, downstream outage | Required for migration slices and compute saga. |

## Required Contract Coverage

- Every internal owner-read or command API has a provider test and at least one
  consumer test.
- Every event has a schema fixture, producer test, consumer idempotency test,
  and compatibility rule.
- Contract tests must cover additive schema changes, unknown fields, missing
  optional fields, duplicate events, and stale read model behavior.
- Consumers must remain compatible until the producer deprecation window closes.

## E2E Scope

Required E2E focuses on core user journeys:

- login/auth projection, project membership, plan/queue binding;
- ConfigFile lifecycle and job/Pod/Deployment dispatch;
- runtime duration cleanup, plan-window eviction, preemption;
- image request/build governance and catalog visibility;
- storage mount-plan and IDE lifecycle;
- service isolation, bad credentials, missing dependency fail-closed behavior.

Live E2E remains opt-in for Kubernetes, LDAP, Harbor, Longhorn, and other
external systems. Live skips must happen before mutating external resources and
must state the missing prerequisite.

## Staging Evidence

Each deployable unit must capture:

- applied candidate version and config source;
- `/healthz`, `/readyz`, `/metrics`, and service-registry evidence;
- one read-only synthetic smoke endpoint per logical service in the unit;
- critical cross-unit journey smoke where applicable;
- rollback command, rollback result, redeploy result, and post-redeploy smoke;
- request IDs, trace IDs, version, and timestamps for evidence artifacts.

## Baseline Commands

Local and CI gates continue to include:

```sh
go -C backend test ./... -count=1
go -C backend vet ./...
go -C backend build ./...
bash backend/scripts/ci-security-gate.sh quick
```

Architecture and release branches should additionally run focused E2E, Docker
collaboration smoke, security scans, and Sonar when credentials are configured.
Live staging evidence is mandatory before a GA architecture milestone can be
called complete.

## Acceptance Criteria

- No new unregistered cross-service store dependency.
- No unversioned internal contract or event schema.
- No migration without rollback and reconciliation evidence.
- No deployable unit without owner, SLO, runbook, smoke, and rollback evidence.
- No PR merge if a required gate is skipped without an owner-approved reason.
