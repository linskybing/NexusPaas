# Coding Guidelines

These rules reduce common agent coding mistakes.

## 1. Think Before Coding

Do not assume. Do not hide confusion.

Before implementation:

- State assumptions.
- Identify unclear requirements.
- Surface tradeoffs.
- Ask or document uncertainty.
- Push back against unnecessary complexity.

## 2. Simplicity First

Use the minimum code that solves the requirement.

Rules:

- No features beyond what was asked.
- No speculative abstractions.
- No configurability unless required.
- No new framework unless approved in the plan.
- No code for future use.
- If the solution is larger than necessary, simplify it.

## 3. Surgical Changes

Touch only what is required.

Rules:

- Do not refactor unrelated code.
- Do not reformat unrelated files.
- Match existing style.
- Remove only unused code created by the current change.
- Mention unrelated dead code; do not delete it without approval.

Every changed line must trace back to the approved plan.

## 4. Goal-Driven Execution

Convert tasks into verifiable goals.

Example:

```text
1. Add request validation
   verify: invalid input tests fail before implementation and pass after

2. Extract application service
   verify: existing API behavior remains unchanged

3. Add configuration
   verify: value is loaded from environment, not hardcoded
````

## 5. Backend Architecture Rules

Prefer clear layers:

```text
transport / handler
application / usecase
domain
repository / infrastructure adapter
```

Rules:

* Domain logic must not depend on HTTP handlers.
* Domain logic must not directly depend on database clients.
* Infrastructure should implement application/domain interfaces.
* Interfaces should be small.
* Avoid interfaces for single-use code unless needed for testing or boundaries.
* Avoid circular dependencies.

## 6. API Rules

* Preserve existing API behavior unless the approved plan changes it.
* Keep error response format consistent.
* Keep authentication and authorization behavior consistent.
* Add contract tests for service boundary changes.
* Version breaking changes.

## 7. Configuration Rules

* No hardcoded environment-specific values.
* No committed secrets.
* Required config must fail fast at startup.
* Defaults must be safe for local development only.
* Config names must be documented.

## 8. Logging and Observability Rules

* Log to stdout/stderr.
* Do not log secrets, tokens, passwords, or private data.
* Include request, job, service, or trace identifiers when available.
* Avoid noisy logs in hot paths.
* Add metrics/traces only when required by the plan.

## 9. Testing Rules

* Add tests for changed behavior.
* Prefer focused unit tests for domain/application logic.
* Add integration tests for database, Redis, Kubernetes, or service contract changes.
* Do not weaken existing tests.
* Do not delete failing tests unless the behavior is intentionally removed.
* Run relevant verification before requesting review.

## 10. Forbidden Patterns

Do not:

* Implement unapproved work.
* Mix unrelated refactors into the task.
* Reformat large unrelated areas.
* Add speculative shared SDKs.
* Add generic event buses without requirement.
* Add distributed transactions without approval.
* Share mutable database ownership across services.
* Hardcode config or secrets.
* Silence tests by weakening assertions.
