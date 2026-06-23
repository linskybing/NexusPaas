# ADR 0005: JWT/JWKS Library Verifier

Date: 2026-06-22

Status: Accepted

## Context

The first-release security tracker required replacing custom JWT/JWKS parsing
and RSA/ECDSA signature verification in the backend platform verifier. The
previous implementation manually parsed compact JWTs, parsed RSA/EC JWKs,
selected keys, and verified RS*/ES* signatures.

Context7 was queried for `github.com/coreos/go-oidc/v3/oidc`, but the MCP
returned an invalid API key error. Implementation used primary sources instead:

- `https://pkg.go.dev/github.com/coreos/go-oidc/v3/oidc`
- `https://github.com/coreos/go-oidc`

## Decision

Use `github.com/coreos/go-oidc/v3` as the single OIDC/JWT/JWK dependency for
platform JWT verification.

The platform keeps the existing internal boundary:

```go
func (v *jwtVerifier) Verify(ctx context.Context, token string) (map[string]any, error)
```

`jwtVerifier` constructs an `oidc.NewRemoteKeySet` from the configured JWKS URL
and an `oidc.NewVerifier` from the configured issuer. The verifier explicitly
allows the existing RS256/RS384/RS512 and ES256/ES384/ES512 algorithms.

`go-oidc` is configured with `SkipClientIDCheck: true` because the platform
trusts a configured audience map, not one client id. Audience validation remains
local against `Config.JWTAudiences`.

`go-oidc` is configured with `SkipExpiryCheck: true` because its built-in time
checks do not exactly match the existing one-minute platform skew contract.
Local validation remains authoritative for required `exp`, `nbf`, and future
`iat`, including the one-minute skew boundary.

## Consequences

- Production code no longer owns custom JWK-to-RSA/EC parsing or custom
  RSA/ECDSA JWT signature verification.
- `JWKSURL`, `JWTIssuer`, and `JWTAudiences` remain the trust roots.
- `sub` remains required.
- JWT `jti` revocation and existing `jwtClaimsUser` role/user mapping are
  preserved.
- HMAC and `none` algorithms remain rejected.
- This closes only the JWT/JWKS library-verifier slice. It does not prove live
  OIDC provider operation, service credential rotation, workload identity,
  mTLS/SPIFFE, Docker-backed readiness, live-cluster readiness, or full GA
  launch readiness.

## Reversal Criteria

Revisit this decision if:

- production tokens are proven not to be compatible with `go-oidc` ID-token
  verification,
- accepted RS256 or ES256 behavior cannot be preserved,
- security review requires a different maintained library,
- the library becomes unmaintained or has unresolved critical verifier issues.

## Verification

Focused tests cover RS256 bearer/cookie authorization, ES256 acceptance,
multi-audience preservation, wrong issuer/audience rejection, missing
subject/expiry rejection, one-minute `exp`/`nbf`/future-`iat` boundary behavior,
malformed/none/HMAC/tampered token rejection, JWKS with no usable signing key
rejection, admin role mapping, token-safe failure logging, and `jti`
revocation.
