# Resilience And Runtime

Use this reference when designing production behavior, failure handling, or runtime platform
expectations.

## Failure Model

- Assume remote calls can fail, timeout, duplicate, reorder, or return stale information.
- Define the user-visible behavior for dependency failure: fail closed, degrade, queue, compensate,
  or serve cached data.
- Avoid making one service's partial outage cascade into a platform outage.
- Treat retries, queues, and caches as capacity tools that need limits.

## Timeout And Retry Rules

- Set explicit timeouts on every network call.
- Ensure caller timeout budgets are shorter than user-facing request budgets.
- Retry only transient failures and only for idempotent operations.
- Use exponential backoff with jitter and a bounded retry count.
- Fail fast for validation errors, authorization errors, missing resources, and other non-transient
  failures.

## Circuit Breakers And Bulkheads

- Use circuit breakers around unreliable or high-latency dependencies.
- Open the circuit after repeated failures and return a controlled fallback or immediate error.
- Add metrics for circuit state, open duration, and fallback rate.
- Use bulkheads to isolate thread pools, queues, connection pools, and worker capacity by dependency
  or tenant where needed.
- Add load shedding when queues or concurrency limits exceed safe thresholds.

## Stateless Runtime

- Keep service processes stateless. Persist durable state in backing services.
- Do not rely on sticky sessions or local disk for user/session durability.
- Store deployment-varying configuration outside code.
- Treat databases, brokers, caches, and third-party APIs as attached resources addressed by
  configuration.
- Stream logs to stdout/stderr or the platform logging path; do not require services to manage log
  files.

## Orchestration And Health

- Use orchestration platforms for scheduling, restarts, scaling, service discovery, and rolling
  updates.
- Separate liveness, readiness, and startup checks.
- Make readiness depend on critical local prerequisites, not every downstream service in a way that
  causes restart loops.
- Implement graceful shutdown and drain in-flight requests or messages.
- Keep background workers bounded and observable.

## Runtime Red Flags

- Infinite retries or retry storms during outages.
- No timeout on HTTP, database, broker, or cloud SDK calls.
- One shared connection pool for unrelated dependencies.
- Long synchronous call chains in user-facing requests.
- Services that only work when started in a specific order.
