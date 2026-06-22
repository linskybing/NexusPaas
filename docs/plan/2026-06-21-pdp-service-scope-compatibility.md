# PDP Service Scope Compatibility

Status: Implemented; Reviewer approved

Reviewer: Boole approved the initial plan and the metadata correction revision.

## Objective

Restore live remote PDP calls that use the already configured
`permissions:enforce` service scope, without broadening API key privileges or
changing production secrets.

## Background

The transactional outbox live smoke exposed a fail-closed integration issue:
`request-notification-service` calls `authorization-policy-service` with the
shared service key from `AUTHORIZATION_POLICY_API_KEY`, and that key is mapped to
the `permissions:enforce` scope. The route scope matcher currently accepts
generic write/read scopes, operation ids, and `service/resource:*` variants, but
not the stable `<resource>:<action>` form used by that service key.

The result is a 403 before `/api/v1/permissions/enforce` reaches the PDP handler,
even though the exact raw policy tuple can match once the request reaches the
handler.

## Scope

- Extend `routeScopeCandidates` to include `<resource>:<route.Action>` and
  `<service>:<route.Action>` candidates.
- Correct the authorization-policy `/api/v1/permissions/enforce` route metadata
  from generic `decisions/create` to `permissions/enforce`, so the existing
  least-privilege `permissions:enforce` service scope describes the real handler.
- Preserve existing scope candidates and behavior.
- Add focused unit coverage for `permissions:enforce` and for a service-level
  action candidate.
- Re-run focused platform tests, full backend tests if touched code passes, quick
  security gate, and Sonar if the code diff remains.
- Re-run the live request-notification form create smoke after the fix using the
  existing `AUTHORIZATION_POLICY_API_KEY` and a temporary exact raw policy row.

## Non-Goals

- No wildcard scope grant.
- No production secret mutation.
- No PDP behavior change after a request reaches the authorization-policy handler.
- No broader route/auth refactor.
- No HTTP route, request body, response body, or policy tuple contract change.

## Acceptance

- A principal with `permissions:enforce` can pass the scope gate for
  `/api/v1/permissions/enforce`.
- Existing admin/wildcard/operation-id scope behavior remains covered.
- Live remote PDP call from request-notification succeeds without secret patching.
- Temporary raw policy rows used for live smoke are removed after the test.

## Implementation Evidence

- Added route action scope candidates for non-empty service/resource parts.
- Corrected `/api/v1/permissions/enforce` metadata to
  `permissions/enforce`.
- Added focused platform scope tests for:
  `permissions:enforce`, `authorization-policy-service:enforce`, operation id,
  wildcard, admin, and unmatched scopes.
- Added authorization-policy spec test for the real enforce route metadata.
- Focused tests passed:
  `go -C backend test ./internal/platform -run 'PrincipalScopesAllow|RemotePDP|ValidatePolicyDecisionPoint|OperationalEndpoints' -count=1`.
- Authorization-policy spec tests passed:
  `go -C backend test ./internal/services/authorizationpolicy -run 'Spec' -count=1`.
- Full backend tests passed:
  `go -C backend test ./... -count=1`.
- Quick gate passed:
  `bash backend/scripts/ci-security-gate.sh quick`.
- Coverage regenerated:
  `go -C backend test ./... -coverprofile=coverage.out -count=1`.
- SonarScanner Quality Gate passed.
- Final live image:
  `localhost:5000/nexuspaas-backend:ci-ga-pdp-scope-20260620163744`
  (`sha256:1817b0c42c37fe6e4d75e1155f7022084aac675dfb52857f16f7b45299b6af62`).
- All 15 backend deployments rolled out to that image and reported `1/1` ready.
- Live direct PDP check using request-notification's existing
  `AUTHORIZATION_POLICY_API_KEY` returned HTTP 200 and `allowed: true` while the
  temporary exact policy row existed.
- After cleanup, the same direct PDP check returned HTTP 200 and
  `allowed: false`, proving the service key still passes the scope gate and the
  denial comes from policy data.
- Temporary raw policy rows and smoke form record were removed; cleanup
  verification returned `0` rows for generic policy, normalized policy, and smoke
  form checks.
