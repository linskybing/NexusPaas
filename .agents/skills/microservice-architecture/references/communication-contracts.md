# Communication Contracts

Use this reference when designing APIs, events, gateway behavior, and versioning.

## Choose The Communication Style

- Use synchronous request/response for immediate reads, command acceptance, and low-latency
  interactions where the caller needs a direct answer.
- Use asynchronous messaging for workflows that can tolerate delay, fan-out, decoupling, retries, or
  buffering.
- Use events to publish facts that already happened, not commands disguised as facts.
- Avoid synchronous chains longer than the latency and availability budget can support.

## API Design

- Model APIs around domain behavior and resources, not internal tables.
- Keep contracts coarse enough to avoid chatty calls.
- Hide implementation details such as schema names, storage layout, internal enum leaks, or
  framework-specific errors.
- Define error shape, retryability, idempotency keys, and timeout expectations.
- Version public and cross-service APIs with backward-compatible evolution as the default.

## Events And Messages

- Name events as past-tense facts, for example `OrderPlaced`.
- Include event ID, schema version, producer, timestamp, aggregate ID, and correlation or causation
  IDs.
- Make consumers idempotent; delivery may be duplicated or delayed.
- Use durable brokers for critical workflows. Do not replace a broker with best-effort HTTP
  callbacks unless loss is acceptable.
- Define retention, replay policy, poison-message handling, and dead-letter routing before
  production.

## Gateway Policy

- Let gateways handle routing, TLS termination, authentication integration, coarse authorization,
  rate limits, request shaping, and cross-cutting logging.
- Keep business rules and domain orchestration out of the gateway.
- Avoid gateway aggregation when it turns the gateway into a domain service.
- For client-specific aggregation, consider a backend-for-frontend only when the client experience
  truly needs it.

## Versioning And Compatibility

- Prefer additive changes: new fields, new event versions, and tolerant readers.
- Never remove or repurpose fields until all consumers have migrated.
- Track producers and consumers of every contract.
- Add consumer-driven contract tests for critical integrations.
- Set an explicit deprecation window for breaking changes.

## Coupling Signals

- Consumers know provider database structure.
- A change in one service forces same-day redeploy of several others.
- Internal APIs expose implementation-specific state transitions.
- Multiple services must coordinate to validate a single business invariant.
- Request chains grow because one service lacks the data it should own.
