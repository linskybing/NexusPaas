# ADR 0003: GA Service Identity Direction

Status: Accepted
Date: 2026-06-19

## Context

Production Beta supports static `SERVICE_API_KEY` for service-to-service calls.
That is acceptable as a transition fallback, but GA decomposition requires a
rotatable, workload-bound identity model for independently deployable units.
Gateway authentication alone is not sufficient because each owning unit must
authenticate callers, enforce domain authorization, and emit evidence that the
right service identity was used.

The roadmap names service identity as a remaining blocker. This ADR records the
GA direction without changing runtime configuration in this docs-only slice.

## Decision

The GA service-to-service path will use Kubernetes workload identity or an
approved equivalent as the preferred service identity mechanism. Static
`SERVICE_API_KEY` remains a Production Beta fallback and emergency transition
mechanism until staging proves the replacement path.

Each deployable unit must authenticate incoming internal calls with service
identity and must authorize the requested operation at the owning unit. The
gateway can authenticate edge users and route traffic, but it does not replace
unit-level service identity or domain authorization.

Service mesh or mTLS can be added later if workload identity, network policy,
and observability are insufficient for a concrete production need. It is not a
Day 0-15 prerequisite.

## Requirements

- Service identity is workload-bound, rotatable, and environment-scoped.
- Internal callers carry both user context where allowed and service identity for
  the calling workload.
- Owning units enforce authorization for domain operations instead of trusting
  gateway routing alone.
- Policy tests cover allowed caller, denied caller, missing identity, bad
  identity, expired or revoked identity, and forged user headers.
- Observability records safe service identity labels or trace attributes without
  logging raw credentials, tokens, cookies, or assertions.
- Rollback can restore static-key fallback only as an explicit, time-bound
  transition with `problem.md` tracking.

## Compatibility And Contract Requirements

- External `/api/v1` compatibility does not change.
- Existing Beta service calls may keep static keys until replacement staging
  evidence exists.
- Future runtime slices must document issuer, audience, rotation, revocation,
  policy mapping, local-development behavior, and emergency fallback.
- Service identity changes that touch cross-service behavior require focused
  integration or E2E tests for critical workflows.

## Consequences

- GA security posture moves from shared static secrets toward workload-bound
  trust.
- Secret leakage risk decreases when raw service keys no longer carry the normal
  staging and GA path.
- Operators need evidence for identity issuance, rotation, revocation, denied
  caller behavior, and trace-safe labeling.
- Implementation must account for local developer workflows and opt-in live
  Kubernetes prerequisites.

## Rejected Alternatives

| Alternative | Reason Rejected |
| --- | --- |
| Keep static `SERVICE_API_KEY` as the GA path | Static shared secrets are harder to rotate, scope, revoke, and audit across deployable units. |
| Trust only the gateway | Internal unit calls and background workers still need caller identity and domain authorization. |
| Add service mesh before workload identity evidence | Adds operational complexity before the immediate identity and policy gap is proven closed. |
| Log raw service credentials for debugging | Violates secret-handling rules and increases incident blast radius. |

## Follow-up Evidence

- Choose and document the exact workload identity or approved equivalent for
  staging.
- Add policy tests for allowed, denied, missing, forged, revoked, and expired
  internal identities.
- Capture staging evidence for identity issuance, rotation or replacement,
  trace-safe labels, and fail-closed behavior.
- Keep the service identity blocker open in `problem.md` until the staging GA
  path no longer depends on static keys.

## Reversal

A future ADR can choose another identity mechanism only if it is workload-bound,
rotatable, observable without leaking credentials, and supported by policy tests
and staging evidence. Reversal must preserve external `/api/v1` compatibility and
must not broaden secret exposure.
