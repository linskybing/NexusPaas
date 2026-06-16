# Testing And Delivery

Use this reference when planning test strategy, CI/CD, rollout, and rollback for microservice
systems.

## Test Strategy

- Keep fast unit tests for domain logic inside each service.
- Use integration tests for owned infrastructure boundaries: database, broker, cache, filesystem,
  and external SDK wrappers.
- Add consumer-driven contract tests for service APIs and events.
- Use end-to-end tests sparingly for critical journeys; they are expensive and should not be the
  only confidence source.
- Test failure behavior: timeouts, retries, duplicate events, stale reads, broker lag, and
  downstream outages.

## Contract Testing

- Treat contracts as versioned artifacts.
- Test both request/response APIs and event schemas.
- Keep providers backward-compatible until all consumers have moved.
- Track consumer ownership and compatibility status.
- Add contract checks to CI before deployment.

## CI/CD Expectations

- Build, test, scan, package, and deploy each service independently.
- Keep shared libraries small and stable; avoid forcing synchronized releases.
- Scan dependencies, images, IaC, and policy definitions.
- Promote the same artifact across environments rather than rebuilding per environment.
- Make rollback or roll-forward paths routine, not exceptional.

## Progressive Delivery

- Use canary, blue/green, feature flags, or traffic splitting for risky changes.
- Monitor SLOs, error rates, latency, and business metrics during rollout.
- Stop or roll back automatically when rollout health fails.
- Use idempotent migrations and backward-compatible schema changes.
- Separate deployment from release when product risk is high.

## Environment Strategy

- Prefer production-like dependency behavior in staging for critical workflows.
- Use service virtualization only when real dependencies are too costly or unsafe.
- Seed test data through public APIs or supported fixtures.
- Isolate test tenants, namespaces, accounts, or databases to avoid cross-test contamination.

## Delivery Red Flags

- All services must deploy together.
- Contract changes rely on manual Slack coordination.
- End-to-end tests are the only integration safety net.
- Rollback requires database restore.
- A failed deployment leaves queues, jobs, or migrations in an unknown state.
