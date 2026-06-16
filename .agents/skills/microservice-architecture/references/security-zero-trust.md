# Security And Zero Trust

Use this reference when designing authentication, authorization, identity propagation, service mesh,
secrets, network policy, or security gates.

## Security Posture

- Treat every service, API, database, broker, and workload identity as a protected resource.
- Do not trust network location. Authenticate and authorize every interaction.
- Use defense in depth: gateway checks, service-level checks, and business-rule checks inside the
  service.
- Prefer platform-provided security libraries and policy engines over custom per-service
  authorization systems.

## Gateway Is Not Enough

- Gateways are useful for coarse controls, authentication integration, request filtering, rate
  limits, and routing.
- Service-level authorization is still required for internal calls and direct service exposure
  mistakes.
- Domain-specific access rules belong in the service that owns the domain behavior.
- Avoid putting domain authorization decisions exclusively in a proxy.

## Identity Propagation

- Propagate caller context with a trusted internal representation, not raw browser tokens passed
  through every service.
- Keep internal identity tokens or passports private to internal traffic.
- Include only the claims needed by downstream services.
- Bind identity propagation to trace/correlation context where useful for audit.
- Define how service identities and end-user identities combine for delegated operations.

## Service-To-Service Authentication

- Use mTLS, workload identity, signed service tokens, or service mesh identity where the platform
  supports them.
- Rotate keys and certificates automatically.
- Scope service credentials to the minimum needed resources.
- Avoid long-lived static shared secrets between services.
- Add network policy or equivalent segmentation around service groups.

## Policy-As-Code And DevSecOps

- Store policies in version control.
- Test policy changes in CI/CD.
- Enforce policy at deployment and runtime where possible.
- Add image scanning, dependency scanning, IaC scanning, and admission controls for production
  workloads.
- Track exceptions with owner, expiry, and compensating controls.

## Logging And Secrets

- Sanitize logs for credentials, tokens, API keys, PII, and sensitive payloads.
- Include correlation IDs in logs for cross-service investigations.
- Use structured logs with service, version, environment, trace/span IDs, and security-relevant
  context.
- Store secrets in a secrets manager or orchestrator secret mechanism, not in source, images, or
  plain config files.
- Audit administrative and cross-tenant operations.
